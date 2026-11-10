package data

import (
	"context"
	"database/sql"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
)

// ensureClassificationTables 建 D6 密级面表（镜像迁移 20261327 DDL 的
// 相关子集；测试自包含，与 ensureOffboardTable 同策略）。db 必须与测试
// Data 同一 schema——复用 setupEnvelopeTestRepo 返回的句柄。
func ensureClassificationTables(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS classification_labels (
			level       INTEGER PRIMARY KEY,
			name        TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			updated_at  TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS model_policy (
			id              INTEGER PRIMARY KEY,
			local_threshold INTEGER NOT NULL DEFAULT 3,
			updated_by      TEXT NOT NULL DEFAULT '',
			updated_at      TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id       TEXT PRIMARY KEY,
			ceiling  INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS session_runs (
			id         TEXT PRIMARY KEY,
			session_id TEXT NOT NULL DEFAULT '',
			ceiling    INTEGER NOT NULL DEFAULT 0
		)`,
	}
	for _, s := range stmts {
		if _, err := db.ExecContext(ctx, s); err != nil {
			t.Fatalf("create classification table: %v", err)
		}
	}
	// EnsureSessionMemorySchema 可能已建过同名 sessions 表（无 ceiling 列，
	// CREATE IF NOT EXISTS 不会补齐）——按迁移 20261327 同款口径补列。
	for _, alter := range []string{
		`ALTER TABLE sessions ADD COLUMN IF NOT EXISTS ceiling INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE session_runs ADD COLUMN IF NOT EXISTS ceiling INTEGER NOT NULL DEFAULT 0`,
	} {
		if _, err := db.ExecContext(ctx, alter); err != nil {
			t.Fatalf("alter ceiling column: %v", err)
		}
	}
}

func newClassificationTestRepo(t *testing.T) (*secopsClassificationRepo, context.Context, *sql.DB) {
	t.Helper()
	d, _, db := setupEnvelopeTestRepo(t, 0x2a)
	ctx := context.Background()
	ensureClassificationTables(t, ctx, db)
	return &secopsClassificationRepo{data: d}, ctx, db
}

func TestSecopsClassificationRepo_Labels(t *testing.T) {
	repo, ctx, _ := newClassificationTestRepo(t)

	if err := repo.PutLabel(ctx, biz.ClassificationLabel{Level: 3, Name: "秘密", Description: "强制本地"}); err != nil {
		t.Fatalf("put label: %v", err)
	}
	// upsert 分支：同名 level 改名，行数不增。
	if err := repo.PutLabel(ctx, biz.ClassificationLabel{Level: 3, Name: "机密(原秘密)", Description: "改名"}); err != nil {
		t.Fatalf("put label upsert: %v", err)
	}
	if err := repo.PutLabel(ctx, biz.ClassificationLabel{Level: 1, Name: "公开"}); err != nil {
		t.Fatalf("put label L1: %v", err)
	}

	labels, err := repo.ListLabels(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(labels) != 2 {
		t.Fatalf("want 2 labels after upsert, got %d: %+v", len(labels), labels)
	}
	if labels[0].Level != 1 || labels[1].Level != 3 {
		t.Fatalf("order must be level ASC: %+v", labels)
	}
	if labels[1].Name != "机密(原秘密)" || labels[1].Description != "改名" {
		t.Fatalf("upsert not applied: %+v", labels[1])
	}
	if labels[0].UpdatedAt.IsZero() || labels[1].UpdatedAt.IsZero() {
		t.Fatalf("updated_at must be stamped: %+v", labels)
	}
}

func TestSecopsClassificationRepo_ModelPolicy(t *testing.T) {
	repo, ctx, db := newClassificationTestRepo(t)

	// 表空 → (nil, nil)，biz 层补默认 3。
	policy, err := repo.GetPolicy(ctx)
	if err != nil || policy != nil {
		t.Fatalf("empty policy must be (nil,nil): %v %v", policy, err)
	}

	if err := repo.SetPolicy(ctx, 4, "7"); err != nil {
		t.Fatalf("set policy: %v", err)
	}
	if err := repo.SetPolicy(ctx, 2, "8"); err != nil {
		t.Fatalf("set policy upsert: %v", err)
	}
	policy, err = repo.GetPolicy(ctx)
	if err != nil || policy == nil {
		t.Fatalf("get policy: %v %v", policy, err)
	}
	if policy.LocalThreshold != 2 || policy.UpdatedBy != "8" {
		t.Fatalf("policy mismatch: %+v", policy)
	}
	if policy.UpdatedAt.IsZero() {
		t.Fatal("updated_at must be stamped")
	}
	var rows int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM model_policy`).Scan(&rows); err != nil {
		t.Fatalf("count: %v", err)
	}
	if rows != 1 {
		t.Fatalf("model_policy must stay single-row, got %d", rows)
	}
}

func TestSecopsClassificationRepo_Waterline(t *testing.T) {
	repo, ctx, db := newClassificationTestRepo(t)

	// sessions 表由 Ent schema 建（title 为唯一无默认 NOT NULL 列）——
	// 种子必须带 title；ceiling 由上方 ALTER 保证存在。
	if _, err := db.ExecContext(ctx,
		`INSERT INTO sessions (id, title, ceiling) VALUES ('sess-w1', 'w1', 1), ('sess-missing-ceiling', 'm', 0)`); err != nil {
		t.Fatalf("seed sessions: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO session_runs (id, session_id, ceiling) VALUES ('run-w1', 'sess-w1', 0)`); err != nil {
		t.Fatalf("seed runs: %v", err)
	}

	// 未定级/无行 → 0。
	ceiling, err := repo.GetSessionCeiling(ctx, "sess-missing-ceiling")
	if err != nil || ceiling != 0 {
		t.Fatalf("unset ceiling: %d %v", ceiling, err)
	}
	ceiling, err = repo.GetSessionCeiling(ctx, "no-such-session")
	if err != nil || ceiling != 0 {
		t.Fatalf("missing session reads as 0: %d %v", ceiling, err)
	}

	// GREATEST 只升不降：1 → 3 → 2(不降) → 5。
	if ceiling, err = repo.RaiseSessionCeiling(ctx, "sess-w1", 3); err != nil || ceiling != 3 {
		t.Fatalf("raise to 3: %d %v", ceiling, err)
	}
	if ceiling, err = repo.RaiseSessionCeiling(ctx, "sess-w1", 2); err != nil || ceiling != 3 {
		t.Fatalf("lower raise must stay 3: %d %v", ceiling, err)
	}
	if ceiling, err = repo.RaiseSessionCeiling(ctx, "sess-w1", 5); err != nil || ceiling != 5 {
		t.Fatalf("raise to 5: %d %v", ceiling, err)
	}

	// run 水位线同语义。
	if ceiling, err = repo.RaiseRunCeiling(ctx, "run-w1", 4); err != nil || ceiling != 4 {
		t.Fatalf("raise run: %d %v", ceiling, err)
	}
	if ceiling, err = repo.RaiseRunCeiling(ctx, "run-w1", 1); err != nil || ceiling != 4 {
		t.Fatalf("lower run raise must stay 4: %d %v", ceiling, err)
	}

	// 不存在会话/run 显式抬升 → NotFound（不隐式建行）。
	if _, err = repo.RaiseSessionCeiling(ctx, "no-such-session", 3); !apierror.IsCode(err, apierror.CodeNotFound) {
		t.Fatalf("raise missing session must be NotFound: %v", err)
	}
	if _, err = repo.RaiseRunCeiling(ctx, "no-such-run", 3); !apierror.IsCode(err, apierror.CodeNotFound) {
		t.Fatalf("raise missing run must be NotFound: %v", err)
	}
}
