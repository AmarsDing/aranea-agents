# Aranea-Agents AI 开发规范

> **文档地位**：AI 编码时的**唯一行为准则**。
> **规范冲突优先级**：本文 > 所有其他 docs 下的规范文档。
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
- [第七章：前端编码规范](#第七章前端编码规范)
- [第八章：UI/UX 执行规范](#第八章uiux-执行规范)
- [第九章：AI 编码自检清单](#第九章ai-编码自检清单)
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
| 11 | 不得修改 protoc 生成代码 | 改 proto → `make api` → 提交生成物 |
| 12 | 不得在 Server 层写业务路由或手写 `HandleFunc` | 只做 `Register*HTTPServer`/`Register*ServiceServer` |

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
├─ 新增前端页面 ─────────────────→ services/ → features/ → stores/ → pages/ → components/
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

#### 新增前端页面

```
1. web/src/services/index.ts               ← createXxxService
2. web/src/features/<domain>/api.ts        ← HTTP 调用封装
3. web/src/stores/<domain>/                ← Pinia Store（actions 触发请求）
4. web/src/pages/                          ← 页面路由 + 布局
5. web/src/components/<domain>/            ← 展示组件（props in / emits out）
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

> 本节合并了所有 import 约束与红线，是依赖方向的**唯一权威来源**。

| 包路径 | ✅ 允许 import | ❌ 禁止 import |
|--------|----------------|----------------|
| `internal/server/*` | `internal/service`、`internal/conf`、kratos、`pkg/auth`、`pkg/validate` | `pkg/trpc-agent-go` / 框架运行时私有 import、`runner.Runner`、`llmagent.New` |
| `internal/biz/*` | stdlib、kratos errors、本仓 biz/data API | `pkg/trpc-agent-go` 任何包、`api/*/v1`、框架运行时 toolset/skill 类型 |
| `internal/service/*` | `internal/biz`、`internal/team`、`internal/agent`、`internal/session/trpc`、`internal/provider`、`internal/tools`，框架 Runner/Agent 装配 API | 绕过 `internal/tools` 大量直连拼装底层 `tool` |
| `internal/agent/*` | `internal/biz`、`internal/provider`、`internal/data/...`（如需）、`internal/session/trpc`、`pkg/trpc-agent-go` / 框架运行时 | — |
| `internal/team/*` | `internal/biz`、`internal/agent`、`internal/provider`、`internal/tools`、`pkg/trpc-agent-go` / 框架运行时 | — |
| `internal/provider/*` | `internal/biz`、`pkg/trpc-agent-go` / 框架 `model` 适配 | — |
| `internal/tools/*` | `internal/biz`、框架 `tool` API（由 `pkg/trpc-agent-go` 暴露或兼容层 re-export） | — |

**正反对照**：

```go
// ✅ 正确：biz 层只依赖 stdlib + kratos errors + 本仓 biz/data
import (
    "context"
    stderrors "errors"
    kerrors "github.com/go-kratos/kratos/v2/errors"
    "aranea-agents/internal/biz"
)

// ❌ 错误：biz 层 import 框架运行时
import (
    "aranea-agents/pkg/trpc-agent-go/runner"    // 红线 2
    "aranea-agents/pkg/trpc-agent-go/tool"       // 红线 8
    "aranea-agents/api/chat/v1"                   // biz 禁止 import proto
)
```

---

## 第二章：分层编码规范

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
// ✅ 正确：Service 是框架调用的唯一桥点
func (s *ChatService) SendChatMessage(ctx context.Context, req *chatv1.SendChatMessageRequest) (*chatv1.SendChatMessageResponse, error) {
    // 1. proto → biz 参数
    // 2. 调 biz Usecase 获取 Agent/Session
    // 3. 调 internal/agent 构建 Agent
    // 4. 调 internal/agent 构建 Runner
    // 5. runner.Run → 事件流 → 投影为 proto 响应
}

// ❌ 错误：在 server 层或 biz 层直接使用框架运行时
// internal/server/http.go 中 new runner.Runner  → 红线 1
// internal/biz/agent.go 中 import runner        → 红线 2
```

**桥接约定**：`internal/service` 内 Kratos service 在方法中构造框架 `Runner`，将 RPC/HTTP 请求译为会话执行入口，将会话事件流投影为 unary 或 WebSocket。**不在 `internal/server` 或 `internal/biz` 中直接使用框架运行时。**

### 2.2 Biz 层——领域核心

**职责**：定义领域模型、Usecase 编排、Repo 接口。

**编码规则**：

1. **模型定义**：纯 Go struct，字段用基本类型，不用 proto 类型

```go
// ✅ 正确：纯 Go 类型
type Agent struct {
    ID          string
    AgentKey    string
    DisplayName string
    Provider    string
    Model       string
    Status      string
    Settings    *AgentRuntimeSettings
}

// ❌ 错误：使用 proto 类型
type Agent struct {
    ID       string
    Status   chatv1.AgentStatus   // 禁止：biz 不得 import proto
}
```

2. **Repo 接口定义在 biz**：

```go
type AgentRepository interface {
    GetAgentByID(ctx context.Context, id string) (Agent, error)
    SearchAgents(ctx context.Context, q AgentListQuery) (AgentListResult, error)
    CreateAgent(ctx context.Context, a Agent) (Agent, error)
    UpdateAgent(ctx context.Context, a Agent) (Agent, error)
    DeleteAgent(ctx context.Context, id string) error
}
```

3. **Usecase 结构**：

```go
type AgentUsecase struct {
    repo  AgentRepository
    tools ToolRepo
}

func NewAgentUsecase(repo AgentRepository, tools ToolRepo) *AgentUsecase {
    return &AgentUsecase{repo: repo, tools: tools}
}
```

4. **错误处理**：

```go
// ✅ 正确：使用 kratos errors
if id == "" {
    return Agent{}, kerrors.BadRequest("AGENT", "id is required")
}
if stderrors.Is(err, sql.ErrNoRows) {
    return Agent{}, kerrors.NotFound("AGENT", "agent not found")
}

// ❌ 错误：使用 fmt.Errorf
return Agent{}, fmt.Errorf("agent not found: %w", err)
```

5. **分页**：统一使用 `biz.ListOption` + `pagination.go` 的 `ListOffset/ListLimit/ListFilter/ListOrderBy`
6. **禁止 import**：`api/*/v1`、`pkg/trpc-agent-go` 任何包（见 §1.3）

### 2.3 Data 层——数据访问

**职责**：实现 biz 定义的 Repo 接口，封装数据库操作。

**编码规则**：

1. **Repo 结构体**：

```go
type agentRepo struct {
    data *Data
}

func NewAgentRepo(d *Data) biz.AgentRepository {
    return &agentRepo{data: d}
}
```

2. **数据库访问**：

```go
// ✅ 正确：通过 Data 访问
d.Ent()       // SQLite → *ent.Client
d.Postgres()  // pgvector → *sql.DB

// ❌ 错误：另开连接
sql.Open("sqlite3", dsn)  // 红线 10：不得在 NewData 外另开 SQLite
```

3. **Ent 转换函数**：`entXxxToBiz` / `bizXxxToEnt`，放在对应 Repo 文件中
4. **新增实体流程**：见[任务速查卡 - 新增数据实体](#新增数据实体)

### 2.4 Server 层——传输注册

**职责**：创建 HTTP/gRPC/WebSocket 实例，注册 Service。

```go
// ✅ 正确：只做注册
v1.RegisterXxxHTTPServer(srv, svc)
v1.RegisterXxxServiceServer(srv, svc)

// ❌ 错误：写业务路由
srv.Route("/v1").HandleFunc("/custom", handler)  // 红线 12
```

**中间件**：统一在 `NewHTTPServer`/`NewGRPCServer` 中注册。

---

## 第三章：Agent 运行时规范

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
// 1. Service 层组装 BuilderDeps
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

// 2. 构建 Agent
root, err := agent.BuildLLMAgent(ctx, ag, deps)

// 3. 构建 Runner
runner, err := agent.NewTRPCRunnerForRuntime(root, sessSvc, s.runtime)

// 4. 执行
eventCh, err := runner.Run(ctx, userID, sessionID, userMessage)
```

**规则**：

| 规则 | 说明 |
|------|------|
| BuilderDeps 是 DTO | 不含框架运行时类型，只含 biz 模型 + 可选依赖标记 |
| Memory 由 Wire 注入 | 通过 `runtimedeps.Runtime.SessionMemory`，不在 Service 手动选择 |
| 工具统一挂载 | 通过 `TurnMount.Attach`，不分散在多处 |

### 3.4 工具装配规范

**核心装配入口**：`internal/tools/toolset.go` 的 `Assemble(ctx, cfg)`

**适配层入口**：`internal/tools/trpc/toolsets.go` 的 `BuildToolsets(ctx, cfg)`

**调用链**：

```
trpc_build.go:buildToolsetsForAgent
  → 构造 ToolsetConfig（基于 effective tool keys）
  → tooltrpc.BuildToolsets(ctx, cfg)
    → tools.Assemble(ctx, assemblyCfg)
      → 遍历 Registry() 匹配 enabled tools
      → 调用 Factory/ToolSetFactory 实例化
      → 处理 AgentToolConfig、MCPServerConfig、MCPBrokerConfig
      → 追加 CustomTools
      → 返回 AssembledToolsets{ToolSets, Tools}
```

**装配顺序**（在 `Assemble` 内部）：
1. Registry 注册工具（按 enabled 列表匹配，调用 Factory/ToolSetFactory）
2. 带配置覆盖的工具（file→WithBaseDir、geminifetch→WithModel 等）
3. OpenAPI Spec ToolSet
4. workspace_exec 扩展工具（write_stdin、kill_session）
5. AgentTool（`AgentToolConfig` → `trpcagenttool.NewTool`）
6. MCP ToolSet（`MCPServerConfig` → `trpcmcp.NewMCPToolSet`）
7. MCP Broker（`MCPBrokerConfig` → `trpcmcpbroker.New` → `broker.Tools()`）
8. CustomTools

**AssemblyConfig 关键字段**：

```go
type AssemblyConfig struct {
    EnabledTools  []string
    FilesystemDir string
    GeminiModel   string
    GoogleAPIKey  string
    GoogleCX      string
    ClaudeCodeDir string
    OpenAPISpecs  []OpenAPISpecConfig
    AgentTools    []AgentToolConfig
    MCPServers    []MCPServerConfig
    MCPBroker     *MCPBrokerConfig
    CustomTools   []Tool
}
```

**规则**：

| # | 规则 | ✅ 正确 | ❌ 错误 |
|---|------|---------|---------|
| 1 | 新增工具先注册 | `Registry()` 注册 `ToolRegistration` + `builtin_tools_seed.go` 种子 | 直接在 Service 中手写 tool 实例 |
| 2 | 需配置的工具 | `AssemblyConfig` 增加字段 + `Assemble` 增加覆盖逻辑 | 硬编码配置值 |
| 3 | Chat/Team 共用 | 同一 `BuildToolsets` 逻辑 | Chat 和 Team 各写一套装配 |
| 4 | 工具策略 | biz 层解析为 effective tool keys，tools 层只做框架映射 | tools 层解析 allow/deny 策略 |
| 5 | 适配层职责 | `ToolsetConfig` → `AssemblyConfig` → `Assemble` | 适配层直接拼装底层 tool |

### 3.5 Team 编排规范

**两种模式**：

| 模式 | 实现方式 | 适用场景 |
|------|----------|----------|
| Coordinator | 协调者 Agent 调度成员作为工具 | 需要中央决策 |
| Swarm | 成员间 `transfer_to_agent` 传递控制权 | 自由协作 |

**编码规则**：

1. **Team Runner 在 `internal/team`**：不溢出到 service 或 biz
2. **成员 Agent 独立构建**：每个成员用自己的 Settings、Skill 策略、MCP 服务器列表
3. **事件流通过 `biz.TeamRunEventBroker`** 发布 WebSocket

### 3.6 记忆系统规范

**框架记忆架构**（`pkg/trpc-agent-go/memory`）：

| 组件 | 职责 |
|------|------|
| `memory.Service` | 记忆 CRUD 接口（Add/Update/Delete/Clear/Read/Search） |
| `memory/tool.ToolSet` | 6 个记忆工具（add/search/load/update/delete/clear） |
| `memory/extractor` | 自动提取（LLM 从对话中提取 fact/episode） |
| `memory.Kind` | 记忆类型：`fact`（事实）/ `episode`（情景） |

**项目当前使用**：SQLite 适配器（`internal/memory/trpc` → `NewSQLiteMemoryService`），底层使用 `internal/data/sessionmemory.Store`。框架还支持 sqlitevec/postgres/pgvector/redis/mem0 等后端，按需接入。

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
| 4 | load/preload 行为 | 与实际后端一致，后端未就绪时不在 prompt 中宣称 | 无条件宣称支持 load_memory |
| 5 | 记忆写入 | 经 broker/async 异步写 | 在 plugin 回调中直接写库（红线 3） |
| 6 | 搜索能力 | `HybridSearch`（向量+字面混合）、`Kind` 过滤、时间范围、去重 | 只做字面搜索 |

### 3.7 Provider 集成约定

| 原则 | ✅ 正确 | ❌ 错误 |
|------|---------|---------|
| 厂商连接收口 | `internal/provider` 承载初始化、解析、调用 | 在 agent/service 中直接写 HTTP 客户端 |
| 契约对齐 | 入参/出参以 `pkg/trpc-agent-go/model` 为准 | 在业务包中平行维护另一套驱动接口 |
| 业务集成 | 选厂商/流式/用量解析在 `internal/provider` | agent/team/service 中重复实现厂商逻辑 |
| 新增厂商 | 扩展 `Registry` + 子包实现 `model.LLM` | 在 agent 中硬编码厂商 URL |

### 3.8 Stream 流式工具规范

**框架三层 Tool 接口**：

```go
type Tool interface { Declaration() *Declaration }
type CallableTool interface { Call(ctx, jsonArgs) (any, error); Tool }
type StreamableTool interface { StreamableCall(ctx, jsonArgs) (*StreamReader, error); Tool }
```

**Stream 核心类型**：

| 类型 | 作用 |
|------|------|
| `tool.NewStream(bufferSize)` | 创建双向流（Reader + Writer） |
| `StreamChunk{Content, Metadata}` | 流式数据单元 |
| `FinalResultChunk{Result}` | 标记最终结构化结果 |
| `FinalResultStateChunk{Result, StateDelta}` | 最终结果 + 状态增量 |
| `StreamableFunctionTool[I,O]` | 包装流式函数为 StreamableTool |

**执行流程**：

1. 框架检测工具是否实现 `StreamableTool` 接口
2. 是 → 调用 `StreamableCall` → 返回 `StreamReader` → 循环 `Recv()` 消费
3. 遇到 `FinalResultChunk` → 保留为最终结果
4. 遇到 `FinalResultStateChunk` → 保留结果 + 发出 `StateDelta` 事件
5. 遇到 `io.EOF` → 流结束
6. 否 → 调用 `Call` → 返回同步结果

**AG-UI 集成**：`agui.WithStreamingToolResultActivityEnabled(true)` 开启后，中间结果转为 Activity 事件（类型 `tool.result.stream`，ID `tool-result-activity-` + toolCallID）。

**规则**：

| # | 规则 | ✅ 正确 | ❌ 错误 |
|---|------|---------|---------|
| 1 | 调用方式选择 | 框架自动根据接口类型分派 | 手动判断调用 Call 还是 StreamableCall |
| 2 | bufferSize | 默认即可，长时间运行的工具可适当增大 | 所有工具统一设大 bufferSize |
| 3 | 结束标记 | 流式工具必须以 `FinalResultChunk` 或 `FinalResultStateChunk` 结束 | 流结束时无 FinalResult（框架拼接所有 chunk 文本作为结果） |
| 4 | context 取消 | Writer 通过 `closed` channel 感知 Reader 取消并处理 | 忽略 context 取消 |

### 3.9 Agent-as-Tool 与 MCP Broker 规范

**Agent-as-Tool**（`trpcagenttool.NewTool`）：

```go
type AgentToolConfig struct {
    Agent             trpcagent.Agent
    Name              string
    Description       string
    SkipSummarization bool
    StreamInner       bool
    HistoryScope      trpcagenttool.HistoryScope
    ResponseMode      trpcagenttool.ResponseMode
}
```

| 选项 | 效果 |
|------|------|
| `SkipSummarization=false` | 子 Agent 输出被摘要后返回给父 Agent |
| `StreamInner=true` | 子 Agent 的流式事件转发到父级 |
| `ResponseMode=FinalOnly` | 只返回子 Agent 最后一条 assistant 消息 |
| `HistoryScope` | 控制传递给子 Agent 的对话历史范围 |

**MCP Broker**（`trpcmcpbroker.New`）——4 个运行时发现工具：

| 工具 | 功能 |
|------|------|
| `mcp_list_servers` | 列出已注册的命名 MCP 服务器 |
| `mcp_list_tools` | 连接服务器并列出工具摘要 |

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
make config  # 仅改 conf.proto 时
```

**必须提交生成物**：`*.pb.go`、`*_http.pb.go`、`*_grpc.pb.go`、`web/src/services/`

**禁止修改工具生成的代码。**

### 4.3 新增 API 检查清单

- [ ] `api/**/*.proto`：RPC + HTTP path + 请求/响应已定义
- [ ] `make api` 已执行，Go + TS 生成物已提交
- [ ] `internal/biz`：模型 + Repo 接口 + Usecase，无 `import api/...`
- [ ] `internal/data`：Ent Schema + Repo，仅 `Ent()`/`Postgres()` 访问
- [ ] `internal/service`：嵌入 `Unimplemented*`，proto ↔ biz 映射完整
- [ ] `internal/server`：`Register*HTTPServer`，无非 proto 手写业务路由
- [ ] `web/src/services/index.ts`：导出 `createXXXService`
- [ ] Wire 已更新，`go build ./cmd/admin` 通过

### 4.4 迁移与迭代硬约束

**协议（`api/**/*.proto`）**：

1. 对外能力必须在 Proto 中印全：同一业务的 service 应列出该域在 `/v1/...` 暴露的全部 RPC，并配以 `google.api.http`。禁止「一半在 proto，一半用手写 srv.Route / HandleFunc / HandlePrefix / 独立 *_route.go」
2. 修改 `.proto` 必须跑生成：根目录 `make api`，提交 `*.pb.go`、`*_http.pb.go`、`*_grpc.pb.go` 与 `web/src/services`。禁止只改契约不重生

**持久化（`internal/data`）**：

1. SQLite 侧以 `*ent.Client` 为主入口：经 `NewData` 打开的 `Ent()`，Repo 持有 `*data.Data`。禁止在 `NewData` 里再 `sql.Open` 同一 DSN 并联池化 SQLite
2. 表结构进 Ent：`internal/data/ent/schema` 声明实体；禁止长期平行维护「仅存 SQL、不进 Ent」而无说明
3. 复杂 WHERE / BLOB：优先 `predicate` + `dialect/sql`（如 `ExprP`、`And/Or`），避免整页复制裸露 SQL 与 Ent 分叉

**HTTP / gRPC 挂载（`internal/server`）**：

1. 业务模块 HTTP 只做 `Register<Module>HTTPServer(srv, svc)`，gRPC 只做 `Register<Module>ServiceServer`。禁止在同一业务域叠加未写入 proto 的手写路由
2. 横切路由（健康检查、网关、探测等）单独列出，不充当业务 `FooService` 的补丁契约

### 4.5 横切与运维边界

- **`GET /healthz`**：在 `cmd/admin` 挂载，响应 `{"status":"ok"}`；常与鉴权的 `noAuthPaths` 放行配合探针
- **`CRON_CHAT_DISPATCH_ORIGIN`**：Cron 到期任务 POST 发送聊天消息的目标根 URL（无尾部 `/`），路径为 `{origin}/v1/chat/messages`。未设置：Cron 聊天任务无法执行
- **`CRON_RUNNER_INTERVAL`**：Cron tick，`time.ParseDuration`；空或非法默认 `1m`
- **`CRON_RUNNER_DISABLED`**：设为 `1` 则不启动 `internal/cronrunner`

### 4.6 用量上报与双写

**HTTP**：`POST /v1/usage/token-events`，请求体为完整 `TokenUsageEvent`。`ctx.Bind` 使用 `encoding/json` 标签，字段名为 snake_case。

**前端注意**：`protoc-gen-typescript-http` 生成的默认体可能对嵌套消息用 `JSON.stringify` 产出 camelCase 键，与 Go `json` 标签不一致，导致静默丢字段或校验失败。此类接口应在 `features/<域>/api.ts` 中用 `kratosApi.post` 等显式构造 snake_case。

**单一写入方（避免重复计数）**：

| 场景 | 说明 |
|------|------|
| 常见风险 | 对话完成已由后端写入 `model_token_usage_events` 时，若在浏览器 `onDone` / WebSocket 结束再 POST，会对同一轮交互重复插入 |
| 目标态 | 仅服务端在同一请求路径写入用量时，浏览器不应再报同一事件 |
| 例外 | 仅当服务端确认从不写入且不重叠会话/id 时，才可单独浏览器上报；须在 PR 写明 |
| 过渡 | 若须二选一并行，应有 feature flag，默认只开一侧。禁止在未知后端是否已写时默认开启浏览器 ingest |

---

## 第五章：Go 代码风格

### 5.1 命名规范

| 场景 | 规范 | ✅ 示例 | ❌ 示例 |
|------|------|---------|---------|
| 包名 | 小写单词，不用下划线 | `agent`, `mcpmount` | `agent_svc`, `MCPMount` |
| 文件名 | 小写+下划线，按职责拆分 | `agent_repo.go`, `trpc_build.go` | `agentRepo.go`, `agent.go`（职责不清） |
| 结构体 | 大驼峰，名词 | `AgentUsecase`, `TurnMount` | `agentUsecase`, `TurnMountHandler` |
| 接口 | 大驼峰，名词+后缀 | `AgentRepository`, `MemoryService` | `AgentRepo`, `IMemory` |
| 函数 | 大驼峰导出/小驼峰内部 | `NewAgentUsecase`, `fromProtoRuntime` | `new_agent_usecase` |
| 常量 | 大驼峰导出/小驼峰内部 | `DefaultAppName`, `streamQueryKey` | `DEFAULT_APP_NAME` |
| 错误变量 | `Err` 前缀 | `ErrNotFound`, `ErrAppNameRequired` | `ErrorNotFound`, `notFoundErr` |

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
// ✅ 正确：使用 kratos errors
func (u *AgentUsecase) Get(ctx context.Context, id string) (Agent, error) {
    if id == "" {
        return Agent{}, kerrors.BadRequest("AGENT", "id is required")
    }
    a, err := u.repo.GetAgentByID(ctx, id)
    if err != nil {
        if stderrors.Is(err, sql.ErrNoRows) {
            return Agent{}, kerrors.NotFound("AGENT", "agent not found")
        }
        return Agent{}, kerrors.InternalServer("AGENT", err.Error())
    }
    return a, nil
}

// ❌ 错误：使用 fmt.Errorf
return Agent{}, fmt.Errorf("agent not found: %w", err)
```

**错误映射规则**：

| 场景 | 使用 |
|------|------|
| 参数校验失败 | `kerrors.BadRequest` |
| 记录不存在 | `kerrors.NotFound` |
| 内部错误 | `kerrors.InternalServer` |
| 框架返回的 error | `kerrors.FromError(err)` |

### 5.4 依赖注入

1. **Wire ProviderSet**：每层一个，在 `biz.go`/`data.go`/`service.go`/`server.go` 中定义
2. **构造函数参数**：只接收接口或具体依赖，不接收 `*Data` 之外的"上帝对象"
3. **禁止手动 `wire_gen.go`**：必须通过 `wire` 命令生成

### 5.5 并发与资源

| 规则 | ✅ 正确 | ❌ 错误 |
|------|---------|---------|
| context 传递 | 所有跨层调用必须传递 `ctx` | `go func() { doWork() }()` 不传 ctx |
| goroutine | 必须处理 context 取消和 panic recovery | goroutine 中不处理 panic |
| MCP 子进程 | context 取消时清理 | 子进程泄漏 |
| WebSocket 流 | 处理客户端断连 | 不检测客户端断连 |
| 共享状态 | `sync.Mutex`/`sync.RWMutex` | 全局变量 |

---

## 第六章：模块化设计

### 6.1 新增功能模块的标准结构

以新增 `workflow` 模块为例：

```
api/kratos/workflow/v1/workflow.proto     ← API 契约
internal/biz/workflow.go                  ← 模型 + Repo 接口 + Usecase
internal/biz/workflow_types.go            ← 领域类型定义（如需拆分）
internal/data/workflow_repo.go            ← Repo 实现
internal/data/ent/schema/workflow.go      ← Ent Schema
internal/service/workflow.go              ← Service 实现
internal/server/http.go                   ← 注册 RegisterWorkflowHTTPServer
cmd/admin/wire.go                         ← Wire 注入
```

### 6.2 模块间通信

| 方式 | ✅ 正确 | ❌ 错误 |
|------|---------|---------|
| 同步调用 | Usecase 之间通过接口调用 | 直接 import 另一模块的 data |
| 异步事件 | 通过 `Broker` 发布/订阅（如 `TeamRunEventBroker`） | 通过全局变量共享状态 |
| 状态共享 | Pinia Store / 数据库 | 包级变量 |

### 6.3 接口隔离

```go
// ✅ 正确：窄接口，按需定义
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

// ❌ 错误：一个巨大接口包含所有方法
type AgentRepository interface {
    GetAgentByID(...)
    CreateAgent(...)
    UpdateAgent(...)
    SearchAgents(...)
    DeleteAgent(...)
    GetAgentByName(...)    // 不需要的方法也塞进来
    BulkUpdate(...)        // 与核心 CRUD 无关
}
```

### 6.4 配置管理

1. **配置来源优先级**：环境变量 > 系统设置 > 配置文件 > 代码默认值
2. **配置结构**：在 `internal/conf/conf.proto` 中定义
3. **热更新**：通过 Kratos config source 支持，不自行实现 watch

---

## 第七章：前端编码规范

### 7.1 数据流方向（禁止逆行）

```
features/<域>/api.ts     ← 纯 HTTP 与类型
        ↓
Pinia Store              ← 状态 + actions
        ↓
Composable               ← 页面级薄 API
        ↓
Page                     ← 路由 + 布局
        ↓ props
Component                ← 展示：props in / emits out
```

### 7.2 展示组件禁令

| ❌ 禁止 | 说明 |
|---------|------|
| `useXxxStore` / `defineStore` | 状态在 Store |
| `features/*/api` / `services/` / `axios` | 请求在 Store |
| `watch` + fetch + ref 共享业务数据 | 进 Store |

### 7.3 API 客户端

1. 新增模块：在 `web/src/services/index.ts` 导出 `createXxxService`
2. 统一使用 `requestHandler`，不另建 axios 实例
3. 后端根地址统一用 `getBackendOrigin()`

### 7.4 前端迭代节奏

对每个业务域建议使用同一节奏（可拆分 PR）：
1. **API 层**：`services/index.ts` → `features/<域>/api.ts`
2. **状态层**：Store actions 触发请求，Composable 封装
3. **页面层**：Page 瘦路由 + 布局，展示组件 `defineProps` + `defineEmits`
4. **组件化**：重复 UI 抽到 `components/<域>/`
5. **不写裸路径**：所有 `/v1/...` 走 `createXxxService`

### 7.5 展示组件速记

**MUST**：新 HTTP 在 `features/<domain>/api.ts`；经 `services/index` 的 `createFooService` + `requestHandler` 访问 `/v1/...`；触发请求仅在 Pinia actions；展示组件 `defineProps` + `defineEmits`；新 store `stores/<domain>/` 具名导出。

**MUST NOT**（展示组件）：`useXxxStore`/`defineStore`/`storeToRefs`（非白名单容器）；`features/*/api` / `services` 入口 / `axios` / `kratosApi` / `create*Service()` 用于远程读写；`watch`+fetch+ref 承载跨组件业务数据。

---

## 第八章：UI/UX 执行规范

> 数值与 token 为实现权威，不要用「相近」色替代。

### 8.1 强制自检

| 检查项 | 要求 |
|--------|------|
| 玻璃材质 | 半透明 + `backdrop-filter` / `-webkit-backdrop-filter`，blur 一般 12–24px，移动端 8–12px |
| 边框 | 半透明；禁止纯黑或纯白硬边作玻璃边框 |
| 阴影 | 日间优先不靠重 `box-shadow`，用厚度与边框；夜间用微弱光晕 |
| 昼夜结构 | 间距·圆角·字体阶梯不变，只换语义色与材质参数 |
| 日间锚点 | 金盏花 `#E9A23B`（悬 `#D48C1A`）贯穿主按钮、链接、`:focus-visible`、表单聚焦边；禁用日间以青紫霓虹为默认强调 |
| 夜间霓虹 | `#00E5FF`、`#A855F7` 仅占交互焦点与小面积强调渐变，禁用铺满；日间不得将它们作默认强调 |

### 8.2 CSS 变量 Token

**实现路径**：`web/src/css/theme/_css-vars-light.sass`（`:root`）、`_css-vars-dark.sass`（`body.body--dark`）；聚合入口 `web/src/css/app-theme.sass`。页面与组件取值用 `var(--*)`，一般不硬编码 hex。

**日间核心 Token**：

| Token | 值 | 用途 |
|-------|-----|------|
| `--canvas-base` | `#FEFBF4` | 主画布 |
| `--glass-surface` | `rgba(255,253,245,0.65)` | 标准玻璃 |
| `--glass-blur-default` | `18px` | 与 surface 配对 |
| `--glass-border` | `rgba(235,220,200,0.7)` | 边框 |
| `--glass-elevated` | `rgba(255,255,255,0.72)` | 弹层 |
| `--color-accent` | `#E9A23B` | 主操作 |
| `--color-accent-hover` | `#D48C1A` | 主操作悬 |
| `--color-text-primary` | `#3A322C` | 正文 |
| `--color-text-secondary` | `#8B7A6B` | 辅文案 |

**夜间核心 Token**：

| Token | 值 | 用途 |
|-------|-----|------|
| `--canvas-base` | `#090D14` | 画布 |
| `--glass-surface` | `rgba(18,24,34,0.65)` | 玻璃 |
| `--glass-border` | `rgba(255,255,255,0.08)` | 边框 |
| `--color-accent` | `#00E5FF` | 霓虹主强调 |
| `--color-neon-cyan` | `#00E5FF` | 焦点/链接 |
| `--color-neon-violet` | `#A855F7` | 二级渐变 |
| `--color-text-primary` | `#EBEBF0` | — |

**最小玻璃片段**：

```css
background: var(--glass-surface);
backdrop-filter: blur(var(--glass-blur-default));
-webkit-backdrop-filter: blur(var(--glass-blur-default));
```

### 8.3 样式工程规则

| 层级 | 路径 | 职责 |
|------|------|------|
| 构建常量 | `web/src/css/quasar-variables.sass` | `$primary` 等；不随 Dark 重算 |
| Token | `app-theme.sass` → `theme/*` | CSS 变量 |
| 全局类 | `app-global.sass` | 字体、shell、页面 class |
| 入口链 | `style.sass` → `css/style.sass` | 构建 `css: ['style.sass']` |

1. 新 token → `_css-vars-*.sass` 或新 partial 并由 `app-theme` 聚合
2. 新页面/布局 class → `app-global`
3. 主强调、链接、焦点以 `--color-accent`；`$primary` 仅兼容默认 Quasar；禁止运行时改 `quasar-variables`
4. Token 增殖仅在 `theme/` 扩充；勿并行第二全局 CSS 入口

### 8.4 组件数值

**按钮**：昼主 `#E9A23B` 字白圆角 10px；昼次透明字 `#3A322C`；夜主 `rgba(0,229,255,0.15)` 霓虹边字 cyan。

**卡片**：昼玻璃 `rgba(255,253,245,0.65)`+blur18 无重阴影；夜 `rgba(18,24,34,0.65)`+blur+webkit。

**对话框**：`background: var(--glass-elevated)`；`backdrop-filter` + `-webkit-backdrop-filter` 用 `blur(var(--glass-blur-elevated))`；边 `var(--glass-border)`；圆角 20–24px；主 CTA 用 `var(--color-accent)`。

**输入**：昼实体 `#fff` 底边 `#D0C0A8` 聚焦 `#E9A23B`；夜深透+白边渐变聚焦青；圆角 12–16px。

**导航**：昼奶色半透明+blur；夜 `rgba(9,13,20,0.7)` blur 20。

### 8.5 布局与排版

间距刻度：`4,8,12,16,20,24,32,48,64` px。圆角：控件 5–8；卡片/面板 16–20；大模块 28–36；胶囊 56–980；圆 50%。层级不靠重阴影，靠不透明、blur、边亮与昼夜焦点策略。

展示字体：`SF Pro Display, Inter Tight, Helvetica Neue, sans-serif`。正文：`SF Pro Text, Inter, Helvetica Neue, sans-serif`。

### 8.6 Do / Don't

| ✅ Do | ❌ Don't |
|-------|----------|
| 全昼夜磨砂玻璃 | 昼大白硬块铺满 |
| 昼奶油 rgba255,253,245系 | 层级靠堆砌阴影 |
| 夜深透+弱光 | 同层混搭实体与玻璃 |
| 强调仅锚点 | 玻璃上大纯色块挡内容 |
| — | 移动端忽略 blur 降级 |

### 8.7 响应式

断点遵从项目全局。移动端 blur 8–12px，动效降级。

---

## 第九章：AI 编码自检清单

> AI 每次代码改动**必须**逐项确认。违反红线立即停手。

### 改动前（定位与合规）

- [ ] 确认改动属于哪个层（service/biz/data/agent/tools/team/provider）→ 参见[决策树](#决策树我的代码该放哪)
- [ ] 确认依赖方向是否合规（不违反向内依赖原则）→ 参见[红线](#红线违反即停)
- [ ] 确认是否需要新增 proto 定义 → 参见[任务速查卡](#任务速查卡)
- [ ] 确认是否涉及 `pkg/trpc-agent-go` 框架 API → 先查框架 API 再实现

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

### 改动后（构建与验证）

- [ ] `make api` 已执行（如改了 proto）
- [ ] Wire 已重新生成：`cd cmd/admin && wire`
- [ ] `go build ./cmd/admin` 通过
- [ ] 无红线违反 → 参见[红线](#红线违反即停)
- [ ] 新增能力两处生效（Chat + Team 共用装配路径）

### 全链路合并检查

- [ ] `api/**/*.proto` 覆盖本迭代全部 `/v1` 能力，`make api`，Go + TS 已提交
- [ ] `internal/biz` / data / service / server 合规；Wire + `go build ./cmd/admin`
- [ ] `web/src/services/index.ts` `createXXXService`
- [ ] `features/<域>/api` + Pinia，展示组件门禁，浮层 `emit` 链闭环
- [ ] UX：玻璃双前缀、变量 token、组件数值/Do-Don't；深浅色自检
- [ ] `LEGACY_*` / 用量 ingest：按需阅读并声明

若需求违反本文硬性分层（例如必须在叶子组件打点请求），须在 PR 写明例外、边界、偿还计划，否则评审可拒。

---

## 附录

### 附录 A：关键文件索引

| 文件 | 用途 |
|------|------|
| `docs/需求/` | 全部需求设计文档（权威源） |
| `docs/需求/23 tools.md` | Tools 需求文档（含 Stream/Memory/AgentTool/MCPBroker/商业级工具） |
| `docs/需求/23 tools.design.md` | Tools 设计文档（含 Stream 流式机制/Memory 记忆/AgentTool/MCPBroker/扩展设计） |
| `docs/guides/AI-DEVELOPMENT-SPECIFICATION.md` | 本文档：AI 开发规范 |
| `.cursor/rules/trpc-agent-framework-first.mdc` | 框架优先规则 |
| `internal/tools/toolset.go` | 工具注册中心（Registry + AssemblyConfig + Assemble） |
| `internal/tools/tool.go` | 项目级工具类型别名（Tool/CallableTool/StreamableTool/ToolSet） |
| `internal/tools/trpc/toolsets.go` | 向后兼容适配层（ToolsetConfig → BuildToolsets） |
| `internal/tools/doc.go` | 工具包文档（框架能力说明 + 注册表 + 自定义工具指南） |
| `internal/memory/trpc/sqlite_adapter.go` | Memory Service SQLite 适配器 |
| `internal/agent/trpc_build.go` | Agent 构建 + 工具集装配入口 |

### 附录 B：Wire 注入模板

```go
// biz/biz.go
var ProviderSet = wire.NewSet(
    NewXxxUsecase,
)

// data/data.go
var ProviderSet = wire.NewSet(
    NewData,
    NewXxxRepo,
)

// service/service.go
var ProviderSet = wire.NewSet(
    NewXxxService,
)

// server/server.go
var ProviderSet = wire.NewSet(NewGRPCServer, NewHTTPServer, NewWSServer)

// cmd/admin/wire.go
func wireApp(*conf.Server, *conf.Data, log.Logger) (wireOut, func(), error) {
    panic(wire.Build(
        server.ProviderSet,
        data.ProviderSet,
        biz.ProviderSet,
        service.ProviderSet,
        newApp,
    ))
}
```

### 附录 C：新增 Ent 实体模板

```go
// internal/data/ent/schema/xxx.go
type Xxx struct {
    ent.Schema
}

func (Xxx) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").DefaultFunc(func() string {
            return uuid.New().String()
        }),
        field.String("name").NotEmpty(),
        field.String("status").Default("active"),
        field.Time("created_at").Default(time.Now),
        field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
    }
}

func (Xxx) Edges() []ent.Edge {
    return nil
}

func (Xxx) Indexes() []ent.Index {
    return []ent.Index{
        index.Fields("name"),
    }
}
```

### 附录 D：平台目标架构演进方向

> **性质**：本节描述的是**目标架构**，与当前项目结构存在差距。新模块、PR 若与之冲突，必须在本节登记例外或修正本节。AI 编码时以当前结构为准，不得按本节重构现有代码。

#### D.1 设计原则

1. **限界上下文（Bounded Context）**：按业务概念切分，不按技术层切分。Context 内部允许丰富，Context 之间只能通过端口、命令、事件、查询协作
2. **端口与适配器（Hexagonal）**：每个 Context 对外只暴露端口（Port），所有实现细节是适配器（Adapter）——HTTP、CLI、SQLite、tRPC-Agent-Go、LLM SDK、向量库等都是"被适配"的
3. **接口清晰（四要素显式）**：模块边界（哪个 Context）、调用协议（命令/查询/事件签名）、数据格式（领域类型 + JSON schema）、依赖方向（谁依赖谁），四者必须写进 `ports/` 包并在文档中固化
4. **不变与变化分离**：协议、领域类型、事件信封、能力契约属于不变层；后端实现、运行时、SDK、UI 属于变化层
5. **单一职责**：一个包只解决一类技术问题；一个 Context 只解决一类业务问题
6. **共享内核最小化**：内核只放所有 Context 都依赖且长期稳定的内容（ID、时间、错误、事件信封、运行上下文、Module 接口），绝不放业务策略与领域逻辑
7. **依赖方向单向收敛**：`adapter → context → kernel`、`runtime adapter → context.port`，绝不反向
8. **可观测优先**：tracing、结构化事件、WebSocket、审计、用量从 Day 1 起即作为架构约束而非补丁

#### D.2 Context 划分

每个 Context 对内：domain · application · ports · 内部组件；对外：仅 ports（命令/查询/事件/能力契约）。

#### D.3 能力执行链

Capability 的运行时调用必须经 `application → executor → middleware → backends`，**禁止**跳过 executor。

#### D.4 跨 Context 协作规则

- 跨 Context 协作只走 `kernel/contracts/`
- `<context>/domain` 与 `<context>/application` 不允许 import 其它 Context
- Kernel 准入条件：≥3 Context 实现/消费 + 已稳定 ≥2 个 PR 周期

#### D.5 SQL 归属

表前缀必须等于 Context 名（`identity_*` / `catalog_*` / `capability_*` / `conversation_*` / `memory_*` / `operations_*`），SQL 仅在 `<context>/adapters/sqlite/**` 出现。

#### D.6 迁移原则

- 按映射表一行 = 一个 PR
- 违反红线立即停手
- 新代码落到目标 Context 下，禁止在旧路径新增文件
- 目录全扫描优先：分析任何框架/库时，第一步是 ls -la 获取完整文件列表，逐文件阅读，不跳过任何文件
- 建立能力清单矩阵：对框架的每个文件/模块，建立 [文件] → [能力] → [项目集成状态] 的三列矩阵
- 接口+实现双路径：不仅看接口定义（`tool.go`），还要看实现文件（`callbacks.go`、`filter.go`）和辅助文件（`context.go`、`final_result.go`）
- 框架 Option 全枚举：对 `llmagent.Option` 等配置入口做全量枚举，确保每个 `With*` 函数都有对应的项目集成路径
