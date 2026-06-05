# taxonomy-rename Specification

## Purpose
TBD - created by archiving change phase1-taxonomy-review-fixes. Update Purpose after archive.
## Requirements
### Requirement: selfmedia variant 命名修复
selfmedia/agents.yaml 中 4 个使用连字符的 variant SHALL 改为下划线，匹配设计规范 `^[a-z0-9_]+$`。

#### Scenario: data-driven → data_driven
- **WHEN** 查看 `internal/scenario/selfmedia/agents.yaml`
- **THEN** variant `data-driven` SHALL 变更为 `data_driven`，对应 agent_key `webnovel-author-data-driven` SHALL 变更为 `webnovel-author-data_driven`

#### Scenario: geography-history → geography_history
- **WHEN** 查看 `internal/scenario/selfmedia/agents.yaml`
- **THEN** variant `geography-history` SHALL 变更为 `geography_history`，对应 agent_key `worldbuilding-designer-geography-history` SHALL 变更为 `worldbuilding-designer-geography_history`

#### Scenario: magic-system → magic_system
- **WHEN** 查看 `internal/scenario/selfmedia/agents.yaml`
- **THEN** variant `magic-system` SHALL 变更为 `magic_system`，对应 agent_key `worldbuilding-designer-magic-system` SHALL 变更为 `worldbuilding-designer-magic_system`

#### Scenario: platform-adapt → platform_adapt
- **WHEN** 查看 `internal/scenario/selfmedia/agents.yaml`
- **THEN** variant `platform-adapt` SHALL 变更为 `platform_adapt`，对应 agent_key `video-scriptwriter-platform-adapt` SHALL 变更为 `video-scriptwriter-platform_adapt`

#### Scenario: prompt 文件路径同步
- **WHEN** 查看 `internal/scenario/selfmedia/prompts/positions/` 目录
- **THEN** 含连字符的 prompt 子目录/文件名 SHALL 同步重命名为下划线格式

### Requirement: agent.proto 字段重命名
`api/kratos/agent/v1/agent.proto` 中 `category_position_id` 字段 SHALL 重命名为 `taxonomy_position_id`。

#### Scenario: Agent message 字段重命名
- **WHEN** 查看 `api/kratos/agent/v1/agent.proto`
- **THEN** `string category_position_id = 11;` SHALL 变更为 `string taxonomy_position_id = 11;`

#### Scenario: CreateAgentRequest 字段重命名
- **WHEN** 查看 `api/kratos/agent/v1/agent.proto`
- **THEN** `string category_position_id = 7;` SHALL 变更为 `string taxonomy_position_id = 7;`

#### Scenario: Proto 代码重新生成
- **WHEN** 运行 `make api`
- **THEN** 生成代码 SHALL 使用 `TaxonomyPositionId`（Go）/ `taxonomy_position_id`（JSON），旧 `CategoryPositionId` SHALL 不存在

#### Scenario: 前端 wireNormalize 映射更新
- **WHEN** 查看 `web/src/features/agents/wireNormalize.ts`
- **THEN** `categoryPositionId` 兼容映射 SHALL 更新为 `taxonomyPositionId`，`pickStr(w, 'categoryPositionId', 'category_position_id')` SHALL 变更为 `pickStr(w, 'taxonomyPositionId', 'taxonomy_position_id')`

### Requirement: 前端变量名统一为 taxonomy*
前端所有 `category*` 变量/函数名 SHALL 重命名为 `taxonomy*`。

#### Scenario: Store 变量重命名
- **WHEN** 查看 `web/src/stores/agents/index.ts` 和 `web/src/stores/platform/index.ts`
- **THEN** `categoryTree` SHALL 变更为 `taxonomyTree`，`loadCategoryTree` SHALL 变更为 `loadTaxonomyTree`，`selectedCategory` SHALL 变更为 `selectedTaxonomy`，`categoryPositionOptions` SHALL 变更为 `taxonomyPositionOptions`，`categoryLabel` SHALL 变更为 `taxonomyLabel`

#### Scenario: Composable/Component 变量重命名
- **WHEN** 查看所有使用 `categoryTree`/`loadCategoryTree`/`selectedCategory` 的前端文件
- **THEN** 所有引用 SHALL 统一更新为 `taxonomyTree`/`loadTaxonomyTree`/`selectedTaxonomy`

#### Scenario: 工具函数重命名
- **WHEN** 查看 `web/src/components/agents/agentUi.ts`
- **THEN** `flattenCategoryPositions` SHALL 变更为 `flattenTaxonomyPositions`

#### Scenario: 兼容别名清理
- **WHEN** 查看 `web/src/features/platform/taxonomyTreeUtils.ts`
- **THEN** `CategoryLevel`/`CategoryQTreeNode` legacy alias 导出 SHALL 被移除

### Requirement: 死代码文件清理
以下死代码文件 SHALL 被删除。

#### Scenario: agent_category 前端客户端删除
- **WHEN** 查看 `web/src/services/kratos/agent_category/` 目录
- **THEN** 该目录 SHALL 不存在

#### Scenario: CategoryTreeNodeHeader.vue 删除
- **WHEN** 查看 `web/src/components/agents/CategoryTreeNodeHeader.vue`
- **THEN** 该文件 SHALL 不存在

#### Scenario: categoryTreeUtils.ts 删除
- **WHEN** 查看 `web/src/features/platform/categoryTreeUtils.ts`
- **THEN** 该文件 SHALL 不存在

#### Scenario: categories.yaml 删除
- **WHEN** 查看 `internal/scenario/categories.yaml`
- **THEN** 该文件 SHALL 不存在

### Requirement: IndustryTaxonomyService gRPC 注册
IndustryTaxonomyService SHALL 在 gRPC server 中注册。

#### Scenario: gRPC 注册补充
- **WHEN** 查看 `internal/server/grpc.go`
- **THEN** SHALL 存在 `industrytaxonomyv1.RegisterIndustryTaxonomyServiceServer(srv, s.IndustryTaxonomy)` 调用

### Requirement: 前端残留验证
所有修改完成后 SHALL 通过全局 grep 验证无残留。

#### Scenario: 后端残留验证
- **WHEN** 在后端代码中 grep `AgentCategory`/`agent_category`/`category_position_id`
- **THEN** SHALL 返回 0 处匹配

#### Scenario: 前端残留验证
- **WHEN** 在前端代码中 grep `AgentCategory`/`agent-category`/`agent-categories`/`categoryTree`/`loadCategoryTree`/`selectedCategory`
- **THEN** SHALL 返回 0 处匹配

