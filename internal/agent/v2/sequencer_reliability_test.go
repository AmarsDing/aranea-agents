package v2

// sequencer_reliability_test.go — 2026-08-13 编排审查 P0 修复（F1-F4）的回归测试。
//
// 覆盖：
//   F1/B1  流式批合并 timer 闭包竞态（迟到回调 close 共享变量 → panic → 管道死亡）
//   F2/Y5  持久化事件入队超时必须落死信，不得静默丢弃
//   F3/Y6  Publish/Flush 与 Close 并发不得 send-on-closed-channel panic
//   F4a/Y7 persistWithRetry 末次 attempt 后不得多睡一拍
//   F4b/A5 deadLetterRing 同实体去重必须保留最新事件（对齐 durable store upsert 语义）

import (
	"context"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// F1: 迟到 timer 回调不得 panic / 杀死 publishLoop。
//
// 竞态时序（修复前）：A(流式)入队 → pending(A,chA,timerA)；B(不同 step，流式)
// 入队 → flush A、pendingDone=nil → pending(B,chB,timerB)。timerA 迟到触发，
// 闭包读取共享变量 pendingDone——此时已是 chB：close(chB) 令 B 被提前 flush，
// pendingDone 归 nil；timerB 随后触发 close(nil) → panic。safego recover 后
// publishLoop 退出，事件管道静默死亡。
//
// 检测方式：hammer 若干 A/B 对后做活性检查——管道死亡时 Flush 必超时。
func TestSequencer_StreamingTimerLateFireKeepsPipelineAlive(t *testing.T) {
	t.Parallel()
	rs := &fakeRepoSet{}
	bus := &fakeBus{}
	s := NewSequencer(rs, bus, loggateway.NewNoop(),
		WithPublishBuffer(16),
		WithPersistBuffer(16),
		WithDeltaBatchInterval(time.Millisecond), // 1ms 窗口放大竞态
	)
	t.Cleanup(func() { _ = s.Close() })

	ctx := context.Background()
	for i := 0; i < 20; i++ {
		// 每条 Publish 带短超时 ctx：管道若已死亡，快速失败而非挂死测试。
		pctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
		s.Publish(pctx, biz.NewStepStreamingEvent("sess-1", "task-1", "step-a", "content", "x"))
		s.Publish(pctx, biz.NewStepStreamingEvent("sess-1", "task-1", "step-b", "content", "y"))
		cancel()
		time.Sleep(4 * time.Millisecond) // 让两个 timer 都触发
	}

	// 活性检查：publishLoop 存活则正常处理后续事件。
	s.Publish(ctx, biz.NewTaskCreatedEvent(biz.Task{ID: "t-live", SessionID: "sess-1", Version: 1}))
	fctx, fcancel := context.WithTimeout(ctx, 5*time.Second)
	defer fcancel()
	if err := s.Flush(fctx); err != nil {
		t.Fatalf("event pipeline dead after streaming timer race: %v", err)
	}
}

// F2: 持久化事件在 publishQueue 持续打满超过 persistEnqueueTimeout 时，
// 必须落入死信（可重放），而非仅日志丢弃。
func TestSequencer_PersistEnqueueTimeoutGoesToDeadLetter(t *testing.T) {
	t.Parallel()
	rs := &fakeRepoSet{}
	gate := make(chan struct{})
	bus := &gatingBus{gate: gate}
	s := NewSequencer(rs, bus, loggateway.NewNoop(),
		WithPublishBuffer(1),
		WithPersistBuffer(16),
		WithDeltaBatchInterval(time.Millisecond*4),
		WithPersistEnqueueTimeout(50*time.Millisecond),
		WithDeadLetterReplayLoopDisabled(),
	)
	t.Cleanup(func() {
		close(gate) // 放行 publishLoop，让 Close 能排干
		_ = s.Close()
	})

	ctx := context.Background()
	// ev1 出队后 publishLoop 阻塞在 bus.Publish（gate 关闭）。
	s.Publish(ctx, biz.NewTaskCreatedEvent(biz.Task{ID: "t-1", SessionID: "s-1", Version: 1}))
	// 等 ev1 被取走（队列腾空再填入 ev2）。
	deadline := time.After(2 * time.Second)
	for {
		if len(s.publishQueue) == 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("ev1 never dequeued")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	// ev2 占满 buffer=1 的队列。
	s.Publish(ctx, biz.NewTaskCreatedEvent(biz.Task{ID: "t-2", SessionID: "s-1", Version: 1}))
	// ev3 入队超时（50ms）→ 必须落死信。
	s.Publish(ctx, biz.NewTaskCreatedEvent(biz.Task{ID: "t-3", SessionID: "s-1", Version: 1}))

	if got := s.DeadLetterCount(); got != 1 {
		t.Fatalf("expected 1 dead-lettered event after enqueue timeout, got %d", got)
	}
}

// gatingBus 在 gate 打开前阻塞所有 Publish（尊重 ctx）。
type gatingBus struct {
	gate chan struct{}
	mu   sync.Mutex
	pub  []biz.Event
}

func (b *gatingBus) Publish(ctx context.Context, e biz.Event) {
	select {
	case <-b.gate:
	case <-ctx.Done():
	}
	b.mu.Lock()
	b.pub = append(b.pub, e)
	b.mu.Unlock()
}

func (b *gatingBus) Subscribe(_ biz.EventSubscribeOptions) (<-chan biz.Event, func()) {
	ch := make(chan biz.Event)
	return ch, func() {}
}

// F3: 并发 Publish/Flush 与 Close 不得 panic（send on closed channel）。
func TestSequencer_PublishConcurrentWithCloseNoPanic(t *testing.T) {
	t.Parallel()
	for round := 0; round < 30; round++ {
		rs := &fakeRepoSet{}
		bus := &fakeBus{}
		s := NewSequencer(rs, bus, loggateway.NewNoop(),
			WithPublishBuffer(1), // 小 buffer 制造发送阻塞，放大竞态窗口
			WithPersistBuffer(4),
			WithDeltaBatchInterval(time.Millisecond*4),
			WithDeadLetterReplayLoopDisabled(),
		)
		var wg sync.WaitGroup
		for g := 0; g < 8; g++ {
			wg.Add(1)
			go func(g int) {
				defer wg.Done()
				ctx := context.Background()
				for i := 0; i < 50; i++ {
					s.Publish(ctx, biz.NewTaskCreatedEvent(biz.Task{
						ID: "t", SessionID: "s", Version: 1,
					}))
				}
			}(g)
		}
		time.Sleep(time.Millisecond) // 让发布者进入飞行状态
		_ = s.Close()
		wg.Wait()
	}
}

// F4a: persistWithRetry 在最后一次 attempt 后不得再做无效退避睡眠。
// 5 次 attempt + 4 次退避（20+40+80+160=300ms）；修复前多睡 320ms（≈620ms）。
func TestSequencer_PersistRetrySkipsTrailingSleep(t *testing.T) {
	t.Parallel()
	rs := &failingRepoSet{fail: true}
	bus := &fakeBus{}
	s := NewSequencer(rs, bus, loggateway.NewNoop(),
		WithPublishBuffer(4),
		WithPersistBuffer(4),
		WithPersistMaxRetries(5),
		WithPersistBackoff(20*time.Millisecond),
		WithDeadLetterReplayLoopDisabled(),
	)
	t.Cleanup(func() { _ = s.Close() })

	start := time.Now()
	s.persist.persistWithRetry(biz.NewTaskCreatedEvent(biz.Task{ID: "t-fail", SessionID: "s-1", Version: 1}))
	elapsed := time.Since(start)

	if elapsed < 280*time.Millisecond {
		t.Fatalf("backoff sleeps missing: elapsed=%v, want >= ~300ms", elapsed)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("trailing sleep after last attempt not removed: elapsed=%v, want ~300ms (before fix ~620ms)", elapsed)
	}
}

// F4b: 同实体事件重复入死信环时保留最新（对齐 durable store 的 upsert 语义）——
// 旧事件携带的是过期实体状态，重放/巡检应以最新为准。
func TestDeadLetterRing_PushDuplicateKeepsNewest(t *testing.T) {
	t.Parallel()
	r := newDeadLetterRing(8)
	first := biz.NewTaskCreatedEvent(biz.Task{ID: "t-1", SessionID: "s-1", Version: 1})
	second := biz.NewTaskCreatedEvent(biz.Task{ID: "t-1", SessionID: "s-1", Version: 2})

	r.Push(first)
	r.Push(second)

	if r.Len() != 1 {
		t.Fatalf("expected dedup to keep 1 event, got %d", r.Len())
	}
	if r.buf[0] != second {
		t.Fatalf("expected newest event retained, got %+v", r.buf[0])
	}
}
