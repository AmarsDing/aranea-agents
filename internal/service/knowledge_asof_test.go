package service

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/internal/knowledge"
	"aranea-agents/pkg/loggateway"
)

// ── G-05-asof（ADR-KN-20261020-K）：as-of 时态读面 custom HTTP 契约钉 ─────────
// 钉点：RFC3339 解析四象限/边界、span 透传进 biz.SearchQuery（经 Retriever →
// repo.SearchChunks 的 lastSearchQ 捕获）、图谱 handler 时态端口接线与 JSON 形状
// （snake_case，前端 pickStr 双制式映射直接复用）。

type stubAsOfGraphReader struct {
	calls         int
	gotCollection string
	gotTypes      []string
	gotSpan       bizknowledge.TemporalSpan
	links         []bizknowledge.Link
	err           error
}

func (s *stubAsOfGraphReader) ListCollectionLinksAsOf(_ context.Context, collectionID string, linkTypes []string, span bizknowledge.TemporalSpan) ([]bizknowledge.Link, error) {
	s.calls++
	s.gotCollection = collectionID
	s.gotTypes = linkTypes
	s.gotSpan = span
	return s.links, s.err
}

func newAsOfSearchService(t *testing.T, withRetriever bool) (*KnowledgeService, *us14MemRepo) {
	t.Helper()
	repo := newUS14MemRepo()
	uc := biz.NewKnowledgeUsecaseFromRepo(repo)
	deps := KnowledgeSearchDeps{}
	if withRetriever {
		deps.Retriever = knowledge.NewRetriever(us14QueryEmbedder{}, repo, nil, loggateway.NewNoop())
	}
	svc := NewKnowledgeService(uc, nil, deps, nil, nil, nil, nil, nil, nil, loggateway.NewNoop())
	// 带语义层的固定检索库（无 embedding_model 的词法库会走 BM25 降级面，
	// 与 us14 现有 fixture 同口径）。
	if _, err := repo.CreateCollection(context.Background(), biz.KnowledgeCollection{
		ID: "c1", Name: "n", EmbeddingModel: "m", Dim: 3,
	}); err != nil {
		t.Fatal(err)
	}
	return svc, repo
}

func asOfSearchRequest(t *testing.T, body string) (*httptest.ResponseRecorder, *http.Request) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/knowledge/search/asof", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	return rec, req
}

func TestServeSearchAsOf_SpanReachesQuery(t *testing.T) {
	svc, repo := newAsOfSearchService(t, true)
	validAt := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	recordedAt := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	rec, req := asOfSearchRequest(t, `{
		"collection_id": "c1", "query": "gateway timeout", "top_k": 5,
		"valid_at": "2026-10-01T00:00:00Z", "recorded_at": "2026-10-20T12:00:00Z"
	}`)
	svc.ServeSearchAsOf(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	// G-32 T4 钉迁：span 非零经 Retriever 时态 dense 腿断 ChunkTemporalReader
	// 端口（查询正文已向量化为 vec），不再经 repo.SearchChunks。
	got := repo.lastDenseAsOf
	if !got.Called {
		t.Fatal("span 非零必须触达时态 dense 腿（端口未接线将结构性 Unavailable）")
	}
	if got.CollectionID != "c1" || got.TopK != 5 || len(got.Vec) == 0 {
		t.Errorf("dense 腿查询字段 wrong（collection/topk/向量化查询）: %+v", got)
	}
	if !got.Span.ValidAt.Equal(validAt) || !got.Span.RecordedAt.Equal(recordedAt) {
		t.Errorf("Temporal = %+v, want (%v, %v)", got.Span, validAt, recordedAt)
	}
	if repo.lastSearchQ.Query != "" {
		t.Errorf("span 非零不得旁路 current 快报路径（SearchChunks 被触达 %+v）", repo.lastSearchQ)
	}
	// 空结果集 chunks 形状：[] 而非 null（前端 .map 契约）。
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if chunks, ok := resp["chunks"].([]any); !ok || chunks == nil {
		t.Errorf("chunks missing or wrong type: %#v", resp["chunks"])
	}
}

func TestServeSearchAsOf_SingleAxisAndZeroSpan(t *testing.T) {
	svc, repo := newAsOfSearchService(t, true)
	// 仅 T 轴：R 轴归零。
	rec, req := asOfSearchRequest(t, `{"collection_id":"c1","query":"q","valid_at":"2026-10-01T00:00:00Z"}`)
	svc.ServeSearchAsOf(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("T-only status = %d: %s", rec.Code, rec.Body.String())
	}
	if got := repo.lastDenseAsOf.Span; !got.ValidOn() || got.RecordedOn() {
		t.Errorf("T-only Temporal wrong: %+v", got)
	}
	// 双轴皆空 → 零 span（纯字符串空白同样按零处理）。
	rec, req = asOfSearchRequest(t, `{"collection_id":"c1","query":"q","valid_at":"  "}`)
	svc.ServeSearchAsOf(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("zero-span status = %d: %s", rec.Code, rec.Body.String())
	}
	if got := repo.lastSearchQ.Temporal; got.Any() {
		t.Errorf("zero-span Temporal should be zero, got %+v", got)
	}
}

func TestServeSearchAsOf_InvalidSpan_isBadRequest(t *testing.T) {
	svc, repo := newAsOfSearchService(t, true)
	for _, body := range []string{
		`{"collection_id":"c1","query":"q","valid_at":"not-a-time"}`,
		`{"collection_id":"c1","query":"q","recorded_at":"2026-10-20"}`,
		`{"collection_id":"c1","query":"q","valid_at":"2026-10-01T00:00:00"}`,
	} {
		rec, req := asOfSearchRequest(t, body)
		svc.ServeSearchAsOf(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("body %s: status = %d, want 400", body, rec.Code)
		}
	}
	// 语法校验先于一切存储/装配（与图谱腿 stub.calls==0 同款纪律）：
	// 错 span 的 400 必须发生在任何 repo 读之前。
	if repo.lastSearchQ.Query != "" || repo.lastSearchQ.CollectionID != "" {
		t.Errorf("invalid span must not reach SearchChunks, got %+v", repo.lastSearchQ)
	}
}

func TestServeSearchAsOf_ValidationEdges(t *testing.T) {
	svc, _ := newAsOfSearchService(t, true)
	// query 必填。
	rec, req := asOfSearchRequest(t, `{"collection_id":"c1","query":"  "}`)
	svc.ServeSearchAsOf(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("empty query: status = %d, want 400", rec.Code)
	}
	// 未知库 → 404（biz.ErrNotFound 映射）。
	rec, req = asOfSearchRequest(t, `{"collection_id":"nope","query":"q"}`)
	svc.ServeSearchAsOf(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown collection: status = %d, want 404", rec.Code)
	}
	// 未接线检索器 → 500（writeKnowledgeJSONError 单映射点：Unavailable 落 default，
	// 与既有 knowledge custom 路由语义一致，不为新面开第二套映射）。
	svcNoR, _ := newAsOfSearchService(t, false)
	rec, req = asOfSearchRequest(t, `{"query":"q"}`)
	svcNoR.ServeSearchAsOf(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("retriever nil: status = %d, want 500", rec.Code)
	}
}

// ── 图谱 as-of ─────────────────────────────────────────────────────────────

func newAsOfGraphFixture(t *testing.T, links []bizknowledge.Link) (*KnowledgeService, *stubAsOfGraphReader) {
	t.Helper()
	repo := newUS14MemRepo()
	uc := biz.NewKnowledgeUsecaseFromRepo(repo)
	stub := &stubAsOfGraphReader{links: links}
	uc.SetTemporalGraphRepo(stub)
	svc := NewKnowledgeService(uc, nil, KnowledgeSearchDeps{}, nil, nil, nil, nil, nil, nil, loggateway.NewNoop())
	ctx := context.Background()
	if _, err := repo.CreateCollection(ctx, biz.KnowledgeCollection{ID: "c1", Name: "n"}); err != nil {
		t.Fatal(err)
	}
	for _, d := range []biz.KnowledgeDocument{
		{ID: "d1", CollectionID: "c1", Source: "a.md", RelPath: "notes/a.md", DocType: "note"},
		{ID: "d2", CollectionID: "c1", Source: "b.md", RelPath: "notes/b.md", DocType: "note"},
		{ID: "d3", CollectionID: "c1", Source: "c.md", RelPath: "readme.md", DocType: "note"},
	} {
		if _, err := repo.CreateDocument(ctx, d); err != nil {
			t.Fatal(err)
		}
	}
	return svc, stub
}

func asOfGraphRequest(t *testing.T, rawQuery string) (*httptest.ResponseRecorder, *http.Request) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/knowledge/vaults/c1/graph/asof?"+rawQuery, nil)
	rec := httptest.NewRecorder()
	return rec, req
}

func TestServeCollectionGraphAsOf_SpanAndTypesReachPort(t *testing.T) {
	svc, stub := newAsOfGraphFixture(t, []bizknowledge.Link{
		{DocID: "d1", TargetDocID: "d2", LinkType: "explicit"},
		{DocID: "d1", TargetDocID: "ghost", LinkType: "explicit"}, // 悬空 → 剔除
	})
	rec, req := asOfGraphRequest(t, "valid_at=2026-10-01T00%3A00%3A00Z&link_types=explicit,semantic")
	svc.ServeCollectionGraphAsOf(rec, req, "c1")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if stub.calls != 1 {
		t.Fatalf("temporal port calls = %d, want 1", stub.calls)
	}
	if !stub.gotSpan.ValidOn() || stub.gotSpan.RecordedOn() {
		t.Errorf("span passthrough wrong: %+v", stub.gotSpan)
	}
	if len(stub.gotTypes) != 2 || stub.gotTypes[0] != "explicit" {
		t.Errorf("types passthrough wrong: %v", stub.gotTypes)
	}
	var resp struct {
		Nodes []struct {
			DocID   string `json:"doc_id"`
			RelPath string `json:"rel_path"`
			Degree  int    `json:"degree"`
		} `json:"nodes"`
		Edges []struct {
			Source string `json:"source"`
			Target string `json:"target"`
			Type   string `json:"type"`
		} `json:"edges"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Nodes) != 3 {
		t.Fatalf("nodes = %d, want 3", len(resp.Nodes))
	}
	if len(resp.Edges) != 1 || resp.Edges[0].Source != "d1" || resp.Edges[0].Target != "d2" {
		t.Fatalf("edges wrong: %+v", resp.Edges)
	}
}

func TestServeCollectionGraphAsOf_ZeroSpanSkipsTemporalPort(t *testing.T) {
	svc, stub := newAsOfGraphFixture(t, []bizknowledge.Link{
		{DocID: "d1", TargetDocID: "d2", LinkType: "explicit"},
	})
	rec, req := asOfGraphRequest(t, "")
	svc.ServeCollectionGraphAsOf(rec, req, "c1")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if stub.calls != 0 {
		t.Errorf("zero-span must byte-delegate to current path (graphLinks), temporal port called %d times", stub.calls)
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// graphLinks 未接线 → 仅节点无边（降级语义与 ListCollectionGraph 一致）。
	if edges, ok := resp["edges"].([]any); ok && len(edges) != 0 {
		t.Errorf("zero-span edges = %v, want empty", edges)
	}
	if nodes, ok := resp["nodes"].([]any); !ok || len(nodes) != 3 {
		t.Errorf("nodes wrong: %#v", resp["nodes"])
	}
}

func TestServeCollectionGraphAsOf_InvalidSpan_isBadRequest(t *testing.T) {
	svc, stub := newAsOfGraphFixture(t, nil)
	rec, req := asOfGraphRequest(t, "recorded_at=soon")
	svc.ServeCollectionGraphAsOf(rec, req, "c1")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if stub.calls != 0 {
		t.Error("invalid span must not reach the temporal port")
	}
}
