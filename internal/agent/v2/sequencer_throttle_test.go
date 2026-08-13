package v2

// sequencer_throttle_test.go — P3：persistWithRetry 重试耗尽后的死信 Error
// 日志必须限流。DB 持续故障时每个持久化事件都会耗尽重试并打一条 Error，
// 形成日志洪峰；死信入环/入表行为本身不得被限流。

import (
	"context"
	"fmt"
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
		s.persist.persistWithRetry(biz.NewTaskCreatedEvent(biz.Task{ID: "t-fail", SessionID: "s-1", Version: 1}))
	}
	if got := lg.errorCount(); got != 1 {
		t.Errorf("error logs = %d after 10 exhausted persists, want 1 (throttled)", got)
	}
	// 死信行为不得被限流：事件必须仍然入环（同实体去重后为 1）。
	if got := s.DeadLetterCount(); got != 1 {
		t.Errorf("dead letters = %d, want 1 (dead-lettering must not be throttled)", got)
	}
}

// ---------------------------------------------------------------------------
// R1-R4：管道停滞风暴链残留限流。DB 挂/管道停滞时，同一条失败链上的
// 入队超时 Error、persistChan 满 Warn、死信落库 Warn、outbox 插入 Warn
// 都必须各自限流，否则 P3 的限流被旁路。
// ---------------------------------------------------------------------------

type warnCountingLogger struct {
	mu    sync.Mutex
	warns int
}

func (l *warnCountingLogger) Debug(string, ...loggateway.Field) {}
func (l *warnCountingLogger) Info(string, ...loggateway.Field)  {}
func (l *warnCountingLogger) Warn(string, ...loggateway.Field) {
	l.mu.Lock()
	l.warns++
	l.mu.Unlock()
}
func (l *warnCountingLogger) Error(string, ...loggateway.Field)          {}
func (l *warnCountingLogger) With(...loggateway.Field) loggateway.Logger { return l }

func (l *warnCountingLogger) warnCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.warns
}

func taskCreatedEv(i int) biz.Event {
	return biz.NewTaskCreatedEvent(biz.Task{ID: fmt.Sprintf("t-%d", i), SessionID: "s-1", Version: 1})
}

// blockingBus stalls the publishLoop inside the sync bus publish so the
// publish queue backs up deterministically.
type blockingBus struct {
	fakeBus
	entered chan struct{}
	gate    chan struct{}
	once    sync.Once
}

func (b *blockingBus) Publish(ctx context.Context, e biz.Event) {
	b.once.Do(func() { close(b.entered) })
	<-b.gate
	b.fakeBus.Publish(ctx, e)
}

// R1: when the pipeline stalls (publishLoop blocked), every persist-class
// Publish eventually times out enqueueing and logs an Error. The Error must
// be throttled; the dead-letter push must not.
func TestSequencer_EnqueueTimeoutErrorLogThrottled(t *testing.T) {
	rs := &fakeRepoSet{}
	bus := &blockingBus{entered: make(chan struct{}), gate: make(chan struct{})}
	lg := &errorCountingLogger{}
	s := NewSequencer(rs, bus, lg,
		WithPublishBuffer(1),
		WithPersistBuffer(1),
		WithPersistEnqueueTimeout(20*time.Millisecond),
		WithDeadLetterReplayLoopDisabled(),
	)
	t.Cleanup(func() { _ = s.Close() })
	// LIFO: gate is released BEFORE Close waits for the publishLoop.
	t.Cleanup(func() { close(bus.gate) })

	ctx := context.Background()
	s.Publish(ctx, taskCreatedEv(0))
	<-bus.entered                    // publishLoop now blocked inside bus.Publish
	s.Publish(ctx, taskCreatedEv(1)) // occupies the single queue slot
	for i := 2; i < 7; i++ {
		s.Publish(ctx, taskCreatedEv(i)) // each waits out the enqueue timeout
	}
	if got := lg.errorCount(); got != 1 {
		t.Errorf("enqueue-timeout error logs = %d, want 1 (throttled)", got)
	}
}

// blockingRepoSet stalls the persistLoop inside the first Upsert until the
// per-attempt context (2s in persistWithRetry) expires.
type blockingRepoSet struct {
	fakeRepoSet
	entered chan struct{}
	once    sync.Once
}

func (b *blockingRepoSet) UpsertTask(ctx context.Context, _ biz.Task) (biz.Task, error) {
	b.once.Do(func() { close(b.entered) })
	<-ctx.Done()
	return biz.Task{}, ctx.Err()
}

// R2: when the persist worker is stuck (DB slow), persistChan fills and every
// subsequent persist event logs "persist channel full" — unbounded without a
// throttle.
func TestSequencer_PersistChannelFullWarnThrottled(t *testing.T) {
	rs := &blockingRepoSet{entered: make(chan struct{})}
	bus := &fakeBus{}
	lg := &warnCountingLogger{}
	s := NewSequencer(rs, bus, lg,
		WithPersistBuffer(1),
		WithPersistMaxRetries(1),
		WithDeadLetterReplayLoopDisabled(),
	)
	t.Cleanup(func() { _ = s.Close() })

	s.processTask(publishTask{event: taskCreatedEv(0), persist: true})
	<-rs.entered                                           // persistLoop now blocked inside UpsertTask (up to 2s)
	s.persist.ch <- persistItem{event: taskCreatedEv(99)} // occupy the single slot
	for i := 1; i <= 10; i++ {
		s.processTask(publishTask{event: taskCreatedEv(i), persist: true})
	}
	if got := lg.warnCount(); got != 1 {
		t.Errorf("persist-channel-full warns = %d, want 1 (throttled)", got)
	}
}

// failingDeadLetterStore fails every durable save (DB down).
type failingDeadLetterStore struct {
	fakeDeadLetterStore
}

func (f *failingDeadLetterStore) SaveEventDeadLetter(context.Context, biz.EventDeadLetter) error {
	return errTestFailure
}

// R3: when the dead-letter store is down, every dead-lettered event also logs
// "durable dead-letter save failed" — stacking on top of the drop-site log.
func TestSequencer_DeadLetterSaveWarnThrottled(t *testing.T) {
	rs := &fakeRepoSet{}
	bus := &fakeBus{}
	lg := &warnCountingLogger{}
	s := NewSequencer(rs, bus, lg,
		WithDeadLetterStore(&failingDeadLetterStore{}),
		WithDeadLetterReplayLoopDisabled(),
	)
	t.Cleanup(func() { _ = s.Close() })

	for i := 0; i < 10; i++ {
		s.persist.pushDeadLetter(taskCreatedEv(i))
	}
	if got := lg.warnCount(); got != 1 {
		t.Errorf("dead-letter save warns = %d, want 1 (throttled)", got)
	}
	// 落库失败不得影响内存死信行为。
	if got := s.DeadLetterCount(); got != 10 {
		t.Errorf("dead letters = %d, want 10 (dead-lettering must not be throttled)", got)
	}
}

// failingOutbox fails every outbox Insert (DB down).
type failingOutbox struct {
	fakeOutbox
}

func (f *failingOutbox) Insert(context.Context, biz.EventDeliveryOutboxRow) error {
	return errTestFailure
}

// R4: when the outbox table is down, every critical event logs
// "outbox insert failed" at critical-event frequency.
func TestSequencer_OutboxInsertWarnThrottled(t *testing.T) {
	rs := &fakeRepoSet{}
	bus := &fakeBus{}
	lg := &warnCountingLogger{}
	s := NewSequencer(rs, bus, lg,
		WithEventOutbox(&failingOutbox{}),
		WithDeadLetterReplayLoopDisabled(),
	)
	t.Cleanup(func() { _ = s.Close() })

	task := biz.Task{ID: "task-1", SessionID: "sess-1", Status: biz.TaskStatusCompleted, Seq: 9, Version: 2}
	ctx := context.Background()
	for i := 0; i < 10; i++ {
		s.publishCritical(ctx, biz.NewTaskCompletedEvent(task))
	}
	if got := lg.warnCount(); got != 1 {
		t.Errorf("outbox insert warns = %d, want 1 (throttled)", got)
	}
}
