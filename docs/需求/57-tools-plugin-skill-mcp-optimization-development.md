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
| Wave 2 P2 实施 | ✅ 大部分完成（P2-01~05/07/09/10/25/26/27/28/29/30 + P3-12/13 + D-M1 已完成） |
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
| 安全 | TPM-P2-08（SerpAPI key）、TPM-P2-11/12（PII 泄露）、TPM-P2-27（OAuth stale token） |
| 死配置 | TPM-P2-03/09/10（claudefetch / admin_bypass / confirm_tools / role_rules） ✅ |
| 静默吞错 | TPM-P2-01（skill fail-open） ✅、TPM-P2-02（OpenAPI loader） ✅、TPM-P2-13（cost DB）、TPM-P2-21（reload）、TPM-P2-25（alert） |
| 性能 | TPM-P2-05（cache eviction） ✅、TPM-P2-14/15/18/26/30 |
| 缺测试 | TPM-P2-29（mcp probe/health/alert）、skill 全套 |
| 代码卫生 | TPM-P2-04（configString 统一） ✅ |

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
| 2 | Review-1 | 2026-05-28 | — | 删除 ToTRPCConnectionConfig 死代码，消除双 source of truth |
| 2 | Review-2 | 2026-05-28 | — | cost_guard TryConsume 安全网语义注释 |
| 2 | Review-3 | 2026-05-28 | — | cost_guard fallback bypass daily_budget 设计意图注释 |
