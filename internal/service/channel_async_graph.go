package service

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/team"

	"github.com/google/uuid"
	kerrors "github.com/go-kratos/kratos/v2/errors"
)

// ExecuteGraphBuildConfig executes a graph from a build config and returns the execution ID.
// This is the biz-level entry point; proto mapping happens only in the gRPC handler.
func (s *GraphService) ExecuteGraphBuildConfig(
	ctx context.Context,
	graphID, sessionID string,
	cfg biz.GraphBuildConfig,
	initialState map[string]any,
) (string, error) {
	execID := uuid.NewString()
	exec, err := s.uc.ExecuteGraphBuildConfig(ctx, graphID, sessionID, execID, cfg, initialState)
	if err != nil {
		if s.graphTel != nil {
			s.graphTel.EnsureFinished(execID, err)
		}
		return "", err
	}
	if s.orchProjector != nil {
		def := biz.GraphDefinitionFromBuildConfig(cfg, graphID, graphID)
		s.orchProjector.Start(context.Background(), sessionID, execID, graphID, def)
	}
	return exec.ID, nil
}

// ExecuteGraphByID executes a stored graph by ID and returns the execution ID.
// Implements channelGraphExecutor without exposing proto types.
func (s *GraphService) ExecuteGraphByID(ctx context.Context, graphID, sessionID string, initialState map[string]any) (string, error) {
	execID := uuid.NewString()
	exec, err := s.uc.ExecuteGraph(ctx, graphID, sessionID, execID, initialState)
	if err != nil {
		if s.graphTel != nil {
			s.graphTel.EnsureFinished(execID, err)
		}
		return "", err
	}
	return exec.ID, nil
}

func (h *ChannelIngress) executeAsyncGraphTarget(
	ctx context.Context,
	target biz.ChannelAsyncGraphTarget,
	sessionID string,
	initialState map[string]any,
) (targetType, targetID, asyncID string, err error) {
	if h == nil || h.graphs == nil {
		return "", "", "", kerrors.BadRequest("CHANNEL", "graph service not configured")
	}
	switch target.TargetType {
	case "graph":
		execID, gerr := h.graphs.ExecuteGraphByID(ctx, target.GraphID, sessionID, initialState)
		if gerr != nil {
			return "", "", "", gerr
		}
		return "graph", target.GraphID, strings.TrimSpace(execID), nil
	case "team_graph":
		teamRow, terr := h.channels.GetTeamByID(ctx, target.TeamID)
		if terr != nil {
			return "", "", "", terr
		}
		def, perr := team.ParseDefinition(teamRow.DefinitionJSON)
		if perr != nil {
			return "", "", "", perr
		}
		agentKey := h.channels.AgentKeyResolver(ctx)
		cfg, cerr := team.CompileToGraphRuntimeConfigFromJSON(ctx, def, teamRow.DefinitionJSON, agentKey, nil, h.lg)
		if cerr != nil {
			return "", "", "", cerr
		}
		graphID := strings.TrimSpace(team.LinkedGraphIDFromDefinition(teamRow.DefinitionJSON))
		if graphID == "" {
			graphID = "team-" + strings.TrimSpace(teamRow.ID)
		}
		execID, gerr := h.graphs.ExecuteGraphBuildConfig(ctx, graphID, sessionID, cfg, initialState)
		if gerr != nil {
			return "", "", "", gerr
		}
		return "team_graph", graphID, strings.TrimSpace(execID), nil
	default:
		return "", "", "", kerrors.BadRequest("CHANNEL", fmt.Sprintf("unsupported target type %q", target.TargetType))
	}
}

