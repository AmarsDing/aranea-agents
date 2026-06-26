package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"aranea-agents/internal/biz"
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
	graphObjectTypeNodeStart    = "graph.node.start"
	graphObjectTypeNodeComplete = "graph.node.complete"
	graphObjectTypeNodeError    = "graph.node.error"
	graphMetadataKeyNode        = "_node_metadata"
)

// Stuck tool result i18n/fallback constants used by OnStuckTools to mark
// in-flight tool cards as failed when the turn ends without tool_result.
const (
	stuckToolResultI18nKey  = "chat.tool.stuckTimeout"
	stuckToolResultFallback = "工具执行未返回结果，已自动标记为失败。如需重试请重新发送指令"
)

// graphNodeMetadata mirrors trpc-agent-go/graph.NodeExecutionMetadata
// (only the fields we need for plan step mapping).
type graphNodeMetadata struct {
	NodeID     string `json:"nodeId"`
	NodeType   string `json:"nodeType,omitempty"`
	StepNumber int    `json:"stepNumber,omitempty"`
	ModelName  string `json:"modelName,omitempty"`
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
type ActivityProjector struct {
	mu           sync.Mutex
	eventBus     biz.ActivityEventBus
	activityRepo biz.ActivityWriter
	metaResolver ActivityMetaResolver
	lg           loggateway.Logger

	// sequencer guarantees per-activity FIFO event ordering.
	// Each activity gets its own channel and consumer goroutine.
	// This fixes B-01 (start/delta ordering), B-04 (delta holds global lock),
	// and B-05 (async start races with sync delta).
	sequencer *activityEventSequencer

	// seq is a monotonically increasing counter assigned to every emitted
	// event. It lets the frontend reconstruct the exact backend emission
	// order even when the per-activity sequencer publishes events for different
	// activities concurrently (e.g. thinking done vs reply start).
	seq int64

	// Turn-scoped state (reset per turn)
	rootActivityID string
	activities     map[string]*biz.Activity // id -> activity
	toolCalls      map[string]*biz.Activity // tool_call_id -> action activity
	kindAuthorMap  map[string]string        // "kind:author" -> activity ID (O(1) lookup)
	reasoningBuf   map[string]*strings.Builder
	meta           ProjectMeta
	planActivityID string // current turn's plan activity ID (graph node events)
	planStepIndex  int    // monotonic counter for plan steps within this turn

	// memberToolCalls tracks per-member tool call counts for team run step
	// persistence. Populated directly from OnToolCall so stream_consumer no
	// longer needs EventProjector output to compute MemberToolCalls.
	memberToolCalls map[string]int

	// toolCategorizer classifies tool names into functional categories for
	// frontend rendering. Defaults to noop (returns ToolCategoryOther) when
	// not configured; set via SetToolCategorizer.
	toolCategorizer ToolCategorizer

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
func NewActivityProjector(eventBus biz.ActivityEventBus, activityRepo biz.ActivityWriter, lg loggateway.Logger) *ActivityProjector {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	p := &ActivityProjector{
		eventBus:        eventBus,
		activityRepo:    activityRepo,
		lg:              lg,
		toolCategorizer: NewNoopToolCategorizer(),
	}
	p.sequencer = newActivityEventSequencer(eventBus, lg)
	p.sequencer.SetActivityRepo(activityRepo)
	return p
}

// SetToolCategorizer configures the tool categorizer for action Activities.
// When set, OnToolCall populates Activity.ToolCategory for frontend rendering.
func (p *ActivityProjector) SetToolCategorizer(c ToolCategorizer) {
	if c == nil {
		c = NewNoopToolCategorizer()
	}
	p.mu.Lock()
	p.toolCategorizer = c
	p.mu.Unlock()
}

// activitySeq returns the pre-allocated Seq for an Activity.
// All Activity creation paths in On* methods MUST allocate Seq at the entry
// point (under p.mu). This function asserts the invariant and returns the
// pre-allocated value. Lazy allocation is no longer supported — see the
// architecture decision in docs/superpowers/specs/2026-06-27-chat-ui-streaming-fix-design.md.
func (p *ActivityProjector) activitySeq(a *biz.Activity) int64 {
	if a.Seq == 0 {
		panic(fmt.Sprintf("activity %s (%s) has Seq=0 — seq must be pre-allocated in On* entry", a.ID, a.Kind))
	}
	return a.Seq
}

// transitionActivityStatus validates and applies a state transition on the
// activity using the AS-FSM-01 state machine.
//
// On illegal transitions, the target status is still applied (to preserve the
// existing behavior of recording the intended terminal state for debugging),
// but a warning is logged so operators can detect state-machine violations.
// This avoids breaking the streaming flow when a transition rule is missing
// from the table, while still surfacing the violation.
//
// Caller must hold p.mu.
func (p *ActivityProjector) transitionActivityStatus(a *biz.Activity, target biz.ActivityStatus) {
	from := a.Status
	if from == target {
		return
	}
	if !biz.CanTransitionActivityStatus(from, target) {
		p.lg.Warn("ActivityProjector: illegal activity state transition",
			loggateway.StepID("agent.activity_projector.transition"),
			loggateway.Str("activity_id", a.ID),
			loggateway.Str("kind", string(a.Kind)),
			loggateway.Str("from", string(from)),
			loggateway.Str("to", string(target)),
		)
	}
	a.Status = target
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

// ListDeadLetterActivities returns activities whose persist failed after all
// retries, filtered by sessionID. The WS reconnect replay path should merge
// these with ListActivities RPC results to avoid showing users a gap for
// events that were live-delivered via WS but could not be persisted.
//
// Returns nil if no dead-letter entries exist for the session.
func (p *ActivityProjector) ListDeadLetterActivities(sessionID string) []biz.Activity {
	if p.sequencer == nil {
		return nil
	}
	return p.sequencer.ListDeadLetterActivities(sessionID)
}

// Configure sets the ProjectMeta for the current turn.
func (p *ActivityProjector) Configure(meta ProjectMeta, resolver ActivityMetaResolver) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.meta = meta
	p.metaResolver = resolver
}

// resolveSpiritSessionID returns the spirit session ID for the current turn.
// When ProjectMeta.SpiritSessionID is set (sub-session scenario), it is used;
// otherwise falls back to SessionID (spirit root or standalone session).
// Caller must hold p.mu.
func (p *ActivityProjector) resolveSpiritSessionID() string {
	if p.meta.SpiritSessionID != "" {
		return p.meta.SpiritSessionID
	}
	return p.meta.SessionID
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
	p.memberToolCalls = make(map[string]int)
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
		// Reasoning is emitted before text so that the timeline always presents
		// thinking ahead of the reply it supports, even when a single chunk
		// contains both content types.
		if ev.Response.IsPartial {
			if reasoning != "" {
				p.OnReasoningDelta(ctx, author, reasoning, true)
			}
			if text != "" {
				if isMember {
					p.OnMemberMessageDelta(ctx, author, text)
				} else {
					p.OnTextDelta(ctx, author, text)
				}
			}
		} else {
			// Final chunks must finalize the corresponding activity even when
			// the payload is empty, otherwise streaming activities stay in the
			// running state and accumulated content is lost.
			p.OnReasoningDone(ctx, author, reasoning, false)
			if isMember {
				p.OnMemberMessageDone(ctx, author, text)
			} else {
				p.OnTextDone(ctx, author, text)
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
		// Reasoning is emitted before text so that the timeline presents
		// thinking ahead of the final reply.
		if reasoning != "" {
			p.OnReasoningDone(ctx, author, reasoning, false)
		}
		if text != "" {
			if isMember {
				p.OnMemberMessageDone(ctx, author, text)
			} else {
				p.OnTextDone(ctx, author, text)
			}
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

	// Avoid double-encoding string content: tool runtimes return a JSON string
	// as msg.Content, and json.Marshal would wrap it in another pair of quotes.
	resultJSON := msg.Content
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

	// Resolve spirit session ID: use explicit SpiritSessionID when set (sub-session),
	// otherwise fall back to SessionID (spirit root or standalone session).
	spiritSessionID := meta.SpiritSessionID
	if spiritSessionID == "" {
		spiritSessionID = meta.SessionID
	}

	id := uuid.NewString()
	now := time.Now().UTC()
	a := &biz.Activity{
		ID:              id,
		Kind:            biz.ActivityKindTask,
		Status:          biz.ActivityStatusRunning,
		SessionID:       meta.SessionID,
		TurnID:          meta.RequestID,
		Timestamp:       now,
		Seq:             atomic.AddInt64(&p.seq, 1),
		AgentKey:        meta.AgentID,
		AgentName:       meta.AgentDisplayName,
		SpiritSessionID: spiritSessionID,
		TeamID:          meta.TeamID,
		Content:         meta.TaskContent,
	}
	p.rootActivityID = id
	p.activities[id] = a

	p.publishAndPersist(ctx, a, biz.ActivityEventCreated)
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
			Seq:              atomic.AddInt64(&p.seq, 1),
			AgentKey:         author,
			AgentName:        p.resolveAgentName(ctx, author),
			SpiritSessionID:  p.resolveSpiritSessionID(),
			TeamID:           p.meta.TeamID,
		}
		p.activities[id] = a
		p.kindAuthorMap[kindKey(biz.ActivityKindThinking, author)] = id
		p.publishAndPersist(ctx, a, biz.ActivityEventCreated)
		activityID = id
	}

	// Emit delta
	a := p.activities[activityID]
	a.Reasoning += chunk
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
		if fullReasoning != "" {
			a.Content = fullReasoning
			a.Reasoning = ""
		} else {
			a.Content = a.Reasoning
			a.Reasoning = ""
		}
	} else {
		if fullReasoning != "" {
			a.Reasoning = fullReasoning
		}
		// Remove completed thinking from lookup so the next ReAct round
		// creates a new thinking Activity instead of appending to this one.
		// This mirrors OnToolResult's delete(p.toolCalls, toolCallID) pattern.
		delete(p.kindAuthorMap, kindKey(biz.ActivityKindThinking, author))
	}

	p.transitionActivityStatus(a, biz.ActivityStatusCompleted)
	now := time.Now().UTC()
	a.DurationMs = now.Sub(a.Timestamp).Milliseconds()
	a.Collapsed = true

	p.publishAndPersist(ctx, a, biz.ActivityEventCompleted)
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
			Seq:              atomic.AddInt64(&p.seq, 1),
			AgentKey:         author,
			AgentName:        p.resolveAgentName(ctx, author),
			SpiritSessionID:  p.resolveSpiritSessionID(),
			TeamID:           p.meta.TeamID,
		}
		p.activities[id] = a
		p.kindAuthorMap[kindKey(biz.ActivityKindReply, author)] = id
		p.publishAndPersist(ctx, a, biz.ActivityEventCreated)
		activityID = id
	}

	a := p.activities[activityID]
	a.Content += chunk
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
			Seq:              atomic.AddInt64(&p.seq, 1),
			AgentKey:         author,
			AgentName:        p.resolveAgentName(ctx, author),
			SpiritSessionID:  p.resolveSpiritSessionID(),
			TeamID:           p.meta.TeamID,
			Meta:             map[string]any{"member_id": author},
		}
		p.activities[id] = a
		p.kindAuthorMap[kindKey(biz.ActivityKindReply, author)] = id
		p.publishAndPersist(ctx, a, biz.ActivityEventCreated)
		activityID = id
	}

	a := p.activities[activityID]
	a.Content += chunk
	p.publishActivityDelta(ctx, a, "content", chunk)
}

// OnMemberMessageDone finalizes a team member's reply activity.
// If no prior delta created the reply (e.g. the model only emits text in the
// final chunk), a new activity is created from the final text so the UI still
// has a reply to render.
func (p *ActivityProjector) OnMemberMessageDone(ctx context.Context, author string, fullText string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	activityID := p.findActivityByKindAuthor(biz.ActivityKindReply, author)
	var a *biz.Activity
	if activityID == "" {
		if fullText == "" {
			return
		}
		id := uuid.NewString()
		now := time.Now().UTC()
		a = &biz.Activity{
			ID:               id,
			Kind:             biz.ActivityKindReply,
			Status:           biz.ActivityStatusRunning,
			SessionID:        p.meta.SessionID,
			TurnID:           p.meta.RequestID,
			ParentActivityID: p.rootActivityID,
			Timestamp:        now,
			Seq:              atomic.AddInt64(&p.seq, 1),
			Content:          fullText,
			AgentKey:         author,
			AgentName:        p.resolveAgentName(ctx, author),
			SpiritSessionID:  p.resolveSpiritSessionID(),
			TeamID:           p.meta.TeamID,
			Meta:             map[string]any{"member_id": author},
		}
		p.activities[id] = a
		p.kindAuthorMap[kindKey(biz.ActivityKindReply, author)] = id
		p.publishAndPersist(ctx, a, biz.ActivityEventCreated)
	} else {
		a = p.activities[activityID]
		if fullText != "" {
			a.Content = fullText
		}
	}
	p.transitionActivityStatus(a, biz.ActivityStatusCompleted)
	now := time.Now().UTC()
	a.DurationMs = now.Sub(a.Timestamp).Milliseconds()
	a.Collapsed = false

	// Remove completed reply from lookup so the next member message
	// creates a new reply Activity.
	delete(p.kindAuthorMap, kindKey(biz.ActivityKindReply, author))

	p.publishAndPersist(ctx, a, biz.ActivityEventCompleted)
}

// OnTextDone finalizes a reply activity.
// If no prior delta created the reply (e.g. the model only emits text in the
// final chunk after reasoning/tool calls), a new activity is created from the
// final text so the reply UI is not left empty.
func (p *ActivityProjector) OnTextDone(ctx context.Context, author string, fullText string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	activityID := p.findActivityByKindAuthor(biz.ActivityKindReply, author)
	var a *biz.Activity
	if activityID == "" {
		if fullText == "" {
			return
		}
		id := uuid.NewString()
		now := time.Now().UTC()
		a = &biz.Activity{
			ID:               id,
			Kind:             biz.ActivityKindReply,
			Status:           biz.ActivityStatusRunning,
			SessionID:        p.meta.SessionID,
			TurnID:           p.meta.RequestID,
			ParentActivityID: p.rootActivityID,
			Timestamp:        now,
			Seq:              atomic.AddInt64(&p.seq, 1),
			Content:          fullText,
			AgentKey:         author,
			AgentName:        p.resolveAgentName(ctx, author),
			SpiritSessionID:  p.resolveSpiritSessionID(),
			TeamID:           p.meta.TeamID,
		}
		p.activities[id] = a
		p.kindAuthorMap[kindKey(biz.ActivityKindReply, author)] = id
		p.publishAndPersist(ctx, a, biz.ActivityEventCreated)
	} else {
		a = p.activities[activityID]
		if fullText != "" {
			a.Content = fullText
		}
	}
	p.transitionActivityStatus(a, biz.ActivityStatusCompleted)
	now := time.Now().UTC()
	a.DurationMs = now.Sub(a.Timestamp).Milliseconds()
	a.Collapsed = false

	// Remove completed reply from lookup so the next ReAct round
	// creates a new reply Activity instead of appending to this one.
	delete(p.kindAuthorMap, kindKey(biz.ActivityKindReply, author))

	p.publishAndPersist(ctx, a, biz.ActivityEventCompleted)
}

// OnToolCall creates or updates an action Activity for a tool call.
// Streaming tool calls may arrive as multiple deltas for the same tool_call_id;
// those deltas are merged into a single Activity.
func (p *ActivityProjector) OnToolCall(ctx context.Context, toolCallID, toolName, argsJSON, author string, startedAt time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Merge streaming deltas for the same tool call.
	if existing, ok := p.toolCalls[toolCallID]; ok {
		if toolName != "" {
			existing.ToolName = toolName
		}
		if argsJSON != "" {
			existing.ToolArguments += argsJSON
		}
		// Persist streaming tool call deltas so the accumulated arguments are
		// available for history and the UI does not show stale empty args.
		p.publishActivityDeltaWithPersist(ctx, existing, "tool_arguments", argsJSON, true)
		return
	}

	id := uuid.NewString()
	a := &biz.Activity{
		ID:               id,
		Kind:             biz.ActivityKindAction,
		Status:           biz.ActivityStatusToolRunning,
		SessionID:        p.meta.SessionID,
		TurnID:           p.meta.RequestID,
		ParentActivityID: p.rootActivityID,
		Timestamp:        startedAt,
		Seq:              atomic.AddInt64(&p.seq, 1),
		ToolName:         toolName,
		ToolCallID:       toolCallID,
		ToolArguments:    argsJSON,
		AgentKey:         author,
		AgentName:        p.resolveAgentName(ctx, author),
		SpiritSessionID:  p.resolveSpiritSessionID(),
		TeamID:           p.meta.TeamID,
	}
	// Classify tool category for frontend rendering (shell/browser/file/...).
	if p.toolCategorizer != nil {
		a.ToolCategory = p.toolCategorizer.Categorize(toolName)
	}

	// Resolve display label
	if p.metaResolver != nil {
		if label := p.metaResolver.ResolveDisplayLabel(ctx, toolName); label != "" {
			a.Label = label
		}
	}

	p.activities[id] = a
	p.toolCalls[toolCallID] = a

	// Track per-member tool call counts for team run step persistence.
	// stream_consumer reads this in AF mode instead of deriving it from
	// EventProjector envelopes.
	if isTeamMemberAuthor(author, p.meta) {
		if p.memberToolCalls == nil {
			p.memberToolCalls = make(map[string]int)
		}
		p.memberToolCalls[author]++
	}

	p.publishAndPersist(ctx, a, biz.ActivityEventCreated)
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
		p.transitionActivityStatus(a, biz.ActivityStatusCompleted)
	case "failed":
		p.transitionActivityStatus(a, biz.ActivityStatusFailed)
	default:
		p.transitionActivityStatus(a, biz.ActivityStatusCompleted)
	}

	now := time.Now().UTC()
	a.DurationMs = now.Sub(a.Timestamp).Milliseconds()
	a.Collapsed = true

	p.publishAndPersist(ctx, a, biz.ActivityEventCompleted)
	delete(p.toolCalls, toolCallID)
}

// ActivityUsage carries token consumption data for a completed turn.
type ActivityUsage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// OnError marks the root task Activity as failed and attaches error info.
//
// Phase 3 cleanup: errors are no longer expressed as a separate error-kind
// Activity. Instead, the root task Activity transitions to status=failed
// and the error message + classification are stored in Meta. This gives
// the frontend a single source of truth for turn failure (task.failed)
// without needing a parallel error kind.
//
// If no root task Activity exists (e.g. error before OnTurnStart), a
// minimal failed task Activity is created so the error is still visible.
func (p *ActivityProjector) OnError(ctx context.Context, errMsg string, errType string, errCode string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now().UTC()

	// Attach error classification to Meta.
	meta := map[string]any{
		"error_message": errMsg,
	}
	if errType != "" {
		meta["error_type"] = errType
	}
	if errCode != "" {
		meta["error_code"] = errCode
	}

	// Case 1: root task Activity exists — transition it to failed.
	if p.rootActivityID != "" {
		a, ok := p.activities[p.rootActivityID]
		if ok {
			p.transitionActivityStatus(a, biz.ActivityStatusFailed)
			a.DurationMs = now.Sub(a.Timestamp).Milliseconds()
			if a.Meta == nil {
				a.Meta = make(map[string]any)
			}
			for k, v := range meta {
				a.Meta[k] = v
			}
			if a.Content == "" {
				a.Content = errMsg
			}
			p.publishAndPersist(ctx, a, biz.ActivityEventCompleted)
			return
		}
	}

	// Case 2: no root task Activity — create a minimal failed task Activity
	// so the error is still surfaced to the frontend.
	id := uuid.NewString()
	a := &biz.Activity{
		ID:              id,
		Kind:            biz.ActivityKindTask,
		Status:          biz.ActivityStatusFailed,
		SessionID:       p.meta.SessionID,
		TurnID:          p.meta.RequestID,
		Timestamp:       now,
		Seq:             atomic.AddInt64(&p.seq, 1),
		DurationMs:      0,
		Content:         errMsg,
		AgentKey:        p.meta.AgentID,
		AgentName:       p.meta.AgentDisplayName,
		SpiritSessionID: p.meta.SpiritSessionID,
		TeamID:          p.meta.TeamID,
		Meta:            meta,
	}
	p.rootActivityID = id
	p.activities[id] = a
	p.publishAndPersist(ctx, a, biz.ActivityEventCreated)
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
		Seq:              atomic.AddInt64(&p.seq, 1),
		Content:          content,
		Meta:             map[string]any{"noticeType": noticeType},
	}
	p.activities[id] = activity

	// Notice is immediately completed (pending → completed in the same call).
	// The sequencer guarantees start → done ordering per-activity, so we can
	// safely use two publishAndPersist calls without a manual goroutine.
	p.publishAndPersist(ctx, activity, biz.ActivityEventCreated)
	activity.Status = biz.ActivityStatusCompleted
	activity.DurationMs = time.Now().UTC().Sub(activity.Timestamp).Milliseconds()
	p.publishAndPersist(ctx, activity, biz.ActivityEventCompleted)

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
		Seq:              atomic.AddInt64(&p.seq, 1),
		Content:          params.Content,
		Meta:             map[string]any{"toolName": params.ToolName, "toolArguments": params.ToolArguments},
	}
	p.activities[id] = activity
	p.publishAndPersist(ctx, activity, biz.ActivityEventCreated)

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
	p.publishAndPersist(ctx, activity, biz.ActivityEventCompleted)

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
			Seq:       atomic.AddInt64(&p.seq, 1),
			// B-07: Set ParentActivityID to the current turn's root task
			// Activity so the plan nests under the task in the Activity tree
			// (frontend ActivityStream recursive rendering). Without this,
			// the plan becomes a sibling root of the task, breaking the
			// parent-child visual hierarchy.
			ParentActivityID: p.rootActivityID,
			Content:          "执行计划",
			Meta:             map[string]any{"steps": []biz.ActivityPlanStep{}},
		}
		p.activities[id] = planAct
		p.planActivityID = id
		p.publishAndPersist(ctx, planAct, biz.ActivityEventCreated)
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
		ID:     meta.NodeID,
		Label:  planStepLabel(meta),
		Status: biz.ActivityStatusRunning,
	}
	steps = append(steps, step)
	planAct.Meta["steps"] = steps
	p.publishAndPersist(ctx, planAct, biz.ActivityEventStreaming)
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
		p.publishAndPersist(ctx, planAct, biz.ActivityEventCompleted)
	} else {
		p.publishAndPersist(ctx, planAct, biz.ActivityEventStreaming)
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
		Seq:       atomic.AddInt64(&p.seq, 1),
		Content:   title,
		Meta:      map[string]any{"steps": steps},
	}
	p.activities[id] = activity
	p.publishAndPersist(ctx, activity, biz.ActivityEventCreated)

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
		p.publishAndPersist(ctx, activity, biz.ActivityEventCompleted)
	} else {
		activity.Status = biz.ActivityStatusRunning
		ev := p.buildActivityEvent(activity, biz.ActivityEventStreaming)
		ev.DeltaField = "steps"
		ev.DeltaChunk = ""
		p.publishEvent(ctx, ev, activity)
	}

	return activity, nil
}

// MemberToolCalls returns a copy of the per-member tool call counts observed
// during the current turn. Used by stream_consumer in AF mode to populate
// EventStreamResult.MemberToolCalls for team run step persistence.
func (p *ActivityProjector) MemberToolCalls() map[string]int {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.memberToolCalls) == 0 {
		return nil
	}
	out := make(map[string]int, len(p.memberToolCalls))
	for k, v := range p.memberToolCalls {
		out[k] = v
	}
	return out
}

// OnStuckTools finalizes action Activities whose tool_result never arrived.
// Called from stream_consumer.finalize() when the turn ends with pending tool calls.
// It publishes activity_done(kind=action, status=failed) envelopes so the
// frontend can update the tool card from running → failed.
func (p *ActivityProjector) OnStuckTools(ctx context.Context) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for toolCallID, a := range p.toolCalls {
		// Skip activities that already received a terminal status; only
		// tool_running activities are stuck.
		if a.Status != biz.ActivityStatusToolRunning {
			continue
		}

		p.lg.Warn("stuck tool detected at turn finalization",
			loggateway.StepID("agent.activity_projector.stuck_tool"),
			loggateway.Str("tool_call_id", toolCallID),
			loggateway.Str("tool_name", a.ToolName),
			loggateway.Str("status", string(a.Status)),
			loggateway.Str("session_id", p.meta.SessionID),
		)

		// Mark as failed with timeout error
		errPayload, _ := json.Marshal(map[string]string{
			"error":    stuckToolResultFallback,
			"i18n_key": stuckToolResultI18nKey,
		})
		a.ToolResult = string(errPayload)
		a.ToolErrorCode = contract.ErrorCodeToolTimeout
		p.transitionActivityStatus(a, biz.ActivityStatusFailed)
		now := time.Now().UTC()
		a.DurationMs = now.Sub(a.Timestamp).Milliseconds()
		a.Collapsed = true

		p.publishAndPersist(ctx, a, biz.ActivityEventCompleted)
		delete(p.toolCalls, toolCallID)
	}
}

// OnTurnEnd finalizes the root task Activity with optional token usage.
func (p *ActivityProjector) OnTurnEnd(ctx context.Context, usage *ActivityUsage) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Finalize any still-running child activities so the UI does not leave
	// streaming indicators (reply/thinking/action) dangling when the turn ends.
	now := time.Now().UTC()
	for _, a := range p.activities {
		switch a.Kind {
		case biz.ActivityKindReply, biz.ActivityKindThinking:
			if a.Status == biz.ActivityStatusRunning {
				p.transitionActivityStatus(a, biz.ActivityStatusCompleted)
				a.DurationMs = now.Sub(a.Timestamp).Milliseconds()
				a.Collapsed = a.Kind == biz.ActivityKindThinking
				p.publishAndPersist(ctx, a, biz.ActivityEventCompleted)
			}
		case biz.ActivityKindAction:
			if a.Status == biz.ActivityStatusToolRunning {
				p.transitionActivityStatus(a, biz.ActivityStatusCompleted)
				a.DurationMs = now.Sub(a.Timestamp).Milliseconds()
				a.Collapsed = true
				p.publishAndPersist(ctx, a, biz.ActivityEventCompleted)
			}
		}
	}

	if p.rootActivityID == "" {
		return
	}
	a, ok := p.activities[p.rootActivityID]
	if !ok {
		return
	}

	// Respect terminal states set by OnError (Failed) or upstream cancel.
	// Overwriting a Failed root with Completed would hide the error from
	// the frontend and the activity record.
	if biz.IsActivityTerminal(a.Status) {
		// Still attach token usage if provided, but do not change status.
		if usage != nil {
			a.PromptTokens = int64(usage.PromptTokens)
			a.CompletionTokens = int64(usage.CompletionTokens)
			if a.Meta == nil {
				a.Meta = make(map[string]any)
			}
			a.Meta["usage"] = map[string]any{
				"prompt_tokens":     usage.PromptTokens,
				"completion_tokens": usage.CompletionTokens,
				"total_tokens":      usage.TotalTokens,
			}
			p.publishAndPersist(ctx, a, biz.ActivityEventUpdated)
		}
		return
	}

	p.transitionActivityStatus(a, biz.ActivityStatusCompleted)
	a.DurationMs = now.Sub(a.Timestamp).Milliseconds()

	// Store token usage in the root task Activity record.
	// This enables future migration away from the merged assistant ChatMessage,
	// since token stats will be available directly from Activity data.
	if usage != nil {
		a.PromptTokens = int64(usage.PromptTokens)
		a.CompletionTokens = int64(usage.CompletionTokens)
	}

	// Attach usage so the frontend can update session context metrics
	// without needing the legacy runner_completion envelope.
	if usage != nil {
		if a.Meta == nil {
			a.Meta = make(map[string]any)
		}
		a.Meta["usage"] = map[string]any{
			"prompt_tokens":     usage.PromptTokens,
			"completion_tokens": usage.CompletionTokens,
			"total_tokens":      usage.TotalTokens,
		}
	}
	ev := p.buildActivityEvent(a, biz.ActivityEventCompleted)
	p.publishEvent(ctx, ev, a)
}

// publishAndPersist publishes an ActivityEvent and persists to DB.
// Called with p.mu held — copies activity data before enqueuing to sequencer.
// The sequencer's per-activity channel guarantees FIFO ordering (created →
// streaming → completed) while performing I/O outside the mutex.
func (p *ActivityProjector) publishAndPersist(ctx context.Context, a *biz.Activity, eventType biz.ActivityEventType) {
	ev := p.buildActivityEvent(a, eventType)
	p.publishEvent(ctx, ev, a)
}

// publishEvent publishes a pre-built ActivityEvent and persists the activity to DB.
// Called with p.mu held — copies activity data before enqueuing to sequencer.
// The sequencer consumer goroutine performs the actual I/O (publish + persist)
// outside the caller's critical section.
func (p *ActivityProjector) publishEvent(ctx context.Context, ev biz.ActivityEvent, a *biz.Activity) {
	if p.sequencer == nil {
		return
	}
	activityCopy := *a
	if err := p.sequencer.publish(ctx, a.ID, publishTask{
		event:    ev,
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

// publishActivityDelta publishes a streaming ActivityEvent with a content patch.
// Called with p.mu held — enqueues to sequencer for ordered async publishing.
// Unlike the previous sync implementation, this no longer blocks the event loop
// on bus.Publish; backpressure is applied via the sequencer's channel buffer.
func (p *ActivityProjector) publishActivityDelta(ctx context.Context, a *biz.Activity, field, chunk string) {
	p.publishActivityDeltaWithPersist(ctx, a, field, chunk, false)
}

// publishActivityDeltaWithPersist publishes a streaming ActivityEvent and,
// when persist is true, also persists the updated activity. Used for streaming
// tool call deltas whose accumulated arguments must be saved.
func (p *ActivityProjector) publishActivityDeltaWithPersist(ctx context.Context, a *biz.Activity, field, chunk string, persist bool) {
	if p.sequencer == nil {
		return
	}
	ev := p.buildActivityEvent(a, biz.ActivityEventStreaming)
	ev.DeltaField = field
	ev.DeltaChunk = chunk
	activityCopy := *a
	if err := p.sequencer.publish(ctx, a.ID, publishTask{
		event:    ev,
		persist:  persist,
		activity: activityCopy,
	}); err != nil {
		p.lg.Warn("activity delta publish failed",
			loggateway.StepID("agent.activity_projector.delta"),
			loggateway.Str("activity_id", a.ID),
			loggateway.Err(err))
	}
}

// buildActivityEvent creates an ActivityEvent for an Activity lifecycle event.
// The Activity snapshot is included directly — no metadata packing needed,
// simplifying the frontend contract compared to the legacy Envelope format.
func (p *ActivityProjector) buildActivityEvent(a *biz.Activity, eventType biz.ActivityEventType) biz.ActivityEvent {
	// Assign the global emission sequence on first event for this activity.
	// This lets the frontend order events even when the per-activity sequencer
	// publishes different activities concurrently.
	p.activitySeq(a)

	// Build a redacted copy for the event payload. The redaction limit
	// (512 bytes) matches biz.redactActivityJSON and the frontend
	// ACTIVITY_JSON_PREVIEW_LIMIT, ensuring consistency.
	snapshot := *a
	if snapshot.ToolArguments != "" {
		snapshot.ToolArguments = biz.RedactActivityJSON(snapshot.ToolArguments)
	}
	if snapshot.ToolResult != "" {
		snapshot.ToolResult = biz.RedactActivityJSON(snapshot.ToolResult)
	}

	return biz.ActivityEvent{
		Event:    eventType,
		Activity: snapshot,
		Domain:   biz.ActivityDomainChat,
	}
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

// EmitSystemEvent publishes a transient system-domain ActivityEvent that is
// NOT persisted to the activities table. It is only delivered to live WS
// subscribers via ActivityEventBus.
//
// This is the migration target for legacy Envelope publishers that emit
// domain/system events (organization CRUD, borrow requests, skill evolution,
// knowledge ingest, etc.). These events drive live UI updates (notifications,
// sidebar badges) but do not belong to any chat turn's Activity timeline,
// so they should not pollute the activities table.
//
// The activity is emitted as a single ActivityEventCreated with
// status=Completed (one-shot notification). For multi-stage system events
// (e.g. long-running skill evolution), callers should emit separate
// created → completed pairs by calling this method twice with the same
// Activity ID — but the simpler one-shot form covers >90% of system events.
//
// Domain is set to ActivityDomainSystem, which causes the sequencer to
// skip persistence (persist=false) while still broadcasting via eventBus.
func (p *ActivityProjector) EmitSystemEvent(ctx context.Context, kind biz.ActivityKind, content string, meta map[string]any) {
	if p == nil || p.sequencer == nil {
		return
	}
	now := time.Now().UTC()
	activity := biz.Activity{
		ID:        uuid.NewString(),
		Kind:      kind,
		Status:    biz.ActivityStatusCompleted,
		Timestamp: now,
		Content:   content,
		Meta:      meta,
	}
	ev := biz.ActivityEvent{
		Event:     biz.ActivityEventCreated,
		Activity:  activity,
		Domain:    biz.ActivityDomainSystem,
	}
	if err := p.sequencer.publish(ctx, activity.ID, publishTask{
		event:    ev,
		persist:  false,
		activity: activity,
	}); err != nil {
		p.lg.Warn("system event emit failed",
			loggateway.StepID("agent.activity_projector.emit_system_event"),
			loggateway.Str("activity_id", activity.ID),
			loggateway.Str("kind", string(kind)),
			loggateway.Err(err))
	}
}
