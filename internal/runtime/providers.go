package runtime

import (
	"database/sql"
	"fmt"

	graphtrpc "aranea-agents/internal/graph/trpc"
	sessiontrpc "aranea-agents/internal/session/trpc"
	"aranea-agents/pkg/loggateway"

	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

// NewTRPCSessionService builds the framework session service from the shared SQLite pool.
// Deprecated: prefer aranea-agents/internal/session/trpc.NewTRPCSessionService directly.
func NewTRPCSessionService(rawDB *sql.DB, lg loggateway.Logger) trpcsession.Service {
	return sessiontrpc.NewTRPCSessionService(rawDB, lg)
}

// NewGraphCheckpointSaver builds the graph checkpoint saver from the shared SQLite pool.
func NewGraphCheckpointSaver(rawDB *sql.DB, lg loggateway.Logger) (*graphtrpc.SQLiteCheckpointSaver, error) {
	if rawDB == nil {
		return nil, fmt.Errorf("runtime: sqlite raw db is nil")
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return graphtrpc.NewSQLiteCheckpointSaver(rawDB, lg)
}
