package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	return NewKnowledgeService(uc, nil, KnowledgeSearchDeps{}, nil, nil, knowledge.NewAssetStore(assetRoot), nil, nil, loggateway.NewNoop()), repo
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
