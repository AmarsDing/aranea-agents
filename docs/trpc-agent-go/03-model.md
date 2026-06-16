# 模型（Model）— 框架对齐分析

> 模块路径：`pkg/trpc-agent-go/model/`
> 项目实现路径：`internal/provider/`、`internal/agent/`、`internal/biz/llm_provider_model.go`、`internal/data/llm_provider_model.go`
> 当前对齐度：★★★★☆

---

## 一、框架能力全景

### 1.1 核心接口

| 接口 | 方法 | 说明 |
|------|------|------|
| `model.Model` | `GenerateContent(ctx, *Request) (<-chan *Response, error)` | 核心调用接口，返回响应 channel |
| `model.Model` | `Info() Info` | 返回模型名称和上下文窗口信息 |
| `model.IterModel` | `GenerateContentIter(ctx, *Request) (Seq[*Response], error)` | 迭代器模式扩展，减少 goroutine/channel 开销 |
| `model.FileDownloader` | `DownloadFile(ctx, fileID) ([]byte, string, error)` | 可选接口，文件下载（OpenAI 实现） |
| `model.TokenCounter` | `CountTokens(ctx, Message) (int, error)` | Token 计数接口 |
| `model.TokenCounter` | `CountTokensRange(ctx, []Message, start, end int) (int, error)` | 范围计数 |
| `model.TailoringStrategy` | `TailorMessages(ctx, []Message, maxTokens int) ([]Message, error)` | 裁剪策略接口 |
| `provider.Provider` | `func(opts *Options) (model.Model, error)` | Provider 工厂函数签名 |

### 1.2 关键类型

| 类型 | 说明 |
|------|------|
| `model.Info` | 模型元信息：`Name` + `ContextWindow` |
| `model.Message` | 统一消息结构：Role/Content/ContentParts/ToolCalls/ToolID/ReasoningContent 等 |
| `model.Request` | 请求结构：Messages + GenerationConfig + StructuredOutput + Tools + ExtraFields + Headers |
| `model.GenerationConfig` | 生成参数：MaxTokens/Temperature/TopP/Stream/Stop/ReasoningEffort/ThinkingEnabled/ThinkingTokens 等 |
| `model.GenerationConfigPatch` | per-request 覆盖，nil 字段表示"不覆盖" |
| `model.Response` | 响应结构：Choices/Usage/Error/Done/IsPartial/Timestamp |
| `model.Usage` | Token 用量：PromptTokens/CompletionTokens/TotalTokens + PromptTokensDetails（CachedTokens/CacheReadTokens）+ CompletionTokensDetails（ReasoningTokens）+ TimingInfo |
| `model.ToolCall` | 工具调用：Type/Function/ID/Index |
| `model.StructuredOutput` | 结构化输出：Type + JSONSchema |
| `model.ResponseError` | API 级错误（与函数级 error 分离） |
| `model.TokenTailoringConfig` | Token 预算参数：SafetyMarginRatio/ReserveOutputTokens/ProtocolOverheadTokens 等 |

### 1.3 扩展点

| 扩展点 | 机制 | 适用场景 |
|--------|------|---------|
| 实现 `model.Model` 接口 | 自定义 Provider 后端 | 新增 LLM Provider |
| 实现 `model.IterModel` 接口 | 迭代器模式 | 减少 goroutine 开销 |
| 实现 `model.FileDownloader` 接口 | 文件下载能力 | 支持文件下载的 Provider |
| 实现 `model.TokenCounter` 接口 | 自定义 Token 计数 | 精确 Token 计算 |
| 实现 `model.TailoringStrategy` 接口 | 自定义裁剪策略 | 非默认裁剪算法 |
| `provider.Register(name, Provider)` | 注册自定义 Provider | 扩展 Provider 工厂 |
| `model.RegisterModelContextWindow` | 注册模型上下文窗口 | 新模型窗口信息 |
| `model.BeforeModelCallbackStructured` | 请求前回调 | 请求修改/拦截/日志 |
| `model.AfterModelCallbackStructured` | 响应后回调 | 响应修改/日志/指标 |
| `failover.New(WithCandidates(...))` | 故障转移包装 | 主备切换 |
| `hedge.New(WithCandidates/WithDelay)` | 对冲请求包装 | 降低首 token 延迟 |
| `agent.WithModelSelector(fn)` | 动态模型选择 | 每次 LLM 调用选择不同模型 |

### 1.4 配置选项

#### Provider 级 Option

| Option | 说明 | 默认值 |
|--------|------|--------|
| `provider.WithAPIKey(key)` | API 密钥 | 环境变量 |
| `provider.WithBaseURL(url)` | 基础 URL | Provider 默认 |
| `provider.WithVariant(variant)` | OpenAI 兼容变体 | — |
| `provider.WithHTTPClientName(name)` | HTTP 客户端逻辑名 | — |
| `provider.WithHTTPClientTransport(transport)` | HTTP Transport | http.DefaultTransport |
| `provider.WithHeaders(headers)` | 静态 HTTP 头 | — |
| `provider.WithCallbacks(cb)` | Provider 级回调 | — |
| `provider.WithChannelBufferSize(size)` | 响应 channel 缓冲区 | — |
| `provider.WithExtraFields(fields)` | 请求体额外字段 | — |
| `provider.WithEnableTokenTailoring(enabled)` | 启用自动 Token 裁剪 | false |
| `provider.WithMaxInputTokens(limit)` | 最大输入 Token 数 | 自动计算 |
| `provider.WithContextWindow(tokens)` | 上下文窗口大小 | 内置映射表 |
| `provider.WithTokenCounter(counter)` | 自定义 Token 计数器 | SimpleTokenCounter |
| `provider.WithTailoringStrategy(strategy)` | 自定义裁剪策略 | MiddleOutStrategy |
| `provider.WithTokenTailoringConfig(config)` | Token 预算参数 | 默认参数 |
| `provider.WithOpenAIOption(opt...)` | 原始 OpenAI Option | — |
| `provider.WithAnthropicOption(opt...)` | 原始 Anthropic Option | — |
| `provider.WithGeminiOption(opt...)` | 原始 Gemini Option | — |
| `provider.WithOllamaOption(opt...)` | 原始 Ollama Option | — |
| `provider.WithHunyuanOption(opt...)` | 原始 Hunyuan Option | — |

#### OpenAI 级 Option（部分）

| Option | 说明 |
|--------|------|
| `openai.WithVariant(variant)` | VariantOpenAI/VariantDeepSeek/VariantQwen/VariantHunyuan |
| `openai.WithShowToolCallDelta(show)` | 流式暴露 tool_call 增量 |
| `openai.WithReasoningContentBackfill(enabled)` | 回填空 reasoning_content |
| `openai.WithOptimizeForCache(optimize)` | 系统消息前置优化缓存 |
| `openai.WithBatchCompletionWindow(window)` | Batch API 完成窗口 |

#### Anthropic 级 Option（部分）

| Option | 说明 |
|--------|------|
| `anthropic.WithCacheSystemPrompt(cache)` | 缓存系统提示 |
| `anthropic.WithCacheTools(cache)` | 缓存工具定义 |
| `anthropic.WithCacheMessages(cache)` | 缓存多轮对话 |
| `anthropic.WithShowToolCallDelta(show)` | 流式暴露 tool call 增量 |

#### Failover/Hedge Option

| Option | 说明 |
|--------|------|
| `failover.WithCandidates(...)` | 主备候选 Model 列表 |
| `hedge.WithCandidates(...)` | 对冲候选 Model 列表 |
| `hedge.WithDelay(duration)` | 对冲延迟（默认 100ms） |
| `hedge.WithDelays(delays ...)` | 绝对偏移量延迟序列 |
| `hedge.WithContextWindow(tokens)` | 上下文窗口覆盖 |

### 1.5 框架内置实现

| 实现 | 路径 | 说明 |
|------|------|------|
| OpenAI Model | `model/openai/` | 含 DeepSeek/Qwen/Hunyuan Variant，Batch API，文件上传下载 |
| Anthropic Model | `model/anthropic/` | 含 Prompt Cache（3 断点）、Thinking 配置、自适应 Thinking 模型 |
| Gemini Model | `model/gemini/` | 含 MALFORMED_FUNCTION_CALL 自动重试、ThoughtSignature 透传 |
| Ollama Model | `model/ollama/` | 本地模型支持 |
| Hunyuan Model | `model/hunyuan/` | 腾讯混元（SecretID/SecretKey 认证） |
| Bedrock Model | `model/bedrock/` | AWS Bedrock |
| HuggingFace Model | `model/huggingface/` | HuggingFace Inference API |
| Failover 包装器 | `model/failover/` | 主备故障切换，首个非错误 chunk 前允许切换 |
| Hedge 包装器 | `model/hedge/` | 对冲首 token 尾延迟 |
| SimpleTokenCounter | `model/token_tailor.go` | 启发式计数：UTF-8 rune / approxRunesPerToken(4.0) |
| tiktoken Counter | `model/tiktoken/` | 精确计数：基于 tiktoken-go，按模型选择编码器 |
| MiddleOutStrategy | `model/token_tailor.go` | 中间裁剪（默认），保留头尾 |
| HeadOutStrategy | `model/token_tailor.go` | 头部裁剪（最旧优先） |
| TailOutStrategy | `model/token_tailor.go` | 尾部裁剪（最新优先） |
| 模型上下文窗口映射 | `model/internal/model/model_info.go` | 100+ 模型窗口信息，精确匹配→前缀匹配→fallback 128K |
| Provider 注册表 | `model/provider/` | 统一工厂入口，5 个内置 + 支持自定义注册 |

---

## 二、项目实现现状

### 2.1 框架接口实现情况

| 框架接口/功能 | 项目实现 | 合规性 | 说明 |
|--------------|---------|--------|------|
| `model.Model` 接口（使用侧） | ✅ 通过 `provider.Model()` 工厂创建 | ✅ | 所有 LLM 调用均委托框架 |
| `model.IterModel` 接口 | ❌ 未使用 | ⚠️ | 项目未利用迭代器模式降低开销 |
| `model.FileDownloader` 接口 | ❌ 未使用 | ⚠️ | 未使用文件下载能力 |
| `provider.Model()` 工厂 | ✅ 核心入口 | ✅ | `TRPCModelForProviderModel` 通过此工厂创建 |
| `provider.Register()` 注册 | ✅ 使用 | ✅ | 注册了 HuggingFace 和 Bedrock |
| `failover.New()` | ✅ 使用 | ✅ | `wrapFailover` 封装 |
| `hedge.New()` | ✅ 使用 | ✅ | `wrapHedge` 封装 |
| `agent.WithModelSelector()` | ✅ 深度使用 | ✅ | 5 种选择器 + 链式组合 |
| Token Tailoring | ✅ 使用 | ✅ | 通过 Provider Option 注入 |
| BeforeModel/AfterModel 回调 | ✅ 深度使用 | ✅ | 7 个 BeforeModel + 2 个 AfterModel Hook |
| Provider 级回调（ChatRequest 等） | ❌ 未使用 | ⚠️ | 未使用 4 种 Provider 级回调 |
| StructuredOutput | ⚠️ 仅能力标记 | ⚠️ | 目录中标记能力，未使用 `WithStructuredOutputJSON` |
| Batch API | ❌ 未使用 | — | 无业务场景 |
| 模型上下文窗口映射 | ✅ 使用 | ✅ | 通过 `WithContextWindow` 传入 |
| 消息验证与修复 | ✅ 框架自动 | ✅ | 框架内部自动调用 |

### 2.2 自建功能清单

| 自建功能 | 实现位置 | 替代框架功能 | 自建原因 |
|---------|---------|-------------|---------|
| 模型目录管理（CRUD + 凭据加密） | `internal/biz/llm_provider_model.go` + `internal/data/llm_provider_model.go` | 无 | 框架无模型目录管理能力，属于业务层功能 |
| 凭据加密（AES-256-GCM） | `internal/biz/credential_crypto.go` | 无 | 框架无凭据持久化/加密能力 |
| models.dev 目录同步 | `internal/biz/model_registry.go` + `internal/modelregistry/` | 无 | 框架无远程目录同步能力 |
| Provider 迁移 | `internal/data/model_registry_apply.go` | 无 | 框架无 Provider 代码迁移能力 |
| Rate Limit Transport（令牌桶） | `internal/provider/rate_limit_transport.go` | 无 | 框架无 HTTP 级限流机制 |
| Retry Transport（指数退避） | `internal/provider/retry_transport.go` | 框架 OpenAI SDK 内置重试 | 框架重试仅限 OpenAI SDK，项目需跨 Provider 统一重试 |
| Circuit Breaker Transport（熔断器） | `internal/provider/circuit_breaker_transport.go` + `internal/biz/tool/circuit_breaker.go` | 无 | 框架无熔断器机制 |
| Metrics Model（指标采集装饰器） | `internal/provider/metrics_model.go` | 无 | 框架无 Provider 级指标采集 |
| Capabilities 能力查询 | `internal/provider/capabilities.go` | 无 | 框架无模型能力元数据管理 |
| 5 种 ModelSelector 策略 | `internal/agent/model_selector.go` | 框架仅定义 `ModelSelector` 接口 | 框架提供接口，项目实现业务策略（合理） |
| Callback Chain 适配层 | `internal/agent/callbacks/` + `internal/agent/callback_chain.go` | 框架 `model.Callbacks` | 项目需要优先级排序、分层、插件注册等框架不支持的编排能力 |
| LLMCaller（非 Agent 场景） | `internal/agent/llm_caller_impl.go` + `internal/agent/llmcompat/` | 无 | 框架无独立 LLM 调用器抽象 |
| Session 标题 LLM 生成 | `internal/service/session_title_llm.go` | 无 | 业务功能，直接使用框架 Model |
| Token 用量统计 | `internal/data/ent/schema/model_token_usage_hourly.go` | 无 | 框架无用量持久化能力 |
| 模型定价规则 | `internal/data/ent/schema/model_pricing_rule.go` | 无 | 框架无定价管理能力 |
| Provider 类型映射 | `internal/provider/trpc_llm.go`（`MapProviderType`/`mapVariant`） | 无 | 项目 provider_type 与框架 provider 名称的映射逻辑 |

### 2.3 未使用的框架功能

| 框架功能 | 未使用原因 | 是否需要启用 |
|---------|-----------|-------------|
| `model.IterModel` 接口 | 项目不直接调用 `GenerateContent`，通过 Agent/Runner 间接调用；Runner 内部已使用 | 否（Runner 已自动使用） |
| `model.FileDownloader` 接口 | 项目无文件下载业务场景 | 否 |
| Provider 级回调（ChatRequest/ChatResponse/ChatChunk/ChatStreamComplete） | 项目使用 BeforeModel/AfterModel 回调已满足需求，Provider 级回调粒度过细 | 评估中（可用于调试/审计） |
| `WithStructuredOutputJSON` | 项目无结构化输出业务场景 | 否 |
| Batch API | 项目无批量处理业务场景 | 否 |
| `model.SimpleTokenCounter` | 项目未显式指定计数器，使用框架默认 | 否（默认即 SimpleTokenCounter） |
| `tiktoken` 精确计数器 | 项目未配置，使用 SimpleTokenCounter 启发式计数 | 评估中（精确计数可提升裁剪准确性） |
| `WithTailoringStrategy` | 项目使用默认 MiddleOutStrategy | 评估中（某些场景可能需要 HeadOut） |
| `WithTokenTailoringConfig` | 项目使用默认预算参数 | 评估中（可微调安全边际比例） |
| Anthropic Prompt Cache（3 断点） | 项目仅通过 `WithCacheSystemPrompt/Tools/Messages` 启用，未精细控制断点数量 | 评估中（可优化缓存命中率） |
| OpenAI `WithOptimizeForCache` | 项目已通过 config_json 配置启用 | 已启用 |
| OpenAI `WithReasoningContentBackfill` | 项目已通过 config_json 配置启用 | 已启用 |
| `hedge.WithDelays`（多级延迟） | 项目仅使用 `WithDelay` 单级延迟 | 评估中（多级延迟可更精细控制对冲） |

---

## 三、对比分析

### 3.1 框架优势（项目应采纳的）

| # | 框架优势 | 项目现状 | 对齐收益 |
|---|---------|---------|---------|
| 1 | **tiktoken 精确计数器** | 使用 SimpleTokenCounter 启发式计数（rune/4），裁剪精度较低 | 提升裁剪准确性，减少超出上下文窗口或浪费窗口空间的风险 |
| 2 | **Provider 级回调**（ChatRequest/ChatResponse/ChatChunk/ChatStreamComplete） | 未使用，缺少请求/响应级可观测性 | 可用于请求审计、调试、延迟分析，无需自建 |
| 3 | **`hedge.WithDelays` 多级延迟** | 仅使用 `WithDelay` 单级延迟 | 多级延迟可更精细控制对冲策略（如 50ms/150ms/300ms） |
| 4 | **`WithTailoringStrategy` 可选策略** | 使用默认 MiddleOut，未暴露给用户配置 | 可让用户按场景选择裁剪策略 |
| 5 | **`WithTokenTailoringConfig` 预算参数** | 使用默认参数（SafetyMarginRatio=10%、ReserveOutputTokens=2048） | 可按模型微调预算参数，提升窗口利用率 |

### 3.2 项目优势（框架缺失的）

| # | 项目优势 | 框架现状 | 建议处理 |
|---|---------|---------|---------|
| 1 | **Rate Limit Transport**（令牌桶限流） | 框架无 HTTP 级限流机制 | 贡献回框架（通用需求） |
| 2 | **Retry Transport**（跨 Provider 统一重试） | 框架仅 OpenAI SDK 内置重试，其他 Provider 无重试 | 贡献回框架（通用需求） |
| 3 | **Circuit Breaker Transport**（熔断器） | 框架无熔断器机制 | 贡献回框架（通用需求） |
| 4 | **Metrics Model**（Provider 级指标采集） | 框架无 Provider 级指标采集装饰器 | 贡献回框架 |
| 5 | **模型目录管理**（CRUD + 凭据加密 + 定价） | 框架无模型目录管理能力 | 保持自建（业务层功能） |
| 6 | **models.dev 目录同步** | 框架无远程目录同步能力 | 保持自建（业务层功能） |
| 7 | **Callback Chain 适配层**（优先级排序 + 分层 + 插件注册） | 框架 `model.Callbacks` 仅支持扁平列表 | 贡献回框架（优先级回调链） |
| 8 | **5 种 ModelSelector 策略** | 框架仅定义 `ModelSelector` 接口，无内置策略 | 贡献回框架（CostAware/QualityAware/LatencyAware） |
| 9 | **LLMCaller 抽象**（非 Agent 场景） | 框架无独立 LLM 调用器抽象 | 评估中（可作为框架工具类） |
| 10 | **Provider 类型映射**（provider_type → 框架 provider name + variant） | 框架无统一映射机制 | 保持自建（项目特有逻辑） |

### 3.3 差异根因分析

| 差异点 | 根因 | 影响范围 |
|--------|------|---------|
| 自建 Rate Limit/Retry/Circuit Breaker Transport | **功能缺失**：框架无 HTTP 级弹性机制，仅 OpenAI SDK 有内置重试 | `internal/provider/` 3 个 Transport 文件 |
| 自建 Metrics Model | **功能缺失**：框架无 Provider 级指标采集 | `internal/provider/metrics_model.go` |
| 自建 Callback Chain 适配层 | **功能不足**：框架 Callbacks 仅支持扁平列表，无优先级/分层/插件注册 | `internal/agent/callbacks/` + `callback_chain.go` |
| 自建 5 种 ModelSelector 策略 | **架构决策**：框架仅定义接口，策略实现属业务层（合理） | `internal/agent/model_selector.go` |
| 自建模型目录管理 | **架构决策**：框架定位为运行时，目录管理属业务层 | `internal/biz/llm_provider_model.go` + `internal/data/` |
| 未使用 tiktoken 精确计数器 | **认知缺失**：项目未评估 SimpleTokenCounter 的精度是否满足需求 | Token 裁剪精度 |
| 未使用 Provider 级回调 | **认知缺失**：项目未意识到此回调可用于请求级可观测性 | 可观测性 |
| 未使用 IterModel/FileDownloader | **无业务需求**：项目通过 Runner 间接调用，无文件下载场景 | 无影响 |

---

## 四、对齐方案

### 4.1 对齐项清单

| # | 对齐项 | 类型 | 优先级 | 影响范围 | 预期收益 |
|---|--------|------|--------|---------|---------|
| 1 | 启用 tiktoken 精确计数器 | 启用框架功能 | P2 | `internal/provider/trpc_llm.go` | 裁剪精度提升 |
| 2 | 启用 Provider 级回调 | 启用框架功能 | P3 | `internal/provider/trpc_llm.go` | 请求级可观测性 |
| 3 | 暴露 TailoringStrategy 配置 | 启用框架功能 | P3 | `internal/provider/catalog.go` + `trpc_llm.go` | 用户可按场景选择裁剪策略 |
| 4 | 暴露 TokenTailoringConfig 配置 | 启用框架功能 | P3 | `internal/provider/catalog.go` + `trpc_llm.go` | 可微调预算参数 |
| 5 | 贡献 Rate Limit Transport 回框架 | 贡献回框架 | P2 | `internal/provider/rate_limit_transport.go` | 减少自建代码维护 |
| 6 | 贡献 Retry Transport 回框架 | 贡献回框架 | P2 | `internal/provider/retry_transport.go` | 减少自建代码维护 |
| 7 | 贡献 Circuit Breaker Transport 回框架 | 贡献回框架 | P2 | `internal/provider/circuit_breaker_transport.go` + `internal/biz/tool/circuit_breaker.go` | 减少自建代码维护 |
| 8 | 贡献 Metrics Model 回框架 | 贡献回框架 | P3 | `internal/provider/metrics_model.go` | 减少自建代码维护 |
| 9 | 贡献 Callback Chain 回框架 | 贡献回框架 | P2 | `internal/agent/callbacks/` + `callback_chain.go` | 减少自建代码维护 |
| 10 | 贡献 ModelSelector 策略回框架 | 贡献回框架 | P3 | `internal/agent/model_selector.go` | 减少自建代码维护 |

### 4.2 对齐项详情

#### 对齐项 #1：启用 tiktoken 精确计数器

**类型**：启用框架功能

**现状**：
- 项目当前实现：使用 `SimpleTokenCounter`（启发式：UTF-8 rune / 4.0），精度约 ±25%
- 框架提供能力：`tiktoken.Counter`（基于 tiktoken-go，按模型选择编码器，精度 >95%）

**对齐方案**：
1. 在 `ProviderModelConfig` 中新增 `TokenCounterType` 字段（`simple`/`tiktoken`，默认 `simple` 保持兼容）
2. 在 `buildProviderOptions` 中根据配置选择计数器：
   ```go
   if cfg.TokenCounterType == "tiktoken" {
       counter, err := trpctiktoken.New(cfg.ModelAPI)
       if err != nil {
           lg.Warn("tiktoken init failed, fallback to simple", ...)
       } else {
           opts = append(opts, trpcprovider.WithTokenCounter(counter))
       }
   }
   ```
3. 在 `llm_provider_models` 的 `config_json` 中支持 `token_counter_type` 字段
4. 在 `llminspect` 探测时默认推荐 `tiktoken`

**代码变更范围**：
- 新增：无
- 修改：`internal/provider/catalog.go`（新增字段）、`internal/provider/trpc_llm.go`（构建逻辑）、`internal/llminspect/inspect.go`（默认推荐）
- 删除：无

**兼容性风险**：
- 低。tiktoken 初始化失败时自动 fallback 到 SimpleTokenCounter

**回退方案**：
- 将 `token_counter_type` 设为 `simple` 或删除该字段

**验证方法**：
- 单元测试：验证 tiktoken 计数器创建和 fallback 逻辑
- 集成测试：对比 SimpleTokenCounter 和 tiktoken 的裁剪结果差异

**预期收益**：
- 代码减少：约 0 行（新增配置逻辑）
- 性能影响：tiktoken 计数略慢（需查表），但裁剪更精准，减少超出窗口或浪费窗口
- 维护成本：无变化
- 功能增强：裁剪精度从 ±25% 提升到 >95%

---

#### 对齐项 #2：启用 Provider 级回调

**类型**：启用框架功能

**现状**：
- 项目当前实现：未使用 Provider 级回调（ChatRequest/ChatResponse/ChatChunk/ChatStreamComplete）
- 框架提供能力：4 个 Provider 级回调点，可在请求/响应/流式 chunk/流式完成时触发

**对齐方案**：
1. 在 `buildProviderOptions` 中构建 `provider.Callbacks`：
   ```go
   callbacks := provider.Callbacks{
       OpenAIChatRequest: func(ctx context.Context, req *openaisdk.ChatCompletionNewParams) {
           // 记录请求元数据（model、messages 数、tools 数）
       },
       OpenAIChatChunk: func(ctx context.Context, chunk *openaisdk.ChatCompletionChunk) {
           // 记录 chunk 级延迟指标
       },
       // Anthropic/Gemini/Ollama 类似
   }
   opts = append(opts, provider.WithCallbacks(callbacks))
   ```
2. 回调中注入 traceID、stepID 等上下文信息，用于请求追踪

**代码变更范围**：
- 新增：无
- 修改：`internal/provider/trpc_llm.go`（构建 Callbacks）
- 删除：无

**兼容性风险**：
- 低。回调为观察者模式，不影响主流程

**回退方案**：
- 移除 Callbacks 构建代码

**验证方法**：
- 单元测试：验证回调触发和上下文传递
- 集成测试：验证请求追踪链路完整性

**预期收益**：
- 代码减少：约 0 行
- 性能影响：微乎其微（回调开销极低）
- 维护成本：无变化
- 功能增强：获得请求级可观测性（请求参数审计、chunk 级延迟分析）

---

#### 对齐项 #3：暴露 TailoringStrategy 配置

**类型**：启用框架功能

**现状**：
- 项目当前实现：使用默认 `MiddleOutStrategy`，用户无法选择裁剪策略
- 框架提供能力：3 种策略（MiddleOut/HeadOut/TailOut），通过 `WithTailoringStrategy` 配置

**对齐方案**：
1. 在 `ProviderModelConfig` 中新增 `TailoringStrategy` 字段（`middle_out`/`head_out`/`tail_out`，默认 `middle_out`）
2. 在 `buildProviderOptions` 中根据配置构建策略：
   ```go
   if cfg.TailoringStrategy != "" && cfg.EnableTokenTailoring {
       var counter model.TokenCounter = model.NewSimpleTokenCounter()
       // 如果启用了 tiktoken，使用 tiktoken counter
       strategy := buildTailoringStrategy(cfg.TailoringStrategy, counter)
       opts = append(opts, trpcprovider.WithTailoringStrategy(strategy))
   }
   ```
3. 在 `config_json` 中支持 `tailoring_strategy` 字段

**代码变更范围**：
- 新增：无
- 修改：`internal/provider/catalog.go`、`internal/provider/trpc_llm.go`
- 删除：无

**兼容性风险**：
- 低。默认值保持 `middle_out`，与当前行为一致

**回退方案**：
- 删除 `tailoring_strategy` 字段或设为 `middle_out`

**验证方法**：
- 单元测试：验证 3 种策略构建逻辑
- 集成测试：验证不同策略的裁剪结果

**预期收益**：
- 代码减少：约 0 行
- 性能影响：无
- 维护成本：无变化
- 功能增强：用户可按场景选择裁剪策略（如知识库场景用 HeadOut 保留最新对话）

---

#### 对齐项 #4：暴露 TokenTailoringConfig 配置

**类型**：启用框架功能

**现状**：
- 项目当前实现：使用框架默认预算参数（SafetyMarginRatio=10%、ReserveOutputTokens=2048、ProtocolOverheadTokens=512）
- 框架提供能力：`WithTokenTailoringConfig` 可自定义所有预算参数

**对齐方案**：
1. 在 `ProviderModelConfig` 中新增 `TokenTailoring` 嵌套配置：
   ```go
   type TokenTailoringConfig struct {
       SafetyMarginRatio    float64 `json:"safety_margin_ratio"`     // 默认 0.10
       ReserveOutputTokens  int     `json:"reserve_output_tokens"`   // 默认 2048
       ProtocolOverheadTokens int   `json:"protocol_overhead_tokens"` // 默认 512
       InputTokensFloor     int     `json:"input_tokens_floor"`      // 默认 1024
       MaxInputTokensRatio  float64 `json:"max_input_tokens_ratio"`  // 默认 1.0
   }
   ```
2. 在 `buildProviderOptions` 中构建并注入配置

**代码变更范围**：
- 新增：无
- 修改：`internal/provider/catalog.go`、`internal/provider/trpc_llm.go`
- 删除：无

**兼容性风险**：
- 低。所有字段有默认值，与当前行为一致

**回退方案**：
- 删除配置字段，回退到默认参数

**验证方法**：
- 单元测试：验证预算计算公式
- 集成测试：验证不同参数下的裁剪行为

**预期收益**：
- 代码减少：约 0 行
- 性能影响：无
- 维护成本：无变化
- 功能增强：可按模型微调预算参数，提升窗口利用率（如长输出模型可增大 ReserveOutputTokens）

---

#### 对齐项 #5：贡献 Rate Limit Transport 回框架

**类型**：贡献回框架

**现状**：
- 项目当前实现：`rate_limit_transport.go`（令牌桶算法，RPM 限流，BUG-8 修复）
- 框架提供能力：无 HTTP 级限流机制

**对齐方案**：
1. 将 `rate_limit_transport.go` 适配为框架 `model/` 下的通用 Transport Option
2. 框架侧新增 `provider.WithRateLimit(rpm)` Option
3. 项目侧移除自建实现，改用框架 Option
4. 保留 BUG-8 修复（负 elapsed 保护）

**代码变更范围**：
- 新增：`pkg/trpc-agent-go/model/ratelimit/`（框架侧）
- 修改：`internal/provider/trpc_llm.go`（改用框架 Option）
- 删除：`internal/provider/rate_limit_transport.go`

**兼容性风险**：
- 中。需确保框架实现与项目行为一致（令牌桶算法、负 elapsed 保护）

**回退方案**：
- 恢复自建 `rate_limit_transport.go`

**验证方法**：
- 单元测试：验证令牌桶算法正确性
- 压测：验证 RPM 限流效果

**预期收益**：
- 代码减少：约 80 行（1 个文件）
- 性能影响：无
- 维护成本：减少框架升级时的适配工作
- 功能增强：其他框架用户也可使用限流能力

---

#### 对齐项 #6：贡献 Retry Transport 回框架

**类型**：贡献回框架

**现状**：
- 项目当前实现：`retry_transport.go`（指数退避，5xx/429 重试，请求体重置）
- 框架提供能力：仅 OpenAI SDK 内置重试（`WithMaxRetries`），其他 Provider 无重试

**对齐方案**：
1. 将 `retry_transport.go` 适配为框架 `model/` 下的通用 Transport Option
2. 框架侧新增 `provider.WithRetry(maxRetries, baseDelay, maxDelay)` Option
3. 项目侧移除自建实现，改用框架 Option
4. 注意：OpenAI Provider 已有 SDK 级重试，需避免双重重试

**代码变更范围**：
- 新增：`pkg/trpc-agent-go/model/retry/`（框架侧）
- 修改：`internal/provider/trpc_llm.go`（改用框架 Option）
- 删除：`internal/provider/retry_transport.go`

**兼容性风险**：
- 中。需处理与 OpenAI SDK 内置重试的冲突

**回退方案**：
- 恢复自建 `retry_transport.go`

**验证方法**：
- 单元测试：验证重试逻辑（5xx/429/网络错误）
- 集成测试：验证与 OpenAI SDK 重试的兼容性

**预期收益**：
- 代码减少：约 100 行（1 个文件）
- 性能影响：无
- 维护成本：减少框架升级时的适配工作
- 功能增强：所有 Provider 统一重试能力

---

#### 对齐项 #7：贡献 Circuit Breaker Transport 回框架

**类型**：贡献回框架

**现状**：
- 项目当前实现：`circuit_breaker_transport.go` + `biz/tool/circuit_breaker.go`（三态状态机，预设配置，持久化）
- 框架提供能力：无熔断器机制

**对齐方案**：
1. 将熔断器核心逻辑（三态状态机）贡献到框架 `model/circuitbreaker/`
2. 框架侧新增 `provider.WithCircuitBreaker(config)` Option
3. 项目侧移除 Transport 包装，改用框架 Option
4. 预设配置和 Registry 保留在项目业务层（框架只提供核心机制）

**代码变更范围**：
- 新增：`pkg/trpc-agent-go/model/circuitbreaker/`（框架侧）
- 修改：`internal/provider/trpc_llm.go`（改用框架 Option）
- 删除：`internal/provider/circuit_breaker_transport.go`（Transport 包装层）
- 保留：`internal/biz/tool/circuit_breaker.go`（预设配置和 Registry 属业务层）

**兼容性风险**：
- 中。需确保框架实现与项目行为一致（状态转换、半开探测）

**回退方案**：
- 恢复自建 `circuit_breaker_transport.go`

**验证方法**：
- 单元测试：验证三态状态机转换
- 集成测试：验证熔断和恢复行为

**预期收益**：
- 代码减少：约 60 行（Transport 包装层）
- 性能影响：无
- 维护成本：减少框架升级时的适配工作
- 功能增强：其他框架用户也可使用熔断器能力

---

#### 对齐项 #8：贡献 Metrics Model 回框架

**类型**：贡献回框架

**现状**：
- 项目当前实现：`metrics_model.go`（装饰器模式，记录 ProviderRequestTotal/Duration）
- 框架提供能力：无 Provider 级指标采集

**对齐方案**：
1. 将 `metricsModel` 贡献到框架 `model/metrics/`
2. 框架侧新增 `provider.WithMetrics(prometheusRegistry)` 或回调式 Option
3. 项目侧移除自建实现，改用框架 Option

**代码变更范围**：
- 新增：`pkg/trpc-agent-go/model/metrics/`（框架侧）
- 修改：`internal/provider/trpc_llm.go`（改用框架 Option）
- 删除：`internal/provider/metrics_model.go`

**兼容性风险**：
- 低。装饰器模式，不影响核心流程

**回退方案**：
- 恢复自建 `metrics_model.go`

**验证方法**：
- 单元测试：验证指标采集正确性
- 集成测试：验证 Prometheus 指标暴露

**预期收益**：
- 代码减少：约 70 行（1 个文件）
- 性能影响：无
- 维护成本：减少框架升级时的适配工作
- 功能增强：其他框架用户也可使用指标采集

---

#### 对齐项 #9：贡献 Callback Chain 回框架

**类型**：贡献回框架

**现状**：
- 项目当前实现：`internal/agent/callbacks/` + `callback_chain.go`（优先级排序、分层 Static/SemiStatic/Dynamic、插件注册、panic 恢复）
- 框架提供能力：`model.Callbacks` 仅支持扁平列表，无优先级/分层/插件注册

**对齐方案**：
1. 将优先级排序和分层机制贡献到框架 `model/callbacks/`
2. 框架侧增强 `Callbacks` 支持：
   - 优先级排序（数值越小越先执行）
   - 分层（Static/SemiStatic/Dynamic）
   - `continueOnError` / `continueOnResponse` 控制
3. 项目侧简化适配层，改用框架增强版

**代码变更范围**：
- 新增：框架侧增强
- 修改：`internal/agent/callbacks/`（简化适配层）
- 删除：部分适配代码

**兼容性风险**：
- 中。回调执行顺序可能因框架实现差异而变化

**回退方案**：
- 保留项目自建 Callback Chain

**验证方法**：
- 单元测试：验证优先级排序和分层行为
- 集成测试：验证回调链执行顺序与项目一致

**预期收益**：
- 代码减少：约 150 行（适配层简化）
- 性能影响：无
- 维护成本：减少框架升级时的适配工作
- 功能增强：其他框架用户也可使用优先级回调链

---

#### 对齐项 #10：贡献 ModelSelector 策略回框架

**类型**：贡献回框架

**现状**：
- 项目当前实现：5 种策略（PluginCostGuard/CostAware/QualityAware/LatencyAware/PluginModelSelector）+ ChainedModelSelector
- 框架提供能力：仅定义 `ModelSelector` 接口，无内置策略

**对齐方案**：
1. 将通用策略（CostAware/QualityAware/LatencyAware）贡献到框架 `model/selector/`
2. 框架侧提供内置策略工厂：
   - `selector.NewCostAware(catalog)` — 成本优先
   - `selector.NewQualityAware(catalog)` — 质量优先
   - `selector.NewLatencyAware(catalog)` — 延迟优先
   - `selector.NewChained(selectors...)` — 链式组合
3. 项目侧保留业务特定策略（PluginCostGuard/PluginModelSelector），通用策略改用框架实现

**代码变更范围**：
- 新增：`pkg/trpc-agent-go/model/selector/`（框架侧）
- 修改：`internal/agent/model_selector.go`（改用框架策略）
- 删除：通用策略实现代码（约 200 行）

**兼容性风险**：
- 中。需确保框架策略与项目行为一致

**回退方案**：
- 保留项目自建策略

**验证方法**：
- 单元测试：验证各策略选择逻辑
- 集成测试：验证链式组合行为

**预期收益**：
- 代码减少：约 200 行（通用策略实现）
- 性能影响：无
- 维护成本：减少框架升级时的适配工作
- 功能增强：其他框架用户也可使用内置策略

---

## 五、实施路线

### 5.1 阶段规划

| 阶段 | 对齐项 | 前置依赖 | 预计工作量 |
|------|--------|---------|-----------|
| Phase 1 | #1（tiktoken）、#3（TailoringStrategy）、#4（TokenTailoringConfig） | 无 | 小 |
| Phase 2 | #5（Rate Limit）、#6（Retry）、#7（Circuit Breaker） | 框架接受贡献 | 大 |
| Phase 3 | #8（Metrics Model）、#9（Callback Chain）、#10（ModelSelector 策略） | Phase 2 | 大 |
| Phase 4 | #2（Provider 级回调） | 无 | 小 |

### 5.2 风险与缓解

| 风险 | 概率 | 影响 | 缓解措施 |
|------|------|------|---------|
| 框架不接受弹性组件贡献 | 中 | 中 | 保持自建，但确保自建实现与框架 Option 机制兼容 |
| tiktoken 初始化失败（模型名不在映射表中） | 低 | 低 | 自动 fallback 到 SimpleTokenCounter，记录 Warn 日志 |
| 贡献后框架 API 变更导致项目适配 | 低 | 中 | 贡献时确保 API 稳定，项目侧保留适配层 |
| Callback Chain 贡献后执行顺序变化 | 中 | 高 | 贡献前对比测试所有回调执行顺序 |
| ModelSelector 策略贡献后选择结果不一致 | 中 | 中 | 贡献前对比测试所有策略选择结果 |

---

## 六、附录

### A. 框架示例代码参考（必填）

| 示例 | 路径 | 关键 API | 初始化模式 | 与项目实现差异 |
|------|------|---------|-----------|--------------|
| Model 基础 | `examples/model/main.go` | `openai.New(modelName)` + `GenerateContent` | 直接创建，环境变量自动读取 | 项目通过 `provider.Model()` 工厂 + config_json 配置 |
| Model Batch | `examples/model/batch/main.go` | `CreateBatch`/`RetrieveBatch`/`CancelBatch` | 直接创建 | 项目未使用 Batch API |
| Model Failover | `examples/model/failover/` | `failover.New(WithCandidates(...))` | 装饰器组合 | 项目使用方式一致（`wrapFailover`） |
| Model Hedge | `examples/model/hedge/` | `hedge.New(WithCandidates/WithDelay)` | 装饰器组合 | 项目使用方式一致（`wrapHedge`），但未使用 `WithDelays` |
| Model PromptMap | `examples/model/promptmap/main.go` | `WithModels`/`WithModelInstructions`/`WithModelGlobalInstructions` | Agent 构造时注册模型映射 | 项目未使用按模型映射指令，使用 ModelSelector 替代 |
| Model Retry | `examples/model/retry/main.go` | `openai.WithOpenAIOptions(WithMaxRetries)` | SDK 级重试 | 项目使用自建 Retry Transport（跨 Provider 统一重试） |
| Model Selector | `examples/model/selector/` | `agent.WithModelSelector(fn)` + `Invocation.SetState` | RunOption 传入选择函数 | 项目使用方式一致，但实现了 5 种业务策略 |
| Model Switch | `examples/model/switch/main.go` | `SetModel`/`SetModelByName`/`WithModel`/`WithModelName` | Agent 级 + 请求级切换 | 项目使用方式一致 |
| Provider | `examples/provider/main.go` | `provider.Model(providerName, modelName, opts...)` | 工厂函数 | 项目使用方式一致（`TRPCModelForProviderModel` 封装） |
| Tailor | `examples/tailor/main.go` + `helper.go` | `WithEnableTokenTailoring`/`WithMaxInputTokens`/`WithTokenCounter`/`WithTailoringStrategy` | Provider Option 注入 | 项目使用方式一致，但未暴露 TailoringStrategy 和 TokenCounter 配置 |

### B. 框架文档参考

| 文档 | 路径 |
|------|------|
| Model 模块完整文档 | `docs/mkdocs/zh/model.md` |
