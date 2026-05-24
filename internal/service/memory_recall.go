package service

import (
	"context"
	"strings"

	v1 "aranea-agents/api/kratos/memory/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/sessionmemory"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

func (s *MemoryService) DebugMemoryRecall(ctx context.Context, req *v1.DebugMemoryRecallRequest) (*v1.DebugMemoryRecallResponse, error) {
	if s.memStore == nil {
		return nil, kerrors.InternalServer("MEMORY", "session memory store not wired")
	}
	agentID := strings.TrimSpace(req.GetAgentId())
	if agentID == "" {
		return nil, kerrors.BadRequest("MEMORY", "agent_id is required")
	}
	l2lim := req.GetL2Limit()
	if l2lim <= 0 {
		l2lim = 5
	}
	l3lim := req.GetL3Limit()
	if l3lim <= 0 {
		l3lim = 8
	}
	l2rows, err := s.memStore.RecallL2EpisodesDebug(ctx, agentID, strings.TrimSpace(req.GetSessionId()), strings.TrimSpace(req.GetQuery()), l2lim)
	if err != nil {
		return nil, err
	}
	l3rows, err := s.memStore.RecallL3FactsDebug(ctx, "agent", agentID, strings.TrimSpace(req.GetUserId()), strings.TrimSpace(req.GetQuery()), l3lim)
	if err != nil {
		return nil, err
	}
	out := &v1.DebugMemoryRecallResponse{}
	for _, row := range l2rows {
		out.L2Hits = append(out.L2Hits, pbRecallHit(row))
	}
	for _, row := range l3rows {
		out.L3Hits = append(out.L3Hits, pbRecallHit(row))
	}
	return out, nil
}

func (s *MemoryService) CompositeSearchMemories(ctx context.Context, req *v1.CompositeSearchMemoriesRequest) (*v1.CompositeSearchMemoriesResponse, error) {
	if s.memStore == nil {
		return nil, kerrors.InternalServer("MEMORY", "session memory store not wired")
	}
	agentID := strings.TrimSpace(req.GetAgentId())
	query := strings.TrimSpace(req.GetQuery())
	if agentID == "" || query == "" {
		return nil, kerrors.BadRequest("MEMORY", "agent_id and query are required")
	}
	lim := req.GetLimit()
	if lim <= 0 {
		lim = 10
	}
	rows, err := s.memStore.CompositeSearchMemories(ctx, agentID, strings.TrimSpace(req.GetSessionId()), strings.TrimSpace(req.GetUserId()), query, lim)
	if err != nil {
		return nil, err
	}
	out := &v1.CompositeSearchMemoriesResponse{}
	for _, row := range rows {
		text := strings.TrimSpace(row.Title)
		if text == "" {
			text = strings.TrimSpace(row.Statement)
		}
		if text == "" {
			text = strings.TrimSpace(row.Summary)
		}
		out.Items = append(out.Items, &v1.CompositeSearchHit{
			Layer: row.Layer,
			Id:    row.ID,
			Text:  text,
			Score: row.Scores.Total,
		})
	}
	return out, nil
}

func (s *MemoryService) GetMemoryWorkerStatus(_ context.Context, _ *v1.GetMemoryWorkerStatusRequest) (*v1.MemoryWorkerStatus, error) {
	done, dead, fallback, backfill, avg := biz.MemoryWorkerStatsGlobal().Snapshot()
	return &v1.MemoryWorkerStatus{
		JobsDone:            done,
		JobsDead:            dead,
		LlmFallbackTotal:    fallback,
		AvgExtractionSeconds: avg,
		EpisodeBackfillTotal: backfill,
	}, nil
}

func pbRecallHit(row sessionmemory.RecallDebugRow) *v1.MemoryRecallHit {
	return &v1.MemoryRecallHit{
		Layer:     row.Layer,
		Id:        row.ID,
		Title:     row.Title,
		Summary:   row.Summary,
		Statement: row.Statement,
		Scores: &v1.MemoryRecallScoreBreakdown{
			Keyword:      row.Scores.Keyword,
			Vector:       row.Scores.Vector,
			Importance:   row.Scores.Importance,
			Recency:      row.Scores.Recency,
			CrossEncoder: row.Scores.CrossEncoder,
			Total:        row.Scores.Total,
		},
	}
}
