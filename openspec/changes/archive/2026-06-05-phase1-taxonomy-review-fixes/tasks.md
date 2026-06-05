## Non-Goals

- 不合并 IndustryTaxonomyService 与 TaxonomyService（评估为后续独立变更）
- 不修改 `variantSafeRe` 正则表达式
- 不新增行业或岗位
- 不改变前端组件架构

## 1. 阻断项修复 — selfmedia variant 连字符

- [x] 1.1 修改 `internal/scenario/selfmedia/agents.yaml`：`data-driven` → `data_driven`，同步更新 agent_key
- [x] 1.2 修改 `internal/scenario/selfmedia/agents.yaml`：`geography-history` → `geography_history`，同步更新 agent_key
- [x] 1.3 修改 `internal/scenario/selfmedia/agents.yaml`：`magic-system` → `magic_system`，同步更新 agent_key
- [x] 1.4 修改 `internal/scenario/selfmedia/agents.yaml`：`platform-adapt` → `platform_adapt`，同步更新 agent_key
- [x] 1.5 同步重命名 `internal/scenario/selfmedia/prompts/positions/` 下含连字符的 prompt 子目录/文件名
- [x] 1.6 验证：grep selfmedia/agents.yaml 无连字符 variant

## 2. 阻断项修复 — agent.proto 字段重命名

- [x] 2.1 修改 `api/kratos/agent/v1/agent.proto`：`category_position_id` → `taxonomy_position_id`（Agent message + CreateAgentRequest message）
- [x] 2.2 运行 `make api` 重新生成 Proto 代码
- [x] 2.3 更新 `internal/service/agent.go`：`pb.GetCategoryPositionId()` → `pb.GetTaxonomyPositionId()`，`CategoryPositionId:` → `TaxonomyPositionId:`
- [x] 2.4 更新 `internal/service/service_agent_mapping_test.go`：映射字段名同步
- [x] 2.5 更新 `web/src/features/agents/wireNormalize.ts`：`categoryPositionId` → `taxonomyPositionId`，移除 `category_position_id` fallback
- [x] 2.6 更新 `web/src/features/agents/api.ts`：`categoryPositionId` → `taxonomyPositionId`
- [x] 2.7 验证：`go build ./cmd/admin` + `npx quasar build`

## 3. 死代码文件清理

- [x] 3.1 删除 `web/src/services/kratos/agent_category/` 目录
- [x] 3.2 删除 `web/src/components/agents/CategoryTreeNodeHeader.vue`
- [x] 3.3 删除 `web/src/features/platform/categoryTreeUtils.ts`
- [x] 3.4 删除 `internal/scenario/categories.yaml`
- [x] 3.5 验证：确认无文件引用被删除的模块

## 4. IndustryTaxonomyService gRPC 注册

- [x] 4.1 在 `internal/server/grpc.go` 添加 `industrytaxonomyv1.RegisterIndustryTaxonomyServiceServer(srv, s.IndustryTaxonomy)`
- [x] 4.2 添加 `industrytaxonomyv1` import
- [x] 4.3 验证：`go build ./cmd/admin`

## 5. 前端变量名统一 — Store 层

- [x] 5.1 修改 `web/src/stores/platform/index.ts`：`categoryTree` → `taxonomyTree`，`loadCategoryTree` → `loadTaxonomyTree`
- [x] 5.2 修改 `web/src/stores/agents/index.ts`：`selectedCategory` → `selectedTaxonomy`，`categoryTree` → `taxonomyTree`，`categoryLabel` → `taxonomyLabel`，`categoryPositionOptions` → `taxonomyPositionOptions`
- [x] 5.3 更新 Store 测试文件 `web/src/stores/__tests__/agents.store.spec.ts` 中对应变量名

## 6. 前端变量名统一 — Composable/Feature 层

- [x] 6.1 修改 `web/src/features/agents/useAgentsPage.ts`：`selectedCategory` → `selectedTaxonomy`，`categoryTree` → `taxonomyTree`，`categoryLabel` → `taxonomyLabel`
- [x] 6.2 修改 `web/src/features/teams/useTeamsPage.ts`：`categoryTree` → `taxonomyTree`，`loadCategoryTree` → `loadTaxonomyTree`
- [x] 6.3 修改 `web/src/features/platform/useTaxonomyPage.ts`：`loadCategoryTree` → `loadTaxonomyTree`，`categoryTree` → `taxonomyTree`
- [x] 6.4 修改 `web/src/features/chat/composables/useChatEntityNav.ts`：`categoryTree` → `taxonomyTree`，`loadCategoryTree` → `loadTaxonomyTree`
- [x] 6.5 修改 `web/src/features/chat/composables/useChatWorkspace.ts`：`loadCategoryTree` → `loadTaxonomyTree`，`categoryTree` → `taxonomyTree`

## 7. 前端变量名统一 — Component/Page 层

- [x] 7.1 修改 `web/src/pages/AgentsPage.vue`：`selectedCategory` → `selectedTaxonomy`，`categoryTree` → `taxonomyTree`，`categoryLabel` → `taxonomyLabel`
- [x] 7.2 修改 `web/src/components/agents/AgentsFiltersCard.vue`：`selectedCategory` → `selectedTaxonomy`，`categoryTree` → `taxonomyTree`
- [x] 7.3 修改 `web/src/components/agents/AgentCreateDialog.vue`：`categoryTree` → `taxonomyTree`
- [x] 7.4 修改 `web/src/components/agents/AgentCard.vue`：`categoryLabel` → `taxonomyLabel`
- [x] 7.5 修改 `web/src/components/agents/agentUi.ts`：`flattenCategoryPositions` → `flattenTaxonomyPositions`
- [x] 7.6 修改 `web/src/components/agents/agentTableUi.ts`：`categoryLabel` → `taxonomyLabel`
- [x] 7.7 修改 `web/src/components/teams/teamUtils.ts`：`categoryTree` 参数名 → `taxonomyTree`
- [x] 7.8 修改 `web/src/components/teams/TeamEditorDialog.vue`：`category_industry_id` → `taxonomy_industry_id`
- [x] 7.9 修改 `web/src/features/teams/types.ts`：`category_industry_id` → `taxonomy_industry_id`
- [x] 7.10 修改 `web/src/features/teams/api.ts`：`category_industry_id` → `taxonomy_industry_id`
- [x] 7.11 清理 `web/src/features/platform/taxonomyTreeUtils.ts`：移除 `CategoryLevel`/`CategoryQTreeNode` legacy alias 导出
- [x] 7.12 修改 `web/src/components/agents/TaxonomyPicker.vue`：`CategoryLevel` → `TaxonomyLevel`
- [x] 7.13 更新测试文件 `web/src/components/teams/__tests__/teamUtils.spec.ts` 中对应变量名

## 8. taxonomy.yaml 丰富字段补全

- [x] 8.1 为 taxonomy.yaml 所有部门添加 `responsibilities` 字段
- [x] 8.2 为 taxonomy.yaml 所有岗位添加 `seniority_level`、`skills_required`、`responsibilities`、`variants` 字段
- [x] 8.3 更新 `internal/scenario/loader/taxonomy_loader.go`：支持解析新字段
- [x] 8.4 更新 seed 逻辑：将丰富字段写入 `metadata_json`
- [x] 8.5 验证：应用启动后 taxonomy 节点的 metadata_json 非空

## 9. selfmedia 部门 key 对齐

- [x] 9.1 拆分 `content_creation` 为 `fiction_writing` + `content_graphic`，迁移岗位定义
- [x] 9.2 重命名 `growth_monetization` → `distribution`
- [x] 9.3 更新 selfmedia/agents.yaml 中引用旧部门 key 的 agent position_key
- [x] 9.4 同步更新 selfmedia/prompts/positions/ 目录结构
- [x] 9.5 验证：taxonomy.yaml selfmedia 部门 key 与设计 §5.3 一致

## 10. finance/agents.yaml 缺失补全

- [x] 10.1 添加 `trading_coordinator/critic` variant（agent_key: `trading-coordinator-critic`）
- [x] 10.2 添加 `report_writer/chart` variant（agent_key: `report-writer-chart`）
- [x] 10.3 添加 `team-research-pipeline` 团队定义
- [x] 10.4 添加 `team-deep-dive-critic` 团队定义
- [x] 10.5 验证：finance/agents.yaml 所有 agent_key 唯一，team 成员引用正确

## 11. softwaredev/agents.yaml P1 Agent 补全

- [x] 11.1 添加 backend 岗位 Agent（Java 3 + Python 2 + Rust 2 + C++ 2 + DBA 2 = 11 个）
- [x] 11.2 添加 frontend 岗位 Agent（React 3 + TypeScript 2 + 性能 2 + UI/UX 1 = 8 个）
- [x] 11.3 添加 gamedev 岗位 Agent（UE 游戏逻辑 2 + UE 图形 2 + 服务端 2 + TA 1 + 策划 1 = 8 个）
- [x] 11.4 验证：softwaredev agents.yaml Agent 总数 ≥ 37

## 12. softwaredev/agents.yaml P2+P3 Agent 补全

- [x] 12.1 添加 P2 批次 Agent（devops 10 + architecture 7 + qa 6 = 23 个）
- [x] 12.2 添加 P3 批次 Agent（mobiledev 9 + dataeng 5 + security 4 + productpm 4 = 22 个）
- [x] 12.3 验证：softwaredev agents.yaml Agent 总数 ≥ 82

## 13. 全量验证

- [x] 13.1 后端：`make api && make wire && make build && make test`
- [x] 13.2 前端：`cd web && pnpm lint && pnpm build`
- [x] 13.3 后端 grep 验证：`AgentCategory`/`agent_category`/`category_position_id` 0 残留
- [x] 13.4 前端 grep 验证：`AgentCategory`/`agent-category`/`categoryTree`/`loadCategoryTree`/`selectedCategory` 0 残留
