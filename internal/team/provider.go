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
			// P0 终态一致性：先收割丢失的 running run（watch 死亡导致永不收口），
			// 再做 P3-3 挂起与硬清理——硬清理只驱逐会话不 finalize，必须把
			// reconcile 排在它前面（阈值 45m < SessionMaxAge 2h 保证先触发）。
			coord.ReconcileStaleRuns(appctx.Ctx(), time.Now(), 0)
			// P3-3 (ADR-D): suspend idle waiting_human sessions first (memory
			// evict, DB retained), then hard-clean sessions stale beyond maxAge.
			coord.SuspendIdleWaits(time.Now(), coord.cfg.SuspendIdleThreshold)
			coord.CleanupStaleSessions()
		}
	})
	return coord
}

// ProviderSet wires team runtime.
var ProviderSet = wire.NewSet(NewRunner, ProvideTeamGraphRunCoordinator, NewTeamRunMediator)