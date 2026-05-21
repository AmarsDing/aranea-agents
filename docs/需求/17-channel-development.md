# Channel 渠道 — 开发计划

> **版本**：2026-05-21 | **状态**：🟢 飞书/钉钉/企微/Slack/Telegram Webhook 入站；异步投递 worker；前端 Webhook 复制
> **需求**：[17 channel.md](./17%20channel.md) · **设计**：[17 channel.design.md](./17%20channel.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：EP-BIZ-08

---

## 1. 模块定位

Channel 渠道管理：管理多平台消息渠道（飞书/微信/钉钉/Slack 等），包括渠道配置、凭据管理、Webhook 接收、消息投递。

**代码锚点**：
- `api/kratos/channel/v1/` — Channel CRUD RPC
- `internal/service/channel.go` — ChannelService
- `internal/service/channel_ingress*.go` — Webhook 入站
- `internal/service/channel_delivery_worker.go` — 异步出站投递
- `internal/biz/channel.go` / `channel_delivery.go` — Usecase
- `internal/channel/{lark,dingtalk,wecom,slack,telegram}/` — 平台适配器
- `internal/cronrunner/jobs/channel_delivery.go` — 投递 worker（5s）
- `internal/cronrunner/jobs/channel_health.go` — 连接状态（10min）

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| Channel CRUD | ✅ | Create/Update/Delete/Get/List/Toggle |
| 渠道目录 | ✅ | `ListChannelCatalog` |
| 凭据管理 | ✅ | `enc:` / `env:` |
| Webhook 接收 | ✅ | `POST /webhooks/{channel_key}` |
| 平台适配器 | 🟡 | feishu / dingtalk / wecom / **slack / telegram** |
| 异步投递 | ✅ | `EnqueueOutboundDelivery` + worker 重试（最多 3 次） |
| 连接测试 | ✅ | 飞书 token / Slack auth.test / Telegram getMe |
| 连接状态检测 | ✅ | `ChannelHealthScanner` |
| 前端 Webhook 复制 | ✅ | 列表操作列 + 编辑页只读 URL |
| WhatsApp 等 | ❌ | catalog 有定义，无入站/出站 |

---

## 3. 差距与优化

1. **P2**：WhatsApp / Discord 等平台 Webhook 入站与出站适配器。
2. **P3**：投递 dead-letter 与监控指标（Prometheus）。
3. **P3**：列表页平台图标资源映射（`metadata_json.icon_url`）。

---

## 4. 任务清单（近期完成）

| # | 任务 | 状态 |
|---|------|------|
| 8 | Slack / Telegram 适配器 | ✅ |
| 9 | 异步投递 worker + 幂等键 | ✅ |
| 10 | 前端「复制 Webhook URL」+ 外部 ID 列 | ✅ |

---

## 5. 验收标准

- [x] Slack / Telegram Webhook 可触发 Agent 对话并异步回复
- [x] `channel_delivery` pending/retry 由 worker 消费
- [x] 列表可复制 Webhook URL、展示 external_id
- [x] `go test ./internal/channel/... ./internal/biz/... -run 'TestChannel|TestParse|TestVerify'` 通过

---

## 6. 运维

- `CHANNEL_DELIVERY_DISABLED=1` — 关闭异步投递 worker（回退为仅入队不发送）
- `CHANNEL_HEALTH_DISABLED=1` — 关闭连接状态扫描
- Webhook 路径默认 `/webhooks/{channel_key}`，可在 `config_json.webhook.path` 覆盖
