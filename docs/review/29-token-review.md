# 29 Token / Usage / Quota Review

> **评分**：79 / 100 | **风险等级**：P1  
> **文档**：[29-token-development.md](../需求/29-token-development.md)  
> **代码锚点**：`internal/service/usage.go` · `internal/biz/usage*.go` · `internal/biz/usage_quota*.go` · `web/src/features/usage/`  
> **审查时间**：2026-05-21

---

## 评分详情

| 维度 | 得分 | 满分 | 评述 |
|------|------|------|------|
| 需求符合度 | 16 | 20 | Usage 事件、Quota MVP、Budget Alert、billable 读层均已实现；定价规则未配置时静默失败问题待解 |
| 架构一致性 | 21 | 25 | Usage 通过 EventBus 流式写入 ✅；Quota 在 Turn 前 `CheckQuota` 拦截 ✅；billable 排除 `team_turn` ✅ |
| 后端实现质量 | 17 | 20 | `usage_hourly` 聚合 ✅；`budget_alert` 异步告警 ✅；`team_member` 并行模式 `agent_key` 回写 ✅ |
| 前端实现质量 | 13 | 15 | 概览 Dashboard ✅；`/usage/events` 详细列表 + 筛选 + CSV 导出 ✅；Agent 权限 Tab 配额配置 ✅ |
| 测试与验证 | 6 | 10 | `usage_where_test.go` ✅；Quota 拦截路径测试待补 |
| 文档一致性 | 6 | 10 | `29-token-development.md` §9 billable 读层同步；趋势组件路径 2026-05-21 更正 |

---

## 已验收功能

| 功能 | 状态 |
|------|------|
| `model_token_usage_events` 逐条记录 | ✅ |
| EventBus 流式写入 | ✅ |
| `usage_hourly` 聚合 | ✅ |
| `CheckQuota` Turn 前拦截 | ✅ |
| Quota MVP（Agent/global scope）| ✅ |
| `budget_alerts` 异步预算告警 | ✅ |
| 低性价比模型告警 | ✅ |
| billable 排除 `team_turn` | ✅ §3.6 |
| `GET /v1/usage/events/export` CSV 下载 | ✅ |
| Agent 权限 Tab 配额配置面板 | ✅ |
| `model_pricing_rules` 定价配置 | ✅ |
| Team 会话 Quota（成员逐一检查）| ✅ |
| 全平台月预算（`system_settings.global_monthly_micro_usd`）| ✅ |
| 定价规则未配置时的用户提示 | ❌ |

---

## 统计口径

| 口径 | 含义 |
|------|------|
| `chat_turn` | 单 Agent 对话（billable）|
| `team_member` | Team 成员步骤（billable，parallel 模式按 `agent_key` 回写）|
| `team_turn` | Team 整轮聚合（对账用，不计入 billable）|

**关键规则**：概览/排行/配额已用额仅计 `chat_turn` + `team_member`；`/usage/events` 列表含 `team_turn`（需用户理解）。

---

## 主要风险

### P1

| ID | 问题 | 建议修复 |
|----|------|---------|
| TOK-P1-01 | 定价规则未配置时 `total_cost_micro_usd=0` 且 Quota SUM 失效，无任何用户提示 | 在 `/models` 页添加警告横幅；在 Usage 列表 cost=0 时加提示 |
| TOK-P1-02 | Quota 拦截路径（`CheckQuota` → 拒绝 turn）缺乏单测 | 补 Quota 拦截单测，含 Agent/global 两种 scope |

### P2

| ID | 问题 | 建议修复 |
|----|------|---------|
| TOK-P2-01 | `/usage/quotas` 旧路由重定向至 `/agents`（书签兼容），需文档化说明 | 在 `frontend-pages.md` 中注明此重定向 |
| TOK-P2-02 | `team_turn` vs `team_member` 口径差异对用户认知有挑战 | 在 Usage 列表页添加口径说明工具提示 |

---

## 建议优化路径

1. 添加定价规则未配置的用户提示（P1）。
2. 补 Quota 拦截单测（P1）。
3. 在 Usage 列表添加统计口径说明（P2）。
