package heal_test

import (
	"context"
	"os"
	"testing"
	"time"

	"aranea-agents/internal/biz/monitor"
	"aranea-agents/pkg/loggateway"
)

func TestMain(m *testing.M) {
	loggateway.SetGlobal(loggateway.NewNoop())
	os.Exit(m.Run())
}

// mockEventTraceRepo implements monitor.EventRepo + monitor.TraceRepo for
// DiagBundleGenerator / SelfHealUsecase tests. Only the fn fields used by a
// test need to be set; everything else returns nil-safe zero values.
type mockEventTraceRepo struct {
	listMonitorEventsFn       func(ctx context.Context, query monitor.EventsQuery) (monitor.ListResult, error)
	getMonitorEventFn         func(ctx context.Context, id string) (monitor.PlatformRow, error)
	insertMonitorEventFn      func(ctx context.Context, ev monitor.EventWrite) error
	countMonitorEventsSinceFn func(ctx context.Context, eventKey, status, sinceRFC3339, untilRFC3339 string) (int32, error)
	listMonitorTracesFn       func(ctx context.Context, query monitor.TracesQuery) (monitor.ListResult, error)
	getMonitorTraceFn         func(ctx context.Context, id string) (monitor.PlatformRow, error)
}

func (m *mockEventTraceRepo) InsertMonitorEvent(ctx context.Context, ev monitor.EventWrite) error {
	if m.insertMonitorEventFn != nil {
		return m.insertMonitorEventFn(ctx, ev)
	}
	return nil
}

func (m *mockEventTraceRepo) ListMonitorEvents(ctx context.Context, query monitor.EventsQuery) (monitor.ListResult, error) {
	if m.listMonitorEventsFn != nil {
		return m.listMonitorEventsFn(ctx, query)
	}
	return monitor.ListResult{}, nil
}

func (m *mockEventTraceRepo) GetMonitorEvent(ctx context.Context, id string) (monitor.PlatformRow, error) {
	if m.getMonitorEventFn != nil {
		return m.getMonitorEventFn(ctx, id)
	}
	return monitor.PlatformRow{}, nil
}

func (m *mockEventTraceRepo) CountMonitorEventsSince(ctx context.Context, eventKey, status, sinceRFC3339, untilRFC3339 string) (int32, error) {
	if m.countMonitorEventsSinceFn != nil {
		return m.countMonitorEventsSinceFn(ctx, eventKey, status, sinceRFC3339, untilRFC3339)
	}
	return 0, nil
}

func (m *mockEventTraceRepo) DeleteMonitorEventsOlderThan(_ context.Context, _ time.Time) (int, error) {
	return 0, nil
}

func (m *mockEventTraceRepo) ListMonitorTraces(ctx context.Context, query monitor.TracesQuery) (monitor.ListResult, error) {
	if m.listMonitorTracesFn != nil {
		return m.listMonitorTracesFn(ctx, query)
	}
	return monitor.ListResult{}, nil
}

func (m *mockEventTraceRepo) GetMonitorTrace(ctx context.Context, id string) (monitor.PlatformRow, error) {
	if m.getMonitorTraceFn != nil {
		return m.getMonitorTraceFn(ctx, id)
	}
	return monitor.PlatformRow{}, nil
}

func (m *mockEventTraceRepo) InsertMonitorTrace(_ context.Context, _ monitor.TraceWrite) error {
	return nil
}

func (m *mockEventTraceRepo) UpsertMonitorTraceSpan(_ context.Context, _ monitor.TraceSpanWrite) error {
	return nil
}

func (m *mockEventTraceRepo) UpdateMonitorTraceCompletion(_ context.Context, _ string, _ monitor.TraceCompletion) error {
	return nil
}

func (m *mockEventTraceRepo) InterruptStaleTraces(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}

func (m *mockEventTraceRepo) EnsureTraceSchema(_ context.Context) error {
	return nil
}
