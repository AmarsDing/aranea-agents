package plugintrpc

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/metrics"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

func (m *Manager) dispatchHookOnEvent(
	ctx context.Context,
	invocation *trpcagent.Invocation,
	e *trpcevent.Event,
) (*trpcevent.Event, error) {
	if m == nil || m.hooks == nil || e == nil {
		return e, nil
	}
	_, agentKey := sessionAgentKey(ctx, invocation)
	agentID := m.platformAgentID(ctx, agentKey)
	if agentID == "" {
		agentID = agentKeyFromInvocation(invocation)
	}
	if agentID == "" {
		agentID = agentKey
	}
	resolved := m.hooks.Resolve(agentID, agentKey)
	if len(resolved) == 0 {
		return e, nil
	}
	eventType := eventTypeLabel(e)
	var blockedErr error
	for _, rh := range resolved {
		if rh.Rule.CallbackPoint != "on_event" {
			continue
		}
		if !hookEventMatches(rh.Rule.Condition, eventType) {
			continue
		}
		var stats StatsRecorder
		var notifier *HookNotifier
		if m.rt != nil {
			stats = m.rt.stats
			notifier = m.rt.HookNotifier()
		}
		func() {
			defer func() { recoverHookPanic("on_event", recover(), nil) }()
			if err := executeHookAction(ctx, stats, notifier, rh, "on_event", agentID, agentKey, "", e, m.lg); err != nil {
				if metrics.IsBlockedErr(err) {
					blockedErr = err
					return
				}
				getHookLogger().Warn("hook: non-block error suppressed", "point", "on_event", "agent_id", agentID, "error", err)
			}
		}()
		if blockedErr != nil {
			return e, blockedErr
		}
	}
	return e, nil
}

func hookEventMatches(cond biz.HookCondition, eventType string) bool {
	want := strings.TrimSpace(cond.EventType)
	if want == "" {
		return true
	}
	return strings.EqualFold(want, eventType)
}

// eventTypeLabel returns a fine-grained event type label for hook rule matching.
// Maps framework event.Object to a stable label string that hook conditions can match against.
func eventTypeLabel(e *trpcevent.Event) string {
	if e == nil {
		return ""
	}
	if e.IsRunnerCompletion() {
		return "runner_completion"
	}
	if e.Response == nil {
		return "event"
	}
	// Fine-grained classification based on event.Object (framework model.ObjectType).
	switch e.Response.Object {
	case trpcmodel.ObjectTypeChatCompletionChunk:
		return "chat.completion.chunk"
	case trpcmodel.ObjectTypeChatCompletion:
		return "chat.completion"
	case trpcmodel.ObjectTypeToolResponse:
		return "tool.response"
	case trpcmodel.ObjectTypeError:
		return "error"
	case trpcmodel.ObjectTypeTransfer:
		return "agent.transfer"
	case trpcmodel.ObjectTypeStateUpdate:
		return "state.update"
	case trpcmodel.ObjectTypePreprocessingBasic,
		trpcmodel.ObjectTypePreprocessingContent,
		trpcmodel.ObjectTypePreprocessingIdentity,
		trpcmodel.ObjectTypePreprocessingInstruction,
		trpcmodel.ObjectTypePreprocessingPlanning:
		return "preprocessing"
	case trpcmodel.ObjectTypePostprocessingPlanning,
		trpcmodel.ObjectTypePostprocessingCodeExecution:
		return "postprocessing"
	default:
		return "model_response"
	}
}
