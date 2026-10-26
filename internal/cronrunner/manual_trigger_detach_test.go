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

// H5（V 桶）pin：WithoutCancel 连父 deadline 一起剥掉，裸 detach 会让 PG
// stall 的 goroutine 无界悬挂。detachManualTriggerCtx 必须在剥离取消传播的
// 同时叠 manualTriggerMaxDuration 上限，并保留父 ctx 的值。
func TestDetachManualTriggerCtxCapsDeadline(t *testing.T) {
	type ctxKey struct{}
	parent := context.WithValue(context.Background(), ctxKey{}, "v")

	bg, cancel := detachManualTriggerCtx(parent)
	defer cancel()

	dl, ok := bg.Deadline()
	if !ok {
		t.Fatal("detached ctx must carry a cap deadline (H5: bare WithoutCancel hangs forever on a stalled dependency)")
	}
	if d := time.Until(dl); d <= 0 || d > manualTriggerMaxDuration {
		t.Fatalf("deadline delta = %v, want within (0, %v]", d, manualTriggerMaxDuration)
	}
	if got := bg.Value(ctxKey{}); got != "v" {
		t.Fatalf("WithoutCancel value propagation broken: got %v", got)
	}

	// 父取消不得传播（E2E-H5 语义不回退）。
	parent2, pcancel := context.WithCancel(context.Background())
	bg2, cancel2 := detachManualTriggerCtx(parent2)
	defer cancel2()
	pcancel()
	if err := bg2.Err(); err != nil {
		t.Fatalf("parent cancel leaked into detached ctx: %v", err)
	}
}
