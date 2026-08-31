package main

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/mod/modfile"
)

// trackedPins are the shared leaf dependencies most likely to drift silently:
// a consumer (merlin, kestrel, ...) can pin an older version than what hawk's
// own go.mod requires, and Go's minimal version selection will silently pull
// in hawk's newer version at build time without the consumer's own CI ever
// having tested it. See docs/compatibility.md.
var trackedPins = []string{
	"github.com/GrayCodeAI/eagle",
	"github.com/GrayCodeAI/falcon",
}

// checkDrift compares hawk's own go.mod requirements for trackedPins against
// what each sibling repo (a peer checkout of a hawk dependency in the shared
// workspace) declares for the same modules in its own go.mod. It never fails —
// this is advisory, printed for humans/CI logs to notice, not a build gate.
func checkDrift(repoRoot string) error {
	hawkRequires, err := readRequires(filepath.Join(repoRoot, "go.mod"))
	if err != nil {
		return fmt.Errorf("read hawk go.mod: %w", err)
	}

	workspaceDir := filepath.Join(repoRoot, "..")
	entries, err := os.ReadDir(workspaceDir)
	if err != nil {
		return fmt.Errorf("read workspace (%s): %w", workspaceDir, err)
	}

	fmt.Println("Pin freshness (advisory — see docs/compatibility.md):")
	drifted := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		modPath := filepath.Join(workspaceDir, e.Name(), "go.mod")
		consumerRequires, err := readRequires(modPath)
		if err != nil {
			continue // sibling not a Go module / no go.mod — skip silently
		}
		for _, pin := range trackedPins {
			hawkVer, hawkHas := hawkRequires[pin]
			consumerVer, consumerHas := consumerRequires[pin]
			if !hawkHas || !consumerHas || hawkVer == consumerVer {
				continue
			}
			drifted++
			fmt.Printf("  %-22s requires %s@%s, hawk requires %s@%s\n",
				e.Name(), pin, consumerVer, pin, hawkVer)
		}
	}
	if drifted == 0 {
		fmt.Println("  OK — no drift between hawk's pins and sibling consumers")
	}
	return nil
}

func readRequires(path string) (map[string]string, error) {
	raw, err := os.ReadFile(path) // #nosec G304 -- path is constructed by caller from known filesystem entries
	if err != nil {
		return nil, err
	}
	mf, err := modfile.Parse(path, raw, nil)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(mf.Require))
	for _, r := range mf.Require {
		out[r.Mod.Path] = r.Mod.Version
	}
	return out, nil
}
