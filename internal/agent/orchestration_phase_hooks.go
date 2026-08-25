package agent

import (
	"context"
	"strings"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/tools/deferred"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	"trpc.group/trpc-go/trpc-agent-go/toolsnapshot"
)

// newOrchestrationPhasePromoteBeforeHook activates deferred closeout tools for
// the current Spirit session phase so they appear in Request.Tools on the first
// LLM call (no tool_load / not-found loop). Idle keeps the WP-4 four-tool set.
func newOrchestrationPhasePromoteBeforeHook(dm *deferred.DeferredToolManager, lg loggateway.Logger) callbacks.Callback {
	if dm == nil {
		return nil
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return callbacks.NewBeforeAgentHook(1, func(ctx context.Context, args *trpcagent.BeforeAgentArgs) (*trpcagent.BeforeAgentResult, error) {
		orch, ok := biz.SpiritTurnOrchestrationFrom(ctx)
		if !ok || orch.Phase == biz.SpiritPhaseIdle || orch.Phase == "" {
			return &trpcagent.BeforeAgentResult{Context: ctx}, nil
		}
		activated := 0
		for _, name := range biz.PhasePromotedToolNames(orch.Phase) {
			if !dm.IsInCatalog(name) {
				continue
			}
			if dm.IsActivated(ctx, name) {
				activated++
				continue
			}
			if _, err := dm.Activate(ctx, name); err != nil {
				lg.Warn("编排阶段提升延迟工具失败",
					loggateway.StepID("agent.orch_phase.promote"),
					loggateway.Str("tool", name),
					loggateway.Str("phase", string(orch.Phase)),
					loggateway.Err(err),
				)
				continue
			}
			activated++
		}
		if activated > 0 {
			toolsnapshot.InvalidateFromContext(ctx)
		}
		return &trpcagent.BeforeAgentResult{Context: ctx}, nil
	})
}

// newOrchestrationBriefBeforeHook appends the session-phase brief as a trailing
// dynamic cue so Ready/Orchestrating turns can answer from facts without
// loading deliverable tools first.
func newOrchestrationBriefBeforeHook() callbacks.Callback {
	return callbacks.NewBeforeModelHook(5, callbacks.LayerDynamic, func(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
		if args == nil || args.Request == nil {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		orch, ok := biz.SpiritTurnOrchestrationFrom(ctx)
		if !ok {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		brief := strings.TrimSpace(orch.Brief)
		if brief == "" {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		if orchestrationBriefAlreadyPresent(args.Request.Messages, brief) {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		args.Request.Messages = appendDynamicCue(args.Request.Messages, orchBriefCueMarker+brief)
		return &trpcmodel.BeforeModelResult{Context: ctx}, nil
	})
}

func orchestrationBriefAlreadyPresent(msgs []trpcmodel.Message, brief string) bool {
	for _, m := range msgs {
		if strings.Contains(m.Content, "## Session orchestration") || strings.Contains(m.Content, brief) {
			return true
		}
	}
	return false
}
