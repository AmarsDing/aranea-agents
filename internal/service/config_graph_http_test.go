package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	bizcg "aranea-agents/internal/biz/configgraph"
	"aranea-agents/pkg/auth"
)

// ── fakes ────────────────────────────────────────────────────────────────────

type fakeCGRebuilder struct {
	mu      sync.Mutex
	current int64
	ready   bool
	running bool
	async   []int64 // gens passed to RebuildAsync
	last    *bizcg.RebuildResult
	lastAt  time.Time
}

func (f *fakeCGRebuilder) Current() int64 { return f.current }
func (f *fakeCGRebuilder) Ready() bool    { return f.ready }
func (f *fakeCGRebuilder) Running() bool  { return f.running }

func (f *fakeCGRebuilder) RebuildAsync() (int64, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	gen := f.current + 1
	f.async = append(f.async, gen)
	return gen, true
}

func (f *fakeCGRebuilder) LastRebuild() (bizcg.RebuildResult, time.Time, bool) {
	if f.last == nil {
		return bizcg.RebuildResult{}, time.Time{}, false
	}
	return *f.last, f.lastAt, true
}

type fakeCGRepo struct {
	counts    bizcg.Counts
	countsErr error
	nodes     []bizcg.Node
	nodesErr  error
	lastFilt  bizcg.NodeFilter
}

func (f *fakeCGRepo) UpsertNodes(context.Context, []bizcg.Node) error     { return nil }
func (f *fakeCGRepo) UpsertEdges(context.Context, []bizcg.StoredEdge) error { return nil }
func (f *fakeCGRepo) MaxGeneration(context.Context) (int64, error)        { return 0, nil }
func (f *fakeCGRepo) DeleteGenerationBelow(context.Context, int64) (int64, error) {
	return 0, nil
}
func (f *fakeCGRepo) DeleteOutEdges(context.Context, string, int64) error { return nil }
func (f *fakeCGRepo) Counts(context.Context, int64) (bizcg.Counts, error) { return f.counts, f.countsErr }
func (f *fakeCGRepo) ListNodes(_ context.Context, filter bizcg.NodeFilter) ([]bizcg.Node, error) {
	f.lastFilt = filter
	return f.nodes, f.nodesErr
}

// ── helpers ──────────────────────────────────────────────────────────────────

func cgAdminCtx() context.Context {
	return auth.NewContext(context.Background(), &auth.Auth{UserID: 7, Access: "admin"})
}

func cgUserCtx() context.Context {
	return auth.NewContext(context.Background(), &auth.Auth{UserID: 7, Access: "user"})
}

func cgRequest(t *testing.T, method, target string, ctx context.Context) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, target, nil)
	if ctx != nil {
		req = req.WithContext(ctx)
	}
	return req
}

func cgDecode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("response not JSON: %v body=%q", err, rec.Body.String())
	}
	return out
}

func cgErrorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	body := cgDecode(t, rec)
	errObj, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("no error object in body: %v", body)
	}
	code, _ := errObj["code"].(string)
	return code
}

// ── tests ────────────────────────────────────────────────────────────────────

func TestConfigGraphService_NilDeps(t *testing.T) {
	if NewConfigGraphService(nil, &fakeCGRepo{}, nil) != nil {
		t.Fatal("nil rebuilder must yield nil service")
	}
	if NewConfigGraphService(&fakeCGRebuilder{}, nil, nil) != nil {
		t.Fatal("nil repo must yield nil service")
	}
}

func TestConfigGraphService_AuthGate(t *testing.T) {
	svc := NewConfigGraphService(&fakeCGRebuilder{current: 1, ready: true}, &fakeCGRepo{}, nil)

	// 无认证 → 401
	rec := httptest.NewRecorder()
	svc.ServeStatus(rec, cgRequest(t, http.MethodGet, "/api/v1/config-graph/status", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no-auth status = %d, want 401", rec.Code)
	}

	// 非 admin → 403（三端点同门禁，抽查 rebuild）
	rec = httptest.NewRecorder()
	svc.ServeRebuild(rec, cgRequest(t, http.MethodPost, "/api/v1/config-graph/rebuild", cgUserCtx()))
	if rec.Code != http.StatusForbidden || cgErrorCode(t, rec) != "CONFIG_GRAPH.FORBIDDEN" {
		t.Fatalf("user rebuild = %d %s", rec.Code, cgErrorCode(t, rec))
	}

	rec = httptest.NewRecorder()
	svc.ServeNodes(rec, cgRequest(t, http.MethodGet, "/api/v1/config-graph/nodes", cgUserCtx()))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("user nodes = %d, want 403", rec.Code)
	}
}

func TestConfigGraphService_Rebuild(t *testing.T) {
	rb := &fakeCGRebuilder{current: 3, ready: true}
	svc := NewConfigGraphService(rb, &fakeCGRepo{}, nil)

	rec := httptest.NewRecorder()
	svc.ServeRebuild(rec, cgRequest(t, http.MethodPost, "/api/v1/config-graph/rebuild", cgAdminCtx()))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("rebuild = %d, want 202", rec.Code)
	}
	body := cgDecode(t, rec)
	if body["generation"] != float64(4) || body["started"] != true {
		t.Fatalf("rebuild body = %v", body)
	}
	if len(rb.async) != 1 || rb.async[0] != 4 {
		t.Fatalf("async calls = %v", rb.async)
	}
}

func TestConfigGraphService_Status(t *testing.T) {
	at := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	rb := &fakeCGRebuilder{
		current: 2, ready: true,
		last:   &bizcg.RebuildResult{Generation: 2, Nodes: 100, Edges: 200, Broken: 3, Elapsed: 1500 * time.Millisecond},
		lastAt: at,
	}
	repo := &fakeCGRepo{counts: bizcg.Counts{Nodes: 100, Edges: 200, Broken: 3}}
	svc := NewConfigGraphService(rb, repo, nil)

	rec := httptest.NewRecorder()
	svc.ServeStatus(rec, cgRequest(t, http.MethodGet, "/api/v1/config-graph/status", cgAdminCtx()))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := cgDecode(t, rec)
	if body["ready"] != true || body["generation"] != float64(2) {
		t.Fatalf("status body = %v", body)
	}
	if body["nodes"] != float64(100) || body["edges"] != float64(200) || body["broken"] != float64(3) {
		t.Fatalf("status counts = %v", body)
	}
	last, ok := body["last_rebuild"].(map[string]any)
	if !ok || last["elapsed_ms"] != float64(1500) || last["at"] != "2026-08-26T12:00:00Z" {
		t.Fatalf("last_rebuild = %v", body["last_rebuild"])
	}
}

func TestConfigGraphService_StatusNotReady(t *testing.T) {
	svc := NewConfigGraphService(&fakeCGRebuilder{}, &fakeCGRepo{}, nil)
	rec := httptest.NewRecorder()
	svc.ServeStatus(rec, cgRequest(t, http.MethodGet, "/api/v1/config-graph/status", cgAdminCtx()))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (status must work pre-build)", rec.Code)
	}
	body := cgDecode(t, rec)
	if body["ready"] != false || body["generation"] != float64(0) {
		t.Fatalf("pre-build status = %v", body)
	}
	if _, has := body["nodes"]; has {
		t.Fatalf("pre-build status must omit counts: %v", body)
	}
}

func TestConfigGraphService_Nodes(t *testing.T) {
	rb := &fakeCGRebuilder{current: 2, ready: true}
	repo := &fakeCGRepo{nodes: []bizcg.Node{{
		ID: "n1", NodeType: bizcg.NodeTypeAgent, RefID: "a1", NodeKey: "ops",
		DisplayName: "Ops", WorkspaceID: "ws1", Status: bizcg.NodeStatusActive,
		Attrs: map[string]any{"kind": "react"},
	}}}
	svc := NewConfigGraphService(rb, repo, nil)

	rec := httptest.NewRecorder()
	svc.ServeNodes(rec, cgRequest(t, http.MethodGet,
		"/api/v1/config-graph/nodes?type=agent&key=ops&workspace=ws1&limit=50", cgAdminCtx()))
	if rec.Code != http.StatusOK {
		t.Fatalf("nodes = %d, want 200", rec.Code)
	}
	if repo.lastFilt.Generation != 2 || repo.lastFilt.NodeType != "agent" ||
		repo.lastFilt.KeyContains != "ops" || repo.lastFilt.WorkspaceID != "ws1" || repo.lastFilt.Limit != 50 {
		t.Fatalf("filter = %+v", repo.lastFilt)
	}
	body := cgDecode(t, rec)
	items, ok := body["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("items = %v", body["items"])
	}
	first := items[0].(map[string]any)
	if first["node_key"] != "ops" || first["node_type"] != "agent" {
		t.Fatalf("item = %v", first)
	}
}

func TestConfigGraphService_NodesNotReadyAndBadRequest(t *testing.T) {
	// 未建图 → 503 NOT_READY
	svc := NewConfigGraphService(&fakeCGRebuilder{}, &fakeCGRepo{}, nil)
	rec := httptest.NewRecorder()
	svc.ServeNodes(rec, cgRequest(t, http.MethodGet, "/api/v1/config-graph/nodes", cgAdminCtx()))
	if rec.Code != http.StatusServiceUnavailable || cgErrorCode(t, rec) != "CONFIG_GRAPH.NOT_READY" {
		t.Fatalf("pre-build nodes = %d %s", rec.Code, cgErrorCode(t, rec))
	}

	svc = NewConfigGraphService(&fakeCGRebuilder{current: 1, ready: true}, &fakeCGRepo{}, nil)
	// 非法 type → 400
	rec = httptest.NewRecorder()
	svc.ServeNodes(rec, cgRequest(t, http.MethodGet, "/api/v1/config-graph/nodes?type=bogus", cgAdminCtx()))
	if rec.Code != http.StatusBadRequest || cgErrorCode(t, rec) != "CONFIG_GRAPH.BAD_REQUEST" {
		t.Fatalf("bad type = %d %s", rec.Code, cgErrorCode(t, rec))
	}
	// 非法 limit → 400
	rec = httptest.NewRecorder()
	svc.ServeNodes(rec, cgRequest(t, http.MethodGet, "/api/v1/config-graph/nodes?limit=-3", cgAdminCtx()))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad limit = %d, want 400", rec.Code)
	}
	rec = httptest.NewRecorder()
	svc.ServeNodes(rec, cgRequest(t, http.MethodGet, "/api/v1/config-graph/nodes?limit=abc", cgAdminCtx()))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("non-numeric limit = %d, want 400", rec.Code)
	}
}
