package engine

import "github.com/GrayCodeAI/hawk/internal/engine/scaffold"

type (
	Template         = scaffold.Template
	TemplateFile     = scaffold.TemplateFile
	TemplateVariable = scaffold.TemplateVariable
	Scaffolder       = scaffold.Scaffolder
	Recipe           = scaffold.Recipe
	RecipeRegistry   = scaffold.RecipeRegistry
	PromptPattern    = scaffold.PromptPattern
	PatternLibrary   = scaffold.PatternLibrary
	Skill            = scaffold.Skill
	SkillStep        = scaffold.SkillStep
	SkillResult      = scaffold.SkillResult
	SkillRegistry    = scaffold.SkillRegistry
	FewShotStore     = scaffold.FewShotStore
	FewShotExample   = scaffold.FewShotExample
)

func NewScaffolder() *Scaffolder                   { return scaffold.NewScaffolder() }
func NewRecipeRegistry(dir string) *RecipeRegistry { return scaffold.NewRecipeRegistry(dir) }
func NewPatternLibrary(dir string) *PatternLibrary { return scaffold.NewPatternLibrary(dir) }
func NewSkillRegistry(dir string) *SkillRegistry   { return scaffold.NewSkillRegistry(dir) }
func NewFewShotStore() *FewShotStore               { return scaffold.NewFewShotStore() }
func FormatPattern(p *PromptPattern) string        { return scaffold.FormatPattern(p) }
func FormatSkill(s *Skill) string                  { return scaffold.FormatSkill(s) }
