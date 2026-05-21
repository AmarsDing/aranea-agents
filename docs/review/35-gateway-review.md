# 35 Gateway / RunRegistry Review

> **评分**：83 / 100 | **风险等级**：P1  
> **文档**：[35 gateway.md](../需求/35%20gateway.md) · [35-gateway-development.md](../需求/35-gateway-development.md)  
> **代码锚点**：`internal/runtime/run_registry.go` · `internal/runtime/runner_manager.go` · `internal/service/gateway.go` · `internal/biz/webhook.go`  
> **审查时间**：2026-05-21

---

## 评分详情

| 维度 | 得分 | 满分 | 评述 |
|------|------|------|------|
| 需求符合度 | 17 | 20 | RunRegistry/ChatUsecase/Webhook CRUD 均已落地；Follow-up Queue Phase 1.5 前端 UX 待补 |
| 架构一致性 | 22 | 25 | Chat/Team/Cron/Channel 共用 RunGateway 接口 ✅；运行控制统一到 runtime 层 ✅；`setRunStatus` 与 `ChatUsecase.SetRunStatus` 有轻微双路径 |
| 后端实现质量 | 18 | 20 | RunRegistry 会话级 cancel/status/enqueue/artifact/ingest 全部落地；出站 Webhook HMAC-SHA256 签名 ✅ |
| 前端实现质量 | 13 | 15 | Gateway Webhook CRUD 为 API-only（无管理页，文档已说明）；Follow-up Queue 前端展示 ✅ 但 Cursor 式连续发送 UX 待补 |
| 测试与验证 | 7 | 10 | Webhook CRUD 有 HTTP 测试；`chat_cancel_run_test.go` ✅；Follow-up Queue E2E 测试待补 |
| 文档一致性 | 6 | 10 | 35-gateway-development.md 已同步 Phase 3 Webhook；ChatUsecase 接入 changelog 已更新 |

---

## 模块定位

Gateway 是运行控制的核心枢纽，负责：
- `RunRegistry`：每个会话的 active run 注册（cancel / status / enqueue / artifact / ingest）
- `RunnerManager`：统一 Runner 装配入口（ArtifactService / SessionIngestor / AgentFactory / AwaitUserReplyRouting 注入）
- `RunGateway` 接口：Chat/Team/Cron/Channel 共用运行入口
- `ChatUsecase`：Follow-up Queue 编排（EnqueueUserMessage / Pending CRUD）
- 出站 Webhook：运行终态（completed/failed/cancelled）HMAC 回调

---

## 已验收功能

| 功能 | 状态 |
|------|------|
| RunRegistry（cancel/status/enqueue/artifact/ingest） | ✅ M1 |
| RunnerManager（统一 Runner 装配） | ✅ M1 |
| ChatUsecase 接入 ChatService | ✅ |
| Chat/Team/Cron/Channel 共用 RunGateway | ✅ |
| EnqueueUserMessage（Follow-up Queue） | ✅ |
| StopGeneration → `run_status` cancelled | ✅ |
| 出站 Webhook CRUD（POST/GET/PUT/DELETE） | ✅ Phase 3 |
| Webhook HMAC-SHA256 终态回调 | ✅ Phase 3 |
| ArtifactService / SessionIngestor 注入 | ✅ |
| AgentFactory / AwaitUserReplyRouting 注入 | ✅ |

---

## 主要风险

### P1

| ID | 问题 | 建议修复 |
|----|------|---------|
| GW-P1-01 | `setRunStatus` 在 Service 层和 `ChatUsecase.SetRunStatus` 在 biz 层形成两条状态更新路径 | 统一为一条路径；推荐经 EventBus `run_status` Envelope 触发 |
| GW-P1-02 | Follow-up Queue Phase 1.5 前端 UX（运行中连续发送、WS 驱动刷新）待补 | 实现 `useChatSender` 改进 + `message_queued` hint 监听 |
| GW-P1-03 | Webhook 出站 UI（无管理页）——需要 API 用户手动 REST 管理——对非技术用户不友好 | 规划 Webhook 管理页或在 Agent 设置页内嵌 |

### P2

| ID | 问题 | 建议修复 |
|----|------|---------|
| GW-P2-01 | `ManagedRunner.Cancel` 未统一写 RunRegistry 终态（cancelled） | 确保 Cancel 路径经 `publishRunStatus` 写库 |
| GW-P2-02 | Follow-up Queue E2E 测试（从 WS enqueue → Pending → dequeue → 新 turn）缺失 | 补 E2E 测试 |

---

## 建议优化路径

1. 统一 `setRunStatus` 双路径。
2. 补 Follow-up Queue E2E 测试。
3. 规划 Webhook 管理页（至少在 Agent 设置页内嵌 Webhook 配置）。
