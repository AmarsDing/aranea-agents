package data

import (
	"context"
	"database/sql"
	"sync"
	"sync/atomic"
	"testing"

	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/safego"
	"github.com/stretchr/testify/require"
)

func setupTeamNoteRepo(t *testing.T) (context.Context, *knowledgeRepo, string, string) {
	t.Helper()
	ctx, r, colID, docID := setupParentChildRepo(t)
	r.data.rwDB = NewReadWriteDB(r.data.Postgres(), r.data.Postgres())
	_, err := r.data.Postgres().ExecContext(ctx, `UPDATE knowledge_collections SET workspace='ws-a', vault_backend='team' WHERE id=$1`, colID)
	require.NoError(t, err)
	ctx = workspace.WithContext(ctx, "ws-a")
	require.NoError(t, r.UpdateDocumentContent(ctx, docID, "old content", true))
	_, err = r.CommitIndexedDocument(ctx, colID, docID, []biz.KnowledgeChunk{{ID: "old-chunk", DocID: docID, CollectionID: colID, Content: "old content", Embedding: []float32{1, 0, 0}}}, 1, nil)
	require.NoError(t, err)
	return ctx, r, colID, docID
}

func assertTeamNoteState(t *testing.T, ctx context.Context, r *knowledgeRepo, colID, docID, content, status string, chunks, docs int) {
	t.Helper()
	doc, err := r.GetDocument(ctx, docID)
	require.NoError(t, err)
	require.Equal(t, content, doc.ContentText)
	require.Equal(t, status, doc.Status)
	require.Equal(t, chunks, doc.ChunkCount)
	var actualChunks int
	require.NoError(t, r.data.Postgres().QueryRowContext(ctx, `SELECT count(*) FROM knowledge_chunks WHERE doc_id=$1`, docID).Scan(&actualChunks))
	require.Equal(t, chunks, actualChunks)
	col, err := r.GetCollection(ctx, colID)
	require.NoError(t, err)
	require.Equal(t, docs, col.DocumentCount)
	require.Equal(t, chunks, col.ChunkCount)
}

func TestKnowledgeTeamNote_CASInvalidatesOldChunksAndRemainsRepairable(t *testing.T) {
	ctx, r, colID, docID := setupTeamNoteRepo(t)
	applied, err := r.ReplaceTeamDocumentContent(ctx, docID, "stale", "must not overwrite")
	require.NoError(t, err)
	require.False(t, applied)
	assertTeamNoteState(t, ctx, r, colID, docID, "old content", "indexed", 1, 1)
	applied, err = r.ReplaceTeamDocumentContent(ctx, docID, bizknowledge.HashContent("old content"), "new accepted content")
	require.NoError(t, err)
	require.True(t, applied)
	assertTeamNoteState(t, ctx, r, colID, docID, "new accepted content", "pending", 0, 0)
	pending, err := r.ListDocumentsPendingReembed(ctx, colID, 3)
	require.NoError(t, err)
	require.Len(t, pending, 1, "accepted body with failed replay must be repair-worker eligible")
	require.Equal(t, bizknowledge.HashContent("new accepted content"), pending[0].ContentHash)
	// A later repair restores indexed status and counts exactly once.
	require.NoError(t, r.UpdateDocumentStatus(ctx, docID, "indexing", "", 0))
	_, err = r.CommitIndexedDocument(ctx, colID, docID, []biz.KnowledgeChunk{{ID: "new-chunk", DocID: docID, CollectionID: colID, Content: "new accepted content", Embedding: []float32{0, 1, 0}}}, 1, nil)
	require.NoError(t, err)
	assertTeamNoteState(t, ctx, r, colID, docID, "new accepted content", "indexed", 1, 1)
}

func TestKnowledgeTeamNote_StorageFailureRollsBackChunkDeletion(t *testing.T) {
	ctx, r, colID, docID := setupTeamNoteRepo(t)
	applied, err := r.ReplaceTeamDocumentContent(ctx, docID, bizknowledge.HashContent("old content"), "invalid\x00text")
	require.Error(t, err, "Postgres rejects a NUL byte after the attempted chunk deletion")
	require.False(t, applied)
	assertTeamNoteState(t, ctx, r, colID, docID, "old content", "indexed", 1, 1)
}

func TestKnowledgeTeamNote_ConcurrentCASOnlyOneWriterCommits(t *testing.T) {
	ctx, r, colID, docID := setupTeamNoteRepo(t)
	var wg sync.WaitGroup
	var wins, conflicts, failures atomic.Int32
	for _, body := range []string{"writer A", "writer B"} {
		content := body
		wg.Add(1)
		safego.Go(ctx, "team-note-pg-cas-test", func() {
			defer wg.Done()
			applied, err := r.ReplaceTeamDocumentContent(ctx, docID, bizknowledge.HashContent("old content"), content)
			if err != nil {
				failures.Add(1)
			} else if applied {
				wins.Add(1)
			} else {
				conflicts.Add(1)
			}
		})
	}
	wg.Wait()
	require.Zero(t, failures.Load())
	require.EqualValues(t, 1, wins.Load())
	require.EqualValues(t, 1, conflicts.Load())
	doc, err := r.GetDocument(ctx, docID)
	require.NoError(t, err)
	require.Contains(t, []string{"writer A", "writer B"}, doc.ContentText)
	assertTeamNoteState(t, ctx, r, colID, docID, doc.ContentText, "pending", 0, 0)
}

func TestKnowledgeTeamNote_RejectForeignSharedAndLocalCollections(t *testing.T) {
	for _, tc := range []struct{ name, ws, root string }{
		{"foreign", "ws-b", ""}, {"shared", "", ""}, {"local", "ws-a", "F:/local-vault"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, r, colID, docID := setupTeamNoteRepo(t)
			_, err := r.data.Postgres().ExecContext(ctx, `UPDATE knowledge_collections SET workspace=$2, root_path=$3 WHERE id=$1`, colID, tc.ws, tc.root)
			require.NoError(t, err)
			applied, err := r.ReplaceTeamDocumentContent(ctx, docID, bizknowledge.HashContent("old content"), "forbidden")
			require.Error(t, err)
			require.False(t, applied)
			assertTeamNoteState(t, workspace.WithSystemWorkspace(ctx), r, colID, docID, "old content", "indexed", 1, 1)
		})
	}
}

func TestKnowledgeTeamNote_OuterTransactionOwnsBodyAndDeleteRollback(t *testing.T) {
	ctx, r, colID, docID := setupTeamNoteRepo(t)
	err := r.knowledgeExecInTx(ctx, func(txCtx context.Context, _ *sql.Tx) error {
		applied, err := r.ReplaceTeamDocumentContent(txCtx, docID, bizknowledge.HashContent("old content"), "uncommitted")
		require.NoError(t, err)
		require.True(t, applied)
		read, err := r.GetDocument(txCtx, docID)
		require.NoError(t, err)
		require.Equal(t, "uncommitted", read.ContentText, "document reads use outer transaction")
		require.NoError(t, r.DeleteDocument(txCtx, docID))
		_, err = r.GetDocument(txCtx, docID)
		require.True(t, apierror.IsCode(err, apierror.CodeNotFound))
		return apierror.Internal("KNOWLEDGE", "injected governance rollback")
	})
	require.Error(t, err)
	assertTeamNoteState(t, ctx, r, colID, docID, "old content", "indexed", 1, 1)
}

func TestKnowledgeTeamNote_StaleChunkCommitCannotReplaceNewerBody(t *testing.T) {
	ctx, r, colID, docID := setupTeamNoteRepo(t)
	staleCtx := bizknowledge.WithDocumentContentVersion(ctx, docID, "old content")
	applied, err := r.ReplaceTeamDocumentContent(ctx, docID, bizknowledge.HashContent("old content"), "new body")
	require.NoError(t, err)
	require.True(t, applied)
	require.NoError(t, r.UpdateDocumentStatus(ctx, docID, "indexing", "", 0))
	newCtx := bizknowledge.WithDocumentContentVersion(ctx, docID, "new body")
	_, err = r.CommitIndexedDocument(newCtx, colID, docID, []biz.KnowledgeChunk{{ID: "current-version-chunk", DocID: docID, CollectionID: colID, Content: "new body", Embedding: []float32{0, 1, 0}}}, 1, nil)
	require.NoError(t, err)
	require.NoError(t, r.UpdateDocumentStatus(ctx, docID, "indexing", "", 1))
	_, err = r.CommitIndexedDocument(staleCtx, colID, docID, []biz.KnowledgeChunk{{ID: "stale-version-chunk", DocID: docID, CollectionID: colID, Content: "old content", Embedding: []float32{1, 0, 0}}}, 0, nil)
	require.True(t, apierror.IsCode(err, apierror.CodeConflict))
	assertTeamNoteState(t, ctx, r, colID, docID, "new body", "indexing", 1, 1)
	var chunkID, chunkContent string
	require.NoError(t, r.data.Postgres().QueryRowContext(ctx, `SELECT id, content FROM knowledge_chunks WHERE doc_id=$1`, docID).Scan(&chunkID, &chunkContent))
	require.Equal(t, "pc-doc-v2-ch-0", chunkID, "conflict must occur before deleting existing chunks; v2 代际命名行必须存活")
	require.Equal(t, "new body", chunkContent, "surviving row must carry the current version body")
}

func TestKnowledgeTeamNote_StaleStatusCannotClaimNewPendingBody(t *testing.T) {
	ctx, r, colID, docID := setupTeamNoteRepo(t)
	staleCtx := bizknowledge.WithDocumentContentVersion(ctx, docID, "old content")
	applied, err := r.ReplaceTeamDocumentContent(ctx, docID, bizknowledge.HashContent("old content"), "new body")
	require.NoError(t, err)
	require.True(t, applied)
	err = r.UpdateDocumentStatus(staleCtx, docID, "indexing", "", 1)
	require.True(t, apierror.IsCode(err, apierror.CodeConflict))
	assertTeamNoteState(t, ctx, r, colID, docID, "new body", "pending", 0, 0)
}

func TestKnowledgeTeamNote_EmptyPendingNoteRemainsRepairable(t *testing.T) {
	ctx, r, colID, docID := setupTeamNoteRepo(t)
	applied, err := r.ReplaceTeamDocumentContent(ctx, docID, bizknowledge.HashContent("old content"), "")
	require.NoError(t, err)
	require.True(t, applied)
	pending, err := r.ListDocumentsPendingReembed(ctx, colID, 3)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	require.Empty(t, pending[0].ContentText)
	versionCtx := bizknowledge.WithDocumentContentVersion(ctx, docID, "")
	require.NoError(t, r.UpdateDocumentStatus(versionCtx, docID, "indexing", "", 0))
	_, err = r.CommitIndexedDocument(versionCtx, colID, docID, nil, 1, nil)
	require.NoError(t, err)
	assertTeamNoteState(t, ctx, r, colID, docID, "", "indexed", 0, 1)
}
