package data

import (
	"context"
	"sort"
	"testing"

	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/auth"
)

// TestDocVisibilityPredicate_SharedOverlapPG 是 G-08 谓词 `shared_with && $n::text[]`
// 的真实 PG 行为验证（TEXT[] overlap 属 PG 内核能力，不依赖 pgvector——本测试
// 自建最小表，可本地绿；全量 schema 集成见 knowledge_graph_test.go 系）。
// 行级语义（ADR-KN-20261020-D/E）：
//   - anonymous：仅 collection
//   - uid-only：collection + 属主
//   - scope-only（Agent org ctx）：collection + shared∩非空
//   - system：整体旁路
func TestDocVisibilityPredicate_SharedOverlapPG(t *testing.T) {
	db := testhelper.SetupTestPGRaw(t)
	ctx := context.Background()

	stmts := []string{
		`CREATE TABLE knowledge_documents (
			id            TEXT PRIMARY KEY,
			collection_id TEXT NOT NULL,
			visibility    TEXT,
			owner_user_id TEXT   NOT NULL DEFAULT '',
			shared_with   TEXT[] NOT NULL DEFAULT '{}'
		)`,
		`CREATE INDEX knowledge_documents_shared_with_gin ON knowledge_documents USING GIN (shared_with)`,
		`INSERT INTO knowledge_documents (id, collection_id, visibility, owner_user_id, shared_with) VALUES
			('pub',  'c1', 'collection', '',   '{}'),
			('mine', 'c1', 'private',    '42', '{}'),
			('oth',  'c1', 'private',    '7',  '{}'),
			('shrA', 'c1', 'shared',     '',    '{deptA}'),
			('shrB', 'c1', 'shared',     '7',   '{posX}'),
			('shr0', 'c1', 'shared',     '',    '{}'),
			('null', 'c1', NULL,         '',    '{}')`,
	}
	for _, s := range stmts {
		if _, err := db.ExecContext(ctx, s); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}

	visibleIDs := func(t *testing.T, c context.Context) []string {
		t.Helper()
		q := `SELECT id FROM knowledge_documents WHERE collection_id = $1`
		args := []any{"c1"}
		if clause, pargs := docRowVisibilityClause(c, len(args)+1); clause != "" {
			q += " " + clause
			args = append(args, pargs...)
		}
		q += ` ORDER BY id`
		rows, err := db.QueryContext(ctx, q, args...)
		if err != nil {
			t.Fatalf("query: %v", err)
		}
		defer rows.Close()
		var ids []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				t.Fatalf("scan: %v", err)
			}
			ids = append(ids, id)
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("rows: %v", err)
		}
		return ids
	}
	assertIDs := func(t *testing.T, name string, c context.Context, want []string) {
		t.Helper()
		got := visibleIDs(t, c)
		sort.Strings(got)
		if len(got) != len(want) {
			t.Fatalf("%s: got %v, want %v", name, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("%s: got %v, want %v", name, got, want)
			}
		}
	}

	t.Run("anonymous sees collection only", func(t *testing.T) {
		assertIDs(t, "anon", ctx, []string{"null", "pub"})
	})
	t.Run("uid owner sees own private", func(t *testing.T) {
		assertIDs(t, "uid42", auth.NewContext(ctx, &auth.Auth{UserID: 42}), []string{"mine", "null", "pub"})
	})
	t.Run("uid sees own private + other-owned shared", func(t *testing.T) {
		assertIDs(t, "uid7", auth.NewContext(ctx, &auth.Auth{UserID: 7}), []string{"null", "oth", "pub", "shrB"})
	})
	t.Run("agent scope hits shared overlap", func(t *testing.T) {
		scoped := bizknowledge.WithOrgScope(ctx, []string{"deptA"})
		assertIDs(t, "deptA-scope", scoped, []string{"null", "pub", "shrA"})
	})
	t.Run("agent scope position hit", func(t *testing.T) {
		scoped := bizknowledge.WithOrgScope(ctx, []string{"posX", "deptUNREL"})
		assertIDs(t, "posX-scope", scoped, []string{"null", "pub", "shrB"})
	})
	t.Run("empty shared_with is fail-closed", func(t *testing.T) {
		scoped := bizknowledge.WithOrgScope(ctx, []string{"deptA", "posX"})
		ids := visibleIDs(t, scoped)
		for _, id := range ids {
			if id == "shr0" {
				t.Fatalf("empty shared_with must stay invisible: %v", ids)
			}
		}
	})
	t.Run("system bypasses everything", func(t *testing.T) {
		sys := workspace.WithSystemWorkspace(ctx)
		assertIDs(t, "sys", sys, []string{"mine", "null", "oth", "pub", "shr0", "shrA", "shrB"})
	})
}
