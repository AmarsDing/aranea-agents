package service

import (
	"context"
	"fmt"
	"strings"
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
	org       biz.OrganizationReader
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
// Used by resolveTeamOrg to map member keys to department / borrow IDs.
func (o *RealTeamOrchestrator) SetAgentReader(r biz.AgentReader) {
	o.agentRdr = r
}

// SetOrganizationReader injects org lookup for majority-vote DepartmentID
// when PlanStep.DepartmentID is empty (members have positions).
func (o *RealTeamOrchestrator) SetOrganizationReader(org biz.OrganizationReader) {
	o.org = org
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
	// Allocator must fill PlanStep.AgentKeys. A SearchAgents(limit:3) fallback
	// previously bound every empty step to the same three recently-updated
	// agents and silently assembled the wrong roster.
	agentKeys := step.AgentKeys
	if len(agentKeys) == 0 {
		o.lg.Warn("PlanStep.AgentKeys 为空，拒绝编排",
			loggateway.Str("step_id", step.ID),
			loggateway.Str("spirit_session_id", spiritSessionID))
		return nil, fmt.Errorf("plan step %s has empty agent keys", step.ID)
	}
	o.lg.Info("Orchestrate 解析 AgentKeys",
		loggateway.Str("step_id", step.ID),
		loggateway.Str("source", agentKeysSource(step.AgentKeys)),
		loggateway.Int("agent_count", len(agentKeys)),
		loggateway.Any("agent_keys", agentKeys))
	// 构建 SpiritTeamParams。AutoStart=false：手动启动 team_run，
	// 确保 channel 在 StartTeamTurn 之前存入 pending map。
	// Intra-team Mode comes from PlanStep (planner strategy + member count).
	// Parallel JSON sets last member as synthesizer in buildSpiritTeamDefinitionJSON.
	//
	// P0-①: 透传 DAG 依赖（step.DependsOn）到团队记录，下游调度与前端 DAG
	// 渲染依赖此字段。形式契约（Deliverables/InputContract）由 P1 planner
	// schema 扩展填充。
	mode := strings.TrimSpace(step.Mode)
	if mode == "" {
		mode = biz.SpiritTeamModeForStep("", len(agentKeys))
	}
	deptID, crossIDs := o.resolveTeamOrg(ctx, step, agentKeys)
	params := biz.SpiritTeamParams{
		SpiritSessionID:         spiritSessionID,
		TaskDescription:         taskDesc,
		AgentKeys:               agentKeys,
		DagNodeID:               step.ID,
		DependsOn:               step.DependsOn,
		Mode:                    mode,
		AutoStart:               false,
		DepartmentID:            deptID,
		CrossDeptMemberAgentIDs: crossIDs,
		// P1 形式契约（B.10.15.2）：PlanStep → Team 落库，
		// 供 dagRun advisory 契约验证与下游注入读取。
		Deliverables:    step.Deliverables,
		InputContract:   step.InputContract,
		GraphTemplateID: step.GraphTemplateID,
		CollectionIDs:   step.CollectionIDs,
	}
	team, teamSession, memberSessions, err := o.assembler.AssembleTeam(ctx, params)
	if err != nil {
		o.lg.Error("AssembleTeam 失败",
			loggateway.Str("step_id", step.ID),
			loggateway.Str("spirit_session_id", spiritSessionID),
			loggateway.Err(err))
		return nil, err
	}
	alreadyRunning := team.Status == biz.TeamStatusRunning
	// 创建 channel 并存入 pending map（必须在 StartTeamTurn 之前）。
	ch := make(chan biz.TeamCompleteEvent, 1)
	if existing, loaded := o.pending.LoadOrStore(team.ID, pendingTeamCompletion{ch: ch, stepID: step.ID}); loaded {
		// 同 team 已有等待者（重复 Orchestrate / 复用 running 团队）：
		// 不要覆盖原 channel，否则先派发的 dispatchStep 会永久阻塞。
		if pc, ok := existing.(pendingTeamCompletion); ok && pc.ch != nil {
			ch = pc.ch
		}
		o.lg.Info("team_run 已在等待完成，跳过重复启动",
			loggateway.Str("team_id", team.ID),
			loggateway.Str("step_id", step.ID),
			loggateway.Bool("already_running", alreadyRunning))
	} else if alreadyRunning {
		o.lg.Info("复用已在执行的团队，跳过 StartTeamTurn",
			loggateway.Str("team_id", team.ID),
			loggateway.Str("step_id", step.ID))
	} else {
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

// resolveTeamOrg picks the team's home department and borrow agent IDs.
// Preference: PlanStep.DepartmentID; else majority vote of member positions.
func (o *RealTeamOrchestrator) resolveTeamOrg(ctx context.Context, step biz.PlanStep, agentKeys []string) (departmentID string, crossDeptAgentIDs []string) {
	departmentID = strings.TrimSpace(step.DepartmentID)
	// Playbook-declared department key takes priority over majority vote:
	// when DepartmentID is empty, resolve DepartmentKey → DepartmentID via org tree.
	if departmentID == "" && strings.TrimSpace(step.DepartmentKey) != "" && o.org != nil {
		if node, err := o.org.GetOrgNodeByKey(ctx, strings.TrimSpace(step.DepartmentKey)); err == nil && node.ID != "" {
			departmentID = node.ID
		}
	}
	type member struct {
		id   string
		key  string
		dept string
	}
	members := make([]member, 0, len(agentKeys))
	if o.agentRdr != nil {
		for _, k := range agentKeys {
			ag, err := o.agentRdr.GetAgentByAgentKey(ctx, k)
			if err != nil {
				continue
			}
			dept := departmentID
			if o.org != nil {
				if d := departmentOfAgent(ctx, o.org, ag); d != "" {
					dept = d
				}
			}
			members = append(members, member{id: ag.ID, key: ag.AgentKey, dept: dept})
		}
	}
	if departmentID == "" {
		counts := map[string]int{}
		best, bestN := "", 0
		for _, m := range members {
			if m.dept == "" {
				continue
			}
			counts[m.dept]++
			if counts[m.dept] > bestN {
				best = m.dept
				bestN = counts[m.dept]
			}
		}
		departmentID = best
	}
	crossWant := make(map[string]struct{}, len(step.CrossDeptMemberKeys))
	for _, k := range step.CrossDeptMemberKeys {
		crossWant[k] = struct{}{}
	}
	for _, m := range members {
		if m.id == "" {
			continue
		}
		if _, marked := crossWant[m.key]; marked {
			crossDeptAgentIDs = append(crossDeptAgentIDs, m.id)
			continue
		}
		if departmentID != "" && m.dept != "" && m.dept != departmentID {
			crossDeptAgentIDs = append(crossDeptAgentIDs, m.id)
		}
	}
	return departmentID, crossDeptAgentIDs
}

func departmentOfAgent(ctx context.Context, org biz.OrganizationReader, ag biz.Agent) string {
	pid := strings.TrimSpace(ag.PositionID)
	if pid == "" || org == nil {
		return ""
	}
	n, err := org.GetOrgNode(ctx, pid)
	if err != nil {
		return ""
	}
	switch n.Level {
	case "department":
		return n.ID
	case "position":
		if strings.TrimSpace(n.ParentID) == "" {
			return ""
		}
		parent, err := org.GetOrgNode(ctx, n.ParentID)
		if err != nil || parent.Level != "department" {
			return ""
		}
		return parent.ID
	default:
		return ""
	}
}

// agentKeysSource 返回 AgentKeys 来源标识，用于日志诊断。
func agentKeysSource(stepKeys []string) string {
	if len(stepKeys) > 0 {
		return "plan_step"
	}
	return "empty"
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
