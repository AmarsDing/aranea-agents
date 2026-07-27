# 07 - P1 Delta 更新协议与归因观测 设计

> 模块：Phase 3 进化能力 / Harness RSI P1（协议层）
> 关联评审：[2026-07-25-review-harness-rsi-gap-analysis.md](../../reports/2026-07-25-review-harness-rsi-gap-analysis.md) §5 P1
> 前置实施：[06-P0-LLM-Curator与Reload接线.design.md](./06-P0-LLM-Curator与Reload接线.design.md)（已完成）
> 关联模块文档：[01-学习闭环.md](./01-学习闭环.md)、[02-技能自创建.md](./02-技能自创建.md)
> 状态：已实施（2026-07-26）

---

## 1. 背景与目标

P0 完成后，进化闭环已通电（观测 → LLM Curator → Gate → 审批 → Reload），但仍是「整体替换 + 无归因 + 聚合观测」形态。P1 在不动架构的前提下补齐协议层四个缺口：

| # | 缺口（评审 §3.2） | P1 对策 |
|---|------------------|---------|
| 1 | 整体替换 → Context Collapse 风险 | **Delta 更新协议**：规则块结构 + 操作序列局部更新 |
| 2 | 无规则级归因，无法回答「上次改进有没有用」 | **计数归因**：规则块 helpful/harmful 计数 + 改进有效性裁决 + Gate 历史有效性维度 |
| 3 | 观测是聚合指标不是原始 trace | **trace 级观测**：Curator prompt 注入近期失败经验报告片段 |
| 4 | Solve 未接线（无真实任务执行验证） | **Solve 回放**：evaluation 数据集回放作为 Gate 功能验证维度 |

P3（Meta-Harness / STOP）按评审结论仅以 ADR 归档，不实施。

## 2. 非目标

- 不改 UnifiedEvolutionSuggestion 存储结构（归因/基线/delta 均走 metadata JSON 键）
- 不改 skill 版本不可变性（计数更新发生在新版本注册时的正文生成环节，不就地改写旧版本）
- 不接 agent 级 evolution（`evolution.go` 的 persona 整体替换路径本次不动，delta 协议仅作用于 skill 路径）
- 不做完整 agent runtime 级 Solve（数据集回放走平台 LLM 直接执行，不经 agent 编排）

## 3. Delta 更新协议

### 3.1 规则块格式（Rule Block）

SKILL.md 正文中，可操作规则用 HTML 注释标记包裹：

```markdown
<!-- aranea:rule id="timeout-retry" helpful=3 harmful=1 -->
当工具调用超时时，先指数退避重试一次，再降级到备选方案。
<!-- /aranea:rule -->
```

- `id`：规则固定 ID（kebab-case，同一正文内唯一），跨版本保持稳定——归因的锚点
- `helpful`/`harmful`：归因计数器，缺省为 0；解析后原样回写，普通 Markdown 渲染器将标记视为注释不可见
- 规则块之外的正文（标题、说明、frontmatter）为**非规则段**，delta 更新不触碰

### 3.2 操作序列（Delta Ops）

Curator 在 delta 模式下只输出 JSON 操作序列，由程序执行局部更新：

```json
[
  {"op": "modify", "rule_id": "timeout-retry", "content": "当工具调用超时时，最多重试两次（指数退避），随后降级。"},
  {"op": "add", "rule_id": "rate-limit-backoff", "content": "遇到 429 时休眠 Retry-After 指定的秒数。"},
  {"op": "remove", "rule_id": "legacy-fallback"},
  {"op": "merge", "rule_id": "timeout-retry", "content": "重试间隔上限 30 秒。"}
]
```

| op | 语义 | 目标规则必须存在？ |
|----|------|-------------------|
| `add` | 文末追加新规则块（ID 不得与现有重复） | 否（必须不存在） |
| `modify` | 替换规则内容（保留计数器） | 是 |
| `merge` | 将内容追加到现有规则内容末尾（保留计数器） | 是 |
| `remove` | 删除整个规则块 | 是 |

**严格性**：任何 op 引用不存在的 rule_id（或 add 重复 ID）→ 整个 delta 拒绝，返回错误 → 调用方回退到全量重写模式。操作按数组顺序依次应用。

### 3.3 解析器与应用器（`internal/biz/skill_delta_protocol.go`）

```go
// RuleBlock 是一条带固定 ID 与归因计数的规则块。
type RuleBlock struct {
    ID      string
    Helpful int
    Harmful int
    Content string
}

// RuleDocument 是解析后的正文：非规则段（segments）与规则块（rules）保序交错。
type RuleDocument struct { /* segments []string, rules []*RuleBlock, order []elem */ }

func ParseRuleBlocks(body string) (*RuleDocument, error)
func HasRuleBlocks(body string) bool
func (d *RuleDocument) Render() string  // 保序回写，计数器回写进标记

type DeltaOp struct {
    Op      string `json:"op"`       // add | modify | merge | remove
    RuleID  string `json:"rule_id"`
    Content string `json:"content"`
}

func ParseDeltaOpsJSON(text string) ([]DeltaOp, error) // 容忍 ```json 围栏
func ApplyDeltaOps(doc *RuleDocument, ops []DeltaOp) (changedIDs []string, err error)
```

### 3.4 Curator 双模式（`llm_skill_evolver.go` 改造）

```
EvolveDraft:
  currentBody = GetLatestSkillMarkdown(skillID)
  if HasRuleBlocks(currentBody):
      → delta 模式：system prompt 要求输出 JSON ops
      → ParseDeltaOpsJSON → ParseRuleBlocks(currentBody)
      → 归因计数归账（见 §4.3）→ ApplyDeltaOps → doc.Render() 即 draft
      → delta JSON 存入 input.DeltaOpsOut（供建议 metadata 落账）
  else:
      → 全量重写模式（现状行为），system prompt 新增指令：
        「可操作规则必须用 <!-- aranea:rule --> 块包裹并给出语义化 id」
        —— 使下一次进化周期自动进入 delta 模式（有机迁移，无需存量改写）
```

评审风险表「首版协议兼容整体替换（无规则 ID 时降级为全量，标记 Warn）」→ 全量重写模式即该降级路径，delta 解析/应用失败时 Warn 日志并回退全量。

## 4. 计数归因

### 4.1 基线记录（触发时）

`CheckEvolutionTriggers` 创建建议时已计算 7d 成功率（`metrics7d.SuccessRate`）。新增 metadata 键随建议落库：

| metadata 键 | 内容 |
|-------------|------|
| `baseline_success_rate` | 触发时 7d 成功率（无 7d 数据时用 30d 值） |
| `delta_ops` | 本次 draft 实际应用的 delta JSON（仅 delta 模式） |
| `effectiveness` | 归因裁决：`helpful` / `harmful` / `neutral` / `insufficient_data`（由下一周期回写） |

### 4.2 有效性裁决（`internal/biz/skill_attribution.go`）

新一周期 draft 生成前，对**最近一次 applied** 建议裁决：

```go
type EvolutionAttribution struct {
    Verdict             string   // helpful | harmful | neutral | insufficient_data | ""
    BaselineSuccessRate float64
    CurrentSuccessRate  float64
    AffectedRuleIDs     []string // 上一次 applied delta 触碰的规则 ID
}

func (uc *SkillIntelligenceUsecase) AttributeLastEvolution(ctx context.Context, skillID string) *EvolutionAttribution
```

流程：
1. unifiedStore 取该 skill 最近一条 `status=applied` 建议（无 → nil，首次进化）
2. 读 `baseline_success_rate`（缺 → nil，无法裁决）
3. `aggregator.GetHealthMetrics(skillID, since=appliedAt)` 得改进后成功率；调用次数 < 5 → `insufficient_data`
4. Δ ≥ +5pp → helpful；Δ ≤ −5pp → harmful；其余 → neutral
5. 裁决经 `UpdateMetadataKey(effectiveness)` 回写到该 applied 建议（幂等，已裁决过则跳过）
6. 返回裁决 + 从该建议 `delta_ops` 解析出的 AffectedRuleIDs（全量模式为空）

### 4.3 计数归账（delta 应用时）

delta 模式下，`ApplyDeltaOps` 前：若 `input.Attribution.Verdict == helpful` → 将 `AffectedRuleIDs` 中仍存在规则的 `helpful+1`；`harmful` → `harmful+1`。计数随新正文（Render 产物）落库为新版本内容——**不就地改写旧版本**，版本不可变性保持。

### 4.4 Gate 第五维：历史改进有效性

`GateVerifier` 新增可选维度 `effectiveness`（options 注入 `SkillLookupReader` 后启用）：

- 取当前正文规则计数；凡 `harmful >= 3`（`GateHarmfulRuleRejectThreshold`）的规则，若 draft **原样保留其内容**（同 ID 且内容未实质变化）→ 拒绝：「规则 X 连续 3 次改进无效，禁止原样保留，须重写或移除」
- draft 重写或移除该规则 → 通过；无有害规则 → 通过
- 未注入 reader / 正文无规则块 → 跳过（通过），nil-safe

> **语义备注（一周期滞后）**：本维度读取的是**当前已发布版本**的计数，不含本周期 evolver 内刚完成的归账 bump。规则 harmful=2 且本周期再判 harmful（归账后达 3）时，本周期 Gate 不拒绝，下一周期才触发。这是「基于已结算计数裁决」的有意语义，避免对未落账数据做判断。

## 5. trace 级观测

`SkillDraftInput` 新增：

```go
type TraceSnippet struct {
    FailureTags       []string
    FlowSummary       string
    RootCauseAnalysis string
    CreatedAt         time.Time
}

type SkillDraftInput struct {
    SkillID       string
    SuggestType   EvolutionSuggestionType
    TriggerReason string
    Attribution   *EvolutionAttribution // nil = 无历史
    Traces        []TraceSnippet        // nil/空 = 无 trace 证据
    DeltaOpsOut   *[]DeltaOp            // delta 模式时回写实际应用的 ops（供 metadata 落账）
}
```

`generateDraft`（usecase）：draft 前取该 skill 最近 10 条经验报告，筛 `IsSuccess=false` 取最新 3 条组装 Traces。Curator user prompt 新增「## 近期失败轨迹」段落（FlowSummary + FailureTags + RootCauseAnalysis），替代「只有聚合指标」的现状——侦探拿到现场证据。LLMSkillEvolver 无新依赖（trace 由 usecase 组装经 input 传入，保持 evolver 纯粹）。

## 6. Solve 接线（数据集回放进 Gate）

### 6.1 新 port（biz）

```go
// SkillReplayRunner 用 evaluation 数据集对 draft 做真实任务回放（Solve 阶段）。
// Stability:evolving
type SkillReplayRunner interface {
    Replay(ctx context.Context, skillID string, draftBody string, maxCases int) (*SkillReplayResult, error)
}

type SkillReplayResult struct {
    DatasetID   string
    DatasetName string
    Total       int
    Passed      int
    PassRate    float64
}
```

### 6.2 生产实现（service 层 `skill_replay_runner.go`）

- **数据集寻址约定**：`ListDatasets(workspace="")` 中名称 == skill 的 Name 或 Slug 者即该 skill 的评测集（skill 经 `SkillQueryReader` 查询）；找不到 → 返回 `ErrNoReplayDataset`（Gate 视为跳过，不阻断）
- **回放执行**：取前 `maxCases`（=5）条 case，逐条以 draft 正文为 system、case.Input 为 user 调 DefaultRefineLLM（单次 30s 超时，对齐 evolver 先例）
- **评分**：contains-match（大小写不敏感子串包含 ExpectedOutput）；pass rate ≥ 0.6（`ReplayPassThreshold`）为通过
- LLM 未配置 → 返回错误，Gate 按「回放不可用跳过」处理（Warn 日志，不阻断）——与项目 best-effort 降级风格一致

### 6.3 Gate 功能维升级

`GateVerifier` 经 options 注入 `SkillReplayRunner` 后，功能维在 sandbox 通过后追加回放检查：

```
functional 维度 = sandbox 通过 AND (replay 跳过 OR replay pass_rate >= 0.6)
```

拒绝原因含数据集名与通过率（如 `dataset replay pass rate 40% < 60% (dataset=timeout-cases, 2/5)`）。

### 6.4 GateVerifier 构造改造

`NewGateVerifier(sandboxRunner, lintChecker)` 保持原签名，新增 variadic options：

```go
type GateOption func(*GateVerifier)
func WithReplayRunner(r SkillReplayRunner) GateOption
func WithSkillLookup(r SkillLookupReader) GateOption // effectiveness 维依赖
```

`biz.ProvideSkillMergeGateVerifier` 从 biz ProviderSet 移除，wire.go 新增 provider 组装含 options 的 GateVerifier（注入 sandboxRunner + replayRunner + skillRepo）。DI 环检查：replayRunner(service) → evaluation.Usecase + LLMCaller + skillRepo，不经 SkillIntelligenceUsecase，无新环。

## 7. 数据流总图（P1 完成后）

```
CheckEvolutionTriggers ── 落库 baseline_success_rate ──┐
                                                        ▼
RunCuratorFlow / 审批异步:  AttributeLastEvolution（裁决上次 applied，回写 effectiveness）
                            generateDraft(input{Attribution, Traces, DeltaOpsOut})
                              ├─ delta 模式（有规则块）：LLM ops → 计数归账 → ApplyDeltaOps → Render
                              └─ 全量模式（无规则块）：LLM 全文（指令包裹规则块）
                            落库 delta_ops（delta 模式）
                            Gate 五维：functional(sandbox+数据集回放) / security / performance / style / effectiveness
                            审批 → Reload 注册新版本（规则计数随正文持久化）
```

## 8. 改动文件清单

| 文件 | 改动 |
|------|------|
| `internal/biz/skill_delta_protocol.go` | 新增：规则块解析/渲染、delta ops 解析/应用 |
| `internal/biz/skill_attribution.go` | 新增：EvolutionAttribution + AttributeLastEvolution |
| `internal/biz/llm_skill_evolver.go` | 改造：双模式 + trace/归因注入 prompt |
| `internal/biz/skill_evolution_loop.go` | 改造：SkillReplayRunner port、GateOption、Gate 第五维 + 功能维回放 |
| `internal/biz/skill_intelligence.go` | 改造：generateDraft 组装 Attribution/Traces/落 delta_ops；CheckEvolutionTriggers 落 baseline |
| `internal/biz/skill_evolution_unified.go` | 新增 3 个 EvoMeta 键常量 |
| `internal/biz/biz.go` | ProviderSet 移除 ProvideSkillMergeGateVerifier |
| `internal/service/skill_replay_runner.go` | 新增：数据集回放实现 |
| `cmd/admin/wire.go` | GateVerifier provider（含 options）、replay runner provider |
| 各 `_test.go` | delta 协议 / 归因 / evolver 双模式 / Gate 新维度 / replay runner |

## 9. 验证方案

- 单测：解析器（含畸形输入回退）、四种 op + 严格性错误、计数归账、归因裁决四象限 + insufficient_data 守卫、Gate effectiveness 维、replay contains-match 评分、双模式 evolver
- `go build ./...` + `go test ./internal/biz/... ./internal/data/... ./internal/service/...` 全绿
