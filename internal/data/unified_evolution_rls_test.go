package data

import (
	"context"
	"sort"
	"strings"
	"testing"

	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"

	"github.com/lib/pq"
)

// 迁移 20261212（unified_evolution_workspace，2026-08-14 上线）RLS 回归。
// 钉住 policy tenant_workspace_isolation 的语义矩阵：
//   - 租户行仅归属租户可见（GUC app.workspace_id 匹配）
//   - workspace_id = ” 共享行全员可见
//   - GUC ∈ (”, '__system__') 全量可见（系统通道）
//   - GUC 未设置（NULL）仅共享行可见
//   - 写路径同策略：跨租户 UPDATE/DELETE 命中 0 行
//
// 迁移为 ENABLE only（无 FORCE），superuser/表主恒绕过 RLS，
// 故断言全部经非 owner 探测角色执行；建表/播种/迁移执行走 superuser 连接。
func TestUnifiedEvolutionRLSWorkspaceIsolation(t *testing.T) {
	client, db := testhelper.SetupTestPG(t)
	ctx := context.Background()

	if err := EnsureUnifiedEvolutionSchema(ctx, client); err != nil {
		t.Fatalf("ensure unified evolution schema: %v", err)
	}
	// 执行真实迁移产物（列/索引/backfill/ENABLE RLS/CREATE POLICY），
	// 而非在测试里镜像 SQL——迁移文件变更会直接反映到本测试。
	if err := ddlUnifiedEvolutionWorkspace(ctx, db, client, DialectPostgres, loggateway.NewNoop()); err != nil {
		t.Fatalf("apply migration 20261212: %v", err)
	}

	// 播种（superuser 绕过 RLS）：租户 A 行 / 租户 B 行 / 共享行。
	// target_id 逐行唯一——idx_ues_pending_target 对 (target_type,target_id) pending 行有唯一约束。
	for _, row := range []struct{ id, ws string }{
		{"rls-a1", "tenant-a"},
		{"rls-b1", "tenant-b"},
		{"rls-shared", ""},
	} {
		if _, err := db.ExecContext(ctx, `
			INSERT INTO unified_evolution_suggestions
			  (id, target_type, target_id, workspace_id, action_type, created_at)
			VALUES ($1, 'agent', $2, $3, 'create', '2026-08-15T00:00:00Z')`, row.id, "ag-"+row.id, row.ws); err != nil {
			t.Fatalf("seed %s: %v", row.id, err)
		}
	}

	// 非 owner 探测角色（角色是库级对象，按 schema 后缀唯一化避免并行冲突）。
	var schema string
	if err := db.QueryRowContext(ctx, `SELECT current_schema()`).Scan(&schema); err != nil {
		t.Fatalf("current_schema: %v", err)
	}
	probe := pq.QuoteIdentifier("rls_probe_" + strings.TrimPrefix(schema, "test_"))
	for _, stmt := range []string{
		`CREATE ROLE ` + probe + ` NOLOGIN`,
		`GRANT USAGE ON SCHEMA ` + pq.QuoteIdentifier(schema) + ` TO ` + probe,
		`GRANT SELECT, UPDATE, DELETE ON unified_evolution_suggestions TO ` + probe,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("setup probe role: %v (stmt: %s)", err, stmt)
		}
	}
	t.Cleanup(func() {
		// 先于 testhelper 的 schema drop 执行（LIFO），此时连接尚未关闭。
		cleanupCtx := context.Background()
		_, _ = db.ExecContext(cleanupCtx, `DROP OWNED BY `+probe)
		_, _ = db.ExecContext(cleanupCtx, `DROP ROLE `+probe)
	})

	// SET ROLE 与 GUC 都是会话级状态，必须钉在同一条专用连接上。
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("acquire conn: %v", err)
	}
	defer func() {
		resetCtx := context.Background()
		_, _ = conn.ExecContext(resetCtx, `RESET ROLE`)
		_, _ = conn.ExecContext(resetCtx, `SELECT set_config('app.workspace_id', '', false)`)
		_ = conn.Close()
	}()
	if _, err := conn.ExecContext(ctx, `SET ROLE `+probe); err != nil {
		t.Fatalf("set role: %v", err)
	}

	setWorkspace := func(t *testing.T, ws string) {
		t.Helper()
		if _, err := conn.ExecContext(ctx, `SELECT set_config('app.workspace_id', $1, false)`, ws); err != nil {
			t.Fatalf("set app.workspace_id=%q: %v", ws, err)
		}
	}
	visibleIDs := func(t *testing.T) []string {
		t.Helper()
		rows, err := conn.QueryContext(ctx, `SELECT id FROM unified_evolution_suggestions`)
		if err != nil {
			t.Fatalf("query visible ids: %v", err)
		}
		defer rows.Close()
		var ids []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				t.Fatalf("scan id: %v", err)
			}
			ids = append(ids, id)
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("iterate ids: %v", err)
		}
		sort.Strings(ids)
		return ids
	}
	assertVisible := func(t *testing.T, label string, want []string) {
		t.Helper()
		got := visibleIDs(t)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("%s: visible = %v, want %v", label, got, want)
		}
	}

	// 1. GUC 未设置（会话全新，current_setting(..., true) = NULL）→ 仅共享行。
	assertVisible(t, "guc unset", []string{"rls-shared"})

	// 2. 租户 A → 自有行 + 共享行；租户 B 行不可见。
	setWorkspace(t, "tenant-a")
	assertVisible(t, "tenant-a", []string{"rls-a1", "rls-shared"})

	// 3. 租户 B → 对称隔离。
	setWorkspace(t, "tenant-b")
	assertVisible(t, "tenant-b", []string{"rls-b1", "rls-shared"})

	// 4. 系统通道 → 全量可见。
	setWorkspace(t, "__system__")
	assertVisible(t, "__system__", []string{"rls-a1", "rls-b1", "rls-shared"})

	// 5. 空 GUC（未接线部署形态）→ 全量可见（与 20261011 phase1 同策略）。
	setWorkspace(t, "")
	assertVisible(t, "empty guc", []string{"rls-a1", "rls-b1", "rls-shared"})

	// 6. 写隔离：USING 子句同时约束 UPDATE/DELETE 的可见行集。
	setWorkspace(t, "tenant-a")
	res, err := conn.ExecContext(ctx, `UPDATE unified_evolution_suggestions SET status = 'x' WHERE id = 'rls-b1'`)
	if err != nil {
		t.Fatalf("cross-tenant update: %v", err)
	}
	if n, _ := res.RowsAffected(); n != 0 {
		t.Fatalf("cross-tenant update affected %d rows, want 0", n)
	}
	res, err = conn.ExecContext(ctx, `DELETE FROM unified_evolution_suggestions WHERE id = 'rls-b1'`)
	if err != nil {
		t.Fatalf("cross-tenant delete: %v", err)
	}
	if n, _ := res.RowsAffected(); n != 0 {
		t.Fatalf("cross-tenant delete affected %d rows, want 0", n)
	}
	res, err = conn.ExecContext(ctx, `UPDATE unified_evolution_suggestions SET status = 'done' WHERE id = 'rls-a1'`)
	if err != nil {
		t.Fatalf("own-tenant update: %v", err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		t.Fatalf("own-tenant update affected %d rows, want 1", n)
	}
}
