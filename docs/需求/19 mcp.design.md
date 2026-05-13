# MCP 集成模块 — 实现设计文档

> 对应需求：`19 mcp.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

MCP（Model Context Protocol）服务器管理：注册、发现、工具挂载、连接池。核心包 `internal/tools/mcpmount/`。

---

## 二、Proto 层

### 2.1 现有 Proto

文件：`api/kratos/mcp_server/v1/mcp_server.proto`

```protobuf
service McpServerService {
  rpc ListMcpServers(ListMcpServersRequest) returns (ListMcpServersResponse) {
    option (google.api.http) = { get: "/v1/mcp-servers" };
  }
  rpc CreateMcpServer(CreateMcpServerRequest) returns (McpServer) {
    option (google.api.http) = { post: "/v1/mcp-servers" body: "*" };
  }
  rpc UpdateMcpServer(UpdateMcpServerRequest) returns (McpServer) {
    option (google.api.http) = { patch: "/v1/mcp-servers/{id}" body: "*" };
  }
  rpc DeleteMcpServer(DeleteMcpServerRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = { delete: "/v1/mcp-servers/{id}" };
  }
}
```

### 2.2 待新增

| RPC | 路径 | 用途 |
|-----|------|------|
| `TestMcpConnection` | `POST /v1/mcp-servers/{id}/test` | 测试 MCP 连接 |
| `DiscoverMcpTools` | `GET /v1/mcp-servers/{id}/tools` | 发现 MCP 工具 |
| `GetMcpServerStatus` | `GET /v1/mcp-servers/{id}/status` | 连接状态 |

---

## 三、Biz 层

### 3.1 领域模型

```go
type MCPServer struct {
    ID           string
    Name         string
    ServerType   string  // "sse"/"streamable_http"/"stdio"
    BaseURL      string
    Command      string  // stdio 模式
    Args         string
    EnvJSON      string
    HeadersJSON  string
    Status       string  // "active"/"inactive"/"error"
    CreatedAt    string
    UpdatedAt    string
}
```

### 3.2 Usecase

```go
func (uc *MCPServerUsecase) List(ctx, query) (MCPServerListResult, error)
func (uc *MCPServerUsecase) Create(ctx, m MCPServer) (MCPServer, error)
func (uc *MCPServerUsecase) Update(ctx, m MCPServer) (MCPServer, error)
func (uc *MCPServerUsecase) Delete(ctx, id) error
```

### 3.3 Agent MCP 关联

```go
// internal/biz/agent_mcp_effective.go
type AgentMCPTooling struct {
    agents *AgentUsecase
    mcp    *MCPServerUsecase
}

func (t *AgentMCPTooling) EffectiveServers(ctx, agentID string) ([]MCPServer, error)
```

---

## 四、Data 层

### 4.1 Ent Schema

文件：`internal/data/ent/schema/platform_mcp_server.go`

关键字段：
- `name` (TEXT, unique)
- `server_type` (TEXT)
- `base_url` (TEXT, optional)
- `command` (TEXT, optional)
- `args` (TEXT, optional)
- `env_json` (TEXT, optional)
- `headers_json` (TEXT, optional)

---

## 五、运行时层

### 5.1 MCP 工具挂载

```go
// internal/tools/mcpmount/
func AppendEffectiveMCPServerToolsets(ctx, ag, mcpUC, catalog) ([]tool.Toolset, error)
```

### 5.2 MCPBroker（待实现）

```go
// internal/tools/mcpbroker/
type Broker struct {
    mu      sync.RWMutex
    clients map[string]*MCPClient
}

func (b *Broker) GetClient(ctx, serverID string) (*MCPClient, error)
func (b *Broker) CloseAll() error
```

### 5.3 运行时发现（待实现）

Agent 运行时通过 `mcp_tool_search` 工具搜索可用 MCP 工具。

---

## 六、Service 层

```go
func (s *MCPServerService) ListMcpServers(ctx, req) (*ListMcpServersResponse, error)
func (s *MCPServerService) CreateMcpServer(ctx, req) (*McpServer, error)
func (s *MCPServerService) UpdateMcpServer(ctx, req) (*McpServer, error)
func (s *MCPServerService) DeleteMcpServer(ctx, req) (*emptypb.Empty, error)
```

---

## 七、Wire 注入

已有，无需新增。

---

## 八、Web 前端设计

### 8.1 文件结构

```
web/src/features/mcp/
├── api.ts
├── types.ts
├── McpServerFormDialog.vue
├── McpServerItem.vue
└── components/
    └── McpServerListPage.vue
```

### 8.2 组件设计

**McpServerFormDialog.vue**：

| 控件 | 绑定 | 说明 |
|------|------|------|
| `QInput` 名称 | `name` | 必填 |
| `QSelect` 类型 | `serverType` | sse/streamable_http/stdio |
| `QInput` URL | `baseURL` | SSE/HTTP 模式 |
| `QInput` 命令 | `command` | stdio 模式 |
| `QInput` 参数 | `args` | stdio 模式 |
| `QBtn` 测试 | — | 测试连接 |

**McpServerItem.vue**：MCP 服务器卡片，显示名称/类型/状态/工具数量

### 8.3 API

```typescript
export async function listMcpServers(query: McpServerQuery): Promise<McpServerListResult>
export async function createMcpServer(req: CreateMcpServerRequest): Promise<McpServer>
export async function updateMcpServer(id: string, req: UpdateMcpServerRequest): Promise<McpServer>
export async function deleteMcpServer(id: string): Promise<void>
export async function testMcpConnection(id: string): Promise<TestResult>
export async function discoverMcpTools(id: string): Promise<McpTool[]>
```
