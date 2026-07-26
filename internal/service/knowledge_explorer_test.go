package service

import (
	"context"
	"strings"
	"testing"

	v1 "aranea-agents/api/kratos/knowledge/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/loggateway"
)

// ── P3 资源管理器：service 层 stub（DocumentPathReader / ResolvedLinkReader） ──

type stubExplorerPathReader struct {
	paths []biz.KnowledgeDocumentPath
	err   error
}

func (s *stubExplorerPathReader) ListDocumentPaths(_ context.Context, _ string) ([]biz.KnowledgeDocumentPath, error) {
	return s.paths, s.err
}

type stubExplorerLinkReader struct {
	links []biz.KnowledgeResolvedLink
	err   error
}

func (s *stubExplorerLinkReader) ListResolvedLinks(_ context.Context, _, _, _ string) ([]biz.KnowledgeResolvedLink, error) {
	return s.links, s.err
}

func newExplorerService(t *testing.T, paths biz.KnowledgeDocumentPathReader, links biz.KnowledgeResolvedLinkReader) (*KnowledgeService, *us14MemRepo) {
	t.Helper()
	repo := newUS14MemRepo()
	uc := biz.NewKnowledgeUsecaseFromRepo(repo)
	uc.SetExplorerRepos(paths, links)
	return NewKnowledgeService(uc, nil, KnowledgeSearchDeps{}, nil, nil, nil, nil, nil, loggateway.NewNoop()), repo
}

// ── ListVaultTree ────────────────────────────────────────────────────────────

func TestKnowledgeService_ListVaultTree(t *testing.T) {
	svc, repo := newExplorerService(t, &stubExplorerPathReader{paths: []biz.KnowledgeDocumentPath{
		{ID: "d1", RelPath: "notes/a.md", Source: "a.md", Summary: "s1", Tags: []string{"x"}, DocType: "note", Status: "indexed", SizeBytes: 10, UpdatedAt: "2026-07-01"},
		{ID: "d2", RelPath: "notes/deep/b.md", Source: "b.md"},
		{ID: "d3", RelPath: "readme.md", Source: "readme.md"},
	}}, nil)
	col, err := repo.CreateCollection(context.Background(), biz.KnowledgeCollection{ID: "c1", Name: "vault"})
	if err != nil {
		t.Fatal(err)
	}

	resp, err := svc.ListVaultTree(context.Background(), &v1.ListVaultTreeRequest{CollectionId: col.ID})
	if err != nil {
		t.Fatalf("ListVaultTree: %v", err)
	}
	// 根层：notes 目录在前，readme.md 文件在后
	if len(resp.GetItems()) != 2 {
		t.Fatalf("root items = %d, want 2: %+v", len(resp.GetItems()), resp.GetItems())
	}
	if resp.GetItems()[0].GetKind() != "dir" || resp.GetItems()[0].GetName() != "notes" || resp.GetItems()[0].GetPath() != "notes/" {
		t.Errorf("dir node wrong: %+v", resp.GetItems()[0])
	}
	f := resp.GetItems()[1]
	if f.GetKind() != "file" || f.GetDocId() != "d3" || f.GetName() != "readme.md" {
		t.Errorf("file node wrong: %+v", f)
	}

	// 懒加载子层：notes/ 下 a.md 文件 + deep 目录
	resp2, err := svc.ListVaultTree(context.Background(), &v1.ListVaultTreeRequest{CollectionId: col.ID, Prefix: "notes/"})
	if err != nil {
		t.Fatalf("ListVaultTree notes/: %v", err)
	}
	if len(resp2.GetItems()) != 2 || resp2.GetItems()[0].GetName() != "deep" || resp2.GetItems()[1].GetName() != "a.md" {
		t.Errorf("notes/ children wrong: %+v", resp2.GetItems())
	}
	fileNode := resp2.GetItems()[1]
	if fileNode.GetSummary() != "s1" || fileNode.GetDocType() != "note" || fileNode.GetStatus() != "indexed" ||
		fileNode.GetSizeBytes() != 10 || fileNode.GetUpdatedAt() != "2026-07-01" || len(fileNode.GetTags()) != 1 {
		t.Errorf("file node first-density fields not mapped: %+v", fileNode)
	}
}

// 未接线 paths 时必须显式报错（不可静默返回空树）。
func TestKnowledgeService_ListVaultTree_Unconfigured(t *testing.T) {
	repo := newUS14MemRepo()
	uc := biz.NewKnowledgeUsecaseFromRepo(repo) // 不调 SetExplorerRepos
	svc := NewKnowledgeService(uc, nil, KnowledgeSearchDeps{}, nil, nil, nil, nil, nil, loggateway.NewNoop())
	col, err := repo.CreateCollection(context.Background(), biz.KnowledgeCollection{ID: "c1", Name: "vault"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.ListVaultTree(context.Background(), &v1.ListVaultTreeRequest{CollectionId: col.ID})
	if err == nil || !strings.Contains(err.Error(), "vault explorer not configured") {
		t.Errorf("unconfigured paths must error, got %v", err)
	}
}

// 跨租户访问被拒（C-01：NotFound 防泄漏）。
func TestKnowledgeService_ListVaultTree_CrossTenantDenied(t *testing.T) {
	svc, repo := newExplorerService(t, &stubExplorerPathReader{}, nil)
	if _, err := repo.CreateCollection(context.Background(), biz.KnowledgeCollection{ID: "c1", Name: "vault", Workspace: "ws-other"}); err != nil {
		t.Fatal(err)
	}
	ctx := workspace.WithContext(context.Background(), "ws-mine")
	_, err := svc.ListVaultTree(ctx, &v1.ListVaultTreeRequest{CollectionId: "c1"})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("cross-tenant tree must be NotFound, got %v", err)
	}
}

// ── ListDocumentLinks ────────────────────────────────────────────────────────

func TestKnowledgeService_ListDocumentLinks(t *testing.T) {
	svc, repo := newExplorerService(t, nil, &stubExplorerLinkReader{links: []biz.KnowledgeResolvedLink{
		{TargetDocID: "d2", TargetSource: "b.md", TargetRelPath: "notes/b.md", LinkType: "explicit", Context: "[[b]]", Direction: "out"},
		{TargetDocID: "d3", TargetSource: "c.md", TargetRelPath: "", LinkType: "entity", Context: "退款, 政策", Direction: "in"},
	}})
	if _, err := repo.CreateCollection(context.Background(), biz.KnowledgeCollection{ID: "c1", Name: "vault"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateDocument(context.Background(), biz.KnowledgeDocument{ID: "d1", CollectionID: "c1", Source: "a.md"}); err != nil {
		t.Fatal(err)
	}

	resp, err := svc.ListDocumentLinks(context.Background(), &v1.ListDocumentLinksRequest{Id: "d1"})
	if err != nil {
		t.Fatalf("ListDocumentLinks: %v", err)
	}
	if len(resp.GetItems()) != 2 {
		t.Fatalf("links = %d, want 2", len(resp.GetItems()))
	}
	out := resp.GetItems()[0]
	if out.GetTargetDocId() != "d2" || out.GetLinkType() != "explicit" || out.GetDirection() != "out" ||
		out.GetContext() != "[[b]]" || out.GetTargetRelPath() != "notes/b.md" {
		t.Errorf("out link not mapped: %+v", out)
	}
	in := resp.GetItems()[1]
	if in.GetDirection() != "in" || in.GetLinkType() != "entity" || in.GetTargetSource() != "c.md" {
		t.Errorf("in link not mapped: %+v", in)
	}
}

// 未接线 resolvedLinks 时降级返回空（关联为可选增强）。
func TestKnowledgeService_ListDocumentLinks_UnwiredDegrades(t *testing.T) {
	repo := newUS14MemRepo()
	uc := biz.NewKnowledgeUsecaseFromRepo(repo)
	svc := NewKnowledgeService(uc, nil, KnowledgeSearchDeps{}, nil, nil, nil, nil, nil, loggateway.NewNoop())
	if _, err := repo.CreateCollection(context.Background(), biz.KnowledgeCollection{ID: "c1", Name: "vault"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateDocument(context.Background(), biz.KnowledgeDocument{ID: "d1", CollectionID: "c1", Source: "a.md"}); err != nil {
		t.Fatal(err)
	}
	resp, err := svc.ListDocumentLinks(context.Background(), &v1.ListDocumentLinksRequest{Id: "d1"})
	if err != nil {
		t.Fatalf("unwired links must degrade to empty, got %v", err)
	}
	if len(resp.GetItems()) != 0 {
		t.Errorf("unwired links = %d, want 0", len(resp.GetItems()))
	}
}

// ── proto 字段映射（P3 新增 vault/摘要卡字段） ───────────────────────────────

func TestToProtoCollection_VaultFields(t *testing.T) {
	p := toProtoCollection(biz.KnowledgeCollection{
		ID: "c1", Name: "v", RootPath: "/data/vault", SyncState: "active", LastSyncAt: "2026-07-26T10:00:00Z",
	})
	if p.GetRootPath() != "/data/vault" || p.GetSyncState() != "active" || p.GetLastSyncAt() != "2026-07-26T10:00:00Z" {
		t.Errorf("collection vault fields not mapped: %+v", p)
	}
}

func TestToProtoDocument_VaultFields(t *testing.T) {
	p := toProtoDocument(biz.KnowledgeDocument{
		ID: "d1", CollectionID: "c1", Source: "a.md", RelPath: "notes/a.md",
		Summary: "摘要", Tags: []string{"x", "y"}, DocType: "note",
	})
	if p.GetRelPath() != "notes/a.md" || p.GetSummary() != "摘要" || p.GetDocType() != "note" {
		t.Errorf("document vault fields not mapped: %+v", p)
	}
	if len(p.GetTags()) != 2 || p.GetTags()[0] != "x" {
		t.Errorf("document tags not mapped: %+v", p.GetTags())
	}
}
