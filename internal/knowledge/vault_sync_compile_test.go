package knowledge

import (
	"context"
	"fmt"
	"testing"

	bizknowledge "aranea-agents/internal/biz/knowledge"
)

// ── M0 摄取编译：二进制文件经编译端口抽取为 Markdown ────────────────────────

// stubCompiler 测试编译桩：记录调用、返回固定 Markdown。
type stubCompiler struct {
	calls     []string // 记录每次编译的 relPath
	returnMD  string   // 返回的 Markdown 正文
	returnErr error    // 注入错误
}

func (s *stubCompiler) Compile(_ context.Context, relPath string, raw []byte) (string, string, error) {
	s.calls = append(s.calls, relPath)
	if s.returnErr != nil {
		return "", "", s.returnErr
	}
	return s.returnMD, "text/markdown", nil
}

// 二进制文件（pdf）经编译端口抽取为 Markdown 进索引。
func TestVaultSyncApplier_BinaryCompiledToMarkdown(t *testing.T) {
	root := t.TempDir()
	writeVaultFile(t, root, "docs/report.pdf", "%PDF-fake-binary")

	repo := newVaultSyncMemRepo()
	vault := bizknowledge.Collection{ID: "col1", RootPath: root, SyncState: "active"}
	repo.collections[vault.ID] = vault

	compiler := &stubCompiler{returnMD: "# 报告标题\n\n抽取后的正文内容。"}
	applier := newTestApplier(repo, nil)
	applier.SetCompiler(compiler)

	if err := applier.ApplyEvents(context.Background(), vault, []bizknowledge.ChangeEvent{createdEvent("docs/report.pdf", "%PDF-fake-binary")}); err != nil {
		t.Fatalf("apply binary created: %v", err)
	}

	if len(compiler.calls) != 1 || compiler.calls[0] != "docs/report.pdf" {
		t.Fatalf("compiler calls = %v, want [docs/report.pdf]", compiler.calls)
	}
	if len(repo.documents) != 1 {
		t.Fatalf("expected 1 document, got %d", len(repo.documents))
	}
	var doc bizknowledge.Document
	for _, d := range repo.documents {
		doc = d
	}
	if doc.ContentText != "# 报告标题\n\n抽取后的正文内容。" {
		t.Errorf("ContentText = %q, want compiled markdown", doc.ContentText)
	}
	if doc.Status != "indexed" {
		t.Errorf("Status = %q, want indexed", doc.Status)
	}
	if doc.ChunkCount == 0 {
		t.Error("expected chunks built from compiled markdown")
	}
}

// 文本直读（.md）不经编译端口（保持零成本 + frontmatter 解析）。
func TestVaultSyncApplier_TextDirectSkipsCompiler(t *testing.T) {
	root := t.TempDir()
	writeVaultFile(t, root, "notes/a.md", testVaultMD)

	repo := newVaultSyncMemRepo()
	vault := bizknowledge.Collection{ID: "col1", RootPath: root, SyncState: "active"}
	repo.collections[vault.ID] = vault

	compiler := &stubCompiler{returnMD: "不应被调用"}
	applier := newTestApplier(repo, nil)
	applier.SetCompiler(compiler)

	if err := applier.ApplyEvents(context.Background(), vault, []bizknowledge.ChangeEvent{createdEvent("notes/a.md", testVaultMD)}); err != nil {
		t.Fatalf("apply md created: %v", err)
	}
	if len(compiler.calls) != 0 {
		t.Errorf("compiler 不应被调用（.md 直读），calls = %v", compiler.calls)
	}
}

// 编译失败 → 文档 status=error，不阻断后续事件。
func TestVaultSyncApplier_CompileFailure_MarksError(t *testing.T) {
	root := t.TempDir()
	writeVaultFile(t, root, "a.pdf", "%PDF-1")
	writeVaultFile(t, root, "b.md", testVaultMD)

	repo := newVaultSyncMemRepo()
	vault := bizknowledge.Collection{ID: "col1", RootPath: root, SyncState: "active"}
	repo.collections[vault.ID] = vault

	compiler := &stubCompiler{returnErr: fmt.Errorf("vision llm timeout")}
	applier := newTestApplier(repo, nil)
	applier.SetCompiler(compiler)

	events := []bizknowledge.ChangeEvent{
		createdEvent("a.pdf", "%PDF-1"),
		createdEvent("b.md", testVaultMD),
	}
	// ApplyEvents 单事件失败不阻断，返回首个错误。
	_ = applier.ApplyEvents(context.Background(), vault, events)

	var pdfDoc, mdDoc bizknowledge.Document
	for _, d := range repo.documents {
		switch d.RelPath {
		case "a.pdf":
			pdfDoc = d
		case "b.md":
			mdDoc = d
		}
	}
	if pdfDoc.Status != "error" {
		t.Errorf("pdf Status = %q, want error（编译失败）", pdfDoc.Status)
	}
	if mdDoc.Status != "indexed" {
		t.Errorf("md Status = %q, want indexed（不被阻断）", mdDoc.Status)
	}
}

// 无编译端口（nil）时二进制文件降级：status=error，不 panic。
func TestVaultSyncApplier_NoCompiler_BinaryDegrades(t *testing.T) {
	root := t.TempDir()
	writeVaultFile(t, root, "a.pdf", "%PDF-1")

	repo := newVaultSyncMemRepo()
	vault := bizknowledge.Collection{ID: "col1", RootPath: root, SyncState: "active"}
	repo.collections[vault.ID] = vault

	applier := newTestApplier(repo, nil) // 不 SetCompiler
	_ = applier.ApplyEvents(context.Background(), vault, []bizknowledge.ChangeEvent{createdEvent("a.pdf", "%PDF-1")})

	var doc bizknowledge.Document
	for _, d := range repo.documents {
		doc = d
	}
	if doc.Status != "error" {
		t.Errorf("Status = %q, want error（无编译端口降级）", doc.Status)
	}
}

func TestIsContextOverflow(t *testing.T) {
	if !isContextOverflow(fmt.Errorf("prompt too long: 4173 > n_ctx 4096")) {
		t.Fatal("n_ctx overflow must be detected")
	}
	if isContextOverflow(fmt.Errorf("vision llm timeout")) {
		t.Fatal("generic timeout is not context overflow")
	}
}

func TestVaultSyncApplier_CompileFailure_BackoffSkipsNextTick(t *testing.T) {
	root := t.TempDir()
	writeVaultFile(t, root, "shot.png", "fake-png")

	repo := newVaultSyncMemRepo()
	vault := bizknowledge.Collection{ID: "col1", RootPath: root, SyncState: "active"}
	repo.collections[vault.ID] = vault

	compiler := &stubCompiler{returnErr: fmt.Errorf("4173 > n_ctx 4096")}
	applier := newTestApplier(repo, nil)
	applier.SetCompiler(compiler)

	ev := createdEvent("shot.png", "fake-png")
	_ = applier.ApplyEvents(context.Background(), vault, []bizknowledge.ChangeEvent{ev})
	if len(compiler.calls) != 1 {
		t.Fatalf("first tick compile calls = %d, want 1", len(compiler.calls))
	}
	var doc bizknowledge.Document
	for _, d := range repo.documents {
		if d.RelPath == "shot.png" {
			doc = d
			break
		}
	}
	if doc.Status != "error" || doc.EmbedFailCount != 1 {
		t.Fatalf("after fail status=%q failCount=%d, want error/1", doc.Status, doc.EmbedFailCount)
	}

	_ = applier.ApplyEvents(context.Background(), vault, []bizknowledge.ChangeEvent{ev})
	if len(compiler.calls) != 1 {
		t.Fatalf("backoff tick must skip compile, calls = %d", len(compiler.calls))
	}
}
