package engine

import "github.com/GrayCodeAI/hawk/internal/engine/docs"

type DocGenerator = docs.DocGenerator
type DocSection = docs.DocSection
type ProjectDoc = docs.ProjectDoc
type PackageDoc = docs.PackageDoc
type FunctionDoc = docs.FunctionDoc
type ParamDoc = docs.ParamDoc
type TypeDoc = docs.TypeDoc
type FieldDoc = docs.FieldDoc
type DocSource = docs.DocSource
type DocResult = docs.DocResult
type ExternalDocs = docs.ExternalDocs
type DocUpdate = docs.DocUpdate
type DocUpdater = docs.DocUpdater

var NewDocGenerator = docs.NewDocGenerator
var NewExternalDocs = docs.NewExternalDocs
var NewDocUpdater = docs.NewDocUpdater
var RenderMarkdown = docs.RenderMarkdown
var RenderHTML = docs.RenderHTML
var GenerateREADME = docs.GenerateREADME
