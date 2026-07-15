package service

import (
	graphv1 "aranea-agents/api/kratos/graph/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

type GraphService struct {
	graphv1.UnimplementedGraphServiceServer

	uc            *biz.GraphUsecase
	taskUC        *biz.TaskUsecase
	graphTel      *GraphExecutionTelemetry
	orchProjector *GraphOrchestrationProjector
	lg            loggateway.Logger
}

var _ biz.GraphExecutor = (*GraphService)(nil)

func NewGraphService(uc *biz.GraphUsecase, taskUC *biz.TaskUsecase, graphTel *GraphExecutionTelemetry, orchProjector *GraphOrchestrationProjector, _ *GraphTaskRuntime, lg loggateway.Logger) *GraphService {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &GraphService{uc: uc, taskUC: taskUC, graphTel: graphTel, orchProjector: orchProjector, lg: lg}
}
