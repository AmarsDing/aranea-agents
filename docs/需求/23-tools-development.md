# Tools 工具 — 开发计划

> **版本**：7.0（2026-05-29）| **状态**：✅ 核心已实现；**Phase 4 片段编辑 ✅**；**Phase 5 工作区统一 ✅**；**Phase 6 架构优化 ✅**；**Round 3 P2/P3 修复 ✅**；**Phase 7 质量加固 ✅**；**Phase 8 ISP + 测试 + Knowledge ✅**
> **需求**：[23 tools.md](./23%20tools.md) · [23 tools-fragment-edit.md](./23%20tools-fragment-edit.md) · **设计**：[23 tools.design.md](./23%20tools.design.md) · [23 tools-fragment-edit.design.md](./23%20tools-fragment-edit.design.md) · **结构**：[23 tools struct design.md](./23%20tools%20struct%20design.md)
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
| Registry | `internal/tools/toolset.go` | Registry() + Assemble() 编排调度 + 子装配器 |
| Tags | `internal/tools/tool.go` | RegistryByTag / RegistryByCategory 查询 |
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
| **工作区统一（file+shell）** | ✅ 已实现 | `applyToolWorkspaceDirs` + `ShellExecDir` + hostexec `WithBaseDir` |
| **shell 参数 schema** | ✅ 已实现 | seed `workdir`；`hostexecnorm` 兼容 `working_dir` |
| **confirm 覆盖 exec_command** | ✅ 已实现 | `runtimeConfirmAliases`：`exec_command` ↔ `shell_exec` |
| **workspace_exec 装配** | ✅ 已修复 | registry 不独立挂载 nil executor；仅 CodeExecutor 路径 |

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
| 17 | **P1** | 片段级文件编辑 `diff_edit` | ✅ Phase 4 |
| 18 | **P1** | unified / hunk 补丁 `patch_file` | ✅ Phase 4 |
| 19 | **P1** | SessionFileState 会话缓存 | ✅ `toolcache.FileView` + `editcontent.go` |
| 20 | **P2** | `edit_file` 别名迁移至 `diff_edit` | ✅ Phase 4.10 |
| 21 | **P2** | 大文件行区间 patch | 📋 >1MB 仅加载 hunk ±context |
| 22 | **P2** | Activity diff 预览 | 📋 消费 `structured_patch` 字段 |
| 23 | **P0** | 工作区统一：hostexec 绑 `workspace_root` | ✅ Phase 5 |
| 24 | **P0** | `workdir` schema + `working_dir` 兼容映射 | ✅ Phase 5 |
| 25 | **P0** | tool_confirm 覆盖 `exec_command` | ✅ Phase 5 |
| 26 | **P1** | `claude_code` 默认工作区 | ✅ Phase 5 |
| 27 | **P2** | `workspace_exec` 仅 CodeExecutor 就绪时装配 | ✅ Phase 5.2；见 [32-codeexecutor-development.md](./32-codeexecutor-development.md) |
| 28 | **P1** | TestTool / prompt 与工作区口径同步 | ✅ Phase 5 |
| 29 | **P1** | ToolRepo 接口拆分（红线 #15） | ✅ Phase 6：18 方法 → 8 子接口 + ToolCatalogReader |
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

> **说明**：曾起草「53 Desktop App」文档，**不实施**；Shell 工作区优化归属本模块 Phase 5，不涉及 Electron/App 打包。

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

### Phase 4：片段级文件编辑（P1）

**目标**：在默认 `file` ToolSet 内提供 Cursor 式片段编辑（`diff_edit` / `patch_file`）与会话缓存，降低 token 与磁盘往返。需求与设计见 [23 tools-fragment-edit.md](./23%20tools-fragment-edit.md) · [23 tools-fragment-edit.design.md](./23%20tools-fragment-edit.design.md)。

**任务**：

| # | 任务 | 涉及文件 | 验收标准 |
|---|------|----------|----------|
| 4.1 | 抽取 `textfile` 共享包（encoding / line ending / quote fuzzy） | `pkg/trpc-agent-go/tool/internal/textfile/` · `tool/claudecode/` 改 import | ✅ claudecode 复用 textfile |
| 4.2 | 实现 `patch` 包（hunk 类型、apply、unified 解析） | `pkg/trpc-agent-go/tool/file/patch/` | ✅ `patch_test.go` |
| 4.3 | 实现 `patch_file` 工具 | `pkg/trpc-agent-go/tool/file/patchfile.go` · `file.go` | ✅ unified + hunk；原子写盘 |
| 4.4 | 实现 `diff_edit` 工具 | `pkg/trpc-agent-go/tool/file/diffedit.go` | ✅ 多 edit 原子；结构化错误 |
| 4.5 | 实现 SessionFileState | `editcontent.go` · `internal/toolcache/file_views.go` | ✅ `TestFileViewCache_SkipsSecondRead` |
| 4.6 | catalog 种子 + Effective Tools 组 | `builtin_tools_seed.go` · `agent_effective_tools.go` | ✅ filesystem 组含新 key |
| 4.7 | testexec + Activity 标签 | `testexec/config.go` · `activity_meta.go` | ✅ 在线测试 + 活动流中文名 |
| 4.8 | Agent Prompt 工作流 | `internal/agent/prompt.go` | ✅ diff_edit 优先工作流 |
| 4.9 | 前端 catalog 同步（若硬编码） | `useAgentToolsCatalog.ts` | ✅ defaultNativeToolKeys |
| 4.10 | Phase 2：`edit_file` 别名迁移（可选） | `runtime_alias.go` | ✅ `edit_file` → `diff_edit` |

**验收**（与需求 §5 对齐）：

- [x] `diff_edit` 单调用多片段替换且原子提交（`TestDiffEdit_MultiEditAtomic`）
- [x] `patch_file` unified diff 应用；hunk mismatch 零副作用（`TestPatchFile_Unified`）
- [x] SessionFileState 命中；外部 mtime 变化拒绝覆盖（`TestFileViewCache_SkipsSecondRead`）
- [x] `replace_content` / `save_file` 无破坏性变更
- [x] `go test ./tool/file/... ./tool/file/patch/...`（在 `pkg/trpc-agent-go` 模块内）

**建议迭代顺序**：4.1 → 4.2 → 4.3 → 4.4 → 4.5 → 4.6–4.9 → 4.10

---

### Phase 5：工具工作区统一（P0）

**目标**：Cursor 式项目根——file 与 `shell_exec` 共用 `workspace_root`；审计其余工具是否需要目录。

**背景差距**（2026-05-22 排查）：

- `file` 已通过 `resolveAgentFilesystemDir` 绑定工作区。
- `hostexec` 装配未调用 `WithBaseDir`，命令落在 Server **进程当前目录**。
- Catalog `working_dir` 与 hostexec `workdir` 不一致，Agent 传参被忽略。
- `tool_confirm` 未覆盖运行时名 `exec_command`。

**任务**：

| ID | 任务 | 涉及文件 | 验收 |
|----|------|----------|------|
| TW-5-01 | 抽取 / 复用 `resolveToolWorkspaceRoot` | `internal/agent/tool_assembly.go` | ✅ 单次解析，file+shell 同值 |
| TW-5-02 | `AssemblyConfig` / `ToolsetConfig` 增加 `ShellExecDir` | `toolset.go`, `trpc/toolsets.go` | ✅ 桥接层传递 |
| TW-5-03 | hostexec `WithBaseDir(ShellExecDir)` | `internal/tools/toolset.go` | ✅ `TestAssemble_hostexecUsesShellExecDir` |
| TW-5-04 | `shell_exec` runtime_config `base_dir` | `trpc/runtime_config.go` | ✅ Override 可覆盖 |
| TW-5-05 | seed 参数改为 `workdir` | `builtin_tools_seed.go` | ✅ 与 hostexec schema 一致 |
| TW-5-06 | `working_dir` → `workdir` 兼容 | `internal/tools/hostexecnorm` | ✅ 旧 JSON 仍可用 |
| TW-5-07 | confirm 映射 `exec_command` | `tool_confirm_gate.go`, `confirmationMap` | ✅ 确认 UI 触发 |
| TW-5-08 | 更新 `RuntimeCapabilityCue` | `internal/agent/prompt.go` | ✅ 口径与实现一致 |
| TW-5-09 | TestTool shell 传 workspace | `testexec/config.go` | ✅ 在线测试可跑 |
| TW-5-10 | `claude_code` 默认 `workspace_root` | `tool_assembly.go`, `runtime_config.go` | ✅ 未配 claude_code_dir 时回退 |

**Phase 5.2（P2，可选同迭代）**：

| ID | 任务 | 说明 |
|----|------|------|
| TW-5-11 | `workspace_exec` 禁止 nil executor 独立挂载 | ✅ 仅 `WithCodeExecutor` 路径启用 |
| TW-5-12 | 文档矩阵与 Skill CodeExecutor 根目录说明 | ✅ 设计 §7.8.2；execution-plan 已同步 |

**Phase 5 验收**：

- [x] 设置 `ARANEA_WORKSPACE_ROOT` 为测试项目根（单元测试 + 装配路径）
- [x] 启用 `shell_exec` + Agent profile 允许 runtime
- [x] `exec_command` 在 workspace 内执行（`TestAssemble_hostexecUsesShellExecDir`）
- [x] file 与 shell 共用 `resolveToolWorkspaceRoot`
- [x] 拒绝 tool_confirm → `blocked`（`exec_command` 别名覆盖）
- [x] 无需 App 壳；Web 联调即可验证

**建议顺序**：TW-5-01 → 5-03 → 5-05/5-06 → 5-07 → 5-08/5-09 → 5-10 → 5-11

---

### Phase 6：架构优化（ISP 合规 + 代码质量）

**目标**：从业务、用户、架构三个角度系统性优化 tools 模块，消除红线违规、降低圈复杂度、补充测试覆盖。

**任务**：

| ID | 任务 | 涉及文件 | 验收 |
|----|------|----------|------|
| TO-6-01 | ToolRepo 接口拆分 | `internal/biz/tool/tool.go` | ✅ 18 方法 → 8 子接口 + ToolCatalogReader 窄接口 |
| TO-6-02 | 窄接口传播 | `agent_usecase.go` / `agent/prompt.go` / `team/runner.go` / `runtime/deps.go` / `wire.go` | ✅ ToolCatalogReader 全链路一致 |
| TO-6-03 | Assemble 子装配器 | `internal/tools/toolset.go` | ✅ 170 行 → 12 个独立函数 |
| TO-6-04 | ToolRegistration Tags | `internal/tools/tool.go` / `toolset.go` | ✅ Tags 字段 + RegistryByTag/RegistryByCategory |
| TO-6-05 | kanban Bridge 拆分 | `internal/tools/kanban/bridge.go` | ✅ 9 方法 → BridgeReader/Writer/Lifecycle |
| TO-6-06 | 补充单元测试 | `kanban/` / `knowledge/` / `mcpobserve/` | ✅ 40+ 用例全部通过 |
| TO-6-07 | ResultCache LRU + 锁保护 | `internal/tools/cache/result_cache.go` | ✅ accessedAt + evictLRULocked + globalMu |
| TO-6-08 | 修复预存编译错误 | `memory_l4_cascade.go` / `timeline_hydrate.go` / `knowledge/tool.go` | ✅ uc.store → 子字段；messageSearchReader；Search 签名 |

**Phase 6 验收**：

- [x] `go build ./internal/biz/... ./internal/tools/...` 编译通过
- [x] `go test ./internal/tools/... ./internal/biz/tool/... -count=1` 全部通过（17 个包）
- [x] aranea-review 技能审查通过，无阻断项
- [x] ToolRepo / Bridge 子接口方法数均 ≤ 5（红线 #15 合规）
- [x] ToolCatalogReader 窄接口从 biz → agent → team → runtime → wire 全链路传播

### Phase 7：质量加固（错误处理 + 可观测性 + 并发安全）

**目标**：从业务、用户、架构三个角度继续深化 tools 模块质量，消除 data 层错误处理不规范、装配静默跳过、全局变量竞态等问题。

**任务**：

| ID | 任务 | 涉及文件 | 验收 |
|----|------|----------|------|
| TO-7-01 | data 层 kerrors 迁移 | `internal/data/tool.go` / `tool_audit.go` | ✅ 19 处 errors.New/sql.ErrNoRows → kerrors |
| TO-7-02 | Assemble 静默跳过添加日志 | `internal/tools/toolset.go` | ✅ geminifetch/google_search/Factory nil 添加 SysLogWarn |
| TO-7-03 | KnowledgeReflect 映射补全 | `internal/tools/trpc/effective_config.go` | ✅ ToolsetConfigFromEffectiveKeys 添加映射 |
| TO-7-04 | toolWebResChecker 线程安全 | `internal/biz/tool/tool_catalog_runtime.go` | ✅ sync.RWMutex 保护全局变量读写 |
| TO-7-05 | TestTool 超时控制 + 错误不吞掉 | `internal/biz/tool/tool_test_invoke.go` | ✅ WithTimeout(5s) + SysLogWarn 记录失败 |
| TO-7-06 | testexec knowledge_reflect case | `internal/tools/testexec/config.go` | ✅ 显式 case 返回 false |
| TO-7-07 | filterCache LRU + RWMutex + 可观测性 | `internal/tools/skillruntime/filter.go` | ✅ LRU 驱逐 + atomic 计数器 + Stats() + RWMutex |

**Phase 7 验收**：

- [x] `go test ./internal/tools/... ./internal/biz/tool/... -count=1` 全部通过（17 个包）
- [x] aranea-review 技能审查通过，无阻断项
- [x] data 层所有错误返回使用 kerrors（BE1 合规）
- [x] 全局变量均有锁保护（BC3 合规）
- [x] 无错误被静默吞掉（BE4 合规）

---

## 5. 任务清单

| # | 任务 | 优先级 | Phase | 依赖 | 代码锚点 |
|---|------|--------|-------|------|----------|
| 1 | ToolOverride 运行时读取与生效 | P2 | 1 | — | `trpc_build.go` → `buildToolsetsForAgent` |
| 2 | ToolOverride 前端管理页 | P2 | 1 | #1 | 前端 Agent 设置页 |
| 3 | `TestTool` RPC + 在线测试 | P3 | 2 | — | `tool.proto` / `tool.go` |
| 4 | `tool_invocation_audit` 表 + 写入 | P3 | 3 | — | Ent schema / `trpc_build.go` |
| 5 | 审计查询 API + 前端页 | P3 | 3 | #4 | `tool.proto` / 前端 |
| 6 | 片段编辑 `diff_edit` + `patch_file` | P1 | 4 | — | ✅ `pkg/trpc-agent-go/tool/file/` |
| 7 | SessionFileState 会话缓存 | P1 | 4 | #6 | ✅ `toolcache.FileView` |
| 8 | catalog / Prompt / Activity 集成 | P1 | 4 | #6 | ✅ seed / prompt / activity_meta |
| 9 | `edit_file` 别名迁移（可选） | P2 | 4 | #6 | ✅ → `diff_edit` |
| 10 | 工作区统一（hostexec + schema + confirm） | **P0** | **5** | — | ✅ TW-5-01–5-10 |
| 11 | `workspace_exec` / CodeExecutor 装配 | P2 | 5.2 | 10 | ✅ `toolset.go` registry nil |
| 12 | ToolRepo 接口拆分 + 窄接口传播 | **P1** | **6** | — | ✅ 8 子接口 + ToolCatalogReader |
| 13 | Assemble 子装配器重构 | P1 | 6 | — | ✅ 12 个 assembleXxx 函数 |
| 14 | ToolRegistration Tags + 查询 | P2 | 6 | — | ✅ RegistryByTag/RegistryByCategory |
| 15 | kanban Bridge 接口拆分 | P2 | 6 | — | ✅ BridgeReader/Writer/Lifecycle |
| 16 | 补充单元测试 | P3 | 6 | — | ✅ 40+ 用例 |
| 17 | ResultCache LRU + 锁保护 | P3 | 6 | — | ✅ evictLRULocked + globalMu |
| 18 | data 层 kerrors 迁移 | **P1** | **7** | — | ✅ 19 处 errors.New/sql.ErrNoRows → kerrors |
| 19 | Assemble 静默跳过添加日志 | P1 | 7 | — | ✅ SysLogWarn |
| 20 | KnowledgeReflect 映射补全 | P1 | 7 | — | ✅ effective_config.go |
| 21 | toolWebResChecker 线程安全 | P1 | 7 | — | ✅ sync.RWMutex |
| 22 | TestTool 超时 + 错误不吞 | P2 | 7 | — | ✅ WithTimeout + SysLogWarn |
| 23 | testexec knowledge_reflect case | P2 | 7 | — | ✅ 显式 case |
| 24 | filterCache LRU + RWMutex + Stats | P2 | 7 | — | ✅ atomic 计数器 |

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

### Phase 4（片段级文件编辑）

- [x] `diff_edit` / `patch_file` 运行时可用（默认 file ToolSet）
- [x] SessionFileState 同 invocation 缓存生效（`toolcache.FileView`）
- [x] catalog / Effective Tools / Prompt / Activity 集成完成

### Phase 5（工具工作区统一）

- [x] file 与 shell 共用 `workspace_root`
- [x] `workdir` schema 与 `working_dir` 兼容
- [x] `exec_command` 纳入 tool_confirm
- [x] Web 联调验收通过（不依赖 App 打包）

### Phase 6（架构优化）

- [x] ToolRepo 18 方法拆分为 8 子接口（红线 #15 合规）
- [x] ToolCatalogReader 窄接口全链路传播
- [x] Assemble 170 行拆分为 12 子装配器
- [x] ToolRegistration Tags 字段 + RegistryByTag/RegistryByCategory
- [x] kanban Bridge 9 方法拆分为 3 子接口
- [x] kanban/knowledge/mcpobserve 单元测试 40+ 用例
- [x] ResultCache LRU 驱逐 + 全局单例锁保护
- [x] aranea-review 审查通过，无阻断项

### Phase 7（质量加固）

- [x] data 层 19 处 errors.New/sql.ErrNoRows → kerrors
- [x] Assemble 静默跳过添加 SysLogWarn 日志
- [x] KnowledgeReflect 映射补全
- [x] toolWebResChecker 全局变量 sync.RWMutex 保护
- [x] TestTool WithTimeout(5s) + 错误不吞掉
- [x] testexec knowledge_reflect 显式 case
- [x] filterCache LRU + RWMutex + atomic 计数器 + Stats()
- [x] aranea-review 审查通过，无阻断项

### Phase 7a（测试补全 + 红线修复 — 前序迭代）

- [x] `channel.go:315` 裸 `go func()` 修复为 `safego.Go`（红线 #13）
- [x] `ToolsetConfigHasAny` 全 20 分支测试覆盖
- [x] `ToolsetConfigFromEffectiveKeys` 全 15 映射路径 + 9 filesystem 子键测试覆盖
- [x] aranea-review 审查通过，0 阻断项

### Phase 8（ISP 合规 + 测试补全 + Knowledge 增强）

**目标**：完成 AgentRepository/ChannelRepo ISP 拆分、ToolFilterForPrefix 测试覆盖、Knowledge AdaptiveRouter 全链路注入、BM25 双路径搜索优化。

**任务**：

| ID | 任务 | 涉及文件 | 验收 |
|----|------|----------|------|
| TO-8-01 | ChannelRepo 14 方法 → 4 子接口 | `internal/biz/channel.go` | ✅ ChannelReader(3)/Writer(3)/CredentialRepo(3)/DeliveryRepo(4) |
| TO-8-02 | AgentRepository 17 方法 → 4 子接口 + 2 独立 | `internal/biz/agent_usecase.go` | ✅ AgentReader(4)/Writer(3)/RuntimeSettingsRepo(2)/PromptFileRepo(5) + ListAgentCreators + ExecInTx |
| TO-8-03 | ToolFilterForPrefix 7 用例测试 | `internal/tools/toolset_filter_test.go` | ✅ 空前缀/空白/匹配/不匹配/nil Tool/nil Declaration/TrimSpace |
| TO-8-04 | effective_config 测试扩展 | `internal/tools/trpc/effective_config_test.go` | ✅ 15 映射路径 + 20 分支全覆盖 |
| TO-8-05 | channel.go safego.Go 修复 | `internal/biz/channel.go` | ✅ 红 线 #9 合规 |
| TO-8-06 | AdaptiveRouter 全链路注入 | `chat_orchestrator.go` / `runner.go` / `wire.go` | ✅ Chat + Team 双路径共用 |
| TO-8-07 | BM25 双路径搜索 | `internal/data/knowledge.go` | ✅ tsvector + pg_trgm + mergeBM25Results |
| TO-8-08 | RetrievalEvaluator 逻辑修正 | `internal/knowledge/retrieval_evaluator.go` | ✅ 空 chunks 先于 nil LLM 检查 |
| TO-8-09 | RetrievalEvaluator 测试 | `internal/knowledge/retrieval_evaluator_test.go` | ✅ nil LLM/空 chunks/parseAssessment/buildChunksSummary/truncateString/parseJSONLoose |
| TO-8-10 | HybridRetriever 清理 | `internal/knowledge/hybrid_retriever.go` | ✅ 移除未用 cosineSimilarity + math import |
| TO-8-11 | AdaptiveRouter 简化 | `internal/knowledge/adaptive_router.go` | ✅ Search 签名简化 + nil guard + SysLogWarn |
| TO-8-12 | biz/skill 测试补全 | `internal/biz/skill/skill_test.go` | ✅ 15 个新测试函数 |

**Phase 8 验收**：

- [x] `go vet ./internal/biz/...` 编译通过
- [x] `go build ./internal/biz/...` 编译通过
- [x] `go test ./internal/tools/ -run TestToolFilterForPrefix -count=1 -v` 7 用例全通过
- [x] aranea-review 审查通过，0 阻断项、4 建议、1 提示
- [x] AgentRepository / ChannelRepo 子接口方法数均 ≤ 5（红线 #15 合规）
- [x] AdaptiveRouter 通过 RuntimeTooling + context 注入，Chat/Team 共用逻辑（BR7 合规）

---

## 8. 剩余工作

### P1（高优先级）

| # | 任务 | 说明 |
|---|------|------|
| 1 | `buildMCPToolSet` / `buildMCPBrokerTools` 测试 | Assemble 子装配器中 MCP 路径无测试覆盖 |
| 2 | Ent schema 重新生成 | `internal/data/` 编译错误（Visibility/FallbackConfigJSON/FileManifestJSON/MessageID/CompressVersion 字段缺失），需 `go generate` 重新生成 |

### P2（中优先级）

| # | 任务 | 说明 |
|---|------|------|
| 3 | `custom/` 包冒烟测试 | 用户模板工具无测试 |
| 4 | `memory/` 包测试 | 整个包无测试文件 |
| 5 | tools 层 `fmt.Errorf` 评估 | `internal/tools/knowledge/tool.go` 13 处 `fmt.Errorf`，评估是否转为 `kerrors.BadRequest` |
| 6 | BM25 分数归一化 | `mergeBM25Results` 直接拼接 tsvector 和 trigram 结果，分数尺度不同可能导致排序偏差 |
| 7 | `AdaptiveRouter.Search` 签名简化 | 调用方常传 `nil` + `""`，考虑提供简化签名 |

### P3（低优先级）

| # | 任务 | 说明 |
|---|------|------|
| 8 | `BuildToolsets` 集成测试 | 核心桥接函数需 mock-heavy 测试 |
| 9 | E2E 全链路测试 | `read_file` → `diff_edit` → shell 读同路径 |
| 10 | `AgentPromptFileRepo` 监控 | 恰好 5 方法处于红线边界，新增方法需立即拆分 |

---

## 7. 依赖与风险

| 项 | 说明 |
|----|------|
| ToolOverride 运行时 | 需在 `buildToolsetsForAgent` 中读取 Override 列表并调整装配逻辑；需注意 Override 与 Effective Tools 策略的优先级 |
| 在线测试 | 需构造临时 Agent 执行单次工具调用，需考虑安全隔离与超时 |
| 审计日志 | 需注意存储膨胀；建议自动清理策略（默认 90 天） |
| BeforeTool Callback | 框架已支持但项目未使用；可用于动态参数注入、权限校验、审批流前置检查 |
| MCP 工具安全 | MCP Broker 的 `mcp_call` 可动态调用任意已注册 MCP 工具，需在生产环境限制 AdHoc HTTP |
| 片段编辑与 claudecode 重复 | 抽取 `tool/internal/textfile` 共享逻辑，避免双份 patch 实现；claudecode 仍负责 Bash / Notebook |
| SessionFileState 删盘边界 | 同 invocation 内删盘后仍可用 cache 编辑（`TestFileViewCache_SkipsSecondRead`）；外部变更靠 `mtime_ms`；见 [Phase 4 Review FRAG-P2-03](../review/2026-05-22-Tools-Phase4-Fragment-Edit-Review.md) |
| 别名迁移 | `edit_file` → `diff_edit`（2026-05-22）；**`runtime_alias.go` 与 `tool_policy_keys.go` 须同步** |
| **工作区统一** | ✅ Phase 5：`ShellExecDir` + `hostexecnorm` + confirm 别名 |
| **53 Desktop App 文档** | 已删除、不实施；Shell 优化归属 Phase 5（已完成） |
