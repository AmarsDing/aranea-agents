package biz

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Meta keys used to carry DomainEvent fields that have no direct counterpart
// on the Activity struct. Storing them in Activity.Meta allows lossless
// round-trip conversion between DomainEvent and ActivityEvent.
const (
	metaDomainEventType = "domain_event_type" // original DomainEvent.Type
	metaAuthor          = "author"            // DomainEvent.Author
	metaRequestID       = "request_id"
	metaInvocationID    = "invocation_id"
	metaRunID           = "run_id"
	metaTraceID         = "trace_id"
	metaRunKind         = "run_kind"
	metaUsageEventID    = "usage_event_id"
	metaTurnStartedAt   = "turn_started_at" // RFC3339Nano

	// presence markers for optional structs (distinguish present-but-zero from absent)
	metaHasContent    = "has_content"
	metaHasStateDelta = "has_state_delta"
	metaHasError      = "has_error"
	metaHasUsage      = "has_usage"
	metaHasToolCall   = "has_tool_call"
	metaHasGraphNode  = "has_graph_node"

	// scalar fields stored in Meta
	metaIsPartial        = "is_partial"
	metaStateDeltaOp     = "state_delta_op"
	metaStateDeltaPath   = "state_delta_path"
	metaStateDeltaValue  = "state_delta_value_json"
	metaErrorType        = "error_type"
	metaErrorMessage     = "error_message"
	metaUsageTotalTokens = "usage_total_tokens"
	metaToolCallStatus   = "tool_call_status"
	metaGraphNodeID      = "graph_node_id"
	metaGraphNodeError   = "graph_node_error"
)

// --- field-level conversion helpers (eliminate duplicated mapping between directions) ---

func copyContentToActivity(src *DomainContent, act *Activity, meta map[string]any) {
	act.Content = src.Text
	act.Reasoning = src.Reasoning
	meta[metaHasContent] = true
	meta[metaIsPartial] = src.IsPartial
}

func copyContentFromActivity(act Activity, meta map[string]any) *DomainContent {
	if !metaBool(meta, metaHasContent) {
		return nil
	}
	return &DomainContent{
		Text:      act.Content,
		Reasoning: act.Reasoning,
		IsPartial: metaBool(meta, metaIsPartial),
	}
}

func copyStateDeltaToActivity(src *DomainStateDelta, meta map[string]any) {
	meta[metaHasStateDelta] = true
	meta[metaStateDeltaOp] = src.Operation
	meta[metaStateDeltaPath] = src.Path
	meta[metaStateDeltaValue] = src.ValueJSON
}

func copyStateDeltaFromActivity(meta map[string]any) *DomainStateDelta {
	if !metaBool(meta, metaHasStateDelta) {
		return nil
	}
	return &DomainStateDelta{
		Operation: metaString(meta, metaStateDeltaOp),
		Path:      metaString(meta, metaStateDeltaPath),
		ValueJSON: metaString(meta, metaStateDeltaValue),
	}
}

func copyErrorToActivity(src *DomainError, act *Activity, meta map[string]any) {
	meta[metaHasError] = true
	meta[metaErrorType] = src.Type
	meta[metaErrorMessage] = src.Message
	if src.Type != "" {
		act.ToolErrorCode = src.Type
	}
}

func copyErrorFromActivity(act Activity, meta map[string]any) *DomainError {
	if !metaBool(meta, metaHasError) {
		return nil
	}
	return &DomainError{
		Type:    metaString(meta, metaErrorType),
		Message: metaString(meta, metaErrorMessage),
	}
}

func copyUsageToActivity(src *DomainUsage, act *Activity, meta map[string]any) {
	act.PromptTokens = int64(src.PromptTokens)
	act.CompletionTokens = int64(src.CompletionTokens)
	meta[metaHasUsage] = true
	meta[metaUsageTotalTokens] = src.TotalTokens
}

func copyUsageFromActivity(act Activity, meta map[string]any) *DomainUsage {
	if !metaBool(meta, metaHasUsage) {
		return nil
	}
	return &DomainUsage{
		PromptTokens:     int(act.PromptTokens),
		CompletionTokens: int(act.CompletionTokens),
		TotalTokens:      metaInt(meta, metaUsageTotalTokens),
	}
}

func copyToolCallToActivity(src *DomainToolCall, act *Activity, meta map[string]any) {
	act.ToolCallID = src.ID
	act.ToolName = src.Name
	act.ToolArguments = src.ArgumentsJSON
	act.ToolResult = src.ResultJSON
	act.ToolDurationMs = src.DurationMS
	meta[metaHasToolCall] = true
	meta[metaToolCallStatus] = src.Status
}

func copyToolCallFromActivity(act Activity, meta map[string]any) *DomainToolCall {
	if !metaBool(meta, metaHasToolCall) {
		return nil
	}
	return &DomainToolCall{
		ID:            act.ToolCallID,
		Name:          act.ToolName,
		ArgumentsJSON: act.ToolArguments,
		ResultJSON:    act.ToolResult,
		Status:        metaString(meta, metaToolCallStatus),
		DurationMS:    act.ToolDurationMs,
	}
}

func copyGraphNodeToActivity(src *DomainGraphNode, meta map[string]any) {
	meta[metaHasGraphNode] = true
	meta[metaGraphNodeID] = src.NodeID
	meta[metaGraphNodeError] = src.Error
}

func copyGraphNodeFromActivity(meta map[string]any) *DomainGraphNode {
	if !metaBool(meta, metaHasGraphNode) {
		return nil
	}
	return &DomainGraphNode{
		NodeID: metaString(meta, metaGraphNodeID),
		Error:  metaString(meta, metaGraphNodeError),
	}
}

// --- top-level conversion ---

// domainEventToActivityEvent converts a biz.DomainEvent into a
// biz.ActivityEvent with Domain=system. The original DomainEvent type and
// correlation fields are preserved in Activity.Meta for round-trip fidelity.
func domainEventToActivityEvent(de DomainEvent) ActivityEvent {
	if de.ID == "" {
		de.ID = uuid.NewString()
	}
	ts := de.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}

	kind, status := domainEventKindStatus(de.Type, de.Error != nil)
	meta := make(map[string]any, 16)
	act := Activity{
		ID:         de.ID,
		Kind:       kind,
		Status:     status,
		SessionID:  de.SessionID,
		TeamID:     de.TeamID,
		Timestamp:  ts,
		DurationMs: de.DurationMS,
		AgentKey:   de.AgentID,
		AgentName:  de.AgentDisplayName,
		Meta:       meta,
	}

	if de.Content != nil {
		copyContentToActivity(de.Content, &act, meta)
	}
	if de.StateDelta != nil {
		copyStateDeltaToActivity(de.StateDelta, meta)
	}
	if de.Error != nil {
		copyErrorToActivity(de.Error, &act, meta)
	}
	if de.Usage != nil {
		copyUsageToActivity(de.Usage, &act, meta)
	}
	if de.ToolCall != nil {
		copyToolCallToActivity(de.ToolCall, &act, meta)
	}
	if de.GraphNode != nil {
		copyGraphNodeToActivity(de.GraphNode, meta)
	}

	storeActivityCorrelation(&de, meta)

	return ActivityEvent{
		Event:    ActivityEventCreated,
		Activity: act,
		Domain:   ActivityDomainSystem,
	}
}

// activityEventToDomainEvent converts a system-domain ActivityEvent back into
// a biz.DomainEvent. Returns nil for chat-domain events (they should not reach
// DomainEvent subscribers).
func activityEventToDomainEvent(ev ActivityEvent) *DomainEvent {
	if ev.Domain != ActivityDomainSystem {
		return nil
	}
	act := ev.Activity
	meta := act.Meta
	if meta == nil {
		meta = map[string]any{}
	}

	ts := act.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}
	de := &DomainEvent{
		ID:               act.ID,
		Type:             DomainEventType(metaString(meta, metaDomainEventType)),
		SessionID:        act.SessionID,
		TeamID:           act.TeamID,
		Timestamp:        ts,
		DurationMS:       act.DurationMs,
		AgentID:          act.AgentKey,
		AgentDisplayName: act.AgentName,
	}

	if de.Content = copyContentFromActivity(act, meta); de.Content == nil {
		// fallback: reconstruct from non-empty Activity content fields
		if act.Content != "" || act.Reasoning != "" {
			de.Content = &DomainContent{
				Text:      act.Content,
				Reasoning: act.Reasoning,
				IsPartial: metaBool(meta, metaIsPartial),
			}
		}
	}
	de.StateDelta = copyStateDeltaFromActivity(meta)
	de.Error = copyErrorFromActivity(act, meta)
	de.Usage = copyUsageFromActivity(act, meta)
	de.ToolCall = copyToolCallFromActivity(act, meta)
	de.GraphNode = copyGraphNodeFromActivity(meta)

	applyActivityCorrelation(de, meta)
	return de
}

// applyActivityCorrelation copies correlation IDs from the meta map onto the
// DomainEvent. RunID falls back to InvocationID when not explicitly set.
func applyActivityCorrelation(de *DomainEvent, meta map[string]any) {
	de.Author = metaString(meta, metaAuthor)
	de.RequestID = strings.TrimSpace(metaString(meta, metaRequestID))
	de.InvocationID = strings.TrimSpace(metaString(meta, metaInvocationID))
	de.RunID = metaString(meta, metaRunID)
	de.TraceID = metaString(meta, metaTraceID)
	de.RunKind = metaString(meta, metaRunKind)
	de.UsageEventID = metaString(meta, metaUsageEventID)
	if v := metaString(meta, metaTurnStartedAt); v != "" {
		if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
			de.TurnStartedAt = t
		}
	}
	if de.RunID == "" {
		de.RunID = de.InvocationID
	}
}

// applyActivityCorrelation for the forward direction (DomainEvent → meta).
func storeActivityCorrelation(de *DomainEvent, meta map[string]any) {
	meta[metaDomainEventType] = string(de.Type)
	meta[metaAuthor] = de.Author
	if de.RequestID != "" {
		meta[metaRequestID] = de.RequestID
	}
	if de.InvocationID != "" {
		meta[metaInvocationID] = de.InvocationID
	}
	if de.RunID != "" {
		meta[metaRunID] = de.RunID
	}
	if de.TraceID != "" {
		meta[metaTraceID] = de.TraceID
	}
	if de.RunKind != "" {
		meta[metaRunKind] = de.RunKind
	}
	if de.UsageEventID != "" {
		meta[metaUsageEventID] = de.UsageEventID
	}
	if !de.TurnStartedAt.IsZero() {
		meta[metaTurnStartedAt] = de.TurnStartedAt.Format(time.RFC3339Nano)
	}
}

// domainEventKindStatus maps a DomainEventType (+ error presence) to the
// appropriate ActivityKind and ActivityStatus.
func domainEventKindStatus(t DomainEventType, hasError bool) (ActivityKind, ActivityStatus) {
	switch t {
	case DomainEventTextDelta:
		return ActivityKindReply, ActivityStatusCompleted
	case DomainEventToolCall:
		if hasError {
			return ActivityKindAction, ActivityStatusFailed
		}
		return ActivityKindAction, ActivityStatusRunning
	case DomainEventToolResult:
		if hasError {
			return ActivityKindAction, ActivityStatusFailed
		}
		return ActivityKindAction, ActivityStatusCompleted
	case DomainEventRunnerCompletion:
		if hasError {
			return ActivityKindTask, ActivityStatusFailed
		}
		return ActivityKindTask, ActivityStatusCompleted
	case DomainEventError:
		return ActivityKindNotice, ActivityStatusFailed
	case DomainEventGraphNodeStart:
		return ActivityKindGraphStage, ActivityStatusRunning
	case DomainEventGraphNodeEnd:
		return ActivityKindGraphStage, ActivityStatusCompleted
	case DomainEventGraphNodeError:
		return ActivityKindGraphStage, ActivityStatusFailed
	case DomainEventGraphInterrupt:
		return ActivityKindGraphStage, ActivityStatusInterrupted
	default:
		return ActivityKindNotice, ActivityStatusCompleted
	}
}

// --- shared meta helpers (used by other biz files) ---

func metaString(meta map[string]any, key string) string {
	v, ok := meta[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case json.Number:
		return t.String()
	default:
		return strings.TrimSpace(fmt.Sprint(t))
	}
}

func metaBool(meta map[string]any, key string) bool {
	v, ok := meta[key]
	if !ok || v == nil {
		return false
	}
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return strings.EqualFold(strings.TrimSpace(t), "true") || strings.TrimSpace(t) == "1"
	default:
		return false
	}
}

// metaInt reads an integer value from a meta map. Handles int, int64, float64,
// json.Number, and string representations.
func metaInt(meta map[string]any, key string) int {
	v, ok := meta[key]
	if !ok || v == nil {
		return 0
	}
	switch t := v.(type) {
	case int:
		return t
	case int64:
		return int(t)
	case float64:
		return int(t)
	case json.Number:
		n, err := t.Int64()
		if err != nil {
			return 0
		}
		return int(n)
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(t))
		if err != nil {
			return 0
		}
		return n
	default:
		return 0
	}
}
