package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/agentbridge"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
)

const (
	noticeCodingTaskApproval = "coding_task_approval"
	defaultApprovalTimeout   = 5 * time.Minute
)

// approvalDecision is one user (or timeout) resolution of a pending ACP permission.
type approvalDecision struct {
	optionID string
	err      error
}

type pendingApproval struct {
	taskID    string
	sessionID string
	stepID    string
	title     string
	options   []agentbridge.PermissionOption
	done      chan approvalDecision
}

// ConfirmSink persists companion confirm steps so HoloConfirmCard can render
// source=external_coding. Optional: nil sink still emits the WS notice.
type ConfirmSink interface {
	CreateStep(ctx context.Context, s biz.Step) (biz.Step, error)
	UpdateStep(ctx context.Context, s biz.Step) (biz.Step, error)
}

// SetConfirmSink wires step persistence after ChatService exists
// (newApp BeforeStart). Safe to call once.
func (s *AgentBridgeService) SetConfirmSink(w ConfirmSink) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.confirmSink = w
}

// SetApprovalTimeout overrides the 5-minute default (tests).
func (s *AgentBridgeService) SetApprovalTimeout(d time.Duration) {
	if s == nil || d <= 0 {
		return
	}
	s.mu.Lock()
	s.approvalTimeout = d
	s.mu.Unlock()
}

func (s *AgentBridgeService) approvalWait() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.approvalTimeout > 0 {
		return s.approvalTimeout
	}
	return defaultApprovalTimeout
}

// OnPermission implements agentbridge.EventHandler: M2 审批中继。
// allow_always 仅缓存本任务（设计 §8），不落 tool-grants。
func (h *bridgeProgressHandler) OnPermission(ctx context.Context, title string, opts []agentbridge.PermissionOption) (string, error) {
	<-h.ready
	if h.taskID == "" {
		return "", apierror.Internal(apierror.DomainAgentBridge, "permission before task bind")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if id, ok := h.svc.cachedAlwaysOption(h.taskID, opts); ok {
		return id, nil
	}
	uc, err := h.svc.usecase()
	if err != nil {
		return "", err
	}
	if err := uc.MarkAwaitingApproval(h.taskID); err != nil {
		return "", err
	}

	pending := &pendingApproval{
		taskID:    h.taskID,
		sessionID: h.sessionID,
		title:     strings.TrimSpace(title),
		options:   opts,
		done:      make(chan approvalDecision, 1),
	}
	h.svc.storePending(pending)
	h.svc.emitApprovalRequest(ctx, h, pending)

	timeout := h.svc.approvalWait()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case dec := <-pending.done:
		h.svc.clearPending(h.taskID)
		if dec.err != nil {
			return "", dec.err
		}
		if err := uc.ResumeFromApproval(h.taskID); err != nil {
			h.svc.lg.Warn("agentbridge resume after approval failed",
				loggateway.Str("task_id", h.taskID), loggateway.Err(err))
		}
		return dec.optionID, nil
	case <-timer.C:
		h.svc.clearPending(h.taskID)
		h.svc.emitApprovalTimeout(h.sessionID, h.taskID, h.agentKey)
		if err := uc.CancelFromApprovalTimeout(h.taskID); err != nil {
			h.svc.lg.Warn("agentbridge approval timeout cancel failed",
				loggateway.Str("task_id", h.taskID), loggateway.Err(err))
		}
		return "", apierror.Unavailable(apierror.DomainAgentBridge, "approval timed out")
	case <-ctx.Done():
		h.svc.clearPending(h.taskID)
		return "", ctx.Err()
	}
}

func (s *AgentBridgeService) cachedAlwaysOption(taskID string, opts []agentbridge.PermissionOption) (string, bool) {
	if _, ok := s.always.Load(taskID); !ok {
		return "", false
	}
	id, _, err := agentbridge.ResolvePermissionOption(opts, agentbridge.DecisionApprove, true)
	if err != nil {
		return "", false
	}
	return id, true
}

func (s *AgentBridgeService) storePending(p *pendingApproval) {
	s.pending.Store(p.taskID, p)
}

func (s *AgentBridgeService) clearPending(taskID string) {
	s.pending.Delete(taskID)
}

func (s *AgentBridgeService) loadPending(taskID string) *pendingApproval {
	v, ok := s.pending.Load(taskID)
	if !ok {
		return nil
	}
	p, _ := v.(*pendingApproval)
	return p
}

func (s *AgentBridgeService) emitApprovalRequest(ctx context.Context, h *bridgeProgressHandler, p *pendingApproval) {
	spoken := approvalSpokenPrompt(h.agentKey, h.project, p.title)
	args, _ := json.Marshal(map[string]any{
		"source":       agentbridge.ToolExternalCoding,
		"agent_key":    h.agentKey,
		"project_name": h.project,
		"task_id":      h.taskID,
		"title":        p.title,
		"target":       p.title,
	})
	stepID := "bridge-confirm-" + uuid.NewString()
	step := biz.Step{
		ID:              stepID,
		TurnID:          stepID, // orphan confirm: unique turn_id+seq, no chat turn
		TaskID:          h.taskID,
		SessionID:       h.sessionID,
		SpiritSessionID: h.sessionID,
		Kind:            biz.StepKindConfirm,
		AuthorAgentKey:  h.agentKey,
		Seq:             1,
		Version:         1,
		Content:         spoken,
		ToolName:        agentbridge.ToolExternalCoding,
		ToolArgs:        args,
		Status:          biz.StepStatusToolBlocked,
		StartedAt:       s.clock(),
		Danger:          approvalLooksDangerous(p.title),
	}
	if sink := s.confirmWriter(); sink != nil {
		if created, err := sink.CreateStep(ctx, step); err != nil {
			s.lg.Warn("agentbridge confirm step create failed",
				loggateway.Str("task_id", h.taskID), loggateway.Err(err))
		} else {
			step = created
		}
	}
	p.stepID = step.ID
	s.publish(biz.NewSystemNoticeEvent(h.sessionID, noticeCodingTaskApproval, spoken, map[string]any{
		"task_id":      h.taskID,
		"agent_key":    h.agentKey,
		"project_name": h.project,
		"session_id":   h.sessionID,
		"step_id":      step.ID,
		"title":        p.title,
		"source":       agentbridge.ToolExternalCoding,
		"speak":        true,
	}))
	s.publish(biz.NewStepCreatedEvent(step))
	s.emitter(ctx, h.sessionID).LogDone("agentbridge.approval.request", spoken,
		event.P("task_id", h.taskID),
		event.P("agent_key", h.agentKey),
		event.P("title", p.title))
}

func (s *AgentBridgeService) emitApprovalTimeout(sessionID, taskID, agentKey string) {
	msg := "编程工具审批已超时，任务已取消"
	s.publish(biz.NewSystemNoticeEvent(sessionID, noticeCodingTaskCancelled, msg, map[string]any{
		"task_id":    taskID,
		"agent_key":  agentKey,
		"session_id": sessionID,
		"reason":     "approval_timeout",
		"speak":      true,
	}))
	s.emitter(context.Background(), sessionID).LogWarn("agentbridge.approval.timeout", "", msg,
		event.P("task_id", taskID), event.P("agent_key", agentKey))
}

func (s *AgentBridgeService) confirmWriter() ConfirmSink {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.confirmSink
}

// ConfirmBridgePermission unblocks a waiting OnPermission. decision is
// approve / deny / always (companion tokens; approve_session treated as always
// for this task-scoped cache).
func (s *AgentBridgeService) ConfirmBridgePermission(ctx context.Context, taskID, decision string) error {
	if s == nil {
		return apierror.Internal(apierror.DomainAgentBridge, "service unavailable")
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return apierror.BadRequest(apierror.DomainAgentBridge, "task_id is required")
	}
	p := s.loadPending(taskID)
	if p == nil {
		return apierror.Conflict(apierror.DomainAgentBridge, "no pending approval for task %s", taskID)
	}
	decision = normalizeBridgeDecision(decision)
	id, remember, err := agentbridge.ResolvePermissionOption(p.options, decision, false)
	if err != nil {
		return err
	}
	if remember {
		s.always.Store(taskID, true)
	}
	select {
	case p.done <- approvalDecision{optionID: id}:
		return nil
	default:
		return apierror.Conflict(apierror.DomainAgentBridge, "approval already resolved")
	}
}

// ConfirmBridgePermissionFromStep is the ConfirmActivity adapter.
func (s *AgentBridgeService) ConfirmBridgePermissionFromStep(_ context.Context, step biz.Step, reply string, approved bool) error {
	taskID := taskIDFromConfirmArgs(step.ToolArgs)
	if taskID == "" {
		taskID = strings.TrimSpace(step.TaskID)
	}
	return s.ConfirmBridgePermission(context.Background(), taskID, decisionFromConfirmReply(reply, approved))
}

func normalizeBridgeDecision(decision string) string {
	d := strings.ToLower(strings.TrimSpace(decision))
	switch d {
	case "approve", "approved", "__aranea:tool_confirm:approve":
		return agentbridge.DecisionApprove
	case "deny", "rejected", "reject", "__aranea:tool_confirm:deny":
		return agentbridge.DecisionDeny
	case "always", "approve_always", "approve_session",
		"__aranea:tool_confirm:approve_always",
		"__aranea:tool_confirm:approve_session":
		return agentbridge.DecisionAlways
	default:
		return d
	}
}

func decisionFromConfirmReply(reply string, approved bool) string {
	if d := normalizeBridgeDecision(reply); d == agentbridge.DecisionApprove ||
		d == agentbridge.DecisionDeny || d == agentbridge.DecisionAlways {
		return d
	}
	if approved {
		return agentbridge.DecisionApprove
	}
	return agentbridge.DecisionDeny
}

func taskIDFromConfirmArgs(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return ""
	}
	id, _ := m["task_id"].(string)
	return strings.TrimSpace(id)
}

func approvalSpokenPrompt(agentKey, project, title string) string {
	who := strings.TrimSpace(agentKey)
	if who == "" {
		who = "编程工具"
	}
	if p := strings.TrimSpace(project); p != "" {
		who = who + " · " + p
	}
	action := strings.TrimSpace(title)
	if action == "" {
		action = "执行一项操作"
	}
	return fmt.Sprintf("%s 想执行 %s，允许吗？", who, action)
}

func approvalLooksDangerous(title string) bool {
	t := strings.ToLower(title)
	for _, k := range []string{"rm ", "del ", "delete", "git push", "drop ", "format ", "shutdown"} {
		if strings.Contains(t, k) {
			return true
		}
	}
	return false
}

// IsExternalCodingConfirm reports whether a confirm step is a bridge relay card.
func IsExternalCodingConfirm(step biz.Step) bool {
	if step.ToolName == agentbridge.ToolExternalCoding {
		return true
	}
	if len(step.ToolArgs) == 0 {
		return false
	}
	var m map[string]any
	if json.Unmarshal(step.ToolArgs, &m) != nil {
		return false
	}
	src, _ := m["source"].(string)
	return src == agentbridge.ToolExternalCoding
}

var (
	_ agentbridge.EventHandler = (*bridgeProgressHandler)(nil)
)
