// Package docs is the Stage-1 namespace for documentation generation,
// external docs fetching, and doc updating. See ../REFACTOR_PLAN.md.
package docs

import "github.com/GrayCodeAI/hawk/internal/engine"

type (
	DocGenerator = engine.DocGenerator
	DocSection   = engine.DocSection
	ProjectDoc   = engine.ProjectDoc
	PackageDoc   = engine.PackageDoc
	FunctionDoc  = engine.FunctionDoc
	ParamDoc     = engine.ParamDoc
	TypeDoc      = engine.TypeDoc
	FieldDoc     = engine.FieldDoc
	DocSource    = engine.DocSource
	DocResult    = engine.DocResult
	ExternalDocs = engine.ExternalDocs
	DocUpdate    = engine.DocUpdate
	DocUpdater   = engine.DocUpdater
)

func NewDocGenerator(projectDir string) *DocGenerator { return engine.NewDocGenerator(projectDir) }
func NewExternalDocs() *ExternalDocs                  { return engine.NewExternalDocs() }
func NewDocUpdater() *DocUpdater                      { return engine.NewDocUpdater() }
func RenderMarkdown(doc *ProjectDoc) string           { return engine.RenderMarkdown(doc) }
