# M56 — 业务逻辑优化（BLO）开发计划

> **版本**：2026-05-26 · **状态**：📋 待启动 · **EP**：EP-BLO-M56
> **需求**：[56 business-logic-optimization.md](./56%20business-logic-optimization.md)
> **背景 Review**：[2026-05-26-Channel-Chat-AgentTeam-Flow-Review.md](../review/2026-05-26-Channel-Chat-AgentTeam-Flow-Review.md)
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

## 1. 当前状态（2026-05-26）

| 项 | 状态 | 备注 |
|----|------|------|
| 需求文档 | ✅ | 见 §需求 |
| 设计文档（详细）| 📋 | BLO-5 启动前补 `56 business-logic-optimization.design.md` |
| 数据库迁移 | 📋 | 4 个新表 + 1 个 ALTER |
| Feature flag 框架 | ✅ 复用 | `pkg/auth/features.go` 模式 |
| 监控看板 | 📋 | Datadog 4 张图表 |
| 红线 CI | ✅ | `make runtime-boundary` 已落 |

---

## 2. 前置准备（先做，0.5 周）

### BLO-PRE-01 — 详细设计文档
- **产出**：`docs/需求/56 business-logic-optimization.design.md`
- **内容**：
  - 5 个主题的详细类图 / 状态机 / 时序图
  - 端口接口完整签名（Go 代码块）
  - DB 迁移 SQL 完整版（含回滚）
  - Feature flag 命名约定与默认值
- **负责**：架构 owner · **验收**：评审通过

### BLO-PRE-02 — Feature flag 注入框架
- **产出**：`internal/conf/features_blo.go`
- **flag 命名**：
  - `BLO_UNIFIED_JOB_ENABLED`（BLO-5）
  - `BLO_PENDING_TASK_V2`（BLO-4）
  - `BLO_ESCALATION_V2`（BLO-2）
  - `BLO_INTENT_CLASSIFIER`（BLO-1）
  - `BLO_TRIGGER_RULES`（BLO-3）
- **默认**：全部关闭，dev 环境可单独开启
- **验收**：单测覆盖 flag 读取与默认值

### BLO-PRE-03 — Datadog 看板雏形
- **产出**：4 张面板（BackgroundJob / PendingTask / Escalation / IntentClassifier）
- **验收**：面板已可显示零数据骨架

---

## 3. BLO-5 — Unified BackgroundJob（第一波，3 周）

> **依赖**：无 · **解锁**：BLO-4 / BLO-2 / BLO-3

### Sprint A1：抽象与数据层（1 周）

| 任务 ID | 内容 | 文件 | 工时 |
|---------|------|------|------|
| BLO-5-BIZ-01 | 定义 `biz/backgroundjob/job.go` 类型 + 端口 | `internal/biz/backgroundjob/job.go`（新） | 1d |
| BLO-5-BIZ-02 | `BackgroundJobRepo` 接口与 mock | `backgroundjob/repo.go`（新） | 1d |
| BLO-5-DATA-01 | Ent schema `background_job` + 迁移 | `internal/data/ent/schema/background_job.go`（新） | 1d |
| BLO-5-DATA-02 | Ent repo 实现 `BackgroundJobRepo` | `internal/data/background_job.go`（新） | 1d |
| BLO-5-DATA-03 | TryClaim 原子认领（事务 + UPDATE...WHERE status='queued'） | 同上 | 0.5d |
| BLO-5-TEST-01 | Repo 单测（CRUD + TryClaim 竞态） | `background_job_test.go` | 0.5d |

**Gate A1**：`go test ./internal/biz/backgroundjob/... ./internal/data/...` 全过

### Sprint A2：Dispatcher 与 Runner（1 周）

| 任务 ID | 内容 | 文件 | 工时 |
|---------|------|------|------|
| BLO-5-RT-01 | `BackgroundJobDispatcher` 实现（含 priority queue） | `internal/runtime/backgroundjob/dispatcher.go`（新） | 1.5d |
| BLO-5-RT-02 | 双 worker 池（priority<50 实时池 + priority>=50 后台池） | `internal/runtime/backgroundjob/worker.go`（新） | 1d |
| BLO-5-RT-03 | Runner 注册机制 + Cancel 信号广播 | `internal/runtime/backgroundjob/registry.go`（新） | 1d |
| BLO-5-RT-04 | ParentJobID DAG 调度（子 Job 等父完成） | `internal/runtime/backgroundjob/dag.go`（新） | 1d |
| BLO-5-TEST-02 | Dispatcher 集成测试（含 cancel 级联 + DAG） | `dispatcher_test.go` | 0.5d |

**Gate A2**：模拟提交 100 Job（含 parent/child）全部正确状态流转 + 取消父级联取消子

### Sprint A3：现有系统接入 + RPC（1 周）

| 任务 ID | 内容 | 文件 | 工时 |
|---------|------|------|------|
| BLO-5-MIGRATE-01 | `SessionRunDurableWorker` 注册为 `kind=session_run_durable` Runner | `internal/service/session_run_durable_worker.go` | 1d |
| BLO-5-MIGRATE-02 | `ChannelAsyncGraph` watch 改为 `kind=channel_async` Runner | `internal/service/channel_async_graph.go` | 1d |
| BLO-5-MIGRATE-03 | `channel_turn_job` 写入路径 **双写** 旧表 + 新表（flag 开启时） | `internal/service/channel_ingress_job.go` | 1d |
| BLO-5-API-01 | proto `backgroundjob/v1/backgroundjob.proto` + 代码生成 | `api/kratos/backgroundjob/v1/` | 1d |
| BLO-5-SVC-01 | Service `BackgroundJobService` 实现 + 路由注册 | `internal/service/backgroundjob.go`（新） | 1d |
| BLO-5-WEB-01 | 前端 `useBackgroundJobs` composable + 替换 Chat/Channel Jobs 面板数据源 | `web/src/features/jobs/`（新） | 2d |

**Gate A3**：
- Channel `/async` 与 Chat `/background` 双轨同时工作，前端 Jobs 面板从新 API 取数
- `GET /v1/background-jobs?owner_type=session` 与 `?owner_type=channel` 返回统一 schema

---

## 4. BLO-4 — Non-Blocking HITL（第二波，2.5 周）

> **依赖**：BLO-5 A1 完成 · **解锁**：BLO-1 完整版

### Sprint B1：数据与端口（1 周）

| 任务 ID | 内容 | 文件 | 工时 |
|---------|------|------|------|
| BLO-4-BIZ-01 | `biz/session/pending_task.go` 类型 + Usecase | 新 | 1d |
| BLO-4-BIZ-02 | `PendingTaskRepo` + Ent schema `pending_task` | `internal/data/ent/schema/pending_task.go`（新） | 1d |
| BLO-4-DATA-01 | Ent repo 实现 + 单测 | `internal/data/pending_task.go`（新） | 1d |
| BLO-4-RT-01 | `RunRegistry` 增加 `PendingTaskID` 字段（不再用 `awaiting_user` 锁 session） | `internal/runtime/run_registry.go` | 1d |
| BLO-4-API-01 | proto 新增 `PendingTask` + `AnswerPendingTaskRequest` + RPC | `api/kratos/chat/v1/chat.proto` | 0.5d |

### Sprint B2：链路改造（1 周）

| 任务 ID | 内容 | 文件 | 工时 |
|---------|------|------|------|
| BLO-4-SVC-01 | `makeAwaitReplyFunc` 改写：持久化 PendingTask + **释放 session_lock** | `chat_orchestrator_turn.go:291-322` | 2d |
| BLO-4-SVC-02 | `AnswerPendingTask` → Submit BackgroundJob (`kind=agent_turn_resume`) | `chat.go` + 新 Runner | 1d |
| BLO-4-SVC-03 | Runner `agent_turn_resume` 实现（复用 durable resume 路径） | `internal/runtime/backgroundjob/runners.go` | 1d |
| BLO-4-CHANNEL-01 | IM 卡片渲染加入 `task_id` 隐藏字段 | `internal/biz/channel_im_render.go` | 1d |

### Sprint B3：UI + 收口（0.5 周）

| 任务 ID | 内容 | 文件 | 工时 |
|---------|------|------|------|
| BLO-4-WEB-01 | `usePendingTasks` composable | `web/src/features/chat/composables/` | 1d |
| BLO-4-WEB-02 | PendingTask 提示横幅（非阻塞 UI） | `web/src/features/chat/components/PendingTaskBanner.vue`（新） | 1d |
| BLO-4-TEST-01 | E2E：HITL 期间同 Session 发起新 Turn 成功 | `chat_pending_task_test.go`（新） | 1d |

**Gate B**：
- `await_user_reply` 期间在同 Session 发新消息得到响应
- PendingTask 超时后用户回复返回 410 Gone

---

## 5. BLO-2 — Multi-Signal Escalation（第二波并行，1 周）

> **依赖**：BLO-5 A1 完成 · **并行**：与 BLO-4 同时进行

| 任务 ID | 内容 | 文件 | 工时 |
|---------|------|------|------|
| BLO-2-BIZ-01 | `biz/session_run_escalation.go` 类型 + Policy 接口 | 新 | 0.5d |
| BLO-2-BIZ-02 | 默认 Policy 实现 + 单测覆盖 §4.4 全部规则 | `escalation_default_policy.go`（新） | 1.5d |
| BLO-2-SVC-01 | `chat_orchestrator_session_run.go` budget watcher 改为 EscalationPolicy 适配 | `chat_orchestrator_session_run.go` | 1d |
| BLO-2-SVC-02 | Escalation decision 记录到 `session_run.escalation_reason` + 通过 BackgroundJob (BLO-5) 升级 | 同上 | 1d |
| BLO-2-DATA-01 | `ALTER TABLE session_run ADD COLUMN escalation_reason / escalation_signals_json` | `internal/data/ent/schema/session_run.go` | 0.5d |
| BLO-2-WS-01 | `escalation_decision` Envelope + 前端展示 reason | `internal/event/envelope.go` · `web/.../components/EscalationNotice.vue` | 1d |
| BLO-2-TEST-01 | 单测：8 个 tool call / 50k token / graph 进入 / user_declared 等触发条件 | `escalation_policy_test.go`（新） | 0.5d |

**Gate C**：
- 模拟 `tool_calls=9` 的 turn 自动升 durable，IM 卡片显示"工具调用超过 8 次，转后台"
- `/background` 命令立即升级

---

## 6. BLO-1 — Intent-Aware Admission（第三波，2.5 周）

> **依赖**：BLO-4 完成（PendingTask 路径已 ready） · **解锁**：BLO-3 部分功能

### Sprint D1：Classifier v0 关键词（1 周）

| 任务 ID | 内容 | 文件 | 工时 |
|---------|------|------|------|
| BLO-1-BIZ-01 | `biz/turn_intent.go` 类型 + ClassifierV0（关键词 + heuristic） | 新 | 1d |
| BLO-1-BIZ-02 | 多语言关键词字典（中文 + 英文 + 日文） | `turn_intent_keywords.go`（新） | 0.5d |
| BLO-1-SVC-01 | `admission_gate` 在策略评估前调用 Classifier | `chat_orchestrator_turn.go:checkTurnAdmission` | 1.5d |
| BLO-1-SVC-02 | `ingress_policy.go` 决策表增加 intent 维度 | `internal/service/ingress_policy.go` | 1d |
| BLO-1-TEST-01 | 关键词单测 + 决策表覆盖 | `turn_intent_test.go`（新） | 1d |

### Sprint D2：Classifier v1 LLM 增强（1 周）

| 任务 ID | 内容 | 文件 | 工时 |
|---------|------|------|------|
| BLO-1-BIZ-03 | ClassifierV1（轻量 LLM 调用 + 5min 缓存） | `turn_intent_llm.go`（新） | 2d |
| BLO-1-CONF-01 | 配置项：classifier_model / cache_ttl / disabled_for_agent_ids | `configs/config.yaml` | 0.5d |
| BLO-1-BIZ-04 | Fallback 链：V1 失败 → V0 → 原 ingress_policy | 同上 | 0.5d |
| BLO-1-METRIC-01 | classifier_latency / classifier_confidence / classifier_accuracy 指标 | `internal/metrics/` | 0.5d |
| BLO-1-TEST-02 | LLM 调用单测（mock）+ 缓存命中率 | `turn_intent_llm_test.go`（新） | 0.5d |

### Sprint D3：UI + 灰度（0.5 周）

| 任务 ID | 内容 | 文件 | 工时 |
|---------|------|------|------|
| BLO-1-WS-01 | `intent_classified` Envelope（debug only） | `internal/event/envelope.go` | 0.5d |
| BLO-1-WEB-01 | 开发者面板显示 intent + confidence（默认折叠） | `web/.../components/DevInspector.vue` | 0.5d |
| BLO-1-E2E-01 | E2E：Web 与飞书连发同样输入得到一致 intent 分类 | `intent_admission_e2e_test.go`（新） | 1d |

**Gate D**：
- 100 条人工构造测试集 Classifier 准确率 > 85%
- Web / Channel 同输入下 admission 决策一致

---

## 7. BLO-3 — Channel Trigger Rules（第四波，4 周）

> **依赖**：BLO-5 完成（schedule 通过 BackgroundJob 调度）· **解锁**：日报 / 静默观察 / 评估收集

### Sprint E1：数据与端口（1 周）

| 任务 ID | 内容 | 文件 | 工时 |
|---------|------|------|------|
| BLO-3-BIZ-01 | `biz/channel_trigger.go` 类型 + Usecase | 新 | 1d |
| BLO-3-BIZ-02 | `biz/channel_observation.go` 类型 + Usecase（含 TTL sweeper） | 新 | 1d |
| BLO-3-DATA-01 | Ent schema `channel_trigger` + `channel_observation` | 新 | 1d |
| BLO-3-BIZ-03 | `TriggerEvaluator` 接口 + 默认实现（5 种规则） | `channel_trigger_evaluator.go`（新） | 2d |

### Sprint E2：入站链路接入（1 周）

| 任务 ID | 内容 | 文件 | 工时 |
|---------|------|------|------|
| BLO-3-SVC-01 | `acceptInbound` 之前增加 TriggerEvaluator 评估 | `channel_ingress_accept.go` | 1.5d |
| BLO-3-SVC-02 | `kind=silent` 写入 ChannelObservation；`kind=schedule` 注册 BackgroundJob | 同上 | 1d |
| BLO-3-SVC-03 | `kind=reaction` 写入 evaluation_feedback（接 M33） | `channel_ingress_card_action.go` 扩展 | 1d |
| BLO-3-MEMORY-01 | L2 episodic recall 包含 ChannelObservation（最近 7 天） | `internal/agent/memory_inject.go` | 1d |
| BLO-3-MIGRATE-01 | 现有 Channel 自动插入 `kind=mention` 默认规则 | `cmd/admin/migrations.go` | 0.5d |

### Sprint E3：管理 UI（1 周）

| 任务 ID | 内容 | 文件 | 工时 |
|---------|------|------|------|
| BLO-3-API-01 | proto `ListChannelTriggers / CreateTrigger / UpdateTrigger / DeleteTrigger` | `api/kratos/channel/v1/channel.proto` | 0.5d |
| BLO-3-SVC-04 | Service 实现 + RBAC（仅 admin 可改 schedule） | `channel.go` 扩展 | 1.5d |
| BLO-3-WEB-01 | `ChannelTriggersPanel.vue` 列表 + 表单 | `web/src/features/channels/components/` | 2d |
| BLO-3-WEB-02 | Schedule cron 可视化编辑器（复用已有 cron UI） | `web/src/features/cron/composables/` | 1d |

### Sprint E4：合规 + 收口（1 周）

| 任务 ID | 内容 | 文件 | 工时 |
|---------|------|------|------|
| BLO-3-COMPLIANCE-01 | 群内首次启用 `silent` 规则时 IM 通知"本群已开启智能观察" | `channel_im_render.go` 扩展 | 1d |
| BLO-3-COMPLIANCE-02 | `channel_observation` retain_until 默认 7 天 + sweeper cron | `internal/cronrunner/observation_sweeper.go`（新） | 1d |
| BLO-3-DOC-01 | 用户文档 / 管理员手册 | `docs/scenarios/channel-triggers.md`（新） | 1d |
| BLO-3-E2E-01 | E2E：日报 / keyword / reaction 全部触发 | `channel_trigger_e2e_test.go`（新） | 1d |

**Gate E**：
- 配置 cron `0 18 * * *` 触发日报 Agent 调用并生成消息
- 群内 `silent` 模式不响应但被 L2 召回
- Reaction → evaluation 表记录

---

## 8. 收口与下线（3 周）

### Sprint F1：灰度（2 周）

| 任务 ID | 内容 | 工时 |
|---------|------|------|
| BLO-ROLLOUT-01 | dev / staging 全 flag 开启 7 天 soak | 7d |
| BLO-ROLLOUT-02 | prod 单租户灰度 BLO-5 / BLO-4 / BLO-2 7 天 | 7d |
| BLO-ROLLOUT-03 | prod 全量 BLO-1 / BLO-3 | 同上 |

### Sprint F2：旧路径下线（1 周）

| 任务 ID | 内容 | 文件 | 工时 |
|---------|------|------|------|
| BLO-SUNSET-01 | `session_run.budget_watcher` 旧路径删除（仅留兜底） | `session_run_budget.go` | 1d |
| BLO-SUNSET-02 | `channel_turn_job` 旧表停止写入（仅查询保留） | `channel_ingress_job.go` | 1d |
| BLO-SUNSET-03 | 旧 `awaiting_user` session_lock 持有逻辑删除 | `chat_orchestrator_turn.go` | 1d |
| BLO-SUNSET-04 | 旧 `channel_access` allowlist 由 `kind=mention` 规则替代（迁移脚本） | `cmd/admin/migrations.go` | 1d |
| BLO-SUNSET-05 | 旧 Jobs 面板 API 标 deprecated | OpenAPI + Web | 0.5d |

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

## 13. 相关链接

- 需求主文档：[56 business-logic-optimization.md](./56%20business-logic-optimization.md)
- 详细设计（待补）：`56 business-logic-optimization.design.md`
- 背景 Review：[2026-05-26-Channel-Chat-AgentTeam-Flow-Review.md](../review/2026-05-26-Channel-Chat-AgentTeam-Flow-Review.md)
- 框架边界：[../AGENT_RUNTIME_BOUNDARY.md](../AGENT_RUNTIME_BOUNDARY.md)
- 红线规则：[.cursor/rules/trpc-agent-framework-first.mdc](../../.cursor/rules/trpc-agent-framework-first.mdc)
- AI 编码规范：[../guides/AI-DEVELOPMENT-SPECIFICATION.md](../guides/AI-DEVELOPMENT-SPECIFICATION.md)
