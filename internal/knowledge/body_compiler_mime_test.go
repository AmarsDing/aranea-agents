package knowledge

import (
	"context"
	"testing"

	bizknowledge "aranea-agents/internal/biz/knowledge"
	"github.com/stretchr/testify/require"
)

type compilerMIMEExtractor struct{}

func (compilerMIMEExtractor) Supports(string, string) bool { return true }
func (compilerMIMEExtractor) Extract(context.Context, []byte, string, string) (string, error) {
	return "# Extracted body", nil
}

func TestBodyCompiler_PreservesOriginalMIME(t *testing.T) {
	c := NewBodyCompiler(NewExtractorRegistry(compilerMIMEExtractor{}))
	for _, tc := range []struct{ path, mime string }{
		{"a.pdf", "application/pdf"}, {"a.png", "image/png"},
		{"a.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
	} {
		body, mime, err := c.Compile(context.Background(), tc.path, []byte("binary"))
		require.NoError(t, err)
		require.Equal(t, "# Extracted body", body)
		require.Equal(t, tc.mime, mime)
	}
}

func TestVaultSync_OriginalMIMEEvenWhenCompilerReturnsMarkdown(t *testing.T) {
	root := t.TempDir()
	writeVaultFile(t, root, "report.pdf", "%PDF-fake")
	repo := newVaultSyncMemRepo()
	vault := bizknowledge.Collection{ID: "mime-col", RootPath: root}
	repo.collections[vault.ID] = vault
	applier := newTestApplier(repo, nil)
	applier.SetCompiler(&stubCompiler{returnMD: "# Extracted report"})
	require.NoError(t, applier.ApplyEvents(context.Background(), vault, []bizknowledge.ChangeEvent{createdEvent("report.pdf", "%PDF-fake")}))
	for _, doc := range repo.documents {
		require.Equal(t, "application/pdf", doc.MimeType)
		require.False(t, bizknowledge.VaultDocumentEditable(doc))
	}
}
