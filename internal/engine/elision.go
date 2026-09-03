package engine

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/GrayCodeAI/graycode-cli/internal/engine/token"
)

// elisionNotice computes a verified-facts suffix for a truncation marker from
// the content being dropped. Facts state only what holds across every elided
// unit (see tok invariants): constants, exact enumerations, numeric ranges,
// distinct-count coverage. Anything uncertain is omitted — a bare count is
// better than a wrong fact, and a wrong fact reads as complete.
//
// JSON arrays of records get field-level facts; log-shaped text gets level
// distribution; anything else falls back to the line count.
func elisionNotice(dropped string) string {
	trimmed := strings.TrimSpace(dropped)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "[") {
		return jsonRecordsFacts(trimmed)
	}
	// A structural cut inside a JSON array leaves a fragment of whole records
	// separated by commas, optionally ending with the array's closing bracket
	// ("{…},{…}]" or "{…},{…},"). Strip both artifacts, normalize to an
	// array, and retry once; anything unparseable still falls back.
	if strings.HasPrefix(trimmed, "{") {
		frag := strings.TrimRight(trimmed, " \t\r\n")
		frag = strings.TrimSuffix(frag, "]")
		frag = strings.TrimRight(frag, ", \t\r\n")
		if facts := jsonRecordsFacts("[" + frag + "]"); facts != "" {
			return facts
		}
	}
	lines := splitNonEmptyLines(trimmed)
	if len(lines) >= 3 {
		if facts := token.LogInvariants(lines); facts != "" {
			return fmt.Sprintf("%d lines elided: %s", len(lines), facts)
		}
		return fmt.Sprintf("%d lines elided", len(lines))
	}
	return ""
}

func jsonRecordsFacts(arrayText string) string {
	var records []json.RawMessage
	if json.Unmarshal([]byte(arrayText), &records) != nil || len(records) == 0 {
		return ""
	}
	if facts := token.JSONInvariants(records); facts != "" {
		return fmt.Sprintf("%d records elided: %s", len(records), facts)
	}
	return fmt.Sprintf("%d records elided", len(records))
}

func splitNonEmptyLines(s string) []string {
	raw := strings.Split(s, "\n")
	out := make([]string, 0, len(raw))
	for _, l := range raw {
		if t := strings.TrimSpace(l); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// appendElisionMarker attaches an invariant-bearing marker to kept output.
func appendElisionMarker(kept, dropped string) string {
	if n := elisionNotice(dropped); n != "" {
		return kept + "\n... [" + n + "]"
	}
	return kept + "\n... (truncated)"
}
