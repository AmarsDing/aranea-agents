package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/event"
	"aranea-agents/internal/provider"
	"aranea-agents/pkg/loggateway"

	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

type ProjectMeta struct {
	SessionID          string
	RequestID          string
	InvocationID       string
	ParentInvocationID string
	TeamID             string
	Branch             string
	FilterKey          string
	RunID              string
	TraceID            string
	AgentID            string
	AgentDisplayName   string
	ContextWindow      int
	TurnPromptTokens   int
	TurnCompletionTok  int
	MemberAgentKeys    map[string]struct{} // agent_key set for team member_* envelopes
	Source             string
}

type EventProjector struct {
	eventBus      event.Bus
	memberStarted map[string]bool
	toolCalls     map[string]toolCallCache
	streamText    map[string]*strings.Builder
	metaResolver  ActivityMetaResolver
	projectMeta   ProjectMeta
	lg            loggateway.Logger
}

type toolCallCache struct {
	name      string
	argsJSON  string
	author    string
	startedAt time.Time
}

func NewEventProjector(eventBus event.Bus, lg loggateway.Logger) *EventProjector {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &EventProjector{eventBus: eventBus, lg: lg}
}

func (p *EventProjector) Configure(meta ProjectMeta, resolver ActivityMetaResolver) {
	p.projectMeta = meta
	p.metaResolver = resolver
}

func (p *EventProjector) ensureToolCallCache() {
	if p.toolCalls == nil {
		p.toolCalls = make(map[string]toolCallCache)
	}
}

func (p *EventProjector) ProjectAndPublish(ctx context.Context, ev *trpcevent.Event, meta ProjectMeta) {
	if ev == nil {
		return
	}
	p.projectMeta = meta
	envelopes := p.Project(ctx, ev, meta)
	for _, env := range envelopes {
		p.eventBus.Publish(ctx, env)
	}
}

func (p *EventProjector) Project(ctx context.Context, ev *trpcevent.Event, meta ProjectMeta) []event.Envelope {
	if ev == nil {
		return nil
	}

	if ev.IsRunnerCompletion() {
		return []event.Envelope{p.buildRunnerCompletionEnvelope(ev, meta)}
	}

	if ev.Response != nil && ev.Response.Error != nil {
		return []event.Envelope{p.buildErrorEnvelope(ev, meta)}
	}

	if len(ev.StateDelta) > 0 {
		return []event.Envelope{p.buildStateDeltaEnvelope(ev, meta)}
	}

	if ev.Response == nil {
		return nil
	}

	objType := ev.Response.Object
	switch objType {
	case trpcmodel.ObjectTypeChatCompletionChunk:
		return p.projectChatCompletionChunk(ctx, ev, meta)
	case trpcmodel.ObjectTypeChatCompletion:
		return p.projectChatCompletion(ctx, ev, meta)
	case trpcmodel.ObjectTypeToolResponse:
		return []event.Envelope{p.buildToolResultEnvelope(ctx, ev, meta)}
	case trpcmodel.ObjectTypeTransfer:
		return []event.Envelope{p.buildTransferEnvelope(ev, meta)}
	default:
		return nil
	}
}

func (p *EventProjector) baseEnvelope(ev *trpcevent.Event, meta ProjectMeta, typ event.EnvelopeType) event.Envelope {
	env := event.NewEnvelope(typ, ev.Author, meta.SessionID)
	if ev.ID != "" {
		env.ID = ev.ID
	}
	env.RequestID = meta.RequestID
	if ev.RequestID != "" {
		env.RequestID = ev.RequestID
	}
	env.InvocationID = ev.InvocationID
	if meta.InvocationID != "" {
		env.InvocationID = meta.InvocationID
	}
	env.ParentInvocationID = ev.ParentInvocationID
	if meta.ParentInvocationID != "" {
		env.ParentInvocationID = meta.ParentInvocationID
	}
	env.Branch = coalesceStr(ev.Branch, meta.Branch)
	env.FilterKey = coalesceStr(ev.FilterKey, meta.FilterKey)
	env.Tag = ev.Tag
	env.TeamID = meta.TeamID
	env.Version = ev.Version
	if !ev.Timestamp.IsZero() {
		env.Timestamp = ev.Timestamp.UTC().Format(time.RFC3339Nano)
	}
	if len(ev.Extensions) > 0 {
		env.Extensions = make(map[string]string, len(ev.Extensions))
		for k, v := range ev.Extensions {
			env.Extensions[k] = string(v)
		}
	}
	if ev.Actions != nil {
		env.Actions = &event.EnvelopeActions{
			SkipSummarization: ev.Actions.SkipSummarization,
		}
	}
	if src := strings.TrimSpace(meta.Source); src != "" {
		env.Source = src
	}
	return env
}

func (p *EventProjector) projectChatCompletionChunk(ctx context.Context, ev *trpcevent.Event, meta ProjectMeta) []event.Envelope {
	var envelopes []event.Envelope
	for _, choice := range ev.Response.Choices {
		msg := choice.Message
		delta := choice.Delta

		hasToolCalls := len(msg.ToolCalls) > 0 || len(delta.ToolCalls) > 0
		text, reasoning := ChoiceStreamContent(choice, ev.Response.IsPartial)
		hasContent := text != ""
		hasReasoning := reasoning != ""

		if hasToolCalls {
			allCalls := append(msg.ToolCalls, delta.ToolCalls...)
			for _, tc := range allCalls {
				env := p.baseEnvelope(ev, meta, event.EnvelopeTypeToolCall)
				argsJSON := string(tc.Function.Arguments)
				startedAt := time.Now().UTC()
				if !ev.Timestamp.IsZero() {
					startedAt = ev.Timestamp.UTC()
				}
				p.ensureToolCallCache()
				p.toolCalls[tc.ID] = toolCallCache{
					name:      tc.Function.Name,
					argsJSON:  argsJSON,
					author:    ev.Author,
					startedAt: startedAt,
				}
				_, isLongRunning := ev.LongRunningToolIDs[tc.ID]
				env.ToolCall = p.buildToolCallEnvelope(ctx, tc.ID, tc.Function.Name, argsJSON, "", "calling", ev.Author, startedAt, nil, 0, "", isLongRunning)
				p.attachActivityMetadata(&env)
				envelopes = append(envelopes, env)
			}
		}

		if hasContent || hasReasoning {
			rawText := text
			rawReasoning := reasoning

			if isTeamMemberAuthor(ev.Author, meta) {
				envelopes = append(envelopes, p.projectMemberText(ev, meta, rawText, rawReasoning, ev.Response.IsPartial)...)
				continue
			}

			if ev.Response.IsPartial {
				textDelta := p.visibleStreamDelta(streamKey(ev.Author, meta), rawText)
				reasoningDelta := p.visibleStreamDelta(streamKey(ev.Author, meta)+":reasoning", rawReasoning)
				if textDelta == "" && reasoningDelta == "" {
					continue
				}
				env := p.baseEnvelope(ev, meta, event.EnvelopeTypeTextDelta)
				env.Content = &event.EnvelopeContent{
					Text:      textDelta,
					Reasoning: reasoningDelta,
					IsPartial: true,
				}
				envelopes = append(envelopes, env)
			} else {
				textDone := rawText
				reasoningDone := rawReasoning
				if b := p.streamBuilder(streamKey(ev.Author, meta)); b != nil && b.Len() > 0 {
					textDone = b.String()
				}
				if b := p.streamBuilder(streamKey(ev.Author, meta) + ":reasoning"); b != nil && b.Len() > 0 {
					reasoningDone = b.String()
				}
				env := p.baseEnvelope(ev, meta, event.EnvelopeTypeTextDone)
				env.Content = &event.EnvelopeContent{
					Text:      textDone,
					Reasoning: reasoningDone,
					IsPartial: false,
				}
				envelopes = append(envelopes, env)
			}
		}
	}
	return envelopes
}

func (p *EventProjector) projectChatCompletion(ctx context.Context, ev *trpcevent.Event, meta ProjectMeta) []event.Envelope {
	var envelopes []event.Envelope
	for _, choice := range ev.Response.Choices {
		msg := choice.Message
		text := strings.TrimSpace(msg.Content)
		reasoning := strings.TrimSpace(msg.ReasoningContent)

		if len(msg.ToolCalls) > 0 {
			for _, tc := range msg.ToolCalls {
				env := p.baseEnvelope(ev, meta, event.EnvelopeTypeToolCall)
				argsJSON := string(tc.Function.Arguments)
				startedAt := time.Now().UTC()
				if !ev.Timestamp.IsZero() {
					startedAt = ev.Timestamp.UTC()
				}
				p.ensureToolCallCache()
				p.toolCalls[tc.ID] = toolCallCache{
					name:      tc.Function.Name,
					argsJSON:  argsJSON,
					author:    ev.Author,
					startedAt: startedAt,
				}
				env.ToolCall = p.buildToolCallEnvelope(ctx, tc.ID, tc.Function.Name, argsJSON, "", "calling", ev.Author, startedAt, nil, 0, "", false)
				p.attachActivityMetadata(&env)
				envelopes = append(envelopes, env)
			}
		}

		if text != "" || reasoning != "" {
			env := p.baseEnvelope(ev, meta, event.EnvelopeTypeTextDone)
			env.Content = &event.EnvelopeContent{
				Text:      text,
				Reasoning: reasoning,
				IsPartial: false,
			}
			envelopes = append(envelopes, env)
		}
	}
	return envelopes
}

func (p *EventProjector) buildRunnerCompletionEnvelope(ev *trpcevent.Event, meta ProjectMeta) event.Envelope {
	env := p.baseEnvelope(ev, meta, event.EnvelopeTypeRunnerCompletion)
	if ev.Response != nil {
		if ev.Response.Usage != nil {
			u := &event.EnvelopeUsage{
				PromptTokens:     ev.Response.Usage.PromptTokens,
				CompletionTokens: ev.Response.Usage.CompletionTokens,
				TotalTokens:      ev.Response.Usage.TotalTokens,
			}
			if meta.ContextWindow > 0 {
				u.MaxTokens = meta.ContextWindow
			}
			if meta.TurnPromptTokens > 0 || meta.TurnCompletionTok > 0 {
				u.PromptTokens = meta.TurnPromptTokens
				u.CompletionTokens = meta.TurnCompletionTok
				u.TotalTokens = meta.TurnPromptTokens + meta.TurnCompletionTok
				u.TurnTotalTokens = u.TotalTokens
				u.ContextPromptTokens = meta.TurnPromptTokens
			} else if ev.Response.Usage.PromptTokens > 0 {
				u.ContextPromptTokens = ev.Response.Usage.PromptTokens
			}
			env.Usage = u
		}
		if ev.Response.Error != nil {
			errType := ev.Response.Error.Type
			if errType == "" {
				errType = "run_error"
			}
			env.Error = &event.EnvelopeError{
				Type:    errType,
				Code:    errType,
				Message: ev.Response.Error.Message,
			}
		}
	}
	runKind := "chat"
	if strings.TrimSpace(meta.TeamID) != "" {
		runKind = "team"
	}
	md := map[string]any{"run_kind": runKind}
	if v := strings.TrimSpace(meta.RunID); v != "" {
		md["run_id"] = v
	}
	if v := strings.TrimSpace(meta.TraceID); v != "" {
		md["trace_id"] = v
	}
	if v := strings.TrimSpace(meta.AgentID); v != "" {
		md["agent_id"] = v
	}
	if v := strings.TrimSpace(meta.AgentDisplayName); v != "" {
		md["agent_display_name"] = v
	}
	if len(md) > 0 {
		env.Metadata = md
	}
	return env
}

func (p *EventProjector) buildErrorEnvelope(ev *trpcevent.Event, meta ProjectMeta) event.Envelope {
	env := p.baseEnvelope(ev, meta, event.EnvelopeTypeError)
	if ev.Response != nil && ev.Response.Error != nil {
		errType := ev.Response.Error.Type
		if errType == "" {
			errType = "run_error"
		}
		env.Error = &event.EnvelopeError{
			Type:    errType,
			Code:    errType,
			Message: ev.Response.Error.Message,
		}
	}
	return env
}

func (p *EventProjector) buildStateDeltaEnvelope(ev *trpcevent.Event, meta ProjectMeta) event.Envelope {
	env := p.baseEnvelope(ev, meta, event.EnvelopeTypeStateDelta)
	if len(ev.StateDelta) > 0 {
		combined, _ := json.Marshal(ev.StateDelta)
		env.StateDelta = &event.EnvelopeStateDelta{
			Operation: "set",
			Path:      "__state__",
			ValueJSON: string(combined),
		}
	}
	return env
}

func (p *EventProjector) buildToolResultEnvelope(ctx context.Context, ev *trpcevent.Event, meta ProjectMeta) event.Envelope {
	env := p.baseEnvelope(ev, meta, event.EnvelopeTypeToolResult)
	if ev.Response == nil || len(ev.Response.Choices) == 0 {
		return env
	}

	msg := ev.Response.Choices[0].Message
	toolID := strings.TrimSpace(msg.ToolID)
	toolName := coalesceStr(strings.TrimSpace(msg.ToolName), strings.TrimSpace(ev.Author))
	argsJSON := ""
	author := ev.Author
	startedAt := time.Time{}
	if toolID != "" {
		if cached, ok := p.toolCalls[toolID]; ok {
			toolName = coalesceStr(toolName, cached.name)
			argsJSON = cached.argsJSON
			author = coalesceStr(author, cached.author)
			startedAt = cached.startedAt
		}
		if argsJSON == "" {
			argsJSON = p.lookupToolCallArgs(ev, toolID)
		}
	}

	resultRaw, _ := json.Marshal(msg.Content)
	resultJSON := string(resultRaw)
	status := "success"
	errorCode := ""
	var errMsg string
	if ev.Response.Error != nil {
		status = "failed"
		errorCode = coalesceStr(ev.Response.Error.Type, event.ErrorCodeToolError)
		errMsg = ev.Response.Error.Message
		if resultRaw == nil || string(resultRaw) == "null" || string(resultRaw) == `""` {
			resultRaw, _ = json.Marshal(map[string]string{"error": errMsg})
		}
	}

	finishedAt := time.Now().UTC()
	if !ev.Timestamp.IsZero() {
		finishedAt = ev.Timestamp.UTC()
	}
	var durationMS int64
	if !startedAt.IsZero() {
		durationMS = finishedAt.Sub(startedAt).Milliseconds()
		if durationMS < 0 {
			durationMS = 0
		}
	}

	// Flatten todo_write result: add a human-readable summary so the
	// ChatExecutionCard can display one line instead of raw nested JSON.
	if toolName == "todo_write" {
		resultJSON = flattenTodoWriteResult(resultJSON)
	}

	env.ToolCall = p.buildToolCallEnvelope(ctx, toolID, toolName, argsJSON, resultJSON, status, author, startedAt, &finishedAt, durationMS, errorCode, false)
	if errMsg != "" && env.ToolCall.ResultJSON != "" {
		env.ToolCall.ResultJSON = mergeToolErrorResult(env.ToolCall.ResultJSON, errMsg)
	}
	p.attachActivityMetadata(&env)
	return env
}

func (p *EventProjector) buildToolCallEnvelope(
	ctx context.Context,
	id, name, argsJSON, resultJSON, status, author string,
	startedAt time.Time,
	finishedAt *time.Time,
	durationMS int64,
	errorCode string,
	isLongRunning bool,
) *event.EnvelopeToolCall {
	metaInput := BuildActivityMeta(ctx, ActivityMetaInput{
		ToolName:      name,
		ArgumentsJSON: argsJSON,
		ResultJSON:    resultJSON,
		Status:        status,
		Author:        author,
		StartedAt:     startedAt,
		FinishedAt:    finishedAt,
		DurationMS:    durationMS,
		ErrorCode:     errorCode,
	}, p.metaResolver)
	agentName := firstNonEmptyStr(metaInput.AgentName, p.projectMeta.AgentDisplayName, author, metaInput.AgentKey)
	tc := &event.EnvelopeToolCall{
		ID:            id,
		Name:          name,
		ArgumentsJSON: metaInput.ArgumentsJSON,
		ResultJSON:    metaInput.ResultJSON,
		Status:        status,
		DurationMS:    durationMS,
		IsLongRunning: isLongRunning,
		ActivityKind:  metaInput.ActivityKind,
		DisplayLabel:  metaInput.DisplayLabel,
		IconKey:       metaInput.IconKey,
		Summary:       metaInput.Summary,
		StartedAt:     metaInput.StartedAt,
		FinishedAt:    metaInput.FinishedAt,
		ErrorCode:     metaInput.ErrorCode,
		AgentKey:      metaInput.AgentKey,
		AgentID:       firstNonEmptyStr(metaInput.AgentID, p.projectMeta.AgentID),
		AgentName:     agentName,
		RunID:         strings.TrimSpace(p.projectMeta.RunID),
		TraceID:       strings.TrimSpace(p.projectMeta.TraceID),
	}
	tc.ValidateErrorCode()
	return tc
}

func (p *EventProjector) attachActivityMetadata(env *event.Envelope) {
	if env == nil || env.ToolCall == nil {
		return
	}
	tc := env.ToolCall
	if env.Metadata == nil {
		env.Metadata = make(map[string]any, 4)
	}
	env.Metadata["activity_kind"] = tc.ActivityKind
	env.Metadata["display_label"] = tc.DisplayLabel
	if tc.Summary != "" {
		env.Metadata["summary"] = tc.Summary
	}
	if tc.RunID != "" {
		env.Metadata["run_id"] = tc.RunID
	}
	if tc.TraceID != "" {
		env.Metadata["trace_id"] = tc.TraceID
	}
	if tc.AgentName != "" {
		env.Metadata["agent_name"] = tc.AgentName
	}
}

func (p *EventProjector) lookupToolCallArgs(ev *trpcevent.Event, toolID string) string {
	argsByID, ok, err := trpcevent.GetExtension[map[string]string](ev, trpcevent.ToolCallArgsExtensionKey)
	if err != nil {
		p.lg.Warn("tool call args extension lookup failed", loggateway.StepID("agent.event.tool_args_lookup"), loggateway.Str("tool_id", toolID), loggateway.Err(err))
		return ""
	}
	if !ok {
		return ""
	}
	return argsByID[toolID]
}

func mergeToolErrorResult(resultJSON, errMsg string) string {
	var parsed map[string]any
	if err := json.Unmarshal([]byte(resultJSON), &parsed); err != nil || parsed == nil {
		out, _ := json.Marshal(map[string]string{"error": errMsg})
		return string(out)
	}
	if _, ok := parsed["error"]; !ok && errMsg != "" {
		parsed["error"] = errMsg
	}
	out, err := json.Marshal(parsed)
	if err != nil {
		return resultJSON
	}
	return string(out)
}

// flattenTodoWriteResult parses a todo_write tool result and injects a
// "summary" field that the ChatExecutionCard can display as a one-liner
// instead of dumping the entire nested JSON. The original fields (message,
// todos, oldTodos) are preserved so useTodoBoard.ts continues to work.
func flattenTodoWriteResult(resultJSON string) string {
	var parsed map[string]any
	if err := json.Unmarshal([]byte(resultJSON), &parsed); err != nil || parsed == nil {
		return resultJSON
	}
	todosRaw, ok := parsed["todos"]
	if !ok {
		return resultJSON
	}
	todosArr, ok := todosRaw.([]any)
	if !ok || len(todosArr) == 0 {
		parsed["summary"] = "0 tasks"
		out, err := json.Marshal(parsed)
		if err != nil {
			return resultJSON
		}
		return string(out)
	}
	counts := map[string]int{"pending": 0, "in_progress": 0, "completed": 0}
	for _, item := range todosArr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		statusVal, _ := m["status"].(string)
		if _, valid := counts[statusVal]; valid {
			counts[statusVal]++
		}
	}
	summary := fmt.Sprintf("%d tasks: %d pending, %d in_progress, %d completed",
		len(todosArr), counts["pending"], counts["in_progress"], counts["completed"])
	parsed["summary"] = summary
	out, err := json.Marshal(parsed)
	if err != nil {
		return resultJSON
	}
	return string(out)
}

func (p *EventProjector) buildTransferEnvelope(ev *trpcevent.Event, meta ProjectMeta) event.Envelope {
	env := p.baseEnvelope(ev, meta, event.EnvelopeTypeTransfer)
	if ev.Response != nil && len(ev.Response.Choices) > 0 {
		msg := ev.Response.Choices[0].Message
		parts := strings.SplitN(msg.Content, "→", 2)
		from := strings.TrimSpace(parts[0])
		to := ""
		if len(parts) > 1 {
			to = strings.TrimSpace(parts[1])
		}
		env.Transfer = &event.EnvelopeTransfer{
			FromAgent: from,
			ToAgent:   to,
		}
	}
	if env.Transfer == nil {
		env.Transfer = &event.EnvelopeTransfer{
			FromAgent: ev.ParentInvocationID,
			ToAgent:   ev.Author,
		}
	}
	return env
}

func (p *EventProjector) BuildLogEnvelope(level, message, source, sessionID string) event.Envelope {
	env := event.NewEnvelope(event.EnvelopeTypeLog, source, sessionID)
	env.Metadata = map[string]any{
		"level":  level,
		"source": source,
	}
	env.Content = &event.EnvelopeContent{
		Text:      message,
		IsPartial: false,
	}
	return env
}

func (p *EventProjector) BuildIntentPassEnvelope(payload map[string]any, sessionID, teamID string) event.Envelope {
	env := event.NewEnvelope(event.EnvelopeTypeIntentPass, "system", sessionID)
	env.TeamID = teamID
	env.Metadata = payload
	return env
}

func (p *EventProjector) BuildMemberMessageStartEnvelope(author, sessionID, teamID, branch string) event.Envelope {
	env := event.NewEnvelope(event.EnvelopeTypeMemberMessageStart, author, sessionID)
	env.TeamID = teamID
	env.Branch = branch
	return env
}

func (p *EventProjector) BuildMemberDeltaEnvelope(author, sessionID, teamID, text string) event.Envelope {
	env := event.NewEnvelope(event.EnvelopeTypeMemberDelta, author, sessionID)
	env.TeamID = teamID
	env.Content = &event.EnvelopeContent{
		Text:      text,
		IsPartial: true,
	}
	return env
}

func (p *EventProjector) BuildMemberMessageDoneEnvelope(author, sessionID, teamID, text string) event.Envelope {
	env := event.NewEnvelope(event.EnvelopeTypeMemberMessageDone, author, sessionID)
	env.TeamID = teamID
	env.Content = &event.EnvelopeContent{
		Text:      text,
		IsPartial: false,
	}
	return env
}

func isTeamMemberAuthor(author string, meta ProjectMeta) bool {
	if meta.TeamID == "" || strings.TrimSpace(author) == "" {
		return false
	}
	if strings.HasPrefix(author, "team") {
		return false
	}
	if len(meta.MemberAgentKeys) == 0 {
		return true
	}
	_, ok := meta.MemberAgentKeys[author]
	return ok
}

func (p *EventProjector) projectMemberText(ev *trpcevent.Event, meta ProjectMeta, text, reasoning string, isPartial bool) []event.Envelope {
	author := strings.TrimSpace(ev.Author)
	if author == "" {
		return nil
	}
	if p.memberStarted == nil {
		p.memberStarted = make(map[string]bool)
	}
	var out []event.Envelope
	if !p.memberStarted[author] {
		p.memberStarted[author] = true
		out = append(out, p.BuildMemberMessageStartEnvelope(author, meta.SessionID, meta.TeamID, ev.Branch))
	}
	key := streamKey(author, meta)
	textDelta := p.visibleStreamDelta(key, text)
	reasoningDelta := p.visibleStreamDelta(key+":reasoning", reasoning)
	combined := strings.TrimSpace(textDelta)
	if combined == "" {
		combined = strings.TrimSpace(reasoningDelta)
	}
	if combined == "" {
		return out
	}
	if isPartial {
		out = append(out, p.BuildMemberDeltaEnvelope(author, meta.SessionID, meta.TeamID, combined))
	} else {
		out = append(out, p.BuildMemberMessageDoneEnvelope(author, meta.SessionID, meta.TeamID, combined))
	}
	return out
}

func coalesceStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func streamKey(author string, meta ProjectMeta) string {
	key := strings.TrimSpace(author)
	if key == "" {
		key = strings.TrimSpace(meta.SessionID)
	}
	return key
}

func (p *EventProjector) streamBuilder(key string) *strings.Builder {
	if p.streamText == nil {
		p.streamText = make(map[string]*strings.Builder)
	}
	b, ok := p.streamText[key]
	if !ok {
		b = &strings.Builder{}
		p.streamText[key] = b
	}
	return b
}

func (p *EventProjector) visibleStreamDelta(key, chunk string) string {
	if strings.TrimSpace(chunk) == "" {
		return ""
	}
	return provider.VisibleStreamingDelta(p.streamBuilder(key), chunk)
}

func roughTokenEstimateFromText(text string) int {
	return len(text) / 4
}

func FormatMonitorMessage(phase, sessionID string, args ...any) string {
	var sb strings.Builder
	sb.WriteString(phase)
	fmt.Fprintf(&sb, " session_id=%s", sessionID)
	for i := 0; i+1 < len(args); i += 2 {
		fmt.Fprintf(&sb, " %v=%v", args[i], args[i+1])
	}
	return sb.String()
}
