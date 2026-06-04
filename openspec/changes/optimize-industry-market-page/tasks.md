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

- [ ] **1.1** 在 `useIndustryMarket` 返回值中新增 `summary` computed

```ts
const summary = computed(() => ({
  total: industries.value.length,
  enabled: industries.value.filter(i => i.status === 'enabled').length,
  departments: industries.value.reduce((s, i) => s + i.deptCount, 0),
  positions: industries.value.reduce((s, i) => s + i.posCount, 0),
  agents: industries.value.reduce((s, i) => s + i.agentCount, 0),
  installed: industries.value.reduce((s, i) => s + i.installed, 0),
}));
```

**DoD:**
- 字段类型完整（TypeScript 编译通过）
- `cd web && pnpm typecheck` 或 `pnpm build` 通过
- 现有调用方（IndustryMarketPage）行为不变

---

## 2. 新增 IndustryMetricStrip 组件（4 KPI glass 卡）

**Files:**
- Create: `web/src/components/industries/IndustryMetricStrip.vue`

- [ ] **2.1** 创建组件，props 接 `summary: { total, enabled, departments, positions, agents, installed }`

布局：4 列 grid（桌面）/ 2 列（移动）
- 卡 1：已启用行业 / 总数 + 本月新增
- 卡 2：部门总数 / 跨所有行业
- 卡 3：岗位总数 / 人均 Agent 数（agent/pos ratio）
- 卡 4：Agent 总数 / 已部署实例

样式：沿用项目 [registry-page.sass](../../web/src/css/theme/_registry-page.sass) 的 `app-metrics-card` 模式（glass + 1px border + 24px padding）

**DoD:**
- 组件存在，模板渲染 4 个卡
- 数字使用 `font-feature-settings: 'tnum' 1` 等宽对齐
- 传入 prop 变化时正确响应

---

## 3. 新增 IndustryMarketToolbar 组件

**Files:**
- Create: `web/src/components/industries/IndustryMarketToolbar.vue`

- [ ] **3.1** 创建组件，props: `modelValue: { query, statusFilter, sourceFilter, view, counts }`，emits: `update:modelValue`

布局：
- 左：搜索框（⌕ icon + input + ⌘K kbd）
- 中：状态 chips（全部/启用/停用，带 count）+ 来源 chips（全部/系统/自建）
- 右：视图切换（网格/列表，icon-toggle）

**DoD:**
- 搜索框带 `⌘K` 快捷键（focus）
- chip 点击切换 active
- 视图切换 emit 改变 `view`
- v-model 双向绑定正常

---

## 4. 重写 IndustryCard 组件

**Files:**
- Modify: `web/src/components/industries/IndustryCard.vue`

- [ ] **4.1** 重写卡片，props: `industry: Industry`, `isOpen: boolean`, emits: `select`

模板：
- head: monogram（40×40 渐变方块，2 字母） + 名称 + key (mono) + 状态 pill
- desc: 描述（最多 2 行 truncate）
- divider: 1px
- metrics row: 4 个等宽列（部门/岗位/Agent/已部署，mono 数字）
- foot: "查看部门" 链接 + 来源标签（SYSTEM）

样式：
- 边框/背景沿用 glass tokens
- hover: `translateY(-2px)` + border-color 变 amber
- isOpen: border 变 amber + box-shadow 强调

**DoD:**
- 组件替换现有实现
- 视觉与 [a-default.png](../../docs/design-experiments/industries/a-default.png) 一致
- `pnpm build` 通过

---

## 5. 新增 IndustryTableRow 组件

**Files:**
- Create: `web/src/components/industries/IndustryTableRow.vue`

- [ ] **5.1** 创建组件，props: `industry: Industry`, emits: `select`

模板：单行（用于 table 视图的 tbody）
- col 1: monogram + 名称 + key
- col 2: 描述
- col 3-5: 部门/岗位/Agent（mono 数字，右对齐）
- col 6: 已部署（mono 数字）
- col 7: 状态 pill
- col 8: 来源标签

样式：沿用 [registry-page.sass](../../web/src/css/theme/_registry-page.sass) table 模式（hover 灰底、1px 边、紧凑 13px）

**DoD:**
- 组件存在
- hover 状态正常
- emit `select` 触发上层打开 drawer

---

## 6. 新增 IndustryDrawer 组件

**Files:**
- Create: `web/src/components/industries/IndustryDrawer.vue`

- [ ] **6.1** 创建组件，props: `modelValue: boolean`, `industry: Industry | null`, `departments: Department[]`, emits: `update:modelValue`, `install`, `view-prompts`

模板：
- 背景遮罩（`rgba(44, 34, 24, 0.5)` + blur 3px）
- 右侧抽屉 480px 宽
- header: monogram + 名称 + key + 来源 + 关闭按钮
- body: 描述 + 3 mini-metric 卡 + "部门与岗位" section
- dept list: 每部门卡片 + 岗位行（dot + 名称 + P级 badge）
- footer: "查看 Prompt 模板"（ghost）+ "安装行业 →"（primary）

行为：
- 打开/关闭动画（slide-in 240ms cubic-bezier）
- `Esc` 键关闭
- 遮罩点击关闭

样式：沿用 [drawer-pattern.sass](../../web/src/css/theme/_drawer-pattern.sass)（如有）或独立 SCSS

**DoD:**
- 抽屉打开/关闭动画流畅
- 部门+岗位数据正确渲染
- `Esc` 与遮罩点击都关闭
- 三个 emit 正确触发

---

## 7. 重写 IndustryMarketPage 为编排层

**Files:**
- Modify: `web/src/pages/industries/IndustryMarketPage.vue`

- [ ] **7.1** 重写为编排层，引入 5 个新/重写子组件

模板结构：
```
<q-page>
  <AppPageHero title="行业模板库" subtitle="..." :actions="[...]" />
  <IndustryMetricStrip :summary="summary" />
  <IndustryMarketToolbar v-model="filters" :counts="counts" />
  <!-- 内容区：根据 view 切换 grid 或 table -->
  <IndustryCard v-for="ind in filtered" :industry="ind" :is-open="openKey === ind.key" @select="openDrawer" />
  <IndustryCtaCard />  <!-- 申请新行业，dashed 边框 -->
  <IndustryDrawer v-model="drawerOpen" :industry="active" :departments="depts" @install="..." />
</q-page>
```

`setup()` 中：
- `industries`, `summary` 来自 `useIndustryMarket`
- `filters` ref: `{ query, statusFilter, sourceFilter, view }`
- `filtered` computed
- `openKey` ref
- `drawerOpen` computed from `openKey`
- `depts` for active industry（独立 fetch，参考 `useIndustryDetail`）

**DoD:**
- 页面渲染与 [a-default.png](../../docs/design-experiments/industries/a-default.png) 一致
- 搜索/筛选/视图切换交互正常
- 卡片点击打开 drawer，drawer 关闭恢复
- `pnpm build` 通过

---

## 8. 新增 SCSS partial（可选 — 若现有 tokens 不够）

**Files:**
- Create: `web/src/css/theme/_industry-market.sass`
- Modify: `web/src/css/app-theme.sass`（仅在样式不够时引入 `@use`）

- [ ] **8.1** 仅在 IndustryCard / IndustryDrawer 需要 tokens 之外的自定义样式时新增

最小化新增：drawer 的 slideIn 动画、CTA 卡的 dashed 边框样式、metric 卡的内层 padding 调整

**DoD:**
- 引入 app-theme 后全站无样式泄漏
- dark 模式（如果启用）样式 fallback 正常

---

## 9. 补 i18n key

**Files:**
- Modify: `web/src/i18n/locales/zh-CN.ts`

- [ ] **9.1** 在 `industries` 命名空间下补 key

```ts
industries: {
  market: {
    title: '行业模板库',
    kicker: 'Industry Template Library',
    subtitle: '一个行业即一个完整场景包...',
    actions: {
      refresh: '刷新',
      export: '导出清单',
      requestNew: '申请新行业',
    },
    metrics: {
      enabled: '已启用行业',
      departments: '部门总数',
      positions: '岗位总数',
      agents: 'Agent 总数',
      enabledPerMonth: '本月新增',
      agentsPerPosition: '人均 Agent / 岗',
      installedTotal: '已部署实例',
    },
    filters: {
      searchPlaceholder: '搜索行业名称 / key / 描述…',
      statusAll: '全部',
      statusEnabled: '启用',
      statusDisabled: '停用',
      sourceAll: '全部',
      sourceSystem: '系统',
      sourceCustom: '自建',
      viewGrid: '网格',
      viewTable: '列表',
    },
    card: {
      viewDepts: '查看部门',
      ctaTitle: '申请新行业',
      ctaSubtitle: '描述你的业务场景，平台团队会评估并发布',
    },
    drawer: {
      close: '关闭',
      sectionDepts: '部门与岗位',
      actionViewPrompts: '查看 Prompt 模板',
      actionInstall: '安装行业',
    },
  },
}
```

- [ ] **9.2** 同步到 en-US.ts（结构对齐，文案英化）

**DoD:**
- 全站 grep 无 `T('industries.market.*')` 落空
- 中英文切换均渲染正常

---

## 10. 全量验证

- [ ] **10.1** 前端 lint

Run: `cd web && pnpm lint`
Expected: PASS

- [ ] **10.2** 前端 build

Run: `cd web && pnpm build`
Expected: PASS

- [ ] **10.3** 前端 typecheck

Run: `cd web && pnpm typecheck`（或 `vue-tsc --noEmit`）
Expected: PASS

- [ ] **10.4** 手动集成测试

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

- [ ] **10.5** Final commit

```bash
git add -A
git commit -m "feat(industries): redesign market page with metrics/toolbar/drawer (Direction A)"
```
