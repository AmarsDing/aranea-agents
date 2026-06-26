# 消息机制 — 需求规格

> 消息机制是 Aranea-Agents 全场景通信的底层基础设施，负责 Agent / Team / Graph / Monitor / Channel 等所有模块的事件产生、路由、传输与消费。本文档定义消息机制的产品需求。
>
> - **设计文档**：[51-message-mechanism.design.md](./51-message-mechanism.design.md)（架构设计、代码分层、Proto/API 契约、数据模型、接口定义、序列图、前端组件设计）
> - **开发计划**：[51-message-mechanism.development.md](./51-message-mechanism.development.md)（代码锚点、任务清单、Phase 划分、状态）
> - **架构变更依据**：ADR-02（Activity-First 持久化）+ ADR-03（统一总线架构）+ Chat 模块重构方案（均已归档，设计内容已并入本文档）

---

## 一、背景与目标

### 1.1 架构演进

消息机制经历三个阶段：

| 阶段 | 模型 | 状态 |
|------|------|------|
| v1（已废弃） | Envelope 信封模型（72 种 EnvelopeType + 3 Bus 并存：SessionBus/MonitorBus/ActivityBus） | 已删除 |
| v2（已废弃） | Envelope + Activity 双总线并存（WBPF 持久化） | 已删除 |
| **v3（现行）** | **单一 Activity 模型 + 2 Bus（ActivityEventBus + MonitorEventBus）+ 并行异步持久化** | ✅ |

**v3 设计原则（Activity-First，AF）**：后端将运行时事件投影为 Activity 语义单元，前端零推断消费。`activities` 表是唯一真相源；Envelope / ChatMessage / WAL / EventStore / EventBuffer 概念彻底废弃。

### 1.2 目标

| 目标 | 说明 |
|------|------|
| 单一 Activity 模型 | 所有 chat / system 业务的语义单元都表达为 Activity，前端按 kind + event 渲染，零推断 |
| 双 Bus 职责清晰 | ActivityEventBus 传输 chat+system 业务事件；MonitorEventBus 传输高频监控事件（log/flow_log/mcp/alert） |
| 持久化与推送解耦 | 持久化 fire-and-forget，推送同步执行；DB I/O 不阻塞 WS 推送 |
| 最终一致性 | 持久化失败通过重试 + 死信缓冲 + API Backfill 补偿 |
| 双向通信 | WebSocket 原生支持上行（cancel/enqueue/subscribe），无需额外 HTTP 端点 |
| 通道复用 | 一个 WebSocket 连接承载所有事件类型，多路复用 |
| 错误模型简化 | 删除 `ActivityKindError`，turn 失败统一用 `task.failed` 表达 |
| 可扩展 | 新场景（Graph 节点事件、A2A 消息、Artifact 通知）无需修改核心机制 |

---

## 二、范围定义

### 2.1 已实现

- 单一 Activity 模型：`activities` 表为唯一真相源；`messages` / `event_store` / `event_wal` 表已 DROP
- 10 种 ActivityKind（无 error kind）：task / thinking / action / reply / plan / confirm / notice / session / team_stage / graph_stage
- 7 种 ActivityEventType：created / streaming / updated / completed / failed / cancelled / child_created
- ActivityDomain 区分持久化：`chat` 持久化到 Activity 表；`system` 仅推送 WS，不持久化
- 2 Bus 架构：ActivityEventBus（biz.ActivityEvent）+ MonitorEventBus（contract.MonitorEvent）
- 并行异步持久化：persistChan（fire-and-forget）+ 同步 eventBus.Publish（per-activity FIFO）
- 重试预算：5 次（100/200/400/800/1600ms，总 3100ms），可通过 `done` 通道中断
- 死信环形缓冲：容量 512，FIFO 淘汰，activityID 去重，通过 `ListDeadLetterActivities(sessionID)` 暴露
- API Backfill：前端 WS 重连或显式 reload 时，通过 `listActivities(sessionId)` 拉取最新持久化状态
- WebSocket 统一传输：挂入 Kratos HTTP Server，2 pump（activityEventPump + monitorEventPump）
- 双向通信：cancel / user_message / enqueue_message / subscribe / enable_log 上行
- 心跳与断连检测：应用层 ping/pong（25s 间隔）+ 协议层 Ping/Pong（30s 间隔）
- 全局监控模式：`session_id=*` 连接可订阅所有 Session 的 Monitor 事件
- 服务端优雅关闭：`server_shutdown` 下行通知
- 删除的旧体系：Envelope / EnvelopeType / RouteChannel / WAL / WBPF / BlockUpTo（用于 Activity 事件） / EventProjector / EventStore / EventBuffer / SessionBus / MonitorBus（旧 Envelope 版）

### 2.2 未来扩展

- 消息引用/回复（Activity `parent_activity_id` + meta.quote 扩展）
- A2A 协议消息映射
- Artifact 通知事件
- 事件时间线可视化（前端）
- JWT 连接期间定期校验 token 有效性

---

## 三、用户场景

### 场景 1：Chat 对话流式交互

用户在 Chat 页面与 Agent 对话，实时看到思考流、工具调用、回复流，可随时取消生成。

```
1. 前端建立 WS 连接
2. 发送用户消息（上行 user_message）
3. 实时接收 ActivityEvent：
   - task.created（用户消息入栈）
   - thinking.created / thinking.streaming / thinking.completed（推理过程）
   - action.created / action.streaming / action.completed（工具调用）
   - reply.created / reply.streaming / reply.completed（Agent 回复）
   - task.completed（turn 完成）
4. 可随时发送 cancel 取消生成（task.cancelled）
5. 可发送 enqueue_message 中途插入消息（SteerableRunner）
```

### 场景 2：Team 多 Agent 协作

用户与 Team 对话，看到团队阶段变更、成员子任务折叠展开。

```
1. 前端连接 WS，订阅 team_stage 事件
2. 收到 team_stage.created（stage=assembled）— 团队已组建
3. 收到 team_stage.updated（stage=executing）— 进入执行
4. 收到 team_stage.child_created — 子 Activity 产生（成员任务）
5. 展开成员子 session，懒加载其 Activity 流（task/thinking/action/reply）
6. 收到 team_stage.completed（stage=completed）— 团队完成
7. 收到 team_stage.failed / team_stage.cancelled — 团队失败/取消
```

### 场景 3：Graph 工作流执行

用户运行 Graph 工作流，看到节点开始/结束、DAG 进度，支持 HITL 中断恢复。

```
1. 前端连接 WS，订阅 graph_stage 事件
2. 收到 graph_stage.created（stage=planned）— Graph 已规划
3. 收到 graph_stage.updated（meta.current_node=xxx）— 高亮当前执行节点
4. 收到 graph_stage.child_created — 子节点 Activity 产生
5. 收到 graph_stage.completed — 所有节点完成
6. 收到 graph_stage.failed（meta.error_node=xxx）— 高亮错误节点
7. HITL 中断时收到 task.completed（tag=interrupt），用户审批后发送 user_message 恢复
```

### 场景 4：Monitor 运维日志

运维人员动态订阅 Monitor 日志，按级别过滤，无需独立连接。

```
1. 前端发送 enable_log(enabled:true) 开启日志流
2. 收到 MonitorEvent（type=log / flow_log / mcp.* / alert.*）
3. 不需要时发送 enable_log(enabled:false) 关闭
4. 全局模式（session_id=*）可监控所有 Session
```

### 场景 5：断连恢复（API Backfill）

用户网络抖动导致 WS 断连，重连后通过 API 拉取最新状态。

```
1. WS 断连，前端自动重连（指数退避）
2. 重连成功后调用 ListActivities(sessionId) API
3. 服务端返回该 session 当前所有持久化的 Activity（最新快照）
4. 前端用 API 返回值补齐缺失状态（最终一致性兜底）
5. 切换回实时流，继续接收新的 ActivityEvent
```

### 场景 6：System 通知

非 chat 工作单元的系统事件（编排阶段通知、临时提示等）通过 ActivityEvent 推送但不持久化。

```
1. 后端产生 Domain=system 的 ActivityEvent（如 notice.created）
2. ActivityEventSequencer 跳过持久化（persist=false），仅 publish 到 WS
3. 前端作为通知处理（toast/notification），不加入时间线
4. reload 时不重现（未持久化）
```

---

## 四、产品需求

### 4.1 单一 Activity 模型

**需求**：所有 chat / system 业务事件统一为 Activity 语义单元，前端按 `kind` + `event` + `status` + `meta` 渲染。

| 字段类别 | 字段 | 说明 |
|---------|------|------|
| 主键 | id | Activity 唯一 ID |
| 分类 | kind / status | 10 种 kind + 9 种 status（pending/running/tool_running/tool_blocked/completed/failed/partial_failure/cancelled/interrupted） |
| 归属 | session_id / turn_id / parent_activity_id / spirit_session_id / team_id / dag_node_id | 树形/层级归属 |
| 时间 | timestamp / duration_ms / seq | ISO8601 起始时间 / 持续毫秒 / 全局发射序列号（前端稳定排序） |
| Token | prompt_tokens / completion_tokens | kind=task 根 Activity 的用量统计 |
| 内容 | content / reasoning | 文本/推理内容 |
| 工具 | tool_name / tool_category / tool_call_id / tool_arguments / tool_result / tool_duration_ms / tool_error_code | kind=action 专用 |
| 阶段 | stage / depends_on | kind=session/team_stage/graph_stage 专用 |
| Agent | agent_key / agent_name | 发言 Agent 标识 |
| 显示 | collapsed / label | 折叠状态/标签 |
| 元数据 | meta | Kind-specific 扩展（成员列表/DAG 节点/进度/token_usage/error_message 等） |

**验收标准**：所有 chat/system 业务事件统一为 Activity，前端按 `kind` 动态分发到对应 Block 组件

### 4.2 ActivityKind 枚举（10 种，无 error kind）

| Kind | 说明 | 典型事件 |
|------|------|---------|
| `task` | 用户消息 / 任务根 / turn 容器 | created / completed / failed / cancelled |
| `thinking` | 推理过程 | created / streaming / completed / failed |
| `action` | 工具调用 | created / streaming / updated / completed / failed / cancelled |
| `reply` | Agent 回复（含团队成员回复） | created / streaming / completed / failed |
| `plan` | 计划 | created / streaming / updated / completed |
| `confirm` | 确认 | created / completed / cancelled |
| `notice` | 通知（含编排阶段、用户反馈、system 通知） | created / updated |
| `session` | Session 生命周期 | created / updated / completed |
| `team_stage` | 团队阶段 | created / updated / completed / failed / cancelled / child_created |
| `graph_stage` | Graph 阶段 | created / updated / completed / failed / child_created |

**关键设计**：不保留 `ActivityKindError`。错误是其他 Activity 的终态，用 `event=failed` 表达（如工具失败 = `kind=action` + `event=failed`，团队失败 = `kind=team_stage` + `event=failed`），避免同一错误产生两个 Activity。

**已删除的 legacy kind**：`sub_task_board`（前端无 UI）、`delegate`（无调用方）、`error`（被 `task.failed` 替代）

**验收标准**：前端按 kind 动态选择渲染组件，所有模式（spirit/team/agent）使用同一渲染管线

### 4.3 ActivityEventType 枚举（7 种业务语义事件）

| Event | 业务含义 | 前端行为 |
|-------|---------|---------|
| `created` | 新 Activity 创建（思考/工具/回复/团队阶段等开始） | 新增对应 Block 组件 |
| `streaming` | 流式追加（思考流式、回复流式、工具参数流式） | 向现有 Block 追加文本，光标闪烁；meta.delta_field 标识追加字段（content/reasoning/tool_arguments） |
| `updated` | 状态变更（非流式；阶段变更、进度更新、成员列表变更） | 更新 Block 的状态/阶段/进度，不追加文本；meta.changed_fields 标识变更字段 |
| `completed` | 正常完成 | 停止光标，标记完成状态，可展开查看详情 |
| `failed` | 失败（独立事件，非 completed + status=failed） | 高亮显示错误，展示错误详情，可重试；meta.error_code + meta.error_message 标识错误 |
| `cancelled` | 取消（用户主动停止） | 标记为已取消，展示取消原因；meta.cancel_reason 标识原因 |
| `child_created` | 子 Activity 创建（父 Activity 的事件，通知前端在父 Block 下新增子 Block） | 在父 Block 下新增子 Block（折叠状态）；meta.child_activity_id 标识子 Activity |

**streaming vs updated 边界**（必须遵守）：

| 维度 | streaming | updated |
|------|-----------|---------|
| 变更类型 | 文本追加（content/reasoning/tool_arguments） | 非文本变更（status/stage/progress/成员列表） |
| 频率 | 高频（每 token） | 低频（阶段变更） |
| 前端行为 | 追加文本，光标闪烁 | 更新状态/进度，不追加文本 |
| 批量合并 | 是（16ms 窗口） | 否 |
| meta 字段 | `meta.delta_field` | `meta.changed_fields` |

**child_created 语义**：是**父 Activity 的事件**，通知前端在父 Block 下新增子 Block。子 Activity 有自己完整的生命周期（独立发送 `created`/`streaming`/`completed`/...），父子解耦，子 Activity 可独立查询和渲染。

**验收标准**：每种事件类型有明确的业务语义，前端按 event 类型决定渲染动作（新增/追加/更新/完成/失败/取消/子项新增）

### 4.4 ActivityDomain 字段（chat / system）

**需求**：通过 Domain 字段区分持久化策略

| Domain | 含义 | 持久化 | 前端处理 |
|--------|------|--------|---------|
| `chat` | Chat 工作单元（task/thinking/action/reply/plan/confirm/notice/session/team_stage/graph_stage） | ✅ 持久化到 Activity 表 | 加入时间线渲染 |
| `system` | 系统通知（非 chat 工作单元，如编排阶段提示、临时通知） | ❌ 跳过持久化 | 作为通知处理（toast/notification），不加入时间线 |

**验收标准**：Domain=system 事件不写入 DB，前端 reload 时不重现；Domain=chat 事件持久化并参与时间线渲染

### 4.5 双 Bus 架构

**需求**：两类事件走独立 Bus，职责清晰

| Bus | 传输类型 | 承载事件 | 持久化 |
|-----|---------|---------|--------|
| `ActivityEventBus` | `biz.ActivityEvent` | chat 业务事件（Domain=chat）+ system 通知事件（Domain=system） | chat 持久化，system 不持久化 |
| `MonitorEventBus` | `contract.MonitorEvent` | 高频监控事件（log / flow_log / mcp.* / alert.*） | 不持久化（FlowLog 持久化由独立 consumer 处理） |

**已删除的 legacy Bus**：SessionBus（传输 Envelope）、旧 Envelope-based MonitorBus、ActivityBus（v2 双总线期）

**验收标准**：业务事件与监控事件走独立 Bus，互不挤压；Monitor 高频事件不阻塞 chat 业务事件推送

### 4.6 并行异步持久化

**需求**：持久化与 WS 推送同时异步进行，互不阻塞

| 维度 | 说明 |
|------|------|
| 持久化 | fire-and-forget：投递到 persistChan（buffered channel），独立 worker goroutine 消费，FIFO 处理 |
| 推送 | 同步：调用 eventBus.Publish，保留 per-activity FIFO 顺序 |
| 阻塞 | consume 不等持久化，吞吐量由 WS 推送延迟（~5ms）决定，而非 DB I/O |
| channel 满兜底 | persistChan 满时回退到同步 persistWithRetry（极端场景兜底） |

**验收标准**：DB I/O 不阻塞 WS 推送；推送延迟 < 50ms（P99）；持久化失败不影响实时推送

### 4.7 持久化失败补偿（三重保障）

**需求**：持久化失败通过三重机制保证最终一致性

| 保障 | 说明 |
|------|------|
| 重试预算 | 5 次重试，指数退避（100/200/400/800/1600ms，总 3100ms），可通过 `done` 通道中断（Close 期间立即放弃重试转入死信） |
| 死信环形缓冲 | 重试耗尽后，失败的 Activity 进入 `deadLetter` 环形缓冲（容量 512，FIFO 淘汰）；同一 activityID 多次失败按最新快照去重；通过 `ListDeadLetterActivities(sessionID)` 暴露给 WS 重连补偿 |
| API Backfill | 前端在 WS 重连或显式 reload 时，通过 `listActivities(sessionId)` API 拉取最新持久化状态，作为最终一致兜底 |

**验收标准**：持久化失败不丢失数据（在死信缓冲容量内）；前端 reload 后状态与 DB 一致

### 4.8 OnError 语义（无 ActivityKindError）

**需求**：turn 失败统一用 `task.failed` 表达，不产生 parallel error Activity

| 场景 | 处理 |
|------|------|
| 存在 root task | 将 root task Activity 转换为 `status=failed`，错误信息存入 `Meta.error_message/error_type/error_code` |
| 无 root task | 创建一个最小化的 failed task Activity 兜底 |
| OnTurnEnd 终态保护 | 若 root 已是终态（Completed/Failed/Cancelled/Interrupted/PartialFailure），OnTurnEnd 不覆盖状态，仅附加 token usage |

**验收标准**：turn 失败时仅有一个 `task.failed` Activity，前端无需合并 parallel error kind

### 4.9 WebSocket 统一传输

**需求**：所有实时事件通过 WebSocket 统一传输，WS 为 Chat / Team / Graph / Monitor 主通道

| 能力 | 说明 |
|------|------|
| 连接端点 | `WS /v1/ws?session_id=xxx&token=jwt_token` |
| 挂载方式 | 通过 `RegisterOnKratos` 挂入 Kratos HTTP Server，不独立监听端口 |
| 认证方式 | 三级回退：URL `token` 参数 → `Authorization: Bearer` Header → `access_token` Cookie |
| 双向通信 | 下行推送 ActivityEvent / MonitorEvent，上行接收 user_message / cancel / enqueue_message / subscribe / enable_log |
| 多路复用 | 通过 `channel` 字段区分 chat / monitor / system（已删除 team/graph channel 概念，统一到 chat） |
| 心跳检测 | 客户端每 25s 发送应用层 ping，服务端回复 pong；协议层 Ping/Pong 30s 间隔 |
| 自动重连 | 指数退避（1s/2s/4s/8s/16s/30s cap） |
| 重连恢复 | 通过 `ListActivities` RPC 拉取最新状态（替代旧 EventBuffer replay） |
| 全局监控 | `session_id=*` 连接可订阅所有 Session 的 Monitor 事件（限 3 连接） |
| 优雅关闭 | 服务端 Stop 时广播 `server_shutdown` 下行通知 |

**WS 下行协议**：
- `activity_event?` — ActivityEvent JSON（chat + system 业务事件）
- `monitor_event?` — MonitorEvent JSON（log/flow_log/mcp/alert）
- ~~`envelope?`~~ — 已删除（旧 Envelope 通道）

**验收标准**：
- 一个 WS 连接承载所有事件类型
- 上行消息（cancel/enqueue/subscribe）无需额外 HTTP 端点
- 断连后可自动重连，通过 API 恢复状态
- Cookie 认证支持浏览器 WebSocket 原生连接
- 全局监控模式可跨 Session 订阅

### 4.10 Channel 多路复用

**需求**：一个 WS 连接通过 Channel 区分不同事件流

| Channel | 方向 | 说明 | 默认订阅 |
|---------|------|------|---------|
| `chat` | 双向 | ActivityEvent（chat + system 域） + 用户消息上行 | 连接即订阅 |
| `monitor` | 下行 | MonitorEvent（运维日志、系统事件） | 需 enable_log 开启 |
| `system` | 下行 | 系统通知（connected/pong/server_shutdown） | 连接即订阅 |

**已删除的 legacy channel**：`team` / `graph` / `knowledge`（统一到 chat，所有 Activity 事件走 chat channel）

**验收标准**：
- 不同 Channel 的事件互不干扰
- 客户端可动态 subscribe/unsubscribe Channel
- Channel 路由由服务端自动完成

### 4.11 上行消息

**需求**：客户端通过 WS 发送上行消息

| 上行类型 | 说明 |
|---------|------|
| `user_message` | 发送聊天消息（content + agent_key + team_id + options）；A2UI `userAction` 复用同一上行 |
| `cancel` | 停止当前生成 |
| `enqueue_message` | 中途插入消息（SteerableRunner） |
| `subscribe` | 动态订阅通道（含 filter_key） |
| `unsubscribe` | 取消订阅通道 |
| `enable_log` | 开启/关闭 Monitor 日志流 |
| `ping` | 心跳 |

**验收标准**：
- 所有上行消息通过 WS 发送，无需额外 HTTP 端点
- cancel 可立即停止当前生成，与 HTTP `POST /v1/chat/stop` 行为一致
- enable_log 可动态开启 Monitor 日志流

### 4.12 Monitor 日志统一接入

**需求**：Monitor 日志统一为 MonitorEvent 格式，通过 WS 推送

| 来源 | metadata.source / type | 说明 |
|------|------------------------|------|
| Runner 生命周期 | `team-runner` / `chat-native` | Agent 启动/完成/错误 |
| Tool 执行 | `tool` | 工具调用开始/结束/错误 |
| LLM 调用 | `llm` | 模型请求/响应/重试 |
| 系统事件 | `system` | 内存/连接/配置变更 |
| Intent Pass | `intent-pass` | 意图识别日志 |
| TraceEmitter / SysLog | `step_id` + 中文 title | 业务/系统 FlowLog → MonitorEvent |

**验收标准**：
- Monitor 日志不再需要独立端口
- 前端可通过 enable_log 动态开启日志流
- 日志与 Agent 事件在同一连接中传输（不同 channel 隔离）
- 全局模式（session_id=*）可监控所有 Session

### 4.13 内部消费者

**需求**：ActivityEventBus / MonitorEventBus 内部订阅者处理副作用

| 消费者 | 订阅 Bus | 职责 |
|--------|---------|------|
| ToolCallConsumer | ActivityEventBus | 工具调用记录到 ToolInvocation（action.completed/failed） |
| CallbackConsumer | ActivityEventBus | Webhook 回调（task.completed/failed/cancelled 终态） |
| UsageRollupConsumer | ActivityEventBus | Token 用量汇总（task.completed） |
| UserFeedbackConsumer | ActivityEventBus | 用户反馈处理（notice.created with meta.feedback） |
| FlowLogPersistConsumer | MonitorEventBus | FlowLog 持久化（type=flow_log） |

**已删除的 legacy consumer**：MessageStoreConsumer（messages 表已删除）、EventBusConsumer（拆分为上述 typed consumer）、eventBufferHandler / eventPersistHandler / stateDeltaHandler / runnerCompletionHandler（旧 EventBusConsumer 编排器）

**验收标准**：
- 各消费者独立运行，互不影响
- 消费者失败不影响事件路由和其他消费者

---

## 五、非功能需求

| 维度 | 要求 |
|------|------|
| 可靠性 | Activity 事件采用并行异步持久化 + 重试 + 死信 + API Backfill；订阅者需幂等（重放走 ListActivities API） |
| 可观测性 | 持久化失败率、WS 推送延迟、死信缓冲占用率可通过日志/指标观测 |
| 性能 | 高频事件（streaming/updated）异步持久化、失败丢弃；推送延迟 < 50ms（P99） |
| 兼容性 | 新增 ActivityKind / ActivityEventType 不破坏现有前端解析（向后兼容） |
| 隔离性 | Monitor 高频事件（log/flow_log）与 Chat 业务事件走独立 Bus，避免相互挤压 |

---

## 六、性能需求

| 指标 | 要求 |
|------|------|
| WS 最大连接数 | 默认 10000，可配置 |
| 每 Session 最大连接数 | 默认 5 |
| 全局监控最大连接数 | 默认 3 |
| ActivityEventBus 订阅者 buffer | 默认 128，最大 512 |
| WS 推送延迟 P99 | < 50ms（不受 DB I/O 阻塞） |
| Activity 持久化延迟 P99 | < 100ms |
| Activity 持久化失败率 | < 0.1% |
| 死信缓冲容量 | 512 条/Session（FIFO 淘汰） |
| 重试预算 | 5 次，总 3100ms（100/200/400/800/1600ms 指数退避） |
| 前端 backfill 触发率 | < 5%（WS 重连或 reload 时） |
| 单条 ActivityEvent 大小 | 最大 1MB |
| WS 写超时 | 10s |
| WS 读超时（无 pong） | 60s |
| 协议层心跳间隔 | 30s |
| 应用层心跳间隔 | 25s |

---

## 七、安全需求

| 风险 | 缓解措施 |
|------|---------|
| WS 跨域 | Origin 白名单校验（`OriginAllowed`：localhost + 环境变量 `KRATOS_HTTP_EXTRA_CORS_ORIGINS`） |
| 事件泄露 | ActivityEventBus 订阅时按 session_id 路由，WS 连接校验 token 归属 |
| WS 消息注入 | 上行消息类型白名单，payload 校验 |
| XSS via content | Activity content 做 HTML 转义后渲染（前端职责） |
| DDoS via 大量连接 | 限制每 Session 最大连接数（5）+ 全局监控连接数（3） |
| JWT 过期 | WS 连接期间定期校验 token 有效性（**未实现**） |
| 消息大小 | WS ReadLimit 1MB，超限断连 |
| WS 认证 | 三级回退认证（URL token → Authorization Header → Cookie） |

---

## 八、验收标准总览

1. ✅ 所有 chat/system 业务事件统一为 Activity（10 种 kind，无 error kind）
2. ✅ 7 种 ActivityEventType 有明确业务语义，前端按 event 决定渲染动作
3. ✅ ActivityDomain 区分持久化（chat 持久化，system 不持久化）
4. ✅ 双 Bus 架构（ActivityEventBus + MonitorEventBus），职责清晰
5. ✅ 一个 WS 连接承载所有事件类型（chat/monitor/system）
6. ✅ 上行消息（cancel/enqueue/subscribe/enable_log）通过 WS 发送，无需额外 HTTP
7. ✅ 断连后可自动重连，通过 ListActivities API 恢复状态
8. ✅ 持久化与推送并行异步，DB I/O 不阻塞 WS 推送（< 50ms P99）
9. ✅ 持久化失败三重保障（重试 + 死信 + API Backfill），最终一致
10. ✅ OnError 用 `task.failed` 表达，无 parallel error kind
11. ✅ Monitor 日志统一为 MonitorEvent，不再需要独立端口
12. ✅ 内部消费者独立运行，互不影响
13. ✅ 安全：Origin 校验、session_id 路由、消息大小限制、Cookie 认证支持
14. ✅ 应用层心跳：客户端 25s 间隔 ping，服务端 pong 回复
15. ✅ 全局监控模式：session_id=* 可跨 Session 订阅
16. ✅ 服务端优雅关闭：server_shutdown 通知
17. ✅ 旧体系（Envelope / WAL / WBPF / EventStore / EventBuffer / SessionBus / Envelope-based MonitorBus）已彻底删除
