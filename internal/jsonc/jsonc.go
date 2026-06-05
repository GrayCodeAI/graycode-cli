// Package jsonc is a minimal JSON-with-Comments (JSONC) parser.
//
// It accepts the same input as encoding/json.Unmarshal, plus:
//
//   - // line comments to end of line
//   - /* block comments */
//   - trailing commas in objects and arrays
//
// Output is identical to standard JSON parse for the same input
// without comments. Comment text is discarded; the result is a
// normal Go interface{} (or other target type) populated as if
// the comments were never there.
//
// GrayCode native implementation.
// bin/lib/settings.js (parseJSONC). Ported to native Go.
package jsonc

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// ErrInvalidJSON is returned when the input cannot be parsed even
// after comment/trailing-comma stripping.
var ErrInvalidJSON = errors.New("jsonc: invalid input")

// Strip removes JSONC comments and trailing commas from src and
// returns a byte slice suitable for encoding/json.Unmarshal.
func Strip(src []byte) ([]byte, error) {
	out := make([]byte, 0, len(src))
	i := 0
	for i < len(src) {
		// Block comment /* ... */
		if i+1 < len(src) && src[i] == '/' && src[i+1] == '*' {
			end := bytes.Index(src[i+2:], []byte("*/"))
			if end < 0 {
				return nil, fmt.Errorf("%w: unterminated block comment", ErrInvalidJSON)
			}
			i += 2 + end + 2
			continue
		}
		// Line comment // ... \n
		if i+1 < len(src) && src[i] == '/' && src[i+1] == '/' {
			end := bytes.IndexByte(src[i:], '\n')
			if end < 0 {
				// Comment goes to EOF
				i = len(src)
				continue
			}
			i += end
			continue
		}
		// String literal: copy verbatim (don't strip inside)
		if src[i] == '"' {
			j := i + 1
			for j < len(src) {
				if src[j] == '\\' && j+1 < len(src) {
					j += 2
					continue
				}
				if src[j] == '"' {
					j++
					break
				}
				j++
			}
			out = append(out, src[i:j]...)
			i = j
			continue
		}
		out = append(out, src[i])
		i++
	}
	// Strip trailing commas: ",]" or ",}" -> "] or "}"
	out = stripTrailingCommas(out)
	return out, nil
}

// stripTrailingCommas removes ",]" and ",}" sequences that are not
// inside strings. Operates on a fresh byte slice.
func stripTrailingCommas(src []byte) []byte {
	out := make([]byte, 0, len(src))
	inStr := false
	for i := 0; i < len(src); i++ {
		c := src[i]
		if c == '"' && (i == 0 || src[i-1] != '\\') {
			inStr = !inStr
		}
		if !inStr && c == ',' {
			// Look ahead for the next non-whitespace byte
			j := i + 1
			for j < len(src) && (src[j] == ' ' || src[j] == '\t' || src[j] == '\n' || src[j] == '\r') {
				j++
			}
			if j < len(src) && (src[j] == ']' || src[j] == '}') {
				// Skip the comma
				continue
			}
		}
		out = append(out, c)
	}
	return out
}

// Unmarshal is a drop-in replacement for json.Unmarshal that
// accepts JSONC input (comments + trailing commas).
func Unmarshal(data []byte, v interface{}) error {
	cleaned, err := Strip(data)
	if err != nil {
		return err
	}
	return json.Unmarshal(cleaned, v)
}

// MarshalIndent is a thin wrapper over json.MarshalIndent kept here
// for API symmetry. The output is plain JSON (no comments).
func MarshalIndent(v interface{}, prefix, indent string) ([]byte, error) {
	return json.MarshalIndent(v, prefix, indent)
}

// Valid reports whether data is valid JSONC.
func Valid(data []byte) bool {
	_, err := Strip(data)
	if err != nil {
		return false
	}
	return json.Valid(stripTrailingCommas(mustStripComments(data)))
}

// mustStripComments is a helper for Valid that ignores errors
// (since we're only checking validity).
func mustStripComments(data []byte) []byte {
	out, _ := Strip(data)
	return out
}

// PrettyError returns a human-friendly error message for a JSONC
// parse failure. It is intended for display in CLI output.
func PrettyError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	msg = strings.TrimPrefix(msg, "jsonc: ")
	return msg
}

// Compile-time guard: io is imported for future streaming parsers.
var _ = io.EOF
