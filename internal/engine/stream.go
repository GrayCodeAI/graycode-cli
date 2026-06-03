package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/GrayCodeAI/hawk/internal/types"

	"github.com/GrayCodeAI/hawk/internal/engine/branching"
	"github.com/GrayCodeAI/hawk/internal/engine/ctxmgr"
	"github.com/GrayCodeAI/hawk/internal/engine/lifecycle"
	"github.com/GrayCodeAI/hawk/internal/hooks"
	analytics "github.com/GrayCodeAI/hawk/internal/observability"
	"github.com/GrayCodeAI/hawk/internal/observability/oteltrace"
	modelPkg "github.com/GrayCodeAI/hawk/internal/provider/routing"
	"github.com/GrayCodeAI/hawk/internal/resilience/retry"
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

	// Start session-level trace span
	var sessionSpan *oteltrace.Span
	if s.Tracer != nil {
		ctx, sessionSpan = oteltrace.StartSessionSpan(ctx, s.Tracer, fmt.Sprintf("%d", sessionStart.UnixNano()))
		defer oteltrace.EndSpanWithError(sessionSpan, nil)
	}

	// Self-improvement: run OnSessionEnd when the loop exits (regardless of how)
	defer func() {
		success := ctx.Err() == nil
		if s.Lifecycle != nil {
			outcome := SessionOutcome{
				Success:  success,
				Duration: time.Since(sessionStart),
			}
			if len(s.messages) > 0 {
				for _, m := range s.messages {
					if m.Role == "user" && len(m.ToolResults) == 0 && outcome.TaskGoal == "" {
						outcome.TaskGoal = m.Content
					}
				}
			}
			_ = s.Lifecycle.OnSessionEnd(ctx, s, outcome)
		}
		// Enhanced memory: session-end processing (confidence, diff, continuity)
		if s.EnhancedMemory != nil {
			s.EnhancedMemory.EndSession(success)
		}
		// Yaad: save session summary
		if s.Memory != nil {
			taskGoal := ""
			if len(s.messages) > 0 {
				for _, m := range s.messages {
					if m.Role == "user" && len(m.ToolResults) == 0 && taskGoal == "" {
						taskGoal = m.Content
					}
				}
			}
			if taskGoal != "" {
				summary := fmt.Sprintf("Session goal: %s", taskGoal)
				if !success {
					summary += " (interrupted)"
				}
				_ = s.Memory.Remember(summary, "session")
			}
		}

		// Few-shot learning: record successful session patterns
		if success && s.FewShotStore != nil && len(s.messages) >= 2 {
			taskGoal := ""
			response := ""
			for _, m := range s.messages {
				if m.Role == "user" && len(m.ToolResults) == 0 && taskGoal == "" {
					taskGoal = m.Content
				}
				if m.Role == "assistant" && m.Content != "" {
					response = m.Content
				}
			}
			if taskGoal != "" && response != "" {
				s.FewShotStore.Record(taskGoal, response, "general")
			}
		}

		// Adaptive prompt: learn from user corrections in this session
		if s.AdaptivePrompt != nil {
			for _, m := range s.messages {
				if m.Role == "user" && len(m.ToolResults) == 0 {
					s.AdaptivePrompt.LearnFromFeedback(m.Content)
				}
			}
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

	// Few-shot learning: inject relevant examples from past successful sessions
	if s.FewShotStore != nil && len(s.messages) > 0 {
		lastMsg := s.messages[len(s.messages)-1].Content
		if fewShotCtx := s.FewShotStore.FormatForPrompt(lastMsg); fewShotCtx != "" {
			s.AppendSystemContext(fewShotCtx)
		}
	}

	// Adaptive prompt: inject user preferences learned from corrections
	if s.AdaptivePrompt != nil {
		if prefs := s.AdaptivePrompt.FormatForPrompt(); prefs != "" {
			s.AppendSystemContext(prefs)
		}
	}

	// Agents accumulator: inject learnings from previous sessions
	if s.AgentsAccum != nil {
		if learnings := s.AgentsAccum.ForPrompt(5); learnings != "" {
			s.AppendSystemContext(learnings)
		}
	}

	recoveryCount := 0
	turnCount := 0
	toolTurns := 0 // turns that used tools (for skill distillation)
	var toolsUsedSet map[string]bool
	var filesModifiedSet map[string]bool
	snowball := branching.NewSnowballDetector(500000) // 500K token ceiling
	loopDet := NewLoopDetector(10, DoomLoopThreshold) // 10-step window, 3 repeats = doom loop

	for {
		if !s.checkGuardConditions(ctx, ch, turnCount, snowball, loopDet) {
			return
		}
		turnCount++

		if s.Limits != nil {
			s.Limits.RecordTurn()
		}

		// Belief maintenance: prune stale beliefs (injected at query time below)
		if s.Beliefs != nil && s.Beliefs.Size() > 0 {
			s.Beliefs.Prune(turnCount)
		}
		// Auto-compact if conversation is too long (message count)
		if len(s.messages) > maxContextMessages {
			s.messages = ctxmgr.CollapseRepeatedMessages(s.messages)
			if len(s.messages) > maxContextMessages {
				s.smartCompact()
			}
		}

		// Auto-compact if token usage exceeds context budget allocation
		convTokens := EstimateTokens(s.messages)
		if info, ok := modelPkg.Find(s.model); ok && info.ContextSize > 0 {
			budget := ctxmgr.NewContextBudget(info.ContextSize)
			if budget.ShouldCompact(convTokens) {
				s.smartCompact()
			}
		}

		// Integration pipeline: pre-query (intent, tools, budget, injection scan, cache)
		if s.Pipeline != nil {
			lastUserMsg := ""
			for i := len(s.messages) - 1; i >= 0; i-- {
				if s.messages[i].Role == "user" && len(s.messages[i].ToolResults) == 0 {
					lastUserMsg = s.messages[i].Content
					break
				}
			}
			preResult := s.Pipeline.PreQuery(s.messages, lastUserMsg)
			if preResult != nil {
				// Cache hit: short-circuit the LLM call
				if preResult.CacheHit && preResult.CachedResponse != "" {
					ch <- StreamEvent{Type: "content", Content: preResult.CachedResponse}
					s.messages = append(s.messages, types.EyrieMessage{Role: "assistant", Content: preResult.CachedResponse})
					ch <- StreamEvent{Type: "done"}
					return
				}
				// Injection detected: warn but continue
				if preResult.InjectionRisk != nil && preResult.InjectionRisk.IsRisky {
					s.log.Warn("injection risk detected", map[string]interface{}{
						"level": preResult.InjectionRisk.RiskLevel,
					})
				}
				// Apply adaptive system prompt if generated
				if preResult.SystemPrompt != "" {
					s.system = preResult.SystemPrompt
				}
			}
		}

		// Pre-query hook
		_ = hooks.Execute(ctx, hooks.EventPreQuery, map[string]interface{}{
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
		activeModel := strings.TrimSpace(s.model)
		if activeModel == "" {
			activeModel = strings.TrimSpace(s.Cost.Model)
		}
		if s.Cascade != nil && s.Cascade.Enabled {
			lastUserMsg := ""
			for i := len(s.messages) - 1; i >= 0; i-- {
				if s.messages[i].Role == "user" {
					lastUserMsg = s.messages[i].Content
					break
				}
			}
			activeModel = s.Cascade.SelectModel(lastUserMsg, activeModel, "")
		}
		if strings.TrimSpace(activeModel) == "" {
			ch <- StreamEvent{Type: "error", Content: "no model selected — open /config → Models and pick one"}
			return
		}

		// Yaad: recall and refresh memories before every LLM call
		if s.Memory != nil && len(s.messages) > 0 {
			lastMsg := s.messages[len(s.messages)-1].Content
			remembered, err := s.Memory.Recall(lastMsg, 3000)
			if err == nil && remembered != "" {
				s.ReplaceSystemContextSection("## Relevant Memories\n", "## Relevant Memories\n"+remembered)
			}
		}

		opts := types.ChatOptions{
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
		// Plan mode: steer the model to research and propose a plan only, and to
		// call ExitPlanMode for approval before any changes. Ephemeral (not
		// persisted to s.system) so it disappears once build mode resumes.
		if s.Perm != nil && s.Perm.Mode == PermissionModePlan {
			opts.System += planModeSystemPrompt
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

		// Circuit breaker: select provider with failover (legacy single-provider clients only).
		if s.Router != nil && !s.DeploymentRouting {
			if selectedProvider, err := s.Router.SelectProvider(s.provider); err == nil && selectedProvider != s.provider {
				s.log.Info("provider failover", map[string]interface{}{"from": s.provider, "to": selectedProvider})
				opts.Provider = selectedProvider
			}
		}

		// Count actual input tokens for precise budget tracking
		inputTokens := 0
		for _, msg := range s.messages {
			inputTokens += CountTokensFast(msg.Content)
			for _, tr := range msg.ToolResults {
				inputTokens += CountTokensFast(tr.Content)
			}
		}
		inputTokens += CountTokensFast(s.system)
		s.log.Info("token count", map[string]interface{}{"input_tokens": inputTokens, "model": s.model})

		// Cost warning for expensive calls
		if inPrice, outPrice := ModelPricing(s.model); true {
			estCost := float64(inputTokens)*inPrice/1_000_000 + float64(maxTok)*outPrice/1_000_000
			if estCost > 0.50 {
				ch <- StreamEvent{Type: "content", Content: fmt.Sprintf("\n⚠ This request will use ~%d tokens (~$%.2f). Continue? The agent will proceed automatically.\n", inputTokens+maxTok, estCost)}
			}
		}

		var result *types.StreamResult
		var err error

		// Use retry for transient errors
		retryCfg := retry.DefaultConfig()
		retryCfg.MaxRetries = 2
		retryCfg.BaseDelay = 500 * time.Millisecond

		s.metrics.Counter("api.requests").Inc()
		apiStart := time.Now()

		// Trace: start agent loop span for this turn
		var loopSpan *oteltrace.Span
		if s.Tracer != nil {
			ctx, loopSpan = oteltrace.StartAgentLoopSpan(ctx, s.Tracer, s.provider, activeModel, len(s.messages))
		}

		// Rate limit: wait for a token before making the LLM call
		if s.RateLimiter != nil {
			if err := s.RateLimiter.Wait(ctx); err != nil {
				ch <- StreamEvent{Type: "error", Content: err.Error()}
				return
			}
		}

		contCfg := types.DefaultContinuationConfig()
		err = retry.Do(ctx, retryCfg, func() error {
			result, err = s.client.StreamChatContinue(ctx, s.messages, opts, contCfg)
			if err != nil {
				if strings.Contains(err.Error(), "too long") || strings.Contains(err.Error(), "too many tokens") {
					s.compact()
					result, err = s.client.StreamChatContinue(ctx, s.messages, opts, contCfg)
				}
			}
			return err
		})

		apiDuration := time.Since(apiStart)
		s.metrics.Timer("api.latency").Record(apiDuration)
		s.metrics.Timer("api.last_latency").Record(apiDuration)

		if err != nil {
			// End trace span with error
			if loopSpan != nil {
				oteltrace.EndSpanWithError(loopSpan, err)
			}
			// Record failure for circuit breaker
			if s.Router != nil && !s.DeploymentRouting {
				s.Router.RecordFailure(s.provider, err)
			}
			s.log.Error("stream error", map[string]interface{}{
				"error": err.Error(),
			})
			ch <- StreamEvent{Type: "error", Content: err.Error()}
			return
		}

		// Record success for circuit breaker
		if s.Router != nil && !s.DeploymentRouting {
			s.Router.RecordSuccess(s.provider, apiDuration)
		}

		var textContent strings.Builder
		var toolCalls []types.ToolCall
		var stopReason string
		var lastUsage *types.EyrieUsage

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
					textContent.WriteString(ev.Content)
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
							inPrice, outPrice := ModelPricing(activeModel)
							cost := float64(ev.Usage.PromptTokens)*inPrice/1_000_000 + float64(ev.Usage.CompletionTokens)*outPrice/1_000_000
							_ = s.CostTracker.Record(analytics.CostEntry{
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

			// Notify consumer to discard previously streamed content for this turn.
			// The consumer should treat content before this event as stale.
			ch <- StreamEvent{Type: "retry", Content: fmt.Sprintf("retrying (attempt %d)", streamAttempt+2)}

			// Re-open the stream for retry
			result, err = s.client.StreamChatContinue(ctx, s.messages, opts, contCfg)
			if err != nil {
				ch <- StreamEvent{Type: "error", Content: err.Error()}
				return
			}
			// Reset accumulated state for the retry
			textContent.Reset()
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
		if len(toolCalls) == 0 && strings.Contains(textContent.String(), "<|tool_calls_section_begin|>") {
			cleanText, inlineCalls := types.ParseInlineToolCalls(textContent.String())
			if len(inlineCalls) > 0 {
				textContent.Reset()
				textContent.WriteString(cleanText)
				toolCalls = append(toolCalls, inlineCalls...)
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
			"content":  textContent.String(),
			"tools":    len(toolCalls),
		})

		// End agent loop trace span for this turn
		if loopSpan != nil {
			loopSpan.SetTag("tools", fmt.Sprintf("%d", len(toolCalls)))
			oteltrace.EndSpanWithError(loopSpan, nil)
		}

		// Activity nudge: remind agent to persist learnings if idle
		if s.Activity != nil {
			if nudge := s.Activity.NudgeMessage(); nudge != "" {
				s.AppendSystemContext(nudge)
			}
		}

		// Handle max_tokens recovery
		if stopReason == "max_tokens" && len(toolCalls) == 0 && recoveryCount < maxRecoveryRetries {
			recoveryCount++
			s.messages = append(s.messages, types.EyrieMessage{Role: "assistant", Content: textContent.String()})
			s.messages = append(s.messages, types.EyrieMessage{Role: "user", Content: "Continue from where you left off."})
			continue
		}

		// No tool calls — done
		if len(toolCalls) == 0 {
			// Integration pipeline: post-response (format, score, redact, cache, learn)
			if s.Pipeline != nil && textContent.Len() > 0 {
				postResult := s.Pipeline.PostResponse(textContent.String(), s.messages)
				if postResult != nil && postResult.FormattedResponse != "" {
					textContent.Reset()
					textContent.WriteString(postResult.FormattedResponse)
				}
			}
			if textContent.Len() > 0 {
				s.messages = append(s.messages, types.EyrieMessage{Role: "assistant", Content: textContent.String()})
				// Auto-remember corrections and learnings
				if s.Memory != nil && shouldRemember(textContent.String()) {
					go func(content string) {
						// Use timeout context so goroutine doesn't hang if backend is slow.
						rCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
						defer cancel()
						_ = s.Memory.Remember(content, "assistant_learning")
						_ = rCtx // timeout context available if Remember is extended to accept it
					}(textContent.String())
				}
			}
			// Sleeptime: background memory consolidation
			if s.Sleeptime != nil && s.Sleeptime.ShouldRun() && s.YaadBridge != nil && s.YaadBridge.Ready() {
				// Snapshot messages to avoid data race with main loop appending
				msgs := make([]types.EyrieMessage, len(s.messages))
				copy(msgs, s.messages)
				go func() {
					var transcript []string
					for _, m := range msgs {
						transcript = append(transcript, m.Role+": "+m.Content)
					}
					memState := ""
					if s.Memory != nil {
						memState, _ = s.Memory.Recall("", 2000)
					}
					prompt := s.Sleeptime.BuildConsolidationPrompt(transcript, memState)
					// Use timeout context to prevent goroutine leak if LLM hangs
					sCtx, sCancel := context.WithTimeout(context.Background(), 2*time.Minute)
					defer sCancel()
					resp, err := s.client.Chat(sCtx, []types.EyrieMessage{
						{Role: "user", Content: prompt},
					}, types.ChatOptions{Provider: s.provider, Model: s.model, MaxTokens: 2048})
					if err != nil || resp == nil {
						return
					}
					lifecycle.ParseAndApplyMemoryOps(s.YaadBridge, resp.Content)
				}()
			}
			// Skill distillation: extract reusable skill from multi-turn tasks
			if s.SkillDistiller != nil && toolTurns >= 5 && s.YaadBridge != nil && s.YaadBridge.Ready() {
				// Snapshot messages to avoid data race with main loop appending
				msgs := make([]types.EyrieMessage, len(s.messages))
				copy(msgs, s.messages)
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
					if len(msgs) > 0 {
						taskDesc = msgs[0].Content
					}
					sd := s.SkillDistiller
					prompt := sd.BuildSkillPrompt(taskDesc, tools, files, textContent.String())
					// Use timeout context to prevent goroutine leak if LLM hangs
					dCtx, dCancel := context.WithTimeout(context.Background(), 2*time.Minute)
					defer dCancel()
					resp, err := s.client.Chat(dCtx, []types.EyrieMessage{
						{Role: "user", Content: prompt},
					}, types.ChatOptions{Provider: s.provider, Model: s.model, MaxTokens: 2048})
					if err != nil || resp == nil {
						return
					}
					skill, err := sd.ParseSkill(resp.Content)
					if err != nil {
						return
					}
					content, _ := json.Marshal(skill)
					_ = s.YaadBridge.Remember(string(content), "skill")
				}()
			}
			ch <- StreamEvent{Type: "done"}
			// Integration pipeline: end-session (assess, learn, store experience)
			if s.Pipeline != nil {
				taskGoal := ""
				for _, m := range s.messages {
					if m.Role == "user" && len(m.ToolResults) == 0 {
						taskGoal = m.Content
						break
					}
				}
				go s.Pipeline.EndSession(ctx.Err() == nil, taskGoal)
			}
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

		// Backtrack: record decision point when tool calls are pending
		if s.Backtrack != nil && len(toolCalls) > 0 {
			var toolNames []string
			for _, tc := range toolCalls {
				toolNames = append(toolNames, tc.Name)
			}
			s.Backtrack.RecordDecision(turnCount, strings.Join(toolNames, ", "), nil, s.messages)
		}

		results := s.executeToolCalls(ctx, toolCalls, ch, turnCount, textContent.String())

		// Auto-snapshot after write operations for granular undo
		if s.Snapshots != nil && len(toolCalls) > 0 {
			var writeNames []string
			safeConcurrent := map[string]bool{"Read": true, "Grep": true, "Glob": true, "LS": true, "WebSearch": true, "WebFetch": true, "ToolSearch": true}
			for _, tc := range toolCalls {
				if !safeConcurrent[tc.Name] {
					writeNames = append(writeNames, tc.Name)
				}
			}
			if len(writeNames) > 0 {
				go func() { _, _ = s.Snapshots.Track(strings.Join(writeNames, ", ")) }()
			}
		}

		// Append assistant message with tool_use blocks
		assistContent := textContent.String()
		if assistContent == "" && len(toolCalls) > 0 {
			assistContent = " " // non-empty to satisfy APIs that reject empty content
		}
		s.messages = append(s.messages, types.EyrieMessage{
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
			msg := types.EyrieMessage{
				Role:    "user",
				Content: resultContent,
				ToolResults: []types.ToolResult{{
					ToolUseID: r.tc.ID,
					Content:   resultContent,
					IsError:   r.isErr,
				}},
			}
			// Multi-modal vision: extract data URIs from tool results and attach as images
			if !r.isErr && strings.Contains(resultContent, "data:") && strings.Contains(resultContent, ";base64,") {
				if imgURI := extractDataURI(resultContent); imgURI != "" {
					msg.Images = []string{imgURI}
				}
			}
			s.messages = append(s.messages, msg)
		}

		// --- STEERING: Inject user guidance between tool batches ---
		if s.Steering != nil && s.Steering.HasPending() {
			for _, steer := range s.Steering.Drain() {
				s.messages = append(s.messages, types.EyrieMessage{
					Role:    "user",
					Content: "[User guidance during execution]: " + steer.Content,
				})
				ch <- StreamEvent{Type: "content", Content: "\n[Steering received: " + steer.Content + "]\n"}
			}
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

		// Auto-remember: save conversation context and insights to yaad after each turn
		if s.Memory != nil {
			userMsg := ""
			assistantMsg := ""
			for i := len(s.messages) - 1; i >= 0; i-- {
				m := s.messages[i]
				if m.Role == "assistant" && m.Content != "" && assistantMsg == "" {
					assistantMsg = m.Content
				}
				if m.Role == "user" && len(m.ToolResults) == 0 && userMsg == "" {
					userMsg = m.Content
				}
				if userMsg != "" && assistantMsg != "" {
					break
				}
			}
			if userMsg != "" && assistantMsg != "" {
				condensed := fmt.Sprintf("Q: %s\nA: %s", truncate(userMsg, 200), truncate(assistantMsg, 300))
				_ = s.Memory.Remember(condensed, "conversation")
			}
			// Also save insights if the response has learning signals
			if assistantMsg != "" && shouldRemember(assistantMsg) {
				_ = s.Memory.Remember(truncate(assistantMsg, 500), "insight")
			}
		}
	}
}

// extractDataURI extracts the first base64 data URI from a string.
// Returns the full data URI (e.g., "data:image/png;base64,...") or empty string.
func extractDataURI(s string) string {
	idx := strings.Index(s, "data:")
	if idx < 0 {
		return ""
	}
	rest := s[idx:]
	endIdx := strings.IndexAny(rest, " \n\t\"'")
	if endIdx < 0 {
		return rest
	}
	return rest[:endIdx]
}
