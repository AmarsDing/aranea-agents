package evaluation

import (
	"context"
	"fmt"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/provider"
	"aranea-agents/pkg/loggateway"

	"trpc.group/trpc-go/trpc-agent-go/agent/llmagent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	"trpc.group/trpc-go/trpc-agent-go/runner"
)

// The framework evaluator prompts each define their own required output
// format (two-line reasoning/label for llm_final_response, JSON rubricScores
// for llm_rubric_response). The judge's system instruction must stay neutral:
// imposing a format here conflicts with the user-message prompt and causes
// intermittent parse failures (observed: "no final response blocks found").
const judgeSystemInstruction = `You are an expert evaluation judge. Carefully follow the user's evaluation instructions and produce your judgment in EXACTLY the output format the user specifies, with no extra commentary.`

const (
	judgeMaxTokens   = 512
	judgeTemperature = 0.3
)

// NewJudgeRunner creates a runner.Runner backed by the project's LLM catalog for
// use with the framework's WithJudgeRunner option. The runner wraps an LLM agent
// that produces structured {score, reason} output compatible with the framework's
// llm_final_response evaluator.
//
// Model resolution follows the same precedence as NewLLMJudge:
// env KRATOS_EVAL_JUDGE_* → system_settings → env KRATOS_EVAL_SIM_* → catalog mini/flash.
func NewJudgeRunner(
	catalog *biz.LlmProviderModelUsecase,
	rt *provider.RoundTrip,
	sys EvalLLMSettingsReader,
	lg loggateway.Logger,
) (runner.Runner, error) {
	if catalog == nil || rt == nil {
		return nil, fmt.Errorf("judge runner: catalog or round trip nil")
	}
	m, err := resolveJudgeModel(context.Background(), catalog, rt, sys, lg)
	if err != nil {
		return nil, fmt.Errorf("judge runner: resolve model: %w", err)
	}
	agent := llmagent.New(
		"eval-judge",
		llmagent.WithModel(m),
		llmagent.WithInstruction(judgeSystemInstruction),
		llmagent.WithDescription("Aranea evaluation LLM judge"),
		llmagent.WithGenerationConfig(trpcmodel.GenerationConfig{
			MaxTokens:   intPtr(judgeMaxTokens),
			Temperature: floatPtr(judgeTemperature),
			Stream:      false,
		}),
	)
	return runner.NewRunner(AppName+"-judge", agent), nil
}
