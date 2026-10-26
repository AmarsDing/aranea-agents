// knowledge_source_sync_repo_test.go —— G-38 SourceSync W1 PG 钉阵
// （四表 hand-repo 行为钉 + 迁移/Ensure 双轨钉 + partial-unique 活性唯一钉
// + P10 保留双 DELETE 钉；隔离 schema 走 testhelper，与 20261294 迁移测试
// 家族同构）。
package data

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/biz/knowledge/sourcesync"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/apierror"
)

var sourceSyncMigrationFiles = []string{
	"sql/migrations/20261298_knowledge_source_connectors.sql",
	"sql/migrations/20261299_knowledge_source_connector_items.sql",
	"sql/migrations/20261300_knowledge_source_connector_credential.sql",
	"sql/migrations/20261301_knowledge_source_connector_runs.sql",
}

// ── (a) 迁移幂等 + regclass + 黄金列集合钉（迁移轨道） ──

func TestSourceSyncMigrations_Idempotent(t *testing.T) {
	db := testhelper.SetupTestPGRaw(t)
	ctx := context.Background()
	for round := 0; round < 2; round++ {
		for _, f := range sourceSyncMigrationFiles {
			raw, err := os.ReadFile(f)
			if err != nil {
				t.Fatalf("read %s: %v", f, err)
			}
			if _, err := db.ExecContext(ctx, string(raw)); err != nil {
				t.Fatalf("apply %s round %d: %v", f, round, err)
			}
		}
	}
	for _, table := range sourceSyncTables {
		var reg *string
		if err := db.QueryRowContext(ctx,
			"SELECT to_regclass(current_schema() || '.' || $1)::text", table).Scan(&reg); err != nil {
			t.Fatalf("regclass %s: %v", table, err)
		}
		if reg == nil || *reg == "" {
			t.Fatalf("table %s missing after migrations", table)
		}
		mismatch, err := assertSourceSyncColumnSet(ctx, db, table, sourceSyncTableColumnSet[table])
		if err != nil {
			t.Fatalf("column set %s: %v", table, err)
		}
		if mismatch != "" {
			t.Fatalf("migration track golden mismatch: %s", mismatch)
		}
	}
}

// ── (b) 双轨 Ensure 镜像钉 ──

func TestSourceSyncEnsure_DualTrackMirror(t *testing.T) {
	db := testhelper.SetupTestPGRaw(t)
	ctx := context.Background()
	if err := EnsureSourceSyncSchema(ctx, db); err != nil {
		t.Fatalf("ensure round 1: %v", err)
	}
	if err := EnsureSourceSyncSchema(ctx, db); err != nil {
		t.Fatalf("ensure round 2: %v", err)
	}
	for _, table := range sourceSyncTables {
		mismatch, err := assertSourceSyncColumnSet(ctx, db, table, sourceSyncTableColumnSet[table])
		if err != nil {
			t.Fatalf("column set %s: %v", table, err)
		}
		if mismatch != "" {
			t.Fatalf("ensure track golden mismatch: %s", mismatch)
		}
	}
	if err := EnsureSourceSyncSchema(ctx, nil); err != nil {
		t.Fatalf("nil-safe ensure: %v", err)
	}
}

// openSourceSyncRepo 建库建表（Ensure 轨道）后返回四仓聚合面。
func openSourceSyncRepo(t *testing.T) SourceSyncRepos {
	t.Helper()
	d := openTestData(t)
	if err := EnsureSourceSyncSchema(context.Background(), d.RWDB().WriteHandle()); err != nil {
		t.Fatalf("ensure schema: %v", err)
	}
	return NewSourceSyncRepo(d)
}

func seedSourceSyncConnector(t *testing.T, repo SourceSyncRepos, id, workspaceID, collectionID string, enabled bool, state sourcesync.SyncState, nextRunAt time.Time) {
	t.Helper()
	err := repo.CreateConnector(context.Background(), sourcesync.Connector{
		ID: id, Kind: sourcesync.KindSharedDir, Name: "c-" + id,
		WorkspaceID: workspaceID, CollectionID: collectionID,
		ConfigJSON: `{}`, IntervalSeconds: sourcesync.MinIntervalSeconds,
		Enabled: enabled, SyncState: state, NextRunAt: nextRunAt,
		MaxItemsPerRun:      sourcesync.DefaultMaxItemsPerRun,
		MaxDeletesPerRun:    sourcesync.DefaultMaxDeletesPerRun,
		MaxFetchBytesPerRun: sourcesync.DefaultMaxFetchBytesPerRun,
	})
	if err != nil {
		t.Fatalf("seed connector %s: %v", id, err)
	}
}

// ── (c) connector CRUD 往返 + due 拾取门控钉 ──

func TestSourceSyncConnector_CRUDRoundtrip(t *testing.T) {
	repo := openSourceSyncRepo(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	seedSourceSyncConnector(t, repo, "sc-1", "ws-1", "col-1", true, sourcesync.SyncStateIdle, now)

	got, found, err := repo.GetConnector(ctx, "sc-1")
	if err != nil || !found {
		t.Fatalf("get: found=%v err=%v", found, err)
	}
	if got.Kind != sourcesync.KindSharedDir || got.CollectionID != "col-1" {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Fatal("created/updated must be stamped by data side")
	}
	if !got.NextRunAt.Equal(now) {
		t.Fatalf("next_run_at mismatch: got %v want %v", got.NextRunAt, now)
	}

	got.Name = "renamed"
	got.SyncState = sourcesync.SyncStateError
	got.ErrorCode = sourcesync.ErrCodeWalkFailed
	ok, err := repo.UpdateConnector(ctx, got)
	if err != nil || !ok {
		t.Fatalf("update: ok=%v err=%v", ok, err)
	}
	again, _, _ := repo.GetConnector(ctx, "sc-1")
	if again.Name != "renamed" || again.SyncState != sourcesync.SyncStateError || again.ErrorCode != sourcesync.ErrCodeWalkFailed {
		t.Fatalf("update not applied: %+v", again)
	}

	if _, found, err = repo.GetConnector(ctx, "ghost"); err != nil || found {
		t.Fatalf("ghost get: found=%v err=%v", found, err)
	}
	if ok, err = repo.UpdateConnector(ctx, sourcesync.Connector{ID: "ghost", Kind: sourcesync.KindSharedDir}); err != nil || ok {
		t.Fatalf("ghost update: ok=%v err=%v", ok, err)
	}

	seedSourceSyncConnector(t, repo, "sc-2", "ws-1", "col-2", false, sourcesync.SyncStateIdle, now)
	list, err := repo.ListConnectorsByWorkspace(ctx, "ws-1")
	if err != nil || len(list) != 2 {
		t.Fatalf("workspace list: n=%d err=%v", len(list), err)
	}
	other, err := repo.ListConnectorsByWorkspace(ctx, "ws-other")
	if err != nil || len(other) != 0 {
		t.Fatalf("cross-workspace leak: n=%d err=%v", len(other), err)
	}
}

func TestSourceSyncConnector_ListDueGating(t *testing.T) {
	repo := openSourceSyncRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()
	past := now.Add(-time.Minute)
	future := now.Add(time.Hour)

	seedSourceSyncConnector(t, repo, "due-idle", "ws-d", "col-d1", true, sourcesync.SyncStateIdle, past)
	seedSourceSyncConnector(t, repo, "due-error", "ws-d", "col-d2", true, sourcesync.SyncStateError, past)
	seedSourceSyncConnector(t, repo, "ex-disabled", "ws-d", "col-d3", false, sourcesync.SyncStateIdle, past)
	seedSourceSyncConnector(t, repo, "ex-future", "ws-d", "col-d4", true, sourcesync.SyncStateIdle, future)
	seedSourceSyncConnector(t, repo, "ex-running", "ws-d", "col-d5", true, sourcesync.SyncStateRunning, past)
	seedSourceSyncConnector(t, repo, "ex-paused", "ws-d", "col-d6", true, sourcesync.SyncStateAuthPaused, past)
	seedSourceSyncConnector(t, repo, "ex-norun", "ws-d", "col-d7", true, sourcesync.SyncStateIdle, time.Time{})

	due, err := repo.ListDueConnectors(ctx, now, 0)
	if err != nil {
		t.Fatalf("list due: %v", err)
	}
	ids := map[string]bool{}
	for _, c := range due {
		ids[c.ID] = true
	}
	for _, want := range []string{"due-idle", "due-error"} {
		if !ids[want] {
			t.Fatalf("due missing %s", want)
		}
	}
	for _, banned := range []string{"ex-disabled", "ex-future", "ex-running", "ex-paused", "ex-norun"} {
		if ids[banned] {
			t.Fatalf("due leaked %s", banned)
		}
	}

	earlier := now.Add(-time.Hour)
	seedSourceSyncConnector(t, repo, "due-earlier", "ws-d", "col-d8", true, sourcesync.SyncStateIdle, earlier)
	due2, _ := repo.ListDueConnectors(ctx, now, 0)
	if len(due2) < 1 || due2[0].ID != "due-earlier" {
		t.Fatalf("due ASC order broken")
	}
	limited, _ := repo.ListDueConnectors(ctx, now, 1)
	if len(limited) != 1 {
		t.Fatalf("due limit broken: n=%d", len(limited))
	}
}

// ── (d) 软删级联：connector 删除 → 凭据同笔软删 ──

func TestSourceSyncConnector_SoftDeleteCascade(t *testing.T) {
	repo := openSourceSyncRepo(t)
	ctx := context.Background()
	seedSourceSyncConnector(t, repo, "sc-cas", "ws-1", "col-cas", true, sourcesync.SyncStateIdle, time.Now())
	if err := repo.UpsertCredential(ctx, sourcesync.Credential{
		ID: "cred-1", ConnectorID: "sc-cas", CredentialKey: "token",
		Status: "active", SecretRef: "enc:v1:abc", MetadataJSON: `{}`,
	}); err != nil {
		t.Fatalf("seed credential: %v", err)
	}

	ok, err := repo.SoftDeleteConnector(ctx, "sc-cas")
	if err != nil || !ok {
		t.Fatalf("soft delete: ok=%v err=%v", ok, err)
	}
	if _, found, _ := repo.GetConnector(ctx, "sc-cas"); found {
		t.Fatal("connector still visible after soft delete")
	}
	if _, found, _ := repo.GetCredential(ctx, "sc-cas", "token"); found {
		t.Fatal("credential not cascaded")
	}
	creds, err := repo.ListCredentialsByConnector(ctx, "sc-cas")
	if err != nil || len(creds) != 0 {
		t.Fatalf("credential list after cascade: n=%d err=%v", len(creds), err)
	}
	if ok, err = repo.SoftDeleteConnector(ctx, "sc-cas"); err != nil || ok {
		t.Fatalf("re-soft-delete: ok=%v err=%v", ok, err)
	}
	seedSourceSyncConnector(t, repo, "sc-cas-2", "ws-1", "col-cas", true, sourcesync.SyncStateIdle, time.Now())
}

// ── (e) collection 活性部分唯一钉（23505 → CodeConflict） ──

func TestSourceSyncConnector_CollectionActiveUnique(t *testing.T) {
	repo := openSourceSyncRepo(t)
	ctx := context.Background()
	seedSourceSyncConnector(t, repo, "u-1", "ws-1", "col-u", true, sourcesync.SyncStateIdle, time.Now())

	err := repo.CreateConnector(ctx, sourcesync.Connector{
		ID: "u-2", Kind: sourcesync.KindNotion, Name: "dup",
		WorkspaceID: "ws-1", CollectionID: "col-u",
		ConfigJSON: `{}`, IntervalSeconds: sourcesync.MinIntervalSeconds,
		MaxItemsPerRun: 1, MaxDeletesPerRun: 1, MaxFetchBytesPerRun: 1,
	})
	if !apierror.IsCode(err, apierror.CodeConflict) {
		t.Fatalf("expected CodeConflict on active duplicate, got %v", err)
	}
	if ok, _ := repo.SoftDeleteConnector(ctx, "u-1"); !ok {
		t.Fatal("soft delete failed")
	}
	seedSourceSyncConnector(t, repo, "u-3", "ws-1", "col-u", true, sourcesync.SyncStateIdle, time.Now())
}

// ── (f) 凭据 partial-unique 钉（P1）：原生 23505、upsert 原位更新、软删重建新行 ──

func TestSourceSyncCredential_PartialUniquePin(t *testing.T) {
	d := openTestData(t)
	if err := EnsureSourceSyncSchema(context.Background(), d.RWDB().WriteHandle()); err != nil {
		t.Fatal(err)
	}
	repo := NewSourceSyncRepo(d)
	ctx := context.Background()
	raw := d.RWDB().WriteHandle()

	// 原生 SQL 双活插入 → 23505（索引实质钉，与 upsert 仲裁无关路径）。
	if _, err := raw.ExecContext(ctx, `
INSERT INTO knowledge_source_connector_credential (id, connector_id, credential_key, status, secret_ref, metadata_json, created_at, updated_at, deleted_at)
VALUES ('raw-1', 'pc-raw', 'token', 'active', 'enc:v1:x', '{}', '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z', '')`); err != nil {
		t.Fatalf("raw seed: %v", err)
	}
	_, rawErr := raw.ExecContext(ctx, `
INSERT INTO knowledge_source_connector_credential (id, connector_id, credential_key, status, secret_ref, metadata_json, created_at, updated_at, deleted_at)
VALUES ('raw-2', 'pc-raw', 'token', 'active', 'enc:v1:y', '{}', '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z', '')`)
	if !isPgUniqueViolation(rawErr) {
		t.Fatalf("expected 23505 from raw duplicate, got %v", rawErr)
	}
	if !apierror.IsCode(entErrToBizErr(rawErr, sourceSyncModule), apierror.CodeConflict) {
		t.Fatal("23505 must translate to CodeConflict")
	}

	seedSourceSyncConnector(t, repo, "pc-1", "ws-1", "col-pc", true, sourcesync.SyncStateIdle, time.Now())
	first := sourcesync.Credential{
		ID: "pc-cred-1", ConnectorID: "pc-1", CredentialKey: "token",
		Status: "active", SecretRef: "enc:v1:first", MetadataJSON: `{"bot_id":"b1"}`,
	}
	if err := repo.UpsertCredential(ctx, first); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	got, found, _ := repo.GetCredential(ctx, "pc-1", "token")
	if !found || got.SecretRef != "enc:v1:first" {
		t.Fatalf("first read: %+v found=%v", got, found)
	}

	// upsert 活性冲突 → 原位更新：id/created_at 保留，secret_ref/metadata 翻页。
	second := sourcesync.Credential{
		ID: "pc-cred-2", ConnectorID: "pc-1", CredentialKey: "token",
		Status: "rotated", SecretRef: "enc:v1:second", MetadataJSON: `{"bot_id":"b2"}`,
	}
	if err := repo.UpsertCredential(ctx, second); err != nil {
		t.Fatalf("conflict upsert: %v", err)
	}
	got2, _, _ := repo.GetCredential(ctx, "pc-1", "token")
	if got2.ID != "pc-cred-1" {
		t.Fatalf("arbiter must update in place (id preserved), got %s", got2.ID)
	}
	if got2.SecretRef != "enc:v1:second" || got2.Status != "rotated" {
		t.Fatalf("arbiter update not applied: %+v", got2)
	}
	if !got2.CreatedAt.Equal(got.CreatedAt) {
		t.Fatalf("created_at must survive arbiter update")
	}

	// 软删 → 同 key 重建 = 新行（墓碑豁免钉）。
	if ok, err := repo.SoftDeleteCredential(ctx, "pc-1", "token"); err != nil || !ok {
		t.Fatalf("soft delete credential: ok=%v err=%v", ok, err)
	}
	third := sourcesync.Credential{
		ID: "pc-cred-3", ConnectorID: "pc-1", CredentialKey: "token",
		Status: "active", SecretRef: "enc:v1:third", MetadataJSON: `{}`,
	}
	if err := repo.UpsertCredential(ctx, third); err != nil {
		t.Fatalf("recreate after soft delete: %v", err)
	}
	got3, _, _ := repo.GetCredential(ctx, "pc-1", "token")
	if got3.ID != "pc-cred-3" {
		t.Fatalf("recreate must be a NEW row, got id %s", got3.ID)
	}
}

func TestSourceSyncCredential_SecretRefGate(t *testing.T) {
	repo := openSourceSyncRepo(t)
	ctx := context.Background()
	err := repo.UpsertCredential(ctx, sourcesync.Credential{
		ID: "g-1", ConnectorID: "g-c", CredentialKey: "token", SecretRef: "",
	})
	if !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("empty secret_ref must be BadRequest, got %v", err)
	}
}

// ── (g) items 生命周期：flap 复位钉 / Set* 守卫 / 缺席圈定 / adopt 安全 ──

func TestSourceSyncItem_Lifecycle(t *testing.T) {
	repo := openSourceSyncRepo(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	if err := repo.UpsertItemSeen(ctx, "ic-1", "ext-1", "doc-1", "fp-1", true, "run-1", now); err != nil {
		t.Fatalf("upsert seen: %v", err)
	}
	it, found, _ := repo.GetItem(ctx, "ic-1", "ext-1")
	if !found || it.Status != sourcesync.ItemStatusActive || it.Fingerprint != "fp-1" {
		t.Fatalf("first seen: %+v found=%v", it, found)
	}

	if ok, _ := repo.SetItemDocument(ctx, "ic-1", "ext-1", "doc-9", false); !ok {
		t.Fatal("set document")
	}
	if ok, _ := repo.SetItemContentHash(ctx, "ic-1", "ext-1", "hash-9"); !ok {
		t.Fatal("set content hash")
	}
	if ok, _ := repo.SetItemError(ctx, "ic-1", "ext-1", "fetch boom"); !ok {
		t.Fatal("set error")
	}

	n, err := repo.MarkAbsentItemsMissing(ctx, "ic-1", "run-2")
	if err != nil || n != 1 {
		t.Fatalf("mark absent: n=%d err=%v", n, err)
	}
	it, _, _ = repo.GetItem(ctx, "ic-1", "ext-1")
	if it.Status != sourcesync.ItemStatusMissing || it.MissCount != 1 {
		t.Fatalf("absent not marked: %+v", it)
	}

	later := now.Add(time.Hour)
	if err := repo.UpsertItemSeen(ctx, "ic-1", "ext-1", "doc-9", "fp-2", false, "run-3", later); err != nil {
		t.Fatalf("re-see: %v", err)
	}
	it, _, _ = repo.GetItem(ctx, "ic-1", "ext-1")
	if it.Status != sourcesync.ItemStatusActive || it.MissCount != 0 || it.ErrorMessage != "" {
		t.Fatalf("flap reset broken: %+v", it)
	}
	if it.Fingerprint != "fp-2" || it.LastSeenRunID != "run-3" {
		t.Fatalf("fingerprint/run not advanced: %+v", it)
	}
	if it.DocumentID != "doc-9" || it.ContentHash != "hash-9" || it.OwnLifecycle {
		t.Fatalf("binding/checkpoint/adopt must survive presence upsert: %+v", it)
	}

	if ok, _ := repo.MarkItemDeleted(ctx, "ic-1", "ext-1", later); !ok {
		t.Fatal("mark deleted")
	}
	if ok, _ := repo.SetItemContentHash(ctx, "ic-1", "ext-1", "hash-x"); ok {
		t.Fatal("SetItemContentHash must refuse deleted row")
	}
	if ok, _ := repo.SetItemDocument(ctx, "ic-1", "ext-1", "doc-x", true); ok {
		t.Fatal("SetItemDocument must refuse deleted row")
	}
	if ok, _ := repo.SetItemError(ctx, "ic-1", "ext-1", "x"); ok {
		t.Fatal("SetItemError must refuse deleted row")
	}

	if err := repo.UpsertItemSeen(ctx, "ic-1", "ext-1", "doc-9", "fp-3", false, "run-4", later.Add(time.Hour)); err != nil {
		t.Fatalf("resurrect: %v", err)
	}
	it, _, _ = repo.GetItem(ctx, "ic-1", "ext-1")
	if it.Status != sourcesync.ItemStatusActive || !it.DeletedAt.IsZero() {
		t.Fatalf("resurrection broken: %+v", it)
	}

	if err := repo.UpsertItemSeen(ctx, "ic-1", "ext-2", "doc-2", "fp", true, "run-4", now); err != nil {
		t.Fatal(err)
	}
	if cnt, _ := repo.CountActiveItems(ctx, "ic-1"); cnt != 2 {
		t.Fatalf("active count: %d", cnt)
	}
	if _, err := repo.MarkAbsentItemsMissing(ctx, "ic-1", "run-5"); err != nil {
		t.Fatal(err)
	}
	missing, err := repo.ListItemsByStatus(ctx, "ic-1", sourcesync.ItemStatusMissing, 0)
	if err != nil || len(missing) != 2 {
		t.Fatalf("missing list: n=%d err=%v", len(missing), err)
	}
	if cnt, _ := repo.CountActiveItems(ctx, "ic-1"); cnt != 0 {
		t.Fatalf("active count after absent: %d", cnt)
	}
}

// ── (h) P10 保留双 DELETE：runs 保 100 最新 + items 90 天硬清 ──

func TestSourceSyncRun_RetentionKeepsNewest100(t *testing.T) {
	repo := openSourceSyncRepo(t)
	ctx := context.Background()
	base := time.Now().UTC().Add(-time.Hour)

	for i := 0; i < 105; i++ {
		run := sourcesync.Run{
			ID:          fmt.Sprintf("run-%03d", i),
			ConnectorID: "rc-1",
			StartedAt:   base.Add(time.Duration(i) * time.Minute),
			Status:      sourcesync.RunStatusSuccess,
		}
		if err := repo.InsertRunWithRetention(ctx, run); err != nil {
			t.Fatalf("insert run %d: %v", i, err)
		}
	}
	runs, err := repo.ListRuns(ctx, "rc-1", 500)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != sourcesync.RunKeep {
		t.Fatalf("retention must keep %d, got %d", sourcesync.RunKeep, len(runs))
	}
	if runs[0].ID != "run-104" {
		t.Fatalf("newest run must be kept first, got %s", runs[0].ID)
	}
	for _, r := range runs {
		if r.ID == "run-000" || r.ID == "run-004" {
			t.Fatalf("oldest run %s must be pruned", r.ID)
		}
	}

	if err := repo.InsertRunWithRetention(ctx, sourcesync.Run{
		ID: "other-1", ConnectorID: "rc-2",
		StartedAt: base, Status: sourcesync.RunStatusFailed,
	}); err != nil {
		t.Fatal(err)
	}
	other, _ := repo.ListRuns(ctx, "rc-2", 500)
	if len(other) != 1 {
		t.Fatalf("cross-connector isolation broken: n=%d", len(other))
	}
}

func TestSourceSyncRun_ItemRetentionSecondDelete(t *testing.T) {
	repo := openSourceSyncRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()
	old := now.Add(-sourcesync.ItemRetention - time.Hour)
	recent := now.Add(-sourcesync.ItemRetention + time.Hour)

	seed := func(ext string, seenAt time.Time) {
		t.Helper()
		if err := repo.UpsertItemSeen(context.Background(), "ir-1", ext, "doc-"+ext, "fp", true, "run-x", seenAt); err != nil {
			t.Fatal(err)
		}
	}
	// 超窗 missing（last_seen_at 准）→ 必清。
	seed("old-missing", old)
	if _, err := repo.MarkAbsentItemsMissing(ctx, "ir-1", "run-y"); err != nil {
		t.Fatal(err)
	}
	// 窗内 missing → 必留。
	seed("recent-missing", recent)
	if _, err := repo.MarkAbsentItemsMissing(ctx, "ir-1", "run-z"); err != nil {
		t.Fatal(err)
	}
	// 超窗 deleted（deleted_at 准）→ 必清。
	seed("old-deleted", old)
	if ok, _ := repo.MarkItemDeleted(ctx, "ir-1", "old-deleted", old); !ok {
		t.Fatal("mark old deleted")
	}
	// adopt active 行（own_lifecycle=false）→ 永不被姊妹条款碰。
	seed("adopt-active", now)
	if ok, _ := repo.SetItemDocument(ctx, "ir-1", "adopt-active", "doc-adopt", false); !ok {
		t.Fatal("set adopt")
	}

	if err := repo.InsertRunWithRetention(ctx, sourcesync.Run{
		ID: "final-1", ConnectorID: "ir-1",
		StartedAt: now, Status: sourcesync.RunStatusPartial,
	}); err != nil {
		t.Fatalf("finalize: %v", err)
	}

	mustGone := func(ext string) {
		t.Helper()
		if _, found, _ := repo.GetItem(context.Background(), "ir-1", ext); found {
			t.Fatalf("%s must be purged by 90-day retention", ext)
		}
	}
	mustStay := func(ext string) {
		t.Helper()
		if _, found, _ := repo.GetItem(context.Background(), "ir-1", ext); !found {
			t.Fatalf("%s must survive retention window", ext)
		}
	}
	mustGone("old-missing")
	mustGone("old-deleted")
	mustStay("recent-missing")
	mustStay("adopt-active")
	it, _, _ := repo.GetItem(ctx, "ir-1", "adopt-active")
	if it.OwnLifecycle {
		t.Fatal("adopt flag flipped by retention")
	}
}

// ── (i) DDL CHECK 违例分级钉（kind 白名单 23514 → CodeBadRequest） ──

func TestSourceSyncDDL_CheckViolationMapping(t *testing.T) {
	repo := openSourceSyncRepo(t)
	ctx := context.Background()
	err := repo.CreateConnector(ctx, sourcesync.Connector{
		ID: "bad-kind", Kind: sourcesync.Kind("web"), Name: "bad",
		WorkspaceID: "ws-1", CollectionID: "col-bad",
		ConfigJSON: `{}`, IntervalSeconds: sourcesync.MinIntervalSeconds,
		MaxItemsPerRun: 1, MaxDeletesPerRun: 1, MaxFetchBytesPerRun: 1,
	})
	if !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("kind CHECK violation must be CodeBadRequest, got %v", err)
	}
	err = repo.InsertRunWithRetention(ctx, sourcesync.Run{
		ID: "bad-run", ConnectorID: "ir-x",
		StartedAt: time.Now(), Status: sourcesync.RunStatus("running"),
	})
	if !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("run status CHECK violation must be CodeBadRequest, got %v", err)
	}
}

// ── (j) nil-data 安全壳钉 ──

func TestSourceSyncRepo_NilDataShell(t *testing.T) {
	repo := NewSourceSyncRepo(nil)
	ctx := context.Background()

	if err := repo.CreateConnector(ctx, sourcesync.Connector{ID: "x"}); !apierror.IsCode(err, apierror.CodeInternal) {
		t.Fatalf("nil create: %v", err)
	}
	if _, found, err := repo.GetConnector(ctx, "x"); found || !apierror.IsCode(err, apierror.CodeInternal) {
		t.Fatalf("nil get: found=%v err=%v", found, err)
	}
	if _, err := repo.ListDueConnectors(ctx, time.Now(), 0); !apierror.IsCode(err, apierror.CodeInternal) {
		t.Fatalf("nil due: %v", err)
	}
	if err := repo.UpsertItemSeen(ctx, "c", "e", "d", "f", true, "r", time.Now()); !apierror.IsCode(err, apierror.CodeInternal) {
		t.Fatalf("nil item: %v", err)
	}
	if err := repo.UpsertCredential(ctx, sourcesync.Credential{ID: "x", ConnectorID: "c", CredentialKey: "k", SecretRef: "enc:v1:x"}); !apierror.IsCode(err, apierror.CodeInternal) {
		t.Fatalf("nil credential: %v", err)
	}
	if err := repo.InsertRunWithRetention(ctx, sourcesync.Run{ID: "r", ConnectorID: "c", StartedAt: time.Now(), Status: sourcesync.RunStatusSuccess}); !apierror.IsCode(err, apierror.CodeInternal) {
		t.Fatalf("nil run: %v", err)
	}
}

// ── 长 error_message 截断落库钉 ──

func TestSourceSyncConnector_ErrorMessageTruncation(t *testing.T) {
	repo := openSourceSyncRepo(t)
	ctx := context.Background()
	seedSourceSyncConnector(t, repo, "tr-1", "ws-1", "col-tr", true, sourcesync.SyncStateIdle, time.Now())
	got, _, _ := repo.GetConnector(ctx, "tr-1")
	got.ErrorMessage = strings.Repeat("知", 600)
	got.ErrorCode = sourcesync.ErrCodeFetchFailed
	if ok, err := repo.UpdateConnector(ctx, got); err != nil || !ok {
		t.Fatalf("update: ok=%v err=%v", ok, err)
	}
	again, _, _ := repo.GetConnector(ctx, "tr-1")
	if n := len([]rune(again.ErrorMessage)); n != 512 {
		t.Fatalf("error_message must truncate to 512 runes, got %d", n)
	}
}
