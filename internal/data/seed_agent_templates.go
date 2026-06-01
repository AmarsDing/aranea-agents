package data

import (
	"context"
	"fmt"
	"time"

	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/scenario/loader"
)

func SeedAgentTemplates(ctx context.Context, client *ent.Client, scenarioDir string) error {
	if client == nil {
		return nil
	}
	applied, err := isMigrationApplied(ctx, client, SeedAgentTemplatesV1)
	if err != nil {
		return fmt.Errorf("check seed agent templates v1: %w", err)
	}
	if applied {
		return nil
	}

	spec, err := loader.LoadAgentTemplatesSpec(scenarioDir)
	if err != nil {
		return fmt.Errorf("load agent templates spec: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)

	for _, t := range spec.Templates {
		id := "tpl_" + t.Key
		const q = `INSERT INTO agent_templates (id, template_key, label, icon, display_name, provider, model, description, sort_order, is_system, created_at, updated_at, deleted_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?, '')
			ON CONFLICT(template_key) DO UPDATE SET label=excluded.label, icon=excluded.icon, display_name=excluded.display_name, provider=excluded.provider, model=excluded.model, description=excluded.description, sort_order=excluded.sort_order, is_system=excluded.is_system, updated_at=excluded.updated_at`
		if _, err := client.ExecContext(ctx, q, id, t.Key, t.Label, t.Icon, t.DisplayName, t.Provider, t.Model, t.Description, t.SortOrder, now, now); err != nil {
			return fmt.Errorf("seed agent template %q: %w", t.Key, err)
		}
	}

	if err := recordMigrationApplied(ctx, client, SeedAgentTemplatesV1, "agent_templates_v1"); err != nil {
		return fmt.Errorf("record seed agent templates v1: %w", err)
	}
	return nil
}
