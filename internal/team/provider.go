package team

import (
	"context"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	"github.com/google/wire"
)

// ProvideTeamGraphRunCoordinator wires a singleton coordinator for team graph HITL/resume
// and starts a background ticker that evicts sessions older than sessionMaxAge.
func ProvideTeamGraphRunCoordinator(graphs *biz.GraphUsecase, teams biz.TeamRunRepo, bus event.Bus, sessionRepo biz.TeamGraphSessionRepo, lg loggateway.Logger) *TeamGraphRunCoordinator {
	coord := NewTeamGraphRunCoordinator(graphs, teams, bus, sessionRepo, nil, lg)
	interval := coord.cfg.CleanupInterval
	if interval <= 0 {
		interval = defaultCleanupInterval
	}
	safego.Go(context.Background(), "team.graph.coordinator.cleanup", func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			coord.CleanupStaleSessions()
		}
	})
	return coord
}

// ProviderSet wires team runtime.
var ProviderSet = wire.NewSet(NewRunner, ProvideTeamGraphRunCoordinator)
