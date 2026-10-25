package data

import (
	"context"
	"errors"
	"testing"
	"time"

	"aranea-agents/internal/biz/shared"
	biztool "aranea-agents/internal/biz/tool"
)

// ensureApprovalPolicyTable 在隔离 schema 内建表——逐句镜像迁移
// 20261291_session_approval_policies.sql（raw-SQL 表不经 ent Schema.Create；
// 迁移文件改动必须同步本镜像，列序钉死测试依赖不漂移）。
func ensureApprovalPolicyTable(t *testing.T, d *Data) {
	t.Helper()
	ctx := context.Background()
	db := d.RWDB().WriteDB(ctx)
	if db == nil {
		t.Fatal("test data rwdb unavailable")
	}
	_, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS session_approval_policies (
  id             TEXT PRIMARY KEY,
  scope          TEXT NOT NULL,
  scope_key      TEXT NOT NULL,
  tool_pattern   TEXT NOT NULL,
  command_prefix TEXT NOT NULL DEFAULT '',
  args_pattern   TEXT NOT NULL DEFAULT '',
  mode           TEXT NOT NULL,
  priority       INTEGER NOT NULL DEFAULT 100,
  enabled        INTEGER NOT NULL DEFAULT 1,
  shadow_only    INTEGER NOT NULL DEFAULT 0,
  expires_at     INTEGER NOT NULL DEFAULT 0,
  created_at     INTEGER NOT NULL DEFAULT 0,
  created_by     TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_sap_scope ON session_approval_policies(scope, scope_key, enabled, priority);
`)
	if err != nil {
		t.Fatalf("create session_approval_policies mirror DDL: %v", err)
	}
}

// TestApprovalPolicyRepo_Roundtrip 覆盖 PG 仓储读写回路：upsert 新建/更新 →
// GetByID → List（管理面全量，含 disabled/过期）→ ListEnabled（enabled+未过期
// 过滤 + priority 升序/同档 id 字典序）→ Delete 幂等 → NotFound。行语义钉死
// 迁移 20261291 列序（13 字段），批 C activation 的 wire/读路径依赖不漂移。
func TestApprovalPolicyRepo_Roundtrip(t *testing.T) {
	t.Parallel()
	d := openTestData(t)
	ensureApprovalPolicyTable(t, d)
	repo := NewApprovalPolicyRepo(d)
	ctx := context.Background()
	now := time.Now().Unix()

	p := biztool.ApprovalPolicy{
		ID:           "ap-test-1",
		Scope:        string(biztool.ApprovalScopeSession),
		ScopeKey:     "sess-rp-1",
		ToolPattern:  "*",
		Mode:         string(biztool.ApprovalModeAsk),
		Priority:     200,
		Enabled:      true,
		ShadowOnly:   false,
		ExpiresAt:    0,
		CreatedAt:    now,
		CreatedBy:    "test",
	}
	if err := repo.UpsertApprovalPolicy(ctx, p); err != nil {
		t.Fatalf("UpsertApprovalPolicy create: %v", err)
	}
	got, err := repo.GetApprovalPolicyByID(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetApprovalPolicyByID: %v", err)
	}
	if got.ScopeKey != p.ScopeKey || got.Mode != p.Mode || !got.Enabled || got.Priority != 200 {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}

	// 更新（同 ID upsert 覆盖）：模式 never + 过期时间已过期 → ListEnabled 过滤。
	p.Mode = string(biztool.ApprovalModeNever)
	p.ExpiresAt = now - 10
	if err := repo.UpsertApprovalPolicy(ctx, p); err != nil {
		t.Fatalf("UpsertApprovalPolicy update: %v", err)
	}
	admin, err := repo.ListApprovalPolicies(ctx, p.Scope, p.ScopeKey)
	if err != nil || len(admin) != 1 {
		t.Fatalf("ListApprovalPolicies = (%v,%v), want 1 row", len(admin), err)
	}
	enabled, err := repo.ListEnabledApprovalPolicies(ctx, p.Scope, p.ScopeKey, now)
	if err != nil || len(enabled) != 0 {
		t.Fatalf("expired rule must not be enabled, got (%v,%v)", len(enabled), err)
	}

	// 过期恢复 + 第二条同 scope 低 priority 规则 → ListEnabled 两枚且按 priority 排序。
	p.ExpiresAt = 0
	if err := repo.UpsertApprovalPolicy(ctx, p); err != nil {
		t.Fatalf("restore expiry: %v", err)
	}
	p2 := p
	p2.ID = "ap-test-2"
	p2.Priority = 50
	p2.Mode = string(biztool.ApprovalModeTrusted)
	if err := repo.UpsertApprovalPolicy(ctx, p2); err != nil {
		t.Fatalf("UpsertApprovalPolicy second: %v", err)
	}
	enabled, err = repo.ListEnabledApprovalPolicies(ctx, p.Scope, p.ScopeKey, now)
	if err != nil || len(enabled) != 2 {
		t.Fatalf("ListEnabledApprovalPolicies = (%v,%v), want 2 rows", len(enabled), err)
	}
	if enabled[0].ID != p2.ID || enabled[1].ID != p.ID {
		t.Fatalf("priority ordering mismatch: %s, %s", enabled[0].ID, enabled[1].ID)
	}
	// 作用域隔离：另一 scope_key 不可见。
	other, err := repo.ListEnabledApprovalPolicies(ctx, p.Scope, "sess-rp-other", now)
	if err != nil || len(other) != 0 {
		t.Fatalf("scope leak: got (%v,%v)", len(other), err)
	}

	if err := repo.DeleteApprovalPolicy(ctx, p.ID); err != nil {
		t.Fatalf("DeleteApprovalPolicy: %v", err)
	}
	if err := repo.DeleteApprovalPolicy(ctx, p.ID); err != nil {
		t.Fatalf("DeleteApprovalPolicy must be idempotent: %v", err)
	}
	if _, err := repo.GetApprovalPolicyByID(ctx, p.ID); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("deleted row must be NotFound, got %v", err)
	}
}

// TestApprovalPolicyRepo_UpsertRejectsInvalid 写路径单一校验函数拒收：
// 空 scope_key / 非法 mode / 坏 tool_pattern（与 usecase/仓储共用
// ValidateApprovalPolicy，防两套语义）。
func TestApprovalPolicyRepo_UpsertRejectsInvalid(t *testing.T) {
	t.Parallel()
	d := openTestData(t)
	ensureApprovalPolicyTable(t, d)
	repo := NewApprovalPolicyRepo(d)
	ctx := context.Background()
	base := biztool.ApprovalPolicy{
		ID: "ap-bad-1", Scope: string(biztool.ApprovalScopeSession), ScopeKey: "s",
		ToolPattern: "*", Mode: string(biztool.ApprovalModeAsk),
	}
	bad := base
	bad.Mode = "auto" // 第四档未立项（§6.8 defer③），写入期拒收
	if err := repo.UpsertApprovalPolicy(ctx, bad); err == nil {
		t.Fatal("invalid mode must be rejected")
	}
	bad = base
	bad.ToolPattern = "re:[unclosed"
	if err := repo.UpsertApprovalPolicy(ctx, bad); err == nil {
		t.Fatal("uncompilable tool_pattern must be rejected")
	}
	bad = base
	bad.Scope = "org"
	if err := repo.UpsertApprovalPolicy(ctx, bad); err == nil {
		t.Fatal("scope outside session|agent must be rejected")
	}
}

// TestApprovalPolicyRepo_NilDataSafe nil Data 兜底：读 nil、写 Internal 错。
func TestApprovalPolicyRepo_NilDataSafe(t *testing.T) {
	t.Parallel()
	repo := NewApprovalPolicyRepo(nil)
	ctx := context.Background()
	if rows, err := repo.ListApprovalPolicies(ctx, "session", "s"); rows != nil || err != nil {
		t.Fatalf("nil data List must be (nil,nil), got (%v,%v)", rows, err)
	}
	if err := repo.UpsertApprovalPolicy(ctx, biztool.ApprovalPolicy{}); err == nil {
		t.Fatal("nil data Upsert must report unavailable")
	}
}
