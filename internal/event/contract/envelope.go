// Package contract defines the pure interfaces and value objects for the event system.
// Biz layer should only import this package, never the parent event package (which contains implementations).
package contract

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

// EnvelopeType labels the kind of event carried by an Envelope.
type EnvelopeType string

const (
	EnvelopeTypeTextDelta          EnvelopeType = "text_delta"
	EnvelopeTypeTextDone           EnvelopeType = "text_done"
	EnvelopeTypeToolCall           EnvelopeType = "tool_call"
	EnvelopeTypeToolResult         EnvelopeType = "tool_result"
	EnvelopeTypeStateDelta         EnvelopeType = "state_delta"
	EnvelopeTypeTransfer           EnvelopeType = "transfer"
	EnvelopeTypeRunnerCompletion   EnvelopeType = "runner_completion"
	// EnvelopeTypeContextUsage carries mid-turn context window fill (ReAct sub-steps).
	EnvelopeTypeContextUsage EnvelopeType = "context_usage"
	EnvelopeTypeRunStatus          EnvelopeType = "run_status"
	EnvelopeTypeError              EnvelopeType = "error"
	EnvelopeTypeLog                EnvelopeType = "log"
	EnvelopeTypeFlowLog            EnvelopeType = "flow_log"
	EnvelopeTypeGraphNodeStart     EnvelopeType = "graph_node_start"
	EnvelopeTypeGraphNodeEnd       EnvelopeType = "graph_node_end"
	EnvelopeTypeCheckpoint         EnvelopeType = "checkpoint"
	EnvelopeTypeIntentPass         EnvelopeType = "intent_pass"
	EnvelopeTypeMemberMessageStart EnvelopeType = "member_message_start"
	EnvelopeTypeMemberDelta        EnvelopeType = "member_delta"
	EnvelopeTypeMemberMessageDone  EnvelopeType = "member_message_done"
	EnvelopeTypeTeamRunStarted     EnvelopeType = "team_run_started"
	EnvelopeTypeTeamRunFinished    EnvelopeType = "team_run_finished"
	EnvelopeTypeTeamStepStarted    EnvelopeType = "team_step_started"
	EnvelopeTypeTeamStepFinished   EnvelopeType = "team_step_finished"
	EnvelopeTypeTeamRunFailed      EnvelopeType = "team_run_failed"
	EnvelopeTypeTeamSummary        EnvelopeType = "team_summary"
	EnvelopeTypeGraphStep          EnvelopeType = "graph_step"
	EnvelopeTypeGraphExecutionDone EnvelopeType = "graph_execution_done"
	EnvelopeTypeGraphNodeError     EnvelopeType = "graph_node_error"
	EnvelopeTypeGraphNodeCustom    EnvelopeType = "graph_node_custom"
	EnvelopeTypeGraphTaskStatus    EnvelopeType = "graph_task_status"
	EnvelopeTypeKnowledgeIngest    EnvelopeType = "knowledge_ingest"
	EnvelopeTypeMCPSessionReconnect EnvelopeType = "mcp.session.reconnect"
	EnvelopeTypeMCPHealthAlert      EnvelopeType = "mcp.health.alert"
	EnvelopeTypeAlertNotify                 EnvelopeType = "alert.notify"
	EnvelopeTypeOrchestrationAgentStatus    EnvelopeType = "orchestration_agent_status"
	EnvelopeTypeUserFeedback                EnvelopeType = "user_feedback"
	EnvelopeTypeSessionStatusChanged        EnvelopeType = "session.status_changed"
	EnvelopeTypeSpiritTeamAssembled         EnvelopeType = "spirit_team_assembled"
	EnvelopeTypeSpiritTeamCompleted         EnvelopeType = "spirit_team_completed"
	EnvelopeTypeSpiritTeamFailed            EnvelopeType = "spirit_team_failed"
	EnvelopeTypeSpiritTeamProgress          EnvelopeType = "spirit_team_progress"
	EnvelopeTypeSpiritTeamsAllCompleted     EnvelopeType = "spirit_teams_all_completed"
	EnvelopeTypeSpiritSynthesisCompleted    EnvelopeType = "spirit_synthesis_completed"
	EnvelopeTypeSpiritPlanCreated           EnvelopeType = "spirit_plan_created"
	EnvelopeTypeSpiritAllocationCreated     EnvelopeType = "spirit_allocation_created"
	EnvelopeTypeSpiritOrchestrationStarted  EnvelopeType = "spirit_orchestration_started"
	EnvelopeTypeSpiritOrchestrationCheckpoint EnvelopeType = "spirit_orchestration_checkpoint"
	EnvelopeTypeSpiritOrchestrationInterrupted EnvelopeType = "spirit_orchestration_interrupted"
	EnvelopeTypeTokenUsage                  EnvelopeType = "token_usage"
	EnvelopeTypeMetricsUpdated              EnvelopeType = "metrics_updated"

	// Butler orchestration events
	EnvelopeTypeButlerOrchestrationStarted  EnvelopeType = "butler.orchestration.started"
	EnvelopeTypeButlerOrchestrationCompleted EnvelopeType = "butler.orchestration.completed"
	EnvelopeTypeButlerOrchestrationFailed   EnvelopeType = "butler.orchestration.failed"

	// Skill evolution events
	EnvelopeTypeSkillHealthChanged          EnvelopeType = "skill.health_changed"
	EnvelopeTypeSkillEvolutionProposed      EnvelopeType = "skill.evolution_proposed"

	// Monitor self-healing events
	EnvelopeTypeMonitorAutoHealed           EnvelopeType = "monitor.auto_healed"
	EnvelopeTypeMonitorSelfCheckCompleted   EnvelopeType = "monitor.self_check_completed"
)

// Envelope is the universal event carrier.
type Envelope struct {
	ID                 string       `json:"id"`
	Type               EnvelopeType `json:"type"`
	Author             string       `json:"author"`
	SessionID          string       `json:"session_id"`
	TeamID             string       `json:"team_id,omitempty"`
	RequestID          string       `json:"request_id,omitempty"`
	InvocationID       string       `json:"invocation_id,omitempty"`
	ParentInvocationID string       `json:"parent_invocation_id,omitempty"`
	Branch             string       `json:"branch,omitempty"`
	FilterKey          string       `json:"filter_key,omitempty"`
	Tag                string       `json:"tag,omitempty"`
	Timestamp          string       `json:"timestamp"`
	Version            int          `json:"version"`
	Channel            string       `json:"channel,omitempty"`

	Content    *EnvelopeContent    `json:"content,omitempty"`
	ToolCall   *EnvelopeToolCall   `json:"tool_call,omitempty"`
	StateDelta *EnvelopeStateDelta `json:"state_delta,omitempty"`
	Transfer   *EnvelopeTransfer   `json:"transfer,omitempty"`
	Error      *EnvelopeError      `json:"error,omitempty"`
	Usage      *EnvelopeUsage      `json:"usage,omitempty"`
	TokenUsage *EnvelopeTokenUsage `json:"token_usage,omitempty"`
	Extensions map[string]string   `json:"extensions,omitempty"`
	Actions    *EnvelopeActions    `json:"actions,omitempty"`
	Trace      *EnvelopeTrace      `json:"trace,omitempty"`
	Metadata   map[string]any      `json:"metadata,omitempty"`

	SessionRevision int64  `json:"session_revision,omitempty"`
	Source          string `json:"source,omitempty"`
	JobID           string `json:"job_id,omitempty"`
	TurnID          string `json:"turn_id,omitempty"`
}

type EnvelopeContent struct {
	Text      string `json:"text"`
	Reasoning string `json:"reasoning,omitempty"`
	IsPartial bool   `json:"is_partial"`
}

type EnvelopeToolCall struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	ArgumentsJSON string `json:"arguments_json"`
	ResultJSON    string `json:"result_json,omitempty"`
	Status        string `json:"status"`
	DurationMS    int64  `json:"duration_ms,omitempty"`
	IsLongRunning bool   `json:"is_long_running,omitempty"`

	ActivityKind string `json:"activity_kind,omitempty"`
	DisplayLabel string `json:"display_label,omitempty"`
	IconKey      string `json:"icon_key,omitempty"`
	Summary      string `json:"summary,omitempty"`
	StartedAt    string `json:"started_at,omitempty"`
	FinishedAt   string `json:"finished_at,omitempty"`
	ErrorCode    string `json:"error_code,omitempty"`
	AgentKey     string `json:"agent_key,omitempty"`
	AgentID      string `json:"agent_id,omitempty"`
	AgentName    string `json:"agent_name,omitempty"`
	RunID        string `json:"run_id,omitempty"`
	TraceID      string `json:"trace_id,omitempty"`
}

type EnvelopeStateDelta struct {
	Operation string `json:"operation"`
	Path      string `json:"path"`
	ValueJSON string `json:"value_json"`
}

type EnvelopeTransfer struct {
	FromAgent string `json:"from_agent"`
	ToAgent   string `json:"to_agent"`
}

type EnvelopeError struct {
	Type      string `json:"type"`
	Code      string `json:"code,omitempty"`
	Message   string `json:"message"`
	Hint      string `json:"hint,omitempty"`
	PendingID string `json:"pending_id,omitempty"`
}

type EnvelopeUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
	// MaxTokens is the configured context window for the active model/session.
	MaxTokens int `json:"max_tokens,omitempty"`
	// ContextPromptTokens is the max prompt_tokens in the turn (context window fill).
	ContextPromptTokens int `json:"context_prompt_tokens,omitempty"`
	// TurnTotalTokens is prompt+completion accumulated for the current turn (ReAct multi-call safe).
	TurnTotalTokens int `json:"turn_total_tokens,omitempty"`
}

type EnvelopeActions struct {
	SkipSummarization bool `json:"skip_summarization,omitempty"`
}

type EnvelopeTokenUsage struct {
	ID                       string  `json:"id"`
	OccurredAt               string  `json:"occurred_at"`
	DateKey                  string  `json:"date_key"`
	HourKey                  string  `json:"hour_key"`
	WorkspaceID              string  `json:"workspace_id"`
	UserID                   string  `json:"user_id"`
	TeamID                   string  `json:"team_id"`
	AgentID                  string  `json:"agent_id"`
	AgentKey                 string  `json:"agent_key"`
	SessionID                string  `json:"session_id"`
	MessageID                string  `json:"message_id"`
	RequestID                string  `json:"request_id"`
	ProviderCode             string  `json:"provider_code"`
	CanonicalProviderCode    string  `json:"canonical_provider_code"`
	ProviderType             string  `json:"provider_type"`
	ProviderDisplayName      string  `json:"provider_display_name"`
	ModelAPIID               string  `json:"model_api_id"`
	ModelDisplayName         string  `json:"model_display_name"`
	ModelCategoryJSON        string  `json:"model_category_json"`
	UsageKind                string  `json:"usage_kind"`
	CallCount                int     `json:"call_count"`
	InputTokens              int     `json:"input_tokens"`
	OutputTokens             int     `json:"output_tokens"`
	CachedInputTokens        int     `json:"cached_input_tokens"`
	CacheWriteTokens         int     `json:"cache_write_tokens"`
	ReasoningTokens          int     `json:"reasoning_tokens"`
	EmbeddingTokens          int     `json:"embedding_tokens"`
	TotalTokens              int     `json:"total_tokens"`
	InputPriceMicroUSDPer1K  int64   `json:"input_price_micro_usd_per_1k"`
	OutputPriceMicroUSDPer1K int64   `json:"output_price_micro_usd_per_1k"`
	CachedInputPriceMicroUSDPer1K int64 `json:"cached_input_price_micro_usd_per_1k"`
	CacheWritePriceMicroUSDPer1K  int64 `json:"cache_write_price_micro_usd_per_1k"`
	ReasoningPriceMicroUSDPer1K   int64 `json:"reasoning_price_micro_usd_per_1k"`
	EmbeddingPriceMicroUSDPer1K   int64 `json:"embedding_price_micro_usd_per_1k"`
	InputCostMicroUSD        int64   `json:"input_cost_micro_usd"`
	OutputCostMicroUSD       int64   `json:"output_cost_micro_usd"`
	CachedInputCostMicroUSD  int64   `json:"cached_input_cost_micro_usd"`
	CacheWriteCostMicroUSD   int64   `json:"cache_write_cost_micro_usd"`
	ReasoningCostMicroUSD    int64   `json:"reasoning_cost_micro_usd"`
	EmbeddingCostMicroUSD    int64   `json:"embedding_cost_micro_usd"`
	TotalCostMicroUSD        int64   `json:"total_cost_micro_usd"`
	LatencyMS                int     `json:"latency_ms"`
	TimeToFirstTokenMS       int     `json:"time_to_first_token_ms"`
	TokensPerSecond          float64 `json:"tokens_per_second"`
	Status                   string  `json:"status"`
	ErrorCode                string  `json:"error_code"`
	ErrorMessage             string  `json:"error_message"`
	RetryCount               int     `json:"retry_count"`
	PromptMode               string  `json:"prompt_mode"`
	MaxOutputTokens          int     `json:"max_output_tokens"`
	ContextWindowK           int     `json:"context_window_k"`
	StreamEnabled            bool    `json:"stream_enabled"`
	MetadataJSON             string  `json:"metadata_json"`
	CreatedAt                string  `json:"created_at"`
}

type EnvelopeTrace struct {
	AgentName    string `json:"agent_name"`
	InvocationID string `json:"invocation_id"`
	StepCount    int    `json:"step_count"`
	DurationMS   int64  `json:"duration_ms,omitempty"`
}

// NewEnvelope creates a new Envelope with a generated ID and current timestamp.
func NewEnvelope(typ EnvelopeType, author, sessionID string) Envelope {
	return Envelope{
		ID:        uuid.NewString(),
		Type:      typ,
		Author:    author,
		SessionID: sessionID,
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Version:   1,
	}
}

// RouteChannel returns the logical channel name for routing an envelope.
// TECH-DEBT: RouteChannel switch-case violates OCP. Consider per-domain routing tables
// or attaching channel metadata to EnvelopeType definitions.
func RouteChannel(env Envelope) string {
	switch env.Type {
	case EnvelopeTypeLog, EnvelopeTypeFlowLog:
		return "monitor"
	case EnvelopeTypeMemberMessageStart, EnvelopeTypeMemberDelta, EnvelopeTypeMemberMessageDone,
		EnvelopeTypeTeamRunStarted, EnvelopeTypeTeamRunFinished, EnvelopeTypeTeamStepStarted,
		EnvelopeTypeTeamStepFinished, EnvelopeTypeTeamRunFailed, EnvelopeTypeTeamSummary,
		EnvelopeTypeOrchestrationAgentStatus:
		return "team"
	case EnvelopeTypeGraphNodeStart, EnvelopeTypeGraphNodeEnd, EnvelopeTypeCheckpoint,
		EnvelopeTypeGraphStep, EnvelopeTypeGraphExecutionDone, EnvelopeTypeGraphNodeError,
		EnvelopeTypeGraphNodeCustom, EnvelopeTypeGraphTaskStatus:
		return "graph"
	case EnvelopeTypeKnowledgeIngest:
		return "knowledge"
	case EnvelopeTypeMCPSessionReconnect, EnvelopeTypeMCPHealthAlert, EnvelopeTypeAlertNotify:
		return "monitor"
	case EnvelopeTypeSessionStatusChanged:
		return "chat"
	case EnvelopeTypeMetricsUpdated:
		return "chat"
	case EnvelopeTypeSpiritTeamAssembled, EnvelopeTypeSpiritTeamCompleted, EnvelopeTypeSpiritTeamFailed, EnvelopeTypeSpiritTeamProgress, EnvelopeTypeSpiritTeamsAllCompleted, EnvelopeTypeSpiritSynthesisCompleted,
		EnvelopeTypeSpiritPlanCreated, EnvelopeTypeSpiritAllocationCreated, EnvelopeTypeSpiritOrchestrationStarted, EnvelopeTypeSpiritOrchestrationCheckpoint, EnvelopeTypeSpiritOrchestrationInterrupted,
		EnvelopeTypeButlerOrchestrationStarted, EnvelopeTypeButlerOrchestrationCompleted, EnvelopeTypeButlerOrchestrationFailed,
		EnvelopeTypeSkillHealthChanged, EnvelopeTypeSkillEvolutionProposed:
		return "chat"
	case EnvelopeTypeMonitorAutoHealed, EnvelopeTypeMonitorSelfCheckCompleted:
		return "monitor"
	default:
		if env.TeamID != "" {
			return "team"
		}
		return "chat"
	}
}

// MatchFilterKey checks if a subscriber's filter key matches an event's filter key.
func MatchFilterKey(subscriberKey, eventKey string) bool {
	if subscriberKey == "" || eventKey == "" {
		return true
	}
	sk := subscriberKey + "/"
	ek := eventKey + "/"
	return strings.HasPrefix(sk, ek) || strings.HasPrefix(ek, sk)
}

// Clone returns a deep copy of the Envelope.
func (e Envelope) Clone() Envelope {
	clone := e
	if e.Content != nil {
		c := *e.Content
		clone.Content = &c
	}
	if e.ToolCall != nil {
		tc := *e.ToolCall
		clone.ToolCall = &tc
	}
	if e.StateDelta != nil {
		sd := *e.StateDelta
		clone.StateDelta = &sd
	}
	if e.Transfer != nil {
		t := *e.Transfer
		clone.Transfer = &t
	}
	if e.Error != nil {
		err := *e.Error
		clone.Error = &err
	}
	if e.Usage != nil {
		u := *e.Usage
		clone.Usage = &u
	}
	if e.TokenUsage != nil {
		tu := *e.TokenUsage
		clone.TokenUsage = &tu
	}
	if e.Extensions != nil {
		clone.Extensions = make(map[string]string, len(e.Extensions))
		for k, v := range e.Extensions {
			clone.Extensions[k] = v
		}
	}
	if e.Actions != nil {
		a := *e.Actions
		clone.Actions = &a
	}
	if e.Trace != nil {
		t := *e.Trace
		clone.Trace = &t
	}
	if e.Metadata != nil {
		clone.Metadata = make(map[string]any, len(e.Metadata))
		for k, v := range e.Metadata {
			clone.Metadata[k] = v
		}
	}
	return clone
}

// ContainsTag checks if the envelope has the given tag.
func (e Envelope) ContainsTag(tag string) bool {
	if e.Tag == tag {
		return true
	}
	if e.Tag == "" || tag == "" {
		return false
	}
	for _, part := range strings.Split(e.Tag, ",") {
		if strings.TrimSpace(part) == tag {
			return true
		}
	}
	return false
}
