package service

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	v1 "aranea-agents/api/kratos/knowledge/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/knowledge"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/loggateway"
)

// ── G2-B6：GET /v1/knowledge/documents/{id}/asset 原始文件流 ─────────────────

func newAssetService(t *testing.T, assetRoot string) (*KnowledgeService, *us14MemRepo) {
	t.Helper()
	repo := newUS14MemRepo()
	uc := biz.NewKnowledgeUsecaseFromRepo(repo)
	uc.SetVaultFiler(biz.NewKnowledgeVaultFiler(nil))
	return NewKnowledgeService(uc, nil, KnowledgeSearchDeps{}, nil, nil, knowledge.NewAssetStore(assetRoot), nil, nil, nil, loggateway.NewNoop()), repo
}

// vault 图片：从 collection root + rel_path 流式输出，inline 渲染。
func TestKnowledgeService_ServeDocumentAsset_VaultImage(t *testing.T) {
	root := t.TempDir()
	svc, repo := newAssetService(t, t.TempDir())
	if _, err := repo.CreateCollection(context.Background(), biz.KnowledgeCollection{ID: "c1", Name: "vault", RootPath: root, Workspace: workspace.DefaultWorkspaceID}); err != nil {
		t.Fatal(err)
	}
	png := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A} // PNG magic
	if err := os.MkdirAll(filepath.Join(root, "pics"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "pics", "a.png"), png, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateDocument(context.Background(), biz.KnowledgeDocument{
		ID: "d1", CollectionID: "c1", RelPath: "pics/a.png", Source: "a.png", MimeType: "image/png", Status: "indexed",
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/knowledge/documents/d1/asset", nil)
	rec := httptest.NewRecorder()
	svc.ServeDocumentAsset(rec, req, "d1")

	res := rec.Result()
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); ct != "image/png" {
		t.Errorf("Content-Type = %q, want image/png", ct)
	}
	if cd := res.Header.Get("Content-Disposition"); !strings.HasPrefix(cd, "inline") || !strings.Contains(cd, "a.png") {
		t.Errorf("image must be inline with filename, got %q", cd)
	}
	body := rec.Body.Bytes()
	if len(body) != len(png) {
		t.Errorf("body len = %d, want %d", len(body), len(png))
	}
}

// 历史非 vault 文档：经 AssetStore 解析 asset_uri 输出。
func TestKnowledgeService_ServeDocumentAsset_LegacyAssetURI(t *testing.T) {
	assetRoot := t.TempDir()
	svc, repo := newAssetService(t, assetRoot)
	if _, err := repo.CreateCollection(context.Background(), biz.KnowledgeCollection{ID: "c1", Name: "col", Workspace: workspace.DefaultWorkspaceID}); err != nil {
		t.Fatal(err)
	}
	raw := []byte("fake-mp4-bytes")
	uri, err := knowledge.NewAssetStore(assetRoot).Save("d1", "clip.mp4", raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateDocument(context.Background(), biz.KnowledgeDocument{
		ID: "d1", CollectionID: "c1", Source: "clip.mp4", MimeType: "video/mp4", AssetURI: uri, Status: "indexed",
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/knowledge/documents/d1/asset", nil)
	rec := httptest.NewRecorder()
	svc.ServeDocumentAsset(rec, req, "d1")

	res := rec.Result()
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if cd := res.Header.Get("Content-Disposition"); !strings.HasPrefix(cd, "inline") {
		t.Errorf("video must be inline, got %q", cd)
	}
	if rec.Body.String() != string(raw) {
		t.Errorf("body = %q, want %q", rec.Body.String(), string(raw))
	}
}

// word 文档：attachment 下载（不内联）。
func TestKnowledgeService_ServeDocumentAsset_WordAttachment(t *testing.T) {
	root := t.TempDir()
	svc, repo := newAssetService(t, t.TempDir())
	if _, err := repo.CreateCollection(context.Background(), biz.KnowledgeCollection{ID: "c1", Name: "vault", RootPath: root, Workspace: workspace.DefaultWorkspaceID}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "report.docx"), []byte("docx"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateDocument(context.Background(), biz.KnowledgeDocument{
		ID: "d1", CollectionID: "c1", RelPath: "report.docx", Source: "report.docx",
		MimeType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document", Status: "indexed",
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/knowledge/documents/d1/asset", nil)
	rec := httptest.NewRecorder()
	svc.ServeDocumentAsset(rec, req, "d1")

	res := rec.Result()
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if cd := res.Header.Get("Content-Disposition"); !strings.HasPrefix(cd, "attachment") {
		t.Errorf("word must be attachment, got %q", cd)
	}
}

// 无 asset 的纯文本文档 → 404。
func TestKnowledgeService_ServeDocumentAsset_NoAsset(t *testing.T) {
	svc, repo := newAssetService(t, t.TempDir())
	if _, err := repo.CreateCollection(context.Background(), biz.KnowledgeCollection{ID: "c1", Name: "col", Workspace: workspace.DefaultWorkspaceID}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateDocument(context.Background(), biz.KnowledgeDocument{
		ID: "d1", CollectionID: "c1", Source: "a.txt", MimeType: "text/plain", Status: "indexed",
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/knowledge/documents/d1/asset", nil)
	rec := httptest.NewRecorder()
	svc.ServeDocumentAsset(rec, req, "d1")

	if rec.Result().StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Result().StatusCode)
	}
}

// 跨租户访问 → 404（C-01 防泄漏）。
func TestKnowledgeService_ServeDocumentAsset_CrossTenantDenied(t *testing.T) {
	root := t.TempDir()
	svc, repo := newAssetService(t, t.TempDir())
	if _, err := repo.CreateCollection(context.Background(), biz.KnowledgeCollection{ID: "c1", Name: "vault", RootPath: root, Workspace: "ws-other"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.png"), []byte{0x89}, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateDocument(context.Background(), biz.KnowledgeDocument{
		ID: "d1", CollectionID: "c1", RelPath: "a.png", Source: "a.png", MimeType: "image/png", Status: "indexed",
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/knowledge/documents/d1/asset", nil)
	req = req.WithContext(workspace.WithContext(req.Context(), "ws-mine"))
	rec := httptest.NewRecorder()
	svc.ServeDocumentAsset(rec, req, "d1")

	if rec.Result().StatusCode != http.StatusNotFound {
		t.Errorf("cross-tenant status = %d, want 404", rec.Result().StatusCode)
	}
}

// ── G-07 原件留存：AssetStore 从图片推广到全部可提取源 ──────────────────────

// 非 vault 的可提取源（文本/Office）入库即落 AssetStore，asset_uri 记录血缘——
// 页码锚定（G-07）需原件回跳。此前仅图片分支留存。
// 对抗审查 F10：asset 文件名必须由 doc ID 派生（sanitize(uuid)=uuid 原样）——
// doc.ID 提前生成缺失时每篇文档都落 <root>/asset.<ext> 互相覆盖（静默数据丢失），
// 两连传共存断言即捕获该回归。
func TestIngestDocument_AssetSavedForExtractableSource(t *testing.T) {
	assetRoot := t.TempDir()
	repo := newUS14MemRepo()
	uc := biz.NewKnowledgeUsecaseFromRepo(repo)
	svc := NewKnowledgeService(uc, nil, KnowledgeSearchDeps{}, nil,
		knowledge.NewExtractorRegistry(knowledge.NewTextExtractor()),
		knowledge.NewAssetStore(assetRoot), nil, nil, nil, loggateway.NewNoop())

	ingest := func(source, content string) (respID, uri string) {
		t.Helper()
		resp, err := svc.IngestDocument(context.Background(), &v1.IngestDocumentRequest{
			CollectionId:  "",
			Source:        source,
			MimeType:      "text/plain",
			ContentBase64: base64.StdEncoding.EncodeToString([]byte(content)),
		})
		if err != nil {
			t.Fatalf("ingest extractable source must succeed: %v", err)
		}
		doc, err := repo.GetDocument(context.Background(), resp.GetId())
		if err != nil {
			t.Fatal(err)
		}
		if doc.AssetURI == "" {
			t.Fatalf("extractable non-vault doc must carry asset_uri (G-07), doc %+v", doc)
		}
		return resp.GetId(), doc.AssetURI
	}

	raw1 := "hello G-07 original asset"
	id1, uri1 := ingest("notes.txt", raw1)
	if uri1 != id1+".txt" {
		t.Fatalf("asset name must derive from doc id (F10), got %q want %q", uri1, id1+".txt")
	}
	id2, uri2 := ingest("more.txt", "second doc bytes")
	if uri2 == uri1 || uri2 != id2+".txt" {
		t.Fatalf("second doc must not clobber first asset, uri1=%q uri2=%q", uri1, uri2)
	}
	saved, err := os.ReadFile(filepath.Join(assetRoot, uri1))
	if err != nil {
		t.Fatalf("asset file must exist under root: %v", err)
	}
	if string(saved) != raw1 {
		t.Fatalf("asset bytes mismatch: %q want %q", saved, raw1)
	}
	if _, err := os.Stat(filepath.Join(assetRoot, uri2)); err != nil {
		t.Fatalf("second asset file must coexist: %v", err)
	}
}

// vault 上传（target_dir）经 rel_path 已持原件镜像，不双份落 AssetStore。
func TestIngestDocument_VaultUploadSkipsAssetStore(t *testing.T) {
	assetRoot := t.TempDir()
	root := t.TempDir()
	repo := newUS14MemRepo()
	uc := biz.NewKnowledgeUsecaseFromRepo(repo)
	uc.SetVaultFiler(biz.NewKnowledgeVaultFiler(nil))
	uc.SetVaultApplier(&stubVaultDocApplier{repo: repo})
	svc := NewKnowledgeService(uc, nil, KnowledgeSearchDeps{}, nil,
		knowledge.NewExtractorRegistry(knowledge.NewTextExtractor()),
		knowledge.NewAssetStore(assetRoot), nil, nil, nil, loggateway.NewNoop())
	if _, err := repo.CreateCollection(context.Background(), biz.KnowledgeCollection{
		ID: "c1", Name: "vault", RootPath: root, Workspace: workspace.DefaultWorkspaceID,
	}); err != nil {
		t.Fatal(err)
	}

	resp, err := svc.IngestDocument(context.Background(), &v1.IngestDocumentRequest{
		CollectionId:  "c1",
		Source:        "guide.md",
		MimeType:      "text/markdown",
		TargetDir:     "docs",
		ContentBase64: base64.StdEncoding.EncodeToString([]byte("# guide\ncontent")),
	})
	if err != nil {
		t.Fatalf("vault-target ingest must succeed: %v", err)
	}
	doc, err := repo.GetDocument(context.Background(), resp.GetId())
	if err != nil {
		t.Fatal(err)
	}
	if doc.AssetURI != "" {
		t.Fatalf("vault doc must not double-store into AssetStore, got %q", doc.AssetURI)
	}
	if doc.RelPath == "" {
		t.Fatal("vault doc must carry rel_path (original mirror)")
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(doc.RelPath))); err != nil {
		t.Fatalf("vault original file must exist: %v", err)
	}
	entries, err := os.ReadDir(assetRoot)
	if err == nil && len(entries) != 0 {
		t.Fatalf("asset root must stay empty for vault uploads, got %d entries", len(entries))
	}
}

// 对抗审查 F9：asset 先落盘后 CreateDocument 失败须补偿删除（不留孤儿文件）。
type failingCreateRepo struct {
	*us14MemRepo
	err error
}

func (r *failingCreateRepo) CreateDocument(context.Context, biz.KnowledgeDocument) (biz.KnowledgeDocument, error) {
	return biz.KnowledgeDocument{}, r.err
}

func TestIngestDocument_AssetCompensatedOnCreateFailure(t *testing.T) {
	assetRoot := t.TempDir()
	repo := &failingCreateRepo{us14MemRepo: newUS14MemRepo(), err: errors.New("db down")}
	uc := biz.NewKnowledgeUsecaseFromRepo(repo)
	svc := NewKnowledgeService(uc, nil, KnowledgeSearchDeps{}, nil,
		knowledge.NewExtractorRegistry(knowledge.NewTextExtractor()),
		knowledge.NewAssetStore(assetRoot), nil, nil, nil, loggateway.NewNoop())

	_, err := svc.IngestDocument(context.Background(), &v1.IngestDocumentRequest{
		CollectionId:  "",
		Source:        "notes.txt",
		MimeType:      "text/plain",
		ContentBase64: base64.StdEncoding.EncodeToString([]byte("orphan candidate")),
	})
	if err == nil {
		t.Fatal("ingest must fail when CreateDocument fails")
	}
	entries, rdErr := os.ReadDir(assetRoot)
	if rdErr != nil {
		t.Fatalf("asset root must be readable: %v", rdErr)
	}
	if len(entries) != 0 {
		t.Fatalf("asset must be compensated on create failure, found %d orphan(s)", len(entries))
	}
}
