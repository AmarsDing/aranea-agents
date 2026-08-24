# Tools 工具 — 开发计划

> **版本**：9.2（2026-07-20）| **状态**：✅ 核心已实现；**Phase 4 片段编辑 ✅**（catalog/策略层 + 运行时工具全部实现）；**Phase 5 工作区统一 ✅**；**Phase 6 架构优化 ✅**；**Phase 7 质量加固 ✅**；**Phase 8 ISP + 测试 + Knowledge ✅**；**Round 5 Wire 窄接口 + 错误规范 ✅**
> **需求**：[23-tools.md](./23-tools.md) · **设计**：[23-tools.design.md](./23-tools.design.md)

---

## 1. 模块定位

Tools 工具系统：管理 Agent 可调用的工具（内置工具 + 自定义工具 + MCP 工具），支持工具的注册、发现、调用和参数校验。

**代码锚点**：

| 层 | 文件 | 职责 |
|----|------|------|
| Proto | `api/kratos/tool/v1/tool.proto` | Tool CRUD / Override / Invocation / Audit RPC 定义 |
| Service | `internal/service/tool.go` | ToolService：CRUD + Override + Invocation + Audit 查询 |
| Biz | `internal/biz/tool/tool.go` | ToolUsecase：领域逻辑 + Override + Invocation 记录 |
| Biz（子模块） | `internal/biz/tool/tool_ref.go` | `ResolveToolKey`：Proto `tool_id` ↔ catalog `tool_key` |
| Biz（子模块） | `internal/biz/tool/tool_policy_keys.go` | Policy Alias 映射 + NormalizeToolPolicyKey + PropagateAllow/DenyAliases |
| Biz（子模块） | `internal/biz/tool/tool_catalog_runtime.go` | toolWebResChecker（线程安全）+ catalog 运行时计算 |
| Biz（子模块） | `internal/biz/tool/tool_test_invoke.go` | TestTool 在线测试（WithTimeout + 错误不吞） |
| Biz（子模块） | `internal/biz/tool/tool_validate.go` | validateToolUpsert + validateToolConfigFields（gojsonschema） |
| Biz（子模块） | `internal/biz/tool/tool_preview.go` | RedactToolPreview + SanitizeToolInvocationWrite |
| Biz（子模块） | `internal/biz/tool/requires_confirmation.go` | 高风险工具确认策略 |
| Biz（子模块） | `internal/biz/tool/circuit_breaker.go` | 工具调用熔断器 |
| Biz（Effective） | `internal/biz/agent_effective_tools.go` | Effective Tools 计算：profile + allow/deny + catalog |
| Data | `internal/data/tool.go` | ToolRepo：Ent ORM + 原生 SQL 混合 |
| Data | `internal/data/tool_audit.go` | ToolInvocationAuditRepo |
| Data | `internal/data/builtin_tools_seed.go` | 内置工具种子数据 |
| Ent Schema | `internal/data/ent/schema/platform_tool.go` | `tools` 表（fallback_config_json StorageKey=default_config_json） |
| Ent Schema | `internal/data/ent/schema/tool_invocation.go` | `tool_invocations` 表（含 streaming/chunk_count/deleted_at） |
| Ent Schema | `internal/data/ent/schema/tool_invocation_audit.go` | `tool_invocation_audit` 表 |
| Ent Schema | `internal/data/ent/schema/tool_agent_override.go` | `tool_agent_overrides` 表 |
| Registry | `internal/tools/toolset.go` | Registry() + Assemble() 编排调度 + 12 子装配器 |
| Tags | `internal/tools/tool.go` | ToolRegistration（含 Deferred/Examples/Group）+ RegistryByTag / RegistryByCategory |
| Adapter | `internal/tools/trpc/toolsets.go` | BuildToolsets()：ToolsetConfig → AssemblyConfig |
| Adapter | `internal/tools/trpc/effective_config.go` | ToolsetConfigFromEffectiveKeys + ToolsetConfigHasAny |
| Alias | `internal/tools/alias/alias.go` | RuntimeToolNameAliases（12 条映射，与 policy alias 双向一致 TPM-P1-01） |
| Injection | `internal/agent/trpc_build.go` | BuildTRPCLLMAgent()：工具注入入口 |
| Assembly | `internal/agent/tool_assembly.go` | buildToolsetsForAgent + MCP + Override 配置合并 + 工作区统一 + ApplyDecorators |
| Decorator | `internal/tools/decorator.go` | ToolDecorator（P0-G3 超时 + P0-D 结果预算 + P2-E 缓存）+ streamableToolDecorator |
| Decorator Apply | `internal/tools/decorator_apply.go` | ApplyDecorators：包装 standalone Tools + ToolSets |
| Safety | `internal/tools/safety.go` | ClassifyTool（P1-C 工具安全分类：ConcurrentSafe / Exclusive） |
| Recorder | `internal/agent/tool_invocation_recorder.go` | AfterTool 调用记录 + 预览截断 |
| Runtime | `internal/agent/tool_runtime_options.go` | Filter / Retry 策略 |
| Cache | `internal/tools/cache/result_cache.go` | ResultCache LRU + 锁保护 |
| SkillFilter | `internal/tools/skillruntime/filter.go` | filterCache LRU + RWMutex + Stats() |
| Kanban | `internal/tools/kanban/bridge.go` | BridgeReader/Writer/Lifecycle 子接口 |
| TestExec | `internal/tools/testexec/config.go` | 在线测试配置（含 diff_edit/patch_file case） |
| Prompt | `internal/agent/prompt.go` | RuntimeCapabilityCue + diff_edit 优先工作流提示 |

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
| Tool Retry | ✅ 已实现 | 默认开启；`SelectiveRetryOn`：ConcurrentSafe 瞬时网络/EOF + 结果级 HTTP 429/5xx + `%v` 包装超时；Exclusive/写文件不重试。Ent/前端新建默认 true；存量 false 由 DDL 20261228 翻 true |
| Tool 并行 | ✅ 已实现 | WithEnableParallelTools（默认开启）+ ToolDecorator（Exclusive 互斥 + 预算/缓存；超时由回调链） |
| **工具超时单一来源** | ✅ 已实现 | 装饰器 Timeout=0；`ToolsExecutionTimeoutSec`（默认 10min）经 callback 生效 |
| **Exclusive 进程内互斥** | ✅ 已实现 | hostexec 族串行；文件写按路径互斥；`read_file` 同路径共享；`list_file`/`search_*` 覆盖子树与写互斥（单表+条件变量，无父子锁序死锁）；`StreamableCall` 持锁至流结束 |
| **参数别名归一** | ✅ 已实现 | `NormalizeInvocation` 单一入口（装饰器 Call 前）；实现：`hostexecnorm` / `filenorm` / `argnorm`。别名重写 `AliasRewriteTotal` + Debug `tool.args.normalized`。catalog 在线测试与 graph 工具节点 `ApplyDefaultDecorators` |
| **分层文件锁 + worktree** | ✅ 已实现 | 默认路径锁；git 仓 LLM 写才 worktree。批执行用 `BatchExecuteAssembledTools` 走同一 Call，不套第二套 worktree |
| **结果大小一层主裁** | ✅ 已实现 | 装饰器预算/卸载为主；AfterTool limiter 仅兜底未装饰字符串，跳过信封与预算覆盖 |
| Memory Tools | ✅ 已实现 | memorytool.DefaultTools() (5 个标准工具) |
| Knowledge Search | ✅ 已实现 | knowledgepkg.NewSearchTool() + WithRetriever |
| MCP ToolSet | ✅ 已实现 | trpcmcp.NewMCPToolSet (stdio/sse/streamable_http) |
| MCP Broker | ✅ 已实现 | trpcmcpbroker.New (mcp_list_servers/tools/inspect/call) |
| Agent-as-Tool | ✅ 已实现 | trpcagenttool.NewTool (composition/delegation) |
| ServiceAwaitReply | ✅ 已实现 | serviceawaitreply.ServiceTool (阻塞式等待) |
| 调用记录 | ✅ 已实现 | ToolInvocationWrite + RecordToolInvocationAsync |
| 参数详情 | ✅ 已实现 | GetToolInvocationParams (脱敏参数查询) |
| 前端管理 | ✅ 已实现 | Tool 详情页 Override + Agent 设置页「工具覆盖」 |
| 管理界面增强 | ✅ 已实现（2026-08-11） | 列表新增覆盖/成功率/耗时/最近调用列 + 来源/仅看异常筛选（`abnormal` 查询参数 + 相关子查询）；调用记录页 Session/仅看错误筛选 + `ToolRunDetailDialog.vue` 详情弹窗（参数/输出/错误/上下文 4 Tab） |
| TRPC 需确认 | ✅ 已实现 | `ApplyConfirmationPolicy` + BeforeTool `blocked` |
| 调用统计闭环 | ✅ 已实现 | `duration_ms` + `p95_duration_ms` + Prometheus + 列表 SQL 聚合 |
| **工作区统一（file+shell）** | ✅ 已实现 | `applyToolWorkspaceDirs` + `ShellExecDir` + hostexec `WithBaseDir` |
| **shell 环境注入与脱敏** | ✅ 已实现 | hostexec `WithBaseEnv` + redacting ToolSet |
| **shell 参数 schema** | ✅ 已实现 | seed `workdir`；`hostexecnorm` 兼容 `working_dir` |
| **confirm 覆盖 exec_command** | ✅ 已实现 | `runtimeConfirmAliases`：`exec_command` ↔ `shell_exec` |
| **workspace_exec 装配** | ✅ 已修复 | registry 不独立挂载 nil executor；仅 CodeExecutor 路径 |
| **浏览器自动化** | ✅ 已实现 | `browser` Playwright MCP 桥接 + 种子表 |
| **统一消息发送** | ✅ 已实现 | `message` ToolSet + OutboundRouter |
| **子代理工具** | ✅ 已实现 | `subagents_spawn/list/get/cancel` + SubAgentService |
| **延迟工具机制** | ✅ 已实现 | `read_tool_result` Deferred 通道 + ToolSearchTool |
| **电子表格读取** | ✅ 已实现 | `read_spreadsheet` + 种子表 |
| **知识反思工具** | ✅ 已实现 | `knowledge_reflect` + 种子表 |
| **工具调用审计** | ✅ 已实现 | `tool_invocation_audit` 表 + ListToolInvocationAudits API + 前端 `/tools/audits` |
| **Profile 扩展** | ✅ 已实现 | 9 种 profile（新增 minimal/safe/system_admin/spirit） |
| **片段编辑 catalog/策略层** | ✅ 已实现 | seed 含 `diff_edit`/`patch_file`；testexec/prompt/alias/diffEditHelpers 已就绪 |
| **片段编辑运行时工具** | ✅ 已实现 | `diffedit.go`/`patchfile.go`/`editcontent.go`/`patch/`/`textfile/`/`internal/toolcache/file_views.go` 全部实现；种子启用 |

---

## 3. 差距与优化

| # | 优先级 | 差距 | 说明 |
|---|--------|------|------|
| 1 | **P2** | ToolOverride 运行时生效 | ✅ `ApplyRuntimeConfigMaps` + `ApplyConfirmationPolicy` + `ApplyAgentToolOverrides` |
| 2 | **P2** | ToolOverride 前端集成 | ✅ Agent 设置页「工具覆盖」面板 + `GET /v1/agents/{agent_id}/tool-overrides` |
| 3 | **P3** | 自定义工具在线测试 | ✅ `TestTool` RPC + 工具详情「在线测试」 |
| 4 | **P3** | 工具调用审计日志 | ✅ `tool_invocation_audit` + `ListToolInvocationAudits` + 前端 `/tools/audits` + 90 天 cron |
| 5 | **P3** | BeforeTool Callback | ✅ `tool_args_guard` 系统字段剥离；权限/动态注入可后续扩展 |
| 6 | **P4** | Tool Cache | ✅ 装饰器 `IsCacheable` 为网络 ConcurrentSafe 主缓存；BeforeTool `ResultCache` 经 `CatalogResultCacheAllowed` 跳过装饰器已缓存/写/file，避免双缓存 |
| 7 | **P2** | MCP 工程化 | ✅ 认证/重连/ Broker 自动发现；生产 `AllowAdHocHTTP` 需 `ARANEA_MCP_ALLOW_ADHOC_HTTP` |
| 8 | **P2** | Proto `tool_id` 语义统一 | ✅ `ResolveToolKey` + `ListRunsForTool`；Override Upsert 写入 `tool_id` |
| 9 | **P3** | `runtime_status` / `runtime_kind` 填充 | ✅ `EnrichToolRuntimeFields`（Biz 层计算，List/Get 返回） |
| 10 | **P3** | CreateTool 业务校验 | ✅ `validateToolUpsert` + `validateToolConfigFields`（gojsonschema） |
| 11 | **P3** | TestTool 参数脱敏 | ✅ `RedactToolPreview` + `SanitizeToolInvocationWrite` |
| 12 | **P3** | 工具调用审计 | ✅ 表 + API + 前端 `/tools/audits` + 90 天 cron |
| 13 | **P3** | BeforeTool 系统字段剥离 | ✅ `tool_args_guard` BeforeTool hook |
| 14 | **P1** | effective key → ToolsetConfig 映射 | ✅ `ToolsetConfigFromEffectiveKeys` |
| 15 | **P2** | MCP 默认超时 | ✅ `normalizeMCPServerTimeout`（60s） |
| 16 | **P4** | `streaming` / `chunk_count` | ✅ `tool_invocations` 列 + 记录器 + Proto |
| 17 | **P1** | 片段级文件编辑 `diff_edit` 运行时工具 | ✅ `diffedit.go` 已实现（多 edit 原子 apply + 结构化错误） |
| 18 | **P1** | unified / hunk 补丁 `patch_file` 运行时工具 | ✅ `patchfile.go` 已实现（patch/hunks 互斥 + hunk_mismatch 错误） |
| 19 | **P1** | SessionFileState 会话缓存 | ✅ `internal/toolcache/file_views.go` 已实现（per-invocation FileView） |
| 20 | **P2** | `edit_file` 别名迁移至 `diff_edit` | ✅ `internal/tools/alias/alias.go` + `internal/biz/tool/tool_policy_keys.go` 已同步 |
| 21 | **P2** | 大文件行区间 patch | 📋 >1MB 仅加载 hunk ±context |
| 22 | **P2** | Activity diff 预览 | 📋 消费 `structured_patch` 字段 |
| 23 | **P0** | 工作区统一：hostexec 绑 `workspace_root` | ✅ Phase 5 |
| 24 | **P0** | `workdir` schema + `working_dir` 兼容映射 | ✅ Phase 5 |
| 25 | **P0** | tool_confirm 覆盖 `exec_command` | ✅ Phase 5 |
| 26 | **P1** | `claude_code` 默认工作区 | ✅ Phase 5 |
| 27 | **P2** | `workspace_exec` 仅 CodeExecutor 就绪时装配 | ✅ Phase 5.2；见 [32-codeexecutor-development.md](./32-codeexecutor-development.md) |
| 28 | **P1** | TestTool / prompt 与工作区口径同步 | ✅ Phase 5 |
| 29 | **P1** | ToolRepo 接口拆分（红线 #15） | ✅ Phase 6：18 方法 → 8 子接口 + ToolRegistryReader |
| 30 | **P1** | Assemble 函数重构 | ✅ Phase 6：170 行 → 12 子装配器 |
| 31 | **P2** | ToolRegistration Tags 字段 | ✅ Phase 6：RegistryByTag / RegistryByCategory |
| 32 | **P2** | kanban Bridge 接口拆分（红线 #15） | ✅ Phase 6：9 方法 → 3 子接口 |
| 33 | **P3** | 补充单元测试 | ✅ Phase 6：kanban/knowledge/mcpobserve 40+ 用例 |
| 34 | **P3** | ResultCache LRU 驱逐 + 锁保护 | ✅ Phase 6 |
| 35 | **P1** | data 层 errors.New/sql.ErrNoRows → kerrors | ✅ Phase 7 |
| 36 | **P1** | Assemble 静默跳过添加 SysLogWarn | ✅ Phase 7 |
| 37 | **P1** | KnowledgeReflect 映射补全 | ✅ Phase 7 |
| 38 | **P1** | toolWebResChecker 线程安全 | ✅ Phase 7 |
| 39 | **P2** | TestTool 超时控制 + 错误不吞掉 | ✅ Phase 7 |
| 40 | **P2** | testexec knowledge_reflect 显式 case | ✅ Phase 7 |
| 41 | **P2** | filterCache LRU + RWMutex + 可观测性 | ✅ Phase 7 |
| 42 | **P2** | `replace_content` 走 textfile 编解码 | ✅ Round 3：`loadEditSnapshot`/`commitEditSnapshot` 替换 raw I/O |
| 43 | **P2** | seed 同步 description + schema | ✅ Round 3：`COALESCE(NULLIF)` 模式 |
| 44 | **P2** | `resolveWorkdir` 绝对路径校验 | ✅ Round 3：workspace 子路径包含检查 |
| 45 | **P3** | `editFileSnapshot.Raw` 死代码 | ✅ Round 3：已删除 |
| 46 | **P2** | `/tools/audits` 页面文档 | ✅ Round 3：`frontend-pages.md` 已补充 |
| 47 | **P2** | wire.go `*biz.SkillUsecase` 具体类型 → 窄接口 | ✅ Round 5：`watch.SkillReader`/`SkillWriter` + `biz.FilesystemHealthReader` |
| 48 | **P2** | CreateSkillDir 空 slug 校验 | ✅ Round 5：kerrors.BadRequest 拒绝空/不安全 slug |
| 49 | **P2** | testexec/trpc fmt.Errorf → kerrors | ✅ Round 5：3 处业务错误迁移 |
| 50 | **P1** | RunHealthChecks 闭包变量编译错误 | ✅ Round 5：循环变量捕获修复 |
| 51 | **P3** | 浏览器自动化工具 | ✅ `browser` Playwright MCP 桥接 |
| 52 | **P3** | 统一消息发送工具 | ✅ `message` ToolSet + OutboundRouter |
| 53 | **P2** | 子代理工具 | ✅ `subagents_*` + SubAgentService |
| 54 | **P2** | 延迟工具机制 | ✅ `read_tool_result` Deferred 通道 |
| 55 | **P3** | Profile 扩展 | ✅ 9 种 profile（minimal/safe/system_admin/spirit） |
| 56 | **P0** | 熔断器 HalfOpen 探针遗弃回收 | ✅ Phase 9：`probeClaimedAt` 追踪 + 超时回收（`circuit_breaker.go`） |
| 57 | **P1** | 工具行为版本化 | ✅ Phase 9：`ToolRegistration.BehaviorVersion` + Registry `(name, version)` 索引（`internal/tools/tool.go`） |
| 58 | **P1** | Reminder 机制（工具副作用反馈闭环） | ✅ Phase 9：`internal/agent/tool_reminder.go`（文件修改后提醒跑测试） |

> **说明**：曾起草「53 Desktop App」文档，**不实施**；Shell 工作区优化归属本模块 Phase 5，不涉及桌面 App 打包（Tauri）。

---

## 4. 开发阶段

### Phase 1：ToolOverride 运行时生效（P2）✅

**目标**：让 `tool_agent_overrides` 中存储的覆盖配置在 Agent 运行时实际生效。

**任务**：

| # | 任务 | 涉及文件 | 验收标准 | 状态 |
|---|------|----------|----------|------|
| 1.1 | 读取 Agent 的 ToolOverride 列表 | `internal/agent/trpc_build.go` | `buildToolsetsForAgent` 能获取指定 Agent 的 Override 列表 | ✅ |
| 1.2 | Override `enabled=false` 时跳过工具装配 | `internal/tools/toolset.go` / `trpc/toolsets.go` | Override 禁用的工具不出现在 AssembledToolsets 中 | ✅ |
| 1.3 | Override `config_override_json` 注入工具配置 | `internal/tools/toolset.go` | Override 的配置覆盖默认配置（如 FilesystemDir、API Key 等） | ✅ |
| 1.4 | Override `requires_confirmation` 注入工具声明 | 工具装配逻辑 | 高风险工具的确认标记可通过 Override 调整 | ✅ |
| 1.5 | 前端 Agent 设置页展示 Override 管理 | 前端 | 可为单个 Agent 覆盖特定工具的启用/配置/确认 | ✅ |

**验收**：
- [x] Agent 可通过 Override 禁用全局启用的工具
- [x] Agent 可通过 Override 启用全局禁用的工具
- [x] Agent 可通过 Override 覆盖工具配置参数
- [x] 前端可管理 Agent 级工具覆盖

### Phase 2：工具在线测试（P3）✅

**目标**：用户可在配置自定义工具时在线验证工具是否可用。

**任务**：

| # | 任务 | 涉及文件 | 验收标准 | 状态 |
|---|------|----------|----------|------|
| 2.1 | 定义 `TestTool` Proto RPC | `api/kratos/tool/v1/tool.proto` | 接受 tool_id + 测试参数，返回执行结果或错误 | ✅ |
| 2.2 | 实现 `TestTool` Service/Biz | `internal/service/tool.go` / `internal/biz/tool/tool_test_invoke.go` | 构造临时 Agent 执行单次工具调用（WithTimeout 5s） | ✅ |
| 2.3 | 前端工具配置页集成测试按钮 | 前端 | 配置自定义工具后可点击「测试」验证可用性 | ✅ |

**验收**：
- [x] 自定义工具可在配置时在线测试
- [x] 测试结果展示成功/失败 + 输出预览
- [x] 测试执行有超时保护（默认 5s）

### Phase 3：工具调用审计日志（P3）✅

**目标**：结构化审计工具调用，支持追溯谁在何时调用了什么工具。

**任务**：

| # | 任务 | 涉及文件 | 验收标准 | 状态 |
|---|------|----------|----------|------|
| 3.1 | 定义 `tool_invocation_audit` 表 | `internal/data/ent/schema/tool_invocation_audit.go` | 包含 invocation_id、agent_id、user_id、tool_key、action、result_summary、timestamp | ✅ |
| 3.2 | 审计记录写入 | `internal/agent/trpc_build.go` AfterTool callback | 每次工具调用自动写入审计记录 | ✅ |
| 3.3 | 审计查询 API | `api/kratos/tool/v1/tool.proto` + Service + Biz | 支持按 agent/user/tool/time 范围查询 | ✅ |
| 3.4 | 前端审计日志页 | 前端 `/tools/audits` | 管理员可查看工具调用审计日志 | ✅ |

**验收**：
- [x] 工具调用可审计追溯
- [x] 审计日志支持按 Agent / 工具 / 时间范围查询
- [x] 审计日志有自动清理策略（默认保留 90 天）

### Phase 4：片段级文件编辑（P1）✅

**目标**：在默认 `file` ToolSet 内提供 Cursor 式片段编辑（`diff_edit` / `patch_file`）与会话缓存，降低 token 与磁盘往返。需求见 [23-tools.md](./23-tools.md) §5，设计见 [23-tools.design.md](./23-tools.design.md) §13。

**当前状态**：✅ 全部实现（catalog/策略层 + 运行时工具）。

**任务**：

| # | 任务 | 涉及文件 | 验收标准 | 状态 |
|---|------|----------|----------|------|
| 4.1 | 抽取 `textfile` 共享包（encoding / line ending / quote fuzzy） | `pkg/trpc-agent-go/tool/internal/textfile/` · `tool/claudecode/` 改 import | claudecode 复用 textfile | ✅ |
| 4.2 | 实现 `patch` 包（hunk 类型、apply、unified 解析） | `pkg/trpc-agent-go/tool/file/patch/` | `patch_test.go` | ✅ |
| 4.3 | 实现 `patch_file` 工具 | `pkg/trpc-agent-go/tool/file/patchfile.go` · `file.go` | unified + hunk；原子写盘 | ✅ |
| 4.4 | 实现 `diff_edit` 工具 | `pkg/trpc-agent-go/tool/file/diffedit.go` | 多 edit 原子；结构化错误 | ✅ |
| 4.5 | 实现 SessionFileState | `pkg/trpc-agent-go/tool/file/editcontent.go` · `pkg/trpc-agent-go/internal/toolcache/file_views.go` | `TestFileViewCache_SkipsSecondRead` | ✅ |
| 4.6 | catalog 种子 + Effective Tools 组 | `internal/data/builtin_tools_seed.go` · `internal/biz/agent_effective_tools.go` | filesystem 组含 `diff_edit`/`patch_file` | ✅ |
| 4.7 | testexec + Activity 标签 | `internal/tools/testexec/config.go` · `activity_meta.go` | 在线测试 case + 活动流中文名 | ✅ |
| 4.8 | Agent Prompt 工作流 | `internal/agent/prompt.go` | diff_edit 优先工作流提示 | ✅ |
| 4.9 | 前端 catalog 同步 | `web/src/features/chat/diffEditHelpers.ts` · `web/src/features/agents/useAgentToolsCatalog.ts` | diff_edit/patch_file 事件处理 + defaultNativeToolKeys | ✅ |
| 4.10 | `edit_file` 别名迁移 | `internal/tools/alias/alias.go` · `internal/biz/tool/tool_policy_keys.go` | `edit_file` → `diff_edit` 双向一致（TPM-P1-01） | ✅ |

**验收**（与需求 §5 对齐）：

- [x] `diff_edit` 单调用多片段替换且原子提交
- [x] `patch_file` unified diff 应用；hunk mismatch 零副作用
- [x] SessionFileState 命中；外部 mtime 变化拒绝覆盖（`expected_mtime_ms` 乐观锁）
- [x] `replace_content` / `save_file` 无破坏性变更
- [x] catalog 种子含 `diff_edit`/`patch_file`；testexec/prompt/alias/前端已就绪
- [x] `go test ./tool/file/... ./tool/file/patch/...`（在 `pkg/trpc-agent-go` 模块内）— 新增 43 测试全过；`read_file` 响应含 `mtime_ms`；`save_file`/`replace_content` 写盘后刷新 FileView

**迭代顺序**（已执行）：4.1 → 4.2 → 4.5（editcontent + file_views）→ 4.4 → 4.3 → file.go 注册 + read/save/replace FileView 集成 → 4.6–4.10（已完成）

---

### Phase 5：工具工作区统一（P0）✅

**目标**：Cursor 式项目根——file 与 `shell_exec` 共用 `workspace_root`；审计其余工具是否需要目录。

**背景差距**（2026-05-22 排查）：

- `file` 已通过 `resolveAgentFilesystemDir` 绑定工作区。
- `hostexec` 装配未调用 `WithBaseDir`，命令落在 Server **进程当前目录**。
- Catalog `working_dir` 与 hostexec `workdir` 不一致，Agent 传参被忽略。
- `tool_confirm` 未覆盖运行时名 `exec_command`。

**任务**：

| ID | 任务 | 涉及文件 | 验收 | 状态 |
|----|------|----------|------|------|
| TW-5-01 | 抽取 / 复用 `resolveToolWorkspaceRoot` | `internal/agent/tool_assembly.go` | 单次解析，file+shell 同值 | ✅ |
| TW-5-02 | `AssemblyConfig` / `ToolsetConfig` 增加 `ShellExecDir` | `internal/tools/toolset.go`, `internal/tools/trpc/toolsets.go` | 桥接层传递 | ✅ |
| TW-5-03 | hostexec `WithBaseDir(ShellExecDir)` | `internal/tools/toolset.go` | `TestAssemble_hostexecUsesShellExecDir` | ✅ |
| TW-5-04 | `shell_exec` runtime_config `base_dir` | `internal/tools/trpc/runtime_config.go` | Override 可覆盖 | ✅ |
| TW-5-05 | seed 参数改为 `workdir` | `internal/data/builtin_tools_seed.go` | 与 hostexec schema 一致 | ✅ |
| TW-5-06 | `working_dir` → `workdir` 兼容 | `internal/tools/hostexecnorm` | 旧 JSON 仍可用 | ✅ |
| TW-5-07 | confirm 映射 `exec_command` | `internal/agent/tool_confirm_gate.go`, `confirmationMap` | 确认 UI 触发 | ✅ |
| TW-5-08 | 更新 `RuntimeCapabilityCue` | `internal/agent/prompt.go` | 口径与实现一致 | ✅ |
| TW-5-09 | TestTool shell 传 workspace | `internal/tools/testexec/config.go` | 在线测试可跑 | ✅ |
| TW-5-10 | `claude_code` 默认 `workspace_root` | `internal/agent/tool_assembly.go`, `internal/tools/trpc/runtime_config.go` | 未配 claude_code_dir 时回退 | ✅ |
| TW-5-13 | `ShellExecEnv` 注入 hostexec base env 并保持结果脱敏 | `internal/tools/hostexec/toolset.go` | `TestBuildHostexecToolSetInjectsAndRedactsEnvironment` | ✅ |

**Phase 5.2（P2，可选同迭代）**：

| ID | 任务 | 说明 | 状态 |
|----|------|------|------|
| TW-5-11 | `workspace_exec` 禁止 nil executor 独立挂载 | 仅 `WithCodeExecutor` 路径启用 | ✅ |
| TW-5-12 | 文档矩阵与 Skill CodeExecutor 根目录说明 | 设计 §7.8.2；execution-plan 已同步 | ✅ |

**Phase 5 验收**：

- [x] 设置 `ARANEA_WORKSPACE_ROOT` 为测试项目根（单元测试 + 装配路径）
- [x] 启用 `shell_exec` + Agent profile 允许 runtime
- [x] `exec_command` 在 workspace 内执行（`TestAssemble_hostexecUsesShellExecDir`）
- [x] `ShellExecEnv` 在命令中可读取，敏感值不出现在工具结果
- [x] file 与 shell 共用 `resolveToolWorkspaceRoot`
- [x] 拒绝 tool_confirm → `blocked`（`exec_command` 别名覆盖）
- [x] 无需 App 壳；Web 联调即可验证

**建议顺序**：TW-5-01 → 5-03 → 5-05/5-06 → 5-07 → 5-08/5-09 → 5-10 → 5-11

---

### Phase 6：架构优化（ISP 合规 + 代码质量）✅

**目标**：从业务、用户、架构三个角度系统性优化 tools 模块，消除红线违规、降低圈复杂度、补充测试覆盖。

**任务**：

| ID | 任务 | 涉及文件 | 验收 | 状态 |
|----|------|----------|------|------|
| TO-6-01 | ToolRepo 接口拆分 | `internal/biz/tool/tool.go` | 18 方法 → 8 子接口 + ToolRegistryReader 窄接口 | ✅ |
| TO-6-02 | 窄接口传播 | `internal/biz/agent_usecase.go` / `internal/agent/prompt.go` / `internal/team/runner.go` / `internal/runtime/deps.go` / `wire.go` | ToolRegistryReader 全链路一致 | ✅ |
| TO-6-03 | Assemble 子装配器 | `internal/tools/toolset.go` | 170 行 → 12 个独立函数 | ✅ |
| TO-6-04 | ToolRegistration Tags | `internal/tools/tool.go` / `toolset.go` | Tags 字段 + RegistryByTag/RegistryByCategory | ✅ |
| TO-6-05 | kanban Bridge 拆分 | `internal/tools/kanban/bridge.go` | 9 方法 → BridgeReader/Writer/Lifecycle | ✅ |
| TO-6-06 | 补充单元测试 | `internal/tools/kanban/` / `knowledge/` / `mcpobserve/` | 40+ 用例全部通过 | ✅ |
| TO-6-07 | ResultCache LRU + 锁保护 | `internal/tools/cache/result_cache.go` | accessedAt + evictLRULocked + 内部 RWMutex | ✅ |
| TO-6-08 | 修复预存编译错误 | `memory_l4_cascade.go` / `timeline_hydrate.go` / `knowledge/tool.go` | uc.store → 子字段；messageSearchReader；Search 签名 | ✅ |

**Phase 6 验收**：

- [x] `go build ./internal/biz/... ./internal/tools/...` 编译通过
- [x] `go test ./internal/tools/... ./internal/biz/tool/... -count=1` 全部通过（17 个包）
- [x] aranea-review 技能审查通过，无阻断项
- [x] ToolRepo / Bridge 子接口方法数均 ≤ 5（红线 #15 合规）
- [x] ToolRegistryReader 窄接口从 biz → agent → team → runtime → wire 全链路传播

### Phase 7：质量加固（错误处理 + 可观测性 + 并发安全）✅

**目标**：从业务、用户、架构三个角度继续深化 tools 模块质量，消除 data 层错误处理不规范、装配静默跳过、全局变量竞态等问题。

**任务**：

| ID | 任务 | 涉及文件 | 验收 | 状态 |
|----|------|----------|------|------|
| TO-7-01 | data 层 kerrors 迁移 | `internal/data/tool.go` / `tool_audit.go` | 19 处 errors.New/sql.ErrNoRows → kerrors | ✅ |
| TO-7-02 | Assemble 静默跳过添加日志 | `internal/tools/toolset.go` | geminifetch/google_search/Factory nil 添加 SysLogWarn | ✅ |
| TO-7-03 | KnowledgeReflect 映射补全 | `internal/tools/trpc/effective_config.go` | ToolsetConfigFromEffectiveKeys 添加映射 | ✅ |
| TO-7-04 | toolWebResChecker 线程安全 | `internal/biz/tool/tool_catalog_runtime.go` | sync.RWMutex 保护全局变量读写 | ✅ |
| TO-7-05 | TestTool 超时控制 + 错误不吞掉 | `internal/biz/tool/tool_test_invoke.go` | WithTimeout(5s) + SysLogWarn 记录失败 | ✅ |
| TO-7-06 | testexec knowledge_reflect case | `internal/tools/testexec/config.go` | 显式 case 返回 false | ✅ |
| TO-7-07 | filterCache LRU + RWMutex + 可观测性 | `internal/tools/skillruntime/filter.go` | LRU 驱逐 + atomic 计数器 + Stats() + RWMutex | ✅ |

**Phase 7 验收**：

- [x] `go test ./internal/tools/... ./internal/biz/tool/... -count=1` 全部通过（17 个包）
- [x] aranea-review 技能审查通过，无阻断项
- [x] data 层所有错误返回使用 kerrors（BE1 合规）
- [x] 全局变量均有锁保护（BC3 合规）
- [x] 无错误被静默吞掉（BE4 合规）

### Phase 8：ISP 合规 + 测试补全 + Knowledge 增强 ✅

**目标**：完成 AgentRepository/ChannelRepo ISP 拆分、ToolFilterForPrefix 测试覆盖、Knowledge AdaptiveRouter 全链路注入、BM25 双路径搜索优化。

**任务**：

| ID | 任务 | 涉及文件 | 验收 | 状态 |
|----|------|----------|------|------|
| TO-8-01 | ChannelRepo 14 方法 → 4 子接口 | `internal/biz/channel.go` | ChannelReader(3)/Writer(3)/CredentialRepo(3)/DeliveryRepo(4) | ✅ |
| TO-8-02 | AgentRepository 17 方法 → 4 子接口 + 2 独立 | `internal/biz/agent_usecase.go` | AgentReader(4)/Writer(3)/RuntimeSettingsRepo(2)/PromptFileRepo(5) + ListAgentCreators + ExecInTx | ✅ |
| TO-8-03 | ToolFilterForPrefix 7 用例测试 | `internal/tools/toolset_filter_test.go` | 空前缀/空白/匹配/不匹配/nil Tool/nil Declaration/TrimSpace | ✅ |
| TO-8-04 | effective_config 测试扩展 | `internal/tools/trpc/effective_config_test.go` | 15 映射路径 + 20 分支全覆盖 | ✅ |
| TO-8-05 | channel.go safego.Go 修复 | `internal/biz/channel.go` | 红线 #9 合规 | ✅ |
| TO-8-06 | AdaptiveRouter 全链路注入 | `chat_orchestrator.go` / `runner.go` / `wire.go` | Chat + Team 双路径共用 | ✅ |
| TO-8-07 | BM25 双路径搜索 | `internal/data/knowledge.go` | tsvector + pg_trgm + mergeBM25Results | ✅ |
| TO-8-08 | RetrievalEvaluator 逻辑修正 | `internal/knowledge/retrieval_evaluator.go` | 空 chunks 先于 nil LLM 检查 | ✅ |
| TO-8-09 | RetrievalEvaluator 测试 | `internal/knowledge/retrieval_evaluator_test.go` | nil LLM/空 chunks/parseAssessment/buildChunksSummary/truncateString/parseJSONLoose | ✅ |
| TO-8-10 | HybridRetriever 清理 | `internal/knowledge/hybrid_retriever.go` | 移除未用 cosineSimilarity + math import | ✅ |
| TO-8-11 | AdaptiveRouter 简化 | `internal/knowledge/adaptive_router.go` | Search 签名简化 + nil guard + SysLogWarn | ✅ |
| TO-8-12 | biz/skill 测试补全 | `internal/biz/skill/skill_test.go` | 15 个新测试函数 | ✅ |

**Phase 8 验收**：

- [x] `go vet ./internal/biz/...` 编译通过
- [x] `go build ./internal/biz/...` 编译通过
- [x] `go test ./internal/tools/ -run TestToolFilterForPrefix -count=1 -v` 7 用例全通过
- [x] aranea-review 审查通过，0 阻断项、4 建议、1 提示
- [x] AgentRepository / ChannelRepo 子接口方法数均 ≤ 5（红线 #15 合规）
- [x] AdaptiveRouter 通过 RuntimeTooling + context 注入，Chat/Team 共用逻辑（BR7 合规）

### Round 5：Wire 窄接口 + 错误规范 ✅

**目标**：将 wire.go 中具体 Usecase 类型替换为窄接口，统一错误处理规范。

**任务**：

| ID | 任务 | 涉及文件 | 验收 | 状态 |
|----|------|----------|------|------|
| R5-01 | wire.go `*biz.SkillUsecase` → 窄接口 | `wire.go` / `internal/biz/skill/skill.go` | `watch.SkillReader`/`SkillWriter` + `biz.FilesystemHealthReader` | ✅ |
| R5-02 | CreateSkillDir 空 slug 校验 | `internal/biz/skill/skill.go` | kerrors.BadRequest 拒绝空/不安全 slug + 路径安全检查 | ✅ |
| R5-03 | testexec/trpc fmt.Errorf → kerrors | `internal/tools/testexec/` / `internal/tools/trpc/` | 3 处业务错误迁移 | ✅ |
| R5-04 | RunHealthChecks 闭包变量修复 | `internal/biz/` | 循环变量捕获修复（编译错误） | ✅ |

**Round 5 验收**：

- [x] `go build ./...` 编译通过
- [x] wire.go 中 SkillUsecase 具体类型已替换为窄接口
- [x] 业务错误统一使用 kerrors（BE1 合规）

### Phase 9：Grok Build 借鉴（P0/P1 加固，2026-07-20）✅

**目标**：借鉴 Grok Build 对比分析（`docs/reports/2026-07-19-analysis-grok-build-function-by-function-comparison.md`），落地 tools 域 3 项改进。

**任务**：

| ID | 任务 | 涉及文件 | 验收 | 状态 |
|----|------|----------|------|------|
| TO-9-01 | 熔断器 HalfOpen 探针遗弃回收 | `internal/biz/tool/circuit_breaker.go` | `probeClaimedAt` 追踪探针认领时间；超过 recoveryTimeout 未上报结果的探针槽位自动回收，不再困死 HalfOpen | ✅ |
| TO-9-02 | 工具行为版本化 | `internal/tools/tool.go` | `ToolRegistration.BehaviorVersion` 字段；Registry 按 `(name, behavior_version)` 索引，`Resolve(name, version)` 锁定版本保证会话复现一致 | ✅ |
| TO-9-03 | Reminder 机制（工具副作用反馈） | `internal/agent/tool_reminder.go` | 跟踪 edit/write 类工具调用；turn 结束时若未观察到测试运行则生成 reminder 注入上下文 | ✅ |

**Phase 9 验收**：

- [x] `go test ./internal/biz/tool/ -run TestCircuitBreaker -count=1` 通过（含探针遗弃回归用例）
- [x] `go test ./internal/tools/ -run TestToolRegistry_BehaviorVersioning -count=1` 通过
- [x] `go test ./internal/agent/ -run TestToolReminder -count=1` 通过
- [x] TDD 流程：失败测试 → 最小实现 → 回归测试

### Phase 10：工具结果卸载（Result Offloading，2026-07-26）✅

**目标**：超预算工具结果从「不可逆截断」升级为「可逆卸载」——全量 JSON 存 artifact 服务，LLM 上下文只留 ref + 双端预览，read_file 分页读回（设计见 [23-tools.design.md §7.1.2](./23-tools.design.md#712-工具结果卸载result-offloading)）。

**任务**：

| ID | 任务 | 涉及文件 | 验收 | 状态 |
|----|------|----------|------|------|
| TO-10-01 | 卸载主路径：超限时 SaveArtifact + 卸载信封 | `internal/tools/decorator.go` | `truncateResult(ctx, jsonArgs, result)` 内分流；信封含 offloaded/ref/tool/original_size/preview_head/preview_tail/read_hint | ✅ |
| TO-10-02 | 确定性命名 + 排除清单 + 降级 | `internal/tools/decorator.go` | 文件名 `tool_results/<tool>/<sha256(args)[:16]>.json`；`read_file` 排除防递归；无 invocation/service/保存失败回退截断信封（行为与旧版一致） | ✅ |
| TO-10-03 | TDD 测试 | `internal/tools/decorator_offload_test.go` | 卸载信封+全量可回读、ref 确定性去重、read_file 排除、无 service 回退 4 用例 | ✅ |

**Phase 10 验收**：

- [x] `go test ./internal/tools/... -count=1` 全部通过（含 4 个新卸载用例 + 既有截断回归）
- [x] `go test ./internal/agent/... -count=1` 通过
- [x] `go build ./...` exit 0

**后续（未实施）**：P1 过期 tool response 清理（Anthropic `clear_tool_uses` 式，框架 messages 构建层），独立迭代。

---

## 5. 任务清单

| # | 任务 | 优先级 | Phase | 依赖 | 代码锚点 | 状态 |
|---|------|--------|-------|------|----------|------|
| 1 | ToolOverride 运行时读取与生效 | P2 | 1 | — | `internal/agent/trpc_build.go` → `buildToolsetsForAgent` | ✅ |
| 2 | ToolOverride 前端管理页 | P2 | 1 | #1 | 前端 Agent 设置页 | ✅ |
| 3 | `TestTool` RPC + 在线测试 | P3 | 2 | — | `api/kratos/tool/v1/tool.proto` / `internal/biz/tool/tool_test_invoke.go` | ✅ |
| 4 | `tool_invocation_audit` 表 + 写入 | P3 | 3 | — | `internal/data/ent/schema/tool_invocation_audit.go` / `internal/agent/trpc_build.go` | ✅ |
| 5 | 审计查询 API + 前端页 | P3 | 3 | #4 | `api/kratos/tool/v1/tool.proto` / 前端 `/tools/audits` | ✅ |
| 6 | 片段编辑 `diff_edit` + `patch_file` 运行时工具 | P1 | 4 | — | 📋 `pkg/trpc-agent-go/tool/file/diffedit.go` / `patchfile.go`（待创建） | ❌ |
| 7 | SessionFileState 会话缓存 | P1 | 4 | #6 | 📋 `pkg/trpc-agent-go/internal/toolcache/file_views.go`（待创建） | ❌ |
| 8 | catalog / Prompt / Activity 集成 | P1 | 4 | #6 | ✅ `internal/data/builtin_tools_seed.go` / `internal/agent/prompt.go` / `internal/tools/testexec/config.go` | ✅ |
| 9 | `edit_file` 别名迁移 | P2 | 4 | #6 | ✅ `internal/tools/alias/alias.go` / `internal/biz/tool/tool_policy_keys.go` | ✅ |
| 10 | 工作区统一（hostexec + schema + confirm） | **P0** | **5** | — | ✅ `internal/agent/tool_assembly.go` TW-5-01–5-10 | ✅ |
| 11 | `workspace_exec` / CodeExecutor 装配 | P2 | 5.2 | 10 | ✅ `internal/tools/toolset.go` registry nil | ✅ |
| 12 | ToolRepo 接口拆分 + 窄接口传播 | **P1** | **6** | — | ✅ `internal/biz/tool/tool.go` 8 子接口 + ToolRegistryReader | ✅ |
| 13 | Assemble 子装配器重构 | P1 | 6 | — | ✅ `internal/tools/toolset.go` 12 个 assembleXxx 函数 | ✅ |
| 14 | ToolRegistration Tags + 查询 | P2 | 6 | — | ✅ `internal/tools/tool.go` RegistryByTag/RegistryByCategory | ✅ |
| 15 | kanban Bridge 接口拆分 | P2 | 6 | — | ✅ `internal/tools/kanban/bridge.go` BridgeReader/Writer/Lifecycle | ✅ |
| 16 | 补充单元测试 | P3 | 6 | — | ✅ `internal/tools/kanban/` / `knowledge/` / `mcpobserve/` 40+ 用例 | ✅ |
| 17 | ResultCache LRU + 锁保护 | P3 | 6 | — | ✅ `internal/tools/cache/result_cache.go` evictLRULocked + 内部 RWMutex | ✅ |
| 18 | data 层 kerrors 迁移 | **P1** | **7** | — | ✅ `internal/data/tool.go` / `tool_audit.go` 19 处 | ✅ |
| 19 | Assemble 静默跳过添加日志 | P1 | 7 | — | ✅ `internal/tools/toolset.go` SysLogWarn | ✅ |
| 20 | KnowledgeReflect 映射补全 | P1 | 7 | — | ✅ `internal/tools/trpc/effective_config.go` | ✅ |
| 21 | toolWebResChecker 线程安全 | P1 | 7 | — | ✅ `internal/biz/tool/tool_catalog_runtime.go` sync.RWMutex | ✅ |
| 22 | TestTool 超时 + 错误不吞 | P2 | 7 | — | ✅ `internal/biz/tool/tool_test_invoke.go` WithTimeout + SysLogWarn | ✅ |
| 23 | testexec knowledge_reflect case | P2 | 7 | — | ✅ `internal/tools/testexec/config.go` 显式 case | ✅ |
| 24 | filterCache LRU + RWMutex + Stats | P2 | 7 | — | ✅ `internal/tools/skillruntime/filter.go` atomic 计数器 | ✅ |
| 25 | wire.go SkillUsecase → 窄接口 | **P2** | **R5** | — | ✅ `wire.go` watch.SkillReader/Writer + FilesystemHealthReader | ✅ |
| 26 | CreateSkillDir 空 slug 校验 | P2 | R5 | — | ✅ `internal/biz/skill/skill.go` kerrors.BadRequest | ✅ |
| 27 | testexec/trpc fmt.Errorf → kerrors | P2 | R5 | — | ✅ 3 处业务错误迁移 | ✅ |
| 28 | RunHealthChecks 闭包变量修复 | **P1** | **R5** | — | ✅ 循环变量捕获 | ✅ |
| 29 | ChannelRepo ISP 拆分 | P2 | 8 | — | ✅ `internal/biz/channel.go` 4 子接口 | ✅ |
| 30 | AgentRepository ISP 拆分 | P2 | 8 | — | ✅ `internal/biz/agent_usecase.go` 4 子接口 + 2 独立 | ✅ |
| 31 | ToolFilterForPrefix 测试 | P3 | 8 | — | ✅ `internal/tools/toolset_filter_test.go` 7 用例 | ✅ |
| 32 | effective_config 测试扩展 | P3 | 8 | — | ✅ `internal/tools/trpc/effective_config_test.go` 15 映射 + 20 分支 | ✅ |
| 33 | 熔断器探针遗弃回收 | **P0** | 9 | — | ✅ `internal/biz/tool/circuit_breaker.go` probeClaimedAt | ✅ |
| 34 | 工具行为版本化 | **P1** | 9 | — | ✅ `internal/tools/tool.go` BehaviorVersion + Resolve(name, version) | ✅ |
| 35 | Reminder 机制 | **P1** | 9 | — | ✅ `internal/agent/tool_reminder.go` | ✅ |

---

## 6. 验收标准

### 核心功能（已实现）

- [x] Tool 列表展示内置/MCP/系统工具的启用状态、分类、风险级别、schema
- [x] Tool 详情展示描述、参数 schema、返回结构、配置、Agent 覆盖
- [x] 全局启用/停用 + Agent 级 allow/deny 覆盖
- [x] Effective Tools 基于 profile + allow/deny + catalog 计算
- [x] 工具调用记录（参数摘要、结果摘要、耗时、P95、状态、错误、streaming/chunk_count）
- [x] 工具参数脱敏查询
- [x] 工具使用统计（调用次数、成功率、平均耗时、P95）
- [x] Agent 工具覆盖 CRUD（`tool_agent_overrides`）
- [x] Memory / Knowledge / MCP / Agent-as-Tool 运行时注入
- [x] Callbacks / Filter / Retry / Parallel 框架机制集成
- [x] Agent 可通过 Override 在运行时覆盖特定工具的参数/启用/确认
- [x] 自定义工具可在配置时在线测试（`POST /v1/tools/{id}/test`）
- [x] 工具调用可审计追溯（`GET /v1/tools/audits`；保留策略运维侧 90 天）

### Phase 4（片段级文件编辑）✅

- [x] `diff_edit` / `patch_file` 运行时可用
- [x] SessionFileState 同 invocation 缓存生效（`internal/toolcache/file_views.go`）
- [x] catalog / Effective Tools / Prompt / Activity / 别名 / 前端集成完成

### Phase 5（工具工作区统一）✅

- [x] file 与 shell 共用 `workspace_root`
- [x] `workdir` schema 与 `working_dir` 兼容
- [x] `exec_command` 纳入 tool_confirm
- [x] Web 联调验收通过（不依赖 App 打包）

### Phase 6（架构优化）✅

- [x] ToolRepo 18 方法拆分为 8 子接口（红线 #15 合规）
- [x] ToolRegistryReader 窄接口全链路传播
- [x] Assemble 170 行拆分为 12 子装配器
- [x] ToolRegistration Tags 字段 + RegistryByTag/RegistryByCategory
- [x] kanban Bridge 9 方法拆分为 3 子接口
- [x] kanban/knowledge/mcpobserve 单元测试 40+ 用例
- [x] ResultCache LRU 驱逐 + 锁保护（2026-08-14：`cache.Global()`/`SetGlobal()` 已删，agent 层包级私有 `defaultToolResultCache` 单例 + `TRPCBuilderDeps.ResultCache` 可注入）
- [x] aranea-review 审查通过，无阻断项

### Phase 7（质量加固）✅

- [x] data 层 19 处 errors.New/sql.ErrNoRows → kerrors
- [x] Assemble 静默跳过添加 SysLogWarn 日志
- [x] KnowledgeReflect 映射补全
- [x] toolWebResChecker 全局变量 sync.RWMutex 保护
- [x] TestTool WithTimeout(5s) + 错误不吞掉
- [x] testexec knowledge_reflect 显式 case
- [x] filterCache LRU + RWMutex + atomic 计数器 + Stats()
- [x] aranea-review 审查通过，无阻断项

### Phase 7a（测试补全 + 红线修复）✅

- [x] `channel.go:315` 裸 `go func()` 修复为 `safego.Go`（红线 #13）
- [x] `ToolsetConfigHasAny` 全 20 分支测试覆盖
- [x] `ToolsetConfigFromEffectiveKeys` 全 15 映射路径 + 9 filesystem 子键测试覆盖
- [x] aranea-review 审查通过，0 阻断项

### Phase 8（ISP 合规 + 测试补全 + Knowledge 增强）✅

- [x] ChannelRepo 14 方法 → 4 子接口（红线 #15 合规）
- [x] AgentRepository 17 方法 → 4 子接口 + 2 独立（红线 #15 合规）
- [x] ToolFilterForPrefix 7 用例测试全通过
- [x] effective_config 15 映射路径 + 20 分支测试全覆盖
- [x] AdaptiveRouter 通过 RuntimeTooling + context 注入，Chat/Team 共用逻辑（BR7 合规）
- [x] aranea-review 审查通过，0 阻断项、4 建议、1 提示

### Round 5（Wire 窄接口 + 错误规范）✅

- [x] wire.go 中 `*biz.SkillUsecase` 替换为 `watch.SkillReader`/`SkillWriter` + `biz.FilesystemHealthReader`
- [x] CreateSkillDir 空/不安全 slug 校验（kerrors.BadRequest）
- [x] testexec/trpc 3 处 fmt.Errorf → kerrors
- [x] RunHealthChecks 闭包变量捕获修复

### Round 6（2026-08-14 深度评审修复）✅

- [x] BUG-1：`PropagateAllowAliases` 改双向传播（alias↔canon 等价类，循环至稳定覆盖链式别名 shell→shell_exec→exec_command）
- [x] BUG-2：`MergeToolConfigMaps` 参数顺序修正，`config_json`（用户配置）覆盖 `default_config_json`（默认值）
- [x] PERF-1：ResultCache 策略解析从每次调用 `GetTool` 聚合查询改为装配期 catalog 快照（`cachePolicyFromSnapshot`）；`cache.Global()`/`SetGlobal()` 删除，`TRPCBuilderDeps.ResultCache` 可注入
- [x] PERF-2：P95 耗时改 `percentile_cont(0.95) WITHIN GROUP`（Postgres 精确插值分位数，替代 top-5% 均值）
- [x] PERF-3：`toolSelectSQL` stats/p95/last 子查询加 90 天时间窗（`toolStatsWindowDays`），限制扫描范围
- [x] CONSISTENCY-1：preview/summary 截断改 rune 安全 `truncateUTF8`（防 PG 22021 invalid byte sequence）
- [x] CONSISTENCY-2：`tool_invocations` 级联清理补充 `tool_invocation_params`（引用完整性）
- [x] DEAD-1：`check_progress` 死代码删除（种子表 + `spirit_tools.go` + `plan_executor.go` + `reply_reminder_inject.go`；存量库 `syncRemovedBuiltinToolPatches` 幂等软删；DECISION.md 提示词同步清理）
- [x] DEAD-2：`FilesystemDirWithDir`/`FilesystemDirFromContext` 死代码删除
- [x] 门禁：`go build ./internal/... ./api/... ./pkg/...` ✅ / `go vet`（tools 相关包）✅ / `go test`（biz/tool、tools/*、data、agent、service、scenario/system）✅

### Round 7（2026-08-24 精灵系统配置治理）✅

背景：精灵系统配置审计发现 prompt 契约与工具策略互相打架、目录死配置、prompt 排序副作用。逐项修复：

- [x] P1-1 `spirit` profile 移除 `shell_exec` + `group:computeruse`（`agent_tool_policy.go`）——CAPABILITIES.md 契约是"编排者不直接执行 shell/桌面自动化"，策略层此前却放行；测试 `TestProfileAllowSet_spiritExcludesShellAndComputerUse` / `TestBuildAgentEffectiveTools_spiritComputerUseDenied` 锁定新契约；`test/agent-audit/audit.py` PROFILES 同步
- [x] P1-2 DECISION.md 删除不存在的 `subagents_wait` 引用（`subagents_get` 的 `block_until_ms` 覆盖等待语义）
- [x] P1-3 `get_team_deliverable` 补入 `builtinPlatformToolSeeds`（此前仅有实现 + profile 命名，catalog 缺行导致 effective-tools 展示/确认门禁覆盖不到）
- [x] P1-4 DEAD-3：软删 6 个 legacy spirit 工具（`assemble_team`/`assess_complexity`/`cancel_team`/`check_team_progress`/`list_butlers`/`query_butler_status`，实现已并入 `plan_and_execute`/系统通知且无任何 profile 命名），照 DEAD-1 先例走 `syncRemovedBuiltinToolPatches`
- [x] P1-5 迁移 `20261243 builtin_platform_tools_spirit_cleanup_reseed`（`ddlBuiltinPlatformTools`，存量库补目录行 + 软删；`TestMigrationVersionsGloballyUnique` 守卫通过）
- [x] P2-1 精灵 prompt 装配顺序显式化：IDENTITY(0)→CAPABILITIES(1)→DECISION(2)（`seed_system_admin.go` 按白名单顺序排序，不再依赖 `os.ReadDir` 字母序把身份卡垫到底）
- [x] P2-2 前端 `__spirit__` 字面量统一为 `SPIRIT_AGENT_KEY`（唯一出处 `web/src/features/spirit/types.ts`，10 处生产代码替换，测试保留字面量）
- [x] P3-1（D8，用户裁定）`SeedSpiritAgent` seed 模型字面量 openrouter/gpt-4.1-mini → deepseek/deepseek-v4-flash，对齐 8-23 治理基线（ON CONFLICT 不回写 provider/model，仅约束全新安装）
- [x] P3-2（D9，用户裁定）语音管家快路径收敛：qVoice seed 扩展 `subagents_enabled=false` + `clarification_enabled=false` + `skill_load_mode='progressive'`（ADR 记入 1-chat.design.md B.10.22.5）
- [x] P3-3（D10，用户裁定）`__spirit__` 运维护栏保持零值（max_llm_calls/max_tool_iterations/context_window=0、heartbeat 关），0 语义=不限/框架默认，决策理由入 ADR
- [x] 门禁：`go test ./internal/biz/ ./internal/agent/ ./internal/tools/...` ✅ / vitest（spirit/voice 相关 20 例）✅ / eslint 0 error ✅ / Docker dev-up + DB 核验（deliverable 入目录、6 工具软删、DECISION 重 seed、语音管家三项收敛）✅

---

## 7. 依赖与风险

| 项 | 说明 |
|----|------|
| ToolOverride 运行时 | ✅ 已在 `buildToolsetsForAgent` 中读取 Override 列表并调整装配逻辑；Override 与 Effective Tools 策略的优先级已处理 |
| 在线测试 | ✅ 已构造临时 Agent 执行单次工具调用，含安全隔离与 5s 超时 |
| 审计日志 | ✅ 已实现自动清理策略（默认保留 90 天） |
| BeforeTool Callback | ✅ `tool_args_guard` 系统字段剥离；权限/动态注入可后续扩展 |
| MCP 工具安全 | ✅ MCP Broker 的 `mcp_call` 在生产环境限制 AdHoc HTTP（`ARANEA_MCP_ALLOW_ADHOC_HTTP`） |
| 片段编辑与 claudecode 重复 | 📋 待实现：抽取 `tool/internal/textfile` 共享逻辑，避免双份 patch 实现；claudecode 仍负责 Bash / Notebook |
| SessionFileState 删盘边界 | 📋 待实现：同 invocation 内删盘后仍可用 cache 编辑；外部变更靠 `mtime_ms` |
| 别名迁移 | ✅ `edit_file` → `diff_edit`（2026-05-22）；`internal/tools/alias/alias.go` 与 `internal/biz/tool/tool_policy_keys.go` 已同步（TPM-P1-01） |
| **工作区统一** | ✅ Phase 5：`ShellExecDir` + `hostexecnorm` + confirm 别名 |
| **53 Desktop App 文档** | 已删除、不实施；Shell 优化归属 Phase 5（已完成） |

---

## 8. 剩余工作

### P1（高优先级）

| # | 任务 | 说明 |
|---|------|------|
| 1 | 片段编辑运行时工具实现 | `diffedit.go`/`patchfile.go`/`editcontent.go`/`patch/`/`textfile/`/`internal/toolcache/file_views.go` 均待创建；catalog/策略层已就绪 |
| 2 | `buildMCPToolSet` / `buildMCPBrokerTools` 测试 | Assemble 子装配器中 MCP 路径无测试覆盖 |

### P2（中优先级）

| # | 任务 | 说明 |
|---|------|------|
| 3 | `custom/` 包冒烟测试 | 用户模板工具无测试 |
| 4 | `memory/` 包测试 | 整个包无测试文件 |
| 5 | tools 层 `fmt.Errorf` 评估 | 工具执行层（kanban/webresearch/cli_admin/alias）70 处 `fmt.Errorf` 属于框架工具 Call 返回值，不经过 kerrors 链，保持不变 |
| 6 | BM25 分数归一化 | `mergeBM25Results` 直接拼接 tsvector 和 trigram 结果，分数尺度不同可能导致排序偏差 |
| 7 | `AdaptiveRouter.Search` 签名简化 | 调用方常传 `nil` + `""`，考虑提供简化签名 |
| 8 | `slugify("")` 全局唯一 slug 生成 | 空 slug 已在 `CreateSkillDir` 校验拦截，但 `slugify` 本身仍生成 "skill-0" 非唯一值 |
| 9 | `kanban` / `spirit` 纳入 toolGroup | `toolGroupsBrowser` / `group:subagent` 已建且 `full` profile 已包含；剩余：kanban 无独立组（现经 KanbanBridge 特殊装配）、无 `toolGroupsSpirit`（spirit profile 直接枚举 key，无法 `group:spirit` 引用） |
| 10 | 大文件行区间 patch | >1MB 仅加载 hunk ±context |
| 11 | Activity diff 预览 | 消费 `structured_patch` 字段 |

### P3（低优先级）

| # | 任务 | 说明 |
|---|------|------|
| 12 | `BuildToolsets` 集成测试 | 核心桥接函数需 mock-heavy 测试 |
| 13 | E2E 全链路测试 | `read_file` → `diff_edit` → shell 读同路径（依赖片段编辑运行时工具实现） |
| 14 | `AgentPromptFileRepo` 监控 | 恰好 5 方法处于红线边界，新增方法需立即拆分 |
| 15 | `biz/` 预存测试失败修复 | `TestRecordReconnectMetadata`/`TestAgentRuntimeSettings_DomainAccessors`/`TestValidateRalphLoopSettings` 3 个测试失败 |

---

## 9. 改动文件清单（按 Phase 汇总）

### Phase 1-3（已实现）

- `api/kratos/tool/v1/tool.proto` — TestTool / Audit / Override RPC 定义
- `internal/service/tool.go` — ToolService 扩展
- `internal/biz/tool/tool.go` — ToolUsecase 扩展
- `internal/biz/tool/tool_test_invoke.go` — 在线测试
- `internal/biz/tool/tool_preview.go` — 参数脱敏
- `internal/biz/tool/tool_validate.go` — 业务校验
- `internal/data/tool.go` — ToolRepo 扩展
- `internal/data/tool_audit.go` — 审计 Repo
- `internal/data/ent/schema/tool_invocation_audit.go` — 审计表 Schema
- `internal/data/ent/schema/tool_agent_override.go` — 覆盖表 Schema
- `internal/agent/trpc_build.go` — Override 运行时生效 + AfterTool 审计
- `internal/agent/tool_assembly.go` — Override 配置合并
- 前端：`web/src/features/tools/` / `web/src/features/agents/` — Override 管理 + 审计页

### Phase 4（已实现）

catalog/策略层：
- `internal/data/builtin_tools_seed.go` — `diff_edit`/`patch_file` 种子条目
- `internal/tools/testexec/config.go` — diff_edit/patch_file case
- `internal/agent/prompt.go` — diff_edit 优先工作流提示
- `internal/tools/alias/alias.go` — `edit_file` → `diff_edit` 别名
- `internal/biz/tool/tool_policy_keys.go` — Policy Alias 双向一致
- `web/src/features/chat/diffEditHelpers.ts` — diff_edit/patch_file 事件处理
- `web/src/features/agents/useAgentToolsCatalog.ts` — defaultNativeToolKeys

运行时工具：
- `pkg/trpc-agent-go/tool/internal/textfile/` — 共享编码/行尾/quote fuzzy（claudecode 已复用）
- `pkg/trpc-agent-go/tool/file/patch/` — hunk 类型 + apply + unified 解析 + validate
- `pkg/trpc-agent-go/tool/file/patchfile.go` — patch_file 运行时工具（patch/hunks 互斥 + 原子写盘）
- `pkg/trpc-agent-go/tool/file/diffedit.go` — diff_edit 运行时工具（多 edit 原子 + 结构化错误）
- `pkg/trpc-agent-go/tool/file/editcontent.go` — SessionFileState load/commit 编排 + 原子写盘
- `pkg/trpc-agent-go/internal/toolcache/file_views.go` — per-invocation FileView 缓存
- `pkg/trpc-agent-go/tool/file/file.go` — `diff_edit`/`patch_file` 注册 + `WithDiffEditEnabled`/`WithPatchFileEnabled`
- `pkg/trpc-agent-go/tool/file/readfile.go` — 响应含 `mtime_ms` + 读后缓存 FileView
- `pkg/trpc-agent-go/tool/file/savefile.go` / `replacecontent.go` — 写盘后刷新 FileView

### Phase 5（已实现）

- `internal/agent/tool_assembly.go` — `resolveToolWorkspaceRoot` + `applyToolWorkspaceDirs`
- `internal/tools/toolset.go` — `AssemblyConfig.ShellExecDir` + hostexec `WithBaseDir`
- `internal/tools/trpc/toolsets.go` — `ToolsetConfig.ShellExecDir` 桥接
- `internal/tools/trpc/runtime_config.go` — `shell_exec` base_dir + claude_code 默认工作区
- `internal/data/builtin_tools_seed.go` — `workdir` 参数
- `internal/tools/hostexecnorm/` — `working_dir` 兼容映射
- `internal/agent/tool_confirm_gate.go` — `exec_command` 别名覆盖
- `internal/agent/prompt.go` — RuntimeCapabilityCue 更新
- `internal/tools/testexec/config.go` — TestTool shell 传 workspace

### Phase 6（已实现）

- `internal/biz/tool/tool.go` — 8 子接口 + ToolRegistryReader
- `internal/biz/agent_usecase.go` / `internal/agent/prompt.go` / `internal/team/runner.go` / `internal/runtime/deps.go` / `wire.go` — 窄接口传播
- `internal/tools/toolset.go` — 12 子装配器
- `internal/tools/tool.go` — ToolRegistration Tags + RegistryByTag/RegistryByCategory
- `internal/tools/kanban/bridge.go` — BridgeReader/Writer/Lifecycle
- `internal/tools/cache/result_cache.go` — LRU + 锁保护
- `internal/tools/kanban/` / `knowledge/` / `mcpobserve/` — 40+ 单元测试

### Phase 7（已实现）

- `internal/data/tool.go` / `tool_audit.go` — 19 处 kerrors 迁移
- `internal/tools/toolset.go` — Assemble 静默跳过 SysLogWarn
- `internal/tools/trpc/effective_config.go` — KnowledgeReflect 映射补全
- `internal/biz/tool/tool_catalog_runtime.go` — sync.RWMutex
- `internal/biz/tool/tool_test_invoke.go` — WithTimeout + 错误不吞
- `internal/tools/testexec/config.go` — knowledge_reflect 显式 case
- `internal/tools/skillruntime/filter.go` — LRU + RWMutex + Stats()

### Phase 8（已实现）

- `internal/biz/channel.go` — 4 子接口 + safego.Go
- `internal/biz/agent_usecase.go` — 4 子接口 + 2 独立
- `internal/tools/toolset_filter_test.go` — 7 用例
- `internal/tools/trpc/effective_config_test.go` — 15 映射 + 20 分支
- `internal/knowledge/adaptive_router.go` / `retrieval_evaluator.go` / `hybrid_retriever.go` — 简化 + 修正
- `internal/data/knowledge.go` — BM25 双路径
- `internal/biz/skill/skill_test.go` — 15 个新测试

### Round 5（已实现）

- `wire.go` — SkillUsecase → 窄接口
- `internal/biz/skill/skill.go` — CreateSkillDir 校验 + SkillReader/Writer
- `internal/tools/testexec/` / `internal/tools/trpc/` — 3 处 fmt.Errorf → kerrors
- `internal/biz/` — RunHealthChecks 闭包变量修复

### Phase 9（已实现，2026-07-20 Grok Build 借鉴）

- `internal/biz/tool/circuit_breaker.go` — HalfOpen 探针 `probeClaimedAt` 追踪 + 超时回收
- `internal/biz/tool/circuit_breaker_test.go` — 探针遗弃回归用例
- `internal/tools/tool.go` — `ToolRegistration.BehaviorVersion` + Registry `(name, behavior_version)` 索引
- `internal/tools/tool_version_test.go` — 版本锁定解析测试
- `internal/agent/tool_reminder.go` — Reminder 机制（文件修改 → 测试提醒闭环）
- `internal/agent/tool_reminder_test.go` — Reminder 收集测试

### Round 6（已实现，2026-08-14 深度评审修复）

- `internal/biz/tool/tool_policy_keys.go` — BUG-1 别名双向传播
- `internal/biz/agent_tool_bindings.go` — BUG-2 配置合并顺序
- `internal/agent/tool_result_cache.go` / `internal/tools/cache/result_cache.go` — PERF-1 快照 + DI
- `internal/data/tool.go` / `internal/data/tool_test.go` — PERF-2/3 + CONSISTENCY-1 truncateUTF8
- `internal/data/cascade_delete.go` — CONSISTENCY-2 级联清理
- `internal/data/tool_audit.go` — 审计 summary rune 安全截断
- `internal/tools/spirit_tools.go` / `internal/service/plan_executor.go` / `internal/agent/reply_reminder_inject.go` / `internal/data/builtin_tools_seed.go` — DEAD-1 check_progress 移除
- `internal/tools/toolset.go` — DEAD-2 FilesystemDir* 移除
- `internal/scenario/system/prompts/DECISION.md` / `internal/agent/prompt.go` — 提示词/注释同步清理

### Round 6 补充（2026-08-14 装配层提示级清理）

- `internal/tools/toolset.go` — deliverable 注册项改 `AssembledElsewhere` 占位（生产挂载路径收敛为 team 层 CustomTools 契约注入，消除 set/get/ack_deliverable 同名重复挂载的潜在风险）；`buildMCPToolSet` 补充 error 返回值语义注释
- `internal/tools/toolset_assemble.go` — 新增 `dedupFlatToolNames`（Assemble Phase 11）：扁平工具同名 earlier-wins 去重 + Warn；扁平工具 vs 静态 ToolSet 成员同名交叉检测（仅 Warn；MCP 等动态 ToolSet 不枚举避免网络往返）
- `internal/tools/toolset_more_test.go` — `TestAssemble_duplicateCustomToolNames` / `TestAssemble_deliverableNotMountedFromRegistry` 回归用例
- 验证：`go build ./internal/tools/... ./internal/agent/... ./internal/team/...` ✅ / `go vet ./internal/tools/...` ✅ / `go test ./internal/tools/ -count=1`（全量）✅ / deliverable、deferred、agent（Build/Tool 相关）✅；`internal/team` 测试包编译失败为存量问题（`parity_run_e2e_test.go` 缺 `GetTaskDeadLetter`，与本批无关）

### Round 6 补充二（2026-08-14 实施后 review 修复）

- `internal/tools/toolset.go` — `placeholderToolSetFactory`/`placeholderToolFactory` 接入全部 11 处 AssembledElsewhere 占位注册项（file/hostexec/google_search/message/claudecode/browser/deliverable/client + geminifetch/workspace_exec/read_tool_result），消除"helper 已定义未接线"的死代码状态；各条目占位原因注释保留在字段上方
- `internal/tools/decorator.go` — streamableToolDecorator 流代理 goroutine 由裸 `go func()` 改 `safego.Go(streamCtx, "tools.stream_proxy", ...)`（红线 #13：panic recovery）
- 验证：独立 GOCACHE 下 `go build ./internal/tools` ✅ / `go vet`（tools/agent/biz/tool/data）✅ / `go test ./internal/tools/ -count=1` 全量 ✅ / biz/tool、deferred、deliverable、cache、trpc、agent(Tool)、data(Tool,PG) ✅

### Round 7（2026-08-18 Cursor 对齐：搜索 / 诊断 / 终端）

| 项 | 状态 | 证据 |
|----|------|------|
| `search_content` 走 ripgrep | ✅ | `internal/tools/rgsearch`：`rg --json`，无匹配 exit 1 当成功；无 rg 回退 Go 扫盘；结果 cap 50 文件 / 20 行 / 200 命中 |
| `read_lints` | ✅ | `internal/tools/readlints`：`go vet`；catalog/filesystem 组；结果不缓存 |
| reminder 覆盖 `save_file` | ✅ | `tool_reminder.go` 精确名 + `file_name`；`read_lints` 清 reminder |
| shell 输出文件 / 流式 / 正则唤醒 | ✅ | `hostexec.WrapSessionEnhance`：`.aranea/shell/<session>.log`、`StreamableCall`、`notify_pattern`/`block_until_ms` |
| coding 默认可用 `shell_exec` | ✅ | profile 含 `shell_exec`；catalog 仍 `enabled=false`（opt-in-only），需确认 |

- `internal/tools/rgsearch/` — file ToolSet wrap，装配于 `assembleBuiltinToolsets`（worktree wrap 之前）
- `internal/tools/readlints/` — FunctionTool；`ToolsetConfig.ReadLints` → Assemble
- `internal/tools/hostexec/session_wrap.go` — 红acted 之后 wrap
- `internal/tools/hostexecnorm` — `block_until_ms` → `yield_time_ms`，`notify_on_output` → `notify_pattern`
- `internal/biz/agent_tool_policy.go` — coding + filesystem 组
- `internal/agent/tool_reminder.go` / `prompt.go`

### Round 8（2026-08-18 Cursor 对齐：Grep 细节 / 终端元数据 / 浏览器会话 / 子代理 kind）

| 项 | 状态 | 证据 |
|----|------|------|
| Grep 上下文 / type / 分页 | ✅ | `rgsearch` 传 `-A/-B/-C`、`--type`、`-U`；解析 `type:context`；`head_limit`/`offset`；`filenorm` 别名 |
| 终端 running_for / hung / stdin await | ✅ | `hostexec` session meta；`write_stdin` 支持 `notify_pattern`/`block_until_ms` |
| 浏览器串行 + snapshot 提示 | ✅ | `browser.WrapSession`：互斥锁；mutating 结果 `next_tool=browser_snapshot` |
| 子代理 kind + 阻塞 get | ✅ | `kind=explore\|verify\|general`；`subagents_get.block_until_ms`；`running_for_ms` |

- `internal/tools/rgsearch/`、`internal/tools/filenorm`
- `internal/tools/hostexec/session_wrap.go`、`session_meta.go`
- `internal/tools/browser/session_wrap.go`（装配于 `assembleBrowserToolset`）
- `internal/tools/subagent/kind.go`、`service.go`
- `internal/agent/prompt.go` 运行时 cue

### Round 9（2026-08-18 Cursor 对齐：诊断深度 / delete_file / 终端 pid）

| 项 | 状态 | 证据 |
|----|------|------|
| 空 path `read_lints` 走近期编辑 | ✅ | `internal/tools/editstamp`：写成功记 `.aranea/edited-paths.txt`（最多 32）；省略 path 时 lint 这些文件 |
| Go / Python / JS 诊断 | ✅ | Go `go vet`；Python `py_compile`（无解释器则跳过）；JS `node --check` |
| `delete_file` | ✅ | `internal/tools/deletefile`：工作区单文件；拒绝目录 / `.git` / 符号链接 / 区外；coding profile + filesystem 组 |
| 终端 `pid` | ✅ | hostexec `execResult.PID` / poll `PID`；`exec_command` 与 `write_stdin` 结果带 `pid` |

- `internal/tools/editstamp/`、`internal/tools/readlints/`、`internal/tools/deletefile/`
- `pkg/trpc-agent-go/tool/hostexec` — running 结果暴露 pid（产品 wrap 透传）
- `internal/agent/prompt.go` — 省略 path + `delete_file` cue

### Round 10（2026-08-18 spirit 闲聊 schema：computer_use / shell / 编排图 deferred）

| 项 | 状态 | 证据 |
|----|------|------|
| spirit 核心去掉 `build_orchestration_graph` | ✅ | `internal/tools/deferred/split.go` |
| computer_use_* / graph 映射到自身 | ✅ | `registry_map.go`；CustomTool 才能被 FinalizeDeferredTools 按名包装 |
| ToolFilter 收录 BaseName | ✅ | `shell`/`shell_exec` 别名不再漏完整 schema |
| aliasTool.ShouldDefer 委托 inner | ✅ | `runtime_alias.go` |

- `internal/tools/deferred/split.go`、`registry_map.go`、`tool_search.go`
- `internal/tools/runtime_alias.go`
- 测试：`split_test.go`、`registry_map_test.go`、`integration_test.go`、`runtime_alias_test.go`

### Round 11（2026-08-18 spirit 闲聊 schema：记忆写工具 / 上游交付物 deferred）

| 项 | 状态 | 证据 |
|----|------|------|
| 侧通道并入 deferred | ✅ | `MergeSideChannelDeferred`：`memory_add`/`update`/`delete`/`load` + `read_upstream_deliverable` |
| 扁平工具映射到自身 | ✅ | `registry_map.go`；不映射 `memory_search` / `memory_remember` |
| shard_plan 自动分离后合并 | ✅ | `resolveDeferredToolNames` |

- `internal/tools/deferred/split.go`、`registry_map.go`
- `internal/agent/shard_plan.go`
- 测试：`split_test.go`、`registry_map_test.go`、`integration_test.go`

### Round 12（2026-08-18 spirit 闲聊 schema：working_memory ToolSet deferred）

| 项 | 状态 | 证据 |
|----|------|------|
| 侧通道并入 `working_memory_*` | ✅ | `MergeSideChannelDeferred`；registry 名 `working_memory` |
| ToolFilter 按 NamedTool + BaseName 隐藏 | ✅ | `working_memory_write` / 声明名 `write` |

- `internal/tools/deferred/split.go`
- 测试：`split_test.go`、`registry_map_test.go`、`integration_test.go`

### Round 13（2026-08-19 审查：核心集白名单 + 编排收口 deferred + catalog cue 压缩）

| 项 | 状态 | 证据 |
|----|------|------|
| 核心集改为白名单合并 | ✅ | `MergeNonCoreMappedDeferred` 遍历已映射 − 核心；不再手写侧通道名单 |
| spirit 常驻收窄 | ✅ | 只留 `plan_and_execute` / `datetime` / `memory_search` / `memory_remember` |
| 收口工具映射到自身 | ✅ | `synthesize_results` / `get_team_deliverable` / `cancel_orchestration` |
| catalog cue 首句压缩 | ✅ | `compactCatalogDesc` ≤80 字 |

- `internal/tools/deferred/split.go`、`registry_map.go`、`catalog_cue.go`
- `internal/agent/shard_plan.go`
- 测试：`split_test.go`、`registry_map_test.go`、`catalog_cue_test.go`

### Round 14（2026-08-19 审查续：框架 skill 泄漏 + 指令与 tools 块对齐）

| 项 | 状态 | 证据 |
|----|------|------|
| spirit 只常驻 `skill_load` | ✅ | `allowedSkillToolsForAgent` → `WithAllowedSkillTools`；`skill_select_docs` 不再进 Request.Tools |
| M71 会话考古 deferred | ✅ | `search_messages` / `list_agent_sessions` / `read_session_history` 映射到自身 |
| CAPABILITIES 对齐常驻集 | ✅ | 不再把 `exec_command` / 收口工具写成「核心」；按需 `tool_load` |

- `internal/agent/prompt_mode.go`、`trpc_build.go`
- `internal/tools/deferred/registry_map.go`
- `internal/scenario/system/prompts/CAPABILITIES.md`、`DECISION.md`、`orchestrator.md`
- 测试：`prompt_mode_test.go`、`split_test.go`、`registry_map_test.go`


