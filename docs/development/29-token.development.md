# Token 用量 — 开发计划

> **版本**：2026-06-17 | **状态**：✅ 全部 Phase 已实现（P3 待扩展项见 §2.3）
> **需求**：[29 token.md](./29%20token.md) · **设计**：[29 token.design.md](./29%20token.design.md) · **前端页面**：[frontend-pages.md](./frontend-pages.md) §4.2
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：—

---

## 1. 模块定位

Token 用量管理：记录和统计 Agent / Team 运行的 Token 消耗，支持按时间 / Agent / Provider / Model 维度查询和聚合，支持预算管控和异常检测。

---

## 2. 现状评估

### 2.1 后端

| 项 | 状态 | 证据 |
|----|------|------|
| 明细表 `model_token_usage_events` | ✅ | `internal/data/sql/migrations/20260717_usage_events_schema.sql` + `20260612_pricing_rule_patches.sql`（DDL 管理） |
| 日聚合表 `model_token_usage_daily` | ✅ | DDL 管理；写入时自动 upsert |
| 小时聚合表 `model_token_usage_hourly` | ✅ | `internal/data/ent/schema/model_token_usage_hourly.go`（Ent Schema） |
| 价格规则表 `model_pricing_rules` | ✅ | `internal/data/ent/schema/model_pricing_rule.go` |
| 用量记录写入 | ✅ | 主路径 `recordTurnUsage`；Team `team_member` + `team_turn`（`internal/team/usage_record.go`、`usage_tokens.go`、`turn_helpers.MemberUsage`） |
| Session 聚合更新 | ✅ | `internal/data/usage_write.go` 事务内 UPDATE sessions |
| 日聚合 / 小时聚合 upsert | ✅ | `internal/data/usage_write.go` → `upsertModelTokenUsageDaily` / `RollupDailyHourly` |
| 用量概览 API | ✅ | `GetUsageOverview` + `quota_dashboard`（活跃 Agent 配额汇总） |
| 趋势查询 API | ✅ | `ListUsageTrends`；`granularity=hour` → `model_token_usage_hourly` |
| Top 模型排行 API | ✅ | `UsageService.ListTopModels` |
| Top Agent 排行 API | ✅ | `UsageService.ListTopAgents` |
| 明细列表 API | ✅ | `UsageService.ListUsageEvents` |
| 用量事件写入 API | ✅ | `UsageService.RecordTokenUsageEvent` |
| 查询筛选（Provider/Model/Agent/Team/Kind/Status/时间） | ✅ | `UsageQuery` + `usageWhere(billableOnly)` |
| 可计费聚合（排除 team_turn） | ✅ | `sqlUsageBillableKind`；概览/趋势/Top/配额 SUM |
| 异常请求筛选 | ✅ | `status = "abnormal"` → `status <> 'success'` |
| Wire 注入 | ✅ | `NewUsageRepo` / `provideUsageUsecase` / `NewUsageService` |
| 用量限额（quota） | ✅ | `usage_quotas` + `CheckQuota`（单 Agent + Team 成员）；`internal/data/usage_quota.go` |
| 配额拦截（TurnAdmissionUsecase） | ✅ | `internal/biz/turn_admission.go` → `EnforceChatTurnQuotas` / `EnforceTeamMemberQuotas`；Service 层 `chat_orchestrator_turn.go` / `team_turn_hooks.go` / `chat_orchestrator_turn_metrics.go` |
| 写入时费用计算 | ✅ | `RecordTokenUsageEvent` → `GetActiveModelPricing` + `ApplyTokenUsageCosts` |
| 用量告警（budget alert） | ✅ | `budget_alerts` + `EvaluateBudgetAlerts` → `usage.budget_alert` 监控事件 |
| 价格回退 | ✅ | `model_pricing_rules` 优先，否则 `llm_provider_models.config_json` 单价 |
| 价格自动同步 | 部分 | Provider 模型 inspect/保存 → `syncProviderModelPricing`；无独立定价 UI；无 API 定时拉取 |
| CSV 导出 | ✅ | `GET /v1/usage/events/export` → `ExportUsageEventsResponse.csv` |
| 低性价比模型识别 | ✅ | `InefficientModels` + `UsageOverview.inefficient_models` |
| 事件清理 | ✅ | `PurgeUsageEvents` RPC（`retain_days` 参数） |

### 2.2 前端

| 项 | 状态 | 证据 |
|----|------|------|
| 类型定义 | ✅ | `web/src/features/usage/types.ts` |
| API 调用层 | ✅ | `web/src/features/usage/api.ts`（含 snake_case ↔ camelCase 转换） |
| 限额 + 告警 API | ✅ | `web/src/features/usage/quotaApi.ts`（quota + budget alert 合并） |
| 限额 + 告警 composable | ✅ | `web/src/features/usage/useAgentUsageQuota.ts` |
| 概览页 composable | ✅ | `web/src/features/usage/useOverviewPage.ts` |
| 明细页 composable | ✅ | `web/src/features/usage/useUsageEventsPage.ts` |
| 核心指标卡片 | ✅ | `web/src/components/usage/UsageMetricCards.vue` |
| 趋势图 | ✅ | `web/src/components/usage/UsageTrendChart.vue` + `features/usage/usageTrendMetrics.ts` |
| Top 模型排行 | ✅ | `web/src/components/usage/UsageTopModels.vue` |
| Top Agent 排行 | ✅ | `web/src/components/usage/UsageTopAgents.vue` |
| 异常请求列表 | ✅ | `web/src/components/usage/UsageAnomalyList.vue` |
| 低性价比模型 | ✅ | `web/src/components/usage/UsageInefficientModels.vue` |
| 明细列表页 | ✅ | `UsageEventsPage.vue`（费用/来源/错误列） |
| 限额配置 UI | ✅ | `AgentUsageQuotaPanel`（权限 Tab）；Agent Tab 预算字段已弃用展示 |
| 告警配置 UI | ✅ | `AgentUsageQuotaPanel` 预算告警阈值 |
| CSV 导出 | ✅ | `UsageEventsPage` 导出按钮 |
| 月预算使用率卡片 | ✅ | `UsageMetricCards`（`quota_dashboard`） |
| 小时趋势 | ✅ | 概览「趋势粒度」→ `granularity=hour` |

### 2.3 差距总结（2026-06-17 复审后）

| 优先级 | 差距 | 影响 |
|--------|------|------|
| P3 | daily/hourly rollup 写入层与 billable 口径完全对齐 | 读层已过滤；写入 rollup 仍含 team_turn 维度 |
| P3 | Team 维度概览 API / 前端 Team 用量卡片 | 需 `team_id` 汇总接口或复用 events 聚合 |
| P3 | 价格自动同步（OpenRouter / Anthropic / Gemini / OpenAI API 定时拉取） | 当前仅 `syncProviderModelPricing` 手动触发 |
| P3 | Provider 独立定价 UI | 单价维护入口分散在模型页 |
| P3 | `cancelled` 流式中断落库路径未统一验证 | 验收 §5.1 部分项待补测 |

---

## 3. 开发阶段

### Phase 1：用量限额（P2）

目标：为 Agent / 用户 / 全局设置月度费用预算，超限拦截。

### Phase 2：前端补全（P2）

目标：独立明细列表页 + 限额配置 UI。

### Phase 3：用量告警 + 价格同步（P3）

目标：阈值告警通知 + 价格自动同步。

### Phase 4：增强分析（P3）

目标：小时聚合 + CSV 导出 + 低性价比模型识别。

---

## 4. 任务清单

### Phase 1：用量限额（P2）

| # | 任务 | 涉及层 | 说明 |
|---|------|--------|------|
| 1 | `usage_quotas` 表 + Ent Schema | Data | 新增 `usage_quotas` 表 |
| 2 | `UsageQuota` 领域模型 + Repo 接口 | Biz | `UsageQuotaRepo` 接口定义 |
| 3 | `UsageQuotaRepo` 实现 | Data | SQLite CRUD |
| 4 | Proto 新增 `GetUsageQuota` / `SetUsageQuota` / `CheckUsageQuota` | Proto | `usage/v1/usage.proto` |
| 5 | `UsageService` 新增 quota RPC | Service | proto ↔ biz 映射 |
| 6 | Chat turn 前检查 quota | Biz/Service | `TurnAdmissionUsecase.EnforceChatTurnQuotas`（`internal/biz/turn_admission.go`）；Service 层 `chat_orchestrator_turn.go` 调用 |
| 7 | Wire 注入 | Wire | `NewUsageQuotaRepo` / 扩展 ProviderSet |
| 8 | 单元测试 | Test | quota 检查逻辑 |

### Phase 2：前端补全（P2）

| # | 任务 | 涉及层 | 说明 |
|---|------|--------|------|
| 9 | 独立明细列表页组件 | Web | `UsageEventsPage.vue`，支持筛选/分页/排序 |
| 10 | 限额配置 API + 组件 | Web | `quotaApi.ts` + `AgentUsageQuotaPanel.vue` |
| 11 | 概览页月预算使用率卡片 | Web | 当 quota 存在时展示使用率 |

### Phase 3：用量告警 + 价格同步（P3）

| # | 任务 | 涉及层 | 说明 |
|---|------|--------|------|
| 12 | `budget_alerts` 表 + Ent Schema | Data | 新增 `budget_alerts` 表 |
| 13 | `BudgetAlert` 领域模型 + Repo 接口 | Biz | `BudgetAlertRepo` 接口定义 |
| 14 | `BudgetAlertRepo` 实现 | Data | SQLite CRUD |
| 15 | Proto 新增 `ListBudgetAlerts` / `SetBudgetAlert` | Proto | `usage/v1/usage.proto` |
| 16 | `UsageService` 新增 alert RPC | Service | proto ↔ biz 映射 |
| 17 | 告警触发逻辑 | Biz | `RecordTokenUsageEvent` 后检查阈值 |
| 18 | 告警通知 | Service | 系统通知（EventBus → 前端 WebSocket） |
| 19 | 告警配置 API + 组件 | Web | `quotaApi.ts` 合并告警 + `AgentUsageQuotaPanel` 告警阈值 |
| 20 | 价格自动同步 | Data | OpenRouter / Anthropic / Gemini / OpenAI API 定时拉取 |

### Phase 4：增强分析（P3）

| # | 任务 | 涉及层 | 说明 |
|---|------|--------|------|
| 21 | `model_token_usage_hourly` 表 + upsert | Data | 小时聚合，同 daily 结构 |
| 22 | 小时级趋势查询 API | Biz/Service | `UsageQuery` 增加 granularity 参数 |
| 23 | CSV 导出 | Service/Web | 明细列表导出 |
| 24 | 低性价比模型识别 | Biz | 高成本 + 低 TPS + 高失败率标记 |

---

## 5. 验收标准

### Phase 1

- [x] 可为 **Agent** 设置月度费用预算（`usage_quotas`）
- [x] 用户 / 全局 scope（`CheckQuota` 支持 `agent` / `user` / `global`）
- [x] 超过预算后 Agent / Team 对话被拦截（`USAGE_QUOTA`）
- [x] 周期由 `period_start` / `period_end` 界定（保存时默认当月）
- [x] quota 延迟基准测试（`internal/biz/usage_quota_bench_test.go`）

### Phase 2

- [x] 独立明细列表页（筛选；表格分页）
- [x] 限额配置 UI（权限 Tab `AgentUsageQuotaPanel`）
- [x] 概览页展示月预算使用率（#11）

### Phase 3

- [x] 达到告警阈值时写入监控（`usage.budget_alert`）
- [x] 告警阈值可配置（Agent 权限 Tab）
- [ ] 价格规则可从 Provider API 自动同步（当前仅手动触发）

### Phase 4

- [x] 小时级趋势查询可用（`granularity=hour`）
- [x] CSV 导出功能可用
- [x] 低性价比模型可被识别和标记（`InefficientModels` + `UsageInefficientModels.vue`）

---

## 6. 依赖与风险

| 风险 | 影响 | 缓解 |
|------|------|------|
| Quota 检查增加 Chat turn 延迟 | 用户体验 | 1 次 DB 查询，预期 < 10ms；可加内存缓存 |
| Quota 重置周期边界 | 月末/月初可能重复计算 | 使用 `period_start` / `period_end` 明确界定 |
| 价格自动同步 API 变更 | 同步失败 | 保留 manual fallback；同步失败不阻塞 |
| 告警通知频率过高 | 通知疲劳 | 去重 + 频率限制（60min 冷却） |

---

## 7. 迭代：用量三页与配额链路（2026-05-20）

**目标**：统一概览（聚合）、用量事件（明细）、监控（系统可观测）职责；使 `usage_quotas` 与 `total_cost_micro_usd` 在 Chat/Team Turn 前真实生效。

**架构**：写入时 `RecordTokenUsageEvent` 归一化 `status`、按 `model_pricing_rules` 补单价并计算 micro-USD；查询侧 `usageWhere` 状态别名；Team 在 `RunTurn` 前对启用成员 `CheckQuota`；配额配置仅 **Agent 设置 → 权限 Tab**（`usage_quotas`），弃用 Agent Tab `budget_monthly_cents` 展示。

### 7.1 已完成

| 域 | 项 | 证据 |
|----|-----|------|
| Biz/Data | `usage_cost.go`、`GetActiveModelPricing`、`enrichTokenUsagePricing` | `internal/biz/usage/usage.go` |
| Service | `TurnAdmissionUsecase` 配额拦截 | `internal/biz/turn_admission.go`、`internal/service/chat_orchestrator_turn.go`、`team_turn_hooks.go` |
| Web | `UsageEventsPage` 费用/来源/错误列；权限 Tab 配额 | `web/src/pages/UsageEventsPage.vue`、`AgentUsageQuotaPanel.vue` |
| 文档 | 三页职责 | `frontend-pages.md` §4.2 |

### 7.2 验收（本迭代）

1. 保存 Agent 配额后 `usage_quotas` 有行；Chat Turn 超限返回 `USAGE_QUOTA`。
2. Provider 模型含单价时，新写入 `model_token_usage_events.total_cost_micro_usd > 0`。
3. `/usage/events` 可按 `success`/`error` 筛选历史 `ok`/异常行。
4. Team 会话任一启用成员超限则整轮拒绝。

### 7.3 迭代二（2026-05-20 下午）

| 域 | 项 | 证据 |
|----|-----|------|
| Team | 成员 step + 整轮聚合 | `internal/team/usage_record.go`、`usage_tokens.go`、`agent/turn_helpers.go`、`persistStep` |
| Data | `budget_alerts`、`model_token_usage_hourly` | `ent/schema/budget_alert.go`、`model_token_usage_hourly.go`、`usage_quota.go` |
| API | `quota_dashboard`、`ListBudgetAlerts`、`ExportUsageEvents`、`granularity` | `usage.proto` |
| 定价 | Provider `config_json` 回退 | `usage_pricing.go` |
| Web | 配额卡片、小时趋势、CSV、告警 UI | `UsageMetricCards`、`OverviewPage`、`UsageEventsPage`、`AgentUsageQuotaPanel` |

### 7.4 待办（P3）

- [ ] 价格规则自动同步（OpenRouter / Anthropic / Gemini / OpenAI API 定时拉取）
- [ ] Provider 模型页独立定价编辑 UI
- [ ] `cancelled` 流式中断落库路径统一验证
- [ ] daily/hourly rollup 写入层与 billable 口径完全对齐
- [ ] Team 维度概览 API / 前端 Team 用量卡片

---

## 8. 代码审计与架构复审（2026-05-20）

### 8.1 架构符合度

| 层级 | 评价 | 说明 |
|------|------|------|
| Proto / Service | ✅ | `UsageService` 薄映射；Chat 配额在 `TurnAdmissionUsecase`（biz）+ `ChatOrchestrator`（service），不污染 `biz` 对框架的边界 |
| Biz | ✅ | `UsageUsecase` + `UsageRepo`；费用纯函数在 `usage_cost.go`；配额拦截独立为 `TurnAdmissionUsecase` |
| Data | ✅ | 单表事务写入 + 日聚合；定价查询独立 `usage_pricing.go` |
| Web | ✅ | `features/usage/*` 与页面组件分离；配额 composable 可复用 |

### 8.2 已修复问题（本复审）

| 问题 | 严重度 | 修复 |
|------|--------|------|
| `nativeSendChatMessage` 与 `recordTurnUsage` **双写**用量 | 高 | 移除 ingress 调用；ingress 默认关（`CHAT_RECORD_USAGE_INGRESS=1` 才写） |
| EventBus Runner 用量缺 `id` 导致写入失败 | 高 | `event_bus_runner_handler` 补 `uuid` + `usage_kind` |
| 聚合 SQL 未计 `ok`/`error` 历史状态 | 中 | `usage_status_sql.go` 统一 `IN (...)` |
| `error` 状态未归一为 `failed` | 中 | `NormalizeUsageStatus` |
| 概览缺「查看明细」入口 | 低 | `OverviewPage` → `/usage/events` |

### 8.3 仍须关注

| 项 | 建议 |
|----|------|
| 主写入路径 | **`recordTurnUsage`**（`usage_kind=chat_turn`，含 provider/agent/定价） |
| Runner 辅助路径 | `CHAT_RECORD_RUNNER_USAGE=1` 时 EventBus 写入，字段较少 |
| 费用为 0 | Provider 模型未配置单价 → 配额 SUM 无效；需在 `/models` 维护价格 |
| `agents.budget_monthly_cents` | 未接入 `CheckQuota`，仅 DB 兼容 |
| Team 用量明细 | `team_member`：`ConsumeEventStream` 按 `agent_key` 汇总 `MemberUsage` 后 `persistStep` 写入；无成员级 usage 时 anchor（sortIdx=0）回退整轮 tokens |
| 聚合重复计费 | 读层默认排除 `team_turn` | `usage_sql.go` + `usageWhere(..., true)`；明细 `billableOnly=false` |
| 配额拦截位置 | 已从 Service 层 `chat_quota.go`（已删除）重构为 Biz 层 `TurnAdmissionUsecase`（`internal/biz/turn_admission.go`），Service 层通过 `ChatOrchestrator.admission()` 调用 |

### 8.4 写入路径（真相源）

```
Chat Turn 结束 → service/turn_usage.recordTurnUsage
              → biz.RecordTokenUsageEvent（归一 status + 定价 + 费用）
              → data/usage_write（events + sessions 聚合 + daily/hourly upsert）

Team RunTurn 结束 → agent.ConsumeEventStream（MemberUsage 按 agent_key）
                 → team.persistStep → recordMemberUsage（usage_kind=team_member）
                 → team.recordTeamRunUsage（usage_kind=team_turn，整轮聚合）

（可选）CHAT_RECORD_RUNNER_USAGE=1 → event_bus_runner_handler
（已停用默认）CHAT_RECORD_USAGE_INGRESS=1 → recordChatIngressUsage
```

### 8.5 规范复盘修复（2026-05-20）

| 问题 | 规范条目 | 修复 |
|------|----------|------|
| `NewUsageService` 构造器内 `SetAlertNotifier` 副作用 | 后端 DI 应集中在 Wire | `provideUsageUsecase`（`cmd/admin/wire.go`） |
| `Overview` 忽略 `granularity`，前端双请求 | API 语义一致 | `UsageUsecase.Overview` 改调 `Trends()` |
| Team 用量写入 `_` 吞错 | FlowLog v2（禁 slog） | `event.CtxFlowLogWarn` → `team.usage_record_fail` |
| 手改 `cmd/admin/wire_gen.go` | 红线 #11 | 仅改 `wire.go` + `make wire` / `make all` |
| Page 直接 `import features/usage/api` | 前端红线 #11 | `useOverviewPage` / `useUsageEventsPage` + `useUsageStore` |
| `AgentUsageQuotaPanel` 直接调 API | 红线 #2/#4 | 告警并入 `useAgentUsageQuota` + `quotaApi` |
| `budgetAlertApi.ts` 碎片化 | §1.2 域内 api 收敛 | 合并进 `quotaApi.ts` |
| Service 层 `chat_quota.go` 直接调 `CheckQuota` | 配额拦截应内聚到 Biz | 重构为 `TurnAdmissionUsecase`（`internal/biz/turn_admission.go`），Service 层通过 `ChatOrchestrator.admission()` 调用 |

### 8.6 待优化清单（P2/P3）— 2026-05-20 已落地

| # | 项 | 状态 | 落地说明 |
|---|-----|------|----------|
| O1 | `budget_alerts` / `hourly` Ent Schema | ✅ | `ent/schema/budget_alert.go`、`model_token_usage_hourly.go`、`usage_quota.go`；移除 `EnsureUsageExtraSchema` / `usage_schema_extra.go`；`make generate` |
| O2 | `data.SetBudgetAlert` 错误类型 | ✅ | `biz.ErrBudgetAlertNotFound`、`ErrUsageScopeRequired`；`mapUsageRepoErr` |
| O3 | `EvaluateBudgetAlerts` 热路径成本 | ✅ | `scheduleBudgetAlerts` → `safego.Go`；`TotalCostMicroUSD<=0` 跳过 |
| O4 | Team 并行成员 tokens=0 | ✅ | `EventStreamResult.MemberUsage`（`ev.Author` + `Usage`）；`stepTokensForMember` → `persistStep` → `team_member`；整轮聚合仍写 `team_turn`；框架未上报成员 usage 时 anchor 回退 |
| O5 | user/global `usage_quotas` scope | ✅ | `SumScopeCostInPeriod` + `quotaSpent`；Chat Turn 前 `TurnAdmissionUsecase.EnforceChatTurnQuotas`；**全局配额**在 **系统设置** `/settings` 配置（`global_monthly_micro_usd` → 同步 `usage_quotas` global/global） |
| O6 | 低性价比模型识别（#24） | ✅ | `InefficientModels` + `UsageOverview.inefficient_models`；`UsageInefficientModels.vue` |
| O7 | Provider 独立定价 UI | 暂缓 | `/models`（`ResourceManagerPage`）已可维护单价；独立定价页非本期 |
| O8 | `quota` 延迟基准测试 | ✅ | `internal/biz/usage_quota_bench_test.go`；`go test -bench=BenchmarkCheckQuota` |

**生成物纪律**：Proto/API/Wire 变更后执行 `make all`（或 `make api` + `make wire`），**禁止**手改 `wire_gen.go`、`api/**/*.pb.go`、`web/src/services/**` 生成 TS。

**可观测性纪律**：`internal/` 业务路径禁 `slog`；用量/Team 失败用 `CtxFlowLogWarn`（步骤注册表见 `52-flow-logger.design.md` §5.1）。

---

## 9. Team 统计整合（2026-05-20）

**目标**：平台可计费口径 = `chat_turn` + `team_member`，`team_turn` 仅对账；Team 整体 = 同 `team_id` 的 `team_member` 之和。需求 §3.6 · 设计 §4.5。

### 9.1 任务与状态

| 优先级 | 任务 | 层 | 状态 | 证据 |
|--------|------|-----|------|------|
| P0 | 文档：需求 §3.6、设计 §4.5、本 §9 | Docs | ✅ | 三文档边界已拆分 |
| P1 | 读层 `sqlUsageBillableKind` + `usageWhere(billableOnly)` | Data | ✅ | `usage.go`、`usage_quota.go`、`usage_hourly.go` |
| P1 | `UsageQuery.team_id` / `usage_kind`；Proto 字段 | API/Biz | ✅ | `usage.proto`、`usage_mapper.go`、`biz/usage/usage.go` 常量 |
| P1 | 单元测试 billable WHERE | Test | ✅ | `usage_where_test.go` |
| P2 | 明细页筛选 `team_id`、`usage_kind` | Web | ✅ | `UsageEventsPage`、`types.ts`、`api.ts` |
| P3 | daily/hourly upsert 不写或 rollup 查询排除 `team_turn` | Data | 暂缓 | 读层已正确；写入层对齐见 §7.4 |
| P3 | `GetTeamUsageSummary(team_id)` 或概览 Team 卡片 | Biz/Service/Web | 待办 | 产品需 Team 页展示时再开 |

### 9.2 验收

1. 同一 Team 一轮 parallel 运行：概览/Top Agent 费用 ≈ 各 `team_member` 之和，**不**再加 `team_turn`。
2. `/usage/events` 可筛 `usage_kind=team_turn` 且仍出现在列表。
3. `CheckQuota` / `SumScopeCostInPeriod` 与概览 SUM 使用同一 billable 子句。
4. `go test ./internal/data/... -run UsageWhere` 通过。

### 9.3 与 execution-plan

进度登记见 [execution-plan.md](../guides/execution-plan.md) M4 Token 行；changelog [2026-05-20-Usage-Quota-Events.md](../changelog/2026-05-20-Usage-Quota-Events.md) 迭代七（billable 读层）。

---

## 10. 改动文件清单

### 后端

| 文件 | 说明 |
|------|------|
| `api/kratos/usage/v1/usage.proto` | Proto 契约（13 个 RPC） |
| `internal/biz/usage/usage.go` | Usecase + 领域模型 + Repo 接口 |
| `internal/biz/usage/pricing_snapshot.go` | 定价快照 |
| `internal/biz/usage/export_test.go` | 导出测试辅助 |
| `internal/biz/usage.go` | 类型别名透传到 `biz` 包 |
| `internal/biz/turn_admission.go` | 配额拦截 usecase |
| `internal/biz/event_bus_runner_handler.go` | Runner 用量辅助路径 |
| `internal/biz/team_types.go` | Team 用量类型 |
| `internal/data/usage.go` | Repo 实现 |
| `internal/data/usage_write.go` | 写入流程 |
| `internal/data/usage_daily.go` | 日聚合 |
| `internal/data/usage_hourly.go` | 小时聚合 |
| `internal/data/usage_quota.go` | 配额 Repo |
| `internal/data/usage_budget_alert.go` | 告警 Repo |
| `internal/data/usage_pricing.go` | 定价查询 |
| `internal/data/usage_sql.go` | SQL 常量 + 查询构建 |
| `internal/data/usage_breakdown_alias.go` | 占比别名 |
| `internal/data/ent/schema/usage_quota.go` | Ent Schema |
| `internal/data/ent/schema/budget_alert.go` | Ent Schema |
| `internal/data/ent/schema/model_token_usage_hourly.go` | Ent Schema |
| `internal/data/ent/schema/model_pricing_rule.go` | Ent Schema |
| `internal/data/sql/migrations/20260717_usage_events_schema.sql` | DDL 迁移（events + daily） |
| `internal/data/sql/migrations/20260612_pricing_rule_patches.sql` | DDL 补丁（cache_write 等） |
| `internal/service/usage.go` | UsageService |
| `internal/service/usage_mapper.go` | Proto ↔ Biz 映射 |
| `internal/service/usage_alert_notifier.go` | AlertNotifier 实现 |
| `internal/service/turn_usage.go` | 主写入路径 |
| `internal/service/chat_usage_ingress.go` | 遗留 ingress 路径 |
| `internal/service/chat_orchestrator_turn.go` | 配额拦截调用 |
| `internal/service/chat_orchestrator_turn_metrics.go` | Team 成员配额包装 |
| `internal/service/team_turn_hooks.go` | Team 整轮配额 |
| `internal/team/usage_record.go` | Team 用量记录 |
| `internal/team/usage_tokens.go` | Team Token 计算 |
| `cmd/admin/wire.go` | Wire 注入 + 窄接口适配器 |

### 前端

| 文件 | 说明 |
|------|------|
| `web/src/features/usage/api.ts` | API 调用层 |
| `web/src/features/usage/quotaApi.ts` | 限额 + 告警 API |
| `web/src/features/usage/types.ts` | TypeScript 类型 |
| `web/src/features/usage/useOverviewPage.ts` | 概览页 composable |
| `web/src/features/usage/useUsageEventsPage.ts` | 明细页 composable |
| `web/src/features/usage/useAgentUsageQuota.ts` | 限额 + 告警 composable |
| `web/src/features/usage/useUsageChart.ts` | 趋势图 composable |
| `web/src/features/usage/useProviderTrendDialog.ts` | Provider 趋势弹窗 |
| `web/src/features/usage/usageTrendMetrics.ts` | 趋势指标 |
| `web/src/features/usage/usageTableUi.ts` | 表格 UI |
| `web/src/features/usage/usageEcharts.ts` | ECharts 配置 |
| `web/src/features/usage/usageBreakdownSlices.ts` | 占比切片 |
| `web/src/features/usage/pricingWarning.ts` | 定价警告 |
| `web/src/features/usage/moneyFormat.ts` | 金额格式化 |
| `web/src/components/usage/*.vue` | 用量组件（24 个） |
| `web/src/components/agents/AgentUsageQuotaPanel.vue` | 限额 + 告警配置 |
| `web/src/pages/OverviewPage.vue` | 概览页 |
| `web/src/pages/UsageEventsPage.vue` | 明细页 |

### 测试

| 文件 | 说明 |
|------|------|
| `internal/biz/usage/usage_test.go` | Usecase 测试 |
| `internal/biz/usage/usage_more_test.go` | Usecase 补充测试 |
| `internal/biz/usage/usage_usecase_test.go` | Usecase 测试 |
| `internal/biz/usage/usage_usecase_more_test.go` | Usecase 补充测试 |
| `internal/biz/usage/usage_internal_test.go` | 内部测试 |
| `internal/biz/usage_quota_test.go` | 配额测试 |
| `internal/biz/usage_quota_bench_test.go` | 配额基准测试 |
| `internal/biz/usage_cost_test.go` | 费用计算测试 |
| `internal/biz/usage_inefficient_test.go` | 低性价比模型测试 |
| `internal/data/usage_write_test.go` | 写入测试 |
| `internal/data/usage_where_test.go` | 查询构建测试 |
| `internal/data/usage_breakdown_alias_test.go` | 占比别名测试 |
| `internal/service/usage_mapper_test.go` | 映射测试 |
| `internal/team/usage_tokens_test.go` | Team Token 测试 |
