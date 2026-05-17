# MCP 协议 — 开发计划

> **版本**：2026-05-17 | **状态**：✅ 端到端可用
> **需求**：[19 mcp.md](./19%20mcp.md) · **设计**：[19 mcp.design.md](./19%20mcp.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：—

---

## 1. 模块定位

MCP（Model Context Protocol）集成：支持 Agent 通过 MCP 协议连接外部工具服务器，动态发现和调用工具。

**代码锚点**：
- `api/kratos/mcp_server/v1/` — MCPServer CRUD RPC
- `internal/service/mcp_server.go` — MCPServerService
- `internal/biz/mcp_server.go` — MCPServerUsecase
- `internal/data/mcp_server.go` — MCPServerRepo
- `internal/agent/trpc_build.go` — MCP 工具注入
- `internal/biz/agent_mcp_effective.go` — Effective MCP 计算

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| MCPServer CRUD | ✅ | Create/Update/Delete/Get/List |
| MCP 工具发现 | ✅ | 连接 MCP 服务器 → 列出工具 |
| MCP 工具注入 | ✅ | `BuildTRPCLLMAgent` 中 `WithMCPTools` |
| Effective MCP | ✅ | `agent_mcp_effective.go` |
| 前端管理 | ✅ | MCP Server 设置页 |

---

## 3. 差距与优化

1. **P2**：MCP Server 无健康检查，连接失败时无自动重连。
2. **P3**：MCP 工具调用无超时控制，长时间运行的 MCP 工具可能阻塞 Agent。
3. **P3**：MCP Server 无认证配置（如 OAuth2），仅支持无认证连接。

---

## 4. 开发阶段

- **Phase 1**：MCP Server 健康检查 + 自动重连
- **Phase 2**：MCP 工具调用超时控制
- **Phase 3**：MCP Server 认证配置

---

## 5. 任务清单

| # | 任务 | 优先级 | EP |
|---|------|--------|-----|
| 1 | MCP Server 健康检查定时任务 | P2 | — |
| 2 | MCP 工具调用超时（默认 60s） | P3 | — |
| 3 | MCP Server 认证配置（OAuth2/API Key） | P3 | — |

---

## 6. 验收标准

- [ ] MCP Server 连接断开后可自动重连
- [ ] MCP 工具调用超时后优雅降级
- [ ] MCP Server 可配置认证信息

---

## 7. 依赖与风险

- MCP 协议仍在演进，需关注兼容性
- 认证配置需与 Provider 凭据加密对齐
