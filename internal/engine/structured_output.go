package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/GrayCodeAI/hawk/internal/types"
)

// structured_output.go implements JSON-schema-constrained responses (Task A).
//
// The session-level option Session.OutputSchema is plumbed into eyrie's
// ChatOptions.ResponseFormat (json_schema) on the streaming path (see
// stream.go). On the non-streaming ChatStructured path below, the response is
// validated against the schema and, on mismatch, retried exactly once with a
// corrective instruction appended.
//
// Validation is intentionally dependency-free: it covers valid-JSON parsing
// plus the most common JSON Schema constraints (type, required, properties
// types). It is a best-effort guard, not a full Draft-2020 validator.

// SchemaError describes why a response failed schema validation.
type SchemaError struct {
	Reason string
}

func (e *SchemaError) Error() string { return "schema validation failed: " + e.Reason }

// ChatStructured sends msgs to the model requesting a JSON-schema-constrained
// response. The schema is plumbed into ResponseFormat. The result is validated
// against schema; on mismatch it retries exactly once with a corrective
// message, then returns the (still-validated-or-not) response along with any
// validation error from the final attempt.
//
// schema must be a JSON Schema document (as a string). A blank schema disables
// validation and behaves like an ordinary Chat call.
func (s *Session) ChatStructured(ctx context.Context, msgs []types.EyrieMessage, opts types.ChatOptions, schema string) (*types.EyrieResponse, error) {
	if schema != "" {
		opts.ResponseFormat = &types.ResponseFormat{Type: "json_schema", Schema: schema}
	}

	resp, err := s.client.Chat(ctx, msgs, opts)
	if err != nil {
		return resp, err
	}
	if schema == "" {
		return resp, nil
	}

	if vErr := ValidateAgainstSchema(responseText(resp), schema); vErr == nil {
		return resp, nil
	} else {
		// Retry once with a corrective instruction.
		retryMsgs := append([]types.EyrieMessage(nil), msgs...)
		retryMsgs = append(retryMsgs,
			types.EyrieMessage{Role: "assistant", Content: responseText(resp)},
			types.EyrieMessage{Role: "user", Content: fmt.Sprintf(
				"Your previous response did not conform to the required JSON schema (%s). "+
					"Respond again with ONLY valid JSON that matches the schema. Schema:\n%s",
				vErr.Error(), schema)},
		)
		retryResp, retryErr := s.client.Chat(ctx, retryMsgs, opts)
		if retryErr != nil {
			return resp, retryErr
		}
		if vErr2 := ValidateAgainstSchema(responseText(retryResp), schema); vErr2 != nil {
			return retryResp, vErr2
		}
		return retryResp, nil
	}
}

// responseText returns the textual content of a response, tolerating nil.
func responseText(resp *types.EyrieResponse) string {
	if resp == nil {
		return ""
	}
	return resp.Content
}

// ValidateAgainstSchema checks that content is JSON conforming to the most
// common constraints expressed by schema. Supported: type (object/array/
// string/number/integer/boolean/null), required (array of property names), and
// per-property type checks under properties. Unknown keywords are ignored.
func ValidateAgainstSchema(content, schema string) error {
	content = extractJSON(content)
	if strings.TrimSpace(content) == "" {
		return &SchemaError{Reason: "empty response"}
	}

	var doc interface{}
	if err := json.Unmarshal([]byte(content), &doc); err != nil {
		return &SchemaError{Reason: "response is not valid JSON: " + err.Error()}
	}

	var sch map[string]interface{}
	if err := json.Unmarshal([]byte(schema), &sch); err != nil {
		// An unparseable schema cannot constrain anything; treat valid JSON as ok.
		return nil
	}
	return validateValue(doc, sch, "$")
}

func validateValue(v interface{}, sch map[string]interface{}, path string) error {
	if t, ok := sch["type"].(string); ok {
		if err := checkType(v, t, path); err != nil {
			return err
		}
	}

	obj, isObj := v.(map[string]interface{})
	if isObj {
		if reqRaw, ok := sch["required"].([]interface{}); ok {
			for _, r := range reqRaw {
				name, _ := r.(string)
				if name == "" {
					continue
				}
				if _, present := obj[name]; !present {
					return &SchemaError{Reason: fmt.Sprintf("%s: missing required property %q", path, name)}
				}
			}
		}
		if props, ok := sch["properties"].(map[string]interface{}); ok {
			for name, raw := range props {
				propSch, ok := raw.(map[string]interface{})
				if !ok {
					continue
				}
				if val, present := obj[name]; present {
					if err := validateValue(val, propSch, path+"."+name); err != nil {
						return err
					}
				}
			}
		}
	}

	if arr, isArr := v.([]interface{}); isArr {
		if items, ok := sch["items"].(map[string]interface{}); ok {
			for i, el := range arr {
				if err := validateValue(el, items, fmt.Sprintf("%s[%d]", path, i)); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func checkType(v interface{}, t, path string) error {
	ok := false
	switch t {
	case "object":
		_, ok = v.(map[string]interface{})
	case "array":
		_, ok = v.([]interface{})
	case "string":
		_, ok = v.(string)
	case "boolean":
		_, ok = v.(bool)
	case "null":
		ok = v == nil
	case "number":
		_, ok = v.(float64)
	case "integer":
		if f, isF := v.(float64); isF {
			ok = f == float64(int64(f))
		}
	default:
		ok = true // unknown type keyword: don't reject
	}
	if !ok {
		return &SchemaError{Reason: fmt.Sprintf("%s: expected type %q", path, t)}
	}
	return nil
}

// extractJSON pulls a JSON value out of content that may be wrapped in markdown
// code fences or surrounded by prose. It returns the original string when no
// fenced/embedded object is found.
func extractJSON(content string) string {
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "```") {
		// Strip a leading ```json or ``` fence and the trailing fence.
		if nl := strings.IndexByte(content, '\n'); nl >= 0 {
			content = content[nl+1:]
		}
		if end := strings.LastIndex(content, "```"); end >= 0 {
			content = content[:end]
		}
		content = strings.TrimSpace(content)
	}
	return content
}
