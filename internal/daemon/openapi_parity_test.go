package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestOpenAPISpecMatchesRegisteredRoutes fails when api/openapi.yaml and the
// routes registered on the daemon mux drift apart in either direction. Every
// operation in the spec must have a registered handler, and every registered
// handler must be documented in the spec.
func TestOpenAPISpecMatchesRegisteredRoutes(t *testing.T) {
	specPath := filepath.Join("..", "..", "api", "openapi.yaml")
	raw, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("read spec: %v", err)
	}

	var spec struct {
		Paths map[string]map[string]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(raw, &spec); err != nil {
		t.Fatalf("parse spec: %v", err)
	}
	if len(spec.Paths) == 0 {
		t.Fatal("spec has no paths — parse failure or empty spec")
	}

	httpMethods := map[string]bool{
		"get": true, "post": true, "put": true, "patch": true,
		"delete": true, "head": true, "options": true,
	}
	specOps := map[string]bool{}
	for path, ops := range spec.Paths {
		for method := range ops {
			if httpMethods[strings.ToLower(method)] {
				specOps[strings.ToUpper(method)+" "+path] = true
			}
		}
	}

	srv := New(DefaultConfig(), nil)
	registered := map[string]bool{}
	for _, pattern := range srv.RoutePatterns() {
		registered[pattern] = true
	}

	var problems []string
	for op := range specOps {
		if !registered[op] {
			problems = append(problems, fmt.Sprintf("documented in openapi.yaml but not registered on the daemon: %s", op))
		}
	}
	for pattern := range registered {
		if !specOps[pattern] {
			problems = append(problems, fmt.Sprintf("registered on the daemon but missing from openapi.yaml: %s", pattern))
		}
	}
	sort.Strings(problems)
	for _, p := range problems {
		t.Error(p)
	}
}
