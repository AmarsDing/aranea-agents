package team

import (
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/appctx"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	"github.com/google/wire"
)

// ProvideTeamGraphRunCoordinator wires a singleton coordinator for team graph HITL/resume
// and starts a background ticker that evicts sessions older than sessionMaxAge.
func ProvideTeamGraphRunCoordinator(graphs *biz.GraphUsecase, teamRunReader biz.TeamRunReader, teamRunWriter biz.TeamRunWriter, runTransitioner biz.TeamRunStatusTransitioner, bus event.Bus, sessionRepo biz.TeamGraphSessionRepo, lg loggateway.Logger) *TeamGraphRunCoordinator {
	coord := NewTeamGraphRunCoordinator(graphs, teamRunReader, teamRunWriter, runTransitioner, bus, sessionRepo, nil, lg)
	interval := coord.cfg.CleanupInterval
	if interval <= 0 {
		interval = defaultCleanupInterval
	}
	safego.Go(appctx.Ctx(), "team.graph.coordinator.cleanup", func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			coord.CleanupStaleSessions()
		}
	})
	return coord
}

// ProviderSet wires team runtime.
var ProviderSet = wire.NewSet(NewRunner, ProvideTeamGraphRunCoordinator, NewTeamRunMediator)
