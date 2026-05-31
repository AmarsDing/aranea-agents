package service

import (
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

func ProvideGraphUsecase(
	repo biz.GraphRepo,
	runRepo biz.GraphRunRepo,
	factory biz.GraphBuilderFactory,
	telemetry *GraphExecutionTelemetry,
	orchProjector *GraphOrchestrationProjector,
) *biz.GraphUsecase {
	observer := compositeGraphExecutionObserver{telemetry, orchProjector}
	return biz.NewGraphUsecase(repo, runRepo, factory, observer, loggateway.Global())
}
