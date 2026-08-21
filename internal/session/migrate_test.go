package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func freshSessionsDir(t *testing.T) string {
	t.Helper()
	sdir := setTestSessionsDir(t, t.TempDir())
	if err := os.MkdirAll(sdir, 0o700); err != nil {
		t.Fatal(err)
	}
	return sdir
}

func writeLegacySessionJSON(t *testing.T, sdir, id, extra string) {
	t.Helper()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	body := `{"id":"` + id + `","messages":[{"role":"user","content":"hello"}],"created_at":"` +
		now.Format(time.RFC3339) + `","updated_at":"` + now.Format(time.RFC3339) + `"` + extra + `}`
	if err := os.WriteFile(filepath.Join(sdir, id+".json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestMigrateSessionLegacyJSONToJSONL(t *testing.T) {
	sdir := freshSessionsDir(t)
	id := "migrate-legacy"
	writeLegacySessionJSON(t, sdir, id, "")

	res, err := MigrateSession(id, false)
	if err != nil {
		t.Fatalf("MigrateSession: %v", err)
	}
	if res.FromVersion != 0 || res.ToVersion != SessionFormatVersion {
		t.Fatalf("versions = %d->%d, want 0->%d", res.FromVersion, res.ToVersion, SessionFormatVersion)
	}
	if _, err := os.Stat(filepath.Join(sdir, id+".json")); !os.IsNotExist(err) {
		t.Errorf("legacy .json still present, want removed")
	}
	jsonl, err := os.ReadFile(filepath.Join(sdir, id+".jsonl"))
	if err != nil {
		t.Fatalf("read .jsonl: %v", err)
	}
	if !strings.Contains(string(jsonl), "hello") {
		t.Errorf("migrated .jsonl lost message content:\n%s", jsonl)
	}
}

func TestMigrateSessionJSONLAlreadyCurrent(t *testing.T) {
	sdir := freshSessionsDir(t)
	id := "migrate-current"
	body := `{"type":"session_meta","id":"` + id + `","format_version":1}` + "\n" +
		`{"type":"message.user","seq":1}` + "\n"
	if err := os.WriteFile(filepath.Join(sdir, id+".jsonl"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	res, err := MigrateSession(id, false)
	if err != nil {
		t.Fatalf("MigrateSession: %v", err)
	}
	if res.FromVersion < SessionFormatVersion {
		t.Fatalf("from = %d, want >= current %d", res.FromVersion, SessionFormatVersion)
	}
}

func TestMigrateSessionJSONLVersionBump(t *testing.T) {
	sdir := freshSessionsDir(t)
	id := "migrate-bump"
	body := `{"type":"session_meta","id":"` + id + `"}` + "\n" +
		`{"type":"message.user","seq":1}` + "\n"
	if err := os.WriteFile(filepath.Join(sdir, id+".jsonl"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	res, err := MigrateSession(id, false)
	if err != nil {
		t.Fatalf("MigrateSession: %v", err)
	}
	if res.FromVersion != 0 || res.ToVersion != SessionFormatVersion {
		t.Fatalf("versions = %d->%d, want 0->%d", res.FromVersion, res.ToVersion, SessionFormatVersion)
	}
	data, _ := os.ReadFile(filepath.Join(sdir, id+".jsonl"))
	if !strings.Contains(string(data), `"format_version":1`) {
		t.Errorf("meta line not bumped to current:\n%s", data)
	}
}

func TestMigrateSessionOversizedRefused(t *testing.T) {
	sdir := freshSessionsDir(t)
	id := "migrate-big"
	pad := strings.Repeat("x", 40<<20) // 40 MiB, over the 32 MiB threshold
	writeLegacySessionJSON(t, sdir, id, `,"pad":"`+pad+`"`)

	if _, err := MigrateSession(id, false); err == nil {
		t.Fatal("expected oversized error without --allow-large")
	}
	if _, err := MigrateSession(id, true); err != nil {
		t.Fatalf("MigrateSession with allowLarge: %v", err)
	}
}

func TestMigrateSessionNotFound(t *testing.T) {
	setTestSessionsDir(t, t.TempDir())
	if _, err := MigrateSession("migrate-none", false); err == nil {
		t.Fatal("expected not-found error")
	}
}
