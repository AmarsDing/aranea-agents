## MODIFIED Requirements

### Requirement: Ent Schema 重命名 — AgentCategory → IndustryTaxonomy
Ent Schema 中 `AgentCategory` struct SHALL 重命名为 `IndustryTaxonomy`，物理表名 SHALL 从 `agent_category_nodes` 变更为 `industry_taxonomy`。`agent.go` schema 中的 `category_position_id` 字段 SHALL 重命名为 `taxonomy_position_id`。

#### Scenario: Ent Schema struct 重命名
- **WHEN** 运行 `go generate ./internal/data/ent`
- **THEN** 生成的 Ent 代码 SHALL 使用 `IndustryTaxonomy` struct 名，`Table()` 方法 SHALL 返回 `"industry_taxonomy"`，`Annotations()` 中 `entsql.Annotation.Table` SHALL 为 `"industry_taxonomy"`

#### Scenario: Agent schema 字段重命名
- **WHEN** 查看 `internal/data/ent/schema/agent.go`
- **THEN** 字段 `category_position_id` SHALL 变更为 `taxonomy_position_id`，注释 SHALL 更新为 `FK to taxonomy positions`

#### Scenario: 生成代码验证
- **WHEN** Ent 代码重新生成完成
- **THEN** `internal/data/ent/` 下 SHALL 存在 `industrytaxonomy.go` / `industrytaxonomy_create.go` / `industrytaxonomy_query.go` 等文件，`agent.go` / `agent_create.go` 中 SHALL 使用 `TaxonomyPositionID`，旧的 `agentcategory.go` 等文件 SHALL 不存在

### Requirement: Biz 层重命名 — AgentCategory → TaxonomyNode
Biz 层所有 `AgentCategory` 相关类型和接口 SHALL 重命名为 `TaxonomyNode` 体系。文件 `agent_category.go` SHALL 重命名为 `taxonomy.go`。

#### Scenario: Biz struct 重命名
- **WHEN** 查看 `internal/biz/taxonomy.go`
- **THEN** 以下重命名 SHALL 生效：`AgentCategory` → `TaxonomyNode`，`AgentCategoryTreeNode` → `TaxonomyTreeNode`，`CategoryAncestors` → `TaxonomyAncestors`

#### Scenario: Biz 接口重命名
- **WHEN** 查看 `internal/biz/taxonomy.go`
- **THEN** 以下接口重命名 SHALL 生效：`AgentCategoryReader` → `TaxonomyReader`，`AgentCategoryWriter` → `TaxonomyWriter`，`AgentCategoryRepo` → `TaxonomyRepo`

#### Scenario: Biz Usecase 重命名
- **WHEN** 查看 `internal/biz/taxonomy.go`
- **THEN** `AgentCategoryUsecase` SHALL 重命名为 `TaxonomyUsecase`，`NewAgentCategoryUsecase` SHALL 重命名为 `NewTaxonomyUsecase`，`normalizeAgentCategory` SHALL 重命名为 `normalizeTaxonomy`，所有方法接收者 SHALL 从 `uc *AgentCategoryUsecase` 变更为 `uc *TaxonomyUsecase`

#### Scenario: Agent 类型字段重命名
- **WHEN** 查看 `internal/biz/agent_types.go`
- **THEN** `CategoryPositionID string` SHALL 变更为 `TaxonomyPositionID string`

#### Scenario: Agent Usecase 字段重命名
- **WHEN** 查看 `internal/biz/agent_usecase.go`
- **THEN** 所有 `CategoryPositionID` 引用 SHALL 变更为 `TaxonomyPositionID`

#### Scenario: Biz ProviderSet 更新
- **WHEN** 查看 `internal/biz/biz.go`
- **THEN** `NewAgentCategoryUsecase` SHALL 变更为 `NewTaxonomyUsecase`，`NewIndustryUsecase` / `NewDepartmentUsecase` / `NewPositionUsecase` SHALL 被移除

### Requirement: Data 层重命名 — agentCategoryRepo → TaxonomyRepo
Data 层 `agentCategoryRepo` struct SHALL 重命名为 `TaxonomyRepo`，文件 `agent_category.go` SHALL 重命名为 `taxonomy.go`。

#### Scenario: Data Repo struct 重命名
- **WHEN** 查看 `internal/data/taxonomy.go`
- **THEN** `agentCategoryRepo` SHALL 重命名为 `TaxonomyRepo`，`NewAgentCategoryRepo` SHALL 重命名为 `NewTaxonomyRepo`，`entToBizCat` SHALL 重命名为 `entToBizTaxonomy`，编译期接口检查 SHALL 为 `var _ biz.TaxonomyRepo = (*TaxonomyRepo)(nil)`

#### Scenario: Data 层 Ent 查询迁移
- **WHEN** 查看 `internal/data/taxonomy.go`
- **THEN** 所有 Ent 查询 SHALL 从 `client.AgentCategory` 变更为 `client.IndustryTaxonomy`，所有 `biz.AgentCategory` 返回值 SHALL 变更为 `biz.TaxonomyNode`

#### Scenario: Data ProviderSet 更新
- **WHEN** 查看 `internal/data/data.go`
- **THEN** `NewAgentCategoryRepo` SHALL 变更为 `NewTaxonomyRepo`，`SeedBuiltinAgentCategories` SHALL 变更为 `SeedBuiltinTaxonomy`，`NewIndustryRepo` / `NewDepartmentRepo` / `NewPositionRepo` / `SeedBuiltinIndustries` SHALL 被移除

#### Scenario: Agent Repo 字段迁移
- **WHEN** 查看 `internal/data/agent_repo.go`
- **THEN** `AgentCategory.Query()` SHALL 变更为 `IndustryTaxonomy.Query()`，`CategoryPositionID` SHALL 变更为 `TaxonomyPositionID`，`agent.CategoryPositionIDEQ` SHALL 变更为 `agent.TaxonomyPositionIDEQ`，`agent.CategoryPositionIDIn` SHALL 变更为 `agent.TaxonomyPositionIDIn`，`SetCategoryPositionID` SHALL 变更为 `SetTaxonomyPositionID`

#### Scenario: Seed 文件重命名
- **WHEN** 查看 `internal/data/seed_builtin_taxonomy.go`
- **THEN** `SeedBuiltinAgentCategories` SHALL 重命名为 `SeedBuiltinTaxonomy`，SQL 表名 `agent_category_nodes` SHALL 变更为 `industry_taxonomy`，SQL 列名 `category_key` SHALL 变更为 `taxonomy_key`，SQL 列名 `category_position_id` SHALL 变更为 `taxonomy_position_id`

### Requirement: Proto 重命名 — agent_category → taxonomy
Proto 定义中 `agent_category` package SHALL 重命名为 `taxonomy`，所有 message 和 service SHALL 重命名。

#### Scenario: Proto package 和文件重命名
- **WHEN** 查看 `api/kratos/taxonomy/v1/taxonomy.proto`
- **THEN** `package agent_category.v1` SHALL 变更为 `package taxonomy.v1`，`option go_package` 路径 SHALL 更新，旧目录 `api/kratos/agent_category/` SHALL 不存在

#### Scenario: Proto message 重命名
- **WHEN** 查看 `api/kratos/taxonomy/v1/taxonomy.proto`
- **THEN** `message AgentCategory` SHALL 变更为 `message TaxonomyNode`，`message AgentCategoryTreeNode` SHALL 变更为 `message TaxonomyTreeNode`，`category_position_id` 字段 SHALL 变更为 `taxonomy_position_id`

#### Scenario: Proto service 重命名
- **WHEN** 查看 `api/kratos/taxonomy/v1/taxonomy.proto`
- **THEN** `service AgentCategoryService` SHALL 变更为 `service TaxonomyService`，HTTP 路由 `/v1/agent-categories` SHALL 变更为 `/v1/taxonomy`

#### Scenario: Proto 代码重新生成
- **WHEN** 运行 `make api`
- **THEN** SHALL 生成 `api/kratos/taxonomy/v1/taxonomy.pb.go` 等文件，旧 `api/kratos/agent_category/` 目录 SHALL 不存在

### Requirement: Service 层重命名 — AgentCategoryService → TaxonomyService
Service 层 `AgentCategoryService` SHALL 重命名为 `TaxonomyService`，文件 `agent_category.go` SHALL 重命名为 `taxonomy.go`。

#### Scenario: Service struct 重命名
- **WHEN** 查看 `internal/service/taxonomy.go`
- **THEN** `AgentCategoryService` SHALL 重命名为 `TaxonomyService`，`NewAgentCategoryService` SHALL 重命名为 `NewTaxonomyService`，`AgentCategoryUsecase` 依赖 SHALL 变更为 `TaxonomyUsecase`，import 路径 SHALL 从 `agent_category/v1` 变更为 `taxonomy/v1`

#### Scenario: 转换函数重命名
- **WHEN** 查看 `internal/service/taxonomy.go`
- **THEN** `toProtoCat` SHALL 重命名为 `toProtoTaxonomy`，`fromProtoCat` SHALL 重命名为 `fromProtoTaxonomy`，所有 `v1.AgentCategory` SHALL 变更为 `v1.TaxonomyNode`，所有 `v1.AgentCategoryTreeNode` SHALL 变更为 `v1.TaxonomyTreeNode`

#### Scenario: Service ProviderSet 更新
- **WHEN** 查看 `internal/service/service.go`
- **THEN** `NewAgentCategoryService` SHALL 变更为 `NewTaxonomyService`，`NewIndustryService` SHALL 被移除

#### Scenario: Agent Service 字段映射
- **WHEN** 查看 `internal/service/agent.go`
- **THEN** `CategoryPositionID` 字段 SHALL 变更为 `TaxonomyPositionID`，Proto 字段映射 SHALL 更新

#### Scenario: Chat Orchestrator 三表依赖移除
- **WHEN** 查看 `internal/service/chat_orchestrator.go`
- **THEN** `IndustryUC *biz.IndustryUsecase` SHALL 被移除，`DepartmentUC *biz.DepartmentUsecase` SHALL 被移除，`PositionUC *biz.PositionUsecase` SHALL 被移除，相关功能 SHALL 通过 `TaxonomyUsecase` 访问

### Requirement: Server 层注册重命名
Server 层的 gRPC/HTTP 注册和服务注册表 SHALL 更新为新的 Taxonomy 命名。

#### Scenario: gRPC 注册更新
- **WHEN** 查看 `internal/server/grpc.go`
- **THEN** `agentcategoryv1` import SHALL 变更为 `taxonomyv1`，`RegisterAgentCategoryServiceServer` SHALL 变更为 `RegisterTaxonomyServiceServer`，`s.AgentCat` SHALL 变更为 `s.Taxonomy`，`RegisterIndustryServiceServer` SHALL 被移除

#### Scenario: HTTP 注册更新
- **WHEN** 查看 `internal/server/http.go`
- **THEN** `RegisterAgentCategoryServiceHTTPServer` SHALL 变更为 `RegisterTaxonomyServiceHTTPServer`，`RegisterIndustryServiceHTTPServer` SHALL 被移除

#### Scenario: Service Registry 更新
- **WHEN** 查看 `internal/server/service_registry.go`
- **THEN** `AgentCat *service.AgentCategoryService` SHALL 变更为 `Taxonomy *service.TaxonomyService`，`Industry *service.IndustryService` SHALL 被移除

### Requirement: Agent 运行时依赖重命名
Agent 运行时（builder/prompt/trpc_build/prompt_preview）中所有 `AgentCategory` 引用 SHALL 更新为 `Taxonomy`。

#### Scenario: Prompt 依赖更新
- **WHEN** 查看 `internal/agent/prompt.go`
- **THEN** `AgentCategory *biz.AgentCategoryUsecase` SHALL 变更为 `Taxonomy *biz.TaxonomyUsecase`，`PositionUC *biz.PositionUsecase` SHALL 被移除

#### Scenario: Builder 依赖更新
- **WHEN** 查看 `internal/agent/builder_deps.go`
- **THEN** `AgentCategory *biz.AgentCategoryUsecase` SHALL 变更为 `Taxonomy *biz.TaxonomyUsecase`，`IndustryUC` / `DepartmentUC` / `PositionUC` SHALL 被移除，注释中 `agent_category_nodes` SHALL 变更为 `industry_taxonomy`

#### Scenario: Trpc Build 引用更新
- **WHEN** 查看 `internal/agent/trpc_build.go`
- **THEN** `deps.AgentCategory.BuildResponsibility` SHALL 变更为 `deps.Taxonomy.BuildResponsibility`，`CategoryPositionID` SHALL 变更为 `TaxonomyPositionID`

#### Scenario: Prompt Preview 引用更新
- **WHEN** 查看 `internal/agent/prompt_preview.go`
- **THEN** `deps.AgentCategory.BuildResponsibility` SHALL 变更为 `deps.Taxonomy.BuildResponsibility`，`CategoryPositionID` SHALL 变更为 `TaxonomyPositionID`

### Requirement: Wire 注入重命名
Wire DI 配置和 `cmd/admin` 入口 SHALL 更新为新的 Taxonomy 命名。

#### Scenario: Wire 配置更新
- **WHEN** 查看 `cmd/admin/wire.go`
- **THEN** `IndustryUsecase` / `DepartmentUsecase` / `PositionUsecase` 相关声明和赋值 SHALL 被移除，`AgentCategoryUsecase` SHALL 变更为 `TaxonomyUsecase`，`AgentCategoryService` SHALL 变更为 `TaxonomyService`

#### Scenario: Main 入口更新
- **WHEN** 查看 `cmd/admin/main.go`
- **THEN** `positionUC *biz.PositionUsecase` SHALL 被移除或变更为 `taxonomyUC *biz.TaxonomyUsecase`

#### Scenario: Wire 重新生成
- **WHEN** 运行 `make wire`
- **THEN** `wire_gen.go` SHALL 重新生成成功，无错误

### Requirement: HTTP 路由迁移
所有 `/v1/agent-categories` 路由 SHALL 迁移到 `/v1/taxonomy`。

#### Scenario: 列表路由迁移
- **WHEN** 客户端请求 `GET /v1/taxonomy`
- **THEN** 系统 SHALL 返回与原 `GET /v1/agent-categories` 相同的 TaxonomyNode 列表

#### Scenario: 树形路由迁移
- **WHEN** 客户端请求 `GET /v1/taxonomy/tree`
- **THEN** 系统 SHALL 返回与原 `GET /v1/agent-categories/tree` 相同的 TaxonomyTreeNode

#### Scenario: CRUD 路由迁移
- **WHEN** 客户端请求 `POST /v1/taxonomy`、`GET /v1/taxonomy/{id}`、`PATCH /v1/taxonomy/{id}`、`DELETE /v1/taxonomy/{id}`
- **THEN** 系统 SHALL 执行与原 `/v1/agent-categories` 对应路由相同的操作

#### Scenario: 排序路由迁移
- **WHEN** 客户端请求 `PUT /v1/taxonomy/reorder`
- **THEN** 系统 SHALL 执行与原 `PUT /v1/agent-categories/reorder` 相同的排序操作

#### Scenario: 旧路由不可用
- **WHEN** 客户端请求 `GET /v1/agent-categories`
- **THEN** 系统 SHALL 返回 404 Not Found

### Requirement: 前端 API 层重命名
前端 API 层所有 `agent-categories` 资源名和函数名 SHALL 重命名为 `taxonomy`。

#### Scenario: Service 客户端重命名
- **WHEN** 查看 `web/src/services/index.ts`
- **THEN** `createAgentCategoryServiceClient` SHALL 变更为 `createTaxonomyServiceClient`，`createAgentCategoryService` SHALL 变更为 `createTaxonomyService`，import 路径 SHALL 从 `./kratos/agent_category/v1/index` 变更为 `./kratos/taxonomy/v1/index`

#### Scenario: Platform API 函数重命名
- **WHEN** 查看 `web/src/features/platform/api.ts`
- **THEN** `agentCategoryWireToPlatform` SHALL 变更为 `taxonomyWireToPlatform`，`mapAgentCategoryTreeNode` SHALL 变更为 `mapTaxonomyTreeNode`，资源名 `"agent-categories"` SHALL 变更为 `"taxonomy"`，`ListAgentCategoryTree` SHALL 变更为 `ListTaxonomyTree`，`CreateAgentCategory` SHALL 变更为 `CreateTaxonomy`，`GetAgentCategory` SHALL 变更为 `GetTaxonomy`，`UpdateAgentCategory` SHALL 变更为 `UpdateTaxonomy`，`DeleteAgentCategory` SHALL 变更为 `DeleteTaxonomy`，`reorderAgentCategories` SHALL 变更为 `reorderTaxonomy`，`category_position_id` SHALL 变更为 `taxonomy_position_id`

#### Scenario: Platform 类型重命名
- **WHEN** 查看 `web/src/features/platform/types.ts`
- **THEN** `PlatformResourceName` 联合类型中 `"agent-categories"` SHALL 变更为 `"taxonomy"`

### Requirement: 前端组件和页面重命名
前端所有 `AgentCategory` 命名的组件和页面 SHALL 重命名为 `Taxonomy`。

#### Scenario: 组件文件重命名
- **WHEN** 查看前端组件目录
- **THEN** 以下文件重命名 SHALL 生效：`AgentCategoryTree.vue` → `TaxonomyTree.vue`，`AgentCategoryPositionCard.vue` → `TaxonomyPositionCard.vue`，`AgentCategoryPicker.vue` → `TaxonomyPicker.vue`，`AgentCategoryFilter.vue` → `TaxonomyFilter.vue`

#### Scenario: 页面文件重命名
- **WHEN** 查看前端页面目录
- **THEN** `AgentCategoriesPage.vue` SHALL 重命名为 `TaxonomyPage.vue`

#### Scenario: Composable 重命名
- **WHEN** 查看前端 feature 目录
- **THEN** `useAgentCategoriesPage.ts` SHALL 重命名为 `useTaxonomyPage.ts`

#### Scenario: 组件内部引用更新
- **WHEN** 查看重命名后的组件文件
- **THEN** CSS class 名 `agent-category-*` SHALL 变更为 `taxonomy-*`，组件 import 名和注册名 SHALL 更新

### Requirement: 前端 Store、路由、工具函数重命名
前端 Store、路由配置、工具函数中所有 `AgentCategory` / `agent-categories` 引用 SHALL 更新。

#### Scenario: Store 更新
- **WHEN** 查看 `web/src/stores/agents/index.ts` 和 `web/src/stores/platform/index.ts`
- **THEN** `"agent-categories"` 资源名 SHALL 变更为 `"taxonomy"`，`category_position_id` SHALL 变更为 `taxonomy_position_id`

#### Scenario: 路由更新
- **WHEN** 查看 `web/src/router/routes.ts`
- **THEN** `"settings/agent-categories"` SHALL 变更为 `"settings/taxonomy"`，`AgentCategoriesPage` 组件引用 SHALL 变更为 `TaxonomyPage`

#### Scenario: 导航更新
- **WHEN** 查看 `web/src/config/sideNav.ts`
- **THEN** `/settings/agent-categories` 路径 SHALL 变更为 `/settings/taxonomy`

#### Scenario: Agent feature 层更新
- **WHEN** 查看 `web/src/features/agents/types.ts` 和 `wireNormalize.ts`
- **THEN** `category_position_id: string` SHALL 变更为 `taxonomy_position_id: string`，wireNormalize 映射 SHALL 更新为 `taxonomy_position_id`

#### Scenario: 工具函数重命名
- **WHEN** 查看 `web/src/features/platform/` 目录
- **THEN** `categoryTreeUtils.ts` SHALL 重命名为 `taxonomyTreeUtils.ts`，`categoryTreeUtils.ts` 可保留为兼容 re-export 层

### Requirement: 前端 CSS/Sass 类名重命名
前端所有 `agent-category-*` CSS 类名 SHALL 重命名为 `taxonomy-*`。

#### Scenario: Entity pages Sass 更新
- **WHEN** 查看 `web/src/css/theme/_entity-pages.sass`
- **THEN** `.agent-categories-page` SHALL 变更为 `.taxonomy-page`，`.agent-category-filter` SHALL 变更为 `.taxonomy-filter`，`.agent-category-picker` SHALL 变更为 `.taxonomy-picker`，`.agent-category-field` SHALL 变更为 `.taxonomy-field`

#### Scenario: Form layout Sass 更新
- **WHEN** 查看 `web/src/css/theme/_form-layout.sass`
- **THEN** `.agent-category-field` SHALL 变更为 `.taxonomy-field`

### Requirement: Scenario Loader 重命名
Scenario loader 中 `categories` 相关命名 SHALL 重命名为 `taxonomy`。

#### Scenario: Loader 文件重命名
- **WHEN** 查看 `internal/scenario/loader/` 目录
- **THEN** `categories_loader.go` SHALL 重命名为 `taxonomy_loader.go`，`LoadCategoriesYAML` SHALL 重命名为 `LoadTaxonomySpec`，内部类型 `CategoriesSpec` SHALL 重命名为 `TaxonomySpec`，`CategoryIndustrySpec` SHALL 重命名为 `TaxonomyIndustrySpec`

#### Scenario: YAML 文件重命名
- **WHEN** 查看 `internal/scenario/` 目录
- **THEN** `categories.yaml` SHALL 重命名为 `taxonomy.yaml`

#### Scenario: Loader.go 依赖更新
- **WHEN** 查看 `internal/scenario/loader/loader.go`
- **THEN** `PositionUC *biz.PositionUsecase` SHALL 被移除，相关方法中 `PositionUsecase` 引用 SHALL 变更为 `TaxonomyUsecase`

### Requirement: CLI 工具引用更新
所有 CLI 工具中的 `AgentCategory` 引用 SHALL 更新为 `Taxonomy`。

#### Scenario: orgimport 路由更新
- **WHEN** 查看 `internal/orgimport/applier.go`
- **THEN** `/v1/agent-categories/` SHALL 变更为 `/v1/taxonomy/`，`category_position_id` SHALL 变更为 `taxonomy_position_id`

### Requirement: 全链路编译验证
重命名完成后 SHALL 通过全量编译和测试验证。

#### Scenario: 后端编译验证
- **WHEN** 运行 `make api && make wire && make build && make test && make lint`
- **THEN** 所有命令 SHALL 成功通过，无错误

#### Scenario: 前端编译验证
- **WHEN** 运行 `cd web && pnpm lint && pnpm build`
- **THEN** 所有命令 SHALL 成功通过，无错误

#### Scenario: 残留引用验证
- **WHEN** 在后端代码中 grep `AgentCategory` / `agent_category` / `IndustryUsecase` / `DepartmentUsecase` / `PositionUsecase`
- **THEN** SHALL 返回 0 处匹配（注释中的历史说明除外）

#### Scenario: 前端残留引用验证
- **WHEN** 在前端代码中 grep `AgentCategory` / `agent-category` / `agent-categories`
- **THEN** SHALL 返回 0 处匹配（注释中的历史说明除外）
