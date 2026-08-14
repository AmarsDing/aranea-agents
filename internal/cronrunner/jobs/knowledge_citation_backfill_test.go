package jobs

import (
	"context"
	"errors"
	"testing"
	"time"

	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/pkg/loggateway"
)

// ── chunkCited heuristics（委托 factCited，知识侧只验证适配语义） ──────────

func TestChunkCited_IDReference(t *testing.T) {
	chunk := bizknowledge.CitationChunkRef{ChunkID: "abcdef12-3456-7890", Content: "完全不相关的知识块内容"}
	reply := "根据资料 [chunk:abcdef12] 可知……"
	if !chunkCited(normalizeCitationText(reply), chunk) {
		t.Fatal("reply echoing the chunk short ID should count as cited")
	}
}

func TestChunkCited_ContentOverlap(t *testing.T) {
	chunk := bizknowledge.CitationChunkRef{ChunkID: "k1", Content: "向量数据库通过近似最近邻索引加速高维相似度检索"}
	reply := "是的，向量数据库通过近似最近邻索引加速高维相似度检索，所以查询很快。"
	if !chunkCited(normalizeCitationText(reply), chunk) {
		t.Fatal("reply with high content overlap should count as cited")
	}
}

func TestChunkCited_NoOverlap(t *testing.T) {
	chunk := bizknowledge.CitationChunkRef{ChunkID: "k1", Content: "向量数据库通过近似最近邻索引加速高维相似度检索"}
	reply := "今天天气真不错，适合出门散步。"
	if chunkCited(normalizeCitationText(reply), chunk) {
		t.Fatal("unrelated reply must not count as cited")
	}
}

// ── worker pass ─────────────────────────────────────────────────────────

type fakeKnowledgeCitationTraceReader struct {
	candidates []bizknowledge.KnowledgeCitationCandidate
	err        error
}

func (f *fakeKnowledgeCitationTraceReader) ListKnowledgeCitationCandidates(_ context.Context, _ time.Time, _ int) ([]bizknowledge.KnowledgeCitationCandidate, error) {
	return f.candidates, f.err
}

type fakeKnowledgeChunkCitationRecorder struct {
	recorded []bizknowledge.ChunkCitation
	err      error
}

func (f *fakeKnowledgeChunkCitationRecorder) RecordChunkCitations(_ context.Context, citations []bizknowledge.ChunkCitation) error {
	if f.err != nil {
		return f.err
	}
	f.recorded = append(f.recorded, citations...)
	return nil
}

func TestKnowledgeCitationBackfill_RecordsDetectedCitations(t *testing.T) {
	reader := &fakeKnowledgeCitationTraceReader{candidates: []bizknowledge.KnowledgeCitationCandidate{
		{
			TurnID:    "turn-1",
			ReplyText: "好的，根据资料，向量数据库通过近似最近邻索引加速检索。",
			Chunks: []bizknowledge.CitationChunkRef{
				{ChunkID: "chunk-cited", Content: "向量数据库通过近似最近邻索引加速检索"},
				{ChunkID: "chunk-not-cited", Content: "蒸馏咖啡的萃取温度应当控制在九十二度"},
			},
		},
	}}
	recorder := &fakeKnowledgeChunkCitationRecorder{}
	w := NewKnowledgeCitationBackfillWorker(0, reader, recorder, loggateway.NewNoop())
	w.RunOnce(context.Background())
	if len(recorder.recorded) != 1 || recorder.recorded[0].ChunkID != "chunk-cited" || recorder.recorded[0].TurnID != "turn-1" {
		t.Fatalf("recorded = %+v, want exactly one citation for chunk-cited/turn-1", recorder.recorded)
	}
}

func TestKnowledgeCitationBackfill_ReaderErrorIsGraceful(t *testing.T) {
	reader := &fakeKnowledgeCitationTraceReader{err: errors.New("db down")}
	recorder := &fakeKnowledgeChunkCitationRecorder{}
	w := NewKnowledgeCitationBackfillWorker(0, reader, recorder, loggateway.NewNoop())
	w.RunOnce(context.Background()) // must not panic
	if len(recorder.recorded) != 0 {
		t.Fatalf("recorded = %+v, want none on reader error", recorder.recorded)
	}
}

func TestKnowledgeCitationBackfill_RecorderErrorIsGraceful(t *testing.T) {
	reader := &fakeKnowledgeCitationTraceReader{candidates: []bizknowledge.KnowledgeCitationCandidate{
		{TurnID: "turn-1", ReplyText: "向量数据库通过近似最近邻索引加速检索", Chunks: []bizknowledge.CitationChunkRef{{ChunkID: "k1", Content: "向量数据库通过近似最近邻索引加速检索"}}},
	}}
	recorder := &fakeKnowledgeChunkCitationRecorder{err: errors.New("db down")}
	w := NewKnowledgeCitationBackfillWorker(0, reader, recorder, loggateway.NewNoop())
	w.RunOnce(context.Background()) // must not panic
}

func TestKnowledgeCitationBackfill_NilDepsReturnNil(t *testing.T) {
	if w := NewKnowledgeCitationBackfillWorker(0, nil, &fakeKnowledgeChunkCitationRecorder{}, loggateway.NewNoop()); w != nil {
		t.Fatal("nil reader should yield nil worker")
	}
	if w := NewKnowledgeCitationBackfillWorker(0, &fakeKnowledgeCitationTraceReader{}, nil, loggateway.NewNoop()); w != nil {
		t.Fatal("nil recorder should yield nil worker")
	}
}

func TestKnowledgeCitationBackfill_NoCandidates(t *testing.T) {
	reader := &fakeKnowledgeCitationTraceReader{}
	recorder := &fakeKnowledgeChunkCitationRecorder{}
	w := NewKnowledgeCitationBackfillWorker(0, reader, recorder, loggateway.NewNoop())
	w.RunOnce(context.Background())
	if len(recorder.recorded) != 0 {
		t.Fatalf("recorded = %+v, want none when no candidates", recorder.recorded)
	}
}
