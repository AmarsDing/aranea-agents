package knowledge

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
)

// ScopedNeighborChunkLister applies the original search scope before choosing
// neighbor chunks. Implementations must also retain the document ACL predicate.
type ScopedNeighborChunkLister interface {
	ListChunksByDocumentsForSearch(context.Context, biz.KnowledgeSearchQuery, []string, int) ([]biz.KnowledgeChunk, error)
}

func hasSearchScope(q biz.KnowledgeSearchQuery) bool {
	if strings.TrimSpace(q.PathPrefix) != "" || strings.TrimSpace(q.FilterJSON) != "" {
		return true
	}
	for _, p := range q.ExcludePathPrefixes {
		if strings.TrimSpace(p) != "" {
			return true
		}
	}
	return false
}

func scopedNeighborChunks(ctx context.Context, source NeighborChunkLister, q biz.KnowledgeSearchQuery, docs []string, limit int) ([]biz.KnowledgeChunk, error) {
	if scoped, ok := source.(ScopedNeighborChunkLister); ok {
		return scoped.ListChunksByDocumentsForSearch(ctx, q, docs, limit)
	}
	if hasSearchScope(q) {
		return nil, apierror.Unavailable(apierror.DomainKnowledge, "scoped graph expansion is not configured")
	}
	return source.ListChunksByDocuments(ctx, q.CollectionID, docs, limit)
}
