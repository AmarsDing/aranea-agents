package evaluation

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpceval "trpc.group/trpc-go/trpc-agent-go/evaluation"
	evalresultinmemory "trpc.group/trpc-go/trpc-agent-go/evaluation/evalresult/inmemory"
	evalsetinmemory "trpc.group/trpc-go/trpc-agent-go/evaluation/evalset/inmemory"
	"trpc.group/trpc-go/trpc-agent-go/evaluation/evaluator/registry"
	metricinmemory "trpc.group/trpc-go/trpc-agent-go/evaluation/metric/inmemory"
	"trpc.group/trpc-go/trpc-agent-go/evaluation/status"
	"trpc.group/trpc-go/trpc-agent-go/evaluation/usersimulation"
	"trpc.group/trpc-go/trpc-agent-go/runner"
)

// FrameworkBridge runs evaluations via trpc-agent-go AgentEvaluator (MultiRun, etc.).
type FrameworkBridge struct {
	runFactory   func(agentID string) (runner.Runner, error)
	llmJudge     LLMJudge
	llmUserSim   usersimulation.Simulator
	multiRunCfg  MultiRunConfig
	lg           loggateway.Logger
}

func NewFrameworkBridge(
	runFactory func(agentID string) (runner.Runner, error),
	judge LLMJudge,
	llmUserSim usersimulation.Simulator,
	multiRunCfg MultiRunConfig,
	lg loggateway.Logger,
) *FrameworkBridge {
	return &FrameworkBridge{runFactory: runFactory, llmJudge: judge, llmUserSim: llmUserSim, multiRunCfg: multiRunCfg, lg: lg}
}

// RunConfig holds per-run framework options.
type RunConfig struct {
	AgentID           string
	NumRuns           int
	Metrics           map[string]bool
	UseUserSimulation bool
}

// Execute runs AgentEvaluator and returns biz case results + aggregate scores.
func (b *FrameworkBridge) Execute(
	ctx context.Context,
	dataset biz.EvalDataset,
	cases []biz.EvalCase,
	cfg RunConfig,
) ([]biz.EvalCaseResult, map[string]float32, float32, float32, error) {
	if b == nil || b.runFactory == nil {
		return nil, nil, 0, 0, fmt.Errorf("framework bridge not configured")
	}
	numRuns := cfg.NumRuns
	if numRuns <= 0 {
		numRuns = 1
	}
	mrc := b.multiRunCfg
	if mrc.NumRuns <= 0 {
		mrc.NumRuns = numRuns
	}
	run, err := b.runFactory(cfg.AgentID)
	if err != nil {
		return nil, nil, 0, 0, err
	}
	defer run.Close()

	evalSetMgr := evalsetinmemory.New()
	metricMgr := metricinmemory.New()
	resultMgr := evalresultinmemory.New()
	reg := registry.New()

	evalSetID := dataset.ID
	if _, err := evalSetMgr.Create(ctx, AppName, evalSetID); err != nil {
		return nil, nil, 0, 0, fmt.Errorf("create eval set: %w", err)
	}
	es := BizCasesToEvalSet(dataset, cases)
	for _, c := range es.EvalCases {
		if err := evalSetMgr.AddCase(ctx, AppName, evalSetID, c); err != nil {
			return nil, nil, 0, 0, fmt.Errorf("add eval case: %w", err)
		}
	}
	if err := registerFrameworkMetrics(ctx, metricMgr, evalSetID, metricSet(cfg.Metrics)); err != nil {
		return nil, nil, 0, 0, err
	}

	opts := []trpceval.Option{
		trpceval.WithEvalSetManager(evalSetMgr),
		trpceval.WithMetricManager(metricMgr),
		trpceval.WithEvalResultManager(resultMgr),
		trpceval.WithRegistry(reg),
	}
	opts = append(opts, mrc.ToOptions()...)
	if sim := resolveUserSimulator(cases, cfg, b.llmUserSim); sim != nil {
		opts = append(opts, trpceval.WithUserSimulator(sim))
	}

	evaluator, err := trpceval.New(
		AppName,
		run,
		opts...,
	)
	if err != nil {
		return nil, nil, 0, 0, fmt.Errorf("create agent evaluator: %w", err)
	}
	defer evaluator.Close()

	result, err := evaluator.Evaluate(ctx, evalSetID)
	if err != nil {
		return nil, nil, 0, 0, fmt.Errorf("framework evaluate: %w", err)
	}
	passAtK, passHatK := computePassMetrics(result, numRuns)

	caseByID := make(map[string]biz.EvalCase, len(cases))
	for _, c := range cases {
		caseByID[c.ID] = c
	}

	out := make([]biz.EvalCaseResult, 0, len(result.EvalCases))
	agg := map[string]float32{}
	aggCount := map[string]float32{}

	for _, cr := range result.EvalCases {
		bc, ok := caseByID[cr.EvalCaseID]
		if !ok {
			continue
		}
		res := biz.EvalCaseResult{
			ID:         newEvalResultID(),
			CaseID:     bc.ID,
			ScoresJSON: "{}",
		}
		if len(cr.RunDetails) > 0 {
			rd := cr.RunDetails[len(cr.RunDetails)-1]
			if rd != nil && rd.Inference != nil && len(rd.Inference.Inferences) > 0 {
				inv := rd.Inference.Inferences[0]
				if inv.FinalResponse != nil {
					res.ActualOutput = inv.FinalResponse.Content
				}
				if rd.Inference.ErrorMessage != "" {
					res.ErrorMessage = rd.Inference.ErrorMessage
				}
			}
		}
		if len(cr.EvalCaseResults) > 0 {
			last := cr.EvalCaseResults[len(cr.EvalCaseResults)-1]
			if last != nil && last.FinalEvalStatus == status.EvalStatusFailed && res.ErrorMessage == "" {
				res.ErrorMessage = last.ErrorMessage
			}
		}
		for _, mr := range cr.MetricResults {
			if mr == nil {
				continue
			}
			name := strings.TrimSpace(mr.MetricName)
			score := float32(mr.Score)
			agg[name] += score
			aggCount[name]++
			applyMetricResult(&res, name, score, mr.Threshold)
		}
		if cfg.Metrics[MetricLLMAsJudge] && b.llmJudge != nil && res.ActualOutput != "" {
			score, judgeErr := b.llmJudge(ctx, bc.Input, bc.ExpectedOutput, res.ActualOutput)
			if judgeErr != nil {
				// EV-04: log failure and count as 0 in denominator so the average is
				// not inflated by skipping failed judge calls. Append to any pre-existing
				// inference error to preserve both error contexts.
				b.lg.Warn("eval.llm_judge.failed",
					loggateway.StepID("system.auto_memory.extract_fail"),
					loggateway.Str("case_id", bc.ID),
					loggateway.Err(judgeErr),
				)
				judgeErrMsg := "llm_judge failed: " + judgeErr.Error()
				if res.ErrorMessage != "" {
					res.ErrorMessage = res.ErrorMessage + "; " + judgeErrMsg
				} else {
					res.ErrorMessage = judgeErrMsg
				}
				aggCount[MetricLLMAsJudge]++
			} else {
				res.LLMJudgeScore = score
				agg[MetricLLMAsJudge] += score
				aggCount[MetricLLMAsJudge]++
			}
		}
		out = append(out, res)
	}

	scores := map[string]float32{}
	for name, sum := range agg {
		if aggCount[name] > 0 {
			scores[name] = sum / aggCount[name]
		}
	}
	return out, scores, passAtK, passHatK, nil
}
