# Chat 执行过程卡片 — 需求规格

> **模块**：Chat 对话 · 执行可见性  
> **上级模块**：[1 chat.md](./1%20chat.md)  
> **技术设计**：[1 chat-execution-trace.design.md](./1%20chat-execution-trace.design.md)  
> **遵循**：[guides/AI-DEVELOPMENT-SPECIFICATION.md](../guides/AI-DEVELOPMENT-SPECIFICATION.md) · [guides/frontend-guide.md](../guides/frontend-guide.md)

---

## 1. 背景与目标

用户在 Chat 中与 Agent 对话时，需要**实时看到 Agent 正在做什么**（调用工具、加载/运行 Skill、通过 MCP 访问外部能力等），而不是仅看到最终文本回复或零散的 Markdown 提示。

本需求在**聊天消息流**中插入可折叠的**执行过程卡片（Execution Activity Card）**：卡片以图标 + 名称 + 状态为主信息，默认折叠详情，展开后可查看参数与结果。

> **与 Monitor 的边界**：Monitor → Logs / Traces 面向运维排障；Chat 执行卡片面向**终端用户理解 Agent 行为**，二者可共享同一条 `trace_id` / Span，但**不在 Chat 中展示 FlowLog 原始步骤**。

---

## 2. 用户故事

| ID | 角色 | 故事 | 优先级 |
|----|------|------|--------|
| U1 | 对话用户 | 发送消息后，我能看到 Agent 依次调用了哪些工具/Skill/MCP，以及当前是否仍在执行 | P0 |
| U2 | 对话用户 | 每张卡片标题显示**能力名称**（如 `read_file`、`skill_run`、`mcp_call`），执行中显示「正在执行」态，完成后显示**耗时** | P0 |
| U3 | 对话用户 | 卡片根据**成功 / 失败 / 阻塞（需确认）** 显示不同颜色与图标，失败时我能看到错误摘要 | P0 |
| U4 | 对话用户 | 卡片**默认折叠**，不占用阅读空间；需要时点击展开查看参数 JSON、结果摘要或 stderr | P0 |
| U5 | Team 用户 | 在 Team 会话中，卡片标明**执行成员 Agent**（author / agent_key） | P1 |
| U6 | 对话用户 | 刷新页面或 WS 重连后，仍能从历史消息中恢复已完成的执行卡片（与当轮持久化一致） | P1 |
| U7 | 管理员 | 敏感参数（API Key、token）在卡片详情中**脱敏**，不泄露明文 | P0 |

---

## 3. 功能范围

### 3.1 纳入范围（P0）

| 能力类型 | 典型名称 | 说明 |
|----------|----------|------|
| 平台工具 | `read_file`、`save_file`、`exec_command`、`todo_write` 等 | catalog / runtime 工具 |
| Skill | `skill_load`、`skill_run`、`skill_search` | 框架 Skill 工具族 |
| MCP | `mcp_call`、`mcp_list_tools` 及 MCP ToolSet 挂载工具 | 含 server_key / tool 名 |
| 内置能力 | `knowledge_search`、`call_agent`、`await_user_reply` | 随 Agent 策略挂载 |

### 3.2 纳入范围（P1）

| 能力类型 | 说明 |
|----------|------|
| 子 Agent | `transfer_to_agent`、`spawn_subagent` |
| Memory | `load_memory`、`preload_memory` |
| Team 步骤 | 与 `team_step_*` 事件对齐的成员级卡片（可选与成员气泡并列） |

### 3.3 不纳入范围

- Monitor FlowLog / Process Log 的 UI 迁移（仍在 Monitor → Logs）
- Graph 节点执行卡片（归属 Graph 执行页，见 `graphs` 模块）
- 修改 Agent 框架 `pkg/trpc-agent-go` 内部 Tool 语义（Aranea 仅在投影层扩展）

---

## 4. 交互规格

### 4.1 卡片布局（折叠态 — 默认）

```
┌─────────────────────────────────────────────────────────┐
│ [图标]  read_file                    正在执行…  ⏳      │
└─────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────┐
│ [图标]  skill_run · planning-and-task    1.2s  ✓      │
└─────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────┐
│ [图标]  mcp_call · github/list_issues    320ms  ✗      │
└─────────────────────────────────────────────────────────┘
```

| 区域 | 规则 |
|------|------|
| 图标 | 按 `activity_kind` 映射（tool / skill / mcp / …），见设计文档 §6 |
| 主标题 | `display_label`：优先 catalog 中文名，其次 runtime 名 |
| 副标题（可选） | 单行摘要：路径、skill 名、MCP server:tool，**不显示完整 JSON** |
| 右侧状态 | 执行中：`正在执行` + 动画；完成：耗时（≥1s 保留 1 位小数 + `s`，否则 `ms`） |
| 状态标记 | 成功 ✓ / 失败 ✗ / 阻塞 ⚠ / 长任务 ⏱ |

### 4.2 卡片布局（展开态）

点击卡片头部或 chevron 展开：

- **参数**：格式化 JSON（可滚动，最大高度约 280px）
- **结果**：stdout / JSON / 错误信息分区展示
- **元数据**（可选折叠）：`trace_id`、`run_id`、`invocation_id`、`duration_ms`

### 4.3 时间线位置

- 卡片插入在**当轮助手回复流**中，与 `text_delta` 交错，顺序等于 Agent **实际执行顺序**
- 同一 `activity_id` 仅对应**一张卡片**：`running` → `success|failed` 为**原地更新**，不新增重复行
- 不替代最终助手 Markdown 气泡；卡片与文本气泡均为 `role=assistant` 时间线上的独立条目

### 4.4 状态文案

| status | 折叠态标签 | 颜色语义 |
|--------|------------|----------|
| `running` | 正在执行 | warning / accent（UX `--color-warning`） |
| `success` | （仅耗时） | success（`--color-success`） |
| `failed` | 失败 + 耗时 | danger（`--color-danger`） |
| `blocked` | 待确认 | warning |
| `cancelled` | 已取消 | muted |

---

## 5. 非功能需求

| 项 | 要求 |
|----|------|
| 实时性 | WS 到达后 **200ms 内** UI 反映 running；完成态随 `tool_result` 即时更新 |
| 性能 | 单轮 ≥50 张卡片时列表仍流畅（虚拟滚动或增量 DOM，见设计 §8） |
| 可访问性 | 卡片 header 为 `button` 或带 `aria-expanded`；状态不仅依赖颜色 |
| 国际化 | 文案走 `vue-i18n`（`chat.activity.*`） |
| 安全 | 密钥字段脱敏；详情默认可复制但带审计提示（P2） |

---

## 6. 验收标准

- [x] 单 Agent 对话：调用任意已挂载工具时，Chat 中出现对应卡片；执行中显示「正在执行」，完成后显示耗时与成功/失败态
- [x] Skill：`skill_load` / `skill_run` 显示 Skill 图标与 skill 名称摘要
- [x] MCP：`mcp_call` 显示 server 与 tool 名摘要
- [x] 卡片**默认折叠**；展开后可见参数与结果
- [x] 同一工具调用不产生 duplicate 卡片（id 稳定 upsert）
- [x] WS 断线重连 + `last_event_id` 回放后，卡片状态与线上一致
- [x] 刷新会话历史：已完成轮次的卡片从 `messages` 只读还原（持久化命中）
- [x] Team 会话：卡片展示成员 Agent 标识（P1）
- [x] 失败工具调用：卡片红色边框 + 错误摘要，助手正文仍可继续输出

---

## 7. 关联文档

| 文档 | 关系 |
|------|------|
| [1 chat.md](./1%20chat.md) | Chat 主需求、WS Envelope 总览 |
| [1 chat.design.md](./1%20chat.design.md) | 现有 `tool_call` / `tool_result` 投影 |
| [52-flow-logger.design.md](./52-flow-logger.design.md) | Span / trace_id 同源；Chat 不展示 flow_log |
| [23 tools.md](./23%20tools.md) | 工具 catalog 与 risk_level |
| [20 skill.md](./20%20skill.md) | Skill 运行时 |
| [frontend/UX.md](../frontend/UX.md) | 玻璃卡片视觉 token |
