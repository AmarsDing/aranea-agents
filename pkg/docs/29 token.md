# 模型 Token 消耗统计设计

本文档设计模型 Token 消耗与费用统计的数据结构，用于分析 **token 消耗、费用消耗、模型使用占比、历史趋势、Agent/用户维度成本**，帮助运营和开发精准把控模型使用情况。

---

## 1. 核心目标

| 目标 | 说明 |
|------|------|
| Token 消耗分析 | 统计输入、输出、缓存、推理、Embedding 等 Token 消耗 |
| 费用分析 | 按模型计费规则计算每次调用成本，并支持历史价格快照 |
| 模型占比 | 统计不同 Provider / Model 的调用量、Token 占比、费用占比 |
| 历史趋势 | 按小时、天、周、月查看 token 和费用趋势 |
| Agent 成本 | 分析各 Agent、会话、用户、团队的消耗 |
| 成本管控 | 支持预算、异常峰值、失败重试、低效模型识别 |

---

## 2. 设计原则

1. **明细流水是事实源**：每次模型调用产生一条不可变消耗记录。
2. **费用必须保留价格快照**：历史费用不能因为后续模型价格变化而被重新计算。
3. **输入/输出分开统计**：不同模型通常 input/output 单价不同。
4. **支持流式与非流式**：流式结束后写入完整消耗记录。
5. **统计口径明确**：成功、失败、取消、超时、重试都要可区分。
6. **聚合表可重算**：小时/日聚合用于性能优化，来源仍是明细表。

---

## 3. 明细表：`model_token_usage_events`

一条记录代表一次模型请求的最终消耗结果。它是所有统计的源数据。

```sql
CREATE TABLE IF NOT EXISTS model_token_usage_events (
  id TEXT PRIMARY KEY,

  -- 时间维度
  occurred_at TEXT NOT NULL,
  date_key TEXT NOT NULL,              -- YYYY-MM-DD
  hour_key TEXT NOT NULL,              -- YYYY-MM-DD HH:00

  -- 归属维度
  workspace_id TEXT NOT NULL DEFAULT '',
  user_id TEXT NOT NULL DEFAULT '',
  team_id TEXT NOT NULL DEFAULT '',
  agent_id TEXT NOT NULL DEFAULT '',
  agent_key TEXT NOT NULL DEFAULT '',
  session_id TEXT NOT NULL DEFAULT '',
  message_id TEXT NOT NULL DEFAULT '',
  request_id TEXT NOT NULL DEFAULT '',

  -- 模型维度
  provider_code TEXT NOT NULL DEFAULT '',
  provider_type TEXT NOT NULL DEFAULT '',
  provider_display_name TEXT NOT NULL DEFAULT '',
  model_api_id TEXT NOT NULL DEFAULT '',
  model_display_name TEXT NOT NULL DEFAULT '',
  model_category_json TEXT NOT NULL DEFAULT '[]',

  -- 调用类型
  usage_kind TEXT NOT NULL DEFAULT 'chat',
  -- chat / embedding / rerank / image / tool_call / summary / memory

  -- 调用次数
  call_count INTEGER NOT NULL DEFAULT 1,
  -- 明细表中通常固定为 1；聚合时 SUM(call_count) 得到调用次数。
  -- 保留字段是为了让所有统计口径统一使用 call_count，而不是混用 COUNT(*)。

  -- Token 明细
  input_tokens INTEGER NOT NULL DEFAULT 0,
  output_tokens INTEGER NOT NULL DEFAULT 0,
  cached_input_tokens INTEGER NOT NULL DEFAULT 0,
  reasoning_tokens INTEGER NOT NULL DEFAULT 0,
  embedding_tokens INTEGER NOT NULL DEFAULT 0,
  total_tokens INTEGER NOT NULL DEFAULT 0,

  -- 价格快照：单位建议 micro USD，避免浮点误差
  input_price_micro_usd_per_1k INTEGER NOT NULL DEFAULT 0,
  output_price_micro_usd_per_1k INTEGER NOT NULL DEFAULT 0,
  cached_input_price_micro_usd_per_1k INTEGER NOT NULL DEFAULT 0,
  reasoning_price_micro_usd_per_1k INTEGER NOT NULL DEFAULT 0,
  embedding_price_micro_usd_per_1k INTEGER NOT NULL DEFAULT 0,

  -- 成本结果：一次调用最终费用
  input_cost_micro_usd INTEGER NOT NULL DEFAULT 0,
  output_cost_micro_usd INTEGER NOT NULL DEFAULT 0,
  cached_input_cost_micro_usd INTEGER NOT NULL DEFAULT 0,
  reasoning_cost_micro_usd INTEGER NOT NULL DEFAULT 0,
  embedding_cost_micro_usd INTEGER NOT NULL DEFAULT 0,
  total_cost_micro_usd INTEGER NOT NULL DEFAULT 0,

  -- 性能与状态
  latency_ms INTEGER NOT NULL DEFAULT 0,
  time_to_first_token_ms INTEGER NOT NULL DEFAULT 0,
  tokens_per_second REAL NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'success',
  -- success / failed / cancelled / timeout / partial

  error_code TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '',
  retry_count INTEGER NOT NULL DEFAULT 0,

  -- 请求上下文快照
  prompt_mode TEXT NOT NULL DEFAULT '',
  max_output_tokens INTEGER NOT NULL DEFAULT 0,
  context_window_k INTEGER NOT NULL DEFAULT 0,
  stream_enabled INTEGER NOT NULL DEFAULT 0,

  -- 扩展字段
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL
);
```

### 推荐索引

```sql
CREATE INDEX IF NOT EXISTS idx_usage_events_time
  ON model_token_usage_events(occurred_at);

CREATE INDEX IF NOT EXISTS idx_usage_events_date_model
  ON model_token_usage_events(date_key, provider_code, model_api_id);

CREATE INDEX IF NOT EXISTS idx_usage_events_agent_time
  ON model_token_usage_events(agent_id, occurred_at);

CREATE INDEX IF NOT EXISTS idx_usage_events_session
  ON model_token_usage_events(session_id);

CREATE INDEX IF NOT EXISTS idx_usage_events_status
  ON model_token_usage_events(status, occurred_at);
```

---

## 4. 模型价格表：`model_pricing_rules`

用于维护当前价格规则。实际费用计算时，应把价格复制到 `model_token_usage_events` 作为快照。

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

## 5. 聚合表：`model_token_usage_daily`

用于趋势图、模型占比、费用看板。可由明细表定时重算。

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

如需更细趋势，可增加同结构小时表：`model_token_usage_hourly`。

---

## 6. 费用计算口径

建议所有费用用 **micro USD** 存储：

- `1 USD = 1,000,000 micro USD`
- 避免浮点误差
- 前端展示时再转换为美元或人民币

计算公式：

```text
input_cost_micro_usd =
  input_tokens * input_price_micro_usd_per_1k / 1000

output_cost_micro_usd =
  output_tokens * output_price_micro_usd_per_1k / 1000

total_cost_micro_usd =
  input_cost + output_cost + cached_input_cost + reasoning_cost + embedding_cost
```

---

## 7. 典型分析查询

### 7.1 最近 30 天消耗趋势

```sql
SELECT
  date_key,
  SUM(call_count) AS call_count,
  SUM(total_tokens) AS total_tokens,
  SUM(total_cost_micro_usd) AS total_cost_micro_usd
FROM model_token_usage_daily
WHERE date_key >= date('now', '-30 day')
GROUP BY date_key
ORDER BY date_key ASC;
```

### 7.2 模型费用占比

```sql
SELECT
  provider_code,
  model_api_id,
  SUM(total_cost_micro_usd) AS cost,
  SUM(total_tokens) AS tokens,
  SUM(call_count) AS call_count,
  COUNT(*) AS requests
FROM model_token_usage_events
WHERE occurred_at >= datetime('now', '-30 day')
  AND status = 'success'
GROUP BY provider_code, model_api_id
ORDER BY cost DESC;
```

### 7.3 Agent 成本排行

```sql
SELECT
  agent_id,
  agent_key,
  SUM(total_cost_micro_usd) AS cost,
  SUM(total_tokens) AS tokens,
  SUM(call_count) AS call_count,
  COUNT(*) AS requests
FROM model_token_usage_events
WHERE date_key >= date('now', '-7 day')
GROUP BY agent_id, agent_key
ORDER BY cost DESC;
```

### 7.4 高成本异常请求

```sql
SELECT
  occurred_at,
  agent_key,
  provider_code,
  model_api_id,
  input_tokens,
  output_tokens,
  total_cost_micro_usd,
  latency_ms
FROM model_token_usage_events
WHERE total_cost_micro_usd > 100000
ORDER BY total_cost_micro_usd DESC
LIMIT 50;
```

---

## 8. 写入时机

| 场景 | 写入策略 |
|------|----------|
| 普通非流式回复 | 模型返回后写入明细表 |
| 流式回复成功 | `done` 后写入明细表 |
| 用户中断流式 | 写入 `status = cancelled`，记录已产生的 output_tokens |
| 模型请求失败 | 写入 `status = failed`，记录错误码与错误信息 |
| 请求超时 | 写入 `status = timeout` |
| 重试 | `retry_count` 记录重试次数，或每次重试独立记录并通过 `request_id` 关联 |

---

## 9. 前端看板建议

### 9.1 概览页入口

建议在现有 **Overview / 概览** 页面增加一个「模型消耗」模块，作为首页运营看板的一部分；点击模块右上角「查看明细」进入完整 Token 用量页。

页面顶部筛选：

| 筛选项 | 说明 |
|--------|------|
| 时间范围 | 今日 / 7 天 / 30 天 / 本月 / 自定义 |
| Provider | 全部 / OpenRouter / Anthropic / Gemini 等 |
| 模型 | 选择具体 `model_api_id` |
| Agent | 按 Agent 查看消耗 |
| 状态 | 全部 / 成功 / 失败 / 取消 / 超时 |

### 9.2 概览核心卡片

- 今日 Token
- 今日调用次数
- 今日费用
- 本月费用
- 本月调用次数
- 平均响应耗时
- 平均 TPS

推荐卡片布局：

| 卡片 | 主指标 | 辅助指标 |
|------|--------|----------|
| 今日调用 | `SUM(call_count)` | 较昨日变化百分比 |
| 今日 Token | `SUM(total_tokens)` | 输入 / 输出 Token |
| 今日费用 | `SUM(total_cost_micro_usd)` | 较昨日变化百分比 |
| 本月费用 | 本月累计费用 | 月预算使用率 |
| 平均延迟 | `AVG(latency_ms)` | P95 延迟 |
| 平均 TPS | `AVG(tokens_per_second)` | 最高 / 最低模型 |

### 9.3 趋势图

- 调用次数趋势：按天/小时
- Token 消耗趋势：按天/小时
- 费用趋势：按天/小时
- 输入/输出 Token 堆叠趋势

趋势图建议：

| 图表 | 展示内容 | 用途 |
|------|----------|------|
| 调用次数趋势 | `SUM(call_count)` 按时间分组 | 判断使用活跃度 |
| Token 趋势 | input/output/total token 堆叠 | 判断上下文与输出变化 |
| 费用趋势 | `SUM(total_cost_micro_usd)` | 发现成本峰值 |
| 成功率趋势 | success / failed / timeout | 发现模型或网络异常 |

### 9.4 占比分析

- 模型调用次数占比
- 模型费用占比
- 模型 Token 占比
- Provider 占比
- Agent 成本占比

占比图建议：

| 图表 | 口径 |
|------|------|
| 模型调用占比 | 按 `provider_code + model_api_id` 汇总 `call_count` |
| 模型费用占比 | 按模型汇总 `total_cost_micro_usd` |
| Provider 费用占比 | 按 `provider_code` 汇总费用 |
| Agent 调用占比 | 按 `agent_id` 汇总 `call_count` |

### 9.5 Top 排行

在概览页底部放 2～3 个 Top 列表：

| 模块 | 字段 |
|------|------|
| Top 模型消耗 | 模型、调用次数、Token、费用、平均 TPS |
| Top Agent 成本 | Agent、调用次数、费用、平均延迟 |
| 异常请求 | 时间、模型、Agent、状态、错误、费用 |

### 9.6 明细列表

完整明细页字段建议：

- 时间
- Agent
- Provider / Model
- 调用次数
- 输入 Token
- 输出 Token
- 总 Token
- 费用
- 延迟
- TPS
- 状态
- 错误信息

---

## 10. 与现有模型管理的关系

`llm_provider_models.config_json` 可继续保存模型静态信息和最近计算结果：

```json
{
  "model_size_label": "70B",
  "context_window_k": 128,
  "max_output_tokens": 8192,
  "tokens_per_second": 52.3
}
```

但统计分析不应依赖这个字段，而应依赖 `model_token_usage_events` 明细表。

---

## 11. 后续扩展

- 支持按用户/团队预算告警
- 支持模型费用阈值提醒
- 支持低性价比模型识别：高成本、低 TPS、高失败率
- 支持价格自动同步：OpenRouter / Gemini / Anthropic / OpenAI
- 支持导出 CSV
- 支持按 Agent、模型、用户维度设置月度预算
