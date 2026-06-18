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
	EnvelopeTypeTextDelta        EnvelopeType = "text_delta"
	EnvelopeTypeTextDone         EnvelopeType = "text_done"
	EnvelopeTypeToolCall         EnvelopeType = "tool_call"
	EnvelopeTypeToolResult       EnvelopeType = "tool_result"
	EnvelopeTypeStateDelta       EnvelopeType = "state_delta"
	EnvelopeTypeTransfer         EnvelopeType = "transfer"
	EnvelopeTypeRunnerCompletion EnvelopeType = "runner_completion"
	// EnvelopeTypeContextUsage carries mid-turn context window fill (ReAct sub-steps).
	EnvelopeTypeContextUsage                   EnvelopeType = "context_usage"
	EnvelopeTypeRunStatus                      EnvelopeType = "run_status"
	EnvelopeTypeError                          EnvelopeType = "error"
	EnvelopeTypeLog                            EnvelopeType = "log"
	EnvelopeTypeFlowLog                        EnvelopeType = "flow_log"
	EnvelopeTypeGraphNodeStart                 EnvelopeType = "graph_node_start"
	EnvelopeTypeGraphNodeEnd                   EnvelopeType = "graph_node_end"
	EnvelopeTypeCheckpoint                     EnvelopeType = "checkpoint"
	EnvelopeTypeIntentPass                     EnvelopeType = "intent_pass"
	EnvelopeTypeMemberMessageStart             EnvelopeType = "member_message_start"
	EnvelopeTypeMemberDelta                    EnvelopeType = "member_delta"
	EnvelopeTypeMemberMessageDone              EnvelopeType = "member_message_done"
	EnvelopeTypeTeamRunStarted                 EnvelopeType = "team_run_started"
	EnvelopeTypeTeamRunFinished                EnvelopeType = "team_run_finished"
	EnvelopeTypeTeamStepStarted                EnvelopeType = "team_step_started"
	EnvelopeTypeTeamStepFinished               EnvelopeType = "team_step_finished"
	EnvelopeTypeTeamRunFailed                  EnvelopeType = "team_run_failed"
	EnvelopeTypeTeamSummary                    EnvelopeType = "team_summary"
	EnvelopeTypeGraphStep                      EnvelopeType = "graph_step"
	EnvelopeTypeGraphExecutionDone             EnvelopeType = "graph_execution_done"
	EnvelopeTypeGraphNodeError                 EnvelopeType = "graph_node_error"
	EnvelopeTypeGraphNodeCustom                EnvelopeType = "graph_node_custom"
	EnvelopeTypeGraphTaskStatus                EnvelopeType = "graph_task_status"
	EnvelopeTypeKnowledgeIngest                EnvelopeType = "knowledge_ingest"
	EnvelopeTypeMCPSessionReconnect            EnvelopeType = "mcp.session.reconnect"
	EnvelopeTypeMCPHealthAlert                 EnvelopeType = "mcp.health.alert"
	EnvelopeTypeAlertNotify                    EnvelopeType = "alert.notify"
	EnvelopeTypeOrchestrationAgentStatus       EnvelopeType = "orchestration_agent_status"
	EnvelopeTypeUserFeedback                   EnvelopeType = "user_feedback"
	EnvelopeTypeSessionStatusChanged           EnvelopeType = "session.status_changed"
	EnvelopeTypeSpiritTeamAssembled            EnvelopeType = "spirit_team_assembled"
	EnvelopeTypeSpiritTeamCompleted            EnvelopeType = "spirit_team_completed"
	EnvelopeTypeSpiritTeamFailed               EnvelopeType = "spirit_team_failed"
	EnvelopeTypeSpiritTeamCancelled            EnvelopeType = "spirit_team_cancelled"
	EnvelopeTypeSpiritTeamInterrupted          EnvelopeType = "spirit_team_interrupted"
	EnvelopeTypeSpiritTeamProgress             EnvelopeType = "spirit_team_progress"
	EnvelopeTypeSpiritTeamsAllCompleted        EnvelopeType = "spirit_teams_all_completed"
	EnvelopeTypeSpiritSynthesisCompleted       EnvelopeType = "spirit_synthesis_completed"
	EnvelopeTypeSpiritPlanCreated              EnvelopeType = "spirit_plan_created"
	EnvelopeTypeSpiritAllocationCreated        EnvelopeType = "spirit_allocation_created"
	EnvelopeTypeSpiritOrchestrationStarted     EnvelopeType = "spirit_orchestration_started"
	EnvelopeTypeSpiritOrchestrationCheckpoint  EnvelopeType = "spirit_orchestration_checkpoint"
	EnvelopeTypeSpiritOrchestrationInterrupted EnvelopeType = "spirit_orchestration_interrupted"

	// Planning phase timeline events (P1-2). Published by PrePlanningGate to make
	// the complexity assessment + gate decision observable on the frontend timeline.
	EnvelopeTypePlanningPhaseStart    EnvelopeType = "planning_phase_start"
	EnvelopeTypePlanningPhaseProgress EnvelopeType = "planning_phase_progress"
	EnvelopeTypePlanningPhaseDone     EnvelopeType = "planning_phase_done"

	// Wave 1 batch-registered event types (§12.0 preprocessing).
	// Registered upfront to avoid merge conflicts when P1-4/P1-7/P2-2/P2-3 land
	// in parallel waves. Each task will populate the corresponding Content payload.

	// EnvelopeTypeRunHeartbeat carries periodic run progress (percent/current step/ETA).
	// Published by RunHeartbeatEmitter every 10s so the frontend can detect stale
	// runs within 30s (P1-7). Classified as Informational (AS-EVT-01): loss only
	// degrades progress visibility, does not corrupt state.
	EnvelopeTypeRunHeartbeat EnvelopeType = "run_heartbeat"

	// EnvelopeTypeAgentCreated signals that AgentFactory dynamically created a new
	// Agent (P1-4). Frontend shows "系统创建了新 Agent [name]" notification.
	// Classified as Informational (AS-EVT-01): the Agent is already persisted to DB;
	// this event only drives UI visibility.
	EnvelopeTypeAgentCreated EnvelopeType = "agent_created"

	// EnvelopeTypeGraphReplanned signals that RuntimeReplanner adjusted the graph
	// topology after a node failure (P2-2): retry/reroute/insert_fallback/rebuild_subgraph.
	// Classified as Important (AS-EVT-01): loss causes frontend topology drift but
	// execution continues; persisted asynchronously.
	EnvelopeTypeGraphReplanned EnvelopeType = "graph_replanned"

	// EnvelopeTypeGraphTopologyEvolved signals that TopologyEvolver dynamically
	// added a transfer edge during execution (P2-3). Classified as Important
	// (AS-EVT-01): same rationale as GraphReplanned.
	EnvelopeTypeGraphTopologyEvolved EnvelopeType = "graph_topology_evolved"

	EnvelopeTypeTokenUsage     EnvelopeType = "token_usage"
	EnvelopeTypeMetricsUpdated EnvelopeType = "metrics_updated"

	// EnvelopeTypeExecutionProgress carries a single orchestration step's start/done/error
	// status. It is published to the chat channel so the AgentTreeTimeline can render
	// an inline progress card during the long 5-15s wait (e.g. LLM first byte).
	// See docs/reports/2026-06-10-proposal-execution-progress-inline.md
	EnvelopeTypeExecutionProgress EnvelopeType = "execution_progress"

	// Butler orchestration events
	EnvelopeTypeButlerOrchestrationStarted   EnvelopeType = "butler.orchestration.started"
	EnvelopeTypeButlerOrchestrationCompleted EnvelopeType = "butler.orchestration.completed"
	EnvelopeTypeButlerOrchestrationFailed    EnvelopeType = "butler.orchestration.failed"

	// Skill evolution events
	EnvelopeTypeSkillHealthChanged     EnvelopeType = "skill.health_changed"
	EnvelopeTypeSkillEvolutionProposed EnvelopeType = "skill.evolution_proposed"

	// Orchestration evolution events (DQ-score-driven closed loop)
	EnvelopeTypeOrchestrationEvolutionSuggested EnvelopeType = "orchestration.evolution_suggested"
	EnvelopeTypeOrchestrationCacheHit           EnvelopeType = "orchestration.cache_hit"

	// Monitor self-healing events
	EnvelopeTypeMonitorAutoHealed         EnvelopeType = "monitor.auto_healed"
	EnvelopeTypeMonitorSelfCheckCompleted EnvelopeType = "monitor.self_check_completed"

	// Borrow request events (cross-department collaboration)
	EnvelopeTypeBorrowApproved     EnvelopeType = "borrow.approved"
	EnvelopeTypeBorrowRejected     EnvelopeType = "borrow.rejected"
	EnvelopeTypeBorrowAutoApproved EnvelopeType = "borrow.auto_approved"

	// Organization CRUD events
	EnvelopeTypeOrganizationCreated EnvelopeType = "organization.created"
	EnvelopeTypeOrganizationUpdated EnvelopeType = "organization.updated"
	EnvelopeTypeOrganizationDeleted EnvelopeType = "organization.deleted"

	// Activity-First lifecycle events (AF phase)
	// ActivityProjector projects runtime events into Activity semantic units
	// and pushes them to the frontend, eliminating frontend inference.
	EnvelopeTypeActivityStart      EnvelopeType = "activity_start"
	EnvelopeTypeActivityDelta      EnvelopeType = "activity_delta"
	EnvelopeTypeActivityDone       EnvelopeType = "activity_done"
	EnvelopeTypeActivityChildStart EnvelopeType = "activity_child_start"

	// EnvelopeTypeLLMRetry signals that the LLM provider call is being retried
	// after a transient failure (5xx / 429 / network error). Published by the
	// provider retry transport so the frontend can show "正在重试" feedback.
	// Classified as Important (AS-EVT-01): loss degrades user feedback but does
	// not corrupt state; the retry itself still proceeds.
	EnvelopeTypeLLMRetry EnvelopeType = "llm_retry"
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

// EnvelopeToolCall error_code constants.
const (
	ErrorCodeToolTimeout          = "tool_timeout"
	ErrorCodeToolError            = "tool_error"
	ErrorCodeConfirmationRequired = "confirmation_required"
	ErrorCodeConfirmationDenied   = "confirmation_denied"
	ErrorCodeConfirmationTimeout  = "confirmation_timeout"
)

// ValidErrorCodes is the set of allowed error_code values for EnvelopeToolCall.
var ValidErrorCodes = map[string]bool{
	ErrorCodeToolTimeout:          true,
	ErrorCodeToolError:            true,
	ErrorCodeConfirmationRequired: true,
	ErrorCodeConfirmationDenied:   true,
	ErrorCodeConfirmationTimeout:  true,
}

// ValidateErrorCode ensures ErrorCode is one of the known values.
// Unknown codes are replaced with the generic ErrorCodeToolError fallback.
func (e *EnvelopeToolCall) ValidateErrorCode() {
	if e.ErrorCode != "" && !ValidErrorCodes[e.ErrorCode] {
		e.ErrorCode = ErrorCodeToolError
	}
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

type EnvelopeTrace struct {
	AgentName    string `json:"agent_name"`
	InvocationID string `json:"invocation_id"`
	StepCount    int    `json:"step_count"`
	DurationMS   int64  `json:"duration_ms,omitempty"`
}

// AgentCreatedContent is the Content payload for EnvelopeTypeAgentCreated (P1-4).
// It is carried in Envelope.Metadata (not Envelope.Content, which is reserved
// for chat text) so subscribers can render "系统创建了新 Agent" notifications.
type AgentCreatedContent struct {
	AgentKey    string `json:"agent_key"`
	DisplayName string `json:"display_name"`
	Source      string `json:"source"`  // "system" for AgentFactory-created agents
	Trigger     string `json:"trigger"` // the TaskProfile.TaskDescription that triggered creation
}

// RunHeartbeatContent is the metadata payload for EnvelopeTypeRunHeartbeat (P1-7).
// Carried in Envelope.Metadata so subscribers can detect stale runs within 30s.
type RunHeartbeatContent struct {
	RunID           string  `json:"run_id"`
	ProgressPercent float64 `json:"progress_percent"`
	CurrentStep     string  `json:"current_step"`
	TotalSteps      int     `json:"total_steps"`
	ETA             string  `json:"eta"`
}

// GraphTopologyEvolvedContent is the metadata payload for
// EnvelopeTypeGraphTopologyEvolved (P2-3). Carried in Envelope.Metadata so
// subscribers can render the topology evolution on the frontend timeline.
// Classified as Important (AS-EVT-01): loss causes topology drift but
// execution continues.
type GraphTopologyEvolvedContent struct {
	ExecutionID string `json:"execution_id"`
	GraphID     string `json:"graph_id"`
	FromNode    string `json:"from_node"`
	ToNode      string `json:"to_node"`
	EdgeKind    string `json:"edge_kind"`
	Reason      string `json:"reason"`
	Evidence    string `json:"evidence"`
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

// channelRegistry maps EnvelopeType to its logical channel name.
// New event types should register via RegisterChannelRoute instead of
// adding cases to RouteChannel.
var channelRegistry map[EnvelopeType]string

// RegisterChannelRoute registers the logical channel for an EnvelopeType.
// This is the OCP-compliant way to define routing: each domain registers
// its own event types at init time rather than modifying a central switch.
func RegisterChannelRoute(typ EnvelopeType, channel string) {
	if channelRegistry == nil {
		channelRegistry = make(map[EnvelopeType]string)
	}
	channelRegistry[typ] = channel
}

// RegisterChannelRoutes bulk-registers channel routes for multiple EnvelopeTypes.
func RegisterChannelRoutes(channel string, types ...EnvelopeType) {
	for _, t := range types {
		RegisterChannelRoute(t, channel)
	}
}

func init() {
	// Monitor channel: logs, flow logs, MCP health, alerts, self-healing
	RegisterChannelRoutes("monitor",
		EnvelopeTypeLog, EnvelopeTypeFlowLog,
		EnvelopeTypeMCPSessionReconnect, EnvelopeTypeMCPHealthAlert, EnvelopeTypeAlertNotify,
		EnvelopeTypeMonitorAutoHealed, EnvelopeTypeMonitorSelfCheckCompleted,
	)

	// Team channel: member messages, team run lifecycle, orchestration status
	RegisterChannelRoutes("team",
		EnvelopeTypeMemberMessageStart, EnvelopeTypeMemberDelta, EnvelopeTypeMemberMessageDone,
		EnvelopeTypeTeamRunStarted, EnvelopeTypeTeamRunFinished, EnvelopeTypeTeamStepStarted,
		EnvelopeTypeTeamStepFinished, EnvelopeTypeTeamRunFailed, EnvelopeTypeTeamSummary,
		EnvelopeTypeOrchestrationAgentStatus,
	)

	// Graph channel: graph node lifecycle, checkpoints, execution
	RegisterChannelRoutes("graph",
		EnvelopeTypeGraphNodeStart, EnvelopeTypeGraphNodeEnd, EnvelopeTypeCheckpoint,
		EnvelopeTypeGraphStep, EnvelopeTypeGraphExecutionDone, EnvelopeTypeGraphNodeError,
		EnvelopeTypeGraphNodeCustom, EnvelopeTypeGraphTaskStatus,
		// Runtime graph topology changes (P2-2/P2-3): replan + evolution visibility
		EnvelopeTypeGraphReplanned, EnvelopeTypeGraphTopologyEvolved,
	)

	// Knowledge channel: ingestion events
	RegisterChannelRoutes("knowledge",
		EnvelopeTypeKnowledgeIngest,
	)

	// Chat channel: session status, metrics, spirit orchestration, butler, skill evolution
	RegisterChannelRoutes("chat",
		EnvelopeTypeSessionStatusChanged, EnvelopeTypeMetricsUpdated,
		EnvelopeTypeSpiritTeamAssembled, EnvelopeTypeSpiritTeamCompleted, EnvelopeTypeSpiritTeamFailed,
		EnvelopeTypeSpiritTeamInterrupted,
		EnvelopeTypeSpiritTeamProgress, EnvelopeTypeSpiritTeamsAllCompleted, EnvelopeTypeSpiritSynthesisCompleted,
		EnvelopeTypeSpiritPlanCreated, EnvelopeTypeSpiritAllocationCreated,
		EnvelopeTypeSpiritOrchestrationStarted, EnvelopeTypeSpiritOrchestrationCheckpoint,
		EnvelopeTypeSpiritOrchestrationInterrupted,
		// Planning phase timeline (P1-2): gate decision visibility
		EnvelopeTypePlanningPhaseStart, EnvelopeTypePlanningPhaseProgress, EnvelopeTypePlanningPhaseDone,
		// Run heartbeat (P1-7): frontend stale-run detection within 30s
		EnvelopeTypeRunHeartbeat,
		// AgentFactory notification (P1-4): "系统创建了新 Agent" visibility
		EnvelopeTypeAgentCreated,
		EnvelopeTypeButlerOrchestrationStarted, EnvelopeTypeButlerOrchestrationCompleted,
		EnvelopeTypeButlerOrchestrationFailed,
		EnvelopeTypeSkillHealthChanged, EnvelopeTypeSkillEvolutionProposed,
		EnvelopeTypeOrchestrationEvolutionSuggested, EnvelopeTypeOrchestrationCacheHit,
		// Chat-visible execution progress (LLM invoke, intent pass, tool dispatch, etc.)
		// P0: covers the 5-15s silent wait. See proposal-execution-progress-inline.md.
		EnvelopeTypeExecutionProgress,
		// Organization CRUD events
		EnvelopeTypeOrganizationCreated, EnvelopeTypeOrganizationUpdated, EnvelopeTypeOrganizationDeleted,
		// Activity-First lifecycle events
		EnvelopeTypeActivityStart, EnvelopeTypeActivityDelta, EnvelopeTypeActivityDone, EnvelopeTypeActivityChildStart,
		// LLM retry feedback (T1.2): "正在重试" notification
		EnvelopeTypeLLMRetry,
	)
}

// RouteChannel returns the logical channel name for routing an envelope.
// Routes are resolved via the registry (populated by RegisterChannelRoute).
// Unregistered types fall back to TeamID-based inference or "chat" as default.
func RouteChannel(env Envelope) string {
	if ch, ok := channelRegistry[env.Type]; ok {
		return ch
	}
	if env.TeamID != "" {
		return "team"
	}
	return "chat"
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
