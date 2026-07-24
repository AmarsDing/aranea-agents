package data

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

func newChannelTurnJobTestRepo(t *testing.T) biz.ChannelTurnJobRepo {
	t.Helper()
	ctx := context.Background()
	db := testhelper.SetupTestPGRaw(t)
	if err := EnsureChannelTurnJobSchema(ctx, db); err != nil {
		t.Fatal(err)
	}
	// Initialize lg to avoid nil pointer panic in ExecInTx (tx.go:22).
	return NewChannelTurnJobRepo(&Data{rawDB: db, readDB: db, rwDB: NewReadWriteDB(db, db), lg: loggateway.NewNoop(), dialect: DialectPostgres})
}

func TestChannelTurnJobCreateReturnsStableIDOnConflict(t *testing.T) {
	ctx := context.Background()
	repo := newChannelTurnJobTestRepo(t)

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
	repo := newChannelTurnJobTestRepo(t)

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
	repo := newChannelTurnJobTestRepo(t)

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
