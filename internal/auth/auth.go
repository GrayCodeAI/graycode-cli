package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/GrayCodeAI/hawk/internal/storage"
)

// TokenStore manages authentication tokens.
type TokenStore struct {
	tokens map[string]string // provider -> token
}

// NewTokenStore creates a new token store.
func NewTokenStore() *TokenStore {
	return &TokenStore{tokens: make(map[string]string)}
}

// Load loads tokens from secure storage.
// Deprecated: Use SecureStorage directly to load tokens. This stub always
// returns an empty token map. Migrate callers to SecureStorage.Get/Set.
func (t *TokenStore) Load() error {
	// Stub: no-op. Existing callers that relied on this get an empty token
	// map. New code should use SecureStorage directly.
	t.tokens = make(map[string]string)
	return nil
}

// Save saves tokens to secure storage.
// Deprecated: Use SecureStorage directly to persist tokens. This stub is a
// no-op. Migrate callers to SecureStorage.Set.
func (t *TokenStore) Save() error {
	// Stub: no-op. Tokens held in-memory only; they are lost on process exit.
	// Use SecureStorage for persistent, OS-keychain-backed storage.
	return nil
}

// Get returns a token for a provider.
func (t *TokenStore) Get(provider string) string {
	return t.tokens[provider]
}

// Set sets a token for a provider.
func (t *TokenStore) Set(provider, token string) {
	t.tokens[provider] = token
}

// Has returns true if a token exists for a provider.
func (t *TokenStore) Has(provider string) bool {
	_, ok := t.tokens[provider]
	return ok
}

// SecureStorage handles secure token storage using OS keychain/keyring.
type SecureStorage struct {
	service string
}

// NewSecureStorage creates a new secure storage.
func NewSecureStorage(service string) *SecureStorage {
	return &SecureStorage{service: service}
}

// Get retrieves a token from secure storage.
func (s *SecureStorage) Get(account string) (string, error) {
	if runtime.GOOS == "darwin" {
		return s.getMacOS(account)
	}
	if runtime.GOOS == "windows" {
		return s.getWindows(account)
	}
	// Fallback to file-based storage for Linux (keyring handled by eyrie layer)
	return s.getFile(account)
}

// Set stores a token in secure storage.
func (s *SecureStorage) Set(account, token string) error {
	if runtime.GOOS == "darwin" {
		return s.setMacOS(account, token)
	}
	if runtime.GOOS == "windows" {
		return s.setWindows(account, token)
	}
	return s.setFile(account, token)
}

func (s *SecureStorage) getMacOS(account string) (string, error) {
	// Use security command to get from keychain
	data, err := execCommand("security", "find-generic-password", "-s", s.service, "-a", account, "-w")
	if err != nil {
		return "", err
	}
	return data, nil
}

func (s *SecureStorage) setMacOS(account, token string) error {
	// Feed the command to `security -i` via stdin so the secret never appears
	// in the process argument list (argv is visible to all local users via ps
	// for the lifetime of the call).
	if strings.ContainsAny(s.service+account+token, "\n\r") {
		return fmt.Errorf("keychain values must not contain newlines")
	}
	cmd := exec.CommandContext(context.Background(), "security", "-i")
	cmd.Stdin = strings.NewReader("add-generic-password -U -s " + securityQuote(s.service) +
		" -a " + securityQuote(account) + " -w " + securityQuote(token) + "\n")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("security add-generic-password: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// securityQuote quotes an argument for the `security -i` interactive command
// parser: wraps it in double quotes and escapes backslashes and double quotes.
func securityQuote(v string) string {
	r := strings.NewReplacer("\\", "\\\\", "\"", "\\\"")
	return "\"" + r.Replace(v) + "\""
}

// powershellQuote escapes a string for single-quoted PowerShell string literal
// context by doubling any embedded single quotes.
func powershellQuote(v string) string {
	return strings.ReplaceAll(v, "'", "''")
}

// winCredScriptPrefix defines C# P/Invoke signatures for the Windows
// Credential Manager API (advapi32.dll) and exposes [WinCred]::Get and
// [WinCred]::Set static methods. Works on all Windows 7+ installations
// without requiring extra PowerShell modules.
var winCredScriptPrefix = []string{
	"$code = @\"",
	"using System;",
	"using System.Runtime.InteropServices;",
	"using System.Text;",
	"public static class WinCred {",
	"    [DllImport(\"advapi32.dll\", SetLastError = true, CharSet = CharSet.Unicode)]",
	"    static extern bool CredRead(string target, int type, int reservedFlag, out IntPtr credentialPtr);",
	"    [DllImport(\"advapi32.dll\", SetLastError = true, CharSet = CharSet.Unicode)]",
	"    static extern bool CredWrite(ref CREDENTIAL credential, uint flags);",
	"    [DllImport(\"advapi32.dll\", SetLastError = true)]",
	"    static extern void CredFree(IntPtr buffer);",
	"    [StructLayout(LayoutKind.Sequential, CharSet = CharSet.Unicode)]",
	"    struct CREDENTIAL {",
	"        public int flags;",
	"        public int type;",
	"        public IntPtr targetName;",
	"        public IntPtr comment;",
	"        public long lastWritten;",
	"        public int credentialBlobSize;",
	"        public IntPtr credentialBlob;",
	"        public int persist;",
	"        public int attributeCount;",
	"        public IntPtr attributes;",
	"        public IntPtr targetAlias;",
	"        public IntPtr userName;",
	"    }",
	"    const int CRED_TYPE_GENERIC = 1;",
	"    const int CRED_PERSIST_LOCAL_MACHINE = 2;",
	"    public static string Get(string target) {",
	"        IntPtr ptr;",
	"        if (!CredRead(target, CRED_TYPE_GENERIC, 0, out ptr)) return null;",
	"        var c = (CREDENTIAL)Marshal.PtrToStructure(ptr, typeof(CREDENTIAL));",
	"        string pass = c.credentialBlob != IntPtr.Zero",
	"            ? Marshal.PtrToStringUni(c.credentialBlob, c.credentialBlobSize / 2)",
	"            : null;",
	"        CredFree(ptr);",
	"        return pass;",
	"    }",
	"    public static void Set(string target, string user, string password) {",
	"        var c = new CREDENTIAL();",
	"        c.type = CRED_TYPE_GENERIC;",
	"        c.targetName = Marshal.StringToCoTaskMemUni(target);",
	"        c.userName = Marshal.StringToCoTaskMemUni(user);",
	"        byte[] bytes = Encoding.Unicode.GetBytes(password);",
	"        c.credentialBlob = Marshal.AllocCoTaskMem(bytes.Length);",
	"        Marshal.Copy(bytes, 0, c.credentialBlob, bytes.Length);",
	"        c.credentialBlobSize = bytes.Length;",
	"        c.persist = CRED_PERSIST_LOCAL_MACHINE;",
	"        try {",
	"            CredWrite(ref c, 0);",
	"        } finally {",
	"            Marshal.ZeroFreeCoTaskMemUnicode(c.targetName);",
	"            Marshal.ZeroFreeCoTaskMemUnicode(c.userName);",
	"            byte[] zeros = new byte[bytes.Length];",
	"            Marshal.Copy(zeros, 0, c.credentialBlob, bytes.Length);",
	"            for (int i = 0; i < bytes.Length; i++) bytes[i] = 0;",
	"            Marshal.FreeCoTaskMem(c.credentialBlob);",
	"        }",
	"    }",
	"}",
	"\"@",
	"Add-Type -TypeDefinition $code -Language CSharp",
}

func buildWinCredScript(tail string) string {
	return strings.Join(winCredScriptPrefix, "\n") + "\n" + tail
}

func (s *SecureStorage) getWindows(account string) (string, error) {
	target := s.service + "::" + account
	script := buildWinCredScript(fmt.Sprintf("[WinCred]::Get('%s')", powershellQuote(target)))
	// Pipe the script via stdin so the credential target is not exposed
	// in process argv (visible to all local users via ps/Process Explorer).
	cmd := exec.CommandContext(context.Background(), "powershell.exe", "-NoProfile", "-Command")
	cmd.Stdin = strings.NewReader(script)
	data, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (s *SecureStorage) setWindows(account, token string) error {
	target := s.service + "::" + account
	script := buildWinCredScript(fmt.Sprintf(
		"[WinCred]::Set('%s', '%s', '%s')",
		powershellQuote(target), powershellQuote(account), powershellQuote(token),
	))
	cmd := exec.CommandContext(context.Background(), "powershell.exe", "-NoProfile", "-Command")
	cmd.Stdin = strings.NewReader(script)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("windows credential store: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (s *SecureStorage) getFile(account string) (string, error) {
	path := filepath.Join(storage.ConfigDir(), ".tokens")
	data, err := os.ReadFile(path) // #nosec G304 -- path is the fixed internal token store location, not external input
	if err != nil {
		return "", err
	}
	var tokens map[string]string
	if err := json.Unmarshal(data, &tokens); err != nil {
		return "", err
	}
	return tokens[account], nil
}

func (s *SecureStorage) setFile(account, token string) error {
	path := filepath.Join(storage.ConfigDir(), ".tokens")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	var tokens map[string]string
	// #nosec G304 -- path is the fixed internal token store location, not external input
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &tokens)
	}
	if tokens == nil {
		tokens = make(map[string]string)
	}
	tokens[account] = token
	data, _ := json.Marshal(tokens)
	return os.WriteFile(path, data, 0o600)
}

// GenerateNonce generates a random nonce for OAuth.
func GenerateNonce() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}

func execCommand(name string, args ...string) (string, error) {
	cmd := exec.CommandContext(context.Background(), name, args...) // #nosec G204 -- executable is selected by the platform credential backend
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("%s: %w: %s", name, err, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return "", fmt.Errorf("%s: %w", name, err)
	}
	return strings.TrimSpace(string(out)), nil
}
