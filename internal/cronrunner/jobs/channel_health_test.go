package jobs

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/pkg/loggateway"
)

type fakeHealthCheckRunner struct {
	err  error
	call int
}

func (f *fakeHealthCheckRunner) RunHealthChecks(_ context.Context) error {
	f.call++
	return f.err
}

func TestChannelHealthScanner_ProcessOnce_EmitsFlowLogOnError(t *testing.T) {
	flowLog := &canaryFakeFlowLog{}
	uc := &fakeHealthCheckRunner{err: errors.New("channel health boom")}
	w := NewChannelHealthScanner(0, uc, loggateway.NewNoop(), flowLog)

	w.processOnce(context.Background())

	if uc.call != 1 {
		t.Fatalf("RunHealthChecks calls = %d, want 1", uc.call)
	}
	if len(flowLog.errors) != 1 {
		t.Fatalf("flow errors = %v, want 1 entry", flowLog.errors)
	}
	want := "system.channel_health.failed"
	if got := flowLog.errors[0]; len(got) < len(want) || got[:len(want)] != want {
		t.Fatalf("flow error stepID = %q, want prefix %q", got, want)
	}
}

func TestChannelHealthScanner_ProcessOnce_NoFlowLogOnSuccess(t *testing.T) {
	flowLog := &canaryFakeFlowLog{}
	w := NewChannelHealthScanner(0, &fakeHealthCheckRunner{}, loggateway.NewNoop(), flowLog)

	w.processOnce(context.Background())

	if len(flowLog.errors) != 0 {
		t.Fatalf("flow errors = %v, want none", flowLog.errors)
	}
}
