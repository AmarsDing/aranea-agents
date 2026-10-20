package service

import (
	"context"
	"testing"
	"time"

	"encoding/json"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/agentbridge"
)

func TestAgentBridgeService_ApprovalRelayApprove(t *testing.T) {
	e := newABSvcEnv()
	e.seedAgent("codex")
	e.seedProject("aranea", `F:\aranea`)

	got := make(chan string, 1)
	started := make(chan struct{})
	e.factory.sessions = []*abFakeSession{{
		promptFn: func(ctx context.Context, _, _ string, h agentbridge.EventHandler) (string, error) {
			close(started)
			id, err := h.OnPermission(ctx, "go test ./...", []agentbridge.PermissionOption{
				{OptionID: "allow", Name: "允许", Kind: "allow_once"},
				{OptionID: "deny", Name: "拒绝", Kind: "reject_once"},
			})
			if err != nil {
				return "", err
			}
			got <- id
			return "ok", nil
		},
	}}

	res, err := e.svc.DispatchTask(context.Background(), "sess-1", "codex", "aranea", "跑测试")
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	<-started
	notices := e.bus.waitNoticeType(t, noticeCodingTaskApproval, 1)
	if notices[0].Meta["source"] != agentbridge.ToolExternalCoding {
		t.Fatalf("approval notice source = %v", notices[0].Meta["source"])
	}
	if err := e.svc.ConfirmBridgePermission(context.Background(), res.Task.ID, agentbridge.DecisionApprove); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	select {
	case id := <-got:
		if id != "allow" {
			t.Fatalf("option = %q, want allow", id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("OnPermission did not return")
	}
}

func TestAgentBridgeService_ApprovalAlwaysCachesThisTask(t *testing.T) {
	e := newABSvcEnv()
	e.seedAgent("codex")
	e.seedProject("aranea", `F:\aranea`)

	ids := make(chan string, 2)
	started := make(chan struct{})
	opts := []agentbridge.PermissionOption{
		{OptionID: "allow", Kind: "allow_once"},
		{OptionID: "always", Kind: "allow_always"},
		{OptionID: "deny", Kind: "reject_once"},
	}
	e.factory.sessions = []*abFakeSession{{
		promptFn: func(ctx context.Context, _, _ string, h agentbridge.EventHandler) (string, error) {
			close(started)
			id1, err := h.OnPermission(ctx, "write file", opts)
			if err != nil {
				return "", err
			}
			ids <- id1
			id2, err := h.OnPermission(ctx, "write file again", opts)
			if err != nil {
				return "", err
			}
			ids <- id2
			return "ok", nil
		},
	}}

	res, err := e.svc.DispatchTask(context.Background(), "sess-1", "codex", "aranea", "改文件")
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	<-started
	e.bus.waitNoticeType(t, noticeCodingTaskApproval, 1)
	if err := e.svc.ConfirmBridgePermission(context.Background(), res.Task.ID, agentbridge.DecisionAlways); err != nil {
		t.Fatalf("confirm always: %v", err)
	}
	select {
	case id := <-ids:
		if id != "always" {
			t.Fatalf("first option = %q", id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first OnPermission blocked")
	}
	select {
	case id := <-ids:
		if id != "always" {
			t.Fatalf("cached option = %q, want always (no second card)", id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("second OnPermission should auto-allow")
	}
	if n := len(e.bus.noticesByType(noticeCodingTaskApproval)); n != 1 {
		t.Fatalf("second permission must not emit another card, notices=%d", n)
	}
}

func TestAgentBridgeService_ApprovalTimeoutCancelsTask(t *testing.T) {
	e := newABSvcEnv()
	e.seedAgent("codex")
	e.seedProject("aranea", `F:\aranea`)
	e.svc.SetApprovalTimeout(30 * time.Millisecond)

	done := make(chan error, 1)
	e.factory.sessions = []*abFakeSession{{
		promptFn: func(ctx context.Context, _, _ string, h agentbridge.EventHandler) (string, error) {
			_, err := h.OnPermission(ctx, "rm -rf /", []agentbridge.PermissionOption{
				{OptionID: "allow", Kind: "allow_once"},
				{OptionID: "deny", Kind: "reject_once"},
			})
			done <- err
			return "", err
		},
	}}

	res, err := e.svc.DispatchTask(context.Background(), "sess-1", "codex", "aranea", "危险")
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("timeout must return error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("OnPermission did not time out")
	}
	task, err := e.svc.GetTask(context.Background(), res.Task.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if task.Status != agentbridge.StatusCancelled {
		t.Fatalf("status = %s, want cancelled", task.Status)
	}
}

// TestAgentBridgeService_CancelDuringApprovalUnblocksPermission S5（M4 P0-3 golden）：
// 审批中用户取消不再 Conflict（awaiting_approval→cancelling 合法边），且挂起的
// OnPermission 被驳回式解除（不再僵等超时），任务经 run 协程收尾 cancelled。
func TestAgentBridgeService_CancelDuringApprovalUnblocksPermission(t *testing.T) {
	e := newABSvcEnv()
	e.seedAgent("codex")
	e.seedProject("aranea", `F:\aranea`)

	done := make(chan error, 1)
	e.factory.sessions = []*abFakeSession{{
		promptFn: func(ctx context.Context, _, _ string, h agentbridge.EventHandler) (string, error) {
			_, err := h.OnPermission(ctx, "rm -rf build/", []agentbridge.PermissionOption{
				{OptionID: "allow", Kind: "allow_once"},
				{OptionID: "deny", Kind: "reject_once"},
			})
			done <- err
			return "", err
		},
	}}
	res, err := e.svc.DispatchTask(context.Background(), "sess-1", "codex", "aranea", "清理")
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	e.bus.waitNoticeType(t, noticeCodingTaskApproval, 1)
	if err := e.svc.CancelTask(context.Background(), res.Task.ID); err != nil {
		t.Fatalf("cancel under awaiting_approval must not conflict: %v", err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("OnPermission must be rejected on cancel")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("OnPermission stuck after cancel (S5 regression)")
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		task, gerr := e.svc.GetTask(context.Background(), res.Task.ID)
		if gerr == nil && task.Status == agentbridge.StatusCancelled {
			break
		}
		if time.Now().After(deadline) {
			st := ""
			if task != nil {
				st = string(task.Status)
			}
			t.Fatalf("task did not reach cancelled (status=%s)", st)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestAgentBridgeService_ConfirmFromStepForgeryRejected S6（M4 P0-3 golden）：
// 确认卡强绑定——前缀/任务/会话/cardId 任一项不符即拒绝（不释放真实挂起审批）；
// 真实卡经 step_id+会话+任务全绑定校验后放行。
func TestAgentBridgeService_ConfirmFromStepForgeryRejected(t *testing.T) {
	e := newABSvcEnv()
	e.seedAgent("codex")
	e.seedProject("aranea", `F:\aranea`)
	e.svc.SetApprovalTimeout(2 * time.Second)

	started := make(chan struct{})
	e.factory.sessions = []*abFakeSession{{
		promptFn: func(ctx context.Context, _, _ string, h agentbridge.EventHandler) (string, error) {
			close(started)
			_, err := h.OnPermission(ctx, "rm -rf /", bridgePermOpts)
			return "", err
		},
	}}
	res, err := e.svc.DispatchTask(context.Background(), "sess-1", "codex", "aranea", "危险")
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	<-started
	notices := e.bus.waitNoticeType(t, noticeCodingTaskApproval, 1)
	stepID, _ := notices[0].Meta["step_id"].(string)
	if stepID == "" {
		t.Fatal("approval notice must carry step_id")
	}
	mkStep := func(id, sessionID, taskID string) biz.Step {
		args, _ := json.Marshal(map[string]any{
			"source": agentbridge.ToolExternalCoding, "agent_key": "codex",
			"task_id": res.Task.ID, "title": "rm -rf /", "target": "rm -rf /",
		})
		return biz.Step{
			ID: id, TurnID: id, TaskID: taskID, SessionID: sessionID,
			Kind: biz.StepKindConfirm, ToolName: agentbridge.ToolExternalCoding,
			ToolArgs: args, Status: biz.StepStatusToolBlocked,
		}
	}
	// ①缺桥接卡前缀（普通内部确认伪冒桥接决议）
	if err := e.svc.ConfirmBridgePermissionFromStep(context.Background(), mkStep("step-foreign-1", "sess-1", res.Task.ID), "", true); err == nil {
		t.Fatal("card without bridge-confirm- prefix must be rejected")
	}
	// ②会话不符（跨会话重放）
	if err := e.svc.ConfirmBridgePermissionFromStep(context.Background(), mkStep(stepID, "sess-OTHER", res.Task.ID), "", true); err == nil {
		t.Fatal("card session mismatch must be rejected")
	}
	// ③任务绑定不符（step.TaskID ≠ args.task_id）
	if err := e.svc.ConfirmBridgePermissionFromStep(context.Background(), mkStep(stepID, "sess-1", "task-OTHER"), "", true); err == nil {
		t.Fatal("card task binding mismatch must be rejected")
	}
	// ④card id 与 pending 不符（伪造 stepID）
	if err := e.svc.ConfirmBridgePermissionFromStep(context.Background(), mkStep("bridge-confirm-forged", "sess-1", res.Task.ID), "", true); err == nil {
		t.Fatal("forged card id must be rejected")
	}
	// ⑤真实卡（全绑定一致）放行
	if err := e.svc.ConfirmBridgePermissionFromStep(context.Background(), mkStep(stepID, "sess-1", res.Task.ID), "", true); err != nil {
		t.Fatalf("authentic card must pass: %v", err)
	}
}

func TestApprovalSpokenPrompt(t *testing.T) {
	got := approvalSpokenPrompt("codex", "aranea", "go test")
	if got != "codex · aranea 想执行 go test，允许吗？" {
		t.Fatalf("spoken = %q", got)
	}
}

func TestIsExternalCodingConfirm(t *testing.T) {
	if !IsExternalCodingConfirm(biz.Step{ToolName: agentbridge.ToolExternalCoding, ToolArgs: json.RawMessage(`{"source":"other"}`)}) {
		t.Fatal("tool name should win")
	}
	if !IsExternalCodingConfirm(biz.Step{ToolName: "shell_exec", ToolArgs: json.RawMessage(`{"source":"external_coding"}`)}) {
		t.Fatal("source field should match")
	}
	if IsExternalCodingConfirm(biz.Step{ToolName: "shell_exec", ToolArgs: json.RawMessage(`{"source":"internal"}`)}) {
		t.Fatal("internal source must not match")
	}
}
