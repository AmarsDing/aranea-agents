# 模型与供应商相关数据库定义（SQLite）

本文档与 **`pkg/backend/internal/repository/migrations/0001_init.sql`** 及启动时的 **`ensureLegacyColumns`** 对齐，并补充与 **`pkg/docs/9 provider.md`** 的产品字段映射说明。

---

## 1. `llm_provider_models`

平台资源表，HTTP API 资源名为 `llm-provider-models`。一行表示一个「供应商 + 模型 API ID」组合。

### 1.1 基线 DDL（迁移内）

```sql
CREATE TABLE IF NOT EXISTS llm_provider_models (
  id TEXT PRIMARY KEY,
  model_key TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'active',
  enabled INTEGER NOT NULL DEFAULT 1,
  sort_order INTEGER NOT NULL DEFAULT 0,
  provider TEXT NOT NULL DEFAULT '',
  model TEXT NOT NULL DEFAULT '',
  config_json TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  deleted_at TEXT NOT NULL DEFAULT ''
);
```

### 1.2 遗留列补齐（`sqlite.go` → `ensureLegacyColumns`）

新库在首次迁移后会通过 `ALTER TABLE` 追加（若尚不存在）：

| 列名 | SQLite 类型片段 |
|------|------------------|
| `parent_id` | `TEXT NOT NULL DEFAULT ''` |
| `level` | `TEXT NOT NULL DEFAULT ''` |
| `agent_id` | `TEXT NOT NULL DEFAULT ''` |

### 1.3 索引（迁移内）

```sql
CREATE INDEX IF NOT EXISTS idx_provider_models_provider ON llm_provider_models(provider, enabled, sort_order);
```

### 1.4 列语义与产品文档对照

| 列名 | 后端 / 域模型 | 与 `9 provider.md` 概念对应 |
|------|----------------|---------------------------|
| `id` | 资源主键 | 行 ID（UUID 等） |
| `model_key` | 全局唯一业务键 | 常为 `provider:model_api_id` 形式（种子数据如此） |
| `name` | 展示名称 | 对应 UI「名称」与部分 `model_display_name` |
| `description` | 描述 | 可选说明 |
| `status` | 如 `active` | 生命周期状态（与 `enabled` 并存） |
| `enabled` | `1` / `0` | 对应 `is_enabled` |
| `sort_order` | 整数 | 列表排序 |
| `provider` | **供应商代码** | **`provider_code`**（小写 slug） |
| `model` | **模型 API ID** | **`model_api_id`** |
| `config_json` | 连接与扩展配置 JSON | 见 §2 |
| `metadata_json` | 元数据 JSON | 健康状态、自检结果等扩展 |
| `created_at` / `updated_at` / `deleted_at` | ISO 文本时间、软删 | 与通用平台资源一致 |

**唯一约束（迁移层）**：当前基线为 **`model_key` UNIQUE**。产品文档建议的 **`(provider_code, model_api_id)`** 唯一需通过 **`model_key`** 约定或后续迁移加强（例如唯一索引 `(provider, model) WHERE deleted_at = ''`）。

**连接信息批量一致**：产品要求同一 `provider_code` 下多行共享 `api_base_url` / 密钥；实现上应在更新时按 `provider` 批量同步 `config_json` 中相关字段（见 `9 provider.md` §5）。

---

## 2. `llm_provider_models.config_json` / `metadata_json` 约定

后端代码中已读取的字段（非穷举，可与产品扩展合并进同一 JSON）：

**连接与类型（自检、对话适配）**

| JSON 字段 | 说明 |
|-----------|------|
| `provider_type` | 厂商类型，如 OpenAI Compatible、Anthropic |
| `api_base_url` | API 基础 URL |
| `api_key` | API 密钥（明文存于本地 SQLite，部署需加固） |

**计价（写入后同步 `model_pricing_rules`）**

| JSON 字段 | 说明 |
|-----------|------|
| `input_price_micro_usd_per_1k` | 输入价（微美元 / 1K tokens） |
| `output_price_micro_usd_per_1k` | 输出价 |
| `cached_input_price_micro_usd_per_1k` | 缓存输入价 |
| `reasoning_price_micro_usd_per_1k` | 推理 token 价 |
| `embedding_price_micro_usd_per_1k` | 嵌入价 |

**产品文档 `9 provider.md` 建议、尚未拆成独立列的字段**（可落在 `config_json` 或 `metadata_json`，待迁移归一）：

- `provider_display_name`
- `model_display_name`（可与 `name` 重复时以列为准）
- `model_category`：对象数组 `[{ "value", "label", "tooltip" }]`
- `model_size_label`、`context_window_k`、`max_output_tokens`
- `model_rating`（1～100）、`tokens_per_second`、`model_hotness_score`
- 近 30 天统计：`usage_call_count_30d`、`usage_total_tokens_30d`、`usage_cost_micro_usd_30d`、`success_rate_30d`、`avg_latency_ms_30d`、`last_used_at`

统计类字段亦可由 **`model_token_usage_events` / `model_token_usage_daily`** 聚合后回写快照（与产品 §5 描述一致）。

---

## 3. `model_token_usage_events`

单次模型调用（或可归一为一次计费单元）的明细，用于趋势、热度与成本分析。

```sql
CREATE TABLE IF NOT EXISTS model_token_usage_events (
  id TEXT PRIMARY KEY,
  occurred_at TEXT NOT NULL,
  date_key TEXT NOT NULL,
  hour_key TEXT NOT NULL,
  workspace_id TEXT NOT NULL DEFAULT '',
  user_id TEXT NOT NULL DEFAULT '',
  team_id TEXT NOT NULL DEFAULT '',
  agent_id TEXT NOT NULL DEFAULT '',
  agent_key TEXT NOT NULL DEFAULT '',
  session_id TEXT NOT NULL DEFAULT '',
  message_id TEXT NOT NULL DEFAULT '',
  request_id TEXT NOT NULL DEFAULT '',
  provider_code TEXT NOT NULL DEFAULT '',
  provider_type TEXT NOT NULL DEFAULT '',
  provider_display_name TEXT NOT NULL DEFAULT '',
  model_api_id TEXT NOT NULL DEFAULT '',
  model_display_name TEXT NOT NULL DEFAULT '',
  model_category_json TEXT NOT NULL DEFAULT '[]',
  usage_kind TEXT NOT NULL DEFAULT 'chat',
  call_count INTEGER NOT NULL DEFAULT 1,
  input_tokens INTEGER NOT NULL DEFAULT 0,
  output_tokens INTEGER NOT NULL DEFAULT 0,
  cached_input_tokens INTEGER NOT NULL DEFAULT 0,
  reasoning_tokens INTEGER NOT NULL DEFAULT 0,
  embedding_tokens INTEGER NOT NULL DEFAULT 0,
  total_tokens INTEGER NOT NULL DEFAULT 0,
  input_price_micro_usd_per_1k INTEGER NOT NULL DEFAULT 0,
  output_price_micro_usd_per_1k INTEGER NOT NULL DEFAULT 0,
  cached_input_price_micro_usd_per_1k INTEGER NOT NULL DEFAULT 0,
  reasoning_price_micro_usd_per_1k INTEGER NOT NULL DEFAULT 0,
  embedding_price_micro_usd_per_1k INTEGER NOT NULL DEFAULT 0,
  input_cost_micro_usd INTEGER NOT NULL DEFAULT 0,
  output_cost_micro_usd INTEGER NOT NULL DEFAULT 0,
  cached_input_cost_micro_usd INTEGER NOT NULL DEFAULT 0,
  reasoning_cost_micro_usd INTEGER NOT NULL DEFAULT 0,
  embedding_cost_micro_usd INTEGER NOT NULL DEFAULT 0,
  total_cost_micro_usd INTEGER NOT NULL DEFAULT 0,
  latency_ms INTEGER NOT NULL DEFAULT 0,
  time_to_first_token_ms INTEGER NOT NULL DEFAULT 0,
  tokens_per_second REAL NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'success',
  error_code TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '',
  retry_count INTEGER NOT NULL DEFAULT 0,
  prompt_mode TEXT NOT NULL DEFAULT '',
  max_output_tokens INTEGER NOT NULL DEFAULT 0,
  context_window_k INTEGER NOT NULL DEFAULT 0,
  stream_enabled INTEGER NOT NULL DEFAULT 0,
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL
);
```

**索引（迁移内节选）**

```sql
CREATE INDEX IF NOT EXISTS idx_usage_events_time ON model_token_usage_events(occurred_at);
CREATE INDEX IF NOT EXISTS idx_usage_events_date_model ON model_token_usage_events(date_key, provider_code, model_api_id);
CREATE INDEX IF NOT EXISTS idx_usage_events_agent_time ON model_token_usage_events(agent_id, occurred_at);
CREATE INDEX IF NOT EXISTS idx_usage_events_session ON model_token_usage_events(session_id);
CREATE INDEX IF NOT EXISTS idx_usage_events_status ON model_token_usage_events(status, occurred_at);
```

域模型参考：`domain.ModelTokenUsageEvent`（`pkg/backend/internal/domain/models.go`）。

---

## 4. `model_token_usage_daily`

按日、工作区、Agent、供应商、模型、`usage_kind` 聚合的汇总表。

```sql
CREATE TABLE IF NOT EXISTS model_token_usage_daily (
  id TEXT PRIMARY KEY,
  date_key TEXT NOT NULL,
  workspace_id TEXT NOT NULL DEFAULT '',
  agent_id TEXT NOT NULL DEFAULT '',
  agent_key TEXT NOT NULL DEFAULT '',
  provider_code TEXT NOT NULL DEFAULT '',
  model_api_id TEXT NOT NULL DEFAULT '',
  usage_kind TEXT NOT NULL DEFAULT 'chat',
  call_count INTEGER NOT NULL DEFAULT 0,
  request_count INTEGER NOT NULL DEFAULT 0,
  success_count INTEGER NOT NULL DEFAULT 0,
  failed_count INTEGER NOT NULL DEFAULT 0,
  cancelled_count INTEGER NOT NULL DEFAULT 0,
  input_tokens INTEGER NOT NULL DEFAULT 0,
  output_tokens INTEGER NOT NULL DEFAULT 0,
  cached_input_tokens INTEGER NOT NULL DEFAULT 0,
  reasoning_tokens INTEGER NOT NULL DEFAULT 0,
  embedding_tokens INTEGER NOT NULL DEFAULT 0,
  total_tokens INTEGER NOT NULL DEFAULT 0,
  total_cost_micro_usd INTEGER NOT NULL DEFAULT 0,
  avg_latency_ms REAL NOT NULL DEFAULT 0,
  avg_tokens_per_second REAL NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(date_key, workspace_id, agent_id, provider_code, model_api_id, usage_kind)
);
```

```sql
CREATE INDEX IF NOT EXISTS idx_usage_daily_date_model ON model_token_usage_daily(date_key, provider_code, model_api_id);
```

---

## 5. `model_pricing_rules`

模型计价规则；在保存 / 更新 `llm-provider-models` 时，若 `config_json` 中含非零价格字段，后端会 **Upsert** 此表（见 `PlatformService.syncProviderModelPricing`）。

```sql
CREATE TABLE IF NOT EXISTS model_pricing_rules (
  id TEXT PRIMARY KEY,
  provider_code TEXT NOT NULL,
  model_api_id TEXT NOT NULL,
  currency TEXT NOT NULL DEFAULT 'USD',
  input_price_micro_usd_per_1k INTEGER NOT NULL DEFAULT 0,
  output_price_micro_usd_per_1k INTEGER NOT NULL DEFAULT 0,
  cached_input_price_micro_usd_per_1k INTEGER NOT NULL DEFAULT 0,
  reasoning_price_micro_usd_per_1k INTEGER NOT NULL DEFAULT 0,
  embedding_price_micro_usd_per_1k INTEGER NOT NULL DEFAULT 0,
  effective_from TEXT NOT NULL,
  effective_to TEXT NOT NULL DEFAULT '',
  is_active INTEGER NOT NULL DEFAULT 1,
  source TEXT NOT NULL DEFAULT 'manual',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(provider_code, model_api_id, effective_from)
);
```

---

## 6. 关联：`agents` 中的模型引用

智能体行通过 **`provider`**、**`model`** 与 `llm_provider_models` 中已启用且未删除的组合校验（见 `ValidateProviderModel`）。

```sql
-- 节选（完整定义见同迁移文件）
CREATE TABLE IF NOT EXISTS agents (
  ...
  provider TEXT NOT NULL,
  model TEXT NOT NULL,
  ...
);
```

---

## 7. 维护说明

- 权威 DDL 以仓库内 **`0001_init.sql`** 为准；本文档在结构变更时需同步更新。
- 时间字段在 SQLite 中为 **TEXT**（ISO 8601 风格字符串），与后端 `nowISO()` 等一致。
- `usage_kind` 区分 chat / embedding 等调用类型，与产品 Embedding 子配置及记忆链路对齐时沿用同一套事件与聚合。
