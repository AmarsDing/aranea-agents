package service

import (
	"context"

	v1 "aranea-agents/api/kratos/team/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/team"
)

func compileSnapshotToProto(snap team.CompileSnapshot) *v1.CompileTeamGraphResponse {
	resp := &v1.CompileTeamGraphResponse{
		TemplateId:  snap.TemplateID,
		Mode:        snap.Mode,
		EntryPoint:  snap.EntryPoint,
		FinishPoint: snap.FinishPoint,
		GraphJson:   snap.GraphJSON,
		Valid:       snap.Valid,
	}
	if snap.CompileError != "" {
		resp.Issues = append(resp.Issues, &v1.CompileTeamGraphValidationIssue{
			Code:    "compile_error",
			Message: snap.CompileError,
		})
	}
	for _, n := range snap.Nodes {
		resp.Nodes = append(resp.Nodes, &v1.CompiledGraphNodeView{
			Id:               n.ID,
			Type:             n.Type,
			AgentName:        n.AgentName,
			Role:             n.RequiredRole,
			Description:      n.Description,
			AgentDisplayName: n.Description,
			TaskPrompt:       n.Instruction,
		})
	}
	for _, e := range snap.Edges {
		resp.Edges = append(resp.Edges, &v1.CompiledGraphEdgeView{From: e.From, To: e.To, EdgeKind: e.Kind})
	}
	for _, ce := range snap.ConditionalEdges {
		resp.ConditionalEdges = append(resp.ConditionalEdges, &v1.CompiledGraphConditionalEdgeView{
			From:    ce.From,
			PathMap: ce.PathMap,
		})
	}
	return resp
}

func (s *TeamService) buildObservatoryCompiledTopology(ctx context.Context, definitionJSON string) *v1.CompileTeamGraphResponse {
	def, err := team.ParseDefinition(definitionJSON)
	if err != nil {
		return &v1.CompileTeamGraphResponse{
			Valid: false,
			Issues: []*v1.CompileTeamGraphValidationIssue{{
				Code:    "invalid_definition",
				Message: err.Error(),
			}},
		}
	}
	resp, _ := s.buildCompileTeamGraphResponse(ctx, def, definitionJSON)
	return resp
}

func toProtoActivitySnapshot(a *biz.ActivitySnapshot) *v1.ActivitySnapshotView {
	if a == nil {
		return nil
	}
	return &v1.ActivitySnapshotView{
		Kind:          a.Kind,
		DisplayLabel:  a.DisplayLabel,
		ToolName:      a.ToolName,
		Status:        a.Status,
		Summary:       a.Summary,
		ArgumentsJson: a.ArgumentsJSON,
		ResultJson:    a.ResultJSON,
		StartedAt:     a.StartedAt,
		FinishedAt:    a.FinishedAt,
		DurationMs:    a.DurationMS,
		ErrorCode:     a.ErrorCode,
	}
}

func toProtoAgentNodeState(n biz.AgentNodeState) *v1.AgentNodeStateView {
	out := &v1.AgentNodeStateView{
		NodeId:          n.NodeID,
		AgentId:         n.AgentID,
		AgentKey:        n.AgentKey,
		AgentName:       n.AgentName,
		Role:            n.Role,
		Status:          string(n.Status),
		DisplayStatus:   string(n.DisplayStatus),
		Phase:           string(n.Phase),
		RetryCount:      int32(n.RetryCount),
		InputPreview:    n.InputPreview,
		OutputPreview:   n.OutputPreview,
		ErrorMessage:    n.ErrorMessage,
		CurrentActivity: toProtoActivitySnapshot(n.CurrentActivity),
	}
	for _, snap := range n.ActivityHistory {
		cp := snap
		out.ActivityHistory = append(out.ActivityHistory, toProtoActivitySnapshot(&cp))
	}
	return out
}

func (s *TeamService) GetTeamRunObservatory(ctx context.Context, req *v1.GetTeamRunObservatoryRequest) (*v1.GetTeamRunObservatoryResponse, error) {
	obs, err := s.uc.GetRunObservatory(ctx, req.GetRunId())
	if err != nil {
		return nil, mapTeamErr(err)
	}
	resp := &v1.GetTeamRunObservatoryResponse{
		RunId:                  obs.RunID,
		TeamId:                 obs.TeamID,
		SessionId:              obs.SessionID,
		Status:                 obs.Status,
		Mode:                   obs.Mode,
		GraphExecutionId:       obs.GraphExecutionID,
		DefinitionSnapshotJson: obs.DefinitionSnapshotJSON,
		CompiledTopology:       s.buildObservatoryCompiledTopology(ctx, obs.DefinitionSnapshotJSON),
		Nodes:                  make([]*v1.AgentNodeStateView, 0, len(obs.Nodes)),
	}
	for _, n := range obs.Nodes {
		cp := n
		resp.Nodes = append(resp.Nodes, toProtoAgentNodeState(cp))
	}
	return resp, nil
}
