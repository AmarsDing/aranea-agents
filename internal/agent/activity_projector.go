package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"

	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// Graph node event object types (mirrored from trpc-agent-go/graph to avoid
// importing the graph package, which pulls in broken transitive dependencies).
const (
	graphObjectTypeNodeStart   = "graph.node.start"
	graphObjectTypeNodeComplete = "graph.node.complete"
	graphObjectTypeNodeError   = "graph.node.error"
	graphMetadataKeyNode       = "_node_metadata"
)

// graphNodeMetadata mirrors trpc-agent-go/graph.NodeExecutionMetadata
// (only the fields we need for plan step mapping).
type graphNodeMetadata struct {
	NodeID      string `json:"nodeId"`
	NodeType    string `json:"nodeType,omitempty"`
	StepNumber  int    `json:"stepNumber,omitempty"`
	ModelName   string `json:"modelName,omitempty"`
}

// planStepLabel builds a human-readable label for a plan step from graph node
// metadata. Falls back to NodeID when no richer info is available.
//
// Priority: StepNumber + NodeType → StepNumber → NodeType + NodeID → NodeID
func planStepLabel(meta graphNodeMetadata) string {
	typeLabel := nodeTypeLabel(meta.NodeType)
	if meta.StepNumber > 0 && typeLabel != "" {
		return fmt.Sprintf("步骤 %d · %s", meta.StepNumber, typeLabel)
	}
	if meta.StepNumber > 0 {
		return fmt.Sprintf("步骤 %d", meta.StepNumber)
	}
	if typeLabel != "" {
		return fmt.Sprintf("%s · %s", typeLabel, meta.NodeID)
	}
	return meta.NodeID
}

// nodeTypeLabel maps a graph NodeType to a Chinese display label.
func nodeTypeLabel(nodeType string) string {
	switch strings.TrimSpace(nodeType) {
	case "function":
		return "函数"
	case "llm":
		return "LLM"
	case "tool":
		return "工具"
	case "agent":
		return "Agent"
	case "join":
		return "汇合"
	case "router":
		return "路由"
	default:
		return ""
	}
}

// ActivityProjector projects runtime events into Activity semantic units
// and publishes them via WS, eliminating frontend inference.
// It runs parallel to EventProjector during the AF-1 dual-emission phase.
type ActivityProjector struct {
	mu           sync.Mutex
	eventBus     event.Bus
	activityRepo biz.ActivityWriter
	metaResolver ActivityMetaResolver
	lg           loggateway.Logger

	// sequencer guarantees per-activity FIFO event ordering.
	// Each activity gets its own channel and consumer goroutine.
	// This fixes B-01 (start/delta ordering), B-04 (delta holds global lock),
	// and B-05 (async start races with sync delta).
	sequencer *activityEventSequencer

	// Turn-scoped state (reset per turn)
	rootActivityID string
	activities     map[string]*biz.Activity // id -> activity
	toolCalls      map[string]*biz.Activity // tool_call_id -> action activity
	kindAuthorMap  map[string]string        // "kind:author" -> activity ID (O(1) lookup)
	reasoningBuf   map[string]*strings.Builder
	meta           ProjectMeta
	planActivityID string                   // current turn's plan activity ID (graph node events)
	planStepIndex  int                      // monotonic counter for plan steps within this turn

	// resetDone prevents Reset() from clearing state that was initialized
	// before stream consumption started. When the projector is pre-created
	// (e.g. in invokeTurnLLMAndStream for early emission access), Reset()
	// is called once to initialize maps; subsequent Reset() calls from
	// newTurnStreamConsumer become no-ops to avoid clearing activities
	// emitted by hooks/plugins during the LLM call.
	resetDone bool

	// turnStarted prevents duplicate root task activities when OnTurnStart
	// is called both early (pre-creation path) and in newTurnStreamConsumer.
	turnStarted bool
}

// NewActivityProjector creates a new ActivityProjector.
func NewActivityProjector(eventBus event.Bus, activityRepo biz.ActivityWriter, lg loggateway.Logger) *ActivityProjector {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	p := &ActivityProjector{
		eventBus:     eventBus,
		activityRepo: activityRepo,
		lg:           lg,
	}
	p.sequencer = newActivityEventSequencer(eventBus, lg)
	p.sequencer.activityRepo = activityRepo
	return p
}

// Close releases resources held by the projector, including the event sequencer.
// It blocks until all queued events have been published and persisted.
// After Close, subsequent publish operations are silently dropped.
// Close must be called when the projector is no longer needed (typically after
// stream consumption finalizes) to prevent goroutine leaks.
func (p *ActivityProjector) Close() {
	if p.sequencer != nil {
		p.sequencer.Close()
	}
}

// Configure sets the ProjectMeta for the current turn.
func (p *ActivityProjector) Configure(meta ProjectMeta, resolver ActivityMetaResolver) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.meta = meta
	p.metaResolver = resolver
}

// Reset clears turn-scoped state. Called at the start of each turn.
// When the projector is pre-created (resetDone=true), Reset() becomes a
// no-op to preserve activities emitted by hooks/plugins before stream
// consumption started.
func (p *ActivityProjector) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.resetDone {
		return
	}
	p.rootActivityID = ""
	p.activities = make(map[string]*biz.Activity)
	p.toolCalls = make(map[string]*biz.Activity)
	p.kindAuthorMap = make(map[string]string)
	p.reasoningBuf = make(map[string]*strings.Builder)
	p.planActivityID = ""
	p.planStepIndex = 0
	p.resetDone = true
	p.turnStarted = false
}

// ProcessEvent dispatches a trpc-agent-go event to the appropriate On* callback.
// This is the AF-3 entry point: ActivityProjector directly consumes trpc events
// instead of depending on EventProjector output.
func (p *ActivityProjector) ProcessEvent(ctx context.Context, ev *trpcevent.Event) {
	if ev == nil {
		return
	}

	// RunnerCompletion is handled by OnTurnEnd (called separately from finalize).
	// Error events are forwarded to OnError.
	if ev.Response != nil && ev.Response.Error != nil {
		errType := ev.Response.Error.Type
		if errType == "" {
			errType = "run_error"
		}
		p.OnError(ctx, ev.Response.Error.Message, errType, "")
		return
	}

	if ev.Response == nil {
		return
	}

	objType := ev.Response.Object
	switch objType {
	case trpcmodel.ObjectTypeChatCompletionChunk:
		p.processChatCompletionChunk(ctx, ev)
	case trpcmodel.ObjectTypeChatCompletion:
		p.processChatCompletion(ctx, ev)
	case trpcmodel.ObjectTypeToolResponse:
		p.processToolResponse(ctx, ev)
	case graphObjectTypeNodeStart:
		p.processGraphNodeStart(ctx, ev)
	case graphObjectTypeNodeComplete, graphObjectTypeNodeError:
		p.processGraphNodeComplete(ctx, ev)
	}
}

// processChatCompletionChunk handles streaming chat completion events.
func (p *ActivityProjector) processChatCompletionChunk(ctx context.Context, ev *trpcevent.Event) {
	for _, choice := range ev.Response.Choices {
		msg := choice.Message
		delta := choice.Delta
		author := ev.Author

		// Tool calls
		allToolCalls := append(msg.ToolCalls, delta.ToolCalls...)
		for _, tc := range allToolCalls {
			startedAt := time.Now().UTC()
			if !ev.Timestamp.IsZero() {
				startedAt = ev.Timestamp.UTC()
			}
			p.OnToolCall(ctx, tc.ID, tc.Function.Name, string(tc.Function.Arguments), author, startedAt)
		}

		// Text and reasoning content
		text, reasoning := ChoiceStreamContent(choice, ev.Response.IsPartial)
		// AF-GAP-04: Route team member authors to OnMemberMessage* so the
		// resulting reply Activity carries meta.member_id for frontend
		// differentiation. Non-member authors (coordinator, single-agent chat)
		// continue through OnTextDelta/OnTextDone/OnReasoning*.
		isMember := isTeamMemberAuthor(author, p.meta)
		if text != "" {
			if ev.Response.IsPartial {
				if isMember {
					p.OnMemberMessageDelta(ctx, author, text)
				} else {
					p.OnTextDelta(ctx, author, text)
				}
			} else {
				if isMember {
					p.OnMemberMessageDone(ctx, author, text)
				} else {
					p.OnTextDone(ctx, author, text)
				}
			}
		}
		if reasoning != "" {
			if ev.Response.IsPartial {
				p.OnReasoningDelta(ctx, author, reasoning, true)
			} else {
				p.OnReasoningDone(ctx, author, reasoning, false)
			}
		}
	}
}

// processChatCompletion handles non-streaming chat completion events.
func (p *ActivityProjector) processChatCompletion(ctx context.Context, ev *trpcevent.Event) {
	for _, choice := range ev.Response.Choices {
		msg := choice.Message
		author := ev.Author

		// Tool calls
		for _, tc := range msg.ToolCalls {
			startedAt := time.Now().UTC()
			if !ev.Timestamp.IsZero() {
				startedAt = ev.Timestamp.UTC()
			}
			p.OnToolCall(ctx, tc.ID, tc.Function.Name, string(tc.Function.Arguments), author, startedAt)
		}

		// Text and reasoning content
		text := strings.TrimSpace(msg.Content)
		reasoning := strings.TrimSpace(msg.ReasoningContent)
		// AF-GAP-04: Route team member authors to OnMemberMessage* so the
		// resulting reply Activity carries meta.member_id.
		isMember := isTeamMemberAuthor(author, p.meta)
		if text != "" {
			if isMember {
				p.OnMemberMessageDone(ctx, author, text)
			} else {
				p.OnTextDone(ctx, author, text)
			}
		}
		if reasoning != "" {
			p.OnReasoningDone(ctx, author, reasoning, false)
		}
	}
}

// processToolResponse handles tool result events.
func (p *ActivityProjector) processToolResponse(ctx context.Context, ev *trpcevent.Event) {
	if len(ev.Response.Choices) == 0 {
		return
	}
	msg := ev.Response.Choices[0].Message
	toolID := strings.TrimSpace(msg.ToolID)
	if toolID == "" {
		return
	}

	resultRaw, _ := json.Marshal(msg.Content)
	resultJSON := string(resultRaw)
	status := "success"
	errorCode := ""
	if ev.Response.Error != nil {
		status = "failed"
		errorCode = ev.Response.Error.Type
		if errorCode == "" {
			errorCode = "tool_error"
		}
	}

	// Calculate duration from cached tool call
	var durationMs int64
	p.mu.Lock()
	if a, ok := p.toolCalls[toolID]; ok {
		if !a.Timestamp.IsZero() {
			finishedAt := time.Now().UTC()
			if !ev.Timestamp.IsZero() {
				finishedAt = ev.Timestamp.UTC()
			}
			durationMs = finishedAt.Sub(a.Timestamp).Milliseconds()
			if durationMs < 0 {
				durationMs = 0
			}
		}
	}
	p.mu.Unlock()

	p.OnToolResult(ctx, toolID, resultJSON, status, errorCode, durationMs)
}

// OnTurnStart creates the root task Activity for a new turn.
// When called multiple times (e.g. pre-creation path + newTurnStreamConsumer),
// only the first call creates the root activity; subsequent calls are no-ops.
func (p *ActivityProjector) OnTurnStart(ctx context.Context, meta ProjectMeta) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.turnStarted {
		return
	}
	p.turnStarted = true
	p.meta = meta

	id := uuid.NewString()
	now := time.Now().UTC()
	a := &biz.Activity{
		ID:              id,
		Kind:            biz.ActivityKindTask,
		Status:          biz.ActivityStatusRunning,
		SessionID:       meta.SessionID,
		TurnID:          meta.RequestID,
		Timestamp:       now,
		AgentKey:        meta.AgentID,
		AgentName:       meta.AgentDisplayName,
		SpiritSessionID: meta.SessionID,
		TeamID:          meta.TeamID,
		Content:         meta.TaskContent,
	}
	p.rootActivityID = id
	p.activities[id] = a

	p.publishAndPersist(ctx, a, contract.EnvelopeTypeActivityStart)
}

// OnReasoningDelta handles a reasoning content chunk during streaming.
func (p *ActivityProjector) OnReasoningDelta(ctx context.Context, author string, chunk string, isPartial bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	key := author
	if p.reasoningBuf[key] == nil {
		p.reasoningBuf[key] = &strings.Builder{}
	}
	p.reasoningBuf[key].WriteString(chunk)

	// Find or create the thinking activity for this author
	activityID := p.findActivityByKindAuthor(biz.ActivityKindThinking, author)
	if activityID == "" {
		// Create new thinking activity
		id := uuid.NewString()
		now := time.Now().UTC()
		a := &biz.Activity{
			ID:               id,
			Kind:             biz.ActivityKindThinking,
			Status:           biz.ActivityStatusRunning,
			SessionID:        p.meta.SessionID,
			TurnID:           p.meta.RequestID,
			ParentActivityID: p.rootActivityID,
			Timestamp:        now,
			AgentKey:         author,
			AgentName:        p.resolveAgentName(ctx, author),
			SpiritSessionID:  p.meta.SessionID,
			TeamID:           p.meta.TeamID,
		}
		p.activities[id] = a
		p.kindAuthorMap[kindKey(biz.ActivityKindThinking, author)] = id
		p.publishAndPersist(ctx, a, contract.EnvelopeTypeActivityStart)
		activityID = id
	}

	// Emit delta
	a := p.activities[activityID]
	p.publishActivityDelta(ctx, a, "reasoning", chunk)
}

// OnReasoningDone finalizes a thinking activity. If reasoning_as_display is true,
// the activity is upgraded to reply kind.
func (p *ActivityProjector) OnReasoningDone(ctx context.Context, author string, fullReasoning string, reasoningAsDisplay bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	activityID := p.findActivityByKindAuthor(biz.ActivityKindThinking, author)
	if activityID == "" {
		return
	}
	a := p.activities[activityID]

	// If reasoning_as_display, upgrade to reply
	if reasoningAsDisplay {
		// Update kindAuthorMap: remove old thinking key, add reply key
		delete(p.kindAuthorMap, kindKey(biz.ActivityKindThinking, author))
		p.kindAuthorMap[kindKey(biz.ActivityKindReply, author)] = activityID
		a.Kind = biz.ActivityKindReply
		a.Content = fullReasoning
		a.Reasoning = ""
	} else {
		a.Reasoning = fullReasoning
		// Remove completed thinking from lookup so the next ReAct round
		// creates a new thinking Activity instead of appending to this one.
		// This mirrors OnToolResult's delete(p.toolCalls, toolCallID) pattern.
		delete(p.kindAuthorMap, kindKey(biz.ActivityKindThinking, author))
	}

	a.Status = biz.ActivityStatusCompleted
	now := time.Now().UTC()
	a.DurationMs = now.Sub(a.Timestamp).Milliseconds()
	a.Collapsed = true

	p.publishAndPersist(ctx, a, contract.EnvelopeTypeActivityDone)
}

// OnTextDelta handles a text content chunk during streaming.
func (p *ActivityProjector) OnTextDelta(ctx context.Context, author string, chunk string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Find or create reply activity
	activityID := p.findActivityByKindAuthor(biz.ActivityKindReply, author)
	if activityID == "" {
		id := uuid.NewString()
		now := time.Now().UTC()
		a := &biz.Activity{
			ID:               id,
			Kind:             biz.ActivityKindReply,
			Status:           biz.ActivityStatusRunning,
			SessionID:        p.meta.SessionID,
			TurnID:           p.meta.RequestID,
			ParentActivityID: p.rootActivityID,
			Timestamp:        now,
			AgentKey:         author,
			AgentName:        p.resolveAgentName(ctx, author),
			SpiritSessionID:  p.meta.SessionID,
			TeamID:           p.meta.TeamID,
		}
		p.activities[id] = a
		p.kindAuthorMap[kindKey(biz.ActivityKindReply, author)] = id
		p.publishAndPersist(ctx, a, contract.EnvelopeTypeActivityStart)
		activityID = id
	}

	a := p.activities[activityID]
	p.publishActivityDelta(ctx, a, "content", chunk)
}

// OnMemberMessageDelta handles a text content chunk for a team member message.
// Unlike OnTextDelta, the resulting reply Activity is tagged with
// meta.member_id so the frontend can distinguish member replies from the
// coordinator's reply (AF-GAP-04).
func (p *ActivityProjector) OnMemberMessageDelta(ctx context.Context, author string, chunk string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Find or create member reply activity
	activityID := p.findActivityByKindAuthor(biz.ActivityKindReply, author)
	if activityID == "" {
		id := uuid.NewString()
		now := time.Now().UTC()
		a := &biz.Activity{
			ID:               id,
			Kind:             biz.ActivityKindReply,
			Status:           biz.ActivityStatusRunning,
			SessionID:        p.meta.SessionID,
			TurnID:           p.meta.RequestID,
			ParentActivityID: p.rootActivityID,
			Timestamp:        now,
			AgentKey:         author,
			AgentName:        p.resolveAgentName(ctx, author),
			SpiritSessionID:  p.meta.SessionID,
			TeamID:           p.meta.TeamID,
			Meta:             map[string]any{"member_id": author},
		}
		p.activities[id] = a
		p.kindAuthorMap[kindKey(biz.ActivityKindReply, author)] = id
		p.publishAndPersist(ctx, a, contract.EnvelopeTypeActivityStart)
		activityID = id
	}

	a := p.activities[activityID]
	p.publishActivityDelta(ctx, a, "content", chunk)
}

// OnMemberMessageDone finalizes a team member's reply activity.
func (p *ActivityProjector) OnMemberMessageDone(ctx context.Context, author string, fullText string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	activityID := p.findActivityByKindAuthor(biz.ActivityKindReply, author)
	if activityID == "" {
		// Defensive: ignore Done without a prior Delta (e.g. empty member message)
		return
	}
	a := p.activities[activityID]
	a.Content = fullText
	a.Status = biz.ActivityStatusCompleted
	now := time.Now().UTC()
	a.DurationMs = now.Sub(a.Timestamp).Milliseconds()
	a.Collapsed = false

	// Remove completed reply from lookup so the next member message
	// creates a new reply Activity.
	delete(p.kindAuthorMap, kindKey(biz.ActivityKindReply, author))

	p.publishAndPersist(ctx, a, contract.EnvelopeTypeActivityDone)
}

// OnTextDone finalizes a reply activity.
func (p *ActivityProjector) OnTextDone(ctx context.Context, author string, fullText string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	activityID := p.findActivityByKindAuthor(biz.ActivityKindReply, author)
	if activityID == "" {
		return
	}
	a := p.activities[activityID]
	a.Content = fullText
	a.Status = biz.ActivityStatusCompleted
	now := time.Now().UTC()
	a.DurationMs = now.Sub(a.Timestamp).Milliseconds()
	a.Collapsed = false

	// Remove completed reply from lookup so the next ReAct round
	// creates a new reply Activity instead of appending to this one.
	delete(p.kindAuthorMap, kindKey(biz.ActivityKindReply, author))

	p.publishAndPersist(ctx, a, contract.EnvelopeTypeActivityDone)
}

// OnToolCall creates an action Activity when a tool call is detected.
func (p *ActivityProjector) OnToolCall(ctx context.Context, toolCallID, toolName, argsJSON, author string, startedAt time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()

	id := uuid.NewString()
	a := &biz.Activity{
		ID:               id,
		Kind:             biz.ActivityKindAction,
		Status:           biz.ActivityStatusToolRunning,
		SessionID:        p.meta.SessionID,
		TurnID:           p.meta.RequestID,
		ParentActivityID: p.rootActivityID,
		Timestamp:        startedAt,
		ToolName:         toolName,
		ToolCallID:       toolCallID,
		ToolArguments:    argsJSON,
		AgentKey:         author,
		AgentName:        p.resolveAgentName(ctx, author),
		SpiritSessionID:  p.meta.SessionID,
		TeamID:           p.meta.TeamID,
	}

	// Resolve display label
	if p.metaResolver != nil {
		if label := p.metaResolver.ResolveDisplayLabel(ctx, toolName); label != "" {
			a.Label = label
		}
	}

	p.activities[id] = a
	p.toolCalls[toolCallID] = a

	p.publishAndPersist(ctx, a, contract.EnvelopeTypeActivityStart)
}

// OnToolResult finalizes an action Activity when a tool result arrives.
func (p *ActivityProjector) OnToolResult(ctx context.Context, toolCallID, resultJSON, status, errorCode string, durationMs int64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	a, ok := p.toolCalls[toolCallID]
	if !ok {
		return
	}

	a.ToolResult = resultJSON
	a.ToolDurationMs = durationMs
	a.ToolErrorCode = errorCode

	switch status {
	case "success":
		a.Status = biz.ActivityStatusCompleted
	case "failed":
		a.Status = biz.ActivityStatusFailed
	default:
		a.Status = biz.ActivityStatusCompleted
	}

	now := time.Now().UTC()
	a.DurationMs = now.Sub(a.Timestamp).Milliseconds()
	a.Collapsed = true

	p.publishAndPersist(ctx, a, contract.EnvelopeTypeActivityDone)
	delete(p.toolCalls, toolCallID)
}

// OnDelegate creates a delegate + sub_task_board Activity for Spirit→Team delegation.
func (p *ActivityProjector) OnDelegate(ctx context.Context, teamID, spiritSessionID string, dagNodeID string, dependsOn []string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	id := uuid.NewString()
	now := time.Now().UTC()
	a := &biz.Activity{
		ID:               id,
		Kind:             biz.ActivityKindDelegate,
		Status:           biz.ActivityStatusRunning,
		SessionID:        p.meta.SessionID,
		TurnID:           p.meta.RequestID,
		ParentActivityID: p.rootActivityID,
		Timestamp:        now,
		TeamID:           teamID,
		SpiritSessionID:  spiritSessionID,
		DagNodeID:        dagNodeID,
		DependsOn:        dependsOn,
		AgentKey:         p.meta.AgentID,
		AgentName:        p.meta.AgentDisplayName,
	}

	p.activities[id] = a
	p.publishAndPersist(ctx, a, contract.EnvelopeTypeActivityStart)

	// Also create a sub_task_board child
	childID := uuid.NewString()
	child := &biz.Activity{
		ID:               childID,
		Kind:             biz.ActivityKindSubTaskBoard,
		Status:           biz.ActivityStatusRunning,
		SessionID:        p.meta.SessionID,
		TurnID:           p.meta.RequestID,
		ParentActivityID: id,
		Timestamp:        now,
		ChildBoardID:     childID,
		TeamID:           teamID,
		SpiritSessionID:  spiritSessionID,
		DagNodeID:        dagNodeID,
		DependsOn:        dependsOn,
	}
	a.ChildBoardID = childID

	p.activities[childID] = child
	p.publishAndPersist(ctx, child, contract.EnvelopeTypeActivityChildStart)
}

// ActivityUsage carries token consumption data for a completed turn.
type ActivityUsage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// OnError creates an error Activity with full error classification.
func (p *ActivityProjector) OnError(ctx context.Context, errMsg string, errType string, errCode string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	id := uuid.NewString()
	now := time.Now().UTC()
	a := &biz.Activity{
		ID:               id,
		Kind:             biz.ActivityKindError,
		Status:           biz.ActivityStatusFailed,
		SessionID:        p.meta.SessionID,
		TurnID:           p.meta.RequestID,
		ParentActivityID: p.rootActivityID,
		Timestamp:        now,
		Content:          errMsg,
		AgentKey:         p.meta.AgentID,
		AgentName:        p.meta.AgentDisplayName,
	}

	p.activities[id] = a

	env := p.buildActivityEnvelope(a, contract.EnvelopeTypeActivityStart)
	// Attach error classification so the frontend can drive messageStore
	// error handling (markStreamingMessagesFailed, onErrorNotify) without
	// needing the legacy error envelope.
	if errType != "" {
		env.Metadata["error_type"] = errType
	}
	if errCode != "" {
		env.Metadata["error_code"] = errCode
	}
	p.publishEnvelope(ctx, env, a)
}

// OnNotice creates a notice Activity for system notifications.
func (p *ActivityProjector) OnNotice(ctx context.Context, turnID, sessionID string, content string, noticeType string) (*biz.Activity, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	id := uuid.NewString()
	now := time.Now().UTC()
	activity := &biz.Activity{
		ID:               id,
		Kind:             biz.ActivityKindNotice,
		Status:           biz.ActivityStatusPending,
		SessionID:        sessionID,
		TurnID:           turnID,
		ParentActivityID: p.rootActivityID,
		Timestamp:        now,
		Content:          content,
		Meta:             map[string]any{"noticeType": noticeType},
	}
	p.activities[id] = activity

	// Notice is immediately completed (pending → completed in the same call).
	// The sequencer guarantees start → done ordering per-activity, so we can
	// safely use two publishAndPersist calls without a manual goroutine.
	p.publishAndPersist(ctx, activity, contract.EnvelopeTypeActivityStart)
	activity.Status = biz.ActivityStatusCompleted
	activity.DurationMs = time.Now().UTC().Sub(activity.Timestamp).Milliseconds()
	p.publishAndPersist(ctx, activity, contract.EnvelopeTypeActivityDone)

	return activity, nil
}

// EmitNotice implements biz.ActivityEmitter. It is a thin wrapper over OnNotice
// for use by plugins/hooks that access the projector via context.
// The turn/session IDs are derived from the projector's stored ProjectMeta.
func (p *ActivityProjector) EmitNotice(ctx context.Context, content, noticeType string) error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	turnID, sessionID := p.meta.RequestID, p.meta.SessionID
	p.mu.Unlock()
	_, err := p.OnNotice(ctx, turnID, sessionID, content, noticeType)
	return err
}

// ConfirmRequestParams holds the parameters for creating a confirm Activity.
type ConfirmRequestParams struct {
	ToolName      string
	ToolArguments string
	Content       string
}

// OnConfirmRequest creates a confirm Activity that blocks until user responds.
func (p *ActivityProjector) OnConfirmRequest(ctx context.Context, turnID, sessionID string, params ConfirmRequestParams) (*biz.Activity, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	id := uuid.NewString()
	now := time.Now().UTC()
	activity := &biz.Activity{
		ID:               id,
		Kind:             biz.ActivityKindConfirm,
		Status:           biz.ActivityStatusToolBlocked,
		SessionID:        sessionID,
		TurnID:           turnID,
		ParentActivityID: p.rootActivityID,
		Timestamp:        now,
		Content:          params.Content,
		Meta:             map[string]any{"toolName": params.ToolName, "toolArguments": params.ToolArguments},
	}
	p.activities[id] = activity
	p.publishAndPersist(ctx, activity, contract.EnvelopeTypeActivityStart)

	return activity, nil
}

// OnConfirmResult updates a confirm Activity with the user's response.
func (p *ActivityProjector) OnConfirmResult(ctx context.Context, activityID string, approved bool) (*biz.Activity, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	activity, ok := p.activities[activityID]
	if !ok {
		return nil, apierror.NotFound("activity", "activity not found: %s", activityID)
	}
	if activity.Kind != biz.ActivityKindConfirm {
		return nil, apierror.BadRequest("activity", "expected confirm kind, got %s", activity.Kind)
	}
	if approved {
		activity.Status = biz.ActivityStatusCompleted
	} else {
		activity.Status = biz.ActivityStatusCancelled
	}
	activity.DurationMs = time.Now().UTC().Sub(activity.Timestamp).Milliseconds()
	p.publishAndPersist(ctx, activity, contract.EnvelopeTypeActivityDone)

	return activity, nil
}

// EmitConfirmRequest implements biz.ActivityEmitter. It is a thin wrapper over
// OnConfirmRequest for use by plugins/hooks that access the projector via context.
// The turn/session IDs are derived from the projector's stored ProjectMeta.
func (p *ActivityProjector) EmitConfirmRequest(ctx context.Context, params biz.ActivityConfirmParams) (string, error) {
	if p == nil {
		return "", nil
	}
	p.mu.Lock()
	turnID, sessionID := p.meta.RequestID, p.meta.SessionID
	p.mu.Unlock()
	a, err := p.OnConfirmRequest(ctx, turnID, sessionID, ConfirmRequestParams{
		ToolName:      params.ToolName,
		ToolArguments: params.ToolArguments,
		Content:       params.Content,
	})
	if err != nil || a == nil {
		return "", err
	}
	return a.ID, nil
}

// EmitConfirmResult implements biz.ActivityEmitter. It is a thin wrapper over
// OnConfirmResult for use by plugins/hooks that access the projector via context.
func (p *ActivityProjector) EmitConfirmResult(ctx context.Context, activityID string, approved bool) error {
	if p == nil {
		return nil
	}
	_, err := p.OnConfirmResult(ctx, activityID, approved)
	return err
}

// compile-time interface check
var _ biz.ActivityEmitter = (*ActivityProjector)(nil)

// processGraphNodeStart handles graph.node.start events by lazily creating a plan
// Activity on first node arrival and registering a pending step.
// This is N-02 Plan Activity runtime integration: graph nodes → plan steps.
func (p *ActivityProjector) processGraphNodeStart(ctx context.Context, ev *trpcevent.Event) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Extract node metadata from StateDelta
	if ev.StateDelta == nil {
		return
	}
	nodeRaw, ok := ev.StateDelta[graphMetadataKeyNode]
	if !ok {
		return
	}
	var meta graphNodeMetadata
	if err := json.Unmarshal(nodeRaw, &meta); err != nil {
		return
	}
	if meta.NodeID == "" {
		return
	}

	// Lazily create plan Activity on first node arrival (inline to avoid deadlock)
	if p.planActivityID == "" {
		id := uuid.NewString()
		now := time.Now().UTC()
		planAct := &biz.Activity{
			ID:        id,
			Kind:      biz.ActivityKindPlan,
			Status:    biz.ActivityStatusRunning,
			SessionID: p.meta.SessionID,
			TurnID:    p.meta.RequestID,
			Timestamp: now,
			Content:   "执行计划",
			Meta:      map[string]any{"steps": []biz.ActivityPlanStep{}},
		}
		p.activities[id] = planAct
		p.planActivityID = id
		p.publishAndPersist(ctx, planAct, contract.EnvelopeTypeActivityStart)
	}

	// Append a new step to the plan
	planAct, ok := p.activities[p.planActivityID]
	if !ok {
		return
	}
	stepsRaw := planAct.Meta["steps"]
	steps, _ := stepsRaw.([]biz.ActivityPlanStep)
	if steps == nil {
		steps = []biz.ActivityPlanStep{}
	}
	p.planStepIndex++
	step := biz.ActivityPlanStep{
		ID:      meta.NodeID,
		Label:   planStepLabel(meta),
		Status:  biz.ActivityStatusRunning,
	}
	steps = append(steps, step)
	planAct.Meta["steps"] = steps
	p.publishAndPersist(ctx, planAct, contract.EnvelopeTypeActivityDelta)
}

// processGraphNodeComplete handles graph.node.complete and graph.node.error events
// by updating the corresponding plan step's status.
func (p *ActivityProjector) processGraphNodeComplete(ctx context.Context, ev *trpcevent.Event) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.planActivityID == "" || ev.StateDelta == nil {
		return
	}
	nodeRaw, ok := ev.StateDelta[graphMetadataKeyNode]
	if !ok {
		return
	}
	var meta graphNodeMetadata
	if err := json.Unmarshal(nodeRaw, &meta); err != nil {
		return
	}
	if meta.NodeID == "" {
		return
	}

	planAct, ok := p.activities[p.planActivityID]
	if !ok {
		return
	}
	stepsRaw := planAct.Meta["steps"]
	steps, _ := stepsRaw.([]biz.ActivityPlanStep)

	// Find and update the step matching this node ID
	newStatus := biz.ActivityStatusCompleted
	if ev.Response.Object == graphObjectTypeNodeError {
		newStatus = biz.ActivityStatusFailed
	}
	for i := range steps {
		if steps[i].ID == meta.NodeID {
			steps[i].Status = newStatus
			break
		}
	}
	planAct.Meta["steps"] = steps

	// Check if all steps are done
	allDone := true
	for _, s := range steps {
		if s.Status != biz.ActivityStatusCompleted && s.Status != biz.ActivityStatusFailed {
			allDone = false
			break
		}
	}
	if allDone {
		planAct.Status = biz.ActivityStatusCompleted
		p.publishAndPersist(ctx, planAct, contract.EnvelopeTypeActivityDone)
	} else {
		p.publishAndPersist(ctx, planAct, contract.EnvelopeTypeActivityDelta)
	}
}

func (p *ActivityProjector) OnPlanStart(ctx context.Context, turnID, sessionID string, title string, steps []biz.ActivityPlanStep) (*biz.Activity, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	id := uuid.NewString()
	now := time.Now().UTC()
	activity := &biz.Activity{
		ID:        id,
		Kind:      biz.ActivityKindPlan,
		Status:    biz.ActivityStatusPending,
		SessionID: sessionID,
		TurnID:    turnID,
		Timestamp: now,
		Content:   title,
		Meta:      map[string]any{"steps": steps},
	}
	p.activities[id] = activity
	p.publishAndPersist(ctx, activity, contract.EnvelopeTypeActivityStart)

	return activity, nil
}

// OnPlanStepUpdate updates a step's status within a plan Activity.
func (p *ActivityProjector) OnPlanStepUpdate(ctx context.Context, activityID string, stepID string, status biz.ActivityStatus) (*biz.Activity, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	activity, ok := p.activities[activityID]
	if !ok {
		return nil, apierror.NotFound("activity", "activity not found: %s", activityID)
	}
	if activity.Kind != biz.ActivityKindPlan {
		return nil, apierror.BadRequest("activity", "expected plan kind, got %s", activity.Kind)
	}
	stepsRaw, ok := activity.Meta["steps"]
	if !ok {
		return nil, apierror.BadRequest("activity", "plan activity has no steps metadata")
	}
	steps, ok := stepsRaw.([]biz.ActivityPlanStep)
	if !ok {
		return nil, apierror.BadRequest("activity", "plan activity steps has invalid type: %T", stepsRaw)
	}
	if steps == nil {
		return nil, apierror.BadRequest("activity", "plan activity has nil steps")
	}
	stepFound := false
	for i := range steps {
		if steps[i].ID == stepID {
			steps[i].Status = status
			stepFound = true
			break
		}
	}
	if !stepFound {
		return nil, apierror.NotFound("step", "step not found in plan: %s", stepID)
	}
	activity.Meta["steps"] = steps

	// Update plan status based on steps
	allCompleted := true
	anyFailed := false
	for _, s := range steps {
		if s.Status != biz.ActivityStatusCompleted && s.Status != biz.ActivityStatusFailed {
			allCompleted = false
		}
		if s.Status == biz.ActivityStatusFailed {
			anyFailed = true
		}
	}

	if allCompleted {
		if anyFailed {
			activity.Status = biz.ActivityStatusPartialFailure
		} else {
			activity.Status = biz.ActivityStatusCompleted
		}
		activity.DurationMs = time.Now().UTC().Sub(activity.Timestamp).Milliseconds()
		p.publishAndPersist(ctx, activity, contract.EnvelopeTypeActivityDone)
	} else {
		activity.Status = biz.ActivityStatusRunning
		env := p.buildActivityEnvelope(activity, contract.EnvelopeTypeActivityDelta)
		if env.Metadata == nil {
			env.Metadata = make(map[string]any)
		}
		env.Metadata["delta_field"] = "steps"
		env.Metadata["delta_chunk"] = ""
		p.publishEnvelope(ctx, env, activity)
	}

	return activity, nil
}

// OnStuckTools finalizes action Activities whose tool_result never arrived.
// Called from stream_consumer.finalize() when the turn ends with pending tool calls.
// This is the AF equivalent of PublishStuckToolResultEnvelopes — instead of
// publishing a legacy tool_result envelope (which AF mode doesn't process),
// it publishes activity_done(kind=action, status=failed) envelopes so the
// frontend can update the tool card from running → failed.
func (p *ActivityProjector) OnStuckTools(ctx context.Context, pending map[string]event.EnvelopeToolCall) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for toolCallID := range pending {
		a, ok := p.toolCalls[toolCallID]
		if !ok {
			continue
		}

		// Mark as failed with timeout error
		errPayload, _ := json.Marshal(map[string]string{
			"error":    stuckToolResultFallback,
			"i18n_key": stuckToolResultI18nKey,
		})
		a.ToolResult = string(errPayload)
		a.ToolErrorCode = contract.ErrorCodeToolTimeout
		a.Status = biz.ActivityStatusFailed
		now := time.Now().UTC()
		a.DurationMs = now.Sub(a.Timestamp).Milliseconds()
		a.Collapsed = true

		p.publishAndPersist(ctx, a, contract.EnvelopeTypeActivityDone)
		delete(p.toolCalls, toolCallID)
	}
}

// OnTurnEnd finalizes the root task Activity with optional token usage.
func (p *ActivityProjector) OnTurnEnd(ctx context.Context, usage *ActivityUsage) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.rootActivityID == "" {
		return
	}
	a, ok := p.activities[p.rootActivityID]
	if !ok {
		return
	}

	a.Status = biz.ActivityStatusCompleted
	now := time.Now().UTC()
	a.DurationMs = now.Sub(a.Timestamp).Milliseconds()

	// Store token usage in the root task Activity record.
	// This enables future migration away from the merged assistant ChatMessage,
	// since token stats will be available directly from Activity data.
	if usage != nil {
		a.PromptTokens = int64(usage.PromptTokens)
		a.CompletionTokens = int64(usage.CompletionTokens)
	}

	env := p.buildActivityEnvelope(a, contract.EnvelopeTypeActivityDone)
	// Attach usage so the frontend can update session context metrics
	// without needing the legacy runner_completion envelope.
	if usage != nil {
		env.Metadata["usage"] = map[string]any{
			"prompt_tokens":     usage.PromptTokens,
			"completion_tokens": usage.CompletionTokens,
			"total_tokens":      usage.TotalTokens,
		}
	}
	p.publishEnvelope(ctx, env, a)
}

// publishAndPersist publishes an Activity envelope and persists to DB.
// Called with p.mu held — copies activity data before enqueuing to sequencer.
// The sequencer's per-activity channel guarantees FIFO ordering (start → delta
// → done) while performing I/O outside the mutex.
func (p *ActivityProjector) publishAndPersist(ctx context.Context, a *biz.Activity, envType contract.EnvelopeType) {
	env := p.buildActivityEnvelope(a, envType)
	p.publishEnvelope(ctx, env, a)
}

// publishEnvelope publishes a pre-built envelope and persists the activity to DB.
// Called with p.mu held — copies activity data before enqueuing to sequencer.
// The sequencer consumer goroutine performs the actual I/O (publish + persist)
// outside the caller's critical section.
func (p *ActivityProjector) publishEnvelope(ctx context.Context, env contract.Envelope, a *biz.Activity) {
	if p.sequencer == nil {
		return
	}
	activityCopy := *a
	if err := p.sequencer.publish(ctx, a.ID, publishTask{
		env:      env,
		persist:  true,
		activity: activityCopy,
	}); err != nil {
		p.lg.Warn("activity publish failed",
			loggateway.StepID("agent.activity_projector.publish"),
			loggateway.Str("activity_id", a.ID),
			loggateway.Str("kind", string(a.Kind)),
			loggateway.Err(err))
	}
}

// publishActivityDelta publishes an activity_delta envelope with a content patch.
// Called with p.mu held — enqueues to sequencer for ordered async publishing.
// Unlike the previous sync implementation, this no longer blocks the event loop
// on bus.Publish; backpressure is applied via the sequencer's channel buffer.
func (p *ActivityProjector) publishActivityDelta(ctx context.Context, a *biz.Activity, field, chunk string) {
	if p.sequencer == nil {
		return
	}
	env := p.buildActivityEnvelope(a, contract.EnvelopeTypeActivityDelta)
	if env.Metadata == nil {
		env.Metadata = make(map[string]any)
	}
	env.Metadata["delta_field"] = field
	env.Metadata["delta_chunk"] = chunk
	if err := p.sequencer.publish(ctx, a.ID, publishTask{
		env:     env,
		persist: false,
	}); err != nil {
		p.lg.Warn("activity delta publish failed",
			loggateway.StepID("agent.activity_projector.delta"),
			loggateway.Str("activity_id", a.ID),
			loggateway.Err(err))
	}
}

// buildActivityEnvelope creates an Envelope for an Activity event.
func (p *ActivityProjector) buildActivityEnvelope(a *biz.Activity, envType contract.EnvelopeType) contract.Envelope {
	env := contract.NewEnvelope(envType, a.AgentKey, a.SessionID)
	env.TurnID = a.TurnID
	env.TeamID = a.TeamID
	env.Metadata = map[string]any{
		"activity_id":        a.ID,
		"kind":               string(a.Kind),
		"status":             string(a.Status),
		"parent_activity_id": a.ParentActivityID,
		"timestamp":          a.Timestamp.UTC().Format(time.RFC3339Nano),
		"duration_ms":        a.DurationMs,
		"collapsed":          a.Collapsed,
		// AF-correlation: 前端 useConversationTimeline 通过 turn_id 将 Activity 记录关联到
		// UserTurn；handleActivityStart 从 metadata 读取 turn_id/session_id。缺失会导致
		// Activity 记录被 if (!tid) continue 跳过，思考和回复 UI 不显示。
		"turn_id":    a.TurnID,
		"session_id": a.SessionID,
	}

	// Content fields
	if a.Content != "" {
		env.Metadata["content"] = a.Content
	}
	if a.Reasoning != "" {
		env.Metadata["reasoning"] = a.Reasoning
	}

	// Tool fields — include redacted arguments/result so the frontend message
	// list can render tool call messages without a separate API round-trip.
	// The redaction limit (512 bytes) matches biz.redactActivityJSON and the
	// frontend ACTIVITY_JSON_PREVIEW_LIMIT, ensuring consistency.
	if a.ToolName != "" {
		env.Metadata["tool_name"] = a.ToolName
	}
	if a.ToolCallID != "" {
		env.Metadata["tool_call_id"] = a.ToolCallID
	}
	if a.ToolArguments != "" {
		env.Metadata["tool_arguments"] = biz.RedactActivityJSON(a.ToolArguments)
	}
	if a.ToolResult != "" {
		env.Metadata["tool_result"] = biz.RedactActivityJSON(a.ToolResult)
	}
	if a.ToolDurationMs > 0 {
		env.Metadata["tool_duration_ms"] = a.ToolDurationMs
	}
	if a.ToolErrorCode != "" {
		env.Metadata["tool_error_code"] = a.ToolErrorCode
	}

	// Sub-task board
	if a.ChildBoardID != "" {
		env.Metadata["child_board_id"] = a.ChildBoardID
	}

	// Spirit extension
	if a.SpiritSessionID != "" {
		env.Metadata["spirit_session_id"] = a.SpiritSessionID
	}
	if a.DagNodeID != "" {
		env.Metadata["dag_node_id"] = a.DagNodeID
	}
	if len(a.DependsOn) > 0 {
		env.Metadata["depends_on"] = a.DependsOn
	}

	// Agent info
	if a.AgentKey != "" {
		env.Metadata["agent_key"] = a.AgentKey
	}
	if a.AgentName != "" {
		env.Metadata["agent_name"] = a.AgentName
	}
	if a.Label != "" {
		env.Metadata["label"] = a.Label
	}

	// Meta fields (for notice/confirm/plan kinds)
	if a.Meta != nil {
		env.Metadata["meta"] = a.Meta
	}

	return env
}

// findActivityByKindAuthor finds an activity by kind and agent key (O(1) via kindAuthorMap).
func (p *ActivityProjector) findActivityByKindAuthor(kind biz.ActivityKind, author string) string {
	return p.kindAuthorMap[kindKey(kind, author)]
}

// kindKey builds the composite key for kindAuthorMap.
func kindKey(kind biz.ActivityKind, author string) string {
	return string(kind) + ":" + author
}

// resolveAgentName resolves the display name for an agent key.
func (p *ActivityProjector) resolveAgentName(ctx context.Context, agentKey string) string {
	if p.metaResolver != nil {
		if name := p.metaResolver.ResolveAgentDisplayName(ctx, agentKey); name != "" {
			return name
		}
	}
	return agentKey
}
