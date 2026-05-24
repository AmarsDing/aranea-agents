package session

import (
	"database/sql"
	"log"

	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

// NewTRPCSessionService builds the framework session service from the shared SQLite pool.
func NewTRPCSessionService(rawDB *sql.DB) trpcsession.Service {
	if rawDB == nil {
		return NewInMemorySessionService()
	}
	svc, err := NewSQLiteSessionService(rawDB)
	if err != nil {
		log.Printf("[session] trpc SQLite session service unavailable (%v); using in-memory store — runner state will not persist across restarts", err)
		return NewInMemorySessionService()
	}
	return svc
}
