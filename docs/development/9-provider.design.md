# Provider 模型层 — 实现设计文档

> 对应需求：[9 provider.md](./9%20provider.md)
> 开发进度与任务清单：[9 provider.development.md](./9%20provider.development.md)

---

## 一、模块概述

LLM Provider/Model 管理：多厂商注册、模型目录、Failover/Hedge 高可用、TokenTailor 自动裁剪。核心包 `internal/provider` 桥接 trpc-agent-go `model` 包。

**分层职责**：

| 层 | 包路径 | 职责 |
|----|--------|------|
| Proto | `api/kratos/llm_provider_model/v1/` | HTTP/gRPC 契约：CRUD + Inspect + ValidatePair + RevealCredentials |
| Service | `internal/service/llm_provider_model.go` | Proto ↔ Biz 转换、HTTP 入口 |
| Biz | `internal/biz/llm_provider_model.go` | 领域模型、Usecase、Repo 子接口、InspectMerge、ModelPricingRule、ModelCapabilities |
| Biz（凭据） | `internal/biz/credential_crypto.go`、`credential_key.go`、`channel_credential_crypto.go` | AES-256-GCM 加解密 |
| Data | `internal/data/llm_provider_model.go` | Ent ORM Repo 实现 |
| Schema | `internal/data/ent/schema/llm_provider_model.go`、`model_pricing_rule.go` | Ent Schema（唯一真相源） |
| Provider 桥接 | `internal/provider/trpc_llm.go`、`catalog.go`、`roundtrip.go`、`stream_delta.go` | config_json → trpc model.Model 装配 |
| Inspect | `internal/llminspect/inspect.go` | 远程模型元数据探测 |
| 前端 | `web/src/config/providerPresets.ts`、`web/src/features/platform/`、`web/src/components/platform/` | 预设、composable、组件 |

---

## 二、trpc-agent-go Model 体系

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

| Provider 类型 | trpc 包 | 认证方式 | 默认 Base URL | 协议 | `provider.Model()` 第一个参数 |
|--------------|---------|---------|--------------|------|---------------------------|
| **OpenAI Compatible** | `model/openai` | API Key | `https://api.openai.com/v1` | OpenAI Chat Completions | `"openai"` |
| **Anthropic** | `model/anthropic` | API Key | `https://api.anthropic.com` | Anthropic Messages | `"anthropic"` |
| **Gemini** | `model/gemini` | API Key | `https://generativelanguage.googleapis.com` | Google GenAI | `"gemini"` |
| **Ollama** | `model/ollama` | 无（本地） | `http://localhost:11434` | Ollama Chat | `"ollama"` |
| **Hunyuan** | `model/hunyuan` | SecretId + SecretKey | `https://hunyuan.tencentcloudapi.com` | 混元私有协议 | `"hunyuan"` |
| **HuggingFace** | `model/huggingface` | API Key | `https://router.huggingface.co` | HF Inference | `"huggingface"` |
| **Bedrock** | `model/bedrock` | AWS Config | — | AWS Bedrock Runtime | `"bedrock"` |

> HuggingFace 和 Bedrock 的 Provider 类型枚举值已预留，待 trpc 上游注册到 `provider` 工厂后即可启用。

### 2.2 OpenAI Compatible 子类型（Variant）

OpenAI Provider 通过 `Variant` 区分子类型行为差异，由 `openai.WithVariant()` 或 `provider.WithVariant()` 设置：

| Variant | 枚举值 | 行为差异 | 默认 Base URL |
|---------|--------|---------|--------------|
| **OpenAI** | `openai` | 默认；自动 prompt cache 优化 | `https://api.openai.com/v1` |
| **DeepSeek** | `deepseek` | 自动回填 `reasoning_content`；思考模式使用 `{"type":"enabled"/"disabled"}` 格式；`textOnlyMessageContent=true` | `https://api.deepseek.com` |
| **Qwen** | `qwen` | 默认 Base URL 指向阿里云；思考模式使用 `enabled_thinking` key | `https://dashscope.aliyuncs.com/compatible-mode/v1` |
| **Hunyuan** | `hunyuan` | 特定文件处理逻辑；文件上传使用 multipart form | — |

**Variant 自动推断**：若未显式设置 Variant，OpenAI Model 会根据 `BaseURL` 自动推断（如 BaseURL 包含 `api.deepseek.com` 则推断为 `deepseek`）。

### 2.3 高可用模式

| 模式 | trpc 包 | 说明 | 关键配置 |
|------|---------|------|---------|
| **Failover** | `model/failover` | 按顺序尝试候选模型，首个成功响应即返回；适用于主备切换 | `WithCandidates(m1, m2, ...)` |
| **Hedge** | `model/hedge` | 并发发起多个候选请求，首个有效响应即返回；可配置延迟启动偏移；适用于降低尾部延迟 | `WithCandidates(...)`、`WithDelay(100ms)`、`WithDelays(...)`、`WithName(...)`、`WithContextWindow(...)` |

> **注**：trpc-agent-go 的 `model/failover` 和 `model/hedge` 包提供 `WithSwitchCallback`。
> HA 切换可观测性：运行时切换经回调写入 `loggateway`（step ID `system.provider.ha_failover` / `system.provider.ha_hedge`，Monitor FlowLog 已注册标题）+ `WrapModelWithMetrics` Prometheus 指标；候选构建失败时 `lg.Warn`。

### 2.4 通用能力（provider.Option）

| 能力 | provider.Option | 说明 |
|------|----------------|------|
| **Token Tailoring** | `WithEnableTokenTailoring(bool)` | 自动裁剪输入 Token 以适应上下文窗口 |
| **Tailoring Strategy** | `WithTailoringStrategy(model.TailoringStrategy)` | 自定义裁剪策略（默认 `MiddleOutStrategy`） |
| **Token Tailoring Config** | `WithTokenTailoringConfig(*model.TokenTailoringConfig)` | 裁剪预算参数（SafetyMarginRatio 等） |
| **Context Window** | `WithContextWindow(int)` | 实例级覆盖上下文窗口大小 |
| **Max Input Tokens** | `WithMaxInputTokens(int)` | 最大输入 Token 限制 |
| **Channel Buffer Size** | `WithChannelBufferSize(int)` | 响应通道缓冲区大小（默认 256） |
| **Custom HTTP Transport** | `WithHTTPClientTransport(http.RoundTripper)` | 注入自定义 Transport，用于代理、超时、链路追踪 |
| **Callbacks** | `WithCallbacks(provider.Callbacks)` | 四阶段回调（Request/Response/Chunk/StreamComplete） |

### 2.5 Provider 专属能力

#### OpenAI 专属

| 能力 | Option | 说明 |
|------|--------|------|
| **Variant 选择** | `WithVariant(openai.Variant)` | 选择 OpenAI/DeepSeek/Qwen/Hunyuan 行为模式 |
| **Prompt Cache 优化** | `WithOptimizeForCache(bool)` | 将 system 消息前置以提高缓存命中率 |
| **Reasoning 回填** | `WithReasoningContentBackfill(bool)` | DeepSeek Variant 默认启用 |
| **Tool Call Delta** | `WithShowToolCallDelta(bool)` | 流式响应中暴露 tool_call 增量 |

#### Anthropic 专属

| 能力 | Option | 说明 |
|------|--------|------|
| **System Prompt Cache** | `WithCacheSystemPrompt(bool)` | 缓存系统提示（90% 输入折扣） |
| **Tools Cache** | `WithCacheTools(bool)` | 缓存工具定义（90% 输入折扣） |
| **Messages Cache** | `WithCacheMessages(bool)` | 多轮对话缓存 |
| **Tool Call Delta** | `WithShowToolCallDelta(bool)` | 流式响应中暴露 tool_call 增量 |

#### Gemini 专属

| 能力 | Option | 说明 |
|------|--------|------|
| **Client Config** | `WithGeminiClientConfig(*genai.ClientConfig)` | 自定义 Gemini 客户端配置（APIKey / Backend / HTTPClient） |

#### Ollama 专属

| 能力 | Option | 说明 |
|------|--------|------|
| **Host** | `WithHost(string)` | Ollama 服务地址（默认 `http://localhost:11434`） |
| **Keep Alive** | `WithKeepAlive(time.Duration)` | 模型保持加载的时长 |

#### Hunyuan 专属

| 能力 | Option | 说明 |
|------|--------|------|
| **SecretId** | `WithSecretId(string)` | 腾讯云 SecretId |
| **SecretKey** | `WithSecretKey(string)` | 腾讯云 SecretKey |
| **Host** | `WithHost(string)` | 混元服务地址 |

---

## 三、Provider 类型与前端预设映射

### 3.1 前端类型定义

文件：`web/src/config/providerRuntimeOverlay.types.ts`、`web/src/config/providerPresets.ts`

```typescript
export type AuthType = "api_key" | "secret_id_key" | "aws_config" | "none";
export type ProviderType = "openai" | "anthropic" | "gemini" | "ollama" | "hunyuan" | "huggingface" | "bedrock";
export type OpenAIVariant = "openai" | "deepseek" | "qwen" | "hunyuan";

export type ProviderPreset = {
  key: string;
  label: string;
  providerCode: string;
  providerType: ProviderType;
  variant?: OpenAIVariant;
  authType: AuthType;
  apiBaseUrl: string;
  metadataApi: "full" | "partial" | "limited" | "none";
  metadataNote: string;
  models: ProviderModelPreset[];
};
```

### 3.2 预设架构

前端预设采用 **shell + overlay** 架构：

- `providerPresets.ts` 的 `PROVIDER_PRESETS`：13 个预设 shell（OpenAI / Anthropic / Google / DeepSeek / 阿里云百炼 / Moonshot CN / 智谱 AI / OpenRouter / Ollama / 腾讯混元 / HuggingFace / AWS Bedrock / 完全自定义）
- `providerRuntimeOverlay.ts` + `provider_runtime_overlay.json`：models.dev provider id → trpc 运行时（providerType / variant / authType / apiBaseUrl）映射，与后端 `internal/modelcatalog/runtime_overlay.json` 同步
- `presetShell(key, label, providerCode, authOverride?)` 工厂函数：从 overlay 加载 runtime profile，构造预设 shell

### 3.3 选项常量

```typescript
export const PROVIDER_TYPE_OPTIONS: { label: string; value: ProviderType }[] = [
  { label: "OpenAI Compatible", value: "openai" },
  { label: "Anthropic", value: "anthropic" },
  { label: "Gemini", value: "gemini" },
  { label: "Ollama", value: "ollama" },
  { label: "Hunyuan", value: "hunyuan" },
  { label: "HuggingFace", value: "huggingface" },
  { label: "Bedrock", value: "bedrock" },
];

export const VARIANT_OPTIONS: { label: string; value: OpenAIVariant }[] = [
  { label: "OpenAI", value: "openai" },
  { label: "DeepSeek", value: "deepseek" },
  { label: "Qwen", value: "qwen" },
  { label: "Hunyuan", value: "hunyuan" },
];
```

### 3.4 authType 与 UI 表单联动

| authType | 显示的认证字段 | 隐藏的认证字段 |
|----------|--------------|--------------|
| `api_key` | API 基础 URL、API 密钥 | Secret ID/Key、AWS Region |
| `secret_id_key` | API 基础 URL、Secret ID、Secret Key | API 密钥、AWS Region |
| `aws_config` | AWS Region | API 基础 URL、API 密钥、Secret ID/Key |
| `none` | API 基础 URL（Host） | API 密钥、Secret ID/Key、AWS Region |

---

## 四、Proto 层

文件：`api/kratos/llm_provider_model/v1/llm_provider_model.proto`

### 4.1 完整 Proto 定义

```protobuf
syntax = "proto3";

package kratos.llm_provider_model.v1;

import "google/api/annotations.proto";
import "google/api/field_behavior.proto";
import "google/protobuf/empty.proto";

option go_package = "aranea-agents/api/kratos/llm_provider_model/v1;v1";

message ProviderModel {
  string id = 1;
  string key = 2;
  string name = 3;
  string description = 4;
  string status = 5;
  bool enabled = 6;
  int32 sort_order = 7;
  string provider = 8;
  string model = 9;
  string config_json = 10;
  string metadata_json = 11;
  string created_at = 12;
  string updated_at = 13;
  string deleted_at = 14;
  ModelCapabilities capabilities = 15;
  bool pricing_configured = 16;
}

message ModelCapabilities {
  bool text = 1;
  bool vision = 2;
  bool audio = 3;
  bool file = 4;
  bool tool_call = 5;
  bool cache = 6;
  bool thinking = 7;
  bool text_only = 8;
}

// ListProviderModelsRequest 携带可选的分页 + 搜索参数（管理端注册表 UI 使用）。
// page 与 page_size 均为 0 时回退为 legacy 全量目录列表（选择器/健康检查/运行时消费方）。
message ListProviderModelsRequest {
  int32 page = 1;      // 1-based
  int32 page_size = 2; // default: 20, max: 100
  string search = 3;
}

message ListProviderModelsResponse {
  repeated ProviderModel items = 1;
  int32 total = 2;
  int32 page = 3;
  int32 page_size = 4;
}

message CreateProviderModelRequest {
  string key = 1 [(google.api.field_behavior) = REQUIRED];
  string name = 2 [(google.api.field_behavior) = REQUIRED];
  string description = 3;
  string status = 4;
  bool enabled = 5;
  int32 sort_order = 6;
  string provider = 7;
  string model = 8;
  string config_json = 9;
  string metadata_json = 10;
  ModelCapabilities capabilities = 11;
}

message GetProviderModelRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
}

message UpdateProviderModelRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
  ProviderModel provider_model = 2;
}

message DeleteProviderModelRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
}

message ValidateProviderPairRequest {
  string provider = 1;
  string model = 2;
}

message ValidateProviderPairResponse {
  bool ok = 1;
  string message = 2;
}

message InspectProviderModelRequest {
  string resource_id = 1;
  string provider_code = 2;
  string provider_type = 3;
  string model_api_id = 4;
  string api_base_url = 5;
  string api_key = 6;
  string variant = 7;
  string secret_id = 8;
  string secret_key = 9;
  string aws_region = 10;
}

message RevealProviderModelCredentialsRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
}

message HACandidateCredential {
  string name = 1;
  string api_key = 2;
}

message RevealProviderModelCredentialsResponse {
  string api_key = 1;
  string secret_key = 2;
  bool has_api_key = 3;
  bool has_secret_key = 4;
  repeated HACandidateCredential ha_candidates = 5;
}

message InspectProviderModelResponse {
  bool ok = 1;
  string message = 2;
  string provider_code = 3;
  string provider_type = 4;
  string model_api_id = 5;
  string model_display_name = 6;
  string model_size_label = 7;
  int32 context_window_k = 8;
  int32 max_output_tokens = 9;
  int64 input_price_micro_usd_per_1k = 10;
  int64 output_price_micro_usd_per_1k = 11;
  int64 cached_input_price_micro_usd_per_1k = 12;
  int64 reasoning_price_micro_usd_per_1k = 13;
  int64 embedding_price_micro_usd_per_1k = 14;
  string source = 15;
  string raw_metadata_json = 16;
  string variant = 17;
  bool enable_token_tailoring = 18;
  bool supports_cache = 19;
  bool supports_thinking = 20;
}

service LlmProviderModelService {
  rpc ListProviderModels(ListProviderModelsRequest) returns (ListProviderModelsResponse) {
    option (google.api.http) = {get: "/v1/llm-provider-models"};
  }
  rpc CreateProviderModel(CreateProviderModelRequest) returns (ProviderModel) {
    option (google.api.http) = {post: "/v1/llm-provider-models" body: "*"};
  }
  rpc GetProviderModel(GetProviderModelRequest) returns (ProviderModel) {
    option (google.api.http) = {get: "/v1/llm-provider-models/{id}"};
  }
  rpc RevealProviderModelCredentials(RevealProviderModelCredentialsRequest) returns (RevealProviderModelCredentialsResponse) {
    option (google.api.http) = {get: "/v1/llm-provider-models/{id}/credentials"};
  }
  rpc UpdateProviderModel(UpdateProviderModelRequest) returns (ProviderModel) {
    option (google.api.http) = {patch: "/v1/llm-provider-models/{id}" body: "provider_model"};
  }
  rpc DeleteProviderModel(DeleteProviderModelRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = {delete: "/v1/llm-provider-models/{id}"};
  }
  rpc InspectProviderModel(InspectProviderModelRequest) returns (InspectProviderModelResponse) {
    option (google.api.http) = {post: "/v1/llm-provider-models/inspect" body: "*"};
  }
  rpc ValidateProviderPair(ValidateProviderPairRequest) returns (ValidateProviderPairResponse) {
    option (google.api.http) = {post: "/v1/agents/validate-model" body: "*"};
  }
}
```

### 4.2 API 端点表

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/v1/llm-provider-models` | 列表（支持 `page`/`page_size`/`search` 查询参数；不带分页参数时返回全量目录，供选择器/健康检查使用） |
| POST | `/v1/llm-provider-models` | 创建 |
| GET | `/v1/llm-provider-models/{id}` | 详情 |
| GET | `/v1/llm-provider-models/{id}/credentials` | 揭示解密后的凭据（管理后台编辑用） |
| PATCH | `/v1/llm-provider-models/{id}` | 更新 |
| DELETE | `/v1/llm-provider-models/{id}` | 删除（软删） |
| POST | `/v1/llm-provider-models/inspect` | 检查模型连通性并回填元数据 |
| POST | `/v1/agents/validate-model` | 验证 Provider+Model 对是否可用 |

### 4.3 关键字段说明

- `ProviderModel.capabilities`（field 15）：模型能力位（text/vision/audio/file/tool_call/cache/thinking/text_only），由后端 `CapabilitiesForProviderModel` 推导
- `ProviderModel.pricing_configured`（field 16）：是否已配置定价（由 `configJSONHasPricing` 检测）
- `InspectProviderModelRequest`：含 `variant` / `secret_id` / `secret_key` / `aws_region` 字段，支持 Hunyuan/Bedrock Inspect
- `InspectProviderModelResponse`：含 `variant` / `enable_token_tailoring` / `supports_cache` / `supports_thinking` 字段
- `RevealProviderModelCredentials`：管理后台编辑时揭示解密后的 api_key / secret_key / ha_candidates 凭据

---

## 五、Biz 层

文件：`internal/biz/llm_provider_model.go`

### 5.1 领域模型

```go
type ProviderModel struct {
    ID                   string
    Key                  string // model_key
    Name                 string
    Description          string
    Status               string
    Enabled              bool
    SortOrder            int
    Provider             string
    Model                string
    ConfigJSON           string
    MetadataJSON         string
    Capabilities         ModelCapabilities
    CapabilitiesExplicit bool
    PricingConfigured    bool
    CreatedAt            string
    UpdatedAt            string
    DeletedAt            string
}

type ModelCapabilities struct {
    Text     bool `json:"text"`
    Vision   bool `json:"vision"`
    Audio    bool `json:"audio"`
    File     bool `json:"file"`
    ToolCall bool `json:"tool_call"`
    Cache    bool `json:"cache"`
    Thinking bool `json:"thinking"`
    TextOnly bool `json:"text_only"`
}

type ModelPricingRule struct {
    ID                            string
    ProviderCode                  string
    ModelAPIID                    string
    Currency                      string
    InputPriceMicroUSDPer1K       int64
    OutputPriceMicroUSDPer1K      int64
    CachedInputPriceMicroUSDPer1K int64
    CacheWritePriceMicroUSDPer1K  int64
    ReasoningPriceMicroUSDPer1K   int64
    EmbeddingPriceMicroUSDPer1K   int64
    InputPriceUSDPer1M            float64
    OutputPriceUSDPer1M           float64
    CachedInputPriceUSDPer1M      float64
    CacheWritePriceUSDPer1M       float64
    ReasoningPriceUSDPer1M        float64
    EmbeddingPriceUSDPer1M        float64
    EffectiveFrom                 string
    EffectiveTo                   string
    IsActive                      bool
    Source                        string
    MetadataJSON                  string
}

type InspectMerge struct {
    ResourceID   string
    ProviderCode string
    ProviderType string
    ModelAPIID   string
    APIBaseURL   string
    APIKey       string
    Variant      string
    SecretID     string
    SecretKey    string
    AWSRegion    string
}

type LLMInspectResult struct {
    OK                            bool
    Message                       string
    ProviderCode                  string
    ProviderType                  string
    ModelAPIID                    string
    ModelDisplayName              string
    ModelSizeLabel                string
    ContextWindowK                int
    MaxOutputTokens               int
    InputPriceMicroUSDPer1K       int64
    OutputPriceMicroUSDPer1K      int64
    CachedInputPriceMicroUSDPer1K int64
    ReasoningPriceMicroUSDPer1K   int64
    EmbeddingPriceMicroUSDPer1K   int64
    Source                        string
    RawMetadataJSON               string
    Variant                       string
    EnableTokenTailoring          bool
    SupportsCache                 bool
    SupportsThinking              bool
}
```

### 5.2 Repo 子接口（Stability:stable）

```go
type LlmProviderModelReader interface {
    ListProviderModels(ctx context.Context) ([]ProviderModel, error)
    SearchProviderModels(ctx context.Context, q ProviderModelListQuery) (ProviderModelListResult, error)
    GetProviderModel(ctx context.Context, id string) (ProviderModel, error)
    GetProviderModelByProviderAndModel(ctx context.Context, provider, model string) (ProviderModel, error)
}

// ProviderModelListQuery 管理端模型列表的分页/筛选输入
type ProviderModelListQuery struct {
    Search string
    Limit  int
    Offset int
}

// ProviderModelListResult 一页模型 + 筛选范围内总数
type ProviderModelListResult struct {
    Items  []ProviderModel
    Total  int
    Limit  int
    Offset int
}

type LlmProviderModelWriter interface {
    CreateProviderModel(ctx context.Context, m ProviderModel) (ProviderModel, error)
    UpdateProviderModel(ctx context.Context, m ProviderModel) (ProviderModel, error)
    DeleteProviderModel(ctx context.Context, id string) error
    UpdateProviderModelStatus(ctx context.Context, id string, status string) error
}

type LlmProviderModelValidator interface {
    ValidateProviderPair(ctx context.Context, provider, model string) (bool, error)
}

type ModelPricingRepo interface {
    UpsertModelPricingRule(ctx context.Context, rule ModelPricingRule) error
}

// 组合接口（仅用于 Wire 绑定与编译期检查）
type LlmProviderModelRepo interface {
    LlmProviderModelReader
    LlmProviderModelWriter
    LlmProviderModelValidator
    ModelPricingRepo
}

// 消费方按需依赖窄接口
type LlmProviderModelReaderWriter interface {
    LlmProviderModelReader
    LlmProviderModelWriter
}

type LlmProviderModelApplyBackend interface {
    LlmProviderModelReader
    LlmProviderModelWriter
    ModelPricingRepo
}
```

### 5.3 Usecase

```go
type LlmProviderModelUsecase struct {
    reader     LlmProviderModelReader
    writer     LlmProviderModelWriter
    validator  LlmProviderModelValidator
    pricing    ModelPricingRepo
    inspector  LLMInspector
    crypto     *CredentialCrypto
    agentRefs  AgentReferenceChecker
    // statsInjector 注入 30 天用量统计到响应的 ConfigJSON（仅响应装饰，不持久化）
    statsInjector *ModelStatsInjector
    lg         loggateway.Logger
}

func NewLlmProviderModelUsecase(
    reader LlmProviderModelReader,
    writer LlmProviderModelWriter,
    validator LlmProviderModelValidator,
    pricing ModelPricingRepo,
    inspector LLMInspector,
    crypto *CredentialCrypto,
    agentRefs AgentReferenceChecker,
    statsInjector *ModelStatsInjector, // 可为 nil
    lg loggateway.Logger,
) *LlmProviderModelUsecase

// 方法
func (u *LlmProviderModelUsecase) List(ctx context.Context) ([]ProviderModel, error)
func (u *LlmProviderModelUsecase) ListPaged(ctx context.Context, q ProviderModelListQuery) (ProviderModelListResult, error)
func (u *LlmProviderModelUsecase) Get(ctx context.Context, id string) (ProviderModel, error)
func (u *LlmProviderModelUsecase) GetByProviderAndModel(ctx context.Context, provider, model string) (ProviderModel, error)
func (u *LlmProviderModelUsecase) Create(ctx context.Context, in ProviderModel) (ProviderModel, error)
func (u *LlmProviderModelUsecase) Update(ctx context.Context, id string, patch ProviderModel) (ProviderModel, error)
func (u *LlmProviderModelUsecase) Delete(ctx context.Context, id string) error
func (u *LlmProviderModelUsecase) ValidatePair(ctx context.Context, provider, model string) (bool, string, error)
func (u *LlmProviderModelUsecase) Inspect(ctx context.Context, in InspectMerge) (LLMInspectResult, error)
```

**关键行为**：
- `List` / `Get`：通过 `sanitizeProviderModelForAPI` 脱敏后返回
- `List` / `ListPaged` / `Update`：返回前经 `statsInjector.InjectStats` 注入 30 天用量统计（`usage_*_30d`、`model_hotness_score` 等，见 §6.3）。前端用 PATCH 响应整行替换列表行，故 `Update` 必须与列表保持同样的装饰，否则统计列被清零
- `GetByProviderAndModel`：通过 `crypto.PrepareProviderModelForRuntime` 解密后返回（运行时使用）
- `Create` / `Update`：通过 `crypto.RequireKeyForPlaintext` + `crypto.ProcessConfigJSONForStorage` 加密后写入；同步调用 `syncProviderModelPricing` 更新定价规则（best-effort，失败不回滚）
- `Inspect`：通过 `mergeInspectConfigJSON` 合并请求字段与已存 config_json，再调用 `inspector.Run`

### 5.4 InspectMerge 合并逻辑

```go
func mergeInspectConfigJSON(cfg string, in *InspectMerge) {
    var c struct {
        ProviderType string `json:"provider_type"`
        APIBaseURL   string `json:"api_base_url"`
        APIKey       string `json:"api_key"`
        Variant      string `json:"variant"`
        SecretID     string `json:"secret_id"`
        SecretKey    string `json:"secret_key"`
        AWSRegion    string `json:"aws_region"`
    }
    if json.Unmarshal([]byte(cfg), &c) != nil {
        return
    }
    // 请求字段为空时从已存 config_json 回填
    if in.ProviderType == "" { in.ProviderType = c.ProviderType }
    if in.APIBaseURL == ""   { in.APIBaseURL = c.APIBaseURL }
    if in.APIKey == ""       { in.APIKey = c.APIKey }
    if in.Variant == ""      { in.Variant = c.Variant }
    if in.SecretID == ""     { in.SecretID = c.SecretID }
    if in.SecretKey == ""    { in.SecretKey = c.SecretKey }
    if in.AWSRegion == ""    { in.AWSRegion = c.AWSRegion }
}
```

### 5.5 凭据加密（CredentialCrypto）

文件：`internal/biz/credential_crypto.go`、`credential_key.go`、`channel_credential_crypto.go`

- 算法：AES-256-GCM，密钥来自 `ARANEA_CREDENTIAL_KEY` 环境变量
- 注入方式：Wire 构造函数注入（`NewCredentialCrypto`），消除全局 `SetCredentialKeyResolver`
- 关键方法：
  - `RequireKeyForPlaintext(ctx, cfg)` — 明文含凭据时强制要求密钥可用
  - `ProcessConfigJSONForStorage(ctx, cfg)` — 存储前加密 api_key / secret_key / ha_candidates[].api_key
  - `PrepareProviderModelForRuntime(ctx, m)` — 运行时解密 `(ProviderModel, error)`
  - `DecryptConfigJSONForRuntime(ctx, cfg)` — 返回 `(string, error)`，解密失败时 SysLogWarn
  - `IsCredentialEncryptionAvailable()` — 降级提示用
- 降级策略：密钥未配置时，启动警告 + 前端 q-banner 警告；写入时拒绝明文凭据；读取时返回原 JSON

---

## 六、Data 层

### 6.1 Ent Schema — `llm_provider_models`

文件：`internal/data/ent/schema/llm_provider_model.go`

```go
type LlmProviderModel struct {
    ent.Schema
}

func (LlmProviderModel) Annotations() []schema.Annotation {
    return []schema.Annotation{
        entsql.Annotation{Table: "llm_provider_models"},
    }
}

func (LlmProviderModel) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").Immutable().Unique().MaxLen(256),
        field.String("model_key").Unique().MaxLen(512),
        field.String("name").MaxLen(1024),
        field.Text("description").Default(""),
        field.String("status").Default("active"),
        field.Bool("enabled").Default(true),
        field.Int("sort_order").Default(0),
        field.String("provider").Default(""),
        field.String("model").Default(""),
        field.Text("config_json").Default(""),
        field.Text("metadata_json").Default(""),
        field.Bool("capability_text").Default(false),
        field.Bool("capability_vision").Default(false),
        field.Bool("capability_audio").Default(false),
        field.Bool("capability_file").Default(false),
        field.Bool("capability_tool_call").Default(false),
        field.Bool("capability_cache").Default(false),
        field.Bool("capability_thinking").Default(false),
        field.Bool("capability_text_only").Default(false),
        field.Bool("capabilities_explicit").Default(false),
        field.String("created_at").Default(""),
        field.String("updated_at").Default(""),
        field.String("deleted_at").Default(""),
    }
}

func (LlmProviderModel) Indexes() []ent.Index {
    return []ent.Index{
        index.Fields("provider", "enabled", "sort_order").StorageKey("idx_provider_models_provider"),
    }
}
```

**设计说明**：
- Provider 类型、Variant、认证信息、高可用配置等全部存储在 `config_json` JSON 字段中，不拆分为独立列
- 模型能力（`capability_*`）拆分为独立列，便于查询和索引
- `capabilities_explicit` 标记能力是否由用户显式指定（vs 由 Variant/配置推导）
- 唯一约束：`model_key` UNIQUE；业务唯一性：`(provider, model)` 由应用层保证

### 6.2 Ent Schema — `model_pricing_rules`

文件：`internal/data/ent/schema/model_pricing_rule.go`

```go
type ModelPricingRule struct {
    ent.Schema
}

func (ModelPricingRule) Annotations() []schema.Annotation {
    return []schema.Annotation{
        entsql.Annotation{Table: "model_pricing_rules"},
    }
}

func (ModelPricingRule) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").Immutable().Unique().MaxLen(512),
        field.String("provider_code").MaxLen(256),
        field.String("model_api_id").MaxLen(512),
        field.String("currency").Default("USD"),
        field.Int64("input_price_micro_usd_per_1k").Default(0),
        field.Int64("output_price_micro_usd_per_1k").Default(0),
        field.Int64("cached_input_price_micro_usd_per_1k").Default(0),
        field.Int64("reasoning_price_micro_usd_per_1k").Default(0),
        field.Int64("embedding_price_micro_usd_per_1k").Default(0),
        field.Int64("cache_write_price_micro_usd_per_1k").Default(0),
        field.Float("input_price_usd_per_1m").Default(0),
        field.Float("output_price_usd_per_1m").Default(0),
        field.Float("cached_input_price_usd_per_1m").Default(0),
        field.Float("reasoning_price_usd_per_1m").Default(0),
        field.Float("embedding_price_usd_per_1m").Default(0),
        field.Float("cache_write_price_usd_per_1m").Default(0),
        field.String("effective_from").Default(""),
        field.String("effective_to").Default(""),
        field.Bool("is_active").Default(true),
        field.String("source").Default("manual"),
        field.Text("metadata_json").Default("{}"),
        field.String("created_at").Default(""),
        field.String("updated_at").Default(""),
    }
}

func (ModelPricingRule) Indexes() []ent.Index {
    return []ent.Index{
        index.Fields("provider_code", "model_api_id", "effective_from").Unique(),
        index.Fields("provider_code", "model_api_id", "is_active", "effective_from").StorageKey("idx_pricing_rules_model_active"),
    }
}
```

### 6.3 `config_json` 结构

`config_json` 存储 Provider 连接配置和 trpc 运行时选项，完整结构如下：

```jsonc
{
  "provider_type": "openai",           // trpc Provider 类型枚举
  "variant": "deepseek",               // OpenAI Variant（仅 provider_type=openai）
  "provider_display_name": "DeepSeek", // UI 展示名
  "api_base_url": "https://api.deepseek.com",
  "api_key": "sk-...",                 // 写入后不回读；编辑时用 api_key_set 标记
  "api_key_set": true,                 // 标记是否已设置密钥
  "secret_id": "",                     // Hunyuan 专用
  "secret_key": "",                    // Hunyuan 专用
  "aws_region": "",                    // Bedrock 专用

  "model_category": [                  // 模型能力分类
    { "value": "reasoning", "label": "推理 / 复杂问题", "tooltip": "数学、逻辑、多步推导" }
  ],
  "model_size_label": "67B",
  "context_window_k": 128,
  "max_output_tokens": 8192,

  "input_price_micro_usd_per_1k": 550,
  "output_price_micro_usd_per_1k": 2190,
  "cached_input_price_micro_usd_per_1k": 0,
  "reasoning_price_micro_usd_per_1k": 2190,
  "embedding_price_micro_usd_per_1k": 0,

  "ha_mode": "",                       // "" | "failover" | "hedge"
  "ha_candidates": [
    { "name": "gpt-4o", "provider_type": "openai", "base_url": "https://backup.example.com/v1", "api_key": "sk-backup" }
  ],
  "ha_hedge_delay_ms": 100,

  "enable_token_tailoring": true,
  "token_tailoring_strategy": "middle-out",  // middle-out | head-out | tail-out
  "token_tailoring_safety_margin": 0.1,
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
  "channel_buffer_size": 256,

  "rate_limit_rpm": 0,                  // 速率限制（每分钟请求数）

  "retry_max_attempts": 0,
  "retry_base_delay_ms": 1000,
  "retry_max_delay_ms": 30000,

  "circuit_breaker_enabled": false,
  "circuit_breaker_failure_threshold": 3,
  "circuit_breaker_recovery_sec": 30,

  "capabilities": {                     // 模型能力位（与 capability_* 列同步）
    "text": true, "vision": false, "audio": false, "file": true,
    "tool_call": true, "cache": false, "thinking": false, "text_only": false
  },

  "tokens_per_second": null,
  "model_hotness_score": null,
  "usage_call_count_30d": null,
  "usage_total_tokens_30d": null,
  "usage_cost_micro_usd_30d": null,
  "success_rate_30d": null,
  "avg_latency_ms_30d": null,
  "last_used_at": "",
  "model_rating": 60,

  "raw_metadata_json": "",
  "metadata_source": ""
}
```

> **Token Tailoring 默认开启**：`enable_token_tailoring` 未显式配置且 `context_window_k > 0` 时，`catalogConfigToConfig`（`internal/provider/catalog.go`）默认启用裁剪（截断优于 API 上下文溢出硬错误）；显式配置 `"enable_token_tailoring": false` 可关闭。

### 6.4 Data 层 Repo 实现

文件：`internal/data/llm_provider_model.go`

```go
type llmProviderModelRepo struct {
    data *Data
}

var _ biz.LlmProviderModelRepo = (*llmProviderModelRepo)(nil)

func NewLlmProviderModelRepo(d *Data) biz.LlmProviderModelRepo

// 关键方法
func entToBizPM(lg loggateway.Logger, e *ent.LlmProviderModel) biz.ProviderModel
func (r *llmProviderModelRepo) ListProviderModels(ctx context.Context) ([]biz.ProviderModel, error)
func (r *llmProviderModelRepo) GetProviderModel(ctx context.Context, id string) (biz.ProviderModel, error)
func (r *llmProviderModelRepo) GetProviderModelByProviderAndModel(ctx context.Context, provider, model string) (biz.ProviderModel, error)
func (r *llmProviderModelRepo) CreateProviderModel(ctx context.Context, m biz.ProviderModel) (biz.ProviderModel, error)
func (r *llmProviderModelRepo) UpdateProviderModel(ctx context.Context, m biz.ProviderModel) (biz.ProviderModel, error)
func (r *llmProviderModelRepo) DeleteProviderModel(ctx context.Context, id string) error  // 软删：set deleted_at + status=deleted
func (r *llmProviderModelRepo) UpdateProviderModelStatus(ctx context.Context, id string, status string) error
func (r *llmProviderModelRepo) ValidateProviderPair(ctx context.Context, provider, model string) (bool, error)
func (r *llmProviderModelRepo) UpsertModelPricingRule(ctx context.Context, rule biz.ModelPricingRule) error  // 事务安全
```

**读写分离**：读用 `r.data.RW().Read(ctx)`，写用 `r.data.RW().Write(ctx)`。

**PricingConfigured 计算**：`configJSONHasPricing(lg, cfg)` 检测 config_json 是否含有效定价字段。

---

## 七、运行时层（Provider 桥接）

### 7.1 ProviderModelConfig

文件：`internal/provider/catalog.go`

```go
type ProviderModelConfig struct {
    ProviderType               string
    Variant                    string
    BaseURL                    string
    APIKey                     string
    ModelAPI                   string
    SecretID                   string
    SecretKey                  string
    AWSRegion                  string
    EnableTokenTailoring       bool
    TokenTailoringStrategy     string  // "middle-out" | "head-out" | "tail-out"
    TokenTailoringSafetyMargin float64
    ContextWindow              int
    MaxInputTokens             int
    OptimizeForCache           bool
    ReasoningBackfill          bool
    ShowToolCallDelta          bool
    Cache                      CacheConfig
    KeepAliveMinutes           int
    ChannelBufferSize          int
    HA                         HAConfig
    RateLimitRPM               int
    Capabilities               biz.ModelCapabilities
    Retry                      RetryConfig
    CB                         CBConfig
}

type HACandidateConfig struct {
    Name         string `json:"name"`
    ProviderType string `json:"provider_type"`
    BaseURL      string `json:"base_url"`
    APIKey       string `json:"api_key"`
}

type HAConfig struct {
    Mode         string
    Candidates   []HACandidateConfig
    HedgeDelayMs int
}

type CacheConfig struct {
    SystemPrompt bool
    Tools        bool
    Messages     bool
}

type RetryConfig struct {
    MaxAttempts int  // T1.2: -1 = 无限重试（默认）, 0 = 禁用, >0 = 有限次
    BaseDelayMs int
    MaxDelayMs  int
}

type CBConfig struct {
    Enabled          bool
    FailureThreshold int
    RecoverySec      int
}
```

### 7.2 trpc Model 构建

文件：`internal/provider/trpc_llm.go`

```go
func TRPCModelForProviderModel(ctx context.Context, catalog biz.TeamModelCatalog, rt *RoundTrip, prov, modelAPI string, lg loggateway.Logger) (trpcmodel.Model, error) {
    pm, err := catalog.GetByProviderAndModel(ctx, strings.TrimSpace(prov), strings.TrimSpace(modelAPI))
    if err != nil { return nil, err }
    cfg, err := ResolveModelConfig(ModelCatalogInput{Model: pm.Model, ConfigJSON: pm.ConfigJSON})
    if err != nil { return nil, err }
    return trpcModelFromProviderModelConfig(ctx, cfg, rt, lg)
}

func trpcModelFromProviderModelConfig(ctx context.Context, cfg ProviderModelConfig, rt *RoundTrip, lg loggateway.Logger) (trpcmodel.Model, error) {
    // 1. URL 安全校验（outboundguard.ValidateURL）
    // 2. MapProviderType 映射 provider_type → trpc provider name
    // 3. buildProviderOptions 构建通用 + 专属选项
    // 4. trpcprovider.Model() 工厂创建实例
    // 5. WrapModelWithMetrics 装饰指标
    // 6. wrapHA 包装 Failover/Hedge
}

func MapProviderType(pt string) string {
    switch strings.ToLower(strings.TrimSpace(pt)) {
    case "anthropic":              return "anthropic"
    case "gemini", "google gemini": return "gemini"
    case "ollama":                 return "ollama"
    case "hunyuan":                return "hunyuan"
    case "huggingface":            return "huggingface"
    case "bedrock":                return "bedrock"
    default:                       return "openai"
    }
}
```

### 7.3 Provider 专属选项构建

```go
func buildProviderOptions(cfg ProviderModelConfig, rt *RoundTrip, lg loggateway.Logger) []trpcprovider.Option {
    var opts []trpcprovider.Option
    // 通用选项：APIKey / BaseURL / Variant / ChannelBufferSize / TokenTailoring / MaxInputTokens
    // 自定义 Transport：rate-limit / retry / circuit-breaker 包装
    // 专属选项：buildOpenAISpecificOptions / buildAnthropicSpecificOptions / buildGeminiSpecificOptions / buildOllamaSpecificOptions / buildHunyuanSpecificOptions
    return opts
}

// 各 Provider 专属 builder 通过 trpcprovider.WithXxxOption(providerOpts...) 注入
func buildOpenAISpecificOptions(cfg ProviderModelConfig) []trpcprovider.Option    // OptimizeForCache / ReasoningBackfill / ShowToolCallDelta / ContextWindow
func buildAnthropicSpecificOptions(cfg ProviderModelConfig) []trpcprovider.Option // CacheSystemPrompt / CacheTools / CacheMessages / ShowToolCallDelta
func buildGeminiSpecificOptions(cfg ProviderModelConfig, rt *RoundTrip) []trpcprovider.Option // GeminiClientConfig (APIKey + Backend + HTTPClient) / MaxInputTokens
func buildOllamaSpecificOptions(cfg ProviderModelConfig) []trpcprovider.Option    // KeepAlive / MaxInputTokens
func buildHunyuanSpecificOptions(cfg ProviderModelConfig) []trpcprovider.Option   // SecretId / SecretKey / ContextWindow
```

### 7.4 Failover/Hedge 包装

```go
func wrapHA(primary trpcmodel.Model, cfg ProviderModelConfig, rt *RoundTrip, lg loggateway.Logger) (trpcmodel.Model, error) {
    switch strings.ToLower(strings.TrimSpace(cfg.HA.Mode)) {
    case "failover": return wrapFailover(cfg, rt, primary, lg)
    case "hedge":    return wrapHedge(cfg, rt, primary, lg)
    }
    return primary, nil
}

func wrapFailover(cfg ProviderModelConfig, rt *RoundTrip, primary trpcmodel.Model, lg loggateway.Logger) (trpcmodel.Model, error) {
    // 1. 构建候选模型列表（含 outboundguard.ValidateURL 预检 + WrapModelWithMetrics）
    // 2. trpcfailover.New(trpcfailover.WithCandidates(candidates...))
    // 3. 候选构建失败时 lg.Warn 记录（step ID: provider.ha_failover_candidate_skip）
}

func wrapHedge(cfg ProviderModelConfig, rt *RoundTrip, primary trpcmodel.Model, lg loggateway.Logger) (trpcmodel.Model, error) {
    // 1. 构建候选模型列表（含预检 + 指标装饰）
    // 2. trpchedge.New(trpchedge.WithCandidates(...), trpchedge.WithDelay(...))
    // 3. 候选构建失败时 lg.Warn 记录（step ID: provider.ha_hedge_candidate_skip）
}
```

### 7.5 能力推导

文件：`internal/provider/trpc_llm.go`、`internal/provider/capabilities.go`

```go
// CapabilitiesForProviderModel 返回 provider-model 行的有效能力集。
// 显式值优先；否则从 Variant（DeepSeek 强制 TextOnly）和 caching/thinking 标志推导。
func CapabilitiesForProviderModel(pm biz.ProviderModel) biz.ModelCapabilities

// ModelSupportsImageAttachments 判断模型是否支持图像附件
func ModelSupportsImageAttachments(ctx context.Context, catalog biz.TeamModelCatalog, prov, model string, lg loggateway.Logger) bool

// ModelSupportsFileAttachments 判断模型是否支持文件附件
func ModelSupportsFileAttachments(ctx context.Context, catalog biz.TeamModelCatalog, prov, model string, lg loggateway.Logger) bool
```

**BR10 例外（显式豁免）**：`internal/agent/llmcompat` 仅用于意图识别等侧信道调用，在仅有 inline `ProviderAPIConfig`（无 catalog 行）时走 `trpcprovider.Model` + `WrapModelWithMetrics`。主对话/Team/Graph 运行时必须经 `TRPCModelForProviderModel`。

### 7.6 HTTP Transport 链

文件：`internal/provider/roundtrip.go`、`rate_limit_transport.go`、`retry_transport.go`、`circuit_breaker_transport.go`、`timeout_transport.go`、`retry_classifier.go`

Transport 装饰链（按需启用）：

```
http.DefaultTransport (或 rt.HTTP.Transport)
    ↓ newTimeoutTransport (若 rt.HTTP.Timeout > 0；为每次 attempt 注入 deadline，防止无限挂起)
    ↓ wrapRateLimitTransport (若 rate_limit_rpm > 0)
    ↓ wrapRetryTransport (若 retry_max_attempts != 0；T1.2: -1=无限, 0=禁用, >0=有限)
    ↓ wrapCircuitBreakerTransport (若 circuit_breaker_enabled)
    → trpcprovider.WithHTTPClientTransport(transport)
```

> **T1.2 LLM 重试增强（Sprint 1, 2026-06-18）**：
> - 默认 `MaxAttempts = -1`（无限重试），指数退避 1s/2s/4s/8s/16s/30s（封顶 30s）
> - `retryTransport` 改为 `for {}` 循环 + `shouldRetry(attempt)` 方法，支持 `maxRetries=-1` 无限模式
> - 新增 `RetryCallback` 类型：`func(req *http.Request, attempt, maxRetries int, err error, delay time.Duration)`
> - 回调在 backoff sleep 前触发，由 `runtime.TurnDeps.RoundTripForSession(sessionID)` 注入，发布 `llm_retry` Envelope 到 chat channel
> - 分层合规：`provider` 包不导入 `event` 包，回调由 runtime 层桥接事件发布
>
> **超时错误分类（2026-07-25）**：
> - `timeoutTransport` 触发 per-attempt 超时时返回 `attemptTimeoutError`（包装 `context.DeadlineExceeded`），`classifyError` 判定为 `RetryWithBackoff`——挂起重连属瞬时故障
> - 调用方主动取消/调用方 deadline（裸 `context.Canceled` / `context.DeadlineExceeded`）仍判定 `RetryFatal`，不重试
> - 判别顺序：`attemptTimeoutError` 必须先于 `context.Canceled/DeadlineExceeded` 检查（其 Unwrap 链含 `DeadlineExceeded`）
> - 前端配套：`llm_retry` notice → `stores/chat/llmRetryStore` → `LlmRetryBanner` 重连横幅；流恢复（`step.streaming`/`turn.started`）或终态时清除

---

## 八、Service 层

文件：`internal/service/llm_provider_model.go`

```go
type LlmProviderModelService struct {
    v1.UnimplementedLlmProviderModelServiceServer
    uc  *biz.LlmProviderModelUsecase
    mon *biz.MonitorUsecase
    lg  loggateway.Logger
}

func NewLlmProviderModelService(uc *biz.LlmProviderModelUsecase, mon *biz.MonitorUsecase, lg loggateway.Logger) *LlmProviderModelService

// Proto ↔ Biz 转换
func toProtoPM(m biz.ProviderModel) *v1.ProviderModel  // 含 CapabilitiesForProviderModel 推导
func patchFromProto(pb *v1.ProviderModel) biz.ProviderModel

// RPC 实现
func (s *LlmProviderModelService) ListProviderModels(ctx, *v1.ListProviderModelsRequest) (*v1.ListProviderModelsResponse, error)
//   - page/page_size 任一为 0 时返回全量目录；否则走 uc.ListPaged 服务端分页 + search 过滤
func (s *LlmProviderModelService) CreateProviderModel(ctx, *v1.CreateProviderModelRequest) (*v1.ProviderModel, error)
func (s *LlmProviderModelService) GetProviderModel(ctx, *v1.GetProviderModelRequest) (*v1.ProviderModel, error)
func (s *LlmProviderModelService) RevealProviderModelCredentials(ctx, *v1.RevealProviderModelCredentialsRequest) (*v1.RevealProviderModelCredentialsResponse, error)
func (s *LlmProviderModelService) UpdateProviderModel(ctx, *v1.UpdateProviderModelRequest) (*v1.ProviderModel, error)
func (s *LlmProviderModelService) DeleteProviderModel(ctx, *v1.DeleteProviderModelRequest) (*emptypb.Empty, error)
func (s *LlmProviderModelService) InspectProviderModel(ctx, *v1.InspectProviderModelRequest) (*v1.InspectProviderModelResponse, error)
func (s *LlmProviderModelService) ValidateProviderPair(ctx, *v1.ValidateProviderPairRequest) (*v1.ValidateProviderPairResponse, error)
```

**关键映射**：
- `toProtoPM`：调用 `provider.CapabilitiesForProviderModel(m)` 推导能力位，填充 `Capabilities` 和 `PricingConfigured`
- `InspectProviderModel`：将 Proto 请求字段（含 variant / secret_id / secret_key / aws_region）映射到 `biz.InspectMerge`，调用 `uc.Inspect`
- `RevealProviderModelCredentials`：调用 Usecase 解密并返回 api_key / secret_key / ha_candidates 凭据

---

## 九、Wire 注入

```
data.ProviderSet   → NewLlmProviderModelRepo → Wire 绑定 biz.LlmProviderModelRepo
                   → NewCredentialCrypto（凭据加密，构造函数注入）
                   → NewMCPRepo → Wire 绑定 biz.MCPServerReader

biz.ProviderSet    → NewLlmProviderModelUsecase（接收 reader/writer/validator/pricing/inspector/crypto/agentRefs/lg）

service.ProviderSet → NewLlmProviderModelService（接收 uc + lg）
```

**DI 合规要点**：
- `CredentialCrypto` 通过构造函数注入到 `LlmProviderModelUsecase` / `ChannelUsecase` / `MCPServerUsecase` / `SystemSettingService`
- 已删除全局 `SetInspector` 和 `SetCredentialKeyResolver`（改为构造函数注入）
- `NewSystemSettingRepo` 不再有全局副作用

---

## 十、Web 前端设计

### 10.1 文件结构

```
web/src/
├── config/
│   ├── providerPresets.ts              ← Provider 预设配置（13 个 shell）
│   ├── providerRuntimeOverlay.ts       ← models.dev → trpc 运行时映射
│   ├── providerRuntimeOverlay.types.ts ← 类型定义
│   └── provider_runtime_overlay.json   ← overlay 数据（与后端同步）
├── features/
│   └── platform/
│       ├── api.ts                      ← 平台资源 API
│       ├── types.ts                    ← 统一类型（ProviderConfig / ModelCategory / CapabilityChip）
│       ├── providerUtils.ts            ← 共享工具函数
│       ├── useProviderList.ts          ← Provider 列表 composable
│       ├── useProviderCatalog.ts       ← 目录选择 composable
│       ├── useProviderCredentials.ts   ← 凭据管理 composable
│       ├── useProviderInspect.ts       ← Inspect 检查 composable
│       ├── useProviderPresets.ts       ← 预设应用 composable
│       ├── useProviderWizard.ts        ← Provider 向导编排 composable
│       ├── useProviderSave.ts          ← Provider 保存 composable
│       ├── useResourceManagerPage.ts   ← 资源管理页面 composable
│       └── usePlatformResource.ts      ← 组合式函数
├── components/
│   └── platform/
│       ├── ProviderModelsTable.vue     ← 列表表格（Variant Chip + HA Chip + 定价警告）
│       ├── ProviderTrendDialog.vue     ← 趋势看板
│       ├── ProviderHAConfig.vue        ← 高可用配置组件
│       ├── ProviderWizardStep1Connect.vue ← 步骤 1：连接与身份
│       ├── ProviderWizardStep2Specs.vue   ← 步骤 2：模型分类与规格
│       ├── ProviderWizardStep3HA.vue      ← 步骤 3：高可用配置
│       ├── ProviderWizardStep4Advanced.vue ← 步骤 4：高级选项
│       ├── ProviderLogo.vue            ← Provider 图标
│       └── providerModelUi.ts          ← 表格列定义 + UI 工具函数
└── pages/
    └── ResourceManagerPage.vue         ← 资源管理页面（QStepper 四步表单 + 凭据加密警告）
```

### 10.2 类型定义

文件：`web/src/features/platform/types.ts`、`web/src/config/providerRuntimeOverlay.types.ts`

```typescript
export type AuthType = "api_key" | "secret_id_key" | "aws_config" | "none";
export type ProviderType = "openai" | "anthropic" | "gemini" | "ollama" | "hunyuan" | "huggingface" | "bedrock";
export type OpenAIVariant = "openai" | "deepseek" | "qwen" | "hunyuan";

export type ProviderPreset = {
  key: string;
  label: string;
  providerCode: string;
  providerType: ProviderType;
  variant?: OpenAIVariant;
  authType: AuthType;
  apiBaseUrl: string;
  metadataApi: "full" | "partial" | "limited" | "none";
  metadataNote: string;
  models: ProviderModelPreset[];
};
```

### 10.3 ProviderModelsTable.vue

列表表格组件，展示模型信息和操作按钮。

**Props**：

```typescript
defineProps<{
  rows: PlatformResource[];
  loading?: boolean;
}>();
```

**展示区域**：

| 区域 | 内容 |
|------|------|
| 身份 | 状态点 + Provider 展示名 + 模型名 + Provider/Type Chip + **Variant Chip**（仅 OpenAI 类型且 Variant ≠ openai 时显示） |
| 模型类型 | 模型分类 Chip 列表 |
| 指标 | 模型大小 / 上下文 / TPS |
| 使用情况 | 热度进度条 + 30天调用/费用 |
| 密钥 | API 密钥设置状态 Chip |
| 高可用 | **Failover 蓝色 Chip / Hedge 紫色 Chip** |
| 定价 | **定价缺失警告图标**（PricingConfigured=false 时显示） |
| 操作 | 启用 Toggle + 趋势/编辑/删除按钮 |

### 10.4 ProviderTrendDialog.vue

模型历史趋势看板弹窗。

**Props**：

```typescript
defineProps<{
  modelValue: boolean;
  row: PlatformResource | null;
}>();
```

**展示内容**：

| 模块 | 内容 |
|------|------|
| 摘要卡片 | 热度 / 30天调用 / 30天Token / 30天费用 |
| 趋势柱状图 | 每日 Token 消耗柱状图 |
| 详情表格 | 成功率 / 平均延迟 / TPS / 上下文 / 最大输出 |

### 10.5 ResourceManagerPage.vue

资源管理页面，集成 QStepper 四步表单、Provider 类型筛选、Variant Chip、高可用 Chip、凭据加密降级警告。

**四步表单组件**：
- `ProviderWizardStep1Connect.vue` — 连接与身份（Provider 预设 / 类型 / Variant / 编码 / API URL / 密钥 / SecretId/Key / AWS Region）
- `ProviderWizardStep2Specs.vue` — 模型分类与规格（分类 / 大小 / 上下文 / 最大输出 / 定价 / 检查模型）
- `ProviderWizardStep3HA.vue` — 高可用配置（HA 模式 / 候选模型 / Hedge 延迟）
- `ProviderWizardStep4Advanced.vue` — 高级选项（Token Tailoring / Cache / Tool Call Delta / Keep Alive / Channel Buffer）

**表单数据结构**：

```typescript
type ProviderFormData = {
  presetKey: string;
  providerType: ProviderType;
  variant: OpenAIVariant;
  providerCode: string;
  providerDisplayName: string;
  modelApiId: string;
  modelDisplayName: string;
  apiBaseUrl: string;
  apiKey: string;
  secretId: string;
  secretKey: string;
  awsRegion: string;
  enabled: boolean;

  modelCategory: string[];
  modelSizeLabel: string;
  contextWindowK: number | null;
  maxOutputTokens: number | null;
  inputPrice: number | null;
  outputPrice: number | null;
  cachedInputPrice: number | null;
  reasoningPrice: number | null;
  embeddingPrice: number | null;

  haMode: "" | "failover" | "hedge";
  haCandidates: { name: string; providerType: ProviderType; baseUrl: string; apiKey: string }[];
  haHedgeDelayMs: number;

  enableTokenTailoring: boolean;
  optimizeForCache: boolean;
  reasoningBackfill: boolean;
  showToolCallDelta: boolean;
  cacheSystemPrompt: boolean;
  cacheTools: boolean;
  cacheMessages: boolean;
  keepAliveMinutes: number;
  channelBufferSize: number;
};
```

**提交逻辑**：将 `ProviderFormData` 序列化为 `config_json` JSON 字符串，通过 `createPlatformResource` / `updatePlatformResource` 提交。

### 10.6 ProviderHAConfig.vue

高可用配置独立组件，供步骤 3 使用。支持候选模型条目的增删改、Hedge 延迟配置。

### 10.7 API 调用

文件：`web/src/features/platform/api.ts`

```typescript
export async function listPlatformResources(resource: "llm-provider-models"): Promise<PlatformResource[]>
export async function createPlatformResource(resource: "llm-provider-models", payload: PlatformResourceInput): Promise<PlatformResource>
export async function updatePlatformResource(resource: "llm-provider-models", id: string, payload: Partial<PlatformResourceInput>): Promise<PlatformResource>
export async function deletePlatformResource(resource: "llm-provider-models", id: string): Promise<void>
export async function inspectProviderModel(input: InspectProviderModelInput): Promise<InspectProviderModelResult>
export async function validateModel(provider: string, model: string): Promise<ValidateModelResult>
```

**数据流合规**：`useAgentProviderModelPicker` + `useProviderWizard` 均走 Store，不直接调 API。

---

## 十一、LLM Gateway 设计参考与演进方向

> 本节整合自 `architecture/platform-architecture.md` 第二篇，描述 LLM Gateway 的三层架构、路由策略、容错降级与本项目落点。

### 11.1 解决的问题

收口全部模型调用，避免：多厂商协议分裂、密钥与路由散落各微服务、无统一容错与配额、成本与 SLA 不可见。核心价值：**统一管控、可配置路由、多层降级、可观测与成本归因**。

### 11.2 三层架构

| 层 | 职责 | Aranea 落点 |
|----|------|------------|
| **接入** | 协议/参数归一、鉴权、限流与配额 → 产出内部标准请求 | Kratos HTTP 中间件 + Identity/Capability；入口构造 `RuntimeContext` |
| **决策** | 路由引擎、实例健康与延迟视图、负载均衡、降级编排；策略应动态可配 | `Capability.Provider` / ModelProfile + Operations（开关、告警）；可为独立路由服务演进 |
| **出口** | 厂商请求/响应转换、SSE 流式归一、结构化日志与指标 | `internal/provider` + Provider 适配器；结构化日志与 span |

### 11.3 路由策略（多策略组合）

1. **按能力**：任务类型或标签 → 匹配模型族（推理 / 代码 / 长上下文 / 轻量等）
2. **按成本**：级联——小模型先答，质量闸门不过关再上大模型
3. **按延迟**：滑动窗口 P95；综合推理时延 + 网络 RTT；实时路径显式标记
4. **语义路由（可选）**：嵌入 + 阈值 → 任务类型；纳入误判与隐私、运维成本评审

### 11.4 容错：四层降级 + 熔断

1. **同模型**：默认无限重试（`MaxAttempts = -1`，T1.2）+ 指数退避（1s/2s/4s/8s/16s/30s 封顶）；只对可重试错误（超时、429 等），参数/鉴权类立即失败；每次重试经 `RetryCallback` 发布 `llm_retry` 事件通知前端
2. **跨厂商**：降级链预置 + 出口层协议转换，对上游尽量无感
3. **跨等级**：降级轻量或小上下文（需产品预先接受体验下限）
4. **兜底**：语义缓存、固定话术、人工接管

**熔断**：LLM 长尾延迟常态，阈值应显著宽于通用 RPC；结合错误率 / 慢请求占比开合，避免误杀。

### 11.5 负载均衡（LLM 特化）

不宜简单轮询：按并发、队列深度、预估 token 吞吐加权；多区域时在地缘就近与实例空闲度间折衷。

### 11.6 统一协议面

对外以主流开放对话 API 为事实标准可显著降改造成本。须统一：SSE 事件形态、工具/函数调用 JSON、token 计数口径、厂商错误码映射为网关错误码，以及厂商专有能力的受限扩展出口。

### 11.7 语义缓存与可观测

- **缓存**：适用于稳态、弱时效问答；强时效资讯、强个性化、合规敏感场景应禁用或短 TTL
- **可观测**：每请求 trace、路由决策、降级层级、目标模型/厂商、token 与成本；监控成功率、延迟分位、路由命中与降级占比，反哺调参

---

*文档版本：基于 trpc-agent-go `model/provider` 体系设计；与 `8 agent-title.md` §8.1、`2 agents-create.md` 对齐。LLM Gateway 整合自 `architecture/platform-architecture.md`。开发进度与任务清单见 [9-provider.development.md](./9-provider.development.md)。*
