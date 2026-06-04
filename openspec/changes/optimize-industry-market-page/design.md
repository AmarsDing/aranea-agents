# 行业管理界面优化（市场页）— 设计

> 日期：2026-06-04
> 状态：基于 Direction A 方案（信息建筑派 · Pentagram/Linear 风格）

---

## 1. Context

[IndustryMarketPage.vue](file:///f:/aranea-agents/web/src/pages/industries/IndustryMarketPage.vue) 当前实现为 50 行极简页：

- 一个 `q-page` + 标题 + 描述
- 3 个 `<IndustryCard />` 在 `col-12 col-md-4` grid
- [IndustryCard.vue](file:///f:/aranea-agents/web/src/components/industries/IndustryCard.vue) 只显示 emoji + 名称 + 描述
- 无搜索、无筛选、无视图切换、无 metric、无 drawer
- 不引用项目任何 design token（[app-theme.sass](../../../../web/src/css/app-theme.sass) / [cream-constants.sass](../../../../web/src/css/theme/_cream-constants.sass) / [registry-page.sass](../../../../web/src/css/theme/_registry-page.sass)）

**数据现状**（[types.ts](../../../../web/src/features/industries/types.ts)）：`Industry` 类型当前字段 = `id, key, name, icon, description, scenario_key, enabled, sort_order`。**没有** `deptCount / posCount / agentCount / installed` 字段（与 [TaxonomyIndustryCard.vue](../../../../web/src/components/agents/TaxonomyIndustryCard.vue) 用的 `Industry` 是不同 feature 的同名概念）。后端 list endpoint 当前不返回 counts。

**因此 metric 改为客户端并行计算**：在 `useIndustryMarket.fetchIndustries` 内对每个 industry 额外 `Promise.all([listDepartments, listPositions])` 取长度。当前 3 行业 = 3+3=6 次请求，无明显性能问题；未来 > 10 行业时应在后端 list endpoint 聚合（独立 change）。

设计探索产物：[docs/design-experiments/industries/](../../../../docs/design-experiments/industries/) 三个方向 demo，已选 A。

---

## 2. Goals / Non-Goals

**Goals：**
- 把市场页升级为可比较、可筛选的"行业模板注册表"
- 沿用项目 design system，**0 新增依赖**（仍用 quasar + vuedraggable + 现有 SCSS tokens）
- 沿用项目分层（services/ → features/ → stores/ → composables/ → pages/ → components/）

**Non-Goals：**
- 不动 IndustryDetailPage / IndustryWizard
- 不动后端、不动 store 数据流
- 不引入 dark theme
- 不引入 B / C 方向设计（如果未来要，独立 change）
- 不动 i18n 主入口结构（仅加 key）

---

## 3. Decisions

### 3.1 拆分 IndustryMarketPage 为编排层 + 4 个新子组件

```
IndustryMarketPage.vue                ← 编排层：hero + metrics + toolbar + content 切换
  ├─ IndustryMetricStrip.vue          ← 4 KPI glass 卡
  ├─ IndustryMarketToolbar.vue        ← 搜索 + 状态 chips + 来源 chips + 视图切换
  ├─ IndustryCard.vue                 ← 重写：monogram + 4 metric + hover lift
  ├─ IndustryTableRow.vue             ← 列表视图的单行（密集对比）
  └─ IndustryDrawer.vue               ← 侧滑详情：部门 + 岗位列表
```

理由：
- 单文件超 200 行不易维护，编排层负责状态，子组件各司其职
- IndustryDrawer 复用 IndustryDetailPage 的部门/岗位数据（无需重写数据获取）
- 4 个 metric 子组件与 [TaxonomyIndustryCard.vue](../../../../web/src/components/agents/TaxonomyIndustryCard.vue) 字段对齐，方便后续抽取共享类型

### 3.2 不引入新 composable，沿用 `useIndustryMarket` + 轻扩展

**保持 store 单一数据源**，状态（搜索词/筛选/view/drawer 开关）保留在 `IndustryMarketPage.vue` 的 `setup()` ref 中：

- 搜索：local `query` ref，透传 IndustryMarketToolbar
- 筛选：local `statusFilter` / `sourceFilter` ref
- 视图：local `view` ref（'grid' | 'table'）
- drawer：local `openKey` ref

**`useIndustryMarket` 扩展**：
1. `summary` computed（总部门/岗位/Agent/已部署数）
2. `fetchIndustries` 内部对每个 industry 并行 `listDepartments + listPositions` 注入 `deptCount/posCount`（Agent 数 = pos 数 × 平均，每个 position 配 1 个 Agent MVP，后续支持 1:N）
3. 行业类型扩展（仅前端）：`Industry` 加可选 `deptCount?: number; posCount?: number; agentCount?: number`，**不破坏后端**

**不引入 `useIndustrySearch` 等独立 composable**——本页面 UI 状态内聚，无复用场景，过度抽象反而违反红线（[aranea-frontend-guide §1](../../../../.trae/skills/aranea-frontend-guide/SKILL.md)）。

### 3.3 视图切换不路由化

保持在同一页面内 `view` ref 切换（grid ↔ table），URL 不加 `?view=table` 路由。理由：
- 这是临时视图偏好，不是可分享的页面状态
- 切换不应触发组件 remount（保留搜索词/筛选/滚动位置）
- 未来如要持久化到 URL，1 行 `useRoute().query` 即可

### 3.4 monogram 替代 emoji 杜绝 AI slop

行业"icon" 改为 monogram：取行业 key 的大写首字母（SD / SM / FI），放进一个 40×40 圆角矩形 + 行业专属渐变背景：
- 软件开发：`linear-gradient(135deg, #4F46E5 0%, #312E81 100%)` indigo
- 自媒体：`linear-gradient(135deg, #E55C5C 0%, #9B2226 100%)` rose
- 金融：`linear-gradient(135deg, #0EA5E9 0%, #075985 100%)` sky

理由：
- emoji 在多 OS 渲染差异大，且为典型 AI slop
- 字母 monogram 配合渐变既保留视觉识别度又不失简约
- 颜色由行业 key 派生，**新增行业时无需手动选色**（用 key 的 hash 选预设 3 色之一即可）

### 3.5 侧滑 drawer 而非路由跳转

点击行业卡 → 右侧滑出 drawer（480px 宽），显示部门+岗位列表。理由：
- 市场页是"浏览"语境，跳走破坏列表上下文
- drawer 内可继续操作（安装、查看 Prompt），不打断浏览节奏
- 与 [drawer-pattern.sass](../../../../web/src/css/theme/_drawer-pattern.sass) 已有的模式一致
- 满足 [aranea-frontend-guide §1 红线 #11](../../../../.trae/skills/aranea-frontend-guide/SKILL.md)：Dialog/Drawer 用于不打断主流程的二级操作

### 3.6 列表/网格双视图并存

默认 grid（视觉享受），table 切换为「密集对比」——同行显示更多 metric，可一次性扫 8-12 个行业。理由：
- 满足「数据密度可切换」的探索需求
- 沿用项目 [registry-page.sass](../../../../web/src/css/theme/_registry-page.sass) 的 table 模式

### 3.7 CTA 卡 "申请新行业"

3 个实际行业 + 1 个 dashed 边框 CTA 卡，CTA 卡显示"申请新行业"+"提交业务场景描述，平台团队会评估"。理由：
- 设计上明确展示「行业可扩展性」
- 不堆假数据（避免 AI slop #5 — "data slop"）
- 与 [IndustryWizard](../../../../web/src/features/industries/useIndustryWizard.ts) 解耦，本卡只触发 dialog / 路由跳转

### 3.8 沿用现有 design tokens，**0 新增 CSS 入口**

| 用到的 token | 含义 |
|-------------|------|
| `--glass-surface` / `--glass-border` / `--glass-blur-default` | 卡背景 |
| `--color-accent` (#DCA03E) | 主 accent（启用状态、CTA 按钮） |
| `--color-success` / `--color-success-soft` | 启用 pill |
| `--color-text-primary` / `--color-text-secondary` / `--color-text-tertiary` | 文本层次 |
| `--space-1` ~ `--space-8` | 间距体系 |

新增样式仅作为 `app-theme.sass` 的 partial（或独立 `_industry-market.sass`），不破坏现有结构。

---

## 4. Risks / Trade-offs

| 风险 | 缓解 |
|------|------|
| IndustryMarketPage.vue 重写后单文件仍可能 ≥ 200 行 | 拆出 4 个子组件后编排层预计 80-120 行，达标 |
| 4 metric 字段（deptCount 等）后端若未聚合返回，前端 reduce 会导致大列表性能问题 | 当前 3 行业数据极小可忽略；如未来 >50 行业，扩展 store 提供 `summary` 字段 |
| monogram 颜色用 key hash 可能撞色 | 行业 key 当前由运营预设，未来 key 池增长后再扩色板 |
| drawer 引入后，部分用户可能预期"点击卡直接进详情页" | 在卡上 hover 高亮 + 抽屉外遮罩点击关闭，给出明确的"这是快速查看而非跳转"暗示 |
| 不持久化视图偏好到 localStorage | 接受——用户每次主动切换成本极低，过度持久化反而干扰；如产品反馈要做再加 |

---

## 5. Migration Plan

无后端变更，部署即生效。回滚方式：git revert 即可（纯前端）。

---

## 6. Open Questions

无。所有决策在 Direction A demo 中已可视化验证，参见 [a-information-architecture.html](../../../../docs/design-experiments/industries/a-information-architecture.html)。
