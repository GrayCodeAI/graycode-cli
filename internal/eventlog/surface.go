package eventlog

import (
	"fmt"
)

// Surface op marker values carried by a surface-eligible event's SurfaceOp.
const (
	SurfaceOpAppend  = "append"
	SurfaceOpReplace = "replace"
)

// SurfaceFoldReplacement records one positional replacement operation observed
// while folding a session surface: the replacing event's seq and the surface
// range it shadowed. Ported from DSH's surface.ts SurfaceFoldReplacement.
type SurfaceFoldReplacement struct {
	// Seq is the seq of the event that replaced the prior surface range.
	Seq uint64
	// Start is the declared inclusive start seq of the replaced surface range.
	Start uint64
	// End is the declared inclusive end seq of the replaced surface range.
	End uint64
	// ShadowedSeqs are the actual surface entries removed by the operation, in
	// surface order.
	ShadowedSeqs []uint64
}

// SurfaceFoldResult is the complete result of replaying the surface operations
// in a session log. Ported from DSH's surface.ts SurfaceFoldResult.
type SurfaceFoldResult struct {
	// Nodes are the current surface event sequences in model-visible order.
	Nodes []uint64
	// Replacements are the replacement operations in event order.
	Replacements []SurfaceFoldReplacement
}

// surfaceFoldState is the mutable state shared by complete and incremental
// folds. Replacement history is deliberately not retained here: the live
// surface only needs the current nodes and a monotonic generation counter so a
// consumer can detect a positional rewrite cheaply (DSH parity).
type surfaceFoldState struct {
	nodes      []uint64
	replaceGen uint64
}

// surfaceReplacePlan is a validated replacement transition that has not yet
// mutated the fold state. Ported from DSH's SurfaceReplacePlan.
type surfaceReplacePlan struct {
	start        uint64
	end          uint64
	startIdx     int
	endIdx       int
	shadowedSeqs []uint64
}

// surfacePlan is one validated surface transition that has not yet mutated the
// fold state. Ported from DSH's SurfacePlan (append | replace).
type surfacePlan struct {
	kind    string // "append" | "replace"
	seq     uint64
	replace *surfaceReplacePlan
}

// indexOf returns the index of seq in nodes, or -1 when absent.
func indexOfSeq(nodes []uint64, seq uint64) int {
	for i, s := range nodes {
		if s == seq {
			return i
		}
	}
	return -1
}

// surfaceOpOf validates an event's local surface eligibility and returns its
// operation. Non-surface-eligible types must carry neither a surfaceOp marker
// nor source-event references; surface-eligible types must carry a marker.
// Ported from DSH's surfaceOpOf.
func surfaceOpOf(ev Event) (*SurfaceOp, error) {
	if !ev.Type.IsSurfaceEligible() {
		if ev.SurfaceOp != nil {
			return nil, fmt.Errorf("eventlog: session event %q is not surface-eligible and cannot carry surfaceOp", ev.Type)
		}
		if len(ev.SourceEventSeqs) > 0 {
			return nil, fmt.Errorf("eventlog: session event %q is not surface-eligible and cannot carry sourceEventSeqs", ev.Type)
		}
		return nil, nil
	}
	if ev.SurfaceOp == nil {
		return nil, fmt.Errorf("eventlog: session event %q is surface-eligible and requires a surfaceOp marker", ev.Type)
	}
	op := ev.SurfaceOp
	if op.Op != SurfaceOpAppend && op.Op != SurfaceOpReplace {
		return nil, fmt.Errorf("eventlog: session event %q carries an invalid surfaceOp op %q", ev.Type, op.Op)
	}
	// Start/End are unsigned absolute seqs; any replace must name a range that
	// exists in the surface, which replacementRange enforces, so no further
	// shape check is needed here.
	return op, nil
}

// assertProvenance validates an event's cited source-event seqs against prior
// log entries and (for a replacement) the shadowed surface range. Sources must
// reference strictly earlier events, be free of duplicates, and cover every
// shadowed surface node. Ported from DSH's assertProvenance.
func assertProvenance(ev Event, shadowedSeqs []uint64) error {
	// DSH invariant: an explicitly-present-but-empty source list is only
	// tolerated on assistant/message; every other surface event that carries
	// the field must cite at least one earlier event.
	if ev.SourceEventSeqs != nil && len(ev.SourceEventSeqs) == 0 && ev.Type != AssistantMsg {
		return fmt.Errorf("eventlog: session event %q sourceEventSeqs must not be empty (only assistant/message may carry an explicit empty list)", ev.Type)
	}
	sources := make(map[uint64]bool)
	for _, source := range ev.SourceEventSeqs {
		if sources[source] {
			return fmt.Errorf("eventlog: session event %q sourceEventSeqs must not contain duplicates (%d)", ev.Type, source)
		}
		if source >= ev.Seq {
			return fmt.Errorf("eventlog: session event %q sourceEventSeqs must reference earlier events: %d >= current seq %d", ev.Type, source, ev.Seq)
		}
		sources[source] = true
	}
	for _, seq := range shadowedSeqs {
		if !sources[seq] {
			return fmt.Errorf("eventlog: surface replace: sourceEventSeqs must include every shadowed surface node; missing %d", seq)
		}
	}
	return nil
}

// replacementRange locates a replacement range in the current fold state
// without mutating it. Ported from DSH's replacementRange.
func replacementRange(state *surfaceFoldState, op *SurfaceOp) (*surfaceReplacePlan, error) {
	startIdx := indexOfSeq(state.nodes, op.Start)
	if startIdx == -1 {
		return nil, fmt.Errorf("eventlog: surface replace: start seq %d not found in surface", op.Start)
	}
	endIdx := indexOfSeq(state.nodes, op.End)
	if endIdx == -1 {
		return nil, fmt.Errorf("eventlog: surface replace: end seq %d not found in surface", op.End)
	}
	if startIdx > endIdx {
		return nil, fmt.Errorf("eventlog: surface replace: start seq %d (index %d) is after end seq %d (index %d)", op.Start, startIdx, op.End, endIdx)
	}
	shadowed := make([]uint64, endIdx-startIdx+1)
	copy(shadowed, state.nodes[startIdx:endIdx+1])
	return &surfaceReplacePlan{
		start:        op.Start,
		end:          op.End,
		startIdx:     startIdx,
		endIdx:       endIdx,
		shadowedSeqs: shadowed,
	}, nil
}

// toolResultDataEqualRest reports whether two tool/result payloads agree on
// every field except their content, mirroring DSH's tool-result rewrite rule:
// a surface replacement of a tool/result may change only the content text; the
// call identity, turn/step coordinates, and error flag must be preserved.
func toolResultDataEqualRest(a, b ToolResultPayload) bool {
	return a.Turn == b.Turn &&
		a.Step == b.Step &&
		a.ToolUseID == b.ToolUseID &&
		a.IsError == b.IsError
}

// assertToolResultRewrite restricts a tool/result replacement to rewriting
// exactly one current result's content. Ported from DSH's
// assertToolResultRewrite.
func assertToolResultRewrite(ev Event, shadowedSeqs []uint64, events []Event, baseSeq uint64) error {
	if ev.Type != ToolResult {
		return nil
	}
	if len(shadowedSeqs) != 1 {
		return fmt.Errorf("eventlog: tool/result surface replacement must rewrite exactly one current node")
	}
	for _, originalSeq := range shadowedSeqs {
		if originalSeq < baseSeq || originalSeq-baseSeq >= uint64(len(events)) {
			return fmt.Errorf("eventlog: tool/result surface replacement shadowed seq %d is outside the log window", originalSeq)
		}
		original := events[originalSeq-baseSeq]
		if original.Type != ToolResult {
			return fmt.Errorf("eventlog: tool/result surface replacement must target a current tool/result")
		}
		orig, okO := original.Data.(ToolResultPayload)
		repl, okR := ev.Data.(ToolResultPayload)
		if !okO || !okR {
			return fmt.Errorf("eventlog: tool/result surface replacement must carry a ToolResultPayload")
		}
		if !toolResultDataEqualRest(orig, repl) {
			return fmt.Errorf("eventlog: tool/result surface replacement may change only content")
		}
	}
	return nil
}

// planSurfaceEvent validates one event at its replay boundary and prepares its
// atomic fold transition without mutating the committed fold state. Ported
// from DSH's planSurfaceEvent.
func planSurfaceEvent(state *surfaceFoldState, ev Event, expectedSeq uint64, events []Event, baseSeq uint64) (*surfacePlan, error) {
	if ev.Seq != expectedSeq {
		return nil, fmt.Errorf("eventlog: session event seq %d is not contiguous; expected %d", ev.Seq, expectedSeq)
	}
	op, err := surfaceOpOf(ev)
	if err != nil || op == nil {
		return nil, err
	}
	if op.Op == SurfaceOpAppend {
		if err := assertProvenance(ev, nil); err != nil {
			return nil, err
		}
		return &surfacePlan{kind: SurfaceOpAppend, seq: ev.Seq}, nil
	}
	repl, err := replacementRange(state, op)
	if err != nil {
		return nil, err
	}
	if err := assertProvenance(ev, repl.shadowedSeqs); err != nil {
		return nil, err
	}
	if err := assertToolResultRewrite(ev, repl.shadowedSeqs, events, baseSeq); err != nil {
		return nil, err
	}
	return &surfacePlan{kind: SurfaceOpReplace, seq: ev.Seq, replace: repl}, nil
}

// applySurfacePlan commits one previously validated surface transition and
// returns replacement metadata when one occurred. Ported from DSH's
// applySurfacePlan.
func applySurfacePlan(state *surfaceFoldState, plan *surfacePlan) *SurfaceFoldReplacement {
	if plan == nil {
		return nil
	}
	if plan.kind == SurfaceOpAppend {
		state.nodes = append(state.nodes, plan.seq)
		return nil
	}
	repl := plan.replace
	head := append([]uint64{}, state.nodes[:repl.startIdx]...)
	tail := append([]uint64{}, state.nodes[repl.endIdx+1:]...)
	state.nodes = append(head, append([]uint64{plan.seq}, tail...)...)
	state.replaceGen++
	return &SurfaceFoldReplacement{
		Seq:          plan.seq,
		Start:        repl.start,
		End:          repl.end,
		ShadowedSeqs: append([]uint64{}, repl.shadowedSeqs...),
	}
}

// applySurfaceEvent validates and applies one event's transition, returning
// replacement metadata only when one occurred. Ported from DSH's
// applySurfaceEvent.
func applySurfaceEvent(state *surfaceFoldState, ev Event, expectedSeq uint64, events []Event, baseSeq uint64) (*SurfaceFoldReplacement, error) {
	plan, err := planSurfaceEvent(state, ev, expectedSeq, events, baseSeq)
	if err != nil {
		return nil, err
	}
	return applySurfacePlan(state, plan), nil
}

// FoldSurface replays a complete session log through the canonical surface
// fold: it returns the current surface event sequences in model-visible order
// together with the committed replacement history. It fails loud — returning
// the first violation — whenever an event violates surface metadata,
// source-event references, replacement range, contiguity, or tool-result
// rewrite rules. This is the pure, deterministic counterpart to the
// incremental SurfaceManager. Ported from DSH's foldSurface.
func FoldSurface(events []Event) (SurfaceFoldResult, error) {
	state := &surfaceFoldState{}
	var baseSeq uint64
	if len(events) > 0 {
		baseSeq = events[0].Seq
	}
	var result SurfaceFoldResult
	for i, ev := range events {
		expectedSeq := baseSeq + uint64(i)
		repl, err := applySurfaceEvent(state, ev, expectedSeq, events, baseSeq)
		if err != nil {
			return SurfaceFoldResult{}, err
		}
		if repl != nil {
			result.Replacements = append(result.Replacements, *repl)
		}
	}
	result.Nodes = append([]uint64{}, state.nodes...)
	return result, nil
}

// SurfaceManager is the incremental, live counterpart to FoldSurface. It is
// bound to a *Log and lazily folds only the events appended since the last
// access, so a consumer can observe the model-visible surface and detect
// positional rewrites (replaceGeneration) without replaying the log. Ported
// from DSH's SurfaceManager.
//
// ValidateNext pre-flights an event against the committed fold state and
// returns an error without mutating it, so a caller can reject an invalid
// append/replace before it is ever admitted to the log (atomic admission).
type SurfaceManager struct {
	log *Log

	state     surfaceFoldState
	baseSeq   uint64
	started   bool
	processed int
}

// NewSurfaceManager binds an incremental surface manager to l. The fold is
// lazy: nothing is computed until Nodes, ReplaceGeneration, or ValidateNext is
// called.
func NewSurfaceManager(l *Log) *SurfaceManager {
	return &SurfaceManager{log: l}
}

// catchUp folds every log event not yet reflected in the committed surface,
// returning the first validation error encountered.
func (m *SurfaceManager) catchUp() error {
	events := m.log.Snapshot()
	if len(events) == 0 {
		return nil
	}
	if !m.started {
		m.baseSeq = events[0].Seq
		m.started = true
	}
	for m.processed < len(events) {
		ev := events[m.processed]
		expectedSeq := m.baseSeq + uint64(m.processed)
		repl, err := applySurfaceEvent(&m.state, ev, expectedSeq, events, m.baseSeq)
		if err != nil {
			return err
		}
		_ = repl
		m.processed++
	}
	return nil
}

// Nodes returns the current model-visible surface event sequences in order,
// folding any log delta first.
func (m *SurfaceManager) Nodes() ([]uint64, error) {
	if err := m.catchUp(); err != nil {
		return nil, err
	}
	return append([]uint64{}, m.state.nodes...), nil
}

// ReplaceGeneration returns a monotonically increasing count of replacement
// operations committed to the surface, folding any log delta first. It starts
// at 0 and increments once per successful replacement, so a consumer can cheaply
// detect a positional rewrite.
func (m *SurfaceManager) ReplaceGeneration() (uint64, error) {
	if err := m.catchUp(); err != nil {
		return 0, err
	}
	return m.state.replaceGen, nil
}

// ValidateNext pre-flights ev against the committed surface. It validates that
// adding ev as the next event (at the next contiguous seq) would be legal and
// returns the first violation as an error. It never mutates the committed fold
// state, so a rejected candidate leaves both the surface and the generation
// counter unchanged (atomic admission at the orchestration layer).
func (m *SurfaceManager) ValidateNext(ev Event) error {
	if err := m.catchUp(); err != nil {
		return err
	}
	events := m.log.Snapshot()
	// The candidate occupies the next contiguous position after the committed tail.
	expectedSeq := m.baseSeq + uint64(m.processed)
	_, err := planSurfaceEvent(&m.state, ev, expectedSeq, events, m.baseSeq)
	return err
}
