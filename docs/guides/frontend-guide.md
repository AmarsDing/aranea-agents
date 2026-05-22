# Aranea-Agents AI 前端开发规范

> **文档地位**：AI 编码时的**唯一前端行为准则**（Vue 3 / Quasar / Pinia / TypeScript）。
> **规范冲突优先级**：本文 > `frontend/` 下所有参考文档。
> **后端规范**：见 [AI-DEVELOPMENT-SPECIFICATION.md](./AI-DEVELOPMENT-SPECIFICATION.md)，不在本文范围。
> **阅读方式**：先看「速查卡」掌握红线与决策路径，再按需翻阅详细规范。

---

## 目录

- [速查卡](#速查卡)
  - [红线（违反即停）](#红线违反即停)
  - [决策树（代码该放哪？）](#决策树代码该放哪)
  - [任务速查卡](#任务速查卡)
- [第一章：数据流与分层](#第一章数据流与分层)
  - [1.4 页面瘦身与拆分（评审沉淀）](#14-页面瘦身与拆分评审沉淀)
- [第二章：各层编码规范](#第二章各层编码规范)
- [第三章：UX 主题规范](#第三章ux-主题规范)
- [第四章：迁移剧本](#第四章迁移剧本)
- [第五章：AI 编码自检清单](#第五章ai-编码自检清单)

---

## 速查卡

> AI 每次前端编码前**必须**先查阅本节。

### 红线（违反即停）

| # | 红线 | 正确做法 |
|---|------|----------|
| 1 | 展示组件（`components/**/*.vue`）不得 import `useXxxStore` / `defineStore` | 状态与请求收敛在 Store |
| 2 | 展示组件不得 import `features/*/api`、`services/`、`axios`、`kratosApi`、`create*Service()` | 网络请求只在 Store action 中触发 |
| 3 | 展示组件不得 `watch` + fetch + `ref` 存跨组件共享的业务数据 | 应进 Store |
| 4 | Dialog / Drawer / 浮层组件不得在组件内直接调 API | `emit('submit', payload)`，由 Page 或 Store action 调 API |
| 5 | 展示组件 `.vue` 必须放在 `components/<域>/`，禁止放在 `features/<域>/` | `features/<域>/` 只放 api.ts、composable、容器组件 |
| 6 | 新 Store 必须在 `stores/index.ts` 具名导出，不得删除 default export Pinia 工厂 | 保持 Quasar Pinia 安装方式一致 |
| 7 | 新 HTTP 调用必须写在 `features/<域>/api.ts`，经 `services/index.ts` 的 `create*Service()` 或 `kratosApi` | 禁止在 `.vue` 中写裸 URL 或散装 `axios` |
| 8 | 浮层视觉必须遵守 UX 规范：`backdrop-filter` + `-webkit-backdrop-filter` 成对；主按钮用 `--color-accent` | 禁止日间用夜间霓虹青紫作默认强调 |
| 9 | 禁止运行时用脚本改 `quasar-variables` | 昼夜仅用 Dark + CSS 变量 + body 选择器 |
| 10 | 禁止与 `app-global.sass` 平行的第二套全局 CSS 入口 | 新 token 只在 `theme/` 增加 partial，由 `app-theme.sass` 聚合 |
| 11 | **Page**（`pages/**/*Page.vue`）不得直接 `import` `features/*/api` | 请求经 **Store action**；编排经 **`features/<域>/useXxx.ts`** composable |
| 12 | 展示组件从 `features/<域>/api.ts` **引类型**（含 re-export） | 共享类型放在 **`features/<域>/types.ts`**，组件只 import types |
| 13 | 单页 `*Page.vue` 的 `<script setup>` 不宜长期超过 **~200 行**（不含 import） | 拆 **Dialog 组件** + **域内 composable** + **子面板组件** |

### 决策树（代码该放哪？）

```
你要做什么？
│
├─ 新增 HTTP 请求 ──────────────→ features/<域>/api.ts（经 services/ 或 kratosApi）
│
├─ 新增业务状态/加载/错误 ──────→ stores/<域>/index.ts（action 内调 api）
│
├─ 多页面复用同一套逻辑 ────────→ composables/useXxx.ts（组合 Store）
│
├─ 单页编排过重（列表+详情+多 Dialog）→ features/<域>/useXxxPage.ts 或 useXxxPanel.ts
│
├─ 新增 Dialog / 浮层（仅 UI）────→ components/<域>/*Dialog.vue（props/emits）
│
├─ 新增展示组件 ────────────────→ components/<域>/*.vue（仅 props/emits）
│
├─ 新增页面 ────────────────────→ pages/**/*Page.vue（布局 + composable + 传参）
│
├─ 新增 CSS 变量 ───────────────→ css/theme/_css-vars-light.sass + _css-vars-dark.sass
│
├─ 新增 Dialog 毛玻璃 / 宽度 ───→ css/theme/_glass-dialog.sass（`.cursor/rules/glass-dialog.mdc`）
└─ 新增全局样式类 ──────────────→ css/app-global.sass
```

### 任务速查卡

#### 新增前端功能

```
1. features/<域>/api.ts              ← HTTP 门面（create*Service / kratosApi）
2. stores/<域>/index.ts              ← state + actions（调 api）
3. stores/index.ts                   ← 具名导出新 Store
4. features/<域>/useXxxPage.ts       ← 页面编排（组合 Store，可选）
5. components/<域>/*Dialog.vue       ← Dialog / Panel（props/emits）
6. pages/**/*Page.vue                ← 页面（布局 + composable + 传参）
7. 验证：pnpm lint && pnpm test && pnpm build
```

#### 迁移旧代码

```
1. 画数据流：标出谁发请求、谁保存列表、谁被多页面读取
2. 抽 Service：请求逻辑挪到 features/<域>/api.ts
3. 建/扩展 Store：新增 action，把 ref 列表、loading 移入 state
4. 写/收窄 Composable：useXxx 只暴露 storeToRefs / 调 store action
5. 瘦 Page：删除散装请求，换 composable；超 ~200 行按 §1.4 拆 Dialog/Panel
6. 瘦组件：删除 Store/API import，改为 props
7. 回归：相关路由点一遍；检查无循环依赖
```

---

## 第一章：数据流与分层

### 1.1 唯一合法数据流

```
API / Service（纯函数，只谈 HTTP 与类型）
        ↓ 仅能被 Store actions 调用
Pinia Store（状态 + 业务过程 + loading/error）
        ↓ 仅能被 Composable / Page 读取与触发 action
Composable（为页面打包响应式 API，可组合多 Store）
        ↓
Page（布局、路由、把数据交给子组件）
        ↓ props
Component（仅展示：props in / emits out）
```

**用词约定**：

- **展示组件**：`components/**` 下、以渲染与交互表象为主的 `.vue` 文件；禁止依赖 Pinia、禁止调用业务 API。
- **页面**：`pages/**` 下的 `*Page.vue`；可调用 Composable，禁止长串散装 `await fetch`。

### 1.2 目录映射

| 层级 | 路径 | 职责 |
|------|------|------|
| HTTP 纯封装 | `features/<域>/api.ts`、`services/index.ts` | 只做请求与类型映射，不持有业务 loading |
| Store | `stores/<域>/index.ts`，经 `stores/index.ts` 具名导出 | 领域状态 + actions |
| Composable | `composables/useXxx.ts`（跨域）或 `features/<域>/useXxx.ts`（域内） | 暴露给 Page 的薄 API |
| 展示组件 | `components/<域>/**/*.vue` | props / emits |
| 页面 | `pages/**/*Page.vue` | 组合布局与 Composable |
| Feature | `features/<域>/` | api.ts、域内 composable、容器组件（须白名单） |

### 1.3 路径硬性规则

1. 展示组件 `.vue` **必须**放在 `components/<域>/`，**禁止**放在 `features/<域>/` 或 `pages/`
2. `features/<域>/` 只放：api.ts、域内 composable、容器组件（须首行注释 `// Container: approved because …`）
3. Dialog / Drawer / 浮层默认视为展示组件，同路径规则；`emit('submit', payload)` 由 Page / Store action 调 API
4. 与展示子组件紧耦合的纯函数/常量可共址为 `components/<域>/*.ts` 或 `features/<域>/*Ui.ts`；展示组件 **type-only** import 优先 `features/<域>/types.ts`
5. **Page** 与 **展示组件** 均不得直接调用 `features/<域>/api`；Page 通过 Store 或域内 composable 间接访问

### 1.4 页面瘦身与拆分（评审沉淀）

当 `*Page.vue` 同时承担「列表 + 侧栏/详情 + 多个 Dialog + 轮询/确认框」时，按下列顺序拆分，避免 AI 生成「巨型 Page + 组件内调 Store」：

| 拆出物 | 路径 | 职责 | 仓库示例 |
|--------|------|------|----------|
| Store | `stores/<域>/index.ts` | 列表/详情状态、`loadXxx` / `saveXxx` | `stores/tools`、`stores/knowledge` |
| 域内 composable | `features/<域>/useXxxPage.ts` 或 `useXxxPanel.ts` | 筛选、选中项、Dialog 开关、确认框、`$q.notify` | `useKnowledgePage`、`useToolEditor`、`useAgentToolOverrides` |
| 子面板 | `components/<域>/*Panel.vue` | 单 Tab/单区块展示，props/emits | `KnowledgeDocumentsPanel`、`KnowledgeSearchPanel` |
| Dialog | `components/<域>/*Dialog.vue` | 表单 UI，`v-model:open` + `emit('submit')` | `ToolEditorDialog`、`EvaluationCreateDialog` |
| 详情抽屉内容 | `components/<域>/*Content.vue` | 多 Tab 详情，数据由 Page composable 注入 | `ToolDetailContent` |

**目标形态**（以 Tools / Knowledge / Agent 设置为准绳）：

```
Page.vue          ← import composable + storeToRefs；模板只做布局与事件绑定
  ├─ *Dialog.vue  ← 纯展示
  ├─ *Panel.vue   ← 纯展示
  └─ composable   ← 调 Store；不放在 .vue 里 watch+fetch
```

**反例（评审中已修复，禁止回退）**：

- `EvaluationPage` / `KnowledgePage` 在 Page 内 `import { listXxx } from '../features/.../api'`
- `AgentToolOverridesPanel` 内 `useToolsStore()` + `fetchCatalog`
- `ToolDetailContent` 内 `testTool()` 与 `useToolsStore`
- 单文件 Page >300 行且含完整 CRUD Dialog 模板

---

## 第二章：各层编码规范

### 2.1 API / Service 层

| 规则 | ✅ 正确 | ❌ 错误 |
|------|---------|---------|
| 一个函数对应一个后端能力 | `fetchAgent(id: string)` | 一个函数做 CRUD 全部 |
| Kratos 调用 | `import { createXxxService } from "../../services"` | 在 .vue 中直接调 |
| 过渡旧前缀 | `features/<域>/api.ts` 或 `legacyRest.ts` 收口 | 在 .vue 写裸 URL |
| 禁止 | — | 读 `useRoute`、改 Pinia、`$q.notify` |

### 2.2 Store 层

| 规则 | ✅ 正确 | ❌ 错误 |
|------|---------|---------|
| 按域拆分 | `stores/agents/`、`stores/avatar/` | 单文件持续增长 |
| 异步/错误/列表重置 | 放在 actions | 在组件中散装处理 |
| 对外暴露 | 清晰的 `loadXxx` / `saveXxx` | 外部随意 patch |
| 新 Store | `stores/index.ts` 具名导出 + 保留 default export | 删除 Pinia 工厂 |

### 2.3 Composable 层

| 规则 | ✅ 正确 | ❌ 错误 |
|------|---------|---------|
| 命名 | `use` 前缀 | 无前缀 |
| 返回 | `ref`/`computed`/方法 | 返回 Store 实例 |
| 默认依赖 | 只依赖 Store | 直接调 Service（须标技术债） |
| 技术债标注 | `// TECH-DEBT: direct API call; move to store — issue #xxx` | 无标注 |

### 2.4 展示组件层

| 规则 | ✅ 正确 | ❌ 错误 |
|------|---------|---------|
| 路径 | `components/<域>/` | `features/<域>/` |
| 接口 | `defineProps` + `defineEmits` | 内部拉数据 |
| 计算属性 | 仅依赖 props 的 `computed` | 依赖 Store/API |
| 本地状态 | `expanded`、`tab` 等 UI 状态 | 承载业务真源 |
| 禁止 import | — | `useXxxStore`、`features/*/api`、`axios`、`kratosApi` |
| 类型 import | `import type { X } from '../../features/<域>/types'` | `from '.../api'`（即使仅类型） |

### 2.5 Page 层

| 规则 | ✅ 正确 | ❌ 错误 |
|------|---------|---------|
| 职责 | 布局 + `useRoute` + composable + 传参 + 处理 emits | 大段业务 if/else |
| 数据请求 | `storeToRefs` + `await store.loadXxx()` 或域内 composable | Page 内 `import { listXxx } from '../features/.../api'` |
| 理想行数 | `<script setup>` 以 import + composable 解构 + 少量绑定为主（**≤~200 行**） | 含完整 Dialog 表单 + 表格列定义 + 全部 CRUD |
| 子组件接线 | composable 返回的 `ref` 用解构后传给 props（模板自动解包） | 在模板写 `panel.foo.value` |
| Agent 设置类 Tab | Page 用 `AgentToolsSection`（容器内 `useAgentToolOverrides`） | Page 手写十几项 props 或 Panel 内 `useXxxStore()` |

### 2.6 域内 Composable（页面编排）

| 规则 | ✅ 正确 | ❌ 错误 |
|------|---------|---------|
| 放置 | `features/<域>/useXxxPage.ts`、`useXxxPanel.ts`、`useToolEditor.ts` | 把编排逻辑留在 300+ 行 Page |
| 依赖 | `useXxxStore()`、`useQuasar()`、必要时 `useRoute` | 在 composable 中绕过 Store 直接 `features/*/api`（须标 TECH-DEBT 并尽快迁入 Store） |
| 返回 | `ref` / `computed` / 方法；Page 解构后绑定模板 | 返回未解构的巨型对象并在模板深层 `.value` |
| Dialog 状态 | `createOpen`、`editorOpen` 等由 composable 持有 | Dialog 组件内 `watch` + fetch |
| 共享 UI 常量 | `features/<域>/knowledgeUi.ts`、`components/<域>/toolUi.ts` | 在 Page 与 Panel 各复制一份 `docColumns` |

**命名建议**：

- `useKnowledgePage` — 整页状态（集合列表、选中、入库/检索）
- `useToolDetailPanel(toolRef)` — 详情抽屉内 Tab（覆盖、调用记录、在线测试）
- `useToolEditor(onSaved)` — 新建/编辑 Dialog + JSON 校验
- `useAgentToolOverrides(agentId)` — 由 `AgentToolsSection` 容器调用；Panel 只展示

---

## 第三章：UX 主题规范

> 完整规范见 [frontend/UX.md](../frontend/UX.md)。本章为 AI 编码时的速查要点。

### 3.1 日夜双模核心原则

| 原则 | 规则 |
|------|------|
| 双模 | 间距、圆角、字体阶梯、布局结构日夜一致，仅换色与材质参数 |
| Quasar | `Dark.set()` → `body.body--dark`，样式分叉用 `body:not(.body--dark)` / `body.body--dark` |
| 日间强调 | 金盏花锚点 `#E9A23B`（悬停 `#D48C1A`）贯连主按钮、链接、`:focus-visible` |
| 夜间霓虹 | `#00E5FF` / `#A855F7` 仅用于交互焦点、强调边、动态渐变；日间不得使用 |

### 3.2 玻璃材质（所有浮层必须）

```css
background: var(--glass-surface);
backdrop-filter: blur(var(--glass-blur-default));
-webkit-backdrop-filter: blur(var(--glass-blur-default));
```

**禁止**：纯黑/纯白实线作玻璃边框；日间重 `box-shadow` 分层。

### 3.3 CSS 变量使用规则

| 规则 | 说明 |
|------|------|
| 页面/组件用 `var(--*)` | 勿硬编码 hex（除 UX 文档明确要求处） |
| 新 token | `theme/_css-vars-light.sass` / `_css-vars-dark.sass`，由 `app-theme.sass` 聚合 |
| 新布局/页面 class | `app-global.sass` |
| 主强调/链接/焦点 | 以 `--color-accent` 为准 |
| 禁止 | 运行时改 `quasar-variables`；第二套全局 CSS 入口 |

### 3.4 关键 Token 速查

| Token | 日间 | 夜间 |
|-------|------|------|
| `--canvas-base` | `#FEFBF4` | `#090D14` |
| `--glass-surface` | `rgba(255,253,245,0.65)` | `rgba(18,24,34,0.65)` |
| `--glass-border` | `rgba(235,220,200,0.7)` | `rgba(255,255,255,0.08)` |
| `--color-accent` | `#E9A23B` | `#00E5FF` |
| `--color-text-primary` | `#3A322C` | `#EBEBF0` |
| `--glass-elevated` | `rgba(255,255,255,0.72)` | — |
| `--glass-blur-default` | `18px` | `18px` |
| `--glass-blur-elevated` | `24px` | `24px` |

### 3.5 组件数值速查

| 组件 | 规则 |
|------|------|
| 按钮 | 圆角 10px；内边距 10px 20px；主按钮 `--color-accent` |
| 卡片 | 玻璃 `--glass-surface` + blur 18px；圆角 16-20px；无重投影 |
| 对话框 | `app-dialog-card` + 可选 `app-glass-dialog`（`theme/_glass-dialog.sass`）；见 `.cursor/rules/glass-dialog.mdc` |
| 输入 | 圆角 12-16px；聚焦边 `--color-accent` |
| 间距刻度 | 4, 8, 12, 16, 20, 24, 32, 48, 64 px（昼夜同一套） |
| 圆角阶梯 | 控件 5-8px；卡片 16-20px；大模块 28-36px；胶囊 56-980px |

---

## 第四章：迁移剧本

当 AI 或开发者接到「迁移旧代码」任务时，按顺序执行：

1. **画数据流**：标出当前「谁发起请求、谁保存列表、谁被多页面读取」
2. **抽 Service**：请求逻辑挪到 `features/<域>/api.ts`（Kratos 调用经 `services/index.ts`）
3. **建或扩展 Store**：新增 action `loadXxx`，把原 `ref` 列表、`loading` 移入 state
4. **写或收窄 Composable**：`useXxx` 只暴露 `storeToRefs` / 调 `store.loadXxx`
5. **瘦 Page**：删除散装请求，换 composable；超 ~200 行则按 [§1.4](#14-页面瘦身与拆分评审沉淀) 拆 Dialog/Panel
6. **瘦组件**：删除 Store/API import，改为 props；原 `emit` 由 Page 接住再调 composable/store
7. **回归**：相关路由点一遍；检查无循环依赖（Store 勿 import `.vue`）

---

## 第五章：AI 编码自检清单

做完功能或重构后，逐条勾选：

- [ ] 展示组件是否直接调用 API / Store？若有 → 已上收或已备案例外
- [ ] **Page** 是否直接 `import` `features/*/api`？若有 → 迁入 Store + composable
- [ ] 新网络请求是否只出现在 `features/*/api.ts` 或 `services/`，且由 Store action 触发？
- [ ] 同一数据是否在多组件重复 fetch？若是 → 已合并到 Store 单一数据源
- [ ] Page 是否仅组合 composable + 传参，无大段业务 if/else？**脚本是否 ≤~200 行**？
- [ ] 多 Dialog / 多 Tab 是否已拆为 `components/<域>/*Dialog.vue`、`*Panel.vue`？
- [ ] 组件类型是否从 `features/<域>/types.ts` 引入（而非 `api.ts`）？
- [ ] Agent 设置等业务 Panel 是否由 Page 注入 composable 数据（而非 Panel 内 useStore）？
- [ ] 新增 Store 是否已在 `stores/index.ts` 具名导出？未破坏 default export Pinia？
- [ ] 浮层是否遵守 UX 规范（玻璃材质 + 双前缀 blur + `--color-accent`）？
- [ ] 日间是否未使用夜间霓虹青紫作默认强调？
- [ ] Quasar Pinia 安装方式是否与现有仓库一致（避免双 Pinia）？
