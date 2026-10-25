package knowledge

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

type authorizationKnowledgeRepo struct {
	stubKnowledgeRepo
	collectionErr error
	searches      int
}

func (r *authorizationKnowledgeRepo) GetCollection(context.Context, string) (biz.KnowledgeCollection, error) {
	return r.collection, r.collectionErr
}

func (r *authorizationKnowledgeRepo) SearchChunks(context.Context, biz.KnowledgeSearchQuery, []float32) ([]biz.KnowledgeChunk, error) {
	r.searches++
	return r.chunks, nil
}

func (r *authorizationKnowledgeRepo) SearchChunksBM25(context.Context, biz.KnowledgeSearchQuery) ([]biz.KnowledgeChunk, error) {
	r.searches++
	return r.chunks, nil
}

func TestRetrievalAuthorization_AllBranchesFailClosed(t *testing.T) {
	for _, branch := range []string{"retriever_dense", "retriever_lexical", "hybrid_dense", "hybrid_lexical"} {
		for _, rejected := range []string{"cross_workspace", "lookup_failed"} {
			t.Run(branch+"/"+rejected, func(t *testing.T) {
				repo := &authorizationKnowledgeRepo{stubKnowledgeRepo: stubKnowledgeRepo{
					collection: biz.KnowledgeCollection{ID: "other", Workspace: "ws-other", EmbeddingModel: "m"},
					chunks:     []biz.KnowledgeChunk{{ID: "secret", Content: "must never be returned"}},
				}}
				if rejected == "lookup_failed" {
					repo.collectionErr = apierror.NotFound(apierror.DomainKnowledge, "not found")
				}
				var embedder QueryEmbedder = stubEmbedder{}
				if branch == "retriever_lexical" || branch == "hybrid_lexical" {
					embedder = nil
				}
				ret := NewRetriever(embedder, repo, nil, loggateway.NewNoop())
				q := biz.KnowledgeSearchQuery{CollectionID: "other", Query: "facts", TopK: 5}
				ctx := workspace.WithContext(context.Background(), "ws-caller")
				var err error
				if branch == "hybrid_dense" || branch == "hybrid_lexical" {
					_, err = NewHybridRetriever(ret, repo, loggateway.NewNoop()).Search(ctx, q, HybridDense)
				} else {
					_, err = ret.Search(ctx, q)
				}
				if err == nil || repo.searches != 0 {
					t.Fatalf("authorization must stop before any retrieval: err=%v searches=%d", err, repo.searches)
				}
			})
		}
	}
}

func TestRetrievalAuthorization_SharedAndSystemReads(t *testing.T) {
	for _, system := range []bool{false, true} {
		repo := &authorizationKnowledgeRepo{stubKnowledgeRepo: stubKnowledgeRepo{
			collection: biz.KnowledgeCollection{ID: "col", EmbeddingModel: "m"},
			chunks:     []biz.KnowledgeChunk{{ID: "allowed"}},
		}}
		ctx := workspace.WithContext(context.Background(), "ws-caller")
		if system {
			ctx = workspace.WithSystemWorkspace(ctx)
			repo.collection.Workspace = "ws-other"
		}
		got, err := NewRetriever(stubEmbedder{}, repo, nil, loggateway.NewNoop()).Search(ctx,
			biz.KnowledgeSearchQuery{CollectionID: "col", Query: "facts", TopK: 5})
		if err != nil || len(got) != 1 {
			t.Fatalf("shared/system read rejected: system=%v got=%v err=%v", system, got, err)
		}
	}
}
