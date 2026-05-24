package runtime

import (
	"database/sql"
	"fmt"

	graphtrpc "aranea-agents/internal/graph/trpc"
	sessiontrpc "aranea-agents/internal/session/trpc"

	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

// NewTRPCSessionService builds the framework session service from the shared SQLite pool.
// Deprecated: prefer aranea-agents/internal/session/trpc.NewTRPCSessionService directly.
func NewTRPCSessionService(rawDB *sql.DB) trpcsession.Service {
	return sessiontrpc.NewTRPCSessionService(rawDB)
}

// NewGraphCheckpointSaver builds the graph checkpoint saver from the shared SQLite pool.
func NewGraphCheckpointSaver(rawDB *sql.DB) (*graphtrpc.SQLiteCheckpointSaver, error) {
	if rawDB == nil {
		return nil, fmt.Errorf("runtime: sqlite raw db is nil")
	}
	return graphtrpc.NewSQLiteCheckpointSaver(rawDB)
}
