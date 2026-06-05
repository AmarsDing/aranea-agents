# 种子数据版本门控 + 分类体系统一 设计文档

> 日期: 2026-05-30
> 状态: 部分实现（版本门控 + Agent 模版 + Team 行业归属已完成；分类体系统一采用 industry_taxonomy 方案替代）

## 一、目标

1. **版本门控**: 配置文件(YAML)驱动的种子数据，仅在版本号变更时录入数据库，日常启动零开销 ✅
2. **分类统一**: 废弃 industries/departments/positions 三表，统一为单表 ✅（实际使用 `industry_taxonomy` 表替代原设计的 `agent_category_nodes`）
3. **Agent 模版 YAML 化**: 7 个硬编码模版移入 YAML 配置，录入数据库 ✅
4. **Team 行业归属**: Team 模型增加 category_industry_id 字段，显式存储 ✅

## 二、版本门控机制

复用 `schema_migrations` 表，为每类种子分配版本号常量:

```go
const (
    SeedCategoriesV2     = 20260530
    SeedTaxonomyV1       = 20260529
    SeedAgentTemplatesV1 = 20260801
    SeedIndustryAgentsV1 = 20260804
)
```

> **实现偏差**：相比原始设计，新增了 `SeedTaxonomyV1 = 20260529` 常量用于分类体系种子。`SeedAgentTemplatesV1` 和 `SeedIndustryAgentsV1` 的版本号也不同于原设计。

执行流程:
- `isMigrationApplied(version)` → 已录入则跳过
- 未录入 → 从 YAML 加载 → ON CONFLICT DO UPDATE 写入 → `recordMigrationApplied(version, name)`
- 配置变更时递增版本号常量

## 三、分类体系统一

> **实现偏差说明**：原始设计使用 `agent_category_nodes` 单表，实际实现使用 `industry_taxonomy` 单表。两者语义等价，但表名和字段名不同。

### 删除 ✅
- `industries`/`departments`/`positions` 三张表及 Ent Schema — 已删除
- `IndustryUsecase`/`DepartmentUsecase`/`PositionUsecase` 三个 Usecase — 已删除
- `industryRepo`/`departmentRepo`/`positionRepo` 三个 Repo — 已删除
- `SeedBuiltinIndustries` 种子函数 — 已删除

### 替代方案（实际实现）
- `industry_taxonomy` 单表（替代原设计的 `agent_category_nodes`）
  - 字段：`id`, `taxonomy_key`, `name`, `description`, `status`, `enabled`, `sort_order`, `parent_id`, `level`, `scenario_key`, `workspace_id`, `owner_user_id`, `is_system`, `config_json`, `metadata_json`, `created_at`, `updated_at`, `deleted_at`
  - `level` 字段值：`industry` / `department` / `position`
  - `scenario_key` 字段：对应原设计的 `scenario_key`
  - 索引：`idx_taxonomy_parent(parent_id, sort_order)`, `idx_taxonomy_level(level, sort_order)`
- `IndustryTaxonomyService`（替代原设计的 `AgentCategoryUsecase`）
- `SeedBuiltinTaxonomy`（替代原设计的 `SeedBuiltinAgentCategories`，从 `taxonomy.yaml` 加载）

### IndustryService 改造
- 使用 `IndustryTaxonomyService` 查询 `industry_taxonomy` 表
- HTTP 路由保持兼容

## 四、Agent 模版 YAML 化 ✅

- `internal/scenario/agent_templates.yaml` — 已创建
- `agent_templates` 数据库表 — 已创建（Ent Schema: `internal/data/ent/schema/agent_template.go`）
- `SeedAgentTemplates()` 从 YAML 加载并写入数据库 — 已实现
- `internal/scenario/loader/agent_templates_loader.go` — YAML 加载器

## 五、Team 行业归属 ✅

- Team 模型增加 `category_industry_id` 字段 — 已实现（`internal/data/ent/schema/team.go`）
- 创建/编辑时从成员 Agent 推导或用户显式选择
- 前端 `groupTeamsByIndustry` 直接读取字段

## 六、YAML 配置文件结构

```
internal/scenario/
├── categories.yaml          ← 行业/部门/岗位层级定义
├── taxonomy.yaml            ← 分类体系定义（用于 SeedBuiltinTaxonomy）
├── agent_templates.yaml     ← Agent 预设模版
├── finance/agents.yaml      ← 金融行业 Agent/Team
├── selfmedia/agents.yaml    ← 自媒体行业 Agent/Team
├── softwaredev/agents.yaml  ← 软件开发行业 Agent/Team
└── packs/
    ├── builtin-templates/   ← 内置模版 Pack（含 agent + graph + taxonomy）
    └── finance/             ← 金融行业 Pack
```

> **实现偏差**：新增了 `taxonomy.yaml` 和 `packs/` 目录结构。Pack 系统由 `aranea-pack-import-export` change 引入。

## 七、影响面

- 后端: ~29 个文件
- 前端: ~14 个文件
- Proto: industry.proto 保持 HTTP 路由兼容

## 八、实施顺序

1. ✅ 版本门控种子机制 + categories.yaml + taxonomy.yaml
2. ✅ 分类体系统一（使用 industry_taxonomy 表替代 agent_category_nodes）
3. ✅ Agent 模版 YAML 化 + Team 行业归属字段
4. ⬜ 前端适配
5. ⬜ aranea-review 审查
