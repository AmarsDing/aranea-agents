package data

import (
	"context"

	"aranea-agents/internal/conf"
	"aranea-agents/internal/data/ent"
)

// ensureInitialAdminFromConfig inserts id=1 admin when the admins table is empty and
// data.initial_admin is set in YAML (password plaintext → MD5 hex, same as Login).
//
// When conf.Data has no initial_admin field in the generated protobuf, this is a no-op.
func ensureInitialAdminFromConfig(ctx context.Context, client *ent.Client, d *conf.Data) error {
	if client == nil || d == nil {
		return nil
	}
	// Regenerate internal/conf from proto when Data.initial_admin is added; then wire getters here.
	return nil
}
