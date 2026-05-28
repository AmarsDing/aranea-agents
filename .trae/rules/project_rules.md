# Aranea-Agents 项目开发规则

> AI 每次开发必须遵守。细节按框架约定走，本规则只约束大框架。

---

## 一、项目概览

Aranea-Agents 是基于 trpc-agent-go 的多智能体编排平台。以 Kratos v2 为传输壳层、trpc-agent-go 为运行时内核。

**技术栈**：Go + Kratos v2（HTTP/gRPC/WebSocket）| trpc-agent-go（Agent 运行时）| Vue 3 + Quasar + Pinia + TypeScript | SQLite（Ent ORM）| Wire（编译期 DI）

**双框架分工**：
- Kratos v2：传输层（HTTP/gRPC/WebSocket）、配置、鉴权、中间件、Wire DI
- trpc-agent-go：Agent 编排（Runner/Agent/Session/Memory/Tool/Event/Skill/Graph/Team）

---

## 二、架构铁律（违反即停）

### 2.1 依赖方向

```
api/**/*.proto          ← 唯一对外契约
        ↓
internal/service        ← 传输桥点：proto ↔ biz 映射 + Runner 编排
        ↓
internal/biz            ← 领域模型 + Usecase + Repo 接口定义
        ↓
internal/data           ← Repo 实现（Ent ORM + SQLite）
```

**跨层只允许向内依赖。违反即停。**

### 2.2 后端红线

| # | 红线 | 正确做法 |
|---|------|----------|
| 1 | `internal/biz` 不得 import `pkg/trpc-agent-go` 任何包 | 框架交互通过 `internal/agent`/`internal/tools` 桥接 |
| 2 | `internal/biz` 不得 import `api/*/v1` proto 包 | proto 映射只在 Service 层；biz 定义端口接口 |
| 3 | Runner 装配只在 `internal/service` | `internal/server` 不得 new `runner.Runner` 或 `llmagent.New` |
| 4 | Service 层不得写业务逻辑 | Service 只做 proto↔biz 映射 + Runner 编排 |
| 5 | Server 层不得写业务路由 | 只做 `Register*HTTPServer`/`Register*ServiceServer` |
| 6 | 不得修改 protoc/wire 等工具生成的代码 | 改 proto → `make api`；改 wire 声明 → `make wire` |
| 7 | 跨模块调用不得持有对方 Service 具体类型 | 通过 biz 级窄接口（端口）交互，Wire 绑定在 Service 层 |
| 8 | 框架 plugin 回调不得直接写数据库 | 经 broker/async 异步写 |
| 9 | 所有 `go func()` 必须走 `pkg/safego` | 禁止裸 `go func()` 不处理 panic |
| 10 | 禁止使用 `log/slog` 记录日志 | 统一使用 `internal/event` 的 `FlowLog` |
| 11 | 不得在 `NewData` 外另开 SQLite 连接 | 仅通过 `d.Ent()` 访问 SQLite |
| 12 | 不得新增已无调用者的 deprecated 方法 | 死代码即删 |

### 2.3 前端红线

| # | 红线 | 正确做法 |
|---|------|----------|
| 1 | 展示组件不得直接调用 API / Store | 状态与请求收敛在 Store，组件仅 props in / emits out |
| 2 | 网络请求只在 Store action 中触发 | 禁止在 `.vue` 中写裸 URL 或散装 `axios` |
| 3 | 展示组件 `.vue` 放 `components/<域>/` | `features/<域>/` 只放 api.ts、composable、容器组件 |
| 4 | Page 不得直接 import `features/*/api` | 请求经 Store action；编排经 composable |
| 5 | 新 Store 必须在 `stores/index.ts` 具名导出 | 保持 Quasar Pinia 安装方式一致 |
| 6 | 禁止运行时用脚本改 `quasar-variables` | 昼夜仅用 Dark + CSS 变量 + body 选择器 |

---

## 三、各层职责与约定

### 3.1 后端分层

| 层 | 职责 | 关键约定 |
|----|------|----------|
| **Server** | 传输注册 + 中间件 | 只做 `Register*HTTPServer`，中间件统一在此注册 |
| **Service** | proto↔biz 映射 + Runner 编排 | 类型转换 `toProtoXxx`/`fromProtoXxx`，错误映射用 `kerrors` |
| **Biz** | 领域模型 + Usecase + Repo 接口 | 纯 Go struct，错误用 `kerrors`，禁止 `fmt.Errorf` |
| **Data** | Repo 实现（Ent ORM） | 仅通过 `d.Ent()`/`d.Postgres()` 访问，转换函数 `entXxxToBiz`/`bizXxxToEnt` |

### 3.2 Agent 运行时模块

```
internal/service        ← Runner 装配入口
internal/agent          ← Agent 构建（BuildLLMAgent、Memory、Plugins）
internal/team           ← Team 工作流
internal/tools          ← 工具注册中心 + Assemble 装配
internal/provider       ← LLM 模型驱动
internal/memory         ← 会话记忆
internal/session        ← 会话存储
internal/graph          ← 图编排
internal/channel        ← 渠道集成
internal/cronrunner     ← 定时任务
```

**框架真相源**：`pkg/trpc-agent-go` 是 Agent 框架的唯一真相源。先查框架 API 后再实现，不复制框架内部逻辑。

**工具装配**：新增工具先在 `Registry()` 注册 `ToolRegistration` + `builtin_tools_seed.go` 种子，Chat/Team 共用同一 `BuildToolsets` 逻辑。

**记忆系统**：记忆工具通过 `memory.Service.Tools()` 注入，记忆写入经 broker/async 异步写。

**Provider 集成**：厂商连接收口在 `internal/provider`，契约对齐以 `pkg/trpc-agent-go/model` 为准。

### 3.3 前端数据流

```
services/index.ts (createXxxService)
  → features/<域>/api.ts (HTTP 门面 + 类型归一化)
    → stores/<域>/index.ts (状态 + action 调 api)
      → features/<域>/useXxxPage.ts (composable 组合 Store)
        → pages/XxxPage.vue (布局 + 传参)
          → components/<域>/*.vue (纯展示：props in / emits out)
```

**CSS 主题**：单入口 `app-theme.sass` 聚合 partial；昼夜切换用 CSS 变量 + Quasar Dark mode；禁止与 `app-global.sass` 平行的第二套全局 CSS 入口。

---

## 四、决策树（代码该放哪？）

### 后端

```
新增 HTTP/gRPC 接口  → api/**/*.proto → internal/service → internal/server
新增业务逻辑        → internal/biz（模型 + Repo 接口 + Usecase）
新增数据库表/查询   → internal/data/ent/schema → go generate → internal/data
新增 LLM Agent 能力 → internal/agent（BuildLLMAgent 扩展）
新增工具           → internal/tools（Registry 注册 + Assemble 装配）
新增 Team 工作流   → internal/team（BuildWorkflowRoot）
新增 LLM 厂商      → internal/provider（实现 model.LLM）
新增记忆能力       → internal/memory（适配器 → trpcmemory.Service）
新增横切关注点     → internal/server + pkg/auth
```

### 前端

```
新增 HTTP 请求       → features/<域>/api.ts（经 services/ 或 kratosApi）
新增业务状态/加载    → stores/<域>/index.ts（action 内调 api）
多页面复用逻辑      → composables/useXxx.ts（组合 Store）
单页编排过重        → features/<域>/useXxxPage.ts 或 useXxxPanel.ts
新增 Dialog / 浮层  → components/<域>/*Dialog.vue（props/emits）
新增展示组件        → components/<域>/*.vue（仅 props/emits）
新增页面           → pages/**/*Page.vue（布局 + composable + 传参）
新增 CSS 变量       → css/theme/_css-vars-light.sass + _css-vars-dark.sass
```

---

## 五、Go 编码约定

### 命名

- 包名：小写单词，不用下划线（`agent`, `mcp/config`）
- 结构体/接口：大驼峰，名词（`AgentUsecase`, `AgentRepository`）
- 函数：大驼峰导出/小驼峰内部（`NewAgentUsecase`, `fromProtoRuntime`）
- 错误变量：`Err` 前缀（`ErrNotFound`）

### 错误处理

统一使用 `kerrors`，禁止 `fmt.Errorf` 返回业务错误：

```go
kerrors.BadRequest("AGENT", "id is required")
kerrors.NotFound("AGENT", "agent not found")
kerrors.InternalServer("AGENT", err.Error())
```

### 依赖注入

- Wire ProviderSet：每层一个（`biz.go`/`data.go`/`service.go`/`server.go`）
- 构造函数参数：只接收接口或具体依赖，不接收"上帝对象"
- 禁止手动编辑 `wire_gen.go`，必须通过 `make wire` 生成

### 并发

- 所有跨层调用必须传递 `ctx`
- goroutine 必须走 `pkg/safego.Go` / `pkg/safego.GoRecover`
- 共享状态用 `sync.Mutex`/`sync.RWMutex`，不用全局变量

---

## 六、模块间通信

| 方式 | 正确 | 错误 |
|------|------|------|
| 同步调用 | Usecase 之间通过接口调用 | 直接 import 另一模块的 data |
| 异步事件 | 通过 `Broker` 发布/订阅 | 通过全局变量共享状态 |
| 跨模块调用 | 通过 biz 级窄接口（端口） | 持有对方 Service 完整具体类型 |

**端口设计**：接口定义在 biz 层，Wire 绑定在 service 层，返回值用 biz 类型不返回 proto 类型。

---

## 七、验证

| 改动类型 | 最小验证 |
|----------|----------|
| 仅 Service + 单测 | `go test ./internal/service/... -run TestXxx -count=1` |
| 仅 Biz / Data | `go test ./internal/biz/... ./internal/data/... -count=1` |
| Proto 变更 | `make api && go build ./...` |
| Wire 注入 | `make wire && go build ./cmd/admin` |
| 前端 | `cd web && pnpm lint && pnpm test && pnpm build` |
| **提交前（全量）** | 后端：`make api && make wire && make build && make test && make lint`；前端：`cd web && pnpm lint && pnpm test && pnpm build` |

---

## 八、任务执行纪律

- 有任务 ID 时：只读对应 development.md / blueprint 中该 ID 块
- 列假设 → 编码 → 分级验证 → 通过后再扩 scope
- 只改与任务直接相关的文件；不顺带 refactor 相邻模块
