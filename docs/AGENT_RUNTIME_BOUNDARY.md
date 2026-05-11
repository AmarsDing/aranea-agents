# Kratos v2 与 tRPC-Agent-Go 运行时边界

面向 admin（`cmd/admin`）：**Kratos v2** 负责传输与横切；**pkg/trpc-agent-go（tRPC-Agent-Go）** 负责 agent 业务执行。本仓库将框架集成**按领域拆开**，不再使用单独的 `internal/adkadapter`：

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
| `internal/server/*` | `internal/service`, `internal/conf`, kratos, `pkg/auth`, `pkg/validate` | `pkg/trpc-agent-go` / 框架运行时私有 import（参见 `go.mod`） |
| `internal/biz/*` | stdlib, kratos errors, 本仓 biz/data API | `pkg/trpc-agent-go` / 框架运行时私有 import |
| `internal/service/*` | `internal/biz`, `internal/team`, `internal/agent`, `internal/agent/adksvc`, `internal/provider`, `internal/tools`，以及框架提供的 Runner / Agent 装配 API（import 路径以 `go.mod` 为准） | 在 service 中绕过 `internal/tools` 大量直连拼装底层 `tool` |
| `internal/agent/*`（含 `adksvc`） | `internal/biz`, `internal/provider`, `internal/data/...`（如需要）, `pkg/trpc-agent-go` / 框架运行时 | — |
| `internal/team/*` | `internal/biz`, `internal/agent`, `internal/provider`, `internal/tools`, `pkg/trpc-agent-go` / 框架运行时 | — |
| `internal/provider/*` | `internal/biz`, `pkg/trpc-agent-go` / 框架 `model` 适配 | — |
| `internal/tools/*` | `internal/biz`, 框架 `tool` API（由 `pkg/trpc-agent-go` 暴露或兼容层 re-export，以 `go.mod` 为准） | — |

**桥接约定**：`internal/service` 内 Kratos service 在方法中构造框架 `Runner`，将 RPC/HTTP 请求译为会话执行入口，将会话事件流投影为 unary 或 SSE；不在 `internal/server` 或 `internal/biz` 中直接使用框架运行时。

## 八条「不准做」清单

1. **`internal/server/*` 不得 new `runner.Runner` 或 `llmagent.New`**：Runner 仅在 `internal/service` / `internal/team` 等业需编排处构造（由 `cmd/admin` wire 注入依赖）。
2. **`internal/biz/*` 不得 import `pkg/trpc-agent-go` / 框架运行时**：配置与仓储仍是真相源，不参与执行链。
3. **框架 `plugin` 回调不得直接写数据库**：业务写库仍在 service/biz/repository；插件内可发布事件到 broker 再异步写。
4. **不得绕过 `internal/agent/adksvc` 把 Ent 行塞进 `session.Event`**：会话快照读写仅经 `BizSessionService`。
5. **不得在 transport 层解析工具参数或拼接 prompt**：在 service + `internal/agent` / `internal/tools` 层完成。
6. **不得为框架运行时另起独立 HTTP 监听**：若挂 A2A，须用 Kratos `http.Server` 的 `Handle`。
7. **不得把 Kratos `middleware` 逻辑复制进 `pkg/trpc-agent-go`**：可观测性与请求元数据通过 context / OTel TracerProvider 对齐。
8. **Skill 与框架 toolset 边界**：平台 Skill → 框架装配落在 **`internal/tools/skillruntime`**（及 `catalog.Options.SkillsFS`）；勿在 `internal/biz` 直接依赖框架运行时。渠道侧预留 **`internal/channel/adk`**（仅占位时勿塞业务逻辑）。

## 相关文档

- `docs/AGENT_SKILLS_TOOLS_MCP_MEMORY.md` — Skill / tools / MCP / 记忆运行时与演进计划（含 `TurnMount`、`adkdeps`、`AgentMCPTooling`）
- `docs/AI-全栈新功能开发规范.md` — 全栈开发规范
- `.cursor/rules/trpc-agent-framework-first.mdc` — Cursor 规则摘要
