package service

import (
	"context"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	sessstatus "aranea-agents/internal/biz/session"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/internal/tools"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
)

var (
	_ tools.SpiritTeamAssemblerPort  = (*SpiritTeamAssembler)(nil)
	_ tools.SpiritTeamQueryPort      = (*SpiritTeamAssembler)(nil)
	_ tools.SpiritTeamControllerPort = (*SpiritTeamAssembler)(nil)
	_ biz.TeamStarterPort            = (*TeamStarter)(nil)
	_ biz.TimeoutHandler             = (*TeamStarter)(nil)
	_ biz.AllTeamsCompletedNotifier  = (*TeamStarter)(nil)
)

// TeamStarter manages team turn lifecycle.
//
// 事件总线：`eventBus`/`seq` 发布 team_stage 与 notice。
// Chat Graph 块由 v2 GraphStage/GraphNode（PublishV2Board + PlanExecutor）维护，
// 不再注入 v1 ActivityEventBus。
//
// 字段数 15（D2-②+D3-FIX-3 后为 AS-COG-01 上限边缘；再增字段前应先提取
// outcome/notice 子结构，勿直接堆字段）。
type TeamStarter struct {
	sessions *biz.SessionUsecase
	team     TeamOrchestrationDeps
	eventBus biz.EventBus      // v2: team_stage + notice events
	seq      rt.EventPublisher // v2 Sequencer（FIFO + retry）
	lg       loggateway.Logger
	// turnGateway is used to inject a synthetic message into the Spirit
	// session when all teams complete, triggering the LLM to continue
	// with synthesis. This replaces the LLM-polling pattern (check_progress
	// tool) with a system-push pattern.
	turnGateway biz.TurnGateway
	// synthesisTriggered guards against concurrent duplicate synthesis
	// triggers per spirit session. checkAllTeamsCompleted is called from
	// both HandleTeamTurnResult (goroutine) and NotifyAllTeamsCompleted
	// (background poller goroutine). LoadOrStore ensures only one synthesis
	// message is injected per orchestration round.
	// Keyed by spiritSessionID so concurrent spirit sessions don't interfere.
	// The guard is reset in StartTeamTurn when a new round begins (team
	// (re)start), so round-N+1 completion can trigger synthesis again.
	synthesisTriggered sync.Map

	// planExecutor is the v2 forward DAG scheduler that replaces the
	// reverse-sync updatePlanStepForTeam method. Injected via SetPlanExecutor
	// (post-construction) to avoid a Wire cycle: PlanExecutor's TeamOrchestrator
	// dependency is satisfied by SpiritTeamAssembler, which itself depends on
	// TeamStarter. May be nil in v1-only deployments.
	planExecutor *PlanExecutor
	// taskV2Reader is used by resolveLatestUserTaskID to look up the most
	// recent user-input Task for the spirit session, so the system-push
	// synthesis Turn can be attached to that existing Task (ParentTaskID)
	// instead of creating a new Task. Design: 2026-07-02-llm-activity-
	// ordering-design §3.2.1.
	taskV2Reader biz.TaskV2Reader
	// 2026-07-05 P1 #9d-2（AS-FSM-01）：TeamStage 状态机 + Reader，用于修复
	// spirit_team.go 中 5 处 Version=0 Bug（状态未持久化）+ 添加状态机校验。
	teamStageR biz.TeamStageV2Reader
	tsSM       *biz.TeamStageStateMachine
	// 2026-07-05 P1 #9e（AS-FSM-01）：TeamRun v2 状态机 + Reader，用于修复
	// publishV2TeamRunCompletion 中 Version=2 硬编码 Bug + 添加状态机校验。
	teamRunR biz.TeamRunV2Reader
	trSM     *biz.TeamRunV2StateMachine

	// runGuard 是 dispatch/调度入口的运行互斥护栏（D2-②，87-orchestration-
	// chain-hardening）：与 chat 入口（executeTeamTurnViaHooks）共享同一
	// RunRegistry + 会话互斥锁，消除调度链与 chat 入口对同一 team session
	// 的双跑。ProvideChatService（chat_wire.go）在 SetTurnGateway 回填后
	// 后注入；nil（单测/v1-only）走 legacy 路径（TransitionStatus 中止语义
	// 无例外生效）。
	runGuard *TeamRunGuard

	// synthesisAttempts 记录每 spirit session 的 synthesis 同步失败重试次数
	// （D3-FIX-3 ④ 复活预算）：ExecuteTurn 同步失败释放 CAS 后允许的廉价
	// 重试上限（reviveMaxAttempts=3）；StartTeamTurn 新一轮删除解锁。
	// 零值可直接使用；与 synthesisTriggered 同 keyed，wire 无需改。
	synthesisAttempts sync.Map
}

// TeamRunGuard 是 TeamStarter 的可选运行护栏（D2-②）：由 chat_wire.go 在
// ProvideChatService 后注入（与 SetTurnGateway 同一接口断言模式），不进入
// TeamStarterPort 接口，5 个调用方零改动。
type TeamRunGuard struct {
	// Runs 与 chat 入口共享的运行注册表；锁内 HasActive 判定 busy skip
	// （INV-M4：busy skip 返回 nil，不是错误）。
	Runs *rt.RunRegistry
	// Lock 返回会话互斥的解锁函数（ChatOrchestrator.lockSession 语义）。
	Lock func(sessionID string) (unlock func())
	// DrainPending 在 turn 结束后排空会话排队队列（可为 nil → 跳过）。
	// 生产回填为 ChatOrchestrator.drainPendingAfterDispatchTeamTurn。
	DrainPending func(sessionID string)
}

// SetRunGuard 后注入运行护栏（打破 Wire 循环；与 SetTurnGateway 同模式）。
func (s *TeamStarter) SetRunGuard(g *TeamRunGuard) {
	s.runGuard = g
}

func NewTeamStarter(
	sessions *biz.SessionUsecase,
	team TeamOrchestrationDeps,
	eventBus biz.EventBus,
	seq rt.EventPublisher,
	lg loggateway.Logger,
	taskV2Reader biz.TaskV2Reader,
	teamStageR biz.TeamStageV2Reader,
	teamRunR biz.TeamRunV2Reader,
) *TeamStarter {
	return &TeamStarter{
		sessions:     sessions,
		team:         team,
		eventBus:     eventBus,
		seq:          seq,
		lg:           lg,
		taskV2Reader: taskV2Reader,
		teamStageR:   teamStageR,
		tsSM:         biz.NewTeamStageStateMachine(),
		teamRunR:     teamRunR,
		trSM:         biz.NewTeamRunV2StateMachine(),
	}
}

// SetTurnGateway injects the turn gateway after construction to break the
// Wire cycle: ChatService → TeamStarterPort → TurnGateway → ChatService.
// turnGateway is used in checkAllTeamsCompleted for the system-push pattern
// (synthesize results → inject message into Spirit session).
func (s *TeamStarter) SetTurnGateway(gw biz.TurnGateway) {
	s.turnGateway = gw
}

// SetPlanExecutor injects the v2 forward DAG scheduler after construction.
// This breaks a Wire cycle: PlanExecutor → TeamOrchestrator → SpiritTeamAssembler
// → TeamStarter. May be nil in v1-only deployments (the reverse-sync
// updatePlanStepForTeam method has been removed; v1 plan steps are no longer
// auto-updated by team completion — the v2 PlanExecutor handles this when wired).
func (s *TeamStarter) SetPlanExecutor(pe *PlanExecutor) {
	s.planExecutor = pe
}

// publishV2Event publishes a v2 event via seq (DB persist + WS) when available,
// falling back to eventBus (WS only) otherwise.
// 2026-07-04 问题 2 修复：之前所有 TeamStage/Step 事件用 eventBus.Publish，
// 不经过 sequencer.persistAction，刷新后数据丢失。改为优先用 seq.Publish
// 确保落库；v1-only 部署（seq=nil）保留 eventBus 兜底。
func (s *TeamStarter) publishV2Event(ctx context.Context, e biz.Event) {
	if s.seq != nil {
		s.seq.Publish(ctx, e)
		return
	}
	if s.eventBus != nil {
		s.eventBus.Publish(ctx, e)
	}
}

// publishV2EventAssembler is the SpiritTeamAssembler counterpart of
// publishV2Event. Same semantics: seq first (persist + WS), eventBus fallback.
func (a *SpiritTeamAssembler) publishV2Event(ctx context.Context, e biz.Event) {
	if a.seq != nil {
		a.seq.Publish(ctx, e)
		return
	}
	if a.eventBus != nil {
		a.eventBus.Publish(ctx, e)
	}
}

// v2EventReady returns true if either seq or eventBus is wired.
// Used to guard TeamStage/Step publish blocks (replacing bare eventBus nil-check).
func (s *TeamStarter) v2EventReady() bool {
	return s.seq != nil || s.eventBus != nil
}

// v2EventReady is the SpiritTeamAssembler counterpart.
func (a *SpiritTeamAssembler) v2EventReady() bool {
	return a.seq != nil || a.eventBus != nil
}

func (s *TeamStarter) StartTeamTurn(ctx context.Context, sessionID string, content string) error {
	if s.team.TeamsNative == nil {
		return apierror.Internal("SPIRIT", "team runner not available")
	}
	// sess 查询保持锁前语义：查询失败 → 404 原样返回，不占会话互斥。
	sess, err := s.sessions.Get(ctx, sessionID)
	if err != nil {
		return apierror.NotFound("SPIRIT", "team session not found")
	}

	// D2-②（87-orchestration-chain-hardening）：dispatch/调度入口接入运行
	// 注册 + 会话互斥，与 chat 入口（executeTeamTurnViaHooks）共享同一
	// RunRegistry——消除「调度链与 chat 双跑同一 team session」（双 runner
	// 互踩 v2 版本守卫/终态守卫）。busy skip 返回 nil（INV-M4：重复派单不是
	// 错误，5 个调用方零改动）；guard 为 nil（单测/v1-only）走 legacy 路径。
	guard := s.runGuard
	unlock := func() {}
	if guard != nil && guard.Lock != nil {
		unlock = guard.Lock(sessionID)
	}
	// once 化解锁：busy/skip 提前路径与注册后释放路径都会解锁，defer 兜底
	// panic 路径不泄漏锁。
	var unlockOnce sync.Once
	doUnlock := func() { unlockOnce.Do(unlock) }
	defer doUnlock()

	if guard != nil && guard.Runs != nil && guard.Runs.HasActive(sessionID) {
		doUnlock()
		s.lg.Info("team session 已有活跃 run（chat/调度双跑防护），跳过重复启动",
			loggateway.StepID("spirit.start_team_busy"),
			loggateway.Str("session_id", sessionID),
		)
		return nil
	}
	// legacy skip 语义不变（移至锁内：会话层与注册表双重在跑判定同窗口）。
	if sess.Status == string(sessstatus.SessionStatusRunning) {
		doUnlock()
		s.lg.Info("团队 session 已在运行中，跳过重复启动",
			loggateway.StepID("spirit.start_team_skip"),
			loggateway.Str("session_id", sessionID),
		)
		return nil
	}

	// D2-②：会话 running 转移失败不再沉默续跑（失败即存在并发写者——
	// 继续启动 = 调度/chat 双跑的另一形态），apierror 类型保持原样返回中止。
	if err := s.sessions.TransitionStatus(ctx, sessionID, sessstatus.SessionStatusRunning, ""); err != nil {
		s.lg.Warn("团队 Session 状态转换到 running 失败，中止启动",
			loggateway.StepID("spirit.transition_running_err"),
			loggateway.Str("session_id", sessionID),
			loggateway.Err(err),
		)
		return err
	}

	spiritSessionID := strings.TrimSpace(sess.ParentSessionID)
	teamID := strings.TrimSpace(sess.TeamID)

	// 团队（重）启动 = 新一轮编排进行中：重置 synthesis CAS 守卫，使本轮
	// 全部完成后能再次触发 synthesis。否则同一 spirit session 的第二轮编排
	// （或失败团队重试）完成后 guard 永久拦截，synthesis turn 不再注入，
	// 父 Task 将永远停在 Running（task.completed 延迟到 synthesis turn）。
	// D3-FIX-3 ⑤：CAS 重置同时清空复活预算——新一轮可重新计数重试。
	if spiritSessionID != "" {
		s.synthesisTriggered.Delete(spiritSessionID)
		s.synthesisAttempts.Delete(spiritSessionID)
	}

	// Fetch the team so we can include team_name in team_stage events. Without
	// team_name, the frontend TeamCard's displayTeamName degrades to generic
	// status text ("assembling"). The fetch is cheap (single DB row by ID).
	var team biz.Team
	if teamID != "" {
		var updateErr error
		team, updateErr = s.team.TeamUC.TransitionStatus(ctx, teamID, biz.TeamStatusRunning)
		if updateErr != nil {
			s.lg.Warn("更新团队状态为 running 失败",
				loggateway.StepID("spirit.team.running_err"),
				loggateway.Str("team_id", teamID),
				loggateway.Err(updateErr),
			)
			// Fallback: try plain Get so we still have DisplayName for events.
			if t, getErr := s.team.TeamUC.Get(ctx, teamID); getErr == nil {
				team = t
			}
		}
	}

	if s.v2EventReady() && teamID != "" && spiritSessionID != "" {
		// Phase 3b-D Task 10: migrated to v2 NewTeamStageUpdatedEvent.
		// DependsOn is carried by the TeamStage entity (type-safe).
		// 2026-07-04 问题 3 修复：携带 TeamName 让前端展示团队名称而非 ID；
		// 同时把 Stage 从字面量 "progress"（不在枚举内）改为枚举常量
		// TeamStageStageExecuting。
		// 2026-07-04 问题 2 修复：改用 publishV2Event（seq 优先持久化）。
		// 2026-07-05 P1 #9d-2（AS-FSM-01）：用状态机校验 Pending → Running 转换，
		// 修复 Version=0 Bug（状态未持久化）。
		dependsOn := team.DependsOn
		tsID := string(agent.NewTeamStageActivityID(teamID, string(agent.RootTaskActivityIDFromCtx(ctx))))
		newStatus, newVersion, tsOK := resolveTeamStageUpdate(ctx, s.teamStageR, s.tsSM,
			tsID, biz.TeamStageEventStart, biz.TeamStageStatusRunning, s.lg)
		if tsOK {
			ts := biz.TeamStage{
				ID:     tsID,
				TeamID: teamID,
				// 2026-07-06 修复：补全 TaskID。之前 StartTeamTurn 创建 TeamStage 时
				// 未设置 TaskID（仅 publishSpiritTeamAssembled 设置了），但
				// resolveTeamStageUpdate fallback 会返回 Version=100，大于
				// publishSpiritTeamAssembled 的 Version=1，导致 UpsertTeamStage
				// 通过 SetTaskID("") 覆盖已有 TaskID，前端 listTeamStagesV2(taskId)
				// 查询返回空，team 面板无法渲染。
				// ctx 中携带 RootTaskActivityID 由 PlanExecutor 注入（plan_executor.go
				// ContextWithRootTaskActivityID），context.WithoutCancel 保留该值。
				TaskID:    string(agent.RootTaskActivityIDFromCtx(ctx)),
				TeamName:  team.DisplayName,
				SessionID: spiritSessionID,
				Status:    newStatus,
				Stage:     biz.TeamStageStageExecuting,
				DependsOn: dependsOn,
				StartedAt: time.Now().UTC(),
				Version:   newVersion,
			}
			s.publishV2Event(ctx, biz.NewTeamStageUpdatedEvent(ts))
		}
		// P0-2：tsOK=false（状态机拒绝，当前 TeamStage 已终态）时跳过发布，
		// 不覆盖终态；turn 启动主流程继续。
	}

	if s.team.SpiritUC != nil && teamID != "" {
		s.team.SpiritUC.RegisterTeamTimeout(ctx, team)
	}

	input := biz.TurnInput{
		SessionID: sessionID,
		Content:   content,
	}

	turnCtx := ctx
	cancel := context.CancelFunc(func() {})
	if spiritSessionID != "" && s.team.SpiritUC != nil {
		resolvedCfg := s.team.SpiritUC.GetParallelConfig(ctx, spiritSessionID)
		// F1（2026-09-03 lbg-verify-planner 复盘）：turn ctx 改用绝对存活上限
		// （maxLifetime，默认 4h）而非 idle 窗口——600s wall-clock 曾在成员
		// LLM 首字节重试预算（~213s）未耗尽时掐断执行中团队。成员级活性由
		// runner 守卫链（first-byte/stall/no-progress/token budget）负责，
		// 团队级 idle 判死由 biz AfterFunc 探测负责；此处仅作 goroutine
		// 泄漏的绝对兜底。
		if timeout := resolvedCfg.TeamMaxLifetime(); timeout > 0 {
			turnCtx, cancel = context.WithTimeout(ctx, timeout)
		}
	}

	// 锁内注册可取消运行实体（次序与 chat 入口 chat_orchestrator_turn.go
	// 一致：StoreCancelable → unlock → RunTurnFromInput）——注册前锁内
	// HasActive 判定关闭 chat/dispatch 交错 TOCTOU。
	runID := uuid.NewString()
	if guard != nil && guard.Runs != nil {
		guard.Runs.StoreCancelable(sessionID, runID, cancel)
	}
	doUnlock()
	defer func() {
		if guard != nil && guard.Runs != nil {
			guard.Runs.Finish(sessionID, runID)
		}
		cancel()
		// D2-②：turn 结束后排空排队队列（调度入口此前无人排干——排队消息
		// 与「稍后执行」类消息丢失）。
		if guard != nil && guard.DrainPending != nil {
			guard.DrainPending(sessionID)
		}
	}()

	_, _, err = s.team.TeamsNative.RunTurnFromInput(turnCtx, sess, input)

	if err != nil {
		// D1-F1（87-orchestration-chain-hardening）：用户停止导致的 runner
		// 中断（与 runWasCancelled 同一判别 isUserCancelledErr）→ 会话落
		// interrupted("user_cancelled")、团队终态回调 cancelled，errMsg 保留
		// 原文（含 "context canceled" 信号供下游分类器）。CancelledTeams>0 →
		// checkAllTeamsCompleted 跳过 synthesis（中断不出报告）。终态落账
		// 一律在 context.WithoutCancel 之后的上下文执行，取消不传染持久化。
		cancelled := isUserCancelledErr(turnCtx, err)
		bookCtx := context.WithoutCancel(ctx)
		reason := sessstatus.StatusReasonError
		if cancelled {
			reason = sessstatus.StatusReasonUserCancelled
		}
		if transErr := s.sessions.TransitionStatus(bookCtx, sessionID, sessstatus.SessionStatusInterrupted, reason); transErr != nil {
			s.lg.Warn("团队 Session 状态转换到 interrupted 失败",
				loggateway.StepID("spirit.transition_interrupted_err"),
				loggateway.Str("session_id", sessionID),
				loggateway.Err(transErr),
			)
		}
		s.lg.Warn("自动启动团队 Turn 失败",
			loggateway.StepID("spirit.start_team_fail"),
			loggateway.Str("session_id", sessionID),
			loggateway.Bool("user_cancelled", cancelled),
			loggateway.Err(err),
		)
		if spiritSessionID != "" && teamID != "" {
			teamStatus := biz.TeamStatusFailed
			if cancelled {
				teamStatus = biz.TeamStatusCancelled
			}
			s.HandleTeamTurnResult(bookCtx, spiritSessionID, teamID, teamStatus, err.Error(), "")
		}
		return err
	}

	if err := s.sessions.TransitionStatus(ctx, sessionID, sessstatus.SessionStatusCompleted, ""); err != nil {
		s.lg.Warn("团队 Session 状态转换到 completed 失败",
			loggateway.StepID("spirit.transition_completed_err"),
			loggateway.Str("session_id", sessionID),
			loggateway.Err(err),
		)
	}
	if spiritSessionID != "" && teamID != "" {
		s.HandleTeamTurnResult(ctx, spiritSessionID, teamID, biz.TeamStatusCompleted, "", "")
	}
	return nil
}
