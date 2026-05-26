package agent

import (
	"context"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

func newRuntimeCueBeforeHook(ag biz.Agent, deps TRPCBuilderDeps) callbacks.Callback {
	if ag.Settings == nil {
		return nil
	}
	level := capabilityCueLevelForMode(ag.SystemPromptMode)
	if level == cueLevelMinimal && !ag.Settings.ToolsEnabled && !ag.Settings.SubagentsEnabled {
		return nil
	}
	return callbacks.NewBeforeModelHook(4, func(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
		if args == nil || args.Request == nil {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		promptDeps := Deps{
			Agents:              deps.Agents,
			AgentUC:             deps.AgentUC,
			SQLiteSessionMemory: deps.HasMemory,
		}
		cue := RuntimeCapabilityCue(ctx, promptDeps, ag)
		if cue == "" {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		sys := trpcmodel.NewSystemMessage(cue)
		args.Request.Messages = append([]trpcmodel.Message{sys}, args.Request.Messages...)
		return &trpcmodel.BeforeModelResult{Context: ctx}, nil
	})
}
