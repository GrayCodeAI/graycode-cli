package cmd

import (
	"testing"

	hawkbranch "github.com/GrayCodeAI/hawk/internal/engine/branching"
	"github.com/GrayCodeAI/hawk/internal/provider/routing"
)

func TestCascadeSelectsForOpenCodeGoHi(t *testing.T) {
	roles := routing.DefaultRoles("opencodego/minimax-m2.5")
	t.Logf("cheapest=%s commit=%s", routing.CheapestForProvider("opencodego", "minimax-m2.5"), roles.Commit)
	cr := hawkbranch.NewCascadeRouter("opencodego/minimax-m2.5", roles)
	cr.Enabled = true
	got := cr.SelectModel("Hi", "opencodego/minimax-m2.5", "")
	t.Logf("selected=%s", got)
}
