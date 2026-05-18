# MCP 集成模块 — 实现设计文档

> 对应需求：`19 mcp.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

MCP（Model Context Protocol）服务器管理：注册、发现、工具挂载、连接池。核心包 `internal/tools/mcpmount/`。

---

## 二、Proto 层

文件：`api/kratos/mcp_server/v1/mcp_server.proto`

```protobuf
service MCPServerService {
  rpc ListMCPServers(google.protobuf.Empty) returns (ListMCPServersResponse) {
    option (google.api.http) = { get: "/v1/mcp-servers" };
  }
  rpc CreateMCPServer(CreateMCPServerRequest) returns (MCPServer) {
    option (google.api.http) = { post: "/v1/mcp-servers" body: "*" };
  }
  rpc GetMCPServer(GetMCPServerRequest) returns (MCPServer) {
    option (google.api.http) = { get: "/v1/mcp-servers/{id}" };
  }
  rpc UpdateMCPServer(UpdateMCPServerRequest) returns (MCPServer) {
    option (google.api.http) = { patch: "/v1/mcp-servers/{id}" body: "mcp_server" };
  }
  rpc DeleteMCPServer(DeleteMCPServerRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = { delete: "/v1/mcp-servers/{id}" };
  }
  rpc TestMCPServer(TestMCPServerRequest) returns (MCPServerTestResponse) {
    option (google.api.http) = { post: "/v1/mcp-servers/{id}/test" body: "*" };
  }
}
```

**MCPServer 消息**：`id` / `key` / `name` / `description` / `status` / `enabled` / `sort_order` / `config_json` / `metadata_json` / `created_at` / `updated_at` / `deleted_at`

**MCPServerTestResponse 消息**：`ok` / `status` / `message` / `details_json`

---

## 三、Biz 层

### 3.1 领域模型

```go
type MCPServer struct {
    ID           string
    Key          string
    Name         string
    Description  string
    Status       string
    Enabled      bool
    SortOrder    int
    ConfigJSON   string
    MetadataJSON string
    CreatedAt    string
    UpdatedAt    string
    DeletedAt    string
}
```

`ConfigJSON` 承载传输配置（transport / url / command / args / headers / env / tool_prefix / timeout_sec / require_user_credentials），由 `mcpmount.ServerConfig` 解析。

`MetadataJSON` 承载健康元数据（health_status / last_health_at / last_error_message），由前端 `McpServerMetadata` 解析。

### 3.2 Usecase

```go
func (u *MCPServerUsecase) List(ctx) ([]MCPServer, error)
func (u *MCPServerUsecase) Get(ctx, id) (MCPServer, error)
func (u *MCPServerUsecase) Create(ctx, in MCPServer) (MCPServer, error)
func (u *MCPServerUsecase) Update(ctx, id, patch MCPServer) (MCPServer, error)
func (u *MCPServerUsecase) Delete(ctx, id) error
func (u *MCPServerUsecase) TestMCPServer(ctx, id) (mcpprobe.TestResult, error)
```

### 3.3 Agent MCP 关联

```go
type AgentMCPTooling struct {
    agents *AgentUsecase
    mcp    *MCPServerUsecase
}

func (t *AgentMCPTooling) EffectiveServersForAgent(ctx, agentID) ([]EffectiveMCPServer, error)
```

**生效规则**：Agent 需启用 `mcp_tool_set`（`biz.ToolKeyMCPToolSet`）。在 `tools_allow_json` / `tools_deny_json` 中使用 `mcp:<server_key>` 前缀限制挂载的服务器列表；未配置任何 `mcp:` 项时为「所有已启用且 active 的平台服务器」。

### 3.4 MCP 策略过滤

```go
func MCPPolicyFromAgentEffectiveTools(eff AgentEffectiveTools) EffectiveMCPPolicy
func FilterEffectiveMCPServers(servers []EffectiveMCPServer, pol EffectiveMCPPolicy) []EffectiveMCPServer
```

---

## 四、Data 层

### 4.1 Ent Schema

文件：`internal/data/ent/schema/platform_mcp_server.go`

表名 `mcp_server`，关键字段：
- `id` (STRING, immutable, unique, max 256)
- `server_key` (STRING, unique, max 512)
- `name` (STRING, max 1024)
- `description` (TEXT, default "")
- `status` (STRING, default "active")
- `enabled` (BOOL, default true)
- `sort_order` (INT, default 0)
- `config_json` (TEXT, default "")
- `metadata_json` (TEXT, default "")
- `created_at` / `updated_at` / `deleted_at` (STRING)

### 4.2 Repo 接口

```go
type MCPServerRepo interface {
    ListMCPServers(ctx) ([]MCPServer, error)
    GetMCPServer(ctx, id) (MCPServer, error)
    CreateMCPServer(ctx, m MCPServer) (MCPServer, error)
    UpdateMCPServer(ctx, m MCPServer) (MCPServer, error)
    DeleteMCPServer(ctx, id) error
}
```

删除为软删除（设置 `deleted_at` + `status="deleted"`）。

---

## 五、运行时层

### 5.1 MCP 工具挂载

`internal/tools/mcpmount/` 提供 `ServerConfig` 解析与 `trpcmcp.ConnectionConfig` 转换：

```go
func parseServerConfigJSON(raw string) (ServerConfig, error)
func toTRPCConnectionConfig(sc ServerConfig) trpcmcp.ConnectionConfig
```

`internal/tools/toolset.go` 中 `buildMCPToolSet` 和 `buildMCPBrokerTools` 负责将配置转为 trpc-agent-go 的 `MCPToolSet` / `MCPBroker` 实例。

### 5.2 MCPBroker

`internal/tools/toolset.go` 中 `buildMCPBrokerTools` 已集成 `trpcmcpbroker.New`，提供运行时 MCP 发现工具（`mcp_list_servers` / `mcp_list_tools` / `mcp_inspect_tools` / `mcp_call`）。

### 5.3 连通性探测

`internal/mcpprobe/eval.go` 提供 `Evaluate(enabled, configJSON)` 函数：
- `stdio`：校验 command 路径是否在 PATH 中
- `sse` / `streamable_http`：HTTP GET 请求 + SSRF 校验（禁止 localhost / 私有地址）

### 5.4 健康检查定时探活

`internal/mcphealth/runner.go` 提供后台定时探活：

```go
type Runner struct { deps Deps }
func (r *Runner) Start(ctx, interval)
func (r *Runner) probeAll(ctx)
func (r *Runner) probeOne(ctx, srv MCPServer)
```

- 默认间隔 5 分钟，可通过 `MCP_HEALTH_INTERVAL` 环境变量配置
- 设置 `MCP_HEALTH_DISABLED=1` 可禁用
- 遍历所有 `enabled` 且未删除的 MCP Server，调用 `mcpprobe.Evaluate` 探活
- 探活结果通过 `MCPServerUsecase.PersistHealth` 写入 `metadata_json`（health_status / last_health_at / last_error_message）
- 每次探活并发执行（`safego.Go`），互不阻塞
- Prometheus 指标：`aranea_mcp_health_probe_total` / `aranea_mcp_health_probe_duration_seconds`

### 5.5 运行时装配链路

```
AgentMCPTooling.EffectiveServersForAgent
  → MCPServerUsecase.List (过滤 enabled + active)
  → FilterEffectiveMCPServers (apply mcp: allow/deny policy)
  → mcpmount.ServerConfig → trpcmcp.ConnectionConfig
  → buildMCPToolSet / buildMCPBrokerTools
  → Agent BuilderDeps.Toolsets
```

---

## 六、Service 层

```go
func (s *MCPServerService) ListMCPServers(ctx, _) (*ListMCPServersResponse, error)
func (s *MCPServerService) GetMCPServer(ctx, req) (*MCPServer, error)
func (s *MCPServerService) CreateMCPServer(ctx, req) (*MCPServer, error)
func (s *MCPServerService) UpdateMCPServer(ctx, req) (*MCPServer, error)
func (s *MCPServerService) DeleteMCPServer(ctx, req) (*emptypb.Empty, error)
func (s *MCPServerService) TestMCPServer(ctx, req) (*MCPServerTestResponse, error)
```

`TestMCPServer` 调用 `mcpprobe.Evaluate` 并持久化健康元数据到 `metadata_json`。

---

## 七、Wire 注入

已有：`NewMCPServerUsecase(repo)` → `NewMCPServerService(uc)` → `NewAgentMCPTooling(agents, mcp)` → `runtime.PersistenceSet.AgentMCP`

新增：`provideMCPHealthRunnerDeps(mcpRepo, mcpUC)` → `provideMCPHealthRunner(deps)` → `wireOut.MCPHealthProbe` → `main.go` 启动

---

## 八、Web 前端设计

### 8.1 文件结构

```
web/src/features/mcp/
├── api.ts                    — CRUD + TestMCPServer 客户端
├── types.ts                  — McpTransport / McpServerConfig / McpServerMetadata / McpServerFormValue
├── McpServerFormDialog.vue   — 添加/编辑对话框
└── McpServerItem.vue         — 服务器卡片组件

web/src/pages/
└── McpServersPage.vue        — 列表页
```

### 8.2 组件设计

**McpServerFormDialog.vue**：

| 控件 | 绑定 | 说明 |
|------|------|------|
| `QInput` name | `form.name` | 必填，slug 校验 |
| `QInput` display_name | `form.display_name` | 展示用名称 |
| `QBtnToggle` transport | `form.transport` | stdio / SSE / Streamable HTTP |
| `QInput` URL | `form.url` | SSE/HTTP 模式显示 |
| `QInput` command | `form.command` | stdio 模式显示 |
| `QInput` args | `form.argsText` | 每行一个参数 |
| 动态键值 headers | `form.headers` | key + value 行 |
| 动态键值 env | `form.env` | key + value 行 |
| `QInput` tool_prefix | `form.tool_prefix` | 前缀 `mcp_` |
| `QInput` timeout_sec | `form.timeout_sec` | 默认 60s |
| `QToggle` enabled | `form.enabled` | 默认开 |
| `QToggle` require_user_credentials | `form.require_user_credentials` | 默认关 |
| `QBtn` 测试连接 | — | 先保存再测试 |

**McpServerItem.vue**：MCP 服务器卡片，显示名称/传输/地址/状态灯/健康信息/操作按钮

**McpServersPage.vue**：列表页，含搜索/状态灯/空态/刷新/CRUD

### 8.3 API

```typescript
listMcpServers(): Promise<PlatformResource[]>
createMcpServer(payload: PlatformResourceInput): Promise<PlatformResource>
updateMcpServer(id: string, payload: Partial<PlatformResourceInput>): Promise<PlatformResource>
deleteMcpServer(id: string): Promise<void>
testMcpServer(id: string): Promise<McpServerTestResult>
```

---

*文档版本：2.0 — 2026-05-18 文档治理：与代码实现对齐，移除"待实现"标记（迁移至 `19-mcp-development.md`），补全 Proto/Biz/Data/Service/运行时层现状。*
