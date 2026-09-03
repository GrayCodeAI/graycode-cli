package plugin

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/GrayCodeAI/graycode-cli/internal/storage"
)

// SkillProvider defines the interface for skill sources.
type SkillProvider interface {
	Name() string
	List(ctx context.Context, cwd string) ([]SkillEntry, error)
	Get(ctx context.Context, cwd, name string) (*SkillEntry, error)
}

// GlobalSkillRegistry manages multi-layer skill providers with shadowing semantics.
type GlobalSkillRegistry struct {
	mu        sync.RWMutex
	providers []SkillProvider
}

// DefaultRegistry is the process-wide skill registry.
var DefaultRegistry = NewSkillRegistry()

// NewSkillRegistry creates a new registry initialized with the default filesystem provider.
func NewSkillRegistry() *GlobalSkillRegistry {
	r := &GlobalSkillRegistry{}
	_ = r.Register(NewFilesystemSkillProvider())
	return r
}

// Register adds a provider to the front of the provider list (higher priority)
// and returns a disposer function that unregisters it.
func (r *GlobalSkillRegistry) Register(p SkillProvider) func() {
	r.mu.Lock()
	r.providers = append([]SkillProvider{p}, r.providers...)
	r.mu.Unlock()

	return func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		for i, prov := range r.providers {
			if prov == p {
				r.providers = append(r.providers[:i], r.providers[i+1:]...)
				break
			}
		}
	}
}

// List returns all active skills visible for the given cwd, with higher-priority
// providers shadowing lower-priority providers on name collision (nearest wins).
func (r *GlobalSkillRegistry) List(ctx context.Context, cwd string) ([]SkillEntry, error) {
	r.mu.RLock()
	providers := make([]SkillProvider, len(r.providers))
	copy(providers, r.providers)
	r.mu.RUnlock()

	seen := make(map[string]bool)
	var result []SkillEntry

	for _, p := range providers {
		entries, err := p.List(ctx, cwd)
		if err != nil {
			continue
		}
		for _, e := range entries {
			lowerName := strings.ToLower(e.Name)
			if !seen[lowerName] {
				seen[lowerName] = true
				if e.Provider == "" {
					e.Provider = p.Name()
				}
				result = append(result, e)
			}
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result, nil
}

// Get returns the winning skill definition for a given name in cwd.
func (r *GlobalSkillRegistry) Get(ctx context.Context, cwd, name string) (*SkillEntry, error) {
	r.mu.RLock()
	providers := make([]SkillProvider, len(r.providers))
	copy(providers, r.providers)
	r.mu.RUnlock()

	lowerTarget := strings.ToLower(name)
	for _, p := range providers {
		entry, err := p.Get(ctx, cwd, name)
		if err == nil && entry != nil && strings.ToLower(entry.Name) == lowerTarget {
			if entry.Provider == "" {
				entry.Provider = p.Name()
			}
			return entry, nil
		}
	}
	return nil, fmt.Errorf("skill %q not found", name)
}

// FilesystemSkillProvider discovers skills on disk across scoped project roots and user roots.
type FilesystemSkillProvider struct {
	name string
}

// NewFilesystemSkillProvider creates a filesystem provider.
func NewFilesystemSkillProvider() *FilesystemSkillProvider {
	return &FilesystemSkillProvider{name: "filesystem"}
}

func (p *FilesystemSkillProvider) Name() string {
	if p.name != "" {
		return p.name
	}
	return "filesystem"
}

func (p *FilesystemSkillProvider) List(_ context.Context, cwd string) ([]SkillEntry, error) {
	roots := SkillRoots(cwd)
	seen := make(map[string]bool)
	var entries []SkillEntry

	for _, root := range roots {
		dirEntries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, entry := range dirEntries {
			path := filepath.Join(root, entry.Name())
			if entry.IsDir() {
				// Directory-based skill (e.g. skills/my-skill/SKILL.md)
				for _, filename := range []string{"SKILL.md", "skill.md", entry.Name() + ".md"} {
					candidate := filepath.Join(path, filename)
					if fileExists(candidate) {
						s, ok := readSkillFile(candidate, entry.Name())
						if ok {
							lowerName := strings.ToLower(s.Name)
							if !seen[lowerName] {
								seen[lowerName] = true
								entries = append(entries, s)
							}
						}
						break
					}
				}
				continue
			}
			if strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
				name := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
				s, ok := readSkillFile(path, name)
				if ok {
					lowerName := strings.ToLower(s.Name)
					if !seen[lowerName] {
						seen[lowerName] = true
						entries = append(entries, s)
					}
				}
			}
		}
	}

	return entries, nil
}

func (p *FilesystemSkillProvider) Get(ctx context.Context, cwd, name string) (*SkillEntry, error) {
	skills, err := p.List(ctx, cwd)
	if err != nil {
		return nil, err
	}
	lowerTarget := strings.ToLower(name)
	for _, s := range skills {
		if strings.ToLower(s.Name) == lowerTarget {
			return &s, nil
		}
	}
	return nil, fmt.Errorf("skill %q not found in filesystem provider", name)
}

func readSkillFile(path, defaultName string) (SkillEntry, bool) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is derived from discovered skill directories, not direct external input
	if err != nil {
		return SkillEntry{}, false
	}
	s, ok := parseSkillFrontMatter(string(data))
	if !ok {
		// If frontmatter is missing name, fall back to default filename/directory name
		s = Skill{
			Name:        defaultName,
			Description: "",
			Content:     string(data),
		}
	}
	if s.Name == "" {
		s.Name = defaultName
	}
	return SkillEntry{
		Name:         s.Name,
		Description:  s.Description,
		Content:      s.Content,
		Path:         path,
		ResourceBase: filepath.Dir(path),
		Invocation:   s.Invocation,
		Provider:     "filesystem",
	}, true
}

// SkillRoots returns all search directories for filesystem skills ordered from
// most specific (project-local cwd) to most general (user state/home).
func SkillRoots(cwd string) []string {
	var roots []string
	if cwd == "" {
		if cur, err := os.Getwd(); err == nil {
			cwd = cur
		}
	}
	if cwd != "" {
		roots = append(
			roots,
			filepath.Join(cwd, ".agents", "skills"),
			filepath.Join(cwd, ".claude", "skills"),
			filepath.Join(cwd, ".codex", "skills"),
			filepath.Join(cwd, ".zero", "skills"),
			filepath.Join(cwd, "skills"),
		)
	}
	roots = append(roots, filepath.Join(storage.StateDir(), "skills"))
	if home, err := os.UserHomeDir(); err == nil {
		roots = append(
			roots,
			filepath.Join(home, ".graycode", "skills"),
			filepath.Join(home, ".agents", "skills"),
			filepath.Join(home, ".claude", "skills"),
			filepath.Join(home, ".codex", "skills"),
		)
	}
	return roots
}

// RuntimeSkillProvider provides in-memory programmatic skills.
type RuntimeSkillProvider struct {
	mu     sync.RWMutex
	name   string
	skills map[string]SkillEntry
}

// NewRuntimeSkillProvider creates a new in-memory skill provider.
func NewRuntimeSkillProvider(name string) *RuntimeSkillProvider {
	if name == "" {
		name = "runtime"
	}
	return &RuntimeSkillProvider{
		name:   name,
		skills: make(map[string]SkillEntry),
	}
}

func (p *RuntimeSkillProvider) Name() string {
	return p.name
}

func (p *RuntimeSkillProvider) AddSkill(entry SkillEntry) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if entry.Provider == "" {
		entry.Provider = p.name
	}
	p.skills[strings.ToLower(entry.Name)] = entry
}

func (p *RuntimeSkillProvider) RemoveSkill(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.skills, strings.ToLower(name))
}

func (p *RuntimeSkillProvider) List(_ context.Context, _ string) ([]SkillEntry, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	entries := make([]SkillEntry, 0, len(p.skills))
	for _, s := range p.skills {
		entries = append(entries, s)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})
	return entries, nil
}

func (p *RuntimeSkillProvider) Get(_ context.Context, _, name string) (*SkillEntry, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	entry, ok := p.skills[strings.ToLower(name)]
	if !ok {
		return nil, fmt.Errorf("skill %q not found in provider %s", name, p.name)
	}
	return &entry, nil
}
