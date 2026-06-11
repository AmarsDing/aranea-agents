package biz

import (
	"strings"

	"aranea-agents/pkg/apierror"
)

// ChannelAsyncGraphTarget resolves a channel long-task async graph execution target.
type ChannelAsyncGraphTarget struct {
	TargetType string // graph | team_graph
	GraphID    string
	TeamID     string
}

// ResolveChannelAsyncGraphTarget picks async graph execution target from channel long-task config.
// async_team_id takes precedence and uses the team compile path; async_graph_id uses stored graph.
func ResolveChannelAsyncGraphTarget(cfg ChannelLongTaskConfig) (ChannelAsyncGraphTarget, error) {
	teamID := strings.TrimSpace(cfg.AsyncTeamID)
	if teamID != "" {
		return ChannelAsyncGraphTarget{TargetType: "team_graph", TeamID: teamID}, nil
	}
	graphID := strings.TrimSpace(cfg.AsyncGraphID)
	if graphID != "" {
		return ChannelAsyncGraphTarget{TargetType: "graph", GraphID: graphID}, nil
	}
	return ChannelAsyncGraphTarget{}, apierror.BadRequest("CHANNEL", "channel async: no async_graph_id or async_team_id configured")
}
