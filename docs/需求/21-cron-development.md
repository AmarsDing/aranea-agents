# Cron 定时任务 — 开发计划

> **版本**：2026-05-18 | **状态**：🟢 调度引擎已实现；前端管理页已完成
> **需求**：[21 cron.md](./21%20cron.md) · **设计**：[21 cron.design.md](./21%20cron.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：EP-BIZ-09

---

## 1. 模块定位

Cron 定时任务：支持 Agent/Team 按计划自动执行，包括 cron 表达式调度、执行历史、失败重试与死信机制。

**代码锚点**：
- `api/kratos/cron/v1/` — CronJob CRUD RPC
- `internal/service/cron.go` — CronService
- `internal/biz/cron.go` — CronUsecase + CronTaskPatch + CronTaskRunInput
- `internal/data/cron.go` — CronRepo
- `internal/data/ent/schema/cron_task.go` — Ent Schema（任务表）
- `internal/data/ent/schema/cron_task_run.go` — Ent Schema（执行记录表）
- `internal/cronrunner/runner.go` — 调度引擎（Runner + dispatchWithRetry + dead letter）

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| CronJob CRUD | ✅ | Create/Update/Delete/Get/List |
| Cron 表达式 | ✅ | `cron_expression` 字段 |
| 调度引擎 | ✅ | `internal/cronrunner/runner.go` — 15s 轮询 + next_run_at 过滤 |
| 执行历史 | ✅ | `cron_task_run` 表 + `ListCronTaskRuns` API |
| 失败重试 | ✅ | `dispatchWithRetry` 指数退避 30s/2m/10m |
| 死信机制 | ✅ | 连续失败 ≥3 次进入 `dead` 状态，Prometheus 指标 |
| 前端管理页 | ✅ | `CronTasksPage.vue` + `CronRunsPage.vue` |
| Wire 注入 | ✅ | `cronrunner.ProviderSet → NewRunner` |

---

## 3. 差距与优化

1. **P2**：`retry_max_attempts` 配置尚未实现——当前重试次数硬编码为 3 次，无法按任务自定义。
2. **P3**：前端缺少「重置失败计数」按钮——需求文档 §7.5 要求提供 UI 重置 `dead` 任务。
3. **P4**：`next_run_at <= now` 筛选在 Go 进程内完成，可优化为数据库层查询以减少内存开销。

---

## 4. 开发阶段

- **Phase 1（EP-BIZ-09）**：✅ 调度引擎（cronrunner + safego + dispatchWithRetry）
- **Phase 2**：✅ 执行历史记录（cron_task_run 表 + API）
- **Phase 3**：✅ 失败重试 + 死信 + Prometheus 指标

---

## 5. 任务清单

| # | 任务 | 优先级 | EP | 状态 |
|---|------|--------|-----|------|
| 1 | `internal/cronrunner/runner.go`：Runner + dispatchWithRetry | P1 | EP-BIZ-09 | ✅ |
| 2 | `cron_task_run` Ent 表 + 执行历史查询 API | P2 | — | ✅ |
| 3 | 失败重试（指数退避 30s/2m/10m） | P3 | — | ✅ |
| 4 | 死信机制（连续失败 ≥3 → dead） | P3 | — | ✅ |
| 5 | Wire 注入 Runner 到启动流程 | P1 | EP-BIZ-09 | ✅ |
| 6 | 前端 CronTasksPage + CronRunsPage | P2 | — | ✅ |
| 7 | `CronTaskPatch` 结构体修复 UpdateTask 零值歧义 | P1 | — | ✅ |
| 8 | `CronTaskRunInput` 结构体重构 InsertCronTaskRun | P2 | — | ✅ |
| 9 | `retry_max_attempts` 按任务自定义重试次数 | P2 | — | ❌ |
| 10 | 前端「重置失败计数」按钮 | P3 | — | ❌ |
| 11 | `next_run_at` 筛选下推到数据库层 | P4 | — | ❌ |

---

## 6. 验收标准

- [x] CronJob 创建后按 cron 表达式自动执行
- [x] 执行历史可查询
- [x] `go test ./internal/cronrunner/...` 通过
- [x] 失败重试 + 死信机制工作正常
- [x] 前端管理页可 CRUD + 查看执行历史
- [ ] `retry_max_attempts` 可按任务配置
- [ ] 前端可重置 dead 任务

---

## 7. 依赖与风险

- 调度引擎已与 Chat/Team 对话流程集成（通过 `ChatService.Send`）
- 单实例调度，分布式场景下需考虑调度一致性（当前未做分布式锁）
- `metadata_json` 中 `next_run_at` 的计算在 Go 进程内完成，多实例部署时可能重复触发
