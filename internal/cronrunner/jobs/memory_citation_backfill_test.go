package jobs

import (
	"context"
	"errors"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// ── factCited heuristics ────────────────────────────────────────────────

func TestFactCited_IDReference(t *testing.T) {
	fact := biz.CitationFactRef{FactID: "abcdef12-3456-7890-abcd-ef1234567890", Statement: "完全不相关的陈述内容"}
	reply := "根据记忆 [id:abcdef12] 来看，答案是……"
	if !factCited(normalizeCitationText(reply), fact) {
		t.Fatal("reply echoing the provenance short ID should count as cited")
	}
}

func TestFactCited_ShortStatementContainment(t *testing.T) {
	fact := biz.CitationFactRef{FactID: "f1", Statement: "用户喜欢喝咖啡"}
	reply := "好的，我记得用户喜欢喝咖啡，给你推荐美式。"
	if !factCited(normalizeCitationText(reply), fact) {
		t.Fatal("reply containing the full short statement should count as cited")
	}
}

func TestFactCited_LongStatementGramOverlap(t *testing.T) {
	fact := biz.CitationFactRef{FactID: "f1", Statement: "用户是一名后端工程师主要使用Go语言开发"}
	// Reply paraphrases but keeps most 8-rune windows of the statement.
	reply := "了解到用户是一名后端工程师主要使用Go语言开发，所以推荐相关实践。"
	if !factCited(normalizeCitationText(reply), fact) {
		t.Fatal("reply with ≥50% k-gram overlap should count as cited")
	}
}

func TestFactCited_NoOverlap(t *testing.T) {
	fact := biz.CitationFactRef{FactID: "f1", Statement: "用户是一名后端工程师主要使用Go语言开发"}
	reply := "今天天气真不错，适合出门散步。"
	if factCited(normalizeCitationText(reply), fact) {
		t.Fatal("unrelated reply must not count as cited")
	}
}

func TestFactCited_TooShortStatement(t *testing.T) {
	fact := biz.CitationFactRef{FactID: "f1", Statement: "咖啡"}
	reply := "咖啡很好喝"
	if factCited(normalizeCitationText(reply), fact) {
		t.Fatal("statements shorter than the min signal length must be skipped")
	}
}

func TestFactCited_CaseInsensitiveEnglish(t *testing.T) {
	fact := biz.CitationFactRef{FactID: "f1", Statement: "User prefers dark mode"}
	reply := "Sure, since the user prefers dark mode I will keep it."
	if !factCited(normalizeCitationText(reply), fact) {
		t.Fatal("matching must be case-insensitive")
	}
}

// ── worker pass ─────────────────────────────────────────────────────────

type fakeCitationTraceReader struct {
	candidates []biz.CitationCandidate
	err        error
}

func (f *fakeCitationTraceReader) ListCitationCandidates(_ context.Context, _ time.Time, _ int) ([]biz.CitationCandidate, error) {
	return f.candidates, f.err
}

type fakeCitationRecorder struct {
	recorded []biz.FactCitation
	err      error
}

func (f *fakeCitationRecorder) RecordFactCitations(_ context.Context, citations []biz.FactCitation) error {
	if f.err != nil {
		return f.err
	}
	f.recorded = append(f.recorded, citations...)
	return nil
}

func TestCitationBackfill_RecordsDetectedCitations(t *testing.T) {
	reader := &fakeCitationTraceReader{candidates: []biz.CitationCandidate{
		{
			TurnID:    "turn-1",
			ReplyText: "好的，我记得用户喜欢喝咖啡。",
			Facts: []biz.CitationFactRef{
				{FactID: "fact-cited", Statement: "用户喜欢喝咖啡"},
				{FactID: "fact-not-cited", Statement: "用户是一名后端工程师"},
			},
		},
	}}
	recorder := &fakeCitationRecorder{}
	w := NewMemoryCitationBackfillWorker(0, reader, recorder, loggateway.NewNoop())
	w.RunOnce(context.Background())
	if len(recorder.recorded) != 1 || recorder.recorded[0].FactID != "fact-cited" || recorder.recorded[0].TurnID != "turn-1" {
		t.Fatalf("recorded = %+v, want exactly one citation for fact-cited/turn-1", recorder.recorded)
	}
}

func TestCitationBackfill_ReaderErrorIsGraceful(t *testing.T) {
	reader := &fakeCitationTraceReader{err: errors.New("db down")}
	recorder := &fakeCitationRecorder{}
	w := NewMemoryCitationBackfillWorker(0, reader, recorder, loggateway.NewNoop())
	w.RunOnce(context.Background()) // must not panic
	if len(recorder.recorded) != 0 {
		t.Fatalf("recorded = %+v, want none on reader error", recorder.recorded)
	}
}

func TestCitationBackfill_RecorderErrorIsGraceful(t *testing.T) {
	reader := &fakeCitationTraceReader{candidates: []biz.CitationCandidate{
		{TurnID: "turn-1", ReplyText: "用户喜欢喝咖啡", Facts: []biz.CitationFactRef{{FactID: "f1", Statement: "用户喜欢喝咖啡"}}},
	}}
	recorder := &fakeCitationRecorder{err: errors.New("db down")}
	w := NewMemoryCitationBackfillWorker(0, reader, recorder, loggateway.NewNoop())
	w.RunOnce(context.Background()) // must not panic
}

func TestCitationBackfill_NilDepsReturnNil(t *testing.T) {
	if w := NewMemoryCitationBackfillWorker(0, nil, &fakeCitationRecorder{}, loggateway.NewNoop()); w != nil {
		t.Fatal("nil reader should yield nil worker")
	}
	if w := NewMemoryCitationBackfillWorker(0, &fakeCitationTraceReader{}, nil, loggateway.NewNoop()); w != nil {
		t.Fatal("nil recorder should yield nil worker")
	}
}

func TestCitationBackfill_NoCandidates(t *testing.T) {
	reader := &fakeCitationTraceReader{}
	recorder := &fakeCitationRecorder{}
	w := NewMemoryCitationBackfillWorker(0, reader, recorder, loggateway.NewNoop())
	w.RunOnce(context.Background())
	if len(recorder.recorded) != 0 {
		t.Fatalf("recorded = %+v, want none when no candidates", recorder.recorded)
	}
}
