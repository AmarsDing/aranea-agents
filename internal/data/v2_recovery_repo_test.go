package data

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// 2026-07-21 P1-5 修复：v2 实体 orphaned-recovery。
// 进程重启后 in-flight（pending/running/tool_running/paused）实体必须被
// 批量终态化，version+1，completed_at/finished_at 落上恢复时间；
// 已是终态的记录不得被触碰。
// 2026-07-22 L3：task → interrupted（可续跑），其余执行内部实体 → failed。

func TestV2RecoveryRepo_FailOrphanedInFlight(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewV2RecoveryRepo(d, loggateway.NewNoop())
	ctx := context.Background()
	start := time.Now().UTC().Truncate(time.Second)

	// --- 准备：每张表 1 条 in-flight + 1 条 terminal ---
	taskRepo := NewTaskV2Repo(d, loggateway.NewNoop())
	if _, err := taskRepo.CreateTask(ctx, biz.Task{ID: "t-orphan", SessionID: "s-1", Status: biz.TaskStatusRunning, Seq: 1, Version: 3, CreatedAt: start, UpdatedAt: start}); err != nil {
		t.Fatalf("seed task orphan: %v", err)
	}
	if _, err := taskRepo.CreateTask(ctx, biz.Task{ID: "t-done", SessionID: "s-1", Status: biz.TaskStatusCompleted, Seq: 2, Version: 2, CreatedAt: start, UpdatedAt: start}); err != nil {
		t.Fatalf("seed task done: %v", err)
	}

	turnRepo := NewTurnV2Repo(d, loggateway.NewNoop())
	if _, err := turnRepo.CreateTurn(ctx, biz.Turn{ID: "tn-orphan", TaskID: "t-orphan", SessionID: "s-1", SpiritSessionID: "s-1", Status: biz.TurnStatusRunning, StartedAt: start, Seq: 1, Version: 4}); err != nil {
		t.Fatalf("seed turn orphan: %v", err)
	}
	if _, err := turnRepo.CreateTurn(ctx, biz.Turn{ID: "tn-done", TaskID: "t-done", SessionID: "s-1", SpiritSessionID: "s-1", Status: biz.TurnStatusCompleted, StartedAt: start, Seq: 2, Version: 2}); err != nil {
		t.Fatalf("seed turn done: %v", err)
	}

	stepRepo := NewStepV2Repo(d, loggateway.NewNoop())
	if _, err := stepRepo.CreateStep(ctx, biz.Step{ID: "st-orphan", TurnID: "tn-orphan", TaskID: "t-orphan", SessionID: "s-1", SpiritSessionID: "s-1", Kind: biz.StepKindAction, Status: biz.StepStatusToolRunning, StartedAt: start, Seq: 1, Version: 5}); err != nil {
		t.Fatalf("seed step orphan: %v", err)
	}
	if _, err := stepRepo.CreateStep(ctx, biz.Step{ID: "st-done", TurnID: "tn-done", TaskID: "t-done", SessionID: "s-1", SpiritSessionID: "s-1", Kind: biz.StepKindReply, Status: biz.StepStatusCompleted, StartedAt: start, Seq: 2, Version: 2}); err != nil {
		t.Fatalf("seed step done: %v", err)
	}

	tsRepo := NewTeamStageV2Repo(d, loggateway.NewNoop())
	if _, err := tsRepo.CreateTeamStage(ctx, biz.TeamStage{ID: "ts-orphan", TaskID: "t-orphan", TurnID: "tn-orphan", SessionID: "s-1", TeamID: "team-1", Status: biz.TeamStageStatusRunning, Stage: biz.TeamStageStageExecuting, StartedAt: start, Seq: 1, Version: 6}); err != nil {
		t.Fatalf("seed team stage orphan: %v", err)
	}
	if _, err := tsRepo.CreateTeamStage(ctx, biz.TeamStage{ID: "ts-done", TaskID: "t-done", TurnID: "tn-done", SessionID: "s-1", TeamID: "team-1", Status: biz.TeamStageStatusCompleted, Stage: biz.TeamStageStageExecuting, StartedAt: start, Seq: 2, Version: 2}); err != nil {
		t.Fatalf("seed team stage done: %v", err)
	}

	trRepo := NewTeamRunV2Repo(d, loggateway.NewNoop())
	if _, err := trRepo.CreateTeamRun(ctx, biz.TeamRun{ID: "tr-orphan", TeamStageID: "ts-orphan", TaskID: "t-orphan", SessionID: "s-1", SpiritSessionID: "s-1", Status: biz.TeamRunV2StatusRunning, StartedAt: start, Seq: 1, Version: 7}); err != nil {
		t.Fatalf("seed team run orphan: %v", err)
	}
	if _, err := trRepo.CreateTeamRun(ctx, biz.TeamRun{ID: "tr-done", TeamStageID: "ts-done", TaskID: "t-done", SessionID: "s-1", SpiritSessionID: "s-1", Status: biz.TeamRunV2StatusCompleted, StartedAt: start, Seq: 2, Version: 2}); err != nil {
		t.Fatalf("seed team run done: %v", err)
	}

	msRepo := NewMemberSessionV2Repo(d, loggateway.NewNoop())
	if _, err := msRepo.CreateMemberSession(ctx, biz.MemberSession{ID: "ms-orphan", TeamRunID: "tr-orphan", TeamStageID: "ts-orphan", TaskID: "t-orphan", SessionID: "s-m1", SpiritSessionID: "s-1", AgentKey: "a-1", Status: biz.MemberSessionStatusRunning, StartedAt: start, Seq: 1, Version: 8}); err != nil {
		t.Fatalf("seed member session orphan: %v", err)
	}
	if _, err := msRepo.CreateMemberSession(ctx, biz.MemberSession{ID: "ms-done", TeamRunID: "tr-done", TeamStageID: "ts-done", TaskID: "t-done", SessionID: "s-m2", SpiritSessionID: "s-1", AgentKey: "a-2", Status: biz.MemberSessionStatusCompleted, StartedAt: start, Seq: 2, Version: 2}); err != nil {
		t.Fatalf("seed member session done: %v", err)
	}

	// --- 执行恢复 ---
	recoverAt := start.Add(time.Hour)
	stats, interrupted, err := repo.FailOrphanedInFlight(ctx, recoverAt)
	if err != nil {
		t.Fatalf("FailOrphanedInFlight: %v", err)
	}
	want := biz.V2RecoveryStats{Tasks: 1, Turns: 1, Steps: 1, TeamStages: 1, TeamRuns: 1, MemberSessions: 1}
	if stats != want {
		t.Fatalf("stats = %+v, want %+v", stats, want)
	}
	// L3: 被 interrupted 的 task 必须带回 ref（启动 guard 据此发 session notice）。
	if len(interrupted) != 1 || interrupted[0].TaskID != "t-orphan" || interrupted[0].SessionID != "s-1" {
		t.Fatalf("interrupted refs = %+v, want [t-orphan/s-1]", interrupted)
	}

	// --- 断言 in-flight 记录被终态化 ---
	task, _ := taskRepo.GetTask(ctx, "t-orphan")
	if task.Status != biz.TaskStatusInterrupted {
		t.Errorf("task status = %s, want interrupted (L3: resumable)", task.Status)
	}
	if task.Version != 4 {
		t.Errorf("task version = %d, want 4 (3+1)", task.Version)
	}
	if task.CompletedAt == nil || !task.CompletedAt.Equal(recoverAt) {
		t.Errorf("task completed_at = %v, want %v", task.CompletedAt, recoverAt)
	}
	if !task.UpdatedAt.Equal(recoverAt) {
		t.Errorf("task updated_at = %v, want %v", task.UpdatedAt, recoverAt)
	}

	turn, _ := turnRepo.GetTurn(ctx, "tn-orphan")
	if turn.Status != biz.TurnStatusFailed {
		t.Errorf("turn status = %s, want failed", turn.Status)
	}
	if turn.Version != 5 {
		t.Errorf("turn version = %d, want 5 (4+1)", turn.Version)
	}
	if turn.CompletedAt == nil || !turn.CompletedAt.Equal(recoverAt) {
		t.Errorf("turn completed_at = %v, want %v", turn.CompletedAt, recoverAt)
	}

	step, _ := stepRepo.GetStep(ctx, "st-orphan")
	if step.Status != biz.StepStatusFailed {
		t.Errorf("step status = %s, want failed", step.Status)
	}
	if step.Version != 6 {
		t.Errorf("step version = %d, want 6 (5+1)", step.Version)
	}
	if step.CompletedAt == nil || !step.CompletedAt.Equal(recoverAt) {
		t.Errorf("step completed_at = %v, want %v", step.CompletedAt, recoverAt)
	}

	ts, _ := tsRepo.GetTeamStage(ctx, "ts-orphan")
	if ts.Status != biz.TeamStageStatusFailed {
		t.Errorf("team stage status = %s, want failed", ts.Status)
	}
	if ts.Version != 7 {
		t.Errorf("team stage version = %d, want 7 (6+1)", ts.Version)
	}
	if ts.CompletedAt == nil || !ts.CompletedAt.Equal(recoverAt) {
		t.Errorf("team stage completed_at = %v, want %v", ts.CompletedAt, recoverAt)
	}

	tr, _ := trRepo.GetTeamRun(ctx, "tr-orphan")
	if tr.Status != biz.TeamRunV2StatusFailed {
		t.Errorf("team run status = %s, want failed", tr.Status)
	}
	if tr.Version != 8 {
		t.Errorf("team run version = %d, want 8 (7+1)", tr.Version)
	}
	if tr.CompletedAt == nil || !tr.CompletedAt.Equal(recoverAt) {
		t.Errorf("team run completed_at = %v, want %v", tr.CompletedAt, recoverAt)
	}
	if tr.Error == "" {
		t.Errorf("team run error should record recovery reason")
	}

	ms, _ := msRepo.GetMemberSession(ctx, "ms-orphan")
	if ms.Status != biz.MemberSessionStatusFailed {
		t.Errorf("member session status = %s, want failed", ms.Status)
	}
	if ms.Version != 9 {
		t.Errorf("member session version = %d, want 9 (8+1)", ms.Version)
	}
	if ms.FinishedAt == nil || !ms.FinishedAt.Equal(recoverAt) {
		t.Errorf("member session finished_at = %v, want %v", ms.FinishedAt, recoverAt)
	}
	if ms.Error == "" {
		t.Errorf("member session error should record recovery reason")
	}

	// --- 断言 terminal 记录未被触碰 ---
	taskDone, _ := taskRepo.GetTask(ctx, "t-done")
	if taskDone.Status != biz.TaskStatusCompleted || taskDone.Version != 2 {
		t.Errorf("terminal task touched: status=%s version=%d", taskDone.Status, taskDone.Version)
	}
	stepDone, _ := stepRepo.GetStep(ctx, "st-done")
	if stepDone.Status != biz.StepStatusCompleted || stepDone.Version != 2 {
		t.Errorf("terminal step touched: status=%s version=%d", stepDone.Status, stepDone.Version)
	}
	trDone, _ := trRepo.GetTeamRun(ctx, "tr-done")
	if trDone.Status != biz.TeamRunV2StatusCompleted || trDone.Version != 2 {
		t.Errorf("terminal team run touched: status=%s version=%d", trDone.Status, trDone.Version)
	}
}

func TestV2RecoveryRepo_FailOrphanedInFlight_NoOrphans(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewV2RecoveryRepo(d, loggateway.NewNoop())
	ctx := context.Background()

	stats, interrupted, err := repo.FailOrphanedInFlight(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("FailOrphanedInFlight: %v", err)
	}
	if stats != (biz.V2RecoveryStats{}) {
		t.Fatalf("stats = %+v, want zero", stats)
	}
	if len(interrupted) != 0 {
		t.Fatalf("interrupted refs = %+v, want empty", interrupted)
	}
}
