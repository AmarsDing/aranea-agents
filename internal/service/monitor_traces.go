package service

import (
	"context"

	v1 "aranea-agents/api/kratos/monitor/v1"
	"aranea-agents/internal/biz"
)

func (s *MonitorService) ListMonitorTraces(ctx context.Context, in *v1.ListMonitorTracesRequest) (*v1.ListMonitorTracesResponse, error) {
	result, err := s.uc.ListMonitorTraces(ctx, biz.MonitorTracesQuery{
		Limit:           in.GetLimit(),
		Offset:          in.GetOffset(),
		AgentID:         in.GetAgentId(),
		Provider:        in.GetProvider(),
		Model:           in.GetModel(),
		Status:          in.GetStatus(),
		Keyword:         in.GetKeyword(),
		ExcludeInternal: in.GetExcludeInternal(),
		Domain:          in.GetDomain(),
	})
	if err != nil {
		return nil, err
	}
	out := make([]*v1.MonitorPlatformRow, 0, len(result.Items))
	for _, row := range result.Items {
		out = append(out, bizMonitorRowToProto(row, s.lg))
	}
	return &v1.ListMonitorTracesResponse{
		Items:        out,
		Total:        result.Total,
		StatusCounts: result.StatusCounts,
		DomainCounts: result.DomainCounts,
	}, nil
}
