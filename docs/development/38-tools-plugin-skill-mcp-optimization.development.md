# M57 — Tools / Plugin / Skill / MCP 子系统优化 — 开发计划

> **版本**：2026-06-06 | **状态**：🟢 Wave 1/2 已完成，Wave 3/4 待启动
> **EP**：EP-TPM-M57
> **与 M56 关系**：**正交**——M56 BLO 聚焦"Channel x Chat x Agent/Team 主链路"业务模型；M57 TPM 聚焦"tools/plugin/skill/mcp 平台子系统"代码债。两者可并行。

### 需求来源

| 子系统 | 需求文档 | 设计文档 | 开发计划 |
|--------|----------|----------|----------|
| MCP | [19-mcp.md](./19%20mcp.md) | [19-mcp.design.md](./19%20mcp.design.md) | [19-mcp.development.md](./19%20mcp.development.md) |
| Skill | [20-skill.md](./20%20skill.md) | [20-skill.design.md](./20%20skill.design.md) | [20-skill.development.md](./20%20skill.development.md) |
| Plugin | [22-plugin.md](./22%20plugin.md) | [22-plugin.design.md](./22%20plugin.design.md) | [22-plugin.development.md](./22%20plugin.development.md) |
| Tools | [23-tools.md](./23%20tools.md) | [23-tools.design.md](./23%20tools.design.md) | [23-tools.development.md](./23%20tools.development.md) |

> 本文档为跨子系统优化的**协调跟踪文档**，各子系统的详细需求和设计见上表对应文档。

---

## 1. 任务 ID 编码约定

| 等级 | 含义 | 数量 |
|------|------|------|
| **TPM-P1-** | P1 关键路径 | 12 项 |
| **TPM-P2-** | P2 强化 | 30 项 |
| **TPM-P3-** | P3 维护性 | 13 项 |
| **TPM-D-** | 业务逻辑重设计 | 14 项 |
| **TPM-Q-** | 速胜 | 10 项 |

---

## 2. 现状评估（2026-06-06）

| 项 | 状态 |
|----|------|
| Wave 1 P1 实施 | ✅ 全部完成（12/12） |
| Wave 2 P2 实施 | ✅ 全部完成（30/30） |
| Wave 2 P3 实施 | ✅ 已完成 2 项（P3-12/13） |
| Wave 2 D 重设计 | ✅ 已完成 3 项（D-M1/M2 + P1-08 Saga/P1-09 OAuth） |
| Wave 2 Skill OOP/测试/错误 | ✅ 全部完成（15/15） |
| Wave 3/4 | 📋 待启动 |

---

## 3. Wave 1：P1 关键路径

| # | ID | 子系统 | 体量 | 状态 | 关键文件 |
|---|----|-------|-----|------|----------|
| 1 | **TPM-P1-02** | tools | XS | ✅ | `internal/tools/runtime_alias.go` |
| 2 | **TPM-P1-11** | mcp | S | ✅ | `internal/mcp/probe/eval.go`（outboundguard.NewClient 已内置 CheckRedirect） |
| 3 | **TPM-P1-07** | skill | S | ✅ | `internal/skill/importer/engine.go` + `helpers.go`（ensurePathWithin zipslip 防护） |
| 4 | **TPM-P1-10 + P1-12** | mcp | M | ✅ | `internal/mcp/config/config.go` · `internal/tools/toolset.go` · `internal/tools/mcpobserve/observe.go` |
| 5 | **TPM-P1-01** | tools | S | ✅ | `internal/tools/runtime_alias.go` · `internal/biz/tool/tool_policy_keys.go` |
| 6 | **TPM-P1-05** | plugin | M | ✅ | `internal/plugin/trpc/hook_resilience.go` |
| 7 | **TPM-P1-04** | plugin | M | ✅ | `internal/plugin/trpc/output_policy.go` |
| 8 | **TPM-P1-06** | skill | S | ✅ | `internal/skill/trpc/db_repository.go` |
| 9 | **TPM-P1-03** | plugin | M | ✅ | `internal/plugin/trpc/cost_guard.go` |
| 10 | **TPM-P1-08** | skill | L | ✅ | `internal/skill/importer/engine.go` |
| 11 | **TPM-P1-09** | mcp | L | ✅ | `internal/mcp/probe/eval.go` |

### 验收 Gate

每个 P1 完成必须满足：

1. **go build ./... 通过**
2. **对应子系统单测 `go test ./internal/{tools,plugin,skill,mcp}/... -count=1` 通过**
3. **新增至少 1 个回归测试**（针对修复的 bug 路径）

---

## 4. Wave 2：P2 + 部分重设计

### 4.1 P2 优先级（30 条）

按"安全 → 死配置 → 静默吞错 → 性能"分组：

| 组 | 包含 ID |
|----|---------|
| 安全 | TPM-P2-08（SerpAPI key）✅、TPM-P2-11/12（PII 泄露）✅、TPM-P2-27（OAuth stale token）✅ |
| 死配置 | TPM-P2-03/09/10（claudefetch / admin_bypass / confirm_tools / role_rules）✅ |
| 静默吞错 | TPM-P2-01（skill fail-open）✅、TPM-P2-02（OpenAPI loader）✅、TPM-P2-13（cost DB）✅、TPM-P2-21（reload）✅、TPM-P2-25（alert）✅ |
| 性能 | TPM-P2-05（cache eviction）✅、TPM-P2-14/15/16/18/22/26/30 ✅ |
| 安全路径 | TPM-P2-19（ReadSkillDirFiles 不安全路径）✅、TPM-P2-20（Multipart 限制不一致）✅ |
| 缺测试 | TPM-P2-29（mcp probe/health/alert）✅、skill 全套 ✅ |
| 代码卫生 | TPM-P2-04（configString 统一）✅ |
| 功能增强 | TPM-P2-17（job map TTL）✅、TPM-P2-23（RBAC）✅、TPM-P2-24（版本回滚）✅ |

### 4.2 Wave 2 包含的重设计

| ID | 主题 | 状态 |
|----|------|------|
| TPM-D-M1 | Transport 类型化 + ToConnectionConfig 委托 | ✅ |
| TPM-D-M2 | Probe 策略化（ProbeStrategy + AuthAwareProbe） | ✅ |
| TPM-P1-08 | skill Saga apply 补偿删除 | ✅ |
| TPM-P1-09 | mcp probe OAuth via TokenResolver | ✅ |

### 4.3 Wave 2 补充修复（Review / OOP / 测试 / 错误规范）

| 类别 | 完成项 |
|------|--------|
| Review 修复 | Review-1~9 + Review2-2~7（死代码删除 / DRY 违规 / 语义修复 / SSRF 防护 / TOCTOU 竞态 / 日志红线 / 死代码函数） |
| OOP 重构 | OOP-1（kanban Bridge 接口拆分）· OOP-2（skillruntime 窄接口）· OOP-3（serviceawaitreply 错误处理）· OOP-SKILL-01（importer 窄接口）· OOP-SKILL-02（SkillReader/Writer/Filesystem 子接口拆分） |
| 测试补全 | TST-SKILL-01~07（biz/skill 85 + storage 40 + skillruntime 61 + validate 53 + chat 22+ + skillrouter 6 + manifest 6） |
| 错误规范 | ERR-SKILL-01/02（importer sentinel error + detailError 类型） |
| 日志迁移 | LOG-SKILL-01（watch/ kratos/v2/log → FlowLog） |
| 安全合规 | SAFE-SKILL-01（watch/runner.go safego.Go 合规） |
| 杂项修复 | Fix-Misc / Fix-Misc-2 / Fix-Knowledge / Fix-Skill-01 |

---

## 5. Wave 3：架构升级（待启动）

| ID | 主题 | 子系统 | 依赖 |
|----|------|--------|------|
| TPM-D-P1 | Cost Guard Reservation Pattern（Prepare/Commit/Reconcile/Release） | plugin | — |
| TPM-D-P2 | Hook Isolation Layer（panic+timeout+bulkhead）全量版本 | plugin | — |
| TPM-D-P4 | Output Policy Streaming State Machine 全量版本 | plugin | — |
| TPM-D-T2 | Tool Effective Plan with Reason | tools | — |
| TPM-D-S1 | Skill Saga Import | skill | TPM-D-S3 |
| TPM-D-S3 | Skill Version Copy-on-Write + Rollback | skill | — |
| TPM-D-S4 | Skill Policy Trace | skill | — |
| §16.0 模式 A | Schema-as-Code（contract 包） | 跨子系统 | — |

> 各项的详细架构设计待启动时出独立 design 文档。

---

## 6. Wave 4：中长期愿景（待启动）

| ID | 主题 | 子系统 |
|----|------|--------|
| §16.0 模式 B | Event Sourcing（skill/mcp metadata 改事件流） | 跨子系统 |
| §16.0 模式 C | Policy Engine（OPA-lite） | 跨子系统 |
| TPM-D-M3 | MCP Server Lifecycle FSM | mcp |
| TPM-D-P3 | Plugin Scope Hierarchical | plugin |

> 各项的详细架构设计待启动时出独立 design 文档。

---

## 7. Skill 子系统剩余优化项

| ID | 优先级 | 主题 | 说明 |
|----|--------|------|------|
| SKILL-P2-01 | P2 | `internal/tools/` ~84 处 `fmt.Errorf` | 工具执行层错误，非业务错误，不经过 kerrors 链，低优先级 |
| SKILL-P2-02 | P2 | `SkillFileReader` 6 方法（略超 ≤5） | 可接受；SkillFilePathResolver 已提取独立子接口 |
| SKILL-P2-03 | P2 | `slugify("")` 固定生成 "skill-0" | 非唯一，需补全局唯一 slug 生成逻辑 |
| SKILL-P2-04 | P2 | `GetImportJob` 读操作用 `Lock()` | 改为 `RLock()` 提升并发读性能 |
| SKILL-P2-05 | P2 | `ApplyImport` 中文错误消息 | 统一为英文 sentinel error |
| SKILL-P2-06 | P2 | `watch.Runner` + `trpc.DBRepositoryAdapter` 依赖 `*biz.SkillUsecase` 具体类型 | 改用窄接口依赖 |

---

## 8. 依赖与风险

| 风险 | 等级 | 缓解 |
|------|------|------|
| Wave 1 P1-10 transport 改动影响多个 caller | 低 | 类型保持 alias `string`，逐步迁移；优先 NormalizeTransport 在 caller 侧统一 |
| Wave 1 P1-05 panic recover 改变错误传播语义 | 低 | recover → log + record + 返回 nil，与现有 resilient swallow 同语义 |
| Wave 1 P1-01 web_search alias 重定向影响生产 LLM 调用 | 中 | "runtime alias 删除"方案，保留 policy 端 alias，runtime alias 表只留确实需要的 |
| Wave 2 死配置删除破坏现有 DB row | 低 | schema 兼容删除（DB 字段保留，code 端忽略）；新 schema 不含字段 |

---

## 9. 文档约定

- 每个任务 PR 标题：`[TPM-P1-XX] {description}`
- 每个 PR 必须更新本文档对应任务状态（`📋 → 🚧 → ✅`）
- 重大架构改动（D 类）单独出 design 文档

---

## 10. 相关链接

- 并行计划：[56-business-logic-optimization.development.md](./56-business-logic-optimization.development.md)（M56 BLO）
- 框架边界：[AGENT_RUNTIME_BOUNDARY.md](../AGENT_RUNTIME_BOUNDARY.md)
- AI 编码规范：[AI-DEVELOPMENT-SPECIFICATION.md](../guides/AI-DEVELOPMENT-SPECIFICATION.md)

---

## 11. 执行进度表

| Wave | ID | 完成日期 | 备注 |
|------|----|---------|------|
| 1 | TPM-P1-02 | ✅ | aliasTool.Call → error |
| 1 | TPM-P1-11 | ✅ | mcp probe CheckRedirect（outboundguard.NewClient 已内置） |
| 1 | TPM-P1-07 | ✅ | skill zipslip filepath.Rel（ensurePathWithin） |
| 1 | TPM-P1-10 | ✅ | mcp Transport 类型化 + UnmarshalJSON 自动 normalize |
| 1 | TPM-P1-12 | ✅ | mcp ToConnectionConfig 唯一映射入口 + Env 补全 |
| 1 | TPM-P1-01 | ✅ | web_search alias 对齐 |
| 1 | TPM-P1-05 | ✅ | plugin chain panic recover + 回归测试 |
| 1 | TPM-P1-04 | ✅ | output_policy.onEvent block_on_violation + Delta.Content 修复 |
| 1 | TPM-P1-06 | ✅ | skill DBRepositoryAdapter Summary.Name=slug |
| 1 | TPM-P1-03 | ✅ | cost_guard fallback bypass daily budget + 回归测试 |
| 2 | TPM-P1-08 | 2026-05-26 | skill ApplyImport 补偿删除（createdSkillRecord + compensate） |
| 2 | TPM-P1-09 | 2026-05-26 | mcp probe 401/403 → auth_required，doHTTPProbe 旁路 |
| 2 | TPM-P2-01 | ✅ | skillruntime filter fail-open → fail-closed |
| 2 | TPM-P2-02 | ✅ | OpenAPI loader 错误 SysLogWarn |
| 2 | TPM-P2-03 | ✅ | claudefetch 返回 (nil, nil) 对齐 geminifetch |
| 2 | TPM-P2-04 | ✅ | ConfigString 统一到 tools 包 |
| 2 | TPM-P2-05 | ✅ | skillruntime cache filterCache 512 条上限 |
| 2 | TPM-P2-07 | 2026-05-28 | mcpobserve context.Background() → context.WithoutCancel |
| 2 | TPM-P2-08 | 2026-05-28 | SerpAPI key redactedURL + 注释警示 |
| 2 | TPM-P2-09 | ✅ | cost_guard admin_bypass 死配置删除 |
| 2 | TPM-P2-10 | ✅ | permission_guard confirm_tools/role_rules 死配置删除 |
| 2 | TPM-P2-11 | 2026-05-28 | audit_log.summarizeMessages 加 maybeRedact 脱敏 |
| 2 | TPM-P2-12 | 2026-05-28 | skill_usage_tracker input/output preview 加 redactText 脱敏 |
| 2 | TPM-P2-13 | 2026-05-28 | cost_guard AddTokens 写库失败不再静默吞错 |
| 2 | TPM-P2-14 | 2026-05-28 | retry_and_reflect global scope 加 1h TTL eviction |
| 2 | TPM-P2-16 | 2026-05-28 | dispatchHookOnEvent 失败策略与 chain hook 统一 |
| 2 | TPM-P2-17 | 2026-05-28 | Engine jobs map 加 2h TTL 过期清理 |
| 2 | TPM-P2-18 | 2026-05-28 | inspectSimilarity LLM 调用加上限 50 次 |
| 2 | TPM-P2-19 | 2026-05-28 | ReadSkillDirFiles 不安全路径改为返回错误 |
| 2 | TPM-P2-20 | 2026-05-28 | Multipart 25MB→20MB 与 engine MaxZipBytes 统一 |
| 2 | TPM-P2-21 | 2026-05-28 | skill DBRepositoryAdapter reload/loadBody 失败加 SysLogWarn |
| 2 | TPM-P2-22 | 2026-05-28 | watch/runner.go childWatches 并发写加 mu 保护 |
| 2 | TPM-P2-23 | 2026-05-29 | Skill RBAC：requireAdminAccess + applySkillPermission（admin/non-admin 权限掩码） |
| 2 | TPM-P2-24 | 2026-05-29 | Skill 版本回滚：RollbackSkillVersion（不可变策略 + patch 递增） |
| 2 | TPM-P2-25 | 2026-05-28 | alert.MarkHealthAlertEmitted 失败不再静默吞错 |
| 2 | TPM-P2-26 | 2026-05-28 | mcpobserve O(n) 全表扫 → GetMCPServerByKey 精确查询 |
| 2 | TPM-P2-27 | 2026-05-28 | OAuth refresh 失败不再 fallback 陈旧 access_token |
| 2 | TPM-P2-28 | 2026-05-28 | metadata 并发写 → UpdateMCPServerMetadata 只写 metadata+status 字段 |
| 2 | TPM-P2-29 | 2026-05-28 | probe + alert + config 单元测试补全 |
| 2 | TPM-P2-30 | 2026-05-28 | health probeAll semaphore bounded concurrency（max=8） |
| 2 | TPM-P3-12 | 2026-05-28 | mcp magic numbers 集中到 internal/mcp/defaults.go |
| 2 | TPM-P3-13 | 2026-05-28 | classify 前缀启发式评估后保留（误判风险极低） |
| 2 | TPM-D-M1 | 2026-05-28 | Transport 类型化 + ToConnectionConfig 委托 |
| 2 | TPM-D-M2 | 2026-05-28 | Probe 策略化：ProbeStrategy + AuthAwareProbe + ProbeMode + 28 个单测 |
| 2 | Review-1~9 | 2026-05-28 | 死代码删除 / DRY 违规 / 语义修复 / 日志红线 |
| 2 | Review2-2~7 | 2026-05-28 | SSRF 防护 / TOCTOU 竞态 / 随机淘汰 / 死代码函数 / 日志 / APIToken 遮蔽 |
| 2 | OOP-1~3 | 2026-05-28 | kanban Bridge 拆分 / skillruntime 窄接口 / serviceawaitreply 错误处理 |
| 2 | OOP-SKILL-01~02 | 2026-05-29 | importer 窄接口 / SkillReader/Writer/Filesystem 子接口拆分 |
| 2 | TST-SKILL-01~07 | 2026-05-29 | biz/skill 85 + storage 40 + skillruntime 61 + validate 53 + chat 22+ + skillrouter 6 + manifest 6 |
| 2 | ERR-SKILL-01~02 | 2026-05-29 | importer sentinel error + detailError 类型 |
| 2 | LOG-SKILL-01 | 2026-05-29 | watch/ 日志迁移 FlowLog |
| 2 | SAFE-SKILL-01 | 2026-05-29 | watch/runner.go safego.Go 合规 |
| 2 | Fix-Misc/Misc-2 | 2026-05-28~29 | SkillInvocationWrite 字段修复 / memory_l4_cascade 编译修复 |
| 2 | Fix-Knowledge | 2026-05-28 | hybrid_retriever.go 重复声明删除 |
| 2 | Fix-Skill-01 | 2026-05-29 | chat.go providerModelHasCredentials + resolveChatModel + json.Unmarshal 修复 |

---

## 12. 完成统计

### Wave 1+2 总体

| 类别 | 完成数 | 总数 | 完成率 |
|------|--------|------|--------|
| P1 关键路径 | 12 | 12 | 100% |
| P2 强化 | 30 | 30 | 100% |
| P3 维护性 | 2 | 13 | 15% |
| D 重设计 | 3 | 14 | 21% |
| Q 速胜 | 9 | 10 | 90% |
| Skill OOP/测试/错误 | 15 | 15 | 100% |

### 建议下一步优先级

1. **SKILL-P2-04**（`RLock` 改造）— 最小改动、即时收益
2. **SKILL-P2-05**（中文错误消息统一）— 小改动、代码卫生
3. **SKILL-P2-06**（窄接口依赖）— 架构合规、中等改动
4. **SKILL-P2-03**（slug 唯一性）— 功能正确性
5. **TPM-D-S3**（Skill Version CoW + Rollback）— Wave 3 关键路径
6. **TPM-D-S1**（Skill Saga Import）— 依赖 D-S3
