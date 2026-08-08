package evaluation

import (
	"context"
	"fmt"

	evalfinalresponse "trpc.group/trpc-go/trpc-agent-go/evaluation/evaluator/finalresponse"
	llmrubricresponse "trpc.group/trpc-go/trpc-agent-go/evaluation/evaluator/llm/rubricresponse"
	evaltooltrajectory "trpc.group/trpc-go/trpc-agent-go/evaluation/evaluator/tooltrajectory"
	"trpc.group/trpc-go/trpc-agent-go/evaluation/metric"
	"trpc.group/trpc-go/trpc-agent-go/evaluation/metric/criterion"
	"trpc.group/trpc-go/trpc-agent-go/evaluation/metric/criterion/finalresponse"
	cjson "trpc.group/trpc-go/trpc-agent-go/evaluation/metric/criterion/json"
	criterionllm "trpc.group/trpc-go/trpc-agent-go/evaluation/metric/criterion/llm"
	crouge "trpc.group/trpc-go/trpc-agent-go/evaluation/metric/criterion/rouge"
	"trpc.group/trpc-go/trpc-agent-go/evaluation/metric/criterion/text"
	"trpc.group/trpc-go/trpc-agent-go/evaluation/metric/criterion/tooltrajectory"
	cxml "trpc.group/trpc-go/trpc-agent-go/evaluation/metric/criterion/xml"
)

// Framework evaluator names, resolved from the evaluators themselves.
// A metric whose EvaluatorName does not resolve in the framework registry is
// SILENTLY SKIPPED (local.go treats os.ErrNotExist as "skip"), which used to
// zero out every deterministic metric.
var (
	finalResponseEvaluatorName  = evalfinalresponse.New().Name()
	toolTrajectoryEvaluatorName = evaltooltrajectory.New().Name()
	rubricResponseEvaluatorName = llmrubricresponse.New().Name()
)

const (
	MetricJSONMatch      = "json_match"
	MetricXMLMatch       = "xml_match"
	MetricRougeL         = "rouge_l"
	MetricToolTrajectory = "tool_trajectory"
)

// AllFrameworkMetrics lists every supported framework metric key.
func AllFrameworkMetrics() []string {
	return append(FrameworkMetricNames(),
		MetricJSONMatch, MetricXMLMatch, MetricRougeL, MetricToolTrajectory)
}

func registerFrameworkMetrics(ctx context.Context, mgr metric.Manager, evalSetID string, want metricSet) error {
	specs := buildMetricSpecs(want)
	if len(specs) == 0 {
		want = allMetrics()
		specs = buildMetricSpecs(want)
	}
	for _, s := range specs {
		if err := mgr.Add(ctx, AppName, evalSetID, &metric.EvalMetric{
			MetricName:    s.name,
			Threshold:     s.threshold,
			Criterion:     s.crit,
			EvaluatorName: s.evaluatorName,
		}); err != nil {
			return fmt.Errorf("register metric %s: %w", s.name, err)
		}
	}
	return nil
}

type metricSpec struct {
	name          string
	threshold     float64
	crit          *criterion.Criterion
	evaluatorName string
}

func buildMetricSpecs(want metricSet) []metricSpec {
	var specs []metricSpec
	add := func(name string, threshold float64, crit *criterion.Criterion, evaluatorName string) {
		if want[name] {
			specs = append(specs, metricSpec{name: name, threshold: threshold, crit: crit, evaluatorName: evaluatorName})
		}
	}
	add(MetricExactMatch, 1.0, criterion.New(criterion.WithFinalResponse(finalresponse.New(
		finalresponse.WithTextCriterion(&text.TextCriterion{MatchStrategy: text.TextMatchStrategyExact}),
	))), finalResponseEvaluatorName)
	add(MetricContainsMatch, 1.0, criterion.New(criterion.WithFinalResponse(finalresponse.New(
		finalresponse.WithTextCriterion(&text.TextCriterion{MatchStrategy: text.TextMatchStrategyContains}),
	))), finalResponseEvaluatorName)
	add(MetricJSONMatch, 1.0, criterion.New(criterion.WithFinalResponse(finalresponse.New(
		finalresponse.WithJSONCriterion(cjson.New()),
	))), finalResponseEvaluatorName)
	add(MetricXMLMatch, 1.0, criterion.New(criterion.WithFinalResponse(finalresponse.New(
		finalresponse.WithXMLCriterion(cxml.New()),
	))), finalResponseEvaluatorName)
	add(MetricRougeL, 0.5, criterion.New(criterion.WithFinalResponse(finalresponse.New(
		finalresponse.WithRougeCriterion(crouge.New(crouge.WithRougeType("rougeL"))),
	))), finalResponseEvaluatorName)
	add(MetricToolTrajectory, 1.0, criterion.New(criterion.WithToolTrajectory(
		tooltrajectory.New(tooltrajectory.WithOrderSensitive(true)),
	)), toolTrajectoryEvaluatorName)
	add(MetricToolCallAccuracy, 1.0, criterion.New(criterion.WithToolTrajectory(tooltrajectory.New())), toolTrajectoryEvaluatorName)
	// P1-7/P3-2: llm_as_judge uses the framework's llm_rubric_response evaluator.
	// Its prompt renders the case's rubrics (merged from EvalCase.Rubrics by the
	// service layer) and asks the judge for per-rubric JSON scores, so judge
	// reasons always reflect the case's scoring standard. Cases without a
	// custom rubric get a synthesized default in the evalset adapter.
	add(MetricLLMAsJudge, 0.5, criterion.New(criterion.WithLLMJudge(&criterionllm.LLMCriterion{})), rubricResponseEvaluatorName)
	return specs
}
