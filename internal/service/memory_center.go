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
	if err := s.requireAdmin(); err != nil {
		return nil, err
	}
	agentID := strings.TrimSpace(req.GetAgentId())
	if agentID == "" {
		return nil, apierror.BadRequest(apierror.DomainMemory, "agent_id is required")
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

func (s *MemoryService) GetUnifiedMemoryGraph(ctx context.Context, req *v1.GetUnifiedMemoryGraphRequest) (*v1.GetUnifiedMemoryGraphResponse, error) {
	if err := s.requireAdmin(); err != nil {
		return nil, err
	}
	agentID := strings.TrimSpace(req.GetAgentId())
	if agentID == "" {
		return nil, apierror.BadRequest(apierror.DomainMemory, "agent_id is required")
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
