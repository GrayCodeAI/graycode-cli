package remote

import (
	"testing"
	"time"
)

func TestDefaultServerConfig(t *testing.T) {
	c := DefaultServerConfig()
	if c.Port != 8080 {
		t.Fatalf("expected port 8080, got %d", c.Port)
	}
	if c.MaxSessions != 100 {
		t.Fatalf("expected max sessions 100, got %d", c.MaxSessions)
	}
}

func TestValidate(t *testing.T) {
	c := DefaultServerConfig()
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}

	c.Port = 0
	if err := c.Validate(); err == nil {
		t.Fatal("expected error for invalid port")
	}

	c.Port = 8080
	c.MaxSessions = 0
	if err := c.Validate(); err == nil {
		t.Fatal("expected error for invalid max sessions")
	}
}

func TestManager(t *testing.T) {
	m := NewManager(DefaultServerConfig())

	if len(m.List()) != 0 {
		t.Fatal("expected empty list")
	}

	s, err := m.Create("localhost", 22, "user", Auth{Type: "key"})
	if err != nil {
		t.Fatal(err)
	}
	if s.ID == "" {
		t.Fatal("expected session ID")
	}

	if len(m.List()) != 1 {
		t.Fatal("expected 1 session")
	}

	found, ok := m.Get(s.ID)
	if !ok {
		t.Fatal("expected to find session")
	}
	if found.Host != "localhost" {
		t.Fatalf("expected host localhost, got %q", found.Host)
	}

	m.Ping(s.ID)
	if found.LastPing.IsZero() {
		t.Fatal("expected ping to update")
	}

	m.Remove(s.ID)
	if len(m.List()) != 0 {
		t.Fatal("expected empty list after remove")
	}
}

func TestCleanup(t *testing.T) {
	m := NewManager(DefaultServerConfig())
	s, _ := m.Create("localhost", 22, "user", Auth{Type: "key"})

	// Don't ping, session should be stale
	removed := m.Cleanup(1 * time.Nanosecond)
	if removed != 1 {
		t.Fatalf("expected 1 removed, got %d", removed)
	}

	_, ok := m.Get(s.ID)
	if ok {
		t.Fatal("expected session to be removed")
	}
}
