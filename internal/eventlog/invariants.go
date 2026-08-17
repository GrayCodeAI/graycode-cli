package eventlog

import (
	"errors"
	"fmt"
)

// ErrUnknownType is returned when a durable record carries an event kind this
// build does not know. The stance is fail-loud: a record the accountable
// build cannot project must not be silently reinterpreted.
var ErrUnknownType = errors.New("eventlog: unknown event type")

// Validate checks the snapshot for invariant violations before it is trusted:
// every event kind must be known, the format version (read from the SessionMeta
// head event) must match SessionFormatVersion, surface-eligible events must
// carry a SurfaceOp marker (and non-surface events must not), and sequence numbers
// must be strictly increasing (the record is append-only). It returns the first
// violation.
func Validate(events []Event) error {
	var prev uint64
	for i, ev := range events {
		if !ev.Type.Known() {
			return fmt.Errorf("%w: %q at index %d", ErrUnknownType, ev.Type, i)
		}
		if ev.Type == SessionMeta {
			if meta, ok := ev.Data.(Meta); ok {
				if meta.FormatVersion != 0 && meta.FormatVersion != SessionFormatVersion {
					return fmt.Errorf("%w: log version %d, this build reads %d (session event at index %d)",
						ErrForeignFormatVersion, meta.FormatVersion, SessionFormatVersion, i)
				}
			}
		}
		// DSH invariant: surface-eligible events must carry a surfaceOp marker;
		// non-surface events must not. A log without markers is treated as
		// all-append (backward compatible with version-1 logs written before
		// this invariant was enforced).
		if ev.Type.IsSurfaceEligible() {
			if ev.SurfaceOp != nil && ev.SurfaceOp.Op != "append" && ev.SurfaceOp.Op != "replace" {
				return fmt.Errorf("eventlog: invalid surfaceOp %q at index %d (must be append or replace)", ev.SurfaceOp.Op, i)
			}
		} else if ev.SurfaceOp != nil {
			return fmt.Errorf("eventlog: non-surface event %q at index %d carries a surfaceOp marker", ev.Type, i)
		}
		if i > 0 && ev.Seq <= prev {
			return fmt.Errorf("eventlog: sequence not monotonic at index %d: seq %d after %d", i, ev.Seq, prev)
		}
		prev = ev.Seq
	}
	return nil
}
