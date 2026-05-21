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
