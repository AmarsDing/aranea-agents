package data

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
)

func TestKnowledgeRepo_ListChunksByDocumentsForSearch_ScopeAndACL(t *testing.T) {
	repo := setupKnowledgeSearchRepo(t)
	seedPathPrefixChunks(t, repo)
	ctx := context.Background()
	docs := []string{"d1", "d2", "d3", "d4"}
	if _, err := repo.data.Postgres().ExecContext(ctx,
		`UPDATE knowledge_chunks SET metadata = '{"category":"approved"}' WHERE id IN ('k1','k2')`); err != nil {
		t.Fatal(err)
	}
	q := biz.KnowledgeSearchQuery{CollectionID: "c1", PathPrefix: "notes", FilterJSON: `{"category":"approved"}`}
	got, err := repo.ListChunksByDocumentsForSearch(ctx, q, docs, 1)
	if err != nil {
		t.Fatal(err)
	}
	assertChunkIDs(t, got, "k1")
	q.PathPrefix = ""
	q.ExcludePathPrefixes = []string{"notes/"}
	got, err = repo.ListChunksByDocumentsForSearch(ctx, q, docs, 1)
	if err != nil {
		t.Fatal(err)
	}
	assertChunkIDs(t, got, "k2")
	if _, err := repo.data.Postgres().ExecContext(ctx,
		`UPDATE knowledge_documents SET visibility = 'private', owner_user_id = '42' WHERE id = 'd2'`); err != nil {
		t.Fatal(err)
	}
	got, err = repo.ListChunksByDocumentsForSearch(ctx, q, docs, 1)
	if err != nil {
		t.Fatal(err)
	}
	assertChunkIDs(t, got)
}
