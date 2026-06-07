# Model Catalog（models.dev 集成）

> **版本**：2026-06-06 | **状态**：✅ 核心可用（P0–P3 + Backlog 高/中/低 已完成）
> **外部数据源**：[anomalyco/models.dev](https://github.com/anomalyco/models.dev) · **唯一官方 API**：`https://models.dev/api.json`
> **关联**：[9 provider.md](./9%20provider.md) · **开发计划**：[12-model-catalog.development.md](./12-model-catalog.development.md)

---

## 1. 目标

将 [models.dev](https://models.dev) 作为 **AI 模型规格的外部真相源**，Aranea 本地缓存 JSON，用于：

- Provider / 模型添加表单的**默认值**（定价 USD/1M、context、能力）
- 定期 **sync** 更新已有 `llm_provider_models` 规格与定价
- **强制迁移** 旧 provider 命名到 models.dev id
- **Agent / Session** 绑定 provider/model 随迁移更新
- System Settings 独立 Tab：策略、手动 sync、JSON 查看、更新日志、**迁移预览与一键对齐**（无用户可编辑映射表）

**边界**：models.dev **仅提供默认参数**，不参与任何运行时业务（连接、鉴权、Turn 执行仍走 trpc-agent-go + `internal/provider`）。

> **说明**：上游不存在 `models.json` 公开端点（`/models.json` 会 302）；本地 `source_url` 白名单仅允许 `https://models.dev/api.json`。

---

## 2. 已确认决策

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

## 3. 架构

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

**代码锚点**：

| 路径 | 职责 |
|------|------|
| `api/kratos/model_catalog/v1/model_catalog.proto` | Catalog API |
| `internal/modelregistry/` | fetch、store、sync、apply、migrate、overlay、logos、chips |
| `internal/biz/model_registry.go` | Usecase + 动态 Store root |
| `internal/data/model_registry_apply.go` | ApplyBackend（事务迁移） |
| `internal/data/usage_breakdown_alias.go` | Usage 展示 alias |
| `web/src/features/model-catalog/` | 前端 API + applyCatalog |
| `web/src/pages/SystemSettingsCatalogTab.vue` | Settings Tab |
| `internal/modelregistry/runtime_overlay.json` | Go embed 运行时映射（与 web 同名 JSON 同步） |

---

## 4. models.dev 参数白名单

### 4.1 Provider 级

| models.dev 字段 | 用途 | Aranea 写入 | 状态 |
|-----------------|------|-------------|------|
| `id` | provider_code | `llm_provider_models.provider` | ✅ |
| `name` | 展示名 | `config_json.provider_display_name` | ✅ |
| `doc` | 文档 | `metadata_json.catalog_doc` | ✅ |
| `env` | 环境变量提示 | `metadata_json.catalog_env` | ✅ |
| `npm` | 参考（不驱动运行时） | `metadata_json.catalog_npm` | ✅ |
| `api` | 默认 base URL | `config_json.api_base_url`（overlay 模式） | ✅ |

### 4.2 Model 级

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

### 4.3 Aranea 独有（永不同步自 models.dev）

```
provider_type, variant, api_key, secret_*, aws_region, ha_*,
enable_token_tailoring, optimize_for_cache, rate_limit_rpm,
model_category（用户运营分类）, usage_* 统计, tokens_per_second
```

### 4.4 Runtime Overlay（Aranea 维护）

models.dev id → trpc 运行时：`provider_type`、`variant`、`auth_type`、中国区 URL 等。
**单一 JSON 源**：`internal/modelregistry/runtime_overlay.json`（Go embed）；前端 `web/src/config/provider_runtime_overlay.json` 须手动保持同步。

---

## 5. 本地存储

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

## 6. 更新策略

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

## 7. Provider 命名对齐（内置，不可编辑）

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

## 8. config_json 定价结构（USD/1M）

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

## 9. API

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/v1/model-catalog/status` | meta + policy 摘要 |
| GET | `/v1/model-catalog/policy` | 读取策略 |
| PUT | `/v1/model-catalog/policy` | 更新策略 |
| GET | `/v1/model-catalog/providers` | Provider 列表（含 logo_url / logo_cached） |
| GET | `/v1/model-catalog/providers/{id}/models` | Model 列表（含 cache/reasoning USD/1M） |
| GET | `/v1/model-catalog/raw` | 格式化 JSON |
| GET | `/v1/model-catalog/sync-logs` | 更新日志 |
| POST | `/v1/model-catalog/sync` | 手动 sync `{ dry_run }`；响应含 `apply_errors` / `apply_failed` |
| POST | `/v1/model-catalog/preview-migration` | 迁移影响预览 |
| POST | `/v1/model-catalog/apply-migration` | 立即执行内置 provider 绑定迁移 |
| GET | `/v1/model-catalog/provider-migration` | 只读内置规则 + checkpoint |
| GET | `/v1/model-catalog/logos/{provider_id}` | Provider SVG logo |

---

## 10. UI

### System Settings — Model Catalog Tab

- 状态概览、策略表单、手动 sync / dry run、JSON 查看器、同步日志
- Provider 迁移预览 + **立即对齐**（只读内置规则列表）

### Resource Manager

- **目录模式**：选 Provider → 选 Model → 自动填 config（USD/1M、chips）
- **自定义模式**：Ollama / 本地，`catalog_source=custom`
- 列表行：`ProviderLogo` + capability chips

---

## 11. 验收标准

- [x] System Settings 有 Model Catalog Tab，可 sync、看 JSON、看日志、配策略
- [x] 本地 `current.json` 在 sync 后更新；ETag 304 保留旧缓存；断网可用旧缓存
- [x] 目录模式选模型自动填 USD/1M 定价与 capability_chips
- [x] 自定义模式可添加 Ollama 本地模型
- [x] sync 后 deprecated 模型 auto disabled（catalog_managed）
- [x] 强制迁移更新 agents / sessions / eval 等绑定（事务）
- [x] Usage 报表 alias 合并旧 provider 维度（Top / 筛选 / 事件展示）
- [x] Provider logo 本地缓存与 API 展示
- [x] Usage **计费内核** USD/1M 优先（`ApplyTokenUsageCosts` → `usageCostMicro` 优先 USD/1M；micro/1K 仅 legacy fallback）
- [x] `interleaved` 字段 merge（`applyInterleavedHints` → `interleaved` / `interleaved_field` / `reasoning_content_backfill`）

---

## 12. 已完成（P0–P3，2026-05-25）

| 优先级 | 项 | 说明 |
|--------|-----|------|
| P0 | metadata merge | `catalog_env`、`release_date`、`last_updated` 写入 metadata_json |
| P0 | Usage alias 扩展 | summary/trends 按 provider 筛选含 legacy；events 展示 alias；Top 合并 |
| P0 | **迁移内置化** | 移除 Settings 可编辑 `migration-map`；`ProviderMigration` embed 单源；Usage alias 与绑定迁移共用 |
| P1 | Runtime overlay 统一 | `runtime_overlay.json` Go embed + 前端 JSON（含 huggingface） |
| P1 | Logo 404 回退 | sync 拉取失败 → `default.svg`；API 无缓存读 default |
| P1 | ListCatalogModels 定价 | proto 暴露 cache_read/write、reasoning、structured_output 等；前端 chips 对齐 |
| P1 | **apply 失败可见** | sync 响应 `apply_errors`；scheduled 日志 `partial`；UI warning |
| P1 | **手动对齐 API** | `POST apply-migration` + Settings「立即对齐」 |
| P2 | urlguard | 仅允许 `/api.json`；拒绝 `/models.json` |
| P2 | 集成测试 | apply_test、sync 304_test、usage alias_test、sqlite 迁移_test |
| P2 | **migration-checkpoint** | 替代可写 `migration-map.json`；记录 `applied_at` + `version` |
| P3 | ETag 条件 GET | `If-None-Match` + 304 跳过 catalog 写入 |
| P3 | **canonical_provider_code** | usage 事件写入时冗余 canonical id；读侧优先该列 |

**同期已完成（Review 修复）**：动态 Store root、SSRF、sync 互斥、custom skip、cache_write 进 cost block、apply 失败 surfacing、事务迁移、migration 预览多表计数、`system_settings.update_time` 迁移 SQL 修复。

---

## 13. 待优化（Backlog）

按 **影响 × 工作量** 排序，供下一轮迭代。

### 已完成（高 + 中优先级，2026-05）

| # | 项 | 实现要点 |
|---|-----|----------|
| 1 | **`cache_write` 定价落库** | `model_pricing_rules` + usage 事件增列；UpsertModelPricing / ApplyTokenUsageCosts / enrichPricing |
| 2 | **`interleaved` 字段** | `Model.Interleaved` + `applyInterleavedHints()` → `interleaved` / `interleaved_field` / `reasoning_content_backfill` |
| 3 | **Usage 计费 USD/1M 内核** | pricing snapshot 双写 USD + micro；`ApplyTokenUsageCosts` 优先 USD/1M（micro 仍保留双读 fallback） |
| 4 | **Runtime overlay 双文件** | `make check-overlay` + `TestRuntimeOverlayMatchesWebCopy` |
| 5 | **目录选模 vision chip** | proto `modality_input`/`modality_output`；`applyCatalog.ts` buildCapabilityChips |
| 6 | **migration-map UI** | ~~已删除~~ → 只读内置规则 + 一键对齐 |
| 7 | **Logo proactive default** | sync 开始时拉取 `logos/default.svg` |
| 8 | **Resource Manager env 提示** | `catalogProviderHint` + `catalog_env` 横幅 |

### 已完成（低优先级，2026-05）

| # | 项 | 实现要点 |
|---|-----|----------|
| 10 | **OpenAPI WARN** | `PreviewMigration` POST 增加 `body: "*"` |
| 11 | **大 catalog 性能** | `offset` 分页 + `SearchCatalogRaw` 服务端 JSON 搜索；Settings Tab 不再全量拉 raw |
| 12 | **Sync 重试** | `fetchCatalogWithRetry` 指数退避（最多 4 次，503/5xx/网络错误） |
| 13 | **Provider 文档链接** | Resource Manager / Settings 目录浏览可点击 doc ↗ |
| 3+ | **Usage micro 主路径** | `ApplyTokenUsageCosts` 优先 USD/1M；micro 仅 legacy 回退 |

### 低优先级（剩余）

| # | 项 | 现状 | 建议 |
|---|-----|------|------|
| 9 | **测试覆盖** | 单元 + sqlite；无 E2E sync→apply→RM | enttest + 前端 smoke |
| 14 | **Settings Tab env 展示** | 数据已映射（`catalogWire.ts`），但 Settings Provider 浏览列表未渲染 `env` 字段 | 在 Provider item 中展示环境变量 |
| 15 | **Usage 事件表 USD/1M 列** | `TokenUsageEvent` struct 有 USD/1M 字段，但 `model_token_usage_events` 表仅存 micro 列，USD/1M 价格落盘后丢失 | events 表追加 `*_price_usd_per_1m` 列 |

### 已知限制（文档化，非必改）

- Usage **历史 DB 不 rewrite** `provider_code`；新行写 `canonical_provider_code`；读侧 alias 合并展示
- `catalog_managed=false` / custom 永不 auto_apply
- models.dev **logo 不在 JSON 内**，独立 `https://models.dev/logos/{id}.svg`
- 修改 wire / proto 后须 **`make api` + 重启 admin**
- `model_token_usage_events` 表仅存 micro_usd 列，USD/1M 价格不持久化（计算时从 pricing_rules 实时读取）

---

## 14. 验证

```bash
make api && make wire-admin && go build ./cmd/admin
go test ./internal/modelregistry/... ./internal/data/ -run 'Catalog|Migration|Usage|Merge' -count=1
```

手动：System Settings → Model Catalog → 立即同步 → 迁移预览 → **立即对齐** → Resource Manager 目录选模。
