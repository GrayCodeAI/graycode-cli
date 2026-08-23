//go:build !cgo

package tool

import (
	"context"
	"fmt"
)

// matchSourceWithEngine is unavailable without cgo; the tool reports the
// limitation instead of pretending to search.
func runCodeMatch(ctx context.Context, root, pattern, language string, limit int) (string, error) {
	return "", fmt.Errorf("CodeMatch requires a cgo build (tree-sitter grammars are linked at compile time)")
}

type matchEngine struct{}

func newMatchEngine() *matchEngine { return &matchEngine{} }
