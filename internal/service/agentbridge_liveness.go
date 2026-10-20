package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/agentbridge"
	"aranea-agents/internal/biz/decision"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/auth"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
)

// agentbridge_liveness.go M4 P3-5（设计「活跃性监管」§B/C）：
// service 侧活跃性监管出口——阶梯迁移流程日志、用户提示、G3 失联确认卡
// （复用审批卡通道，source=external_coding_liveness）。

const (
	// noticeCodingTaskLiveness WS 通知类型：阶梯徽标/提示（task 行已含
	// liveness 字段，前端可从 ListTasks/GetTask 渲染徽标；本通知做即时触达）。
	noticeCodingTaskLiveness = "coding_task_liveness"
	// livenessStepPrefix 失联确认卡 step ID 前缀（S6 绑定校验用）。
	livenessStepPrefix = "bridge-liveness-"
	// livenessDecisionKeep / livenessDecisionKill 失联处置决议。
	livenessDecisionKeep = "keep"
	livenessDecisionKill = "kill"
)

// pendingLiveness 是一张挂起的失联确认卡（等待用户决议或超时）。
type pendingLiveness struct {
	taskID    string
	sessionID string
	stepID    string
	agentKey  string
	title     string
	done      chan approvalDecision
}

// ---- AgentBridgeService 实现 agentbridge.LivenessListener / LivenessDecider ----

// OnLivenessState 阶梯迁移：流程日志 + WS 徽标通知（设计 §C；
// agentbridge.liveness.{suspect,lost,recovered,kill} 见 flow_log_steps 注册表）。
func (s *AgentBridgeService) OnLivenessState(t *agentbridge.CodingTask, from, to string) {
	if t == nil {
		return
	}
	ctx := context.Background()
	agentKey, projectName := s.resolveNames(t.Workspace, t.AgentID, t.ProjectID)
	props := []event.Pair{
		event.P("task_id", t.ID),
		event.P("agent_key", agentKey),
		event.P("project_name", projectName),
		event.P("transition", from+"->"+to),
	}
	switch to {
	case agentbridge.LivenessSuspect:
		s.emitter(ctx, t.SessionID).LogWarn("agentbridge.liveness.suspect", "", "编程任务静默超时，活跃性降级为可疑", props...)
	case agentbridge.LivenessLost:
		s.emitter(ctx, t.SessionID).LogError("agentbridge.liveness.lost", "编程任务与 agent 失联（所有活跃性信号缺席）", props...)
	case "killed":
		s.emitter(ctx, t.SessionID).LogError("agentbridge.liveness.kill", "活跃性监管强制终止进程组", props...)
	case agentbridge.LivenessHealthy:
		if from != "" && from != to {
			s.emitter(ctx, t.SessionID).LogDone("agentbridge.liveness.recovered", "编程任务恢复活动", props...)
		}
	}
	// 徽标通知（面板即时刷新；数据权威仍在任务行 liveness 字段）。
	s.publish(biz.NewSystemNoticeEvent(t.SessionID, noticeCodingTaskLiveness, "", map[string]any{
		"task_id":        t.ID,
		"agent_key":      agentKey,
		"session_id":     t.SessionID,
		"liveness_state": to,
		"from_state":     from,
		"speak":          false,
	}))
}

// OnLivenessHint 阶梯用户提示（G1「静默中」/G2 静默超 N 分钟/软超时/处置结果）。
// speak=false：以进度面板系统提示为主，不发语音（避免阶梯抖动刷语音）。
func (s *AgentBridgeService) OnLivenessHint(t *agentbridge.CodingTask, text string) {
	if t == nil || strings.TrimSpace(text) == "" {
		return
	}
	agentKey, projectName := s.resolveNames(t.Workspace, t.AgentID, t.ProjectID)
	s.publish(biz.NewSystemNoticeEvent(t.SessionID, noticeCodingTaskLiveness, text, map[string]any{
		"task_id":        t.ID,
		"agent_key":      agentKey,
		"project_name":   projectName,
		"session_id":     t.SessionID,
		"liveness_state": t.LivenessState,
		"kind":           "hint",
		"speak":          false,
	}))
}

// AskLivenessAction 实现 LivenessDecider：G3 失联确认卡。
// 发卡 → 等待用户决议（approved=继续等待 / rejected=终止任务）；
// ctx（biz 已包 lost_decision_timeout）到期返回错误，biz 按设计 §C 处置=终止。
func (s *AgentBridgeService) AskLivenessAction(ctx context.Context, t *agentbridge.CodingTask, opts []agentbridge.PermissionOption) (string, error) {
	if t == nil {
		return "", apierror.Internal(apierror.DomainAgentBridge, "liveness ask on nil task")
	}
	agentKey, projectName := s.resolveNames(t.Workspace, t.AgentID, t.ProjectID)
	p := &pendingLiveness{
		taskID:    t.ID,
		sessionID: t.SessionID,
		agentKey:  agentKey,
		title:     "编程任务疑似失联",
		done:      make(chan approvalDecision, 1),
	}
	s.pendingLiv.Store(t.ID, p)
	defer s.pendingLiv.Delete(t.ID)

	spoken := fmt.Sprintf("编程任务「%s」已静默超时，疑似失联，是否继续等待？", projectName)
	s.emitLivenessCard(ctx, t, p, agentKey, projectName, spoken)

	select {
	case dec := <-p.done:
		if dec.err != nil {
			return "", dec.err
		}
		return dec.optionID, nil
	case <-ctx.Done():
		s.emitLivenessCardTimeout(t, agentKey)
		return "", ctx.Err()
	}
}

// emitLivenessCard 落确认卡（复用 HoloConfirmCard 渲染：ToolName=external_coding，
// source=external_coding_liveness）+ WS notice + step 持久化（可选 sink）。
func (s *AgentBridgeService) emitLivenessCard(ctx context.Context, t *agentbridge.CodingTask, p *pendingLiveness, agentKey, projectName, spoken string) {
	args, _ := json.Marshal(map[string]any{
		"source":         agentbridge.ToolExternalCodingLiveness,
		"agent_key":      agentKey,
		"project_name":   projectName,
		"task_id":        t.ID,
		"title":          p.title,
		"last_excerpt":   t.LastExcerpt,
		"liveness_state": t.LivenessState,
	})
	stepID := livenessStepPrefix + uuid.NewString()
	step := biz.Step{
		ID:              stepID,
		TurnID:          stepID, // orphan confirm：唯一 turn_id+seq，与聊天回合无关
		TaskID:          t.ID,
		SessionID:       t.SessionID,
		SpiritSessionID: t.SessionID,
		Kind:            biz.StepKindConfirm,
		AuthorAgentKey:  agentKey,
		Seq:             1,
		Version:         1,
		Content:         spoken,
		ToolName:        agentbridge.ToolExternalCoding,
		ToolArgs:        args,
		Status:          biz.StepStatusToolBlocked,
		StartedAt:       s.clock(),
	}
	if sink := s.confirmWriter(); sink != nil {
		if created, err := sink.CreateStep(ctx, step); err != nil {
			s.lg.Warn("agentbridge liveness confirm step create failed",
				loggateway.Str("task_id", t.ID), loggateway.Err(err))
		} else {
			step = created
		}
	}
	p.stepID = step.ID
	s.publish(biz.NewSystemNoticeEvent(t.SessionID, noticeCodingTaskApproval, spoken, map[string]any{
		"task_id":        t.ID,
		"agent_key":      agentKey,
		"project_name":   projectName,
		"session_id":     t.SessionID,
		"step_id":        step.ID,
		"title":          p.title,
		"source":         agentbridge.ToolExternalCodingLiveness,
		"liveness_state": t.LivenessState,
		"speak":          true, // 失联处置须用户拍板：发声提醒
	}))
	s.publish(biz.NewStepCreatedEvent(step))
}

// emitLivenessCardTimeout 失联确认卡超时（biz 将按设计 §C 处置=终止）。
func (s *AgentBridgeService) emitLivenessCardTimeout(t *agentbridge.CodingTask, agentKey string) {
	s.publish(biz.NewSystemNoticeEvent(t.SessionID, noticeCodingTaskLiveness, "失联确认超时，任务将被终止", map[string]any{
		"task_id":    t.ID,
		"agent_key":  agentKey,
		"session_id": t.SessionID,
		"kind":       "liveness_card_timeout",
		"speak":      true,
	}))
}

// rejectPendingLiveness 任务到达终态/取消路径时解除挂起失联卡（同 rejectPending 语义）。
func (s *AgentBridgeService) rejectPendingLiveness(taskID string, err error) {
	v, ok := s.pendingLiv.Load(taskID)
	if !ok {
		return
	}
	p, _ := v.(*pendingLiveness)
	select {
	case p.done <- approvalDecision{err: err}:
	default:
	}
}

// ConfirmBridgeLiveness 解除挂起的失联确认卡。decision 为
// confirm-reply 归一后的 approve/deny：approve=继续等待，deny=终止任务。
func (s *AgentBridgeService) ConfirmBridgeLiveness(ctx context.Context, taskID, decision string) error {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return apierror.BadRequest(apierror.DomainAgentBridge, "task_id is required")
	}
	v, ok := s.pendingLiv.Load(taskID)
	if !ok {
		return apierror.Conflict(apierror.DomainAgentBridge, "no pending liveness confirmation for task %s", taskID)
	}
	p, _ := v.(*pendingLiveness)
	d := normalizeBridgeDecision(decision)
	optionID := "keep_waiting"
	outcome := "keep_waiting"
	if d == agentbridge.DecisionDeny {
		optionID = "kill"
		outcome = "kill"
	}
	select {
	case p.done <- approvalDecision{optionID: optionID}:
		s.emitLivenessDecisionRecord(ctx, p, taskID, outcome)
		return nil
	default:
		return apierror.Conflict(apierror.DomainAgentBridge, "liveness confirmation already resolved")
	}
}

// ConfirmBridgeLivenessFromStep 是 ConfirmActivity 适配器（镜像 S6 绑定校验：
// step 前缀 + task/session/agent 多重一致性，防伪造他会话确认卡越权处置）。
func (s *AgentBridgeService) ConfirmBridgeLivenessFromStep(ctx context.Context, step biz.Step, reply string, approved bool) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if !strings.HasPrefix(step.ID, livenessStepPrefix) {
		return apierror.Forbidden(apierror.DomainAgentBridge, "not a coding-liveness confirm card")
	}
	args := map[string]any{}
	if len(step.ToolArgs) > 0 {
		_ = json.Unmarshal(step.ToolArgs, &args)
	}
	taskID, _ := args["task_id"].(string)
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		taskID = strings.TrimSpace(step.TaskID)
	}
	if taskID == "" {
		return apierror.BadRequest(apierror.DomainAgentBridge, "confirm card missing task binding")
	}
	if st := strings.TrimSpace(step.TaskID); st != "" && st != taskID {
		return apierror.Forbidden(apierror.DomainAgentBridge, "confirm card task binding mismatch")
	}
	v, ok := s.pendingLiv.Load(taskID)
	if !ok {
		return apierror.Conflict(apierror.DomainAgentBridge, "no pending liveness confirmation for task %s", taskID)
	}
	p, _ := v.(*pendingLiveness)
	if step.SessionID != "" && p.sessionID != "" && step.SessionID != p.sessionID {
		return apierror.Forbidden(apierror.DomainAgentBridge, "confirm card session mismatch")
	}
	if ak, _ := args["agent_key"].(string); ak != "" && p.agentKey != "" && ak != p.agentKey {
		return apierror.Forbidden(apierror.DomainAgentBridge, "confirm card agent mismatch")
	}
	if p.stepID != "" && step.ID != p.stepID {
		return apierror.Forbidden(apierror.DomainAgentBridge, "confirm card does not match pending liveness confirmation")
	}
	decision := livenessDecisionKeep
	if !approved {
		decision = livenessDecisionKill
	}
	// normalizeBridgeDecision 兼容直接决议串。
	return s.ConfirmBridgeLiveness(ctx, taskID, mapLivenessDecisionToBridge(reply, decision))
}

// mapLivenessDecisionToBridge 把失联语义映射到 bridge 决议域（keep→approve / kill→deny）。
func mapLivenessDecisionToBridge(reply, fallback string) string {
	if d := normalizeBridgeDecision(reply); d == agentbridge.DecisionApprove || d == agentbridge.DecisionDeny {
		return d
	}
	if fallback == livenessDecisionKill {
		return agentbridge.DecisionDeny
	}
	return agentbridge.DecisionApprove
}

// emitLivenessDecisionRecord 失联处置决议双写 M80（镜像 emitBridgeDecisionRecord）。
func (s *AgentBridgeService) emitLivenessDecisionRecord(ctx context.Context, p *pendingLiveness, taskID, outcome string) {
	scenario := fmt.Sprintf("失联处置确认: %s (external_coding_liveness)", p.title)
	flowCtx := event.WithTraceEmitter(ctx, s.emitter(ctx, p.sessionID))
	userID := "unknown"
	if a, ok := auth.FromContext(ctx); ok && a != nil && a.UserID > 0 {
		userID = fmt.Sprintf("%d", a.UserID)
	}
	metadata := map[string]any{"decision_reason": "external_coding_liveness"}
	if sid := strings.TrimSpace(p.sessionID); sid != "" {
		metadata["session_id"] = sid
	}
	entities := []decision.EntityRef{{Type: "tool", Key: agentbridge.ToolExternalCoding}}
	if p.agentKey != "" {
		entities = append(entities, decision.EntityRef{Type: "agent", Key: p.agentKey})
	}
	event.EmitDecision(flowCtx, s.decisionCollector(), decision.Record{
		DecisionKey:     uuid.NewString(),
		Category:        decision.CategoryHITLApproval,
		Scenario:        scenario,
		Outcome:         outcome,
		ActorType:       decision.ActorHuman,
		ActorKey:        userID,
		RelatedEntities: entities,
		SourceRef:       decision.SourceRef{TaskID: taskID, SessionID: strings.TrimSpace(p.sessionID)},
		Metadata:        metadata,
	}, "system.gate.hitl_approval", scenario,
		event.P("trigger", "hitl_liveness"),
		event.P("outcome", outcome),
		event.P("tool_key", agentbridge.ToolExternalCoding))
}

// IsExternalCodingLivenessConfirm 报告 step 是否为失联处置确认卡
// （source=external_coding_liveness；须在 IsExternalCodingConfirm 之前判定）。
func IsExternalCodingLivenessConfirm(step biz.Step) bool {
	if len(step.ToolArgs) == 0 {
		return false
	}
	var m map[string]any
	if json.Unmarshal(step.ToolArgs, &m) != nil {
		return false
	}
	src, _ := m["source"].(string)
	return src == agentbridge.ToolExternalCodingLiveness
}

var (
	_ agentbridge.LivenessDecider  = (*AgentBridgeService)(nil)
	_ agentbridge.LivenessListener = (*AgentBridgeService)(nil)
)
