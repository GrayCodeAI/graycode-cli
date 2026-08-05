package tool

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
)

type SpecCorrectTool struct{}

func (SpecCorrectTool) Name() string { return "SpecCorrect" }

func (SpecCorrectTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var p struct {
		ScanDir string `json:"scan_dir"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	if p.ScanDir == "" {
		p.ScanDir, _ = os.Getwd()
	}

	var b strings.Builder
	b.WriteString("Self-Correction Loop")

	if output, err := runCorrectCmd(p.ScanDir, "go", "test", "./..."); err != nil {
		b.WriteString("FAIL: " + output)
	} else {
		b.WriteString("OK")
	}

	return b.String(), nil
}

func runCorrectCmd(dir, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func init() { _ = SpecCorrectTool{} }
