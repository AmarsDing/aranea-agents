package knowledge

// ── G5-F B11：ListEntityMergeSuggestions（norm 冲突组 + embedding 高相似对） ──

import (
	"context"
	"errors"
	"testing"
)

// fakeEntityGovRepo EntityRepo 最小实现（仅 ListEntities 有数据；其余方法 stub）。
type fakeEntityGovRepo struct {
	entities []Entity
	err      error
}

func (f *fakeEntityGovRepo) ReplaceDocEntities(context.Context, string, string, []DocEntity) ([]int64, error) {
	return nil, nil
}
func (f *fakeEntityGovRepo) FindEntityCooccurrences(context.Context, string, []int64, string, int) ([]EntityCooccurrence, error) {
	return nil, nil
}
func (f *fakeEntityGovRepo) MergeEntities(context.Context, string, int64, []int64) (EntityMergeResult, error) {
	return EntityMergeResult{}, nil
}
func (f *fakeEntityGovRepo) ListEntities(context.Context, string) ([]Entity, error) {
	return f.entities, f.err
}

type fakeEntityEmbedder struct {
	vecs  [][]float32
	err   error
	calls int
}

func (f *fakeEntityEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	if len(texts) != len(f.vecs) {
		return nil, errors.New("vec/text count mismatch")
	}
	return f.vecs, nil
}

func newSuggestUsecase(repo EntityRepo) *Usecase {
	u := NewUsecaseFromRepo(nil)
	u.SetLinkRepos(nil, repo)
	return u
}

func TestListEntityMergeSuggestions_NormConflictGroups(t *testing.T) {
	repo := &fakeEntityGovRepo{entities: []Entity{
		{ID: 1, Name: "AI", NameNorm: "ai"},
		{ID: 2, Name: "ai", NameNorm: "ai"},
		{ID: 3, Name: "ＡＩ", NameNorm: "ai"},
		{ID: 4, Name: "RAG", NameNorm: "rag"},
	}}
	u := newSuggestUsecase(repo)

	got, err := u.ListEntityMergeSuggestions(context.Background(), "c1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("suggestions = %v, want 2（冲突组 3 成员 → keeper + 2 对）", got)
	}
	for _, s := range got {
		if s.KeeperID != 1 || s.KeeperName != "AI" {
			t.Errorf("keeper = (%d,%q), want (1,AI)（id 最小者）", s.KeeperID, s.KeeperName)
		}
		if s.Source != EntityMergeSourceNorm || s.Tier != EntityMergeTierAuto || s.Similarity != 1.0 {
			t.Errorf("suggestion = %+v, want source=norm tier=auto sim=1.0", s)
		}
	}
	if got[0].MergeeID != 2 || got[1].MergeeID != 3 {
		t.Errorf("mergees = (%d,%d), want (2,3)（id 序）", got[0].MergeeID, got[1].MergeeID)
	}
}

func TestListEntityMergeSuggestions_EmbeddingPairs(t *testing.T) {
	repo := &fakeEntityGovRepo{entities: []Entity{
		{ID: 1, Name: "机器学习", NameNorm: "机器学习"},
		{ID: 2, Name: "机械学习", NameNorm: "机械学习"},
		{ID: 3, Name: "财报", NameNorm: "财报"},
		{ID: 4, Name: "季度财报", NameNorm: "季度财报"},
	}}
	// 1↔2 极相似（auto）；3↔4 中等相似（suggest）；其余正交。
	emb := &fakeEntityEmbedder{vecs: [][]float32{
		{1, 0, 0},
		{0.99, 0.01, 0},
		{0, 1, 0},
		{0, 0.85, 0.527},
	}}
	u := newSuggestUsecase(repo)

	got, err := u.ListEntityMergeSuggestions(context.Background(), "c1", emb)
	if err != nil {
		t.Fatal(err)
	}
	if emb.calls != 1 {
		t.Fatalf("embed calls = %d, want 1（单批）", emb.calls)
	}
	if len(got) != 2 {
		t.Fatalf("suggestions = %v, want 2（auto + suggest 各一）", got)
	}
	// 相似度降序：auto 在前。
	auto, suggest := got[0], got[1]
	if auto.KeeperID != 1 || auto.MergeeID != 2 || auto.Source != EntityMergeSourceEmbedding || auto.Tier != EntityMergeTierAuto {
		t.Errorf("auto = %+v, want 1←2 embedding/auto", auto)
	}
	if suggest.KeeperID != 3 || suggest.MergeeID != 4 || suggest.Tier != EntityMergeTierSuggest {
		t.Errorf("suggest = %+v, want 3←4 suggest", suggest)
	}
	if suggest.Similarity < 0.80 || suggest.Similarity >= 0.90 {
		t.Errorf("suggest similarity = %v, want [0.80,0.90)", suggest.Similarity)
	}
}

// 未配置 embedding：仅 norm 组（NFR-15）；embedder 失败同样降级 norm-only。
func TestListEntityMergeSuggestions_EmbedderDegrades(t *testing.T) {
	repo := &fakeEntityGovRepo{entities: []Entity{
		{ID: 1, Name: "AI", NameNorm: "ai"},
		{ID: 2, Name: "ai", NameNorm: "ai"},
		{ID: 3, Name: "RAG", NameNorm: "rag"},
	}}
	u := newSuggestUsecase(repo)

	// nil embedder。
	got, err := u.ListEntityMergeSuggestions(context.Background(), "c1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Source != EntityMergeSourceNorm {
		t.Fatalf("nil embedder suggestions = %v, want 1 norm", got)
	}

	// embedder 报错 → norm-only 不报错。
	got, err = u.ListEntityMergeSuggestions(context.Background(), "c1", &fakeEntityEmbedder{err: errors.New("no provider")})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Source != EntityMergeSourceNorm {
		t.Fatalf("failing embedder suggestions = %v, want 1 norm", got)
	}
}

// 未接线 EntityRepo：降级空（对齐 ListDocumentLinks 语义）。
func TestListEntityMergeSuggestions_UnwiredDegrades(t *testing.T) {
	u := NewUsecaseFromRepo(nil)
	got, err := u.ListEntityMergeSuggestions(context.Background(), "c1", nil)
	if err != nil || len(got) != 0 {
		t.Fatalf("unwired: got=%v err=%v, want empty,nil", got, err)
	}
}
