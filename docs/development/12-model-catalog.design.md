# Model Catalog — 设计文档

> **版本**：2026-06-17 | **状态**：✅ 核心可用
> **需求**：[12 model-catalog.md](./12-model-catalog.md) · **开发计划**：[12-model-catalog.development.md](./12-model-catalog.development.md)
> **外部数据源**：[anomalyco/models.dev](https://github.com/anomalyco/models.dev) · **唯一官方 API**：`https://models.dev/api.json`
> **关联**：[9 provider.md](./9%20provider.md)

---

## 1. 已确认决策

| 项 | 决策 |
|----|------|
| Provider 命名 | 使用 models.dev `provider.id`（如 `google`、`alibaba-cn`） |
| 定价单位 | **USD / 1M tokens**，与 models.dev 一致；config_json 与 pricing_rules 双写 USD；Usage 计费内核 USD/1M 优先（micro/1K 仅 legacy fallback） |
| Usage 历史 | DB 历史 `provider_code` **不 rewrite**；新事件写入 `canonical_provider_code`；报表/筛选读侧 alias（含 legacy 扩展） |
| deprecated | catalog `status=deprecated` → 自动 `enabled=false`（`catalog_managed` 记录） |
| catalog 无条目 | **自定义 Provider/模型** 路径，面向 Ollama / 本地 OpenAI 兼容部署 |
| 自动更新 DB | sync 后按策略 merge 已有 Provider 规格与定价；失败 surfacing 到 sync 响应 |
| Provider 迁移 | **内置** `ProviderMigration`（Go embed，随版本发布）；sync / 手动 **自动事务迁移**；`migration-checkpoint.json` 仅记录进度 |
| Provider Logo | 同步时缓存至 `logos/{id}.svg`；404 回退 `default.svg`；API `GET /v1/model-catalog/logos/{id}` |

---

## 2. 架构

```
models.dev/api.json
        ↓ sync（ETag 304 / scheduled / manual）
{root}/data/model-catalog/
  current.json + current.meta.json + policy.json + migration-checkpoint.json + logos/*.svg
        ↓
ModelCatalogService API
        ├→ System Settings Tab（策略 / 日志 / JSON / 迁移预览 / 一键对齐）
        ├→ Resource Manager（目录 / 自定义 → 填默认值）
        └→ Sync Applier（merge DB / 迁移 / deprecated / pricing_rules）
```

**边界**：models.dev **仅提供默认参数**，不参与任何运行时业务（连接、鉴权、Turn 执行仍走 trpc-agent-go + `internal/provider`）。

> **说明**：上游不存在 `models.json` 公开端点（`/models.json` 会 302）；本地 `source_url` 白名单仅允许 `https://models.dev/api.json`。

**代码锚点**：

| 路径 | 职责 |
|------|------|
| `api/kratos/model_catalog/v1/model_catalog.proto` | Catalog API |
| `internal/modelregistry/` | fetch、store、sync、apply、migrate、overlay、logos、chips |
| `internal/biz/model_registry.go` | Usecase + 动态 Store root |
| `internal/data/model_registry_apply.go` | ApplyBackend（事务迁移） |
| `internal/data/usage_write.go` / `internal/data/usage.go` | Usage 展示 alias + `canonical_provider_code` |
| `internal/service/model_catalog.go` | gRPC/HTTP 服务实现 |
| `web/src/features/model-catalog/` | 前端 API + applyCatalog + providerLogo + catalogCategories + providerMigration |
| `web/src/pages/SystemSettingsCatalogTab.vue` | Settings Tab |
| `internal/modelregistry/runtime_overlay.json` | Go embed 运行时映射（与 web 同名 JSON 同步） |
| `internal/modelregistry/overlay.go` | `ProviderMigration` embed 真相源 |

---

## 3. models.dev 参数白名单

### 3.1 Provider 级

| models.dev 字段 | 用途 | Aranea 写入 | 状态 |
|-----------------|------|-------------|------|
| `id` | provider_code | `llm_provider_models.provider` | ✅ |
| `name` | 展示名 | `config_json.provider_display_name` | ✅ |
| `doc` | 文档 | `metadata_json.catalog_doc` | ✅ |
| `env` | 环境变量提示 | `metadata_json.catalog_env` | ✅ |
| `npm` | 参考（不驱动运行时） | `metadata_json.catalog_npm` | ✅ |
| `api` | 默认 base URL | `config_json.api_base_url`（overlay 模式） | ✅ |

### 3.2 Model 级

| models.dev 字段 | 用途 | Aranea 写入 | 状态 |
|-----------------|------|-------------|------|
| `id` | model_api_id | `llm_provider_models.model` | ✅ |
| `name` | 展示名 | `name` / `config_json.model_display_name` | ✅ |
| `family` | 型号族 | `metadata_json.catalog_family` | ✅ |
| `release_date` / `last_updated` | 元信息 | `metadata_json` | ✅ |
| `knowledge` | 知识截止 | `metadata_json.catalog_knowledge` | ✅ |
| `status` | alpha/beta/deprecated | `metadata_json.catalog_status` + chip | ✅ |
| `attachment` / `reasoning` / `tool_call` 等 | 能力 chip | `config_json.capability_chips[]` | ✅ |
| `structured_output` / `temperature` / `open_weights` | 能力 chip | 同上 | ✅ |
| `interleaved` | 推理格式 hint | `config_json` 推理相关 hint（`interleaved` / `interleaved_field` / `reasoning_content_backfill`） | ✅ |
| `cost.*` | **USD/1M** | `config_json.cost.*_usd_per_1m` | ✅（含 cache_read/write） |
| `limit.*` | token 限制 | `config_json.limit.*_tokens` | ✅ |
| `modalities.*` | 模态 | `metadata_json.catalog_modalities`；vision chip（从 modality_input/output 推导） | ✅ |

### 3.3 Aranea 独有（永不同步自 models.dev）

```
provider_type, variant, api_key, secret_*, aws_region, ha_*,
enable_token_tailoring, optimize_for_cache, rate_limit_rpm,
model_category（用户运营分类）, usage_* 统计, tokens_per_second
```

### 3.4 Runtime Overlay（Aranea 维护）

models.dev id → trpc 运行时：`provider_type`、`variant`、`auth_type`、中国区 URL 等。
**单一 JSON 源**：`internal/modelregistry/runtime_overlay.json`（Go embed）；前端 `web/src/config/provider_runtime_overlay.json` 须手动保持同步。

---

## 4. 本地存储

```
{root_directory}/data/model-catalog/
  policy.json           # 同步策略（source_url, sync_policy, interval, auto_apply）
  current.json          # 完整 catalog（models.dev api.json 结构）
  current.meta.json     # synced_at, etag, sha256, counts
  sync-logs.jsonl       # 追加式日志（apply 失败时 status=partial）
  migration-checkpoint.json  # 上次成功迁移时间戳 + 版本（非用户配置）
  logos/                # {provider_id}.svg（sync 时拉取；失败用 default.svg）
```

---

## 5. 更新策略

| sync_policy | 行为 |
|-------------|------|
| `off` | 不自动 sync |
| `scheduled` | 每 `sync_interval_hours`（默认 24）拉取；带 **If-None-Match**，304 跳过写入 |

| auto_apply | 行为 |
|------------|------|
| `none` | 只更新本地 JSON + logos |
| `metadata_and_pricing` | 默认：merge 规格+定价；不覆盖密钥/HA/用户 category |
| `full_spec` | 含 context/limit/modality/capability_chips |
| `full_spec_and_runtime_overlay` | 含 catalog.api / overlay 的 base URL（不覆盖用户改过的 URL） |

### deprecated / custom

- `catalog_managed=true` 且 `status=deprecated` → `enabled=false`
- `catalog_source=custom` 或 `catalog_managed=false` → 跳过 auto_apply

---

## 6. Provider 命名对齐（内置，不可编辑）

| 旧 provider_code | models.dev id |
|------------------|---------------|
| `aliyun-qwen` | `alibaba-cn` |
| `tencent-hunyuan` | `hunyuan` |
| `moonshot-kimi` | `moonshotai-cn` |
| `zhipu-glm` | `zhipuai` |
| `gemini` | `google` |
| `custom` | 不迁移 |

**真相源**：`internal/modelregistry/overlay.go` → `ProviderMigration`（与 `runtime_overlay.json` 同级，随发版更新）。

**写入侧**：sync 的 `auto_apply` 或 `POST /v1/model-catalog/apply-migration` 事务更新 agents / sessions / eval / runtime / skills / knowledge_embed / web_research / llm 行。

**读取侧（Usage）**：历史行保留原 `provider_code`；新事件额外写入 `canonical_provider_code`；报表 Top/筛选/事件展示走同一套 `MigrateProviderCode` alias。

**不提供** Settings 映射表编辑；`migration-map.json` 已废弃（若存在则忽略）。

---

## 7. config_json 定价结构（USD/1M）

```json
{
  "cost": {
    "input_usd_per_1m": 3.0,
    "output_usd_per_1m": 15.0,
    "cache_read_usd_per_1m": 0.3,
    "cache_write_usd_per_1m": 3.75,
    "reasoning_usd_per_1m": 15.0,
    "embedding_usd_per_1m": 0
  },
  "limit": { "context_tokens": 1000000, "output_tokens": 64000 },
  "capability_chips": [{ "key": "tool_call", "label": "工具调用", "source": "catalog" }],
  "catalog_managed": true,
  "catalog_source": "models.dev"
}
```

Usage 计费：`costUSD = tokens × priceUSDPer1M / 1_000_000`；计费内核 `ApplyTokenUsageCosts` 已以 USD/1M 为主路径（`usageCostMicro` 优先 USD/1M，micro/1K 仅 legacy fallback）；`model_pricing_rules` 双写 USD/1M + micro/1K 列；事件表仍存 **micro-USD** 整数。

---

## 8. API 契约

> 真相源：`api/kratos/model_catalog/v1/model_catalog.proto`

| 方法 | 路径 | RPC | 说明 |
|------|------|-----|------|
| GET | `/v1/model-catalog/status` | `GetModelCatalogStatus` | meta + policy 摘要 |
| GET | `/v1/model-catalog/policy` | `GetModelCatalogPolicy` | 读取策略 |
| PUT | `/v1/model-catalog/policy` | `UpdateModelCatalogPolicy` | 更新策略 |
| GET | `/v1/model-catalog/providers` | `ListCatalogProviders` | Provider 列表（含 logo_url / logo_cached） |
| GET | `/v1/model-catalog/providers/{provider_id}/models` | `ListCatalogModels` | Model 列表（含 cache/reasoning USD/1M） |
| GET | `/v1/model-catalog/raw` | `GetModelCatalogRaw` | 格式化 JSON |
| GET | `/v1/model-catalog/raw/search` | `SearchCatalogRaw` | 服务端 JSON 搜索（分页） |
| GET | `/v1/model-catalog/sync-logs` | `ListModelCatalogSyncLogs` | 更新日志 |
| POST | `/v1/model-catalog/sync` | `SyncModelCatalog` | 手动 sync `{ dry_run }`；响应含 `apply_errors` / `apply_failed` |
| POST | `/v1/model-catalog/preview-migration` | `PreviewMigration` | 迁移影响预览 |
| POST | `/v1/model-catalog/apply-migration` | `ApplyProviderMigration` | 立即执行内置 provider 绑定迁移 |
| GET | `/v1/model-catalog/provider-migration` | `GetProviderMigrationRules` | 只读内置规则 + checkpoint |
| GET | `/v1/model-catalog/logos/{provider_id}` | `GetCatalogProviderLogo` | Provider SVG logo |

---

## 9. UI 设计

### System Settings — Model Catalog Tab

- 状态概览、策略表单、手动 sync / dry run、JSON 查看器、同步日志
- Provider 迁移预览 + **立即对齐**（只读内置规则列表）

### Resource Manager

- **目录模式**：选 Provider → 选 Model → 自动填 config（USD/1M、chips）
- **自定义模式**：Ollama / 本地，`catalog_source=custom`
- 列表行：`ProviderLogo` + capability chips

---

## 10. 已知限制（文档化，非必改）

- Usage **历史 DB 不 rewrite** `provider_code`；新行写 `canonical_provider_code`；读侧 alias 合并展示
- `catalog_managed=false` / custom 永不 auto_apply
- models.dev **logo 不在 JSON 内**，独立 `https://models.dev/logos/{id}.svg`
- 修改 wire / proto 后须 **`make api` + 重启 admin**
- `model_token_usage_events` 表仅存 micro_usd 列，USD/1M 价格不持久化（计算时从 pricing_rules 实时读取）
