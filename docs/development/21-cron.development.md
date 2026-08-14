# Cron 定时任务 — 开发计划

> **版本**：2026-06-17 | **状态**：🟢 核心完成（迭代 2 已交付手动触发 + 重试表单）
> **需求**：[21 cron.md](./21-cron.md) · **设计**：[21-cron.design.md](./21-cron.design.md)
> **EP**：EP-BIZ-09

> **内容边界**：本文档描述模块定位、代码锚点、现状评估、差距优化、Phase 划分、任务清单（含状态）、验收标准、改动文件清单。
> - 用户故事 / 验收标准 / 交互规格 → 见 [21-cron.md](./21-cron.md)
> - 架构 / Proto 契约 / 数据模型 / 接口定义 → 见 [21-cron.design.md](./21-cron.design.md)

---

## 1. 模块定位

Cron 定时任务：支持 Agent/Team/ModelRegistrySync 按计划自动执行，包括 cron 表达式 / 间隔 / 单次触发、执行历史、失败重试与死信机制。

**核心能力**：
- 三种计划类型（interval/cron/once）自动调度
- Agent / Team / ModelRegistrySync 三种目标类型
- 执行历史记录与查询
- 失败重试（指数退避 30s/2m/10m）+ 死信保护（连续 3 次失败停止调度）
- 手动触发 + 重置失败计数
- Prometheus 指标 + 死信告警事件

---

## 2. 代码锚点

### 2.1 后端

| 文件 | 职责 |
|------|------|
| `api/kratos/cron/v1/cron.proto` | Proto 契约：CronTask CRUD + ListCronTaskRuns + TriggerCronTask + ResetCronTaskFailures |
| `internal/service/cron.go` | CronService（传输桥点，实现 v1.CronServiceServer） |
| `internal/service/cron_trigger_gateway_adapter.go` | CronTriggerGateway 适配器（桥接 gateway 调用到 CronService.uc） |
| `internal/service/cron_mapping_test.go` | Proto ↔ Biz 类型转换测试 |
| `internal/service/cron_test.go` | CronService 单元测试 |
| `internal/biz/cron.go` | 类型 alias 文件（CronTask / CronRepo / CronUsecase 等到 cron 子包） |
| `internal/biz/cron/cron.go` | Usecase + Repo 接口 + 领域模型 + ValidateTaskConfig + ResetFailureMetadata |
| `internal/biz/cron/cron_test.go` | Usecase 单元测试 |
| `internal/biz/cron/cron_usecase_test.go` | Usecase 补充测试 |
| `internal/biz/cron/usecase_test.go` | Usecase 补充测试 |
| `internal/biz/cron_metadata_test.go` | metadata_json 解析测试 |
| `internal/data/cron.go` | CronRepo（Ent 实现） |
| `internal/data/ent/schema/cron_task.go` | Ent Schema（cron_task 表） |
| `internal/data/ent/schema/cron_task_run.go` | Ent Schema（cron_task_run 表） |
| `internal/cronrunner/runner.go` | Runner 主循环 + dispatch + TriggerTask + Prometheus 指标 |
| `internal/cronrunner/execute.go` | executeTask / finalizeRun / lockTask / recordScheduleFailure |
| `internal/cronrunner/schedule.go` | config_json / metadata_json 解析 + next_run_at 计算 + retryPlan |
| `internal/cronrunner/errors.go` | schedule 解析错误（errRunAtRequired 等） |
| `internal/cronrunner/seed_model_registry.go` | SeedModelRegistrySyncTask（注入模型注册表同步种子任务） |
| `internal/cronrunner/jobs/channel_delivery.go` | Channel 出站投递扫描；多实例 `ClaimPendingDeliveries`（SKIP LOCKED） |
| `internal/cronrunner/jobs/channel_turn_job_sweeper.go` | Channel TurnJob 卡死恢复；`TransitionIfStale` + running 租约 |
| `internal/cronrunner/dispatch_test.go` | dispatch 单元测试 |
| `internal/cronrunner/execute_test.go` | execute 单元测试 |
| `internal/cronrunner/retry_test.go` | retry 单元测试 |
| `cmd/admin/wire.go` | provideCronRunnerDeps + provideCronRunner（CRON_RUNNER_DISABLED=1 返回 nil） |
| `cmd/admin/workers.go` | startBackgroundWorkers → CronRunner.Start |
| `cmd/admin/main.go` | 注入 CronRunner 到 backgroundWorkersConfig |

### 2.2 前端

| 文件 | 职责 |
|------|------|
| `web/src/router/routes.ts` | `/cron` 路由 → CronTasksPage |
| `web/src/pages/CronTasksPage.vue` | 定时任务管理主页（含执行历史弹窗入口） |
| `web/src/components/cron/CronTaskFormDialog.vue` | 创建/编辑对话框 |
| `web/src/components/cron/CronTaskFormFields.vue` | 表单字段（主容器） |
| `web/src/components/cron/CronTaskFormTargetFields.vue` | 目标类型选择（Agent/Team） |
| `web/src/components/cron/CronTaskFormScheduleFields.vue` | 计划类型选择（interval/cron/once） |
| `web/src/components/cron/CronRunsDialog.vue` | 执行历史弹窗 |
| `web/src/components/cron/cronTaskUtils.ts` | 表单工具函数 |
| `web/src/features/cron/api.ts` | 前端 API 与 wire 转换 |
| `web/src/features/cron/types.ts` | TypeScript 类型定义 |
| `web/src/features/cron/useCronTasksPage.ts` | 列表页 Composable |
| `web/src/features/cron/cronTableUi.ts` | 表格 UI 辅助 |
| `web/src/features/cron/__tests__/cron-types.spec.ts` | 类型测试 |

### 2.3 配置与环境变量

| 变量 | 默认 | 说明 |
|------|------|------|
| `CRON_RUNNER_INTERVAL` | `1m` | 调度器 tick 间隔；<=0 或解析失败回退 1m |
| `CRON_RUNNER_DISABLED` | （未设置） | `=1` 时不创建 Runner（provideCronRunner 返回 nil） |
| `CRON_CHAT_DISPATCH_ORIGIN` | （未设置） | HTTP fallback origin（指向 admin `/v1/chat/messages`）；未设置且 `Chat` 依赖为 nil 时 dispatch 失败 |

---

## 3. 现状评估（2026-06-17 代码审计）

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
| Wire / 启动 | ✅ | `cmd/admin/wire.go` → `provideCronRunner`；`workers.go` → `CronRunner.Start` |
| 手动触发 | ✅ | 异步 `POST /v1/cron-tasks/{id}/trigger` → 立即返回 `pending` run |
| `retry_max_attempts` 表单 | ✅ | 创建/编辑对话框可配置 |
| P1 并发/ skipped / GetRun | ✅ | per-task 锁、busy→skipped、`GetCronTaskRun` |
| pre-dispatch 失败统一 | ✅ | 无效 config/schedule → `failure` run + `recordScheduleFailure`（移除旧 `recordSkipped`） |
| P2 biz 触发 + manual 语义 | ✅ | `CronUsecase.TriggerTask`；manual 不改 next_run_at / once / dead |
| metadata 锁内 reload | ✅ | `finishTaskRun` dispatch 后重读 task/meta，避免 lost update |
| Trigger insert 持锁 | ✅ | `TriggerTask` 在 `lockTask` 内 insert pending run |
| P3 ResetFailures RPC | ✅ | `POST /v1/cron-tasks/{id}/reset-failures` |
| CronTriggerGateway 适配器 | ✅ | `cron_trigger_gateway_adapter.go` 实现 `biz.CronTriggerGateway` |
| ModelRegistrySync 种子任务 | ✅ | `seed_model_registry.go` + `biz.SeedModelRegistryCronTask` |
| 到期任务 DB 筛选 | ❌ P4 | `next_run_at` 在 `metadata_json`，仍全表扫描后在 Go 内过滤 |
| 列表服务端分页/搜索 | ❌ P3 | 前端本地 filter；Proto 无 search/page 参数 |
| 多租户隔离 | ❌ P3 | 当前未强制 `tenant_id` 隔离 |
| 分布式锁 | ❌ P4 | 单实例 `TryLock`；多实例可能重复触发 |
| Data 层错误翻译 | ⚠️ 技术债 | `internal/data/cron.go` 直接返回 `biz.ErrCronNotFound`，未使用 `entErrToBizErr`（DB-DEBT-04） |

---

## 4. 差距与优化（按优先级）

| 优先级 | 项 | 状态 | 说明 |
|--------|-----|------|------|
| **P3** | ListCronTasks 服务端 search/page | ❌ 待做 | Proto 增加搜索/分页参数；Data 层 Ent 查询支持；前端切换服务端分页 |
| **P3** | 多租户隔离 | ❌ 待做 | Schema 增加 `tenant_id`；Repo 查询过滤；Usecase 注入 tenant context |
| **P4** | 到期任务查询优化 | ❌ 待做 | `next_run_at` 物理列或 JSON 索引；DB 层筛选到期任务 |
| **P4** | 分布式锁 | ❌ 待做 | 多实例部署时使用 Redis/DB 分布式锁防重复触发 |
| **技术债** | Data 层错误翻译 | ⚠️ 待重构 | `internal/data/cron.go` 错误处理改用 `entErrToBizErr`（DB-DEBT-04） |

---

## 5. 开发阶段

- **Phase 1（EP-BIZ-09）**：✅ 调度引擎 + RunCronTurn + Wire 启动
- **Phase 2**：✅ 执行历史 + 失败重试 + 死信 + Prometheus
- **Phase 3**：✅ 专用前端页（`/cron`）+ 执行历史弹窗（`CronRunsDialog`）+ dead 重置
- **Phase 4（迭代 2）**：✅ 手动触发 + retry 表单 + 重试默认值修复

---

## 6. 任务清单

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
| 12 | `CronTriggerGateway` 适配器 | P2 | ✅ |
| 13 | `SeedModelRegistrySyncTask` 种子任务 | P2 | ✅ |
| 14 | ListCronTasks 服务端 search/page | P3 | ❌ |
| 15 | 多租户隔离 | P3 | ❌ |
| 16 | `next_run_at` 下推 DB 层 | P4 | ❌ |
| 17 | 分布式锁（多实例防重复触发） | P4 | ❌ |
| 18 | Data 层错误翻译改用 `entErrToBizErr` | 技术债 | ❌ |

---

## 7. 验收标准

- [x] Cron 任务按 interval/cron/once 自动执行
- [x] 执行历史可查询（含 trigger、run_id）
- [x] `go test ./internal/cronrunner/...` 通过
- [x] 失败重试 + 死信 + 前端 dead 重置
- [x] 前端 CRUD + 执行历史 + 失败 tooltip 跳转
- [x] `retry_max_attempts`：未设置=3 次退避，0=不重试
- [x] `POST /v1/cron-tasks/{id}/trigger` 可手动执行
- [x] 表单可编辑 `retry_max_attempts`
- [x] `POST /v1/cron-tasks/{id}/reset-failures` 可重置死信任务
- [x] ModelRegistrySync 种子任务自动注入
- [ ] ListCronTasks 服务端 search/page（P3 待做）
- [ ] 多租户隔离（P3 待做）
- [ ] `next_run_at` DB 层筛选（P4 待做）
- [ ] 分布式锁（P4 待做）

---

## 8. 改动文件清单

### 8.1 已交付（Phase 1-4）

**后端**：
- `api/kratos/cron/v1/cron.proto` — Proto 契约
- `internal/service/cron.go` — CronService
- `internal/service/cron_trigger_gateway_adapter.go` — Gateway 适配器
- `internal/service/cron_mapping_test.go` / `cron_test.go` — 测试
- `internal/biz/cron.go` — alias 文件
- `internal/biz/cron/cron.go` — Usecase + Repo 接口 + 领域模型
- `internal/biz/cron/*_test.go` — 测试
- `internal/biz/cron_metadata_test.go` — 测试
- `internal/data/cron.go` — CronRepo
- `internal/data/ent/schema/cron_task.go` / `cron_task_run.go` — Ent Schema
- `internal/cronrunner/runner.go` / `execute.go` / `schedule.go` / `errors.go` / `seed_model_registry.go` — 调度器
- `internal/cronrunner/*_test.go` — 测试
- `cmd/admin/wire.go` — Wire 注入
- `cmd/admin/workers.go` — 启动
- `cmd/admin/main.go` — 配置装配

**前端**：
- `web/src/router/routes.ts` — `/cron` 路由
- `web/src/pages/CronTasksPage.vue` — 管理主页
- `web/src/components/cron/*.vue` / `cronTaskUtils.ts` — 组件
- `web/src/features/cron/api.ts` / `types.ts` / `useCronTasksPage.ts` / `cronTableUi.ts` — feature 模块
- `web/src/features/cron/__tests__/cron-types.spec.ts` — 测试

### 8.2 待交付（P3-P4）

- Proto 增加 search/page 参数 → `api/kratos/cron/v1/cron.proto` + 生成物
- Data 层查询支持 search/page → `internal/data/cron.go`
- 多租户 Schema 迁移 → `internal/data/ent/schema/cron_task.go` + DDL 迁移
- `next_run_at` 物理列 → `internal/data/ent/schema/cron_task.go` + DDL 迁移 + `internal/cronrunner/runner.go` 查询优化
- 分布式锁 → `internal/cronrunner/runner.go` + 锁实现
- Data 层错误翻译重构 → `internal/data/cron.go`

---

## 9. 依赖与风险

- **执行路径依赖**：与 Chat/Team 共用 `RunGateway`（`RunCronTurn` → `RunNativeTurnUnary`）
- **并发风险**：单实例 `TryLock`；多实例可能重复触发（P4 分布式锁未做）
- **性能风险**：`next_run_at` 存于 `metadata_json`，到期筛选在进程内完成（P4 待优化）
- **数据一致性**：`metadata_json` 写入前 reload 任务避免 lost update；`once` 任务执行后自动暂停
- **错误处理技术债**：Data 层未使用 `entErrToBizErr`（DB-DEBT-04），需重构
- **环境变量依赖**：`CRON_RUNNER_INTERVAL`（默认 1m）、`CRON_RUNNER_DISABLED=1` 关闭调度、`CRON_CHAT_DISPATCH_ORIGIN` HTTP fallback

---

## 10. 本期实施范围（迭代 2 已交付）

- `/cron`：专用定时任务管理页（含执行历史弹窗 `CronRunsDialog`），替代通用 `ResourceManagerPage`
- `GET /v1/cron-task-runs`：读取已有 `cron_task_run` 表，返回最近运行记录
- 创建 / 编辑 / 删除 / 启停：继续使用 `/v1/cron-tasks` 通用资源 CRUD
- 手动触发：`POST /v1/cron-tasks/{id}/trigger`（异步执行，返回 `pending` run）
- 重置失败计数：`POST /v1/cron-tasks/{id}/reset-failures`
- 后端 Cron runner：定时扫描到期任务，调动 Agent / Team / ModelRegistrySync，并回写执行历史与统计字段
- `retry_max_attempts` 表单字段：0=禁用，默认 3
- `CronTriggerGateway` 适配器：桥接 gateway 调用到 CronService
- `SeedModelRegistrySyncTask`：注入模型注册表同步种子任务

---

*文档版本：2.0 — 按三件套内容边界重组：合并需求文档迁移来的本期实施范围，补全代码锚点（cron 子包、适配器、execute.go 拆分、前端 composable），对齐状态标记与代码真实状态。*
