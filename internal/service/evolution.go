package service

import (
	"context"
	"time"

	v1 "aranea-agents/api/kratos/evolution/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"google.golang.org/protobuf/types/known/timestamppb"
)

// evolutionDiversityDefaultWindow 是 since 缺省时的默认观测窗口（最近 24h）。
const evolutionDiversityDefaultWindow = 24 * time.Hour

// EvolutionService 实现 kratos.evolution.v1.EvolutionService：统一进化建议
// 的平台级观测端点（P3 M5）。只读，无写路径。
type EvolutionService struct {
	v1.UnimplementedEvolutionServiceServer

	diversity biz.UnifiedEvolutionDiversityReader
	lg        loggateway.Logger
}

func NewEvolutionService(diversity biz.UnifiedEvolutionDiversityReader, lg loggateway.Logger) *EvolutionService {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &EvolutionService{diversity: diversity, lg: lg}
}

func (s *EvolutionService) GetEvolutionDiversityOverview(ctx context.Context, req *v1.GetEvolutionDiversityOverviewRequest) (*v1.GetEvolutionDiversityOverviewResponse, error) {
	if s.diversity == nil {
		return nil, apierror.Unavailable("EVOLUTION", "diversity reader not configured")
	}
	since := time.Now().Add(-evolutionDiversityDefaultWindow)
	if req.GetSince() != nil {
		since = req.GetSince().AsTime()
	}
	stats, err := s.diversity.GetDiversityOverview(ctx, since, int(req.GetTopTools()))
	if err != nil {
		return nil, err
	}
	resp := &v1.GetEvolutionDiversityOverviewResponse{}
	for i := range stats {
		resp.Buckets = append(resp.Buckets, &v1.EvolutionDiversityBucket{
			TriggerSource: stats[i].TriggerSource,
			Count:         int32(stats[i].Count),
			LatestAt:      timestamppb.New(stats[i].LatestAt),
			TopTools:      stats[i].TopTools,
		})
	}
	return resp, nil
}
