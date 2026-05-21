# Tools 工具 — 开发计划

> **版本**：5.0（修订）| **状态**：✅ 核心已实现；MCP 工程化（timeout/重连/认证/Broker 发现）待补
> **需求**：[23 tools.md](./23%20tools.md) · **设计**：[23 tools.design.md](./23%20tools.design.md) · **结构**：[23 tools struct design.md](./23%20tools%20struct%20design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：—

---

## 1. 模块定位

Tools 工具系统：管理 Agent 可调用的工具（内置工具 + 自定义工具 + MCP 工具），支持工具的注册、发现、调用和参数校验。

**代码锚点**：

| 层 | 文件 | 职责 |
|----|------|------|
| Proto | `api/kratos/tool/v1/` | Tool CRUD / Override / Invocation RPC 定义 |
| Service | `internal/service/tool.go` | ToolService：CRUD + Override + Invocation 查询 |
| Biz | `internal/biz/tool.go` | ToolUsecase：领域逻辑 + Override + Invocation 记录 |
| Data | `internal/data/tool.go` | ToolRepo：Ent ORM + 原生 SQL 混合 |
| Registry | `internal/tools/toolset.go` | Registry() + Assemble()：注册表与装配 |
| Adapter | `internal/tools/trpc/toolsets.go` | BuildToolsets()：ToolsetConfig → AssemblyConfig |
| Injection | `internal/agent/trpc_build.go` | BuildTRPCLLMAgent()：工具注入入口 |
| Assembly | `internal/agent/tool_assembly.go` | buildToolsetsForAgent + MCP + Override 配置合并 |
| Recorder | `internal/agent/tool_invocation_recorder.go` | AfterTool 调用记录 + 预览截断 |
| Runtime | `internal/agent/tool_runtime_options.go` | Filter / Retry 策略 |
| Ref | `internal/biz/tool_ref.go` | `ResolveToolKey`：Proto `tool_id` ↔ catalog `tool_key` |
| Policy | `internal/biz/agent_effective_tools.go` | Effective Tools 计算：profile + allow/deny |
| Seed | `internal/data/builtin_tools_seed.go` | 内置工具种子数据 |

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| Tool CRUD | ✅ 已实现 | Create/Update/Delete/Get/List + SearchTools |
| Tool 类型 | ✅ 已实现 | builtin / custom / mcp / system / external |
| Tool Override | ✅ 已实现 | `tool_agent_overrides` 表 + List/Upsert/Delete RPC |
| Effective Tools | ✅ 已实现 | `agent_effective_tools.go`：profile + allow/deny + catalog |
| Tool 注入 | ✅ 已实现 | `BuildTRPCLLMAgent` 中 WithTools + WithToolSets |
| Tool 参数 Schema | ✅ 已实现 | `parameters_json` / `result_schema_json` / `config_schema_json` |
| Tool Callbacks | ✅ 已实现 | AfterTool 记录 ToolInvocation |
| Tool Filter | ✅ 已实现 | ExcludeToolNamesFilter (deny 列表) |
| Tool Retry | ✅ 已实现 | RetryPolicy (可配置 maxAttempts/backoff/jitter) |
| Tool 并行 | ✅ 已实现 | WithEnableParallelTools |
| Memory Tools | ✅ 已实现 | memorytool.DefaultTools() (5 个标准工具) |
| Knowledge Search | ✅ 已实现 | knowledgepkg.NewSearchTool() + WithRetriever |
| MCP ToolSet | ✅ 已实现 | trpcmcp.NewMCPToolSet (stdio/sse/streamable_http) |
| MCP Broker | ✅ 已实现 | trpcmcpbroker.New (mcp_list_servers/tools/inspect/call) |
| Agent-as-Tool | ✅ 已实现 | trpcagenttool.NewTool (composition/delegation) |
| ServiceAwaitReply | ✅ 已实现 | serviceawaitreply.ServiceTool (阻塞式等待) |
| 调用记录 | ✅ 已实现 | ToolInvocationWrite + RecordToolInvocationAsync |
| 参数详情 | ✅ 已实现 | GetToolInvocationParams (脱敏参数查询) |
| 前端管理 | ✅ 已实现 | Tool 详情页 Override + Agent 设置页「工具覆盖」 |
| TRPC 需确认 | ✅ 已实现 | `ApplyConfirmationPolicy` + BeforeTool `blocked` |
| 调用统计闭环 | ✅ 已实现 | `duration_ms` + Prometheus + 列表 SQL 聚合 |

---

## 3. 差距与优化

| # | 优先级 | 差距 | 说明 |
|---|--------|------|------|
| 1 | **P2** | ToolOverride 运行时生效 | ✅ `ApplyRuntimeConfigMaps` + `ApplyConfirmationPolicy` + `ApplyAgentToolOverrides` |
| 2 | **P2** | ToolOverride 前端集成 | ✅ Agent 设置页「工具覆盖」面板 + `GET /v1/agents/{agent_id}/tool-overrides` |
| 3 | **P3** | 自定义工具在线测试 | ✅ `TestTool` RPC + 工具详情「在线测试」 |
| 4 | **P3** | 工具调用审计日志 | ✅ `tool_invocation_audit` + `ListToolInvocationAudits`；前端审计页待补 |
| 5 | **P3** | BeforeTool Callback | ✅ `tool_args_guard` 系统字段剥离；权限/动态注入可后续扩展 |
| 6 | **P4** | Tool Cache | ✅ `internal/tools/cache` + Before/AfterTool hooks；`metadata_json.cache_enabled` / `cache_ttl_sec` |
| 7 | **P2** | MCP 工程化 | ✅ 认证/重连/ Broker 自动发现；生产 `AllowAdHocHTTP` 需 `ARANEA_MCP_ALLOW_ADHOC_HTTP` |
| 8 | **P2** | Proto `tool_id` 语义统一 | ✅ `ResolveToolKey` + `ListRunsForTool`；Override Upsert 写入 `tool_id` |
| 9 | **P3** | `runtime_status` / `runtime_kind` 填充 | ✅ `EnrichToolCatalogRuntime`（Biz 层计算，List/Get 返回） |
| 10 | **P3** | CreateTool 业务校验 | ✅ `validateToolUpsert` + `validateToolConfigFields`（gojsonschema） |
| 11 | **P3** | TestTool 参数脱敏 | ✅ `RedactToolPreview` + `SanitizeToolInvocationWrite` |
| 12 | **P3** | 工具调用审计 | ✅ 表 + API + 前端 `/tools/audits` + 90 天 cron |
| 13 | **P3** | BeforeTool 系统字段剥离 | ✅ `tool_args_guard` BeforeTool hook |
| 14 | **P1** | effective key → ToolsetConfig 映射 | ✅ `ToolsetConfigFromEffectiveKeys` |
| 15 | **P2** | MCP 默认超时 | ✅ `normalizeMCPServerTimeout`（60s） |
| 16 | **P4** | `streaming` / `chunk_count` | ✅ `tool_invocations` 列 + 记录器 + Proto |

---

## 4. 开发阶段

### Phase 1：ToolOverride 运行时生效（P2）

**目标**：让 `tool_agent_overrides` 中存储的覆盖配置在 Agent 运行时实际生效。

**任务**：

| # | 任务 | 涉及文件 | 验收标准 |
|---|------|----------|----------|
| 1.1 | 读取 Agent 的 ToolOverride 列表 | `internal/agent/trpc_build.go` | `buildToolsetsForAgent` 能获取指定 Agent 的 Override 列表 |
| 1.2 | Override `enabled=false` 时跳过工具装配 | `internal/tools/toolset.go` 或 `trpc/toolsets.go` | Override 禁用的工具不出现在 AssembledToolsets 中 |
| 1.3 | Override `config_override_json` 注入工具配置 | `internal/tools/toolset.go` | Override 的配置覆盖默认配置（如 FilesystemDir、API Key 等） |
| 1.4 | Override `requires_confirmation` 注入工具声明 | 工具装配逻辑 | 高风险工具的确认标记可通过 Override 调整 |
| 1.5 | 前端 Agent 设置页展示 Override 管理 | 前端 | 可为单个 Agent 覆盖特定工具的启用/配置/确认 |

**验收**：
- [ ] Agent 可通过 Override 禁用全局启用的工具
- [ ] Agent 可通过 Override 启用全局禁用的工具
- [ ] Agent 可通过 Override 覆盖工具配置参数
- [ ] 前端可管理 Agent 级工具覆盖

### Phase 2：工具在线测试（P3）

**目标**：用户可在配置自定义工具时在线验证工具是否可用。

**任务**：

| # | 任务 | 涉及文件 | 验收标准 |
|---|------|----------|----------|
| 2.1 | 定义 `TestTool` Proto RPC | `api/kratos/tool/v1/tool.proto` | 接受 tool_id + 测试参数，返回执行结果或错误 |
| 2.2 | 实现 `TestTool` Service/Biz | `internal/service/tool.go` / `internal/biz/tool.go` | 构造临时 Agent 执行单次工具调用 |
| 2.3 | 前端工具配置页集成测试按钮 | 前端 | 配置自定义工具后可点击「测试」验证可用性 |

**验收**：
- [ ] 自定义工具可在配置时在线测试
- [ ] 测试结果展示成功/失败 + 输出预览
- [ ] 测试执行有超时保护（默认 30s）

### Phase 3：工具调用审计日志（P3）

**目标**：结构化审计工具调用，支持追溯谁在何时调用了什么工具。

**任务**：

| # | 任务 | 涉及文件 | 验收标准 |
|---|------|----------|----------|
| 3.1 | 定义 `tool_invocation_audit` 表 | Ent schema | 包含 invocation_id、agent_id、user_id、tool_key、action、result_summary、timestamp |
| 3.2 | 审计记录写入 | `internal/agent/trpc_build.go` AfterTool callback | 每次工具调用自动写入审计记录 |
| 3.3 | 审计查询 API | Proto + Service + Biz | 支持按 agent/user/tool/time 范围查询 |
| 3.4 | 前端审计日志页 | 前端 | 管理员可查看工具调用审计日志 |

**验收**：
- [ ] 工具调用可审计追溯
- [ ] 审计日志支持按 Agent / 工具 / 时间范围查询
- [ ] 审计日志有自动清理策略（默认保留 90 天）

---

## 5. 任务清单

| # | 任务 | 优先级 | Phase | 依赖 | 代码锚点 |
|---|------|--------|-------|------|----------|
| 1 | ToolOverride 运行时读取与生效 | P2 | 1 | — | `trpc_build.go` → `buildToolsetsForAgent` |
| 2 | ToolOverride 前端管理页 | P2 | 1 | #1 | 前端 Agent 设置页 |
| 3 | `TestTool` RPC + 在线测试 | P3 | 2 | — | `tool.proto` / `tool.go` |
| 4 | `tool_invocation_audit` 表 + 写入 | P3 | 3 | — | Ent schema / `trpc_build.go` |
| 5 | 审计查询 API + 前端页 | P3 | 3 | #4 | `tool.proto` / 前端 |

---

## 6. 验收标准

### 核心功能（已实现）

- [x] Tool 列表展示内置/MCP/系统工具的启用状态、分类、风险级别、schema
- [x] Tool 详情展示描述、参数 schema、返回结构、配置、Agent 覆盖
- [x] 全局启用/停用 + Agent 级 allow/deny 覆盖
- [x] Effective Tools 基于 profile + allow/deny + catalog 计算
- [x] 工具调用记录（参数摘要、结果摘要、耗时、状态、错误）
- [x] 工具参数脱敏查询
- [x] 工具使用统计（调用次数、成功率、平均耗时）
- [x] Agent 工具覆盖 CRUD（`tool_agent_overrides`）
- [x] Memory / Knowledge / MCP / Agent-as-Tool 运行时注入
- [x] Callbacks / Filter / Retry / Parallel 框架机制集成

### 待实现

- [x] Agent 可通过 Override 在运行时覆盖特定工具的参数/启用/确认
- [x] 自定义工具可在配置时在线测试（`POST /v1/tools/{id}/test`）
- [x] 工具调用可审计追溯（`GET /v1/tools/audits`；保留策略运维侧 90 天）

---

## 7. 依赖与风险

| 项 | 说明 |
|----|------|
| ToolOverride 运行时 | 需在 `buildToolsetsForAgent` 中读取 Override 列表并调整装配逻辑；需注意 Override 与 Effective Tools 策略的优先级 |
| 在线测试 | 需构造临时 Agent 执行单次工具调用，需考虑安全隔离与超时 |
| 审计日志 | 需注意存储膨胀；建议自动清理策略（默认 90 天） |
| BeforeTool Callback | 框架已支持但项目未使用；可用于动态参数注入、权限校验、审批流前置检查 |
| MCP 工具安全 | MCP Broker 的 `mcp_call` 可动态调用任意已注册 MCP 工具，需在生产环境限制 AdHoc HTTP |
