package service

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	v1 "aranea-agents/api/kratos/knowledge/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/knowledge"
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

// ── G1-B2：CreateVaultDir / CreateVaultDocument ───────────────────────────────

// stubVaultDocApplier 模拟立即索引：直接写文档镜像进 mem repo。
type stubVaultDocApplier struct{ repo *us14MemRepo }

func (s *stubVaultDocApplier) ApplyOne(ctx context.Context, vault biz.KnowledgeCollection, relPath string) error {
	_, err := s.repo.CreateDocument(ctx, biz.KnowledgeDocument{
		ID: "doc-" + relPath, CollectionID: vault.ID, RelPath: relPath, Source: relPath, Status: "indexed",
	})
	return err
}

func newVaultWriteService(t *testing.T) (*KnowledgeService, *us14MemRepo) {
	t.Helper()
	repo := newUS14MemRepo()
	uc := biz.NewKnowledgeUsecaseFromRepo(repo)
	uc.SetVaultFiler(biz.NewKnowledgeVaultFiler(nil))
	uc.SetVaultApplier(&stubVaultDocApplier{repo: repo})
	return NewKnowledgeService(uc, nil, KnowledgeSearchDeps{}, nil, nil, nil, nil, nil, loggateway.NewNoop()), repo
}

func TestKnowledgeService_CreateVaultDir(t *testing.T) {
	root := t.TempDir()
	svc, repo := newVaultWriteService(t)
	if _, err := repo.CreateCollection(context.Background(), biz.KnowledgeCollection{ID: "c1", Name: "vault", RootPath: root, Workspace: workspace.DefaultWorkspaceID}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateVaultDir(context.Background(), &v1.CreateVaultDirRequest{CollectionId: "c1", DirPath: "guides/setup"}); err != nil {
		t.Fatalf("CreateVaultDir: %v", err)
	}
	info, err := os.Stat(filepath.Join(root, "guides", "setup"))
	if err != nil || !info.IsDir() {
		t.Errorf("dir must exist on FS: %v", err)
	}
}

func TestKnowledgeService_CreateVaultDir_CrossTenantDenied(t *testing.T) {
	root := t.TempDir()
	svc, repo := newVaultWriteService(t)
	if _, err := repo.CreateCollection(context.Background(), biz.KnowledgeCollection{ID: "c1", Name: "vault", RootPath: root, Workspace: "ws-other"}); err != nil {
		t.Fatal(err)
	}
	ctx := workspace.WithContext(context.Background(), "ws-mine")
	_, err := svc.CreateVaultDir(ctx, &v1.CreateVaultDirRequest{CollectionId: "c1", DirPath: "x"})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("cross-tenant must be NotFound, got %v", err)
	}
}

func TestKnowledgeService_CreateVaultDocument(t *testing.T) {
	root := t.TempDir()
	svc, repo := newVaultWriteService(t)
	if _, err := repo.CreateCollection(context.Background(), biz.KnowledgeCollection{ID: "c1", Name: "vault", RootPath: root, Workspace: workspace.DefaultWorkspaceID}); err != nil {
		t.Fatal(err)
	}
	resp, err := svc.CreateVaultDocument(context.Background(), &v1.CreateVaultDocumentRequest{CollectionId: "c1", RelPath: "notes/new.md"})
	if err != nil {
		t.Fatalf("CreateVaultDocument: %v", err)
	}
	if resp.GetId() == "" || resp.GetRelPath() != "notes/new.md" {
		t.Errorf("unexpected doc: %+v", resp)
	}
	data, err := os.ReadFile(filepath.Join(root, "notes", "new.md"))
	if err != nil {
		t.Fatalf("doc file must exist on FS: %v", err)
	}
	if !strings.Contains(string(data), "created:") || !strings.Contains(string(data), "# ") {
		t.Errorf("template must be frontmatter + empty heading, got:\n%s", string(data))
	}
	// 重复创建 → Conflict
	if _, err := svc.CreateVaultDocument(context.Background(), &v1.CreateVaultDocumentRequest{CollectionId: "c1", RelPath: "notes/new.md"}); err == nil {
		t.Error("duplicate create must conflict")
	}
}

// ── G2-B5：GetDocumentContent raw/base_hash + UpdateDocumentContent ──────────

func TestKnowledgeService_GetDocumentContent_VaultRaw(t *testing.T) {
	root := t.TempDir()
	svc, repo := newVaultWriteService(t)
	if _, err := repo.CreateCollection(context.Background(), biz.KnowledgeCollection{ID: "c1", Name: "vault", RootPath: root, Workspace: workspace.DefaultWorkspaceID}); err != nil {
		t.Fatal(err)
	}
	fileContent := "---\ncreated: 2026-07-30T00:00:00Z\n---\n\n# 原文\n"
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes", "a.md"), []byte(fileContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateDocument(context.Background(), biz.KnowledgeDocument{
		ID: "d1", CollectionID: "c1", RelPath: "notes/a.md", Source: "a.md",
		ContentText: "# 整理后文本", Organized: true, Status: "indexed",
	}); err != nil {
		t.Fatal(err)
	}

	resp, err := svc.GetDocumentContent(context.Background(), &v1.GetDocumentContentRequest{Id: "d1"})
	if err != nil {
		t.Fatalf("GetDocumentContent: %v", err)
	}
	if resp.GetContentText() != "# 整理后文本" {
		t.Errorf("content_text must stay the organized preview, got %q", resp.GetContentText())
	}
	if resp.GetRawContent() != "# 原文\n" {
		t.Errorf("raw_content must be the vault file body, got %q", resp.GetRawContent())
	}
	if resp.GetBaseHash() == "" {
		t.Error("base_hash must be present for vault documents")
	}
}

func TestKnowledgeService_UpdateDocumentContent(t *testing.T) {
	root := t.TempDir()
	svc, repo := newVaultWriteService(t)
	if _, err := repo.CreateCollection(context.Background(), biz.KnowledgeCollection{ID: "c1", Name: "vault", RootPath: root, Workspace: workspace.DefaultWorkspaceID}); err != nil {
		t.Fatal(err)
	}
	original := "---\ncreated: 2026-07-30T00:00:00Z\n---\n\n# 旧\n"
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes", "a.md"), []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateDocument(context.Background(), biz.KnowledgeDocument{
		ID: "d1", CollectionID: "c1", RelPath: "notes/a.md", Source: "a.md", Status: "indexed",
	}); err != nil {
		t.Fatal(err)
	}

	content, err := svc.GetDocumentContent(context.Background(), &v1.GetDocumentContentRequest{Id: "d1"})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := svc.UpdateDocumentContent(context.Background(), &v1.UpdateDocumentContentRequest{
		Id: "d1", Content: "# 新正文\n", BaseHash: content.GetBaseHash(),
	})
	if err != nil {
		t.Fatalf("UpdateDocumentContent: %v", err)
	}
	if resp.GetConflict() {
		t.Error("matching base_hash must not conflict")
	}
	if resp.GetDocument().GetId() == "" {
		t.Errorf("updated document must be returned: %+v", resp.GetDocument())
	}
	data, _ := os.ReadFile(filepath.Join(root, "notes", "a.md"))
	if !strings.Contains(string(data), "# 新正文") || !strings.Contains(string(data), "created:") {
		t.Errorf("file must have new body + preserved frontmatter, got:\n%s", string(data))
	}

	// 陈旧 base_hash（文件刚被我方写入）→ conflict=true 但仍写入
	resp2, err := svc.UpdateDocumentContent(context.Background(), &v1.UpdateDocumentContentRequest{
		Id: "d1", Content: "# 再次编辑\n", BaseHash: content.GetBaseHash(),
	})
	if err != nil {
		t.Fatalf("stale base_hash must still succeed (both copies kept): %v", err)
	}
	if !resp2.GetConflict() {
		t.Error("stale base_hash must flag conflict")
	}
}

// ── G3-B4：MoveDocumentToDir ────────────────────────────────────────────────

func TestKnowledgeService_MoveDocumentToDir(t *testing.T) {
	root := t.TempDir()
	svc, repo := newVaultWriteService(t)
	if _, err := repo.CreateCollection(context.Background(), biz.KnowledgeCollection{ID: "c1", Name: "vault", RootPath: root, Workspace: workspace.DefaultWorkspaceID}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes", "B.md"), []byte("# B\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateDocument(context.Background(), biz.KnowledgeDocument{
		ID: "d1", CollectionID: "c1", RelPath: "notes/B.md", Source: "notes/B.md", Status: "indexed",
	}); err != nil {
		t.Fatal(err)
	}

	doc, err := svc.MoveDocumentToDir(context.Background(), &v1.MoveDocumentToDirRequest{Id: "d1", TargetDir: "archive"})
	if err != nil {
		t.Fatalf("MoveDocumentToDir: %v", err)
	}
	if doc.GetRelPath() != "archive/B.md" {
		t.Errorf("rel_path = %q, want archive/B.md", doc.GetRelPath())
	}
	if doc.GetId() != "d1" {
		t.Errorf("document identity must be kept, got id %q", doc.GetId())
	}
	if _, err := os.Stat(filepath.Join(root, "archive", "B.md")); err != nil {
		t.Errorf("file must exist at target dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "notes", "B.md")); !os.IsNotExist(err) {
		t.Error("source file must be gone after move")
	}

	// 同名冲突（默认策略）→ CodeConflict；rename 策略 → 保留两份自动改名
	// 先移回 notes（首次移动已腾空，无冲突），再手动占位 archive/B.md。
	if _, err := svc.MoveDocumentToDir(context.Background(), &v1.MoveDocumentToDirRequest{Id: "d1", TargetDir: "notes"}); err != nil {
		t.Fatalf("move back to notes: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "archive", "B.md"), []byte("# 占位\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.MoveDocumentToDir(context.Background(), &v1.MoveDocumentToDirRequest{Id: "d1", TargetDir: "archive"}); err == nil {
		t.Error("name clash with default policy must conflict")
	}
	doc2, err := svc.MoveDocumentToDir(context.Background(), &v1.MoveDocumentToDirRequest{Id: "d1", TargetDir: "archive", ConflictPolicy: "rename"})
	if err != nil {
		t.Fatalf("rename policy: %v", err)
	}
	if doc2.GetRelPath() != "archive/B (2).md" {
		t.Errorf("rename policy rel_path = %q, want archive/B (2).md", doc2.GetRelPath())
	}
}

// 跨租户访问被拒（C-01：NotFound 防泄漏）。
func TestKnowledgeService_MoveDocumentToDir_CrossTenantDenied(t *testing.T) {
	root := t.TempDir()
	svc, repo := newVaultWriteService(t)
	if _, err := repo.CreateCollection(context.Background(), biz.KnowledgeCollection{ID: "c1", Name: "vault", RootPath: root, Workspace: "ws-other"}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes", "B.md"), []byte("# B\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateDocument(context.Background(), biz.KnowledgeDocument{
		ID: "d1", CollectionID: "c1", RelPath: "notes/B.md", Source: "notes/B.md", Status: "indexed",
	}); err != nil {
		t.Fatal(err)
	}
	ctx := workspace.WithContext(context.Background(), "ws-mine")
	_, err := svc.MoveDocumentToDir(ctx, &v1.MoveDocumentToDirRequest{Id: "d1", TargetDir: "archive"})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("cross-tenant move must be NotFound, got %v", err)
	}
	// 文件不得被移动
	if _, statErr := os.Stat(filepath.Join(root, "notes", "B.md")); statErr != nil {
		t.Error("cross-tenant attempt must not move the file")
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

// ── G4-B8：ListCollectionGraph 单库全量图谱 ─────────────────────────────────

type stubCollectionLinkReader struct {
	links     []knowledge.Link
	err       error
	gotTypes  []string
	gotCollID string
}

func (s *stubCollectionLinkReader) ListCollectionLinks(_ context.Context, collectionID string, linkTypes []string) ([]knowledge.Link, error) {
	s.gotCollID = collectionID
	s.gotTypes = linkTypes
	return s.links, s.err
}

func newGraphService(t *testing.T, links biz.KnowledgeCollectionLinkReader) (*KnowledgeService, *us14MemRepo) {
	t.Helper()
	repo := newUS14MemRepo()
	uc := biz.NewKnowledgeUsecaseFromRepo(repo)
	uc.SetGraphRepo(links)
	return NewKnowledgeService(uc, nil, KnowledgeSearchDeps{}, nil, nil, nil, nil, nil, loggateway.NewNoop()), repo
}

func TestKnowledgeService_ListCollectionGraph(t *testing.T) {
	lr := &stubCollectionLinkReader{links: []knowledge.Link{
		{DocID: "d1", TargetDocID: "d2", LinkType: knowledge.LinkTypeExplicit, Context: "[[b]]"},
		{DocID: "d1", TargetDocID: "d3", LinkType: knowledge.LinkTypeEntity, Context: "营收"},
		{DocID: "d1", TargetDocID: "ghost", LinkType: knowledge.LinkTypeExplicit}, // 悬空 → 剔除
	}}
	svc, repo := newGraphService(t, lr)
	ctx := context.Background()
	if _, err := repo.CreateCollection(ctx, biz.KnowledgeCollection{ID: "c1", Name: "vault"}); err != nil {
		t.Fatal(err)
	}
	for _, d := range []biz.KnowledgeDocument{
		{ID: "d1", CollectionID: "c1", Source: "a.md", RelPath: "notes/a.md", DocType: "note"},
		{ID: "d2", CollectionID: "c1", Source: "b.md", RelPath: "notes/b.md", DocType: "report"},
		{ID: "d3", CollectionID: "c1", Source: "q1.md", RelPath: "reports/q1.md"},
	} {
		if _, err := repo.CreateDocument(ctx, d); err != nil {
			t.Fatal(err)
		}
	}

	// 全量：3 节点 + 2 边（悬空剔除）；degree 双向累计。
	resp, err := svc.ListCollectionGraph(ctx, &v1.ListCollectionGraphRequest{CollectionId: "c1"})
	if err != nil {
		t.Fatalf("ListCollectionGraph: %v", err)
	}
	if len(resp.GetNodes()) != 3 {
		t.Fatalf("nodes = %d, want 3", len(resp.GetNodes()))
	}
	if len(resp.GetEdges()) != 2 {
		t.Fatalf("edges = %d, want 2 (dangling dropped): %+v", len(resp.GetEdges()), resp.GetEdges())
	}
	// 节点顺序依赖 repo ListDocuments 实现，按 doc_id 索引断言。
	byID := make(map[string]*v1.CollectionGraphNode, len(resp.GetNodes()))
	for _, n := range resp.GetNodes() {
		byID[n.GetDocId()] = n
	}
	n1 := byID["d1"]
	if n1 == nil || n1.GetName() != "a.md" || n1.GetRelPath() != "notes/a.md" ||
		n1.GetDocType() != "note" || n1.GetDegree() != 2 {
		t.Errorf("node d1 wrong: %+v", n1)
	}
	if byID["d2"].GetDegree() != 1 || byID["d3"].GetDegree() != 1 {
		t.Errorf("degree wrong: d2=%d d3=%d, want 1/1", byID["d2"].GetDegree(), byID["d3"].GetDegree())
	}
	edgeOK := false
	for _, e := range resp.GetEdges() {
		if e.GetSource() == "d1" && e.GetTarget() == "d2" && e.GetType() == "explicit" {
			edgeOK = true
		}
	}
	if !edgeOK {
		t.Errorf("explicit edge d1→d2 missing: %+v", resp.GetEdges())
	}

	// 类型过滤透传到 repo。
	if _, err := svc.ListCollectionGraph(ctx, &v1.ListCollectionGraphRequest{CollectionId: "c1", LinkTypes: []string{"explicit"}}); err != nil {
		t.Fatal(err)
	}
	if len(lr.gotTypes) != 1 || lr.gotTypes[0] != "explicit" {
		t.Errorf("link_types not plumbed: %v", lr.gotTypes)
	}

	// 目录前缀：notes/ 仅 d1/d2 节点，跨界边（d1→d3）剔除。
	resp, err = svc.ListCollectionGraph(ctx, &v1.ListCollectionGraphRequest{CollectionId: "c1", PathPrefix: "notes"})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.GetNodes()) != 2 || len(resp.GetEdges()) != 1 {
		t.Errorf("prefix graph = %d nodes / %d edges, want 2/1", len(resp.GetNodes()), len(resp.GetEdges()))
	}
}

// 未接线 graphLinks 时降级为仅节点无边（与 ListDocumentLinks 降级语义一致）。
func TestKnowledgeService_ListCollectionGraph_UnwiredDegrades(t *testing.T) {
	svc, repo := newGraphService(t, nil)
	ctx := context.Background()
	if _, err := repo.CreateCollection(ctx, biz.KnowledgeCollection{ID: "c1", Name: "vault"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateDocument(ctx, biz.KnowledgeDocument{ID: "d1", CollectionID: "c1", Source: "a.md", RelPath: "a.md"}); err != nil {
		t.Fatal(err)
	}
	resp, err := svc.ListCollectionGraph(ctx, &v1.ListCollectionGraphRequest{CollectionId: "c1"})
	if err != nil {
		t.Fatalf("unwired graph must degrade, got %v", err)
	}
	if len(resp.GetNodes()) != 1 || len(resp.GetEdges()) != 0 {
		t.Errorf("unwired graph = %d nodes / %d edges, want 1/0", len(resp.GetNodes()), len(resp.GetEdges()))
	}
}

// 跨租户访问被拒（C-01：NotFound 防泄漏）。
func TestKnowledgeService_ListCollectionGraph_CrossTenantDenied(t *testing.T) {
	svc, repo := newGraphService(t, &stubCollectionLinkReader{})
	if _, err := repo.CreateCollection(context.Background(), biz.KnowledgeCollection{ID: "c1", Name: "vault", Workspace: "ws-other"}); err != nil {
		t.Fatal(err)
	}
	ctx := workspace.WithContext(context.Background(), "ws-mine")
	_, err := svc.ListCollectionGraph(ctx, &v1.ListCollectionGraphRequest{CollectionId: "c1"})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("cross-tenant graph must be NotFound, got %v", err)
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

// ── G1-B3：IngestDocument target_dir（上传到指定子目录） ─────────────────────

func TestKnowledgeService_IngestDocument_TargetDir(t *testing.T) {
	root := t.TempDir()
	svc, repo := newVaultWriteService(t)
	if _, err := repo.CreateCollection(context.Background(), biz.KnowledgeCollection{ID: "c1", Name: "vault", RootPath: root, Workspace: workspace.DefaultWorkspaceID}); err != nil {
		t.Fatal(err)
	}
	raw := []byte("hello vault upload")
	resp, err := svc.IngestDocument(context.Background(), &v1.IngestDocumentRequest{
		CollectionId:  "c1",
		Source:        "notes.txt",
		MimeType:      "text/plain",
		ContentBase64: base64.StdEncoding.EncodeToString(raw),
		TargetDir:     "uploads/docs",
	})
	if err != nil {
		t.Fatalf("ingest with target_dir: %v", err)
	}
	// 文件落盘（vault 文件系统为真相源）
	data, err := os.ReadFile(filepath.Join(root, "uploads", "docs", "notes.txt"))
	if err != nil {
		t.Fatalf("uploaded file must exist on FS: %v", err)
	}
	if string(data) != string(raw) {
		t.Errorf("file content mismatch: %q", string(data))
	}
	// 文档镜像带 rel_path + content_hash（同步链视为已应用，不重复处理）
	doc, err := repo.GetDocument(context.Background(), resp.GetId())
	if err != nil {
		t.Fatal(err)
	}
	if doc.RelPath != "uploads/docs/notes.txt" {
		t.Errorf("RelPath = %q, want uploads/docs/notes.txt", doc.RelPath)
	}
	if doc.ContentHash != biz.KnowledgeHashContent(string(raw)) {
		t.Errorf("ContentHash = %q, want file hash", doc.ContentHash)
	}
}

func TestKnowledgeService_IngestDocument_TargetDir_NotVault(t *testing.T) {
	svc, repo := newVaultWriteService(t)
	if _, err := repo.CreateCollection(context.Background(), biz.KnowledgeCollection{ID: "legacy", Name: "old", Workspace: workspace.DefaultWorkspaceID}); err != nil {
		t.Fatal(err)
	}
	_, err := svc.IngestDocument(context.Background(), &v1.IngestDocumentRequest{
		CollectionId:  "legacy",
		Source:        "a.txt",
		MimeType:      "text/plain",
		ContentBase64: base64.StdEncoding.EncodeToString([]byte("x")),
		TargetDir:     "docs",
	})
	if err == nil || !strings.Contains(err.Error(), "not a vault") {
		t.Errorf("target_dir on legacy collection must be BadRequest, got %v", err)
	}
}

func TestKnowledgeService_IngestDocument_TargetDir_Conflict(t *testing.T) {
	root := t.TempDir()
	svc, repo := newVaultWriteService(t)
	if _, err := repo.CreateCollection(context.Background(), biz.KnowledgeCollection{ID: "c1", Name: "vault", RootPath: root, Workspace: workspace.DefaultWorkspaceID}); err != nil {
		t.Fatal(err)
	}
	req := &v1.IngestDocumentRequest{
		CollectionId:  "c1",
		Source:        "dup.txt",
		MimeType:      "text/plain",
		ContentBase64: base64.StdEncoding.EncodeToString([]byte("one")),
		TargetDir:     "docs",
	}
	if _, err := svc.IngestDocument(context.Background(), req); err != nil {
		t.Fatalf("first ingest: %v", err)
	}
	// 同名再传 → CodeConflict，原文件保持原样
	_, err := svc.IngestDocument(context.Background(), &v1.IngestDocumentRequest{
		CollectionId:  "c1",
		Source:        "dup.txt",
		MimeType:      "text/plain",
		ContentBase64: base64.StdEncoding.EncodeToString([]byte("two")),
		TargetDir:     "docs",
	})
	if err == nil {
		t.Fatal("duplicate upload must conflict")
	}
	data, _ := os.ReadFile(filepath.Join(root, "docs", "dup.txt"))
	if string(data) != "one" {
		t.Errorf("original file must be preserved, got %q", string(data))
	}
}
