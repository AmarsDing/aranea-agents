package agent

import (
	"context"
	"strings"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

func memoryRuntimeContext(inv *trpcagent.Invocation, ag biz.Agent) biz.MemoryRuntimeContext {
	rt := biz.MemoryRuntimeContext{
		AgentID: strings.TrimSpace(ag.ID),
	}
	if inv != nil && inv.Session != nil {
		rt.UserID = strings.TrimSpace(inv.Session.UserID)
		rt.Workspace = sessionStateString(inv.Session.State, "workspace")
		rt.TeamID = sessionStateString(inv.Session.State, "team_id")
	}
	if rt.Workspace == "" && ag.Settings != nil {
		rt.Workspace = strings.TrimSpace(ag.Settings.Workspace)
	}
	return rt
}

func sessionStateString(state map[string][]byte, key string) string {
	if state == nil {
		return ""
	}
	if b, ok := state[key]; ok {
		return strings.TrimSpace(string(b))
	}
	return ""
}

func newMemoryInjectBeforeHook(ag biz.Agent, deps TRPCBuilderDeps) callbacks.Callback {
	policy := biz.ResolveMemoryRuntimePolicy(ag.Settings)
	if !policy.MasterEnabled || !policy.AnyInject() {
		return nil
	}
	hasDep := (policy.InjectL1 || policy.InjectL4) && deps.MemoryAdmin != nil
	hasDep = hasDep || (policy.RecallL2 && deps.MemoryL2Recall != nil)
	hasDep = hasDep || (policy.InjectL3 && deps.MemoryL3Recall != nil)
	hasDep = hasDep || (policy.RecallL2 && policy.InjectL3 && deps.MemoryCompositeRecall != nil)
	if !hasDep {
		return nil
	}
	return callbacks.NewBeforeModelHook(5, func(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
		if args == nil || args.Request == nil {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		cue := buildRuntimeMemoryCue(ctx, deps, ag, args.Request.Messages)
		if cue == "" {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		sys := trpcmodel.NewSystemMessage(cue)
		args.Request.Messages = append([]trpcmodel.Message{sys}, args.Request.Messages...)
		return &trpcmodel.BeforeModelResult{Context: ctx}, nil
	})
}

func buildRuntimeMemoryCue(ctx context.Context, deps TRPCBuilderDeps, ag biz.Agent, messages []trpcmodel.Message) string {
	policy := biz.ResolveMemoryRuntimePolicy(ag.Settings)
	if !policy.MasterEnabled {
		return ""
	}
	inv, ok := trpcagent.InvocationFromContext(ctx)
	if !ok || inv == nil || inv.Session == nil {
		return ""
	}
	rt := memoryRuntimeContext(inv, ag)
	sessionID := strings.TrimSpace(inv.Session.ID)
	keyword := RecallKeywordFromMessages(messages)
	var parts []string
	if policy.InjectL1 {
		if l1 := L1MemoryCue(ctx, deps.MemoryAdmin, ag, policy, sessionID); l1 != "" {
			parts = append(parts, l1)
		}
	}
	if policy.RecallL2 && policy.InjectL3 && deps.MemoryCompositeRecall != nil {
		if composite := CompositeMemoryCue(ctx, deps.MemoryCompositeRecall, ag, policy, rt, sessionID, keyword, 0); composite != "" {
			parts = append(parts, composite)
		}
	} else {
		if policy.RecallL2 {
			if l2 := L2MemoryCue(ctx, deps.MemoryL2Recall, ag, policy, sessionID, keyword, 0); l2 != "" {
				parts = append(parts, l2)
			}
		}
		if policy.InjectL3 {
			if l3 := L3MemoryCue(ctx, deps.MemoryL3Recall, ag, policy, rt, keyword, 0); l3 != "" {
				parts = append(parts, l3)
			}
		}
	}
	if policy.InjectL4 {
		if l4 := L4MemoryCue(ctx, deps.MemoryAdmin, ag, policy, keyword); l4 != "" {
			parts = append(parts, l4)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func lastUserMessageText(messages []trpcmodel.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != trpcmodel.RoleUser {
			continue
		}
		if t := strings.TrimSpace(messages[i].Content); t != "" {
			if len(t) > 120 {
				return safeTruncate(t, 120)
			}
			return t
		}
	}
	return ""
}
