# Callback 回调 — 开发计划

> **版本**：2026-05-17 | **状态**：🟡 基础 CRUD 可用；❌ 回调投递未实现
> **需求**：[28 callback.md](./28%20callback.md) · **设计**：[28 callback.design.md](./28%20callback.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：—

---

## 1. 模块定位

Callback 回调：管理 Agent/Team 运行完成后的回调通知，支持 Webhook 回调、回调重试和回调日志。

**代码锚点**：
- `api/kratos/callback/v1/` — Callback CRUD RPC
- `internal/service/callback.go` — CallbackService
- `internal/biz/callback.go` — CallbackUsecase
- `internal/data/callback.go` — CallbackRepo

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| Callback CRUD | ✅ | Create/Update/Delete/Get/List |
| 回调投递 | ❌ | 无 Webhook 投递 worker |
| 回调重试 | ❌ | 无重试逻辑 |
| 回调日志 | ❌ | 无 `callback_delivery` 表 |
| 签名验证 | ❌ | 无 HMAC 签名 |

---

## 3. 差距与优化

1. **P1**：回调投递未实现，Callback 仅存储不执行。
2. **P2**：无回调重试机制，投递失败后无法自动重试。
3. **P2**：无回调日志，无法追溯投递历史。
4. **P3**：无 HMAC 签名，接收方无法验证回调真实性。

---

## 4. 开发阶段

- **Phase 1**：回调投递 worker（HTTP POST + 异步队列）
- **Phase 2**：回调重试（指数退避）+ 回调日志
- **Phase 3**：HMAC 签名验证

---

## 5. 任务清单

| # | 任务 | 优先级 | EP |
|---|------|--------|-----|
| 1 | `internal/callback/worker.go`：HTTP POST 投递 | P1 | — |
| 2 | `callback_delivery` Ent 表 + 投递历史查询 API | P2 | — |
| 3 | 回调重试（指数退避，最多 5 次） | P2 | — |
| 4 | HMAC-SHA256 签名 | P3 | — |
| 5 | Wire 注入 Worker 到启动流程 | P1 | — |

---

## 6. 验收标准

- [ ] Agent/Team 运行完成后自动投递回调
- [ ] 投递失败后自动重试
- [ ] 回调日志可查询
- [ ] `go test ./internal/callback/...` 通过

---

## 7. 依赖与风险

- 回调投递需考虑目标服务不可用的情况
- 需防止回调风暴（限流）
