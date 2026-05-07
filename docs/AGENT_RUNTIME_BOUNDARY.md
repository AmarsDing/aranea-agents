# Kratos v2 与 ADK Runtime 运行时边界

面向 admin（`cmd/admin`）：**Kratos v2** 负责传输与横切；**pkg/adk-go（ADK）** 负责 agent 业务执行。本仓库将 ADK 集成**按领域拆开**，不再使用单独的 `internal/adkadapter`：

| 能力 | 主要包 |
|------|--------|
| `session.Service`（`adk_snapshot_json` 读写） | `internal/agent/adksvc`（`BizSessionService`） |
| `llmagent` 构建、`model.LLM` | `internal/agent`（`BuildLLMAgent`）、`internal/provider`（`ModelForProviderModel`） |
| 工具有效列表 → `tool.Tool` | `internal/tools`（`ToolsForAgent`） |
| Runner 内存 / 插件 / 用户 ID 上下文 | `internal/agent`（`NewADKMemoryService`、`DefaultRunnerPlugins`、`UserIDFromCtx` 等） |
| Team 工作流根 Agent | `internal/team`（`BuildWorkflowRoot` + `runner.Run`） |

## 强制依赖方向

| 包路径 | 允许 import | 禁止 import |
|--------|-------------|-------------|
| `internal/server/*` | `internal/service`, `internal/conf`, kratos, `pkg/auth`, `pkg/validate` | `google.golang.org/adk/...` |
| `internal/biz/*` | stdlib, kratos errors, 本仓 biz/data API | `google.golang.org/adk/...` |
| `internal/service/*` | `internal/biz`, `internal/team`, `internal/agent`, `internal/agent/adksvc`, `internal/provider`, `internal/tools`, `google.golang.org/adk/runner`, `google.golang.org/adk/agent`（仅 Runner / RunConfig） | 在 service 中直接拼装大量 `adk/tool`（优先经 `internal/tools`） |
| `internal/agent/*`（含 `adksvc`） | `internal/biz`, `internal/provider`, `internal/data/...`（如需要）, `google.golang.org/adk/...` | — |
| `internal/team/*` | `internal/biz`, `internal/agent`, `internal/provider`, `internal/tools`, `google.golang.org/adk/...` | — |
| `internal/provider/*` | `internal/biz`, `google.golang.org/adk/...`（model 适配） | — |
| `internal/tools/*` | `internal/biz`, `google.golang.org/adk/tool` | — |

**桥接约定**：`internal/service` 内 Kratos service 在方法中 `runner.New(...)`，将 RPC/HTTP 请求译为 `runner.Run`，将 `*session.Event` 流投影为 unary 或 SSE；不在 `internal/server` 或 `internal/biz` 中直接使用 ADK。

## 八条「不准做」清单

1. **`internal/server/*` 不得 new `runner.Runner` 或 `llmagent.New`**：Runner 仅在 `internal/service` / `internal/team` 等业需编排处构造（由 `cmd/admin` wire 注入依赖）。
2. **`internal/biz/*` 不得 import ADK**：配置与仓储仍是真相源，不参与执行链。
3. **ADK `plugin.Plugin` 的回调不得直接写数据库**：业务写库仍在 service/biz/repository；插件内可发布事件到 broker 再异步写。
4. **不得绕过 `internal/agent/adksvc` 把 Ent 行塞进 `session.Event`**：会话快照读写仅经 `BizSessionService`。
5. **不得在 transport 层解析工具参数或拼接 prompt**：在 service + `internal/agent` / `internal/tools` 层完成。
6. **不得为 ADK 另起独立 HTTP 监听**：若挂 A2A，须用 Kratos `http.Server` 的 `Handle`。
7. **不得把 Kratos `middleware` 逻辑复制进 `pkg/adk-go`**：可观测性与请求元数据通过 context / OTel TracerProvider 对齐。
8. **预留目录**：`internal/channel/adk`、`internal/skill/adk` 仅占位，避免将来再次集中「大适配包」；渠道/Skill 相关 ADK 扩展应落在对应包内。

## 相关文档

- `internal/AGENT_TEAM_DESIGN.md` — Agent/Team 行为与 API 契约
- `docs/AI-全栈新功能开发规范.md` — 全栈开发规范
- `.cursor/rules/adk-framework-first.mdc` — Cursor 规则摘要
