package engine

import "github.com/GrayCodeAI/hawk/internal/engine/docs"

type (
	DocGenerator = docs.DocGenerator
	DocSection   = docs.DocSection
	ProjectDoc   = docs.ProjectDoc
	PackageDoc   = docs.PackageDoc
	FunctionDoc  = docs.FunctionDoc
	ParamDoc     = docs.ParamDoc
	TypeDoc      = docs.TypeDoc
	FieldDoc     = docs.FieldDoc
	DocSource    = docs.DocSource
	DocResult    = docs.DocResult
	ExternalDocs = docs.ExternalDocs
	DocUpdate    = docs.DocUpdate
	DocUpdater   = docs.DocUpdater
)

var (
	NewDocGenerator = docs.NewDocGenerator
	NewExternalDocs = docs.NewExternalDocs
	NewDocUpdater   = docs.NewDocUpdater
	RenderMarkdown  = docs.RenderMarkdown
	RenderHTML      = docs.RenderHTML
	GenerateREADME  = docs.GenerateREADME
)
