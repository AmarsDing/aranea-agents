# 模块开发计划索引

> **编制**：2026-05-17  
> **用途**：各功能域的迭代路线、差距分析与可拆 PR 任务；**进度真相**仍以 [`guides/execution-plan.md`](../guides/execution-plan.md) 附录 A 为准。  
> **生成**：可由 `gen_dev_plans.py` 再生成骨架；手工增补的模块以文件正文为准。

---

## 如何使用

1. 开工前读对应 `*-development.md` + `需求/<n> *.md` + `*.design.md`。
2. 在 `execution-plan.md` §3 / §5 找到关联 `EP-*` 编号。
3. PR 合并后更新：附录 A、需求文档「现状对齐」、本计划 Phase 勾选（可选）、`changelog/`。

---

## 接入层 / 对话

| 计划 | 需求文档 | 状态摘要 |
|------|----------|----------|
| [1-chat-development.md](./1-chat-development.md) | 1 chat | ✅ 端到端 |
| [message-development.md](./message-development.md) | 51 / 51a / 51b | 🟡 WS 主 + SSE 兼容 |
| [10-session-development.md](./10-session-development.md) | 10 session | ✅ |
| [8-agent-title-development.md](./8-agent-title-development.md) | 8 agent-title | ✅ |

---

## Agent 域

| 计划 | 需求文档 | 状态摘要 |
|------|----------|----------|
| [2-agents-create-development.md](./2-agents-create-development.md) | 2 agents-create | ✅ |
| [3-agent-list-development.md](./3-agent-list-development.md) | 3 agent-list | ✅ |
| [4-agent-type-development.md](./4-agent-type-development.md) | 4 agent-type | ✅ |
| [5-agent-setting-development.md](./5-agent-setting-development.md) | 5 agent-setting | ✅；Override EP-BIZ-06 |
| [6-agent-setting-file-development.md](./6-agent-setting-file-development.md) | 6 agent-setting-file | ✅ |
| [7-agent-evolution-development.md](./7-agent-evolution-development.md) | 7 agent-evolution | 🟡 Scanner 未实现 |
| [50-avatar-development.md](./50-avatar-development.md) | 50 Avatar | ✅ |

---

## 运行时 / 编排

| 计划 | 需求文档 | 状态摘要 |
|------|----------|----------|
| [40-runner-development.md](./40-runner-development.md) | 40 runner | ✅ |
| [11-multi-agent-development.md](./11-multi-agent-development.md) | 11 multi-agent | ✅ |
| [36-graph-development.md](./36-graph-development.md) | 36 graph-workflow | ✅ |
| [39-planner-development.md](./39-planner-development.md) | 39 planner | ✅ |
| [22-plugin-development.md](./22-plugin-development.md) | 22 plugin | ✅ 注入；EP-CB-01 |
| [28-callback-development.md](./28-callback-development.md) | 28 callback | 🟡 Tool ✅ |
| [35-gateway-development.md](./35-gateway-development.md) | 35 gateway | 🟡 |
| [34-event-development.md](./34-event-development.md) | 34 event-system | ✅ |

---

## 能力扩展

| 计划 | 需求文档 | 状态摘要 |
|------|----------|----------|
| [20-skill-development.md](./20-skill-development.md) | 20 skill | ✅ |
| [23-tools-development.md](./23-tools-development.md) | 23 tools | 🟡 Override |
| [19-mcp-development.md](./19-mcp-development.md) | 19 mcp | 🟡 |
| [32-codeexecutor-development.md](./32-codeexecutor-development.md) | 32 codeexecutor | ✅ selector |
| [37-knowledge-development.md](./37-knowledge-development.md) | 37 knowledge | 🟡 EP-DATA-01 |
| [27-artifact-development.md](./27-artifact-development.md) | 27 artifact | 🟡 |
| [26-a2a-development.md](./26-a2a-development.md) | 26 a2a-protocol | 🟡 |
| [33-evaluation-development.md](./33-evaluation-development.md) | 33 evaluation | 🟡 |
| [memory-development.md](./memory-development.md) | 12–16 / 38 / 31 | 🟡 L4 缺 |

---

## 平台 / 运维

| 计划 | 需求文档 | 状态摘要 |
|------|----------|----------|
| [0-system-development.md](./0-system-development.md) | 0 系统框图 | 架构维护 |
| [admin-auth-development.md](./admin-auth-development.md) | Admin（附录 A） | ✅；M2 租户 |
| [9-provider-development.md](./9-provider-development.md) | 9 provider | ✅ |
| [17-channel-development.md](./17-channel-development.md) | 17 channel | 🟡 飞书 |
| [18-monitor-development.md](./18-monitor-development.md) | 18 monitor | ✅ |
| [21-cron-development.md](./21-cron-development.md) | 21 cron | ✅ |
| [29-token-development.md](./29-token-development.md) | 29 token | ✅ |
| [24-telemetry-development.md](./24-telemetry-development.md) | 24 telemetry | 📄 占位 |

---

## 远期 / 占位

| 计划 | 需求文档 | 状态摘要 |
|------|----------|----------|
| [25-cli-development.md](./25-cli-development.md) | 25 cli | ❌ M5 |
| [30-ecosystem-development.md](./30-ecosystem-development.md) | 30 ecosystem | 📄 M5 |
| [tts-development.md](./tts-development.md) | tts | 📄 M5 |

---

## 横切优先级（所有模块）

| EP | 说明 | 影响模块 |
|----|------|----------|
| EP-DATA-01 | 启动期 `Ensure*Schema`；Knowledge nil Repo | 37 / 33 / 26 |
| EP-WS-01/02 | Ent Hook + workspace 写断言 | 几乎全部写路径 |
| EP-CB-01 | Callback Chain → LLMAgent | 22 / 28 |
| EP-A2A-01/02 | A2A 真派发 + 鉴权 | 26 |
| EP-BIZ-06 | tool_override CRUD | 5 / 23 |

---

## 文档缺口说明

- `docs/README.md` 曾引用 `channel-requirements-analysis.md`，文件不存在；已在 [17-channel-development.md](./17-channel-development.md) Phase 1 跟踪，或后续补写分析文档。
- `31 memery.md` 为历史拼写遗留；记忆 UX 以 `38 memory.md` + [memory-development.md](./memory-development.md) 为准。
