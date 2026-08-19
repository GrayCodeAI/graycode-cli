package plugin

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// ComputeSkillDigest computes a deterministic SHA-256 hex digest over the
// sorted (name, description) pairs of model-invocable skills.
func ComputeSkillDigest(skills []SkillEntry) string {
	var invocable []SkillEntry
	for _, s := range skills {
		if s.Invocation.IsModelInvocable() {
			invocable = append(invocable, s)
		}
	}
	if len(invocable) == 0 {
		return "empty"
	}

	sort.Slice(invocable, func(i, j int) bool {
		return invocable[i].Name < invocable[j].Name
	})

	h := sha256.New()
	for _, s := range invocable {
		h.Write([]byte(fmt.Sprintf("%s:%s\n", s.Name, s.Description)))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// RenderSkillCatalogMessage renders the canonical context statement for the skill catalog.
// If skills is empty, it renders an explicit tombstone retiring previously known skills.
func RenderSkillCatalogMessage(skills []SkillEntry, digest string) string {
	var invocable []SkillEntry
	for _, s := range skills {
		if s.Invocation.IsModelInvocable() {
			invocable = append(invocable, s)
		}
	}

	if len(invocable) == 0 {
		return fmt.Sprintf("Available skills (digest: %s):\n(No active skills available. Any previous skills are retired.)", digest)
	}

	sort.Slice(invocable, func(i, j int) bool {
		return invocable[i].Name < invocable[j].Name
	})

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Available skills (digest: %s):\n", digest))
	for _, s := range invocable {
		if s.Description != "" {
			b.WriteString(fmt.Sprintf("- %s: %s\n", s.Name, s.Description))
		} else {
			b.WriteString(fmt.Sprintf("- %s\n", s.Name))
		}
	}
	return strings.TrimRight(b.String(), "\n")
}
