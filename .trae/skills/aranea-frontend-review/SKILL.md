---
name: "aranea-frontend-review"
description: "Aranea-Agents 前端代码审查指导。当审查前端代码的数据流合规、组件分层、UX 主题、聊天消息分组、业务逻辑归属时自动触发，提供结构化审查清单与业务相关检查。"
---

# Aranea-Agents 前端代码审查指导

> **项目特定约束**见 `aranea-frontend-guide` SKILL。**通用 Vue 3 审查**见 `vue-frontend-guide` SKILL。
> 本文聚焦**项目业务合规性审查**，补充通用审查未覆盖的领域。

---

## 审查流程

```
1. 数据流合规审查 → 2. 组件分层审查 → 3. 业务逻辑归属审查 → 4. 聊天消息分组审查 → 5. UX 主题审查 → 6. 构建与回归审查
```

---

## 一、数据流合规审查

> 核心原则：API → Store → Composable → Page → Component（props/emits）

| # | 检查项 | 严重级别 | 说明 |
|---|--------|----------|------|
| D1 | 展示组件是否 import Store/API | 🔴 阻断 | `components/**/*.vue` 禁止 `useXxxStore`、`features/*/api`、`axios`、`kratosApi` |
| D2 | Page 是否直接 import api | 🔴 阻断 | `pages/**/*Page.vue` 禁止 `import { listXxx } from '../features/.../api'` |
| D3 | 同一数据是否多处重复 fetch | 🟡 建议 | 应合并到 Store 单一数据源 |
| D4 | Dialog/浮层是否内部调 API | 🔴 阻断 | 应 `emit('submit', payload)`，由 Page/Store 调 API |
| D5 | 展示组件是否 watch+fetch+ref 存业务数据 | 🔴 阻断 | 应进 Store |
| D6 | 新 HTTP 调用是否在 api.ts | 🔴 阻断 | 禁止 `.vue` 中裸 URL 或散装 axios |
| D7 | Store 是否在 stores/index.ts 具名导出 | 🟡 建议 | 保持 Quasar Pinia 安装方式一致 |

**检测方法**：

```bash
# 检测展示组件中的 Store/API import
grep -rn "useXxxStore\|from.*api\|from.*services\|axios\|kratosApi" web/src/components/
# 检测 Page 中的直接 API import
grep -rn "from.*features.*api" web/src/pages/
```

---

## 二、组件分层审查

| # | 检查项 | 严重级别 | 说明 |
|---|--------|----------|------|
| L1 | 展示组件是否放在 components/<域>/ | 🔴 阻断 | 禁止放在 features/<域>/ 或 pages/ |
| L2 | features/<域>/ 是否只有 api.ts/composable/容器 | 🟡 建议 | 容器组件须首行注释 `// Container: approved because …` |
| L3 | Page script 是否超过 ~200 行 | 🟡 建议 | 超过则拆 Dialog/Panel/composable |
| L4 | 组件类型是否从 types.ts 引入 | 🟡 建议 | 禁止从 api.ts 引入（即使仅类型） |
| L5 | 域内 composable 是否绕过 Store 直接调 API | 🟡 建议 | 须标 `// TECH-DEBT` 并尽快迁入 Store |
| L6 | 多 Dialog/Tab 是否已拆分 | 🟡 建议 | 拆为 `*Dialog.vue`、`*Panel.vue` |

**检测方法**：

```bash
# 检测 Page 行数
find web/src/pages -name "*Page.vue" -exec sh -c 'lines=$(grep -c "" "$1"); if [ $lines -gt 250 ]; then echo "$1: $lines lines"; fi' _ {} \;
# 检测 features 下的 .vue 文件
find web/src/features -name "*.vue"
```

---

## 三、业务逻辑归属审查

> 检查业务逻辑是否放在正确的层

| # | 检查项 | 严重级别 | 说明 |
|---|--------|----------|------|
| B1 | 数据转换（API→领域）是否在 api.ts | 🟡 建议 | 不在组件中做 API 响应转换 |
| B2 | 列表筛选/排序/分页逻辑是否在 Store/Composable | 🟡 建议 | 不在组件 computed 中做复杂业务计算 |
| B3 | 错误处理是否在 Store action | 🟡 建议 | 组件只展示 error 状态，不 catch 后静默 |
| B4 | $q.notify 是否在 Composable/Store | 🟡 建议 | 不在展示组件中直接调 $q.notify |
| B5 | Agent 设置 Panel 是否由 Page 注入数据 | 🟡 建议 | Panel 内不应 useStore |
| B6 | 共享 UI 常量是否在 *Ui.ts | 🟢 提示 | 不在 Page 与 Panel 各复制一份 |
| B7 | 表格列定义是否在 *Ui.ts / *TableUi.ts | 🔴 阻断 | 禁止写在 Table.vue 或 Page script 内 |

---

## 四、聊天消息分组审查

> 红线 #14 专项检查

| # | 检查项 | 严重级别 | 说明 |
|---|--------|----------|------|
| M1 | 是否使用 turn_index 做分组决策 | 🔴 阻断 | 前端禁止读取或推算 turn_index |
| M2 | groupMessagesByTurn 是否按 role=user 边界 | 🔴 阻断 | 堆栈模型：user 开新 block，后续归入 |
| M3 | in-flight 消息排序是否正确 | 🟡 建议 | pending-user-*/ws-stream-*/member-* 排在持久化消息之后 |
| M4 | mergeSessionMessages 是否按 created_at 排序 | 🟡 建议 | 持久化按时间，in-flight 排最后 |
| M5 | 是否存在 deriveTurnKey/inferTurnIndex 等推算函数 | 🔴 阻断 | 禁止任何 turn_index 推算逻辑 |

**检测方法**：

```bash
# 检测 turn_index 使用
grep -rn "turn_index\|deriveTurnKey\|inferAssistant\|inferTool\|realignEphemeral\|nextUserTurnIndex" web/src/
```

---

## 五、UX 主题审查

| # | 检查项 | 严重级别 | 说明 |
|---|--------|----------|------|
| U1 | 是否硬编码 hex/rgb | 🟡 建议 | 页面/组件应用 `var(--*)`（token 定义文件除外） |
| U2 | 日间是否使用霓虹青紫作默认强调 | 🔴 阻断 | `#00E5FF` / `#A855F7` 仅夜间焦点/边 |
| U3 | 浮层是否 backdrop-filter 成对 | 🔴 阻断 | 必须同时写 `-webkit-backdrop-filter` |
| U4 | Dialog 是否使用 app-dialog-card | 🟡 建议 | 新 Dialog 必须带 `app-dialog-card` |
| U5 | 是否有第二套全局 CSS 入口 | 🔴 阻断 | 新 token 只在 theme/ 增加 partial |
| U6 | 是否运行时改 quasar-variables | 🔴 阻断 | 昼夜仅用 Dark + CSS 变量 + body 选择器 |
| U7 | 主按钮/链接/焦点是否用 --color-accent | 🟡 建议 | 日间 `#E9A23B`，夜间 `#00E5FF` |
| U8 | 聊天气泡是否用 positive 绿描边 | 🟡 建议 | 助手气泡用 `--glass-elevated` + `--glass-border` |
| U9 | Registry 表格是否用 AppRegistryTable | 🔴 阻断 | 禁止裸 `q-table` |
| U10 | 表格列宽是否用 registryCol + REGISTRY_COL_W | 🔴 阻断 | 禁止手写 width style |

**检测方法**：

```bash
# 检测硬编码 hex（排除 token 定义文件）
grep -rn "#[0-9a-fA-F]\{3,8\}" web/src/components/ web/src/pages/ --include="*.vue" --include="*.ts"
# 检测裸 q-table
grep -rn "<q-table" web/src/ --include="*.vue"
# 检测 backdrop-filter 缺失
grep -rn "backdrop-filter" web/src/ --include="*.vue" --include="*.sass" | grep -v "webkit"
```

---

## 六、构建与回归审查

| # | 检查项 | 严重级别 | 说明 |
|---|--------|----------|------|
| R1 | pnpm lint 是否通过 | 🔴 阻断 | lint 不通过不可合并 |
| R2 | pnpm test 是否通过 | 🔴 阻断 | 测试不通过不可合并 |
| R3 | pnpm build 是否通过 | 🔴 阻断 | 构建不通过不可合并 |
| R4 | 昼/夜是否各看一眼 | 🟡 建议 | 或用 `/dev/theme-preview` |
| R5 | 新增 Store 是否破坏 default export Pinia | 🔴 阻断 | Quasar Pinia 安装方式须一致 |
| R6 | 是否有循环依赖 | 🟡 建议 | Store 勿 import .vue |

---

## 审查输出模板

```markdown
## 前端代码审查报告

### 概要

| 维度 | 🔴 阻断 | 🟡 建议 | 🟢 提示 | 合计 |
|------|---------|---------|---------|------|
| 数据流合规 | | | | |
| 组件分层 | | | | |
| 业务逻辑归属 | | | | |
| 聊天消息分组 | | | | |
| UX 主题 | | | | |
| 构建与回归 | | | | |

### 阻断项（必须修复）

| ID | 维度 | 文件 | 问题描述 | 修复建议 |
|----|------|------|----------|----------|
| | | | | |

### 建议项（推荐修复）

| ID | 维度 | 文件 | 问题描述 | 修复建议 |
|----|------|------|----------|----------|
| | | | | |

### 亮点

-
```

---

## 严重级别定义

| 级别 | 含义 | 处理方式 |
|------|------|----------|
| 🔴 阻断 | 违反红线或导致 bug | 必须修复，不可合并 |
| 🟡 建议 | 不符合最佳实践 | 推荐修复，可后续迭代 |
| 🟢 提示 | 可改进但不紧急 | 记录备忘 |

---

## 审查决策树

```
发现展示组件 import Store/API？
  → 🔴 阻断：上收到 Store/Composable

发现 Page 直接 import api？
  → 🔴 阻断：迁入 Store + composable

发现 turn_index 用于分组？
  → 🔴 阻断：改用堆栈模型

发现硬编码 hex？
  → 🟡 建议：替换为 CSS 变量

发现 Page >200 行？
  → 🟡 建议：拆 Dialog/Panel/composable

发现 TECH-DEBT 未标注？
  → 🟡 建议：添加标注并创建 issue

发现新 Dialog 无 app-dialog-card？
  → 🟡 建议：添加公共样式 class
```
