# Tools 工具模块 — 实现设计文档

> 对应需求：[23-tools.md](./23-tools.md) · 开发计划：[23-tools.development.md](./23-tools.development.md)
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md` · 框架：trpc-agent-go

---

## 一、模块概述

工具注册与管理：CallableTool / StreamableTool / MCP Tool 统一目录、Agent 工具绑定、运行时挂载。工具是 Agent 可调用的具体外部能力，与 Plugin（运行时拦截器）和 Skill（面向 Agent 的能力+知识包）有明确边界。

核心能力：
- 工具目录 CRUD（含内置工具 + MCP 工具 + 外部工具）
- 工具启用/停用/风险等级管理（高风险启用需 `confirm_intent = I_UNDERSTAND_RISK`）
- Agent 工具绑定与生效矩阵（profile + allow/deny + catalog）
- 工具调用记录（ToolInvocation）查询与审计（ToolInvocationAudit）
- 运行时工具挂载（trpc-agent-go Tool/ToolSet 适配）
- 工具工作区统一（file / shell / claude_code 共用 `workspace_root`）
- 片段级文件编辑（`diff_edit` / `patch_file`）— catalog 与运行时工具均已实现

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
option java_multiple_files = true;
option java_package = "api.kratos.tool.v1";

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
  string runtime_status = 19;           // "available"/"registered_only"/"disabled"
  string runtime_kind = 20;             // "function"/"streaming"/"approval"
  int32 invoke_count = 21;
  int32 invoke_count_24h = 22;
  int32 success_count = 23;
  int32 failure_count = 24;
  int32 blocked_count = 25;
  int32 agent_override_count = 26;
  int32 repaired_count = 34;          // 修复守卫：参数畸形但修复成功（90d 窗口聚合 metadata_json.args_repaired）
  int32 invalid_count = 35;           // 修复守卫：参数畸形且不可修复（90d 窗口聚合 metadata_json.args_invalid）
  optional double avg_duration_ms = 27;
  string last_invoked_at = 28;
  string last_status = 29;              // "success"/"error"/"blocked"
  string created_at = 30;
  string updated_at = 31;
  ToolPermissions permissions = 32;
  double p95_duration_ms = 33;          // P95 耗时（毫秒）
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
  string sort = 8;                      // 排序字段
  bool abnormal = 9;                    // true：仅返回最近一次调用以 error/blocked 收尾的工具（「仅看异常」）
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
  // confirm_intent must be "I_UNDERSTAND_RISK" when enabling high/critical
  // risk tools. Replaces the old confirm_key which matched the tool key
  // (a guessable value that provided no real security).
  string confirm_intent = 3;
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
  bool streaming = 27;                  // 是否流式调用
  int32 chunk_count = 28;               // 流式分片数
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
  optional bool has_error = 9;          // 仅查失败记录
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
  string mode = 6;                      // "inherit"/"override"/"deny"
  string config_override_json = 7;
  bool requires_confirmation = 8;
  string created_at = 9;
  string updated_at = 10;
}

message ListToolAgentOverridesRequest {
  string tool_id = 1 [(google.api.field_behavior) = REQUIRED];
}

message ListToolAgentOverridesResponse {
  repeated ToolAgentOverride items = 1;
}

message ListToolAgentOverridesByAgentRequest {
  string agent_id = 1 [(google.api.field_behavior) = REQUIRED];
}

// 独立响应类型，避免与 ListToolAgentOverridesResponse 耦合
message ListToolAgentOverridesByAgentResponse {
  repeated ToolAgentOverride items = 1;
}

message UpsertToolAgentOverrideRequest {
  string tool_id = 1 [(google.api.field_behavior) = REQUIRED];
  string agent_id = 2 [(google.api.field_behavior) = REQUIRED];
  bool enabled = 3;
  string mode = 4;
  string config_override_json = 5;
  bool requires_confirmation = 6;
}

message DeleteToolAgentOverrideRequest {
  string tool_id = 1 [(google.api.field_behavior) = REQUIRED];
  string agent_id = 2 [(google.api.field_behavior) = REQUIRED];
}

message ToolInvocationParam {
  string id = 1;
  string invocation_id = 2;
  string tool_key = 3;
  string params_json = 4;
  bool redaction_applied = 5;
  string created_at = 6;
}

message GetToolInvocationParamsRequest {
  string invocation_id = 1 [(google.api.field_behavior) = REQUIRED];
}

message UpdateToolConfigRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
  string config_json = 2 [(google.api.field_behavior) = REQUIRED];
}

message TestToolRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
  string arguments_json = 2;
  int32 timeout_sec = 3;
}

message TestToolResponse {
  string status = 1;                    // "success"/"error"/"timeout"
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
  // 静态 GET 路径必须先于 /v1/tools/{id} 注册，避免 gorilla/mux 把
  // "runs"/"audits" 等变量段误匹配为 {id}。
  rpc ListTools(ListToolsRequest) returns (ListToolsResponse) {
    option (google.api.http) = {get: "/v1/tools"};
  }
  rpc ListToolRuns(ListToolRunsRequest) returns (ListToolRunsResponse) {
    option (google.api.http) = {get: "/v1/tools/runs"};
  }
  rpc ListToolInvocationAudits(ListToolInvocationAuditsRequest) returns (ListToolInvocationAuditsResponse) {
    option (google.api.http) = {get: "/v1/tools/audits"};
  }
  rpc GetToolInvocationParams(GetToolInvocationParamsRequest) returns (ToolInvocationParam) {
    option (google.api.http) = {get: "/v1/tools/runs/{invocation_id}/params"};
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
  rpc ListToolRunsForTool(ListToolRunsForToolRequest) returns (ListToolRunsResponse) {
    option (google.api.http) = {get: "/v1/tools/{tool_id}/runs"};
  }
  rpc ListToolAgentOverrides(ListToolAgentOverridesRequest) returns (ListToolAgentOverridesResponse) {
    option (google.api.http) = {get: "/v1/tools/{tool_id}/agent-overrides"};
  }
  rpc ListToolAgentOverridesByAgent(ListToolAgentOverridesByAgentRequest) returns (ListToolAgentOverridesByAgentResponse) {
    option (google.api.http) = {get: "/v1/agents/{agent_id}/tool-overrides"};
  }
  rpc UpsertToolAgentOverride(UpsertToolAgentOverrideRequest) returns (ToolAgentOverride) {
    option (google.api.http) = {put: "/v1/tools/{tool_id}/agent-overrides/{agent_id}" body: "*"};
  }
  rpc DeleteToolAgentOverride(DeleteToolAgentOverrideRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = {delete: "/v1/tools/{tool_id}/agent-overrides/{agent_id}"};
  }
  rpc UpdateToolConfig(UpdateToolConfigRequest) returns (Tool) {
    option (google.api.http) = {put: "/v1/tools/{id}/config" body: "*"};
  }
  rpc TestTool(TestToolRequest) returns (TestToolResponse) {
    option (google.api.http) = {post: "/v1/tools/{id}/test" body: "*"};
  }
}
```

### 2.3 HTTP API 汇总

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/v1/tools` | 列表查询，支持 search/category/source/risk_level/enabled/sort/abnormal 筛选，含 Summary |
| GET | `/v1/tools/runs` | 全局工具调用记录查询（静态路径，先于 `{id}` 注册） |
| GET | `/v1/tools/audits` | 工具调用审计日志查询（静态路径，先于 `{id}` 注册） |
| GET | `/v1/tools/runs/{invocation_id}/params` | 查询工具调用脱敏参数 |
| GET | `/v1/tools/{id}` | 获取单个工具详情 |
| POST | `/v1/tools` | 创建工具 |
| PUT | `/v1/tools/{id}` | 更新工具 |
| DELETE | `/v1/tools/{id}` | 软删除工具 |
| PATCH | `/v1/tools/{id}/enabled` | 启用/停用工具（高风险启用需 `confirm_intent=I_UNDERSTAND_RISK`） |
| GET | `/v1/tools/{tool_id}/runs` | 指定工具的调用记录查询 |
| GET | `/v1/tools/{tool_id}/agent-overrides` | 查询工具的 Agent 覆盖列表 |
| GET | `/v1/agents/{agent_id}/tool-overrides` | 查询 Agent 的工具覆盖列表 |
| PUT | `/v1/tools/{tool_id}/agent-overrides/{agent_id}` | 创建/更新 Agent 工具覆盖 |
| DELETE | `/v1/tools/{tool_id}/agent-overrides/{agent_id}` | 删除 Agent 工具覆盖 |
| GET | `/v1/agents/{agent_id}/tool-grants` | 查询 Agent 的持久化工具授权列表 |
| DELETE | `/v1/agents/{agent_id}/tool-grants/{tool_key}` | 撤销 Agent 的工具授权 |
| PUT | `/v1/tools/{id}/config` | 更新工具配置（`config_json` 必填） |
| POST | `/v1/tools/{id}/test` | 在线测试工具 |

> **路径设计要点**：Override 路径使用 `tool_id`（而非 `tool_key`），与 `ResolveToolKey` 解析链对齐；`/v1/tools/runs/{invocation_id}/params` 路径与 `/v1/tools/{id}` 区分，避免 mux 路由冲突。

---

## 三、Biz 层

### 3.1 领域模型

文件路径：`internal/biz/tool/tool.go`

```go
type Tool struct {
    ID                   string
    Key                  string              // 唯一标识，如 "duckduckgo_search"
    DisplayName          string
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
    RuntimeStatus        string              // "available"/"registered_only"/"disabled"
    RuntimeKind          string              // "function"/"streaming"/"approval"
    InvokeCount          int                 // 总调用次数
    InvokeCount24h       int                 // 24h 调用次数
    SuccessCount         int
    FailureCount         int
    BlockedCount         int
    AgentOverrideCount   int                 // Agent 级别覆盖数
    AvgDurationMS        *float64            // 平均耗时（毫秒）
    P95DurationMS        float64             // P95 耗时（毫秒）
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
    Streaming        bool                // 是否流式调用
    ChunkCount       int                 // 流式分片数
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
    Streaming     bool
    ChunkCount    int
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
    Mode                 string              // "inherit"/"override"/"deny"
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
    HasError  *bool               // 仅查失败记录
    Limit     int
    Offset    int
}

type ToolRunResult struct {
    Items  []ToolInvocation
    Total  int
    Limit  int
    Offset int
}

// 审计写入模型
type ToolInvocationAuditWrite struct {
    InvocationID  string
    ToolKey       string
    AgentID       string
    UserID        string
    SessionID     string
    Action        string
    ResultSummary string
    Status        string
    Source        string
}

// 审计读取模型
type ToolInvocationAudit struct {
    ID            string
    InvocationID  string
    ToolKey       string
    AgentID       string
    UserID        string
    SessionID     string
    Action        string
    ResultSummary string
    Status        string
    Source        string
    CreatedAt     string
}

type ToolAuditQuery struct {
    ToolKey   string
    AgentID   string
    UserID    string
    SessionID string
    Status    string
    From      string
    To        string
    Limit     int
    Offset    int
}

type ToolAuditResult struct {
    Items  []ToolInvocationAudit
    Total  int
    Limit  int
    Offset int
}
```

### 3.2 Repo 接口（8 子接口 + 组合 + 窄接口）

文件路径：`internal/biz/tool/tool.go`

为遵守红线 #15（Repository 接口方法 ≤ 5），`ToolRepo` 拆分为 8 个子接口 + 1 个组合接口 + 1 个窄接口：

```go
// Stability:stable
type ToolReader interface {
    SearchTools(ctx context.Context, q ToolListQuery) (ToolListResult, error)
    GetTool(ctx context.Context, idOrKey string) (Tool, error)
    // ListToolCatalogEntries 单次 IN 批量查询轻量构建期目录行（key/config_json/
    // default_config_json/requires_confirmation），替代 Agent 构建期逐键 GetTool 循环（N+1）。
    ListToolCatalogEntries(ctx context.Context, keys []string) ([]ToolCatalogEntry, error)
}

// Stability:stable
type ToolWriter interface {
    CreateTool(ctx context.Context, in ToolUpsertInput) (Tool, error)
    UpdateTool(ctx context.Context, idOrKey string, in ToolUpsertInput) (Tool, error)
    DeleteTool(ctx context.Context, idOrKey string) error
    UpdateToolEnabled(ctx context.Context, idOrKey string, enabled bool) (Tool, error)
    UpdateToolConfig(ctx context.Context, idOrKey string, configJSON string) (Tool, error)
}

// Stability:stable
type ToolInvocationReader interface {
    SearchToolInvocations(ctx context.Context, q ToolRunQuery) (ToolRunResult, error)
    GetToolInvocationParams(ctx context.Context, invocationID string) (ToolInvocationParam, error)
}

// Stability:stable
type ToolInvocationWriter interface {
    RecordToolInvocation(ctx context.Context, in ToolInvocationWrite) error
}

// Stability:stable
type ToolAuditRepo interface {
    RecordToolInvocationAudit(ctx context.Context, in ToolInvocationAuditWrite) error
    SearchToolInvocationAudits(ctx context.Context, q ToolAuditQuery) (ToolAuditResult, error)
    PurgeToolInvocationAuditsBefore(ctx context.Context, cutoffRFC3339 string) (int64, error)
}

// Stability:stable
type ToolOverrideReader interface {
    ListToolAgentOverrides(ctx context.Context, toolKey string) ([]ToolAgentOverride, error)
    ListToolAgentOverridesByAgent(ctx context.Context, agentID string) ([]ToolAgentOverride, error)
}

// Stability:stable
type ToolOverrideWriter interface {
    UpsertToolAgentOverride(ctx context.Context, in ToolAgentOverrideInput, toolID string) (ToolAgentOverride, error)
    DeleteToolAgentOverride(ctx context.Context, toolKey string, agentID string) error
}

type ToolSyncer interface {
    SyncBuiltinTools(ctx context.Context) error
}

// Stability:stable — 窄接口：ToolReader + ToolOverrideReader
type ToolRegistryReader interface {
    ToolReader
    ToolOverrideReader
}

// Stability:stable — 组合接口（嵌入 8 子接口），保持向后兼容
type ToolRepo interface {
    ToolReader
    ToolWriter
    ToolInvocationReader
    ToolInvocationWriter
    ToolAuditRepo
    ToolOverrideReader
    ToolOverrideWriter
    ToolSyncer
}

// SettingRepo 提供工具 usecase 所需的系统设置只读访问
// Stability:stable
type SettingRepo interface {
    GetWebResearch(ctx context.Context) (WebResearchSetting, error)
}

type WebResearchSetting struct {
    Provider    string
    APIKey      string
    HasAPIKey   bool
    MaxResults  int
    FetchTop    int
    SearchDepth string
    TimeoutSec  int
    HTTPProxy   string
}
```

**窄接口传播**：

| 消费者 | 依赖接口 | 说明 |
|--------|----------|------|
| `ToolUsecase` | `ToolRepo`（全量） | Usecase 需要全量访问 |
| `AgentUsecase.tools` | `ToolRegistryReader` | 只需 SearchTools + Override 查询 |
| `agent.Deps.ToolRegistry` | `biz.ToolRegistryReader` | 只需读取工具目录 |
| `team.Runner.toolRegistry` | `biz.ToolRegistryReader` | 只需读取工具目录 |
| `runtime.TurnReadDeps.Tools` | `biz.ToolRegistryReader` | 只需读取工具目录 |

### 3.3 Usecase

```go
type ToolUsecase struct {
    repo          ToolRepo
    sys           SettingRepo
    tester        ToolTester
    webResChecker WebResearchReadinessChecker
    lg            loggateway.Logger
}

func NewToolUsecase(repo ToolRepo, sys SettingRepo, lg loggateway.Logger, opts ...ToolUsecaseOption) *ToolUsecase

// 选项：
//   WithToolTester(tester)              — 注入在线测试器
//   WithWebResearchChecker(checker)     — 注入 Web 研究就绪检查器
```

**核心方法签名**：

```go
func (u *ToolUsecase) ListTools(ctx, q ToolListQuery) (ToolListResult, error)
// - 校验分页：Limit 默认 20，上限 100，Offset >= 0
// - 调用 repo.SearchTools
// - enrichToolList：注入 WebResearch 平台状态 + 就绪检查

func (u *ToolUsecase) GetTool(ctx, id string) (Tool, error)
// - 校验 id 非空
// - 调用 repo.GetTool（支持 id 或 key 查找）
// - EnrichToolRuntimeFieldsWithPlatform：注入运行时字段

func (u *ToolUsecase) Create(ctx, in ToolUpsertInput) (Tool, error)
// - validateToolUpsert：校验 key/display_name 非空
// - 调用 repo.CreateTool

func (u *ToolUsecase) Update(ctx, id string, in ToolUpsertInput) (Tool, error)
// - 校验 id 非空
// - assertToolMutable：readonly 工具禁止修改
// - 调用 repo.UpdateTool

func (u *ToolUsecase) Delete(ctx, id string) error
// - 校验 id 非空
// - assertToolDeletable：内置工具禁止删除
// - 调用 repo.DeleteTool（软删除）

// ConfirmIntentValue 是启用 high/critical 风险工具时 confirm_intent 参数的必需值。
// 确保调用方显式确认风险，而非匹配可猜测的 key。
const ConfirmIntentValue = "I_UNDERSTAND_RISK"

func (u *ToolUsecase) ToggleEnabled(ctx, id string, enabled bool, confirmIntent ...string) (Tool, error)
// - 校验 id 非空
// - 启用 high/critical 风险工具时，confirmIntent[0] 必须等于 ConfirmIntentValue
// - 调用 repo.UpdateToolEnabled

func (u *ToolUsecase) UpdateToolConfig(ctx, id string, configJSON string) (Tool, error)
// - 校验 id 非空
// - configJSON 为空时默认 "{}"
// - validateToolConfigAgainstSchema：用 gojsonschema 校验配置符合 ConfigSchemaJSON
// - 调用 repo.UpdateToolConfig

func (u *ToolUsecase) ListRuns(ctx, q ToolRunQuery) (ToolRunResult, error)
// - 校验分页参数
// - 调用 repo.SearchToolInvocations

func (u *ToolUsecase) ListRunsForTool(ctx, toolIDOrKey string, q ToolRunQuery) (ToolRunResult, error)
// - ResolveToolKey：将 id 或 key 解析为 catalog tool_key
// - 调用 ListRuns

func (u *ToolUsecase) RecordToolInvocationAudit(ctx, in ToolInvocationAuditWrite) error

func (u *ToolUsecase) ListInvocationAudits(ctx, q ToolAuditQuery) (ToolAuditResult, error)

// ToolAuditRetentionDays 是审计日志默认保留天数
const ToolAuditRetentionDays = 90

func (u *ToolUsecase) PurgeOldInvocationAudits(ctx) (int64, error)
// - cutoff = now - 90d
// - 调用 repo.PurgeToolInvocationAuditsBefore

func (u *ToolUsecase) SyncBuiltinTools(ctx) error

func (u *ToolUsecase) GetToolInvocationParams(ctx, invocationID string) (ToolInvocationParam, error)

func (u *ToolUsecase) ListToolAgentOverrides(ctx, toolIDOrKey string) ([]ToolAgentOverride, error)
// - ResolveToolKey：将 id 或 key 解析为 catalog tool_key
// - 调用 repo.ListToolAgentOverrides

func (u *ToolUsecase) ListToolAgentOverridesByAgent(ctx, agentID string) ([]ToolAgentOverride, error)

func (u *ToolUsecase) UpsertToolAgentOverride(ctx, in ToolAgentOverrideInput) (ToolAgentOverride, error)
// - 校验 agentID 非空
// - GetTool：解析 tool_key → tool.ID
// - Mode 默认 "inherit"，ConfigOverrideJSON 默认 "{}"
// - 调用 repo.UpsertToolAgentOverride(ctx, in, tool.ID)

func (u *ToolUsecase) DeleteToolAgentOverride(ctx, toolIDOrKey string, agentID string) error
// - ResolveToolKey
// - 调用 repo.DeleteToolAgentOverride
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
        // StorageKey 显式映射列名，避免依赖 Ent 默认复数化规则
        field.Text("fallback_config_json").StorageKey("default_config_json").Default("{}"),
        field.Text("metadata_json").Default("{}"),
        field.String("created_at").Default(""),
        field.String("updated_at").Default(""),
        field.String("deleted_at").Default(""),
    }
}

func (PlatformTool) Indexes() []ent.Index {
    return []ent.Index{
        index.Fields("category").StorageKey("idx_tools_category"),
        index.Fields("enabled").StorageKey("idx_tools_enabled"),
        index.Fields("deleted_at").StorageKey("idx_tools_deleted_at"),
        index.Fields("enabled", "deleted_at").StorageKey("idx_tools_enabled_deleted"),
        index.Fields("category", "enabled", "deleted_at").StorageKey("idx_tools_cat_enabled_deleted"),
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
        field.Bool("streaming").Default(false),
        field.Int("chunk_count").Default(0),
        field.Text("metadata_json").Default("{}"),
        field.String("created_at"),
        field.String("deleted_at").Default("").Optional(),
    }
}

func (ToolInvocation) Indexes() []ent.Index {
    return []ent.Index{
        index.Fields("tool_key", "started_at").StorageKey("idx_tool_invocations_tool_time"),
        index.Fields("agent_id", "started_at").StorageKey("idx_tool_invocations_agent_time"),
        index.Fields("session_id").StorageKey("idx_tool_invocations_session"),
        index.Fields("status").StorageKey("idx_tool_invocations_status"),
        index.Fields("deleted_at").StorageKey("idx_tool_invocations_deleted_at"),
    }
}
```

#### ToolInvocationAudit（工具调用审计表）

文件路径：`internal/data/ent/schema/tool_invocation_audit.go`

映射数据库表 `tool_invocation_audit`。

```go
type ToolInvocationAudit struct {
    ent.Schema
}

func (ToolInvocationAudit) Annotations() []schema.Annotation {
    return []schema.Annotation{
        entsql.Annotation{Table: "tool_invocation_audit"},
    }
}

func (ToolInvocationAudit) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").Immutable().Unique().MaxLen(256),
        field.String("invocation_id").Default(""),
        field.String("tool_key"),
        field.String("agent_id").Default(""),
        field.String("user_id").Default(""),
        field.String("session_id").Default(""),
        field.String("action").Default("tool.call"),
        field.Text("result_summary").Default(""),
        field.String("status").Default("success"),
        field.String("source").Default("adk"),
        field.String("created_at"),
    }
}

func (ToolInvocationAudit) Indexes() []ent.Index {
    return []ent.Index{
        index.Fields("tool_key", "created_at"),
        index.Fields("agent_id", "created_at"),
        index.Fields("user_id", "created_at"),
        index.Fields("session_id").StorageKey("idx_tool_invocation_audit_session"),
    }
}
```

#### ToolAgentOverride（Agent 工具覆盖表）

文件路径：`internal/data/ent/schema/tool_agent_override.go`

映射数据库表 `tool_agent_overrides`。

```go
type ToolAgentOverride struct {
    ent.Schema
}

func (ToolAgentOverride) Annotations() []schema.Annotation {
    return []schema.Annotation{
        entsql.Annotation{Table: "tool_agent_overrides"},
    }
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
        field.String("created_at"),
        field.String("updated_at"),
        field.String("deleted_at").Default(""),
    }
}

func (ToolAgentOverride) Indexes() []ent.Index {
    return []ent.Index{
        index.Fields("tool_key", "agent_id").Unique(),
        index.Fields("agent_id"),
        index.Fields("tool_key"),
    }
}
```

### 4.2 Repo 实现

文件路径：`internal/data/tool.go`、`internal/data/tool_audit.go`

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

**聚合查询 SQL**（节选，2026-08-14 口径）：

```go
func toolSelectSQL(d Dialect) string {
    // d.Greatest 方言适配：Postgres=GREATEST（MAX 仅聚合），SQLite=MAX 标量。
    // toolSelectPrefixArgs() 提供前导参数：24h cutoff + 3× stats 窗口 cutoff
    // （toolStatsWindowDays=90，与 biz.ToolAuditRetentionDays 对齐）。
    return `
        SELECT t.id, t.tool_key, ...,
               COALESCE(stats.invoke_count, 0), COALESCE(stats.invoke_count_24h, 0), ...,
               stats.avg_duration_ms, COALESCE(p95.p95_duration_ms, 0),
               COALESCE(last.started_at, ''), COALESCE(last.status, ''),
               t.created_at, t.updated_at, t.deleted_at
        FROM tools t
        LEFT JOIN (
            SELECT tool_key,
                   COUNT(1) AS invoke_count,
                   SUM(CASE WHEN started_at >= ? THEN 1 ELSE 0 END) AS invoke_count_24h,
                   SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) AS success_count,
                   -- 状态口径统一（S0，2026-08-14）：runtime recorder 写 'failed'，
                   -- 遗留 a2a/testexec 路径写 'error'，failure_count 必须同时覆盖两者，
                   -- 否则 runtime 失败从工具列表统计中消失。
                   SUM(CASE WHEN status IN ('failed', 'error') THEN 1 ELSE 0 END) AS failure_count,
                   SUM(CASE WHEN status = 'blocked' THEN 1 ELSE 0 END) AS blocked_count,
                   -- 参数质量（S1）：metadata_json 的 args_repaired/args_invalid 布尔标记，
                   -- JSON 提取走 Dialect.JSONExtract（PG ->> 得 'true'，SQLite json_extract 得 1）。
                   SUM(CASE WHEN <json_extract metadata_json.args_repaired> THEN 1 ELSE 0 END) AS repaired_count,
                   SUM(CASE WHEN <json_extract metadata_json.args_invalid> THEN 1 ELSE 0 END) AS invalid_count,
                   AVG(duration_ms) AS avg_duration_ms
            FROM tool_invocations
            WHERE started_at >= ?          -- PERF-3：90d 统计窗口
            GROUP BY tool_key
        ) stats ON stats.tool_key = t.tool_key
        LEFT JOIN (
            -- PERF-2：Postgres 用 percentile_cont(0.95) WITHIN GROUP（精确插值分位数），
            -- 取代旧"top 5% 均值"近似；SQLite 无有序集聚合，保留 top-5% AVG（仅 dev/CLI）。
            SELECT tool_key,
                   percentile_cont(0.95) WITHIN GROUP (ORDER BY duration_ms) AS p95_duration_ms
            FROM tool_invocations
            WHERE started_at >= ?          -- 同一 90d 窗口
            GROUP BY tool_key
        ) p95 ON p95.tool_key = t.tool_key
        LEFT JOIN (
            SELECT tool_key, COUNT(1) AS agent_override_count
            FROM tool_agent_overrides
            WHERE deleted_at = ''
            GROUP BY tool_key
        ) overrides ON overrides.tool_key = t.tool_key
        LEFT JOIN (
            SELECT tool_key, started_at, status FROM (
                SELECT ti.tool_key, ti.started_at, ti.status,
                       ROW_NUMBER() OVER (PARTITION BY ti.tool_key ORDER BY ti.started_at DESC, ti.id DESC) AS rn
                FROM tool_invocations ti
                WHERE ti.started_at >= ?    -- 同一 90d 窗口
            ) WHERE rn = 1
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

**CreateTool**：

```go
func (r *toolRepo) CreateTool(ctx context.Context, in biz.ToolUpsertInput) (biz.Tool, error)
// - 校验 key 非空
// - applyBuiltinToolDefaults：补全默认值（source="builtin", risk_level="low", JSON 默认 "{}"）
// - 生成 ID：优先 "tool_{key}"，否则 uniqueToolID
// - 使用 Ent ORM 插入
// - 返回 r.GetTool(ctx, id)
```

**UpdateTool**：

```go
func (r *toolRepo) UpdateTool(ctx context.Context, idOrKey string, in biz.ToolUpsertInput) (Tool, error)
// - 通过 toolByIDOrKey 查找现有记录
// - applyBuiltinToolDefaults
// - 使用 Ent ORM 更新
// - 返回 r.GetTool(ctx, key)
```

**DeleteTool**（软删除）：

```go
func (r *toolRepo) DeleteTool(ctx context.Context, idOrKey string) error
// - 通过 toolByIDOrKey 查找
// - 设置 deleted_at = nowRFC3339()
```

**RecordToolInvocation**（工具调用记录写入）：

```go
func (r *toolRepo) RecordToolInvocation(ctx context.Context, in biz.ToolInvocationWrite) error
// - 生成 ID
// - input_preview/output_preview 截断至 2000 字符
// - 写入 tool_invocations 表（含 streaming / chunk_count）
```

**SearchToolInvocations**（工具调用记录查询）：

```go
func (r *toolRepo) SearchToolInvocations(ctx context.Context, q biz.ToolRunQuery) (biz.ToolRunResult, error)
// - 原始 SQL 查询 tool_invocations 表（过滤 deleted_at = ''）
// - LEFT JOIN tools（获取 display_name）和 agents（获取 agent display_name）
// - 支持 tool_key/agent_id/session_id/status/from/to/has_error 筛选
// - ORDER BY started_at DESC, created_at DESC
```

**错误翻译**：所有 Repo 方法的数据库错误必须经 `entErrToBizErr(err, domain)` 翻译（红线 DB-R5）。

### 4.3 内置工具种子

文件路径：`internal/data/builtin_tools_seed.go`

**核心机制**：

1. `builtinPlatformToolSeeds` 定义所有内置工具的初始数据（key、displayName、description、category、riskLevel、enabled、paramsSchema、registryName）
2. `ensureBuiltinPlatformTools` 在数据库初始化时执行 `INSERT ... ON CONFLICT(tool_key) DO NOTHING`
3. `syncBuiltinToolsFromRegistry` 从 `tools.Registry()` 同步 risk_level、requires_confirmation、supports_streaming、supports_concurrency 到 tools 表
4. `syncBuiltinWebToolCatalogPatches` 用 `UPDATE ... WHERE tool_key = ?` 修补已有 DB 的 catalog 元数据（description、params_schema、config_schema、enabled）

**关键约束**：`registryName` 字段将 seed 行与 `tools.Registry()` 中的 `ToolRegistration` 关联，确保 seed 的 tool_key 与框架工具的 `Declaration().Name` 一致。

**种子表覆盖范围**（节选）：
- `datetime`、`web_research`、`duckduckgo_search`、`web_fetch`、`gemini_web_fetch`、`google_search`、`arxiv_search`、`wikipedia_search`
- `read_file`、`read_multiple_files`、`save_file`、`list_file`、`search_file`、`search_content`、`read_lints`、`delete_file`、`replace_content`、`diff_edit`、`patch_file`
- `skill_search`、`use_skill`、`memory_search`、`memory_get`
- `read_image`、`read_document`、`read_spreadsheet`、`create_image`、`tts`
- `shell_exec`、`send_email`、`todo_write`、`await_user_reply`、`kanban`、`claude_code`、`workspace_exec`
- `call_agent`、`knowledge_search`、`knowledge_reflect`、`mcp_tool_set`、`mcp_broker`
- `working_memory_read`、`working_memory_list`、`working_memory_write`、`working_memory_patch`、`working_memory_delete`
- `model_registry_sync`、`browser`、`read_tool_result`
- `plan_and_execute`、`cancel_orchestration`、`synthesize_results`、`build_orchestration_graph`

> **2026-08-14 移除**：`check_progress` 已从种子表与运行时删除（DEAD-1，系统推送模式 `checkAllTeamsCompleted` → `SynthesizeResults` → `ExecuteTurn` 取代 LLM 轮询）；存量库由 `syncRemovedBuiltinToolPatches` 启动时幂等软删。

> **注意**：`message` 与 `subagents_*`（spawn/list/get/cancel）种子条目已补齐（默认停用，`subagents_cancel` 需确认），分类为 `orchestration` / `session`。
>
> `gemini_web_fetch` 种子 schema 与 live `Declaration` 一致为必填 `prompt`（URL 写在提示词里）；旧字段 `url` 仍由 `argnorm` 映射。存量库由 `syncBuiltinWebToolCatalogPatches` 刷 schema。

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

- `bizToolToProto(t biz.Tool) *v1.Tool`：转换工具模型，注意 `AvgDurationMS` 是 `*float64` → `optional double`，`P95DurationMS` 是 `float64` → `double`
- `bizSummaryToProto(s biz.ToolSummary) *v1.ToolSummary`
- `bizInvocationToProto(x biz.ToolInvocation) *v1.ToolInvocation`：含 `streaming` / `chunk_count`
- `bizAuditToProto(x biz.ToolInvocationAudit) *v1.ToolInvocationAudit`

**RPC 方法实现**：

| 方法 | 说明 |
|------|------|
| `ListTools` | 分页查询 + Summary，使用 `biz.PageToLimitOffset` |
| `ListToolRuns` | 全局调用记录查询（支持 `has_error` 筛选） |
| `ListToolInvocationAudits` | 审计日志查询 |
| `GetToolInvocationParams` | 调用参数脱敏查询 |
| `GetTool` | 按 ID 查找，sql.ErrNoRows → NotFound |
| `CreateTool` | CreateToolRequest → ToolUpsertInput → uc.Create |
| `UpdateTool` | UpdateToolRequest → ToolUpsertInput → uc.Update |
| `DeleteTool` | uc.Delete，sql.ErrNoRows → NotFound |
| `ToggleToolEnabled` | 透传 `confirm_intent` 给 uc.ToggleEnabled |
| `ListToolRunsForTool` | 按 tool_id 解析为 tool_key 后查询 |
| `ListToolAgentOverrides` | 按 tool_id 解析为 tool_key 后查询 |
| `ListToolAgentOverridesByAgent` | 按 agent_id 查询 |
| `UpsertToolAgentOverride` | 透传 tool_id 给 uc（uc 内部解析为 tool_key + tool.ID） |
| `DeleteToolAgentOverride` | 按 tool_id 解析为 tool_key 后删除 |
| `UpdateToolConfig` | uc.UpdateToolConfig |
| `TestTool` | uc.tester.Execute（5s 超时） |

---

## 六、Wire 注入

```
data.ProviderSet  → NewToolRepo
biz.ProviderSet   → NewToolUsecase
service.ProviderSet → NewToolService
```

**窄接口 provider**（红线 #7）：

| Provider | 参数 | 说明 |
|----------|------|------|
| `provideSkillWatchRunner` | `watch.SkillReader` + `watch.SkillWriter` | 已改造 |
| `provideMonitorUsecase` | `biz.FilesystemHealthReader` | 通过 `provideFilesystemHealthReader` 适配 |
| `provideChatServiceDeps` | `*biz.SkillUsecase`（待改造） | 影响面大，后续迭代 |

---

## 七、运行时层

### 7.1 整体架构

```
Agent 构建请求
  → BuildTRPCLLMAgent(ctx, ag, deps)
    → loadEffectiveToolKeys / loadToolBuildCatalog
    → buildToolsetsForAgent
      → tools.Assemble                         // 装配（不再单独套 argnorm/filenorm 包装）
      → tools.ApplyDefaultDecorators           // 唯一 Call 入口：归一化 + 锁 + 预算 + 缓存
    → llmagent.WithToolSets / WithTools / WithToolCallbacks / WithToolCallRetryPolicy
    → llmagent.WithEnableParallelTools(true)

一次 Call：
  BeforeTool（JSON 修复 / 系统字段剥离 / 确认门）
    → ToolDecorator.Call
         NormalizeInvocation（hostexec + file + web/search 别名）
         → 路径锁 / family 锁
         → inner（git 仓写文件才进 worktree；否则活树）
```

批执行（Spirit）：`BatchExecuteAssembledTools` 走同一 `CallableTool.Call`，清除 `IsolationStrategy` 且不复制 Wire worktree。`ParallelToolExecutor` 的 worktree 只给**自己写文件的 raw handler**。

### 7.1.1 工具装饰器（ToolDecorator）

文件路径：`internal/tools/decorator.go`、`internal/tools/decorator_apply.go`、`internal/tools/safety.go`

**设计决策**：详见 ADR [2026-06-15-review-adr-tool-parallel-execution.md](../reports/2026-06-15-review-adr-tool-parallel-execution.md)

**三层保护**（项目层实现，不修改框架内部）：

| 方案 | 能力 | 默认值 | 实现位置 |
|------|------|--------|----------|
| P0-G3 | 每次工具调用执行超时 | **回调链** `ToolsExecutionTimeoutSec`（0→10min）；装饰器 `Timeout=0` 不另加 60s。`plan_and_execute` 仍覆盖为 3min | `toolExecutionTimeoutHooks` + `ToolDecorator.applyTimeout` |
| P0-D | 工具结果大小预算 + 截断/卸载 | 10KB（`DefaultResultBudget`）；超限优先卸载信封 | `ToolDecorator.truncateResult` |
| P0-D2 | AfterTool 字符串兜底截断 | 50k runes；**仅未装饰工具**；跳过信封/预算覆盖/已截断标记 | `NewOutputSizeLimiterHook`（priority 60） |
| P2-E | 确定性缓存 | 仅网络 ConcurrentSafe（`web_fetch`/search），默认 TTL 60s、invocation 作用域；file 族不缓存。BeforeTool `ResultCache` 对 `IsCacheable`/写/file 跳过，避免双缓存 | `IsCacheable` + `CatalogResultCacheAllowed` + `ToolDecorator.lookupCache/storeCache` |
| P1-C | 工具安全分类 | 运行时名映射到 Registry；未知默认 Exclusive | `ClassifyTool(name)` |
| P1-C2 | Exclusive 进程内互斥 | hostexec 族共享一把锁；文件写按 `file_name` 分锁；`list_file`/`search_*` 对目录树共享覆盖锁（与子路径写互斥，不同目录仍并行）；`read_file` 同路径共享 | `fileLockRequests` + `filePathLockTable` + `ToolDecorator.Call` |

**接口设计**：

- `NewToolDecorator(inner, cfg)` 返回 `trpctool.CallableTool` 接口类型
- 内部工具支持流式时返回 `*streamableToolDecorator`（满足 `StreamableTool`）
- 内部工具不支持流式时返回 `*ToolDecorator`（**不**满足 `StreamableTool`，避免框架误分类）
- 编译期接口断言：`var _ trpctool.CallableTool = (*ToolDecorator)(nil)` + `var _ trpctool.StreamableTool = (*streamableToolDecorator)(nil)`

**已知限制**：

- DeferredManager 延迟加载的工具不经过装饰器（超时仍由回调链 `ToolsExecutionTimeoutSec` 覆盖；字符串结果由 AfterTool limiter 兜底截断）
- ToolSet 管理的工具由 `decoratedToolSet` 按声明名复用装饰器实例，网络 ConcurrentSafe 缓存可命中（file 族仍不缓存）
- 缓存踩踏：并发首次调用相同参数可能多次执行 inner tool（P2 优先级可接受）
- Exclusive 互斥是进程内、非可重入；嵌套调用同一 family 会自锁（当前 hostexec/file-write 无此路径）
- Git worktree：默认隔离是分层路径锁（agent 工作区通常不是 git）。仅当工作区已是 git 仓时，LLM 文件写经 `wrapFileToolSetWithWorktree` 提交合并。`ParallelToolExecutor` 的 worktree 只服务 raw handler（`IsolationStrategy=worktree`）；装配后的工具用 `BatchExecuteAssembledTools`，避免第二套 worktree。`executeOne` 在 isolator handler 为空时把调用方 handler 放到 worktree 目录（ctx）里执行。

### 7.1.2 工具结果卸载（Result Offloading）

**背景**：P0-D 的不可逆截断会永久丢失超预算结果（bff43a17 事故：28.9KB 交付物被截断信封包装导致 StateDelta 解析失败）。对齐业界 SOTA（Anthropic Context Editing、Manus full/compact 双表示、DeepAgents FilesystemMiddleware eviction），超限时**优先卸载而非截断**——数据永不丢弃，上下文中只留可回读引用。

**决策分流**（`ToolDecorator.truncateResult` 内，超预算后按序判定）：

| 顺序 | 条件 | 行为 |
|------|------|------|
| 1 | StateDelta 工具 | 原样放行（既有豁免，不变） |
| 2 | 卸载排除清单（`read_file`） | 回退截断信封（防"读文件→卸载→需再读文件"递归回归） |
| 3 | invocation + artifact service + session IDs 完整 | **卸载**：全量 JSON 存 artifact，返回卸载信封 |
| 4 | 以上均不满足（无 session/服务不可用/保存失败） | 回退现有截断信封，行为与旧版完全一致 |

**卸载信封**（进 LLM 上下文，替代截断信封）：

```json
{
  "offloaded": true,
  "ref": "artifact://tool_results/<tool>/<sha256(args)[:16]>.json@<version>",
  "tool": "<工具名>",
  "original_size": 28912,
  "preview_head": "<前 2KB>",
  "preview_tail": "<后 512B>",
  "read_hint": "Result too large for context. Full JSON saved to ref. Use read_file with file_name=ref and start_line/num_lines to page through it."
}
```

**关键设计决策**：

- **存储复用**：经 `codeexecutor.SaveArtifactHelper` 写入现有 artifact 服务，session 级隔离（AppName/UserID/SessionID），天然多租户安全；不引入新表、新存储、新配置。
- **读回复用**：`read_file` 已支持 `artifact://` 引用 + `start_line/num_lines` 分页，LLM 按 `read_hint` 自取回，零新工具。
- **确定性命名**：`tool_results/<tool>/<sha256(args)[:16]>.json`，同工具同参数幂等复用，防存储膨胀。
- **双端预览**：head 2KB + tail 512B（尾部常含结论/错误码），多数浅层问题无需读回。
- **上下文确定性**：同一结果生成同一信封，append-only，不破坏 KV-cache 前缀。
- **可观测性**：卸载记 Info 日志（tool/original_size/ref）；回退记 Warn 日志。

**后续迭代（P1，未实施）**：Anthropic `clear_tool_uses` 式过期 tool response 清理（oldest-first 占位符化，保留最近 N 个），位于框架 messages 构建层，需评估 KV-cache 失效权衡，独立迭代。

### 7.1.3 结果大小治理优先级

三层各管各的，禁止二次裁切：

| 层 | 时机 | 对象 | 行为 |
|----|------|------|------|
| **主裁** `ToolDecorator.truncateResult` | 工具 `Call` 返回后 | 已装饰工具 | JSON 超 `ResultBudget` → 卸载信封或截断信封（`map`） |
| **兜底** `NewOutputSizeLimiterHook` | AfterTool priority 60 | **未装饰**字符串结果（Deferred / 部分 MCP） | 超 50k runes 就地截断并追加 `[output truncated:]` |
| **入库** `ToolResultGate` | BeforeModel | 即将进模型的 user/tool content | 超限持久化 blob，与上两层独立 |

兜底层跳过：`truncated`/`offloaded` 信封、`builtinResultBudgetOverrides` 命中的工具（如 `browser_snapshot`、`read_upstream_deliverable`）、已含 `[output truncated:` 的字符串。

### 7.2 工具注册表

文件路径：`internal/tools/tool.go`（类型）、`internal/tools/toolset.go`（Registry + Assemble）

**核心类型**：

```go
// internal/tools/tool.go
type Tool            = trpctool.Tool
type CallableTool    = trpctool.CallableTool
type StreamableTool  = trpctool.StreamableTool
type ToolSet         = trpctool.ToolSet
type Declaration     = trpctool.Declaration
type Schema          = trpctool.Schema

type ToolUseExample struct {
    UserQuery   string          `json:"user_query"`
    ToolName    string          `json:"tool_name"`
    Arguments   json.RawMessage `json:"arguments,omitempty"`
    Explanation string          `json:"explanation,omitempty"`
}

type ToolRegistration struct {
    Name                 string
    Description          string
    Factory              func(ctx context.Context) (Tool, error)       // 单工具工厂
    ToolSetFactory       func(ctx context.Context) (ToolSet, error)    // 工具集工厂
    EnabledByDefault     bool
    Category             string   // filesystem / execution / web / search / ...
    Tags                 []string // 分类标签，支持 RegistryByTag / RegistryByCategory
    RiskLevel            string   // low / medium / high / critical
    RequiresConfirmation bool
    SupportsStreaming    bool
    SupportsConcurrency  bool
    Deferred             bool                 // 延迟加载标记
    Examples             []ToolUseExample     // 工具使用示例（供 prompt 增强）
    Group                string               // 工具分组（如 "file_edit" / "web_search"）
}
```

**AssemblyConfig**（实际结构，子配置分组以符合 AS-COG-01 字段 ≤ 15）：

```go
type AssemblyConfig struct {
    EnabledTools  []string
    DeferredTools []string
    FilesystemDir string
    ShellExec     ShellExecConfig
    Search        SearchConfig
    ClaudeCode    ClaudeCodeConfig
    OpenAPISpecs  []OpenAPISpecConfig
    AgentTools    []AgentToolConfig
    MCP           MCPConfig
    Session       SessionConfig
    Browser       *browser.PlaywrightMCPConfig
    Lg            loggateway.Logger
}

type ShellExecConfig struct {
    Dir string
    Env map[string]string
}

type SearchConfig struct {
    GeminiModel  string
    GoogleAPIKey string
    GoogleCX     string
}

type ClaudeCodeConfig struct {
    Dir              string
    ReadOnly         bool
    MaxFileSize      int64
    WebFetch         *WebFetchConfig
    WebSearch        *WebSearchConfig
    CommandAllowList []string
}

type MCPConfig struct {
    Servers []MCPServerConfig
    Broker  *MCPBrokerConfig
}

type SessionConfig struct {
    MemoryEnabled    bool
    MemoryTools      []Tool
    CustomTools      []Tool
    OutboundRouter   *outbound.Router
    SubAgentService  *subagenttool.Service
    BlobReader       biz.ToolResultBlobReader
}

type AssembledToolsets struct {
    ToolSets        []ToolSet
    Tools           []Tool
    DeferredManager *deferred.DeferredToolManager
}
```

**Registry() 注册的工具**（31 项）：

| 注册名 | Category | 类型 | 风险 | 默认启用 | Deferred | Group | 框架包 |
|--------|----------|------|------|----------|----------|-------|--------|
| `file` | filesystem | ToolSet | low | ✅ | — | file_edit | `trpc-agent-go/tool/file` |
| `hostexec` | execution | ToolSet | critical | ❌ | — | — | `trpc-agent-go/tool/hostexec` |
| `httpfetch` | web | Tool | medium | ❌ | — | web_search | `trpc-agent-go/tool/webfetch/httpfetch` |
| `geminifetch` | web | Tool | medium | ❌ | — | — | `trpc-agent-go/tool/webfetch/geminifetch` |
| `duckduckgo` | search | Tool | medium | ❌ | — | web_search | `trpc-agent-go/tool/duckduckgo` |
| `google_search` | search | ToolSet | medium | ❌ | — | web_search | `trpc-agent-go/tool/google/search` |
| `arxiv_search` | search | ToolSet | low | ❌ | — | — | `trpc-agent-go/tool/arxivsearch` |
| `wikipedia` | search | ToolSet | low | ❌ | — | — | `trpc-agent-go/tool/wikipedia` |
| `email` | communication | ToolSet | high | ❌ | — | — | `trpc-agent-go/tool/email` |
| `message` | communication | ToolSet | high | ❌ | — | — | OutboundRouter（统一消息发送） |
| `todo` | productivity | Tool | low | ❌ | — | — | `trpc-agent-go/tool/todo` |
| `await_user_reply` | interaction | Tool | low | ❌ | — | — | `trpc-agent-go/tool/awaitreply` |
| `claudecode` | coding | ToolSet | critical | ❌ | — | file_edit | `trpc-agent-go/tool/claudecode` |
| `workspace_exec` | execution | Tool | critical | ❌ | — | — | CodeExecutor 路径（未独立挂载） |
| `openapi` | integration | ToolSet | medium | ❌ | — | — | `trpc-agent-go/tool/openapi` |
| `agent` | composition | — | medium | ❌ | — | — | 运行时通过 AgentToolConfig 注入 |
| `mcp` | integration | — | medium | ❌ | — | — | 运行时通过 MCPServerConfig 注入 |
| `mcpbroker` | integration | — | medium | ❌ | — | — | 运行时通过 MCPBrokerConfig 注入 |
| `model_registry_sync` | system | — | medium | ❌ | — | — | 仅元数据，无 Factory |
| `subagents_spawn` | composition | — | medium | ❌ | — | — | 仅元数据，运行时通过 SubAgentService 注入 |
| `subagents_list` | composition | — | low | ❌ | — | — | 仅元数据 |
| `subagents_get` | composition | — | low | ❌ | — | — | 仅元数据 |
| `subagents_cancel` | composition | — | medium | ❌ | — | — | 仅元数据 |
| `browser` | browser | ToolSet | critical | ❌ | — | — | Playwright MCP 桥接 |
| `read_document` | media | — | medium | ✅ | — | — | 文档读取 |
| `read_spreadsheet` | media | — | medium | ✅ | — | — | 表格读取 |
| `read_lints` | filesystem | Tool | low | ✅ | — | — | 改后诊断：空 path 读 `.aranea/edited-paths.txt`；Go `go vet` / Python `py_compile` / JS `node --check`；结果不缓存 |
| `delete_file` | filesystem | Tool | medium | ✅ | — | — | 工作区单文件删除；拒绝目录 / `.git` / 符号链接 / 工作区外；Exclusive，不重试 |
| `read_tool_result` | system | Tool | low | ✅ | ✅ | — | 延迟工具结果读取 |
| `working_memory` | memory | ToolSet | low | ✅ | — | — | `internal/tools/working_memory` |
| `deliverable` | team | ToolSet | low | ❌ | — | — | `deliverabletools.ToolSet`（set/get_deliverable 跨 Agent 交付） |
| `datetime` | system | Tool | low | ✅ | — | — | 内置时间工具 |
| `media` | media | Tool | medium | ✅ | — | — | 媒体生成（文生图/文生视频/图生视频）；Factory 返回 nil，经 MediaProvider 在 Agent 级装配 |

**Assemble 流程**：

1. `ValidateRuntimeAliasesAgainstPolicy`：校验 runtime alias 与 policy alias 一致
2. 遍历 `Registry()`，按 `EnabledTools` 列表过滤
3. 对每个注册项，调用 `Factory` 或 `ToolSetFactory` 创建工具实例
4. 对需要额外配置的工具（file、geminifetch、google_search、claudecode），用配置覆盖默认实例
5. 处理 OpenAPI spec → `openapi.NewToolSet`
6. 处理 workspace_exec → 仅 `WithCodeExecutor` 路径启用
7. 处理 AgentTool → `agent.NewTool`
8. 处理 MCP Server → `mcp.NewMCPToolSet`
9. 处理 MCP Broker → `mcpbroker.New`
10. 处理 DeferredTools → 构建 `DeferredToolManager`
11. 追加 CustomTools / MemoryTools / SubAgentTools

### 7.3 桥接层

文件路径：`internal/tools/trpc/toolsets.go`

`ToolsetConfig` → `AssemblyConfig` 的适配层，供 `trpc_build.go` 调用：

```go
type ToolsetConfig struct {
    Filesystem       bool
    FilesystemDir    string
    ShellExec        bool
    ShellExecDir     string
    ShellExecEnv     map[string]string
    WebFetch         bool
    WebSearch        bool
    WebResearch      bool
    WebResearchCfg   webresearchpkg.Config
    GeminiFetch      bool
    GeminiModel      string
    GoogleSearch     bool
    GoogleAPIKey     string
    GoogleCX         string
    ArxivSearch      bool
    Wikipedia        bool
    Email            bool
    Todo             bool
    AwaitReply       bool
    AwaitHook        ReplyFunc          // 阻塞式等待回调
    ClaudeCode       bool
    ClaudeCodeDir    string
    OpenAPISpecs     []OpenAPISpecConfig
    WorkspaceExec    bool
    AgentTools       []AgentToolConfig
    MCPServers       []MCPServerConfig
    MCPBroker        *MCPBrokerConfig
    CustomTools      []trpctool.Tool
    KnowledgeSearch  bool
    KnowledgeReflect bool
    CallAgent        bool
    Kanban           bool
    KanbanBridge     kanbanpkg.Bridge
    MemoryEnabled    bool
    MemoryTools      []trpctool.Tool
    DeferredTools    []string
    BlobReader       biz.ToolResultBlobReader
    ReadDocument     bool
    ReadSpreadsheet  bool
    ReadLints        bool
    DeleteFile       bool
    WorkingMemory    bool
    Datetime         bool
    OutboundRouter   *outbound.Router
    SubAgentService  *subagenttool.Service
}

func BuildToolsets(ctx context.Context, cfg ToolsetConfig, lg loggateway.Logger) (*AssembledToolsets, error)
```

**关键映射**：`ToolsetConfig` 的布尔字段映射到 `AssemblyConfig.EnabledTools` 列表中的注册名。

**特殊处理**：
- `AwaitReply + AwaitHook != nil` → 使用 `serviceawaitreply.New()` 替代框架内置工具
- `KnowledgeSearch` → `knowledgepkg.NewSearchTool()` 追加到 CustomTools
- `CallAgent` → `a2a.NewCallAgentTool()` 追加到 CustomTools
- `Kanban` → `kanbanpkg.NewToolset(cfg.KanbanBridge)` 追加到 ToolSets

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
| `minimal` / `safe` / `system_admin` / `spirit` | 扩展 profile（见开发计划 Phase 8） |

**计算语义**：
- `tools.enabled=true`（catalog 行）= "默认开放"：profile 门控通过且不在 deny 列表即可用
- `tools.enabled=false` = "仅显式允许"：必须在 allow 列表中才可用
- Deny 列表和 `ToolsEnabled=false` 全局开关覆盖一切

**Tool Group 展开**（节选）：

```go
var toolGroupsFilesystem = []string{
    "read_file", "read_multiple_files", "save_file", "list_file",
    "search_file", "search_content", "replace_content",
    "diff_edit", "patch_file", "read_lints", "delete_file",
}
var toolGroupsWeb       = []string{"duckduckgo_search", "web_fetch", "gemini_web_fetch", "google_search", "arxiv_search", "wikipedia_search"}
var toolGroupsMemory    = []string{"memory_search", "memory_get"}
var toolGroupsSkill     = []string{"skill_search", "use_skill"}
var toolGroupsMedia     = []string{"read_image", "read_document", "create_image", "tts"}
var toolGroupsRuntime   = []string{"shell_exec", "claude_code", "workspace_exec"}
var toolGroupsMessaging = []string{"send_email"}
var toolGroupsSession   = []string{"await_user_reply", "todo_write"}
```

### 7.5 工具注入与回调

文件路径：`internal/agent/trpc_build.go`、`internal/agent/tool_invocation_recorder.go`

**buildToolsetsForAgent**：

```go
func buildToolsetsForAgent(ctx context.Context, ag biz.Agent, deps TRPCBuilderDeps, plan *toolBuildPlan) (*tooltrpc.AssembledToolsets, error)
```

`toolBuildPlan` 打包每次构建只加载一次的 eff 键集、工具构建目录快照（`toolBuildCatalog`，2 条批量 SQL）与确认门，由 `BuildTRPCLLMAgent` 构建并共享给回调链，替代原先三处各自逐键 `GetTool` 聚合查询（70 工具 × 3 ≈ 210 次，冷构建约 10s）。快照降级语义：批量加载失败 runtime config 全跳过（fail-soft）、确认门 fail-closed；工具行缺失 fail-closed。eff 映射到 `ToolsetConfig` 的布尔字段，调用 `BuildToolsets`。

**工具记录回调**（`internal/agent/callback_chain.go`）：

记录器已迁移至产品回调链装配：`productCallbackChainWithRegistry` 在 `ag.Settings.ToolsEnabled` 时注册 `callbacks.NewToolRecorderCallback(50, ...)`，工具执行完成后异步记录调用：

```go
entries = append(entries, callbacks.NewToolRecorderCallback(50, func(ctx context.Context, args *trpctool.AfterToolArgs) (*trpctool.AfterToolResult, error) {
    recordToolInvocationAfter(ctx, args, ag, deps)
    return &trpctool.AfterToolResult{}, nil
}))
```

**recordToolInvocationAfter**（`internal/agent/tool_invocation_recorder.go`）：

- 从 `trpcagent.InvocationFromContext(ctx)` 提取 sessionID、userID、agentKey
- `previewFromArgs` / `previewFromResult` 截断至 2000 字符
- 使用 `safego.Go` 异步写入 `ToolInvocationWrite`（含 `Streaming` / `ChunkCount`）

**buildToolFilter**：

```go
func buildToolFilter(s *biz.AgentRuntimeSettings) trpctool.FilterFunc
```

从 `ToolsDenyJSON` 构建 `tool.NewExcludeToolNamesFilter`，注入到 `llmagent.WithToolFilter`。

**buildToolRetryPolicy**：

```go
func buildToolRetryPolicy(s *biz.AgentRuntimeSettings) *trpctool.RetryPolicy
```

从 `AgentRuntimeSettings` 读取重试配置（MaxAttempts、InitialInterval、BackoffFactor、MaxInterval、Jitter），注入到 `llmagent.WithToolCallRetryPolicy`。默认 `ToolsRetryEnabled=true`；`RetryOn` 为产品层 `tools.SelectiveRetryOn`（`DefaultRetryOn` ∪ 结果级/包装瞬态失败，再 ∩ `IsRetryableTool`）：ConcurrentSafe 可重试瞬时网络/EOF、duckduckgo 等 `%v` 包装超时、以及 `web_fetch` 结果内 HTTP 429/5xx（装饰器 `FlagTransientResult` 实现 `RetryResultError`，避免框架把 `(result, nil)` 当成功）。hostexec 与文件写永不重试。Ent `tools_retry_enabled` / `tools_parallel_enabled` 与前端表单新建默认均为 true；存量 false 行由 DDL `20261228` 一次性翻成 true。

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
├── tool.go                          — 项目级工具类型别名 + ToolRegistration + RegistryByTag/RegistryByCategory
├── toolset.go                       — Registry() + AssemblyConfig + Assemble() + 子装配器
├── doc.go                           — 包文档
├── alias/
│   └── alias.go                     — RuntimeToolNameAliases（重导出，破除 import 循环）
├── trpc/
│   ├── toolsets.go                  — ToolsetConfig → AssemblyConfig 桥接层
│   ├── effective_config.go          — Effective tool keys → ToolsetConfig 映射
│   ├── runtime_config.go            — 运行时配置解析
│   ├── confirmation.go              — 工具确认策略
│   └── toolset_prune.go             — 工具集裁剪
├── cache/                           — 工具结果缓存（LRU 驱逐 + TTL）
├── custom/                          — 自定义工具实现
├── hostexecnorm/                    — 主机执行参数归一化（cmd/cwd/working_dir/timeout → schema；command 数组与 args）
├── filenorm/                        — 文件工具参数归一化（path/content/search_content/行号 → schema）
├── argnorm/                         — 独立工具参数归一化（web_fetch url→urls；搜索 q→query）
├── kanban/                          — 看板工具集（Bridge 拆分为 Reader/Writer/Lifecycle）
├── knowledge/                       — Knowledge 搜索工具
├── mcpobserve/                      — MCP 运行时可观测性
├── memory/                          — Memory 工具
├── preview/                         — 工具调用预览脱敏
├── serviceawaitreply/               — 服务级 await_user_reply（阻塞式）
├── skillrouter/                     — Skill 检测与分类
├── skillruntime/                    — Skill 工具集解析（filterCache LRU + RWMutex）
├── subagent/                        — 子代理工具服务
├── testexec/                        — 工具在线测试
├── webresearch/                     — Web 搜索工具
├── working_memory/                  — 工作记忆工具集
├── outbound/                        — 统一消息发送工具
├── deferred/                        — 延迟工具加载机制
└── browser/                         — 浏览器自动化（Playwright MCP 桥接）
```

### 7.8 工具工作区统一（Phase 5）

**目标**：Cursor 式单一项目根——需要「目录」的运行时工具共用 `workspace_root`；装配层一次解析、多处注入。

#### 7.8.1 解析链

文件路径：`internal/agent/tool_assembly.go`（`resolveToolWorkspaceRoot`）

```text
Tool / Override config: filesystem_dir | base_dir | working_dir | root_dir
  → system_settings.root_directory
  → env ARANEA_WORKSPACE_ROOT | WORKSPACE_ROOT
  → {root}/workspace/{agent_key}
  → mkdir 校验
= workspace_root
```

`buildToolsetsForAgent` 在 desktop 联调与桌面应用（Tauri）打包下语义相同：均解析为 **本机绝对路径**。

`applyToolWorkspaceDirs`：一次解析 `workspace_root` → 同时赋 `FilesystemDir`、`ShellExecDir`；`ClaudeCodeDir` 空则回退。

#### 7.8.2 工具 × 目录矩阵

| 注册名 | Catalog / 运行时 | 需要工作区？ | 现状 |
|--------|------------------|-------------|------|
| `file` | `read_file` … `patch_file`；`search_content` 优先 ripgrep（`-A/-B/-C`/`type`/分页） | ✅ 严格 | `FilesystemDir` = `workspace_root` ✅ |
| `read_lints` | `read_lints` | ✅ 严格 | 与 file 共用 `FilesystemDir`；空 path → editstamp |
| `delete_file` | `delete_file` | ✅ 严格 | 与 file 共用 `FilesystemDir`；`document.ValidatePath` |
| `hostexec` | `shell_exec` → `exec_command` | ✅ 默认 cwd | `ShellExecDir` = `workspace_root`；会话输出 `.aranea/shell/`；`pid` / `running_for_ms` / `hung`；`write_stdin` 可 await |
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
| `internal/tools/toolset.go` | `AssemblyConfig.ShellExec.Dir/Env` → hostexec 包装层 |
| `internal/tools/hostexec/toolset.go` | `Dir` 通过 `WithBaseDir` 设默认 cwd；`Env` 通过 `WithBaseEnv` 注入每个子进程，并由返回值包装层脱敏 |
| `internal/tools/trpc/toolsets.go` | `ToolsetConfig.ShellExecDir` / `ShellExecEnv`；Build 时写入 `AssemblyConfig` |
| `internal/agent/tool_assembly.go` | `resolveToolWorkspaceRoot` + `applyToolWorkspaceDirs` |
| `internal/tools/trpc/runtime_config.go` | `shell_exec` config 支持 `base_dir` / `shell_root` |
| `internal/data/builtin_tools_seed.go` | `shell_exec` 参数改为 `workdir`（与 hostexec 一致） |
| `internal/tools.NormalizeInvocation` | **唯一参数归一化入口**（装饰器 Call 前、worktree 内层、锁/缓存所见均为归一后 JSON）。实现仍分 `hostexecnorm` / `filenorm` / `argnorm`。别名重写记 `AliasRewriteTotal` + Debug `tool.args.normalized` |
| `internal/tools/hostexecnorm` | `cmd`/`cmd_line` → `command`；argv 数组与 `args`/`argv` 拼成字符串（含空格的数组元素加引号）；`working_dir`/`cwd`/`dir` → `workdir`；`timeout` → `timeout_sec` |
| `internal/tools/filenorm` | `path`/`file` → `file_name`（含 `delete_file`）；`content`/`body` → `contents`（save_file）；`old`/`new` → `old_string`/`new_string`（replace_content）；`dir` → `path`（list_file）；`glob` → `pattern`（search_file）；`query`/`pattern`/`glob` → `content_pattern`/`file_pattern`（search_content）；`-A`/`-B`/`-C`/`type`/`multiline`/`head_limit`/`offset`（search_content）；`start`/`limit` → `start_line`/`num_lines`（read_file） |
| `internal/tools/argnorm` | `web_fetch`：`url` → `urls[]`；搜索类：`q`/`search`/`keyword` → `query`；`gemini_web_fetch`：`url` → `prompt` |
| `internal/tools.BatchExecuteAssembledTools` | Spirit/批执行走已装饰 `Call`；清除 IsolationStrategy 且不复制 Wire worktree |
| `internal/tools/testexec` / `graph/adapter` | Assemble 后 `ApplyDefaultDecorators`，与 LLM 同一 Call 路径 |
| `internal/tools/testexec/config.go` | 在线测试 shell 时传入 workspace |

#### 7.8.4 Shell 参数与确认

| 项 | 设计 |
|----|------|
| 调用参数 | `command`（必填）、`workdir`（可选，相对 `workspace_root`） |
| 兼容 | `hostexecnorm` 将 `cmd`/`cwd`/`working_dir`/`timeout` 映射为 `command`/`workdir`/`timeout_sec`；`command` 可为字符串数组，并可与 `args` 拼接 |
| 环境变量 | `ShellExec.Env` 作为 base env 注入命令；单次调用 `env` 可覆盖同名值 |
| 结果脱敏 | 配置环境变量中的敏感名称/值在结构化结果返回前替换为 redaction marker |
| 确认门控 | `tool_confirm_gate` 同时匹配 `shell_exec` 与 `exec_command`（runtime alias） |
| Prompt | `RuntimeCapabilityCue` 表述默认 cwd=工作区 |

#### 7.8.5 工具授权决策链

基于 Grok 的 8 级决策链借鉴，实现三层授权机制：

**决策链顺序**：

```
1. 默认允许（catalog/plugin 均不需要确认）→ 直接执行
2. 持久化授权（persisted grant）→ 直接执行（记录 decision_reason）
3. 会话授权（session grant）→ 直接执行（记录 decision_reason）
4. catalog 需要确认 → 弹窗提示（记录 decision_reason）
5. plugin 需要确认 → 弹窗提示（记录 decision_reason）
```

**授权类型**：

| 类型 | 存储 | 生命周期 | 作用域 |
|------|------|---------|--------|
| 单次批准 | 无 | 当前工具调用 | (agentID, toolKey) |
| 会话授权 | 内存（sync.Map） | 会话结束 | (sessionID, agentID, toolKey) |
| 持久化授权 | 数据库（tool_grants 表） | 跨会话 | (agentID, toolKey) |

**关键文件**：

| 文件 | 职责 |
|------|------|
| `internal/tools/serviceawaitreply/tool_confirm.go` | 四态确认回复常量（approve/deny/approve_session/approve_always） |
| `internal/agent/tool_grant_store.go` | 会话级授权存储（TTL 惰性清理） |
| `internal/data/ent/schema/tool_grant.go` | 持久化授权 Schema |
| `internal/data/tool_grant.go` | 持久化授权 Repo 实现 |
| `internal/biz/tool/tool_grant.go` | 持久化授权 Biz 层 |
| `internal/agent/tool_confirm_gate.go` | 决策链核心逻辑（decide 方法） |
| `internal/agent/tool_confirmation.go` | 确认回复处理与授权副作用 |

**前端四按钮确认卡片**：

| 按钮 | 回复 Token | 授权效果 |
|------|-----------|---------|
| 允许本次 | `__aranea:tool_confirm:approve` | 仅批准当前调用 |
| 拒绝 | `__aranea:tool_confirm:deny` | 拒绝并取消工具执行 |
| 会话内始终允许 | `__aranea:tool_confirm:approve_session` | 批准 + 会话级授权 |
| 始终允许 | `__aranea:tool_confirm:approve_always` | 批准 + 持久化授权 |

#### 7.8.6 与桌面 App 打包

App 壳、桌面应用打包（Tauri）**不在本模块范围**（曾起草编号 53 文档，**不实施**）。工作区路径仍通过系统设置 / 环境变量 / Tool 配置注入，与是否打包为桌面应用无关。

### 7.9 延迟工具机制（Deferred Tools）

部分工具（如 `read_tool_result`）标记为 `Deferred: true`，不随 Agent 初始化时立即装配，而是通过延迟加载机制按需实例化。

**核心流程**：

```
Assemble()
  → 识别 Deferred: true 的注册项
  → 构建 DeferredToolEntry 列表
  → 创建 DeferredToolManager
    → ToolSearchTool（搜索可用延迟工具）
    → DeferredCallableTool（按需实例化并调用）
  → 追加到 AssembledToolsets.Tools
```

**spirit 档闲聊常驻 vs deferred（2026-08-19）**：WP-4 核心集是常驻白名单。`SplitCoreResidentTools("spirit")` 只留 `plan_and_execute` / `datetime` / `memory_search` / `memory_remember`。收口类 `synthesize_results` / `get_team_deliverable` / `cancel_orchestration` 与构图、computer_use、shell、M71 会话考古（`search_messages` / `list_agent_sessions` / `read_session_history`）一律 deferred。catalog `eff` 之外的 MemoryTools / working_memory / 扁平 CustomTool 不再靠手写侧通道名单，而是 `MergeNonCoreMappedDeferred`：所有已映射且不在核心集的 biz key 并进 deferred；`FinalizeDeferredTools` 只包装实际装配到的项。扁平 CustomTool 必须映射到自身 Declaration 名。`ToolFilter` names 同时收录 catalog `Name` 与 `BaseName`。目录 cue 只保留首句且 ≤80 字，避免 deferred 变多后尾部 token 把 schema 节省吃掉。框架 skill 工具在 llmagent 装配之后注入，不能靠 registry 映射包装：spirit/chat_only 用 `WithAllowedSkillTools(skill_load)`，去掉 `skill_select_docs` / `skill_list_docs`。`CAPABILITIES.md` 只描述常驻工具，其余写明先 `tool_load`。

**同一目标禁止重复规划（2026-08-22）**：`plan_and_execute` 在分解前 `ListAllTeams`；若本会话已有与 `task_prompt` 重叠的 running/completed 团队，直接返回 `reuse_existing=true` + `existing_teams` + `next_action`（先 `tool_load get_team_deliverable`），不跑 LLM 分解。用户明确「重新组建」或 `force_new=true` 才开新 DAG。流式分解不再套 `DecomposeLLMTimeout=60s` 子超时（idle 45s + 外层 3min）。

### 7.10 子代理工具（SubAgent Tools）

子代理工具通过 `SubAgentService` 注入，支持运行时动态生成、列表、查询和取消子代理。

**注入路径**：

```
AssemblyConfig.Session.SubAgentService
  → SubAgentService.FrameworkTools()
  → 按 enabled 列表过滤
  → 追加到 AssembledToolsets.Tools
```

**4 个子代理工具**：`subagents_spawn`、`subagents_list`、`subagents_get`、`subagents_cancel`。`subagents_spawn` 接受 `kind`/`subagent_type`（explore|verify|general）以注入不同系统提示；`subagents_get` 接受 `block_until_ms` 等到终态；列表/获取结果带 `running_for_ms`。

### 7.11 Runtime Alias 与 Policy Alias

**Runtime Alias**（`internal/tools/alias/alias.go`）：LLM 调用工具时使用的别名 → 框架工具 Declaration().Name 的映射。

```go
var RuntimeToolNameAliases = map[string]string{
    "write_file":       "save_file",
    "edit_file":        "diff_edit",
    "list_files":       "list_file",
    "workspace_search": "search_content",
    "shell":            "shell_exec",
    "shell_exec":       "exec_command",
    "todo":             "todo_write",
    "gemini_fetch":     "gemini_web_fetch",
    "wikipedia":        "wikipedia_search",
    "email":            "send_email",
    "await_reply":      "await_user_reply",
    "web_search":       "web_research",
}
```

**Policy Alias**（`internal/biz/tool/tool_policy_keys.go`）：UI/API/legacy 名 → catalog tool_key 的映射，用于 effective tool 策略计算。

**铁律**：两份 alias map 必须保持一致（TPM-P1-01）。`ValidateRuntimeAliasesAgainstPolicy` 在 Assemble 启动时校验。`PropagateAllowAliases` / `PropagateDenyAliases` 处理链式别名（如 `shell` → `shell_exec` → `exec_command`）。**allow 与 deny 均为双向传播**（2026-08-14 BUG-1）：别名表定义的是等价类，给/禁任一名称即给/禁整个类；此前 allow 仅单向（alias→canon），用户写 canonical key（如 `shell_exec`）时传播断裂导致工具不可见。

---

## 七-B、LLM Provider Tool Calling 适配

> **定位**：trpc-agent-go 运行时中各 LLM Provider 对 `tool.Declaration` → API 请求格式的转换规范

### 1. Provider 适配架构

```
tool.Declaration (InputSchema *Schema)
  → 各 Provider 的 convertTools() 函数
  → OpenAI:  tools[].function.parameters
  → Anthropic: tools[].input_schema
  → Gemini:   functionDeclarations[].parametersJsonSchema
  → Ollama:   tools[].function.parameters
```

### 2. SanitizeToolName 统一规则

所有 Provider 的 `convertTools()` 必须对 `declaration.Name` 调用 `tool.SanitizeToolName()`，将非法字符替换为下划线。此规则确保工具名在所有 LLM API 中兼容（`^[a-zA-Z0-9_-]+$`）。

| Provider | 位置 | 状态 |
|----------|------|------|
| OpenAI | `model/openai/openai.go` | ✅ |
| Anthropic | `model/anthropic/anthropic.go` | ✅ |
| Gemini | `model/gemini/gemini.go` | ✅ |
| Ollama | `model/ollama/ollama.go` | ✅ |

### 3. nil InputSchema 防护

`tool.Declaration.InputSchema` 类型为 `*Schema`（指针），可以为 nil。无参数工具（如 `transfer_to_agent`）可能不设置 InputSchema。

**规则**：所有 Provider 的 `convertTools()` 必须在访问 `InputSchema` 前做 nil 检查，nil 时提供默认空 object schema。

### 4. JSON Schema `required` 字段映射

JSON Schema 中 `required` 是顶层字段（`InputSchema.Required`），列出哪些属性名是必填的。**不得**使用 `prop.Required`（嵌套对象的子级 required 列表）。

| Provider | 映射方式 | 状态 |
|----------|---------|------|
| OpenAI | `declaration.InputSchema.Required` | ✅ |
| Anthropic | `declaration.InputSchema.Required` | ✅ |
| Gemini | `normalizeToolSchema` 整体序列化 | ✅ |
| Ollama | `decl.InputSchema.Required` | ✅（原错误使用 `prop.Required` 已修复） |

### 5. Tool Result 回传格式

| Provider | 回传方式 | 注意事项 |
|----------|---------|---------|
| OpenAI | `role: "tool"`, `tool_call_id` 匹配 | 标准格式 |
| Anthropic | `type: "tool_result"` block 嵌入 user 消息 | `tool_use_id` 匹配 |
| Gemini | `FunctionResponse` part（role=user） | 按 Name + 位置匹配，需确保 ToolName 非空 |
| Ollama | 降级为 `role: "user"` 消息 | 不支持 tool role，需嵌入 `[Tool Result: name (id: xxx)]` 前缀 |

### 6. buildDefaultToolMessage 规范

`buildDefaultToolMessage(toolCallID, toolName, result)` 必须同时填充 `ToolID` 和 `ToolName`，确保下游 Provider（特别是 Gemini）能正确构建 `FunctionResponse.Name`。

### 7. 工具调用成功率增强

> **定位**：基于竞品调研（Pydantic AI / Instructor / LangChain / Vercel AI SDK / Dify）的最优实践，提升 LLM 调用工具的成功率

#### 7.1 JSON 参数修复默认开启

**问题**：`ToolCallArgumentsJSONRepairEnabled` 默认关闭，小模型/开源模型频繁生成不合法 JSON 参数导致工具调用失败。

**方案**：将 `IsToolCallArgumentsJSONRepairEnabled` 默认值从 `false` 改为 `true`。`*bool` 三态设计（nil=默认启用, true=显式启用, false=显式禁用）保留用户完整控制权。

#### 7.2 Ollama OutputSchema 描述格式统一

**修复**：`desc += "Output schema: "` → `desc += "\nOutput schema: "`，与 OpenAI/Anthropic 保持一致。

#### 7.3 Ollama Schema 约束迁移到描述

**问题**：Ollama SDK 不支持 `additionalProperties`/`default`/嵌套 `required`/`$ref`，以及新增的约束字段（`minLength`/`maxLength`/`pattern`/`minimum`/`maximum`/`minItems`/`maxItems`）。

**方案**：新增 `appendSchemaConstraintsToDescription` 函数，将 Ollama SDK 不支持的约束以自然语言追加到属性描述中。

#### 7.4 JSON 解析错误增强反馈

**方案**：新增 `enhanceJSONParseError` 函数，在 JSON 解析错误时将 InputSchema 追加到错误消息中。使用类型化错误匹配（`json.SyntaxError`/`json.UnmarshalTypeError`）+ 字符串前缀兜底。

#### 7.5 清洗名查找缓存优化

**方案**：`lookupBySanitizedName` 改为调用 `buildSanitizedNameCache` 构建 map 后 O(1) 查找。

#### 7.6 tool.Schema 约束字段扩展

**方案**：扩展 `Schema` struct 新增 7 个约束字段（全部指针类型 + `omitempty`），更新 `ExtraFields()` 方法将约束字段纳入返回值，新增 `AppendConstraintsToDescription` 公开函数供 Provider 适配层使用。

---

## 八、Web 前端设计

### 8.1 文件结构

```
web/src/
├── features/tools/
│   ├── api.ts                        # API 调用封装
│   ├── types.ts                      # TypeScript 类型定义
│   └── toolUi.ts                     # UI 辅助函数
└── pages/
    ├── ToolsPage.vue                 # 工具目录管理页
    ├── ToolRunsPage.vue              # 工具调用记录页
    └── ToolAuditsPage.vue            # 工具调用审计页
└── components/tools/
    ├── ToolsTable.vue                # 工具列表表格
    ├── ToolsMetricStrip.vue          # Summary 统计卡片
    ├── ToolCatalogFilters.vue        # 筛选卡片
    ├── ToolHeroSection.vue           # 顶部 Hero 区域
    ├── ToolGlassPanel.vue            # 玻璃质感面板
    ├── ToolJsonBlock.vue             # JSON 展示块
    ├── ToolSchemaForm.vue            # Schema 表单
    ├── ToolDetailDrawer.vue          # 详情抽屉（容器）
    ├── ToolDetailParamsPanel.vue     # 详情-参数面板
    ├── ToolDetailConfigPanel.vue     # 详情-配置面板（可编辑）
    ├── ToolEditorDialog.vue          # 编辑弹窗（容器）
    ├── ToolOverrideEditorDialog.vue  # Override 编辑弹窗
    ├── ToolRunsTable.vue             # 调用记录表格
    ├── ToolRunsFilters.vue           # 调用记录筛选
    ├── ToolAuditsTable.vue           # 审计日志表格
    ├── ToolAuditsFilters.vue         # 审计日志筛选
    └── editor/
        ├── ToolSchemaBuilder.vue     # Schema 构建器
        ├── ToolEditorHelpDrawer.vue  # 编辑帮助抽屉
        └── ToolFieldHintInput.vue    # 字段提示输入
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
  source: 'builtin' | 'mcp' | 'system' | 'external' | string;
  risk_level: 'low' | 'medium' | 'high' | 'critical' | string;
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
  runtime_status?: 'available' | 'registered_only' | 'disabled' | string;
  runtime_kind?: 'function' | 'streaming' | 'approval' | string;
  invoke_count: number;
  invoke_count_24h: number;
  success_count: number;
  failure_count: number;
  blocked_count: number;
  agent_override_count: number;
  avg_duration_ms: number | null;
  p95_duration_ms: number;
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
  sort?: string;
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
  status: 'success' | 'error' | 'blocked' | 'cancelled' | string;
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
  streaming?: boolean;
  chunk_count?: number;
};

export type ToolInvocationAudit = {
  id: string;
  invocation_id: string;
  tool_key: string;
  agent_id: string;
  user_id: string;
  session_id: string;
  action: string;
  result_summary: string;
  status: string;
  source: string;
  created_at: string;
};

export type ToolAuditQuery = {
  tool_key?: string;
  agent_id?: string;
  user_id?: string;
  session_id?: string;
  status?: string;
  from?: string;
  to?: string;
  page?: number;
  page_size?: number;
};

export type ToolRunQuery = {
  tool_key?: string;
  agent_id?: string;
  session_id?: string;
  status?: string;
  from?: string;
  to?: string;
  has_error?: boolean;
  page?: number;
  page_size?: number;
};

export type PaginatedResponse<T> = {
  items: T[];
  page: number;
  page_size: number;
  total: number;
};

export type ToolAgentOverride = {
  id: string;
  tool_id: string;
  tool_key: string;
  agent_id: string;
  enabled: boolean;
  mode: 'inherit' | 'allow' | 'deny' | string;
  config_override_json: string;
  requires_confirmation: boolean;
  created_at: string;
  updated_at: string;
};

// 注意：input 使用 tool_id（与 Proto 一致），而非 tool_key
export type ToolAgentOverrideInput = {
  tool_id: string;
  agent_id: string;
  enabled?: boolean;
  mode?: string;
  config_override_json?: string;
  requires_confirmation?: boolean;
};

export type AgentEffectiveTool = {
  tool_key: string;
  display_name: string;
  category: string;
  source: string;
  enabled: boolean;
  effective_state: 'allowed' | 'denied' | string;
  reason: string;
};

export type AgentEffectiveTools = {
  tools_enabled: boolean;
  profile: string;
  allow: string[];
  deny: string[];
  items: AgentEffectiveTool[];
};

export type ToolTestResult = {
  status: string;
  result_preview: string;
  error_message: string;
  duration_ms: number;
};
```

### 8.3 API 封装

文件路径：`web/src/features/tools/api.ts`

```typescript
// 工具目录 CRUD
export async function listTools(query: ToolListQuery = {}): Promise<ToolListResponse>
export async function getTool(id: string): Promise<Tool>
export async function createTool(input: ToolUpsertInput): Promise<Tool>
export async function updateTool(id: string, input: ToolUpsertInput): Promise<Tool>
export async function deleteTool(id: string): Promise<void>

// 启用/停用（高风险启用需 confirmIntent = 'I_UNDERSTAND_RISK'）
export async function toggleToolEnabled(id: string, enabled: boolean, confirmIntent?: string): Promise<Tool>

// 配置更新
export async function updateToolConfig(id: string, configJson: string): Promise<Tool>

// 在线测试
export async function testTool(toolId: string, argumentsJson = '{}', timeoutSec = 30): Promise<ToolTestResult>

// 调用记录
export async function listToolRunsForTool(id: string, query: ToolRunQuery = {}): Promise<PaginatedResponse<ToolInvocation>>
export async function listToolRuns(query: ToolRunQuery = {}): Promise<PaginatedResponse<ToolInvocation>>

// 调用参数（脱敏）
// 注意：通过 toolApi.GetToolInvocationParams 调用，路径 /v1/tools/runs/{invocation_id}/params

// Agent 工具覆盖（使用 tool_id，与 Proto 一致）
export async function listToolAgentOverrides(toolId: string): Promise<ToolAgentOverride[]>
export async function listToolAgentOverridesByAgent(agentId: string): Promise<ToolAgentOverride[]>
export async function upsertToolAgentOverride(input: {
  tool_id: string;
  agent_id: string;
  enabled?: boolean;
  mode?: string;
  config_override_json?: string;
  requires_confirmation?: boolean;
}): Promise<ToolAgentOverride>
export async function deleteToolAgentOverride(toolId: string, agentId: string): Promise<void>

// 审计日志
export async function listToolInvocationAudits(query: ToolAuditQuery = {}): Promise<PaginatedResponse<ToolInvocationAudit>>

// Agent 生效工具矩阵（通过 agent/v1 API）
export async function getAgentEffectiveTools(agentId: string): Promise<AgentEffectiveTools>
```

### 8.4 Chat Tool 事件流与状态映射

#### 数据流

```
后端 EventProjector.buildToolCallEnvelope()
  → Envelope { type: "tool_call", tool_call: EnvelopeToolCall }
  → WebSocket → 前端 streamHandlers
  → envelopeToToolEvent(env, 'before') → ToolUseEvent { status: 'running' }
  → AgentToolSection.vue 渲染

后端 EventProjector.buildToolResultEnvelope()
  → Envelope { type: "tool_result", tool_call: EnvelopeToolCall }
  → WebSocket → 前端 streamHandlers
  → envelopeToToolEvent(env, 'after') → ToolUseEvent { status: 'success'|'failed'|... }
  → mergeToolEvents() 合并 before+after
  → AgentToolSection.vue 更新
```

#### EnvelopeToolCall 字段映射

后端 `EnvelopeToolCall`（Go）与前端 `EnvelopeToolCall`（TS）字段完全一致，JSON tag 统一使用 snake_case：

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | string | 工具调用 ID |
| `name` | string | 工具名 |
| `arguments_json` | string | 参数 JSON |
| `result_json` | string? | 结果 JSON |
| `status` | string | wire 状态 |
| `duration_ms` | number? | 耗时 |
| `is_long_running` | boolean? | 长运行标记 |
| `activity_kind` | string? | 活动分类 |
| `display_label` | string? | 显示标签 |
| `icon_key` | string? | 图标 key |
| `summary` | string? | 摘要 |
| `started_at` | string? | 开始时间 |
| `finished_at` | string? | 结束时间 |
| `error_code` | string? | 错误码 |
| `agent_key` | string? | Agent key |
| `agent_id` | string? | Agent ID |
| `agent_name` | string? | Agent 名称 |
| `run_id` | string? | 运行 ID |
| `trace_id` | string? | 追踪 ID |

#### Wire Status → Canonical Status 映射

| Wire 值 | Canonical 值 | Message Status |
|---------|-------------|----------------|
| `calling` | `running` | `tool_running` |
| `running` | `running` | `tool_running` |
| `in_progress` | `running` | `tool_running` |
| `success` | `success` | `tool_success` |
| `failed` | `failed` | `tool_failed` |
| `error` | `failed` | `tool_failed` |
| `blocked` | `blocked` | `tool_blocked` |
| `cancelled` | `cancelled` | `tool_cancelled` |
| `interrupted` | `cancelled` | `tool_cancelled` |

**ToolUseEvent.status 类型**：`'running' | 'success' | 'failed' | 'blocked' | 'cancelled' | string`

> 注意：`'error'` 已从类型定义中移除，canonicalToolStatus 将 wire `"error"` 映射为 `'failed'`。

### 8.5 页面组件

#### ToolsPage.vue

文件路径：`web/src/pages/ToolsPage.vue`

**页面结构**：

1. **顶部 Hero 区域**（`ToolHeroSection`）：标题 "工具管理" + Summary 统计卡片（`ToolsMetricStrip`）+ 刷新/新建按钮
2. **筛选卡片**（`ToolCatalogFilters`）：搜索框 + Category 下拉 + Source 下拉 + Risk Level 下拉 + 启用状态筛选
3. **数据表格**（`ToolsTable`）：展示工具列表
4. **新建/编辑弹窗**（`ToolEditorDialog` + `editor/*` 子组件）：4 Tab — 基础 / 运行策略 / 参数与配置 / 高级
5. **详情抽屉**（`ToolDetailDrawer` + `ToolDetailParamsPanel` + `ToolDetailConfigPanel`）：5 Tab — 概览 / 参数 / 配置（可编辑，`PUT /v1/tools/{id}/config`）/ Agent / 调用；Agent Tab 含 **生效摘要**

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
| 成功率 | success_count / (success+failure+blocked) | 成功率百分比；副行「一次合法率」= 1 − (repaired_count+invalid_count) / invoke_count（90d 窗口，<95% 标黄，tooltip 列明细） |
| 平均耗时 | avg_duration_ms | 毫秒 |
| 启用 | enabled | Toggle 开关（高风险启用需 confirm_intent） |
| 操作 | id | 查看/编辑/删除/配置按钮 |

#### ToolRunsPage.vue

文件路径：`web/src/pages/ToolRunsPage.vue`

**页面结构**：

1. **顶部 Hero 区域**：标题 "工具调用记录"
2. **筛选卡片**（`ToolRunsFilters`）：Tool Key + Agent ID + Session ID + Status + 时间范围 + Has Error
3. **数据表格**（`ToolRunsTable`）：展示调用记录列表

**表格列定义**：

| 列名 | 字段 | 说明 |
|------|------|------|
| 工具 | tool_key + tool_display_name | |
| Agent | agent_key + agent_display_name | |
| Session | session_id | |
| 状态 | status | success=绿/error=红/blocked=橙/cancelled=灰 |
| 流式 | streaming + chunk_count | 流式调用标记 + 分片数 |
| 开始时间 | started_at | |
| 耗时 | duration_ms | 毫秒 |
| 输入摘要 | input_preview | 截断展示 |
| 输出摘要 | output_preview | 截断展示 |
| 错误 | error_code + error_message | error 状态时展示 |
| 脱敏 | redaction_applied | 是否已脱敏 |

#### ToolAuditsPage.vue

文件路径：`web/src/pages/ToolAuditsPage.vue`

**页面结构**：

1. **顶部 Hero 区域**：标题 "工具调用审计"
2. **筛选卡片**（`ToolAuditsFilters`）：Tool Key + Agent ID + User ID + Session ID + Status + 时间范围
3. **数据表格**（`ToolAuditsTable`）：展示审计日志列表

**表格列定义**：

| 列名 | 字段 | 说明 |
|------|------|------|
| 工具 | tool_key | |
| Agent | agent_id | |
| User | user_id | |
| Session | session_id | |
| Action | action | 如 `tool.call` |
| 结果摘要 | result_summary | |
| 状态 | status | success=绿/error=红 |
| 来源 | source | 如 `adk` |
| 时间 | created_at | |

### 8.6 路由

| 路径 | 组件 | 说明 |
|------|------|------|
| `/tools` | ToolsPage | 工具目录管理页 |
| `/tools/runs` | ToolRunsPage | 工具调用记录页 |
| `/tools/audits` | ToolAuditsPage | 工具调用审计页 |

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

### 9.2 流式工具执行流程

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

### 9.3 AG-UI 集成

```go
// agui.WithStreamingToolResultActivityEnabled(true)
// 开启后，流式中间结果转为 Activity 事件：
// - ActivityType = "tool.result.stream"
// - ActivityMessageID = "tool-result-activity-" + toolCallID
// 工具结束时仍发一条 TOOL_CALL_RESULT
```

### 9.4 项目集成设计

**tool_invocations 扩展**（已实现）：

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
AssemblyConfig.MCP.Broker
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

// 项目集成：callback_chain.go → NewToolRecorderCallback(50) → recordToolInvocationAfter
entries = append(entries, callbacks.NewToolRecorderCallback(50, func(ctx context.Context, args *trpctool.AfterToolArgs) (*trpctool.AfterToolResult, error) {
    recordToolInvocationAfter(ctx, args, ag, deps)
    return &trpctool.AfterToolResult{}, nil
}))
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

> **状态**：✅ catalog 与运行时工具均已实现
> **需求**：[23-tools.md §子模块 Tools Fragment Edit](./23-tools.md) · **开发计划**：[23-tools.development.md §Phase 4](./23-tools.development.md)

在 §7 运行时层 `file` ToolSet 上扩展两个 CallableTool：

| 工具 | 职责 | 运行时状态 |
|------|------|-----------|
| `diff_edit` | 多片段 SEARCH/REPLACE，内存原子 apply | ✅ 已实现 |
| `patch_file` | unified diff 或结构化 hunk 应用 | ✅ 已实现 |

**catalog 与策略层**：

| 位置 | 状态 | 说明 |
|------|------|------|
| `internal/data/builtin_tools_seed.go` | ✅ | `diff_edit` / `patch_file` seed 已添加（registryName=`file`） |
| `internal/biz/agent_effective_tools.go` | ✅ | `toolGroupsFilesystem` 含 `diff_edit` / `patch_file` |
| `internal/biz/tool/tool_policy_keys.go` | ✅ | `edit_file` → `diff_edit` policy alias |
| `internal/tools/alias/alias.go` | ✅ | `edit_file` → `diff_edit` runtime alias |
| `internal/tools/testexec/config.go` | ✅ | `diff_edit` / `patch_file` 映射到 `file` |
| `internal/tools/trpc/effective_config.go` | ✅ | effective key 映射 |
| `internal/tools/trpc/runtime_config.go` | ✅ | runtime config 映射 |
| `internal/agent/activity_meta.go` | ✅ | `diff_edit` → 片段编辑、`patch_file` → 应用补丁 中文标签 |
| `internal/agent/prompt.go` | ✅ | `diff_edit` / `patch_file` 工作流提示 |

**运行时工具实现**：

| 位置 | 状态 | 说明 |
|----------|------|------|
| `pkg/trpc-agent-go/tool/file/diffedit.go` | ✅ | `diff_edit` 工具：多 edit 内存原子 apply + 结构化错误（`edit_not_unique` / `edit_not_found`） |
| `pkg/trpc-agent-go/tool/file/patchfile.go` | ✅ | `patch_file` 工具：patch/hunks 互斥校验 + 原子写盘 + `hunk_mismatch` 结构化错误 |
| `pkg/trpc-agent-go/tool/file/editcontent.go` | ✅ | load/commit 编排 + SessionFileState + 原子写盘（temp + rename） |
| `pkg/trpc-agent-go/tool/file/patch/` | ✅ | hunk 类型 / apply（drift tolerance）/ unified 解析 / validate |
| `pkg/trpc-agent-go/tool/internal/textfile/` | ✅ | 共享编码 / 行结束 / 引号 fuzzy（claudecode 复用） |
| `pkg/trpc-agent-go/internal/toolcache/file_views.go` | ✅ | per-invocation FileView 存取（`agent.Invocation.State`） |
| `pkg/trpc-agent-go/tool/file/file.go` | ✅ | `diff_edit` / `patch_file` 注册 + `WithDiffEditEnabled` / `WithPatchFileEnabled` |
| `pkg/trpc-agent-go/tool/file/readfile.go` | ✅ | 响应含 `mtime_ms`；读后缓存 FileView |
| `pkg/trpc-agent-go/tool/file/savefile.go` / `replacecontent.go` | ✅ | 写盘后刷新 FileView（读回磁盘解码） |

**实现红线**：

- 实现放在 `pkg/trpc-agent-go/tool/file`，**禁止**在 `internal/biz` import trpc-agent-go 实现编辑逻辑
- `internal/tools` 仅做装配、别名、catalog，不写 patch 算法
- `runtime_alias.go`（实际为 `internal/tools/alias/alias.go`）与 `tool_policy_keys.go`（实际为 `internal/biz/tool/tool_policy_keys.go`）须同步

**详细设计**（与运行时实现一致）：

### 13.1 工具 API

#### `diff_edit`

Declaration name：`diff_edit`

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

#### `patch_file`

Declaration name：`patch_file`

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

### 13.2 SessionFileState

**数据结构**（实现于 `editcontent.go` + `pkg/trpc-agent-go/internal/toolcache/file_views.go`）：

```go
// pkg/trpc-agent-go/internal/toolcache/file_views.go
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

**读写协议**：

| 操作 | 行为 |
|------|------|
| `read_file` | 读盘后 `storeFileViewAfterRead`；响应含 **`mtime_ms`** |
| `diff_edit` / `patch_file` | `loadEditSnapshot` → cache 命中且 mtime 一致则跳过 ReadFile → apply → `commitEditSnapshot` → `storeFileView` |
| `save_file` / `replace_content` | 写盘后 `storeSaveFileView`（读回磁盘再解码，保持 encoding 一致） |
| mtime 不匹配 | 返回 `file_modified_externally`，hint re-read |

### 13.3 错误响应结构

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

### 13.4 Limits 常量

| 常量 | 默认值 |
|------|--------|
| `maxEditsPerCall` | 20 |
| `maxPatchBytes` | 256 KiB |
| `maxEditSearchBytes` | 64 KiB per search block |
| `maxEditReplaceBytes` | 256 KiB per replace block |

### 13.5 性能设计

| 层级 | 手段 | 预期 |
|------|------|------|
| 模型 | 片段参数 | 生成时间主导项，显著下降 |
| 运行时 | SessionFileState | 同文件二次编辑省 1 次 ReadFile |
| 运行时 | 单次多 edit | N 次 tool call → 1 次 |
| 运行时 | 内存 patch | 毫秒级（Go string splice） |
| Phase 2 | 行区间读 | >1MB 文件仅加载 hunk ±context |

### 13.6 安全

- 复用 `resolvePath`、`maxFileSize`、文本校验（与 `read_file` / `replace_content` 一致）
- `maxPatchBytes` / `maxEditSearchBytes` / `maxEditReplaceBytes` 防 DoS
- **同文件勿并行** `diff_edit` / `patch_file`（Invocation 缓存无锁；Prompt + `expected_mtime_ms` 软/硬约束）
- 写操作保留原 `FileMode`
- 不经过 Tool 结果缓存（非幂等）

---

## 十四、Data 层错误处理规范

Data 层统一使用 `kerrors` 返回错误，禁止 `errors.New` / `sql.ErrNoRows`：

| 场景 | 使用 |
|------|------|
| Ent 客户端不可用 | `kerrors.InternalServer("TOOL", "ent client unavailable")` |
| 记录不存在 | `kerrors.NotFound("TOOL", "tool not found")` |
| 参数校验失败 | `kerrors.BadRequest("TOOL", "tool key is required")` |

所有 Repo 方法的数据库错误必须经 `entErrToBizErr(err, domain)` 翻译（红线 DB-R5）。

---

## 十五、装配可观测性规范

`Assemble` 子装配器在以下场景必须通过 `lg.Warn` 记录日志（loggateway.Logger）：

| 场景 | 日志事件 | 说明 |
|------|----------|------|
| 工具已启用但配置缺失 | `system.tool_assembly_skip` | geminifetch 无 model、google_search 无 apiKey/cx |
| Factory 返回 nil 无 error | `system.tool_assembly_skip` | stub 工具或占位注册项 |
| OpenAPI spec 加载失败 | `system.builtin_tools_sync_fail` | 已有 |

---

## 十六、并发安全规范

| 全局变量 | 保护方式 | 位置 |
|----------|----------|------|
| `aliasRewriteTotal` | `atomic.Int64`（`AliasRewriteTotal`） | `internal/tools/normalize_invocation.go` |
| `defaultToolResultCache` | `ResultCache` 内部锁（包级私有单例，2026-08-14 取代已删除的 `cache.Global()`/`SetGlobal()`；测试经 `TRPCBuilderDeps.ResultCache` 注入隔离实例）。`web_fetch` 等 `IsCacheable` 工具由装饰器缓存，回调 hook 经 `CatalogResultCacheAllowed` 跳过 | `internal/agent/tool_result_cache.go` |
| `toolWebResChecker` | `sync.RWMutex`（`toolWebResCheckerMu`） | `internal/biz/tool/tool_catalog_runtime.go` |
| `filterCache.entries` | `sync.RWMutex`（读用 RLock，写用 Lock） | `internal/tools/skillruntime/filter.go` |
| `filterCache` 计数器 | `atomic` | `internal/tools/skillruntime/filter.go` |

---

## 十七、Skill 文件系统安全规范

`CreateSkillDir` 必须校验 slug 参数：

| 校验规则 | 错误类型 | 说明 |
|----------|----------|------|
| 空 slug | `kerrors.BadRequest("SKILL", "slug is required")` | 防止 SKILL.md 写入根目录 |
| 包含 `..` | `kerrors.BadRequest("SKILL", "slug contains unsafe path characters")` | 防止路径遍历 |
| 以 `/` 开头 | `kerrors.BadRequest("SKILL", "slug contains unsafe path characters")` | 防止绝对路径逃逸 |

---

## 十八、新增工具的步骤清单

1. 在 `internal/tools/toolset.go` 的 `Registry()` 中添加 `ToolRegistration` 条目（含 Tags / Group / Deferred）
2. 若工具需要配置，在 `AssemblyConfig` 的对应子配置（`ShellExec` / `Search` / `ClaudeCode` / `MCP` / `Session` / `Browser`）中添加字段
3. 在 `internal/tools/toolset.go` 中添加 `assembleXxx()` 子装配器函数
4. 在 `internal/tools/trpc/toolsets.go` 的 `BuildToolsets()` 中添加启用标志映射
5. 在 `internal/agent/trpc_build.go` 的 `buildToolsetsForAgent()` 中添加 effective key 到配置的映射
6. 在 `internal/data/builtin_tools_seed.go` 中添加种子数据（key 必须与 `Declaration().Name` 一致）
7. 在 `internal/biz/agent_effective_tools.go` 中按需更新 tool group 和 profile 定义
8. 编写单元测试验证注册 → 装配 → 注入链路
9. 在 `internal/tools/trpc/effective_config.go` 的 `ToolsetConfigFromEffectiveKeys` 中添加映射
10. 在 `internal/tools/testexec/config.go` 中添加在线测试支持判断
11. 若有别名，同步更新 `internal/tools/alias/alias.go` 和 `internal/biz/tool/tool_policy_keys.go`

---

*文档版本：4.2 — Phase 4 片段编辑运行时工具实现完成（diffedit/patchfile/editcontent/patch/textfile/file_views），同步代码漂移：Registry 31 项（+deliverable/media）、记录回调函数名、种子表补全说明（2026-07-20）。*
