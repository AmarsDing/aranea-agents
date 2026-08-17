package data

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
)

func TestListDocumentsPendingReembed_LexicalChunksAreHealthy(t *testing.T) {
	repo := setupKnowledgeSearchRepo(t)
	ctx := context.Background()
	for _, col := range []biz.KnowledgeCollection{
		{ID: "pending-lexical", Name: "lexical"},
		{ID: "pending-semantic", Name: "semantic"},
	} {
		if _, err := repo.CreateCollection(ctx, col); err != nil {
			t.Fatal(err)
		}
		docID := col.ID + "-doc"
		if _, err := repo.CreateDocument(ctx, biz.KnowledgeDocument{
			ID: docID, CollectionID: col.ID, Source: docID + ".md",
			ContentText: "searchable content", Status: "indexed",
		}); err != nil {
			t.Fatal(err)
		}
		if err := repo.InsertChunks(ctx, []biz.KnowledgeChunk{{
			ID: docID + "-chunk", DocID: docID, CollectionID: col.ID, Content: "searchable content",
		}}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := repo.EnableCollectionSemantic(ctx, "pending-semantic", "m", 3); err != nil {
		t.Fatal(err)
	}

	lexical, err := repo.ListDocumentsPendingReembed(ctx, "pending-lexical")
	if err != nil {
		t.Fatal(err)
	}
	if len(lexical) != 0 {
		t.Fatalf("lexical chunks intentionally have NULL embeddings and must be healthy: %+v", lexical)
	}
	semantic, err := repo.ListDocumentsPendingReembed(ctx, "pending-semantic")
	if err != nil {
		t.Fatal(err)
	}
	if len(semantic) != 1 {
		t.Fatalf("semantic NULL embedding must be repaired: %+v", semantic)
	}
}
