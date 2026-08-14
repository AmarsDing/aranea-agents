package evaluation

import (
	"fmt"
	"strings"
)

// metricSet holds which metrics to compute for a run.
type metricSet map[string]bool

// allMetrics returns the default metric set (four built-in metrics).
func allMetrics() metricSet {
	return parseMetrics("")
}

// ExtendedMetricNames lists opt-in framework metrics beyond the default four.
func ExtendedMetricNames() []string {
	return []string{MetricJSONMatch, MetricXMLMatch, MetricRougeL, MetricToolTrajectory}
}

// ValidateMetricNames rejects unknown comma-separated metric keys (Y6). An
// empty string means "default four" and is valid. Without this guard a typo
// (e.g. "exact_mach") produced an empty spec list and registerFrameworkMetrics
// silently fell back to allMetrics — the run then reported scores the caller
// never asked for.
func ValidateMetricNames(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	valid := make(map[string]bool, 8)
	for _, n := range AllFrameworkMetrics() {
		valid[n] = true
	}
	var unknown []string
	for _, m := range strings.Split(raw, ",") {
		key := strings.TrimSpace(m)
		if key == "" {
			continue
		}
		if !valid[key] {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) > 0 {
		return fmt.Errorf("unknown metrics: %s (valid: %s)",
			strings.Join(unknown, ","), strings.Join(AllFrameworkMetrics(), ","))
	}
	return nil
}

// An empty string means all four built-in metrics.
func parseMetrics(raw string) metricSet {
	all := metricSet{
		MetricExactMatch:       true,
		MetricContainsMatch:    true,
		MetricLLMAsJudge:       true,
		MetricToolCallAccuracy: true,
	}
	if strings.TrimSpace(raw) == "" {
		return all
	}
	result := make(metricSet)
	for _, m := range strings.Split(raw, ",") {
		key := strings.TrimSpace(m)
		if key != "" {
			result[key] = true
		}
	}
	return result
}
