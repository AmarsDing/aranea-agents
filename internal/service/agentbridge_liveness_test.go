package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/agentbridge"
)

// ---------- M4 P3-5：失联确认卡（service 侧有效出口） ----------

// liveTask 构造挂在 pendingLiv 语义下的任务快照（不派发 ACP 进程）。
func liveTask() *agentbridge.CodingTask {
	return &agentbridge.CodingTask{
		ID:        "task-liv-1",
		Workspace: "default",
		SessionID: "sess-1",
		AgentID:   "agent-codex",
		ProjectID: "proj-aranea",
		Status:    agentbridge.StatusRunning,
	}
}

// AskLivenessAction 发卡 → 用户批准（继续等待）→ 返回 keep_waiting。
func TestAgentBridgeService_LivenessCardApproveKeepsWaiting(t *testing.T) {
	e := newABSvcEnv()
	e.seedAgent("codex")
	e.seedProject("aranea", `F:\aranea`)

	decision := make(chan string, 1)
	go func() {
		id, err := e.svc.AskLivenessAction(context.Background(), liveTask(),
			[]agentbridge.PermissionOption{
				{OptionID: "keep_waiting", Name: "继续等待", Kind: "allow_once"},
				{OptionID: "kill", Name: "终止任务", Kind: "reject_once"},
			})
		if err != nil {
			decision <- "ERR:" + err.Error()
			return
		}
		decision <- id
	}()

	// 等到失联卡通知发出、pending 挂起。
	e.bus.waitNoticeType(t, noticeCodingTaskApproval, 1)
	if err := e.svc.ConfirmBridgeLiveness(context.Background(), "task-liv-1", agentbridge.DecisionApprove); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	select {
	case got := <-decision:
		if got != "keep_waiting" {
			t.Fatalf("expected keep_waiting, got %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("AskLivenessAction did not return after confirm")
	}
}

// AskLivenessAction 发卡 → 用户拒绝 → 返回 kill（biz 侧将进入处置链）。
func TestAgentBridgeService_LivenessCardDenyKills(t *testing.T) {
	e := newABSvcEnv()
	e.seedAgent("codex")
	e.seedProject("aranea", `F:\aranea`)

	decision := make(chan string, 1)
	go func() {
		id, err := e.svc.AskLivenessAction(context.Background(), liveTask(),
			[]agentbridge.PermissionOption{
				{OptionID: "keep_waiting", Name: "继续等待", Kind: "allow_once"},
				{OptionID: "kill", Name: "终止任务", Kind: "reject_once"},
			})
		if err != nil {
			decision <- "ERR:" + err.Error()
			return
		}
		decision <- id
	}()

	e.bus.waitNoticeType(t, noticeCodingTaskApproval, 1)
	if err := e.svc.ConfirmBridgeLiveness(context.Background(), "task-liv-1", agentbridge.DecisionDeny); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	select {
	case got := <-decision:
		if got != "kill" {
			t.Fatalf("expected kill, got %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("AskLivenessAction did not return after deny")
	}
}

// AskLivenessAction：ctx 超时（biz lost_decision_timeout）→ 返回错误。
func TestAgentBridgeService_LivenessCardTimeout(t *testing.T) {
	e := newABSvcEnv()
	e.seedAgent("codex")
	e.seedProject("aranea", `F:\aranea`)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := e.svc.AskLivenessAction(ctx, liveTask(),
		[]agentbridge.PermissionOption{{OptionID: "keep_waiting", Name: "继续等待", Kind: "allow_once"}})
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

// ConfirmBridgeLivenessFromStep：伪造前缀/会话错配被拒（S6 绑定校验镜像）。
func TestAgentBridgeService_LivenessFromStepBinding(t *testing.T) {
	e := newABSvcEnv()
	e.seedAgent("codex")
	e.seedProject("aranea", `F:\aranea`)

	done := make(chan struct{})
	go func() {
		_, _ = e.svc.AskLivenessAction(context.Background(), liveTask(),
			[]agentbridge.PermissionOption{{OptionID: "keep_waiting", Name: "继续等待", Kind: "allow_once"}})
		close(done)
	}()
	e.bus.waitNoticeType(t, noticeCodingTaskApproval, 1)
	v, _ := e.svc.pendingLiv.Load("task-liv-1")
	p := v.(*pendingLiveness)

	mkStep := func(id, sessionID string) biz.Step {
		args, _ := json.Marshal(map[string]any{
			"source":    agentbridge.ToolExternalCodingLiveness,
			"task_id":   "task-liv-1",
			"agent_key": "codex",
		})
		return biz.Step{
			ID: id, TurnID: id, TaskID: "task-liv-1", SessionID: sessionID,
			Kind: biz.StepKindConfirm, ToolName: agentbridge.ToolExternalCoding,
			ToolArgs: args, Status: biz.StepStatusToolBlocked,
		}
	}
	step := mkStep(p.stepID, "sess-1")

	// 非本前缀的 step 必拒。
	bad := mkStep("bridge-confirm-x", "sess-1")
	if err := e.svc.ConfirmBridgeLivenessFromStep(context.Background(), bad, "", true); err == nil {
		t.Fatal("expected reject for foreign prefix")
	}
	// 会话错配必拒。
	mismatch := mkStep(p.stepID, "sess-OTHER")
	if err := e.svc.ConfirmBridgeLivenessFromStep(context.Background(), mismatch, "", true); err == nil {
		t.Fatal("expected reject for session mismatch")
	}
	// 正卡批准 → 解除挂起。
	if err := e.svc.ConfirmBridgeLivenessFromStep(context.Background(), step, "", true); err != nil {
		t.Fatalf("confirm from step: %v", err)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("liveness ask not unblocked by step confirm")
	}
}

// OnLivenessState：阶梯迁移发 WS 徽标通知（flow 步骤可能在 infra=nil 时只落进程日志，不阻塞）。
func TestAgentBridgeService_OnLivenessStatePublishes(t *testing.T) {
	e := newABSvcEnv()
	e.seedAgent("codex")
	e.seedProject("aranea", `F:\aranea`)

	task := liveTask()
	e.svc.OnLivenessState(task, agentbridge.LivenessSilent, agentbridge.LivenessSuspect)
	notices := e.bus.noticesByType(noticeCodingTaskLiveness)
	if len(notices) == 0 {
		t.Fatal("expected liveness notice published")
	}
	meta := notices[0].Meta
	if meta["liveness_state"] != agentbridge.LivenessSuspect {
		t.Fatalf("unexpected state meta: %v", meta["liveness_state"])
	}
}
