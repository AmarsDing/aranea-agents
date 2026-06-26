package agent

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
)

// BenchmarkSequencerV2_Throughput measures max events/second for v2 sequencer.
// Reports ns/op for a single publish call. Should be < 100μs/op (well below
// the 16ms batch window), so the single-publish-worker design does not
// become a bottleneck under streaming load.
func BenchmarkSequencerV2_Throughput(b *testing.B) {
	eventBus := &recordingEventBus{}
	repo := &fakeActivityRepo{}
	seq := newActivityEventSequencer(eventBus, nil)
	seq.SetActivityRepo(repo)

	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
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
		_ = seq.publish(ctx, a.ID, publishTask{
			event:    ev,
			activity: a,
		})
	}
	b.StopTimer()
	seq.Close()
}
