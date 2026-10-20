package graphrag

import (
	"context"
	"errors"
	"math"
	"reflect"
	"testing"
)

// fakeLow 组合端口假实现（函数字段驱动）。
type fakeLow struct {
	findEntities func(ctx context.Context, query string, maxN int) ([]EntityRef, error)
	docsOf       func(ctx context.Context, entityIDs []string) (map[string][]string, error)
	rank         func(ctx context.Context, query string, docIDs []string, topK int) ([]ChunkHit, error)
}

func (f fakeLow) FindEntities(ctx context.Context, query string, maxN int) ([]EntityRef, error) {
	if f.findEntities == nil {
		return nil, nil
	}
	return f.findEntities(ctx, query, maxN)
}

func (f fakeLow) DocsOfEntities(ctx context.Context, entityIDs []string) (map[string][]string, error) {
	if f.docsOf == nil {
		return nil, nil
	}
	return f.docsOf(ctx, entityIDs)
}

func (f fakeLow) Rank(ctx context.Context, query string, docIDs []string, topK int) ([]ChunkHit, error) {
	return f.rank(ctx, query, docIDs, topK)
}

type fakeExpander struct {
	edges []LinkEdge
	err   error
	calls int
}

func (f *fakeExpander) Neighbors(_ context.Context, _ []string, _ int) ([]LinkEdge, error) {
	f.calls++
	return f.edges, f.err
}

type fakeSummarizer struct {
	out   string
	err   error
	texts []string
}

func (f *fakeSummarizer) Summarize(_ context.Context, texts []string) (string, error) {
	f.texts = append([]string(nil), texts...)
	return f.out, f.err
}

// oneHitPerDoc 每个候选文档按 docIDs 序生成一条命中（RRF 只看 rank 不看原始分）。
func oneHitPerDoc(_ context.Context, _ string, docIDs []string, topK int) ([]ChunkHit, error) {
	hits := make([]ChunkHit, 0, len(docIDs))
	for i, d := range docIDs {
		hits = append(hits, ChunkHit{ChunkID: d + "-c1", DocID: d, Score: 100 - float64(i)})
	}
	if len(hits) > topK {
		hits = hits[:topK]
	}
	return hits, nil
}

// baseFixture 标准场景：低层 d1,d2；扩展边 d1-d3、d3-d4 → 扩展候选 d3,d4。
func baseFixture() (fakeLow, *fakeExpander) {
	low := fakeLow{
		findEntities: func(_ context.Context, _ string, maxN int) ([]EntityRef, error) {
			if maxN != maxAnchorEntities {
				return nil, errors.New("unexpected maxN")
			}
			return []EntityRef{{ID: "e1", Name: "甲", Weight: 1}}, nil
		},
		docsOf: func(_ context.Context, _ []string) (map[string][]string, error) {
			return map[string][]string{"e1": {"d1", "d2"}}, nil
		},
		rank: oneHitPerDoc,
	}
	exp := &fakeExpander{edges: []LinkEdge{
		{SrcDocID: "d1", DstDocID: "d3", Weight: 1},
		{SrcDocID: "d3", DstDocID: "d4", Weight: 1},
	}}
	return low, exp
}

func approxEq(a, b float64) bool { return math.Abs(a-b) < 1e-12 }

func TestDualLevelSearch_RRFFusionOrder(t *testing.T) {
	low, exp := baseFixture()
	hits, tr, err := DualLevelSearch(context.Background(), "q", low, nil, exp, 4)
	if err != nil {
		t.Fatalf("DualLevelSearch() error = %v", err)
	}
	wantIDs := []string{"d1-c1", "d2-c1", "d3-c1", "d4-c1"}
	gotIDs := make([]string, len(hits))
	for i, h := range hits {
		gotIDs[i] = h.ChunkID
	}
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Fatalf("融合顺序 = %v，期望 %v", gotIDs, wantIDs)
	}
	// RRF(k=60)：低层 rank1=1/61、rank2=1/62；扩展 rank1/2 加权 0.7。
	wantScores := []float64{1.0 / 61, 1.0 / 62, 0.7 / 61, 0.7 / 62}
	for i, h := range hits {
		if !approxEq(h.Score, wantScores[i]) {
			t.Fatalf("hits[%d].Score = %v，期望 %v", i, h.Score, wantScores[i])
		}
	}
	wantTr := DualTrace{Entities: 1, LowDocs: 2, LowHits: 2, ExpansionDocs: 2, ExpansionHits: 2, FusedHits: 4, Theme: &ThemeTrace{CommunityID: 0, DocCount: 2}}
	if !reflect.DeepEqual(tr, wantTr) {
		t.Fatalf("trace = %+v，期望 %+v", tr, wantTr)
	}
}

func TestDualLevelSearch_ExpansionDecay(t *testing.T) {
	low, exp := baseFixture()
	hits, _, err := DualLevelSearch(context.Background(), "q", low, nil, exp, 4)
	if err != nil {
		t.Fatalf("DualLevelSearch() error = %v", err)
	}
	// 同 rank(1) 下扩展命中得分恰为低层的 0.7 倍（衰减可见于融合输出）。
	if !approxEq(hits[2].Score, 0.7*hits[0].Score) {
		t.Fatalf("扩展衰减不成立: %v vs 0.7*%v", hits[2].Score, hits[0].Score)
	}
}

func TestDualLevelSearch_ThemeWithSummarizer(t *testing.T) {
	low, exp := baseFixture()
	sum := &fakeSummarizer{out: "主题摘要"}
	_, tr, err := DualLevelSearch(context.Background(), "q", low, sum, exp, 4)
	if err != nil {
		t.Fatalf("DualLevelSearch() error = %v", err)
	}
	if tr.Theme == nil {
		t.Fatal("Theme 为 nil")
	}
	// 社区 0 = {d1,d4}（命中 2 条 > 社区 1 {d3} 的 1 条），摘要入参为
	// 该社区命中 chunk 按融合序的 ChunkID 列表。
	if tr.Theme.CommunityID != 0 || tr.Theme.DocCount != 2 {
		t.Fatalf("Theme = %+v", tr.Theme)
	}
	if tr.Theme.Summary != "主题摘要" {
		t.Fatalf("Summary = %q", tr.Theme.Summary)
	}
	if want := []string{"d1-c1", "d4-c1"}; !reflect.DeepEqual(sum.texts, want) {
		t.Fatalf("Summarizer 入参 = %v，期望 %v", sum.texts, want)
	}
}

func TestDualLevelSearch_SummarizerNilAndErrorDegrade(t *testing.T) {
	low, exp := baseFixture()
	// Summarizer=nil：Theme 正常产出、Summary 为空。
	_, tr, err := DualLevelSearch(context.Background(), "q", low, nil, exp, 4)
	if err != nil {
		t.Fatalf("DualLevelSearch() error = %v", err)
	}
	if tr.Theme == nil || tr.Theme.Summary != "" {
		t.Fatalf("nil summarizer Theme = %+v", tr.Theme)
	}
	// Summarizer 报错：降级为空摘要，整体不失败。
	sum := &fakeSummarizer{err: errors.New("llm down")}
	hits, tr2, err := DualLevelSearch(context.Background(), "q", low, sum, exp, 4)
	if err != nil {
		t.Fatalf("摘要失败不应使整体失败: %v", err)
	}
	if tr2.Theme == nil || tr2.Theme.Summary != "" {
		t.Fatalf("摘要失败 Theme = %+v", tr2.Theme)
	}
	if len(hits) != 4 {
		t.Fatalf("摘要失败影响排序: hits = %v", hits)
	}
}

func TestDualLevelSearch_TopKTruncation(t *testing.T) {
	low, exp := baseFixture()
	hits, tr, err := DualLevelSearch(context.Background(), "q", low, nil, exp, 2)
	if err != nil {
		t.Fatalf("DualLevelSearch() error = %v", err)
	}
	if len(hits) != 2 || hits[0].ChunkID != "d1-c1" || hits[1].ChunkID != "d2-c1" {
		t.Fatalf("截断结果 = %v", hits)
	}
	if tr.FusedHits != 2 {
		t.Fatalf("FusedHits = %d", tr.FusedHits)
	}
}

func TestDualLevelSearch_LowDocsDedupe(t *testing.T) {
	var rankedDocs []string
	low := fakeLow{
		findEntities: func(_ context.Context, _ string, _ int) ([]EntityRef, error) {
			return []EntityRef{{ID: "e2"}, {ID: "e1"}}, nil
		},
		docsOf: func(_ context.Context, _ []string) (map[string][]string, error) {
			return map[string][]string{"e1": {"d1", "d2"}, "e2": {"d2", "d3"}}, nil
		},
		rank: func(ctx context.Context, q string, docIDs []string, topK int) ([]ChunkHit, error) {
			rankedDocs = append([]string(nil), docIDs...)
			return oneHitPerDoc(ctx, q, docIDs, topK)
		},
	}
	_, tr, err := DualLevelSearch(context.Background(), "q", low, nil, &fakeExpander{}, 8)
	if err != nil {
		t.Fatalf("DualLevelSearch() error = %v", err)
	}
	// 按实体锚定序（e2 在前）去重：d2,d3,d1。
	if want := []string{"d2", "d3", "d1"}; !reflect.DeepEqual(rankedDocs, want) {
		t.Fatalf("候选文档 = %v，期望 %v", rankedDocs, want)
	}
	if tr.LowDocs != 3 {
		t.Fatalf("LowDocs = %d", tr.LowDocs)
	}
}

func TestDualLevelSearch_EmptySkipsDownstream(t *testing.T) {
	exp := &fakeExpander{}
	// 无实体锚点。
	low := fakeLow{}
	hits, tr, err := DualLevelSearch(context.Background(), "q", low, nil, exp, 4)
	if err != nil {
		t.Fatalf("DualLevelSearch() error = %v", err)
	}
	if len(hits) != 0 || tr != (DualTrace{}) {
		t.Fatalf("无实体应返回空与零值 trace: hits=%v trace=%+v", hits, tr)
	}
	if exp.calls != 0 {
		t.Fatalf("不应调用扩展端口: calls=%d", exp.calls)
	}
	// 有实体但无归属文档。
	low2 := fakeLow{
		findEntities: func(_ context.Context, _ string, _ int) ([]EntityRef, error) {
			return []EntityRef{{ID: "e1"}}, nil
		},
		docsOf: func(_ context.Context, _ []string) (map[string][]string, error) {
			return map[string][]string{"e1": {}}, nil
		},
	}
	hits, tr, err = DualLevelSearch(context.Background(), "q", low2, nil, exp, 4)
	if err != nil {
		t.Fatalf("DualLevelSearch() error = %v", err)
	}
	if len(hits) != 0 || tr.LowDocs != 0 || exp.calls != 0 {
		t.Fatalf("无候选文档应短路: hits=%v trace=%+v calls=%d", hits, tr, exp.calls)
	}
}

func TestDualLevelSearch_Errors(t *testing.T) {
	sentinel := errors.New("boom")
	// topK 校验。
	if _, _, err := DualLevelSearch(context.Background(), "q", fakeLow{}, nil, &fakeExpander{}, 0); err == nil {
		t.Fatal("topK<=0 应报错")
	}
	// nil 端口校验。
	if _, _, err := DualLevelSearch(context.Background(), "q", nil, nil, &fakeExpander{}, 4); err == nil {
		t.Fatal("low=nil 应报错")
	}
	// 实体锚定错误包装上抛。
	low := fakeLow{
		findEntities: func(_ context.Context, _ string, _ int) ([]EntityRef, error) {
			return nil, sentinel
		},
	}
	if _, _, err := DualLevelSearch(context.Background(), "q", low, nil, &fakeExpander{}, 4); !errors.Is(err, sentinel) {
		t.Fatalf("实体锚定错误未包装: %v", err)
	}
	// 扩展错误包装上抛。
	low2, _ := baseFixture()
	if _, _, err := DualLevelSearch(context.Background(), "q", low2, nil, &fakeExpander{err: sentinel}, 4); !errors.Is(err, sentinel) {
		t.Fatalf("扩展错误未包装: %v", err)
	}
	// 低层排序错误包装上抛。
	low3, _ := baseFixture()
	low3.rank = func(context.Context, string, []string, int) ([]ChunkHit, error) { return nil, sentinel }
	if _, _, err := DualLevelSearch(context.Background(), "q", low3, nil, &fakeExpander{}, 4); !errors.Is(err, sentinel) {
		t.Fatalf("排序错误未包装: %v", err)
	}
}
