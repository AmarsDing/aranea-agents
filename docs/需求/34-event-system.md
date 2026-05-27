# M18: Event 事件系统 — 详细需求

> 对标 trpc-agent-go `event` 包，完善项目的事件系统。

---

## 1. 能力现状

| 能力 | 状态 | 说明 |
|------|------|------|
| 事件发布/订阅 | ✅ | Bus + 多种背压策略（DropOldest / DropNewest / BlockUpTo） |
| 事件推流（WebSocket） | ✅ | 统一 WS 传输，Chat / Monitor / Team / Graph 复用单连接 |
| 事件投影 | ✅ | trpc-agent-go Event → Envelope 投影，保留完整元数据 |
| StateDelta | ✅ | 事件携带状态增量，自动应用到 Session State |
| Branch 追踪 | ✅ | 事件携带 Branch 字段，多 Agent 场景记录执行路径 |
| Actions 流控制 | ✅ | SkipSummarization 等流控制提示 |
| Clone 深拷贝 | ✅ | Envelope.Clone() |
| LongRunningToolIDs | ✅ | 投影层映射为 ToolCall.IsLongRunning |
| FilterKey 层级过滤 | ✅ | 前缀匹配规则，WS 连接支持 filter_key 参数 |
| Tag 业务标签 | ✅ | Envelope.ContainsTag() 逗号分隔匹配（trpc Event 框架侧为分号） |
| run_status 运行态 | ✅ | Chat RunGateway 发布；前端 `envelopeRunStatus.ts` 解析 |
| flow_log 流程日志 | ✅ | TraceEmitter / SysLog*；Monitor Logs Tab 消费 |
| team_summary 摘要 | ✅ | Team Runner 投影；`teamRunEventFromEnvelope.ts` |
| knowledge_ingest 入库进度 | ✅ | Knowledge WS 通道 `knowledge` |
| 可观测性事件 | ✅ | `mcp.session.reconnect` / `alert.notify` → monitor 通道 |
| Extensions 扩展元数据 | ✅ | 命名空间化 map[string]string |
| 事件缓冲与重放 | ✅ | 环形缓冲 + TTL 淘汰 + lastEventID 断连重放 |
| 事件持久化 | ✅ | SQLite `event_store` + 异步 `eventPersistHandler`（排除 log/flow_log） |
| 事件回放 API | ✅ | `GET /v1/events` 按 session/时间/类型分页查询 |
| 前端 Chat 事件检视 | ✅ | SessionTimelineDialog 双 Tab + Inspector 组件 |

---

## 2. 需求清单

### 2.1 事件推流增强

**用户故事**：作为平台开发者，我希望事件推流携带完整元数据，以便前端能根据 FilterKey/Branch/Tag 等字段进行精细化展示和过滤。

**功能规格**：
- 事件推流包含 StateDelta / Extensions / FilterKey / Branch / Tag / Actions
- 前端可根据 FilterKey 过滤显示
- 前端可根据 Branch 追踪执行链
- 向后兼容：现有事件格式不变

**验收标准**：事件推流包含完整事件元数据，前端可按层级过滤事件流

### 2.2 StateDelta 处理

**用户故事**：作为 Agent 开发者，我希望事件中的 StateDelta 能自动应用到 Session State，以便 Agent 运行时状态能被持久化和共享。

**功能规格**：
- 事件携带 StateDelta（operation: set / append / delete，path，value_json）
- Runner 处理事件时自动将 StateDelta 合并到 Session State
- 前端可订阅状态变更通知

**验收标准**：StateDelta 正确应用到 Session State（set / append / delete 三种操作）

### 2.3 FilterKey 层级过滤

**用户故事**：作为前端开发者，我希望按层级过滤事件流，以便在多 Agent 场景中只关注特定 Agent 子树的事件。

**功能规格**：
- 事件携带 FilterKey（如 `agent_a/agent_b` 格式）
- 前缀匹配规则：`agent_a` 匹配 `agent_a/agent_b`，反之亦然
- 客户端连接时可指定 filter_key 参数

**验收标准**：前端可按层级过滤事件流

### 2.4 Branch 追踪

**用户故事**：作为平台运维者，我希望追踪 Agent 执行链，以便理解多 Agent 协作中的调用路径和决策过程。

**功能规格**：
- 事件携带 Branch 字段，记录 Agent 执行路径
- 事件携带 InvocationID / ParentInvocationID，构成调用树
- 前端可展示执行树

**验收标准**：多 Agent 场景中可追踪执行链

### 2.5 Extensions 扩展元数据

**用户故事**：作为 Agent 开发者，我希望事件可携带自定义扩展元数据，以便传递工具调用参数等额外信息而不污染主事件字段。

**功能规格**：
- 事件可携带 Extensions（命名空间化的 key-value 对）
- 命名空间化，避免冲突（如 `trpc_agent.tool_call_args`）
- 前端可解析和显示

**验收标准**：事件可携带自定义扩展元数据

### 2.6 Actions 流控制

**用户故事**：作为 Agent 开发者，我希望事件可携带流控制提示，以便 Runner 根据提示调整行为（如跳过摘要）。

**功能规格**：
- 事件可携带 Actions（如 SkipSummarization）
- Runner 根据 Actions 调整后续处理逻辑

**验收标准**：Runner 正确处理 Actions 提示

### 2.7 事件持久化

**用户故事**：作为平台运维者，我希望系统重启后可查询历史事件，以便进行问题排查和审计。

**功能规格**：
- 事件写入持久化存储
- 可按 session_id / 时间范围 / 事件类型查询
- 支持存储膨胀控制（TTL 清理或容量限制）

**验收标准**：系统重启后可查询历史事件

### 2.8 事件回放 API

**用户故事**：作为前端开发者，我希望按时间范围回放事件，以便重现历史事件序列和调试。

**功能规格**：
- 提供 HTTP API 按时间范围查询历史事件
- 返回事件列表，支持分页

**验收标准**：可按时间范围回放事件

### 2.9 Chat 会话事件检视

**用户故事**：作为平台使用者，我在 Chat 工作区查看当前会话的实时与历史 Envelope，理解 Agent 执行路径与状态变更。

**功能规格**：
- 在 Chat 工作区内打开**会话级检视面板**（Drawer/Dialog，非第四列固定侧边栏）
- 双 Tab：**历史 Trace**（Session HTTP 时间线）与 **实时 Envelope**（WS + `GET /v1/events` 回放）
- 实时 Tab：按事件类型 / 分支 / 标签 / 关键词过滤
- 展示 Branch 调用树（InvocationID / ParentInvocationID）
- 展示 StateDelta 变更（set / append / delete）与 Transfer 标签
- 长时运行工具进度指示（ToolCall.is_long_running）

**与 Monitor 分工**：Monitor Events Tab 面向跨会话运维；Chat 检视面向**当前 session** 的调试与理解执行链。

**验收标准**：Chat 内可按类型/分支/标签过滤会话事件流，Branch 树与 StateDelta 指示器可用

---

## 3. 验收标准总览

1. ✅ 事件推流包含完整事件元数据（StateDelta / Extensions / FilterKey / Branch / Tag / Actions）
2. ✅ StateDelta 正确应用到 Session State（set / append / delete 三种操作）
3. ✅ 前端可按层级过滤事件流（FilterKey 前缀匹配）
4. ✅ 多 Agent 场景中可追踪执行链（Branch + InvocationID / ParentInvocationID）
5. ✅ 事件可携带自定义扩展元数据（Extensions 命名空间化）
6. ✅ Runner 正确处理 Actions 提示（SkipSummarization）
7. ✅ 系统重启后可查询历史事件
8. ✅ 可按时间范围回放事件
9. Chat 会话事件检视：Drawer/Dialog 双 Tab（Trace + 实时 Envelope），支持类型/分支/标签过滤与 Branch 树
