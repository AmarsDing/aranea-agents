package runtime

import (
	"database/sql"

	"aranea-agents/internal/event/contract"
	graphtrpc "aranea-agents/internal/graph/trpc"
	"aranea-agents/pkg/loggateway"
)

// NewGraphCheckpointSaver builds the graph checkpoint saver.
// pgDSN must be non-empty; Postgres is the only supported backend after A6.
// Returns an error if pgDSN is empty.
// monitorBus is nil-safe: when nil, checkpoint flow-log emission is skipped.
func NewGraphCheckpointSaver(rawDB *sql.DB, pgDSN string, monitorBus contract.MonitorBus, lg loggateway.Logger) (*graphtrpc.CheckpointSaver, error) {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	// nil-db check and pgDSN-empty check are delegated to
	// graphtrpc.NewCheckpointSaver which returns proper apierror.BadRequest.
	return graphtrpc.NewCheckpointSaver(rawDB, pgDSN, monitorBus, lg)
}
