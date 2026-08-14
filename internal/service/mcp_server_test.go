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
