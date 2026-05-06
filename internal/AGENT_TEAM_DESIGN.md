# Agent & Team 运行时设计（Aranea Admin）

## Kratos v2 + ADK Runtime 分工（迁移后）

- **Kratos v2**：HTTP/gRPC/SSE 传输、`wire` 注入、`conf.Bootstrap`、`recovery` / `tracing` / `auth` / CORS 等中间件、kratos log 与 `kerrors`。**`internal/server` 禁止 import `google.golang.org/adk`**。
- **ADK Runtime（`pkg/adk-go`）**：`runner.Runner`、`llmagent` / workflow agents、工具循环、会话事件、插件与遥测。执行入口在 **`internal/service` + `internal/adkadapter`**，不在 transport 层。
- **边界全文**：见仓库根目录 **`docs/AGENT_RUNTIME_BOUNDARY.md`**（依赖方向与「八不准」）。

以下各节描述业务契约；实现细节以 ADK Runner 事件流为准。

## 1. 目标

- **`internal/agent`**：单次「catalog Agent + OpenAI 兼容提供商」的会话回合执行（拼 system、历史、调用、落库策略），与 HTTP 传输无关。
- **`internal/team`**：根据 `biz.Team.DefinitionJSON` 编排多 Agent（顺序 / 并行 + 合成器），写入 `team_runs` / `team_run_steps` 并可选通过 `TeamRunEventBroker` 推送事件；会话消息仍落在既有 `SessionRepository`。
- **`internal/service/chat*.go`**：仅做网关（legacy / 原生），解析请求、加载 `Session`，然后委托 `agent` 或 `team`。

## 2. 配置来源（唯一真相在 biz）

| 配置 | biz 来源 |
|------|-----------|
| Agent 文案 / Prompt 文件 | `Agent`, `AgentRepository.ListAgentPromptFiles` |
| Provider / Model / API | `Session` 覆盖 + `Agent` 默认 + `LlmProviderModelUsecase.GetByProviderAndModel` |
| Team 拓扑 | `Team.DefinitionJSON`（`mode`、`members`、`synthesizer_agent_id`） |
| Team 运行记录 | `TeamRepository`（本设计扩展 `CreateTeamRun` / `UpdateTeamRun` / `CreateTeamRunStep`） |

不在 biz 中的能力（例如 ADK `llmagent`、MCP 工具实例化）不在此包实现；后续可加 `Deps` 接口扩展。

## 3. 原生执行范围

- **Agent**：仅 **OpenAI 兼容** `/chat/completions`。若 `api_base_url` Host 为 Anthropic 官方 `api.anthropic.com`，仍会拒绝（原生 `/v1/messages` 与当前实现不兼容）；OpenAI 兼容网关（LiteLLM、OpenRouter 等）即使 `provider_type` 标为 anthropic 也可使用。
- **Team**：
  - **sequential**：按 `members` 顺序，逐步将**用户本轮输入**接在历史消息后调用各成员 Agent；每步产出一条 assistant 消息（`options_json` 含 `team_member`）。
  - **parallel**：对启用成员并发调用；若有 `synthesizer_agent_id` 或 role=`synthesizer` 的成员，用合成 Agent 将各条产出合并为最终回复；否则依赖 biz 校验已在并行多成员时要求 synthesizer。
  - **coordinator / critic_loop / adaptive**：原生执行器返回「未实现」，需 `LEGACY_REST_ORIGIN` 或后续迭代。

## 4. 会话与 API 契约

- **单 Agent 会话**（`owner_type=agent`）：行为与重构前一致（unary `AppendChatTurn`；stream 先 user 再 assistant）。
- **Team 会话**（`owner_type=team`）：`session.team_id` 必填；请求若带 `team_id` 须与之一致。
- **Unary 响应**：`agent_message` 为 **最后一轮助手消息**（team 下多为合成器或顺序最后一格）。
- **Stream**：`user_message` 后，`delta` 为模型 **增量**（OpenAI `stream: true` SSE 解析）；最后 `done` 含完整 `agent_message`。团队 **sequential** 仅**最后一员**、`parallel` 仅**合成器**（或 **单成员无合成器**）对流式输出；中间顺序成员仍用非流式请求，避免多路 SSE 交错。

## 5. 与 pkg/adk-go 的关系

当前仓库 **未** 将 `google.golang.org/adk` 链入主模块；本实现为 **进程内 Go 编排**，语义上对齐 ADK examples 中 sequential / parallel 子 Agent 组合，便于后续把 `internal/agent` 的 `ExecuteOpenAICompatTurn` 替换为 `llmagent` + `runner`。

## 6. 依赖图

```
service (chat) → agent (ExecuteOpenAICompatTurn)
service (chat) → team (Runner.RunTurn) → agent (按成员执行)
team → biz.TeamRepository (+ run/step 写入)
agent → biz.AgentRepository, LlmProviderModelUsecase
```

## 7. 文件布局

- `internal/agent/` — `openai_compat.go`, `options.go`, `prompt.go`, `turn.go`（含 `ExecuteOpenAIRelayStep`）, `parallel.go`（`ExecuteOpenAIParallelMember`）
- `internal/team/` — `definition.go`, `runner.go`, `provider.go`（wire）
