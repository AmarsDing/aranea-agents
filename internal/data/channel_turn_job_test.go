package data

import (
	"context"
	"sync"
	"testing"
	"time"

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

func TestChannelTurnJobTransitionIfStale_ConcurrentOnlyOneWins(t *testing.T) {
	ctx := context.Background()
	repo := newChannelTurnJobTestRepo(t)
	stale := time.Now().UTC().Add(-31 * time.Minute).Format(time.RFC3339)
	id, err := repo.Create(ctx, biz.ChannelTurnJob{
		ID:             "job-claim-1",
		ChannelID:      "ch-1",
		IdempotencyKey: "idem-claim-1",
		Status:         biz.ChannelTurnJobStatusRunning,
		CreatedAt:      stale,
		UpdatedAt:      stale,
	})
	if err != nil {
		t.Fatal(err)
	}
	cutoff := time.Now().UTC().Add(-30 * time.Minute).Format(time.RFC3339)

	const n = 8
	var mu sync.Mutex
	var wins int
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			ok, err := repo.TransitionIfStale(ctx, id, biz.ChannelTurnJobStatusRunning, biz.ChannelTurnJobStatusFailed,
				"lease expired", "", "", cutoff)
			if err != nil {
				t.Errorf("TransitionIfStale: %v", err)
				return
			}
			if ok {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if wins != 1 {
		t.Fatalf("concurrent TransitionIfStale wins = %d, want 1", wins)
	}
	got, err := repo.GetByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != biz.ChannelTurnJobStatusFailed {
		t.Fatalf("status = %q, want failed", got.Status)
	}
}

func TestChannelTurnJobTransitionIfStale_FreshLeaseNotClaimed(t *testing.T) {
	ctx := context.Background()
	repo := newChannelTurnJobTestRepo(t)
	now := biz.ChannelTurnJobNow()
	id, err := repo.Create(ctx, biz.ChannelTurnJob{
		ID:             "job-fresh",
		ChannelID:      "ch-1",
		IdempotencyKey: "idem-fresh",
		Status:         biz.ChannelTurnJobStatusRunning,
		CreatedAt:      now,
		UpdatedAt:      now,
	})
	if err != nil {
		t.Fatal(err)
	}
	cutoff := time.Now().UTC().Add(-30 * time.Minute).Format(time.RFC3339)
	ok, err := repo.TransitionIfStale(ctx, id, biz.ChannelTurnJobStatusRunning, biz.ChannelTurnJobStatusFailed,
		"lease expired", "", "", cutoff)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("fresh running job must not be claimed by expired-lease cutoff")
	}
}
