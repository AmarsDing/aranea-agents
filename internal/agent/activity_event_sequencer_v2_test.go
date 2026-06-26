package agent

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"aranea-agents/internal/biz"
)

// TestSequencerV2_SinglePublishWorker_FIFO verifies that publish events are
// emitted in strict FIFO order matching enqueue order (which matches seq).
//
// Under the v2 single-publish-worker architecture, all events are processed
// by one goroutine, so a single activity's events MUST come out in the
// exact order they were enqueued.
func TestSequencerV2_SinglePublishWorker_FIFO(t *testing.T) {
	t.Parallel()

	eventBus := &recordingEventBus{}
	repo := &fakeActivityRepo{}
	seq := newActivityEventSequencer(eventBus, nil)
	seq.SetActivityRepo(repo)

	// Enqueue 100 tasks with incrementing seq
	const N = 100
	for i := 0; i < N; i++ {
		a := biz.Activity{
			ID:        bizID(i),
			Kind:      biz.ActivityKindReply,
			Status:    biz.ActivityStatusRunning,
			SessionID: "sess-1",
			Seq:       int64(i + 1),
		}
		ev := biz.ActivityEvent{
			Event:    biz.ActivityEventStreaming,
			Activity: a,
		}
		if err := seq.publish(context.Background(), a.ID, publishTask{
			event:    ev,
			persist:  false,
			activity: a,
		}); err != nil {
			t.Fatalf("publish %d failed: %v", i, err)
		}
	}

	seq.Close()

	// Verify: eventBus received events in seq order
	received := eventBus.receivedSeq()
	if len(received) != N {
		t.Fatalf("expected %d events, got %d", N, len(received))
	}
	for i, seqVal := range received {
		if seqVal != int64(i+1) {
			t.Errorf("event %d: expected seq=%d, got seq=%d", i, i+1, seqVal)
		}
	}
}

// TestSequencerV2_CrossActivityOrder_Concurrent simulates concurrent thinking
// + reply creation and verifies reply always comes after thinking in publish
// order. The OLD per-activity channel architecture had race conditions here
// because different activities publish concurrently and can interleave on the
// bus.
//
// Under v2 single-publish-worker architecture, ALL events flow through one
// FIFO queue, so the seq order assigned at On* entry is preserved.
func TestSequencerV2_CrossActivityOrder_Concurrent(t *testing.T) {
	t.Parallel()

	eventBus := &recordingEventBus{}
	repo := &fakeActivityRepo{}
	seq := newActivityEventSequencer(eventBus, nil)
	seq.SetActivityRepo(repo)

	const N = 1000
	var counter atomic.Int64
	var wg sync.WaitGroup
	wg.Add(N * 2)

	// Enqueue N "thinking created" tasks
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			seqNum := counter.Add(1)
			a := biz.Activity{
				ID:    "think-" + bizIDint(seqNum),
				Kind:  biz.ActivityKindThinking,
				Seq:   seqNum,
			}
			_ = seq.publish(context.Background(), a.ID, publishTask{
				event:    biz.ActivityEvent{Event: biz.ActivityEventCreated, Activity: a},
				activity: a,
			})
		}()
	}

	// Enqueue N "reply created" tasks (with higher seq — assigned after thinking)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			seqNum := counter.Add(1)
			a := biz.Activity{
				ID:    "reply-" + bizIDint(seqNum),
				Kind:  biz.ActivityKindReply,
				Seq:   seqNum,
			}
			_ = seq.publish(context.Background(), a.ID, publishTask{
				event:    biz.ActivityEvent{Event: biz.ActivityEventCreated, Activity: a},
				activity: a,
			})
		}()
	}

	wg.Wait()
	// Wait for publish worker to drain
	time.Sleep(100 * time.Millisecond)
	seq.Close()

	// Verify: all 2N events received
	received := eventBus.received()
	if len(received) != N*2 {
		t.Fatalf("expected %d events, got %d", N*2, len(received))
	}

	// Verify: seq values are in monotonic order (this is what the OLD design failed)
	lastSeq := int64(0)
	for i, a := range received {
		if a.Seq <= lastSeq && i > 0 {
			t.Errorf("event %d: seq=%d not strictly greater than previous seq=%d (cross-activity order broken)", i, a.Seq, lastSeq)
			break
		}
		lastSeq = a.Seq
	}
}

// recordingEventBus captures all published events in arrival order.
type recordingEventBus struct {
	mu       sync.Mutex
	received_ []biz.Activity
}

func (b *recordingEventBus) Publish(_ context.Context, ev biz.ActivityEvent) {
	b.mu.Lock()
	b.received_ = append(b.received_, ev.Activity)
	b.mu.Unlock()
}

func (b *recordingEventBus) Subscribe(_ biz.ActivityEventSubscribeOptions) (<-chan biz.ActivityEvent, func()) {
	ch := make(chan biz.ActivityEvent)
	return ch, func() {}
}

func (b *recordingEventBus) DropCount() uint64 { return 0 }

func (b *recordingEventBus) received() []biz.Activity {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]biz.Activity, len(b.received_))
	copy(out, b.received_)
	return out
}

func (b *recordingEventBus) receivedSeq() []int64 {
	acts := b.received()
	out := make([]int64, len(acts))
	for i, a := range acts {
		out[i] = a.Seq
	}
	return out
}

func bizID(i int) string {
	return "act-" + bizIDint(int64(i))
}

func bizIDint(i int64) string {
	const digits = "0123456789"
	if i == 0 {
		return "0"
	}
	out := []byte{}
	for i > 0 {
		out = append([]byte{digits[i%10]}, out...)
		i /= 10
	}
	return string(out)
}

// fakeActivityRepo is a no-op biz.ActivityWriter used to trigger the
// SetActivityRepo path without exercising the persistence path itself.
type fakeActivityRepo struct{}

func (f *fakeActivityRepo) CreateActivity(_ context.Context, a biz.Activity) (biz.Activity, error) {
	return a, nil
}

func (f *fakeActivityRepo) UpdateActivity(_ context.Context, a biz.Activity) (biz.Activity, error) {
	return a, nil
}

func (f *fakeActivityRepo) UpsertActivity(_ context.Context, a biz.Activity) (biz.Activity, error) {
	return a, nil
}
