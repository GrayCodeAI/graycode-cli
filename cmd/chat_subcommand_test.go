//nolint:errcheck
package cmd

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// mockSubcommand is a test fixture for ChatSubcommand. It records
// the args and text it was called with so tests can assert on the
// dispatch path. The Handle method returns a nil tea.Cmd (the
// common case — most subcommands don't enqueue side effects).
type mockSubcommand struct {
	name        string
	aliases     []string
	description string
	usage       string

	// Recorded on each Handle call.
	calls       int
	lastArgs    []string
	lastText    string
	lastModel   *chatModel
	customCmd   tea.Cmd // optional; if non-nil, returned by Handle
	customModel tea.Model
}

func (m *mockSubcommand) Name() string             { return m.name }
func (m *mockSubcommand) Aliases() []string        { return m.aliases }
func (m *mockSubcommand) Description() string      { return m.description }
func (m *mockSubcommand) Usage() string            { return m.usage }
func (m *mockSubcommand) Handle(ml *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	m.calls++
	m.lastArgs = args
	m.lastText = text
	m.lastModel = ml
	return m.customModel, m.customCmd
}

// --- registry basics ---

func TestNewSubcommandRegistry_Empty(t *testing.T) {
	r := NewSubcommandRegistry()
	if r == nil {
		t.Fatal("NewSubcommandRegistry returned nil")
	}
	if r.Size() != 0 {
		t.Errorf("new registry Size = %d, want 0", r.Size())
	}
	if names := r.Names(); len(names) != 0 {
		t.Errorf("new registry Names = %v, want empty", names)
	}
}

func TestSubcommandRegistry_RegisterAndLookup(t *testing.T) {
	r := NewSubcommandRegistry()
	r.Register(&mockSubcommand{name: "help", description: "show help"})

	got, ok := r.Lookup("help")
	if !ok {
		t.Fatal("Lookup(help) = false, want true")
	}
	if got.Name() != "help" {
		t.Errorf("Lookup(help).Name = %q, want help", got.Name())
	}
}

func TestSubcommandRegistry_LookupMissing(t *testing.T) {
	r := NewSubcommandRegistry()
	r.Register(&mockSubcommand{name: "help"})

	if _, ok := r.Lookup("nonexistent"); ok {
		t.Error("Lookup(nonexistent) = true, want false")
	}
}

func TestSubcommandRegistry_Aliases(t *testing.T) {
	r := NewSubcommandRegistry()
	r.Register(&mockSubcommand{
		name:    "exit",
		aliases: []string{"quit", "bye"},
	})

	// Primary name resolves.
	if got, ok := r.Lookup("exit"); !ok || got.Name() != "exit" {
		t.Errorf("Lookup(exit) = (%v, %v), want (exit, true)", got, ok)
	}

	// Aliases resolve to the same primary.
	for _, alias := range []string{"quit", "bye"} {
		got, ok := r.Lookup(alias)
		if !ok {
			t.Errorf("Lookup(%q) = false, want true", alias)
			continue
		}
		if got.Name() != "exit" {
			t.Errorf("Lookup(%q).Name = %q, want exit", alias, got.Name())
		}
	}
}

func TestSubcommandRegistry_All(t *testing.T) {
	r := NewSubcommandRegistry()
	r.Register(&mockSubcommand{name: "help"})
	r.Register(&mockSubcommand{name: "model"})
	r.Register(&mockSubcommand{name: "memory"})

	all := r.All()
	if len(all) != 3 {
		t.Fatalf("All() = %d subcommands, want 3", len(all))
	}
	// All() is sorted by name.
	want := []string{"help", "memory", "model"}
	for i, c := range all {
		if c.Name() != want[i] {
			t.Errorf("All()[%d].Name = %q, want %q", i, c.Name(), want[i])
		}
	}
}

func TestSubcommandRegistry_DuplicateRegistrationIsNoOp(t *testing.T) {
	r := NewSubcommandRegistry()
	first := &mockSubcommand{name: "help", description: "first"}
	second := &mockSubcommand{name: "help", description: "second"}
	r.Register(first)
	r.Register(second) // should be a no-op

	got, ok := r.Lookup("help")
	if !ok {
		t.Fatal("Lookup(help) = false, want true")
	}
	if got.Description() != "first" {
		t.Errorf("got Description = %q, want 'first' (first registration wins)", got.Description())
	}
}

func TestSubcommandRegistry_NilIsSafe(t *testing.T) {
	r := NewSubcommandRegistry()
	r.Register(nil) // should not panic
	if r.Size() != 0 {
		t.Errorf("Size = %d after Register(nil), want 0", r.Size())
	}
}

func TestSubcommandRegistry_NamesIsSorted(t *testing.T) {
	r := NewSubcommandRegistry()
	r.Register(&mockSubcommand{name: "zebra"})
	r.Register(&mockSubcommand{name: "alpha"})
	r.Register(&mockSubcommand{name: "mango"})

	want := []string{"alpha", "mango", "zebra"}
	got := r.Names()
	if len(got) != len(want) {
		t.Fatalf("Names = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Names[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// --- subcommand-interface contract ---

func TestSubcommandInterface_Accessors(t *testing.T) {
	m := &mockSubcommand{
		name:        "save",
		aliases:     []string{"s", "w"},
		description: "save the world",
		usage:       "save [name]",
	}
	if m.Name() != "save" {
		t.Errorf("Name = %q", m.Name())
	}
	if got := m.Aliases(); len(got) != 2 || got[0] != "s" || got[1] != "w" {
		t.Errorf("Aliases = %v", got)
	}
	if m.Description() != "save the world" {
		t.Errorf("Description = %q", m.Description())
	}
	if m.Usage() != "save [name]" {
		t.Errorf("Usage = %q", m.Usage())
	}
}

func TestSubcommandInterface_DefaultImplementationsAreEmpty(t *testing.T) {
	// An implementation that returns zero values for everything
	// should compile and be safe to register. The empty Aliases
	// / Description / Usage are valid defaults.
	m := &zeroSubcommand{name: "noop"}
	r := NewSubcommandRegistry()
	r.Register(m) // should not panic

	if got, ok := r.Lookup("noop"); !ok || got.Name() != "noop" {
		t.Errorf("Lookup(noop) = (%v, %v), want (noop, true)", got, ok)
	}
}

// zeroSubcommand is a minimal ChatSubcommand that returns zero
// values for everything except Name(). It tests the "implement
// only Name" path.
type zeroSubcommand struct{ name string }

func (z *zeroSubcommand) Name() string                                          { return z.name }
func (z *zeroSubcommand) Aliases() []string                                     { return nil }
func (z *zeroSubcommand) Description() string                                   { return "" }
func (z *zeroSubcommand) Usage() string                                         { return "" }
func (z *zeroSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	return m, nil
}

// --- migration scaffolding ---

// TestMigrationExample_HelpSubcommand demonstrates the canonical
// pattern for migrating a handler from chat_commands.go's switch
// statement into a registered ChatSubcommand. New subcommands
// should follow this template.
func TestMigrationExample_HelpSubcommand(t *testing.T) {
	r := NewSubcommandRegistry()
	r.Register(&helpSubcommand{})

	cmd, ok := r.Lookup("help")
	if !ok {
		t.Fatal("Lookup(help) = false, want true")
	}
	if cmd.Name() != "help" {
		t.Errorf("Name = %q, want help", cmd.Name())
	}
	if cmd.Description() != "show this help" {
		t.Errorf("Description = %q", cmd.Description())
	}
	if cmd.Usage() != "" {
		t.Errorf("Usage = %q, want empty (no args)", cmd.Usage())
	}
}

// --- concurrency ---

func TestSubcommandRegistry_ConcurrentRegisterAndLookup(t *testing.T) {
	r := NewSubcommandRegistry()
	const goroutines = 20
	done := make(chan struct{})

	// Writers
	for i := 0; i < goroutines; i++ {
		go func(i int) {
			for j := 0; j < 50; j++ {
				r.Register(&mockSubcommand{
					name:        cmdName(i, j),
					description: "concurrent test",
				})
			}
			done <- struct{}{}
		}(i)
	}

	// Readers
	for i := 0; i < goroutines; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_, _ = r.Lookup("nonexistent")
				_ = r.Names()
				_ = r.Size()
			}
			done <- struct{}{}
		}()
	}

	// Wait for all goroutines.
	for i := 0; i < goroutines*2; i++ {
		<-done
	}
	// Just verifying no race / panic; the final Size is
	// unpredictable because of duplicate-registration dedup.
}

// cmdName is a tiny helper that returns a deterministic name for
// the i-th writer's j-th registration. Used to generate
// non-colliding names in the concurrent test.
func cmdName(i, j int) string {
	var sb strings.Builder
	sb.WriteByte('c')
	sb.WriteByte('m')
	sb.WriteByte('d')
	for _, c := range []byte{byte('a' + (i % 26)), byte('a' + (j % 26))} {
		sb.WriteByte(c)
	}
	return sb.String()
}

// --- migrated command: /branch ---
//
// These tests verify the first command migrated from
// chat_commands.go into the new SubcommandRegistry pattern. They
// run as a sanity check that the init() registration works and
// the subcommand's contract is honored.

func TestBranchSubcommand_Registered(t *testing.T) {
	cmd, ok := subcommandRegistry.Lookup("branch")
	if !ok {
		t.Fatal("/branch not registered in subcommandRegistry")
	}
	if cmd.Name() != "branch" {
		t.Errorf("Name = %q, want branch", cmd.Name())
	}
	if cmd.Description() == "" {
		t.Error("Description is empty; should describe the command for /help")
	}
	if cmd.Usage() != "" {
		t.Errorf("Usage = %q, want empty (no args)", cmd.Usage())
	}
	if len(cmd.Aliases()) != 0 {
		t.Errorf("Aliases = %v, want empty", cmd.Aliases())
	}
}

func TestBranchSubcommand_NotInChatCommands(t *testing.T) {
	// Regression guard: the migrated /branch case in chat_commands.go
	// should be removed so the dispatcher doesn't double-fire.
	// (This is a TODO for the next sub-PR; the case is still
	// present in chat_commands.go today.)
	t.Skip("TODO: remove /branch case from chat_commands.go when handleCommand migrates to the registry")
}

func TestBranchSubcommand_SizeIncreasesAfterRegistration(t *testing.T) {
	// The init() in chat_subcommand_branch.go registers one command.
	// Verify the package-level registry is non-empty (i.e. the
	// init() function ran when the package was loaded).
	if subcommandRegistry.Size() < 1 {
		t.Errorf("subcommandRegistry.Size = %d, want >= 1 (init() should have registered /branch)",
			subcommandRegistry.Size())
	}
}

func TestSubcommandRegistry_AllContainsBranch(t *testing.T) {
	// All() should include /branch (along with any other
	// subcommands added by future init() functions).
	all := subcommandRegistry.All()
	found := false
	for _, c := range all {
		if c.Name() == "branch" {
			found = true
			break
		}
	}
	if !found {
		t.Error("/branch not in subcommandRegistry.All()")
	}
}

// --- migrated commands: /version, /env, /doctor, /init, /focus,
//     /pin, /files, /commit, /session ---
//
// These tests verify the second batch of commands migrated from
// chat_commands.go into the new SubcommandRegistry pattern.

func TestVersionSubcommand_Registered(t *testing.T) {
	cmd, ok := subcommandRegistry.Lookup("version")
	if !ok {
		t.Fatal("/version not registered in subcommandRegistry")
	}
	if cmd.Name() != "version" {
		t.Errorf("Name = %q, want version", cmd.Name())
	}
	if cmd.Description() == "" {
		t.Error("Description is empty; should describe the command for /help")
	}
}

func TestEnvSubcommand_Registered(t *testing.T) {
	cmd, ok := subcommandRegistry.Lookup("env")
	if !ok {
		t.Fatal("/env not registered in subcommandRegistry")
	}
	if cmd.Name() != "env" {
		t.Errorf("Name = %q, want env", cmd.Name())
	}
}

func TestDoctorSubcommand_Registered(t *testing.T) {
	cmd, ok := subcommandRegistry.Lookup("doctor")
	if !ok {
		t.Fatal("/doctor not registered in subcommandRegistry")
	}
	if cmd.Name() != "doctor" {
		t.Errorf("Name = %q, want doctor", cmd.Name())
	}
}

func TestInitSubcommand_Registered(t *testing.T) {
	cmd, ok := subcommandRegistry.Lookup("init")
	if !ok {
		t.Fatal("/init not registered in subcommandRegistry")
	}
	if cmd.Name() != "init" {
		t.Errorf("Name = %q, want init", cmd.Name())
	}
}

func TestFocusSubcommand_Registered(t *testing.T) {
	cmd, ok := subcommandRegistry.Lookup("focus")
	if !ok {
		t.Fatal("/focus not registered in subcommandRegistry")
	}
	if cmd.Name() != "focus" {
		t.Errorf("Name = %q, want focus", cmd.Name())
	}
	if cmd.Usage() == "" {
		t.Error("Usage is empty; /focus requires path args")
	}
}

func TestPinSubcommand_Registered(t *testing.T) {
	cmd, ok := subcommandRegistry.Lookup("pin")
	if !ok {
		t.Fatal("/pin not registered in subcommandRegistry")
	}
	if cmd.Name() != "pin" {
		t.Errorf("Name = %q, want pin", cmd.Name())
	}
}

func TestFilesSubcommand_Registered(t *testing.T) {
	cmd, ok := subcommandRegistry.Lookup("files")
	if !ok {
		t.Fatal("/files not registered in subcommandRegistry")
	}
	if cmd.Name() != "files" {
		t.Errorf("Name = %q, want files", cmd.Name())
	}
}

func TestCommitSubcommand_Registered(t *testing.T) {
	cmd, ok := subcommandRegistry.Lookup("commit")
	if !ok {
		t.Fatal("/commit not registered in subcommandRegistry")
	}
	if cmd.Name() != "commit" {
		t.Errorf("Name = %q, want commit", cmd.Name())
	}
}

func TestSessionSubcommand_AliasesRegistered(t *testing.T) {
	// sessionSubcommand has 8 names: /clear (primary), /compact,
	// /diff, /recover, /resume, /history, /quit, /exit.
	for _, name := range []string{"clear", "compact", "diff", "recover", "resume", "history", "quit", "exit"} {
		if _, ok := subcommandRegistry.Lookup(name); !ok {
			t.Errorf("/%s not registered (session subcommand should cover all)", name)
		}
	}
}

func TestSubcommandRegistry_MigratedCount(t *testing.T) {
	// After the H5 batch-2 migration, the registry should have
	// at least: branch, version, env, doctor, init, focus, pin,
	// files, commit, session (10 total).
	if got := subcommandRegistry.Size(); got < 10 {
		t.Errorf("subcommandRegistry.Size = %d, want >= 10 (after H5 batch-2)", got)
	}
}
