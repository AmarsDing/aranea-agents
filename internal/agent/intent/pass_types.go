package intent

import (
	"aranea-agents/internal/biz"
	"os"
	"strings"
)

// ClarificationQuestion 是澄清问题的 LLM 输出契约（与 biz 持久化契约同型）。
type ClarificationQuestion = biz.ClarificationQuestion

const (
	// RiskFlagNeedsClarification 表示 LLM 判定存在阻塞性歧义，需要先向用户澄清。
	RiskFlagNeedsClarification = "needs_clarification"
	// MaxClarificationQuestions 澄清问题数上限（防 LLM 过度生成）。
	MaxClarificationQuestions = 5
	// MaxSubIntents 子意图数上限（2026-08-28 方案①，防 LLM 过度拆分）。
	MaxSubIntents = 4
)

// SubIntent 是复合请求中识别出的一个独立可执行动作（2026-08-28 方案①多意图）。
// 仅当用户消息含 ≥2 个独立可交付动作时由 intent pass 输出；单意图请求为空，
// 全部下游行为与此前一致（向后兼容）。
type SubIntent struct {
	Goal       string   `json:"goal"`
	IntentKind string   `json:"intent_kind"`
	ToolHints  []string `json:"tool_hints,omitempty"`
}

// Artifact is the structured output of the intent pass (subset of design doc).
type Artifact struct {
	RefinedGoal     string                  `json:"refined_goal"`
	IntentKind      string                  `json:"intent_kind"`
	SuccessCriteria []string                `json:"success_criteria"`
	Ambiguities     []string                `json:"ambiguities"`
	SearchHints     []string                `json:"search_hints"`
	ToolHints       []string                `json:"tool_hints,omitempty"`
	RiskFlags       []string                `json:"risk_flags"`
	Clarifications  []ClarificationQuestion `json:"clarifications,omitempty"`
	// SubIntents 复合意图的全部动作清单（含首要动作，按执行顺序）。顶层
	// refined_goal/intent_kind 始终描述首要动作——复合任务的第二动作不再
	// 依赖主 LLM 读文字的自觉（P-INTENT-SKIP 事故的识别层根修）。
	SubIntents []SubIntent `json:"sub_intents,omitempty"`
}

// HasRiskFlag reports whether the artifact carries the given risk flag.
func (a *Artifact) HasRiskFlag(flag string) bool {
	if a == nil {
		return false
	}
	for _, f := range a.RiskFlags {
		if f == flag {
			return true
		}
	}
	return false
}

// NeedsClarification reports whether the intent pass requests user clarification
// with at least one structured question. 仅有问题而无 risk flag 时不触发（防过度打扰）。
func (a *Artifact) NeedsClarification() bool {
	return a.HasRiskFlag(RiskFlagNeedsClarification) && len(a.Clarifications) > 0
}

// highRiskFlags 是高风险标记集合：命中任一即否决假设式前进（auto_default），
// 必须挂起等待用户显式确认。needs_clarification 本身不属于高风险。
var highRiskFlags = map[string]struct{}{
	"touches_auth":   {},
	"migrations":     {},
	"sensitive_data": {},
	"compliance":     {},
	"destructive":    {},
	"irreversible":   {},
}

// HasHighRiskFlag reports whether the artifact carries any high-risk flag.
func (a *Artifact) HasHighRiskFlag() bool {
	if a == nil {
		return false
	}
	for _, f := range a.RiskFlags {
		if _, ok := highRiskFlags[f]; ok {
			return true
		}
	}
	return false
}

// ClarificationQuestions returns the questions capped at MaxClarificationQuestions.
func (a *Artifact) ClarificationQuestions() []ClarificationQuestion {
	if a == nil {
		return nil
	}
	if len(a.Clarifications) > MaxClarificationQuestions {
		return a.Clarifications[:MaxClarificationQuestions]
	}
	return a.Clarifications
}

// AllToolHints 返回顶层与子意图 tool_hints 的并集（去重、保序：顶层优先，再按
// 子意图顺序）。复合意图的第二动作工具（如「查数据+发邮件」的邮件工具）常进
// 不了顶层 hints（围绕单一目标产出），并集保证预激活/task planner 覆盖全部
// 动作（2026-08-28 方案①消费侧根修）。
func (a *Artifact) AllToolHints() []string {
	if a == nil {
		return nil
	}
	seen := make(map[string]struct{}, len(a.ToolHints)+4)
	out := make([]string, 0, len(a.ToolHints)+4)
	add := func(hints []string) {
		for _, h := range hints {
			h = strings.TrimSpace(h)
			if h == "" {
				continue
			}
			if _, dup := seen[h]; dup {
				continue
			}
			seen[h] = struct{}{}
			out = append(out, h)
		}
	}
	add(a.ToolHints)
	for _, s := range a.SubIntents {
		add(s.ToolHints)
	}
	return out
}

// CloneWithoutClarification returns a copy of the artifact with the
// clarification residue stripped (questions, ambiguities, and the
// needs_clarification risk flag). Used by the clarification-resume path:
// the reused artifact is injected as turn context again, and carrying the
// already-answered questions could nudge the LLM to re-ask them.
// The receiver is not mutated.
func (a *Artifact) CloneWithoutClarification() *Artifact {
	if a == nil {
		return nil
	}
	cp := *a
	cp.Clarifications = nil
	cp.Ambiguities = nil
	if len(a.RiskFlags) > 0 {
		flags := make([]string, 0, len(a.RiskFlags))
		for _, f := range a.RiskFlags {
			if f != RiskFlagNeedsClarification {
				flags = append(flags, f)
			}
		}
		cp.RiskFlags = flags
	}
	return &cp
}

const intentSystemCoding = `You classify and restate the user's request for a coding assistant. Reply with ONE JSON object only, no markdown fences, no commentary. Keys:
- refined_goal (string): one clear sentence of what the user wants.
- intent_kind (string): one of code_change, explain, debug, doc, research, other.
- success_criteria (array of strings): measurable checks (e.g. "tests pass").
- ambiguities (array of strings): questions that still need human clarification, or [].
- search_hints (array of strings): short literals useful for codebase search (identifiers, error substrings, file name fragments).
- tool_hints (array of strings, at most 8): runtime tool slugs likely needed this turn (e.g. search_content, diff_edit, exec_command, web_fetch). Omit tools you are unsure about. Do not invent names.
- risk_flags (array of strings): e.g. touches_auth, migrations, destructive, irreversible, or []. Include "destructive" when the request would destroy/overwrite data, inject faults, or perform irreversible operations. Include "needs_clarification" ONLY when a blocking ambiguity exists (proceeding without an answer would likely produce a wrong-direction or heavily-reworked result).
- clarifications (array of objects, present only when risk_flags contains "needs_clarification", at most 5): blocking questions for the user. Each object: {"question": string, "mode": "single"|"multi", "options": array of strings (2-6), "recommended": array of strings (subset of options, your best default)}. Omit for minor style preferences — never ask when you can reasonably decide yourself.
- sub_intents (array of objects, at most 4, omit entirely when the request is a single action): when the message contains 2 or more INDEPENDENT deliverable actions (each produces its own deliverable or state change, e.g. "check the metrics of X then email a summary to ops", "fix the bug and update the docs"), list ALL actions in execution order. Each object: {"goal": string (one sentence), "intent_kind": string (same enum as intent_kind above), "tool_hints": array of strings (same rules as tool_hints above)}. Do NOT split modifiers, methods or formats of one action ("write a REST API in Go" and "query the data and render it as a table" are each ONE action). When sub_intents is present, the top-level refined_goal/intent_kind/tool_hints still describe the primary (first) action.
An entity name that cannot be uniquely resolved IS a blocking ambiguity when the task depends on that entity's data: e.g. a company/brand named only by a colloquial or ambiguous name with no well-known unique referent ("金鹏科技" — which company?), or a stock mentioned without ticker/exchange. Ask for the identifying detail (full official name, ticker, exchange) via clarifications instead of guessing.
Requests to open an app or URL on the user's own machine (e.g. "打开微信", "open wechat", "打开浏览器访问 xxx") are handled by client tools executing on the user's device: the target environment is the user's local machine by default. This is NOT a blocking ambiguity — do not mark needs_clarification for such requests, and do not ask which environment/OS to run on.
When a "Recent conversation" section precedes the user message, use it to resolve pronouns, ellipses and follow-up references (e.g. "它", "这个", "that one") BEFORE flagging ambiguity, and never ask about facts already established in the conversation. When a clarification is genuinely blocking, always provide your best default in "recommended" — the system may act on recommended defaults autonomously.
For missing data that is NOT blocking: do not only ask. State the data gap, continue with labeled assumptions marked 待核, and proceed. Reserve needs_clarification for cases where guessing would likely produce a wrong-direction result.
Write all user-facing strings (refined_goal, ambiguities, clarifications questions and options) in the same language as the user's request.`

const intentSystemGeneral = `You classify and restate the user's request. Reply with ONE JSON object only, no markdown fences, no commentary. Keys:
- refined_goal (string): one clear sentence of what the user wants.
- intent_kind (string): one of task, question, analysis, creative, research, other.
- success_criteria (array of strings): measurable checks, or [].
- ambiguities (array of strings): questions that still need human clarification, or [].
- search_hints (array of strings): short keywords useful for retrieval or search tools, or [].
- tool_hints (array of strings, at most 8): runtime tool slugs likely needed this turn (e.g. web_fetch, knowledge_search, memory_search). Omit tools you are unsure about. Do not invent names.
- risk_flags (array of strings): e.g. sensitive_data, compliance, destructive, irreversible, or []. Include "destructive" when the request would destroy/overwrite data, inject faults, or perform irreversible operations. Include "needs_clarification" ONLY when a blocking ambiguity exists (proceeding without an answer would likely produce a wrong-direction or heavily-reworked result).
- clarifications (array of objects, present only when risk_flags contains "needs_clarification", at most 5): blocking questions for the user. Each object: {"question": string, "mode": "single"|"multi", "options": array of strings (2-6), "recommended": array of strings (subset of options, your best default)}. Omit for minor style preferences — never ask when you can reasonably decide yourself.
- sub_intents (array of objects, at most 4, omit entirely when the request is a single action): when the message contains 2 or more INDEPENDENT deliverable actions (each produces its own deliverable or state change, e.g. "check the metrics of X then email a summary to ops", "fix the bug and update the docs"), list ALL actions in execution order. Each object: {"goal": string (one sentence), "intent_kind": string (same enum as intent_kind above), "tool_hints": array of strings (same rules as tool_hints above)}. Do NOT split modifiers, methods or formats of one action ("write a REST API in Go" and "query the data and render it as a table" are each ONE action). When sub_intents is present, the top-level refined_goal/intent_kind/tool_hints still describe the primary (first) action.
An entity name that cannot be uniquely resolved IS a blocking ambiguity when the task depends on that entity's data: e.g. a company/brand named only by a colloquial or ambiguous name with no well-known unique referent ("金鹏科技" — which company?), or a stock mentioned without ticker/exchange. Ask for the identifying detail (full official name, ticker, exchange) via clarifications instead of guessing.
Requests to open an app or URL on the user's own machine (e.g. "打开微信", "open wechat", "打开浏览器访问 xxx") are handled by client tools executing on the user's device: the target environment is the user's local machine by default. This is NOT a blocking ambiguity — do not mark needs_clarification for such requests, and do not ask which environment/OS to run on.
When a "Recent conversation" section precedes the user message, use it to resolve pronouns, ellipses and follow-up references (e.g. "它", "这个", "that one") BEFORE flagging ambiguity, and never ask about facts already established in the conversation. When a clarification is genuinely blocking, always provide your best default in "recommended" — the system may act on recommended defaults autonomously.
For missing data that is NOT blocking: do not only ask. State the data gap, continue with labeled assumptions marked 待核, and proceed. Reserve needs_clarification for cases where guessing would likely produce a wrong-direction result.
Write all user-facing strings (refined_goal, ambiguities, clarifications questions and options) in the same language as the user's request.`

// ForceDestructiveFlag checks the original user input against the deterministic
// input-risk scan and force-adds matched risk flags (currently "destructive").
// This is the L2 deterministic safety net behind the L1 prompt guidance
// (BUG-MON-A). No-op when art is nil or a flag is already present.
//
// 2026-08-28 起扫描逻辑迁至 scan.go（ScanInputRisk）并加固（rm 族正则对齐 L3
// 20261267 标准）——扫描独立为 turn 入口通道，不再绑死本 pass 的 LLM 成败
// （缺口 A/B 根修，见 scan.go 头注释）。
func ForceDestructiveFlag(userText string, art *Artifact) {
	if art == nil {
		return
	}
	for _, flag := range ScanInputRisk(userText) {
		if !art.HasRiskFlag(flag) {
			art.RiskFlags = append(art.RiskFlags, flag)
		}
	}
}

// PassEffective returns whether the intent pass should run (extra LLM call).
// Per-agent default comes from agent_runtime_settings.intent_pass_enabled (default true for new agents, P1-1).
// ARANEA_INTENT_PASS: unset → follow agent; "0"/"false"/"off"/"no" → off; "1"/"true"/"on"/"yes" → on; other non-empty values → follow agent.
func PassEffective(agentIntentPassEnabled bool) bool {
	v := strings.TrimSpace(os.Getenv("ARANEA_INTENT_PASS"))
	if v == "" {
		return agentIntentPassEnabled
	}
	v = strings.ToLower(v)
	if v == "0" || v == "false" || v == "off" || v == "no" {
		return false
	}
	if v == "1" || v == "true" || v == "on" || v == "yes" {
		return true
	}
	return agentIntentPassEnabled
}

// ModelOverrideFromEnv reads the optional intent-pass model override.
// ARANEA_INTENT_PASS_MODEL: model name (e.g. "gpt-4.1-mini"). When set, the
// intent pass uses this model instead of the session model — intent
// classification is a lightweight task well served by mini models, and the
// pass sits on the critical path before the main LLM starts streaming.
// ARANEA_INTENT_PASS_PROVIDER: optional provider override; empty inherits the
// session provider (model-only is the common case).
// Returns ("", "") when no override is configured.
func ModelOverrideFromEnv() (provider, model string) {
	model = strings.TrimSpace(os.Getenv("ARANEA_INTENT_PASS_MODEL"))
	if model == "" {
		return "", ""
	}
	provider = strings.TrimSpace(os.Getenv("ARANEA_INTENT_PASS_PROVIDER"))
	return provider, model
}

// IntentPassFromAgent returns persisted intent-pass preference; default ON when settings are missing (P1-1).
// An explicit IntentPassEnabled=false still disables the pass (agent setting can turn OFF).
func IntentPassFromAgent(ag biz.Agent) bool {
	if ag.Settings != nil {
		return ag.Settings.IntentPassEnabled
	}
	return true
}

// ShouldRun reports whether the intent pass should execute for this agent and user text.
// 2026-07-23: 取消 20 字符下限——短歧义消息（如"帮我做个应用"）正是澄清门的目标场景，
// 长度门槛会导致最需要澄清的输入永远走不到澄清门。
func ShouldRun(ag biz.Agent, userText string) bool {
	if !PassEffective(IntentPassFromAgent(ag)) {
		return false
	}
	return strings.TrimSpace(userText) != ""
}

// IntentSystemForAgent selects coding vs general classifier prompt.
func IntentSystemForAgent(ag biz.Agent) string {
	if agentUsesCodingIntentTemplate(ag) {
		return intentSystemCoding
	}
	return intentSystemGeneral
}

func agentUsesCodingIntentTemplate(ag biz.Agent) bool {
	if ag.Settings == nil || !ag.Settings.ToolsEnabled {
		return false
	}
	mode := strings.ToLower(strings.TrimSpace(ag.SystemPromptMode))
	if mode == "none" || mode == "minimized" {
		return false
	}
	profile := strings.ToLower(strings.TrimSpace(ag.Settings.ToolsProfile))
	return profile == "" || profile == "full" || profile == "coding" || profile == "developer"
}
