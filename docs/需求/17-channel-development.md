# Channel 渠道 — 开发计划

> **版本**：2026-05-17 | **状态**：🟡 基础 CRUD 可用；❌ Webhook/消息投递未实现
> **需求**：[17 channel.md](./17%20channel.md) · **设计**：[17 channel.design.md](./17%20channel.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：EP-BIZ-08

---

## 1. 模块定位

Channel 渠道管理：管理多平台消息渠道（飞书/微信/钉钉/Slack 等），包括渠道配置、凭据管理、Webhook 接收、消息投递。

**代码锚点**：
- `api/kratos/channel/v1/` — Channel CRUD RPC
- `internal/service/channel.go` — ChannelService
- `internal/biz/channel.go` — ChannelUsecase
- `internal/data/channel.go` — ChannelRepo
- `internal/data/ent/schema/channel.go` — Ent Schema

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| Channel CRUD | ✅ | Create/Update/Delete/Get/List |
| 渠道类型 | ✅ | `config_json.type` 字段 |
| 凭据管理 | ✅ | `channel_credential` 关联表 |
| Webhook 接收 | ❌ | 无 Webhook 接收端点 |
| 消息投递 | ❌ | 无 `channel_delivery` 实现 |
| 平台适配器 | ❌ | 无各平台 SDK 集成 |
| 连接状态检测 | ❌ | 无健康检查 |

---

## 3. 差距与优化

1. **P1（EP-BIZ-08）**：Webhook 接收端点未实现，外部平台消息无法进入系统。
2. **P1（EP-BIZ-08）**：消息投递未实现，Agent 回复无法推送到外部平台。
3. **P2**：各平台适配器（飞书/微信/钉钉/Slack）未实现，仅有数据模型。
4. **P2**：渠道凭据未加密存储。
5. **P3**：渠道连接状态检测未实现。

---

## 4. 开发阶段

- **Phase 1（EP-BIZ-08）**：Webhook 接收端点 + 消息路由
- **Phase 2（EP-BIZ-08）**：消息投递框架 + 飞书适配器
- **Phase 3**：微信/钉钉/Slack 适配器
- **Phase 4**：凭据加密 + 连接状态检测

---

## 5. 任务清单

| # | 任务 | 优先级 | EP |
|---|------|--------|-----|
| 1 | Webhook 接收端点：`POST /api/v1/channels/:id/webhook` | P1 | EP-BIZ-08 |
| 2 | 消息路由：Webhook → Channel → Agent/Team | P1 | EP-BIZ-08 |
| 3 | 消息投递框架：`channel_delivery` 表 + 投递 worker | P1 | EP-BIZ-08 |
| 4 | 飞书适配器：事件订阅 + 消息发送 | P2 | EP-BIZ-08 |
| 5 | 微信/钉钉/Slack 适配器 | P2 | — |
| 6 | 凭据加密存储 | P2 | — |
| 7 | 连接状态检测定时任务 | P3 | — |

---

## 6. 验收标准

- [ ] 飞书 Webhook 消息可进入系统并触发 Agent 对话
- [ ] Agent 回复可推送到飞书
- [ ] 渠道列表可显示连接状态
- [ ] `go test ./internal/biz/... -run TestChannel` 通过

---

## 7. 依赖与风险

- 各平台 SDK 需单独集成，维护成本高
- Webhook 安全性需验证签名
- 消息投递需考虑重试和幂等
