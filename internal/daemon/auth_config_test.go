package daemon

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestIsLoopbackHost(t *testing.T) {
	cases := []struct {
		host string
		want bool
	}{
		// Loopback (allowed)
		{"127.0.0.1", true},
		{"127.0.0.53", true}, // systemd-resolved-style; still in 127.0.0.0/8
		{"::1", true},
		{"localhost", true},
		// Non-loopback (must require apiKey)
		{"", false}, // empty host is treated as unsafe (refuse to start)
		{"0.0.0.0", false},
		{"::", false},
		{"192.168.1.1", false},
		{"10.0.0.1", false},
		{"8.8.8.8", false},
		{"example.com", false},
		{"localhost.evil.example", false}, // suffix-collision guard
	}
	for _, tc := range cases {
		t.Run(tc.host, func(t *testing.T) {
			if got := isLoopbackHost(tc.host); got != tc.want {
				t.Errorf("isLoopbackHost(%q) = %v, want %v", tc.host, got, tc.want)
			}
		})
	}
}

func TestValidateAuthConfig(t *testing.T) {
	cases := []struct {
		name    string
		apiKey  string
		addr    string
		tls     bool
		wantErr bool
	}{
		// With API key: loopback binds are allowed (plaintext is fine
		// on loopback — the key never leaves the host).
		{"apiKey set, loopback", "secret", "127.0.0.1:4590", false, false},
		{"apiKey set, IPv6 loopback", "secret", "[::1]:4590", false, false},
		// Non-loopback binds require TLS even with an API key: the key
		// and full conversation history would otherwise travel in plaintext.
		{"apiKey set, wildcard no TLS refused", "secret", "0.0.0.0:4590", false, true},
		{"apiKey set, public IP no TLS refused", "secret", "192.168.1.1:4590", false, true},
		{"apiKey set, wildcard with TLS", "secret", "0.0.0.0:4590", true, false},
		{"apiKey set, public IP with TLS", "secret", "192.168.1.1:4590", true, false},
		// Without API key: loopback only.
		{"no key, IPv4 loopback", "", "127.0.0.1:4590", false, false},
		{"no key, IPv6 loopback", "", "[::1]:4590", false, false},
		{"no key, localhost name", "", "localhost:4590", false, false},
		{"no key, wildcard refused", "", "0.0.0.0:4590", false, true},
		{"no key, IPv6 wildcard refused", "", "[::]:4590", false, true},
		{"no key, private IP refused", "", "192.168.1.1:4590", false, true},
		{"no key, public IP refused", "", "8.8.8.8:4590", false, true},
		{"no key, hostname refused", "", "example.com:4590", false, true},
		{"no key, no host part refused", "", ":4590", false, true},
		{"no key, invalid addr refused", "", "not-a-valid-address", false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &Server{apiKey: tc.apiKey, addr: tc.addr}
			if tc.tls {
				s.tlsCertFile = "cert.pem"
				s.tlsKeyFile = "key.pem"
			}
			err := s.validateAuthConfig()
			if tc.wantErr && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateAuthConfig_ErrorMentionsRemediation(t *testing.T) {
	s := &Server{apiKey: "", addr: "0.0.0.0:4590"}
	err := s.validateAuthConfig()
	if err == nil {
		t.Fatal("expected error for no key + wildcard bind")
	}
	msg := err.Error()
	if !strings.Contains(msg, "apiKey") {
		t.Errorf("error should mention apiKey, got: %q", msg)
	}
	if !strings.Contains(msg, "127.0.0.1") {
		t.Errorf("error should mention loopback bind as remediation, got: %q", msg)
	}
}

func TestWarnInsecureAuthConfig_NoKey_LogsWarn(t *testing.T) {
	var buf bytes.Buffer
	orig := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(orig) })

	s := &Server{apiKey: "", addr: "127.0.0.1:4590"}
	s.warnInsecureAuthConfig()

	out := buf.String()
	if !strings.Contains(out, "WARN") {
		t.Errorf("expected WARN level, got: %q", out)
	}
	if !strings.Contains(out, "API key") {
		t.Errorf("expected message to mention API key, got: %q", out)
	}
	if !strings.Contains(out, "loopback") {
		t.Errorf("expected message to mention loopback, got: %q", out)
	}
	if !strings.Contains(out, "127.0.0.1:4590") {
		t.Errorf("expected log to include addr, got: %q", out)
	}
}

func TestWarnInsecureAuthConfig_WithKey_NoLog(t *testing.T) {
	var buf bytes.Buffer
	orig := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(orig) })

	s := &Server{apiKey: "secret", addr: "0.0.0.0:4590"}
	s.warnInsecureAuthConfig()

	if buf.Len() != 0 {
		t.Errorf("expected no log when apiKey is set, got: %q", buf.String())
	}
}
