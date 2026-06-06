# Plugin 插件模块 — 实现设计文档

> 对应需求：[22 plugin.md](./22%20plugin.md)
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

Plugin 是 ADK 运行时的回调扩展机制，用于在 Agent 执行过程中插入治理、调试、增强、风控等逻辑。Plugin 与 Skill / Tool 的边界：

- **Skill**：面向 Agent 的能力、知识、脚本和使用规范。
- **Tool**：Agent 可调用的具体外部能力。
- **Plugin**：运行时拦截器 / 中间件，改变或增强 Agent 执行链路。

当前系统管理内置 Plugin 的启用、配置、排序和绑定关系，不支持用户上传任意 Go 插件代码。

**回调编排（2026-05-28）**：编排注释已合并至 `internal/plugin/trpc/manager.go` 顶部。DB 内置 Plugin 走 Runner `WithPlugins`（按 `sort_order`）；框架 Plugin（Identity、Guardrail、ToolCallID、MessageMerger）由 `Manager.RunnerPluginsForAgent` 自动追加；产品 Hook 规则走 LLMAgent Callback Chain；`model_router` / `cost_guard` 的 catalog 换模走 `agent.ModelSelector`。`confirmation_guard` Runner Plugin 直接阻断（BeforeTool CustomResult），不再依赖 Chain ConfirmGate。

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

message UpdatePluginSortOrderRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
  int32 sort_order = 2;
}

message UpdatePluginScopeRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
  string scope = 2 [(google.api.field_behavior) = REQUIRED];  // "global" 或 agent_id
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
  rpc UpdatePluginSortOrder(UpdatePluginSortOrderRequest) returns (Plugin) {
    option (google.api.http) = {
      patch: "/v1/plugins/{id}/sort-order"
      body: "*"
    };
  }
  rpc UpdatePluginScope(UpdatePluginScopeRequest) returns (Plugin) {
    option (google.api.http) = {
      patch: "/v1/plugins/{id}/scope"
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
| PATCH | `/v1/plugins/{id}/sort-order` | 更新插件执行顺序 |
| PATCH | `/v1/plugins/{id}/scope` | 更新插件作用域（global 或 agent_id） |

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
    GetByKey(ctx context.Context, key string) (Plugin, error)
    CreatePlugin(ctx context.Context, p Plugin) (Plugin, error)
    UpdatePluginEnabled(ctx context.Context, id string, enabled bool) (Plugin, error)
    UpdatePluginConfig(ctx context.Context, id string, configJSON string) (Plugin, error)
    UpdateSortOrder(ctx context.Context, id string, sortOrder int) (Plugin, error)
    UpdateScope(ctx context.Context, id string, scope string) (Plugin, error)
    IncrementStats(ctx context.Context, key string, delta PluginStatUpdate) error
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

func (u *PluginUsecase) UpdateSortOrder(ctx context.Context, id string, sortOrder int) (Plugin, error)
// - 校验 id 非空
// - 调用 repo.UpdateSortOrder

func (u *PluginUsecase) UpdateScope(ctx context.Context, id string, scope string) (Plugin, error)
// - 校验 id 非空
// - scope 为空时默认 "global"
// - 调用 repo.UpdateScope

func (u *PluginUsecase) GetByKey(ctx context.Context, key string) (Plugin, error)
// - 调用 repo.GetByKey

func (u *PluginUsecase) Create(ctx context.Context, p Plugin) (Plugin, error)
// - 校验 Key 非空且唯一
// - 调用 repo.CreatePlugin
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
        Permissions:       biz.AdminPluginPerms(),
    }
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
    base := r.pluginSearchQuery(q)
    total, err := base.Count(ctx)
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

**UpdateSortOrder**：

```go
func (r *pluginRepo) UpdateSortOrder(ctx context.Context, id string, sortOrder int) (biz.Plugin, error) {
    err := r.data.entClient.PlatformPlugin.UpdateOneID(id).
        SetSortOrder(sortOrder).
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

### 5.1 构造函数

```go
type PluginService struct {
    v1.UnimplementedPluginServiceServer

    uc      *biz.PluginUsecase
    runtime *plugintrpc.Runtime
}

func NewPluginService(uc *biz.PluginUsecase, runtime *plugintrpc.Runtime) *PluginService {
    return &PluginService{uc: uc, runtime: runtime}
}
```

### 5.2 热重载机制

每次写操作（ToggleEnabled / UpdateConfig / UpdateSortOrder）成功后，异步触发 `reloadRuntime()`：

```go
func (s *PluginService) reloadRuntime(ctx context.Context) {
    if s.runtime == nil {
        return
    }
    safego.Go(ctx, "plugin.reloadRuntime", func() {
        result, err := s.uc.List(context.Background(), biz.PluginListQuery{Enabled: "true", Limit: 200})
        if err != nil {
            slog.Warn("plugin.reloadRuntime: list enabled failed", "error", err)
            return
        }
        s.runtime.Apply(context.Background(), result.Items)
    })
}
```

热重载数据流：

```
写操作 → DB 更新 → reloadRuntime() → uc.List(enabled=true) → runtime.Apply() → 下次 Runner 创建获取最新插件
```

### 5.3 Biz → Proto 转换

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

### 5.4 RPC 方法

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
    s.reloadRuntime(ctx)
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
    s.reloadRuntime(ctx)
    return toProtoPlugin(out), nil
}
```

**UpdatePluginSortOrder**：

```go
func (s *PluginService) UpdatePluginSortOrder(ctx context.Context, req *v1.UpdatePluginSortOrderRequest) (*v1.Plugin, error) {
    out, err := s.uc.UpdateSortOrder(ctx, req.GetId(), int(req.GetSortOrder()))
    if err != nil {
        if errors.Is(err, sql.ErrNoRows) {
            return nil, kerrors.NotFound("PLUGIN", "plugin not found")
        }
        return nil, err
    }
    s.reloadRuntime(ctx)
    return toProtoPlugin(out), nil
}
```

**UpdatePluginScope**：

```go
func (s *PluginService) UpdatePluginScope(ctx context.Context, req *v1.UpdatePluginScopeRequest) (*v1.Plugin, error) {
    out, err := s.uc.UpdateScope(ctx, req.GetId(), req.GetScope())
    if err != nil {
        if errors.Is(err, sql.ErrNoRows) {
            return nil, kerrors.NotFound("PLUGIN", "plugin not found")
        }
        return nil, err
    }
    s.reloadRuntime(ctx)
    return toProtoPlugin(out), nil
}
```

---

## 六、Wire 注入

```
data.ProviderSet  → NewPluginRepo
biz.ProviderSet   → NewPluginUsecase
service.ProviderSet → NewPluginService(uc, runtime)
```

`plugintrpc.Runtime` 通过 Wire 注入到 `PluginService`，确保 Service 层可触发热重载。

---

## 七、Plugin 运行时架构

### 7.1 核心组件

| 组件 | 文件路径 | 职责 |
|------|----------|------|
| `plugintrpc.Runtime` | `internal/plugin/trpc/runtime.go` | 管理活跃插件实例，支持热重载 |
| `plugintrpc.adapt` | `internal/plugin/trpc/adapter.go` | 将 `biz.Plugin` 转换为 `trpcplugin.Plugin` |
| `plugintrpc.builtin` | `internal/plugin/trpc/adapter.go` | 根据 Key 创建具体内置插件实例 |
| `AuditLogPlugin` | `internal/plugin/trpc/audit.go` | 内置审计日志插件 |

### 7.2 Runtime 热重载

```go
type Runtime struct {
    mu          sync.RWMutex
    active      []runtimeEntry
    stats       StatsRecorder
    retryWorker *HookRetryWorker
}

type runtimeEntry struct {
    plugin        trpcplugin.Plugin
    scope         string
    sortOrder     int
    orchestration PluginOrchestrationPath
}

// Apply 替换活跃插件集（仅实例化 enabled + 已知 key 的插件）
func (rt *Runtime) Apply(_ context.Context, plugins []biz.Plugin) {
    built := make([]runtimeEntry, 0, len(plugins))
    for _, p := range plugins {
        if tp := adapt(p, rt.stats, rt.bus, rt); tp != nil {
            built = append(built, runtimeEntry{plugin: tp, scope: p.Scope, sortOrder: p.SortOrder})
        }
    }
    rt.mu.Lock()
    rt.active = built
    rt.mu.Unlock()
}

// PluginsForAgent returns active plugins for the agent.
func (rt *Runtime) PluginsForAgent(agentID string) []trpcplugin.Plugin { ... }

// Close stops background workers (hook retry worker, stats batch worker).
func (rt *Runtime) Close() { ... }
```
```

线程安全保证：
- `Apply()` 使用写锁替换整个 `active` 切片。
- `Plugins()` 使用读锁返回快照副本。
- 并发安全，无需额外同步。

### 7.3 Adapter 适配层

```go
// adaptedPlugin 包装 Plugin 实例及其已解析的配置，消除 Apply 中重复 parsePluginConfig
type adaptedPlugin struct {
    plugin              trpcplugin.Plugin
    modelRouter         *ModelRouterConfig
    costGuard           *CostGuardConfig
    confirmationGuard   *ConfirmationGuardConfig
}

// adapt 将 biz.Plugin 转换为 adaptedPlugin（含已解析配置）
// disabled 或未知 key 返回 nil
func adapt(p biz.Plugin, stats StatsRecorder, bus event.Bus, rt *Runtime) *adaptedPlugin {
    if !p.Enabled {
        return nil
    }
    ValidatePluginCallbackPoints(p)
    tp := builtin(p, stats, bus, rt)
    if tp == nil {
        return nil
    }
    ap := &adaptedPlugin{plugin: tp}
    key := strings.ToLower(strings.TrimSpace(p.Key))
    switch key {
    case "model_router":
        var cfg ModelRouterConfig
        parsePluginConfig(p.ConfigJSON, p.DefaultConfigJSON, &cfg)
        ap.modelRouter = &cfg
    case "cost_guard":
        var cfg CostGuardConfig
        parsePluginConfig(p.ConfigJSON, p.DefaultConfigJSON, &cfg)
        ap.costGuard = &cfg
    case "confirmation_guard":
        var cfg ConfirmationGuardConfig
        parsePluginConfig(p.ConfigJSON, p.DefaultConfigJSON, &cfg)
        ap.confirmationGuard = &cfg
    }
    return ap
}

// builtin 根据 key 创建具体内置插件实例
// 未知 key 返回 nil（静默跳过，防止数据库实验数据破坏运行时）
func builtin(p biz.Plugin, stats StatsRecorder, bus event.Bus, rt *Runtime) trpcplugin.Plugin {
    key := strings.ToLower(strings.TrimSpace(p.Key))
    switch key {
    case "audit_log":
        return NewAuditLogPlugin(p, stats, bus)
    // 后续内置插件在此添加 case
    default:
        return nil
    }
}
```

### 7.4 Plugin 注入 Runner 数据流

```
数据库 plugins 表
    ↓ PluginService.reloadRuntime()
    ↓ uc.List(enabled=true)
    ↓ runtime.Apply(enabledPlugins)
    ↓
plugintrpc.Runtime.active []trpcplugin.Plugin
    ↓ runtime.Plugins() 返回快照
    ↓
internal/agent/trpc_runtime.go:43
    trpcrunner.WithPlugins(deps.Plugins...)
    ↓
Runner 创建 plugin.Manager
    ↓
执行期间回调触发
```

关键注入点：
- `internal/agent/trpc_runtime.go:43`：`trpcrunner.WithPlugins(deps.Plugins...)`
- `internal/agent/turn_helpers.go:37`：`deps.Plugins = plugins`
- `internal/service/trpc_turn.go:58`：`s.pluginRT.Plugins()` 获取插件列表

### 7.5 trpc-agent-go Plugin 接口

框架定义的 `plugin.Plugin` 接口：

```go
type Plugin interface {
    Name() string
    Register(r *Registry)
}

type Closer interface {
    Close(ctx context.Context) error
}
```

框架 `plugin.Registry` 提供的 7 个注册方法：

| 方法 | 回调签名 | 触发时机 |
|------|----------|----------|
| `r.BeforeAgent(cb)` | `agent.BeforeAgentCallbackStructured` | Agent 执行前 |
| `r.AfterAgent(cb)` | `agent.AfterAgentCallbackStructured` | Agent 执行后 |
| `r.BeforeModel(cb)` | `model.BeforeModelCallbackStructured` | 模型请求前 |
| `r.AfterModel(cb)` | `model.AfterModelCallbackStructured` | 模型响应后 |
| `r.BeforeTool(cb)` | `tool.BeforeToolCallbackStructured` | 工具执行前 |
| `r.AfterTool(cb)` | `tool.AfterToolCallbackStructured` | 工具执行后 |
| `r.OnEvent(hook)` | `plugin.EventHook` | 事件经过 Runner 时 |

> **注意**：框架不提供 `OnUserMessage`、`BeforeRun`、`AfterRun`、`OnModelError`、`OnToolError` 等回调点。如需处理模型/工具错误，在 `AfterModel` / `AfterTool` 中检查 `args.Error != nil`。

框架 `plugin.Manager` 组合回调：

```go
type Manager struct {
    plugins        []Plugin
    agentCallbacks *agent.Callbacks
    modelCallbacks *model.Callbacks
    toolCallbacks  *tool.Callbacks
    eventHooks     []namedEventHook
}

func NewManager(plugins ...Plugin) (*Manager, error)
func (m *Manager) AgentCallbacks() *agent.Callbacks
func (m *Manager) ModelCallbacks() *model.Callbacks
func (m *Manager) ToolCallbacks() *tool.Callbacks
func (m *Manager) OnEvent(ctx, invocation, event) (*event.Event, error)
func (m *Manager) Close(ctx context.Context) error
```

---

## 八、内置 Plugin 实现

### 8.1 内置插件列表

9 个内置插件的完整定义（Key、Category、RiskLevel、默认状态、Callback Points、配置项）参见需求文档 §2.1–§2.10。

本节仅列出 Key 与文件映射，供开发参考：

| Key | 实现文件 | 注册回调点 |
|-----|----------|------------|
| `audit_log` | `internal/plugin/trpc/audit.go` | BeforeAgent, AfterAgent, BeforeModel, AfterModel, BeforeTool, AfterTool, OnEvent |
| `skill_usage_tracker` | `internal/plugin/trpc/skill_tracker.go` | BeforeTool, AfterTool |
| `retry_and_reflect` | `internal/plugin/trpc/retry_reflect.go` | AfterAgent, AfterTool |
| `sensitive_data_mask` | `internal/plugin/trpc/sensitive_mask.go` | BeforeModel, AfterModel |
| `confirmation_guard` | `internal/plugin/trpc/confirmation_guard.go` | BeforeTool（直接阻断） |
| `cost_guard` | `internal/plugin/trpc/cost_guard.go` | BeforeModel |
| `model_router` | `internal/plugin/trpc/model_router.go` | BeforeModel |
| `permission_guard` | `internal/plugin/trpc/permission_guard.go` | BeforeTool |
| `output_policy` | `internal/plugin/trpc/output_policy.go` | AfterModel, OnEvent |

### 8.0 框架 Plugin（自动注入，无需 DB 配置）

以下 trpc-agent-go 框架 Plugin 由 `Manager.RunnerPluginsForAgent` 自动追加到每个 Runner，无需在 DB 中启用/配置：

| Plugin | 桥接文件 | 注册回调点 | 功能 |
|--------|----------|------------|------|
| `identity` | `internal/plugin/trpc/identity_bridge.go` | BeforeAgent, BeforeTool | 用户身份解析与透传（Headers/EnvVars/Token）；支持 `ContextWithToken` 从 ctx 提取 access token |
| `guardrail` | `internal/plugin/trpc/guardrail_bridge.go` | BeforeModel（PromptInjection + UnsafeIntent） | 输入侧安全护栏：rule-based 快速过滤（5 类 PromptInjection + 6 类 UnsafeIntent）+ 可选 LLM 深度审查（`GuardrailReviewers` 链式组合）；`normalizeInput` 过滤零宽字符（`Cf`/`Mn`/`Me`）+ 小写归一化；`detectPromptInjection`/`detectUnsafeIntent` 匹配前先归一化，防止零宽字符绕过 |
| `tool_call_id` | 框架 `plugin/toolcallid` | AfterModel | 规范化跨厂商 ToolCall ID |
| `consecutive_message_merger` | 框架 `plugin/messagemerger` | BeforeModel | 合并同角色连续消息，减少 Token 消耗 |

### 8.2 已实现：AuditLogPlugin

文件路径：`internal/plugin/trpc/audit.go`

```go
type AuditLogPlugin struct {
    base basePlugin  // 嵌入 basePlugin（name + stats + logger），消除 9 个插件的重复代码
}

var _ trpcplugin.Plugin = (*AuditLogPlugin)(nil)

func (a *AuditLogPlugin) Name() string { return a.base.Name() }

func (a *AuditLogPlugin) Register(r *trpcplugin.Registry) {
    r.AfterTool(func(ctx context.Context, args *trpctool.AfterToolArgs) (*trpctool.AfterToolResult, error) {
        status := "ok"
        if args.Error != nil {
            status = "error"
        }
        a.base.record(ctx, "after_tool", status)
        return &trpctool.AfterToolResult{}, nil
    })
}
```

**basePlugin 嵌入**（`internal/plugin/trpc/base_plugin.go`）：9 个内置插件共用 `name`/`stats`/`logger` 字段和 `record()` 方法，消除重复代码。

**PluginSafeLogger**（`internal/plugin/trpc/safe_logger.go`）：统一使用 `event.SysLog*` 记录日志（符合红线 16），同时通过 `event.Bus` 发布到 monitor channel。不再直接写 `os.Stderr`。

### 8.3 内置插件实现模板

每个内置插件需实现以下结构：

```go
type XxxPlugin struct {
    base   basePlugin       // 嵌入 basePlugin（name + stats + logger）
    config XxxConfig
}

var _ trpcplugin.Plugin = (*XxxPlugin)(nil)

func (p *XxxPlugin) Name() string { return p.base.Name() }

func (p *XxxPlugin) Register(r *trpcplugin.Registry) {
    // 注册所需的回调点
    // 使用 p.base.record(ctx, point, status) 记录统计
}
```

实现步骤：
1. 在 `internal/plugin/trpc/` 下创建新文件（如 `sensitive_mask.go`）。
2. 定义插件结构体，实现 `plugin.Plugin` 接口。
3. 在 `Register` 方法中注册所需回调点。
4. 在 `adapter.go` 的 `builtin()` 函数中添加 case 分支。
5. 在数据库 `plugins` 表中插入对应记录（或通过启动同步机制）。

### 8.4 各内置 Plugin 配置 Schema

**runtime_audit**：
```json
{
  "type": "object",
  "properties": {
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

## 九、前端技术实现

> 页面结构、交互逻辑和 UI 组件设计参见需求文档 §3 和 §8。本节仅描述前端技术实现细节。

### 9.1 API 调用封装

文件路径：`web/src/services/plugin.ts`

```typescript
export async function listPlugins(params: ListPluginsParams): Promise<ListPluginsResult> {
  const svc = createPluginService();
  const res = await svc.ListPlugins({
    search: params.search,
    category: params.category,
    enabled: params.enabled,
    callbackPoint: params.callback_point,
    page: params.page,
    pageSize: params.page_size,
  });
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

export async function updatePluginSortOrder(id: string, sortOrder: number): Promise<Plugin> {
  const svc = createPluginService();
  const row = await svc.UpdatePluginSortOrder({ id, sortOrder });
  return mapPluginRow(row);
}

export async function updatePluginScope(id: string, scope: string): Promise<Plugin> {
  const svc = createPluginService();
  const row = await svc.UpdatePluginScope({ id, scope });
  return mapPluginRow(row);
}
```

### 9.2 页面组件

#### PluginsPage.vue

文件路径：`web/src/pages/PluginsPage.vue`

页面结构和交互逻辑参见需求文档 §3。本节描述技术实现要点：

**数据流**：
1. `onMounted` 调用 `listPlugins()` 加载列表。
2. 筛选条件变化时自动重载列表（page 重置为 1）。
3. Toggle 启用状态：调用 `togglePluginEnabled`，成功后更新本地行数据。
4. 保存配置：调用 `updatePluginConfig`，成功后更新本地行数据并关闭弹窗。
5. 调整排序：调用 `updatePluginSortOrder`，成功后更新本地行数据。
6. 更新作用域：调用 `updatePluginScope`，成功后更新本地行数据。
7. 所有操作带 loading 状态和错误通知。

**Category 下拉选项**：`observability`, `guard`, `tracking`, `debug`, `routing`, `policy`

**风险等级颜色映射**：
- `high` → negative (红色)
- `medium` → warning (橙色)
- `low` → positive (绿色)

### 9.3 路由

| 路径 | 组件 | 说明 |
|------|------|------|
| `/plugins` | PluginsPage | Plugin 管理页 |

---

## 十、Plugin 种子同步机制

### 10.1 问题

内置插件定义在代码中（`adapter.go` 的 `builtin()` 函数），但运行时配置（启用状态、配置 JSON、排序）存储在数据库 `plugins` 表。系统启动时需要确保数据库中存在所有内置插件的记录。

### 10.2 设计方案

在 `PluginService` 构造时或服务启动阶段，执行种子同步：

```go
func (s *PluginService) seedBuiltinPlugins(ctx context.Context) {
    builtins := builtinPluginDefs()
    for _, def := range builtins {
        _, err := s.uc.GetByKey(ctx, def.Key)
        if err == sql.ErrNoRows {
            s.uc.Create(ctx, def)
        }
    }
    s.reloadRuntime(ctx)
}
```

### 10.3 内置插件定义表

在 `internal/plugin/trpc/registry.go` 中维护内置插件定义：

```go
type BuiltinPluginDef struct {
    Key               string
    Name              string
    Description       string
    Category          string
    RiskLevel         string
    DefaultEnabled    bool
    Scope             string
    CallbackPoints    []string
    SortOrder         int
    ConfigSchemaJSON  string
    DefaultConfigJSON string
}

func builtinPluginDefs() []BuiltinPluginDef {
    return []BuiltinPluginDef{
        {
            Key: "runtime_audit", Name: "运行日志和审计",
            Description: "记录 Agent 执行链路的完整日志",
            Category: "observability", RiskLevel: "low",
            DefaultEnabled: false, Scope: "global",
            CallbackPoints: []string{"before_agent", "after_agent", "before_model", "after_model", "before_tool", "after_tool", "on_event"},
            SortOrder: 100, ConfigSchemaJSON: runtimeAuditSchema, DefaultConfigJSON: runtimeAuditDefaults,
        },
        // ... 其余 8 个内置插件定义
    }
}
```

### 10.4 同步规则

1. **启动同步**：服务启动时，遍历 `builtinPluginDefs()`，对数据库中不存在的 key 执行 `Create`。
2. **不覆盖**：已存在的记录不更新（保留用户修改的配置和启用状态）。
3. **同步后热重载**：种子同步完成后调用 `reloadRuntime()`。
4. **新增插件**：后续版本新增内置插件时，只需在 `builtinPluginDefs()` 添加定义并更新 `builtin()` 的 switch case，下次启动自动同步。

### 10.5 Biz 层扩展

`PluginRepo` 需新增 `GetByKey`、`CreatePlugin`、`UpdateScope`、`IncrementStats` 方法（完整接口定义见 §3.2）。

`PluginUsecase` 对应新增（完整方法签名见 §3.3）：

```go
func (u *PluginUsecase) GetByKey(ctx context.Context, key string) (Plugin, error)
func (u *PluginUsecase) Create(ctx context.Context, p Plugin) (Plugin, error)
func (u *PluginUsecase) UpdateScope(ctx context.Context, id string, scope string) (Plugin, error)
```

### 10.6 Data 层实现

`internal/data/plugin.go` 需新增以下方法实现：

```go
func (r *pluginRepo) GetByKey(ctx context.Context, key string) (biz.Plugin, error) {
    row, err := r.data.entClient.PlatformPlugin.Query().
        Where(platformplugin.PluginKeyEQ(key), platformplugin.DeletedAtEQ("")).
        Only(ctx)
    if err != nil {
        if ent.IsNotFound(err) {
            return biz.Plugin{}, sql.ErrNoRows
        }
        return biz.Plugin{}, err
    }
    return entToBizPlugin(row), nil
}

func (r *pluginRepo) CreatePlugin(ctx context.Context, p biz.Plugin) (biz.Plugin, error) {
    cbsJSON, _ := json.Marshal(p.CallbackPoints)
    row, err := r.data.entClient.PlatformPlugin.Create().
        SetID(p.ID).
        SetPluginKey(p.Key).
        SetName(p.Name).
        SetDescription(p.Description).
        SetCategory(p.Category).
        SetRiskLevel(p.RiskLevel).
        SetEnabled(p.Enabled).
        SetScope(p.Scope).
        SetCallbackPointsJSON(string(cbsJSON)).
        SetSortOrder(p.SortOrder).
        SetConfigSchemaJSON(p.ConfigSchemaJSON).
        SetConfigJSON(p.ConfigJSON).
        SetFallbackConfigJSON(p.DefaultConfigJSON).
        SetCreatedAt(nowRFC3339()).
        SetUpdatedAt(nowRFC3339()).
        Save(ctx)
    if err != nil {
        return biz.Plugin{}, err
    }
    return entToBizPlugin(row), nil
}

func (r *pluginRepo) UpdateScope(ctx context.Context, id string, scope string) (biz.Plugin, error) {
    err := r.data.entClient.PlatformPlugin.UpdateOneID(id).
        SetScope(scope).
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

func (r *pluginRepo) IncrementStats(ctx context.Context, key string, delta biz.PluginStatUpdate) error {
    _, err := r.data.entClient.PlatformPlugin.Update().
        Where(platformplugin.PluginKeyEQ(key)).
        AddInvokeCount(delta.InvokeCount).
        AddBlockCount(delta.BlockDelta).
        AddErrorCount(delta.ErrorDelta).
        SetLastInvokedAt(nowRFC3339()).
        SetLastStatus(delta.LastStatus).
        Save(ctx)
    return err
}
```

---

## 十一、EP-CB-01 集成方案

### 11.1 当前状态

| 回调层 | 注入方式 | 状态 |
|--------|----------|------|
| Tool AfterTool | `trpcllmagent.WithToolCallbacks(callbacks)` | ✅ 已接入 |
| Plugin Runtime | `trpcrunner.WithPlugins(plugins...)` | ✅ 已接入 |
| Agent BeforeAgent/AfterAgent | `trpcllmagent.WithAgentCallbacks(callbacks)` | ❌ 未接入 |
| Model BeforeModel/AfterModel | `trpcllmagent.WithModelCallbacks(callbacks)` | ❌ 未接入 |

### 11.2 两条回调路径

Plugin 系统存在两条并行的回调注入路径，EP-CB-01 完成后需要统一：

**路径 A：Plugin Runtime → Runner 级别**

```
plugintrpc.Runtime.Plugins() → trpcrunner.WithPlugins(plugins...)
→ Runner 创建 plugin.Manager
→ Manager 在 Runner 执行期间自动调用 Agent/Model/Tool 回调
```

- 这是 `trpc-agent-go` 框架原生路径。
- Plugin 通过 `Register(r *Registry)` 注册回调到 `Manager`。
- `Manager.AgentCallbacks()` / `ModelCallbacks()` / `ToolCallbacks()` 返回框架原生回调集合。
- **当前仅 Tool 回调被框架自动触发**，Agent/Model 回调需要 EP-CB-01 挂载。

**路径 B：Callback Chain → Agent 级别**

```
callbacks.Chain → AdaptAgentCallbacks() / AdaptModelCallbacks() / AdaptToolCallbacks()
→ trpcllmagent.WithAgentCallbacks() / WithModelCallbacks() / WithToolCallbacks()
→ LLMAgent 构造时注入
```

- 这是 Aranea 产品层路径，位于 `internal/agent/callbacks/`。
- Chain 支持优先级排序和产品层 Hook 桥接。
- **当前仅 Tool 回调通过 Chain 接入**（`buildToolCallbacks`）。

### 11.3 统一方案

EP-CB-01 的核心是将路径 A 和路径 B 合并，使 Plugin Runtime 的回调通过 Chain 注入 LLMAgent：

```
Plugin Runtime → adaptPluginsToChainEntries() → callbacks.Chain
→ Chain.AdaptAgentCallbacks() / AdaptModelCallbacks() / AdaptToolCallbacks()
→ trpcllmagent.WithAgentCallbacks() / WithModelCallbacks() / WithToolCallbacks()
```

具体步骤：

1. **新增适配器**：在 `internal/plugin/trpc/` 下新增 `chain_adapter.go`，将 `trpcplugin.Plugin` 转换为 `callbacks.Callback` 条目。

```go
// chain_adapter.go 核心逻辑

// adaptPluginToChainEntries 将一个 trpcplugin.Plugin 拆解为多个 callbacks.Callback 条目。
// 实现方式：创建临时 Registry，让 Plugin 注册回调，然后从 Registry 中提取各回调点，
// 包装为 callbacks.BeforeAgentHookFunc / AfterAgentHookFunc / BeforeModelHookFunc / AfterModelHookFunc 等。
func adaptPluginToChainEntries(p trpcplugin.Plugin, priority int) []callbacks.Callback {
    reg := trpcplugin.NewRegistry()  // 临时 Registry
    p.Register(reg)                  // 让 Plugin 注册回调到临时 Registry

    var entries []callbacks.Callback

    // 从 reg 中提取 BeforeAgent 回调
    for _, cb := range reg.BeforeAgentCallbacks() {
        entries = append(entries, callbacks.NewBeforeAgentHook(priority, cb))
    }
    // 从 reg 中提取 AfterAgent 回调
    for _, cb := range reg.AfterAgentCallbacks() {
        entries = append(entries, callbacks.NewAfterAgentHook(priority, cb))
    }
    // BeforeModel / AfterModel 同理
    for _, cb := range reg.BeforeModelCallbacks() {
        entries = append(entries, callbacks.NewBeforeModelHook(priority, cb))
    }
    for _, cb := range reg.AfterModelCallbacks() {
        entries = append(entries, callbacks.NewAfterModelHook(priority, cb))
    }
    // BeforeTool / AfterTool
    for _, cb := range reg.BeforeToolCallbacks() {
        entries = append(entries, callbacks.NewBeforeToolHook(priority, cb))
    }
    for _, cb := range reg.AfterToolCallbacks() {
        entries = append(entries, callbacks.NewAfterToolHook(priority, cb))
    }
    return entries
}
```

> **注意**：`trpcplugin.Registry` 的具体导出 API 需要查看 `trpc-agent-go` 框架源码确认。如果 Registry 不暴露回调列表，则需要改用 `plugin.NewManager(p)` 创建 Manager，再从 `Manager.AgentCallbacks()` / `ModelCallbacks()` / `ToolCallbacks()` 中提取回调，包装为 Chain 条目。

2. **新增适配器函数**：在 `internal/agent/callbacks/adapter.go` 中补充 `BeforeModelHookFunc` 和 `AfterModelHookFunc`（参照已有的 `BeforeAgentHookFunc` 和 `AfterAgentHookFunc`）。

```go
type BeforeModelHookFunc struct {
    priority int
    fn       trpcmodel.BeforeModelCallbackStructured
}

func NewBeforeModelHook(priority int, fn trpcmodel.BeforeModelCallbackStructured) *BeforeModelHookFunc

type AfterModelHookFunc struct {
    priority int
    fn       trpcmodel.AfterModelCallbackStructured
}

func NewAfterModelHook(priority int, fn trpcmodel.AfterModelCallbackStructured) *AfterModelHookFunc
```

3. **修改 `BuildTRPCLLMAgent`**：在 `internal/agent/trpc_build.go` 中，构建 Chain 时合并 Plugin 回调和产品层 Hook。

```go
// 伪代码
chain := callbacks.NewChain()

// 1. 加入 Plugin Runtime 回调
for _, entry := range adaptPluginToChainEntries(plugins...) {
    chain = chain.Append(entry)
}

// 2. 加入产品层 Hook（未来）
// for _, hook := range hookResolver.Resolve(agentID) { ... }

// 3. 注入到 LLMAgent
opts = append(opts, trpcllmagent.WithAgentCallbacks(chain.AdaptAgentCallbacks()))
opts = append(opts, trpcllmagent.WithModelCallbacks(chain.AdaptModelCallbacks()))
opts = append(opts, trpcllmagent.WithToolCallbacks(chain.AdaptToolCallbacks()))
```

3. **保留 Runner 级别 Plugin 注入**：`trpcrunner.WithPlugins()` 继续用于 Runner 级别的 `OnEvent` 回调，因为 `OnEvent` 不属于 Agent/Model/Tool 生命周期。

### 11.4 EP-CB-01 完成后的回调触发矩阵

| 回调点 | 触发方式 | 当前 | EP-CB-01 后 |
|--------|----------|------|-------------|
| BeforeAgent | LLMAgent 构造时 WithAgentCallbacks | ❌ | ✅ |
| AfterAgent | LLMAgent 构造时 WithAgentCallbacks | ❌ | ✅ |
| BeforeModel | LLMAgent 构造时 WithModelCallbacks | ❌ | ✅ |
| AfterModel | LLMAgent 构造时 WithModelCallbacks | ❌ | ✅ |
| BeforeTool | LLMAgent 构造时 WithToolCallbacks | ✅ | ✅（经 Chain） |
| AfterTool | LLMAgent 构造时 WithToolCallbacks | ✅ | ✅（经 Chain） |
| OnEvent | Runner 级别 WithPlugins | ✅ | ✅ |

---

## 十二、Plugin 生命周期与错误处理

### 12.1 完整生命周期

```
1. 定义阶段：builtinPluginDefs() 定义内置插件元数据
2. 种子同步：服务启动 → seedBuiltinPlugins() → 写入 DB
3. 热重载：DB 变更 → reloadRuntime() → Runtime.Apply()
4. 适配：adapt(biz.Plugin) → builtin(key) → 具体 Plugin 实例
5. 注入 Runner：runtime.Plugins() → WithPlugins() / Chain → LLMAgent
6. 回调执行：Runner/Agent/Model/Tool 在固定节点触发回调
7. 统计更新：回调执行后更新 invoke_count / block_count / error_count
8. 关闭：plugin.Closer.Close(ctx) 逆序释放资源
```

### 12.2 错误处理策略

Plugin 回调错误不应导致 Agent 执行崩溃。策略如下：

| 场景 | 处理方式 |
|------|----------|
| Plugin `adapt()` 返回 nil | 静默跳过（未知 key 或 disabled） |
| Before* 回调返回 error | 框架默认中断执行链；建议通过 `WithContinueOnError(true)` 改为记录错误并继续 |
| After* 回调返回 error | 记录错误日志，不影响已完成的执行结果 |
| `CustomResponse` / `CustomResult` | 框架默认跳过后续同类回调；可通过 `WithContinueOnResponse(true)` 改变 |
| Plugin `Register()` panic | `plugin.NewManager()` 内部 recover，记录错误并跳过该 Plugin |
| `reloadRuntime()` 失败 | 记录 warn 日志，保留上一次 active 插件集 |

### 12.3 回调返回值语义

| 返回值 | 含义 | 对执行链的影响 |
|--------|------|----------------|
| `nil, nil` | 通过，不做修改 | 继续下一个回调 |
| `result, nil`（含 CustomResponse） | 拦截/改写 | 默认跳过后续同类回调 |
| `nil, error` | 回调执行出错 | 默认中断执行链 |
| `result, error` | 出错但有部分结果 | 默认中断执行链 |

---

## 十三、统计更新机制

### 13.1 当前状态

`plugins` 表有 `invoke_count`、`block_count`、`error_count`、`last_invoked_at`、`last_status` 字段，但当前无更新机制。

### 13.2 设计方案

在 Plugin 回调内部，通过 `safego.Go` 异步更新统计：

```go
func (a *AuditLogPlugin) afterToolCallback(ctx context.Context, args *trpctool.AfterToolArgs) (*trpctool.AfterToolResult, error) {
    // ... 执行审计逻辑 ...

    // 异步更新统计
    safego.Go(ctx, "plugin.updateStats", func() {
        status := "success"
        if args.Error != nil {
            status = "error"
        }
        a.statsUpdater.Update(ctx, a.name, PluginStatUpdate{
            InvokeCount: 1,
            ErrorDelta:  boolToInt(args.Error != nil),
            BlockDelta:  0,
            LastStatus:  status,
        })
    })

    return &trpctool.AfterToolResult{}, nil
}
```

### 13.3 StatsUpdater 接口

```go
type PluginStatUpdate struct {
    InvokeCount int
    BlockDelta  int
    ErrorDelta  int
    LastStatus  string
}

type StatsUpdater interface {
    Update(ctx context.Context, pluginKey string, delta PluginStatUpdate) error
}
```

实现方式：直接调用 `PluginRepo` 的增量更新方法（需新增）：

```go
type PluginRepo interface {
    // ... 现有方法 ...
    IncrementStats(ctx context.Context, key string, delta PluginStatUpdate) error
}
```

### 13.4 统计更新时机

| 回调点 | 更新 invoke_count | 更新 block_count | 更新 error_count | 更新 last_status |
|--------|-------------------|------------------|------------------|------------------|
| Before* 返回 CustomResponse | ✅ | ✅ | — | "blocked" |
| After* 正常返回 | ✅ | — | — | "success" |
| After* 检测到 args.Error | ✅ | — | ✅ | "error" |
| 回调自身返回 error | ✅ | — | ✅ | "error" |

### 13.5 注入方式

`StatsUpdater` 通过 Plugin 构造函数注入，在 `adapter.go` 的 `builtin()` 中传入：

```go
func builtin(p biz.Plugin, stats StatsUpdater) trpcplugin.Plugin {
    key := strings.ToLower(strings.TrimSpace(p.Key))
    switch key {
    case "audit_log", "audit-log", "auditlog":
        return &AuditLogPlugin{name: p.Key, stats: stats}
    // ...
    }
}
```

`Runtime.Apply()` 调用 `adapt()` 时传入 `StatsUpdater` 实例。

---

## 十四、配置校验机制

### 14.1 当前状态

`PluginUsecase.UpdateConfig` 仅校验 `json.Valid()`，不校验配置是否符合 `config_schema_json` 定义的 JSON Schema。

### 14.2 设计方案

在 `PluginUsecase.UpdateConfig` 中增加 JSON Schema 校验：

```go
func (u *PluginUsecase) UpdateConfig(ctx context.Context, id string, configJSON string) (Plugin, error) {
    if strings.TrimSpace(id) == "" {
        return Plugin{}, errors.BadRequest("PLUGIN", "id is required")
    }
    if strings.TrimSpace(configJSON) == "" {
        configJSON = "{}"
    }
    if !json.Valid([]byte(configJSON)) {
        return Plugin{}, errors.BadRequest("PLUGIN", "config_json must be valid JSON")
    }

    // JSON Schema 校验
    p, err := u.repo.GetPlugin(ctx, id)
    if err != nil {
        return Plugin{}, err
    }
    if schema := strings.TrimSpace(p.ConfigSchemaJSON); schema != "" && schema != "{}" {
        if err := validateJSONSchema(schema, configJSON); err != nil {
            return Plugin{}, errors.BadRequest("PLUGIN", fmt.Sprintf("config validation failed: %v", err))
        }
    }

    return u.repo.UpdatePluginConfig(ctx, id, configJSON)
}
```

### 14.3 JSON Schema 校验库

推荐使用 `github.com/xeipuuv/gojsonschema`，需在 `go.mod` 中添加依赖。

```go
func validateJSONSchema(schemaJSON, docJSON string) error {
    schemaLoader := gojsonschema.NewStringLoader(schemaJSON)
    docLoader := gojsonschema.NewStringLoader(docJSON)
    result, err := gojsonschema.Validate(schemaLoader, docLoader)
    if err != nil {
        return fmt.Errorf("schema validation error: %w", err)
    }
    if !result.Valid() {
        var msgs []string
        for _, desc := range result.Errors() {
            msgs = append(msgs, desc.String())
        }
        return fmt.Errorf("config does not match schema: %s", strings.Join(msgs, "; "))
    }
    return nil
}
```

---

## 十五、Agent 绑定机制

### 15.1 当前状态

`plugins` 表有 `scope` 字段（"global" 或 agent_id），但运行时未消费此字段——`Runtime.Apply()` 加载所有 enabled 插件，不区分 scope。

### 15.2 设计方案

**方案：Runner 创建时按 scope 过滤**

`Runtime.Apply()` 仍加载所有 enabled 插件（含 scope 信息），但在 `Plugins()` 方法中增加 scope 过滤参数：

```go
type ScopedPlugin struct {
    Plugin trpcplugin.Plugin
    Scope  string // "global" 或 agent_id
}

type Runtime struct {
    mu     sync.RWMutex
    active []ScopedPlugin
}

func (rt *Runtime) PluginsForAgent(agentID string) []trpcplugin.Plugin {
    rt.mu.RLock()
    defer rt.mu.RUnlock()
    var out []trpcplugin.Plugin
    for _, sp := range rt.active {
        if sp.Scope == "global" || sp.Scope == agentID {
            out = append(out, sp.Plugin)
        }
    }
    return out
}
```

调用方修改：

```go
// internal/service/trpc_turn.go
plugins := s.pluginRT.PluginsForAgent(agentID)
runnerDeps := chatagent.NewRunnerDepsFromRuntime(s.td.Persist.Session, s.td.Persist.Memory, plugins...)
```

### 15.3 前端交互

Agent 绑定的 UI 交互设计参见需求文档 §8.3。

### 15.4 scope 字段扩展

当前 `scope` 为单值（"global" 或单个 agent_id）。如需支持多 Agent 绑定，可改为逗号分隔的 agent_id 列表或新增关联表。本期建议保持单值 + global，后续迭代扩展。
