package agent

import (
	"context"
	"strings"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

func newMemoryInjectBeforeHook(ag biz.Agent, deps TRPCBuilderDeps) callbacks.Callback {
	if ag.Settings == nil {
		return nil
	}
	l3 := ag.Settings.L3Enabled && ag.Settings.L0InjectL3
	l2 := ag.Settings.L2RecallEnabled
	if !l3 && !l2 {
		return nil
	}
	if l2 && deps.MemoryL2Recall == nil && l3 && deps.MemoryL3Recall == nil {
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
	inv, ok := trpcagent.InvocationFromContext(ctx)
	if !ok || inv == nil || inv.Session == nil {
		return ""
	}
	userID := strings.TrimSpace(inv.Session.UserID)
	sessionID := strings.TrimSpace(inv.Session.ID)
	keyword := lastUserMessageText(messages)
	var parts []string
	if ag.Settings.L2RecallEnabled {
		if l2 := L2MemoryCue(ctx, deps.MemoryL2Recall, ag, sessionID, keyword, 0); l2 != "" {
			parts = append(parts, l2)
		}
	}
	if ag.Settings.L3Enabled && ag.Settings.L0InjectL3 {
		if l3 := L3MemoryCue(ctx, deps.MemoryL3Recall, ag, userID, keyword, 0); l3 != "" {
			parts = append(parts, l3)
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
				return t[:120]
			}
			return t
		}
	}
	return ""
}
