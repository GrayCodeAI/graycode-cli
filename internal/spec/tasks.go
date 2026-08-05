package spec

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

type Task struct {
	ID          string
	Number      string
	Description string
	Phase       string
	Files       []string
	DependsOn   []string
	ReqIDs      []string
	LineNumber  int
}

type TaskGroup struct {
	Tasks    []Task
	Parallel bool
	Reason   string
}

var (
	reTaskCheckbox  = regexp.MustCompile(`^- \[([ xX])\]\s+(.+)$`)
	reTaskPhaseHdr  = regexp.MustCompile(`^##\s+(\d+)\.\s*(.+)$`)
	reTaskFileRef   = regexp.MustCompile(`(?i)(?:files?[:\s]+)([\w./]+(?:\.go|\.ts|\.py|\.rs|\.js)?)`)
	reTaskReqRef    = regexp.MustCompile(`REQ-(\d+)(?:\.(\d+))?(?:\.(\d+))?`)
	reTaskDependsOn = regexp.MustCompile(`(?i)(?:depends?[:\s]+)(.+)`)
)

func ParseTasks(content string) []Task {
	var tasks []Task
	lines := strings.Split(content, "\n")
	currentPhase := "General"

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if m := reTaskPhaseHdr.FindStringSubmatch(trimmed); m != nil {
			currentPhase = strings.TrimSpace(m[2])
			continue
		}
		if m := reTaskCheckbox.FindStringSubmatch(trimmed); m != nil {
			done := strings.ToLower(m[1]) == "x"
			if done {
				continue
			}
			desc := m[2]
			task := Task{
				ID:          extractTaskID(desc),
				Number:      extractTaskNumber(desc),
				Description: desc,
				Phase:       currentPhase,
				Files:       extractFileRefs(desc),
				ReqIDs:      extractReqRefs(desc),
				LineNumber:  i + 1,
			}
			if deps := reTaskDependsOn.FindStringSubmatch(desc); deps != nil {
				task.DependsOn = parseDependsList(deps[1])
			}
			tasks = append(tasks, task)
		}
	}
	return tasks
}

func extractTaskID(desc string) string {
	if m := reTaskReqRef.FindStringSubmatch(desc); m != nil {
		return m[0]
	}
	return ""
}

func extractTaskNumber(desc string) string {
	fields := strings.Fields(desc)
	if len(fields) > 0 {
		last := fields[len(fields)-1]
		if strings.HasSuffix(last, ".") && len(last) <= 5 {
			return strings.TrimSuffix(last, ".")
		}
	}
	return ""
}

func extractFileRefs(desc string) []string {
	matches := reTaskFileRef.FindAllStringSubmatch(desc, -1)
	var files []string
	for _, m := range matches {
		files = append(files, m[1])
	}
	return files
}

func extractReqRefs(desc string) []string {
	return reTaskReqRef.FindAllString(desc, -1)
}

func parseDependsList(s string) []string {
	parts := strings.Split(s, ",")
	var deps []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			deps = append(deps, trimmed)
		}
	}
	return deps
}

func AnalyzeTaskGroups(tasks []Task) []TaskGroup {
	if len(tasks) == 0 {
		return nil
	}

	var groups []TaskGroup
	taskFiles := make(map[string]bool)
	for _, t := range tasks {
		for _, f := range t.Files {
			taskFiles[f] = true
		}
	}

	fileUsers := make(map[string][]string)
	for _, t := range tasks {
		for _, f := range t.Files {
			fileUsers[f] = append(fileUsers[f], t.ID)
		}
	}

	sharedFiles := make(map[string]bool)
	for f, users := range fileUsers {
		if len(users) > 1 {
			sharedFiles[f] = true
		}
	}

	var parallel []Task
	var sequential []Task
	for _, t := range tasks {
		hasConflict := false
		for _, f := range t.Files {
			if sharedFiles[f] {
				hasConflict = true
				break
			}
		}
		if len(t.DependsOn) > 0 {
			hasConflict = true
		}
		if hasConflict {
			sequential = append(sequential, t)
		} else {
			parallel = append(parallel, t)
		}
	}

	if len(parallel) > 0 {
		groups = append(groups, TaskGroup{
			Tasks:    parallel,
			Parallel: true,
			Reason:   "No file conflicts or explicit dependencies",
		})
	}

	if len(sequential) > 0 {
		sort.Slice(sequential, func(i, j int) bool {
			return len(sequential[i].DependsOn) < len(sequential[j].DependsOn)
		})
		groups = append(groups, TaskGroup{
			Tasks:    sequential,
			Parallel: false,
			Reason:   "Has file conflicts or explicit dependencies",
		})
	}

	return groups
}

func FormatTaskGroups(groups []TaskGroup) string {
	var b strings.Builder
	b.WriteString("## Task Execution Plan\n\n")
	for i, g := range groups {
		if g.Parallel {
			fmt.Fprintf(&b, "### Group %d (Parallel)\n\n%d tasks can execute concurrently: %s\n\n", i+1, len(g.Tasks), g.Reason)
		} else {
			fmt.Fprintf(&b, "### Group %d (Sequential)\n\n%d tasks must execute in order: %s\n\n", i+1, len(g.Tasks), g.Reason)
		}
		for _, t := range g.Tasks {
			fmt.Fprintf(&b, "- %s\n", t.Description)
			if len(t.Files) > 0 {
				fmt.Fprintf(&b, "  Files: %s\n", strings.Join(t.Files, ", "))
			}
			if len(t.ReqIDs) > 0 {
				fmt.Fprintf(&b, "  Reqs: %s\n", strings.Join(t.ReqIDs, ", "))
			}
		}
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}
