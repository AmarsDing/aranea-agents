# Tools / Plugin / Skill / MCP 代码层综合 Review

> **总评分**：**82 / 100** | **整体风险等级**：P1
> **审查时间**：2026-05-26
> **范围**：`internal/tools/`（62 文件）· `internal/plugin/trpc/`（48 文件）· `internal/skill/`（16 文件）· `internal/mcp/`（9 文件）+ 相关运行时桥接（`internal/agent/tool_assembly.go`、`internal/agent/mcp_oauth.go`、`internal/service/skill_import_http.go`）
> **聚焦**：四大子系统的架构边界、运行时与平台层分离、扩展性、安全性、可观测性
> **真相源**：`docs/AGENT_RUNTIME_BOUNDARY.md`、`.cursor/rules/trpc-agent-framework-first.mdc`、`docs/需求/19 mcp.design.md`、`docs/需求/22 plugin.design.md`
> **历史 Review**：
> - [23-tools-review.md](./23-tools-review.md) · [Phase 4](./2026-05-22-Tools-Phase4-Fragment-Edit-Review.md) · [Phase 5](./2026-05-22-Tools-Phase5-Workspace-Unification-Review.md)
> - [22-28 Plugin/Callback](./22-28-plugin-callback-review.md)
> - [20-skill-review.md](./20-skill-review.md)
> - [19-mcp-review.md](./19-mcp-review.md)

---

## 0. 总览：四个子系统的定位与分数

| 子系统 | 文件数 | LoC（估） | 分数 | 风险 | 一句话定位 |
|--------|-------|-----------|------|------|------------|
| **internal/tools** | 62 | ~4.8k | **86** / 100 | P2 | tRPC Agent 框架工具的装配与平台扩展（webresearch/kanban/knowledge/skill/MCP） |
| **internal/plugin/trpc** | 48 | ~4.8k | **80** / 100 | P1 | 9 内置 Plugin + Hook Chain + ModelSelector + OnEvent 四层编排 |
| **internal/skill** | 16 | ~2.4k | **78** / 100 | P1 | ZIP 导入 + Layer A/B 运行时 + fsnotify 双写同步 |
| **internal/mcp** | 9 | ~0.6k | **80** / 100 | P1 | MCP 平台层：config/probe/metadata/health/alert/classify |
| **加权平均** | 135 | ~12.6k | **82** | P1 | 边界基本干净；P1 集中在协议漂移与 fail-open |

> **加权说明**：plugin / tools 各占 30%，skill / mcp 各占 20%。

---

## 1. 评分详情（按子系统拆分）

### 1.1 internal/tools — 86 / 100

| 维度 | 得分 | 满分 | 评述 |
|------|------|------|------|
| 业务正确性 | 17 | 20 | 装配/runtime/alias 三链清晰；`web_search` alias 在 biz 与 runtime 双向漂移（→`web_research` vs →`duckduckgo_search`） |
| 架构一致性 | 22 | 25 | 中央 Registry + `Assemble()` + `trpc/` 适配层 + 领域子包；`internal/biz` 0 处反向 import `internal/tools`（验证通过） |
| 后端实现质量 | 17 | 20 | `PruneUnconfiguredToolFlags` 防止 credential 缺失导致整 Agent 失败；webresearch 支持 partial 结果；hostexec normalize 桥接 schema 漂移 |
| 测试覆盖 | 7 | 10 | toolset_assemble/workspace、prune、webresearch、hostexecnorm 都有；kanban/knowledge/serviceawaitreply 测试稀薄 |
| 可扩展性 | 14 | 15 | 子包横向扩展无侵入；OpenAPI specs 错误被 `continue` 静默吞掉 |
| 文档/注释 | 9 | 10 | `doc.go` 高质量；`runtime_alias.go` 与 `biz/tool/tool_policy_keys.go` 存在错位需对齐 |

### 1.2 internal/plugin/trpc — 80 / 100

| 维度 | 得分 | 满分 | 评述 |
|------|------|------|------|
| 业务正确性 | 15 | 20 | 9 内置 Plugin 全 ✅；**cost_guard 双重 block**（ModelSelector 已 fallback 后 `TryConsume` 仍硬拦截）；**output_policy on_event 不强制 block** |
| 架构一致性 | 22 | 25 | `orchestration.go` 四层注释清晰；Manager / Runtime 双重持有但红线无违反；`internal/biz` 0 处 import `internal/plugin` |
| 后端实现质量 | 16 | 20 | 9 内置 builtin / 4 层优先级（3-50 / 200+ / 300+） / Scope 隔离 / safe logger / SSRF 防护齐全；**整链无 panic recover** |
| 测试覆盖 | 7 | 10 | orchestration_policy / cost_guard scope / hook_notify SSRF / chain mirror double-trigger 都测；P1 双重 block 路径无回归保护 |
| 可扩展性 | 12 | 15 | Plugin / Hook / OnEvent 加新点低成本；**`admin_bypass`/`confirm_tools`/`role_rules` 三个 schema 字段未被读取**（死配置）|
| 文档/注释 | 8 | 10 | 顶部 orchestration 块 + `ValidatePluginCallbackPoints` warning 链 ✅；优先级阶梯需 admin 文档显式列出 |

### 1.3 internal/skill — 78 / 100

| 维度 | 得分 | 满分 | 评述 |
|------|------|------|------|
| 业务正确性 | 14 | 20 | importer / watch / trpc / storage 四层闭环；**`Summary.Name` 用了 display name 而非 slug，过滤器按 slug 匹配 → 不命中**；**`ApplyImport` 无事务，部分写入留垃圾** |
| 架构一致性 | 22 | 25 | 红线无违反（biz 不 import skill / framework）；watch + importer 共享 `ValidateSkillPackage` |
| 后端实现质量 | 16 | 20 | ZIP size limit / 黑名单 / 高风险扩展拦截 / fsnotify debounce 2s / 5min reconcile 都到位；zipslip 仅 `strings.Contains("..")` 不彻底 |
| 测试覆盖 | 4 | 10 | 仅 2 个 trivial 测试（slug canon 化）；zipslip / partial apply / watch race / filter slug-vs-name 全 0 |
| 可扩展性 | 12 | 15 | 版本回滚未实现；RBAC 全 hard-coded `true` |
| 文档/注释 | 7 | 10 | 与 20-skill-development 对齐；i18n 占位 `"????"` / `"?"` 分隔符未完成 |

### 1.4 internal/mcp — 80 / 100

| 维度 | 得分 | 满分 | 评述 |
|------|------|------|------|
| 业务正确性 | 16 | 20 | config / probe / metadata / health / alert 5 个子包职责清楚；**transport alias 在 config / probe / runtime / oauth 4 处分裂**（`streamable` vs `streamable_http`） |
| 架构一致性 | 22 | 25 | `MCPProber` / `MCPMetadataEditor` 走 biz 端口；platform 与 runtime 完全分离 |
| 后端实现质量 | 16 | 20 | SSRF 防护（DNS + RFC1918）；`TryLock` 防止 probe pile-up；`ShouldEmitHealthAlert` debounce；**HTTP redirect 未拦截**（可绕过 SSRF） |
| 测试覆盖 | 5 | 10 | config / classify / metadata 有；**probe / health / alert 零测试** |
| 可扩展性 | 12 | 15 | `ToTRPCConnectionConfig` 写了但**生产从未调用**，runtime 在 `toolset.go` 手写 mapping → 双 source of truth |
| 文档/注释 | 9 | 10 | 与 `19 mcp.design.md` 对齐 |

---

## 2. 各子系统架构图（一句话定位）

```
┌─────────────────────────────────────────────────────────────────────────┐
│                          internal/agent                                  │
│  trpc_build.go  ←  ChatOrchestrator / TeamRunner / GraphRunner            │
│                                                                          │
│      ┌───────────────────────┬───────────────┬────────────────┐         │
│      ▼                       ▼               ▼                ▼         │
│  tool_assembly.go      plugintrpc.MergeChain  skillruntime    mcp_oauth │
│   (装配工具)             (产品 Chain ⊕ Hook)    (Layer A/B)    (Token 解析)│
└──────┬─────────────────────┬────────────────┬─────────────────┬─────────┘
       │                     │                │                 │
   internal/                internal/      internal/         internal/
    tools/                 plugin/trpc/    skill/           mcp/
       │                     │                │                 │
   ┌───┴────┐         ┌─────┼─────┬────────┐ │            ┌────┴────┐
   │ trpc/  │         │ 9 内置  │ Hook  │ Chain │   importer│           │ probe   │
   │ adapter│         │ Plugin │ Bridge│ Chain │            │           │ health  │
   ├────────┤         │ Runner │ Event │ Mirror│   trpc/   │           │ alert   │
   │ web    │         │ Manager│       │       │   (FS/DB) │           │ classify│
   │ kanban │         └─────────┴───────┴───────┘   storage │           │ metadata│
   │ knowl. │                                       watch  │           │ config  │
   │ memory │                                              │           │         │
   │ skill  │                                                          └─────────┘
   │ mcp    │                                            
   │ ...    │              
   └────────┘             
       │                                                                
       ▼                                                                
   pkg/trpc-agent-go/tool/* (framework 真相源)
```

---

## 3. 跨子系统的"七大红线"核查

| 红线 | tools | plugin | skill | mcp |
|------|-------|--------|-------|-----|
| `internal/biz` 不 import 子包 | ✅ | ✅ | ✅ | ✅ |
| `internal/biz` 不 import `pkg/trpc-agent-go` | ✅（间接） | ✅ | ✅ | ✅ |
| Runner/Manager 装配在 `internal/service` 而非 `internal/server` | ✅ | ✅（`PluginService` / `HookService`）| ✅（importer 走 service） | ✅（`MCPServerUsecase`）|
| `internal/server` 0 处 import 子包 | ✅ | ✅ | ⚠️ `skill_import_http.go` 在 `internal/service`，但走 `registerCustomRoutes` 绕过 proto 契约 | ✅ |
| 子包之间无循环依赖 | ✅ | ✅ | ✅ | ✅ |
| 与 framework 类型保持 alias（不 fork 实现）| ✅（`tool.go` 全 alias） | ✅（`trpcplugin.Plugin` 直实现） | ✅（`trpcskill.Repository` 适配）| ✅（`MCPServerConfig.ToConnectionConfig` 统一映射） |
| 无 `panic()` 直接抛出 | ✅ | ✅ 2026-05-28：`wrapResilient` + `recoverHookPanic` 已覆盖 | ✅ | ✅ |

---

## 4. P1 问题清单（本迭代必须处理）

| ID | 子系统 | 问题 | 位置 | 影响 | 建议修复 |
|----|-------|------|------|------|---------|
| **TPM-P1-01** | tools | `web_search` alias **二义性**：biz `tool_policy_keys.go:11` → `web_research`；`runtime_alias.go:24` → `duckduckgo_search` | `internal/biz/tool/tool_policy_keys.go:11` · `internal/tools/runtime_alias.go:24` | Policy 解析认为 `web_search` = `web_research`；LLM 真的调用 `web_search` 时却路由到 `duckduckgo_search`，两条路径分裂 | 二选一：（a）`runtime_alias.go` 删除 `web_search` 映射并依赖 policy normalization；（b）biz 与 runtime 都映射到 `web_research`。增加 integration 测试 |
| **TPM-P1-02** | tools | `aliasTool.Call` 对非 `CallableTool` 返回 `(nil, nil)` | `internal/tools/runtime_alias.go:45-50` | LLM 收到"成功且空"响应，难定位失败 | 返回 `fmt.Errorf("alias %q: inner tool is not callable", a.name)` |
| **TPM-P1-03** | plugin | cost_guard **双重 block**：ModelSelector 已通过 `ResolveCostGuardTarget` 路由到 fallback model，`beforeModel` 中 `TryConsume(DailyTokenBudget, est)` 仍可能失败并 hard block | `internal/plugin/trpc/cost_guard.go:74-81` | 配了 fallback 的 Agent 在日额度耗尽时仍 turn-blocked，违反 "fallback 优先于硬拦" 设计意图 | 在 `beforeModel` 检测当前 model 是否为 ModelSelector 选出的 fallback，是则跳过 `TryConsume` 失败 block；或在 selector 阶段就 consume |
| **TPM-P1-04** | plugin | `output_policy.onEvent` 在违规时**只 log 不拦截**，不论 `block_on_violation` 配置 | `internal/plugin/trpc/output_policy.go:78-96` | 流式违规内容仍透传到客户端，与 `afterModel` 路径行为不一致；admin 配 `block_on_violation=true` 形同虚设 | 流式路径应返回 modified `Event`（去除违规片段）或 `CustomResponse: blockedModelResponse(...)` |
| **TPM-P1-05** | plugin | 整个 plugin chain 无 `recover()` 包裹 | `internal/plugin/trpc/hook_resilience.go` 整文件 + Chain 装配 | 任一 hook panic → 整个 turn 崩；`hook_resilience.go` 只吞 error 不吞 panic | 在 `wrapResilient` 与 Chain wrapper 中 `defer recover()` → log + `record(.., "panic")` + 继续 |
| **TPM-P1-06** | skill | `DBRepositoryAdapter` 设置 `Summary.Name = c.Name`（display name），但 `filter.go` 与 `skillruntime/filter.go` 都以 slug allow-list 比对 `summary.Name` | `internal/skill/trpc/db_repository.go:139` · `internal/skill/trpc/filter.go:18-19` · `internal/tools/skillruntime/filter.go:37-38` | DB-backed skill 全部被过滤掉（slug ≠ display name）；Layer A 实际不生效 | `DBRepositoryAdapter` 改为 `Summary.Name = slug`（保留 display name 走 `Description` 或新字段），或 filter 改为按 repository key/slug 匹配 |
| **TPM-P1-07** | skill | importer ZIP `createImportedSkill` 缺 **路径包含校验**：仅 `strings.Contains(clean, "..")`，未做 `filepath.Rel(targetDir, joined)` | `internal/skill/importer/engine.go:255-260` | Windows drive 前缀、编码变体可能逃逸 `targetDir`（zipslip 不完全防护） | `joined := filepath.Join(targetDir, clean)`; `rel, err := filepath.Rel(targetDir, joined)`; 若 `err != nil` 或 `strings.HasPrefix(rel, "..")` 则拒绝 |
| **TPM-P1-08** | skill | `ApplyImport` 循环写入无事务/回滚：前几条 `createImportedSkill` 已写盘 + DB row，后续失败直接 `return` | `internal/skill/importer/engine.go:167-181` | 残留 orphan 目录与 DB row；重试可能命中唯一索引冲突 | 包 transaction 或实现 compensating delete；至少在 doc 标注"非原子"并补 cleanup CLI |
| **TPM-P1-09** | mcp | ~~`probe.evaluateHTTP` 不处理 OAuth~~ ✅ 2026-05-26：probe 401/403 → `auth_required` 状态；完整 OAuth 探活待 D-M2 | `internal/mcp/probe/eval.go:81-85` | Admin 健康面板对 OAuth MCP 持续告警，与运行时实际可连接矛盾；alert 噪音 | （a）为 oauth2 类型 transport 注入 token 解析接口（保持 mcp 包不 import agent），（b）或明确文档"探活仅校验网络连通" |
| **TPM-P1-10** | mcp | ~~Transport alias 4 处分裂~~ ✅ 2026-05-28：Transport 类型化 + UnmarshalJSON 自动 normalize；probe/mcpobserve 统一 | `internal/mcp/config/config.go:83-95` · `internal/mcp/probe/eval.go:30-37` · `internal/tools/mcpobserve/observe.go:96` | DB 里存 `streamable` 的 server：runtime 正常运行但 probe 永远 `transport 必须是 ...` 报错 | 全链统一调 `config.NormalizeTransport`；运行时装配前先 normalize；probe switch 改为 normalize 后比较 |
| **TPM-P1-11** | mcp | ~~probe SSRF redirect 绕过~~ ✅ 2026-05-26：`outboundguard.NewClient` 已内置 CheckRedirect | `internal/mcp/probe/eval.go:76` | 攻击者构造公网 URL 301 → `127.0.0.1`，SSRF | `client.CheckRedirect = func(req *http.Request, via []*http.Request) error { return validatePublicHost(req.URL.Hostname()) }` |
| **TPM-P1-12** | mcp | ~~ToTRPCConnectionConfig 生产未调用~~ ✅ 2026-05-28：ToTRPCConnectionConfig 已删除；MCPServerConfig.ToConnectionConfig 统一映射入口 | `internal/mcp/config/config.go` 全函数 · `internal/tools/toolset.go::buildMCPToolSet` | "标准" mapping 与"实际" mapping 双 source of truth，新增字段一处加一处忘 | 将 `ToTRPCConnectionConfig` 升级为唯一入口（补 `Env`/auth/reconnect），runtime 改调；或删除该函数避免假权威 |

---

## 5. P2 问题清单（下一迭代）

| ID | 子系统 | 问题 | 建议 |
|----|-------|------|------|
| **TPM-P2-01** | tools | ~~`skillruntime/filter.go::allowedSlugs` 在 `ResolveSkillSlugs` 出错时 **fail open** 到所有已发布 skill~~ ✅ 2026-05-28：改为 fail-closed | 改 fail closed（空集 + log + metric） |
| **TPM-P2-02** | tools | ~~`toolset.go::Assemble` OpenAPI loader 错误被 `continue` 静默吞掉~~ ✅ 2026-05-28：改为 SysLogWarn | 收集错误返回，至少 log，避免 misconfig 静默 |
| **TPM-P2-03** | tools | ~~`claudefetch` 注册表条目永远返回 `fmt.Errorf("not yet implemented")`~~ ✅ 2026-05-28：对齐 geminifetch 返回 (nil, nil) | 对齐 `geminifetch` 模式返回 `(nil, nil)` 或从 registry 删除 |
| **TPM-P2-04** | tools | ~~三处 `configString` helper 重复~~ ✅ 2026-05-28：统一到 tools 包 | 抽 `internal/tools/keys` 单一 mapping |
| **TPM-P2-05** | tools | ~~`skillruntime/filter.go::cache` 用 `sync.Map` 按 invocation ID 累积无 eviction~~ ✅ 2026-05-28：加 512 条上限 | LRU 或 invocation 结束清理 |
| **TPM-P2-06** | tools | ~~`kanban/bridge.go::globalBridge` 包级可变全局变量无锁~~ ✅ 2026-05-28：已重构为 Wire DI 注入 | 优先 context 注入；保留全局则加 `sync.Once` |
| **TPM-P2-07** | tools | ~~mcpobserve 元数据写入 `context.Background()` 丢 trace~~ ✅ 2026-05-28：改为 `context.WithoutCancel(ctx)` | `context.WithoutCancel(ctx)` 或保留 timeout |
| **TPM-P2-08** | tools | ~~`serpapi.go` API key 走 query string 进 access log~~ ✅ 2026-05-28：SerpAPI 不支持 header 鉴权，已加 redactedURL + 注释警示 | header 鉴权（若 plan 支持） |
| **TPM-P2-09** | plugin | ~~`cost_guard.admin_bypass` schema 字段在 `cost_guard.go:41` 默认置 true 但**从未读取**~~ ✅ 2026-05-28：字段与 schema 已删除 | 实现 admin bypass 或从 schema 删除 |
| **TPM-P2-10** | plugin | ~~`permission_guard.confirm_tools` 字段 schema 有但**未实现**；`role_rules` 同样~~ ✅ 2026-05-28：confirm_tools/role_rules 字段与 schema 已删除 | 实现或从 schema 删除 |
| **TPM-P2-11** | plugin | ~~`audit_log.beforeModel.summarizeMessages` 不走 `maybeRedact`，模型 request 可泄敏~~ ✅ 2026-05-28：summarizeMessages 加 maybeRedact | 加 redact；与 `audit_log.afterModel` 对齐 |
| **TPM-P2-12** | plugin | ~~`skill_usage_tracker` 记录原始 tool args 无 redact~~ ✅ 2026-05-28：input/output preview 加 redactText | 走 `redactText` |
| **TPM-P2-13** | plugin | ~~`cost_guard_budget.AddTokens` 写库失败 `_ =` 静默~~ ✅ 2026-05-28：改为 SysLogWarn | log + metric；本地累加但 cross-process 漂移 |
| **TPM-P2-14** | plugin | ~~`retry_and_reflect` `tracking_scope: "global"` 用 `map[string]int` 无 eviction~~ ✅ 2026-05-28：加 1h TTL eviction | LRU/TTL |
| **TPM-P2-15** | plugin | ~~`Manager.OnEvent` 每事件 `trpcplugin.NewManager(...)`~~ ✅ 2026-05-28：已重构为复用单一 Manager 实例 | scope 缓存 manager 实例 |
| **TPM-P2-16** | plugin | ~~`dispatchHookOnEvent` 与 chain hook 失败策略不一致（一个 propagate block error，一个 resilient swallow）~~ ✅ 2026-05-28：统一为 resilient swallow + block 传播 | 统一 policy 文档 |
| **TPM-P2-17** | skill | ~~importer 内存 job map 不持久 + 无 TTL~~ ✅ 2026-05-28：加 2h TTL 过期清理 + evictExpiredLocked | DB / Redis + 过期清理 |
| **TPM-P2-18** | skill | ~~`inspectSimilarity` 对每个 candidate × 现有 skill LLM 调用，无上限/批处理~~ ✅ 2026-05-28：加 50 次上限 | 加 cap；batch prompt |
| **TPM-P2-19** | skill | ~~`validate.go::ReadSkillDirFiles` 不安全相对路径**静默返回**~~ ✅ 2026-05-28：改为返回错误（与 ZIP 严格性对齐） | 失败 fail；与 ZIP 严格性对齐 |
| **TPM-P2-20** | skill | ~~`skill_import_http.go` Multipart 25MB vs engine 20MB **限制不一致**~~ ✅ 2026-05-28：已统一为 `MaxZipBytes` 常量 | 统一常量 |
| **TPM-P2-21** | skill | ~~`DBRepositoryAdapter.reload` 失败**保留陈旧缓存无 log**~~ ✅ 2026-05-28：reload/loadBody 失败加 SysLogWarn | log + metric + stale gauge |
| **TPM-P2-22** | skill | ~~`watch/runner.go::childWatches` 并发写无锁~~ ✅ 2026-05-28：加 mu 保护 | mutex 或 single-writer goroutine |
| **TPM-P2-23** | skill | `SkillPermissions` 全 `true` 硬编码 | 接入 RBAC |
| **TPM-P2-24** | skill | 无版本回滚 API（即使数据层有 `skill_version` 表）| 暴露 rollback RPC |
| **TPM-P2-25** | mcp | ~~`alert.MarkHealthAlertEmitted` 失败 `_ =` 吞掉~~ ✅ 2026-05-28：改为 log + metric | log + metric |
| **TPM-P2-26** | mcp | ~~mcpobserve 每次 reconnect O(n) 全表扫 server 找 key~~ ✅ 2026-05-28：改 `GetMCPServerByKey` | 改 `GetByKey` |
| **TPM-P2-27** | mcp | ~~OAuth refresh 失败 fallback 用陈旧 `access_token`，掩盖过期~~ ✅ 2026-05-28：强失败不再 fallback | 强失败 + 持久化 rotated refresh token |
| **TPM-P2-28** | mcp | ~~metadata row 并发 health + reconnect 写为 last-write-wins~~ ✅ 2026-05-28：`UpdateMCPServerMetadata` 只写 metadata+status 字段 | 乐观锁或字段级 merge |
| **TPM-P2-29** | mcp | ~~`probe/health/alert` 三个包零测试~~ ✅ 2026-05-28：probe + alert + config 测试已补全 | 至少 stdio/HTTP probe + ctx cancel + alert debounce |
| **TPM-P2-30** | mcp | ~~health.probeAll 每 server `safego.Go` 无 worker pool 上限~~ ✅ 2026-05-28：semaphore bounded concurrency（max=8） | bounded concurrency |

---

## 6. P3 问题清单（优化）

| ID | 子系统 | 问题 |
|----|-------|------|
| TPM-P3-01 | tools | `workspace_exec` 在 registry 是 no-op factory，实际由 `WithCodeExecutor` 挂载 |
| TPM-P3-02 | tools | `preview.RedactAndTruncate` 按 byte 截断可能切多字节字符 |
| TPM-P3-03 | tools | `cache.ResultCache` 每次满都 evictOne，O(n) |
| TPM-P3-04 | tools | `skillrouter/taxonomy.go` 三叶硬编码 |
| TPM-P3-05 | tools | `doc.go` 提到与 `internal/data/builtin_tools_seed.go` 手工同步，建议 CI 校验 |
| TPM-P3-06 | plugin | 优先级 magic numbers（10/50/200/300/500ms/8s/12000/`÷4`）散落 |
| TPM-P3-07 | plugin | `confirmation_guard` 与 Chain ConfirmGate 概念重叠，admin 文档需注 |
| TPM-P3-08 | plugin | 无 plugin version 字段，兼容性靠 key 字符串 |
| TPM-P3-09 | skill | `slugify` 空输入 fallback `skill-0`（`len("")=0`）始终同名 |
| TPM-P3-10 | skill | `importBlockMessages` 用 `"?"` 拼接（疑似 newline 占位损坏）|
| TPM-P3-11 | skill | importer/validator 多处 `"?????..."` 占位中文未完成 |
| TPM-P3-12 | mcp | ~~大量 magic numbers~~ ✅ 2026-05-28：集中到 `internal/mcp/defaults.go` |
| TPM-P3-13 | mcp | ~~`classify.IsMCPToolInvocation` 仅前缀启发式~~ ✅ 2026-05-28：评估后保留，`mcp_`+`__` 模式与框架一致 |

---

## 7. 业务逻辑分析（深度）

### 7.1 工具装配（tools）

`internal/agent/tool_assembly.go` 是入口，调用栈：

```
ChatOrchestrator.BuildRunOptions
  → buildToolAssembly (effective keys → ToolsetConfig)
  → trpc.BuildToolsets (flag map → 自定义 tool 注入)
  → tools.Assemble (Registry factory + Config override + alias)
  → trpc-agent-go/tool/* (framework 真相源)
```

**正确性优点**：
- `PruneUnconfiguredToolFlags` 凡是 credential 缺失（Google CSE / Tavily 等）→ 跳过而非整 Agent 失败。
- `webresearch.Tool` 实现 partial result + per-result fetch timeout，缺失 enrichment 仍返回 search 结果。
- `serviceawaitreply` 与 framework `await_user_reply` 双路径，未配 `AwaitHook` 时降级到 framework，测试友好。

**正确性风险**：见 §4 P1-01 / P1-02 与 §5 P2-01..P2-08。

### 7.2 Plugin Chain 编排（plugin）

四层编排（`orchestration.go` 已文档化）：

| 层 | 触发顺序 | 来源 |
|----|---------|------|
| Product 固定 callbacks | 3..50 (args=3, cache=4, timing=5, confirm=10, recorder=50) | `internal/agent/*` |
| Chain-mirrored plugins | 200 + sort_order | `plugin_chain_mirror.go` |
| User Hooks | 300 + sort_order | `hook_callbacks.go` |
| Runner WithPlugins | DB sort_order ASC | `runtime.go::RunnerPluginsForAgent` |

**正确性优点**：
- `orchestration_policy.go` 显式规定哪些 plugin **必须**留在 Runner（exclusive）、哪些**可选**进 Chain（`skill_usage_tracker` 白名单）；防止双重触发，有测试覆盖。
- `ModelSelector` 与 `BeforeModel` plugin 在 router/cost_guard 上分工：plugin 仅 telemetry，selector 才是真路由（避免与框架已有的 selector 重复发挥）。
- `cost_guard_budget.go` 用 atomic UPSERT 在数据库层做跨进程一致性，本地用 mutex tracker 做高并发热路径。

**正确性风险**：
1. §4 P1-03 cost_guard 双重 block — 现状 `costGuardShouldBlock` 在有 fallback 时返回 `(false, "")` 让通过，但同函数下一段 `TryConsume` 失败直接 hard block，逻辑分裂。
2. §4 P1-04 output_policy.onEvent 流式漏检。
3. §4 P1-05 整链无 panic recover，是稳定性最大隐患。

### 7.3 Skill 导入与运行时（skill）

```
ZIP / multipart → SkillService.StartImport
                → engine.inspectSkillZip
                  ├─ size / extension / 高危文件验证
                  ├─ similarity (LLM × 现有)
                  └─ jobState in-memory map (无 TTL)
                → engine.ApplyImport (per-decision loop)
                  └─ createImportedSkill (写盘 → DB row)  ⚠️ 无事务

watch.Runner ← fsnotify + 5min reconcile
              → syncSlug → UpsertSkillFromDisk (data 层)
                  ├─ 已发布 → RevertedToDraft（安全闸）
                  └─ 草稿 → 原地覆盖

trpc.DBRepositoryAdapter  ← Agent 装配时
              → ListEnabledPublishedCandidates
              → lazy loadBody (按需读全文)
              ⚠️ Summary.Name = c.Name (display name 而非 slug)

tools/skillruntime.AgentVisibilityFilter
              → ResolveSkillSlugs (Layer A allow/deny + Layer B intent)
              → cache sync.Map
              ⚠️ Layer B 出错 → fail open
              ⚠️ 比对 summary.Name (display name) vs slug allow-list → 全部不命中
```

**正确性优点**：
- `RevertedToDraft` 保护已发布 skill 不被磁盘静默覆盖（人工再确认）。
- watch debounce 2s + reconcile 5min（环境变量可关）合理平衡延迟与负载。
- ZIP 黑名单（`.exe / .bat / .cmd / .ps1 / .dll / .so / .dylib`）+ 高风险显式 approve 流程。

**正确性风险**：
- §4 P1-06 / P1-07 / P1-08 三个 P1（filter 错位 / zipslip 不彻底 / 部分写入无回滚）— 都属业务逻辑关键路径。
- 多处 `"????"` / `"?"` 占位字符串是 i18n 未完成痕迹，影响用户体验。

### 7.4 MCP 平台 vs 运行时（mcp）

```
                平台层 internal/mcp/                运行时 internal/tools/ + internal/agent/
                  │                                  │
config_json ─→ config.ParseServerConfigJSON  ─→ tool_assembly.go::resolveMCPServers
                  │                                  ├─ applyMCPAuthHeaders (OAuth)
                  │                                  └─ toolset.go::buildMCPToolSet
                  │                                       ├─ trpcmcp.NewMCPToolSet
                  │                                       └─ WithSessionReconnect(N)
                  │                                  
probe.Evaluate  ⟶ TestResult (admin 测试 / health)    mcpobserve.ObserverForServer
                                                       ├─ Prometheus
                                                       ├─ event.Bus
                                                       └─ RecordReconnectMetadata
                  │                                  
metadata.Apply* ⟵ alert.MaybeEmitAfterHealth        
                  │                                  
health.Runner   ⟶ TestMCPServer × N (5min ticker)   
```

**正确性优点**：
- 平台层与运行时彻底分离：`internal/mcp` 不 import `internal/agent` / `pkg/trpc-agent-go`（除 `ToTRPCConnectionConfig` 引用 framework 类型且未启用）。
- `health.Runner.probeAll` 用 `TryLock` 防止 probe 周期堆积。
- `alert.ShouldEmitHealthAlert` 做了"持续 ≥ N min" + "距上次告警 ≥ N min" 的双 debounce。
- SSRF DNS 解析后逐 IP 校验 loopback / private / link-local。

**正确性风险**：
- §4 P1-09..P1-12 四个 P1 — probe / runtime / observer 协议字段在 4 处分裂，是 MCP 子系统最大债务。

---

## 8. 代码质量评估

### 8.1 复杂度热点（>200 行 / 单函数 >80 行）

| 文件 | 行数 | 主热函数 | 评级 |
|------|------|----------|------|
| `tools/toolset.go` | 535 | `Assemble()` ~155 | ⚠️ Registry + Assemble + MCP build 三职责同居 |
| `tools/testexec/execute.go` | 249 | `Execute()` ~140 | OK |
| `tools/skillruntime/resolve.go` | 238 | `ResolveSkillSlugs` 多层 | OK |
| `tools/kanban/tools.go` | 236 | 9 个 handler | OK |
| `plugin/trpc/runtime.go` | 262 | `Apply()` | ⚠️ Runtime God-struct（plugins + budgets + notifier + bus + catalog） |
| `plugin/trpc/hook_callbacks.go` | 247 | `executeHookAction` ~90 | OK |
| `plugin/trpc/audit.go` | 228 | 7 lifecycle handlers | OK |
| `skill/importer/engine.go` | ~420 | `inspectSkillZip` + `ApplyImport` | ⚠️ 最大单文件 |
| `skill/watch/runner.go` | ~393 | `Start` + `syncSlug` | OK |
| `mcp/*` | 全 <130 | — | ✅ 最优 |

### 8.2 命名与一致性

- **优点**：
  - tools 子包按领域命名（`webresearch / kanban / knowledge / skillruntime / hostexecnorm`），目录结构反映 domain。
  - plugin 9 个内置统一 `<Name>Plugin` struct + `Register(r *trpcplugin.Registry)`，模板化。
  - mcp 5 个子包各自单一职责，命名直接（`probe / health / alert / classify / metadata`）。
- **瑕疵**：
  - tools `runtime_alias.go` vs biz `tool_policy_keys.go` 漂移（见 P1-01）。
  - skill `Summary.Name` 在 FS adapter 是 slug，DB adapter 是 display name —— 同字段语义双义（见 P1-06）。
  - mcp transport 字符串在 4 处不一致（见 P1-10）。

### 8.3 死代码 / 死配置

| 符号 | 文件 | 性质 |
|------|------|------|
| `mcp/config.ToTRPCConnectionConfig` | `internal/mcp/config/config.go` | ~~生产 0 引用~~ ✅ 2026-05-28：已删除，runtime 统一走 `toolset.go::MCPServerConfig.ToConnectionConfig` |
| `cost_guard.admin_bypass` | `internal/plugin/trpc/cost_guard.go:18,41` | ~~schema 暴露 + 默认 true，代码从不读~~ ✅ 2026-05-28：字段与 schema 已删除 |
| `permission_guard.confirm_tools` | `internal/plugin/trpc/permission_guard.go` | ~~解析后未使用~~ ✅ 2026-05-28：字段与 schema 已删除 |
| `permission_guard.role_rules` | JSON schema | ~~仅 schema，无实现~~ ✅ 2026-05-28：schema 已删除 |
| `claudefetch` registry entry | `internal/tools/toolset.go:73-78` | ~~factory 永远 error~~ ✅ 2026-05-28：改为 `(nil, nil)` 对齐 geminifetch |
| `skillruntime/resolve.go` 死函数 | `internal/tools/skillruntime/resolve.go` | ✅ 2026-05-28 Review2-5：删除 applyLayerA/filterByAllTags/filterByIntentPaths/scoreCandidates（均有 WithReasons 版本替代） |
| `skillruntime/filter.go` normalizeSlugSlice | `internal/tools/skillruntime/filter.go` | ✅ 2026-05-28 Review2-4：删除死代码函数 |
| `tools.workspace_exec` factory | registry | no-op；实际由 `WithCodeExecutor` 挂载 |

### 8.4 错误处理风格

- **统一模式**：`logger.Warn / Info` + 多用 `event.CtxFlowLog*`；少数关键路径 return error 终结。
- **风险**：
  - plugin / skill / mcp 多处 `_ = repo.Xxx(ctx, ...)` 静默忽略（参见 P2-13 / P2-21 / P2-25）。
  - ~~**整个 plugin / tools / skill / mcp 子系统都无 `recover()`**——hook panic 直接传播到框架。~~ ✅ 2026-05-28：P1-05 hook_resilience 已添加 recoverHookPanic + wrapResilient
  - skill / mcp 多处 `_ = json.Unmarshal(...)` 静默接受 partial config。
  - ~~`skillruntime/filter.go` 使用 `slog.Warn` 违反红线 #10~~ ✅ 2026-05-28：已替换为 `event.SysLogWarn`

### 8.5 并发安全

| 位置 | 评估 |
|------|------|
| `tools.Registry` | `sync.Once` 懒初始化，安全 |
| `tools/cache.ResultCache` | ✅ RWMutex 配对正确；2026-05-28 Review2-3：TOCTOU 竞态已修复（过期条目升级写锁后重新检查） |
| `tools/skillruntime.filterCache` | ✅ 2026-05-28：mutex + atomic.Int64，512 条上限自动清理，替代原 sync.Map |
| `tools/mcpobserve` 包级 RWMutex | wire-once 后只读，OK；但隐式全局耦合 |
| `tools/kanban.globalBridge` | ✅ 2026-05-28：已重构为 Wire DI 注入 |
| `plugin/trpc/runtime.go` plugins | RWMutex 读多写少 OK |
| `plugin/trpc/cost_guard_budget.go` | per-tracker mutex + DB UPSERT，cross-process 一致性靠 DB 原子；本地 fail-silent 是问题（P2-13） |
| `skill/watch/runner.go::childWatches` | ✅ 2026-05-28：加 mu 保护（P2-22） |
| `mcp/health/runner.go::TryLock` | 正确（防 pile-up） |
| `mcp/health.probeAll → safego.Go` | ✅ 2026-05-28：semaphore bounded concurrency（max=8）（P2-30） |

### 8.6 测试质量

| 子系统 | 单元测试 | 边界 | E2E | 评级 |
|--------|---------|------|-----|------|
| tools | toolset_assemble / workspace / prune / webresearch / hostexecnorm / cache / preview | ★★★★ | webresearch integration ★★ | 总体良好；缺 kanban / knowledge |
| plugin | orchestration_policy / cost_guard scope / hook_notify SSRF / chain mirror / builtin / ~~cost_guard double-block~~ ✅ / ~~output_policy streaming~~ ✅ / ~~hook_resilience~~ ✅ | ★★★★ | 几乎无端到端 turn 测试 | ✅ 2026-05-28：P1-03/04/05 回归测试已补全 |
| skill | 仅 slug 化 2 个 trivial 测 | ★ | 全无 | 测试覆盖**最弱**子系统 |
| mcp | config / classify / metadata | ★★ | 全无 | probe / health / alert 三个核心包**零测试** |

---

## 9. 性能与资源效率

| 项 | 现状 | 评级 |
|----|------|------|
| 工具装配 | Registry 一次性初始化；Assemble 按需挂载；OpenAPI loader 失败 `continue` | OK，但 OpenAPI 静默是 robust 度问题 |
| webresearch | search + 5 个并发 fetch；URL fetch 走 framework `httpfetch` 批量；4 MiB / 12000 byte 限制 | ★★★★ |
| plugin chain | 每个 turn 装配 chain；OnEvent **每事件 `NewManager`**（P2-15） | 流式热路径有改进空间 |
| cost_guard `estimateRequestTokens` | 遍历完整 session 历史 + `EventMu.RLock` | O(events) per call，长 session 注意 |
| skill watch | debounce 2s + reconcile 5min（可关）；ZIP 全量 in-memory（≤20MB） | OK；~~similarity LLM 调用无 cap（P2-18）~~ ✅ 2026-05-28：已加 50 次上限 |
| mcp health | 5min ticker × server；`safego.Go` 无 worker pool | 中等规模 OK；fleet 大时建议加 bounded |
| mcp reconnect | runtime SSE/streamable=3，stdio=0 | 合理 |

---

## 10. 安全审查

| 类别 | 风险点 | 缓解 |
|------|--------|------|
| **SSRF** | `mcp.probe.evaluateHTTP` 缺 redirect 校验（P1-11） | 必修 |
| **SSRF** | plugin `hook_notify` 已有 `webhookurl.ValidateNotifyURL` | ✅ 已防 |
| **SSRF** | tools `cli_admin/pkg_install_from_url.go` 缺 URL 校验 | ✅ 2026-05-28 Review2-2：已加 validateRepoURL + isPrivateIP |
| **Path traversal** | skill ZIP zipslip 不完全（P1-07） | 必修 |
| **PII 泄露** | plugin `audit_log.beforeModel` 不 redact request（P2-11） | 修复 |
| **PII 泄露** | plugin `skill_usage_tracker` 原始 args 日志（P2-12） | 修复 |
| **OAuth** | `mcp_oauth.go` refresh 失败回退陈旧 token（P2-27） | 修复 |
| **凭证** | ~~tools `serpapi.go` API key in query string（P2-08）~~ ✅ 2026-05-28：已加 redactedURL 脱敏 |
| **凭证** | tools `cli_admin/registry.go` Deps.String() 泄露 APIToken | ✅ 2026-05-28 Review2-7：Deps.String() 遮蔽 APIToken 为 "***" |
| **RBAC** | skill 全 `true` 硬编码（P2-23） | 规划 |
| **凭证存储** | mcp `config_json` 含 client_secret / refresh_token（明文 in row） | 需要 KMS / 字段加密（超出本 review 范围） |

---

## 11. 兼容性与依赖

- **框架版本耦合**：tools / skill / mcp 子系统全部经 `pkg/trpc-agent-go/tool|skill|mcp` 的 alias type 或直接 import；框架升级直接影响所有四个子系统。建议：
  - tools 用 alias 隔离（已做）。
  - plugin 直接实现 `trpcplugin.Plugin` 接口，升级风险较高。
  - skill `DBRepositoryAdapter` / `FSRepositoryAdapter` 实现 `trpcskill.Repository`。
  - mcp `ToTRPCConnectionConfig` 是唯一一处显式依赖 framework 类型的 mapper（且未启用）。
- **协议字段漂移**：MCP transport（P1-10）是当前最严重的内部兼容问题。
- **数据库 schema 兼容**：plugin `config_schema_json` 与代码读取不一致（P2-09/10）是另一种"自洽性破损"。
- **第三方依赖**：webresearch（Tavily/SerpAPI）、Google CSE、Wikipedia 等都有 prune 降级，单 provider 失效不影响 Agent 整体构建。

---

## 12. 可维护性总结

| 维度 | 评价 |
|------|------|
| 模块化 | ★★★★ 四个子系统、各自子包都遵循单一职责（tools 子包按领域、plugin 按 plugin 名、skill 按 4 层、mcp 按 5 子包） |
| 注释 | tools `doc.go` 与 plugin `orchestration.go` 顶部说明优秀；skill 多处占位字符串需补；mcp 注释偏少但函数小 |
| 变更影响 | tools / plugin 的 wire 收敛在 `cmd/admin/wire.go` 和 `internal/service`，影响半径可控；skill 多处共享 root 解析未抽常量；mcp 由于 4 处 transport 重复，加新 transport 改 4 处 |
| 向后兼容 | 无 plugin version 字段；skill 无版本回滚；mcp transport 别名漂移 — 三处共性是缺乏"协议契约定义" |
| 技术债 | TPM-P1-* 12 条 P1 + 30 条 P2；偿还计划建议优先级见 §15 |

---

## 13. 业务规则与领域模型审查

| 子系统 | 领域模型贴合 | 复杂业务规则正确性 |
|--------|-------------|------------------|
| tools | "Catalog × Profile × Agent override × Runtime alias × Confirm policy" 五层模型；effective key resolution 走 biz `computePolicyAllowedSet/DenySet`，规则集中 | ✅ 总体正确；alias 漂移（P1-01）是规则一致性问题 |
| plugin | "DB row × DefaultConfig × Plugin scope × Callback point × Orchestration policy" 五层；ConfirmGate 与 plugin `permission_guard` 边界清晰 | ⚠️ cost_guard fallback vs hard block 规则（P1-03）需澄清产品意图 |
| skill | "Layer A 白名单 × Layer B 意图 × FS source × DB source × Published vs Draft" 五层；`RevertedToDraft` 是安全闸的核心规则 | ⚠️ FS / DB 双 source 下 Summary.Name 语义不统一（P1-06） |
| mcp | "Platform config × Auth × Transport × Health × Alert × Reconnect" 六层；`MCPServerUsecase` 是单一事实 | ⚠️ Transport alias 漂移导致同一 server 在 probe / runtime 表现不一致（P1-10） |

---

## 14. 回归风险点（已修复但缺测试覆盖）

| 修复主题 | 风险 | 当前测试覆盖 |
|---------|------|-------------|
| skill `RevertedToDraft` 安全闸 | 已发布 skill 被磁盘静默覆盖 | 缺集成测试 |
| plugin `confirmation_guard` Telemetry-only 与 Chain ConfirmGate 分工 | 双重触发 | `plugin_chain_mirror_test.go` ✅ |
| tools `PruneUnconfiguredToolFlags` | credential 缺失整 Agent 失败 | `trpc/toolset_prune_test.go` ✅ |
| mcp `ShouldEmitHealthAlert` debounce | 告警风暴 | `metadata_test.go` ✅ |
| mcp `health.Runner.TryLock` 防 pile-up | 间隔短于 probe 时长堆积 | 无 |
| skill watch debounce + reconcile | 漏写 / 重复写 | 无 |

---

## 15. 优先级修复路线（推荐迭代计划）

### Sprint N（本迭代）— P1 关键路径

1. **mcp transport 统一**（P1-10/P1-12）：抽出 `NormalizeTransport` 全链调用 + 启用 `ToTRPCConnectionConfig` 或删之。
2. **plugin cost_guard 双重 block 修复**（P1-03）：fallback 路径跳过 TryConsume hard block。
3. **plugin output_policy on_event 兑现 block_on_violation**（P1-04）。
4. **plugin chain panic recover**（P1-05）：`defer recover()` 包裹 hook 与 chain wrapper。
5. **skill filter slug 一致性**（P1-06）：`Summary.Name = slug` 或 filter 改 key 比对。
6. **skill ZIP zipslip 完全防护**（P1-07）：`filepath.Rel` 容器校验。
7. **tools web_search alias 对齐**（P1-01）。

### Sprint N+1 — P1 收尾 + P2 安全/正确性

8. mcp probe SSRF redirect 拦截（P1-11）。
9. mcp probe OAuth 模式（P1-09）。
10. skill ApplyImport 事务 / compensating（P1-08）。
11. tools alias `(nil, nil)` 改返 error（P1-02）。
12. plugin redaction 覆盖 audit / skill_usage_tracker（P2-11/12）。
13. plugin 死配置清理（P2-09/10）。

### Sprint N+2 — 测试基线 + P2/P3 维护性

14. 补 mcp probe/health/alert 测试（P2-29）。
15. 补 skill zipslip / partial apply / watch race / filter 回归测试。
16. 补 plugin cost_guard double-block 回归测试。
17. 统一 tools 三处 `configString` / catalog-key mapping（P2-04）。
18. magic number 集中（P3-06/12）。

---

## 16. 业务逻辑重设计建议（"如果让我重新设计"）

> 本节回答："**除了打补丁，业务逻辑层面有哪些值得重新思考的设计点？**"
> 每个建议都给出：**动机 / 当前问题 / 目标设计 / 迁移路径 / 工作量**。
> 标注 **\[Greenfield]** 表示推荐重写；**\[Refactor]** 表示可在现有代码上演进；**\[Spike]** 表示先做一个 PoC 验证。

### 16.0 跨子系统通用设计模式（三条主线）

#### 模式 A：**契约先行（Schema-as-Code）**

**动机**：当前 plugin / mcp / tools 三处都出现"schema 字段 vs 代码读取"漂移（`admin_bypass` 写 schema 不读，`web_search` alias 双向不一致，transport 字段 4 处不同步）。本质是"**没有单一契约文件**"——schema 在 JSON 字符串里、解析在 Go struct 里、阅读在另一个 Go 文件里。

**目标设计**：

```go
// pkg/contract/plugin/cost_guard.go — 单一真相源
package costguard

type Config struct {
    DailyTokenBudget int      `json:"daily_token_budget" validate:"min=0" desc:"日 token 上限，0=不限"`
    MaxPromptTokens  int      `json:"max_prompt_tokens"  validate:"min=0"`
    BlockedModels    []string `json:"blocked_models"`
    FallbackModel    string   `json:"fallback_model"`
    AdminBypass      bool     `json:"admin_bypass"       deprecated:"true"` // ❌ 编译期标记死字段
}

// JSON schema 由 reflect 自动生成（github.com/invopop/jsonschema 或自写 reflector）
func Schema() *jsonschema.Schema { return jsonschema.Reflect(&Config{}) }
```

- **registry seed** 不再手写 `config_schema_json`，改用 `costguard.Schema()` 序列化。
- 死字段（`AdminBypass`）打 `deprecated:"true"`，CI 在 schema diff 时报警。
- biz / runtime 都 import 同一 `pkg/contract/*`，杜绝双 source。

**迁移路径**：
1. 先抽出 `pkg/contract/plugin/` `pkg/contract/mcp/` `pkg/contract/tools/` 三个包，逐 plugin 迁。
2. 老 schema JSON 保留为 fallback，CI 对比"reflect 生成的" vs "DB 存的"，差异告警。
3. 每个 plugin 迁完后从 `registry.go` 删掉手写 schema。

**工作量**：**M**（每子系统 2-3 天）。

---

#### 模式 B：**单向数据流（State Machine + Event Sourcing 轻量版）**

**动机**：当前 skill 导入、mcp 健康状态、plugin cost budget 都是"`metadata_json` 字段 + 多处直接 mutate"的混合状态，导致：
- skill `ApplyImport` 写盘 + 写库分两步无回滚；
- mcp `health` 与 `reconnect observer` 并发写 `metadata_json` last-write-wins；
- plugin budget DB 写失败 `_ =` 后本地 / 远端漂移。

**目标设计**：把"状态"建模为**显式状态机** + **事件追加**：

```go
// internal/biz/skill/state.go
type ImportEvent interface {
    eventTag()
    AppliedAt() time.Time
}

type ImportStarted    struct{ JobID string; ZipSHA string; At time.Time }
type ImportValidated  struct{ JobID string; CandidateIDs []string; At time.Time }
type ImportApplied    struct{ JobID string; SkillID, Slug, Dir string; At time.Time }
type ImportRolledBack struct{ JobID string; Reason string; At time.Time }

// 状态由 fold(events) 得到，UI / 监控直接订阅事件流
func Fold(events []ImportEvent) ImportJob { /* ... */ }
```

- **写盘 + 写库 + 索引** 都通过 `repository.AppendEvent(...)` 触发，失败自动产生 `ImportRolledBack` 补偿事件。
- mcp `metadata` 同理：`HealthProbed{ok}` `ReconnectAttempted` `AlertEmitted` 三类事件，"当前状态"是 fold 结果。
- 好处：天然审计、并发安全（append-only）、回放调试、UI 可订阅 stream。

**迁移路径**：先在 skill 子系统试点（变更最小），稳定后推广到 mcp metadata、plugin audit。

**工作量**：**L**（每子系统 1-2 周）。建议 **\[Spike]** 先跑一个 skill import 的 PoC。

---

#### 模式 C：**Policy Engine 抽离（OPA-lite）**

**动机**：现在 "**谁可以调用哪个 tool / plugin / skill**" 散在五个地方：
- tools: `biz/tool/tool_policy_keys.go` + `internal/agent/tool_assembly.go` + Profile/Agent override
- plugin: `permission_guard.deny_tools` + ConfirmGate + orchestration_policy exclusive list
- skill: Layer A allow/deny + Layer B intent + RBAC（未实现）
- mcp: `Enabled` 标志 + Agent 绑定
- cron / channel: 各自有独立 ACL

策略多源 + 各自语言 → 漂移与 fail-open 的温床（P2-01 fail-open，P1-01 alias 漂移皆此类）。

**目标设计**：一个**统一的策略评估器**（不是上 OPA / Cedar，先做轻量内置）：

```go
// internal/policy/engine.go
type Subject struct{ AgentID, UserID, WorkspaceID, ChannelID string; Roles []string }
type Resource struct{ Kind, Key string; Attrs map[string]any }
type Decision struct{ Allow bool; Reason string; Obligations []Obligation } // 例如 "require_confirm"

type Rule struct {
    When   Predicate     // attribute-based 谓词
    Effect Effect        // allow / deny / require_confirm
    Reason string
}
```

- tools / plugin / skill / mcp 都通过 `engine.Evaluate(subj, res, action)` 获得决策。
- 单一 `default = deny` 策略 → 消除 fail-open。
- Reason 链可追溯到具体规则，前端能展示"为何被拒"。

**迁移路径**：
1. **\[Spike]** 先把 `permission_guard.deny_tools` 包成 Rule[]，跑通最简路径。
2. 把 skill Layer A 也接入。
3. 最后吃掉 tools alias / Profile override。

**工作量**：**L**（核心引擎 1 周 + 每子系统接入 3-5 天）。是中长期投资，**短期不必赶**。

---

### 16.1 tools 子系统业务逻辑优化

#### TPM-D-T1：**Tool 名称解析层抽出为单一服务** \[Refactor]

**当前**：tool 名字解析散在四处——
- LLM 调用名 → catalog tool_key：`biz.NormalizeToolPolicyKey`
- Catalog → runtime declaration name：`tools.ApplyRuntimeNameAliases`
- Confirm policy 别名：`tools/trpc/confirmation.go`
- Effective key resolution：`computePolicyAllowedSet/DenySet`

→ 每加一个新工具，要同时改 2-4 个 map（P1-01 漂移直接来源）。

**目标设计**：

```go
// internal/biz/tool/resolver.go
type Resolver struct {
    aliases map[string]string // unified: any-name → canonical tool_key
}

// 唯一入口：API/LLM 任意名字 → catalog 主键
func (r *Resolver) Canonical(name string) string

// 唯一入口：catalog tool_key → runtime declaration name（多数 = key，少数有 alias）
func (r *Resolver) RuntimeName(key string) string

// catalog tool_key → declaration aliases（运行时为 LLM 注册多个名字）
func (r *Resolver) RuntimeAliases(key string) []string

// 启动时校验：alias DAG 无环、所有 alias 终点都在 registry
func (r *Resolver) Validate(registry RegistryView) error
```

- **`Validate()` 在 wire 阶段调一次**，CI 失败即阻断启动 —— 杜绝运行时漂移。
- biz 提供单一 alias 表（YAML 文件 + Go embed），runtime 不再有第二张表。
- `runtime_alias.go` 删除，并把现有的 `RuntimeToolNameAliases` 合入 `biz/tool/aliases.yaml`，区分 "policy-only" / "runtime-only" / "both" 三种 scope。

**迁移路径**：先建 Resolver、把现有两张 map 合表 + 一致性测试；再逐处 caller 改调 Resolver。

**工作量**：**S**（2-3 天）。**强烈推荐**——这是修 P1-01 的根本方法。

---

#### TPM-D-T2：**Effective Tools 计算改为"Effective Plan"，显式 fail-closed** \[Refactor]

**当前**：`internal/agent/tool_assembly.go` 直接拿 `effectiveKeys []string` 然后映射成 flag → `Assemble`。
- 中间 prune / alias / runtime config 各自独立，**没有任何阶段记录"为什么这个 tool 没出现"**。
- skillruntime fail-open（P2-01）就是这种"中间状态不可见"的副作用。

**目标设计**：

```go
type ToolPlanEntry struct {
    Key             string       // catalog tool_key
    Source          string       // "policy_allow" / "profile_default" / "agent_override" / "skill_layerA"
    Status          PlanStatus   // included / pruned_no_credential / pruned_skill_filter / pruned_alias_conflict
    DeniedBy        string       // 如有，记录拒绝来源
    ResolvedAliases []string
    Reason          string
}

type ToolPlan struct {
    AgentID string
    Entries []ToolPlanEntry
    Hash    string // 用于灰度与 cache
}
```

- `BuildEffectivePlan(ctx, agent) (*ToolPlan, error)` 是新的入口；`Assemble` 接受 Plan 而非 flag map。
- 每个 entry 携带"为什么"，前端 `Agent → Tools` 调试面板可直接展示。
- **fail-closed**：任何 ResolveSkillSlugs 错误产生 `Status: pruned_skill_filter, Reason: ErrXxx`，**不再静默放过所有 skill**。
- Plan 可被 cache（按 hash），多 turn 复用降低装配开销。

**迁移路径**：先生成 Plan 与现有逻辑 parity 比对（log "would-be plan"）；稳定后切流。

**工作量**：**M**（1 周）。**推荐**——同时解 P1-01/02 + P2-01/02。

---

#### TPM-D-T3：**Confirm Policy 与 Permission Guard 合一** \[Refactor]

**当前**：tool 调用"是否需要人工确认"的逻辑分三处——
- tools `trpc/confirmation.go`（声明 patcher）
- plugin `confirmation_guard.go`（telemetry）
- plugin `permission_guard.deny_tools`（hard block）
- `internal/agent` ConfirmGate（实际 block + emit await_user_reply）

四处 + 跨包；产品意图实际是单一"call-policy"。

**目标设计**：在 biz 暴露 `ToolCallPolicy` 单接口：

```go
type CallPolicy struct {
    Action    Action // allow / deny / confirm
    Reason    string
    Confirmer string // 若 confirm: 谁来确认（user / admin / role:reviewer）
    Timeout   time.Duration
}

type CallPolicyProvider interface {
    Resolve(ctx context.Context, agentID, toolKey string, args json.RawMessage) CallPolicy
}
```

- 由 plugin / catalog / agent override 三方共同 contribute rule，单一 `Resolve` 汇总。
- 现有 ConfirmGate / permission_guard / confirmation_guard 全部退化为同一个 provider 的不同 rule source。
- 与 §16.0 模式 C（Policy Engine）天然衔接，先建小型版。

**工作量**：**M-L**（1-2 周）。**中等优先**——能消掉一类 P3 概念重叠（P3-07）。

---

### 16.2 plugin 子系统业务逻辑优化

#### TPM-D-P1：**Cost Guard 改为"Reservation Pattern"** \[Refactor]

**当前问题**（P1-03 的根因）：
- ModelSelector 看预算 → 决定是否换 fallback model（**不消费**）。
- `beforeModel` 估 token → `TryConsume`（**消费**）。
- `afterModel` 拿到真实 usage → **从不调整**。

→ 三处独立，且 ModelSelector 的"避让"动作与 `beforeModel` 的"硬拦"动作互相不感知（同一 turn 内 fallback 路径也可能因为同一日额度耗尽被 hard block）。

**目标设计 — 三阶段预算事务**：

```go
type BudgetReservation struct {
    ID       string
    AgentID  string
    Model    string // 实际将要调用的 model
    Reserved int    // 预占的 token（估算）
    Status   string // reserved / consumed / released
}

type CostGuardEngine interface {
    // 阶段 1：选 model 时调用 → 返回最终 model + 一个预占凭证
    PrepareCall(ctx context.Context, candidates []string, est int) (chosenModel string, res *BudgetReservation, err error)

    // 阶段 2：调用前 commit 预占（幂等）
    Commit(ctx context.Context, resID string) error

    // 阶段 3：调用后用真实 usage 修正
    Reconcile(ctx context.Context, resID string, actualPromptTokens, actualCompletionTokens int) error

    // 失败回滚
    Release(ctx context.Context, resID string) error
}
```

- ModelSelector 调 `PrepareCall(["gpt-4o", "gpt-4o-mini"], est)` —— 引擎一处决定 fallback 与预占。
- `beforeModel` 调 `Commit(resID)` —— 幂等，不会重复扣。
- `afterModel` 调 `Reconcile(resID, actualPrompt, actualCompletion)` —— 把估算值矫正为实际值（少扣补、多扣退）。
- 异常路径 `Release` 释放预占。
- **天然解 P1-03**：fallback 路径已经预占成功，`beforeModel` 不会再次失败。
- **天然解 P2-13**：DB UPSERT 失败 → `PrepareCall` 直接 error，不存在"本地累加 / 远端未写"的 split brain。

**迁移路径**：
1. 先实现 `Engine` 接口 + 内存 + DB 双实现。
2. ModelSelector 改调 `PrepareCall`，老的 `ResolveCostGuardTarget` 内部 delegate。
3. `beforeModel` 改 `Commit`，`afterModel` 加 `Reconcile`。
4. 灰度按 plugin scope 切换。

**工作量**：**M**（1 周）。**强烈推荐**——这是 cost_guard 唯一干净的解法。

---

#### TPM-D-P2：**Hook Chain 引入 Isolation Layer（panic-safe + timeout + bulkhead）** \[Refactor]

**当前**：`hook_resilience.go` 只吞 error，无 `recover()`、无 timeout、单 hook 卡住会拖垮整 turn（P1-05）。

**目标设计**：

```go
type IsolatedHook struct {
    inner       HookFn
    timeout     time.Duration
    onPanic     func(ctx context.Context, err any, stack []byte)
    bulkhead    chan struct{} // semaphore 限制 in-flight
    metrics     HookMetrics
}

func (h *IsolatedHook) Run(ctx context.Context, args HookArgs) (HookResult, error) {
    select {
    case h.bulkhead <- struct{}{}:
        defer func() { <-h.bulkhead }()
    default:
        return HookResult{}, ErrHookOverloaded // 立刻降级，不排队
    }

    cctx, cancel := context.WithTimeout(ctx, h.timeout)
    defer cancel()

    out := make(chan struct{ res HookResult; err error }, 1)
    go func() {
        defer func() {
            if r := recover(); r != nil {
                h.onPanic(cctx, r, debug.Stack())
                out <- struct{...}{HookResult{}, fmt.Errorf("hook panic: %v", r)}
            }
        }()
        res, err := h.inner(cctx, args)
        out <- struct{...}{res, err}
    }()

    select {
    case r := <-out: return r.res, r.err
    case <-cctx.Done(): return HookResult{}, ErrHookTimeout
    }
}
```

- 每个 hook 独立 panic 域、超时、并发上限（bulkhead 防雪崩）。
- 监控 hook 健康度 → 自动熔断（连续 N 次 timeout 后短路）。
- **配合 §16.0 模式 A**：hook 超时与 bulkhead 都进 hook 配置 schema。

**工作量**：**S-M**（3-5 天）。**强烈推荐**——是 P1-05 的根本解。

---

#### TPM-D-P3：**Plugin Scope 改为分层 hierarchical** \[Refactor]

**当前**：plugin scope 只有"global" vs "agent_id"，缺少中间层；导致工作区级、Team 级、Channel 级配置都要被迫归到 global 或一个个 agent。

**目标设计**：

```
scope hierarchy:
  system (硬编码默认)
    └─ workspace (per-tenant)
         └─ team (per-team)
              └─ agent (per-agent)
                   └─ session (per-turn override，例如 admin 单次调试)
```

- 每个 plugin row 写明 `scope_kind` + `scope_id` + `priority`。
- Runtime 解析时按 hierarchy 合并：低层覆盖高层，merge strategy 可选（`replace` / `merge_map` / `append_array`）。
- 与 mcp / skill 的 enabled binding 也能用同一套 scope 机制。

**工作量**：**M-L**（1-2 周；DB schema 改动 + UI 改动）。**中等优先**——产品需求驱动，技术上没卡点。

---

#### TPM-D-P4：**Output Policy 改为"流式状态机"** \[Refactor]

**当前**（P1-04 的根因）：`onEvent` 看不到完整文本 —— 只有当前 chunk，违规 pattern 可能跨 chunk → 既不敢 block 又不敢放过，所以只 log。

**目标设计**：

```go
type StreamingPolicyMatcher struct {
    patterns  []*regexp.Regexp
    window    *RingBuffer // 跨 chunk 滑窗
    matched   atomic.Bool
}

func (m *StreamingPolicyMatcher) Feed(chunk string) (matched bool, matchedPattern string) {
    m.window.Append(chunk)
    return m.scan(m.window.View())
}
```

- `onEvent` 把每个 chunk 喂给 matcher；命中后产生 `BlockEvent` 替换后续 chunk。
- Matcher 配置 window size（默认 4KB，足够覆盖典型 jailbreak / secret 泄露片段）。
- 与现有 `afterModel` 共用 matcher 接口，配置一致。
- **能真正兑现 `block_on_violation`** —— 流式时 splice 出 `[BLOCKED]` 占位。

**工作量**：**S-M**（3-5 天）。**推荐**——直接修 P1-04 且具备产品价值。

---

### 16.3 skill 子系统业务逻辑优化

#### TPM-D-S1：**ZIP Import 改为 Saga / Two-Phase Apply** \[Refactor]

**当前问题**（P1-08）：`ApplyImport` 在 for loop 内 `createImportedSkill`，写盘 + 写库分两步、loop 内中断留 orphan。

**目标设计 — Stage + Commit 两阶段**：

```
Phase 1 — Stage（全部成功才进入 Phase 2）
  for each decision:
    - 计算 final slug / files
    - 写入临时目录 staging/<jobID>/<slug>/
    - 不入主表，仅 staging 表占位

Phase 2 — Commit（事务，多记录可分批）
  begin transaction:
    for each staged:
      - mv staging/<jobID>/<slug> → <root>/<slug>
      - INSERT skill row + skill_version row
    if any failure:
      compensating cleanup → 移走 staged 目录、回滚事务
  commit
```

- **天然原子性**：失败时清理临时目录，主目录无残留。
- 配合 §16.0 模式 B 的 ImportEvent 流，每阶段产生事件，前端实时看到进度。
- staging 目录 TTL 24h，避免崩溃残留。

**工作量**：**M**（1 周）。**推荐**——根本解 P1-08。

---

#### TPM-D-S2：**Skill Repository 引入 Domain Index（slug → key / display → meta 分离）** \[Refactor]

**当前问题**（P1-06）：`trpcskill.Summary` 单一字段 `Name` 同时承担 "filter key" 与 "human label"，FS adapter 用 slug、DB adapter 用 display name → filter 失效。

**目标设计**：把"runtime 主键"与"展示名"分离：

```go
type SkillEntry struct {
    Key         string // canonical slug — runtime 主键 / filter 主键
    DisplayName string // human label — 展示用
    Description string
    Body        string
    Tags        []string
}

type Repository interface {
    Get(ctx context.Context, key string) (SkillEntry, error)
    List(ctx context.Context) iter.Seq[SkillEntry]
}

type VisibilityFilter func(ctx context.Context, key string) bool // 改为按 key
```

- framework 升级阻力：若 `trpcskill.Summary.Name` 是 framework 类型，则在 Aranea 侧再包一层 `SkillEntry`，filter 不再走 framework 的 `VisibilityFilter(summary Summary)`，而是 Aranea 自己的 `VisibilityFilter(key string)`。
- 短期补丁：`DBRepositoryAdapter` 写 `Summary.Name = slug`，display name 走 `Summary.Description` 前缀（次优）。
- 长期：与上游 trpc-agent-go 提 PR，让 `Summary` 多一个 `Key` 字段。

**工作量**：**S**（短期补丁 1 天） / **M**（长期 1 周）。**强烈推荐先做短期**。

---

#### TPM-D-S3：**Skill Version 升级为 Copy-on-Write + Rollback API** \[Greenfield]

**当前**：disk 改了就 in-place 更新 latest version；published skill 触发 `RevertedToDraft` 安全闸；**没有 rollback API**（即使 `skill_version` 表存在）。

**目标设计**：

```go
// 每次 ApplyImport / disk 变更都产生新 version row，draft 状态
// Publish 是把 version 标 active，旧 active → archived（保留 N=10 个）
// Rollback 是把任一 archived version 重新标 active

type SkillVersion struct {
    SkillID    string
    Version    string    // semver: 1.0.0 / 1.0.1 / ...
    Status     string    // draft / active / archived
    Source     string    // import / disk_sync / api
    ImportJobID string   // 追溯创建来源
    CreatedAt  time.Time
}

type SkillUsecase interface {
    PublishVersion(ctx context.Context, skillID, versionID string) error
    RollbackTo(ctx context.Context, skillID, versionID string) (newVersionID string, err error) // 不删，复制为新 version
    ListVersions(ctx context.Context, skillID string) ([]SkillVersion, error)
}
```

- Rollback 不是"删后退"，而是"把旧版本拷贝成新版本号"，保留历史。
- disk 改动产生 `version=auto-N+1, status=draft`，**不再覆盖 active**——彻底消除 `RevertedToDraft` 这种"被动安全闸"。
- 前端有 version 列表 + diff 视图。

**工作量**：**L**（2 周；schema 演进 + UI + API + 数据迁移）。**中长期**。

---

#### TPM-D-S4：**Layer A/B 改为可解释的 Policy Trace** \[Refactor]

**当前**：`AgentVisibilityFilter.allow(summary)` 返回 bool，**用户不知道为什么某个 skill 没出现**。配合 fail-open（P2-01）调试极其困难。

**目标设计**：

```go
type SkillDecision struct {
    Skill    string
    Allowed  bool
    Layer    string // "A_allow" / "A_deny" / "B_intent_miss" / "B_score_below" / "filter_error"
    Score    float64
    Reason   string // 例：B 层根据 query "如何重启服务" 没有命中 "服务运维" 意图
}

func ResolveWithTrace(ctx, agent, query) ([]SkillDecision, error)
```

- Trace 落到 invocation `metadata`，Monitor 页可展开。
- fail-closed by default：错误产生 `Reason: "resolve error: ..."` + `Allowed: false`。
- 配合 §16.0 模式 C 自然推广到统一 policy。

**工作量**：**S-M**（3-5 天）。**强烈推荐**——产品价值高且修 P2-01。

---

### 16.4 mcp 子系统业务逻辑优化

#### TPM-D-M1：**Transport 类型化 + 单一 Codec** \[Refactor]

**当前**（P1-10/P1-12）：transport 用 free-form string，4 处 switch。

**目标设计**：

```go
package mcp

type Transport string

const (
    TransportStdio      Transport = "stdio"
    TransportSSE        Transport = "sse"
    TransportStreamable Transport = "streamable"
)

var transportAliases = map[string]Transport{
    "stdio": TransportStdio,
    "sse": TransportSSE,
    "streamable": TransportStreamable,
    "streamable_http": TransportStreamable,
    "streamablehttp": TransportStreamable,
}

func ParseTransport(s string) (Transport, error) {
    v, ok := transportAliases[strings.ToLower(strings.TrimSpace(s))]
    if !ok { return "", fmt.Errorf("unknown transport: %s", s) }
    return v, nil
}

// ServerConfig 的 Transport 字段类型从 string → Transport
type ServerConfig struct {
    Transport Transport `json:"transport"`
    ...
}

// JSON 反序列化时自动 normalize（强制单一表示）
func (t *Transport) UnmarshalJSON(data []byte) error { ... }
```

- 一旦类型化，**所有 switch 都用类型常量**，IDE 改名一处全改。
- runtime 装配走 `ToTRPCConnectionConfig`（启用并补全字段），probe / observer / reconnect 都共用。
- 不认识的 transport 在 JSON parse 阶段就报错，不会"留到 runtime 失败"。

**工作量**：**S**（2-3 天）。**强烈推荐**——根本解 P1-10/12。

---

#### TPM-D-M2：**Probe 策略化（Handshake Strategy）** \[Refactor]

**当前**（P1-09 的根因）：probe 写死"网络连通性" → 与运行时连接路径完全不同，OAuth 探不通。

**目标设计**：

```go
type ProbeStrategy interface {
    Name() string
    Probe(ctx context.Context, cfg ServerConfig, deps ProbeDeps) ProbeResult
}

type ProbeDeps struct {
    TokenResolver func(ctx context.Context, auth AuthConfig) (string, error) // 注入解决循环依赖
    Clock         clock.Clock
}

// 实现：
type StdioProbeStrategy struct{}     // PATH check
type HTTPConnectivityStrategy struct{} // 现状的 HTTP GET
type MCPHandshakeStrategy struct{}   // 真的发 initialize，验证 protocol version

// admin 可选 probe mode：connectivity / oauth / full_handshake
```

- 平台层不 import agent；通过注入的 `TokenResolver` 解循环。
- admin 可选择 probe 严格度，"network-only" / "with-auth" / "full"。
- 配合 §16.0 模式 B 把 probe 结果作为事件 append 到 metadata。

**工作量**：**M**（1 周）。**推荐**——同时解 P1-09 + P2-29 的测试基础。

---

#### TPM-D-M3：**Health / Reconnect / Alert 统一为单一 Server Lifecycle FSM** \[Greenfield]

**当前**：health probe 写 `last_error_message`、reconnect observer 写 `reconnect_count`、alert 写 `last_health_alert_at` —— 三处独立的 metadata mutation，并发 last-write-wins（P2-28）。

**目标设计**：把 mcp server "活的" 状态建模成一个 FSM：

```
                  ┌───────┐
                  │ unknown│
                  └───┬───┘
                      │ probe_ok
                      ▼
              ┌──────────────┐  reconnect_started  ┌────────────┐
              │   healthy    │ ───────────────────▶│ degraded   │
              └──────┬───────┘                     └──────┬─────┘
                     │ probe_fail × N                     │
                     ▼                                    │
              ┌──────────────┐  recover                   │
              │  unhealthy   │ ◀──────────────────────────┘
              └──────┬───────┘  alert_emitted (debounced)
                     │
                     ▼
              ┌──────────────┐
              │   alerting   │
              └──────────────┘
```

- **唯一的 mutator** 是 FSM `Transition(event)`，并发安全（mutex / single-writer）。
- 状态切换产生事件，alert / dashboard / Prometheus 都订阅同一事件。
- `metadata_json` 退化为"派生状态镜像"，主真相是 events 表。

**工作量**：**M-L**（1-2 周）。**中长期**——若团队接受 §16.0 模式 B（事件溯源），自然落地。

---

### 16.5 跨子系统的小型业务优化（10 条速胜）

| ID | 子系统 | 改动 | 收益 | 工作量 |
|----|-------|------|------|-------|
| TPM-Q-01 | tools | webresearch 加 **per-provider circuit breaker**（Tavily 503 后 30s 内只走 SerpAPI） | 故障期降级更快 | XS |
| TPM-Q-02 | tools | `cache.ResultCache` 改 LRU（标准库或 hashicorp/golang-lru） | 高并发 evict O(1) | XS |
| TPM-Q-03 | tools | mcp 工具调用引入 **per-server timeout budget**（一个 turn 内调同 server 多次共享一个总 budget） | 防慢 MCP 拖垮整 turn | S |
| TPM-Q-04 | plugin | `retry_and_reflect` 加 **反思次数 + 模型质量评分** 联动（连续 N 次低分自动停） | 防无效反思循环 | S |
| TPM-Q-05 | plugin | `audit_log` 输出走 **batched async writer**（10ms 攒一批） | 减少高频 turn 的 log IO | S |
| TPM-Q-06 | plugin | OnEvent dispatch 改 **fan-out 并发**（不同 hook 并发执行，block 路径仍串行）| 监控类 hook 延迟敏感 | S |
| TPM-Q-07 | skill | watch 改 **per-slug actor**（每 slug 一个 goroutine + chan，避免全局 mutex） | 并发同步无 race（P2-22） | S |
| TPM-Q-08 | skill | importer LLM similarity 加 **embedding 预筛**（cos sim > 0.5 才做 LLM 判定） | LLM 调用 N → log N | M |
| TPM-Q-09 | mcp | health.Runner 加 **bounded worker pool**（默认 8 并发） | 大规模 fleet 防雪崩 | XS |
| TPM-Q-10 | mcp | OAuth 引入 **proactive refresh**（过期前 60s 主动刷新，失败立即报警） | 消除 P2-27 静默失败 | S |

---

### 16.6 建议的迭代节奏

| 阶段 | 推荐重设计项 | 周期 |
|------|------------|------|
| **Wave 1**（修 P1） | TPM-D-T1（tool resolver）· TPM-D-P1（cost reservation）· TPM-D-P2（hook isolation）· TPM-D-S2 短期补丁（skill key）· TPM-D-M1（mcp transport 类型化） | 2-3 周 |
| **Wave 2**（业务可观测） | TPM-D-T2（effective plan）· TPM-D-S4（skill policy trace）· TPM-D-M2（probe strategy）· §16.5 速胜 10 条 | 3-4 周 |
| **Wave 3**（架构升级） | §16.0 模式 A（schema-as-code）· TPM-D-P4（streaming policy）· TPM-D-S1（saga import）· TPM-D-S3（version rollback） | 4-6 周 |
| **Wave 4**（中长期）| §16.0 模式 B（event sourcing）· 模式 C（policy engine）· TPM-D-M3（FSM） · TPM-D-P3（hierarchical scope） | 8-12 周 |

> Wave 1 与 §15 Sprint N 重叠 80%，两者协同推进；其余 wave 是"如果团队有半年余量推荐怎么走"的远景。

---

## 17. 验证命令

```bash
# 子系统单元测试
go test ./internal/tools/... -count=1
go test ./internal/plugin/... -count=1
go test ./internal/skill/... -count=1
go test ./internal/mcp/... -count=1

# 关键路径
go test ./internal/tools/... -run 'Prune|Assemble|Workspace|Webresearch|Hostexec' -count=1 -v
go test ./internal/plugin/... -run 'Orchestration|CostGuard|HookNotify|ChainMirror|Builtin' -count=1 -v
go test ./internal/skill/... -run 'Validate' -count=1 -v
go test ./internal/mcp/... -run 'Config|Classify|Metadata' -count=1 -v

# race 检测（强烈推荐打开 — skill/watch 与 plugin 都有未加锁路径）
go test ./internal/tools/... ./internal/plugin/... ./internal/skill/... ./internal/mcp/... -race -count=1

# 全量
go build ./...
make ci
```

---

## 18. 总结

- **架构边界基本干净**：四个子系统都遵循 trpc-agent-framework-first 红线，`internal/biz` 不反向 import 任一子包；Runner / Manager / Coordinator 装配在 service 层；子包之间无循环依赖。
- **职责分层清晰**：tools 的"Registry + Assemble + 子包" 三段结构、plugin 的"四层编排"、skill 的"importer / watch / trpc / storage"、mcp 的"platform / runtime 分离"——四套子系统都有可读的拓扑。
- **主要技术债集中在三类**：
  1. **协议/字段漂移**：tools `web_search` alias 双向不一致；plugin 死 schema 字段；skill `Summary.Name` 语义双义；mcp transport 4 处分裂。
  2. **Fail-open / 静默吞错**：skillruntime filter fail open；OpenAPI loader 静默 skip；alert 持久化 `_ =` 吞掉；cost_guard budget DB 写失败 `_ =`。
  3. **关键路径缺测试**：skill 全子系统几乎裸；mcp probe/health/alert 0 测试；plugin cost_guard double-block 回归 0；chain panic recover 0。
- **建议**：本迭代优先解决 §15 Sprint N 七条 P1，重点是 **mcp transport 统一 + plugin cost_guard/panic 修复 + skill filter/zipslip 修复**——这三组直接影响线上正确性与安全性；下一迭代补 P1 收尾、安全 redaction 与 dead config 清理；再下一迭代补测试基线。
- **业务逻辑层面的中长期升级**：§16 给出了"如果重新设计"的 14 条建议，分四 wave 推进：
  - **Wave 1**（与 §15 Sprint N 同步）：tool resolver 统一、cost_guard reservation pattern、hook isolation、mcp transport 类型化、skill key 短期补丁 —— 既修 P1 也是结构改良。
  - **Wave 2-3**：Effective Plan / Streaming Policy / Saga Import / Version Rollback —— 把"静默吞错"和"无原子性"这两类债从根上消掉。
  - **Wave 4**：跨子系统的三个通用模式（Schema-as-Code / Event Sourcing / Policy Engine）—— 是 6 个月级的远景，能让 plugin/mcp/skill/tools 共享一套契约、状态、策略基础设施。

> 评分加权后 **82 / 100**：架构面满分项较多，业务正确性与测试覆盖拖后腿。修复 §4 的 12 个 P1 后，预期可达 88+；落地 §16 Wave 1-2 后，预期可达 92+。
