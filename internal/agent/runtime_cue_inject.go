package agent

import (
	"context"
	"strings"
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
			// B3：static hook 每轮同样先算 cue 再查重，复用缓存避免 DB。
			CachedEffectiveTools: deps.CachedEffectiveTools,
		}
		cue := StaticRuntimeCapabilityCue(ctx, promptDeps, ag)
		if cue == "" {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		// Production bakes this cue into WithInstruction. Skip a second system
		// message when the instruction already carries it so DeepSeek sees one
		// contiguous system prefix. Tests that use a fake base system still
		// insert after the existing system block.
		if staticRuntimeCueAlreadyPresent(args.Request.Messages) {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
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
			// B3：复用 BUILD 期预取的 effective tools，工具循环每轮省 ≈4 次 DB。
			CachedEffectiveTools: deps.CachedEffectiveTools,
		}
		cue := DynamicRuntimeCapabilityCue(ctx, promptDeps, ag)
		if cue == "" {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		// 上下文预算台账（29-token §9.6）：仅计量，不改注入逻辑。
		recordContextBudgetOnce(ctx, ContextBudgetCategoryOtherDynamic, utf8.RuneCountInString(cue))
		// Prefix stabilization (WP-1): content changes when MCP tools reconnect
		// or tool config flips mid-session. Append as a user-role cue at the
		// END so it stays out of DeepSeek's system prefix.
		args.Request.Messages = appendDynamicCue(args.Request.Messages, cue)
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

func staticRuntimeCueAlreadyPresent(msgs []trpcmodel.Message) bool {
	for _, m := range msgs {
		if m.Role == trpcmodel.RoleSystem && strings.Contains(m.Content, "## Runtime capability policy") {
			return true
		}
	}
	return false
}
