---
name: "aranea-frontend-guide"
description: "Aranea-Agents 项目前端统一编码指南。当在本项目编写 Vue 3/Quasar/Pinia/TypeScript 前端代码时自动触发，提供数据流铁律、分层规范、UX 主题、聊天消息分组、代码探索与验证的完整指导。"
---

# Aranea-Agents 前端统一编码指南

> **文档地位**：本项目前端编码的权威规范。`project_rules.md` 为索引 + 全局约束，详细规范只在本 SKILL 中；内容冲突时以 SKILL 为准。
> **通用 Vue 3 编程规范**：见 `vue-frontend-guide` SKILL（组件设计、Composable 模式、TypeScript 类型等）。
> **后端规范**：见 `aranea-coding-guide` SKILL，不在本文范围。

---

## 目录

- [第一章：14 条红线](#第一章14-条红线)
- [第二章：决策树](#第二章决策树)
- [第三章：数据流与分层](#第三章数据流与分层)
- [第四章：各层编码规范](#第四章各层编码规范)
- [第五章：聊天消息分组规范](#第五章聊天消息分组规范)
- [第六章：UX 主题规范](#第六章ux-主题规范)
- [第七章：Dialog 毛玻璃规范](#第七章dialog-毛玻璃规范)
- [第八章：Registry 列表表格规范](#第八章registry-列表表格规范)
- [第九章：任务速查卡](#第九章任务速查卡)
- [第十章：AI 编码自检清单](#第十章ai-编码自检清单)
- [第十一章：验证命令](#第十一章验证命令)
- [第十二章：模块关联强制检查](#第十二章模块关联强制检查)
- [第十三章：编程规范](#第十三章编程规范)

---

## 第一章：14 条红线

> 违反即停，不可绕过。

| # | 红线 | 正确做法 |
|---|------|----------|
| 1 | 展示组件（`components/**/*.vue`）不得 import `useXxxStore` / `defineStore` | 状态与请求收敛在 Store |
| 2 | 展示组件不得 import `features/*/api`、`services/`、`axios`、`kratosApi`、`create*Service()` | 网络请求只在 Store action 中触发 |
| 3 | 展示组件不得 `watch` + fetch + `ref` 存跨组件共享的业务数据 | 应进 Store |
| 4 | Dialog / Drawer / 浮层组件不得在组件内直接调 API | `emit('submit', payload)`，由 Page 或 Store action 调 API |
| 5 | 展示组件 `.vue` 必须放在 `components/<域>/`，禁止放在 `features/<域>/` | `features/<域>/` 只放 api.ts、composable、容器组件 |
| 6 | 新 Store 必须在 `stores/index.ts` 具名导出，不得删除 default export Pinia 工厂 → **编程规范 CS-F1** | 保持 Quasar Pinia 安装方式一致 |
| 7 | 新 HTTP 调用必须写在 `features/<域>/api.ts`，经 `services/index.ts` 的 `create*Service()` 或 `kratosApi` | 禁止在 `.vue` 中写裸 URL 或散装 `axios` |
| 8 | 浮层视觉必须遵守 UX 规范：`backdrop-filter` + `-webkit-backdrop-filter` 成对；主按钮用 `--color-accent` → **编程规范 CS-F2** | 禁止日间用夜间霓虹青紫作默认强调 |
| 9 | 禁止运行时用脚本改 `quasar-variables` | 昼夜仅用 Dark + CSS 变量 + body 选择器 |
| 10 | 禁止与 `app-global.sass` 平行的第二套全局 CSS 入口 | 新 token 只在 `theme/` 增加 partial，由 `app-theme.sass` 聚合 |
| 11 | **Page**（`pages/**/*Page.vue`）不得直接 `import` `features/*/api` | 请求经 **Store action**；编排经 **`features/<域>/useXxx.ts`** composable |
| 12 | 展示组件从 `features/<域>/api.ts` **引类型**（含 re-export） | 共享类型放在 **`features/<域>/types.ts`**，组件只 import types |
| 13 | 单页 `*Page.vue` 的 `<script setup>` 不宜长期超过 **~200 行**（不含 import）→ **编程规范 CS-F3** | 拆 **Dialog 组件** + **域内 composable** + **子面板组件** |
| 14 | 前端禁止使用 `turn_index` 做消息分组，聊天消息分组必须使用堆栈模型 | `groupMessagesByTurn` 按 `role=user` 边界 + 时间顺序 |

> **降级说明**：红线 #6（Store 导出）→ CS-F1、#8（UX 视觉）→ CS-F2、#13（Page 行数）→ CS-F3 已降级为编程规范（见第十三章），因可通过 linter/编码约定约束，不属于架构边界违反。红线编号不变，但违反级别从"阻断"降为"建议"。

---

## 第二章：决策树

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
├─ 新增 Dialog 毛玻璃 / 宽度 ───→ css/theme/_glass-dialog.sass
│
└─ 新增全局样式类 ──────────────→ css/app-global.sass
```

---

## 第三章：数据流与分层

### 3.1 唯一合法数据流

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

### 3.2 目录映射

| 层级 | 路径 | 职责 |
|------|------|------|
| HTTP 纯封装 | `features/<域>/api.ts`、`services/index.ts` | 只做请求与类型映射，不持有业务 loading |
| Store | `stores/<域>/index.ts`，经 `stores/index.ts` 具名导出 | 领域状态 + actions |
| Composable | `composables/useXxx.ts`（跨域）或 `features/<域>/useXxx.ts`（域内） | 暴露给 Page 的薄 API |
| 展示组件 | `components/<域>/**/*.vue` | props / emits |
| 页面 | `pages/**/*Page.vue` | 组合布局与 Composable |
| Feature | `features/<域>/` | api.ts、域内 composable、容器组件（须白名单） |

### 3.3 路径硬性规则

1. 展示组件 `.vue` **必须**放在 `components/<域>/`，**禁止**放在 `features/<域>/` 或 `pages/`
2. `features/<域>/` 只放：api.ts、域内 composable、容器组件（须首行注释 `// Container: approved because …`）
3. Dialog / Drawer / 浮层默认视为展示组件，同路径规则；`emit('submit', payload)` 由 Page / Store action 调 API
4. 与展示子组件紧耦合的纯函数/常量可共址为 `components/<域>/*.ts` 或 `features/<域>/*Ui.ts`；展示组件 **type-only** import 优先 `features/<域>/types.ts`
5. **Page** 与 **展示组件** 均不得直接调用 `features/<域>/api`；Page 通过 Store 或域内 composable 间接访问

### 3.4 页面瘦身与拆分

当 `*Page.vue` 同时承担「列表 + 侧栏/详情 + 多个 Dialog + 轮询/确认框」时，按下列顺序拆分：

| 拆出物 | 路径 | 职责 |
|--------|------|------|
| Store | `stores/<域>/index.ts` | 列表/详情状态、`loadXxx` / `saveXxx` |
| 域内 composable | `features/<域>/useXxxPage.ts` 或 `useXxxPanel.ts` | 筛选、选中项、Dialog 开关、确认框、`$q.notify` |
| 子面板 | `components/<域>/*Panel.vue` | 单 Tab/单区块展示，props/emits |
| Dialog | `components/<域>/*Dialog.vue` | 表单 UI，`v-model:open` + `emit('submit')` |
| 详情抽屉内容 | `components/<域>/*Content.vue` | 多 Tab 详情，数据由 Page composable 注入 |

**目标形态**：

```
Page.vue          ← import composable + storeToRefs；模板只做布局与事件绑定
  ├─ *Dialog.vue  ← 纯展示
  ├─ *Panel.vue   ← 纯展示
  └─ composable   ← 调 Store；不放在 .vue 里 watch+fetch
```

**反例（已修复，禁止回退）**：
- Page 内 `import { listXxx } from '../features/.../api'`
- Panel 内 `useToolsStore()` + `fetchCatalog`
- 单文件 Page >300 行且含完整 CRUD Dialog 模板

---

## 第四章：各层编码规范

### 4.1 API / Service 层

| 规则 | ✅ 正确 | ❌ 错误 |
|------|---------|---------|
| 一个函数对应一个后端能力 | `fetchAgent(id: string)` | 一个函数做 CRUD 全部 |
| Kratos 调用 | `import { createXxxService } from "../../services"` | 在 .vue 中直接调 |
| 过渡旧前缀 | `features/<域>/api.ts` 或 `legacyRest.ts` 收口 | 在 .vue 写裸 URL |
| 禁止 | — | 读 `useRoute`、改 Pinia、`$q.notify` |

### 4.2 Store 层

| 规则 | ✅ 正确 | ❌ 错误 |
|------|---------|---------|
| 按域拆分 | `stores/agents/`、`stores/avatar/` | 单文件持续增长 |
| 异步/错误/列表重置 | 放在 actions | 在组件中散装处理 |
| 对外暴露 | 清晰的 `loadXxx` / `saveXxx` | 外部随意 patch |
| 新 Store | `stores/index.ts` 具名导出 + 保留 default export | 删除 Pinia 工厂 |

### 4.3 Composable 层

| 规则 | ✅ 正确 | ❌ 错误 |
|------|---------|---------|
| 命名 | `use` 前缀 | 无前缀 |
| 返回 | `ref`/`computed`/方法 | 返回 Store 实例 |
| 默认依赖 | 只依赖 Store | 直接调 Service（须标技术债） |
| 技术债标注 | `// TECH-DEBT: direct API call; move to store — issue #xxx` | 无标注 |

### 4.4 展示组件层

| 规则 | ✅ 正确 | ❌ 错误 |
|------|---------|---------|
| 路径 | `components/<域>/` | `features/<域>/` |
| 接口 | `defineProps` + `defineEmits` | 内部拉数据 |
| 计算属性 | 仅依赖 props 的 `computed` | 依赖 Store/API |
| 本地状态 | `expanded`、`tab` 等 UI 状态 | 承载业务真源 |
| 禁止 import | — | `useXxxStore`、`features/*/api`、`axios`、`kratosApi` |
| 类型 import | `import type { X } from '../../features/<域>/types'` | `from '.../api'`（即使仅类型） |

### 4.5 Page 层

| 规则 | ✅ 正确 | ❌ 错误 |
|------|---------|---------|
| 职责 | 布局 + `useRoute` + composable + 传参 + 处理 emits | 大段业务 if/else |
| 数据请求 | `storeToRefs` + `await store.loadXxx()` 或域内 composable | Page 内 `import { listXxx } from '../features/.../api'` |
| 理想行数 | `<script setup>` ≤~200 行 | 含完整 Dialog 表单 + 表格列定义 + 全部 CRUD |
| 子组件接线 | composable 返回的 `ref` 用解构后传给 props | 在模板写 `panel.foo.value` |
| Agent 设置类 Tab | Page 用 `AgentToolsSection`（容器内 `useAgentToolOverrides`） | Page 手写十几项 props 或 Panel 内 `useXxxStore()` |

### 4.6 域内 Composable（页面编排）

| 规则 | ✅ 正确 | ❌ 错误 |
|------|---------|---------|
| 放置 | `features/<域>/useXxxPage.ts`、`useXxxPanel.ts`、`useToolEditor.ts` | 把编排逻辑留在 300+ 行 Page |
| 依赖 | `useXxxStore()`、`useQuasar()`、必要时 `useRoute` | 在 composable 中绕过 Store 直接 `features/*/api`（须标 TECH-DEBT） |
| 返回 | `ref` / `computed` / 方法；Page 解构后绑定模板 | 返回未解构的巨型对象并在模板深层 `.value` |
| Dialog 状态 | `createOpen`、`editorOpen` 等由 composable 持有 | Dialog 组件内 `watch` + fetch |
| 共享 UI 常量 | `features/<域>/knowledgeUi.ts`、`components/<域>/*TableUi.ts` | 在 Page 与 Panel 各复制一份 `docColumns` |

**命名建议**：

- `useKnowledgePage` — 整页状态（集合列表、选中、入库/检索）
- `useToolDetailPanel(toolRef)` — 详情抽屉内 Tab（覆盖、调用记录、在线测试）
- `useToolEditor(onSaved)` — 新建/编辑 Dialog + JSON 校验
- `useAgentToolOverrides(agentId)` — 由 `AgentToolsSection` 容器调用；Panel 只展示

---

## 第五章：聊天消息分组规范

> 红线 #14 的详细说明。违反会导致消息闪烁、错位、归入错误轮次。

### 5.1 核心原则：堆栈式分组

聊天消息按时间顺序排列，`role=user` 消息开启新的 TurnBlock，后续非 user 消息归入当前 block。`turn_index` 是后端数据字段，前端不得用于分组决策。

**消息流示例**：

```
[user] 你好           ← 开新 block
[assistant] thinking  ← 归入当前 block
[tool] search()       ← 归入当前 block
[assistant] thinking  ← 归入当前 block
[assistant] 结果...   ← 归入当前 block
[user] 再问           ← 开新 block
[assistant] 回答      ← 归入当前 block
```

### 5.2 in-flight 消息生命周期

```
用户点击发送
  → 创建 pending-user-{uuid} 占位消息（role=user）
  → 输入框清空，占位消息立即显示
  → WS 发送 user_message
  → 服务端返回 text_delta → 创建 ws-stream-{sessionId}（role=assistant）
  → groupMessagesByTurn：pending-user 开新 block，ws-stream 归入同一 block
  → runner_completion → loadMessages 获取服务端持久化消息
  → mergeSessionMessages 用服务端消息替换占位消息（按 content 匹配）
```

### 5.3 必须遵守

1. `groupMessagesByTurn` 分组算法：按 `created_at` 排序 → `role=user` 开新 block → 后续消息归入当前 block
2. in-flight 消息（`pending-user-*`、`ws-stream-*`、`member-*`）排序在持久化消息之后
3. `mergeSessionMessages` 排序：持久化消息按 `created_at`，in-flight 消息排最后
4. `turn_index` 字段保留在 `Message` 类型中（后端需要），但前端代码不得读取或推算

### 5.4 禁止的模式

- ❌ `deriveTurnKey()` — 基于 `turn_index` 奇偶规则推算分组 key
- ❌ `inferAssistantStreamTurnIndex()` / `inferToolActivityTurnIndex()` — 推算 in-flight 消息的 turn_index
- ❌ `realignEphemeralTurnIndexes()` — 修正前端推算的 turn_index 值
- ❌ `nextUserTurnIndex()` — 扫描消息列表推算下一个 turn_index
- ❌ `activeRequestId` / `request_id` 关联 — 用 request_id 补偿 turn_index 分组缺陷
- ❌ `mergeSessionMessages` 排序中 `turn_index=0 → 9999` hack
- ❌ 任何读取 `message.turn_index` 做前端分组决策的代码

---

## 第六章：UX 主题规范

### 6.1 日夜双模核心原则

| 原则 | 规则 |
|------|------|
| 双模 | 间距、圆角、字体阶梯、布局结构日夜一致，仅换色与材质参数 |
| Quasar | `Dark.set()` → `body.body--dark`，样式分叉用 `body:not(.body--dark)` / `body.body--dark` |
| 日间强调 | 金盏花锚点 `#E9A23B`（悬停 `#D48C1A`）贯连主按钮、链接、`:focus-visible` |
| 夜间霓虹 | `#00E5FF` / `#A855F7` 仅用于交互焦点、强调边、动态渐变；日间不得使用 |

### 6.2 玻璃材质（所有浮层必须）

```css
background: var(--glass-surface);
backdrop-filter: blur(var(--glass-blur-default));
-webkit-backdrop-filter: blur(var(--glass-blur-default));
```

**禁止**：纯黑/纯白实线作玻璃边框；日间重 `box-shadow` 分层。

### 6.3 CSS 变量使用规则

| 规则 | 说明 |
|------|------|
| 页面/组件用 `var(--*)` | 勿硬编码 hex（除 UX 文档明确要求处） |
| 新 token | `theme/_css-vars-light.sass` / `_css-vars-dark.sass`，由 `app-theme.sass` 聚合 |
| 新布局/页面 class | `app-global.sass` |
| 主强调/链接/焦点 | 以 `--color-accent` 为准 |
| 禁止 | 运行时改 `quasar-variables`；第二套全局 CSS 入口 |

### 6.4 完整 CSS 变量 Token 表

**实现位置**：`web/src/css/theme/_css-vars-light.sass`（`:root`）、`_css-vars-dark.sass`（`body.body--dark`）；聚合入口 `app-theme.sass`。页面/组件用 `var(--*)`，勿硬编码 hex（除本文档明确要求处）。

#### 6.4.1 日间（`:root`）

| Token | 值 | 用途 |
|-------|-----|------|
| `--canvas-base` | `#FEFBF4` | 主画布底 |
| `--glass-surface` | `rgba(255, 253, 245, 0.65)` | 标准玻璃 |
| `--glass-surface-hover` | `rgba(255, 253, 245, 0.78)` | 玻璃悬停 |
| `--glass-blur-default` | `18px` | 与 surface 配对 |
| `--glass-blur-hover` | `20px` | 悬停略增 |
| `--glass-border` | `rgba(235, 220, 200, 0.7)` | 玻璃边 |
| `--glass-elevated` | `rgba(255, 255, 255, 0.72)` | 弹层 |
| `--glass-blur-elevated` | `24px` | 抬高 blur |
| `--color-accent` | `#E9A23B` | 主操作、链接、焦点 |
| `--color-accent-hover` | `#D48C1A` | 主操作悬停 |
| `--focus-ring-light` | `2px solid rgba(233, 162, 59, 0.45)` | `:focus-visible` |
| `--interaction-surface-hover` | `#FEF3E4` | 次级悬停衬底 |
| `--glass-inner-highlight` | `inset 0 1px 0 rgba(255, 255, 255, 0.45)` | 可选顶缘高光 |
| `--color-text-primary` | `#3A322C` | 主文案 |
| `--color-text-secondary` | `#8B7A6B` | 辅文案 |
| `--color-icon-muted` | `#B8A590` | 图标/线 |
| `--color-success` | `#4CAF7C` | 成功 |
| `--color-warning` | `#F09B54` | 警告 |
| `--color-danger` | `#E55C5C` | 危险 |
| `--nav-bg-light` | `rgba(255, 249, 236, 0.85)` | 日间顶栏底（+ blur） |

#### 6.4.2 夜间（`body.body--dark`）

| Token | 值 | 用途 |
|-------|-----|------|
| `--canvas-base` | `#090D14` | 主画布底 |
| `--glass-surface` | `rgba(18, 24, 34, 0.65)` | 标准玻璃 |
| `--glass-surface-hover` | `rgba(22, 28, 40, 0.75)` | 玻璃悬停 |
| `--glass-border` | `rgba(255, 255, 255, 0.08)` | 细边 |
| `--glass-border-hover` | `rgba(255, 255, 255, 0.16)` | 悬停边 |
| `--color-accent` | `#00E5FF` | 夜间语义强调 |
| `--color-accent-hover` | `#5aebff` | 悬停 |
| `--color-neon-cyan` | `#00E5FF` | 焦点/链接 |
| `--color-neon-violet` | `#A855F7` | 二级强调 |
| `--gradient-flow-border` | `linear-gradient(120deg, #00E5FF, #A855F7, #00E5FF)` | 流动边 |
| `--color-text-primary` | `#EBEBF0` | 主文案 |
| `--color-text-secondary` | `#9CA0B0` | 辅文案 |
| `--color-success` | `#3FE0A0` | 成功 |
| `--color-warning` | `#FFAF4D` | 警告 |
| `--color-danger` | `#FF5E7A` | 危险 |
| `--nav-bg-dark` | `rgba(9, 13, 20, 0.7)` | 顶栏底（+ blur 20px） |
| `--nav-divider-dark` | `rgba(255, 255, 255, 0.06)` | 栏底分割线 |

### 6.5 日间交互增强速查

| 场景 | 推荐处理 | 避免 |
|------|----------|------|
| 主操作 | 实心或高对比填充 `#E9A23B`，字 `#FFFFFF`，悬停 `#D48C1A` | 大区域渐变抢镜 |
| 链接 / 次要强调 | 字色或下划线用 `#E9A23B`，悬停加深为 `#D48C1A` | 夜间霓虹青紫 |
| `:focus-visible` | `outline` / `ring` 使用 `#E9A23B`（或 `rgba(233,162,59,0.45)` 2px），与背景对比足够 | 仅依赖浏览器默认蓝环 |
| 可点击玻璃卡片 | 悬停：`rgba(255,253,245,0.78)` + `blur(20px)`，边框可向 `rgba(235,220,200,0.85)` 过渡 | 突然加深阴影代替材质变化 |
| 图标按钮 | 默认 `#B8A590`，悬停/激活 `#E9A23B` 或 `#3A322C`（按层级） | 彩虹或霓虹描边 |

**夜间霓虹**：`#00E5FF` / `#A855F7` **仅用于**交互焦点、强调边、动态渐变，避免铺满界面；**日间不得**使用该组色作默认强调（避免昼夜调性串味）。

### 6.6 组件详细数值

#### 6.6.1 按钮

| 模式 | 背景 | 字/边 | 悬停 | 圆角 / 其它 |
|------|------|--------|------|-------------|
| 昼·主 | `#E9A23B` | `#fff` | `#D48C1A` | 圆角 `10px`；内边距 `10px 20px`；可按 `scale(0.98)` |
| 昼·次 | 透明 | 字 `#3A322C`，边 `1px solid #D0C0A8` | 底 `#FEF3E4` | — |
| 昼·玻璃次 | `rgba(255,253,245,0.5)` + blur | 边 `#EDE3D3` | 按 §6.5 玻璃交互 | — |
| 夜·主（霓虹） | `rgba(0,229,255,0.15)` | 边 `1px solid #00E5FF`，字 `#00E5FF` | alpha→0.25；`box-shadow: 0 0 16px rgba(0,229,255,0.3)` | — |
| 夜·次 | 玻璃材质 | 边 `rgba(255,255,255,0.1)`，字 `#EBEBF0` | — | — |
| 胶囊 CTA | — | — | — | 圆角 `18px–980px` 按需 |

#### 6.6.2 卡片

| 模式 | 规则 |
|------|------|
| 昼·玻璃（默认浮层） | `rgba(255,253,245,0.65)` + blur 18px；边 `rgba(235,220,200,0.7)`；**无**重投影 |
| 昼·实体（慎用） | 底 `#FFFDF5`；边 `#EDE3D3`；影 `0 2px 12px rgba(0,0,0,0.04)`；圆角 16–20px。**同一层级勿与玻璃混用** |
| 夜 | `rgba(18,24,34,0.65)` + blur 18px + webkit；边 `rgba(255,255,255,0.08)`；悬停边 → `--glass-border-hover`；选中可 `box-shadow: 0 0 20px rgba(0,229,255,0.15)` |

#### 6.6.3 对话框（`q-dialog` 内主卡片）

- 背景 `var(--glass-elevated)`，`backdrop-filter` + `-webkit-backdrop-filter` 使用 `var(--glass-blur-elevated)`；边框 `var(--glass-border)`；圆角 20–24px
- 主按钮：优先 `var(--color-accent)` / `var(--color-accent-hover)`（随昼夜 token 切换），**禁止**在日间把霓虹青紫当默认主色

#### 6.6.4 输入

| 模式 | 规则 |
|------|------|
| 昼·实体 | 底 `#fff`；边 `#D0C0A8`；聚焦边 `#E9A23B`；字 `#3A322C` |
| 昼·玻璃 | 底 `rgba(255,255,255,0.5)` blur 8px；边 `rgba(208,192,168,0.6)`；聚焦 `#E9A23B` |
| 夜 | 底 `rgba(18,24,34,0.45)` blur 8px；边 `rgba(255,255,255,0.1)`；聚焦 `#00E5FF` + 微光晕 |
| 圆角 | `12px–16px` |

#### 6.6.5 导航 / 工具条

| 模式 | 规则 |
|------|------|
| 昼 | 底 `rgba(255,249,236,0.85)` + blur；字暖棕系；可无下边线，或 `1px solid rgba(235,220,200,0.6)`；奶霜顶栏可用 `rgba(255,253,245,0.72)` + blur 20px |
| 夜 | 底 `rgba(9,13,20,0.7)` blur 20px；底分割 `1px`、`rgba(255,255,255,0.06)`；链接 `#EBEBF0`，悬停点亮 `#00E5FF` |

#### 6.6.6 媒体

昼：图贴在奶油底上。夜：可略压暗或加玻璃蒙层；产品图夜间仅**极弱**青辉，勿抢主体。

### 6.7 排版

| 项 | 值 |
|----|-----|
| 展示 | `SF Pro Display, Inter Tight, Helvetica Neue, sans-serif` |
| 正文 | `SF Pro Text, Inter, Helvetica Neue, sans-serif` |
| 字色 | 昼 `var(--color-text-primary)` / 夜同 token |
| 标题 | 负字距、偏紧行高 |
| 夜间标题（特殊模块可选） | `text-shadow: 0 0 12px rgba(0, 229, 255, 0.15)` |

字号阶梯（若项目另有全局阶梯文档）与之对齐；**切换昼夜不改阶梯与行高体系**。

### 6.8 夜间特效（可选）

| 用途 | 要点 |
|------|------|
| 流动边 | `border-image` 或背景渐变；色 `#00E5FF` ↔ `#A855F7` |
| 扫描光 | 大面积 Hero 伪元素慢速带；**不挡阅读** |
| 霓虹 | `filter: drop-shadow(0 0 8px #00E5FF)` 小面积 |
| 移动降级 | 流光改静态渐变；blur 降至 8–12px |

### 6.9 布局规范

| 项 | 规则 |
|----|------|
| 间距刻度 | `4, 8, 12, 16, 20, 24, 32, 48, 64` px；**昼夜同一套** |
| 圆角 | 控件 5–8px；卡片/面板 16–20px；大模块 28–36px；胶囊 56–980px；圆 `50%` |
| 密度 | 营销宽留白；数据页可更密 |
| 表单区宽度 | 单列表单 `max-width: 960px`（`.app-form-shell` / `.settings-grid`）；双栏能力卡 `1200px`（`.app-form-wide`） |
| 字段宽度 | 短字段 `320px`（`.app-field-sm`）；名称/下拉 `480px`（`.app-field-md`）；描述/Prompt `72ch`（`.app-field-long`） |
| 字段网格 | 多列配置用 `.app-form-field-grid`（`auto-fill, minmax(200px, 280px)`），禁止裸 `col-12` 拉满超宽屏 |
| 按钮 | 主操作 **auto width**，`.app-actions-bar` 右对齐；**禁止**配置页 `full-width` 主按钮（移动 `.app-btn-block-mobile` 除外） |
| Chat composer | `.chat-composer-inner` 限宽 `900px` 居中，与消息区对齐 |
| Dialog | `.app-dialog-card` + `--glass-elevated`；宽 `640–860px` |
| Z / 层级 | L0=`--canvas-base`；L1/L2 用玻璃不透明度与 blur 差、边框亮度区分；焦点昼 `#E9A23B`、夜霓虹边。**摒弃**靠重投影分层层级 |

### 6.10 交互安全规范

| 场景 | 要求 |
|------|------|
| 破坏性操作 | 删除、回滚、终止、清除等不可逆操作**必须**有 `$q.dialog` 二次确认 |
| 表单提交 | 必填字段**必须**有 `:rules` 前端校验；提交按钮**必须**有 `:disable` 绑定 |
| 编辑器关闭 | 有未保存变更时关闭**必须**弹出确认；使用 `persistent` + dirty 检测 |
| IME 输入 | Chat 发送须同时检查 `event.isComposing` 和 `event.keyCode === 229` |
| 收藏/置顶 | 单字段切换只 patch 该字段，不提交完整 form |
| 加载失败 | 不使用 `router.back()` 强制跳转；显示错误页 + 重试按钮 |
| 删除按钮 | 列表项删除按钮应 hover 时才显示（`opacity: 0 → 1`），不始终暴露 |

### 6.11 响应式

断点跟随项目全局配置。移动：blur **8–12px**；§6.8 动效降级静态。

### 6.12 Do / Don't

**Do**：全昼夜浮层磨砂玻璃；昼奶油 rgba(255,253,245,…)；夜深半透明 + 微光；强调仅交互锚点；层级靠模糊与边框。

**Don't**：昼默认大白实心不透明大块；默认重阴影堆层级；同层混实体与玻璃；玻璃上大色块糊满；移动端忽视性能（须降 blur / 简化光效）。

### 6.13 样式工程落点

| 层级 | 路径 | 职责 |
|------|------|------|
| 构建期 | `web/src/css/quasar-variables.sass` | `$primary` 等（Vite `sassVariables`）；**不**随 Dark 重算 |
| Token | `web/src/css/app-theme.sass` → `web/src/css/theme/*` | `$cream-*`、`:root` / `body.body--dark` |
| 全局类 | `web/src/css/app-global.sass` | 字体、shell、页面级 class；昼夜用 `body:not(.body--dark)` / `body.body--dark` |
| Quasar 链（约定） | `web/src/style.sass` → `web/src/css/style.sass` | 配置里为 `css: ['style.sass']`（相对 `src/`）。**业务样式只改 `css/`**；根文件仅一行 `@import`。`client-entry` 固定 `import 'src/css/style.sass'`。 |

**Token 膨胀**：仅在 `web/src/css/theme/` 增加 `_*.sass`，由 `app-theme.sass` 聚合；可按域拆文件，每文件可含并列的 `:root` 与 `body.body--dark` 块；**禁止**与 `app-global` 平行的第二套全局 CSS 入口。

### 6.14 聊天消息卡片

- **结构**：头像外置 → 元信息行（名/时间）→ **玻璃 prose 卡片**（Markdown）
- **助手气泡**：`--glass-elevated`（昼）/ `--glass-surface`（夜）；`1px var(--glass-border)`；**禁止**-success/positive 绿描边
- **用户气泡**：昼实心 `--color-accent`；夜半透明青边
- **Markdown**：`.chat-message-prose`；标题 `--color-text-heading`；链接 `--color-accent`
- **Quasar**：`q-chat-message` 设 `bg-color="transparent"`；全局覆盖 `.q-message-text` 背景
- **流式**：左侧 3px inset 强调条或轻脉动，**禁止**整卡绿色外框

### 6.15 反模式

- 粗纯色边框（尤其 `#4CAF7C` / Quasar `positive` 当消息底）
- 标题 24px+ 压在 14px 正文上
- 靛紫渐变当默认主色（与项目金盏花/青霓虹冲突）
- 日间青紫霓虹 glow
- 夜间把 `--color-accent` 铺满 Stepper 圆点、竖条标题、实心主按钮
- 深色底上用 `--color-surface-*` / `text-grey-7` 当正文色
- 弹窗内再嵌套「下一步」+ 底部「保存」双套导航

---

## 第七章：Dialog 毛玻璃规范

### 7.1 必用 class

| 场景 | `q-card` class |
|------|----------------|
| 所有标准弹窗 | `app-dialog-card` + 宽度修饰符 |
| 头 + 可滚动内容 + 底栏 | 再加 `app-glass-dialog` |

**禁止**在 Dialog 内单独写实色 `background: rgba(255,255,255,0.9+)` 或复制旧版毛玻璃规则。

### 7.2 推荐 DOM 结构

```vue
<q-dialog v-model="open" persistent>
  <q-card class="app-dialog-card app-dialog-card--md app-glass-dialog">
    <q-card-section class="app-glass-dialog__head row items-start justify-between no-wrap">
      <div class="col min-width-0">
        <div class="app-glass-dialog__title">标题</div>
        <div v-if="subtitle" class="app-glass-dialog__subtitle">副标题</div>
      </div>
      <q-btn flat dense round icon="close" v-close-popup />
    </q-card-section>
    <q-separator />
    <div class="app-glass-dialog__scroll">
      <q-card-section class="app-dialog-body app-glass-dialog__body">
        <!-- 表单 -->
      </q-card-section>
    </div>
    <q-separator />
    <q-card-actions align="right" class="app-actions-bar app-glass-dialog__actions">
      <!-- 取消 / 保存 -->
    </q-card-actions>
  </q-card>
</q-dialog>
```

### 7.3 宽度修饰符

- `--sm` 640px · `--md` 720px · `--lg` 880px · `--xl` 980px · `--900` 900px
- 小屏统一 `max-width: min(<width>, 94vw)`

### 7.4 内层面板

- 配置类内容优先 `app-musebot-*` 排版类；**不要**在 `app-glass-dialog` 内再套实色大卡片
- 区块提示可用 `app-dialog-section`（半透明嵌套玻璃）

### 7.5 CSS 变量（勿在组件内硬编码）

- `--app-dialog-shell-bg`：卡片外壳
- `--app-dialog-chrome-bg`：头 / 底栏
- `--app-dialog-body-bg`：滚动中区；**勿用 α≥0.5**

---

## 第八章：Registry 列表表格规范

### 8.1 三层架构

| 层 | 路径 | 职责 |
|----|------|------|
| 基础设施 | `web/src/features/ui/registryTableColumns.ts` | `registryCol`、`REGISTRY_COL_W`、`registryColActions` / `registryColEnabled` |
| 基础设施 | `web/src/features/ui/useResizableRegistryColumns.ts` | 表头拖拽 + localStorage |
| 基础设施 | `web/src/components/layout/AppRegistryTable.vue` | 统一 QTable 壳（flat、dense、分页隐藏、列拖拽） |
| 领域 UI | `components/<域>/*Ui.ts` 或 `features/<域>/*TableUi.ts` | `XXX_TABLE_COLUMNS` + 格式化函数 |
| 展示 | `components/<域>/*Table.vue` | 仅 `#body-cell-*` slots，**不写 columns 数组** |
| 样式 | `web/src/css/theme/_registry-page.sass` | `.app-registry-*` 单元格语义类、表格 chrome |

轻量只读表（无 loading / 分页）可用 `AppRegistryMarkupTable.vue`。

### 8.2 红线

1. **列定义不得写在 `*Table.vue` 或 Page script 内** — 放在 `components/<域>/*Ui.ts` 或 `features/<域>/*TableUi.ts`
2. **禁止**手写 `{ style: "width: …", headerStyle: "width: …" }` — 使用 `registryCol()` + `REGISTRY_COL_W`
3. **列表表必须用** `AppRegistryTable`（或只读小表用 `AppRegistryMarkupTable`），禁止裸 `q-table`
4. **单元格**优先 `.app-registry-cell-primary` / `__sub` / `__actions`；域样式用 `table-class` + `_registry-page.sass`

### 8.3 列 API

```typescript
import { REGISTRY_COL_W, registryCol, registryColActions, registryColEnabled } from "../../features/ui/registryTableColumns";

registryCol<Row>("name", "名称", "name", "left", REGISTRY_COL_W.name);
registryColEnabled<Row>();
registryColActions<Row>();
```

参数顺序：`name` → `label` → `field` → **`align`** → `width` → `extra?`

### 8.4 列宽 token（`REGISTRY_COL_W`）

| Token | 典型用途 |
|-------|----------|
| `name` / `nameWide` | 名称列（14% / 18%） |
| `desc` | 规则、描述（16%） |
| `status` / `category` / `time` | 状态、分类、时间 |
| `agent` / `session` | Agent、Session |
| `enabled` | Toggle 列（64px，居中） |
| `metric` / `narrow` | 数值、短标签（72px / 64px） |
| `actions` / `actionsWide` | 操作列（108px / 148px，右对齐） |

需要完整 CSS（如 `max-width`）时 width 传 `"16%; max-width: 168px"`。

### 8.5 对齐约定

| 列类型 | align |
|--------|-------|
| 名称、描述、时间 | `left` |
| Toggle、状态 badge | `center` |
| 操作按钮 | `right` |
| 纯数字 | `right`（推荐） |

自定义 `#body-cell-*` 内若用 `row items-center` 等 flex，列 `align` 可能不明显，需在 slot 内用 `justify-*` 微调。

### 8.6 Preset

- `registryColEnabled()` — 启用 Toggle 列
- `registryColActions(width?, label?, field?)` — 操作列 + `app-registry-col-actions` 类

### 8.7 表格组件用法

```vue
<AppRegistryTable
  table-class="tools-data-table"
  :columns="TOOL_TABLE_COLUMNS"
  column-persist-key="tools-table"
  hide-pagination
  :pagination="{ rowsPerPage: 0 }"
>
  <template #body-cell-name="props">
    <q-td :props="props">
      <div class="app-registry-cell-primary ellipsis">{{ props.row.display_name }}</div>
      <div class="app-registry-cell-sub ellipsis">{{ props.row.key }}</div>
    </q-td>
  </template>
</AppRegistryTable>
```

| Prop | 说明 |
|------|------|
| `table-class` | 域特有样式时注册到 `_registry-page.sass`（如 `.tools-data-table.q-table`） |
| `column-persist-key` | 同页多表必须唯一；拖拽宽度存 localStorage |
| `:shell="false"` | Dialog / 嵌套 panel 内由外层提供玻璃壳 |
| `:resizable="false"` | 小表 / Dialog 内只读表可关闭拖拽 |

### 8.8 单元格样式

优先使用全局语义类（`_registry-page.sass`）：

| 类名 | 用途 |
|------|------|
| `.app-registry-cell-primary` | 主文本 |
| `.app-registry-cell-sub` | 副文本（key、时间戳） |
| `.app-registry-cell-desc` | 多行描述 |
| `.app-registry-cell-actions` | 操作按钮组 |
| `.app-registry-chip-wrap` | tag / chip 容器 |
| `.app-registry-icon-btn` | 圆形 icon 按钮 |

域特有样式：`.{domain}-data-table__*`（BEM），写在 `_registry-page.sass` 对应块内，**勿**在 Table.vue scoped 里重复 thead/td padding。

### 8.9 文件命名

| 场景 | 导出 | 文件示例 |
|------|------|----------|
| 单表 | `TOOL_TABLE_COLUMNS` | `components/tools/toolUi.ts` |
| 多表同域 | `SKILL_TABLE_COLUMNS` / `SKILL_RUNS_TABLE_COLUMNS` | `components/skills/skillTableUi.ts` |
| Page composable | `CRON_TASK_TABLE_COLUMNS` | `features/cron/cronTableUi.ts` |
| 需动态 field | `buildAgentTableColumns(...)` | `components/agents/agentTableUi.ts` |

### 8.10 新表 Checklist

- [ ] 在 `*Ui.ts` / `*TableUi.ts` 定义 `XXX_TABLE_COLUMNS`（`registryCol` + `REGISTRY_COL_W`）
- [ ] `*Table.vue` 只用 `AppRegistryTable` + body-cell slots
- [ ] 设置 `column-persist-key`
- [ ] 单元格用 `.app-registry-cell-*`
- [ ] 有域样式时：`table-class` + `_registry-page.sass` 注册
- [ ] `cd web && pnpm build`

### 8.11 参考实现

| 场景 | 文件 |
|------|------|
| 标准 Registry 列表 | `PluginsTable.vue` + `pluginUi.ts` |
| 多 variant 列 | `HooksTable.vue` + `hookTableUi.ts` |
| Provider 宽表 | `ProviderModelsTable.vue` + `providerModelUi.ts` |
| 会话 sticky 操作列 | `sessionUi.ts`（`app-registry-col-actions`） |
| 基础设施源码 | `registryTableColumns.ts`、`AppRegistryTable.vue` |

---

## 第九章：任务速查卡

### 新增前端功能

```
1. features/<域>/api.ts              ← HTTP 门面（create*Service / kratosApi）
2. stores/<域>/index.ts              ← state + actions（调 api）
3. stores/index.ts                   ← 具名导出新 Store
4. features/<域>/useXxxPage.ts       ← 页面编排（组合 Store，可选）
5. components/<域>/*Dialog.vue       ← Dialog / Panel（props/emits）
6. pages/**/*Page.vue                ← 页面（布局 + composable + 传参）
7. 验证：pnpm lint && pnpm test && pnpm build
```

### 迁移旧代码

```
1. 画数据流：标出谁发请求、谁保存列表、谁被多页面读取
2. 抽 Service：请求逻辑挪到 features/<域>/api.ts
3. 建/扩展 Store：新增 action，把 ref 列表、loading 移入 state
4. 写/收窄 Composable：useXxx 只暴露 storeToRefs / 调 store action
5. 瘦 Page：删除散装请求，换 composable；超 ~200 行按 §3.4 拆 Dialog/Panel
6. 瘦组件：删除 Store/API import，改为 props
7. 回归：相关路由点一遍；检查无循环依赖
```

---

## 第十章：AI 编码自检清单

### 改动中（逐层检查）

- [ ] **已读模块交叉参考手册**：在 `openspec/specs/module-cross-reference-full.md` 中找到目标前端模块卡片，确认后端对应 Service/Proto/Store、跨 Store 依赖、事件消费
- [ ] 展示组件是否直接调用 API / Store？若有 → 已上收或已备案例外
- [ ] **Page** 是否直接 `import` `features/*/api`？若有 → 迁入 Store + composable
- [ ] 新网络请求是否只出现在 `features/*/api.ts` 或 `services/`，且由 Store action 触发？
- [ ] 同一数据是否在多组件重复 fetch？若是 → 已合并到 Store 单一数据源
- [ ] Page 是否仅组合 composable + 传参，无大段业务 if/else？**脚本是否 ≤~200 行**？
- [ ] 多 Dialog / 多 Tab 是否已拆为 `components/<域>/*Dialog.vue`、`*Panel.vue`？
- [ ] 组件类型是否从 `features/<域>/types.ts` 引入（而非 `api.ts`）？
- [ ] Agent 设置等业务 Panel 是否由 Page 注入 composable 数据（而非 Panel 内 useStore）？
- [ ] 新增 Store 是否已在 `stores/index.ts` 具名导出？未破坏 default export Pinia？
- [ ] 聊天消息分组是否使用堆栈模型（`role=user` 边界），未使用 `turn_index`？
- [ ] 浮层是否遵守 UX 规范（玻璃材质 + 双前缀 blur + `--color-accent`）？
- [ ] 日间是否未使用夜间霓虹青紫作默认强调？
- [ ] Dialog 是否使用 `app-dialog-card` + 宽度修饰符？
- [ ] Registry 表格是否使用 `AppRegistryTable` + `registryCol()`？
- [ ] Quasar Pinia 安装方式是否与现有仓库一致？

### 改动后（构建与验证）

- [ ] `pnpm lint` 通过
- [ ] `pnpm test` 通过
- [ ] `pnpm build` 通过
- [ ] 昼/夜各看一眼（或 `/dev/theme-preview`）
- [ ] 无红线违反
- [ ] **编程规范合规**：CS-F4 props 有类型、CS-F6 无 any 类型、CS-F7 事件命名 onXxx、CS-F8 技术债务已标记

---

## 第十一章：验证命令

| 改动类型 | 最小验证 |
|----------|----------|
| 仅组件样式 | `cd web && pnpm stylelint && pnpm build` |
| 仅 Store/Composable | `cd web && pnpm lint && pnpm test` |
| 新增页面/功能 | `cd web && pnpm lint && pnpm test && pnpm build` |
| **提交前（全量）** | `cd web && pnpm lint && pnpm test && pnpm build` |

---

## 附录：样式落点速查

| 内容 | 路径 |
|------|------|
| 新 token | `web/src/css/theme/_css-vars-*.sass` |
| 表单布局 / Dialog | `web/src/css/theme/_form-layout.sass` |
| Registry 表格列 / chrome | `web/src/css/theme/_registry-page.sass` |
| 页面/聊天全局类 | `web/src/css/app-global.sass` |
| 页面 Hero / 登录 / 代码块 | `web/src/css/theme/_page-patterns.sass` |
| Dialog 毛玻璃 | `web/src/css/theme/_glass-dialog.sass` |
| 布局/动画（仅本组件） | `web/src/components/**` scoped sass |

---

## 第十三章：编程规范

> 编程规范是编码质量的硬约束，可通过 linter/编码约定自动或半自动执行。违反不等于架构破坏，但影响代码质量和可维护性。完整维度检查清单见 `docs/review-dimension-checklists.md`。

| 编号 | 规范 | 约束方式 | 来源 |
|------|------|----------|------|
| CS-F1 | 新 Store 必须在 `stores/index.ts` 具名导出，不得删除 default export Pinia 工厂 | ESLint | 原红线 #6 |
| CS-F2 | 浮层视觉必须遵守 UX 规范：`backdrop-filter` 成对；主按钮用 `--color-accent` | 审查 | 原红线 #8 |
| CS-F3 | 单页 `*Page.vue` 的 `<script setup>` 不宜超过 ~200 行 | ESLint/审查 | 原红线 #13 |
| CS-F4 | 组件 props 必须有 TypeScript 类型定义（`defineProps<T>()`） | TypeScript | 新增 |
| CS-F5 | 核心业务 Store/composable 必须有单元测试 | CI 门槛 | 新增 |
| CS-F6 | 禁止 `any` 类型，必须用具体类型或泛型 | TypeScript strict | 新增 |
| CS-F7 | 事件处理器命名：`onXxx`（如 `onSubmit`、`onDelete`） | 审查 | 新增 |
| CS-F8 | 技术债务用 `// TECH-DEBT:` 标记，含 issue 编号 | linter/审查 | 新增 |

### 13.1 编程规范与红线的关系

| 维度 | 红线（架构边界） | 编程规范（编码质量） |
|------|-----------------|---------------------|
| 违反后果 | 数据流混乱/组件耦合/消息分组错误 | 代码质量下降/可维护性降低 |
| 检测方式 | 代码审查（人工） | linter/TypeScript（自动）+ 审查 |
| 修复优先级 | 🔴 阻断（必须修复） | 🟡 建议（推荐修复） |
| 示例 | 展示组件 import Store | Page 超过 200 行 |

### 13.2 维度检查清单引用

编码时按维度 A 面预防，详见 `docs/review-dimension-checklists.md`：
- 所有编码：维度 1（架构）、2（质量）、3（正确性）、8（错误处理）
- 涉及 DB/API：+ 维度 9（前端性能）、10（前端安全）
- 涉及 Store/composable：+ 维度 6（可测试性）、11（业务逻辑）
- 涉及跨模块：+ 维度 7（可维护性）、12（文档同步）

## 附录：代表文件（抄作业起点）

| 场景 | 文件 |
|------|------|
| Provider 表 + 编辑向导 | `ResourceManagerPage.vue`、`ProviderModelsTable.vue`、`providerModelUi.ts` |
| Registry 表格规范 | `PluginsTable.vue`、`registryTableColumns.ts` |
| 趋势弹窗 ECharts | `ProviderTrendDialog.vue`、`UsageTrendChart.vue` |
| Team 紧凑卡片 | `TeamCard.vue`、`_entity-pages.sass` |
| Dialog 毛玻璃 | `_glass-dialog.sass` |
| 侧栏 | `MainLayout.vue`、`_sidebar.sass` |

---

## 第十二章：模块关联强制检查

> **前端模块不是孤岛。改任何 Store/Page/组件前，必须先读关联文档。** 违反即停。

### 12.1 关联文档索引

| 文档 | 路径 | 定位 |
|------|------|------|
| **架构蓝图** | `openspec/specs/architecture-blueprint.md` | 前端全貌：36 个 Store、实时层、路由 |
| **模块交叉参考** | `openspec/specs/module-cross-reference-full.md` | §三·前端模块上下文卡 + §六·前后端对齐表 |

### 12.2 开发前强制步骤

```
步骤 1：确定前端域（chat/tools/agents/...）→ 读交叉参考 §三 对应前端模块卡片
步骤 2：检查「后端对应」→ 确认 Service/Proto/Store 对齐
步骤 3：检查「跨 Store 依赖」→ 是否需要 sessionSync 事件总线
步骤 4：检查「事件消费」→ 新增 WS 消息类型是否已在 dispatcher 注册
步骤 5：查 §六·前后端对齐表 → 确认后端改动是否需要前端同步
```

### 12.3 典型遗漏案例

| 遗漏 | 后果 |
|------|------|
| 后端新增了 proto 字段但前端 api.ts 没更新 | 前端取不到新字段 |
| 新增 Store 但没在 stores/index.ts 导出 | Quasar Pinia 安装不一致 |
| 跨 Store 直接 import 导致循环依赖 | 运行时 undefined |
| 新增 Envelope 类型但 dispatcher 没注册处理函数 | WS 消息被静默丢弃 |
| 展示组件 import 了 Store | 违反红线 #1 |
| Dialog 内直接调 API | 违反红线 #4 |
