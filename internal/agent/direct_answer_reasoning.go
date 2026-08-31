package agent

import (
	"context"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// directAnswerThinkingTokens caps reasoning on Spirit/chat direct-answer
// turns (eval: 2k–5k reasoning on S01-style replies; target ≤800).
const directAnswerThinkingTokens = 800

const directAnswerReasoningEffort = "low"

// newDirectAnswerReasoningBudgetBeforeHook limits thinking on conversational
// direct-answer turns. Voice / skip-intent already disable thinking;
// planning and in-flight tool loops keep provider defaults so tool JSON
// is not starved. Specialists are not capped.
func newDirectAnswerReasoningBudgetBeforeHook(ag biz.Agent) callbacks.Callback {
	if !usesConversationalContextBudget(ag) {
		return nil
	}
	return callbacks.NewBeforeModelHook(4, callbacks.LayerDynamic, func(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
		if args == nil || args.Request == nil {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		if VoiceFastPathFromContext(ctx) || ThinkingDisabledFromContext(ctx) {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		if _, ok := ForcePlanningRouteFromCtx(ctx); ok {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		if requestLooksLikeToolLoop(args.Request) {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		effort := directAnswerReasoningEffort
		tok := directAnswerThinkingTokens
		args.Request.GenerationConfig.ReasoningEffort = &effort
		args.Request.GenerationConfig.ThinkingTokens = &tok
		return &trpcmodel.BeforeModelResult{Context: ctx}, nil
	})
}

func requestLooksLikeToolLoop(req *trpcmodel.Request) bool {
	if req == nil {
		return false
	}
	for _, m := range req.Messages {
		if len(m.ToolCalls) > 0 || m.Role == trpcmodel.RoleTool {
			return true
		}
	}
	return false
}
