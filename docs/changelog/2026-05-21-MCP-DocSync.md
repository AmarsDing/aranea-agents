# MCP 文档同步 + 包结构 SRP

**日期**：2026-05-21  
**模块**：MCP (19)

## 摘要

- 对照代码更新 `19 mcp.md`、`19 mcp.design.md`、`19-mcp-development.md`（包路径、OAuth2、超时、重连、Broker 自动挂载）。
- 新增 `internal/mcp/config`（统一 `config_json` 解析）与 `internal/mcp/metadata`（健康/重连元数据合并）。
- 删除未引用的 `internal/tools/mcpmount`、`internal/mcp/mount`；运行时装配以 `agent/tool_assembly.go` + `tools/toolset.go` 为准。

## 代码

| 文件 | 变更 |
|------|------|
| `internal/mcp/config/config.go` |  canonical `ServerConfig` / `AuthConfig` / `ToTRPCConnectionConfig` |
| `internal/mcp/metadata/metadata.go` | `ApplyHealth` / `ApplyReconnect` |
| `internal/mcp/probe/eval.go` | 使用 `mcp/config` |
| `internal/biz/mcp_server.go` | 健康/重连持久化委托 `mcp/metadata` |
| `internal/agent/tool_assembly.go` | 使用 `mcp/config`，移除重复 JSON 结构体 |
| `internal/tools/mcpmount/*` | 删除（无引用） |
| `internal/mcp/mount/config.go` | 删除（无引用） |

## 文档

- `docs/需求/19 mcp.md`
- `docs/需求/19 mcp.design.md`
- `docs/需求/19-mcp-development.md`
- `docs/README.md` §5.2 MCP 索引

## 验证

```bash
go test ./internal/mcp/... ./internal/biz/... -count=1
```
