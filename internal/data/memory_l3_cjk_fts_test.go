package data

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// ──────────────────────────────────────────────────────────
// AMC C3-B: trgm scope 先行（2026-10-20 事故治本）+ CJK bigram FTS 通道
// （migration 20261292）。全部跑 testhelper 真 PG + 生产 migration 文件。
// ──────────────────────────────────────────────────────────

const cjkFTSMigrationFile = "sql/migrations/20261292_memory_facts_cjk_bigram_fts.sql"

// TestBigramTextExtraction_PG asserts bigram_text() tokenization on a real
// Postgres (migration file executed twice → the ALTER/INDEX statements skip
// on the missing memory_facts table via 42P01 tolerance, and idempotency of
// CREATE OR REPLACE FUNCTION is proven).
func TestBigramTextExtraction_PG(t *testing.T) {
	ctx := context.Background()
	db := testhelper.SetupTestPGRaw(t)
	for round := 0; round < 2; round++ {
		if err := executeSQLFileWithDialect(ctx, db, cjkFTSMigrationFile, DialectPostgres, loggateway.NewNoop()); err != nil {
			t.Fatalf("round %d apply %s: %v", round, cjkFTSMigrationFile, err)
		}
	}
	cases := []struct{ in, want string }{
		{"喜欢蓝色", "喜欢 欢蓝 蓝色"},
		{"Alice 喜欢蓝色", "alice 喜欢 欢蓝 蓝色"},
		{"SRV-DB-03 已迁移", "srv db 03 已迁 迁移"},
		{"中", "中"}, // 单 CJK 字符段保留原字（无法成二元组）
		{"云服务器alice喜欢茶", "云服 服务 务器 alice 喜欢 欢茶"},
		{"hello world", "hello world"},
		{"", ""},
	}
	for _, c := range cases {
		var got string
		if err := db.QueryRowContext(ctx, `SELECT bigram_text($1)`, c.in).Scan(&got); err != nil {
			t.Fatalf("bigram_text(%q): %v", c.in, err)
		}
		if got != c.want {
			t.Errorf("bigram_text(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	var gotNull *string
	if err := db.QueryRowContext(ctx, `SELECT bigram_text(NULL)`).Scan(&gotNull); err != nil {
		t.Fatalf("bigram_text(NULL): %v", err)
	}
	if gotNull != nil {
		t.Errorf("bigram_text(NULL) = %q, want NULL (STRICT)", *gotNull)
	}
}

// setupL3CJKFTSTestRepo builds the isolated PG repo (memory_facts via
// EnsureSessionMemorySchema) and applies the 20261292 migration twice
// (idempotency proof), enabling the bigram_text() functional GIN index
// (AMC C3 修复 #9: 不再建 STORED 生成列，索引表达式直接挂
// to_tsvector('simple', bigram_text(statement)) on production DDL shape.
func setupL3CJKFTSTestRepo(t *testing.T) *l3FactRepo {
	t.Helper()
	r := setupL3FTSTestRepo(t, nil, 0)
	db := r.data.rawDB
	if db == nil {
		t.Fatal("rawDB is nil")
	}
	for round := 0; round < 2; round++ {
		if err := executeSQLFileWithDialect(context.Background(), db, cjkFTSMigrationFile, DialectPostgres, loggateway.NewNoop()); err != nil {
			t.Fatalf("round %d apply %s: %v", round, cjkFTSMigrationFile, err)
		}
	}
	// 函数式索引存在且索引定义挂在 bigram_text 表达式上（#9 后无生成列）。
	var idxDef string
	if err := db.QueryRowContext(context.Background(),
		`SELECT indexdef FROM pg_indexes
		 WHERE tablename='memory_facts' AND indexname='idx_memory_facts_fts_cjk'`).Scan(&idxDef); err != nil {
		t.Fatalf("check idx_memory_facts_fts_cjk: %v", err)
	}
	if !strings.Contains(idxDef, "bigram_text") {
		t.Fatalf("idx_memory_facts_fts_cjk is not the bigram_text functional index: %s", idxDef)
	}
	return r
}

// TestSearchL3FTSCJK_ChineseQuery_PG: a CJK question must召回 only-partially-
// overlapping 中文事实 via the bigram channel, merge into ftsCandidateIDs
// without weakening the English list, and respect scope isolation.
func TestSearchL3FTSCJK_ChineseQuery_PG(t *testing.T) {
	r := setupL3CJKFTSTestRepo(t)
	ctx := context.Background()
	scope := "agent-cjk-fts"
	aliceID := upsertL3Fact(t, r, scope, "Alice 喜欢蓝色", 0.5)
	bobID := upsertL3Fact(t, r, scope, "Bob 喜欢咖啡", 0.5)
	codeID := upsertL3Fact(t, r, scope, "套餐 ZXQ4817 包含甜点", 0.5)

	ids, err := r.searchL3FTSCJK(ctx, "agent", scope, "", "喜欢蓝色吗", 10)
	if err != nil {
		t.Fatalf("searchL3FTSCJK: %v", err)
	}
	// 「喜欢蓝色吗」bigrams = 喜欢/欢蓝/蓝色/色吗；Alice 命中 3 个、Bob 仅
	// 命中 喜欢 一个 → Alice 必须排第一；「套餐」事实零共享二元组不得入选。
	if len(ids) == 0 || ids[0] != aliceID {
		t.Fatalf("ids=%v, want [%s ...] first", ids, aliceID)
	}
	idSet := map[string]struct{}{}
	for _, id := range ids {
		idSet[id] = struct{}{}
	}
	if _, ok := idSet[aliceID]; !ok {
		t.Fatalf("CJK-matched fact %s missing, ids=%v", aliceID, ids)
	}
	if _, ok := idSet[bobID]; !ok {
		t.Fatalf("shared-bigram fact %s should be recalled, ids=%v", bobID, ids)
	}
	if _, ok := idSet[codeID]; ok {
		t.Fatalf("zero-overlap fact %s must not enter CJK candidates, ids=%v", codeID, ids)
	}

	// ftsCandidateIDs 合并：CJK 候选并入，且不产生重复。
	merged := r.ftsCandidateIDs(ctx, "agent", scope, "", "喜欢蓝色吗", 10)
	seen := map[string]int{}
	for _, id := range merged {
		seen[id]++
	}
	if seen[aliceID] == 0 {
		t.Fatalf("merged ftsCandidateIDs missing CJK hit %s: %v", aliceID, merged)
	}
	for id, n := range seen {
		if n > 1 {
			t.Fatalf("merged ftsCandidateIDs duplicated %s: %v", id, merged)
		}
	}

	// Scope 隔离：同语句在另一 scope 出现不得泄漏。
	upsertL3Fact(t, r, "agent-cjk-other", "Alice 喜欢蓝色", 0.5)
	ids, err = r.searchL3FTSCJK(ctx, "agent", scope, "", "喜欢蓝色吗", 10)
	if err != nil {
		t.Fatalf("searchL3FTSCJK scoped: %v", err)
	}
	for _, id := range ids {
		if id != aliceID && id != bobID {
			t.Fatalf("scope leak: ids=%v, want only facts of %s", ids, scope)
		}
	}

	// 纯 ASCII 查询不进 CJK 通道（英文 FTS 行为不变，见 TestSearchL3FTS_*）。
	ids, err = r.searchL3FTSCJK(ctx, "agent", scope, "", "ZXQ4817", 10)
	if err != nil || ids != nil {
		t.Fatalf("pure-ASCII query: ids=%v err=%v, want nil/nil", ids, err)
	}
}

// TestSearchL3Trigram_ScopeFirstUserIsolation_PG is the 2026-10-20 事故治本
// regression: with 5000 noise rows belonging to other users in the table,
// the scope-first trigram query must still hit only the caller's facts and
// return the correct row (the scoped CTE bounds %> / word_similarity to the
// newest 2000 in-scope rows instead of a whole-table GIN candidate scan).
func TestSearchL3Trigram_ScopeFirstUserIsolation_PG(t *testing.T) {
	r := setupL3FTSTestRepo(t, nil, 0)
	enableL3Trigram(t, r)
	ctx := context.Background()
	db := r.data.rawDB

	// 5000 条异用户噪声（直插 raw SQL 控制成本），其中一条故意含目标中文串，
	// 用以断言 user_id/scope 隔离边界——它绝不可进入本用户的候选。
	const noise = 5000
	for i := 0; i < noise; i++ {
		stmt := fmt.Sprintf("noise fact %d about server maintenance windows", i)
		if i == 0 {
			stmt = "noise-user-0 喜欢蓝色" // 隔离探针
		}
		if _, err := db.ExecContext(ctx, `
			INSERT INTO memory_facts (id, scope_type, scope_id, user_id, agent_id, statement, fingerprint, created_at, updated_at)
			VALUES ($1, 'user', $2, $2, $2, $3, $4, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
			fmt.Sprintf("noise-%d", i), fmt.Sprintf("noise-user-%d", i), stmt, fmt.Sprintf("fp-noise-%d", i)); err != nil {
			t.Fatalf("insert noise %d: %v", i, err)
		}
	}
	// 目标用户事实（早于噪声的 created_at——若退化为全表 GIN 扫描后排序，
	// 或 scope 先行失效，5000 条新噪声必然淹没/泄入结果）。
	raw, err := r.UpsertFactRow(ctx, biz.FactUpsert{
		ScopeType: "user", ScopeID: "alice", UserID: "alice", AgentID: "alice",
		Statement: "Alice 喜欢蓝色", FactKind: "preference", Importance: 0.7,
		Confidence: 0.9, SourceKind: "manual", CreatedAt: "2025-12-31T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("upsert target: %v", err)
	}
	targetID := factRowID(t, raw)

	ids, err := r.searchL3Trigram(ctx, "user", "alice", "alice", "喜欢蓝色", 10)
	if err != nil {
		t.Fatalf("searchL3Trigram: %v", err)
	}
	if len(ids) != 1 || ids[0] != targetID {
		t.Fatalf("ids=%v (len %d), want exactly [%s] — scope isolation breached or 2000-row window starved by noise", ids, len(ids), targetID)
	}
}

// TestFTSCJKChannel_MissingColumnDegrades_PG: 老库未应用 20261292
// （bigram_text 不存在 → 42883；#9 后不再引用生成列，42703 仅可能来自
// r1 时期的列设计库）时，CJK 通道必须记 debug 并跳过，绝不能使召回报错。
func TestFTSCJKChannel_MissingColumnDegrades_PG(t *testing.T) {
	r := setupL3FTSTestRepo(t, nil, 0) // 刻意不执行 20261292 迁移
	ctx := context.Background()
	scope := "agent-cjk-unmigrated"
	targetID := upsertL3Fact(t, r, scope, "Deploy token ZXQ4817 belongs to Falcon project", 0.5)

	ids, err := r.searchL3FTSCJK(ctx, "agent", scope, "", "喜欢蓝色", 10)
	if err != nil || ids != nil {
		t.Fatalf("missing-migration CJK search: ids=%v err=%v, want nil/nil (channel skip)", ids, err)
	}

	// 合并包装同样调用安全：英文通道照常返回，CJK 通道静默跳过。
	merged := r.ftsCandidateIDs(ctx, "agent", scope, "", "喜欢蓝色 ZXQ4817", 10)
	found := false
	for _, id := range merged {
		if id == targetID {
			found = true
		}
	}
	if !found {
		t.Fatalf("merged ftsCandidateIDs must still return English FTS hit %s, got %v", targetID, merged)
	}

	// ftsExtraCandidatesAt 路径（暴力召回注入）同样不得因缺列报错。
	scored, err := r.ftsExtraCandidatesAt(ctx, "agent", scope, "", "喜欢蓝色 ZXQ4817", nil, map[string]struct{}{}, 0, time.Now().UTC(), l3FTSCandidateLimit)
	if err != nil {
		t.Fatalf("ftsExtraCandidatesAt on unmigrated schema must not fail: %v", err)
	}
	if len(scored) == 0 {
		t.Fatal("ftsExtraCandidatesAt returned no candidates on unmigrated schema")
	}
}

// TestMergeRankedIDLists covers the dedup/ordering helper: primary list wins,
// blanks dropped, duplicates removed, empty in → nil out.
func TestMergeRankedIDLists(t *testing.T) {
	got := mergeRankedIDLists([]string{"a", "b"}, []string{"b", "c"}, nil)
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i, id := range want {
		if got[i] != id {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	if got := mergeRankedIDLists(nil, []string{" ", ""}); got != nil {
		t.Fatalf("blank-only input: got %v, want nil", got)
	}
}
