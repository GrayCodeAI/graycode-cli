package engine

import (
	"context"
	"time"

	"github.com/GrayCodeAI/hawk/internal/observability/metrics"
	"github.com/GrayCodeAI/hawk/internal/resilience/ratelimit"
	"github.com/GrayCodeAI/hawk/internal/resilience/retry"
	"github.com/GrayCodeAI/hawk/internal/types"

	modelPkg "github.com/GrayCodeAI/hawk/internal/provider/routing"
)

// ChatService is the Session's view of the LLM transport. It owns the
// eyrie client, the provider/model identity, API keys, the circuit-breaker
// router, the rate limiter, and the streaming-with-continuation retry
// logic. It is constructed once in NewSessionWithClient and consulted by
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
	// apiKeys is provider→key, used for legacy single-provider clients.
	apiKeys map[string]string
	// router is the legacy single-provider circuit breaker. Bypassed
	// when DeploymentRouting is true (the DeploymentRouter has its own
	// per-deployment breakers).
	router *modelPkg.Router
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
	// glmThinkingEnabled toggles GLM/Z.ai extended reasoning on outgoing
	// requests. nil leaves the model default.
	glmThinkingEnabled *bool
}

// ChatServiceConfig bundles the optional fields the constructor doesn't
// require. NewSessionWithClient sets sensible defaults for any zero-valued
// field; tests can override individual fields.
type ChatServiceConfig struct {
	Provider           string
	Model              string
	APIKeys            map[string]string
	Router             *modelPkg.Router
	DeploymentRouting  bool
	RateLimiter        *ratelimit.Limiter
	Metrics            *metrics.Registry
	RetryConfig        retry.Config
	ContinuationConfig types.ContinuationConfig
	OutputSchema       string
	GLMThinkingEnabled *bool
}

// NewChatService constructs a ChatService with sensible defaults for any
// zero-valued field in cfg. The client must be non-nil.
func NewChatService(client ChatClient, cfg ChatServiceConfig) *ChatService {
	if cfg.APIKeys == nil {
		cfg.APIKeys = map[string]string{}
	}
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
	return &ChatService{
		client:             client,
		provider:           cfg.Provider,
		model:              cfg.Model,
		apiKeys:            cfg.APIKeys,
		router:             cfg.Router,
		deploymentRouting:  cfg.DeploymentRouting,
		rateLimiter:        cfg.RateLimiter,
		metrics:            cfg.Metrics,
		retryCfg:           cfg.RetryConfig,
		contCfg:            cfg.ContinuationConfig,
		outputSchema:       cfg.OutputSchema,
		glmThinkingEnabled: cfg.GLMThinkingEnabled,
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

// APIKeys returns the provider→key map. Used by Session.SubSession to
// clone credentials for sub-agents.
func (c *ChatService) APIKeys() map[string]string { return c.apiKeys }

// DeploymentRouting reports whether the underlying client is catalog-backed
// (true) or a single-provider transport (false).
func (c *ChatService) DeploymentRouting() bool { return c.deploymentRouting }

// SetAPIKey stores a provider→key mapping.
func (c *ChatService) SetAPIKey(provider, key string) {
	c.apiKeys[provider] = key
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
// changes). Preserves the APIKeys and other config.
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
	// GLM/Z.ai extended reasoning toggle: only meaningful for the z-ai
	// provider, where eyrie emits thinking={type:enabled|disabled}.
	if c.provider == "z-ai" && c.glmThinkingEnabled != nil {
		opts.GLMThinkingEnabled = c.glmThinkingEnabled
	}
	// Structured output: request a JSON-schema-constrained response when set.
	if c.outputSchema != "" {
		opts.ResponseFormat = &types.ResponseFormat{Type: "json_schema", Schema: c.outputSchema}
	}
	return opts
}

// Stream issues a streaming LLM call with retry, rate-limit, and circuit-
// breaker accounting. The returned *types.StreamResult's Events channel
// emits EyrieStreamEvent values; the caller must Close() the result when
// done.
//
// On context cancellation mid-call, returns the cancellation error wrapped
// with whatever partial state the upstream had emitted (caller should
// check ctx.Err()).
func (c *ChatService) Stream(ctx context.Context, messages []types.EyrieMessage, opts types.ChatOptions) (*types.StreamResult, error) {
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
			if isContextOverflow(callErr) {
				result, callErr = c.client.StreamChatContinue(ctx, messages, opts, c.contCfg)
			}
		}
		return callErr
	})
	if err != nil {
		c.recordFailure(err)
		return nil, err
	}
	c.recordSuccess()
	return result, nil
}

// Chat issues a non-streaming LLM call. Used by background goroutines
// (sleeptime consolidation, skill distillation) that don't need
// incremental events.
func (c *ChatService) Chat(ctx context.Context, messages []types.EyrieMessage, opts types.ChatOptions) (*types.EyrieResponse, error) {
	return c.client.Chat(ctx, messages, opts)
}

// recordSuccess records a successful LLM call against the legacy circuit-
// breaker router. No-op when DeploymentRouting is on (the DeploymentRouter
// has its own breakers).
func (c *ChatService) recordSuccess() {
	if c.router != nil && !c.deploymentRouting {
		c.router.RecordSuccess(c.provider, 0)
	}
}

// recordFailure records a failed LLM call against the legacy circuit-
// breaker router. No-op when DeploymentRouting is on.
func (c *ChatService) recordFailure(err error) {
	if c.router != nil && !c.deploymentRouting {
		c.router.RecordFailure(c.provider, err)
	}
}

// isContextOverflow reports whether err looks like a "context too long"
// error from the upstream provider. Used by Stream() to trigger an
// emergency context-compact + retry.
func isContextOverflow(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return contains(msg, "too long") || contains(msg, "too many tokens")
}

func contains(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) && (s == sub || (len(s) > 0 && indexOf(s, sub) >= 0))
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
