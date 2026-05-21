package data

import (
	"context"
	_ "embed"

	"aranea-agents/internal/data/ent"
)

//go:embed sql/hook_delivery.sql
var hookDeliveryDDL string

// EnsureHookDeliverySchema creates hook_deliveries table if missing.
func EnsureHookDeliverySchema(ctx context.Context, client *ent.Client) error {
	return execDDLFile(ctx, client, hookDeliveryDDL, "hook_delivery")
}
