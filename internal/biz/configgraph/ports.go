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

	// FindNode dual-resolves one node of one generation by (node_type, ref):
	// ref_id match wins over node_key on collision (NodeIndex semantics).
	// Returns apierror CodeNotFound when absent.
	FindNode(ctx context.Context, generation int64, nodeType, ref string) (Node, error)
	// WalkGraph traverses edges from startID within one generation:
	// reverse=true walks inbound edges (blast radius), false outbound edges
	// (dependencies). Cycles are cut via a CTE path array; broken edges
	// (dst_id='') never enter the walk. Rows are capped repo-side.
	WalkGraph(ctx context.Context, generation int64, startID string, reverse bool, maxDepth int) ([]WalkRow, error)
	// ListNodeEdges returns one node's adjacency within one generation:
	// outbound / inbound / broken-outbound edges (evidence preserved).
	ListNodeEdges(ctx context.Context, generation int64, nodeID string) (out, in, broken []StoredEdge, err error)
	// ListBrokenEdgesTargeting returns broken edges whose preserved dst_key
	// matches any of keys (intended-target attribution for the impact
	// broken section: references that tried to point at the target).
	ListBrokenEdgesTargeting(ctx context.Context, generation int64, keys []string) ([]StoredEdge, error)
	// CountActiveSessions counts non-deleted, non-archived sessions bound to
	// any of the given agent/team ref ids (design §5.1 signals; v1 reads the
	// sessions table directly — same DB, read-only probe).
	CountActiveSessions(ctx context.Context, agentIDs, teamIDs []string) (int64, error)

	// ListAllEdges returns every edge of one generation — health analysis
	// derives god-node fan stats, cycle DFS, and broken grouping in memory
	// (design §5.4 sanctions full scans at thousand-edge scale). No row cap.
	ListAllEdges(ctx context.Context, generation int64) ([]StoredEdge, error)
	// ListAllNodes returns every node of one generation (health analysis:
	// god/cycle node readability + duplicate-prompt grouping). No row cap.
	ListAllNodes(ctx context.Context, generation int64) ([]Node, error)
}
