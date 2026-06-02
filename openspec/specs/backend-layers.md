# 后端分层规范

> 来源：项目规则 + `aranea-coding-guide` SKILL 精简版。
> 架构上下文详见 [`architecture-blueprint.md`](./architecture-blueprint.md) §三。

---

## 一、依赖方向

**跨层只允许向内依赖。违反即停。**

```
api/**/*.proto → internal/service → internal/biz → internal/data
```

---

## 二、各层约束

### Server 层 (`internal/server/`)

| 规则 | 说明 |
|------|------|
| 只做传输注册 | `RegisterXxxHTTPServer` / `RegisterXxxServiceServer` |
| 中间件统一在此注册 | recovery → tracing → logging → auth → cors |
| 不得 new Runner | 不得 `runner.Runner` 或 `llmagent.New`（红线 #3） |
| 不得写业务路由 | 只做注册，不写逻辑（红线 #5） |

### Service 层 (`internal/service/`)

| 规则 | 说明 |
|------|------|
| proto ↔ biz 类型映射 | `toProtoXxx` / `fromProtoXxx` |
| Runner 装配唯一入口 | 唯一允许创建 Runner 的层（红线 #3） |
| 不得写业务逻辑 | Service 只做映射 + 编排（红线 #4） |
| 不得直接依赖 Repo | 通过 Usecase 层访问（红线 #13） |
| 错误映射用 `kerrors` | 禁止 `fmt.Errorf` |

当前共 38 个 Service struct（含 ChatService 实现 7 个 biz 端口接口）。

### Biz 层 (`internal/biz/`)

| 规则 | 说明 |
|------|------|
| 禁止 import `pkg/trpc-agent-go` | 框架交互通过 `internal/agent`/`internal/tools` 桥接（红线 #1） |
| 禁止 import `api/*/v1` | proto 映射只在 Service 层（红线 #2） |
| 定义 Repo 接口 | 接口在 biz 定义，data 层实现 |
| 定义跨模块端口接口 | 端口在 biz 定义，Wire 绑定在 service 层 |
| 错误用 `kerrors` | 禁止 `fmt.Errorf` 返回业务错误 |
| Repo 接口方法 ≤ 5 | 超过按职责域拆分子接口（红线 #15） |

当前共 36 个 Usecase struct，295 个接口定义分布在 100 个文件中。

### Data 层 (`internal/data/`)

| 规则 | 说明 |
|------|------|
| 仅通过 `d.Ent()` / `d.Postgres()` 访问 | 不得另开 SQLite 连接（红线 #11） |
| 编译期接口检查 | 65 条 `var _ biz.XxxRepo = (*xxxRepo)(nil)` 编译期接口检查，~60 个唯一 Repo/Adapter 实现 |
| 转换函数 | `entXxxToBiz` / `bizXxxToEnt` |

---

## 三、Agent 运行时集成铁律

| # | 铁律 | 正确做法 |
|---|------|---------|
| A1 | 所有 Agent 必须实现 `agent.Agent` 接口（5 方法） | `Run/Tools/Info/SubAgents/FindSubAgent` |
| A2 | 事件发射必须走 `agent.EmitEvent(ctx, inv, ch, evt)` | 禁止 `event.EmitEvent(context.Background(), ch, evt)` |
| A3 | Agent.Run() 内部不得发射 `ObjectTypeRunnerCompletion` | Runner 层统一发射 |
| A4 | 后台/定时 Agent 必须通过 `Runner.Run()` 调用 | 参考框架 `openclaw/internal/cron/service.go` |
| A5 | 工具构建使用 `function.NewFunctionTool[I, O]` | 禁止手动实现 `CallableTool` 接口 |
| A6 | 程序化 Agent 也必须走 Runner | Runner 管理 Session/Invocation/事件流生命周期 |
| A7 | 工具结果门控：`tool_result_gate` 控制 tool_result 事件是否推送给前端 | `ToolResultGate` 端口接口，Service 层实现 |

---

## 四、工具装配

新增工具流程：

1. `Registry()` 注册 `ToolRegistration`
2. `builtin_tools_seed.go` 添加种子
3. Chat/Team 共用同一 `BuildToolsets` 逻辑

装配顺序：Registry 注册 → 配置覆盖 → OpenAPI → workspace_exec → AgentTool → MCP ToolSet → MCP Broker → CustomTools

当前共 28 个注册工具 + ~37 个运行时注入工具（kanban/memory/subagent/modelsync/cli_admin/skills_butler/spirit_tools 等）。

---

## 五、记忆系统

### 5.1 两种工具路径

| 路径 | 工具 | 注入方式 |
|------|------|---------|
| 框架记忆工具 | memory_add/search/delete 等 6 个 | `memory.Service.Tools()` → 过滤 → `AssemblyConfig.MemoryTools` + `llmagent.WithMemoryService(service)` |
| L1 工作记忆工具 | working_memory.read/write/list/patch/delete 5 个 | `working_memory.ToolSet` → `BeforeToolHook` 注入 L1TaskWriter/L1FieldWriter/L1AdminReader/sessionID/agentID |

### 5.2 写入红线

- 记忆写入经 broker/async 异步写（红线 #8）
- L1 工作记忆工具写入是同步的（Agent 主动调用，非 plugin 回调）

### 5.3 五层架构

| 层级 | 存储 | 核心接口 | 关键 Worker |
|------|------|----------|-------------|
| L0 会话快照 | SQLite Session | `L0AdminStore` | 无 |
| L1 工作记忆 | SQLite Memory | `L1TaskWriter`(4) + `L1FieldWriter`(4) + `L1AdminReader`(4) | `MemoryL1ArchiveWorker`(5min) |
| L2 会话事件 | SQLite + pgvector | `L2RecallStore` + `L2EpisodeWriter` + `L2ConsolidationStore`(2) | `MemoryL2ConsolidateWorker`(10min) + `MemoryL2DecayWorker` |
| L3 语义知识 | SQLite + pgvector | `L3FactAdminStore` + `L3ConflictStore`(2) + `PIIReviewStore`(3) | `MemoryL3DecayWorker` + `MemoryFactIndexReconciler`(6h) |
| L4 持久进化 | SQLite Memory | `L4EntityStore`(5) + `L4EvolutionStore`(4) | `MemoryL4DecayWorker` |

### 5.4 关键数据流

- **L1→L2 桥接**：`EndL1Task` → `archiveAndCreateEpisode` → `ArchiveL1Task` + `InsertL1ArchiveEpisode`
- **L2 Consolidation**：Episode pending → `MemoryL2ConsolidateWorker` → consolidated（实际 LLM 提取由 AutoMemoryWorker 完成）
- **L3 冲突检测**：`UpsertFactRow` → `DetectFactConflicts`（best-effort）→ `IncrementConflictCount`
- **L3 PII 审核**：`ListPIIFlaggedFacts` / `ApprovePIIFact` / `RejectPIIFact`
- **L3 5维评分**：keyword(0.25) + vector(0.30) + importance(0.20) + recency(0.15) + quality(0.10)

### 5.5 SessionAdminStore 迁移

`SessionAdminStore`（38 方法）是向后兼容的组合接口，已标记 Deprecated。新代码应依赖细粒度子接口：
- `L0AdminStore`、`L1AdminReader`、`L1TaskWriter`、`L1FieldWriter`、`L1IdleTaskReader`
- `L2RecallStore`、`L2EpisodeWriter`、`L2ConsolidationStore`
- `L3FactAdminStore`、`L3ConflictStore`、`PIIReviewStore`
- `L4EntityStore`、`L4EvolutionStore`

---

## 六、Provider 集成

- 厂商连接收口在 `internal/provider`
- 契约对齐以 `pkg/trpc-agent-go/model` 为准
- 7 种 Provider：OpenAI/Anthropic/Gemini/Ollama/Hunyuan/HuggingFace/Bedrock
- HA 策略：Failover / Hedge

---

## 七、横切约束

| # | 约束 | 说明 |
|---|------|------|
| 1 | 所有 `go func()` 必须走 `pkg/safego` | 禁止裸 `go func()` 不处理 panic（红线 #9） |
| 2 | 禁止 `log/slog` | 统一使用 `pkg/loggateway.Logger`（红线 #10） |
| 3 | 跨模块调用通过 biz 级窄接口 | 禁止持有对方 Service 具体类型（红线 #7） |
| 4 | 异步事件通过 Broker 发布/订阅 | 禁止全局变量共享状态 |
| 5 | 框架 plugin 回调不得直接写数据库 | 经 broker/async 异步写（红线 #8） |
| 6 | 压缩操作 CAS + 事务 | `TryIncrementCompressVersion` + `CompressSessionInTx`（红线 #14） |
| 7 | 不得修改工具生成的代码 | protoc/wire/Ent 等，改源头 → 重新生成（红线 #6） |
| 8 | 不得新增已无调用者的 deprecated 方法 | 死代码即删（红线 #12） |

---

## 八、Wire 依赖注入

- Wire ProviderSet：每层一个（`biz.go` / `data.go` / `service.go` / `server.go`）
  - biz.ProviderSet — 36 个 Usecase
  - data.ProviderSet — ~60 个 Repo/Adapter 实现
  - service.ProviderSet — 38 个 Service + 16 条 Wire 接口绑定
  - cmd/admin/wire.go 额外 19 条 wire.Bind（跨层绑定 + biz 子接口窄化绑定）。
- 构造函数参数：只接收接口或具体依赖，不接收"上帝对象"
- 禁止手动编辑 `wire_gen.go`，必须通过 `make wire` 生成
- 关键绑定详见 [`architecture-blueprint.md`](./architecture-blueprint.md) §八

---

## 九、错误处理

统一使用 `kerrors`，禁止 `fmt.Errorf` 返回业务错误。示例详见 [`architecture-blueprint.md`](./architecture-blueprint.md) §三"错误处理规范"。

---

## 十、验证命令

| 改动类型 | 最小验证 |
|---------|---------|
| 仅 Service + 单测 | `go test ./internal/service/... -run TestXxx -count=1` |
| 仅 Biz/Data | `go test ./internal/biz/... ./internal/data/... -count=1` |
| Proto 变更 | `make api && go build ./...` |
| Wire 注入 | `make wire && go build ./cmd/admin` |
| **提交前（全量）** | `make api && make wire && make build && make test && make lint` |
