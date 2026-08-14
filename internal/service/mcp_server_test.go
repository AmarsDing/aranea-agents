package service_test

import (
	"context"
	"testing"
	"time"

	mcpv1 "aranea-agents/api/kratos/mcp_server/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/service"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/auth"
	"aranea-agents/pkg/loggateway"
)

// --- fakes -----------------------------------------------------------------

type fakeMCPRepo struct {
	servers map[string]biz.MCPServer
	// lastListQuery records the query passed to ListMCPServersPaged so tests
	// can assert request-message params reached the usecase.
	lastListQuery biz.MCPListQuery
	pagedCalled   bool
}

func (r *fakeMCPRepo) ListMCPServers(_ context.Context, q biz.MCPListQuery) ([]biz.MCPServer, error) {
	out := make([]biz.MCPServer, 0, len(r.servers))
	for _, s := range r.servers {
		out = append(out, s)
	}
	return out, nil
}

func (r *fakeMCPRepo) ListMCPServersPaged(_ context.Context, q biz.MCPListQuery) (biz.MCPListResult, error) {
	r.pagedCalled = true
	r.lastListQuery = q
	return biz.MCPListResult{Items: []biz.MCPServer{}, Total: 0, Limit: q.Limit, Offset: q.Offset}, nil
}

func (r *fakeMCPRepo) GetMCPServer(_ context.Context, id string) (biz.MCPServer, error) {
	if s, ok := r.servers[id]; ok {
		return s, nil
	}
	return biz.MCPServer{}, apierror.NotFound("MCP_SERVER", "mcp server not found")
}

func (r *fakeMCPRepo) GetMCPServerByKey(_ context.Context, key string) (biz.MCPServer, error) {
	for _, s := range r.servers {
		if s.Key == key {
			return s, nil
		}
	}
	return biz.MCPServer{}, apierror.NotFound("MCP_SERVER", "mcp server not found")
}

func (r *fakeMCPRepo) CreateMCPServer(_ context.Context, m biz.MCPServer) (biz.MCPServer, error) {
	return m, nil
}

func (r *fakeMCPRepo) UpdateMCPServer(_ context.Context, m biz.MCPServer) (biz.MCPServer, error) {
	r.servers[m.ID] = m
	return m, nil
}

func (r *fakeMCPRepo) DeleteMCPServer(_ context.Context, _ string) error { return nil }

func (r *fakeMCPRepo) UpdateMCPServerMetadata(_ context.Context, id, metadataJSON, status string) error {
	s := r.servers[id]
	s.MetadataJSON = metadataJSON
	if status != "" {
		s.Status = status
	}
	r.servers[id] = s
	return nil
}

func (r *fakeMCPRepo) UpdateMCPServerConfigJSON(_ context.Context, id, configJSON string) error {
	s := r.servers[id]
	s.ConfigJSON = configJSON
	r.servers[id] = s
	return nil
}

type fakeMCPProber struct{}

func (fakeMCPProber) Evaluate(_ context.Context, _ bool, _ string) biz.MCPTestResult {
	return biz.MCPTestResult{OK: true, Status: "ok", Message: "connected"}
}

type fakeMCPMetaEdit struct{}

func (fakeMCPMetaEdit) Parse(raw string) map[string]any { return map[string]any{} }
func (fakeMCPMetaEdit) Marshal(m map[string]any) (string, error) {
	return `{"health_status":"ok"}`, nil
}
func (fakeMCPMetaEdit) ApplyHealth(m map[string]any, _ string, _ bool, _ string, _ time.Time) (map[string]any, string) {
	return m, "active"
}
func (fakeMCPMetaEdit) ApplyReconnect(m map[string]any, _ time.Time) map[string]any { return m }
func (fakeMCPMetaEdit) MarkHealthAlert(m map[string]any, _ time.Time) map[string]any {
	return m
}

func newTestMCPServerService(repo *fakeMCPRepo) *service.MCPServerService {
	uc := biz.NewMCPServerUsecase(repo, nil, fakeMCPProber{}, fakeMCPMetaEdit{}, nil)
	return service.NewMCPServerService(uc, nil, nil, nil)
}

// fakeMCPCredRepo records user-credential writes for assertions.
type fakeMCPCredRepo struct {
	upserted []biz.MCPServerUserCredential
	deleted  bool
}

func (r *fakeMCPCredRepo) ListMCPServerUserCredentials(_ context.Context, _, _ string) ([]biz.MCPServerUserCredential, error) {
	return nil, nil
}

func (r *fakeMCPCredRepo) UpsertMCPServerUserCredential(_ context.Context, cred biz.MCPServerUserCredential) (biz.MCPServerUserCredential, error) {
	r.upserted = append(r.upserted, cred)
	return cred, nil
}

func (r *fakeMCPCredRepo) DeleteMCPServerUserCredential(_ context.Context, _, _, _ string) error {
	r.deleted = true
	return nil
}

func newTestMCPCredentialService(repo *fakeMCPRepo, credRepo *fakeMCPCredRepo) *service.MCPServerService {
	crypto := biz.NewCredentialCrypto(func(context.Context) ([]byte, error) { return make([]byte, 32), nil }, loggateway.NewNoop())
	uc := biz.NewMCPServerUsecase(repo, credRepo, fakeMCPProber{}, fakeMCPMetaEdit{}, crypto)
	return service.NewMCPServerService(uc, nil, nil, nil)
}

// --- tests -----------------------------------------------------------------

// Regression: the UI 测试连接 button 404'd for shared/built-in servers
// (workspace_id="") because TestMCPServer used the mutate-level IDOR guard,
// which fail-closes on shared resources. Probing is a read-class operation
// (it only refreshes system health bookkeeping metadata, same as the
// background health runner), so tenant callers must be allowed.
func TestMCPServerService_TestMCPServer_SharedServerAllowedForTenant(t *testing.T) {
	repo := &fakeMCPRepo{servers: map[string]biz.MCPServer{
		"shared-1": {ID: "shared-1", Key: "playwright", Name: "Playwright", Enabled: true, WorkspaceID: ""},
	}}
	svc := newTestMCPServerService(repo)

	ctx := workspace.WithContext(context.Background(), "default")
	res, err := svc.TestMCPServer(ctx, &mcpv1.TestMCPServerRequest{Id: "shared-1"})
	if err != nil {
		t.Fatalf("tenant probing shared server must succeed, got %v", err)
	}
	if !res.GetOk() {
		t.Fatalf("expected ok=true, got %+v", res)
	}
}

func TestMCPServerService_TestMCPServer_CrossTenantRejected(t *testing.T) {
	repo := &fakeMCPRepo{servers: map[string]biz.MCPServer{
		"priv-1": {ID: "priv-1", Key: "priv", Enabled: true, WorkspaceID: "ws-owner"},
	}}
	svc := newTestMCPServerService(repo)

	ctx := workspace.WithContext(context.Background(), "ws-attacker")
	_, err := svc.TestMCPServer(ctx, &mcpv1.TestMCPServerRequest{Id: "priv-1"})
	if err == nil {
		t.Fatal("cross-tenant probe must be rejected")
	}
	if !apierror.IsCode(err, apierror.CodeNotFound) {
		t.Fatalf("cross-tenant probe must surface as NotFound (existence hidden), got %v", err)
	}
}

func TestMCPServerService_TestMCPServer_OwnServerAllowed(t *testing.T) {
	repo := &fakeMCPRepo{servers: map[string]biz.MCPServer{
		"own-1": {ID: "own-1", Key: "own", Enabled: true, WorkspaceID: "default"},
	}}
	svc := newTestMCPServerService(repo)

	ctx := workspace.WithContext(context.Background(), "default")
	if _, err := svc.TestMCPServer(ctx, &mcpv1.TestMCPServerRequest{Id: "own-1"}); err != nil {
		t.Fatalf("tenant probing own server must succeed, got %v", err)
	}
}

// Shared flag mapping: derived from workspace_id=="" without exposing the raw
// tenant ID to the UI.
func TestToProtoMCP_SharedFlag(t *testing.T) {
	if got := service.ToProtoMCP(biz.MCPServer{ID: "a", WorkspaceID: ""}); !got.Shared {
		t.Error("workspace_id=\"\" must map to Shared=true")
	}
	if got := service.ToProtoMCP(biz.MCPServer{ID: "b", WorkspaceID: "default"}); got.Shared {
		t.Error("workspace_id=\"default\" must map to Shared=false")
	}
}

// ListMCPServers must read page/page_size/search from the proto request
// message (the generated TS client serializes them into the query string;
// previously the rpc took google.protobuf.Empty and the params were silently
// dropped, breaking server-side pagination/search on the MCP page).
func TestMCPServerService_ListMCPServers_RequestMessageParams(t *testing.T) {
	repo := &fakeMCPRepo{servers: map[string]biz.MCPServer{}}
	svc := newTestMCPServerService(repo)

	ctx := workspace.WithContext(context.Background(), "default")
	_, err := svc.ListMCPServers(ctx, &mcpv1.ListMCPServersRequest{Page: 2, PageSize: 10, Search: "play"})
	if err != nil {
		t.Fatalf("ListMCPServers: %v", err)
	}
	if !repo.pagedCalled {
		t.Fatal("paged path must be used when request carries page params")
	}
	if repo.lastListQuery.Search != "play" {
		t.Errorf("Search = %q, want %q", repo.lastListQuery.Search, "play")
	}
	if repo.lastListQuery.Limit != 10 {
		t.Errorf("Limit = %d, want 10", repo.lastListQuery.Limit)
	}
	if repo.lastListQuery.Offset != 10 {
		t.Errorf("Offset = %d, want 10 (page 2, size 10)", repo.lastListQuery.Offset)
	}
}

// Zero page params keep the legacy unpaginated path (pickers / CLI / health).
func TestMCPServerService_ListMCPServers_UnpaginatedByDefault(t *testing.T) {
	repo := &fakeMCPRepo{servers: map[string]biz.MCPServer{
		"s1": {ID: "s1", Key: "k1", WorkspaceID: ""},
	}}
	svc := newTestMCPServerService(repo)

	ctx := workspace.WithContext(context.Background(), "default")
	res, err := svc.ListMCPServers(ctx, &mcpv1.ListMCPServersRequest{})
	if err != nil {
		t.Fatalf("ListMCPServers: %v", err)
	}
	if repo.pagedCalled {
		t.Fatal("unpaginated path must be used when no page params present")
	}
	if len(res.GetItems()) != 1 {
		t.Fatalf("items = %d, want 1", len(res.GetItems()))
	}
	if !res.GetItems()[0].GetShared() {
		t.Error("shared server must carry Shared=true in list response")
	}
}

// Regression (2026-08-14 N1): upserting a per-user credential on a
// shared/built-in server (workspace_id="") 404'd for every HTTP caller
// because the RPC used the mutate-level IDOR guard, which fail-closes on
// shared resources — while the UI credentials button is enabled for exactly
// those rows (require_user_credentials). User credentials are the caller's
// own data (resolveMCPCredentialUserID binds non-admins to their own
// user_id), not the shared server's config, so the read-level guard is the
// correct classification — same as TestMCPServer.
func TestMCPServerService_UpsertCredential_SharedServerAllowedForTenant(t *testing.T) {
	repo := &fakeMCPRepo{servers: map[string]biz.MCPServer{
		"shared-1": {ID: "shared-1", Key: "playwright", Name: "Playwright", Enabled: true, WorkspaceID: ""},
	}}
	credRepo := &fakeMCPCredRepo{}
	svc := newTestMCPCredentialService(repo, credRepo)

	ctx := auth.NewContext(workspace.WithContext(context.Background(), "default"), &auth.Auth{UserID: 42, Access: "user"})
	_, err := svc.UpsertMCPServerUserCredential(ctx, &mcpv1.UpsertMCPServerUserCredentialRequest{
		McpServerId:   "shared-1",
		CredentialKey: "authorization",
		Secret:        "sk-test",
	})
	if err != nil {
		t.Fatalf("tenant upserting own credential on shared server must succeed, got %v", err)
	}
	if len(credRepo.upserted) != 1 {
		t.Fatalf("credential must be persisted, upserted=%d", len(credRepo.upserted))
	}
	if got := credRepo.upserted[0].UserID; got != "42" {
		t.Errorf("credential must bind to the caller's own user_id, got %q", got)
	}
}

func TestMCPServerService_DeleteCredential_SharedServerAllowedForTenant(t *testing.T) {
	repo := &fakeMCPRepo{servers: map[string]biz.MCPServer{
		"shared-1": {ID: "shared-1", Key: "playwright", Name: "Playwright", Enabled: true, WorkspaceID: ""},
	}}
	credRepo := &fakeMCPCredRepo{}
	svc := newTestMCPCredentialService(repo, credRepo)

	ctx := auth.NewContext(workspace.WithContext(context.Background(), "default"), &auth.Auth{UserID: 42, Access: "user"})
	_, err := svc.DeleteMCPServerUserCredential(ctx, &mcpv1.DeleteMCPServerUserCredentialRequest{
		McpServerId:   "shared-1",
		CredentialKey: "authorization",
	})
	if err != nil {
		t.Fatalf("tenant deleting own credential on shared server must succeed, got %v", err)
	}
	if !credRepo.deleted {
		t.Fatal("credential delete must reach the repo")
	}
}

// Cross-tenant guard must still hold on the read-level classification:
// a tenant cannot configure credentials on another tenant's PRIVATE server
// (the workspace-scoped Get already hides it → NotFound).
func TestMCPServerService_UpsertCredential_CrossTenantRejected(t *testing.T) {
	repo := &fakeMCPRepo{servers: map[string]biz.MCPServer{
		"priv-1": {ID: "priv-1", Key: "priv", Enabled: true, WorkspaceID: "ws-owner"},
	}}
	credRepo := &fakeMCPCredRepo{}
	svc := newTestMCPCredentialService(repo, credRepo)

	ctx := auth.NewContext(workspace.WithContext(context.Background(), "ws-attacker"), &auth.Auth{UserID: 42, Access: "user"})
	_, err := svc.UpsertMCPServerUserCredential(ctx, &mcpv1.UpsertMCPServerUserCredentialRequest{
		McpServerId:   "priv-1",
		CredentialKey: "authorization",
		Secret:        "sk-test",
	})
	if err == nil {
		t.Fatal("cross-tenant credential upsert must be rejected")
	}
	if !apierror.IsCode(err, apierror.CodeNotFound) {
		t.Fatalf("cross-tenant upsert must surface as NotFound (existence hidden), got %v", err)
	}
	if len(credRepo.upserted) != 0 {
		t.Fatal("rejected upsert must not reach the repo")
	}
}

// Regression (2026-08-14 N2): Update must not write the system-managed
// metadata_json/status columns. The admin form round-trips the row snapshot
// (status + metadata_json read at dialog-open time), so a full-row update
// rolls back every health-probe/reconnect bookkeeping write made while the
// operator was editing. Those two fields are owned by the health runner /
// metadata writer paths only.
func TestMCPServerService_UpdateMCPServer_IgnoresSystemManagedFields(t *testing.T) {
	repo := &fakeMCPRepo{servers: map[string]biz.MCPServer{
		"s1": {
			ID: "s1", Key: "k1", Name: "old", Enabled: true, WorkspaceID: "default",
			Status:       "error",
			MetadataJSON: `{"health_status":"error","reconnect_count":3}`,
		},
	}}
	svc := newTestMCPServerService(repo)

	ctx := auth.NewContext(workspace.WithContext(context.Background(), "default"), &auth.Auth{UserID: 1, Access: "admin"})
	_, err := svc.UpdateMCPServer(ctx, &mcpv1.UpdateMCPServerRequest{
		Id: "s1",
		McpServer: &mcpv1.MCPServer{
			Name:         "renamed",
			Enabled:      true,
			Status:       "active", // stale form snapshot — must be ignored
			MetadataJson: `{}`,     // stale form snapshot — must be ignored
		},
	})
	if err != nil {
		t.Fatalf("UpdateMCPServer: %v", err)
	}
	got := repo.servers["s1"]
	if got.Name != "renamed" {
		t.Errorf("name not updated: %q", got.Name)
	}
	if got.Status != "error" {
		t.Errorf("status is system-managed and must not be overwritten by update, got %q", got.Status)
	}
	if got.MetadataJSON != `{"health_status":"error","reconnect_count":3}` {
		t.Errorf("metadata_json must survive admin update, got %q", got.MetadataJSON)
	}
}
