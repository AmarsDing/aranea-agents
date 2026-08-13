package biz

import (
	"context"
	"errors"
	"testing"
	"time"

	"aranea-agents/internal/biz/flowlog"
	"aranea-agents/internal/event/contract"
)

type failingFlowLogRepo struct{}

func (failingFlowLogRepo) Insert(context.Context, flowlog.Record) error {
	return errors.New("db down")
}
func (failingFlowLogRepo) List(context.Context, flowlog.Query) (flowlog.ListResult, error) {
	return flowlog.ListResult{}, nil
}
func (failingFlowLogRepo) DeleteOlderThan(context.Context, time.Time) (int64, error) {
	return 0, nil
}

type stubMonitorBus struct{}

func (stubMonitorBus) Publish(context.Context, contract.MonitorEvent) {}
func (stubMonitorBus) Subscribe(contract.MonitorSubscribeOptions) (<-chan contract.MonitorEvent, func()) {
	ch := make(chan contract.MonitorEvent)
	return ch, func() {}
}
func (stubMonitorBus) DropCount() uint64 { return 0 }

type countSessionLogWriter struct{ warns int }

func (w *countSessionLogWriter) LogSessionWarn(context.Context, string, string, string, ...LogPair) {
	w.warns++
}
func (w *countSessionLogWriter) LogSessionError(context.Context, string, string, string, ...LogPair) {
}

type countFlowLogWriter struct{ errs int }

func (w *countFlowLogWriter) LogFlowStart(context.Context, string, string, string, ...LogPair) {}
func (w *countFlowLogWriter) LogFlowDone(context.Context, string, string, string, ...LogPair)  {}
func (w *countFlowLogWriter) LogFlowWarn(context.Context, string, string, string, ...LogPair)  {}
func (w *countFlowLogWriter) LogFlowError(context.Context, string, string, string, ...LogPair) {
	w.errs++
}

// P2: when the flow-log DB insert keeps failing, the consumer must throttle
// its Warn/FlowError emissions — otherwise every incoming flow event produces
// a process-log Warn AND a self-referential flow-log error event (which
// re-enters this consumer and fails again), doubling the storm.
func TestFlowLogPersistConsumer_SaveFailureThrottled(t *testing.T) {
	uc := NewFlowLogUsecase(failingFlowLogRepo{})
	sw := &countSessionLogWriter{}
	fw := &countFlowLogWriter{}
	c := newFlowLogPersistConsumer(uc, sw, stubMonitorBus{}, fw)
	if c == nil {
		t.Fatal("consumer is nil")
	}
	ev := contract.MonitorEvent{
		ID:        "e1",
		Type:      contract.MonitorEventTypeFlowLog,
		Timestamp: time.Now().UTC(),
		SessionID: "s1",
		Metadata:  map[string]any{"step_id": "chat.turn", "session_id": "s1"},
	}
	for i := 0; i < 10; i++ {
		c.handle(context.Background(), ev)
	}
	if sw.warns != 1 {
		t.Errorf("session warns = %d after 10 Save failures, want 1 (throttled)", sw.warns)
	}
	if fw.errs != 1 {
		t.Errorf("flow errors = %d after 10 Save failures, want 1 (throttled)", fw.errs)
	}
}
