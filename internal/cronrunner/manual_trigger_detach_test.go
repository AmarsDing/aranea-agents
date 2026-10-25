package cronrunner

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// ctxAwareCronExecRepo mimics real PG semantics: row writes fail when the
// caller ctx is already dead (the in-memory harness used by execute_test.go
// ignores ctx and cannot detect the E2E-H5 regression).
type ctxAwareCronExecRepo struct {
	*memCronExecRepo
}

func (m *ctxAwareCronExecRepo) UpdateCronTaskRun(ctx context.Context, id, status, finishedAt, outputJSON, errorMessage string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return m.memCronExecRepo.UpdateCronTaskRun(ctx, id, status, finishedAt, outputJSON, errorMessage)
}

// E2E-H5: TriggerTask must detach the async manual run from the HTTP
// request ctx (context.WithoutCancel, 样板 channel_ingress_async.go:205).
// Otherwise the handler's return cancels the turn and the finalize write
// dies with the same ctx — cron_task_run rows stayed "pending" forever
// (no stale/sweep reconciliation exists for manual runs).
func TestTriggerTask_SurvivesRequestCtxCancel(t *testing.T) {
	repo := &ctxAwareCronExecRepo{newMemCronExecRepo()}
	repo.mu.Lock()
	repo.tasks["t-manual"] = biz.CronTask{
		ID:         "t-manual",
		TaskKey:    "job",
		Name:       "Manual",
		Status:     "active",
		Enabled:    true,
		ConfigJSON: `{"message":"hi","schedule_type":"interval","interval_seconds":60,"retry_max_attempts":0}`,
	}
	repo.mu.Unlock()
	r := NewRunner(Deps{Cron: repo}, loggateway.NewNoop())

	ctx, cancel := context.WithCancel(context.Background())
	run, err := r.TriggerTask(ctx, "t-manual")
	if err != nil {
		t.Fatalf("TriggerTask: %v", err)
	}
	cancel() // simulate HTTP handler return right after ACK

	// The async turn itself will fail fast (no chat/session deps wired in this
	// harness) — what matters is the finalize write reaching a TERMINAL status
	// on the detached ctx rather than dying mid-flight on the cancelled one.
	deadline := time.Now().Add(3 * time.Second)
	for {
		repo.mu.Lock()
		got := repo.runs[run.ID]
		repo.mu.Unlock()
		if got.Status != "" && got.Status != "pending" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("run stuck in %q — async manual trigger died with the HTTP request ctx (E2E-H5)", got.Status)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
