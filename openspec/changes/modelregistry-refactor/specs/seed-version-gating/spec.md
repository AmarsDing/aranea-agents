# 种子数据版本门控 + 分类体系统一 设计文档

> 日期: 2026-05-30
> 状态: 已批准

## 一、目标

1. **版本门控**: 配置文件(YAML)驱动的种子数据，仅在版本号变更时录入数据库，日常启动零开销
2. **分类统一**: 废弃 industries/departments/positions 三表，统一为 agent_category_nodes 单表
3. **Agent 模版 YAML 化**: 7 个硬编码模版移入 YAML 配置，录入数据库
4. **Team 行业归属**: Team 模型增加 category_industry_id 字段，显式存储

## 二、版本门控机制

复用 `schema_migrations` 表，为每类种子分配版本号常量:

```go
const (
    SeedCategoriesV2     = 20260530
    SeedAgentTemplatesV1 = 20260531
    SeedIndustryAgentsV1 = 20260601
)
```

执行流程:
- `isMigrationApplied(version)` → 已录入则跳过
- 未录入 → 从 YAML 加载 → ON CONFLICT DO UPDATE 写入 → `recordMigrationApplied(version, name)`
- 配置变更时递增版本号常量

## 三、分类体系统一

### 删除
- `industries`/`departments`/`positions` 三张表及 Ent Schema
- `IndustryUsecase`/`DepartmentUsecase`/`PositionUsecase` 三个 Usecase
- `industryRepo`/`departmentRepo`/`positionRepo` 三个 Repo
- `SeedBuiltinIndustries` 种子函数

### 保留
- `agent_category_nodes` 单表 (已有 level/parent_id 字段)
- `AgentCategoryUsecase` (扩展方法)
- `SeedBuiltinAgentCategories` (改为从 YAML 加载)

### 新增
- `agent_category_nodes` 增加 `scenario_key` 字段
- `CategoryAncestors` 类型替代 `PositionAncestors`
- `AgentCategoryUsecase` 增加 `ListByLevel`/`GetAncestors`/`GetPositionPrompt`/`ListPositionVariants` 方法

### IndustryService 改造
- 6 个 RPC 保持 HTTP 路由不变(前端兼容)
- 内部改为查询 AgentCategoryUsecase

## 四、Agent 模版 YAML 化

- 新建 `internal/scenario/agent_templates.yaml`
- 新建 `agent_templates` 数据库表
- `ListAgentTemplates()` 从数据库读取
- 前端删除 `descriptionTemplates` 本地 fallback

## 五、Team 行业归属

- Team 模型增加 `category_industry_id` 字段
- 创建/编辑时从成员 Agent 推导或用户显式选择
- 前端 `groupTeamsByIndustry` 直接读取字段

## 六、YAML 配置文件结构

```
internal/scenario/
├── categories.yaml          ← 行业/部门/岗位层级定义
├── agent_templates.yaml     ← Agent 预设模版
├── finance/agents.yaml      ← 金融行业 Agent/Team
├── selfmedia/agents.yaml    ← 自媒体行业 Agent/Team
└── softwaredev/agents.yaml  ← 软件开发行业 Agent/Team
```

## 七、影响面

- 后端: ~29 个文件
- 前端: ~14 个文件
- Proto: industry.proto 保持 HTTP 路由兼容

## 八、实施顺序

1. 版本门控种子机制 + categories.yaml
2. 分类体系统一(废弃三表，IndustryService 改造)
3. Agent 模版 YAML 化 + Team 行业归属字段
4. 前端适配
5. aranea-review 审查
