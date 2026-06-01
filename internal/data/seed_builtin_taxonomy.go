package data

import (
	"context"
	"fmt"
	"time"

	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/scenario/loader"
	"aranea-agents/pkg/loggateway"
)

func SeedBuiltinTaxonomy(ctx context.Context, client *ent.Client, scenarioDir string, lg loggateway.Logger) error {
	if client == nil {
		return nil
	}
	applied, err := isMigrationApplied(ctx, client, SeedTaxonomyV1, lg)
	if err != nil {
		return fmt.Errorf("check seed taxonomy v1: %w", err)
	}
	if applied {
		return nil
	}

	spec, err := loader.LoadTaxonomySpec(scenarioDir)
	if err != nil {
		return fmt.Errorf("load taxonomy spec: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)

	for _, ind := range spec.Industries {
		indID := "tax_industry_" + ind.Key
		indTaxonomyKey := ind.Key
		const q = `INSERT INTO industry_taxonomy (id, taxonomy_key, name, description, status, enabled, sort_order, parent_id, level, workspace_id, owner_user_id, is_system, config_json, metadata_json, created_at, updated_at, deleted_at)
			VALUES (?, ?, ?, ?, 'active', 1, ?, ?, 'industry', '', '', 1, '', '', ?, ?, '')
			ON CONFLICT(taxonomy_key) DO UPDATE SET name=excluded.name, description=excluded.description, sort_order=excluded.sort_order, parent_id=excluded.parent_id, level=excluded.level, is_system=excluded.is_system, updated_at=excluded.updated_at`
		if _, err := client.ExecContext(ctx, q, indID, indTaxonomyKey, ind.Name, ind.Description, ind.SortOrder, "", now, now); err != nil {
			lg.Warn("seed step failed", loggateway.StepID("data.seed.taxonomy.industry"), loggateway.Str("key", indTaxonomyKey), loggateway.Err(err))
			return fmt.Errorf("seed industry node %q: %w", indTaxonomyKey, err)
		}

		for _, dept := range ind.Departments {
			deptID := "tax_dept_" + ind.Key + "_" + dept.Key
			deptTaxonomyKey := ind.Key + "/" + dept.Key
			const qDept = `INSERT INTO industry_taxonomy (id, taxonomy_key, name, description, status, enabled, sort_order, parent_id, level, workspace_id, owner_user_id, is_system, config_json, metadata_json, created_at, updated_at, deleted_at)
				VALUES (?, ?, ?, ?, 'active', 1, ?, ?, 'department', '', '', 1, '', '', ?, ?, '')
				ON CONFLICT(taxonomy_key) DO UPDATE SET name=excluded.name, description=excluded.description, sort_order=excluded.sort_order, parent_id=excluded.parent_id, level=excluded.level, is_system=excluded.is_system, updated_at=excluded.updated_at`
			if _, err := client.ExecContext(ctx, qDept, deptID, deptTaxonomyKey, dept.Name, dept.Description, dept.SortOrder, indID, now, now); err != nil {
				lg.Warn("seed step failed", loggateway.StepID("data.seed.taxonomy.department"), loggateway.Str("key", deptTaxonomyKey), loggateway.Err(err))
				return fmt.Errorf("seed department node %q: %w", deptTaxonomyKey, err)
			}

			for _, pos := range dept.Positions {
				posID := "tax_pos_" + pos.Key
				posTaxonomyKey := ind.Key + "/" + dept.Key + "/" + pos.Key
				const qPos = `INSERT INTO industry_taxonomy (id, taxonomy_key, name, description, status, enabled, sort_order, parent_id, level, workspace_id, owner_user_id, is_system, config_json, metadata_json, created_at, updated_at, deleted_at)
					VALUES (?, ?, ?, ?, 'active', 1, ?, ?, 'position', '', '', 1, '', '', ?, ?, '')
					ON CONFLICT(taxonomy_key) DO UPDATE SET name=excluded.name, description=excluded.description, sort_order=excluded.sort_order, parent_id=excluded.parent_id, level=excluded.level, is_system=excluded.is_system, updated_at=excluded.updated_at`
				if _, err := client.ExecContext(ctx, qPos, posID, posTaxonomyKey, pos.Name, pos.Description, pos.SortOrder, deptID, now, now); err != nil {
					lg.Warn("seed step failed", loggateway.StepID("data.seed.taxonomy.position"), loggateway.Str("key", posTaxonomyKey), loggateway.Err(err))
					return fmt.Errorf("seed position node %q: %w", posTaxonomyKey, err)
				}
			}
		}
	}

	if err := recordMigrationApplied(ctx, client, SeedTaxonomyV1, "taxonomy_v1", lg); err != nil {
		lg.Warn("seed step failed", loggateway.StepID("data.seed.taxonomy.record"), loggateway.Err(err))
		return fmt.Errorf("record seed taxonomy v1: %w", err)
	}
	return nil
}
