package monitor_test

import (
	"context"
	"testing"

	"aranea-agents/internal/biz/monitor"
	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/loggateway"
)

type mockBus struct {
	ch      chan contract.MonitorEvent
	unsub   func()
	dropCnt uint64
}

func newMockBus() *mockBus {
	return &mockBus{
		ch: make(chan contract.MonitorEvent, 64),
	}
}

func (m *mockBus) Publish(_ context.Context, ev contract.MonitorEvent) {
	select {
	case m.ch <- ev:
	default:
		m.dropCnt++
	}
}

func (m *mockBus) Subscribe(_ contract.MonitorSubscribeOptions) (<-chan contract.MonitorEvent, func()) {
	return m.ch, func() {}
}

func (m *mockBus) DropCount() uint64 { return m.dropCnt }

func TestAlertEvalWorker_OnCompletion_Success(t *testing.T) {
	uc := monitor.NewUsecase(nil, nil, nil, nil, nil, nil)
	rb := monitor.NewMetricRingBuffer()
	w := monitor.NewAlertEvalWorker(uc, rb, loggateway.NewNoop())
	w.OnCompletion("success", 150)
}

func TestAlertEvalWorker_OnCompletion_Error(t *testing.T) {
	uc := monitor.NewUsecase(nil, nil, nil, nil, nil, nil)
	rb := monitor.NewMetricRingBuffer()
	w := monitor.NewAlertEvalWorker(uc, rb, loggateway.NewNoop())
	w.OnCompletion("error", 300)
}

func TestAlertEvalWorker_OnCompletion_NilWorker(t *testing.T) {
	var w *monitor.AlertEvalWorker
	w.OnCompletion("success", 100)
}

func TestAlertEvalWorker_OnCompletion_NilBuffer(t *testing.T) {
	uc := monitor.NewUsecase(nil, nil, nil, nil, nil, nil)
	w := monitor.NewAlertEvalWorker(uc, nil, loggateway.NewNoop())
	w.OnCompletion("success", 100)
}

func TestAlertEvalWorker_Ready_BeforeStart(t *testing.T) {
	uc := monitor.NewUsecase(nil, nil, nil, nil, nil, nil)
	rb := monitor.NewMetricRingBuffer()
	w := monitor.NewAlertEvalWorker(uc, rb, loggateway.NewNoop())
	if w.Ready() {
		t.Error("expected Ready() = false before Start")
	}
}

func TestAlertEvalWorker_Ready_NilWorker(t *testing.T) {
	var w *monitor.AlertEvalWorker
	if w.Ready() {
		t.Error("nil worker Ready() should return false")
	}
}

func TestAlertEvalWorker_NilUsecase(t *testing.T) {
	w := monitor.NewAlertEvalWorker(nil, monitor.NewMetricRingBuffer(), loggateway.NewNoop())
	if w != nil {
		t.Error("NewAlertEvalWorker(nil, ...) should return nil")
	}
}
