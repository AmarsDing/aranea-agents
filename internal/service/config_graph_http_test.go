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
	"aranea-agents/pkg/apierror"
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
	// P1 查询 fake 行为。
	target    bizcg.Node
	targetErr error
	walkRows  []bizcg.WalkRow
	outEdges  []bizcg.StoredEdge
	inEdges   []bizcg.StoredEdge
	brokenOwn []bizcg.StoredEdge
	allNodes  []bizcg.Node
	allEdges  []bizcg.StoredEdge
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
func (f *fakeCGRepo) ListBrokenEdgesTargeting(context.Context, int64, []string) ([]bizcg.StoredEdge, error) {
	return nil, nil
}
func (f *fakeCGRepo) CountActiveSessions(context.Context, []string, []string) (int64, error) {
	return 0, nil
}
func (f *fakeCGRepo) FindNode(_ context.Context, _ int64, _, _ string) (bizcg.Node, error) {
	if f.targetErr != nil {
		return bizcg.Node{}, f.targetErr
	}
	return f.target, nil
}
func (f *fakeCGRepo) WalkGraph(context.Context, int64, string, bool, int) ([]bizcg.WalkRow, error) {
	return f.walkRows, nil
}
func (f *fakeCGRepo) ListNodeEdges(context.Context, int64, string) ([]bizcg.StoredEdge, []bizcg.StoredEdge, []bizcg.StoredEdge, error) {
	return f.outEdges, f.inEdges, f.brokenOwn, nil
}
func (f *fakeCGRepo) ListAllNodes(context.Context, int64) ([]bizcg.Node, error) {
	return f.allNodes, nil
}
func (f *fakeCGRepo) ListAllEdges(context.Context, int64) ([]bizcg.StoredEdge, error) {
	return f.allEdges, nil
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

// ── P1 查询端点 ────────────────────────────────────────────────────────────

var cgQueryTarget = bizcg.Node{
	ID: "n-tool", NodeType: bizcg.NodeTypeTool, RefID: "uuid-tool", NodeKey: "shell",
	Attrs: map[string]any{"risk_level": "high"},
}

func TestConfigGraphService_Impact(t *testing.T) {
	repo := &fakeCGRepo{
		target: cgQueryTarget,
		walkRows: []bizcg.WalkRow{
			{Edge: bizcg.StoredEdge{SrcID: "n-agent-a", DstID: "n-tool", Type: bizcg.EdgeTypeGrantedTool,
				Evidence: map[string]any{bizcg.EvidenceKeyGrantOrigin: bizcg.GrantOriginOverride}},
				Node:  bizcg.Node{ID: "n-agent-a", NodeType: bizcg.NodeTypeAgent, RefID: "uuid-agent-a", NodeKey: "agent-a"},
				Depth: 1, Via: []string{bizcg.EdgeTypeGrantedTool}},
		},
	}
	svc := NewConfigGraphService(&fakeCGRebuilder{current: 7, ready: true}, repo, nil)

	rec := httptest.NewRecorder()
	svc.ServeImpact(rec, cgRequest(t, http.MethodGet,
		"/api/v1/config-graph/nodes/tool/shell/impact?depth=3", cgAdminCtx()), "tool", "shell")
	if rec.Code != http.StatusOK {
		t.Fatalf("impact = %d body=%s", rec.Code, rec.Body.String())
	}
	body := cgDecode(t, rec)
	if body["generation"] != float64(7) {
		t.Fatalf("generation = %v", body["generation"])
	}
	nodes, ok := body["nodes"].([]any)
	if !ok || len(nodes) != 1 {
		t.Fatalf("nodes = %v", body["nodes"])
	}
	// risk：override 30 + 高危工具目标 20 = 50 → medium。
	risk, ok := body["risk"].(map[string]any)
	if !ok || risk["level"] != "medium" || risk["score"] != float64(50) {
		t.Fatalf("risk = %v", body["risk"])
	}
}

func TestConfigGraphService_ImpactErrors(t *testing.T) {
	svc := NewConfigGraphService(&fakeCGRebuilder{current: 7, ready: true}, &fakeCGRepo{target: cgQueryTarget}, nil)

	// 非 admin → 403。
	rec := httptest.NewRecorder()
	svc.ServeImpact(rec, cgRequest(t, http.MethodGet, "/x", cgUserCtx()), "tool", "shell")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("user impact = %d, want 403", rec.Code)
	}
	// 非法 type → 400。
	rec = httptest.NewRecorder()
	svc.ServeImpact(rec, cgRequest(t, http.MethodGet, "/x", cgAdminCtx()), "bogus", "shell")
	if rec.Code != http.StatusBadRequest || cgErrorCode(t, rec) != "CONFIG_GRAPH.BAD_REQUEST" {
		t.Fatalf("bad type = %d %s", rec.Code, cgErrorCode(t, rec))
	}
	// 空 ref → 400。
	rec = httptest.NewRecorder()
	svc.ServeImpact(rec, cgRequest(t, http.MethodGet, "/x", cgAdminCtx()), "tool", "  ")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty ref = %d, want 400", rec.Code)
	}
	// 非整数 depth → 400。
	rec = httptest.NewRecorder()
	svc.ServeImpact(rec, cgRequest(t, http.MethodGet, "/x?depth=abc", cgAdminCtx()), "tool", "shell")
	if rec.Code != http.StatusBadRequest || cgErrorCode(t, rec) != "CONFIG_GRAPH.BAD_REQUEST" {
		t.Fatalf("bad depth = %d %s", rec.Code, cgErrorCode(t, rec))
	}
	// 未建图 → 503 NOT_READY。
	svcNR := NewConfigGraphService(&fakeCGRebuilder{}, &fakeCGRepo{target: cgQueryTarget}, nil)
	rec = httptest.NewRecorder()
	svcNR.ServeImpact(rec, cgRequest(t, http.MethodGet, "/x", cgAdminCtx()), "tool", "shell")
	if rec.Code != http.StatusServiceUnavailable || cgErrorCode(t, rec) != "CONFIG_GRAPH.NOT_READY" {
		t.Fatalf("not ready = %d %s", rec.Code, cgErrorCode(t, rec))
	}
	// 目标不存在 → 404 NODE_NOT_FOUND。
	svcNF := NewConfigGraphService(&fakeCGRebuilder{current: 7, ready: true},
		&fakeCGRepo{targetErr: apierror.NotFound("CONFIG_GRAPH", "node not found")}, nil)
	rec = httptest.NewRecorder()
	svcNF.ServeImpact(rec, cgRequest(t, http.MethodGet, "/x", cgAdminCtx()), "tool", "ghost")
	if rec.Code != http.StatusNotFound || cgErrorCode(t, rec) != "CONFIG_GRAPH.NODE_NOT_FOUND" {
		t.Fatalf("not found = %d %s", rec.Code, cgErrorCode(t, rec))
	}
}

func TestConfigGraphService_Dependencies(t *testing.T) {
	repo := &fakeCGRepo{
		target: bizcg.Node{ID: "n-agent-a", NodeType: bizcg.NodeTypeAgent, RefID: "uuid-agent-a", NodeKey: "agent-a"},
		walkRows: []bizcg.WalkRow{
			{Edge: bizcg.StoredEdge{SrcID: "n-agent-a", DstID: "n-tool", Type: bizcg.EdgeTypeGrantedTool},
				Node:  bizcg.Node{ID: "n-tool", NodeType: bizcg.NodeTypeTool, RefID: "uuid-tool", NodeKey: "shell"},
				Depth: 1, Via: []string{bizcg.EdgeTypeGrantedTool}},
		},
	}
	svc := NewConfigGraphService(&fakeCGRebuilder{current: 7, ready: true}, repo, nil)

	rec := httptest.NewRecorder()
	svc.ServeDependencies(rec, cgRequest(t, http.MethodGet, "/x", cgAdminCtx()), "agent", "agent-a")
	if rec.Code != http.StatusOK {
		t.Fatalf("dependencies = %d body=%s", rec.Code, rec.Body.String())
	}
	body := cgDecode(t, rec)
	nodes, ok := body["nodes"].([]any)
	if !ok || len(nodes) != 1 {
		t.Fatalf("nodes = %v", body["nodes"])
	}
}

func TestConfigGraphService_NodeEdges(t *testing.T) {
	repo := &fakeCGRepo{
		target:    cgQueryTarget,
		outEdges:  []bizcg.StoredEdge{{ID: "e1", SrcID: "n-tool", DstID: "n-x", Type: bizcg.EdgeTypeHookRef}},
		inEdges:   []bizcg.StoredEdge{{ID: "e2", SrcID: "n-agent-a", DstID: "n-tool", Type: bizcg.EdgeTypeGrantedTool}},
		brokenOwn: []bizcg.StoredEdge{{ID: "e3", SrcID: "n-tool", Type: bizcg.EdgeTypeHookRef,
			Evidence: map[string]any{bizcg.EvidenceKeyBroken: true}}},
	}
	svc := NewConfigGraphService(&fakeCGRebuilder{current: 7, ready: true}, repo, nil)

	rec := httptest.NewRecorder()
	svc.ServeNodeEdges(rec, cgRequest(t, http.MethodGet, "/x", cgAdminCtx()), "tool", "shell")
	if rec.Code != http.StatusOK {
		t.Fatalf("edges = %d body=%s", rec.Code, rec.Body.String())
	}
	body := cgDecode(t, rec)
	for _, seg := range []string{"out", "in", "broken"} {
		rows, ok := body[seg].([]any)
		if !ok || len(rows) != 1 {
			t.Fatalf("%s = %v", seg, body[seg])
		}
	}
}

func TestConfigGraphService_Health(t *testing.T) {
	repo := &fakeCGRepo{
		allNodes: []bizcg.Node{
			{ID: "n-sa", NodeType: bizcg.NodeTypeSkill, RefID: "r-sa", NodeKey: "skill-a", Status: bizcg.NodeStatusActive},
			{ID: "n-sb", NodeType: bizcg.NodeTypeSkill, RefID: "r-sb", NodeKey: "skill-b", Status: bizcg.NodeStatusActive},
		},
		allEdges: []bizcg.StoredEdge{
			{ID: "e1", SrcID: "n-sa", DstID: "n-sb", Type: bizcg.EdgeTypeSkillParent, Evidence: map[string]any{}},
			{ID: "e2", SrcID: "n-sb", DstID: "n-sa", Type: bizcg.EdgeTypeSkillParent, Evidence: map[string]any{}},
		},
	}
	svc := NewConfigGraphService(&fakeCGRebuilder{current: 7, ready: true}, repo, nil)

	rec := httptest.NewRecorder()
	svc.ServeHealth(rec, cgRequest(t, http.MethodGet, "/api/v1/config-graph/health", cgAdminCtx()))
	if rec.Code != http.StatusOK {
		t.Fatalf("health = %d body=%s", rec.Code, rec.Body.String())
	}
	body := cgDecode(t, rec)
	if body["generation"] != float64(7) {
		t.Fatalf("generation = %v", body["generation"])
	}
	cycles, ok := body["cycles"].([]any)
	if !ok || len(cycles) != 1 {
		t.Fatalf("cycles = %v", body["cycles"])
	}
	// 四段键齐全（空段为 [] 而非缺失）。
	for _, key := range []string{"god_nodes", "broken_by_type", "duplicate_prompts"} {
		if _, has := body[key]; !has {
			t.Fatalf("missing key %s in %v", key, body)
		}
	}

	// 未建图 → 503。
	svcNR := NewConfigGraphService(&fakeCGRebuilder{}, &fakeCGRepo{}, nil)
	rec = httptest.NewRecorder()
	svcNR.ServeHealth(rec, cgRequest(t, http.MethodGet, "/x", cgAdminCtx()))
	if rec.Code != http.StatusServiceUnavailable || cgErrorCode(t, rec) != "CONFIG_GRAPH.NOT_READY" {
		t.Fatalf("health not ready = %d %s", rec.Code, cgErrorCode(t, rec))
	}
}
