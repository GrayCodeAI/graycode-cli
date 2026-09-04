package env

import (
	"os"
	"strings"
	"testing"
)

func TestSubprocessEnv_ScrubsCredentials(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-secret")
	t.Setenv("OPENAI_API_KEY", "sk-openai-secret")
	t.Setenv("HOME", "/Users/test")
	t.Setenv("PATH", "/usr/bin:/bin")
	t.Setenv("GRAYCODE_SESSION_ID", "abc123")

	got := SubprocessEnv()

	values := map[string]string{}
	for _, kv := range got {
		name, val, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		values[name] = val
	}
	if _, ok := values["ANTHROPIC_API_KEY"]; ok {
		t.Fatal("ANTHROPIC_API_KEY leaked into subprocess env")
	}
	if _, ok := values["OPENAI_API_KEY"]; ok {
		t.Fatal("OPENAI_API_KEY leaked into subprocess env")
	}
	if values["HOME"] != "/Users/test" {
		t.Fatalf("HOME should pass through, got %q", values["HOME"])
	}
	if values["PATH"] != "/usr/bin:/bin" {
		t.Fatalf("PATH should pass through, got %q", values["PATH"])
	}
	if values["GRAYCODE_SESSION_ID"] != "abc123" {
		t.Fatalf("non-credential vars should pass through, got %q", values["GRAYCODE_SESSION_ID"])
	}
}

func TestSubprocessEnv_NoEnv(t *testing.T) {
	os.Clearenv()
	got := SubprocessEnv()
	if len(got) != 0 {
		t.Fatalf("expected empty env, got %v", got)
	}
}
