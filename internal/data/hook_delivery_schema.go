package data

import (
	"context"
	_ "embed"

	"aranea-agents/internal/data/ent"
	"aranea-agents/pkg/loggateway"
)

//go:embed sql/hook_delivery.sql
var hookDeliveryDDL string

// EnsureHookDeliverySchema creates hook_deliveries table if missing and applies column patches.
func EnsureHookDeliverySchema(ctx context.Context, client *ent.Client, lg loggateway.Logger) error {
	if err := execDDLFile(ctx, client, hookDeliveryDDL, "hook_delivery"); err != nil {
		return err
	}
	return ensureHookDeliveryPatches(ctx, client, lg)
}
