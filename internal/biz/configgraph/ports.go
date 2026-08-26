package configgraph

import "context"

// Repo persists graph nodes/edges and generation metadata.
// Implementation: raw-SQL repo in internal/data/configgraph (ent schemas only
// document the contract, per the event_delivery_outbox precedent).
//
// Generation semantics: full rebuild writes generation current+1, then the
// biz-layer rebuilder switches its in-memory current pointer (startup seeds it
// from MaxGeneration) and asynchronously deletes generations below current-1.
// Incremental recompute writes into the current generation via DeleteOutEdges
// + Upsert. Queries always read the current generation.
//
// Stability: Evolving
type Repo interface {
	// UpsertNodes batch-inserts/refresh nodes (conflict target
	// (node_type, ref_id, generation); created_at kept on conflict).
	// Rows without ID/NodeType/RefID are skipped.
	UpsertNodes(ctx context.Context, nodes []Node) error
	// UpsertEdges batch-inserts/refresh edges (conflict target
	// (src_id, dst_id, edge_type, generation)).
	UpsertEdges(ctx context.Context, edges []StoredEdge) error
	// MaxGeneration returns MAX(generation) over nodes (0 when empty) —
	// startup seed for the in-memory current-generation pointer.
	MaxGeneration(ctx context.Context) (int64, error)
	// DeleteGenerationBelow removes nodes+edges with generation < belowGen
	// (rebuild cleanup keeps one old generation for reconciliation).
	// Returns total rows deleted.
	DeleteGenerationBelow(ctx context.Context, belowGen int64) (int64, error)
	// DeleteOutEdges removes one node's outgoing edges within a generation
	// (incremental recompute: delete-then-insert).
	DeleteOutEdges(ctx context.Context, srcID string, generation int64) error
	// ListNodes searches nodes of one generation (nodes-search API).
	ListNodes(ctx context.Context, filter NodeFilter) ([]Node, error)
	// Counts aggregates node/edge/broken counts of one generation (status API).
	Counts(ctx context.Context, generation int64) (Counts, error)
}
