package service

import (
	"context"
	"encoding/json"
	"strings"

	v1 "aranea-agents/api/kratos/team/v1"
	"aranea-agents/internal/biz"
	graphadapter "aranea-agents/internal/graph/adapter"
	graphtrpc "aranea-agents/internal/graph/trpc"
	"aranea-agents/internal/team"
	"aranea-agents/pkg/apierror"
)

func (s *TeamService) CompileTeamGraph(ctx context.Context, req *v1.CompileTeamGraphRequest) (*v1.CompileTeamGraphResponse, error) {
	teamID := strings.TrimSpace(req.GetTeamId())
	if teamID == "" {
		return nil, apierror.BadRequest("TEAM", "team_id is required")
	}
	if err := s.assertTeamAccess(ctx, teamID); err != nil { // N5: IDOR
		return nil, err
	}
	rawDef := strings.TrimSpace(req.GetDefinitionJson())
	if rawDef == "" {
		t, err := s.uc.Get(ctx, teamID)
		if err != nil {
			return nil, mapTeamErr(err)
		}
		rawDef = t.DefinitionJSON
	}
	def, err := team.ParseDefinition(rawDef)
	if err != nil {
		return nil, apierror.BadRequest("TEAM", "invalid definition_json: "+err.Error())
	}
	return s.buildCompileTeamGraphResponse(ctx, def, rawDef)
}

func (s *TeamService) buildCompileTeamGraphResponse(ctx context.Context, def team.Definition, rawDefinitionJSON string) (*v1.CompileTeamGraphResponse, error) {
	agentKey := s.compileAgentKeyResolver(ctx)
	cfg, taskMeta, compileErr := team.CompileToGraphBuildConfigFromJSON(def, rawDefinitionJSON, agentKey, s.lg)
	resp := &v1.CompileTeamGraphResponse{
		TemplateId: team.CompileTemplateID(def.Mode),
		Mode:       strings.ToLower(strings.TrimSpace(def.Mode)),
		Valid:      compileErr == nil,
		// ADR-08 A2: template path emits the canonical embedded graph spec so
		// frontend editors stop re-implementing mode->graph generation locally.
		DefinitionGraphJson: team.DefinitionGraphSpecJSON(ctx, def, rawDefinitionJSON, s.lg),
	}
	if compileErr != nil {
		resp.Issues = append(resp.Issues, &v1.CompileTeamGraphValidationIssue{
			Code:    "compile_error",
			Message: compileErr.Error(),
		})
		return resp, nil
	}
	resp.EntryPoint = cfg.EntryPoint
	resp.FinishPoint = cfg.FinishPoint
	resp.Nodes = s.buildCompiledGraphNodeViews(ctx, def, cfg.Nodes, taskMeta)
	for _, e := range cfg.Edges {
		resp.Edges = append(resp.Edges, &v1.CompiledGraphEdgeView{From: e.From, To: e.To, EdgeKind: e.Kind})
	}
	for _, ce := range cfg.ConditionalEdges {
		resp.ConditionalEdges = append(resp.ConditionalEdges, &v1.CompiledGraphConditionalEdgeView{
			From:    ce.From,
			PathMap: ce.PathMap,
		})
	}
	if b, err := json.Marshal(cfg); err == nil {
		resp.GraphJson = string(b)
	}
	validation := graphadapter.ValidateBizGraphBuildConfig(ctx, cfg, s.agentExistsByName(ctx))
	for _, issue := range validation.Errors {
		resp.Issues = append(resp.Issues, &v1.CompileTeamGraphValidationIssue{
			Code:    string(issue.Code),
			NodeId:  issue.NodeID,
			Field:   issue.Field,
			Message: issue.Message,
		})
		resp.Valid = false
	}
	for _, issue := range validation.Warnings {
		resp.Issues = append(resp.Issues, &v1.CompileTeamGraphValidationIssue{
			Code:    string(issue.Code),
			NodeId:  issue.NodeID,
			Field:   issue.Field,
			Message: issue.Message,
			Warning: true,
		})
	}
	return resp, nil
}

func (s *TeamService) compileAgentKeyResolver(ctx context.Context) team.CompileAgentKey {
	if s.agents == nil {
		return nil
	}
	return func(agentID string) string {
		ag, err := s.agents.Get(ctx, strings.TrimSpace(agentID))
		if err != nil {
			return ""
		}
		return strings.TrimSpace(ag.AgentKey)
	}
}

func (s *TeamService) agentExistsByName(ctx context.Context) graphtrpc.AgentExistenceChecker {
	if s.agents == nil {
		return func(context.Context, string) bool { return true }
	}
	return func(checkCtx context.Context, agentName string) bool {
		name := strings.TrimSpace(agentName)
		if name == "" {
			return false
		}
		_, err := s.agents.GetByAgentKey(checkCtx, name)
		return err == nil
	}
}

func (s *TeamService) exportStructureViaCompiler(ctx context.Context, teamID string) (*biz.TeamStructureSnapshot, error) {
	t, err := s.uc.Get(ctx, teamID)
	if err != nil {
		return nil, err
	}
	def, err := team.ParseDefinition(t.DefinitionJSON)
	if err != nil {
		return nil, apierror.BadRequest("TEAM", "invalid definition_json")
	}
	return team.ExportStructureSnapshot(t.TeamKey, t.DisplayName, def, s.compileAgentKeyResolver(ctx), s.lg)
}
