# Tools 模块 — 文档对齐与 P0–P3 优化

**日期**：2026-05-21  
**模块**：Tools (23)

## 摘要

对照 `23 tools.md` / `23 tools.design.md` / `23 tools struct design.md` / `23-tools-development.md` 审计实现差距；修复 Proto `tool_id` 与 catalog `tool_key` 混用；按 SRP 拆分 Agent 工具装配；完成 P1–P3 架构与业务优化项。

## P0 — 语义与查询修复

| 项 | 说明 |
|----|------|
| `biz.ResolveToolKey` | 统一 id/key 解析；Override / ListRunsForTool 经 Biz 层归一化 |
| `ListRunsForTool` | 修复按 catalog id 查询调用记录无结果 |
| `UpsertToolAgentOverride` | 写入 `tool_agent_overrides.tool_id` |
| 列表搜索 | `SearchTools` 增加 `category` 字段匹配（产品 §5.1） |

## P1 — 映射与复用

| 项 | 说明 |
|----|------|
| SRP 拆分 | `tool_assembly.go` / `tool_runtime_options.go` / `tool_invocation_recorder.go` 从 `trpc_build.go` 剥离 |
| `ToolsetConfigFromEffectiveKeys` | `internal/tools/trpc/effective_config.go` 单一映射源 |
| `JSONStringList` | `internal/biz/json_list.go` 复用 allow/deny 解析 |

## P2 — 运行时与校验

| 项 | 说明 |
|----|------|
| `runtime_status` / `runtime_kind` | `EnrichToolCatalogRuntime`（Biz 层 List/Get 计算） |
| `validateToolUpsert` | Create/Update/Delete 业务校验 + readonly 保护 |
| MCP 默认超时 | `normalizeMCPServerTimeout`（60s） |

## P3 — 安全与审计

| 项 | 说明 |
|----|------|
| 参数脱敏 | `RedactToolPreview` + `SanitizeToolInvocationWrite` |
| BeforeTool 系统字段剥离 | `tool_args_guard` hook（§0.3 禁止模型注入系统键） |
| 工具调用审计 | `tool_invocation_audit` 表 + `ListToolInvocationAudits` + AfterTool 写入 |

## 附带修复

- `internal/memory` ↔ `internal/memory/trpc` import cycle：`NewRuntimeSet` 迁至 trpc 子包；`auto_memory_queue` 去重
- `tool_args_guard_test` 断言 hook 修改后的 `BeforeToolArgs.Arguments`

## 文档变更

- `23 tools.md`：在线测试标记为已实现
- `23-tools-development.md`：差距表 §3 同步 P1–P3 完成项

## 仍待实现

- P2 MCP 认证 / 重连 / 生产级 Broker 发现
- P2 `config` vs `config_schema` 深度校验
- P3 前端审计页、90 天 retention cron
- P4 Tool Cache；`tool_invocations.streaming` / `chunk_count`
