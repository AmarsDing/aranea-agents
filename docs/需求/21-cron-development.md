# Cron 定时任务 — 开发计划

> **版本**：2026-05-17 | **状态**：🟡 基础 CRUD 可用；❌ 调度引擎未实现
> **需求**：[21 cron.md](./21%20cron.md) · **设计**：[21 cron.design.md](./21%20cron.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：EP-BIZ-09

---

## 1. 模块定位

Cron 定时任务：支持 Agent/Team 按计划自动执行，包括 cron 表达式调度、执行历史、失败重试。

**代码锚点**：
- `api/kratos/cron/v1/` — CronJob CRUD RPC
- `internal/service/cron.go` — CronService
- `internal/biz/cron.go` — CronUsecase
- `internal/data/cron.go` — CronRepo
- `internal/data/ent/schema/cron_job.go` — Ent Schema

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| CronJob CRUD | ✅ | Create/Update/Delete/Get/List |
| Cron 表达式 | ✅ | `cron_expression` 字段 |
| 调度引擎 | ❌ | 无 cron scheduler worker |
| 执行历史 | ❌ | 无 `cron_execution` 表 |
| 失败重试 | ❌ | 无重试逻辑 |

---

## 3. 差距与优化

1. **P1（EP-BIZ-09）**：调度引擎未实现，CronJob 仅存储不执行。
2. **P2**：无执行历史记录，无法追溯定时任务执行情况。
3. **P3**：无失败重试机制。

---

## 4. 开发阶段

- **Phase 1（EP-BIZ-09）**：调度引擎（robfig/cron + safego）
- **Phase 2**：执行历史记录
- **Phase 3**：失败重试 + 告警

---

## 5. 任务清单

| # | 任务 | 优先级 | EP |
|---|------|--------|-----|
| 1 | `internal/cron/scheduler.go`：robfig/cron + safego | P1 | EP-BIZ-09 |
| 2 | CronJob CRUD 时注册/注销调度 | P1 | EP-BIZ-09 |
| 3 | `cron_execution` Ent 表 + 执行历史查询 API | P2 | — |
| 4 | 失败重试（指数退避） | P3 | — |
| 5 | Wire 注入 Scheduler 到启动流程 | P1 | EP-BIZ-09 |

---

## 6. 验收标准

- [ ] CronJob 创建后按 cron 表达式自动执行
- [ ] 执行历史可查询
- [ ] `go test ./internal/cron/...` 通过

---

## 7. 依赖与风险

- 调度引擎需与 Chat/Team 对话流程集成
- 需考虑分布式场景下的调度一致性
