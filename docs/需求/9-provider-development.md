# Provider — 开发计划

> **版本**：2026-05-17 | **状态**：✅ 端到端可用（核心链路）
> **需求**：[9 provider.md](./9%20provider.md) · **设计**：[9 provider.design.md](./9%20provider.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：—

---

## 1. 模块定位

Provider 管理：基于 trpc-agent-go `model` 体系的多厂商 LLM Provider 管理。支持 5 种原生 Provider（OpenAI / Anthropic / Gemini / Ollama / Hunyuan）+ 4 种 OpenAI Variant（OpenAI / DeepSeek / Qwen / Hunyuan）+ Failover/Hedge 高可用模式 + TokenTailor 自动裁剪 + Inspect 远程元数据探测 + Pricing 定价规则。

**代码锚点**：
- `api/kratos/llm_provider_model/v1/llm_provider_model.proto` — Proto CRUD + Inspect + ValidatePair
- `internal/service/llm_provider_model.go` — LlmProviderModelService
- `internal/biz/llm_provider_model.go` — LlmProviderModelUsecase + InspectMerge + ModelPricingRule
- `internal/data/llm_provider_model.go` — LlmProviderModelRepo（Ent ORM + SQLite）
- `internal/data/ent/schema/llm_provider_model.go` — Ent Schema
- `internal/data/ent/schema/model_pricing_rule.go` — 定价规则 Schema
- `internal/provider/trpc_llm.go` — Provider → trpc Model 装配（含 HA）
- `internal/provider/catalog.go` — CatalogConfig 解析与合并
- `internal/provider/roundtrip.go` — HTTP Transport 注入
- `internal/provider/stream_delta.go` — 流式 Delta 合并
- `internal/llminspect/inspect.go` — 远程模型元数据探测
- `internal/agent/trpc_build.go` — Agent 构建时调用 TRPCModelForProviderModel
- `internal/service/session_title_llm.go` — Session 标题生成调用 TRPCModelForProviderModel
- `web/src/config/providerPresets.ts` — 前端 Provider 预设（20 个厂商）
- `web/src/components/platform/ProviderModelRow.vue` — 列表行组件
- `web/src/components/platform/ProviderTrendDialog.vue` — 趋势看板组件
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
| Failover/Hedge HA | ✅ | `wrapHA` + `wrapFailover` / `wrapHedge`；候选模型构建 |
| TokenTailoring | ✅ | `WithEnableTokenTailoring` 透传 |
| Provider 专属选项 | ✅ | OpenAI（Cache/Backfill/Delta）、Anthropic（Cache 三项）、Gemini（ClientConfig）、Ollama（KeepAlive）、Hunyuan（SecretId/Key） |
| Pricing 定价规则 | ✅ | `UpsertModelPricingRule`；Create/Update 时自动同步 |
| 前端预设 | ✅ | 20 个 Provider 预设；7 种 ProviderType；4 种 Variant；AuthType 联动 |
| 前端列表行 | ✅ | ProviderModelRow：类型 Chip、分类 Chip、热度、用量、密钥状态、Toggle |
| 前端趋势看板 | ✅ | ProviderTrendDialog：30 天趋势柱状图、汇总卡片、详情表 |
| 前端管理页面 | ✅ | ResourceManagerPage：搜索、分页、创建/编辑弹窗 |
| Inspect 请求扩展字段 | ⏳ | Proto 缺 variant / secret_id / secret_key / aws_region；biz InspectMerge 缺同字段 |
| Inspect 响应扩展字段 | ⏳ | Proto 缺 variant / enable_token_tailoring / supports_cache / supports_thinking |
| mergeInspectConfigJSON 扩展 | ⏳ | 当前仅合并 provider_type / api_base_url / api_key，缺 variant / secret_id / secret_key / aws_region |
| 前端 Variant Chip | ⏳ | ProviderModelRow 未展示 Variant Chip |
| 前端 HA Chip | ⏳ | ProviderModelRow 未展示 Failover/Hedge Chip |
| 前端四步表单 | ⏳ | 当前为单弹窗表单，非设计文档 §6 的四步表单 |
| Gemini/Ollama/Hunyuan Inspect | ⏳ | llminspect 仅支持 OpenRouter / OpenAI-Compatible / Anthropic 三条路径 |
| HuggingFace/Bedrock Provider | ⏳ | trpc 上游未注册到 provider 工厂；前端预设已预留 |
| 凭据加密 | ⏳ | api_key 明文存 SQLite config_json |

---

## 3. 差距与优化

1. **P1**：Inspect 请求/响应扩展字段（variant / secret_id / secret_key / aws_region / enable_token_tailoring / supports_cache / supports_thinking）未添加到 Proto 和 biz 层，导致 Hunyuan 和 Bedrock 的 Inspect 无法传递认证信息。
2. **P1**：`mergeInspectConfigJSON` 仅合并 3 个字段，Hunyuan（secret_id/secret_key）和 Bedrock（aws_region）的 Inspect 无法从已保存配置中回填。
3. **P2**：前端 ProviderModelRow 缺少 Variant Chip 和 HA Chip 展示，用户无法直观区分 DeepSeek/Qwen Variant 和高可用模式。
4. **P2**：前端添加/编辑弹窗为单步表单，设计文档 §6 要求四步表单（连接与身份 → 模型分类与规格 → 高可用配置 → 高级选项）。
5. **P2**：llminspect 缺少 Gemini / Ollama / Hunyuan 专属探测路径，这三类 Provider 的 Inspect 会走 OpenAI-Compatible 兜底（可能失败）。
6. **P2**：凭据未加密存储（api_key 明文存 SQLite config_json），需 AES-256-GCM 加密或使用 vault。
7. **P3**：Provider 速率限制（rate_limit）配置未实现，无法在 Agent 运行时限制调用频率。
8. **P3**：Provider 健康检查定时任务未实现，仅手动触发 Inspect。

---

## 4. 开发阶段

- **Phase 1**：Inspect 扩展字段（Proto + biz + service + 前端）— 解锁 Hunyuan/Bedrock Inspect
- **Phase 2**：前端 UI 增强（Variant Chip + HA Chip + 四步表单）
- **Phase 3**：llminspect 路径扩展（Gemini / Ollama / Hunyuan 专属探测）
- **Phase 4**：凭据加密存储（AES-256-GCM）
- **Phase 5**：速率限制 + 健康检查定时任务

---

## 5. 任务清单

| # | 任务 | 优先级 | EP | 代码位置 |
|---|------|--------|-----|---------|
| 1 | Inspect 请求扩展：Proto 增加 variant/secret_id/secret_key/aws_region | P1 | — | `api/kratos/llm_provider_model/v1/` |
| 2 | Inspect 响应扩展：Proto 增加 variant/enable_token_tailoring/supports_cache/supports_thinking | P1 | — | `api/kratos/llm_provider_model/v1/` |
| 3 | biz InspectMerge 增加 Variant/SecretID/SecretKey/AWSRegion 字段 | P1 | — | `internal/biz/llm_provider_model.go` |
| 4 | mergeInspectConfigJSON 扩展：合并 variant/secret_id/secret_key/aws_region | P1 | — | `internal/biz/llm_provider_model.go` |
| 5 | service Inspect 映射新字段 | P1 | — | `internal/service/llm_provider_model.go` |
| 6 | 前端 ProviderModelRow 增加 Variant Chip | P2 | — | `web/src/components/platform/ProviderModelRow.vue` |
| 7 | 前端 ProviderModelRow 增加 HA Chip（Failover/Hedge） | P2 | — | `web/src/components/platform/ProviderModelRow.vue` |
| 8 | 前端添加/编辑弹窗改为四步表单 | P2 | — | `web/src/pages/ResourceManagerPage.vue` |
| 9 | llminspect 增加 Gemini 专属探测路径 | P2 | — | `internal/llminspect/inspect.go` |
| 10 | llminspect 增加 Ollama 专属探测路径 | P2 | — | `internal/llminspect/inspect.go` |
| 11 | llminspect 增加 Hunyuan 专属探测路径 | P2 | — | `internal/llminspect/inspect.go` |
| 12 | 凭据加密：AES-256-GCM 加密 api_key | P2 | — | `internal/biz/llm_provider_model.go` + `internal/data/` |
| 13 | 速率限制：provider 表增加 rate_limit 字段 + 运行时限流 | P3 | — | `internal/provider/trpc_llm.go` |
| 14 | 健康检查：5min ticker + safego | P3 | — | 新建 `internal/provider/health.go` |

---

## 6. 验收标准

- [ ] Inspect 请求/响应包含 variant / secret_id / secret_key / aws_region 等扩展字段
- [ ] Hunyuan Provider Inspect 能从已保存配置回填 SecretId/SecretKey
- [ ] 前端列表行展示 Variant Chip 和 HA Chip
- [ ] 前端添加/编辑弹窗为四步表单
- [ ] Gemini / Ollama / Hunyuan Inspect 有专属探测路径
- [ ] api_key 在数据库中为密文
- [ ] 速率限制在 Agent 运行时生效
- [ ] Provider 异常时自动标记状态

---

## 7. 依赖与风险

- Inspect 扩展字段需同步更新 Proto（`make api`）和前端类型
- 凭据加密需迁移现有明文数据
- 速率限制需与 trpc-agent-go LLM 调用链集成
- HuggingFace / Bedrock Provider 依赖 trpc 上游注册到 provider 工厂
- llminspect Gemini / Ollama / Hunyuan 专属探测需研究各厂商 API 规范
