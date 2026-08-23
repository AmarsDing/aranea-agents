package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"
)

type autoTitleCall struct {
	sessionID string
	content   string
}

type recordingAutoTitler struct {
	mu     sync.Mutex
	calls  []autoTitleCall
	err    error
	signal chan struct{}
}

func newRecordingAutoTitler() *recordingAutoTitler {
	return &recordingAutoTitler{signal: make(chan struct{}, 16)}
}

func (r *recordingAutoTitler) AutoTitleFromUserMessage(_ context.Context, sessionID, content string) error {
	r.mu.Lock()
	r.calls = append(r.calls, autoTitleCall{sessionID: sessionID, content: content})
	r.mu.Unlock()
	select {
	case r.signal <- struct{}{}:
	default:
	}
	return r.err
}

func (r *recordingAutoTitler) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.calls)
}

func (r *recordingAutoTitler) last() autoTitleCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls[len(r.calls)-1]
}

// task.created carrying SessionID+UserMessage must reach the title runner
// (the v2 native chat path trigger restored by BUG-01).
func TestSessionAutoTitleSubscriber_TaskCreatedTriggers(t *testing.T) {
	bus := event.NewV2Bus()
	titler := newRecordingAutoTitler()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	startSessionAutoTitleSubscriber(ctx, bus, titler, loggateway.NewNoop())

	bus.Publish(ctx, biz.NewTaskCreatedEvent(biz.Task{
		ID:          "task-1",
		SessionID:   "sess-1",
		UserMessage: "请用三句话介绍巴黎。",
	}))

	select {
	case <-titler.signal:
	case <-time.After(2 * time.Second):
		t.Fatal("expected AutoTitleFromUserMessage to be called")
	}
	if got := titler.last(); got.sessionID != "sess-1" || got.content != "请用三句话介绍巴黎。" {
		t.Fatalf("unexpected call args: %+v", got)
	}
}

// Non-task events and task.created without user message must be ignored.
func TestSessionAutoTitleSubscriber_FiltersIrrelevantEvents(t *testing.T) {
	bus := event.NewV2Bus()
	titler := newRecordingAutoTitler()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	startSessionAutoTitleSubscriber(ctx, bus, titler, loggateway.NewNoop())

	// Non-task event type.
	bus.Publish(ctx, biz.NewStepCreatedEvent(biz.Step{ID: "step-1", TaskID: "task-1"}))
	// task.created with empty UserMessage.
	bus.Publish(ctx, biz.NewTaskCreatedEvent(biz.Task{ID: "task-2", SessionID: "sess-2"}))
	// task.created with empty SessionID.
	bus.Publish(ctx, biz.NewTaskCreatedEvent(biz.Task{ID: "task-3", UserMessage: "hi"}))

	// Give the drain loop a chance to (incorrectly) fire.
	time.Sleep(300 * time.Millisecond)
	if n := titler.count(); n != 0 {
		t.Fatalf("expected 0 calls, got %d", n)
	}
}

// A failing title runner must not kill the drain loop — the next event is
// still processed (title is best-effort and must never affect chat).
func TestSessionAutoTitleSubscriber_ErrorDoesNotStopLoop(t *testing.T) {
	bus := event.NewV2Bus()
	titler := newRecordingAutoTitler()
	titler.err = errors.New("db down")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	startSessionAutoTitleSubscriber(ctx, bus, titler, loggateway.NewNoop())

	bus.Publish(ctx, biz.NewTaskCreatedEvent(biz.Task{ID: "task-1", SessionID: "sess-1", UserMessage: "first"}))
	select {
	case <-titler.signal:
	case <-time.After(2 * time.Second):
		t.Fatal("first event not processed")
	}

	titler.err = nil
	bus.Publish(ctx, biz.NewTaskCreatedEvent(biz.Task{ID: "task-2", SessionID: "sess-2", UserMessage: "second"}))
	select {
	case <-titler.signal:
	case <-time.After(2 * time.Second):
		t.Fatal("second event not processed after error")
	}
	if n := titler.count(); n != 2 {
		t.Fatalf("expected 2 calls, got %d", n)
	}
}

// Nil bus / nil titler must be a safe no-op (wire optional paths).
func TestSessionAutoTitleSubscriber_NilDepsSafe(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	startSessionAutoTitleSubscriber(ctx, nil, newRecordingAutoTitler(), loggateway.NewNoop())
	startSessionAutoTitleSubscriber(ctx, event.NewV2Bus(), nil, loggateway.NewNoop())
}
