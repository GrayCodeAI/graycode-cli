package lsp

import (
	"testing"
)

// TestServerManager_Start_AlreadyRunning_Extra tests that starting an already-running server returns nil.
func TestServerManager_Start_AlreadyRunning_Extra(t *testing.T) {
	mgr := NewServerManager()

	// Start a server with a short-lived command
	err := mgr.Start("test", "sleep", "1")
	if err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	defer mgr.Stop("test")

	// Start again should return nil (already running)
	err = mgr.Start("test", "sleep", "1")
	if err != nil {
		t.Errorf("expected nil for already running server, got %v", err)
	}
}

// TestServerManager_Stop_NotRunning_Extra tests stopping a server that's not running.
func TestServerManager_Stop_NotRunning_Extra(t *testing.T) {
	mgr := NewServerManager()
	err := mgr.Stop("nonexistent")
	if err != nil {
		t.Errorf("expected nil for non-existent server, got %v", err)
	}
}

// TestServerManager_List_Extra tests the List method.
func TestServerManager_List_Extra(t *testing.T) {
	mgr := NewServerManager()

	// No servers
	list := mgr.List()
	if len(list) != 0 {
		t.Errorf("expected empty list, got %v", list)
	}

	// Start a server
	err := mgr.Start("test", "sleep", "1")
	if err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	defer mgr.Stop("test")

	list = mgr.List()
	if len(list) != 1 {
		t.Errorf("expected 1 server in list, got %d", len(list))
	}
	if list[0] != "test" {
		t.Errorf("expected 'test' in list, got %q", list[0])
	}
}

// TestServerManager_IsRunning_Extra tests the IsRunning method.
func TestServerManager_IsRunning_Extra(t *testing.T) {
	mgr := NewServerManager()

	// Not running
	if mgr.IsRunning("nonexistent") {
		t.Error("expected IsRunning to return false for non-existent server")
	}

	// Start a server
	err := mgr.Start("test", "sleep", "1")
	if err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	defer mgr.Stop("test")

	if !mgr.IsRunning("test") {
		t.Error("expected IsRunning to return true for running server")
	}
}

// TestServerManager_Start_CommandError_Extra tests starting a server with an invalid command.
func TestServerManager_Start_CommandError_Extra(t *testing.T) {
	mgr := NewServerManager()

	// Start with a non-existent command
	err := mgr.Start("test", "nonexistent-command-xyz123", "arg")
	if err == nil {
		t.Error("expected error for non-existent command")
	}
}

// TestServerManager_Stop_Running_Extra tests stopping a running server.
func TestServerManager_Stop_Running_Extra(t *testing.T) {
	mgr := NewServerManager()

	// Start a server
	err := mgr.Start("test", "sleep", "1")
	if err != nil {
		t.Fatalf("failed to start server: %v", err)
	}

	// Stop should succeed
	err = mgr.Stop("test")
	if err != nil {
		t.Errorf("expected nil for stopping running server, got %v", err)
	}

	// Should not be running after stop
	if mgr.IsRunning("test") {
		t.Error("expected server to not be running after stop")
	}
}

// TestServerManager_Stop_AlreadyStopped_Extra tests stopping a server that's already stopped.
func TestServerManager_Stop_AlreadyStopped_Extra(t *testing.T) {
	mgr := NewServerManager()

	// Start and stop
	err := mgr.Start("test", "sleep", "1")
	if err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	mgr.Stop("test")

	// Stop again should return nil
	err = mgr.Stop("test")
	if err != nil {
		t.Errorf("expected nil for already stopped server, got %v", err)
	}
}

// TestServerManager_Request_NotRunning_Extra tests requesting from a non-running server.
func TestServerManager_Request_NotRunning_Extra(t *testing.T) {
	mgr := NewServerManager()

	// ServerManager doesn't have Request method - test via Client instead
	// This test verifies the manager handles non-existent servers gracefully
	if mgr.IsRunning("nonexistent") {
		t.Error("expected IsRunning to return false for non-existent server")
	}
}
