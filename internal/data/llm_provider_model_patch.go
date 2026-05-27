package data

import (
	"context"

	"aranea-agents/internal/data/ent"
)

// ensureLlmProviderModelCapabilityPatches adds capability columns for existing
// SQLite installs created before the catalog carried first-class capabilities.
func ensureLlmProviderModelCapabilityPatches(ctx context.Context, c *ent.Client) error {
	if c == nil {
		return nil
	}
	patches := []struct {
		col string
		ddl string
	}{
		{"capability_text", `ALTER TABLE llm_provider_models ADD COLUMN capability_text INTEGER NOT NULL DEFAULT 0`},
		{"capability_vision", `ALTER TABLE llm_provider_models ADD COLUMN capability_vision INTEGER NOT NULL DEFAULT 0`},
		{"capability_audio", `ALTER TABLE llm_provider_models ADD COLUMN capability_audio INTEGER NOT NULL DEFAULT 0`},
		{"capability_file", `ALTER TABLE llm_provider_models ADD COLUMN capability_file INTEGER NOT NULL DEFAULT 0`},
		{"capability_tool_call", `ALTER TABLE llm_provider_models ADD COLUMN capability_tool_call INTEGER NOT NULL DEFAULT 0`},
		{"capability_cache", `ALTER TABLE llm_provider_models ADD COLUMN capability_cache INTEGER NOT NULL DEFAULT 0`},
		{"capability_thinking", `ALTER TABLE llm_provider_models ADD COLUMN capability_thinking INTEGER NOT NULL DEFAULT 0`},
		{"capability_text_only", `ALTER TABLE llm_provider_models ADD COLUMN capability_text_only INTEGER NOT NULL DEFAULT 0`},
		{"capabilities_explicit", `ALTER TABLE llm_provider_models ADD COLUMN capabilities_explicit INTEGER NOT NULL DEFAULT 0`},
	}
	for _, p := range patches {
		has, err := sqliteColumnExists(ctx, c, "llm_provider_models", p.col)
		if err != nil {
			return err
		}
		if has {
			continue
		}
		if _, err := c.ExecContext(ctx, p.ddl); err != nil {
			return err
		}
	}
	return nil
}
