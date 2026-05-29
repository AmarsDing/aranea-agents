# Provider — 开发计划

> **版本**：2026-05-29 | **状态**：✅ 端到端可用（核心链路）+ 代码质量优化完成
> **需求**：[9 provider.md](./9%20provider.md) · **设计**：[9 provider.design.md](./9%20provider.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：—

---

## 1. 模块定位

Provider 管理：基于 trpc-agent-go `model` 体系的多厂商 LLM Provider 管理。支持 5 种原生 Provider（OpenAI / Anthropic / Gemini / Ollama / Hunyuan）+ 4 种 OpenAI Variant（OpenAI / DeepSeek / Qwen / Hunyuan）+ Failover/Hedge 高可用模式 + TokenTailor 自动裁剪 + Inspect 远程元数据探测 + Pricing 定价规则。

**代码锚点**：
- `api/kratos/llm_provider_model/v1/llm_provider_model.proto` — Proto CRUD + Inspect + ValidatePair
- `internal/service/llm_provider_model.go` — LlmProviderModelService
- `internal/biz/llm_provider_model.go` — LlmProviderModelUsecase + InspectMerge + ModelPricingRule + 子接口（Reader/Writer/Validator/Pricing）
- `internal/data/llm_provider_model.go` — LlmProviderModelRepo（Ent ORM + SQLite）
- `internal/data/ent/schema/llm_provider_model.go` — Ent Schema
- `internal/data/ent/schema/model_pricing_rule.go` — 定价规则 Schema
- `internal/provider/trpc_llm.go` — Provider → trpc Model 装配（含 HA + 预检 + 指标）
- `internal/provider/catalog.go` — CatalogConfig 解析与合并（使用 biz.ModelCapabilities）
- `internal/provider/roundtrip.go` — HTTP Transport 注入
- `internal/provider/stream_delta.go` — 流式 Delta 合并
- `internal/llminspect/inspect.go` — 远程模型元数据探测
- `internal/agent/trpc_build.go` — Agent 构建时调用 TRPCModelForProviderModel
- `internal/service/session_title_llm.go` — Session 标题生成调用 TRPCModelForProviderModel
- `web/src/config/providerPresets.ts` — 前端 Provider 预设（20 个厂商）
- `web/src/features/platform/types.ts` — 统一类型定义（ProviderConfig / ModelCategory / CapabilityChip）
- `web/src/features/platform/providerUtils.ts` — 共享工具函数（errorMessage / toNullableNumber / toNumber / getConfig / getCategories）
- `web/src/features/platform/useProviderList.ts` — Provider 列表逻辑 composable
- `web/src/features/platform/useProviderWizard.ts` — Provider 向导表单逻辑 composable
- `web/src/features/platform/useResourceManagerPage.ts` — Provider 管理页面编排 composable
- `web/src/components/platform/ProviderModelsTable.vue` — 列表表格组件
- `web/src/components/platform/ProviderTrendDialog.vue` — 趋势看板组件
- `web/src/components/platform/providerModelUi.ts` — 表格列定义 + UI 工具函数
- `web/src/pages/ResourceManagerPage.vue` — Provider 管理页面
- `web/src/features/platform/api.ts` — 前端 API 层

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| Provider Model CRUD | ✅ | List / Create / Get / Update / Delete（软删）全链路 |
| Inspect 远程探测 | ✅ | OpenRouter / OpenAI-Compatible / Anthropic 三条探测路径；DeepSeek 路由 |
| ValidatePair 校验 | ✅ | 按 provider + model 查询 enabled 行 |
| trpc Model 装配 | ✅ | `TRPCModelForProviderModel` 按 provider_type 分发构建；5 种原生 Provider + Variant |
| Failover/Hedge HA | ✅ | `wrapHA` + `wrapFailover` / `wrapHedge`；候选模型构建（含预检 + 指标装饰） |
| TokenTailoring | ✅ | `WithEnableTokenTailoring` 透传 |
| Provider 专属选项 | ✅ | OpenAI（Cache/Backfill/Delta）、Anthropic（Cache 三项）、Gemini（ClientConfig）、Ollama（KeepAlive）、Hunyuan（SecretId/Key） |
| Pricing 定价规则 | ✅ | `UpsertModelPricingRule`；Create/Update 时自动同步 |
| 前端预设 | ✅ | 20 个 Provider 预设；7 种 ProviderType；4 种 Variant；AuthType 联动 |
| 前端列表行 | ✅ | ProviderModelsTable：类型 Chip、分类 Chip、热度、用量、密钥状态、Toggle |
| 前端趋势看板 | ✅ | ProviderTrendDialog：30 天趋势柱状图、汇总卡片、详情表 |
| 前端管理页面 | ✅ | ResourceManagerPage：搜索、分页、创建/编辑弹窗 |
| Inspect 请求扩展字段 | ✅ | Proto + biz + service + 前端 Inspect 入参 |
| Inspect 响应扩展字段 | ✅ | Proto + service 映射 |
| mergeInspectConfigJSON 扩展 | ✅ | 含 variant / secret_id / secret_key / aws_region；needInspectMerge 支持混元/Bedrock |
| 前端 Variant Chip | ✅ | ProviderModelsTable |
| 前端 HA Chip | ✅ | ProviderModelsTable |
| 前端四步表单 | ✅ | ResourceManagerPage QStepper + ProviderHAConfig |
| Gemini/Ollama/Hunyuan Inspect | ✅ | llminspect 专属路径 + 单测 |
| HuggingFace/Bedrock Provider | ✅ | register_extra.go + MapProviderType |
| 凭据加密 | ✅ | AES-256-GCM（ARANEA_CREDENTIAL_KEY）；List/Get 脱敏 |
| 速率限制 | ✅ | config_json rate_limit_rpm + RoundTrip 令牌桶 |
| 健康检查 | ✅ | ProviderHealthScanner 5min |
| Repo 接口拆分 | ✅ | LlmProviderModelRepo → Reader/Writer/Validator/Pricing 子接口（红线 15 合规） |
| ModelCapabilities 统一 | ✅ | biz 层唯一定义，provider 层引用 biz.ModelCapabilities |
| HA 候选模型预检 + 指标 | ✅ | trpcModelFromCandidate 增加 outboundguard.ValidateURL + WrapModelWithMetrics |
| 前端类型统一 | ✅ | ProviderConfig / ModelCategory / CapabilityChip 统一到 types.ts |
| 前端 composable 拆分 | ✅ | useResourceManagerPage(1392→215) + useProviderList(168) + useProviderWizard(1123) |
| 前端工具函数提取 | ✅ | providerUtils.ts（errorMessage/toNullableNumber/toNumber/getConfig/getCategories） |
| 前端数据流合规 | ✅ | useAgentProviderModelPicker + useProviderWizard 均走 Store，不直接调 API |

---

## 3. 差距与优化

### 已完成优化（2026-05-29）

| # | 优化项 | 优先级 | 变更说明 |
|---|--------|--------|----------|
| O1 | Repo 接口拆分 | P1 | `LlmProviderModelRepo`(8方法) → 4 个子接口 + 1 个组合接口，红线 15 合规 |
| O2 | ModelCapabilities 统一 | P1 | biz 层唯一定义 + json tags，provider 层引用 biz.ModelCapabilities，消除重复映射 |
| O3 | HA 候选模型预检 + 指标 | P2 | `trpcModelFromCandidate` 增加 `outboundguard.ValidateURL` + `WrapModelWithMetrics` |
| O4 | 清理 HuggingFace 空壳 | P2 | `buildHuggingFaceSpecificOptions` 从无效守卫改为 `_ CatalogConfig` 签名 |
| O5 | wrapHA 移除无用 ctx | P3 | `wrapHA` 签名从 `(ctx, primary, cfg, rt)` 简化为 `(primary, cfg, rt)` |
| O6 | 前端类型统一 | P1 | `ProviderConfig`/`ModelCategory`/`CapabilityChip` 统一到 `types.ts` |
| O7 | 前端 composable 拆分 | P1 | `useResourceManagerPage`(1392→215行) + `useProviderList`(168行) + `useProviderWizard`(1123行) |
| O8 | 前端工具函数提取 | P2 | `errorMessage`/`toNullableNumber`/`toNumber`/`getConfig`/`getCategories` → `providerUtils.ts` |
| O9 | 前端数据流合规 | P2 | `useAgentProviderModelPicker` + `useProviderWizard` 改走 Store，不直接调 API |

### 剩余优化项

| # | 差距 | 优先级 | 说明 |
|---|------|--------|------|
| R1 | biz ↔ provider 双向依赖完全隔离 | P1 | 当前 provider import biz（合规），但 biz 通过 SetInspector/SetCredentialKeyResolver 全局注入仍存在；需改为 Wire 构造函数注入 |
| R2 | 未配置定价规则时的用户提示 | P1 | 前端已有 `showPricingWarning`，但后端 `total_cost_micro_usd=0` 配额 SUM 失效需在 Monitor 中提示 |
| R3 | Provider 适配路径单测 | P2 | `trpc_llm.go` 各厂商 buildXxxSpecificOptions 缺乏单测 |
| R4 | HA 故障转移事件可视化 | P2 | failover/hedge 切换行为未在 Monitor Events 中展示 |
| R5 | UpsertModelPricingRule 事务 | P2 | 当前先查后写不在同一事务中，存在竞态条件 |
| R6 | RunHealthChecks 并发安全 | P2 | 健康检查直接修改快照行状态后 Update，可能覆盖中间修改 |
| R7 | useProviderWizard 仍较大 | P2 | 1123 行，可进一步拆分（目录选择、凭据管理、Inspect 逻辑） |
| R8 | 凭据加密降级风险 | P3 | ARANEA_CREDENTIAL_KEY 未配置时系统半工作状态，需明确提示或强制配置 |

---

## 4. 开发阶段

- **Phase 1**：Inspect 扩展字段（Proto + biz + service + 前端）— ✅ 解锁 Hunyuan/Bedrock Inspect
- **Phase 2**：前端 UI 增强（Variant Chip + HA Chip + 四步表单）— ✅
- **Phase 3**：llminspect 路径扩展（Gemini / Ollama / Hunyuan 专属探测）— ✅
- **Phase 4**：凭据加密存储（AES-256-GCM）— ✅
- **Phase 5**：速率限制 + 健康检查定时任务 — ✅
- **Phase 6**：代码质量优化（接口拆分 / 类型统一 / composable 拆分 / 数据流合规）— ✅ 2026-05-29
- **Phase 7**：架构深度优化（Wire DI 改造 / 事务安全 / 单测补充）— 待开发

---

## 5. 任务清单

| # | 任务 | 优先级 | EP | 状态 |
|---|------|--------|-----|------|
| 1 | Inspect 请求扩展：Proto 增加 variant/secret_id/secret_key/aws_region | P1 | — | ✅ |
| 2 | Inspect 响应扩展：Proto 增加 variant/enable_token_tailoring/supports_cache/supports_thinking | P1 | — | ✅ |
| 3 | biz InspectMerge 增加 Variant/SecretID/SecretKey/AWSRegion 字段 | P1 | — | ✅ |
| 4 | mergeInspectConfigJSON 扩展：合并 variant/secret_id/secret_key/aws_region | P1 | — | ✅ |
| 5 | service Inspect 映射新字段 | P1 | — | ✅ |
| 6 | 前端 ProviderModelRow 增加 Variant Chip | P2 | — | ✅ |
| 7 | 前端 ProviderModelRow 增加 HA Chip（Failover/Hedge） | P2 | — | ✅ |
| 8 | 前端添加/编辑弹窗改为四步表单 | P2 | — | ✅ |
| 9 | llminspect 增加 Gemini 专属探测路径 | P2 | — | ✅ |
| 10 | llminspect 增加 Ollama 专属探测路径 | P2 | — | ✅ |
| 11 | llminspect 增加 Hunyuan 专属探测路径 | P2 | — | ✅ |
| 12 | 凭据加密：AES-256-GCM 加密 api_key | P2 | — | ✅ |
| 13 | 速率限制：provider 表增加 rate_limit 字段 + 运行时限流 | P3 | — | ✅ |
| 14 | 健康检查：5min ticker + safego | P3 | — | ✅ |
| 15 | Repo 接口拆分：LlmProviderModelRepo → 子接口 | P1 | — | ✅ |
| 16 | ModelCapabilities 统一到 biz 层 | P1 | — | ✅ |
| 17 | HA 候选模型预检 + 指标装饰 | P2 | — | ✅ |
| 18 | 清理 buildHuggingFaceSpecificOptions 空壳 | P2 | — | ✅ |
| 19 | wrapHA 移除无用 ctx 参数 | P3 | — | ✅ |
| 20 | 前端 ProviderConfig 类型统一到 types.ts | P1 | — | ✅ |
| 21 | 前端 useResourceManagerPage composable 拆分 | P1 | — | ✅ |
| 22 | 前端工具函数提取到 providerUtils.ts | P2 | — | ✅ |
| 23 | useAgentProviderModelPicker 改走 Store | P2 | — | ✅ |
| 24 | useProviderWizard validateModel 改走 Store | P2 | — | ✅ |
| 25 | SetInspector/SetCredentialKeyResolver 改为 Wire 构造函数注入 | P1 | — | ⏳ |
| 26 | UpsertModelPricingRule 事务安全 | P2 | — | ⏳ |
| 27 | Provider 适配路径单测 | P2 | — | ⏳ |
| 28 | HA 故障转移事件可视化 | P2 | — | ⏳ |
| 29 | RunHealthChecks 并发安全 | P2 | — | ⏳ |
| 30 | useProviderWizard 进一步拆分 | P2 | — | ⏳ |

---

## 6. 验收标准

- [x] Inspect 请求/响应包含 variant / secret_id / secret_key / aws_region 等扩展字段
- [x] Hunyuan Provider Inspect 能从已保存配置回填 SecretId/SecretKey
- [x] 前端列表行展示 Variant Chip 和 HA Chip
- [x] 前端添加/编辑弹窗为四步表单
- [x] Gemini / Ollama / Hunyuan Inspect 有专属探测路径
- [x] api_key 在数据库中为密文（需配置 ARANEA_CREDENTIAL_KEY）
- [x] 速率限制在 Agent 运行时生效
- [x] Provider 异常时自动标记状态（degraded）
- [x] LlmProviderModelRepo 接口方法 ≤ 5（子接口拆分）
- [x] ModelCapabilities 在 biz 层唯一定义
- [x] HA 候选模型经过预检和指标装饰
- [x] 前端 ProviderConfig 类型统一到 types.ts
- [x] 前端 Page 编排 composable ≤ ~200 行
- [x] 前端数据流合规（无直接 API 调用绕过 Store）
- [ ] SetInspector/SetCredentialKeyResolver 通过 Wire 构造函数注入
- [ ] UpsertModelPricingRule 在事务中执行
- [ ] Provider 适配路径有单测覆盖

---

## 7. 依赖与风险

- Inspect 扩展字段需同步更新 Proto（`make api`）和前端类型
- 凭据加密需迁移现有明文数据
- 速率限制需与 trpc-agent-go LLM 调用链集成
- HuggingFace / Bedrock Provider 依赖 trpc 上游注册到 provider 工厂
- llminspect Gemini / Ollama / Hunyuan 专属探测需研究各厂商 API 规范
- SetInspector/SetCredentialKeyResolver 改 Wire 注入需解决 biz ↔ llminspect 循环依赖
- UpsertModelPricingRule 事务改造需评估 Ent 事务 API 兼容性
