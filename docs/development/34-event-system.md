# M18: Event 事件系统 — 详细需求

> 对标 trpc-agent-go `event` 包，完善项目的事件系统。

> 进度状态（已实现/未实现能力清单）见 [34-event-system.development.md](./34-event-system.development.md) §2。

---

## 1. 需求清单

### 1.1 事件推流增强

**用户故事**：作为平台开发者，我希望事件推流携带完整元数据，以便前端能根据 FilterKey/Branch/Tag 等字段进行精细化展示和过滤。

**功能规格**：
- 事件推流包含 StateDelta / Extensions / FilterKey / Branch / Tag / Actions
- 前端可根据 FilterKey 过滤显示
- 前端可根据 Branch 追踪执行链
- 向后兼容：现有事件格式不变

**验收标准**：事件推流包含完整事件元数据，前端可按层级过滤事件流

### 1.2 StateDelta 处理

**用户故事**：作为 Agent 开发者，我希望事件中的 StateDelta 能自动应用到 Session State，以便 Agent 运行时状态能被持久化和共享。

**功能规格**：
- 事件携带 StateDelta（operation: set / append / delete，path，value_json）
- Runner 处理事件时自动将 StateDelta 合并到 Session State
- 前端可订阅状态变更通知

**验收标准**：StateDelta 正确应用到 Session State（set / append / delete 三种操作）

### 1.3 FilterKey 层级过滤

**用户故事**：作为前端开发者，我希望按层级过滤事件流，以便在多 Agent 场景中只关注特定 Agent 子树的事件。

**功能规格**：
- 事件携带 FilterKey（如 `agent_a/agent_b` 格式）
- 前缀匹配规则：`agent_a` 匹配 `agent_a/agent_b`，反之亦然
- 客户端连接时可指定 filter_key 参数

**验收标准**：前端可按层级过滤事件流

### 1.4 Branch 追踪

**用户故事**：作为平台运维者，我希望追踪 Agent 执行链，以便理解多 Agent 协作中的调用路径和决策过程。

**功能规格**：
- 事件携带 Branch 字段，记录 Agent 执行路径
- 事件携带 InvocationID / ParentInvocationID，构成调用树
- 前端可展示执行树

**验收标准**：多 Agent 场景中可追踪执行链

### 1.5 Extensions 扩展元数据

**用户故事**：作为 Agent 开发者，我希望事件可携带自定义扩展元数据，以便传递工具调用参数等额外信息而不污染主事件字段。

**功能规格**：
- 事件可携带 Extensions（命名空间化的 key-value 对）
- 命名空间化，避免冲突（如 `trpc_agent.tool_call_args`）
- 前端可解析和显示

**验收标准**：事件可携带自定义扩展元数据

### 1.6 Actions 流控制

**用户故事**：作为 Agent 开发者，我希望事件可携带流控制提示，以便 Runner 根据提示调整行为（如跳过摘要）。

**功能规格**：
- 事件可携带 Actions（如 SkipSummarization）
- Runner 根据 Actions 调整后续处理逻辑

**验收标准**：Runner 正确处理 Actions 提示

### 1.7 事件持久化

**用户故事**：作为平台运维者，我希望系统重启后可查询历史事件，以便进行问题排查和审计。

**功能规格**：
- 事件写入持久化存储
- 可按 session_id / 时间范围 / 事件类型查询
- 支持存储膨胀控制（TTL 清理或容量限制）

**验收标准**：系统重启后可查询历史事件

### 1.8 事件回放 API

**用户故事**：作为前端开发者，我希望按时间范围回放事件，以便重现历史事件序列和调试。

**功能规格**：
- 提供 HTTP API 按时间范围查询历史事件
- 返回事件列表，支持分页

**验收标准**：可按时间范围回放事件

### 1.9 Chat 会话事件检视

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

### 1.10 工具生命周期事件与自动触发（BabyAGI 启发，P2）

> 来源：BabyAGI Triggers 机制（GitHub 22k+ stars），竞品分析差距 #8
> 对应需求：`docs/competitive-gap-requirements-2026-05-31.md` P2-10

**用户故事**：作为平台开发者，我希望工具/Agent 的变更能自动触发相关操作（如新工具注册后自动生成描述和 embedding），以便减少人工编排。

**背景**：BabyAGI 的 functionz 框架实现了 Triggers 机制——当函数被添加或更新时，自动触发相关函数执行（如自动生成描述和 embedding）。当前 Aranea 的 `EventBus` 主要用于 WS 推送和内部消费者，工具/Agent 变更没有自动触发链。

**功能规格**：
- 增加 `ToolRegistered`/`ToolUpdated`/`ToolRemoved` 三种工具生命周期事件类型（EnvelopeType）
- `ToolRegistered` 事件触发 LLM 自动生成工具描述和 embedding（供 `tool_search` 语义检索）
- `ToolUpdated` 事件触发 `BuildTRPCAgentCached` 缓存失效
- `ToolRemoved` 事件触发依赖该工具的 Agent 配置告警
- 所有触发操作经 broker/async 异步执行（遵守红线 #8：框架 plugin 回调不得直接写数据库）
- 触发结果（成功/失败）记录到 FlowLog

**验收标准**：
- 新工具注册后自动生成描述和 embedding
- 工具更新后相关 Agent 缓存自动失效
- 触发操作异步执行，不阻塞主流程
- 触发结果在 FlowLog 中可追踪

---

## 2. 非功能需求

| 维度 | 要求 |
|------|------|
| 可靠性 | 关键事件（ToolResult / Error / RunnerCompletion / Checkpoint）必须 WBPF（先写后发），进程崩溃不丢失 |
| 可观测性 | 事件丢弃可观测（Prometheus `EventBusDropped` + FlowLog 打点） |
| 性能 | 高频事件（log / flow_log / text_delta / member_delta）不持久化、不阻塞主流程 |
| 兼容性 | 新增事件类型与字段不破坏现有前端解析（向后兼容） |
| 隔离性 | Monitor 高频事件（flow_log / log）与 Chat 业务事件走独立 Bus，避免相互挤压 |

---

## 3. 验收标准总览

1. 事件推流包含完整事件元数据（StateDelta / Extensions / FilterKey / Branch / Tag / Actions）
2. StateDelta 正确应用到 Session State（set / append / delete 三种操作）
3. 前端可按层级过滤事件流（FilterKey 前缀匹配）
4. 多 Agent 场景中可追踪执行链（Branch + InvocationID / ParentInvocationID）
5. 事件可携带自定义扩展元数据（Extensions 命名空间化）
6. Runner 正确处理 Actions 提示（SkipSummarization）
7. 系统重启后可查询历史事件
8. 可按时间范围回放事件
9. Chat 会话事件检视：Drawer/Dialog 双 Tab（Trace + 实时 Envelope），支持类型/分支/标签过滤与 Branch 树
10. 新工具注册后自动生成描述和 embedding（P2）
11. 工具更新后相关 Agent 缓存自动失效（P2）
12. 触发操作异步执行，不阻塞主流程（P2）
13. 触发结果在 FlowLog 中可追踪（P2）
