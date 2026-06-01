# 后端分层规范

> 来源：项目规则 + `aranea-coding-guide` SKILL 精简版。

---

## 一、依赖方向

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

### Biz 层 (`internal/biz/`)

| 规则 | 说明 |
|------|------|
| 禁止 import `pkg/trpc-agent-go` | 框架交互通过 `internal/agent`/`internal/tools` 桥接（红线 #1） |
| 禁止 import `api/*/v1` | proto 映射只在 Service 层（红线 #2） |
| 定义 Repo 接口 | 接口在 biz 定义，data 层实现 |
| 定义跨模块端口接口 | 端口在 biz 定义，Wire 绑定在 service 层 |
| 错误用 `kerrors` | 禁止 `fmt.Errorf` 返回业务错误 |
| Repo 接口方法 ≤ 5 | 超过按职责域拆分子接口（红线 #15） |

### Data 层 (`internal/data/`)

| 规则 | 说明 |
|------|------|
| 仅通过 `d.Ent()` / `d.Postgres()` 访问 | 不得另开 SQLite 连接（红线 #11） |
| 编译期接口检查 | `var _ biz.XxxRepo = (*xxxRepo)(nil)` |
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

---

## 四、工具装配

新增工具流程：

1. `Registry()` 注册 `ToolRegistration`
2. `builtin_tools_seed.go` 添加种子
3. Chat/Team 共用同一 `BuildToolsets` 逻辑

装配顺序：Registry 注册 → 配置覆盖 → OpenAPI → workspace_exec → AgentTool → MCP ToolSet → MCP Broker → CustomTools

---

## 五、记忆系统

- 记忆工具通过 `memory.Service.Tools()` 注入
- 记忆写入经 broker/async 异步写（红线 #8）
- 5 层：L0 快照 → L1 字段 → L2 事实 → L3 实体 → L4 级联

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
- 构造函数参数：只接收接口或具体依赖，不接收"上帝对象"
- 禁止手动编辑 `wire_gen.go`，必须通过 `make wire` 生成

---

## 九、错误处理

统一使用 `kerrors`，禁止 `fmt.Errorf` 返回业务错误：

```go
kerrors.BadRequest("AGENT", "id is required")
kerrors.NotFound("AGENT", "agent not found")
kerrors.InternalServer("AGENT", err.Error())
```

---

## 十、验证命令

| 改动类型 | 最小验证 |
|---------|---------|
| 仅 Service + 单测 | `go test ./internal/service/... -run TestXxx -count=1` |
| 仅 Biz/Data | `go test ./internal/biz/... ./internal/data/... -count=1` |
| Proto 变更 | `make api && go build ./...` |
| Wire 注入 | `make wire && go build ./cmd/admin` |
| **提交前（全量）** | `make api && make wire && make build && make test && make lint` |
