# Ecosystem 生态 — 开发计划

> **版本**：2026-06-06 | **状态**：🟡 Phase 1 MVP 已落地 + Preset Seed Refactor 已落地
> **需求**：[30 ecosystem.md](./30-ecosystem.md) · **设计**：[30-ecosystem.design.md](./30-ecosystem.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：—

---

## 1. 模块定位

Ecosystem 生态：Agent/Skill/Team 的市场与分享平台，支持发布、发现、安装和评价；以及附带生态行业预设的加载/卸载管理。

**代码锚点**：
- `api/kratos/ecosystem/v1/ecosystem.proto`
- `internal/biz/ecosystem/ecosystem.go` · `internal/biz/ecosystem_preset.go`
- `internal/data/ecosystem.go` · `internal/data/ecosystem_preset.go`
- `internal/service/ecosystem.go` · `internal/service/ecosystem_preset.go`
- `web/src/pages/EcosystemPage.vue`（路由 `/shop`）
- `web/src/features/ecosystem/` · `web/src/stores/ecosystem/`

---

## 2. 现状评估

### 2.1 生态市场（Marketplace）

| 项 | 状态 | 证据 |
|----|------|------|
| Proto 定义 | ✅ | `ecosystem.proto` — 5 RPC（List/Get/Publish/Install/Uninstall） |
| Biz 层 | ✅ | `biz/ecosystem/` 子包，Usecase + Repo 接口 |
| Service 层 | ✅ | `service/ecosystem.go` 实现 gRPC 服务 |
| Data 层 | ✅ | `data/ecosystem.go`，原生 SQL 操作 `ecosystem_products` + `ecosystem_installs` |
| 数据库表 | ✅ | DDL + 迁移（V20260703） |
| Wire 注入 | ✅ | `cmd/admin/wire.go` 已注册 |
| 前端页面 | ✅ | `EcosystemPage.vue` + `features/ecosystem/` + Pinia store |
| 安装逻辑 | ⚠️ | 仅记录安装关系，未解析 `config_json` 创建实际 Agent/Team/Skill |
| 评价/评分 | ❌ | 无 RateProduct RPC，无 `marketplace_reviews` 表 |
| 商品版本 | ❌ | 无 `marketplace_product_versions` 表 |
| 交易/订单 | ❌ | 无 `marketplace_orders` 表 |

### 2.2 附带生态（Preset Seed Refactor）

| 项 | 状态 | 证据 |
|----|------|------|
| Kind 枚举统一 | ✅ | Agent/Team Kind = `user \| system_builtin \| ecosystem_preset \| marketplace \| certified` |
| Team.kind 字段 | ✅ | Ent schema + DDL 迁移（V20260718） |
| ecosystem_loaded 存储 | ✅ | `system_settings.ecosystem_loaded` JSON 列 |
| Preset Load/Unload Biz | ✅ | `biz/ecosystem_preset.go`，含互斥锁 |
| Preset Service | ✅ | HTTP handler，3 端点（load/unload/status） |
| Preset Data | ✅ | `data/ecosystem_preset.go`，含级联删除逻辑 |
| 前端 Preset 管理 | ✅ | `system-settings` store/API 集成 |
| KindBadge 组件 | ✅ | `components/agents/KindBadge.vue` |
| SeedPackIndustry | ✅ | `data/seed_pack.go`，支持 `WithKindOverride` |
| builtin-templates Pack | ✅ | 7 agents + 2 teams + 6 graphs，完整 |
| finance Pack | ⚠️ | manifest 声明 38 agents/8 teams，实际仅 1 agent yaml |
| selfmedia Pack | ❌ | 目录为空，无数据文件 |
| softwaredev Pack | ❌ | 目录为空，无数据文件 |
| 行业分类树形布局 | ❌ | 前端未改造为树形折叠布局 |
| system_builtin 删除保护 | ⚠️ | Kind 枚举已到位，API 层 403 保护待确认 |

---

## 3. 差距与优化

1. **P1**：Marketplace 安装逻辑为桩实现，安装商品不会创建实际资源
2. **P1**：selfmedia/softwaredev 行业 Pack 数据完全缺失，finance 数据严重不完整
3. **P2**：行业分类前端未改造为树形折叠布局（FR-15/FR-16）
4. **P2**：评价/评分系统未实现
5. **P3**：商品版本管理和兼容性检查未实现
6. **P3**：交易/订单系统未实现

---

## 4. 开发阶段

### 子模块 A：生态市场（Marketplace）

- **Phase 1**：基础框架 ✅ — Proto/Biz/Service/Data/Wire/前端页面已落地
- **Phase 2**：安装逻辑补全 — 解析 config_json 创建实际 Agent/Team/Skill
- **Phase 3**：评价/评分系统
- **Phase 4**：版本管理 + 兼容性检查
- **Phase 5**：交易商业化

### 子模块 B：附带生态（Preset Seed Refactor）

- **Phase 1**：架构改造 ✅ — Kind 统一/Preset API/Store/KindBadge 已落地
- **Phase 2**：行业 Pack 数据补全 — finance 补全 + selfmedia/softwaredev 创建
- **Phase 3**：前端树形布局 — 行业分类管理页改造
- **Phase 4**：system_builtin 删除保护完善

---

## 5. 任务清单

### 子模块 A：生态市场

| # | 任务 | 优先级 | 状态 |
|---|------|--------|------|
| A1 | Proto 定义（5 RPC） | P2 | ✅ |
| A2 | Biz/Service/Data 层 | P2 | ✅ |
| A3 | 前端 EcosystemPage | P2 | ✅ |
| A4 | Wire 注入 | P2 | ✅ |
| A5 | 安装逻辑补全（解析 config_json 创建资源） | P1 | ❌ |
| A6 | 评价/评分系统 | P3 | ❌ |
| A7 | 商品版本管理 | P3 | ❌ |
| A8 | 交易/订单系统 | P3 | ❌ |

### 子模块 B：附带生态

| # | 任务 | 优先级 | 状态 |
|---|------|--------|------|
| B1 | Agent/Team Kind 枚举统一 | P1 | ✅ |
| B2 | ecosystem_loaded 存储 | P1 | ✅ |
| B3 | Preset Load/Unload/Status API | P1 | ✅ |
| B4 | 前端 Preset 管理（system-settings 集成） | P1 | ✅ |
| B5 | KindBadge 组件 | P2 | ✅ |
| B6 | SeedPackIndustry + WithKindOverride | P1 | ✅ |
| B7 | builtin-templates Pack 数据 | P1 | ✅ |
| B8 | finance Pack 数据补全 | P1 | ❌ |
| B9 | selfmedia Pack 数据创建 | P1 | ❌ |
| B10 | softwaredev Pack 数据创建 | P1 | ❌ |
| B11 | 行业分类树形布局（FR-15/FR-16） | P2 | ❌ |
| B12 | system_builtin 删除保护完善 | P2 | ❌ |

---

## 6. 验收标准

### 子模块 A：生态市场

- [x] 用户可发布 Agent/Skill/Team 到市场
- [x] 用户可浏览和搜索市场商品
- [ ] 安装商品后实际创建对应 Agent/Team/Skill 资源
- [ ] 用户可对资源进行评价

### 子模块 B：附带生态

- [x] Agent/Team Kind 枚举统一为 5 种值
- [x] 用户可在系统设置页加载/卸载行业预设
- [x] 加载后 Agent/Team Kind 标记为 ecosystem_preset
- [x] KindBadge 正确显示各 Kind 徽章
- [ ] 三个行业 Pack 数据完整可用
- [ ] 行业分类页展示为树形折叠布局

---

## 7. 依赖与风险

- Marketplace 安装逻辑需与 Agent/Skill/Team 创建流程联动
- 行业 Pack 数据需业务方提供行业知识内容
- 需考虑安全审核机制（Phase 3+）
- selfmedia/softwaredev Pack 数据从零创建，工作量大
