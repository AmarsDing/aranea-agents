//go:build ignore

package data

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/pack"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/graph/trpc"
	"aranea-agents/internal/scenario/loader"
	"aranea-agents/pkg/loggateway"
)

// SeedPackBuiltinTemplates 使用 Pack 引擎加载内置模板（taxonomy + agent templates + graph templates）。
// 在 P1 阶段调用，使用 overwrite 冲突策略。
func SeedPackBuiltinTemplates(ctx context.Context, client *ent.Client, scenarioDir string, lg loggateway.Logger) error {
	// 版本门控：检查是否已应用
	applied, err := isMigrationApplied(ctx, client, SeedPackBuiltinV1, lg)
	if err != nil {
		return fmt.Errorf("check seed pack builtin v1: %w", err)
	}
	if applied {
		return nil
	}

	// 从 scenarioDir 读取 builtin-templates Pack
	// scenarioDir = "internal/scenario"，os.DirFS 需要以其父目录 "internal" 为根
	fsys := os.DirFS(filepath.Join(scenarioDir, ".."))
	p, readErr := pack.ReadPackFromFS(fsys, "scenario/packs/builtin-templates")
	if readErr != nil {
		return fmt.Errorf("read builtin-templates pack: %w", readErr)
	}

	// 创建 Importer 并导入
	importer := newPackImporter(client)
	result, importErr := importer.Import(ctx, p, pack.ConflictOverwrite)
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

	// 同时写入 agent_templates 表（兼容现有前端）
	if tmplErr := seedAgentTemplatesCompat(ctx, client, scenarioDir, lg); tmplErr != nil {
		lg.Warn("agent templates compat seed failed", loggateway.StepID("data.seed.pack_builtin"), loggateway.Err(tmplErr))
	}

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
// 在 Lazy 阶段调用，使用 overwrite 冲突策略。
func SeedPackIndustry(ctx context.Context, client *ent.Client, scenarioDir, industryKey string, lg loggateway.Logger) error {
	// 版本门控
	versionKey := industryPackVersion(industryKey)
	applied, err := isMigrationApplied(ctx, client, versionKey, lg)
	if err != nil {
		return fmt.Errorf("check seed pack %s: %w", industryKey, err)
	}
	if applied {
		return nil
	}

	// 从现有 agents.yaml 加载并转换为 Pack 格式
	spec, loadErr := loader.LoadIndustrySpec(scenarioDir, industryKey)
	if loadErr != nil {
		return fmt.Errorf("load industry spec %s: %w", industryKey, loadErr)
	}

	p, convertErr := pack.ConvertIndustrySpecToPack(spec)
	if convertErr != nil {
		return fmt.Errorf("convert industry spec %s to pack: %w", industryKey, convertErr)
	}

	// 创建 Importer 并导入
	importer := newPackImporter(client)
	result, importErr := importer.Import(ctx, p, pack.ConflictOverwrite)
	if importErr != nil {
		return fmt.Errorf("import %s pack: %w", industryKey, importErr)
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

	// 记录版本
	if recordErr := recordMigrationApplied(ctx, client, versionKey, fmt.Sprintf("pack_%s_v1", industryKey), lg); recordErr != nil {
		return fmt.Errorf("record pack %s v1: %w", industryKey, recordErr)
	}

	return nil
}

// industryPackVersion 返回行业 Pack 的版本号常量。
func industryPackVersion(industryKey string) int {
	switch industryKey {
	case "finance":
		return SeedPackFinanceV1
	case "selfmedia":
		return SeedPackSelfmediaV1
	case "softwaredev":
		return SeedPackSoftwaredevV1
	default:
		return SeedPackIndustryBase + hashIndustryKey(industryKey)
	}
}

// hashIndustryKey 将行业 key 映射为版本号偏移量。
func hashIndustryKey(key string) int {
	h := 0
	for _, c := range key {
		h = h*31 + int(c)
	}
	if h < 0 {
		h = -h
	}
	return h % 1000
}

// seedAgentTemplatesCompat 写入 agent_templates 表（兼容现有前端）。
func seedAgentTemplatesCompat(ctx context.Context, client *ent.Client, scenarioDir string, lg loggateway.Logger) error {
	return SeedAgentTemplates(ctx, client, scenarioDir, lg)
}

// seedGraphTemplatesCompat 写入 graph_definitions 表。
func seedGraphTemplatesCompat(ctx context.Context, client *ent.Client, lg loggateway.Logger) error {
	templates := trpc.ListBuiltinTemplates()
	graphRepo := NewGraphRepo(&Data{entClient: client, readClient: client, lg: lg}, lg)
	if graphRepo == nil {
		return nil
	}
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
		def.Nodes = make([]biz.NodeDef, len(buildConfig.Nodes))
		for i, n := range buildConfig.Nodes {
			def.Nodes[i] = n.NodeDef
		}
		def.Edges = buildConfig.Edges
		def.ConditionalEdges = make([]biz.ConditionalEdgeDef, len(buildConfig.ConditionalEdges))
		for i, ce := range buildConfig.ConditionalEdges {
			def.ConditionalEdges[i] = ce.ConditionalEdgeDef
		}
		def.StateFields = buildConfig.StateFields

		if _, err := graphRepo.SaveGraphDefinition(ctx, def); err != nil {
			lg.Warn("seed graph template failed", loggateway.StepID("data.seed.graph_template"), loggateway.Str("id", tmpl.ID), loggateway.Err(err))
			// 不中断，继续尝试下一个
		}
	}
	return nil
}

// packImporterRepo 是 ImporterRepo 的适配器，通过 ent.Client 实现所有接口方法。
type packImporterRepo struct {
	client *ent.Client
	data   *Data
	lg     loggateway.Logger
}

// newPackImporter 创建一个使用 ent.Client 的 Pack 导入器。
func newPackImporter(client *ent.Client) *pack.Importer {
	// 创建一个临时 Data 实例用于 Repo 创建
	d := &Data{entClient: client, readClient: client, lg: loggateway.Nop()}
	repo := &packImporterRepo{
		client: client,
		data:   d,
		lg:     loggateway.Nop(),
	}
	return pack.NewImporter(repo)
}

// 以下方法实现 pack.ImporterRepo 接口。
// 由于 Pack 导入引擎需要 biz 层的 Repo 接口，
// 这里通过 data 层的 Repo 实现来桥接。

// 注意：实际的 Repo 方法委托给 data 层已有的 Repo 实现。
// 由于 pack.ImporterRepo 接口与 data 层 Repo 接口不完全一致，
// 我们需要一个适配层。这里简化处理，在 seed_pack_adapter.go 中实现。
