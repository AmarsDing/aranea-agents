package knowledge

import (
	"context"
	"testing"

	"aranea-agents/pkg/apierror"
	"github.com/stretchr/testify/require"
)

func TestDocumentContentVersion_IsScopedToOneDocument(t *testing.T) {
	ctx := WithDocumentContentVersion(context.Background(), "doc-a", "body snapshot")
	hash, ok := DocumentContentVersionFromContext(ctx, "doc-a")
	require.True(t, ok)
	require.Equal(t, HashContent("body snapshot"), hash)
	_, ok = DocumentContentVersionFromContext(ctx, "doc-b")
	require.False(t, ok, "one document's expected version must not constrain another")
	_, ok = DocumentContentVersionFromContext(context.Background(), "doc-a")
	require.False(t, ok)
}

func TestDocumentContentVersion_StaleCommitRejectsBeforePendingFSM(t *testing.T) {
	mr := noOpMockRepo()
	mr.docGetFn = func(context.Context, string) (Document, error) {
		return Document{ID: "doc", CollectionID: "col", ContentText: "new body", Status: "pending"}, nil
	}
	ctx := WithDocumentContentVersion(context.Background(), "doc", "old body")
	_, err := NewUsecaseFromRepo(mr).CommitIndexedDocument(ctx, "col", "doc", nil, 0)
	require.True(t, apierror.IsCode(err, apierror.CodeConflict), "stale worker must report version conflict, not an unrelated pending FSM error")
}

func TestDocumentContentVersion_StaleStartCannotClaimNewPendingBody(t *testing.T) {
	mr := noOpMockRepo()
	mr.docGetFn = func(context.Context, string) (Document, error) {
		return Document{ID: "doc", ContentText: "new body", Status: "pending"}, nil
	}
	called := false
	mr.docUpdateFn = func(context.Context, string, string, string, int) error { called = true; return nil }
	ctx := WithDocumentContentVersion(context.Background(), "doc", "old body")
	err := NewUsecaseFromRepo(mr).UpdateDocumentStatus(ctx, "doc", "indexing", "", 0)
	require.True(t, apierror.IsCode(err, apierror.CodeConflict))
	require.False(t, called, "stale worker must leave the new pending revision repairable")
}
