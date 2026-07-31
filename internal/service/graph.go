package service

import (
	"context"

	graphv1 "aranea-agents/api/kratos/graph/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/loggateway"
)

type GraphService struct {
	graphv1.UnimplementedGraphServiceServer

	uc            *biz.GraphUsecase
	taskUC        *biz.TaskUsecase
	graphTel      *GraphExecutionTelemetry
	orchProjector *GraphOrchestrationProjector
	monitorBus    contract.MonitorBus
	lg            loggateway.Logger
}

var _ biz.GraphExecutor = (*GraphService)(nil)

func NewGraphService(uc *biz.GraphUsecase, taskUC *biz.TaskUsecase, graphTel *GraphExecutionTelemetry, orchProjector *GraphOrchestrationProjector, _ *GraphTaskRuntime, monitorBus contract.MonitorBus, lg loggateway.Logger) *GraphService {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &GraphService{uc: uc, taskUC: taskUC, graphTel: graphTel, orchProjector: orchProjector, monitorBus: monitorBus, lg: lg}
}

// graphFlow builds a run-scoped flow-log emitter for the graph domain.
// Returns nil when no monitor bus is configured (nil-safe: emission skipped).
func (s *GraphService) graphFlow(ctx context.Context, sessionID, runID string) *event.TraceEmitter {
	if s == nil || s.monitorBus == nil {
		return nil
	}
	return event.NewTraceEmitterForRun(event.TraceEmitterOpts{
		Ctx:       ctx,
		SessionID: sessionID,
		RunID:     runID,
		Domain:    event.TraceDomainGraph,
		LG:        s.lg,
		Infra:     event.NewInfraFromBus(s.monitorBus),
	})
}
