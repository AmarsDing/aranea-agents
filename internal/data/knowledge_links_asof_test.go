package data

import (
	"context"
	"reflect"
	"testing"
	"time"

	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// ── G-05-asof（ADR-KN-20261020-J）：双时态读端口四象限矩阵 ─────────────────
//
// 语义矩阵（生产可达状态推导）：
//
//	zero-span   → current 快报（原函数字节级委托：active 走 valid_to IS NULL 原文本
//	              含 co_activated；collection 不做 valid_to 过滤——前端现状红线）
//	T-only      → tstzrange(valid_from, COALESCE(valid_to,'infinity')) @> $T，排 co_activated
//	R-only      → recorded_at <= $R AND (valid_to IS NULL OR valid_to > $R)，排 co_activated
//	    （valid_to 恒等于关闭时刻 NOW() 兼记录端；非 co_activated 行 valid_to 恰升迁一次）
//	T∧R 双轴    → recorded_at <= $R AND ( @>T OR (valid_from <= $T AND valid_to > $R) )
//	    （R 晚于关边记录=已知窗口夹 T 仍回放；R 早于关边记录=当时仍信开）
//
// 失真契约（钉死不可见/不可回放）：
//   - co_activated 永不进 as-of 分支（原位复活抹除衰减关闭区间）；
//   - closed-on-arrival 空区间（valid_from==valid_to）任何象限不可见；
//   - 属性（weight_f/context）返回当前值，不作时态断言。
//
// 场景行（全为生产可达形态；测试直写 timestamp 列摆位）：
//
//	A explicit    d1→d2 rA  [T0,∞)  rec=T0          —— 恒开
//	B explicit    d1→d3 rB  [T0,T2) rec=T0          —— T2 关闭
//	C semantic    d1→d4 rC  [T2,∞)  rec=T2          —— T2 才开且晚记录
//	D co_activated d2→d3    [T0,∞)  rec=T0          —— 仅 current 可见
//	E semantic    d2→d4 rE  [T1,T1) rec=T1 空区间   —— 任何象限不可见
//
// 查询端点集 {d1,d2}。T0=now-4h, T1=now-2h, T2=now-1h, R0=T0+30m, Rmid=T2+30m。

type asofRow struct{ doc, target, linkType, relation string }

func (r asofRow) key() string { return r.doc + ">" + r.target + "|" + r.linkType + "|" + r.relation }

// linkTriple 用于 collection 读断言（Link 投影无 relation 列，三字段组合在场景内唯一）。
func linkTriple(doc, target, linkType string) string { return doc + ">" + target + "|" + linkType }

func seedAsOfEdges(t *testing.T, repo *knowledgeRepo) (T0, T1, T2, R0, Rmid time.Time) {
	t.Helper()
	ctx := context.Background()
	if _, err := repo.data.Postgres().ExecContext(ctx,
		`INSERT INTO knowledge_collections (id, name, embedding_model) VALUES ('c1','c1','m')`); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"d1", "d2", "d3", "d4"} {
		if _, err := repo.data.Postgres().ExecContext(ctx,
			`INSERT INTO knowledge_documents (id, collection_id, source) VALUES ($1,'c1','s')`, id); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	T0, T1, T2 = now.Add(-4*time.Hour), now.Add(-2*time.Hour), now.Add(-1*time.Hour)
	R0, Rmid = T0.Add(30*time.Minute), T2.Add(30*time.Minute)
	rows := []struct {
		doc, target, lt, rel string
		from                 time.Time
		to                   *time.Time
		rec                  time.Time
	}{
		{"d1", "d2", "explicit", "rA", T0, nil, T0},
		{"d1", "d3", "explicit", "rB", T0, &T2, T0},
		{"d1", "d4", "semantic", "rC", T2, nil, T2},
		{"d2", "d3", "co_activated", "", T0, nil, T0},
		{"d2", "d4", "semantic", "rE", T1, &T1, T1},
	}
	for _, r := range rows {
		if _, err := repo.data.Postgres().ExecContext(ctx,
			`INSERT INTO knowledge_links (collection_id, doc_id, target_doc_id, link_type, relation, weight, weight_f, valid_from, valid_to, recorded_at)
			 VALUES ('c1',$1,$2,$3,$4,1,1.0,$5,$6,$7)`,
			r.doc, r.target, r.lt, r.rel, r.from, r.to, r.rec); err != nil {
			t.Fatalf("seed %v: %v", r.rel, err)
		}
	}
	return
}

func activeKeys(links []bizknowledge.ActiveLink) map[string]bool {
	out := map[string]bool{}
	for _, l := range links {
		out[(asofRow{l.DocID, l.TargetDocID, l.LinkType, l.Relation}).key()] = true
	}
	return out
}

func activeOrdered(got []bizknowledge.ActiveLink) []string {
	out := make([]string, 0, len(got))
	for _, l := range got {
		out = append(out, (asofRow{l.DocID, l.TargetDocID, l.LinkType, l.Relation}).key())
	}
	return out
}

func collectionOrdered(got []bizknowledge.Link) []string {
	out := make([]string, 0, len(got))
	for _, l := range got {
		out = append(out, linkTriple(l.DocID, l.TargetDocID, l.LinkType))
	}
	return out
}

func wantKeys(t *testing.T, got map[string]bool, want []asofRow) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("rows = %v, want keys of %v", got, want)
	}
	for _, w := range want {
		if !got[w.key()] {
			t.Fatalf("missing %v in %v", w, got)
		}
	}
}

func TestTemporalActiveLinkReader_FourQuadrantMatrix(t *testing.T) {
	repo := setupKnowledgeSearchRepo(t)
	ctx := context.Background()
	_, T1, T2, R0, Rmid := seedAsOfEdges(t, repo)

	reader, ok := interface{}(repo).(bizknowledge.TemporalActiveLinkReader)
	if !ok {
		t.Fatal("knowledgeRepo does not implement bizknowledge.TemporalActiveLinkReader")
	}
	docIDs := []string{"d1", "d2"}
	A := asofRow{"d1", "d2", "explicit", "rA"}
	B := asofRow{"d1", "d3", "explicit", "rB"}
	C := asofRow{"d1", "d4", "semantic", "rC"}
	D := asofRow{"d2", "d3", "co_activated", ""}

	matrix := []struct {
		name string
		span bizknowledge.TemporalSpan
		want []asofRow
	}{
		{"current(zero span,含 co_activated)", bizknowledge.TemporalSpan{}, []asofRow{A, C, D}},
		{"T=T1(彼时有效:B 未关/C 未开/E 空区间不可见)", bizknowledge.TemporalSpan{ValidAt: T1}, []asofRow{A, B}},
		{"T=T2(上界排他:B 窗口除名/C 窗口含端)", bizknowledge.TemporalSpan{ValidAt: T2}, []asofRow{A, C}},
		{"R=R0(R 时所信现行图:B 仍信开/C 尚未知)", bizknowledge.TemporalSpan{RecordedAt: R0}, []asofRow{A, B}},
		{"R=Rmid(B 已知关/C 已知开)", bizknowledge.TemporalSpan{RecordedAt: Rmid}, []asofRow{A, C}},
		{"dual(T1,R0)(T 有效 ∧ R 时信开)", bizknowledge.TemporalSpan{ValidAt: T1, RecordedAt: R0}, []asofRow{A, B}},
		{"dual(T1,Rmid)(追溯后知:B 窗口含 T1)", bizknowledge.TemporalSpan{ValidAt: T1, RecordedAt: Rmid}, []asofRow{A, B}},
		{"dual(T2,Rmid)(关边上界排他:B 死区)", bizknowledge.TemporalSpan{ValidAt: T2, RecordedAt: Rmid}, []asofRow{A, C}},
	}
	for _, tc := range matrix {
		got, err := reader.ListActiveLinksAsOf(ctx, "c1", docIDs, tc.span)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		wantKeys(t, activeKeys(got), tc.want)
	}
}

func TestTemporalCollectionLinkReader_MatrixAndOrder(t *testing.T) {
	repo := setupKnowledgeSearchRepo(t)
	ctx := context.Background()
	_, T1, T2, _, Rmid := seedAsOfEdges(t, repo)

	reader, ok := interface{}(repo).(bizknowledge.TemporalCollectionLinkReader)
	if !ok {
		t.Fatal("knowledgeRepo does not implement bizknowledge.TemporalCollectionLinkReader")
	}
	A := linkTriple("d1", "d2", "explicit")
	B := linkTriple("d1", "d3", "explicit")
	C := linkTriple("d1", "d4", "semantic")
	D := linkTriple("d2", "d3", "co_activated")
	E := linkTriple("d2", "d4", "semantic")

	assertOrdered := func(name string, span bizknowledge.TemporalSpan, linkTypes []string, want []string) {
		t.Helper()
		got, err := reader.ListCollectionLinksAsOf(ctx, "c1", linkTypes, span)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !reflect.DeepEqual(collectionOrdered(got), want) {
			t.Fatalf("%s: rows = %v, want %v（ORDER BY id 稳序钉）", name, collectionOrdered(got), want)
		}
	}

	// zero span → 字节级委托 legacy：不做 valid_to 过滤（前端现状红线），
	// closed B、空区间 E、co_activated D 全部照常返回，id 升序=A..E。
	assertOrdered("zero span 全量 legacy 语义", bizknowledge.TemporalSpan{}, nil, []string{A, B, C, D, E})
	// T=T1 全类型：B 未关/C 未开/E 空区间不可见/D 排除。
	assertOrdered("T=T1 全类型", bizknowledge.TemporalSpan{ValidAt: T1}, nil, []string{A, B})
	// linkTypes 过滤与 as-of 谓词叠加。
	assertOrdered("T=T1 semantic-only（C 未开/E 空区间）", bizknowledge.TemporalSpan{ValidAt: T1}, []string{"semantic"}, []string{})
	assertOrdered("T=T1 explicit-only", bizknowledge.TemporalSpan{ValidAt: T1}, []string{"explicit"}, []string{A, B})
	// T=T2：B 上界排他除名、C 下界含端。
	assertOrdered("T=T2 全类型", bizknowledge.TemporalSpan{ValidAt: T2}, nil, []string{A, C})
	// R=Rmid：B 已知关/C 已知开。
	assertOrdered("R=Rmid 全类型", bizknowledge.TemporalSpan{RecordedAt: Rmid}, nil, []string{A, C})
	// 双轴：B 窗口含 T1（追溯后知仍回放）。
	assertOrdered("dual(T1,Rmid) 全类型", bizknowledge.TemporalSpan{ValidAt: T1, RecordedAt: Rmid}, nil, []string{A, B})
}

// TestReplaceLinks_RecordedAtImmutable 回归钉（ADR-J R 轴语义成立前提）：
// 任何生产写路径（原位更新/关闭+插新）都不得改写已存在行的 recorded_at——
// 一旦动摇，R 轴回放即失真。
func TestReplaceLinks_RecordedAtImmutable(t *testing.T) {
	repo := setupKnowledgeSearchRepo(t)
	ctx := context.Background()
	if _, err := repo.data.Postgres().ExecContext(ctx,
		`INSERT INTO knowledge_collections (id, name, embedding_model) VALUES ('c1','c1','m')`); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.data.Postgres().ExecContext(ctx,
		`INSERT INTO knowledge_documents (id, collection_id, source) VALUES ('d1','c1','s'),('d2','c1','s')`); err != nil {
		t.Fatal(err)
	}
	if err := repo.ReplaceLinks(ctx, "c1", "d1", "semantic", []bizknowledge.Link{
		{TargetDocID: "d2", Context: "v1", Weight: 1},
	}); err != nil {
		t.Fatal(err)
	}
	var rec1 time.Time
	if err := repo.data.Postgres().QueryRowContext(ctx,
		`SELECT recorded_at FROM knowledge_links WHERE doc_id='d1' AND target_doc_id='d2' AND valid_to IS NULL`).Scan(&rec1); err != nil {
		t.Fatal(err)
	}
	// 同边原位 UPDATE（context/weight 变化）：recorded_at 不得动。
	if err := repo.ReplaceLinks(ctx, "c1", "d1", "semantic", []bizknowledge.Link{
		{TargetDocID: "d2", Context: "v2", Weight: 2},
	}); err != nil {
		t.Fatal(err)
	}
	var rec2 time.Time
	if err := repo.data.Postgres().QueryRowContext(ctx,
		`SELECT recorded_at FROM knowledge_links WHERE doc_id='d1' AND target_doc_id='d2' AND valid_to IS NULL`).Scan(&rec2); err != nil {
		t.Fatal(err)
	}
	if !rec1.Equal(rec2) {
		t.Fatalf("recorded_at mutated by in-place update: %v → %v", rec1, rec2)
	}
	// 关边（valid_to 变迁）：recorded_at 同样不得动——valid_to 兼记录端语义依赖此。
	if err := repo.ReplaceLinks(ctx, "c1", "d1", "semantic", nil); err != nil {
		t.Fatal(err)
	}
	var rec3 time.Time
	var vt *time.Time
	if err := repo.data.Postgres().QueryRowContext(ctx,
		`SELECT recorded_at, valid_to FROM knowledge_links WHERE doc_id='d1' AND target_doc_id='d2'`).Scan(&rec3, &vt); err != nil {
		t.Fatal(err)
	}
	if !rec1.Equal(rec3) || vt == nil {
		t.Fatalf("close must set valid_to only: rec %v→%v, valid_to=%v", rec1, rec3, vt)
	}
}

// 迁移 20261287（索引维度复审 F1）：recorded_at 组合索引落位 + 幂等重跑。
func TestMigration20261287_RecordIndex(t *testing.T) {
	db := testhelper.SetupTestPGRaw(t)
	ctx := context.Background()
	// 存量最小形态：links 带双时态列但无 R 轴索引（20261220 后、20261287 前）。
	legacy := []string{
		`CREATE TABLE knowledge_collections (
			id TEXT PRIMARY KEY, name TEXT NOT NULL, embedding_model TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE knowledge_documents (
			id TEXT PRIMARY KEY,
			collection_id TEXT NOT NULL REFERENCES knowledge_collections(id) ON DELETE CASCADE,
			source TEXT NOT NULL)`,
		`CREATE TABLE knowledge_links (
			id BIGSERIAL PRIMARY KEY,
			collection_id TEXT NOT NULL REFERENCES knowledge_collections(id) ON DELETE CASCADE,
			doc_id TEXT NOT NULL REFERENCES knowledge_documents(id) ON DELETE CASCADE,
			target_doc_id TEXT NOT NULL REFERENCES knowledge_documents(id) ON DELETE CASCADE,
			link_type TEXT NOT NULL, valid_from TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			valid_to TIMESTAMPTZ, recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`,
	}
	for _, stmt := range legacy {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("legacy shape: %v", err)
		}
	}
	run := func() {
		if err := executeSQLFileWithDialect(ctx, db,
			"sql/migrations/20261287_knowledge_links_recorded_idx.sql", DialectPostgres, loggateway.NewNoop()); err != nil {
			t.Fatalf("migration: %v", err)
		}
	}
	run()
	var cnt int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pg_indexes WHERE indexname = 'knowledge_links_recorded_idx'`).Scan(&cnt); err != nil {
		t.Fatal(err)
	}
	if cnt != 1 {
		t.Fatalf("knowledge_links_recorded_idx missing after migration")
	}
	run() // 幂等重跑（DB-N6）
}
