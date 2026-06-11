package service

import (
	"os"
	"strconv"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

func ProvideGraphUsecase(
	repo biz.GraphRepo,
	runRepo biz.GraphRunRepo,
	factory biz.GraphBuilderFactory,
	compiledTeamRepo biz.CompiledTeamRepo,
	telemetry *GraphExecutionTelemetry,
	orchProjector *GraphOrchestrationProjector,
	lg loggateway.Logger,
) *biz.GraphUsecase {
	observer := compositeGraphExecutionObserver{telemetry, orchProjector}
	return biz.NewGraphUsecase(biz.GraphUsecaseDeps{
		Repo: repo, RunRepo: runRepo, Factory: factory,
		Observer: observer, CompiledTeam: compiledTeamRepo, Lg: lg,
		GCConfig: resolveGraphGCConfig(lg),
	})
}

// resolveGraphGCConfig builds a GraphGCConfig from environment variables,
// falling back to DefaultGraphGCConfig when unset or invalid.
//
// Environment variables:
//   - GRAPH_GC_INTERVAL:         GC interval (Go duration, e.g. "5m")
//   - GRAPH_GC_EXECUTION_MAX_AGE: max age for stale executions (Go duration)
//   - GRAPH_GC_MAX_EXECUTIONS:   max cached executions before GC triggers
func resolveGraphGCConfig(lg loggateway.Logger) biz.GraphGCConfig {
	cfg := biz.DefaultGraphGCConfig()
	if v := os.Getenv("GRAPH_GC_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Interval = d
		} else {
			lg.Warn("invalid GRAPH_GC_INTERVAL, using default",
				loggateway.Str("value", v),
				loggateway.Err(err),
			)
		}
	}
	if v := os.Getenv("GRAPH_GC_EXECUTION_MAX_AGE"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.ExecutionMaxAge = d
		} else {
			lg.Warn("invalid GRAPH_GC_EXECUTION_MAX_AGE, using default",
				loggateway.Str("value", v),
				loggateway.Err(err),
			)
		}
	}
	if v := os.Getenv("GRAPH_GC_MAX_EXECUTIONS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.MaxExecutions = n
		} else if err != nil || n <= 0 {
			lg.Warn("invalid GRAPH_GC_MAX_EXECUTIONS, using default",
				loggateway.Str("value", v),
				loggateway.Err(err),
			)
		}
	}
	return cfg
}
