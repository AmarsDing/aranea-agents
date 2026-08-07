package data

import (
	"context"
	"encoding/json"
	"fmt"

	"aranea-agents/internal/data/ent"
	"aranea-agents/pkg/loggateway"
)

// RunTeamDeliverableChannelRepairMigration heals teams whose persisted
// definition_json lost enable_state_deliverable.
//
// 2026-08-07 根因：Team × Graph 保存钩子 materializeAndBind 经
// OrchestrationSpecToDefinitionJSON 重序列化 definition_json，而
// OrchestrationSpec 此前缺少 enable_state_deliverable /
// deliverable_contract / verification_gates 字段，导致 Spirit 装配写入的
// 交付物通道配置在落库时被静默丢弃。后果链：运行期 ParseDefinition 读到
// false → 成员不注入 set_deliverable 工具 → 真实交付物闸门
// HasRealDeliverable 判失败 → DAG 下游节点永不派发。
//
// 结构修复（OrchestrationSpec 补字段）阻止新增丢字段；本迁移修复存量行。
// 修复口径与装配规则一致：dag_node_id 非空（DAG 团队）或成员数 > 1 的团队，
// 且 definition_json 未显式携带 enable_state_deliverable 时补 true。
// 显式 false 视为有意设置，尊重不动。损坏 JSON 跳过不阻断。
func RunTeamDeliverableChannelRepairMigration(ctx context.Context, client *ent.Client, d Dialect, lg loggateway.Logger) error {
	if client == nil {
		return fmt.Errorf("team deliverable channel repair migration: ent client required")
	}
	applied, err := isMigrationApplied(ctx, client, MigrationTeamDeliverableChannelRepair, lg)
	if err != nil {
		return fmt.Errorf("team deliverable channel repair migration: check gate: %w", err)
	}
	if applied {
		return nil
	}

	hasTable, err := tableExistsWithDialect(ctx, client, lg, "teams", d)
	if err != nil {
		return fmt.Errorf("team deliverable channel repair migration: check table: %w", err)
	}
	healed := 0
	if hasTable {
		rows, err := client.QueryContext(ctx,
			`SELECT id, definition_json, COALESCE(dag_node_id, '') FROM teams WHERE deleted_at = ''`)
		if err != nil {
			return fmt.Errorf("team deliverable channel repair migration: query: %w", err)
		}
		type teamRow struct {
			id      string
			defJSON string
			dagNode string
		}
		var candidates []teamRow
		for rows.Next() {
			var r teamRow
			if err := rows.Scan(&r.id, &r.defJSON, &r.dagNode); err != nil {
				rows.Close()
				return fmt.Errorf("team deliverable channel repair migration: scan: %w", err)
			}
			candidates = append(candidates, r)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return fmt.Errorf("team deliverable channel repair migration: rows: %w", err)
		}
		rows.Close()

		for _, r := range candidates {
			var body map[string]any
			if err := json.Unmarshal([]byte(r.defJSON), &body); err != nil {
				lg.Warn("team definition_json 不可解析，跳过修复",
					loggateway.StepID("migration.team_deliverable_channel_repair"),
					loggateway.Str("team_id", r.id))
				continue
			}
			if _, exists := body["enable_state_deliverable"]; exists {
				continue // 显式值（true/false）尊重不动
			}
			members, _ := body["members"].([]any)
			if r.dagNode == "" && len(members) <= 1 {
				continue // 非 DAG 且单成员：装配规则本就不开启通道
			}
			body["enable_state_deliverable"] = true
			out, err := json.Marshal(body)
			if err != nil {
				return fmt.Errorf("team deliverable channel repair migration: marshal team %s: %w", r.id, err)
			}
			if _, err := client.ExecContext(ctx,
				`UPDATE teams SET definition_json = $1 WHERE id = $2`, string(out), r.id); err != nil {
				return fmt.Errorf("team deliverable channel repair migration: update team %s: %w", r.id, err)
			}
			healed++
		}
		if healed > 0 {
			lg.Info("teams healed with enable_state_deliverable=true",
				loggateway.StepID("migration.team_deliverable_channel_repair"),
				loggateway.Int("rows", healed))
		}
	}

	if err := recordMigrationApplied(ctx, client, d, MigrationTeamDeliverableChannelRepair, migrationNameTeamDeliverableChannelRepair, lg); err != nil {
		return fmt.Errorf("team deliverable channel repair migration: record: %w", err)
	}
	return nil
}
