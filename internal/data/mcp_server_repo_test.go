package data

import (
	"context"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
)

// MCP 仓储 PG 集成测试（2026-10-18 审计整改 B7）。
// 覆盖 data 层此前零直测的 mcpServerRepo：软删可见性、workspace 租户过滤、
// UpdateMCPServer 只写管理列（不回写 status/metadata_json，N2 设计）、
// Delete 事务级联软删用户凭据、部分唯一索引下同 key 软删后重建。
// 运行依赖本地 PG（testhelper.SetupTestPG；ARANEA_TEST_PG_DSN 可覆盖）。

func mcpTestServer(id, key, workspaceID string) biz.MCPServer {
	return biz.MCPServer{
		ID:           id,
		Key:          key,
		Name:         "name-" + key,
		Description:  "desc-" + key,
		Status:       "active",
		Enabled:      true,
		SortOrder:    0,
		ConfigJSON:   `{"transport":"stdio","command":"x"}`,
		MetadataJSON: `{"health_status":"ok"}`,
		WorkspaceID:  workspaceID,
	}
}

func TestMCPServerRepo_WorkspaceVisibilityAndSearch(t *testing.T) {
	d := newTestDataPG(t)
	r := NewMCPServerRepo(d)
	ctx := context.Background()

	for _, m := range []biz.MCPServer{
		mcpTestServer("mcp-t-shared", "shared-srv", ""),
		mcpTestServer("mcp-t-ws1", "ws1-srv", "ws1"),
		mcpTestServer("mcp-t-ws2", "ws2-srv", "ws2"),
	} {
		if _, err := r.CreateMCPServer(ctx, m); err != nil {
			t.Fatalf("CreateMCPServer(%s): %v", m.Key, err)
		}
	}

	// 租户 ws1 视角：共享 + 本工作区，不可见 ws2。
	res, err := r.ListMCPServersPaged(ctx, biz.MCPListQuery{WorkspaceID: "ws1", Limit: 50})
	if err != nil {
		t.Fatalf("ListMCPServersPaged: %v", err)
	}
	keys := map[string]bool{}
	for _, item := range res.Items {
		keys[item.Key] = true
	}
	if !keys["shared-srv"] || !keys["ws1-srv"] {
		t.Fatalf("tenant ws1 should see shared+own, got %v", keys)
	}
	if keys["ws2-srv"] {
		t.Fatalf("tenant ws1 must not see ws2-srv, got %v", keys)
	}

	// 系统调用方（空 workspace）：全量可见。
	all, err := r.ListMCPServers(ctx, biz.MCPListQuery{})
	if err != nil {
		t.Fatalf("ListMCPServers: %v", err)
	}
	if len(all) < 3 {
		t.Fatalf("system caller should see all 3, got %d", len(all))
	}

	// 搜索大小写不敏感，且同样受 workspace 过滤约束。
	searchRes, err := r.ListMCPServersPaged(ctx, biz.MCPListQuery{WorkspaceID: "ws1", Search: "WS2-SRV", Limit: 50})
	if err != nil {
		t.Fatalf("ListMCPServersPaged search: %v", err)
	}
	if searchRes.Total != 0 {
		t.Fatalf("search for ws2-srv as ws1 must yield 0, got %d", searchRes.Total)
	}
	searchAll, err := r.ListMCPServersPaged(ctx, biz.MCPListQuery{Search: "shared-srv", Limit: 50})
	if err != nil {
		t.Fatalf("ListMCPServersPaged search all: %v", err)
	}
	if searchAll.Total != 1 {
		t.Fatalf("search shared-srv: want total=1, got %d", searchAll.Total)
	}
}

func TestMCPServerRepo_UpdateWritesOnlyAdminColumns(t *testing.T) {
	d := newTestDataPG(t)
	r := NewMCPServerRepo(d)
	ctx := context.Background()

	if _, err := r.CreateMCPServer(ctx, mcpTestServer("mcp-t-upd", "upd-srv", "ws1")); err != nil {
		t.Fatalf("CreateMCPServer: %v", err)
	}
	// 模拟健康巡检写字段级系统列。
	if err := r.UpdateMCPServerMetadata(ctx, "mcp-t-upd", `{"health_status":"error","last_error_message":"boom"}`, "error"); err != nil {
		t.Fatalf("UpdateMCPServerMetadata: %v", err)
	}

	// 管理面更新：调用方携带的是编辑前的陈旧快照（status=active、旧 metadata），
	// 落地后不得回滚健康巡检写入的系统列。
	stale := mcpTestServer("mcp-t-upd", "upd-srv", "ws1")
	stale.Description = "updated-desc"
	stale.ConfigJSON = `{"transport":"stdio","command":"y"}`
	if _, err := r.UpdateMCPServer(ctx, stale); err != nil {
		t.Fatalf("UpdateMCPServer: %v", err)
	}

	got, err := r.GetMCPServer(ctx, "mcp-t-upd")
	if err != nil {
		t.Fatalf("GetMCPServer: %v", err)
	}
	if got.Description != "updated-desc" || !strings.Contains(got.ConfigJSON, `"command":"y"`) {
		t.Fatalf("admin columns not updated: %+v", got)
	}
	if got.Status != "error" {
		t.Fatalf("status clobbered by admin update: got %q, want error", got.Status)
	}
	if !strings.Contains(got.MetadataJSON, `"health_status":"error"`) {
		t.Fatalf("metadata_json clobbered by admin update: got %q", got.MetadataJSON)
	}
}

func TestMCPServerRepo_DeleteCascadesCredentials(t *testing.T) {
	d := newTestDataPG(t)
	r := NewMCPServerRepo(d)
	creds := NewMCPServerUserCredentialRepo(d)
	ctx := context.Background()

	if _, err := r.CreateMCPServer(ctx, mcpTestServer("mcp-t-del", "del-srv", "ws1")); err != nil {
		t.Fatalf("CreateMCPServer: %v", err)
	}
	if _, err := creds.UpsertMCPServerUserCredential(ctx, biz.MCPServerUserCredential{
		ID:            "mcp-t-cred-1",
		MCPServerID:   "mcp-t-del",
		UserID:        "u1",
		CredentialKey: "Authorization",
		SecretRef:     "enc:test",
	}); err != nil {
		t.Fatalf("UpsertMCPServerUserCredential: %v", err)
	}

	if err := r.DeleteMCPServer(ctx, "mcp-t-del"); err != nil {
		t.Fatalf("DeleteMCPServer: %v", err)
	}

	// 删除后：本体不可见。
	if _, err := r.GetMCPServer(ctx, "mcp-t-del"); err == nil {
		t.Fatal("deleted server should not be readable")
	}
	// 级联：用户凭据一并软删。
	remaining, err := creds.ListMCPServerUserCredentials(ctx, "mcp-t-del", "")
	if err != nil {
		t.Fatalf("ListMCPServerUserCredentials: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("credentials should be cascade-soft-deleted, got %d", len(remaining))
	}
}

func TestMCPServerRepo_SoftDeleteAllowsRecreateSameKey(t *testing.T) {
	d := newTestDataPG(t)
	r := NewMCPServerRepo(d)
	ctx := context.Background()

	if _, err := r.CreateMCPServer(ctx, mcpTestServer("mcp-t-rc1", "recreate-srv", "ws1")); err != nil {
		t.Fatalf("CreateMCPServer: %v", err)
	}
	if err := r.DeleteMCPServer(ctx, "mcp-t-rc1"); err != nil {
		t.Fatalf("DeleteMCPServer: %v", err)
	}

	// 部分唯一索引（WHERE deleted_at = ''）只约束活跃行：同 key 重建必须成功。
	recreated, err := r.CreateMCPServer(ctx, mcpTestServer("mcp-t-rc2", "recreate-srv", "ws1"))
	if err != nil {
		t.Fatalf("recreate same key after soft delete: %v", err)
	}
	if recreated.ID == "mcp-t-rc1" {
		t.Fatal("recreated row must be a fresh row, not the soft-deleted one")
	}

	// 活跃行之间同 key 仍须冲突（唯一索引生效）。
	dup := mcpTestServer("mcp-t-rc3", "recreate-srv", "ws1")
	if _, err := r.CreateMCPServer(ctx, dup); err == nil {
		t.Fatal("duplicate active server_key must be rejected by partial unique index")
	}
}
