package team

import (
	"time"

	"aranea-agents/internal/biz"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/pkg/appctx"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	"github.com/google/wire"
)

// ProvideTeamGraphRunCoordinator wires a singleton coordinator for team graph HITL/resume
// and starts a background ticker that evicts sessions older than sessionMaxAge.
func ProvideTeamGraphRunCoordinator(graphs *biz.GraphUsecase, teamRunReader biz.TeamRunReader, teamRunWriter biz.TeamRunWriter, runTransitioner biz.TeamRunStatusTransitioner, eventBus biz.EventBus, seq rt.EventPublisher, sessionRepo biz.TeamGraphSessionRepo, lg loggateway.Logger) *TeamGraphRunCoordinator {
	coord := NewTeamGraphRunCoordinator(graphs, teamRunReader, teamRunWriter, runTransitioner, eventBus, seq, sessionRepo, nil, lg)
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
