# Tools 模块端到端深度审计（每工具逐核 · 调用链 · UI 操作 · 成功观测）

> 日期：2026-10-18 ｜ 类型：audit ｜ 范围：`internal/tools/**`、`internal/agent/tool_*`、`internal/biz/tool/**`、`internal/service/tool.go`、`api/kratos/tool/v1/tool.proto`、`internal/data/{tool.go, ent/schema/tool_invocation.go}`、`cmd/admin/wire*`、前端 `web/src/features/tools/**`、`web/src/stores/tools/**`、`web/src/components/tools/**`、`web/src/components/agents/AgentToolsSection.vue`
> 方法：原计划三路并行子代理深挖，遇持续 429 限流全部中断；改为主线只读直查取证（Grep/Read），结论逐项给出证据位置。
> 关联：本文档在 `docs/reports/2026-10-18-review-tools-module.md`（代码级审查，19 项发现已修）之后，针对**业务完备性**维度再查一轮。

## 0. 结论先行

- **33 个注册工具：0 个残缺桩函数、0 个永远报错、0 个注册后无法装配**；未实现的仅为「配置门」（缺 API key/SMTP 等），属设计行为。
- **调用链闭合且可观测**：BeforeTool（参数修复 → 熔断 → 权限/安全门 → 确认门）→ 执行 → AfterTool（记录三写 + 计数 + 审计 + 学习环）。24h KPI 卡片有明确 SQL 数据源且与明细页同源。
- **契约 22/22 对齐**；高危启用 `I_UNDERSTAND_RISK` 三端（UI→proto→biz）一致。
- **UI 全部动作闭环**（上一轮已按维度审查 + 本轮契约抽查），在线测试为真实执行器。
- 缺陷：**0 🔴 / 5 🟡 / 3 🟢**，全部记录于 §4。

## 1. 每个工具逐个核查

装配点图例：`Registry` = 登记工厂直装；`builtin` = [toolset_assemble.go:103](internal/tools/toolset_assemble.go#L103) `assembleBuiltinToolsets`；`search` = `:146`、`claudecode` = `:184`、`openapi` = `:231`、`agent` = `:267`、`mcp` = `:288`、`session` = `:398`、`subagent` = `:431`、`browser` = `:454`、`blob` = `:486`、`deferred` = `:501`；`Agent级注入` = 构造 Agent 时由设置驱动注入。

| # | 工具 | 类别 | 风险 | 确认门 | 默认 | 装配点 | 状态与证据 |
|---|------|------|------|--------|------|--------|-----------|
| 1 | `file` | filesystem | low | — | ✅开 | builtin（rgsearch + workspace 包装） | ✅ 完备；`internal/tools/rgsearch/`、`file_worktree_wrap.go` 均有实质测试 |
| 2 | `hostexec` | execution | **critical** | ✅ | 关 | builtin | ✅ 完备；`hostexec/redaction.go` 命令回显脱敏有测试 |
| 3 | `httpfetch` | web | medium | — | 关 | Registry（outboundguard SSRF 保护客户端，[toolset.go:95-99](internal/tools/toolset.go#L95)） | ✅ 完备 |
| 4 | `geminifetch` | web | medium | — | 关 | search（需 Gemini 配置） | ✅ 完备；配置门=未配置不装配 |
| 5 | `duckduckgo` | search | medium | — | 关 | Registry（HTML SERP；注释说明天气/新闻无 abstract 的设计取舍，[:126-129](internal/tools/toolset.go#L126)） | ✅ 完备 |
| 6 | `google_search` | search | medium | — | 关 | search（需 API key + CX） | ✅ 完备；配置门 |
| 7 | `arxiv_search` | search | low | — | 关 | Registry | ✅ 完备 |
| 8 | `wikipedia` | search | low | — | 关 | Registry | ✅ 完备 |
| 9 | `email` | communication | high | ✅ | 关 | Registry（SMTP 运行时配置） | ✅ 完备；缺 SMTP 配置时调用可读失败 |
| 10 | `message` | communication | high | ✅ | 关 | session（channel 路由） | ✅ 完备；`internal/outbound` 路由驱动 |
| 11 | `todo` | session | low | — | 关 | Registry | ✅ 完备 |
| 12 | `await_user_reply` | session | low | — | 关 | Registry | ✅ 完备 |
| 13 | `claudecode` | coding | **critical** | ✅ | 关 | claudecode（基础目录/只读/沙箱覆盖） | ✅ 完备；`claudecode_sandbox.go` 单测存在 |
| 14 | `openapi` | integration | medium | — | 关 | openapi（spec 驱动） | ✅ 实现完整；⚠️ 开启却未配置 spec 时**整个 Agent 装配失败**（见 §4-A5） |
| 15 | `agent` | composition | medium | — | 关 | agent（AgentToolConfig 驱动） | ✅ 完备 |
| 16 | `mcp` | integration | medium | — | 关 | mcp（MCPConfig：stdio/sse/http + 调用时凭证注入） | ✅ 完备；`mcp_production_test.go`/`mcp_verify_issue_test.go` |
| 17 | `mcpbroker` | integration | medium | — | 关 | mcp（发现/调用代理） | ✅ 完备 |
| 18 | `model_registry_sync` | system | medium | — | 关 | Agent 级注入（`modelsync/`） | ✅ 完备 |
| 19-22 | `subagents_spawn`/`_list`/`_get`/`_cancel` | composition | medium/low/low/medium | — | 关 | subagent（SubAgentService 驱动） | ✅ 完备；`subagent/` 有测试 |
| 23 | `browser` | browser | **critical** | ✅ | 关 | browser（Playwright MCP） | ✅ 完备；`browser/` 有 navigation_guard/config 测试 |
| 24 | `read_document` | media | medium | — | ✅开 | builtin（[:127](internal/tools/toolset_assemble.go#L127)） | ✅ 完备；⚠️ 登记元数据未标 `AssembledElsewhere`（§4-G1） |
| 25 | `read_lints` | filesystem | low | — | ✅开 | builtin | ✅ 完备 |
| 26 | `delete_file` | filesystem | medium | — | ✅开 | builtin | ✅ 完备 |
| 27 | `read_spreadsheet` | media | medium | — | ✅开 | builtin（[:131](internal/tools/toolset_assemble.go#L131)） | ✅ 完备；同 G1 |
| 28 | `read_tool_result` | system | low | — | ✅开 | blob + deferred | ✅ 完备；大结果 blob 侧车存取 |
| 29 | `working_memory` | memory | low | — | ✅开 | Registry | ✅ 完备 |
| 30 | `deliverable` | team | low | — | 关 | team runtime 注入（注释已在 [toolset.go:395-401](internal/tools/toolset.go#L395) 说明「不在通用装配重复挂载」的设计） | ✅ 完备 |
| 31 | `client` | interaction | medium | ✅ | 关 | session（clientbridge 进程单例） | ✅ 完备 |
| 32 | `datetime` | system | low | — | ✅开 | Registry | ✅ 完备 |
| 33 | `media` | media | medium | — | ✅开 | Agent 级注入（MediaProvider，[:436-440](internal/tools/toolset.go#L436)） | ✅ 完备；无 provider 时裁剪 |

**跨表核验**：别名策略一致（runtime 别名 15 条 vs biz 策略 14 条，`claude_code→claudecode` 仅 runtime，TPM-P1-01 无分裂）；catalog 与 Registry key 一一对应（上一轮审查已复验）；内部包全量 grep 无 TODO 桩函数（现存 TODO 均为带实现的债务注释）。

**默认开启 9 个**：file、read_document、read_lints、delete_file、read_spreadsheet、read_tool_result、working_memory、datetime、media——其中写侧能力仅 `delete_file`/`working_memory`，无高危默认，安全立场合理。
**确认门默认强制 6 个**：hostexec、email、message、claudecode、browser、client，与风险等级匹配（critical 全部强制确认）。

## 2. Agent 调用链与「调用是否成功」观测

### 2.1 链路（自证取证）

```
LLM tool_call
 ├─ BeforeTool（callback_chain.go 顺序链）
 │   ├─ repair_guard(prio1)：参数修复 ctx 标记（quality 标记消费于 recorder）
 │   ├─ 熔断 Allow()(prio5)：tools/agent/tool_circuit_breaker.go:103 —Open 时 CustomResult 短路
 │   ├─ permission_guard / command_safety（settings 门）
 │   └─ 确认门：plugin/trpc/runtime.go:168 → biz.RequiresConfirmationForAgent（上轮已改轻量查询）
 ├─ 实际执行（trpc ToolSet.Call）
 └─ AfterTool
     ├─ 熔断判定：IsTransientError（DeadlineExceeded 已计失败，上轮修复）
     ├─ recorder(prio50) → recordToolInvocationAfter（tool_invocation_recorder.go）
     ├─ next_action 跟踪 / result cache / 补偿跟踪 / reminder / 输出截断
     └─ TraceEmitter.CompleteToolCall + turntrace 桥
```

### 2.2 成功观测数据源

- **明细**：`tool_invitations`（tool_runs 接口）+ `tool_invocation_params`（参数详情，8000 截断、同一 ToolCallID upsert 去重）+ `tool_invocation_audits`。三写全在 10s 硬超时后台 goroutine（[tool_invocation_recorder.go:282-347](internal/agent/tool_invocation_recorder.go#L282)），失败 Warn 不阻塞调用。
- **状态判定**：`invocationStatusFromAfter` — `error → failed`；GNS3 族工具按 JSON 字段级判定业务失败（F11 修复后非子串猜）；`success` 为其余。
- **KPI**：`computeToolSummary`（[data/tool.go:250-265](internal/data/tool.go#L250)）从同源表聚合 24h 调用量/失败率，失败 = `failed+error+blocked` 口径，与明细页一致。
- **租户隔离**：runs 列表有 A3 租户边界（service/tool.go:369 注释 `tenant callers only see runs owned by their workspace`）。
- **指标**：Prometheus `ToolInvocationTotal{tool,status}` + MCP/Skill 分类计数。

### 2.3 三层结论

1. **"调用是否成功"在整个链路都有落点**：LLM 看到结果或短路文案，运行时写明细三表，仪表板聚合 KPI，审计页可回溯参数/结果预览。
2. **双层脱敏 belt+braces**：运行记录经 `internal/tools/preview.RedactAndTruncate` → biz 侧 `RecordToolInvocation` 再经 `RedactToolPreview`（上轮已加固 JWT/AKIA/厂商 token/PEM + 时间戳保护）。
3. **在线测试**：`TestTool → toolTesterAdapter → testexec.Execute`（[wire_adapters.go:211-241](cmd/admin/wire_adapters.go#L211)）真实构造执行器；`testexec/config.go` 为 30+ key 提供参数模板；不支持的 key 返回结构化"暂不支持"错误（[execute.go:75](internal/tools/testexec/execute.go#L75)），无伪成功。

## 3. UI 操作闭环核验

| 动作 | 端到端路径 | 状态 |
|------|-----------|------|
| 列表/搜索/过滤/分页 | api.listTools → tools store → useToolsPage → ToolsPage | ✅；`abnormal` 参数直达后端 where（contract 抽查通过） |
| 详情抽屉各 tab | tools store.detail → toolDetail 懒加载 | ✅（上轮 F1 旧数据缺陷已修） |
| 新建/编辑/删除 | editor store → create/update/delete + readonly 拦截（useToolToggle 测过） | ✅ |
| 启用/禁用（高危确认） | useToolToggle → toggleToolEnabled(confirm_intent) → proto field 3 → service → biz ToggleEnabled | ✅ 三端一致；9 个新单测全绿 |
| 配置编辑（密钥掩码） | UpdateToolConfig + MergeMaskedToolConfig | ✅（上轮审查核实） |
| 在线测试 | detail.runToolTest → TestTool → testexec | ✅ 结果/失败均有 UI 渲染 |
| Agent 绑定/覆盖 | effective 矩阵 → upsert/delete override | ✅；⚠️ 存量类型漂移（§4-G2） |
| 运行记录/审计 | ToolRunsPage/ToolAuditsPage → ListToolRuns/ListToolInvocationAudits | ✅ 参数预览双层脱敏 |
| 摘要卡片 | ListTools.summary → computeToolSummary | ✅ 数据源同源（口径见 §4-A2） |

## 4. 发现清单

### 🟡 建议项

| # | 发现 | 证据 | 状态（2026-10-19 修复复盘） |
|---|------|------|--------|
| A1 | **工具熔断器注册表无持久化、且为 per-agent 实例**：`buildCircuitBreakerRegistry`（[agent/tool_circuit_breaker.go:18-47](internal/agent/tool_circuit_breaker.go#L18)）不挂 `WithStateRepo`，且在每个 agent 构建时（[callback_chain.go:262](internal/agent/callback_chain.go#L262)）重建——Agent 设置变更触发缓存失效即全部复位 Closed；跨 agent 的同名工具互不认账。相比之下 NodeCircuitBreakerRegistry（[node_circuit_breaker.go:55](internal/biz/node_circuit_breaker.go#L55)）已接 StateRepo，双语义不对称且未文档化 | 代码直证 | ✅ **已修**。ADR-84（`docs/development/84-adr-tool-circuit-breaker-persistence.md`）定案后落地：进程级共享注册表 `ProvideToolCircuitBreakerRegistry`（StateRepo 持久化 + 启动恢复）；名空间 `tool:<workspaceID>:<tool>`（共享/落表）与 `tool_agent:<agentID>:<tool>`（overrides 隔离/易失）经 `WithPersistFilter` 分流；回调链由 per-build 改为 deps 注入；新增 8 个单测（过滤/恢复/降级/同空间共享/跨空间隔离/Agent 隔离）全绿 |
| A2 | **顶部 KPI 24h 口径全局**：`computeToolSummary` 的 `tool_invocations` 聚合无条件（[data/tool.go:252-258](internal/data/tool.go#L252)），该表 schema 无 workspace 列（[ent/schema/tool_invocation.go:22-51](internal/data/ent/schema/tool_invocation.go#L22)）。单工作空间部署无碍；按工作空间强隔离则工具总数过滤与调用量统计口径不一（前者走 `toolWhereClause(q)`） | 代码直证 | 📌 **保持设计**（本迭代不动）：拉通需改 schema + 写入路径 + 统计口径，收益/成本比低；已在本报告显式记录为已知口径，后续随多工作空间产品需求一并设计 |
| A3 | ~~**确认拦截调用完全不落记录/审计**~~ | 复审修正 | ✅ **前提修正 + 残余出口补齐**。复审 [tool_confirmation.go](internal/agent/tool_confirmation.go) 全分支证伪原判：**denied / timeout / 无回复通道**三条出口早已自录 `status=blocked` 行（细粒度 ErrorCode：confirmation_denied/timeout/required），预审器 153 行提前 return 是防双写守卫（同 ToolCallID upsert 终态升级）。唯一真漏的「等待确认失败（ctx 取消/通道异常）」出口已补自录（一致性对齐）；防双写处加定案注释禁止误删 |
| A4 | **双脱敏链分叉且时间戳口径不一**：运行链 `internal/tools/preview`（pattern 丰富）与 biz 链 `tool_preview.go`（上轮加固）各自独立维护同类 regex；preview 包 phoneRE 仍是宽松版（`(+\|00)?`+9–15 位裸数字，[preview.go:12](internal/tools/preview/preview.go#L12)）会把 10/13 位 Unix 时间戳当电话脱敏 | 代码直证 | ✅ **已修**。preview 包升级为唯一权威实现（B6 时间戳安全 phoneRE/JWT {5,} 段/AWS 8 前缀/GitHub 全家族/glpat/hf_/pk_ Stripe/PEM dotall+截断头兜底 ∪ 原细粒度模式，全部 \\b 锚定防 task-/risk- 误伤）；biz `RedactToolPreview` 薄委托，标记统一 `[secret redacted]`；新增 4 组回归单测（厂商家族/截断 PEM/时间戳存活/长后缀防误伤）全绿 |
| A5 | **openapi 开启但无 spec 配置 → 整个 Agent 装配失败**：`assembleFromRegistry` 对工厂错误直接 Internal 上抛，openapi Registry 工厂即「requires spec configuration」报错 | 代码直证 | ✅ **已修**。openapi 条目转 `placeholderToolSetFactory + AssembledElsewhere`（与 browser 等占位模式一致），`assembleOpenAPIToolsets` 增 enabled-但无-spec Warn 降级；回归测试 `TestAssemble_openapiWithoutSpecsDegrades` 全绿 |

### 🟢 提示项

| # | 发现 | 状态（2026-10-19） |
|---|------|------|
| G1 | `read_document`/`read_spreadsheet` 登记无 `Factory` 也未标 `AssembledElsewhere`，靠 builtin 阶段补装——功能性正确，但占位元数据不规范 | ✅ 已按 `read_lints` 同款补 `placeholderToolFactory` + `AssembledElsewhere` + 装配点注释 |
| G2 | 存量类型错 `AgentToolsSection.vue:141`（props 与 `AgentToolOverrideEditorDialog` 已漂移） | ✅ 已修且发现实质功能缺陷：对话框要求的 `modeOptions` 未传 + `@remove` 监听的 emit 不存在——**编辑器内删除链路运行期不可达**。已恢复：父组件透传 `modeOptions`（组合式函数本就暴露），对话框补 `remove` emit 与编辑态删除按钮 |
| G3 | `deliverable` 等带长注释的设计取舍已是文档化良实 | ✅ 保持（作为正面惯例，已在本轮 A5/G1 修复中沿用同款装配注释） |

## 5. 修复优先级与实施结果

按既定顺序全部完成（2026-10-19）：

1. ~~A5（openapi 无配置拖垮装配）~~ ✅
2. ~~A4（脱敏单源）~~ ✅
3. ~~A1（熔断器语义对齐）~~ ✅（先 ADR-84 后实施）
4. ~~A3（blocked 落档评审）~~ ✅（结论：既定设计正确 + 补 1 条残余出口）
5. ~~G1/G2（元数据与类型错）~~ ✅

A2 评审结论为「保持设计 + 显式记录口径」，不进本期改动清单。

### 5.1 变更文件清单

| 模块 | 文件 | 事项 |
|------|------|------|
| tools | `internal/tools/toolset.go` | A5 openapi 占位化 + G1 两条目元数据 |
| tools | `internal/tools/toolset_assemble.go` | A5 enabled-无-spec Warn 降级 |
| tools | `internal/tools/toolset_assemble_test.go` | A5 降级回归测试 |
| tools | `internal/tools/preview/preview.go` | A4 唯一权威脱敏实现 |
| tools | `internal/tools/preview/preview_test.go` | A4 回归测试 ×4 |
| biz | `internal/biz/tool/tool_preview.go` | A4 薄委托 |
| biz | `internal/biz/tool/tool_preview_test.go` | A4 标记口径对齐 |
| biz | `internal/biz/tool/circuit_breaker_config.go` | A1 `WithPersistFilter` + 三处过滤接入 |
| biz | `internal/biz/tool/circuit_breaker_provider.go`（新） | A1 进程级共享注册表提供器（ADR-84 命名空间） |
| biz | `internal/biz/tool/circuit_breaker_provider_test.go`（新） | A1 过滤/恢复/降级测试 |
| agent | `internal/agent/tool_circuit_breaker.go` | A1 全限定名 + 共享注册表改写 |
| agent | `internal/agent/tool_circuit_breaker_test.go` | A1 重写 + 共享/隔离/Agent 专属用例 |
| agent | `internal/agent/callback_chain.go` | A1 熔断 hook 改 deps 注入 |
| agent | `internal/agent/builder_deps.go` | A1 字段注释双角色化 |
| agent | `internal/agent/tool_confirmation.go` | A3 等待失败出口自录 blocked |
| agent | `internal/agent/tool_invocation_recorder.go` | A3 防双写守卫定案注释 |
| wire | `cmd/admin/wire.go` + `wire_gen.go` | A1 provider 注册 + 两处 BuilderDeps 注入（wire 重生成） |
| 前端 | `web/src/components/agents/AgentToolsSection.vue` | G2 透传 modeOptions |
| 前端 | `web/src/components/agents/AgentToolOverrideEditorDialog.vue` | G2 补 remove emit + 编辑态删除按钮 |
| 前端 | `web/src/features/agents/__tests__/useAgentToolOverrides.spec.ts` | G2 周边：终验时发现的存量 TS2322（夹具 `Record<string, unknown>+??` 推断 `{}` 与 `AgentEffectiveTool` 不兼容；HEAD 已存在、非本轮引入）收窄为精确 `Partial<Pick<…>>` |
| docs | `docs/development/84-adr-tool-circuit-breaker-persistence.md`（新） | A1 决策记录 |

### 5.2 回归验证结果（2026-10-19 终验）

| 域 | 命令面 | 结果 |
|----|--------|------|
| Go 构建 | `go build ./internal/tools/... ./internal/agent/... ./cmd/admin/...`（含 wire 重生成） | ✅ 通过 |
| Go 测试 | `go test ./internal/tools/... ./internal/biz/tool/ ./internal/agent/`（agent 侧聚焦 `CircuitBreaker\|ToolInvocation\|ToolConfirm\|Recorder`） | ✅ 全部 `ok`（biz/tool 7.851s、agent 2.698s，装配/熔断/记录/确认全链路绿） |
| 前端类型 | `vue-tsc --noEmit` 全量 + `AgentTool` 域过滤 | ✅ 该域 0 错（G2 漂移 + 存量 spec TS2322 均清零） |
| 前端测试 | `vitest run src/features/tools src/features/agents` | ✅ 13 文件 / 90 用例全过 |
