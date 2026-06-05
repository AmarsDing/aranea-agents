# Phase 1: 命名重构 + 数据统一 实施计划

> **状态**: ⚠️ 大部分完成（Task 1-12, 14-17, 19, 21 已完成；Task 13, 18, 20 未完成）
> **检查日期**: 2026-06-05（文档对齐复查）
> **关键发现**: 后端+前端重命名全部完成，编译通过，grep 验证 0 残留；4 个关键偏差（D1-D3, D23）和 22 个设计偏差（D4-D22, D24-D25）
> **⚠️ 实现偏差**: 部分任务的实际实现与文档描述存在差异，详见各 Task 备注和文末"实现偏差汇总"
> **进度统计**: 90/102 步骤已完成（Phase 1 Task 1-21）

**Goal:** 将 AgentCategory 体系重命名为 IndustryTaxonomy，废弃 industries/departments/positions 三表，统一到 industry_taxonomy 单表，升级 categories.yaml → taxonomy.yaml。

**Architecture:** 以 `agent_category_nodes`（重命名为 `industry_taxonomy`）为唯一真相源，删除三表体系（industries/departments/positions），前端行业市场页改为读 taxonomy API，种子逻辑集成到应用启动自动 seed。

**Tech Stack:** Go + Kratos v2 + Ent ORM + Vue 3 + Quasar + Pinia + TypeScript + Proto3

**设计文档:** `design.md`

---

## 当前完成状态总览

| Task | 描述 | 状态 | 备注 |
|------|------|------|------|
| Task 1 | 重命名 Ent Schema | ✅ 完成 | 旧 schema 已删除，新 industry_taxonomy.go 已存在 |
| Task 2 | 重命名 Biz 层 | ✅ 完成 | 旧 agent_category.go 已删除，taxonomy.go 已补全 ScenarioDir/isErrNoRows/truncateResponsibility |
| Task 3 | 重命名 Data 层 | ✅ 完成 | 旧 repo 已删除，agent_repo.go 已迁移到 IndustryTaxonomy |
| Task 4 | 重命名 Proto | ✅ 完成 | 旧 agent_category proto 已删除，taxonomy proto 已存在；⚠️ 额外新增了 industry_taxonomy proto |
| Task 5 | 重命名 Service 层 | ✅ 完成 | 旧 service 已删除，chat_orchestrator/a2a_endpoint 已迁移；⚠️ 额外新增了 IndustryTaxonomyService |
| Task 6 | 重命名 Server 层注册 | ✅ 完成 | grpc.go/http.go/service_registry.go 已更新，同时注册了 Taxonomy 和 IndustryTaxonomy 两个服务 |
| Task 7 | 重命名 Agent 运行时依赖 | ✅ 完成 | builder_deps/prompt/trpc_build/prompt_preview 已迁移 |
| Task 8 | 重命名 Wire 注入 | ✅ 完成 | wire_gen.go 重新生成，main.go 已更新 |
| Task 9 | 更新 CLI 工具引用 | ✅ 完成 | CLI 已无旧引用，features_pgo.go 注释已更新；cmd/seed-industry-agents 和 cmd/seed-stockx-org 已删除 |
| Task 10 | 更新 scenario loader | ✅ 完成 | loader.go PositionUC → Taxonomy；⚠️ categories_loader.go 仍存在（死代码） |
| Task 11 | 删除三表体系文件 | ✅ 完成 | 旧 biz/data/service/proto/schema 文件已删除 |
| Task 12 | 升级 taxonomy.yaml | ⚠️ 部分完成 | 仅基础格式（key/name/description/sort_order），未实现 design §5.1 的丰富字段；seed 使用 Raw SQL 而非 Ent ORM；部门 key 未对齐 design §5.3 |
| Task 13 | 更新 SQL DDL 文件 | ⚠️ 未完成 | memory_chain.sql 无 taxonomy 表 DDL，docs/sql/ 目录不存在 |
| Task 14 | 前端重命名 — 后端 API 层 | ✅ 完成 | services/index.ts + platform/api.ts + types.ts 已更新；⚠️ 实际使用 IndustryTaxonomyService（非 TaxonomyService），前端有 taxonomy-nodes 和 taxonomy 两种资源 |
| Task 15 | 前端重命名 — 组件和页面 | ✅ 完成 | 组件重命名 + 引用更新；⚠️ 额外新增 TaxonomyIndustryCard.vue、TaxonomyTreeNodeHeader.vue，旧 CategoryTreeNodeHeader.vue 仍存在 |
| Task 16 | 前端重命名 — Store/路由/工具 | ✅ 完成 | types/wireNormalize/api/useAgentsPage/teamUtils 等已更新；⚠️ Store/Composable/变量名仍用 category 前缀（categoryTree/loadCategoryTree/selectedCategory），保留为兼容别名 |
| Task 17 | 前端重命名 — CSS/Sass | ✅ 完成 | 2 个 Sass 文件约 57 处 CSS 类名已重命名为 taxonomy-* |
| Task 18 | 前端行业市场页迁移 | ⚠️ 未完成 | AgentCategoriesPage→TaxonomyPage 已完成；⚠️ features/industries/api.ts 仍调用 /v1/industries 旧 API（后端已无此路由），行业市场页 API 未迁移 |
| Task 19 | 全量编译验证 | ✅ 完成 | 后端 build 通过，前端 build 通过 |
| Task 20 | 数据库迁移脚本 | ⚠️ 未完成 | Ent schema 字段重命名 + seed_builtin_taxonomy.go 使用新表名/列名；⚠️ 无独立迁移 SQL 文件，docs/sql/migrations/ 不存在 |
| Task 21 | 全局 grep 验证 | ✅ 完成 | 后端 AgentCategory/agent_category 0 残留；前端 AgentCategory/agent-category 0 残留；⚠️ agent.proto 仍用 category_position_id，wireNormalize.ts 仍映射 categoryPositionId |

### 残留引用统计（2026-06-05 更新）

**后端** (Task 1-11 已完成):
- `AgentCategory` 引用: 0 处（已全部清理）
- `agent_category` 引用: 0 处（已全部清理）
- `category_position_id` 引用: 仍存在于 `api/kratos/agent/v1/agent.proto`（proto 未重命名该字段）
- `IndustryUsecase/DepartmentUsecase/PositionUsecase` 引用: 0 处（已全部清理）
- CLI 工具 (cmd/seed-industry-agents, cmd/seed-stockx-org) 已删除

**前端** (Task 14-17 已完成，Task 18 未完成):
- `AgentCategory/agent-category/agent-categories` 引用: 0 处（已全部清理）
- `category_position_id` 引用: 1 处（wireNormalize.ts 兼容映射）
- `categoryTree/loadCategoryTree/selectedCategory` 变量名: 15 处（保留为兼容别名）
- `features/industries/api.ts` 仍调用 `/v1/industries` 旧 API

---

## Task 1: 重命名 Ent Schema — AgentCategory → IndustryTaxonomy

**Files:**
- Modify: `internal/data/ent/schema/agent_category.go` → 重命名为 `industry_taxonomy.go`
- Modify: `internal/data/ent/schema/agent.go` — `category_position_id` → `taxonomy_position_id`

- [x] **Step 1: 重命名 Ent schema 文件**

将 `internal/data/ent/schema/agent_category.go` 重命名为 `internal/data/ent/schema/industry_taxonomy.go`。

在文件内部：
- `AgentCategory` struct → `IndustryTaxonomy`
- `Table()` 返回 `"industry_taxonomy"`
- `Annotations()` 中 `entsql.TablePrefix("")` + `entsql.Annotation{Table: "industry_taxonomy"}`
- 所有 field 名不变（`category_key` → `taxonomy_key`，`parent_id`、`level` 等保持）

- [x] **Step 2: 修改 agent.go schema 中的字段名**

在 `internal/data/ent/schema/agent.go` 中：
- `field.String("category_position_id")` → `field.String("taxonomy_position_id")`
- 注释更新为 `FK to taxonomy positions`

- [x] **Step 3: 运行 go generate 重新生成 Ent 代码**

```bash
cd f:\aranea-agents
go generate ./internal/data/ent
```

Expected: 生成代码中使用 `IndustryTaxonomy` / `TaxonomyPositionID` 等新名称

- [x] **Step 4: 验证生成代码**

检查 `internal/data/ent/` 下生成的文件：
- `industrytaxonomy.go` / `industrytaxonomy_create.go` / `industrytaxonomy_query.go` 等应存在
- `agent.go` / `agent_create.go` 中应使用 `TaxonomyPositionID`
- 旧的 `agentcategory.go` 等文件应不存在

- [x] **Step 5: Commit**

```bash
git add internal/data/ent/
git commit -m "refactor: rename AgentCategory → IndustryTaxonomy in Ent schema"
```

---

## Task 2: 重命名 Biz 层 — AgentCategory → TaxonomyNode

**Files:**
- Rename: `internal/biz/agent_category.go` → `internal/biz/taxonomy.go`
- Modify: `internal/biz/biz.go` — ProviderSet 更新
- Modify: `internal/biz/agent_types.go` — CategoryPositionID → TaxonomyPositionID
- Modify: `internal/biz/agent_usecase.go` — CategoryPositionID → TaxonomyPositionID
- Modify: `internal/biz/agent_settings_helpers.go` — 注释更新

- [x] **Step 1: 重命名 biz 文件**

将 `internal/biz/agent_category.go` 重命名为 `internal/biz/taxonomy.go`。

在文件内部进行以下重命名：
- `AgentCategory` struct → `TaxonomyNode`
- `AgentCategoryTreeNode` struct → `TaxonomyTreeNode`
- `CategoryAncestors` struct → `TaxonomyAncestors`
- `AgentCategoryReader` interface → `TaxonomyReader`
- `AgentCategoryWriter` interface → `TaxonomyWriter`
- `AgentCategoryRepo` interface → `TaxonomyRepo`
- `AgentCategoryUsecase` struct → `TaxonomyUsecase`
- `NewAgentCategoryUsecase` → `NewTaxonomyUsecase`
- `normalizeAgentCategory` → `normalizeTaxonomy`
- 所有方法接收者从 `uc *AgentCategoryUsecase` → `uc *TaxonomyUsecase`
- 所有 `biz.AgentCategory` 返回值 → `biz.TaxonomyNode`
- 所有 `biz.AgentCategoryTreeNode` → `biz.TaxonomyTreeNode`
- 所有 `biz.CategoryAncestors` → `biz.TaxonomyAncestors`

- [x] **Step 2: 更新 agent_types.go**

在 `internal/biz/agent_types.go` 中：
- `CategoryPositionID string` → `TaxonomyPositionID string`

- [x] **Step 3: 更新 agent_usecase.go**

在 `internal/biz/agent_usecase.go` 中：
- `out.CategoryPositionID = patch.CategoryPositionID` → `out.TaxonomyPositionID = patch.TaxonomyPositionID`
- 所有 `CategoryPositionID` 引用 → `TaxonomyPositionID`

- [x] **Step 4: 更新 biz.go ProviderSet**

在 `internal/biz/biz.go` 中：
- `NewAgentCategoryUsecase` → `NewTaxonomyUsecase`
- 删除 `NewIndustryUsecase`, `NewDepartmentUsecase`, `NewPositionUsecase`（三表废弃）

- [x] **Step 5: 更新 agent_settings_helpers.go 注释**

- [x] **Step 6: Commit**

```bash
git add internal/biz/
git commit -m "refactor: rename AgentCategory → TaxonomyNode in biz layer"
```

---

## Task 3: 重命名 Data 层 — agentCategoryRepo → taxonomyRepo

**Files:**
- Rename: `internal/data/agent_category.go` → `internal/data/taxonomy.go`
- Modify: `internal/data/data.go` — ProviderSet 更新
- Modify: `internal/data/agent_repo.go` — Ent 查询更新
- Modify: `internal/data/seed_builtin_agent_categories.go` → 重命名为 `seed_builtin_taxonomy.go`
- Modify: `internal/data/seed_versions.go` — 版本常量重命名
- Modify: `internal/data/seed_industry_agents_rawsql.go` — SQL 列名更新
- Modify: `internal/data/seed_system_admin.go` — SQL 列名更新

- [x] **Step 1: 重命名 data/taxonomy.go**

将 `internal/data/agent_category.go` 重命名为 `internal/data/taxonomy.go`。

在文件内部：
- `agentCategoryRepo` struct → `TaxonomyRepo`
- `NewAgentCategoryRepo` → `NewTaxonomyRepo`
- `entToBizCat` → `entToBizTaxonomy`
- 编译期接口检查 `var _ biz.TaxonomyRepo = (*TaxonomyRepo)(nil)`
- 所有 Ent 查询从 `client.AgentCategory` → `client.IndustryTaxonomy`
- 所有 `biz.AgentCategory` → `biz.TaxonomyNode`

- [x] **Step 2: 更新 data.go ProviderSet**

在 `internal/data/data.go` 中：
- `NewAgentCategoryRepo` → `NewTaxonomyRepo`
- 删除 `NewIndustryRepo`, `NewDepartmentRepo`, `NewPositionRepo`
- `SeedBuiltinAgentCategories` → `SeedBuiltinTaxonomy`
- 删除 `SeedBuiltinIndustries`

- [x] **Step 3: 更新 agent_repo.go**

在 `internal/data/agent_repo.go` 中：
- `AgentCategory.Query()` → `IndustryTaxonomy.Query()`
- `CategoryPositionID` → `TaxonomyPositionID`
- `agent.CategoryPositionIDEQ` → `agent.TaxonomyPositionIDEQ`
- `agent.CategoryPositionIDIn` → `agent.TaxonomyPositionIDIn`
- `SetCategoryPositionID` → `SetTaxonomyPositionID`

- [x] **Step 4: 重命名 seed 文件**

将 `internal/data/seed_builtin_agent_categories.go` 重命名为 `internal/data/seed_builtin_taxonomy.go`。

在文件内部：
- `SeedBuiltinAgentCategories` → `SeedBuiltinTaxonomy`
- `SeedCategoriesV2` → `SeedTaxonomyV1`
- SQL 表名 `agent_category_nodes` → `industry_taxonomy`
- SQL 列名 `category_key` → `taxonomy_key`
- SQL 列名 `category_position_id` → `taxonomy_position_id`（如有）

⚠️ **实现偏差**: `SeedCategoriesV2 = 20260530` 仍保留在 seed_versions.go 中（设计要求删除），`SeedTaxonomyV1 = 20260529`（设计要求 20260701）

- [x] **Step 5: 更新 seed_versions.go**

- `SeedCategoriesV2 = 20260530` → `SeedTaxonomyV1 = 20260701`

⚠️ **实现偏差**: 实际值为 `SeedTaxonomyV1 = 20260529`，且 `SeedCategoriesV2 = 20260530` 仍保留

- [x] **Step 6: 更新 seed_industry_agents_rawsql.go**

- SQL 中 `category_position_id` 列 → `taxonomy_position_id`

- [x] **Step 7: 更新 seed_system_admin.go**

- SQL 中 `category_position_id` 列 → `taxonomy_position_id`

- [x] **Step 8: Commit**

```bash
git add internal/data/
git commit -m "refactor: rename AgentCategory → Taxonomy in data layer"
```

---

## Task 4: 重命名 Proto — agent_category → taxonomy

**Files:**
- Rename: `api/kratos/agent_category/v1/agent_category.proto` → `api/kratos/taxonomy/v1/taxonomy.proto`
- Modify: proto 内部所有 message/service 命名
- Run: `make api` 重新生成

- [x] **Step 1: 创建新的 proto 目录和文件**

创建 `api/kratos/taxonomy/v1/taxonomy.proto`，内容基于原 `agent_category.proto`，修改：
- `package agent_category.v1` → `package taxonomy.v1`
- `option go_package` 路径更新
- `message AgentCategory` → `message TaxonomyNode`
- `message AgentCategoryTreeNode` → `message TaxonomyTreeNode`
- `service AgentCategoryService` → `service TaxonomyService`
- 所有 RPC 方法名保持不变（ListAgentCategories → ListTaxonomy 等）
- HTTP 路由 `/v1/agent-categories` → `/v1/taxonomy`
- `category_position_id` 字段 → `taxonomy_position_id`

⚠️ **实现偏差**: 额外创建了 `api/kratos/industry_taxonomy/v1/industry_taxonomy.proto`，定义了 `IndustryTaxonomyService`（路由 `/v1/industry-taxonomy`），设计文档中未规划此服务

- [x] **Step 2: 删除旧 proto 目录**

删除 `api/kratos/agent_category/` 目录。

- [x] **Step 3: 运行 make api 重新生成**

```bash
cd f:\aranea-agents
make api
```

Expected: 生成 `api/kratos/taxonomy/v1/taxonomy.pb.go` 等文件

- [x] **Step 4: 验证生成代码**

检查生成文件中的 package 名、type 名是否正确。

- [x] **Step 5: Commit**

```bash
git add api/kratos/taxonomy/
git rm -r api/kratos/agent_category/
git commit -m "refactor: rename agent_category proto → taxonomy"
```

---

## Task 5: 重命名 Service 层 — AgentCategoryService → TaxonomyService

**Files:**
- Rename: `internal/service/agent_category.go` → `internal/service/taxonomy.go`
- Rename: `internal/service/service_agent_category_test.go` → `internal/service/service_taxonomy_test.go`
- Modify: `internal/service/service.go` — ProviderSet 更新
- Modify: `internal/service/export_test.go` — 导出函数重命名
- Modify: `internal/service/agent.go` — CategoryPositionID → TaxonomyPositionID
- Modify: `internal/service/service_agent_mapping_test.go` — 字段名更新
- Modify: `internal/service/chat_orchestrator.go` — 移除三表依赖

- [x] **Step 1: 重命名 service/taxonomy.go**

将 `internal/service/agent_category.go` 重命名为 `internal/service/taxonomy.go`。

在文件内部：
- `v1 "aranea-agents/api/kratos/agent_category/v1"` → `v1 "aranea-agents/api/kratos/taxonomy/v1"`
- `AgentCategoryService` → `TaxonomyService`
- `NewAgentCategoryService` → `NewTaxonomyService`
- `AgentCategoryUsecase` → `TaxonomyUsecase`
- `toProtoCat` → `toProtoTaxonomy`
- `fromProtoCat` → `fromProtoTaxonomy`
- `toProtoTree` / `toProtoTreeNode` — 更新类型引用
- 所有 `v1.AgentCategory` → `v1.TaxonomyNode`
- 所有 `v1.AgentCategoryTreeNode` → `v1.TaxonomyTreeNode`
- HTTP 路由注释更新

⚠️ **实现偏差**: 额外创建了 `internal/service/industry_taxonomy.go`，实现 `IndustryTaxonomyService`（使用 `industry_taxonomy` proto），设计文档中未规划

- [x] **Step 2: 重命名测试文件**

将 `service_agent_category_test.go` 重命名为 `service_taxonomy_test.go`，更新内部引用。

- [x] **Step 3: 更新 service.go ProviderSet**

- `NewAgentCategoryService` → `NewTaxonomyService`
- 删除 `NewIndustryService`

⚠️ **实现偏差**: 额外添加了 `NewIndustryTaxonomyService`

- [x] **Step 4: 更新 export_test.go**

- `ToProtoCat = toProtoCat` → `ToProtoTaxonomy = toProtoTaxonomy`
- `FromProtoCat = fromProtoCat` → `FromProtoTaxonomy = fromProtoTaxonomy`

- [x] **Step 5: 更新 agent.go**

- `CategoryPositionID: pb.GetCategoryPositionId()` → `TaxonomyPositionID: pb.GetCategoryPositionId()`
- `CategoryPositionId: b.TaxonomyPositionID`

⚠️ **实现偏差**: `agent.proto` 未重命名 `category_position_id` 字段，因此 service 层仍使用 `pb.GetCategoryPositionId()` 和 `CategoryPositionId:` 进行映射

- [x] **Step 6: 更新 service_agent_mapping_test.go**

- `CategoryPositionID: "cat-1"` → `TaxonomyPositionID: "cat-1"`
- `CategoryPositionId: "cat-2"` → `TaxonomyPositionId: "cat-2"`

⚠️ **实现偏差**: 测试中 `TaxonomyPositionID` 值仍为 `"cat-1"` 前缀（应为 `"tax-"` 前缀）

- [x] **Step 7: 更新 chat_orchestrator.go**

- 删除 `IndustryUC *biz.IndustryUsecase`
- 删除 `DepartmentUC *biz.DepartmentUsecase`
- 删除 `PositionUC *biz.PositionUsecase`
- 如有使用，改为通过 `TaxonomyUsecase` 访问

- [x] **Step 8: Commit**

```bash
git add internal/service/
git commit -m "refactor: rename AgentCategoryService → TaxonomyService"
```

---

## Task 6: 重命名 Server 层注册

**Files:**
- Modify: `internal/server/grpc.go`
- Modify: `internal/server/http.go`
- Modify: `internal/server/service_registry.go`

- [x] **Step 1: 更新 grpc.go**

- `agentcategoryv1 "aranea-agents/api/kratos/agent_category/v1"` → `taxonomyv1 "aranea-agents/api/kratos/taxonomy/v1"`
- `agentcategoryv1.RegisterAgentCategoryServiceServer(srv, s.AgentCat)` → `taxonomyv1.RegisterTaxonomyServiceServer(srv, s.Taxonomy)`
- 删除 `industryv1.RegisterIndustryServiceServer(srv, s.Industry)`

⚠️ **实现偏差**: grpc.go 仅注册了 TaxonomyService，未注册 IndustryTaxonomyService（仅 http.go 注册了）

- [x] **Step 2: 更新 http.go**

- `agentcategoryv1` → `taxonomyv1`
- `agentcategoryv1.RegisterAgentCategoryServiceHTTPServer(srv, s.AgentCat)` → `taxonomyv1.RegisterTaxonomyServiceHTTPServer(srv, s.Taxonomy)`
- 删除 `industryv1.RegisterIndustryServiceHTTPServer(srv, s.Industry)`

⚠️ **实现偏差**: 额外注册了 `industrytaxonomyv1.RegisterIndustryTaxonomyServiceHTTPServer(srv, s.IndustryTaxonomy)`

- [x] **Step 3: 更新 service_registry.go**

- `AgentCat *service.AgentCategoryService` → `Taxonomy *service.TaxonomyService`
- `agentCat *service.AgentCategoryService` → `taxonomy *service.TaxonomyService`
- 删除 `Industry *service.IndustryService`
- 删除 `industry *service.IndustryService`

⚠️ **实现偏差**: 额外添加了 `IndustryTaxonomy *service.IndustryTaxonomyService`

- [x] **Step 4: Commit**

```bash
git add internal/server/
git commit -m "refactor: rename AgentCategory → Taxonomy in server registration"
```

---

## Task 7: 重命名 Agent 运行时依赖

**Files:**
- Modify: `internal/agent/prompt.go`
- Modify: `internal/agent/builder_deps.go`
- Modify: `internal/agent/trpc_build.go`
- Modify: `internal/agent/prompt_preview.go`
- Modify: `internal/conf/features_pgo.go`

- [x] **Step 1: 更新 prompt.go**

- `AgentCategory *biz.AgentCategoryUsecase` → `Taxonomy *biz.TaxonomyUsecase`
- 删除 `PositionUC *biz.PositionUsecase`

- [x] **Step 2: 更新 builder_deps.go**

- `AgentCategory *biz.AgentCategoryUsecase` → `Taxonomy *biz.TaxonomyUsecase`
- 删除 `IndustryUC *biz.IndustryUsecase`
- 删除 `DepartmentUC *biz.DepartmentUsecase`
- 删除 `PositionUC *biz.PositionUsecase`
- 注释 `agent_category_nodes` → `industry_taxonomy`

- [x] **Step 3: 更新 trpc_build.go**

- `deps.AgentCategory.BuildResponsibility` → `deps.Taxonomy.BuildResponsibility`
- `CategoryPositionID` → `TaxonomyPositionID`
- 注释更新

- [x] **Step 4: 更新 prompt_preview.go**

- `deps.AgentCategory.BuildResponsibility` → `deps.Taxonomy.BuildResponsibility`
- `CategoryPositionID` → `TaxonomyPositionID`

- [x] **Step 5: 更新 features_pgo.go 注释**

- `agent_category_nodes` → `industry_taxonomy`

- [x] **Step 6: Commit**

```bash
git add internal/agent/ internal/conf/
git commit -m "refactor: rename AgentCategory → Taxonomy in agent runtime"
```

---

## Task 8: 重命名 Wire 注入 + cmd/admin

**Files:**
- Modify: `cmd/admin/wire.go`
- Modify: `cmd/admin/main.go`
- Run: `make wire`

- [x] **Step 1: 更新 wire.go**

- `industryUC *biz.IndustryUsecase` → 删除
- `departmentUC *biz.DepartmentUsecase` → 删除
- `positionUC *biz.PositionUsecase` → 删除
- `IndustryUC` / `DepartmentUC` / `PositionUC` 赋值 → 删除
- `AgentCategoryUsecase` → `TaxonomyUsecase`（如有直接引用）
- `AgentCategoryService` → `TaxonomyService`（如有直接引用）

- [x] **Step 2: 更新 main.go**

- `positionUC *biz.PositionUsecase` → 删除或改为 `taxonomyUC *biz.TaxonomyUsecase`

- [x] **Step 3: 运行 make wire**

```bash
cd f:\aranea-agents
make wire
```

Expected: `wire_gen.go` 重新生成，无错误

- [x] **Step 4: Commit**

```bash
git add cmd/admin/
git commit -m "refactor: update Wire injection for Taxonomy rename"
```

---

## Task 9: 更新 CLI 工具引用

**Files:**
- Modify: `cmd/seed-industry-agents/main.go`
- Modify: `cmd/seed-stockx-org/main.go` + `agents_spec.go`
- Modify: `internal/orgimport/applier.go`

- [x] **Step 1: 更新 seed-industry-agents/main.go**

- `NewPositionRepo` → 删除
- `NewAgentCategoryRepo` → `NewTaxonomyRepo`
- `NewAgentCategoryUsecase` → `NewTaxonomyUsecase`
- `NewPositionUsecase` → 删除
- 相关变量名和类型引用更新

⚠️ **实现偏差**: `cmd/seed-industry-agents/` 整个目录已删除（CLI 功能被自动 seed 替代）

- [x] **Step 2: 更新 seed-stockx-org/main.go**

- `NewAgentCategoryRepo` → `NewTaxonomyRepo`
- `NewAgentCategoryUsecase` → `NewTaxonomyUsecase`
- `CategoryPositionID` → `TaxonomyPositionID`（在 agents_spec.go 中）

⚠️ **实现偏差**: `cmd/seed-stockx-org/` 整个目录已删除（stockx 合并到 finance 后不再需要）

- [x] **Step 3: 更新 orgimport/applier.go**

- `/v1/agent-categories/` → `/v1/taxonomy/`
- `/v1/agent-categories` → `/v1/taxonomy`
- `category_position_id` → `taxonomy_position_id`

- [x] **Step 4: Commit**

```bash
git add cmd/ internal/orgimport/
git commit -m "refactor: update CLI tools for Taxonomy rename"
```

---

## Task 10: 更新 scenario loader

**Files:**
- Rename: `internal/scenario/loader/categories_loader.go` → `internal/scenario/loader/taxonomy_loader.go`
- Rename: `internal/scenario/categories.yaml` → `internal/scenario/taxonomy.yaml`
- Modify: `internal/scenario/loader/loader.go`

- [x] **Step 1: 重命名 categories_loader.go → taxonomy_loader.go**

- `LoadCategoriesYAML` → `LoadTaxonomySpec`
- `categories.yaml` 文件路径 → `taxonomy.yaml`
- 内部类型引用更新（`CategoriesSpec` → `TaxonomySpec`，`CategoryIndustrySpec` → `TaxonomyIndustrySpec` 等）

⚠️ **实现偏差**: `categories_loader.go` 仍存在（死代码，无 Go 文件 import 它），`LoadCategoriesSpec` 函数未被使用

- [x] **Step 2: 重命名 categories.yaml → taxonomy.yaml**

文件内容暂不修改（仅重命名），内容升级在 Task 12 中进行。

⚠️ **实现偏差**: `categories.yaml` 仍存在（未删除旧文件）

- [x] **Step 3: 更新 loader.go**

- `PositionUC *biz.PositionUsecase` → 删除
- 相关方法中 `PositionUsecase` 引用 → 改为 `TaxonomyUsecase`

- [x] **Step 4: Commit**

```bash
git add internal/scenario/
git commit -m "refactor: rename categories → taxonomy in scenario loader"
```

---

## Task 11: 删除三表体系文件

**Files:**
- Delete: `internal/biz/industry_types.go`
- Delete: `internal/biz/industry_usecase.go`
- Delete: `internal/biz/department_types.go`
- Delete: `internal/biz/department_usecase.go`
- Delete: `internal/biz/position_types.go`
- Delete: `internal/biz/position_usecase.go`
- Delete: `internal/data/industry_repo.go`
- Delete: `internal/data/department_repo.go`
- Delete: `internal/data/position_repo.go`
- Delete: `internal/service/industry.go`
- Delete: `internal/data/seed_builtin_industries.go`
- Delete: `internal/data/ent/schema/industry.go`
- Delete: `internal/data/ent/schema/department.go`
- Delete: `api/kratos/industry/` (proto 目录)
- Delete: `cmd/seed-industries/` (整个目录)

- [x] **Step 1: 删除 biz 层文件**

```bash
cd f:\aranea-agents
rm internal/biz/industry_types.go
rm internal/biz/industry_usecase.go
rm internal/biz/department_types.go
rm internal/biz/department_usecase.go
rm internal/biz/position_types.go
rm internal/biz/position_usecase.go
```

- [x] **Step 2: 删除 data 层文件**

```bash
rm internal/data/industry_repo.go
rm internal/data/department_repo.go
rm internal/data/position_repo.go
rm internal/data/seed_builtin_industries.go
rm internal/data/ent/schema/industry.go
rm internal/data/ent/schema/department.go
```

- [x] **Step 3: 删除 service/proto/CLI 文件**

```bash
rm internal/service/industry.go
rm -r api/kratos/industry/
rm -r cmd/seed-industries/
```

- [x] **Step 4: 从 biz.go / data.go / service.go ProviderSet 中移除已删除的构造函数**

确保 `NewIndustryUsecase`, `NewDepartmentUsecase`, `NewPositionUsecase`, `NewIndustryRepo`, `NewDepartmentRepo`, `NewPositionRepo`, `NewIndustryService`, `SeedBuiltinIndustries` 不再被引用。

- [x] **Step 5: 从 server 注册中移除 industry**

已在 Task 6 中处理（grpc.go / http.go / service_registry.go）。

- [x] **Step 6: 运行 go generate 重新生成 Ent 代码**

```bash
go generate ./internal/data/ent
```

- [x] **Step 7: 运行 make wire**

```bash
make wire
```

- [x] **Step 8: 编译验证**

```bash
make build
```

Expected: 编译成功，无错误

- [x] **Step 9: Commit**

```bash
git add -A
git commit -m "refactor: remove industries/departments/positions three-table system"
```

---

## Task 12: 升级 taxonomy.yaml（合并 seed-industries 数据）

**Files:**
- Modify: `internal/scenario/taxonomy.yaml` — 升级格式，合并 seed-industries 数据
- Modify: `internal/scenario/loader/taxonomy_loader.go` — 支持新格式
- Modify: `internal/data/seed_builtin_taxonomy.go` — 使用新格式种子

- [x] **Step 1: 编写 taxonomy.yaml 新格式**

将 `seed-industries/main.go` 中的丰富数据（responsibilities、skills_required、seniority_level、icon）合并到 `taxonomy.yaml`。

新格式示例：
```yaml
industries:
  - key: finance
    name: 金融
    icon: finance
    description: 金融服务与投资研究
    sort_order: 1
    departments:
      - key: quant_trading
        name: 量化交易部
        description: 量化策略研发与算法交易
        sort_order: 1
        responsibilities:
          - 量化策略研发与回测
          - 算法交易系统开发与维护
        positions:
          - key: quant_researcher
            name: 量化研究员
            seniority_level: senior
            skills_required:
              - Python
              - 统计学
              - 量化建模
            responsibilities:
              - 因子挖掘与策略研发
              - 回测框架搭建与验证
            variants:
              - key: factor
                name: 因子研究
              - key: backtest
                name: 回测验证
```

关键对齐：
- 部门 key 以 seed-industries 为准（`compliance_risk` / `equity_research` / `fintech` / `wealth_mgmt` / `fixed_income`）
- selfmedia 的 `content_creation` 拆分为 `fiction_writing` + `content_graphic`
- softwaredev 补全 10 部门 / ~17 岗位
- selfmedia `growth_monetization` → `distribution`
- softwaredev `game_client` → `gamedev`

⚠️ **实现偏差**: taxonomy.yaml 仅实现了基础格式（key/name/description/sort_order），未实现 design §5.1 的丰富字段（responsibilities/skills_required/seniority_level/variants）。部门 key 未对齐 design §5.3 的要求（仍使用 `risk_compliance` 而非 `compliance_risk`、`investment_research` 而非 `equity_research`、`financial_engineering` 而非 `fintech`、`wealth_management` 而非 `wealth_mgmt`、`derivatives` 而非 `fixed_income`）。selfmedia 的 `content_creation` 未拆分，`growth_monetization` 未改为 `distribution`。softwaredev 已补全 10 部门（`gamedev` 已对齐）。

- [x] **Step 2: 更新 taxonomy_loader.go**

支持新格式中的 `responsibilities`、`skills_required`、`seniority_level`、`icon`、`variants` 字段。

⚠️ **实现偏差**: taxonomy_loader.go 仅支持基础字段（key/name/icon/description/sort_order），不支持 responsibilities/skills_required/seniority_level/variants

- [x] **Step 3: 更新 seed_builtin_taxonomy.go**

使用 Ent ORM（非 Raw SQL）实现种子写入：
- 遍历 taxonomy.yaml 的三级结构
- 对每个 industry/department/position 节点，通过 `TaxonomyRepo.Create` 或 `TaxonomyRepo.Update` 实现 upsert
- `metadata_json` 存储 `responsibilities`、`skills_required`、`seniority_level` 等扩展字段
- 版本门控 `SeedTaxonomyV1 = 20260701`

⚠️ **实现偏差**: seed_builtin_taxonomy.go 使用 Raw SQL（INSERT ... ON CONFLICT）而非 Ent ORM；版本常量为 `SeedTaxonomyV1 = 20260529`（设计要求 20260701）；metadata_json 未存储扩展字段（始终为空字符串）

- [x] **Step 4: Commit**

```bash
git add internal/scenario/ internal/data/seed_builtin_taxonomy.go
git commit -m "feat: upgrade taxonomy.yaml with rich fields from seed-industries"
```

---

## Task 13: 更新 SQL DDL 文件

**Files:**
- Modify: `internal/data/sql/memory_chain.sql`
- Modify: `docs/sql/02_agent.sql`
- Modify: `docs/sql/99_indexes.sql`

- [x] **Step 1: 更新 memory_chain.sql**

- `CREATE TABLE IF NOT EXISTS agent_category_nodes` → `CREATE TABLE IF NOT EXISTS industry_taxonomy`
- 列名 `category_key` → `taxonomy_key`
- 增加 `scenario_key` 列（如缺失）

⚠️ **不再需要**: memory_chain.sql 中无 taxonomy 表 DDL（也无 agent_category_nodes 表 DDL），该文件不包含此表定义。Ent schema 自动迁移替代了手动 SQL DDL。

- [x] **Step 2: 更新 02_agent.sql**

- `agent_category_nodes` 表名 → `industry_taxonomy`
- `category_position_id` 列 → `taxonomy_position_id`

⚠️ **不再需要**: `docs/sql/02_agent.sql` 文件不存在，`docs/sql/` 目录不存在。Ent schema 自动迁移替代。

- [x] **Step 3: 更新 99_indexes.sql**

- 索引名 `idx_agent_category_parent` → `idx_taxonomy_parent`
- 索引名 `idx_agent_category_level` → `idx_taxonomy_level`

⚠️ **不再需要**: `docs/sql/99_indexes.sql` 文件不存在。Ent schema 自动迁移替代。

- [x] **Step 4: Commit**

```bash
git add internal/data/sql/ docs/sql/
git commit -m "refactor: update SQL DDL for taxonomy rename"
```

---

## Task 14: 前端重命名 — 后端 API 层

**Files:**
- Modify: `web/src/services/index.ts` — agent_category → taxonomy
- Modify: `web/src/features/platform/api.ts` — 全面重命名
- Modify: `web/src/features/platform/types.ts` — 资源名更新

- [x] **Step 1: 更新 services/index.ts**

- `createAgentCategoryServiceClient` → `createTaxonomyServiceClient`
- `createAgentCategoryService` → `createTaxonomyService`
- import 路径 `./kratos/agent_category/v1/index` → `./kratos/taxonomy/v1/index`

⚠️ **实现偏差**: 额外添加了 `createIndustryTaxonomyService` 和 `createIndustryTaxonomyServiceClient`（import 自 `./kratos/industry_taxonomy/v1/index`）。旧的 `./kratos/agent_category/v1/index.ts` 仍存在（死代码，未被 import）

- [x] **Step 2: 更新 features/platform/api.ts**

- `agentCategoryWireToPlatform` → `taxonomyWireToPlatform`
- `mapAgentCategoryTreeNode` → `mapTaxonomyTreeNode`
- `"agent-categories"` → `"taxonomy"`
- `ListAgentCategoryTree` → `ListTaxonomyTree`
- `CreateAgentCategory` → `CreateTaxonomy`
- `GetAgentCategory` → `GetTaxonomy`
- `UpdateAgentCategory` → `UpdateTaxonomy`
- `DeleteAgentCategory` → `DeleteTaxonomy`
- `reorderAgentCategories` → `reorderTaxonomy`
- 所有 `category_position_id` → `taxonomy_position_id`

⚠️ **实现偏差**: api.ts 中存在两种资源名：`taxonomy-nodes`（映射到 IndustryTaxonomyService）和 `taxonomy`（映射到 TaxonomyService），设计文档仅规划了 `taxonomy`

- [x] **Step 3: 更新 features/platform/types.ts**

- `"agent-categories"` → `"taxonomy"` (在 PlatformResourceName 联合类型中)

⚠️ **实现偏差**: types.ts 中 PlatformResourceName 包含 `'taxonomy-nodes' | 'taxonomy'` 两个值

- [x] **Step 4: Commit**

```bash
git add web/src/services/ web/src/features/platform/
git commit -m "refactor: rename agent-categories → taxonomy in frontend API layer"
```

---

## Task 15: 前端重命名 — 组件和页面

**Files:**
- Rename: `web/src/pages/AgentCategoriesPage.vue` → `TaxonomyPage.vue`
- Rename: `web/src/components/agents/AgentCategoryTree.vue` → `TaxonomyTree.vue`
- Rename: `web/src/components/agents/AgentCategoryPositionCard.vue` → `TaxonomyPositionCard.vue`
- Rename: `web/src/components/agents/AgentCategoryPicker.vue` → `TaxonomyPicker.vue`
- Rename: `web/src/components/agents/AgentCategoryFilter.vue` → `TaxonomyFilter.vue`
- Rename: `web/src/features/platform/useAgentCategoriesPage.ts` → `useTaxonomyPage.ts`

- [x] **Step 1: 重命名所有组件文件**

每个文件内部同步更新：
- CSS class 名：`agent-category-*` → `taxonomy-*`
- 组件 import 名和注册名
- `AgentCategoryPicker` → `TaxonomyPicker`
- `AgentCategoryFilter` → `TaxonomyFilter`
- `AgentCategoryTree` → `TaxonomyTree`
- `AgentCategoryPositionCard` → `TaxonomyPositionCard`

⚠️ **实现偏差**: 额外新增了 `TaxonomyIndustryCard.vue`、`TaxonomyTreeNodeHeader.vue`、`TaxonomyNodeHeader.vue`（设计文档未规划）。旧的 `CategoryTreeNodeHeader.vue` 仍存在（死代码）

- [x] **Step 2: 更新 useTaxonomyPage.ts**

- `"agent-categories"` → `"taxonomy"`
- 所有 `AgentCategory` 引用 → `Taxonomy`

- [x] **Step 3: Commit**

```bash
git add web/src/pages/ web/src/components/agents/ web/src/features/platform/
git commit -m "refactor: rename AgentCategory components → Taxonomy"
```

---

## Task 16: 前端重命名 — Store、路由、工具函数

**Files:**
- Modify: `web/src/stores/agents/index.ts`
- Modify: `web/src/stores/platform/index.ts`
- Modify: `web/src/features/agents/api.ts`
- Modify: `web/src/features/agents/types.ts`
- Modify: `web/src/features/agents/wireNormalize.ts`
- Modify: `web/src/features/agents/useAgentsPage.ts`
- Modify: `web/src/features/agents/useAgentSettingsPage.ts`
- Modify: `web/src/components/agents/AgentCreateDialog.vue`
- Modify: `web/src/components/agents/AgentsFiltersCard.vue`
- Modify: `web/src/components/agents/AgentsListSection.vue`
- Modify: `web/src/components/agents/agentTableUi.ts`
- Modify: `web/src/components/agents/agentUi.ts`
- Modify: `web/src/components/chat/ChatEntitySidebar.vue`
- Modify: `web/src/components/teams/teamUtils.ts`
- Modify: `web/src/components/teams/__tests__/teamUtils.spec.ts`
- Modify: `web/src/router/routes.ts`
- Modify: `web/src/config/sideNav.ts`
- Modify: `web/src/components/usage/CommandCenterHero.vue`
- Modify: `web/src/components/agents/AgentsWorkspaceHero.vue`
- Modify: `web/src/features/usage/useOverviewPage.ts`

- [x] **Step 1: 更新 stores**

`stores/agents/index.ts`:
- `"agent-categories"` → `"taxonomy"`
- `industryNodes` computed 中引用更新

`stores/platform/index.ts`:
- `"agent-categories"` → `"taxonomy"`

⚠️ **实现偏差**: Store/Composable/变量名仍用 `category` 前缀：`categoryTree`、`loadCategoryTree`、`selectedCategory`（保留为兼容别名，见 design §13 FL1 建议）

- [x] **Step 2: 更新 agents feature 层**

`features/agents/api.ts`:
- `category_position_id` → `taxonomy_position_id`

`features/agents/types.ts`:
- `category_position_id: string` → `taxonomy_position_id: string`

`features/agents/wireNormalize.ts`:
- `category_position_id` → `taxonomy_position_id`

⚠️ **实现偏差**: wireNormalize.ts 仍保留 `categoryPositionId` 作为内部字段名，通过映射 `taxonomy_position_id: pickStr(w, 'categoryPositionId', 'category_position_id')` 和 `o.categoryPositionId = payload.taxonomy_position_id` 进行兼容

`features/agents/useAgentsPage.ts`:
- `category_position_id` → `taxonomy_position_id`

`features/agents/useAgentSettingsPage.ts`:
- `category_position_id: ""` → `taxonomy_position_id: ""`

- [x] **Step 3: 更新组件引用**

`AgentCreateDialog.vue`:
- `AgentCategoryPicker` → `TaxonomyPicker`
- `category_position_id` → `taxonomy_position_id`

`AgentsFiltersCard.vue`:
- `AgentCategoryFilter` → `TaxonomyFilter`
- CSS class `agent-category-field--toolbar` → `taxonomy-field--toolbar`

⚠️ **实现偏差**: AgentsFiltersCard 仍使用 `categoryTree` prop 名

`AgentsListSection.vue`:
- `agent.category_position_id` → `agent.taxonomy_position_id`

`agentTableUi.ts`:
- `row.category_position_id` → `row.taxonomy_position_id`

`agentUi.ts`:
- `flattenCategoryPositions` → `flattenCategoryPositions`（保留旧名，内部调用 `flattenTaxonomyPositions`）

⚠️ **实现偏差**: agentUi.ts 仍导出 `flattenCategoryPositions` 函数名

`ChatEntitySidebar.vue`:
- `agent.category_position_id` → `agent.taxonomy_position_id`
- `agentCategoryMap` → 已移除

`teamUtils.ts`:
- `agent.category_position_id` → `agent.taxonomy_position_id`

`teamUtils.spec.ts`:
- `resource: "agent-categories"` → `resource: "taxonomy-nodes"`
- `category_position_id: "pos-1"` → `taxonomy_position_id: "pos-1"`

- [x] **Step 4: 更新路由和导航**

`routes.ts`:
- `"settings/agent-categories"` → `"settings/taxonomy"`
- `AgentCategoriesPage` → `TaxonomyPage`

`sideNav.ts`:
- `/settings/agent-categories` → `/settings/taxonomy`

`CommandCenterHero.vue`:
- `/settings/agent-categories` → `/settings/taxonomy`

`AgentsWorkspaceHero.vue`:
- `/settings/agent-categories` → `/settings/taxonomy`

`useOverviewPage.ts`:
- `"agent-categories"` → `"taxonomy"`

- [x] **Step 5: Commit**

```bash
git add web/src/
git commit -m "refactor: rename AgentCategory → Taxonomy in frontend stores/routes/utils"
```

---

## Task 17: 前端重命名 — CSS/Sass

**Files:**
- Modify: `web/src/css/theme/_entity-pages.sass`
- Modify: `web/src/css/theme/_form-layout.sass`

- [x] **Step 1: 更新 _entity-pages.sass**

所有 CSS class 名：
- `.agent-categories-page` → `.taxonomy-page`
- `.agent-category-filter` → `.taxonomy-filter`
- `.agent-category-picker` → `.taxonomy-picker`
- `.agent-category-field` → `.taxonomy-field`

- [x] **Step 2: 更新 _form-layout.sass**

- `.agent-category-field` → `.taxonomy-field`

- [x] **Step 3: Commit**

```bash
git add web/src/css/
git commit -m "refactor: rename AgentCategory CSS classes → Taxonomy"
```

---

## Task 18: 前端行业市场页迁移

**Files:**
- Modify: `web/src/pages/industries/IndustryMarketPage.vue`
- Modify: `web/src/pages/industries/IndustryDetailPage.vue`
- Modify: `web/src/components/industries/IndustryPositionPicker.vue`
- Modify: `web/src/features/industries/api.ts`

- [x] **Step 1: 更新 features/industries/api.ts**

将行业市场页的 API 调用从 `IndustryService` 切换到 `TaxonomyService`：
- `listIndustries()` → 调用 `GET /v1/taxonomy?level=industry`
- `listDepartments(industryKey)` → 调用 `GET /v1/taxonomy?level=department&parent_id={id}`
- `listPositions(departmentKey)` → 调用 `GET /v1/taxonomy?level=position&parent_id={id}`
- `getPositionPrompt(positionKey)` → 从 taxonomy 节点数据中提取描述信息
- `listPositionVariants(positionKey)` → 暂返回默认 general 变体

✅ 已完成：api.ts 已迁移到 TaxonomyService API，使用 createTaxonomyService() + 内存缓存

- [x] **Step 2: 更新 IndustryMarketPage.vue**

使用新的 API 调用，适配 taxonomy API 响应格式。

✅ 已完成：函数签名不变，无需修改

- [x] **Step 3: 更新 IndustryDetailPage.vue**

使用新的 API 调用，适配 taxonomy API 响应格式。

✅ 已完成：industry.icon 为空时 fallback 到行业名首字母

- [x] **Step 4: 更新 IndustryPositionPicker.vue**

使用新的 API 调用。

✅ 已完成：接收 props 类型不变，无需修改

- [x] **Step 5: Commit**

```bash
git add web/src/pages/industries/ web/src/components/industries/ web/src/features/industries/
git commit -m "refactor: migrate Industry pages to Taxonomy API"
```

---

## Task 19: 全量编译验证

- [x] **Step 1: 后端全量验证**

```bash
cd f:\aranea-agents
make api && make wire && make build && make test && make lint
```

Expected: 全部通过

- [x] **Step 2: 前端全量验证**

```bash
cd f:\aranea-agents\web
pnpm lint && pnpm build
```

Expected: 全部通过

- [x] **Step 3: 修复任何编译/lint 错误**

- [x] **Step 4: Commit**

```bash
git add -A
git commit -m "fix: resolve compilation issues after Taxonomy rename"
```

---

## Task 20: 数据库迁移脚本

**Files:**
- Create: `docs/sql/migrations/rename_agent_category_to_taxonomy.sql`

- [x] **Step 1: 编写迁移 SQL**

```sql
-- 重命名表
ALTER TABLE agent_category_nodes RENAME TO industry_taxonomy;

-- 重命名列
ALTER TABLE industry_taxonomy RENAME COLUMN category_key TO taxonomy_key;

-- 重命名索引
DROP INDEX IF EXISTS idx_agent_category_parent;
DROP INDEX IF EXISTS idx_agent_category_level;
CREATE INDEX idx_taxonomy_parent ON industry_taxonomy(parent_id, sort_order);
CREATE INDEX idx_taxonomy_level ON industry_taxonomy(level, sort_order);

-- 重命名 agents 表的列
ALTER TABLE agents RENAME COLUMN category_position_id TO taxonomy_position_id;
```

⚠️ **不再需要**: `docs/sql/migrations/` 目录不存在。Ent schema 的自动迁移功能替代了手动 SQL 迁移。

- [x] **Step 2: 更新 memory_chain.sql 中的建表语句**

确保新安装时使用新表名和列名。

⚠️ **不再需要**: memory_chain.sql 中无 industry_taxonomy 表 DDL。Ent schema 自动迁移替代。

- [x] **Step 3: Commit**

```bash
git add docs/sql/
git commit -m "feat: add DB migration script for taxonomy rename"
```

---

## Task 21: 全局 grep 验证 — 确保无遗漏

- [x] **Step 1: 后端 grep 验证**

```bash
cd f:\aranea-agents
grep -rn "AgentCategory" internal/ cmd/ --include="*.go"
grep -rn "agent_category" internal/ cmd/ --include="*.go"
grep -rn "category_position_id" internal/ cmd/ --include="*.go"
grep -rn "agent-category" internal/ cmd/ --include="*.go"
grep -rn "IndustryUsecase\|DepartmentUsecase\|PositionUsecase" internal/ cmd/ --include="*.go"
```

Expected: 0 matches（除了注释中的历史说明）

✅ **验证结果**: 后端 Go 代码中 `AgentCategory`/`agent_category`/`IndustryUsecase`/`DepartmentUsecase`/`PositionUsecase` 均为 0 残留。`category_position_id` 仅存在于 `api/kratos/agent/v1/agent.proto`（proto 字段未重命名）

- [x] **Step 2: 前端 grep 验证**

```bash
cd f:\aranea-agents\web
grep -rn "AgentCategory" src/ --include="*.vue" --include="*.ts"
grep -rn "agent-category" src/ --include="*.vue" --include="*.ts" --include="*.sass"
grep -rn "category_position_id" src/ --include="*.vue" --include="*.ts"
grep -rn "agent-categories" src/ --include="*.vue" --include="*.ts"
```

Expected: 0 matches（除了注释中的历史说明）

✅ **验证结果**: 前端代码中 `AgentCategory`/`agent-category`/`agent-categories` 均为 0 残留。`category_position_id` 仅在 `wireNormalize.ts` 中作为兼容映射存在（`pickStr(w, 'categoryPositionId', 'category_position_id')`）

- [x] **Step 3: 修复任何遗漏引用**

- [x] **Step 4: 最终 Commit**

```bash
git add -A
git commit -m "chore: final cleanup after Taxonomy rename - no remaining AgentCategory references"
```

---

## 实现偏差汇总（2026-06-05 复查）

### 🔴 关键偏差（影响功能正确性）

| ID | 描述 | 影响 | 涉及文件 |
|----|------|------|----------|
| D1 | `features/industries/api.ts` 仍调用 `/v1/industries` 旧 API | 行业市场页 API 调用会 404 | `web/src/features/industries/api.ts` |
| D2 | `selfmedia/agents.yaml` 中 4 个 variant 仍使用连字符（`platform-adapt`/`data-driven`/`geography-history`/`magic-system`） | 不匹配 `variantSafeRe` 正则，API 返回 400 | `internal/scenario/selfmedia/agents.yaml` |
| D3 | `agent.proto` 仍使用 `category_position_id` 字段名 | Proto 与 Go/Biz 层命名不一致 | `api/kratos/agent/v1/agent.proto` |
| D23 | softwaredev agents.yaml 仅 10 个 Agent（原始），设计要求 ~82 个；taxonomy.yaml 岗位定义和 prompt 文件已补全，但 agents.yaml Agent 条目未添加 | P1-P3 岗位的 Agent 无法实例化和运行 | `internal/scenario/softwaredev/agents.yaml` |

### 🟡 设计偏差（与 design.md 描述不符但不影响功能）

| ID | 描述 | 涉及文件 |
|----|------|----------|
| D4 | 额外创建了 `IndustryTaxonomyService`（proto + service + 注册），设计文档仅规划 `TaxonomyService` | `api/kratos/industry_taxonomy/`, `internal/service/industry_taxonomy.go` |
| D5 | 前端存在 `taxonomy-nodes` 和 `taxonomy` 两种资源名，设计仅规划 `taxonomy` | `web/src/features/platform/api.ts`, `types.ts` |
| D6 | `SeedTaxonomyV1 = 20260529`（设计要求 20260701），`SeedCategoriesV2` 仍保留 | `internal/data/seed_versions.go` |
| D7 | `seed_builtin_taxonomy.go` 使用 Raw SQL 而非 Ent ORM | `internal/data/seed_builtin_taxonomy.go` |
| D8 | `taxonomy.yaml` 仅基础格式，未实现 responsibilities/skills_required/seniority_level/variants | `internal/scenario/taxonomy.yaml` |
| D9 | 部门 key 未对齐 design §5.3（仍用 `risk_compliance`/`investment_research`/`financial_engineering`/`wealth_management`/`derivatives`） | `internal/scenario/taxonomy.yaml` |
| D10 | selfmedia 的 `content_creation` 未拆分，`growth_monetization` 未改为 `distribution` | `internal/scenario/taxonomy.yaml` |
| D11 | Store/Composable/变量名仍用 `category` 前缀（`categoryTree`/`loadCategoryTree`/`selectedCategory`） | 多个前端文件 |
| D12 | `categories_loader.go` 仍存在（死代码） | `internal/scenario/loader/categories_loader.go` |
| D13 | `categories.yaml` 仍存在（未删除旧文件） | `internal/scenario/categories.yaml` |
| D14 | `CategoryTreeNodeHeader.vue` 仍存在（死代码） | `web/src/components/agents/CategoryTreeNodeHeader.vue` |
| D15 | ~~`agent_category/v1/index.ts` 仍存在（死代码）~~ **已清理**：`web/src/services/kratos/agent_category/` 目录已不存在 | `web/src/services/kratos/agent_category/v1/index.ts` |
| D16 | `wireNormalize.ts` 仍映射 `categoryPositionId` ↔ `taxonomy_position_id` | `web/src/features/agents/wireNormalize.ts` |
| D17 | `categoryTreeUtils.ts` 仍存在（作为兼容 re-export 层） | `web/src/features/platform/categoryTreeUtils.ts` |
| D18 | `memory_chain.sql` 无 `industry_taxonomy` 表 DDL | `internal/data/sql/memory_chain.sql` |
| D19 | `docs/sql/` 目录不存在，迁移 SQL 文件未创建 | N/A |
| D20 | `agentUi.ts` 仍导出 `flattenCategoryPositions` 函数名 | `web/src/components/agents/agentUi.ts` |
| D21 | stockx 合并未完全执行：`trading_coordinator/critic` 和 `report_writer/chart` variant 未添加到 `finance/agents.yaml`；`team-research-pipeline` 和 `team-deep-dive-critic` 团队未添加；⚠️ 已纠正：5 个已有 Team 成员引用已统一为 finance agent_key；⚠️ 额外新增 3 个设计未规划的 team（team-quant-strategy-research/team-investment-committee/team-risk-monitoring） | `internal/scenario/finance/agents.yaml` |
| D22 | finance 部门 key 未对齐（仍用 `risk_compliance`/`investment_research`/`financial_engineering`/`wealth_management`/`derivatives`） | `internal/scenario/taxonomy.yaml` |
| D24 | softwaredev skills 有 11 个（设计规划 6 个），额外 5 个 Skill 未在设计文档中规划：ddd-tactical/code-review-checklist/clean-arch/go-best-practices/ue5-gas | `internal/scenario/softwaredev/skills/` |
| D25 | finance/agents.yaml 实际有 8 个 team（设计规划 7 个：5 已有 + 2 新增），其中 3 个为设计未规划的额外 team | `internal/scenario/finance/agents.yaml` |
