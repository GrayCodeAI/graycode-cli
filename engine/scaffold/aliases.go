// Package scaffold is the Stage-1 namespace for scaffolding, recipes,
// patterns, skills, and few-shot types. See ../REFACTOR_PLAN.md.
package scaffold

import "github.com/GrayCodeAI/hawk/engine"

type Template = engine.Template
type TemplateFile = engine.TemplateFile
type TemplateVariable = engine.TemplateVariable
type Scaffolder = engine.Scaffolder
type Recipe = engine.Recipe
type RecipeRegistry = engine.RecipeRegistry
type PromptPattern = engine.PromptPattern
type PatternLibrary = engine.PatternLibrary
type Skill = engine.Skill
type SkillStep = engine.SkillStep
type SkillResult = engine.SkillResult
type SkillRegistry = engine.SkillRegistry
type FewShotStore = engine.FewShotStore
type FewShotExample = engine.FewShotExample

func NewScaffolder() *Scaffolder              { return engine.NewScaffolder() }
func NewRecipeRegistry(dir string) *RecipeRegistry { return engine.NewRecipeRegistry(dir) }
func NewPatternLibrary(dir string) *PatternLibrary { return engine.NewPatternLibrary(dir) }
func NewSkillRegistry(dir string) *SkillRegistry   { return engine.NewSkillRegistry(dir) }
func NewFewShotStore() *FewShotStore               { return engine.NewFewShotStore() }
func FormatPattern(p *PromptPattern) string        { return engine.FormatPattern(p) }
func FormatSkill(s *Skill) string                  { return engine.FormatSkill(s) }
