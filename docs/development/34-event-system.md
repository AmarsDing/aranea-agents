# M18: Event 事件系统 — 详细需求

> 对标 trpc-agent-go `event` 包，完善项目的事件系统。
> **架构变更依据**：
> - ADR-02 Activity-First 事件持久化（已归档，设计内容已并入本文档）
> - ADR-03 统一总线架构（已归档，设计内容已并入本文档）
> - Chat 模块重构方案（已归档，设计内容已并入本文档）

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

### 1.7 Activity 事件持久化

**用户故事**：作为平台运维者，我希望系统重启后可查询历史 Activity 事件，以便进行问题排查和审计。

**功能规格**：
- `Domain=chat` 的 Activity 事件写入 `activities` 表（单一真相源，已替代 `messages` / `event_store` / `event_wal` 表）
- `Domain=system` 的 Activity 事件仅推送 WS，不持久化
- Monitor 事件（log / flow_log / mcp.* / alert.*）不持久化（FlowLog 持久化由独立 consumer 处理）
- 可按 session_id / turn_id / 时间范围 / kind 查询 Activity
- 支持存储膨胀控制（按 session 保留窗口、TTL 清理）

**验收标准**：系统重启后可查询历史 Activity 事件；`Domain=system` 事件不写入 DB

### 1.8 事件回放 API（API Backfill）

**用户故事**：作为前端开发者，我希望 WS 重连后能拉取最近的事件序列，以便恢复一致的会话视图。

**功能规格**：
- 提供 `ListActivities(sessionId)` RPC 拉取持久化 Activity 列表（替代旧的 WS EventBuffer replay 机制）
- WS 重连时前端主动调用 API Backfill 拉取断连期间丢失的事件
- 返回 Activity 列表，支持按 turn / 时间范围过滤

**验收标准**：WS 重连后通过 API Backfill 恢复一致视图，无需服务端维护 replay Buffer

### 1.9 Chat 会话事件检视

**用户故事**：作为平台使用者，我在 Chat 工作区查看当前会话的实时与历史 ActivityEvent，理解 Agent 执行路径与状态变更。

**功能规格**：
- 在 Chat 工作区内打开**会话级检视面板**（Drawer/Dialog，非第四列固定侧边栏）
- 双 Tab：**历史 Trace**（Session HTTP 时间线，从 Activity 表查询）与 **实时 ActivityEvent**（WS activity_event 下行 + `ListActivities` API Backfill）
- 实时 Tab：按 ActivityKind / ActivityEventType / Agent / 关键词过滤
- 展示 Activity 树形结构（parent_activity_id 嵌套）
- 展示 Activity 状态变更（streaming / updated / completed / failed / cancelled）
- 长时运行工具进度指示（kind=action + tool_category）

**与 Monitor 分工**：Monitor Events Tab 面向跨会话运维（消费 MonitorEvent）；Chat 检视面向**当前 session** 的调试与理解执行链（消费 ActivityEvent）。

**验收标准**：Chat 内可按 kind/event/agent 过滤会话事件流，Activity 树与状态指示器可用

### 1.10 工具生命周期事件与自动触发（BabyAGI 启发，P2）

> 来源：BabyAGI Triggers 机制（GitHub 22k+ stars），竞品分析差距 #8
> 对应需求：`docs/competitive-gap-requirements-2026-05-31.md` P2-10

**用户故事**：作为平台开发者，我希望工具/Agent 的变更能自动触发相关操作（如新工具注册后自动生成描述和 embedding），以便减少人工编排。

**背景**：BabyAGI 的 functionz 框架实现了 Triggers 机制——当函数被添加或更新时，自动触发相关函数执行（如自动生成描述和 embedding）。当前 Aranea 的事件系统由双 Bus 承担（`ActivityEventBus` 传输 chat+system 业务事件，`MonitorEventBus` 传输监控事件），工具/Agent 变更没有自动触发链。

**功能规格**：
- 增加 `ToolRegistered`/`ToolUpdated`/`ToolRemoved` 三种工具生命周期事件
  - 优先采用 `ActivityEvent`（`Domain=system`，仅 WS 推送不持久化）承载，便于前端实时感知工具变更
  - 触发结果（成功/失败）通过 `MonitorEvent`（`type=flow_log`）记录到 FlowLog
- `ToolRegistered` 事件触发 LLM 自动生成工具描述和 embedding（供 `tool_search` 语义检索）
- `ToolUpdated` 事件触发 `BuildTRPCAgentCached` 缓存失效
- `ToolRemoved` 事件触发依赖该工具的 Agent 配置告警
- 所有触发操作异步执行（遵守红线 #8：框架 plugin 回调不得直接写数据库）
- 触发结果（成功/失败）记录到 FlowLog

**验收标准**：
- 新工具注册后自动生成描述和 embedding
- 工具更新后相关 Agent 缓存自动失效
- 触发操作异步执行，不阻塞主流程
- 触发结果在 FlowLog 中可追踪

### 1.11 双 Bus 架构（ActivityEventBus + MonitorEventBus）

**用户故事**：作为平台开发者，我希望 chat 业务事件与高频监控事件走独立 Bus 传输，避免相互挤压，且前端能按统一类型模型消费。

**背景**：legacy 3 Bus 并存（SessionBus + MonitorBus + ActivityBus，均传输 Envelope）已废弃。新架构采用 2 Bus：`ActivityEventBus` 传输 `biz.ActivityEvent`（chat+system 业务事件），`MonitorEventBus` 传输 `contract.MonitorEvent`（log/flow_log/mcp.*/alert.*）。

**功能规格**：
- `ActivityEventBus`：传输 `ActivityEvent`（包含 `Event` / `Activity` / `Domain` 三字段），保留 per-activity FIFO 推送顺序
- `MonitorEventBus`：传输 `MonitorEvent`（高频、不持久化、best-effort 投递），DropPolicy + DropCount 可观测
- WS Server 运行 2 个 pump（activityEventPump + monitorEventPump），下行为 `activity_event?` + `monitor_event?` JSON
- 前端通过本地类型（ActivityEvent / MonitorEvent）解耦，按 Domain / kind / type 路由到对应 UI
- `Domain=system` 的 ActivityEvent 仅推送 WS 不持久化（前端作为通知处理）
- `Domain=chat` 的 ActivityEvent 持久化到 Activity 表并加入时间线渲染

**验收标准**：
- chat 业务事件与监控事件走独立 Bus，互不阻塞
- WS 单连接多路复用 2 类下行消息
- 前端可按 Domain / kind / type 区分处理

### 1.12 事件可靠性分级（基于 ActivityDomain + 持久化策略）

**用户故事**：作为平台运维者，我希望不同级别的事件有明确的可靠性保证，关键事件不丢失，高频事件不阻塞系统。

**功能规格**：
- **`Domain=chat` Activity 事件**：并行异步持久化（fire-and-forget）+ 三重补偿
  - 持久化与推送解耦：持久化失败不阻塞 WS 推送
  - 重试预算：5 次指数退避（100/200/400/800/1600ms，总 3100ms），可被 Close 中断
  - Dead-letter 环形缓冲：容量 512，FIFO 淘汰，activityID 去重
  - API Backfill：WS 重连或显式 reload 时通过 `ListActivities` RPC 拉取最终一致状态
- **`Domain=system` Activity 事件**：best-effort 推送（不持久化，丢失仅影响通知）
- **Monitor 事件**（log/flow_log/mcp.*/alert.*）：best-effort 推送（不持久化，丢失仅影响可观测性）
- **FlowLog 持久化**：由独立 `FlowLogPersistConsumer` 订阅 `MonitorEventBus` 异步落库（与事件投递解耦）
- **订阅者幂等**：所有 typed consumer 必须幂等（同一事件重复投递不产生副作用）

**验收标准**：
- `Domain=chat` 事件持久化失败不阻塞 WS 推送，且通过重试 + dead-letter + API Backfill 保证最终一致
- `Domain=system` 与 Monitor 事件丢失不影响业务状态
- 订阅者幂等，重复投递无副作用

---

## 2. 非功能需求

| 维度 | 要求 |
|------|------|
| 可靠性 | `Domain=chat` Activity 事件采用并行异步持久化（持久化与推送解耦），失败通过重试预算 + dead-letter 环形缓冲 + API Backfill 三重补偿保证最终一致；订阅者必须幂等 |
| 可观测性 | 事件丢弃可观测（Prometheus `EventBusDropped` + `MonitorBusDropCount` + FlowLog 打点）；dead-letter 通过 `ListDeadLetterActivities(sessionID)` 暴露 |
| 性能 | 持久化 fire-and-forget，WS 推送同步（~5ms/event），DB I/O 不阻塞推送；高频 Monitor 事件（log / flow_log）不持久化、不阻塞主流程 |
| 兼容性 | 新增 ActivityKind / ActivityEventType / MonitorEventType 不破坏现有前端解析（向后兼容） |
| 隔离性 | `ActivityEventBus`（chat+system 业务事件）与 `MonitorEventBus`（log/flow_log/mcp/alert）独立 Bus，避免相互挤压；WS 2 pump 独立运行 |
| 顺序性 | per-activity FIFO（同一 Activity 的事件按 created/streaming/updated/completed 顺序推送）；跨 Activity 不保证全局顺序 |

---

## 3. 验收标准总览

1. 事件推流包含完整事件元数据（StateDelta / Extensions / FilterKey / Branch / Tag / Actions）
2. StateDelta 正确应用到 Session State（set / append / delete 三种操作）
3. 前端可按层级过滤事件流（FilterKey 前缀匹配）
4. 多 Agent 场景中可追踪执行链（Branch + InvocationID / ParentInvocationID）
5. 事件可携带自定义扩展元数据（Extensions 命名空间化）
6. Runner 正确处理 Actions 提示（SkipSummarization）
7. 系统重启后可查询历史 Activity 事件（`Domain=chat` 持久化，`Domain=system` 不持久化）
8. WS 重连后通过 `ListActivities` RPC（API Backfill）恢复一致视图，无需服务端 replay Buffer
9. Chat 会话事件检视：Drawer/Dialog 双 Tab（Trace + 实时 ActivityEvent），支持 kind/event/agent 过滤与 Activity 树
10. 新工具注册后自动生成描述和 embedding（P2）
11. 工具更新后相关 Agent 缓存自动失效（P2）
12. 触发操作异步执行，不阻塞主流程（P2）
13. 触发结果在 FlowLog 中可追踪（P2）
14. 双 Bus 架构：`ActivityEventBus` 与 `MonitorEventBus` 独立运行，WS 2 pump 多路复用
15. `Domain=chat` 事件持久化失败不阻塞 WS 推送，通过重试 + dead-letter + API Backfill 保证最终一致
16. `Domain=system` 与 Monitor 事件丢失不影响业务状态
17. 订阅者幂等，重复投递无副作用
