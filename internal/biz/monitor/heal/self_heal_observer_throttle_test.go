package heal_test

import (
	"context"
	"errors"
	"sync"
	"testing"


	"aranea-agents/internal/biz/monitor/heal"
	"aranea-agents/pkg/loggateway"
)

// errorCountingLogger counts Error calls so tests can assert on log volume.
type errorCountingLogger struct {
	mu     sync.Mutex
	errors int
}

func (l *errorCountingLogger) Debug(string, ...loggateway.Field) {}
func (l *errorCountingLogger) Info(string, ...loggateway.Field)  {}
func (l *errorCountingLogger) Warn(string, ...loggateway.Field)  {}
func (l *errorCountingLogger) Error(string, ...loggateway.Field) {
	l.mu.Lock()
	l.errors++
	l.mu.Unlock()
}
func (l *errorCountingLogger) With(...loggateway.Field) loggateway.Logger { return l }

func (l *errorCountingLogger) errorCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.errors
}

// P4: when HealRecordRepo.InsertHealRecord keeps failing (DB down), the
// observer must throttle its Error log — otherwise every error flow event
// produces one heal-record insert failure Error, flooding the pipeline.
// The insert itself must still be attempted for every record.
func TestSelfHealObserver_PersistFailureLogThrottled(t *testing.T) {
	var insertCalls int
	repo := &mockHealRecordRepo{
		insertFn: func(context.Context, heal.HealRecord) error {
			insertCalls++
			return errors.New("db down")
		},
	}
	lg := &errorCountingLogger{}
	engine := heal.NewRootCauseEngine(loggateway.NewNoop())
	o, err := heal.NewSelfHealObserver(nil, repo, engine, nil, lg)
	if err != nil {
		t.Fatalf("NewSelfHealObserver: %v", err)
	}

	meta := errorMeta("chat.turn", map[string]any{"auto_healed": true, "heal_success": true})
	for i := 0; i < 10; i++ {
		o.ObserveFlowLogEvent(context.Background(), meta)
	}
	if insertCalls != 10 {
		t.Errorf("insert attempts = %d, want 10 (persistence must not be throttled)", insertCalls)
	}
	if got := lg.errorCount(); got != 1 {
		t.Errorf("error logs = %d after 10 insert failures, want 1 (throttled)", got)
	}
}
