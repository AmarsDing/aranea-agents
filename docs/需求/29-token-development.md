# Token 用量 — 开发计划

> **版本**：2026-05-20 | **状态**：✅ 基础 + 配额/明细/Team 写入/告警/小时聚合已实现
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
| 明细表 `model_token_usage_events` | ✅ | `docs/sql/08_usage.sql` · `internal/data/sessionmemory/memory_chain.sql` |
| 日聚合表 `model_token_usage_daily` | ✅ | 写入时自动 upsert |
| 价格规则表 `model_pricing_rules` | ✅ | `internal/data/ent/schema/model_pricing_rule.go` |
| 用量记录写入 | ✅ | 主路径 `recordTurnUsage`；Team `usage_kind=team_member`（`internal/team/usage_record.go`） |
| Session 聚合更新 | ✅ | `internal/data/usage_write.go` 事务内 UPDATE sessions |
| 日聚合 upsert | ✅ | `internal/data/usage_write.go` → `upsertModelTokenUsageDaily` |
| 用量概览 API | ✅ | `GetUsageOverview` + `quota_dashboard`（活跃 Agent 配额汇总） |
| 趋势查询 API | ✅ | `ListUsageTrends`；`granularity=hour` → `model_token_usage_hourly` |
| Top 模型排行 API | ✅ | `UsageService.ListTopModels` |
| Top Agent 排行 API | ✅ | `UsageService.ListTopAgents` |
| 明细列表 API | ✅ | `UsageService.ListUsageEvents` |
| 用量事件写入 API | ✅ | `UsageService.RecordTokenUsageEvent` |
| 查询筛选（Provider/Model/Agent/Status/时间） | ✅ | `UsageQuery` + `usageWhere()` |
| 异常请求筛选 | ✅ | `status = "abnormal"` → `status <> 'success'` |
| Wire 注入 | ✅ | `NewUsageRepo` / `NewUsageUsecase` / `NewUsageService` |
| 用量限额（quota） | ✅ | `usage_quotas` + `CheckQuota`（单 Agent + Team 成员）；`internal/data/usage_quota.go` |
| 写入时费用计算 | ✅ | `RecordTokenUsageEvent` → `GetActiveModelPricing` + `ApplyTokenUsageCosts` |
| 用量告警（budget alert） | ✅ | `budget_alerts` + `EvaluateBudgetAlerts` → `usage.budget_alert` 监控事件 |
| 价格回退 | ✅ | `model_pricing_rules` 优先，否则 `llm_provider_models.config_json` 单价 |
| 价格自动同步 | 部分 | Provider 模型 inspect/保存 → `syncProviderModelPricing`；无独立定价 UI |
| 小时聚合表 | ✅ | `model_token_usage_hourly` + 写入 upsert |
| CSV 导出 | ✅ | `GET /v1/usage/events/export` |

### 2.2 前端

| 项 | 状态 | 证据 |
|----|------|------|
| 类型定义 | ✅ | `web/src/features/usage/types.ts` |
| API 调用层 | ✅ | `web/src/features/usage/api.ts`（含 snake_case ↔ camelCase 转换） |
| 核心指标卡片 | ✅ | `web/src/components/usage/UsageMetricCards.vue` |
| 趋势图 | ✅ | `web/src/components/usage/UsageTrendPanel.vue` |
| Top 模型排行 | ✅ | `web/src/components/usage/UsageTopModels.vue` |
| Top Agent 排行 | ✅ | `web/src/components/usage/UsageTopAgents.vue` |
| 异常请求列表 | ✅ | `web/src/components/usage/UsageAnomalyList.vue` |
| 明细列表页 | ✅ | `UsageEventsPage.vue`（费用/来源/错误列） |
| 限额配置 UI | ✅ | `AgentUsageQuotaPanel`（权限 Tab）；Agent Tab 预算字段已弃用展示 |
| 告警配置 UI | ✅ | `AgentUsageQuotaPanel` 预算告警阈值 |
| CSV 导出 | ✅ | `UsageEventsPage` 导出按钮 |
| 月预算使用率卡片 | ✅ | `UsageMetricCards`（`quota_dashboard`） |
| 小时趋势 | ✅ | 概览「趋势粒度」→ `granularity=hour` |

### 2.3 差距总结（2026-05-20 复审后）

| 优先级 | 差距 | 影响 |
|--------|------|------|
| P2 | 仅 `scope_type=agent` 配额；user/global 未实现 | 多租户预算粒度不足 |
| P2 | user/global 配额 scope | 多租户预算粒度不足 |
| P3 | 低性价比模型识别（#24） | 运营分析增强 |
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
| 1 | `usage_quotas` 表 + SQL 迁移 | Data | 新增 `usage_quotas` 表 |
| 2 | `UsageQuota` 领域模型 + Repo 接口 | Biz | `UsageQuotaRepo` 接口定义 |
| 3 | `UsageQuotaRepo` 实现 | Data | SQLite CRUD |
| 4 | Proto 新增 `GetUsageQuota` / `SetUsageQuota` / `CheckUsageQuota` | Proto | `usage/v1/usage.proto` |
| 5 | `UsageService` 新增 quota RPC | Service | proto ↔ biz 映射 |
| 6 | Chat turn 前检查 quota | Service | `ChatService.RunNativeTurnUnary` 入口拦截 |
| 7 | Wire 注入 | Wire | `NewUsageQuotaRepo` / 扩展 ProviderSet |
| 8 | 单元测试 | Test | quota 检查逻辑 |

### Phase 2：前端补全（P2）

| # | 任务 | 涉及层 | 说明 |
|---|------|--------|------|
| 9 | 独立明细列表页组件 | Web | `UsageEventsPage.vue`，支持筛选/分页/排序 |
| 10 | 限额配置 API + 组件 | Web | `quotaApi.ts` + `UsageQuotaEditor.vue` |
| 11 | 概览页月预算使用率卡片 | Web | 当 quota 存在时展示使用率 |

### Phase 3：用量告警 + 价格同步（P3）

| # | 任务 | 涉及层 | 说明 |
|---|------|--------|------|
| 12 | `budget_alerts` 表 + SQL 迁移 | Data | 新增 `budget_alerts` 表 |
| 13 | `BudgetAlert` 领域模型 + Repo 接口 | Biz | `BudgetAlertRepo` 接口定义 |
| 14 | `BudgetAlertRepo` 实现 | Data | SQLite CRUD |
| 15 | Proto 新增 `ListBudgetAlerts` / `SetBudgetAlert` | Proto | `usage/v1/usage.proto` |
| 16 | `UsageService` 新增 alert RPC | Service | proto ↔ biz 映射 |
| 17 | 告警触发逻辑 | Biz | `RecordTokenUsageEvent` 后检查阈值 |
| 18 | 告警通知 | Service | 系统通知（EventBus → 前端 WebSocket） |
| 19 | 告警配置 API + 组件 | Web | `budgetAlertApi.ts` + `BudgetAlertEditor.vue` |
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
- [ ] 用户 / 全局 scope（`CheckQuota` 仅实现 `agent`）
- [x] 超过预算后 Agent / Team 对话被拦截（`USAGE_QUOTA`）
- [x] 周期由 `period_start` / `period_end` 界定（保存时默认当月）
- [ ] quota 延迟基准测试（预期单次 SUM < 50ms）

### Phase 2

- [x] 独立明细列表页（筛选；表格分页）
- [x] 限额配置 UI（权限 Tab `AgentUsageQuotaPanel`）
- [x] 概览页展示月预算使用率（#11）

### Phase 3

- [x] 达到告警阈值时写入监控（`usage.budget_alert`）
- [x] 告警阈值可配置（Agent 权限 Tab）
- [ ] 价格规则可从 Provider API 自动同步

### Phase 4

- [x] 小时级趋势查询可用（`granularity=hour`）
- [x] CSV 导出功能可用
- [ ] 低性价比模型可被识别和标记

---

## 6. 依赖与风险

| 风险 | 影响 | 缓解 |
|------|------|------|
| Quota 检查增加 Chat turn 延迟 | 用户体验 | 1 次 DB 查询，预期 < 10ms；可加内存缓存 |
| Quota 重置周期边界 | 月末/月初可能重复计算 | 使用 `period_start` / `period_end` 明确界定 |
| 价格自动同步 API 变更 | 同步失败 | 保留 manual fallback；同步失败不阻塞 |
| 告警通知频率过高 | 通知疲劳 | 去重 + 频率限制 |

---

## 7. 迭代：用量三页与配额链路（2026-05-20）

**目标**：统一概览（聚合）、用量事件（明细）、监控（系统可观测）职责；使 `usage_quotas` 与 `total_cost_micro_usd` 在 Chat/Team Turn 前真实生效。

**架构**：写入时 `RecordTokenUsageEvent` 归一化 `status`、按 `model_pricing_rules` 补单价并计算 micro-USD；查询侧 `usageWhere` 状态别名；Team 在 `RunTurn` 前对启用成员 `CheckQuota`；配额配置仅 **Agent 设置 → 权限 Tab**（`usage_quotas`），弃用 Agent Tab `budget_monthly_cents` 展示。

### 7.1 已完成

| 域 | 项 | 证据 |
|----|-----|------|
| Biz/Data | `usage_cost.go`、`GetActiveModelPricing`、`enrichTokenUsagePricing` | `internal/biz/usage.go` |
| Service | `checkTeamMemberQuotas` | `internal/service/chat_quota.go`、`chat_native.go` |
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
| Team | 成员 step 写入 `usage_kind=team_member` | `internal/team/usage_record.go`、`persistStep` |
| Data | `budget_alerts`、`model_token_usage_hourly` | `ent/schema/*`、`08_usage.sql` |
| API | `quota_dashboard`、`ListBudgetAlerts`、`ExportUsageEvents`、`granularity` | `usage.proto` |
| 定价 | Provider `config_json` 回退 | `usage_pricing.go` |
| Web | 配额卡片、小时趋势、CSV、告警 UI | `UsageMetricCards`、`OverviewPage`、`UsageEventsPage`、`AgentUsageQuotaPanel` |

### 7.4 待办（P3）

- [ ] Provider 模型页独立定价编辑 UI
- [ ] 低性价比模型识别（任务 #24）

---

## 8. 代码审计与架构复审（2026-05-20）

### 8.1 架构符合度

| 层级 | 评价 | 说明 |
|------|------|------|
| Proto / Service | ✅ | `UsageService` 薄映射；Chat 配额在 `service/chat_native` + `chat_quota`，不污染 `biz` 对框架的边界 |
| Biz | ✅ | `UsageUsecase` + `UsageRepo`；费用纯函数在 `usage_cost.go` |
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
| Team 用量明细 | `team_member` 在 `persistStep` 写入；并行 fan-out 子成员若 tokens=0 仍无行 |

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

### 8.6 待优化清单（P2/P3）— 2026-05-20 已落地

| # | 项 | 状态 | 落地说明 |
|---|-----|------|----------|
| O1 | `budget_alerts` / `hourly` Ent Schema | ✅ | `ent/schema/budget_alert.go`、`model_token_usage_hourly.go`、`usage_quota.go`；移除 `EnsureUsageExtraSchema` / `usage_schema_extra.go`；`make generate` |
| O2 | `data.SetBudgetAlert` 错误类型 | ✅ | `biz.ErrBudgetAlertNotFound`、`ErrUsageScopeRequired`；`mapUsageRepoErr` |
| O3 | `EvaluateBudgetAlerts` 热路径成本 | ✅ | `scheduleBudgetAlerts` → `safego.Go`；`TotalCostMicroUSD<=0` 跳过 |
| O4 | Team 并行成员 tokens=0 | ⚠️ 部分 | 新增 `usage_kind=team_turn` 聚合写入；`team_member` 仍依赖 step 有 token；并行 fan-out 无 token 子成员仍无行 |
| O5 | user/global `usage_quotas` scope | ✅ | `SumScopeCostInPeriod` + `quotaSpent`；Chat Turn 前 `enforceChatTurnQuotas`（agent/user/`global` scope_id） |
| O6 | 低性价比模型识别（#24） | ✅ | `InefficientModels` + `UsageOverview.inefficient_models`；`UsageInefficientModels.vue` |
| O7 | Provider 独立定价 UI | 暂缓 | `/models`（`ResourceManagerPage`）已可维护单价；独立定价页非本期 |
| O8 | `quota` 延迟基准测试 | ✅ | `internal/biz/usage_quota_bench_test.go`；`go test -bench=BenchmarkCheckQuota` |

**生成物纪律**：Proto/API/Wire 变更后执行 `make all`（或 `make api` + `make wire`），**禁止**手改 `wire_gen.go`、`api/**/*.pb.go`、`web/src/services/**` 生成 TS。

**可观测性纪律**：`internal/` 业务路径禁 `slog`；用量/Team 失败用 `CtxFlowLogWarn`（步骤注册表见 `52-flow-logger.design.md` §5.1）。

### 8.4 写入路径（真相源）

```
Chat Turn 结束 → service/turn_usage.recordTurnUsage
              → biz.RecordTokenUsageEvent（归一 status + 定价 + 费用）
              → data/usage_write（events + sessions 聚合 + daily upsert）

（可选）CHAT_RECORD_RUNNER_USAGE=1 → event_bus_runner_handler
（已停用默认）CHAT_RECORD_USAGE_INGRESS=1 → recordChatIngressUsage
```
