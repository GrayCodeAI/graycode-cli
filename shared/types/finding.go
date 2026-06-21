// Package types is a deprecated compatibility layer for shared Hawk ecosystem types.
// New cross-repo contracts belong in github.com/GrayCodeAI/hawk-core-contracts/types.
package types

import contracts "github.com/GrayCodeAI/hawk-core-contracts/types"

// Deprecated: use github.com/GrayCodeAI/hawk-core-contracts/types.Finding.
// Finding represents a unified code-analysis concern sourced from sight, inspect, or manual review.
type Finding = contracts.Finding

// Deprecated: use github.com/GrayCodeAI/hawk-core-contracts/types.FindingSlice.
// FindingSlice is a sortable slice of Findings.
type FindingSlice = contracts.FindingSlice

// Deprecated: use github.com/GrayCodeAI/hawk-core-contracts/types.FindingSummary.
// FindingSummary provides aggregate counts over a set of findings.
type FindingSummary = contracts.FindingSummary

// FindingFromSight constructs a Finding from a sight (AST/static-analysis) result.
// Deprecated: use github.com/GrayCodeAI/hawk-core-contracts/types.FindingFromSight.
var FindingFromSight = contracts.FindingFromSight

// FindingFromInspect constructs a Finding from an inspect (linting/analysis) result.
// Deprecated: use github.com/GrayCodeAI/hawk-core-contracts/types.FindingFromInspect.
var FindingFromInspect = contracts.FindingFromInspect
