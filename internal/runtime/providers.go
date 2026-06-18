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
func NewTRPCSessionService(pgDSN string, lg loggateway.Logger, summarizerCfg sessiontrpc.SummarizerConfig) trpcsession.Service {
	return sessiontrpc.NewTRPCSessionService(pgDSN, lg, summarizerCfg)
}

// NewGraphCheckpointSaver builds the graph checkpoint saver.
// pgDSN must be non-empty; Postgres is the only supported backend after A6.
// Returns an error if pgDSN is empty.
func NewGraphCheckpointSaver(rawDB *sql.DB, pgDSN string, lg loggateway.Logger) (*graphtrpc.CheckpointSaver, error) {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	// nil-db check and pgDSN-empty check are delegated to
	// graphtrpc.NewCheckpointSaver which returns proper apierror.BadRequest.
	return graphtrpc.NewCheckpointSaver(rawDB, pgDSN, lg)
}
