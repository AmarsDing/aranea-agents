package data

import (
	"context"
	"os"
	"path/filepath"

	"aranea-agents/internal/biz/pack"
	"aranea-agents/internal/data/ent"
	"aranea-agents/pkg/loggateway"
)

// SeedPackTwinMonitor 导入 twinmonitor-pack（TwinMonitor 综合监控平台问答包）。
//
// 在 P1 阶段调用，位于 SeedPackItOps 之后。使用 ConflictOverwrite 冲突策略，
// kind 覆盖为 ecosystem_preset。twinmonitor-pack 无 taxonomy（单 Agent 问答助手，
// 不占组织树），仅注册 1 个 agent「twin_butler__general」。通过版本号门控确保
// 只导入一次。
func SeedPackTwinMonitor(ctx context.Context, client *ent.Client, d Dialect, scenarioDir string, lg loggateway.Logger) error {
	applied, err := isMigrationApplied(ctx, client, SeedPackTwinMonitorV1, lg)
	if err != nil {
		return entErrToBizErr(err, "SEED")
	}
	if applied {
		return nil
	}

	// 从 scenarioDir 读取 twinmonitor-pack
	// scenarioDir = "internal/scenario"，os.DirFS 需要以其父目录 "internal" 为根
	fsys := os.DirFS(filepath.Join(scenarioDir, ".."))
	p, readErr := pack.ReadPackFromFS(fsys, "scenario/packs/twinmonitor-pack")
	if readErr != nil {
		return entErrToBizErr(readErr, "SEED")
	}

	// 创建 Importer 并导入
	importer := newPackImporter(client, lg)
	result, importErr := importer.Import(ctx, p, pack.ConflictOverwrite, pack.WithKindOverride("ecosystem_preset"))
	if importErr != nil {
		return entErrToBizErr(importErr, "SEED")
	}

	lg.Info("twinmonitor-pack seed completed",
		loggateway.StepID("data.seed.pack_twinmonitor"),
		loggateway.Int("agents_created", result.AgentsCreated),
		loggateway.Int("agents_updated", result.AgentsUpdated),
		loggateway.Int("failures", len(result.Failures)))
	// 逐条打出失败详情：数量日志无法定位导入失败的实体与原因（TS9-BUG-3 排查教训）。
	for _, f := range result.Failures {
		lg.Warn("twinmonitor-pack import failure",
			loggateway.StepID("data.seed.pack_twinmonitor"),
			loggateway.Str("entity_type", f.EntityType),
			loggateway.Str("entity_key", f.Key),
			loggateway.Str("reason", f.Reason))
	}

	// 记录版本
	if recordErr := recordMigrationApplied(ctx, client, d, SeedPackTwinMonitorV1, "pack_twinmonitor_v1", lg); recordErr != nil {
		return entErrToBizErr(recordErr, "SEED")
	}

	return nil
}
