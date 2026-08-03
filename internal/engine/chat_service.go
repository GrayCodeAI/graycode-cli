package engine

import (
	"context"
	"strings"
	"time"

	"github.com/GrayCodeAI/eyrie/engine"
	"github.com/GrayCodeAI/hawk/internal/observability/metrics"
	"github.com/GrayCodeAI/hawk/internal/resilience/ratelimit"
	"github.com/GrayCodeAI/hawk/internal/resilience/retry"
	"github.com/GrayCodeAI/hawk/internal/types"
)

// ChatService is the Session's view of the LLM transport. It owns the
// Eyrie client, provider/model identity, and compatibility-only rate/retry
// controls. Production facade clients own resilience inside Eyrie. The service
// is constructed once in NewSessionWithClient and consulted by
// agentLoop every turn.
//
// Extracted from Session in the god-object decomposition. Session now
// holds *ChatService instead of the 8+ individual fields this service
// previously inlined. See docs/session-decomposition.md for the migration
// plan.
type ChatService struct {
	// client is the eyrie transport. Always non-nil after construction.
	client ChatClient
	// provider / model are the active LLM identity.
	provider string
	model    string
	// deploymentRouting is true when the client is catalog-backed
	// (e.g. DeploymentRouter from eyrie/runtime.ChatProvider).
	deploymentRouting bool
	// rateLimiter is the per-session token bucket.
	rateLimiter *ratelimit.Limiter
	// metrics is the Session-level metrics registry.
	metrics *metrics.Registry
	// retryCfg is the HTTP-retry config for the LLM call.
	retryCfg retry.Config
	// contCfg is the continuation config for StreamChatContinue.
	contCfg types.ContinuationConfig
	// outputSchema, when non-empty, requests a JSON-schema-constrained
	// response. Plumbed into eyrie's ChatOptions.ResponseFormat.
	outputSchema string
	// thinkingEnabled is the generic host preference for provider thinking /
	// reasoning toggles (Z.AI, LongCat, Agnes, …). nil leaves provider default.
	thinkingEnabled *bool
}

// ChatServiceConfig bundles the optional fields the constructor doesn't
// require. NewSessionWithClient sets sensible defaults for any zero-valued
// field; tests can override individual fields.
type ChatServiceConfig struct {
	Provider           string
	Model              string
	DeploymentRouting  bool
	RateLimiter        *ratelimit.Limiter
	Metrics            *metrics.Registry
	RetryConfig        retry.Config
	ContinuationConfig types.ContinuationConfig
	OutputSchema       string
	ThinkingEnabled    *bool
	// GLMThinkingEnabled is accepted as a deprecated alias of ThinkingEnabled.
	GLMThinkingEnabled *bool
}

// NewChatService constructs a ChatService with sensible defaults for any
// zero-valued field in cfg. The client must be non-nil.
func NewChatService(client ChatClient, cfg ChatServiceConfig) *ChatService {
	if cfg.RetryConfig.MaxRetries == 0 {
		cfg.RetryConfig = retry.DefaultConfig()
		cfg.RetryConfig.MaxRetries = 2
		cfg.RetryConfig.BaseDelay = 500 * time.Millisecond
	}
	if cfg.ContinuationConfig.MaxContinuations == 0 {
		cfg.ContinuationConfig = types.DefaultContinuationConfig()
	}
	if cfg.Metrics == nil {
		cfg.Metrics = metrics.NewRegistry()
	}
	thinking := cfg.ThinkingEnabled
	if thinking == nil {
		thinking = cfg.GLMThinkingEnabled
	}
	return &ChatService{
		client:            client,
		provider:          cfg.Provider,
		model:             cfg.Model,
		deploymentRouting: cfg.DeploymentRouting,
		rateLimiter:       cfg.RateLimiter,
		metrics:           cfg.Metrics,
		retryCfg:          cfg.RetryConfig,
		contCfg:           cfg.ContinuationConfig,
		outputSchema:      cfg.OutputSchema,
		thinkingEnabled:   thinking,
	}
}

// Client returns the underlying eyrie client. Exposed for callers (e.g.
// background goroutines) that need to issue one-off LLM calls without
// the agent-loop retry wrapper.
func (c *ChatService) Client() ChatClient { return c.client }

// Provider returns the active provider identifier.
func (c *ChatService) Provider() string { return c.provider }

// Model returns the active model identifier.
func (c *ChatService) Model() string { return c.model }

// DeploymentRouting reports whether the underlying client is catalog-backed
// (true) or a single-provider transport (false).
func (c *ChatService) DeploymentRouting() bool { return c.deploymentRouting }

// SetThinkingEnabled sets the generic host thinking/reasoning toggle for
// providers that support it (Z.AI, LongCat, Agnes, …).
func (c *ChatService) SetThinkingEnabled(v *bool) {
	c.thinkingEnabled = v
}

// SetGLMThinkingEnabled is a deprecated alias of SetThinkingEnabled.
func (c *ChatService) SetGLMThinkingEnabled(v *bool) {
	c.SetThinkingEnabled(v)
}

// SetModel updates the active model. The next StreamChat will use the new
// model.
func (c *ChatService) SetModel(model string) {
	c.model = model
}

// SetProvider updates the active provider.
func (c *ChatService) SetProvider(provider string) {
	c.provider = provider
}

// Reattach swaps the underlying client (e.g. after deployment routing
// changes).
func (c *ChatService) Reattach(client ChatClient, provider string) {
	if client == nil {
		return
	}
	c.client = client
	if provider != "" {
		c.provider = provider
	}
}

// BuildOptions constructs a types.ChatOptions for an outgoing LLM call,
// encoding all the knobs the agent loop needs (system prompt, model,
// max tokens, tools, structured output, etc.).
func (c *ChatService) BuildOptions(systemPrompt, activeModel string, maxTokens int, tools []types.EyrieTool) types.ChatOptions {
	opts := types.ChatOptions{
		Provider:      c.provider,
		Model:         activeModel,
		MaxTokens:     maxTokens,
		System:        systemPrompt,
		EnableCaching: c.provider == "anthropic",
		Tools:         tools,
	}
	if supportsThinkingToggle(c.provider) && c.thinkingEnabled != nil {
		opts.ThinkingEnabled = c.thinkingEnabled
		opts.GLMThinkingEnabled = c.thinkingEnabled // alias for older adapters
	}
	// Structured output: request a JSON-schema-constrained response when set.
	if c.outputSchema != "" {
		opts.ResponseFormat = &types.ResponseFormat{Type: "json_schema", Schema: c.outputSchema}
	}
	return opts
}

// Stream issues a streaming LLM call with retry, rate-limit, and
// emergency-compact. The returned *types.StreamResult's Events channel
// emits EyrieStreamEvent values; the caller must Close() the result
// when done.
//
// On context cancellation mid-call, returns the cancellation error wrapped
// with whatever partial state the upstream had emitted (caller should
// check ctx.Err()).
//
// Eyrie facade clients advertise that they manage provider resilience. For
// those clients this service records the product metric and delegates exactly
// once; injected legacy clients retain Hawk's compatibility retry/rate layer.
func (c *ChatService) Stream(ctx context.Context, messages []types.EyrieMessage, opts types.ChatOptions) (*types.StreamResult, error) {
	if clientManagesResilience(c.client) {
		c.metrics.Counter("api.requests").Inc()
		return c.client.StreamChatContinue(ctx, messages, opts, c.contCfg)
	}
	// Rate limit: wait for a token before making the LLM call
	if c.rateLimiter != nil {
		if waitErr := c.rateLimiter.Wait(ctx); waitErr != nil {
			return nil, waitErr
		}
	}
	c.metrics.Counter("api.requests").Inc()

	var result *types.StreamResult
	err := retry.Do(ctx, c.retryCfg, func() error {
		var callErr error
		result, callErr = c.client.StreamChatContinue(ctx, messages, opts, c.contCfg)
		if callErr != nil {
			// On context overflow, do an emergency compact and retry once.
			// Previously this re-sent the unmodified messages — a no-op that
			// wasted spend and always overflows again (H3). Now we actually
			// shrink the transcript beneath the ceiling first.
			if isContextOverflow(callErr) {
				compacted := emergencyCompact(messages)
				result, callErr = c.client.StreamChatContinue(ctx, compacted, opts, c.contCfg)
			}
		}
		return callErr
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// emergencyCompactMin / emergencyCompactWindow control the aggressive
// trimming applied when a provider rejects the context as too long.
const (
	emergencyCompactMin    = 8  // never compact below this many messages
	emergencyCompactWindow = 24 // keep this many trailing messages (+ system)
)

// emergencyCompact trims an overflowing transcript so a single retry fits
// under the provider ceiling. It keeps the system prompt and the most recent
// emergencyCompactWindow messages. This is a last-resort path (the normal
// context governor in the agent loop does the real summarization); here we
// only need the retry to succeed once.
func emergencyCompact(messages []types.EyrieMessage) []types.EyrieMessage {
	if len(messages) <= emergencyCompactMin {
		return messages
	}
	out := make([]types.EyrieMessage, 0, emergencyCompactWindow+1)
	for _, m := range messages {
		if m.Role == "system" {
			out = append(out, m)
			break
		}
	}
	start := len(messages) - emergencyCompactWindow
	if start < len(out) {
		start = len(out)
	}
	out = append(out, messages[start:]...)
	return out
}

// Chat issues a non-streaming LLM call. Used by background goroutines
// (sleeptime consolidation, skill distillation) that don't need
// incremental events.
func (c *ChatService) Chat(ctx context.Context, messages []types.EyrieMessage, opts types.ChatOptions) (*types.EyrieResponse, error) {
	return c.client.Chat(ctx, messages, opts)
}

// isContextOverflow reports whether err looks like a "context too long"
// error from the upstream provider. Used by Stream() to trigger an
// emergency context-compact + retry. Matches structured provider signals
// (context_length_exceeded / context_length_error) and the common
// "N tokens exceeds the limit" phrasing, but requires a token/context
// qualifier alongside "too long" so ordinary "request timeout, too long"
// errors don't spuriously trigger an emergency compact.
func isContextOverflow(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	switch {
	case contains(msg, "context_length_exceeded"),
		contains(msg, "context_length_error"),
		contains(msg, "exceeds the limit"):
		return true
	}
	if contains(msg, "too long") || contains(msg, "too many tokens") {
		return contains(msg, "context") ||
			contains(msg, "tokens") ||
			contains(msg, "token") ||
			contains(msg, "limit")
	}
	return false
}

func contains(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) && (s == sub || (len(s) > 0 && indexOf(s, sub) >= 0))
}

// supportsThinkingToggle reports providers that honor ThinkingEnabled on the
// OpenAI-compat wire (each with its own ThinkingFormat).
func supportsThinkingToggle(provider string) bool {
	return engine.ThinkingToggleSupported(provider)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
