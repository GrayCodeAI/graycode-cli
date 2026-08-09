package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/GrayCodeAI/hawk/internal/provider/gateway"
	"github.com/GrayCodeAI/hawk/internal/types"

	"github.com/GrayCodeAI/hawk/internal/engine/branching"
	"github.com/GrayCodeAI/hawk/internal/engine/lifecycle"
	"github.com/GrayCodeAI/hawk/internal/hooks"
	"github.com/GrayCodeAI/hawk/internal/observability/oteltrace"
	"github.com/GrayCodeAI/hawk/internal/plugin"
	"github.com/GrayCodeAI/hawk/internal/tool"

	"github.com/GrayCodeAI/hawk/internal/ui/icons"
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
	if s.TracerValue() != nil {
		ctx, sessionSpan = oteltrace.StartSessionSpan(ctx, s.TracerValue(), fmt.Sprintf("%d", sessionStart.UnixNano()))
		defer oteltrace.EndSpanWithError(sessionSpan, nil)
	}

	// Lifecycle and memory bookkeeping consume immutable service snapshots,
	// keeping the agent loop independent of backend-specific state.
	defer func() {
		success := ctx.Err() == nil
		messages := s.Persistence().Messages()
		s.LifecycleSvc().Finalize(ctx, messages, success, time.Since(sessionStart), s.CostValue().TotalUSD())
		s.MemorySvc().Finalize(messages, success)
	}()

	// Session start hook
	hooks.ExecuteAsync(ctx, hooks.EventSessionStart, map[string]interface{}{
		"provider": s.ChatLLM().Provider(),
		"model":    s.ChatLLM().Model(),
	})

	// Self-improvement: inject learned guidelines and skills from prior sessions
	if s.LifecycleSvc().Lifecycle() != nil && len(s.Persistence().RawMessages()) > 0 {
		lastMsg := s.Persistence().RawMessages()[len(s.Persistence().RawMessages())-1].Content
		if learnedCtx := s.LifecycleSvc().StartContext(ctx, lastMsg); learnedCtx != "" {
			s.AppendSystemContext(learnedCtx)
		}
	}

	// Inject remembered context through the memory service boundary.
	if len(s.Persistence().RawMessages()) > 0 {
		lastMsg := s.Persistence().RawMessages()[len(s.Persistence().RawMessages())-1].Content
		if remembered := s.MemorySvc().RecallContext(ctx, lastMsg, 2000); remembered != "" {
			s.AppendSystemContext(remembered)
		}
	}

	// Few-shot learning: inject relevant examples from past successful sessions
	if s.LifecycleSvc().FewShotStore() != nil && len(s.Persistence().RawMessages()) > 0 {
		lastMsg := s.Persistence().RawMessages()[len(s.Persistence().RawMessages())-1].Content
		if fewShotCtx := s.LifecycleSvc().FewShotStore().FormatForPrompt(lastMsg); fewShotCtx != "" {
			s.AppendSystemContext(fewShotCtx)
		}
	}

	// Adaptive prompt: inject user preferences learned from corrections
	if s.LifecycleSvc().AdaptivePrompt() != nil {
		if prefs := s.LifecycleSvc().AdaptivePrompt().FormatForPrompt(); prefs != "" {
			s.AppendSystemContext(prefs)
		}
	}

	// Agents accumulator: inject learnings from previous sessions
	if s.LifecycleSvc().AgentsAccum() != nil {
		if learnings := s.LifecycleSvc().AgentsAccum().ForPrompt(5); learnings != "" {
			s.AppendSystemContext(learnings)
		}
	}

	// Auto-skill: load smart skills once at session start for per-turn matching
	s.LifecycleSvc().LoadSmartSkills()

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

		if s.LifecycleSvc().Limits() != nil {
			s.LifecycleSvc().Limits().RecordTurn()
		}

		// Belief maintenance: prune stale beliefs (injected at query time below)
		if s.LifecycleSvc().Beliefs() != nil && s.LifecycleSvc().Beliefs().Size() > 0 {
			s.LifecycleSvc().Beliefs().Prune(turnCount)
		}
		// Context governor: collapse → micro/smart/truncate (settings threshold %).
		tokensBefore := EstimateTokens(s.Persistence().RawMessages())
		if s.WillCompactBeforeTurn() {
			ch <- StreamEvent{Type: "compact_start"}
		}
		if compactStrategy, didCompact := s.ManageContextBeforeTurn(ctx); didCompact {
			tokensAfter := EstimateTokens(s.Persistence().RawMessages())
			s.Logger().Info("context compacted", map[string]interface{}{
				"strategy": compactStrategy,
				"messages": len(s.Persistence().RawMessages()),
			})
			ch <- StreamEvent{
				Type:         "compact",
				Content:      compactStrategy,
				TokensBefore: tokensBefore,
				TokensAfter:  tokensAfter,
			}
		}

		// Integration pipeline: pre-query (intent, tools, budget, injection scan, cache)
		if s.LifecycleSvc().Pipeline() != nil {
			lastUserMsg := ""
			for i := len(s.Persistence().RawMessages()) - 1; i >= 0; i-- {
				if s.Persistence().RawMessages()[i].Role == "user" && len(s.Persistence().RawMessages()[i].ToolResults) == 0 {
					lastUserMsg = s.Persistence().RawMessages()[i].Content
					break
				}
			}
			preResult := s.LifecycleSvc().Pipeline().PreQuery(s.Persistence().RawMessages(), lastUserMsg)
			if preResult != nil {
				// Cache hit: short-circuit the LLM call
				if preResult.CacheHit && preResult.CachedResponse != "" {
					ch <- StreamEvent{Type: "content", Content: preResult.CachedResponse}
					s.Persistence().SetRawMessages(append(s.Persistence().RawMessages(), types.EyrieMessage{Role: "assistant", Content: preResult.CachedResponse}))
					ch <- StreamEvent{Type: "done"}
					return
				}
				if preResult.InjectionRisk != nil && preResult.InjectionRisk.IsRisky {
					if preResult.InjectionRisk.RiskLevel == "high" {
						ch <- StreamEvent{Type: "error", Content: "High-risk prompt injection detected. Message blocked."}
						return
					}
					s.Logger().Warn("injection risk detected", map[string]interface{}{
						"level": preResult.InjectionRisk.RiskLevel,
					})
				}
				// Apply adaptive system prompt if generated
				if preResult.SystemPrompt != "" {
					s.Persistence().SetSystem(preResult.SystemPrompt)
				}
			}
		}

		// Pre-query hook
		_ = hooks.Execute(ctx, hooks.EventPreQuery, map[string]interface{}{
			"provider": s.ChatLLM().Provider(),
			"model":    s.ChatLLM().Model(),
			"messages": len(s.Persistence().RawMessages()),
		})

		s.Logger().Info("stream query", map[string]interface{}{
			"provider": s.ChatLLM().Provider(),
			"model":    s.ChatLLM().Model(),
			"messages": len(s.Persistence().RawMessages()),
		})

		// Dynamic max_tokens based on task type and recent tool patterns
		taskType := classifyPromptForBudget(s.Persistence().RawMessages())
		contextSize := s.ContextWindowSize()
		maxTok := DynamicMaxTokens(s.Persistence().RawMessages(), contextSize, taskType)

		// Model cascade: select optimal model for this request
		activeModel := strings.TrimSpace(s.ChatLLM().Model())
		if activeModel == "" {
			activeModel = strings.TrimSpace(s.ChatLLM().Model())
		}
		if s.LifecycleSvc().Cascade() != nil && s.LifecycleSvc().Cascade().Enabled {
			lastUserMsg := ""
			for i := len(s.Persistence().RawMessages()) - 1; i >= 0; i-- {
				if s.Persistence().RawMessages()[i].Role == "user" {
					lastUserMsg = s.Persistence().RawMessages()[i].Content
					break
				}
			}
			activeModel = s.LifecycleSvc().Cascade().SelectModel(lastUserMsg, activeModel, "")
		}
		if strings.TrimSpace(activeModel) == "" {
			ch <- StreamEvent{Type: "error", Content: "no model selected — open /config → Models and pick one"}
			return
		}

		// Recall and refresh memories before every LLM call through the
		// memory service boundary.
		if len(s.Persistence().RawMessages()) > 0 {
			lastMsg := s.Persistence().RawMessages()[len(s.Persistence().RawMessages())-1].Content
			if remembered := s.MemorySvc().RecallContext(ctx, lastMsg, 3000); remembered != "" {
				s.ReplaceSystemContextSection("## Relevant Memories\n", remembered)
			}
		}

		// Build the LLM ChatOptions via the ChatService. The service owns
		// the GLMThinking toggle, output schema, anthropic caching flag,
		// and the active provider/model — building opts manually here
		// would duplicate that logic.
		baseOpts := s.ChatLLM().BuildOptions(s.Persistence().System(), activeModel, maxTok, nil)
		opts := baseOpts
		// Inject beliefs as ephemeral context (not persisted to s.Persistence().System())
		if s.LifecycleSvc().Beliefs() != nil && s.LifecycleSvc().Beliefs().Size() > 0 {
			if summary := s.LifecycleSvc().Beliefs().FormatForPrompt(); summary != "" {
				opts.System += "\n\n## Agent Beliefs\n" + summary
			}
		}
		// Auto-skill: match smart skills against the last user message and
		// inject a compact listing. The LLM uses the Skill tool for full content.
		smartSkills := s.LifecycleSvc().SmartSkills()
		if len(smartSkills) > 0 {
			lastUserMsg := ""
			for i := len(s.Persistence().RawMessages()) - 1; i >= 0; i-- {
				if s.Persistence().RawMessages()[i].Role == "user" && len(s.Persistence().RawMessages()[i].ToolResults) == 0 {
					lastUserMsg = s.Persistence().RawMessages()[i].Content
					break
				}
			}
			if lastUserMsg != "" {
				if matched := plugin.MatchSkillsByContext(smartSkills, lastUserMsg); len(matched) > 0 {
					if skillsPrompt := plugin.FormatSkillsCompact(matched); skillsPrompt != "" {
						opts.System += "\n\n" + skillsPrompt
					}
				}
			}
		}
		// Spec stage: steer the model through Specify -> Plan -> Tasks and an
		// explicit approval handoff before any changes. Ephemeral (not
		// persisted to s.Persistence().System()) so it disappears once the
		// stage advances to Implementing.
		if stage := s.PermSvc().SpecStage(); stage != SpecStageNone && stage != SpecStageImplementing {
			opts.System += specStageSystemPrompt
			// Inject project constitution as governing principles
			if constitution := constitutionForPrompt(s.PermSvc().SpecSlug()); constitution != "" {
				opts.System += constitution
			}
			// Inject user's spec configuration (language, framework, etc.)
			// as context so the model writes specs that match preferences.
			if cfgPrompt := specConfigForPrompt(); cfgPrompt != "" {
				opts.System += cfgPrompt
			}
		}
		// Work mode (plan/act/review) — ephemeral product control plane.
		if addon := s.workModeSystemAddon(); addon != "" {
			opts.System += "\n\n" + addon
		}
		if s.Tools() != nil && s.Tools().Registry() != nil {
			// Promote only the small set of registered tools that match the
			// current request. This keeps the default schema compact while
			// making URL, verification, git, and code-intelligence requests
			// discoverable without requiring the model to guess ToolSearch.
			lastUserMsg := ""
			for i := len(s.Persistence().RawMessages()) - 1; i >= 0; i-- {
				msg := s.Persistence().RawMessages()[i]
				if msg.Role == "user" && len(msg.ToolResults) == 0 {
					lastUserMsg = msg.Content
					break
				}
			}
			if lastUserMsg != "" {
				s.Tools().Registry().PromoteForIntent(lastUserMsg)
			}
			opts.Tools = s.Tools().Registry().EyrieTools()
		}

		// Inject memory metadata from yaad
		if s.MemorySvc().Yaad() != nil && s.MemorySvc().Yaad().Ready() {
			if _, contents, err := s.MemorySvc().Yaad().SearchByType("convention", 100); err == nil {
				convCount := len(contents)
				if _, dContents, err := s.MemorySvc().Yaad().SearchByType("decision", 100); err == nil {
					decCount := len(dContents)
					total := convCount + decCount
					if total > 0 {
						opts.System += fmt.Sprintf("\n\nMemory: %d nodes (%d conventions, %d decisions)", total, convCount, decCount)
					}
				}
			}
		}

		// Count actual input tokens for precise budget tracking
		inputTokens := 0
		for _, msg := range s.Persistence().RawMessages() {
			inputTokens += CountTokensFast(msg.Content)
			for _, tr := range msg.ToolResults {
				inputTokens += CountTokensFast(tr.Content)
			}
		}
		inputTokens += CountTokensFast(s.Persistence().System())
		s.Logger().Info("token count", map[string]interface{}{"input_tokens": inputTokens, "model": s.ChatLLM().Model()})

		// Cost warning for expensive calls
		inPrice, outPrice := ModelPricing(s.ChatLLM().Model())
		estCost := float64(inputTokens)*inPrice/1_000_000 + float64(maxTok)*outPrice/1_000_000
		if estCost > 0.50 {
			ch <- StreamEvent{Type: "blast_radius", Content: fmt.Sprintf("%s This request will use ~%d tokens (~$%.2f). Continue? The agent will proceed automatically.", icons.Alert(), inputTokens+maxTok, estCost)}
		}

		// Trace: start agent loop span for this turn
		var loopSpan *oteltrace.Span
		if s.TracerValue() != nil {
			ctx, loopSpan = oteltrace.StartAgentLoopSpan(ctx, s.TracerValue(), s.ChatLLM().Provider(), activeModel, len(s.Persistence().RawMessages()))
		}

		// Issue the LLM call via the ChatService. The service handles
		// rate limit, retry, and emergency compact internally; the
		// api.requests counter is incremented inside ChatService.Stream.
		// Hawk records product-level latency; provider health and circuit
		// breaking are owned by Eyrie's routed transport.
		apiStart := time.Now()
		managesResilience := clientManagesResilience(s.ChatLLM().Client())
		result, err := s.ChatLLM().Stream(ctx, s.Persistence().RawMessages(), opts)
		apiDuration := time.Since(apiStart)
		s.Metrics().Timer("api.latency").Record(apiDuration)
		s.Metrics().Timer("api.last_latency").Record(apiDuration)

		if err != nil {
			// End trace span with error
			if loopSpan != nil {
				oteltrace.EndSpanWithError(loopSpan, err)
			}
			s.Logger().Error("stream error", map[string]interface{}{
				"error": err.Error(),
			})
			ch <- StreamEvent{Type: "error", Content: err.Error()}
			return
		}

		var textContent strings.Builder
		var toolCalls []types.ToolCall
		var stopReason string
		var lastUsage *types.EyrieUsage
		var usageLedger streamUsageLedger
		resolvedProvider := strings.TrimSpace(s.ChatLLM().Provider())
		resolvedModel := strings.TrimSpace(activeModel)

		// Compatibility clients retain Hawk's historical stream retry and
		// reasoning-only recovery. Eyrie facade clients already normalize and
		// recover provider streams, so Hawk must consume their result exactly once.
		const maxStreamRetries = 2
		var streamErr error
		var sawThinking bool
		for streamAttempt := 0; streamAttempt <= maxStreamRetries; streamAttempt++ {
			streamErr = nil
			sawThinking = false
		eventLoop:
			for {
				select {
				case <-ctx.Done():
					result.Close()
					return
				case ev, ok := <-result.Events:
					if !ok {
						break eventLoop
					}
					switch ev.Type {
					case "route_selected", "route_changed":
						var changed bool
						resolvedProvider, resolvedModel, changed = updateResolvedRoute(resolvedProvider, resolvedModel, ev.Route)
						if changed {
							s.Logger().Info("engine route selected", map[string]interface{}{
								"provider": resolvedProvider,
								"model":    resolvedModel,
							})
						}
						if loopSpan != nil {
							loopSpan.SetTag("provider", resolvedProvider)
							loopSpan.SetTag("model", resolvedModel)
						}
					case "content":
						textContent.WriteString(ev.Content)
						ch <- StreamEvent{Type: "content", Content: ev.Content}
					case "thinking":
						if strings.TrimSpace(ev.Thinking) != "" {
							sawThinking = true
						}
						ch <- StreamEvent{Type: "thinking", Content: ev.Thinking}
					case "tool_call":
						if ev.ToolCall != nil {
							toolCalls = append(toolCalls, *ev.ToolCall)
						}
					case "usage":
						if ev.Usage != nil {
							lastUsage = ev.Usage
							if usageLedger.shouldRecord(ev.Usage, false) {
								s.recordStreamUsage(ch, ev.Usage.PromptTokens, ev.Usage.CompletionTokens, resolvedProvider, resolvedModel, taskType, apiStart)
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
						if ev.Usage != nil {
							lastUsage = ev.Usage
							if usageLedger.shouldRecord(ev.Usage, true) {
								s.recordStreamUsage(ch, ev.Usage.PromptTokens, ev.Usage.CompletionTokens, resolvedProvider, resolvedModel, taskType, apiStart)
							}
						}
					}
				}
			}
			result.Close()

			thinkingOnly := streamErr == nil && textContent.Len() == 0 && len(toolCalls) == 0 && sawThinking
			if thinkingOnly && !managesResilience {
				if resp, chatErr := s.ChatLLM().Chat(ctx, s.Persistence().RawMessages(), opts); chatErr == nil && resp != nil && strings.TrimSpace(resp.Content) != "" {
					resolvedProvider, resolvedModel, _ = updateResolvedRoute(resolvedProvider, resolvedModel, resp.Route)
					content := resp.Content
					textContent.WriteString(content)
					ch <- StreamEvent{Type: "content", Content: content}
					if len(resp.ToolCalls) > 0 {
						toolCalls = append(toolCalls, resp.ToolCalls...)
					}
					if resp.FinishReason != "" {
						stopReason = resp.FinishReason
					}
					if resp.Usage != nil {
						lastUsage = resp.Usage
						s.recordStreamUsage(ch, resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resolvedProvider, resolvedModel, taskType, apiStart)
					}
					streamErr = nil
					break
				}
				ch <- StreamEvent{Type: "error", Content: "The model produced internal reasoning but no reply."}
				result.Close()
				return
			}

			shouldRetry := !managesResilience && streamErr != nil && isRetryableStreamError(streamErr)
			if !shouldRetry {
				break
			}
			if streamAttempt >= maxStreamRetries {
				break
			}
			retryReason := "transient stream error"
			s.Logger().Warn("stream retry", map[string]interface{}{
				"attempt": streamAttempt + 1,
				"reason":  retryReason,
				"error":   streamErr.Error(),
			})
			retryTimer := time.NewTimer(streamRetryDelay(streamErr, streamAttempt))
			select {
			case <-retryTimer.C:
			case <-ctx.Done():
				retryTimer.Stop()
				ch <- StreamEvent{Type: "error", Content: "stream retry cancelled: " + ctx.Err().Error()}
				result.Close()
				return
			}

			// Notify consumer to discard previously streamed content for this turn.
			ch <- StreamEvent{Type: "retry", Content: fmt.Sprintf("retrying after %s (attempt %d)", retryReason, streamAttempt+2)}

			// Re-open the stream for retry. We bypass the ChatService
			// here on purpose: ChatService.Stream has its own retry
			// loop, and stacking that on top of this secondary
			// stream-error retry would double-retry network blips.
			// The session agent loop owns this layer.
			result, err = s.ChatLLM().Client().StreamChatContinue(ctx, s.Persistence().RawMessages(), opts, types.DefaultContinuationConfig())
			if err != nil {
				ch <- StreamEvent{Type: "error", Content: err.Error()}
				return
			}
			// Reset accumulated state for the retry
			textContent.Reset()
			toolCalls = nil
			stopReason = ""
			lastUsage = nil
			usageLedger.reset()
			streamErr = nil
		}
		if streamErr != nil {
			ch <- StreamEvent{Type: "error", Content: streamErr.Error()}
			return
		}

		// Providers like OpenCode Go often omit stream usage; estimate so billing footer updates.
		if lastUsage == nil && (textContent.Len() > 0 || len(toolCalls) > 0) {
			completionEst := estimateStreamCompletionTokens(textContent.String(), toolCalls)
			if inputTokens > 0 || completionEst > 0 {
				s.recordStreamUsage(ch, inputTokens, completionEst, resolvedProvider, resolvedModel, taskType, apiStart)
				lastUsage = &types.EyrieUsage{
					PromptTokens:     inputTokens,
					CompletionTokens: completionEst,
				}
			}
		}

		// Snowball detector: record usage after each API response
		if lastUsage != nil {
			progress := 0.5
			if len(toolCalls) > 0 {
				progress = 1.0
			}
			snowball.RecordTurn(lastUsage.PromptTokens+lastUsage.CompletionTokens, progress)
			s.recordEyrieOperationObservation(
				resolvedProvider,
				resolvedModel,
				stopReason,
				textContent.String(),
				len(toolCalls),
				lastUsage,
			)
		}

		// Budget enforcement
		limits := s.LifecycleSvc().Limits()
		// Sync the tracker's cost accounting with the session's authoritative
		// cost accumulator so LimitTracker.IsExceeded() (checked every turn by
		// checkGuardConditions) enforces the same budget as this explicit check.
		limits.SetCostUSD(s.CostValue().TotalUSD())
		if limits.MaxBudgetUSD() > 0 && s.CostValue().TotalUSD() >= limits.MaxBudgetUSD() {
			ch <- StreamEvent{Type: "content", Content: fmt.Sprintf("\n\nBudget limit reached ($%.2f spent of $%.2f).", s.CostValue().TotalUSD(), limits.MaxBudgetUSD())}
			ch <- StreamEvent{Type: "done"}
			return
		}

		// Check for inline tool calls in text (some providers embed tool calls in
		// text): Moonshot/kimi <|tool_calls_section_begin|> or Hermes/Nous
		// <tool_call> (Qwen and most OpenAI-compatible local models).
		if len(toolCalls) == 0 && (strings.Contains(textContent.String(), "<|tool_calls_section_begin|>") || strings.Contains(textContent.String(), "<tool_call>")) {
			cleanText, engineCalls := gateway.ParseInlineToolCalls(textContent.String())
			inlineCalls := engineCalls
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
			"provider": resolvedProvider,
			"model":    resolvedModel,
			"content":  textContent.String(),
			"tools":    len(toolCalls),
		})

		// End agent loop trace span for this turn
		if loopSpan != nil {
			loopSpan.SetTag("tools", fmt.Sprintf("%d", len(toolCalls)))
			oteltrace.EndSpanWithError(loopSpan, nil)
		}

		// Activity nudge: remind agent to persist learnings if idle
		if s.MemorySvc().Activity() != nil {
			if nudge := s.MemorySvc().Activity().NudgeMessage(); nudge != "" {
				s.AppendSystemContext(nudge)
			}
		}

		// Compatibility-only max_tokens recovery. Eyrie's engine facade owns
		// continuation and exposes one normalized stream to Hawk. Legacy clients
		// retain the historical synthetic turn so injected integrations do not
		// change behavior while they migrate to the facade.
		if !managesResilience && stopReason == "max_tokens" && len(toolCalls) == 0 && recoveryCount < maxRecoveryRetries {
			recoveryCount++
			s.Persistence().SetRawMessages(append(s.Persistence().RawMessages(), types.EyrieMessage{Role: "assistant", Content: textContent.String()}))
			s.Persistence().SetRawMessages(append(s.Persistence().RawMessages(), types.EyrieMessage{Role: "user", Content: "Continue from where you left off."}))
			continue
		}

		// No tool calls — done
		if len(toolCalls) == 0 {
			// Integration pipeline: post-response (format, score, redact, cache, learn)
			if s.LifecycleSvc().Pipeline() != nil && textContent.Len() > 0 {
				postResult := s.LifecycleSvc().Pipeline().PostResponse(textContent.String(), s.Persistence().RawMessages())
				if postResult != nil {
					s.recordTokRedactionObservation(textContent.String(), postResult.SecretMatches, postResult.SecretTypes)
				}
				if postResult != nil && postResult.FormattedResponse != "" {
					textContent.Reset()
					textContent.WriteString(postResult.FormattedResponse)
				}
			}
			if textContent.Len() > 0 {
				s.Persistence().SetRawMessages(append(s.Persistence().RawMessages(), types.EyrieMessage{Role: "assistant", Content: textContent.String()}))
				// Auto-remember corrections and learnings
				if s.MemorySvc().Memory() != nil && shouldRemember(textContent.String()) {
					go func(content string) {
						// Use timeout context so goroutine doesn't hang if backend is slow.
						rCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
						defer cancel()
						_ = s.MemorySvc().Memory().Remember(content, "assistant_learning")
						_ = rCtx // timeout context available if Remember is extended to accept it
					}(textContent.String())
				}
			}
			// Sleeptime: background memory consolidation
			if s.MemorySvc().Sleeptime() != nil && s.MemorySvc().Sleeptime().ShouldRun() && s.MemorySvc().Yaad() != nil && s.MemorySvc().Yaad().Ready() {
				// Snapshot messages to avoid data race with main loop appending
				msgs := make([]types.EyrieMessage, len(s.Persistence().RawMessages()))
				copy(msgs, s.Persistence().RawMessages())
				go func() {
					var transcript []string
					for _, m := range msgs {
						transcript = append(transcript, m.Role+": "+m.Content)
					}
					memState := ""
					if s.MemorySvc().Memory() != nil {
						memState, _ = s.MemorySvc().Memory().Recall("", 2000)
					}
					prompt := s.MemorySvc().Sleeptime().BuildConsolidationPrompt(transcript, memState)
					// Use timeout context to prevent goroutine leak if LLM hangs
					sCtx, sCancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
					defer sCancel()
					resp, err := s.ChatLLM().Chat(sCtx, []types.EyrieMessage{
						{Role: "user", Content: prompt},
					}, types.ChatOptions{Provider: s.ChatLLM().Provider(), Model: s.ChatLLM().Model(), MaxTokens: 2048})
					if err != nil || resp == nil {
						return
					}
					if err := lifecycle.ParseAndApplyMemoryOps(s.MemorySvc().Yaad(), resp.Content); err != nil {
						slog.Warn("memory ops", "error", err)
					}
				}()
			}
			// Skill distillation: extract reusable skill from multi-turn tasks
			if s.MemorySvc().SkillDistiller() != nil && toolTurns >= 5 && s.MemorySvc().Yaad() != nil && s.MemorySvc().Yaad().Ready() {
				// Snapshot messages to avoid data race with main loop appending
				msgs := make([]types.EyrieMessage, len(s.Persistence().RawMessages()))
				copy(msgs, s.Persistence().RawMessages())
				// Snapshot the tool/file sets too, so the goroutine never
				// reads the live maps while the main loop writes them on a
				// later tool turn.
				toolsSnapshot := make([]string, 0, len(toolsUsedSet))
				for t := range toolsUsedSet {
					toolsSnapshot = append(toolsSnapshot, t)
				}
				filesSnapshot := make([]string, 0, len(filesModifiedSet))
				for f := range filesModifiedSet {
					filesSnapshot = append(filesSnapshot, f)
				}
				go func() {
					tools := toolsSnapshot
					files := filesSnapshot
					taskDesc := ""
					if len(msgs) > 0 {
						taskDesc = msgs[0].Content
					}
					sd := s.MemorySvc().SkillDistiller()
					prompt := sd.BuildSkillPrompt(taskDesc, tools, files, textContent.String())
					// Use timeout context to prevent goroutine leak if LLM hangs
					dCtx, dCancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
					defer dCancel()
					resp, err := s.ChatLLM().Chat(dCtx, []types.EyrieMessage{
						{Role: "user", Content: prompt},
					}, types.ChatOptions{Provider: s.ChatLLM().Provider(), Model: s.ChatLLM().Model(), MaxTokens: 2048})
					if err != nil || resp == nil {
						return
					}
					skill, err := sd.ParseSkill(resp.Content)
					if err != nil {
						return
					}
					content, _ := json.Marshal(skill)
					_ = s.MemorySvc().Yaad().Remember(string(content), "skill")
				}()
			}
			ch <- StreamEvent{Type: "done"}
			// Integration pipeline: end-session (assess, learn, store experience)
			if s.LifecycleSvc().Pipeline() != nil {
				taskGoal := ""
				for _, m := range s.Persistence().RawMessages() {
					if m.Role == "user" && len(m.ToolResults) == 0 {
						taskGoal = m.Content
						break
					}
				}
				// End-session persistence must complete before the stream closes. A
				// detached goroutine can write after callers tear down their state
				// directory, losing feedback and racing test/process cleanup.
				s.LifecycleSvc().Pipeline().EndSession(context.WithoutCancel(ctx), ctx.Err() == nil, taskGoal)
			}
			// Session end hook
			hooks.ExecuteAsync(context.WithoutCancel(ctx), hooks.EventSessionEnd, map[string]interface{}{
				"provider": s.ChatLLM().Provider(),
				"model":    s.ChatLLM().Model(),
				"messages": len(s.Persistence().RawMessages()),
			})
			// Drain the async hook queue with a bounded wait so post-session
			// observers finish before the process exits; nothing new is
			// scheduled after this point (M19). Timeout guards a hung hook.
			waitCtx, waitCancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
			defer waitCancel()
			if err := hooks.WaitAsync(waitCtx); err != nil {
				slog.Warn("session end hooks", "error", err)
			}
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
		if s.LifecycleSvc().Backtrack() != nil && len(toolCalls) > 0 {
			var toolNames []string
			for _, tc := range toolCalls {
				toolNames = append(toolNames, tc.Name)
			}
			s.LifecycleSvc().Backtrack().RecordDecision(turnCount, strings.Join(toolNames, ", "), nil, s.Persistence().RawMessages())
		}

		results := s.Tools().ExecuteAll(ctx, toolCalls, ch, turnCount, textContent.String())

		// Auto-snapshot after write operations for granular undo
		if s.Tools().Snapshots() != nil && len(toolCalls) > 0 {
			var writeNames []string
			for _, tc := range toolCalls {
				if !tool.IsReadOnly(tc.Name) {
					writeNames = append(writeNames, tc.Name)
				}
			}
			if len(writeNames) > 0 {
				go func() {
					// Bound the snapshot so a slow filesystem doesn't
					// leak a goroutine after the session ends.
					snapCtx, snapCancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
					defer snapCancel()
					_, _ = s.Tools().Snapshots().TrackCtx(snapCtx, strings.Join(writeNames, ", "))
				}()
			}
		}

		// Append assistant message with tool_use blocks
		assistContent := textContent.String()
		if assistContent == "" && len(toolCalls) > 0 {
			assistContent = " " // non-empty to satisfy APIs that reject empty content
		}
		s.Persistence().SetRawMessages(append(s.Persistence().RawMessages(), types.EyrieMessage{
			Role:    "assistant",
			Content: assistContent,
			ToolUse: toolCalls,
		}))
		// Append tool results as proper tool_result messages. Tool output is
		// redacted before it is appended so secrets never reach the model;
		// the user-facing stream events already carried the raw output.
		for _, r := range results {
			resultContent := r.output
			if resultContent == "" {
				resultContent = "(no output)"
			}
			resultContent = s.redactToolResult(resultContent)
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
			s.Persistence().SetRawMessages(append(s.Persistence().RawMessages(), msg))
		}

		// --- STEERING: Inject user guidance between tool batches ---
		if s.Persistence().Steering() != nil && s.Persistence().Steering().HasPending() {
			for _, steer := range s.Persistence().Steering().Drain() {
				s.Persistence().SetRawMessages(append(s.Persistence().RawMessages(), types.EyrieMessage{
					Role:    "user",
					Content: "[User guidance during execution]: " + steer.Content,
				}))
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
		if s.Tools().Sandbox() != nil && s.Tools().Sandbox().IsEnabled() {
			pending := s.Tools().Sandbox().List()
			if len(pending) > 0 {
				ch <- StreamEvent{Type: "content", Content: fmt.Sprintf("\n[%d change(s) staged for review]", len(pending))}
			}
		}

		// Auto-remember: save conversation context and insights to yaad after each turn
		if s.MemorySvc().Memory() != nil {
			userMsg := ""
			assistantMsg := ""
			for i := len(s.Persistence().RawMessages()) - 1; i >= 0; i-- {
				m := s.Persistence().RawMessages()[i]
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
				_ = s.MemorySvc().Memory().Remember(condensed, "conversation")
			}
			// Also save insights if the response has learning signals
			if assistantMsg != "" && shouldRemember(assistantMsg) {
				_ = s.MemorySvc().Memory().Remember(truncate(assistantMsg, 500), "insight")
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
