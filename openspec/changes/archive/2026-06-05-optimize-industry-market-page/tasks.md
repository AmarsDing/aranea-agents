# 行业管理界面优化（市场页）— 任务清单

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把 [IndustryMarketPage.vue](file:///f:/aranea-agents/web/src/pages/industries/IndustryMarketPage.vue) 从 50 行极简页升级为可比较、可筛选、带 drawer 的「行业模板注册表」，沿用项目 glass/cream/amber 设计 token，0 新增依赖。

**Architecture:** 编排层 + 4 子组件。`IndustryMarketPage` 变薄，新增 `IndustryMetricStrip` / `IndustryMarketToolbar` / `IndustryTableRow` / `IndustryDrawer` 4 个展示组件，重写 `IndustryCard`、扩展 `useIndustryMarket`。

**Tech Stack:** Vue 3 + Quasar + Pinia + TypeScript + vuedraggable（已装） + 现有 SCSS tokens

**Design Doc:** [design.md](./design.md)
**Design Experiments:** [docs/design-experiments/industries/](../../docs/design-experiments/industries/a-information-architecture.html)

---

## 1. 扩展 useIndustryMarket composable

**Files:**
- Modify: `web/src/features/industries/useIndustryMarket.ts`

- [x] **1.1** 在 `useIndustryMarket` 返回值中新增 `summary` computed

**实际实现差异**：
- `summary` 逻辑委托到独立模块 `web/src/features/industries/industryMarketFilters.ts` 的 `summarizeIndustries()` 函数，composable 内为 `computed(() => summarizeIndustries(industries.value))`
- `IndustrySummary` 接口比任务多了一个 `disabled` 字段
- composable 额外提供了 `applyFilters()`、`fetchIndustryDetail()`、`clearIndustryDetail()` 等方法（任务 7 所需基础设施）

**DoD:**
- [x] 字段类型完整（TypeScript 编译通过）
- [x] 现有调用方（IndustryMarketPage）行为不变

---

## 2. 新增 IndustryMetricStrip 组件（4 KPI glass 卡）

**Files:**
- Create: `web/src/components/industries/IndustryMetricStrip.vue`

- [x] **2.1** 创建组件，props 接 `summary: { total, enabled, departments, positions, agents, installed }`

**实际实现**：与任务要求一致，4 列 grid（桌面）/ 2 列（移动），4 个 KPI 卡，使用 `app-metrics-grid` class 和 `app-mono` 等宽数字。

**DoD:**
- [x] 组件存在，模板渲染 4 个卡
- [x] 数字使用 `font-feature-settings: 'tnum' 1` 等宽对齐
- [x] 传入 prop 变化时正确响应

---

## 3. 新增 IndustryMarketToolbar 组件

**Files:**
- Create: `web/src/components/industries/IndustryMarketToolbar.vue`

- [x] **3.1** 创建组件，props: `modelValue: { query, statusFilter, sourceFilter, view, counts }`，emits: `update:modelValue`

**实际实现差异**：
- `counts` 从 modelValue 中拆出为独立 prop（更合理的设计）
- 搜索框支持 `@keydown.meta.k` 快捷键
- 状态 chips + 来源 chips + 视图切换均已实现

**DoD:**
- [x] 搜索框带 `⌘K` 快捷键（focus）
- [x] chip 点击切换 active
- [x] 视图切换 emit 改变 `view`
- [x] v-model 双向绑定正常

---

## 4. 重写 IndustryCard 组件

**Files:**
- Modify: `web/src/components/industries/IndustryCard.vue`

- [x] **4.1** 重写卡片，props: `industry: Industry`, `isOpen: boolean`, emits: `select`

**实际实现**：与任务要求一致。monogram (40x40 渐变方块) + 名称 + key (mono) + 状态 pill + 描述 2 行 truncate + divider + 4 个 metrics 列 + foot（查看部门 + 来源标签）。hover: translateY(-2px) + amber border，isOpen: amber border + shadow。

**DoD:**
- [x] 组件替换现有实现
- [x] `pnpm build` 通过

---

## 5. 新增 IndustryTableRow 组件

**Files:**
- Create: `web/src/components/industries/IndustryTableRow.vue`

- [x] **5.1** 创建组件，props: `industry: Industry`, emits: `select`

**实际实现**：与任务要求一致。8 列布局（monogram+名称+key / 描述 / 部门 / 岗位 / Agent / 已部署 / 状态 pill / 来源标签），hover 灰底。

**DoD:**
- [x] 组件存在
- [x] hover 状态正常
- [x] emit `select` 触发上层打开 drawer

---

## 6. 新增 IndustryDrawer 组件

**Files:**
- Create: `web/src/components/industries/IndustryDrawer.vue`

- [x] **6.1** 创建组件，props: `modelValue: boolean`, `industry: Industry | null`, `departments: Department[]`, emits: `update:modelValue`, `install`, `view-prompts`

**实际实现差异**：
- `departments: Department[]` 改为 `detail: IndustryDetail`（含 departments + positionsByDept），更合理的设计
- 额外添加了 `detailLoading` prop
- 其余与任务要求一致：遮罩 + blur、480px 宽、header/body/footer 结构、slide-in 240ms 动画、Esc 关闭、遮罩点击关闭

**DoD:**
- [x] 抽屉打开/关闭动画流畅
- [x] 部门+岗位数据正确渲染
- [x] `Esc` 与遮罩点击都关闭
- [x] 三个 emit 正确触发

---

## 7. 重写 IndustryMarketPage 为编排层

**Files:**
- Modify: `web/src/pages/industries/IndustryMarketPage.vue`

- [x] **7.1** 重写为编排层，引入 5 个新/重写子组件

**实际实现差异**：
- CTA 区域直接内联在页面中，未创建独立 `IndustryCtaCard.vue` 组件
- 额外增加了 "signature quote" 区域（任务未提及）
- 额外增加了空状态处理（任务未明确提及但属于合理补充）
- 其余与任务要求一致：AppPageHero + IndustryMetricStrip + IndustryMarketToolbar + grid/table 视图切换 + IndustryCard/IndustryTableRow + IndustryDrawer

**DoD:**
- [x] 搜索/筛选/视图切换交互正常
- [x] 卡片点击打开 drawer，drawer 关闭恢复
- [x] `pnpm build` 通过

---

## 8. 新增 SCSS partial（可选 — 若现有 tokens 不够）

**Files:**
- Create: `web/src/css/theme/_industry-market.sass`
- Modify: `web/src/css/app-theme.sass`（仅在样式不够时引入 `@use`）

- [x] **8.1** 不需要实施 — 所有组件样式使用 scoped SCSS + 项目已有 CSS 变量 token，drawer 动画也在组件 scoped 样式中定义，无需额外全局 partial

**DoD:**
- [x] 引入 app-theme 后全站无样式泄漏（N/A — 未新增全局样式入口）

---

## 9. 补 i18n key

**Files:**
- Modify: `web/src/i18n/locales/zh-CN.ts`
- Modify: `web/src/i18n/locales/en-US.ts`

- [x] **9.1** 在 `industries` 命名空间下补 key

**实际实现差异**：
- i18n key 使用扁平结构（如 `industries.market.actionRefresh`）而非任务描述的嵌套结构（如 `industries.market.actions.refresh`），这是 Vue I18n 常见做法
- 额外添加了任务未要求的 key：`metricEnabledDelta`、`metricEnabledFoot`、`metricDepartmentsFoot`、`metricPositionsFoot`、`metricAgentsFoot`、`searchKbd`、`metricDept`、`metricPos`、`metricAgent`、`metricInstalled`、`emptyTitle`、`emptyHint`、`drawerLoadingDepts`、`drawerNoDepts`、`signatureQuote`、`signatureCaption`、`noMetricHint`、`tableDesc`、`tableStatus`、`tableSource`

- [x] **9.2** 同步到 en-US.ts（结构对齐，文案英化）

**DoD:**
- [x] 全站 grep 无 `T('industries.market.*')` 落空
- [x] 中英文切换均渲染正常

---

## 10. 全量验证

- [x] **10.1** 前端 lint

Run: `cd web && pnpm lint`
Expected: PASS

- [x] **10.2** 前端 build

Run: `cd web && pnpm build`
Expected: PASS

- [x] **10.3** 前端 typecheck

Run: `cd web && pnpm typecheck`（或 `vue-tsc --noEmit`）
Expected: PASS

- [x] **10.4** 手动集成测试

1. 打开 `/industries` 路由
2. 验证 4 metric 卡数字正确（21 部门 / 105 岗位 / 208 Agent）
3. 验证搜索 "软" → 只显示软件开发
4. 验证状态 chip 切换 "启用" → 全部行业仍显示
5. 验证视图切换：网格 ↔ 列表 切换正常，搜索词保留
6. 验证点击行业卡 → drawer 滑出，显示部门+岗位
7. 验证 drawer `Esc` 关闭
8. 验证 drawer 遮罩点击关闭
9. 验证 CTA 卡 "申请新行业" 显示正确
10. 验证 light/dark 模式（若启用）样式 fallback

- [x] **10.5** Final commit

```bash
git add -A
git commit -m "feat(industries): redesign market page with metrics/toolbar/drawer (Direction A)"
```

---

## 额外文件（任务未提及但实现中创建）

以下文件是实现中创建的，不在原始 tasks.md 的文件列表中：

1. **`web/src/features/industries/industryMarketFilters.ts`** — 筛选/聚合纯函数模块，包含 `IndustryStatusFilter`、`IndustrySourceFilter`、`IndustryFilters` 类型，以及 `filterIndustries()`、`summarizeIndustries()` 函数
2. **`web/src/features/industries/industryMonogram.ts`** — monogram 工具函数，提供 `monoBgForKey()` 和 `monoLettersForKey()`，被 IndustryCard/IndustryDrawer/IndustryTableRow 共享
3. **`web/src/features/industries/types.ts`** 扩展 — `Industry` 类型新增可选字段 `deptCount?`、`posCount?`、`agentCount?`、`installed?`（后端不返回故为可选，客户端并行拉取后填充）
