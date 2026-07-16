package service

import (
	"context"
	"strings"

	v1 "aranea-agents/api/kratos/memory/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
)

func (s *MemoryService) DebugMemoryRecall(ctx context.Context, req *v1.DebugMemoryRecallRequest) (*v1.DebugMemoryRecallResponse, error) {
	if s.debugRecaller == nil {
		return nil, apierror.Internal("MEMORY", "session memory debug recaller not wired")
	}
	agentID := strings.TrimSpace(req.GetAgentId())
	if agentID == "" {
		return nil, apierror.BadRequest("MEMORY", "agent_id is required")
	}
	if _, err := authorizeMemoryScope(ctx, "agent", agentID, false); err != nil {
		return nil, err
	}
	userID := strings.TrimSpace(req.GetUserId())
	if userID != "" {
		if _, err := authorizeMemoryScope(ctx, "user", userID, false); err != nil {
			return nil, err
		}
	}
	l2lim := req.GetL2Limit()
	if l2lim <= 0 {
		l2lim = 5
	}
	l3lim := req.GetL3Limit()
	if l3lim <= 0 {
		l3lim = 8
	}
	l2rows, err := s.debugRecaller.RecallL2EpisodesDebug(ctx, agentID, strings.TrimSpace(req.GetSessionId()), strings.TrimSpace(req.GetQuery()), l2lim)
	if err != nil {
		return nil, err
	}
	l3rows, err := s.debugRecaller.RecallL3FactsDebug(ctx, "agent", agentID, userID, strings.TrimSpace(req.GetQuery()), l3lim)
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
	if s.debugRecaller == nil {
		return nil, apierror.Internal("MEMORY", "session memory debug recaller not wired")
	}
	agentID := strings.TrimSpace(req.GetAgentId())
	query := strings.TrimSpace(req.GetQuery())
	if agentID == "" || query == "" {
		return nil, apierror.BadRequest("MEMORY", "agent_id and query are required")
	}
	if _, err := authorizeMemoryScope(ctx, "agent", agentID, false); err != nil {
		return nil, err
	}
	userID := strings.TrimSpace(req.GetUserId())
	if userID != "" {
		if _, err := authorizeMemoryScope(ctx, "user", userID, false); err != nil {
			return nil, err
		}
	}
	lim := req.GetLimit()
	if lim <= 0 {
		lim = 10
	}
	rows, err := s.debugRecaller.CompositeSearchMemories(ctx, agentID, strings.TrimSpace(req.GetSessionId()), userID, query, lim)
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

func (s *MemoryService) GetMemoryWorkerStatus(ctx context.Context, _ *v1.GetMemoryWorkerStatusRequest) (*v1.MemoryWorkerStatus, error) {
	done, dead, fallback, backfill, avg := s.workerStats.Snapshot()
	out := &v1.MemoryWorkerStatus{
		JobsDone:             done,
		JobsDead:             dead,
		LlmFallbackTotal:     fallback,
		AvgExtractionSeconds: avg,
		EpisodeBackfillTotal: backfill,
		DbAvailable:          true,
	}
	if s.factIndexCounter != nil {
		fresh, stale, disabled, err := s.factIndexCounter.CountFactsByIndexStatus(ctx)
		if err != nil {
			out.DbAvailable = false
		} else {
			out.FactIndexStaleCount = stale
			out.FactIndexDisabledCount = disabled
			_ = fresh
		}
	}
	if s.deadLetterRepo != nil {
		pending, replayed, abandoned, err := s.deadLetterRepo.CountDeadLettersByState(ctx)
		if err != nil {
			out.DbAvailable = false
		} else {
			out.DeadLetterPending = pending
			_ = replayed
			_ = abandoned
		}
	}
	if s.queueStats != nil {
		highLen, normalLen, lowLen, highCap, normalCap, lowCap, dropped, debounced := s.queueStats.QueueLaneStats()
		out.QueueHigh = &v1.MemoryWorkerStatus_QueueStats{
			Capacity:       int64(highCap),
			InFlight:       int64(highLen),
			DroppedTotal:   dropped,
			DebouncedTotal: debounced,
		}
		out.QueueNormal = &v1.MemoryWorkerStatus_QueueStats{
			Capacity: int64(normalCap),
			InFlight: int64(normalLen),
		}
		out.QueueLow = &v1.MemoryWorkerStatus_QueueStats{
			Capacity: int64(lowCap),
			InFlight: int64(lowLen),
		}
	}
	return out, nil
}

func pbRecallHit(row biz.RecallDebugRow) *v1.MemoryRecallHit {
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
			QualityScore: row.Scores.QualityScore,
			CrossEncoder: row.Scores.CrossEncoder,
			Total:        row.Scores.Total,
		},
	}
}
