# Tools 模块需求/设计 vs 实现验证报告

> 对应需求：`23 tools.md` / `23 tools.design.md` / `23 tools struct design.md`
> 生成日期：2026-05-15

---

## 一、总体评估

| 维度 | 完成度 | 说明 |
|------|--------|------|
| Proto/API 层 | **95%** | 8 个 RPC 全部实现，缺少 sort/confirm_key/has_error 等辅助字段 |
| Biz 层 | **90%** | 核心模型和 Usecase 完整，缺少参数详情和 Agent 覆盖 CRUD |
| Data 层 | **80%** | tools/tool_invocations 完整，缺少 3 张辅助表的 Ent Schema 和 Repo |
| Service 层 | **90%** | 全部 RPC 方法已实现 |
| 前端页面 | **85%** | 两个主页面和 11 个组件已实现，详情抽屉缺少 Agent/调用 Tab |
| 运行时工具注册 | **85%** | Registry + Assemble 完整，缺少审计记录和中间件链 |
| 结构设计文档的扩展架构 | **15%** | tooldef/toolctx/adkbridge/middleware/executor/backends 均未实现 |

---

## 二、已完全实现

| # | 需求/设计项 | 实现位置 |
|---|------------|----------|
| 1 | Proto 定义（Tool/ToolInvocation/ToolSummary 全部消息 + 8 个 RPC） | `api/kratos/tool/v1/tool.proto` |
| 2 | Biz 领域模型（Tool/ToolInvocation/ToolSummary/ToolUpsertInput/ToolListQuery/ToolRunQuery） | `internal/biz/tool.go` |
| 3 | ToolRepo 接口（7 个方法） | `internal/biz/tool.go` |
| 4 | ToolUsecase（7 个方法 + 分页校验） | `internal/biz/tool.go` |
| 5 | Data 层 Repo 实现（SearchTools 含 LEFT JOIN 聚合、Summary 计算、CRUD） | `internal/data/tool.go` |
| 6 | PlatformTool Ent Schema（18 个字段，映射 tools 表） | `internal/data/ent/schema/platform_tool.go` |
| 7 | ToolInvocation Ent Schema（23 个字段，映射 tool_invocations 表） | `internal/data/ent/schema/tool_invocation.go` |
| 8 | Service 层全部 RPC（ListTools/GetTool/CreateTool/UpdateTool/DeleteTool/ToggleToolEnabled/ListToolRuns/ListToolRunsForTool） | `internal/service/tool.go` |
| 9 | Wire 注入（data→NewToolRepo, biz→NewToolUsecase, service→NewToolService） | data.go/biz.go/service.go ProviderSet |
| 10 | Agent Effective Tools API（GetAgentEffectiveTools + UpdateAgentToolPolicy） | `internal/service/agent.go` |
| 11 | Agent Effective Tools Biz（GetEffectiveTools + UpdateAgentToolPolicy + 工具组 + 别名映射） | `internal/biz/agent_effective_tools.go` |
| 12 | 工具策略别名映射（shell→shell_exec, web_search→duckduckgo_search 等） | `internal/biz/tool_policy_keys.go` |
| 13 | 前端 types.ts（Tool/ToolInvocation/ToolSummary/AgentEffectiveTools 等全部类型） | `web/src/features/tools/types.ts` |
| 14 | 前端 api.ts（9 个 API 函数 + Kratos→Legacy 转换） | `web/src/features/tools/api.ts` |
| 15 | ToolsPage.vue（Hero+Metrics+Filters+Table+Detail+Editor） | `web/src/pages/ToolsPage.vue` |
| 16 | ToolRunsPage.vue（Hero+Filters+Table+Pagination） | `web/src/pages/ToolRunsPage.vue` |
| 17 | 11 个前端组件（ToolHeroSection/ToolsMetricStrip/ToolCatalogFilters/ToolsTable/ToolDetailContent/ToolEditorForm/ToolRunsTable/ToolRunsFilters/ToolJsonBlock/ToolGlassPanel/toolUi.ts） | `web/src/components/tools/` |
| 18 | 路由 `/tools` + `/tools/runs` | `web/src/router/routes.ts` |
| 19 | Agent 设置页工具配置（tools_enabled/profile/allow/deny/concurrent_allow/retry/streaming/parallel） | `web/src/pages/AgentSettingsPage.vue` |
| 20 | 运行时工具注册表（17 个注册项：file/hostexec/httpfetch/geminifetch/duckduckgo/google_search/arxiv_search/wikipedia/email/todo/await_user_reply/claudecode/workspace_exec/openapi/agent/mcp/mcpbroker） | `internal/tools/toolset.go` |
| 21 | ToolsetConfig + BuildToolsets（trpc 适配层） | `internal/tools/trpc/toolsets.go` |
| 22 | 工具回调（AfterTool callback） | `internal/agent/trpc_build.go` |
| 23 | 工具过滤（Allow/Deny Filter） | `internal/agent/trpc_build.go` |
| 24 | 工具重试策略（RetryPolicy） | `internal/agent/trpc_build.go` |
| 25 | SQL 建表（tools/tool_agent_overrides/tool_invocations/tool_invocation_params/tool_usage_daily） | `internal/data/sessionmemory/memory_chain.sql` |

---

## 三、未实现

| # | 需求/设计项 | 来源文档 | 说明 |
|---|------------|----------|------|
| 1 | **tool_agent_overrides Ent Schema + CRUD API** | tools.md §7.2 / tools.design.md §四 | SQL 表已建，但无 Ent Schema、无 Repo 方法、无 `GET/PUT /v1/tools/:id/agent-overrides` API |
| 2 | **tool_invocation_params Ent Schema + 查询 API** | tools.md §6.3 / §7.4 | SQL 表已建，但无 Ent Schema、无 Repo 方法、无参数详情查看 API |
| 3 | **tool_usage_daily Ent Schema + 聚合逻辑** | tools.md §7.5 | SQL 表已建，但无 Ent Schema、无日聚合写入/查询逻辑 |
| 4 | **p95_duration_ms 统计** | tools.md §4.2 表格列 | SQL 表有此字段，但 Biz 模型和 Proto 均未包含 |
| 5 | **SyncBuiltinTools / 目录种子机制** | tools.md §4.1 "同步内置工具"按钮 | 无启动时自动同步 Registry 到 tools 表的机制；agent_effective_tools.go 用硬编码 synthetic 函数替代 |
| 6 | **ToolResolver 独立组件** | tools.md §1.2 / struct design §四 | 无独立 ToolResolver；当前逻辑分散在 agent_effective_tools.go 和 trpc_build.go |
| 7 | **ToggleToolEnabled confirm_key** | tools.md §8.3 | Proto 只有 id+enabled，无 confirm_key 字段用于高风险工具二次确认 |
| 8 | **ListTools sort 参数** | tools.md §8.1 | Proto 无 sort 字段，当前固定按 category+display_name 排序 |
| 9 | **ListToolRuns has_error 过滤** | tools.md §8.8 | Proto 和 Repo 均无 has_error 过滤 |
| 10 | **PUT /v1/tools/:id/config 独立端点** | tools.md §8.4 | 无独立配置更新端点，只能通过 UpdateTool 全量更新 |
| 11 | **tooldef.Tool 接口** | struct design §四 | 未实现自定义 Tool 接口层，直接使用 trpc-agent-go 的 tool.Tool |
| 12 | **toolctx.ToolContext** | struct design §四 | 未实现加强版上下文，直接使用标准 context.Context |
| 13 | **middleware 洋葱模型链** | struct design §五 | 未实现 Validation/Auth/Cache/Retry/Tracing/Approval 中间件链 |
| 14 | **executor 执行器** | struct design §六 | 未实现独立执行器，工具直接由框架调用 |
| 15 | **adkbridge 适配层** | struct design §六 | 未实现 ToADKTool 适配器，使用 internal/tools/trpc/toolsets.go 替代 |
| 16 | **ApprovableTool + ApprovalMiddleware** | struct design §十一 | 未实现人机审批流（Human-in-the-Loop） |
| 17 | **Long-running Tool 支持** | struct design §十一 | 未实现 LongRunningFunctionTool 适配 |
| 18 | **流式工具 SSE 推流集成** | struct design §十二 | trpc-agent-go 有 StreamableTool，但无 tool_invocations streaming/chunk_count 列，无 SSE 推流文档 |
| 19 | **backends 运行时工具实现** | struct design §七 | 未按设计文档的 backends/ 目录结构实现，直接使用 trpc-agent-go 内置工具 |
| 20 | **OpenTelemetry 集成** | struct design §九 | 未实现工具调用级别的 OTel Span |
| 21 | **Tool Audit Recorder（运行时调用落库）** | tools.md §1.2 | AfterTool callback 存在但未写入 tool_invocations 表 |

---

## 四、部分实现

| # | 需求/设计项 | 现状 | 缺失部分 |
|---|------------|------|----------|
| 1 | **详情抽屉 5 Tab** | ToolDetailContent 使用 ExpansionItem 展示概览/参数/配置 | 缺少 **Agent Tab**（哪些 Agent 使用该工具、allow/deny 覆盖）和 **调用 Tab**（最近 20 条调用记录） |
| 2 | **高风险工具二次确认** | 前端有 risk_level 展示 | 缺少启用高风险工具时的确认弹窗（需输入 tool key 确认），后端无 confirm_key 校验 |
| 3 | **工具调用审计** | AfterTool callback 存在 | callback 仅做空操作，未将参数/结果/耗时写入 tool_invocations 表 |
| 4 | **Schema 渲染表单** | 配置 JSON 以 textarea 展示 | 未按 config_schema_json 动态渲染表单控件 |
| 5 | **MCP Tool 发现** | Registry 有 mcp/mcpbroker 注册项 | 需求文档说"本期先预留 source=mcp"，当前已基本实现 MCP 工具挂载 |

---

## 五、待办清单

### P0 — 核心缺失（影响需求闭环）

- [ ] **P0-1: 运行时调用落库** — AfterTool callback 需实际写入 tool_invocations 表（参数摘要/结果摘要/耗时/状态/错误），否则调用记录页永远为空
- [ ] **P0-2: SyncBuiltinTools 种子机制** — 启动时将 Registry 同步到 tools 表，消除 agent_effective_tools.go 中的 synthetic 硬编码函数
- [ ] **P0-3: tool_agent_overrides Ent Schema + Repo + API** — SQL 表已建但无 Ent Schema/Repo 方法/GET+PUT `/v1/tools/:id/agent-overrides` API

### P1 — 重要功能补全

- [ ] **P1-1: 详情抽屉 Agent Tab** — 展示使用该工具的 Agent、allow/deny 覆盖、profile 命中
- [ ] **P1-2: 详情抽屉调用 Tab** — 展示最近 20 条调用记录、错误摘要、跳转完整记录
- [ ] **P1-3: 高风险工具二次确认** — 前端启用时弹窗确认（runtime/messaging/external 需输入 tool key）；后端 ToggleToolEnabled 增加 confirm_key 校验
- [ ] **P1-4: tool_invocation_params Ent Schema + 查询 API** — SQL 表已建但无 Ent Schema/Repo/参数详情查看 API（管理员可查看脱敏后参数）
- [ ] **P1-5: p95_duration_ms 统计** — 需求文档表格列定义了 P95 耗时，Biz 模型和 Proto 均未包含
- [ ] **P1-6: ListTools sort 参数** — 需求文档要求支持 last_invoked_at/invoke_count/failure_rate 排序，当前固定 category+display_name
- [ ] **P1-7: ListToolRuns has_error 过滤** — 需求文档 §8.8 要求 has_error boolean 筛选，Proto 和 Repo 均未实现
- [ ] **P1-8: PUT /v1/tools/:id/config 独立端点** — 需求文档 §8.4 要求独立配置更新端点，后端需用 config_schema_json 校验
- [ ] **P1-9: Schema 渲染表单** — 配置 JSON 当前以 textarea 展示，需按 config_schema_json 动态渲染表单控件

### P2 — 架构增强（struct design 文档方向）

- [ ] **P2-1: ToolResolver 独立组件** — 将分散在 agent_effective_tools.go 和 trpc_build.go 的工具解析逻辑抽取为独立 ToolResolver
- [ ] **P2-2: tooldef.Tool + toolctx.ToolContext 接口层** — struct design §四 要求的自定义 Tool 接口和加强版上下文
- [ ] **P2-3: middleware 洋葱模型链** — struct design §五 Validation/Auth/Cache/Retry/Tracing/Approval 中间件链
- [ ] **P2-4: executor 执行器** — struct design §六 独立执行器串联 middleware 链并执行工具
- [ ] **P2-5: adkbridge 适配层** — struct design §六 ToADKTool 适配器，将内部 Tool 映射为框架 FunctionTool/StreamingTool
- [ ] **P2-6: ApprovableTool + ApprovalMiddleware + Long-running Tool** — struct design §十一 人机审批流（Human-in-the-Loop）
- [ ] **P2-7: 流式工具 SSE 推流集成** — struct design §十二 tool_invocations 增加 streaming/chunk_count 列，SSE 推送中间结果
- [ ] **P2-8: OpenTelemetry 工具调用追踪** — struct design §九 工具调用级别 OTel Span 创建与传播
- [ ] **P2-9: tool_usage_daily 日聚合** — SQL 表已建但无 Ent Schema/日聚合写入逻辑/趋势查询 API
- [ ] **P2-10: backends 运行时工具实现目录** — struct design §七 按设计文档 backends/ 目录结构实现业务工具

---

## 六、结论

**Tools 模块的 CRUD 管理链路（Proto → Biz → Data → Service → Frontend）已完整闭环**，前端两个页面和 11 个组件覆盖了工具目录管理和调用记录查看的核心场景。Agent 工具策略（Effective Tools + Policy Update）也已实现。

**主要差距集中在三个方面**：

1. **运行时审计未闭环**：AfterTool callback 未落库，调用记录页无数据来源
2. **3 张辅助表有 SQL 无 Ent Schema/Repo/API**：tool_agent_overrides、tool_invocation_params、tool_usage_daily
3. **struct design 文档的扩展架构（tooldef/middleware/executor/adkbridge/approval）基本未落地**，当前直接使用 trpc-agent-go 原生工具体系
