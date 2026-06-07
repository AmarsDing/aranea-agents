# Tools 工具模块 — 实现设计文档

> 对应需求：`23 tools.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

工具注册与管理：CallableTool/StreamableTool/MCP Tool 统一目录、Agent 工具绑定、运行时挂载。工具是 Agent 可调用的具体外部能力，与 Plugin（运行时拦截器）和 Skill（面向 Agent 的能力+知识包）有明确边界。

核心能力：
- 工具目录 CRUD（含内置工具 + MCP 工具 + 外部工具）
- 工具启用/停用/风险等级管理
- Agent 工具绑定与生效矩阵
- 工具调用记录（ToolInvocation）查询
- 运行时工具挂载（trpc-agent-go Tool/ToolSet 适配）

---

## 二、Proto 层

### 2.1 文件路径

`api/kratos/tool/v1/tool.proto`

### 2.2 完整 Proto 定义

```protobuf
syntax = "proto3";

package kratos.tool.v1;

import "google/api/annotations.proto";
import "google/api/field_behavior.proto";
import "google/protobuf/empty.proto";

option go_package = "aranea-agents/api/kratos/tool/v1;v1";

message ToolPermissions {
  bool can_manage = 1;
}

message Tool {
  string id = 1;
  string key = 2;
  string display_name = 3;
  string description = 4;
  string category = 5;
  string source = 6;                    // "builtin"/"mcp"/"system"/"external"
  string risk_level = 7;                // "low"/"medium"/"high"/"critical"
  bool enabled = 8;
  bool readonly = 9;
  bool requires_confirmation = 10;
  bool supports_streaming = 11;
  bool supports_concurrency = 12;
  string parameters_schema_json = 13;   // JSON Schema 输入参数
  string result_schema_json = 14;       // JSON Schema 输出结果
  string config_schema_json = 15;       // JSON Schema 配置项
  string config_json = 16;              // 当前生效配置
  string default_config_json = 17;      // 出厂默认配置
  string metadata_json = 18;            // 扩展元数据
  string runtime_status = 19;           // "available"/"catalog_only"/"disabled"
  string runtime_kind = 20;             // "function"/"streaming"/"approval"
  int32 invoke_count = 21;
  int32 invoke_count_24h = 22;
  int32 success_count = 23;
  int32 failure_count = 24;
  int32 blocked_count = 25;
  int32 agent_override_count = 26;
  optional double avg_duration_ms = 27;
  string last_invoked_at = 28;
  string last_status = 29;              // "success"/"error"/"blocked"
  string created_at = 30;
  string updated_at = 31;
  ToolPermissions permissions = 32;
}

message ToolSummary {
  int32 total_tools = 1;
  int32 enabled_tools = 2;
  int32 high_risk_enabled = 3;
  int32 calls_24h = 4;
  double failure_rate_24h = 5;
}

message ListToolsRequest {
  string search = 1;
  string category = 2;
  string source = 3;
  string risk_level = 4;
  string enabled = 5;                   // ""/"true"/"false" 三态
  int32 page = 6;
  int32 page_size = 7;
}

message ListToolsResponse {
  repeated Tool items = 1;
  int32 total = 2;
  int32 page = 3;
  int32 page_size = 4;
  ToolSummary summary = 5;
}

message GetToolRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
}

message CreateToolRequest {
  string key = 1 [(google.api.field_behavior) = REQUIRED];
  string display_name = 2 [(google.api.field_behavior) = REQUIRED];
  string description = 3;
  string category = 4;
  string source = 5;
  string risk_level = 6;
  bool enabled = 7;
  bool readonly = 8;
  bool requires_confirmation = 9;
  bool supports_streaming = 10;
  bool supports_concurrency = 11;
  string parameters_schema_json = 12;
  string result_schema_json = 13;
  string config_schema_json = 14;
  string config_json = 15;
  string default_config_json = 16;
  string metadata_json = 17;
}

message UpdateToolRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
  string key = 2 [(google.api.field_behavior) = REQUIRED];
  string display_name = 3 [(google.api.field_behavior) = REQUIRED];
  string description = 4;
  string category = 5;
  string source = 6;
  string risk_level = 7;
  bool enabled = 8;
  bool readonly = 9;
  bool requires_confirmation = 10;
  bool supports_streaming = 11;
  bool supports_concurrency = 12;
  string parameters_schema_json = 13;
  string result_schema_json = 14;
  string config_schema_json = 15;
  string config_json = 16;
  string default_config_json = 17;
  string metadata_json = 18;
}

message DeleteToolRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
}

message ToggleToolEnabledRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
  bool enabled = 2;
}

message ToolInvocation {
  string id = 1;
  string request_id = 2;
  string invocation_id = 3;
  string tool_id = 4;
  string tool_key = 5;
  string tool_display_name = 6;
  string agent_id = 7;
  string agent_key = 8;
  string agent_display_name = 9;
  string session_id = 10;
  string message_id = 11;
  string user_id = 12;
  string source = 13;
  string status = 14;                   // "success"/"error"/"blocked"/"cancelled"
  string started_at = 15;
  string ended_at = 16;
  int32 duration_ms = 17;
  string input_preview = 18;
  string input_hash = 19;
  string output_preview = 20;
  string output_hash = 21;
  string error_code = 22;
  string error_message = 23;
  bool redaction_applied = 24;
  string metadata_json = 25;
  string created_at = 26;
}

message ListToolRunsRequest {
  string tool_key = 1;
  string agent_id = 2;
  string session_id = 3;
  string status = 4;
  string from = 5;
  string to = 6;
  int32 page = 7;
  int32 page_size = 8;
}

message ListToolRunsResponse {
  repeated ToolInvocation items = 1;
  int32 total = 2;
  int32 page = 3;
  int32 page_size = 4;
}

message ListToolRunsForToolRequest {
  string tool_id = 1 [(google.api.field_behavior) = REQUIRED];
  string agent_id = 2;
  string session_id = 3;
  string status = 4;
  string from = 5;
  string to = 6;
  int32 page = 7;
  int32 page_size = 8;
}

message ToolAgentOverride {
  string id = 1;
  string tool_id = 2;
  string tool_key = 3;
  string agent_id = 4;
  bool enabled = 5;
  string mode = 6;                    // "inherit"/"override"/"deny"
  string config_override_json = 7;
  bool requires_confirmation = 8;
  string created_at = 9;
  string updated_at = 10;
}

message ListToolAgentOverridesRequest {
  string tool_key = 1 [(google.api.field_behavior) = REQUIRED];
}

message ListToolAgentOverridesByAgentRequest {
  string agent_id = 1 [(google.api.field_behavior) = REQUIRED];
}

message ListToolAgentOverridesResponse {
  repeated ToolAgentOverride items = 1;
}

message UpsertToolAgentOverrideRequest {
  string tool_key = 1 [(google.api.field_behavior) = REQUIRED];
  string agent_id = 2 [(google.api.field_behavior) = REQUIRED];
  bool enabled = 3;
  string mode = 4;
  string config_override_json = 5;
  bool requires_confirmation = 6;
}

message DeleteToolAgentOverrideRequest {
  string tool_key = 1 [(google.api.field_behavior) = REQUIRED];
  string agent_id = 2 [(google.api.field_behavior) = REQUIRED];
}

message GetToolInvocationParamsRequest {
  string invocation_id = 1 [(google.api.field_behavior) = REQUIRED];
}

message ToolInvocationParam {
  string id = 1;
  string invocation_id = 2;
  string tool_key = 3;
  string params_json = 4;
  bool redaction_applied = 5;
  string created_at = 6;
}

message UpdateToolConfigRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
  string config_json = 2;
}

message TestToolRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
  string arguments_json = 2;
  int32 timeout_sec = 3;
}

message TestToolResponse {
  string status = 1;                   // "success"/"error"/"timeout"
  string result_preview = 2;
  string error_message = 3;
  int32 duration_ms = 4;
}

message ToolInvocationAudit {
  string id = 1;
  string invocation_id = 2;
  string tool_key = 3;
  string agent_id = 4;
  string user_id = 5;
  string session_id = 6;
  string action = 7;
  string result_summary = 8;
  string status = 9;
  string source = 10;
  string created_at = 11;
}

message ListToolInvocationAuditsRequest {
  string tool_key = 1;
  string agent_id = 2;
  string user_id = 3;
  string session_id = 4;
  string status = 5;
  string from = 6;
  string to = 7;
  int32 page = 8;
  int32 page_size = 9;
}

message ListToolInvocationAuditsResponse {
  repeated ToolInvocationAudit items = 1;
  int32 total = 2;
  int32 page = 3;
  int32 page_size = 4;
}

service ToolService {
  rpc ListTools(ListToolsRequest) returns (ListToolsResponse) {
    option (google.api.http) = {get: "/v1/tools"};
  }
  rpc GetTool(GetToolRequest) returns (Tool) {
    option (google.api.http) = {get: "/v1/tools/{id}"};
  }
  rpc CreateTool(CreateToolRequest) returns (Tool) {
    option (google.api.http) = {post: "/v1/tools" body: "*"};
  }
  rpc UpdateTool(UpdateToolRequest) returns (Tool) {
    option (google.api.http) = {put: "/v1/tools/{id}" body: "*"};
  }
  rpc DeleteTool(DeleteToolRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = {delete: "/v1/tools/{id}"};
  }
  rpc ToggleToolEnabled(ToggleToolEnabledRequest) returns (Tool) {
    option (google.api.http) = {patch: "/v1/tools/{id}/enabled" body: "*"};
  }
  rpc ListToolRuns(ListToolRunsRequest) returns (ListToolRunsResponse) {
    option (google.api.http) = {get: "/v1/tools/runs"};
  }
  rpc ListToolRunsForTool(ListToolRunsForToolRequest) returns (ListToolRunsResponse) {
    option (google.api.http) = {get: "/v1/tools/{tool_id}/runs"};
  }
  rpc ListToolAgentOverrides(ListToolAgentOverridesRequest) returns (ListToolAgentOverridesResponse) {
    option (google.api.http) = {get: "/v1/tools/{tool_key}/overrides"};
  }
  rpc ListToolAgentOverridesByAgent(ListToolAgentOverridesByAgentRequest) returns (ListToolAgentOverridesResponse) {
    option (google.api.http) = {get: "/v1/agents/{agent_id}/tool-overrides"};
  }
  rpc UpsertToolAgentOverride(UpsertToolAgentOverrideRequest) returns (ToolAgentOverride) {
    option (google.api.http) = {put: "/v1/tools/{tool_key}/overrides/{agent_id}" body: "*"};
  }
  rpc DeleteToolAgentOverride(DeleteToolAgentOverrideRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = {delete: "/v1/tools/{tool_key}/overrides/{agent_id}"};
  }
  rpc GetToolInvocationParams(GetToolInvocationParamsRequest) returns (ToolInvocationParam) {
    option (google.api.http) = {get: "/v1/tools/invocations/{invocation_id}/params"};
  }
  rpc UpdateToolConfig(UpdateToolConfigRequest) returns (Tool) {
    option (google.api.http) = {put: "/v1/tools/{id}/config" body: "*"};
  }
  rpc TestTool(TestToolRequest) returns (TestToolResponse) {
    option (google.api.http) = {post: "/v1/tools/{id}/test" body: "*"};
  }
  rpc ListToolInvocationAudits(ListToolInvocationAuditsRequest) returns (ListToolInvocationAuditsResponse) {
    option (google.api.http) = {get: "/v1/tools/audits"};
  }
}
```

### 2.3 HTTP API 汇总

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/v1/tools` | 列表查询，支持 search/category/source/risk_level/enabled 筛选，含 Summary |
| GET | `/v1/tools/{id}` | 获取单个工具详情 |
| POST | `/v1/tools` | 创建工具 |
| PUT | `/v1/tools/{id}` | 更新工具 |
| DELETE | `/v1/tools/{id}` | 软删除工具 |
| PATCH | `/v1/tools/{id}/enabled` | 启用/停用工具 |
| GET | `/v1/tools/runs` | 全局工具调用记录查询 |
| GET | `/v1/tools/{tool_id}/runs` | 指定工具的调用记录查询 |
| GET | `/v1/tools/{tool_key}/overrides` | 查询工具的 Agent 覆盖列表 |
| GET | `/v1/agents/{agent_id}/tool-overrides` | 查询 Agent 的工具覆盖列表 |
| PUT | `/v1/tools/{tool_key}/overrides/{agent_id}` | 创建/更新 Agent 工具覆盖 |
| DELETE | `/v1/tools/{tool_key}/overrides/{agent_id}` | 删除 Agent 工具覆盖 |
| GET | `/v1/tools/invocations/{invocation_id}/params` | 查询工具调用脱敏参数 |
| PUT | `/v1/tools/{id}/config` | 更新工具配置 |
| POST | `/v1/tools/{id}/test` | 在线测试工具 |
| GET | `/v1/tools/audits` | 查询工具调用审计日志 |

---

## 三、Biz 层

### 3.1 领域模型

```go
type Tool struct {
    ID                   string
    Key                  string              // 唯一标识，如 "duckduckgo_search"
    DisplayName          string              // 显示名称
    Description          string
    Category             string              // "system"/"filesystem"/"web"/...
    Source               string              // "builtin"/"mcp"/"system"/"external"
    RiskLevel            string              // "low"/"medium"/"high"/"critical"
    Enabled              bool
    Readonly             bool                // 内置工具不可编辑
    RequiresConfirmation bool                // 高风险工具需确认
    SupportsStreaming    bool
    SupportsConcurrency  bool
    ParametersSchemaJSON string              // JSON Schema 输入参数
    ResultSchemaJSON     string              // JSON Schema 输出结果
    ConfigSchemaJSON     string              // JSON Schema 配置项
    ConfigJSON           string              // 当前生效配置
    DefaultConfigJSON    string              // 出厂默认配置
    MetadataJSON         string              // 扩展元数据
    RuntimeStatus        string              // "available"/"catalog_only"/"disabled"
    RuntimeKind          string              // "function"/"streaming"/"approval"
    InvokeCount          int                 // 总调用次数
    InvokeCount24h       int                 // 24h 调用次数
    SuccessCount         int
    FailureCount         int
    BlockedCount         int
    AgentOverrideCount   int                 // Agent 级别覆盖数
    AvgDurationMS        *float64            // 平均耗时（毫秒）
    P95DurationMS        float64             // P95 耗时
    LastInvokedAt        string
    LastStatus           string              // "success"/"error"/"blocked"
    CreatedAt            string
    UpdatedAt            string
    DeletedAt            string
    Permissions          ToolPermissions
}

type ToolPermissions struct {
    CanManage bool
}

type ToolUpsertInput struct {
    ID                   string
    Key                  string
    DisplayName          string
    Description          string
    Category             string
    Source               string
    RiskLevel            string
    Enabled              bool
    Readonly             bool
    RequiresConfirmation bool
    SupportsStreaming    bool
    SupportsConcurrency  bool
    ParametersSchemaJSON string
    ResultSchemaJSON     string
    ConfigSchemaJSON     string
    ConfigJSON           string
    DefaultConfigJSON    string
    MetadataJSON         string
}

type ToolListQuery struct {
    Search    string
    Category  string
    Source    string
    RiskLevel string
    Enabled   string              // ""/"true"/"false" 三态
    Sort      string
    Limit     int
    Offset    int
}

type ToolListResult struct {
    Items   []Tool
    Total   int
    Limit   int
    Offset  int
    Summary ToolSummary
}

type ToolSummary struct {
    TotalTools      int
    EnabledTools    int
    HighRiskEnabled int
    Calls24h        int
    FailureRate24h  float64
}

type ToolInvocation struct {
    ID               string
    RequestID        string
    InvocationID     string
    ToolID           string
    ToolKey          string
    ToolDisplayName  string
    AgentID          string
    AgentKey         string
    AgentDisplayName string
    SessionID        string
    MessageID        string
    UserID           string
    Source           string
    Status           string              // "success"/"error"/"blocked"/"cancelled"
    StartedAt        string
    EndedAt          string
    DurationMS       int
    InputPreview     string
    InputHash        string
    OutputPreview    string
    OutputHash       string
    ErrorCode        string
    ErrorMessage     string
    RedactionApplied bool
    MetadataJSON     string
    CreatedAt        string
}

type ToolInvocationWrite struct {
    ToolKey       string
    AgentID       string
    AgentKey      string
    SessionID     string
    UserID        string
    Status        string
    DurationMS    int
    StartedAt     string
    EndedAt       string
    InputPreview  string
    InputHash     string
    OutputPreview string
    OutputHash    string
    ErrorCode     string
    ErrorMessage  string
    Source        string
    ToolCallID    string
}

type ToolInvocationParam struct {
    ID               string
    InvocationID     string
    ToolKey          string
    ParamsJSON       string
    RedactionApplied bool
    CreatedAt        string
}

type ToolAgentOverride struct {
    ID                   string
    ToolID               string
    ToolKey              string
    AgentID              string
    Enabled              bool
    Mode                 string
    ConfigOverrideJSON   string
    RequiresConfirmation bool
    CreatedAt            string
    UpdatedAt            string
}

type ToolAgentOverrideInput struct {
    ToolKey              string
    AgentID              string
    Enabled              bool
    Mode                 string
    ConfigOverrideJSON   string
    RequiresConfirmation bool
}

type ToolRunQuery struct {
    ToolKey   string
    AgentID   string
    SessionID string
    Status    string
    From      string
    To        string
    HasError  *bool
    Limit     int
    Offset    int
}

type ToolRunResult struct {
    Items  []ToolInvocation
    Total  int
    Limit  int
    Offset int
}
```

### 3.2 Repo 接口

```go
type ToolRepo interface {
    SearchTools(ctx context.Context, q ToolListQuery) (ToolListResult, error)
    GetTool(ctx context.Context, idOrKey string) (Tool, error)
    CreateTool(ctx context.Context, in ToolUpsertInput) (Tool, error)
    UpdateTool(ctx context.Context, idOrKey string, in ToolUpsertInput) (Tool, error)
    DeleteTool(ctx context.Context, idOrKey string) error
    UpdateToolEnabled(ctx context.Context, idOrKey string, enabled bool) (Tool, error)
    UpdateToolConfig(ctx context.Context, idOrKey string, configJSON string) (Tool, error)
    SearchToolInvocations(ctx context.Context, q ToolRunQuery) (ToolRunResult, error)
    RecordToolInvocation(ctx context.Context, in ToolInvocationWrite) error
    SyncBuiltinTools(ctx context.Context) error
    GetToolInvocationParams(ctx context.Context, invocationID string) (ToolInvocationParam, error)
    ListToolAgentOverrides(ctx context.Context, toolKey string) ([]ToolAgentOverride, error)
    UpsertToolAgentOverride(ctx context.Context, in ToolAgentOverrideInput) (ToolAgentOverride, error)
    DeleteToolAgentOverride(ctx context.Context, toolKey string, agentID string) error
}
```

### 3.3 Usecase

```go
type ToolUsecase struct {
    repo ToolRepo
}

func NewToolUsecase(repo ToolRepo) *ToolUsecase

func (u *ToolUsecase) ListTools(ctx context.Context, q ToolListQuery) (ToolListResult, error)
// - 校验分页参数：Limit 默认 20，上限 100，Offset >= 0
// - 调用 repo.SearchTools

func (u *ToolUsecase) GetTool(ctx context.Context, id string) (Tool, error)
// - 校验 id 非空
// - 调用 repo.GetTool（支持 id 或 key 查找）

func (u *ToolUsecase) Create(ctx context.Context, in ToolUpsertInput) (Tool, error)
// - 调用 repo.CreateTool

func (u *ToolUsecase) Update(ctx context.Context, id string, in ToolUpsertInput) (Tool, error)
// - 校验 id 非空
// - 调用 repo.UpdateTool

func (u *ToolUsecase) Delete(ctx context.Context, id string) error
// - 校验 id 非空
// - 调用 repo.DeleteTool（软删除）

func (u *ToolUsecase) ToggleEnabled(ctx context.Context, id string, enabled bool, confirmKey ...string) (Tool, error)
// - 校验 id 非空
// - 高风险工具启用需 confirmKey 匹配 tool.Key

func (u *ToolUsecase) UpdateToolConfig(ctx context.Context, id string, configJSON string) (Tool, error)
// - 校验 id 非空
// - configJSON 为空时默认 "{}"
// - 调用 repo.UpdateToolConfig

func (u *ToolUsecase) ListRuns(ctx context.Context, q ToolRunQuery) (ToolRunResult, error)
// - 校验分页参数
// - 调用 repo.SearchToolInvocations

func (u *ToolUsecase) RecordToolInvocation(ctx context.Context, in ToolInvocationWrite) error
// - 调用 repo.RecordToolInvocation

func (u *ToolUsecase) SyncBuiltinTools(ctx context.Context) error
// - 调用 repo.SyncBuiltinTools

func (u *ToolUsecase) GetToolInvocationParams(ctx context.Context, invocationID string) (ToolInvocationParam, error)
// - 校验 invocationID 非空
// - 调用 repo.GetToolInvocationParams

func (u *ToolUsecase) ListToolAgentOverrides(ctx context.Context, toolKey string) ([]ToolAgentOverride, error)
// - 校验 toolKey 非空

func (u *ToolUsecase) UpsertToolAgentOverride(ctx context.Context, in ToolAgentOverrideInput) (ToolAgentOverride, error)
// - 校验 toolKey、agentID 非空
// - Mode 默认 "inherit"，ConfigOverrideJSON 默认 "{}"

func (u *ToolUsecase) DeleteToolAgentOverride(ctx context.Context, toolKey string, agentID string) error
// - 校验 toolKey、agentID 非空
```

---

## 四、Data 层

### 4.1 Ent Schema

#### PlatformTool（工具目录表）

文件路径：`internal/data/ent/schema/platform_tool.go`

Ent 类型名 `PlatformTool`（避免与 Go 内置 `tool` 冲突），映射数据库表 `tools`。

```go
type PlatformTool struct {
    ent.Schema
}

func (PlatformTool) Annotations() []schema.Annotation {
    return []schema.Annotation{
        entsql.Annotation{Table: "tools"},
    }
}

func (PlatformTool) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").Immutable().Unique().MaxLen(256),
        field.String("tool_key").Unique().MaxLen(512),
        field.String("display_name").MaxLen(1024),
        field.Text("description").Default(""),
        field.String("category").Default("system"),
        field.String("source").Default("builtin"),
        field.String("risk_level").Default("low"),
        field.Bool("enabled").Default(true),
        field.Bool("readonly").Default(false),
        field.Bool("requires_confirmation").Default(false),
        field.Bool("supports_streaming").Default(false),
        field.Bool("supports_concurrency").Default(false),
        field.Text("parameters_schema_json").Default("{}"),
        field.Text("result_schema_json").Default("{}"),
        field.Text("config_schema_json").Default("{}"),
        field.Text("config_json").Default("{}"),
        field.Text("fallback_config_json").StorageKey("default_config_json").Default("{}"),
        field.Text("metadata_json").Default("{}"),
        field.String("created_at").Default(""),
        field.String("updated_at").Default(""),
        field.String("deleted_at").Default(""),
    }
}
```

#### ToolInvocation（工具调用记录表）

文件路径：`internal/data/ent/schema/tool_invocation.go`

映射数据库表 `tool_invocations`。

```go
type ToolInvocation struct {
    ent.Schema
}

func (ToolInvocation) Annotations() []schema.Annotation {
    return []schema.Annotation{
        entsql.Annotation{Table: "tool_invocations"},
    }
}

func (ToolInvocation) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").Immutable().Unique().MaxLen(256),
        field.String("request_id").Default(""),
        field.String("invocation_id").Default(""),
        field.String("tool_id").Default(""),
        field.String("tool_key"),
        field.String("agent_id").Default(""),
        field.String("agent_key").Default(""),
        field.String("session_id").Default(""),
        field.String("message_id").Default(""),
        field.String("user_id").Default(""),
        field.String("source").Default("adk"),
        field.String("status").Default("success"),
        field.String("started_at"),
        field.String("ended_at").Default(""),
        field.Int("duration_ms").Default(0),
        field.Text("input_preview").Default(""),
        field.String("input_hash").Default(""),
        field.Text("output_preview").Default(""),
        field.String("output_hash").Default(""),
        field.String("error_code").Default(""),
        field.Text("error_message").Default(""),
        field.Bool("redaction_applied").Default(true),
        field.Text("metadata_json").Default("{}"),
        field.String("created_at"),
    }
}
```

#### ToolAgentOverride（Agent 工具覆盖表）

映射数据库表 `tool_agent_overrides`。

```go
type ToolAgentOverride struct {
    ent.Schema
}

func (ToolAgentOverride) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").Immutable().Unique().MaxLen(256),
        field.String("tool_id").Default(""),
        field.String("tool_key"),
        field.String("agent_id"),
        field.Bool("enabled").Default(true),
        field.String("mode").Default("inherit"),       // "inherit"/"override"/"deny"
        field.Text("config_override_json").Default("{}"),
        field.Bool("requires_confirmation").Default(false),
        field.String("created_at").Default(""),
        field.String("updated_at").Default(""),
        field.String("deleted_at").Default(""),
    }
}
```

### 4.2 Repo 实现

文件路径：`internal/data/tool.go`

**关键实现特点**：

1. **SearchTools 使用原始 SQL**：因为需要 LEFT JOIN `tool_invocations`（统计聚合）和 `tool_agent_overrides`（Agent 覆盖计数），无法纯 Ent ORM 完成。

```go
type toolRepo struct {
    data *Data
}

func NewToolRepo(d *Data) biz.ToolRepo {
    return &toolRepo{data: d}
}
```

**聚合查询 SQL**：

```go
func toolSelectSQL() string {
    return `
        SELECT t.id, t.tool_key, t.display_name, t.description, t.category, t.source, t.risk_level,
               t.enabled, t.readonly, t.requires_confirmation, t.supports_streaming, t.supports_concurrency,
               t.parameters_schema_json, t.result_schema_json, t.config_schema_json, t.config_json, t.default_config_json, t.metadata_json,
               COALESCE(stats.invoke_count, 0), COALESCE(stats.invoke_count_24h, 0), COALESCE(stats.success_count, 0),
               COALESCE(stats.failure_count, 0), COALESCE(stats.blocked_count, 0), COALESCE(overrides.agent_override_count, 0),
               stats.avg_duration_ms, COALESCE(last.started_at, ''), COALESCE(last.status, ''),
               t.created_at, t.updated_at, t.deleted_at
        FROM tools t
        LEFT JOIN (
            SELECT tool_key,
                   COUNT(1) AS invoke_count,
                   SUM(CASE WHEN started_at >= ? THEN 1 ELSE 0 END) AS invoke_count_24h,
                   SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) AS success_count,
                   SUM(CASE WHEN status = 'error' THEN 1 ELSE 0 END) AS failure_count,
                   SUM(CASE WHEN status = 'blocked' THEN 1 ELSE 0 END) AS blocked_count,
                   AVG(duration_ms) AS avg_duration_ms
            FROM tool_invocations
            GROUP BY tool_key
        ) stats ON stats.tool_key = t.tool_key
        LEFT JOIN (
            SELECT tool_key, COUNT(1) AS agent_override_count
            FROM tool_agent_overrides
            WHERE deleted_at = ''
            GROUP BY tool_key
        ) overrides ON overrides.tool_key = t.tool_key
        LEFT JOIN (
            SELECT ti.tool_key, ti.started_at, ti.status
            FROM tool_invocations ti
            INNER JOIN (
                SELECT tool_key, MAX(started_at) AS max_started_at
                FROM tool_invocations
                GROUP BY tool_key
            ) latest ON latest.tool_key = ti.tool_key AND latest.max_started_at = ti.started_at
        ) last ON last.tool_key = t.tool_key`
}
```

**筛选条件构建**：

```go
func toolWhereClause(q biz.ToolListQuery) (string, []any) {
    where := []string{"t.deleted_at = ''"}
    args := []any{}
    if search := strings.TrimSpace(q.Search); search != "" {
        like := "%" + strings.ToLower(search) + "%"
        where = append(where, "(LOWER(t.tool_key) LIKE ? OR LOWER(t.display_name) LIKE ? OR LOWER(t.description) LIKE ?)")
        args = append(args, like, like, like)
    }
    if q.Category != "" { where = append(where, "t.category = ?"); args = append(args, q.Category) }
    if q.Source != ""   { where = append(where, "t.source = ?");   args = append(args, q.Source) }
    if q.RiskLevel != ""{ where = append(where, "t.risk_level = ?"); args = append(args, q.RiskLevel) }
    if q.Enabled == "true" || q.Enabled == "false" {
        where = append(where, "t.enabled = ?"); args = append(args, q.Enabled == "true")
    }
    return strings.Join(where, " AND "), args
}
```

**Summary 计算**：

```go
func (r *toolRepo) computeToolSummary(ctx context.Context, client *ent.Client, q biz.ToolListQuery) (biz.ToolSummary, error) {
    where, args := toolWhereClause(q)
    var s biz.ToolSummary
    // 统计总数、启用数、高风险启用数
    // 统计 24h 调用数和失败率
    // ...
}
```

**CreateTool**：

```go
func (r *toolRepo) CreateTool(ctx context.Context, in biz.ToolUpsertInput) (biz.Tool, error) {
    // 校验 key 非空
    // applyBuiltinToolDefaults：补全默认值（source="builtin", risk_level="low", JSON 默认 "{}"）
    // 生成 ID：优先 "tool_{key}"，否则 uniqueToolID
    // 使用 Ent ORM 插入
    // 返回 r.GetTool(ctx, id)
}
```

**UpdateTool**：

```go
func (r *toolRepo) UpdateTool(ctx context.Context, idOrKey string, in biz.ToolUpsertInput) (biz.Tool, error) {
    // 通过 toolByIDOrKey 查找现有记录
    // applyBuiltinToolDefaults
    // 使用 Ent ORM 更新
    // 返回 r.GetTool(ctx, key)
}
```

**DeleteTool**（软删除）：

```go
func (r *toolRepo) DeleteTool(ctx context.Context, idOrKey string) error {
    // 通过 toolByIDOrKey 查找
    // 设置 deleted_at = nowRFC3339()
}
```

**RecordToolInvocation**（工具调用记录写入）：

```go
func (r *toolRepo) RecordToolInvocation(ctx context.Context, in biz.ToolInvocationWrite) error
// - 生成 ID
// - input_preview/output_preview 截断至 2000 字符
// - 写入 tool_invocations 表
```

**SearchToolInvocations**（工具调用记录查询）：

```go
func (r *toolRepo) SearchToolInvocations(ctx context.Context, q biz.ToolRunQuery) (biz.ToolRunResult, error) {
    // 原始 SQL 查询 tool_invocations 表
    // LEFT JOIN tools（获取 display_name）和 agents（获取 agent display_name）
    // 支持 tool_key/agent_id/session_id/status/from/to/has_error 筛选
    // ORDER BY started_at DESC, created_at DESC
}
```

### 4.3 内置工具种子

文件路径：`internal/data/builtin_tools_seed.go`

**核心机制**：

1. `builtinPlatformToolSeeds` 定义所有内置工具的初始数据（key、displayName、description、category、riskLevel、enabled、paramsSchema、registryName）
2. `ensureBuiltinPlatformTools` 在数据库初始化时执行 `INSERT ... ON CONFLICT(tool_key) DO NOTHING`
3. `syncBuiltinToolsFromRegistry` 从 `tools.Registry()` 同步 risk_level、requires_confirmation、supports_streaming、supports_concurrency 到 tools 表

**关键约束**：`registryName` 字段将 seed 行与 `tools.Registry()` 中的 `ToolRegistration` 关联，确保 seed 的 tool_key 与框架工具的 `Declaration().Name` 一致。

---

## 五、Service 层

文件路径：`internal/service/tool.go`

```go
type ToolService struct {
    v1.UnimplementedToolServiceServer
    uc *biz.ToolUsecase
}

func NewToolService(uc *biz.ToolUsecase) *ToolService
```

**Biz → Proto 转换函数**：

- `bizToolToProto(t biz.Tool) *v1.Tool`：转换工具模型，注意 `AvgDurationMS` 是 `*float64` → `optional double`
- `bizSummaryToProto(s biz.ToolSummary) *v1.ToolSummary`
- `bizInvocationToProto(x biz.ToolInvocation) *v1.ToolInvocation`

**RPC 方法实现**：

| 方法 | 说明 |
|------|------|
| `ListTools` | 分页查询 + Summary，使用 `biz.PageToLimitOffset` |
| `GetTool` | 按 ID 查找，sql.ErrNoRows → NotFound |
| `CreateTool` | CreateToolRequest → ToolUpsertInput → uc.Create |
| `UpdateTool` | UpdateToolRequest → ToolUpsertInput → uc.Update |
| `DeleteTool` | uc.Delete，sql.ErrNoRows → NotFound |
| `ToggleToolEnabled` | uc.ToggleEnabled |
| `ListToolRuns` | 全局调用记录查询 |
| `ListToolRunsForTool` | 指定工具的调用记录查询 |

---

## 六、Wire 注入

```
data.ProviderSet  → NewToolRepo
biz.ProviderSet   → NewToolUsecase
service.ProviderSet → NewToolService
```

---

## 七、运行时层

### 7.1 整体架构

```
Agent 构建请求
  → BuildTRPCLLMAgent(ctx, ag, deps)
    → loadEffectiveToolKeys(ctx, deps, agentID)  // 计算生效工具
    → buildToolsetsForAgent(ctx, ag, deps)        // 装配工具集
      → tooltrpc.BuildToolsets(ctx, cfg)          // 桥接层
        → tools.Assemble(ctx, assemblyCfg)        // 核心装配
    → llmagent.WithToolSets(ts.ToolSets)           // 注入 ToolSet
    → llmagent.WithTools(ts.Tools)                 // 注入 Tool
    → llmagent.WithToolFilter(filter)              // 注入过滤
    → llmagent.WithToolCallbacks(callbacks)        // 注入回调
    → llmagent.WithToolCallRetryPolicy(policy)     // 注入重试策略
    → llmagent.WithEnableParallelTools(true)       // 注入并行执行
```

### 7.2 工具注册表

文件路径：`internal/tools/toolset.go`

**核心类型**：

```go
type ToolRegistration struct {
    Name                 string
    Description          string
    Category             string
    Factory              func(ctx context.Context) (Tool, error)       // 单工具工厂
    ToolSetFactory       func(ctx context.Context) (ToolSet, error)    // 工具集工厂
    EnabledByDefault     bool
    RiskLevel            string
    RequiresConfirmation bool
    SupportsStreaming    bool
    SupportsConcurrency  bool
}

type AssemblyConfig struct {
    EnabledTools  []string
    FilesystemDir string
    ShellExecDir  string   // Phase 5：hostexec WithBaseDir；默认同 FilesystemDir / workspace_root
    GeminiModel   string
    GoogleAPIKey  string
    GoogleCX      string
    ClaudeCodeDir string
    OpenAPISpecs  []OpenAPISpecConfig
    AgentTools    []AgentToolConfig
    MCPServers    []MCPServerConfig
    MCPBroker     *MCPBrokerConfig
    CustomTools   []Tool
}

type AssembledToolsets struct {
    ToolSets []ToolSet
    Tools    []Tool
}
```

**Registry() 注册的工具**：

| 注册名 | 分类 | 类型 | 风险 | 默认启用 | 框架包 |
|--------|------|------|------|----------|--------|
| `file` | filesystem | ToolSet | low | ✅ | `trpc-agent-go/tool/file` |
| `hostexec` | execution | ToolSet | critical | ❌ | `trpc-agent-go/tool/hostexec` |
| `httpfetch` | web | Tool | medium | ❌ | `trpc-agent-go/tool/webfetch/httpfetch` |
| `claudefetch` | web | Tool | medium | ❌ | 框架存根 |
| `geminifetch` | web | Tool | medium | ❌ | `trpc-agent-go/tool/webfetch/geminifetch` |
| `duckduckgo` | search | Tool | medium | ❌ | `trpc-agent-go/tool/duckduckgo` |
| `google_search` | search | ToolSet | medium | ❌ | `trpc-agent-go/tool/google/search` |
| `arxiv_search` | search | ToolSet | low | ❌ | `trpc-agent-go/tool/arxivsearch` |
| `wikipedia` | search | ToolSet | low | ❌ | `trpc-agent-go/tool/wikipedia` |
| `email` | communication | ToolSet | high | ❌ | `trpc-agent-go/tool/email` |
| `todo` | productivity | Tool | low | ❌ | `trpc-agent-go/tool/todo` |
| `await_user_reply` | interaction | Tool | low | ❌ | `trpc-agent-go/tool/awaitreply` |
| `claudecode` | coding | ToolSet | critical | ❌ | `trpc-agent-go/tool/claudecode` |
| `workspace_exec` | execution | Tool | critical | ❌ | `trpc-agent-go/tool/workspaceexec` |
| `openapi` | integration | ToolSet | medium | ❌ | `trpc-agent-go/tool/openapi` |
| `agent` | composition | — | medium | ❌ | `trpc-agent-go/tool/agent` |
| `mcp` | integration | — | medium | ❌ | `trpc-agent-go/tool/mcp` |
| `mcpbroker` | integration | — | medium | ❌ | `trpc-agent-go/tool/mcpbroker` |
| `browser` | browser | ToolSet | critical | ❌ | `trpc-agent-go/tool/browser`（Playwright MCP 桥接） |
| `read_spreadsheet` | media | Tool | medium | ❌ | `trpc-agent-go/tool/readspreadsheet` |
| `read_tool_result` | system | Tool | low | ❌ | `Deferred: true`；延迟工具结果读取 |
| `model_registry_sync` | system | — | medium | ❌ | 仅元数据，无 Factory |
| `working_memory` | memory | ToolSet | low | ❌ | `trpc-agent-go/tool/workingmemory` |
| `message` | messaging | ToolSet | medium | ❌ | 统一消息发送（OutboundRouter） |
| `subagents_spawn` | composition | — | medium | ❌ | 仅元数据，运行时通过 SubAgentService 注入 |
| `subagents_list` | composition | — | low | ❌ | 仅元数据 |
| `subagents_get` | composition | — | low | ❌ | 仅元数据 |
| `subagents_cancel` | composition | — | medium | ❌ | 仅元数据 |

**Assemble 流程**：

1. 遍历 `Registry()`，按 `EnabledTools` 列表过滤
2. 对每个注册项，调用 `Factory` 或 `ToolSetFactory` 创建工具实例
3. 对需要额外配置的工具（file、geminifetch、google_search、claudecode），用配置覆盖默认实例
4. 处理 OpenAPI spec → `openapi.NewToolSet`
5. 处理 workspace_exec → 额外注入 `write_stdin`、`kill_session` 工具
6. 处理 AgentTool → `agent.NewTool`
7. 处理 MCP Server → `mcp.NewMCPToolSet`
8. 处理 MCP Broker → `mcpbroker.New`
9. 追加 CustomTools

### 7.3 桥接层

文件路径：`internal/tools/trpc/toolsets.go`

`ToolsetConfig` → `AssemblyConfig` 的适配层，供 `trpc_build.go` 调用：

```go
type ToolsetConfig struct {
    Filesystem      bool
    FilesystemDir   string
    ShellExec       bool
    WebFetch        bool
    WebSearch       bool
    GeminiFetch     bool
    GeminiModel     string
    GoogleSearch    bool
    GoogleAPIKey    string
    GoogleCX        string
    ArxivSearch     bool
    Wikipedia       bool
    Email           bool
    Todo            bool
    AwaitReply      bool
    AwaitHook       ReplyFunc          // 阻塞式等待回调
    ClaudeCode      bool
    ClaudeCodeDir   string
    OpenAPISpecs    []OpenAPISpecConfig
    WorkspaceExec   bool
    AgentTools      []AgentToolConfig
    MCPServers      []MCPServerConfig
    MCPBroker       *MCPBrokerConfig
    CustomTools     []trpctool.Tool
    KnowledgeReflect bool
    CallAgent        bool
    Browser          *browser.PlaywrightMCPConfig
    SubAgentService  SubAgentServiceProvider
    OutboundRouter   OutboundRouterProvider
    BlobReader       BlobReaderProvider
    DeferredTools    []DeferredToolEntry
    KnowledgeSearch bool
    CallAgent       bool
}
func BuildToolsets(ctx context.Context, cfg ToolsetConfig) (*AssembledToolsets, error)
```

**关键映射**：`ToolsetConfig` 的布尔字段映射到 `AssemblyConfig.EnabledTools` 列表中的注册名。

**特殊处理**：
- `AwaitReply + AwaitHook != nil` → 使用 `serviceawaitreply.New()` 替代框架内置工具
- `KnowledgeSearch` → `knowledgepkg.NewSearchTool()` 追加到 CustomTools
- `CallAgent` → `a2a.NewCallAgentTool()` 追加到 CustomTools

### 7.4 Effective Tools 计算

文件路径：`internal/biz/agent_effective_tools.go`

**核心流程**：

```
AgentRuntimeSettings
  → profileAllowSet(profile)           // Profile 预设展开
  → computePolicyAllowedSet(...)        // 合并 allow 列表
  → computePolicyDenySet(...)           // 展开 deny 列表
  → 遍历 catalog
    → computeEffectiveToolState(...)    // 逐工具计算
  → 合成工具（shell_exec、duckduckgo_search、web_fetch 在 catalog 缺失时合成）
```

**Profile 预设**：

| Profile | 允许的工具 |
|---------|-----------|
| `chat_only` | 无 |
| `read_only` | datetime + filesystem(读) + todo |
| `coding` | filesystem + web + skill + session + datetime |
| `research` | web(搜索+抓取) + filesystem(读) + skill + memory + todo + datetime |
| `full` | 全部分组 |

**计算语义**：
- `tools.enabled=true`（catalog 行）= "默认开放"：profile 门控通过且不在 deny 列表即可用
- `tools.enabled=false` = "仅显式允许"：必须在 allow 列表中才可用
- Deny 列表和 `ToolsEnabled=false` 全局开关覆盖一切

### 7.5 工具注入与回调

文件路径：`internal/agent/trpc_build.go`

**buildToolsetsForAgent**：

```go
func buildToolsetsForAgent(ctx context.Context, ag biz.Agent, deps TRPCBuilderDeps) (*tooltrpc.AssembledToolsets, error)
```

从 `loadEffectiveToolKeys` 获取生效工具 key 集合，映射到 `ToolsetConfig` 的布尔字段，调用 `BuildToolsets`。

**buildToolCallbacks**：

```go
func buildToolCallbacks(s *biz.AgentRuntimeSettings, ag biz.Agent, deps TRPCBuilderDeps) *trpctool.Callbacks
```

注册 `AfterTool` 回调，在工具执行完成后异步记录调用：

```go
callbacks.RegisterAfterTool(func(ctx context.Context, args *trpctool.AfterToolArgs) (*trpctool.AfterToolResult, error) {
    recordToolInvocationAsync(ctx, args, ag, deps)
    return &trpctool.AfterToolResult{}, nil
})
```

**recordToolInvocationAsync**：

- 从 `trpcagent.InvocationFromContext(ctx)` 提取 sessionID、userID、agentKey
- `previewFromArgs` / `previewFromResult` 截断至 2000 字符
- 使用 `safego.Go` 异步写入 `ToolInvocationWrite`

**buildToolFilter**：

```go
func buildToolFilter(s *biz.AgentRuntimeSettings) trpctool.FilterFunc
```

从 `ToolsDenyJSON` 构建 `tool.NewExcludeToolNamesFilter`，注入到 `llmagent.WithToolFilter`。

**buildToolRetryPolicy**：

```go
func buildToolRetryPolicy(s *biz.AgentRuntimeSettings) *trpctool.RetryPolicy
```

从 `AgentRuntimeSettings` 读取重试配置（MaxAttempts、InitialInterval、BackoffFactor、MaxInterval、Jitter），注入到 `llmagent.WithToolCallRetryPolicy`。

### 7.6 Memory 工具注入

```go
// EP-RT-05: Memory tools 仅在 HasMemory=true 时注入
if ag.Settings.MemoryEnabled && deps.HasMemory {
    opts = append(opts, trpcllmagent.WithTools(memorytool.DefaultTools()))
}
```

### 7.7 目录结构

```
internal/tools/
├── tool.go                          — 项目级工具类型别名（Tool/CallableTool/StreamableTool/ToolSet/Declaration/Schema）
├── toolset.go                       — Registry() + AssemblyConfig + Assemble()
├── doc.go                           — 包文档
├── trpc/
│   └── toolsets.go                  — ToolsetConfig → AssemblyConfig 桥接层
├── custom/                          — 自定义工具实现
├── knowledge/                       — Knowledge 搜索工具
├── serviceawaitreply/               — 阻塞式等待用户回复工具
├── memory/                          — Memory 工具
├── mcpmount/                        — MCP 服务器发现与 ToolSet 装配
├── skillrouter/                     — Skill 检测与分类
└── skillruntime/                    — Skill 工具集解析
├── outbound/                 — 统一消息发送工具
│   └── message.go            — NewMessageTool(OutboundRouter)
├── deferred/                 — 延迟工具加载机制
│   └── deferred.go           — DeferredToolEntry + NewToolSearchTool + NewDeferredCallableTool
```

### 7.8 工具工作区统一（Phase 5）

**目标**：Cursor 式单一项目根——需要「目录」的运行时工具共用 `workspace_root`；装配层一次解析、多处注入。

#### 7.8.1 解析链

文件路径：`internal/agent/tool_assembly.go`（`resolveToolWorkspaceRoot` ✅）

```text
Tool / Override config: filesystem_dir | base_dir | working_dir | root_dir
  → system_settings.root_directory
  → env ARANEA_WORKSPACE_ROOT | WORKSPACE_ROOT
  → {root}/workspace/{agent_key}
  → mkdir 校验
= workspace_root
```

`buildToolsetsForAgent` 在 desktop 联调与将来 Electron 打包下语义相同：均解析为 **本机绝对路径**。

#### 7.8.2 工具 × 目录矩阵

| 注册名 | Catalog / 运行时 | 需要工作区？ | 现状 |
|--------|------------------|-------------|------|
| `file` | `read_file` … `patch_file` | ✅ 严格 | `FilesystemDir` = `workspace_root` ✅ |
| `hostexec` | `shell_exec` → `exec_command` | ✅ 默认 cwd | `ShellExecDir` = `workspace_root` ✅ |
| `claudecode` | `claude_code` | ✅ 可选独立 | 默认 `workspace_root`；可 Override ✅ |
| `workspace_exec` | `workspace_exec` | ✅ CodeExecutor | 仅 `WithCodeExecutor` 路径 ✅ |
| Skill `CodeExecutor` | Skill 脚本执行 | ✅ Skill 根 | `buildSkillDeps` 独立 `rootDir`；与 agent workspace 可不同 |
| `httpfetch` / 搜索类 | web / search | ❌ | — |
| `email` / `todo` / `await_user_reply` | — | ❌ | — |
| `mcp` / `mcpbroker` / `openapi` | integration | ❌ | 远端/契约 |
| Memory / `knowledge_search` | memory | ❌ | 后端存储 |
| `agent` / `call_agent` | composition | ❌ | — |

#### 7.8.3 装配改动

| 文件 | 改动 |
|------|------|
| `internal/tools/toolset.go` | `enabled["hostexec"] && ShellExecDir != ""` → `hostexec.NewToolSet(WithBaseDir(ShellExecDir))` |
| `internal/tools/trpc/toolsets.go` | `ToolsetConfig.ShellExecDir`；Build 时写入 `AssemblyConfig` |
| `internal/agent/tool_assembly.go` | 解析 `workspace_root` → 同时赋 `FilesystemDir`、`ShellExecDir`；`ClaudeCodeDir` 空则回退 |
| `internal/tools/trpc/runtime_config.go` | `shell_exec` config 支持 `base_dir` / `shell_root` |
| `internal/data/builtin_tools_seed.go` | `shell_exec` 参数改为 `workdir`（与 hostexec 一致） |
| `internal/tools/testexec/config.go` | 在线测试 shell 时传入 workspace |

#### 7.8.4 Shell 参数与确认

| 项 | 设计 |
|----|------|
| 调用参数 | `command`（必填）、`workdir`（可选，相对 `workspace_root`） |
| 兼容 | Tool 层将 `working_dir` 映射为 `workdir` |
| 确认门控 | `tool_confirm_gate` 同时匹配 `shell_exec` 与 `exec_command` |
| Prompt | `RuntimeCapabilityCue` 表述默认 cwd=工作区，删除与错误实现绑定的 sandbox 文案 |

#### 7.8.5 与 Electron / App 打包

App 壳、Electron 打包 **不在本模块范围**（曾起草编号 53 文档，**不实施**）。工作区路径仍通过系统设置 / 环境变量 / Tool 配置注入，与是否 Electron 无关。

### 7.9 延迟工具机制（Deferred Tools）

部分工具（如 `read_tool_result`）标记为 `Deferred: true`，不随 Agent 初始化时立即装配，而是通过延迟加载机制按需实例化。

**核心流程**：

```
Assemble()
  → 识别 Deferred: true 的注册项
  → 构建 DeferredToolEntry 列表
  → 创建 ToolSearchTool（搜索可用延迟工具）
  → 创建 DeferredCallableTool（按需实例化并调用）
  → 追加到 AssembledToolsets.Tools
```

### 7.10 子代理工具（SubAgent Tools）

子代理工具通过 `SubAgentService` 注入，支持运行时动态生成、列表、查询和取消子代理。

**注入路径**：

```
AssemblyConfig.SubAgentService
  → SubAgentService.FrameworkTools()
  → 按 enabled 列表过滤
  → 追加到 AssembledToolsets.Tools
```

**4 个子代理工具**：`subagents_spawn`、`subagents_list`、`subagents_get`、`subagents_cancel`。

---

## 八、Web 前端设计

### 8.1 文件结构

```
web/src/
├── features/tools/
│   ├── api.ts                        # API 调用封装
│   └── types.ts                      # TypeScript 类型定义
└── pages/
    ├── ToolsPage.vue                  # 工具目录管理页
    └── ToolRunsPage.vue              # 工具调用记录页
```

### 8.2 TypeScript 类型定义

文件路径：`web/src/features/tools/types.ts`

```typescript
export type ToolPermissions = {
  can_manage: boolean;
};

export type Tool = {
  id: string;
  key: string;
  display_name: string;
  description: string;
  category: string;
  source: "builtin" | "mcp" | "system" | "external" | string;
  risk_level: "low" | "medium" | "high" | "critical" | string;
  enabled: boolean;
  readonly: boolean;
  requires_confirmation: boolean;
  supports_streaming: boolean;
  supports_concurrency: boolean;
  parameters_schema_json: string;
  result_schema_json: string;
  config_schema_json: string;
  config_json: string;
  default_config_json: string;
  metadata_json: string;
  runtime_status?: "available" | "catalog_only" | "disabled" | string;
  runtime_kind?: "function" | "streaming" | "approval" | string;
  invoke_count: number;
  invoke_count_24h: number;
  success_count: number;
  failure_count: number;
  blocked_count: number;
  agent_override_count: number;
  avg_duration_ms: number | null;
  last_invoked_at?: string;
  last_status?: string;
  created_at: string;
  updated_at: string;
  permissions: ToolPermissions;
};

export type ToolUpsertInput = {
  key: string;
  display_name: string;
  description: string;
  category: string;
  source: string;
  risk_level: string;
  enabled: boolean;
  readonly: boolean;
  requires_confirmation: boolean;
  supports_streaming: boolean;
  supports_concurrency: boolean;
  parameters_schema_json: string;
  result_schema_json: string;
  config_schema_json: string;
  config_json: string;
  default_config_json: string;
  metadata_json: string;
};

export type ToolSummary = {
  total_tools: number;
  enabled_tools: number;
  high_risk_enabled: number;
  calls_24h: number;
  failure_rate_24h: number;
};

export type ToolListQuery = {
  search?: string;
  category?: string;
  source?: string;
  risk_level?: string;
  enabled?: boolean | null;
  page?: number;
  page_size?: number;
};

export type ToolListResponse = {
  items: Tool[];
  page: number;
  page_size: number;
  total: number;
  summary: ToolSummary;
};

export type ToolInvocation = {
  id: string;
  request_id: string;
  invocation_id: string;
  tool_id: string;
  tool_key: string;
  tool_display_name: string;
  agent_id: string;
  agent_key: string;
  agent_display_name: string;
  session_id: string;
  message_id: string;
  user_id: string;
  source: string;
  status: "success" | "error" | "blocked" | "cancelled" | string;
  started_at: string;
  ended_at: string;
  duration_ms: number;
  input_preview: string;
  input_hash: string;
  output_preview: string;
  output_hash: string;
  error_code: string;
  error_message: string;
  redaction_applied: boolean;
  metadata_json: string;
  created_at: string;
};

export type ToolRunQuery = {
  tool_key?: string;
  agent_id?: string;
  session_id?: string;
  status?: string;
  from?: string;
  to?: string;
  page?: number;
  page_size?: number;
};

export type PaginatedResponse<T> = {
  items: T[];
  page: number;
  page_size: number;
  total: number;
};

export type AgentEffectiveTool = {
  tool_key: string;
  display_name: string;
  category: string;
  source: string;
  enabled: boolean;
  effective_state: "allowed" | "denied" | string;
  reason: string;
};

export type AgentEffectiveTools = {
  tools_enabled: boolean;
  profile: string;
  allow: string[];
  deny: string[];
  items: AgentEffectiveTool[];
};
```

### 8.3 API 封装

文件路径：`web/src/features/tools/api.ts`

```typescript
export async function listTools(query: ToolListQuery = {}): Promise<ToolListResponse>
export async function getTool(id: string): Promise<Tool>
export async function createTool(input: ToolUpsertInput): Promise<Tool>
export async function updateTool(id: string, input: ToolUpsertInput): Promise<Tool>
export async function deleteTool(id: string): Promise<void>
export async function toggleToolEnabled(id: string, enabled: boolean): Promise<Tool>
export async function listToolRunsForTool(id: string, query: ToolRunQuery = {}): Promise<PaginatedResponse<ToolInvocation>>
export async function listToolRuns(query: ToolRunQuery = {}): Promise<PaginatedResponse<ToolInvocation>>
export async function getAgentEffectiveTools(agentId: string): Promise<AgentEffectiveTools>
```

### 8.4 页面组件

#### ToolsPage.vue

文件路径：`web/src/pages/ToolsPage.vue`

**页面结构**：

1. **顶部 Hero 区域**：标题 "工具管理" + Summary 统计卡片 + 刷新/新建按钮
2. **筛选卡片**：搜索框 + Category 下拉 + Source 下拉 + Risk Level 下拉 + 启用状态筛选
3. **数据表格**：展示工具列表
4. **新建/编辑弹窗**（`ToolEditorDialog` + `ToolEditorForm`）：4 Tab — 基础 / 运行策略 / 参数与配置 / 高级；文案 `features/tools/toolEditorCopy.ts`；子组件 `components/tools/editor/*`
5. **详情弹窗**（`ToolDetailContent`）：5 Tab — 概览 / 参数 / 配置（可编辑，`PUT /v1/tools/{id}/config`）/ Agent / 调用；Agent Tab 含 **生效摘要**（§5.6 UX-02）

~~6. **配置弹窗**~~：已合并至详情「配置」Tab 与编辑弹窗「参数与配置」Tab。

**§5.6 后续 UX**（chip 语义、Schema 边界、保存引导等）见 [23-tools.md §5.6](./23%20tools.md#56-后续迭代业务评审-2026-05-28)。

**Summary 统计卡片**：
- 总工具数 / 已启用数 / 高风险启用数
- 24h 调用数 / 24h 失败率

**表格列定义**：

| 列名 | 字段 | 说明 |
|------|------|------|
| 工具 | key + display_name | 唯一标识 + 显示名称 |
| 说明 | description | 工具描述 |
| 类型/来源 | category + source | Category Chip + Source Chip |
| 风险 | risk_level | 颜色区分（low=绿/medium=橙/high=红/critical=深红） |
| 确认 | requires_confirmation | 是否需要确认标记 |
| 流式 | supports_streaming | 是否支持流式 |
| 调用 | invoke_count + invoke_count_24h | 总调用 / 24h 调用 |
| 成功率 | success_count / (success+failure+blocked) | 成功率百分比 |
| 平均耗时 | avg_duration_ms | 毫秒 |
| 启用 | enabled | Toggle 开关 |
| 操作 | id | 查看/编辑/删除/配置按钮 |

#### ToolRunsPage.vue

文件路径：`web/src/pages/ToolRunsPage.vue`

**页面结构**：

1. **顶部 Hero 区域**：标题 "工具调用记录"
2. **筛选卡片**：Tool Key + Agent ID + Session ID + Status + 时间范围
3. **数据表格**：展示调用记录列表

**表格列定义**：

| 列名 | 字段 | 说明 |
|------|------|------|
| 工具 | tool_key + tool_display_name | |
| Agent | agent_key + agent_display_name | |
| Session | session_id | |
| 状态 | status | success=绿/error=红/blocked=橙/cancelled=灰 |
| 开始时间 | started_at | |
| 耗时 | duration_ms | 毫秒 |
| 输入摘要 | input_preview | 截断展示 |
| 输出摘要 | output_preview | 截断展示 |
| 错误 | error_code + error_message | error 状态时展示 |
| 脱敏 | redaction_applied | 是否已脱敏 |

### 8.5 路由

| 路径 | 组件 | 说明 |
|------|------|------|
| `/tools` | ToolsPage | 工具目录管理页 |
| `/tools/runs` | ToolRunsPage | 工具调用记录页 |

---

## 九、Stream 流式工具机制设计

### 9.1 框架核心接口

trpc-agent-go 的流式工具体系基于以下核心类型：

```go
// tool.Tool 三层接口
type Tool interface { Declaration() *Declaration }
type CallableTool interface { Call(ctx, jsonArgs) (any, error); Tool }
type StreamableTool interface { StreamableCall(ctx, jsonArgs) (*StreamReader, error); Tool }

// tool.Stream 双向流
type Stream struct {
    Reader *StreamReader  // 消费端
    Writer *StreamWriter  // 生产端
}

// tool.StreamChunk 流式数据单元
type StreamChunk struct {
    Content  any      `json:"content"`
    Metadata Metadata `json:"metadata,omitempty"`
}

// tool.FinalResultChunk 最终结果标记
type FinalResultChunk struct { Result any }

// tool.FinalResultStateChunk 最终结果 + 状态增量
type FinalResultStateChunk struct {
    Result     any
    StateDelta map[string][]byte
}
```

### 9.2 Stream 内部实现

```go
// 底层基于 channel 的泛型流
type stream[T any] struct {
    items  chan streamItem[T]   // 缓冲 channel
    closed chan struct{}        // 关闭信号
}

// NewStream(bufferSize) 创建流
// - bufferSize 控制 Send 阻塞前的队列深度
// - closed channel 用于 Reader 主动取消时通知 Writer 停止
```

### 9.3 流式工具执行流程

```
LLM 返回 tool_call
  → 框架检测工具是否实现 StreamableTool
  → 是：调用 StreamableCall(ctx, args) → 返回 StreamReader
     → 循环 Recv() 消费 StreamChunk
     → 遇到 FinalResultChunk → 保留为最终结果
     → 遇到 FinalResultStateChunk → 保留结果 + 发出 StateDelta 事件
     → 遇到 io.EOF → 流结束
     → 非最终 chunk → 转为文本拼接（若无 FinalResultChunk）
  → 否：调用 Call(ctx, args) → 返回同步结果
```

### 9.4 AG-UI 集成

```go
// agui.WithStreamingToolResultActivityEnabled(true)
// 开启后，流式中间结果转为 Activity 事件：
// - ActivityType = "tool.result.stream"
// - ActivityMessageID = "tool-result-activity-" + toolCallID
// 工具结束时仍发一条 TOOL_CALL_RESULT
```

### 9.5 项目集成设计

**ToolsetConfig 扩展**（✅ 已实现）：

```go
type ToolsetConfig struct {
    // ... 已有字段
    StreamingEnabled bool  // 全局流式工具开关
}
```

**tool_invocations 扩展**（✅ 已实现）：

```sql
ALTER TABLE tool_invocations ADD COLUMN streaming INTEGER NOT NULL DEFAULT 0;
ALTER TABLE tool_invocations ADD COLUMN chunk_count INTEGER NOT NULL DEFAULT 0;
```

**WS 推流**：

- 通过 EventBus + `/v1/ws` 推送工具流式中间结果
- 流式工具的中间结果通过 `StreamingToolResultActivityType` 转为 Activity 事件
- 前端可按 `activityType === "tool.result.stream"` 过滤展示实时进度

---

## 十、Memory 记忆工具设计

### 10.1 框架架构

```
memory/
├── memory.go           # Service 接口 + Kind/Metadata/Entry/Key/UserKey
├── tool/tool.go        # 6 个记忆工具（add/search/load/update/delete/clear）
├── extractor/          # 自动提取（LLM 从对话中提取记忆）
├── inmemory/           # 内存后端
├── sqlite/             # SQLite 后端
├── sqlitevec/          # SQLite + 向量搜索
├── postgres/           # PostgreSQL 后端
├── pgvector/           # pgvector 向量搜索
├── mysql/              # MySQL 后端
├── mysqlvec/           # MySQL 向量搜索
├── redis/              # Redis 后端
└── mem0/               # Mem0 平台适配
```

### 10.2 Service 接口

```go
type Service interface {
    AddMemory(ctx, userKey UserKey, memory string, topics []string, ...AddOption) error
    UpdateMemory(ctx, memoryKey Key, memory string, topics []string, ...UpdateOption) error
    DeleteMemory(ctx, memoryKey Key) error
    ClearMemories(ctx, userKey UserKey) error
    ReadMemories(ctx, userKey UserKey, limit int) ([]*Entry, error)
    SearchMemories(ctx, userKey UserKey, query string, ...SearchOption) ([]*Entry, error)
    Tools() []tool.Tool
    EnqueueAutoMemoryJob(ctx, sess *session.Session) error
    Close() error
}
```

### 10.3 工具注入路径

```
AgentRuntimeSettings.MemoryEnabled=true + HasMemory=true
  → memorytool.DefaultTools()
  → llmagent.WithTools(memTools)
  → llmagent.WithMemoryService(service)
```

### 10.4 MemoryConfig 设计

```go
type MemoryConfig struct {
    Backend       string  // "sqlite" / "sqlitevec" / "postgres" / "pgvector" / "mysql" / "redis" / "mem0" / "inmemory"
    ConnectionDSN string
    AutoExtract   bool
    Mode          string  // "off" / "agentic" / "auto" / "both"
}
```

### 10.5 两种记忆模式

**Agentic 模式**：Agent 主动调用 `memory_add` / `memory_search` 等工具

**Auto 模式**：对话结束后，Runner 调用 `service.EnqueueAutoMemoryJob`，LLM 自动提取 fact/episode

---

## 十一、AgentTool 与 MCPBroker 设计

### 11.1 AgentTool 设计

```go
type AgentToolConfig struct {
    Agent             trpcagent.Agent
    Name              string
    Description       string
    SkipSummarization bool
    StreamInner       bool
    HistoryScope      trpcagenttool.HistoryScope
    ResponseMode      trpcagenttool.ResponseMode
}
```

**Assemble 路径**：

```
AssemblyConfig.AgentTools
  → 遍历每个 AgentToolConfig
  → trpcagenttool.NewTool(agent, opts...)
  → 追加到 AssembledToolsets.Tools
```

**关键行为**：

| 选项 | 效果 |
|------|------|
| `SkipSummarization=false` | 子 Agent 输出被摘要后返回 |
| `StreamInner=true` | 子 Agent 的流式事件转发到父级 |
| `ResponseMode=FinalOnly` | 只返回子 Agent 最后一条 assistant 消息 |
| `HistoryScope` | 控制传递给子 Agent 的对话历史范围 |

### 11.2 MCPBroker 设计

```go
type MCPBrokerConfig struct {
    Servers         []MCPServerConfig
    AllowAdHocHTTP  bool
    AdHocTimeoutSec int
}
```

**Assemble 路径**：

```
AssemblyConfig.MCPBroker
  → buildMCPBrokerTools(cfg)
  → trpcmcpbroker.New(opts...)
  → broker.Tools() → 4 个工具
  → 追加到 AssembledToolsets.Tools
```

**4 个 Broker 工具**：

| 工具 | 说明 |
|------|------|
| `mcp_list_servers` | 返回命名服务器列表 |
| `mcp_list_tools` | 连接 MCP 服务器 → ListTools → 返回工具摘要 |
| `mcp_inspect_tools` | 连接 MCP 服务器 → ListTools → 过滤 → 返回 Schema |
| `mcp_call` | 连接 MCP 服务器 → 验证参数 → CallTool → 返回结果 |

**Selector 解析**：

- 命名服务器：`local_stdio_code.add` → server=`local_stdio_code`, tool=`add`
- Ad-hoc HTTP：`https://example.com/mcp.add` → URL=`https://example.com/mcp`, tool=`add`

---

## 十二、工具 Callbacks / Filter / Retry 机制

### 12.1 Callbacks（工具执行生命周期钩子）

```go
type Callbacks struct {
    // BeforeTool: 执行前钩子（跳过执行、修改参数、注入上下文）
    // AfterTool: 执行后钩子（替换结果、跳过摘要）
    // ToolResultMessagesFunc: 自定义工具结果 → 模型消息转换
}

// 项目集成：buildToolCallbacks → AfterTool → recordToolInvocationAsync
callbacks := trpctool.NewCallbacks()
callbacks.RegisterAfterTool(func(ctx context.Context, args *trpctool.AfterToolArgs) (*trpctool.AfterToolResult, error) {
    recordToolInvocationAsync(ctx, args, ag, deps)
    return &trpctool.AfterToolResult{}, nil
})
```

### 12.2 Filter（动态工具可见性控制）

```go
type FilterFunc func(ctx context.Context, tool Tool) bool

// 内置过滤器：
// - tool.NewIncludeToolNamesFilter(names...): 白名单
// - tool.NewExcludeToolNamesFilter(names...): 黑名单

// 项目集成：buildToolFilter → ToolsDenyJSON → ExcludeToolNamesFilter
```

**双层过滤**：
- `WithToolFilter`：控制模型可见的工具列表
- `WithToolExecutionFilter`：控制哪些工具调用可自动执行

### 12.3 Retry（自动重试）

```go
type RetryPolicy struct {
    MaxAttempts     int
    InitialInterval time.Duration
    BackoffFactor   float64
    MaxInterval     time.Duration
    Jitter          bool
    RetryOn         func(ctx, *RetryInfo) (bool, error)
}

// 项目集成：buildToolRetryPolicy → AgentRuntimeSettings → RetryPolicy
```

### 12.4 Merge（结果聚合）

```go
// tool.Merge[T](ts []T) T: 泛型合并函数
// 支持：字符串拼接、数值求和、切片合并、map 合并、struct 逐字段合并
```

---

## 十三、片段级文件编辑（扩展）

> **状态**：✅ 已实现（2026-05-22） | **详设**：[23 tools-fragment-edit.design.md](./23%20tools-fragment-edit.design.md) · **需求**：[23 tools-fragment-edit.md](./23%20tools-fragment-edit.md) · **Review**：[2026-05-22-Tools-Phase4-Fragment-Edit-Review.md](../review/2026-05-22-Tools-Phase4-Fragment-Edit-Review.md) · **changelog**：[Phase 4](../changelog/2026-05-22-Tools-Phase4-Fragment-Edit.md) · [Follow-up](../changelog/2026-05-22-Tools-Phase4-Review-Followup.md)

在 §7 运行时层 `file` ToolSet 上扩展两个 CallableTool：

| 工具 | 职责 |
|------|------|
| `diff_edit` | 多片段 SEARCH/REPLACE，内存原子 apply |
| `patch_file` | unified diff 或结构化 hunk 应用 |

**实现包**：`pkg/trpc-agent-go/tool/file`（非新 Registry 项）。**会话缓存** 经 `internal/toolcache.FileView` 与 `read_file` / 写工具联动，详见扩展设计文档 §4–§6。

本文 §7 注册表与 Assemble 流程**不变**；catalog 种子与 Prompt 集成见开发计划 Phase 4。

---

*文档版本：3.1 — 增加 §十三 片段编辑扩展索引（2026-05-22）。详设见 [23 tools-fragment-edit.design.md](./23%20tools-fragment-edit.design.md)。*


---

## 子模块：Tools Struct 设计

> **对应需求**：[23 tools.md](./23%20tools.md) · **技术设计**：[23 tools.design.md](./23%20tools.design.md) · **开发计划**：[23-tools-development.md](./23-tools-development.md)
> **遵循规范**：`AI-DEVELOPMENT-SPECIFICATION.md`
> **框架**：trpc-agent-go（`trpc.group/trpc-go/trpc-agent-go`）

---

## 一、文档定位与边界

| 维度 | 本文档范围 | 不含 |
|------|-----------|------|
| **焦点** | Go 类型定义、接口签名、目录结构、注册/装配数据流 | 产品需求（见 `23 tools.md`）、API/Proto 设计（见 `23 tools.design.md`）、开发排期（见 `23-tools-development.md`） |
| **深度** | 给出可编译的结构骨架与关键类型；AI 可据此生成实现代码 | 完整业务逻辑实现、前端 UI 设计 |
| **框架** | 以 trpc-agent-go 的 `tool.Tool` / `CallableTool` / `StreamableTool` / `ToolSet` 为核心 | 不自建 tooldef/toolctx 等抽象层；不自建洋葱中间件链 |

---

## 二、框架接口（trpc-agent-go/tool）

项目直接依赖 trpc-agent-go 框架的 tool 包，**不自建** Tool 接口或中间件链。框架提供以下核心接口：

```go
package trpctool // "trpc.group/trpc-go/trpc-agent-go/tool"

// Tool — 基础接口：所有工具的声明入口
type Tool interface {
    Declaration() *Declaration
}

// CallableTool — 可调用工具：Tool + 同步执行
type CallableTool interface {
    Tool
    Call(ctx context.Context, args []byte) (any, error)
}

// StreamableTool — 流式工具：Tool + 流式执行
type StreamableTool interface {
    Tool
    StreamableCall(ctx context.Context, args []byte) (*StreamReader, error)
}

// ToolSet — 工具集：聚合多个 Tool，支持延迟初始化与关闭
type ToolSet interface {
    Name() string
    Tools(ctx context.Context) []Tool
    Close() error
}

// Declaration — 工具声明：暴露给 LLM 的元数据
type Declaration struct {
    Name        string
    Description string
    InputSchema *Schema
}

// Schema — JSON Schema 描述
type Schema struct {
    Type       string
    Properties map[string]*Schema
    Required   []string
    // ... 其他 JSON Schema 字段
}
```

**关键决策**：项目通过 **type alias** 直接复用框架类型，不自建 tooldef / toolctx / middleware / executor 抽象层。横切关注点（校验、重试、追踪、过滤）通过框架内建的 Callbacks / Filter / Retry 机制注入，而非自建洋葱中间件链。

---

## 三、项目级类型定义

### 3.1 类型别名（internal/tools/tool.go）

```go
package tools

import (
    "context"
    trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

type Tool            = trpctool.Tool
type CallableTool    = trpctool.CallableTool
type StreamableTool  = trpctool.StreamableTool
type ToolSet         = trpctool.ToolSet
type Declaration     = trpctool.Declaration
type Schema          = trpctool.Schema
```

### 3.2 工具注册项（ToolRegistration）

```go
type ToolRegistration struct {
    Name                 string
    Description          string
    Factory              func(ctx context.Context) (Tool, error)       // 单工具工厂
    ToolSetFactory       func(ctx context.Context) (ToolSet, error)    // 工具集工厂
    EnabledByDefault     bool
    Category             string   // filesystem / execution / web / search / communication / productivity / interaction / coding / integration / composition
    Tags                 []string // 分类标签，支持 RegistryByTag / RegistryByCategory 查询
    RiskLevel            string   // low / medium / high / critical
    RequiresConfirmation bool
    SupportsStreaming    bool
    SupportsConcurrency  bool
}
```

**语义**：
- `Factory` 与 `ToolSetFactory` 互斥：单工具用 Factory，工具集用 ToolSetFactory
- `EnabledByDefault`：true 表示 catalog 行 `enabled=true`（默认开放），false 表示仅显式允许
- `Category`：与 `builtin_tools_seed.go` 中的分类对齐
- `Tags`：工具分类标签，用于按标签/分类查询注册工具（`RegistryByTag`/`RegistryByCategory`）
- `RiskLevel`：影响前端展示与 Agent 策略

### 3.3 装配配置（AssemblyConfig）

```go
type AssemblyConfig struct {
    EnabledTools  []string            // 从 effective tools 计算得出的启用工具名列表
    FilesystemDir string              // file ToolSet 的根目录覆盖
    GeminiModel   string              // geminifetch 的模型名
    GoogleAPIKey  string              // Google Search API Key
    GoogleCX      string              // Google Search Engine ID
    ClaudeCodeDir string              // claudecode ToolSet 的工作目录
    OpenAPISpecs  []OpenAPISpecConfig // OpenAPI 动态工具集
    AgentTools    []AgentToolConfig   // Agent-as-Tool 配置
    MCPServers    []MCPServerConfig   // MCP 服务器连接配置
    MCPBroker     *MCPBrokerConfig    // MCP Broker 配置
    CustomTools   []Tool              // 额外注入的自定义工具
}
```

### 3.4 装配输出（AssembledToolsets）

```go
type AssembledToolsets struct {
    ToolSets []ToolSet
    Tools    []Tool
}
```

### 3.5 Agent-as-Tool 配置

```go
type AgentToolConfig struct {
    Agent             trpcagent.Agent
    Name              string
    Description       string
    SkipSummarization bool
    StreamInner       bool
    HistoryScope      trpcagenttool.HistoryScope
    ResponseMode      trpcagenttool.ResponseMode
}
```

### 3.6 MCP 配置

```go
type MCPServerConfig struct {
    Name       string
    Transport  string            // stdio / sse / streamable_http
    ServerURL  string            // HTTP 传输的 URL
    Command    string            // stdio 传输的可执行文件
    Args       []string          // stdio 传输的参数
    Headers    map[string]string // HTTP 传输的请求头
    TimeoutSec int               // 连接超时
    ToolPrefix string            // 工具名前缀过滤
}

type MCPBrokerConfig struct {
    Servers         []MCPServerConfig
    AllowAdHocHTTP  bool
    AdHocTimeoutSec int
}

type OpenAPISpecConfig struct {
    Name     string
    SpecURL  string
    SpecData []byte
}
```

---

## 四、目录结构

```
internal/tools/
├── tool.go                  — 类型别名 + ConfigString + RegistryByTag/RegistryByCategory
├── toolset.go               — Registry() 注册表 + Assemble() 编排调度 + 子装配器
├── doc.go                   — 包文档（框架能力速查 + 注册工具清单 + 自定义工具指南）
│
├── trpc/                    — 向后兼容适配层：ToolsetConfig → AssemblyConfig → tools.Assemble()
│   ├── toolsets.go          — BuildToolsets()，被 trpc_build.go 调用
│   ├── effective_config.go  — Effective tool keys → ToolsetConfig 映射
│   ├── runtime_config.go    — 运行时配置解析
│   ├── confirmation.go      — 工具确认策略
│   └── toolset_prune.go     — 工具集裁剪
│
├── cache/                   — 工具结果缓存（LRU 驱逐 + TTL）
│   └── result_cache.go      — ResultCache + Global()/SetGlobal()
│
├── custom/                  — 自定义工具实现
│   └── demo.go              — 示例：使用 function.NewFunctionTool 构建自定义工具
│
├── hostexecnorm/            — 主机执行工具参数归一化
│   ├── normalize.go         — working_dir → workdir 兼容映射
│   └── wrap.go              — ToolSet 包装器
│
├── kanban/                  — 看板工具集（CI/CD 集成）
│   ├── bridge.go            — Bridge/BridgeReader/BridgeWriter/BridgeLifecycle 接口
│   └── tools.go             — NewToolset + Enabled
│
├── knowledge/               — Knowledge 工具
│   └── tool.go              — knowledge_search 工具 + WithRetriever context 注入
│
├── mcpobserve/              — MCP 运行时可观测性
│   └── observe.go           — 重连检测 + SessionReconnectMax
│
├── memory/                  — Memory 工具（add/update/load/search/delete）
│   └── tools.go             — DefaultTools() 返回标准 memory 工具
│
├── preview/                 — 工具调用预览脱敏
│   └── preview.go           — RedactAndTruncate
│
├── serviceawaitreply/       — 服务级 await_user_reply（阻塞式，替代框架内置版）
│   ├── tool.go              — ServiceTool + ReplyFunc context 注入
│   └── tool_confirm.go      — 确认请求上下文
│
├── skillrouter/             — Skill 检测与分类
│   ├── detect.go
│   └── taxonomy.go
│
├── skillruntime/            — Skill 工具集解析
│   ├── runtime.go           — 运行时工具集构建
│   ├── toolset.go           — Skill ToolSet
│   ├── resolve.go           — Skill 解析
│   └── filter.go            — Skill 过滤（fail-closed + 缓存驱逐）
│
├── testexec/                — 工具在线测试
│   ├── config.go            — 测试配置解析
│   └── execute.go           — 测试执行
│
├── webresearch/             — Web 搜索工具
│   ├── tool.go              — web_research CallableTool
│   └── config.go            — 搜索配置
│
└── cli_admin/               — CLI 管理工具
    └── registry.go          — Skill 安装/卸载工具
```

**与旧设计的差异**：不设 `tooldef/`、`toolctx/`、`middleware/`、`executor/`、`adkbridge/`、`backends/`、`telemetry/` 子包。框架接口直接通过 type alias 使用，横切关注点通过框架内建机制注入。

---

## 五、注册表（Registry）

### 5.1 注册工具清单

| 注册名 | Category | Tags | Factory / ToolSetFactory | EnabledByDefault | RiskLevel | RequiresConfirmation |
|--------|----------|------|--------------------------|------------------|-----------|---------------------|
| `file` | filesystem | filesystem, read, write, search | `ToolSetFactory: trpcfile.NewToolSet` | ✅ | low | — |
| `hostexec` | execution | shell, exec, command | `ToolSetFactory: trpchostexec.NewToolSet` | ❌ | critical | ✅ |
| `httpfetch` | web | web, fetch, http | `Factory: trpchttpfetch.NewTool` | ❌ | medium | — |
| `claudefetch` | web | web, fetch, claude | `Factory` (stub) | ❌ | medium | — |
| `geminifetch` | web | web, fetch, gemini | `Factory: trpcgeminifetch.NewTool` | ❌ | medium | — |
| `duckduckgo` | search | search, web | `Factory: trpcduckduckgo.NewTool` | ❌ | medium | — |
| `google_search` | search | search, web, google | `ToolSetFactory: trpcgooglesearch.NewToolSet` | ❌ | medium | — |
| `arxiv_search` | search | search, academic, paper | `ToolSetFactory: trpcarxivsearch.NewToolSet` | ❌ | low | — |
| `wikipedia` | search | search, encyclopedia | `ToolSetFactory: trpcwikipedia.NewToolSet` | ❌ | low | — |
| `email` | communication | email, send, smtp | `ToolSetFactory: trpcemail.NewToolSet` | ❌ | high | ✅ |
| `todo` | productivity | todo, task, manage | `Factory: trpctodo.New` | ❌ | low | — |
| `await_user_reply` | interaction | interaction, reply, await | `Factory: trpcawaitreply.New` | ❌ | low | — |
| `claudecode` | coding | coding, ide, claude | `ToolSetFactory: trpcclaudecode.NewToolSet` | ❌ | critical | ✅ |
| `workspace_exec` | execution | exec, workspace, code | `Factory` (CodeExecutor 路径) | ❌ | critical | ✅ |
| `openapi` | integration | api, rest, openapi | `ToolSetFactory` (需 spec 配置) | ❌ | medium | — |
| `agent` | composition | agent, delegation, composition | 无 Factory（运行时通过 AgentToolConfig 注入） | ❌ | medium | — |
| `mcp` | integration | mcp, integration, protocol | 无 Factory（运行时通过 MCPServerConfig 注入） | ❌ | medium | — |
| `mcpbroker` | integration | mcp, broker, discovery | 无 Factory（运行时通过 MCPBrokerConfig 注入） | ❌ | medium | — |

### 5.2 非 Registry 注入的工具

以下工具不经过 Registry，通过其他路径注入到 Agent：

| 工具 | 注入路径 | 说明 |
|------|----------|------|
| memory (add/update/load/search/delete) | `trpc_build.go` → `memorytool.DefaultTools()` → `WithTools` | 仅当 `MemoryEnabled=true` 且 `HasMemory=true` 时注入 |
| knowledge_search | `trpc/toolsets.go` → `knowledgepkg.NewSearchTool()` → CustomTools | 仅当 `KnowledgeSearch=true` 时注入 |
| call_agent | `trpc/toolsets.go` → `a2a.NewCallAgentTool()` → CustomTools | 仅当 `CallAgent=true` 时注入 |
| await_user_reply (ServiceTool) | `trpc/toolsets.go` → `serviceawaitreply.New()` → CustomTools | 仅当 `AwaitHook != nil` 时注入，替代框架内置版 |
| skill tools | `trpc_build.go` → `WithSkills(repo)` + `WithCodeExecutor` | 通过框架 Skill 机制注入 |

---

## 六、装配数据流

### 6.1 端到端流程

```
AgentRuntimeSettings
        │
        ▼
GetEffectiveTools() ─── 计算 effective tool keys
        │                    (profile + allow/deny + catalog enabled)
        ▼
buildToolsetsForAgent() ─── ToolsetConfig 标记哪些工具启用
        │
        ▼
BuildToolsets() ─── ToolsetConfig → AssemblyConfig
        │              (enabled 名列表 + 配置参数)
        ▼
Assemble() ─── 编排调度，依次调用子装配器：
        │    assembleFromRegistry()    — Registry 注册工具匹配
        │    assembleFilesystem()      — 文件系统工具集（WithBaseDir 覆盖）
        │    assembleHostExec()        — 主机命令执行（WithBaseDir + 归一化包装）
        │    assembleGeminiFetch()     — Gemini 网页抓取（需 GeminiModel）
        │    assembleGoogleSearch()    — Google 搜索（需 APIKey + CX）
        │    assembleClaudeCode()      — Claude Code（WithBaseDir 覆盖）
        │    assembleOpenAPISpecs()    — OpenAPI Spec 工具集
        │    assembleAgentTools()      — Agent-as-Tool
        │    assembleMCPServers()      — MCP 服务器
        │    assembleMCPBroker()       — MCP Broker
        │    assembleMemory()          — 记忆工具
        ▼
AssembledToolsets ─── { ToolSets, Tools }
        │
        ▼
BuildTRPCLLMAgent() ─── WithToolSets + WithTools
        │                 + WithToolFilter (deny)
        │                 + WithToolCallbacks (invocation 记录)
        │                 + WithToolCallRetryPolicy
        │                 + WithEnableParallelTools
        ▼
trpc-agent-go LLMAgent ─── 运行时执行
```

### 6.2 Effective Tools 计算语义

```
catalog.enabled=true  → "默认开放"：profile 门控通过且不在 deny 列表即可用
catalog.enabled=false → "仅显式允许"：必须在 allow 列表中才可用
deny 列表 + ToolsEnabled=false → 覆盖一切
```

计算公式（`computeEffectiveToolState`）：
```
baseEnabled = ToolsEnabled && (catalogOpenByDefault || policyNamesKey)
enabled     = baseEnabled && state == "allowed"  // 未被 deny
```

### 6.3 Tool Key 映射关系

| Registry 名 | Effective Tool Key | Declaration().Name | 说明 |
|-------------|-------------------|-------------------|------|
| `file` | `read_file`, `save_file`, `list_file` 等 | `read_file`, `save_file` 等 | ToolSet 展开为多个 key |
| `hostexec` | `shell_exec` | `shell_exec` | 注册名与 key 不同 |
| `duckduckgo` | `duckduckgo_search` | `duckduckgo_search` | 注册名与 key 不同 |
| `httpfetch` | `web_fetch` | `web_fetch` | 注册名与 key 不同 |
| `geminifetch` | `gemini_web_fetch` | `gemini_web_fetch` | 注册名与 key 不同 |
| `todo` | `todo_write` | `todo_write` | 注册名与 key 不同 |

> **关键约束**：`builtin_tools_seed.go` 中的 tool key 必须与框架工具的 `Declaration().Name` 一致，否则 effective tool 策略无法正确匹配。

---

## 七、框架横切机制映射

项目不自建中间件链，使用 trpc-agent-go 内建机制：

| 横切关注点 | 框架机制 | 项目注入点 | 代码位置 |
|-----------|---------|-----------|---------|
| **调用前拦截** | `tool.Callbacks.RegisterBeforeTool` | — | 暂未使用 |
| **调用后记录** | `tool.Callbacks.RegisterAfterTool` | 记录 ToolInvocation | `trpc_build.go` → `buildToolCallbacks` |
| **工具过滤** | `tool.NewExcludeToolNamesFilter` | deny 列表过滤 | `trpc_build.go` → `buildToolFilter` |
| **自动重试** | `tool.RetryPolicy` | 可配置重试策略 | `trpc_build.go` → `buildToolRetryPolicy` |
| **并行执行** | `llmagent.WithEnableParallelTools` | 开关控制 | `trpc_build.go` |
| **流式工具** | `StreamableTool` + `StreamReader` | 框架内置 | 工具实现侧 |
| **结果合并** | `tool.Merge[T]` | 框架内置 | 框架自动处理 |

### 7.1 Callbacks 注入结构

```go
callbacks := trpctool.NewCallbacks()

callbacks.RegisterAfterTool(func(ctx context.Context, args *trpctool.AfterToolArgs) (*trpctool.AfterToolResult, error) {
    recordToolInvocationAsync(ctx, args, ag, deps)
    return &trpctool.AfterToolResult{}, nil
})

// Agent 集成
llmagent.WithToolCallbacks(callbacks)
```

### 7.2 Filter 注入结构

```go
denyList := jsonStringList(settings.ToolsDenyJSON)
filter := trpctool.NewExcludeToolNamesFilter(denyList...)

// Agent 集成
llmagent.WithToolFilter(filter)
```

### 7.3 RetryPolicy 注入结构

```go
policy := &trpctool.RetryPolicy{
    MaxAttempts:     maxAttempts,      // 默认 2
    InitialInterval: initialMs * time.Millisecond,  // 默认 500ms
    BackoffFactor:   backoff,          // 默认 2.0
    MaxInterval:     maxMs * time.Millisecond,      // 默认 5000ms
    Jitter:          settings.ToolsRetryJitter,
    RetryOn:         trpctool.DefaultRetryOn,  // EOF, network timeout/temporary
}

// Agent 集成
llmagent.WithToolCallRetryPolicy(policy)
```

---

## 八、自定义工具开发模式

### 8.1 使用 function.NewFunctionTool（推荐）

```go
package custom

import (
    "context"
    trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
    "trpc.group/trpc-go/trpc-agent-go/tool/function"
)

type MyInput struct {
    Query string `json:"query" jsonschema:"description=搜索关键词,required"`
    Limit int    `json:"limit" jsonschema:"description=最大结果数,default=5"`
}

type MyOutput struct {
    Results []MyResult `json:"results"`
}

type MyResult struct {
    Title string `json:"title"`
    URL   string `json:"url"`
}

func myExecute(ctx context.Context, input MyInput) (MyOutput, error) {
    // 业务逻辑
    return MyOutput{}, nil
}

func NewMyTool() trpctool.Tool {
    return function.NewFunctionTool(
        myExecute,
        function.WithName("my_search"),
        function.WithDescription("自定义搜索工具"),
    )
}
```

### 8.2 实现 CallableTool 接口

```go
type myTool struct{}

func (t *myTool) Declaration() *trpctool.Declaration {
    return &trpctool.Declaration{
        Name:        "my_tool",
        Description: "手动实现的工具",
        InputSchema: &trpctool.Schema{
            Type:       "object",
            Properties: map[string]*trpctool.Schema{...},
        },
    }
}

func (t *myTool) Call(ctx context.Context, args []byte) (any, error) {
    var input MyInput
    if err := json.Unmarshal(args, &input); err != nil {
        return nil, fmt.Errorf("my_tool: invalid args: %w", err)
    }
    // 业务逻辑
    return result, nil
}
```

### 8.3 注入路径

自定义工具通过 `AssemblyConfig.CustomTools` 注入：

```go
// 方式 1：通过 ToolsetConfig.CustomTools
cfg.CustomTools = append(cfg.CustomTools, custom.NewMyTool())

// 方式 2：在 BuildToolsets 中按条件注入
if cfg.KnowledgeSearch {
    customTools = append(customTools, knowledgepkg.NewSearchTool())
}
```

---

## 九、Tool Key 命名规范

| 规则 | 示例 |
|------|------|
| 使用 snake_case | `duckduckgo_search`、`web_fetch` |
| 动词_名词结构 | `read_file`、`send_email`、`search_content` |
| ToolSet 展开后的子工具保持独立 key | `file` → `read_file` + `save_file` + `list_file` + ... |
| MCP 工具加前缀 | `{server_name}_{original_tool_name}` |
| 注册名与 key 可能不同 | `duckduckgo`(注册名) → `duckduckgo_search`(key) |

**一致性约束**：
- `builtin_tools_seed.go` 的 key = `Declaration().Name` = effective tool policy 的 key
- 三处必须一致，否则策略无法生效

---

## 十、ToolInvocation 记录结构

```go
type ToolInvocationWrite struct {
    ToolKey       string    // Declaration().Name
    AgentID       string
    AgentKey      string
    SessionID     string
    UserID        string
    Status        string    // "success" / "error"
    DurationMS    int
    StartedAt     string    // RFC3339
    EndedAt       string    // RFC3339
    InputPreview  string    // 截断至 2000 字符
    OutputPreview string    // 截断至 2000 字符
    ErrorCode     string
    ErrorMessage  string    // 截断至 500 字符
    Source        string    // "adk"
    ToolCallID    string    // 模型分配的调用 ID
}
```

**脱敏规则**：
- `InputPreview` / `OutputPreview`：截断至 2000 字符，不存完整明文
- `ErrorMessage`：截断至 500 字符
- 敏感字段（API Key、Token 等）不落库

---

## 十一、Effective Tools 策略结构

### 11.1 Profile 预设

```go
var toolProfiles = map[string][]string{
    "chat_only": {},
    "read_only": {"datetime", "read_file", "read_multiple_files", "list_file", "search_file", "search_content", "todo_write"},
    "coding":    {"group:filesystem", "group:web", "group:skill", "group:session", "datetime"},
    "research":  {"duckduckgo_search", "web_fetch", "gemini_web_fetch", "google_search", "arxiv_search", "wikipedia_search",
                  "read_file", "read_multiple_files", "list_file", "search_file", "search_content",
                  "skill_search", "memory_search", "todo_write", "datetime"},
    "full":      {"group:filesystem", "group:web", "group:skill", "group:memory", "group:media",
                  "group:runtime", "group:messaging", "group:session", "group:cli_admin", "datetime"},
}
```

### 11.2 Tool Group 展开

```go
var toolGroupsFilesystem = []string{"read_file", "read_multiple_files", "save_file", "list_file", "search_file", "search_content", "replace_content"}
var toolGroupsWeb       = []string{"duckduckgo_search", "web_fetch", "gemini_web_fetch", "google_search", "arxiv_search", "wikipedia_search"}
var toolGroupsMemory    = []string{"memory_search", "memory_get"}
var toolGroupsSkill     = []string{"skill_search", "use_skill"}
var toolGroupsMedia     = []string{"read_image", "read_document", "create_image", "tts"}
var toolGroupsRuntime   = []string{"shell_exec", "claude_code", "workspace_exec"}
var toolGroupsMessaging = []string{"send_email"}
var toolGroupsSession   = []string{"await_user_reply", "todo_write"}
```

### 11.3 策略计算结果

```go
type AgentEffectiveTools struct {
    ToolsEnabled bool
    Profile      string
    Allow        []string
    Deny         []string
    Items        []EffectiveAgentTool
}

type EffectiveAgentTool struct {
    ToolKey        string
    DisplayName    string
    Category       string
    Source         string
    Enabled        bool
    EffectiveState string   // "allowed" / "denied"
    Reason         string   // "profile:coding" / "agent_deny" / "agent_tools_disabled"
}
```

---

## 十二、Biz 层 ToolRepo 子接口

`ToolRepo` 原有 18 个方法，违反项目红线 #15（Repository 接口方法不得超过 5 个）。已拆分为 8 个子接口 + 1 个组合接口：

| 子接口 | 方法数 | 职责 |
|--------|--------|------|
| `ToolReader` | 2 | 工具查询（SearchTools, GetTool） |
| `ToolWriter` | 5 | 工具增删改（Create, Update, Delete, UpdateEnabled, UpdateConfig） |
| `ToolInvocationReader` | 2 | 调用记录查询（SearchInvocations, GetInvocationParams） |
| `ToolInvocationWriter` | 1 | 调用记录写入（RecordInvocation） |
| `ToolAuditRepo` | 3 | 审计日志（RecordAudit, SearchAudits, PurgeOldAudits） |
| `ToolOverrideReader` | 2 | Agent 覆盖查询（ListOverrides, ListOverridesByAgent） |
| `ToolOverrideWriter` | 2 | Agent 覆盖写入（UpsertOverride, DeleteOverride） |
| `ToolSyncer` | 1 | 内置工具同步（SyncBuiltinTools） |
| `ToolCatalogReader` | 4 | 只读窄接口（= ToolReader + ToolOverrideReader） |

`ToolRepo` 保留为组合接口（嵌入上述 8 个子接口），保持向后兼容。

**窄接口传播**：

| 消费者 | 依赖接口 | 说明 |
|--------|----------|------|
| `ToolUsecase` | `ToolRepo`（全量） | Usecase 需要全量访问 |
| `AgentUsecase.tools` | `ToolCatalogReader` | 只需 SearchTools + Override 查询 |
| `agent.Deps.ToolsCatalog` | `biz.ToolCatalogReader` | 只需读取工具目录 |
| `team.Runner.toolsCatalog` | `biz.ToolCatalogReader` | 只需读取工具目录 |
| `runtime.Catalog.Tools` | `biz.ToolCatalogReader` | 只需读取工具目录 |

---

## 十三、新增工具的步骤清单

1. 在 `internal/tools/toolset.go` 的 `Registry()` 中添加 `ToolRegistration` 条目（含 Tags 标签）
2. 若工具需要配置，在 `AssemblyConfig` 中添加对应字段
3. 在 `internal/tools/toolset.go` 中添加 `assembleXxx()` 子装配器函数
4. 在 `internal/tools/trpc/toolsets.go` 的 `BuildToolsets()` 中添加启用标志映射
5. 在 `internal/agent/trpc_build.go` 的 `buildToolsetsForAgent()` 中添加 effective key 到配置的映射
6. 在 `internal/data/builtin_tools_seed.go` 中添加种子数据（key 必须与 `Declaration().Name` 一致）
7. 在 `internal/biz/agent_effective_tools.go` 中按需更新 tool group 和 profile 定义
8. 编写单元测试验证注册 → 装配 → 注入链路
9. 在 `internal/tools/trpc/effective_config.go` 的 `ToolsetConfigFromEffectiveKeys` 中添加映射
10. 在 `internal/tools/testexec/config.go` 中添加在线测试支持判断

---

## 十四、Data 层错误处理规范

Data 层统一使用 `kerrors` 返回错误，禁止 `errors.New` / `sql.ErrNoRows`：

| 场景 | 使用 |
|------|------|
| Ent 客户端不可用 | `kerrors.InternalServer("TOOL", "ent client unavailable")` |
| 记录不存在 | `kerrors.NotFound("TOOL", "tool not found")` |
| 参数校验失败 | `kerrors.BadRequest("TOOL", "tool key is required")` |

---

## 十五、装配可观测性规范

`Assemble` 子装配器在以下场景必须通过 `event.SysLogWarn` 记录日志：

| 场景 | 日志事件 | 说明 |
|------|----------|------|
| 工具已启用但配置缺失 | `system.tool_assembly_skip` | geminifetch 无 model、google_search 无 apiKey/cx |
| Factory 返回 nil 无 error | `system.tool_assembly_skip` | stub 工具或占位注册项 |
| OpenAPI spec 加载失败 | `system.builtin_tools_sync_fail` | 已有 |

---

## 十六、并发安全规范

| 全局变量 | 保护方式 | 位置 |
|----------|----------|------|
| `globalResultCache` | `sync.RWMutex`（`globalMu`） | `internal/tools/cache/result_cache.go` |
| `toolWebResChecker` | `sync.RWMutex`（`toolWebResCheckerMu`） | `internal/biz/tool/tool_catalog_runtime.go` |
| `filterCache.entries` | `sync.RWMutex`（读用 RLock，写用 Lock） | `internal/tools/skillruntime/filter.go` |

---

## 十七、Wire 窄接口规范

Wire provider 函数不得依赖跨模块具体类型（红线 #7）。已改造的 provider：

| Provider | 旧参数 | 新参数 | 说明 |
|----------|--------|--------|------|
| `provideSkillWatchRunner` | `*biz.SkillUsecase` | `watch.SkillReader` + `watch.SkillWriter` | watch 包已定义窄接口 |
| `provideMonitorUsecase` | `*biz.SkillUsecase` | `biz.FilesystemHealthReader` | 通过 `provideFilesystemHealthReader` 适配 |

**待改造**：

| Provider | 当前参数 | 目标 | 说明 |
|----------|----------|------|------|
| `provideChatServiceDeps` | `*biz.SkillUsecase` | 拆分 `rt.Catalog.SkillUC` 为窄接口 | 影响面大，后续迭代 |

---

## 十八、Skill 文件系统安全规范

`CreateSkillDir` 必须校验 slug 参数：

| 校验规则 | 错误类型 | 说明 |
|----------|----------|------|
| 空 slug | `kerrors.BadRequest("SKILL", "slug is required")` | 防止 SKILL.md 写入根目录 |
| 包含 `..` | `kerrors.BadRequest("SKILL", "slug contains unsafe path characters")` | 防止路径遍历 |
| 以 `/` 开头 | `kerrors.BadRequest("SKILL", "slug contains unsafe path characters")` | 防止绝对路径逃逸 |


---

## 子模块：Tools Fragment Edit 设计

> **版本**：1.1 | **状态**：✅ 已实现（2026-05-22）
> **需求**：[23 tools-fragment-edit.md](./23%20tools-fragment-edit.md) · **开发计划**：[23-tools-development.md §Phase 4](./23-tools-development.md#phase-4片段级文件编辑p1)
> **父设计**：[23 tools.design.md §七 运行时层](./23%20tools.design.md#七运行时层)

---

## 1. 设计目标

在 **trpc-agent-go `tool/file` ToolSet** 内新增 `diff_edit`、`patch_file`，并引入 **SessionFileState** 会话缓存，使片段编辑在运行时达到：

- **O(变更大小)** 的模型输出与内存处理
- **1 读 + 1 写** 的磁盘访问（同 invocation 缓存命中时 0 读）
- **原子多 hunk** 提交

---

## 2. 分层与依赖

```
┌─────────────────────────────────────────────────────────┐
│  internal/agent/prompt.go          ← 编辑工作流提示        │
│  internal/data/builtin_tools_seed  ← catalog 种子         │
│  internal/biz/agent_effective_tools← toolGroupsFilesystem │
│  internal/tools/runtime_alias.go   ← 别名（Phase 2）      │
│  internal/tools/testexec/config.go ← 在线测试映射         │
│  internal/agent/activity_meta.go   ← 活动流中文标签        │
└──────────────────────────┬──────────────────────────────┘
                           │ Assemble(file ToolSet)
┌──────────────────────────▼──────────────────────────────┐
│  internal/tools/toolset.go         ← Registry 不变       │
│  internal/tools/trpc/toolsets.go   ← FilesystemDir 注入  │
└──────────────────────────┬──────────────────────────────┘
                           │
┌──────────────────────────▼──────────────────────────────┐
│  pkg/trpc-agent-go/tool/file/      ← ★ 实现真相源        │
│    diffedit.go / patchfile.go                            │
│    editcontent.go        ← load/commit + SessionFileState │
│    patch/                ← parse / apply / validate        │
│  pkg/trpc-agent-go/tool/internal/textfile/  ← 抽取共享   │
│    （encoding / line ending / quote fuzzy）               │
└───────────────────────────────────────────────────────────┘
         ↑ 复用逻辑来源（抽取，不复制业务到 internal/）
         pkg/trpc-agent-go/tool/claudecode/
           file_state.go · common.go (buildStructuredPatch)
```

**红线**：

- 实现放在 `pkg/trpc-agent-go/tool/file`，**禁止**在 `internal/biz` import trpc-agent-go 实现编辑逻辑
- `internal/tools` 仅做装配、别名、catalog，不写 patch 算法

---

## 3. 工具 API

### 3.1 `diff_edit`

**Declaration name**：`diff_edit`

```json
{
  "type": "object",
  "properties": {
    "file_name": {
      "type": "string",
      "description": "Relative file path under base_directory"
    },
    "edits": {
      "type": "array",
      "minItems": 1,
      "maxItems": 20,
      "items": {
        "type": "object",
        "properties": {
          "search": { "type": "string", "description": "Text to find; multi-line allowed" },
          "replace": { "type": "string", "description": "Replacement text; empty string deletes" },
          "replace_all": { "type": "boolean", "default": false }
        },
        "required": ["search", "replace"]
      }
    },
    "expected_mtime_ms": {
      "type": "integer",
      "description": "Optional optimistic lock from prior read_file"
    }
  },
  "required": ["file_name", "edits"]
}
```

**Description 要点**（写入 function.WithDescription）：

- Read file first; use `search_content` to locate symbols when needed
- Only changed fragments — never whole file
- Prefer over `save_file` for modifications
- Use `patch_file` when you already have unified diff

### 3.2 `patch_file`

**Declaration name**：`patch_file`

```json
{
  "type": "object",
  "properties": {
    "file_name": { "type": "string" },
    "patch": {
      "type": "string",
      "description": "Unified diff text; mutually exclusive with hunks"
    },
    "hunks": {
      "type": "array",
      "description": "Structured hunks; mutually exclusive with patch",
      "items": {
        "type": "object",
        "properties": {
          "old_start": { "type": "integer" },
          "old_lines": { "type": "integer" },
          "new_start": { "type": "integer" },
          "new_lines": { "type": "integer" },
          "lines": {
            "type": "array",
            "items": { "type": "string" },
            "description": "Unified diff body lines with ' ', '-', '+' prefixes"
          }
        },
        "required": ["old_start", "old_lines", "new_start", "new_lines", "lines"]
      }
    },
    "expected_mtime_ms": { "type": "integer" }
  },
  "required": ["file_name"]
}
```

约束：`patch` 与 `hunks` 必须且仅能提供一个。

### 3.3 响应（共用字段）

```json
{
  "base_directory": "...",
  "file_name": "...",
  "applied_edits": 2,
  "total_replacements": 3,
  "structured_patch": [ { "old_start": 42, "old_lines": 5, "new_start": 42, "new_lines": 7, "lines": ["-...", "+..."] } ],
  "message": "Applied 2 edits to 'internal/foo.go'"
}
```

`structured_patch` 格式与 claudecode `patchHunk` 对齐，供 Activity / 前端 diff 预览扩展。

---

## 4. SessionFileState

### 4.1 数据结构

实现位于 **`editcontent.go`**（load/commit 编排）与 **`internal/toolcache/file_views.go`**（per-invocation 存取），**不**挂在 `fileToolSet` 实例字段上。

```go
// internal/toolcache/file_views.go

type FileView struct {
    Content    string
    MtimeMs    int64
    Encoding   string
    LineEnding string
    Mode       os.FileMode
}
```

- 挂在 **`agent.Invocation.State`**（`toolcache.StoreFileViewFromContext`），与 `skill_run` 输出缓存同模式
- 与 `BuildTRPCLLMAgentCached`（Agent LRU ~10min）兼容，避免跨 session 泄漏
- **同一文件勿并行** `diff_edit` / `patch_file`；可选 `expected_mtime_ms`（来自 `read_file.mtime_ms`）作乐观锁

### 4.2 读写协议

| 操作 | 行为 |
|------|------|
| `read_file` | 读盘后 `storeFileViewAfterRead`；响应含 **`mtime_ms`** |
| `diff_edit` / `patch_file` | `loadEditSnapshot` → cache 命中且 mtime 一致则跳过 ReadFile → apply → `commitEditSnapshot` → `storeFileView` |
| `save_file` / `replace_content` | 写盘后 `storeSaveFileView`（读回磁盘再解码，保持 encoding 一致） |
| mtime 不匹配 | 返回 `file_modified_externally`，hint re-read |
| 磁盘文件已删、cache 仍命中 | **同 invocation 设计取舍**：仍用 cache 内容编辑并可写回重建；外部删改以 mtime 为准；见 Review FRAG-P2-03 |

### 4.3 与 claudecode fileState 的关系

| 项 | claudecode | file ToolSet SessionFileState |
|----|------------|-------------------------------|
| read-before-write 强制 | 是 | **否**（Prompt 软约束 + mtime 硬约束） |
| partial read 视图 | 是 | Phase 1 全文件；Phase 2 可记录 slice |
| 引号 fuzzy | 是 | **复用**抽取后的 `findActualString` |

抽取目标包：`pkg/trpc-agent-go/tool/internal/textfile`（claudecode 与 file 共同 import）。

---

## 5. 核心算法

### 5.1 diff_edit 流程

```
resolvePath(file_name)
  → loadContent (cache or ReadFile)
  → validate text / not binary / not .ipynb
  → check expected_mtime_ms
  → for each edit in edits:
        resolve search (exact → whitespace → quote fuzzy)
        count matches; enforce replace_all policy
        apply strings.Replace on in-memory content
  → buildStructuredPatch(old, new)
  → WriteFile (preserve mode)
  → storeView
  → return response
```

任一 edit 失败：**不 WriteFile**，返回结构化错误（含 `edit_index`、`match_lines`）。

### 5.2 patch_file 流程

```
parse patch string OR validate hunks[]
  → loadContent
  → for each hunk (ascending old_start):
        verify '-' lines against file lines at old_start
        apply splice
  → WriteFile + storeView
```

Unified diff 解析子集（Phase 1）：

- `@@ -old_start,old_lines +new_start,new_lines @@` hunk header
- 行前缀：` ` context、`-` delete、`+` insert
- 不支持：git binary patch、`diff --git` 多文件（单文件 only）

### 5.3 patch 包

```
pkg/trpc-agent-go/tool/file/patch/
  hunk.go           // patchHunk 类型（自 claudecode 迁入）
  apply.go          // ApplyHunks(content, hunks) (string, error)
  parse_unified.go  // ParseUnifiedDiff(patch) ([]patchHunk, error)
  validate.go       // ValidateHunk(fileLines, hunk) error
```

---

## 6. file ToolSet 注册

在 `file.go` 的 `NewToolSet` 中：

```go
type fileToolSet struct {
    // ...existing fields...
    diffEditEnabled   bool  // default true
    patchFileEnabled  bool  // default true
}
```

工具列表追加（在 `replaceContentEnabled` 之后）：

- `diffEditTool()`
- `patchFileTool()`

Limits（常量）：

| 常量 | 默认值 |
|------|--------|
| `maxEditsPerCall` | 20 |
| `maxPatchBytes` | 256 KiB |
| `maxEditSearchBytes` | 64 KiB per search block |
| `maxEditReplaceBytes` | 256 KiB per replace block |

---

## 7. 项目集成清单

| # | 位置 | 变更 |
|---|------|------|
| 1 | `internal/data/builtin_tools_seed.go` | 新增 `diff_edit`、`patch_file` seed |
| 2 | `internal/biz/agent_effective_tools.go` | `toolGroupsFilesystem` 追加 key |
| 3 | `internal/biz/tool_policy_keys.go` | 若需 policy alias |
| 4 | `internal/tools/runtime_alias.go` | Phase 2：`edit_file` → `diff_edit`（可选） |
| 4b | `internal/biz/tool_policy_keys.go` | **须与 runtime_alias 同步**（allow/deny 策略键） |
| 5 | `internal/tools/testexec/config.go` | `AssemblyForCatalogKey` 映射到 `file` |
| 6 | `internal/agent/prompt.go` | 编辑工作流：`diff_edit` 优先 |
| 7 | `internal/agent/activity_meta.go` | 中文标签 |
| 8 | `web/src/features/agents/useAgentToolsCatalog.ts` | filesystem 组展示（若硬编码列表） |

**Registry**：仍注册 `file` ToolSet 一条，不新增 Registry 名。

---

## 8. 错误响应结构

工具错误通过 `message` + 结构化 JSON 字段返回（function tool result）：

**diff_edit — 歧义匹配**

```json
{
  "error": "edit_not_unique",
  "edit_index": 1,
  "match_count": 3,
  "match_lines": [42, 108, 205],
  "hint": "Add more context to search or set replace_all=true"
}
```

**patch_file — hunk 不一致**

```json
{
  "error": "hunk_mismatch",
  "hunk_index": 0,
  "expected_lines": ["-    return 1"],
  "actual_lines": ["-    return 0"],
  "hint": "Re-read file and regenerate patch"
}
```

**并发冲突**

```json
{
  "error": "file_modified_externally",
  "hint": "Call read_file again before editing"
}
```

---

## 9. 性能设计

| 层级 | 手段 | 预期 |
|------|------|------|
| 模型 | 片段参数 | 生成时间主导项，显著下降 |
| 运行时 | SessionFileState | 同文件二次编辑省 1 次 ReadFile |
| 运行时 | 单次多 edit | N 次 tool call → 1 次 |
| 运行时 | 内存 patch | 毫秒级（Go string splice） |
| Phase 2 | 行区间读 | >1MB 文件仅加载 hunk ±context |

**端到端**：磁盘 I/O 已接近 Cursor 本地写入；Agent 场景仍受 LLM 推理约束，无法达到 IDE 即时感。

---

## 10. 安全

- 复用 `resolvePath`、`maxFileSize`、文本校验（与 `read_file` / `replace_content` 一致）
- `maxPatchBytes` / `maxEditSearchBytes` / `maxEditReplaceBytes` 防 DoS
- **同文件勿并行** `diff_edit` / `patch_file`（Invocation 缓存无锁；Prompt + `expected_mtime_ms` 软/硬约束）
- 写操作保留原 `FileMode`
- 不经过 Tool 结果缓存（非幂等）

---

## 11. 测试策略

| 包 | 覆盖 |
|----|------|
| `tool/file/patch/*_test.go` | unified 解析、hunk apply、偏移累加 |
| `tool/file/diffedit_test.go` | 多 edit 原子性、fuzzy、歧义、mtime_ms |
| `tool/file/patchfile_test.go` | mismatch 回滚、新建文件 |
| `internal/toolcache/file_views.go` + `editcontent.go` | cache hit / mtime 失效 |
| `internal/tools/testexec/config_test.go` | catalog key 映射 |

---

## 12. 迁移阶段（技术侧）

| 阶段 | 内容 |
|------|------|
| **T1** | 抽取 `textfile` + `patch` 包；`patch_file`（hunks 模式） |
| **T2** | unified diff 解析；`diff_edit` |
| **T3** | SessionFileState + read/save/replace 写回缓存 |
| **T4** | catalog / prompt / activity 集成 |
| **T5** | 可选：`edit_file` 别名 → `diff_edit`；`replace_content` deprecated 描述 |

任务编号与验收勾选见 [23-tools-development.md §Phase 4](./23-tools-development.md#phase-4片段级文件编辑p1)。

---

*文档版本：1.0 — 片段级文件编辑技术设计；不含修复记录与 sprint 进度。*
