package knowledge

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
)

// ── knowledge_recalled notice (29-token P2-2: cited 回采) ─────────────────

// noticeRecorder implements biz.ActivityEmitter capturing EmitNotice calls.
type noticeRecorder struct {
	calls []noticeCall
}

type noticeCall struct{ content, noticeType string }

func (r *noticeRecorder) EmitNotice(_ context.Context, content, noticeType string) error {
	r.calls = append(r.calls, noticeCall{content: content, noticeType: noticeType})
	return nil
}
func (r *noticeRecorder) EmitConfirmRequest(context.Context, biz.ActivityConfirmParams) (string, error) {
	return "", nil
}
func (r *noticeRecorder) EmitConfirmResult(context.Context, string, bool) error { return nil }
func (r *noticeRecorder) EmitConfirmTimeout(context.Context, string) error      { return nil }

func TestEmitKnowledgeRecalledNotice_EmitsJSONPayload(t *testing.T) {
	rec := &noticeRecorder{}
	ctx := biz.WithActivityEmitter(context.Background(), rec)
	chunks := []biz.KnowledgeChunk{
		{ID: "k-1", Content: "向量检索第一段", Score: 0.91, DocID: "doc-1"},
		{ID: "k-2", Content: "向量检索第二段", Score: 0.72, DocID: "doc-2"},
	}
	emitKnowledgeRecalledNotice(ctx, chunks)
	if len(rec.calls) != 1 {
		t.Fatalf("expected 1 notice, got %d", len(rec.calls))
	}
	if rec.calls[0].noticeType != bizknowledge.KnowledgeRecalledNoticeType {
		t.Fatalf("noticeType = %q, want %q", rec.calls[0].noticeType, bizknowledge.KnowledgeRecalledNoticeType)
	}
	var payload knowledgeRecalledNoticePayload
	if err := json.Unmarshal([]byte(rec.calls[0].content), &payload); err != nil {
		t.Fatalf("notice content is not valid JSON: %v", err)
	}
	if len(payload.Chunks) != 2 {
		t.Fatalf("payload chunks = %d, want 2", len(payload.Chunks))
	}
	if payload.Chunks[0].ChunkID != "k-1" || payload.Chunks[0].DocID != "doc-1" || payload.Chunks[0].Score != 0.91 {
		t.Fatalf("unexpected first chunk: %+v", payload.Chunks[0])
	}
}

func TestEmitKnowledgeRecalledNotice_NilEmitterNoPanic(t *testing.T) {
	// 无 emitter（独立工具执行路径）必须静默 no-op。
	emitKnowledgeRecalledNotice(context.Background(), []biz.KnowledgeChunk{{ID: "k-1", Content: "x"}})
}

func TestEmitKnowledgeRecalledNotice_NoChunksSkips(t *testing.T) {
	rec := &noticeRecorder{}
	ctx := biz.WithActivityEmitter(context.Background(), rec)
	emitKnowledgeRecalledNotice(ctx, nil)
	if len(rec.calls) != 0 {
		t.Fatalf("no chunks must not emit, got %d", len(rec.calls))
	}
}

func TestEmitKnowledgeRecalledNotice_CapsAndSkipsBlankIDs(t *testing.T) {
	rec := &noticeRecorder{}
	ctx := biz.WithActivityEmitter(context.Background(), rec)
	chunks := make([]biz.KnowledgeChunk, 0, knowledgeRecalledMaxChunks+3)
	for i := 0; i < knowledgeRecalledMaxChunks+3; i++ {
		chunks = append(chunks, biz.KnowledgeChunk{ID: strings.Repeat("k", 4) + string(rune('a'+i)), Content: "c"})
	}
	chunks = append(chunks, biz.KnowledgeChunk{ID: "  ", Content: "blank id"})
	emitKnowledgeRecalledNotice(ctx, chunks)
	if len(rec.calls) != 1 {
		t.Fatalf("expected 1 notice, got %d", len(rec.calls))
	}
	var payload knowledgeRecalledNoticePayload
	if err := json.Unmarshal([]byte(rec.calls[0].content), &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(payload.Chunks) != knowledgeRecalledMaxChunks {
		t.Fatalf("payload chunks = %d, want cap %d", len(payload.Chunks), knowledgeRecalledMaxChunks)
	}
}

func TestEmitKnowledgeRecalledNotice_TruncatesLongLines(t *testing.T) {
	rec := &noticeRecorder{}
	ctx := biz.WithActivityEmitter(context.Background(), rec)
	long := strings.Repeat("长", knowledgeRecalledMaxLineRunes+50)
	emitKnowledgeRecalledNotice(ctx, []biz.KnowledgeChunk{{ID: "k-1", Content: long}})
	var payload knowledgeRecalledNoticePayload
	if err := json.Unmarshal([]byte(rec.calls[0].content), &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if got := len([]rune(payload.Chunks[0].Line)); got > knowledgeRecalledMaxLineRunes+1 { // +1 for …
		t.Fatalf("line runes = %d, want ≤ %d", got, knowledgeRecalledMaxLineRunes+1)
	}
}
