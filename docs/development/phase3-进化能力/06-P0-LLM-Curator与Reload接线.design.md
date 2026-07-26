# P0 设计：LLM Curator + Reload 接线（进化反思者与应用闭环）

> 类型：设计文档（phase3-进化能力 · 增量）
> 依据：[2026-07-25-review-harness-rsi-gap-analysis.md](../../reports/2026-07-25-review-harness-rsi-gap-analysis.md) §5 P0
> 关联模块文档：[01-学习闭环.md](./01-学习闭环.md)、[02-技能自创建.md](./02-技能自创建.md)
> 状态：已实施（2026-07-26，P0 完成：LLM Curator + Reload 接线 + 审批后触发点 + Wire 注入，build/biz/data/service 测试全绿）

---

## 1. 背景与目标

评审确认：进化管线的调度层（LearningLoopScanner/CuratorWorker）与治理底座（Orchestrator/Gate/回滚/状态机）均已通电，但存在两个断点：

1. **反思者缺位**：全链路无 LLM 参与改进内容生成——`generateRuleBasedDraft` 输出固定模板（注释自承 "rule-based (v1)"），两个调用点（`RunCuratorFlow` step 2、审批后异步 `GenerateDraftForSuggestion`）都是模板
2. **应用闭环断开**：skill 审批通过且 lifecycle=ready 后，无「注册新版本」的生产路径（`SkillReloader` port 无生产实现）

**目标**：补两个生产实现（纯接线，不改架构），使进化闭环达到「观测 → LLM 反思 → Gate 验证 → 人工审批 → 注册新版本（可回滚）」的完整 ACE 形态。

**非目标**（P1+，本设计不覆盖）：delta 更新协议、规则计数归因、trace 级观测、Solve 阶段（数据集回放）、Agent 级自动触发、Meta-Harness。

## 2. 现状数据流

```
CuratorWorker (2h ticker, 日上限20)
  → SkillIntelligenceUsecase.RunCuratorFlow(skillID)
      step1 触发检测（7d 成功率/同标签失败次数）
      step2 generateRuleBasedDraft()          ← 【断点1】模板，无 LLM
      step3 lifecycle → validating
      step4 GateVerifier 四维验证 / rule-based sandbox
      step5 lifecycle → ready / 停留 draft

审批 API (ApproveSkillEvolutionSuggestion)
  → 状态 → approved
  → 异步 GenerateDraftForSuggestion           ← 仍走 generateRuleBasedDraft【断点1】
  → 异步 ValidateSuggestion
  → 【终止】无注册新版本动作                    ← 【断点2】Reload 未接线
```

## 3. 设计一：LLM Curator（SkillEvolver 生产实现）

### 3.1 复用的既有资产

| 资产 | 位置 | 说明 |
|------|------|------|
| `SkillEvolver` port | `internal/biz/skill_evolution_loop.go:103` | `Evolve(ctx, skillID, report) (string, error)`，已定义无需改 |
| `biz.LLMCaller` + `LLMCallerAdapter` 模式 | `internal/skill/auto_creator.go:19-40` | 平台 LLM 调用的既有先例（DefaultRefineLLM 解析 provider/model 凭证） |
| `generateRuleBasedDraft` | `internal/biz/skill_intelligence.go:557` | 保留为 fallback，不删除 |
| `SkillLookupReader.GetLatestSkillMarkdown` | `internal/biz/skill/skill.go:178` | 取当前 skill body 注入 prompt |

### 3.2 新组件：LLMSkillEvolver

归属：`internal/biz`（与 SkillIntelligenceUsecase 同包，避免跨层依赖；LLM 调用经 `biz.LLMCaller` 抽象，不直连 provider）。

```go
// LLMSkillEvolver 基于 LLM 的 SkillEvolver 生产实现。
// LLM 不可用/超时/输出非法时，调用方负责回退 rule-based（见 3.4 统一入口）。
type LLMSkillEvolver struct {
    caller     biz.LLMCaller        // 平台 LLM 抽象（同 auto_creator）
    skills     skill.SkillLookupReader // 取当前 body
    provider   string
    model      string
    lg         loggateway.Logger
}
```

**Prompt 组装**（输入全部来自既有数据，无新采集）：
- 当前 skill body（`GetLatestSkillMarkdown`）
- 触发原因 `triggerReason`（如 "7d 成功率 45% < 60%"）
- 失败标签分布 `matchedTags`（如 `map[timeout:6 parse_error:3]`）
- 聚合指标（成功率/调用次数，来自触发检测已算好的值）

**输出校验**（程序侧，前置过滤）：
1. 非空
2. 长度 ≤ `GateMaxDraftLength`（10000，与 Gate 风格维度一致）
3. 含 Markdown 基本结构（`# ` 标题）

校验失败视为 LLM 输出非法 → 回退 rule-based。

### 3.3 超时与成本

| 项 | 值 | 依据 |
|----|-----|------|
| LLM 调用超时 | 30s | 对齐 MarkdownOrganizer 既有先例 |
| 日调用上限 | 沿用 `CURATOR_DAILY_MAX=20` | CuratorWorker 已有硬顶，LLM 不增加新成本面 |
| 审批后再生成 | 同一 evolver，复用同一超时 | `GenerateDraftForSuggestion` 路径 |

### 3.4 统一 draft 入口（消重）

`SkillIntelligenceUsecase` 新增私有方法，两个调用点统一收口：

```go
// generateDraft 统一 draft 生成入口：优先 LLM evolver，失败回退 rule-based。
// best-effort 语义：LLM 失败不阻断流程，仅 Warn 日志 + 回退。
func (uc *SkillIntelligenceUsecase) generateDraft(ctx context.Context, skillID, triggerReason string, matchedTags map[string]int) (string, error)
```

Usecase 新增依赖字段 `evolver SkillEvolver`（构造注入，nil-safe：nil 时直接走 rule-based）。`RunCuratorFlow` step 2 与 `GenerateDraftForSuggestion` 改为调用 `generateDraft`。

## 4. 设计二：Reload 接线（SkillReloader 生产实现）

### 4.1 复用的既有资产

| 资产 | 位置 | 说明 |
|------|------|------|
| `SkillReloader` port | `internal/biz/skill_evolution_loop.go:113` | `Reload(ctx, skillID, draftBody, parentVersionID, evolutionReason) error` |
| `skill_version` 表 | `internal/data/ent/schema/skill_version.go:36-37` | `parent_version_id`/`evolution_reason` 列**已存在，零 DDL 变更** |
| `RollbackSkillVersion` | `internal/biz/skill/skill.go:208` | 回滚能力已有，天然满足「可回滚」 |
| `UnifiedEvolutionStateMachine` | `internal/biz/skill_evolution_unified.go` | applied 状态已存在，无需扩展状态机 |
| 异步存活模式 | 审批后 `context.WithoutCancel`（B-14 修复） | Reload 沿用同一模式 |

### 4.2 新增 repo 窄接口：SkillVersionWriter

现有 `SkillMutationWriter` 已满 5 方法（DB-N3 上限），故新建窄接口，`skill.Repo` 组合：

```go
// SkillVersionWriter 追加 skill 新版本（进化 Reload 路径）。
// Stability:evolving
type SkillVersionWriter interface {
    // CreateSkillVersion 为已有 skill 追加新版本并切换 current 指针（事务原子）。
    CreateSkillVersion(ctx context.Context, in CreateVersionInput) (SkillVersionDetail, error)
}

type CreateVersionInput struct {
    SkillID          string
    Body             string
    ParentVersionID  string // 当前版本 ID（= 快照，回滚锚点）
    EvolutionReason  string
}
```

data 层实现要点：
- 版本号：当前版本 patch+1（semver 末位自增，与既有版本格式保持一致——实施时确认 `skill_version.version` 现有格式）
- 事务：`Data.ExecInTx`（既有统一入口），同一事务内 insert version 行 + 更新 platform_skill current 指针
- 错误翻译：`entErrToBizErr`（DB-R5）

### 4.3 新组件：SkillVersionReloader

归属：`internal/biz`。

```go
// SkillVersionReloader 是 SkillReloader 的生产实现：注册新版本并切换 current。
// 旧版本经 ParentVersionID 保留，回滚走既有 RollbackSkillVersion。
type SkillVersionReloader struct {
    versions skill.SkillVersionWriter
    skills   skill.SkillLookupReader
    lg       loggateway.Logger
}
```

`Reload` 流程：
1. 取当前版本 ID（parent 锚点；取不到 → 报错，不静默继续）
2. `CreateSkillVersion`（事务内追加 + 切换）
3. 成功 → Info 日志（skill_id / parent_version_id）

### 4.4 触发点与状态流转

审批通过 **且** lifecycle=ready **且** sandbox_passed → 异步 Reload（`context.WithoutCancel`，与审批后 GenerateDraft 同模式）。

```
pending → approved → [异步] GenerateDraft(LLM) → validating → Gate → ready
                                                                    ↓
                                              [异步] Reload → applied（状态机转换）
```

- Reload 失败：仅 Error 日志，suggestion 停留 ready（人工可重试，不自动重试——避免重复注册版本）
- Reload 成功：`UpdateStatus(id, "applied", ...)`，经 `UnifiedEvolutionStateMachine` 校验转换合法性

## 5. 依赖与 Wire

| 新组件 | 注入位置 | 依赖 |
|--------|---------|------|
| `LLMSkillEvolver` | `SkillIntelligenceUsecase` 新字段 `evolver` | `biz.LLMCaller`、`skill.SkillLookupReader`、provider/model 配置 |
| `SkillVersionReloader` | 审批服务路径（`SkillEvolutionSuggestionService` 底层 usecase） | `skill.SkillVersionWriter`、`skill.SkillLookupReader` |

均为构造注入、nil-safe（nil 时行为 = 现状：rule-based / 不 Reload），保证未配置环境下行为不回归。

## 6. 安全与治理（不变量）

以下既有治理全部保持不变，本设计不削弱任何一项：

- 人工审批是终态门：LLM 只影响 draft 质量，不绕过审批
- Gate 四维验证：LLM draft 必须过 Gate 才能 ready（含 security 维度敏感信息检测）
- 冷却期 / pending 去重 / DB UNIQUE：Orchestrator 层不变
- 日上限 20 次：LLM 成本硬顶
- 快照回滚：旧版本经 ParentVersionID 保留，`RollbackSkillVersion` 已有 API

## 7. 风险

| 风险 | 缓解 |
|------|------|
| LLM 输出漂移（改丢原 skill 关键约束） | 输出校验 + Gate + 人工审批三道；P1 delta 协议根治 |
| Reload 与人工编辑并发 | 版本追加走事务；current 指针切换由 DB 行级锁串行化 |
| 审批时无当前版本（parent 锚点缺失） | Reload step1 显式报错，不注册孤儿版本 |

## 8. 验证方案（实施时 TDD）

| 层 | 测试 | 要点 |
|----|------|------|
| 单测 | `LLMSkillEvolver` | mock LLMCaller：成功 / 超时→回退 / 输出非法→回退 / prompt 组装含失败标签 |
| 单测 | `SkillVersionReloader` | 版本追加 + parent 链正确 + 事务原子（current 切换失败则版本行不落） |
| 单测 | `generateDraft` 统一入口 | evolver=nil 走 rule-based；evolver 失败回退 |
| 集成 | RunCuratorFlow 端到端 | 注入 mock evolver，验证 draft→Gate→ready 链路 |
| 回归 | 现有 biz 测试 | rule-based fallback 路径全绿 |

验证命令：`go test ./internal/biz/... -count=1` + `go build ./...`（Windows 环境用 go build，不用 make build）。
