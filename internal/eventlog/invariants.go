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
// every event kind must be known, and sequence numbers must be strictly
// increasing (the record is append-only). It returns the first violation.
func Validate(events []Event) error {
	var prev uint64
	for i, ev := range events {
		if !ev.Type.Known() {
			return fmt.Errorf("%w: %q at index %d", ErrUnknownType, ev.Type, i)
		}
		if i > 0 && ev.Seq <= prev {
			return fmt.Errorf("eventlog: sequence not monotonic at index %d: seq %d after %d", i, ev.Seq, prev)
		}
		prev = ev.Seq
	}
	return nil
}
