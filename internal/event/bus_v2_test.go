package event_test

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
)

// TestV2Bus_TerminalEventBlockUpTo verifies B-06: when a subscriber buffer is
// full, terminal events block briefly instead of being dropped immediately.
func TestV2Bus_TerminalEventBlockUpTo(t *testing.T) {
	t.Parallel()
	bus := event.NewV2Bus()
	// Tiny buffer so we can fill it.
	ch, cancel := bus.Subscribe(biz.EventSubscribeOptions{})
	defer cancel()

	// Fill the default 256 buffer... too large. Instead publish when nobody
	// is draining and use a second subscriber with saturated buffer by
	// not reading from ch while publishing many events.
	//
	// Strategy: create a dedicated bus and manually fill via Publish of
	// non-terminal first (256), then one terminal — terminal should still
	// arrive after we free one slot within the block window.

	// Drain nothing; fill with non-terminal until DropCount increases.
	created := biz.NewTaskCreatedEvent(biz.Task{
		ID: "t-fill", SessionID: "sess-b06", Status: biz.TaskStatusPending,
	})
	for i := 0; i < 300; i++ {
		bus.Publish(context.Background(), created)
	}
	if bus.DropCount() == 0 {
		t.Fatal("expected non-terminal drops when buffer is full")
	}
	dropsBefore := bus.DropCount()

	// Free one slot so the terminal BlockUpTo can succeed.
	select {
	case <-ch:
	default:
		t.Fatal("expected buffered event to drain")
	}

	completed := biz.NewTaskCompletedEvent(biz.Task{
		ID: "t-done", SessionID: "sess-b06", Status: biz.TaskStatusCompleted,
	})
	ctx, cancelPub := context.WithTimeout(context.Background(), time.Second)
	defer cancelPub()
	bus.Publish(ctx, completed)

	gotTerminal := false
	deadline := time.After(500 * time.Millisecond)
drain:
	for {
		select {
		case ev := <-ch:
			if ev != nil && biz.IsTerminalEventKind(ev.EventKind()) {
				gotTerminal = true
				break drain
			}
		case <-deadline:
			break drain
		}
	}
	if !gotTerminal {
		t.Fatalf("expected terminal event to be delivered via BlockUpTo; drops before=%d after=%d",
			dropsBefore, bus.DropCount())
	}
}

func TestIsTerminalEventKind(t *testing.T) {
	t.Parallel()
	if !biz.IsTerminalEventKind(biz.EventKindTaskCompleted) {
		t.Fatal("task.completed should be terminal")
	}
	if biz.IsTerminalEventKind(biz.EventKindTaskCreated) {
		t.Fatal("task.created should not be terminal")
	}
}

func TestIsCriticalDeliveryEvent(t *testing.T) {
	t.Parallel()
	completed := biz.NewTaskCompletedEvent(biz.Task{
		ID: "t1", SessionID: "s1", Status: biz.TaskStatusCompleted,
	})
	if !biz.IsCriticalDeliveryEvent(completed) {
		t.Fatal("task.completed should be critical")
	}
	created := biz.NewTaskCreatedEvent(biz.Task{
		ID: "t2", SessionID: "s1", Status: biz.TaskStatusPending,
	})
	if biz.IsCriticalDeliveryEvent(created) {
		t.Fatal("task.created should not be critical")
	}
	pbTerminal := biz.NewPlanBoardUpdatedEvent(biz.PlanBoard{
		ID: "pb1", SessionID: "s1", Status: biz.PlanStatusCompleted,
	})
	if !biz.IsCriticalDeliveryEvent(pbTerminal) {
		t.Fatal("plan_board.updated with completed status should be critical")
	}
	pbRunning := biz.NewPlanBoardUpdatedEvent(biz.PlanBoard{
		ID: "pb2", SessionID: "s1", Status: biz.PlanStatusExecuting,
	})
	if biz.IsCriticalDeliveryEvent(pbRunning) {
		t.Fatal("plan_board.updated while executing should not be critical")
	}
	orchDone := biz.NewSystemNoticeEvent("s1", "orchestration_completed", "", nil)
	if !biz.IsCriticalDeliveryEvent(orchDone) {
		t.Fatal("orchestration_completed notice should be critical")
	}
	noise := biz.NewSystemNoticeEvent("s1", "knowledge_indexed", "", nil)
	if biz.IsCriticalDeliveryEvent(noise) {
		t.Fatal("generic system.notice should not be critical")
	}
}

// TestV2Bus_PublishDoesNotHoldLockAcrossBlock verifies Publish releases the
// subscriber mutex before BlockUpTo. Otherwise a full-buffer terminal publish
// deadlocks with Subscribe/cancel (needs exclusive Lock).
func TestV2Bus_PublishDoesNotHoldLockAcrossBlock(t *testing.T) {
	t.Parallel()
	bus := event.NewV2Bus()
	ch, cancel := bus.Subscribe(biz.EventSubscribeOptions{})
	defer cancel()

	created := biz.NewTaskCreatedEvent(biz.Task{
		ID: "t-lock", SessionID: "sess-lock", Status: biz.TaskStatusPending,
	})
	for i := 0; i < 300; i++ {
		bus.Publish(context.Background(), created)
	}

	completed := biz.NewTaskCompletedEvent(biz.Task{
		ID: "t-term", SessionID: "sess-lock", Status: biz.TaskStatusCompleted,
	})
	pubDone := make(chan struct{})
	go func() {
		defer close(pubDone)
		// Short deadline so BlockUpTo ends quickly if nobody drains.
		ctx, cancelPub := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancelPub()
		bus.Publish(ctx, completed)
	}()

	// While Publish may be blocked on the full buffer, Subscribe must still
	// make progress (proves RLock is not held across the send).
	subDone := make(chan struct{})
	go func() {
		defer close(subDone)
		_, cancel2 := bus.Subscribe(biz.EventSubscribeOptions{})
		cancel2()
	}()

	select {
	case <-subDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Subscribe blocked while Publish held lock across BlockUpTo")
	}
	select {
	case <-pubDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Publish did not return")
	}
	// Drain so cancel() does not race on a closed full channel in other tests.
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}
