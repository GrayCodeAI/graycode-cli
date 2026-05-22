package engine

import "github.com/GrayCodeAI/hawk/internal/engine/scaffold"

type Template = scaffold.Template
type TemplateFile = scaffold.TemplateFile
type TemplateVariable = scaffold.TemplateVariable
type Scaffolder = scaffold.Scaffolder
type Recipe = scaffold.Recipe
type RecipeRegistry = scaffold.RecipeRegistry
type PromptPattern = scaffold.PromptPattern
type PatternLibrary = scaffold.PatternLibrary
type Skill = scaffold.Skill
type SkillStep = scaffold.SkillStep
type SkillResult = scaffold.SkillResult
type SkillRegistry = scaffold.SkillRegistry
type FewShotStore = scaffold.FewShotStore
type FewShotExample = scaffold.FewShotExample

func NewScaffolder() *Scaffolder                   { return scaffold.NewScaffolder() }
func NewRecipeRegistry(dir string) *RecipeRegistry { return scaffold.NewRecipeRegistry(dir) }
func NewPatternLibrary(dir string) *PatternLibrary { return scaffold.NewPatternLibrary(dir) }
func NewSkillRegistry(dir string) *SkillRegistry   { return scaffold.NewSkillRegistry(dir) }
func NewFewShotStore() *FewShotStore               { return scaffold.NewFewShotStore() }
func FormatPattern(p *PromptPattern) string        { return scaffold.FormatPattern(p) }
func FormatSkill(s *Skill) string                  { return scaffold.FormatSkill(s) }
