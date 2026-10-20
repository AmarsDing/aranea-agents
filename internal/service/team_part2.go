package service

import (
	v1 "aranea-agents/api/kratos/team/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	"context"
)

// findTeamTestRun 按测试会话 ID 在团队最近 run 列表中定位持久化 run 记录；
// 未命中或查询失败返回 ok=false（调用方回退合成轻量响应）。
func (s *TeamService) findTeamTestRun(ctx context.Context, teamID, sessionID string) (biz.TeamRunRecord, bool) {
	runs, err := s.uc.ListRuns(ctx, teamID, 10)
	if err != nil {
		return biz.TeamRunRecord{}, false
	}
	for _, candidate := range runs {
		if candidate.SessionID == sessionID {
			return candidate, true
		}
	}
	return biz.TeamRunRecord{}, false
}

func (s *TeamService) ListTeamRunSteps(ctx context.Context, req *v1.ListTeamRunStepsRequest) (*v1.ListTeamRunStepsResponse, error) {
	if err := s.assertRunTeamAccess(ctx, req.GetRunId()); err != nil {
		return nil, err
	}
	items, err := s.uc.ListRunSteps(ctx, req.GetRunId())
	if err != nil {
		return nil, err
	}
	out := &v1.ListTeamRunStepsResponse{Items: make([]*v1.TeamRunStep, 0, len(items))}
	for i := range items {
		out.Items = append(out.Items, toProtoTeamRunStep(items[i]))
	}
	return out, nil
}

func (s *TeamService) UpdateSwarmMembers(ctx context.Context, req *v1.UpdateSwarmMembersRequest) (*v1.UpdateSwarmMembersResponse, error) {
	if err := s.assertTeamMutateAccess(ctx, req.GetTeamId()); err != nil {
		return nil, err
	}
	s.lg.Info("update swarm members",
		loggateway.StepID("service.team"),
		loggateway.Str("team_id", req.GetTeamId()),
		loggateway.Int("add_count", len(req.GetAddAgentIds())),
		loggateway.Int("remove_count", len(req.GetRemoveAgentIds())))
	updated, err := s.uc.UpdateSwarmMembers(ctx, req.GetTeamId(), req.GetAddAgentIds(), req.GetRemoveAgentIds())
	if err != nil {
		s.lg.Error("update swarm members failed",
			loggateway.StepID("service.team"),
			loggateway.Str("team_id", req.GetTeamId()),
			loggateway.Err(err))
		return nil, mapTeamErr(err)
	}
	return &v1.UpdateSwarmMembersResponse{Updated: updated}, nil
}

func (s *TeamService) ExportTeamStructure(ctx context.Context, req *v1.ExportTeamStructureRequest) (*v1.ExportTeamStructureResponse, error) {
	if err := s.assertTeamAccess(ctx, req.GetTeamId()); err != nil {
		return nil, err
	}
	snap, err := s.exportStructureViaCompiler(ctx, req.GetTeamId())
	if err != nil {
		return nil, mapTeamErr(err)
	}
	resp := &v1.ExportTeamStructureResponse{
		EntryNodeId: snap.EntryNodeID,
	}
	for _, n := range snap.Nodes {
		resp.Nodes = append(resp.Nodes, &v1.StructureNode{NodeId: n.NodeID, Kind: n.Kind, Name: n.Name})
	}
	for _, e := range snap.Edges {
		resp.Edges = append(resp.Edges, &v1.StructureEdge{FromNodeId: e.FromNodeID, ToNodeId: e.ToNodeID})
	}
	for _, sf := range snap.Surfaces {
		resp.Surfaces = append(resp.Surfaces, &v1.StructureSurface{NodeId: sf.NodeID, Name: sf.Name})
	}
	return resp, nil
}

func (s *TeamService) GetTeamRunSummary(ctx context.Context, req *v1.GetTeamRunSummaryRequest) (*v1.GetTeamRunSummaryResponse, error) {
	if err := s.assertRunTeamAccess(ctx, req.GetId()); err != nil {
		return nil, err
	}
	data, err := s.uc.GetRunSummary(ctx, req.GetId())
	if err != nil {
		return nil, mapTeamErr(err)
	}
	return &v1.GetTeamRunSummaryResponse{Summary: toProtoTeamRunSummary(data)}, nil
}
