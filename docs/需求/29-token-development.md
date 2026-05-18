# Token 用量 — 开发计划

> **版本**：2026-05-19 | **状态**：✅ 基础功能已实现
> **需求**：[29 token.md](./29%20token.md) · **设计**：[29 token.design.md](./29%20token.design.md)
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
| 用量记录写入 | ✅ | `internal/service/chat_usage_ingress.go` → `UsageUsecase.RecordTokenUsageEvent` |
| Session 聚合更新 | ✅ | `internal/data/usage_write.go` 事务内 UPDATE sessions |
| 日聚合 upsert | ✅ | `internal/data/usage_write.go` → `upsertModelTokenUsageDaily` |
| 用量概览 API | ✅ | `UsageService.GetUsageOverview` → today/yesterday/month/range/trends/top_models/top_agents/anomalies |
| 趋势查询 API | ✅ | `UsageService.ListUsageTrends` |
| Top 模型排行 API | ✅ | `UsageService.ListTopModels` |
| Top Agent 排行 API | ✅ | `UsageService.ListTopAgents` |
| 明细列表 API | ✅ | `UsageService.ListUsageEvents` |
| 用量事件写入 API | ✅ | `UsageService.RecordTokenUsageEvent` |
| 查询筛选（Provider/Model/Agent/Status/时间） | ✅ | `UsageQuery` + `usageWhere()` |
| 异常请求筛选 | ✅ | `status = "abnormal"` → `status <> 'success'` |
| Wire 注入 | ✅ | `NewUsageRepo` / `NewUsageUsecase` / `NewUsageService` |
| 用量限额（quota） | ❌ | 无 `usage_quotas` 表和代码 |
| 用量告警（budget alert） | ❌ | 无 `budget_alerts` 表和代码 |
| 价格自动同步 | ❌ | `model_pricing_rules` 仅有 Ent Schema，无同步逻辑 |
| 小时聚合表 | ❌ | 仅有 `model_token_usage_daily`，无 hourly |

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
| 明细列表页 | ❌ | 无独立明细页组件 |
| 限额配置 UI | ❌ | 无 |
| 告警配置 UI | ❌ | 无 |
| CSV 导出 | ❌ | 无 |

### 2.3 差距总结

| 优先级 | 差距 | 影响 |
|--------|------|------|
| P2 | 无用量限额，用户可能产生意外高额费用 | 成本失控风险 |
| P2 | 无独立明细列表页，无法做精细分析 | 运营效率受限 |
| P3 | 无用量告警，达到阈值时无法通知用户 | 无法及时预警 |
| P3 | 价格规则无自动同步，需手动维护 | 运营负担 |
| P3 | 无 CSV 导出 | 数据分析不便 |
| P3 | 无小时聚合表 | 短期趋势分析粒度不够 |

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

- [ ] 可为 Agent / 用户 / 全局设置月度费用预算
- [ ] 超过预算后 Agent 对话被拦截并提示
- [ ] 预算每月自动重置
- [ ] quota 检查对 chat 延迟影响 < 50ms

### Phase 2

- [ ] 独立明细列表页支持筛选、分页、排序
- [ ] 限额配置 UI 可正常设置和修改
- [ ] 概览页展示月预算使用率

### Phase 3

- [ ] 达到告警阈值时通知用户
- [ ] 告警阈值可配置
- [ ] 价格规则可从 Provider API 自动同步

### Phase 4

- [ ] 小时级趋势查询可用
- [ ] CSV 导出功能可用
- [ ] 低性价比模型可被识别和标记

---

## 6. 依赖与风险

| 风险 | 影响 | 缓解 |
|------|------|------|
| Quota 检查增加 Chat turn 延迟 | 用户体验 | 1 次 DB 查询，预期 < 10ms；可加内存缓存 |
| Quota 重置周期边界 | 月末/月初可能重复计算 | 使用 `period_start` / `period_end` 明确界定 |
| 价格自动同步 API 变更 | 同步失败 | 保留 manual fallback；同步失败不阻塞 |
| 告警通知频率过高 | 通知疲劳 | 去重 + 频率限制 |
