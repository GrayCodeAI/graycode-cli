package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/GrayCodeAI/eyrie/client"

	"github.com/GrayCodeAI/hawk/analytics"
	"github.com/GrayCodeAI/hawk/hooks"
	modelPkg "github.com/GrayCodeAI/hawk/routing"
	"github.com/GrayCodeAI/hawk/retry"
	"github.com/GrayCodeAI/hawk/tool"
)

// Stream runs the agentic loop: LLM → tool_use → execute → loop.
func (s *Session) Stream(ctx context.Context) (<-chan StreamEvent, error) {
	ch := make(chan StreamEvent, 64)
	go s.agentLoop(ctx, ch)
	return ch, nil
}

func (s *Session) agentLoop(ctx context.Context, ch chan<- StreamEvent) {
	defer close(ch)
	sessionStart := time.Now()

	// Self-improvement: run OnSessionEnd when the loop exits (regardless of how)
	defer func() {
		if s.Lifecycle != nil {
			outcome := SessionOutcome{
				Success:  ctx.Err() == nil,
				Duration: time.Since(sessionStart),
			}
			if len(s.messages) > 0 {
				for _, m := range s.messages {
					if m.Role == "user" && m.ToolResult == nil && outcome.TaskGoal == "" {
						outcome.TaskGoal = m.Content
					}
				}
			}
			_ = s.Lifecycle.OnSessionEnd(ctx, s, outcome)
		}
	}()

	// Session start hook
	hooks.ExecuteAsync(ctx, hooks.EventSessionStart, map[string]interface{}{
		"provider": s.provider,
		"model":    s.model,
	})

	// Self-improvement: inject learned guidelines and skills from prior sessions
	if s.Lifecycle != nil && len(s.messages) > 0 {
		lastMsg := s.messages[len(s.messages)-1].Content
		if learnedCtx := s.Lifecycle.OnSessionStart(ctx, lastMsg); learnedCtx != "" {
			s.AppendSystemContext(learnedCtx)
		}
	}

	// Inject remembered context from yaad into system prompt
	if s.Memory != nil && len(s.messages) > 0 {
		lastMsg := s.messages[len(s.messages)-1].Content
		remembered, err := s.Memory.Recall(lastMsg, 2000)
		if err == nil && remembered != "" {
			s.AppendSystemContext("## Relevant Memories\n" + remembered)
		}
	}

	recoveryCount := 0
	turnCount := 0
	toolTurns := 0 // turns that used tools (for skill distillation)
	var toolsUsedSet map[string]bool
	var filesModifiedSet map[string]bool
	snowball := NewSnowballDetector(500000) // 500K token ceiling
	loopDet := NewLoopDetector(10, 4)       // 10-step window, 4 repeats = stuck

	for {
		// Timeout check: abort if context was cancelled by a time budget
		if ctx.Err() != nil {
			ch <- StreamEvent{Type: "content", Content: "\n\nTime budget exhausted."}
			ch <- StreamEvent{Type: "done"}
			return
		}

		// Snowball abort check
		if snowball.ShouldAbort() {
			ch <- StreamEvent{Type: "content", Content: "\n\n" + snowball.Summary()}
			ch <- StreamEvent{Type: "done"}
			return
		}

		// Loop detection check
		if loopDet.IsLooping() {
			ch <- StreamEvent{Type: "content", Content: "\n\n⚠ " + loopDet.LoopWarning()}
			ch <- StreamEvent{Type: "done"}
			return
		}

		// Safety limits check
		if s.Limits != nil {
			if exceeded, reason := s.Limits.IsExceeded(); exceeded {
				ch <- StreamEvent{Type: "content", Content: fmt.Sprintf("\n\nLimit reached: %s", reason)}
				ch <- StreamEvent{Type: "done"}
				return
			}
		}

		// Enforce MaxTurns budget
		if s.MaxTurns > 0 && turnCount >= s.MaxTurns {
			ch <- StreamEvent{Type: "content", Content: "Turn limit reached — stopping."}
			ch <- StreamEvent{Type: "done"}
			return
		}
		turnCount++

		// Record turn for limits tracking
		if s.Limits != nil {
			s.Limits.RecordTurn()
		}

		// Belief maintenance: prune stale beliefs (injected at query time below)
		if s.Beliefs != nil && s.Beliefs.Size() > 0 {
			s.Beliefs.Prune(turnCount)
		}
		// Auto-compact if conversation is too long (message count)
		if len(s.messages) > maxContextMessages {
			s.messages = CollapseRepeatedMessages(s.messages)
			if len(s.messages) > maxContextMessages {
				s.smartCompact()
			}
		}

		// Auto-compact if token usage exceeds context budget allocation
		convTokens := EstimateTokens(s.messages)
		if info, ok := modelPkg.Find(s.model); ok && info.ContextSize > 0 {
			budget := NewContextBudget(info.ContextSize)
			if budget.ShouldCompact(convTokens) {
				s.smartCompact()
			}
		}

		// Pre-query hook
		hooks.Execute(ctx, hooks.EventPreQuery, map[string]interface{}{
			"provider": s.provider,
			"model":    s.model,
			"messages": len(s.messages),
		})

		s.log.Info("stream query", map[string]interface{}{
			"provider": s.provider,
			"model":    s.model,
			"messages": len(s.messages),
		})

		// Dynamic max_tokens based on task type and recent tool patterns
		taskType := classifyPromptForBudget(s.messages)
		contextSize := 200000
		if info, ok := modelPkg.Find(s.model); ok && info.ContextSize > 0 {
			contextSize = info.ContextSize
		}
		maxTok := DynamicMaxTokens(s.messages, contextSize, taskType)

		// Model cascade: select optimal model for this request
		activeModel := s.model
		if s.Cascade != nil && s.Cascade.Enabled {
			lastUserMsg := ""
			for i := len(s.messages) - 1; i >= 0; i-- {
				if s.messages[i].Role == "user" {
					lastUserMsg = s.messages[i].Content
					break
				}
			}
			activeModel = s.Cascade.SelectModel(lastUserMsg, s.model, "")
		}

		opts := client.ChatOptions{
			Provider:      s.provider,
			Model:         activeModel,
			MaxTokens:     maxTok,
			System:        s.system,
			EnableCaching: s.provider == "anthropic",
		}
		// Inject beliefs as ephemeral context (not persisted to s.system)
		if s.Beliefs != nil && s.Beliefs.Size() > 0 {
			if summary := s.Beliefs.FormatForPrompt(); summary != "" {
				opts.System += "\n\n## Agent Beliefs\n" + summary
			}
		}
		if s.registry != nil {
			opts.Tools = s.registry.EyrieTools()
		}

		// Inject memory metadata from yaad
		if s.YaadBridge != nil && s.YaadBridge.Ready() {
			if _, contents, err := s.YaadBridge.SearchByType("convention", 100); err == nil {
				convCount := len(contents)
				if _, dContents, err := s.YaadBridge.SearchByType("decision", 100); err == nil {
					decCount := len(dContents)
					total := convCount + decCount
					if total > 0 {
						opts.System += fmt.Sprintf("\n\nMemory: %d nodes (%d conventions, %d decisions)", total, convCount, decCount)
					}
				}
			}
		}

		// Circuit breaker: select provider with failover
		if s.Router != nil {
			if selectedProvider, err := s.Router.SelectProvider(s.provider); err == nil && selectedProvider != s.provider {
				s.log.Info("provider failover", map[string]interface{}{"from": s.provider, "to": selectedProvider})
				opts.Provider = selectedProvider
			}
		}

		// Count actual input tokens for precise budget tracking
		inputTokens := 0
		for _, msg := range s.messages {
			inputTokens += CountTokensFast(msg.Content)
			if msg.ToolResult != nil {
				inputTokens += CountTokensFast(msg.ToolResult.Content)
			}
		}
		inputTokens += CountTokensFast(s.system)
		s.log.Info("token count", map[string]interface{}{"input_tokens": inputTokens, "model": s.model})

		// Cost warning for expensive calls
		if inPrice, outPrice := pricingForModel(s.model); true {
			estCost := float64(inputTokens)*inPrice/1_000_000 + float64(maxTok)*outPrice/1_000_000
			if estCost > 0.50 {
				ch <- StreamEvent{Type: "content", Content: fmt.Sprintf("\n⚠ This request will use ~%d tokens (~$%.2f). Continue? The agent will proceed automatically.\n", inputTokens+maxTok, estCost)}
			}
		}

		var result *client.StreamResult
		var err error

		// Use retry for transient errors
		retryCfg := retry.DefaultConfig()
		retryCfg.MaxRetries = 2
		retryCfg.BaseDelay = 500 * time.Millisecond

		s.metrics.Counter("api.requests").Inc()
		apiStart := time.Now()

		err = retry.Do(ctx, retryCfg, func() error {
			result, err = s.client.StreamChat(ctx, s.messages, opts)
			if err != nil {
				if strings.Contains(err.Error(), "too long") || strings.Contains(err.Error(), "too many tokens") {
					s.compact()
					result, err = s.client.StreamChat(ctx, s.messages, opts)
				}
			}
			return err
		})

		apiDuration := time.Since(apiStart)
		s.metrics.Timer("api.latency").Record(apiDuration)
		s.metrics.Timer("api.last_latency").Record(apiDuration)

		if err != nil {
			// Record failure for circuit breaker
			if s.Router != nil {
				s.Router.RecordFailure(s.provider, err)
			}
			s.log.Error("stream error", map[string]interface{}{
				"error": err.Error(),
			})
			ch <- StreamEvent{Type: "error", Content: err.Error()}
			return
		}

		// Record success for circuit breaker
		if s.Router != nil {
			s.Router.RecordSuccess(s.provider, apiDuration)
		}

		var textContent string
		var toolCalls []client.ToolCall
		var stopReason string
		var lastUsage *client.EyrieUsage

		// Streaming with retry for transient stream errors
		const maxStreamRetries = 2
		var streamErr error
		for streamAttempt := 0; streamAttempt <= maxStreamRetries; streamAttempt++ {
			streamErr = nil
			for ev := range result.Events {
				select {
				case <-ctx.Done():
					result.Close()
					return
				default:
				}
				switch ev.Type {
				case "content":
					textContent += ev.Content
					ch <- StreamEvent{Type: "content", Content: ev.Content}
				case "thinking":
					ch <- StreamEvent{Type: "thinking", Content: ev.Thinking}
				case "tool_call":
					if ev.ToolCall != nil {
						toolCalls = append(toolCalls, *ev.ToolCall)
					}
				case "usage":
					if ev.Usage != nil {
						s.Cost.Add(ev.Usage.PromptTokens, ev.Usage.CompletionTokens)
						lastUsage = ev.Usage
						// Persist cost entry for analytics
						if s.CostTracker != nil {
							inPrice, outPrice := pricingForModel(activeModel)
							cost := float64(ev.Usage.PromptTokens)*inPrice/1_000_000 + float64(ev.Usage.CompletionTokens)*outPrice/1_000_000
							s.CostTracker.Record(analytics.CostEntry{
								Model:        activeModel,
								TaskType:     taskType,
								InputTokens:  ev.Usage.PromptTokens,
								OutputTokens: ev.Usage.CompletionTokens,
								CostUSD:      cost,
								Duration:     time.Since(apiStart),
								Kept:         true,
							})
						}
						ch <- StreamEvent{
							Type: "usage",
							Usage: &StreamUsage{
								PromptTokens:     ev.Usage.PromptTokens,
								CompletionTokens: ev.Usage.CompletionTokens,
							},
						}
					}
				case "error":
					streamErr = fmt.Errorf("%s", ev.Error)
					if isRetryableStreamError(streamErr) {
						break // break switch, will check in outer loop
					}
					ch <- StreamEvent{Type: "error", Content: ev.Error}
					result.Close()
					return
				case "done":
					if ev.StopReason != "" {
						stopReason = ev.StopReason
					}
				}
			}
			result.Close()

			if streamErr == nil {
				break
			}
			if !isRetryableStreamError(streamErr) {
				break
			}
			s.log.Warn("stream retry", map[string]interface{}{"attempt": streamAttempt + 1, "error": streamErr.Error()})
			time.Sleep(time.Duration(streamAttempt+1) * time.Second)

			// Re-open the stream for retry
			result, err = s.client.StreamChat(ctx, s.messages, opts)
			if err != nil {
				ch <- StreamEvent{Type: "error", Content: err.Error()}
				return
			}
			// Reset accumulated state for the retry
			textContent = ""
			toolCalls = nil
			stopReason = ""
			lastUsage = nil
		}

		// Snowball detector: record usage after each API response
		if lastUsage != nil {
			progress := 0.5
			if len(toolCalls) > 0 {
				progress = 1.0
			}
			snowball.RecordTurn(lastUsage.PromptTokens+lastUsage.CompletionTokens, progress)
		}

		// Budget enforcement
		if s.MaxBudgetUSD > 0 && s.Cost.TotalUSD() >= s.MaxBudgetUSD {
			ch <- StreamEvent{Type: "content", Content: fmt.Sprintf("\n\nBudget limit reached ($%.2f spent of $%.2f).", s.Cost.TotalUSD(), s.MaxBudgetUSD)}
			ch <- StreamEvent{Type: "done"}
			return
		}

		// Check for inline tool calls in text (some providers embed tool calls in text)
		if len(toolCalls) == 0 && strings.Contains(textContent, "<|tool_calls_section_begin|>") {
			cleanText, inlineCalls := client.ParseInlineToolCalls(textContent)
			if len(inlineCalls) > 0 {
				textContent = cleanText
				for _, ic := range inlineCalls {
					toolCalls = append(toolCalls, ic)
				}
			}
		}

		// Cap tool calls per step
		const maxToolCallsPerStep = 32
		if len(toolCalls) > maxToolCallsPerStep {
			excess := toolCalls[maxToolCallsPerStep:]
			toolCalls = toolCalls[:maxToolCallsPerStep]
			for _, tc := range excess {
				ch <- StreamEvent{Type: "tool_result", ToolName: tc.Name, Content: "Error: too many tool calls in one step (max 32). Retry with fewer calls."}
			}
		}

		// Post-query hook
		hooks.ExecuteAsync(ctx, hooks.EventPostQuery, map[string]interface{}{
			"provider": s.provider,
			"model":    s.model,
			"content":  textContent,
			"tools":    len(toolCalls),
		})

		// Activity nudge: remind agent to persist learnings if idle
		if s.Activity != nil {
			if nudge := s.Activity.NudgeMessage(); nudge != "" {
				s.AppendSystemContext(nudge)
			}
		}

		// Handle max_tokens recovery
		if stopReason == "max_tokens" && len(toolCalls) == 0 && recoveryCount < maxRecoveryRetries {
			recoveryCount++
			s.messages = append(s.messages, client.EyrieMessage{Role: "assistant", Content: textContent})
			s.messages = append(s.messages, client.EyrieMessage{Role: "user", Content: "Continue from where you left off."})
			continue
		}

		// No tool calls — done
		if len(toolCalls) == 0 {
			if textContent != "" {
				s.messages = append(s.messages, client.EyrieMessage{Role: "assistant", Content: textContent})
				// Auto-remember corrections and learnings
				if s.Memory != nil && shouldRemember(textContent) {
					go s.Memory.Remember(textContent, "assistant_learning")
				}
			}
			// Sleeptime: background memory consolidation
			if s.Sleeptime != nil && s.Sleeptime.ShouldRun() && s.YaadBridge != nil && s.YaadBridge.Ready() {
				go func() {
					var transcript []string
					for _, m := range s.messages {
						transcript = append(transcript, m.Role+": "+m.Content)
					}
					memState := ""
					if s.Memory != nil {
						memState, _ = s.Memory.Recall("", 2000)
					}
					prompt := s.Sleeptime.BuildConsolidationPrompt(transcript, memState)
					resp, err := s.client.Chat(context.Background(), []client.EyrieMessage{
						{Role: "user", Content: prompt},
					}, client.ChatOptions{Provider: s.provider, Model: s.model, MaxTokens: 2048})
					if err != nil || resp == nil {
						return
					}
					parseAndApplyMemoryOps(s.YaadBridge, resp.Content)
				}()
			}
			// Skill distillation: extract reusable skill from multi-turn tasks
			if s.SkillDistiller != nil && toolTurns >= 5 && s.YaadBridge != nil && s.YaadBridge.Ready() {
				go func() {
					var tools []string
					for t := range toolsUsedSet {
						tools = append(tools, t)
					}
					var files []string
					for f := range filesModifiedSet {
						files = append(files, f)
					}
					taskDesc := ""
					if len(s.messages) > 0 {
						taskDesc = s.messages[0].Content
					}
					sd := s.SkillDistiller
					prompt := sd.BuildSkillPrompt(taskDesc, tools, files, textContent)
					resp, err := s.client.Chat(context.Background(), []client.EyrieMessage{
						{Role: "user", Content: prompt},
					}, client.ChatOptions{Provider: s.provider, Model: s.model, MaxTokens: 2048})
					if err != nil || resp == nil {
						return
					}
					skill, err := sd.ParseSkill(resp.Content)
					if err != nil {
						return
					}
					content, _ := json.Marshal(skill)
					s.YaadBridge.Remember(string(content), "skill")
				}()
			}
			ch <- StreamEvent{Type: "done"}
			// Session end hook
			hooks.ExecuteAsync(ctx, hooks.EventSessionEnd, map[string]interface{}{
				"provider": s.provider,
				"model":    s.model,
				"messages": len(s.messages),
			})
			return
		}

		// Execute tools and collect results
		recoveryCount = 0
		if toolsUsedSet == nil {
			toolsUsedSet = map[string]bool{}
			filesModifiedSet = map[string]bool{}
		}
		for _, tc := range toolCalls {
			toolsUsedSet[tc.Name] = true
			cn := canonicalToolName(tc.Name)
			if cn == "Write" || cn == "Edit" {
				if p, ok := pathArgument(tc.Arguments); ok {
					filesModifiedSet[p] = true
				}
			}
		}
		toolTurns++
		type toolExecResult struct {
			tc     client.ToolCall
			output string
			isErr  bool
		}

		// Classify tools into concurrent (read-only) and sequential (write) batches
		safeConcurrent := map[string]bool{"Read": true, "Grep": true, "Glob": true, "LS": true, "WebSearch": true, "WebFetch": true, "ToolSearch": true}

		var concurrentCalls []client.ToolCall
		var sequentialCalls []client.ToolCall
		for _, tc := range toolCalls {
			if safeConcurrent[tc.Name] {
				concurrentCalls = append(concurrentCalls, tc)
			} else {
				sequentialCalls = append(sequentialCalls, tc)
			}
		}

		// Backtrack: record decision point when tool calls are pending
		if s.Backtrack != nil && len(toolCalls) > 0 {
			var toolNames []string
			for _, tc := range toolCalls {
				toolNames = append(toolNames, tc.Name)
			}
			s.Backtrack.RecordDecision(turnCount, strings.Join(toolNames, ", "), nil, s.messages)
		}

		var results []toolExecResult
		var mu sync.Mutex

		// executeSingleTool handles permission checking and execution for one tool call.
		executeSingleTool := func(tc client.ToolCall) toolExecResult {
			ch <- StreamEvent{Type: "tool_use", ToolName: tc.Name, ToolID: tc.ID}

			// Check permission for dangerous tools.
			// Use autonomy level to decide: the AutonomyConfig considers whether
			// the tool is read-only/write/bash and whether the specific invocation
			// is classified as safe.
			isSafe := !toolNeedsPermission(tc.Name, tc.Arguments)
			autoCfg := PresetConfig(s.Autonomy)
			needsPerm := autoCfg.NeedsPermission(tc.Name, isSafe)
			if needsPerm && s.PermissionFn != nil {
				summary := toolSummary(tc.Name, tc.Arguments)

				// Bypass killswitch check
				if s.BypassKill.IsEnabled() {
					goto executeTool
				}

				// Classifier-based auto-allow for safe commands
				if s.Classifier != nil && tc.Name == "Bash" {
					if classification := s.Classifier.Classify(summary); classification == "safe" {
						goto executeTool
					}
				}

				// Auto-mode check
				if s.AutoMode != nil {
					if allowed, ok := s.AutoMode.ShouldAutoAllow(tc.Name, summary); ok {
						if allowed {
							goto executeTool
						} else {
							ch <- StreamEvent{Type: "tool_result", ToolName: tc.Name, Content: "Permission denied (auto-mode)."}
							return toolExecResult{tc: tc, output: "Permission denied (auto-mode).", isErr: true}
						}
					}
				}

				// Permission mode check
				if decision := s.modeDecision(tc.Name); decision != nil {
					if !*decision {
						ch <- StreamEvent{Type: "tool_result", ToolName: tc.Name, Content: "Permission denied by permission mode."}
						return toolExecResult{tc: tc, output: "Permission denied by permission mode.", isErr: true}
					}
					goto executeTool
				}

				// Check memory first
				if decision := s.Permissions.Check(tc.Name, summary); decision != nil {
					if !*decision {
						ch <- StreamEvent{Type: "tool_result", ToolName: tc.Name, Content: "Permission denied (rule)."}
						return toolExecResult{tc: tc, output: "Permission denied (rule).", isErr: true}
					}
					// allowed by rule, proceed
				} else {
					// Ask user with timeout
					resp := make(chan bool, 1)
					s.PermissionFn(PermissionRequest{
						ToolName: tc.Name,
						ToolID:   tc.ID,
						Summary:  summary,
						Response: resp,
					})
					select {
					case allowed := <-resp:
						if !allowed {
							ch <- StreamEvent{Type: "tool_result", ToolName: tc.Name, Content: "Permission denied by user."}
							return toolExecResult{tc: tc, output: "Permission denied by user.", isErr: true}
						}
					case <-ctx.Done():
						ch <- StreamEvent{Type: "tool_result", ToolName: tc.Name, Content: "Permission prompt cancelled."}
						return toolExecResult{tc: tc, output: "Permission prompt cancelled.", isErr: true}
					case <-time.After(5 * time.Minute):
						ch <- StreamEvent{Type: "tool_result", ToolName: tc.Name, Content: "Permission prompt timed out."}
						return toolExecResult{tc: tc, output: "Permission prompt timed out.", isErr: true}
					}
				}
			}

		executeTool:
			// Pre-tool hook
			hooks.ExecuteAsync(ctx, hooks.EventPreTool, map[string]interface{}{
				"tool": tc.Name,
				"args": tc.Arguments,
			})

			inputJSON, _ := json.Marshal(tc.Arguments)
			toolCtx := tool.WithToolContext(ctx, &tool.ToolContext{
				AgentSpawnFn: s.AgentSpawnFn,
				AskUserFn:    s.AskUserFn,
				YaadBridge:   s.YaadBridge,
			})
			// Apply per-tool timeout so individual tools cannot block indefinitely.
			toolCtx, toolCancel := context.WithTimeout(toolCtx, toolTimeout(tc.Name))
			output, execErr := s.registry.Execute(toolCtx, tc.Name, inputJSON)
			toolCancel()
			isErr := execErr != nil
			if isErr {
				s.log.Warn("tool execution error", map[string]interface{}{
					"tool":  tc.Name,
					"error": execErr.Error(),
				})
				output = fmt.Sprintf("Error: %s", execErr.Error())
				// Backtrack: mark decision as failed on tool error
				if s.Backtrack != nil {
					s.Backtrack.MarkOutcome(turnCount, "failure")
				}
			} else {
				s.log.Info("tool executed", map[string]interface{}{
					"tool":   tc.Name,
					"output": len(output),
				})
			}

			// Limits: record every tool call
			if s.Limits != nil {
				s.Limits.RecordToolCall(tc.Name)
			}

			// Beliefs: record discoveries from read operations
			canonical := canonicalToolName(tc.Name)
			if s.Beliefs != nil && (canonical == "Read" || canonical == "Grep" || canonical == "Glob" || canonical == "LS") {
				subject := tc.Name
				if p, ok := pathArgument(tc.Arguments); ok {
					subject = p
				}
				// Use first 200 chars of output as summary
				contentSummary := output
				if len(contentSummary) > 200 {
					contentSummary = contentSummary[:200]
				}
				s.Beliefs.Record("file_purpose", subject, contentSummary, turnCount)
			}

			// Beliefs: invalidate beliefs when files are modified
			if s.Beliefs != nil && (canonical == "Write" || canonical == "Edit") {
				if p, ok := pathArgument(tc.Arguments); ok {
					s.Beliefs.Invalidate(p)
				}
			}

			// Critic: pre-screen Write/Edit patches before accepting
			if s.Critic != nil && !isErr && (canonical == "Write" || canonical == "Edit") {
				if p, ok := pathArgument(tc.Arguments); ok {
					// For writes, original content may be empty (new file)
					origContent := ""
					if data, readErr := readFileContent(p); readErr == nil {
						origContent = data
					}
					intent := textContent // use the LLM's text as intent context
					verdict := s.Critic.PreScreenPatch(origContent, output, intent)
					if s.Critic.ShouldBlock(verdict) {
						issueStr := strings.Join(verdict.Issues, "; ")
						output = fmt.Sprintf("Patch rejected by validator: %s. Try again.", issueStr)
						isErr = true
					}
				}
			}

			// Shadow: validate edits in a temporary workspace
			if s.Shadow != nil && !isErr && (canonical == "Write" || canonical == "Edit") {
				if p, ok := pathArgument(tc.Arguments); ok {
					validationErrs := s.Shadow.ValidateEdit(p, output)
					if len(validationErrs) > 0 {
						var warnings []string
						for _, ve := range validationErrs {
							warnings = append(warnings, ve.Message)
						}
						output += fmt.Sprintf("\n\nValidation warnings: %s", strings.Join(warnings, "; "))
					}
				}
			}

			// Sandbox: intercept Write/Edit to stage instead of apply
			if s.Sandbox != nil && s.Sandbox.IsEnabled() && !isErr && (canonical == "Write" || canonical == "Edit") {
				if p, ok := pathArgument(tc.Arguments); ok {
					origContent := ""
					if data, readErr := readFileContent(p); readErr == nil {
						origContent = data
					}
					action := "overwrite"
					if canonical == "Edit" {
						action = "edit"
					}
					s.Sandbox.Stage(p, action, origContent, output)
					output = fmt.Sprintf("Change staged for review (%s: %s)", action, p)
				}
			}

			// Dynamic truncation: 20% of model context window (4 chars/token), floor 5000, hard cap 50KB
			maxChars := 50000
			if info, ok := modelPkg.Find(s.model); ok && info.ContextSize > 0 {
				dynamic := info.ContextSize * 20 / 100 * 4
				if dynamic < 5000 {
					dynamic = 5000
				}
				if dynamic < maxChars {
					maxChars = dynamic
				}
			}
			compressBudget := maxChars / 2
			if len(output) > compressBudget {
				compressed, tokens := CompressForContext(output, compressBudget/4)
				if tokens > 0 && tokens < CountTokensFast(output) {
					output = compressed
				}
			}
			if len(output) > maxChars {
				output = output[:maxChars] + "\n... (truncated)"
			}

			// Post-tool hook
			s.metrics.Counter("tools.executed").Inc()
			if isErr {
				s.metrics.Counter("tools.errors").Inc()
			}

			hooks.ExecuteAsync(ctx, hooks.EventPostTool, map[string]interface{}{
				"tool":   tc.Name,
				"output": output,
				"is_err": isErr,
			})

			ch <- StreamEvent{Type: "tool_result", ToolName: tc.Name, Content: output}
			return toolExecResult{tc: tc, output: output, isErr: isErr}
		}

		// Execute concurrent batch (read-only tools) in parallel
		if len(concurrentCalls) > 0 {
			var wg sync.WaitGroup
			for _, tc := range concurrentCalls {
				wg.Add(1)
				go func(tc client.ToolCall) {
					defer wg.Done()
					r := executeSingleTool(tc)
					mu.Lock()
					results = append(results, r)
					mu.Unlock()
				}(tc)
			}
			wg.Wait()
		}

		// Execute sequential batch (write tools) one-by-one
		for _, tc := range sequentialCalls {
			r := executeSingleTool(tc)
			results = append(results, r)
		}

		// Append assistant message with tool_use blocks
		assistContent := textContent
		if assistContent == "" && len(toolCalls) > 0 {
			assistContent = " " // non-empty to satisfy APIs that reject empty content
		}
		s.messages = append(s.messages, client.EyrieMessage{
			Role:    "assistant",
			Content: assistContent,
			ToolUse: toolCalls,
		})
		// Append tool results as proper tool_result messages
		for _, r := range results {
			resultContent := r.output
			if resultContent == "" {
				resultContent = "(no output)"
			}
			s.messages = append(s.messages, client.EyrieMessage{
				Role:    "user",
				Content: resultContent,
				ToolResult: &client.ToolResult{
					ToolUseID: r.tc.ID,
					Content:   resultContent,
					IsError:   r.isErr,
				},
			})
		}

		// Loop detection: record this step's tool call signatures
		if len(results) > 0 {
			var ldNames, ldInputs, ldOutputs []string
			for _, r := range results {
				ldNames = append(ldNames, r.tc.Name)
				inputJSON, _ := json.Marshal(r.tc.Arguments)
				ldInputs = append(ldInputs, string(inputJSON))
				ldOutputs = append(ldOutputs, r.output)
			}
			loopDet.RecordStep(ldNames, ldInputs, ldOutputs)
		}

		// Sandbox: notify about staged changes after all tools in this turn
		if s.Sandbox != nil && s.Sandbox.IsEnabled() {
			pending := s.Sandbox.List()
			if len(pending) > 0 {
				ch <- StreamEvent{Type: "content", Content: fmt.Sprintf("\n[%d change(s) staged for review]", len(pending))}
			}
		}
	}
}

// toolTimeout returns a per-tool timeout duration based on the tool name.
// Fast file operations get a shorter deadline while Bash gets a longer one.
func toolTimeout(name string) time.Duration {
	switch name {
	case "Read", "Edit", "Write":
		return 30 * time.Second
	case "Bash":
		return 120 * time.Second
	default:
		return 60 * time.Second
	}
}

// isRetryableStreamError checks if a streaming error is transient and worth retrying.
func isRetryableStreamError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "EOF") ||
		strings.Contains(msg, "broken pipe")
}

// shouldRemember returns true if the assistant response contains language that
// suggests a correction, learning, or noteworthy insight worth persisting.
func shouldRemember(content string) bool {
	triggers := []string{"actually", "correction", "instead", "don't", "mistake", "should have", "better approach"}
	lower := strings.ToLower(content)
	for _, t := range triggers {
		if strings.Contains(lower, t) {
			return true
		}
	}
	return false
}
