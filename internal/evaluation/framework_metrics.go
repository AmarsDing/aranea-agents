package evaluation

import (
	"context"
	"fmt"

	"trpc.group/trpc-go/trpc-agent-go/evaluation/metric"
	"trpc.group/trpc-go/trpc-agent-go/evaluation/metric/criterion"
	"trpc.group/trpc-go/trpc-agent-go/evaluation/metric/criterion/finalresponse"
	cjson "trpc.group/trpc-go/trpc-agent-go/evaluation/metric/criterion/json"
	crouge "trpc.group/trpc-go/trpc-agent-go/evaluation/metric/criterion/rouge"
	"trpc.group/trpc-go/trpc-agent-go/evaluation/metric/criterion/text"
	"trpc.group/trpc-go/trpc-agent-go/evaluation/metric/criterion/tooltrajectory"
	cxml "trpc.group/trpc-go/trpc-agent-go/evaluation/metric/criterion/xml"
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
			MetricName: s.name,
			Threshold:  s.threshold,
			Criterion:  s.crit,
		}); err != nil {
			return fmt.Errorf("register metric %s: %w", s.name, err)
		}
	}
	return nil
}

type metricSpec struct {
	name      string
	threshold float64
	crit      *criterion.Criterion
}

func buildMetricSpecs(want metricSet) []metricSpec {
	var specs []metricSpec
	add := func(name string, threshold float64, crit *criterion.Criterion) {
		if want[name] {
			specs = append(specs, metricSpec{name: name, threshold: threshold, crit: crit})
		}
	}
	add(MetricExactMatch, 1.0, criterion.New(criterion.WithFinalResponse(finalresponse.New(
		finalresponse.WithTextCriterion(&text.TextCriterion{MatchStrategy: text.TextMatchStrategyExact}),
	))))
	add(MetricContainsMatch, 1.0, criterion.New(criterion.WithFinalResponse(finalresponse.New(
		finalresponse.WithTextCriterion(&text.TextCriterion{MatchStrategy: text.TextMatchStrategyContains}),
	))))
	add(MetricJSONMatch, 1.0, criterion.New(criterion.WithFinalResponse(finalresponse.New(
		finalresponse.WithJSONCriterion(cjson.New()),
	))))
	add(MetricXMLMatch, 1.0, criterion.New(criterion.WithFinalResponse(finalresponse.New(
		finalresponse.WithXMLCriterion(cxml.New()),
	))))
	add(MetricRougeL, 0.5, criterion.New(criterion.WithFinalResponse(finalresponse.New(
		finalresponse.WithRougeCriterion(crouge.New(crouge.WithRougeType("rougeL"))),
	))))
	add(MetricToolTrajectory, 1.0, criterion.New(criterion.WithToolTrajectory(
		tooltrajectory.New(tooltrajectory.WithOrderSensitive(true)),
	)))
	add(MetricToolCallAccuracy, 1.0, criterion.New(criterion.WithToolTrajectory(tooltrajectory.New())))
	return specs
}
