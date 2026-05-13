# Plugin 插件模块 — 实现设计文档

> 对应需求：`22 plugin.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

插件管理：注册、启用/禁用、配置、Agent 绑定。Plugin 是比 Skill 更大的功能单元，可包含多个工具+提示词+UI 扩展。

---

## 二、Proto 层

### 2.1 待新增

```protobuf
service PluginService {
  rpc ListPlugins(ListPluginsRequest) returns (ListPluginsResponse) {
    option (google.api.http) = { get: "/v1/plugins" };
  }
  rpc GetPlugin(GetPluginRequest) returns (Plugin) {
    option (google.api.http) = { get: "/v1/plugins/{id}" };
  }
  rpc InstallPlugin(InstallPluginRequest) returns (Plugin) {
    option (google.api.http) = { post: "/v1/plugins" body: "*" };
  }
  rpc UninstallPlugin(UninstallPluginRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = { delete: "/v1/plugins/{id}" };
  }
  rpc TogglePlugin(TogglePluginRequest) returns (Plugin) {
    option (google.api.http) = { patch: "/v1/plugins/{id}/toggle" };
  }
  rpc ConfigurePlugin(ConfigurePluginRequest) returns (Plugin) {
    option (google.api.http) = { patch: "/v1/plugins/{id}/config" body: "*" };
  }
}
```

---

## 三、Biz 层

### 3.1 领域模型

```go
type Plugin struct {
    ID          string
    Name        string
    DisplayName string
    Description string
    Version     string
    Author      string
    ConfigSchema string  // JSON Schema
    ConfigJSON   string
    Status      string  // "installed"/"active"/"error"
    CreatedAt   string
    UpdatedAt   string
}

type AgentPlugin struct {
    ID        string
    AgentID   string
    PluginID  string
    Enabled   bool
    ConfigJSON string
}
```

### 3.2 Usecase

```go
func (uc *PluginUsecase) List(ctx, query) (PluginListResult, error)
func (uc *PluginUsecase) Install(ctx, p Plugin) (Plugin, error)
func (uc *PluginUsecase) Uninstall(ctx, id) error
func (uc *PluginUsecase) Toggle(ctx, id) (Plugin, error)
func (uc *PluginUsecase) Configure(ctx, id, config string) (Plugin, error)
```

---

## 四、Data 层

### 4.1 Ent Schema

- `internal/data/ent/schema/plugin.go` — Plugin 主表
- `internal/data/ent/schema/agent_plugin.go` — Agent-Plugin 关联表

---

## 五、Service 层

```go
func (s *PluginService) ListPlugins(ctx, req) (*ListPluginsResponse, error)
func (s *PluginService) InstallPlugin(ctx, req) (*Plugin, error)
func (s *PluginService) UninstallPlugin(ctx, req) (*emptypb.Empty, error)
func (s *PluginService) TogglePlugin(ctx, req) (*Plugin, error)
func (s *PluginService) ConfigurePlugin(ctx, req) (*Plugin, error)
```

---

## 六、Wire 注入

待新增：
```
data.ProviderSet → NewPluginRepo
biz.ProviderSet → NewPluginUsecase
service.ProviderSet → NewPluginService
```

---

## 七、Web 前端设计

### 7.1 文件结构

```
web/src/features/plugins/
├── api.ts
├── types.ts
├── PluginConfigDialog.vue
├── PluginItem.vue
└── components/
    ├── PluginListPage.vue
    └── PluginMarketplace.vue
```

### 7.2 组件设计

**PluginListPage.vue**：已安装插件列表 + 启用/禁用/配置/卸载

**PluginMarketplace.vue**：插件市场浏览（未来扩展）

**PluginConfigDialog.vue**：根据 `configSchema` 动态渲染配置表单

### 7.3 API

```typescript
export async function listPlugins(query: PluginQuery): Promise<PluginListResult>
export async function installPlugin(req: InstallPluginRequest): Promise<Plugin>
export async function uninstallPlugin(id: string): Promise<void>
export async function togglePlugin(id: string): Promise<Plugin>
export async function configurePlugin(id: string, config: string): Promise<Plugin>
```
