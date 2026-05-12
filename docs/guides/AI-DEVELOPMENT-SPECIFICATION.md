# Aranea-Agents AI 开发规范

> **文档地位**：AI 编码时的唯一行为准则。所有代码改动必须遵守本规范。
>
> **规范冲突优先级**：本文 > `AGENT_RUNTIME_BOUNDARY.md` > `AI-全栈新功能开发规范.md` > `接口与数据库开发规范.md`

---

## 第一章：架构总纲

### 1.1 双框架分工

| 框架 | 职责边界 | 禁止 |
|------|----------|------|
| **Kratos v2** | 传输层（HTTP/gRPC/SSE）、配置、鉴权、中间件、Wire 依赖注入 | 不承载 Agent 编排、不实现第二套事件循环 |
| **trpc-agent-go** | Agent 编排（Runner/Agent/Session/Memory/Tool/Event） | 不直接写业务数据库、不处理 HTTP 路由 |

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

---

## 第八章：AI 编码自检清单

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

---

## 附录 A：关键文件索引

| 文件 | 用途 |
|------|------|
| `docs/AGENT_RUNTIME_BOUNDARY.md` | Kratos 与 tRPC-Agent-Go 运行时边界 |
| `docs/AGENT_SKILLS_TOOLS_MCP_MEMORY.md` | Skill/Tools/MCP/记忆运行时详解 |
| `docs/guides/AI-全栈新功能开发规范.md` | 全栈开发规范（含前端） |
| `docs/guides/接口与数据库开发规范.md` | 接口与数据库规范 |
| `docs/design/platform-architecture.md` | 平台架构设计 |
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
