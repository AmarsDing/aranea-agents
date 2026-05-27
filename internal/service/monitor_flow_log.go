package service

import (
	"context"
	"time"

	v1 "aranea-agents/api/kratos/monitor/v1"
	"aranea-agents/internal/biz"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

// FlowLogService handles FlowLog RPC delegation from MonitorService (SRP split).
type FlowLogService struct {
	flowLogs *biz.FlowLogUsecase
}

// NewFlowLogService creates a FlowLogService backed by a FlowLogUsecase.
func NewFlowLogService(flowLogs *biz.FlowLogUsecase) *FlowLogService {
	return &FlowLogService{flowLogs: flowLogs}
}

func (s *FlowLogService) ListFlowLogs(ctx context.Context, in *v1.ListFlowLogsRequest) (*v1.ListFlowLogsResponse, error) {
	if s == nil || s.flowLogs == nil {
		return &v1.ListFlowLogsResponse{}, nil
	}
	since, until, err := parseFlowLogTimeBounds(in.GetSince(), in.GetUntil())
	if err != nil {
		return nil, kerrors.BadRequest("MONITOR", err.Error())
	}
	result, err := s.flowLogs.List(ctx, biz.FlowLogQuery{
		TraceID:   in.GetTraceId(),
		SessionID: in.GetSessionId(),
		RunID:     in.GetRunId(),
		Severity:  in.GetSeverity(),
		Domain:    in.GetDomain(),
		Since:     since,
		Until:     until,
		Limit:     int(in.GetLimit()),
		Offset:    int(in.GetOffset()),
	})
	if err != nil {
		return nil, err
	}
	out := make([]*v1.FlowLogEntry, 0, len(result.Items))
	for i := range result.Items {
		r := result.Items[i]
		out = append(out, &v1.FlowLogEntry{
			Id:          r.ID,
			TraceId:     r.TraceID,
			SessionId:   r.SessionID,
			RunId:       r.RunID,
			TeamId:      r.TeamID,
			Domain:      r.Domain,
			AgentKey:    r.AgentKey,
			StepId:      r.StepID,
			FlowPhase:   r.FlowPhase,
			Severity:    r.Severity,
			Title:       r.Title,
			Message:     r.Message,
			PayloadJson: r.PayloadJSON,
			CreatedAt:   r.CreatedAt.UTC().Format(time.RFC3339Nano),
		})
	}
	return &v1.ListFlowLogsResponse{Items: out, Total: int32(result.Total)}, nil
}
