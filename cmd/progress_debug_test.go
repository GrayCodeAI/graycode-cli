package cmd

import (
	"strings"
	"testing"

	"github.com/GrayCodeAI/hawk/internal/ui/icons"
)

func TestZDebugProgress(t *testing.T) {
	t.Logf("CheckBold: %q", icons.CheckBold())
	pt := NewProgressTracker("Implementing JWT Authentication")
	pt.AddStep("Read existing auth code")
	pt.AddStep("Plan approach")
	pt.AddStep("Implement JWT middleware")
	pt.AddStep("Write tests")
	pt.AddStep("Verify and commit")
	pt.StartStep(0)
	pt.CompleteStep(0)
	pt.StartStep(1)
	pt.CompleteStep(1)
	pt.StartStep(2)
	output := pt.Render()
	t.Logf("Output: %q", output)
	if !strings.Contains(output, icons.CheckBold()+" Plan approach") {
		t.Errorf("missing 'Plan approach' line")
	}
}
