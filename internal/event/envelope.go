package event

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

type EnvelopeType string

const (
	EnvelopeTypeTextDelta          EnvelopeType = "text_delta"
	EnvelopeTypeTextDone           EnvelopeType = "text_done"
	EnvelopeTypeToolCall           EnvelopeType = "tool_call"
	EnvelopeTypeToolResult         EnvelopeType = "tool_result"
	EnvelopeTypeStateDelta         EnvelopeType = "state_delta"
	EnvelopeTypeTransfer           EnvelopeType = "transfer"
	EnvelopeTypeRunnerCompletion   EnvelopeType = "runner_completion"
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
	EnvelopeTypeKnowledgeIngest    EnvelopeType = "knowledge_ingest"
	EnvelopeTypeMCPSessionReconnect EnvelopeType = "mcp.session.reconnect"
	EnvelopeTypeAlertNotify        EnvelopeType = "alert.notify"
)

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
	Extensions map[string]string   `json:"extensions,omitempty"`
	Actions    *EnvelopeActions    `json:"actions,omitempty"`
	Trace      *EnvelopeTrace      `json:"trace,omitempty"`
	Metadata   map[string]any      `json:"metadata,omitempty"`
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
	Message   string `json:"message"`
	PendingID string `json:"pending_id,omitempty"`
}

type EnvelopeUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type EnvelopeActions struct {
	SkipSummarization bool `json:"skip_summarization,omitempty"`
}

type EnvelopeTrace struct {
	AgentName    string `json:"agent_name"`
	InvocationID string `json:"invocation_id"`
	StepCount    int    `json:"step_count"`
	DurationMS   int64  `json:"duration_ms,omitempty"`
}

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

func RouteChannel(env Envelope) string {
	switch env.Type {
	case EnvelopeTypeLog, EnvelopeTypeFlowLog:
		return "monitor"
	case EnvelopeTypeMemberMessageStart, EnvelopeTypeMemberDelta, EnvelopeTypeMemberMessageDone,
		EnvelopeTypeTeamRunStarted, EnvelopeTypeTeamRunFinished, EnvelopeTypeTeamStepStarted,
		EnvelopeTypeTeamStepFinished, EnvelopeTypeTeamRunFailed, EnvelopeTypeTeamSummary:
		return "team"
	case EnvelopeTypeGraphNodeStart, EnvelopeTypeGraphNodeEnd, EnvelopeTypeCheckpoint,
		EnvelopeTypeGraphStep, EnvelopeTypeGraphExecutionDone, EnvelopeTypeGraphNodeError,
		EnvelopeTypeGraphNodeCustom:
		return "graph"
	case EnvelopeTypeKnowledgeIngest:
		return "knowledge"
	case EnvelopeTypeMCPSessionReconnect, EnvelopeTypeAlertNotify:
		return "monitor"
	default:
		if env.TeamID != "" {
			return "team"
		}
		return "chat"
	}
}

func MatchFilterKey(subscriberKey, eventKey string) bool {
	if subscriberKey == "" || eventKey == "" {
		return true
	}
	sk := subscriberKey + "/"
	ek := eventKey + "/"
	return strings.HasPrefix(sk, ek) || strings.HasPrefix(ek, sk)
}

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
