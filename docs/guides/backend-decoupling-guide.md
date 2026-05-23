# 后端模块解耦优化方向

> 本文档基于 2026-05-23 对 `internal/` 全量代码的深度审查（构造函数签名、import 图、Wire 装配点、runtime-boundary 脚本），识别当前耦合热点，给出分阶段优化路线。AI 在做后端架构改动前应先阅读本文。

---

## 1. 当前耦合全景

### 1.1 依赖方向总览

```
api/proto
  ↓
service ←── 桥点：proto ↔ biz + 运行时装配
  ↓
biz ←── 领域模型 + Usecase + Repo 接口
  ↓
data ←── Repo 实现（Ent + SQLite）
```

**合规方向**：service → biz → data。**禁止** data → service、biz → service。

### 1.2 横向依赖现状（internal 子包间，基于实际 import 扫描）

| 包 | 依赖的兄弟包 | 耦合程度 |
|---|---|---|
| `agent` | biz, event (23 文件), provider, knowledge, mcp/config, skill/storage, tools/*, plugin/trpc, session/trpc, a2a/trpc, telemetry, metrics | **极高** |
| `team` | biz, agent (6 文件), runtime (3 文件), event, graph/adapter, graph/trpc, knowledge, plugin/trpc, tools/*, metrics, telemetry | **高** |
| `runtime` | biz, agent (2 文件), event, provider, data/sessionmemory, memory/trpc, graph/trpc, session/trpc | **高** |
| `service` | biz, agent, team, event, channel/*, runtime, knowledge, compress, a2a, graph/*, plugin/trpc, tools/*, metrics, telemetry | **极高** |
| `channel/runtime` | biz, event, channel/port, metrics | 中等 |
| `graph/adapter` | biz, agent, event, graph/trpc, provider, tools/* | 高 |
| `event` | metrics | **低** |
| `data` | biz, ent/*, conf, pgvector, sessionmemory | 低（合规） |

### 1.3 关键发现：agent / team / runtime 不是循环依赖

**实际依赖方向**（经代码验证）：

```
team → agent      (6 个文件：runner_finish_steps, runner_team_trpc, team_graph_run_finisher,
                    team_graph_run_coordinator, runner_helpers, trpc_build, usage_tokens)

team → runtime    (3 个文件：runner_team_trpc, runner, runner_helpers)

runtime → agent   (2 个文件：runner_manager, run_status)

agent → team      ❌ 无依赖
agent → runtime   ❌ 无依赖
runtime → team    ❌ 无依赖
```

这是**单向依赖链**（`team → agent + runtime`，`runtime → agent`），不是循环依赖。但 team 同时依赖 agent 和 runtime 两个包，仍需关注接口稳定性。

---

## 2. 核心耦合问题（按严重程度排序）

### P1：ChatService 是 God Service——27 个 Wire 参数

`provideChatServiceDeps` 在 [wire.go](file:///f:/aranea-agents/cmd/admin/wire.go) 中接收 **27 个参数**来组装 `ChatServiceDeps`：

```go
func provideChatServiceDeps(
    runs *rt.RunRegistry,                          // 1
    teams biz.TeamRepository,                      // 2
    teamsNative *team.Runner,                      // 3
    usage *biz.UsageUsecase,                       // 4
    sessions *biz.SessionUsecase,                  // 5
    agents biz.AgentRepository,                    // 6
    agentsUC *biz.AgentUsecase,                    // 7
    toolsCatalog biz.ToolRepo,                     // 8
    toolUC *biz.ToolUsecase,                       // 9
    llmCatalog *biz.LlmProviderModelUsecase,       // 10
    skillUC *biz.SkillUsecase,                     // 11
    sys biz.SystemSettingRepo,                     // 12
    persist rt.PersistenceSet,                     // 13
    compress biz.NativeTurnCompressor,             // 14
    eventBus event.Bus,                            // 15
    eventBuffer *event.Buffer,                     // 16
    pluginRT *plugintrpc.Runtime,                  // 17
    pluginMgr *plugintrpc.Manager,                 // 18
    skillDBRepo trpcskill.Repository,              // 19
    a2aUC *biz.A2AUsecase,                         // 20
    artifacts *biz.ArtifactUsecase,                // 21
    mcpUC *biz.MCPServerUsecase,                   // 22
    knowledgeRetriever *knowledge.Retriever,       // 23
    mon *biz.MonitorUsecase,                       // 24
    codeExecFactory *localexec.Factory,            // 25
    graphFactory biz.GraphBuilderFactory,           // 26
    graphs *biz.GraphUsecase,                      // 27
    tasks *biz.TaskUsecase,                        // 28
    teamGraphCoord *team.TeamGraphRunCoordinator,  // 29
    turnJobs *biz.ChannelTurnJobUsecase,           // 30
) service.ChatServiceDeps
```

`ChatService` 结构体持有 16 个字段，`ChatServiceDeps` 持有 21 个字段（含嵌入的 `rt.TurnDeps`）。

**问题本质**：ChatService 同时承担了 Chat CRUD、Turn 生命周期、Team 编排、A2A 调用、Knowledge 检索、Plugin 管理、Graph 编排、MCP 工具装配——远超"传输桥点"的职责。

### P2：biz 包过大——单包承载所有领域

`internal/biz` 承载了所有领域模型、Repo 接口和 Usecase。文件数超过 100 个，所有 Usecase 共享同一个 `package biz` 命名空间。

**实际影响**：
- **隐式耦合**：同包 Usecase 可直接访问彼此的内部细节，无需显式依赖注入
- **编译爆炸**：改一个 Usecase 触发整个 biz 包重编译
- **心智负担**：无法快速定位"这个模块包含什么"

**但好消息是**：biz 内 Usecase 间的**显式交叉依赖较少**。审查所有 `NewXxxUsecase` 构造函数发现：

| Usecase | 依赖 | 类型 |
|---|---|---|
| `ChannelTurnJobUsecase` | `*ChannelUsecase` | 具体结构体（应改为接口） |
| `AgentMCPTooling` | `*AgentUsecase` + `*MCPServerUsecase` | 具体结构体（应改为接口） |
| `PluginUsecase` | `AgentRepository` | Repo 接口（合规） |
| `SessionUsecase` | `SessionRepository` + `AgentRepository` + `TeamRepository` | Repo 接口（合规，跨聚合只读） |
| 其余 ~20 个 Usecase | 仅 Repo 接口 | 合规 |

大多数 Usecase 只依赖 Repo 接口，这是好的。问题集中在少数几个交叉引用。

### P3：biz 层反向依赖 internal 子包（12 个文件）

规范红线 2 要求 `biz` 不得 import `pkg/trpc-agent-go`，但 biz 内还存在对 `internal` 子包的直接依赖：

| biz 文件 | 依赖 | 性质 |
|---|---|---|
| `memory_worker.go` | `internal/memory/trpc` | 运行时适配层 |
| `memory_admin_store.go` | `internal/data/sessionmemory` | data 层实现 |
| `web_research_runtime.go` | `internal/tools/webresearch` | 工具实现 |
| `tool_catalog_runtime.go` | `internal/tools/webresearch` | 工具实现 |
| `tool_test_invoke.go` | `internal/tools/testexec` | 工具实现 |
| `mcp_server.go` | `internal/mcp/metadata` + `internal/mcp/probe` | MCP 内部包 |
| `mcp_user_credential.go` | `internal/mcp/config` | MCP 配置 |
| `llm_provider_model.go` | `internal/llminspect` | 检查实现 |
| `webhook_dispatcher.go` | `internal/event` | 事件总线 |
| `domain_event_adapter.go` | `internal/event` | 事件总线 |
| `orchestration_status.go` | `internal/event` | 事件类型 |
| `event_bus_*.go` (8 files) | `internal/event` | 事件消费/发布 |
| `memory_worker.go` | `internal/event` | 事件消费 |

**最严重的是 event 包**：biz 内 **12+ 文件** 直接 import `internal/event`。虽然 `event.Bus` 是接口，但 biz 还使用了 `event.Envelope`、`event.EventType` 等具体类型。

### P4：wire.go 成为耦合汇聚点

[wire.go](file:///f:/aranea-agents/cmd/admin/wire.go) 中的 `provide*` 函数承担了大量跨层组装逻辑：

- `provideChatServiceDeps`：30 个参数
- `provideAutoMemoryWorker`：直接引用 `data.NewL4GraphWriterAdapter`、`memtrpc.NewSQLiteMemoryService`
- `providePluginRuntime`：手动组装回调闭包
- `provideCronRunnerDeps`：引用 `service.ChatService`

wire.go 本应是纯组装，但实际上包含了**业务编排逻辑**（如 `providePluginRuntime` 中的 `SetCatalogConfirmChecker` 闭包）。

### P5：Channel 平台硬编码注册

`channel_platform_registry.go` 硬编码 import 了 9 个平台包。新增平台必须修改此文件。

---

## 3. 优化路线

### Phase 1：ChatService 拆分（优先级最高，收益最大）

**目标**：将 ChatService 的编排职责下放，service 层回归"传输桥点"。

**当前**：
```
ChatService = Chat CRUD + Turn 执行 + Team 编排 + A2A + Knowledge + Plugin + Graph + MCP
```

**目标**：
```
ChatService (service 层)         ← 只做 proto 映射 + HTTP/WS 传输
ChatOrchestrator (biz/chat)      ← Turn 生命周期编排
TurnExecutor (agent 包)          ← 单次 Turn 执行（已有，但 ChatService 绕过了）
TeamRunCoordinator (team 包)     ← Team 运行协调（已有，ChatService 不应直接持有）
```

**具体拆分步骤**：

1. **提取 ChatOrchestrator**：将 `SendChatMessage`、`CancelChat`、`AwaitReply` 的核心逻辑移到 `biz/chat_orchestrator.go`，ChatService 只做参数转换 + 调用

2. **TurnDeps 内聚**：`rt.TurnDeps` 已包含 Catalog + Pipeline + LLMHTTP 等，ChatService 不应再额外持有 `pluginRT`、`pluginManager`、`knowledgeRetriever`、`codeExecFactory`——这些应全部收入 `TurnDeps` 或其扩展

3. **消除 ChatService 对 team.Runner 的直接持有**：ChatService 通过 `ChatOrchestrator` 接口调用，不直接持有 `*team.Runner`

4. **Wire 参数从 30 降到 ≤ 10**：ChatOrchestrator 自己组装内部依赖

| 职责 | 当前位置 | 迁移到 |
|---|---|---|
| Chat CRUD (ListMessages, GetOptions) | ChatService | ChatService (保留) |
| Turn 生命周期 (Send, Cancel, AwaitReply) | ChatService | `biz.ChatOrchestrator` |
| Team 编排 | ChatService.teamsNative | `ChatOrchestrator` 内部引用 |
| A2A 调用 | ChatService.a2aUC | `ChatOrchestrator` 内部引用 |
| Knowledge 检索 | ChatService.knowledgeRetriever | `agent.TurnDeps` 内部 |
| Plugin 管理 | ChatService.pluginRT/pluginManager | `agent.TurnDeps` 内部 |
| Graph 编排 | ChatService.graphFactory/graphs | `ChatOrchestrator` 内部引用 |

### Phase 2：biz 包拆分

**目标**：将 `internal/biz` 按聚合根拆为子包，每个子包内聚自己的模型 + Repo 接口 + Usecase。

```
internal/biz/
├── agent/          ← AgentUsecase + AgentRepository + Agent 模型
├── team/           ← TeamUsecase + TeamRepository + Team 模型
├── channel/        ← ChannelUsecase + ChannelTurnJobUsecase + Channel 模型
├── session/        ← SessionUsecase + SessionRepository + Session 模型
├── chat/           ← ChatUsecase + ChatOrchestrator + Chat 模型
├── tool/           ← ToolUsecase + ToolRepository + Tool 模型
├── skill/          ← SkillUsecase + SkillRepository
├── mcp/            ← MCPServerUsecase + MCP 模型
├── memory/         ← MemoryUsecase + L4GraphUsecase + MemoryWorker
├── graph/          ← GraphUsecase + Graph 模型
├── usage/          ← UsageUsecase + Usage 模型
├── monitor/        ← MonitorUsecase + Monitor 模型
├── hook/           ← HookUsecase + HookDeliveryUsecase
├── cron/           ← CronUsecase
├── plugin/         ← PluginUsecase
├── event/          ← EventStoreUsecase + FlowLogUsecase
├── a2a/            ← A2AUsecase
├── knowledge/      ← KnowledgeUsecase
├── evaluation/     ← EvalUsecase
├── avatar/         ← AvatarUsecase
├── artifact/       ← ArtifactUsecase
├── ecosystem/      ← EcosystemUsecase
├── shared/         ← 跨聚合共享：pagination, errors, json_list, json_schema, channelicons
└── biz.go          ← Wire ProviderSet（聚合各子包）
```

**规则**：
- 子包间**禁止**直接引用具体 Usecase 结构体，必须通过接口
- 子包间接口定义在**消费方**子包（依赖倒置）
- `shared/` 只放纯值对象和通用错误码，不放 Usecase

**迁移策略**：
1. 先建子包目录，将对应文件移动过去，package 名改为子包名
2. 保留 `internal/biz` 作为 re-export 兼容层（`type AgentUsecase = agent.AgentUsecase`），逐步迁移调用方
3. Wire ProviderSet 拆为各子包独立 ProviderSet，biz.go 聚合
4. 优先迁移**无交叉依赖**的 Usecase（avatar, hook, skill, flow_log, eval, ecosystem, artifact），风险最低

**必须同步修复的交叉引用**：

| 当前 | 改为 |
|---|---|
| `ChannelTurnJobUsecase.channels: *ChannelUsecase` | 定义 `ChannelLookup` 接口在 channel 子包 |
| `AgentMCPTooling.agents: *AgentUsecase` | 定义 `AgentEffectiveToolsLookup` 接口 |
| `AgentMCPTooling.mcp: *MCPServerUsecase` | 定义 `MCPToolingLookup` 接口 |

### Phase 3：消除 biz 对 internal 子包的反向依赖

**原则**：biz 层只定义接口，具体实现在 data/service/agent 等上层包注入。

#### 3.1 event 包解耦（影响最大，12+ 文件）

`event.Bus` 已是接口，但 biz 还使用了 `event.Envelope`、`event.EventType`、`event.Buffer` 等具体类型。

**方案**：提取 `internal/event/contract` 子包

```
internal/event/
├── contract/       ← 纯接口 + 值对象：Bus, Envelope, EventType, Subscription
│   ├── bus.go      ← Bus 接口定义
│   ├── envelope.go ← Envelope 值对象
│   └── types.go    ← EventType 常量
├── bus.go          ← Bus 实现
├── buffer.go       ← Buffer 实现
└── ...
```

biz 只 import `event/contract`，不 import `event`（含实现）。

#### 3.2 其他反向依赖

| 当前依赖 | 改为 |
|---|---|
| `biz → internal/memory/trpc` | biz 定义 `MemoryRuntimeProvider` 接口，`runtime` 包实现并注入 |
| `biz → internal/data/sessionmemory` | biz 定义 `SessionMemoryStore` 接口，`data` 包实现并注入 |
| `biz → internal/tools/webresearch` | biz 定义 `WebResearchToolFactory` 接口，`tools/webresearch` 实现并注入 |
| `biz → internal/tools/testexec` | biz 定义 `TestToolInvoker` 接口，`tools/testexec` 实现并注入 |
| `biz → internal/mcp/*` | biz 定义 `MCPProber` / `MCPConfigProvider` 接口，`mcp` 包实现并注入 |
| `biz → internal/llminspect` | biz 定义 `LLMInspector` 接口，`llminspect` 实现并注入 |

### Phase 4：team 包对 agent + runtime 的依赖收敛

**现状**：team 包有 6 个文件依赖 agent、3 个文件依赖 runtime。这是合理的依赖方向（team 编排 agent），但接口边界需收敛。

**目标**：team 只依赖 agent 的**公开接口**，不依赖内部实现细节。

1. **agent 包定义 `AgentBuilder` 接口**：team 通过接口构建 agent，不直接引用 `chatagent.TRPCBuilderDeps` 等具体类型
2. **runtime 包定义 `RunStatusQuerier` 接口**：team 通过接口查询运行状态，不直接引用 `rt.RunRegistry`

### Phase 5：Channel 平台注册解耦

**目标**：新增平台无需修改 `channel_platform_registry.go`。

**方案**：采用 `init()` 自注册模式（参考 `database/sql` 驱动注册模式）。

```go
// internal/channel/port/registry.go
var platforms = map[string]PlatformFactory{}

func RegisterPlatform(name string, factory PlatformFactory) {
    platforms[name] = factory
}
```

各平台包在 `init()` 中自注册：
```go
// internal/channel/lark/lark.go
func init() {
    port.RegisterPlatform("lark", NewLarkPlatform)
}
```

`channel_platform_registry.go` 简化为遍历 `port.ListPlatforms()`。

### Phase 6：wire.go 瘦身

**目标**：wire.go 只做纯组装，不包含业务逻辑。

1. **`providePluginRuntime` 中的 `SetCatalogConfirmChecker` 闭包**：移入 `plugin/trpc` 包内部
2. **`provideAutoMemoryWorker` 中的 `data.NewL4GraphWriterAdapter`**：移入 `data` 包的 ProviderSet
3. **`provideChatServiceDeps` 30 个参数**：Phase 1 完成后自然缩减

---

## 4. 新增模块接入规范

为防止耦合回弹，新增模块必须遵循：

### 4.1 分层规则

| 层 | 允许 import | 禁止 import |
|---|---|---|
| `service` | biz/*, proto, event/contract | agent, team, runtime, tools/*, data |
| `biz/*` | biz/shared, event/contract | service, data, agent, team, runtime, tools/*, mcp/*, memory/* |
| `data` | biz/*, ent, conf | service, agent, team, runtime |
| `agent` | biz/* (接口), event/contract, provider | team, runtime, service |
| `team` | biz/* (接口), agent (接口), event/contract, runtime (接口) | service |
| `runtime` | biz/* (接口), agent (接口), event/contract | team, service |
| `event` | metrics | biz, agent, team, runtime, service |

### 4.2 Usecase 间通信规则

- **同一聚合根内**：Usecase 可直接引用同包其他 Usecase
- **跨聚合根**：必须通过接口，接口定义在消费方
- **跨模块事件**：通过 `event.Bus` 发布/订阅，禁止同步调用

### 4.3 新 Usecase 检查清单

- [ ] Usecase 只依赖 Repo 接口 + 标准库 + biz/shared
- [ ] 如需跨聚合根，定义接口在**本包**，实现由调用方注入
- [ ] 不直接 import `internal/event`（用 `event/contract`）
- [ ] 不直接 import 任何 `tools/*` 实现包
- [ ] 不直接 import `internal/data/*` 实现包
- [ ] 不直接 import `internal/mcp/*` 实现包

### 4.4 Service 层检查清单

- [ ] Service 构造函数参数 ≤ 5 个
- [ ] Service 不持有 `*team.Runner`、`*plugintrpc.Runtime` 等运行时具体类型
- [ ] Service 不写业务逻辑（if/for 业务判断放 biz）
- [ ] Wire provider 函数不包含业务闭包

---

## 5. 量化指标与验收标准

| 指标 | 当前值 | Phase 1 目标 | Phase 3 目标 |
|---|---|---|---|
| `provideChatServiceDeps` 参数数 | 30 | ≤ 10 | ≤ 6 |
| ChatService 结构体字段数 | 16 | ≤ 8 | ≤ 5 |
| biz 包文件数 | 100+ | 100+（先拆 ChatService） | 每子包 ≤ 15 |
| biz 直接依赖 internal 子包数（非 event） | 8 | 8 | 0 |
| biz 依赖 event 包文件数 | 12+ | 12+ | 0（改用 event/contract） |
| team 依赖 agent 文件数 | 6 | 6 | ≤ 3（通过接口） |
| 新增平台需修改文件数 | 1 (registry) | 1 | 0 |
| wire.go provider 函数含业务逻辑 | 3 处 | 3 | 0 |

---

## 6. 实施优先级

| 优先级 | Phase | 预期收益 | 风险 | 前置条件 |
|---|---|---|---|---|
| **P0** | Phase 1: ChatService 拆分 | Wire 参数 30→10，service 回归传输职责 | 涉及运行时行为变更，需 E2E 覆盖 | 无 |
| **P1** | Phase 2: biz 拆包 | 编译速度、心智模型、后续优化基础 | 迁移工作量大，需 Wire 配套调整 | Phase 1（ChatOrchestrator 需要先有 biz/chat 子包） |
| **P2** | Phase 3: 消除 biz 反向依赖 | 依赖方向合规、可独立测试 | 接口抽取需谨慎，避免过度抽象 | Phase 2（子包边界清晰后才好定义接口） |
| **P3** | Phase 4: team 依赖收敛 | team/agent 接口稳定 | 需要接口设计 | Phase 2 |
| **P4** | Phase 5: Channel 自注册 | 扩展性 | 低风险，独立可做 | 无 |
| **P5** | Phase 6: wire.go 瘦身 | 组装逻辑归位 | 低风险 | Phase 1 + Phase 3 |

**关键路径**：Phase 1 → Phase 2 → Phase 3。Phase 4/5/6 可并行。

---

## 7. 反模式警示

以下模式在新增代码中**禁止**引入：

1. **God Service**：一个 Service 持有 10+ Usecase/Runner 依赖 → 应拆为多个专注 Service + biz 层编排器
2. **Biz 直接引用实现包**：`biz → internal/data/*`、`biz → internal/tools/*` → 应定义接口注入
3. **Usecase 间同步调用具体结构体**：`*ChannelUsecase` 在 `ChannelTurnJobUsecase` 中 → 应通过接口
4. **平台硬编码注册**：`switch provider { case "lark": ... case "slack": ... }` → 应自注册
5. **Wire provider 含业务闭包**：`providePluginRuntime` 中的 `SetCatalogConfirmChecker(func()...)` → 应移入实现包
6. **30+ 参数的 provider 函数**：→ 应拆分为多个小 provider + 结构体内聚组装
