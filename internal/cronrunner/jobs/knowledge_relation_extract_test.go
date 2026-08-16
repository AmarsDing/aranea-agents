package jobs

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/internal/knowledge"
	"aranea-agents/pkg/loggateway"
)

// 自治理图谱 M2 关系抽取工人工约：
//   - 每集合只抽热文档（ListHotDocuments 出口），逐文档 ExtractDoc；
//   - 幂等跳过（stats.Skipped）不占 LLM 预算；预算（maxPerPass）耗尽即停；
//   - 单文档失败 Warn 继续，不中断整轮；依赖缺失工人不装配（nil）。

type stubRelationCollectionLister struct {
	cols []bizknowledge.Collection
	err  error
}

func (s *stubRelationCollectionLister) ListCollections(context.Context, string, int, int) ([]bizknowledge.Collection, int, error) {
	return s.cols, len(s.cols), s.err
}

type stubRelationHotDocs struct {
	byCollection map[string][]string
	err          error
}

func (s *stubRelationHotDocs) ListHotDocuments(_ context.Context, collectionID string, _, _, _ int) ([]string, error) {
	return s.byCollection[collectionID], s.err
}

type stubRelationExtractor struct {
	calls   []string
	skipAll bool
	errOn   map[string]error
	stats   map[string]knowledge.RelationExtractStats
}

func (s *stubRelationExtractor) ExtractDoc(_ context.Context, docID string) (knowledge.RelationExtractStats, error) {
	s.calls = append(s.calls, docID)
	if err := s.errOn[docID]; err != nil {
		return knowledge.RelationExtractStats{}, err
	}
	if s.skipAll {
		return knowledge.RelationExtractStats{Skipped: true, SkipReason: "unchanged"}, nil
	}
	if st, ok := s.stats[docID]; ok {
		return st, nil
	}
	return knowledge.RelationExtractStats{Links: 1, OpenLinks: 1}, nil
}

func newRelationExtractWorkerForTest(cols []string, hot map[string][]string, ex *stubRelationExtractor) *KnowledgeRelationExtractWorker {
	lister := &stubRelationCollectionLister{}
	for _, c := range cols {
		lister.cols = append(lister.cols, bizknowledge.Collection{ID: c})
	}
	return NewKnowledgeRelationExtractWorker(time.Minute, lister, &stubRelationHotDocs{byCollection: hot}, ex, loggateway.NewNoop())
}

func TestKnowledgeRelationExtractWorker_RunOnceExtractsHotDocs(t *testing.T) {
	ex := &stubRelationExtractor{}
	w := newRelationExtractWorkerForTest(
		[]string{"c1", "c2"},
		map[string][]string{"c1": {"d1", "d2"}, "c2": {"d3"}},
		ex,
	)
	w.RunOnce(context.Background())
	if len(ex.calls) != 3 {
		t.Fatalf("extract calls = %v, want 3 hot docs", ex.calls)
	}
}

func TestKnowledgeRelationExtractWorker_SkippedDocsDontConsumeBudget(t *testing.T) {
	// 30 个热文档全部幂等跳过：不占预算，一轮内全部访问。
	ex := &stubRelationExtractor{skipAll: true}
	hot := map[string][]string{}
	var cols []string
	for i := 0; i < 3; i++ {
		cid := fmt.Sprintf("c%d", i)
		cols = append(cols, cid)
		for j := 0; j < 10; j++ {
			hot[cid] = append(hot[cid], fmt.Sprintf("%s-d%d", cid, j))
		}
	}
	w := newRelationExtractWorkerForTest(cols, hot, ex)
	w.RunOnce(context.Background())
	if len(ex.calls) != 30 {
		t.Errorf("skipped docs must not consume budget: calls = %d, want 30", len(ex.calls))
	}
}

func TestKnowledgeRelationExtractWorker_BudgetCapsPass(t *testing.T) {
	// 3 集合 × 10 热文档 = 30 候选，预算 20 → 一轮只抽 20。
	ex := &stubRelationExtractor{}
	hot := map[string][]string{}
	var cols []string
	for i := 0; i < 3; i++ {
		cid := fmt.Sprintf("c%d", i)
		cols = append(cols, cid)
		for j := 0; j < 10; j++ {
			hot[cid] = append(hot[cid], fmt.Sprintf("%s-d%d", cid, j))
		}
	}
	w := newRelationExtractWorkerForTest(cols, hot, ex)
	w.RunOnce(context.Background())
	if len(ex.calls) != knowledgeRelationExtractMaxPerPass {
		t.Errorf("budget cap: calls = %d, want %d", len(ex.calls), knowledgeRelationExtractMaxPerPass)
	}
}

func TestKnowledgeRelationExtractWorker_DocFailureContinues(t *testing.T) {
	ex := &stubRelationExtractor{errOn: map[string]error{"d1": errors.New("llm down")}}
	w := newRelationExtractWorkerForTest(
		[]string{"c1"},
		map[string][]string{"c1": {"d1", "d2"}},
		ex,
	)
	w.RunOnce(context.Background())
	if len(ex.calls) != 2 {
		t.Errorf("doc failure must not abort pass: calls = %v", ex.calls)
	}
}

func TestKnowledgeRelationExtractWorker_NilDepsNotAssembled(t *testing.T) {
	ex := &stubRelationExtractor{}
	lister := &stubRelationCollectionLister{}
	hot := &stubRelationHotDocs{}
	if w := NewKnowledgeRelationExtractWorker(0, nil, hot, ex, nil); w != nil {
		t.Error("nil collections lister must yield nil worker")
	}
	if w := NewKnowledgeRelationExtractWorker(0, lister, nil, ex, nil); w != nil {
		t.Error("nil hot lister must yield nil worker")
	}
	if w := NewKnowledgeRelationExtractWorker(0, lister, hot, nil, nil); w != nil {
		t.Error("nil extractor must yield nil worker")
	}
}

func TestKnowledgeRelationExtractWorker_ListCollectionsErrorGraceful(t *testing.T) {
	ex := &stubRelationExtractor{}
	w := NewKnowledgeRelationExtractWorker(time.Minute,
		&stubRelationCollectionLister{err: errors.New("db down")},
		&stubRelationHotDocs{}, ex, loggateway.NewNoop())
	w.RunOnce(context.Background()) // 不 panic、不抽取
	if len(ex.calls) != 0 {
		t.Errorf("collections failure must skip pass, calls = %v", ex.calls)
	}
}

func TestKnowledgeRelationExtractDisabled(t *testing.T) {
	t.Setenv("KNOWLEDGE_RELATION_EXTRACT_DISABLED", "1")
	if !KnowledgeRelationExtractDisabled() {
		t.Error("env=1 must disable worker")
	}
	t.Setenv("KNOWLEDGE_RELATION_EXTRACT_DISABLED", "")
	if KnowledgeRelationExtractDisabled() {
		t.Error("empty env must not disable worker")
	}
}
