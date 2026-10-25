package knowledge

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/auth"
	"github.com/stretchr/testify/require"
)

type lifecyclePathRepo struct {
	Repo
	replace func(context.Context, string, string, string, string) error
}

type lifecycleApplier struct{ apply func() error }

func (a lifecycleApplier) ApplyOne(context.Context, Collection, string) error { return a.apply() }

func TestLifecycle_SaveReturnsSubmittedHashWhenFileChangesAgain(t *testing.T) {
	root := t.TempDir()
	u, mr := vaultWriteUsecase(root)
	require.NoError(t, os.WriteFile(filepath.Join(root, "a.md"), []byte("old"), 0o644))
	doc := Document{ID: "d", CollectionID: "c", RelPath: "a.md", MimeType: "text/markdown"}
	mr.docGetFn = func(context.Context, string) (Document, error) { return doc, nil }
	mr.docGetByRelFn = func(context.Context, string, string) (Document, error) { return doc, nil }
	var submittedHash string
	u.SetVaultApplier(lifecycleApplier{apply: func() error {
		data, err := os.ReadFile(filepath.Join(root, "a.md"))
		if err != nil {
			return err
		}
		submittedHash = HashContent(string(data))
		return os.WriteFile(filepath.Join(root, "a.md"), []byte("external later version"), 0o644)
	}})
	updated, conflict, err := u.UpdateVaultDocumentContent(context.Background(), "d", "submitted", HashContent("old"))
	require.NoError(t, err)
	require.False(t, conflict)
	require.Equal(t, submittedHash, updated.ContentHash)
	require.NotEqual(t, HashContent("external later version"), updated.ContentHash)
}

func (r *lifecyclePathRepo) ReplaceDocumentPath(ctx context.Context, col, src, dst, rel string) error {
	return r.replace(ctx, col, src, dst, rel)
}

func TestLifecycle_BinaryOriginalCannotBeEdited(t *testing.T) {
	for _, ext := range []string{".pdf", ".png", ".docx", ".xlsx", ".pptx"} {
		t.Run(ext, func(t *testing.T) {
			root := t.TempDir()
			raw := []byte{0, 255, 42, 13}
			rel := "original" + ext
			require.NoError(t, os.WriteFile(filepath.Join(root, rel), raw, 0o644))
			u, mr := vaultWriteUsecase(root)
			mr.docGetFn = func(context.Context, string) (Document, error) {
				return Document{ID: "d", CollectionID: "c", RelPath: rel, Source: rel, MimeType: "text/markdown"}, nil
			}
			_, _, err := u.GetVaultDocumentRaw(context.Background(), "d")
			require.True(t, apierror.IsCode(err, apierror.CodeBadRequest))
			_, _, err = u.UpdateVaultDocumentContent(context.Background(), "d", "overwrite", HashContent(string(raw)))
			require.True(t, apierror.IsCode(err, apierror.CodeBadRequest))
			got, err := os.ReadFile(filepath.Join(root, rel))
			require.NoError(t, err)
			require.Equal(t, raw, got)
		})
	}
}

func TestLifecycle_DeleteLocalFileAndCompensateDBFailure(t *testing.T) {
	for _, failDB := range []bool{false, true} {
		name := "success"
		if failDB {
			name = "database_failure"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			u, mr := vaultWriteUsecase(root)
			require.NoError(t, os.WriteFile(filepath.Join(root, "a.md"), []byte("original"), 0o644))
			mr.docGetFn = func(context.Context, string) (Document, error) {
				return Document{ID: "d", CollectionID: "c", RelPath: "a.md"}, nil
			}
			mr.docDeleteFn = func(context.Context, string) error {
				_, err := os.Stat(filepath.Join(root, "a.md"))
				require.True(t, os.IsNotExist(err), "remove authoritative file before DB delete")
				if failDB {
					return apierror.Internal("KNOWLEDGE", "injected DB failure")
				}
				return nil
			}
			err := u.DeleteDocument(context.Background(), "d")
			if failDB {
				require.Error(t, err)
				got, readErr := os.ReadFile(filepath.Join(root, "a.md"))
				require.NoError(t, readErr)
				require.Equal(t, "original", string(got))
			} else {
				require.NoError(t, err)
				snaps, scanErr := NewSyncEngine(nil).Scan(root, nil)
				require.NoError(t, scanErr)
				require.Empty(t, snaps, "restart cannot import deleted file again")
				trash, readErr := os.ReadFile(filepath.Join(root, ".aranea", "trash", "a.md"))
				require.NoError(t, readErr)
				require.Equal(t, "original", string(trash))
			}
		})
	}
}

func TestLifecycle_OverwriteKeepsSourceIdentityAndRestoresOnDBFailure(t *testing.T) {
	for _, failDB := range []bool{false, true} {
		name := "success"
		if failDB {
			name = "database_failure"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			_, mr, _ := moveDocUsecase(t, root)
			require.NoError(t, os.MkdirAll(filepath.Join(root, "archive"), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(root, "archive", "B.md"), []byte("target original"), 0o644))
			mr.docGetByRelFn = func(context.Context, string, string) (Document, error) {
				return Document{ID: "target", CollectionID: "col-1", RelPath: "archive/B.md"}, nil
			}
			called := false
			repo := &lifecyclePathRepo{Repo: mr, replace: func(_ context.Context, col, src, dst, rel string) error {
				called = true
				require.Equal(t, []string{"col-1", "source", "target", "archive/B.md"}, []string{col, src, dst, rel})
				if failDB {
					return apierror.Internal("KNOWLEDGE", "injected DB failure")
				}
				return nil
			}}
			u := NewUsecaseFromRepo(repo)
			u.SetVaultFiler(NewVaultFiler(nil))
			doc, err := u.MoveVaultDocumentToDir(context.Background(), "source", "archive", "overwrite")
			require.True(t, called)
			got, readErr := os.ReadFile(filepath.Join(root, "archive", "B.md"))
			require.NoError(t, readErr)
			if failDB {
				require.Error(t, err)
				require.Equal(t, "target original", string(got))
				src, readErr := os.ReadFile(filepath.Join(root, "notes", "B.md"))
				require.NoError(t, readErr)
				require.Equal(t, "# B\n", string(src))
			} else {
				require.NoError(t, err)
				require.Equal(t, "source", doc.ID)
				require.Equal(t, "archive/B.md", doc.RelPath)
				require.Equal(t, "# B\n", string(got))
				trash, readErr := os.ReadFile(filepath.Join(root, ".aranea", "trash", "archive", "B.md"))
				require.NoError(t, readErr)
				require.Equal(t, "target original", string(trash))
			}
		})
	}
}

func TestLifecycle_CrossLocalCollectionMoveRejected(t *testing.T) {
	for _, localID := range []string{"source", "target"} {
		mr := noOpMockRepo()
		mr.docGetFn = func(context.Context, string) (Document, error) { return Document{ID: "d", CollectionID: "source"}, nil }
		mr.collGetFn = func(_ context.Context, id string) (Collection, error) {
			c := Collection{ID: id, VaultBackend: VaultBackendTeam}
			if id == localID {
				c.RootPath = t.TempDir()
				c.VaultBackend = VaultBackendLocal
			}
			return c, nil
		}
		mr.docMoveFn = func(context.Context, string, string) (Document, error) {
			t.Fatal("must reject before DB mutation")
			return Document{}, nil
		}
		_, err := NewUsecaseFromRepo(mr).MoveDocument(context.Background(), "d", "target")
		require.True(t, apierror.IsCode(err, apierror.CodeBadRequest))
	}
}

func TestLifecycle_MoveWithoutOverwriteRestoresSourceOnDBFailure(t *testing.T) {
	root := t.TempDir()
	u, mr, _ := moveDocUsecase(t, root)
	mr.docRelPathFn = func(context.Context, string, string) error {
		return apierror.Internal("KNOWLEDGE", "injected DB failure")
	}
	_, err := u.MoveVaultDocumentToDir(context.Background(), "source", "archive", "")
	require.Error(t, err)
	source, err := os.ReadFile(filepath.Join(root, "notes", "B.md"))
	require.NoError(t, err)
	require.Equal(t, "# B\n", string(source))
	_, err = os.Stat(filepath.Join(root, "archive", "B.md"))
	require.True(t, os.IsNotExist(err))
}

func TestLifecycle_OverwriteWithoutAtomicRepoKeepsBothFiles(t *testing.T) {
	root := t.TempDir()
	u, mr, _ := moveDocUsecase(t, root)
	require.NoError(t, os.MkdirAll(filepath.Join(root, "archive"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "archive", "B.md"), []byte("target"), 0o644))
	mr.docGetByRelFn = func(context.Context, string, string) (Document, error) { return Document{ID: "target"}, nil }
	_, err := u.MoveVaultDocumentToDir(context.Background(), "source", "archive", "overwrite")
	require.Error(t, err)
	source, err := os.ReadFile(filepath.Join(root, "notes", "B.md"))
	require.NoError(t, err)
	require.Equal(t, "# B\n", string(source))
	target, err := os.ReadFile(filepath.Join(root, "archive", "B.md"))
	require.NoError(t, err)
	require.Equal(t, "target", string(target))
}

func TestLifecycle_CreateDocumentStampsCallerOwner(t *testing.T) {
	mr := noOpMockRepo()
	u := NewUsecaseFromRepo(mr)
	ctx := auth.NewContext(context.Background(), &auth.Auth{UserID: 42})
	doc, err := u.CreateDocument(ctx, Document{CollectionID: "c", Source: "a.md", OwnerUserID: "spoofed"})
	require.NoError(t, err)
	require.Equal(t, "42", doc.OwnerUserID)
	mr.docGetFn = func(context.Context, string) (Document, error) { return doc, nil }
	u.SetDocumentACLStore(&stubACL{})
	private, err := u.UpdateDocumentVisibility(ctx, doc.ID, DocVisibilityPrivate, nil)
	require.NoError(t, err, "uploader can mark their own newly created document private")
	require.Equal(t, DocVisibilityPrivate, private.Visibility)
}
