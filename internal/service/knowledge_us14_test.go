package service

import (
	"context"
	"encoding/base64"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	v1 "aranea-agents/api/kratos/knowledge/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/knowledge"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/loggateway"
)

// ── US-14 in-memory knowledge repo（service 层测试专用） ──────────────────────

type us14MemRepo struct {
	mu          sync.Mutex
	collections map[string]biz.KnowledgeCollection
	documents   map[string]biz.KnowledgeDocument
	// lastSearchQ 捕获最近一次 SearchChunks 收到的查询（G3-B7 映射验证用）。
	lastSearchQ biz.KnowledgeSearchQuery
}

func newUS14MemRepo() *us14MemRepo {
	return &us14MemRepo{
		collections: make(map[string]biz.KnowledgeCollection),
		documents:   make(map[string]biz.KnowledgeDocument),
	}
}

func (m *us14MemRepo) CreateCollection(_ context.Context, c biz.KnowledgeCollection) (biz.KnowledgeCollection, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.collections[c.ID] = c
	return c, nil
}

func (m *us14MemRepo) GetCollection(_ context.Context, id string) (biz.KnowledgeCollection, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.collections[id]
	if !ok {
		return biz.KnowledgeCollection{}, biz.ErrNotFound
	}
	return c, nil
}

func (m *us14MemRepo) ListCollections(_ context.Context, _ string, _, _ int) ([]biz.KnowledgeCollection, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]biz.KnowledgeCollection, 0, len(m.collections))
	for _, c := range m.collections {
		out = append(out, c)
	}
	return out, len(out), nil
}

func (m *us14MemRepo) DeleteCollection(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.collections, id)
	return nil
}

func (m *us14MemRepo) UpdateCollectionCounts(_ context.Context, id string, docD, chunkD int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.collections[id]
	c.DocumentCount += docD
	c.ChunkCount += chunkD
	m.collections[id] = c
	return nil
}

func (m *us14MemRepo) UpdateCollectionSyncState(_ context.Context, id, state string, lastSyncAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.collections[id]
	c.SyncState = state
	c.LastSyncAt = lastSyncAt.UTC().Format(time.RFC3339)
	m.collections[id] = c
	return nil
}

func (m *us14MemRepo) CreateDocument(_ context.Context, d biz.KnowledgeDocument) (biz.KnowledgeDocument, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.documents[d.ID] = d
	return d, nil
}

func (m *us14MemRepo) GetDocument(_ context.Context, id string) (biz.KnowledgeDocument, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.documents[id]
	if !ok {
		return biz.KnowledgeDocument{}, biz.ErrNotFound
	}
	return d, nil
}

func (m *us14MemRepo) GetDocumentByRelPath(_ context.Context, collectionID, relPath string) (biz.KnowledgeDocument, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, d := range m.documents {
		if d.CollectionID == collectionID && d.RelPath == relPath {
			return d, nil
		}
	}
	return biz.KnowledgeDocument{}, biz.ErrNotFound
}

func (m *us14MemRepo) UpdateDocumentRelPath(_ context.Context, id, newRelPath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	d := m.documents[id]
	d.RelPath = newRelPath
	m.documents[id] = d
	return nil
}

func (m *us14MemRepo) UpdateDocumentSyncMeta(_ context.Context, id string, meta biz.KnowledgeDocumentSyncMeta) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	d := m.documents[id]
	d.ContentHash = meta.ContentHash
	d.Summary = meta.Summary
	d.SummaryHash = meta.SummaryHash
	d.Tags = meta.Tags
	d.DocType = meta.DocType
	m.documents[id] = d
	return nil
}

func (m *us14MemRepo) UpdateDocumentStatus(_ context.Context, id, status, errMsg string, cc int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	d := m.documents[id]
	d.Status = status
	d.ErrorMessage = errMsg
	d.ChunkCount = cc
	m.documents[id] = d
	return nil
}

func (m *us14MemRepo) UpdateDocumentContent(_ context.Context, id, contentText string, organized bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	d := m.documents[id]
	d.ContentText = contentText
	d.Organized = organized
	m.documents[id] = d
	return nil
}

func (m *us14MemRepo) ListDocuments(_ context.Context, collectionID string, _, _ int) ([]biz.KnowledgeDocument, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []biz.KnowledgeDocument
	for _, d := range m.documents {
		if d.CollectionID == collectionID {
			out = append(out, d)
		}
	}
	return out, len(out), nil
}

// ListDocumentsPendingReembed B1 内存版：mem 无 chunks 概念，退化为「有正文且非 indexing」
// （排序按 CreatedAt ASC，RFC3339 字典序即时间序）。
func (m *us14MemRepo) ListDocumentsPendingReembed(_ context.Context, collectionID string) ([]biz.KnowledgeDocument, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []biz.KnowledgeDocument
	for _, d := range m.documents {
		if d.CollectionID == collectionID && d.ContentText != "" && d.Status != "indexing" {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt < out[j].CreatedAt })
	return out, nil
}

func (m *us14MemRepo) DeleteDocument(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.documents, id)
	return nil
}

func (m *us14MemRepo) MoveDocument(_ context.Context, id, targetCollectionID string) (biz.KnowledgeDocument, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.documents[id]
	if !ok {
		return biz.KnowledgeDocument{}, biz.ErrNotFound
	}
	d.CollectionID = targetCollectionID
	m.documents[id] = d
	return d, nil
}

func (m *us14MemRepo) InsertChunks(context.Context, []biz.KnowledgeChunk) error { return nil }
func (m *us14MemRepo) DeleteChunksByDocument(context.Context, string) error     { return nil }
func (m *us14MemRepo) SearchChunks(_ context.Context, q biz.KnowledgeSearchQuery, _ []float32) ([]biz.KnowledgeChunk, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastSearchQ = q
	return nil, nil
}

// ── US-14 stubs ───────────────────────────────────────────────────────────────

type us14EmbedderAdmin struct {
	model string
	dim   int
}

func (a *us14EmbedderAdmin) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = []float32{0.1, 0.2, 0.3}
	}
	return out, nil
}
func (a *us14EmbedderAdmin) Dim() int { return a.dim }
func (a *us14EmbedderAdmin) Config() (provider, baseURL, model string, dim int, configured bool, hasAPIKey bool) {
	return "openai", "", a.model, a.dim, true, true
}
func (a *us14EmbedderAdmin) Update(_, _, _, _ string, _ int) {}

type us14QueryEmbedder struct{}

func (us14QueryEmbedder) EmbedSingle(context.Context, string) ([]float32, error) {
	return []float32{0.1, 0.2, 0.3}, nil
}

// ── US-14 规则 1：上传免预选 —— collection_id 留空落入默认知识库 ─────────────

func TestIngestDocument_EmptyCollection_FallsIntoDefaultCollection(t *testing.T) {
	repo := newUS14MemRepo()
	uc := biz.NewKnowledgeUsecase(repo, repo, repo)
	emb := &us14EmbedderAdmin{model: "text-embedding-3-small", dim: 1536}
	svc := NewKnowledgeService(uc, emb, KnowledgeSearchDeps{}, nil, nil, nil, nil, nil, loggateway.NewNoop())

	raw := base64.StdEncoding.EncodeToString([]byte("hello default collection"))
	doc, err := svc.IngestDocument(context.Background(), &v1.IngestDocumentRequest{
		CollectionId:  "",
		Source:        "notes.txt",
		MimeType:      "text/plain",
		ContentBase64: raw,
	})
	if err != nil {
		t.Fatalf("ingest with empty collection_id must succeed via default collection, got %v", err)
	}
	if doc.GetCollectionId() == "" {
		t.Fatal("document must be attached to the lazily-created default collection")
	}
	col, err := repo.GetCollection(context.Background(), doc.GetCollectionId())
	if err != nil {
		t.Fatalf("default collection must exist: %v", err)
	}
	if col.Name != "默认知识库" {
		t.Errorf("default collection name = %q, want 默认知识库", col.Name)
	}
	if col.Dim != 1536 {
		t.Errorf("default collection dim = %d, want 1536 (from embedder config)", col.Dim)
	}
}

// 已存在默认知识库时复用，不重复创建。
func TestIngestDocument_EmptyCollection_ReusesExistingDefaultCollection(t *testing.T) {
	repo := newUS14MemRepo()
	uc := biz.NewKnowledgeUsecase(repo, repo, repo)
	// 生产路径由 service RPC 盖章 workspace（C-01）；测试直调 usecase 需模拟该盖章，
	// 否则空 workspace 被 assertCollectionMutateAccess 视为共享只读。
	existing, err := uc.CreateCollection(context.Background(), biz.KnowledgeCollection{
		Name:           "默认知识库",
		EmbeddingModel: "text-embedding-3-small",
		Dim:            1536,
		Workspace:      workspace.DefaultWorkspaceID,
	})
	if err != nil {
		t.Fatal(err)
	}
	emb := &us14EmbedderAdmin{model: "text-embedding-3-small", dim: 1536}
	svc := NewKnowledgeService(uc, emb, KnowledgeSearchDeps{}, nil, nil, nil, nil, nil, loggateway.NewNoop())

	raw := base64.StdEncoding.EncodeToString([]byte("reuse me"))
	doc, err := svc.IngestDocument(context.Background(), &v1.IngestDocumentRequest{
		CollectionId:  "",
		Source:        "again.txt",
		MimeType:      "text/plain",
		ContentBase64: raw,
	})
	if err != nil {
		t.Fatalf("ingest must reuse existing default collection, got %v", err)
	}
	if doc.GetCollectionId() != existing.ID {
		t.Errorf("document landed in %q, want existing default collection %q", doc.GetCollectionId(), existing.ID)
	}
	cols, total, err := repo.ListCollections(context.Background(), "", 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 {
		t.Errorf("expected exactly 1 collection (reused), got %d: %+v", total, cols)
	}
}

// 显式 collection_id 行为不变。
func TestIngestDocument_ExplicitCollection_Unchanged(t *testing.T) {
	repo := newUS14MemRepo()
	uc := biz.NewKnowledgeUsecase(repo, repo, repo)
	col, err := uc.CreateCollection(context.Background(), biz.KnowledgeCollection{
		Name:           "quant",
		EmbeddingModel: "text-embedding-3-small",
		Dim:            1536,
		Workspace:      workspace.DefaultWorkspaceID, // 模拟 service RPC 的 C-01 盖章
	})
	if err != nil {
		t.Fatal(err)
	}
	emb := &us14EmbedderAdmin{model: "text-embedding-3-small", dim: 1536}
	svc := NewKnowledgeService(uc, emb, KnowledgeSearchDeps{}, nil, nil, nil, nil, nil, loggateway.NewNoop())

	raw := base64.StdEncoding.EncodeToString([]byte("explicit collection"))
	doc, err := svc.IngestDocument(context.Background(), &v1.IngestDocumentRequest{
		CollectionId:  col.ID,
		Source:        "explicit.txt",
		MimeType:      "text/plain",
		ContentBase64: raw,
	})
	if err != nil {
		t.Fatalf("explicit collection ingest failed: %v", err)
	}
	if doc.GetCollectionId() != col.ID {
		t.Errorf("document landed in %q, want explicit %q", doc.GetCollectionId(), col.ID)
	}
}

// ── US-14 规则 2：检索免选择 —— Search collection_id 留空走全库智能路由 ──────

func TestSearch_EmptyCollection_RoutesAcrossAllCollections(t *testing.T) {
	repo := newUS14MemRepo()
	uc := biz.NewKnowledgeUsecase(repo, repo, repo)
	if _, err := uc.CreateCollection(context.Background(), biz.KnowledgeCollection{
		ID: "col-refund", Name: "refund policy", EmbeddingModel: "m", Dim: 3,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := uc.CreateCollection(context.Background(), biz.KnowledgeCollection{
		ID: "col-shipping", Name: "shipping info", EmbeddingModel: "m", Dim: 3,
	}); err != nil {
		t.Fatal(err)
	}

	retriever := knowledge.NewRetriever(us14QueryEmbedder{}, repo, nil, loggateway.NewNoop())
	federated := knowledge.NewFederatedRetrieverWithMeta(nil, retriever, repo, loggateway.NewNoop())
	svc := NewKnowledgeService(uc, nil, KnowledgeSearchDeps{
		Retriever: retriever,
		Federated: federated,
	}, nil, nil, nil, nil, nil, loggateway.NewNoop())

	resp, err := svc.Search(context.Background(), &v1.SearchRequest{
		CollectionId: "",
		Query:        "refund policy",
	})
	if err != nil {
		t.Fatalf("collection-free search must not error, got %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
}

// 系统无任何 Collection 时返回空结果而非错误（LLM 可无知识继续回答）。
func TestSearch_EmptyCollection_ZeroCollections_ReturnsEmpty(t *testing.T) {
	repo := newUS14MemRepo()
	uc := biz.NewKnowledgeUsecase(repo, repo, repo)
	retriever := knowledge.NewRetriever(us14QueryEmbedder{}, repo, nil, loggateway.NewNoop())
	federated := knowledge.NewFederatedRetrieverWithMeta(nil, retriever, repo, loggateway.NewNoop())
	svc := NewKnowledgeService(uc, nil, KnowledgeSearchDeps{
		Retriever: retriever,
		Federated: federated,
	}, nil, nil, nil, nil, nil, loggateway.NewNoop())

	resp, err := svc.Search(context.Background(), &v1.SearchRequest{Query: "anything"})
	if err != nil {
		t.Fatalf("zero-collection search must not error, got %v", err)
	}
	if len(resp.GetChunks()) != 0 {
		t.Errorf("expected 0 chunks, got %d", len(resp.GetChunks()))
	}
}

// 显式 collection_id 行为不变（单库检索）。
func TestSearch_ExplicitCollection_Unchanged(t *testing.T) {
	repo := newUS14MemRepo()
	uc := biz.NewKnowledgeUsecase(repo, repo, repo)
	// C-01 前置校验要求显式 collection 存在；workspace="" 为共享库，读放行。
	if _, err := uc.CreateCollection(context.Background(), biz.KnowledgeCollection{
		ID: "col-x", Name: "x", EmbeddingModel: "m", Dim: 3,
	}); err != nil {
		t.Fatal(err)
	}
	retriever := knowledge.NewRetriever(us14QueryEmbedder{}, repo, nil, loggateway.NewNoop())
	federated := knowledge.NewFederatedRetrieverWithMeta(nil, retriever, repo, loggateway.NewNoop())
	svc := NewKnowledgeService(uc, nil, KnowledgeSearchDeps{
		Retriever: retriever,
		Federated: federated,
	}, nil, nil, nil, nil, nil, loggateway.NewNoop())

	_, err := svc.Search(context.Background(), &v1.SearchRequest{
		CollectionId: "col-x",
		Query:        "test",
	})
	if err != nil {
		t.Fatalf("explicit collection search must not error, got %v", err)
	}
}

// ── G3-B7：SearchRequest.path_prefix → SearchQuery.PathPrefix 映射 ───────────

func TestSearch_PathPrefixPlumbed(t *testing.T) {
	repo := newUS14MemRepo()
	uc := biz.NewKnowledgeUsecase(repo, repo, repo)
	if _, err := uc.CreateCollection(context.Background(), biz.KnowledgeCollection{
		ID: "col-x", Name: "x", EmbeddingModel: "m", Dim: 3,
	}); err != nil {
		t.Fatal(err)
	}
	retriever := knowledge.NewRetriever(us14QueryEmbedder{}, repo, nil, loggateway.NewNoop())
	svc := NewKnowledgeService(uc, nil, KnowledgeSearchDeps{
		Retriever: retriever,
	}, nil, nil, nil, nil, nil, loggateway.NewNoop())

	if _, err := svc.Search(context.Background(), &v1.SearchRequest{
		CollectionId: "col-x",
		Query:        "test",
		PathPrefix:   "notes/guides",
	}); err != nil {
		t.Fatalf("search with path_prefix must not error, got %v", err)
	}
	repo.mu.Lock()
	got := repo.lastSearchQ.PathPrefix
	repo.mu.Unlock()
	if got != "notes/guides" {
		t.Errorf("PathPrefix = %q, want %q（proto → biz 映射丢失）", got, "notes/guides")
	}
}

// ── US-14 规则 4：MoveDocument RPC —— 文档跨库移动 ───────────────────────────

func TestMoveDocument_Success(t *testing.T) {
	repo := newUS14MemRepo()
	uc := biz.NewKnowledgeUsecase(repo, repo, repo)
	src, err := uc.CreateCollection(context.Background(), biz.KnowledgeCollection{
		Name: "默认知识库", EmbeddingModel: "m", Dim: 1536,
	})
	if err != nil {
		t.Fatal(err)
	}
	dst, err := uc.CreateCollection(context.Background(), biz.KnowledgeCollection{
		Name: "quant", EmbeddingModel: "m", Dim: 1536,
	})
	if err != nil {
		t.Fatal(err)
	}
	doc, err := uc.CreateDocument(context.Background(), biz.KnowledgeDocument{
		CollectionID: src.ID,
		Source:       "report.txt",
		MimeType:     "text/plain",
	})
	if err != nil {
		t.Fatal(err)
	}

	svc := NewKnowledgeService(uc, nil, KnowledgeSearchDeps{}, nil, nil, nil, nil, nil, loggateway.NewNoop())
	moved, err := svc.MoveDocument(context.Background(), &v1.MoveDocumentRequest{
		Id:                 doc.ID,
		TargetCollectionId: dst.ID,
	})
	if err != nil {
		t.Fatalf("MoveDocument failed: %v", err)
	}
	if moved.GetCollectionId() != dst.ID {
		t.Errorf("document collection = %q, want %q", moved.GetCollectionId(), dst.ID)
	}
}

// 目标库 dim 不一致 → CodeConflict（向量维度不兼容，禁止移动）。
func TestMoveDocument_DimMismatch_ReturnsConflict(t *testing.T) {
	repo := newUS14MemRepo()
	uc := biz.NewKnowledgeUsecase(repo, repo, repo)
	src, err := uc.CreateCollection(context.Background(), biz.KnowledgeCollection{
		Name: "默认知识库", EmbeddingModel: "m", Dim: 1536,
	})
	if err != nil {
		t.Fatal(err)
	}
	dst, err := uc.CreateCollection(context.Background(), biz.KnowledgeCollection{
		Name: "other-dim", EmbeddingModel: "m", Dim: 768,
	})
	if err != nil {
		t.Fatal(err)
	}
	doc, err := uc.CreateDocument(context.Background(), biz.KnowledgeDocument{
		CollectionID: src.ID,
		Source:       "report.txt",
		MimeType:     "text/plain",
	})
	if err != nil {
		t.Fatal(err)
	}

	svc := NewKnowledgeService(uc, nil, KnowledgeSearchDeps{}, nil, nil, nil, nil, nil, loggateway.NewNoop())
	_, err = svc.MoveDocument(context.Background(), &v1.MoveDocumentRequest{
		Id:                 doc.ID,
		TargetCollectionId: dst.ID,
	})
	if err == nil {
		t.Fatal("expected dim-mismatch conflict error, got nil")
	}
	if !strings.Contains(err.Error(), "dimension") {
		t.Errorf("error should mention dimension mismatch, got %v", err)
	}
}
