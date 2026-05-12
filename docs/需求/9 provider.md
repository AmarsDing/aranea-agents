# LLM Provider 管理

本文档基于 **trpc-agent-go 框架 `model` 体系**重新设计，合并产品/UI 规格、数据与 API 约定、验收要点及功能需求。

**核心变更**：原设计将所有 Provider 类型扁平存储为 `config_json`，运行时仅通过 OpenAI 兼容层调用。新设计对齐 trpc-agent-go 的 **5 种原生 Provider**（OpenAI、Anthropic、Gemini、Ollama、Hunyuan）+ **4 种 OpenAI Variant**（OpenAI、DeepSeek、Qwen、Hunyuan）+ **Failover/Hedge 高可用模式**，在数据表和 UI 中显式区分 Provider 类型，运行时通过 `model/provider` 统一工厂构建 `model.Model` 实例。

> **注意**：trpc-agent-go `model/provider` 当前 `init()` 仅注册 5 种 Provider（openai / anthropic / gemini / ollama / hunyuan）。HuggingFace 和 Bedrock 虽有独立 `model/huggingface` 和 `model/bedrock` 包，但尚未注册到 `provider` 工厂。本设计预留 HuggingFace 和 Bedrock 的 Provider 类型枚举值，待 trpc 上游注册后即可启用。

与 **`8 agent-title.md` §8.1**（供应商–模型级联）、**`2 agents-create.md`**（Provider/模型选择）、**`前端.md`** `agents.provider` 对齐。

---

## 1. 页面定位

| 项目 | 说明 |
|------|------|
| **路由建议** | `/settings/llm-providers`（或项目统一设置前缀下） |
| **用户目标** | 维护可连接的 LLM 厂商；为每个 Provider 配置密钥、启用状态；为 **下属模型** 标注 **能力分类** 与可选性能指标，便于 Agent 选模与运营展示 |
| **入口** | Agent 高级设置 **「管理供应商与模型」**；侧边栏「模型厂商」等 |

---

## 2. trpc-agent-go Model 体系总览

trpc-agent-go 框架提供统一的 `model.Model` 接口，通过 `model/provider` 工厂按 Provider 类型分发构建：

```go
// model.Model 核心接口
type Model interface {
    GenerateContent(ctx context.Context, request *Request) (<-chan *Response, error)
    Info() Info  // 返回 Name + ContextWindow
}

// model.Model 可选扩展接口（同 goroutine 迭代，减少 channel 开销）
type IterModel interface {
    Model
    GenerateContentIter(ctx context.Context, request *Request) (Seq[*Response], error)
}

// model/provider 统一工厂
m, err := provider.Model("openai", "gpt-4o",
    provider.WithAPIKey("sk-..."),
    provider.WithBaseURL("https://api.openai.com/v1"),
    provider.WithVariant("deepseek"),
)
```

### 2.1 原生 Provider 类型

| Provider 类型 | trpc 包 | 认证方式 | 默认 Base URL | 协议 | Go Module | `provider.Model()` 第一个参数 | 工厂注册状态 |
|--------------|---------|---------|--------------|------|-----------|---------------------------|------------|
| **OpenAI Compatible** | `model/openai` | API Key | `https://api.openai.com/v1` | OpenAI Chat Completions | 主模块 | `"openai"` | ✅ 已注册 |
| **Anthropic** | `model/anthropic` | API Key | `https://api.anthropic.com` | Anthropic Messages | 独立 module | `"anthropic"` | ✅ 已注册 |
| **Gemini** | `model/gemini` | API Key | `https://generativelanguage.googleapis.com` | Google GenAI | 独立 module | `"gemini"` | ✅ 已注册 |
| **Ollama** | `model/ollama` | 无（本地） | `http://localhost:11434` | Ollama Chat | 独立 module | `"ollama"` | ✅ 已注册 |
| **Hunyuan** | `model/hunyuan` | SecretId + SecretKey | `https://hunyuan.tencentcloudapi.com` | 混元私有协议 | 主模块 | `"hunyuan"` | ✅ 已注册 |
| **HuggingFace** | `model/huggingface` | API Key | `https://router.huggingface.co` | HF Inference | 主模块 | `"huggingface"` | ⏳ 待注册 |
| **Bedrock** | `model/bedrock` | AWS Config | — | AWS Bedrock Runtime | 独立 module | `"bedrock"` | ⏳ 待注册 |

### 2.2 OpenAI Compatible 子类型（Variant）

OpenAI Provider 通过 `Variant` 区分子类型行为差异，由 `openai.WithVariant()` 或 `provider.WithVariant()` 设置：

| Variant | 枚举值 | 行为差异 | 默认 Base URL | 环境变量 Key |
|---------|--------|---------|--------------|------------|
| **OpenAI** | `openai` | 默认；自动 prompt cache 优化（`OptimizeForCache` 默认启用） | `https://api.openai.com/v1` | `OPENAI_API_KEY` |
| **DeepSeek** | `deepseek` | 自动回填 `reasoning_content`（`ReasoningContentBackfill` 默认启用）；思考模式使用 `{"type":"enabled"/"disabled"}` 格式；`textOnlyMessageContent=true` | `https://api.deepseek.com` | `DEEPSEEK_API_KEY` |
| **Qwen** | `qwen` | 默认 Base URL 指向阿里云；思考模式使用 `enabled_thinking` key | `https://dashscope.aliyuncs.com/compatible-mode/v1` | `DASHSCOPE_API_KEY` |
| **Hunyuan** | `hunyuan` | 特定文件处理逻辑（`OmitFileContentParts`）；文件上传使用 multipart form；文件删除使用 POST + JSON body | — | — |

**Variant 自动推断**：若未显式设置 Variant，OpenAI Model 会根据 `BaseURL` 自动推断（如 BaseURL 包含 `api.deepseek.com` 则推断为 `deepseek`）。

### 2.3 高可用模式

| 模式 | trpc 包 | 说明 | 关键配置 |
|------|---------|------|---------|
| **Failover** | `model/failover` | 按顺序尝试候选模型，首个成功响应即返回；适用于主备切换 | `WithCandidates(m1, m2, ...)` |
| **Hedge** | `model/hedge` | 并发发起多个候选请求，首个有效响应即返回；可配置延迟启动偏移；适用于降低尾部延迟 | `WithCandidates(...)`, `WithDelay(100ms)`, `WithDelays(...)`, `WithName(...)`, `WithContextWindow(...)` |

**Failover 示例**：
```go
primary, _ := openai.New("gpt-4o", openai.WithAPIKey("sk-primary"))
backup, _ := openai.New("gpt-4o", openai.WithAPIKey("sk-backup"), openai.WithBaseURL("https://backup.example.com/v1"))
fo, _ := failover.New(failover.WithCandidates(primary, backup))
```

**Hedge 示例**：
```go
fast, _ := openai.New("gpt-4o-mini", openai.WithAPIKey("sk-..."))
powerful, _ := openai.New("gpt-4o", openai.WithAPIKey("sk-..."))
h, _ := hedge.New(
    hedge.WithCandidates(fast, powerful),
    hedge.WithDelay(100*time.Millisecond),
    hedge.WithName("hedge-gpt"),
)
```

### 2.4 通用能力

所有 Provider 共享以下能力（通过 `model/provider.Options` 统一配置）：

| 能力 | provider.Option | 说明 |
|------|----------------|------|
| **Token Tailoring** | `WithEnableTokenTailoring(bool)` | 自动裁剪输入 Token 以适应上下文窗口 |
| **Context Window** | `WithContextWindow(int)` | 实例级覆盖上下文窗口大小 |
| **Max Input Tokens** | `WithMaxInputTokens(int)` | 最大输入 Token 限制 |
| **Token Counter** | `WithTokenCounter(model.TokenCounter)` | 自定义 Token 计数器（默认 `SimpleTokenCounter`，4 字符 ≈ 1 Token） |
| **Tailoring Strategy** | `WithTailoringStrategy(model.TailoringStrategy)` | 自定义裁剪策略（默认 `MiddleOutStrategy`） |
| **Token Tailoring Config** | `WithTokenTailoringConfig(*model.TokenTailoringConfig)` | 裁剪预算参数（ProtocolOverhead / ReserveOutput / SafetyMargin 等） |
| **Channel Buffer Size** | `WithChannelBufferSize(int)` | 响应通道缓冲区大小（默认 256） |
| **Custom HTTP Transport** | `WithHTTPClientTransport(http.RoundTripper)` | 注入自定义 Transport，用于代理、超时、链路追踪 |
| **Extra Headers** | `WithHeaders(map[string]string)` | 向请求注入额外 HTTP 头 |
| **Extra Fields** | `WithExtraFields(map[string]any)` | 向请求体注入额外字段 |
| **Callbacks** | `WithCallbacks(provider.Callbacks)` | 四阶段回调（Request/Response/Chunk/StreamComplete），按 Provider 分发 |

### 2.5 Provider 专属能力

#### OpenAI 专属

| 能力 | Option | 说明 |
|------|--------|------|
| **Variant 选择** | `WithVariant(openai.Variant)` | 选择 OpenAI/DeepSeek/Qwen/Hunyuan 行为模式 |
| **Prompt Cache 优化** | `WithOptimizeForCache(bool)` | 将 system 消息前置以提高缓存命中率（VariantOpenAI 默认启用） |
| **Reasoning 回填** | `WithReasoningContentBackfill(bool)` | DeepSeek Variant 默认启用；为无推理内容的 assistant 消息回填空 reasoning_content |
| **Tool Call Delta** | `WithShowToolCallDelta(bool)` | 流式响应中暴露 tool_call 增量 |
| **Batch API** | `WithBatchCompletionWindow`, `WithBatchMetadata`, `WithBatchBaseURL` | 批量处理 API |
| **文件处理** | `WithOmitFileContentParts(bool)` | 从请求中移除文件内容部分（Hunyuan Variant 使用） |
| **Telemetry** | `WithChatTelemetry(bool)` | 启用直接模型调用的遥测 |
| **OpenAI SDK 原生** | `WithOpenAIOptions(...openaiopt.RequestOption)` | 透传 OpenAI Go SDK 原生选项 |

#### Anthropic 专属

| 能力 | Option | 说明 |
|------|--------|------|
| **System Prompt Cache** | `WithCacheSystemPrompt(bool)` | 缓存系统提示（90% 输入折扣）；默认关闭 |
| **Tools Cache** | `WithCacheTools(bool)` | 缓存工具定义（90% 输入折扣）；默认关闭 |
| **Messages Cache** | `WithCacheMessages(bool)` | 多轮对话缓存（动态移动缓存断点到最新 assistant 消息）；默认关闭 |
| **Tool Call Delta** | `WithShowToolCallDelta(bool)` | 流式响应中暴露 tool_call 增量 |
| **Anthropic SDK 原生** | `WithAnthropicClientOptions(...option.RequestOption)` | 透传 Anthropic Go SDK 原生选项 |

#### Gemini 专属

| 能力 | Option | 说明 |
|------|--------|------|
| **Client Config** | `WithGeminiClientConfig(*genai.ClientConfig)` | 自定义 Gemini 客户端配置 |

#### Ollama 专属

| 能力 | Option | 说明 |
|------|--------|------|
| **Host** | `WithHost(string)` | Ollama 服务地址（默认 `http://localhost:11434`） |
| **Keep Alive** | `WithKeepAlive(time.Duration)` | 模型保持加载的时长 |
| **Options** | `WithOptions(map[string]any)` | Ollama API 额外参数 |

#### Hunyuan 专属

| 能力 | Option | 说明 |
|------|--------|------|
| **SecretId** | `WithSecretId(string)` | 腾讯云 SecretId |
| **SecretKey** | `WithSecretKey(string)` | 腾讯云 SecretKey |
| **Host** | `WithHost(string)` | 混元服务地址 |
| **Base URL** | `WithBaseUrl(string)` | 混元 API 基础 URL |

#### HuggingFace 专属

| 能力 | Option | 说明 |
|------|--------|------|
| **API Key** | `WithAPIKey(string)` | HuggingFace API Key（环境变量 `HUGGINGFACE_API_KEY`） |
| **Base URL** | `WithBaseURL(string)` | 默认 `https://router.huggingface.co` |
| **Extra Headers** | `WithExtraHeaders(map[string]string)` | 额外 HTTP 头 |
| **Extra Fields** | `WithExtraFields(map[string]any)` | 额外请求字段 |

#### Bedrock 专属

| 能力 | Option | 说明 |
|------|--------|------|
| **AWS Config** | `WithAWSConfig(aws.Config)` | AWS 配置（通常通过 `config.LoadDefaultConfig(ctx)` 获取） |
| **Bedrock Options** | `WithBedrockOptions(...func(*bedrockruntime.Options))` | Bedrock Runtime 客户端选项 |
| **Custom Client** | `WithClient(BedrockClient)` | 自定义 Bedrock 客户端（测试用） |

### 2.6 内置模型上下文窗口注册表

trpc-agent-go 内置了 150+ 模型的上下文窗口大小映射（`model/internal/model/model_info.go`），支持：

- **精确匹配**：模型名大小写不敏感精确查找
- **前缀匹配**：按 `-` / `@` / `:` 分隔符前缀匹配（如 `gpt-4o-mini-2024-07-18` 匹配 `gpt-4o-mini`）
- **运行时注册**：`model.RegisterModelContextWindow(name, size)` / `model.RegisterModelContextWindows(map)`
- **查询**：`model.LookupModelContextWindow(name) (int, bool)` / `model.Info().ContextWindow`

主要模型族覆盖：

| 模型族 | 示例 | 上下文窗口 |
|--------|------|-----------|
| OpenAI GPT-5.x | gpt-5.4, gpt-5.4-pro | 1,050,000 |
| OpenAI GPT-4.1 | gpt-4.1, gpt-4.1-mini, gpt-4.1-nano | 1,047,576 |
| OpenAI o-series | o3, o4-mini | 200,000 |
| Anthropic Claude 4.6 | claude-opus-4-6, claude-sonnet-4-6 | 1,000,000 |
| Anthropic Claude 4.5 | claude-opus-4-5, claude-sonnet-4-5 | 200,000 |
| Gemini 2.5/3.0 | gemini-2.5-pro, gemini-3.0-pro | 2,097,152 |
| DeepSeek V4 | deepseek-v4-pro, deepseek-v4-flash | 1,000,000 |
| Qwen 3.5+ | qwen3.5-plus, qwen3.5-flash | 1,000,000 |
| Hunyuan 2.0 | hunyuan-2.0-instruct, hunyuan-2.0-thinking | 147,456 / 196,608 |
| GLM 5.x | glm-5, glm-5.1 | 200,000 / 204,800 |
| Kimi K2 | kimi-k2.5 | 256,000 |
| MiniMax M2 | minimax-m2.7 | 204,800 |

---

## 3. Provider 类型与前端预设映射

前端 `providerPresets.ts` 已维护 18+ 个 Provider 预设。新设计将预设中的 `providerType` 对齐 trpc Provider 类型枚举，新增 `variant` 和 `authType` 字段：

### 3.1 前端类型定义更新

```typescript
export type AuthType = "api_key" | "secret_id_key" | "aws_config" | "none";

export type ProviderPreset = {
  key: string;
  label: string;
  providerCode: string;
  providerType: "openai" | "anthropic" | "gemini" | "ollama" | "hunyuan" | "huggingface" | "bedrock";
  variant?: "openai" | "deepseek" | "qwen" | "hunyuan";
  authType: AuthType;
  apiBaseUrl: string;
  metadataApi: "full" | "partial" | "limited" | "none";
  metadataNote: string;
  models: ProviderModelPreset[];
};
```

### 3.2 预设映射表

| 前端预设 key | 前端 label | trpc Provider 类型 | trpc Variant | authType | 说明 |
|-------------|-----------|-------------------|-------------|----------|------|
| `openai` | OpenAI | `openai` | `openai` | `api_key` | 标准 OpenAI |
| `anthropic` | Anthropic (Claude) | `anthropic` | — | `api_key` | 原生 Anthropic SDK |
| `gemini` | Google (Gemini) | `gemini` | — | `api_key` | 原生 Gemini SDK |
| `deepseek` | DeepSeek | `openai` | `deepseek` | `api_key` | OpenAI 兼容 + DeepSeek Variant |
| `aliyun-qwen` | 阿里云 (通义千问) | `openai` | `qwen` | `api_key` | OpenAI 兼容 + Qwen Variant |
| `tencent-hunyuan` | 腾讯云 (混元) | `hunyuan` | — | `secret_id_key` | 混元私有协议 |
| `ollama` | Ollama | `ollama` | — | `none` | 本地模型服务 |
| `groq` | Groq | `openai` | `openai` | `api_key` | OpenAI 兼容 |
| `azure` | Azure OpenAI | `openai` | `openai` | `api_key` | OpenAI 兼容 |
| `mistral` | Mistral AI | `openai` | `openai` | `api_key` | OpenAI 兼容 |
| `together` | Together AI | `openai` | `openai` | `api_key` | OpenAI 兼容 |
| `fireworks` | Fireworks AI | `openai` | `openai` | `api_key` | OpenAI 兼容 |
| `openrouter` | OpenRouter | `openai` | `openai` | `api_key` | 聚合平台 |
| `baidu-qianfan` | 百度智能云 (千帆) | `openai` | `openai` | `api_key` | OpenAI 兼容 |
| `zhipu-glm` | 智谱AI (GLM) | `openai` | `openai` | `api_key` | OpenAI 兼容 |
| `meta-llama` | Meta (Llama) | `openai` | `openai` | `api_key` | OpenAI 兼容 |
| `moonshot-kimi` | 月之暗面 (Kimi) | `openai` | `openai` | `api_key` | OpenAI 兼容 |
| `volcengine-doubao` | 字节跳动 (豆包) | `openai` | `openai` | `api_key` | OpenAI 兼容 |
| `cohere` | Cohere | `openai` | `openai` | `api_key` | OpenAI 兼容 |
| `iflytek-spark` | 科大讯飞 (星火) | `openai` | `openai` | `api_key` | WebSocket 转发 |
| `stability` | Stability AI | `openai` | `openai` | `api_key` | 图像生成 |
| `huggingface` | HuggingFace | `huggingface` | — | `api_key` | HF Inference（⏳ 待注册） |
| `bedrock` | AWS Bedrock | `bedrock` | — | `aws_config` | AWS Bedrock Runtime（⏳ 待注册） |
| `custom` | 自定义 | `openai` | `openai` | `api_key` | 手动配置 |

### 3.3 authType 与 UI 表单联动

| authType | 显示的认证字段 | 隐藏的认证字段 |
|----------|--------------|--------------|
| `api_key` | API 基础 URL、API 密钥 | Secret ID/Key、AWS Region |
| `secret_id_key` | API 基础 URL、Secret ID、Secret Key | API 密钥、AWS Region |
| `aws_config` | AWS Region | API 基础 URL、API 密钥、Secret ID/Key |
| `none` | API 基础 URL（Host） | API 密钥、Secret ID/Key、AWS Region |

### 3.4 前端 `providerTypeOptions` 更新

当前前端 `providerTypeOptions` 为自由文本：
```typescript
// 当前（旧）
const providerTypeOptions = [
  { label: "OpenAI Compatible", value: "OpenAI Compatible" },
  { label: "Anthropic", value: "Anthropic" },
  { label: "Google Gemini", value: "Google Gemini" },
  { label: "Azure OpenAI", value: "Azure OpenAI" },
  { label: "Ollama", value: "Ollama" },
  { label: "自定义", value: "Custom" }
];
```

更新为 trpc Provider 类型枚举：
```typescript
// 新（对齐 trpc）
const providerTypeOptions = [
  { label: "OpenAI Compatible", value: "openai" },
  { label: "Anthropic", value: "anthropic" },
  { label: "Gemini", value: "gemini" },
  { label: "Ollama", value: "ollama" },
  { label: "Hunyuan", value: "hunyuan" },
  { label: "HuggingFace", value: "huggingface" },
  { label: "Bedrock", value: "bedrock" }
];
```

---

## 4. 列表页布局

| 区域 | 说明 |
|------|------|
| **标题区** | 主标题 **Provider**；副标题 **管理 LLM Provider**；右上 **`+ 添加Provider`**（主色 `QBtn`） |
| **搜索** | `QInput` 带搜索图标；占位 **搜索Provider…**；对 `provider_code`、`name`、`model_api_id` 等做前端过滤或 `GET ?q=` |
| **Provider 类型筛选** | `QSelect` 多选；选项为 trpc Provider 类型：OpenAI Compatible / Anthropic / Gemini / Ollama / Hunyuan / HuggingFace / Bedrock |
| **列表** | `QList` 卡片行；见 §5 |
| **分页** | 底部：总条数、`per_page`（如 20）、翻页 |

---

## 5. 列表行（一行对应 `llm_provider_models` 一条模型）

从左到右建议布局：

| 元素 | 说明 |
|------|------|
| **图标 + 名称** | 小图标（如芯片）+ **`provider_code`** + **`model_display_name` 或 `model_api_id`** + **状态绿点**（连接正常/已配置等，规则由后端定） |
| **Provider 类型** | `QChip` 展示 trpc Provider 类型：**OpenAI** / **Anthropic** / **Gemini** / **Ollama** / **Hunyuan** / **HuggingFace** / **Bedrock**；不同类型用不同颜色区分 |
| **Variant** | 仅 Provider 类型 = OpenAI 时显示：`QChip` 小标签，如 **DeepSeek** / **Qwen** / **Hunyuan**；非 OpenAI 类型或 Variant = openai 时隐藏 |
| **模型分类** | 展示当前模型所选分类的 **`label`**（展示名），可用 **`QChip`**；**鼠标悬停**时 **`QTooltip`** 显示该类型的 **一句话说明** |
| **模型大小** | 只读展示 **`model_size_label`**（如 `7B` / `70B`）；空则 **「—」** 或隐藏 |
| **上下文** | 只读展示 **`context_window_k`**（如 `128K`）；帮助用户快速判断长文本能力 |
| **热度** | 展示 **`model_hotness_score`**（0～100），用 **进度条 + 等级文案** 表达近期使用活跃度；见 §5.1 |
| **近 30 天调用** | 展示 **`usage_call_count_30d`**；暂无统计时展示 **「—」** |
| **近 30 天费用** | 展示 **`usage_cost_micro_usd_30d`** 格式化后的费用；暂无统计时展示 **「—」** |
| **TPS** | 只读展示 **`tokens_per_second`**；空则 **「—」** |
| **成功率 / 延迟** | 可选展示 **`success_rate_30d`**、**`avg_latency_ms_30d`** |
| **API 密钥** | 钥匙图标 + **已设置API密钥** / **未设置**（未设置可标橙或警告色） |
| **高可用** | 若配置了 Failover/Hedge，显示 `QChip`：**Failover**（蓝色）/ **Hedge**（紫色）；Tooltip 显示候选模型列表 |
| **启用** | **`QToggle`**：开关 ON = 启用，OFF = 停用；变更即 **PATCH** `is_enabled` |
| **历史趋势** | **`QBtn` flat** `query_stats` 图标或文案 **趋势**；打开该模型的历史趋势看板（见 §5.2） |
| **编辑** | **`QBtn` flat** 文案 **编辑** 或 `edit` 图标 |
| **删除** | **`QBtn` flat** `delete` 图标；二次确认后 **DELETE** 或软删 |

**交互**：

- 点击 **编辑** → 打开与「添加」同构的 **编辑弹窗**（§6），`GET /llm-provider-models/:id` 预填当前行。
- 列表行内 **开关** 仅切换启用；不打开弹窗。
- 点击 **趋势** → 打开模型历史趋势看板。

### 5.1 模型热度显示

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

### 5.2 历史趋势看板

点击列表行 **趋势** 按钮打开 `QDialog` / 右侧抽屉 / 独立页面。

建议展示：

| 模块 | 内容 |
|------|------|
| 顶部摘要 | 模型名、Provider、热度、30 天调用、30 天 Token、30 天费用 |
| 趋势图 | 调用次数趋势、Token 趋势、费用趋势、平均延迟趋势 |
| 占比 | 该模型在全部模型中的调用占比、Token 占比、费用占比 |
| 性能 | 平均 TPS、平均延迟、P95 延迟、成功率、失败次数 |
| 最近调用 | 最近 20 条调用记录：时间、Agent、Token、费用、状态、耗时 |

看板入口可先实现为弹窗占位，后续接入 `model_token_usage_events` / `model_token_usage_daily` 后展示真实图表。

### 5.3 列表字段建议分层

| 层级 | 字段 | 说明 |
|------|------|------|
| 第一优先级 | 名称、Provider 类型、模型分类、热度、启用、编辑/删除 | 日常管理必看 |
| 第二优先级 | Variant、模型大小、上下文、最大输出 Token、TPS、高可用 | 选模和性能判断 |
| 第三优先级 | 30 天调用、30 天费用、成功率、平均延迟 | 使用情况和成本判断 |
| 展开/Tooltip | 最近调用时间、失败次数、费用占比、Token 占比 | 详情辅助信息 |

---

## 6. 添加 / 编辑 Provider 弹窗

`QDialog`；标题 **添加Provider** / **编辑Provider**；副标题 **配置 LLM Provider 连接**。

弹窗分为四个步骤/标签页：**① 连接与身份** → **② 模型分类与规格** → **③ 高可用配置** → **④ 高级选项**。

### 6.1 连接与身份

| 字段 | 控件 | 校验 / 说明 |
|------|------|-------------|
| **Provider 预设** | `QSelect` | 选项来自 `PROVIDER_PRESETS`；选择后自动填充下方字段 |
| **Provider 类型** * | `QSelect` | 选项：OpenAI Compatible / Anthropic / Gemini / Ollama / Hunyuan / HuggingFace / Bedrock；选择后自动切换下方表单字段和 Variant 选项 |
| **Variant** | `QSelect` | 仅 Provider 类型 = OpenAI Compatible 时显示；选项：OpenAI / DeepSeek / Qwen / Hunyuan；默认 OpenAI；选择后自动预填默认 Base URL |
| **Provider 编码** * | `QInput` | 占位「例如 openrouter」；**小写字母、数字、连字符**；映射 `provider_code`；同一厂商多模型时多行共用同一 `provider_code` |
| **Provider 显示名** | `QInput` | 映射 `config_json.provider_display_name` |
| **模型 API ID** * | `QInput` | 映射 `model_api_id`；与厂商文档一致 |
| **模型展示名** | `QInput`（可选） | 映射 `metadata_json.model_display_name` |
| **API 基础 URL** | `QInput` | `https://…`；根据 Provider 类型 + Variant 自动预填默认值（见 §2.1 和 §2.2） |
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

### 6.2 模型分类与规格

| 字段 | 控件 | 说明 | 数据库字段 |
|------|------|------|-----------|
| **模型分类** | `QSelect` multiple | 选项见 §6.2.1 | `config_json.model_category` |
| **模型大小标签** | `QInput` | 如 `7B` / `70B` | `config_json.model_size_label` |
| **上下文窗口** | `QInput type=number` suffix `K tokens` | 模型上下文窗口大小（千 Token） | `config_json.context_window_k` |
| **最大输出 Token** | `QInput type=number` | 单次最大输出 Token | `config_json.max_output_tokens` |
| **输入价格** | `QInput type=number` suffix `µ$/1K token` | 每百万输入 Token 的微美元单价 | `config_json.input_price_micro_usd_per_1k` |
| **输出价格** | `QInput type=number` suffix `µ$/1K token` | 每百万输出 Token 的微美元单价 | `config_json.output_price_micro_usd_per_1k` |
| **缓存输入价格** | `QInput type=number` suffix `µ$/1K token` | 缓存输入 Token 单价（Anthropic 等） | `config_json.cached_input_price_micro_usd_per_1k` |
| **推理价格** | `QInput type=number` suffix `µ$/1K token` | 推理 Token 单价（DeepSeek 等） | `config_json.reasoning_price_micro_usd_per_1k` |
| **嵌入价格** | `QInput type=number` suffix `µ$/1K token` | 嵌入 Token 单价 | `config_json.embedding_price_micro_usd_per_1k` |
| **检查模型** | `QBtn` | 调用 `POST /v1/llm-provider-models/inspect`，自动回填上述字段 | — |

#### 6.2.1 模型分类选项

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

### 6.3 高可用配置

| 字段 | 控件 | 说明 | 数据库字段 |
|------|------|------|-----------|
| **高可用模式** | `QSelect` | 选项：无 / Failover / Hedge | `config_json.ha_mode` |
| **候选模型** | `QBtn` + 动态列表 | 添加候选模型条目（模型名 + Base URL + API Key） | `config_json.ha_candidates` |
| **Hedge 延迟** | `QInput type=number` suffix `ms` | 仅 Hedge 模式；候选请求延迟启动间隔（默认 100ms） | `config_json.ha_hedge_delay_ms` |

**候选模型条目结构**：

```jsonc
{
  "ha_candidates": [
    { "name": "gpt-4o", "base_url": "https://backup.example.com/v1", "api_key": "sk-backup" },
    { "name": "gpt-4o-mini", "base_url": "https://api.openai.com/v1", "api_key": "sk-fallback" }
  ]
}
```

### 6.4 高级选项

| 字段 | 控件 | 说明 | 数据库字段 | 适用 Provider |
|------|------|------|-----------|--------------|
| **Token Tailoring** | `QToggle` | 自动裁剪输入 Token 以适应上下文窗口 | `config_json.enable_token_tailoring` | 全部 |
| **Prompt Cache 优化** | `QToggle` | 将 system 消息前置以提高缓存命中率 | `config_json.optimize_for_cache` | OpenAI |
| **Reasoning 回填** | `QToggle` | 为无推理内容的 assistant 消息回填空 reasoning_content | `config_json.reasoning_content_backfill` | OpenAI (DeepSeek) |
| **Tool Call Delta** | `QToggle` | 流式响应中暴露 tool_call 增量 | `config_json.show_tool_call_delta` | OpenAI, Anthropic |
| **System Prompt Cache** | `QToggle` | 缓存系统提示（90% 输入折扣） | `config_json.cache_system_prompt` | Anthropic |
| **Tools Cache** | `QToggle` | 缓存工具定义（90% 输入折扣） | `config_json.cache_tools` | Anthropic |
| **Messages Cache** | `QToggle` | 多轮对话缓存 | `config_json.cache_messages` | Anthropic |
| **Keep Alive** | `QInput type=number` + suffix `分钟` | 模型保持加载的时长 | `config_json.keep_alive_minutes` | Ollama |
| **Ollama Options** | `QInput` JSON 编辑器 | Ollama API 额外参数 | `config_json.ollama_options` | Ollama |
| **Extra Headers** | `QInput` JSON 编辑器 | 额外 HTTP 头 | `config_json.extra_headers` | OpenAI, Anthropic, HuggingFace |
| **Extra Fields** | `QInput` JSON 编辑器 | 额外请求体字段 | `config_json.extra_fields` | OpenAI, HuggingFace |
| **Channel Buffer Size** | `QInput type=number` | 响应通道缓冲区大小（默认 256） | `config_json.channel_buffer_size` | 全部 |

---

## 7. 数据表设计

### 7.1 主表：`llm_provider_models`

**一行 = 一条「某厂商连接下的一个可选模型」**。连接字段在 **同一 `provider_code` 的多行之间重复存储**。

| 字段名 | 类型 | 说明 |
|--------|------|------|
| `id` | STRING PK | UUID 或雪花 |
| `model_key` | STRING UNIQUE | 业务键（兼容旧字段） |
| `name` | STRING | 展示名 |
| `description` | TEXT | 描述 |
| `status` | STRING | 状态：active / deleted |
| `enabled` | BOOL | 是否启用 |
| `sort_order` | INT | 同一 `provider_code` 内的展示顺序 |
| `provider` | STRING | `provider_code`，小写 slug |
| `model` | STRING | `model_api_id`，与厂商文档一致 |
| `config_json` | TEXT | JSON；结构见 §7.2 |
| `metadata_json` | TEXT | JSON；结构见 §7.3 |
| `created_at` | STRING | 创建时间 |
| `updated_at` | STRING | 更新时间 |
| `deleted_at` | STRING | 软删时间 |

**唯一约束**：`(provider, model)` 联合唯一。

**实现注意**：修改 **API 地址/密钥** 时，应对 `WHERE provider = ?` 批量更新，避免同厂商各行连接信息漂移。

### 7.2 `config_json` 结构

`config_json` 存储 Provider 连接配置和 trpc 运行时选项，按 `provider_type` 分化：

```jsonc
{
  // ─── 连接与身份 ───
  "provider_type": "openai",           // trpc Provider 类型枚举：openai | anthropic | gemini | ollama | hunyuan | huggingface | bedrock
  "variant": "deepseek",               // OpenAI Variant：openai | deepseek | qwen | hunyuan（仅 provider_type=openai 时有效）
  "provider_display_name": "DeepSeek", // UI 展示名
  "api_base_url": "https://api.deepseek.com",
  "api_key": "sk-...",                 // 写入后不回读；编辑时用 api_key_set 标记
  "api_key_set": true,                 // 标记是否已设置密钥（编辑时判断）
  "secret_id": "",                     // Hunyuan 专用
  "secret_key": "",                    // Hunyuan 专用
  "aws_region": "",                    // Bedrock 专用

  // ─── 模型规格 ───
  "model_category": [                  // 模型能力分类
    { "value": "reasoning", "label": "推理 / 复杂问题", "tooltip": "数学、逻辑、多步推导" }
  ],
  "model_size_label": "67B",
  "context_window_k": 128,
  "max_output_tokens": 8192,

  // ─── 定价 ───
  "input_price_micro_usd_per_1k": 550,
  "output_price_micro_usd_per_1k": 2190,
  "cached_input_price_micro_usd_per_1k": 0,
  "reasoning_price_micro_usd_per_1k": 2190,
  "embedding_price_micro_usd_per_1k": 0,

  // ─── 高可用 ───
  "ha_mode": "",                       // "" | "failover" | "hedge"
  "ha_candidates": [                   // 候选模型列表
    { "name": "gpt-4o", "base_url": "https://backup.example.com/v1", "api_key": "sk-backup" }
  ],
  "ha_hedge_delay_ms": 100,            // Hedge 延迟（ms）

  // ─── 高级选项 ───
  "enable_token_tailoring": true,
  "optimize_for_cache": false,          // OpenAI 专用
  "reasoning_content_backfill": true,   // OpenAI (DeepSeek) 专用
  "show_tool_call_delta": false,        // OpenAI, Anthropic
  "cache_system_prompt": false,         // Anthropic 专用
  "cache_tools": false,                 // Anthropic 专用
  "cache_messages": false,              // Anthropic 专用
  "keep_alive_minutes": 5,              // Ollama 专用
  "ollama_options": {},                 // Ollama 专用
  "extra_headers": {},                  // OpenAI, Anthropic, HuggingFace
  "extra_fields": {},                   // OpenAI, HuggingFace
  "channel_buffer_size": 256,           // 全部

  // ─── 运营统计（由后端统计服务写入） ───
  "tokens_per_second": null,
  "model_hotness_score": null,
  "usage_call_count_30d": null,
  "usage_total_tokens_30d": null,
  "usage_cost_micro_usd_30d": null,
  "success_rate_30d": null,
  "avg_latency_ms_30d": null,
  "last_used_at": "",
  "model_rating": 60,

  // ─── 元数据来源 ───
  "raw_metadata_json": "",
  "metadata_source": ""
}
```

#### 7.2.1 `config_json` 按 Provider 类型必填/选填矩阵

| 字段 | openai | anthropic | gemini | ollama | hunyuan | huggingface | bedrock |
|------|--------|-----------|--------|--------|---------|-------------|---------|
| `provider_type` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `variant` | 选填 | — | — | — | — | — | — |
| `api_base_url` | ✅ | ✅ | ✅ | ✅（Host） | ✅ | ✅ | — |
| `api_key` | ✅ | ✅ | ✅ | — | — | ✅ | — |
| `secret_id` | — | — | — | — | ✅ | — | — |
| `secret_key` | — | — | — | — | ✅ | — | — |
| `aws_region` | — | — | — | — | — | — | ✅ |
| `optimize_for_cache` | 选填 | — | — | — | — | — | — |
| `reasoning_content_backfill` | 选填 | — | — | — | — | — | — |
| `show_tool_call_delta` | 选填 | 选填 | — | — | — | — | — |
| `cache_system_prompt` | — | 选填 | — | — | — | — | — |
| `cache_tools` | — | 选填 | — | — | — | — | — |
| `cache_messages` | — | 选填 | — | — | — | — | — |
| `keep_alive_minutes` | — | — | — | 选填 | — | — | — |
| `ollama_options` | — | — | — | 选填 | — | — | — |
| `extra_headers` | 选填 | 选填 | — | — | — | 选填 | — |
| `extra_fields` | 选填 | — | — | — | — | 选填 | — |

### 7.3 `metadata_json` 结构

`metadata_json` 存储 UI 展示和统计信息，与运行时无关：

```jsonc
{
  "model_rating": 60,           // 用户评分 0-100
  "model_display_name": ""      // 模型展示名覆盖
}
```

---

## 8. 后端运行时：从 config_json 到 trpc model.Model

### 8.1 当前实现

当前 `internal/provider/trpc_llm.go` 仅支持 OpenAI Provider，通过 `newOpenAIModel` 构建 `openai.New()` 实例，并支持 Failover/Hedge 包装。

### 8.2 目标实现：按 provider_type 分发

```go
func trpcModelFromCatalogConfig(cfg CatalogConfig, rt *RoundTrip) (trpcmodel.Model, error) {
    switch cfg.ProviderType {
    case "openai":
        return buildOpenAIModel(cfg, rt)
    case "anthropic":
        return buildAnthropicModel(cfg, rt)
    case "gemini":
        return buildGeminiModel(cfg, rt)
    case "ollama":
        return buildOllamaModel(cfg, rt)
    case "hunyuan":
        return buildHunyuanModel(cfg, rt)
    case "huggingface":
        return buildHuggingFaceModel(cfg, rt)
    case "bedrock":
        return buildBedrockModel(cfg, rt)
    default:
        return buildOpenAIModel(cfg, rt) // 兜底走 OpenAI 兼容
    }
}
```

### 8.3 各 Provider 构建逻辑

#### OpenAI

```go
func buildOpenAIModel(cfg CatalogConfig, rt *RoundTrip) (trpcmodel.Model, error) {
    opts := []trpcopenai.Option{}
    if cfg.BaseURL != "" { opts = append(opts, trpcopenai.WithBaseURL(cfg.BaseURL)) }
    if cfg.APIKey != "" { opts = append(opts, trpcopenai.WithAPIKey(cfg.APIKey)) }
    if cfg.Variant != "" { opts = append(opts, trpcopenai.WithVariant(trpcopenai.Variant(cfg.Variant))) }
    if cfg.EnableTokenTailoring { opts = append(opts, trpcopenai.WithEnableTokenTailoring(true)) }
    if cfg.ContextWindow > 0 { opts = append(opts, trpcopenai.WithContextWindow(cfg.ContextWindow)) }
    // ... 其他选项
    m := trpcopenai.New(cfg.ModelAPI, opts...)
    return wrapHA(m, cfg, rt) // Failover/Hedge 包装
}
```

#### Anthropic

```go
func buildAnthropicModel(cfg CatalogConfig, rt *RoundTrip) (trpcmodel.Model, error) {
    opts := []trpcanthropic.Option{}
    if cfg.BaseURL != "" { opts = append(opts, trpcanthropic.WithBaseURL(cfg.BaseURL)) }
    if cfg.APIKey != "" { opts = append(opts, trpcanthropic.WithAPIKey(cfg.APIKey)) }
    if cfg.CacheSystemPrompt { opts = append(opts, trpcanthropic.WithCacheSystemPrompt(true)) }
    if cfg.CacheTools { opts = append(opts, trpcanthropic.WithCacheTools(true)) }
    if cfg.CacheMessages { opts = append(opts, trpcanthropic.WithCacheMessages(true)) }
    // ... 其他选项
    return trpcanthropic.New(cfg.ModelAPI, opts...), nil
}
```

#### Gemini

```go
func buildGeminiModel(cfg CatalogConfig, rt *RoundTrip) (trpcmodel.Model, error) {
    opts := []trpcgemini.Option{}
    if cfg.APIKey != "" { /* Gemini 通过 ClientConfig 传入 APIKey */ }
    if cfg.EnableTokenTailoring { opts = append(opts, trpcgemini.WithEnableTokenTailoring(true)) }
    // ... 其他选项
    return trpcgemini.New(context.Background(), cfg.ModelAPI, opts...), nil
}
```

#### Ollama

```go
func buildOllamaModel(cfg CatalogConfig, rt *RoundTrip) (trpcmodel.Model, error) {
    opts := []trpcollama.Option{}
    if cfg.BaseURL != "" { opts = append(opts, trpcollama.WithHost(cfg.BaseURL)) }
    if cfg.ContextWindow > 0 { opts = append(opts, trpcollama.WithContextWindow(cfg.ContextWindow)) }
    // ... 其他选项
    return trpcollama.New(cfg.ModelAPI, opts...), nil
}
```

#### Hunyuan

```go
func buildHunyuanModel(cfg CatalogConfig, rt *RoundTrip) (trpcmodel.Model, error) {
    opts := []trpchunyuan.Option{}
    if cfg.BaseURL != "" { opts = append(opts, trpchunyuan.WithHost(cfg.BaseURL)) }
    if cfg.SecretID != "" { opts = append(opts, trpchunyuan.WithSecretId(cfg.SecretID)) }
    if cfg.SecretKey != "" { opts = append(opts, trpchunyuan.WithSecretKey(cfg.SecretKey)) }
    // ... 其他选项
    return trpchunyuan.New(cfg.ModelAPI, opts...), nil
}
```

### 8.4 CatalogConfig 扩展

当前 `CatalogConfig` 仅包含 `ProviderType`、`BaseURL`、`APIKey`、`ModelAPI`。需扩展以支持所有 Provider 的配置：

```go
type CatalogConfig struct {
    ProviderType           string
    Variant                string
    BaseURL                string
    APIKey                 string
    ModelAPI               string
    SecretID               string // Hunyuan
    SecretKey              string // Hunyuan
    AWSRegion              string // Bedrock
    EnableTokenTailoring   bool
    ContextWindow          int
    MaxInputTokens         int
    OptimizeForCache       bool   // OpenAI
    ReasoningBackfill      bool   // OpenAI (DeepSeek)
    ShowToolCallDelta      bool   // OpenAI, Anthropic
    CacheSystemPrompt      bool   // Anthropic
    CacheTools             bool   // Anthropic
    CacheMessages          bool   // Anthropic
    KeepAliveMinutes       int    // Ollama
    OllamaOptions          map[string]any // Ollama
    ExtraHeaders           map[string]string
    ExtraFields            map[string]any
    ChannelBufferSize      int
    HAMode                 string // "" | "failover" | "hedge"
    HACandidates           []HACandidateConfig
    HAHedgeDelayMs         int
}
```

---

## 9. API 设计

### 9.1 现有 API（保持兼容）

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/v1/llm-provider-models` | 列表 |
| POST | `/v1/llm-provider-models` | 创建 |
| GET | `/v1/llm-provider-models/{id}` | 详情 |
| PATCH | `/v1/llm-provider-models/{id}` | 更新 |
| DELETE | `/v1/llm-provider-models/{id}` | 删除 |
| POST | `/v1/llm-provider-models/inspect` | 检查模型连通性并回填元数据 |
| POST | `/v1/agents/validate-model` | 验证 Provider+Model 对是否可用 |

### 9.2 Inspect 请求扩展

`InspectProviderModelRequest` 新增字段：

| 字段 | 类型 | 说明 |
|------|------|------|
| `provider_type` | string | trpc Provider 类型枚举 |
| `variant` | string | OpenAI Variant（仅 provider_type=openai 时有效） |
| `secret_id` | string | Hunyuan SecretId |
| `secret_key` | string | Hunyuan SecretKey |
| `aws_region` | string | Bedrock AWS Region |

### 9.3 Inspect 响应扩展

`InspectProviderModelResponse` 新增字段：

| 字段 | 类型 | 说明 |
|------|------|------|
| `variant` | string | 检测到的 OpenAI Variant |
| `context_window` | int32 | 上下文窗口大小（Token） |
| `enable_token_tailoring` | bool | 是否启用 Token Tailoring |
| `supports_cache` | bool | 是否支持 Prompt Cache |
| `supports_thinking` | bool | 是否支持思考/推理模式 |

---

## 10. 前端改造清单

### 10.1 `providerPresets.ts` 改造

1. **`ProviderPreset.providerType`**：从自由文本改为 trpc 枚举值
   - `"OpenAI Compatible"` → `"openai"`
   - `"Anthropic"` → `"anthropic"`
   - `"Google Gemini"` → `"gemini"`
   - `"Ollama"` → `"ollama"`
   - `"Custom"` → `"openai"`（自定义走 OpenAI 兼容）

2. **新增 `ProviderPreset.variant`** 字段
   - `deepseek` 预设：`variant: "deepseek"`
   - `aliyun-qwen` 预设：`variant: "qwen"`
   - 其他 OpenAI 兼容：`variant: "openai"` 或不设置

3. **新增 `ProviderPreset.authType`** 字段
   - 大部分预设：`authType: "api_key"`
   - `tencent-hunyuan`：`authType: "secret_id_key"`
   - `ollama`：`authType: "none"`
   - `bedrock`：`authType: "aws_config"`

4. **新增 `ProviderPreset.providerTypeLabel`** 字段（UI 展示用）
   - `"openai"` → `"OpenAI Compatible"`
   - `"anthropic"` → `"Anthropic"`
   - `"gemini"` → `"Gemini"`
   - `"ollama"` → `"Ollama"`
   - `"hunyuan"` → `"Hunyuan"`

### 10.2 `ResourceManagerPage.vue` 改造

1. **`providerTypeOptions`** 更新为 trpc 枚举值（见 §3.4）
2. **`providerForm`** 新增 `variant`、`secret_id`、`secret_key`、`aws_region` 字段
3. **表单联动**：根据 `providerType` + `authType` 动态显示/隐藏认证字段
4. **Variant 选择**：Provider 类型 = openai 时显示 Variant 下拉
5. **高可用配置**：新增 HA 模式选择和候选模型管理
6. **高级选项**：按 Provider 类型显示对应的高级选项
7. **`buildProviderPayload()`**：将新字段写入 `config_json`

### 10.3 `ProviderModelRow.vue` 改造

1. **Provider 类型 Chip**：使用 trpc 枚举值映射的展示名
2. **Variant Chip**：仅 OpenAI 类型且 Variant ≠ openai 时显示
3. **高可用 Chip**：显示 Failover/Hedge 状态

---

## 11. 后端改造清单

### 11.1 `internal/provider/catalog.go`

- `CatalogConfig` 扩展（见 §8.4）
- `catalogConfigJSON` 扩展以解析新字段
- `MergeCatalogConfig` 支持新字段覆盖

### 11.2 `internal/provider/trpc_llm.go`

- `trpcModelFromCatalogConfig` 按 `provider_type` 分发（见 §8.2）
- 新增 `buildAnthropicModel`、`buildGeminiModel`、`buildOllamaModel`、`buildHunyuanModel` 等构建函数
- `parseFailoverModels` / `parseHedgeModels` 从 `config_json` 解析 HA 配置

### 11.3 `internal/biz/llm_provider_model.go`

- `InspectMerge` 新增 `variant`、`secret_id`、`secret_key`、`aws_region` 字段
- `Inspect` 方法根据 `provider_type` 选择不同的检查逻辑

### 11.4 `api/kratos/llm_provider_model/v1/llm_provider_model.proto`

- `InspectProviderModelRequest` 新增字段
- `InspectProviderModelResponse` 新增字段

---

## 12. 验收要点

### 12.1 功能验收

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

### 12.2 数据兼容性

| # | 验收项 | 优先级 |
|---|--------|--------|
| 1 | 旧数据 `provider_type: "OpenAI Compatible"` 自动映射为 `"openai"` | P0 |
| 2 | 旧数据 `provider_type: "Anthropic"` 自动映射为 `"anthropic"` | P0 |
| 3 | 旧数据 `provider_type: "Google Gemini"` 自动映射为 `"gemini"` | P0 |
| 4 | 旧数据 `provider_type: "Ollama"` 自动映射为 `"ollama"` | P0 |
| 5 | 旧数据 `provider_type: "Custom"` 兜底为 `"openai"` | P0 |
| 6 | 旧数据无 `variant` 字段时，OpenAI 类型默认 Variant = `"openai"` | P1 |

### 12.3 性能验收

| # | 验收项 | 优先级 |
|---|--------|--------|
| 1 | Provider 列表加载 < 500ms | P1 |
| 2 | 模型检查（Inspect）响应 < 5s | P1 |
| 3 | Failover 切换延迟 < 1s | P2 |

---

*文档版本：基于 trpc-agent-go `model/provider` 体系重新设计；与 `8 agent-title.md` §8.1、`2 agents-create.md` 对齐。*
