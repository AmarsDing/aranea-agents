package session

import (
	"database/sql"

	"aranea-agents/internal/event"

	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

// NewTRPCSessionService builds the framework session service from the shared SQLite pool.
func NewTRPCSessionService(rawDB *sql.DB) trpcsession.Service {
	if rawDB == nil {
		return NewInMemorySessionService()
	}
	svc, err := NewSQLiteSessionService(rawDB)
	if err != nil {
		event.SysLogWarn("session.factory", "trpc SQLite session service unavailable, using in-memory fallback", event.P("error", err.Error()))
		return NewInMemorySessionService()
	}
	return svc
}
