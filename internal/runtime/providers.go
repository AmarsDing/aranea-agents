package runtime

import (
	"database/sql"

	graphtrpc "aranea-agents/internal/graph/trpc"
	sessiontrpc "aranea-agents/internal/session/trpc"
	"aranea-agents/pkg/loggateway"

	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

// NewTRPCSessionService builds the framework session service.
// Deprecated: prefer aranea-agents/internal/session/trpc.NewTRPCSessionService directly.
func NewTRPCSessionService(rawDB *sql.DB, pgDSN string, lg loggateway.Logger, summarizerCfg sessiontrpc.SummarizerConfig) trpcsession.Service {
	return sessiontrpc.NewTRPCSessionService(rawDB, pgDSN, lg, summarizerCfg)
}

// NewGraphCheckpointSaver builds the graph checkpoint saver.
// When pgDSN is non-empty, a Postgres-backed saver is created; otherwise SQLite.
func NewGraphCheckpointSaver(rawDB *sql.DB, pgDSN string, lg loggateway.Logger) (*graphtrpc.CheckpointSaver, error) {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	// nil-db check is delegated to graphtrpc.NewCheckpointSaver which returns
	// a proper apierror.BadRequest.
	return graphtrpc.NewCheckpointSaver(rawDB, pgDSN, lg)
}
