package evaluation

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"

	trpceval "trpc.group/trpc-go/trpc-agent-go/evaluation"
	evalresultinmemory "trpc.group/trpc-go/trpc-agent-go/evaluation/evalresult/inmemory"
	evalsetinmemory "trpc.group/trpc-go/trpc-agent-go/evaluation/evalset/inmemory"
	"trpc.group/trpc-go/trpc-agent-go/evaluation/evaluator/registry"
	"trpc.group/trpc-go/trpc-agent-go/evaluation/metric"
	"trpc.group/trpc-go/trpc-agent-go/evaluation/metric/criterion"
	"trpc.group/trpc-go/trpc-agent-go/evaluation/metric/criterion/finalresponse"
	"trpc.group/trpc-go/trpc-agent-go/evaluation/metric/criterion/text"
	"trpc.group/trpc-go/trpc-agent-go/evaluation/metric/criterion/tooltrajectory"
	metricinmemory "trpc.group/trpc-go/trpc-agent-go/evaluation/metric/inmemory"
	"trpc.group/trpc-go/trpc-agent-go/evaluation/status"
	"trpc.group/trpc-go/trpc-agent-go/runner"
)

// FrameworkBridge runs evaluations via trpc-agent-go AgentEvaluator (MultiRun, etc.).
type FrameworkBridge struct {
	runFactory func(agentID string) (runner.Runner, error)
	llmJudge   LLMJudge
}

// NewFrameworkBridge constructs a FrameworkBridge.
func NewFrameworkBridge(runFactory func(agentID string) (runner.Runner, error), judge LLMJudge) *FrameworkBridge {
	return &FrameworkBridge{runFactory: runFactory, llmJudge: judge}
}

// RunConfig holds per-run framework options.
type RunConfig struct {
	AgentID  string
	NumRuns  int
	Metrics  map[string]bool
}

// Execute runs AgentEvaluator and returns biz case results + aggregate scores.
func (b *FrameworkBridge) Execute(
	ctx context.Context,
	dataset biz.EvalDataset,
	cases []biz.EvalCase,
	cfg RunConfig,
) ([]biz.EvalCaseResult, map[string]float32, error) {
	if b == nil || b.runFactory == nil {
		return nil, nil, fmt.Errorf("framework bridge not configured")
	}
	numRuns := cfg.NumRuns
	if numRuns <= 0 {
		numRuns = 1
	}
	run, err := b.runFactory(cfg.AgentID)
	if err != nil {
		return nil, nil, err
	}
	defer run.Close()

	evalSetMgr := evalsetinmemory.New()
	metricMgr := metricinmemory.New()
	resultMgr := evalresultinmemory.New()
	reg := registry.New()

	evalSetID := dataset.ID
	if _, err := evalSetMgr.Create(ctx, AppName, evalSetID); err != nil {
		return nil, nil, fmt.Errorf("create eval set: %w", err)
	}
	es := BizCasesToEvalSet(dataset, cases)
	for _, c := range es.EvalCases {
		if err := evalSetMgr.AddCase(ctx, AppName, evalSetID, c); err != nil {
			return nil, nil, fmt.Errorf("add eval case: %w", err)
		}
	}
	if err := registerFrameworkMetrics(ctx, metricMgr, evalSetID, cfg.Metrics); err != nil {
		return nil, nil, err
	}

	evaluator, err := trpceval.New(
		AppName,
		run,
		trpceval.WithEvalSetManager(evalSetMgr),
		trpceval.WithMetricManager(metricMgr),
		trpceval.WithEvalResultManager(resultMgr),
		trpceval.WithRegistry(reg),
		trpceval.WithNumRuns(numRuns),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("create agent evaluator: %w", err)
	}
	defer evaluator.Close()

	result, err := evaluator.Evaluate(ctx, evalSetID, trpceval.WithRunDetailsEnabled(true))
	if err != nil {
		return nil, nil, fmt.Errorf("framework evaluate: %w", err)
	}

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
			ID:     newEvalResultID(),
			CaseID: bc.ID,
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
			switch name {
			case MetricExactMatch:
				res.ExactMatch = score >= float32(mr.Threshold)
			case MetricContainsMatch:
				res.ContainsMatch = score >= float32(mr.Threshold)
			case MetricToolCallAccuracy:
				res.ToolCallAccuracy = score
			}
		}
		if cfg.Metrics[MetricLLMAsJudge] && b.llmJudge != nil && res.ActualOutput != "" {
			score, judgeErr := b.llmJudge(ctx, bc.Input, bc.ExpectedOutput, res.ActualOutput)
			if judgeErr == nil {
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
	return out, scores, nil
}

func registerFrameworkMetrics(ctx context.Context, mgr metric.Manager, evalSetID string, want map[string]bool) error {
	type spec struct {
		name      string
		threshold float64
		crit      *criterion.Criterion
	}
	specs := []spec{}
	if want[MetricExactMatch] {
		specs = append(specs, spec{
			name:      MetricExactMatch,
			threshold: 1.0,
			crit: criterion.New(criterion.WithFinalResponse(finalresponse.New(
				finalresponse.WithTextCriterion(&text.TextCriterion{MatchStrategy: text.TextMatchStrategyExact}),
			))),
		})
	}
	if want[MetricContainsMatch] {
		specs = append(specs, spec{
			name:      MetricContainsMatch,
			threshold: 1.0,
			crit: criterion.New(criterion.WithFinalResponse(finalresponse.New(
				finalresponse.WithTextCriterion(&text.TextCriterion{MatchStrategy: text.TextMatchStrategyContains}),
			))),
		})
	}
	if want[MetricToolCallAccuracy] {
		specs = append(specs, spec{
			name:      MetricToolCallAccuracy,
			threshold: 1.0,
			crit:      criterion.New(criterion.WithToolTrajectory(tooltrajectory.New())),
		})
	}
	if len(specs) == 0 {
		specs = append(specs, spec{
			name:      MetricExactMatch,
			threshold: 1.0,
			crit: criterion.New(criterion.WithFinalResponse(finalresponse.New(
				finalresponse.WithTextCriterion(&text.TextCriterion{MatchStrategy: text.TextMatchStrategyExact}),
			))),
		})
	}
	for _, s := range specs {
		if err := mgr.Add(ctx, AppName, evalSetID, &metric.EvalMetric{
			MetricName: s.name,
			Threshold:  s.threshold,
			Criterion:  s.crit,
		}); err != nil {
			return fmt.Errorf("register metric %s: %w", s.name, err)
		}
	}
	return nil
}
