# Channel 渠道 — 开发计划

> **版本**：2026-05-22 | **状态**：🟢 9 平台连接；Runtime 生产级重连 + 流式出站 MVP  
> **需求**：[17 channel.md](./17%20channel.md) · **设计**：[17 channel.design.md](./17%20channel.design.md) · **业务集成**：[17-channel-agent-team-integration.md](./17-channel-agent-team-integration.md)  
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
| D3 | 路由 UI：Team / dm_scope / rules | 🟡 | Agent/Team 下拉 ✅；`dm_scope` 下拉 ✅；rules 表 ⏳ |
| D5 | Web Chat 同步 Channel 入站 | ✅ | `useChatInboundSync` + session `metadata_json.source=channel` |
| D6 | 路由变更重置 peer 绑定 | ✅ | `UpdateChannel` + `DeleteByChannelID` |
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

## 11. 文档修订记录

| 版本 | 日期 | 说明 |
|------|------|------|
| 1.1 | 2026-05-22 | §10 Phase E 长任务：任务板 E0–E6、迭代顺序、验收映射 |
