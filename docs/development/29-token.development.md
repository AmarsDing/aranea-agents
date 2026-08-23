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
| 明细列表页 | ✅ | `UsageEventsPage.vue`（费用/来源/错误列；名称列 + 状态标签本地化见 §12） |
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

**目标**：平台可计费口径 = `chat_turn` + `team_member`，`team_turn` 仅对账；Team 整体 = 同 `team_id` 的 `team_member` 之和。需求 §3.6 · 设计 §4.6。

### 9.1 任务与状态

| 优先级 | 任务 | 层 | 状态 | 证据 |
|--------|------|-----|------|------|
| P0 | 文档：需求 §3.6、设计 §4.6、本 §9 | Docs | ✅ | 三文档边界已拆分 |
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

---

## 11. 费用计算修复与历史回填（2026-07-25）

**背景**：用户反馈 Token 总量与官网一致但费用偏差大。排查发现 4 类根因，全部修复。

### 11.1 根因与修复

| # | 根因 | 修复 | 层 | 状态 |
|---|------|------|-----|------|
| 1 | 缓存命中 token 未从输入 token 中扣除，且未按缓存价计费 | `ApplyTokenUsageCosts` 改为 `(input − cached_billable) × 输入价` + `cached_billable × 缓存价`；单价优先 USD/1M | Biz | ✅ |
| 2 | DeepSeek `prompt_cache_hit_tokens` 不被 OpenAI SDK 解析，cached 恒为 0 | 新增 `internal/provider/usage_tap_transport.go`，传输层改写为标准 `prompt_tokens_details.cached_tokens`（SSE/JSON），装配于 `trpc_llm.go`；`turn_helpers.go` 累计 `CachedTokens` | Provider/Agent | ✅ |
| 3 | `PersistGraphRunStep` 硬编码空 provider/model → 355 条 `team_member` 事件 `model_api_id` 为空、无法定价 | 以 anchor Agent `Provider`/`Model` 兜底（`FirstNonEmpty`） | Team | ✅ |
| 4 | 缺 `openrouter/gpt-4.1-mini`、`deepseek/deepseek-chat` 定价规则 → 324 条事件费用为 0；DeepSeek 缓存价误配 0.014（官网 0.0028，2026-04-26 起生效） | 插入 2 条 `model_pricing_rules`（manual）；deepseek 规则与 `llm_provider_models.config_json` 缓存价改回 0.0028（用户确认按官网价） | Data | ✅ |

### 11.2 历史数据回填（一次性，2026-07-25 已执行）

- 324 条 model 非空但 cost=0 事件：按规则重算价格快照与费用（251 openrouter/gpt-4.1-mini + 34 deepseek-chat + 39 deepseek-v4-flash）。
- 355 条空 model `team_member` 事件：token 与兄弟 `team_turn` 事件全等（355/355 验证），从其复制 provider/model/价格/费用。
- 受影响日期（06-19 起 27 天）daily/hourly 汇总表删除后由 events 全量重建（daily 192 行 / hourly 563 行）。
- 验收：cost=0 事件清零；events 费用总和 = daily 汇总总和 = 8,684,060 micro USD。

### 11.3 改动文件（本迭代新增）

| 文件 | 说明 |
|------|------|
| `internal/provider/usage_tap_transport.go` | DeepSeek 缓存命中响应改写 RoundTripper |
| `internal/provider/trpc_llm.go` | 装配 usageTapTransport |
| `internal/biz/usage/usage.go` | `ApplyTokenUsageCosts` 缓存扣除公式 |
| `internal/agent/turn_helpers.go` | `CachedTokens` 累计 |
| `internal/team/team_graph_run_finisher.go` | anchor provider/model 兜底 |

---

## 12. 明细页可读性改造（2026-07-30）

**背景**：用户反馈用量事件页可用性差——Provider/模型/Agent/Team 需手输 ID、表格 Agent/Session 列显示裸 ID 看不懂、操作按钮溢出横向滚动条、状态标签为英文硬编码。

### 12.1 任务与状态

| # | 任务 | 层 | 状态 | 证据 |
|---|------|-----|------|------|
| 1 | `TokenUsageEvent` 增加 `agent_name` / `session_title` / `team_name`（proto 51–53 + biz + mapper） | API/Biz/Service | ✅ | `usage.proto`、`internal/biz/usage/usage.go`、`internal/service/usage_mapper.go` |
| 2 | `ListModelUsageEvents` 标量子查询解析显示名（`sqlUsageEventNames`） | Data | ✅ | `internal/data/usage.go`；设计 §4.4 |
| 3 | 筛选项改下拉：Provider/模型（platformStore 派生 + 联动）、Agent/Team（目录 Store，显示名称提交 ID） | Web | ✅ | `useUsageEventsPage.ts` |
| 4 | 表格名称列：Agent 主行名称 + 次要行 ID；Session 标题 + 悬停完整 ID | Web | ✅ | `UsageEventsPage.vue` |
| 5 | 状态标签公共组件 + i18n（zh-CN / en-US） | Web | ✅ | `components/common/AppStatusChip.vue`、`features/ui/appStatusMeta.ts`、`locales/*` `common.status.*` |
| 6 | 工具栏紧凑化：字段收窄 + `flex-wrap` 换行，消除横向滚动条 | Web | ✅ | `UsageEventsPage.vue` scoped 样式 |

### 12.2 验收

1. 明细页 Provider / 模型 / Agent / Team 均为下拉选择，显示名称、提交 code/ID；模型选项随 Provider 联动。
2. 表格 Agent / Session / Team 列显示名称；ID 经次要行或悬停查看；已删除实体回退显示 ID。
3. 状态列标签随界面语言切换中/英文；`AppStatusChip` 可供其他页面复用。
4. 工具栏在常规宽度下无横向滚动条，空间不足时换行。
5. `make api` + `go build ./...` + data/biz/service usage 测试通过；前端 eslint（任务文件）/ `check:layer` / vitest / `pnpm build` 通过。

### 12.3 改动文件（本迭代新增）

| 文件 | 说明 |
|------|------|
| `api/kratos/usage/v1/usage.proto` | `TokenUsageEvent` 增加字段 51–53 |
| `internal/biz/usage/usage.go` | `TokenUsageEvent` 增加 `AgentName` / `SessionTitle` / `TeamName` |
| `internal/data/usage.go` | `sqlUsageEventNames` 标量子查询 + scan 追加 3 列 |
| `internal/service/usage_mapper.go` | proto 映射追加 3 字段 |
| `web/src/features/usage/types.ts` | `ModelTokenUsageEvent` 增加 3 字段 |
| `web/src/features/usage/api.ts` | 响应映射（snake/camel 双兼容） |
| `web/src/features/usage/useUsageEventsPage.ts` | 下拉选项（provider/model/agent/team）+ Provider↔模型联动 + statusOptions 复用 appStatusMeta |
| `web/src/pages/UsageEventsPage.vue` | 筛选改 `q-select`、名称列模板、`AppStatusChip`、紧凑工具栏样式 |
| `web/src/components/common/AppStatusChip.vue` | **新增** 公共状态标签组件 |
| `web/src/features/ui/appStatusMeta.ts` | **新增** 状态枚举 → tone/图标/i18n key 元数据 |
| `web/src/i18n/locales/zh-CN.ts` / `en-US.ts` | `common.status.*` 双语词条 |

---

## 13. 缓存命中率护栏与上下文预算台账（阶段 0，2026-08-13 立项）

> 设计：[29-token.design.md §九](./29-token.design.md#九缓存命中率护栏与上下文预算台账阶段-0-设计2026-08-13) ｜ 调研：`docs/reports/2026-08-13-research-llm-context-pipeline-optimization.md`
> 性质：纯观测性改造，不改注入逻辑；阈值默认 0.5（已确认）。

### 13.1 任务与状态

| # | 任务 | 层 | 状态 | 证据 |
|---|------|-----|------|------|
| 0.1 | ContextBudget 收集器 + 9 处 hook 计量 + `chat.context_budget` 台账日志 | Agent/Service | ✅ | `internal/agent/context_budget.go`、7 测试 PASS；`go build ./cmd/... ./internal/... ./api/... ./pkg/...` exit 0 |
| 0.2 | 前缀字节级稳定回归测试 `prompt_prefix_stability_test.go`（离线） | Agent | ✅ | 3 用例 PASS；发现整链消息顺序：静态区=[base system, static cue]（N=2），dynamic capability cue 为 LayerSemiStatic |
| 0.3 | `CacheHitRatioStats` 聚合查询（biz 窄接口 + data SQL，排除 prompt<1024 样本） | Biz/Data | ✅ | `internal/biz/usage/cache_hit.go`、`internal/data/usage_cache_hit.go`；`TestCacheHitRatioStats_Aggregates`/`_Empty` 真 PG PASS（2026-08-13） |
| 0.4 | `llm.cache_hit_ratio_low` 告警规则（1h 窗、样本≥20、阈值默认 0.5 可配） | Biz/Monitor | ✅ | `internal/biz/monitor/alert_metric_cache_hit.go`；`go test ./internal/biz/monitor -run TestCacheHitRatio` PASS；Wire 装配入告警注册表（2026-08-13） |
| 0.5 | 运行时验证：重启后端 → 真实对话 → 台账分量合理 | Service | ✅ | 见 §13.1.2 |
| 0.6 | turn 级缓存命中落库：`session_turns.cached_input_tokens` 列（方案 B，turn 详情/评测证据免 join 即得命中率） | Ent/Data/Service | ✅ | `internal/data/ent/schema/session_turn.go` + 迁移 `20261239_session_turn_cached_input`（幂等，已在 aranea-postgres 验证重跑跳过）；`RecordSessionTurn` Create/Update 双路径无条件写入（0 是有意义观测）；单聊/团队调用点均传 `CachedTok`；`chat_turn_metrics_session_turn_test.go` 2 用例 PASS（2026-08-22） |
| 0.7 | `GetCacheHitRatioStats` RPC（`GET /v1/usage/cache-hit-ratio-stats`，`window_hours` 默认 1 clamp [1,168]，平台级门禁同 budget alerts） | API/Service/Biz | ✅ | `api/kratos/usage/v1/usage.proto`；biz `Usecase.CacheHitRatioStats` 窄接口委托（无能力返回空）；service clamp+mapper；biz 3 用例 + service 7 用例 PASS；聚合 SQL 在 aranea-postgres 手工验证（窗口/最小 prompt/team_turn 排除、P50 插值全对）（2026-08-22） |
| 0.8 | 前端命中率卡片 `UsageCacheHitRatio.vue`（OverviewPage，1h/24h/7d 窗口切换，P50<0.5 红色低水位，403 静默隐藏） | Web | ✅ | 数据流经 `stores/usage.loadCacheHitStats`（R-FE1 分层合规）；`stores/__tests__/usage.spec.ts` 3 用例 PASS；eslint/vue-tsc 干净（2026-08-22） |
| 0.9 | 方案 B 全量验证收口 | 全栈 | ✅ | `make api`/`make wire` 重跑无新 diff、`make build` exit 0；`go test -race` biz/usage + service + data 三包全过（PG 测试库须 `ARANEA_TEST_PG_DSN` 指向 twinserver-postgres 5432 密码 123456，默认 DSN 的 Hangshan@123 已失效）；前端 vue-tsc/eslint/vitest/i18n 键一致性（4616 zh / 4599 en）全过（2026-08-22）。既有问题不归属本改动：lint R7 `twin_openapi_compat.go` mux.HandleFunc（8-17 提交既有）、`chat_orchestrator_turn.go:722` vet intentCancel（并发会话改动）、`useUsageEventsPage.ts:223` 等 vue-tsc 错误（HEAD 既有） |

### 13.1.1 运行时基线测量（2026-08-13，68 样本，日志 log-2026-08-11~12）

| agent | model | n | avg ratio | p50 | min | avg prompt |
|---|---|---|---|---|---|---|
| `__spirit__` | deepseek-v4-flash | 20 | **0.533** | 0.801 | 0.000 | **60,180** |
| `__voice_butler__` | deepseek-v4-flash | 43 | 0.701 | 0.992 | 0.000 | 17,457 |
| `__memory__` | deepseek-v4-flash | 5 | 0.995 | 0.995 | 0.991 | 26,303 |

**基线结论**：
1. `__spirit__` 单轮 prompt 平均 60K token 且命中率均值仅 0.533（大量 0.000/0.07-0.19 样本，命中时也只命中前 ~4-10K）——**缓存击穿真实存在，且 60K 已进入 context rot 区间**（RULER 有效上下文拐点 32-64K），印证 P0-3 与 P2-6 的紧迫性。
2. `__voice_butler__` 呈双峰分布：~0.99 或 ~0.26（5760/20K，固定断点）或 0——疑似 TTL 过期或前缀在固定位置断裂，待阶段 1 结合 `chat.context_budget` 台账定位。
3. `__memory__`（sleep-time 提取）0.995 稳定——前缀稳定化机制本身有效，问题集中在 spirit/voice 主链路的动态区。
4. 告警阈值 0.5 合理性确认：spirit 均值 0.533 恰在阈值线，足以捕捉回归。

### 13.1.2 运行时验证：chat.context_budget 台账（2026-08-13 13:38，任务 0.5）

后端以新构建重启（`bin/admin.exe`，config.local.yaml + PG 便携实例），真实用户对话流量（`__memory__` 离线整理会话 e26c2ef6）下 `bin/logs/aranea-pipeline.log` 产出台账：

```json
{"step_id":"chat.context_budget","agent_key":"__memory__","run_id":"0bde5c8d-...",
 "static_prefix_tokens":286,"tools_schema_tokens":21076,"tools_count":53,
 "memory_l1_tokens":0,"memory_l4_tokens":333,"memory_composite_tokens":465,
 "knowledge_cue_tokens":184,"skill_guidance_tokens":212,"other_dynamic_tokens":99,
 "est_total_input":22655,"static_ratio":0.0126,"cache_hit_ratio":0.9947}
```

**验证结论**：
1. **字段完整**：9 类分量 + tools_count + static_ratio + cache_hit_ratio 全部产出，与设计 §9.6 一致。
2. **数值自洽**：各分类求和 = est_total_input（286+21076+184+465+0+333+212+99 = 22655 ✓），无漏计/重复计量。
3. **与 provider 缓存互证**：cache_hit_ratio=0.9947，与台账稳定区（static_prefix+tools_schema=21362，占 94.3%）吻合——剩余 ~5% 为连续两轮间亦保持稳定的历史/cue 尾部。
4. **关键量化发现**：`tools_schema_tokens` 占输入 **93%**（21076/22655，53 个工具全量挂载）——工具 schema 膨胀被台账直接量化，为阶段 1 Tool RAG（验收基准：tools_schema_tokens 下降 ≥80%）提供基线。
5. 注意：static_ratio 仅计量静态前缀区，不含 tools_schema；解读"可缓存占比"时应以 static_prefix+tools_schema 合并口径为准（本样本 94.3% ≈ 实测缓存命中 99.5% 的下界）。

### 13.2 验收

1. ✅ 单测：收集器聚合（7 例）、告警阈值边界（<1024 排除，biz/monitor PASS）、前缀稳定测试灵敏度（人为变动字节应失败，3 例 PASS）。
2. 全量 `make test` 留待阶段 0 收尾统一执行（本次范围内相关包：agent/service/biz/monitor/data 定向测试均 PASS）。
3. ✅ 运行时验证：见 §13.1.2——台账字段完整、分量求和自洽、cache_hit_ratio 与台账稳定区互证。
4. 阶段 1 Tool RAG 上线前后同会话 `tools_schema_tokens` 下降 ≥80%（作为该优化项验收基准）。

## 14. 工具调用链路综合修复（2026-08-13，WP-1~WP-4）

背景：基于生产库 `tool_invocations`（9507 次调用）+ `model_token_usage_events` + `chat.context_budget` 台账的全量分析（详见 `docs/reports/2026-08-13-research-llm-context-pipeline-optimization.md` 及当次会话分析），总体工具调用失败率 5.1%，其中执行层（hostexec 0.1%、file 类 1-3.5%）已健康，失败大头为编排层（synthesize_results 71.7%、subagents_* 56-67%）；spirit 缓存命中率 0.533 且大量 0.000 样本。

### 14.1 评审结论（原方案 3 个架构矛盾）

| # | 矛盾 | 修正 |
|---|------|------|
| M1 | Tool RAG 动态工具集进 tools 块 = 每轮字节级击穿前缀缓存（tools 块在请求最前） | tools 块只放会话内字节稳定的核心集；长尾走两段式（见 §14.4） |
| M2 | 「Declaration 动态写入当前状态」同样击穿缓存 | 前置条件写静态文本；动态状态走消息末尾 cue 或拦截时结构化返回 |
| M3 | 缓存 0.533 根因未定位 | 已定位：`DynamicRuntimeCapabilityCue` 内容每轮可变（"Effective tool keys this turn"、条件性 spirit fallback 行）却被 `insertAfterLastSystem` 钉在前缀区（runtime_cue_inject.go），且测试把错误位置 pin 成了「契约」 |

新增架构原则 **P-1 缓存不可侵犯**：请求头部（tools 块 + system + insertAfterLastSystem 区）只允许会话内字节稳定的内容；一切动态内容 append 到消息末尾尾部区。

### 14.2 任务与状态

| # | 任务 | 状态 | 说明 |
|---|------|------|------|
| WP-1 | dynamic runtime cue 移到消息末尾（append） | ✅ | runtime_cue_inject.go L74 insertAfterLastSystem→append；测试契约更正（prompt_prefix_position_test.go 头部注释 + TestDynamicRuntimeCueHook_AppendsCueAtEnd）。前缀区收敛为真正字节稳定 |
| WP-2a | teamCompletionGuard 扩展到 synthesize_results | ✅ | 原只拦 get_team_deliverable；现同前置条件（teams running 时拦截 + 结构化进度提示 "x/y 已完成"），消除 124 次 CONFLICT 失败（每次还省 3 轮 × 60K retry_reflect 重试）。新增 tool_team_completion_guard_test.go 4 例 |
| WP-2b | Declaration 静态前置条件 + 429 增强 | ✅ | synthesize_results 描述补前置条件（spirit_tools.go + builtin_tools_seed.go 同步）；subagents_spawn 描述内联并发上限（进程级固定值，字节稳定）；429 错误带排序后的活跃 run id 列表（保留 deterministic 关键词子串，不误触发重试） |
| WP-2c | retry_reflect 结构化限流归类 deterministic | ✅ | isDeterministicToolError 按 error CODE 识别 apierror.CodeRateLimit（子代理并发上限、配额窗口）——反射重试无法解除并发 cap，原始错误已带可执行指引，不再消耗重试预算；纯字符串 "rate limit exceeded" 第三方错误保持可重试。新增 TestIsDeterministicToolError_StructuredRateLimit（含 wrapped 链） |
| WP-3 | todo_write 59 次空 error 归因 | ✅ 无需修复 | 全部为 2026-06-07~06-15 历史 stuck-timeout 看门狗记录（event_bus/trpc 两路径，error_message 列未填、output_preview 含 stuckTimeout 标记），近 2 个月无新增——路径已自愈 |
| WP-4 | Tool RAG 修订为两段式缓存安全设计 | ✅ 已实现（2026-08-13） | 见 §14.4 |

### 14.3 验收

1. ✅ `go test ./internal/agent/ -run "TestTeamCompletionGuard|TestDynamicRuntimeCueHook|前缀相关 12 例"` 全绿；WP-1/WP-2a 均先红后绿（TDD）。
2. ✅ 最终全量验证（2026-08-13）：`go build ./cmd/... ./internal/... ./api/... ./pkg/...` 通过；`go test ./internal/agent/ ./internal/tools/... ./internal/plugin/trpc/ -count=1` 全部 ok（36 个包，含此前因 mock 缺方法失败的 internal/tools/knowledge——已补 EnableCollectionSemantic）。
3. ✅ 系统性复审（2026-08-13）：前缀区残留扫描确认仅 static runtime cue（会话内字节稳定）与压缩截断标记（仅在截断时触发，历史已被重写、前缀本就失效，不构成每轮击穿）使用 insertAfterLastSystem；memory/knowledge/skill/reply reminder/intent/dynamic runtime cue 全部位于消息末尾尾部区。无新增问题。
4. ⏳ 生产回归口径（下次部署后观测）：spirit 缓存命中率 0.533 → 预期 ≥0.9（0.000 样本消失）；synthesize_results 失败率 71.7% → 预期趋近 0（拦截不计失败）；subagents_spawn 429 失败占比下降。
5. ✅ WP-4 验证（2026-08-13）：`go build ./cmd/... ./internal/... ./api/... ./pkg/...` 通过；`go test ./internal/tools/deferred/... ./internal/tools/ -count=1 -v` 全绿（含新增 27 例：SplitCoreResidentTools 6、ToolLoadTool 7、RenderCatalogCue 7、registry_map 7）；`go test ./internal/agent/... -count=1` 全绿（8 包）；`go vet` 干净。均先红后绿（TDD）。

### 14.4 WP-4 两段式工具加载设计（阶段 1 T-003 修订，✅ 已实现 2026-08-13）

- **核心常驻集**：tools 块只放高频核心工具（会话内字节稳定）。`SplitCoreResidentTools`（internal/tools/deferred/split.go）按 profile（spirit/coding/chat_only + 默认兜底）把有效工具 key 分为核心集与延迟集，两侧均排序保证确定性。
- **静态目录 cue**：延迟工具以「工具名 + 一句话描述」清单注入（`RenderCatalogCue`，internal/tools/deferred/catalog_cue.go），按 category → name 双层排序、无动态状态，经 BeforeModel hook（internal/agent/tool_catalog_cue.go）**append 到消息末尾尾部区**（遵守 P-1 缓存不可侵犯），并计入 `ContextBudgetCategoryToolsSchema` 台账。
- **tool_load 元工具**（internal/tools/deferred/tool_load.go）：模型按需激活延迟工具——校验在 catalog 内 → `manager.Activate` 惰性解析真实工具 → `Discover` 标记，返回成功消息 + 工具描述；重复激活/未知工具/空名均返回结构化失败（不 panic、不污染消息流）。
- **集成路径**：`buildToolsetsForAgent`（tool_assembly.go）——`ToolsDeferredJSON` 手动配置优先；否则自动分离（`SplitCoreResidentTools` + `RegistryNamesForBizKeys` biz key → registry 名映射，registry_map.go）。`AssembleToolsets`（toolset_assemble.go）把延迟工具注册进 catalog 并挂 `tool_search`/`tool_load` 两个元工具；`toolset.go` 从 enabled 集中删除延迟项，防止 builtin/search/claudecode 装配器重复注册。
- **spirit 闲聊（2026-08-23）**：核心集白名单 = `plan_and_execute` / `datetime` / `memory_search` / `memory_remember` / `web_research`。`duckduckgo_search` / `web_fetch` 改 deferred（闲聊只留一条网页路径）。`GovernToolDeclaration` 剥掉常驻工具 `OutputSchema`（OpenAI/DeepSeek 会把输出 JSON 拼进 description）。系统头只挂 `IDENTITY` / `CAPABILITIES` / `DECISION`（不再灌 `company_lead` / `orchestrator`），并去掉 Spirit 的 coding `working_contract`。`web_research` 默认 `FetchTop=2`、每源正文 2.4KB，装饰器再截到 4KB。`MergeNonCoreMappedDeferred` 把已映射且非核心的工具（含侧通道 MemoryTools / working_memory / 收口 CustomTool / M71 会话考古）并进 deferred。目录 cue 压缩为首句。spirit/chat_only 只注册 `skill_load`。`CAPABILITIES.md` 与 Request.Tools 对齐，避免把已 deferred 的工具写成核心。
- Tool RAG 检索结果只影响「目录 cue 中标注哪些为推荐」，不改变 tools 块内容——规避 M1 矛盾。
- 验收基准沿用：tools_schema_tokens -80%、工具选择金标集准确率 ≥90%（待生产回归观测）。

### 14.5 改动文件（本迭代）

- `internal/agent/runtime_cue_inject.go`（WP-1）
- `internal/agent/prompt_prefix_position_test.go`（WP-1 测试契约）
- `internal/agent/tool_team_completion_guard.go`（WP-2a）
- `internal/agent/tool_team_completion_guard_test.go`（WP-2a 新增测试）
- `internal/tools/spirit_tools.go`（WP-2b 描述）
- `internal/data/builtin_tools_seed.go`（WP-2b 种子同步）
- `internal/tools/subagent/service.go`（WP-2b 描述 + 429 增强）
- `internal/plugin/trpc/retry_reflect.go`（WP-2c CodeRateLimit 确定性分类）
- `internal/plugin/trpc/retry_reflect_test.go`（WP-2c 新增测试）
- `internal/tools/knowledge/tool_more_test.go`（mock 补 EnableCollectionSemantic，修复并行会话接口演进导致的构建失败）
- `internal/tools/deferred/split.go`（WP-4 核心/延迟分离）+ `split_test.go`
- `internal/tools/deferred/tool_load.go`（WP-4 tool_load 元工具）+ `tool_load_test.go`
- `internal/tools/deferred/catalog_cue.go`（WP-4 静态目录 cue 渲染）+ `catalog_cue_test.go`
- `internal/tools/deferred/registry_map.go`（WP-4 biz key → registry 名映射）+ `registry_map_test.go`
- `internal/tools/deferred/tool_search.go`（WP-4 补 Catalog() 访问器）
- `internal/agent/tool_catalog_cue.go`（WP-4 BeforeModel hook 注入目录 cue）
- `internal/agent/tool_assembly.go`（WP-4 自动分离集成）
- `internal/agent/callback_chain.go`（WP-4 注册目录 cue hook）
- `internal/tools/toolset_assemble.go`（WP-4 挂 tool_search/tool_load 元工具）
- `internal/tools/toolset.go`（WP-4 enabled 集删除延迟项防重复注册）

## 15. 第二轮全链路深入审查与修复（2026-08-13，N1~N8）

背景：阶段 0（§13）与 WP-1~WP-4（§14）完成后，对「指令输入 → 意图识别 → 工具链加载 → 知识库加载 → 记忆加载 → LLM 响应」整条链路做第二轮深入审查，发现 8 项提升点（N1~N8），经评审后全部修复。

### 15.1 发现与修复状态

| # | 发现 | 修复 | 状态 |
|---|------|------|------|
| N1 | skill guidance 注入位置不当：半静态 skill 指引经 insertAfterLastSystem 钉在前缀区，穿透前缀缓存 | 移到动态层，与 memory/knowledge cue 对齐 append 到消息末尾尾部区（遵守 §14 P-1 缓存不可侵犯） | ✅ |
| N2 | 历史无上限增长：压缩级联（ContextCompaction/MemoryCompact/SessionSummary）默认关闭，存量行保持旧值 | 默认开启（agent_defaults.go + ent schema 默认值）+ 数据迁移翻转存量行（compression_default_on_migrate.go） | ✅ |
| N3 | context_budget 台账 history 盲区：非 system 消息 token 不计入台账，无法观测历史膨胀 | 新增 `ContextBudgetCategoryHistory` + `newContextBudgetHistoryBeforeHook`（每请求去重），注册进 callback_chain；service 台账日志补 history_tokens | ✅ |
| N4 | full-profile 路径 BatchGetSkillGuidance 每轮查库：工具循环内同一 invocation 重复 DB 调用放大 | per-invocation memoize（`skillGuidanceCueMemoStateKey`），工具循环内仅首次查库 | ✅ |
| N5 | 意图产物消费率低：分解 prompt 未携带 SuccessCriteria/SearchHints，子任务契约与成功标准脱节 | task_planner_impl.go 分解 prompt 补 SuccessCriteria/SearchHints | ✅ |
| N6 | DeferredTools 机制空转风险：无法回答「哪些工具 schema 最大」——聚合 tools_schema_tokens 看不出单个工具占比 | 台账增加 per-tool top-5 schema 观测（ContextBudgetToolSize + SetTopTools，按 chars 降序取前 5）；service 台账日志输出 `top_tool_schemas`（name + est_tokens）。**范围调整**：先做观测定位真大工具，再决定是否配置 deferred loading | ✅ |
| N7 | 压缩与缓存告警交互误报：压缩轮次把历史重写为新前缀击穿缓存，其巨大 token 量主导加权 ratio（sum(cached)/sum(prompt)），每次压缩都误报 | 告警改 key P50 单轮命中率中位数（对压缩离群鲁棒）；SQL 聚合粒度从 agent_key 调整为 (provider, model)，P50 由 `percentile_cont` 在正确粒度直接计算；加权 ratio 保留仅作诊断。详见 29-token.design.md §9.3/§9.4 | ✅ |
| N8 | RebuildMemoryInjectForCompaction 死代码：其设计的「原地打补丁」场景不成立——MemoryInject 是 BeforeModel hook（priority 5），在框架压缩与 Aranea 紧急压缩（priority 3）之后才执行，压缩轮次会用最新数据重建完整 cue；memory-inject 消息是请求级装饰不持久化，框架重建的 request 不残留旧 cue；若接入框架 tail-processor 槽位会与本 hook 双重注入 | 删除函数及测试（memory_inject.go 留 NOTE 说明为何无需该入口） | ✅ |

### 15.2 验收

1. ✅ TDD：N3/N6 新增观测均先写失败测试（context_budget_test.go：history 记录、top-5 排序与截断、bare ctx no-op）再实现；N8 删除后清理孤儿测试。
2. ✅ 全量验证（2026-08-13）：`go build ./cmd/... ./internal/... ./api/... ./pkg/...` 通过；`go test ./internal/agent/ -count=1`（28.1s）、`go test ./internal/biz/monitor/ ./internal/biz/usage/ -count=1`、`go test ./internal/service/ -run TestChatTurnMetrics -count=1 -v`（5 例）、`go test ./internal/data/ -run CacheHit -count=1 -v`（真 PG，2 例）全部 PASS；`go vet` 相关 5 包干净。
3. ⏳ 生产回归口径（下次部署后观测）：台账 `top_tool_schemas` 输出可用于定位 DeferredTools 配置目标；N7 告警在压缩开启后不再误报（压缩日加权 ratio 暴跌但 P50 稳定）。

### 15.3 改动文件（本迭代）

- `internal/agent/context_budget.go`（N3 history 类目 + N6 top-tools 观测）
- `internal/agent/context_budget_test.go`（N3/N6 测试）
- `internal/agent/callback_chain.go`（N3 注册 history hook）
- `internal/agent/skill_guidance_inject.go`（N1 注入位置 + N4 memoize）
- `internal/agent/memory_inject.go`（N8 删除死代码，留 NOTE）
- `internal/agent/prompt_prefix_position_test.go`（N8 清理孤儿测试）
- `internal/agent/task_planner_impl.go`（N5 分解 prompt 补 SuccessCriteria/SearchHints）
- `internal/biz/agent_defaults.go`（N2 压缩级联默认开）
- `internal/data/ent/schema/agent_runtime_setting.go`（N2 schema 默认值）
- `internal/data/compression_default_on_migrate.go`（N2 存量行迁移）
- `internal/biz/usage/cache_hit.go`（N7 CacheHitRatioStat 去 AgentKey + P50Ratio）
- `internal/data/usage_cache_hit.go`（N7 SQL 聚合粒度调整）
- `internal/biz/monitor/alert_metric_cache_hit.go`（N7 告警改 key P50Ratio）
- `internal/service/chat_turn_metrics.go`（N3 history_tokens + N6 top_tool_schemas 日志字段）
- `docs/development/29-token.design.md`（N7 §9.3/§9.4 修订说明）

---

## 16. Deferred 工具加载深度修复（2026-08-13 第二轮，P0/P1/P2 共 8 项）

> 背景：§14 WP-4 实现了两段式工具加载框架（split + catalog cue + tool_load/tool_search + assemble 集成），但第二轮深度审查发现 8 项关键缺陷会导致功能空转或不完整。

### 16.1 发现与修复

| # | 发现 | 修复 | 状态 |
|---|------|------|------|
| P0-A | tool_load 返回空 schema：Activate 只标记不返回 InputSchema，模型拿到激活成功但无法构造参数 | Activate 返回完整 Declaration（含 InputSchema），tool_load 将其透传给模型；同时调用 toolsnapshot.InvalidateFromContext 强制刷新 tools 块 | ✅ |
| P0-B | DeferredCallableTool 是 factory 模式：Declaration 无 InputSchema，Call 内部二次初始化 factory（延迟创建 + 装饰器丢失） | 改 eager-inner 模式：包装已完全装配的工具（含完整 schema + 超时/结果预算/缓存装饰器），Declaration() 始终返回内部工具完整声明 | ✅ |
| P0-C | DeferredToolManager 用内存 map 存激活状态：进程内全局共享，跨 session 泄漏 | 激活状态存入 session state（`temp:deferred:activated`，JSON []string），per-session 隔离；readActivatedSet/writeActivatedSet 通过 InvocationFromContext 访问 | ✅ |
| P1-A | tool_search 自动激活工具（Discover）：模型搜到即调，绕过 tool_load 显式激活流程 | 移除 Discover 调用，tool_search 纯检索不激活；模型须显式 tool_load 后才可调用 | ✅ |
| P1-B | tool_assembly.go 手动配置粒度错误：ToolsDeferredJSON 传 registry 名但 SplitCoreResidentTools 期望 biz key | 手动配置路径增加 RegistryNamesForBizKeys 转换；自动分离路径保持 biz key 粒度 | ✅ |
| P2-A | aliasTool/confirmationTool 无 InnerTool：filter 无法穿透包装层检查延迟工具激活状态 | aliasTool、confirmationTool、confirmationCallable 均实现 InnerTool()；ToolFilter 递归解包（InnerTool → Original，8 层防循环） | ✅ |
| P2-B | ToolSet 前缀工具 catalog 名与 LLM 调用名不匹配：catalog 用基础名（read_file）但 LLM 看到运行时名（file_read_file） | catalog 存运行时名（Name）+ 基础名（BaseName）；Activate 同时激活两个名；tool_load 返回 schema 时 Name 覆盖为运行时名 | ✅ |
| P2-C | assemble 路径 ToolSet 扫描遗漏：只匹配延迟注册表名，未处理 ToolSet 内工具逐个注册 | assembleDeferredTools 扫描所有 ToolSet，名称匹配延迟注册表名时其所有工具均注册为延迟工具（运行时名 = ToolSet名_工具名） | ✅ |

### 16.2 验收

1. ✅ `go build ./internal/tools/... ./internal/agent/...` 通过
2. ✅ `go test ./internal/tools/deferred/... -count=1 -v` 52 例全绿（catalog_cue 7 + deferred_tool 6 + tool_search 4 + tool_load 7 + integration 7 + registry_map 7 + split 6 + tool_search_test 4 + tool_load_test 4）
3. ⏳ 生产回归：验证两段式加载在真实 Agent 对话中生效（catalog cue 注入 + tool_load 激活 + 工具调用成功）

### 16.3 改动文件（本迭代）

- `internal/tools/deferred/activation.go`（P0-C 新增：per-session 激活状态管理）
- `internal/tools/deferred/deferred_tool.go`（P0-B 重构：eager-inner 模式 + InnerTool）
- `internal/tools/deferred/tool_load.go`（P0-A 返回完整 schema + 快照失效 + 运行时名/基础名双激活）
- `internal/tools/deferred/tool_search.go`（P1-A 移除 Discover + P2-A 递归解包 filter）
- `internal/tools/deferred/toolset.go`（P0-B 配套：DeferredToolSet 适配 eager-inner）
- `internal/tools/deferred/deferred_tool_test.go`（P0-B 重写测试）
- `internal/tools/deferred/integration_test.go`（P0-C/P2-A/P2-B 重写测试）
- `internal/tools/deferred/tool_load_test.go`（P0-A/P2-B 重写测试）
- `internal/tools/deferred/tool_search_test.go`（P1-A 移除 Factory 字段）
- `internal/tools/toolset_assemble.go`（P2-C ToolSet 扫描 + 运行时名）
- `internal/tools/runtime_alias.go`（P2-A InnerTool）
- `internal/tools/trpc/confirmation.go`（P2-A InnerTool）
- `internal/agent/tool_assembly.go`（P1-B 手动配置粒度修正）

## 17. 工具调用质量度量与上下文预算可视化（2026-08-14，P1 共 5 项）

> 背景：对照 trae/cursor/codex 等工具「一次成功、低 token」的表现，立项分析本项目工具调用的 token 消耗与成功率。核心发现：参数修复护栏（tool_args_repair_guard）静默修复 JSON 但不留痕，无法回答「模型一次合法率」；context_budget 台账只落日志不进指标系统，无法聚合观测。本迭代落地质量度量数据通路；消费侧（API/UI 面板）留给下一迭代。

### 17.1 发现与修复

| # | 发现 | 修复 | 状态 |
|---|------|------|------|
| Q1 | 参数修复护栏静默工作：repair guard 修复/拒绝 JSON 后无任何记录，工具成功率无法分解为「模型一次合法」vs「护栏补救」 | repair guard BeforeTool hook 经 ctx 写入质量标记（`toolArgsQuality{Repaired,Invalid}`）；invocation recorder 消费并落入 `ToolInvocationWrite.ArgsRepaired/ArgsInvalid` + `aranea_tool_args_guard_total{tool,outcome}` counter | ✅ |
| Q2 | 质量标记无持久化：tool_invocations 行不含修复标记，事后无法聚合 | `invocationMetaJSON` 将 `args_repaired/args_invalid` 写入 metadata_json（仅 true 时写入，空信号保持 `{}` 不污染） | ✅ |
| Q3 | 无工具质量聚合查询：回答不了「哪个工具一次合法率最低」 | 新增 `ToolQualityStatsReader` 窄接口（biz/tool）+ `toolQualityStatsRepo`（Raw SQL 聚合，`Dialect.JSONExtract` 双方言提取 metadata_json 标记），输出 ToolQualityStat（总数/成功/失败/修复数/一次合法率/平均耗时） | ✅ |
| Q4 | context_budget 台账只落日志：无法按 category 聚合观测分布 | 新增 `aranea_context_budget_tokens{category}` Histogram，chat_turn_metrics 台账出口同步 Observe | ✅ |
| Q5 | preview 截断按字节切 CJK 多字节字符：产生非法 UTF-8，Postgres 22021 拒写整行 | 新增 `truncateUTF8`（rune 边界安全截断），input/output preview 截断统一改走它 | ✅ |

### 17.2 验收

1. ✅ data 层（隔离缓存全量重建 `go test -a`，真实 PG）：`TestToolQualityStatsRepo_AggregatesArgsQuality`（聚合/一次合法率/agent 过滤）、`TestInvocationMetaJSON_ArgsQualityFlags`、`TestTruncateUTF8`（8 子用例）全绿
2. ✅ agent 层（2026-08-14 02:03 稳定窗口）：19 例全绿——`TestToolArgsRepair_MarksRepairedInContext`/`MarksInvalidWhenUnrepairable`/`ValidArgsNoMarkers`（ctx 标记）、`TestRecordToolInvocation_ArgsGuardOutcomeMetrics`（repaired/invalid 两 counter 子用例），14 例存量 repair 测试无回归
3. ✅ service 层：`TestRecordContextBudgetLog_ObservesCategoryHistogram`（Histogram 观测）+ `NoBudgetNoObservation`（空预算不观测）全绿
4. ✅ 全量构建 `go build ./cmd/... ./internal/... ./api/... ./pkg/...` exit 0；`go vet`（agent/service/data/biz/tool/metrics）exit 0；araneactl lint 本迭代文件 0 违规（存量 3 处违规属其他模块：twin_openapi_compat R7、knowledge workbench 2×R-FE1）
5. ⏳ 消费侧接线：`NewToolQualityStatsRepo` 尚未 Wire 绑定 / 无 RPC 暴露（P2 迭代：质量面板 API + UI）

### 17.3 改动文件（本迭代）

- `internal/agent/tool_args_repair_guard.go`（Q1 ctx 质量标记）
- `internal/agent/tool_invocation_recorder.go`（Q1 消费标记 + counter）
- `internal/biz/tool/tool.go`（Q3 ToolInvocationWrite 字段 + ToolQualityStat + ToolQualityStatsReader）
- `internal/biz/tool_reexport.go`（Q3 再导出）
- `internal/data/tool.go`（Q2 metadata 映射 + Q5 truncateUTF8）
- `internal/data/tool_quality_stats_repo.go`（Q3 新增聚合查询）
- `internal/metrics/vars.go`（Q1 ToolArgsGuardTotal + Q4 ContextBudgetTokens）
- `internal/service/chat_turn_metrics.go`（Q4 Histogram 观测）
- 测试：`tool_args_repair_guard_test.go`、`tool_invocation_recorder_test.go`（agent）；`tool_quality_stats_test.go`、`tool_test.go`（data）；`chat_turn_metrics_budget_test.go`（service）

### 17.4 并行会话干扰记录（2026-08-14）

本迭代验证期间另一会话正在跨包重构（agent_usecase 拆分 / data agent_repo_convert 抽取 / service agent_proto 抽取 / ent 重新生成），工作区长时间处于中间态：

- 共享 GOCACHE 混入新旧 ent 对象，导致 `toolinvocation.IDValidator` nil panic 幻影——`go test -a` 强制全量重建后同源码全绿，证实非真实 bug
- 并行提交遗留 3 处编译破损（其验证只做 `go build`，不覆盖 _test.go）：`tool_result_cache_test.go` 缺/多 `biz` import、`skill_catalog_test.go` map 值类型与匿名 struct 声明不符、`biz/evaluation/evaluation.go` 缺 `time` import——对方重构落定后由本会话顺手修复（各 1-2 行）
- 教训复核：并行 churn 期间一切编译结论以隔离缓存 + 全量重建为准（`go test -a` 或独立 GOCACHE 子目录）；后台轮询脚本须 `Start-Process` 脱离 AI 终端，否则随终端回收被杀（0xC000013A）

## 18. 提示词注入链路全链路优化（2026-08-14，P0 止血 / P1 命中率与 token / P2 度量闭环）

> 背景：立项分析「指令输入 → 工具链加载 → 知识库加载 → 记忆加载 → LLM 响应」全链路的业务逻辑、命中率与 token 消耗，并对照业界方案（Hermes 式分层上下文、工具检索/RAG-MCP 等前沿做法）形成方案。用户确认全批实施（P0+P1+P2）。核心结论：注入链多点各自为政（截断口径不一、无统一终审闸门）、session summary 污染 system[0] 破坏前缀缓存、MCP schema 无总量预算、deferred 工具纯静态目录发现率不可观测、召回 query 带寒暄噪声。

### 18.1 发现与修复

| # | 发现 | 修复 | 状态 |
|---|------|------|------|
| P0-A+D | 压缩闸门在注入前估算、按 rune 截断、marker 落点错位：cue 注入后真实超限无人兜底，rune 口径与 token 口径偏差大 | 终审 hook 统一压缩闸门（internal/agent/context_compression_inject.go）：注入后计数、token 口径截断（target factor 0.9）+ 截后复验、marker 落点修正（snapToSafeBoundary + insertMarkerAfterHead） | ✅ |
| P0-B | session summary 注入 system[0]：每轮变化的摘要污染可缓存前缀，前缀缓存全失效 | summary 改以 user 消息注入尾部 append 区（trpc_build.go），system[0] 保持会话内字节稳定 | ✅ |
| P0-C | 请求级硬上限缺失：极端溢出时无降级路径，直接超窗报错 | 降级链：超限先丢尾部动态 cue（catalog/memory/knowledge 可重建），再截历史，最后复验；每步落流程/进程日志 | ✅ |
| P1-1 | 召回 query 原样进检索：寒暄前缀/多问题标点污染关键词，命中率低 | recall_keyword.go：cleanRecallQuery 去寒暄/ filler 前缀；多查询扩展（问号分段、question-last 打包，≤4 段）；120 rune 预算截尾；lastUserMessageText 不再预截断（预算归 cleanRecallQuery 所有） | ✅ |
| P1-2 | MCP 工具 schema 无治理：单工具 declaration 可数千 token，多 server 叠加无总量预算，直连注入失控 | mcp_schema_govern.go：单 declaration 软上限 2400 chars（截 description/剥 OutputSchema/schema 节点 description+enum 截断）；总量硬预算 16000 chars（≈4.6K token）；超预算自动降级 broker 模式（mcp_list_servers/mcp_list_tools/mcp_inspect_tools/mcp_call），无显式 broker 配置时生成 fallback；ToolSet 包装治理（governMCPToolIfNeeded）；toolset_assemble 集成 + brokerAdded 去重 | ✅ |
| P1-3 | 截断口径各自为政：多处本地副本、byte/rune 混用，工具结果 byte 切 CJK 产出 U+FFFD 污染模型输入 | pkg/strutil 单一阈值链：TruncateRunesEllipsis（prompt 注入链字段/块级统一入口）+ SliceBytesRuneSafe（tail/head/middle 三模式 rune 边界安全）； decorator.go sliceForMode、case_prompt、l4_prompt、plugin/trpc、knowledge、data 各截断点全部改走 strutil，本地副本删除 | ✅ |
| P1-4 | deferred 工具发现率不可观测：静态目录 cue 全量平铺，模型漏看无兜底；「搜索→激活→使用」漏斗无度量 | 语义预激活：catalog_rank.go 按当前用户 query 对 catalog 排序，Top-3 以「推荐区」提升进 cue（推荐区在目录分组前；无匹配时与静态版字节一致；cue 在消息尾部、前缀缓存之外，动态渲染零缓存成本）；tool_search 改共享同一打分器；≥3 runes 子串护栏防短虚词噪声（"me" 误中 "runtime"）。漏斗度量：`aranea_deferred_tool_search_total{has_results}`（发现）+ `aranea_deferred_tool_activation_total{tool,outcome}`（激活）+ `aranea_deferred_catalog_recommend_total{matched}`（预激活覆盖率），使用段复用 `aranea_tool_invocation_total{tool,status}` | ✅ |
| P2-1 | context_budget 台账无法跨 turn 聚合观测 | 聚合只读 API：`GetContextBudgetStats`（GET /v1/usage/context-budget-stats）四视图（overall/agents/trends/top_tools）；data 层 PG JSONB 两查询取 (agent,day) 最细粒度 grains，biz 层 rollup 三视图；窄接口 `ContextBudgetStatsRepo` | ✅ |
| P2-2 | 知识/记忆 cue 的 cited 引用无回采追踪，命中率闭环缺最后一公里 | cited 回采 + 引用追踪：知识侧 `knowledge_search`/`knowledge_reflect` 每次检索发射 `knowledge_recalled` notice（chunk 集合）→ `KnowledgeCitationBackfillWorker`（10m 轮询/1h 窗口）join 终态回复做 ID 引用 + 8-rune k-gram 启发式判定 → `(chunk,turn)` 去重账本首次命中 `cited_count`+1；记忆侧已于 70 模块完工 | ✅ |

### 18.2 验收

1. ✅ P0-A+D：`context_compression_inject_test.go` 终审闸门用例（注入后计数/token 截断/复验/marker 落点/降级链丢尾部 cue）全绿，先红后绿
2. ✅ P0-B：`trpc_build_runtime_options_test.go` 断言 summary 以 user 模式注入尾部 append 区
3. ✅ P1-1：`recall_keyword_test.go` 寒暄清洗/多问题分段保留/120 rune 预算用例全绿
4. ✅ P1-2：`mcp_schema_govern_test.go` 软上限截断/嵌套 schema 治理/总量预算降级/Kept 包装/broker 去重用例全绿
5. ✅ P1-3：`pkg/strutil` 单测（rune 省略号/三模式 rune 边界切片/CJK 不碎）+ 各迁移点包测试全绿；knowledge 桩补齐 `EnableCollectionSemantic` 后 data/knowledge 构建恢复
6. ✅ P1-4（TDD 先红后绿）：`catalog_rank_test.go` 9 例（排序/子词匹配/短词护栏/limit/确定性/推荐区渲染/无推荐=静态字节一致）+ `funnel_metrics_test.go` 2 例（search/activation counter delta）+ `tool_catalog_cue_test.go` 4 例（推荐注入/无匹配静态/nil 安全）全绿；deferred 存量 52 例无回归
7. ✅ 全量构建 `go build ./cmd/... ./internal/... ./api/... ./pkg/...` exit 0；`go vet`（deferred/agent/metrics）exit 0；agent 8 包、tools 包全量测试 exit 0
8. ✅ P2-1：`context_budget_test.go`（rollup/累加器）+ `usage_context_budget_mapper_test.go` 全绿；`make api` 重新生成 pb；全量构建 exit 0
9. ✅ P2-2（知识侧 cited 回采）：`knowledge_citation_test.go` 3 例 PG 集成（去重账本幂等/候选 join/缺失 chunk 剔除）+ `knowledge_citation_backfill_test.go` 8 例（启发式/worker 容错/nil 安全）+ `tool_notice_test.go` 5 例（payload 形状/无 emitter no-op/上限截断）全绿；`make wire` 重生成；全量构建 + vet exit 0

### 18.3 改动文件（本迭代）

- `internal/agent/context_compression_inject.go`（P0-A/C/D 终审闸门+降级链）+ `context_compression_inject_test.go`
- `internal/agent/trpc_build.go`（P0-B summary 尾部注入）+ `trpc_build_runtime_options_test.go`
- `internal/agent/recall_keyword.go`（P1-1 清洗+多查询扩展）+ `recall_keyword_test.go`、`internal/agent/memory_inject.go`（lastUserMessageText 让位）
- `internal/tools/mcp_schema_govern.go`（P1-2 治理核心）+ `mcp_schema_govern_test.go`、`internal/tools/toolset_assemble.go`（集成+去重）
- `pkg/strutil/strutil.go`（P1-3 单阈值链）+ 迁移点：`internal/tools/decorator.go`、`internal/agent/case_prompt.go`、`internal/agent/l4_prompt.go` 等
- `internal/tools/deferred/catalog_rank.go`（P1-4 排序+推荐渲染）+ `catalog_rank_test.go`、`funnel_metrics_test.go`
- `internal/tools/deferred/tool_search.go`（共享打分器+发现段 counter）、`tool_load.go`（激活段 counter）
- `internal/agent/tool_catalog_cue.go`（动态推荐渲染+覆盖率 counter）+ `tool_catalog_cue_test.go`
- `internal/metrics/vars.go`（DeferredToolSearchTotal / DeferredToolActivationTotal / DeferredCatalogRecommendTotal）
- P2-1：`api/kratos/usage/v1/usage.proto`（GetContextBudgetStats + 4 message）、`internal/biz/usage/context_budget.go`（窄接口+rollup）+ `context_budget_test.go`、`internal/data/usage_context_budget.go`（PG JSONB 两查询）、`internal/service/usage.go` + `usage_mapper.go` + `usage_context_budget_mapper_test.go`、`web/src/services/kratos/usage/v1/index.ts`
- P2-2：`internal/biz/knowledge/citation.go`（ChunkCitation/两端口/KnowledgeRecalledNoticeType）、`internal/data/sql/migrations/20261215_knowledge_citation_counters.sql`（cited_count + knowledge_chunk_citations 账本）+ 注册表 + `knowledge.go` EnsureKnowledgeSchema fresh 形态、`internal/data/knowledge_citation.go`（trace reader+recorder）+ `knowledge_citation_test.go`、`internal/cronrunner/jobs/knowledge_citation_backfill.go`（worker，chunkCited 委托 factCited）+ 测试、`internal/tools/knowledge/tool.go`（search/reflect 两处发射）+ `tool_notice_test.go`、`internal/biz/session/activity_message_adapter.go` + `web/src/features/chat/noticeFilter.ts`（notice 过滤双侧）、`cmd/admin/wire.go`/`wire_gen.go`/`workers.go`（装配+启动）

### 18.4 P1-4 已知限制与后续方向

- 预激活排序为纯关键词/子串匹配：纯中文 query 对英文工具名/描述无效。后续可复用知识库 embedding 基础设施做语义召回（需评估 per-turn 同步 embedding 的延迟成本，~50-200ms）
- 「激活未使用」的 per-session 观测需 PromQL 差集近似；精确闭环依赖 P2-2 引用追踪
- 并行会话干扰：本迭代期间另一会话持续重构 service/agent 包（git status 可见大量非本任务 WIP），`llm_caller_impl.go` 曾出现瞬时 import 破损（重读已落定）；门禁结论以退出码 + 落盘日志为准（终端回显被并行输出污染）
