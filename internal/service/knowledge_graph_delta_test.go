package service

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
)

// ── SP1-D D-2：knowledge.graph.delta WS 增量事件适配器 ───────────────────────

type fakeEventBus struct{ published []biz.Event }

func (f *fakeEventBus) Publish(_ context.Context, e biz.Event) { f.published = append(f.published, e) }
func (f *fakeEventBus) Subscribe(biz.EventSubscribeOptions) (<-chan biz.Event, func()) {
	return nil, func() {}
}

func graphDeltaEdge(coll, srcBlk, srcDoc, dstColl, dstDoc, dstBlk, raw string) bizknowledge.KnowledgeBlockRefEdge {
	return bizknowledge.KnowledgeBlockRefEdge{
		CollectionID:    coll,
		SrcBlockID:      srcBlk,
		SrcDocID:        srcDoc,
		DstCollectionID: dstColl,
		DstDocID:        dstDoc,
		DstBlockID:      dstBlk,
		RawTarget:       raw,
		EdgeType:        "ref",
		Context:         "ctx " + raw,
	}
}

// TestGraphDeltaPublisher 发布 SystemNoticeEvent（noticeType=knowledge.graph.delta），
// meta 携带 version + added/removed 边负载；空 delta 与 nil bus 不发布。
func TestGraphDeltaPublisher(t *testing.T) {
	bus := &fakeEventBus{}
	pub := newKnowledgeGraphDeltaPublisher(bus)

	added := graphDeltaEdge("c1", "b1", "d1", "c2", "d2", "tb", "A#^x")
	removed := graphDeltaEdge("c1", "b2", "d1", "", "", "", "Ghost")
	pub.PublishGraphDelta(context.Background(), bizknowledge.GraphDelta{
		Added:   []bizknowledge.KnowledgeBlockRefEdge{added},
		Removed: []bizknowledge.KnowledgeBlockRefEdge{removed},
		Version: 7,
	})

	if len(bus.published) != 1 {
		t.Fatalf("published = %d, want 1", len(bus.published))
	}
	notice, ok := bus.published[0].(*biz.SystemNoticeEvent)
	if !ok {
		t.Fatalf("event type = %T, want *biz.SystemNoticeEvent", bus.published[0])
	}
	if notice.NoticeType != "knowledge.graph.delta" {
		t.Errorf("NoticeType = %q, want knowledge.graph.delta", notice.NoticeType)
	}
	meta := notice.Meta
	if meta["event_type"] != "knowledge.graph.delta" {
		t.Errorf("meta.event_type = %v", meta["event_type"])
	}
	if meta["version"] != uint64(7) {
		t.Errorf("meta.version = %v, want 7", meta["version"])
	}
	addedMeta, ok := meta["added"].([]map[string]any)
	if !ok || len(addedMeta) != 1 {
		t.Fatalf("meta.added = %v", meta["added"])
	}
	if addedMeta[0]["src_block_id"] != "b1" || addedMeta[0]["dst_block_id"] != "tb" || addedMeta[0]["raw_target"] != "A#^x" {
		t.Errorf("added 边负载错误: %v", addedMeta[0])
	}
	removedMeta, ok := meta["removed"].([]map[string]any)
	if !ok || len(removedMeta) != 1 || removedMeta[0]["raw_target"] != "Ghost" {
		t.Errorf("removed 边负载错误: %v", meta["removed"])
	}

	// 空 delta 不发布（无变化重建不制造 WS 噪声）。
	pub.PublishGraphDelta(context.Background(), bizknowledge.GraphDelta{Version: 7})
	if len(bus.published) != 1 {
		t.Fatalf("空 delta 后 published = %d, want 1", len(bus.published))
	}

	// nil bus → nil publisher（降级安全）。
	if p := newKnowledgeGraphDeltaPublisher(nil); p != nil {
		t.Error("nil bus 应返回 nil publisher")
	}
}
