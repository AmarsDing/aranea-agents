# Cron 定时任务 — 开发计划

> **版本**：2026-06-06 | **状态**：🟢 核心完成（迭代 2 已交付手动触发 + 重试表单）
> **需求**：[21 cron.md](./21%20cron.md) · **设计**：[21 cron.design.md](./21%20cron.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：EP-BIZ-09

---

## 1. 模块定位

Cron 定时任务：支持 Agent/Team/ModelRegistrySync 按计划自动执行，包括 cron 表达式 / 间隔 / 单次触发、执行历史、失败重试与死信机制。

**代码锚点**：
- `api/kratos/cron/v1/cron.proto` — CronTask CRUD + ListCronTaskRuns + TriggerCronTask + ResetCronTaskFailures
- `internal/service/cron.go` — CronService（传输桥点）
- `internal/biz/cron.go` — CronUsecase + CronTaskPatch + CronTaskRunInput
- `internal/data/cron.go` — CronRepo（Ent）
- `internal/data/ent/schema/cron_task.go` / `cron_task_run.go` — 表结构
- `internal/cronrunner/runner.go` — 调度引擎（RunCronTurn、dispatchWithRetry、dead letter）
- `internal/cronrunner/schedule.go` — config_json / metadata_json 解析与 next_run_at 计算
- `web/src/pages/CronTasksPage.vue` — 专用管理页（含执行历史弹窗入口）
- `web/src/components/cron/CronRunsDialog.vue` — 执行历史弹窗
- `web/src/features/cron/api.ts` — 前端 API 与 wire 转换
- `cmd/admin/main.go` — `CronRunner.Start`，间隔 `CRON_RUNNER_INTERVAL`（默认 1m）

---

## 2. 现状评估（2026-06-06 代码审计）

| 项 | 状态 | 证据 |
|----|------|------|
| CronTask CRUD | ✅ | `CronService` Create/Update/Delete/Get/List |
| 三种计划类型 | ✅ | `schedule.go` interval / cron / once + `next_run_at` |
| 调度引擎 | ✅ | `runner.go` 轮询 + `metadata_json.next_run_at` 到期筛选 |
| Agent / Team / ModelRegistrySync 执行 | ✅ | `RunCronTurn`（EP-RT-07）；HTTP fallback 保留；`model_registry_sync` 目标类型 |
| 执行历史 | ✅ | `cron_task_run` + `GET /v1/cron-task-runs` |
| 失败重试 | ✅ | `dispatchWithRetry` 指数退避 30s/2m/10m |
| `retry_max_attempts` | ✅ | `config_json.retry_max_attempts`；未设置默认 3，0=禁用 |
| 死信机制 | ✅ | 连续失败 ≥3 → `status=dead` + `cron.dead_letter` 事件 + Prometheus |
| 前端管理页 | ✅ | `/cron` QTable + 搜索/状态筛选 + 失败 tooltip |
| 执行历史弹窗 | ✅ | `CronRunsDialog` 弹窗，支持按任务/状态筛选 + 前端分页 |
| 重置失败计数 | ✅ | `CronTasksPage` dead 任务 `restart_alt` 按钮 |
| Wire / 启动 | ✅ | `cmd/admin/wire.go` → `provideCronRunner` |
| 手动触发 | ✅ | 异步 `POST /v1/cron-tasks/{id}/trigger` → 立即返回 `pending` run |
| `retry_max_attempts` 表单 | ✅ | 创建/编辑对话框可配置 |
| P1 并发/ skipped / GetRun | ✅ | per-task 锁、busy→skipped、`GetCronTaskRun` |
| pre-dispatch 失败统一 | ✅ | 无效 config/schedule → `failure` run + `recordScheduleFailure`（移除旧 `recordSkipped`） |
| P2 biz 触发 + manual 语义 | ✅ | `CronUsecase.TriggerTask`；manual 不改 next_run_at / once / dead |
| metadata 锁内 reload | ✅ | `finishTaskRun` dispatch 后重读 task/meta，避免 lost update |
| Trigger insert 持锁 | ✅ | `TriggerTask` 在 `lockTask` 内 insert pending run |
| P3 ResetFailures RPC | ✅ | `POST /v1/cron-tasks/{id}/reset-failures` |
| 到期任务 DB 筛选 | ❌ P4 | `next_run_at` 在 `metadata_json`，仍全表扫描后在 Go 内过滤 |
| 列表服务端分页/搜索 | ❌ P3 | 前端本地 filter；Proto 无 search/page 参数 |

---

## 3. 差距与优化（按优先级）

| 优先级 | 项 | 状态 |
|--------|-----|------|
| **P3** | ListCronTasks 服务端 search/page | ❌ 待做 |
| **P4** | 到期任务查询优化（`next_run_at` 物理列/JSON 索引） | ❌ 待做 |
| **P4** | 分布式锁（多实例防重复触发） | ❌ 待做 |

---

## 4. 开发阶段

- **Phase 1（EP-BIZ-09）**：✅ 调度引擎 + RunCronTurn + Wire 启动
- **Phase 2**：✅ 执行历史 + 失败重试 + 死信 + Prometheus
- **Phase 3**：✅ 专用前端页（`/cron`）+ 执行历史弹窗（`CronRunsDialog`）+ dead 重置
- **Phase 4（迭代 2）**：✅ 手动触发 + retry 表单 + 重试默认值修复

---

## 5. 任务清单

| # | 任务 | 优先级 | 状态 |
|---|------|--------|------|
| 1 | `cronrunner/runner.go`：Runner + dispatchWithRetry | P1 | ✅ |
| 2 | `cron_task_run` 表 + ListCronTaskRuns API | P2 | ✅ |
| 3 | 失败重试（30s/2m/10m）+ panic 恢复 | P3 | ✅ |
| 4 | 死信（≥3 连续失败）+ 指标 + 事件 | P3 | ✅ |
| 5 | Wire 注入 + `cmd/admin` 启动 | P1 | ✅ |
| 6 | 前端 CronTasksPage + CronRunsDialog | P2 | ✅ |
| 7 | CronTaskPatch 修复 Update 零值歧义 | P1 | ✅ |
| 8 | dead 任务「重置失败计数」UI | P3 | ✅ |
| 9 | `retry_max_attempts` 后端（默认 3 / 0 禁用） | P2 | ✅ |
| 10 | `TriggerCronTask` RPC + Runner.TriggerTask | P2 | ✅ |
| 11 | 表单 `retry_max_attempts` + 列表「立即执行」 | P2 | ✅ |
| 12 | ListCronTasks 服务端 search/page | P3 | ❌ |
| 13 | `next_run_at` 下推 DB 层 | P4 | ❌ |

---

## 6. 验收标准

- [x] Cron 任务按 interval/cron/once 自动执行
- [x] 执行历史可查询（含 trigger、run_id）
- [x] `go test ./internal/cronrunner/...` 通过
- [x] 失败重试 + 死信 + 前端 dead 重置
- [x] 前端 CRUD + 执行历史 + 失败 tooltip 跳转
- [x] `retry_max_attempts`：未设置=3 次退避，0=不重试
- [x] `POST /v1/cron-tasks/{id}/trigger` 可手动执行
- [x] 表单可编辑 `retry_max_attempts`

---

## 7. 依赖与风险

- 执行路径与 Chat/Team 共用 `RunGateway`（`RunCronTurn` → `RunNativeTurnUnary`）
- 单实例 `TryLock`；多实例可能重复触发（P4 分布式锁未做）
- `next_run_at` 存于 `metadata_json`，到期筛选在进程内完成
- 环境变量：`CRON_RUNNER_INTERVAL`（默认 1m）、`CRON_RUNNER_DISABLED=1` 关闭调度
