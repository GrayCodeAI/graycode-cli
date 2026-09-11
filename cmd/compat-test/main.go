// compat-test reads compatibility-matrix.json and runs basic compatibility
// checks across the listed components.
//
// Today this is intentionally minimal — it validates the matrix file
// structurally and reports the resolved versions for a chosen matrix entry
// so humans / CI can sanity-check what they're about to release together.
//
// Future passes can extend it to actually clone each component at the
// pinned version and run integration tests; the wire format here is the
// same one the compatibility-test workflow consumes, so additions here
// flow straight into CI.
//
// Usage:
//
//	go run ./cmd/compat-test                 # validate, dump 'next' matrix
//	go run ./cmd/compat-test -matrix=stable  # dump a specific matrix
//	go run ./cmd/compat-test -matrix=stable -strict
//	                                         # exit non-zero if any
//	                                         # component lacks a version
//	go run ./cmd/compat-test -check-external # advisory: compare hawk's own
//	                                         # go.mod pins for shared leaf
//	                                         # deps against what each
//	                                         # sibling repo declares.
//	                                         # Always exits 0; see drift.go.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

type matrixFile struct {
	Description  string              `json:"description"`
	Version      string              `json:"version"`
	Updated      string              `json:"updated"`
	Components   []string            `json:"components"`
	Dependencies map[string][]string `json:"dependencies"`
	Matrices     []matrix            `json:"matrices"`
}

type matrix struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Components  map[string]string `json:"components"`
}

func main() {
	matrixName := flag.String("matrix", "next", "matrix entry to inspect (next, stable, ...)")
	strict := flag.Bool("strict", false, "exit non-zero if any component lacks a pinned version")
	path := flag.String("file", findMatrixFile(), "path to compatibility-matrix.json")
	checkExternal := flag.Bool("check-external", false, "advisory: report pin drift against sibling repos and exit (see drift.go)")
	flag.Parse()

	if *path == "" {
		die("compatibility-matrix.json not found in current dir or repo root")
	}

	if *checkExternal {
		repoRoot := filepath.Dir(filepath.Dir(*path)) // testdata/compatibility-matrix.json -> repo root
		if err := checkDrift(repoRoot); err != nil {
			// Advisory tool: report the problem but still exit 0.
			fmt.Fprintf(os.Stderr, "compat-test: check-external: %v\n", err)
		}
		return
	}

	raw, err := os.ReadFile(*path)
	if err != nil {
		die("read %s: %v", *path, err)
	}

	var mf matrixFile
	if err := json.Unmarshal(raw, &mf); err != nil {
		die("parse %s: %v", *path, err)
	}

	if err := validate(mf); err != nil {
		die("matrix invalid: %v", err)
	}

	target, ok := findMatrix(mf.Matrices, *matrixName)
	if !ok {
		die("matrix %q not found. available: %s", *matrixName, listMatrixNames(mf.Matrices))
	}

	if err := report(mf, target, *strict); err != nil {
		die("%v", err)
	}
}

// validate enforces the same constraints the workflow validator does:
// every component listed in `components` must appear in every matrix entry,
// and every key in `dependencies` must be a known component.
func validate(mf matrixFile) error {
	known := make(map[string]bool, len(mf.Components))
	for _, c := range mf.Components {
		known[c] = true
	}

	for _, m := range mf.Matrices {
		var missing, extra []string
		for _, c := range mf.Components {
			if _, ok := m.Components[c]; !ok {
				missing = append(missing, c)
			}
		}
		for c := range m.Components {
			if !known[c] {
				extra = append(extra, c)
			}
		}
		if len(missing) > 0 || len(extra) > 0 {
			sort.Strings(missing)
			sort.Strings(extra)
			return fmt.Errorf("matrix %q: missing=%v extra=%v", m.Name, missing, extra)
		}
	}

	for k := range mf.Dependencies {
		if !known[k] {
			return fmt.Errorf("dependency declared for unknown component %q", k)
		}
	}
	return nil
}

func findMatrix(ms []matrix, name string) (matrix, bool) {
	for _, m := range ms {
		if m.Name == name {
			return m, true
		}
	}
	return matrix{}, false
}

func listMatrixNames(ms []matrix) string {
	names := make([]string, len(ms))
	for i, m := range ms {
		names[i] = m.Name
	}
	return fmt.Sprintf("%v", names)
}

func report(mf matrixFile, m matrix, strict bool) error {
	fmt.Printf("Matrix: %s\n", m.Name)
	if m.Description != "" {
		fmt.Printf("  %s\n", m.Description)
	}
	fmt.Println()

	keys := make([]string, 0, len(m.Components))
	for k := range m.Components {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	missing := 0
	for _, k := range keys {
		v := m.Components[k]
		if v == "" {
			missing++
			fmt.Printf("  %-20s (no version pinned)\n", k)
		} else {
			fmt.Printf("  %-20s %s\n", k, v)
		}
	}

	if strict && missing > 0 {
		return fmt.Errorf("strict mode: %d components lack a pinned version", missing)
	}
	return nil
}

// findMatrixFile locates the cross-repo compatibility matrix (testdata/compatibility-matrix.json).
// It must not pick hawk/platform-capabilities.json, which is a different document.
func findMatrixFile() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	candidates := []string{
		"testdata/compatibility-matrix.json",
		"hawk/testdata/compatibility-matrix.json",
	}
	for i := 0; i < 6; i++ {
		for _, rel := range candidates {
			p := filepath.Join(dir, rel)
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

func die(format string, args ...any) {
	fmt.Fprintln(os.Stderr, "compat-test: "+fmt.Sprintf(format, args...))
	os.Exit(1)
}
