package team

import (
	"os"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/decision"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/pkg/appctx"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	"github.com/google/wire"
)

// ProvideTeamGraphRunCoordinator wires a singleton coordinator for team graph HITL/resume
// and starts a background ticker that evicts sessions older than sessionMaxAge.
func ProvideTeamGraphRunCoordinator(graphs *biz.GraphUsecase, teamRunReader biz.TeamRunReader, teamRunWriter biz.TeamRunWriter, runTransitioner biz.TeamRunStatusTransitioner, eventBus biz.EventBus, seq rt.EventPublisher, sessionRepo biz.TeamGraphSessionRepo, flowLog biz.FlowLogWriter, decisions decision.Collector, lg loggateway.Logger) *TeamGraphRunCoordinator {
	coord := NewTeamGraphRunCoordinator(graphs, teamRunReader, teamRunWriter, runTransitioner, eventBus, seq, sessionRepo, nil, lg)
	// 83 §4.1：崩溃续跑审计通道（flowlog + decision 双写）。
	coord.SetRecoveryAudit(flowLog, decisions)
	// 83-长时运行韧性：TEAM_RUN_CRASH_RESUME_DISABLED=1 时启动对账回退旧判死路径。
	// 启动期读取一次注入，不在热路径重复读 env。
	if os.Getenv("TEAM_RUN_CRASH_RESUME_DISABLED") == "1" {
		coord.SetCrashResumeEnabled(false)
		lg.Warn("team graph crash resume disabled via TEAM_RUN_CRASH_RESUME_DISABLED",
			loggateway.StepID("team.session.crash_resume_disabled"))
	}
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