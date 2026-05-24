# Channel 渠道 — 开发计划

> **版本**：2026-05-24 | **状态**：🟢 9 平台连接；Runtime 生产级重连 + 流式出站 MVP  
> **需求**：[17 channel.md](./17%20channel.md) · **设计**：[17 channel.design.md](./17%20channel.design.md) · **业务集成**：[17-channel-agent-team-integration.md](./17-channel-agent-team-integration.md) · [**外部参考借鉴手册**](./17-channel-external-reference-playbook.md) · [**四层目标架构**](./0-module-decoupling-architecture.md#31-推荐目标架构channel--chat--agent) · [**Phase DECO**](./17-channel-development.md#14-phase-deco--四层架构解耦deco)  
> **Hermes 对照**：[17 channel.design.md §十四](./17%20channel.design.md#十四hermes-agent-对照消息流转与飞书特殊处理) · Phase F backlog 见 **§11**  
> **平台参考**：[MuseBot](https://github.com/yincongcyincong/MuseBot) `robot/`（MIT）  
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：EP-BIZ-08

---

## 1. 模块定位

Channel：在 Kratos 层实现外部 IM 平台连接，参考 MuseBot 的 SDK 选型与连接模式，桥接到 `ChatService` Agent 运行时。

**代码锚点**：

- `api/kratos/channel/v1/channel.proto`
- `internal/service/channel.go` / `channel_ingress*.go` / `channel_delivery_worker.go` / `channel_runtime.go`
- `internal/biz/channel*.go` / `channel_catalog.go`
- `internal/channel/{lark,dingtalk,wecom,slack,telegram,discord,wechat,onebot,qq,runtime}/`
- `internal/cronrunner/jobs/channel_delivery.go` / `channel_health.go`
- `web/src/features/channels/` · `web/src/components/channels/`

**MuseBot 对照文件**（连接层移植时阅读）：

| 平台 | MuseBot |
|------|---------|
| 飞书 | `robot/lark.go` |
| 钉钉 | `robot/ding.go` |
| 企微 | `robot/comwechat.go` |
| 微信 | `robot/wechat.go` |
| Slack | `robot/slack.go` |
| Telegram | `robot/telegram.go` |
| Discord | `robot/discord.go` |
| QQ | `robot/qq.go` |
| OneBot | `robot/personalqq.go` |
| Webhook 路由 | `http/http.go` · `http/communicate.go` |

---

## 2. 现状评估

| 项 | 状态 | 说明 |
|----|------|------|
| Webhook 入站 7 平台 | ✅ | feishu / dingtalk / wecom / slack / telegram / wechat / onebot |
| 统一 ProcessInbound | ✅ | webhook + runtime 共用；流式/一元分支 |
| 异步 delivery + 重试 | ✅ | worker 5s，指数退避，最多 3 次 |
| delivery Prometheus + dead-letter | ✅ | `aranea_channel_delivery_total{platform,status}` |
| DB 多实例 + 凭据加密 | ✅ | `channel` + `channel_credential` |
| Catalog bundled 标记 | ✅ | 9/10 平台 bundled（qq ✅ botgo） |
| MuseBot 全平台 Catalog 规格 | ✅ | 文档 + catalog 10 项 |
| 长连接 Runtime | ✅ | larkws / ding stream / socketmode / polling / discord |
| Manager.Reload reconcile | ✅ | config/enabled/receive_mode fingerprint |
| Runtime 断线重连 | ✅ | `runSupervised` 指数退避 1s→5m |
| Runtime fingerprint 含凭据 revision | ✅ | `CredentialsRevision` + CRUD reload |
| 流式 edit 回复 | ✅ MVP | Telegram / Feishu / Slack；其余 unary 回退 |
| 流式错误传播 + Prometheus | ✅ | `OnReplyDelta` 中断 + `aranea_channel_stream_update_total` |
| platformAdapters 统一出站/流式 | ✅ | `channel_platform_registry.go` |
| 全平台 webhook 单测 | 🟡 | 部分（lark/dingtalk/slack/telegram/wecom/wechat/onebot） |
| 前端 MuseBot 布局 + composable | ✅ | `useChannelEditorForm.ts` |

---

## 3. 平台矩阵（MuseBot 对齐）

| type | 连接模式（MuseBot） | Aranea 现状 | bundled | SDK |
|------|---------------------|-------------|---------|-----|
| 飞书 / feishu | larkws（默认） | larkws ✅ · Webhook UI 暂未开放 · 流式 patch ✅ | ✅ | larksuite/oapi-sdk-go |
| `dingtalk` | Stream / webhook | webhook ✅ · stream ✅ | ✅ | dingtalk-stream-sdk-go |
| `wecom` | webhook | ✅ | ✅ | PowerWeChat / 自研 |
| `wecom-app` | webhook | ✅ | ✅ | 同上 |
| `wechat` | webhook | webhook ✅ · outbound ✅ | ✅ | PowerWeChat |
| `slack` | Socket Mode / Events | Events ✅ · socketmode ✅ · 流式 update ✅ | ✅ | slack-go + socketmode |
| `telegram` | polling / webhook | webhook ✅ · polling ✅ · 流式 edit ✅ | ✅ | go-telegram-bot-api |
| `discord` | gateway WS | gateway ✅ · outbound ✅ | ✅ | discordgo |
| `qq` | webhook + botgo WS | webhook ✅ · outbound ✅ | ✅ | botgo |
| `personal_qq` | OneBot HTTP | webhook ✅ · outbound ✅ | ✅ | OneBot 协议 |

---

## 4. 路线图

### Phase A — 巩固 Webhook（P0）

| # | 任务 | 状态 |
|---|------|------|
| A1 | 文档按 MuseBot 重写 | ✅ |
| A2 | Catalog 扩展至 10 平台 | ✅ |
| A3 | 全平台 webhook 验签单测 | 🟡 |
| A4 | `channel/port.go` 接口草案 | ⏳ |

### Phase B — ChannelRuntimeManager（P1）

| # | 任务 | 状态 | MuseBot 参考 |
|---|------|------|--------------|
| B1 | `channel_runtime.go` + Wire | ✅ | `StartRobot()` |
| B2 | 飞书 `receive_mode=websocket` | ✅ | `StartLarkRobot` + larkws |
| B3 | 钉钉 `receive_mode=stream` | ✅ | `StartDingRobot` + StreamClient |
| B4 | Slack `receive_mode=socket_mode` | ✅ | `StartSlackRobot` |
| B5 | Telegram `receive_mode=polling` | ✅ | `StartTelegramRobot` |
| B6 | Discord gateway | ✅ | `discord.go` |
| B7 | Manager.Reload reconcile | ✅ | — |
| B8 | 断线重连 + Prometheus | ✅ | `runSupervised` |
| B9 | 定期 Reload 兜底 | ✅ | 2m ticker |

### Phase C — 新平台连接（P1–P2）

| # | 任务 | 状态 | MuseBot 参考 |
|---|------|------|--------------|
| C1 | 微信公众号 `wechat` | ✅ | `wechat.go` + PowerWeChat |
| C2 | Discord `discord` | ✅ | `discord.go` + discordgo |
| C3 | QQ 官方 `qq` | ✅ | `qq.go` + botgo |
| C4 | OneBot `personal_qq` | ✅ | `personalqq.go` |

### Phase D — 体验增强（P2）

| # | 任务 | 状态 | MuseBot 参考 |
|---|------|------|--------------|
| D1 | 流式回复 StreamOutbound | ✅ MVP | Telegram/Feishu/Slack |
| D2 | 群 @ 门控 + allowlist | ✅ | `internal/biz/channel_access.go` + `channel_ingress_access.go` |
| D3 | 路由 UI：Team / dm_scope / rules | ✅ | Agent/Team 下拉 ✅；`dm_scope` 下拉 ✅；rules 表 ✅ |
| D5 | Web Chat 同步 Channel 入站 | ✅ | `useChatInboundSync` + session `metadata_json.source=channel` |
| D6 | 路由变更重置 peer 绑定 | ✅ | `UpdateChannel` + `DeleteByChannelID` |
| D7 | Stale peer bind 自动 rebind | ✅ | CC-HOT-01 · Session 软删后飞书入站自动建新 Session |
| D4 | delivery Prometheus + dead-letter | ✅ | `aranea_channel_delivery_*` |

---

## 5. 流式出站（W5）

**开关**：`config_json.config.streaming_enabled: true`

**路径**：

```
ProcessInbound → processInboundStreaming → RunNativeTurnStreaming
  → trpc_turn OnReplyDelta → platform StreamSender.Update
```

| 平台 | 首条 | 增量 | 实现 |
|------|------|------|------|
| `telegram` | sendMessage | editMessageText（2s 节流） | `internal/channel/telegram/stream_outbound.go` |
| `feishu` | im.v1 messages POST | im.v1 messages PATCH | `internal/channel/lark/stream_outbound.go` |
| `slack` | chat.postMessage | chat.update（2s 节流） | `internal/channel/slack/stream_outbound.go` |
| 其他 | — | unary 回退 | delivery 队列 |

流式回合不走 outbound delivery 队列；失败写入 `channel_delivery` audit（`status=streamed|error`）。

---

## 6. Runtime 运维

| 指标 | 说明 |
|------|------|
| `aranea_channel_runtime_connected{platform,receive_mode}` | 活跃长连接数 |
| `aranea_channel_runtime_reconnect_total{platform,receive_mode,outcome}` | disconnect / attempt |
| `aranea_channel_stream_update_total{platform,phase,result}` | 流式 delta / flush（ok / error） |
| `aranea_channel_delivery_total{platform,status}` | delivery 含 dead_letter |

**行为**：

- 意外断线：`runSupervised` 指数退避重连（1s → 5m）
- CRUD / 凭据变更：`ChannelService.reloadRuntime()` → fingerprint reconcile（含 `CredentialsRevision`）
- 定期兜底：启动后每 **2 分钟** `Manager.Reload()`（可配置/关闭）
- **fingerprint 不含 `UpdatedAt`**（2026-05-22）：避免健康检查 metadata 更新触发双 WebSocket；替换连接器前等待旧实例 `done`

**入站门控（2026-05-22）**：

- `lark.AcceptFeishuInbound`：WS / Webhook 统一；仅 `sender_type=user`、必须有 `message_id`
- `channel_inbound_receipt`：同一 `feishu:{message_id}` 只 Turn 一次
- 审计：`channel.inbound.receive` · `channel.runtime.connector_start`
- 详见 [changelog/2026-05-22-Channel-Inbound-Root-Cause.md](../changelog/2026-05-22-Channel-Inbound-Root-Cause.md) · [review/2026-05-22-Channel-Inbound-Review.md](../review/2026-05-22-Channel-Inbound-Review.md)

---

## 7. 验收标准

- [x] 文档以 MuseBot 平台连接为参考源（无 GoClaw 依赖）
- [x] 7 平台 Webhook 端到端可用
- [x] Runtime scaffold（5 长连接 + Reload reconcile）
- [x] Runtime 断线重连 + 定期 Reload
- [x] 流式出站 Telegram / Feishu / Slack
- [x] delivery Prometheus dead-letter 指标
- [x] 前端 MuseBot 布局 + `useChannelEditorForm`
- [x] Catalog 含 MuseBot 10 平台
- [x] `go test ./internal/channel/... ./internal/service/...` 全绿
- [ ] Phase B 多实例生产压测（可选）

---

## 8. Review 优化闭合（2026-05-22）

| 项 | 实现 |
|----|------|
| `trpc_turn.go` UTF-8 | 恢复中文 FlowLog + `OnReplyDelta` wiring |
| Feishu WS `receive_id_type` | `lark.ResolveReceiveTarget` + StreamSender 动态 type |
| Runtime 凭据 fingerprint | `runtime.CredentialsRevision` |
| 流式错误传播 | `turn_helpers.OnReplyDelta` 失败中断 turn |
| 流式指标 | `aranea_channel_stream_update_total` |
| 统一 platform registry | `platformAdapters`（`channel_platform_registry.go`） |
| Session SRP | `channel_ingress_session.go` + `prepareChannelChatRequest` |
| 流式失败 fail-fast | `streamPreviewTurnError` → `TurnErrStreamPreviewFailed` |
| Feishu unary receive_id_type | `FeishuTextSender` + `SendTextMessage` |
| Runtime 凭据读取失败 | `event.SysLogWarn` → `channel.runtime.credentials_fail` |
| Feishu WS 异步入站 | `safego.Go` + `chat_id` 出站 + 群 @ 门控 |
| Feishu 默认连接模式 | catalog / UI 默认 `websocket`；Webhook 暂未开放 |
| Channel ROUTING UI | Agent / Team 下拉联动系统实例 |

详见 [17-channel-review.md](../review/17-channel-review.md)（**92/100**）。

---

## 9. 运维

| 变量 | 作用 |
|------|------|
| `CHANNEL_DELIVERY_DISABLED=1` | 关闭出站 worker |
| `CHANNEL_HEALTH_DISABLED=1` | 关闭健康扫描 |
| `CHANNEL_RUNTIME_DISABLED=1` | 关闭长连接 Runtime |
| `CHANNEL_RUNTIME_RELOAD_INTERVAL` | 定期 Reload 间隔（默认 `2m`；`0`/`off` 关闭） |
| `ARANEA_CREDENTIAL_KEY` | 凭据加密 |

Webhook 默认：`/webhooks/{channel_key}`。

---

## 10. 长任务异步执行（Phase E）

> **需求**：[17 channel.md §8](./17%20channel.md#8-长任务场景飞书-channel)  
> **设计**：[17 channel.design.md §十二](./17%20channel.design.md#十二长任务异步执行设计)  
> **M55 延伸**（24h Job、auto 路由、Web Job 面板）：[55-chat-channel-cursor-development.md §Phase A/F](./55-chat-channel-cursor-development.md#phase-a--配置与路由p0约-3-天)  
> **验收 ID**：LT-01 – LT-07

### 10.1 目标与原则

- **Accept ≠ Execute**：Ingress 快速响应平台；Turn 在后台完成。  
- **SRP**：受理 / 执行 / 进度 / 出站 分文件；biz 只做 Job 与配置。  
- **复用 Chat 路径**：不复制 `trpc_turn`；Channel 仅加 IM 投影与 Job 审计。  
- **影响域可控**：P0 仅改 `channel_ingress*` + config helper；P1 加表与 Projector；P2 才触 Graph/Cron。

### 10.2 任务板

#### E0 — 配置与契约（P0 前置，1d）

| ID | 任务 | 包 / 文件 | 状态 | 验收 |
|----|------|-----------|------|------|
| E0-1 | `ChannelLongTaskConfig` 解析 + 单测 | `biz/channel_config_helpers.go` | ✅ | 默认值与 §8.4 一致 |
| E0-2 | `TurnOutcome` 枚举（ok / queued / rejected / error） | `biz/channel_turn_outcome.go` + `runChatTurnWithOutcome` | ✅ | `runChatTurn*` 可返回 |
| E0-3 | 文档 / Catalog schema 补字段说明 | `17 channel.md` · 前端 schema（可选） | ✅ | 需求 §8.4 已定义 |

#### E1 — 快速 ACK + Webhook 异步（P0，2–3d）

| ID | 任务 | 包 / 文件 | 状态 | 验收 |
|----|------|-----------|------|------|
| E1-1 | `acceptInbound`：ACK 出站 + delivery 记录 | `service/channel_ingress_accept.go` | ✅ | LT-01 ACK ≤2s |
| E1-2 | Webhook：`200` 后 `safego` 执行 Turn | `service/channel_ingress_http.go` | ✅ | LT-04 HTTP ≤3s |
| E1-3 | WS：`HandleWSInbound` 统一走 accept → execute | `channel/lark/ws_inbound.go` · `ProcessInbound` | ✅ | 与 Webhook 行为一致 |
| E1-4 | 入队时 outbound `ack_on_queued` | `service/channel_ingress_execute.go` + stream | ✅ | LT-03 |
| E1-5 | 单测：ctx timeout / config 解析 | `channel_turn_context_test.go` · `channel_config_helpers_test.go` | ✅ | CI 绿 |

**依赖**：E0-1、E0-2。  
**不影响**：Web Chat RPC、gRPC Chat API（TurnOutcome 对 Web 透明）。

#### E2 — Channel 级超时（P0，1d）

| ID | 任务 | 包 / 文件 | 状态 | 验收 |
|----|------|-----------|------|------|
| E2-1 | Execute 层注入 `turn_timeout_sec` / `first_byte_timeout_sec` | `service/channel_ingress_execute.go` → ctx | ✅ | LT-05 |
| E2-2 | `trpc_turn` 读取 ctx 可选 first-byte deadline | `service/trpc_turn.go` | ✅ | Team 5min 工具场景 |
| E2-3 | 超时 IM 错误文案 + FlowLog `channel.turn.timeout` | ingress + `lark/ws_inbound` notify | ✅ | `channel.turn.execute/done/timeout` + SysLog |

**依赖**：E1。  
**影响域**：仅 Channel 入站 ctx；Web Chat 仍用全局默认。

#### E3 — ChannelTurnJob（P1，3–4d）

| ID | 任务 | 包 / 文件 | 状态 | 验收 |
|----|------|-----------|------|------|
| E3-1 | Ent schema + SQL + Repo | `data/channel_turn_job*.go` · `docs/sql/04_channel.sql` | ✅ | 启动 Ensure + SQL 文档 |
| E3-2 | `biz.ChannelTurnJob` + Repo 接口 | `biz/channel_turn_job.go` | ✅ | 幂等键唯一 |
| E3-3 | Execute 创建 Job accepted→running→终态 | `channel_ingress_job.go` · execute | ✅ | LT-07 可查询 |
| E3-4 | Prometheus `aranea_channel_turn_*` | `internal/metrics/vars.go` | ✅ | duration + job_total |
| E3-5 | Admin API `ListChannelTurnJobs`（可选 P1.5） | `api/.../channel.proto` · `ChannelTurnJobsPanel` | ✅ | GET `/v1/channels/{id}/turn-jobs` |

**依赖**：E1。  
**SRP**：Job 持久化不在 Ingress 文件内联 SQL。

#### E4 — 进度投影（P1，3–4d）

| ID | 任务 | 包 / 文件 | 状态 | 验收 |
|----|------|-----------|------|------|
| E4-1 | `ChannelProgressProjector` 注册 / 注销 | `service/channel_progress_projector.go` | ✅ | Turn 结束 defer 取消 |
| E4-2 | 订阅 tool / member / heartbeat | 同上 + EventBus filter | ✅ | LT-02 工具有文案 |
| E4-3 | `progress_mode` / `progress_quiet_sec` 配置门控 | `biz/channel_config_helpers.go` | ✅ | off 时不订阅 |
| E4-4 | 指标 `aranea_channel_progress_patch_total` | metrics | ✅ | ok/error 计数 |

**依赖**：E1、E3（preview_message_id）。  
**不影响**：WebSocket 客户端 Envelope 语义。

#### E5 — 前端配置（P1，1–2d）

| ID | 任务 | 包 / 文件 | 状态 | 验收 |
|----|------|-----------|------|------|
| E5-1 | Channel 编辑：ACK / 超时 / progress 表单项 | `channelPlatformFields.ts` · `useChannelEditorForm.ts` | ✅ | LONG TASK 分区保存 config_json |
| E5-2 | 长任务推荐预设（客服 / Team 模板） | `channelLongTaskPresets.ts` | ✅ | 与 §8.6 一致 |

**依赖**：E0-1。

#### E6 — 超长任务 async（P2，5d+）

| ID | 任务 | 包 / 文件 | 状态 | 验收 |
|----|------|-----------|------|------|
| E6-1 | `execution_mode=async` Accept 触发 Graph/Cron | `channel_ingress_async.go` · accept | ✅ | 立即 async ACK |
| E6-2 | 完成事件 → IM 出站 | `watchAsyncGraphCompletion` | ✅ | Graph done 通知 |
| E6-3 | `/async` 命令或意图路由（auto） | `biz.ShouldRunAsync` | ✅ | auto 模式 |
| E6-4 | 飞书「取消」→ `CancelRun` | `channel_ingress_cancel.go` | ✅ | 取消/ cancel / /cancel |

**依赖**：E3；Graph/Cron 稳定 API。  
**影响域**：Graph(36)、Cron(21)、Event(34)。

### 10.3 推荐迭代顺序

```
迭代 E-a（P0，约 1 周）
  E0 → E1 → E2 → 验收 LT-01/03/04/05/06

迭代 E-b（P1，约 1.5 周）
  E3 → E4 → E5 → 验收 LT-02/07

迭代 E-c（P2，按需）
  E6 + 飞书交互卡片
```

### 10.4 验证命令

```bash
make wire && make api && make build
go test ./internal/biz/... ./internal/service/... -count=1
make runtime-boundary
```

### 10.5 文档与 changelog

| 交付 | 路径 |
|------|------|
| 需求 | `17 channel.md` §8 |
| 设计 | `17 channel.design.md` §十二 |
| 集成差距 | `17-channel-agent-team-integration.md` §6 |
| 实现完成后 | `changelog/YYYY-MM-DD-Channel-Long-Task-Phase-E.md` |


---

**Hermes 对照文件**（飞书 IM 行为，连接层非 LLM）：

| 主题 | Hermes |
|------|--------|
| 飞书适配器 | `gateway/platforms/feishu.py` |
| Gateway 调度 | `gateway/run.py` |
| 会话键 | `gateway/session.py` `build_session_key` |
| 流式出站 | `gateway/stream_consumer.py` |
| 用户文档 | `website/docs/user-guide/messaging/feishu.md` |

---

## 11. Phase F — Hermes 飞书借鉴（P1–P2）

> **设计对照**：[17 channel.design.md §十四](./17%20channel.design.md#十四hermes-agent-对照消息流转与飞书特殊处理)  
> **背景**：Hermes 个人 Gateway 在飞书 text 分批、Reaction 反馈、thread 回复、Webhook 限流等方面有成熟实现；Aranea 在 IM Preview / Turn Job / Tool Card 已领先，本 Phase 补齐连接层缺口。

### 11.1 任务板

| ID | 优先级 | 任务 | 包 / 文件 | 状态 | 验收 |
|----|--------|------|-----------|------|------|
| F-01 | P1 | WS ping/reconnect 可配置项写入 `config_json.config` | `runtime/supervisor.go` | 🟡 | `ws_reconnect_interval_sec` ✅；larkws ping 参数仍依赖 SDK |
| F-01b | P1 | Runtime 连接状态 chip（connected_since / last_disconnect） | `runtime/connection.go` · `ChannelsTable.vue` | ✅ | ListChannels 合并 `runtime_connected*` metadata |
| F-03 | P1 | Webhook 入站 per `channel_key` + IP 限流（如 120/min） | `channel_ingress_ratelimit.go` | 🟡 | per `channel_key` 120/min ✅；IP 维度 ⏳ |
| F-04 | P1 | 飞书 text 入站 debounce 合并（500–800ms；大段 +2s） | `lark/inbound_batch.go` · `ws.go` | ✅ | WS 路径 per-peer 合并 |
| F-06 | P1 | `thread_id` 入 OutboundMeta + `thread_sessions_per_user` 路由 | `inbound_build.go` · `channel_ingress_peer.go` | ✅ | 话题群独立 peer_key |
| F-06b | P1 | 出站 `reply_in_thread` / receive_id_type=thread_id | `feishu_outbound.go` | ✅ | delivery Extra 透传 |
| F-07 | P1 | 流式 PATCH 达 `PlatformTextLimit` 主动 split 新消息 | `TurnPreviewCoordinator` · `channel_delivery_worker.go` | ✅ | `im_split_overflow` 流式 + 出站 SplitPages |
| F-11 | P1 | Webhook `encrypt_key` 加密事件体 AES 解密 | `lark/event_decrypt.go` | ✅ | `{"encrypt":"..."}` 解密后验签解析 |
| F-02 | P2 | 同 `app_id` 多 enabled channel 启动冲突检测 | `runtime/manager.go` | ✅ | 第二 channel SysLog + 跳过 WS 启动 |
| F-05 | P2 | 入站 post 转 plain text；image 附件（可选） | `parse_message.go` | 🟡 | post ✅；image 附件 ⏳ |
| F-08 | P2 | 出站 `msg_type=post`（markdown 子集） | `feishu_outbound.go` | ⏳ | 粗体/链接保留 |
| F-09 | P2 | 首字节前 Reaction「处理中」；结束移除 | `lark/reaction.go` | ✅ | `config.processing_reaction` 开启时生效 |
| F-10 | P2 | `busy_input_mode: queue \| followup \| interrupt` | `channel_config_helpers.go` · ingress | ✅ | queue/followup steer+合并；interrupt CancelRun |

### 11.2 不建议照搬（Aranea 已更优或架构不同）

| Hermes 能力 | 原因 |
|-------------|------|
| PTY 嵌入 TUI Chat | Aranea 原生 Web Chat + WS Envelope |
| 单文件 SessionStore JSON | DB `channel_peer_session` + Ent |
| DM pairing 8 位码 | 企业场景用 `allowed_user_ids` / Admin |
| 8000 字多段 **独立 send**（非 PATCH） | 优先 F-07 split + 现有 `im_split_overflow` |
| Curator / Skill 自治 | 属 Agent 域，非 Channel |

### 11.3 推荐迭代顺序

```
迭代 F-a（P1，约 1 周）
  F-04 → F-07 → F-11 → F-03 → 飞书 E2E 回归 LT-01/03

迭代 F-b（P1，约 1 周）
  F-06 + F-06b → F-01/F-01b → 话题群验收

迭代 F-c（P2，按需）
  F-09 → F-10 → F-05 → F-08
```

### 11.4 验证命令

```bash
go test ./internal/channel/lark/... ./internal/service/... -run 'Feishu|Inbound|Stream|Webhook' -count=1
go test ./internal/channel/preview/... -count=1
```

---

## 12. IM Preview — E2E 验收清单（LT-01–07）

> 原 `需求/17-channel-development.md#12-im-preview--e2e-验收清单lt-0107` 已并入本文。

## 前置条件

| 项 | 要求 |
|----|------|
| Channel | 飞书平台、`streaming_enabled=true`、`im_render_mode=transcript` |
| 推荐 preset | 「飞书 · IM Preview（推荐）」或 `channelImPreviewDefaults.ts` |
| Card（可选） | `im_tool_card_mode=feishu_append` |
| Web 深链 | `metadata.web_app_origin` 指向 Web Admin（如 `https://admin.example.com`） |
| 分页（可选） | `im_split_overflow=true` |

---

## LT-01 — 长任务 ACK + 流式 PATCH

| 步骤 | 预期 |
|------|------|
| 1. 发送需 ~2min 的生成请求 | ≤2s 内飞书出现**单条** preview（含 ACK 文案） |
| 2. Turn 执行中 | preview **PATCH** 演进：正文 → 工具 → 正文（非覆盖 ACK） |
| 3. 静默 ≥ `progress_quiet_sec` 且无进行中工具 | preview 追加心跳行（不覆盖 transcript） |
| 4. Turn 完成 | 最终 preview 含完整回复 |

**指标**：`aranea_channel_stream_update_total{phase=flush,result=ok}` · FlowLog `channel.preview.patch`

---

## LT-02 — 思考链（可选）

| 步骤 | 预期 |
|------|------|
| 配置 `im_render_mode=transcript_with_reasoning`（或 `im_show_reasoning: true`） | preview PATCH 含 **`【思考过程】`** 段（截断至 `im_reasoning_max_chars`）与 **`【正文】`** 段 |
| Turn 完成（Durable / 长任务）且存在 `reasoning_markdown` | 飞书收到 **Card 2.0「Agent 回复」**（思考 + 正文分区）或带标签纯文本 |
| `reply_only` 模式 | 仅 outbound **正文**（不含思考段） |

**代码**：`internal/channel/preview/render.go` · `format_im.go` · `feishu_reply_card.go`

---

## LT-02b — 出站文本格式化

| 步骤 | 预期 |
|------|------|
| Unary / 完成通知正文 | Markdown/ReAct 标签清理为 IM 可读纯文本（`FormatAssistantReplyForIM`） |
| 流式 preview transcript | 保留工具行，仅 MD 清理（`FormatRenderedTranscriptForIM`） |
| 格式化后空串 | FlowLog `channel.outbound.text` warn + fallback 文案 |

---

## LT-03 — 排队 / 并发 Turn

| 步骤 | 预期 |
|------|------|
| 同 session 已有 active run 时再发消息 | 排队 ACK（`ack_on_queued`）或 queued Job 文案 |

---

## LT-04 — 工具 Card（`feishu_append`）

| 步骤 | 预期 |
|------|------|
| 1. 工具 `tool_call` | 追加 1 条 Interactive Card（橙色 🔄 进行中） |
| 2. 同工具 `tool_result` | **PATCH 同一条 Card**（绿色 ✓ / 红色 ✕），非第二条消息 |
| 3. 多工具 Turn | 每工具 1 条 Card（create + update） |
| 4. Web 详情按钮 | 打开 `{web_app_origin}/sessions/{id}?focus=tool&tool_id={id}` |

**指标**：`aranea_channel_tool_card_total{phase=send|update,result=ok}`

---

## LT-05 — 超长分页（`im_split_overflow`）

| 步骤 | 预期 |
|------|------|
| Turn 结束 preview 超出飞书单条上限 | 首条 PATCH 截断页 + delivery worker 分页 enqueue 后续消息 |

---

## LT-06 — 取消 / 超时

| 步骤 | 预期 |
|------|------|
| 用户取消或 Turn 超时 | preview 保留已投影进度；Job 终态 failed/timeout；IM 固定错误文案 |

---

## LT-07 — 运维可观测

| 步骤 | 预期 |
|------|------|
| Monitor / Session 按 `session_id` | 可见 Channel Turn Job、FlowLog（`channel.turn.*` / `channel.preview.patch` / `channel.tool.card`） |
| Session Timeline 深链 | `focus=tool&tool_id` 高亮对应工具事件 |

---

## M55 Chat×Channel 验收（CC-E2E-01）

| ID | 步骤 | 预期 |
|----|------|------|
| M55-SYNC-01 | 飞书 Turn 进行中，Web 打开同 Session | ≤5s 内 user / running 可见；顶栏 `rev` 递增 |
| M55-SYNC-02 | 飞书 Turn 完成，Web 已打开 | assistant 自动出现；UserBubble 显示「飞书」来源徽标 |
| M55-UI-01 | 100+ 消息 Session，TurnBlock 开启 | 滚动流畅（虚拟列表）；工具默认折叠 |
| M55-UI-02 | 单轮 20+ 工具 | ToolStrip 折叠/展开正常 |
| M55-JOB-01 | 飞书 `/async` 或长任务关键词 | Web「后台任务」面板 ≤3s 出现 Job |

### M55-RUN — Run 生命周期 E2E（CC-R / CC-R-OPT）

> **Review**：[2026-05-23-M55-Run-Lifecycle-Review.md](../review/2026-05-23-M55-Run-Lifecycle-Review.md) · **前置**：飞书应用订阅 **`card.action.trigger`**

| ID | 步骤 | 预期 |
|----|------|------|
| M55-RUN-01 | Channel 长任务 preset · 软预算到达 | 收到 Feishu 交互卡片「后台继续」或文本提示；FlowLog `run.budget.soft` |
| M55-RUN-02 | 点击卡片「后台继续」或回复 `/background` | IM 回复「已转入后台」；`session_runs.phase=durable`；checkpoint 行存在 |
| M55-RUN-03 | admin 重启或等待 Worker poll | Durable Worker **仅一次** 续跑；Turn 完成 → `phase=completed` |
| M55-RUN-04 | Web 打开同 Session · 后台任务面板 | 点击 forum 图标 → TurnBlock 滚动到对应 `turn_id` |

**平台矩阵（CC-R-OPT-11）**

| 平台 | 软预算通知 | 后台确认 |
|------|-----------|----------|
| feishu | 交互卡片 + callback | 卡片按钮 / `/background` |
| 其他 | 文本 outbound | `/background` |

```bash
# Sprint 1 优化落地后
go test ./internal/service/... -run 'DurableWorker|EscalateSessionRun|BudgetWatcher' -count=1
go test ./internal/biz/... -run SessionRun -count=1
# 后端（通用）
go test ./internal/service/ -run 'EnsureChannelSession|ListSessionMessages_afterRevision|ListChatBackgroundJobs' -count=1
# 前端
cd web && pnpm test -- messageSourceMeta groupMessagesByTurn
```

---

## 回归命令

```bash
go test ./internal/biz/... ./internal/channel/preview/... ./internal/channel/lark/... -count=1
go test ./internal/service/ -run "TurnPreview|Interactive" -count=1
```

---

## 已知限制

- 飞书 Card 为静态 JSON；进行中状态为 emoji 文案，非动画。
- Card HTTP 在独立 goroutine 发送，不阻塞 EventBus 消费。
- 无 tenant 时 LT-04/05 仅能通过 httptest 契约测（`interactive_card_test.go`）验证。
---

## 13. Phase G — 外部参考借鉴（CH-BOR）

> **权威正文**：[17-channel-external-reference-playbook.md](./17-channel-external-reference-playbook.md) · **Review**：[2026-05-24-Channel-External-Reference-Playbook-Review.md](../review/2026-05-24-Channel-External-Reference-Playbook-Review.md)  
> **与 §11 分工**：Phase F = Hermes **飞书平台特化**；Phase G = GoClaw + trpc OpenClaw **跨平台调度/网关**模式。

| ID | 优先级 | 内容 | 状态 |
|----|--------|------|------|
| CH-BOR-01 | P0 | followup 队列合并 | ✅ |
| CH-BOR-02 | P0 | 群聊/DM 并发上限 | ✅ |
| CH-BOR-03 | P0 | 忙线 intent（cancel/status/steer） | ✅ |
| CH-BOR-04 | P0 | intent metrics + FlowLog | ✅ |
| CH-BOR-05 | P1 | Ingress debounce | ✅ |
| CH-BOR-06 | P1 | Ingress dedupe | ✅ |
| CH-BOR-07 | P1 | Run 级 preview registry | ✅ |
| CH-BOR-08 | P1 | Block/final outbound 去重规则 | ✅ |
| CH-BOR-09 | P1 | Provider 错误 taxonomy | ✅ |
| CH-BOR-10 | P2 | local_key / OutboundMeta 契约（`port/meta.go`） | ✅ |
| CH-BOR-11 | P2 | context 阈值降并发（`context_admission_threshold`） | ✅ |
| CH-BOR-12 | P2 | stream sanitize（`preview/sanitize.go`） | ✅ |
| CH-BOR-13 | P3 | Lane scheduler（`runtime/lane.go`） | ✅ |
| CH-BOR-14 | P3 | durable turn compaction hook（`BeforeDurableTurn`） | ✅ |

---

## 14. Phase DECO — 四层架构解耦（DECO-*）

> **权威架构**：[0-module-decoupling-architecture.md §3.1](./0-module-decoupling-architecture.md#31-推荐目标架构channel--chat--agent) · **后端路线**：[同文档 §6 Phase B1–B2](./0-module-decoupling-architecture.md#6-后端解耦路线)  
> **与 §11/§13 分工**：Phase F = 飞书平台特化；Phase G = 外部调度借鉴（CH-BOR）；**Phase DECO = 四层落地（Ingress / Policy / Turn / Projector）**  
> **跨模块**：DECO-06/12/13 亦见 [1-chat-development.md](./1-chat-development.md) · [55-chat-channel-cursor-development.md](./55-chat-channel-cursor-development.md)

### 14.1 任务板

#### DECO-a — 巩固 L3 + L4（P0）

| ID | 层 | 内容 | 落点 | 验收 | 状态 |
|----|-----|------|------|------|------|
| DECO-01 | L4 | revision E2E：飞书 Turn 中/完成 → Web 增量可见 | M55-SYNC-01/02 · `useChatInboundSync` | 同 session ≤5s 见 user/running；completed 后 assistant 出现 | 🟡 [E2E 归档](../changelog/2026-05-24-DECO-01-Feishu-Web-E2E-Archive.md) + [Holistic Fix](../changelog/2026-05-24-DECO-01-Channel-Sync-Holistic-Fix.md)；**Review P1** [DECO-R-P1-01~02](../review/2026-05-24-DECO-01-Channel-Sync-Holistic-Fix-Review.md#p1--当前迭代应修) 待收敛 |
| DECO-02 | L3 | Channel 全路径 `RunNativeTurnWithOutcome`，去掉 empty-reply 启发式 | `channel_ingress_turn.go` · `NativeTurnResult` | `TestFormatChannelTurn*` + 飞书 unary/stream 回归 | ✅ |

#### DECO-b — 抽 L3 TurnExecutor（P0–P2）

| ID | 层 | 内容 | 落点 | 验收 | 状态 |
|----|-----|------|------|------|------|
| DECO-03 | L3 | `TurnExecutor` 包骨架：`ExecuteTurn(ctx, TurnInput) (TurnResult, error)` | `internal/runtime/turn/` 或 `internal/biz` 端口 + service 实现 | 单测覆盖 admit/reject/outcome；`make runtime-boundary` | ✅ |
| DECO-04 | L3 | admission · RunRegistry · PendingQueue · trace/usage 迁入 TurnExecutor | `chat_orchestrator_turn.go` 瘦身 | Orchestrator 不再内联 `DecideTurnAdmission` 大块逻辑 | ✅ |
| DECO-05 | L1→L3 | Channel 经 TurnExecutor；`NativeTurnGateway` 保持窄门面 | `chat_native.go` · Wire | Channel 仍不 import proto；ingress 无新增 ChatService 方法依赖 | ✅ |
| DECO-06 | L1→L3 | Web Chat / WS turn 经同一 TurnExecutor | `chat.go` · `ws` 投影 | Web 与 Channel cancel/enqueue 行为一致 | ✅ |
| DECO-07 | L1→L3 | Cron / A2A turn 路径对齐 TurnExecutor | `cronrunner` · `a2a` | 三入口共享单测：`TestTurnExecutor_*` | ✅ |
| DECO-15 | L3 | Team turn 以 hook 接入 TurnExecutor（`BuildRunner` / `ProjectRuntimeEvent`） | `internal/team` · orchestrator | Agent/Team cancel/status 一致；见 §6 Phase B2 | ✅ |

#### DECO-c — 抽 L2 IngressPolicy（P0–P1，衔接 CH-BOR）

| ID | 层 | 内容 | 落点 | 验收 | 状态 |
|----|-----|------|------|------|------|
| DECO-08 | L2 | `IngressDecision`：`admit \| queue \| steer \| reject_busy \| route_async` + 单测 | `internal/service/ingress_policy.go` | 纯函数可测；不 import trpc | ✅ |
| DECO-09 | L2 | Channel：`executeInboundTurn` 前 Policy 链（debounce/dedupe/intent 入口） | `channel_ingress_execute.go` | 实现后关闭对应 CH-BOR-03/05/06；busy 不误 interrupt | ✅ |
| DECO-10 | L1 | Web `enqueue_message` / busy 与 Channel 共用 Policy 结果类型 | `chat_orchestrator` admission | 同一 session Web+IM 并发时策略一致 | ✅ |

#### DECO-d — M55 双平面（P0–P1）

| ID | 层 | 内容 | 落点 | 验收 | 状态 |
|----|-----|------|------|------|------|
| DECO-11 | L2 | sync / async / durable 路由入 IngressPolicy（非 Turn 内硬编码） | `channel_config_helpers` · ingress route | 长任务关键词 → async Job；Sync 封顶 15min；见 M55 §4.1 | ✅ |
| DECO-12 | L4 | Web「后台任务」面板绑定 `ChannelTurnJob` + EventBus | `55-chat-channel-cursor-development` · `web/features/channels` | M55-JOB-01：飞书 `/async` 后 Web ≤3s 见 Job | ✅ |

#### DECO-e — L4 SessionProjection（P1–P2，衔接 CH-BOR）

| ID | 层 | 内容 | 落点 | 验收 | 状态 |
|----|-----|------|------|------|------|
| DECO-13 | L4 | biz `SessionProjection` 端口：`ListMessagesAfterRevision` | `internal/biz` + service | Web/Monitor 不读 runner 内部字段 | ✅ |
| DECO-14 | L4 | 前端 hydrate 仅经 revision + SessionProjection | `useChatInboundSync.ts` | `sync` 不误触发 turn complete；失败可观测 | ✅ |
| — | L4 | preview registry / block-final / error taxonomy | 见 **CH-BOR-07–09** | 不重复 DECO ID | ✅ |

### 14.2 推荐迭代顺序

```
DECO-a（巩固，约 3 天）
  DECO-01 E2E 验收固化 → DECO-02 确认 ✅

DECO-b（TurnExecutor，约 1–2 周）
  DECO-03 → DECO-04 → DECO-05 → DECO-06 → DECO-07

DECO-c + CH-BOR P0（约 1 周，可并行 DECO-b 后半）
  DECO-08 → DECO-09 → CH-BOR-03/01/04 → DECO-10

DECO-d（M55，约 1 周）
  DECO-11 → DECO-12

DECO-e（投影 + Team，约 1 周）
  DECO-13 → DECO-14 → DECO-15 · CH-BOR-07–09

Phase G-c（P2/P3，2026-05-24 完成）
  CH-BOR-10 → CH-BOR-11 → CH-BOR-12 → CH-BOR-13/14 ✅
```

### 14.3 验证命令

```bash
make runtime-boundary
go test ./internal/service/ -run 'TurnAdmission|ChannelIngress|TurnPreview|DECO01|NativeTurn' -count=1
go test ./internal/channel/port/... ./internal/channel/preview/... -count=1
go test ./internal/runtime/... -count=1
go test ./internal/event/... -count=1
# 前端（DECO-01/14）
cd web && pnpm test -- useChatInboundSync inboundSyncEnvelope decoP2Sync
```

### 14.4 与 CH-BOR / M55 映射

| DECO | 衔接 |
|------|------|
| DECO-09 | CH-BOR-01、03、05、06 |
| DECO-11 | M55 Phase A（CC-A 长任务路由） |
| DECO-12 | M55 Phase C/D（Job 面板） |
| DECO-13/14 | M55 session_revision · TurnBlock |
| CH-BOR-07–09 | 在 DECO-e 阶段执行，不单独开 DECO ID |
| CH-BOR-10–14 | Phase G-c：`port/meta` · context admission · sanitize · lane · durable compact |

---

## 15. 文档修订记录

| 版本 | 日期 | 说明 |
|------|------|------|
| 1.1 | 2026-05-22 | §10 Phase E 长任务：任务板 E0–E6、迭代顺序、验收映射 |
| 1.2 | 2026-05-24 | §11 Phase F Hermes 飞书借鉴：F-01–F-11 任务板 |
| 1.3 | 2026-05-24 | §13 Phase G：CH-BOR 任务板；链入外部参考借鉴手册 |
| 1.4 | 2026-05-24 | 头部链入解耦文档 §3.1 四层目标架构 |
| 1.5 | 2026-05-24 | §14 Phase DECO：四层解耦任务板 DECO-01–15 |
| 1.6 | 2026-05-24 | §13 Phase G-c：CH-BOR-10–14 ✅；DECO-01 单测；§14.3 验证命令更新 |
