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
