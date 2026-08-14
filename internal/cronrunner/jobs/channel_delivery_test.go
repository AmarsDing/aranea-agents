package jobs

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"
)

type fakePendingDeliveryProcessor struct {
	err  error
	call int
}

func (f *fakePendingDeliveryProcessor) ProcessPending(_ context.Context, _ int) error {
	f.call++
	return f.err
}

// blockingPendingDeliveryProcessor blocks until released, simulating a slow batch.
type blockingPendingDeliveryProcessor struct {
	entered  atomic.Int32
	release  chan struct{}
	released atomic.Bool
}

func (f *blockingPendingDeliveryProcessor) ProcessPending(_ context.Context, _ int) error {
	f.entered.Add(1)
	if f.released.CompareAndSwap(false, true) {
		close(f.release)
	}
	<-f.release
	return nil
}

// Overlapping ticks must not run ProcessPending concurrently (CH-R2): a second
// run while the first is still in flight is skipped, preventing duplicate sends.
func TestChannelDeliveryWorker_SingleFlightSkipsOverlap(t *testing.T) {
	proc := &blockingPendingDeliveryProcessor{release: make(chan struct{})}
	w := NewChannelDeliveryWorker(0, proc, loggateway.NewNoop(), nil)

	done := make(chan struct{})
	go func() {
		w.processOnce(context.Background())
		close(done)
	}()
	deadline := time.Now().Add(2 * time.Second)
	for proc.entered.Load() < 1 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if proc.entered.Load() != 1 {
		t.Fatal("first processOnce did not start")
	}

	// Second run while first is in flight must be skipped.
	w.processOnce(context.Background())
	if got := proc.entered.Load(); got != 1 {
		t.Fatalf("ProcessPending entered %d times, want 1 (single-flight)", got)
	}

	<-done
	// After completion a new run may proceed.
	w.processOnce(context.Background())
	if got := proc.entered.Load(); got != 2 {
		t.Fatalf("ProcessPending entered %d times after release, want 2", got)
	}
}


func TestChannelDeliveryWorker_ProcessOnce_EmitsFlowLogOnError(t *testing.T) {
	flowLog := &canaryFakeFlowLog{}
	worker := &fakePendingDeliveryProcessor{err: errors.New("delivery boom")}
	w := NewChannelDeliveryWorker(0, worker, loggateway.NewNoop(), flowLog)

	w.processOnce(context.Background())

	if worker.call != 1 {
		t.Fatalf("ProcessPending calls = %d, want 1", worker.call)
	}
	if len(flowLog.errors) != 1 {
		t.Fatalf("flow errors = %v, want 1 entry", flowLog.errors)
	}
	want := "system.channel_delivery.failed"
	if got := flowLog.errors[0]; len(got) < len(want) || got[:len(want)] != want {
		t.Fatalf("flow error stepID = %q, want prefix %q", got, want)
	}
}

func TestChannelDeliveryWorker_ProcessOnce_NoFlowLogOnSuccess(t *testing.T) {
	flowLog := &canaryFakeFlowLog{}
	w := NewChannelDeliveryWorker(0, &fakePendingDeliveryProcessor{}, loggateway.NewNoop(), flowLog)

	w.processOnce(context.Background())

	if len(flowLog.errors) != 0 {
		t.Fatalf("flow errors = %v, want none", flowLog.errors)
	}
}

func TestChannelDeliveryWorker_ProcessOnce_NilFlowLogDoesNotPanic(t *testing.T) {
	w := NewChannelDeliveryWorker(0, &fakePendingDeliveryProcessor{err: errors.New("boom")}, loggateway.NewNoop(), nil)
	w.processOnce(context.Background())
}
