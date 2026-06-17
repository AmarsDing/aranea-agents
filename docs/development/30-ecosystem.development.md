# Ecosystem 生态 — 开发计划

> **版本**：2026-06-17 | **状态**：🟡 Phase 1 MVP 已落地 + Preset 架构改造已落地（L2 管道阻塞中）
> **需求**：[30-ecosystem.md](./30-ecosystem.md) · **设计**：[30-ecosystem.design.md](./30-ecosystem.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：—

---

## 1. 模块定位

Ecosystem 生态：Agent/Skill/Team 的市场与分享平台，支持发布、发现、安装和评价；以及附带生态行业预设的加载/卸载管理。

**代码锚点**：

| 层 | 路径 |
|----|------|
| Proto | `api/kratos/ecosystem/v1/ecosystem.proto` |
| Biz | `internal/biz/ecosystem/ecosystem.go` · `internal/biz/ecosystem.go`（重导出）· `internal/biz/ecosystem_preset.go` |
| Data | `internal/data/ecosystem.go` · `internal/data/ecosystem_preset.go` · `internal/data/seed_pack.go` |
| Service | `internal/service/ecosystem.go` · `internal/service/ecosystem_preset.go` |
| Server | `internal/server/http.go`（路由注册） |
| Wire | `cmd/admin/wire.go`（`provideEcosystemPresetScenarioDir` + `wire.Bind`） |
| Schema | `internal/data/ent/schema/agent.go`（kind）· `internal/data/ent/schema/team.go`（kind）· `internal/data/ent/schema/system_setting.go`（ecosystem_loaded） |
| DDL | `internal/data/sql/ecosystem_product.sql` · `internal/data/sql/migrations/20260718_ecosystem_preset_schema.sql` · `internal/data/ddl_migration_registry.go` |
| 前端 | `web/src/pages/EcosystemPage.vue`（路由 `/shop`）· `web/src/features/ecosystem/` · `web/src/stores/ecosystem/` · `web/src/features/system-settings/` · `web/src/stores/system-settings/` · `web/src/components/agents/KindBadge.vue` |
| 场景数据 | `internal/scenario/packs/builtin-templates/` · `internal/scenario/packs/finance/` · `internal/scenario/{finance,selfmedia,softwaredev}/agents.yaml` |

---

## 2. 现状评估

### 2.1 生态市场（Marketplace）

| 项 | 状态 | 证据 |
|----|------|------|
| Proto 定义 | ✅ | `ecosystem.proto` — 5 RPC（List/Get/Publish/Install/Uninstall），无 RateProduct |
| Biz 层 | ✅ | `biz/ecosystem/` 子包，Usecase + Repo 接口，构造注入 `loggateway.Logger` |
| Service 层 | ✅ | `service/ecosystem.go` 实现 gRPC 服务，`GetProduct` 返回 `NotFound` 错误码 |
| Data 层 | ✅ | `data/ecosystem.go`，原生 SQL 操作 `ecosystem_products` + `ecosystem_installs`，读写分离 + 事务包裹 install |
| 数据库表 | ✅ | DDL `internal/data/sql/ecosystem_product.sql` + 迁移 V20260703（`ddlEcosystemSchema`） |
| Wire 注入 | ✅ | `data/biz/service.ProviderSet` + `cmd/admin/wire.go` 已注册 |
| HTTP 路由 | ✅ | `internal/server/http.go:135` 注册 `ecosystemv1.RegisterEcosystemServiceHTTPServer` |
| 前端页面 | ✅ | `EcosystemPage.vue`（技术预览状态）+ `features/ecosystem/` + Pinia store |
| 前端类型 | ⚠️ | `features/ecosystem/types.ts` 仅含 8 字段子集，未覆盖 Proto 全部字段（rating/price_cents/config_json 等） |
| 安装逻辑 | ⚠️ | 仅 `RecordInstall` 记录安装关系 + 自增 `install_count`，未解析 `config_json` 创建实际 Agent/Team/Skill |
| 评价/评分 | ❌ | 无 RateProduct RPC，无 `marketplace_reviews` 表 |
| 商品版本 | ❌ | 无 `marketplace_product_versions` 表 |
| 交易/订单 | ❌ | 无 `marketplace_orders` 表 |

### 2.2 附带生态（Preset Seed Refactor）

| 项 | 状态 | 证据 |
|----|------|------|
| Kind 枚举统一 | ✅ | Agent/Team Kind = `user \| system_builtin \| ecosystem_preset \| marketplace \| certified`，Ent Schema `agent.go`/`team.go` |
| Team.kind 字段 | ✅ | Ent schema `team.go:39` + DDL 迁移 V20260718 |
| ecosystem_loaded 存储 | ✅ | `system_settings.ecosystem_loaded` TEXT 列，Ent Schema `system_setting.go:47` |
| Preset Load/Unload/Status Biz | ✅ | `biz/ecosystem_preset.go`，含 `sync.Mutex` 防并发 |
| Preset Service | ✅ | `service/ecosystem_preset.go`，3 HTTP handler |
| Preset Data | ✅ | `data/ecosystem_preset.go`，含级联删除（taxonomy → agents → teams）+ 跨行业 Team 成员清理 |
| HTTP 路由 | ✅ | `internal/server/http.go:192-194` 注册 3 端点 |
| Wire 注入 | ✅ | `wire.Bind(EcosystemPresetRepo)` + `wire.Bind(PackSeeder)` + `provideEcosystemPresetScenarioDir` |
| 前端 Preset 管理 | ✅ | `features/system-settings/useEcosystemPreset.ts` + `stores/system-settings/index.ts` |
| KindBadge 组件 | ✅ | `components/agents/KindBadge.vue`，4 种 Kind 徽章 |
| system_builtin 删除保护（后端） | ✅ | `biz/agent_usecase.go:579` + `biz/team_usecase.go:528` 返回 `Forbidden` |
| system_builtin 删除保护（前端） | ✅ | `components/teams/TeamCard.vue` 隐藏删除/复制按钮；`ChatEntitySidebar.vue` 过滤 system_builtin |
| SeedPackBuiltinTemplates | ✅ | `data/seed_pack.go`，使用 Pack 引擎 + `WithKindOverride("ecosystem_preset")`，7 agents + 2 teams + 6 graphs |
| SeedPackIndustry 实现 | ❌ | `data/seed_pack.go:125` — `pack.ConvertCompanySpecToPack` 未实现，返回 `BadRequest` 错误，L2 管道阻塞 |
| finance 场景数据 | ✅ | `internal/scenario/packs/finance/manifest.yaml`（38 agents + 8 teams）+ `internal/scenario/finance/agents.yaml`（完整） |
| selfmedia 场景数据 | ✅ | `internal/scenario/selfmedia/agents.yaml`（40+ agents + 6 teams 完整） |
| softwaredev 场景数据 | ✅ | `internal/scenario/softwaredev/agents.yaml`（80+ agents + 8 teams 完整） |
| 行业分类树形布局 | ❌ | 前端未改造为树形折叠布局（FR-15/FR-16） |
| builtin-templates Kind 标记 | ⚠️ | 当前 `WithKindOverride("ecosystem_preset")`，按 L1 定义应为 `system_builtin`，待确认 |

### 2.3 数据迁移

| 迁移 | 状态 | 证据 |
|------|------|------|
| V20260703 `ecosystem_schema` | ✅ | `ddlEcosystemSchema` → `EnsureEcosystemSchema`，创建 2 表 + 2 索引 |
| V20260718 `ecosystem_preset_schema` DDL | ✅ | `sql/migrations/20260718_ecosystem_preset_schema.sql` — `system_settings.ecosystem_loaded` + `teams.kind` |
| V20260718 数据迁移 | ✅ | `ddlEcosystemPresetDataMigration` — agents.kind system→system_builtin、industry_template→ecosystem_preset；teams.kind source=imported→ecosystem_preset |

---

## 3. 差距与优化

1. **P0 阻塞**：`SeedPackIndustry` 因 `pack.ConvertCompanySpecToPack` 未实现而返回错误，L2 行业 Pack 加载管道完全不可用。调用 `POST /api/v1/admin/ecosystem/preset/load` 会返回 400。
2. **P1**：Marketplace 安装逻辑为桩实现，安装商品不会创建实际资源（不解析 `config_json`）
3. **P1**：builtin-templates Pack 当前使用 `ecosystem_preset` 而非 `system_builtin`，与 L1 层级定义不一致，需确认是否为设计意图
4. **P2**：行业分类前端未改造为树形折叠布局（FR-15/FR-16）
5. **P2**：前端 `EcosystemProduct` 类型为简化子集，缺少 rating/price_cents/config_json 等字段
6. **P3**：评价/评分系统未实现
7. **P3**：商品版本管理和兼容性检查未实现
8. **P3**：交易/订单系统未实现

---

## 4. 开发阶段

### 子模块 A：生态市场（Marketplace）

| 阶段 | 工作 | 状态 |
|------|------|------|
| Phase 1 | 基础框架 — Proto/Biz/Service/Data/Wire/前端页面 | ✅ 已完成 |
| Phase 2 | 安装逻辑补全 — 解析 config_json 创建实际 Agent/Team/Skill | ❌ 待开发 |
| Phase 3 | 评价/评分系统 — RateProduct RPC + reviews 表 | ❌ 待开发 |
| Phase 4 | 版本管理 + 兼容性检查 — product_versions 表 | ❌ 待开发 |
| Phase 5 | 交易商业化 — orders 表 + 支付集成 | ❌ 待开发 |

### 子模块 B：附带生态（Preset Seed Refactor）

| 阶段 | 工作 | 状态 |
|------|------|------|
| Phase 1 | 架构改造 — Kind 统一/Preset API/Store/KindBadge/删除保护 | ✅ 已完成 |
| Phase 2 | L2 管道打通 — 实现 `pack.ConvertCompanySpecToPack` | ❌ 阻塞中 |
| Phase 3 | 行业 Pack 数据验证 — finance/selfmedia/softwaredev 加载验证 | ❌ 待开发（依赖 Phase 2） |
| Phase 4 | 前端树形布局 — 行业分类管理页改造（FR-15/FR-16） | ❌ 待开发 |
| Phase 5 | builtin-templates Kind 标记确认与修正 | ❌ 待开发 |

---

## 5. 任务清单

### 子模块 A：生态市场

| # | 任务 | 优先级 | 状态 |
|---|------|--------|------|
| A1 | Proto 定义（5 RPC） | P2 | ✅ |
| A2 | Biz/Service/Data 层 | P2 | ✅ |
| A3 | 前端 EcosystemPage | P2 | ✅ |
| A4 | Wire 注入 + HTTP 路由注册 | P2 | ✅ |
| A5 | 安装逻辑补全（解析 config_json 创建资源） | P1 | ❌ |
| A6 | 前端类型补全（对齐 Proto 全字段） | P2 | ❌ |
| A7 | 评价/评分系统 | P3 | ❌ |
| A8 | 商品版本管理 | P3 | ❌ |
| A9 | 交易/订单系统 | P3 | ❌ |

### 子模块 B：附带生态

| # | 任务 | 优先级 | 状态 |
|---|------|--------|------|
| B1 | Agent/Team Kind 枚举统一 | P1 | ✅ |
| B2 | ecosystem_loaded 存储 | P1 | ✅ |
| B3 | Preset Load/Unload/Status API（Biz+Service+Data） | P1 | ✅ |
| B4 | 前端 Preset 管理（system-settings 集成） | P1 | ✅ |
| B5 | KindBadge 组件 | P2 | ✅ |
| B6 | system_builtin 删除保护（后端 Forbidden + 前端隐藏按钮） | P1 | ✅ |
| B7 | SeedPackBuiltinTemplates + WithKindOverride | P1 | ✅ |
| B8 | SeedPackIndustry 实现（`pack.ConvertCompanySpecToPack`） | P0 | ❌ 阻塞中 |
| B9 | finance Pack 加载验证 | P1 | ❌ 依赖 B8 |
| B10 | selfmedia Pack 加载验证 | P1 | ❌ 依赖 B8 |
| B11 | softwaredev Pack 加载验证 | P1 | ❌ 依赖 B8 |
| B12 | builtin-templates Kind 标记确认（`ecosystem_preset` vs `system_builtin`） | P2 | ❌ |
| B13 | 行业分类树形布局（FR-15/FR-16） | P2 | ❌ |

---

## 6. 验收标准

### 子模块 A：生态市场

- [x] 用户可发布 Agent/Skill/Team 到市场
- [x] 用户可浏览和搜索市场商品
- [x] 商品安装/卸载 API 可用（记录安装关系）
- [ ] 安装商品后实际创建对应 Agent/Team/Skill 资源
- [ ] 前端商品卡片展示完整字段（rating/price 等）
- [ ] 用户可对资源进行评价

### 子模块 B：附带生态

- [x] Agent/Team Kind 枚举统一为 5 种值
- [x] Team 表新增 kind 字段，与 Agent Kind 对齐
- [x] 数据迁移完成（system→system_builtin、industry_template→ecosystem_preset）
- [x] system_builtin Agent/Team 不可删除（后端 403 + 前端隐藏按钮）
- [x] 用户可在系统设置页查看加载状态、触发加载/卸载
- [x] KindBadge 正确显示各 Kind 徽章
- [x] 卸载时跨行业 Team 保留，仅移除被卸载行业的 Agent 成员
- [ ] 调用 `POST /api/v1/admin/ecosystem/preset/load` 实际加载行业数据（当前返回 400）
- [ ] 三个行业 Pack 数据可成功加载并验证
- [ ] 行业分类页展示为树形折叠布局

---

## 7. 改动文件清单

### 已完成改动（Phase 1）

**后端**：
- `api/kratos/ecosystem/v1/ecosystem.proto` — 新增
- `internal/biz/ecosystem/ecosystem.go` — 新增
- `internal/biz/ecosystem.go` — 重导出
- `internal/biz/ecosystem_preset.go` — 新增
- `internal/data/ecosystem.go` — 新增
- `internal/data/ecosystem_preset.go` — 新增
- `internal/data/seed_pack.go` — 新增 `SeedPackBuiltinTemplates`/`SeedPackIndustry`/`PackSeeder`
- `internal/data/sql/ecosystem_product.sql` — 新增
- `internal/data/sql/migrations/20260718_ecosystem_preset_schema.sql` — 新增
- `internal/data/ddl_migration_registry.go` — 注册 V20260703/V20260718
- `internal/data/plugin_run_schema.go` — `EnsureEcosystemSchema`
- `internal/data/ent/schema/agent.go` — `kind` 枚举
- `internal/data/ent/schema/team.go` — `kind` 枚举 + 索引
- `internal/data/ent/schema/system_setting.go` — `ecosystem_loaded` 字段
- `internal/service/ecosystem.go` — 新增
- `internal/service/ecosystem_preset.go` — 新增
- `internal/server/http.go` — 路由注册
- `internal/biz/agent_usecase.go` — system_builtin 删除保护
- `internal/biz/team_usecase.go` — system_builtin 删除保护
- `cmd/admin/wire.go` — Wire 绑定

**前端**：
- `web/src/pages/EcosystemPage.vue` — 新增
- `web/src/features/ecosystem/{api,types,useEcosystemPage}.ts` — 新增
- `web/src/stores/ecosystem/index.ts` — 新增
- `web/src/features/system-settings/{api,types,useEcosystemPreset}.ts` — 新增/扩展
- `web/src/stores/system-settings/index.ts` — 扩展
- `web/src/components/agents/KindBadge.vue` — 新增
- `web/src/components/teams/TeamCard.vue` — system_builtin 保护
- `web/src/components/chat/ChatEntitySidebar.vue` — system_builtin 过滤
- `web/src/router/routes.ts` — `/shop` 路由

### 待改动（Phase 2+）

- `internal/data/seed_pack.go` — 实现 `pack.ConvertCompanySpecToPack`（B8）
- `internal/biz/ecosystem/ecosystem.go` — Install 解析 config_json（A5）
- `web/src/features/ecosystem/types.ts` — 补全字段（A6）
- 行业分类前端组件（B13）

---

## 8. 依赖与风险

- **P0 阻塞**：`pack.ConvertCompanySpecToPack` 未实现，导致 L2 行业 Pack 加载管道完全不可用。需先实现 CompanySpec → Pack 转换函数才能解锁后续行业加载验证。
- Marketplace 安装逻辑需与 Agent/Skill/Team 创建流程联动
- 行业 Pack 场景数据已完整（finance/selfmedia/softwaredev 的 agents.yaml 均有完整内容），但需通过 L2 管道加载验证
- builtin-templates Pack 当前 Kind 标记为 `ecosystem_preset` 而非 `system_builtin`，与 L1 层级定义不一致，需确认设计意图
- 需考虑安全审核机制（Phase 3+）
