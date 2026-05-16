// Package docs is the Stage-1 namespace for documentation generation,
// external docs fetching, and doc updating. See ../REFACTOR_PLAN.md.
package docs

import "github.com/GrayCodeAI/hawk/engine"

type DocGenerator = engine.DocGenerator
type DocSection = engine.DocSection
type ProjectDoc = engine.ProjectDoc
type PackageDoc = engine.PackageDoc
type FunctionDoc = engine.FunctionDoc
type ParamDoc = engine.ParamDoc
type TypeDoc = engine.TypeDoc
type FieldDoc = engine.FieldDoc
type DocSource = engine.DocSource
type DocResult = engine.DocResult
type ExternalDocs = engine.ExternalDocs
type DocUpdate = engine.DocUpdate
type DocUpdater = engine.DocUpdater

func NewDocGenerator(projectDir string) *DocGenerator { return engine.NewDocGenerator(projectDir) }
func NewExternalDocs() *ExternalDocs                  { return engine.NewExternalDocs() }
func NewDocUpdater() *DocUpdater                      { return engine.NewDocUpdater() }
func RenderMarkdown(doc *ProjectDoc) string           { return engine.RenderMarkdown(doc) }
