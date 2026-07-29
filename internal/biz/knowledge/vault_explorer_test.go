package knowledge

import (
	"context"
	"fmt"
	"testing"

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
