package service

import (
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
	return biz.NewGraphUsecase(repo, runRepo, factory, observer, compiledTeamRepo, lg)
}
