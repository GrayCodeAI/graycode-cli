package engine

import (
	"strings"
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/plugin"
	"github.com/GrayCodeAI/graycode-cli/internal/tool"
)

func TestSession_EnsureSkillCatalogStatement(t *testing.T) {
	runtime := plugin.NewRuntimeSkillProvider("engine-test-provider")
	disposer := plugin.DefaultRegistry.Register(runtime)
	defer disposer()

	registry := tool.NewRegistry(tool.SkillTool{})
	sess := NewSession("", "", "system prompt", registry)

	// Initially no skills -> returns empty string (no previous catalog was published)
	stmt0 := sess.EnsureSkillCatalogStatement()
	if stmt0 != "" {
		t.Fatalf("expected empty statement when no skills and no prior catalog, got %q", stmt0)
	}

	// Add skill A
	runtime.AddSkill(plugin.SkillEntry{
		Name:        "lint-rule",
		Description: "Check lint rules",
		Invocation:  plugin.NewInvocationPolicy(true, true),
	})

	// Turn 1: Catalog should be published
	stmt1 := sess.EnsureSkillCatalogStatement()
	if stmt1 == "" {
		t.Fatal("expected catalog statement on first skill publication")
	}
	if !strings.Contains(stmt1, "Available skills (digest: ") {
		t.Fatalf("stmt1 %q missing digest header", stmt1)
	}
	if !strings.Contains(stmt1, "lint-rule: Check lint rules") {
		t.Fatalf("stmt1 %q missing lint-rule", stmt1)
	}

	// Turn 2: Unchanged catalog -> must return empty string (zero token waste!)
	stmt2 := sess.EnsureSkillCatalogStatement()
	if stmt2 != "" {
		t.Fatalf("expected empty statement when catalog unchanged, got %q", stmt2)
	}

	// Turn 3: Add skill B -> digest changes -> emits updated catalog
	runtime.AddSkill(plugin.SkillEntry{
		Name:        "format-code",
		Description: "Format source code",
		Invocation:  plugin.NewInvocationPolicy(true, true),
	})
	stmt3 := sess.EnsureSkillCatalogStatement()
	if stmt3 == "" {
		t.Fatal("expected updated catalog statement when skills change")
	}
	if !strings.Contains(stmt3, "format-code: Format source code") {
		t.Fatalf("stmt3 %q missing format-code", stmt3)
	}

	// Turn 4: Remove all skills -> emits tombstone retiring older names
	runtime.RemoveSkill("lint-rule")
	runtime.RemoveSkill("format-code")
	stmt4 := sess.EnsureSkillCatalogStatement()
	if stmt4 == "" {
		t.Fatal("expected tombstone statement when all skills removed")
	}
	if !strings.Contains(stmt4, "retired") {
		t.Fatalf("stmt4 %q missing retirement notice", stmt4)
	}

	// Turn 5: Still empty -> unchanged -> returns empty string
	stmt5 := sess.EnsureSkillCatalogStatement()
	if stmt5 != "" {
		t.Fatalf("expected empty statement after tombstone already emitted, got %q", stmt5)
	}
}
