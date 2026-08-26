package service

import (
	"context"
	"strings"

	v1 "aranea-agents/api/kratos/memory/v1"
	"aranea-agents/pkg/apierror"
)

// Memory-center handlers (layer panorama + unified cross-layer graph).
// Design: docs/development/memory/memory.design.md §10.2.

func (s *MemoryService) GetMemoryLayerOverview(ctx context.Context, req *v1.GetMemoryLayerOverviewRequest) (*v1.GetMemoryLayerOverviewResponse, error) {
	if err := s.requireWired(); err != nil {
		return nil, err
	}
	agentID := strings.TrimSpace(req.GetAgentId())
	if err := s.assertAgentMemoryAccess(ctx, agentID); err != nil {
		return nil, err
	}
	ov, err := s.admin.GetLayerOverview(ctx, agentID, strings.TrimSpace(req.GetSessionId()))
	if err != nil {
		return nil, err
	}
	out := &v1.GetMemoryLayerOverviewResponse{}
	for _, l := range ov.Layers {
		out.Layers = append(out.Layers, &v1.MemoryLayerStat{
			Layer:        l.Layer,
			ItemCount:    l.ItemCount,
			TodayAdded:   l.TodayAdded,
			RecallHits:   l.RecallHits,
			Health:       l.Health,
			HeadlineJson: l.HeadlineJSON,
		})
	}
	for _, a := range ov.ActionItems {
		out.ActionItems = append(out.ActionItems, &v1.MemoryActionItem{
			Kind:      a.Kind,
			Count:     a.Count,
			TargetTab: a.TargetTab,
		})
	}
	for _, item := range ov.ActivityFeed {
		out.ActivityFeed = append(out.ActivityFeed, &v1.MemoryActivityItem{
			Ts:        item.Ts,
			Kind:      item.Kind,
			LayerFrom: item.LayerFrom,
			LayerTo:   item.LayerTo,
			Summary:   item.Summary,
		})
	}
	return out, nil
}

// ListMemoryEpisodes serves the P3 browse tab: paginated L2 episodes
// (created_at DESC) for one agent, optionally filtered by session.
// Design: docs/development/memory/memory.design.md §10.6.
func (s *MemoryService) ListMemoryEpisodes(ctx context.Context, req *v1.ListMemoryEpisodesRequest) (*v1.ListMemoryEpisodesResponse, error) {
	if err := s.requireWired(); err != nil {
		return nil, err
	}
	agentID := strings.TrimSpace(req.GetAgentId())
	if err := s.assertAgentMemoryAccess(ctx, agentID); err != nil {
		return nil, err
	}
	items, total, err := s.admin.ListEpisodesAdmin(ctx, agentID, strings.TrimSpace(req.GetSessionId()), req.GetLimit(), req.GetOffset())
	if err != nil {
		return nil, err
	}
	out := &v1.ListMemoryEpisodesResponse{Total: total}
	for _, it := range items {
		out.Items = append(out.Items, &v1.MemoryEpisode{
			Id:                  it.ID,
			SessionId:           it.SessionID,
			AgentId:             it.AgentID,
			EpisodeKind:         it.Kind,
			Title:               it.Title,
			OutcomeSummary:      it.OutcomeSummary,
			Importance:          it.Importance,
			ConsolidationStatus: it.ConsolidationStatus,
			ConsolidatedL3Count: it.ConsolidatedL3Count,
			EndedAt:             it.EndedAt,
			CreatedAt:           it.CreatedAt,
		})
	}
	return out, nil
}

func (s *MemoryService) GetUnifiedMemoryGraph(ctx context.Context, req *v1.GetUnifiedMemoryGraphRequest) (*v1.GetUnifiedMemoryGraphResponse, error) {
	if err := s.requireWired(); err != nil {
		return nil, err
	}
	agentID := strings.TrimSpace(req.GetAgentId())
	if err := s.assertAgentMemoryAccess(ctx, agentID); err != nil {
		return nil, err
	}
	g, err := s.admin.GetUnifiedMemoryGraph(ctx, agentID, strings.TrimSpace(req.GetFocus()), req.GetHops(), req.GetMinWeight(), req.GetLayers())
	if err != nil {
		return nil, err
	}
	out := &v1.GetUnifiedMemoryGraphResponse{
		Focus:       g.Focus,
		EmptyReason: g.EmptyReason,
	}
	for _, n := range g.Nodes {
		out.Nodes = append(out.Nodes, &v1.UnifiedGraphNode{
			Id:       n.ID,
			Layer:    n.Layer,
			Kind:     n.Kind,
			Label:    n.Label,
			Weight:   n.Weight,
			MetaJson: n.MetaJSON,
		})
	}
	for _, e := range g.Edges {
		out.Edges = append(out.Edges, &v1.UnifiedGraphEdge{
			Source:   e.Source,
			Target:   e.Target,
			Type:     e.Type,
			Label:    e.Label,
			Weight:   e.Weight,
			Polarity: e.Polarity,
		})
	}
	out.NodeCount = int32(len(out.Nodes))
	out.EdgeCount = int32(len(out.Edges))
	out.FilteredEdgeCount = g.FilteredEdgeCount
	return out, nil
}

// ListMemoryFactPendings serves the R3 approval-layer pending tab
// (79-runtime-governance Phase 3.5): withheld high-risk fact writes for one
// agent, newest first, with proposed vs prior bodies and adjudicator reason.
// Decisions are made in the twinmonitor approval center — this is read-only.
func (s *MemoryService) ListMemoryFactPendings(ctx context.Context, req *v1.ListMemoryFactPendingsRequest) (*v1.ListMemoryFactPendingsResponse, error) {
	if err := s.requireWired(); err != nil {
		return nil, err
	}
	if s.factPendingStore == nil {
		return nil, apierror.Internal(apierror.DomainMemory, "memory fact pending store not wired")
	}
	agentID := strings.TrimSpace(req.GetAgentId())
	if err := s.assertAgentMemoryAccess(ctx, agentID); err != nil {
		return nil, err
	}
	recs, err := s.factPendingStore.ListPending(ctx, agentID, strings.TrimSpace(req.GetStatus()), int(req.GetLimit()))
	if err != nil {
		return nil, err
	}
	out := &v1.ListMemoryFactPendingsResponse{}
	for _, r := range recs {
		out.Items = append(out.Items, &v1.MemoryFactPendingItem{
			Id:                r.ID,
			AgentId:           r.AgentID,
			FactKey:           r.FactKey,
			Verdict:           r.Verdict,
			ProposedBody:      r.ProposedBody,
			PriorBody:         r.PriorBody,
			AdjudicatorReason: r.AdjudicatorReason,
			Status:            r.Status,
			Approver:          r.Approver,
			CreatedAt:         r.CreatedAt,
			DecidedAt:         r.DecidedAt,
		})
	}
	return out, nil
}
