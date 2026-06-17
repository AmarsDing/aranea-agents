# LLM Provider 管理

本文档定义 LLM Provider 管理模块的**用户故事、功能需求、验收标准与交互规格**。

**核心产品目标**：对齐 trpc-agent-go 的 **5 种原生 Provider**（OpenAI、Anthropic、Gemini、Ollama、Hunyuan）+ **4 种 OpenAI Variant**（OpenAI、DeepSeek、Qwen、Hunyuan）+ **Failover/Hedge 高可用模式**，在 UI 中显式区分 Provider 类型，让用户能为每个 Provider 配置密钥、启用状态，并为下属模型标注能力分类与可选性能指标，便于 Agent 选模与运营展示。

> **技术设计、Proto 契约、数据模型、代码分层、开发进度**：见 [9-provider.design.md](./9-provider.design.md) 与 [9-provider.development.md](./9-provider.development.md)。

与 **`8 agent-title.md` §8.1**（供应商–模型级联）、**`2 agents-create.md`**（Provider/模型选择）、**`前端.md`** `agents.provider` 对齐。

---

## 1. 页面定位

| 项目 | 说明 |
|------|------|
| **路由建议** | `/settings/llm-providers`（或项目统一设置前缀下） |
| **用户目标** | 维护可连接的 LLM 厂商；为每个 Provider 配置密钥、启用状态；为 **下属模型** 标注 **能力分类** 与可选性能指标，便于 Agent 选模与运营展示 |
| **入口** | Agent 高级设置 **「管理供应商与模型」**；侧边栏「模型厂商」等 |

---

## 2. 列表页布局

| 区域 | 说明 |
|------|------|
| **标题区** | 主标题 **Provider**；副标题 **管理 LLM Provider**；右上 **`+ 添加Provider`**（主色 `QBtn`） |
| **搜索** | `QInput` 带搜索图标；占位 **搜索Provider…**；对 `provider_code`、`name`、`model_api_id` 等做前端过滤或 `GET ?q=` |
| **Provider 类型筛选** | `QSelect` 多选；选项为 trpc Provider 类型：OpenAI Compatible / Anthropic / Gemini / Ollama / Hunyuan / HuggingFace / Bedrock |
| **列表** | `QList` 卡片行；见 §3 |
| **分页** | 底部：总条数、`per_page`（如 20）、翻页 |

---

## 3. 列表行（一行对应一个模型）

从左到右建议布局：

| 元素 | 说明 |
|------|------|
| **图标 + 名称** | 小图标（如芯片）+ **`provider_code`** + **`model_display_name` 或 `model_api_id`** + **状态绿点**（连接正常/已配置等，规则由后端定） |
| **Provider 类型** | `QChip` 展示 trpc Provider 类型：**OpenAI** / **Anthropic** / **Gemini** / **Ollama** / **Hunyuan** / **HuggingFace** / **Bedrock**；不同类型用不同颜色区分 |
| **Variant** | 仅 Provider 类型 = OpenAI 时显示：`QChip` 小标签，如 **DeepSeek** / **Qwen** / **Hunyuan**；非 OpenAI 类型或 Variant = openai 时隐藏 |
| **模型分类** | 展示当前模型所选分类的 **`label`**（展示名），可用 **`QChip`**；**鼠标悬停**时 **`QTooltip`** 显示该类型的 **一句话说明** |
| **模型大小** | 只读展示 **`model_size_label`**（如 `7B` / `70B`）；空则 **「—」** 或隐藏 |
| **上下文** | 只读展示 **`context_window_k`**（如 `128K`）；帮助用户快速判断长文本能力 |
| **热度** | 展示 **`model_hotness_score`**（0～100），用 **进度条 + 等级文案** 表达近期使用活跃度；见 §3.1 |
| **近 30 天调用** | 展示 **`usage_call_count_30d`**；暂无统计时展示 **「—」** |
| **近 30 天费用** | 展示 **`usage_cost_micro_usd_30d`** 格式化后的费用；暂无统计时展示 **「—」** |
| **TPS** | 只读展示 **`tokens_per_second`**；空则 **「—」** |
| **成功率 / 延迟** | 可选展示 **`success_rate_30d`**、**`avg_latency_ms_30d`** |
| **API 密钥** | 钥匙图标 + **已设置API密钥** / **未设置**（未设置可标橙或警告色） |
| **高可用** | 若配置了 Failover/Hedge，显示 `QChip`：**Failover**（蓝色）/ **Hedge**（紫色）；Tooltip 显示候选模型列表 |
| **启用** | **`QToggle`**：开关 ON = 启用，OFF = 停用；变更即 **PATCH** `is_enabled` |
| **历史趋势** | **`QBtn`flat** `query_stats` 图标或文案 **趋势**；打开该模型的历史趋势看板（见 §3.2） |
| **编辑** | **`QBtn`flat** 文案 **编辑** 或 `edit` 图标 |
| **删除** | **`QBtn`flat** `delete` 图标；二次确认后 **DELETE** 或软删 |

**交互**：

- 点击 **编辑** → 打开与「添加」同构的 **编辑弹窗**（§4），`GET /llm-provider-models/:id` 预填当前行。
- 列表行内 **开关** 仅切换启用；不打开弹窗。
- 点击 **趋势** → 打开模型历史趋势看板。

### 3.1 模型热度显示

**热度定义**：`model_hotness_score`，范围 **0～100**，由统计服务根据近期调用次数、Token 消耗、费用占比、成功率等计算。

建议计算口径：

```text
热度 = 近期调用次数标准分 * 0.45
     + Token 消耗标准分 * 0.25
     + 费用占比标准分 * 0.15
     + 成功率修正 * 0.10
     + 最近使用时间修正 * 0.05
```

展示方式：

| 热度分 | 文案 | 样式 |
|--------|------|------|
| 80～100 | 热门 | `QLinearProgress` 红/橙色，`QChip` 显示「热门」 |
| 50～79 | 活跃 | 蓝色，显示「活跃」 |
| 20～49 | 低频 | 灰蓝色，显示「低频」 |
| 0～19 | 冷门 | 灰色，显示「冷门」 |

Tooltip 展示热度来源：近 30 天调用次数、Token、费用、最近一次调用时间、成功率。

### 3.2 历史趋势看板

点击列表行 **趋势** 按钮打开 `QDialog` / 右侧抽屉 / 独立页面。

建议展示：

| 模块 | 内容 |
|------|------|
| 顶部摘要 | 模型名、Provider、热度、30 天调用、30 天 Token、30 天费用 |
| 趋势图 | 调用次数趋势、Token 趋势、费用趋势、平均延迟趋势 |
| 占比 | 该模型在全部模型中的调用占比、Token 占比、费用占比 |
| 性能 | 平均 TPS、平均延迟、P95 延迟、成功率、失败次数 |
| 最近调用 | 最近 20 条调用记录：时间、Agent、Token、费用、状态、耗时 |

### 3.3 列表字段建议分层

| 层级 | 字段 | 说明 |
|------|------|------|
| 第一优先级 | 名称、Provider 类型、模型分类、热度、启用、编辑/删除 | 日常管理必看 |
| 第二优先级 | Variant、模型大小、上下文、最大输出 Token、TPS、高可用 | 选模和性能判断 |
| 第三优先级 | 30 天调用、30 天费用、成功率、平均延迟 | 使用情况和成本判断 |
| 展开/Tooltip | 最近调用时间、失败次数、费用占比、Token 占比 | 详情辅助信息 |

---

## 4. 添加 / 编辑 Provider 弹窗

`QDialog`；标题 **添加Provider** / **编辑Provider**；副标题 **配置 LLM Provider 连接**。

弹窗分为四个步骤/标签页：**① 连接与身份** → **② 模型分类与规格** → **③ 高可用配置** → **④ 高级选项**。

### 4.1 连接与身份

| 字段 | 控件 | 校验 / 说明 |
|------|------|-------------|
| **Provider 预设** | `QSelect` | 选项来自 `PROVIDER_PRESETS`；选择后自动填充下方字段 |
| **Provider 类型** * | `QSelect` | 选项：OpenAI Compatible / Anthropic / Gemini / Ollama / Hunyuan / HuggingFace / Bedrock；选择后自动切换下方表单字段和 Variant 选项 |
| **Variant** | `QSelect` | 仅 Provider 类型 = OpenAI Compatible 时显示；选项：OpenAI / DeepSeek / Qwen / Hunyuan；默认 OpenAI；选择后自动预填默认 Base URL |
| **Provider 编码** * | `QInput` | 占位「例如 openrouter」；**小写字母、数字、连字符**；同一厂商多模型时多行共用同一 `provider_code` |
| **Provider 显示名** | `QInput` | UI 展示名 |
| **模型 API ID** * | `QInput` | 与厂商文档一致 |
| **模型展示名** | `QInput`（可选） | 模型展示名覆盖 |
| **API 基础 URL** | `QInput` | `https://…`；根据 Provider 类型 + Variant 自动预填默认值 |
| **API 密钥** | `QInput` `type=password` 或可切换明文 | 编辑时可 **留空表示不修改**；仅 authType = `api_key` 时显示 |
| **Secret ID** | `QInput` | 仅 authType = `secret_id_key`（Hunyuan）时显示 |
| **Secret Key** | `QInput` `type=password` | 仅 authType = `secret_id_key`（Hunyuan）时显示 |
| **AWS Region** | `QSelect` | 仅 authType = `aws_config`（Bedrock）时显示 |
| **已启用** | `QToggle` | 与列表开关同源 |

**Provider 类型切换逻辑**：

| Provider 类型 | authType | 显示字段 | 隐藏字段 |
|--------------|----------|---------|---------|
| OpenAI Compatible | `api_key` | API 基础 URL、API 密钥、Variant | Secret ID/Key、AWS Region |
| Anthropic | `api_key` | API 基础 URL、API 密钥 | Variant、Secret ID/Key、AWS Region |
| Gemini | `api_key` | API 基础 URL、API 密钥 | Variant、Secret ID/Key、AWS Region |
| Ollama | `none` | API 基础 URL（Host） | API 密钥、Variant、Secret ID/Key、AWS Region |
| Hunyuan | `secret_id_key` | API 基础 URL、Secret ID、Secret Key | API 密钥、Variant、AWS Region |
| HuggingFace | `api_key` | API 基础 URL、API 密钥 | Variant、Secret ID/Key、AWS Region |
| Bedrock | `aws_config` | AWS Region | API 基础 URL、API 密钥、Variant、Secret ID/Key |

### 4.2 模型分类与规格

| 字段 | 控件 | 说明 |
|------|------|------|
| **模型分类** | `QSelect` multiple | 选项见 §4.2.1 |
| **模型大小标签** | `QInput` | 如 `7B` / `70B` |
| **上下文窗口** | `QInput type=number` suffix `K tokens` | 模型上下文窗口大小（千 Token） |
| **最大输出 Token** | `QInput type=number` | 单次最大输出 Token |
| **输入价格** | `QInput type=number` suffix `µ$/1K token` | 每百万输入 Token 的微美元单价 |
| **输出价格** | `QInput type=number` suffix `µ$/1K token` | 每百万输出 Token 的微美元单价 |
| **缓存输入价格** | `QInput type=number` suffix `µ$/1K token` | 缓存输入 Token 单价（Anthropic 等） |
| **推理价格** | `QInput type=number` suffix `µ$/1K token` | 推理 Token 单价（DeepSeek 等） |
| **嵌入价格** | `QInput type=number` suffix `µ$/1K token` | 嵌入 Token 单价 |
| **检查模型** | `QBtn` | 调用 Inspect API，自动回填上述字段 |

#### 4.2.1 模型分类选项

| value | label | tooltip |
|-------|-------|---------|
| `general` | 通用对话 | 均衡，适合日常问答与轻任务 |
| `reasoning` | 推理 / 复杂问题 | 数学、逻辑、多步推导 |
| `code` | 代码 | 生成、解释、重构代码 |
| `long_context` | 长上下文 | 大文档、长会话摘要 |
| `vision` | 视觉 / 多模态 | 图像理解 |
| `embedding` | 向量嵌入 | 记忆、检索 |
| `fast` | 低延迟 | 优先响应速度 |
| `creative` | 创意写作 | 文案、故事、营销 |

### 4.3 高可用配置

| 字段 | 控件 | 说明 |
|------|------|------|
| **高可用模式** | `QSelect` | 选项：无 / Failover / Hedge |
| **候选模型** | `QBtn` + 动态列表 | 添加候选模型条目（模型名 + Base URL + API Key） |
| **Hedge 延迟** | `QInput type=number` suffix `ms` | 仅 Hedge 模式；候选请求延迟启动间隔（默认 100ms） |

**候选模型条目结构**：

```jsonc
{
  "ha_candidates": [
    { "name": "gpt-4o", "base_url": "https://backup.example.com/v1", "api_key": "sk-backup" },
    { "name": "gpt-4o-mini", "base_url": "https://api.openai.com/v1", "api_key": "sk-fallback" }
  ]
}
```

### 4.4 高级选项

| 字段 | 控件 | 说明 | 适用 Provider |
|------|------|------|--------------|
| **Token Tailoring** | `QToggle` | 自动裁剪输入 Token 以适应上下文窗口 | 全部 |
| **Prompt Cache 优化** | `QToggle` | 将 system 消息前置以提高缓存命中率 | OpenAI |
| **Reasoning 回填** | `QToggle` | 为无推理内容的 assistant 消息回填空 reasoning_content | OpenAI (DeepSeek) |
| **Tool Call Delta** | `QToggle` | 流式响应中暴露 tool_call 增量 | OpenAI, Anthropic |
| **System Prompt Cache** | `QToggle` | 缓存系统提示（90% 输入折扣） | Anthropic |
| **Tools Cache** | `QToggle` | 缓存工具定义（90% 输入折扣） | Anthropic |
| **Messages Cache** | `QToggle` | 多轮对话缓存 | Anthropic |
| **Keep Alive** | `QInput type=number` + suffix `分钟` | 模型保持加载的时长 | Ollama |
| **Ollama Options** | `QInput` JSON 编辑器 | Ollama API 额外参数 | Ollama |
| **Extra Headers** | `QInput` JSON 编辑器 | 额外 HTTP 头 | OpenAI, Anthropic, HuggingFace |
| **Extra Fields** | `QInput` JSON 编辑器 | 额外请求体字段 | OpenAI, HuggingFace |
| **Channel Buffer Size** | `QInput type=number` | 响应通道缓冲区大小（默认 256） | 全部 |

---

## 5. 功能验收

### 5.1 功能验收

| # | 验收项 | 优先级 |
|---|--------|--------|
| 1 | 选择 Provider 预设后，自动填充 provider_type / variant / api_base_url / authType | P0 |
| 2 | Provider 类型切换后，表单字段正确显示/隐藏 | P0 |
| 3 | Hunyuan 类型显示 Secret ID/Key 字段，Ollama 类型隐藏 API Key 字段 | P0 |
| 4 | OpenAI 类型显示 Variant 选择，其他类型隐藏 | P0 |
| 5 | 检查模型功能正常，回填 context_window / pricing 等信息 | P0 |
| 6 | 后端按 provider_type 正确构建 trpc model.Model 实例 | P0 |
| 7 | Anthropic 模型通过原生 SDK 调用，不再走 OpenAI 兼容层 | P0 |
| 8 | Gemini 模型通过原生 SDK 调用 | P0 |
| 9 | Hunyuan 模型通过原生 SDK 调用（SecretId/SecretKey 认证） | P0 |
| 10 | Ollama 模型通过原生 SDK 调用（无认证） | P0 |
| 11 | Failover 模式：主模型失败后自动切换到候选模型 | P1 |
| 12 | Hedge 模式：并发请求，首个有效响应返回 | P1 |
| 13 | 高级选项（Token Tailoring / Cache / Tool Call Delta 等）正确传递到 trpc 选项 | P1 |
| 14 | 列表页 Provider 类型 Chip 正确展示 | P1 |
| 15 | 列表页 Variant Chip 仅在 OpenAI + 非 openai Variant 时展示 | P2 |

### 5.2 数据兼容性

| # | 验收项 | 优先级 |
|---|--------|--------|
| 1 | 旧数据 `provider_type: "OpenAI Compatible"` 自动映射为 `"openai"` | P0 |
| 2 | 旧数据 `provider_type: "Anthropic"` 自动映射为 `"anthropic"` | P0 |
| 3 | 旧数据 `provider_type: "Google Gemini"` 自动映射为 `"gemini"` | P0 |
| 4 | 旧数据 `provider_type: "Ollama"` 自动映射为 `"ollama"` | P0 |
| 5 | 旧数据 `provider_type: "Custom"` 兜底为 `"openai"` | P0 |
| 6 | 旧数据无 `variant` 字段时，OpenAI 类型默认 Variant = `"openai"` | P1 |

### 5.3 性能验收

| # | 验收项 | 优先级 |
|---|--------|--------|
| 1 | Provider 列表加载 < 500ms | P1 |
| 2 | 模型检查（Inspect）响应 < 5s | P1 |
| 3 | Failover 切换延迟 < 1s | P2 |

---

## 6. trpc-agent-go 对齐需求（M9 Model 模型层）

本节补充 `plan.md` M9 模块的对齐需求，确保 Model 模型层完全复刻 trpc-agent-go `model` 包能力。

### 6.1 Failover 高可用

**需求**：
- 在 Provider 管理页增加 Failover 配置
- 支持配置主备模型列表
- 主模型失败时自动切换到备模型
- 切换事件记录在 `model_token_usage_events` 中

**验收标准**：主模型失败时自动切换到备模型

### 6.2 Hedge 低延迟

**需求**：
- 在 Provider 管理页增加 Hedge 配置
- 支持配置候选模型列表和延迟偏移
- 并发请求，首个有效响应即返回
- 取消其他未完成请求

**验收标准**：Hedge 模式下响应延迟显著降低

### 6.3 TokenTailor

**需求**：
- 集成 trpc `model/tokentailor` 包
- 当请求 token 超过上下文窗口时自动裁剪
- 裁剪策略：保留系统指令 + 最近 N 轮 + 摘要

**验收标准**：请求 token 超限时自动裁剪而非报错

### 6.4 多模型注册

**需求**：
- 确认 5 种已注册 Provider 可正常使用
- 待 HuggingFace 和 Bedrock 上游注册后启用
- 每种 Provider 的认证方式和配置项完整

**验收标准**：所有已注册 Provider 可正常调用

### 6.5 IterModel 优化

**需求**：
- 检查当前模型是否支持 IterModel 接口
- 支持 IterModel 的模型使用迭代模式
- 不支持的模型回退到 channel 模式

**验收标准**：支持 IterModel 的模型使用迭代模式，性能更优

---

*文档版本：基于 trpc-agent-go `model/provider` 体系重新设计；与 `8 agent-title.md` §8.1、`2 agents-create.md` 对齐。技术设计与开发进度见同目录 `.design.md` 与 `.development.md`。*
