---
name: aranea-frontend-ux
description: >-
  Aranea 前端 UI/UX 设计与实现。在用户要求美化界面、聊天卡片、玻璃材质、昼夜主题、
  或提及 UX/设计规范时使用。项目权威为 docs/frontend/UX.md，本 skill 在其上叠加
  通用产品设计原则（间距、层次、可读性、反模式）。
---

# Aranea 前端 UI/UX

## 权威来源（冲突时以此为准）

1. **`docs/frontend/UX.md`** — 奶油昼 / 玻璃夜 token、玻璃材质、禁止项
2. **`docs/guides/frontend-guide.md`** — 分层、样式落点（`theme/`、`app-global.sass`）
3. **`docs/README.md` §5** — 文档索引

**禁止**：为「好看」引入 UX.md 未允许的霓虹铺满、重投影、硬编码 hex（变量文件除外）、第二套全局 CSS。

## 设计前自检（来自 UI 设计最佳实践）

| 检查 | 要求 |
|------|------|
| 层次 | 用玻璃不透明度 + 边框亮度区分层级，不靠粗色描边 |
| 排版 | 标题阶梯克制（聊天 h1≤20px）；正文 14–15px；行高 1.55–1.75 |
| 色彩 | 中性底 + **一个**强调色（昼 `#E9A23B`，夜 `#00E5FF` 仅焦点/边） |
| 间距 | 8px 网格：气泡内边距 14–16px，段落间距 0.5–0.75em |
| 可扫读 | 长列表/工具表分组或缩进；表格用玻璃底而非饱和色块表头 |
| 状态 | 悬停：玻璃变亮 + 边略提亮；禁用重 box-shadow（昼） |
| 无障碍 | 对比度 WCAG AA；`:focus-visible` 用 `--color-accent` |

## 样式落点

| 内容 | 路径 |
|------|------|
| 新 token | `web/src/css/theme/_css-vars-*.sass` |
| 表单布局 / Dialog | `web/src/css/theme/_form-layout.sass`（`.app-form-*`、`.app-dialog-card`） |
| 页面/聊天全局类 | `web/src/css/app-global.sass`（`.chat-page` 等） |
| 页面 Hero / 登录 / 代码块 | `web/src/css/theme/_page-patterns.sass`（`.app-page-hero`、`.app-code-block`） |
| 布局/动画（仅本组件） | `web/src/components/**` scoped sass |
| 展示组件 | 无 Store/API；props/emits only |

## 聊天消息卡片（专项）

- **结构**：头像外置 → 元信息行（名/时间）→ **玻璃 prose 卡片**（Markdown）
- **助手气泡**：`--glass-elevated`（昼）/ `--glass-surface`（夜）；`1px var(--glass-border)`；**禁止**-success/positive 绿描边
- **用户气泡**：昼实心 `--color-accent`；夜半透明青边（UX §5.1 夜主按钮）
- **Markdown**：`.chat-message-prose`；标题 `--color-text-heading`；链接 `--color-accent`（昼）/ `--color-link`（夜）
- **Quasar**：`q-chat-message` 设 `bg-color="transparent"`；全局覆盖 `.q-message-text` 背景
- **流式**：左侧 3px inset 强调条或轻脉动，**禁止**整卡绿色外框

## 反模式（不要做）

- 粗纯色边框（尤其 `#4CAF7C` / Quasar `positive` 当消息底）
- 标题 24px+ 压在 14px 正文上
- 靛紫渐变当默认主色（与项目金盏花/青霓虹冲突）
- 日间青紫霓虹 glow
- 夜间把 `--color-accent` 铺满 Stepper 圆点、竖条标题、实心主按钮（霓虹感强、突兀）
- 深色底上用 `--color-surface-*` / `text-grey-7` 当正文色（发灰、发空）
- 弹窗内再嵌套「下一步」+ 底部「保存」双套导航

---

## 实体页 / Registry / Dialog 优化 Playbook（可复用）

> 沉淀自模型管理、Team、Channel、分类编辑、趋势弹窗等一轮排错与视觉优化。  
> 适用：列表空/错位、标签看不清、弹窗突兀、图表占位过大、侧栏/滚动条违和。

### A. 先查功能，再查样式

| 现象 | 优先怀疑 | 动作 |
|------|----------|------|
| 分页有总数、列表空白 | 模板用了组件但未 `import` | 对照 template ↔ script imports |
| 表头缺失或列对不齐 | Row 有 grid、Header 无；或列模板不一致 | 抽 `*ListHeader.vue` + `*Row.vue`，共用 `--*-col-template` |
| 深色模式字看不清 | token 用错层级 | 正文 `--color-text-heading`；标签 `--color-text-secondary`；**禁止** `--color-surface-*` 当文字色 |
| Quasar 组件「自带丑」 | 默认 primary/chip/progress 饱和 | 换自定义 span/bar，或 `color-mix(in srgb, var(--color-accent) N%, …)` 且 N≤35 |

### B. 高密度列表（Provider / Skill / Registry 表）

1. **Grid 表**：父级 `.xxx-table` 定义 `--xxx-col-template`；head/body 同一 `grid-template-columns`。
2. **表头**：独立组件 + 11px uppercase + `--color-text-tertiary`；与数据行同 min-width 防横向错位。
3. **Tag**：不用 `q-chip` 叠错误色；用 `.xxx-tag` + 细边框 + secondary/accent 文字。
4. **用量条**：自绘 4px 条（`height: 4px; border-radius: 999px`），不用 `q-linear-progress`（行高被撑爆、灰色轨道过粗）。
5. **样式落点**：页面级 → `_entity-pages.sass`；行组件 → scoped + 共用 class。

### C. Dialog / 多步向导

1. **壳**：`app-dialog-card app-dialog-card--lg|--xl`；标题 `--color-text-heading` + subtitle `--color-text-secondary`。
2. **避免 Quasar `q-stepper`**（四步中文标题易换行、圆点过亮）：改为顶部 **4 列 grid 向导**（`.provider-wizard-nav` 模式）：
   - 短主标题 + 副标题 caption；`is-active` 仅 `accent 7–22%` 玻璃底 + 细边框
   - 步骤区 `v-show` 切换，保留表单 state
3. **滚动**：仅 body 滚；`scrollbar-width: none` + `::-webkit-scrollbar { display: none }`
4. **操作栏**：底部唯一 `app-actions-bar` — 取消 | 上一步 | 下一步 | 保存；**不要**在 scroll 区内再放实心「下一步」
5. **按钮层级**：次要用 `flat`；主保存用玻璃底 + accent 边框（`.provider-dialog-save`），避免 `color="primary"` 大块实心
6. **表单**：`app-form-field-grid--2col`；字段 `gap: 12px`；区块 hint 12px secondary
7. **样式落点**：`_form-layout.sass`（`.provider-wizard-*`、`.app-dialog-*`）

### D. 趋势 / 监控图表

1. **复用基建**：`features/usage/usageEcharts.ts` + `useUsageChart.ts`（勿手写巨大占位 div/圆角柱）。
2. **布局**：上 KPI 条（4 列紧凑卡片）→ 中 Chart（240px 高）→ 下 detail grid（3×2）；指标格式化（TPS 1 位小数、Y 轴 k/M）。
3. **夜图风格**：`baseChartOption` + 虚线 grid；area 渐变 `rgba(0,229,255,0.28→0)`；tooltip 深底细青边；**禁止**全屏 pill 条占满 modal。
4. **参考实现**：`UsageTrendChart.vue`、`ProviderTrendDialog.vue`

### E. 卡片列表（Team 等）

- 限制卡片 max-width（如 grid `minmax(…, 380px)`）；内容单行 ellipsis；去掉双层 nested card 造成的「空且高」。
- 成员/统计合并一行；padding 10–12px 级。

### F. 侧栏 / 壳层

- Mini 宽度与图标区：`--sidebar-mini-width`（如 80px）；图标与 label 用 flex gap，勿靠负 margin。
- 隐藏滚动条：`div.app-sidebar__scroll` + `scrollbar-width: none`，勿在侧栏用显眼 `q-scroll-area` 默认条。

### G. 强调色用量（防「突兀」）

| 用途 | 推荐 |
|------|------|
| 当前步骤/选中 | `color-mix(accent 7–22%, glass-elevated)` + `glass-border` |
| 标题左边条 | **避免** 3px 实心 accent；改用字号/字重分层 |
| Toggle 开 | `color-mix(accent 68%, glass-surface)` 或默认 grey |
| 主按钮 | 玻璃底 + accent 28% 边框；hover 略提亮 |
| 图表线/点 | accent / series[2]；area 透明度 ≤0.35 |

### H. 改完检查清单

```bash
cd web && pnpm build
```

- [ ] 昼/夜各看一眼（或 `/dev/theme-preview`）
- [ ] 列表：表头与行 column 对齐；空态、loading、有数据三态
- [ ] Dialog：1024 / 768 宽步骤 nav 不换行（或 2×2 退化）
- [ ] 无新增硬编码 hex；新样式在 `theme/` 而非散落 scoped 大段重复

### I. 代表文件（抄作业起点）

| 场景 | 文件 |
|------|------|
| Provider 表 + 编辑向导 | `ResourceManagerPage.vue`、`ProviderModelRow.vue`、`ProviderModelListHeader.vue`、`_form-layout.sass` |
| 趋势弹窗 ECharts | `ProviderTrendDialog.vue`、`UsageTrendChart.vue` |
| Team 紧凑卡片 | `TeamCard.vue`、`_entity-pages.sass` |
| 分类编辑弹窗 | `AgentCategoriesPage.vue`（`.category-dialog*`） |
| 侧栏 | `MainLayout.vue`、`_sidebar.sass` |

## 参考 skill

通用组件模式见项目内 `.cursor/skills/ui-design-brain/`（上游：[ui-design-brain](https://github.com/carmahhawwari/ui-design-brain)）。

## 完成验证

```bash
cd web && pnpm run build
```

开发环境主题验收页：`http://localhost:9001/dev/theme-preview`（仅 `import.meta.env.DEV` 注册路由）。
