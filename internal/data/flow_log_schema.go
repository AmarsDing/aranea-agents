package data

import (
	"context"
	_ "embed"

	"aranea-agents/internal/data/ent"
)

//go:embed sql/flow_log.sql
var flowLogDDL string

// EnsureFlowLogSchema creates flow_log_events if missing.
func EnsureFlowLogSchema(ctx context.Context, client *ent.Client) error {
	return execDDLFile(ctx, client, flowLogDDL, "flow_log")
}
