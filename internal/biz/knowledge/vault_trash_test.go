package knowledge

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUsecase_RenameVaultDocumentInPlace(t *testing.T) {
	root := t.TempDir()
	u, mr, newRel := moveDocUsecase(t, root)
	mr.docGetByRelFn = func(_ context.Context, _ string, relPath string) (Document, error) {
		return Document{ID: "d1", CollectionID: "col-1", RelPath: relPath, Status: "indexed"}, nil
	}
	doc, err := u.RelocateVaultDocument(context.Background(), "d1", nil, "周报-定稿.md", "")
	require.NoError(t, err)
	assert.Equal(t, "notes/周报-定稿.md", *newRel)
	assert.Equal(t, "notes/周报-定稿.md", doc.RelPath)
	_, err = os.Stat(filepath.Join(root, "notes", "周报-定稿.md"))
	assert.NoError(t, err)

	for _, bad := range []string{"a/b.md", `a\b.md`, "..", ".aranea"} {
		_, err := u.RelocateVaultDocument(context.Background(), "d1", nil, bad, "")
		assert.Error(t, err, bad)
	}
}

func TestUsecase_VaultTrashListRestorePurge(t *testing.T) {
	root := t.TempDir()
	u, _ := vaultWriteUsecase(root)
	trash := filepath.Join(root, ".aranea", "trash", "notes")
	require.NoError(t, os.MkdirAll(trash, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(trash, "B.md"), []byte("v1"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(trash, "B.1763600000000000000.md"), []byte("v2"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(trash, "C.md"), []byte("gone"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "notes"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "notes", "B.md"), []byte("now"), 0o644))
	ctx := context.Background()

	all, err := u.ListVaultTrash(ctx, "col-1", "")
	require.NoError(t, err)
	assert.Len(t, all, 3)
	versions, err := u.ListVaultTrash(ctx, "col-1", "notes/B.md")
	require.NoError(t, err)
	require.Len(t, versions, 2)
	for _, v := range versions {
		assert.Equal(t, "notes/B.md", v.OrigPath)
	}
	content, err := u.GetVaultTrashContent(ctx, "col-1", "notes/B.1763600000000000000.md")
	require.NoError(t, err)
	assert.Equal(t, "v2", content)
	_, err = u.GetVaultTrashContent(ctx, "col-1", "../../escape.md")
	assert.Error(t, err)

	// 删掉的文件放回原处；原处有同名的改名留两份
	rel, err := u.RestoreVaultTrash(ctx, "col-1", "notes/C.md")
	require.NoError(t, err)
	assert.Equal(t, "notes/C.md", rel)
	rel, err = u.RestoreVaultTrash(ctx, "col-1", "notes/B.md")
	require.NoError(t, err)
	assert.Equal(t, "notes/B (2).md", rel)
	b, _ := os.ReadFile(filepath.Join(root, "notes", "B.md"))
	assert.Equal(t, "now", string(b), "现有文件不能被覆盖")

	n, err := u.PurgeVaultTrash(ctx, "col-1", nil)
	require.NoError(t, err)
	assert.EqualValues(t, 1, n)
	left, _ := u.ListVaultTrash(ctx, "col-1", "")
	assert.Empty(t, left)
}
