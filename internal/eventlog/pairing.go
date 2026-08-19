package eventlog

import "fmt"

// This file ports DSH's compaction tool-pairing boundary contract
// (packages/compaction/compaction/src/tool-pairing.ts + spec): a compaction
// cut is legal only when no open tool.call lacks its matching tool.result
// across the edge. The DSH surface rules are kept verbatim — only
// message.user, message.assistant and tool.result are surface nodes;
// session.compacted and every other durable event are log-only. Shadowed
// events stay in the raw log for deterministic replay; the replace lands a
// fresh high-seq summary node.

// surfaceDelta returns the contribution of one surface node to the
// in-progress tool-call count (DSH eventDelta): assistant/message adds the
// count of its tool-call blocks, tool/result subtracts one, all other surface
// nodes contribute zero.
func surfaceDelta(ev Event) int {
	switch ev.Type {
	case AssistantMsg:
		if m, ok := ev.Data.(Message); ok {
			return len(m.ToolUse)
		}
		return 0
	case ToolResult:
		// Surface replacements (e.g. compaction rewrites) are durable edits,
		// not fresh tool results — exempt, matching ValidateRelations.
		if ev.SurfaceOp != nil && ev.SurfaceOp.Op == "replace" {
			return 0
		}
		if f, ok := ev.Data.(ToolResultPayload); ok {
			// Synthetic tool-not-started results (fail-closed denials before
			// execution) carry the error code but no real prior tool/call —
			// exempt, matching ValidateRelations.
			if f.IsError && f.ToolUseID == "" {
				return 0
			}
		}
		return -1
	default:
		return 0
	}
}

// pairing is the DSH surface fold: the ordered surface node seqs together with
// the tool-pairing balance at every cut position.
type pairing struct {
	surfaceSeqs []uint64 // seqs of surface nodes in append (projection) order
	cutBalanced []bool   // cutBalanced[i] = balance at the cut BEFORE node i (i in [0, len])
}

// computePairing folds the surface of a snapshot. A result with no matching
// open call (beyond the exempt cases) marks the surface corrupt; from that
// point every cut is reported unbalanced (fail-safe: a corrupt surface never
// authorizes a cut). Shadowed events (replaced by a subsequent replace surface op)
// are excluded from the active surface.
func computePairing(events []Event) pairing {
	shadowed := make(map[uint64]bool)
	for _, ev := range events {
		if ev.SurfaceOp != nil && ev.SurfaceOp.Op == "replace" {
			if ev.SurfaceOp.Start > 0 && ev.SurfaceOp.End >= ev.SurfaceOp.Start {
				for s := ev.SurfaceOp.Start; s <= ev.SurfaceOp.End; s++ {
					shadowed[s] = true
				}
			}
			for _, s := range ev.SourceEventSeqs {
				shadowed[s] = true
			}
		}
	}

	p := pairing{cutBalanced: []bool{true}}
	inProgress := 0
	corrupt := false
	for _, ev := range events {
		if !ev.Type.IsSurfaceEligible() || shadowed[ev.Seq] {
			continue
		}
		if !corrupt {
			inProgress += surfaceDelta(ev)
			if inProgress < 0 {
				corrupt = true
			}
		}
		p.surfaceSeqs = append(p.surfaceSeqs, ev.Seq)
		p.cutBalanced = append(p.cutBalanced, !corrupt && inProgress == 0)
	}
	return p
}

func (p pairing) indexOf(seq uint64) (int, bool) {
	for i, s := range p.surfaceSeqs {
		if s == seq {
			return i, true
		}
	}
	return 0, false
}

// ToolPairingBalancedAt reports whether the cut immediately BEFORE the surface
// event at seq is tool-pairing balanced: no open tool.call lacks its matching
// tool.result across the edge. Mirrors DSH's toolPairingBalancedBefore. A seq
// that is not a surface node (or absent from the log) reports false.
func ToolPairingBalancedAt(log *Log, seq uint64) bool {
	if log == nil {
		return false
	}
	p := computePairing(log.Snapshot())
	i, ok := p.indexOf(seq)
	if !ok {
		return false
	}
	return p.cutBalanced[i]
}

// ToolPairingBalancedAfter reports whether the cut immediately AFTER the
// surface event at seq is tool-pairing balanced. Mirrors DSH's
// toolPairingBalancedAfter. A seq that is not a surface node (or absent from
// the log) reports false.
func ToolPairingBalancedAfter(log *Log, seq uint64) bool {
	if log == nil {
		return false
	}
	p := computePairing(log.Snapshot())
	i, ok := p.indexOf(seq)
	if !ok {
		return false
	}
	return p.cutBalanced[i+1]
}

// SurfaceSeqAt returns the seq of the index-th surface node (0-based, in
// append order — the same order ProjectMessages emits). Used to map a
// transcript cut index onto the journal for boundary validation. Returns
// false when the index is out of range.
func SurfaceSeqAt(log *Log, index int) (uint64, bool) {
	if log == nil || index < 0 {
		return 0, false
	}
	p := computePairing(log.Snapshot())
	if index < len(p.surfaceSeqs) {
		return p.surfaceSeqs[index], true
	}
	return 0, false
}

// SurfaceCount returns the number of model-visible surface nodes in the log.
func SurfaceCount(log *Log) int {
	if log == nil {
		return 0
	}
	n := 0
	for _, ev := range log.Snapshot() {
		if ev.Type.IsSurfaceEligible() {
			n++
		}
	}
	return n
}

// SafeCompactionRange reports whether replacing the surface range [start,end]
// (inclusive, by seq) with a summary node is legal, returning a reason when it
// is not. A range is legal only when:
//   - both endpoints are surface nodes and start <= end;
//   - the cut before start is tool-pairing balanced (no unanswered tool.call
//     crosses the leading edge);
//   - the cut after end is tool-pairing balanced (no unanswered tool.call
//     crosses the trailing edge);
//   - the range does not shadow the opening marker of the turn bracket that is
//     still live (open at the log tail). A completed bracket — regardless of
//     where its closing marker sits — is historical and harmless, because
//     shadowed events remain in the raw log for deterministic replay.
func SafeCompactionRange(log *Log, start, end uint64) (bool, string) {
	if log == nil {
		return false, "journal unavailable"
	}
	if start > end {
		return false, "start seq after end seq"
	}
	p := computePairing(log.Snapshot())
	startIdx, okStart := p.indexOf(start)
	if !okStart {
		return false, fmt.Sprintf("seq %d is not a surface node", start)
	}
	endIdx, okEnd := p.indexOf(end)
	if !okEnd {
		return false, fmt.Sprintf("seq %d is not a surface node", end)
	}
	if !p.cutBalanced[startIdx] {
		return false, "cut before start is not tool-pairing balanced"
	}
	if !p.cutBalanced[endIdx+1] {
		return false, "cut after end is not tool-pairing balanced"
	}
	if splitsLiveTurn(log.Snapshot(), start, end) {
		return false, "range shadows the start of a live turn bracket"
	}
	return true, ""
}

// splitsLiveTurn reports whether the range shadows the turn/start marker of
// the bracket still open at the log tail — the live turn that continues to
// produce events after the cut. turn.end markers are appended after the
// turn's final surface node (at the start of the next turn loop), so a
// completed bracket's closing marker naturally lands outside a range that
// ends at a turn boundary; only the bracket that is still open can be
// "live". A turn/start that precedes the range (e.g. turn 1 of a fresh
// session, appended before the first surface node) is not in the shadowed
// span and does not count.
func splitsLiveTurn(events []Event, start, end uint64) bool {
	open := false
	var openStart uint64
	for _, ev := range events {
		switch ev.Type {
		case TurnStart:
			open, openStart = true, ev.Seq
		case TurnEnd:
			open = false
		}
	}
	return open && openStart >= start && openStart <= end
}

// HasOrphanedCompaction reports whether the log contains a compaction.start
// with no matching compaction.end — a crash between start and end leaves such
// an orphaned bracket. Per the DSH lock-bracket durability pattern, an
// orphaned bracket blocks subsequent compaction until adoption repairs it.
// Ends without a prior start are ignored (foreign/historical records cannot
// orphan a start).
func HasOrphanedCompaction(log *Log) bool {
	if log == nil {
		return false
	}
	var openIDs []string
	for _, ev := range log.Snapshot() {
		switch ev.Type {
		case CompactionStart:
			id := ""
			if f, ok := ev.Data.(CompactionStartFact); ok {
				id = f.CompactionID
			}
			openIDs = append(openIDs, id)
		case CompactionEnd:
			if len(openIDs) == 0 {
				continue
			}
			id := ""
			if f, ok := ev.Data.(CompactionEndFact); ok {
				id = f.CompactionID
			}
			if id == "" {
				// No correlation ID: pair by bracket order (most recent open).
				openIDs = openIDs[:len(openIDs)-1]
				continue
			}
			for i := len(openIDs) - 1; i >= 0; i-- {
				if openIDs[i] == id {
					openIDs = append(openIDs[:i], openIDs[i+1:]...)
					break
				}
			}
		}
	}
	return len(openIDs) > 0
}
