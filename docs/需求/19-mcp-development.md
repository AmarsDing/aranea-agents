# MCP 协议 — 开发计划

> **版本**：2026-05-18 | **状态**：🟡 部分实现
> **需求**：[19 mcp.md](./19%20mcp.md) · **设计**：[19 mcp.design.md](./19%20mcp.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：—

---

## 1. 模块定位

MCP（Model Context Protocol）集成：支持 Agent 通过 MCP 协议连接外部工具服务器，动态发现和调用工具。

**代码锚点**：
- `api/kratos/mcp_server/v1/` — MCPServer CRUD + Test RPC
- `internal/service/mcp_server.go` — MCPServerService
- `internal/biz/mcp_server.go` — MCPServerUsecase
- `internal/biz/agent_mcp_effective.go` — Effective MCP 计算 + 策略过滤
- `internal/data/mcp_server.go` — MCPServerRepo（Ent ORM）
- `internal/data/ent/schema/platform_mcp_server.go` — Ent Schema
- `internal/mcpprobe/eval.go` — 连通性探测（stdio/HTTP + SSRF 校验）
- `internal/mcphealth/runner.go` — 后台定时健康探活（MCP_HEALTH_INTERVAL / MCP_HEALTH_DISABLED）
- `internal/tools/mcpmount/` — ServerConfig 解析 + ConnectionConfig 转换
- `internal/tools/toolset.go` — buildMCPToolSet / buildMCPBrokerTools
- `internal/runtime/deps.go` — PersistenceSet.AgentMCP
- `web/src/features/mcp/` — 前端 CRUD + 测试连接
- `web/src/pages/McpServersPage.vue` — 列表页

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| MCPServer CRUD | ✅ | Create/Update/Delete/Get/List + TestMCPServer |
| MCP 连通性探测 | ✅ | `mcpprobe.Evaluate`：stdio 路径校验 + HTTP 连接测试 + SSRF 防护 |
| MCP 工具挂载 | ✅ | `buildMCPToolSet` → `trpcmcp.NewMCPToolSet` |
| MCPBroker 集成 | ✅ | `buildMCPBrokerTools` → `trpcmcpbroker.New` |
| Effective MCP 策略 | ✅ | `AgentMCPTooling.EffectiveServersForAgent` + `mcp:` allow/deny 过滤 |
| 前端管理 | ✅ | McpServersPage + McpServerFormDialog + McpServerItem + 测试连接 |
| 健康元数据持久化 | ✅ | `TestMCPServer` → `persistHealth` 写入 metadata_json |
| 健康检查定时任务 | ✅ | `internal/mcphealth/runner.go`：后台定时探活，写入 metadata_json 的 health_status / last_health_at |
| MCP 工具调用超时 | 🟡 | `config_json.timeout_sec` 已有字段，但运行时未强制执行 |
| MCP Server 认证 | ❌ | 仅 headers 传静态 key，无 OAuth2 / API Key 动态认证 |
| MCP 会话重连 | ❌ | SSE/StreamableHTTP 断线后无自动重连 |
| MCP 运行时发现 | 🟡 | MCPBroker 已提供 `mcp_list_servers` 等工具，但未在 Agent 默认启用 |
| MCP 调用统计闭环 | ❌ | 未与 session 的 `MCPCallCount` 统计打通 |

---

## 3. 运行时装配现状（2026-05-18 对齐）

| 层次 | 状态 | 说明 |
|------|------|------|
| 数据与 API | ✅ | `mcp_server` 表（Ent）、`MCPServerService` gRPC/HTTP、增删改查与 `mcpprobe` 连通性测试 |
| MCP 客户端 | ✅ | `trpcmcp.NewMCPToolSet` + `trpcmcpbroker.New`，已集成于 `internal/tools/toolset.go` |
| 统一装配器 | ✅ | `buildMCPToolSet` / `buildMCPBrokerTools` 在 `Assemble` 中按 `AssemblyConfig.MCPServers` / `MCPBroker` 挂载 |
| 聊天 / Team Runner | ✅ | `AgentMCPTooling` + `mcpmount.ServerConfig`；Wire 通过 `runtime.PersistenceSet.AgentMCP` 注入 |
| Agent 级 MCP 策略 | ✅ | `EffectiveServersForAgent` + `mcp:<server_key>` allow/deny 过滤 |

---

## 4. 演进方向

| 方向 | 现状与问题 | 建议 |
|------|------------|------|
| 健康检查定时任务 | 仅手动测试连接，无后台探活 | 增加 Cron 定时探活，写入 `metadata_json` 的 `health_status` / `last_health_at` |
| MCP 闭环 | `MCPServer` → mcptoolset → BuilderDeps.Toolsets 链路需与 session 的 `MCPCallCount` 统计打通 | 补齐从配置到运行时到统计的端到端验证 |
| 超时与资源 | `timeout_sec` 字段已有但运行时未强制执行 | 在 `buildMCPToolSet` 中将 `timeout_sec` 传入 `ConnectionConfig.Timeout`，确保工具调用超时 |
| 会话重连 | SSE/StreamableHTTP 断线后无自动重连 | 依赖 trpc-agent-go `mcpbroker` 的重连能力，需验证并配置重连策略 |
| 认证配置 | 仅 headers 传静态 key | 增加 OAuth2 / API Key 动态认证支持，与 Provider 凭据加密对齐 |
| 配置来源统一 | MCP 未来将混用 DB 行 + 环境占位 | 在 conf 或单一模块中写下优先级表与是否支持热更新 |

---

## 5. 开发阶段

- **Phase 1**：MCP Server 健康检查定时任务 + 超时强制执行
- **Phase 2**：MCP 调用统计闭环 + MCPBroker 默认启用
- **Phase 3**：MCP Server 认证配置 + 会话重连验证

---

## 6. 任务清单

| # | 任务 | 优先级 | EP | 阶段 |
|---|------|--------|-----|------|
| 1 | ~~MCP Server 健康检查定时任务（Cron 探活 → 更新 metadata_json）~~ ✅ `internal/mcphealth/runner.go` | P2 | — | Phase 1 |
| 2 | MCP 工具调用超时强制执行（`timeout_sec` → `ConnectionConfig.Timeout`） | P2 | — | Phase 1 |
| 3 | MCP 调用统计闭环（MCPCallCount 与 session 统计打通） | P3 | — | Phase 2 |
| 4 | MCPBroker 默认启用（Agent 启用 `mcp_broker` 时自动挂载发现工具） | P3 | — | Phase 2 |
| 5 | MCP Server 认证配置（OAuth2/API Key 动态认证） | P3 | — | Phase 3 |
| 6 | MCP 会话重连验证（SSE/StreamableHTTP 断线自动恢复） | P3 | — | Phase 3 |

---

## 7. 验收标准

- [x] MCP Server 后台定时探活，`metadata_json` 中 `health_status` / `last_health_at` 自动更新
- [ ] MCP 工具调用超时后优雅降级，不阻塞 Agent
- [ ] MCP 调用次数与 session 统计一致
- [ ] MCPBroker 启用后 Agent 可运行时发现 MCP 工具
- [ ] MCP Server 可配置认证信息
- [ ] MCP 连接断开后可自动重连

---

## 8. 依赖与风险

- MCP 协议仍在演进，需关注兼容性
- 认证配置需与 Provider 凭据加密对齐
- 会话重连依赖 trpc-agent-go `mcpbroker` 的重连能力，需确认框架支持程度
