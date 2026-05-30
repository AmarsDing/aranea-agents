package data

import (
	"context"
	"fmt"
	"time"

	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/scenario/loader"
)

func SeedBuiltinAgentCategories(ctx context.Context, client *ent.Client, scenarioDir string) error {
	if client == nil {
		return nil
	}
	applied, err := isMigrationApplied(ctx, client, SeedCategoriesV2)
	if err != nil {
		return fmt.Errorf("check seed categories v2: %w", err)
	}
	if applied {
		return nil
	}

	spec, err := loader.LoadCategoriesSpec(scenarioDir)
	if err != nil {
		return fmt.Errorf("load categories spec: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)

	for _, ind := range spec.Industries {
		indID := "cat_industry_" + ind.Key
		indCategoryKey := ind.Key
		const q = `INSERT INTO agent_category_nodes (id, category_key, name, description, status, enabled, sort_order, parent_id, level, workspace_id, owner_user_id, is_system, config_json, metadata_json, created_at, updated_at, deleted_at)
			VALUES (?, ?, ?, ?, 'active', 1, ?, ?, 'industry', '', '', 1, '', '', ?, ?, '')
			ON CONFLICT(category_key) DO UPDATE SET name=excluded.name, description=excluded.description, sort_order=excluded.sort_order, parent_id=excluded.parent_id, level=excluded.level, is_system=excluded.is_system, updated_at=excluded.updated_at`
		if _, err := client.ExecContext(ctx, q, indID, indCategoryKey, ind.Name, ind.Description, ind.SortOrder, "", now, now); err != nil {
			return fmt.Errorf("seed industry category %q: %w", indCategoryKey, err)
		}

		for _, dept := range ind.Departments {
			deptID := "cat_dept_" + ind.Key + "_" + dept.Key
			deptCategoryKey := ind.Key + "/" + dept.Key
			const qDept = `INSERT INTO agent_category_nodes (id, category_key, name, description, status, enabled, sort_order, parent_id, level, workspace_id, owner_user_id, is_system, config_json, metadata_json, created_at, updated_at, deleted_at)
				VALUES (?, ?, ?, ?, 'active', 1, ?, ?, 'department', '', '', 1, '', '', ?, ?, '')
				ON CONFLICT(category_key) DO UPDATE SET name=excluded.name, description=excluded.description, sort_order=excluded.sort_order, parent_id=excluded.parent_id, level=excluded.level, is_system=excluded.is_system, updated_at=excluded.updated_at`
			if _, err := client.ExecContext(ctx, qDept, deptID, deptCategoryKey, dept.Name, dept.Description, dept.SortOrder, indID, now, now); err != nil {
				return fmt.Errorf("seed department category %q: %w", deptCategoryKey, err)
			}

			for _, pos := range dept.Positions {
				posID := "cat_pos_" + pos.Key
				posCategoryKey := ind.Key + "/" + dept.Key + "/" + pos.Key
				const qPos = `INSERT INTO agent_category_nodes (id, category_key, name, description, status, enabled, sort_order, parent_id, level, workspace_id, owner_user_id, is_system, config_json, metadata_json, created_at, updated_at, deleted_at)
					VALUES (?, ?, ?, ?, 'active', 1, ?, ?, 'position', '', '', 1, '', '', ?, ?, '')
					ON CONFLICT(category_key) DO UPDATE SET name=excluded.name, description=excluded.description, sort_order=excluded.sort_order, parent_id=excluded.parent_id, level=excluded.level, is_system=excluded.is_system, updated_at=excluded.updated_at`
				if _, err := client.ExecContext(ctx, qPos, posID, posCategoryKey, pos.Name, pos.Description, pos.SortOrder, deptID, now, now); err != nil {
					return fmt.Errorf("seed position category %q: %w", posCategoryKey, err)
				}
			}
		}
	}

	if err := recordMigrationApplied(ctx, client, SeedCategoriesV2, "categories_v2"); err != nil {
		return fmt.Errorf("record seed categories v2: %w", err)
	}
	return nil
}
