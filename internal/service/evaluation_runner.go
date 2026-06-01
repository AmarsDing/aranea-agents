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

// NewEvaluationRunner wires AgentRunner + LLMJudge for evaluation runs (EP-RT-08).
func NewEvaluationRunner(
	uc *biz.EvalUsecase,
	turns EvalTurnGateway,
	catalog *biz.LlmProviderModelUsecase,
	sys biz.SystemSettingRepo,
) *evaluation.Runner {
	rt := &provider.RoundTrip{HTTP: &http.Client{Timeout: 120 * time.Second}}
	agentRunner := func(ctx context.Context, agentID, input string) (string, error) {
		if turns == nil {
			return "", nil
		}
		return turns.RunEvalAgentTurn(ctx, agentID, input)
	}
	judge := evaluation.NewLLMJudge(catalog, rt, sys, loggateway.Global())
	runFactory := func(agentID string) (runner.Runner, error) {
		return evaluation.NewChatRunnerAdapter(agentID, agentRunner), nil
	}
	var llmUserSim usersimulation.Simulator
	if sim, err := evaluation.NewLLMUserSimulator(catalog, rt, sys, loggateway.Global()); err == nil {
		llmUserSim = sim
	}
	framework := evaluation.NewFrameworkBridge(runFactory, judge, llmUserSim, evaluation.DefaultMultiRunConfig(), loggateway.Global())
	return evaluation.NewRunner(uc, agentRunner, judge, framework, loggateway.Global())
}
