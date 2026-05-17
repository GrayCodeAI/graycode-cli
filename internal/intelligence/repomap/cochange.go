package repomap

import (
	"os/exec"
	"sort"
	"strings"
)

// CoChangeAnalysis tracks which files frequently change together in git history.
type CoChangeAnalysis struct {
	// cooccurrence[fileA][fileB] = count of commits containing both
	cooccurrence map[string]map[string]int
}

// BuildCoChangeAnalysis parses the last N commits to find co-change patterns.
func BuildCoChangeAnalysis(root string, commitLimit int) (*CoChangeAnalysis, error) {
	if commitLimit <= 0 {
		commitLimit = 100
	}

	cmd := exec.Command("git", "log", "--name-only", "--pretty=format:", "-"+strings.Repeat("0", 0)+itoa(commitLimit))
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return &CoChangeAnalysis{cooccurrence: make(map[string]map[string]int)}, nil
	}

	ca := &CoChangeAnalysis{
		cooccurrence: make(map[string]map[string]int),
	}

	// Parse: commits are separated by blank lines
	commits := splitByEmptyLine(string(out))
	for _, commit := range commits {
		files := nonEmptyLines(commit)
		if len(files) < 2 {
			continue
		}
		// Record co-occurrence for each pair
		for i := 0; i < len(files); i++ {
			for j := i + 1; j < len(files); j++ {
				ca.record(files[i], files[j])
			}
		}
	}

	return ca, nil
}

// RelatedFiles returns files that frequently co-change with the given file,
// sorted by co-change frequency.
func (ca *CoChangeAnalysis) RelatedFiles(filePath string, topK int) []string {
	if ca == nil || ca.cooccurrence == nil {
		return nil
	}

	peers := ca.cooccurrence[filePath]
	if len(peers) == 0 {
		return nil
	}

	type scored struct {
		path  string
		count int
	}
	var candidates []scored
	for path, count := range peers {
		candidates = append(candidates, scored{path, count})
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].count > candidates[j].count
	})

	if topK > len(candidates) {
		topK = len(candidates)
	}

	result := make([]string, topK)
	for i := 0; i < topK; i++ {
		result[i] = candidates[i].path
	}
	return result
}

func (ca *CoChangeAnalysis) record(a, b string) {
	if ca.cooccurrence[a] == nil {
		ca.cooccurrence[a] = make(map[string]int)
	}
	if ca.cooccurrence[b] == nil {
		ca.cooccurrence[b] = make(map[string]int)
	}
	ca.cooccurrence[a][b]++
	ca.cooccurrence[b][a]++
}

func splitByEmptyLine(s string) []string {
	var chunks []string
	var current strings.Builder
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) == "" {
			if current.Len() > 0 {
				chunks = append(chunks, current.String())
				current.Reset()
			}
		} else {
			current.WriteString(line)
			current.WriteString("\n")
		}
	}
	if current.Len() > 0 {
		chunks = append(chunks, current.String())
	}
	return chunks
}

func nonEmptyLines(s string) []string {
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func itoa(n int) string {
	if n <= 0 {
		return "100"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}
