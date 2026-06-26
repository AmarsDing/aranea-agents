// Package contract defines the pure interfaces and value objects for the event system.
// Biz layer should only import this package, never the parent event package (which contains implementations).
//
// This file holds the Envelope-era types that remain in active production use
// after ADR-03 Phase 5 Blocker G deleted the legacy Envelope plumbing. New code
// MUST NOT introduce additional Envelope dependencies — use biz.ActivityEvent
// (Domain=chat|system) for chat/system events, or MonitorEvent for monitor events.
package contract

// EnvelopeError carries a normalized error payload for WS error responses.
// Consumed by internal/service (envelope_error.go, chat_event_publisher.go)
// to populate ActivityEvent.Meta when a turn fails.
type EnvelopeError struct {
	Type      string `json:"type"`
	Code      string `json:"code,omitempty"`
	Message   string `json:"message"`
	Hint      string `json:"hint,omitempty"`
	PendingID string `json:"pending_id,omitempty"`
}

// EnvelopeTokenUsage is the token-usage payload carried by ActivityEvent
// (Meta["token_usage"]) and persisted by the usage rollup consumer
// (internal/biz/event_bus_usage_rollup_consumer.go).
type EnvelopeTokenUsage struct {
	ID                            string  `json:"id"`
	OccurredAt                    string  `json:"occurred_at"`
	DateKey                       string  `json:"date_key"`
	HourKey                       string  `json:"hour_key"`
	WorkspaceID                   string  `json:"workspace_id"`
	UserID                        string  `json:"user_id"`
	TeamID                        string  `json:"team_id"`
	AgentID                       string  `json:"agent_id"`
	AgentKey                      string  `json:"agent_key"`
	SessionID                     string  `json:"session_id"`
	MessageID                     string  `json:"message_id"`
	RequestID                     string  `json:"request_id"`
	ProviderCode                  string  `json:"provider_code"`
	CanonicalProviderCode         string  `json:"canonical_provider_code"`
	ProviderType                  string  `json:"provider_type"`
	ProviderDisplayName           string  `json:"provider_display_name"`
	ModelAPIID                    string  `json:"model_api_id"`
	ModelDisplayName              string  `json:"model_display_name"`
	ModelCategoryJSON             string  `json:"model_category_json"`
	UsageKind                     string  `json:"usage_kind"`
	CallCount                     int     `json:"call_count"`
	InputTokens                   int     `json:"input_tokens"`
	OutputTokens                  int     `json:"output_tokens"`
	CachedInputTokens             int     `json:"cached_input_tokens"`
	CacheWriteTokens              int     `json:"cache_write_tokens"`
	ReasoningTokens               int     `json:"reasoning_tokens"`
	EmbeddingTokens               int     `json:"embedding_tokens"`
	TotalTokens                   int     `json:"total_tokens"`
	InputPriceMicroUSDPer1K       int64   `json:"input_price_micro_usd_per_1k"`
	OutputPriceMicroUSDPer1K      int64   `json:"output_price_micro_usd_per_1k"`
	CachedInputPriceMicroUSDPer1K int64   `json:"cached_input_price_micro_usd_per_1k"`
	CacheWritePriceMicroUSDPer1K  int64   `json:"cache_write_price_micro_usd_per_1k"`
	ReasoningPriceMicroUSDPer1K   int64   `json:"reasoning_price_micro_usd_per_1k"`
	EmbeddingPriceMicroUSDPer1K   int64   `json:"embedding_price_micro_usd_per_1k"`
	InputCostMicroUSD             int64   `json:"input_cost_micro_usd"`
	OutputCostMicroUSD            int64   `json:"output_cost_micro_usd"`
	CachedInputCostMicroUSD       int64   `json:"cached_input_cost_micro_usd"`
	CacheWriteCostMicroUSD        int64   `json:"cache_write_cost_micro_usd"`
	ReasoningCostMicroUSD         int64   `json:"reasoning_cost_micro_usd"`
	EmbeddingCostMicroUSD         int64   `json:"embedding_cost_micro_usd"`
	TotalCostMicroUSD             int64   `json:"total_cost_micro_usd"`
	LatencyMS                     int     `json:"latency_ms"`
	TimeToFirstTokenMS            int     `json:"time_to_first_token_ms"`
	TokensPerSecond               float64 `json:"tokens_per_second"`
	Status                        string  `json:"status"`
	ErrorCode                     string  `json:"error_code"`
	ErrorMessage                  string  `json:"error_message"`
	RetryCount                    int     `json:"retry_count"`
	PromptMode                    string  `json:"prompt_mode"`
	MaxOutputTokens               int     `json:"max_output_tokens"`
	ContextWindowK                int     `json:"context_window_k"`
	StreamEnabled                 bool    `json:"stream_enabled"`
	MetadataJSON                  string  `json:"metadata_json"`
	CreatedAt                     string  `json:"created_at"`
}

// Tool invocation error_code constants. These originate from the Envelope era
// but remain in active use by the agent layer (tool_invocation_recorder.go,
// tool_confirmation.go, activity_projector.go) as the canonical string values
// recorded on biz.ToolInvocationWrite.ErrorCode / Activity.ToolErrorCode.
const (
	// ErrorCodeToolTimeout indicates a tool execution exceeded its deadline.
	// Used by internal/agent/activity_projector.go (stuck-tool sweep).
	ErrorCodeToolTimeout = "tool_timeout"
	// ErrorCodeToolError is the generic fallback for tool invocation failures.
	// Used by internal/agent/tool_invocation_recorder.go.
	ErrorCodeToolError = "tool_error"
	// ErrorCodeConfirmationRequired indicates a tool is awaiting user approval.
	// Used by internal/agent/tool_confirmation.go and tool_invocation_recorder.go.
	ErrorCodeConfirmationRequired = "confirmation_required"
	// ErrorCodeConfirmationDenied indicates the user rejected a tool confirmation.
	// Used by internal/agent/tool_confirmation.go.
	ErrorCodeConfirmationDenied = "confirmation_denied"
	// ErrorCodeConfirmationTimeout indicates a tool confirmation request expired.
	// Used by internal/agent/tool_confirmation.go.
	ErrorCodeConfirmationTimeout = "confirmation_timeout"
)
