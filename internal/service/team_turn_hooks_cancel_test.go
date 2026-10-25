package service

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/internal/biz"
)

// ── D1-F1：executeTeamTurnViaHooks 取消分流 ────────────────────────────────
// 87-orchestration-chain-hardening：用户停止导致的 runner 中断必须沿团队
// 回调落地 cancelled（根 Task cancelled、HandleTeamTurnResult(cancelled)、
// errMsg 保留原文），而不是 failed。CancelledTeams>0 → synthesis skipped
// （中断不出报告）依赖该终态。

// TestExecuteTeamTurnViaHooks_CancelledErrReportsCancelled pins: runner 返回
// context.Canceled（用户取消）→ 根 Task 终态为 cancelled 而非 failed,
// HandleTeamTurnResult 状态为 cancelled。
func TestExecuteTeamTurnViaHooks_CancelledErrReportsCancelled(t *testing.T) {
	taskWriter := &stubTaskV2Writer{}
	runner := &capturingTeamRunner{runErr: context.Canceled}
	starter := &capturingTeamStarter{}
	orch := newTeamTurnTestOrch(&stubTaskV2Repo{writer: taskWriter}, runner, starter)

	sess := biz.Session{ID: "team-sess-cancel-1", OwnerType: "team", TeamID: "team-1"}

	_, _, err := orch.executeTeamTurnViaHooks(contextWithPendingLoop(context.Background()), sess,
		biz.TurnInput{SessionID: sess.ID, Content: "跑到一半被用户停止"}, nil, func() {})
	if err == nil {
		t.Fatal("runner cancel error must propagate")
	}

	// 根 Task 终态 = cancelled（不是 failed）——用户取消不是失败。
	if len(taskWriter.terminalized) != 1 {
		t.Fatalf("CompleteTaskTerminal calls = %d, want 1", len(taskWriter.terminalized))
	}
	if got := taskWriter.terminalized[0].Status; got != biz.TaskStatusCancelled {
		t.Errorf("terminalized task Status = %q, want %q (user cancel is not failure)", got, biz.TaskStatusCancelled)
	}

	// 团队终态回调 = cancelled（errMsg 保留原文，供下游分类器）。
	if starter.calls != 1 {
		t.Fatalf("HandleTeamTurnResult calls = %d, want 1", starter.calls)
	}
	if starter.status != biz.TeamStatusCancelled {
		t.Errorf("HandleTeamTurnResult status = %q, want %q", starter.status, biz.TeamStatusCancelled)
	}
}

// TestExecuteTeamTurnViaHooks_GenericErrStillReportsFailed pins 回归边界：
// 非取消错误保持既有 failed 路径（根 Task failed + 回调 failed）。
func TestExecuteTeamTurnViaHooks_GenericErrStillReportsFailed(t *testing.T) {
	taskWriter := &stubTaskV2Writer{}
	runner := &capturingTeamRunner{runErr: errors.New("llm boom")}
	starter := &capturingTeamStarter{}
	orch := newTeamTurnTestOrch(&stubTaskV2Repo{writer: taskWriter}, runner, starter)

	sess := biz.Session{ID: "team-sess-fail-1", OwnerType: "team", TeamID: "team-1"}

	_, _, err := orch.executeTeamTurnViaHooks(contextWithPendingLoop(context.Background()), sess,
		biz.TurnInput{SessionID: sess.ID, Content: "ok"}, nil, func() {})
	if err == nil {
		t.Fatal("runner error must propagate")
	}

	if len(taskWriter.terminalized) != 1 {
		t.Fatalf("CompleteTaskTerminal calls = %d, want 1", len(taskWriter.terminalized))
	}
	if got := taskWriter.terminalized[0].Status; got != biz.TaskStatusFailed {
		t.Errorf("terminalized task Status = %q, want %q (generic error stays failed)", got, biz.TaskStatusFailed)
	}
	if starter.calls != 1 || starter.status != biz.TeamStatusFailed {
		t.Fatalf("HandleTeamTurnResult calls=%d status=%q, want 1/failed", starter.calls, starter.status)
	}
}
