# M57 — Tools / Plugin / Skill / MCP 子系统优化开发计划（TPM-OPT）

> **版本**：2026-05-28 · **状态**：🚧 实施中 · **EP**：EP-TPM-M57
> **背景 Review**：[2026-05-26-Tools-Plugin-Skill-MCP-Code-Review.md](../review/2026-05-26-Tools-Plugin-Skill-MCP-Code-Review.md)
> **总工时估算**：12 P1（2-3 周）+ 14 项重设计（4-12 周 Wave 2-4）
> **与 M56 关系**：**正交**——M56 BLO 聚焦"Channel × Chat × Agent/Team 主链路"业务模型；M57 TPM 聚焦"tools/plugin/skill/mcp 平台子系统"代码债。两者可并行。

---

## 0. 任务 ID 编码约定

```
TPM-{P|D|Q}-{序号}        — review 原始 ID 直接复用
TPM-WAVE{N}-{主题}-{序号} — 跨主题汇总时使用
```

| 等级 | 含义 |
|------|------|
| **TPM-P1-** | 12 项 P1 关键路径（Wave 1） |
| **TPM-P2-** | 30 项 P2 强化（Wave 2） |
| **TPM-P3-** | 13 项 P3 维护性（Wave 3） |
| **TPM-D-**  | 14 条业务逻辑重设计（Wave 2/3/4 跨度） |
| **TPM-Q-**  | 10 条速胜（穿插于各 Wave） |

---

## 1. 当前状态（2026-05-28）

| 项 | 状态 |
|----|------|
| Review 文档 | ✅ [`2026-05-26-Tools-Plugin-Skill-MCP-Code-Review.md`](../review/2026-05-26-Tools-Plugin-Skill-MCP-Code-Review.md) |
| 跟踪文档 | ✅ 本文档 |
| Wave 1 P1 实施 | ✅ 全部完成 |
| Wave 2 P2 实施 | ✅ 大部分完成（P2-01~05/07/08/09/10/11/12/13/14/16/18/19/20/21/22/25/26/27/28/29/30 + P3-12/13 + D-M1/M2 已完成） |
| Wave 3/4 | 📋 待启动 |

---

## 2. Wave 1：P1 关键路径（本迭代 / 2-3 周）

### 2.1 实施顺序（按依赖与体量排序）

| # | ID | 子系统 | 体量 | 状态 | 文件 |
|---|----|-------|-----|------|------|
| 1 | **TPM-P1-02** | tools | XS（3 行） | ✅ | `internal/tools/runtime_alias.go` |
| 2 | **TPM-P1-11** | mcp | S（10 行） | ✅ | `internal/mcp/probe/eval.go` |
| 3 | **TPM-P1-07** | skill | S（10 行） | ✅ | `internal/skill/importer/engine.go` |
| 4 | **TPM-P1-10 + P1-12** | mcp | M | ✅ | `internal/mcp/config/config.go` · `internal/mcp/probe/eval.go` · `internal/tools/toolset.go` · `internal/tools/mcpobserve/observe.go` |
| 5 | **TPM-P1-01** | tools | S | ✅ | `internal/tools/runtime_alias.go` · `internal/biz/tool/tool_policy_keys.go` |
| 6 | **TPM-P1-05** | plugin | M | ✅ | `internal/plugin/trpc/hook_resilience.go` |
| 7 | **TPM-P1-04** | plugin | M | ✅ | `internal/plugin/trpc/output_policy.go` |
| 8 | **TPM-P1-06** | skill | S | ✅ | `internal/skill/trpc/db_repository.go` |
| 9 | **TPM-P1-03** | plugin | M | ✅ | `internal/plugin/trpc/cost_guard.go` |
| 10 | **TPM-P1-08** | skill | L | ✅ | `internal/skill/importer/engine.go` |
| 11 | **TPM-P1-09** | mcp | L | ✅ | `internal/mcp/probe/eval.go` |

### 2.2 验收 Gate

每个 P1 完成必须满足：

1. **go build ./... 通过**
2. **对应子系统单测 `go test ./internal/{tools,plugin,skill,mcp}/... -count=1` 通过**
3. **新增至少 1 个回归测试**（针对修复的 bug 路径）
4. **review 文件 §4 P1 表格标记 ✅**

---

## 3. Wave 2：P2 + 部分重设计（下一迭代 / 3-4 周）

### 3.1 P2 优先级（30 条）

按"安全 → 死配置 → 静默吞错 → 性能"分组：

| 组 | 包含 ID |
|----|---------|
| 安全 | TPM-P2-08（SerpAPI key）✅、TPM-P2-11/12（PII 泄露）✅、TPM-P2-27（OAuth stale token）✅ |
| 死配置 | TPM-P2-03/09/10（claudefetch / admin_bypass / confirm_tools / role_rules） ✅ |
| 静默吞错 | TPM-P2-01（skill fail-open） ✅、TPM-P2-02（OpenAPI loader） ✅、TPM-P2-13（cost DB）✅、TPM-P2-21（reload）✅、TPM-P2-25（alert）✅ |
| 性能 | TPM-P2-05（cache eviction） ✅、TPM-P2-14/15/16/18/22/26/30 ✅ |
| 安全路径 | TPM-P2-19（ReadSkillDirFiles 不安全路径）✅、TPM-P2-20（Multipart 限制不一致）✅ |
| 缺测试 | TPM-P2-29（mcp probe/health/alert）✅、skill 全套 |
| 代码卫生 | TPM-P2-04（configString 统一） ✅ |
| 功能增强 | TPM-P2-17（job map TTL）✅、TPM-P2-23（RBAC）📋、TPM-P2-24（版本回滚）📋 |

### 3.2 Wave 2 包含的重设计

- **TPM-D-T1**（Tool Name Resolver 单一服务化）
- **TPM-D-S2 完整版**（Repository Domain Index）
- **TPM-D-M2**（Probe Handshake Strategy）
- **TPM-P1-08**（skill Saga apply） ✅
- **TPM-P1-09**（mcp probe OAuth via TokenResolver） ✅

---

## 4. Wave 3：架构升级（4-6 周）

| ID | 主题 |
|----|------|
| **TPM-D-P1** | Cost Guard Reservation Pattern（Prepare/Commit/Reconcile/Release） |
| **TPM-D-P2** | Hook Isolation Layer（panic+timeout+bulkhead）全量版本 |
| **TPM-D-P4** | Output Policy Streaming State Machine 全量版本 |
| **TPM-D-T2** | Tool Effective Plan with Reason |
| **TPM-D-S1** | Skill Saga Import |
| **TPM-D-S3** | Skill Version Copy-on-Write + Rollback |
| **TPM-D-S4** | Skill Policy Trace |
| **§16.0 模式 A** | Schema-as-Code（contract 包） |

---

## 5. Wave 4：中长期愿景（8-12 周）

| ID | 主题 |
|----|------|
| **§16.0 模式 B** | Event Sourcing（skill/mcp metadata 改事件流） |
| **§16.0 模式 C** | Policy Engine（OPA-lite） |
| **TPM-D-M3** | MCP Server Lifecycle FSM |
| **TPM-D-P3** | Plugin Scope Hierarchical |

---

## 6. 风险与缓解

| 风险 | 等级 | 缓解 |
|------|------|------|
| Wave 1 P1-10 transport 改动影响多个 caller | 低 | 类型保持 alias `string`，逐步迁移；优先 NormalizeTransport 在 caller 侧统一 |
| Wave 1 P1-05 panic recover 改变错误传播语义 | 低 | recover → log + record + 返回 nil，与现有 resilient swallow 同语义 |
| Wave 1 P1-01 web_search alias 重定向影响生产 LLM 调用 | 中 | "runtime alias 删除"方案，保留 policy 端 alias，runtime alias 表只留确实需要的 |
| Wave 2 死配置删除破坏现有 DB row | 低 | schema 兼容删除（DB 字段保留，code 端忽略）；新 schema 不含字段 |

---

## 7. 文档约定

- 每个任务 PR 标题：`[TPM-P1-XX] {description}`
- 每个 PR 必须更新本文档对应任务状态（`📋 → 🚧 → ✅`）
- 完成后在 review 文件 §4 / §5 表格里同步标记 ✅
- 重大架构改动（D 类）单独出 design 文档

---

## 8. 相关链接

- 背景 Review：[../review/2026-05-26-Tools-Plugin-Skill-MCP-Code-Review.md](../review/2026-05-26-Tools-Plugin-Skill-MCP-Code-Review.md)
- 并行计划：[56-business-logic-optimization-development.md](./56-business-logic-optimization-development.md)（M56 BLO）
- 框架边界：[../AGENT_RUNTIME_BOUNDARY.md](../AGENT_RUNTIME_BOUNDARY.md)
- 红线规则：[.cursor/rules/trpc-agent-framework-first.mdc](../../.cursor/rules/trpc-agent-framework-first.mdc)
- AI 编码规范：[../guides/AI-DEVELOPMENT-SPECIFICATION.md](../guides/AI-DEVELOPMENT-SPECIFICATION.md)

---

## 9. 执行进度表

> 每完成一项更新此表 + review §4 P1 表

| Wave | ID | 完成日期 | PR | 备注 |
|------|----|---------|----|------|
| 1 | TPM-P1-02 | ✅ | — | aliasTool.Call → error |
| 1 | TPM-P1-11 | ✅ | — | mcp probe CheckRedirect（outboundguard 已内置） |
| 1 | TPM-P1-07 | ✅ | — | skill zipslip filepath.Rel |
| 1 | TPM-P1-10 | ✅ | — | mcp Transport 类型化 + UnmarshalJSON 自动 normalize |
| 1 | TPM-P1-12 | ✅ | — | mcp ToConnectionConfig 唯一映射入口 + Env 补全 |
| 1 | TPM-P1-01 | ✅ | — | web_search alias 对齐 |
| 1 | TPM-P1-05 | ✅ | — | plugin chain panic recover + 回归测试 |
| 1 | TPM-P1-04 | ✅ | — | output_policy.onEvent block_on_violation + Delta.Content 修复 |
| 1 | TPM-P1-06 | ✅ | — | skill DBRepositoryAdapter Summary.Name=slug |
| 1 | TPM-P1-03 | ✅ | — | cost_guard fallback bypass daily budget + 回归测试 |
| 2 | TPM-P1-08 | 2026-05-26 | — | skill ApplyImport 补偿删除（createdSkillRecord + compensate） |
| 2 | TPM-P1-09 | 2026-05-26 | — | mcp probe 401/403 → auth_required，doHTTPProbe 旁路 |
| 2 | TPM-P2-01 | ✅ | — | skillruntime filter fail-open → fail-closed |
| 2 | TPM-P2-02 | ✅ | — | OpenAPI loader 错误 SysLogWarn |
| 2 | TPM-P2-03 | ✅ | — | claudefetch 返回 (nil, nil) 对齐 geminifetch |
| 2 | TPM-P2-04 | ✅ | — | ConfigString 统一到 tools 包 |
| 2 | TPM-P2-05 | ✅ | — | skillruntime cache filterCache 512 条上限 |
| 2 | TPM-P2-07 | 2026-05-28 | — | mcpobserve context.Background() → context.WithoutCancel |
| 2 | TPM-P2-09 | ✅ | — | cost_guard admin_bypass 死配置删除 |
| 2 | TPM-P2-10 | ✅ | — | permission_guard confirm_tools/role_rules 死配置删除 |
| 2 | TPM-P2-25 | 2026-05-28 | — | alert.MarkHealthAlertEmitted 失败不再静默吞错 |
| 2 | TPM-P2-29 | 2026-05-28 | — | mcp config 单元测试补全（ParseTransport + UnmarshalJSON） |
| 2 | TPM-P3-12 | 2026-05-28 | — | mcp magic numbers 集中到 internal/mcp/defaults.go |
| 2 | TPM-D-M1 | 2026-05-28 | — | Transport 类型化 + ToConnectionConfig 委托已完成；MCPServerConfig 字段仍独立 |
| 2 | TPM-P2-26 | 2026-05-28 | — | mcpobserve O(n) 全表扫 → GetMCPServerByKey 精确查询 |
| 2 | TPM-P2-27 | 2026-05-28 | — | OAuth refresh 失败不再 fallback 陈旧 access_token |
| 2 | TPM-P2-29 | 2026-05-28 | — | probe + alert 单元测试补全（8 个新测试） |
| 2 | TPM-P2-28 | 2026-05-28 | — | metadata 并发写 → UpdateMCPServerMetadata 只写 metadata+status 字段 |
| 2 | TPM-P2-30 | 2026-05-28 | — | health probeAll semaphore bounded concurrency（max=8） |
| 2 | TPM-P3-13 | 2026-05-28 | — | classify 前缀启发式评估后保留（误判风险极低） |
| 2 | Review-1 | 2026-05-28 | ✅ | 删除 ToTRPCConnectionConfig 死代码，消除双 source of truth |
| 2 | Review-2 | 2026-05-28 | ✅ | cost_guard TryConsume 安全网语义注释 |
| 2 | Review-3 | 2026-05-28 | ✅ | cost_guard fallback bypass daily_budget 设计意图注释 |
| 2 | Review-4 | 2026-05-28 | ✅ | filter.go slog.Warn 违反红线 #10 → 替换为 event.SysLogWarn |
| 2 | Review-5 | 2026-05-28 | ✅ | filter.go allowedSlugs 空 if 块简化 |
| 2 | Review-6 | 2026-05-28 | ✅ | 测试代码 promoted field 风格统一为 base: basePlugin{...} |
| 2 | Fix-Misc | 2026-05-28 | ✅ | runner.go SkillInvocationWrite 删除不存在的 SkillName 字段 |
| 2 | TPM-D-M2 | 2026-05-28 | — | Probe 策略化：ProbeStrategy 接口 + ConnectivityProbe + AuthAwareProbe + ProbeMode 配置 + 28 个单测 |
| 2 | TPM-P2-13 | 2026-05-28 | ✅ | cost_guard AddTokens 写库失败不再静默吞错，改为 SysLogWarn |
| 2 | TPM-P2-21 | 2026-05-28 | ✅ | skill DBRepositoryAdapter reload/loadBody 失败加 SysLogWarn |
| 2 | TPM-P2-11 | 2026-05-28 | ✅ | audit_log.summarizeMessages 加 maybeRedact 脱敏 |
| 2 | TPM-P2-12 | 2026-05-28 | ✅ | skill_usage_tracker input/output preview 加 redactText 脱敏 |
| 2 | TPM-P2-08 | 2026-05-28 | ✅ | SerpAPI key redactedURL + 注释警示（SerpAPI 不支持 header 鉴权） |
| 2 | TPM-P2-14 | 2026-05-28 | ✅ | retry_and_reflect global scope 加 1h TTL eviction |
| 2 | TPM-P2-18 | 2026-05-28 | ✅ | inspectSimilarity LLM 调用加上限 50 次 |
| 2 | Review-7 | 2026-05-28 | ✅ | AuthAwareProbe.Probe 提取 validateHTTPConfig 消除 DRY 违规 |
| 2 | Review-8 | 2026-05-28 | ✅ | auth_aware 无 auth 路径翻转 auth_required → OK=false |
| 2 | Review-9 | 2026-05-28 | ✅ | loadBody 失败不缓存空 body Skill |
| 2 | TPM-P2-22 | 2026-05-28 | ✅ | watch/runner.go childWatches 并发写加 mu 保护 |
| 2 | TPM-P2-19 | 2026-05-28 | ✅ | ReadSkillDirFiles 不安全路径改为返回错误（与 ZIP 严格性对齐） |
| 2 | TPM-P2-20 | 2026-05-28 | ✅ | Multipart 25MB→20MB 与 engine MaxZipBytes 统一 |
| 2 | TPM-P2-16 | 2026-05-28 | ✅ | dispatchHookOnEvent 失败策略与 chain hook 统一（resilient swallow + block 传播） |
| 2 | TPM-P2-15 | 2026-05-28 | ✅ | Manager 实际已是 Wire 单例，P2-15 不适用 |
| 2 | TPM-P2-06 | 2026-05-28 | ✅ | globalBridge 已重构为 Wire DI 注入，P2-06 不适用 |
| 2 | TPM-P2-17 | 2026-05-28 | ✅ | Engine jobs map 加 2h TTL 过期清理 + evictExpiredLocked |
| 2 | Review2-2 | 2026-05-28 | ✅ | cli_admin/pkg_install_from_url.go SSRF 防护（validateRepoURL + isPrivateIP） |
| 2 | Review2-3 | 2026-05-28 | ✅ | cache/result_cache.go TOCTOU 竞态修复（过期条目升级写锁后重新检查） |
| 2 | Review2-4 | 2026-05-28 | ✅ | skillruntime/filter.go filterCache 全量驱逐改为随机淘汰 |
| 2 | Review2-5 | 2026-05-28 | ✅ | skillruntime/resolve.go 删除 4 个死代码函数（红线#12） |
| 2 | Review2-6 | 2026-05-28 | ✅ | webresearch/http_client.go 代理 URL 解析失败加 SysLogWarn |
| 2 | Review2-7 | 2026-05-28 | ✅ | cli_admin/registry.go Deps.String() 遮蔽 APIToken |
| 2 | Fix-Knowledge | 2026-05-28 | ✅ | knowledge/hybrid_retriever.go 删除 rerankCandidateLimit/trimChunks 重复声明 |
| 2 | OOP-1 | 2026-05-28 | ✅ | kanban Bridge 9 方法接口拆分为 BridgeReader/BridgeWriter/BridgeLifecycle + Bridge 组合 |
| 2 | OOP-2 | 2026-05-28 | ✅ | skillruntime 定义 SkillResolver/RuntimeSettings 窄接口，替代 *biz.SkillUsecase/*biz.AgentRuntimeSettings 具体类型依赖 |
| 2 | OOP-3 | 2026-05-28 | ✅ | serviceawaitreply/tool.go MarkAwaitingUserReply 错误不再静默吞掉，改为 SysLogWarn |
| 2 | TST-SKILL-01 | 2026-05-29 | ✅ | biz/skill Usecase 48 个单测（CRUD + 权限 + Embedding 缓存 + ParseRuntimePolicy） |
| 2 | TST-SKILL-02 | 2026-05-29 | ✅ | skill/storage SkillFilesystem 37 个单测（SafeFilePath zipslip 防护 + 文件操作 + 辅助函数） |
| 2 | TST-SKILL-03 | 2026-05-29 | ✅ | skillruntime ResolveSkillSlugsDetailed 37 个单测（Layer A/B + 意图路由 + 标签过滤 + Embedding 评分） |
| 2 | ERR-SKILL-01 | 2026-05-29 | ✅ | importer 包 22 处 fmt.Errorf → 16 个 sentinel error + pathError/detailError 类型（支持 errors.Is 链式匹配） |
| 2 | LOG-SKILL-01 | 2026-05-29 | ✅ | watch/ 模块日志迁移 kratos/v2/log → internal/event FlowLog（SysLogInfo/Warn/Error） |
| 2 | SAFE-SKILL-01 | 2026-05-29 | ✅ | watch/runner.go time.AfterFunc 回调走 safego.Go（红线 #13 合规） |
| 2 | OOP-SKILL-01 | 2026-05-29 | ✅ | importer.Engine 改用 SkillImportRepo 窄接口（3 方法）替代完整 biz.SkillRepo（22 方法） |
| 2 | FIX-MISC-2 | 2026-05-29 | ✅ | memory_l4_cascade.go 添加 store 字段修复编译错误；knowledge/tool.go Search 签名对齐 |
| 2 | OOP-SKILL-02 | 2026-05-29 | ✅ | SkillReader(13)→SkillQueryReader(5)+SkillLookupReader(4)+SkillRuntimeReader(4)；SkillWriter(10)→SkillMutationWriter(5)+SkillSyncWriter(5)；SkillFilesystem(10)→SkillFilePathResolver(2)+SkillFileReader(4+2)+SkillFileWriter(3+2) |
| 2 | TST-SKILL-04 | 2026-05-29 | ✅ | importer/validate ValidateSkillPackage 49 个单测（高风险文件/Shell 脚本/重复名/路径安全/slugify） |
| 2 | TST-SKILL-05 | 2026-05-29 | ✅ | importer/chat 22+ 个单测（OpenAI/Anthropic 端点/prompt 构建/JSON 解析/凭证检测/模型解析/HTTP mock） |
| 2 | TST-SKILL-06 | 2026-05-29 | ✅ | skillrouter 6 个补充单测（空查询/单关键词/无匹配/空输入/重复 token/分类叶子） |
| 2 | TST-SKILL-07 | 2026-05-29 | ✅ | manifest Parse 6 个单测（有效/空/无效 YAML/缺 name/tags/taxonomy_paths） |
| 2 | ERR-SKILL-02 | 2026-05-29 | ✅ | importer 包最后 3 处 fmt.Errorf → detailErr()；detailError.Error() 补充 sentinel 消息 |
| 2 | FIX-SKILL-01 | 2026-05-29 | ✅ | chat.go providerModelHasCredentials 支持 api_key_set；resolveChatModel 改用 GetByProviderAndModel；json.Unmarshal 吞错误修复 |

---

## 10. 剩余工作总结（2026-05-29）

### 10.1 Wave 2 已完成统计

| 类别 | 完成数 | 总数 | 完成率 |
|------|--------|------|--------|
| P1 关键路径 | 11 | 11 | 100% |
| P2 强化 | 28 | 30 | 93% |
| P3 维护性 | 2 | 13 | 15% |
| D 重设计 | 3 | 14 | 21% |
| Q 速胜 | 9 | 10 | 90% |
| Skill OOP/测试/错误 | 15 | 15 | 100% |

### 10.2 Wave 2 未完成项（P2-23/P2-24）

| ID | 主题 | 优先级 | 说明 |
|----|------|--------|------|
| TPM-P2-23 | Skill RBAC 权限控制 | P2 | 当前 RBAC hard-coded `true`（DEBT-04），需设计权限模型 |
| TPM-P2-24 | Skill 版本回滚 | P2 | 依赖 TPM-D-S3（Copy-on-Write + Rollback），需先完成架构设计 |

### 10.3 Skill 子系统剩余优化项

| ID | 优先级 | 主题 | 说明 |
|----|--------|------|------|
| SKILL-P2-01 | P2 | `internal/tools/` ~84 处 `fmt.Errorf` | 工具执行层错误，非业务错误，不经过 kerrors 链，低优先级 |
| SKILL-P2-02 | P2 | `SkillFileReader` 6 方法（略超 ≤5） | 可接受；SkillFilePathResolver 已提取独立子接口 |
| SKILL-P2-03 | P2 | `slugify("")` 固定生成 "skill-0" | 非唯一，需补全局唯一 slug 生成逻辑 |
| SKILL-P2-04 | P2 | `GetImportJob` 读操作用 `Lock()` | 改为 `RLock()` 提升并发读性能 |
| SKILL-P2-05 | P2 | `ApplyImport` 中文错误消息 | 统一为英文 sentinel error |
| SKILL-P2-06 | P2 | `watch.Runner` + `trpc.DBRepositoryAdapter` 依赖 `*biz.SkillUsecase` 具体类型 | 改用窄接口依赖 |

### 10.4 Wave 3 架构升级（待启动）

| ID | 主题 | 依赖 |
|----|------|------|
| TPM-D-P1 | Cost Guard Reservation Pattern | — |
| TPM-D-P2 | Hook Isolation Layer 全量版本 | — |
| TPM-D-P4 | Output Policy Streaming State Machine 全量版本 | — |
| TPM-D-T2 | Tool Effective Plan with Reason | — |
| TPM-D-S1 | Skill Saga Import | TPM-D-S3 |
| TPM-D-S3 | Skill Version Copy-on-Write + Rollback | — |
| TPM-D-S4 | Skill Policy Trace | — |
| §16.0 模式 A | Schema-as-Code（contract 包） | — |

### 10.5 Wave 4 中长期愿景（待启动）

| ID | 主题 |
|----|------|
| §16.0 模式 B | Event Sourcing（skill/mcp metadata 改事件流） |
| §16.0 模式 C | Policy Engine（OPA-lite） |
| TPM-D-M3 | MCP Server Lifecycle FSM |
| TPM-D-P3 | Plugin Scope Hierarchical |

### 10.6 建议下一步优先级

1. **SKILL-P2-04**（`RLock` 改造）— 最小改动、即时收益
2. **SKILL-P2-05**（中文错误消息统一）— 小改动、代码卫生
3. **SKILL-P2-06**（窄接口依赖）— 架构合规、中等改动
4. **SKILL-P2-03**（slug 唯一性）— 功能正确性
5. **TPM-D-S3**（Skill Version CoW + Rollback）— Wave 3 关键路径
6. **TPM-D-S1**（Skill Saga Import）— 依赖 D-S3
