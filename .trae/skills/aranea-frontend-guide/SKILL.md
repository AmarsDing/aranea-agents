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

### 6.4 关键 Token 速查

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

### 6.5 组件数值速查

| 组件 | 规则 |
|------|------|
| 按钮 | 圆角 10px；内边距 10px 20px；主按钮 `--color-accent` |
| 卡片 | 玻璃 `--glass-surface` + blur 18px；圆角 16-20px；无重投影 |
| 对话框 | `app-dialog-card` + 可选 `app-glass-dialog` |
| 输入 | 圆角 12-16px；聚焦边 `--color-accent` |
| 间距刻度 | 4, 8, 12, 16, 20, 24, 32, 48, 64 px（昼夜同一套） |
| 圆角阶梯 | 控件 5-8px；卡片 16-20px；大模块 28-36px；胶囊 56-980px |

### 6.6 聊天消息卡片

- **结构**：头像外置 → 元信息行（名/时间）→ **玻璃 prose 卡片**（Markdown）
- **助手气泡**：`--glass-elevated`（昼）/ `--glass-surface`（夜）；`1px var(--glass-border)`；**禁止**-success/positive 绿描边
- **用户气泡**：昼实心 `--color-accent`；夜半透明青边
- **Markdown**：`.chat-message-prose`；标题 `--color-text-heading`；链接 `--color-accent`
- **Quasar**：`q-chat-message` 设 `bg-color="transparent"`；全局覆盖 `.q-message-text` 背景
- **流式**：左侧 3px inset 强调条或轻脉动，**禁止**整卡绿色外框

### 6.7 反模式

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

### 8.1 红线

1. **列定义不得写在 `*Table.vue` 或 Page script 内** — 放在 `components/<域>/*Ui.ts` 或 `features/<域>/*TableUi.ts`
2. **禁止**手写 `{ style: "width: …", headerStyle: "width: …" }` — 使用 `registryCol()` + `REGISTRY_COL_W`
3. **列表表必须用** `AppRegistryTable`（或只读小表用 `AppRegistryMarkupTable`），禁止裸 `q-table`
4. **单元格**优先 `.app-registry-cell-primary` / `__sub` / `__actions`；域样式用 `table-class` + `_registry-page.sass`

### 8.2 列 API

```typescript
import { REGISTRY_COL_W, registryCol, registryColActions, registryColEnabled } from "../../features/ui/registryTableColumns";

registryCol<Row>("name", "名称", "name", "left", REGISTRY_COL_W.name);
registryColEnabled<Row>();
registryColActions<Row>();
```

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

- [ ] **已读模块交叉参考手册**：在 `docs/module-cross-reference.md` 中找到目标前端模块卡片，确认后端对应 Service/Proto/Store、跨 Store 依赖、事件消费
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
| **架构蓝图** | `docs/architecture-blueprint.md` | 前端全貌：36 个 Store、实时层、路由 |
| **模块交叉参考** | `docs/module-cross-reference.md` | §三·前端模块上下文卡 + §六·前后端对齐表 |

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
