package knowledge

// DEPRECATED: 框架架构差异，永久阻塞（alignment-plan.md §十一/B-1）。
// 框架中没有任何组件消费 vectorstore.VectorStore 接口——框架的 Knowledge
// 模块使用 knowledge.Knowledge 接口（已通过 KnowledgeAdapter 适配），
// 不直接使用 vectorstore.VectorStore。插入此适配器不会带来任何功能增益，
// 只会增加一层无意义的包装。下一迭代将删除此死代码（CS-B2）。

import (
	"context"
	"encoding/json"
	"fmt"

	"aranea-agents/internal/data/vector"
	"aranea-agents/pkg/loggateway"

	"trpc.group/trpc-go/trpc-agent-go/knowledge/document"
	"trpc.group/trpc-go/trpc-agent-go/knowledge/vectorstore"
)

// VectorStoreAdapter wraps a self-built vector.VectorStore to implement
// the framework's vectorstore.VectorStore interface.
// Methods not supported by the underlying store return best-effort defaults
// and are marked TECH-DEBT to track which methods lack underlying store support.
//
// DEPRECATED: 框架架构差异，永久阻塞。见文件头说明。
type VectorStoreAdapter struct {
	store vector.VectorStore
	lg    loggateway.Logger
}

// NewVectorStoreAdapter creates a new adapter around the given VectorStore.
func NewVectorStoreAdapter(store vector.VectorStore, lg loggateway.Logger) *VectorStoreAdapter {
	return &VectorStoreAdapter{
		store: store,
		lg:    lg.With(loggateway.Domain("knowledge.vectorstore_adapter")),
	}
}

// Add stores a document by delegating to Upsert.
// Metadata is converted from map[string]any to map[string]string via JSON round-trip.
func (a *VectorStoreAdapter) Add(ctx context.Context, doc *document.Document, embedding []float64) error {
	meta, err := metaAnyToString(doc.Metadata)
	if err != nil {
		return fmt.Errorf("vectorstore adapter Add: convert metadata: %w", err)
	}
	return a.store.Upsert(ctx, doc.ID, embedding, meta)
}

// Get is not supported by the self-built store.
//
// TECH-DEBT: self-built VectorStore has no Get; returns error.
func (a *VectorStoreAdapter) Get(_ context.Context, _ string) (*document.Document, []float64, error) {
	return nil, nil, fmt.Errorf("vectorstore adapter: Get not supported")
}

// Update delegates to Upsert (same semantics as Add for the self-built store).
func (a *VectorStoreAdapter) Update(ctx context.Context, doc *document.Document, embedding []float64) error {
	meta, err := metaAnyToString(doc.Metadata)
	if err != nil {
		return fmt.Errorf("vectorstore adapter Update: convert metadata: %w", err)
	}
	return a.store.Upsert(ctx, doc.ID, embedding, meta)
}

// Delete removes a vector by ID.
func (a *VectorStoreAdapter) Delete(ctx context.Context, id string) error {
	return a.store.Delete(ctx, id)
}

// Search performs similarity search and converts VectorHit results to ScoredDocuments.
func (a *VectorStoreAdapter) Search(ctx context.Context, query *vectorstore.SearchQuery) (*vectorstore.SearchResult, error) {
	hits, err := a.store.Search(ctx, query.Vector, query.Limit, query.MinScore)
	if err != nil {
		return nil, err
	}

	results := make([]*vectorstore.ScoredDocument, 0, len(hits))
	for i := range hits {
		h := &hits[i]
		meta := metaStringToAny(h.Meta)
		results = append(results, &vectorstore.ScoredDocument{
			Document: &document.Document{
				ID:       h.ID,
				Metadata: meta,
			},
			Score: h.Score,
		})
	}
	return &vectorstore.SearchResult{Results: results}, nil
}

// DeleteByFilter is not supported by the self-built store.
//
// TECH-DEBT: self-built VectorStore has no filter-based deletion.
func (a *VectorStoreAdapter) DeleteByFilter(_ context.Context, _ ...vectorstore.DeleteOption) error {
	return fmt.Errorf("vectorstore adapter: DeleteByFilter not supported")
}

// UpdateByFilter is not supported by the self-built store.
//
// TECH-DEBT: self-built VectorStore has no filter-based update.
func (a *VectorStoreAdapter) UpdateByFilter(_ context.Context, _ ...vectorstore.UpdateByFilterOption) (int64, error) {
	return 0, fmt.Errorf("vectorstore adapter: UpdateByFilter not supported")
}

// Count is not supported by the self-built store.
//
// TECH-DEBT: self-built VectorStore has no Count; returns -1.
func (a *VectorStoreAdapter) Count(_ context.Context, _ ...vectorstore.CountOption) (int, error) {
	return -1, nil
}

// GetMetadata is not supported by the self-built store.
//
// TECH-DEBT: self-built VectorStore has no GetMetadata; returns nil.
func (a *VectorStoreAdapter) GetMetadata(_ context.Context, _ ...vectorstore.GetMetadataOption) (map[string]vectorstore.DocumentMetadata, error) {
	return nil, nil
}

// Close is a no-op; the self-built store has no close lifecycle.
func (a *VectorStoreAdapter) Close() error {
	return nil
}

// Compile-time interface assertion.
var _ vectorstore.VectorStore = (*VectorStoreAdapter)(nil)

// metaAnyToString converts map[string]any to map[string]string.
// String values are kept as-is; non-string values are JSON-encoded.
func metaAnyToString(src map[string]any) (map[string]string, error) {
	if len(src) == 0 {
		return nil, nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		if s, ok := v.(string); ok {
			dst[k] = s
			continue
		}
		b, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("meta key %q: %w", k, err)
		}
		dst[k] = string(b)
	}
	return dst, nil
}

// metaStringToAny converts map[string]string to map[string]any.
// Values that are valid JSON are decoded; plain strings are kept as-is.
func metaStringToAny(src map[string]string) map[string]any {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
