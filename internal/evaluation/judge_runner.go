package evaluation

import (
	"context"
	"fmt"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/provider"
	"aranea-agents/pkg/loggateway"

	"trpc.group/trpc-go/trpc-agent-go/agent"
	"trpc.group/trpc-go/trpc-agent-go/agent/llmagent"
	"trpc.group/trpc-go/trpc-agent-go/event"
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
	// evalLLMCallTimeout bounds one judge / user-simulator LLM invocation.
	// Without it a hung provider connection stalled the whole run in
	// "running" forever (one judge call per case, sequentially).
	evalLLMCallTimeout = 2 * time.Minute
)

// timeoutRunner decorates a runner.Runner with a per-Run deadline (Y5). The
// judge and user-simulator make non-streaming LLM calls whose provider
// RoundTrip has no hard guarantee of returning; a wedged connection must fail
// the case instead of hanging the run.
type timeoutRunner struct {
	runner.Runner
	timeout time.Duration
}

func (t *timeoutRunner) Run(
	ctx context.Context,
	userID, sessionID string,
	message trpcmodel.Message,
	runOpts ...agent.RunOption,
) (<-chan *event.Event, error) {
	ctx, cancel := context.WithTimeout(ctx, t.timeout)
	ch, err := t.Runner.Run(ctx, userID, sessionID, message, runOpts...)
	if err != nil {
		cancel()
		return nil, err
	}
	// Forward events so cancel fires exactly when the upstream stream ends
	// (or the deadline hits and the framework stops reading).
	out := make(chan *event.Event, 16)
	go func() {
		defer cancel()
		defer close(out)
		for ev := range ch {
			select {
			case out <- ev:
			case <-ctx.Done():
				// Keep draining until upstream observes cancellation and
				// closes ch; abandoning the read here could leak the
				// upstream producer goroutine.
				go func() {
					for range ch {
					}
				}()
				return
			}
		}
	}()
	return out, nil
}

// NewJudgeRunner creates a runner.Runner backed by the project's LLM catalog for
// use with the framework's WithJudgeRunner option. The runner wraps an LLM agent
// that produces structured {score, reason} output compatible with the framework's
// llm_final_response evaluator.
//
// Model resolution precedence:
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
	return &timeoutRunner{Runner: runner.NewRunner(AppName+"-judge", agent), timeout: evalLLMCallTimeout}, nil
}
