# Aranea-Agents AI 后端开发规范

> **文档地位**：AI 编码时的**唯一行为准则**（后端 Go）。
> **规范冲突优先级**：本文 > 所有其他 docs 下的规范文档。
> **前端规范**：见 [guides/frontend-guide.md](./frontend-guide.md)，不在本文范围。
> **阅读方式**：先看「速查卡」掌握红线与决策路径，再按需翻阅详细规范。

---

## 目录

- [速查卡](#速查卡)
  - [红线（违反即停）](#红线违反即停)
  - [决策树（我的代码该放哪？）](#决策树我的代码该放哪)
  - [任务速查卡](#任务速查卡)
- [第一章：架构总纲](#第一章架构总纲)
- [第二章：分层编码规范](#第二章分层编码规范)
- [第三章：Agent 运行时规范](#第三章agent-运行时规范)
- [第四章：API 与 Proto 规范](#第四章api-与-proto-规范)
- [第五章：Go 代码风格](#第五章go-代码风格)
- [第六章：模块化设计](#第六章模块化设计)
- [第七章：AI 编码自检清单](#第七章ai-编码自检清单)
- [附录](#附录)

---

## 速查卡

> AI 每次编码前**必须**先查阅本节。红线不可违反；决策树定位代码归属；任务速查卡给出步骤。

### 红线（违反即停）

| # | 红线 | 正确做法 |
|---|------|----------|
| 1 | `internal/server/*` 不得 new `runner.Runner` 或 `llmagent.New` | Runner 装配只在 `internal/service` |
| 2 | `internal/biz/*` 不得 import `pkg/trpc-agent-go` 任何包 | 框架交互通过 `internal/agent`/`internal/tools` 桥接 |
| 3 | 框架 `plugin` 回调不得直接写数据库 | 经 broker/async 异步写 |
| 4 | 不得绕过 `internal/session/trpc` 把 Ent 行塞进 `session.Event` | 通过 `session/trpc` 适配 |
| 5 | 不得在 transport 层解析工具参数或拼接 prompt | 工具装配在 `internal/tools`，prompt 在 `internal/agent` |
| 6 | 不得为框架运行时另起独立 HTTP 监听 | 框架运行时复用 Kratos HTTP Server |
| 7 | 不得把 Kratos middleware 逻辑复制进 `pkg/trpc-agent-go` | 中间件只在 `internal/server` |
| 8 | `internal/biz` 不得直接依赖框架运行时 toolset/skill 类型 | 通过 `internal/tools` 的 biz 友好接口 |
| 9 | Service 层不得写业务逻辑 | Service 只做 proto↔biz 映射 + Runner 编排 |
| 10 | 不得在 `NewData` 外另开 SQLite `sql.Open` | 仅通过 `d.Ent()` 访问 SQLite |
| 11 | **不得修改 protoc/wire 等工具生成的代码** | 改 proto → `make api`；改 wire 声明 → `make wire`；提交生成物 |
| 12 | 不得在 Server 层写业务路由或手写 `HandleFunc` | 只做 `Register*HTTPServer`/`Register*ServiceServer` |
| 13 | 所有 `go func()` 必须走 `pkg/safego.Go` / `pkg/safego.GoRecover` | 禁止裸 `go func()` 不处理 panic |
| 14 | 不得在 biz 层使用 `fmt.Errorf` 返回业务错误 | 统一使用 `kerrors.BadRequest/NotFound/InternalServer` |

### 决策树（我的代码该放哪？）

```
你要做什么？
│
├─ 新增 HTTP/gRPC 接口 ──────────→ api/**/*.proto → internal/service → internal/server
│
├─ 新增业务逻辑 ─────────────────→ internal/biz（模型 + Repo 接口 + Usecase）
│
├─ 新增数据库表/查询 ────────────→ internal/data/ent/schema → go generate → internal/data
│
├─ 新增 LLM Agent 能力 ──────────→ internal/agent（BuildLLMAgent 扩展）
│
├─ 新增工具 ─────────────────────→ internal/tools（Registry 注册 + Assemble 装配）
│
├─ 新增 Team 工作流 ─────────────→ internal/team（BuildWorkflowRoot）
│
├─ 新增 LLM 厂商 ────────────────→ internal/provider（实现 model.LLM）
│
├─ 新增记忆能力 ─────────────────→ internal/memory（适配器 → trpcmemory.Service）
│
├─ 新增前端页面 ─────────────────→ 见 guides/frontend-guide.md
│
└─ 新增横切关注点（鉴权/中间件）→ internal/server + pkg/auth
```

### 任务速查卡

#### 新增 API

```
1. api/kratos/<module>/v1/<module>.proto  ← 定义 RPC + HTTP 注解
2. make api                                ← 生成 Go + TS
3. internal/biz/<module>.go                ← 模型 + Repo 接口 + Usecase
4. internal/data/ent/schema/<module>.go    ← Ent Schema
5. go generate ./internal/data/ent         ← 生成 Ent 代码
6. internal/data/<module>_repo.go          ← Repo 实现
7. internal/service/<module>.go            ← Service（嵌入 Unimplemented*）
8. internal/server/http.go                 ← Register*HTTPServer
9. cmd/admin/wire.go                       ← Wire 注入
10. go build ./cmd/admin                   ← 验证编译
11. web/src/services/index.ts              ← 导出 createXxxService
```

#### 新增工具

```
1. internal/tools/registry.go              ← Registry() 中注册 ToolRegistration
2. internal/tools/builtin_tools_seed.go    ← 添加种子数据
3. 需要配置 → AssemblyConfig 增加字段 + Assemble 中增加覆盖逻辑
4. internal/tools/custom/                  ← 自定义工具实现（如需）
5. Chat 和 Team 共用 BuildToolsets，验证两处生效
```

#### 新增数据实体

```
1. internal/data/ent/schema/xxx.go         ← Fields/Index/Edge
2. go generate ./internal/data/ent         ← 生成 Ent 代码
3. internal/data/xxx.go                    ← Repo 实现（entXxxToBiz / bizXxxToEnt）
4. internal/biz/xxx.go                     ← 模型 + Repo 接口 + Usecase
5. internal/biz/biz.go                     ← ProviderSet 添加 NewXxxUsecase
6. internal/data/data.go                   ← ProviderSet 添加 NewXxxRepo
```

---

## 第一章：架构总纲

### 1.1 双框架分工

| 框架 | 职责边界 | 禁止 |
|------|----------|------|
| **Kratos v2** | 传输层（HTTP/gRPC/WebSocket）、配置、鉴权、中间件、Wire 依赖注入 | 不承载 Agent 编排、不实现第二套事件循环 |
| **trpc-agent-go** | Agent 编排（Runner/Agent/Session/Memory/Tool/Event） | 不直接写业务数据库、不处理 HTTP 路由 |

**各包职责映射**：

| 能力 | 主要包 | 关键函数/类型 |
|------|--------|---------------|
| 会话快照读写 | `internal/session/trpc` | `SQLiteSessionService` |
| Agent 构建 | `internal/agent` | `BuildLLMAgent` |
| LLM 模型驱动 | `internal/provider` | `ModelForProviderModel` |
| 工具注册与装配 | `internal/tools` | `Assemble`、`Registry` |
| 工具适配层 | `internal/tools/trpc` | `BuildToolsets` |
| Runner 内存/插件/用户ID | `internal/agent` | `NewTRPCMemoryService`、`DefaultRunnerPlugins`、`UserIDFromCtx` |
| Team 工作流 | `internal/team` | `BuildWorkflowRoot` + `runner.Run` |
| Agent-as-Tool | `internal/tools` | `AgentToolConfig` → `trpcagenttool.NewTool` |
| MCP ToolSet | `internal/tools` | `MCPServerConfig` → `trpcmcp.NewMCPToolSet` |
| MCP Broker | `internal/tools` | `MCPBrokerConfig` → `trpcmcpbroker.New` |
| 记忆服务 | `internal/memory/trpc` | `NewSQLiteMemoryService` |
| Stream 流式工具 | 框架 `tool.StreamableTool` | 项目层无需额外包 |

> 框架各模块的接口定义与详细用法见 [guides/trpc-agent-go-framework.md](./trpc-agent-go-framework.md)。
> Kratos 各层职责与约束见 [guides/kratos-framework-guide.md](./kratos-framework-guide.md)。

### 1.2 依赖方向铁律

```
api/**/*.proto          ← 唯一对外契约
        ↓
internal/service        ← 传输桥点：proto ↔ biz 映射 + 框架 Runner 装配
        ↓
internal/biz            ← 领域模型 + Usecase + Repo 接口定义
        ↓
internal/data           ← Repo 实现（Ent ORM + pgvector）
```

**跨层只允许向内依赖。违反即停。**

### 1.3 逐包 import 规则

| 包路径 | ✅ 允许 import | ❌ 禁止 import |
|--------|----------------|----------------|
| `internal/server/*` | `internal/service`、`internal/conf`、kratos、`pkg/auth`、`pkg/validate` | `pkg/trpc-agent-go` / 框架运行时私有 import、`runner.Runner`、`llmagent.New` |
| `internal/biz/*` | stdlib、kratos errors、本仓 biz/data API | `pkg/trpc-agent-go` 任何包、`api/*/v1`、框架运行时 toolset/skill 类型 |
| `internal/service/*` | `internal/biz`、`internal/team`、`internal/agent`、`internal/session/trpc`、`internal/provider`、`internal/tools`，框架 Runner/Agent 装配 API | 绕过 `internal/tools` 大量直连拼装底层 `tool` |
| `internal/agent/*` | `internal/biz`、`internal/provider`、`internal/data/...`（如需）、`internal/session/trpc`、`pkg/trpc-agent-go` / 框架运行时 | — |
| `internal/team/*` | `internal/biz`、`internal/agent`、`internal/provider`、`internal/tools`、`pkg/trpc-agent-go` / 框架运行时 | — |
| `internal/provider/*` | `internal/biz`、`pkg/trpc-agent-go` / 框架 `model` 适配 | — |
| `internal/tools/*` | `internal/biz`、框架 `tool` API（由 `pkg/trpc-agent-go` 暴露或兼容层 re-export） | — |

---

## 第二章：分层编码规范

> 各层的框架性职责定义见 [guides/kratos-framework-guide.md](./kratos-framework-guide.md)。本章只写项目特有约束。

### 2.1 Service 层——传输桥点

**职责**：实现 proto 接口，做 proto ↔ biz 类型映射，编排框架 Runner 调用。

**结构体模板**：

```go
type XxxService struct {
    v1.UnimplementedXxxServiceServer
    uc *biz.XxxUsecase
}

func NewXxxService(uc *biz.XxxUsecase) *XxxService {
    return &XxxService{uc: uc}
}
```

**编码规则**：

| 规则 | ✅ 正确 | ❌ 错误 |
|------|---------|---------|
| 嵌入 Unimplemented | `v1.UnimplementedXxxServiceServer` | 不嵌入，手写所有方法 |
| 构造函数 | `NewXxxService(uc *biz.XxxUsecase)` | `NewXxxService(uc, repo, db, runner)` |
| 类型转换命名 | `toProtoXxx`（biz→proto）、`fromProtoXxx`（proto→biz） | 在方法内内联转换逻辑 |
| 错误映射 | `kerrors.FromError(err)` 或 `kerrors.BadRequest/InternalServer` | `fmt.Errorf("...")` 返回 |
| 业务逻辑 | 调 `uc.XxxMethod()` | 在 Service 中写 if/for 业务判断 |

**Runner 装配规则**：

```go
func (s *ChatService) SendChatMessage(ctx context.Context, req *chatv1.SendChatMessageRequest) (*chatv1.SendChatMessageResponse, error) {
    // 1. proto → biz 参数
    // 2. 调 biz Usecase 获取 Agent/Session
    // 3. 调 internal/agent 构建 Agent
    // 4. 调 internal/agent 构建 Runner
    // 5. runner.Run → 事件流 → 投影为 proto 响应
}
```

**桥接约定**：`internal/service` 内 Kratos service 在方法中构造框架 `Runner`，将 RPC/HTTP 请求译为会话执行入口，将会话事件流投影为 unary 或 WebSocket。**不在 `internal/server` 或 `internal/biz` 中直接使用框架运行时。**

### 2.2 Biz 层——领域核心

**职责**：定义领域模型、Usecase 编排、Repo 接口。

1. **模型定义**：纯 Go struct，字段用基本类型，不用 proto 类型
2. **Repo 接口定义在 biz**，data 层实现
3. **Usecase 结构**：只接收接口或具体依赖，不接收"上帝对象"
4. **错误处理**：统一使用 `kerrors`，禁止 `fmt.Errorf`
5. **分页**：统一使用 `biz.ListOption` + `pagination.go`
6. **禁止 import**：`api/*/v1`、`pkg/trpc-agent-go` 任何包

### 2.3 Data 层——数据访问

**职责**：实现 biz 定义的 Repo 接口，封装数据库操作。

1. **数据库访问**：仅通过 `d.Ent()` / `d.Postgres()`，禁止另开连接
2. **Ent 转换函数**：`entXxxToBiz` / `bizXxxToEnt`，放在对应 Repo 文件中
3. **新增实体流程**：见[任务速查卡 - 新增数据实体](#新增数据实体)

### 2.4 Server 层——传输注册

**职责**：创建 HTTP/gRPC/WebSocket 实例，注册 Service。

```go
v1.RegisterXxxHTTPServer(srv, svc)
v1.RegisterXxxServiceServer(srv, svc)
```

**禁止**写业务路由：`srv.Route("/v1").HandleFunc("/custom", handler)`（红线 12）

**中间件**：统一在 `NewHTTPServer`/`NewGRPCServer` 中注册。

---

## 第三章：Agent 运行时规范

> 框架核心接口定义见 [guides/trpc-agent-go-framework.md](./trpc-agent-go-framework.md)。本章只写项目层约束。

### 3.1 框架真相源

**`pkg/trpc-agent-go` 是 Agent 框架的唯一真相源。**

| 原则 | ✅ 正确 | ❌ 错误 |
|------|---------|---------|
| 先查框架 API | 查 `pkg/trpc-agent-go` 的 Runner/Agent/Tool API 后再实现 | 在 biz 重写运行时逻辑 |
| 不复制框架 | 调用框架 API | 把框架内部实现整块复制到业务目录 |
| 编排语义归框架 | Runner/Agent/Tool/Session/Event 在框架中 | 在业务包中平行维护编排逻辑 |

### 3.2 运行时装配层次

```
internal/service        ← Runner 装配入口（调 agent/team/tools）
internal/agent          ← Agent 构建（BuildLLMAgent、Memory、Plugins）
internal/team           ← Team 工作流（BuildWorkflowRoot、Runner）
internal/tools          ← 工具注册中心 + Assemble 装配（Registry + AssemblyConfig）
internal/tools/trpc     ← 向后兼容适配层（ToolsetConfig → AssemblyConfig → Assemble）
internal/tools/mcpmount ← MCP 服务器发现与 ToolSet 装配
internal/tools/skillruntime ← Skill 工具集解析
internal/tools/skillrouter  ← Skill 检测与分类
internal/tools/custom   ← 自定义工具实现
internal/provider       ← LLM 模型驱动（ModelForProviderModel）
internal/runtimedeps    ← 运行时依赖注入（TurnDeps、Runtime 聚合）
internal/compress       ← L0 上下文压缩（长对话摘要）
internal/memory         ← 会话记忆（SQLite 适配器 → trpcmemory.Service）
internal/session        ← 会话存储（TRPC SessionService 适配）
internal/skill          ← 技能系统（导入、执行、Watch 热重载）
internal/graph          ← 图编排（TRPC Graph Builder）
internal/channel        ← 渠道集成（飞书 Webhook 等）
internal/cronrunner     ← 定时任务（Cron 调度与执行）
internal/llminspect     ← LLM 调试检查（模型连通性探测）
internal/mcpprobe       ← MCP 探针（服务可用性评估）
```

### 3.3 Agent 构建规范

**BuildLLMAgent 调用链**：

```go
deps := agent.BuilderDeps{
    Catalog:      s.llmCatalog,
    AgentUC:      s.agentsUC,
    Agents:       s.agents,
    ToolsCatalog: s.toolsCatalog,
    RT:           s.runtime,
    Memory:       agent.RunnerMemoryForRuntime(s.runtime),
    Provider:     provider,
    Model:        model,
}
root, err := agent.BuildLLMAgent(ctx, ag, deps)
runner, err := agent.NewTRPCRunnerForRuntime(root, sessSvc, s.runtime)
eventCh, err := runner.Run(ctx, userID, sessionID, userMessage)
```

| 规则 | 说明 |
|------|------|
| BuilderDeps 是 DTO | 不含框架运行时类型，只含 biz 模型 + 可选依赖标记 |
| Memory 由 Wire 注入 | 通过 `runtimedeps.Runtime.SessionMemory`，不在 Service 手动选择 |
| 工具统一挂载 | 通过 `TurnMount.Attach`，不分散在多处 |

### 3.4 工具装配规范

**核心装配入口**：`internal/tools/toolset.go` 的 `Assemble(ctx, cfg)`

**适配层入口**：`internal/tools/trpc/toolsets.go` 的 `BuildToolsets(ctx, cfg)`

**装配顺序**（在 `Assemble` 内部）：
1. Registry 注册工具（按 enabled 列表匹配，调用 Factory/ToolSetFactory）
2. 带配置覆盖的工具（file→WithBaseDir、geminifetch→WithModel 等）
3. OpenAPI Spec ToolSet
4. workspace_exec 扩展工具
5. AgentTool（`AgentToolConfig` → `trpcagenttool.NewTool`）
6. MCP ToolSet（`MCPServerConfig` → `trpcmcp.NewMCPToolSet`）
7. MCP Broker（`MCPBrokerConfig` → `trpcmcpbroker.New` → `broker.Tools()`）
8. CustomTools

**规则**：

| # | 规则 | ✅ 正确 | ❌ 错误 |
|---|------|---------|---------|
| 1 | 新增工具先注册 | `Registry()` 注册 `ToolRegistration` + `builtin_tools_seed.go` 种子 | 直接在 Service 中手写 tool 实例 |
| 2 | 需配置的工具 | `AssemblyConfig` 增加字段 + `Assemble` 增加覆盖逻辑 | 硬编码配置值 |
| 3 | Chat/Team 共用 | 同一 `BuildToolsets` 逻辑 | Chat 和 Team 各写一套装配 |
| 4 | 工具策略 | biz 层解析为 effective tool keys，tools 层只做框架映射 | tools 层解析 allow/deny 策略 |
| 5 | 适配层职责 | `ToolsetConfig` → `AssemblyConfig` → `Assemble` | 适配层直接拼装底层 tool |

### 3.5 Team 编排规范

| 模式 | 实现方式 | 适用场景 |
|------|----------|----------|
| Coordinator | 协调者 Agent 调度成员作为工具 | 需要中央决策 |
| Swarm | 成员间 `transfer_to_agent` 传递控制权 | 自由协作 |

**编码规则**：
1. Team Runner 在 `internal/team`，不溢出到 service 或 biz
2. 成员 Agent 独立构建：每个成员用自己的 Settings、Skill 策略、MCP 服务器列表
3. 事件流通过 `biz.TeamRunEventBroker` 发布 WebSocket

### 3.6 记忆系统规范

| 组件 | 职责 |
|------|------|
| `memory.Service` | 记忆 CRUD 接口（Add/Update/Delete/Clear/Read/Search） |
| `memory/tool.ToolSet` | 6 个记忆工具（add/search/load/update/delete/clear） |
| `memory/extractor` | 自动提取（LLM 从对话中提取 fact/episode） |

**两种记忆模式**：

| 模式 | 行为 | 接入方式 |
|------|------|----------|
| Agentic | Agent 主动调用 `memory_add`/`memory_search` 等工具 | `llmagent.WithMemoryService(service)` |
| Auto | 对话结束后 LLM 自动提取记忆 | `service.EnqueueAutoMemoryJob(ctx, session)` |

**规则**：

| # | 规则 | ✅ 正确 | ❌ 错误 |
|---|------|---------|---------|
| 1 | MemoryService 由 Wire 注入 | 有 Store → `NewSQLiteMemoryService`；无 → in-memory | Service 手动选择后端 |
| 2 | 记忆工具注入 | `service.Tools()` 返回 6 个 `tool.Tool`，追加到 Agent 工具列表 | 手动构造记忆工具实例 |
| 3 | 用户隔离 | `GetAppAndUserFromContext(ctx)` 获取 app+user 维度隔离 | 不做用户隔离 |
| 4 | 记忆写入 | 经 broker/async 异步写 | 在 plugin 回调中直接写库（红线 3） |

### 3.7 Provider 集成约定

| 原则 | ✅ 正确 | ❌ 错误 |
|------|---------|---------|
| 厂商连接收口 | `internal/provider` 承载初始化、解析、调用 | 在 agent/service 中直接写 HTTP 客户端 |
| 契约对齐 | 入参/出参以 `pkg/trpc-agent-go/model` 为准 | 在业务包中平行维护另一套驱动接口 |
| 新增厂商 | 扩展 `Registry` + 子包实现 `model.LLM` | 在 agent 中硬编码厂商 URL |

### 3.8 Stream 流式工具规范

**框架三层 Tool 接口**：`Tool`（Declaration）→ `CallableTool`（+Call）→ `StreamableTool`（+StreamableCall）

**执行流程**：框架自动根据接口类型分派 Call 或 StreamableCall。流式工具必须以 `FinalResultChunk` 或 `FinalResultStateChunk` 结束。

**AG-UI 集成**：`agui.WithStreamingToolResultActivityEnabled(true)` 开启后，中间结果转为 Activity 事件。

### 3.9 Agent-as-Tool 与 MCP Broker 规范

**Agent-as-Tool**（`trpcagenttool.NewTool`）：子 Agent 作为工具，支持 SkipSummarization、StreamInner、HistoryScope、ResponseMode 等选项。

**MCP Broker**（`trpcmcpbroker.New`）：4 个运行时发现工具（`mcp_list_servers`、`mcp_list_tools` 等）。`AllowAdHocHTTP` 默认 false。

---

## 第四章：API 与 Proto 规范

### 4.1 Proto 定义规则

| 规则 | ✅ 正确 | ❌ 错误 |
|------|---------|---------|
| 路径 | `api/kratos/<module>/v1/<module>.proto` | 随意放置 |
| HTTP 注解 | 每个 RPC 配 `google.api.http` | 只定义 RPC 不配 HTTP path |
| 必填标记 | `(google.api.field_behavior) = REQUIRED` | 不标记必填 |
| 命名 | proto 字段 `snake_case`，Go 生成 `CamelCase` | proto 字段用 camelCase |
| 契约完整性 | 全部能力在 proto 中定义 | 一半在 proto，一半手写路由 |

### 4.2 代码生成流程

```bash
make init    # 首次安装插件
make api     # 生成 Go + TypeScript
make wire    # 生成 Wire 依赖注入
make config  # 仅改 conf.proto 时
```

**工具生成物**：`*.pb.go`、`*_http.pb.go`、`*_grpc.pb.go`、`wire_gen.go`、`web/src/services/`

**禁止修改工具生成的代码（红线 11）。**

### 4.3 迁移与迭代硬约束

**协议（`api/**/*.proto`）**：
1. 对外能力必须在 Proto 中印全，禁止「一半在 proto，一半用手写 srv.Route / HandleFunc」
2. 修改 `.proto` 必须跑生成，提交全部生成物

**持久化（`internal/data`）**：
1. SQLite 侧以 `*ent.Client` 为主入口，禁止在 `NewData` 里再 `sql.Open` 同一 DSN
2. 表结构进 Ent，禁止长期平行维护「仅存 SQL、不进 Ent」
3. 复杂 WHERE / BLOB：优先 `predicate` + `dialect/sql`

**HTTP / gRPC 挂载（`internal/server`）**：
1. 业务模块 HTTP 只做 `Register<Module>HTTPServer`，gRPC 只做 `Register<Module>ServiceServer`
2. 横切路由（健康检查、网关、探测等）单独列出，不充当业务 Service 的补丁契约

### 4.4 横切与运维边界

- **`GET /healthz`**：在 `cmd/admin` 挂载，响应 `{"status":"ok"}`
- **`CRON_CHAT_DISPATCH_ORIGIN`**：Cron 到期任务 POST 发送聊天消息的目标根 URL
- **`CRON_RUNNER_INTERVAL`**：Cron tick，默认 `1m`
- **`CRON_RUNNER_DISABLED`**：设为 `1` 则不启动 `internal/cronrunner`

### 4.5 用量上报与双写

**单一写入方（避免重复计数）**：仅服务端在同一请求路径写入用量时，浏览器不应再报同一事件。若须二选一并行，应有 feature flag，默认只开一侧。

---

## 第五章：Go 代码风格

### 5.1 命名规范

| 场景 | 规范 | ✅ 示例 | ❌ 示例 |
|------|------|---------|---------|
| 包名 | 小写单词，不用下划线 | `agent`, `mcpmount` | `agent_svc`, `MCPMount` |
| 文件名 | 小写+下划线，按职责拆分 | `agent_repo.go`, `trpc_build.go` | `agentRepo.go` |
| 结构体 | 大驼峰，名词 | `AgentUsecase`, `TurnMount` | `agentUsecase` |
| 接口 | 大驼峰，名词+后缀 | `AgentRepository`, `MemoryService` | `AgentRepo`, `IMemory` |
| 函数 | 大驼峰导出/小驼峰内部 | `NewAgentUsecase`, `fromProtoRuntime` | `new_agent_usecase` |
| 常量 | 大驼峰导出/小驼峰内部 | `DefaultAppName`, `streamQueryKey` | `DEFAULT_APP_NAME` |
| 错误变量 | `Err` 前缀 | `ErrNotFound`, `ErrAppNameRequired` | `ErrorNotFound` |

### 5.2 函数设计

| 规则 | ✅ 正确 | ❌ 错误 |
|------|---------|---------|
| 单一职责 | 每个函数只做一件事 | 一个函数既查数据又发通知又写日志 |
| 参数 ≤ 5 | 超过则封装为 Option struct | `func Foo(a,b,c,d,e,f,g int)` |
| 返回值 | 业务函数返回 `(result, error)` | 用 `panic` 处理业务逻辑 |
| 构造函数 | 统一 `NewXxx` 命名，返回指针 | `CreateXxx`、`MakeXxx` |
| 类型转换 | 独立函数 `toProtoXxx`/`fromProtoXxx` | 在方法内内联转换逻辑 |

### 5.3 错误处理

```go
if id == "" {
    return Agent{}, kerrors.BadRequest("AGENT", "id is required")
}
if stderrors.Is(err, sql.ErrNoRows) {
    return Agent{}, kerrors.NotFound("AGENT", "agent not found")
}
return Agent{}, kerrors.InternalServer("AGENT", err.Error())
```

| 场景 | 使用 |
|------|------|
| 参数校验失败 | `kerrors.BadRequest` |
| 记录不存在 | `kerrors.NotFound` |
| 内部错误 | `kerrors.InternalServer` |
| 框架返回的 error | `kerrors.FromError(err)` |

### 5.4 依赖注入

1. **Wire ProviderSet**：每层一个，在 `biz.go`/`data.go`/`service.go`/`server.go` 中定义
2. **构造函数参数**：只接收接口或具体依赖，不接收 `*Data` 之外的"上帝对象"
3. **禁止手动编辑 `wire_gen.go`**：必须通过 `make wire` 生成

### 5.5 并发与资源

| 规则 | ✅ 正确 | ❌ 错误 |
|------|---------|---------|
| context 传递 | 所有跨层调用必须传递 `ctx` | `go func() { doWork() }()` 不传 ctx |
| goroutine | 必须走 `pkg/safego.Go` / `pkg/safego.GoRecover`（红线 13） | 裸 `go func()` 不处理 panic |
| MCP 子进程 | context 取消时清理 | 子进程泄漏 |
| WebSocket 流 | 处理客户端断连 | 不检测客户端断连 |
| 共享状态 | `sync.Mutex`/`sync.RWMutex` | 全局变量 |

---

## 第六章：模块化设计

### 6.1 新增功能模块的标准结构

```
api/kratos/<module>/v1/<module>.proto     ← API 契约
internal/biz/<module>.go                  ← 模型 + Repo 接口 + Usecase
internal/biz/<module>_types.go            ← 领域类型定义（如需拆分）
internal/data/<module>_repo.go            ← Repo 实现
internal/data/ent/schema/<module>.go      ← Ent Schema
internal/service/<module>.go              ← Service 实现
internal/server/http.go                   ← 注册 RegisterXxxHTTPServer
cmd/admin/wire.go                         ← Wire 注入
```

### 6.2 模块间通信

| 方式 | ✅ 正确 | ❌ 错误 |
|------|---------|---------|
| 同步调用 | Usecase 之间通过接口调用 | 直接 import 另一模块的 data |
| 异步事件 | 通过 `Broker` 发布/订阅 | 通过全局变量共享状态 |
| 状态共享 | 数据库 | 包级变量 |

### 6.3 接口隔离

```go
type AgentReader interface {
    GetAgentByID(ctx context.Context, id string) (Agent, error)
}

type AgentWriter interface {
    CreateAgent(ctx context.Context, a Agent) (Agent, error)
    UpdateAgent(ctx context.Context, a Agent) (Agent, error)
}

type AgentRepository interface {
    AgentReader
    AgentWriter
    SearchAgents(ctx context.Context, q AgentListQuery) (AgentListResult, error)
    DeleteAgent(ctx context.Context, id string) error
}
```

### 6.4 配置管理

1. **配置来源优先级**：环境变量 > 系统设置 > 配置文件 > 代码默认值
2. **配置结构**：在 `internal/conf/conf.proto` 中定义
3. **热更新**：通过 Kratos config source 支持，不自行实现 watch

---

## 第七章：AI 编码自检清单

### 改动中（逐层检查）

- [ ] **Service 层**：只做映射和编排，无业务逻辑
- [ ] **Biz 层**：无 `pkg/trpc-agent-go` import，无 proto import
- [ ] **Data 层**：仅 `Ent()`/`Postgres()` 访问，无并联 SQLite 连接
- [ ] **Agent/Tools/Team 层**：框架 API 调用合规，不复制框架内部逻辑
- [ ] **新增工具**：先在 `Registry()` 注册 `ToolRegistration`，再在 `builtin_tools_seed.go` 添加种子
- [ ] **流式工具**：实现 `StreamableTool` 接口，必须发送 `FinalResultChunk`
- [ ] **记忆工具**：通过 `memory.Service.Tools()` 注入，不手动构造
- [ ] **MCP Broker**：`AllowAdHocHTTP` 默认 false，安全边界明确
- [ ] **错误处理**：使用 `kerrors`，不用 `fmt.Errorf`
- [ ] **命名**：符合 §5.1 规范
- [ ] **goroutine**：走 `pkg/safego`，无裸 `go func()`

### 改动后（构建与验证）

- [ ] `make api` 已执行（如改了 proto）
- [ ] `make wire` 已执行（如改了 Wire 声明）
- [ ] `go build ./cmd/admin` 通过
- [ ] 无红线违反
- [ ] 新增能力两处生效（Chat + Team 共用装配路径）

### 全链路合并检查

- [ ] `api/**/*.proto` 覆盖本迭代全部 `/v1` 能力，`make api`，Go + TS 已提交
- [ ] `internal/biz` / data / service / server 合规；Wire + `go build ./cmd/admin`
- [ ] `web/src/services/index.ts` `createXXXService`
- [ ] 前端合规：见 [guides/frontend-guide.md](./frontend-guide.md) 自检清单

---

## 附录

### 附录 A：关键文件索引

| 文件 | 用途 |
|------|------|
| `internal/tools/toolset.go` | 工具注册中心（Registry + AssemblyConfig + Assemble） |
| `internal/tools/tool.go` | 项目级工具类型别名 |
| `internal/tools/trpc/toolsets.go` | 向后兼容适配层 |
| `internal/memory/trpc/sqlite_adapter.go` | Memory Service SQLite 适配器 |
| `internal/agent/trpc_build.go` | Agent 构建 + 工具集装配入口 |

### 附录 B：Wire 注入模板

```go
// biz/biz.go
var ProviderSet = wire.NewSet(NewXxxUsecase)

// data/data.go
var ProviderSet = wire.NewSet(NewData, NewXxxRepo)

// service/service.go
var ProviderSet = wire.NewSet(NewXxxService)

// server/server.go
var ProviderSet = wire.NewSet(NewGRPCServer, NewHTTPServer, NewWSServer)

// cmd/admin/wire.go
func wireApp(*conf.Server, *conf.Data, log.Logger) (wireOut, func(), error) {
    panic(wire.Build(
        server.ProviderSet, data.ProviderSet,
        biz.ProviderSet, service.ProviderSet, newApp,
    ))
}
```

### 附录 C：新增 Ent 实体模板

```go
type Xxx struct {
    ent.Schema
}

func (Xxx) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").DefaultFunc(func() string { return uuid.New().String() }),
        field.String("name").NotEmpty(),
        field.String("status").Default("active"),
        field.Time("created_at").Default(time.Now),
        field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
    }
}

func (Xxx) Edges() []ent.Edge { return nil }

func (Xxx) Indexes() []ent.Index {
    return []ent.Index{index.Fields("name")}
}
```
