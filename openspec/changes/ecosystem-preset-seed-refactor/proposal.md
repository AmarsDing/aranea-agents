## Why

当前种子入库系统存在三层问题：**架构混乱**（3 条独立管道并存、版本门控不一致）、**语义不清**（系统内置与系统附带生态混为一谈）、**用户体验缺失**（行业生态自动加载、无前端控制、分类树展示不合理）。需要将种子数据分为"系统内置"（启动强制加载、不可删除）和"系统附带生态"（用户按需加载、可编辑可删除）两层，统一管道，并在前端提供一键加载入口和树形分类管理。

## What Changes

- **BREAKING**: Agent Kind 枚举精简：删除 `system`（无实际使用）和 `industry_template`（语义模糊），新增 `ecosystem_preset`（系统附带生态种子）
- 新增 `system_settings.ecosystem_loaded` 字段，记录附带生态加载状态（支持部分加载追踪）
- 新增 API `POST /api/v1/admin/ecosystem/preset/load`，用户在系统设置页一键加载附带生态
- 新增 API `POST /api/v1/admin/ecosystem/preset/unload`，卸载行业附带生态（删除该行业所有 Agent/Team + 分类节点），前端弹框确认
- Team 表新增 `kind` 字段（枚举值与 Agent Kind 对齐），统一 Agent/Team 权限分类体系
- 删除旧版行业种子管道（`SeedBuiltinIndustryAgents`、Lazy Seeder 行业 Pack），统一走 Pack 引擎 + API 触发
- 行业分类管理页改造为树形折叠 + 岗位卡片混合布局，支持拖拽排序
- 前端 Agent/Team 列表根据 Kind 显示"内置"/"预设"徽章，`system_builtin` 隐藏删除按钮
- `ecosystem_preset` 类型的 Agent/Team 加载后与 `user` 同权（可编辑、可删除）

## Capabilities

### New Capabilities
- `ecosystem-preset-load`: 系统附带生态加载/卸载机制（后端 API + 前端按钮 + 加载状态管理 + 卸载确认）
- `taxonomy-tree-card-layout`: 行业分类树形折叠 + 岗位卡片混合布局（前端组件改造）

### Modified Capabilities
- `agent-kind-enum`: Agent/Team Kind 枚举精简（删除 system/industry_template，新增 ecosystem_preset），Team 新增 kind 字段，含数据迁移
- `seed-pipeline`: 种子管道统一（删除旧版行业种子管道，Pack 引擎支持 kind 覆盖）

## Impact

- **后端 biz 层**: 新增 `EcosystemUsecase`；修改 `pack.Importer` 支持 kind 参数覆盖；删除 `SeedBuiltinIndustryAgents` 旧版种子
- **后端 data 层**: 新增 `ecosystem_loaded` DDL 迁移；修改 `SeedPackIndustry` 增加 kind 参数；删除 Lazy Seeder 行业 Pack 注册；`seed_version.go` 清理旧常量
- **后端 service 层**: 新增 `EcosystemService` + HTTP 路由
- **后端 Ent schema**: `agent.go` Kind 枚举变更；`team.go` 新增 kind 字段；`system_setting.go` 新增字段
- **前端 pages**: `SystemSettingsPage.vue` 新增附带生态区块；`TaxonomyPage.vue` 改造树形布局
- **前端 components**: 新增 `TaxonomyDepartmentNode.vue`；改造 `TaxonomyTree.vue`；Agent/Team 卡片增加 Kind 徽章
- **前端 stores/api**: `system-settings` store 新增加载状态和 action；新增 `ecosystem` API
- **数据库**: 需要数据迁移（`system` → `system_builtin`，`industry_template` → `ecosystem_preset`）

## Non-goals

- 不做行业分类的 CRUD API 重构（复用现有 Taxonomy API）
- 不做 Agent/Team 删除级联逻辑（卸载行业时除外，卸载为显式批量删除）
- 不做附带生态的增量更新（加载一次后不再自动更新，后续版本通过"重新加载"按钮支持）
- 不做商城/认证 Agent 的 Kind 相关改动（本次只涉及内置和附带生态）
- 不做拖拽排序的后端 API 新增（复用现有 `reorderTaxonomy` API）
