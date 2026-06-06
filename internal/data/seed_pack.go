package data

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/pack"
	"aranea-agents/internal/data/ent"
	trpc "aranea-agents/internal/graph/trpc"
	"aranea-agents/internal/scenario/loader"
	"aranea-agents/pkg/loggateway"
)

// SeedPackBuiltinTemplates 使用 Pack 引擎加载内置模板（taxonomy + agent templates + graph templates）。
// 在 P1 阶段调用，使用 overwrite 冲突策略。
// force=true 时跳过版本门控，强制重新导入。
func SeedPackBuiltinTemplates(ctx context.Context, client *ent.Client, scenarioDir string, lg loggateway.Logger, force ...bool) error {
	// 版本门控：检查是否已应用（force=true 时跳过）
	skipVersionCheck := len(force) > 0 && force[0]
	if !skipVersionCheck {
		applied, err := isMigrationApplied(ctx, client, SeedPackBuiltinV1, lg)
		if err != nil {
			return fmt.Errorf("check seed pack builtin v1: %w", err)
		}
		if applied {
			return nil
		}
	}

	// 从 scenarioDir 读取 builtin-templates Pack
	// scenarioDir = "internal/scenario"，os.DirFS 需要以其父目录 "internal" 为根
	fsys := os.DirFS(filepath.Join(scenarioDir, ".."))
	p, readErr := pack.ReadPackFromFS(fsys, "scenario/packs/builtin-templates")
	if readErr != nil {
		return fmt.Errorf("read builtin-templates pack: %w", readErr)
	}

	// 创建 Importer 并导入
	importer := newPackImporter(client, lg)
	result, importErr := importer.Import(ctx, p, pack.ConflictOverwrite, pack.WithKindOverride("ecosystem_preset"))
	if importErr != nil {
		return fmt.Errorf("import builtin-templates pack: %w", importErr)
	}

	lg.Info("builtin-templates pack seed completed",
		loggateway.StepID("data.seed.pack_builtin"),
		loggateway.Int("agents_created", result.AgentsCreated),
		loggateway.Int("agents_updated", result.AgentsUpdated),
		loggateway.Int("graphs_created", result.GraphsCreated),
		loggateway.Int("taxonomy_nodes", result.TaxonomyNodes),
		loggateway.Int("failures", len(result.Failures)))

	// 同时写入 Graph 模板到 graph_definitions 表
	if graphErr := seedGraphTemplatesCompat(ctx, client, lg); graphErr != nil {
		lg.Warn("graph templates compat seed failed", loggateway.StepID("data.seed.pack_builtin"), loggateway.Err(graphErr))
	}

	// 记录版本
	if recordErr := recordMigrationApplied(ctx, client, SeedPackBuiltinV1, "pack_builtin_v1", lg); recordErr != nil {
		return fmt.Errorf("record pack builtin v1: %w", recordErr)
	}

	return nil
}

// SeedPackIndustry 使用 Pack 引擎加载行业数据。
// 在 API 触发时调用，使用 overwrite 冲突策略。
// Returns (agentsCreated, teamsCreated, error).
func SeedPackIndustry(ctx context.Context, client *ent.Client, scenarioDir, industryKey string, kindOverride string, lg loggateway.Logger) (int, int, error) {
	// 从现有 agents.yaml 加载并转换为 Pack 格式
	spec, loadErr := loader.LoadIndustrySpec(scenarioDir, industryKey)
	if loadErr != nil {
		return 0, 0, fmt.Errorf("load industry spec %s: %w", industryKey, loadErr)
	}

	p, convertErr := pack.ConvertIndustrySpecToPack(spec)
	if convertErr != nil {
		return 0, 0, fmt.Errorf("convert industry spec %s to pack: %w", industryKey, convertErr)
	}

	// 创建 Importer 并导入
	importer := newPackImporter(client, lg)
	result, importErr := importer.Import(ctx, p, pack.ConflictOverwrite, pack.WithKindOverride(kindOverride))
	if importErr != nil {
		return 0, 0, fmt.Errorf("import %s pack: %w", industryKey, importErr)
	}

	lg.Info(fmt.Sprintf("%s pack seed completed", industryKey),
		loggateway.StepID("data.seed.pack_industry"),
		loggateway.Str("industry", industryKey),
		loggateway.Int("agents_created", result.AgentsCreated),
		loggateway.Int("agents_updated", result.AgentsUpdated),
		loggateway.Int("agents_skipped", result.AgentsSkipped),
		loggateway.Int("teams_created", result.TeamsCreated),
		loggateway.Int("teams_updated", result.TeamsUpdated),
		loggateway.Int("failures", len(result.Failures)))

	return result.AgentsCreated, result.TeamsCreated, nil
}

// seedGraphTemplatesCompat 写入 graph_definitions 表。
func seedGraphTemplatesCompat(ctx context.Context, client *ent.Client, lg loggateway.Logger) error {
	templates := trpc.ListBuiltinTemplates()
	d := newDataFromClient(client, lg)
	graphRepo := NewGraphRepo(d)
	for _, tmpl := range templates {
		buildConfig := trpc.TemplateToBuildConfig(tmpl)
		def := &biz.GraphDefinition{
			Name:             tmpl.Name,
			Description:      tmpl.Description,
			EntryPoint:       buildConfig.EntryPoint,
			FinishPoint:      buildConfig.FinishPoint,
			EnableCheckpoint: false,
			Version:          1,
			SortOrder:        0,
		}
		def.Nodes = buildConfig.Nodes
		def.Edges = buildConfig.Edges
		def.ConditionalEdges = buildConfig.ConditionalEdges
		def.StateFields = buildConfig.StateFields

		if _, err := graphRepo.SaveDefinition(ctx, def); err != nil {
			lg.Warn("seed graph template failed", loggateway.StepID("data.seed.graph_template"), loggateway.Str("id", tmpl.ID), loggateway.Err(err))
			// 不中断，继续尝试下一个
		}
	}
	return nil
}

// newDataFromClient 创建一个最小化的 Data 实例，用于 seed 场景下创建 Repo。
// WARNING: 此实例缺少 rawDB/readDB/rwDB 字段，仅适用于使用 Ent API（d.RW()/d.Ent()）的 Repo。
// 任何需要原生 SQL（d.RWDB()/d.RawDB()）的操作都会 panic。
func newDataFromClient(client *ent.Client, lg loggateway.Logger) *Data {
	return &Data{
		entClient:  client,
		readClient: client,
		rw:         NewReadWriteClient(client, client),
		lg:         lg,
	}
}

// newPackImporter 创建一个使用 ent.Client 的 Pack 导入器。
func newPackImporter(client *ent.Client, lg loggateway.Logger) *pack.Importer {
	d := newDataFromClient(client, lg)
	adapter := NewPackRepoAdapter(
		NewAgentRepo(d),
		NewTeamRepo(d),
		NewTeamRepo(d),
		NewTaxonomyRepo(d),
		NewGraphRepo(d),
		NewSkillRepo(d),
	)
	return pack.NewImporter(adapter)
}
