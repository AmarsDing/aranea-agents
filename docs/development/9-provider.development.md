# Provider — 开发计划

> **版本**：2026-06-06 | **状态**：✅ 端到端可用（核心链路）+ Phase 6-9 优化完成
> **需求**：[9 provider.md](./9%20provider.md) · **设计**：[9 provider.design.md](./9%20provider.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：—

---

## 1. 模块定位

Provider 管理：基于 trpc-agent-go `model` 体系的多厂商 LLM Provider 管理。支持 5 种原生 Provider（OpenAI / Anthropic / Gemini / Ollama / Hunyuan）+ 4 种 OpenAI Variant（OpenAI / DeepSeek / Qwen / Hunyuan）+ Failover/Hedge 高可用模式 + TokenTailor 自动裁剪 + Inspect 远程元数据探测 + Pricing 定价规则。

**代码锚点**：

### 后端

| 文件 | 职责 |
|------|------|
| `api/kratos/llm_provider_model/v1/llm_provider_model.proto` | Proto CRUD + Inspect + ValidatePair + RevealCredentials（含 pricing_configured / capabilities） |
| `internal/service/llm_provider_model.go` | LlmProviderModelService（Proto ↔ Biz 转换） |
| `internal/biz/llm_provider_model.go` | LlmProviderModelUsecase + InspectMerge + ModelPricingRule + ModelCapabilities + 子接口（Reader/Writer/Validator/Pricing） |
| `internal/biz/credential_crypto.go` | CredentialCrypto 方法（加解密 / 存储处理 / 运行时解密 / 凭据揭示） |
| `internal/biz/credential_key.go` | CredentialCrypto struct（DI 注入，替代全局 SetCredentialKeyResolver） |
| `internal/biz/channel_credential_crypto.go` | CredentialCrypto 方法（渠道凭据加解密） |
| `internal/data/llm_provider_model.go` | LlmProviderModelRepo（Ent ORM + SQLite + 事务 Upsert） |
| `internal/data/ent/schema/llm_provider_model.go` | Ent Schema（含 capability_* 列 + capabilities_explicit） |
| `internal/data/ent/schema/model_pricing_rule.go` | 定价规则 Schema（含 USDPer1M + CacheWritePrice） |
| `internal/provider/trpc_llm.go` | Provider → trpc Model 装配（含 HA + 预检 + 指标 + 故障切换回调） |
| `internal/provider/catalog.go` | ProviderModelConfig 解析与合并（含 Retry/CB/RateLimit/Capabilities） |
| `internal/provider/capabilities.go` | 模型能力推导（CapabilitiesForProviderModel） |
| `internal/provider/roundtrip.go` | HTTP Transport 注入 |
| `internal/provider/rate_limit_transport.go` | 速率限制 Transport（令牌桶） |
| `internal/provider/retry_transport.go` | 重试 Transport（指数退避） |
| `internal/provider/circuit_breaker_transport.go` | 熔断 Transport |
| `internal/provider/metrics_model.go` | 模型指标装饰（WrapModelWithMetrics） |
| `internal/provider/stream_delta.go` | 流式 Delta 合并 |
| `internal/provider/register_extra.go` | HuggingFace/Bedrock Provider 注册预留 |
| `internal/provider/errors.go` | Provider 错误定义 |
| `internal/provider/trpc_llm_options_test.go` | Provider 适配路径单测（37 用例） |
| `internal/provider/trpc_llm_variant_test.go` | Variant 推断单测 |
| `internal/provider/trpc_llm_fixes_test.go` | Provider 修复单测 |
| `internal/provider/stream_delta_test.go` | 流式 Delta 单测 |
| `internal/llminspect/inspect.go` | 远程模型元数据探测（OpenRouter / OpenAI-Compatible / Anthropic / Gemini / Ollama / Hunyuan 路径） |
| `internal/agent/trpc_build.go` | Agent 构建时调用 TRPCModelForProviderModel |
| `internal/service/session_title_llm.go` | Session 标题生成调用 TRPCModelForProviderModel |
| `pkg/trpc-agent-go/model/failover/options.go` | failover WithSwitchCallback 选项 |
| `pkg/trpc-agent-go/model/hedge/options.go` | hedge WithSwitchCallback 选项 |

### 前端

| 文件 | 职责 |
|------|------|
| `web/src/config/providerPresets.ts` | 前端 Provider 预设（13 个 shell + custom） |
| `web/src/config/providerRuntimeOverlay.ts` | models.dev → trpc 运行时映射 |
| `web/src/config/providerRuntimeOverlay.types.ts` | 运行时 overlay 类型定义 |
| `web/src/config/provider_runtime_overlay.json` | overlay 数据（与后端同步） |
| `web/src/features/platform/types.ts` | 统一类型定义（ProviderConfig / ModelCategory / CapabilityChip） |
| `web/src/features/platform/providerUtils.ts` | 共享工具函数（errorMessage / toNullableNumber / toNumber / getConfig / getCategories / hasPricingConfigured） |
| `web/src/features/platform/useProviderList.ts` | Provider 列表逻辑 composable |
| `web/src/features/platform/useProviderCatalog.ts` | 目录选择逻辑 composable |
| `web/src/features/platform/useProviderCredentials.ts` | 凭据管理逻辑 composable |
| `web/src/features/platform/useProviderInspect.ts` | Inspect 检查逻辑 composable |
| `web/src/features/platform/useProviderPresets.ts` | 预设应用逻辑 composable |
| `web/src/features/platform/useProviderWizard.ts` | Provider 向导编排 composable（527 行） |
| `web/src/features/platform/useProviderSave.ts` | Provider 保存逻辑 composable（205 行） |
| `web/src/features/platform/useResourceManagerPage.ts` | Provider 管理页面编排 composable |
| `web/src/features/platform/api.ts` | 前端 API 层 |
| `web/src/components/platform/ProviderModelsTable.vue` | 列表表格组件（含定价警告图标） |
| `web/src/components/platform/ProviderTrendDialog.vue` | 趋势看板组件 |
| `web/src/components/platform/ProviderHAConfig.vue` | 高可用配置组件 |
| `web/src/components/platform/ProviderWizardStep1Connect.vue` | 步骤 1：连接与身份 |
| `web/src/components/platform/ProviderWizardStep2Specs.vue` | 步骤 2：模型分类与规格 |
| `web/src/components/platform/ProviderWizardStep3HA.vue` | 步骤 3：高可用配置 |
| `web/src/components/platform/ProviderWizardStep4Advanced.vue` | 步骤 4：高级选项 |
| `web/src/components/platform/ProviderLogo.vue` | Provider 图标 |
| `web/src/components/platform/providerModelUi.ts` | 表格列定义 + UI 工具函数 |
| `web/src/pages/ResourceManagerPage.vue` | Provider 管理页面（含凭据加密警告） |

---

## 2. 现状评估

### 2.1 实现状态快速索引

| 章节 | 需求描述 | 状态 | 代码证据 |
|------|---------|------|---------|
| §6.1 | Failover 高可用 | ✅ | `wrapFailover` + `trpcfailover.New` + `WithSwitchCallback` 事件 |
| §6.2 | Hedge 低延迟 | ✅ | `wrapHedge` + `trpchedge.New` + `WithSwitchCallback` 事件 |
| §6.3 | TokenTailor | ✅ | `WithEnableTokenTailoring` 透传 + Strategy + SafetyMargin |
| §6.4 | 多模型注册 | ✅ | 5 种已注册 Provider 可正常调用 |
| — | IterModel 优化 | ✅ | `WrapModelWithMetrics` 在 inner 实现 `IterModel` 时返回 `metricsIterModel`，保留迭代路径 |
| — | 5 种原生 Provider + 4 种 Variant | ✅ | `MapProviderType` + `trpcprovider.Model` 工厂 |
| — | Provider 专属选项构建 | ✅ | `buildOpenAISpecificOptions` / `buildAnthropicSpecificOptions` / `buildGeminiSpecificOptions` / `buildOllamaSpecificOptions` / `buildHunyuanSpecificOptions` |
| — | ProviderModelConfig 扩展 | ✅ | `catalog.go` ProviderModelConfig 含 Variant / SecretID / SecretKey / AWSRegion / HA / Retry / CB / RateLimit / Capabilities |
| — | Inspect 请求扩展字段 | ✅ | Proto 含 variant / secret_id / secret_key / aws_region |
| — | Inspect 响应扩展字段 | ✅ | Proto 含 variant / enable_token_tailoring / supports_cache / supports_thinking |
| — | RevealProviderModelCredentials | ✅ | Proto + Service + Usecase 解密返回 |
| — | providerPresets.ts shell + overlay 架构 | ✅ | 13 个预设 shell + providerRuntimeOverlay |
| — | ResourceManagerPage 四步表单 | ✅ | QStepper 四步表单 + ProviderHAConfig |
| — | ProviderModelRow Variant/HA Chip | ✅ | ProviderModelsTable 含 Variant Chip 和 HA Chip |
| — | catalog.go 扩展 | ✅ | ProviderModelConfig 全字段 + MergeModelConfig 全字段合并 |
| — | trpc_llm.go 分发构建 | ✅ | `trpcModelFromProviderModelConfig` + `buildProviderOptions` + 5 种 Provider builder |
| — | biz InspectMerge 扩展 | ✅ | InspectMerge 含 variant / secret_id / secret_key / aws_region；`mergeInspectConfigJSON` 含对应合并 |
| — | Proto pricing_configured | ✅ | Proto field 16 + Service 映射 |
| — | Proto capabilities | ✅ | Proto field 15 + `CapabilitiesForProviderModel` 推导 |
| — | llminspect Gemini/Ollama/Hunyuan 路径 | ✅ | llminspect 专属路径 + 单测 |
| — | HuggingFace / Bedrock Provider | ⏳ 待上游 | trpc provider 工厂未注册；前端预设已预留；`register_extra.go` + `MapProviderType` 已就绪 |
| — | 凭据加密 | ✅ | AES-256-GCM（ARANEA_CREDENTIAL_KEY）；List/Get 脱敏；降级警告 |
| — | Pricing 定价规则 | ✅ | `UpsertModelPricingRule`（事务安全）；Create/Update 时自动同步 |
| — | Agent 构建链路接入 | ✅ | `internal/agent/trpc_build.go` + `internal/service/session_title_llm.go` |
| — | 前端趋势看板 | ✅ | `ProviderTrendDialog.vue`：30 天趋势柱状图、汇总卡片、详情表 |
| — | 速率限制 | ✅ | config_json rate_limit_rpm + RoundTrip 令牌桶 |
| — | 重试 + 熔断 | ✅ | retry_transport.go + circuit_breaker_transport.go |
| — | 健康检查 | ✅ | ProviderHealthScanner 5min；safego.Go；UpdateProviderModelStatus |
| — | 定价缺失提示 | ✅ | 后端 PricingConfigured 字段 + 前端 price_check 警告图标 |
| — | 凭据加密降级提示 | ✅ | CredentialCrypto.IsAvailable + 启动警告日志 + 前端 q-banner 警告 |
| — | HA 故障转移事件可视化 | ✅ | failover/hedge `WithSwitchCallback` → `event.CtxFlowLogWarn`；step ID 已注册 |

### 2.2 现状评估表

| 项 | 状态 | 证据 |
|----|------|------|
| Provider Model CRUD | ✅ | List / Create / Get / Update / Delete（软删）全链路 |
| Inspect 远程探测 | ✅ | OpenRouter / OpenAI-Compatible / Anthropic / Gemini / Ollama / Hunyuan 六条探测路径；DeepSeek 路由 |
| ValidatePair 校验 | ✅ | 按 provider + model 查询 enabled 行 |
| RevealProviderModelCredentials | ✅ | 解密返回 api_key / secret_key / ha_candidates 凭据 |
| trpc Model 装配 | ✅ | `TRPCModelForProviderModel` 按 provider_type 分发构建；5 种原生 Provider + Variant |
| Failover/Hedge HA | ✅ | `wrapHA` + `wrapFailover` / `wrapHedge`；候选模型构建（含预检 + 指标装饰） |
| HA 故障切换事件 | ✅ | failover/hedge `WithSwitchCallback` → `event.CtxFlowLogWarn`；step ID 已注册 |
| TokenTailoring | ✅ | `WithEnableTokenTailoring` 透传 + Strategy + SafetyMargin |
| Provider 专属选项 | ✅ | OpenAI（Cache/Backfill/Delta）、Anthropic（Cache 三项）、Gemini（ClientConfig）、Ollama（KeepAlive）、Hunyuan（SecretId/Key） |
| Pricing 定价规则 | ✅ | `UpsertModelPricingRule`（事务安全）；Create/Update 时自动同步 |
| 前端预设 | ✅ | 13 个 Provider 预设 shell + providerRuntimeOverlay；7 种 ProviderType；4 种 Variant；AuthType 联动 |
| 前端列表行 | ✅ | ProviderModelsTable：类型 Chip、分类 Chip、热度、用量、密钥状态、Toggle、定价警告 |
| 前端趋势看板 | ✅ | ProviderTrendDialog：30 天趋势柱状图、汇总卡片、详情表 |
| 前端管理页面 | ✅ | ResourceManagerPage：搜索、分页、创建/编辑弹窗、凭据加密警告 |
| Inspect 请求扩展字段 | ✅ | Proto + biz + service + 前端 Inspect 入参 |
| Inspect 响应扩展字段 | ✅ | Proto + service 映射 |
| mergeInspectConfigJSON 扩展 | ✅ | 含 variant / secret_id / secret_key / aws_region；needInspectMerge 支持混元/Bedrock |
| 前端 Variant Chip | ✅ | ProviderModelsTable |
| 前端 HA Chip | ✅ | ProviderModelsTable |
| 前端四步表单 | ✅ | ResourceManagerPage QStepper + ProviderHAConfig + ProviderWizardStep1-4 |
| Gemini/Ollama/Hunyuan Inspect | ✅ | llminspect 专属路径 + 单测 |
| HuggingFace/Bedrock Provider | ✅ | register_extra.go + MapProviderType |
| 凭据加密 | ✅ | AES-256-GCM（ARANEA_CREDENTIAL_KEY）；List/Get 脱敏；降级警告 |
| 速率限制 | ✅ | config_json rate_limit_rpm + RoundTrip 令牌桶 |
| 重试 + 熔断 | ✅ | retry_transport.go + circuit_breaker_transport.go |
| 健康检查 | ✅ | ProviderHealthScanner 5min；safego.Go；UpdateProviderModelStatus |
| Repo 接口拆分 | ✅ | LlmProviderModelRepo → Reader/Writer/Validator/Pricing 子接口（红线 15 合规） |
| ModelCapabilities 统一 | ✅ | biz 层唯一定义，provider 层引用 biz.ModelCapabilities |
| HA 候选模型预检 + 指标 | ✅ | trpcModelFromCandidate 增加 outboundguard.ValidateURL + WrapModelWithMetrics |
| 前端类型统一 | ✅ | ProviderConfig / ModelCategory / CapabilityChip 统一到 types.ts |
| 前端 composable 拆分 | ✅ | useResourceManagerPage(215) + useProviderList(168) + useProviderWizard(527) + useProviderCatalog(260) + useProviderCredentials(130) + useProviderInspect(180) + useProviderPresets(65) + useProviderSave(205) |
| 前端工具函数提取 | ✅ | providerUtils.ts（errorMessage/toNullableNumber/toNumber/getConfig/getCategories/hasPricingConfigured） |
| 前端数据流合规 | ✅ | useAgentProviderModelPicker + useProviderWizard 均走 Store，不直接调 API |
| Wire DI 合规 | ✅ | SetInspector + SetCredentialKeyResolver 均已删除，通过 CredentialCrypto 构造函数注入 |
| 事务安全 | ✅ | UpsertModelPricingRule 在 Ent 事务中执行 |
| 并发安全 | ✅ | RunHealthChecks 使用 UpdateProviderModelStatus（只更新 status 字段）+ safego.Go |
| Provider 适配路径单测 | ✅ | 37 个用例覆盖 MapProviderType + 6 个厂商 buildXxxSpecificOptions + CapabilitiesForProviderModel |
| 定价缺失提示 | ✅ | 后端 PricingConfigured 字段 + 前端 price_check 警告图标 |
| 凭据加密降级提示 | ✅ | CredentialCrypto.IsAvailable + 启动警告日志 + 前端 q-banner 警告 |
| 解密失败日志 | ✅ | DecryptConfigJSONForRuntime 解密失败时 SysLogWarn 记录 |
| Proto pricing_configured | ✅ | Proto field 16 + Service 映射 + 前端可直接使用 |
| Proto capabilities | ✅ | Proto field 15 + Service `toProtoPM` 调用 `CapabilitiesForProviderModel` |

---

## 3. 差距与优化

### 已完成优化（Phase 6 — 2026-05-29）

| # | 优化项 | 优先级 | 变更说明 |
|---|--------|--------|----------|
| O1 | Repo 接口拆分 | P1 | `LlmProviderModelRepo`(8方法) → 4 个子接口 + 1 个组合接口，红线 15 合规 |
| O2 | ModelCapabilities 统一 | P1 | biz 层唯一定义 + json tags，provider 层引用 biz.ModelCapabilities，消除重复映射 |
| O3 | HA 候选模型预检 + 指标 | P2 | `trpcModelFromCandidate` 增加 `outboundguard.ValidateURL` + `WrapModelWithMetrics` |
| O4 | 清理 HuggingFace 空壳 | P2 | `buildHuggingFaceSpecificOptions` 从无效守卫改为 `_ ProviderModelConfig` 签名 |
| O5 | wrapHA 移除无用 ctx | P3 | `wrapHA` 签名从 `(ctx, primary, cfg, rt)` 简化为 `(primary, cfg, rt)` |
| O6 | 前端类型统一 | P1 | `ProviderConfig`/`ModelCategory`/`CapabilityChip` 统一到 `types.ts` |
| O7 | 前端 composable 拆分（第一轮） | P1 | `useResourceManagerPage`(1392→215行) + `useProviderList`(168行) + `useProviderWizard`(1123行) |
| O8 | 前端工具函数提取 | P2 | `errorMessage`/`toNullableNumber`/`toNumber`/`getConfig`/`getCategories` → `providerUtils.ts` |
| O9 | 前端数据流合规 | P2 | `useAgentProviderModelPicker` + `useProviderWizard` 改走 Store，不直接调 API |

### 已完成优化（Phase 7 — 2026-05-29）

| # | 优化项 | 优先级 | 变更说明 |
|---|--------|--------|----------|
| O10 | SetInspector 改 Wire 构造函数注入 | P1 | `inspector LLMInspector` 作为构造函数参数，删除 `SetInspector` 方法，消除全局注入反模式 |
| O11 | 定价缺失提示 | P1 | 后端 `PricingConfigured` 字段 + `configJSONHasPricing` + 前端 `price_check` 警告图标 + tooltip |
| O12 | Provider 适配路径单测 | P2 | 37 个用例：MapProviderType(16) + buildXxxSpecificOptions(26) + CapabilitiesForProviderModel(7) |
| O13 | UpsertModelPricingRule 事务安全 | P2 | 查询+更新/创建包裹在 Ent 事务中，消除竞态条件 |
| O14 | RunHealthChecks 并发安全 | P2 | 新增 `UpdateProviderModelStatus` 只更新 status+updated_at；`go func()` → `safego.Go`（红线 13 合规） |
| O15 | useProviderWizard 进一步拆分 | P2 | 1123→808行 + `useProviderCatalog`(260行) + `useProviderCredentials`(130行) |
| O16 | 凭据加密降级提示 | P3 | `IsCredentialEncryptionAvailable` + 启动 `event.SysLogWarn` + 前端 `q-banner` 警告 |

### 已完成优化（Phase 8 — 2026-05-29）

| # | 优化项 | 优先级 | 变更说明 |
|---|--------|--------|----------|
| O17 | SetCredentialKeyResolver 改 Wire 构造函数注入 | P2 | 创建 `CredentialCrypto` struct 封装 resolver + 所有加解密方法；注入到 LlmProviderModelUsecase / ChannelUsecase / MCPServerUsecase / SystemSettingService；删除全局 `credentialKeyResolver` + `SetCredentialKeyResolver`；`NewSystemSettingRepo` 不再有全局副作用（BD3 合规） |
| O18 | HA 故障转移事件可视化 | P2 | failover/hedge 框架包新增 `WithSwitchCallback` 选项；`wrapFailover`/`wrapHedge` 传入回调发射 `event.CtxFlowLogWarn`；step ID `system.provider.ha_failover`/`system.provider.ha_hedge` 已注册 |
| O19 | Proto 增加 pricing_configured 字段 | P3 | Proto `ProviderModel` 增加 `pricing_configured`（field 16）；Service 层 `toProtoPM` 映射 |
| O20 | useProviderWizard 继续拆分 | P3 | 808→649行 + `useProviderInspect`(180行) + `useProviderPresets`(65行)；Inspect/Preset 逻辑独立 composable |
| O21 | DecryptConfigJSONForRuntime 解密失败日志 | P2 | 解密失败时 `event.SysLogWarn` 记录，避免密钥轮换后静默降级 |
| O22 | EncryptChannelSecretRef 错误传播 | P2 | `aesKey` 错误不再吞掉，改为 `errors.InternalServer` 返回 |

### 已完成优化（Phase 9 — 2026-05-29）

| # | 优化项 | 优先级 | 变更说明 |
|---|--------|--------|----------|
| O23 | MCPServerRepo 接口拆分 | 🟡→✅ | `MCPServerRepo`(7方法) → `MCPServerReader`(3) + `MCPServerWriter`(3) + `MCPServerMetadataWriter`(1) + 组合接口；health runner Deps 收窄为 `MCPServerReader`；Wire 绑定 `MCPServerReader→MCPServerRepo` |
| O24 | useProviderWizard 继续拆分 | 🟡→✅ | 649→527行 + `useProviderSave`(205行)；`buildProviderPayload`+`saveProviderRow` 独立 composable |
| O25 | DecryptConfigJSONForRuntime 返回值 | 🟡→✅ | 返回 `(string, error)` 替代 `string`；`PrepareProviderModelForRuntime` 返回 `(ProviderModel, error)`；调用方显式处理 error |
| O26 | RunHealthChecks goroutine ctx 取消 | 🟡→✅ | goroutine 内写操作改用 `context.WithoutCancel(ctx)`；Channel + Provider 两处 RunHealthChecks 均已修复 |
| O27 | service 层遗留编译修复 | P1 | 修复 `resolveCredentialPlain`/`ResolveSecretRef` 调用签名（加 `*biz.ChannelUsecase` 参数）；16 个文件；添加 `CompressorDeps`/`MCPServerReader` Wire 绑定 |

### 剩余优化项

| # | 差距 | 优先级 | 说明 |
|---|------|--------|------|
| R1 | useProviderWizard 仍 527 行 | P3 | 可继续拆 `populateProviderForm`+`resetProviderForm` → `useProviderFormLifecycle.ts` |
| R2 | DecryptConfigJSONForRuntime 降级策略 | P3 | 当前解密失败仍返回原 JSON（降级），可考虑在 Inspect 场景返回 error 而非降级 |
| R3 | AgentRuntimeSettings / RalphLoopSettings 测试失败 | P2 | 非本次改动引起，需单独排查 |
| R4 | IterModel 优化 | ✅ | `WrapModelWithMetrics` 透传 IterModel（`metricsIterModel`） |
| R5 | HuggingFace / Bedrock Provider 注册 | ✅ | `RegisterExtraProviders` 启动注册；前端预设已预留 |

---

## 4. 开发阶段

- **Phase 1**：Inspect 扩展字段（Proto + biz + service + 前端）— ✅ 解锁 Hunyuan/Bedrock Inspect
- **Phase 2**：前端 UI 增强（Variant Chip + HA Chip + 四步表单）— ✅
- **Phase 3**：llminspect 路径扩展（Gemini / Ollama / Hunyuan 专属探测）— ✅
- **Phase 4**：凭据加密存储（AES-256-GCM）— ✅
- **Phase 5**：速率限制 + 健康检查定时任务 — ✅
- **Phase 6**：代码质量优化（接口拆分 / 类型统一 / composable 拆分 / 数据流合规）— ✅ 2026-05-29
- **Phase 7**：架构深度优化（Wire DI / 事务安全 / 并发安全 / 单测 / 降级提示）— ✅ 2026-05-29
- **Phase 8**：全局状态消除 + HA 事件 + Proto 扩展 + 前端继续拆分 — ✅ 2026-05-29
- **Phase 9**：建议项清零 + service 层遗留编译修复 — ✅ 2026-05-29

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
| 25 | SetInspector 改为 Wire 构造函数注入 | P1 | — | ✅ |
| 26 | UpsertModelPricingRule 事务安全 | P2 | — | ✅ |
| 27 | Provider 适配路径单测（37 用例） | P2 | — | ✅ |
| 28 | RunHealthChecks 并发安全 + safego 合规 | P2 | — | ✅ |
| 29 | useProviderWizard 进一步拆分（Catalog + Credentials） | P2 | — | ✅ |
| 30 | 定价缺失提示（后端 PricingConfigured + 前端警告图标） | P1 | — | ✅ |
| 31 | 凭据加密降级提示（IsCredentialEncryptionAvailable + 前端 Banner） | P3 | — | ✅ |
| 32 | SetCredentialKeyResolver 改为 Wire 构造函数注入（CredentialCrypto） | P2 | — | ✅ |
| 33 | HA 故障转移事件可视化（WithSwitchCallback + FlowLog） | P2 | — | ✅ |
| 34 | Proto 增加 pricing_configured 字段 | P3 | — | ✅ |
| 35 | useProviderWizard 继续拆分（Inspect + Presets composable） | P3 | — | ✅ |
| 36 | DecryptConfigJSONForRuntime 解密失败日志 | P2 | — | ✅ |
| 37 | EncryptChannelSecretRef 错误传播修复 | P2 | — | ✅ |
| 38 | MCPServerRepo 接口拆分（Reader/Writer/MetadataWriter） | 🟡 | — | ✅ |
| 39 | useProviderSave composable 提取 | 🟡 | — | ✅ |
| 40 | DecryptConfigJSONForRuntime 返回 (string, error) | 🟡 | — | ✅ |
| 41 | RunHealthChecks goroutine ctx 取消修复 | 🟡 | — | ✅ |
| 42 | service 层 resolveCredentialPlain/ResolveSecretRef 签名修复 | P1 | — | ✅ |
| 43 | Proto 增加 capabilities 字段（field 15）+ ModelCapabilities message | P1 | — | ✅ |
| 44 | Proto 增加 RevealProviderModelCredentials RPC | P2 | — | ✅ |
| 45 | 重试 + 熔断 Transport（retry_transport.go + circuit_breaker_transport.go） | P2 | — | ✅ |
| 46 | 前端 providerRuntimeOverlay 架构（shell + overlay） | P2 | — | ✅ |
| 47 | 前端 ProviderWizardStep1-4 组件拆分 | P2 | — | ✅ |

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
- [x] SetInspector 通过 Wire 构造函数注入
- [x] UpsertModelPricingRule 在事务中执行
- [x] Provider 适配路径有单测覆盖（37 用例）
- [x] RunHealthChecks 并发安全 + safego 合规
- [x] 定价缺失时有用户可见提示
- [x] 凭据加密降级时有用户可见警告
- [x] SetCredentialKeyResolver 通过 Wire 构造函数注入（CredentialCrypto）
- [x] HA 故障转移事件在 Monitor 中可视化
- [x] Proto pricing_configured 字段已添加
- [x] Proto capabilities 字段已添加（field 15 + ModelCapabilities message）
- [x] RevealProviderModelCredentials RPC 已实现
- [x] 解密失败时有日志记录
- [x] MCPServerRepo 接口方法 ≤ 5（子接口拆分）
- [x] DecryptConfigJSONForRuntime 返回 (string, error)
- [x] RunHealthChecks goroutine 写操作不受父 ctx 取消影响
- [x] service 层 resolveCredentialPlain/ResolveSecretRef 调用签名正确
- [x] 重试 + 熔断 Transport 已实现
- [x] 前端 providerRuntimeOverlay 架构已落地

---

## 7. 依赖与风险

- HuggingFace / Bedrock Provider 依赖 trpc 上游注册到 provider 工厂
- CredentialCrypto 通过 Wire 注入，所有需要加解密的 Usecase 均需声明依赖
- IterModel 优化需 trpc-agent-go 框架支持 `model.IterModel` 接口检测
- useProviderWizard 仍 527 行，可继续拆分表单生命周期逻辑
- AgentRuntimeSettings / RalphLoopSettings 测试失败（非本次改动引起，需单独排查）

---

## 8. 演进路线

| 阶段 | 内容 | 前置依赖 | 状态 |
|------|------|----------|------|
| 当前 | Failover/Hedge 模式已可用；Provider 适配器已实现；5 种原生 Provider + 4 种 Variant；凭据加密；速率限制；重试+熔断；健康检查；HA 事件可视化 | — | ✅ |
| P1 | 接入层限流与配额统一；决策层路由引擎（能力/成本/延迟策略） | Provider 类型枚举统一 | ⏳ |
| P2 | 跨厂商降级链配置化；熔断策略 LLM 特化；负载均衡按 token 加权 | P1 | ⏳ |
| P3 | 语义缓存（可选）；全链路 trace 与成本归因；独立路由服务（可选） | P2 | ⏳ |
| P3 | IterModel 优化（支持迭代模式减少 channel 开销） | ✅ WrapModelWithMetrics 透传 | ✅ |
| P3 | HuggingFace / Bedrock Provider 启用 | ✅ RegisterExtraProviders | ✅ |

---

## 9. 涉及文件清单（按层分组）

### Proto 层
- `api/kratos/llm_provider_model/v1/llm_provider_model.proto`

### Service 层
- `internal/service/llm_provider_model.go`
- `internal/service/llm_provider_model_test.go`

### Biz 层
- `internal/biz/llm_provider_model.go`
- `internal/biz/llm_provider_model_pricing_test.go`
- `internal/biz/llm_provider_model_inspect_test.go`
- `internal/biz/credential_crypto.go`
- `internal/biz/credential_crypto_test.go`
- `internal/biz/credential_key.go`
- `internal/biz/credential_key_test.go`
- `internal/biz/channel_credential_crypto.go`

### Data 层
- `internal/data/llm_provider_model.go`
- `internal/data/ent/schema/llm_provider_model.go`
- `internal/data/ent/schema/model_pricing_rule.go`

### Provider 桥接层
- `internal/provider/trpc_llm.go`
- `internal/provider/catalog.go`
- `internal/provider/capabilities.go`
- `internal/provider/roundtrip.go`
- `internal/provider/rate_limit_transport.go`
- `internal/provider/retry_transport.go`
- `internal/provider/circuit_breaker_transport.go`
- `internal/provider/metrics_model.go`
- `internal/provider/stream_delta.go`
- `internal/provider/register_extra.go`
- `internal/provider/errors.go`
- `internal/provider/trpc_llm_options_test.go`
- `internal/provider/trpc_llm_variant_test.go`
- `internal/provider/trpc_llm_fixes_test.go`
- `internal/provider/stream_delta_test.go`

### Inspect 层
- `internal/llminspect/inspect.go`
- `internal/llminspect/inspect_pure_test.go`
- `internal/llminspect/inspect_routes_test.go`

### Agent 构建链路
- `internal/agent/trpc_build.go`
- `internal/service/session_title_llm.go`

### 前端
- `web/src/config/providerPresets.ts`
- `web/src/config/providerRuntimeOverlay.ts`
- `web/src/config/providerRuntimeOverlay.types.ts`
- `web/src/config/provider_runtime_overlay.json`
- `web/src/features/platform/types.ts`
- `web/src/features/platform/providerUtils.ts`
- `web/src/features/platform/api.ts`
- `web/src/features/platform/useProviderList.ts`
- `web/src/features/platform/useProviderCatalog.ts`
- `web/src/features/platform/useProviderCredentials.ts`
- `web/src/features/platform/useProviderInspect.ts`
- `web/src/features/platform/useProviderPresets.ts`
- `web/src/features/platform/useProviderWizard.ts`
- `web/src/features/platform/useProviderSave.ts`
- `web/src/features/platform/useResourceManagerPage.ts`
- `web/src/features/platform/usePlatformResource.ts`
- `web/src/components/platform/ProviderModelsTable.vue`
- `web/src/components/platform/ProviderTrendDialog.vue`
- `web/src/components/platform/ProviderHAConfig.vue`
- `web/src/components/platform/ProviderWizardStep1Connect.vue`
- `web/src/components/platform/ProviderWizardStep2Specs.vue`
- `web/src/components/platform/ProviderWizardStep3HA.vue`
- `web/src/components/platform/ProviderWizardStep4Advanced.vue`
- `web/src/components/platform/ProviderLogo.vue`
- `web/src/components/platform/providerModelUi.ts`
- `web/src/pages/ResourceManagerPage.vue`

### trpc-agent-go 框架包
- `pkg/trpc-agent-go/model/failover/options.go`
- `pkg/trpc-agent-go/model/hedge/options.go`
