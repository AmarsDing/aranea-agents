package evaluation

import (
	trpceval "trpc.group/trpc-go/trpc-agent-go/evaluation"
)

type MultiRunConfig struct {
	NumRuns            int
	ParallelEnabled    bool
	CaseParallelism    int
	ParallelInference  bool
	ParallelEvaluation bool
	RunDetailsEnabled  bool
}

func (c MultiRunConfig) ToOptions() []trpceval.Option {
	opts := []trpceval.Option{}
	if c.NumRuns > 1 {
		opts = append(opts, trpceval.WithNumRuns(c.NumRuns))
	}
	if c.ParallelEnabled {
		opts = append(opts, trpceval.WithNumRunsParallelEnabled(true))
	}
	if c.CaseParallelism > 0 {
		opts = append(opts, trpceval.WithEvalCaseParallelism(c.CaseParallelism))
	}
	if c.ParallelInference {
		opts = append(opts, trpceval.WithEvalCaseParallelInferenceEnabled(true))
	}
	if c.ParallelEvaluation {
		opts = append(opts, trpceval.WithEvalCaseParallelEvaluationEnabled(true))
	}
	if c.RunDetailsEnabled {
		opts = append(opts, trpceval.WithRunDetailsEnabled(true))
	}
	return opts
}

func DefaultMultiRunConfig() MultiRunConfig {
	return MultiRunConfig{
		NumRuns:            1,
		ParallelEnabled:    false,
		CaseParallelism:    1,
		ParallelInference:  false,
		ParallelEvaluation: false,
		RunDetailsEnabled:  true,
	}
}
