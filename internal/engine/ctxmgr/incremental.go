package ctxmgr

import (
	"fmt"
	"time"

	"github.com/GrayCodeAI/hawk/internal/systemcontext"
)

// Section defines a dynamic, incrementally-rendered system-prompt section. It
// is the hawk-host integration of the systemcontext package: each section is a
// typed context source whose value is loaded on demand, and only sections that
// actually change are re-rendered as a mid-conversation update rather than
// rebuilding the entire system prompt.
//
// The section's rendered content is emitted as a markdown block headed by
// Header (e.g. "## Relevant Memories"), matching the section-header convention
// already used by ReplaceSystemContextSection.
type Section struct {
	// Key is the stable namespaced source key (scope/name).
	Key string
	// Header is the markdown header used when the section is rendered.
	Header string
	// Load returns the current section value.
	Load func() (string, error)
}

// IncrementalContext reconciles a set of dynamic system-prompt sections
// against a durable snapshot, emitting only the sections that changed. It
// preserves a stable baseline for the unchanged sections across turns, so a
// provider prompt-cache prefix stays valid.
type IncrementalContext struct {
	ctx  *systemcontext.SystemContext
	rec  *systemcontext.Reconciler
	ep   *systemcontext.Epoch
	segs map[string]Section
}

// NewIncrementalContext builds an incremental context manager from the given
// sections. Callers must supply at least one section.
func NewIncrementalContext(sections []Section) (*IncrementalContext, error) {
	if len(sections) == 0 {
		return nil, fmt.Errorf("ctxmgr: incremental context requires at least one section")
	}
	host := &IncrementalContext{segs: map[string]Section{}}
	// Build typed sources; the value type is a plain string rendered under a
	// markdown header.
	var srcs []systemcontext.Source[string]
	for _, sec := range sections {
		host.segs[sec.Key] = sec
		header := sec.Header
		srcs = append(srcs, systemcontext.Source[string]{
			Key:   systemcontext.NewKey(keyScope(sec.Key), keyName(sec.Key)),
			Codec: systemcontext.JSONCodec(func(a, b string) bool { return a == b }),
			Load:  sec.Load,
			Baseline: func(v string) string {
				return renderSection(header, v)
			},
			Update: func(_, cur string) string {
				return renderSection(header, cur)
			},
		})
	}
	host.ctx = systemcontext.NewAll(srcs...)
	host.rec = systemcontext.NewReconciler(host.ctx)
	return host, nil
}

// Initialize renders the full baseline and stores its snapshot. Call once
// before the first request so an immutable baseline is established.
func (ic *IncrementalContext) Initialize() (string, error) {
	base, snap, err := ic.rec.Initialize()
	if err != nil {
		return "", err
	}
	ic.ep = systemcontext.NewEpoch(base, snap)
	return base, nil
}

// Reconcile admits context changes at a safe request boundary. It returns:
//
//   - (nil, false, nil): nothing changed; reuse the baseline.
//   - (msg, false, nil): a mid-conversation update to inject, with the stable
//     baseline unchanged.
//   - (msg, true, newBaseline): the baseline was replaced (compaction) and msg
//     carries the fresh baseline; the previous baseline must be discarded.
//   - (_, _, err): the reconcile failed; the caller should retry or fall back
//     to a full rebuild.
func (ic *IncrementalContext) Reconcile() (msg *string, replaced bool, newBaseline string, err error) {
	if ic.ep == nil {
		base, e := ic.Initialize()
		if e != nil {
			return nil, false, "", e
		}
		return &base, true, base, nil
	}
	return ic.ep.Prepare(ic.rec)
}

// Baseline returns the current immutable baseline, or "" before Initialize.
func (ic *IncrementalContext) Baseline() string {
	if ic.ep == nil {
		return ""
	}
	return ic.ep.Baseline
}

// SnapshotBytes returns the durable, marshalable snapshot of last-admitted
// section values. Empty string when the context was never initialized.
func (ic *IncrementalContext) SnapshotBytes() ([]byte, error) {
	if ic.ep == nil || ic.ep.Snapshot == nil {
		return nil, nil
	}
	return ic.ep.Snapshot.Marshal()
}

// RestoreSnapshot rebuilds the epoch baseline and snapshot from persisted
// bytes so an ongoing conversation can resume incremental reconciliation
// without re-rendering everything.
func (ic *IncrementalContext) RestoreSnapshot(b []byte) error {
	if ic.ep == nil {
		ic.ep = systemcontext.NewEpoch("", systemcontext.NewSnapshot())
	}
	var snap systemcontext.Snapshot
	if err := snap.Unmarshal(b); err != nil {
		return err
	}
	base := ic.rec.RenderBaseline(snap.Values)
	ic.ep.Baseline = base
	ic.ep.Snapshot = &snap
	return nil
}

// renderSection renders a section's value under its markdown header. The
// header is only emitted when the value is non-empty.
func renderSection(header, value string) string {
	if value == "" {
		return ""
	}
	if header == "" {
		return value
	}
	return header + "\n" + value
}

func keyScope(key string) string {
	// A section key like "memories" or "scope/name" maps to a valid
	// namespaced source key.
	return "hawk"
}

func keyName(key string) string {
	return key
}

// DefaultIncrementalSections returns a reasonable set of dynamic sections for
// a session, wired to the same loaders hawk already uses for each. It is a
// convenience for hosts that want to enable incremental context without
// hand-assembling sections. Loaders may be nil-able wrappers; the returned
// sections must be configured with their Load funcs by the caller.
func DefaultIncrementalSections() []Section {
	return []Section{
		{Key: "environment", Header: "## Environment"},
		{Key: "date", Header: "## Date"},
		{Key: "memories", Header: "## Relevant Memories"},
		{Key: "directives", Header: "## Directives"},
	}
}

// NowStr formats the current time for the date section.
func NowStr() string {
	return time.Now().Format(time.RFC3339)
}
