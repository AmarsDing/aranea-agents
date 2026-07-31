package team

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
	"trpc.group/trpc-go/trpc-agent-go/model"
)

// teeRecordingBus is a biz.EventBus fake that records published events.
type teeRecordingBus struct {
	mu     sync.Mutex
	events []biz.Event
}

func (b *teeRecordingBus) Publish(_ context.Context, e biz.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, e)
}

func (b *teeRecordingBus) Subscribe(biz.EventSubscribeOptions) (<-chan biz.Event, func()) {
	return make(chan biz.Event), func() {}
}

func (b *teeRecordingBus) snapshot() []biz.Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]biz.Event, len(b.events))
	copy(out, b.events)
	return out
}

func teeNodeEvent(t *testing.T, object, nodeID string) *trpcevent.Event {
	t.Helper()
	meta := trpcgraph.NodeExecutionMetadata{NodeID: nodeID, NodeType: trpcgraph.NodeTypeAgent, StepNumber: 1}
	raw, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("marshal node meta: %v", err)
	}
	return &trpcevent.Event{
		Response:   &model.Response{Object: object},
		StateDelta: map[string][]byte{trpcgraph.MetadataKeyNode: raw},
	}
}

// drainTee feeds the input events into the tee via feed and collects the
// forwarded stream, returning after the tee closes the output channel.
func drainTee(t *testing.T, in chan<- *trpcevent.Event, out <-chan *trpcevent.Event, feed func(in chan<- *trpcevent.Event)) []*trpcevent.Event {
	t.Helper()
	go feed(in)
	var got []*trpcevent.Event
	timeout := time.After(5 * time.Second)
	for {
		select {
		case ev, ok := <-out:
			if !ok {
				return got
			}
			got = append(got, ev)
		case <-timeout:
			t.Fatal("tee output channel did not close in time")
		}
	}
}

func waitForNotices(t *testing.T, bus *teeRecordingBus, want int) []biz.Event {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if got := bus.snapshot(); len(got) >= want {
			return got
		}
		time.Sleep(5 * time.Millisecond)
	}
	return bus.snapshot()
}

func TestTeeGraphStageNotices_PublishesNodeLifecycle(t *testing.T) {
	bus := &teeRecordingBus{}
	in := make(chan *trpcevent.Event, 8)
	out := teeGraphStageNotices(in, bus, "sess-1", "spirit-1", "graph-1", "exec-1", loggateway.NewNoop())

	forwarded := drainTee(t, in, out, func(feedIn chan<- *trpcevent.Event) {
		feedIn <- teeNodeEvent(t, trpcgraph.ObjectTypeGraphNodeStart, "member-1")
		feedIn <- teeNodeEvent(t, trpcgraph.ObjectTypeGraphNodeComplete, "member-1")
		feedIn <- &trpcevent.Event{Response: &model.Response{Object: trpcgraph.ObjectTypeGraphExecution, Done: true}}
		close(in)
	})

	if len(forwarded) != 3 {
		t.Fatalf("forwarded %d events, want 3 (tee must not drop)", len(forwarded))
	}

	notices := waitForNotices(t, bus, 3)
	if len(notices) != 3 {
		t.Fatalf("published %d notices, want 3", len(notices))
	}
	wantTypes := []string{"node_start", "node_end", "execution_done"}
	for i, w := range wantTypes {
		notice, ok := notices[i].(*biz.SystemNoticeEvent)
		if !ok {
			t.Fatalf("notice %d is %T, want *biz.SystemNoticeEvent", i, notices[i])
		}
		if notice.NoticeType != w {
			t.Errorf("notice %d type = %q, want %q", i, notice.NoticeType, w)
		}
		if got := metaString(notice.Meta, "activity_kind"); got != string(biz.ActivityKindGraphStage) {
			t.Errorf("notice %d activity_kind = %q, want graph_stage", i, got)
		}
		if got := metaString(notice.Meta, "execution_id"); got != "exec-1" {
			t.Errorf("notice %d execution_id = %q, want exec-1", i, got)
		}
		if notice.SpiritSessionID() != "spirit-1" {
			t.Errorf("notice %d session = %q, want spirit-1 (watch subscription filter)", i, notice.SpiritSessionID())
		}
	}
	if got := metaString(notices[0].(*biz.SystemNoticeEvent).Meta, "node_id"); got != "member-1" {
		t.Errorf("node_start node_id = %q, want member-1", got)
	}
}

func TestTeeGraphStageNotices_FiltersHighFrequencyEvents(t *testing.T) {
	bus := &teeRecordingBus{}
	in := make(chan *trpcevent.Event, 8)
	out := teeGraphStageNotices(in, bus, "sess-1", "spirit-1", "graph-1", "exec-1", loggateway.NewNoop())

	forwarded := drainTee(t, in, out, func(feedIn chan<- *trpcevent.Event) {
		feedIn <- &trpcevent.Event{Response: &model.Response{Object: trpcgraph.ObjectTypeGraphPregelStep}}
		feedIn <- &trpcevent.Event{Response: &model.Response{Object: trpcgraph.ObjectTypeGraphStateUpdate}}
		feedIn <- &trpcevent.Event{Response: &model.Response{Object: "graph.channel.update"}}
		feedIn <- &trpcevent.Event{Response: &model.Response{Object: trpcgraph.ObjectTypeGraphNodeCustom}}
		close(in)
	})

	if len(forwarded) != 4 {
		t.Fatalf("forwarded %d events, want 4 (tee must not drop)", len(forwarded))
	}
	// Allow a brief window for any erroneous publishes to land.
	time.Sleep(50 * time.Millisecond)
	if got := len(bus.snapshot()); got != 0 {
		t.Fatalf("published %d notices for high-frequency events, want 0", got)
	}
}

func TestTeeGraphStageNotices_PassthroughWhenNilBusOrEmptyExec(t *testing.T) {
	in := make(chan *trpcevent.Event)
	if got := teeGraphStageNotices(in, nil, "s", "sp", "g", "exec-1", loggateway.NewNoop()); got != (<-chan *trpcevent.Event)(in) {
		t.Error("nil bus: expected passthrough (same channel)")
	}
	bus := &teeRecordingBus{}
	if got := teeGraphStageNotices(in, bus, "s", "sp", "g", "", loggateway.NewNoop()); got != (<-chan *trpcevent.Event)(in) {
		t.Error("empty execID: expected passthrough (same channel)")
	}
}
