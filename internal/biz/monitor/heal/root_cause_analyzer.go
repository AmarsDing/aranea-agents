package heal

import "context"

// RootCauseAnalyzer abstracts root cause analysis so that usecases depend on
// the interface rather than the concrete RootCauseEngine.
type RootCauseAnalyzer interface {
	// Analyze performs root cause analysis for the given step/phase error and
	// returns the best-matching result. Returns nil if no rule matches.
	Analyze(ctx context.Context, stepID, phase string, err error, metadata map[string]any) (*RootCauseResult, error)

	// AnalyzeFromReport performs root cause analysis from a standardized
	// FailureReport. It converts the report into the internal metadata format
	// and delegates to Analyze. Returns nil if no rule matches.
	AnalyzeFromReport(ctx context.Context, report *FailureReport) (*RootCauseResult, error)
}
