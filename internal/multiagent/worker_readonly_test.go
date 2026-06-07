package mission

import "testing"

// readOnlyWorkerTools must never include a mutating tool. Bash is excluded
// because it can mutate the tree (rm, git, redirects) even though it is not
// nominally a "write" tool.
func TestReadOnlyWorkerToolsAreReadOnly(t *testing.T) {
	mutating := map[string]bool{
		"Write": true, "Edit": true, "Bash": true,
	}
	for _, tl := range readOnlyWorkerTools() {
		name := tl.Name()
		if mutating[name] {
			t.Errorf("read-only worker exposes mutating tool %q", name)
		}
	}
	if len(readOnlyWorkerTools()) == 0 {
		t.Fatal("read-only worker must expose at least Read/Grep/Glob/LS")
	}
}

// The implementation worker, by contrast, must retain write capability.
func TestBaseWorkerToolsCanMutate(t *testing.T) {
	var hasWrite bool
	for _, tl := range baseWorkerTools() {
		switch tl.Name() {
		case "Write", "Edit", "Bash":
			hasWrite = true
		}
	}
	if !hasWrite {
		t.Error("implementation worker should retain a mutating tool")
	}
}
