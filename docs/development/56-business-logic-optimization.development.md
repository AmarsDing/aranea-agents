# M56 — 业务逻辑优化（BLO）开发计划

> **版本**：2026-06-17 · **状态**：🚧 进行中（BLO-5 Sprint A1 已完成） · **EP**：EP-BLO-M56
> **需求**：[56-business-logic-optimization.md](./56-business-logic-optimization.md)
> **设计**：[56-business-logic-optimization.design.md](./56-business-logic-optimization.design.md)
> **总工时估算**：12 周 / 1 个 Quarter

---

## 0. 任务 ID 编码约定

```
BLO-{主题}-{子模块}-{序号}

BLO-5-INFRA-01   # BLO-5 基础设施第 1 项
BLO-4-BIZ-02     # BLO-4 biz 层第 2 项
BLO-1-WEB-03     # BLO-1 前端第 3 项
```

主题与执行顺序：

| 主题 | 名称 | 顺序 | 估时 |
|------|------|------|------|
| BLO-5 | Unified BackgroundJob（基础设施） | **第一波** | 3 周 |
| BLO-4 | Non-Blocking HITL (PendingTask) | 第二波 | 2.5 周 |
| BLO-2 | Multi-Signal Escalation | 第二波（并行） | 1 周 |
| BLO-1 | Intent-Aware Admission | 第三波 | 2.5 周 |
| BLO-3 | Channel Trigger Rules | 第四波 | 4 周 |
| 收口 | 灰度 + 下线 | 最后 | 3 周 |

---

## 1. 当前状态（2026-06-17 核对代码后）

| 项 | 状态 | 备注 |
|----|------|------|
| 需求文档 | ✅ | [56-business-logic-optimization.md](./56-business-logic-optimization.md) |
| 设计文档 | ✅ | [56-business-logic-optimization.design.md](./56-business-logic-optimization.design.md) |
| Feature flag 框架 | ✅ | `internal/conf/features_blo.go` 已落地（5 个 flag） |
| BLO-5 biz 类型 + Repo 端口 | ✅ | `internal/biz/backgroundjob/{job.go,repo.go}` |
| BLO-5 Ent Schema + Repo 实现 | ✅ | `internal/data/ent/schema/background_job.go` + `internal/data/background_job.go` |
| BLO-5 Repo 单测 | ✅ | `internal/biz/backgroundjob/backgroundjob_test.go` + `internal/data/background_job_test.go` |
| BLO-5 Dispatcher / Worker / DAG | 📋 | `internal/runtime/backgroundjob/` 未创建 |
| BLO-5 现有系统接入 + RPC | 📋 | `api/kratos/backgroundjob/v1/` 未创建；`internal/service/backgroundjob.go` 未创建 |
| BLO-4 数据与端口 | 📋 | `internal/biz/session/pending_task.go` 未创建 |
| BLO-4 链路改造 | 📋 | `internal/service/chat_orch_await.go:211` `MakeAwaitReplyFunc` 仍为阻塞实现 |
| BLO-2 Escalation Policy | 📋 | `internal/biz/session_run_escalation.go` 未创建；schema 未加 `escalation_reason` 列 |
| BLO-2 通知器 | ⚠️ 部分 | `internal/service/session_run_escalation_notifier.go` 已存在（接口），未带 reason |
| BLO-1 Intent Classifier | 📋 | `internal/biz/turn_intent.go` 未创建 |
| BLO-3 Trigger Evaluator | 📋 | `internal/biz/channel_trigger.go` 未创建 |
| 监控看板 | 📋 | Datadog 4 张图表未建 |
| 红线 CI | ✅ | `make runtime-boundary` 已落 |
| 旧 budget watcher | ✅ 已移除 | `internal/data/ent/schema/session_run.go` 中 `soft_budget_sec`/`hard_budget_sec` 标注 Deprecated |

---

## 2. 前置准备（先做，0.5 周）

### BLO-PRE-01 — 详细设计文档 ✅
- **产出**：`docs/development/56-business-logic-optimization.design.md`
- **内容**：5 个主题的详细类图 / 状态机 / 时序图、端口接口完整签名、DB 迁移 SQL、Feature flag 命名约定
- **状态**：已完成（2026-06-17）

### BLO-PRE-02 — Feature flag 注入框架 ✅
- **产出**：`internal/conf/features_blo.go`
- **flag 命名**：
  - `BLO_UNIFIED_JOB_ENABLED`（BLO-5）
  - `BLO_PENDING_TASK_V2`（BLO-4）
  - `BLO_ESCALATION_V2`（BLO-2）
  - `BLO_INTENT_CLASSIFIER`（BLO-1）
  - `BLO_TRIGGER_RULES`（BLO-3）
- **默认**：全部关闭，dev 环境可单独开启
- **状态**：已完成；函数：`BLOUnifiedJobEnabled()` / `BLOPendingTaskV2()` / `BLOEscalationV2()` / `BLOIntentClassifier()` / `BLOTriggerRules()`

### BLO-PRE-03 — Datadog 看板雏形 📋
- **产出**：4 张面板（BackgroundJob / PendingTask / Escalation / IntentClassifier）
- **验收**：面板已可显示零数据骨架

---

## 3. BLO-5 — Unified BackgroundJob（第一波，3 周）

> **依赖**：无 · **解锁**：BLO-4 / BLO-2 / BLO-3

### Sprint A1：抽象与数据层（1 周）✅

| 任务 ID | 内容 | 文件 | 工时 | 状态 |
|---------|------|------|------|------|
| BLO-5-BIZ-01 | 定义 `biz/backgroundjob/job.go` 类型 + 端口 | `internal/biz/backgroundjob/job.go` | 1d | ✅ |
| BLO-5-BIZ-02 | `BackgroundJobRepo` 接口与 mock | `internal/biz/backgroundjob/repo.go` | 1d | ✅ |
| BLO-5-DATA-01 | Ent schema `background_jobs` + 迁移 | `internal/data/ent/schema/background_job.go` | 1d | ✅ |
| BLO-5-DATA-02 | Ent repo 实现 `BackgroundJobRepo` | `internal/data/background_job.go` | 1d | ✅ |
| BLO-5-DATA-03 | TryClaim 原子认领（事务 + UPDATE...WHERE status='queued'） | `internal/data/background_job.go` | 0.5d | ✅ |
| BLO-5-TEST-01 | Repo 单测（CRUD + TryClaim 竞态） | `internal/biz/backgroundjob/backgroundjob_test.go` + `internal/data/background_job_test.go` | 0.5d | ✅ |

**Gate A1**：✅ `go test ./internal/biz/backgroundjob/... ./internal/data/...` 全过

**实际落地差异**（与原设计）：
- 表名 `background_jobs`（复数），非 `background_job`
- 字段调整：`deadline` / `result_json` / `visibility` / `error_code` / `error_message` 未实现，改为 `last_error` + `attempts`/`max_attempts` 重试模型
- 时间戳用 int64 Unix 毫秒，非 DATETIME
- Status 枚举：`queued` / `claimed` / `succeeded` / `failed` / `cancelled`（无 `running` / `awaiting` / `timed_out`）

### Sprint A2：Dispatcher 与 Runner（1 周）📋

| 任务 ID | 内容 | 文件 | 工时 | 状态 |
|---------|------|------|------|------|
| BLO-5-RT-01 | `BackgroundJobDispatcher` 实现（含 priority queue） | `internal/runtime/backgroundjob/dispatcher.go`（新） | 1.5d | 📋 |
| BLO-5-RT-02 | 双 worker 池（priority<50 实时池 + priority>=50 后台池） | `internal/runtime/backgroundjob/worker.go`（新） | 1d | 📋 |
| BLO-5-RT-03 | Runner 注册机制 + Cancel 信号广播 | `internal/runtime/backgroundjob/registry.go`（新） | 1d | 📋 |
| BLO-5-RT-04 | ParentJobID DAG 调度（子 Job 等父完成） | `internal/runtime/backgroundjob/dag.go`（新） | 1d | 📋 |
| BLO-5-TEST-02 | Dispatcher 集成测试（含 cancel 级联 + DAG） | `internal/runtime/backgroundjob/dispatcher_test.go`（新） | 0.5d | 📋 |

**Gate A2**：模拟提交 100 Job（含 parent/child）全部正确状态流转 + 取消父级联取消子

### Sprint A3：现有系统接入 + RPC（1 周）📋

| 任务 ID | 内容 | 文件 | 工时 | 状态 |
|---------|------|------|------|------|
| BLO-5-MIGRATE-01 | `SessionRunDurableWorker` 注册为 `kind=session_run_durable` Runner | `internal/service/session_run_durable_worker.go` | 1d | 📋 |
| BLO-5-MIGRATE-02 | `ChannelAsyncGraph` watch 改为 `kind=channel_async` Runner | `internal/service/channel_async_graph.go` | 1d | 📋 |
| BLO-5-MIGRATE-03 | `channel_turn_job` 写入路径 **双写** 旧表 + 新表（flag 开启时） | `internal/service/channel_ingress_job.go` | 1d | 📋 |
| BLO-5-API-01 | proto `backgroundjob/v1/backgroundjob.proto` + 代码生成 | `api/kratos/backgroundjob/v1/`（新） | 1d | 📋 |
| BLO-5-SVC-01 | Service `BackgroundJobService` 实现 + 路由注册 | `internal/service/backgroundjob.go`（新） | 1d | 📋 |
| BLO-5-WEB-01 | 前端 `useBackgroundJobs` composable + 替换 Chat/Channel Jobs 面板数据源 | `web/src/features/jobs/`（新） | 2d | 📋 |

**Gate A3**：
- Channel `/async` 与 Chat `/background` 双轨同时工作，前端 Jobs 面板从新 API 取数
- `GET /v1/background-jobs?owner_type=session` 与 `?owner_type=channel` 返回统一 schema

---

## 4. BLO-4 — Non-Blocking HITL（第二波，2.5 周）

> **依赖**：BLO-5 A1 完成 · **解锁**：BLO-1 完整版

### Sprint B1：数据与端口（1 周）📋

| 任务 ID | 内容 | 文件 | 工时 | 状态 |
|---------|------|------|------|------|
| BLO-4-BIZ-01 | `biz/session/pending_task.go` 类型 + Usecase | `internal/biz/session/pending_task.go`（新） | 1d | 📋 |
| BLO-4-BIZ-02 | `PendingTaskRepo` + Ent schema `pending_task` | `internal/data/ent/schema/pending_task.go`（新） | 1d | 📋 |
| BLO-4-DATA-01 | Ent repo 实现 + 单测 | `internal/data/pending_task.go`（新） | 1d | 📋 |
| BLO-4-RT-01 | `RunRegistry` 增加 `PendingTaskID` 字段（不再用 `awaiting_user` 锁 session） | `internal/runtime/run_registry.go` | 1d | 📋 |
| BLO-4-API-01 | proto 新增 `PendingTask` + `AnswerPendingTaskRequest` + RPC | `api/kratos/chat/v1/chat.proto` | 0.5d | 📋 |

### Sprint B2：链路改造（1 周）📋

| 任务 ID | 内容 | 文件 | 工时 | 状态 |
|---------|------|------|------|------|
| BLO-4-SVC-01 | `MakeAwaitReplyFunc` 改写：持久化 PendingTask + **释放 session_lock** | `internal/service/chat_orch_await.go:211`（`MakeAwaitReplyFunc`） | 2d | 📋 |
| BLO-4-SVC-02 | `AnswerPendingTask` → Submit BackgroundJob (`kind=agent_turn_resume`) | `internal/service/chat.go:336`（`AwaitUserReply`） + 新 Runner | 1d | 📋 |
| BLO-4-SVC-03 | Runner `agent_turn_resume` 实现（复用 durable resume 路径） | `internal/runtime/backgroundjob/runners.go`（新） | 1d | 📋 |
| BLO-4-CHANNEL-01 | IM 卡片渲染加入 `task_id` 隐藏字段 | `internal/biz/channel_im_render.go` | 1d | 📋 |

### Sprint B3：UI + 收口（0.5 周）📋

| 任务 ID | 内容 | 文件 | 工时 | 状态 |
|---------|------|------|------|------|
| BLO-4-WEB-01 | `usePendingTasks` composable | `web/src/features/chat/composables/usePendingTasks.ts`（新） | 1d | 📋 |
| BLO-4-WEB-02 | PendingTask 提示横幅（非阻塞 UI） | `web/src/components/chat/PendingTaskBanner.vue`（新） | 1d | 📋 |
| BLO-4-TEST-01 | E2E：HITL 期间同 Session 发起新 Turn 成功 | `chat_pending_task_test.go`（新） | 1d | 📋 |

**Gate B**：
- `await_user_reply` 期间在同 Session 发新消息得到响应
- PendingTask 超时后用户回复返回 410 Gone

---

## 5. BLO-2 — Multi-Signal Escalation（第二波并行，1 周）

> **依赖**：BLO-5 A1 完成 · **并行**：与 BLO-4 同时进行

| 任务 ID | 内容 | 文件 | 工时 | 状态 |
|---------|------|------|------|------|
| BLO-2-BIZ-01 | `biz/session_run_escalation.go` 类型 + Policy 接口 | `internal/biz/session_run_escalation.go`（新） | 0.5d | 📋 |
| BLO-2-BIZ-02 | 默认 Policy 实现 + 单测覆盖设计文档 §2.4 全部规则 | `internal/biz/escalation_default_policy.go`（新） | 1.5d | 📋 |
| BLO-2-SVC-01 | `chat_orch_session_run_lifecycle.go` 升级路径改为 EscalationPolicy 适配 | `internal/service/chat_orch_session_run_lifecycle.go`（`EscalateToDurableByUser` / `applyDurableTransition`） | 1d | 📋 |
| BLO-2-SVC-02 | Escalation decision 记录到 `session_run.escalation_reason` + 通过 BackgroundJob (BLO-5) 升级 | 同上 | 1d | 📋 |
| BLO-2-DATA-01 | `ALTER TABLE session_run ADD COLUMN escalation_reason / escalation_signals_json` | `internal/data/ent/schema/session_run.go` + DDL 迁移 SQL | 0.5d | 📋 |
| BLO-2-WS-01 | `escalation_decision` Envelope + 前端展示 reason | `internal/event/envelope.go` · `web/src/components/chat/EscalationNotice.vue`（新） | 1d | 📋 |
| BLO-2-TEST-01 | 单测：8 个 tool call / 50k token / graph 进入 / user_declared 等触发条件 | `internal/biz/escalation_policy_test.go`（新） | 0.5d | 📋 |

**Gate C**：
- 模拟 `tool_calls=9` 的 turn 自动升 durable，IM 卡片显示"工具调用超过 8 次，转后台"
- `/background` 命令立即升级

> **注**：原设计文档中提到的 `chat_orchestrator_session_run.go` budget watcher 已不存在（budget 机制已移除）。当前升级入口在 `internal/service/chat_orch_session_run_lifecycle.go` 的 `EscalateToDurableByUser`。

---

## 6. BLO-1 — Intent-Aware Admission（第三波，2.5 周）

> **依赖**：BLO-4 完成（PendingTask 路径已 ready） · **解锁**：BLO-3 部分功能

### Sprint D1：Classifier v0 关键词（1 周）📋

| 任务 ID | 内容 | 文件 | 工时 | 状态 |
|---------|------|------|------|------|
| BLO-1-BIZ-01 | `biz/turn_intent.go` 类型 + ClassifierV0（关键词 + heuristic） | `internal/biz/turn_intent.go`（新） | 1d | 📋 |
| BLO-1-BIZ-02 | 多语言关键词字典（中文 + 英文 + 日文） | `internal/biz/turn_intent_keywords.go`（新） | 0.5d | 📋 |
| BLO-1-SVC-01 | `admission_gate` 在策略评估前调用 Classifier | `internal/service/chat_orchestrator_turn.go:84`（`checkTurnAdmission`） | 1.5d | 📋 |
| BLO-1-SVC-02 | `ingress_policy.go` 决策表增加 intent 维度 | `internal/service/ingress_policy.go` | 1d | 📋 |
| BLO-1-TEST-01 | 关键词单测 + 决策表覆盖 | `internal/biz/turn_intent_test.go`（新） | 1d | 📋 |

### Sprint D2：Classifier v1 LLM 增强（1 周）📋

| 任务 ID | 内容 | 文件 | 工时 | 状态 |
|---------|------|------|------|------|
| BLO-1-BIZ-03 | ClassifierV1（轻量 LLM 调用 + 5min 缓存） | `internal/biz/turn_intent_llm.go`（新） | 2d | 📋 |
| BLO-1-CONF-01 | 配置项：classifier_model / cache_ttl / disabled_for_agent_ids | `configs/config.yaml` | 0.5d | 📋 |
| BLO-1-BIZ-04 | Fallback 链：V1 失败 → V0 → 原 ingress_policy | 同上 | 0.5d | 📋 |
| BLO-1-METRIC-01 | classifier_latency / classifier_confidence / classifier_accuracy 指标 | `internal/metrics/` | 0.5d | 📋 |
| BLO-1-TEST-02 | LLM 调用单测（mock）+ 缓存命中率 | `internal/biz/turn_intent_llm_test.go`（新） | 0.5d | 📋 |

### Sprint D3：UI + 灰度（0.5 周）📋

| 任务 ID | 内容 | 文件 | 工时 | 状态 |
|---------|------|------|------|------|
| BLO-1-WS-01 | `intent_classified` Envelope（debug only） | `internal/event/envelope.go` | 0.5d | 📋 |
| BLO-1-WEB-01 | 开发者面板显示 intent + confidence（默认折叠） | `web/src/components/chat/DevInspector.vue`（新） | 0.5d | 📋 |
| BLO-1-E2E-01 | E2E：Web 与飞书连发同样输入得到一致 intent 分类 | `intent_admission_e2e_test.go`（新） | 1d | 📋 |

**Gate D**：
- 100 条人工构造测试集 Classifier 准确率 > 85%
- Web / Channel 同输入下 admission 决策一致

---

## 7. BLO-3 — Channel Trigger Rules（第四波，4 周）

> **依赖**：BLO-5 完成（schedule 通过 BackgroundJob 调度）· **解锁**：日报 / 静默观察 / 评估收集

### Sprint E1：数据与端口（1 周）📋

| 任务 ID | 内容 | 文件 | 工时 | 状态 |
|---------|------|------|------|------|
| BLO-3-BIZ-01 | `biz/channel_trigger.go` 类型 + Usecase | `internal/biz/channel_trigger.go`（新） | 1d | 📋 |
| BLO-3-BIZ-02 | `biz/channel_observation.go` 类型 + Usecase（含 TTL sweeper） | `internal/biz/channel_observation.go`（新） | 1d | 📋 |
| BLO-3-DATA-01 | Ent schema `channel_trigger` + `channel_observation` | `internal/data/ent/schema/channel_trigger.go` + `channel_observation.go`（新） | 1d | 📋 |
| BLO-3-BIZ-03 | `TriggerEvaluator` 接口 + 默认实现（5 种规则） | `internal/biz/channel_trigger_evaluator.go`（新） | 2d | 📋 |

### Sprint E2：入站链路接入（1 周）📋

| 任务 ID | 内容 | 文件 | 工时 | 状态 |
|---------|------|------|------|------|
| BLO-3-SVC-01 | `acceptInbound` 之前增加 TriggerEvaluator 评估 | `internal/service/channel_ingress_accept.go` | 1.5d | 📋 |
| BLO-3-SVC-02 | `kind=silent` 写入 ChannelObservation；`kind=schedule` 注册 BackgroundJob | 同上 | 1d | 📋 |
| BLO-3-SVC-03 | `kind=reaction` 写入 evaluation_feedback（接 M33） | `internal/service/channel_ingress_card_action.go` | 1d | 📋 |
| BLO-3-MEMORY-01 | L2 episodic recall 包含 ChannelObservation（最近 7 天） | `internal/agent/memory_inject.go` | 1d | 📋 |
| BLO-3-MIGRATE-01 | 现有 Channel 自动插入 `kind=mention` 默认规则 | DDL 迁移 SQL（注册到 `internal/data/ddl_migration_registry.go`） | 0.5d | 📋 |

### Sprint E3：管理 UI（1 周）📋

| 任务 ID | 内容 | 文件 | 工时 | 状态 |
|---------|------|------|------|------|
| BLO-3-API-01 | proto `ListChannelTriggers / CreateTrigger / UpdateTrigger / DeleteTrigger` | `api/kratos/channel/v1/channel.proto` | 0.5d | 📋 |
| BLO-3-SVC-04 | Service 实现 + RBAC（仅 admin 可改 schedule） | `internal/service/channel.go` 扩展 | 1.5d | 📋 |
| BLO-3-WEB-01 | `ChannelTriggersPanel.vue` 列表 + 表单 | `web/src/features/channels/ChannelTriggersPanel.vue`（新） | 2d | 📋 |
| BLO-3-WEB-02 | Schedule cron 可视化编辑器（复用已有 cron UI） | `web/src/features/cron/`（复用 `useCronTasksPage.ts` 模式） | 1d | 📋 |

### Sprint E4：合规 + 收口（1 周）📋

| 任务 ID | 内容 | 文件 | 工时 | 状态 |
|---------|------|------|------|------|
| BLO-3-COMPLIANCE-01 | 群内首次启用 `silent` 规则时 IM 通知"本群已开启智能观察" | `internal/biz/channel_im_render.go` 扩展 | 1d | 📋 |
| BLO-3-COMPLIANCE-02 | `channel_observation` retain_until 默认 7 天 + sweeper cron | `internal/cronrunner/observation_sweeper.go`（新） | 1d | 📋 |
| BLO-3-DOC-01 | 用户文档 / 管理员手册 | `docs/scenarios/channel-triggers.md`（新） | 1d | 📋 |
| BLO-3-E2E-01 | E2E：日报 / keyword / reaction 全部触发 | `channel_trigger_e2e_test.go`（新） | 1d | 📋 |

**Gate E**：
- 配置 cron `0 18 * * *` 触发日报 Agent 调用并生成消息
- 群内 `silent` 模式不响应但被 L2 召回
- Reaction → evaluation 表记录

---

## 8. 收口与下线（3 周）

### Sprint F1：灰度（2 周）📋

| 任务 ID | 内容 | 工时 | 状态 |
|---------|------|------|------|
| BLO-ROLLOUT-01 | dev / staging 全 flag 开启 7 天 soak | 7d | 📋 |
| BLO-ROLLOUT-02 | prod 单租户灰度 BLO-5 / BLO-4 / BLO-2 7 天 | 7d | 📋 |
| BLO-ROLLOUT-03 | prod 全量 BLO-1 / BLO-3 | 同上 | 📋 |

### Sprint F2：旧路径下线（1 周）📋

| 任务 ID | 内容 | 文件 | 工时 | 状态 |
|---------|------|------|------|------|
| BLO-SUNSET-01 | ~~`session_run.budget_watcher` 旧路径删除~~ **已完成**（budget 机制已移除，schema 字段标注 Deprecated） | `internal/data/ent/schema/session_run.go` | 0d | ✅ |
| BLO-SUNSET-02 | `channel_turn_job` 旧表停止写入（仅查询保留） | `internal/service/channel_ingress_job.go` | 1d | 📋 |
| BLO-SUNSET-03 | 旧 `awaiting_user` session_lock 持有逻辑删除（由 BLO-4 PendingTask 替代） | `internal/service/chat_orch_await.go` | 1d | 📋 |
| BLO-SUNSET-04 | 旧 `channel_access` allowlist 由 `kind=mention` 规则替代（迁移脚本） | DDL 迁移 SQL（注册到 `internal/data/ddl_migration_registry.go`） | 1d | 📋 |
| BLO-SUNSET-05 | 旧 Jobs 面板 API 标 deprecated | OpenAPI + Web | 0.5d | 📋 |

**Gate F**：
- 旧路径全部下线（保留只读兼容）
- 灰度期 P95 延迟 / 错误率不劣化
- 全部 5 个 feature flag 默认开启 + 文档更新

---

## 9. 关键验收用例

### BLO-5 单元测试
- `Dispatcher_TryClaim_NoDoubleDispatch`
- `Dispatcher_CascadeCancel_ChildAlsoCancelled`
- `Dispatcher_DAG_ChildWaitsParent`
- `Repo_TryClaim_TransactionIsolation`

### BLO-4 集成测试
- `PendingTask_DuringHITL_NewTurnSucceeds`（Session 不被锁定）
- `PendingTask_TimeoutThenAnswer_Returns410`
- `PendingTask_CardCancel_RunFailedNotified`

### BLO-2 单元测试
- `EscalationPolicy_ToolCallExceeds_Durable`
- `EscalationPolicy_UserDeclared_ImmediateDurable`
- `EscalationPolicy_GraphEntered_AutoDurable`
- `EscalationDecision_RecordedInDB`

### BLO-1 测试集
- 100 条人工标注样本（中/英/日各 30+ 条）准确率 > 85%
- 决策表覆盖：interrupt × 4 入口、append × 4 入口…
- LLM Classifier mock 单测

### BLO-3 集成测试
- `Trigger_MentionRule_DefaultBehaviorPreserved`
- `Trigger_ScheduleRule_SubmitsBackgroundJob`
- `Trigger_KeywordRule_TriggersAgent`
- `Trigger_ReactionRule_WritesEvaluationFeedback`
- `Trigger_SilentRule_NoTurnButObserved`

---

## 10. 风险与缓解

| 风险 | 等级 | 缓解 |
|------|------|------|
| BLO-5 双写期间数据漂移 | 高 | 写新表为主，旧表只读快照；2 sprint 后下线旧写 |
| BLO-1 Classifier 误判中断真实长任务 | 中 | 置信度 < 0.6 回退；可一键关 flag |
| BLO-4 PendingTask 死锁（Run 持有锁但等回复） | 中 | 释放 session_lock 是核心改动；单测专项覆盖 |
| BLO-3 群内静默观察合规风险 | 高 | 默认关闭；启用时 IM 公告；retain TTL 7d 默认 |
| BLO-2 Policy 阈值激进 → 大量任务升 durable | 中 | 灰度阈值可在 config 中热调 |
| BLO-5 旧表迁移失败 → 历史 Job 丢失 | 中 | 迁移脚本备份 + dry-run |
| 跨 sprint 主题相互阻塞 | 中 | A1 完成后 B/C 可并行；E 依赖 BLO-5 完成 |
| BLO-4 改造 `MakeAwaitReplyFunc` 引入 await 回归 | 高 | 保留旧阻塞路径作为 fallback；feature flag 控制 |

---

## 11. 团队与协作

| 角色 | 主要负责 |
|------|----------|
| 架构 owner | BLO-PRE-01 设计文档 / 红线把关 |
| 后端 (biz) | 所有 `BLO-X-BIZ-*` 与 `BLO-X-SVC-*` |
| 后端 (runtime) | BLO-5 Dispatcher + BLO-4 RunRegistry 改造 |
| 后端 (data) | 4 个新表 + 1 个 ALTER + 迁移脚本 |
| 前端 | 所有 `BLO-X-WEB-*`（Jobs 面板、PendingTask 横幅、Trigger 管理 UI） |
| QA | E2E 用例 + 灰度 soak 监控 |
| 运维 | Datadog 看板 / 灰度切换 / 旧路径下线 |

---

## 12. 文档约定

- 每个任务 PR 标题：`[BLO-{id}] {description}`
- 每个 PR 必须更新本文档对应任务的 状态（`📋 → 🚧 → ✅`）
- 主题完成时在需求文档对应章节补 changelog 链接

---

## 13. 改动文件清单（代码锚点核对）

### 已存在文件（BLO-5 Sprint A1 已落地）

| 文件 | 说明 |
|------|------|
| `internal/biz/backgroundjob/job.go` | BackgroundJob 类型 + 状态枚举 |
| `internal/biz/backgroundjob/repo.go` | Repo 端口接口 |
| `internal/biz/backgroundjob/backgroundjob_test.go` | biz 层单测 |
| `internal/data/ent/schema/background_job.go` | Ent Schema（表 `background_jobs`） |
| `internal/data/background_job.go` | Repo 实现 |
| `internal/data/background_job_test.go` | data 层单测 |
| `internal/conf/features_blo.go` | 5 个 feature flag |

### 待新增文件

| 文件 | 任务 |
|------|------|
| `internal/runtime/backgroundjob/dispatcher.go` | BLO-5-RT-01 |
| `internal/runtime/backgroundjob/worker.go` | BLO-5-RT-02 |
| `internal/runtime/backgroundjob/registry.go` | BLO-5-RT-03 |
| `internal/runtime/backgroundjob/dag.go` | BLO-5-RT-04 |
| `internal/service/backgroundjob.go` | BLO-5-SVC-01 |
| `api/kratos/backgroundjob/v1/backgroundjob.proto` | BLO-5-API-01 |
| `internal/biz/session/pending_task.go` | BLO-4-BIZ-01 |
| `internal/data/ent/schema/pending_task.go` | BLO-4-BIZ-02 |
| `internal/data/pending_task.go` | BLO-4-DATA-01 |
| `internal/biz/session_run_escalation.go` | BLO-2-BIZ-01 |
| `internal/biz/escalation_default_policy.go` | BLO-2-BIZ-02 |
| `internal/biz/turn_intent.go` | BLO-1-BIZ-01 |
| `internal/biz/turn_intent_keywords.go` | BLO-1-BIZ-02 |
| `internal/biz/turn_intent_llm.go` | BLO-1-BIZ-03 |
| `internal/biz/channel_trigger.go` | BLO-3-BIZ-01 |
| `internal/biz/channel_observation.go` | BLO-3-BIZ-02 |
| `internal/data/ent/schema/channel_trigger.go` | BLO-3-DATA-01 |
| `internal/data/ent/schema/channel_observation.go` | BLO-3-DATA-01 |
| `internal/biz/channel_trigger_evaluator.go` | BLO-3-BIZ-03 |
| `internal/cronrunner/observation_sweeper.go` | BLO-3-COMPLIANCE-02 |

### 已存在待改造文件

| 文件 | 改造点 | 任务 |
|------|--------|------|
| `internal/service/session_run_durable_worker.go` | 注册为 `kind=session_run_durable` Runner | BLO-5-MIGRATE-01 |
| `internal/service/channel_async_graph.go` | watch 改为 `kind=channel_async` Runner | BLO-5-MIGRATE-02 |
| `internal/service/channel_ingress_job.go` | 双写旧表 + 新表 | BLO-5-MIGRATE-03 |
| `internal/service/chat_orch_await.go` | `MakeAwaitReplyFunc`（line 211）改写为 PendingTask | BLO-4-SVC-01 |
| `internal/service/chat.go` | `AwaitUserReply`（line 336）改为 Submit BackgroundJob | BLO-4-SVC-02 |
| `internal/biz/channel_im_render.go` | IM 卡片加入 `task_id` | BLO-4-CHANNEL-01 |
| `internal/runtime/run_registry.go` | 增加 `PendingTaskID` 字段 | BLO-4-RT-01 |
| `internal/service/chat_orch_session_run_lifecycle.go` | `EscalateToDurableByUser` / `applyDurableTransition` 改为 EscalationPolicy 适配 | BLO-2-SVC-01/02 |
| `internal/data/ent/schema/session_run.go` | 加 `escalation_reason` / `escalation_signals_json` 列 | BLO-2-DATA-01 |
| `internal/service/session_run_escalation_notifier.go` | 通知带 reason | BLO-2-SVC-02 |
| `internal/event/envelope.go` | 新增 5 个事件类型 | BLO-2-WS-01 / BLO-1-WS-01 |
| `internal/service/chat_orchestrator_turn.go` | `checkTurnAdmission`（line 84）接入 Classifier | BLO-1-SVC-01 |
| `internal/service/ingress_policy.go` | 决策表增加 intent 维度 | BLO-1-SVC-02 |
| `internal/service/channel_ingress_accept.go` | `acceptInbound` 之前增加 TriggerEvaluator | BLO-3-SVC-01 |
| `internal/service/channel_ingress_card_action.go` | `kind=reaction` 写入 evaluation_feedback | BLO-3-SVC-03 |
| `internal/agent/memory_inject.go` | L2 recall 包含 ChannelObservation | BLO-3-MEMORY-01 |
| `internal/biz/channel_access.go` | allowlist 由 `kind=mention` 规则替代 | BLO-SUNSET-04 |

---

## 14. 相关链接

- 需求主文档：[56-business-logic-optimization.md](./56-business-logic-optimization.md)
- 设计文档：[56-business-logic-optimization.design.md](./56-business-logic-optimization.design.md)
- M55 Chat × Channel Cursor：[55-chat-channel-cursor.md](./55-chat-channel-cursor.md)
- Channel 模块：[17-channel.md](./17-channel.md) · [17-channel.design.md](./17-channel.design.md) · [17-channel.development.md](./17-channel.development.md)
- AI 编码规范：[../guides/AI-DEVELOPMENT-SPECIFICATION.md](../guides/AI-DEVELOPMENT-SPECIFICATION.md)
