package evaluation

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpceval "trpc.group/trpc-go/trpc-agent-go/evaluation"
	evalresultinmemory "trpc.group/trpc-go/trpc-agent-go/evaluation/evalresult/inmemory"
	evalsetinmemory "trpc.group/trpc-go/trpc-agent-go/evaluation/evalset/inmemory"
	"trpc.group/trpc-go/trpc-agent-go/evaluation/evaluator/registry"
	metricinmemory "trpc.group/trpc-go/trpc-agent-go/evaluation/metric/inmemory"
	"trpc.group/trpc-go/trpc-agent-go/evaluation/service"
	"trpc.group/trpc-go/trpc-agent-go/evaluation/status"
	"trpc.group/trpc-go/trpc-agent-go/evaluation/usersimulation"
	"trpc.group/trpc-go/trpc-agent-go/runner"
)

// frameworkCaseErrPrefix matches the framework's per-case error wrapper
// "inference eval case (evalCaseID=X, sessionID=Y): ". The IDs are unique per
// case execution, so persisting the wrapper verbatim would defeat P2-3 SQL
// failure grouping (GROUP BY error_message) — every case would form its own
// group. The IDs stay available in structured fields/logs.
var frameworkCaseErrPrefix = regexp.MustCompile(`^inference eval case \(evalCaseID=[^,]+, sessionID=[^)]+\):\s*`)

// normalizeCaseErrorMessage strips per-case unique-ID wrappers so identical
// failures cluster into one failure group.
func normalizeCaseErrorMessage(msg string) string {
	return frameworkCaseErrPrefix.ReplaceAllString(msg, "")
}

// FrameworkBridge runs evaluations via trpc-agent-go AgentEvaluator (MultiRun, etc.).
type FrameworkBridge struct {
	runFactory  func(agentID string) (runner.Runner, error)
	judgeRunner runner.Runner
	callbacks   *service.Callbacks
	llmUserSim  usersimulation.Simulator
	multiRunCfg MultiRunConfig
	lg          loggateway.Logger
}

func NewFrameworkBridge(
	runFactory func(agentID string) (runner.Runner, error),
	judgeRunner runner.Runner,
	callbacks *service.Callbacks,
	llmUserSim usersimulation.Simulator,
	multiRunCfg MultiRunConfig,
	lg loggateway.Logger,
) *FrameworkBridge {
	return &FrameworkBridge{
		runFactory:  runFactory,
		judgeRunner: judgeRunner,
		callbacks:   callbacks,
		llmUserSim:  llmUserSim,
		multiRunCfg: multiRunCfg,
		lg:          lg,
	}
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
	// The bridge default (DefaultMultiRunConfig) sets NumRuns=1, so the old
	// "only override when <= 0" guard silently dropped every API-provided
	// num_runs > 1 — MultiRun never activated and pass@k stayed 0. The
	// per-request value always wins over the bridge default.
	mrc.NumRuns = numRuns
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
	es := BizCasesToEvalSet(dataset, cases, b.lg)
	// Case-level rubrics target the llm_as_judge metric instance; the framework
	// fails a case whose rubric references an unregistered metric, so strip
	// them when this run does not compute llm_as_judge.
	stripCaseRubricsWhenNoJudge(es, cfg.Metrics)
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
	if b.judgeRunner != nil {
		opts = append(opts, trpceval.WithJudgeRunner(b.judgeRunner))
	}
	if b.callbacks != nil {
		opts = append(opts, trpceval.WithCallbacks(b.callbacks))
	}
	if sim := resolveUserSimulator(cases, cfg, b.llmUserSim, b.lg); sim != nil {
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
					if strings.TrimSpace(res.ActualOutput) == "" && res.ErrorMessage == "" {
						res.ErrorMessage = "empty actual output"
					}
				}
				if rd.Inference.ErrorMessage != "" {
					res.ErrorMessage = normalizeCaseErrorMessage(rd.Inference.ErrorMessage)
				}
				applyRAGMetrics(ctx, &res, inv, bc.Input, b.judgeRunner)
				if inv.ExecutionTrace != nil {
					res.SessionID = inv.ExecutionTrace.SessionID
					res.TraceRunID = inv.ExecutionTrace.RootInvocationID
				}
				if res.SessionID == "" {
					res.SessionID = inv.InvocationID
				}
			}
		}
		if len(cr.EvalCaseResults) > 0 {
			last := cr.EvalCaseResults[len(cr.EvalCaseResults)-1]
			if last != nil && last.FinalEvalStatus == status.EvalStatusFailed && res.ErrorMessage == "" {
				res.ErrorMessage = normalizeCaseErrorMessage(last.ErrorMessage)
			}
		}
		// K6: surface the judge's rubric verdict in process logs so rubric
		// calibration is observable at runtime (the reason is not persisted).
		// The framework's aggregateCaseRuns drops Details when averaging scores
		// into cr.MetricResults, so the reason is read from per-run results.
		if reason := judgeReasonFromRuns(cr); reason != "" {
			rs := []rune(reason)
			if len(rs) > 500 {
				reason = string(rs[:500]) + "..."
			}
			b.lg.Info("eval judge verdict",
				loggateway.Str("case_id", bc.ID),
				loggateway.Str("reason", reason))
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

// judgeReasonFromRuns extracts the llm_as_judge verdict reason from per-run
// metric results. The framework's aggregateCaseRuns averages scores into
// EvaluationCaseResult.MetricResults but drops Details, so the judge reason
// only survives on each run's OverallEvalMetricResults.
func judgeReasonFromRuns(cr *trpceval.EvaluationCaseResult) string {
	if cr == nil {
		return ""
	}
	for _, run := range cr.EvalCaseResults {
		if run == nil {
			continue
		}
		for _, mr := range run.OverallEvalMetricResults {
			if mr == nil || mr.MetricName != MetricLLMAsJudge || mr.Details == nil {
				continue
			}
			if reason := strings.TrimSpace(mr.Details.Reason); reason != "" {
				return reason
			}
		}
	}
	return ""
}
