package plugintrpc

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/metrics"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
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
			if err := executeHookAction(ctx, stats, notifier, rh, "on_event", agentID, agentKey, "", e); err != nil {
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

func eventTypeLabel(e *trpcevent.Event) string {
	if e == nil {
		return ""
	}
	if e.IsRunnerCompletion() {
		return "runner_completion"
	}
	if e.Response != nil {
		return "model_response"
	}
	return "event"
}
