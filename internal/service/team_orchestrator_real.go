package service

import (
	"context"
	"sync"

	"aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// RealTeamOrchestrator bridges PlanExecutor to SpiritTeamAssembler.
//
// 2026-07-04 问题 4 修复：实现真实的 TeamOrchestrator，替代 Phase 1 的 stub。
//   - Orchestrate: 调用 SpiritTeamAssembler.AssembleTeam 创建 team + session,
//     然后手动启动 team_run（AutoStart=false 避免 channel 未就绪的竞态），
//     返回 channel 等待 team_run 完成。
//   - NotifyTeamCompletion: 由 TeamStarter.HandleTeamTurnResult 通过
//     PlanExecutor.NotifyTeamCompletion 转发调用，发送 TeamCompleteEvent 到 channel.
//
// 设计参考：docs/superpowers/specs/2026-07-02-llm-activity-ordering-design.md
// §3.5.2 执行流程
type RealTeamOrchestrator struct {
	assembler *SpiritTeamAssembler
	starter   biz.TeamStarterPort
	agentRdr  biz.AgentReader
	pending   sync.Map // teamID → pendingTeamCompletion
	lg        loggateway.Logger
}

type pendingTeamCompletion struct {
	ch     chan biz.TeamCompleteEvent
	stepID string
}

// NewRealTeamOrchestrator constructs a RealTeamOrchestrator.
// assembler 和 starter 通过后注入（SetAssembler/SetStarter）打破 Wire 循环：
// PlanExecutor → TeamOrchestrator → SpiritTeamAssembler → TeamStarter → PlanExecutor。
func NewRealTeamOrchestrator(lg loggateway.Logger) *RealTeamOrchestrator {
	return &RealTeamOrchestrator{
		lg: lg.With(loggateway.Domain("team_orchestrator")),
	}
}

// SetAssembler injects the SpiritTeamAssembler after construction.
func (o *RealTeamOrchestrator) SetAssembler(a *SpiritTeamAssembler) {
	o.assembler = a
}

// SetStarter injects the TeamStarterPort after construction.
func (o *RealTeamOrchestrator) SetStarter(s biz.TeamStarterPort) {
	o.starter = s
}

// SetAgentReader injects the AgentReader after construction.
// 用于在 PlanStep 未指定 AgentKeys 时查询可用 agent 列表。
func (o *RealTeamOrchestrator) SetAgentReader(r biz.AgentReader) {
	o.agentRdr = r
}

// Orchestrate creates a team for the given PlanStep and starts its team_run.
// Returns an OrchestrateResult containing the team + member info and a
// completion channel that emits exactly one TeamCompleteEvent when the
// team_run reaches a terminal status, then closes.
func (o *RealTeamOrchestrator) Orchestrate(ctx context.Context, step biz.PlanStep, ts biz.TeamStage) (*OrchestrateResult, error) {
	if o.assembler == nil {
		return nil, errOrchestratorNotReady
	}
	spiritSessionID := ts.SessionID
	taskDesc := step.Description
	if taskDesc == "" {
		taskDesc = step.Label
	}
	// 2026-07-05 Step 4 修复：优先使用 PlanStep.AgentKeys（来自 LLM 分配），
	// 仅在 PlanStep 未携带 AgentKeys 时 fallback 到查 DB 取 active agent。
	// 原先所有 team 都走 resolveAgentKeys 查 DB，导致所有 team 拿到同一批
	// active agent（按 updated_at 降序前 3 个），与 LLM 分配意图不符。
	agentKeys := step.AgentKeys
	if len(agentKeys) == 0 {
		o.lg.Info("PlanStep.AgentKeys 为空，fallback 到查 DB 取 active agent",
			loggateway.Str("step_id", step.ID),
			loggateway.Str("spirit_session_id", spiritSessionID))
		agentKeys = o.resolveAgentKeys(ctx)
	}
	o.lg.Info("Orchestrate 解析 AgentKeys",
		loggateway.Str("step_id", step.ID),
		loggateway.Str("source", agentKeysSource(step.AgentKeys)),
		loggateway.Int("agent_count", len(agentKeys)),
		loggateway.Any("agent_keys", agentKeys))
	// 构建 SpiritTeamParams。AutoStart=false：手动启动 team_run，
	// 确保 channel 在 StartTeamTurn 之前存入 pending map。
	// 2026-07-04 问题 4 修复：使用 coordinator 模式（默认）让第一个成员
	// 自动成为 synthesizer，避免 parallel 模式因缺少 synthesizer 被拒。
	// 参考：internal/biz/team_usecase.go:189 (parallel 模式必须显式指定 synthesizer)
	//
	// P0-①: 透传 DAG 依赖（step.DependsOn）到团队记录，下游调度与前端 DAG
	// 渲染依赖此字段。形式契约（Deliverables/InputContract）由 P1 planner
	// schema 扩展填充。
	params := biz.SpiritTeamParams{
		SpiritSessionID: spiritSessionID,
		TaskDescription: taskDesc,
		AgentKeys:       agentKeys,
		DagNodeID:       step.ID,
		DependsOn:       step.DependsOn,
		Mode:            biz.TeamModeCoordinator,
		AutoStart:       false,
		// P1 形式契约（B.10.15.2）：PlanStep → Team 落库，
		// 供 dagRun advisory 契约验证与下游注入读取。
		Deliverables:  step.Deliverables,
		InputContract: step.InputContract,
	}
	team, teamSession, memberSessions, err := o.assembler.AssembleTeam(ctx, params)
	if err != nil {
		o.lg.Error("AssembleTeam 失败",
			loggateway.Str("step_id", step.ID),
			loggateway.Str("spirit_session_id", spiritSessionID),
			loggateway.Err(err))
		return nil, err
	}
	// 创建 channel 并存入 pending map（必须在 StartTeamTurn 之前）。
	ch := make(chan biz.TeamCompleteEvent, 1)
	o.pending.Store(team.ID, pendingTeamCompletion{ch: ch, stepID: step.ID})
	o.lg.Info("team_run 已创建，等待完成",
		loggateway.Str("team_id", team.ID),
		loggateway.Str("step_id", step.ID),
		loggateway.Str("session_id", teamSession.ID))
	// 手动启动 team_run（AutoStart=false 时 AssembleTeam 不会自动启动）。
	// P0-③a: 首轮输入 = 上游交付物前缀 + 任务描述。上游团队在 RecordTeamCompletion
	// 时已把交付物落库（deliverables_output_json），此处由 biz 层组装前缀；
	// 存储的 team.TaskDescription 保持纯净，前缀只注入 Turn 输入。
	// 2026-07-25 Fix 2b: 统一走 BuildTeamTurnInput（前缀 + 描述 + 交付协议后缀）。
	turnContent := o.assembler.BuildTeamTurnInput(ctx, team)
	if o.starter != nil && teamSession.ID != "" && turnContent != "" {
		safego.Go(ctx, "team_orchestrator.start_turn."+team.ID, func() {
			startCtx := context.WithoutCancel(ctx)
			if startErr := o.starter.StartTeamTurn(startCtx, teamSession.ID, turnContent); startErr != nil {
				o.lg.Warn("StartTeamTurn 失败",
					loggateway.Str("team_id", team.ID),
					loggateway.Err(startErr))
				// 失败时发送失败事件到 channel，避免 dispatchStep 永久阻塞。
				o.NotifyTeamCompletion(team.ID, "", false, startErr.Error())
			}
		})
	} else {
		// 没有 starter 或 session，直接失败。
		o.NotifyTeamCompletion(team.ID, "", false, "starter or session missing")
	}
	// 2026-07-04 问题 4 修复：返回 team + memberSessions + TeamStageID 让
	// dispatchStep 能更新同一 TeamStage 记录（与 publishSpiritTeamAssembled
	// + publishV2TeamRunAndMemberSessions 派生 ID 一致），避免双重创建。
	teamStageID := string(agent.NewTeamStageActivityID(team.ID, string(agent.RootTaskActivityIDFromCtx(ctx))))
	result := &OrchestrateResult{
		Team:           team,
		TeamSession:    teamSession,
		MemberSessions: memberSessions,
		TeamStageID:    teamStageID,
		CompletionChan: ch,
	}
	return result, nil
}

// NotifyTeamCompletion sends a TeamCompleteEvent to the waiting channel.
// Called by PlanExecutor.NotifyTeamCompletion (forwarded from
// TeamStarter.HandleTeamTurnResult).
func (o *RealTeamOrchestrator) NotifyTeamCompletion(teamID, teamRunID string, success bool, errMsg string) {
	v, ok := o.pending.LoadAndDelete(teamID)
	if !ok {
		return // 没有 pending 的 channel，可能是 v1 路径的 team_run
	}
	pc, ok := v.(pendingTeamCompletion)
	if !ok {
		return
	}
	ev := biz.TeamCompleteEvent{
		StepID:    pc.stepID,
		TeamRunID: teamRunID,
		Success:   success,
		ErrorMsg:  errMsg,
	}
	select {
	case pc.ch <- ev:
	default:
		// channel 已满（buffer=1）， shouldn't happen
	}
	close(pc.ch)
	o.lg.Info("team_run 完成通知已发送",
		loggateway.Str("team_id", teamID),
		loggateway.Str("team_run_id", teamRunID),
		loggateway.Str("step_id", pc.stepID),
		loggateway.Bool("success", success))
}

// resolveAgentKeys 是 fallback 路径：当 PlanStep.AgentKeys 为空时查询可用
// active agent 列表作为团队成员。最多返回 3 个 agent，按 updated_at 降序。
// 如果 agentRdr 为 nil 或查询失败，返回空列表（会导致 AssembleTeam 失败，
// 但这比硬编码 agent_key 更安全）。
//
// 2026-07-05 Step 4：原先所有 team 都走此方法查 DB，导致所有 team 拿到同一批
// active agent。现在主路径使用 PlanStep.AgentKeys（来自 LLM 分配），此方法仅作 fallback。
func (o *RealTeamOrchestrator) resolveAgentKeys(ctx context.Context) []string {
	if o.agentRdr == nil {
		return nil
	}
	result, err := o.agentRdr.SearchAgents(ctx, biz.AgentListQuery{
		Status: "active",
		Limit:  3,
	})
	if err != nil {
		o.lg.Warn("查询可用 agent 列表失败",
			loggateway.Err(err))
		return nil
	}
	keys := make([]string, 0, len(result.Items))
	for _, a := range result.Items {
		if a.AgentKey != "" {
			keys = append(keys, a.AgentKey)
		}
	}
	return keys
}

// agentKeysSource 返回 AgentKeys 来源标识，用于日志诊断。
func agentKeysSource(stepKeys []string) string {
	if len(stepKeys) > 0 {
		return "plan_step"
	}
	return "db_fallback"
}

// errOrchestratorNotReady is returned when Orchestrate is called before
// SetAssembler has been invoked.
var errOrchestratorNotReady = &orchestratorNotReadyError{}

type orchestratorNotReadyError struct{}

func (e *orchestratorNotReadyError) Error() string {
	return "team orchestrator not ready: assembler not injected"
}

// 确保接口实现检查
var _ TeamOrchestrator = (*RealTeamOrchestrator)(nil)
var _ TeamCompletionNotifier = (*RealTeamOrchestrator)(nil)
