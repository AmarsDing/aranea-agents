package agent

import (
	"context"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
)

// ActivityProjector projects runtime events into Activity semantic units
// and publishes them via WS, eliminating frontend inference.
// It runs parallel to EventProjector during the AF-1 dual-emission phase.
type ActivityProjector struct {
	mu           sync.Mutex
	eventBus     event.Bus
	activityRepo biz.ActivityWriter
	metaResolver ActivityMetaResolver
	lg           loggateway.Logger

	// Turn-scoped state (reset per turn)
	rootActivityID string
	activities     map[string]*biz.Activity // id -> activity
	toolCalls      map[string]*biz.Activity // tool_call_id -> action activity
	kindAuthorMap  map[string]string        // "kind:author" -> activity ID (O(1) lookup)
	reasoningBuf   map[string]*strings.Builder
	meta           ProjectMeta
}

// NewActivityProjector creates a new ActivityProjector.
func NewActivityProjector(eventBus event.Bus, activityRepo biz.ActivityWriter, lg loggateway.Logger) *ActivityProjector {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &ActivityProjector{
		eventBus:     eventBus,
		activityRepo: activityRepo,
		lg:           lg,
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
func (p *ActivityProjector) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.rootActivityID = ""
	p.activities = make(map[string]*biz.Activity)
	p.toolCalls = make(map[string]*biz.Activity)
	p.kindAuthorMap = make(map[string]string)
	p.reasoningBuf = make(map[string]*strings.Builder)
}

// OnTurnStart creates the root task Activity for a new turn.
func (p *ActivityProjector) OnTurnStart(ctx context.Context, meta ProjectMeta) {
	p.mu.Lock()
	defer p.mu.Unlock()
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

// OnError creates an error Activity.
func (p *ActivityProjector) OnError(ctx context.Context, errMsg string) {
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
	p.publishAndPersist(ctx, a, contract.EnvelopeTypeActivityStart)
}

// OnTurnEnd finalizes the root task Activity.
func (p *ActivityProjector) OnTurnEnd(ctx context.Context) {
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

	p.publishAndPersist(ctx, a, contract.EnvelopeTypeActivityDone)
}

// publishAndPersist publishes an Activity envelope and persists to DB.
// Called with p.mu held — copies data before releasing mutex for I/O.
func (p *ActivityProjector) publishAndPersist(ctx context.Context, a *biz.Activity, envType contract.EnvelopeType) {
	// Copy activity data before I/O to avoid holding mutex during blocking operations
	activityCopy := *a
	env := p.buildActivityEnvelope(a, envType)

	// I/O operations (event bus publish + DB write) happen after mutex release
	// since callers unlock p.mu immediately after this method returns.
	go func() {
		if p.eventBus != nil {
			p.eventBus.Publish(ctx, env)
		}
		if p.activityRepo != nil {
			if _, err := p.activityRepo.UpsertActivity(ctx, activityCopy); err != nil {
				p.lg.Warn("activity persist failed",
					loggateway.StepID("agent.activity_projector.persist"),
					loggateway.Str("activity_id", activityCopy.ID),
					loggateway.Str("kind", string(activityCopy.Kind)),
					loggateway.Err(err))
			}
		}
	}()
}

// publishActivityDelta publishes an activity_delta envelope with a content patch.
func (p *ActivityProjector) publishActivityDelta(ctx context.Context, a *biz.Activity, field, chunk string) {
	env := p.buildActivityEnvelope(a, contract.EnvelopeTypeActivityDelta)
	if env.Metadata == nil {
		env.Metadata = make(map[string]any)
	}
	env.Metadata["delta_field"] = field
	env.Metadata["delta_chunk"] = chunk
	if p.eventBus != nil {
		p.eventBus.Publish(ctx, env)
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
	}

	// Content fields
	if a.Content != "" {
		env.Metadata["content"] = a.Content
	}
	if a.Reasoning != "" {
		env.Metadata["reasoning"] = a.Reasoning
	}

	// Tool fields (exclude sensitive data from WS envelope — available via API)
	if a.ToolName != "" {
		env.Metadata["tool_name"] = a.ToolName
	}
	if a.ToolCallID != "" {
		env.Metadata["tool_call_id"] = a.ToolCallID
	}
	// tool_arguments and tool_result are NOT sent via WS for security;
	// clients should fetch them via the ListActivities API if needed.
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


