# Ecosystem 生态商城开发计划
> **版本**：2026-07-30 | **状态**：✅ Phase 1 MVP 已落地 + Preset 架构改造已落地（L2 管道阻塞中）+ ✅ 商城 UI 骨架（六页）已落地（mock 数据）
> **需求**：[30-ecosystem.md](./30-ecosystem.md) · **设计**：[30-ecosystem.design.md](./30-ecosystem.design.md)
> **进度真相**：[execution-plan.md](./0-system.development.md) · **EP**：—
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
| 前端 | `web/src/pages/EcosystemPage.vue`（路由 `/shop` 浏览首页）· `web/src/pages/ShopAssetPage.vue`（`/shop/a/:slug`）· `web/src/pages/ShopCreatorPage.vue`（`/shop/u/:handle`）· `web/src/pages/ShopMePage.vue`（`/shop/me`）· `web/src/pages/ShopStudioPage.vue`（`/shop/studio`）· `web/src/pages/ShopPublishPage.vue`（`/shop/publish`）· `web/src/features/ecosystem/` · `web/src/stores/ecosystem/` · `web/src/components/ecosystem/`（16 组件）· `web/src/features/system-settings/` · `web/src/stores/system-settings/` · `web/src/components/agents/KindBadge.vue` |
| 场景数据 | `internal/scenario/packs/builtin-templates/` · `internal/scenario/packs/finance/` · `internal/scenario/{finance,selfmedia,softwaredev}/agents.yaml` |

---

## 2. 现状评估

### 2.1 生态市场（Marketplace）
| 项 | 状态 | 证据 |
|----|------|------|
| Proto 定义 | ✅ | `ecosystem.proto` 5 RPC（List/Get/Publish/Install/Uninstall），无 RateProduct |
| Biz 层 | ✅ | `biz/ecosystem/` 子包，Usecase + Repo 接口，构造注入 `loggateway.Logger` |
| Service 层 | ✅ | `service/ecosystem.go` 实现 gRPC 服务，`GetProduct` 返回 `NotFound` 错误 |
| Data 层 | ✅ | `data/ecosystem.go`，原生 SQL 操作 `ecosystem_products` + `ecosystem_installs`，读写分离 + 事务包裹 install |
| 数据库表 | ✅ | DDL `internal/data/sql/ecosystem_product.sql` + 迁移 V20260703（`ddlEcosystemSchema`） |
| Wire 注入 | ✅ | `data/biz/service.ProviderSet` + `cmd/admin/wire.go` 已注册 |
| HTTP 路由 | ✅ | `internal/server/http.go:135` 注册 `ecosystemv1.RegisterEcosystemServiceHTTPServer` |
| 前端 UI 骨架（M57 六页） | ✅ | 浏览首页/详情/创作者/买家工作台/创作者中心/发布向导全部落地，16 个专用组件 + Pinia store 扩展；mock 数据驱动，覆盖 11 类资产（含 org_bundle 组织架构整包） |
| 前端 i18n | ✅ | `shopPage.*` 命名空间中英文全量覆盖；check-i18n 无新增违规（mock.ts 业务数据已入 baseline 豁免） |
| 前端类型（M30 子集） | ⚠️ | M30 `EcosystemProduct` 仍为 8 字段子集，未覆盖 Proto 全部字段（rating/price_cents/config_json 等）；M57 商城已建独立完整类型体系（`MarketAsset` 等） |
| 安装逻辑 | ⚠️ | 仅 `RecordInstall` 记录安装关系 + 自增 `install_count`，未解析 `config_json` 创建实际 Agent/Team/Skill；org_bundle 安装还原（部门/岗位/Agent 生成）未实现 |
| 评价/评分 | ❌ | 无 RateProduct RPC，无 `marketplace_reviews` 表（前端骨架已含评价 UI，待后端落地） |
| 商品版本 | ❌ | 无 `marketplace_product_versions` 表 |
| 交易/订单 | ❌ | 无 `marketplace_orders` 表（前端骨架已含订单 UI，待后端落地） |

### 2.2 附带生态（Preset Seed Refactor）
| 项 | 状态 | 证据 |
|----|------|------|
| Kind 枚举统一 | ✅ | Agent/Team Kind = `user \| system_builtin \| ecosystem_preset \| marketplace \| certified`，Ent Schema `agent.go`/`team.go` |
| Team.kind 字段 | ✅ | Ent schema `team.go:39` + DDL 迁移 V20260718 |
| ecosystem_loaded 存储 | ✅ | `system_settings.ecosystem_loaded` TEXT 列，Ent Schema `system_setting.go:47` |
| Preset Load/Unload/Status Biz | ✅ | `biz/ecosystem_preset.go`，含 `sync.Mutex` 防并发 |
| Preset Service | ✅ | `service/ecosystem_preset.go` + HTTP handler |
| Preset Data | ✅ | `data/ecosystem_preset.go`，含级联删除（taxonomy → agents → teams）+ 跨行业 Team 成员清理 |
| HTTP 路由 | ✅ | `internal/server/http.go:192-194` 注册 3 端点 |
| Wire 注入 | ✅ | `wire.Bind(EcosystemPresetRepo)` + `wire.Bind(PackSeeder)` + `provideEcosystemPresetScenarioDir` |
| 前端 Preset 管理 | ✅ | `features/system-settings/useEcosystemPreset.ts` + `stores/system-settings/index.ts` |
| KindBadge 组件 | ✅ | `components/agents/KindBadge.vue` 4 种 Kind 徽章 |
| system_builtin 删除保护（后端） | ✅ | `biz/agent_usecase.go:579` + `biz/team_usecase.go:528` 返回 `Forbidden` |
| system_builtin 删除保护（前端） | ✅ | `components/teams/TeamCard.vue` 隐藏删除/复制按钮；`ChatEntitySidebar.vue` 过滤 system_builtin |
| SeedPackBuiltinTemplates | ✅ | `data/seed_pack.go`，使用 Pack 引擎 + `WithKindOverride("ecosystem_preset")`： agents + 2 teams + 6 graphs |
| SeedPackIndustry 实现 | ❌ | `data/seed_pack.go:125` 调 `pack.ConvertCompanySpecToPack` 未实现，返回 `BadRequest` 错误，L2 管道阻塞 |
| finance 场景数据 | ✅ | `internal/scenario/packs/finance/manifest.yaml`（8 agents + 8 teams）· `internal/scenario/finance/agents.yaml`（完整） |
| selfmedia 场景数据 | ✅ | `internal/scenario/selfmedia/agents.yaml`（10+ agents + 6 teams 完整） |
| softwaredev 场景数据 | ✅ | `internal/scenario/softwaredev/agents.yaml`（10+ agents + 8 teams 完整） |
| 行业分类树形布局 | ❌ | 前端未改造为树形折叠布局（FR-15/FR-16） |
| builtin-templates Kind 标记 | ⚠️ | 当前 `WithKindOverride("ecosystem_preset")`，按 L1 定义应为 `system_builtin`，待确认 |

### 2.3 数据迁移

| 迁移 | 状态 | 证据 |
|------|------|------|
| V20260703 `ecosystem_schema` | ✅ | `ddlEcosystemSchema` + `EnsureEcosystemSchema`，创建 2 表 + 2 索引 |
| V20260718 `ecosystem_preset_schema` DDL | ✅ | `sql/migrations/20260718_ecosystem_preset_schema.sql`：`system_settings.ecosystem_loaded` + `teams.kind` |
| V20260718 数据迁移 | ✅ | `ddlEcosystemPresetDataMigration`：agents.kind system→system_builtin、industry_template→ecosystem_preset；teams.kind source=imported→ecosystem_preset |

---

## 3. 差距与优化
1. **P0 阻塞**：`SeedPackIndustry` 因 `pack.ConvertCompanySpecToPack` 未实现而返回错误，L2 行业 Pack 加载管道完全不可用。调用 `POST /api/v1/admin/ecosystem/preset/load` 会返回 400
2. **P1**：Marketplace 安装逻辑为桩实现，安装商品不会创建实际资源（不解析 `config_json`）；org_bundle 整包还原（部门/岗位/Agent 树生成）需随安装逻辑一并设计
3. **P1**：商城前端六页当前由 `features/ecosystem/mock.ts` 驱动，待后端 Proto/表结构落地后按 `api.ts` 同签名替换为真实 RPC（替换点已收敛在 api.ts 一层）
4. **P1**：builtin-templates Pack 当前使用 `ecosystem_preset` 而非 `system_builtin`，与 L1 层级定义不一致，需确认是否为设计意图
5. **P2**：行业分类前端未改造为树形折叠布局（FR-15/FR-16）
6. **P2**：M30 `EcosystemProduct` 类型为简化子集，缺少 rating/price_cents/config_json 等字段
7. **P3**：评价/评分系统后端未实现
8. **P3**：商品版本管理和兼容性检查未实现
9. **P3**：交易/订单系统后端未实现
---

## 4. 开发阶段
### 子模块 A：生态市场（Marketplace）
| 阶段 | 工作 | 状态 |
|------|------|------|
| Phase 1 | 基础框架：Proto/Biz/Service/Data/Wire/前端页面 | ✅ 已完成 |
| Phase 1.5 | 商城 UI 骨架（六页）：浏览/详情/创作者/买家工作台/创作者中心/发布向导 + 11 类资产（含 org_bundle）+ mock 数据 + i18n | ✅ 已完成（2026-07-30，mock 驱动） |
| Phase 2 | 安装逻辑补全：解析 config_json 创建实际 Agent/Team/Skill；org_bundle 组织树还原 | ⏳ 待开始 |
| Phase 3 | 评价/评分系统：RateProduct RPC + reviews 表 | ⏳ 待开始 |
| Phase 4 | 版本管理 + 兼容性检查：product_versions 表 | ⏳ 待开始 |
| Phase 5 | 交易商业化：orders 表 + 支付集成 | ⏳ 待开始 |

### 子模块 B：附带生态（Preset Seed Refactor）
| 阶段 | 工作 | 状态 |
|------|------|------|
| Phase 1 | 架构改造：Kind 统一/Preset API/Store/KindBadge/删除保护 | ✅ 已完成 |
| Phase 2 | L2 管道打通：实现 `pack.ConvertCompanySpecToPack` | ❌ 阻塞中 |
| Phase 3 | 行业 Pack 数据验证：finance/selfmedia/softwaredev 加载验证 | ⏳ 待开发（依赖 Phase 2） |
| Phase 4 | 前端树形布局：行业分类管理页改造（FR-15/FR-16） | ⏳ 待开始 |
| Phase 5 | builtin-templates Kind 标记确认与修正 | ⏳ 待开始 |

---

## 5. 任务清单

### 子模块 A：生态市场
| # | 任务 | 优先级 | 状态 |
|---|------|--------|------|
| A1 | Proto 定义 5 RPC | P2 | ✅ |
| A2 | Biz/Service/Data 层 | P2 | ✅ |
| A3 | 前端 EcosystemPage | P2 | ✅ |
| A4 | Wire 注入 + HTTP 路由注册 | P2 | ✅ |
| A5 | 安装逻辑补全（解析 config_json 创建资源，含 org_bundle 组织树还原） | P1 | ⏳ |
| A6 | 前端类型补全（对齐 Proto 全字段） | P2 | ⏳ |
| A7 | 评价/评分系统 | P3 | ⏳ |
| A8 | 商品版本管理 | P3 | ⏳ |
| A9 | 交易/订单系统 | P3 | ⏳ |
| A10 | 商城 UI 骨架六页 + 16 组件 + mock + i18n（含 org_bundle 发布向导与详情预览） | P1 | ✅（2026-07-30） |

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
| B9 | finance Pack 加载验证 | P1 | ⏳ 依赖 B8 |
| B10 | selfmedia Pack 加载验证 | P1 | ⏳ 依赖 B8 |
| B11 | softwaredev Pack 加载验证 | P1 | ⏳ 依赖 B8 |
| B12 | builtin-templates Kind 标记确认（`ecosystem_preset` vs `system_builtin`） | P2 | ⏳ |
| B13 | 行业分类树形布局（FR-15/FR-16） | P2 | ⏳ |

---

## 6. 验收标准

### 子模块 A：生态市场
- [x] 用户可发布 Agent/Skill/Team 到市场
- [x] 用户可浏览和搜索市场商品
- [x] 商品安装/卸载 API 可用（记录安装关系）
- [x] 商城六页 UI 骨架可用：浏览（分类树/过滤/榜单/搜索）、详情（README/权限/评价/组织整包预览）、创作者主页、买家工作台、创作者中心、发布向导（含 org_bundle 组织树勾选）
- [x] 前端昼夜双模渲染正常、无 i18n key 裸奔、无 console error（2026-07-30 浏览器复验通过）
- [ ] 安装商品后实际创建对应 Agent/Team/Skill 资源（org_bundle 还原部门/岗位/Agent）
- [ ] 前端商品卡片展示完整字段（rating/price 等）——UI 骨架已支持，待 M30 真实数据对齐
- [ ] 用户可对资源进行评价（前端已支持，待后端 RPC）

### 子模块 B：附带生态
- [x] Agent/Team Kind 枚举统一（5 种）
- [x] Team 表新增 kind 字段，与 Agent Kind 对齐
- [x] 数据迁移完成（system→system_builtin、industry_template→ecosystem_preset）
- [x] system_builtin Agent/Team 不可删除（后端 403 + 前端隐藏按钮）
- [x] 用户可在系统设置页查看加载状态、触发加载/卸载
- [x] KindBadge 正确显示 4 种 Kind 徽章
- [x] 卸载时跨行业 Team 保留，仅移除被卸载行业的 Agent 成员
- [ ] 调用 `POST /api/v1/admin/ecosystem/preset/load` 实际加载行业数据（当前返回 400）
- [ ] 三个行业 Pack 数据可成功加载并验证
- [ ] 行业分类页展示为树形折叠布局

---

## 7. 改动文件清单

### 已完成改动（Phase 1 + Phase 1.5 UI 骨架）
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

**前端（Phase 1.5 商城 UI 骨架，2026-07-30）**：
- `web/src/pages/EcosystemPage.vue` — 重写为浏览首页（Hero 搜索 + 分类树 + 过滤栏 + 三榜单 + 卡片网格）
- `web/src/pages/ShopAssetPage.vue` — 新增：商品详情页（README/组织整包预览/版本/评价四 Tab + 权限确认弹窗）
- `web/src/pages/ShopCreatorPage.vue` — 新增：创作者主页
- `web/src/pages/ShopMePage.vue` — 新增：买家工作台（已安装/订单 Tabs）
- `web/src/pages/ShopStudioPage.vue` — 新增：创作者中心（统计/资产/收件箱）
- `web/src/pages/ShopPublishPage.vue` — 新增：发布向导（四步 Stepper，含 org_bundle 组织树勾选步骤）
- `web/src/features/ecosystem/types.ts` — 扩展：M57 商城领域类型（MarketAsset/OrgBundleNode/MyInstall/MyOrder/StudioStats/OrgPickNode 等）
- `web/src/features/ecosystem/mock.ts` — 新增：11 类资产 mock 数据（含 2 个 org_bundle 样例、分类树、订单、工作室数据）
- `web/src/features/ecosystem/api.ts` — 扩展：M57 商城 mock API（searchAssets/getAsset/listCategories/install/listMyInstalls/getStudioStats/getOrgPickTree/submitReview 等），与 M30 RPC 双轨并存
- `web/src/features/ecosystem/marketUi.ts` — 新增：资产类型元数据（图标/颜色）+ 价格/安装量格式化
- `web/src/features/ecosystem/useMarketBrowsePage.ts` — 新增：浏览首页 composable（注意：reactive filter 经 store.filter 直接引用）
- `web/src/features/ecosystem/useMarketAssetDetail.ts` — 新增：详情页 composable（安装确认/卸载/评分提交）
- `web/src/stores/ecosystem/index.ts` — 扩展：M57 商城状态（categories/filter/assets/assetDetail/creatorDetail/myInstalls/myOrders/studio*/orgPickTree）与 actions
- `web/src/components/ecosystem/` — 新增 16 组件：AssetCard / AssetTypeIcon / PriceTag / RatingStars / CategoryTree / MarketFilterBar / MarketLeaderboard / InstallConfirmDialog / PermissionList / AssetScreenshots / ReviewSection / ReplyReviewDialog / TrendSparkline / PublishTypeSelect / OrgBundleTree / OrgBundlePicker
- `web/src/router/routes.ts` — 新增 `/shop/a/:slug`、`/shop/u/:handle`、`/shop/me`、`/shop/studio`、`/shop/publish` 路由
- `web/src/i18n/locales/zh-CN.ts` / `en-US.ts` — 新增 `shopPage.*` 命名空间（含 notify* 反馈文案）
- `web/scripts/i18n-baseline.json` — mock.ts 业务数据中文入 baseline 豁免

**前端（Phase 1 既有）**：
- `web/src/features/ecosystem/useEcosystemPage.ts` — M30 既有 composable（保留兼容）
- `web/src/features/system-settings/{api,types,useEcosystemPreset}.ts` — 新增/扩展
- `web/src/stores/system-settings/index.ts` — 扩展
- `web/src/components/agents/KindBadge.vue` — 新增
- `web/src/components/teams/TeamCard.vue` — system_builtin 保护
- `web/src/components/chat/ChatEntitySidebar.vue` — system_builtin 过滤
- `web/src/router/routes.ts` — `/shop` 路由

### 待改动（Phase 2+）
- `internal/data/seed_pack.go` — 实现 `pack.ConvertCompanySpecToPack`（B8）
- `internal/biz/ecosystem/ecosystem.go` — Install 解析 config_json（A5，含 org_bundle 还原）
- `web/src/features/ecosystem/api.ts` — mock 实现替换为真实 RPC（同签名，Phase 2 后端就绪后）
- `web/src/features/ecosystem/types.ts` — M30 子集类型对齐 Proto 全字段（A6）
- 行业分类前端组件（B13）
---

## 8. 依赖与风险
- **P0 阻塞**：`pack.ConvertCompanySpecToPack` 未实现，导致 L2 行业 Pack 加载管道完全不可用。需先实现 CompanySpec → Pack 转换函数才能解锁后续行业加载验证
- Marketplace 安装逻辑需与 Agent/Skill/Team 创建流程联动；org_bundle 安装需联动组织架构（taxonomy）+ Agent 批量创建
- 行业 Pack 场景数据已完整（finance/selfmedia/softwaredev 的 agents.yaml 均有完整内容），但需通过 L2 管道加载验证
- builtin-templates Pack 当前 Kind 标记为 `ecosystem_preset` 而非 `system_builtin`，与 L1 层级定义不一致，需确认设计意图
- 前端商城六页由 mock 驱动；后端落地时需保持 `api.ts` 函数签名稳定，仅替换实现，避免波及页面/组件层
- 需考虑安全审核机制（Phase 3+）

---

## 9. 变更记录

| 日期 | 版本 | 变更 |
|------|------|------|
| 2026-06-17 | 0.1 | Phase 1 MVP + Preset 架构改造落地记录 |
| 2026-07-30 | 0.2 | Phase 1.5 商城 UI 骨架（六页 + 16 组件 + org_bundle）落地；修复文件损坏编码；任务清单新增 A10；验收标准同步浏览器复验结论 |
