package v2

// sequencer_throttle_test.go — P3：persistWithRetry 重试耗尽后的死信 Error
// 日志必须限流。DB 持续故障时每个持久化事件都会耗尽重试并打一条 Error，
// 形成日志洪峰；死信入环/入表行为本身不得被限流。

import (
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

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

func TestSequencer_DeadLetterErrorLogThrottled(t *testing.T) {
	rs := &failingRepoSet{fail: true}
	bus := &fakeBus{}
	lg := &errorCountingLogger{}
	s := NewSequencer(rs, bus, lg,
		WithPublishBuffer(4),
		WithPersistBuffer(4),
		WithPersistMaxRetries(1),
		WithPersistBackoff(time.Millisecond),
		WithDeadLetterReplayLoopDisabled(),
	)
	t.Cleanup(func() { _ = s.Close() })

	for i := 0; i < 10; i++ {
		s.persistWithRetry(biz.NewTaskCreatedEvent(biz.Task{ID: "t-fail", SessionID: "s-1", Version: 1}))
	}
	if got := lg.errorCount(); got != 1 {
		t.Errorf("error logs = %d after 10 exhausted persists, want 1 (throttled)", got)
	}
	// 死信行为不得被限流：事件必须仍然入环（同实体去重后为 1）。
	if got := s.DeadLetterCount(); got != 1 {
		t.Errorf("dead letters = %d, want 1 (dead-lettering must not be throttled)", got)
	}
}
