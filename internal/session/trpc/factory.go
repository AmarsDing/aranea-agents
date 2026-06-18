package session

import (
	"database/sql"

	"aranea-agents/pkg/loggateway"

	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

// NewTRPCSessionService builds the framework session service.
// When pgDSN is non-empty, a Postgres-backed service is created (the Postgres
// service manages its own connection pool from the DSN). Otherwise, the SQLite
// service is used with the provided rawDB.
func NewTRPCSessionService(rawDB *sql.DB, pgDSN string, lg loggateway.Logger, summarizerCfg SummarizerConfig) trpcsession.Service {
	if pgDSN != "" {
		svc, err := NewPostgresSessionService(pgDSN, lg, &summarizerCfg)
		if err != nil {
			lg.Warn("trpc Postgres session service unavailable, using in-memory fallback", loggateway.StepID("session.factory"), loggateway.Err(err))
			return NewInMemorySessionService()
		}
		return svc
	}
	if rawDB == nil {
		return NewInMemorySessionService()
	}
	svc, err := NewSQLiteSessionService(rawDB, lg, &summarizerCfg)
	if err != nil {
		lg.Warn("trpc SQLite session service unavailable, using in-memory fallback", loggateway.StepID("session.factory"), loggateway.Err(err))
		return NewInMemorySessionService()
	}
	return svc
}
