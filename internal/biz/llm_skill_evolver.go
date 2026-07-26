package biz

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// evolverLLMTimeout 单次 LLM draft 生成的硬超时（对齐 MarkdownOrganizer 30s 先例）。
const evolverLLMTimeout = 30 * time.Second

// TraceSnippet 是一条近期失败经验报告的精简片段（trace 级观测证据）。
// 由 usecase 在 draft 生成前组装，经 SkillDraftInput 传给 evolver——侦探
// 拿到现场证据，而不是只有聚合指标。
type TraceSnippet struct {
	FailureTags       []string
	FlowSummary       string
	RootCauseAnalysis string
	CreatedAt         time.Time
}

// SkillDraftInput carries the evolution context available at draft-generation
// call sites (suggestion-shaped, not observation-report-shaped).
type SkillDraftInput struct {
	SkillID       string
	SuggestType   EvolutionSuggestionType
	TriggerReason string
	// Attribution 是最近一次 applied 进化的有效性裁决（nil = 无历史/无法裁决）。
	Attribution *EvolutionAttribution
	// Traces 是近期失败经验报告片段（nil/空 = 无 trace 证据）。
	Traces []TraceSnippet
	// DeltaOpsOut 非 nil 时，delta 模式下实际应用的 ops 会回写到该指针
	// （供调用方落 delta_ops metadata 账）。
	DeltaOpsOut *[]DeltaOp
}

// SkillDraftEvolver generates an improved skill draft from evolution context.
// Implementations must return an error on any failure so callers can fall back
// to the rule-based template (best-effort semantics).
//
// Stability:evolving
type SkillDraftEvolver interface {
	EvolveDraft(ctx context.Context, in SkillDraftInput) (string, error)
}

// LLMSkillEvolver is the production SkillDraftEvolver backed by the platform
// LLM (biz.LLMCaller). It fills the Curator role: reflects on the current
// skill body plus the trigger evidence and rewrites the skill.
//
// 双模式（P1 Delta 更新协议）：
//   - 当前正文含规则块 → delta 模式：LLM 只输出操作序列 JSON，程序执行局部
//     更新（计数归账 → ApplyDeltaOps → Render），避免整体替换的 Context
//     Collapse。
//   - 无规则块 → 全量重写模式：LLM 输出完整正文，system prompt 要求可操作
//     规则用规则块标记包裹，使下一周期自动进入 delta 模式（有机迁移）。
//   - delta 解析/应用失败 → Warn 日志并回退全量重写（协议降级路径）。
type LLMSkillEvolver struct {
	caller   LLMCaller
	skills   SkillLookupReader
	provider string
	model    string
	lg       loggateway.Logger
}

// NewLLMSkillEvolver constructs an LLMSkillEvolver. provider/model come from
// the platform DefaultRefineLLM setting (same resolution as SkillAutoCreator).
func NewLLMSkillEvolver(caller LLMCaller, skills SkillLookupReader, provider, model string, lg loggateway.Logger) *LLMSkillEvolver {
	return &LLMSkillEvolver{
		caller:   caller,
		skills:   skills,
		provider: provider,
		model:    model,
		lg:       lg,
	}
}

// EvolveDraft generates an improved SKILL.md body via LLM. Any failure
// (body fetch, LLM call, output validation) returns an error — the caller
// is expected to degrade to the rule-based template.
func (e *LLMSkillEvolver) EvolveDraft(ctx context.Context, in SkillDraftInput) (string, error) {
	if in.SkillID == "" {
		return "", apierror.BadRequest("LLM_EVOLVER", "skill_id is required")
	}

	// 现场证据：当前 skill body 是反思的基础，取不到则不浪费 LLM 调用。
	currentBody, err := e.skills.GetLatestSkillMarkdown(ctx, in.SkillID)
	if err != nil {
		return "", apierror.Wrap(err, apierror.CodeInternal, "LLM_EVOLVER")
	}

	if HasRuleBlocks(currentBody) {
		draft, dErr := e.evolveDelta(ctx, in, currentBody)
		if dErr == nil {
			return draft, nil
		}
		// 协议降级：delta 解析/应用失败回退全量重写。
		e.lg.Warn("LLMSkillEvolver: delta mode failed, falling back to full rewrite",
			loggateway.StepID("llm_evolver.delta"),
			loggateway.Str("skill_id", in.SkillID),
			loggateway.Err(dErr))
	}
	return e.evolveFullRewrite(ctx, in, currentBody)
}

// evolveDelta runs the delta update protocol: LLM produces an op sequence,
// the program settles the previous cycle's attribution verdict onto the
// touched rules, applies the ops locally, and renders the result.
func (e *LLMSkillEvolver) evolveDelta(ctx context.Context, in SkillDraftInput, currentBody string) (string, error) {
	text, err := e.callLLM(ctx, evolverDeltaSystemPrompt, buildEvolverUserPrompt(in, currentBody))
	if err != nil {
		return "", err
	}
	ops, err := ParseDeltaOpsJSON(text)
	if err != nil {
		return "", err
	}
	doc := ParseRuleBlocks(currentBody)
	// 计数归账：把上一周期裁决记到它触碰的规则上（在应用新 ops 之前，
	// 确保归账基于 ops 修改前的规则集合）。
	if in.Attribution != nil {
		BumpRuleCounters(doc, in.Attribution.AffectedRuleIDs, in.Attribution.Verdict)
	}
	if _, err := ApplyDeltaOps(doc, ops); err != nil {
		return "", err
	}
	if in.DeltaOpsOut != nil {
		*in.DeltaOpsOut = ops
	}
	draft := doc.Render()
	if len(draft) > GateMaxDraftLength {
		return "", apierror.BadRequest("LLM_EVOLVER", "draft exceeds max length %d", GateMaxDraftLength)
	}
	return draft, nil
}

// evolveFullRewrite is the legacy whole-body rewrite path (also the delta
// fallback). The system prompt instructs the LLM to wrap actionable rules in
// rule-block markers so the next cycle enters delta mode organically.
func (e *LLMSkillEvolver) evolveFullRewrite(ctx context.Context, in SkillDraftInput, currentBody string) (string, error) {
	text, err := e.callLLM(ctx, evolverSystemPrompt, buildEvolverUserPrompt(in, currentBody))
	if err != nil {
		return "", err
	}
	return validateEvolverOutput(text)
}

// callLLM performs a single completion with the evolver timeout.
func (e *LLMSkillEvolver) callLLM(ctx context.Context, system, user string) (string, error) {
	llmCtx, cancel := context.WithTimeout(ctx, evolverLLMTimeout)
	defer cancel()
	text, _, callErr := e.caller.Call(llmCtx, LLMCallRequest{
		Provider: e.provider,
		Model:    e.model,
		System:   system,
		User:     user,
	})
	if callErr != nil {
		e.lg.Warn("LLMSkillEvolver: LLM call failed",
			loggateway.StepID("llm_evolver.call"),
			loggateway.Err(callErr))
		return "", apierror.Wrap(callErr, apierror.CodeInternal, "LLM_EVOLVER")
	}
	return text, nil
}

const evolverSystemPrompt = `你是 Skill 进化策展人（Curator）。你的职责是根据运行观测证据改进 Agent 的 SKILL.md。

规则：
1. 输出必须是改进后的完整 SKILL.md 正文，不要输出任何解释、前缀或后缀。
2. 保持原文的语言（中文原文输出中文，英文原文输出英文）。
3. 保留原文的 Markdown 结构与 frontmatter（如有），只做针对性改进。
4. 改进必须针对触发原因中描述的问题，不要无关改写。
5. 不要删除原文中仍然有效的约束、触发条件与失败处理说明。
6. 可操作规则必须用规则块标记包裹并给出语义化 id（kebab-case），格式：
   <!-- aranea:rule id="rule-id" -->
   规则内容
   <!-- /aranea:rule -->
   已有规则块的 id 必须原样保留；规则块之外的说明性正文不要使用该标记。`

const evolverDeltaSystemPrompt = `你是 Skill 进化策展人（Curator）。当前 SKILL.md 采用规则块协议管理，每个规则块形如：

<!-- aranea:rule id="rule-id" helpful=N harmful=M -->
规则内容
<!-- /aranea:rule -->

你的职责：根据运行观测证据，只输出一个 JSON 操作序列对规则块做局部更新，不要输出完整正文，不要输出任何解释。

允许的操作（按数组顺序依次应用）：
- {"op": "modify", "rule_id": "...", "content": "..."}  替换规则内容（计数器保留）
- {"op": "add", "rule_id": "...", "content": "..."}     追加新规则（rule_id 不得与现有重复）
- {"op": "merge", "rule_id": "...", "content": "..."}   将内容追加到现有规则末尾
- {"op": "remove", "rule_id": "..."}                    删除规则

严格约束：
1. modify/merge/remove 的 rule_id 必须引用现有规则；add 的 rule_id 必须是新 id（kebab-case）。
2. 改进必须针对触发原因与失败轨迹中描述的问题，不要无关改写。
3. helpful/harmful 计数高的规则是历史上有效/无效的规则：harmful 计数 >= 3 的规则应重写或移除，不得原样保留。
4. 输出必须是合法 JSON 数组，不要使用 code fence。`

func buildEvolverUserPrompt(in SkillDraftInput, currentBody string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## 进化类型\n%s\n\n## 触发原因（运行观测证据）\n%s\n", in.SuggestType, in.TriggerReason)

	if in.Attribution != nil && in.Attribution.Verdict != "" {
		fmt.Fprintf(&b, "\n## 上一次改进的有效性裁决\n")
		switch in.Attribution.Verdict {
		case EvoEffectivenessHelpful:
			fmt.Fprintf(&b, "上一次改进有效（成功率 %.1f%% → %.1f%%）。受其影响的规则应保留其核心思路。\n",
				in.Attribution.BaselineSuccessRate*100, in.Attribution.CurrentSuccessRate*100)
		case EvoEffectivenessHarmful:
			fmt.Fprintf(&b, "上一次改进反而有害（成功率 %.1f%% → %.1f%%）。受其影响的规则需要重写或移除，不要沿用上次的改法。\n",
				in.Attribution.BaselineSuccessRate*100, in.Attribution.CurrentSuccessRate*100)
		case EvoEffectivenessNeutral:
			fmt.Fprintf(&b, "上一次改进效果不显著（成功率 %.1f%% → %.1f%%）。需要更有针对性的改进。\n",
				in.Attribution.BaselineSuccessRate*100, in.Attribution.CurrentSuccessRate*100)
		default:
			fmt.Fprintf(&b, "上一次改进的数据不足，无法裁决有效性。请基于当前证据独立判断。\n")
		}
	}

	if len(in.Traces) > 0 {
		fmt.Fprintf(&b, "\n## 近期失败轨迹\n")
		for i, t := range in.Traces {
			fmt.Fprintf(&b, "### 失败 %d（%s）\n", i+1, t.CreatedAt.Format("2006-01-02 15:04"))
			if len(t.FailureTags) > 0 {
				fmt.Fprintf(&b, "- 失败标签：%s\n", strings.Join(t.FailureTags, ", "))
			}
			if t.FlowSummary != "" {
				fmt.Fprintf(&b, "- 流程摘要：%s\n", t.FlowSummary)
			}
			if t.RootCauseAnalysis != "" {
				fmt.Fprintf(&b, "- 根因分析：%s\n", t.RootCauseAnalysis)
			}
		}
	}

	fmt.Fprintf(&b, "\n## 当前 Skill 内容\n%s\n", currentBody)
	return b.String()
}

// validateEvolverOutput 程序侧前置校验：非空、长度上限、Markdown 标题；
// 容忍 LLM 常见的 code-fence 包裹并解包。
func validateEvolverOutput(text string) (string, error) {
	draft := strings.TrimSpace(text)
	if draft == "" {
		return "", apierror.BadRequest("LLM_EVOLVER", "LLM returned empty draft")
	}
	// 解包 ```markdown / ``` 围栏
	if strings.HasPrefix(draft, "```") {
		lines := strings.Split(draft, "\n")
		if len(lines) >= 2 {
			lines = lines[1:]
			if n := len(lines); n > 0 && strings.TrimSpace(lines[n-1]) == "```" {
				lines = lines[:n-1]
			}
			draft = strings.TrimSpace(strings.Join(lines, "\n"))
		}
	}
	if draft == "" {
		return "", apierror.BadRequest("LLM_EVOLVER", "LLM returned empty draft after unwrapping")
	}
	if len(draft) > GateMaxDraftLength {
		return "", apierror.BadRequest("LLM_EVOLVER", "draft exceeds max length %d", GateMaxDraftLength)
	}
	if !strings.Contains(draft, "# ") {
		return "", apierror.BadRequest("LLM_EVOLVER", "draft missing markdown heading")
	}
	return draft, nil
}
