package knowledge

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aranea-agents/pkg/apierror"
)

// ── G1-B2：CreateVaultDir / CreateVaultDocument ───────────────────────────────

type stubVaultApplier struct {
	called int
	gotRel string
	err    error
}

func (s *stubVaultApplier) ApplyOne(_ context.Context, _ Collection, relPath string) error {
	s.called++
	s.gotRel = relPath
	return s.err
}

// vaultWriteUsecase 构造 filer 已接线、collection root_path 为 root 的 Usecase。
func vaultWriteUsecase(root string) (*Usecase, *mockRepo) {
	mr := noOpMockRepo()
	mr.collGetFn = func(_ context.Context, id string) (Collection, error) {
		return Collection{ID: id, RootPath: root}, nil
	}
	u := NewUsecaseFromRepo(mr)
	u.SetVaultFiler(NewVaultFiler(nil))
	return u, mr
}

// ── CreateVaultDir ────────────────────────────────────────────────────────────

func TestUsecase_CreateVaultDir(t *testing.T) {
	root := t.TempDir()
	u, _ := vaultWriteUsecase(root)

	require.NoError(t, u.CreateVaultDir(context.Background(), "col-1", "a/b/c"))
	info, err := os.Stat(filepath.Join(root, "a", "b", "c"))
	require.NoError(t, err, "嵌套目录必须落盘")
	assert.True(t, info.IsDir())

	// 幂等：重复创建不报错
	require.NoError(t, u.CreateVaultDir(context.Background(), "col-1", "a/b/c"))
}

func TestUsecase_CreateVaultDir_Rejects(t *testing.T) {
	root := t.TempDir()
	u, _ := vaultWriteUsecase(root)

	require.Error(t, u.CreateVaultDir(context.Background(), "col-1", "../escape"))
	require.Error(t, u.CreateVaultDir(context.Background(), "col-1", ".aranea/x"))
	require.Error(t, u.CreateVaultDir(context.Background(), "col-1", "  "))
}

func TestUsecase_CreateVaultDir_NotVault(t *testing.T) {
	u, _ := vaultWriteUsecase("") // 无 root_path 的历史 Collection
	err := u.CreateVaultDir(context.Background(), "col-1", "docs")
	require.Error(t, err)
	assert.True(t, apierror.IsCode(err, apierror.CodeBadRequest), "非 vault collection 必须 BadRequest")
}

func TestUsecase_CreateVaultDir_FilerNotWired(t *testing.T) {
	u := NewUsecaseFromRepo(noOpMockRepo())
	err := u.CreateVaultDir(context.Background(), "col-1", "docs")
	require.Error(t, err, "filer 未接线必须显式报错")
}

// ── G2-B6：ResolveDocumentAsset ─────────────────────────────────────────────

func TestUsecase_ResolveDocumentAsset_Vault(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "media"), 0o755))
	target := filepath.Join(root, "media", "pic.png")
	require.NoError(t, os.WriteFile(target, []byte("png-bytes"), 0o644))

	u, mr := vaultWriteUsecase(root)
	mr.docGetFn = func(_ context.Context, id string) (Document, error) {
		return Document{ID: id, CollectionID: "col-1", RelPath: "media/pic.png", MimeType: "image/png"}, nil
	}

	ref, err := u.ResolveDocumentAsset(context.Background(), "d1")
	require.NoError(t, err)
	absWant, _ := filepath.Abs(target)
	assert.Equal(t, absWant, ref.AbsPath, "vault 文档直接命中磁盘绝对路径")
	assert.Empty(t, ref.AssetURI)
	assert.Equal(t, "pic.png", ref.Name)
	assert.Equal(t, "image/png", ref.MimeType)
	assert.False(t, ref.ModTime.IsZero(), "ModTime 来自 os.Stat")
}

func TestUsecase_ResolveDocumentAsset_VaultMissingFile(t *testing.T) {
	u, mr := vaultWriteUsecase(t.TempDir())
	mr.docGetFn = func(_ context.Context, id string) (Document, error) {
		return Document{ID: id, CollectionID: "col-1", RelPath: "gone.png"}, nil
	}
	_, err := u.ResolveDocumentAsset(context.Background(), "d1")
	require.Error(t, err)
	assert.True(t, apierror.IsCode(err, apierror.CodeNotFound), "磁盘文件消失必须 NotFound")
}

func TestUsecase_ResolveDocumentAsset_LegacyAssetURI(t *testing.T) {
	u, mr := vaultWriteUsecase(t.TempDir())
	mr.docGetFn = func(_ context.Context, id string) (Document, error) {
		return Document{ID: id, CollectionID: "col-1", AssetURI: "d1.png", Source: "photo.png", MimeType: "image/png"}, nil
	}
	ref, err := u.ResolveDocumentAsset(context.Background(), "d1")
	require.NoError(t, err)
	assert.Empty(t, ref.AbsPath, "AssetURI 路径由 service 层经 AssetStore 解析")
	assert.Equal(t, "d1.png", ref.AssetURI)
	assert.Equal(t, "photo.png", ref.Name, "下载名取 source 文件名")
}

func TestUsecase_ResolveDocumentAsset_NoAsset(t *testing.T) {
	u, mr := vaultWriteUsecase(t.TempDir())
	mr.docGetFn = func(_ context.Context, id string) (Document, error) {
		return Document{ID: id, CollectionID: "col-1", Source: "notes.md"}, nil
	}
	_, err := u.ResolveDocumentAsset(context.Background(), "d1")
	require.Error(t, err)
	assert.True(t, apierror.IsCode(err, apierror.CodeNotFound), "纯文本非 vault 文档无 asset")
}

// ── G2-B5：GetVaultDocumentRaw / UpdateVaultDocumentContent ──────────────────

// vaultDocUsecase 在 vaultWriteUsecase 基础上让 GetDocument 返回 vault .md 文档。
func vaultDocUsecase(root, relPath string) (*Usecase, *mockRepo) {
	u, mr := vaultWriteUsecase(root)
	mr.docGetFn = func(_ context.Context, id string) (Document, error) {
		return Document{ID: id, CollectionID: "col-1", RelPath: relPath, Source: relPath, Status: "indexed"}, nil
	}
	return u, mr
}

func TestUsecase_GetVaultDocumentRaw(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "notes"), 0o755))
	fileContent := "---\ntitle: 测试\ncustom: keep-me\n---\n\n# 标题\n\n正文\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, "notes", "a.md"), []byte(fileContent), 0o644))

	u, _ := vaultDocUsecase(root, "notes/a.md")
	body, baseHash, err := u.GetVaultDocumentRaw(context.Background(), "d1")
	require.NoError(t, err)
	assert.Equal(t, "# 标题\n\n正文\n", body, "raw 只含 body（frontmatter 不进编辑器）")
	assert.Equal(t, HashContent(fileContent), baseHash, "base_hash 必须是整个文件的 hash（WriteDocCAS expectedHash 来源）")
}

func TestUsecase_GetVaultDocumentRaw_NotVault(t *testing.T) {
	u, mr := vaultWriteUsecase(t.TempDir())
	mr.docGetFn = func(_ context.Context, id string) (Document, error) {
		return Document{ID: id, CollectionID: "col-1"}, nil // RelPath 空 = 非 vault 文档
	}
	_, _, err := u.GetVaultDocumentRaw(context.Background(), "d1")
	require.Error(t, err)
	assert.True(t, apierror.IsCode(err, apierror.CodeBadRequest), "非 vault 文档必须 BadRequest")
}

func TestUsecase_UpdateVaultDocumentContent(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "notes"), 0o755))
	original := "---\ntitle: 测试\ncustom: keep-me\n---\n\n# 旧正文\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, "notes", "a.md"), []byte(original), 0o644))

	u, mr := vaultDocUsecase(root, "notes/a.md")
	ap := &stubVaultApplier{}
	u.SetVaultApplier(ap)
	mr.docGetByRelFn = func(_ context.Context, _ string, relPath string) (Document, error) {
		return Document{ID: "d1", CollectionID: "col-1", RelPath: relPath, Status: "indexed"}, nil
	}

	doc, conflict, err := u.UpdateVaultDocumentContent(context.Background(), "d1", "# 新正文\n", HashContent(original))
	require.NoError(t, err)
	assert.False(t, conflict, "hash 匹配不冲突")
	assert.Equal(t, "d1", doc.ID)

	// 写回：frontmatter（含用户自定义 extra）保留，body 替换
	data, err := os.ReadFile(filepath.Join(root, "notes", "a.md"))
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, "custom: keep-me", "用户 Extra 必须保留")
	assert.Contains(t, content, "title: 测试", "受管 frontmatter 必须保留")
	assert.Contains(t, content, "# 新正文")
	assert.NotContains(t, content, "# 旧正文")

	// 触发重索引
	assert.Equal(t, 1, ap.called)
	assert.Equal(t, "notes/a.md", ap.gotRel)
}

func TestUsecase_UpdateVaultDocumentContent_ConflictStillWrites(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "notes"), 0o755))
	disk := "# 磁盘当前版（外部已改）\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, "notes", "a.md"), []byte(disk), 0o644))

	u, mr := vaultDocUsecase(root, "notes/a.md")
	u.SetVaultApplier(&stubVaultApplier{})
	mr.docGetByRelFn = func(_ context.Context, _ string, relPath string) (Document, error) {
		return Document{ID: "d1", RelPath: relPath}, nil
	}

	// base_hash 是旧版本的 hash → 与磁盘不匹配 → 留双份（trash 备份）+ 写入 + conflict=true
	_, conflict, err := u.UpdateVaultDocumentContent(context.Background(), "d1", "# 我的编辑\n", HashContent("# 旧版本\n"))
	require.NoError(t, err)
	assert.True(t, conflict, "外部修改必须标记 conflict")

	data, _ := os.ReadFile(filepath.Join(root, "notes", "a.md"))
	assert.Contains(t, string(data), "# 我的编辑", "编辑内容仍写入（保守默认不丢数据）")

	trash := filepath.Join(root, ".aranea", "trash", "notes")
	entries, err := os.ReadDir(trash)
	require.NoError(t, err, "磁盘旧版必须备份进 trash")
	assert.NotEmpty(t, entries)
}

func TestUsecase_UpdateVaultDocumentContent_NotVault(t *testing.T) {
	u, mr := vaultWriteUsecase(t.TempDir())
	mr.docGetFn = func(_ context.Context, id string) (Document, error) {
		return Document{ID: id, CollectionID: "col-1"}, nil
	}
	_, _, err := u.UpdateVaultDocumentContent(context.Background(), "d1", "x", "")
	require.Error(t, err)
	assert.True(t, apierror.IsCode(err, apierror.CodeBadRequest))
}

// ── CreateVaultDocument ───────────────────────────────────────────────────────

func TestUsecase_CreateVaultDocument(t *testing.T) {
	root := t.TempDir()
	u, mr := vaultWriteUsecase(root)
	ap := &stubVaultApplier{}
	u.SetVaultApplier(ap)
	mr.docGetByRelFn = func(_ context.Context, _ string, relPath string) (Document, error) {
		return Document{ID: "d1", CollectionID: "col-1", RelPath: relPath, Source: relPath, Status: "indexed"}, nil
	}

	doc, err := u.CreateVaultDocument(context.Background(), "col-1", "notes/new-doc.md")
	require.NoError(t, err)
	assert.Equal(t, "d1", doc.ID, "apply 后返回索引文档")

	// 文件落盘：frontmatter（created）+ 空标题模板
	data, err := os.ReadFile(filepath.Join(root, "notes", "new-doc.md"))
	require.NoError(t, err)
	content := string(data)
	assert.True(t, strings.HasPrefix(content, "---\n"), "必须含 frontmatter 块")
	assert.Contains(t, content, "created:")
	assert.Contains(t, content, "# ", "空标题模板")

	// 立即 apply（不等 45s 轮询）
	assert.Equal(t, 1, ap.called)
	assert.Equal(t, "notes/new-doc.md", ap.gotRel)
}

func TestUsecase_CreateVaultDocument_ConflictKeepsOriginal(t *testing.T) {
	root := t.TempDir()
	u, _ := vaultWriteUsecase(root)
	ap := &stubVaultApplier{}
	u.SetVaultApplier(ap)

	require.NoError(t, os.MkdirAll(filepath.Join(root, "notes"), 0o755))
	original := "# 用户原文\n\n不要动我\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, "notes", "exist.md"), []byte(original), 0o644))

	_, err := u.CreateVaultDocument(context.Background(), "col-1", "notes/exist.md")
	require.Error(t, err)
	assert.True(t, apierror.IsCode(err, apierror.CodeConflict), "已存在必须 CodeConflict")

	data, _ := os.ReadFile(filepath.Join(root, "notes", "exist.md"))
	assert.Equal(t, original, string(data), "原文件必须保持原样（create 语义不覆盖）")
	assert.Equal(t, 0, ap.called, "冲突不触发 apply")
}

func TestUsecase_CreateVaultDocument_Rejects(t *testing.T) {
	root := t.TempDir()
	u, _ := vaultWriteUsecase(root)

	_, err := u.CreateVaultDocument(context.Background(), "col-1", "notes/no-ext.txt")
	require.Error(t, err, "非 .md 拒绝")
	assert.True(t, apierror.IsCode(err, apierror.CodeBadRequest))

	_, err = u.CreateVaultDocument(context.Background(), "col-1", "../escape.md")
	require.Error(t, err, "穿越拒绝")
}

func TestUsecase_CreateVaultDocument_ApplierError(t *testing.T) {
	root := t.TempDir()
	u, _ := vaultWriteUsecase(root)
	u.SetVaultApplier(&stubVaultApplier{err: fmt.Errorf("index boom")})

	_, err := u.CreateVaultDocument(context.Background(), "col-1", "a.md")
	require.Error(t, err, "apply 失败必须显式返回（文件已落盘，轮询兜底重试）")
}

func TestUsecase_CreateVaultDocument_ApplierNotWired(t *testing.T) {
	root := t.TempDir()
	u, _ := vaultWriteUsecase(root) // 不接 applier：降级跳过立即索引

	doc, err := u.CreateVaultDocument(context.Background(), "col-1", "a.md")
	require.NoError(t, err)
	assert.Equal(t, "pending", doc.Status, "未接线降级返回 pending 占位（轮询兜底）")
	assert.Equal(t, "a.md", doc.RelPath)
	// 文件仍落盘
	_, statErr := os.Stat(filepath.Join(root, "a.md"))
	require.NoError(t, statErr)
}

func TestUsecase_CreateVaultDocument_NotVault(t *testing.T) {
	u, _ := vaultWriteUsecase("")
	_, err := u.CreateVaultDocument(context.Background(), "col-1", "a.md")
	require.Error(t, err)
	assert.True(t, apierror.IsCode(err, apierror.CodeBadRequest))
}

// ── G1-B3：WriteVaultUpload / RemoveVaultFile（上传到指定子目录） ─────────────

func TestUsecase_WriteVaultUpload(t *testing.T) {
	root := t.TempDir()
	u, _ := vaultWriteUsecase(root)

	raw := []byte("%PDF-1.4 fake")
	rel, err := u.WriteVaultUpload(context.Background(), "col-1", "reports/q1", "C:/Users/x/result.pdf", raw)
	require.NoError(t, err)
	assert.Equal(t, "reports/q1/result.pdf", rel, "rel = target_dir + 文件名（取 source 末段）")

	got, err := os.ReadFile(filepath.Join(root, "reports", "q1", "result.pdf"))
	require.NoError(t, err)
	assert.Equal(t, raw, got)
}

// G1 前端约定：target_dir="/" 表示 vault 根目录（空串保留 = 历史行为，service 不调用）。
func TestUsecase_WriteVaultUpload_RootDir(t *testing.T) {
	root := t.TempDir()
	u, _ := vaultWriteUsecase(root)

	rel, err := u.WriteVaultUpload(context.Background(), "col-1", "/", "photo.png", []byte("png"))
	require.NoError(t, err)
	assert.Equal(t, "photo.png", rel, "根目录上传 rel = 文件名（无前导斜杠）")

	got, err := os.ReadFile(filepath.Join(root, "photo.png"))
	require.NoError(t, err)
	assert.Equal(t, []byte("png"), got)
}

func TestUsecase_WriteVaultUpload_Conflict(t *testing.T) {
	root := t.TempDir()
	u, _ := vaultWriteUsecase(root)

	_, err := u.WriteVaultUpload(context.Background(), "col-1", "docs", "a.md", []byte("one"))
	require.NoError(t, err)
	_, err = u.WriteVaultUpload(context.Background(), "col-1", "docs", "a.md", []byte("two"))
	require.Error(t, err)
	assert.True(t, apierror.IsCode(err, apierror.CodeConflict), "同名必须 CodeConflict（前端弹 覆盖/改名/取消）")
}

// ── G3-B4：MoveVaultDocumentToDir（库内跨目录移动） ─────────────────────────

// stubLinkRepo 内存 LinkRepo（入链重建断言用）。
type stubLinkRepo struct {
	links []Link
}

func (s *stubLinkRepo) ReplaceLinks(_ context.Context, collectionID, docID, linkType string, links []Link) error {
	var kept []Link
	for _, l := range s.links {
		if !(l.CollectionID == collectionID && l.DocID == docID && l.LinkType == linkType) {
			kept = append(kept, l)
		}
	}
	s.links = append(kept, links...)
	return nil
}

func (s *stubLinkRepo) ListLinks(_ context.Context, collectionID, docID, linkType string) ([]Link, error) {
	var out []Link
	for _, l := range s.links {
		if l.CollectionID != collectionID {
			continue
		}
		if l.DocID != docID && l.TargetDocID != docID {
			continue
		}
		if linkType != "" && l.LinkType != linkType {
			continue
		}
		out = append(out, l)
	}
	return out, nil
}

// moveDocUsecase 构造文档 B 位于 notes/B.md 的 vault usecase（文件已落盘）。
// 返回的 mockRepo 捕获 UpdateDocumentRelPath 调用。
func moveDocUsecase(t *testing.T, root string) (*Usecase, *mockRepo, *string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "notes"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "notes", "B.md"), []byte("# B\n"), 0o644))

	u, mr := vaultWriteUsecase(root)
	mr.docGetFn = func(_ context.Context, id string) (Document, error) {
		return Document{ID: id, CollectionID: "col-1", RelPath: "notes/B.md", Source: "notes/B.md", Status: "indexed"}, nil
	}
	var newRel string
	mr.docRelPathFn = func(_ context.Context, _ string, rel string) error {
		newRel = rel
		return nil
	}
	return u, mr, &newRel
}

func TestUsecase_MoveVaultDocumentToDir(t *testing.T) {
	root := t.TempDir()
	u, mr, newRel := moveDocUsecase(t, root)
	mr.docGetByRelFn = func(_ context.Context, _ string, relPath string) (Document, error) {
		return Document{ID: "d1", CollectionID: "col-1", RelPath: relPath, Status: "indexed"}, nil
	}

	doc, err := u.MoveVaultDocumentToDir(context.Background(), "d1", "archive", "")
	require.NoError(t, err)
	assert.Equal(t, "archive/B.md", *newRel, "DB 镜像路径必须更新（保留文档身份与索引）")
	assert.Equal(t, "archive/B.md", doc.RelPath, "返回移动后文档")

	// 文件系统：源消失、目标存在
	_, statErr := os.Stat(filepath.Join(root, "notes", "B.md"))
	assert.True(t, os.IsNotExist(statErr))
	_, statErr = os.Stat(filepath.Join(root, "archive", "B.md"))
	assert.NoError(t, statErr)
}

func TestUsecase_MoveVaultDocumentToDir_RootDir(t *testing.T) {
	root := t.TempDir()
	u, mr, newRel := moveDocUsecase(t, root)
	mr.docGetByRelFn = func(_ context.Context, _ string, relPath string) (Document, error) {
		return Document{ID: "d1", RelPath: relPath}, nil
	}

	// "/" 与 "" 均表示 vault 根目录
	for _, dir := range []string{"/", ""} {
		*newRel = ""
		_, err := u.MoveVaultDocumentToDir(context.Background(), "d1", dir, "")
		require.NoError(t, err)
		assert.Equal(t, "B.md", *newRel, "target_dir=%q 必须落根目录", dir)
		// 还原供下一轮
		require.NoError(t, os.Rename(filepath.Join(root, "B.md"), filepath.Join(root, "notes", "B.md")))
	}
}

func TestUsecase_MoveVaultDocumentToDir_SameDirIdempotent(t *testing.T) {
	root := t.TempDir()
	u, _, newRel := moveDocUsecase(t, root)

	doc, err := u.MoveVaultDocumentToDir(context.Background(), "d1", "notes", "")
	require.NoError(t, err)
	assert.Equal(t, "notes/B.md", doc.RelPath, "同目录移动幂等短路")
	assert.Empty(t, *newRel, "幂等短路不得调用 UpdateDocumentRelPath")
	// 文件保持原位
	_, statErr := os.Stat(filepath.Join(root, "notes", "B.md"))
	assert.NoError(t, statErr)
}

func TestUsecase_MoveVaultDocumentToDir_ConflictPolicies(t *testing.T) {
	root := t.TempDir()
	u, mr, newRel := moveDocUsecase(t, root)
	require.NoError(t, os.MkdirAll(filepath.Join(root, "archive"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "archive", "B.md"), []byte("existing"), 0o644))
	mr.docGetByRelFn = func(_ context.Context, _ string, relPath string) (Document, error) {
		return Document{ID: "d1", RelPath: relPath}, nil
	}

	// 默认策略：CodeConflict，双份保持原样
	_, err := u.MoveVaultDocumentToDir(context.Background(), "d1", "archive", "")
	require.Error(t, err)
	assert.True(t, apierror.IsCode(err, apierror.CodeConflict))
	assert.Empty(t, *newRel, "冲突未解决不得更新 DB")

	// rename 策略：保留两份，自动改名
	doc, err := u.MoveVaultDocumentToDir(context.Background(), "d1", "archive", "rename")
	require.NoError(t, err)
	assert.Equal(t, "archive/B (2).md", *newRel)
	assert.Equal(t, "archive/B (2).md", doc.RelPath)
	data, _ := os.ReadFile(filepath.Join(root, "archive", "B.md"))
	assert.Equal(t, "existing", string(data), "rename 策略不得覆盖既有文件")
	data, _ = os.ReadFile(filepath.Join(root, "archive", "B (2).md"))
	assert.Equal(t, "# B\n", string(data))
}

func TestUsecase_MoveVaultDocumentToDir_NotVault(t *testing.T) {
	u, mr := vaultWriteUsecase(t.TempDir())
	mr.docGetFn = func(_ context.Context, id string) (Document, error) {
		return Document{ID: id, CollectionID: "col-1"}, nil // RelPath 空 = 非 vault 文档
	}
	_, err := u.MoveVaultDocumentToDir(context.Background(), "d1", "archive", "")
	require.Error(t, err)
	assert.True(t, apierror.IsCode(err, apierror.CodeBadRequest))
}

func TestUsecase_MoveVaultDocumentToDir_FilerNotWired(t *testing.T) {
	u := NewUsecaseFromRepo(noOpMockRepo())
	_, err := u.MoveVaultDocumentToDir(context.Background(), "d1", "archive", "")
	require.Error(t, err, "filer 未接线必须显式报错")
}

// 移动后入链修复（rebuildExplicitLinks）：引用被移动文档的其他文档重建出链——
// 精确路径引用（[[notes/B]]）悬空断链；basename 引用（[[B]]）移动后仍匹配保留。
func TestUsecase_MoveVaultDocumentToDir_RebuildsInboundLinks(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "notes"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "notes", "B.md"), []byte("# B\n"), 0o644))
	// A.md 精确路径引用 notes/B；A2.md basename 引用 B
	require.NoError(t, os.WriteFile(filepath.Join(root, "A.md"), []byte("见 [[notes/B]]\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "A2.md"), []byte("见 [[B]]\n"), 0o644))

	u, mr := vaultWriteUsecase(root)
	docs := map[string]Document{
		"b-id":  {ID: "b-id", CollectionID: "col-1", RelPath: "notes/B.md", Status: "indexed"},
		"a-id":  {ID: "a-id", CollectionID: "col-1", RelPath: "A.md", Status: "indexed"},
		"a2-id": {ID: "a2-id", CollectionID: "col-1", RelPath: "A2.md", Status: "indexed"},
	}
	mr.docGetFn = func(_ context.Context, id string) (Document, error) {
		if d, ok := docs[id]; ok {
			return d, nil
		}
		return Document{}, apierror.NotFound("knowledge", "doc %s", id)
	}
	mr.docListFn = func(_ context.Context, _ string, _, _ int) ([]Document, int, error) {
		out := []Document{docs["b-id"], docs["a-id"], docs["a2-id"]}
		return out, len(out), nil
	}
	var newRel string
	mr.docRelPathFn = func(_ context.Context, id string, rel string) error {
		newRel = rel
		// 模拟真实 DB：rel_path 更新后候选文档立即可见新路径（入链重建依赖）。
		if d, ok := docs[id]; ok {
			d.RelPath = rel
			docs[id] = d
		}
		return nil
	}
	mr.docGetByRelFn = func(_ context.Context, _ string, relPath string) (Document, error) {
		return Document{ID: "b-id", CollectionID: "col-1", RelPath: relPath, Status: "indexed"}, nil
	}

	links := &stubLinkRepo{links: []Link{
		{CollectionID: "col-1", DocID: "a-id", TargetDocID: "b-id", LinkType: LinkTypeExplicit, Context: "notes/B"},
		{CollectionID: "col-1", DocID: "a2-id", TargetDocID: "b-id", LinkType: LinkTypeExplicit, Context: "B"},
	}}
	u.SetLinkRepos(links, nil)

	_, err := u.MoveVaultDocumentToDir(context.Background(), "b-id", "archive", "")
	require.NoError(t, err)
	assert.Equal(t, "archive/B.md", newRel)

	// A：精确路径 [[notes/B]] 移动后悬空 → 出链清空
	aLinks, err := links.ListLinks(context.Background(), "col-1", "a-id", LinkTypeExplicit)
	require.NoError(t, err)
	assert.Empty(t, aLinks, "精确路径引用移动后必须断链（诚实反映引用失效）")

	// A2：basename [[B]] 仍匹配 archive/B.md → 出链保留指向 b-id
	a2Links, err := links.ListLinks(context.Background(), "col-1", "a2-id", LinkTypeExplicit)
	require.NoError(t, err)
	require.Len(t, a2Links, 1, "basename 引用移动后仍有效")
	assert.Equal(t, "b-id", a2Links[0].TargetDocID)
}

// 未接线 LinkRepo 时移动主流程不受影响（入链修复降级跳过）。
func TestUsecase_MoveVaultDocumentToDir_LinksDegradeWhenUnset(t *testing.T) {
	root := t.TempDir()
	u, mr, newRel := moveDocUsecase(t, root)
	mr.docGetByRelFn = func(_ context.Context, _ string, relPath string) (Document, error) {
		return Document{ID: "d1", RelPath: relPath}, nil
	}

	_, err := u.MoveVaultDocumentToDir(context.Background(), "d1", "archive", "")
	require.NoError(t, err)
	assert.Equal(t, "archive/B.md", *newRel)
}

func TestUsecase_WriteVaultUpload_Rejects(t *testing.T) {
	root := t.TempDir()
	u, _ := vaultWriteUsecase(root)

	// 非 vault collection
	uLegacy, _ := vaultWriteUsecase("")
	_, err := uLegacy.WriteVaultUpload(context.Background(), "col-1", "docs", "a.md", []byte("x"))
	require.Error(t, err)
	assert.True(t, apierror.IsCode(err, apierror.CodeBadRequest))

	// 空 target_dir：契约规定空 = 现有行为（service 不应调用；biz 防御性拒绝）
	_, err = u.WriteVaultUpload(context.Background(), "col-1", "  ", "a.md", []byte("x"))
	require.Error(t, err)

	// source 无文件名
	_, err = u.WriteVaultUpload(context.Background(), "col-1", "docs", "", []byte("x"))
	require.Error(t, err)
	_, err = u.WriteVaultUpload(context.Background(), "col-1", "docs", "a/b/", []byte("x"))
	require.Error(t, err)

	// 穿越
	_, err = u.WriteVaultUpload(context.Background(), "col-1", "../escape", "a.md", []byte("x"))
	require.Error(t, err)
}

func TestUsecase_RemoveVaultFile(t *testing.T) {
	root := t.TempDir()
	u, _ := vaultWriteUsecase(root)

	rel, err := u.WriteVaultUpload(context.Background(), "col-1", "docs", "a.md", []byte("x"))
	require.NoError(t, err)
	require.NoError(t, u.RemoveVaultFile(context.Background(), "col-1", rel))

	_, statErr := os.Stat(filepath.Join(root, "docs", "a.md"))
	assert.True(t, os.IsNotExist(statErr), "补偿删除必须移除文件")

	// 幂等
	require.NoError(t, u.RemoveVaultFile(context.Background(), "col-1", rel))
}
