package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// TeamConfig represents a team of collaborating agents.
type TeamConfig struct {
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	LeadAgent   string    `json:"leadAgent,omitempty"`
	Members     []string  `json:"members"`
	CreatedAt   time.Time `json:"createdAt"`
}

// TeamRegistry manages active teams.
type TeamRegistry struct {
	mu    sync.RWMutex
	teams map[string]*TeamConfig
}

var globalTeamRegistry = &TeamRegistry{teams: make(map[string]*TeamConfig)}

func GetTeamRegistry() *TeamRegistry { return globalTeamRegistry }

func (r *TeamRegistry) Create(name, description, agentType string) (*TeamConfig, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.teams[name]; exists {
		return nil, fmt.Errorf("team %q already exists", name)
	}
	team := &TeamConfig{
		Name:        name,
		Description: description,
		LeadAgent:   agentType,
		Members:     []string{},
		CreatedAt:   time.Now(),
	}
	r.teams[name] = team

	// Persist team config to disk
	teamDir := teamDirPath(name)
	if err := os.MkdirAll(teamDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating team directory: %w", err)
	}
	configPath := filepath.Join(teamDir, "config.json")
	data, _ := json.MarshalIndent(team, "", "  ")
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		return nil, fmt.Errorf("writing team config: %w", err)
	}

	// Create task list directory for team
	taskDir := taskDirPath(name)
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating task directory: %w", err)
	}

	return team, nil
}

func (r *TeamRegistry) Get(name string) (*TeamConfig, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.teams[name]
	return t, ok
}

func (r *TeamRegistry) Delete(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	team, ok := r.teams[name]
	if !ok {
		return fmt.Errorf("team %q not found", name)
	}
	if len(team.Members) > 0 {
		return fmt.Errorf("team %q still has %d active members; shut them down first", name, len(team.Members))
	}
	delete(r.teams, name)

	// Clean up disk
	_ = os.RemoveAll(teamDirPath(name))
	_ = os.RemoveAll(taskDirPath(name))
	return nil
}

func (r *TeamRegistry) AddMember(teamName, member string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	team, ok := r.teams[teamName]
	if !ok {
		return
	}
	team.Members = append(team.Members, member)
}

func (r *TeamRegistry) RemoveMember(teamName, member string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	team, ok := r.teams[teamName]
	if !ok {
		return
	}
	filtered := team.Members[:0]
	for _, m := range team.Members {
		if m != member {
			filtered = append(filtered, m)
		}
	}
	team.Members = filtered
}

func teamDirPath(name string) string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".hawk", "teams", name)
}

func taskDirPath(name string) string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".hawk", "tasks", name)
}

// TeamCreateTool creates a new team to coordinate multiple agents.
type TeamCreateTool struct{}

func (TeamCreateTool) Name() string        { return "TeamCreate" }
func (TeamCreateTool) Aliases() []string   { return []string{"team_create"} }
func (TeamCreateTool) Description() string { return "Create a new team to coordinate multiple agents" }
func (TeamCreateTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"team_name":   map[string]interface{}{"type": "string", "description": "Name for the new team"},
			"description": map[string]interface{}{"type": "string", "description": "Team description/purpose"},
			"agent_type":  map[string]interface{}{"type": "string", "description": "Type/role of the team lead"},
		},
		"required": []string{"team_name"},
	}
}

func (TeamCreateTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var p struct {
		TeamName    string `json:"team_name"`
		Description string `json:"description"`
		AgentType   string `json:"agent_type"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	if p.TeamName == "" {
		return "", fmt.Errorf("team_name is required")
	}

	team, err := globalTeamRegistry.Create(p.TeamName, p.Description, p.AgentType)
	if err != nil {
		return "", err
	}

	out, _ := json.Marshal(map[string]any{
		"team_name":   team.Name,
		"description": team.Description,
		"team_dir":    teamDirPath(team.Name),
		"task_dir":    taskDirPath(team.Name),
	})
	return string(out), nil
}

// TeamDeleteTool disbands a team and cleans up resources.
type TeamDeleteTool struct{}

func (TeamDeleteTool) Name() string        { return "TeamDelete" }
func (TeamDeleteTool) Aliases() []string   { return []string{"team_delete"} }
func (TeamDeleteTool) Description() string { return "Remove a team and its task directories" }
func (TeamDeleteTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"team_name": map[string]interface{}{"type": "string", "description": "Name of the team to delete"},
		},
		"required": []string{"team_name"},
	}
}

func (TeamDeleteTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var p struct {
		TeamName string `json:"team_name"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	if p.TeamName == "" {
		return "", fmt.Errorf("team_name is required")
	}
	if err := globalTeamRegistry.Delete(p.TeamName); err != nil {
		return "", err
	}
	out, _ := json.Marshal(map[string]any{
		"success":   true,
		"message":   fmt.Sprintf("Team %q deleted", p.TeamName),
		"team_name": p.TeamName,
	})
	return string(out), nil
}
