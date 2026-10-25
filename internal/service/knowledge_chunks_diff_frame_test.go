package service

import (
	"context"
	"sync"
	"testing"

	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
)

// ── G-32 T5 决策9 帧契钉：键集+逐值相等（map 键序断言天然 flaky 故对值对钉）──

// diffFrameBus SystemNoticeEvent 捕获（沟事写锁护——Publish 可能自后台路径唤）。
type diffFrameBus struct {
	mu   sync.Mutex
	evts []biz.Event
}

func (b *diffFrameBus) Publish(_ context.Context, e biz.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.evts = append(b.evts, e)
}

func (b *diffFrameBus) Subscribe(_ biz.EventSubscribeOptions) (<-chan biz.Event, func()) {
	ch := make(chan biz.Event)
	return ch, func() { close(ch) }
}

func (b *diffFrameBus) notices() []*biz.SystemNoticeEvent {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []*biz.SystemNoticeEvent
	for _, e := range b.evts {
		if n, ok := e.(*biz.SystemNoticeEvent); ok {
			out = append(out, n)
		}
	}
	return out
}

// 键集+逐值相等：event_type/collection_id/document_id/added/deleted/kept/
// chunk_count/producer 八键恒等。双口径钳制（T5 终审 wf_f76a9b06-996 R-2）：
// chunk_count=ChildAdded+ChildKept 子块口径——本钉故意令全行口径 sum(5)≠
// 子块口径 sum(4)，钉死帧值取子块字段而非 Added+Kept（parent 行入切片即漂）。
func TestKnowledgeChunksDiffMeta_KeySetAndValues(t *testing.T) {
	stats := bizknowledge.ChunkDiffStats{Added: 2, Deleted: 1, Kept: 3, ChildAdded: 2, ChildKept: 2, VersionNo: 7, FastPath: false, Exact: true}
	meta := KnowledgeChunksDiffMeta("col-9", "doc-7", stats, "ingest")

	want := map[string]any{
		"event_type":    "knowledge_chunks_diff",
		"collection_id": "col-9",
		"document_id":   "doc-7",
		"added":         2,
		"deleted":       1,
		"kept":          3,
		"chunk_count":   4, // ChildAdded(2)+ChildKept(2)——恰与 Added+Kept(5) 分叉
		"producer":      "ingest",
	}
	if len(meta) != len(want) {
		t.Fatalf("meta 键数 = %d, want %d（键集漂移——帧契约破）: %v", len(meta), len(want), meta)
	}
	for k, v := range want {
		got, ok := meta[k]
		if !ok {
			t.Fatalf("meta 缺键 %q（契约键集破）", k)
		}
		if got != v {
			t.Fatalf("meta[%q] = %v, want %v", k, got, v)
		}
	}
}

// Exact 闸钉：Exact=true 发帧（类型/载荷逐值）；Exact=false（退化路径）零发布；
// nil bus 静默不 panic。
func TestKnowledgeChunksDiffFrame_ExactGateAndPublish(t *testing.T) {
	bus := &diffFrameBus{}
	stats := bizknowledge.ChunkDiffStats{Added: 2, Deleted: 1, Kept: 3, ChildAdded: 2, ChildKept: 2, VersionNo: 7, Exact: true}
	PublishKnowledgeChunksDiffFrame(bus, "col-9", "doc-7", stats, "ingest")

	notices := bus.notices()
	if len(notices) != 1 {
		t.Fatalf("帧数 = %d, want 1", len(notices))
	}
	n := notices[0]
	if n.NoticeType != "knowledge_chunks_diff" {
		t.Fatalf("NoticeType = %q, want knowledge_chunks_diff", n.NoticeType)
	}
	meta := n.Meta
	if meta["event_type"] != "knowledge_chunks_diff" || meta["document_id"] != "doc-7" || meta["chunk_count"] != 4 {
		t.Fatalf("帧载荷失真（chunk_count 须子块口径 4 ≠ 全行 5）: %v", meta)
	}

	// Exact=false 伪全量（退化 fakes）——帧函数零发布。
	bus2 := &diffFrameBus{}
	PublishKnowledgeChunksDiffFrame(bus2, "col-9", "doc-7",
		bizknowledge.ChunkDiffStats{Added: 4, Exact: false}, "ingest")
	if got := len(bus2.notices()); got != 0 {
		t.Fatalf("非 Exact 不得发帧（repo 不支持 DiffStats 时零发布）, got %d", got)
	}

	// nil bus：静默（不 panic）。
	PublishKnowledgeChunksDiffFrame(nil, "col-9", "doc-7", stats, "ingest")
}

// 并发帧乱序保障边界：Publish 本身无锁要求由 bus 实现（此钉只锁「发布即
// appends 一次」语义，防帧函数内部双发）。
func TestKnowledgeChunksDiffFrame_SinglePublishPerCall(t *testing.T) {
	bus := &diffFrameBus{}
	stats := bizknowledge.ChunkDiffStats{Added: 1, Exact: true}
	for i := 0; i < 3; i++ {
		PublishKnowledgeChunksDiffFrame(bus, "col", "doc", stats, "vault")
	}
	if got := len(bus.notices()); got != 3 {
		t.Fatalf("帧函数必须每次调用至多发一帧, got %d", got)
	}
}

// 终态帧逐字节不动钉（决策9 对照面）：publishKnowledgeIngest 的 meta 键集与
// 消息前缀是 G-05 已消费契约——diff 帧并存不冒充，本钉防接缝期顺手改型。
func TestPublishKnowledgeIngest_ContractFrozen(t *testing.T) {
	bus := &diffFrameBus{}
	s := &KnowledgeService{eventBus: bus}
	s.publishKnowledgeIngest("col-1", "doc-1", "indexed", "", 12)

	notices := bus.notices()
	if len(notices) != 1 {
		t.Fatalf("终态帧数 = %d, want 1", len(notices))
	}
	n := notices[0]
	if n.NoticeType != "knowledge_ingest" {
		t.Fatalf("NoticeType = %q, want knowledge_ingest", n.NoticeType)
	}
	want := map[string]any{
		"event_type":    "knowledge_ingest",
		"collection_id": "col-1",
		"document_id":   "doc-1",
		"status":        "indexed",
		"error_message": "",
		"chunk_count":   12,
	}
	if len(n.Meta) != len(want) {
		t.Fatalf("终态帧 meta 键数 = %d, want %d: %v", len(n.Meta), len(want), n.Meta)
	}
	for k, v := range want {
		if n.Meta[k] != v {
			t.Fatalf("终态帧 meta[%q] = %v, want %v", k, n.Meta[k], v)
		}
	}
	if n.Message != "Knowledge document ingest: indexed" {
		t.Fatalf("终态帧消息体 = %q（逐字节冻结）", n.Message)
	}
}
