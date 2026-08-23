// Package toolset provides named, composable tool groups for scoping an
// agent's tool surface, adopting Hermes Agent's toolset system. A toolset is
// a named set of tools that can be composed from other toolsets; resolving a
// set expands its Requires transitively (cycle-safe). This lets an agent
// (or a /toolset command) switch between focused surfaces such as
// "research", "dev", or "full_stack" instead of always advertising every tool.
package toolset

import "sort"

// Toolset is a named group of tools, optionally composing other toolsets.
type Toolset struct {
	Name     string
	Tools    []string
	Requires []string // other toolset names to expand
}

// Registry holds the known toolsets and is the lookup source for resolution.
type Registry struct {
	sets map[string]Toolset
}

// NewRegistry builds a registry from the given toolsets, returning an error on
// duplicate names.
func NewRegistry(sets []Toolset) (*Registry, error) {
	r := &Registry{sets: map[string]Toolset{}}
	for _, s := range sets {
		if _, dup := r.sets[s.Name]; dup {
			return nil, errDup(s.Name)
		}
		r.sets[s.Name] = s
	}
	return r, nil
}

// Resolve expands a toolset name to the full, de-duplicated, sorted list of
// tools, including every transitively required toolset. Unknown names return
// an error. Cycles are tolerated (no infinite recursion).
func (r *Registry) Resolve(name string) ([]string, error) {
	seenSet := map[string]bool{}
	seenTool := map[string]bool{}
	var out []string

	var expand func(n string) error
	expand = func(n string) error {
		if seenSet[n] {
			return nil
		}
		s, ok := r.sets[n]
		if !ok {
			return errUnknown(n)
		}
		seenSet[n] = true
		for _, req := range s.Requires {
			if err := expand(req); err != nil {
				return err
			}
		}
		for _, t := range s.Tools {
			if !seenTool[t] {
				seenTool[t] = true
				out = append(out, t)
			}
		}
		return nil
	}
	if err := expand(name); err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

// Names returns all registered toolset names, sorted.
func (r *Registry) Names() []string {
	out := make([]string, 0, len(r.sets))
	for n := range r.sets {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Defaults returns the built-in toolset set.
func Defaults() []Toolset {
	return []Toolset{
		{
			Name:  "research",
			Tools: []string{"WebFetch", "WebSearch", "CodeSearch", "CodeMatch", "Grep", "Glob", "Read"},
		},
		{
			Name:     "dev",
			Tools:    []string{"Read", "Write", "Edit", "Bash", "Grep", "Glob", "CodeMatch", "ProjectVerify", "TaskRun"},
			Requires: []string{"research"},
		},
		{
			Name:     "ops",
			Tools:    []string{"Bash", "CronCreate", "CronDelete", "WebFetch", "Read"},
			Requires: []string{"research"},
		},
		{
			Name:     "full_stack",
			Requires: []string{"dev", "ops"},
		},
	}
}

func errDup(n string) error     { return &RegError{"duplicate toolset: " + n} }
func errUnknown(n string) error { return &RegError{"unknown toolset: " + n} }

// RegError is a toolset registry error.
type RegError struct{ msg string }

func (e *RegError) Error() string { return "toolset: " + e.msg }
