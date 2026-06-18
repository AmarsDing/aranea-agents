package session

import (
	"aranea-agents/pkg/loggateway"

	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

// NewTRPCSessionService builds the framework session service.
// Postgres is the only supported backend in production. When pgDSN is empty
// or Postgres init fails, an in-memory service is used as fallback (mainly
// for tests). The SQLite session service is no longer wired in production
// paths; it remains available in the framework for offline/test use.
func NewTRPCSessionService(pgDSN string, lg loggateway.Logger, summarizerCfg SummarizerConfig) trpcsession.Service {
	if pgDSN != "" {
		svc, err := NewPostgresSessionService(pgDSN, lg, &summarizerCfg)
		if err != nil {
			lg.Warn("trpc Postgres session service unavailable, using in-memory fallback", loggateway.StepID("session.factory"), loggateway.Err(err))
			return NewInMemorySessionService()
		}
		return svc
	}
	return NewInMemorySessionService()
}
