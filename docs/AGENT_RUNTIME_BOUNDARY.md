# Kratos v2 与 ADK Runtime 运行时边界

面向 admin（`cmd/admin`）：**Kratos v2** 负责传输与横切；**pkg/adk-go（ADK）** 负责 agent 业务执行。本仓库通过 **`internal/adkadapter`** 把 `internal/biz` 仓储与配置适配成 ADK 的 `session.Service` / `memory.Service` / `artifact.Service` / `model.LLM` / `agent.Agent`。

## 强制依赖方向

| 包路径 | 允许 import | 禁止 import |
|--------|-------------|-------------|
| `internal/server/*` | `internal/service`, `internal/conf`, kratos, `pkg/auth`, `pkg/validate` | `google.golang.org/adk/...` |
| `internal/biz/*` | stdlib, kratos errors, 本仓 biz/data API | `google.golang.org/adk/...` |
| `internal/service/*` | `internal/biz`, `internal/team`, `internal/adkadapter`, `google.golang.org/adk/runner`, `google.golang.org/adk/agent`（仅 Runner / RunConfig） | `google.golang.org/adk/tool/*` 直接组装（交给 adkadapter） |
| `internal/adkadapter/*` | `internal/biz`, `internal/data/...`, `google.golang.org/adk/...` | — |

**桥接约定**：`internal/service` 内 Kratos service 持有 `*runner.Runner`，将 RPC/HTTP 请求译为 `runner.Run`，将 `*session.Event` 流投影为 unary 或 SSE；不在 `internal/server` 或 `internal/biz` 中直接使用 ADK。

## 八条「不准做」清单

1. **`internal/server/*` 不得 new `runner.Runner` 或 `llmagent.New`**：Runner 仅由 `cmd/admin` wire 注入到 service/adkadapter。
2. **`internal/biz/*` 不得 import ADK**：配置与仓储仍是真相源，不参与执行链。
3. **ADK `plugin.Plugin` 的回调不得直接写数据库**：业务写库仍在 service/biz/repository；插件内可发布事件到 broker 再异步写。
4. **不得绕过 `internal/adkadapter` 把 Ent 行塞进 `session.Event`**：会话快照读写仅经 `BizSessionService`。
5. **不得在 transport 层解析工具参数或拼接 prompt**：仅 service + adkadapter。
6. **不得为 ADK 另起独立 HTTP 监听**：若挂 A2A，须用 Kratos `http.Server` 的 `Handle`。
7. **不得把 Kratos `middleware` 逻辑复制进 `pkg/adk-go`**：可观测性与请求元数据通过 context / OTel TracerProvider 对齐。
8. **`internal/agent` 过渡期后须删除**：执行链统一走 ADK + adapter（见 migration plan）。

## 相关文档

- `internal/AGENT_TEAM_DESIGN.md` — Agent/Team 行为与 API 契约
- `docs/AI-全栈新功能开发规范.md` — 全栈开发规范
- `.cursor/rules/adk-framework-first.mdc` — Cursor 规则摘要
