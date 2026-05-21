# 用量明细与配额链路优化（2026-05-20）

## 背景

概览、用量事件、监控三页职责已划分；配额与费用写入、Team 拦截、重复预算 UI 存在缺口。

## 变更

### 后端

- `RecordTokenUsageEvent`：归一化 `status`（`ok`→`success`），按 `model_pricing_rules` 补单价并计算 `total_cost_micro_usd`。
- `usageWhere`：前端 `success`/`error` 与历史 `ok` 及异常状态对齐。
- Team Chat：`checkTeamMemberQuotas` 在 `RunTurn` 前检查启用成员 Agent 配额。
- 新增 `internal/biz/usage_cost.go`、`internal/data/usage_pricing.go`、`internal/service/chat_quota.go`。

### 前端

- `UsageEventsPage`：费用、来源、错误列；模型筛选；状态筛选项与概览一致。
- `AgentSettingsPage`：移除 Agent Tab 重复月度预算，引导至权限 Tab `AgentUsageQuotaPanel`。

### 文档

- `docs/需求/frontend-pages.md` §4.2
- `docs/需求/29-token-development.md` 现状表
- `docs/需求/29-token-development.md` §7（用量三页与配额链路迭代）

## 复审修复（同日）

- 移除 `SendChatMessage` 对 `recordChatIngressUsage` 的调用，消除与 `recordTurnUsage` 双写；ingress 改为显式 `CHAT_RECORD_USAGE_INGRESS=1` 才启用。
- EventBus Runner 用量补 `id` / `usage_kind`；聚合 SQL 兼容 `ok`/`error`；`error` 归一为 `failed`。
- 概览页增加「查看明细」入口；`usage_quota_test.go`；开发计划 §8 审计表。

## 迭代二（同日）

### 后端

- Team：`recordMemberUsage` → `usage_kind=team_member`（`internal/team/usage_record.go`）。
- 定价：`GetActiveModelPricing` 回退 `llm_provider_models.config_json`。
- Schema：`budget_alerts`、`model_token_usage_hourly`（`EnsureUsageExtraSchema`）。
- API：`quota_dashboard`、`granularity`、`ListBudgetAlerts`、`SetBudgetAlert`、`ExportUsageEvents`。
- 告警：`EvaluateBudgetAlerts` + `NewMonitorBudgetAlertNotifier`。

### 前端

- 概览：月预算使用率卡片、趋势粒度（天/小时）。
- 明细：CSV 导出。
- Agent 权限 Tab：预算告警阈值。

### 规范复盘（同日续）

- Team 用量失败：`slog` → `event.CtxFlowLogWarn`（`team.usage_record_fail`），符合 FlowLog v2。
- Wire：撤销手改 `wire_gen.go`；`provideUsageUsecase` 仅声明于 `cmd/admin/wire.go`，经 **`make wire`** 生成。
- 前端：Page → Store/composable（`useOverviewPage`、`useUsageEventsPage`）。

### 文档

- `docs/sql/08_usage.sql` 补全 quota / alert / hourly 表。
- `29-token-development.md` 现状、§8.5/§8.6（待优化 + 生成物/FlowLog 纪律）。
- `52-flow-logger.design.md` §5.1 增加 `team.usage_record_fail`。

## 迭代三（§8.6 优化清单）

### 后端

- Ent：`budget_alert`、`model_token_usage_hourly`、`usage_quota` schema；删除 `EnsureUsageExtraSchema`。
- Biz：`scheduleBudgetAlerts` 异步；`InefficientModels`；`user`/`global` quota 汇总；`usage_quota_bench_test.go`。
- Team：`usage_kind=team_turn` 聚合（`recordTeamRunUsage`）。
- Data：`mapUsageRepoErr` + `ErrBudgetAlertNotFound`。

### 前端

- 概览：`UsageInefficientModels` 低性价比模型卡片。

### 文档

- `29-token-development.md` §8.6 状态表更新。

## 迭代七（billable 统计口径 · 读层）

### 口径

- 可计费聚合：`chat_turn` + `team_member`；**排除** `team_turn`（整轮对账）。
- Team 整体：`SUM(team_member WHERE team_id=?)`；明细页仍展示全部 kind。

### 后端

- `sqlUsageBillableKind`、`usageWhere(query, billableOnly)`；概览/趋势/Top/配额 SUM 使用 `billableOnly=true`。
- `UsageQuery.team_id` / `usage_kind`（Proto #10/#11）；`usage_where_test.go`。

### 前端

- `/usage/events` 筛选 Team ID、来源 `usage_kind`。

### 文档

- `29 token.md` §3.6 · `29 token.design.md` §4.5 · `29-token-development.md` §9 · `frontend-pages.md` §4.2。

## 迭代六（O4 · Team 成员用量回写）

- `EventStreamResult.MemberUsage`：`internal/agent/turn_helpers.go` 按 `ev.Author`（agent_key）累积成员 `Usage`。
- `stepTokensForMember`：`internal/team/usage_tokens.go`；`runner_team_trpc` 不再仅给 sortIdx=0 填 tokens。
- 单测：`turn_helpers_test.go`、`usage_tokens_test.go`。

## 迭代五（系统设置 · 全局配额）

- `system_setting.global_monthly_micro_usd` + Proto/API/Ent。
- 保存系统设置时 `SystemSettingUsecase` 同步 `usage_quotas`（`global`/`global`）。
- 前端 `/settings` 增加「全平台月预算」表单项。

## 迭代四（架构/SRP 复盘）

### 后端

- Biz：`UsageAnalyticsRepo` / `UsageWriteRepo` / `UsageQuotaRepo` 组合为 `UsageRepo`（`usage_repo.go`）。
- Data：`usage_quotas` / `budget_alerts` 改 Ent CRUD + upsert；移除 `SumAgentCostInPeriod`。
- Service：`usage_mapper.go`（`toProto*` / `fromProto*`）；`enforceChatTurnQuotas`（agent/user/global）。
- 告警：`EvaluateBudgetAlerts` 含 `global` scope。

## 验证

```bash
go generate ./internal/data/ent/...
make wire-admin && make test
go test ./internal/biz/... -count=1
go build ./...
cd web && pnpm test --run && pnpm build
```
