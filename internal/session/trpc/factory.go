package session

import (
	"database/sql"

	"aranea-agents/pkg/loggateway"

	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

// NewTRPCSessionService builds the framework session service from the shared SQLite pool.
func NewTRPCSessionService(rawDB *sql.DB, lg loggateway.Logger) trpcsession.Service {
	if rawDB == nil {
		return NewInMemorySessionService()
	}
	svc, err := NewSQLiteSessionService(rawDB)
	if err != nil {
		lg.Warn("trpc SQLite session service unavailable, using in-memory fallback", loggateway.StepID("session.factory"), loggateway.Err(err))
		return NewInMemorySessionService()
	}
	return svc
}
