package eventlog

import (
	"regexp"
	"strings"
	"unicode"
)

var (
	titleWhitespaceRE = regexp.MustCompile(`\s+`)
	titlePunctRE      = regexp.MustCompile(`[^\p{L}\p{N}\p{P}\p{S}\s]`)
)

// TitleFromMessages is the deterministic, provider-free session title projection.
// It is a pure function of the model-visible events in the log — no LLM call and
// no ambient state — so the same record always yields the same title. User intent
// wins; the assistant's first reply is the fallback; an empty log is "Untitled
// Session".
func TitleFromMessages(msgs []Message) string {
	var candidate string
	for _, m := range msgs {
		if m.Role != "user" {
			continue
		}
		if c := titleFromContent(m.Content); c != "" {
			candidate = c
			break
		}
	}
	if candidate == "" {
		for _, m := range msgs {
			if c := titleFromContent(m.Content); c != "" {
				candidate = c
				break
			}
		}
	}
	if candidate == "" {
		return "Untitled Session"
	}
	return truncateTitle(candidate)
}

func titleFromContent(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	// Use the first semantic line rather than the whole multi-line prompt.
	if idx := strings.IndexByte(content, '\n'); idx >= 0 {
		content = content[:idx]
	}
	// Cut at sentence boundaries for long prompts; prefer whole-sentence titles.
	for _, cut := range []string{". ", "! ", "? ", ".\n", "!\n", "?\n"} {
		if idx := strings.Index(content, cut); idx > 8 {
			candidate := strings.TrimSpace(content[:idx])
			if candidate != "" && len(candidate) >= 4 {
				return candidate
			}
		}
	}
	return strings.TrimSpace(content)
}

func truncateTitle(s string) string {
	s = titlePunctRE.ReplaceAllString(s, "")
	s = titleWhitespaceRE.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)
	if s == "" {
		return "Untitled Session"
	}
	runes := []rune(s)
	if len(runes) <= 72 {
		return s
	}
	cut := string(runes[:69]) + "..."
	return strings.TrimRightFunc(cut, unicode.IsSpace) // trim possible trailing space before ellipsis
}
