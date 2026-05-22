# Provider 模型层 — 实现设计文档

> 对应需求：`9 provider.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

LLM Provider/Model 管理：多厂商注册、模型目录、Failover/Hedge 高可用、TokenTailor 自动裁剪。核心包 `internal/provider` 桥接 trpc-agent-go `model` 包。

**当前实现状态**（2026-05-17 现状对齐）：
- ✅ Proto CRUD + Inspect + ValidatePair 已实现
- ✅ `internal/provider/trpc_llm.go` 已实现按 `provider_type` 分发构建 `model.Model`（5 种原生 Provider + 4 种 Variant）
- ✅ `internal/provider/catalog.go` 已实现 `CatalogConfig` 解析和合并（含所有 Provider 专属字段 + HA 配置）
- ✅ Failover/Hedge 包装已实现（`wrapHA` + `wrapFailover` / `wrapHedge`）
- ✅ Provider 专属选项构建已实现（OpenAI/Anthropic/Gemini/Ollama/Hunyuan 各自 builder）
- ✅ `internal/llminspect/inspect.go` 已实现 OpenRouter / OpenAI-Compatible / Anthropic 三条探测路径 + DeepSeek 路由
- ✅ Pricing 定价规则已实现（`UpsertModelPricingRule`；Create/Update 时自动同步）
- ✅ `internal/provider/stream_delta.go` 流式 Delta 合并已实现
- ✅ `internal/provider/roundtrip.go` HTTP Transport 注入已实现
- ✅ 前端 `providerPresets.ts` 已对齐 trpc Provider 类型枚举（20 个预设）
- ✅ 前端 `ProviderModelRow.vue` 列表行已实现（6 列网格布局、热度、用量、密钥状态）
- ✅ 前端 `ProviderTrendDialog.vue` 趋势看板已实现（30 天趋势柱状图、汇总卡片、详情表）
- ✅ 前端 `ResourceManagerPage.vue` 管理页面已实现（搜索、分页、创建/编辑弹窗）
- ✅ Agent 构建链路已接入（`internal/agent/trpc_build.go` + `internal/service/session_title_llm.go`）
- ⏳ Inspect 请求/响应扩展字段（variant、secret_id、secret_key、aws_region、enable_token_tailoring、supports_cache、supports_thinking）
- ⏳ `mergeInspectConfigJSON` 仅合并 3 个字段，缺 variant / secret_id / secret_key / aws_region
- ⏳ 前端添加/编辑弹窗四步表单（当前为单弹窗表单，非设计文档 §6 的四步表单）
- ⏳ 前端 Variant Chip 展示（ProviderModelRow 未展示 Variant Chip）
- ⏳ 前端 HA Chip 展示（ProviderModelRow 未展示 Failover/Hedge Chip）
- ⏳ llminspect 缺少 Gemini / Ollama / Hunyuan 专属探测路径
- ⏳ HuggingFace / Bedrock Provider 未注册到 trpc provider 工厂（前端预设已预留）
- ⏳ 凭据未加密存储（api_key 明文存 SQLite config_json），前端对apikey 增加显示按钮，点击可以查看明文

---

## 二、Proto 层

### 2.1 现有 Proto（完整定义）

文件：`api/kratos/llm_provider_model/v1/llm_provider_model.proto`

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
}

message ListProviderModelsResponse {
  repeated ProviderModel items = 1;
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
}

service LlmProviderModelService {
  rpc ListProviderModels(google.protobuf.Empty) returns (ListProviderModelsResponse) {
    option (google.api.http) = {get: "/v1/llm-provider-models"};
  }
  rpc CreateProviderModel(CreateProviderModelRequest) returns (ProviderModel) {
    option (google.api.http) = {post: "/v1/llm-provider-models" body: "*"};
  }
  rpc GetProviderModel(GetProviderModelRequest) returns (ProviderModel) {
    option (google.api.http) = {get: "/v1/llm-provider-models/{id}"};
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

### 2.2 待新增 Proto 字段

#### InspectProviderModelRequest 新增字段

```protobuf
message InspectProviderModelRequest {
  // ... 现有字段 1-6 ...
  string variant = 7;       // OpenAI Variant: openai/deepseek/qwen/hunyuan
  string secret_id = 8;     // Hunyuan SecretId
  string secret_key = 9;    // Hunyuan SecretKey
  string aws_region = 10;   // Bedrock AWS Region
}
```

#### InspectProviderModelResponse 新增字段

```protobuf
message InspectProviderModelResponse {
  // ... 现有字段 1-16 ...
  string variant = 17;                // 检测到的 OpenAI Variant
  bool enable_token_tailoring = 18;   // 是否启用 Token Tailoring
  bool supports_cache = 19;           // 是否支持 Prompt Cache
  bool supports_thinking = 20;        // 是否支持思考/推理模式
}
```

---

## 三、Biz 层

### 3.1 领域模型（当前实现）

文件：`internal/biz/llm_provider_model.go`

```go
type ProviderModel struct {
    ID           string
    Key          string
    Name         string
    Description  string
    Status       string
    Enabled      bool
    SortOrder    int
    Provider     string
    Model        string
    ConfigJSON   string
    MetadataJSON string
    CreatedAt    string
    UpdatedAt    string
    DeletedAt    string
}

type ModelPricingRule struct {
    ID                            string
    ProviderCode                  string
    ModelAPIID                    string
    Currency                      string
    InputPriceMicroUSDPer1K       int64
    OutputPriceMicroUSDPer1K      int64
    CachedInputPriceMicroUSDPer1K int64
    ReasoningPriceMicroUSDPer1K   int64
    EmbeddingPriceMicroUSDPer1K   int64
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
}
```

### 3.2 InspectMerge 扩展（待实现）

```go
type InspectMerge struct {
    ResourceID   string
    ProviderCode string
    ProviderType string
    ModelAPIID   string
    APIBaseURL   string
    APIKey       string
    Variant      string // 新增：OpenAI Variant
    SecretID     string // 新增：Hunyuan SecretId
    SecretKey    string // 新增：Hunyuan SecretKey
    AWSRegion    string // 新增：Bedrock AWS Region
}
```

### 3.3 Repo 接口（当前实现）

```go
type LlmProviderModelRepo interface {
    ListProviderModels(ctx context.Context) ([]ProviderModel, error)
    GetProviderModel(ctx context.Context, id string) (ProviderModel, error)
    GetProviderModelByProviderAndModel(ctx context.Context, provider, model string) (ProviderModel, error)
    CreateProviderModel(ctx context.Context, m ProviderModel) (ProviderModel, error)
    UpdateProviderModel(ctx context.Context, m ProviderModel) (ProviderModel, error)
    DeleteProviderModel(ctx context.Context, id string) error
    ValidateProviderPair(ctx context.Context, provider, model string) (bool, error)
    UpsertModelPricingRule(ctx context.Context, rule ModelPricingRule) error
}
```

### 3.4 Usecase（当前实现）

```go
type LlmProviderModelUsecase struct {
    repo LlmProviderModelRepo
}

func NewLlmProviderModelUsecase(repo LlmProviderModelRepo) *LlmProviderModelUsecase
func (u *LlmProviderModelUsecase) List(ctx context.Context) ([]ProviderModel, error)
func (u *LlmProviderModelUsecase) Get(ctx context.Context, id string) (ProviderModel, error)
func (u *LlmProviderModelUsecase) GetByProviderAndModel(ctx context.Context, provider, model string) (ProviderModel, error)
func (u *LlmProviderModelUsecase) Create(ctx context.Context, in ProviderModel) (ProviderModel, error)
func (u *LlmProviderModelUsecase) Update(ctx context.Context, id string, patch ProviderModel) (ProviderModel, error)
func (u *LlmProviderModelUsecase) Delete(ctx context.Context, id string) error
func (u *LlmProviderModelUsecase) ValidatePair(ctx context.Context, provider, model string) (bool, string, error)
func (u *LlmProviderModelUsecase) Inspect(ctx context.Context, in InspectMerge) (llminspect.Result, error)
```

### 3.5 Inspect 方法扩展（待实现）

`Inspect` 方法需扩展 `mergeInspectConfigJSON` 以支持新字段：

```go
func mergeInspectConfigJSON(cfg string, in *InspectMerge) {
    var c struct {
        ProviderType string `json:"provider_type"`
        APIBaseURL   string `json:"api_base_url"`
        APIKey       string `json:"api_key"`
        Variant      string `json:"variant"`       // 新增
        SecretID     string `json:"secret_id"`      // 新增
        SecretKey    string `json:"secret_key"`      // 新增
        AWSRegion    string `json:"aws_region"`      // 新增
    }
    if json.Unmarshal([]byte(cfg), &c) != nil {
        return
    }
    if in.ProviderType == "" {
        in.ProviderType = c.ProviderType
    }
    if in.APIBaseURL == "" {
        in.APIBaseURL = c.APIBaseURL
    }
    if in.APIKey == "" {
        in.APIKey = c.APIKey
    }
    if in.Variant == "" {
        in.Variant = c.Variant
    }
    if in.SecretID == "" {
        in.SecretID = c.SecretID
    }
    if in.SecretKey == "" {
        in.SecretKey = c.SecretKey
    }
    if in.AWSRegion == "" {
        in.AWSRegion = c.AWSRegion
    }
}
```

---

## 四、Data 层

### 4.1 Ent Schema（当前实现）

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
        field.String("created_at").Default(""),
        field.String("updated_at").Default(""),
        field.String("deleted_at").Default(""),
    }
}
```

**设计说明**：Provider 类型、Variant、认证信息、高可用配置等全部存储在 `config_json` JSON 字段中，不拆分为独立列。这与需求文档 §7.2 的 `config_json` 结构对齐。

### 4.2 Data 层 Repo 实现（当前实现）

文件：`internal/data/llm_provider_model.go`

```go
type llmProviderModelRepo struct {
    data *Data
}

func NewLlmProviderModelRepo(d *Data) biz.LlmProviderModelRepo

func entToBizPM(e *ent.LlmProviderModel) biz.ProviderModel {
    return biz.ProviderModel{
        ID: e.ID, Key: e.ModelKey, Name: e.Name,
        Description: e.Description, Status: e.Status, Enabled: e.Enabled,
        SortOrder: e.SortOrder, Provider: e.Provider, Model: e.Model,
        ConfigJSON: e.ConfigJSON, MetadataJSON: e.MetadataJSON,
        CreatedAt: e.CreatedAt, UpdatedAt: e.UpdatedAt, DeletedAt: e.DeletedAt,
    }
}

func (r *llmProviderModelRepo) ListProviderModels(ctx context.Context) ([]biz.ProviderModel, error) {
    rows, err := r.data.entClient.LlmProviderModel.Query().
        Where(llmprovidermodel.DeletedAtEQ("")).
        Order(
            llmprovidermodel.BySortOrder(),
            llmprovidermodel.ByCreatedAt(entsql.OrderDesc()),
        ).
        All(ctx)
    // ...
}

func (r *llmProviderModelRepo) GetProviderModelByProviderAndModel(ctx context.Context, provider, model string) (biz.ProviderModel, error) {
    row, err := r.data.entClient.LlmProviderModel.Query().
        Where(
            llmprovidermodel.ProviderEQ(provider),
            llmprovidermodel.ModelEQ(model),
            llmprovidermodel.EnabledEQ(true),
            llmprovidermodel.DeletedAtEQ(""),
        ).
        Only(ctx)
    // ...
}

func (r *llmProviderModelRepo) CreateProviderModel(ctx context.Context, m biz.ProviderModel) (biz.ProviderModel, error) {
    saved, err := r.data.entClient.LlmProviderModel.Create().
        SetID(m.ID).SetModelKey(m.Key).SetName(m.Name).
        SetDescription(m.Description).SetStatus(m.Status).SetEnabled(m.Enabled).
        SetSortOrder(m.SortOrder).SetProvider(m.Provider).SetModel(m.Model).
        SetConfigJSON(m.ConfigJSON).SetMetadataJSON(m.MetadataJSON).
        SetCreatedAt(m.CreatedAt).SetUpdatedAt(m.UpdatedAt).SetDeletedAt("").
        Save(ctx)
    // ...
}

func (r *llmProviderModelRepo) UpdateProviderModel(ctx context.Context, m biz.ProviderModel) (biz.ProviderModel, error) {
    err := r.data.entClient.LlmProviderModel.UpdateOneID(m.ID).
        SetModelKey(m.Key).SetName(m.Name).
        SetDescription(m.Description).SetStatus(m.Status).SetEnabled(m.Enabled).
        SetSortOrder(m.SortOrder).SetProvider(m.Provider).SetModel(m.Model).
        SetConfigJSON(m.ConfigJSON).SetMetadataJSON(m.MetadataJSON).
        SetUpdatedAt(m.UpdatedAt).
        Exec(ctx)
    // ...
}

func (r *llmProviderModelRepo) DeleteProviderModel(ctx context.Context, id string) error {
    now := nowRFC3339()
    return r.data.entClient.LlmProviderModel.UpdateOneID(id).
        SetDeletedAt(now).SetStatus("deleted").SetUpdatedAt(now).Exec(ctx)
}

func (r *llmProviderModelRepo) UpsertModelPricingRule(ctx context.Context, rule biz.ModelPricingRule) error {
    // 先查活跃规则，存在则更新，不存在则创建
    // ...
}
```

### 4.3 `config_json` 结构定义

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

---

## 五、运行时层（Provider 桥接）

### 5.1 CatalogConfig（当前实现）

文件：`internal/provider/catalog.go`

```go
type CatalogConfig struct {
    ProviderType         string
    Variant              string
    BaseURL              string
    APIKey               string
    ModelAPI             string
    SecretID             string
    SecretKey            string
    AWSRegion            string
    EnableTokenTailoring bool
    ContextWindow        int
    MaxInputTokens       int
    OptimizeForCache     bool
    ReasoningBackfill    bool
    ShowToolCallDelta    bool
    CacheSystemPrompt    bool
    CacheTools           bool
    CacheMessages        bool
    KeepAliveMinutes     int
    ChannelBufferSize    int
    HAMode               string
    HACandidates         []HACandidateConfig
    HAHedgeDelayMs       int
}

type HACandidateConfig struct {
    Name         string `json:"name"`
    ProviderType string `json:"provider_type"`
    BaseURL      string `json:"base_url"`
    APIKey       string `json:"api_key"`
}
```

### 5.2 trpc Model 构建（当前实现）

文件：`internal/provider/trpc_llm.go`

```go
func TRPCModelForProviderModel(ctx context.Context, catalog *biz.LlmProviderModelUsecase, rt *RoundTrip, prov, modelAPI string) (trpcmodel.Model, error) {
    pm, err := catalog.GetByProviderAndModel(ctx, strings.TrimSpace(prov), strings.TrimSpace(modelAPI))
    if err != nil {
        return nil, err
    }
    cfg, err := CatalogFromProviderModel(pm)
    if err != nil {
        return nil, err
    }
    cfg = MergeCatalogConfig(cfg, pm.ConfigJSON)
    return trpcModelFromCatalogConfig(ctx, cfg, rt)
}

func trpcModelFromCatalogConfig(ctx context.Context, cfg CatalogConfig, rt *RoundTrip) (trpcmodel.Model, error) {
    name := strings.TrimSpace(cfg.ModelAPI)
    providerName := MapProviderType(cfg.ProviderType)
    opts := buildProviderOptions(cfg, rt)
    m, err := trpcprovider.Model(providerName, name, opts...)
    if err != nil {
        return nil, err
    }
    return wrapHA(ctx, m, cfg, rt)
}

func MapProviderType(pt string) string {
    switch strings.ToLower(strings.TrimSpace(pt)) {
    case "anthropic": return "anthropic"
    case "gemini":    return "gemini"
    case "ollama":    return "ollama"
    case "hunyuan":   return "hunyuan"
    default:          return "openai"
    }
}
```

### 5.3 Provider 专属选项构建（当前实现）

```go
func buildProviderOptions(cfg CatalogConfig, rt *RoundTrip) []trpcprovider.Option {
    var opts []trpcprovider.Option
    if apiKey := strings.TrimSpace(cfg.APIKey); apiKey != "" {
        opts = append(opts, trpcprovider.WithAPIKey(apiKey))
    }
    if baseURL := strings.TrimSpace(cfg.BaseURL); baseURL != "" {
        opts = append(opts, trpcprovider.WithBaseURL(baseURL))
    }
    if v := mapVariant(cfg.ProviderType); v != "" {
        opts = append(opts, trpcprovider.WithVariant(v))
    } else if v := strings.TrimSpace(cfg.Variant); v != "" {
        opts = append(opts, trpcprovider.WithVariant(v))
    }
    if cfg.ChannelBufferSize > 0 {
        opts = append(opts, trpcprovider.WithChannelBufferSize(cfg.ChannelBufferSize))
    }
    if cfg.EnableTokenTailoring {
        opts = append(opts, trpcprovider.WithEnableTokenTailoring(true))
    }
    if cfg.MaxInputTokens > 0 {
        opts = append(opts, trpcprovider.WithMaxInputTokens(cfg.MaxInputTokens))
    }
    if rt != nil && rt.HTTP != nil && rt.HTTP.Transport != nil {
        opts = append(opts, trpcprovider.WithHTTPClientTransport(rt.HTTP.Transport))
    }
    opts = append(opts, buildOpenAISpecificOptions(cfg)...)
    opts = append(opts, buildAnthropicSpecificOptions(cfg)...)
    opts = append(opts, buildGeminiSpecificOptions(cfg, rt)...)
    opts = append(opts, buildOllamaSpecificOptions(cfg)...)
    opts = append(opts, buildHunyuanSpecificOptions(cfg)...)
    return opts
}

func buildOpenAISpecificOptions(cfg CatalogConfig) []trpcprovider.Option {
    var providerOpts []trpcopenai.Option
    if cfg.OptimizeForCache {
        providerOpts = append(providerOpts, trpcopenai.WithOptimizeForCache(true))
    }
    if cfg.ReasoningBackfill {
        providerOpts = append(providerOpts, trpcopenai.WithReasoningContentBackfill(true))
    }
    if cfg.ShowToolCallDelta {
        providerOpts = append(providerOpts, trpcopenai.WithShowToolCallDelta(true))
    }
    if cfg.ContextWindow > 0 {
        providerOpts = append(providerOpts, trpcopenai.WithContextWindow(cfg.ContextWindow))
    }
    if len(providerOpts) == 0 { return nil }
    return []trpcprovider.Option{trpcprovider.WithOpenAIOption(providerOpts...)}
}

func buildAnthropicSpecificOptions(cfg CatalogConfig) []trpcprovider.Option {
    var providerOpts []trpcanthropic.Option
    if cfg.CacheSystemPrompt {
        providerOpts = append(providerOpts, trpcanthropic.WithCacheSystemPrompt(true))
    }
    if cfg.CacheTools {
        providerOpts = append(providerOpts, trpcanthropic.WithCacheTools(true))
    }
    if cfg.CacheMessages {
        providerOpts = append(providerOpts, trpcanthropic.WithCacheMessages(true))
    }
    if cfg.ShowToolCallDelta {
        providerOpts = append(providerOpts, trpcanthropic.WithShowToolCallDelta(true))
    }
    if len(providerOpts) == 0 { return nil }
    return []trpcprovider.Option{trpcprovider.WithAnthropicOption(providerOpts...)}
}

func buildGeminiSpecificOptions(cfg CatalogConfig, rt *RoundTrip) []trpcprovider.Option {
    var providerOpts []trpcgemini.Option
    apiKey := strings.TrimSpace(cfg.APIKey)
    if apiKey != "" || (rt != nil && rt.HTTP != nil && rt.HTTP.Transport != nil) {
        gcc := &genai.ClientConfig{APIKey: apiKey, Backend: genai.BackendVertexAI}
        if rt != nil && rt.HTTP != nil && rt.HTTP.Transport != nil {
            gcc.HTTPClient = &http.Client{Transport: rt.HTTP.Transport}
        }
        providerOpts = append(providerOpts, trpcgemini.WithGeminiClientConfig(gcc))
    }
    if cfg.ContextWindow > 0 {
        providerOpts = append(providerOpts, trpcgemini.WithMaxInputTokens(cfg.ContextWindow))
    }
    if len(providerOpts) == 0 { return nil }
    return []trpcprovider.Option{trpcprovider.WithGeminiOption(providerOpts...)}
}

func buildOllamaSpecificOptions(cfg CatalogConfig) []trpcprovider.Option {
    var providerOpts []trpcollama.Option
    if cfg.KeepAliveMinutes > 0 {
        providerOpts = append(providerOpts, trpcollama.WithKeepAlive(time.Duration(cfg.KeepAliveMinutes)*time.Minute))
    }
    if cfg.ContextWindow > 0 {
        providerOpts = append(providerOpts, trpcollama.WithMaxInputTokens(cfg.ContextWindow))
    }
    if len(providerOpts) == 0 { return nil }
    return []trpcprovider.Option{trpcprovider.WithOllamaOption(providerOpts...)}
}

func buildHunyuanSpecificOptions(cfg CatalogConfig) []trpcprovider.Option {
    var providerOpts []trpchunyuan.Option
    if secretID := strings.TrimSpace(cfg.SecretID); secretID != "" {
        providerOpts = append(providerOpts, trpchunyuan.WithSecretId(secretID))
    }
    if secretKey := strings.TrimSpace(cfg.SecretKey); secretKey != "" {
        providerOpts = append(providerOpts, trpchunyuan.WithSecretKey(secretKey))
    }
    if cfg.ContextWindow > 0 {
        providerOpts = append(providerOpts, trpchunyuan.WithContextWindow(cfg.ContextWindow))
    }
    if len(providerOpts) == 0 { return nil }
    return []trpcprovider.Option{trpcprovider.WithHunyuanOption(providerOpts...)}
}
```

### 5.4 Failover/Hedge 包装（当前实现）

```go
func wrapHA(ctx context.Context, primary trpcmodel.Model, cfg CatalogConfig, rt *RoundTrip) (trpcmodel.Model, error) {
    switch strings.ToLower(strings.TrimSpace(cfg.HAMode)) {
    case "failover": return wrapFailover(cfg, rt, primary)
    case "hedge":    return wrapHedge(cfg, rt, primary)
    }
    return primary, nil
}

func wrapFailover(cfg CatalogConfig, rt *RoundTrip, primary trpcmodel.Model) (trpcmodel.Model, error) {
    candidates := []trpcmodel.Model{primary}
    for _, c := range cfg.HACandidates {
        m, err := trpcModelFromCandidate(c, rt)
        if err != nil { continue }
        candidates = append(candidates, m)
    }
    if len(candidates) < 2 { return primary, nil }
    fo, err := trpcfailover.New(trpcfailover.WithCandidates(candidates...))
    if err != nil { return primary, nil }
    return fo, nil
}

func wrapHedge(cfg CatalogConfig, rt *RoundTrip, primary trpcmodel.Model) (trpcmodel.Model, error) {
    candidates := []trpcmodel.Model{primary}
    for _, c := range cfg.HACandidates {
        m, err := trpcModelFromCandidate(c, rt)
        if err != nil { continue }
        candidates = append(candidates, m)
    }
    if len(candidates) < 2 { return primary, nil }
    hedgeOpts := []trpchedge.Option{trpchedge.WithCandidates(candidates...)}
    if cfg.HAHedgeDelayMs > 0 {
        hedgeOpts = append(hedgeOpts, trpchedge.WithDelay(time.Duration(cfg.HAHedgeDelayMs)*time.Millisecond))
    }
    h, err := trpchedge.New(hedgeOpts...)
    if err != nil { return primary, nil }
    return h, nil
}

func trpcModelFromCandidate(c HACandidateConfig, rt *RoundTrip) (trpcmodel.Model, error) {
    providerName := MapProviderType(c.ProviderType)
    opts := []trpcprovider.Option{}
    if apiKey := strings.TrimSpace(c.APIKey); apiKey != "" {
        opts = append(opts, trpcprovider.WithAPIKey(apiKey))
    }
    if baseURL := strings.TrimSpace(c.BaseURL); baseURL != "" {
        opts = append(opts, trpcprovider.WithBaseURL(baseURL))
    }
    if rt != nil && rt.HTTP != nil && rt.HTTP.Transport != nil {
        opts = append(opts, trpcprovider.WithHTTPClientTransport(rt.HTTP.Transport))
    }
    return trpcprovider.Model(providerName, c.Name, opts...)
}
```

---

## 六、Service 层

### 6.1 Service 实现（当前实现）

文件：`internal/service/llm_provider_model.go`

```go
type LlmProviderModelService struct {
    v1.UnimplementedLlmProviderModelServiceServer
    uc *biz.LlmProviderModelUsecase
}

func NewLlmProviderModelService(uc *biz.LlmProviderModelUsecase) *LlmProviderModelService {
    return &LlmProviderModelService{uc: uc}
}

func toProtoPM(m biz.ProviderModel) *v1.ProviderModel {
    return &v1.ProviderModel{
        Id: m.ID, Key: m.Key, Name: m.Name, Description: m.Description,
        Status: m.Status, Enabled: m.Enabled, SortOrder: int32(m.SortOrder),
        Provider: m.Provider, Model: m.Model,
        ConfigJson: m.ConfigJSON, MetadataJson: m.MetadataJSON,
        CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt, DeletedAt: m.DeletedAt,
    }
}

func patchFromProto(pb *v1.ProviderModel) biz.ProviderModel {
    if pb == nil { return biz.ProviderModel{} }
    return biz.ProviderModel{
        Key: pb.GetKey(), Name: pb.GetName(), Description: pb.GetDescription(),
        Status: pb.GetStatus(), Enabled: pb.GetEnabled(), SortOrder: int(pb.GetSortOrder()),
        Provider: pb.GetProvider(), Model: pb.GetModel(),
        ConfigJSON: pb.GetConfigJson(), MetadataJSON: pb.GetMetadataJson(),
    }
}

func (s *LlmProviderModelService) ListProviderModels(ctx context.Context, _ *emptypb.Empty) (*v1.ListProviderModelsResponse, error) {
    items, err := s.uc.List(ctx)
    if err != nil { return nil, err }
    resp := &v1.ListProviderModelsResponse{Items: make([]*v1.ProviderModel, 0, len(items))}
    for i := range items {
        resp.Items = append(resp.Items, toProtoPM(items[i]))
    }
    return resp, nil
}

func (s *LlmProviderModelService) CreateProviderModel(ctx context.Context, req *v1.CreateProviderModelRequest) (*v1.ProviderModel, error) {
    in := biz.ProviderModel{
        Key: req.GetKey(), Name: req.GetName(), Description: req.GetDescription(),
        Status: req.GetStatus(), Enabled: req.GetEnabled(), SortOrder: int(req.GetSortOrder()),
        Provider: req.GetProvider(), Model: req.GetModel(),
        ConfigJSON: req.GetConfigJson(), MetadataJSON: req.GetMetadataJson(),
    }
    out, err := s.uc.Create(ctx, in)
    if err != nil { return nil, err }
    return toProtoPM(out), nil
}

func (s *LlmProviderModelService) GetProviderModel(ctx context.Context, req *v1.GetProviderModelRequest) (*v1.ProviderModel, error) {
    m, err := s.uc.Get(ctx, req.GetId())
    if err != nil {
        if errors.Is(err, sql.ErrNoRows) {
            return nil, kerrors.NotFound("LLM_PROVIDER_MODEL", "provider model not found")
        }
        return nil, err
    }
    return toProtoPM(m), nil
}

func (s *LlmProviderModelService) UpdateProviderModel(ctx context.Context, req *v1.UpdateProviderModelRequest) (*v1.ProviderModel, error) {
    if req.GetProviderModel() == nil {
        return nil, kerrors.BadRequest("LLM_PROVIDER_MODEL", "provider_model body is required")
    }
    out, err := s.uc.Update(ctx, req.GetId(), patchFromProto(req.GetProviderModel()))
    if err != nil {
        if errors.Is(err, sql.ErrNoRows) {
            return nil, kerrors.NotFound("LLM_PROVIDER_MODEL", "provider model not found")
        }
        return nil, err
    }
    return toProtoPM(out), nil
}

func (s *LlmProviderModelService) DeleteProviderModel(ctx context.Context, req *v1.DeleteProviderModelRequest) (*emptypb.Empty, error) {
    if err := s.uc.Delete(ctx, req.GetId()); err != nil { return nil, err }
    return &emptypb.Empty{}, nil
}

func (s *LlmProviderModelService) InspectProviderModel(ctx context.Context, req *v1.InspectProviderModelRequest) (*v1.InspectProviderModelResponse, error) {
    out, err := s.uc.Inspect(ctx, biz.InspectMerge{
        ResourceID: req.GetResourceId(), ProviderCode: req.GetProviderCode(),
        ProviderType: req.GetProviderType(), ModelAPIID: req.GetModelApiId(),
        APIBaseURL: req.GetApiBaseUrl(), APIKey: req.GetApiKey(),
    })
    if err != nil { return nil, err }
    return &v1.InspectProviderModelResponse{
        Ok: out.OK, Message: out.Message, ProviderCode: out.ProviderCode,
        ProviderType: out.ProviderType, ModelApiId: out.ModelAPIID,
        ModelDisplayName: out.ModelDisplayName, ModelSizeLabel: out.ModelSizeLabel,
        ContextWindowK: int32(out.ContextWindowK), MaxOutputTokens: int32(out.MaxOutputTokens),
        InputPriceMicroUsdPer_1K: out.InputPriceMicroUSDPer1K,
        OutputPriceMicroUsdPer_1K: out.OutputPriceMicroUSDPer1K,
        CachedInputPriceMicroUsdPer_1K: out.CachedInputPriceMicroUSDPer1K,
        ReasoningPriceMicroUsdPer_1K: out.ReasoningPriceMicroUSDPer1K,
        EmbeddingPriceMicroUsdPer_1K: out.EmbeddingPriceMicroUSDPer1K,
        Source: out.Source, RawMetadataJson: out.RawMetadataJSON,
    }, nil
}

func (s *LlmProviderModelService) ValidateProviderPair(ctx context.Context, req *v1.ValidateProviderPairRequest) (*v1.ValidateProviderPairResponse, error) {
    ok, msg, err := s.uc.ValidatePair(ctx, req.GetProvider(), req.GetModel())
    if err != nil { return nil, err }
    return &v1.ValidateProviderPairResponse{Ok: ok, Message: msg}, nil
}
```

### 6.2 Inspect 扩展（待实现）

Proto 新增字段后，Service 层需更新 `InspectProviderModel` 方法：

```go
func (s *LlmProviderModelService) InspectProviderModel(ctx context.Context, req *v1.InspectProviderModelRequest) (*v1.InspectProviderModelResponse, error) {
    out, err := s.uc.Inspect(ctx, biz.InspectMerge{
        ResourceID:   req.GetResourceId(),
        ProviderCode: req.GetProviderCode(),
        ProviderType: req.GetProviderType(),
        ModelAPIID:   req.GetModelApiId(),
        APIBaseURL:   req.GetApiBaseUrl(),
        APIKey:       req.GetApiKey(),
        Variant:      req.GetVariant(),       // 新增
        SecretID:     req.GetSecretId(),       // 新增
        SecretKey:    req.GetSecretKey(),       // 新增
        AWSRegion:    req.GetAwsRegion(),       // 新增
    })
    if err != nil { return nil, err }
    return &v1.InspectProviderModelResponse{
        // ... 现有字段 ...
        Variant:              out.Variant,              // 新增
        EnableTokenTailoring: out.EnableTokenTailoring,  // 新增
        SupportsCache:        out.SupportsCache,          // 新增
        SupportsThinking:     out.SupportsThinking,       // 新增
    }, nil
}
```

---

## 七、Wire 注入

已有注入链（无需修改）：

```
data.ProviderSet   → NewLlmProviderModelRepo
biz.ProviderSet    → NewLlmProviderModelUsecase
service.ProviderSet → NewLlmProviderModelService
```

---

## 八、Web 前端设计

### 8.1 文件结构

```
web/src/
├── config/
│   └── providerPresets.ts          ← Provider 预设配置（已实现）
├── features/
│   └── platform/
│       ├── api.ts                  ← 平台资源 API（已实现）
│       └── usePlatformResource.ts  ← 组合式函数（已实现）
├── components/
│   └── platform/
│       ├── ProviderModelRow.vue    ← 列表行组件（已实现）
│       ├── ProviderTrendDialog.vue ← 趋势看板（已实现）
│       ├── ProviderFormDialog.vue  ← 添加/编辑弹窗（待实现）
│       └── ProviderHAConfig.vue    ← 高可用配置组件（待实现）
└── pages/
    └── ResourceManagerPage.vue     ← 资源管理页面（已实现，需改造）
```

### 8.2 providerPresets.ts（已实现）

类型定义：

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

预设映射表（18+ 预设，详见 `providerPresets.ts`）：

| 前端预设 key | trpc ProviderType | Variant | authType |
|-------------|-------------------|---------|----------|
| `openai` | `openai` | `openai` | `api_key` |
| `anthropic` | `anthropic` | — | `api_key` |
| `gemini` | `gemini` | — | `api_key` |
| `deepseek` | `openai` | `deepseek` | `api_key` |
| `aliyun-qwen` | `openai` | `qwen` | `api_key` |
| `tencent-hunyuan` | `hunyuan` | — | `secret_id_key` |
| `ollama` | `ollama` | — | `none` |
| `huggingface` | `huggingface` | — | `api_key` |
| `bedrock` | `bedrock` | — | `aws_config` |

### 8.3 ProviderModelRow.vue（已实现）

列表行组件，展示模型信息和操作按钮。

**Props**：

```typescript
defineProps<{
  row: PlatformResource;
  saving?: boolean;
}>();

defineEmits<{
  "toggle-enabled": [row: PlatformResource, enabled: boolean];
  trend: [row: PlatformResource];
  edit: [row: PlatformResource];
  delete: [row: PlatformResource];
}>();
```

**展示区域**（6 列网格布局）：

| 区域 | 内容 |
|------|------|
| 身份 | 状态点 + Provider 展示名 + 模型名 + Provider/Type Chip |
| 模型类型 | 模型分类 Chip 列表 |
| 指标 | 模型大小 / 上下文 / TPS |
| 使用情况 | 热度进度条 + 30天调用/费用 |
| 密钥 | API 密钥设置状态 Chip |
| 操作 | 启用 Toggle + 趋势/编辑/删除按钮 |

**待改造项**：
- 新增 Variant Chip（仅 OpenAI 类型且 Variant ≠ openai 时显示）
- 新增高可用 Chip（Failover 蓝色 / Hedge 紫色）

### 8.4 ProviderTrendDialog.vue（已实现）

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

### 8.5 ProviderFormDialog.vue（待实现）

添加/编辑 Provider 弹窗，四步表单。

**Props**：

```typescript
defineProps<{
  modelValue: boolean;
  editRow?: PlatformResource | null;
}>();

defineEmits<{
  "update:modelValue": [value: boolean];
  saved: [row: PlatformResource];
}>();
```

**步骤一：连接与身份**

```vue
<template>
  <QCardSection>
    <div class="text-subtitle1 q-mb-md">① 连接与身份</div>
    <div class="q-gutter-md">
      <QSelect
        v-model="form.presetKey"
        :options="presetOptions"
        label="Provider 预设"
        emit-value
        map-options
        outlined
        dense
        @update:model-value="onPresetChange"
      />
      <QSelect
        v-model="form.providerType"
        :options="PROVIDER_TYPE_OPTIONS"
        label="Provider 类型"
        emit-value
        map-options
        outlined
        dense
        @update:model-value="onProviderTypeChange"
      />
      <QSelect
        v-if="form.providerType === 'openai'"
        v-model="form.variant"
        :options="VARIANT_OPTIONS"
        label="Variant"
        emit-value
        map-options
        outlined
        dense
        @update:model-value="onVariantChange"
      />
      <QInput v-model="form.providerCode" label="Provider 编码" outlined dense
        :rules="[val => !!val || '必填', val => /^[a-z0-9-]+$/.test(val) || '仅小写字母、数字、连字符']" />
      <QInput v-model="form.providerDisplayName" label="Provider 显示名" outlined dense />
      <QInput v-model="form.modelApiId" label="模型 API ID" outlined dense
        :rules="[val => !!val || '必填']" />
      <QInput v-model="form.modelDisplayName" label="模型展示名" outlined dense />
      <QInput v-model="form.apiBaseUrl" label="API 基础 URL" outlined dense />
      <QInput v-if="authType === 'api_key'" v-model="form.apiKey" label="API 密钥"
        type="password" outlined dense :placeholder="isEdit ? '留空表示不修改' : ''" />
      <template v-if="authType === 'secret_id_key'">
        <QInput v-model="form.secretId" label="Secret ID" outlined dense />
        <QInput v-model="form.secretKey" label="Secret Key" type="password" outlined dense />
      </template>
      <QSelect v-if="authType === 'aws_config'" v-model="form.awsRegion"
        :options="AWS_REGIONS" label="AWS Region" outlined dense emit-value map-options />
      <QToggle v-model="form.enabled" label="已启用" />
    </div>
  </QCardSection>
</template>
```

**步骤二：模型分类与规格**

```vue
<template>
  <QCardSection>
    <div class="text-subtitle1 q-mb-md">② 模型分类与规格</div>
    <div class="q-gutter-md">
      <QSelect v-model="form.modelCategory" :options="MODEL_CATEGORY_OPTIONS"
        label="模型分类" multiple emit-value map-options outlined dense />
      <QInput v-model="form.modelSizeLabel" label="模型大小标签" outlined dense placeholder="如 7B / 70B" />
      <QInput v-model.number="form.contextWindowK" label="上下文窗口" type="number"
        outlined dense suffix="K tokens" />
      <QInput v-model.number="form.maxOutputTokens" label="最大输出 Token" type="number" outlined dense />
      <QInput v-model.number="form.inputPrice" label="输入价格" type="number"
        outlined dense suffix="µ$/1K token" />
      <QInput v-model.number="form.outputPrice" label="输出价格" type="number"
        outlined dense suffix="µ$/1K token" />
      <QInput v-model.number="form.cachedInputPrice" label="缓存输入价格" type="number"
        outlined dense suffix="µ$/1K token" />
      <QInput v-model.number="form.reasoningPrice" label="推理价格" type="number"
        outlined dense suffix="µ$/1K token" />
      <QInput v-model.number="form.embeddingPrice" label="嵌入价格" type="number"
        outlined dense suffix="µ$/1K token" />
      <QBtn label="检查模型" color="primary" outline :loading="inspecting"
        @click="onInspect" />
    </div>
  </QCardSection>
</template>
```

**步骤三：高可用配置**

```vue
<template>
  <QCardSection>
    <div class="text-subtitle1 q-mb-md">③ 高可用配置</div>
    <div class="q-gutter-md">
      <QSelect v-model="form.haMode" :options="HA_MODE_OPTIONS"
        label="高可用模式" emit-value map-options outlined dense />
      <template v-if="form.haMode">
        <div v-for="(c, idx) in form.haCandidates" :key="idx" class="row q-gutter-sm items-end">
          <QInput v-model="c.name" label="模型名" outlined dense class="col-3" />
          <QSelect v-model="c.providerType" :options="PROVIDER_TYPE_OPTIONS"
            label="Provider 类型" emit-value map-options outlined dense class="col-2" />
          <QInput v-model="c.baseUrl" label="Base URL" outlined dense class="col-3" />
          <QInput v-model="c.apiKey" label="API Key" type="password" outlined dense class="col-3" />
          <QBtn flat round dense icon="delete" color="negative" @click="form.haCandidates.splice(idx, 1)" />
        </div>
        <QBtn flat label="+ 添加候选模型" color="primary" @click="form.haCandidates.push({ name: '', providerType: 'openai', baseUrl: '', apiKey: '' })" />
        <QInput v-if="form.haMode === 'hedge'" v-model.number="form.haHedgeDelayMs"
          label="Hedge 延迟" type="number" outlined dense suffix="ms" />
      </template>
    </div>
  </QCardSection>
</template>
```

**步骤四：高级选项**

```vue
<template>
  <QCardSection>
    <div class="text-subtitle1 q-mb-md">④ 高级选项</div>
    <div class="q-gutter-md">
      <QToggle v-model="form.enableTokenTailoring" label="Token Tailoring" />
      <QToggle v-if="form.providerType === 'openai'" v-model="form.optimizeForCache"
        label="Prompt Cache 优化" />
      <QToggle v-if="form.providerType === 'openai' && form.variant === 'deepseek'"
        v-model="form.reasoningBackfill" label="Reasoning 回填" />
      <QToggle v-if="['openai', 'anthropic'].includes(form.providerType)"
        v-model="form.showToolCallDelta" label="Tool Call Delta" />
      <template v-if="form.providerType === 'anthropic'">
        <QToggle v-model="form.cacheSystemPrompt" label="System Prompt Cache" />
        <QToggle v-model="form.cacheTools" label="Tools Cache" />
        <QToggle v-model="form.cacheMessages" label="Messages Cache" />
      </template>
      <QInput v-if="form.providerType === 'ollama'" v-model.number="form.keepAliveMinutes"
        label="Keep Alive" type="number" outlined dense suffix="分钟" />
      <QInput v-model.number="form.channelBufferSize" label="Channel Buffer Size"
        type="number" outlined dense />
    </div>
  </QCardSection>
</template>
```

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

```typescript
function buildConfigJson(form: ProviderFormData): string {
  return JSON.stringify({
    provider_type: form.providerType,
    variant: form.variant || undefined,
    provider_display_name: form.providerDisplayName || undefined,
    api_base_url: form.apiBaseUrl || undefined,
    api_key: form.apiKey || undefined,
    api_key_set: form.apiKey ? true : undefined,
    secret_id: form.secretId || undefined,
    secret_key: form.secretKey || undefined,
    aws_region: form.awsRegion || undefined,
    model_category: form.modelCategory.map(v => MODEL_CATEGORY_MAP[v]),
    model_size_label: form.modelSizeLabel || undefined,
    context_window_k: form.contextWindowK || undefined,
    max_output_tokens: form.maxOutputTokens || undefined,
    input_price_micro_usd_per_1k: form.inputPrice || undefined,
    output_price_micro_usd_per_1k: form.outputPrice || undefined,
    cached_input_price_micro_usd_per_1k: form.cachedInputPrice || undefined,
    reasoning_price_micro_usd_per_1k: form.reasoningPrice || undefined,
    embedding_price_micro_usd_per_1k: form.embeddingPrice || undefined,
    ha_mode: form.haMode || undefined,
    ha_candidates: form.haCandidates.filter(c => c.name),
    ha_hedge_delay_ms: form.haMode === "hedge" ? form.haHedgeDelayMs : undefined,
    enable_token_tailoring: form.enableTokenTailoring || undefined,
    optimize_for_cache: form.optimizeForCache || undefined,
    reasoning_content_backfill: form.reasoningBackfill || undefined,
    show_tool_call_delta: form.showToolCallDelta || undefined,
    cache_system_prompt: form.cacheSystemPrompt || undefined,
    cache_tools: form.cacheTools || undefined,
    cache_messages: form.cacheMessages || undefined,
    keep_alive_minutes: form.keepAliveMinutes || undefined,
    channel_buffer_size: form.channelBufferSize || undefined,
  });
}
```

### 8.6 ProviderHAConfig.vue（待实现）

高可用配置独立组件，供 `ProviderFormDialog.vue` 步骤三使用。

```vue
<template>
  <div class="q-gutter-md">
    <QSelect v-model="modelValue.haMode" :options="haModeOptions"
      label="高可用模式" emit-value map-options outlined dense
      @update:model-value="emitChange" />
    <template v-if="modelValue.haMode">
      <QCard v-for="(c, idx) in modelValue.haCandidates" :key="idx" flat bordered class="q-pa-sm">
        <div class="row q-gutter-sm items-center">
          <QInput v-model="c.name" label="模型名" outlined dense class="col" />
          <QSelect v-model="c.providerType" :options="PROVIDER_TYPE_OPTIONS"
            label="Provider" emit-value map-options outlined dense style="min-width: 140px" />
          <QInput v-model="c.baseUrl" label="Base URL" outlined dense class="col" />
          <QInput v-model="c.apiKey" label="API Key" type="password" outlined dense style="max-width: 200px" />
          <QBtn flat round dense icon="close" color="negative" @click="removeCandidate(idx)" />
        </div>
      </QCard>
      <QBtn flat label="+ 添加候选模型" color="primary" icon="add" @click="addCandidate" />
      <QInput v-if="modelValue.haMode === 'hedge'" v-model.number="modelValue.haHedgeDelayMs"
        label="Hedge 延迟" type="number" outlined dense suffix="ms" min="0" />
    </template>
  </div>
</template>
```

### 8.7 ResourceManagerPage.vue 改造（待实现）

当前页面使用统一的 `ResourceManagerPage.vue` 管理 Provider/Model 列表。需改造：

1. **Provider 类型筛选**：`QSelect` 多选，选项为 `PROVIDER_TYPE_OPTIONS`
2. **ProviderFormDialog 集成**：替换现有添加/编辑弹窗
3. **Variant Chip**：列表行中 Provider 类型为 openai 且 Variant ≠ openai 时显示
4. **高可用 Chip**：列表行中配置了 Failover/Hedge 时显示

### 8.8 API 调用（已实现）

文件：`web/src/features/platform/api.ts`

```typescript
export async function listPlatformResources(resource: "llm-provider-models"): Promise<PlatformResource[]>
export async function createPlatformResource(resource: "llm-provider-models", payload: PlatformResourceInput): Promise<PlatformResource>
export async function updatePlatformResource(resource: "llm-provider-models", id: string, payload: Partial<PlatformResourceInput>): Promise<PlatformResource>
export async function deletePlatformResource(resource: "llm-provider-models", id: string): Promise<void>
export async function inspectProviderModel(input: InspectProviderModelInput): Promise<InspectProviderModelResult>
export async function validateModel(provider: string, model: string): Promise<ValidateModelResult>
```

---

## 九、实现优先级

| 优先级 | 任务 | 涉及文件 |
|--------|------|----------|
| P0 | Proto Inspect 新增字段 | `api/kratos/llm_provider_model/v1/llm_provider_model.proto` |
| P0 | Biz InspectMerge 扩展 | `internal/biz/llm_provider_model.go` |
| P0 | Service Inspect 映射更新 | `internal/service/llm_provider_model.go` |
| P0 | 前端 ProviderFormDialog 四步表单 | `web/src/components/platform/ProviderFormDialog.vue`（新建） |
| P1 | 前端 Variant Chip 展示 | `web/src/components/platform/ProviderModelRow.vue` |
| P1 | 前端高可用 Chip 展示 | `web/src/components/platform/ProviderModelRow.vue` |
| P1 | 前端 ProviderHAConfig 组件 | `web/src/components/platform/ProviderHAConfig.vue`（新建） |
| P2 | ResourceManagerPage Provider 类型筛选 | `web/src/pages/ResourceManagerPage.vue` |
| P2 | 数据兼容性：旧 provider_type 映射 | `internal/provider/catalog.go` |
