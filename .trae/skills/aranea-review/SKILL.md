---
name: "aranea-review"
description: "Aranea-Agents 全栈代码审查指导。当审查本项目代码（Go 后端 + Vue 3 前端）的架构合规、分层规范、数据流、OOP 设计、Agent 运行时集成、聊天消息分组、UX 主题时自动触发，提供结构化审查清单与业务相关检查。"
---

# Aranea-Agents 全栈代码审查指导

> **文档地位**：本项目代码审查的权威规范，统一后端 + 前端审查流程。
> **编码规范参考**：后端见 `aranea-coding-guide` SKILL，前端见 `aranea-frontend-guide` SKILL。
> **通用 OOP 审查**：见 `go-oop-review` SKILL；**通用 Vue 3 审查**：见 `vue-frontend-guide` SKILL。
> 本文聚焦**项目业务合规性审查**，补充通用审查未覆盖的领域。

---

## 审查流程

```
1. 判定审查范围（后端 / 前端 / 全栈）
2. 后端审查：架构合规 → 分层合规 → OOP 审查 → Agent 运行时合规 → 并发安全 → 错误处理
3. 前端审查：数据流合规 → 组件分层 → 业务逻辑归属 → 聊天消息分组 → UX 主题
4. 构建与回归审查
5. 输出统一审查报告
```

---

## 严重级别定义

| 级别 | 含义 | 处理方式 |
|------|------|----------|
| 🔴 阻断 | 违反红线或导致 bug / 安全问题 | 必须修复，不可合并 |
| 🟡 建议 | 不符合最佳实践，影响可维护性 | 推荐修复，可后续迭代 |
| 🟢 提示 | 风格偏好或微优化 | 记录备忘 |

---

## 一、后端架构合规审查

> 核心原则：依赖方向向内，跨层只允许向内依赖。违反即停。

| # | 检查项 | 严重级别 | 说明 |
|---|--------|----------|------|
| BA1 | `internal/server/*` 是否 new `runner.Runner` 或 `llmagent.New` | 🔴 阻断 | Runner 装配只在 `internal/service` |
| BA2 | `internal/biz/*` 是否 import `pkg/trpc-agent-go` 任何包 | 🔴 阻断 | 框架交互通过 `internal/agent`/`internal/tools` 桥接 |
| BA3 | `internal/biz` 是否 import `api/*/v1` proto 包 | 🔴 阻断 | proto 映射只在 Service 层；biz 定义端口接口 |
| BA4 | Service 层是否写了业务逻辑 | 🟡 建议 | Service 只做 proto↔biz 映射 + Runner 编排 |
| BA5 | Server 层是否写了业务路由 | 🔴 阻断 | 只做 `Register*HTTPServer`/`Register*ServiceServer` |
| BA6 | 跨模块调用是否持有对方 Service 具体类型 | 🟡 建议 | 通过 biz 级窄接口（端口）交互，Wire 绑定在 Service 层 |
| BA7 | Wire 绑定是否在 Service 层 | 🟡 建议 | biz 层只定义接口，Wire 绑定收口在 Service |
| BA8 | 是否修改了工具生成的代码（protoc/wire/Ent 等） | 🔴 阻断 | 改源头 → 重新生成 → 提交生成物 |
| BA9 | Graph 运行时类型是否泄漏到 biz | 🟡 建议 | biz 暴露 `GraphBuildConfig`/`GraphRuntime`/`GraphExecutor` 端口 |

**检测方法**：

```bash
# 检测 biz 层违规 import
grep -rn "pkg/trpc-agent-go\|api/kratos.*v1" internal/biz/
# 检测 server 层 Runner 装配
grep -rn "runner.Runner\|llmagent.New\|runner.New" internal/server/
# 检测 Service 层业务逻辑（if/for 超过映射转换）
grep -rn "fmt.Errorf" internal/service/
```

---

## 二、后端分层合规审查

> 逐层检查职责边界与编码规范。

| # | 检查项 | 严重级别 | 说明 |
|---|--------|----------|------|
| BL1 | Service 层类型转换是否用 `toProtoXxx`/`fromProtoXxx` | 🟡 建议 | 禁止在方法内内联转换逻辑 |
| BL2 | Service 层错误映射是否用 `kerrors` | 🔴 阻断 | 禁止 `fmt.Errorf` 返回业务错误 |
| BL3 | Biz 层模型是否用纯 Go struct | 🟡 建议 | 字段用基本类型，不用 proto 类型 |
| BL4 | Biz 层 Repo 接口是否定义在 biz | 🟡 建议 | 实现在 data，接口在 biz |
| BL5 | Data 层是否仅通过 `d.Ent()`/`d.Postgres()` 访问 | 🔴 阻断 | 禁止另开 SQLite 连接 |
| BL6 | Data 层转换函数是否 `entXxxToBiz`/`bizXxxToEnt` | 🟡 建议 | 放在对应 Repo 文件中 |
| BL7 | Data 层是否有编译期接口检查 `var _ biz.XxxRepo = (*xxxRepo)(nil)` | 🟢 提示 | 确保接口实现完整 |
| BL8 | Service 层构造函数是否只接收接口或具体依赖 | 🔴 阻断 | 不接收"上帝对象" |

---

## 三、后端 OOP 审查

> 通用 Go OOP 审查详见 `go-oop-review` SKILL。以下为项目特定补充。

### 3.1 接口审查

| # | 检查项 | 严重级别 | 判定标准 |
|---|--------|----------|----------|
| BI1 | 接口方法数是否 ≤ 5 | 🟡 建议 | 超过 5 个方法应拆分为多个小接口 |
| BI2 | 接口是否定义在使用方 | 🟡 建议 | 端口接口在 biz，实现在 data |
| BI3 | 是否存在"上帝接口" | 🔴 阻断 | 合并了多个不相关职责的接口必须拆分 |
| BI4 | 返回值是否为具体类型 | 🟡 建议 | 函数应返回具体类型，参数接收接口 |
| BI5 | 是否滥用 `interface{}` | 🔴 阻断 | 用泛型或具体类型替代 |
| BI6 | Repository 接口方法是否 ≤ 5 | 🟡 建议 | 超过则按职责域拆分为子接口（`SessionReader`/`SessionWriter`/`MessageReader` 等） |

### 3.2 Struct 审查

| # | 检查项 | 严重级别 | 判定标准 |
|---|--------|----------|----------|
| BS1 | 是否用工厂函数构造 | 🟡 建议 | 禁止裸 `&Xxx{}`，用 `NewXxx()` |
| BS2 | 嵌入是否用于组合而非继承 | 🟡 建议 | 嵌入不建立 is-a 关系，多态靠接口 |
| BS3 | 方法接收者是否一致 | 🟡 建议 | 同一类型所有方法统一用 `*T` 或 `T` |
| BS4 | 构造函数是否接收上帝对象 | 🔴 阻断 | 只接收接口或具体依赖 |
| BS5 | 是否有循环嵌入 | 🔴 阻断 | 嵌入不得形成循环 |

---

## 四、Agent 运行时合规审查

> 检查 Agent 编排、工具装配、记忆系统、Provider 集成是否合规。

| # | 检查项 | 严重级别 | 说明 |
|---|--------|----------|------|
| BR1 | 框架 plugin 回调是否直接写数据库 | 🔴 阻断 | 经 broker/async 异步写 |
| BR2 | 是否绕过 `internal/session/trpc` 把 Ent 行塞进 `session.Event` | 🔴 阻断 | 通过 session/trpc 适配 |
| BR3 | 是否在 transport 层解析工具参数或拼接 prompt | 🔴 阻断 | 工具装配在 `internal/tools`，prompt 在 `internal/agent` |
| BR4 | 是否为框架运行时另起独立 HTTP 监听 | 🔴 阻断 | 框架运行时复用 Kratos HTTP Server |
| BR5 | 是否把 Kratos middleware 逻辑复制进 `pkg/trpc-agent-go` | 🔴 阻断 | 中间件只在 `internal/server` |
| BR6 | 新增工具是否先在 `Registry()` 注册 | 🟡 建议 | `ToolRegistration` + `builtin_tools_seed.go` 种子 |
| BR7 | Chat/Team 是否共用同一 `BuildToolsets` 逻辑 | 🔴 阻断 | 禁止 Chat 和 Team 各写一套装配 |
| BR8 | 记忆工具是否通过 `memory.Service.Tools()` 注入 | 🟡 建议 | 不手动构造记忆工具实例 |
| BR9 | 记忆写入是否经 broker/async 异步写 | 🔴 阻断 | 禁止在 plugin 回调中直接写库 |
| BR10 | Provider 厂商连接是否收口在 `internal/provider` | 🟡 建议 | 禁止在 agent/service 中直接写 HTTP 客户端 |
| BR11 | 契约对齐是否以 `pkg/trpc-agent-go/model` 为准 | 🟡 建议 | 禁止在业务包中平行维护另一套驱动接口 |
| BR12 | 流式工具是否以 `FinalResultChunk` 或 `FinalResultStateChunk` 结束 | 🔴 阻断 | 框架自动分派，流式工具必须正确结束 |
| BR13 | MCP Broker `AllowAdHocHTTP` 是否默认 false | 🔴 阻断 | 安全边界必须明确 |
| BR14 | 工具策略是否在 biz 层解析 | 🟡 建议 | biz 层解析为 effective tool keys，tools 层只做框架映射 |

---

## 五、后端并发安全与错误处理审查

### 5.1 并发安全

| # | 检查项 | 严重级别 | 判定标准 |
|---|--------|----------|----------|
| BC1 | `go func()` 是否走 safego | 🔴 阻断 | 必须用 `pkg/safego.Go` / `pkg/safego.GoRecover` |
| BC2 | 跨层调用是否传递 ctx | 🟡 建议 | 所有跨层调用必须传递 `ctx` |
| BC3 | 共享状态是否有锁保护 | 🔴 阻断 | 共享状态用 `sync.Mutex`/`sync.RWMutex` |
| BC4 | 是否有竞态条件 | 🔴 阻断 | map 并发读写、slice 并发 append |
| BC5 | goroutine 是否可取消 | 🟡 建议 | 长期运行的 goroutine 应支持 ctx 取消 |
| BC6 | MCP 子进程是否在 context 取消时清理 | 🟡 建议 | 防止子进程泄漏 |
| BC7 | WebSocket 流是否处理客户端断连 | 🟡 建议 | 防止资源泄漏 |

### 5.2 错误处理

| # | 检查项 | 严重级别 | 判定标准 |
|---|--------|----------|----------|
| BE1 | 业务错误是否用 kerrors | 🔴 阻断 | 禁止 `fmt.Errorf` 返回业务错误 |
| BE2 | 错误是否丢失上下文 | 🟡 建议 | wrap 错误用 `fmt.Errorf("xxx: %w", err)` |
| BE3 | 错误变量是否 Err 前缀 | 🟢 提示 | `ErrNotFound` 而非 `NotFoundError` |
| BE4 | 是否吞掉了错误 | 🔴 阻断 | `_ = someFunc()` 忽略 error 返回值 |
| BE5 | panic 是否被 recover | 🔴 阻断 | 业务代码禁止裸 panic |

### 5.3 日志

| # | 检查项 | 严重级别 | 判定标准 |
|---|--------|----------|----------|
| BLG1 | 是否使用 `log/slog` | 🔴 阻断 | 统一使用 `internal/event` 的 `FlowLog` |
| BLG2 | 是否有 `fmt.Println` / `log.Println` 调试残留 | 🟡 建议 | 清理或替换为 FlowLog |

---

## 六、后端依赖注入审查

| # | 检查项 | 严重级别 | 说明 |
|---|--------|----------|------|
| BD1 | Wire ProviderSet 是否每层一个 | 🟡 建议 | `biz.go`/`data.go`/`service.go`/`server.go` |
| BD2 | 是否手动编辑 `wire_gen.go` | 🔴 阻断 | 必须通过 `make wire` 生成 |
| BD3 | `cmd/admin/wire.go` 是否有全局副作用注册 | 🔴 阻断 | 全局注册归属 `NewXxxRepo`/`NewXxxUsecase` 构造函数 |
| BD4 | 是否存在占位 `*Bootstrap` 类型 | 🔴 阻断 | 禁止占位类型 + 全局副作用模式 |
| BD5 | Wire 改动后是否执行 `make wire-clean` | 🟡 建议 | PR 须通过 CI `wire-clean` job |

---

## 七、前端数据流合规审查

> 核心原则：API → Store → Composable → Page → Component（props/emits）

| # | 检查项 | 严重级别 | 说明 |
|---|--------|----------|------|
| FD1 | 展示组件是否 import Store/API | 🔴 阻断 | `components/**/*.vue` 禁止 `useXxxStore`、`features/*/api`、`axios`、`kratosApi` |
| FD2 | Page 是否直接 import api | 🔴 阻断 | `pages/**/*Page.vue` 禁止 `import { listXxx } from '../features/.../api'` |
| FD3 | 同一数据是否多处重复 fetch | 🟡 建议 | 应合并到 Store 单一数据源 |
| FD4 | Dialog/浮层是否内部调 API | 🔴 阻断 | 应 `emit('submit', payload)`，由 Page/Store 调 API |
| FD5 | 展示组件是否 watch+fetch+ref 存业务数据 | 🔴 阻断 | 应进 Store |
| FD6 | 新 HTTP 调用是否在 api.ts | 🔴 阻断 | 禁止 `.vue` 中裸 URL 或散装 axios |
| FD7 | Store 是否在 stores/index.ts 具名导出 | 🟡 建议 | 保持 Quasar Pinia 安装方式一致 |
| FD8 | 跨 Store 同步是否通过 `stores/sessionSync.ts` 事件总线 | 🔴 阻断 | 禁止直接 import 另一 Store 导致循环依赖 |

**检测方法**：

```bash
# 检测展示组件中的 Store/API import
grep -rn "useXxxStore\|from.*api\|from.*services\|axios\|kratosApi" web/src/components/
# 检测 Page 中的直接 API import
grep -rn "from.*features.*api" web/src/pages/
```

---

## 八、前端组件分层审查

| # | 检查项 | 严重级别 | 说明 |
|---|--------|----------|------|
| FL1 | 展示组件是否放在 components/<域>/ | 🔴 阻断 | 禁止放在 features/<域>/ 或 pages/ |
| FL2 | features/<域>/ 是否只有 api.ts/composable/容器 | 🟡 建议 | 容器组件须首行注释 `// Container: approved because …` |
| FL3 | Page script 是否超过 ~200 行 | 🟡 建议 | 超过则拆 Dialog/Panel/composable |
| FL4 | 组件类型是否从 types.ts 引入 | 🟡 建议 | 禁止从 api.ts 引入（即使仅类型） |
| FL5 | 域内 composable 是否绕过 Store 直接调 API | 🟡 建议 | 须标 `// TECH-DEBT` 并尽快迁入 Store |
| FL6 | 多 Dialog/Tab 是否已拆分 | 🟡 建议 | 拆为 `*Dialog.vue`、`*Panel.vue` |

**检测方法**：

```bash
# 检测 Page 行数
find web/src/pages -name "*Page.vue" -exec sh -c 'lines=$(grep -c "" "$1"); if [ $lines -gt 250 ]; then echo "$1: $lines lines"; fi' _ {} \;
# 检测 features 下的 .vue 文件
find web/src/features -name "*.vue"
```

---

## 九、前端业务逻辑归属审查

> 检查业务逻辑是否放在正确的层

| # | 检查项 | 严重级别 | 说明 |
|---|--------|----------|------|
| FB1 | 数据转换（API→领域）是否在 api.ts | 🟡 建议 | 不在组件中做 API 响应转换 |
| FB2 | 列表筛选/排序/分页逻辑是否在 Store/Composable | 🟡 建议 | 不在组件 computed 中做复杂业务计算 |
| FB3 | 错误处理是否在 Store action | 🟡 建议 | 组件只展示 error 状态，不 catch 后静默 |
| FB4 | $q.notify 是否在 Composable/Store | 🟡 建议 | 不在展示组件中直接调 $q.notify |
| FB5 | Agent 设置 Panel 是否由 Page 注入数据 | 🟡 建议 | Panel 内不应 useStore |
| FB6 | 共享 UI 常量是否在 *Ui.ts | 🟢 提示 | 不在 Page 与 Panel 各复制一份 |
| FB7 | 表格列定义是否在 *Ui.ts / *TableUi.ts | 🔴 阻断 | 禁止写在 Table.vue 或 Page script 内 |

---

## 十、聊天消息分组审查

> 前端红线 #14 专项检查

| # | 检查项 | 严重级别 | 说明 |
|---|--------|----------|------|
| FM1 | 是否使用 turn_index 做分组决策 | 🔴 阻断 | 前端禁止读取或推算 turn_index |
| FM2 | groupMessagesByTurn 是否按 role=user 边界 | 🔴 阻断 | 堆栈模型：user 开新 block，后续归入 |
| FM3 | in-flight 消息排序是否正确 | 🟡 建议 | pending-user-*/ws-stream-*/member-* 排在持久化消息之后 |
| FM4 | mergeSessionMessages 是否按 created_at 排序 | 🟡 建议 | 持久化按时间，in-flight 排最后 |
| FM5 | 是否存在 deriveTurnKey/inferTurnIndex 等推算函数 | 🔴 阻断 | 禁止任何 turn_index 推算逻辑 |

**检测方法**：

```bash
# 检测 turn_index 使用
grep -rn "turn_index\|deriveTurnKey\|inferAssistant\|inferTool\|realignEphemeral\|nextUserTurnIndex" web/src/
```

---

## 十一、UX 主题审查

| # | 检查项 | 严重级别 | 说明 |
|---|--------|----------|------|
| FU1 | 是否硬编码 hex/rgb | 🟡 建议 | 页面/组件应用 `var(--*)`（token 定义文件除外） |
| FU2 | 日间是否使用霓虹青紫作默认强调 | 🔴 阻断 | `#00E5FF` / `#A855F7` 仅夜间焦点/边 |
| FU3 | 浮层是否 backdrop-filter 成对 | 🔴 阻断 | 必须同时写 `-webkit-backdrop-filter` |
| FU4 | Dialog 是否使用 app-dialog-card | 🟡 建议 | 新 Dialog 必须带 `app-dialog-card` |
| FU5 | 是否有第二套全局 CSS 入口 | 🔴 阻断 | 新 token 只在 theme/ 增加 partial |
| FU6 | 是否运行时改 quasar-variables | 🔴 阻断 | 昼夜仅用 Dark + CSS 变量 + body 选择器 |
| FU7 | 主按钮/链接/焦点是否用 --color-accent | 🟡 建议 | 日间 `#E9A23B`，夜间 `#00E5FF` |
| FU8 | 聊天气泡是否用 positive 绿描边 | 🟡 建议 | 助手气泡用 `--glass-elevated` + `--glass-border` |
| FU9 | Registry 表格是否用 AppRegistryTable | 🔴 阻断 | 禁止裸 `q-table` |
| FU10 | 表格列宽是否用 registryCol + REGISTRY_COL_W | 🔴 阻断 | 禁止手写 width style |

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

## 十二、构建与回归审查

| # | 检查项 | 严重级别 | 说明 |
|---|--------|----------|------|
| FR1 | 后端 `make lint` 是否通过 | 🔴 阻断 | lint 不通过不可合并 |
| FR2 | 后端 `make test` 是否通过 | 🔴 阻断 | 测试不通过不可合并 |
| FR3 | 后端 `make build` 是否通过 | 🔴 阻断 | 构建不通过不可合并 |
| FR4 | 前端 `pnpm lint` 是否通过 | 🔴 阻断 | lint 不通过不可合并 |
| FR5 | 前端 `pnpm test` 是否通过 | 🔴 阻断 | 测试不通过不可合并 |
| FR6 | 前端 `pnpm build` 是否通过 | 🔴 阻断 | 构建不通过不可合并 |
| FR7 | Wire 改动后 `make wire-clean` 是否通过 | 🟡 建议 | PR 须通过 CI wire-clean job |
| FR8 | Proto 改动后 `make api` 是否已执行 | 🔴 阻断 | Go + TS 生成物须提交 |
| FR9 | 新增 Store 是否破坏 default export Pinia | 🔴 阻断 | Quasar Pinia 安装方式须一致 |
| FR10 | 是否有循环依赖 | 🟡 建议 | Store 勿 import .vue |
| FR11 | 昼/夜是否各看一眼 | 🟡 建议 | 或用 `/dev/theme-preview` |

---

## 审查输出模板

```markdown
## Aranea-Agents 全栈代码审查报告

### 概要

| 维度 | 🔴 阻断 | 🟡 建议 | 🟢 提示 | 合计 |
|------|---------|---------|---------|------|
| **后端 — 架构合规** | | | | |
| **后端 — 分层合规** | | | | |
| **后端 — OOP** | | | | |
| **后端 — Agent 运行时** | | | | |
| **后端 — 并发安全** | | | | |
| **后端 — 错误处理** | | | | |
| **后端 — 依赖注入** | | | | |
| **前端 — 数据流合规** | | | | |
| **前端 — 组件分层** | | | | |
| **前端 — 业务逻辑归属** | | | | |
| **前端 — 聊天消息分组** | | | | |
| **前端 — UX 主题** | | | | |
| **构建与回归** | | | | |

### 阻断项（必须修复）

| ID | 维度 | 端 | 文件 | 问题描述 | 修复建议 |
|----|------|----|------|----------|----------|
| | | 后端/前端 | | | |

### 建议项（推荐修复）

| ID | 维度 | 端 | 文件 | 问题描述 | 修复建议 |
|----|------|----|------|----------|----------|
| | | 后端/前端 | | | |

### 提示项（记录备忘）

| ID | 维度 | 端 | 文件 | 描述 |
|----|------|----|------|------|
| | | 后端/前端 | | |

### 亮点

-

### 后端合规性清单

- [ ] 依赖方向向内（biz 不 import data/service/trpc-agent-go/proto）
- [ ] Runner 装配在 Service 层
- [ ] Service 层无业务逻辑
- [ ] 跨模块通过窄接口
- [ ] Wire 绑定在 Service 层
- [ ] 无工具生成代码的手动修改
- [ ] goroutine 走 safego
- [ ] 业务错误用 kerrors
- [ ] 日志用 FlowLog
- [ ] 共享状态有锁保护
- [ ] 无上帝对象注入
- [ ] 接口方法 ≤ 5
- [ ] Repository 接口方法 ≤ 5（否则拆子接口）

### 前端合规性清单

- [ ] 展示组件无 Store/API import
- [ ] Page 无直接 API import
- [ ] Dialog/浮层 emit 而非内部调 API
- [ ] 新 HTTP 调用在 api.ts
- [ ] 跨 Store 同步走 sessionSync 事件总线
- [ ] 聊天消息分组用堆栈模型（非 turn_index）
- [ ] 浮层 backdrop-filter 成对
- [ ] 主按钮用 --color-accent
- [ ] Dialog 用 app-dialog-card
- [ ] Registry 表格用 AppRegistryTable + registryCol()
- [ ] 表格列定义在 *Ui.ts（非 .vue 内）
- [ ] Page script ≤~200 行
```

---

## 审查决策树

### 后端决策树

```
发现 import → biz import data？ → 是 → 🔴 阻断
            → biz import pkg/trpc-agent-go？ → 是 → 🔴 阻断
            → biz import api proto？ → 是 → 🔴 阻断
            → server new Runner？ → 是 → 🔴 阻断

发现接口 → 方法数 > 5？ → 是 → 🟡 拆分
         → 定义在使用方？ → 否 → 🟡 移到使用方
         → 上帝接口？ → 是 → 🔴 必须拆分

发现 struct → 裸构造？ → 是 → 🟡 改工厂函数
            → 嵌入继承思维？ → 是 → 🟡 改用接口多态
            → 上帝对象注入？ → 是 → 🔴 改窄依赖

发现 go func() → 走 safego？ → 否 → 🔴 必须改
               → 传 ctx？ → 否 → 🟡 加 ctx

发现 error → fmt.Errorf 业务错误？ → 是 → 🔴 改 kerrors
           → 吞错误？ → 是 → 🔴 必须处理
           → wrap 丢上下文？ → 是 → 🟡 加 %w

发现日志 → log/slog？ → 是 → 🔴 改 FlowLog

发现 Wire → 手动改 wire_gen.go？ → 是 → 🔴 阻断
          → wire.go 全局副作用？ → 是 → 🔴 阻断

发现工具装配 → 未在 Registry 注册？ → 是 → 🟡 先注册
             → Chat/Team 各写一套？ → 是 → 🔴 阻断
```

### 前端决策树

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

发现裸 q-table？
  → 🔴 阻断：改用 AppRegistryTable

发现表格列定义在 .vue 内？
  → 🔴 阻断：移到 *Ui.ts / *TableUi.ts

发现跨 Store 直接 import？
  → 🔴 阻断：改用 sessionSync 事件总线
```

---

## 审查范围判定

```
变更文件类型？
│
├─ 仅 .go 文件 → 后端审查（第一~六节）
│
├─ 仅 .vue/.ts/.sass 文件 → 前端审查（第七~十一节）
│
├─ .go + .proto → 后端审查 + 构建回归（Proto 变更专项）
│
├─ .go + 前端文件 → 全栈审查
│
└─ 仅配置/文档 → 跳过业务审查，仅检查构建
```
