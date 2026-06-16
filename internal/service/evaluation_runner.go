package service

import (
	"context"
	"net/http"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/evaluation"
	"aranea-agents/internal/provider"
	"aranea-agents/pkg/loggateway"

	"trpc.group/trpc-go/trpc-agent-go/evaluation/usersimulation"
	"trpc.group/trpc-go/trpc-agent-go/runner"
)

// NewEvaluationRunner wires AgentRunner + JudgeRunner for evaluation runs (EP-RT-08).
func NewEvaluationRunner(
	uc *biz.EvalUsecase,
	turns EvalTurnGateway,
	catalog *biz.LlmProviderModelUsecase,
	sys biz.SystemSettingRepo,
	lg loggateway.Logger,
) *evaluation.Runner {
	rt := &provider.RoundTrip{HTTP: &http.Client{Timeout: 120 * time.Second}}
	agentRunner := func(ctx context.Context, agentID, input string) (string, error) {
		if turns == nil {
			return "", nil
		}
		return turns.RunEvalAgentTurn(ctx, agentID, input)
	}
	runFactory := func(agentID string) (runner.Runner, error) {
		return evaluation.NewChatRunnerAdapter(agentID, agentRunner), nil
	}

	// P1-7: Use framework Judge Runner instead of self-built LLMJudge.
	var judgeRunner runner.Runner
	if jr, err := evaluation.NewJudgeRunner(catalog, rt, sys, lg); err == nil {
		judgeRunner = jr
	} else {
		lg.Warn("eval.judge_runner.init_failed, LLM Judge metrics will be unavailable",
			loggateway.StepID("evaluation.judge_runner.init_fail"),
			loggateway.Err(err))
	}

	// P1-8: Enable framework Callbacks for evaluation progress awareness.
	callbacks := evaluation.NewEvalCallbacks(lg)

	var llmUserSim usersimulation.Simulator
	if sim, err := evaluation.NewLLMUserSimulator(catalog, rt, sys, lg); err == nil {
		llmUserSim = sim
	}
	framework := evaluation.NewFrameworkBridge(runFactory, judgeRunner, callbacks, llmUserSim, evaluation.DefaultMultiRunConfig(), lg)
	return evaluation.NewRunner(uc, agentRunner, framework, lg)
}
