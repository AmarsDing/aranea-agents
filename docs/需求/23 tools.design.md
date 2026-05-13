# Tools 工具模块 — 实现设计文档

> 对应需求：`23 tools.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

工具注册与管理：FunctionTool/StreamableTool/MCP Tool 统一目录、Agent 工具绑定、运行时挂载。核心包 `internal/tools/`。

---

## 二、Proto 层

### 2.1 现有 Proto

文件：`api/kratos/tool/v1/tool.proto`

```protobuf
service ToolService {
  rpc ListTools(ListToolsRequest) returns (ListToolsResponse) {
    option (google.api.http) = { get: "/v1/tools" };
  }
  rpc CreateTool(CreateToolRequest) returns (Tool) {
    option (google.api.http) = { post: "/v1/tools" body: "*" };
  }
  rpc UpdateTool(UpdateToolRequest) returns (Tool) {
    option (google.api.http) = { patch: "/v1/tools/{id}" body: "*" };
  }
  rpc DeleteTool(DeleteToolRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = { delete: "/v1/tools/{id}" };
  }
}
```

### 2.2 待新增

| RPC | 路径 | 用途 |
|-----|------|------|
| `GetToolSchema` | `GET /v1/tools/{id}/schema` | 工具参数 Schema |
| `TestTool` | `POST /v1/tools/{id}/test` | 测试工具执行 |

---

## 三、Biz 层

### 3.1 领域模型

```go
type Tool struct {
    ID          string
    Name        string
    DisplayName string
    Description string
    ToolType    string  // "function"/"streamable"/"mcp"
    Parameters  string  // JSON Schema
    Code        string  // function 类型：Go 代码或脚本
    MCPServerID string  // mcp 类型：关联 MCP 服务器
    MCPToolName string  // mcp 类型：MCP 工具名
    Status      string
    CreatedAt   string
    UpdatedAt   string
}

type AgentTool struct {
    ID        string
    AgentID   string
    ToolID    string
    SortOrder int32
}
```

### 3.2 Repo 接口

```go
type ToolRepo interface {
    List(ctx, query) (ToolListResult, error)
    GetByID(ctx, id) (Tool, error)
    Create(ctx, t Tool) (Tool, error)
    Update(ctx, t Tool) (Tool, error)
    Delete(ctx, id) error
    EffectiveTools(ctx, agentID string) ([]Tool, error)
}
```

---

## 四、Data 层

### 4.1 Ent Schema

- `internal/data/ent/schema/tool.go` — Tool 主表
- `internal/data/ent/schema/agent_tool.go` — Agent-Tool 关联表

---

## 五、运行时层

### 5.1 工具挂载

```go
// internal/agent/trpc_build.go
func AppendEffectiveToolsets(ctx, ag, catalog, mcpUC, skillUC) ([]tool.Toolset, error)
```

### 5.2 内置工具

```go
// internal/tools/builtin/
var BuiltinTools = []Tool{
    {Name: "web_search", ToolType: "function", ...},
    {Name: "code_interpreter", ToolType: "function", ...},
}
```

### 5.3 FunctionTool 构建

```go
// internal/tools/trpc_tool.go
func BuildTRPCFunctionTool(t biz.Tool) (tool.Tool, error)
func BuildTRPCStreamableTool(t biz.Tool) (tool.Tool, error)
```

### 5.4 trpc-agent-go 工具集成（待实现）

```go
// internal/tools/trpc_adapter.go
func AdaptToTRPCTool(t biz.Tool) (tool.Tool, error)
```

---

## 六、Service 层

```go
func (s *ToolService) ListTools(ctx, req) (*ListToolsResponse, error)
func (s *ToolService) CreateTool(ctx, req) (*Tool, error)
func (s *ToolService) UpdateTool(ctx, req) (*Tool, error)
func (s *ToolService) DeleteTool(ctx, req) (*emptypb.Empty, error)
```

---

## 七、Wire 注入

已有，无需新增。

---

## 八、Web 前端设计

### 8.1 文件结构

```
web/src/features/tools/
├── api.ts
├── types.ts
├── ToolEditorDialog.vue
├── ToolItem.vue
└── components/
    ├── ToolListPage.vue
    └── ToolSchemaEditor.vue
```

### 8.2 组件设计

**ToolEditorDialog.vue**：

| 控件 | 绑定 | 说明 |
|------|------|------|
| `QInput` 名称 | `name` | 必填 |
| `QSelect` 类型 | `toolType` | function/streamable/mcp |
| `QEditor` 描述 | `description` | Markdown |
| `QCodeEditor` 参数 | `parameters` | JSON Schema |
| `QCodeEditor` 代码 | `code` | function 类型 |
| `QSelect` MCP 服务器 | `mcpServerID` | mcp 类型 |

**ToolSchemaEditor.vue**：JSON Schema 可视化编辑器

### 8.3 API

```typescript
export async function listTools(query: ToolQuery): Promise<ToolListResult>
export async function createTool(req: CreateToolRequest): Promise<Tool>
export async function updateTool(id: string, req: UpdateToolRequest): Promise<Tool>
export async function deleteTool(id: string): Promise<void>
export async function getToolSchema(id: string): Promise<JSONSchema>
export async function testTool(id: string, args: Record<string, any>): Promise<TestResult>
```
