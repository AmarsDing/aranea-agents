## Why

Phase1 Taxonomy Rename and Unify 已归档，但 aranea-review 深度复检发现 2 个阻断项（selfmedia variant 连字符、agent.proto 字段未重命名）和 13 个建议项（前端 68 处 category 变量名残留、3 个死代码文件未删除、softwaredev Agent 严重不足、taxonomy.yaml 缺丰富字段等）。这些问题影响数据正确性和命名一致性，需在进入后续 Phase 之前修复。

## What Changes

- 修复 selfmedia/agents.yaml 中 4 个 variant 的连字符为下划线（`data-driven` → `data_driven` 等），匹配 `variantSafeRe` 设计规范
- **BREAKING** 重命名 agent.proto 中 `category_position_id` → `taxonomy_position_id`，同步更新前端 wireNormalize 兼容映射
- 删除 3 个死代码文件：`web/src/services/kratos/agent_category/`、`CategoryTreeNodeHeader.vue`、`categoryTreeUtils.ts`
- 删除残留旧文件：`internal/scenario/categories.yaml`
- 前端 Store/Composable/变量名批量重命名 `category*` → `taxonomy*`（~68 处）
- 补全 IndustryTaxonomyService 的 gRPC 注册
- 补全 taxonomy.yaml 丰富字段（responsibilities/skills_required/seniority_level/variants）
- 对齐 selfmedia 部门 key（content_creation 拆分、growth_monetization → distribution）
- 补全 finance/agents.yaml 缺失的 variant 和 team
- 补全 softwaredev/agents.yaml P1-P3 Agent 定义

## Capabilities

### New Capabilities

（无新增能力）

### Modified Capabilities

- `taxonomy-rename`: 修复 variant 命名规范、proto 字段重命名、前端变量名统一、死代码清理
- `taxonomy-data-unify`: 补全 taxonomy.yaml 丰富字段、对齐部门 key、补全 finance/softwaredev Agent 定义

## Impact

- **Proto/API（BREAKING）**: `agent.proto` 字段 `category_position_id` → `taxonomy_position_id`，需 `make api` 重新生成，前端 wireNormalize 同步更新
- **后端 Biz/Data/Service**: selfmedia variant 命名修改影响种子数据和 agent key；IndustryTaxonomyService gRPC 注册补充
- **后端 YAML 配置**: taxonomy.yaml 格式升级、finance/softwaredev agents.yaml 补全
- **前端 Store/Composable/Component**: ~68 处变量名 `category*` → `taxonomy*`，3 个死代码文件删除
- **数据库**: variant 命名变更影响已有种子数据，需 re-seed
