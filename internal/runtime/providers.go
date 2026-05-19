package runtime

import (
	"database/sql"
	"fmt"
	"log"

	graphtrpc "aranea-agents/internal/graph/trpc"
	sessiontrpc "aranea-agents/internal/session/trpc"

	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

// NewTRPCSessionService builds the framework session service from the shared SQLite pool.
func NewTRPCSessionService(rawDB *sql.DB) trpcsession.Service {
	if rawDB == nil {
		return sessiontrpc.NewInMemorySessionService()
	}
	svc, err := sessiontrpc.NewSQLiteSessionService(rawDB)
	if err != nil {
		log.Printf("trpc session sqlite init failed, falling back to in-memory: %v", err)
		return sessiontrpc.NewInMemorySessionService()
	}
	return svc
}

// NewGraphCheckpointSaver builds the graph checkpoint saver from the shared SQLite pool.
func NewGraphCheckpointSaver(rawDB *sql.DB) (*graphtrpc.SQLiteCheckpointSaver, error) {
	if rawDB == nil {
		return nil, fmt.Errorf("runtime: sqlite raw db is nil")
	}
	return graphtrpc.NewSQLiteCheckpointSaver(rawDB)
}
