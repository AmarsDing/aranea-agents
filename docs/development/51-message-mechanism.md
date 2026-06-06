# 消息机制 — 需求规格

> **2026-05-19 现状对齐**：§1.1 现状问题 1–8 项落地情况：
> - ✅ 1：事件投影逻辑已抽取到 `EventProjector`，事件循环简化为 `ConsumeEventStream`（投影+发布）。
> - ✅ 2：事件类型已统一为 Envelope type（Chat / Team / Monitor / Graph 共享 `EnvelopeType` 常量）。
> - ✅ 3：`EventBus`（`internal/event/bus.go`）已替代所有独立 Broker。
> - ✅ 4：`EventBuffer`（`internal/event/buffer.go`）环形缓冲区 + WS 重放同步屏障已实现。
> - ✅ 5：双向通信已实现——`cancel` / `user_message` / `enqueue_message` 均通过 WS 上行。
> - ✅ 6：背压策略已实现（`DropOldest` / `DropNewest` / `BlockUpTo` 三级策略 + `Reliable` 关键事件保障 + Prometheus 丢弃计数）。
> - ✅ 7：Monitor 流程日志为 `EnvelopeTypeFlowLog`（Flow Log v2），经 `TraceEmitter` / `SysLog*` 发布，走 WS `channel:monitor`（SlogBridge 已移除）。
> - ✅ 8：WS 单连接复用 Chat / Monitor / Team / Graph / System，`TeamRunEventBroker` / `MonitorLogBroker` / 独立 SSE 端口均已删除。
>
> 后续以 `guides/execution-plan.md` 附录 A "消息机制" 行为准；本文余下作为产品需求基线参考。

---

> 消息机制是 Aranea-Agents 全场景通信的底层基础设施，负责 Agent/Team/Graph/Monitor/Channel 等所有模块的事件产生、路由、传输与消费。本文档定义消息机制的产品需求，技术设计见 [51a 后端消息机制](./51a%20后端消息机制.md) 和 [51b 前端消息机制](./51b%20前端消息机制.md)。

---

## 一、背景与目标

### 1.1 原始问题与解决状态

| # | 问题 | 影响 | 状态 |
|---|------|------|------|
| 1 | 事件投影逻辑散落在 Service 层 | `trpc_turn.go` 中 200+ 行事件循环，混合事件解析、传输写入、消息落库、用量记录 | ✅ 已解决：`ConsumeEventStream` + `EventProjector.ProjectAndPublish` |
| 2 | 事件类型不统一 | Chat 用 `delta/done/tool.call`，Team 用 `TeamRunEvent.Type`，Monitor 用 `log` | ✅ 已解决：统一为 `EnvelopeType` 常量 |
| 3 | 无统一事件总线 | 各 Broker 独立实现，无法跨场景路由 | ✅ 已解决：`event.Bus` 接口 |
| 4 | 无事件持久化与重放 | 断连后无法恢复，前端刷新丢失中间状态 | ✅ 已解决：`event.Buffer` + WS replay 同步屏障 |
| 5 | 无双向通信 | 用户中断、运行中追加消息无法通过现有连接上行 | ✅ 已解决：WS 上行 `cancel` / `user_message` / `enqueue_message`（后者对接 `POST /v1/chat/enqueue` 与 `RunRegistry` steerable enqueue） |
| 6 | 背压缺失 | 慢客户端导致 channel 满后事件被丢弃 | ✅ 已解决：三级 DropPolicy + Reliable 关键事件保障 |
| 7 | Monitor 日志与 Agent 事件割裂 | 运维日志走独立端口，与 Agent 事件无法统一订阅和过滤 | ✅ 已解决：`flow_log` + EventBus + WS channel:monitor |
| 8 | 连接浪费 | Chat + Monitor + Team 需要多个独立连接 | ✅ 已解决：WS 单连接多路复用 |

### 1.2 目标

| 目标 | 说明 |
|------|------|
| 统一事件模型 | 所有通信（Chat/Team/Graph/Channel/Cron/A2A/Monitor）共享同一套事件定义与流转规则 |
| 框架对齐 | 事件模型以 trpc-agent-go `event.Event` 为真相源，项目层只做投影与扩展 |
| 分层清晰 | 事件产生在运行时层、路由在 Event 层、传输在 Server 层，各层职责不越界 |
| 双向通信 | WebSocket 原生支持上行（cancel/enqueue/subscribe），无需额外 HTTP 端点 |
| 通道复用 | 一个 WebSocket 连接承载所有事件类型（chat/monitor/team/graph/system），多路复用 |
| 可扩展 | 新场景（Graph 节点事件、A2A 消息、Artifact 通知）无需修改核心机制 |
| 可靠性 | 背压控制、心跳检测、事件缓冲与重放、自动重连 |
| 传输无关 | 同一 Envelope 可投射到 WebSocket / Webhook，传输层可替换 |

---

## 二、范围定义

### 2.1 已实现

- 统一信封模型（Envelope）：项目层统一的事件传输单元，含 **31 种** `EnvelopeType`（见 [51a §5.4](./51a%20后端消息机制.md#54-事件类型映射)）
- EventBus 统一事件总线：替代所有独立 Broker，统一事件路由，三级背压策略
- EventProjector 事件投影器：将 trpc `event.Event` 投影为 Envelope（Chat + Team 共用）
- WebSocket 统一传输：挂入 Kratos HTTP Server，支持双向通信与多路复用
- Channel 多路复用：一个 WS 连接通过 channel 字段区分 chat/monitor/team/graph/system
- 动态订阅：客户端通过 subscribe/unsubscribe/enable_log 控制订阅范围
- 心跳与断连检测：应用层 ping/pong（25s 间隔）+ 协议层 Ping/Pong（30s 间隔）
- 事件缓冲与重放：环形缓冲区（每 Session 200 条）+ replay 同步屏障
- 内部消费者：`EventBusConsumer` 编排 buffer / runner / state / persist 四 handler（StateDelta、用量、EventBuffer、event_store 持久化）
- Monitor 流程日志：Flow Log v2（`flow_log`）→ WS channel:monitor；进程 `log` 与 `flow_log` 前端分流
- 全局监控模式：`session_id=*` 连接可订阅所有 Session 的 Monitor/Team/Graph 事件
- 服务端优雅关闭：`server_shutdown` 下行通知

### 2.2 未来扩展

- Envelope 持久化到 SQLite（可选）
- Webhook 传输投射（同一 Envelope 投射到 Webhook）
- A2A 协议消息映射
- Artifact 通知事件
- 事件时间线可视化（前端）
- ToolCallConsumer / CallbackConsumer / MessageStoreConsumer 独立消费者
- JWT 连接期间定期校验 token 有效性

---

## 三、用户场景

### 场景 1：Chat 对话流式交互

用户在 Chat 页面与 Agent 对话，实时看到文本增量、工具调用、推理过程，可随时取消生成。

```
1. 前端建立 WS 连接
2. 发送用户消息（上行 user_message）
3. 实时接收 text_delta / tool_call / tool_result / text_done / runner_completion
4. 可随时发送 cancel 取消生成
5. 可发送 enqueue_message 中途插入消息（SteerableRunner）
```

### 场景 2：Team 多 Agent 协作

用户与 Team 对话，看到各成员 Agent 的消息流、Agent 转移动画，可按 filter_key 过滤特定 Agent。

```
1. 前端连接 WS，Team Session 自动订阅 channel:team
2. 收到 member_message_start / member_delta / member_message_done
3. 收到 team_run_started / team_step_finished / team_run_finished / team_run_failed
4. 收到 transfer 事件，显示 Agent 切换动画
5. 可通过 subscribe 动态过滤特定 Agent 的事件
```

### 场景 3：Graph 工作流执行

用户运行 Graph 工作流，看到节点开始/结束、检查点事件，支持 HITL 中断恢复。

```
1. 前端连接 WS，Graph Session 自动订阅 channel:graph
2. 收到 graph_node_start / graph_node_end / graph_node_error / graph_step / graph_execution_done / checkpoint
3. HITL 中断时收到 runner_completion(tag:"interrupt")
4. 用户审批后发送 user_message 恢复执行
```

### 场景 4：Monitor 运维日志

运维人员动态订阅 Monitor 日志，按级别过滤，无需独立连接。

```
1. 前端发送 enable_log(enabled:true) 开启日志流
2. 收到 log 类型 Envelope（含 metadata.level / metadata.source）
3. 不需要时发送 enable_log(enabled:false) 关闭
4. 全局模式（session_id=*）可监控所有 Session
```

### 场景 5：断连恢复

用户网络抖动导致 WS 断连，重连后自动恢复未收到的事件。

```
1. WS 断连，前端自动重连（指数退避）
2. 重连时携带 last_event_id
3. 服务端从 EventBuffer 重放断连期间的事件
4. 重放完成后切换到实时流（replayDone 同步屏障保证顺序）
```

---

## 四、产品需求

### 4.1 统一信封模型（Envelope）

**需求**：所有事件统一为 Envelope 格式传输

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| id | string | 是 | 事件唯一 ID（UUID） |
| type | string | 是 | 事件类型（见 4.2） |
| author | string | 是 | 事件作者（agent_name / user / system） |
| session_id | string | 是 | 所属 Session |
| team_id | string | 否 | 所属 Team（Team/Graph 场景） |
| request_id | string | 否 | 请求 ID |
| invocation_id | string | 否 | 调用 ID |
| parent_invocation_id | string | 否 | 父调用 ID |
| branch | string | 否 | 分支追踪 |
| filter_key | string | 否 | 层级过滤键 |
| tag | string | 否 | 业务标签（逗号分隔） |
| timestamp | string | 是 | ISO 8601 / RFC 3339 Nano 时间戳 |
| version | int | 是 | 版本号 |
| channel | string | 否 | 路由目标 channel（自动填充） |
| content | object | 否 | 文本/推理内容 |
| tool_call | object | 否 | 工具调用信息 |
| state_delta | object | 否 | 状态增量 |
| transfer | object | 否 | Agent 转移信息 |
| error | object | 否 | 错误信息 |
| usage | object | 否 | Token 用量 |
| extensions | map | 否 | 扩展元数据（trpc 框架扩展） |
| actions | object | 否 | 流控制提示 |
| trace | object | 否 | 执行追踪 |
| metadata | map | 否 | 自定义元数据 |

**验收标准**：所有场景的事件统一为 Envelope 格式，前端按 `type` 字段分发处理

### 4.2 事件类型定义

| Envelope type | 说明 | 典型 Channel |
|---------------|------|-------------|
| `text_delta` | 文本增量（流式） | chat |
| `text_done` | 文本完成 | chat |
| `tool_call` | 工具调用开始 | chat |
| `tool_result` | 工具返回结果 | chat |
| `state_delta` | 状态增量 | chat |
| `transfer` | Agent 转移 | chat / team |
| `runner_completion` | 运行完成 | chat |
| `error` | 错误 | chat |
| `log` | 进程/Gateway 文本日志（需 `enable_log`） | monitor |
| `flow_log` | 业务/系统流程日志（TraceEmitter / SysLog；**免** `enable_log`） | monitor |
| `graph_node_start` | Graph 节点开始 | graph |
| `graph_node_end` | Graph 节点结束 | graph |
| `graph_node_error` | Graph 节点错误 | graph |
| `graph_step` | Graph 步骤进度 | graph |
| `graph_execution_done` | Graph 执行完成 | graph |
| `graph_node_custom` | Graph 自定义节点事件 | graph |
| `checkpoint` | 检查点事件 | graph |
| `intent_pass` | 意图识别结果 | chat |
| `member_message_start` | Team 成员消息开始 | team |
| `member_delta` | Team 成员增量 | team |
| `member_message_done` | Team 成员消息完成 | team |
| `team_run_started` | Team 运行开始 | team |
| `team_run_finished` | Team 运行完成 | team |
| `team_run_failed` | Team 运行失败 | team |
| `team_step_started` | Team 步骤开始 | team |
| `team_step_finished` | Team 步骤完成 | team |
| `team_summary` | Team 结构化汇总 | team |
| `run_status` | 运行生命周期（含 Follow-up 入队通知） | chat / team |
| `knowledge_ingest` | 知识库入库进度 | knowledge |
| `mcp.session.reconnect` | MCP 重连通知 | monitor |
| `alert.notify` | 告警通知 | monitor |

#### `run_status` 与 Follow-up Queue（`message_queued`）

Agent/Team 运行中用户连续发送消息（Follow-up Queue，见 [1 chat.md §1.9](./1%20chat.md#19-对话阶段连续发送follow-up-queue--待发送队列)）时，入队成功会额外推送：

```json
{
  "type": "run_status",
  "metadata": {
    "status": "queued",
    "hint": "message_queued"
  }
}
```

| 字段 | 说明 |
|------|------|
| `metadata.status` | 固定为 `queued`（**不**覆盖当前 `running` 等主状态；前端应忽略对 `runStatus` 的覆盖） |
| `metadata.hint` | `message_queued` 表示 Follow-up 入队成功，应触发 `GET /v1/chat/pending` 刷新待发送列表 |

Steerable 直注与 Pending FIFO 降级均会发送此 Envelope。运行取消（HTTP `POST /v1/chat/stop` 与 WS 上行 `cancel`）统一推送 `metadata.status=cancelled` 的 `run_status`，并触发 Webhook `run.cancelled`（与 Gateway 出站回调对齐）。

**验收标准**：每种事件类型有明确的语义定义，前端可按类型分发到对应 UI 组件

### 4.3 WebSocket 统一传输

**需求**：所有实时事件通过 WebSocket 统一传输，WS 为 Chat / Team / Graph / Monitor 主通道；历史 Chat SSE 不再作为当前实现入口

| 能力 | 说明 |
|------|------|
| 连接端点 | `WS /v1/ws?session_id=xxx&token=jwt_token` |
| 挂载方式 | 通过 `RegisterOnKratos` 挂入 Kratos HTTP Server，不独立监听端口 |
| 认证方式 | 三级回退：URL `token` 参数 → `Authorization: Bearer` Header → `access_token` Cookie |
| 双向通信 | 下行推送 Envelope，上行接收 user_message / cancel / enqueue_message / enable_log |
| 多路复用 | 通过 `channel` 字段区分 chat / monitor / team / graph / system |
| 心跳检测 | 客户端每 25s 发送应用层 ping，服务端回复 pong；协议层 Ping/Pong 30s 间隔 |
| 自动重连 | 指数退避（1s/2s/4s/8s/16s/30s cap） |
| 事件重放 | 重连时携带 `last_event_id`，服务端从 EventBuffer 重放 |
| 重放顺序保证 | replayDone 同步屏障：重放期间 eventPump 阻塞，重放完成后才转发实时事件 |
| 全局监控 | `session_id=*` 连接可订阅所有 Session 的 Monitor/Team/Graph 事件（限 3 连接） |
| 优雅关闭 | 服务端 Stop 时广播 `server_shutdown` 下行通知 |

**验收标准**：
- 一个 WS 连接承载所有事件类型
- 上行消息（cancel/enqueue/subscribe）无需额外 HTTP 端点
- 断连后可自动重连并恢复事件
- 重放事件与实时事件不交错，保证顺序
- Cookie 认证支持浏览器 WebSocket 原生连接
- 全局监控模式可跨 Session 订阅

### 4.4 Channel 多路复用

**需求**：一个 WS 连接通过 Channel 区分不同事件流

| Channel | 方向 | 说明 | 默认订阅 |
|---------|------|------|---------|
| `chat` | 双向 | Chat 事件 + 用户消息上行 | 连接即订阅 |
| `monitor` | 下行 | 运维日志、系统事件 | 需 enable_log 开启 |
| `team` | 下行 | Team 运行事件 | 全局模式自动订阅 / 普通模式需 subscribe |
| `graph` | 下行 | Graph 工作流事件 | 全局模式自动订阅 / 普通模式需 subscribe |
| `system` | 下行 | 系统通知（connected/pong/server_shutdown） | 连接即订阅 |

**验收标准**：
- 不同 Channel 的事件互不干扰
- 客户端可动态 subscribe/unsubscribe Channel
- Channel 路由由服务端自动完成（Envelope 根据 type 和 session 类型分配）

### 4.5 上行消息

**需求**：客户端通过 WS 发送上行消息

| 上行类型 | 说明 |
|---------|------|
| `user_message` | 发送聊天消息（content + agent_key + team_id + options）；A2UI `userAction` 复用同一上行，正文为单行 `{"userAction":{...}}`（[39 planner.design.md](./39%20planner.design.md) §7.4）；Chat 用户气泡经 `a2uiUserActionDisplay` 展示摘要而非原始 JSON |
| `cancel` | 停止当前生成 |
| `enqueue_message` | 中途插入消息（SteerableRunner） |
| `subscribe` | 动态订阅通道（含 filter_key） |
| `unsubscribe` | 取消订阅通道 |
| `enable_log` | 开启/关闭 Monitor 日志流 |
| `ping` | 心跳 |

**验收标准**：
- 所有上行消息通过 WS 发送，无需额外 HTTP 端点
- cancel 可立即停止当前生成，并下发 `run_status`（`cancelled`），与 HTTP `POST /v1/chat/stop` 行为一致
- enable_log 可动态开启 Monitor 日志流

### 4.6 EventBus 统一事件总线

**需求**：替代现有独立 Broker，统一事件路由

| 能力 | 说明 |
|------|------|
| 单一发布入口 | 所有事件通过 `EventBus.Publish()` 发布 |
| 多订阅者路由 | 按 session_id / team_id / channel / filter_key / event_types / level_filter 路由 |
| 内部消费 | 状态持久化、用量记录、EventBuffer 追加等作为内部订阅者 |
| 背压控制 | 三级策略：DropOldest / DropNewest / BlockUpTo；Reliable 模式保障关键事件 |
| 关键事件保障 | tool_result / error / runner_completion / graph_node_end / team_run_finished/failed 自动升级为 BlockUpTo |
| FilterKey 匹配 | 对齐 trpc-agent-go 前缀匹配语义 |
| 可观测 | Prometheus 计数：`aranea_event_bus_published_total` / `aranea_event_bus_dropped_total` |

**验收标准**：
- 旧 `TeamRunEventBroker` 和 `MonitorLogBroker` 已删除，功能合并到 EventBus
- 新增订阅者无需修改核心代码
- 慢订阅者不影响其他订阅者

### 4.7 EventProjector 事件投影器

**需求**：将 trpc `event.Event` 投影为项目层 Envelope

| 职责 | 说明 |
|------|------|
| 类型映射 | trpc ObjectType → Envelope type（chat.completion.chunk → text_delta/text_done/tool_call 等） |
| 内容提取 | 从 `model.Response.Choices` 提取文本/推理/工具调用 |
| Team 事件构建 | BuildMemberMessageStart/Delta/DoneEnvelope、BuildLogEnvelope、BuildIntentPassEnvelope |
| 消息落库 | 用户/助手消息写入 Session（在 Service 层完成，非 Projector 职责） |

**验收标准**：
- 事件循环简化为 `ConsumeEventStream`（投影+发布）
- Chat 和 Team 场景共享 `EventProjector.ProjectAndPublish`
- Service 层不再直接管理事件投影逻辑

### 4.8 事件缓冲与重放

**需求**：支持断连后事件恢复

| 能力 | 说明 |
|------|------|
| 内存缓冲区 | 每 Session 保留最近 200 条 Envelope（环形缓冲区） |
| TTL 淘汰 | 30 分钟无访问的 Session 缓冲区自动清理 |
| 重放协议 | replay_start → 重放事件 → replay_end |
| 重连参数 | `last_event_id` 参数指定重放起点 |
| 缓冲淘汰 | 环形缓冲区，超出容量覆盖最旧事件 |
| 同步屏障 | eventPump 阻塞等待 replayDone 通道关闭，保证重放与实时事件不乱序 |

**验收标准**：
- 断连重连后可恢复未收到的事件
- 重放期间新事件缓冲，重放完成后发送
- 缓冲区满时丢弃最旧事件，不影响实时流
- 重放与实时流之间有 replayDone 同步屏障，保证事件不乱序

### 4.9 内部消费者

**需求**：EventBus 内部订阅者处理副作用

| 消费者 | 订阅条件 | 职责 | 状态 |
|--------|---------|------|------|
| EventBusConsumer | 全量（Reliable） | 编排：`eventBufferHandler` + `eventPersistHandler` + `runnerCompletionHandler` + `stateDeltaHandler` | ✅ 已实现（I5-SYS-03） |
| ToolCallConsumer | `tool_result`（终态） | 记录工具调用到 ToolInvocation（`tinv-{tool_call_id}` upsert） | ✅ |
| CallbackConsumer | `run_status` 终态 | 出站 Webhook（`WebhookDispatcher`） | ✅ |
| MessageStoreConsumer | `member_message_done`（Team） | 成员消息落库 `role=member`，不 bump model_call | ✅ |

**验收标准**：
- 各消费者独立运行，互不影响
- 消费者失败不影响事件路由和其他消费者

### 4.10 Monitor 日志统一接入

**需求**：Monitor 日志统一为 Envelope 格式，通过 WS 推送

| 来源 | metadata.source | 说明 |
|------|----------------|------|
| Runner 生命周期 | `team-runner` / `chat-native` | Agent 启动/完成/错误 |
| Tool 执行 | `tool` | 工具调用开始/结束/错误 |
| LLM 调用 | `llm` | 模型请求/响应/重试 |
| 系统事件 | `system` | 内存/连接/配置变更 |
| Intent Pass | `intent-pass` | 意图识别日志 |
| TraceEmitter / SysLog | `step_id` + 中文 title | 业务/系统 FlowLog → Envelope |

**验收标准**：
- Monitor 日志不再需要独立端口
- 前端可通过 enable_log 动态开启日志流
- 日志与 Agent 事件在同一连接中传输
- 全局模式（session_id=*）可监控所有 Session

---

## 五、向后兼容

### 5.1 旧事件名映射

| 旧事件名 | 新 Envelope type | 迁移策略 |
|---------|------------------|---------|
| `delta` | `text_delta` | 前端改为监听 Envelope，按 `type` 分发 |
| `done` | `text_done` + `runner_completion` | `done` 拆分为两个语义明确的事件 |
| `tool.call` | `tool_call` | 名称统一 |
| `user_message` | 保留为独立上行消息 | 用户消息回显不走 Envelope |
| `error` | `error` | 结构增强，保留 `message` 字段 |
| `state_delta` | `state_delta` | 从独立事件变为 Envelope type |
| `intent_pass` | `intent_pass` | 从独立事件变为 Envelope type |

### 5.2 迁移阶段

| 阶段 | 内容 | 状态 |
|------|------|------|
| 一 | EventBus + Envelope 引入，双写期 | ✅ 已完成 |
| 二 | 事件格式统一为 Envelope | ✅ 已完成 |
| 三 | WebSocket 统一传输上线 | ✅ 已完成 |
| 四 | 事件持久化与高级特性 | ✅ 已完成（EventBuffer + replay + 动态订阅） |

---

## 六、性能需求

| 指标 | 要求 | 实际值 |
|------|------|--------|
| WS 最大连接数 | 默认 10000，可配置 | 无硬限制（依赖系统资源） |
| 每 Session 最大连接数 | 默认 5 | 5（`maxSessionConns`） |
| 全局监控最大连接数 | 默认 3 | 3 |
| EventBus 订阅者 buffer | 默认 128，最大 512 | 128–512（可配置） |
| EventBuffer 每 Session | 默认 200 条 Envelope | 200 |
| EventBuffer TTL | 30 分钟 | 30 分钟 |
| 单条 Envelope 大小 | 最大 1MB | 1MB（WS ReadLimit） |
| WS 写超时 | 10s | 10s（`defaultWSWriteWait`） |
| WS 读超时（无 pong） | 60s | 60s（`defaultWSPongWait`） |
| 协议层心跳间隔 | 30s | 30s（`defaultWSPingPeriod`） |
| 应用层心跳间隔 | 25s | 25s（前端 `heartbeatInterval`） |

---

## 七、安全需求

| 风险 | 缓解措施 | 状态 |
|------|---------|------|
| WS 跨域 | Origin 白名单校验（`OriginAllowed`：localhost + 环境变量 `KRATOS_HTTP_EXTRA_CORS_ORIGINS`） | ✅ |
| 事件泄露 | EventBus 订阅时按 session_id 路由，WS 连接校验 token 归属 | ✅ |
| WS 消息注入 | 上行消息类型白名单，payload 校验 | ✅ |
| XSS via content | Envelope content 做 HTML 转义后渲染 | 前端职责 |
| DDoS via 大量连接 | 限制每 Session 最大连接数（5）+ 全局监控连接数（3） | ✅ |
| JWT 过期 | WS 连接期间定期校验 token 有效性 | ❌ 未实现 |
| 消息大小 | WS ReadLimit 1MB，超限断连 | ✅ |
| WS 认证 | 三级回退认证（URL token → Authorization Header → Cookie） | ✅ |

---

## 八、涉及文件

### 8.1 后端已实现

| 文件 | 说明 |
|------|------|
| `internal/event/bus.go` | Bus 接口与实现（三级背压策略、Reliable 关键事件保障、Prometheus 指标） |
| `internal/event/envelope.go` | Envelope 领域模型（31 种 EnvelopeType）、`RouteChannel`、MatchFilterKey、Clone、ContainsTag |
| `internal/event/buffer.go` | 事件内存缓冲区（环形缓冲区、TTL 淘汰、Replay） |
| `internal/event/trace_emitter.go` | Flow Log v2 + usage spans |
| `internal/event/system_flow.go` | 系统域 FlowLog（`SetGlobalBus`） |
| `internal/event/wire.go` | Wire ProviderSet（NewBus + NewBuffer） |
| `internal/server/ws.go` | WSServer（挂入 Kratos HTTP、三级认证、replay 同步屏障、全局监控模式、server_shutdown） |
| `internal/agent/event_projector.go` | EventProjector：trpc Event → Envelope 投影器 + Team/Log/Intent 辅助方法 |
| `internal/agent/turn_helpers.go` | ConsumeEventStream：事件循环简化为投影+发布 |
| `internal/biz/event_bus_consumer.go` | EventBusConsumer 编排器 |
| `internal/biz/event_bus_buffer_handler.go` | EventBuffer 追加 |
| `internal/biz/event_bus_runner_handler.go` | runner_completion → Usage / Monitor / Memory |
| `internal/biz/event_bus_state_handler.go` | state_delta 持久化 |
| `internal/biz/event_persist_handler.go` | event_store 异步持久化（可选） |
| `internal/biz/domain_event.go` | DomainEvent 领域模型（与 Envelope 双向转换） |
| `internal/biz/domain_event_adapter.go` | DomainEvent ↔ Envelope 适配器 |
| `internal/metrics/vars.go` | Prometheus 指标（EventBusPublished / EventBusDropped） |

### 8.2 后端修改

| 文件 | 修改 |
|------|------|
| `internal/service/trpc_turn.go` | 事件循环使用 `ConsumeEventStream`（投影+发布），移除内联 SSE 写入 |
| `internal/service/chat_native.go` | HTTP unary / WS 上行复用 native turn，主路径走 EventBus → WS |
| `internal/service/chat.go` | ChatServiceDeps 新增 EventBus，实现 RunCanceller 接口 |
| `internal/team/runner_team_trpc.go` | Team 事件循环使用 `ConsumeEventStream` + EventBus 发布 Team 生命周期事件 |
| `internal/team/runner_helpers.go` | publishTeamMonitor 传入 sessionID，日志事件可正确路由 |
| `internal/conf/conf.proto` | 新增 `Server.WS` 消息（enable / network / addr） |

### 8.3 后端已删除

| 文件 | 原操作 | 状态 |
|------|--------|------|
| `internal/biz/team_run_events.go` | 合并到 EventBus | ✅ 已删除 |
| `internal/biz/monitor_log_broker.go` | 合并到 EventBus | ✅ 已删除 |
| `internal/server/sse.go` | 独立 SSE 端口删除 | ✅ 已删除 |
| `internal/server/team_run_sse.go` | 合并到 ws.go | ✅ 已删除 |

### 8.4 前端已实现

| 文件 | 说明 |
|------|------|
| `web/src/features/chat/ws-transport.ts` | createWsTransport（应用层心跳、Cookie token 回退、pending 队列、server_shutdown 处理） |
| `web/src/features/chat/envelope.ts` | Envelope 类型定义（31 种，与后端 JSON 对齐） |
| `web/src/features/monitor/useLogStreamHub.ts` | Monitor Logs：`flow_log` / `log` 分流 |
| `web/src/features/chat/dispatcher.ts` | EnvelopeDispatcher 类（onType / onChannel / on 过滤 + matchFilterKey） |
| `web/src/features/chat/useEnvelopeStream.ts` | useEnvelopeStream + useChatStream + useTeamStream + useMonitorStream + useGraphStream |
| `web/src/features/chat/useEventFilter.ts` | 事件过滤辅助 |
| `web/src/config/runtime.ts` | buildWsUrl + readAccessTokenCookie + buildHealthWsUrl |

---

## 九、验收标准总览

1. ✅ 所有场景事件统一为 Envelope 格式，前端按 `type` 字段分发
2. ✅ 一个 WS 连接承载所有事件类型（chat/monitor/team/graph/system；`knowledge` 经 subscribe 或全局监控默认订阅）
3. ✅ 上行消息（cancel/enqueue/subscribe/enable_log）通过 WS 发送，无需额外 HTTP
4. ✅ 断连后可自动重连并恢复事件（EventBuffer 重放）
5. ✅ 重放与实时流有同步屏障，保证事件不乱序
6. ✅ 客户端可动态 subscribe/unsubscribe Channel
7. ✅ Monitor 日志统一为 Envelope，不再需要独立端口
8. ✅ 事件循环简化为投影+发布，副作用由 EventBusConsumer 处理
9. ✅ Chat 和 Team 场景均使用 EventProjector 投影流式事件
10. ✅ 旧 `TeamRunEventBroker` 和 `MonitorLogBroker` 已删除，功能合并到 EventBus
11. ✅ 背压控制：三级策略 + Reliable 关键事件保障
12. ✅ 安全：Origin 校验、session_id 路由、消息大小限制、Cookie 认证支持
13. ✅ 应用层心跳：客户端 25s 间隔 ping，服务端 pong 回复
14. ✅ 全局监控模式：session_id=* 可跨 Session 订阅
15. ✅ 服务端优雅关闭：server_shutdown 通知


---

## 子模块：后端消息机制

> 本文档设计 Aranea-Agents 后端的通信消息机制。核心决策：**WebSocket 统一传输**，挂入 Kratos HTTP Server。前端消息机制见 [51b 前端消息机制](./51b%20前端消息机制.md)。

---

## 一、设计目标

| 目标 | 说明 |
|------|------|
| 统一事件模型 | 所有通信（Chat/Team/Graph/Channel/Cron/A2A/Monitor）共享同一套事件定义与流转规则 |
| 框架对齐 | 事件模型以 trpc-agent-go `event.Event` 为真相源，项目层只做投影与扩展 |
| 分层清晰 | 事件产生在运行时层、路由在 Event 层、传输在 Server 层，各层职责不越界 |
| 双向通信 | WebSocket 原生支持上行（cancel/enqueue/subscribe），无需额外 HTTP 端点 |
| 通道复用 | 一个 WebSocket 连接承载所有事件类型（chat/monitor/team/graph/system），多路复用 |
| 可扩展 | 新场景（Graph 节点事件、A2A 消息、Artifact 通知）无需修改核心机制 |
| 可靠性 | 背压控制、心跳检测、事件缓冲与重放、自动重连 |
| 传输无关 | 同一 Envelope 可投射到 WebSocket / Webhook，传输层可替换 |

---

## 二、设计原则

```
原则 1：trpc-agent-go event.Event 是事件真相源，项目层不重新定义事件语义
原则 2：事件投影（Event → Envelope）是唯一需要项目层实现的逻辑
原则 3：事件路由由统一 EventBus 承担，Service 层不直接管理订阅
原则 4：传输协议（WS）与事件模型解耦，同一事件可投射到多种传输
原则 5：WebSocket 是 Chat / Team / Graph / Monitor 的主传输通道；历史 Chat SSE 不再作为当前实现入口
原则 6：一个 WS 连接通过 Channel 多路复用所有事件类型
原则 7：客户端通过 subscribe/unsubscribe/enable_log 动态控制订阅范围
```

---

## 三、现状复盘与问题

### 3.1 原有问题与解决状态

| # | 问题 | 状态 | 解决方案 |
|---|------|------|---------|
| 1 | 事件投影逻辑散落在 Service 层 | ✅ 已解决 | `ConsumeEventStream` + `EventProjector.ProjectAndPublish` |
| 2 | 事件类型不统一 | ✅ 已解决 | 统一为 `EnvelopeType` 常量（**31 种**，见 §5.4） |
| 3 | 无统一事件总线 | ✅ 已解决 | `event.Bus` 接口（`internal/event/bus.go`） |
| 4 | 无事件持久化与重放 | ✅ 已解决 | `event.Buffer` + WS replay 同步屏障 |
| 5 | 无双向通信 | ✅ 已解决 | WS 上行 cancel / user_message / enqueue_message |
| 6 | 背压缺失 | ✅ 已解决 | 三级 DropPolicy + Reliable 关键事件保障 |
| 7 | Monitor 日志与 Agent 事件割裂 | ✅ 已解决 | `flow_log` + Flow Log v2 + WS channel:monitor |
| 8 | 连接浪费 | ✅ 已解决 | WS 单连接多路复用 |

### 3.2 已删除的旧模块

| 旧模块 | 说明 |
|--------|------|
| `TeamRunEventBroker` | 合并到 EventBus，Team 过滤变为 SubscribeOptions |
| `MonitorLogBroker` + 独立端口 | 合并到 EventBus + Flow Log v2 |
| `streamWriter.Emit()` | 替换为 `EventBus.Publish()` |
| 独立 SSE Server（`:8001`） | 已删除，统一走 WS 传输 |
| `team_run_sse.go` | 合并到 ws.go |

---

## 四、事件流转全景图

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        事件产生层（运行时）                                   │
│                                                                             │
│  trpc Runner.Run()  →  <-chan *event.Event                                 │
│       │                                                                     │
│       │  事件类型：                                                          │
│       │  ├── chat.completion.chunk  (文本增量)                              │
│       │  ├── tool.call / tool.response  (工具调用/结果)                     │
│       │  ├── state.update  (状态增量)                                       │
│       │  ├── agent.transfer  (Agent 间转移)                                 │
│       │  ├── runner.completion  (运行完成)                                  │
│       │  ├── error  (错误)                                                  │
│       │  └── graph.node.* / checkpoint.*  (Graph/Checkpoint 事件)          │
│       ▼                                                                     │
├─────────────────────────────────────────────────────────────────────────────┤
│                        事件投影层（Agent 层）                                 │
│                                                                             │
│  EventProjector.ProjectAndPublish(event.Event, ProjectMeta)                │
│       │                                                                     │
│       │  职责：                                                              │
│       │  1. trpc Event → 统一 Envelope（含完整元数据）                      │
│       │  2. 自动填充 Channel（RouteChannel）                                │
│       │  3. 发布到 EventBus                                                 │
│       │                                                                     │
│       │  辅助方法（Team 场景）：                                             │
│       │  - BuildMemberMessageStartEnvelope                                  │
│       │  - BuildMemberDeltaEnvelope                                         │
│       │  - BuildMemberMessageDoneEnvelope                                   │
│       │  - BuildLogEnvelope                                                 │
│       │  - BuildIntentPassEnvelope                                          │
│       ▼                                                                     │
├─────────────────────────────────────────────────────────────────────────────┤
│                        事件路由层（Event 层 — EventBus）                     │
│                                                                             │
│  EventBus.Publish(Envelope)                                                │
│       │                                                                     │
│       ├──→ WSServer.eventPump    (WS 双向通道，按 session_id 路由)          │
│       │        ├── channel: chat      (Chat 事件)                          │
│       │        ├── channel: monitor   (Monitor 日志)                       │
│       │        ├── channel: team      (Team 事件)                          │
│       │        ├── channel: graph     (Graph 事件)                         │
│       │        └── channel: system    (系统通知)                           │
│       │                                                                     │
│       └──→ EventBusConsumer      (编排：Buffer/Persist/Runner/State 四 handler) │
│                                                                             │
├─────────────────────────────────────────────────────────────────────────────┤
│                        传输层（Server 层）                                    │
│                                                                             │
│  WS /v1/ws?session_id=xxx   ← 统一传输（双向、多路复用、挂入 Kratos HTTP） │
│  HTTP unary / WS 上行      ← POST /v1/chat/messages 或 WS user_message     │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 五、统一信封模型（Envelope）

### 5.1 设计思路

trpc-agent-go 的 `event.Event` 是运行时内部结构，包含 `*model.Response`、`StateDelta`、`Extensions` 等框架细节。直接暴露给传输层会：

1. 违反分层原则（传输层不应依赖框架内部类型）
2. 导致前端需要理解 trpc 框架的事件语义
3. 无法在不修改框架的情况下扩展业务字段

因此设计 **Envelope（信封）** 作为项目层统一的事件传输单元：

- **Envelope 是 event.Event 的投影**，不是替代
- **Envelope 包含完整元数据**，前端可独立消费
- **Envelope 是 JSON 友好的**，所有字段可直接序列化
- **一个 trpc Event 投影为一个 Envelope**，不再拆分为多个传输事件
- **Envelope 服务于 WS 与内部消费者**，传输层只负责编码方式

### 5.2 Envelope 结构

```go
// internal/event/envelope.go

type EnvelopeType string

const (
    EnvelopeTypeTextDelta          EnvelopeType = "text_delta"
    EnvelopeTypeTextDone           EnvelopeType = "text_done"
    EnvelopeTypeToolCall           EnvelopeType = "tool_call"
    EnvelopeTypeToolResult         EnvelopeType = "tool_result"
    EnvelopeTypeStateDelta         EnvelopeType = "state_delta"
    EnvelopeTypeTransfer           EnvelopeType = "transfer"
    EnvelopeTypeRunnerCompletion   EnvelopeType = "runner_completion"
    EnvelopeTypeError              EnvelopeType = "error"
    EnvelopeTypeLog                EnvelopeType = "log"
    EnvelopeTypeGraphNodeStart     EnvelopeType = "graph_node_start"
    EnvelopeTypeGraphNodeEnd       EnvelopeType = "graph_node_end"
    EnvelopeTypeGraphNodeError     EnvelopeType = "graph_node_error"
    EnvelopeTypeGraphStep          EnvelopeType = "graph_step"
    EnvelopeTypeGraphExecutionDone EnvelopeType = "graph_execution_done"
    EnvelopeTypeGraphCustom        EnvelopeType = "graph_node_custom"
    EnvelopeTypeCheckpoint         EnvelopeType = "checkpoint"
    EnvelopeTypeIntentPass         EnvelopeType = "intent_pass"
    EnvelopeTypeMemberMsgStart     EnvelopeType = "member_message_start"
    EnvelopeTypeMemberDelta        EnvelopeType = "member_delta"
    EnvelopeTypeMemberMsgDone      EnvelopeType = "member_message_done"
    EnvelopeTypeTeamRunStarted     EnvelopeType = "team_run_started"
    EnvelopeTypeTeamRunFinished    EnvelopeType = "team_run_finished"
    EnvelopeTypeTeamRunFailed      EnvelopeType = "team_run_failed"
    EnvelopeTypeTeamStepStarted    EnvelopeType = "team_step_started"
    EnvelopeTypeTeamStepFinished   EnvelopeType = "team_step_finished"
)

type Envelope struct {
    ID                 string       `json:"id"`
    Type               EnvelopeType `json:"type"`
    Author             string       `json:"author"`
    SessionID          string       `json:"session_id"`
    TeamID             string       `json:"team_id,omitempty"`
    RequestID          string       `json:"request_id,omitempty"`
    InvocationID       string       `json:"invocation_id,omitempty"`
    ParentInvocationID string       `json:"parent_invocation_id,omitempty"`
    Branch             string       `json:"branch,omitempty"`
    FilterKey          string       `json:"filter_key,omitempty"`
    Tag                string       `json:"tag,omitempty"`
    Timestamp          string       `json:"timestamp"`
    Version            int          `json:"version"`
    Channel            string       `json:"channel,omitempty"`

    Content    *EnvelopeContent    `json:"content,omitempty"`
    ToolCall   *EnvelopeToolCall   `json:"tool_call,omitempty"`
    StateDelta *EnvelopeStateDelta `json:"state_delta,omitempty"`
    Transfer   *EnvelopeTransfer   `json:"transfer,omitempty"`
    Error      *EnvelopeError      `json:"error,omitempty"`
    Usage      *EnvelopeUsage      `json:"usage,omitempty"`
    Extensions map[string]string   `json:"extensions,omitempty"`
    Actions    *EnvelopeActions    `json:"actions,omitempty"`
    Trace      *EnvelopeTrace      `json:"trace,omitempty"`
    Metadata   map[string]any      `json:"metadata,omitempty"`
}
```

### 5.3 辅助方法

```go
func NewEnvelope(typ EnvelopeType, author, sessionID string) Envelope
func (e Envelope) RouteChannel() string
func (e Envelope) MatchFilterKey(key string) bool
func (e Envelope) Clone() Envelope
func (e Envelope) ContainsTag(tag string) bool
```

### 5.4 事件类型映射

| Envelope type | trpc model.ObjectType | 说明 | WS channel |
|---------------|----------------------|------|-----------|
| `text_delta` | `chat.completion.chunk` (IsPartial) | 文本增量 | chat |
| `text_done` | `chat.completion.chunk` (!IsPartial) | 文本完成 | chat |
| `tool_call` | `chat.completion.chunk` (ToolCalls) | 工具调用开始 | chat |
| `tool_result` | `tool.response` | 工具返回结果 | chat |
| `state_delta` | `state.update` | 状态增量 | chat |
| `transfer` | `agent.transfer` | Agent 转移 | chat / team |
| `runner_completion` | `runner.completion` | 运行完成 | chat |
| `run_status` | — | 运行生命周期 / Follow-up 入队（`message_queued`） | chat / team |
| `error` | `error` | 错误 | chat |
| `log` | — | 进程/Gateway 文本日志（需 WS `enable_log`） | monitor |
| `flow_log` | — | 业务/系统流程日志（TraceEmitter / SysLog；**免** `enable_log`） | monitor |
| `team_summary` | — | Team 结构化汇总 | team |
| `knowledge_ingest` | — | 知识库入库进度 | knowledge |
| `mcp.session.reconnect` | — | MCP 会话重连通知 | monitor |
| `alert.notify` | — | 告警通知 | monitor |
| `graph_node_start` | `graph.node.start` | Graph 节点开始 | graph |
| `graph_node_end` | `graph.node.end` | Graph 节点结束 | graph |
| `graph_node_error` | `graph.node.error` | Graph 节点错误 | graph |
| `graph_step` | — | Graph 步骤进度 | graph |
| `graph_execution_done` | — | Graph 执行完成 | graph |
| `graph_node_custom` | — | Graph 自定义节点事件 | graph |
| `checkpoint` | `checkpoint.*` | 检查点事件 | graph |
| `intent_pass` | — | 意图识别结果 | chat |
| `member_message_start` | — | Team 成员消息开始 | team |
| `member_delta` | — | Team 成员增量 | team |
| `member_message_done` | — | Team 成员消息完成 | team |
| `team_run_started` | — | Team 运行开始 | team |
| `team_run_finished` | — | Team 运行完成 | team |
| `team_run_failed` | — | Team 运行失败 | team |
| `team_step_started` | — | Team 步骤开始 | team |
| `team_step_finished` | — | Team 步骤完成 | team |

**关键设计决策**：同一 Envelope 类型可路由到不同 Channel（如 `transfer` 在单 Agent 场景走 `chat`，在 Team 场景走 `team`），由 `RouteChannel()` 根据 TeamID 自动判断。

### 5.5 Channel 自动路由

```go
// internal/event/envelope.go — 包函数 RouteChannel(env Envelope)

func RouteChannel(env Envelope) string {
    switch env.Type {
    case EnvelopeTypeLog, EnvelopeTypeFlowLog,
         EnvelopeTypeMCPSessionReconnect, EnvelopeTypeAlertNotify:
        return "monitor"
    case EnvelopeTypeMemberMessageStart, /* … */ EnvelopeTypeTeamSummary:
        return "team"
    case EnvelopeTypeGraphNodeStart, /* … */ EnvelopeTypeGraphNodeCustom:
        return "graph"
    case EnvelopeTypeKnowledgeIngest:
        return "knowledge"
    default:
        if env.TeamID != "" {
            return "team"
        }
        return "chat"
    }
}
```

---

## 六、EventBus 统一事件总线

### 6.1 设计思路

EventBus 统一所有事件路由：

1. **单一发布入口**：所有事件通过 `EventBus.Publish()` 发布
2. **多订阅者路由**：按 session_id / team_id / channel / filter_key / event_types / level_filter 路由
3. **内部消费**：EventBuffer 追加、StateDelta 持久化、Usage 记录等作为内部订阅者
4. **背压控制**：三级 DropPolicy + Reliable 关键事件保障
5. **Channel 路由**：Envelope 根据 type 和 session 类型自动分配到对应 channel

### 6.2 Bus 接口

```go
// internal/event/bus.go

type Bus interface {
    Publish(ctx context.Context, envelope Envelope)
    Subscribe(opts SubscribeOptions) (<-chan Envelope, func())
    DropCount() uint64
}

type DropPolicy int

const (
    DropOldest DropPolicy = iota
    DropNewest
    BlockUpTo
)

type SubscribeOptions struct {
    SessionID   string
    TeamID      string
    Channel     string
    FilterKey   string
    EventTypes  []EnvelopeType
    LevelFilter string
    BufferSize  int
    DropPolicy  DropPolicy
    Reliable    bool
}
```

### 6.3 Bus 实现

```go
// internal/event/bus.go

type bus struct {
    mu          sync.RWMutex
    subscribers map[uint64]*subscriber
    nextID      uint64
    dropped     atomic.Uint64
}

type subscriber struct {
    ch   chan Envelope
    opts SubscribeOptions
}

func NewBus() Bus {
    return &bus{
        subscribers: make(map[uint64]*subscriber),
    }
}
```

**关键实现细节**：

1. **三级背压策略**：
   - `DropOldest`：channel 满时丢弃最旧事件（默认）
   - `DropNewest`：channel 满时丢弃最新事件
   - `BlockUpTo`：channel 满时阻塞发送者（带超时保护）

2. **Reliable 关键事件保障**：
   - 订阅者设置 `Reliable: true` 时，关键事件类型自动升级为 `BlockUpTo` 策略
   - 关键事件：`tool_result` / `error` / `runner_completion` / `graph_node_end` / `team_run_finished` / `team_run_failed`
   - 确保关键事件不丢失

3. **Prometheus 可观测**：
   - `aranea_event_bus_published_total`：发布事件总数
   - `aranea_event_bus_dropped_total`：丢弃事件总数

### 6.4 订阅者匹配

```go
func (b *bus) matchSubscriber(opts SubscribeOptions, env Envelope) bool {
    if opts.SessionID != "" && opts.SessionID != env.SessionID {
        return false
    }
    if opts.TeamID != "" && opts.TeamID != env.TeamID {
        return false
    }
    if opts.Channel != "" && opts.Channel != env.Channel {
        return false
    }
    if opts.FilterKey != "" && !env.MatchFilterKey(opts.FilterKey) {
        return false
    }
    if len(opts.EventTypes) > 0 {
        found := false
        for _, t := range opts.EventTypes {
            if t == env.Type {
                found = true
                break
            }
        }
        if !found {
            return false
        }
    }
    if opts.LevelFilter != "" && env.Type == EnvelopeTypeLog {
        level, _ := env.Metadata["level"].(string)
        if !matchLevelFilter(opts.LevelFilter, level) {
            return false
        }
    }
    return true
}
```

### 6.5 FilterKey 匹配规则

对齐 trpc-agent-go `event.Event.Filter()` 的前缀匹配语义：

```go
func (e Envelope) MatchFilterKey(key string) bool {
    if key == "" || e.FilterKey == "" {
        return true
    }
    subKey := key + "/"
    envKey := e.FilterKey + "/"
    return strings.HasPrefix(subKey, envKey) || strings.HasPrefix(envKey, subKey)
}
```

---

## 七、EventProjector 事件投影器

### 7.1 设计思路

EventProjector 是 trpc `event.Event` 到项目层 `Envelope` 的唯一转换点。它承担：

1. **类型映射**：trpc ObjectType → Envelope type
2. **内容提取**：从 `model.Response.Choices` 提取文本/推理/工具调用
3. **Team 事件构建**：BuildMemberMessageStart/Delta/DoneEnvelope、BuildLogEnvelope、BuildIntentPassEnvelope
4. **自动发布**：`ProjectAndPublish` 投影后直接发布到 EventBus

### 7.2 EventProjector 接口

```go
// internal/agent/event_projector.go

type EventProjector struct {
    bus Bus
}

type ProjectMeta struct {
    SessionID          string
    RequestID          string
    InvocationID       string
    ParentInvocationID string
    TeamID             string
    Branch             string
    FilterKey          string
}

func (p *EventProjector) ProjectAndPublish(ctx context.Context, ev *event.Event, meta ProjectMeta)
```

### 7.3 Team 辅助方法

```go
func (p *EventProjector) BuildMemberMessageStartEnvelope(author, sessionID, branch, filterKey string) Envelope
func (p *EventProjector) BuildMemberDeltaEnvelope(author, sessionID, text string) Envelope
func (p *EventProjector) BuildMemberMessageDoneEnvelope(author, sessionID, text string) Envelope
func (p *EventProjector) BuildLogEnvelope(author, sessionID, msg, level, source string) Envelope
func (p *EventProjector) BuildIntentPassEnvelope(author, sessionID string, data map[string]any) Envelope
```

### 7.4 事件循环重构

重构后的事件循环从 200+ 行简化为：

```go
// internal/agent/turn_helpers.go

func ConsumeEventStream(
    ctx context.Context,
    events <-chan *event.Event,
    bus event.Bus,
    meta event.ProjectMeta,
) error {
    projector := NewEventProjector(bus)
    for ev := range events {
        projector.ProjectAndPublish(ctx, ev, meta)
    }
    return nil
}
```

所有副作用（WS 写入、消息落库、用量记录）由 EventBus 的订阅者处理，事件循环本身只做投影+发布。

---

## 八、WebSocket Server 实现

### 8.1 Server 层注册

WSServer 通过 `RegisterOnKratos` 挂入 Kratos HTTP Server，不独立监听端口：

```go
// internal/server/ws.go

type WSServer struct {
    mu             sync.RWMutex
    conns          map[string][]*wsConn
    eventBus       event.Bus
    eventBuffer    *event.Buffer
    chatSvc        *service.ChatService
    upgrader       websocket.Upgrader
    globalConns    []*wsConn
    maxSessionConns int
    maxGlobalConns  int
}

func NewWSServer(eventBus event.Bus, buffer *event.Buffer, chatSvc *service.ChatService) *WSServer

func (s *WSServer) RegisterOnKratos(srv *httpm.Server) {
    srv.Route("/v1/ws").GET(s.handleWS)
}
```

### 8.2 WebSocket 连接管理

```go
type wsConn struct {
    conn          *websocket.Conn
    sessionID     string
    mu            sync.Mutex
    subscriptions map[string]*subscription
    cancel        context.CancelFunc
    eventCh       <-chan event.Envelope
    unsub         func()
    replayDone    chan struct{}
    logEnabled    bool
}

func (s *WSServer) handleWS(w http.ResponseWriter, r *http.Request) {
    token := extractToken(r)  // 三级回退：URL → Header → Cookie
    sessionID := r.URL.Query().Get("session_id")
    lastEventID := r.URL.Query().Get("last_event_id")

    if !s.authenticate(token, sessionID) {
        http.Error(w, "unauthorized", http.StatusUnauthorized)
        return
    }

    conn, err := s.upgrader.Upgrade(w, r, nil)
    if err != nil {
        return
    }

    ctx, cancel := context.WithCancel(r.Context())
    wc := &wsConn{
        conn:          conn,
        sessionID:     sessionID,
        subscriptions: make(map[string]*subscription),
        cancel:        cancel,
        logEnabled:    false,
    }

    s.registerConn(wc)
    defer s.unregisterConn(wc)

    s.sendConnected(wc)

    if lastEventID != "" && s.eventBuffer != nil {
        wc.replayDone = make(chan struct{})
        go func() {
            defer close(wc.replayDone)
            s.replayEvents(wc, sessionID, lastEventID)
        }()
    }

    go s.readPump(ctx, wc)
    s.eventPump(wc, eventCh)
}
```

### 8.3 认证三级回退

```go
func extractToken(r *http.Request) string {
    if t := r.URL.Query().Get("token"); t != "" {
        return t
    }
    if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
        return strings.TrimPrefix(h, "Bearer ")
    }
    if c, err := r.Cookie("access_token"); err == nil && c.Value != "" {
        return c.Value
    }
    return ""
}
```

**设计决策**：浏览器 WebSocket API 无法设置自定义 Header，Cookie 认证是浏览器场景的必要回退。

### 8.4 Origin 校验

复用 `cors_filter.go` 中的 `OriginAllowed` 函数：

```go
upgrader: websocket.Upgrader{
    CheckOrigin: func(r *http.Request) bool {
        return OriginAllowed(r.Header.Get("Origin"))
    },
}
```

白名单规则：localhost/127.0.0.1/[::1] 前缀 + 环境变量 `KRATOS_HTTP_EXTRA_CORS_ORIGINS`。

### 8.5 读泵（上行消息处理）

```go
func (s *WSServer) readPump(ctx context.Context, wc *wsConn) {
    defer wc.cancel()
    wc.conn.SetReadLimit(1 << 20)  // 1MB
    wc.conn.SetReadDeadline(time.Now().Add(defaultWSPongWait))
    wc.conn.SetPongHandler(func(string) error {
        wc.conn.SetReadDeadline(time.Now().Add(defaultWSPongWait))
        return nil
    })

    for {
        _, msg, err := wc.conn.ReadMessage()
        if err != nil {
            return
        }
        var up UpstreamMessage
        if json.Unmarshal(msg, &up) != nil {
            continue
        }
        switch up.Type {
        case "user_message":
            s.handleUserMessage(ctx, wc, up)
        case "cancel":
            s.handleCancel(ctx, wc, up)
        case "enqueue_message":
            s.handleEnqueueMessage(ctx, wc, up)
        case "subscribe":
            s.handleSubscribe(ctx, wc, up)
        case "unsubscribe":
            s.handleUnsubscribe(ctx, wc, up)
        case "enable_log":
            s.handleEnableLog(ctx, wc, up)
        case "ping":
            s.sendPong(wc)
        }
    }
}
```

### 8.6 写泵（下行消息推送）

```go
func (s *WSServer) eventPump(wc *wsConn, eventCh <-chan event.Envelope) {
    if wc.replayDone != nil {
        <-wc.replayDone    // 阻塞直到重放完成
    }

    ticker := time.NewTicker(defaultWSPingPeriod)
    defer ticker.Stop()

    for {
        select {
        case <-wc.cancel.Done():
            return
        case env, ok := <-eventCh:
            if !ok {
                return
            }
            wc.conn.SetWriteDeadline(time.Now().Add(defaultWSWriteWait))
            msg := DownstreamMessage{
                Direction: "server_to_client",
                Channel:   env.Channel,
                Envelope:  env,
            }
            data, _ := json.Marshal(msg)
            wc.mu.Lock()
            err := wc.conn.WriteMessage(websocket.TextMessage, data)
            wc.mu.Unlock()
            if err != nil {
                return
            }
        case <-ticker.C:
            wc.conn.SetWriteDeadline(time.Now().Add(defaultWSWriteWait))
            if err := wc.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
                return
            }
        }
    }
}
```

### 8.7 全局监控模式

`session_id=*` 连接可订阅所有 Session 的 Monitor/Team/Graph 事件：

```go
func (s *WSServer) registerConn(wc *wsConn) {
    if wc.sessionID == "*" {
        // 全局监控模式：不绑定特定 Session
        s.mu.Lock()
        if len(s.globalConns) >= s.maxGlobalConns {
            s.mu.Unlock()
            wc.conn.Close()
            return
        }
        s.globalConns = append(s.globalConns, wc)
        s.mu.Unlock()
        // 订阅所有 monitor/team/graph 事件
        ch, unsub := s.eventBus.Subscribe(event.SubscribeOptions{
            Channel:    "monitor",
            BufferSize: 128,
        })
        // ...
    }
}
```

### 8.8 服务端优雅关闭

```go
func (s *WSServer) Stop(ctx context.Context) error {
    // 广播 server_shutdown 到所有连接
    shutdownMsg := DownstreamMessage{
        Direction: "server_to_client",
        Channel:   "system",
        Type:      "server_shutdown",
    }
    // ... 遍历所有连接发送 shutdown 消息
    // ... 关闭所有连接
    return nil
}
```

### 8.9 上行消息处理

| 上行类型 | 处理方法 | 说明 |
|---------|---------|------|
| `user_message` | `handleUserMessage` | 调用 ChatService.RunNativeTurnUnary |
| `cancel` | `handleCancel` | 调用 ChatService.CancelRun |
| `enqueue_message` | `handleEnqueueMessage` | 调用 `ChatService.EnqueueUserMessage`（`POST /v1/chat/enqueue` 等价语义：steerable enqueue 或 pending 入队；无 active run 时返回 `enqueue_rejected` 错误 Envelope） |
| `subscribe` | `handleSubscribe` | 动态订阅通道（含 filter_key） |
| `unsubscribe` | `handleUnsubscribe` | 取消订阅通道 |
| `enable_log` | `handleEnableLog` | 开启/关闭 Monitor 日志流 |
| `ping` | `sendPong` | 心跳回复 |

### 8.10 动态订阅处理

```go
func (s *WSServer) handleSubscribe(ctx context.Context, wc *wsConn, up UpstreamMessage) {
    payload := up.Payload.(map[string]any)
    channel, _ := payload["channel"].(string)
    filterKey, _ := payload["filter_key"].(string)

    wc.subscriptions[channel] = &subscription{
        channel:   channel,
        filterKey: filterKey,
    }

    s.resubscribeEventBus(ctx, wc)
}

func (s *WSServer) resubscribeEventBus(ctx context.Context, wc *wsConn) {
    if wc.unsub != nil {
        wc.unsub()
    }

    opts := event.SubscribeOptions{
        SessionID:  wc.sessionID,
        BufferSize: 128,
    }

    ch, unsub := s.eventBus.Subscribe(opts)
    wc.eventCh = ch
    wc.unsub = unsub
}
```

### 8.11 配置

```protobuf
// internal/conf/conf.proto

message Server {
  message HTTP { ... }
  message GRPC { ... }

  message WS {
    bool enable = 1;
    string network = 2;
    string addr = 3;
  }

  HTTP http = 1;
  GRPC grpc = 2;
  WS ws = 4;
}
```

---

## 九、Monitor 日志统一接入

### 9.1 Flow Log v2（已替代 SlogBridge）

业务与系统可观测日志通过 **Flow Log v2** 发布为 `EnvelopeTypeFlowLog`（非全局 `slog` 桥接）：

| 组件 | 文件 | 说明 |
|------|------|------|
| Turn | `trace_emitter.go` | `NewTraceEmitterForRun` → `emitter.Log*` |
| 系统 | `system_flow.go` | `SetGlobalBus` + `SysLog*` / `SessionSysLog*` |
| 上下文 | `flow_context.go` | `CtxFlowLogWarn/Done/Error` |

`internal/event/slog_bridge.go` **已删除**（2026-05-20）。详见 [changelog](../changelog/2026-05-20-FlowLog-V2-SlogRemoval.md)。

<!-- 历史 SlogBridge 示意（已废弃）：

```go
type SlogBridge struct {
    bus       Bus
    sessionID string
    author    string
}

func (b *SlogBridge) Handle(ctx context.Context, r slog.Record) error {
    env := NewEnvelope(EnvelopeTypeLog, b.author, b.sessionID)
    env.Metadata = map[string]any{
        "level":  r.Level.String(),
        "source": r.Source().Function,
    }
    env.Content = &EnvelopeContent{Text: r.Message}
    b.bus.Publish(ctx, env)
    return nil
}
```
-->

### 9.2 Monitor 事件来源

| 来源 | metadata.source | 说明 |
|------|----------------|------|
| Runner 生命周期 | `team-runner` / `chat-native` | Agent 启动/完成/错误 |
| Tool 执行 | `tool` | 工具调用开始/结束/错误 |
| LLM 调用 | `llm` | 模型请求/响应/重试 |
| 系统事件 | `system` | 内存/连接/配置变更 |
| Intent Pass | `intent-pass` | 意图识别日志 |
| Flow Log v2 | `step_id` | `TraceEmitter` / `SysLog*` → `flow_log` |

**关键设计**：`sessionID` 参数必须传入，因为 EventBus 按 `session_id` 路由事件到 WS 客户端。空 `sessionID` 会导致日志事件无法送达任何订阅者。

---

## 十、内部消费者

### 10.1 EventBusConsumer（编排器，I5-SYS-03）

单一 **Reliable** 订阅；`handleEnvelope` 按职责委托 handler，避免单文件混合 Buffer / 持久化 / 用量 / 状态：

```go
// internal/biz/event_bus_consumer.go

type EventBusConsumer struct {
    eventBus event.Bus
    buffer   *eventBufferHandler      // event_bus_buffer_handler.go
    runner   *runnerCompletionHandler // event_bus_runner_handler.go
    state    *stateDeltaHandler       // event_bus_state_handler.go
    persist  *eventPersistHandler     // event_persist_handler.go（可选 EventStore）
}

func (c *EventBusConsumer) handleEnvelope(ctx context.Context, env event.Envelope) {
    c.buffer.Handle(env)
    if c.persist != nil {
        c.persist.Handle(ctx, env)
    }
    de := envelopeToDomainEvent(env)
    c.handleDomainEvent(ctx, *de) // runner_completion / state_delta
}
```

### 10.2 Handler 与消费者列表

| 组件 | 触发 | 职责 | 状态 |
|------|------|------|------|
| `eventBufferHandler` | 全量 | `event.Buffer.Append`（WS 重放） | ✅ |
| `eventPersistHandler` | 全量（异步队列） | `event_store` 持久化 | ✅（EventStore 启用时） |
| `runnerCompletionHandler` | `DomainEventRunnerCompletion` | Usage / Monitor / TurnMemory | ✅ |
| `stateDeltaHandler` | `DomainEventStateDelta` | Session state 合并写 | ✅ |
| ToolCallConsumer | `tool_result`（终态） | ToolInvocation 落库（upsert，`source=event_bus`） | ✅ P3 |
| CallbackConsumer | `run_status` 终态 | Webhook 回调 | ✅ P3 |
| MessageStoreConsumer | `member_message_done` | Team 成员 `role=member` + `options_json.team_member`（`chat.team_member/v1`） | ✅ P3 |

### 10.3 StateDelta 持久化

实现见 `internal/biz/event_bus_state_handler.go`（`set` / `append` / `delete` 合并 Session state）。

### 10.4 用量记录

实现见 `internal/biz/event_bus_runner_handler.go`（`runner_completion` → `UsageUsecase` + `metadata_json.spans`）。

---

## 十一、事件持久化与重放

### 11.1 内存缓冲区

```go
// internal/event/buffer.go

type Buffer struct {
    mu      sync.RWMutex
    buffers map[string]*ringBuffer
    maxSize int
    ttl     time.Duration
}

func NewBuffer(maxSizePerSession int) *Buffer {
    return &Buffer{
        buffers: make(map[string]*ringBuffer),
        maxSize: maxSizePerSession,
        ttl:     30 * time.Minute,
    }
}

func (b *Buffer) Append(sessionID string, env Envelope)
func (b *Buffer) Replay(sessionID string, afterEventID string) []Envelope
```

### 11.2 WS 重连重放

```
1. 客户端重连时携带 last_event_id
   WS /v1/ws?session_id=sess-uuid&last_event_id=evt-100
2. 服务端从 EventBuffer 查询并重放
3. 先发送 replay_start → 重放事件 → replay_end
4. 重放期间实时事件缓冲，重放完成后发送
```

### 11.3 重放同步屏障

重放与实时事件流之间需要同步，避免事件乱序：

```go
type wsConn struct {
    // ...
    replayDone chan struct{}   // 重放完成后关闭
}

func (s *WSServer) handleWS(...) {
    // ...
    if lastEventID != "" && s.eventBuffer != nil {
        wc.replayDone = make(chan struct{})
        go func() {
            defer close(wc.replayDone)
            s.replayEvents(wc, sessionID, lastEventID)
        }()
    }
    go s.eventPump(wc, eventCh)
}

func (s *WSServer) eventPump(wc *wsConn, eventCh <-chan Envelope) {
    if wc.replayDone != nil {
        <-wc.replayDone    // 阻塞直到重放完成
    }
    for env := range eventCh {
        // 正常转发实时事件
    }
}
```

**关键设计**：`eventPump` 在 `replayDone` 通道关闭前阻塞，确保重放事件全部发送完毕后才开始转发实时事件，避免两者交错。

---

## 十二、场景事件流

### 12.1 Chat 场景

```
用户消息
  → Runner.Run()
    → LLM 调用 → text_delta × N → text_done
    → Tool 调用 → tool_call → tool_result
    → text_delta × N → text_done
  → runner_completion
```

### 12.2 Team 场景

```
用户消息
  → Team Runner.Run()
    → team_run_started
    → Coordinator Agent
      → tool_call: transfer_to_agent(agent_b)
        → Agent B → member_message_start → member_delta × N → member_message_done
        → transfer back
      → team_step_finished
    → team_run_finished / team_run_failed
```

**Team 事件投影**：Team Runner 的事件循环同样使用 `ConsumeEventStream` + `EventProjector.ProjectAndPublish` 将 trpc 事件投影为 Envelope 并发布到 EventBus，与 Chat 场景共享同一套投影逻辑。Team 生命周期事件（team_run_started/finished/failed）由 Team Runner 直接构建并发布。

### 12.3 Graph 场景

```
用户消息
  → GraphAgent.Run()
    → graph_node_start (step_1) → text_delta → graph_node_end
    → graph_node_start (step_2) → tool_call → tool_result → graph_node_end
    → graph_step
    → graph_execution_done
    → checkpoint
```

### 12.4 Channel 场景

```
飞书 Webhook POST
  → ChannelIngress.HandleWebhook()
    → ChatService.RunNativeTurnUnary()
      → runner.Run() → EventProjector → EventBus
        → EventBusConsumer → 状态持久化/用量记录
```

---

## 十三、分层实现清单

### 13.1 Event 层

| 文件 | 说明 |
|------|------|
| `internal/event/bus.go` | Bus 接口与实现（三级背压策略、Reliable 关键事件保障、Prometheus 指标） |
| `internal/event/envelope.go` | Envelope 领域模型（31 种 EnvelopeType）、`RouteChannel`、MatchFilterKey、Clone、ContainsTag |
| `internal/event/buffer.go` | 事件内存缓冲区（环形缓冲区、TTL 淘汰、Replay） |
| `internal/event/trace_emitter.go` | Flow Log v2 |
| `internal/event/system_flow.go` | 系统域 FlowLog |
| `internal/event/wire.go` | Wire ProviderSet（NewBus + NewBuffer） |

### 13.2 Server 层

| 文件 | 说明 |
|------|------|
| `internal/server/ws.go` | WSServer（挂入 Kratos HTTP、三级认证、replay 同步屏障、全局监控模式、server_shutdown） |

### 13.3 Agent 层

| 文件 | 说明 |
|------|------|
| `internal/agent/event_projector.go` | EventProjector：trpc Event → Envelope 投影器 + Team/Log/Intent 辅助方法 |
| `internal/agent/turn_helpers.go` | ConsumeEventStream：事件循环简化为投影+发布 |

### 13.4 Biz 层

| 文件 | 说明 |
|------|------|
| `internal/biz/event_bus_consumer.go` | EventBusConsumer 编排器 |
| `internal/biz/event_bus_*_handler.go` | buffer / runner / state handler |
| `internal/biz/event_persist_handler.go` | event_store 异步持久化 |
| `internal/biz/domain_event.go` | DomainEvent 领域模型（与 Envelope 双向转换） |
| `internal/biz/domain_event_adapter.go` | DomainEvent ↔ Envelope 适配器 |

### 13.5 Service 层修改

| 文件 | 修改 |
|------|------|
| `internal/service/trpc_turn.go` | 事件循环使用 `ConsumeEventStream`（投影+发布），移除内联 SSE 写入 |
| `internal/service/chat_native.go` | HTTP unary / WS 上行复用 native turn，主路径走 EventBus → WS |
| `internal/service/chat.go` | ChatServiceDeps 新增 EventBus，实现 RunCanceller 接口 |

### 13.6 Team 层修改

| 文件 | 修改 |
|------|------|
| `internal/team/runner_team_trpc.go` | Team 事件循环使用 `ConsumeEventStream` + EventBus 发布 Team 生命周期事件 |
| `internal/team/runner_helpers.go` | publishTeamMonitor 传入 sessionID，日志事件可正确路由 |

### 13.7 Metrics

| 文件 | 说明 |
|------|------|
| `internal/metrics/vars.go` | Prometheus 指标（EventBusPublished / EventBusDropped） |

### 13.8 配置变更

| 文件 | 修改 |
|------|------|
| `internal/conf/conf.proto` | 新增 `Server.WS` 消息（enable / network / addr） |

---

## 十四、Wire 注入

```go
// internal/event/wire.go
var ProviderSet = wire.NewSet(
    NewBus,
    NewBuffer,
)

// internal/biz/biz.go
var ProviderSet = wire.NewSet(
    NewEventBusConsumer,
    // ... 其他 biz providers
)

// internal/server/server.go
var ProviderSet = wire.NewSet(
    NewHTTPServer,
    NewGRPCServer,
    NewWSServer,
)
```

---

## 十五、性能考量

### 15.1 背压策略

| 组件 | 策略 |
|------|------|
| EventBus → 慢订阅者 | 三级 DropPolicy（DropOldest / DropNewest / BlockUpTo） |
| Reliable 订阅者 | 关键事件自动升级为 BlockUpTo |
| WS 写入阻塞 | 设置写超时 10s，超时后断连触发重连 |
| WS 读超时 | 60s 无 pong → 断开 |
| Runner 事件通道满 | trpc 框架内部处理（`EmitEventTimeoutErr`） |

### 15.2 连接管理

| 组件 | 限制 |
|------|------|
| 每 Session 最大连接数 | 5（`maxSessionConns`） |
| 全局监控最大连接数 | 3（`maxGlobalConns`） |
| EventBus 订阅者 buffer | 默认 128，最大 512 |
| EventBuffer 每 Session | 200 条 Envelope |
| EventBuffer TTL | 30 分钟 |
| Envelope 大小 | 单条最大 1MB |

### 15.3 延迟优化

| 优化点 | 方法 |
|--------|------|
| WS 消息发送 | 无需 HTTP 解析，帧头仅 2-14 字节 |
| JSON 序列化 | 预分配 buffer，避免频繁分配 |
| EventBus 路由 | 读锁 + 无锁 channel 发送 |
| StateDelta 持久化 | 批量合并（同一 Session 的多个 Delta 合并为一次写） |

---

## 十六、安全考量

| 风险 | 缓解措施 | 状态 |
|------|---------|------|
| WS 跨域 | Origin 白名单校验（`OriginAllowed`：localhost + 环境变量） | ✅ |
| 事件泄露 | EventBus 订阅时按 session_id 路由，WS 连接校验 token 归属 | ✅ |
| WS 消息注入 | 上行消息类型白名单，payload 校验 | ✅ |
| XSS via content | Envelope content 做 HTML 转义后渲染 | 前端职责 |
| DDoS via 大量连接 | 限制每 Session 最大连接数（5）+ 全局监控连接数（3） | ✅ |
| JWT 过期 | WS 连接期间定期校验 token 有效性 | ❌ 未实现 |
| 消息大小 | WS ReadLimit 1MB，超限断连 | ✅ |
| WS 认证 | 三级回退认证（URL token → Authorization Header → Cookie） | ✅ |

---

## 十七、与现有模块的关系

### 17.1 替代关系

| 旧模块 | 新设计 | 状态 |
|--------|--------|------|
| `TeamRunEventBroker` | `EventBus` | ✅ 已删除 |
| `MonitorLogBroker` + 独立端口 | `EventBus` + Flow Log v2 + WS channel:monitor | ✅ 已删除 |
| `streamWriter.Emit()` | `EventBus.Publish()` | ✅ 已替换 |
| `trpc_turn.go` 事件循环 | `ConsumeEventStream` + `EventProjector` | ✅ 已重构 |
| 独立 SSE Server（`:8001`） | 删除 | ✅ 已删除 |
| `team_run_sse.go` | 合并到 ws.go | ✅ 已删除 |

### 17.2 不变部分

| 模块 | 说明 |
|------|------|
| `Runner.Run()` | 框架 API 不变，返回 `<-chan *event.Event` |
| `BuildTRPCLLMAgent` | Agent 构建不变 |
| `NewTRPCRunner` | Runner 创建不变 |
| `ChatService` HTTP 入口 | POST /v1/chat/messages 保留为非流式 / 后台入口 |
| Proto API | 不修改 proto，WS 不走 proto |


---

## 子模块：前端消息机制

> 本文档设计 Aranea-Agents 前端的通信消息机制，聚焦传输协议、客户端实现和场景适配。后端消息机制见 [51a 后端消息机制](./51a%20后端消息机制.md)。

---

## 一、传输层选型

### 1.1 决策

```
WebSocket = 主传输通道（双向通信、多路复用、低延迟）
Chat HTTP = 非流式 / 后台入口（HTTP POST /v1/chat/messages）
```

WebSocket 在实时交互维度优于 HTTP unary：

| 维度 | 说明 |
|------|------|
| **方向性** | 双向通信，cancel/enqueue/subscribe 无需额外 HTTP |
| **连接数** | 1 个 WS 连接复用所有通道（Chat+Monitor+Team+Graph） |
| **浏览器限制** | 无硬限制，多 Session 不受连接数约束 |
| **协议开销** | 握手后仅 2-14 字节帧头，高频事件场景带宽节省显著 |
| **多路复用** | Channel 机制天然支持 monitor/team/chat/graph 统一连接 |

---

## 二、WebSocket 协议

### 2.1 连接建立

```
WS /v1/ws?session_id=sess-uuid&token=jwt_token

认证方式（三级回退）：
1. URL token 参数（优先）
2. Authorization: Bearer Header（浏览器 WebSocket API 不支持，仅非浏览器客户端）
3. access_token Cookie（浏览器场景的必要回退，浏览器 WebSocket API 无法设置自定义 Header）

前端连接时自动从 Cookie 读取 token：
  buildWsUrl({ sessionId, lastEventId, token: readAccessTokenCookie() })

握手时校验：
1. JWT token 有效（从 URL / Header / Cookie 三处提取）
2. session_id 存在且用户有权限
3. Origin 白名单校验

成功后服务端发送：
{
  "direction": "server_to_client",
  "channel": "system",
  "type": "connected",
  "payload": {
    "session_id": "sess-uuid",
    "server_time": "2026-01-01T00:00:00.000Z",
    "subscribed_channels": ["chat", "system"],
    "last_event_id": "evt-100"
  }
}
```

### 2.2 下行消息格式（Server → Client）

所有下行消息统一为 Envelope，通过 `channel` 字段多路复用：

```json
{
  "direction": "server_to_client",
  "channel": "chat | monitor | team | graph | system",
  "envelope": {
    "id": "evt-uuid",
    "type": "text_delta | tool_call | ...",
    "author": "agent_name",
    "session_id": "sess-uuid",
    "team_id": "team-uuid",
    "request_id": "req-uuid",
    "invocation_id": "inv-uuid",
    "parent_invocation_id": "parent-inv-uuid",
    "branch": "agent_a/agent_b",
    "filter_key": "agent_a/agent_b",
    "tag": "code_execution_code;transfer",
    "timestamp": "2026-01-01T00:00:00.000Z",
    "version": 1,
    "channel": "chat",
    "content": { "text": "...", "reasoning": "...", "is_partial": true },
    "tool_call": { "..." },
    "state_delta": { "..." },
    "transfer": { "..." },
    "error": { "..." },
    "usage": { "..." },
    "extensions": { "..." },
    "actions": { "..." },
    "trace": { "..." },
    "metadata": {}
  }
}
```

**系统消息**（非 Envelope）：

```json
{
  "direction": "server_to_client",
  "channel": "system",
  "type": "connected | pong | server_shutdown | replay_start | replay_end",
  "payload": { ... }
}
```

### 2.3 上行消息格式（Client → Server）

```json
{
  "direction": "client_to_server",
  "channel": "chat | control",
  "type": "user_message | cancel | enqueue_message | subscribe | unsubscribe | enable_log | ping",
  "request_id": "req-uuid",
  "payload": {}
}
```

### 2.4 上行消息类型

#### user_message — 发送聊天消息

```json
{
  "direction": "client_to_server",
  "channel": "chat",
  "type": "user_message",
  "request_id": "req-uuid",
  "payload": {
    "content": "帮我分析一下这段代码",
    "agent_key": "default",
    "team_id": "",
    "options": {}
  }
}
```

#### cancel — 停止生成

```json
{
  "direction": "client_to_server",
  "channel": "chat",
  "type": "cancel",
  "request_id": "req-uuid",
  "payload": {}
}
```

#### enqueue_message — 中途插入消息（SteerableRunner）

```json
{
  "direction": "client_to_server",
  "channel": "chat",
  "type": "enqueue_message",
  "request_id": "req-uuid",
  "payload": {
    "content": "请同时考虑性能优化"
  }
}
```

#### subscribe — 动态订阅通道

```json
{
  "direction": "client_to_server",
  "channel": "control",
  "type": "subscribe",
  "payload": {
    "channel": "team",
    "filter_key": "coordinator/agent_b"
  }
}
```

#### unsubscribe — 取消订阅通道

```json
{
  "direction": "client_to_server",
  "channel": "control",
  "type": "unsubscribe",
  "payload": {
    "channel": "team"
  }
}
```

#### enable_log — 开启/关闭 Monitor 日志流

```json
{
  "direction": "client_to_server",
  "channel": "control",
  "type": "enable_log",
  "payload": {
    "enabled": true
  }
}
```

#### ping — 心跳

```json
{
  "direction": "client_to_server",
  "channel": "control",
  "type": "ping",
  "payload": {}
}
```

服务端回复：

```json
{
  "direction": "server_to_client",
  "channel": "system",
  "type": "pong",
  "payload": {
    "server_time": "2026-01-01T00:00:00.000Z"
  }
}
```

### 2.5 Channel 定义

| Channel | 方向 | 说明 | 默认订阅 |
|---------|------|------|---------|
| `chat` | 双向 | Chat 事件 + 用户消息上行 | ✅ 连接即订阅 |
| `monitor` | 下行 | 运维日志、系统事件 | ❌ 需 enable_log 开启 |
| `team` | 下行 | Team 运行事件 | ✅ 全局模式自动订阅 / 普通模式需 subscribe |
| `graph` | 下行 | Graph 工作流事件 | ✅ 全局模式自动订阅 / 普通模式需 subscribe |
| `system` | 下行 | 系统通知（connected/pong/server_shutdown） | ✅ 连接即订阅 |

### 2.6 心跳与断连检测

```
应用层心跳：
  客户端 → 服务端：每 25s 发送应用层 ping
  服务端 → 客户端：回复 pong（含 server_time）

协议层心跳：
  服务端 → 客户端：每 30s 发送 WebSocket Ping 帧
  客户端 → 服务端：自动回复 Pong 帧

客户端检测：
  - 60s 无 pong → 认为连接断开，触发重连
  - 重连策略：指数退避（1s/2s/4s/8s/16s/30s cap）
  - 重连时携带 last_event_id，服务端从 EventBuffer 重放

服务端关闭通知：
  - 服务端优雅关闭时发送 server_shutdown 系统消息
  - 客户端收到后不再自动重连
```

### 2.7 重连与事件重放

```
1. 客户端断连后发起重连
2. WS 握手携带 last_event_id 参数：
   WS /v1/ws?session_id=sess-uuid&last_event_id=evt-100
3. 服务端从 EventBuffer 查询 evt-100 之后的事件
4. 先发送重放事件（channel 不变），再切换到实时流

服务端同步屏障：
  重放期间 eventPump 阻塞等待 replayDone 通道关闭，
  确保重放事件全部发送完毕后才开始转发实时事件，避免两者交错。

重放消息标记：
{
  "direction": "server_to_client",
  "channel": "chat",
  "type": "replay_start",
  "payload": { "count": 15, "from_id": "evt-101", "to_id": "evt-115" }
}
{
  "direction": "server_to_client",
  "channel": "chat",
  "envelope": { ... }  // 重放事件
}
{
  "direction": "server_to_client",
  "channel": "chat",
  "type": "replay_end",
  "payload": { "last_event_id": "evt-115" }
}
```

---

## 三、Envelope 类型定义

### 3.1 TypeScript 类型

```typescript
export interface Envelope {
  id: string;
  type: EnvelopeType;
  author: string;
  session_id: string;
  team_id?: string;
  request_id?: string;
  invocation_id?: string;
  parent_invocation_id?: string;
  branch?: string;
  filter_key?: string;
  tag?: string;
  timestamp: string;
  version: number;
  channel?: string;
  content?: EnvelopeContent;
  tool_call?: EnvelopeToolCall;
  state_delta?: EnvelopeStateDelta;
  transfer?: EnvelopeTransfer;
  error?: EnvelopeError;
  usage?: EnvelopeUsage;
  extensions?: Record<string, string>;
  actions?: EnvelopeActions;
  trace?: EnvelopeTrace;
  metadata?: Record<string, unknown>;
}

export type EnvelopeType =
  | "text_delta"
  | "text_done"
  | "tool_call"
  | "tool_result"
  | "state_delta"
  | "transfer"
  | "runner_completion"
  | "error"
  | "log"
  | "graph_node_start"
  | "graph_node_end"
  | "graph_node_error"
  | "graph_step"
  | "graph_execution_done"
  | "graph_node_custom"
  | "checkpoint"
  | "intent_pass"
  | "member_message_start"
  | "member_delta"
  | "member_message_done"
  | "team_run_started"
  | "team_run_finished"
  | "team_run_failed"
  | "team_step_started"
  | "team_step_finished";

export interface EnvelopeContent {
  text: string;
  reasoning?: string;
  is_partial: boolean;
}

export interface EnvelopeToolCall {
  id: string;
  name: string;
  arguments_json: string;
  result_json: string | null;
  status: "calling" | "running" | "success" | "error";
  duration_ms: number;
  is_long_running: boolean;
}

export interface EnvelopeStateDelta {
  operation: "set" | "append" | "delete";
  path: string;
  value_json: string;
}

export interface EnvelopeTransfer {
  from_agent: string;
  to_agent: string;
}

export interface EnvelopeError {
  type: "run_error" | "stream_error" | "tool_error";
  message: string;
  pending_id: string;
}

export interface EnvelopeUsage {
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
}

export interface EnvelopeActions {
  skip_summarization: boolean;
}

export interface EnvelopeTrace {
  agent_name: string;
  invocation_id: string;
  step_count: number;
  duration_ms?: number;
}

export interface LogMetadata {
  level: "DEBUG" | "INFO" | "WARN" | "ERROR";
  source: string;
  component?: string;
}
```

### 3.2 WS 消息类型

```typescript
export interface WsDownstream {
  direction: "server_to_client";
  channel: string;
  type?: string;
  payload?: Record<string, unknown>;
  envelope?: Envelope;
}

export interface WsUpstream {
  direction: "client_to_server";
  channel: string;
  type: string;
  request_id?: string;
  payload?: Record<string, unknown>;
}
```

### 3.3 事件类型语义

| EnvelopeType | 语义 | 典型处理 |
|-------------|------|---------|
| `text_delta` | 文本增量（流式） | 追加到消息气泡 |
| `text_done` | 文本完成 | 标记消息完成，停止 loading |
| `tool_call` | 工具调用开始 | 显示工具调用卡片（loading 状态） |
| `tool_result` | 工具返回结果 | 更新工具调用卡片（结果/错误） |
| `state_delta` | 状态增量 | 更新 Session 状态（UI 可能不展示） |
| `transfer` | Agent 转移 | 显示 Agent 切换动画 |
| `runner_completion` | 运行完成 | 停止 loading，显示用量 |
| `error` | 错误 | 显示错误提示 |
| `log` | 运维日志 | 写入 Monitor 面板 |
| `graph_node_start` | Graph 节点开始 | 高亮当前节点 |
| `graph_node_end` | Graph 节点结束 | 标记节点完成 |
| `graph_node_error` | Graph 节点错误 | 标记节点错误 |
| `graph_step` | Graph 步骤进度 | 更新进度条 |
| `graph_execution_done` | Graph 执行完成 | 标记工作流完成 |
| `graph_node_custom` | Graph 自定义节点事件 | 自定义渲染 |
| `checkpoint` | 检查点 | 保存断点信息 |
| `intent_pass` | 意图识别 | 显示意图标签 |
| `member_message_start` | Team 成员消息开始 | 显示成员头像+loading |
| `member_delta` | Team 成员增量 | 追加到成员消息气泡 |
| `member_message_done` | Team 成员消息完成 | 标记成员消息完成 |
| `team_run_started` | Team 运行开始 | 显示 Team 运行状态 |
| `team_run_finished` | Team 运行完成 | 标记 Team 运行完成 |
| `team_run_failed` | Team 运行失败 | 显示 Team 运行错误 |
| `team_step_started` | Team 步骤开始 | 显示步骤进度 |
| `team_step_finished` | Team 步骤完成 | 更新步骤状态 |

---

## 四、传输层实现

### 4.1 createWsTransport

```typescript
// web/src/features/chat/ws-transport.ts

export interface WsTransportOptions {
  sessionId: string;
  lastEventId?: string;
  token?: string;
  onEnvelope?: (env: Envelope) => void;
  onConnected?: (info: { sessionId: string; lastEventId?: string }) => void;
  onDisconnected?: () => void;
  onError?: (error: Event) => void;
}

export interface WsTransport {
  connect(): void;
  disconnect(): void;
  send(upstream: WsUpstream): void;
  connected(): boolean;
  lastEventId(): string | undefined;
}

export function createWsTransport(opts: WsTransportOptions): WsTransport {
  let ws: WebSocket | null = null;
  let _connected = false;
  let _lastEventId: string | undefined = opts.lastEventId;
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  let heartbeatTimer: ReturnType<typeof setInterval> | null = null;
  let reconnectAttempts = 0;
  const maxReconnectDelay = 30_000;
  const heartbeatInterval = 25_000;
  const pendingQueue: WsUpstream[] = [];

  function connect(): void {
    if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) {
      return;
    }

    const url = buildWsUrl({
      sessionId: opts.sessionId,
      lastEventId: _lastEventId,
      token: opts.token || readAccessTokenCookie()
    });
    ws = new WebSocket(url);

    ws.onopen = () => {
      _connected = true;
      reconnectAttempts = 0;
      startHeartbeat();
      flushPendingQueue();
    };

    ws.onmessage = (ev: MessageEvent) => {
      try {
        const msg = JSON.parse(ev.data as string) as WsDownstream;
        if (msg.direction !== "server_to_client") return;

        if (msg.type === "connected" && msg.payload) {
          const payload = msg.payload as Record<string, unknown>;
          _lastEventId = (payload.last_event_id as string) || _lastEventId;
          opts.onConnected?.({ sessionId: opts.sessionId, lastEventId: _lastEventId });
          return;
        }

        if (msg.type === "pong") return;

        if (msg.type === "server_shutdown") {
          disconnect();
          return;
        }

        if (msg.envelope) {
          _lastEventId = msg.envelope.id;
          opts.onEnvelope?.(msg.envelope);
        }
      } catch {
        // ignore parse errors
      }
    };

    ws.onclose = () => {
      _connected = false;
      stopHeartbeat();
      opts.onDisconnected?.();
      scheduleReconnect();
    };

    ws.onerror = (e) => {
      opts.onError?.(e);
    };
  }

  function send(upstream: WsUpstream): void {
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify(upstream));
    } else {
      pendingQueue.push(upstream);
    }
  }

  function flushPendingQueue(): void {
    while (pendingQueue.length > 0 && ws && ws.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify(pendingQueue.shift()));
    }
  }

  function startHeartbeat(): void {
    stopHeartbeat();
    heartbeatTimer = setInterval(() => {
      if (ws && ws.readyState === WebSocket.OPEN) {
        send({ direction: "client_to_server", channel: "control", type: "ping" });
      }
    }, heartbeatInterval);
  }

  function stopHeartbeat(): void {
    if (heartbeatTimer) {
      clearInterval(heartbeatTimer);
      heartbeatTimer = null;
    }
  }

  function scheduleReconnect(): void {
    if (reconnectTimer) return;
    const delay = Math.min(1000 * Math.pow(2, reconnectAttempts), maxReconnectDelay);
    reconnectAttempts++;
    reconnectTimer = setTimeout(() => {
      reconnectTimer = null;
      connect();
    }, delay);
  }

  function disconnect(): void {
    if (reconnectTimer) {
      clearTimeout(reconnectTimer);
      reconnectTimer = null;
    }
    stopHeartbeat();
    if (ws) {
      ws.onclose = null;
      ws.close(1000, "client disconnect");
      ws = null;
    }
    _connected = false;
  }

  return {
    connect,
    disconnect,
    send,
    connected: () => _connected,
    lastEventId: () => _lastEventId,
  };
}
```

**关键实现细节**：

1. **Pending 队列**：连接未建立时上行消息入队，连接建立后自动刷新
2. **server_shutdown 处理**：收到服务端关闭通知后主动断开，不再自动重连
3. **Cookie token 回退**：`readAccessTokenCookie()` 从 Cookie 读取 token 作为浏览器场景的认证回退
4. **lastEventId 追踪**：自动记录最新事件 ID，重连时携带

### 4.2 URL 构建

```typescript
// web/src/config/runtime.ts

export function buildWsUrl(opts: {
  sessionId: string;
  lastEventId?: string;
  token?: string;
}): string {
  const base = getWsBaseUrl();
  const params = new URLSearchParams();
  params.set("session_id", opts.sessionId);
  if (opts.lastEventId) params.set("last_event_id", opts.lastEventId);
  if (opts.token) params.set("token", opts.token);
  return `${base}/v1/ws?${params.toString()}`;
}

export function readAccessTokenCookie(): string | undefined {
  if (typeof document === "undefined") return undefined;
  const match = document.cookie.match(/(?:^|;\s*)access_token=([^;]*)/);
  return match ? match[1] : undefined;
}
```

---

## 五、事件分发器

### 5.1 EnvelopeDispatcher 类

```typescript
// web/src/features/chat/dispatcher.ts

export class EnvelopeDispatcher {
  private typeHandlers = new Map<EnvelopeType, Set<(env: Envelope) => void>>();
  private channelHandlers = new Map<string, Set<(env: Envelope) => void>>();
  private globalHandlers = new Set<(env: Envelope) => void>();

  onType(type: EnvelopeType, handler: (env: Envelope) => void): () => void {
    if (!this.typeHandlers.has(type)) {
      this.typeHandlers.set(type, new Set());
    }
    this.typeHandlers.get(type)!.add(handler);
    return () => this.typeHandlers.get(type)?.delete(handler);
  }

  onChannel(channel: string, handler: (env: Envelope) => void): () => void {
    if (!this.channelHandlers.has(channel)) {
      this.channelHandlers.set(channel, new Set());
    }
    this.channelHandlers.get(channel)!.add(handler);
    return () => this.channelHandlers.get(channel)?.delete(handler);
  }

  on(handler: (env: Envelope) => void): () => void {
    this.globalHandlers.add(handler);
    return () => this.globalHandlers.delete(handler);
  }

  dispatch(env: Envelope): void {
    this.globalHandlers.forEach(h => h(env));
    this.typeHandlers.get(env.type)?.forEach(h => h(env));
    if (env.channel) {
      this.channelHandlers.get(env.channel)?.forEach(h => h(env));
    }
  }

  matchFilterKey(env: Envelope, key: string): boolean {
    if (!key || !env.filter_key) return true;
    const subKey = key + "/";
    const envKey = env.filter_key + "/";
    return subKey.startsWith(envKey) || envKey.startsWith(subKey);
  }
}
```

**设计决策**：使用类而非 composable 函数，因为：
- 分发器需要在多个组件间共享实例
- 支持按 type / channel / global 三级过滤
- 提供 `matchFilterKey` 辅助方法对齐后端前缀匹配语义

---

## 六、场景 Hooks

### 6.1 useEnvelopeStream

```typescript
// web/src/features/chat/useEnvelopeStream.ts

export function useEnvelopeStream(sessionId: string) {
  const transport = createWsTransport({
    sessionId,
    onEnvelope: (env) => dispatcher.dispatch(env),
  });

  const dispatcher = new EnvelopeDispatcher();

  onMounted(() => transport.connect());
  onUnmounted(() => transport.disconnect());

  return {
    transport,
    dispatcher,
    send: transport.send,
  };
}
```

### 6.2 useChatStream

```typescript
export function useChatStream(sessionId: string) {
  const { transport, dispatcher } = useEnvelopeStream(sessionId);
  const text = ref("");
  const reasoning = ref("");
  const toolCalls = ref<EnvelopeToolCall[]>([]);
  const done = ref(false);
  const error = ref<EnvelopeError | null>(null);

  dispatcher.onType("text_delta", (env) => {
    text.value += env.content?.text ?? "";
    if (env.content?.reasoning) reasoning.value += env.content.reasoning;
  });
  dispatcher.onType("text_done", () => { /* finalize */ });
  dispatcher.onType("tool_call", (env) => {
    toolCalls.value.push(env.tool_call!);
  });
  dispatcher.onType("tool_result", (env) => {
    const idx = toolCalls.value.findIndex(tc => tc.id === env.tool_call?.id);
    if (idx >= 0) toolCalls.value[idx] = env.tool_call!;
  });
  dispatcher.onType("runner_completion", () => { done.value = true; });
  dispatcher.onType("error", (env) => { error.value = env.error!; });

  function send(content: string, options?: Record<string, unknown>) {
    transport.send({
      direction: "client_to_server",
      channel: "chat",
      type: "user_message",
      payload: { content, ...options },
    });
  }

  function cancel() {
    transport.send({
      direction: "client_to_server",
      channel: "chat",
      type: "cancel",
    });
  }

  return { text, reasoning, toolCalls, done, error, send, cancel };
}
```

### 6.3 useTeamStream

```typescript
export function useTeamStream(sessionId: string) {
  const { transport, dispatcher } = useEnvelopeStream(sessionId);
  const members = ref<Map<string, { author: string; text: string; done: boolean }>>(new Map());
  const transfers = ref<EnvelopeTransfer[]>([]);
  const done = ref(false);

  dispatcher.onType("member_message_start", (env) => {
    members.value.set(env.author, { author: env.author, text: "", done: false });
  });
  dispatcher.onType("member_delta", (env) => {
    const m = members.value.get(env.author);
    if (m) m.text += env.content?.text ?? "";
  });
  dispatcher.onType("member_message_done", (env) => {
    const m = members.value.get(env.author);
    if (m) { m.text = env.content?.text ?? m.text; m.done = true; }
  });
  dispatcher.onType("transfer", (env) => {
    transfers.value.push(env.transfer!);
  });
  dispatcher.onType("team_run_finished", () => { done.value = true; });
  dispatcher.onType("team_run_failed", () => { done.value = true; });

  return { members, transfers, done };
}
```

### 6.4 useMonitorStream

```typescript
export function useMonitorStream(sessionId: string) {
  const { transport, dispatcher } = useEnvelopeStream(sessionId);
  const logs = ref<LogEntry[]>([]);

  function enableLog() {
    transport.send({
      direction: "client_to_server",
      channel: "control",
      type: "enable_log",
      payload: { enabled: true },
    });
  }

  function disableLog() {
    transport.send({
      direction: "client_to_server",
      channel: "control",
      type: "enable_log",
      payload: { enabled: false },
    });
  }

  dispatcher.onType("log", (env) => {
    logs.value.push({
      level: (env.metadata?.level as string) ?? "INFO",
      source: (env.metadata?.source as string) ?? "",
      text: env.content?.text ?? "",
      timestamp: env.timestamp,
    });
  });

  return { logs, enableLog, disableLog };
}
```

### 6.5 useGraphStream

```typescript
export function useGraphStream(sessionId: string) {
  const { transport, dispatcher } = useEnvelopeStream(sessionId);
  const nodes = ref<Map<string, { name: string; status: string; error?: string }>>(new Map());
  const steps = ref(0);
  const done = ref(false);

  dispatcher.onType("graph_node_start", (env) => {
    nodes.value.set(env.author, { name: env.author, status: "running" });
  });
  dispatcher.onType("graph_node_end", (env) => {
    const n = nodes.value.get(env.author);
    if (n) n.status = "done";
  });
  dispatcher.onType("graph_node_error", (env) => {
    const n = nodes.value.get(env.author);
    if (n) { n.status = "error"; n.error = env.error?.message; }
  });
  dispatcher.onType("graph_step", () => { steps.value++; });
  dispatcher.onType("graph_execution_done", () => { done.value = true; });

  return { nodes, steps, done };
}
```

---

## 七、场景交互流程

### 7.1 Chat 对话

```
1. 前端建立 WS 连接
   WS /v1/ws?session_id=sess-1&token=jwt

2. 服务端发送 connected
   ← {channel:"system", type:"connected", payload:{subscribed_channels:["chat","system"]}}

3. 前端发送用户消息
   → {direction:"client_to_server", channel:"chat", type:"user_message", payload:{content:"分析代码"}}

4. 服务端推送事件
   ← {channel:"chat", envelope:{type:"text_delta", content:{text:"我来分析", is_partial:true}}}
   ← {channel:"chat", envelope:{type:"tool_call", tool_call:{name:"read_file",...}}}
   ← {channel:"chat", envelope:{type:"tool_result", tool_call:{name:"read_file", result_json:"..."}}}
   ← {channel:"chat", envelope:{type:"text_delta", content:{text:"这段代码...", is_partial:true}}}
   ← {channel:"chat", envelope:{type:"text_done", content:{text:"这段代码有问题", is_partial:false}}}
   ← {channel:"chat", envelope:{type:"runner_completion", usage:{context_prompt_tokens, max_tokens, turn_total_tokens, ...}}}

**上下文进度条（Composer）**：收到 `context_usage`（ReAct 子步）、`runner_completion` 或 `system.session.compress` 的 `text_done` 时，`streamHandlers` 调用 `sessionContextPatchFromEnvelope` 乐观更新 session store；Composer 圆环 + 副行（ctx/in/out/Σ/费用）与 Usage 大盘口径一致。

5. 前端可随时取消
   → {direction:"client_to_server", channel:"chat", type:"cancel", request_id:"req-1"}

6. 前端可动态开启 Monitor 日志
   → {direction:"client_to_server", channel:"control", type:"enable_log", payload:{enabled:true}}
   ← {channel:"monitor", envelope:{type:"log", metadata:{level:"ERROR",...}}}
```

### 7.2 Team 多 Agent 场景

```
1. 前端连接 WS，发送 subscribe 订阅 team 通道
   → {direction:"client_to_server", channel:"control", type:"subscribe", payload:{channel:"team"}}

2. 收到 Team 生命周期事件
   ← {channel:"team", envelope:{type:"team_run_started"}}
   ← {channel:"team", envelope:{type:"team_step_started"}}

3. 收到成员消息
   ← {channel:"team", envelope:{type:"member_message_start", author:"agent_b", branch:"coordinator/agent_b"}}
   ← {channel:"team", envelope:{type:"member_delta", author:"agent_b", content:{text:"从安全角度看", is_partial:true}}}
   ← {channel:"team", envelope:{type:"member_message_done", author:"agent_b", content:{text:"从安全角度看，这个方案需要...", is_partial:false}}}

4. 收到 Agent 转移
   ← {channel:"team", envelope:{type:"transfer", transfer:{from_agent:"coordinator", to_agent:"agent_b"}}}

5. 前端过滤特定 Agent
   → {direction:"client_to_server", channel:"control", type:"subscribe", payload:{channel:"team", filter_key:"coordinator/agent_b"}}

6. Team 运行完成
   ← {channel:"team", envelope:{type:"team_step_finished"}}
   ← {channel:"team", envelope:{type:"team_run_finished"}}
```

### 7.3 Graph 工作流场景

```
1. 前端连接 WS，发送 subscribe 订阅 graph 通道
   → {direction:"client_to_server", channel:"control", type:"subscribe", payload:{channel:"graph"}}

2. 收到节点事件
   ← {channel:"graph", envelope:{type:"graph_node_start", author:"step_1", trace:{...}}}
   ← {channel:"graph", envelope:{type:"text_delta", content:{text:"分析中...", is_partial:true}}}
   ← {channel:"graph", envelope:{type:"graph_node_end", author:"step_1", trace:{step_count:1}}}

3. 收到步骤进度
   ← {channel:"graph", envelope:{type:"graph_step"}}

4. 收到执行完成
   ← {channel:"graph", envelope:{type:"graph_execution_done"}}

5. 收到检查点
   ← {channel:"graph", envelope:{type:"checkpoint", state_delta:{path:"__checkpoint__", value_json:"..."}}}

6. HITL 中断恢复
   ← {channel:"chat", envelope:{type:"runner_completion", tag:"interrupt"}}
   → 用户审批
   → {direction:"client_to_server", channel:"chat", type:"user_message", payload:{content:"批准"}}
```

### 7.4 Monitor 日志场景

**流程日志（`flow_log`）**：Chat Turn / Team / 系统域经 `TraceEmitter` 推送，**无需** `enable_log`；前端 `useLogStreamHub` → Flow 面板（中文 title + severity 配色）。

**进程日志（`log`）**：Gateway/Plugin 等文本日志，需 `enable_log` 或连接参数 `log_enabled=1`（全局监控在 `ProcessLogEnabled` 时可默认开启）。

```
1. 发 Chat → Monitor Logs「流程」Tab 自动出现 flow_log（无需 enable_log）
   ← {channel:"monitor", envelope:{type:"flow_log", metadata:{severity:"ok", title:"…", step_id:"chat.llm.invoke", trace_id:"…"}}}

2. 可选：开启进程日志
   → {direction:"client_to_server", channel:"control", type:"enable_log", payload:{enabled:true}}
   ← {channel:"monitor", envelope:{type:"log", metadata:{level:"ERROR", source:"tool"}, content:{text:"…"}}}

3. 关闭进程日志（flow_log 仍下发）
   → enable_log enabled:false
```

### 7.5 服务端关闭场景

```
1. 服务端优雅关闭，广播 server_shutdown
   ← {channel:"system", type:"server_shutdown"}

2. 前端收到后不再自动重连
3. 用户可看到"服务已关闭"提示
```

---

## 八、向后兼容与迁移

### 8.1 旧事件名映射

| 旧事件名 | 新 Envelope type | 前端迁移操作 |
|---------|------------------|-------------|
| `delta` | `text_delta` | 监听 Envelope，按 `type` 分发 |
| `done` | `text_done` + `runner_completion` | 拆分处理逻辑 |
| `tool.call` | `tool_call` | 名称统一 |
| `user_message` | 保留为独立上行消息 | 不变 |
| `error` | `error` | 结构增强 |
| `state_delta` | `state_delta` | 从独立事件变为 Envelope type |
| `intent_pass` | `intent_pass` | 从独立事件变为 Envelope type |

### 8.2 迁移阶段

| 阶段 | 内容 | 状态 |
|------|------|------|
| 一 | EventBus + Envelope 引入，双写期 | ✅ 已完成 |
| 二 | 事件格式统一为 Envelope | ✅ 已完成 |
| 三 | WebSocket 统一传输上线 | ✅ 已完成 |
| 四 | 事件持久化与高级特性 | ✅ 已完成（EventBuffer + replay + 动态订阅） |

---

## 九、前端文件结构

```
web/src/
  config/
    runtime.ts                     # buildWsUrl（含 token 参数）+ readAccessTokenCookie + buildHealthWsUrl
  features/
    chat/
      ws-transport.ts              # createWsTransport（含应用层心跳、Cookie token 回退、pending 队列、server_shutdown 处理）
      envelope.ts                  # Envelope 类型定义（31 种，与后端对齐）
      envelopeRunStatus.ts         # run_status 解析
    monitor/
      useLogStreamHub.ts           # flow_log / log 分流；FlowTracePanel 按 trace 过滤
      dispatcher.ts                # EnvelopeDispatcher 类（onType / onChannel / on 过滤 + matchFilterKey）
      useEnvelopeStream.ts         # useEnvelopeStream + 场景 hooks
                                     useChatStream(sessionId) → { text, reasoning, toolCalls, done, error }
                                     useTeamStream(sessionId) → { members, transfers, done }
                                     useMonitorStream(sessionId) → { logs, enableLog, disableLog }
                                     useGraphStream(sessionId) → { nodes, steps, done }
      useEventFilter.ts            # 事件过滤辅助
```

---

## 十、性能考量

### 10.1 前端优化

| 优化点 | 方法 |
|--------|------|
| 消息合并 | 连续 `text_delta` 合并渲染（requestAnimationFrame） |
| 虚拟滚动 | 长对话使用虚拟列表 |
| 背压感知 | WS buffer 未清空时降低渲染频率 |
| 重连去抖 | 指数退避避免频繁重连 |
| 事件去重 | 重放期间跳过已处理的事件 ID |
| Pending 队列 | 连接未建立时上行消息入队，连接后自动刷新 |
