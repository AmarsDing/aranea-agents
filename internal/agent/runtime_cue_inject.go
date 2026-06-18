package agent

import (
	"context"

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
		sys := trpcmodel.NewSystemMessage(cue)
		args.Request.Messages = append([]trpcmodel.Message{sys}, args.Request.Messages...)
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
		sys := trpcmodel.NewSystemMessage(cue)
		args.Request.Messages = append([]trpcmodel.Message{sys}, args.Request.Messages...)
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
