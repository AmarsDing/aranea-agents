package biz

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz/session"
	"aranea-agents/internal/metrics"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// teamTimeoutRegistry tracks pending team timeout timers so they can be
// cancelled when a team reaches a terminal state before the timeout fires.
// Extracted from a raw sync.Map (AS-COG-01: sync.Map should be a named type).
type teamTimeoutRegistry struct {
	timers sync.Map // teamID → *time.Timer
}

func (r *teamTimeoutRegistry) store(teamID string, t *time.Timer) {
	r.timers.Store(teamID, t)
}

// claim removes the pending timer entry. Returns false when the team already
// completed (CancelTimeoutTimer won the race) so the timeout must not fire.
func (r *teamTimeoutRegistry) claim(teamID string) bool {
	_, loaded := r.timers.LoadAndDelete(teamID)
	return loaded
}

func (r *teamTimeoutRegistry) cancel(teamID string) {
	if v, ok := r.timers.LoadAndDelete(teamID); ok {
		if t, ok := v.(*time.Timer); ok {
			t.Stop()
		}
	}
}

func (r *teamTimeoutRegistry) stopAll() {
	r.timers.Range(func(key, value any) bool {
		r.timers.Delete(key)
		if t, ok := value.(*time.Timer); ok {
			t.Stop()
		}
		return true
	})
}

// SpiritOrchestration owns DAG scheduling, timeouts, completion, and team
// run-state for Spirit (DEV-09).
type SpiritOrchestration struct {
	teamUC             SpiritTeamAssembler
	sessionUC          SpiritSessionAccessor
	agentUC            SpiritAgentResolver
	orchCache          *OrchestrationCache
	evolutionSugg      EvolutionSuggestionCreator
	timeoutHandler     TimeoutHandler
	timeoutOnce        sync.Once
	timeouts           *teamTimeoutRegistry
	completionNotifier AllTeamsCompletedNotifier
	pollCtx            context.Context
	pollCancel         context.CancelFunc
	delivery           *SpiritDelivery
	graphSweeper       SpiritTeamGraphSweeper
	lg                 loggateway.Logger
	// P2-2（2026-09-03）：探测失败 strike 计数（teamID → 连续失败次数）。
	// 惰性初始化，直构 struct（测试）零值安全。
	probeStrikesMu   sync.Mutex
	probeFailStrikes map[string]int
}

// errNoTeamRunToProbe 是「已调度未启动」的确定性信号（P2-2）：团队 Running
// 但无任何运行记录——启动链断裂，重查不会改变结果。与 Temporal
// schedule-to-start 同语义：by design 快速失败，不进重试/顺延。
var errNoTeamRunToProbe = errors.New("team has no run session to probe")

// teamProbeMaxStrikes 是 infra 类探测失败的判死阈值：连续 3 次失败
// （≈3 个 idle 窗口的持续故障）fail-closed 判死，替代 P2-2 前的无限顺延
// （病态团队可活到进程重启）。期间任何一次成功探测即清零。
const teamProbeMaxStrikes = 3

func (o *SpiritOrchestration) noteProbeFailure(teamID string) int {
	o.probeStrikesMu.Lock()
	defer o.probeStrikesMu.Unlock()
	if o.probeFailStrikes == nil {
		o.probeFailStrikes = make(map[string]int)
	}
	o.probeFailStrikes[teamID]++
	n := o.probeFailStrikes[teamID]
	metrics.SpiritProbeStrikeTotal.Inc()
	return n
}

func (o *SpiritOrchestration) resetProbeFailures(teamID string) {
	o.probeStrikesMu.Lock()
	defer o.probeStrikesMu.Unlock()
	delete(o.probeFailStrikes, teamID)
}

// teamCreatedOlderThan 判定团队创建时间是否已超 given 时长。CreatedAt 缺失
// 或不可解析时返回 false——数据不全不判死。
func teamCreatedOlderThan(team Team, d time.Duration, now time.Time) bool {
	created, err := time.Parse(time.RFC3339, strings.TrimSpace(team.CreatedAt))
	if err != nil {
		return false
	}
	return now.Sub(created) > d
}

// SetTimeoutHandler injects the service-layer timeout handler.
// Called after construction to break the circular dependency:
// SpiritTeamUsecase → TimeoutHandler → TeamStarter → SpiritTeamController → SpiritTeamUsecase.
// This is a justified exception like L4GraphUsecase.SetCascade.
// Uses sync.Once to ensure the handler is set exactly once.
func (o *SpiritOrchestration) SetTimeoutHandler(h TimeoutHandler) {
	o.timeoutOnce.Do(func() {
		o.timeoutHandler = h
	})
}

// SetAllTeamsCompletedNotifier injects the service-layer completion notifier.
// Called by the background poller when all teams for a spirit session reach
// terminal state. This is the "active notification" path.
func (o *SpiritOrchestration) SetAllTeamsCompletedNotifier(n AllTeamsCompletedNotifier) {
	o.completionNotifier = n
}

// StartBackgroundPolling starts a background goroutine that periodically
// checks all active spirit sessions for team completion. This supplements
// the event-driven path (HandleTeamTurnResult) with a moderate-frequency
// backup to catch cases where completion events are missed.
//
// Default interval: 30 seconds. This is backend logic and does not generate
// frontend-visible activity events.
func (o *SpiritOrchestration) StartBackgroundPolling(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	o.pollCtx, o.pollCancel = context.WithCancel(ctx)
	safego.Go(o.pollCtx, "spirit-team-completion-poller", func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-o.pollCtx.Done():
				return
			case <-ticker.C:
				o.pollTeamCompletions(o.pollCtx)
			}
		}
	})
	o.lg.Info("spirit team completion poller started",
		loggateway.StepID("spirit.poller.started"),
		loggateway.Str("interval", interval.String()),
	)
}

// pollTeamCompletions scans all running sessions and checks if all their
// teams have reached terminal state. When all done, it notifies the service
// layer via completionNotifier.
//
// This is a moderate-frequency backup for the event-driven path
// (HandleTeamTurnResult → checkAllTeamsCompleted). The polling itself does
// not generate frontend-visible activity events.
//
// D3-FIX-1（87-orchestration-chain-hardening）：running 扫描之外追加
// interrupted 复活扫描——「持久失败类中断（error/timeout/server_shutdown
// /unexpected_shutdown）且所有团队已达终态」的精灵会话也需要一次完成
// 兜底通知，否则进程重启/超时收割后 synthesis 永久丢失（dedup/CAS/budget
// 仍在 service 侧守门，本函数只读不重入）。
func (o *SpiritOrchestration) pollTeamCompletions(ctx context.Context) {
	spiritAgentID := o.resolveSpiritAgentID(ctx)
	if spiritAgentID == "" {
		o.lg.Warn("spirit poller: spirit agent not found, skip scan",
			loggateway.StepID("spirit.poller.no_spirit_agent"),
		)
		return
	}
	// running 扫描（既有语义；分页满即止）。
	notifyIfAllDone := func(sessionID string) {
		result := o.CheckAllTeamsCompleted(ctx, sessionID)
		if !result.AllDone {
			return
		}
		o.lg.Info("spirit poller: all teams completed for session",
			loggateway.StepID("spirit.poller.all_done"),
			loggateway.Str("spirit_session_id", sessionID),
			loggateway.Int("total_teams", result.TotalTeams),
		)
		if o.completionNotifier != nil {
			o.completionNotifier.NotifyAllTeamsCompleted(ctx, sessionID)
		}
	}
	for page := 0; page < SpiritPollerMaxPages; page++ {
		sessions, err := o.sessionUC.Search(ctx, SessionSearchQuery{
			Status:   string(session.SessionStatusRunning),
			AgentID:  spiritAgentID,
			RootOnly: true,
			Limit:    SpiritPollerPageSize,
			Offset:   page * SpiritPollerPageSize,
		})
		if err != nil {
			o.lg.Warn("spirit poller: failed to search running sessions",
				loggateway.StepID("spirit.poller.search_err"),
				loggateway.Err(err),
			)
			return
		}
		for _, sess := range sessions.Items {
			if sess.ID == "" {
				continue
			}
			notifyIfAllDone(sess.ID)
		}
		if len(sessions.Items) < SpiritPollerPageSize {
			break
		}
	}
	// interrupted 复活扫描（D3-FIX-1）：白名单中断原因重试，其余
	// （user_cancelled/user_escalated/manual_override/clarification
	// /context_overflow 等用户意图/人工介入类）跳过。
	for page := 0; page < SpiritPollerMaxPages; page++ {
		sessions, err := o.sessionUC.Search(ctx, SessionSearchQuery{
			Status:   string(session.SessionStatusInterrupted),
			AgentID:  spiritAgentID,
			RootOnly: true,
			Limit:    SpiritPollerPageSize,
			Offset:   page * SpiritPollerPageSize,
		})
		if err != nil {
			o.lg.Warn("spirit poller: failed to search interrupted sessions",
				loggateway.StepID("spirit.poller.search_interrupted_err"),
				loggateway.Err(err),
			)
			return
		}
		for _, sess := range sessions.Items {
			if sess.ID == "" || !isRevivableInterruptedReason(sess.StatusReason) {
				continue
			}
			notifyIfAllDone(sess.ID)
		}
		if len(sessions.Items) < SpiritPollerPageSize {
			break
		}
	}
}

// revivableInterruptedReasons 是 D3-FIX-1 的复活白名单：仅持久失败类中断
// 原因可复活重试（进程重启/超时收割/运行错误）；用户/澄清类中断属于用户
// 意图，不得复活。显式集合（白名单），新中断原因默认不复活。
var revivableInterruptedReasons = map[session.SessionStatusReason]struct{}{
	session.StatusReasonError:              {},
	session.StatusReasonTimeout:            {},
	session.StatusReasonServerShutdown:     {},
	session.StatusReasonUnexpectedShutdown: {},
}

func isRevivableInterruptedReason(reason string) bool {
	_, ok := revivableInterruptedReasons[session.SessionStatusReason(reason)]
	return ok
}

// resolveSpiritAgentID returns the catalog ID of the built-in spirit agent.
// Empty string means the poller must not scan (fail-closed: never iterate
// every running chat session).
func (o *SpiritOrchestration) resolveSpiritAgentID(ctx context.Context) string {
	if o.agentUC == nil {
		return ""
	}
	agents, err := o.agentUC.List(ctx, AgentListQuery{Keyword: SpiritAgentKey, Limit: SpiritAgentQueryLimit})
	if err != nil || len(agents.Items) == 0 {
		return ""
	}
	return strings.TrimSpace(agents.Items[0].ID)
}

func (o *SpiritOrchestration) GetMaxParallelTeams(ctx context.Context, spiritSessionID string) int {
	cfg := o.resolveParallelConfig(ctx, spiritSessionID)
	return cfg.MaxConcurrentTeams
}

func (o *SpiritOrchestration) GetParallelConfig(ctx context.Context, spiritSessionID string) ParallelConfig {
	return o.resolveParallelConfig(ctx, spiritSessionID)
}

func (o *SpiritOrchestration) resolveParallelConfig(ctx context.Context, spiritSessionID string) ParallelConfig {
	if o.agentUC == nil {
		return DefaultParallelConfig()
	}
	agents, err := o.agentUC.List(ctx, AgentListQuery{Keyword: SpiritAgentKey, Limit: SpiritAgentQueryLimit})
	if err != nil {
		o.lg.Error("查询精灵 Agent 失败，使用默认并行配置（用户自定义配置将失效）",
			loggateway.StepID("spirit.parallel_config"),
			loggateway.Str("spirit_session_id", spiritSessionID),
			loggateway.Err(err),
		)
		return DefaultParallelConfig()
	}
	if len(agents.Items) == 0 {
		o.lg.Error("精灵 Agent 不存在，使用默认并行配置（用户自定义配置将失效）",
			loggateway.StepID("spirit.parallel_config"),
			loggateway.Str("spirit_session_id", spiritSessionID),
		)
		return DefaultParallelConfig()
	}
	ag := agents.Items[0]

	return ParseParallelConfig(ag.ConfigJSON, o.lg)
}

// Domain: Orchestration — cancel team and its timeout timer.
// reason 是 P2-6 取消原因（空 = user_cancel，保持向后兼容）。
func (o *SpiritOrchestration) CancelTeam(ctx context.Context, teamID string, reason CancelReason) error {
	if strings.TrimSpace(teamID) == "" {
		return apierror.BadRequest("SPIRIT", "team_id is required")
	}
	if reason == "" {
		reason = CancelReasonUser
	}
	o.CancelTimeoutTimer(teamID)
	_, err := o.teamUC.TransitionStatus(ctx, teamID, TeamStatusCancelled)
	if err != nil {
		return err
	}
	return nil
}

// CancelTimeoutTimer stops the timeout timer for a team if one is pending.
// Should be called when a team reaches a terminal state (completed/failed/cancelled)
// to prevent the timeout callback from firing unnecessarily.
func (o *SpiritOrchestration) CancelTimeoutTimer(teamID string) {
	o.timeouts.cancel(teamID)
}

// RegisterTeamTimeout starts the AfterFunc clock for a team that has just
// begun executing. No-op when TeamTimeoutSeconds <= 0.
func (o *SpiritOrchestration) RegisterTeamTimeout(ctx context.Context, team Team) {
	if strings.TrimSpace(team.ID) == "" {
		return
	}
	cfg := o.resolveParallelConfig(ctx, team.SpiritSessionID)
	o.registerTeamTimeout(ctx, cfg, team.ID)
}

// Stop cancels all pending timeout timers and the background polling goroutine.
// Call during application shutdown to prevent callbacks from firing after the
// server has stopped.
func (o *SpiritOrchestration) Stop() {
	if o.pollCancel != nil {
		o.pollCancel()
	}
	if o.timeouts != nil {
		o.timeouts.stopAll()
	}
	o.clearProbeStrikes()
}

// ProbeStrikeCount returns the consecutive probe-failure count for teamID
// (ORCH-05 observability). Zero when unknown.
func (o *SpiritOrchestration) ProbeStrikeCount(teamID string) int {
	if o == nil || teamID == "" {
		return 0
	}
	o.probeStrikesMu.Lock()
	defer o.probeStrikesMu.Unlock()
	return o.probeFailStrikes[teamID]
}

func (o *SpiritOrchestration) clearProbeStrikes() {
	if o == nil {
		return
	}
	o.probeStrikesMu.Lock()
	defer o.probeStrikesMu.Unlock()
	o.probeFailStrikes = make(map[string]int)
}
