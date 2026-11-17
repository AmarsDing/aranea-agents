package data

// V1 续（E 阶段 W2c G）：GitHub 面板台账 roundtrip，跑真实迁移 20261330
// DDL 于 55433 隔离测试库（ARANEA_TEST_PG_DSN），与
// memory_fact_pending_repo_test.go 同例。

import (
	"context"
	"database/sql"
	"testing"

	"aranea-agents/internal/biz/githubpanel"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// newGitHubPanelTestRepo builds the repo over an isolated PG schema with the
// real 20261330 migration DDL applied.
func newGitHubPanelTestRepo(t *testing.T) (*sql.DB, githubpanel.Repo) {
	t.Helper()
	ctx := context.Background()
	db := testhelper.SetupTestPGRaw(t)
	lg := loggateway.NewNoop()
	if err := executeSQLFileWithDialect(ctx, db, "sql/migrations/20261330_github_panel.sql", DialectPostgres, lg); err != nil {
		t.Fatalf("migrate 20261330: %v", err)
	}
	d := &Data{rawDB: db, readDB: db, rwDB: NewReadWriteDB(db, db), lg: lg, dialect: DialectPostgres}
	repo := NewGitHubPanelRepo(d)
	if repo == nil {
		t.Fatal("repo constructor returned nil over live DB")
	}
	return db, repo
}

// TestGitHubPanelRepo_WatchRoundtrip 钉死 upsert 幂等语义：同 full_name 重
// watch 保留原 id/created_at/健康快照列，只更新 credential 绑定。
func TestGitHubPanelRepo_WatchRoundtrip(t *testing.T) {
	ctx := context.Background()
	_, repo := newGitHubPanelTestRepo(t)

	saved, err := repo.UpsertWatch(ctx, githubpanel.WatchedRepo{
		ID: "ghr-1", FullName: "org/repo", CredentialID: "42", CIStatus: "unknown",
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if saved.ID != "ghr-1" || saved.FullName != "org/repo" || saved.CredentialID != "42" {
		t.Fatalf("saved mismatch: %+v", saved)
	}
	if saved.WebhookOK {
		t.Fatal("webhook_ok must default false")
	}
	if saved.CIStatus != "unknown" {
		t.Fatalf("ci_status mismatch: %s", saved.CIStatus)
	}
	if saved.CreatedAt.IsZero() || saved.UpdatedAt.IsZero() {
		t.Fatal("timestamps must be populated")
	}
	if !saved.LastEventAt.IsZero() || !saved.HealthUpdatedAt.IsZero() {
		t.Fatal("nullable snapshot times must be zero")
	}

	// 幂等：同 full_name 重 watch（外部生成新 id）→ 保留原行。
	re, err := repo.UpsertWatch(ctx, githubpanel.WatchedRepo{
		ID: "ghr-NEW", FullName: "org/repo", CredentialID: "77", CIStatus: "green",
	})
	if err != nil {
		t.Fatalf("re-upsert: %v", err)
	}
	if re.ID != "ghr-1" {
		t.Fatalf("re-watch must keep original id, got %s", re.ID)
	}
	if re.CredentialID != "77" {
		t.Fatalf("credential binding must update, got %s", re.CredentialID)
	}
	if !re.CreatedAt.Equal(saved.CreatedAt) {
		t.Fatalf("created_at must survive re-watch: %v vs %v", re.CreatedAt, saved.CreatedAt)
	}
	if re.CIStatus != "unknown" {
		t.Fatalf("health snapshot columns must survive re-watch, got %s", re.CIStatus)
	}

	list, err := repo.ListWatched(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("re-watch must not duplicate rows: n=%d err=%v", len(list), err)
	}
}

// TestGitHubPanelRepo_ListOrder 钉死 created_at ASC 排序（秒级精度并列时
// 以直接改库钉死时间差）。
func TestGitHubPanelRepo_ListOrder(t *testing.T) {
	ctx := context.Background()
	db, repo := newGitHubPanelTestRepo(t)

	for _, r := range []githubpanel.WatchedRepo{
		{ID: "ghr-a", FullName: "zeta/one", CredentialID: "1", CIStatus: "unknown"},
		{ID: "ghr-b", FullName: "alpha/two", CredentialID: "1", CIStatus: "unknown"},
	} {
		if _, err := repo.UpsertWatch(ctx, r); err != nil {
			t.Fatalf("upsert %s: %v", r.FullName, err)
		}
	}
	if _, err := db.ExecContext(ctx,
		`UPDATE github_repos SET created_at = '2026-01-01T00:00:00Z' WHERE id = 'ghr-b'`); err != nil {
		t.Fatalf("pin created_at: %v", err)
	}

	list, err := repo.ListWatched(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("want 2 rows, got %d", len(list))
	}
	if list[0].FullName != "alpha/two" || list[1].FullName != "zeta/one" {
		t.Fatalf("created_at ASC order broken: %s, %s", list[0].FullName, list[1].FullName)
	}
}

// TestGitHubPanelRepo_Unwatch 钉死摘除语义：存在 id 恰一次成功，未知/二次
// 删除 → NotFound。
func TestGitHubPanelRepo_Unwatch(t *testing.T) {
	ctx := context.Background()
	_, repo := newGitHubPanelTestRepo(t)

	if err := repo.Unwatch(ctx, "missing"); !apierror.IsCode(err, apierror.CodeNotFound) {
		t.Fatalf("missing id must be NotFound, got %v", err)
	}
	saved, err := repo.UpsertWatch(ctx, githubpanel.WatchedRepo{
		ID: "ghr-1", FullName: "org/repo", CredentialID: "1", CIStatus: "unknown",
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := repo.Unwatch(ctx, saved.ID); err != nil {
		t.Fatalf("unwatch: %v", err)
	}
	if err := repo.Unwatch(ctx, saved.ID); !apierror.IsCode(err, apierror.CodeNotFound) {
		t.Fatalf("double unwatch must be NotFound, got %v", err)
	}
	list, err := repo.ListWatched(ctx)
	if err != nil || len(list) != 0 {
		t.Fatalf("list after unwatch: n=%d err=%v", len(list), err)
	}
}
