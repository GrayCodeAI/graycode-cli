package cmd

import (
	"testing"

	graycodebranch "github.com/GrayCodeAI/graycode-cli/internal/engine/branching"
	"github.com/GrayCodeAI/graycode-cli/internal/provider/routing"
)

func TestCascadeSelectsForOpenCodeGoHi(t *testing.T) {
	roles := routing.DefaultRoles("opencodego/minimax-m2.5")
	t.Logf("cheapest=%s commit=%s", routing.CheapestForProvider("opencodego", "minimax-m2.5"), roles.Commit)
	cr := graycodebranch.NewCascadeRouter("opencodego/minimax-m2.5", roles)
	cr.Enabled = true
	got := cr.SelectModel("Hi", "opencodego/minimax-m2.5", "")
	t.Logf("selected=%s", got)
}
