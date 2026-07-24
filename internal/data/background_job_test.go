package data

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz/backgroundjob"
)

// openTestData opens a Postgres backed Data instance (isolated test schema).
func openTestData(t *testing.T) *Data {
	t.Helper()
	return newTestDataPG(t)
}

func TestBackgroundJobRepo_CreateAndGet(t *testing.T) {
	d := openTestData(t)
	repo := NewBackgroundJobRepo(d)
	ctx := context.Background()

	req := backgroundjob.CreateRequest{
		Kind:        "test_job",
		OwnerType:   backgroundjob.OwnerTypeSession,
		OwnerID:     "sess-1",
		Priority:    backgroundjob.PriorityNormal,
		MaxAttempts: 3,
		Payload:     []byte(`{"key":"val"}`),
	}
	job, err := repo.Create(ctx, req)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if job.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if job.Status != backgroundjob.StatusQueued {
		t.Fatalf("expected queued, got %s", job.Status)
	}

	got, err := repo.Get(ctx, job.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Kind != "test_job" {
		t.Fatalf("expected kind=test_job, got %s", got.Kind)
	}
}

func TestBackgroundJobRepo_TryClaim(t *testing.T) {
	d := openTestData(t)
	repo := NewBackgroundJobRepo(d)
	ctx := context.Background()

	// Create two queued jobs.
	for _, kind := range []string{"k1", "k2"} {
		if _, err := repo.Create(ctx, backgroundjob.CreateRequest{
			Kind: kind, OwnerType: backgroundjob.OwnerTypeSystem, OwnerID: "sys",
		}); err != nil {
			t.Fatal(err)
		}
	}

	// Claim the first available.
	job, err := repo.TryClaim(ctx, "worker-1", []string{"k1", "k2"})
	if err != nil {
		t.Fatalf("TryClaim: %v", err)
	}
	if job == nil {
		t.Fatal("expected a claimed job")
	}
	if job.Status != backgroundjob.StatusClaimed {
		t.Fatalf("expected claimed, got %s", job.Status)
	}
	if job.Attempts != 1 {
		t.Fatalf("expected attempts=1, got %d", job.Attempts)
	}
}

func TestBackgroundJobRepo_FullLifecycle(t *testing.T) {
	d := openTestData(t)
	repo := NewBackgroundJobRepo(d)
	ctx := context.Background()

	job, _ := repo.Create(ctx, backgroundjob.CreateRequest{
		Kind: "lifecycle_test", OwnerType: backgroundjob.OwnerTypeChannel, OwnerID: "ch-1",
	})

	// Claim.
	claimed, err := repo.TryClaim(ctx, "w1", []string{"lifecycle_test"})
	if err != nil || claimed == nil {
		t.Fatalf("TryClaim: err=%v job=%v", err, claimed)
	}

	// Succeed.
	if err := repo.MarkSucceeded(ctx, claimed.ID); err != nil {
		t.Fatalf("MarkSucceeded: %v", err)
	}
	done, _ := repo.Get(ctx, job.ID)
	if done.Status != backgroundjob.StatusSucceeded {
		t.Fatalf("expected succeeded, got %s", done.Status)
	}
}

func TestBackgroundJobRepo_Cancel(t *testing.T) {
	d := openTestData(t)
	repo := NewBackgroundJobRepo(d)
	ctx := context.Background()

	job, _ := repo.Create(ctx, backgroundjob.CreateRequest{
		Kind: "cancel_test", OwnerType: backgroundjob.OwnerTypeSession, OwnerID: "sess-2",
	})
	if err := repo.Cancel(ctx, job.ID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	got, _ := repo.Get(ctx, job.ID)
	if got.Status != backgroundjob.StatusCancelled {
		t.Fatalf("expected cancelled, got %s", got.Status)
	}
}

func TestBackgroundJobRepo_ScheduledAt(t *testing.T) {
	d := openTestData(t)
	repo := NewBackgroundJobRepo(d)
	ctx := context.Background()

	// Create a job scheduled in the future.
	future := time.Now().Add(1 * time.Hour)
	if _, err := repo.Create(ctx, backgroundjob.CreateRequest{
		Kind: "future_job", OwnerType: backgroundjob.OwnerTypeSystem, OwnerID: "sys",
		ScheduledAt: future,
	}); err != nil {
		t.Fatal(err)
	}

	// TryClaim should return nil because job is not yet due.
	job, err := repo.TryClaim(ctx, "w1", []string{"future_job"})
	if err != nil {
		t.Fatalf("TryClaim: %v", err)
	}
	if job != nil {
		t.Fatalf("expected nil (future job not claimable), got %+v", job)
	}
}

func TestBackgroundJobRepo_CancelByOwner(t *testing.T) {
	d := openTestData(t)
	repo := NewBackgroundJobRepo(d)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if _, err := repo.Create(ctx, backgroundjob.CreateRequest{
			Kind: "bulk_cancel", OwnerType: backgroundjob.OwnerTypeChannel, OwnerID: "ch-99",
		}); err != nil {
			t.Fatal(err)
		}
	}

	n, err := repo.CancelByOwner(ctx, backgroundjob.OwnerTypeChannel, "ch-99")
	if err != nil {
		t.Fatalf("CancelByOwner: %v", err)
	}
	if n != 3 {
		t.Fatalf("expected 3 cancelled, got %d", n)
	}
}
