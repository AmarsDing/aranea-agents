package team

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/decision"
	"aranea-agents/internal/sandbox"
	"aranea-agents/pkg/loggateway"
)

// 83-长时运行韧性：TeamRun 崩溃续跑（checkpoint-based crash resume）。
// 本文件集中 RecoverSessions / tryResumeOrphanedRun 与 startupResumed marker
// 查询，从 team_graph_run_coordinator.go 拆出（AS-COG-01 行数纪律）。

// SetCrashResumeEnabled toggles checkpoint-based crash resume for orphaned
// running team runs (83). Wired from TEAM_RUN_CRASH_RESUME_DISABLED at startup.
func (c *TeamGraphRunCoordinator) SetCrashResumeEnabled(enabled bool) {
	if c == nil {
		return
	}
	c.crashResumeEnabled = enabled
}

// WasStartupResumed implements biz.TeamRunStartupResumeMarker：报告 run 是否在
// 本次启动对账中已从 checkpoint 成功续跑（team 级判死据此跳过）。
func (c *TeamGraphRunCoordinator) WasStartupResumed(runID string) bool {
	if c == nil {
		return false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.startupResumed[strings.TrimSpace(runID)]
	return ok
}

// RecoverSessions rebuilds in-memory sessions from DB after process restart (BL-04b).
// Waiting_human sessions are re-registered so that HITL task completion can resume them.
// Running sessions (83-长时运行韧性)：默认逐个尝试从 graph checkpoint 崩溃续跑
// （RecoverOrphanedExecution 安全闸：无 checkpoint / 定义 hash 变更拒绝），成功则
// 重建 watch 并记入 startupResumed（team 级判死跳过）；失败回退 finalizeTeamRun
// (failed)。开关关闭或恢复路径外的残留 running 行由 MarkOrphanedSessionsTerminal
// 后置兜底清理（旧行为）。
func (c *TeamGraphRunCoordinator) RecoverSessions(ctx context.Context) {
	if c == nil || c.sessionRepo == nil {
		return
	}
	active, err := c.sessionRepo.ListActiveSessions(ctx)
	if err != nil {
		c.lg.Warn("RecoverSessions: ListActiveSessions failed",
			loggateway.StepID("team.session.recover_fail"),
			loggateway.Err(err))
		return
	}
	recovered := 0
	resumed := 0
	for _, dbSess := range active {
		if dbSess.Status == biz.TeamRunStatusRunning {
			// 开关关闭时走旧判死路径（下方 MarkOrphanedSessionsTerminal 兜底）。
			if c.crashResumeEnabled && c.graphs != nil {
				if c.tryResumeOrphanedRun(ctx, dbSess) {
					resumed++
				}
			}
			continue
		}
		sess := c.restoreSession(dbSess)
		c.mu.Lock()
		c.sessions[dbSess.ExecID] = sess
		c.mu.Unlock()
		if dbSess.Status == biz.TeamRunStatusWaitingHuman {
			c.startCompletionWatch(ctx, sess)
		}
		recovered++
	}
	// 兜底：仍残留的 running 会话行（开关关闭 / 恢复失败未走 finalize 的边角）。
	cancelled, err := c.sessionRepo.MarkOrphanedSessionsTerminal(ctx)
	if err != nil {
		c.lg.Warn("RecoverSessions: MarkOrphanedSessionsTerminal failed",
			loggateway.StepID("team.session.recover_fail"),
			loggateway.Err(err))
	}
	if cancelled > 0 {
		c.lg.Warn("RecoverSessions: cancelled orphaned running sessions",
			loggateway.StepID("team.session.orphan_cancelled"), // 原 step_id 保留
			loggateway.Int("count", cancelled))
	}
	if recovered > 0 || resumed > 0 {
		c.lg.Warn("RecoverSessions: recovered sessions from DB",
			loggateway.StepID("team.session.recovered"),
			loggateway.Int("recovered", recovered),
			loggateway.Int("crash_resumed", resumed))
	}
}

// tryResumeOrphanedRun 尝试从 checkpoint 续跑一个孤儿 running 会话（83-长时运行
// 韧性）。成功：重建内存会话 + watch + startupResumed 记忆，返回 true；失败：
// finalizeTeamRun(failed)（幂等收敛 graph_executions + 删除会话行），返回 false。
func (c *TeamGraphRunCoordinator) tryResumeOrphanedRun(ctx context.Context, dbSess biz.TeamGraphSession) bool {
	sess := c.restoreSession(dbSess)
	// 79-runtime-governance 同款重注（team_resume.go:43-48）：恢复执行的成员闸
	// 事件 run 归属 + 沙箱创建预算 run 口径，否则随原执行终结丢失。
	resumeCtx := decision.WithGateRunID(ctx, dbSess.TeamRunID)
	resumeCtx = decision.WithGateSessionID(resumeCtx, dbSess.SessionID)
	resumeCtx = sandbox.WithRunID(resumeCtx, dbSess.TeamRunID)
	if _, err := c.graphs.RecoverOrphanedExecution(resumeCtx, dbSess.ExecID); err != nil {
		c.lg.Warn("RecoverSessions: crash resume failed, finalize as failed",
			loggateway.StepID("team.session.crash_resume_fail"),
			loggateway.Str("exec_id", dbSess.ExecID),
			loggateway.Str("team_run_id", dbSess.TeamRunID),
			loggateway.Err(err))
		c.finalizeTeamRun(ctx, sess, true, fmt.Sprintf("crash resume failed: %v", err))
		return false
	}
	c.mu.Lock()
	c.sessions[dbSess.ExecID] = sess
	c.startupResumed[strings.TrimSpace(dbSess.TeamRunID)] = struct{}{}
	c.mu.Unlock()
	// 续跑成功视同新活动：防止 lastActivityAt 沿用崩溃前时间被 stale 对账误杀。
	c.touchSessionActivity(dbSess.ExecID)
	c.startCompletionWatch(ctx, sess)
	c.lg.Info("RecoverSessions: resumed orphaned running session from checkpoint",
		loggateway.StepID("team.session.crash_resumed"),
		loggateway.Str("exec_id", dbSess.ExecID),
		loggateway.Str("team_run_id", dbSess.TeamRunID))
	return true
}
