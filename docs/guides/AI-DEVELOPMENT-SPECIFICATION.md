# Aranea-Agents AI 开发规范

> **文档地位**：AI 编码时的**唯一行为准则**。所有代码改动必须遵守本规范。本文已整合原 `architecture/runtime-boundary.md`、`AI-全栈新功能开发规范.md`、`接口与数据库开发规范.md`、`frontend/vue-design.md`、`frontend/UX.md` 的全部内容。
>
> **规范冲突优先级**：本文 > 所有其他 docs 下的规范文档

---

## 第一章：架构总纲

### 1.1 双框架分工

| 框架 | 职责边界 | 禁止 |
|------|----------|------|
| **Kratos v2** | 传输层（HTTP/gRPC/SSE）、配置、鉴权、中间件、Wire 依赖注入 | 不承载 Agent 编排、不实现第二套事件循环 |
| **trpc-agent-go** | Agent 编排（Runner/Agent/Session/Memory/Tool/Event） | 不直接写业务数据库、不处理 HTTP 路由 |

**各包职责映射**（框架能力按领域拆开，无单独 `adkadapter`）：

| 能力 | 主要包 |
|------|--------|
| `session.Service`（会话快照读写） | `internal/agent/adksvc`（`BizSessionService`） |
| `llmagent` 构建、`model.LLM` | `internal/agent`（`BuildLLMAgent`）、`internal/provider`（`ModelForProviderModel`） |
| 工具有效列表 → `tool.Tool` | `internal/tools`（`ToolsForAgent`） |
| Runner 内存 / 插件 / 用户 ID 上下文 | `internal/agent`（`NewADKMemoryService`、`DefaultRunnerPlugins`、`UserIDFromCtx`） |
| Team 工作流根 Agent | `internal/team`（`BuildWorkflowRoot` + `runner.Run`） |

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

### 1.3 八条红线

| # | 红线 | 违反后果 |
|---|------|----------|
| 1 | `internal/server/*` 不得 new `runner.Runner` 或 `llmagent.New` | 停 |
| 2 | `internal/biz/*` 不得 import `pkg/trpc-agent-go` 任何包 | 停 |
| 3 | 框架 `plugin` 回调不得直接写数据库 | 停 |
| 4 | 不得绕过 `internal/agent/adksvc` 把 Ent 行塞进 `session.Event` | 停 |
| 5 | 不得在 transport 层解析工具参数或拼接 prompt | 停 |
| 6 | 不得为框架运行时另起独立 HTTP 监听 | 停 |
| 7 | 不得把 Kratos middleware 逻辑复制进 `pkg/trpc-agent-go` | 停 |
| 8 | 不得在 `internal/biz` 直接依赖框架运行时 toolset/skill 类型 | 停 |

### 1.4 逐包 import 规则

| 包路径 | 允许 import | 禁止 import |
|--------|-------------|-------------|
| `internal/server/*` | `internal/service`、`internal/conf`、kratos、`pkg/auth`、`pkg/validate` | `pkg/trpc-agent-go` / 框架运行时私有 import |
| `internal/biz/*` | stdlib、kratos errors、本仓 biz/data API | `pkg/trpc-agent-go` / 框架运行时私有 import |
| `internal/service/*` | `internal/biz`、`internal/team`、`internal/agent`、`internal/agent/adksvc`、`internal/provider`、`internal/tools`，以及框架 Runner/Agent 装配 API | 绕过 `internal/tools` 大量直连拼装底层 `tool` |
| `internal/agent/*`（含 `adksvc`） | `internal/biz`、`internal/provider`、`internal/data/...`（如需）、`pkg/trpc-agent-go` / 框架运行时 | — |
| `internal/team/*` | `internal/biz`、`internal/agent`、`internal/provider`、`internal/tools`、`pkg/trpc-agent-go` / 框架运行时 | — |
| `internal/provider/*` | `internal/biz`、`pkg/trpc-agent-go` / 框架 `model` 适配 | — |
| `internal/tools/*` | `internal/biz`、框架 `tool` API（由 `pkg/trpc-agent-go` 暴露或兼容层 re-export） | — |

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

1. **嵌入 `Unimplemented*Server`**：每个 Service 必须嵌入对应 proto 生成的未实现结构体
2. **构造函数 `NewXxxService`**：只接收 biz Usecase，不做其他初始化
3. **类型转换函数命名**：`toProtoXxx`（biz → proto）、`fromProtoXxx`（proto → biz）
4. **错误映射**：biz 返回的 `error` 用 `kerrors.FromError(err)` 或手动构造 `kerrors.BadRequest/InternalServer`
5. **禁止在 Service 中写业务逻辑**：Service 只做映射和编排，业务逻辑全部在 biz

**Runner 装配规则**：

```go
// 正确：Service 是框架调用的唯一桥点
func (s *ChatService) SendChatMessage(ctx context.Context, req *chatv1.SendChatMessageRequest) (*chatv1.SendChatMessageResponse, error) {
    // 1. proto → biz 参数
    // 2. 调 biz Usecase 获取 Agent/Session
    // 3. 调 internal/agent 构建 Agent
    // 4. 调 internal/agent 构建 Runner
    // 5. runner.Run → 事件流 → 投影为 proto 响应
}
```

**桥接约定**：`internal/service` 内 Kratos service 在方法中构造框架 `Runner`，将 RPC/HTTP 请求译为会话执行入口，将会话事件流投影为 unary 或 SSE。**不在 `internal/server` 或 `internal/biz` 中直接使用框架运行时。**

### 2.2 Biz 层——领域核心

**职责**：定义领域模型、Usecase 编排、Repo 接口。

**编码规则**：

1. **模型定义**：纯 Go struct，字段用基本类型，不用 proto 类型
```go
type Agent struct {
    ID          string
    AgentKey    string
    DisplayName string
    Provider    string
    Model       string
    Status      string
    Settings    *AgentRuntimeSettings
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

4. **错误处理**：使用 `kerrors.BadRequest`/`InternalServer` 等，不使用 `fmt.Errorf`
5. **分页**：统一使用 `biz.ListOption` + `pagination.go` 的 `ListOffset/ListLimit/ListFilter/ListOrderBy`
6. **禁止 import**：`api/*/v1`、`pkg/trpc-agent-go` 任何包

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
   - SQLite：仅通过 `d.Ent()` 访问（`*ent.Client`）
   - pgvector：仅通过 `d.Postgres()` 访问（`*sql.DB`）
   - **禁止**：在 `NewData` 外另开 SQLite `sql.Open`

3. **Ent 转换函数**：`entAgentToBiz` / `bizAgentToEnt`，放在对应 Repo 文件中
4. **新增实体流程**：
   - `internal/data/ent/schema/XXX.go` 定义 Fields/Index/Edge
   - `go generate ./internal/data/ent`
   - `internal/data/xxx.go` 实现 Repo
   - `internal/biz/xxx.go` 定义模型 + Repo 接口 + Usecase

### 2.4 Server 层——传输注册

**职责**：创建 HTTP/gRPC/SSE 实例，注册 Service。

**编码规则**：

1. **HTTP 注册**：只做 `v1.RegisterXxxHTTPServer(srv, service实例)`
2. **gRPC 注册**：只做 `v1.RegisterXxxServiceServer(srv, service实例)`
3. **禁止**：在 Server 层写业务路由、手写 `HandleFunc`
4. **中间件**：统一在 `NewHTTPServer`/`NewGRPCServer` 中注册

---

## 第三章：Agent 运行时编码规范

### 3.1 框架真相源

**`pkg/trpc-agent-go` 是 Agent 框架的唯一真相源。**

- 编排语义（Runner、Agent 树、Tool、Session、Event）必须落在 `pkg/trpc-agent-go`
- **先查框架 API 再实现**，不在 biz 重写运行时
- 不把框架内部实现整块复制到业务目录

### 3.2 运行时装配层次

```
internal/service        ← Runner 装配入口（调 agent/team/tools）
internal/agent          ← Agent 构建（BuildLLMAgent、Memory、Plugins）
internal/team           ← Team 工作流（BuildWorkflowRoot、Runner）
internal/tools          ← 工具装配（TurnMount、Skill、MCP）
internal/provider       ← LLM 模型驱动（ModelForProviderModel）
```

### 3.3 Agent 构建规范

**BuildLLMAgent 调用链**：

```go
// 1. Service 层组装 BuilderDeps
deps := agent.BuilderDeps{
    Catalog:    s.llmCatalog,
    AgentUC:    s.agentsUC,
    Agents:     s.agents,
    ToolsCatalog: s.toolsCatalog,
    RT:         s.adk,
    Memory:     agent.RunnerMemoryForRuntime(s.adk),
    Provider:   provider,
    Model:      model,
}

// 2. 构建 Agent
root, err := agent.BuildLLMAgent(ctx, ag, deps)

// 3. 构建 Runner
runner, err := agent.NewADKRunnerForRuntime(root, sessSvc, s.adk)

// 4. 执行
eventCh, err := runner.Run(ctx, userID, sessionID, userMessage)
```

**规则**：

1. **BuilderDeps 是 Service 与框架之间的 DTO**：不含框架运行时类型，只含 biz 模型 + 可选依赖标记
2. **Memory 注入**：通过 `adkdeps.Runtime.SessionMemory` 由 Wire 注入，不在 Service 手动选择
3. **工具装配**：通过 `TurnMount.Attach` 统一挂载，不分散在多处

### 3.4 TurnMount 工具装配规范

**唯一装配入口**：`internal/tools/turn_mount.go` 的 `TurnMount.Attach`

```go
func (m TurnMount) Attach(ctx context.Context, ag biz.Agent, userQuery string,
    tools *[]tool.Tool, toolsets *[]tool.Toolset) error
```

**装配顺序**：
1. Builtin Tools（`ADKToolsForAgentPolicy`）
2. Skill Toolsets（`skillruntime.AppendEnabledPublishedSkillToolsets`）
3. MCP Toolsets（`mcpmount.AppendEffectiveMCPServerToolsets`）

**规则**：

1. 新增工具类型必须通过 `TurnMount.Attach` 挂载，不另开装配路径
2. Chat 和 Team 共用同一 `TurnMount` 逻辑，避免分叉
3. 工具策略（allow/deny）在 biz 层解析，tools 层只做框架映射

### 3.5 Team 编排规范

**两种模式**：

| 模式 | 实现方式 | 适用场景 |
|------|----------|----------|
| Coordinator | 协调者 Agent 调度成员作为工具 | 需要中央决策 |
| Swarm | 成员间 `transfer_to_agent` 传递控制权 | 自由协作 |

**编码规则**：

1. **Team Runner 在 `internal/team`**：不溢出到 service 或 biz
2. **成员 Agent 独立构建**：每个成员用自己的 Settings、Skill 策略、MCP 服务器列表
3. **事件流通过 `biz.TeamRunEventBroker`** 发布 SSE

### 3.6 记忆系统规范

**五层记忆架构**：

| 层级 | 存储 | 运行时接入 |
|------|------|-----------|
| L0 感官 | SQLite (sessionmemory) | Runner MemoryService |
| L1 工作 | SQLite (sessionmemory) | Runner MemoryService |
| L2 情景 | SQLite (sessionmemory) | Runner MemoryService |
| L3 语义 | pgvector | biz.MemoryUsecase（独立业务线） |
| L4 持久 | SQLite (sessionmemory) | Runner MemoryService |

**规则**：

1. **Runner MemoryService 由 Wire 注入**：有 `sessionmemory.Store` → SQLite 适配器；无 → in-memory
2. **L3 pgvector 是独立业务线**：不自动挂载到 Runner，需显式接入
3. **`load_memory`/`preload_memory`**：行为必须与实际后端一致，后端未就绪时不在 prompt 中宣称
4. **记忆写入**：经 broker/async 异步写，不在 plugin 回调中直接写库

### 3.7 Provider 集成约定

- **`internal/provider`** 承载厂商连接与模型的初始化、解析与调用——目录/Biz 侧 `provider_type` / `api_base_url` / `api_key` / 模型名的合并、`Registry.Resolve` 绑定具体后端、HTTP 传输、以及实现 `pkg/trpc-agent-go` 所定义之 `model.LLM` 的 `GenerateContent`（含流式）
- **契约对齐**：对模型的入参/出参形态以 `pkg/trpc-agent-go/model` 为准；不要在业务包中平行维护另一套「驱动接口」或重复的厂商 HTTP 客户端
- **业务集成**：凡与调用大模型相关的业务能力（选厂商、走补全/流式、聚合用量与文本解析等），优先在 `internal/provider` 及其子包内收口实现；`internal/agent`、`internal/team`、`internal/service` 等仅保留编排、proto/会话消息与 `LLMRequest` 之间的必要适配
- **新增厂商**：通过扩展 `Registry` 注册工厂、在子包中实现 `model.LLM`，并保持与现有 `CatalogClient`、`MergeCatalogIntoRequest` 等辅助方法一致

---

## 第四章：Proto 与 API 规范

### 4.1 Proto 定义规则

1. **路径**：`api/kratos/<module>/v1/<module>.proto`
2. **HTTP 注解**：每个 RPC 必须配 `google.api.http`
3. **必填标记**：使用 `(google.api.field_behavior) = REQUIRED`
4. **命名**：proto 字段 `snake_case`，Go 生成 `CamelCase`
5. **禁止**：一半在 proto、一半手写路由的分裂契约

### 4.2 代码生成流程

```bash
make init    # 首次安装插件
make api     # 生成 Go + TypeScript
make config  # 仅改 conf.proto 时
```

**必须提交生成物**：`*.pb.go`、`*_http.pb.go`、`*_grpc.pb.go`、`web/src/services/`

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
- **`LEGACY_REST_ORIGIN`**：上游根 URL（无尾部 `/`）。`chat_legacy_forward` 将 `/v1/chat/messages` 反向代理到 `{origin}` + 遗留路径推导值；`internal/cronrunner` 到期任务 POST `{origin}` + 遗留 Messages 路径。未设置：`/v1/chat/*` 可能 503
- **`CRON_RUNNER_INTERVAL`**：Cron tick，`time.ParseDuration`；空或非法默认 `1m`
- **`CRON_RUNNER_DISABLED`**：设为 `1` 则不启动 `internal/cronrunner`

### 4.6 用量上报与双写

**HTTP**：`POST /v1/usage/token-events`，请求体为完整 `TokenUsageEvent`。`ctx.Bind` 使用 `encoding/json` 标签，字段名为 snake_case。

**前端注意**：`protoc-gen-typescript-http` 生成的默认体可能对嵌套消息用 `JSON.stringify` 产出 camelCase 键，与 Go `json` 标签不一致，导致静默丢字段或校验失败。此类接口应在 `features/<域>/api.ts` 中用 `kratosApi.post` 等显式构造 snake_case。

**单一写入方（避免重复计数）**：

| 场景 | 说明 |
|------|------|
| 常见风险 | 对话完成已由后端写入 `model_token_usage_events` 时，若在浏览器 `onDone` / SSE 结束再 POST，会对同一轮交互重复插入 |
| 目标态 | 仅服务端在同一请求路径写入用量时，浏览器不应再报同一事件 |
| 例外 | 仅当服务端确认从不写入且不重叠会话/id 时，才可单独浏览器上报；须在 PR 写明 |
| 过渡 | 若须二选一并行，应有 feature flag，默认只开一侧。禁止在未知后端是否已写时默认开启浏览器 ingest |

---

## 第五章：Go 代码风格规范

### 5.1 命名规范

| 场景 | 规范 | 示例 |
|------|------|------|
| 包名 | 小写单词，不用下划线 | `agent`, `mcpmount`, `skillruntime` |
| 文件名 | 小写+下划线，按职责拆分 | `agent_repo.go`, `adk_build.go` |
| 结构体 | 大驼峰，名词 | `AgentUsecase`, `TurnMount` |
| 接口 | 大驼峰，名词+后缀 | `AgentRepository`, `MemoryService` |
| 函数 | 大驼峰导出/小驼峰内部 | `NewAgentUsecase`, `fromProtoRuntime` |
| 常量 | 大驼峰导出/小驼峰内部 | `DefaultAppName`, `streamQueryKey` |
| 错误变量 | `Err` 前缀 | `ErrNotFound`, `ErrAppNameRequired` |

### 5.2 函数设计

1. **单一职责**：每个函数只做一件事
2. **参数不超过 5 个**：超过则封装为 Option struct
3. **返回值**：业务函数返回 `(result, error)`，不用 panic
4. **构造函数**：统一 `NewXxx` 命名，返回指针
5. **类型转换**：独立函数 `toProtoXxx`/`fromProtoXxx`，不在方法内内联

### 5.3 错误处理

```go
// 正确：使用 kratos errors
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
```

**规则**：
1. 不用 `fmt.Errorf` 返回业务错误
2. 不用 `panic` 处理业务逻辑
3. `sql.ErrNoRows` → `kerrors.NotFound`
4. 参数校验失败 → `kerrors.BadRequest`
5. 内部错误 → `kerrors.InternalServer`

### 5.4 依赖注入

1. **Wire ProviderSet**：每层一个，在 `biz.go`/`data.go`/`service.go`/`server.go` 中定义
2. **构造函数参数**：只接收接口或具体依赖，不接收 `*Data` 之外的"上帝对象"
3. **禁止**：手动 `wire_gen.go`，必须通过 `wire` 命令生成

### 5.5 并发与资源

1. **context 传递**：所有跨层调用必须传递 `ctx`
2. **goroutine**：必须处理 context 取消和 panic recovery
3. **MCP 子进程**：必须在 context 取消时清理
4. **SSE 流**：必须处理客户端断连
5. **共享状态**：使用 `sync.Mutex`/`sync.RWMutex`，不用全局变量

---

## 第六章：模块化设计规范

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

1. **同步调用**：Usecase 之间通过接口调用，不直接 import 另一模块的 data
2. **异步事件**：通过 `Broker` 发布/订阅（如 `TeamRunEventBroker`、`MonitorLogBroker`）
3. **禁止**：模块间通过全局变量或包级变量共享状态

### 6.3 接口隔离

```go
// 正确：窄接口，按需定义
type AgentReader interface {
    GetAgentByID(ctx context.Context, id string) (Agent, error)
}

type AgentWriter interface {
    CreateAgent(ctx context.Context, a Agent) (Agent, error)
    UpdateAgent(ctx context.Context, a Agent) (Agent, error)
}

// 完整接口组合
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

| 禁止 | 说明 |
|------|------|
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

## 第八章：UI · UX 执行规范

> 本章整合自 `AI-全栈新功能开发规范.md` 第四部分。数值与 token 为实现权威，不要用「相近」色替代。

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

**Do**：全昼夜磨砂玻璃；昼奶油 rgba255,253,245系；夜深透+弱光；强调仅锚点。

**Don't**：昼大白硬块铺满；层级靠堆砌阴影；同层混搭实体与玻璃；玻璃上大纯色块挡内容；移动端忽略 blur 降级。

### 8.7 响应式

断点遵从项目全局。移动端 blur 8–12px，动效降级。

---

## 第九章：AI 编码自检清单

每次代码改动前，AI 必须逐项确认：

### 改动前

- [ ] 确认改动属于哪个层（service/biz/data/agent/tools/team/provider）
- [ ] 确认依赖方向是否合规（不违反向内依赖原则）
- [ ] 确认是否需要新增 proto 定义
- [ ] 确认是否涉及 `pkg/trpc-agent-go` 框架 API

### 改动中

- [ ] Service 层：只做映射和编排，无业务逻辑
- [ ] Biz 层：无 `pkg/trpc-agent-go` import，无 proto import
- [ ] Data 层：仅 `Ent()`/`Postgres()` 访问，无并联 SQLite 连接
- [ ] Agent/Tools/Team 层：框架 API 调用合规，不复制框架内部逻辑
- [ ] 错误处理：使用 `kerrors`，不用 `fmt.Errorf`
- [ ] 命名：符合 5.1 规范

### 改动后

- [ ] `make api` 已执行（如改了 proto）
- [ ] Wire 已重新生成
- [ ] `go build ./cmd/admin` 通过
- [ ] 无红线违反
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

## 附录 A：关键文件索引

| 文件 | 用途 |
|------|------|
| `docs/需求/` | 全部需求设计文档（权威源） |
| `docs/guides/AI-DEVELOPMENT-SPECIFICATION.md` | 本文档：AI 开发规范 |
| `.cursor/rules/trpc-agent-framework-first.mdc` | 框架优先规则 |

## 附录 B：Wire 注入模板

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
var ProviderSet = wire.NewSet(NewGRPCServer, NewHTTPServer, NewSSEServer)

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

## 附录 C：新增 Ent 实体模板

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

## 第十章：平台目标架构原则

> 本章整合自 `architecture/platform-architecture.md` 第三篇，描述 Aranea 的目标态架构设计原则。所有新模块、PR、专题文档若与之冲突，必须在本章登记例外或修正本章。

### 9.1 设计原则（判定基准）

1. **限界上下文（Bounded Context）**：按业务概念切分，不按技术层切分。Context 内部允许丰富，Context 之间只能通过端口、命令、事件、查询协作
2. **端口与适配器（Hexagonal）**：每个 Context 对外只暴露端口（Port），所有实现细节是适配器（Adapter）——HTTP、CLI、SQLite、tRPC-Agent-Go、LLM SDK、向量库等都是"被适配"的
3. **接口清晰（四要素显式）**：模块边界（哪个 Context）、调用协议（命令/查询/事件签名）、数据格式（领域类型 + JSON schema）、依赖方向（谁依赖谁），四者必须写进 `ports/` 包并在文档中固化
4. **不变与变化分离**：协议、领域类型、事件信封、能力契约属于不变层；后端实现、运行时、SDK、UI 属于变化层
5. **单一职责**：一个包只解决一类技术问题；一个 Context 只解决一类业务问题
6. **共享内核最小化**：内核只放所有 Context 都依赖且长期稳定的内容（ID、时间、错误、事件信封、运行上下文、Module 接口），绝不放业务策略与领域逻辑
7. **依赖方向单向收敛**：`adapter → context → kernel`、`runtime adapter → context.port`，绝不反向
8. **可观测优先**：tracing、结构化事件、SSE、审计、用量从 Day 1 起即作为架构约束而非补丁
9. **可裁剪部署**：通过 launcher 装配不同 Context 子集，业务代码不感知 launcher

### 9.2 六大限界上下文

| Context | 核心领域 | 包含 |
|---------|---------|------|
| **Identity** | 用户与权限 | user、team、workspace、role |
| **Catalog** | Agent 目录 | agent、evolution、prompt |
| **Capability** | 能力管理 | tool、skill、mcp/plugin、hook |
| **Conversation** | 会话与编排 | session、message、channel、team-run |
| **Memory** | 记忆系统 | L0~L4、recall、decay |
| **Operations** | 运维与调度 | cron、monitor、audit、budget |

每个 Context 对内：domain · application · ports · 内部组件；对外：仅 ports（命令/查询/事件/能力契约）。

### 9.3 能力执行链

Capability 的运行时调用必须经 `application → executor → middleware → backends`，**禁止**跳过 executor。

### 9.4 跨 Context 协作规则

- 跨 Context 协作只走 `kernel/contracts/`
- `<context>/domain` 与 `<context>/application` 不允许 import 其它 Context
- Kernel 准入条件：≥3 Context 实现/消费 + 已稳定 ≥2 个 PR 周期

### 9.5 SQL 归属

表前缀必须等于 Context 名（`identity_*` / `catalog_*` / `capability_*` / `conversation_*` / `memory_*` / `operations_*`），SQL 仅在 `<context>/adapters/sqlite/**` 出现。

### 9.6 迁移原则

- 按映射表一行 = 一个 PR
- 违反红线立即停手
- 新代码落到目标 Context 下，禁止在旧路径新增文件
