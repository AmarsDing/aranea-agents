package agent

import (
	"context"
	"unicode/utf8"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

func newStaticRuntimeCueBeforeHook(ag biz.Agent, deps TRPCBuilderDeps) callbacks.Callback {
	if ag.Settings == nil {
		return nil
	}
	level := capabilityCueLevelForMode(ag.SystemPromptMode)
	if level == cueLevelMinimal && !ag.Settings.ToolsEnabled && !ag.Settings.SubagentsEnabled {
		return nil
	}
	customKeys := customToolKeysFromDeps(deps)
	return callbacks.NewBeforeModelHook(4, callbacks.LayerStatic, func(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
		if args == nil || args.Request == nil {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		promptDeps := Deps{
			Agents:                 deps.Agents,
			AgentUC:                deps.AgentUC,
			SessionMemoryAvailable: deps.HasMemory,
			LG:                     deps.Logger(),
			CustomToolKeys:         customKeys,
		}
		cue := StaticRuntimeCapabilityCue(ctx, promptDeps, ag)
		if cue == "" {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		// 上下文预算台账（29-token §9.6）：仅计量，不改注入逻辑。
		recordContextBudgetOnce(ctx, ContextBudgetCategoryStaticPrefix, utf8.RuneCountInString(cue))
		// Prefix stabilization: append after the existing system block so the
		// session-stable prefix stays intact for prompt caching (never prepend).
		sys := trpcmodel.NewSystemMessage(cue)
		args.Request.Messages = insertAfterLastSystem(args.Request.Messages, sys)
		return &trpcmodel.BeforeModelResult{Context: ctx}, nil
	})
}

func newDynamicRuntimeCueBeforeHook(ag biz.Agent, deps TRPCBuilderDeps) callbacks.Callback {
	if ag.Settings == nil {
		return nil
	}
	level := capabilityCueLevelForMode(ag.SystemPromptMode)
	if level == cueLevelMinimal && !ag.Settings.ToolsEnabled && !ag.Settings.SubagentsEnabled {
		return nil
	}
	customKeys := customToolKeysFromDeps(deps)
	return callbacks.NewBeforeModelHook(4, callbacks.LayerSemiStatic, func(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
		if args == nil || args.Request == nil {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		promptDeps := Deps{
			Agents:                 deps.Agents,
			AgentUC:                deps.AgentUC,
			SessionMemoryAvailable: deps.HasMemory,
			LG:                     deps.Logger(),
			CustomToolKeys:         customKeys,
		}
		cue := DynamicRuntimeCapabilityCue(ctx, promptDeps, ag)
		if cue == "" {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		// 上下文预算台账（29-token §9.6）：仅计量，不改注入逻辑。
		recordContextBudgetOnce(ctx, ContextBudgetCategoryOtherDynamic, utf8.RuneCountInString(cue))
		// Prefix stabilization (WP-1): the cue content changes when MCP tools
		// reconnect or tool config flips mid-session, so it must append at the
		// END of the message list (never insertAfterLastSystem) — otherwise the
		// whole [system + history] prefix is invalidated on every change.
		sys := trpcmodel.NewSystemMessage(cue)
		args.Request.Messages = append(args.Request.Messages, sys)
		return &trpcmodel.BeforeModelResult{Context: ctx}, nil
	})
}

// customToolKeysFromDeps extracts tool declaration names from CustomTools
// so the Runtime Cue can include them in the effective tool key list.
func customToolKeysFromDeps(deps TRPCBuilderDeps) []string {
	if len(deps.CustomTools) == 0 {
		return nil
	}
	keys := make([]string, 0, len(deps.CustomTools))
	for _, t := range deps.CustomTools {
		if d := t.Declaration(); d != nil && d.Name != "" {
			keys = append(keys, d.Name)
		}
	}
	if len(keys) == 0 {
		return nil
	}
	return keys
}
