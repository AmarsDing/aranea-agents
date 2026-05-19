package memory

import (
	"context"

	"aranea-agents/internal/data/sessionmemory"
)

// FactUpsert is the L2 fact write payload for session-scoped memory.
type FactUpsert = sessionmemory.MemoryFactUpsert

// EvolutionEventInsert is the evolution timeline write payload.
type EvolutionEventInsert = sessionmemory.EvolutionEventInsert

// SessionAdminStore is the Aranea port for L0–L4 session memory admin APIs.
// Implementations live in internal/data/sessionmemory; service/runtime must not import that package.
type SessionAdminStore interface {
	ListL0SnapshotRows(ctx context.Context, sessionID string, limit int32) ([][]byte, error)
	ListL1TaskRows(ctx context.Context, sessionID, agentID, status, includeEnded string) ([][]byte, error)
	ListL1FieldRows(ctx context.Context, taskID string, includeInternal bool) ([][]byte, error)
	ListFactRows(ctx context.Context, scopeType, scopeID, kind, status, keyword string, limit, offset int32) ([][]byte, int32, int32, int32, error)
	ListEntityRows(ctx context.Context, scopeType, scopeID, workspaceID, userID, entityType, status, keyword string, limit, offset int32) ([][]byte, int32, error)
	NeighborhoodJSON(ctx context.Context, centerID string, hops, maxNodes int32) ([]byte, error)
	AgentIdentityJSON(ctx context.Context, agentID string) ([]byte, error)
	AgentStrategyJSON(ctx context.Context, agentID string) ([]byte, error)
	EvolutionProposalRows(ctx context.Context, agentID, status string, limit int32) ([][]byte, error)
	EvolutionEventRows(ctx context.Context, agentID string, limit int32) ([][]byte, error)
	EvolutionMetricsJSON(ctx context.Context, agentID string) ([]byte, error)
	UpsertFactRow(ctx context.Context, in FactUpsert) ([]byte, error)
	InsertEvolutionEventRow(ctx context.Context, in EvolutionEventInsert) ([]byte, error)
	DeleteSessionEventEntities(ctx context.Context, sessionID string) error
}

// WrapSessionAdminStore returns the data store as the admin port (nil-safe).
func WrapSessionAdminStore(store *sessionmemory.Store) SessionAdminStore {
	if store == nil {
		return nil
	}
	return store
}
