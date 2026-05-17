# Token 用量 — 开发计划

> **版本**：2026-05-17 | **状态**：✅ 端到端可用
> **需求**：[29 token.md](./29%20token.md) · **设计**：[29 token.design.md](./29%20token.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：—

---

## 1. 模块定位

Token 用量管理：记录和统计 Agent/Team 运行的 Token 消耗，支持按时间/Agent/Provider 维度查询和聚合。

**代码锚点**：
- `internal/biz/usage.go` — UsageUsecase
- `internal/data/usage.go` — UsageRepo
- `internal/service/chat_usage_ingress.go` — 用量记录入口
- `internal/data/ent/schema/usage.go` — Ent Schema

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| Token 用量记录 | ✅ | `chat_usage_ingress.go` |
| 用量查询 API | ✅ | `GetUsage` / `ListUsage` RPC |
| 按 Agent 聚合 | ✅ | `agent_id` 字段 |
| 按 Provider 聚合 | ✅ | `provider` 字段 |
| 用量限额 | ❌ | 无 quota 限制 |
| 用量告警 | ❌ | 无阈值告警 |

---

## 3. 差距与优化

1. **P2**：无 Token 用量限额，用户可能产生意外高额费用。
2. **P3**：无用量告警，达到阈值时无法通知用户。

---

## 4. 开发阶段

- **Phase 1**：Token 用量限额（quota 配置 + 超限拦截）
- **Phase 2**：用量告警（阈值配置 + 通知）

---

## 5. 任务清单

| # | 任务 | 优先级 | EP |
|---|------|--------|-----|
| 1 | `usage_quota` 表 + 配置 API | P2 | — |
| 2 | Chat turn 前检查 quota | P2 | — |
| 3 | 用量告警阈值配置 + 通知 | P3 | — |

---

## 6. 验收标准

- [ ] 超过 quota 后 Agent 对话被拦截并提示
- [ ] 达到告警阈值时通知用户

---

## 7. 依赖与风险

- Quota 检查需在 Chat turn 前执行，增加延迟
- 需考虑 quota 重置周期（月/周/日）
