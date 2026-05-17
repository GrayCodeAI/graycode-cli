package inspect

import (
	"context"
	"sync"

	inspectLib "github.com/GrayCodeAI/inspect"
)

// Bridge connects hawk to the inspect site-auditing library.
// If initialization fails, all operations degrade gracefully and return
// empty results rather than errors.
type Bridge struct {
	scanner *inspectLib.Scanner
	mu      sync.Mutex
	ready   bool
}

// NewBridge creates a bridge to the inspect library with the given options.
// Returns a bridge that silently no-ops if initialization fails.
func NewBridge(opts ...inspectLib.Option) *Bridge {
	b := &Bridge{}
	b.init(opts...)
	return b
}

func (b *Bridge) init(opts ...inspectLib.Option) {
	b.scanner = inspectLib.NewScanner(opts...)
	b.ready = true
}

// Ready reports whether the inspect bridge is initialized and usable.
func (b *Bridge) Ready() bool {
	return b.ready
}

// Run crawls the target URL and runs all configured checks, returning a
// complete report with findings and stats. Falls back silently if the
// bridge is not initialized.
func (b *Bridge) Run(ctx context.Context, target string, opts ...inspectLib.Option) (*inspectLib.Report, error) {
	if !b.ready {
		return &inspectLib.Report{Target: target}, nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	// If additional per-call options are provided, create a one-off scanner;
	// otherwise reuse the bridge's scanner.
	if len(opts) > 0 {
		s := inspectLib.NewScanner(opts...)
		return s.Scan(ctx, target)
	}
	return b.scanner.Scan(ctx, target)
}
