package service

import "aranea-agents/internal/biz"

// ProvideGraphUsecase wires the graph execution observer from the service layer into biz.
func ProvideGraphUsecase(
	repo biz.GraphRepo,
	runRepo biz.GraphRunRepo,
	factory biz.GraphBuilderFactory,
	telemetry *GraphExecutionTelemetry,
) *biz.GraphUsecase {
	return biz.NewGraphUsecase(repo, runRepo, factory, telemetry)
}
