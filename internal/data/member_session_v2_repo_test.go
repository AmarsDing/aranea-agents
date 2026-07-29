package data

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

func TestMemberSessionV2Repo_CreateAndGet(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewMemberSessionV2Repo(d, loggateway.NewNoop())
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	created, err := repo.CreateMemberSession(ctx, biz.MemberSession{
		ID: "ms-1", TeamRunID: "tr-1", TeamStageID: "ts-1", TaskID: "t-1",
		SessionID: "s-member-1", SpiritSessionID: "s-1",
		AgentKey: "agent-1", AgentName: "Agent One", AvatarURL: "http://avatar/1.png",
		Status:    biz.MemberSessionStatusRunning,
		StartedAt: now, Seq: 1, Version: 1,
	})
	if err != nil {
		t.Fatalf("CreateMemberSession: %v", err)
	}
	if created.ID != "ms-1" || created.AgentKey != "agent-1" {
		t.Fatalf("created mismatch: %+v", created)
	}

	got, err := repo.GetMemberSession(ctx, "ms-1")
	if err != nil {
		t.Fatalf("GetMemberSession: %v", err)
	}
	if got.AgentKey != "agent-1" || got.AgentName != "Agent One" || got.AvatarURL != "http://avatar/1.png" {
		t.Fatalf("member session mismatch: %+v", got)
	}
	if got.Status != biz.MemberSessionStatusRunning || got.SpiritSessionID != "s-1" {
		t.Fatalf("status/spirit mismatch: %+v", got)
	}
}

func TestMemberSessionV2Repo_Upsert_VersionGuard_WithFinishedAt(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewMemberSessionV2Repo(d, loggateway.NewNoop())
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	_, err := repo.UpsertMemberSession(ctx, biz.MemberSession{
		ID: "ms-2", TeamRunID: "tr-1", TeamStageID: "ts-1", TaskID: "t-1",
		SessionID: "s-member-2", SpiritSessionID: "s-1",
		AgentKey: "agent-2", Status: biz.MemberSessionStatusRunning,
		StartedAt: now, Seq: 2, Version: 1,
	})
	if err != nil {
		t.Fatalf("UpsertMemberSession v1: %v", err)
	}
	// Stale version (0) should not overwrite v1.
	stale, err := repo.UpsertMemberSession(ctx, biz.MemberSession{
		ID: "ms-2", TeamRunID: "tr-1", TeamStageID: "ts-1", TaskID: "t-1",
		SessionID: "s-member-2", SpiritSessionID: "s-1",
		AgentKey: "stale", Status: biz.MemberSessionStatusFailed, Error: "stale error",
		StartedAt: now, Seq: 2, Version: 0,
	})
	if err != nil {
		t.Fatalf("UpsertMemberSession stale: %v", err)
	}
	if stale.Status != biz.MemberSessionStatusRunning || stale.Error != "" {
		t.Fatalf("stale overwrote: %+v", stale)
	}
	// Newer version (2) should overwrite with FinishedAt.
	finishedAt := now.Add(10 * time.Second)
	_, err = repo.UpsertMemberSession(ctx, biz.MemberSession{
		ID: "ms-2", TeamRunID: "tr-1", TeamStageID: "ts-1", TaskID: "t-1",
		SessionID: "s-member-2", SpiritSessionID: "s-1",
		AgentKey: "agent-2", Status: biz.MemberSessionStatusCompleted,
		StartedAt: now, FinishedAt: &finishedAt,
		Seq: 2, Version: 2,
	})
	if err != nil {
		t.Fatalf("UpsertMemberSession v2: %v", err)
	}
	got, _ := repo.GetMemberSession(ctx, "ms-2")
	if got.Status != biz.MemberSessionStatusCompleted || got.Version != 2 {
		t.Fatalf("newer did not overwrite: %+v", got)
	}
	if got.FinishedAt == nil || !got.FinishedAt.Equal(finishedAt) {
		t.Fatalf("finishedAt mismatch: got %+v, expected %v", got.FinishedAt, finishedAt)
	}
}

// 回归（2026-07-29 outcome 哨兵化）：pause/resume 的 Version++ 可使 running
// 成员版本递增到原固定终态带（V=3），导致 outcome 终态事件被 VersionLT 守卫
// 静默拒绝（成员永久 running）。哨兵带（1<<40）必须恒赢任意递增版本，且
// 终态之后的迟到生命周期事件必须被拒绝。
func TestMemberSessionV2Repo_Upsert_OutcomeSentinelAlwaysWins(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewMemberSessionV2Repo(d, loggateway.NewNoop())
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	// created V=1 → pause V=2 → resume V=3（递增写者可越过原固定带 V=3）。
	for _, v := range []int64{1, 2, 3} {
		status := biz.MemberSessionStatusRunning
		if v == 2 {
			status = biz.MemberSessionStatusPaused
		}
		if _, err := repo.UpsertMemberSession(ctx, biz.MemberSession{
			ID: "ms-sentinel", TeamRunID: "tr-1", TeamStageID: "ts-1", TaskID: "t-1",
			SessionID: "s-member-s", SpiritSessionID: "s-1",
			AgentKey: "agent-s", Status: status,
			StartedAt: now, Seq: 1, Version: v,
		}); err != nil {
			t.Fatalf("UpsertMemberSession v%d: %v", v, err)
		}
	}
	// outcome 终态（哨兵带）必须覆盖 running V=3。
	finishedAt := now.Add(30 * time.Second)
	if _, err := repo.UpsertMemberSession(ctx, biz.MemberSession{
		ID: "ms-sentinel", TeamRunID: "tr-1", TeamStageID: "ts-1", TaskID: "t-1",
		SessionID: "s-member-s", SpiritSessionID: "s-1",
		AgentKey: "agent-s", Status: biz.MemberSessionStatusCompleted,
		StartedAt: now, FinishedAt: &finishedAt,
		Seq: 1, Version: biz.MemberSessionVersionOutcome,
	}); err != nil {
		t.Fatalf("UpsertMemberSession outcome: %v", err)
	}
	got, _ := repo.GetMemberSession(ctx, "ms-sentinel")
	if got.Status != biz.MemberSessionStatusCompleted || got.Version != biz.MemberSessionVersionOutcome {
		t.Fatalf("outcome sentinel did not win: %+v", got)
	}
	// 终态之后的迟到生命周期事件（V=4，高于原固定带）必须被拒绝。
	late, err := repo.UpsertMemberSession(ctx, biz.MemberSession{
		ID: "ms-sentinel", TeamRunID: "tr-1", TeamStageID: "ts-1", TaskID: "t-1",
		SessionID: "s-member-s", SpiritSessionID: "s-1",
		AgentKey: "agent-s", Status: biz.MemberSessionStatusPaused,
		StartedAt: now, Seq: 1, Version: 4,
	})
	if err != nil {
		t.Fatalf("UpsertMemberSession late: %v", err)
	}
	if late.Status != biz.MemberSessionStatusCompleted || late.Version != biz.MemberSessionVersionOutcome {
		t.Fatalf("late lifecycle event overwrote terminal: %+v", late)
	}
}

func TestMemberSessionV2Repo_ListByRun_SeqOrder(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewMemberSessionV2Repo(d, loggateway.NewNoop())
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	for i, seq := range []int64{3, 1, 2} {
		_, err := repo.CreateMemberSession(ctx, biz.MemberSession{
			ID: "ord-" + string(rune('a'+i)), TeamRunID: "tr-x", TeamStageID: "ts-1", TaskID: "t-1",
			SessionID: "s-" + string(rune('a'+i)), SpiritSessionID: "s-1",
			AgentKey: "agent-x", Status: biz.MemberSessionStatusPending,
			StartedAt: now, Seq: seq, Version: 1,
		})
		if err != nil {
			t.Fatalf("CreateMemberSession[%d]: %v", i, err)
		}
	}
	sessions, err := repo.ListMemberSessionsByRun(ctx, "tr-x")
	if err != nil {
		t.Fatalf("ListMemberSessionsByRun: %v", err)
	}
	if len(sessions) != 3 || sessions[0].Seq != 1 || sessions[2].Seq != 3 {
		t.Fatalf("order: expected 1,2,3 got %+v", []int64{sessions[0].Seq, sessions[1].Seq, sessions[2].Seq})
	}
}

func TestMemberSessionV2Repo_GetMemberSession_NotFound(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewMemberSessionV2Repo(d, loggateway.NewNoop())
	ctx := context.Background()

	_, err := repo.GetMemberSession(ctx, "nonexistent")
	if err == nil {
		t.Fatalf("expected error for nonexistent member session, got nil")
	}
}

func TestMemberSessionV2Repo_ListOrphanMemberSessionsBySpiritSession(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewMemberSessionV2Repo(d, loggateway.NewNoop())
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	// Team-scoped member (should be excluded).
	_, err := repo.CreateMemberSession(ctx, biz.MemberSession{
		ID: "ms-team", TeamRunID: "tr-1", TeamStageID: "ts-1", TaskID: "t-1",
		SessionID: "s-team", SpiritSessionID: "spirit-1",
		AgentKey: "coder", Status: biz.MemberSessionStatusRunning,
		StartedAt: now, Seq: 1, Version: 1,
	})
	if err != nil {
		t.Fatalf("CreateMemberSession team: %v", err)
	}
	// Mode B orphans for spirit-1.
	_, err = repo.CreateMemberSession(ctx, biz.MemberSession{
		ID: "ms-orphan-2", TeamRunID: "", TeamStageID: "", TaskID: "",
		SessionID: "s-child-2", SpiritSessionID: "spirit-1",
		AgentKey: "subagent:r2", AgentName: "Task B", Status: biz.MemberSessionStatusRunning,
		StartedAt: now, Seq: 2, Version: 1,
	})
	if err != nil {
		t.Fatalf("CreateMemberSession orphan2: %v", err)
	}
	_, err = repo.CreateMemberSession(ctx, biz.MemberSession{
		ID: "ms-orphan-1", TeamRunID: "", TeamStageID: "", TaskID: "",
		SessionID: "s-child-1", SpiritSessionID: "spirit-1",
		AgentKey: "subagent:r1", AgentName: "Task A", Status: biz.MemberSessionStatusCompleted,
		StartedAt: now, Seq: 1, Version: 1,
	})
	if err != nil {
		t.Fatalf("CreateMemberSession orphan1: %v", err)
	}
	// Orphan for a different spirit (should be excluded).
	_, err = repo.CreateMemberSession(ctx, biz.MemberSession{
		ID: "ms-other", TeamRunID: "", TeamStageID: "", TaskID: "",
		SessionID: "s-other", SpiritSessionID: "spirit-2",
		AgentKey: "subagent:rx", Status: biz.MemberSessionStatusRunning,
		StartedAt: now, Seq: 1, Version: 1,
	})
	if err != nil {
		t.Fatalf("CreateMemberSession other: %v", err)
	}

	orphans, err := repo.ListOrphanMemberSessionsBySpiritSession(ctx, "spirit-1")
	if err != nil {
		t.Fatalf("ListOrphanMemberSessionsBySpiritSession: %v", err)
	}
	if len(orphans) != 2 {
		t.Fatalf("want 2 orphans, got %d (%+v)", len(orphans), orphans)
	}
	if orphans[0].ID != "ms-orphan-1" || orphans[1].ID != "ms-orphan-2" {
		t.Fatalf("order/ids: got %s, %s", orphans[0].ID, orphans[1].ID)
	}
	if orphans[0].TeamRunID != "" || orphans[1].TeamRunID != "" {
		t.Fatalf("expected empty TeamRunID: %+v", orphans)
	}
}
