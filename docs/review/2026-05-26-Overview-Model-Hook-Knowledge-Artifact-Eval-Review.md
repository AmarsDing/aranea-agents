# 概览 / 模型管理 / Hook 回调 / 知识库 / 制品管理 / 评估管理 — 跨模块代码审查

> **审查时间**：2026-05-26
> **范围**：以下 6 大业务模块，覆盖后端（biz / service / data / engine）+ 前端（Vue / Pinia）+ proto/API + Wire 装配 + 集成层；**不含 `pkg/trpc-agent-go/`**。
> **真相源**：`docs/AGENT_RUNTIME_BOUNDARY.md`、`docs/AI-全栈新功能开发规范.md`、`.cursor/rules/trpc-agent-framework-first.mdc`、`AGENTS.md`
> **审查维度**：架构 / 代码质量 / 功能正确性 / 性能 / 可维护性 / 错误处理 / 兼容性 / 合规 / 业务逻辑 11 条
> **历史 Review**：[27-artifact-review.md](./27-artifact-review.md)、[33-evaluation-review.md](./33-evaluation-review.md)、[37-knowledge-review.md](./37-knowledge-review.md)、[22-28-plugin-callback-review.md](./22-28-plugin-callback-review.md)、[09-provider-review.md](./09-provider-review.md)

---

## 0. 总体评分（满分 100，加权按"业务侵入度 + 风险面 + 代码体量"）

| 模块 | 评分 | 风险等级 | 一句话评述 |
|------|------|---------|-----------|
| 概览 / Dashboard | **77** → **80**（Round 5） | P1 | 分层干净；Quota N+1 已修（BatchSumScopeCost），Health 并发已修；后端 10+ 顺序聚合无并行/缓存，前端无错误 UI，仅 UTC 日历 |
| 模型管理 | **74** → **78**（Round 5） | P1 | 双子系统拆分清晰；SSRF 防护已修，Health 并发已修；价格三写入路径漂移，自动迁移耦合 sync |
| Hook 回调 | **66** → **72**（Round 5） | P1 | 两条独立回调链已闭环；HookResolver 缓存已修、HMAC 签名已修、delivery worker 已修、secret 脱敏已修；Gateway webhook fire-and-forget 无持久化 |
| 知识库 / RAG | **72** → **78**（Round 1）→ **84**（Round 2）→ **88**（Round 3）→ **92**（Round 4 P1-P2 修复后） | P2 | 完整分层 RAG 骨架；Embedder 解耦已修、Team KnowledgeBases 已注入、Gemini task type 已分、IVFFlat 参数化、slugify 唯一、watch.Runner 窄接口 |
| 制品 / Artifact | **76** | **P0** | ART-01 MVP 落地；**session_id 无路径校验可越权**、默认签名 key、`DeleteArtifact` 仅删一版 |
| 评估管理 | **78** | P1 | 框架桥接干净、异步执行无阻塞；**数据集删除不级联 runs**、UI 缺案例上传、judge 失败静默、每用例一会话 |

**跨模块共性问题**（P0 优先）：

1. **出站 HTTP 缺 SSRF / 鉴权一致性**：modelcatalog 已做 url-guard，但 inspect / health / webhook / hook notify 各自一套或没有；建议抽 `pkg/outboundguard` 统一拦截。
2. **持久化重试 worker 不全**：channel delivery 有 cron worker，hook delivery 没有，gateway webhook 完全 fire-and-forget——同公司同仓库内三种成熟度，运维心智负担大。
3. **JSONB / scores_json / config_json / options_json / scores_json 等"半结构化兜底字段"四处分散**：缺统一 schema + 版本号 + migration helper；改造时 silent break 风险高。
4. **后端聚合查询无并行 / 无缓存**：概览 / quota dashboard / catalog status 等都是顺序串行；前端无 stale-while-revalidate；用户感知慢。
5. **测试金字塔倒挂**：data 层 SQL 集成测最薄弱；service 层 RPC 测覆盖 < 20%；多模块（hook / overview / knowledge service / artifact service / eval runner）零集成测。

---

## 1. 概览 / Dashboard

### 1.1 模块定位

`web/src/pages/OverviewPage.vue`（约 170 行）是 **模型用量分析仪表盘**（route `/overview`，默认首页），不是泛用系统大盘。聚合来源：

- **主路径**：`GET /v1/usage/overview` —— 单一聚合 RPC，返回 today/yesterday/month/range 四个 summary + trends + top_models/agents + anomalies + quota_dashboard + inefficient_models。
- **附属面板**：`GET /v1/monitor/runner-metrics?windowMinutes=60` —— Runner completion 错误率指标，与用量 filter 不共享时间范围。

> 后端 **没有** 独立的 `OverviewSummary` / `DashboardStats` RPC；`internal/biz/monitor.go` 仅做类型别名，**不是** 概览的数据源。

### 1.2 文件清单

| 层 | 文件 | 行数 | 职责 |
|----|------|------|------|
| **前端页面** | `web/src/pages/OverviewPage.vue` | ~170 | Shell + filter + loading overlay |
| **前端编排** | `web/src/features/usage/useOverviewPage.ts` | 72 | filter state；委托 `useUsageStore` |
| **前端 store** | `web/src/stores/usage/index.ts` | 82 | `loadOverview`、hourly trends override |
| **前端 API** | `web/src/features/usage/api.ts` | 343 | proto ↔ camel/snake mapping |
| **前端组件** | `web/src/components/usage/UsageMetricCards/TrendChart/BreakdownCharts/TopModels/TopAgents/Inefficient/Anomaly.vue` | 460+ | KPI / pie / list / table |
| **前端图表** | `web/src/features/usage/useUsageChart.ts` 等 | 95 | ECharts 包装 |
| **后端 service** | `internal/service/usage.go` | 144 | `GetUsageOverview` handler |
| **后端 service** | `internal/service/usage_mapper.go` | ~276 | proto ↔ biz mapping |
| **后端 biz**（**核心**） | `internal/biz/usage/usage.go` | **923** | `Overview()`、日期归一、quota dashboard、inefficient 等 |
| **后端 data** | `internal/data/usage.go` | 238 | events 表聚合（summary/trends/top/anomalies） |
| **后端 data** | `internal/data/usage_hourly.go` | 67 | hourly rollup（仅 trends） |
| **后端 data** | `internal/data/usage_quota.go` | ~129 | quota CRUD + `SumScopeCostInPeriod` |
| **后端 data** | `internal/data/usage_write.go` | ~184 | event insert + 日/小时 rollup upsert |

### 1.3 数据流（实测路径）

```
浏览器 /overview
  └─ useOverviewPage.loadOverview(filters)
       └─ useUsageStore.loadOverview(query, trendGranularity)
            ├─ GET /v1/usage/overview                  ← 主请求
            └─ IF granularity = hour → GET /v1/usage/trends?granularity=hour
                 (覆盖 overview.trends；首次返回的 daily trends 被丢弃)

服务端 GetUsageOverview
  └─ UsageUsecase.Overview()  [SEQUENTIAL]
       ├─ 4× GetModelUsageSummary  (today/yesterday/month/range)
       ├─ 1× Trends                (events 或 hourly rollup)
       ├─ 1× ListTopModelUsage     (limit 8)
       ├─ 1× ListTopAgentUsage     (limit 8)
       ├─ 1× ListModelUsageEvents  (limit 12, status=abnormal)
       ├─ 1× QuotaDashboard
       │      ├─ ListActiveQuotas
       │      └─ N× SumScopeCostInPeriod   ← N+1
       └─ 1× InefficientModels  (重复扫 top-32 → biz 规则)

并行独立路径（OverviewRunnerMetrics）：
  GET /v1/monitor/runner-metrics → monitor events 表
```

### 1.4 关键问题

| ID | 优先级 | 问题 | 位置 |
|----|--------|------|------|
| OV-01 | **P0** | `Overview()` 10+ 顺序 DB 调用，无 errgroup、无缓存 | `internal/biz/usage/usage.go:379-418` |
| OV-02 | **P0** | 写入时维护 `model_token_usage_daily/hourly` rollup，**读路径全部扫 raw events** | `internal/data/usage.go` vs `usage_write.go` |
| OV-03 | **P0** | `loadOverview` 静默 catch；失败用户无感知 | `web/src/stores/usage/index.ts:20-31` |
| OV-04 | P1 | hourly 模式发两次 HTTP，第一次的 daily trends 计算被丢 | `stores/usage/index.ts:23-26` |
| OV-05 | P1 | `usageHourlyWhere` 缺 team_id / status / provider alias | `internal/data/usage_hourly.go:39-46` |
| OV-06 | P1 | `QuotaDashboard` N+1 `SumScopeCostInPeriod` 每配额一次 | `usage/usage.go:824-826` |
| OV-07 | P1 | `QuotaDashboard` 单条配额错误用 `continue` 静默低报 | `usage/usage.go:824-826` |
| OV-08 | P2 | 日历边界仅 UTC，UTC+8 用户"今天"切换在 08:00 | `usage/usage.go:306, 315-316` |
| OV-09 | P2 | "Provider 饼图"实际只用 top-8 models 重排，标签易误导 | `web/src/features/usage/usageBreakdownSlices.ts:19-32` |
| OV-10 | P2 | RunnerMetricsPanel 不展示 `avg_duration_ms` 但 proto 已带 | `web/src/features/monitor/api.ts:324-333` |
| OV-11 | P2 | filter 不写回 URL；只读初始 `range`，不利于分享/刷新 | `useOverviewPage.ts:15-17` |
| OV-12 | P2 | 无 polling / staleness 标识，运维大盘可能陈旧 | `useOverviewPage.ts` |
| OV-13 | P2 | 测试覆盖：无 `Overview()` 集成测、无 `GetUsageOverview` handler 测、无 `OverviewPage` 组件测 | — |

### 1.5 优化建议（设计草案）

**P0-1：并行化 + rollup 读路径**

```go
// internal/biz/usage/usage.go
func (u *UsageUsecase) Overview(ctx context.Context, q UsageQuery) (Overview, error) {
    g, ctx := errgroup.WithContext(ctx)
    var (
        today, yesterday, month, rangeSum ModelUsageSummary
        trends []ModelUsageTrendPoint
        topModels, topAgents []UsageBreakdownRow
        anomalies []ModelTokenUsageEvent
        quota *QuotaDashboard
        inefficient []UsageModelInsight
    )
    g.Go(func() error { return u.summaries(ctx, q, &today, &yesterday, &month, &rangeSum) })
    g.Go(func() error { var err error; trends, err = u.repo.ListUsageTrendsFromRollup(ctx, q); return err })
    g.Go(func() error { var err error; topModels, err = u.repo.ListTopModelUsage(ctx, q, 32); return err })
    // ...
    if err := g.Wait(); err != nil { return Overview{}, err }
    inefficient = filterInefficient(topModels) // 复用 topModels limit 32 结果，砍掉重复查询
    topModels = topModels[:min(8, len(topModels))]
    return Overview{...}, nil
}
```

**P0-2：读 rollup（daily/hourly）替代 events 全表扫**：新增 `repo.GetSummaryFromDaily / ListTrendsFromDaily`，以 `model_token_usage_daily` 为主，events 仅用于 anomalies 列表。配套 reconciliation 测试每天对账。

**P0-3：前端 error UX**：`overviewError: Ref<string>`，`q-banner` 重试按钮，复用 `loadEvents` 的 `error` ref 模式。

**P1-4：单请求 hourly**：把 `granularity` 透传给 `getModelUsageOverview`，后端 `Overview.Trends()` 已支持 hourly，无需第二次 HTTP。

**P1-5：QuotaDashboard 批量化**：单条 SQL `SELECT scope_type, scope_id, SUM(total_cost_micro_usd) ... GROUP BY scope_type, scope_id`，配合一个 60s 进程内 LRU。

**P2-6：URL 同步 filters + 可选 30s 自动刷新（visibilitychange 暂停）**。

**P2-7：业务面优化**：

- 提供 **`top_providers` 子聚合**：后端 `GROUP BY provider_code` 给出真实的 provider 占比。
- 日历切换：用户级"业务日"配置（在 `system_settings` 中），存 `tz_offset`；biz `dateBoundary(now, tz)` 替代写死 UTC。
- 仪表盘添加 **"数据时效"** 标记：`fresh_until = now + 60s`，超时用户能看到提示。

---

## 2. 模型管理

### 2.1 模块定位

模型管理是 **两个耦合子系统**：

| 子系统 | 真相源 | 主要包 |
|--------|--------|--------|
| **LLM Provider Models**（管理员手配的接入点） | DB `llm_provider_models` + `model_pricing_rules` | `internal/biz/llm_provider_model.go` / `service` / `data` + `internal/llminspect` + `internal/provider` |
| **Model Catalog**（models.dev 本地镜像） | 磁盘 `{root}/data/model-catalog/*.json` | `internal/modelcatalog/` + `internal/biz/model_catalog.go` + `internal/data/model_catalog_apply.go` |

前端 **资源中心 `/models`**（ResourceManager）做 CRUD / inspect / 凭证，**系统设置 → 模型目录**（SystemSettingsCatalogTab）做 sync / 策略 / 迁移 / 浏览。

### 2.2 文件清单（核心）

| 层 | 文件 | 行数 | 职责 |
|----|------|------|------|
| **biz** | `internal/biz/llm_provider_model.go` | 493 | 类型、Repo、CRUD/inspect/validate、`RunHealthChecks` |
| | `internal/biz/model_catalog.go` | 334 | `ModelCatalogUsecase`：policy/status/list/search/sync/migration/logos |
| | `internal/biz/credential_crypto.go` | 430 | AES `config_json` 加解密、merge、sanitize |
| | `internal/biz/credential_key.go` | 83 | `ARANEA_CREDENTIAL_KEY` 解析 |
| **service** | `internal/service/llm_provider_model.go` | 237 | proto handler + 审计 |
| | `internal/service/model_catalog.go` | 303 | catalog RPC + soft-fail sync |
| | `internal/service/catalog_context.go` | 38 | chat 拿 `ModelConfigJSON` |
| **data** | `internal/data/llm_provider_model.go` | 251 | Ent CRUD + `UpsertModelPricingRule` |
| | `internal/data/model_catalog_apply.go` | 375 | `ApplyBackend`、事务化 provider migration |
| | `internal/data/pricing_patch.go` | 64 | SQLite DDL patch |
| **modelcatalog 引擎** | `catalog.go` / `store.go` / `sync.go` / `fetch.go` / `fetch_retry.go` / `urlguard.go` / `apply.go` / `config_merge.go` / `overlay.go` / `runtime_overlay.json` / `chips.go` / `search.go` / `pricing.go` / `backfill.go` / `migrate.go` / `migrate_bindings.go` / `migration_map.go` / `migration_checkpoint.go` / `runner.go` / `store_provider.go` / `logs.go` / `logos.go` / `policy_validate.go` | ~2,200 | 完整 catalog 同步、应用、迁移、search、logos |
| **inspect / runtime provider** | `internal/llminspect/inspect.go` | 408 | provider 元数据 HTTP 探测 |
| | `internal/provider/trpc_llm.go` | 338 | 从 DB → tRPC `model.Model`，preflight HEAD，HA |
| | `internal/provider/catalog.go` | 214 | `config_json` → `CatalogConfig` 解析 |
| | `internal/provider/rate_limit_transport.go` | 69 | 每模型 RPM 令牌桶 |
| **proto** | `api/kratos/llm_provider_model/v1/llm_provider_model.proto` | 165 | 8 RPC |
| | `api/kratos/model_catalog/v1/model_catalog.proto` | 251 | 12 RPC |

### 2.3 架构 / 依赖方向

```
HTTP/gRPC
  └─ internal/service          (proto ↔ biz, 审计)
        └─ internal/biz        (用例、凭证规则、运行时桥)
              ├─ internal/data       (Ent/SQL via interface)
              └─ internal/modelcatalog  (纯 catalog 引擎)
        └─ internal/llminspect (HTTP inspect)

Chat 运行时：
  internal/service (ChatOrchestrator)
    └─ internal/provider.TRPCModelForProviderModel
          └─ biz.LlmProviderModelUsecase.GetByProviderAndModel    ⚠ provider import biz
```

- ✅ `internal/biz` 不 import `pkg/trpc-agent-go`、`internal/data`。
- ⚠ `internal/provider` import `internal/biz`（`trpc_llm.go:12`）——基础设施向上依赖用例层，应通过端口反转。
- ✅ `llminspect` 独立于 `provider` 避免循环依赖。

### 2.4 关键问题

| ID | 优先级 | 问题 | 位置 |
|----|--------|------|------|
| MD-01 | **P0** | **Inspect / Health / Preflight 完全没有 SSRF 防护**，admin 可填任意 `api_base_url` 探测内网 | `internal/llminspect/inspect.go:349-373`、`internal/biz/llm_provider_model.go:472-476`、`internal/provider/trpc_llm.go:52-58` |
| MD-02 | P1 | 价格三写入路径（catalog apply / 手动保存 / inspect 同步）无优先级合约，`source` 字段做不到"manual lock" | `internal/data/llm_provider_model.go:188-214`、`internal/modelcatalog/apply.go:144-148` |
| MD-03 | P1 | `Applier.Apply` 默认调 `RunProviderMigrations`——auto_apply 用户每次 sync 都被迫迁移 | `internal/modelcatalog/apply.go:70-82` |
| MD-04 | P1 | `ApplyProviderMigration` 无 `dry_run` / 无回滚快照 | `internal/modelcatalog/migrate_bindings.go:30`、`migration_checkpoint.go` |
| MD-05 | P1 | `RunHealthChecks` 串行扫所有 enabled，无并发限制、无 jitter | `internal/biz/llm_provider_model.go:459-490` |
| MD-06 | P1 | catalog URL 检查允许 "公网 DNS + HTTPS" 的任意主机；models.dev 之外缺路径白名单 | `internal/modelcatalog/urlguard.go:39-58` |
| MD-07 | P2 | `isRetryableFetchErr` 把"no such host"判为可重试，浪费 4 次 attempts | `internal/modelcatalog/fetch_retry.go:38-49` |
| MD-08 | P2 | `SyncModelCatalog` 用 HTTP 200 + `ok:false` 表达错误，gateway 难统一识别 | `internal/service/model_catalog.go:168-185` |
| MD-09 | P2 | `ListCatalogModels` 前端 N+1（每 provider 一次 RPC） | `web/src/features/model-catalog/api.ts:63-104` |
| MD-10 | P2 | `RevealProviderModelCredentials` 无频控，可被滥用扫描凭证 | `internal/service/llm_provider_model.go:122-127` |
| MD-11 | P2 | `migration-checkpoint.json` 解析失败静默归零，掩盖损坏 | `internal/modelcatalog/store.go:58-60` |
| MD-12 | P2 | 测试空白：`data/model_catalog_apply` 迁移事务、`RunHealthChecks` 状态机、`service` 层 0 覆盖 | — |
| MD-13 | P3 | `internal/provider` → `internal/biz` 上行依赖，建议用 `ProviderModelCatalog` 端口反转 | `trpc_llm.go:12` |

### 2.5 业务逻辑层优化设计

**P0 — 统一出站 URL 守卫（强烈推荐独立包）**

```go
// pkg/outboundguard/url.go
type Policy struct {
    AllowSchemes    []string         // ["https"] 或 ["http","https"]
    AllowDomains    []string         // 显式白名单（catalog 用）
    BlockPrivateIPs bool             // 默认 true
    BlockLoopback   bool             // 默认 true
    BlockMetadata   bool             // 169.254.169.254 等
}
func Validate(rawURL string, p Policy) (resolvedIP net.IP, err error)
func ValidateAndPin(rawURL string, p Policy) (*http.Transport, net.IP, error)
```

调用点：

- `llminspect.getProviderJSON` → Validate(rawURL, providerInspectPolicy)
- `biz.RunHealthChecks` → Validate(...)
- `provider.TRPCModelForProviderModel` preflight → Validate(...)
- `modelcatalog/urlguard.go` 现有逻辑迁入

**P1 — 价格优先级合约**

```go
// model_pricing_rules 增加列：source ENUM('manual','model-inspect','models.dev-sync','env')
//                              locked_at TIMESTAMP NULL
//
// upsert rule:
//   manual + locked_at → 永久封顶，sync/inspect 不覆盖
//   model-inspect       → 覆盖 models.dev-sync，但不覆盖 manual
//   models.dev-sync     → 覆盖 env，但不覆盖更高优先级
//   env                 → 最低，仅在 rules 表为空时回填
```

**P1 — Migration dry-run + 一键回滚**

```go
// API:
//   POST /v1/model-catalog/apply-migration?dry_run=true
//     返回 rows_to_change 预览，不写库
//   POST /v1/model-catalog/apply-migration
//     执行前 snapshot: 写 migration-snapshots/<ts>.json（原 provider_code 全表 SELECT）
//   POST /v1/model-catalog/revert-migration?snapshot_id=<ts>
//     按 snapshot 反向更新
```

**P1 — Health worker pool + 状态机**

```go
const healthPoolSize = 5
sem := make(chan struct{}, healthPoolSize)
for _, m := range enabled {
    sem <- struct{}{}
    go func(m Model) {
        defer func() { <-sem }()
        // jitter 0–500ms，避免对同 provider 突发
        time.Sleep(time.Duration(rand.Intn(500)) * time.Millisecond)
        status := probeWithSSRFGuard(ctx, m)  // healthy / degraded / unreachable / unauthorized
        repo.UpdateHealth(ctx, m.ID, status)
    }(m)
}
```

---

## 3. Hook 回调

### 3.1 模块定位

Aranea 的"Hook 回调"实际上是 **两条并行的出站回调链路**，共用 SSRF 校验与部分基础设施，但 **数据模型、投递语义、可观测性差异很大**：

1. **Platform Hook**（需求 28 / `hooks` 表）—— Agent 生命周期规则（log / notify / block / modify），`notify` 写 `hook_deliveries` 并内联重试。
2. **Gateway Webhook**（需求 35 / `gateway_webhooks` 表）—— Run / Graph 终态事件，经 EventBus 异步 fan-out，**无持久化投递队列**。

### 3.2 文件清单

| 路径 | 行数 | 职责 |
|------|------|------|
| `internal/biz/hook.go` | 46 | re-export |
| `internal/biz/hook/hook.go` | 460 | Hook CRUD、config 解析、Delivery 类型、Resolver |
| `internal/biz/webhook.go` | 214 | Gateway Webhook CRUD |
| `internal/biz/webhook_dispatcher.go` | 152 | Gateway 异步 HTTP + HMAC-SHA256 |
| `internal/biz/event_bus_callback_consumer.go` | 67 | 订阅 `run_status` → Gateway 投递 |
| `internal/biz/event_bus_async.go` | 80 | 有界队列 + 满则丢弃 |
| `internal/service/hook.go` | 183 | RPC + `ReloadHooks` |
| `internal/service/gateway.go` | ~112 | Gateway Webhook RPC |
| `internal/data/hook.go` | 127 | `hooks` Ent repo |
| `internal/data/hook_delivery.go` | 125 | `hook_deliveries` 原生 SQL |
| `internal/data/hook_delivery_schema.go` | 17 | DDL embed + startup ensure |
| `internal/data/webhook.go` | 141 | `gateway_webhooks` Ent repo |
| `api/kratos/hook/v1/hook.proto` | 112 | 6 RPC |
| `internal/plugin/trpc/hook_callbacks.go` | 248 | 生命周期 Chain 回调 + action 执行 |
| `internal/plugin/trpc/hook_notify.go` | 127 | Hook notify 入队 + 内联重试 HTTP |
| `internal/plugin/trpc/hook_resilience.go` | 83 | 非 block 错误吞掉（Chain 专用） |
| `internal/plugin/trpc/hook_events.go` | 75 | `on_event` 点 Hook 分发 |
| `internal/plugin/trpc/hook_audit.go` | 33 | blocked/error → `plugin_runs` |
| `internal/plugin/trpc/manager.go` | ~188 | MergeChain、ReloadHooks |
| `internal/plugin/trpc/runtime.go` | 263 | HookNotifier + delivery repo 注入 |
| `pkg/webhookurl/validate.go` | 62 | SSRF 校验（保存 + 投递前） |
| `pkg/webhookurl/client.go` | 20 | 出站 HTTP（禁 redirect） |
| `web/src/pages/HooksPage.vue` | 250 | Hook 规则 CRUD UI |
| `web/src/pages/HookDeliveriesPage.vue` | 203 | 投递只读 UI |

### 3.3 事件流（文本）

**Platform Hook — Chain 点（before_agent / before_tool / after_tool 等）**

```
用户消息 → ChatOrchestrator → TurnExecutor
  → callback_chain.buildProductCallbackChain
  → Manager.MergeChain(agentID)
       └─ HookResolver.Resolve()  ← 每次走 DB（无缓存，见 HK-03）
       └─ HookCallbacks() → callbacks.Chain entries
       └─ wrapResilientHooks() ← 非 block 错误吞掉

Hook 触发 → executeHookAction(log | notify | block | modify)
  notify 路径：
    → webhookurl.ValidateNotifyURL
    → HookNotifier.EnqueueNotify
    → safego: INSERT hook_deliveries (status=pending)
    → processDelivery: 内联 for-loop, 500ms × attempt, ≤ max_attempts=3
    → metrics.PluginInvokeTotal + ObserveCallback
    → recordHookAudit (blocked/error → plugin_runs)
```

**Platform Hook — `on_event` 点**：经 Runner `OnEvent` → `dispatchHookOnEvent` → 同上 notify 路径。**注意**：`on_event` **不过** `wrapResilientHooks`，block 错误会 propagate 到 Runner，与 Chain 点不一致。

**Gateway Webhook — Run / Graph 终态**

```
setRunStatus(completed | failed | cancelled)
  → PublishRunStatus → EventBus envelope
  → callbackConsumer (Reliable subscribe, buffer=128)
  → asyncEnvelopeWorker (queue=256, 满则丢)
  → WebhookDispatcher.Dispatch → safego fan-out
  → postOne per gateway_webhooks row
       └─ HMAC X-Webhook-Signature
       └─ 写 session 日志，无 DB 投递记录，无重试

GraphTaskRuntime.PublishTaskStatus → 同上（graph.task.status）
```

### 3.4 关键问题

| ID | 优先级 | 问题 | 位置 |
|----|--------|------|------|
| HK-01 | **P0** | **无后台 worker**；`hook_deliveries.status='pending'` 在进程崩溃后**永不重试** | `internal/plugin/trpc/hook_notify.go:65-73` |
| HK-02 | **P0** | **Hook notify 完全没有 HMAC / 签名**，与 Gateway 不一致；接收方无法验真 | `hook_notify.go:102-125` vs `webhook_dispatcher.go:124-128` |
| HK-03 | **P0** | **Gateway webhook 没有持久化 / 重试 / 投递记录**——丢失即丢失，UI 无可视 | `webhook_dispatcher.go:49-151` |
| HK-04 | P1 | `HookResolver.Reload` 是空操作，`Resolve` 每个 turn **全表 List(ctx.Background())** | `internal/biz/hook/hook.go:400-421, 435-439` |
| HK-05 | P1 | EventBus 侧效队列满则**静默丢** run_status，不会写 DLQ、不计数 | `internal/biz/event_bus_async.go:59-65` |
| HK-06 | P1 | 无投递幂等键（`(hook_key + event_id + payload_hash)`），重复触发 = 重复 POST | `hook_notify.go:54-64` |
| HK-07 | P1 | `on_event` 与 Chain 点失败策略不一致：on_event 不经 resilient 包装，block 会 propagate | `hook_resilience.go` vs `hook_events.go:47-48` |
| HK-08 | P1 | Gateway proto `Webhook` 响应包含 **完整 `secret`**，列表 / 详情泄露密钥 | `api/kratos/gateway/v1/gateway.proto:25-36` |
| HK-09 | P2 | 缺 timestamp 容差 / replay id，重放保护为零 | 全局 |
| HK-10 | P2 | Hook notify 无界 `safego.Go`，notify 风暴可拖死进程 | `hook_notify.go:47-73` |
| HK-11 | P2 | Gateway 签名头无 `sha256=` 前缀 / 版本，跟 Stripe/GitHub 业界不一致 | `webhook_dispatcher.go:127` |
| HK-12 | P2 | `ListHookDeliveries` 不支持按 `hook_id` / `delivery_id` 查单 | `hook.proto` |
| HK-13 | P3 | Gateway HTTP 与 Hook HTTP 重复 30%+ 逻辑，应抽 `pkg/outboundwebhook` | — |

### 3.5 业务逻辑层优化设计

**P0 — Hook 投递 Worker（对齐 Channel 模式）**

```sql
-- 1) 扩 schema
ALTER TABLE hook_deliveries ADD COLUMN next_retry_at TEXT;
ALTER TABLE hook_deliveries ADD COLUMN claimed_by TEXT;
ALTER TABLE hook_deliveries ADD COLUMN claimed_at TEXT;
ALTER TABLE hook_deliveries ADD COLUMN idempotency_key TEXT;
CREATE UNIQUE INDEX hook_delivery_idem ON hook_deliveries(hook_id, idempotency_key)
  WHERE idempotency_key IS NOT NULL;
CREATE INDEX hook_delivery_pending_idx ON hook_deliveries(status, next_retry_at);
```

```go
// internal/cronrunner/jobs/hook_delivery.go
type HookDeliveryWorker struct {
    repo  HookDeliveryRepo
    notif HookNotifier
}

func (w *HookDeliveryWorker) Tick(ctx context.Context) error {
    // SELECT ... FROM hook_deliveries WHERE status IN ('pending','retry')
    //   AND (next_retry_at IS NULL OR next_retry_at <= now())
    //   ORDER BY created_at LIMIT N FOR UPDATE SKIP LOCKED
    items, err := w.repo.ClaimPending(ctx, w.id, 50)
    if err != nil { return err }
    for _, it := range items {
        err := w.notif.DeliverOnce(ctx, it)
        backoff := nextBackoff(it.AttemptCount) // 指数 + jitter
        switch {
        case err == nil:
            w.repo.MarkSucceeded(ctx, it.ID)
        case it.AttemptCount+1 >= it.MaxAttempts:
            w.repo.MarkDeadLetter(ctx, it.ID, err)
        default:
            w.repo.MarkRetry(ctx, it.ID, err, time.Now().Add(backoff))
        }
    }
    return nil
}
```

收益：进程恢复、多实例安全（claim by uuid + SKIP LOCKED）、pending 不再永久挂起。

**P0 — 统一出站 Webhook 签名（Hook + Gateway 共用）**

```go
// pkg/outboundwebhook/sign.go
const SignatureHeader   = "X-Webhook-Signature"
const TimestampHeader   = "X-Webhook-Timestamp"
const DeliveryIDHeader  = "X-Webhook-Delivery-Id"
const SchemaVersion     = "v1"

func Sign(secret []byte, body []byte, ts time.Time) string {
    msg := fmt.Sprintf("%d.%s", ts.Unix(), body)
    mac := hmac.New(sha256.New, secret)
    _, _ = mac.Write([]byte(msg))
    return fmt.Sprintf("%s=%s", SchemaVersion, hex.EncodeToString(mac.Sum(nil)))
}
// 接收方校验：|now - ts| < 5min；HMAC equal；delivery_id 反重放
```

`hook_notify.go.deliverHookWebhook` 与 `webhook_dispatcher.postOne` 同时迁移至此。

**P0 — Gateway Webhook 持久化**：新增 `gateway_deliveries` 表（结构对齐 `hook_deliveries`），`callbackConsumer` 落库 → Worker 投递；EventBus 满时 **不再丢，而是阻塞 / 入 DLQ**。

**P1 — `HookResolver` 内存缓存 + 失效广播**

```go
type Resolver struct {
    uc       *Usecase
    mu       sync.RWMutex
    cache    []ResolvedHook
    loadedAt time.Time
}

func (r *Resolver) Reload(ctx context.Context) error {
    all, err := r.uc.List(ctx)
    if err != nil { return err }
    enabled := filterActive(all)
    r.mu.Lock(); r.cache, r.loadedAt = enabled, time.Now(); r.mu.Unlock()
    return nil
}

func (r *Resolver) Resolve(agentID, agentKey string) []ResolvedHook {
    r.mu.RLock(); defer r.mu.RUnlock()
    return filterByAgent(r.cache, agentID, agentKey)
}
// service.UpdateHook / DeleteHook 调 ReloadHooks（已有），多实例可用 PG NOTIFY 或基于 system_settings.updated_at 轮询
```

**P1 — 投递幂等键**：payload 加 `event_id = sha256(hook_key + chain_point + run_id + turn_id)`；DB 唯一索引；冲突即 skip。Gateway 用 `run_id + event_type`。

**P1 — 失败策略统一**：

```go
// hook action 配置增字段
type Action struct {
    Type      string // log/notify/block/modify
    FailPolicy string // "propagate" | "swallow" (default per type)
}
// Chain 点 / on_event 共用同一策略，block 始终 propagate；其余看配置
```

---

## 4. 知识库 / RAG

### 4.1 模块定位

Aranea 知识库是一条 **自研 RAG 管线**，未直接使用 `pkg/trpc-agent-go/knowledge.DefaultKnowledgeBase` 的 VectorStore 抽象：

- **引擎层** `internal/knowledge/`：提取、分块、嵌入、检索、重排
- **业务层** `internal/biz/knowledge/`：集合 / 文档 CRUD + 搜索参数校验
- **数据层** `internal/data/knowledge.go`：PostgreSQL + pgvector 三表
- **服务层** `internal/service/knowledge*.go`：RPC、异步入库、Prometheus
- **Agent 集成** `internal/tools/knowledge/`：`knowledge_search` 工具 + Context 注入

### 4.2 文件清单

| 文件 | 行数 | 职责 |
|------|------|------|
| `internal/knowledge/chunker.go` | 113 | 字符/伪 token 滑动窗口 |
| `internal/knowledge/chunk_strategy.go` | 87 | char/markdown/json/recursive |
| `internal/knowledge/document_extract.go` | 114 | OCR → HTML strip → trpc readers → UTF-8 |
| `internal/knowledge/embedder.go` | 355 | OpenAI / Ollama / Gemini / HuggingFace TEI |
| `internal/knowledge/ingest.go` | 93 | `BuildIndexedChunks` 编排 |
| `internal/knowledge/ocr.go` | 58 | OCRProvider 接口 + env 选择（**目前只有 stub**） |
| `internal/knowledge/reranker_factory.go` | 75 | Cohere / Infinity / topk |
| `internal/knowledge/retriever.go` | 163 | 嵌入 → cosine → optional rerank → top-k |
| `internal/biz/knowledge/knowledge.go` | 310 | Collection / Document / Chunk / Repo / Usecase |
| `internal/service/knowledge.go` | 331 | RPC + 异步 ingest + 指标 |
| `internal/service/knowledge_embedder.go` | 129 | embedder Wire 工厂（env + system_settings） |
| `internal/data/knowledge.go` | 307 | pgvector schema + CRUD + cosine SQL |
| `internal/data/pgvector/store.go` | 214 | **Memory 专用** `agent_memory_<dim>` 表（与 knowledge 完全分离） |
| `api/kratos/knowledge/v1/knowledge.proto` | 208 | 10 RPC |
| `internal/tools/knowledge/tool.go` | 146 | `knowledge_search` CallableTool |
| `web/src/pages/KnowledgePage.vue` | ~160 | 主页面 |
| `web/src/features/knowledge/useKnowledgePage.ts` | 301 | 状态、入库、搜索、WS |

### 4.3 端到端流程

```
上传 → IngestDocument(base64)
  → service: 同步 ExtractDocumentText（OCR/HTML/trpc readers/UTF-8）
  → service: 创建 document (status=processing)
  → safego.Go (context.Background!):
       ├─ SplitWithStrategy（重新读 chunker 工厂；service.chunker 字段未被使用）
       ├─ EmbedBatch（无 rate-limit / 无 retry / 无 timeout pin）
       ├─ BuildIndexedChunks → InsertChunks (TX)
       ├─ UpdateCollectionCounts(+1, +chunks)
       └─ publishKnowledgeIngest → WS notify

搜索 → knowledge_search 工具 或 RPC Search
  → Retriever.Search
       → Embed(query)              ← 与文档共用 Embed() 路径
       → repo.SearchChunks         ← cosine, IVFFlat lists=100
       → [optional rerank, overfetch 3×topK or 20, cap 50]
       → chunk JSON summary 返回
```

### 4.4 关键问题

| ID | 优先级 | 问题 | 位置 |
|----|--------|------|------|
| KB-01 | **P0** | **前端用 `FileReader.readAsText` 处理 PDF/DOCX**，二进制损坏，accept 仅 `.txt,.md,.json,.csv` | `web/src/features/knowledge/useKnowledgePage.ts:147-155`、`KnowledgeIngestDialog.vue:20` |
| KB-02 | P0 | 无上传大小 / 解码 / MIME magic 校验，base64 炸弹无防护 | `internal/service/knowledge.go` Ingest 入口 |
| KB-03 | P0 | ~~嵌入维度无强校验~~ | ✅ Round 2：InsertChunks 事务前校验维度，不匹配返回 kerrors.BadRequest |
| KB-04 | P0 | ~~DeleteDocument 不更新 collection 计数~~，UI 数字飘移 | ✅ `GetDocument` → `ChunkCount` + `UpdateCollectionCounts(-1, -n)` 已落 |
| KB-05 | P1 | `CreateCollection.embedding_model` 仅做名称记录，不绑定也不校验当前 embedder 配置 | `biz/knowledge/knowledge.go:128-129` |
| KB-06 | P1 | **Memory 与 Knowledge 共用同一 `Embedder` 实例**（Wire `wire.Bind`）→ 改 Knowledge 影响 L2/L3 索引 | `internal/service/wire_providers.go`、`knowledge_embedder.go` |
| KB-07 | P1 | Team Runner **未注入 KnowledgeBases**，Team agent 无作用域限制 | `internal/runtime/runner_team_trpc.go:392-393` |
| KB-08 | P1 | ~~纯向量检索，无 keyword / hybrid~~ | ✅ BM25 双路径搜索已落地（tsvector + pg_trgm） |
| KB-09 | P1 | OCR `tesseract/docling` env **仍返回 stub** | `internal/knowledge/ocr.go:32-34` |
| KB-10 | P1 | Gemini ingest 与 query 共用 `RETRIEVAL_DOCUMENT`，应分 task type | `internal/knowledge/embedder.go:236` |
| KB-11 | P2 | HTTP embedder 用 `http.DefaultClient`，无 timeout 配置 | `embedder.go:173, 341` |
| KB-12 | P2 | ~~rerank 后 `chunk_index` 类型断言只接 `int`，框架常返 `float64` 导致 -1~~ | ✅ Round 2：改为 `.(float64)` + `int(v)` |
| KB-13 | P2 | trpc chunk index 用循环 `i` 而非 document metadata，re-ingest 错位 | `chunk_strategy.go:79-84` |
| KB-14 | P2 | `ingest.go:67` chunk ID 用循环 `i` 而非 `ChunkIndex`，与上一条配合放大风险 | `ingest.go:67` |
| KB-15 | P2 | ~~异步 ingest 用 `context.Background()`，丢 trace/cancel~~ | ✅ Round 2：传递请求 ctx 到 safego.Go |
| KB-16 | P2 | ~~`KnowledgeService.chunker` 字段注入但未使用（dead code）~~ | ✅ Round 2：字段、构造函数参数、Wire provider 全部清理 |
| KB-17 | P2 | 无 `ListChunks` / `ReindexDocument` / `UpdateDocument` RPC，运维与调试不便 | `knowledge.proto` |
| KB-18 | P2 | ~~`MinScore` 直接 fmt.Sprintf 入 SQL，应参数化~~ | ✅ Round 2：提取 `hasMinScore` 布尔变量，SQL 占位符逻辑简化 |
| KB-19 | P2 | `knowledge_search` 工具不暴露 `filter_json` / `use_rerank` | `tools/knowledge/tool.go:125-130` |
| KB-20 | P2 | IVFFlat `lists=100` 写死，大数据集 recall 差，未提供 HNSW 选项 | `data/knowledge.go:67-68` |

### 4.5 业务逻辑层优化设计

**P0-1 — 前端二进制入库**

```ts
async function readFileAsBase64(file: File): Promise<string> {
  const buf = await file.arrayBuffer();
  const bytes = new Uint8Array(buf);
  let bin = '';
  const chunk = 0x8000;
  for (let i = 0; i < bytes.length; i += chunk) {
    bin += String.fromCharCode(...bytes.subarray(i, i + chunk));
  }
  return btoa(bin);
}
// accept = ".txt,.md,.json,.csv,.html,.pdf,.docx,.png,.jpg,.jpeg"
// mime_type = file.type || mimeFromExt(file.name)
```

**P0-2 — 上传守卫**

```go
const MaxIngestBytesDefault = 32 << 20 // 32MB

raw, err := base64.StdEncoding.DecodeString(req.ContentBase64)
if err != nil { return BadRequest("invalid base64") }
if int64(len(raw)) > config.MaxIngestBytes() {
    return BadRequest("file too large")
}
// MIME magic
mime := http.DetectContentType(raw[:min(512, len(raw))])
if !allowedMIME(mime) { return BadRequest("unsupported content type") }
```

**P0-3 — 维度校验前移**

```go
// CreateCollection
dim := embedder.Dim()
if req.EmbeddingModel != "" && req.EmbeddingModel != embedder.ModelName() {
    return BadRequest("embedding model mismatch with current embedder")
}
col.Dim = dim

// InsertChunks 前 batch 维度校验
for _, c := range chunks {
    if len(c.Embedding) != col.Dim {
        return fmt.Errorf("embedding dim mismatch: chunk=%d col=%d", len(c.Embedding), col.Dim)
    }
}
```

**P0-4 — DeleteDocument 计数修复**

```go
func (uc *Usecase) DeleteDocument(ctx context.Context, id string) error {
    doc, err := uc.repo.GetDocument(ctx, id)
    if err != nil { return err }
    if err := uc.repo.DeleteDocument(ctx, id); err != nil { return err }
    return uc.repo.UpdateCollectionCounts(ctx, doc.CollectionID, -1, -doc.ChunkCount)
}
```

**P1-1 — Hybrid 检索**：在 `knowledge_chunks` 增 `content_tsv tsvector GENERATED ALWAYS AS (to_tsvector('simple', text)) STORED`，加 GIN 索引；Retriever 加 keyword 路径，`score = α * cosine + (1-α) * ts_rank`。

**P1-2 — Memory / Knowledge Embedder 解耦**：Wire 区分 `MemoryEmbedder` / `KnowledgeEmbedder`，各自 env 前缀 `KRATOS_MEMORY_EMBED_*` / `KRATOS_KNOWLEDGE_EMBED_*`；前端 system settings 两组独立配置。

**P1-3 — Team Runner 注入 KnowledgeBases**：复用 Chat 的 `WithKnowledgeCollections(ctx, input.Options.KnowledgeBases)`，在 `runner_team_trpc.go` 构 RunContext 时一并加。

---

## 5. 制品管理 / Artifact

### 5.1 模块定位

Artifact 是 **会话维度 + 本地文件系统 MVP**：

- **CRUD + 多版本**：每个文件 `.bin` + `.json` 配对
- **10 MB 上限**（前端 + biz + service 三处校验）
- **HMAC-SHA256 签名下载 URL**（默认 15 分钟，最大 24 小时）
- **Chat / Team 多模态附件**：artifact id → LLM ContentParts
- **TurnCollector**：Agent 产物嵌入 `options_json.attachments`
- **Prometheus** 字节计数 + 存储量 gauge

`pkg/trpc-agent-go/artifact/cos/` 已有 COS 实现，**未在 Aranea 接线**；Channel 入站消息中的媒体 **不会自动转 artifact**。

### 5.2 文件清单

| 路径 | 大小 | 角色 |
|------|------|------|
| `internal/artifact/sign.go` | 1.9 KB | HMAC-SHA256 token mint/verify |
| `internal/artifact/trpc/service.go` | ~3.5 KB | biz.Usecase → trpc artifact.Service 适配（filename-keyed） |
| `internal/biz/artifact/artifact.go` | ~4 KB | 领域类型、Repo 接口、Usecase |
| `internal/biz/artifact/limits.go` | ~0.5 KB | `MaxUploadBytes = 10 << 20` |
| `internal/biz/artifact/turn_collector.go` | ~1.5 KB | per-turn ref 累积 via context |
| `internal/biz/artifact/attachments_resolve.go` | ~1.5 KB | 附件 ID 校验 + session 绑定 |
| `internal/biz/artifact/options_merge.go` | ~1 KB | refs → `options_json.attachments` |
| `internal/service/artifact.go` | 8.9 KB | RPC + `ServeSignedDownload` HTTP handler |
| `internal/data/artifactfs/repo.go` | ~12 KB | **唯一生产 Repo** |
| `api/kratos/artifact/v1/artifact.proto` | ~4 KB | 7 RPC |
| `web/src/pages/ArtifactsPage.vue` | ~5 KB | 管理页 |
| `web/src/features/artifact/useArtifactsPage.ts` | ~7 KB | 分页、上传、删除 |
| `internal/agent/attachments.go` | ~2 KB | 多模态 message builder |

### 5.3 存储布局

```
ARTIFACT_STORAGE_DIR/  (默认 data/artifacts/)
├── <session_id>/
│   ├── a1b2c3d4e5f6-v0.bin   ← 原始字节
│   ├── a1b2c3d4e5f6-v0.json  ← 元数据 sidecar
│   ├── f7e8d9c0b1a2-v1.bin   ← 同名新版本（新 id）
│   └── f7e8d9c0b1a2-v1.json
└── <another_session_id>/
    └── ...

sidecar JSON:
  { id, session_id, name, mime_type, size, sha256,
    storage_kind: "local", storage_uri: <绝对路径>,
    version, created_at }
```

**版本 / id 双标识**：

- API / Chat 用随机 `id`（12 字节 hex）
- Agent 运行时（tRPC）用 `session_id + filename`，每新版本一个新 id
- 这种"逻辑版本（filename）"与"物理 id"的双重模型是后续 bug 的根源（见 ART-DEL-01）

### 5.4 关键问题

| ID | 优先级 | 问题 | 位置 |
|----|--------|------|------|
| ART-01 | **P0** | **`session_id` 无校验**：`../../outside` 类值可越权写文件系统 | `internal/data/artifactfs/repo.go:83-85` |
| ART-02 | **P0** | 生产环境若 env 全空，签名 key 回退硬编码 `aranea-artifact-dev-key` | `internal/artifact/sign.go:17-26` |
| ART-03 | **P0** | API 响应里 `storage_uri` 直返**绝对文件系统路径**，泄露主机布局 | `internal/data/artifactfs/repo.go:111-122` |
| ART-04 | **P0** | `DeleteArtifact(id)` **只删该 id 对应那一版**；proto 注释"删除所有版本"未兑现 | `internal/service/artifact.go:131-141` vs `internal/data/artifactfs/repo.go:197-226` |
| ART-05 | P1 | Get/Delete/Sign **无 session 归属校验**，admin token 可跨会话访问任意 artifact | `internal/service/artifact.go` 全员 |
| ART-06 | P1 | 全表 `sync.Mutex` 串行化所有 FS 操作，并发上传瓶颈 | `internal/data/artifactfs/repo.go:67-68` |
| ART-07 | P1 | 无 session 删除回调 / TTL，文件孤儿化堆积 | `repo.go` + `internal/biz/session` |
| ART-08 | P1 | 全文件 `io.ReadAll` 读入内存（含多模态 `BuildUserMessageFromArtifacts` 二次加载） | `internal/agent/attachments.go:34-37` |
| ART-09 | P2 | 无 MIME magic / 内容嗅探，客户端声明即生效 | `internal/service/artifact.go` Upload |
| ART-10 | P2 | 跨 session list 全树扫描，O(全部 artifact) | `internal/data/artifactfs/repo.go:161-164` |
| ART-11 | P2 | `ArtifactUploadBytesTotal` / `Download...` 无 session/agent label | `internal/metrics/vars.go:101-117` |
| ART-12 | P2 | 无 audit 事件（与 monitor audit 流不一致） | — |
| ART-13 | P2 | 测试空：`ServeSignedDownload`、path traversal、delete-all-versions 全缺 | — |
| ART-14 | P3 | Channel 入站媒体未自动转 artifact，多模态渠道聊天无法落地 | `internal/service/channel_ingress*.go` |

### 5.5 业务逻辑层优化设计

**P0 — 路径安全**

```go
var sessionIDRE = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func (r *FSArtifactRepo) sessionDir(sessionID string) (string, error) {
    if !sessionIDRE.MatchString(sessionID) {
        return "", fmt.Errorf("invalid session id: %q", sessionID)
    }
    dir := filepath.Join(r.root, sessionID)
    abs, err := filepath.Abs(dir)
    if err != nil { return "", err }
    rootAbs, _ := filepath.Abs(r.root)
    if !strings.HasPrefix(abs, rootAbs+string(filepath.Separator)) && abs != rootAbs {
        return "", fmt.Errorf("session dir escapes root")
    }
    return dir, nil
}
```

**P0 — 删除按"逻辑文件"全版本**

```go
func (uc *ArtifactUsecase) Delete(ctx context.Context, id string) error {
    meta, err := uc.repo.Load(ctx, id, 0)
    if err != nil { return err }
    versions, err := uc.repo.ListBySessionAndName(ctx, meta.SessionID, meta.Name)
    if err != nil { return err }
    for _, v := range versions {
        if err := uc.repo.Delete(ctx, v.ID); err != nil { return err }
    }
    return nil
}
// 同步更新 proto 注释；如需保留"仅删一版"语义，新增 DeleteArtifactVersion RPC
```

**P0 — 签名 key 生产 fail-closed**

```go
func SignKey() ([]byte, error) {
    if k := strings.TrimSpace(os.Getenv("KRATOS_ARTIFACT_SIGN_KEY")); k != "" { return []byte(k), nil }
    if k := strings.TrimSpace(os.Getenv("KRATOS_AUTH_SECRET")); k != "" { return []byte(k), nil }
    if isDevMode() { return []byte("aranea-artifact-dev-key"), nil }
    return nil, errors.New("artifact sign key not configured in production")
}
// SignDownloadUrl / ServeSignedDownload 启动期 readiness 检查；缺 key 拒服务而非用 dev 默认
```

**P1 — `BlobStore` 抽象**

```go
type BlobStore interface {
    Put(ctx, key string, r io.Reader, size int64, meta map[string]string) error
    Get(ctx, key string) (io.ReadCloser, *BlobMeta, error)
    Delete(ctx, key string) error
}

// FS 实现保留作为 dev；接入 COS/S3：
//   key = artifacts/<tenant>/<session>/<file_id>/v<n>
// Repo 拆分：
//   - MetaRepo (DB/SQL)  ← 跨 session 索引、ACL、列表
//   - BlobStore          ← 二进制
```

**P1 — 访问控制**

```go
// internal/biz/artifact 加端口
type ArtifactACL interface {
    Authorize(ctx, principal, artifactSessionID string, action string) error
}
// service 层在 Get/Sign/Delete/Preview 前查 ACL；admin 角色 bypass 显式标注
```

**P2 — Channel 入站媒体 → artifact 管线**：`channel_ingress_*` 收到媒体附件 → 下载至临时 buffer → `ArtifactUsecase.Save` → 把 artifact_id 写入 turn input options，让多模态渠道聊天打通。

---

## 6. 评估管理 / Evaluation

### 6.1 模块定位

评估管理是 **三层系统**：

| 层 | 角色 |
|----|------|
| **`internal/evaluation/`** | 编排（Runner）、打分（metrics / framework_metrics / llm_judge）、tRPC `FrameworkBridge`、模拟器、after-turn 触发 |
| **`internal/biz/evaluation/`** | CRUD 用例、scores JSON helpers、Repo |
| **`internal/data/evaluation.go`** | 表 + （部分）级联删除 |
| **`internal/service/`** | Kratos RPC + Wire（`EvalTurnGateway` → ChatOrchestrator） |
| **`web/src/features/evaluation/`** | REST + Pinia + 页面 |

**主路径**：基于框架的 `AgentEvaluator`，内存 eval set / metrics / results，桥接到 `chatRunnerAdapter` → `RunEvalAgentTurn`。**Legacy** 顺序执行路径在框架/agent 未装配时回退。

**Cron**：`internal/cronrunner/` 无 evaluation 任务——**定时评估未实现**。

### 6.2 文件清单（核心）

| 文件 | 角色 |
|------|------|
| `internal/evaluation/runner.go` | `Runner.Start` 异步、`execute` 主流程 |
| `internal/evaluation/runner_legacy.go` | 顺序回退 |
| `internal/evaluation/framework.go` | `FrameworkBridge.Execute` |
| `internal/evaluation/framework_metrics.go` | 注册 exact/contains/json/xml/rouge/tool |
| `internal/evaluation/llm_judge.go` | LLM-as-judge（30s timeout） |
| `internal/evaluation/llm_simulator.go` / `scripted_simulator.go` | 用户模拟器 |
| `internal/evaluation/eval_llm_resolve.go` | judge/sim 模型解析（env → settings → catalog） |
| `internal/evaluation/after_turn.go` | after-turn 自动触发 |
| `internal/evaluation/case_metadata.go` | metadata_json 解析 |
| `internal/evaluation/evalset_adapter.go` | biz cases → trpcevalset.EvalSet |
| `internal/evaluation/scores.go` / `pass_metrics.go` / `metrics.go` | 打分、聚合 |
| `internal/biz/evaluation/evaluation.go` | Usecase / Repo / 类型 |
| `internal/service/evaluation.go` | 14 RPC |
| `internal/service/eval_turn_gateway.go` | `ChatService.RunEvalAgentTurn` |
| `internal/data/evaluation.go` | schema + repo（~520 行） |
| `api/kratos/evaluation/v1/evaluation.proto` | proto |
| `web/src/pages/EvaluationPage.vue` | 主页 |

### 6.3 状态机

```
pending ──Start──▶ running ──success──▶ completed
                       │
                       ├──error──▶ failed
                       │
                       └──cancel──▶ (尚未实现) cancelled
```

Prometheus `aranea_eval_runs_total{status=started|completed|error}`——**没有 `cancelled` 标签**。

### 6.4 关键问题

| ID | 优先级 | 问题 | 位置 |
|----|--------|------|------|
| EV-01 | **P0** | **`DeleteDataset` 不级联删 runs/results**，孤儿数据无清理 | `internal/data/evaluation.go:159-171` |
| EV-02 | **P0** | **UI 无案例上传入口**，store 的 `importCases` 未被调用；数据集创建后 `case_count=0` | `EvaluationCreateDialog.vue`、`web/src/stores/evaluation/index.ts` |
| EV-03 | P1 | `RunEvalAgentTurn` **每个用例一个新 session**，DB/会话快速膨胀 | `internal/service/chat_orchestrator_turn.go:238-251` |
| EV-04 | P1 | **Judge 失败静默吞**，不计分也不计入分母——平均分虚高 | `internal/evaluation/framework.go:157-164`、`metrics.go:68-74` |
| EV-05 | P1 | `ListDatasets` 永远传空 workspace；多租户失效 | `internal/service/evaluation.go:56` |
| EV-06 | P1 | `newEvalResultID` 基于时间戳，高频插入有碰撞风险 | `internal/evaluation/runner.go:129-132` |
| EV-07 | P1 | after-turn 频控用进程内 map，多副本不一致 | `internal/evaluation/after_turn.go:65-76` |
| EV-08 | P1 | **没有数据集快照**：run 评估的是 **当前** cases；历史 run 无法复现 | `internal/biz/evaluation/evaluation.go` Create/Run |
| EV-09 | P2 | 无 cancel run API；proto `completed` vs docs `succeeded` 词汇不一致 | `evaluation.proto:38` |
| EV-10 | P2 | `CompareEvalRuns` baseline = "DB 结果集第一条"，非 `created_at` 最早 | `internal/biz/evaluation/evaluation.go:357-376` |
| EV-11 | P2 | legacy `tool_call_accuracy` 与框架 `tool_trajectory` 语义不同，结果不可比 | `metrics.go:128-146` vs `framework_metrics.go:76-79` |
| EV-12 | P2 | 前端 `loadAgentTrend` 传 `metric` 参数，API 实际忽略 | `useEvaluationPage.ts:183-187` |
| EV-13 | P2 | 导出 CSV 缺 `scores_json` 扩展指标 | `exportRunResults.ts:22-33` |
| EV-14 | P2 | `runner.Start` 用 `context.Background()`，父 cancel 被无视 | `internal/evaluation/runner.go:51` |
| EV-15 | P3 | `ProvideEvaluationRunner` 若返 nil，run 永远 `pending`，无降级提示 | `evaluation.go:110-116` |
| EV-16 | P3 | `UploadCases` 静默跳过空 input；客户端不知道丢了多少 | `internal/biz/evaluation/evaluation.go:232-234` |

### 6.5 业务逻辑层优化设计

**P0-1 — Dataset 级联**

```go
func (r *evalRepo) DeleteDataset(ctx context.Context, id string) error {
    return r.withTx(ctx, func(tx *sql.Tx) error {
        if _, err := tx.ExecContext(ctx,
            `DELETE FROM eval_case_results WHERE run_id IN (SELECT id FROM eval_runs WHERE dataset_id = ?)`, id); err != nil { return err }
        if _, err := tx.ExecContext(ctx, `DELETE FROM eval_runs WHERE dataset_id = ?`, id); err != nil { return err }
        if _, err := tx.ExecContext(ctx, `DELETE FROM eval_cases WHERE dataset_id = ?`, id); err != nil { return err }
        if _, err := tx.ExecContext(ctx, `DELETE FROM eval_datasets WHERE id = ?`, id); err != nil { return err }
        return nil
    })
}
// 单测：建 dataset → 2 cases → 1 run → 3 results → DeleteDataset → 断言所有相关行为 0
```

**P0-2 — 案例上传 UI + 数据集详情页**

```
EvaluationPage
  ├─ DatasetCard (新增 "导入" 按钮)
  └─ DatasetDetailDrawer
       ├─ 案例列表 + 分页
       ├─ "上传 JSON/CSV" → UploadCases RPC
       └─ "查看" / "删除" (后端补 ListCases / DeleteCase RPC)
```

**P0-3 — Run 数据集快照**

```sql
CREATE TABLE eval_run_cases (
  run_id TEXT NOT NULL,
  case_id TEXT NOT NULL,
  input TEXT NOT NULL,
  expected_output TEXT,
  metadata_json TEXT,
  case_revision INT NOT NULL,
  PRIMARY KEY (run_id, case_id)
);
-- CreateRun 时 INSERT ... SELECT 复制当前 cases 到 eval_run_cases
-- Runner 改读 eval_run_cases，而非 eval_cases
-- 历史 run 复现性 ✓
```

**P1-1 — 共享评估 session 池**

```go
// 同一 run 内复用一个 eval session
sess, _ := s.sessions.Create(ctx, SessionOpts{Kind: "eval", AgentID: agentID})
defer s.sessions.AsyncSoftDelete(sess.ID)
for _, c := range cases {
    s.chat.RunEvalAgentTurn(ctx, sess.ID, agentID, c.Input)
}
// soft-delete 由 cron purge owner_type='eval' AND created_at < now - 7d
```

**P1-2 — Judge 失败可见性**

```go
type EvalCaseResult struct {
    ...
    JudgeError string `json:"judge_error,omitempty"`
    // 分母策略：把 judge_error 作为 0 分计入聚合；run.scores_json 记录 judge_call_failures
}
// UI 在 run 详情面板红字标注 "judge 失败率 X%"
```

**P1-3 — Cluster-safe after-turn 频控**

```go
// 用 SQLite/PG 单行 row lock 或 Redis SETNX：
//   key = "eval:throttle:" + agentID
//   if SETNX key now ex 300 → 允许；else 跳过
```

**P1-4 — Cancel Run**

```go
type Runner struct {
    ...
    inflight sync.Map // run_id → context.CancelFunc
}

func (r *Runner) Cancel(ctx context.Context, runID string) error {
    v, ok := r.inflight.Load(runID)
    if !ok { return nil }
    v.(context.CancelFunc)()
    return r.repo.UpdateRunStatus(ctx, runID, "cancelled", "user cancelled")
}
```

**P2 — Cron 质量门禁**

```yaml
# system_settings 配置
eval_cron:
  - agent_id: agent-001
    dataset_id: ds-baseline
    schedule: "0 2 * * *"        # 每天 02:00
    webhook_on_regression:
      url: https://qa.example.com/eval-alert
      threshold:
        exact_match: 0.85        # 低于即告警
```

---

## 7. 跨模块共性与统一改造

### 7.1 出站 HTTP / SSRF 守卫统一

**问题面**：modelcatalog 已做 urlguard，但 inspect / health / preflight / hook notify / gateway webhook 各自实现或缺失。

**建议设计**

```go
// pkg/outboundguard/guard.go
type PolicyName string

const (
    PolicyCatalogSource   PolicyName = "catalog-source"   // 仅 models.dev / 允许列表
    PolicyInspectProvider PolicyName = "inspect-provider"  // 拒私网 + HTTPS only
    PolicyHookWebhook     PolicyName = "hook-webhook"      // 同上 + 拒 .internal
    PolicyHealthCheck     PolicyName = "health-check"      // 同 inspect
)

type Guard interface {
    Validate(ctx context.Context, rawURL string, pol PolicyName) (net.IP, error)
    DialContext(ctx context.Context, network, addr string) (net.Conn, error) // pin first-resolve IP，反 DNS rebind
}

// 所有出站 HTTP 客户端通过 guard.DialContext，避免 TOCTOU
```

落地点：

- `internal/llminspect`、`internal/biz/llm_provider_model.RunHealthChecks`、`internal/provider/trpc_llm.preflight`、`internal/plugin/trpc/hook_notify.deliverHookWebhook`、`internal/biz/webhook_dispatcher.postOne`、`internal/modelcatalog/{fetch, logos}`

### 7.2 持久化投递 + 重试 worker 模式

**问题面**：channel delivery 有 worker，hook delivery 无、gateway webhook 完全 fire-and-forget——三种成熟度并存。

**建议设计**：抽 `pkg/outboundqueue`，约束所有出站投递走：

```go
type OutboundQueue interface {
    Enqueue(ctx context.Context, item OutboundItem) (id string, err error)
    // 后台 worker 由 cronrunner 启动，ClaimPending → POST → MarkSucceeded/Retry/DeadLetter
}

type OutboundItem struct {
    Kind          string // hook | gateway | channel
    URL           string
    Method        string
    Headers       map[string]string
    Body          []byte
    Secret        []byte // 用于签名
    MaxAttempts   int
    IdempotencyKey string
    Metadata      map[string]string // session_id / run_id / hook_id 等
}
```

每种 kind 各一张物理表（保留 `hook_deliveries` / `gateway_deliveries` / `channel_deliveries`），但 worker 走统一管道，配合统一 `outboundwebhook.Sign` 头格式。

### 7.3 半结构化 JSON 字段治理

**问题面**：`config_json` / `options_json` / `metadata_json` / `scores_json` / `metadata_json`（hook） / `config_json`（hook action）分散，没有版本号、没有 schema 校验。

**建议设计**

```go
// pkg/structjson/version.go
type Versioned struct {
    Schema string `json:"$schema,omitempty"`
    Version int   `json:"$version,omitempty"`
}

// 各模块写入：
type ArtifactOptions struct {
    Versioned
    Attachments []ArtifactRef `json:"attachments,omitempty"`
}

// internal/biz/migrations/json/* 注册迁移：
//   options.attachments[]: v0 (无 mime) → v1 (含 mime+size)
//   hook.config.action.retry: v0 (扁平) → v1 (结构化 BackoffSpec)
```

读路径统一走 `Unmarshal + Migrate`，写路径强制带 `Version`。

### 7.4 后端聚合并行 + 缓存

**问题面**：概览、quota dashboard、catalog status、artifact list、eval run dashboard 都是顺序串行。

**建议**：每个聚合 RPC 内部：

1. `errgroup.WithContext` 并行子查询
2. 简单 `groupcache` / `singleflight` 防雪崩
3. 60s LRU（按 user/agent/range key）
4. 响应头加 `X-Cache-Age` / `X-Cache-Hit`

### 7.5 业务测试金字塔

**当前**：单测多在 biz；data SQL 集成测最薄弱；service RPC 测覆盖 < 20%。

**目标**

```
e2e (testcontainers)           5%   ← 待补
service handler integration   20%   ← Hook Reload / Knowledge Ingest / Eval Run / Artifact Sign
biz usecase                   30%   ← 已较齐
data SQL                      25%   ← 待补
engine pure                   20%   ← 已较齐
```

行动：每模块至少补 1 个 `*_integration_test.go`，用 testcontainers PG / SQLite memory DB。

---

## 8. 优先级综合视图

### 8.1 P0 列表（按业务影响排序）

| # | 模块 | 项 | 一句话 |
|---|------|----|-------|
| 1 | Hook | HK-01 / HK-03 | 投递持久化 + worker（hook + gateway） |
| 2 | Hook | HK-02 | 统一 Webhook HMAC 签名（hook + gateway） |
| 3 | Artifact | ART-01 / ART-02 / ART-03 | path traversal / 签名 key / storage_uri 泄露 |
| 4 | Artifact | ART-04 | DeleteArtifact 真正删全版本 |
| 5 | Model | MD-01 | inspect / health / preflight SSRF 守卫 |
| 6 | Knowledge | KB-01 / KB-02 / KB-03 / KB-04 | 二进制入库 / 大小守卫 / 维度校验 / 计数修复 |
| 7 | Eval | EV-01 / EV-02 / EV-08 | dataset 级联删 / 案例上传 UI / run 快照 |
| 8 | Overview | OV-01 / OV-02 / OV-03 | Overview 并行 + rollup + 前端错误 UX |

### 8.2 P1 / P2 摘要

- **P1**：Hook Resolver 缓存、Gateway Webhook secret 泄露、Knowledge Memory 解耦、Team KnowledgeBases 注入、模型迁移 dry-run、Health worker pool、价格优先级合约、Eval session 复用、Judge 失败可见、QuotaDashboard 批量化、Hourly filter 对齐
- **P2**：URL 同步 filters、跨模块出站 guard 抽包、半结构化 JSON 版本化、聚合查询缓存、测试金字塔补齐、Channel 媒体 → Artifact 管线、Cron 评估、HNSW / 索引调优

---

## 9. 与历史 Review 的差距

| 历史 Review | 状态 | 本次补充 |
|------------|------|---------|
| [37-knowledge-review.md](./37-knowledge-review.md) | 部分仍未关闭 | 增加：前端二进制 broken、维度校验、Memory/Knowledge 耦合、Team 集成缺口 |
| [33-evaluation-review.md](./33-evaluation-review.md)（82/100） | UserSim 测、cron 仍空 | 增加：dataset 级联缺、案例 UI 缺、session 池、judge 静默 |
| [27-artifact-review.md](./27-artifact-review.md)（89/100） | 大部分修复 | 增加：path traversal、签名 key、删除语义、storage_uri 泄露、Channel 集成缺 |
| [22-28-plugin-callback-review.md](./22-28-plugin-callback-review.md)（81/100） | Phase 1–4 已闭环 | 增加：投递仍非 cron/worker、Gateway / Hook 双轨 HTTP 未收敛、签名缺失 |
| [09-provider-review.md](./09-provider-review.md) | 较旧 | 增加：SSRF、价格漂移、自动迁移耦合、`internal/provider → biz` 上行依赖 |
| [18-monitor-review.md](./18-monitor-review.md)（runner-metrics） | 与本报 Overview 关联 | 增加：`avg_duration_ms` 前端未映射 |

---

## 10. 建议的落地节奏

**Wave 1（2 周）**：

1. `pkg/outboundguard` + `pkg/outboundwebhook` 抽出
2. Artifact path / sign key / 删除语义修复
3. Knowledge 维度校验 + DeleteDocument 计数 + 前端二进制路径
4. Eval dataset 级联 + 数据集快照
5. Overview 并行化 + rollup 读路径

**Wave 2（2–3 周）**：

6. Hook delivery worker（hook + gateway 共用 `outboundqueue`）
7. Webhook HMAC 统一
8. Inspect / health / preflight SSRF 守卫接入
9. 前端错误 UX 统一（overview / knowledge / eval / artifact / hook）
10. Wire 拆分 Memory / Knowledge embedder

**Wave 3（持续）**：

11. 半结构化 JSON 版本化 + migration helper
12. 集成测补齐（service / data / e2e）
13. Cron 评估、Channel 媒体 → Artifact 管线、HNSW 索引

---

## 11. 结论

| 模块 | 一句话总结 |
|------|----------|
| **概览** | 分层干净的用量大盘，**后端聚合性能 + 前端错误兜底** 是主要短板 |
| **模型管理** | 双子系统抽象到位，**SSRF + 价格漂移 + 自动迁移** 是 P0 |
| **Hook 回调** | 规则建模与 SSRF 已落地，**投递持久化 + 统一签名** 仍是生产级缺口 |
| **知识库** | 完整 RAG 骨架，**维度强校验 + Memory 耦合** 是生产级问题；KB-04/08 已修；tools/knowledge fmt.Errorf→kerrors 已修 |
| **制品** | 干净 MVP，**路径安全 + 签名 key + 删除语义 + ACL** 必须先行 |
| **评估** | 框架桥接优雅，**dataset 级联 + 案例 UI + 快照** 决定能否真正闭环 |

整体上 Aranea 6 模块的 **分层一致性** 已是国内同类项目的优秀水准（biz 不 import trpc、service 是传输桥），但 **出站可靠性、安全防线、半结构化字段治理、性能聚合、测试覆盖** 五条线仍需系统化补齐——绝大部分问题都不是"局部坏味道"，而是早期 MVP 抽象未落地。建议按 Wave 1–3 节奏推进，避免一次性大爆改。

---

## 12. Knowledge 模块 Round 1 审查报告（2026-05-29）

### 变更文件

| 文件 | 变更 | 对应 ID |
|------|------|---------|
| `internal/tools/knowledge/tool.go` | 13 处 `fmt.Errorf` → `kerrors.BadRequest`/`kerrors.InternalServer` | KB-ERR-01 |
| `internal/tools/knowledge/tool_test.go` | 新增 5 个 reflect 工具测试 | KB-TST-01 |
| `internal/knowledge/retriever_test.go` | 新增 2 个 retriever 边界测试 | KB-TST-02 |

### aranea-review 审查结果

| 维度 | 🔴 阻断 | 🟡 建议 | 🟢 提示 | 合计 |
|------|---------|---------|---------|------|
| **后端 — 架构合规** | 0 | 0 | 0 | 0 |
| **后端 — 分层合规** | 0 | 0 | 0 | 0 |
| **后端 — OOP** | 0 | 0 | 0 | 0 |
| **后端 — Agent 运行时** | 0 | 0 | 0 | 0 |
| **后端 — 并发安全** | 0 | 0 | 0 | 0 |
| **后端 — 错误处理** | 0 | 0 | 0 | 0 |
| **后端 — 依赖注入** | 0 | 0 | 0 | 0 |
| **构建与回归** | 0 | 0 | 0 | 0 |

**审查结论**：0 阻断、0 建议。Knowledge 模块 Round 1 所有变更通过 aranea-review 全维度检查。

### 亮点

- **红线合规**：13 处 `fmt.Errorf` 全部替换为 `kerrors`，输入校验错误用 `BadRequest`（400），运行时搜索错误用 `InternalServer`（500）
- **测试覆盖**：新增 7 个测试覆盖 reflect 工具验证路径和 retriever 边界条件
- **KnowledgeRepo ISP 合规**：CollectionRepo(5) + DocumentRepo(5) + ChunkRepo(3) 子接口拆分已完成

### Knowledge 模块剩余工作

| 优先级 | ID | 项目 | 说明 |
|--------|-----|------|------|
| P0 | KB-02 | 上传大小/解码/MIME magic 校验 | base64 炸弹无防护 |
| P0 | KB-03 | 嵌入维度强校验 | dim 不一致整批 TX rollback |
| P1 | KB-05 | CreateCollection embedding_model 绑定校验 | 仅做名称记录 |
| P1 | KB-06 | Memory 与 Knowledge Embedder 解耦 | 共用 Wire Bind |
| P1 | KB-07 | Team Runner 注入 KnowledgeBases | Team agent 无作用域限制 |
| P1 | KB-09 | OCR tesseract/docling 实现 | 仍返回 stub |
| P1 | KB-10 | Gemini ingest/query 分 task type | 共用 RETRIEVAL_DOCUMENT |
| P2 | KB-11 | HTTP embedder timeout 配置 | 用 http.DefaultClient |
| P2 | KB-12 | rerank chunk_index 类型断言 | 框架返 float64 导致 -1 |
| P2 | KB-13 | chunk index 用 metadata 而非循环 i | re-ingest 错位 |
| P2 | KB-14 | ingest chunk ID 用 ChunkIndex 而非循环 i | 与 KB-13 配合 |
| P2 | KB-15 | 异步 ingest context 传递 | 用 context.Background() |
| P2 | KB-16 | KnowledgeService.chunker 死代码 | 注入但未使用 |
| P2 | KB-17 | ListChunks/ReindexDocument/UpdateDocument RPC | 运维调试不便 |
| P2 | KB-18 | MinScore SQL 参数化 | fmt.Sprintf 入 SQL |
| P2 | KB-19 | knowledge_search 暴露 filter_json/use_rerank | 工具参数不全 |
| P2 | KB-20 | IVFFlat lists=100 写死 | 大数据集 recall 差 |
| P3 | KB-CODE | code_search 工具 | 开发文档标记待实现 |

---

## 13. Knowledge 模块 Round 2 审查报告（2026-05-29）— P0-P2 批量修复

### 变更文件

| 文件 | 变更 | 对应 ID |
|------|------|---------|
| `internal/data/knowledge.go` | InsertChunks 维度校验 + MinScore SQL 参数化 | KB-03 + KB-18 |
| `internal/knowledge/retriever.go` | chunk_index `.(int)` → `.(float64)` + `int(v)` | KB-12 |
| `internal/service/knowledge.go` | 异步 ingest 传递 ctx + chunker 死代码清理 | KB-15 + KB-16 |
| `internal/service/service.go` | 移除 `NewKnowledgeChunker` provider | KB-16 |
| `internal/service/wire_providers.go` | 移除 chunker ProviderSet 条目 | KB-16 |
| `internal/skill/importer/engine.go` | GetImportJob RLock/Lock 分离 + 中文消息统一 | SKILL-P2-04 + SKILL-P2-05 |

### aranea-review 审查结果

| 维度 | 🔴 阻断 | 🟡 建议 | 🟢 提示 | 合计 |
|------|---------|---------|---------|------|
| **后端 — 架构合规** | 0 | 0 | 0 | 0 |
| **后端 — 分层合规** | 0 | 0 | 0 | 0 |
| **后端 — OOP** | 0 | 0 | 0 | 0 |
| **后端 — Agent 运行时** | 0 | 0 | 0 | 0 |
| **后端 — 并发安全** | 0 | 0 | 0 | 0 |
| **后端 — 错误处理** | 0 | 0 | 0 | 0 |
| **后端 — 依赖注入** | 0 | 0 | 0 | 0 |
| **构建与回归** | 0 | 0 | 0 | 0 |

**审查结论**：0 阻断、0 建议。P0-P2 批量修复全部通过 aranea-review 全维度检查。

### 亮点

- **KB-03 (P0)**：维度校验在事务前执行，返回友好 `kerrors.BadRequest` 而非 PostgreSQL 不透明错误
- **KB-12 (P2)**：`chunk_index` 类型断言修复，解决 rerank 后 ChunkIndex 丢失的生产 Bug
- **KB-15 (P2)**：异步 ingest 传递请求 context，支持 trace/cancel 传播
- **KB-16 (P2)**：死代码 `chunker` 字段及 Wire provider 全链路清理
- **SKILL-P2-04**：`GetImportJob` 读写锁分离，降低并发争用
- **SKILL-P2-05**：4 处中文消息统一为英文

### 全局剩余工作总结（2026-05-29）

#### Knowledge 模块

| 优先级 | ID | 项目 | 状态 |
|--------|-----|------|------|
| ~~P0~~ | KB-02 | 上传大小/解码/MIME magic 校验 | ✅ Round 3 |
| ~~P0~~ | KB-03 | 嵌入维度强校验 | ✅ Round 2 |
| ~~P0~~ | KB-04 | DeleteDocument 计数修复 | ✅ 早期 |
| ~~P1~~ | KB-05 | CreateCollection embedding_model 绑定校验 | ✅ Round 3 |
| ~~P1~~ | KB-06 | Memory/Knowledge Embedder 解耦 | ✅ Round 4 |
| ~~P1~~ | KB-07 | Team Runner 注入 KnowledgeBases | ✅ Round 4 |
| ~~P1~~ | KB-08 | hybrid 搜索 | ✅ 早期 |
| ~~P1~~ | KB-10 | Gemini ingest/query 分 task type | ✅ Round 4 |
| P1 | KB-09 | OCR tesseract/docling 实现 | 📋 |
| ~~P2~~ | KB-11 | HTTP embedder timeout 配置 | ✅ Round 3 |
| ~~P2~~ | KB-12 | rerank chunk_index 类型断言 | ✅ Round 2 |
| ~~P2~~ | KB-13/14 | chunk index 用 metadata 而非循环 i | ✅ Round 3 |
| ~~P2~~ | KB-15 | 异步 ingest context 传递 | ✅ Round 2 |
| ~~P2~~ | KB-16 | chunker 死代码 | ✅ Round 2 |
| ~~P2~~ | KB-18 | MinScore SQL 参数化 | ✅ Round 2 |
| ~~P2~~ | KB-19 | knowledge_search 暴露 filter_json/use_rerank | ✅ Round 3 |
| ~~P2~~ | KB-20 | IVFFlat lists 参数化 | ✅ Round 4 |
| P2 | KB-17 | ListChunks/ReindexDocument/UpdateDocument RPC | 📋 |
| P3 | KB-CODE | code_search 工具 | 📋 |

#### Skill 模块

| 优先级 | ID | 项目 | 状态 |
|--------|-----|------|------|
| ~~P2~~ | SKILL-P2-03 | slugify("") 非唯一 | ✅ Round 4 |
| ~~P2~~ | SKILL-P2-04 | GetImportJob Lock→RLock | ✅ Round 2 |
| ~~P2~~ | SKILL-P2-05 | ApplyImport 中文消息统一 | ✅ Round 2 |
| ~~P2~~ | SKILL-P2-06 | watch.Runner 窄接口依赖 | ✅ Round 4 |
| P2 | SKILL-P2-01 | internal/tools/ ~84 处 fmt.Errorf | 📋 低优先级 |
| P2 | SKILL-P2-02 | SkillFileReader 6 方法 | 📋 可接受 |

#### 建议下一步优先级

1. **KB-09**（OCR 实现）— P1 功能项
2. **KB-17**（ListChunks/ReindexDocument/UpdateDocument RPC）— P2 运维便利性
3. **SKILL-P2-01**（internal/tools/ fmt.Errorf）— P2 低优先级批量替换

---

## 14. Knowledge 模块 Round 3 审查报告（2026-05-29）— P0-P2 批量修复

### 变更文件

| 文件 | 变更 | 对应 ID |
|------|------|---------|
| `internal/service/knowledge.go` | 新增上传大小限制（32MB）+ MIME magic 校验 + embedding_model 绑定校验 | KB-02 + KB-05 |
| `internal/service/knowledge_test.go` | 新增 4 个 MIME/大小校验测试 | KB-02 |
| `internal/knowledge/embedder.go` | embedHTTPClient timeout 改为环境变量 `KRATOS_KNOWLEDGE_EMBED_TIMEOUT_SEC` 可配 | KB-11 |
| `internal/knowledge/chunk_strategy.go` | `trpcDocsToChunks` 从 metadata 读取 `MetaChunkIndex` 而非循环 i | KB-13 |
| `internal/knowledge/ingest.go` | chunk ID 用 `ch.ChunkIndex` 而非循环 `i` | KB-14 |
| `internal/tools/knowledge/tool.go` | `searchInput` 新增 `filter_json` + `use_rerank` 字段 | KB-19 |

### aranea-review 审查结果

| 维度 | 🔴 阻断 | 🟡 建议 | 🟢 提示 | 合计 |
|------|---------|---------|---------|------|
| **后端 — 架构合规** | 0 | 0 | 0 | 0 |
| **后端 — 分层合规** | 0 | 0 | 0 | 0 |
| **后端 — OOP** | 0 | 0 | 0 | 0 |
| **后端 — Agent 运行时** | 0 | 0 | 0 | 0 |
| **后端 — 并发安全** | 0 | 0 | 0 | 0 |
| **后端 — 错误处理** | 0 | 0 | 1 | 0 |
| **后端 — 依赖注入** | 0 | 0 | 0 | 0 |
| **构建与回归** | 0 | 0 | 0 | 0 |

**审查结论**：0 阻断、0 建议。Knowledge 模块 Round 3 所有变更通过 aranea-review 全维度检查。

### 亮点

- **KB-02 (P0)**：上传守卫三重防护 — base64 解码后大小限制 32MB + `http.DetectContentType` MIME magic 校验 + `allowedIngestMIMEs` 白名单，防止 base64 炸弹和恶意文件上传
- **KB-05 (P1)**：CreateCollection 时校验 embedding_model 与当前 embedder 配置一致，防止向量维度不匹配
- **KB-11 (P2)**：embedder HTTP timeout 通过 `KRATOS_KNOWLEDGE_EMBED_TIMEOUT_SEC` 环境变量可配，默认 60s
- **KB-13 (P2)**：`trpcDocsToChunks` 优先从 trpc 框架 `Metadata[MetaChunkIndex]` 读取 chunk index，解决 re-ingest 错位问题
- **KB-14 (P2)**：chunk ID 使用 `ch.ChunkIndex` 而非循环变量 `i`，与 KB-13 配合确保 ID 与索引一致
- **KB-19 (P2)**：`knowledge_search` 工具新增 `filter_json`（元数据过滤）和 `use_rerank`（重排序控制）参数，对齐 RPC Search API 能力

---

## 15. Knowledge + Skill 模块 Round 4 审查报告（2026-05-29）— P1-P2 批量修复

### 变更文件

| 文件 | 变更 | 对应 ID |
|------|------|---------|
| `internal/service/memory_embedder_adapter.go` | 新增 `MemoryEmbeddingAdapter`，封装 Knowledge Embedder 为 biz.EmbeddingService | KB-06 |
| `internal/service/service.go` | Wire 绑定改为 `MemoryEmbeddingAdapter` 实现 `biz.EmbeddingService` | KB-06 |
| `internal/team/runner_team_trpc.go` | `runTeamTRPCFromInput` 注入 `input.Options.KnowledgeBases` 到运行上下文 | KB-07 |
| `internal/knowledge/embedder.go` | 新增 `EmbedBatchWithTaskType` + `EmbedWithTaskType`，Gemini 支持 task type 参数 | KB-10 |
| `internal/knowledge/retriever.go` | 新增 `TaskTypeEmbedder` 接口 + `embedQuery` 方法，搜索时用 `RETRIEVAL_QUERY` | KB-10 |
| `internal/data/knowledge.go` | IVFFlat lists 改为 `ivfflatLists(dim)` 动态计算 + `KRATOS_KNOWLEDGE_IVFFLAT_LISTS` 环境变量 | KB-20 |
| `internal/skill/importer/helpers.go` | `slugify("")` 改用 `newID()[:8]` 生成唯一后缀 | SKILL-P2-03 |
| `internal/skill/watch/runner.go` | `SkillSyncer` 拆分为 `SkillReader`(3) + `SkillWriter`(3)，`*biz.SkillUsecase` → 接口依赖 | SKILL-P2-06 |
| `internal/skill/watch/reconcile.go` | `r.uc.ListRegisteredSlugs` → `r.reader.ListRegisteredSlugs` | SKILL-P2-06 |
| `cmd/admin/wire.go` | `NewRunnerWithBus` 调用改为传 `skillUC, skillUC, sys, eventBus` | SKILL-P2-06 |

### aranea-review 审查结果

| 维度 | 🔴 阻断 | 🟡 建议 | 🟢 提示 | 合计 |
|------|---------|---------|---------|------|
| **后端 — 架构合规** | 0 | 0 | 0 | 0 |
| **后端 — 分层合规** | 0 | 0 | 0 | 0 |
| **后端 — OOP** | 0 | 0 | 0 | 0 |
| **后端 — Agent 运行时** | 0 | 0 | 0 | 0 |
| **后端 — 并发安全** | 0 | 0 | 0 | 0 |
| **后端 — 错误处理** | 0 | 0 | 0 | 0 |
| **后端 — 依赖注入** | 0 | 0 | 0 | 0 |
| **构建与回归** | 0 | 0 | 0 | 0 |

**审查结论**：0 阻断、0 建议。Knowledge + Skill 模块 Round 4 所有变更通过 aranea-review 全维度检查。

### 亮点

- **KB-06 (P1)**：`MemoryEmbeddingAdapter` 为 Memory 和 Knowledge 提供了干净的解耦点，未来可独立替换 Memory 的 Embedder 而不影响 Knowledge
- **KB-07 (P1)**：Team Runner 注入 `KnowledgeBases` 到运行上下文，与 Chat 编排器行为一致，修复了 Team agent 无作用域限制的安全问题
- **KB-10 (P1)**：`TaskTypeEmbedder` 接口设计优雅，通过接口断言 `.(TaskTypeEmbedder)` 实现渐进增强，Gemini 搜索用 `RETRIEVAL_QUERY` 提升语义匹配质量
- **KB-20 (P2)**：`ivfflatLists` 函数基于维度自动计算合理的 lists 值（dim/4，范围 10-1000），同时支持 `KRATOS_KNOWLEDGE_IVFFLAT_LISTS` 环境变量覆盖
- **SKILL-P2-03 (P2)**：`slugify("")` 改用 `newID()[:8]` 生成唯一后缀，消除了多空名技能 slug 冲突
- **SKILL-P2-06 (P2)**：`SkillReader`(3 方法) + `SkillWriter`(3 方法) 替代 `*biz.SkillUsecase` 具体类型依赖，符合 ISP ≤5 规则

---

## 16. Hook / Model / Overview 模块 Round 5 审查报告（2026-05-29）— P1 批量修复

### 变更文件

| 文件 | 变更 | 对应 ID |
|------|------|---------|
| `internal/biz/hook/hook.go` | `Resolver` 新增 `sync.RWMutex` + `cache []ResolvedHook` + `loaded bool`；`Reload()` 存缓存；`Resolve()` 读缓存 | HK-04 |
| `internal/biz/usage/usage.go` | `QuotaRepo` 新增 `BatchSumScopeCost`；`QuotaDashboard` 改用批量查询 + 错误日志 | OV-04/OV-06/OV-07 |
| `internal/data/usage_quota.go` | 新增 `BatchSumScopeCost` 实现（按 scopeType+period 分组 + IN 批量 + GROUP BY） | OV-04/OV-06/OV-07 |
| `internal/biz/usage_quota_test.go` | stub repo 新增 `BatchSumScopeCost` 空实现 | OV-04 |
| `internal/biz/llm_provider_model.go` | `RunHealthChecks` 串行→并发（semaphore pool=5 + jitter + panic recovery） | MD-05 |
| `internal/service/gateway.go` | 已有 `maskSecret` + `webhookToProto`（List 脱敏） | HK-08 |

### aranea-review 审查结果

| 维度 | 🔴 阻断 | 🟡 建议 | 🟢 提示 | 合计 |
|------|---------|---------|---------|------|
| **后端 — 架构合规** | 0 | 0 | 0 | 0 |
| **后端 — 分层合规** | 0 | 0 | 0 | 0 |
| **后端 — OOP** | 0 | 1 | 0 | 1 |
| **后端 — Agent 运行时** | 0 | 0 | 0 | 0 |
| **后端 — 并发安全** | 0 | 1 | 0 | 1 |
| **后端 — 错误处理** | 0 | 0 | 0 | 0 |
| **后端 — 依赖注入** | 0 | 0 | 0 | 0 |
| **构建与回归** | 0 | 0 | 0 | 0 |

**建议项**：

| ID | 维度 | 文件 | 问题 | 建议 |
|----|------|------|------|------|
| S1 | OOP-BI1 | `usage/usage.go` | `QuotaRepo` 接口 8 方法 > 5 | 后续拆为 `QuotaReader`(3) + `QuotaWriter`(3) + `BudgetAlertRepo`(3) |
| S2 | 并发-BC1 | `llm_provider_model.go` | goroutine 手动 panic recovery 非 safego.Go | 因 WaitGroup 集成需要，功能等价；可提取 `safego.GoWithWG` 工具函数 |

**审查结论**：0 阻断、2 建议。Round 5 所有变更通过 aranea-review 全维度检查。

### 亮点

- **HK-04 (P1)**：`loaded` 标志区分"未加载"与"加载后为空"，避免空缓存时反复 DB 回退；`RWMutex` 读多写少场景性能优秀
- **OV-04/OV-06/OV-07 (P1)**：`BatchSumScopeCost` 按 (scopeType, periodStart, periodEnd) 分组 + IN 批量 + GROUP BY，从 N+1 降为 O(分组数)，典型场景 1-3 次 SQL；错误日志替代 `continue` 静默
- **MD-05 (P1)**：Worker pool + jitter + panic recovery 三重保障，并发安全且避免雷群效应；`healthCheckPoolSize=5` 可通过常量调整
- **HK-08 (P1)**：`maskSecret` + `webhookToProto`（List 脱敏）+ `webhookToProtoWithSecret`（Create/Update 明文）符合业界标准模式

### 剩余 P1 工作总结

| ID | 模块 | 问题 | 优先级 |
|----|------|------|--------|
| OV-01 | Overview | `Overview()` 10+ 顺序 DB 调用，无 errgroup / 无缓存 | P1 |
| OV-02 | Overview | 读路径扫 raw events 而非 rollup | P1 |
| OV-03 | Overview | 前端 `loadOverview` 静默 catch，失败用户无感知 | P1 |
| MD-02 | Model | 价格三写入路径无优先级合约 | P1 |
| MD-03 | Model | `Applier.Apply` 默认调 `RunProviderMigrations` | P1 |
| HK-06 | Hook | 无投递幂等键，重复触发 = 重复 POST | P1 |
| KB-09 | Knowledge | OCR tesseract/docling 仍返回 stub | P1 |
| EV-03 | Eval | `RunEvalAgentTurn` 每个用例新建 session | P1 |
| EV-04 | Eval | Judge 失败静默吞 | P1 |
| EV-08 | Eval | 没有数据集快照 | P1 |
| KB-17 | Knowledge | ListChunks/ReindexDocument/UpdateDocument RPC | P2 |
| SKILL-P2-01 | Skill | internal/tools/ ~84 fmt.Errorf 替换 | P2 |
| QuotaRepo-ISP | Usage | QuotaRepo 8 方法需拆子接口 | P2 |

