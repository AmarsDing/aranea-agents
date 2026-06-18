package data

import (
	"context"
	"database/sql"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	_ "github.com/glebarez/go-sqlite/compat"
)

func newChannelTurnJobTestRepo(t *testing.T, dbName string) (biz.ChannelTurnJobRepo, *sql.DB) {
	t.Helper()
	ctx := context.Background()
	db, err := sql.Open("sqlite", "file:"+dbName+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	if err := EnsureChannelTurnJobSchema(ctx, db); err != nil {
		t.Fatal(err)
	}
	// Initialize lg to avoid nil pointer panic in ExecInTx (tx.go:22).
	repo := NewChannelTurnJobRepo(&Data{rawDB: db, readDB: db, rwDB: NewReadWriteDB(db, db), lg: loggateway.NewNoop()})
	return repo, db
}

func TestChannelTurnJobCreateReturnsStableIDOnConflict(t *testing.T) {
	ctx := context.Background()
	repo, db := newChannelTurnJobTestRepo(t, "channel_turn_job_test")
	defer db.Close()

	firstID, err := repo.Create(ctx, biz.ChannelTurnJob{
		ID:             "job-1",
		ChannelID:      "ch-1",
		IdempotencyKey: "idem-1",
		Status:         biz.ChannelTurnJobStatusAccepted,
		CreatedAt:      biz.ChannelTurnJobNow(),
		UpdatedAt:      biz.ChannelTurnJobNow(),
	})
	if err != nil {
		t.Fatal(err)
	}
	secondID, err := repo.Create(ctx, biz.ChannelTurnJob{
		ID:             "job-2",
		ChannelID:      "ch-1",
		IdempotencyKey: "idem-1",
		Status:         biz.ChannelTurnJobStatusAccepted,
		CreatedAt:      biz.ChannelTurnJobNow(),
		UpdatedAt:      biz.ChannelTurnJobNow(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if firstID != secondID || firstID != "job-1" {
		t.Fatalf("expected stable id job-1, got %q then %q", firstID, secondID)
	}
}

func TestChannelTurnJobCreatePreservesAsyncQueuedOnConflict(t *testing.T) {
	ctx := context.Background()
	repo, db := newChannelTurnJobTestRepo(t, "channel_turn_job_async_test")
	defer db.Close()

	id, err := repo.Create(ctx, biz.ChannelTurnJob{
		ID:             "job-a",
		ChannelID:      "ch-1",
		IdempotencyKey: "idem-a",
		Status:         biz.ChannelTurnJobStatusAccepted,
		CreatedAt:      biz.ChannelTurnJobNow(),
		UpdatedAt:      biz.ChannelTurnJobNow(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateStatus(ctx, id, biz.ChannelTurnJobStatusAsyncQueued, "", "", ""); err != nil {
		t.Fatal(err)
	}
	_, err = repo.Create(ctx, biz.ChannelTurnJob{
		ID:             "job-b",
		ChannelID:      "ch-1",
		IdempotencyKey: "idem-a",
		Status:         biz.ChannelTurnJobStatusAccepted,
		CreatedAt:      biz.ChannelTurnJobNow(),
		UpdatedAt:      biz.ChannelTurnJobNow(),
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetByIdempotency(ctx, "ch-1", "idem-a")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != biz.ChannelTurnJobStatusAsyncQueued {
		t.Fatalf("status = %q, want async_queued preserved on conflict", got.Status)
	}
}

func TestChannelTurnJobUpdateStatusQueued(t *testing.T) {
	ctx := context.Background()
	repo, db := newChannelTurnJobTestRepo(t, "channel_turn_job_status_test")
	defer db.Close()

	id, err := repo.Create(ctx, biz.ChannelTurnJob{
		ID:             "job-q",
		ChannelID:      "ch-1",
		IdempotencyKey: "idem-q",
		Status:         biz.ChannelTurnJobStatusRunning,
		CreatedAt:      biz.ChannelTurnJobNow(),
		UpdatedAt:      biz.ChannelTurnJobNow(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateStatus(ctx, id, biz.ChannelTurnJobStatusQueued, "", "", ""); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetByIdempotency(ctx, "ch-1", "idem-q")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != biz.ChannelTurnJobStatusQueued {
		t.Fatalf("status = %q, want queued", got.Status)
	}
	if got.FinishedAt == "" {
		t.Fatal("expected finished_at for queued state")
	}
}
