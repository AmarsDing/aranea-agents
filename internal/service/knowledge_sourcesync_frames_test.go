// knowledge_sourcesync_frames_test.go —— G-38 W8 帧缝钉阵（内存 bus 实收断言面）。
package service

import (
	"context"
	"sync"
	"testing"

	"aranea-agents/internal/biz"
	bizsync "aranea-agents/internal/biz/knowledge/sourcesync"
	knowledgesync "aranea-agents/internal/knowledge/sourcesync"
)

// memFrameBus biz.EventBus 最小实收实现（只钉 Publish 面）。
type memFrameBus struct {
	mu     sync.Mutex
	events []*biz.SystemNoticeEvent
}

func (b *memFrameBus) Publish(_ context.Context, e biz.Event) {
	sn, ok := e.(*biz.SystemNoticeEvent)
	if !ok {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, sn)
}

// Subscribe 本钉阵不消费订阅面（biz.EventBus 完整实现义务）。
func (b *memFrameBus) Subscribe(_ biz.EventSubscribeOptions) (<-chan biz.Event, func()) {
	ch := make(chan biz.Event)
	return ch, func() { close(ch) }
}

func TestSourceSyncFrameSink_StartedFinishedPair(t *testing.T) {
	bus := &memFrameBus{}
	sink := NewSourceSyncFrameSink(bus)
	if sink == nil {
		t.Fatal("non-nil bus must yield sink")
	}
	ev := knowledgesync.RunEvent{
		ConnectorID: "c1", Kind: bizsync.KindSharedDir, RunID: "r1", RunToken: "t1",
		Stats:  knowledgesync.RunStats{Seen: 9, Created: 2, Updated: 1, Unchanged: 6, Deleted: 1, MissUnknown: 1, BytesPulled: 4096},
		Status: bizsync.RunStatusSuccess, ErrorCode: bizsync.ErrCodeNone,
	}
	sink.OnRunStarted(context.Background(), ev)
	sink.OnRunFinished(context.Background(), ev)

	if len(bus.events) != 2 {
		t.Fatalf("events = %d, want ≤1 started + ≤1 finished", len(bus.events))
	}
	started, done := bus.events[0], bus.events[1]
	if started.NoticeType != SourceSyncFrameNoticeType || done.NoticeType != SourceSyncFrameNoticeType {
		t.Fatalf("notice type = %q/%q", started.NoticeType, done.NoticeType)
	}
	if started.Meta["event_phase"] != "started" || done.Meta["event_phase"] != "finished" {
		t.Fatalf("phases = %v/%v", started.Meta["event_phase"], done.Meta["event_phase"])
	}
	// started 帧零计数面（泄漏纪律：计数只在终态帧）
	if _, leak := started.Meta["created"]; leak {
		t.Fatalf("started frame must not carry stats: %+v", started.Meta)
	}
	for _, k := range []string{"connector_id", "kind", "run_id", "status", "error_code",
		"seen", "created", "updated", "unchanged", "fetch_failed", "deleted", "miss_unknown", "bytes_pulled"} {
		if _, ok := done.Meta[k]; !ok {
			t.Fatalf("done meta missing key %q: %+v", k, done.Meta)
		}
	}
	if done.Meta["created"] != 2 || done.Meta["deleted"] != 1 || done.Meta["bytes_pulled"] != int64(4096) {
		t.Fatalf("done stats = %+v", done.Meta)
	}
	if done.Meta["status"] != string(bizsync.RunStatusSuccess) {
		t.Fatalf("done status = %v", done.Meta["status"])
	}
}

// TestSourceSyncFrameSink_AuthPausedTransition ⑥-4 auth 迁移帧：auth 家族码
// 原样透传 error_code（前端依此高亮凭据区）——迁移必发由 runner finalize
// 无条件帧点保证，本钉锁载荷。
func TestSourceSyncFrameSink_AuthPausedTransition(t *testing.T) {
	bus := &memFrameBus{}
	sink := NewSourceSyncFrameSink(bus)
	sink.OnRunFinished(context.Background(), knowledgesync.RunEvent{
		ConnectorID: "c1", Kind: bizsync.KindNotion, RunID: "r2",
		Status: bizsync.RunStatusFailed, ErrorCode: bizsync.ErrCodeCredentialMissing,
	})
	if len(bus.events) != 1 {
		t.Fatalf("events = %d", len(bus.events))
	}
	done := bus.events[0]
	if done.Meta["error_code"] != string(bizsync.ErrCodeCredentialMissing) {
		t.Fatalf("auth code passthrough = %v", done.Meta["error_code"])
	}
	if done.Meta["status"] != string(bizsync.RunStatusFailed) {
		t.Fatalf("status = %v", done.Meta["status"])
	}
}

func TestSourceSyncFrameSink_NilBusSilent(t *testing.T) {
	if sink := NewSourceSyncFrameSink(nil); sink != nil {
		t.Fatalf("nil bus must yield nil sink (runner noop 闸接管)")
	}
}
