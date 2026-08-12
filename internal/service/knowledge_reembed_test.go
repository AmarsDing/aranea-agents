package service

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	v1 "aranea-agents/api/kratos/knowledge/v1"
	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"github.com/stretchr/testify/require"
)

// ── B1 ReembedDocuments 测试替身 ─────────────────────────────────────────────

// reembedRepo 内嵌 us14MemRepo（复用集合/文档内存实现），覆写 chunk 端口记录调用。
type reembedRepo struct {
	*us14MemRepo
	mu          sync.Mutex
	deletedDocs []string
	inserted    []biz.KnowledgeChunk
}

func newReembedRepo() *reembedRepo {
	return &reembedRepo{us14MemRepo: newUS14MemRepo()}
}

func (r *reembedRepo) DeleteChunksByDocument(_ context.Context, docID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deletedDocs = append(r.deletedDocs, docID)
	return nil
}

func (r *reembedRepo) InsertChunks(_ context.Context, chunks []biz.KnowledgeChunk) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.inserted = append(r.inserted, chunks...)
	return nil
}

func (r *reembedRepo) deletedCount(docID string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, id := range r.deletedDocs {
		if id == docID {
			n++
		}
	}
	return n
}

func (r *reembedRepo) insertedFor(docID string) []biz.KnowledgeChunk {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []biz.KnowledgeChunk
	for _, c := range r.inserted {
		if c.DocID == docID {
			out = append(out, c)
		}
	}
	return out
}

// reembedBlockSpy 块索引重建探针：重嵌入链路禁止触发 RebuildBlockIndex
// （content_text 未变，块/边不变——与摄取链路区分）。接线后任何 ReplaceDocBlocks
// 调用即视为违规。
type reembedBlockSpy struct {
	mu           sync.Mutex
	replaceCalls int
}

func (s *reembedBlockSpy) ReplaceDocBlocks(context.Context, string, string, []bizknowledge.KnowledgeBlock, []bizknowledge.KnowledgeBlockRefInput) ([]bizknowledge.KnowledgeBlockRefEdge, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.replaceCalls++
	return nil, nil
}

func (s *reembedBlockSpy) ListDocBlocks(context.Context, string) ([]bizknowledge.KnowledgeBlock, error) {
	return nil, nil
}

func (s *reembedBlockSpy) UpdateDocLinkKeys(context.Context, string, string, []string) error {
	return nil
}

func (s *reembedBlockSpy) ListDocsMissingBlockIndex(context.Context, string, int) ([]string, error) {
	return nil, nil
}

func (s *reembedBlockSpy) replaced() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.replaceCalls
}

// reembedStubEmbedder 固定向量 stub（无网络）。
type reembedStubEmbedder struct{}

func (reembedStubEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = []float32{0.1, 0.2, 0.3}
	}
	return out, nil
}

func (reembedStubEmbedder) Dim() int { return 3 }

// ── 构造辅助 ─────────────────────────────────────────────────────────────────

func newReembedService(repo *reembedRepo) (*KnowledgeService, *reembedBlockSpy) {
	uc := biz.NewKnowledgeUsecase(repo, repo, repo)
	spy := &reembedBlockSpy{}
	uc.SetBlockIndexRepos(spy, nil)
	return &KnowledgeService{uc: uc, embedder: reembedStubEmbedder{}, lg: loggateway.NewNoop()}, spy
}

func reembedTenantCtx() context.Context {
	return workspace.WithContext(context.Background(), "ws-1")
}

func seedReembedCollection(t *testing.T, repo *reembedRepo, id, ws, model string) {
	t.Helper()
	_, err := repo.CreateCollection(context.Background(), biz.KnowledgeCollection{
		ID: id, Name: id, Workspace: ws, EmbeddingModel: model,
	})
	require.NoError(t, err)
}

func seedReembedDocument(t *testing.T, repo *reembedRepo, id, colID, content, status string) {
	t.Helper()
	_, err := repo.CreateDocument(context.Background(), biz.KnowledgeDocument{
		ID: id, CollectionID: colID, Source: id + ".md",
		ContentText: content, Status: status, CreatedAt: "2026-08-01T00:00:00Z",
	})
	require.NoError(t, err)
}

// ── 测试用例 ─────────────────────────────────────────────────────────────────

// 词法库（embedding_model 为空）无语义层，重嵌入无意义 → BAD_REQUEST。
func TestReembedDocuments_LexicalCollectionRejected(t *testing.T) {
	repo := newReembedRepo()
	seedReembedCollection(t, repo, "c-lex", "ws-1", "")
	svc, _ := newReembedService(repo)

	_, err := svc.ReembedDocuments(reembedTenantCtx(), &v1.ReembedDocumentsRequest{CollectionId: "c-lex"})
	require.Error(t, err)
	if !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("err = %v, want BAD_REQUEST", err)
	}
}

// 共享集合（workspace 空）对租户 caller 只读（fail-closed）→ NOT_FOUND（防存在性泄漏）。
func TestReembedDocuments_MutateAccessDenied(t *testing.T) {
	repo := newReembedRepo()
	seedReembedCollection(t, repo, "c-shared", "", "m")
	svc, _ := newReembedService(repo)

	_, err := svc.ReembedDocuments(reembedTenantCtx(), &v1.ReembedDocumentsRequest{CollectionId: "c-shared"})
	require.Error(t, err)
	if !apierror.IsCode(err, apierror.CodeNotFound) {
		t.Fatalf("err = %v, want NOT_FOUND", err)
	}
}

// 显式 doc_ids：正文空 / indexing / 不存在 / 跨集合 均计 skipped，正常文档 accepted。
func TestReembedDocuments_ExplicitDocIdsSkipsRules(t *testing.T) {
	repo := newReembedRepo()
	seedReembedCollection(t, repo, "c1", "ws-1", "m")
	seedReembedCollection(t, repo, "c2", "ws-1", "m")
	seedReembedDocument(t, repo, "d-ok", "c1", "hello reembed", "indexed")
	seedReembedDocument(t, repo, "d-empty", "c1", "", "error")
	seedReembedDocument(t, repo, "d-indexing", "c1", "body", "indexing")
	seedReembedDocument(t, repo, "d-foreign", "c2", "foreign body", "indexed")
	svc, _ := newReembedService(repo)

	resp, err := svc.ReembedDocuments(reembedTenantCtx(), &v1.ReembedDocumentsRequest{
		CollectionId: "c1",
		DocIds:       []string{"d-ok", "d-empty", "d-indexing", "d-missing", "d-foreign"},
	})
	require.NoError(t, err)
	if resp.GetAcceptedCount() != 1 || resp.GetSkippedCount() != 4 {
		t.Fatalf("accepted=%d skipped=%d, want 1/4", resp.GetAcceptedCount(), resp.GetSkippedCount())
	}
	// 后台串行管线收敛：d-ok 终态 indexed。
	require.Eventually(t, func() bool {
		d, getErr := repo.GetDocument(context.Background(), "d-ok")
		return getErr == nil && d.Status == "indexed" && d.ChunkCount > 0
	}, 3*time.Second, 10*time.Millisecond)
	// 跳过项不得进入管线。
	for _, id := range []string{"d-empty", "d-indexing", "d-foreign"} {
		if n := repo.deletedCount(id); n != 0 {
			t.Fatalf("skipped doc %s DeleteChunksByDocument 被调用 %d 次", id, n)
		}
	}
}

// doc_ids 空 → 走 ListDocumentsPendingReembed（有正文且非 indexing），accepted = 待重嵌入数。
func TestReembedDocuments_DefaultSelectsPending(t *testing.T) {
	repo := newReembedRepo()
	seedReembedCollection(t, repo, "c1", "ws-1", "m")
	seedReembedDocument(t, repo, "d-p1", "c1", "pending one", "error")
	seedReembedDocument(t, repo, "d-p2", "c1", "pending two", "indexed")
	seedReembedDocument(t, repo, "d-skip-status", "c1", "body", "indexing")
	seedReembedDocument(t, repo, "d-skip-empty", "c1", "", "error")
	svc, _ := newReembedService(repo)

	resp, err := svc.ReembedDocuments(reembedTenantCtx(), &v1.ReembedDocumentsRequest{CollectionId: "c1"})
	require.NoError(t, err)
	if resp.GetAcceptedCount() != 2 || resp.GetSkippedCount() != 0 {
		t.Fatalf("accepted=%d skipped=%d, want 2/0", resp.GetAcceptedCount(), resp.GetSkippedCount())
	}
	require.Eventually(t, func() bool {
		d1, e1 := repo.GetDocument(context.Background(), "d-p1")
		d2, e2 := repo.GetDocument(context.Background(), "d-p2")
		return e1 == nil && e2 == nil && d1.Status == "indexed" && d2.Status == "indexed"
	}, 3*time.Second, 10*time.Millisecond)
}

// 管线语义：以已存 content_text 为正文源重建 chunks+embedding（无需原始文件）；
// DeleteChunksByDocument 先清旧块；禁止触发 RebuildBlockIndex。
func TestReembedDocuments_PipelineReembedsFromContentText(t *testing.T) {
	repo := newReembedRepo()
	seedReembedCollection(t, repo, "c1", "ws-1", "m")
	const content = "重嵌入正文源：UI 上传文档仅存 content_text，无原始文件。"
	// status=error 是维度对账置 NULL 后的典型态（embed_fail 降级）。
	seedReembedDocument(t, repo, "d1", "c1", content, "error")
	svc, spy := newReembedService(repo)

	resp, err := svc.ReembedDocuments(reembedTenantCtx(), &v1.ReembedDocumentsRequest{CollectionId: "c1"})
	require.NoError(t, err)
	if resp.GetAcceptedCount() != 1 {
		t.Fatalf("accepted=%d, want 1", resp.GetAcceptedCount())
	}
	require.Eventually(t, func() bool {
		d, getErr := repo.GetDocument(context.Background(), "d1")
		return getErr == nil && d.Status == "indexed" && d.ChunkCount > 0
	}, 3*time.Second, 10*time.Millisecond)

	// 旧块先被清除（NULL 向量块由新块替换）。
	if n := repo.deletedCount("d1"); n == 0 {
		t.Fatal("DeleteChunksByDocument 未被调用：旧 chunks 必须先清除再重建")
	}
	// 新块正文源自 content_text，向量来自 embedder（语义层重建）。
	chunks := repo.insertedFor("d1")
	if len(chunks) == 0 {
		t.Fatal("InsertChunks 未捕获任何块")
	}
	if !strings.Contains(chunks[0].Content, "重嵌入正文源") {
		t.Fatalf("chunk content = %q, 应源自文档 content_text", chunks[0].Content)
	}
	if len(chunks[0].Embedding) != 3 {
		t.Fatalf("chunk embedding dim = %d, want 3（stub embedder）", len(chunks[0].Embedding))
	}
	// content_text 未变 → 块/边不变 → 禁止触发块级索引重建（与摄取链路区分）。
	if n := spy.replaced(); n != 0 {
		t.Fatalf("ReplaceDocBlocks 被调用 %d 次：重嵌入禁止触发 RebuildBlockIndex", n)
	}
}
