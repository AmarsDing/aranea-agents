package service

import (
	"context"
	"fmt"
	"strings"

	graphv1 "aranea-agents/api/kratos/graph/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/team"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/structpb"
)

func (s *GraphService) ExecuteGraphBuildConfig(
	ctx context.Context,
	graphID, sessionID string,
	cfg biz.GraphBuildConfig,
	initialState map[string]any,
) (*graphv1.ExecuteGraphResponse, error) {
	execID := uuid.NewString()
	exec, err := s.uc.ExecuteGraphBuildConfig(ctx, graphID, sessionID, execID, cfg, initialState)
	if err != nil {
		if s.graphTel != nil {
			s.graphTel.EnsureFinished(execID, err)
		}
		return nil, err
	}
	if s.orchProjector != nil {
		def := biz.GraphDefinitionFromBuildConfig(cfg, graphID, graphID)
		s.orchProjector.Start(context.Background(), sessionID, execID, graphID, def)
	}
	resp := &graphv1.ExecuteGraphResponse{
		ExecutionId: exec.ID,
		Status:      exec.Status,
	}
	return resp, nil
}

func (h *ChannelIngress) executeAsyncGraphTarget(
	ctx context.Context,
	target biz.ChannelAsyncGraphTarget,
	sessionID string,
	initialState map[string]any,
) (targetType, targetID, asyncID string, err error) {
	if h == nil || h.graphs == nil {
		return "", "", "", fmt.Errorf("channel async: graph service not configured")
	}
	switch target.TargetType {
	case "graph":
		st, serr := structpb.NewStruct(initialState)
		if serr != nil {
			return "", "", "", serr
		}
		resp, gerr := h.graphs.ExecuteGraph(ctx, &graphv1.ExecuteGraphRequest{
			GraphId:      target.GraphID,
			SessionId:    sessionID,
			InitialState: st,
		})
		if gerr != nil {
			return "", "", "", gerr
		}
		return "graph", target.GraphID, strings.TrimSpace(resp.GetExecutionId()), nil
	case "team_graph":
		if h.teams == nil {
			return "", "", "", fmt.Errorf("channel async: team repository not configured")
		}
		teamRow, terr := h.teams.GetTeamByID(ctx, target.TeamID)
		if terr != nil {
			return "", "", "", terr
		}
		def, perr := team.ParseDefinition(teamRow.DefinitionJSON)
		if perr != nil {
			return "", "", "", perr
		}
		agentKey := channelCompileAgentKey(ctx, h.agents)
		cfg, cerr := team.CompileToGraphRuntimeConfigFromJSON(ctx, def, teamRow.DefinitionJSON, agentKey, nil)
		if cerr != nil {
			return "", "", "", cerr
		}
		graphID := strings.TrimSpace(team.LinkedGraphIDFromDefinition(teamRow.DefinitionJSON))
		if graphID == "" {
			graphID = "team-" + strings.TrimSpace(teamRow.ID)
		}
		resp, gerr := h.graphs.ExecuteGraphBuildConfig(ctx, graphID, sessionID, cfg, initialState)
		if gerr != nil {
			return "", "", "", gerr
		}
		return "team_graph", graphID, strings.TrimSpace(resp.GetExecutionId()), nil
	default:
		return "", "", "", fmt.Errorf("channel async: unsupported target type %q", target.TargetType)
	}
}

func channelCompileAgentKey(ctx context.Context, agents biz.AgentRepository) team.CompileAgentKey {
	if agents == nil {
		return nil
	}
	return func(agentID string) string {
		ag, err := agents.GetAgentByID(ctx, strings.TrimSpace(agentID))
		if err != nil {
			return ""
		}
		return strings.TrimSpace(ag.AgentKey)
	}
}
