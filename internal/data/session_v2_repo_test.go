package data

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

func TestSessionV2Repo_CreateAndGet(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewSessionV2Repo(d, loggateway.NewNoop())
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	created, err := repo.CreateSession(ctx, biz.SpiritSession{
		ID:            "sess-1",
		UserID:        "user-1",
		SpiritAgentID: "agent-1",
		Status:        biz.SpiritSessionStatusActive,
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if created.ID != "sess-1" || created.UserID != "user-1" {
		t.Fatalf("created mismatch: %+v", created)
	}

	got, err := repo.GetSession(ctx, "sess-1")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.Status != biz.SpiritSessionStatusActive {
		t.Fatalf("status: expected active, got %s", got.Status)
	}
	if got.SpiritAgentID != "agent-1" {
		t.Fatalf("spirit_agent_id: expected agent-1, got %s", got.SpiritAgentID)
	}
}

func TestSessionV2Repo_UpdateStatus(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewSessionV2Repo(d, loggateway.NewNoop())
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	_, _ = repo.CreateSession(ctx, biz.SpiritSession{
		ID:            "sess-2",
		UserID:        "u",
		SpiritAgentID: "a",
		Status:        biz.SpiritSessionStatusActive,
		CreatedAt:     now,
		UpdatedAt:     now,
	})

	if err := repo.UpdateSessionStatus(ctx, "sess-2", biz.SpiritSessionStatusCompleted); err != nil {
		t.Fatalf("UpdateSessionStatus: %v", err)
	}
	got, _ := repo.GetSession(ctx, "sess-2")
	if got.Status != biz.SpiritSessionStatusCompleted {
		t.Fatalf("expected completed, got %s", got.Status)
	}
}

func TestSessionV2Repo_GetSession_NotFound(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewSessionV2Repo(d, loggateway.NewNoop())
	ctx := context.Background()

	_, err := repo.GetSession(ctx, "nonexistent")
	if err == nil {
		t.Fatalf("expected error for nonexistent session, got nil")
	}
}
