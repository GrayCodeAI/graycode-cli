package config

import (
	"encoding/json"
	"testing"
)

func FuzzValidateSettings(f *testing.F) {
	f.Add([]byte(`{"model":"gpt-4","provider":"openai"}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"model":"","provider":""}`))
	f.Add([]byte(`{"max_tokens":-1}`))
	f.Add([]byte(`null`))
	f.Add([]byte(`not json`))
	f.Add([]byte(`{"model":"a]b[c{d","temperature":99.9}`))

	f.Fuzz(func(t *testing.T, data []byte) {
		var s Settings
		if json.Unmarshal(data, &s) != nil {
			return
		}
		// Should never panic regardless of settings content
		result := ValidateSettings(s)
		_ = result.Error()
	})
}
