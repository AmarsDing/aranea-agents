package service

import (
	"context"
	"errors"
	"testing"

	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/pkg/loggateway"
)

// ── stubs ──────────────────────────────────────────────────────────────────

// capturingTeamRunner implements biz.TeamRunnerWirePort, capturing the
// RootTaskActivityID visible in the turn ctx so tests can assert the team
// path entry injected it (S-5). runErr simulates a runner failure.
type capturingTeamRunner struct {
	rootTaskID string
	calls      int
	runErr     error
}

func (s *capturingTeamRunner) RunTurnFromInput(ctx context.Context, _ biz.Session, _ biz.TurnInput) (biz.ChatMessage, biz.ChatMessage, error) {
	s.calls++
	s.rootTaskID = string(chatagent.RootTaskActivityIDFromCtx(ctx))
	if s.runErr != nil {
		return biz.ChatMessage{ID: "u-1"}, biz.ChatMessage{}, s.runErr
	}
	return biz.ChatMessage{ID: "u-1"}, biz.ChatMessage{ID: "a-1"}, nil
}
func (s *capturingTeamRunner) SetMediator(biz.TeamMediatorPort)               {}
func (s *capturingTeamRunner) SetAwaitHookProvider(biz.AwaitHookProvider)     {}
func (s *capturingTeamRunner) SetDeliverableGate(biz.TeamDeliverableGateFunc) {}
func (s *capturingTeamRunner) SetQualityGate(biz.TeamQualityGateFunc)         {}
func (s *capturingTeamRunner) SetRevisionEnqueuer(biz.TeamRevisionEnqueuerFunc) {
}
func (s *capturingTeamRunner) SetNoProgressEnqueuer(biz.TeamRevisionEnqueuerFunc) {
}
func (s *capturingTeamRunner) SetUpstreamDeliverableSeed(biz.TeamUpstreamSeedFunc) {
}

// capturingTeamStarter implements biz.TeamStarterPort, capturing the
// RootTaskActivityID visible in the HandleTeamTurnResult ctx.
type capturingTeamStarter struct {
	rootTaskID string
	status     string
	calls      int
}

func (s *capturingTeamStarter) StartTeamTurn(context.Context, string, string) error { return nil }
func (s *capturingTeamStarter) HandleTeamTurnResult(ctx context.Context, _, _, status, _, _ string) {
	s.calls++
	s.status = status
	s.rootTaskID = string(chatagent.RootTaskActivityIDFromCtx(ctx))
}

func newTeamTurnTestOrch(taskRepo biz.TaskV2Repo, runner biz.TeamRunnerWirePort, starter biz.TeamStarterPort) *ChatOrchestrator {
	return &ChatOrchestrator{
		core: chatTurnCoreDeps{TaskV2: taskRepo},
		teamExecDeps: ChatTeamDeps{
			Team:        TeamOrchestrationDeps{TeamsNative: runner},
			TeamStarter: starter,
		},
		runs:      rt.NewRunRegistry(),
		turnLC:    newNoopChatTurnLifecycle(),
		runMgr:    newNoopChatRunManager(),
		infraDeps: ChatInfraDeps{LG: loggateway.NewNoop()},
	}
}

// ── S-5 tests ────────────────────────────────────────────────────────────────

// TestExecuteTeamTurnViaHooks_InjectsRootTaskID verifies the team chat entry
// injects RootTaskActivityID into the turn ctx (S-5). Without it the runner
// and service terminal pass derive v2 IDs with an empty run dimension, and
// every turn on the same team collides on one team_stages_v2 row.
func TestExecuteTeamTurnViaHooks_InjectsRootTaskID(t *testing.T) {
	taskWriter := &stubTaskV2Writer{}
	runner := &capturingTeamRunner{}
	starter := &capturingTeamStarter{}
	orch := newTeamTurnTestOrch(&stubTaskV2Repo{writer: taskWriter}, runner, starter)

	sess := biz.Session{ID: "team-sess-1", OwnerType: "team", TeamID: "team-1"}
	input := biz.TurnInput{SessionID: sess.ID, Content: "验证消息"}

	if _, _, err := orch.executeTeamTurnViaHooks(contextWithPendingLoop(context.Background()), sess, input, nil, func() {}); err != nil {
		t.Fatalf("executeTeamTurnViaHooks: %v", err)
	}
	if runner.calls != 1 {
		t.Fatalf("RunTurnFromInput calls = %d, want 1", runner.calls)
	}
	if runner.rootTaskID == "" {
		t.Error("RootTaskActivityID empty in runner turn ctx (S-5 injection missing)")
	}
	if starter.calls != 1 || starter.status != "completed" {
		t.Fatalf("HandleTeamTurnResult calls=%d status=%q, want 1/completed", starter.calls, starter.status)
	}
	if starter.rootTaskID != runner.rootTaskID {
		t.Errorf("starter ctx rootTaskID = %q, want same as runner %q", starter.rootTaskID, runner.rootTaskID)
	}
}

// TestExecuteTeamTurnViaHooks_CreatesRootTaskV2 verifies the team chat entry
// persists a root Task v2 row whose ID equals the ctx RootTaskActivityID, so
// the turn tree hydrates under a real tasks_v2 row instead of a ghost ID.
func TestExecuteTeamTurnViaHooks_CreatesRootTaskV2(t *testing.T) {
	taskWriter := &stubTaskV2Writer{}
	runner := &capturingTeamRunner{}
	starter := &capturingTeamStarter{}
	orch := newTeamTurnTestOrch(&stubTaskV2Repo{writer: taskWriter}, runner, starter)

	sess := biz.Session{ID: "team-sess-2", OwnerType: "team", TeamID: "team-1"}
	input := biz.TurnInput{SessionID: sess.ID, Content: "第一轮"}

	if _, _, err := orch.executeTeamTurnViaHooks(contextWithPendingLoop(context.Background()), sess, input, nil, func() {}); err != nil {
		t.Fatalf("executeTeamTurnViaHooks: %v", err)
	}
	if len(taskWriter.upserted) != 1 {
		t.Fatalf("UpsertTask calls = %d, want 1 (root task for team turn)", len(taskWriter.upserted))
	}
	task := taskWriter.upserted[0]
	if task.ID == "" || task.ID != runner.rootTaskID {
		t.Errorf("root task ID = %q, want ctx rootTaskID %q", task.ID, runner.rootTaskID)
	}
	if task.SessionID != sess.ID {
		t.Errorf("root task SessionID = %q, want spirit root %q (standalone fallback)", task.SessionID, sess.ID)
	}
	if task.Status != biz.TaskStatusRunning {
		t.Errorf("root task Status = %q, want running", task.Status)
	}
	if task.UserMessage != "第一轮" {
		t.Errorf("root task UserMessage = %q, want turn content", task.UserMessage)
	}
}

// TestExecuteTeamTurnViaHooks_RootTaskIDIsolation verifies two user turns on
// the same team session get DIFFERENT root task IDs (run isolation
// precondition for S-3), while a continuation turn (ParentTaskID set)
// inherits the parent ID and does not create a new task.
func TestExecuteTeamTurnViaHooks_RootTaskIDIsolation(t *testing.T) {
	taskWriter := &stubTaskV2Writer{}
	runner := &capturingTeamRunner{}
	starter := &capturingTeamStarter{}
	orch := newTeamTurnTestOrch(&stubTaskV2Repo{writer: taskWriter}, runner, starter)

	sess := biz.Session{ID: "team-sess-3", OwnerType: "team", TeamID: "team-1"}

	if _, _, err := orch.executeTeamTurnViaHooks(contextWithPendingLoop(context.Background()), sess, biz.TurnInput{SessionID: sess.ID, Content: "第一轮"}, nil, func() {}); err != nil {
		t.Fatalf("turn 1: %v", err)
	}
	firstID := runner.rootTaskID

	if _, _, err := orch.executeTeamTurnViaHooks(contextWithPendingLoop(context.Background()), sess, biz.TurnInput{SessionID: sess.ID, Content: "第二轮"}, nil, func() {}); err != nil {
		t.Fatalf("turn 2: %v", err)
	}
	secondID := runner.rootTaskID
	if firstID == "" || secondID == "" || firstID == secondID {
		t.Errorf("root task IDs must differ across turns: first=%q second=%q", firstID, secondID)
	}
	if len(taskWriter.upserted) != 2 {
		t.Fatalf("UpsertTask calls = %d, want 2 (one root task per user turn)", len(taskWriter.upserted))
	}

	// Continuation turn: inherits ParentTaskID, no new task row.
	if _, _, err := orch.executeTeamTurnViaHooks(contextWithPendingLoop(context.Background()), sess, biz.TurnInput{SessionID: sess.ID, Content: "续跑", ParentTaskID: firstID}, nil, func() {}); err != nil {
		t.Fatalf("continuation turn: %v", err)
	}
	if runner.rootTaskID != firstID {
		t.Errorf("continuation rootTaskID = %q, want inherited parent %q", runner.rootTaskID, firstID)
	}
	if len(taskWriter.upserted) != 2 {
		t.Errorf("continuation turn must not create a new task, UpsertTask calls = %d", len(taskWriter.upserted))
	}
}

// TestExecuteTeamTurnViaHooks_TerminalizesRootTask verifies the root Task v2
// created for a team chat turn reaches a terminal status when the turn ends —
// otherwise startup recovery would mark every finished team task
// "interrupted" on the next process restart.
func TestExecuteTeamTurnViaHooks_TerminalizesRootTask(t *testing.T) {
	sess := biz.Session{ID: "team-sess-4", OwnerType: "team", TeamID: "team-1"}

	t.Run("success completes task", func(t *testing.T) {
		taskWriter := &stubTaskV2Writer{}
		runner := &capturingTeamRunner{}
		orch := newTeamTurnTestOrch(&stubTaskV2Repo{writer: taskWriter}, runner, &capturingTeamStarter{})

		if _, _, err := orch.executeTeamTurnViaHooks(contextWithPendingLoop(context.Background()), sess, biz.TurnInput{SessionID: sess.ID, Content: "ok"}, nil, func() {}); err != nil {
			t.Fatalf("executeTeamTurnViaHooks: %v", err)
		}
		if len(taskWriter.terminalized) != 1 {
			t.Fatalf("CompleteTaskTerminal calls = %d, want 1", len(taskWriter.terminalized))
		}
		term := taskWriter.terminalized[0]
		if term.ID != runner.rootTaskID {
			t.Errorf("terminalized task ID = %q, want root task %q", term.ID, runner.rootTaskID)
		}
		if term.Status != biz.TaskStatusCompleted {
			t.Errorf("terminalized task Status = %q, want completed", term.Status)
		}
	})

	t.Run("runner error fails task", func(t *testing.T) {
		taskWriter := &stubTaskV2Writer{}
		runner := &capturingTeamRunner{runErr: errors.New("boom")}
		orch := newTeamTurnTestOrch(&stubTaskV2Repo{writer: taskWriter}, runner, &capturingTeamStarter{})

		if _, _, err := orch.executeTeamTurnViaHooks(contextWithPendingLoop(context.Background()), sess, biz.TurnInput{SessionID: sess.ID, Content: "ok"}, nil, func() {}); err == nil {
			t.Fatal("expected runner error to propagate")
		}
		if len(taskWriter.terminalized) != 1 {
			t.Fatalf("CompleteTaskTerminal calls = %d, want 1", len(taskWriter.terminalized))
		}
		if taskWriter.terminalized[0].Status != biz.TaskStatusFailed {
			t.Errorf("terminalized task Status = %q, want failed", taskWriter.terminalized[0].Status)
		}
	})
}
