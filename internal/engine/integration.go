package engine

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/GrayCodeAI/hawk/internal/types"
	"github.com/GrayCodeAI/hawk/internal/engine/ctxmgr"
	"github.com/GrayCodeAI/tok"
)

// ---------------------------------------------------------------------------
// Integration Pipeline — connects hawk's subsystems into working pipelines.
//
// The pipeline orchestrates pre-query, post-response, post-tool, and session
// end phases. Each phase delegates to the relevant subsystems, gathers their
// outputs, and returns a unified result.
// ---------------------------------------------------------------------------

// IntegrationPipeline holds references to all subsystems and orchestrates them
// through the request lifecycle.
type IntegrationPipeline struct {
	// Pre-query pipeline
	IntentClassifier *IntentClassifier
	ToolSelector     *ToolSelector
	ContextDecay     *ctxmgr.ContextDecay
	BudgetAllocator  *BudgetAllocator
	TokenPredictor   *TokenPredictor
	AdaptivePrompt   *SystemPromptBuilder

	// Post-response pipeline
	ResponseFormatter   *ResponseFormatter
	QualityScorer       *QualityScorer
	FileMentionDetector *FileMentionDetector

	// Post-tool pipeline
	LintLoop      *LintLoop
	TestLoop      *TestLoop
	StallDetector *StallDetector
	ErrorRecovery *ErrorRecovery

	// Security pipeline
	InjectionScanner *InjectionScanner
	OutputRedactor   *OutputRedactor

	// Learning pipeline
	ExperienceStore   *ExperienceStore
	KnowledgeBase     *KnowledgeBase
	FeedbackCollector *FeedbackCollector
	SelfAssessor      *SelfAssessor

	// Session management
	Timeline       *Timeline
	TokenReporter  *TokenReporter
	WorkspaceState *WorkspaceState
	CommandHistory *CommandHistory
	ResponseCache  *ResponseCache

	// Internal
	sessionStart time.Time
	mu           sync.RWMutex
}

// ---------------------------------------------------------------------------
// InjectionScanner — lightweight prompt-injection detection.
// ---------------------------------------------------------------------------

// ScanResult describes the outcome of an injection scan.
type ScanResult struct {
	IsRisky    bool
	RiskLevel  string // "none", "low", "medium", "high"
	Patterns   []string
	Suggestion string
}

// InjectionScanner detects potential prompt-injection attempts in user input.
type InjectionScanner struct {
	Patterns []injectionPattern
	mu       sync.RWMutex
}

type injectionPattern struct {
	Name    string
	Pattern string
	Level   string // "low", "medium", "high"
}

// NewInjectionScanner creates an InjectionScanner with built-in detection patterns.
func NewInjectionScanner() *InjectionScanner {
	return &InjectionScanner{
		Patterns: []injectionPattern{
			{Name: "system_override", Pattern: "ignore previous instructions", Level: "high"},
			{Name: "system_override2", Pattern: "ignore all prior instructions", Level: "high"},
			{Name: "role_hijack", Pattern: "you are now", Level: "medium"},
			{Name: "role_hijack2", Pattern: "act as if you are", Level: "medium"},
			{Name: "prompt_leak", Pattern: "print your system prompt", Level: "medium"},
			{Name: "prompt_leak2", Pattern: "show me your instructions", Level: "medium"},
			{Name: "prompt_leak3", Pattern: "reveal your prompt", Level: "medium"},
			{Name: "delimiter_attack", Pattern: "```system", Level: "high"},
			{Name: "delimiter_attack2", Pattern: "---system---", Level: "high"},
			{Name: "encoding_bypass", Pattern: "base64 decode", Level: "low"},
			{Name: "payload_inject", Pattern: "execute this secretly", Level: "high"},
			{Name: "sudo_mode", Pattern: "enter sudo mode", Level: "medium"},
			{Name: "developer_mode", Pattern: "developer mode enabled", Level: "medium"},
			{Name: "jailbreak_dan", Pattern: "DAN mode", Level: "high"},
		},
	}
}

// Scan checks the input for injection patterns and returns a risk assessment.
func (is *InjectionScanner) Scan(input string) *ScanResult {
	is.mu.RLock()
	defer is.mu.RUnlock()

	lower := strings.ToLower(input)
	result := &ScanResult{
		RiskLevel: "none",
	}

	levelPriority := map[string]int{"none": 0, "low": 1, "medium": 2, "high": 3}

	for _, p := range is.Patterns {
		if strings.Contains(lower, strings.ToLower(p.Pattern)) {
			result.IsRisky = true
			result.Patterns = append(result.Patterns, p.Name)
			if levelPriority[p.Level] > levelPriority[result.RiskLevel] {
				result.RiskLevel = p.Level
			}
		}
	}

	if result.IsRisky {
		result.Suggestion = fmt.Sprintf("Detected %d injection pattern(s) at %s risk level. Consider rejecting or sanitizing input.",
			len(result.Patterns), result.RiskLevel)
	}

	return result
}

// ---------------------------------------------------------------------------
// Result Types
// ---------------------------------------------------------------------------

// PreQueryResult holds the aggregated results of the pre-query pipeline.
type PreQueryResult struct {
	Intent           *Intent
	SuggestedTools   []string
	BudgetAllocation map[string]int
	PredictedCost    float64
	SystemPrompt     string
	CacheHit         bool
	CachedResponse   string
	InjectionRisk    *ScanResult
}

// PostResponseResult holds the aggregated results of the post-response pipeline.
type PostResponseResult struct {
	FormattedResponse string
	QualityScore      float64
	MentionedFiles    []string
	TokensUsed        int
}

// PostToolResult holds the aggregated results of the post-tool-execution pipeline.
type PostToolResult struct {
	StallWarning   string
	LintErrors     string
	TestFailures   string
	RecoveryAction string
	ShouldRetry    bool
}

// SessionSummary captures the final assessment when a session ends.
type SessionSummary struct {
	Assessment    *Assessment
	Experience    *Experience
	Summary       string
	TokensTotal   int
	Duration      time.Duration
	FilesModified []string
}

// ---------------------------------------------------------------------------
// Constructor
// ---------------------------------------------------------------------------

// NewIntegrationPipeline initializes all subsystems and returns a ready-to-use
// pipeline orchestrator.
func NewIntegrationPipeline() *IntegrationPipeline {
	return &IntegrationPipeline{
		// Pre-query
		IntentClassifier: NewIntentClassifier(),
		ToolSelector:     NewToolSelector(defaultToolSet()),
		ContextDecay:     ctxmgr.NewContextDecay(30 * time.Minute),
		BudgetAllocator:  newDefaultBudgetAllocator(),
		TokenPredictor:   NewTokenPredictor(),
		AdaptivePrompt:   NewSystemPromptBuilder("", 4096),

		// Post-response
		ResponseFormatter:   NewResponseFormatter(),
		QualityScorer:       NewQualityScorer(),
		FileMentionDetector: NewFileMentionDetector("."),

		// Post-tool
		LintLoop:      NewLintLoop(),
		TestLoop:      NewTestLoop(),
		StallDetector: NewStallDetector(),
		ErrorRecovery: NewErrorRecovery(),

		// Security
		InjectionScanner: NewInjectionScanner(),
		OutputRedactor:   NewOutputRedactor(),

		// Learning
		ExperienceStore:   NewExperienceStore(".hawk/experience"),
		KnowledgeBase:     NewKnowledgeBase(".hawk/knowledge"),
		FeedbackCollector: NewFeedbackCollector(".hawk/feedback"),
		SelfAssessor:      NewSelfAssessor(),

		// Session management
		Timeline:       NewTimeline("default"),
		TokenReporter:  NewTokenReporter(200000),
		WorkspaceState: NewWorkspaceState("."),
		CommandHistory: NewCommandHistory(),
		ResponseCache:  NewResponseCache(1000, 24*time.Hour),

		// Internal
		sessionStart: time.Now(),
	}
}

// defaultToolSet returns the standard set of tools available to the agent.
func defaultToolSet() []ToolInfo {
	return []ToolInfo{
		{Name: "Read", Category: "file", Cost: "free", ReadOnly: true},
		{Name: "Write", Category: "file", Cost: "cheap", ReadOnly: false},
		{Name: "Edit", Category: "file", Cost: "cheap", ReadOnly: false},
		{Name: "Bash", Category: "exec", Cost: "cheap", ReadOnly: false},
		{Name: "Grep", Category: "search", Cost: "free", ReadOnly: true},
		{Name: "Glob", Category: "search", Cost: "free", ReadOnly: true},
		{Name: "LS", Category: "file", Cost: "free", ReadOnly: true},
		{Name: "WebSearch", Category: "web", Cost: "expensive", ReadOnly: true},
		{Name: "Agent", Category: "agent", Cost: "expensive", ReadOnly: false},
	}
}

// newDefaultBudgetAllocator creates a BudgetAllocator pre-configured with standard
// allocations for system, context, tools, and output.
func newDefaultBudgetAllocator() *BudgetAllocator {
	ba := NewBudgetAllocator(128000, 16000)
	ba.Register("system", 2000, 8000, 1, false)
	ba.Register("context", 4000, 64000, 2, true)
	ba.Register("tools", 1000, 32000, 3, true)
	ba.Register("history", 2000, 48000, 4, true)
	return ba
}

// ---------------------------------------------------------------------------
// PreQuery Pipeline
// ---------------------------------------------------------------------------

// PreQuery runs the full pre-query pipeline: classify intent, select tools, apply
// context decay, allocate budget, predict cost, build system prompt, scan for
// injection, and check the response cache.
func (p *IntegrationPipeline) PreQuery(messages []types.EyrieMessage, userInput string) *PreQueryResult {
	p.mu.Lock()
	defer p.mu.Unlock()

	result := &PreQueryResult{}

	// 1. Classify intent
	result.Intent = p.IntentClassifier.Classify(userInput)

	// 2. Select optimal tools based on intent
	toolSelection := p.ToolSelector.Select(userInput, 6)
	result.SuggestedTools = toolSelection.Recommended

	// 3. Apply context decay — prune stale entries
	p.ContextDecay.ApplyDecay()

	// 4. Allocate token budget across sections
	result.BudgetAllocation = p.BudgetAllocator.Allocate()

	// 5. Predict cost
	contextSize := integrationEstimateTokens(messages)
	prediction := p.TokenPredictor.Predict(userInput, contextSize, "")
	if prediction != nil {
		result.PredictedCost = prediction.EstimatedCost
	}

	// 6. Build adaptive system prompt
	buildCtx := PromptBuildContext{
		Task:      userInput,
		TurnCount: len(messages),
	}
	if result.Intent != nil {
		buildCtx.ProjectType = result.Intent.Category
	}
	result.SystemPrompt = p.AdaptivePrompt.Build(buildCtx)

	// 7. Scan for injection attempts
	result.InjectionRisk = p.InjectionScanner.Scan(userInput)

	// 8. Check response cache
	if ShouldCache(userInput) {
		if entry, ok := p.ResponseCache.Get(userInput, ""); ok {
			result.CacheHit = true
			result.CachedResponse = entry.Response
		}
	}

	// 9. Record on timeline
	p.Timeline.AddEvent("pre_query", userInput, map[string]string{
		"intent":     intentCategory(result.Intent),
		"cache_hit":  fmt.Sprintf("%t", result.CacheHit),
		"risk_level": result.InjectionRisk.RiskLevel,
	})

	return result
}

// ---------------------------------------------------------------------------
// PostResponse Pipeline
// ---------------------------------------------------------------------------

// PostResponse runs the full post-response pipeline: format, score quality,
// detect file mentions, redact secrets, update timeline, record tokens, cache,
// and update the experience store.
func (p *IntegrationPipeline) PostResponse(response string, messages []types.EyrieMessage) *PostResponseResult {
	p.mu.Lock()
	defer p.mu.Unlock()

	result := &PostResponseResult{}

	// 1. Format response (fix fences, strip fluff)
	formatted := p.ResponseFormatter.Format(response)
	result.FormattedResponse = formatted.Formatted

	// 2. Score quality
	userPrompt := lastUserMessage(messages)
	scoreCtx := ResponseContext{
		UserPrompt:        userPrompt,
		AssistantResponse: result.FormattedResponse,
		TokensUsed:        EstimateStringTokens(result.FormattedResponse),
	}
	scored := p.QualityScorer.Score(scoreCtx)
	result.QualityScore = scored.Score

	// 3. Detect file mentions
	result.MentionedFiles = p.FileMentionDetector.DetectMentions(result.FormattedResponse)

	// 4. Redact secrets from output (hawk's patterns + tok's 27 patterns)
	result.FormattedResponse = p.OutputRedactor.Redact(result.FormattedResponse)
	result.FormattedResponse = tok.DefaultSecretDetector().RedactSecrets(result.FormattedResponse)

	// 5. Update timeline
	p.Timeline.AddEvent("response", "", map[string]string{
		"quality": fmt.Sprintf("%.2f", result.QualityScore),
		"files":   fmt.Sprintf("%d", len(result.MentionedFiles)),
	})

	// 6. Record token usage
	tokensUsed := EstimateStringTokens(result.FormattedResponse)
	result.TokensUsed = tokensUsed
	p.TokenReporter.Record(0, tokensUsed, "", "", 0)

	// 7. Cache the response
	if ShouldCache(userPrompt) && result.QualityScore >= 0.7 {
		p.ResponseCache.Set(userPrompt, result.FormattedResponse, "", tokensUsed)
	}

	// 8. Update experience store with implicit signal
	_ = p.FeedbackCollector.RecordImplicit(ImplicitSignal{
		Type:      "accepted",
		SessionID: p.Timeline.SessionID,
		Timestamp: time.Now(),
	})

	return result
}

// ---------------------------------------------------------------------------
// PostToolExecution Pipeline
// ---------------------------------------------------------------------------

// PostToolExecution runs checks after a tool completes: stall detection, lint,
// test, error recovery, workspace state update, timeline record, and command
// history update.
func (p *IntegrationPipeline) PostToolExecution(toolName string, args map[string]interface{}, output string, err error) *PostToolResult {
	p.mu.Lock()
	defer p.mu.Unlock()

	result := &PostToolResult{}

	// 1. Check for stalls (repeated calls)
	p.StallDetector.Record(toolName, args, output)
	stallResult := p.StallDetector.Check()
	if stallResult != nil && stallResult.IsStalled {
		result.StallWarning = stallResult.Suggestion
	}

	// 2. Run lint if a file was edited
	if isFileEditTool(toolName) {
		filePath := integrationExtractFilePath(args)
		if filePath != "" {
			lintResult, lintErr := p.LintLoop.RunLint(filePath)
			if lintErr == nil && lintResult != nil && lintResult.ExitCode != 0 {
				result.LintErrors = p.LintLoop.BuildReflectedMessage(lintResult)
			}
		}
	}

	// 3. Run tests if code was modified
	if isCodeModifyTool(toolName) && err == nil {
		// Only flag test failures; actual test execution happens in the loop
		filePath := integrationExtractFilePath(args)
		if filePath != "" && p.TestLoop.ShouldRetry(filePath) {
			result.TestFailures = "Previous test failures unresolved for " + filePath
			result.ShouldRetry = true
		}
	}

	// 4. Attempt error recovery if the tool failed
	if err != nil {
		recoveryCtx := &RecoveryContext{
			Error:        err,
			ErrorMsg:     err.Error(),
			LastToolCall: toolName,
			LastArgs:     args,
		}
		recoveryResult, recoveryErr := p.ErrorRecovery.Recover(err, recoveryCtx)
		if recoveryErr == nil && recoveryResult != nil {
			result.RecoveryAction = recoveryResult.Action
			result.ShouldRetry = recoveryResult.Recovered
		}
	}

	// 5. Update workspace state
	if isFileEditTool(toolName) {
		filePath := integrationExtractFilePath(args)
		if filePath != "" {
			p.WorkspaceState.MarkModified(filePath)
		}
	}

	// 6. Record on timeline
	p.Timeline.AddAction(toolName, integrationExtractFilePath(args), 0)

	// 7. Update command history for Bash calls
	if toolName == "Bash" {
		cmd := ""
		if c, ok := args["command"]; ok {
			cmd, _ = c.(string)
		}
		exitCode := 0
		if err != nil {
			exitCode = 1
		}
		p.CommandHistory.Record(cmd, exitCode, 0, output)
	}

	return result
}

// ---------------------------------------------------------------------------
// EndSession Pipeline
// ---------------------------------------------------------------------------

// EndSession runs the session-end pipeline: self-assess, record experience,
// update knowledge base, collect feedback, and generate a summary.
func (p *IntegrationPipeline) EndSession(success bool, taskGoal string) *SessionSummary {
	p.mu.Lock()
	defer p.mu.Unlock()

	summary := &SessionSummary{
		Duration:      time.Since(p.sessionStart),
		FilesModified: p.WorkspaceState.GetModified(),
	}

	// 1. Self-assess performance
	taskCtx := TaskContext{
		Goal:          taskGoal,
		FilesModified: len(summary.FilesModified),
		Duration:      summary.Duration,
		TestsPassed:   success,
	}
	summary.Assessment = p.SelfAssessor.Assess(taskCtx)

	// 2. Record experience
	outcome := "success"
	if !success {
		outcome = "failure"
	}
	summary.Experience = p.ExperienceStore.Record(
		taskGoal,
		"integration_pipeline",
		outcome,
		nil, // steps
		nil, // tools
		summary.FilesModified,
		summary.TokensTotal,
		summary.Duration,
	)

	// 3. Update knowledge base
	if success && taskGoal != "" {
		_ = p.KnowledgeBase.Add(&KnowledgeEntry{
			Title:      fmt.Sprintf("Completed: %s", truncateString(taskGoal, 60)),
			Content:    fmt.Sprintf("Task completed in %s with %d files modified.", summary.Duration.Round(time.Second), len(summary.FilesModified)),
			Category:   "session_outcome",
			Tags:       []string{"session", outcome},
			Confidence: summary.Assessment.Score,
			CreatedAt:  time.Now(),
		})
	}

	// 4. Collect implicit feedback
	_ = p.FeedbackCollector.RecordImplicit(ImplicitSignal{
		Type:      "accepted",
		SessionID: p.Timeline.SessionID,
		Timestamp: time.Now(),
	})

	// 5. Generate session summary text
	summary.Summary = p.buildSummaryText(success, taskGoal, summary)

	// 6. Record tokens total from reporter
	summary.TokensTotal = p.totalTokensUsed()

	// 7. Persist learning data
	_ = p.ExperienceStore.Save()
	_ = p.KnowledgeBase.Save()
	_ = p.FeedbackCollector.Save()

	return summary
}

// ---------------------------------------------------------------------------
// FormatPipelineStatus
// ---------------------------------------------------------------------------

// FormatPipelineStatus returns a human-readable view of which subsystems are
// active or inactive in the pipeline.
func (p *IntegrationPipeline) FormatPipelineStatus() string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var b strings.Builder
	b.WriteString("=== Integration Pipeline Status ===\n\n")

	b.WriteString("Pre-Query Pipeline:\n")
	b.WriteString(formatStatus("  IntentClassifier", p.IntentClassifier != nil))
	b.WriteString(formatStatus("  ToolSelector", p.ToolSelector != nil))
	b.WriteString(formatStatus("  ContextDecay", p.ContextDecay != nil))
	b.WriteString(formatStatus("  BudgetAllocator", p.BudgetAllocator != nil))
	b.WriteString(formatStatus("  TokenPredictor", p.TokenPredictor != nil))
	b.WriteString(formatStatus("  AdaptivePrompt", p.AdaptivePrompt != nil))

	b.WriteString("\nPost-Response Pipeline:\n")
	b.WriteString(formatStatus("  ResponseFormatter", p.ResponseFormatter != nil))
	b.WriteString(formatStatus("  QualityScorer", p.QualityScorer != nil))
	b.WriteString(formatStatus("  FileMentionDetector", p.FileMentionDetector != nil))

	b.WriteString("\nPost-Tool Pipeline:\n")
	b.WriteString(formatStatus("  LintLoop", p.LintLoop != nil))
	b.WriteString(formatStatus("  TestLoop", p.TestLoop != nil))
	b.WriteString(formatStatus("  StallDetector", p.StallDetector != nil))
	b.WriteString(formatStatus("  ErrorRecovery", p.ErrorRecovery != nil))

	b.WriteString("\nSecurity Pipeline:\n")
	b.WriteString(formatStatus("  InjectionScanner", p.InjectionScanner != nil))
	b.WriteString(formatStatus("  OutputRedactor", p.OutputRedactor != nil))

	b.WriteString("\nLearning Pipeline:\n")
	b.WriteString(formatStatus("  ExperienceStore", p.ExperienceStore != nil))
	b.WriteString(formatStatus("  KnowledgeBase", p.KnowledgeBase != nil))
	b.WriteString(formatStatus("  FeedbackCollector", p.FeedbackCollector != nil))
	b.WriteString(formatStatus("  SelfAssessor", p.SelfAssessor != nil))

	b.WriteString("\nSession Management:\n")
	b.WriteString(formatStatus("  Timeline", p.Timeline != nil))
	b.WriteString(formatStatus("  TokenReporter", p.TokenReporter != nil))
	b.WriteString(formatStatus("  WorkspaceState", p.WorkspaceState != nil))
	b.WriteString(formatStatus("  CommandHistory", p.CommandHistory != nil))
	b.WriteString(formatStatus("  ResponseCache", p.ResponseCache != nil))

	b.WriteString(fmt.Sprintf("\nSession uptime: %s\n", time.Since(p.sessionStart).Round(time.Second)))

	return b.String()
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// formatStatus renders a subsystem status line.
func formatStatus(name string, active bool) string {
	if active {
		return fmt.Sprintf("%s: [active]\n", name)
	}
	return fmt.Sprintf("%s: [inactive]\n", name)
}

// intentCategory safely extracts the category from a potentially-nil intent.
func intentCategory(intent *Intent) string {
	if intent == nil {
		return "unknown"
	}
	return intent.Category
}

// lastUserMessage extracts the last user message from the conversation.
func lastUserMessage(messages []types.EyrieMessage) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" && messages[i].ToolResult == nil {
			return messages[i].Content
		}
	}
	return ""
}

// integrationEstimateTokens gives a rough token count for a message slice.
func integrationEstimateTokens(messages []types.EyrieMessage) int {
	total := 0
	for _, m := range messages {
		total += EstimateStringTokens(m.Content)
	}
	return total
}

// isFileEditTool returns true if the tool modifies files.
func isFileEditTool(toolName string) bool {
	switch toolName {
	case "Edit", "Write", "MultiEdit":
		return true
	}
	return false
}

// isCodeModifyTool returns true if the tool can modify code.
func isCodeModifyTool(toolName string) bool {
	switch toolName {
	case "Edit", "Write", "MultiEdit", "Bash":
		return true
	}
	return false
}

// integrationExtractFilePath pulls the file_path or path from tool args.
func integrationExtractFilePath(args map[string]interface{}) string {
	if args == nil {
		return ""
	}
	if fp, ok := args["file_path"]; ok {
		if s, ok := fp.(string); ok {
			return s
		}
	}
	if fp, ok := args["path"]; ok {
		if s, ok := fp.(string); ok {
			return s
		}
	}
	return ""
}

// truncateString shortens a string to maxLen, appending "..." if truncated.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// buildSummaryText generates a human-readable session summary.
func (p *IntegrationPipeline) buildSummaryText(success bool, taskGoal string, summary *SessionSummary) string {
	var b strings.Builder

	status := "completed successfully"
	if !success {
		status = "ended with issues"
	}
	b.WriteString(fmt.Sprintf("Session %s in %s.\n", status, summary.Duration.Round(time.Second)))

	if taskGoal != "" {
		b.WriteString(fmt.Sprintf("Goal: %s\n", truncateString(taskGoal, 100)))
	}

	if len(summary.FilesModified) > 0 {
		b.WriteString(fmt.Sprintf("Files modified: %d\n", len(summary.FilesModified)))
		for _, f := range summary.FilesModified {
			b.WriteString(fmt.Sprintf("  - %s\n", f))
		}
	}

	if summary.Assessment != nil {
		b.WriteString(fmt.Sprintf("Performance score: %.0f%%\n", summary.Assessment.Score*100))
		if len(summary.Assessment.Improvements) > 0 {
			b.WriteString("Improvements for next time:\n")
			for _, imp := range summary.Assessment.Improvements {
				b.WriteString(fmt.Sprintf("  - %s\n", imp))
			}
		}
	}

	return b.String()
}

// totalTokensUsed reads the total from the token reporter.
func (p *IntegrationPipeline) totalTokensUsed() int {
	remaining := p.TokenReporter.GetRemaining()
	// TokenReporter tracks remaining from a budget; infer used tokens.
	// Budget is set to 200000 in constructor.
	return 200000 - remaining
}
