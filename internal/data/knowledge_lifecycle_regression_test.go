package data

import (
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"github.com/stretchr/testify/require"
)

func TestKnowledgeLifecycle_LexicalFallbackAndOwnerPersistence(t *testing.T) {
	ctx, repo, colID, docID := setupParentChildRepo(t)
	_, err0 := repo.CommitIndexedDocument(ctx, colID, docID, []biz.KnowledgeChunk{{ID: "nil-vector", DocID: docID, CollectionID: colID, Content: "searchable during embedding outage"}}, 1, nil)
	require.NoError(t, err0)
	var nullVector bool
	// T4 ⑩ 代际命名后块 ID 不可预知，按正文锚定该行。
	require.NoError(t, repo.data.Postgres().QueryRowContext(ctx, `SELECT embedding IS NULL FROM knowledge_chunks WHERE content='searchable during embedding outage'`).Scan(&nullVector))
	require.True(t, nullVector)
	// G-39 批次统一语义（ADR-KN-20261021-G）：不再读集合台账 dim，只拒绝批内混维
	// （同一摄取批不可能产两种维度）；统一批（含新模式维度）合法通过——多代
	// 共存由侧表 (chunk_id, dim) 承载。空向量行（无语义层/父块）跳出不参与判定。
	err := repo.assertChunkDimensions(ctx, []biz.KnowledgeChunk{
		{ID: "mixed-2", CollectionID: colID, Embedding: []float32{1, 2}},
		{ID: "mixed-3", CollectionID: colID, Embedding: []float32{1, 2, 3}},
	})
	require.True(t, apierror.IsCode(err, apierror.CodeBadRequest), "mixed-dim batch must remain rejected")
	require.NoError(t, repo.assertChunkDimensions(ctx, []biz.KnowledgeChunk{
		{ID: "uniform-a", CollectionID: colID, Embedding: []float32{1, 2}},
		{ID: "uniform-b", CollectionID: colID, Embedding: []float32{3, 4}},
	}), "uniform batch at any dim must pass (multi-dim coexistence)")
	created, err := repo.CreateDocument(ctx, biz.KnowledgeDocument{ID: "owned", CollectionID: colID, Source: "own.md", Status: "pending", OwnerUserID: "42"})
	require.NoError(t, err)
	require.Equal(t, "42", created.OwnerUserID)
	loaded, err := repo.GetDocument(ctx, "owned")
	require.NoError(t, err)
	require.Equal(t, "42", loaded.OwnerUserID)
}

func TestKnowledgeLifecycle_OverwriteRemovesTargetInSameTransaction(t *testing.T) {
	ctx, repo, colID, docID := setupParentChildRepo(t)
	require.NoError(t, repo.UpdateDocumentRelPath(ctx, docID, "notes/a.md"))
	_, err1 := repo.CommitIndexedDocument(ctx, colID, docID, []biz.KnowledgeChunk{{ID: "source-ch", DocID: docID, CollectionID: colID, Content: "source"}}, 1, nil)
	require.NoError(t, err1)
	_, err := repo.CreateDocument(ctx, biz.KnowledgeDocument{ID: "target", CollectionID: colID, Source: "archive/a.md", RelPath: "archive/a.md", Status: "indexing"})
	require.NoError(t, err)
	_, err2 := repo.CommitIndexedDocument(ctx, colID, "target", []biz.KnowledgeChunk{{ID: "target-ch", DocID: "target", CollectionID: colID, Content: "target"}}, 1, nil)
	require.NoError(t, err2)
	require.NoError(t, repo.ReplaceDocumentPath(ctx, colID, docID, "target", "archive/a.md"))
	_, err = repo.GetDocument(ctx, "target")
	require.True(t, apierror.IsCode(err, apierror.CodeNotFound))
	source, err := repo.GetDocument(ctx, docID)
	require.NoError(t, err)
	require.Equal(t, "archive/a.md", source.RelPath)
	col, err := repo.GetCollection(ctx, colID)
	require.NoError(t, err)
	require.Equal(t, 1, col.DocumentCount)
	require.Equal(t, 1, col.ChunkCount)
}
