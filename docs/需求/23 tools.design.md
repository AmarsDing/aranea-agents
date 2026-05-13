# Tools 工具模块 — 实现设计文档

> 对应需求：`23 tools.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

工具注册与管理：FunctionTool/StreamableTool/MCP Tool 统一目录、Agent 工具绑定、运行时挂载。工具是 Agent 可调用的具体外部能力，与 Plugin（运行时拦截器）和 Skill（面向 Agent 的能力+知识包）有明确边界。

核心能力：
- 工具目录 CRUD（含内置工具 + MCP 工具 + 外部工具）
- 工具启用/停用/风险等级管理
- Agent 工具绑定与生效矩阵
- 工具调用记录（ToolInvocation）查询
- 运行时工具挂载（trpc-agent-go Toolset 适配）

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

---

## 三、Biz 层

### 3.1 领域模型

```go
type Tool struct {
    ID                   string
    Key                  string              // 唯一标识，如 "web_search"
    DisplayName          string              // 显示名称
    Description          string
    Category             string              // "system"/"data"/"network"/...
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

type ToolRunQuery struct {
    ToolKey   string
    AgentID   string
    SessionID string
    Status    string
    From      string
    To        string
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
    SearchToolInvocations(ctx context.Context, q ToolRunQuery) (ToolRunResult, error)
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

func (u *ToolUsecase) ToggleEnabled(ctx context.Context, id string, enabled bool) (Tool, error)
// - 校验 id 非空
// - 调用 repo.UpdateToolEnabled

func (u *ToolUsecase) ListRuns(ctx context.Context, q ToolRunQuery) (ToolRunResult, error)
// - 校验分页参数
// - 调用 repo.SearchToolInvocations
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
    if err := entQueryRowScan(client, ctx, `
        SELECT COALESCE(COUNT(1), 0),
               COALESCE(SUM(CASE WHEN enabled = 1 THEN 1 ELSE 0 END), 0),
               COALESCE(SUM(CASE WHEN enabled = 1 AND risk_level IN ('high', 'critical') THEN 1 ELSE 0 END), 0)
        FROM tools t WHERE `+where, args,
        &s.TotalTools, &s.EnabledTools, &s.HighRiskEnabled); err != nil {
        return biz.ToolSummary{}, err
    }
    // 统计 24h 调用数和失败率
    cutoff := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
    var success24h, failed24h, blocked24h int
    if err := entQueryRowScan(client, ctx, `
        SELECT COALESCE(COUNT(1), 0),
               COALESCE(SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END), 0),
               COALESCE(SUM(CASE WHEN status = 'error' THEN 1 ELSE 0 END), 0),
               COALESCE(SUM(CASE WHEN status = 'blocked' THEN 1 ELSE 0 END), 0)
        FROM tool_invocations WHERE started_at >= ?`,
        []any{cutoff}, &s.Calls24h, &success24h, &failed24h, &blocked24h); err != nil {
        return biz.ToolSummary{}, err
    }
    if s.Calls24h > 0 {
        s.FailureRate24h = float64(failed24h+blocked24h) / float64(s.Calls24h)
    }
    return s, nil
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

**SearchToolInvocations**（工具调用记录查询）：

```go
func (r *toolRepo) SearchToolInvocations(ctx context.Context, q biz.ToolRunQuery) (biz.ToolRunResult, error) {
    // 原始 SQL 查询 tool_invocations 表
    // LEFT JOIN tools（获取 display_name）和 agents（获取 agent display_name）
    // 支持 tool_key/agent_id/session_id/status/from/to 筛选
    // ORDER BY started_at DESC, created_at DESC
}
```

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

### 7.1 工具挂载

```go
// internal/agent/trpc_build.go
func AppendEffectiveToolsets(ctx, ag, catalog, mcpUC, skillUC) ([]tool.Toolset, error)
```

### 7.2 内置工具

```go
// internal/tools/builtin/
var BuiltinTools = []Tool{
    {Name: "web_search", ToolType: "function", ...},
    {Name: "code_interpreter", ToolType: "function", ...},
}
```

### 7.3 FunctionTool 构建

```go
// internal/tools/trpc_tool.go
func BuildTRPCFunctionTool(t biz.Tool) (tool.Tool, error)
func BuildTRPCStreamableTool(t biz.Tool) (tool.Tool, error)
```

### 7.4 trpc-agent-go 工具集成（待实现）

```go
// internal/tools/trpc_adapter.go
func AdaptToTRPCTool(t biz.Tool) (tool.Tool, error)
```

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
// - 调用 toolApi.ListTools
// - kratosToolToLegacy 转换

export async function getTool(id: string): Promise<Tool>
// - 调用 toolApi.GetTool

export async function createTool(input: ToolUpsertInput): Promise<Tool>
// - 调用 toolApi.CreateTool

export async function updateTool(id: string, input: ToolUpsertInput): Promise<Tool>
// - 调用 toolApi.UpdateTool

export async function deleteTool(id: string): Promise<void>
// - 调用 toolApi.DeleteTool

export async function toggleToolEnabled(id: string, enabled: boolean): Promise<Tool>
// - 调用 toolApi.ToggleToolEnabled

export async function listToolRunsForTool(id: string, query: ToolRunQuery = {}): Promise<PaginatedResponse<ToolInvocation>>
// - 调用 toolApi.ListToolRunsForTool

export async function listToolRuns(query: ToolRunQuery = {}): Promise<PaginatedResponse<ToolInvocation>>
// - 调用 toolApi.ListToolRuns

export async function getAgentEffectiveTools(agentId: string): Promise<AgentEffectiveTools>
// - 调用 agentApi.GetAgentEffectiveTools（来自 agent/v1 proto）
```

### 8.4 页面组件

#### ToolsPage.vue

文件路径：`web/src/pages/ToolsPage.vue`

**页面结构**：

1. **顶部 Hero 区域**：标题 "工具管理" + Summary 统计卡片 + 刷新/新建按钮
2. **筛选卡片**：搜索框 + Category 下拉 + Source 下拉 + Risk Level 下拉 + 启用状态筛选
3. **数据表格**：展示工具列表
4. **新建/编辑弹窗**：创建或编辑工具
5. **详情弹窗**：查看工具完整信息
6. **配置弹窗**：编辑工具的 config_json

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

**新建/编辑弹窗控件**：

| 控件 | 绑定 | 说明 |
|------|------|------|
| QInput Key | key | 必填，唯一标识 |
| QInput 显示名称 | display_name | 必填 |
| QInput 描述 | description | |
| QSelect Category | category | system/data/network/... |
| QSelect Source | source | builtin/mcp/system/external |
| QSelect Risk Level | risk_level | low/medium/high/critical |
| QToggle 启用 | enabled | |
| QToggle 只读 | readonly | |
| QToggle 需确认 | requires_confirmation | |
| QToggle 支持流式 | supports_streaming | |
| QToggle 支持并发 | supports_concurrency | |
| QCodeEditor 参数 Schema | parameters_schema_json | JSON Schema |
| QCodeEditor 结果 Schema | result_schema_json | JSON Schema |
| QCodeEditor 配置 Schema | config_schema_json | JSON Schema |
| QCodeEditor 配置 | config_json | JSON |
| QCodeEditor 默认配置 | default_config_json | JSON |
| QCodeEditor 元数据 | metadata_json | JSON |

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
