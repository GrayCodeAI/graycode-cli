package testaudit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArchitectureDocsDoNotContainStaleContractsLanguage(t *testing.T) {
	root := repoRoot(t)

	files := []string{
		"README.md",
		"AGENTS.md",
		"docs/architecture/README.md",
		"docs/architecture/hawk-product-architecture.md",
		"docs/architecture/hawk-core-contracts-spec.md",
		"docs/architecture/hawk-contract-migration-inventory.md",
		"docs/plans/hawk-contracts-migration-backlog.md",
	}

	forbiddenPhrases := []string{
		"hawk-core-contracts` (to add)",
		"planned shared contracts layer",
		"runtime still uses `eyrie/client` provider interfaces and config types",
	}

	for _, rel := range files {
		path := filepath.Join(root, rel)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		content := string(data)
		for _, phrase := range forbiddenPhrases {
			if strings.Contains(content, phrase) {
				t.Fatalf("stale architecture phrase %q found in %s", phrase, rel)
			}
		}
	}
}

func TestArchitectureDocsMentionCurrentReviewVerifyContracts(t *testing.T) {
	root := repoRoot(t)

	checks := map[string][]string{
		"README.md": {
			"hawk-core-contracts/review",
			"hawk-core-contracts/verify",
		},
		"docs/architecture/hawk-product-architecture.md": {
			"hawk-core-contracts/review",
			"hawk-core-contracts/verify",
		},
		"docs/architecture/hawk-core-contracts-spec.md": {
			"hawk-core-contracts/review",
			"hawk-core-contracts/verify",
		},
	}

	for rel, required := range checks {
		path := filepath.Join(root, rel)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		content := string(data)
		for _, phrase := range required {
			if !strings.Contains(content, phrase) {
				t.Fatalf("required phrase %q missing in %s", phrase, rel)
			}
		}
	}
}

func TestArchitectureDocsDescribeSharedTypesAsRemoved(t *testing.T) {
	root := repoRoot(t)

	files := []string{
		"README.md",
		"AGENTS.md",
		"docs/architecture.md",
	}

	for _, rel := range files {
		path := filepath.Join(root, rel)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		content := strings.ToLower(string(data))
		if !strings.Contains(content, "shared/types") {
			t.Fatalf("expected %s to mention shared/types", rel)
		}
		if !strings.Contains(content, "removed") {
			t.Fatalf("expected %s to describe shared/types as removed", rel)
		}
	}
}
