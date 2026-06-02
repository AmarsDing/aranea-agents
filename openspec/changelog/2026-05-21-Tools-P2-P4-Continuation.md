# Tools 模块 — P2–P4 续优化

**日期**：2026-05-21  
**模块**：Tools (23)

## P2

| 项 | 说明 |
|----|------|
| config vs config_schema | `validateToolConfigFields` + `gojsonschema`；MCP `transport`/`command`/`url` 校验 |
| MCP 生产安全 | `ProductionAllowAdHocHTTP`：需 MCP Server `allow_adhoc_http` + 系统设置 `mcp_allow_adhoc_http` |
| UpdateToolConfig | 更新前对照 catalog `config_schema_json` 校验 |

## P3

| 项 | 说明 |
|----|------|
| 审计 retention | `ToolAuditCleanup` cron（24h，默认保留 90 天）；`TOOL_AUDIT_CLEANUP_DISABLED` |
| 前端审计页 | `/tools/audits` — 筛选 tool/agent/user/status，表格展示 action + 摘要 |

## P4

| 项 | 说明 |
|----|------|
| Tool Cache | `internal/tools/cache`；catalog `metadata_json` / `config_json` 的 `cache_enabled` + `cache_ttl_sec` |
| streaming / chunk_count | `tool_invocations.streaming` + `chunk_count`；Proto `ToolInvocation` 字段；MCP `_meta` 推断 |

## 迁移

- 新库：`docs/sql/09_tool.sql` / `memory_chain.sql` 已含 streaming 列
- 存量：`go run ./cmd/sqlmigrate docs/sql/09_tool_invocations_streaming.sql`
