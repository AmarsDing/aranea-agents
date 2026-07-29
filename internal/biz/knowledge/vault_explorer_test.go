package knowledge

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubPathReader struct {
	paths           []DocumentPath
	err             error
	gotCollectionID string
}

func (s *stubPathReader) ListDocumentPaths(_ context.Context, collectionID string) ([]DocumentPath, error) {
	s.gotCollectionID = collectionID
	return s.paths, s.err
}

type stubResolvedLinkReader struct {
	links       []ResolvedLink
	err         error
	gotLinkType string
}

func (s *stubResolvedLinkReader) ListResolvedLinks(_ context.Context, _, _, linkType string) ([]ResolvedLink, error) {
	s.gotLinkType = linkType
	return s.links, s.err
}

func explorerUsecase() *Usecase {
	return NewUsecaseFromRepo(noOpMockRepo())
}

func TestUsecase_ListVaultTree_NotWired(t *testing.T) {
	u := explorerUsecase()
	_, err := u.ListVaultTree(context.Background(), "col-1", "")
	require.Error(t, err, "未接线 DocumentPathReader 必须显式报错（不可静默返回空树）")
}

func TestUsecase_ListVaultTree_RepoError(t *testing.T) {
	u := explorerUsecase()
	u.SetExplorerRepos(&stubPathReader{err: fmt.Errorf("db down")}, nil)
	_, err := u.ListVaultTree(context.Background(), "col-1", "")
	require.Error(t, err)
}

func TestUsecase_ListVaultTree_RootAggregatesDirsAndFiles(t *testing.T) {
	u := explorerUsecase()
	pr := &stubPathReader{paths: []DocumentPath{
		{ID: "d1", RelPath: "notes/a.md", Source: "a.md", Summary: "s1", DocType: "note", Status: "indexed", Tags: []string{"x"}, SizeBytes: 10, UpdatedAt: "2026-07-01"},
		{ID: "d2", RelPath: "notes/deep/b.md", Source: "b.md"},
		{ID: "d3", RelPath: "reports/q1.md", Source: "q1.md"},
		{ID: "d4", RelPath: "readme.md", Source: "readme.md"},
	}}
	u.SetExplorerRepos(pr, nil)

	got, err := u.ListVaultTree(context.Background(), "col-1", "")
	require.NoError(t, err)
	assert.Equal(t, "col-1", pr.gotCollectionID)

	// 目录去重 + 目录排前（按名称）→ notes, reports；文件随后 readme.md
	require.Len(t, got, 3)
	assert.Equal(t, VaultTreeNode{Name: "notes", Path: "notes/", Kind: "dir"}, got[0])
	assert.Equal(t, VaultTreeNode{Name: "reports", Path: "reports/", Kind: "dir"}, got[1])
	assert.Equal(t, "file", got[2].Kind)
	assert.Equal(t, "readme.md", got[2].Name)
	assert.Equal(t, "d4", got[2].DocID)
}

func TestUsecase_ListVaultTree_SubdirDirectChildrenOnly(t *testing.T) {
	u := explorerUsecase()
	u.SetExplorerRepos(&stubPathReader{paths: []DocumentPath{
		{ID: "d1", RelPath: "notes/a.md", Source: "a.md", Summary: "s1", Tags: []string{"x", "y"}, DocType: "note", Status: "indexed", SizeBytes: 42, UpdatedAt: "2026-07-26"},
		{ID: "d2", RelPath: "notes/deep/b.md", Source: "b.md"},
		{ID: "d3", RelPath: "notes2/c.md", Source: "c.md"}, // 前缀相似但非子路径，不得混入
	}}, nil)

	got, err := u.ListVaultTree(context.Background(), "col-1", "notes/") // 尾斜杠归一
	require.NoError(t, err)
	require.Len(t, got, 2)

	assert.Equal(t, "dir", got[0].Kind)
	assert.Equal(t, "deep", got[0].Name)
	assert.Equal(t, "notes/deep/", got[0].Path)

	assert.Equal(t, "file", got[1].Kind)
	assert.Equal(t, "a.md", got[1].Name)
	assert.Equal(t, "notes/a.md", got[1].Path)
	assert.Equal(t, "d1", got[1].DocID)
	assert.Equal(t, "s1", got[1].Summary)
	assert.Equal(t, []string{"x", "y"}, got[1].Tags)
	assert.Equal(t, "note", got[1].DocType)
	assert.Equal(t, "indexed", got[1].Status)
	assert.Equal(t, int64(42), got[1].SizeBytes)
	assert.Equal(t, "2026-07-26", got[1].UpdatedAt)
}

func TestUsecase_ListVaultTree_NonVaultDocsAtRoot(t *testing.T) {
	u := explorerUsecase()
	u.SetExplorerRepos(&stubPathReader{paths: []DocumentPath{
		{ID: "d1", RelPath: "", Source: "粘贴文本.md"}, // 旧库文档无 rel_path → 根层文件
	}}, nil)

	got, err := u.ListVaultTree(context.Background(), "col-1", "")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "file", got[0].Kind)
	assert.Equal(t, "粘贴文本.md", got[0].Name)

	// 非根目录下不出现无 rel_path 文档
	got, err = u.ListVaultTree(context.Background(), "col-1", "anywhere/")
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestUsecase_ListVaultTree_EmptyVault(t *testing.T) {
	u := explorerUsecase()
	u.SetExplorerRepos(&stubPathReader{}, nil)
	got, err := u.ListVaultTree(context.Background(), "col-1", "")
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestUsecase_ListDocumentResolvedLinks(t *testing.T) {
	t.Run("未接线降级为空（与 P2-4 关联一致）", func(t *testing.T) {
		u := explorerUsecase()
		got, err := u.ListDocumentResolvedLinks(context.Background(), "col-1", "d1", "")
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("透传 linkType 过滤", func(t *testing.T) {
		u := explorerUsecase()
		lr := &stubResolvedLinkReader{links: []ResolvedLink{
			{TargetDocID: "d2", TargetSource: "b.md", LinkType: LinkTypeExplicit, Direction: "out"},
		}}
		u.SetExplorerRepos(nil, lr)
		got, err := u.ListDocumentResolvedLinks(context.Background(), "col-1", "d1", LinkTypeExplicit)
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, LinkTypeExplicit, lr.gotLinkType)
		assert.Equal(t, "b.md", got[0].TargetSource)
	})

	t.Run("repo 错误透传", func(t *testing.T) {
		u := explorerUsecase()
		u.SetExplorerRepos(nil, &stubResolvedLinkReader{err: fmt.Errorf("db down")})
		_, err := u.ListDocumentResolvedLinks(context.Background(), "col-1", "d1", "")
		require.Error(t, err)
	})
}

// ── G1-B1：ListVaultTree 扫文件系统目录（空目录可见 + 目录 mtime）────────────
// 目录来自 FS 扫描 ∪ 索引聚合（索引独有目录兜底、无 mtime）；文件节点不变（仍来自索引）。

// vaultTreeUsecaseWithFiler 构造 filer 已接线、collection root_path 为 root 的 Usecase。
func vaultTreeUsecaseWithFiler(root string, pr DocumentPathReader) *Usecase {
	mr := noOpMockRepo()
	mr.collGetFn = func(_ context.Context, id string) (Collection, error) {
		return Collection{ID: id, RootPath: root}, nil
	}
	u := NewUsecaseFromRepo(mr)
	u.SetExplorerRepos(pr, nil)
	u.SetVaultFiler(NewVaultFiler(nil))
	return u
}

func TestUsecase_ListVaultTree_VaultEmptyDirVisibleWithMtime(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "emptydir"), 0o755))
	u := vaultTreeUsecaseWithFiler(root, &stubPathReader{})

	got, err := u.ListVaultTree(context.Background(), "col-1", "")
	require.NoError(t, err)
	require.Len(t, got, 1, "空目录必须可见（FS 扫描，不再依赖索引聚合）")
	assert.Equal(t, "dir", got[0].Kind)
	assert.Equal(t, "emptydir", got[0].Name)
	assert.Equal(t, "emptydir/", got[0].Path)
	_, perr := time.Parse(time.RFC3339, got[0].UpdatedAt)
	assert.NoError(t, perr, "FS 目录节点必须携带 mtime（RFC3339）")
}

func TestUsecase_ListVaultTree_VaultMergesFSDirsWithIndex(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "notes"), 0o755))
	u := vaultTreeUsecaseWithFiler(root, &stubPathReader{paths: []DocumentPath{
		{ID: "d1", RelPath: "notes/a.md", Source: "a.md"},
		{ID: "d2", RelPath: "archive/x.md", Source: "x.md"}, // archive 不在 FS（外部删除待同步）→ 索引目录并集兜底
	}})

	got, err := u.ListVaultTree(context.Background(), "col-1", "")
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "archive", got[0].Name)
	assert.Empty(t, got[0].UpdatedAt, "索引独有目录无 FS mtime")
	assert.Equal(t, "notes", got[1].Name)
	assert.NotEmpty(t, got[1].UpdatedAt, "FS 存在目录带 mtime（FS 节点覆盖索引聚合）")

	// notes/ 下文件仍来自索引
	got, err = u.ListVaultTree(context.Background(), "col-1", "notes/")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "file", got[0].Kind)
	assert.Equal(t, "a.md", got[0].Name)
	assert.Equal(t, "d1", got[0].DocID)
}

func TestUsecase_ListVaultTree_VaultSkipsDotDirs(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".git"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".aranea", "trash"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "docs"), 0o755))
	u := vaultTreeUsecaseWithFiler(root, &stubPathReader{})

	got, err := u.ListVaultTree(context.Background(), "col-1", "")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "docs", got[0].Name)
}

func TestUsecase_ListVaultTree_VaultPrefixMissingOnFSKeepsIndexFiles(t *testing.T) {
	root := t.TempDir()
	u := vaultTreeUsecaseWithFiler(root, &stubPathReader{paths: []DocumentPath{
		{ID: "d1", RelPath: "ghost/a.md", Source: "a.md"},
	}})

	// ghost/ 已从 FS 删除但索引未清理：不报错，索引文件仍呈现
	got, err := u.ListVaultTree(context.Background(), "col-1", "ghost/")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "file", got[0].Kind)
	assert.Equal(t, "a.md", got[0].Name)
}

func TestUsecase_ListVaultTree_LegacyCollectionWithoutRootPath(t *testing.T) {
	// filer 已接线但 collection 无 root_path（历史 Collection）→ 旧索引聚合行为
	u := vaultTreeUsecaseWithFiler("", &stubPathReader{paths: []DocumentPath{
		{ID: "d1", RelPath: "notes/a.md", Source: "a.md"},
	}})

	got, err := u.ListVaultTree(context.Background(), "col-1", "")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "dir", got[0].Kind)
	assert.Equal(t, "notes", got[0].Name)
	assert.Empty(t, got[0].UpdatedAt, "旧库目录聚合无 mtime")
}

func TestUsecase_ListVaultTree_CollectionLookupError(t *testing.T) {
	mr := noOpMockRepo()
	mr.collGetFn = func(_ context.Context, _ string) (Collection, error) {
		return Collection{}, fmt.Errorf("db down")
	}
	u := NewUsecaseFromRepo(mr)
	u.SetExplorerRepos(&stubPathReader{}, nil)
	u.SetVaultFiler(NewVaultFiler(nil))
	_, err := u.ListVaultTree(context.Background(), "col-1", "")
	require.Error(t, err, "filer 接线后 collection 查询失败必须透传")
}
