package knowledge

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"aranea-agents/pkg/loggateway"
)

// ===== G-33 写侧（ADR-KN-20261020-H）：卡片源构造与对账计划 =====

func TestBuildEntityCardSource(t *testing.T) {
	src := BuildGraphCardSource("col", GraphCardEntitySource{
		EntityID: 7, Name: "Claude Code", EntityType: "AI Agent",
		Aliases: []string{"claude-code", "claude code cli", "claude-code"},
	})
	if src.Kind != GraphCardKindEntity || src.RefID != 7 || src.CollectionID != "col" {
		t.Fatalf("key fields wrong: %+v", src)
	}
	want := "Claude Code（AI Agent）\n别名：claude code cli, claude-code"
	if src.Text != want {
		t.Fatalf("text = %q want %q", src.Text, want)
	}
	if src.SourceHash != GraphCardSourceHash(want) {
		t.Fatalf("hash must equal GraphCardSourceHash(text)")
	}
}

func TestBuildEdgeCardSource(t *testing.T) {
	t.Run("subject+evidence split at first separator", func(t *testing.T) {
		src := BuildGraphCardSource("col", GraphCardEdgeSource{
			LinkID: 9, Relation: "depends-on",
			Context:     "Aranea — 旧系统 Aranea 1.0 — 依赖 MySQL",
			TargetTitle: "MySQL 部署手册",
		})
		// 首个 " — " 切分：主语快照不漂白，证据内 em-dash 原样保留。
		want := "Aranea depends-on MySQL 部署手册 — 旧系统 Aranea 1.0 — 依赖 MySQL"
		if src.Text != want {
			t.Fatalf("text = %q want %q", src.Text, want)
		}
		if src.Kind != GraphCardKindEdge || src.RefID != 9 {
			t.Fatalf("key fields wrong: %+v", src)
		}
	})

	t.Run("target title falls back to rel_path basename", func(t *testing.T) {
		src := BuildGraphCardSource("col", GraphCardEdgeSource{
			LinkID: 10, Relation: "applies-to",
			Context:       "灰度发布 — 先 5% 流量",
			TargetRelPath: "rollout/canary-guide.md",
		})
		want := "灰度发布 applies-to canary-guide — 先 5% 流量"
		if src.Text != want {
			t.Fatalf("text = %q want %q", src.Text, want)
		}
	})

	t.Run("context without separator becomes subject-less", func(t *testing.T) {
		// 无 " — " 时整段视为主语名（evidence 空——构造纪律与 relationEdgeContext
		// 空证据锐写对称），刻意不猜证据。
		src := BuildGraphCardSource("col", GraphCardEdgeSource{
			LinkID: 11, Relation: "is-a", Context: "PostgreSQL", TargetTitle: "关系数据库",
		})
		want := "PostgreSQL is-a 关系数据库"
		if src.Text != want {
			t.Fatalf("text = %q want %q", src.Text, want)
		}
	})

	t.Run("240 rune budget enforced at card build", func(t *testing.T) {
		src := BuildGraphCardSource("col", GraphCardEdgeSource{
			LinkID: 12, Relation: "causes",
			Context:     "长上下文 — " + strings.Repeat("证", 300),
			TargetTitle: "某故障",
		})
		if n := len([]rune(src.Text)); n > GraphCardTextMaxRunes+1 {
			t.Fatalf("card too long: %d runes", n)
		}
		if !strings.HasSuffix(src.Text, "…") {
			t.Fatalf("truncated card must carry ellipsis")
		}
	})
}

func TestPlanGraphCardReconcile(t *testing.T) {
	key := func(kind string, id int64) GraphCardKey { return GraphCardKey{Kind: kind, RefID: id} }
	mk := func(kind string, id int64, text string) GraphCardSource {
		return GraphCardSource{Kind: kind, RefID: id, CollectionID: "col", Text: text, SourceHash: GraphCardSourceHash(text)}
	}

	t.Run("three states: missing upsert / drift upsert / orphan delete", func(t *testing.T) {
		sources := []GraphCardSource{mk("entity", 1, "A"), mk("entity", 2, "B2"), mk("edge", 3, "C")}
		existing := []GraphCardRow{
			{Kind: "entity", RefID: 1, CollectionID: "col", Text: "A", SourceHash: GraphCardSourceHash("A")}, // 同名同文 → 不动
			{Kind: "entity", RefID: 2, CollectionID: "col", Text: "B", SourceHash: GraphCardSourceHash("B")}, // 漂移 → 重嵌
			{Kind: "entity", RefID: 4, CollectionID: "col", Text: "D", SourceHash: GraphCardSourceHash("D")}, // 孤儿 → 墓碑
		}
		upserts, deletes := PlanGraphCardReconcile(sources, existing, 100)
		if len(upserts) != 2 || upserts[0].RefID != 2 || upserts[1].RefID != 3 {
			t.Fatalf("upserts = %+v, want [entity/2 drift, edge/3 missing]", upserts)
		}
		if len(deletes) != 1 || deletes[0] != key("entity", 4) {
			t.Fatalf("deletes = %+v, want [entity/4]", deletes)
		}
	})

	t.Run("same ref_id distinct by kind", func(t *testing.T) {
		// 主键 (kind, ref_id)：entity/7 与 edge/7 是两行，互不遮蔽。
		sources := []GraphCardSource{mk("entity", 7, "E7")}
		existing := []GraphCardRow{{Kind: "edge", RefID: 7, CollectionID: "col", Text: "X", SourceHash: GraphCardSourceHash("X")}}
		upserts, deletes := PlanGraphCardReconcile(sources, existing, 100)
		if len(upserts) != 1 || len(deletes) != 1 {
			t.Fatalf("upserts=%+v deletes=%+v, want 各一", upserts, deletes)
		}
	})

	t.Run("budget: deletes first then upserts, deterministic order", func(t *testing.T) {
		sources := []GraphCardSource{mk("entity", 1, "A"), mk("entity", 2, "B"), mk("entity", 3, "C")}
		existing := []GraphCardRow{{Kind: "entity", RefID: 9, CollectionID: "col", Text: "X", SourceHash: GraphCardSourceHash("X")}}
		upserts, deletes := PlanGraphCardReconcile(sources, existing, 2)
		// 墓碑优先（召回卫生：闭合边/归并实体先退出召回），预算余 1 给首个 upsert。
		if len(deletes) != 1 || deletes[0] != key("entity", 9) {
			t.Fatalf("deletes = %+v", deletes)
		}
		if len(upserts) != 1 || upserts[0].RefID != 1 {
			t.Fatalf("upserts = %+v, want [entity/1]", upserts)
		}
	})
}

// ===== 工人编排（端口桩驱动） =====

type fakeGraphCardStore struct {
	collections  []GraphProjectionCollection
	entitiesBy   map[string][]GraphCardEntitySource
	edgesBy      map[string][]GraphCardEdgeSource
	existingBy   map[string][]GraphCardRow
	upserted     []GraphCardUpsert
	deleted      []GraphCardKey
	listCardsErr map[string]error
}

func (f *fakeGraphCardStore) ListGraphCardCollections(_ context.Context) ([]GraphProjectionCollection, error) {
	return f.collections, nil
}
func (f *fakeGraphCardStore) ListGraphCardEntitySources(_ context.Context, col string) ([]GraphCardEntitySource, error) {
	return f.entitiesBy[col], nil
}
func (f *fakeGraphCardStore) ListGraphCardEdgeSources(_ context.Context, col string) ([]GraphCardEdgeSource, error) {
	return f.edgesBy[col], nil
}
func (f *fakeGraphCardStore) ListGraphCards(_ context.Context, col string) ([]GraphCardRow, error) {
	if err := f.listCardsErr[col]; err != nil {
		return nil, err
	}
	return f.existingBy[col], nil
}
func (f *fakeGraphCardStore) UpsertGraphCards(_ context.Context, cards []GraphCardUpsert) error {
	f.upserted = append(f.upserted, cards...)
	return nil
}
func (f *fakeGraphCardStore) DeleteGraphCards(_ context.Context, keys []GraphCardKey) error {
	f.deleted = append(f.deleted, keys...)
	return nil
}

type fakeGraphCardEmbedder struct {
	batchSizes []int
	failAt     map[int]error // 第 n 批失败
	calls      int
}

func (f *fakeGraphCardEmbedder) EmbedGraphCardTexts(_ context.Context, texts []string) ([][]float32, error) {
	f.batchSizes = append(f.batchSizes, len(texts))
	if err := f.failAt[f.calls]; err != nil {
		f.calls++
		return nil, err
	}
	f.calls++
	out := make([][]float32, len(texts))
	for i := range out {
		out[i] = []float32{0.1, 0.2}
	}
	return out, nil
}

func TestGraphProjectionWorker_RunOnce(t *testing.T) {
	mkEntity := func(id int64, name string) GraphCardEntitySource {
		return GraphCardEntitySource{EntityID: id, Name: name}
	}

	t.Run("happy path: two collections, three states, embeddings stored", func(t *testing.T) {
		store := &fakeGraphCardStore{
			collections: []GraphProjectionCollection{{ID: "c1"}, {ID: "c2"}},
			entitiesBy: map[string][]GraphCardEntitySource{
				"c1": {mkEntity(1, "甲"), mkEntity(2, "乙")},
				"c2": {mkEntity(3, "丙")},
			},
			edgesBy: map[string][]GraphCardEdgeSource{},
			existingBy: map[string][]GraphCardRow{
				"c1": {{Kind: "entity", RefID: 1, CollectionID: "c1", Text: "甲", SourceHash: GraphCardSourceHash("甲")}},
				"c2": {{Kind: "entity", RefID: 8, CollectionID: "c2", Text: "孤儿", SourceHash: GraphCardSourceHash("孤儿")}},
			},
		}
		emb := &fakeGraphCardEmbedder{}
		w := NewGraphProjectionWorker(store, emb, testNoopLogger())
		stats, err := w.RunOnce(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if stats.Collections != 2 || stats.Upserted != 2 || stats.Deleted != 1 || stats.Failed != 0 {
			t.Fatalf("stats = %+v, want {2 2 1 0}", stats)
		}
		// c1：entity/1 同名不动、entity/2 缺卡 → 1 upsert；c2：entity/3 缺卡 → 1 upsert、
		// entity/8 孤儿 → 1 delete。嵌入批次 ≤32，维度随行。
		if len(store.upserted) != 2 {
			t.Fatalf("upserted = %+v", store.upserted)
		}
		for _, u := range store.upserted {
			if len(u.Embedding) != 2 || u.SourceHash != GraphCardSourceHash(u.Text) {
				t.Fatalf("upsert malformed: %+v", u)
			}
		}
		if len(store.deleted) != 1 || store.deleted[0].RefID != 8 {
			t.Fatalf("deleted = %+v", store.deleted)
		}
	})

	t.Run("embed batch failure drops only that batch, round continues", func(t *testing.T) {
		store := &fakeGraphCardStore{
			collections: []GraphProjectionCollection{{ID: "c1"}},
			entitiesBy: map[string][]GraphCardEntitySource{
				"c1": func() []GraphCardEntitySource {
					out := make([]GraphCardEntitySource, 0, GraphProjectionEmbedBatch+1)
					for i := 0; i < GraphProjectionEmbedBatch+1; i++ {
						out = append(out, mkEntity(int64(i+1), fmt.Sprintf("实体%d", i+1)))
					}
					return out
				}(),
			},
			edgesBy:    map[string][]GraphCardEdgeSource{},
			existingBy: map[string][]GraphCardRow{},
		}
		emb := &fakeGraphCardEmbedder{failAt: map[int]error{0: errors.New("embed boom")}}
		w := NewGraphProjectionWorker(store, emb, testNoopLogger())
		stats, err := w.RunOnce(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		// 第一批 32 全失败丢弃、第二批 1 成功：失败不阻断整轮。
		if stats.Upserted != 1 || stats.Failed != GraphProjectionEmbedBatch {
			t.Fatalf("stats = %+v, want Upserted=1 Failed=%d", stats, GraphProjectionEmbedBatch)
		}
		if len(store.upserted) != 1 {
			t.Fatalf("upserted = %+v", store.upserted)
		}
	})

	t.Run("per-collection store error isolated", func(t *testing.T) {
		store := &fakeGraphCardStore{
			collections:  []GraphProjectionCollection{{ID: "bad"}, {ID: "c2"}},
			entitiesBy:   map[string][]GraphCardEntitySource{"c2": {mkEntity(3, "丙")}},
			edgesBy:      map[string][]GraphCardEdgeSource{},
			existingBy:   map[string][]GraphCardRow{},
			listCardsErr: map[string]error{"bad": errors.New("pg down")},
		}
		w := NewGraphProjectionWorker(store, &fakeGraphCardEmbedder{}, testNoopLogger())
		stats, err := w.RunOnce(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if stats.Collections != 1 || stats.Failed != 1 || stats.Upserted != 1 {
			t.Fatalf("stats = %+v, want Collections=1 Failed=1 Upserted=1", stats)
		}
	})

	t.Run("global round budget caps across collections", func(t *testing.T) {
		big := make([]GraphCardEntitySource, 0, GraphProjectionRoundBudget+10)
		for i := 0; i < GraphProjectionRoundBudget+10; i++ {
			big = append(big, mkEntity(int64(i+1), fmt.Sprintf("实体%d", i+1)))
		}
		store := &fakeGraphCardStore{
			collections: []GraphProjectionCollection{{ID: "c1"}, {ID: "c2"}},
			entitiesBy:  map[string][]GraphCardEntitySource{"c1": big[:GraphProjectionRoundBudget], "c2": big[GraphProjectionRoundBudget:]},
			edgesBy:     map[string][]GraphCardEdgeSource{},
			existingBy:  map[string][]GraphCardRow{},
		}
		w := NewGraphProjectionWorker(store, &fakeGraphCardEmbedder{}, testNoopLogger())
		stats, err := w.RunOnce(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if stats.Upserted != GraphProjectionRoundBudget {
			t.Fatalf("Upserted = %d, want 预算闸 %d（跨集合累计，c2 全部下轮）", stats.Upserted, GraphProjectionRoundBudget)
		}
	})

	t.Run("nil embedder is a wiring error", func(t *testing.T) {
		w := NewGraphProjectionWorker(&fakeGraphCardStore{collections: nil}, nil, testNoopLogger())
		if _, err := w.RunOnce(context.Background()); err == nil {
			t.Fatal("nil embedder must error loudly（装配缺陷与装配关腿不同语义）")
		}
	})
}

func testNoopLogger() loggateway.Logger { return loggateway.NewNoop() }
