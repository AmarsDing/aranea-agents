package jobs

import (
	"context"
	"errors"
	"testing"

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
