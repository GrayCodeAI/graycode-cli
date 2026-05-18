// Package scaffold is the Stage-1 namespace for scaffolding, recipes,
// patterns, skills, and few-shot types. See ../REFACTOR_PLAN.md.
package scaffold

import "github.com/GrayCodeAI/hawk/internal/engine"

type (
	Template         = engine.Template
	TemplateFile     = engine.TemplateFile
	TemplateVariable = engine.TemplateVariable
	Scaffolder       = engine.Scaffolder
	Recipe           = engine.Recipe
	RecipeRegistry   = engine.RecipeRegistry
	PromptPattern    = engine.PromptPattern
	PatternLibrary   = engine.PatternLibrary
	Skill            = engine.Skill
	SkillStep        = engine.SkillStep
	SkillResult      = engine.SkillResult
	SkillRegistry    = engine.SkillRegistry
	FewShotStore     = engine.FewShotStore
	FewShotExample   = engine.FewShotExample
)

func NewScaffolder() *Scaffolder                   { return engine.NewScaffolder() }
func NewRecipeRegistry(dir string) *RecipeRegistry { return engine.NewRecipeRegistry(dir) }
func NewPatternLibrary(dir string) *PatternLibrary { return engine.NewPatternLibrary(dir) }
func NewSkillRegistry(dir string) *SkillRegistry   { return engine.NewSkillRegistry(dir) }
func NewFewShotStore() *FewShotStore               { return engine.NewFewShotStore() }
func FormatPattern(p *PromptPattern) string        { return engine.FormatPattern(p) }
func FormatSkill(s *Skill) string                  { return engine.FormatSkill(s) }
