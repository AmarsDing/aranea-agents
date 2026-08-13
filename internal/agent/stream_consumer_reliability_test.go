package agent

// stream_consumer_reliability_test.go — 2026-08-13 编排审查 P0 修复（F5-F6）的回归测试。
//
// 覆盖：
//   F6/Y1a doom-loop 早退后必须后台排干 events channel（否则 trpc runner
//           生产者 goroutine 阻塞泄漏，LLM 流持续烧 token）
//   F6/Y1b doom-loop 早退的 turn 终态必须是 Cancelled，不得误标 Completed
//   F5/Y2  consume 循环内 panic 必须兜底：recover + HasError + 终态事件
//           照常发布（Cancelled），不得让前端永远卡在 running

import (
	"context"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/agent/v2"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
)

// captureSeq 实现 v2.SequencerPublisher：捕获全部已发布事件；可选地在首次
// 发布指定 kind 时 panic（注入故障）。
type captureSeq struct {
	mu       sync.Mutex
	events   []biz.Event
	panicOn  biz.EventKind
	panicked bool
}

func (c *captureSeq) Publish(_ context.Context, e biz.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.panicOn != "" && !c.panicked && e.EventKind() == c.panicOn {
		c.panicked = true
		panic("injected sequencer publish panic")
	}
	c.events = append(c.events, e)
}

func (c *captureSeq) turnCompleted() *biz.TurnCompletedEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, e := range c.events {
		if tc, ok := e.(*biz.TurnCompletedEvent); ok {
			return tc
		}
	}
	return nil
}

func newReliabilityTestConsumer(eventsCap int, seq *captureSeq) (*turnStreamConsumer, chan *trpcevent.Event) {
	events := make(chan *trpcevent.Event, eventsCap)
	var opts *StreamConsumeOptions
	var proj *v2.ActivityProjector
	if seq != nil {
		proj = v2.NewActivityProjector(seq, nil, loggateway.NewNoop())
		opts = &StreamConsumeOptions{V2Projector: proj}
	}
	consumer := newTurnStreamConsumer(
		context.Background(), context.Background(),
		ProjectMeta{
			SessionID:       "sess-rel",
			SpiritSessionID: "sess-rel",
			RequestID:       "task-rel",
			InvocationID:    "turn-rel",
		},
		nil, opts, loggateway.NewNoop(),
	)
	return consumer, events
}

// F6/Y1a: doom-loop 早退后，生产者必须能把剩余事件写完并关闭 channel。
// 修复前：consume 早退抛弃 channel，生产者写满 buffer 后永久阻塞（goroutine 泄漏）。
func TestStreamConsumer_DoomLoopDrainsEventChannel(t *testing.T) {
	t.Parallel()
	consumer, events := newReliabilityTestConsumer(8, nil)

	repetitive := "I need to check the file and verify the output."
	producerDone := make(chan struct{})
	go func() {
		defer close(producerDone)
		for i := 0; i < 60; i++ {
			events <- doomLoopDeltaEvent(repetitive)
		}
		close(events)
	}()

	result := consumer.consume(events)
	if !result.DoomLoopDetected {
		t.Fatal("expected DoomLoopDetected=true")
	}

	select {
	case <-producerDone:
		// 生产者完成 → channel 被排干，无泄漏。
	case <-time.After(2 * time.Second):
		t.Fatal("producer goroutine blocked: events channel not drained after doom-loop abort")
	}
}

// F6/Y1b: doom-loop 早退的 turn 必须按 Cancelled 收尾（result 已标错，
// 实体终态不得是 Completed）。
func TestStreamConsumer_DoomLoopMarksTurnCancelled(t *testing.T) {
	t.Parallel()
	seq := &captureSeq{}
	consumer, events := newReliabilityTestConsumer(16, seq)

	repetitive := "I need to check the file and verify the output."
	for i := 0; i < 6; i++ {
		events <- doomLoopDeltaEvent(repetitive)
	}
	close(events)

	result := consumer.consume(events)
	if !result.DoomLoopDetected {
		t.Fatal("expected DoomLoopDetected=true")
	}
	if !consumer.canceled {
		t.Fatal("expected consumer.canceled=true after doom-loop abort (terminal status must not be Completed)")
	}
	tc := seq.turnCompleted()
	if tc == nil {
		t.Fatal("turn.completed event not published on doom-loop abort")
	}
	if tc.Turn.Status != biz.TurnStatusCancelled {
		t.Fatalf("expected turn terminal status %q, got %q", biz.TurnStatusCancelled, tc.Turn.Status)
	}
}

// F5/Y2: consume 循环内 panic（此处注入在 step.created 发布点）必须被兜底：
// 不向上抛、HasError=true、终态事件照常发布且为 Cancelled。
func TestStreamConsumer_PanicRecoveredTurnCancelled(t *testing.T) {
	t.Parallel()
	seq := &captureSeq{panicOn: biz.EventKindStepCreated}
	consumer, events := newReliabilityTestConsumer(16, seq)

	events <- doomLoopDeltaEvent("a perfectly normal first text delta")
	close(events)

	// 修复前：panic 直接穿透 consume，测试进程崩溃（go test 报 panic）。
	result := consumer.consume(events)
	if !result.HasError {
		t.Fatal("expected HasError=true after panic recovery")
	}
	tc := seq.turnCompleted()
	if tc == nil {
		t.Fatal("turn.completed event not published after panic recovery")
	}
	if tc.Turn.Status != biz.TurnStatusCancelled {
		t.Fatalf("expected turn terminal status %q after panic, got %q", biz.TurnStatusCancelled, tc.Turn.Status)
	}
}
