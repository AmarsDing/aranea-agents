# Plugin 插件模块 — 实现设计文档

> 对应需求：`22 plugin.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

Plugin 是 ADK 运行时的回调扩展机制，用于在 Agent 执行过程中插入治理、调试、增强、风控等逻辑。Plugin 与 Skill / Tool 的边界：

- **Skill**：面向 Agent 的能力、知识、脚本和使用规范。
- **Tool**：Agent 可调用的具体外部能力。
- **Plugin**：运行时拦截器 / 中间件，改变或增强 Agent 执行链路。

当前系统管理内置 Plugin 的启用、配置、排序和绑定关系，不支持用户上传任意 Go 插件代码。

---

## 二、Proto 层

### 2.1 文件路径

`api/kratos/plugin/v1/plugin.proto`

### 2.2 完整 Proto 定义

```protobuf
syntax = "proto3";

package kratos.plugin.v1;

import "google/api/annotations.proto";
import "google/api/field_behavior.proto";

option go_package = "aranea-agents/api/kratos/plugin/v1;v1";

message PluginPermissions {
  bool can_view = 1;
  bool can_toggle = 2;
  bool can_edit_config = 3;
  bool can_view_logs = 4;
}

message Plugin {
  string id = 1;
  string key = 2;
  string name = 3;
  string description = 4;
  string category = 5;
  string risk_level = 6;            // "low" / "medium" / "high"
  bool enabled = 7;
  string scope = 8;                 // "global" / agent_id
  repeated string callback_points = 9;
  int32 sort_order = 10;
  string config_schema_json = 11;   // JSON Schema 定义配置项
  string config_json = 12;          // 当前生效配置
  string default_config_json = 13;  // 出厂默认配置
  int32 invoke_count = 14;
  int32 block_count = 15;
  int32 error_count = 16;
  string last_invoked_at = 17;
  string last_status = 18;          // "success" / "blocked" / "error"
  string created_at = 19;
  string updated_at = 20;
  PluginPermissions permissions = 21;
}

message ListPluginsRequest {
  string search = 1;                // 模糊搜索 key/name/description
  string category = 2;              // 精确筛选 category
  string enabled = 3;               // "" / "true" / "false" 三态
  string callback_point = 4;        // 筛选包含某 callback 的插件
  int32 page = 5;
  int32 page_size = 6;
}

message ListPluginsResponse {
  repeated Plugin items = 1;
  int32 total = 2;
  int32 page = 3;
  int32 page_size = 4;
}

message TogglePluginEnabledRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
  bool enabled = 2;
}

message UpdatePluginConfigRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
  string config_json = 2 [(google.api.field_behavior) = REQUIRED];
}

service PluginService {
  rpc ListPlugins(ListPluginsRequest) returns (ListPluginsResponse) {
    option (google.api.http) = {get: "/v1/plugins"};
  }
  rpc TogglePluginEnabled(TogglePluginEnabledRequest) returns (Plugin) {
    option (google.api.http) = {
      patch: "/v1/plugins/{id}/enabled"
      body: "*"
    };
  }
  rpc UpdatePluginConfig(UpdatePluginConfigRequest) returns (Plugin) {
    option (google.api.http) = {
      put: "/v1/plugins/{id}/config"
      body: "*"
    };
  }
}
```

### 2.3 HTTP API 汇总

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/v1/plugins` | 列表查询，支持 search/category/enabled/callback_point 筛选 |
| PATCH | `/v1/plugins/{id}/enabled` | 启用/停用插件 |
| PUT | `/v1/plugins/{id}/config` | 更新插件配置 JSON |

---

## 三、Biz 层

### 3.1 领域模型

```go
type Plugin struct {
    ID                string
    Key               string              // 唯一标识，如 "runtime_audit"
    Name              string              // 显示名称
    Description       string
    Category          string              // "observability"/"guard"/"tracking"/"debug"/"routing"/"policy"
    RiskLevel         string              // "low"/"medium"/"high"
    Enabled           bool
    Scope             string              // "global" 或 agent_id
    CallbackPoints    []string            // 注册的 callback 点列表
    SortOrder         int                 // 执行顺序，数字越小越先执行
    ConfigSchemaJSON  string              // JSON Schema 定义配置项
    ConfigJSON        string              // 当前生效配置
    DefaultConfigJSON string              // 出厂默认配置
    InvokeCount       int                 // 调用次数
    BlockCount        int                 // 拦截次数
    ErrorCount        int                 // 错误次数
    LastInvokedAt     string
    LastStatus        string              // "success"/"blocked"/"error"
    CreatedAt         string
    UpdatedAt         string
    Permissions       PluginPermissions
}

type PluginPermissions struct {
    CanView       bool
    CanToggle     bool
    CanEditConfig bool
    CanViewLogs   bool
}

type PluginListQuery struct {
    Search        string              // 模糊搜索 key/name/description
    Category      string              // 精确筛选
    Enabled       string              // ""/"true"/"false" 三态
    CallbackPoint string              // 筛选包含某 callback 的插件
    Limit         int
    Offset        int
}

type PluginListResult struct {
    Items  []Plugin
    Total  int
    Limit  int
    Offset int
}
```

### 3.2 Repo 接口

```go
type PluginRepo interface {
    SearchPlugins(ctx context.Context, q PluginListQuery) (PluginListResult, error)
    GetPlugin(ctx context.Context, id string) (Plugin, error)
    UpdatePluginEnabled(ctx context.Context, id string, enabled bool) (Plugin, error)
    UpdatePluginConfig(ctx context.Context, id string, configJSON string) (Plugin, error)
}
```

### 3.3 Usecase

```go
type PluginUsecase struct {
    repo PluginRepo
}

func NewPluginUsecase(repo PluginRepo) *PluginUsecase

func (u *PluginUsecase) List(ctx context.Context, q PluginListQuery) (PluginListResult, error)
// - 校验分页参数：Limit 默认 20，上限 100，Offset >= 0
// - 调用 repo.SearchPlugins

func (u *PluginUsecase) ToggleEnabled(ctx context.Context, id string, enabled bool) (Plugin, error)
// - 校验 id 非空
// - 调用 repo.UpdatePluginEnabled

func (u *PluginUsecase) UpdateConfig(ctx context.Context, id string, configJSON string) (Plugin, error)
// - 校验 id 非空
// - configJSON 为空时默认 "{}"
// - 校验 configJSON 是合法 JSON（json.Valid）
// - 调用 repo.UpdatePluginConfig
```

---

## 四、Data 层

### 4.1 Ent Schema

文件路径：`internal/data/ent/schema/plugin.go`

Ent 类型名 `PlatformPlugin`（避免与 Go `plugin` 包冲突），映射数据库表 `plugins`。

```go
type PlatformPlugin struct {
    ent.Schema
}

// 表名映射
func (PlatformPlugin) Annotations() []schema.Annotation {
    return []schema.Annotation{
        entsql.Annotation{Table: "plugins"},
    }
}

func (PlatformPlugin) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").Immutable().Unique().MaxLen(256),
        field.String("plugin_key").Unique().MaxLen(512),
        field.String("name").MaxLen(1024),
        field.Text("description").Default(""),
        field.String("category").Default(""),
        field.String("risk_level").Default("low"),
        field.String("status").Default("active"),
        field.Bool("enabled").Default(false),
        field.String("scope").Default("global").MaxLen(64),
        field.Text("callback_points_json").Default("[]"),
        field.Int("sort_order").Default(0),
        field.Text("config_schema_json").Default("{}"),
        field.Text("config_json").Default("{}"),
        field.Text("fallback_config_json").StorageKey("default_config_json").Default("{}"),
        field.Int("invoke_count").Default(0),
        field.Int("block_count").Default(0),
        field.Int("error_count").Default(0),
        field.String("last_invoked_at").Default(""),
        field.String("last_status").Default(""),
        field.String("created_at").Default(""),
        field.String("updated_at").Default(""),
        field.String("deleted_at").Default(""),
    }
}
```

### 4.2 Repo 实现

文件路径：`internal/data/plugin.go`

```go
type pluginRepo struct {
    data *Data
}

func NewPluginRepo(d *Data) biz.PluginRepo {
    return &pluginRepo{data: d}
}
```

**Ent → Biz 转换函数**：

```go
func entToBizPlugin(e *ent.PlatformPlugin) biz.Plugin {
    var cbs []string
    _ = json.Unmarshal([]byte(e.CallbackPointsJSON), &cbs)
    return biz.Plugin{
        ID:                e.ID,
        Key:               e.PluginKey,
        Name:              e.Name,
        Description:       e.Description,
        Category:          e.Category,
        RiskLevel:         e.RiskLevel,
        Enabled:           e.Enabled,
        Scope:             e.Scope,
        CallbackPoints:    cbs,
        SortOrder:         e.SortOrder,
        ConfigSchemaJSON:  e.ConfigSchemaJSON,
        ConfigJSON:        e.ConfigJSON,
        DefaultConfigJSON: e.FallbackConfigJSON,
        InvokeCount:       e.InvokeCount,
        BlockCount:        e.BlockCount,
        ErrorCount:        e.ErrorCount,
        LastInvokedAt:     e.LastInvokedAt,
        LastStatus:        e.LastStatus,
        CreatedAt:         e.CreatedAt,
        UpdatedAt:         e.UpdatedAt,
        Permissions:       adminPluginPerms(),
    }
}

func adminPluginPerms() biz.PluginPermissions {
    return biz.PluginPermissions{CanView: true, CanToggle: true, CanEditConfig: true, CanViewLogs: true}
}
```

**SearchPlugins 查询构建**：

```go
func (r *pluginRepo) pluginSearchQuery(q biz.PluginListQuery) *ent.PlatformPluginQuery {
    pq := r.data.entClient.PlatformPlugin.Query().Where(platformplugin.DeletedAtEQ(""))
    if s := strings.TrimSpace(q.Search); s != "" {
        pq = pq.Where(
            platformplugin.Or(
                platformplugin.PluginKeyContainsFold(s),
                platformplugin.NameContainsFold(s),
                platformplugin.DescriptionContainsFold(s),
            ),
        )
    }
    if cat := strings.TrimSpace(q.Category); cat != "" {
        pq = pq.Where(platformplugin.CategoryEQ(cat))
    }
    switch strings.TrimSpace(strings.ToLower(q.Enabled)) {
    case "true":
        pq = pq.Where(platformplugin.EnabledEQ(true))
    case "false":
        pq = pq.Where(platformplugin.EnabledEQ(false))
    }
    if cp := strings.TrimSpace(q.CallbackPoint); cp != "" {
        pq = pq.Where(platformplugin.CallbackPointsJSONContainsFold(cp))
    }
    return pq
}

func (r *pluginRepo) SearchPlugins(ctx context.Context, q biz.PluginListQuery) (biz.PluginListResult, error) {
    total, err := r.pluginSearchQuery(q).Count(ctx)
    if err != nil {
        return biz.PluginListResult{}, err
    }
    rows, err := r.pluginSearchQuery(q).
        Order(
            platformplugin.BySortOrder(),
            platformplugin.ByCreatedAt(entsql.OrderDesc()),
        ).
        Limit(q.Limit).
        Offset(q.Offset).
        All(ctx)
    if err != nil {
        return biz.PluginListResult{}, err
    }
    items := make([]biz.Plugin, 0, len(rows))
    for _, e := range rows {
        items = append(items, entToBizPlugin(e))
    }
    return biz.PluginListResult{Items: items, Total: total, Limit: q.Limit, Offset: q.Offset}, nil
}
```

**GetPlugin**：

```go
func (r *pluginRepo) GetPlugin(ctx context.Context, id string) (biz.Plugin, error) {
    row, err := r.data.entClient.PlatformPlugin.Query().
        Where(platformplugin.IDEQ(id), platformplugin.DeletedAtEQ("")).
        Only(ctx)
    if err != nil {
        if ent.IsNotFound(err) {
            return biz.Plugin{}, sql.ErrNoRows
        }
        return biz.Plugin{}, err
    }
    return entToBizPlugin(row), nil
}
```

**UpdatePluginEnabled**：

```go
func (r *pluginRepo) UpdatePluginEnabled(ctx context.Context, id string, enabled bool) (biz.Plugin, error) {
    err := r.data.entClient.PlatformPlugin.UpdateOneID(id).
        SetEnabled(enabled).
        SetUpdatedAt(nowRFC3339()).
        Exec(ctx)
    if err != nil {
        if ent.IsNotFound(err) {
            return biz.Plugin{}, sql.ErrNoRows
        }
        return biz.Plugin{}, err
    }
    return r.GetPlugin(ctx, id)
}
```

**UpdatePluginConfig**：

```go
func (r *pluginRepo) UpdatePluginConfig(ctx context.Context, id string, configJSON string) (biz.Plugin, error) {
    err := r.data.entClient.PlatformPlugin.UpdateOneID(id).
        SetConfigJSON(configJSON).
        SetUpdatedAt(nowRFC3339()).
        Exec(ctx)
    if err != nil {
        if ent.IsNotFound(err) {
            return biz.Plugin{}, sql.ErrNoRows
        }
        return biz.Plugin{}, err
    }
    return r.GetPlugin(ctx, id)
}
```

---

## 五、Service 层

文件路径：`internal/service/plugin.go`

```go
type PluginService struct {
    v1.UnimplementedPluginServiceServer
    uc *biz.PluginUsecase
}

func NewPluginService(uc *biz.PluginUsecase) *PluginService
```

**Biz → Proto 转换**：

```go
func toProtoPlugin(p biz.Plugin) *v1.Plugin {
    return &v1.Plugin{
        Id:                p.ID,
        Key:               p.Key,
        Name:              p.Name,
        Description:       p.Description,
        Category:          p.Category,
        RiskLevel:         p.RiskLevel,
        Enabled:           p.Enabled,
        Scope:             p.Scope,
        CallbackPoints:    p.CallbackPoints,
        SortOrder:         int32(p.SortOrder),
        ConfigSchemaJson:  p.ConfigSchemaJSON,
        ConfigJson:        p.ConfigJSON,
        DefaultConfigJson: p.DefaultConfigJSON,
        InvokeCount:       int32(p.InvokeCount),
        BlockCount:        int32(p.BlockCount),
        ErrorCount:        int32(p.ErrorCount),
        LastInvokedAt:     p.LastInvokedAt,
        LastStatus:        p.LastStatus,
        CreatedAt:         p.CreatedAt,
        UpdatedAt:         p.UpdatedAt,
        Permissions: &v1.PluginPermissions{
            CanView:       p.Permissions.CanView,
            CanToggle:     p.Permissions.CanToggle,
            CanEditConfig: p.Permissions.CanEditConfig,
            CanViewLogs:   p.Permissions.CanViewLogs,
        },
    }
}
```

**ListPlugins**：

```go
func (s *PluginService) ListPlugins(ctx context.Context, req *v1.ListPluginsRequest) (*v1.ListPluginsResponse, error) {
    limit, offset, page, pageSize := biz.PageToLimitOffset(req.GetPage(), req.GetPageSize())
    q := biz.PluginListQuery{
        Search:        req.GetSearch(),
        Category:      req.GetCategory(),
        Enabled:       req.GetEnabled(),
        CallbackPoint: req.GetCallbackPoint(),
        Limit:         limit,
        Offset:        offset,
    }
    result, err := s.uc.List(ctx, q)
    if err != nil {
        return nil, err
    }
    resp := &v1.ListPluginsResponse{
        Total:    int32(result.Total),
        Page:     page,
        PageSize: pageSize,
        Items:    make([]*v1.Plugin, 0, len(result.Items)),
    }
    for i := range result.Items {
        resp.Items = append(resp.Items, toProtoPlugin(result.Items[i]))
    }
    return resp, nil
}
```

**TogglePluginEnabled**：

```go
func (s *PluginService) TogglePluginEnabled(ctx context.Context, req *v1.TogglePluginEnabledRequest) (*v1.Plugin, error) {
    out, err := s.uc.ToggleEnabled(ctx, req.GetId(), req.GetEnabled())
    if err != nil {
        if errors.Is(err, sql.ErrNoRows) {
            return nil, kerrors.NotFound("PLUGIN", "plugin not found")
        }
        return nil, err
    }
    return toProtoPlugin(out), nil
}
```

**UpdatePluginConfig**：

```go
func (s *PluginService) UpdatePluginConfig(ctx context.Context, req *v1.UpdatePluginConfigRequest) (*v1.Plugin, error) {
    out, err := s.uc.UpdateConfig(ctx, req.GetId(), req.GetConfigJson())
    if err != nil {
        if errors.Is(err, sql.ErrNoRows) {
            return nil, kerrors.NotFound("PLUGIN", "plugin not found")
        }
        return nil, err
    }
    return toProtoPlugin(out), nil
}
```

---

## 六、Wire 注入

```
data.ProviderSet  → NewPluginRepo
biz.ProviderSet   → NewPluginUsecase
service.ProviderSet → NewPluginService
```

---

## 七、内置 Plugin 注册机制

### 7.1 编译期内置注册

后端启动时将内置插件同步到数据库 `plugins` 表。内置插件列表：

| Plugin | Key | Category | RiskLevel | 默认状态 | Callback Points |
|--------|-----|----------|-----------|----------|-----------------|
| 运行日志和审计 | `runtime_audit` | observability | low | 开发环境建议启用 | OnUserMessage, BeforeModel, AfterModel, BeforeTool, AfterTool, OnToolError, OnEvent |
| Skill 调用统计 | `skill_usage_tracker` | tracking | low | 可启用 | BeforeTool, AfterTool, OnToolError |
| 工具失败自愈 | `retry_and_reflect` | debug | medium | 可启用 | OnToolError |
| 输入输出脱敏 | `sensitive_data_mask` | guard | medium | 建议启用 | OnUserMessage, BeforeModel, AfterModel |
| 高风险操作确认 | `confirmation_guard` | guard | high | 默认启用但默认拒绝 | BeforeTool |
| 模型成本控制 | `cost_guard` | guard | medium | 可启用 | BeforeModel |
| 模型路由 | `model_router` | routing | low | 可启用 | BeforeModel |
| 工具权限控制 | `permission_guard` | guard | high | 可启用 | BeforeTool |
| 输出策略检查 | `output_policy` | policy | medium | 可启用 | AfterModel |

### 7.2 运行时加载

`adk_runner` 优先读取数据库启用状态和 `config_json` 生成 `runner.PluginConfig`。如果未接入数据库 PluginSource，回退读取 `ADK_RUNNER_PLUGINS` 环境变量。

### 7.3 各内置 Plugin 配置 Schema 示例

**runtime_audit**：
```json
{
  "type": "object",
  "properties": {
    "log_user_message":   { "type": "boolean", "default": true },
    "log_model_request":  { "type": "boolean", "default": true },
    "log_model_response": { "type": "boolean", "default": true },
    "log_tool_args":      { "type": "boolean", "default": true },
    "max_content_length": { "type": "integer", "default": 500 },
    "redact_sensitive":   { "type": "boolean", "default": true }
  }
}
```

**retry_and_reflect**：
```json
{
  "type": "object",
  "properties": {
    "max_retries":                    { "type": "integer", "default": 3 },
    "tracking_scope":                 { "type": "string", "enum": ["invocation","global"], "default": "invocation" },
    "error_if_retry_exceeded":        { "type": "boolean", "default": false },
    "excluded_tools":                 { "type": "array", "items": { "type": "string" }, "default": [] },
    "high_risk_tools_need_confirm":   { "type": "boolean", "default": true }
  }
}
```

**sensitive_data_mask**：
```json
{
  "type": "object",
  "properties": {
    "mask_email":        { "type": "boolean", "default": true },
    "mask_phone":        { "type": "boolean", "default": true },
    "mask_secret":       { "type": "boolean", "default": true },
    "custom_patterns":   { "type": "array", "items": { "type": "object" }, "default": [] },
    "block_leak_output": { "type": "boolean", "default": true }
  }
}
```

**confirmation_guard**：
```json
{
  "type": "object",
  "properties": {
    "confirm_tools":    { "type": "array", "items": { "type": "string" }, "default": [] },
    "confirm_patterns": { "type": "array", "items": { "type": "string" }, "default": [] },
    "timeout_seconds":  { "type": "integer", "default": 300 },
    "default_action":   { "type": "string", "enum": ["reject","allow"], "default": "reject" }
  }
}
```

**cost_guard**：
```json
{
  "type": "object",
  "properties": {
    "daily_token_budget":  { "type": "integer", "default": 0 },
    "max_prompt_tokens":   { "type": "integer", "default": 0 },
    "blocked_models":      { "type": "array", "items": { "type": "string" }, "default": [] },
    "fallback_model":      { "type": "string", "default": "" },
    "admin_bypass":        { "type": "boolean", "default": true }
  }
}
```

**model_router**：
```json
{
  "type": "object",
  "properties": {
    "rules":                 { "type": "array", "items": { "type": "object" }, "default": [] },
    "default_model":         { "type": "string", "default": "" },
    "code_model":            { "type": "string", "default": "" },
    "long_context_model":    { "type": "string", "default": "" },
    "fallback_model":        { "type": "string", "default": "" }
  }
}
```

**permission_guard**：
```json
{
  "type": "object",
  "properties": {
    "deny_tools":       { "type": "array", "items": { "type": "string" }, "default": [] },
    "confirm_tools":    { "type": "array", "items": { "type": "string" }, "default": [] },
    "agent_allowlist":  { "type": "array", "items": { "type": "string" }, "default": [] },
    "role_rules":       { "type": "array", "items": { "type": "object" }, "default": [] }
  }
}
```

**output_policy**：
```json
{
  "type": "object",
  "properties": {
    "blocked_patterns":          { "type": "array", "items": { "type": "string" }, "default": [] },
    "dangerous_command_check":   { "type": "boolean", "default": true },
    "block_on_violation":        { "type": "boolean", "default": true },
    "replacement_message":       { "type": "string", "default": "" }
  }
}
```

**skill_usage_tracker**：
```json
{
  "type": "object",
  "properties": {
    "track_success":          { "type": "boolean", "default": true },
    "track_failure":          { "type": "boolean", "default": true },
    "capture_input_preview":  { "type": "boolean", "default": true },
    "capture_output_preview": { "type": "boolean", "default": true },
    "max_preview_length":     { "type": "integer", "default": 500 }
  }
}
```

---

## 八、Web 前端设计

### 8.1 文件结构

```
web/src/
├── features/plugins/
│   ├── api.ts                    # API 调用封装
│   └── types.ts                  # TypeScript 类型定义
└── pages/
    └── PluginsPage.vue           # Plugin 管理页面
```

### 8.2 TypeScript 类型定义

文件路径：`web/src/features/plugins/types.ts`

```typescript
export type PaginatedResponse<T> = {
  items: T[];
  page: number;
  page_size: number;
  total: number;
};

export type PluginPermissions = {
  can_view: boolean;
  can_toggle: boolean;
  can_edit_config: boolean;
  can_view_logs: boolean;
};

export type Plugin = {
  id: string;
  key: string;
  name: string;
  description: string;
  category: string;
  risk_level: "low" | "medium" | "high" | string;
  enabled: boolean;
  scope: string;
  callback_points: string[];
  sort_order: number;
  config_schema_json: string;
  config_json: string;
  default_config_json: string;
  invoke_count: number;
  block_count: number;
  error_count: number;
  last_invoked_at?: string;
  last_status?: string;
  created_at: string;
  updated_at: string;
  permissions: PluginPermissions;
};

export type PluginListQuery = {
  search?: string;
  category?: string;
  enabled?: boolean | null;
  callback_point?: string;
  page?: number;
  page_size?: number;
};
```

### 8.3 API 封装

文件路径：`web/src/features/plugins/api.ts`

```typescript
import { createPluginService } from "../../services";
import type { PaginatedResponse, Plugin, PluginListQuery } from "./types";

function mapPluginRow(row: unknown): Plugin {
  const r = row as Record<string, unknown>;
  const s = (snake: string, camel: string) => String(r[snake] ?? r[camel] ?? "");
  const n = (snake: string, camel: string) => Number(r[snake] ?? r[camel] ?? 0);
  const b = (snake: string, camel: string) => Boolean(r[snake] ?? r[camel]);
  const rawPerms = r.permissions as Record<string, unknown> | undefined;
  const p = rawPerms ?? {};
  const pb = (snake: string, camel: string) => Boolean(p[snake] ?? p[camel] ?? false);
  const cbs = r.callback_points ?? r.callbackPoints;
  const callbackPoints = Array.isArray(cbs) ? cbs.map((x) => String(x)) : [];
  return {
    id: s("id", "id"),
    key: s("key", "key"),
    name: s("name", "name"),
    description: s("description", "description"),
    category: s("category", "category"),
    risk_level: s("risk_level", "riskLevel") || "low",
    enabled: b("enabled", "enabled"),
    scope: s("scope", "scope"),
    callback_points: callbackPoints,
    sort_order: n("sort_order", "sortOrder"),
    config_schema_json: s("config_schema_json", "configSchemaJson"),
    config_json: s("config_json", "configJson"),
    default_config_json: s("default_config_json", "defaultConfigJson"),
    invoke_count: n("invoke_count", "invokeCount"),
    block_count: n("block_count", "blockCount"),
    error_count: n("error_count", "errorCount"),
    last_invoked_at: s("last_invoked_at", "lastInvokedAt") || undefined,
    last_status: s("last_status", "lastStatus") || undefined,
    created_at: s("created_at", "createdAt"),
    updated_at: s("updated_at", "updatedAt"),
    permissions: {
      can_view: pb("can_view", "canView"),
      can_toggle: pb("can_toggle", "canToggle"),
      can_edit_config: pb("can_edit_config", "canEditConfig"),
      can_view_logs: pb("can_view_logs", "canViewLogs"),
    },
  };
}

export async function listPlugins(query: PluginListQuery = {}): Promise<PaginatedResponse<Plugin>> {
  const svc = createPluginService();
  let enabled: string | undefined;
  if (query.enabled === true) enabled = "true";
  else if (query.enabled === false) enabled = "false";
  const page = query.page ?? 1;
  const pageSize = query.page_size ?? 20;
  const res = await svc.ListPlugins({
    search: query.search?.trim() || undefined,
    category: query.category?.trim() || undefined,
    enabled,
    callbackPoint: query.callback_point?.trim() || undefined,
    page,
    pageSize,
  });
  const items = (res.items ?? []).map(mapPluginRow);
  return { items, total: Number(res.total ?? 0), page: Number(res.page ?? page), page_size: Number(res.pageSize ?? pageSize) };
}

export async function togglePluginEnabled(id: string, enabled: boolean): Promise<Plugin> {
  const svc = createPluginService();
  const row = await svc.TogglePluginEnabled({ id, enabled });
  return mapPluginRow(row);
}

export async function updatePluginConfig(id: string, configJSON: string): Promise<Plugin> {
  const svc = createPluginService();
  const row = await svc.UpdatePluginConfig({ id, configJson: configJSON });
  return mapPluginRow(row);
}
```

### 8.4 页面组件

#### PluginsPage.vue

文件路径：`web/src/pages/PluginsPage.vue`

**页面结构**：

1. **顶部 Hero 区域**：标题 "Plugin 管理" + 刷新按钮
2. **筛选卡片**：搜索框 + Category 下拉 + 启用状态筛选 + Callback 筛选
3. **数据表格**：展示 Plugin 列表
4. **详情弹窗**：查看 Plugin 完整信息
5. **配置弹窗**：编辑 Plugin 的 config_json

**表格列定义**：

| 列名 | 字段 | 说明 |
|------|------|------|
| Plugin | name + key | 名称 + 唯一标识 |
| 说明 | description | 插件描述 |
| 类型/风险 | category + risk_level | Category Chip + Risk Chip（颜色区分） |
| Callback | callback_points | 多个 Chip 展示 |
| 启用 | enabled | Toggle 开关，受 permissions.can_toggle 控制 |
| 操作 | id | 查看详情 + 编辑配置按钮，受 permissions 控制 |

**Category 下拉选项**：`observability`, `guard`, `tracking`, `debug`, `routing`, `policy`

**风险等级颜色映射**：
- `high` → negative (红色)
- `medium` → warning (橙色)
- `low` → positive (绿色)

**详情弹窗内容**：
- 名称 + Key
- 描述
- 类型 / 风险 / 作用域 / 排序
- 调用次数 / 阻断次数 / 错误次数
- 最近状态 / 最近调用时间
- Callback Points 列表
- 配置 JSON（可展开）
- 默认配置（可展开）
- 配置 Schema（可展开）

**配置弹窗内容**：
- 标题：配置 {name}
- JSON 编辑器（textarea + autogrow）
- 实时 JSON 格式校验
- 默认配置 / Schema 参考区（可展开）
- 保存 / 取消按钮

**交互逻辑**：
- 筛选条件变化时自动重载列表（page 重置为 1）
- Toggle 启用状态：调用 `togglePluginEnabled`，成功后更新本地行数据
- 保存配置：调用 `updatePluginConfig`，成功后更新本地行数据并关闭弹窗
- 所有操作带 loading 状态和错误通知

### 8.5 路由

| 路径 | 组件 | 说明 |
|------|------|------|
| `/plugins` | PluginsPage | Plugin 管理页 |
