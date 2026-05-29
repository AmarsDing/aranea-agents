package service

import (
	graphv1 "aranea-agents/api/kratos/graph/v1"
	"aranea-agents/internal/biz"
)

type GraphService struct {
	graphv1.UnimplementedGraphServiceServer

	uc            *biz.GraphUsecase
	taskUC        *biz.TaskUsecase
	graphTel      *GraphExecutionTelemetry
	orchProjector *GraphOrchestrationProjector
}

var _ biz.GraphExecutor = (*GraphService)(nil)

func NewGraphService(uc *biz.GraphUsecase, taskUC *biz.TaskUsecase, graphTel *GraphExecutionTelemetry, orchProjector *GraphOrchestrationProjector, _ *GraphTaskRuntime) *GraphService {
	return &GraphService{uc: uc, taskUC: taskUC, graphTel: graphTel, orchProjector: orchProjector}
}
