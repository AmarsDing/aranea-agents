package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	_ "github.com/lib/pq"
)

func main() {
	dsn := "postgres://postgres:Hangshan%40123@127.0.0.1:5432/aranea?sslmode=disable"
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	out, err := os.Create("seed_verify_result.txt")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create file: %v\n", err)
		os.Exit(1)
	}
	defer out.Close()

	queries := []string{
		`SELECT 'organizations_by_level', level, COUNT(*)::text FROM organizations GROUP BY level ORDER BY level`,
		`SELECT 'agents_by_kind', kind, COUNT(*)::text FROM agents GROUP BY kind ORDER BY kind`,
		`SELECT 'agents_by_variant', COALESCE(agent_variant,'(null)'), COUNT(*)::text FROM agents GROUP BY agent_variant ORDER BY 1`,
		`SELECT 'agent_prompt_files_total', 'all', COUNT(*)::text FROM agent_prompt_files`,
		`SELECT 'teams_by_kind', kind, COUNT(*)::text FROM teams GROUP BY kind ORDER BY kind`,
		`SELECT 'schema_migrations_seed', version::text, name FROM schema_migrations WHERE version IN (20261101,20261102,20261103,20261104) ORDER BY version`,
		`SELECT 'org_companies', org_key, name FROM organizations WHERE level='company' ORDER BY org_key`,
		`SELECT 'org_departments', org_key, name FROM organizations WHERE level='department' ORDER BY org_key`,
		`SELECT 'system_agents', agent_key, COALESCE(agent_variant,'(null)') FROM agents WHERE kind='system_builtin' ORDER BY agent_key`,
		`SELECT 'agents_no_position', agent_key, position_id FROM agents WHERE position_id='' AND kind='ecosystem_preset' LIMIT 10`,
		`SELECT 'prompt_files_per_agent_top', agent_id, COUNT(*)::text FROM agent_prompt_files GROUP BY agent_id ORDER BY COUNT(*) DESC LIMIT 5`,
		`SELECT 'cross_border_agents', agent_key, position_id FROM agents WHERE agent_key LIKE '%carousel%' OR agent_key LIKE '%china_ecommerce%' OR agent_key LIKE '%cross_border%' OR agent_key LIKE '%livestream%' ORDER BY agent_key`,
		`SELECT 'cross_border_dept', id, org_key, parent_id FROM organizations WHERE org_key='cross_border_ecommerce' OR org_key LIKE '%cross_border%'`,
		`SELECT 'orphan_dept_lead', agent_key, agent_variant FROM agents WHERE agent_key LIKE '%dept-lead-risk%' OR agent_key LIKE '%copy%'`,
		`SELECT 'cross_border_positions', id, org_key, parent_id, level FROM organizations WHERE org_key IN ('carousel_growth_engine','china_ecommerce_operator','cross_border_specialist','livestream_commerce_coach') ORDER BY org_key`,
		`SELECT 'orphan_positions', COUNT(*)::text, 'positions_without_dept_parent' FROM organizations p WHERE p.level='position' AND p.parent_id NOT IN (SELECT id FROM organizations WHERE level='department')`,
		`SELECT 'all_dept_count_check', COUNT(*)::text, 'departments_total' FROM organizations WHERE level='department'`,
	}
	for _, q := range queries {
		rows, err := db.QueryContext(ctx, q)
		if err != nil {
			fmt.Fprintf(out, "QUERY FAILED: %s\n  err: %v\n", q, err)
			continue
		}
		fmt.Fprintf(out, "---- %s ----\n", q)
		for rows.Next() {
			var c1, c2, c3 string
			if err := rows.Scan(&c1, &c2, &c3); err != nil {
				fmt.Fprintf(out, "  scan err: %v\n", err)
				break
			}
			fmt.Fprintf(out, "  %s | %s | %s\n", c1, c2, c3)
		}
		rows.Close()
	}
	fmt.Fprintln(out, "=== DONE ===")
}
