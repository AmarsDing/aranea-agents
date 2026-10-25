package graph

import (
	"testing"
	"time"

	"trpc.group/trpc-go/trpc-agent-go/agent"
	"trpc.group/trpc-go/trpc-agent-go/event"
)

// E2E-H1 pins：handleInterrupt 的 interrupt 事件必须经有界阻塞重试投递
// （detached ctx），替代旧的 1s fire-and-forget 静默丢弃。

// pin①：消费方持续停滞（channel 永不排空）时，投递在有界预算内放弃并返回
// ——不 panic、不死等；预算=attemptTimeout×attempts 由调用参数控制。
func TestEmitInterruptEventWithRetry_BlockedConsumerGivesUp(t *testing.T) {
	ch := make(chan *event.Event) // unbuffered, never drained
	execCtx := &ExecutionContext{InvocationID: "inv-h1", EventChan: ch}
	inv := agent.NewInvocation()
	evt := NewPregelInterruptEvent(WithPregelEventInvocationID("inv-h1"))

	start := time.Now()
	done := make(chan struct{})
	go func() {
		emitInterruptEventWithRetry(inv, execCtx, evt, 20*time.Millisecond, 2, "n1", 1)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("emit retry must give up within the bounded budget, not hang the executor")
	}
	if elapsed := time.Since(start); elapsed < 40*time.Millisecond {
		t.Fatalf("expected ~2x20ms of blocking retries, returned after %v", elapsed)
	}
}

// pin②：消费方存活时首发即投递成功。
func TestEmitInterruptEventWithRetry_DeliveredFirstAttempt(t *testing.T) {
	ch := make(chan *event.Event, 1)
	execCtx := &ExecutionContext{InvocationID: "inv-h1", EventChan: ch}
	inv := agent.NewInvocation()
	evt := NewPregelInterruptEvent(WithPregelEventInvocationID("inv-h1"))

	emitInterruptEventWithRetry(inv, execCtx, evt, time.Second, 3, "n1", 1)
	select {
	case got := <-ch:
		if got == nil {
			t.Fatal("delivered nil event")
		}
	default:
		t.Fatal("interrupt event must be delivered on the first attempt when the consumer is alive")
	}
}
