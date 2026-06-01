---
name: "aranea-coding-guide"
description: "Aranea-Agents 项目统一编码指南。当在本项目编写 Go 后端代码时自动触发，提供架构铁律、分层规范、框架集成、代码探索与验证的完整指导。"
---

# Aranea-Agents 统一编码指南

> **文档地位**：本项目 Go 后端编码的权威规范，与 `.trae/rules/project_rules.md` 互补。SKILL 为详细版，rules 为精简版；内容冲突时以 SKILL 为准。
> **通用 Go OOP 规范**：见 `go-oop-guide` SKILL（接口设计、组合嵌入、工厂构造、设计模式等）。
> **前端规范**：见 `aranea-frontend-guide` SKILL，不在本文范围。

---

## 目录

- [第一章：架构总纲](#第一章架构总纲)
- [第二章：19 条红线](#第二章19-条红线)
- [第三章：代码探索约束](#第三章代码探索约束)
- [第四章：决策树](#第四章决策树)
- [第五章：分层编码规范](#第五章分层编码规范)
- [第六章：Agent 运行时规范](#第六章agent-运行时规范)
- [第七章：项目代码风格](#第七章项目代码风格)
- [第八章：API 与 Proto 规范](#第八章api-与-proto-规范)
- [第九章：模块化设计](#第九章模块化设计)
- [第十章：任务速查卡](#第十章任务速查卡)
- [第十一章：AI 编码自检清单](#第十一章ai-编码自检清单)
- [第十二章：验证命令](#第十二章验证命令)
- [第十三章：模块关联强制检查](#第十三章模块关联强制检查)

---

## 第一章：架构总纲

### 1.1 双框架分工

| 框架 | 职责边界 | 禁止 |
|------|----------|------|
| **Kratos v2** | 传输层（HTTP/gRPC/WebSocket）、配置、鉴权、中间件、Wire 依赖注入 | 不承载 Agent 编排、不实现第二套事件循环 |
| **trpc-agent-go** | Agent 编排（Runner/Agent/Session/Memory/Tool/Event/Skill/Graph/Team） | 不直接写业务数据库、不处理 HTTP 路由 |

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

**Kratos 标准 4 层**：

| 包路径 | ✅ 允许 import | ❌ 禁止 import |
|--------|----------------|----------------|
| `internal/server/*` | `internal/service`、`internal/conf`、kratos、`pkg/auth`、`pkg/validate` | `pkg/trpc-agent-go` / 框架运行时私有 import、`runner.Runner`、`llmagent.New` |
| `internal/biz/*` | stdlib、kratos errors、本仓 biz/data API | `pkg/trpc-agent-go` 任何包、`api/*/v1`、框架运行时 toolset/skill 类型 |
| `internal/service/*` | `internal/biz`、项目扩展模块、框架 Runner/Agent 装配 API | 绕过 `internal/tools` 大量直连拼装底层 `tool` |
| `internal/data/*` | `internal/biz`（实现 Repo 接口）、`internal/conf`、Ent、pgvector | `api/*/v1`、`pkg/trpc-agent-go` |

**项目扩展模块**：

| 包路径 | ✅ 允许 import | ❌ 禁止 import |
|--------|----------------|----------------|
| `internal/agent/*` | `internal/biz`、`internal/provider`、`internal/data/...`（如需）、`internal/session/trpc`、`pkg/trpc-agent-go` | — |
| `internal/team/*` | `internal/biz`、`internal/agent`、`internal/provider`、`internal/tools`、`pkg/trpc-agent-go` | `api/*/v1` |
| `internal/channel/*` | `internal/biz`、`internal/channel/port`、`internal/event` | 对方 Service 具体类型、`api/*/v1` |
| `internal/graph/adapter` | `internal/biz`、`internal/agent`、`internal/event` | 无关业务 Usecase |
| `internal/provider/*` | `internal/biz`、`pkg/trpc-agent-go` / 框架 `model` 适配 | — |
| `internal/tools/*` | `internal/biz`、框架 `tool` API | — |

### 1.4 各包职责映射

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

---

## 第二章：19 条红线

> 违反即停，不可绕过。

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
| 11 | 不得修改工具生成的代码（protoc/wire/Ent 等） | 改源头 → 重新生成 → 提交生成物 |
| 12 | 不得在 Server 层写业务路由或手写 `HandleFunc` | 只做 `Register*HTTPServer`/`Register*ServiceServer` |
| 13 | 所有 `go func()` 必须走 `pkg/safego.Go` / `pkg/safego.GoRecover` | 禁止裸 `go func()` 不处理 panic |
| 14 | 不得在 biz 层使用 `fmt.Errorf` 返回业务错误 | 统一使用 `kerrors.BadRequest/NotFound/InternalServer` |
| 15 | 非 Service 层不得 import `api/*/v1` proto 包 | proto 映射只在 Service 层；biz 定义端口接口 |
| 16 | 禁止使用 `log/slog` 记录日志 | 统一使用 `pkg/loggateway.Logger`（`lg.Info/Warn/Error` + `loggateway.StepID/Err/Str`）；`event.SysLog*` / `event.SessionSysLog*` 已废弃 |
| 17 | 跨模块调用不得持有对方 Service 具体类型 | 通过 biz 级窄接口（端口）交互，Wire 绑定在 Service 层 |
| 18 | Graph 运行时类型不得泄漏到 biz | biz 暴露 `GraphBuildConfig`/`GraphRuntime`/`GraphExecutor` 端口 |
| 19 | 不得新增已无调用者的 deprecated 方法 | 死代码即删，不保留 Deprecated 标记 |

---

## 第三章：代码探索约束

> 本项目已配置 CodeGraph MCP。**编码前先查结构，禁止盲目 grep 扫库。**

| # | 约束 | 说明 |
|---|------|------|
| C1 | 结构性查询**必须优先 CodeGraph** | 符号定义、调用链、影响面、模块上下文 → `codegraph_*` 工具 |
| C2 | **禁止**按符号名 grep 先于 CodeGraph | `codegraph_search` 一次返回 kind + 位置 + 签名 |
| C3 | **禁止**用 grep 重复验证 CodeGraph 结构结果 | AST 索引为准；浪费 token 且更易漏 |
| C4 | grep / Read **仅用于**非结构场景 | 字符串字面量、注释、日志文案；或已定位文件内的局部阅读 |
| C5 | 需要模块全貌时用 `codegraph_context` 或 `codegraph_explore` | 不要 `codegraph_search` + 多次 Read 拼装 |
| C6 | 索引缺失时先问用户是否 `codegraph init -i` | 未初始化前可退回 grep，但应提示初始化 |

**工具选型速查**：

| 问题 | 工具 |
|------|------|
| "X 定义在哪？" / "查找符号 X" | `codegraph_search` |
| "谁调用了函数 Y？" | `codegraph_callers` |
| "Y 调用了什么？" | `codegraph_callees` |
| "改 Z 会影响什么？" | `codegraph_impact` |
| "看 Y 的签名/源码" | `codegraph_node` |
| "获取任务/领域的聚焦上下文" | `codegraph_context` |
| "调研陌生模块/主题" | `codegraph_explore` |

---

## 第四章：决策树

### 4.1 代码该放哪？

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
├─ 新增横切关注点（鉴权/中间件）→ internal/server + pkg/auth
│
└─ 新增前端页面 ─────────────────→ 见 docs/guides/frontend-guide.md
```

### 4.2 OOP 抽象选型

> 通用 Go OOP 规范（接口设计、组合嵌入、设计模式等）见 `go-oop-guide` SKILL。

```
需要多态？         → 接口
需要代码复用？     → 组合（嵌入 struct）
需要解耦模块？     → 端口-适配器（接口在 biz，实现在 data）
需要灵活构造？     → Functional Options
需要横切关注点？   → 中间件/装饰器
只在本包用？       → 不需要接口，直接用 struct
```

---

## 第五章：分层编码规范

### 5.1 Service 层——传输桥点

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

### 5.2 Biz 层——领域核心

**职责**：定义领域模型、Usecase 编排、Repo 接口。

1. **模型定义**：纯 Go struct，字段用基本类型，不用 proto 类型
2. **Repo 接口定义在 biz**，data 层实现
3. **Usecase 结构**：只接收接口或具体依赖，不接收"上帝对象"
4. **错误处理**：统一使用 `kerrors`，禁止 `fmt.Errorf`
5. **分页**：统一使用 `biz.ListOption` + `pagination.go`
6. **禁止 import**：`api/*/v1`、`pkg/trpc-agent-go` 任何包

### 5.3 Data 层——数据访问

**职责**：实现 biz 定义的 Repo 接口，封装数据库操作。

1. **数据库访问**：仅通过 `d.Ent()` / `d.Postgres()`，禁止另开连接
2. **Ent 转换函数**：`entXxxToBiz` / `bizXxxToEnt`，放在对应 Repo 文件中
3. **编译期检查**：`var _ biz.XxxRepo = (*xxxRepo)(nil)`

### 5.4 Server 层——传输注册

**职责**：创建 HTTP/gRPC/WebSocket 实例，注册 Service。

```go
v1.RegisterXxxHTTPServer(srv, svc)
v1.RegisterXxxServiceServer(srv, svc)
```

**禁止**写业务路由：`srv.Route("/v1").HandleFunc("/custom", handler)`

**中间件**：统一在 `NewHTTPServer`/`NewGRPCServer` 中注册。推荐注册顺序：`recovery → tracing → logging → auth`

---

## 第六章：Agent 运行时规范

### 6.1 框架真相源

**`pkg/trpc-agent-go` 是 Agent 框架的唯一真相源。**

| 原则 | ✅ 正确 | ❌ 错误 |
|------|---------|---------|
| 先查框架 API | 查 `pkg/trpc-agent-go` 的 Runner/Agent/Tool API 后再实现 | 在 biz 重写运行时逻辑 |
| 不复制框架 | 调用框架 API | 把框架内部实现整块复制到业务目录 |
| 编排语义归框架 | Runner/Agent/Tool/Session/Event 在框架中 | 在业务包中平行维护编排逻辑 |

### 6.2 运行时装配层次

```
internal/service        ← Runner 装配入口（调 agent/team/tools）
internal/agent          ← Agent 构建（BuildLLMAgent、Memory、Plugins）
internal/team           ← Team 工作流（BuildWorkflowRoot、Runner）
internal/tools          ← 工具注册中心 + Assemble 装配（Registry + AssemblyConfig）
internal/tools/trpc     ← 向后兼容适配层（ToolsetConfig → AssemblyConfig → Assemble）
internal/mcp/config|probe|metadata|health ← MCP 配置解析、探活、元数据、定时健康检查
internal/agent/tool_assembly.go ← Agent 回合 MCP 解析与 OAuth 头注入
internal/tools/toolset.go      ← MCPToolSet / MCPBroker 装配
internal/tools/skillruntime    ← Skill 工具集解析
internal/tools/skillrouter     ← Skill 检测与分类
internal/tools/custom         ← 自定义工具实现
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
```

### 6.3 Agent 构建规范

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

### 6.4 工具装配规范

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

### 6.5 Team 编排规范

| 模式 | 实现方式 | 适用场景 |
|------|----------|----------|
| Coordinator | 协调者 Agent 调度成员作为工具 | 需要中央决策 |
| Swarm | 成员间 `transfer_to_agent` 传递控制权 | 自由协作 |

**编码规则**：
1. Team Runner 在 `internal/team`，不溢出到 service 或 biz
2. 成员 Agent 独立构建：每个成员用自己的 Settings、Skill 策略、MCP 服务器列表
3. 事件流通过 `biz.TeamRunEventBroker` 发布 WebSocket

### 6.6 记忆系统规范

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

### 6.7 Provider 集成约定

| 原则 | ✅ 正确 | ❌ 错误 |
|------|---------|---------|
| 厂商连接收口 | `internal/provider` 承载初始化、解析、调用 | 在 agent/service 中直接写 HTTP 客户端 |
| 契约对齐 | 入参/出参以 `pkg/trpc-agent-go/model` 为准 | 在业务包中平行维护另一套驱动接口 |
| 新增厂商 | 扩展 `Registry` + 子包实现 `model.LLM` | 在 agent 中硬编码厂商 URL |

### 6.8 Stream 流式工具规范

**框架三层 Tool 接口**：`Tool`（Declaration）→ `CallableTool`（+Call）→ `StreamableTool`（+StreamableCall）

**执行流程**：框架自动根据接口类型分派 Call 或 StreamableCall。流式工具必须以 `FinalResultChunk` 或 `FinalResultStateChunk` 结束。

### 6.9 Agent-as-Tool 与 MCP Broker 规范

**Agent-as-Tool**（`trpcagenttool.NewTool`）：子 Agent 作为工具，支持 SkipSummarization、StreamInner、HistoryScope、ResponseMode 等选项。

**MCP Broker**（`trpcmcpbroker.New`）：4 个运行时发现工具（`mcp_list_servers`、`mcp_list_tools` 等）。`AllowAdHocHTTP` 默认 false。

### 6.10 框架核心接口速查

**Agent 接口**：

```go
type Agent interface {
    Run(ctx context.Context, invocation *Invocation) (<-chan *event.Event, error)
    Tools() []tool.Tool
    Info() Info
    SubAgents() []Agent
    FindSubAgent(name string) Agent
}
```

**Runner 接口**：

```go
type Runner interface {
    Run(ctx, userID, sessionID string, message model.Message, runOpts ...RunOption) (<-chan *event.Event, error)
    Close() error
}
type ManagedRunner interface {
    Runner
    Cancel(requestID string) bool
    RunStatus(requestID string) (RunStatus, bool)
}
type SteerableRunner interface {
    ManagedRunner
    EnqueueUserMessage(requestID string, message model.Message) error
}
```

**Model 接口**：

```go
type Model interface {
    GenerateContent(ctx context.Context, request *Request) (<-chan *Response, error)
    Info() Info
}
```

**Tool 接口**：

```go
type Tool interface { Declaration() *Declaration }
type CallableTool interface { Call(ctx, jsonArgs) (any, error); Tool }
type StreamableTool interface { StreamableCall(ctx, jsonArgs) (*StreamReader, error); Tool }
```

**Memory Service 接口**：

```go
type Service interface {
    AddMemory(ctx, userKey, memory, topics, ...AddOption) error
    UpdateMemory(ctx, memoryKey, memory, topics, ...UpdateOption) error
    DeleteMemory(ctx, memoryKey) error
    ClearMemories(ctx, userKey) error
    ReadMemories(ctx, userKey, limit) ([]*Entry, error)
    SearchMemories(ctx, userKey, query, ...SearchOption) ([]*Entry, error)
    Tools() []tool.Tool
    EnqueueAutoMemoryJob(ctx, sess) error
    Close() error
}
```

### 6.11 框架包 → 项目包对照

| 框架包 | 项目包 | 职责 |
|--------|--------|------|
| `agent/llmagent` | `internal/agent` | Agent 构建（BuildTRPCLLMAgent） |
| `runner` | `internal/agent` | Runner 创建（NewTRPCRunner） |
| `model/*` | `internal/provider` | LLM 模型适配（TRPCModelForProviderModel） |
| `session/*` | `internal/session` | 会话存储适配 |
| `memory/*` | `internal/memory` | 记忆服务适配 |
| `tool/*` | `internal/tools/trpc` | 工具集适配 |
| `skill` | `internal/skill/trpc` | 技能仓库适配 |
| `team` | `internal/team` | Team 编排 |
| `graph` | `internal/graph` | 图编排 |
| `plugin` | `internal/agent` | 插件注册（DefaultRunnerPlugins） |
| `event` | `internal/service` | 事件投影为 Envelope → EventBus → `/v1/ws` |

### 6.12 关键桥接函数

| 桥接函数 | 位置 | 作用 |
|----------|------|------|
| `BuildTRPCLLMAgent` | `internal/agent/trpc_build.go` | biz.Agent → trpcagent.Agent |
| `NewTRPCRunner` | `internal/agent/trpc_runtime.go` | 创建 Runner 并注入 Session/Memory |
| `RunTRPCUserTurn` | `internal/agent/trpc_runtime.go` | 执行一轮用户对话 |
| `TRPCModelForProviderModel` | `internal/provider/trpc_llm.go` | biz Provider 配置 → trpcmodel.Model |
| `BuildWorkflowRoot` | `internal/team/` | biz Team → trpcagent.Agent（Team 模式） |

---

## 第七章：项目代码风格

> 通用 Go 代码风格（命名、函数设计、错误处理、并发等）见 `go-oop-guide` SKILL。本章只列**项目特定约束**。

### 7.1 错误处理（项目约束）

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

Biz 层错误变量：

```go
var (
    ErrNotFound     = kerrors.NotFound("AGENT", "agent not found")
    ErrInvalidInput = kerrors.BadRequest("AGENT", "invalid input")
)
```

### 7.2 依赖注入（项目约束）

1. **Wire ProviderSet**：每层一个，在 `biz.go`/`data.go`/`service.go`/`server.go` 中定义
2. **构造函数参数**：只接收接口或具体依赖，不接收 `*Data` 之外的"上帝对象"
3. **禁止手动编辑 `wire_gen.go`**：必须通过 `make wire` 生成
4. **`cmd/admin/wire.go` 只做组装**：`wire.Build(...)` + 少量跨层 `provide*`；**禁止**在 provider 内做进程级全局注册
5. **全局/副作用注册归属**：

| 场景 | 正确位置 | 禁止 |
|------|----------|------|
| Repo 就绪后注册 biz 解析器 | `data.NewXxxRepo` / `biz.NewXxxUsecase` 构造函数 | `provideXxxBootstrap` + 占位 `*Bootstrap` 类型 |
| EventBus 等进程单例 | `main.newApp` 的 `kratos.BeforeStart` / `AfterStop` | `cmd/admin/wire.go` 内 `SetGlobal*` |
| 观测/工具包全局钩子 | 对应包的 `New*` 或 `service` 层构造函数 | Wire provider 内 `mcpobserve.Set*` 等 |

6. 改 Wire 后本地 **`make wire-clean`**；PR 须通过 CI `wire-clean` job

**反模式示例（已禁止）**：

```go
// ❌ cmd/admin/wire.go — 占位类型 + 全局副作用
type credentialKeyBootstrap struct{}
func provideCredentialKeyBootstrap(repo biz.SystemSettingRepo) *credentialKeyBootstrap {
    biz.SetCredentialKeyResolver(...)
    return &credentialKeyBootstrap{}
}

// ✅ internal/data/system_setting.go — Repo 构造时注册
func NewSystemSettingRepo(d *Data) biz.SystemSettingRepo {
    repo := &systemSettingRepo{data: d}
    biz.SetCredentialKeyResolver(func(ctx context.Context) ([]byte, error) {
        return biz.ResolveCredentialAESKey(ctx, repo)
    })
    return repo
}
```

### 7.3 并发（项目约束）

| 规则 | ✅ 正确 | ❌ 错误 |
|------|---------|---------|
| context 传递 | 所有跨层调用必须传递 `ctx` | `go func() { doWork() }()` 不传 ctx |
| goroutine | 必须走 `pkg/safego.Go` / `pkg/safego.GoRecover`（红线 13） | 裸 `go func()` 不处理 panic |
| MCP 子进程 | context 取消时清理 | 子进程泄漏 |
| WebSocket 流 | 处理客户端断连 | 不检测客户端断连 |
| 共享状态 | `sync.Mutex`/`sync.RWMutex` | 全局变量 |

### 7.4 日志（项目约束）

**禁止 `log/slog`**，统一使用 `pkg/loggateway.Logger`：

- `x.lg.Info/Warn/Error(msg, loggateway.StepID("..."), loggateway.Err(err))` — 结构化日志
- `x.lg.With(loggateway.SessionID(sid))` — 绑定会话上下文
- `loggateway.Global()` — 独立函数使用全局 Logger
- `event.SysLog*` / `event.SessionSysLog*` — **已废弃**，禁止新增调用

### 7.5 命名（项目补充）

> 通用命名规则见 `go-oop-guide` SKILL §十二。以下为本项目补充。

| 场景 | 规范 | ✅ 示例 |
|------|------|---------|
| 文件名 | 小写+下划线，按职责拆分 | `agent_repo.go`, `trpc_build.go` |
| 类型转换 | 独立函数 | `toProtoXxx`/`fromProtoXxx` |
| 构造函数 | 统一 `NewXxx`，返回指针 | `NewAgentUsecase` |

---

## 第八章：API 与 Proto 规范

### 8.1 Proto 定义规则

| 规则 | ✅ 正确 | ❌ 错误 |
|------|---------|---------|
| 路径 | `api/kratos/<module>/v1/<module>.proto` | 随意放置 |
| HTTP 注解 | 每个 RPC 配 `google.api.http` | 只定义 RPC 不配 HTTP path |
| 必填标记 | `(google.api.field_behavior) = REQUIRED` | 不标记必填 |
| 命名 | proto 字段 `snake_case`，Go 生成 `CamelCase` | proto 字段用 camelCase |
| 契约完整性 | 全部能力在 proto 中定义 | 一半在 proto，一半手写路由 |
| 请求/响应 | `XxxReq` / `XxxReply` | 随意命名 |

### 8.2 代码生成流程

```bash
make init    # 首次安装插件
make api     # 生成 Go + TypeScript
make wire    # 生成 Wire 依赖注入
make config  # 仅改 conf.proto 时
```

**禁止修改工具生成的代码（红线 11）。**

### 8.3 迁移与迭代硬约束

1. 对外能力必须在 Proto 中印全，禁止「一半在 proto，一半用手写 srv.Route / HandleFunc」
2. 修改 `.proto` 必须跑生成，提交全部生成物
3. SQLite 侧以 `*ent.Client` 为主入口，禁止在 `NewData` 里再 `sql.Open` 同一 DSN
4. 表结构进 Ent，禁止长期平行维护「仅存 SQL、不进 Ent」
5. 业务模块 HTTP 只做 `Register<Module>HTTPServer`，gRPC 只做 `Register<Module>ServiceServer`

---

## 第九章：模块化设计

### 9.1 新增功能模块的标准结构

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

### 9.2 模块间通信

| 方式 | ✅ 正确 | ❌ 错误 |
|------|---------|---------|
| 同步调用 | Usecase 之间通过接口调用 | 直接 import 另一模块的 data |
| 异步事件 | 通过 `Broker` 发布/订阅 | 通过全局变量共享状态 |
| 状态共享 | 数据库 | 包级变量 |
| 跨模块调用 | 通过 biz 级窄接口（端口） | 持有对方 Service 完整具体类型 |

### 9.3 模块解耦端口

| 模块 | 端口 | 用途 | 位置 |
|------|------|------|------|
| Channel → Chat | `biz.NativeTurnGateway` | 同步 Turn 执行 + 运行控制 | `internal/biz/turn_input.go` |
| Channel → Graph | `biz.GraphExecutor` | Graph 执行（返回 executionID） | `internal/biz/graph.go` |
| Team → Chat | `biz.TurnInput` | Team turn 输入（proto 映射在 service） | `internal/biz/turn_input.go` |
| Graph → Biz | `biz.GraphBuildConfig` / `biz.GraphRuntime` | Graph 配置与运行时端口 | `internal/biz/` |
| Graph → Resolver | `build_deps.go` 接口 | Agent/Tool/Model resolver 分离注入 | `internal/graph/trpc/build_deps.go` |

**端口设计原则**：

1. **接口定义在 biz 层**：消费方 import biz 接口，不 import 具体实现
2. **Wire 绑定在 service 层**：`wire.Bind(new(biz.XxxPort), new(*XxxService))`
3. **返回值用 biz 类型**：端口方法返回 `string`/`biz.Xxx`，不返回 proto 类型
4. **构造函数收窄**：只接收需要的端口，不接收"上帝对象"

### 9.4 配置管理

1. **配置来源优先级**：环境变量 > 系统设置 > 配置文件 > 代码默认值
2. **配置结构**：在 `internal/conf/conf.proto` 中定义
3. **热更新**：通过 Kratos config source 支持，不自行实现 watch

---

## 第十章：任务速查卡

### 新增 API

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

### 新增工具

```
1. internal/tools/registry.go              ← Registry() 中注册 ToolRegistration
2. internal/tools/builtin_tools_seed.go    ← 添加种子数据
3. 需要配置 → AssemblyConfig 增加字段 + Assemble 中增加覆盖逻辑
4. internal/tools/custom/                  ← 自定义工具实现（如需）
5. Chat 和 Team 共用 BuildToolsets，验证两处生效
```

### 新增数据实体

```
1. internal/data/ent/schema/xxx.go         ← Fields/Index/Edge
2. go generate ./internal/data/ent         ← 生成 Ent 代码
3. internal/data/xxx.go                    ← Repo 实现（entXxxToBiz / bizXxxToEnt）
4. internal/biz/xxx.go                     ← 模型 + Repo 接口 + Usecase
5. internal/biz/biz.go                     ← ProviderSet 添加 NewXxxUsecase
6. internal/data/data.go                   ← ProviderSet 添加 NewXxxRepo
```

### 新增 LLM Provider

```
1. internal/provider/trpc_llm.go           ← MapProviderType 中添加映射
2. 如需自定义选项 → buildProviderOptions 中添加分支
3. 确保框架 model/<provider>/ 包已实现 model.Model 接口
```

### 新增 Session/Memory 后端

```
1. internal/session/ 或 internal/memory/   ← 创建适配器
2. 通过 Wire 注入到 Runner 的 SessionService/MemoryService
3. 框架侧实现 session.Service / memory.Service 接口
```

---

## 第十一章：AI 编码自检清单

### 改动前（代码探索）

- [ ] **结构性问题已用 CodeGraph**：符号 / 调用链 / 影响面 / 模块上下文，未 grep 先于 `codegraph_*`
- [ ] **探索结果可信**：未对 CodeGraph 返回的结构信息做重复 grep 验证
- [ ] **已读模块交叉参考手册**：在 `docs/module-cross-reference.md` 中找到目标模块卡片，确认上游依赖、下游影响、共享契约、事件、数据库、前端对应

### 改动中（逐层检查）

- [ ] **Service 层**：只做映射和编排，无业务逻辑
- [ ] **Biz 层**：无 `pkg/trpc-agent-go` import，无 proto import
- [ ] **Data 层**：仅 `Ent()`/`Postgres()` 访问，无并联 SQLite 连接
- [ ] **Agent/Tools/Team 层**：框架 API 调用合规，不复制框架内部逻辑
- [ ] **模块解耦**：跨模块调用走 biz 级窄接口，不持有对方 Service 具体类型
- [ ] **Channel**：不 import `graphv1` 等 proto 包，不持有 `*ChatService`
- [ ] **Team**：不 import chat proto，输入用 `biz.TurnInput`
- [ ] **Graph**：biz 层不见 trpc graph 类型，resolver 通过接口注入
- [ ] **新增工具**：先在 `Registry()` 注册 `ToolRegistration`，再在 `builtin_tools_seed.go` 添加种子
- [ ] **流式工具**：实现 `StreamableTool` 接口，必须发送 `FinalResultChunk`
- [ ] **记忆工具**：通过 `memory.Service.Tools()` 注入，不手动构造
- [ ] **MCP Broker**：`AllowAdHocHTTP` 默认 false，安全边界明确
- [ ] **错误处理**：使用 `kerrors`，不用 `fmt.Errorf`
- [ ] **日志**：使用 `loggateway.Logger`，不用 `log/slog`，不用 `event.SysLog*`
- [ ] **goroutine**：走 `pkg/safego`，无裸 `go func()`
- [ ] **OOP 合规**：见 `go-oop-guide` SKILL（接口方法 ≤ 5、接口定义在使用方、返回具体类型参数接收接口、无上帝对象注入）

### 改动后（构建与验证）

- [ ] `make api` 已执行（如改了 proto）
- [ ] `make wire` 已执行（如改了 Wire 声明）
- [ ] `make wire-clean` 通过（`wire_gen.go` 与 `wire.go` 同步）
- [ ] `make lint` 通过（含 R11：wire.go 无全局 bootstrap）
- [ ] `go build ./cmd/admin` 通过
- [ ] 无红线违反

### 全链路合并检查

- [ ] `api/**/*.proto` 覆盖本迭代全部 `/v1` 能力，`make api`，Go + TS 已提交
- [ ] `internal/biz` / data / service / server 合规；Wire + `go build ./cmd/admin`
- [ ] `web/src/services/index.ts` `createXXXService`
- [ ] 前端合规：见 `docs/guides/frontend-guide.md` 自检清单

---

## 第十二章：验证命令

| 改动类型 | 最小验证 |
|----------|----------|
| 仅 Service + 单测 | `go test ./internal/service/... -run TestXxx -count=1` |
| 仅 Biz / Data | `go test ./internal/biz/... ./internal/data/... -count=1` |
| Proto 变更 | `make api && go build ./...` |
| Wire 注入 | `make wire && go build ./cmd/admin` |
| 前端 | `cd web && pnpm lint && pnpm test && pnpm build` |
| **提交前（全量）** | 后端：`make api && make wire && make build && make test && make lint`；前端：`cd web && pnpm lint && pnpm test && pnpm build` |

---

## 附录：关键文件索引

| 文件 | 用途 |
|------|------|
| `internal/tools/toolset.go` | 工具注册中心（Registry + AssemblyConfig + Assemble） |
| `internal/tools/tool.go` | 项目级工具类型别名 |
| `internal/tools/trpc/toolsets.go` | 向后兼容适配层 |
| `internal/memory/trpc/sqlite_adapter.go` | Memory Service SQLite 适配器 |
| `internal/agent/trpc_build.go` | Agent 构建 + 工具集装配入口 |
| `internal/agent/trpc_runtime.go` | Runner 创建 + 用户 Turn 执行 |
| `internal/provider/trpc_llm.go` | LLM 模型驱动 |
| `internal/biz/turn_input.go` | Turn 输入端口定义 |
| `internal/biz/graph.go` | Graph 端口定义 |
| `pkg/trpc-agent-go/docs/mkdocs/zh/` | 框架官方文档（深度用法时查阅） |

---

## 第十三章：模块关联强制检查

> **模块不是孤岛。改任何模块前，必须先读关联文档。** 违反即停。

### 13.1 关联文档索引

| 文档 | 路径 | 定位 |
|------|------|------|
| **架构蓝图** | `docs/architecture-blueprint.md` | "每个模块是什么"（静态结构、全貌） |
| **模块交叉参考** | `docs/module-cross-reference.md` | "改模块 X 时必须注意谁"（动态关联、影响面） |

### 13.2 开发前强制步骤

```
步骤 1：定位目标模块 → 读蓝图对应章节（了解静态结构）
步骤 2：读交叉参考手册 → 找到目标模块卡片（8 维度关联）
步骤 3：查变更影响表 → 确定需要同步修改的文件清单
步骤 4：按依赖方向逐层修改 → 验证时覆盖所有影响面
```

### 13.3 场景→章节速查

| 你要做什么 | 必须读的交叉参考章节 |
|-----------|---------------------|
| 新增/修改 biz 端口接口 | §四·端口接口变更影响表 + 目标模块卡片「下游影响」 |
| 新增/修改共享类型（DTO/struct） | §四·共享类型变更影响表 + 目标模块卡片「共享类型」 |
| 新增/修改事件类型（Envelope/EventBus） | §四·事件类型变更影响表 + 目标模块卡片「事件生产/消费」 |
| 新增/修改数据库 Schema | §四·数据库 Schema 变更影响表 + 目标模块卡片「数据库」 |
| 新增 LLM Provider | §五·场景演练 #1（7 个模块链路） |
| 新增工具 | §五·场景演练 #2（6 个模块链路） |
| 新增渠道平台 | §五·场景演练 #3（5 个模块链路） |
| 新增 Envelope 类型 | §五·场景演练 #4（前后端 4 层链路） |
| 修改 TurnInput/TurnResult | §五·场景演练 #5（最大影响面 8 模块） |

### 13.4 典型遗漏案例

| 遗漏 | 后果 |
|------|------|
| 改了 biz 接口签名但忘了 data 层实现 | 编译不通过 |
| 改了 AssemblyConfig 但忘了 service/chat 的 Wire 构造函数 | 运行时 nil pointer |
| 新增 Envelope 类型但没加前端 dispatcher 处理 | WS 消息丢失 |
| 加了 Ent 字段但没跑 go generate | ORM 不认识新列 |
| 改了 TRPCBuilderDeps 但没同步 team/trpc_build.go | Team 功能崩溃 |
| 新增工具但只在 Chat 验证没在 Team 验证 | Team 场景缺工具 |
