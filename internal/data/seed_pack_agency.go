package data

import (
	"context"
	"os"
	"path/filepath"

	"aranea-agents/internal/biz/pack"
	"aranea-agents/internal/data/ent"
	"aranea-agents/pkg/loggateway"
)

// CleanupNonSystemData 清除非系统 agent/team/organization 数据，仅保留 4 个系统必需 agent
// （spirit/memory/skills/system_admin）。
//
// 清除范围：
//   - agent_prompt_files：非系统 agent 的提示词文件
//   - agent_runtime_settings：非系统 agent 的运行时配置
//   - agents：kind != 'system_builtin' 的 agent，以及 agent_variant = 'dept_lead' 的部门主管 agent
//   - teams：kind != 'system_builtin' 的团队
//   - organizations：全部（系统 agent 不依赖 organizations 表）
//
// 保留：4 个系统 agent 及其 prompt files/runtime settings、用户会话数据（sessions/turns/steps）、
// graph_definitions、cron_task、compiled_team 等。
//
// 此迁移通过版本号门控，只执行一次。如需重新清理，需手动删除 schema_migrations 中对应版本记录。
func CleanupNonSystemData(ctx context.Context, client *ent.Client, d Dialect, lg loggateway.Logger) error {
	if client == nil {
		return nil
	}

	// 版本门控：检查是否已应用
	applied, err := isMigrationApplied(ctx, client, SeedCleanupNonSystemV1, lg)
	if err != nil {
		return entErrToBizErr(err, "SEED")
	}
	if applied {
		return nil
	}

	// 保留的 system agent 子查询：kind='system_builtin' 且非 dept_lead
	// dept_lead agent 虽为 system_builtin，但绑定旧部门架构，需清除后由 SeedDeptLeadAgents 重建
	const keepSystemAgentsSQL = `SELECT id FROM agents WHERE kind = 'system_builtin' AND (agent_variant != 'dept_lead' OR agent_variant IS NULL)`

	// 1. 删除非系统 agent 的 prompt files
	if _, err := client.ExecContext(ctx, `DELETE FROM agent_prompt_files WHERE agent_id NOT IN (`+keepSystemAgentsSQL+`)`); err != nil {
		lg.Warn("cleanup: delete agent_prompt_files failed", loggateway.StepID("data.seed.cleanup"), loggateway.Err(err))
		return entErrToBizErr(err, "SEED")
	}

	// 2. 删除非系统 agent 的 runtime settings
	// 注意：agent_runtime_settings 表的主键列名为 agent_id（StorageKey）
	if _, err := client.ExecContext(ctx, `DELETE FROM agent_runtime_settings WHERE agent_id NOT IN (`+keepSystemAgentsSQL+`)`); err != nil {
		lg.Warn("cleanup: delete agent_runtime_settings failed", loggateway.StepID("data.seed.cleanup"), loggateway.Err(err))
		return entErrToBizErr(err, "SEED")
	}

	// 3. 删除非系统 agent（含 dept_lead agent 及遗留 copy agent）
	if _, err := client.ExecContext(ctx, `DELETE FROM agents WHERE kind != 'system_builtin' OR agent_variant = 'dept_lead' OR agent_key LIKE 'dept-lead-%'`); err != nil {
		lg.Warn("cleanup: delete agents failed", loggateway.StepID("data.seed.cleanup"), loggateway.Err(err))
		return entErrToBizErr(err, "SEED")
	}

	// 4. 删除非系统 team
	if _, err := client.ExecContext(ctx, `DELETE FROM teams WHERE kind != 'system_builtin'`); err != nil {
		lg.Warn("cleanup: delete teams failed", loggateway.StepID("data.seed.cleanup"), loggateway.Err(err))
		return entErrToBizErr(err, "SEED")
	}

	// 5. 删除所有 organizations（系统 agent 的 position_id=''，不依赖 organizations 表）
	if _, err := client.ExecContext(ctx, `DELETE FROM organizations`); err != nil {
		lg.Warn("cleanup: delete organizations failed", loggateway.StepID("data.seed.cleanup"), loggateway.Err(err))
		return entErrToBizErr(err, "SEED")
	}

	lg.Info("cleanup: non-system data cleared",
		loggateway.StepID("data.seed.cleanup"),
		loggateway.Str("note", "preserved 4 system agents (spirit/memory/skills/system_admin)"))

	// 记录版本
	if recordErr := recordMigrationApplied(ctx, client, d, SeedCleanupNonSystemV1, "cleanup_non_system_v1", lg); recordErr != nil {
		return entErrToBizErr(recordErr, "SEED")
	}
	return nil
}

// SeedPackAgency 导入 agency-pack（The Agency 230+ AI agent 模板库）。
//
// 在 P1 阶段调用，位于 CleanupNonSystemData 之后。使用 ConflictOverwrite 冲突策略，
// kind 覆盖为 ecosystem_preset。通过版本号门控确保只导入一次。
func SeedPackAgency(ctx context.Context, client *ent.Client, d Dialect, scenarioDir string, lg loggateway.Logger) error {
	applied, err := isMigrationApplied(ctx, client, SeedPackAgencyV1, lg)
	if err != nil {
		return entErrToBizErr(err, "SEED")
	}
	if applied {
		return nil
	}

	// 从 scenarioDir 读取 agency-pack
	// scenarioDir = "internal/scenario"，os.DirFS 需要以其父目录 "internal" 为根
	fsys := os.DirFS(filepath.Join(scenarioDir, ".."))
	p, readErr := pack.ReadPackFromFS(fsys, "scenario/packs/agency-pack")
	if readErr != nil {
		return entErrToBizErr(readErr, "SEED")
	}

	// 创建 Importer 并导入
	importer := newPackImporter(client, lg)
	result, importErr := importer.Import(ctx, p, pack.ConflictOverwrite, pack.WithKindOverride("ecosystem_preset"))
	if importErr != nil {
		return entErrToBizErr(importErr, "SEED")
	}

	lg.Info("agency-pack seed completed",
		loggateway.StepID("data.seed.pack_agency"),
		loggateway.Int("agents_created", result.AgentsCreated),
		loggateway.Int("agents_updated", result.AgentsUpdated),
		loggateway.Int("org_nodes", result.OrgNodes),
		loggateway.Int("failures", len(result.Failures)))

	// 记录版本
	if recordErr := recordMigrationApplied(ctx, client, d, SeedPackAgencyV1, "pack_agency_v1", lg); recordErr != nil {
		return entErrToBizErr(recordErr, "SEED")
	}

	return nil
}

// SeedPackItOps 导入 it-ops-pack（IT 运维行业岗位包）。
//
// 在 P1 阶段调用，位于 SeedPackAgency 之后。使用 ConflictOverwrite 冲突策略，
// kind 覆盖为 ecosystem_preset。组织节点按 level+key 增量合并，不影响 agency-pack
// 已有组织树。通过版本号门控确保只导入一次。
func SeedPackItOps(ctx context.Context, client *ent.Client, d Dialect, scenarioDir string, lg loggateway.Logger) error {
	applied, err := isMigrationApplied(ctx, client, SeedPackItOpsV1, lg)
	if err != nil {
		return entErrToBizErr(err, "SEED")
	}
	if applied {
		return nil
	}

	// 从 scenarioDir 读取 it-ops-pack
	// scenarioDir = "internal/scenario"，os.DirFS 需要以其父目录 "internal" 为根
	fsys := os.DirFS(filepath.Join(scenarioDir, ".."))
	p, readErr := pack.ReadPackFromFS(fsys, "scenario/packs/it-ops-pack")
	if readErr != nil {
		return entErrToBizErr(readErr, "SEED")
	}

	// 创建 Importer 并导入
	importer := newPackImporter(client, lg)
	result, importErr := importer.Import(ctx, p, pack.ConflictOverwrite, pack.WithKindOverride("ecosystem_preset"))
	if importErr != nil {
		return entErrToBizErr(importErr, "SEED")
	}

	lg.Info("it-ops-pack seed completed",
		loggateway.StepID("data.seed.pack_it_ops"),
		loggateway.Int("agents_created", result.AgentsCreated),
		loggateway.Int("agents_updated", result.AgentsUpdated),
		loggateway.Int("org_nodes", result.OrgNodes),
		loggateway.Int("failures", len(result.Failures)))
	// 逐条打出失败详情：数量日志无法定位导入失败的实体与原因（TS9-BUG-3 排查教训）。
	for _, f := range result.Failures {
		lg.Warn("it-ops-pack import failure",
			loggateway.StepID("data.seed.pack_it_ops"),
			loggateway.Str("entity_type", f.EntityType),
			loggateway.Str("entity_key", f.Key),
			loggateway.Str("reason", f.Reason))
	}

	// 记录版本
	if recordErr := recordMigrationApplied(ctx, client, d, SeedPackItOpsV1, "pack_it_ops_v1", lg); recordErr != nil {
		return entErrToBizErr(recordErr, "SEED")
	}

	return nil
}
