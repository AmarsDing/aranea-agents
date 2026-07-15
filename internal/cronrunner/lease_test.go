package cronrunner

import (
	"context"
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"
)

func TestMemTaskLease_TwoRunnersCannotBothHold(t *testing.T) {
	lease := newMemTaskLease()
	release1, ok1 := lease.TryAcquire(context.Background(), "task-a")
	if !ok1 {
		t.Fatal("first acquire should succeed")
	}
	_, ok2 := lease.TryAcquire(context.Background(), "task-a")
	if ok2 {
		t.Fatal("second acquire should fail while first holds")
	}
	release1()
	release3, ok3 := lease.TryAcquire(context.Background(), "task-a")
	if !ok3 {
		t.Fatal("acquire after release should succeed")
	}
	release3()
}

func TestExecuteTask_SkipsWhenLeaseHeld(t *testing.T) {
	repo := newMemCronExecRepo()
	shared := newMemTaskLease()

	r2 := NewRunner(Deps{Cron: repo}, loggateway.NewNoop()).WithLease(shared)

	cfg, err := parseCronTaskConfig(repo.tasks["t1"].ConfigJSON)
	if err != nil {
		t.Fatal(err)
	}
	task := repo.tasks["t1"]
	meta := parseCronTaskMetadata(task.MetadataJSON, loggateway.NewNoop())
	now := time.Now().UTC()

	release, ok := shared.TryAcquire(context.Background(), task.ID)
	if !ok {
		t.Fatal("setup acquire failed")
	}
	defer release()

	id := r2.executeTask(context.Background(), task, cfg, meta, now, "schedule")
	if id != "" {
		t.Fatal("r2 should skip when lease held")
	}
	if len(repo.runs) != 0 {
		t.Fatalf("expected no runs inserted while lease held, got %d", len(repo.runs))
	}
}

func TestAcquireTaskLease_SharedAcrossRunners(t *testing.T) {
	shared := newMemTaskLease()
	r1 := NewRunner(Deps{}, loggateway.NewNoop()).WithLease(shared)
	r2 := NewRunner(Deps{}, loggateway.NewNoop()).WithLease(shared)

	rel1, ok := r1.acquireTaskLease(context.Background(), "job-1")
	if !ok {
		t.Fatal("r1 should acquire")
	}
	_, ok = r2.acquireTaskLease(context.Background(), "job-1")
	if ok {
		t.Fatal("r2 must not acquire while r1 holds")
	}
	rel1()
	rel2, ok := r2.acquireTaskLease(context.Background(), "job-1")
	if !ok {
		t.Fatal("r2 should acquire after r1 release")
	}
	rel2()
}

func TestAlwaysHeldLease_AllowsAcquireWithoutDB(t *testing.T) {
	r := NewRunner(Deps{}, loggateway.NewNoop()) // DB nil → alwaysHeldLease
	rel, ok := r.acquireTaskLease(context.Background(), "any")
	if !ok {
		t.Fatal("nil DB should allow acquire (process mutex only)")
	}
	rel()
}
