package agent

import (
	"context"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/agent/reposkills"
	"aranea-agents/internal/biz"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// workspaceSkillsCueMemoStateKey memoizes the repo-FS skill cue per invocation.
const workspaceSkillsCueMemoStateKey = "aranea.workspace_skills_cue_memo"

func newWorkspaceSkillsBeforeHook(ag biz.Agent, deps TRPCBuilderDeps) callbacks.Callback {
	if !ShouldAttachWorkingContract(ag) {
		return nil
	}
	return callbacks.NewBeforeModelHook(5, callbacks.LayerDynamic, func(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
		if args == nil || args.Request == nil {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		cue := workspaceSkillsCueFromCtx(ctx, ag, deps)
		if cue == "" {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		args.Request.Messages = replaceDynamicCue(args.Request.Messages, workspaceSkillsCueMarker, workspaceSkillsCueMarker+cue)
		return &trpcmodel.BeforeModelResult{Context: ctx}, nil
	})
}

func workspaceSkillsCueFromCtx(ctx context.Context, ag biz.Agent, deps TRPCBuilderDeps) string {
	inv, ok := trpcagent.InvocationFromContext(ctx)
	if ok && inv != nil {
		if cached, hit := inv.GetState(workspaceSkillsCueMemoStateKey); hit {
			if s, isStr := cached.(string); isStr {
				return s
			}
		}
	}
	cwd, err := resolveToolWorkspaceRoot(ctx, ag, deps, "")
	if err != nil || cwd == "" {
		return ""
	}
	cue := reposkills.FormatCue(reposkills.Scan(cwd, []string{cwd}))
	if ok && inv != nil {
		inv.SetState(workspaceSkillsCueMemoStateKey, cue)
	}
	return cue
}
