package biz

import (
	"encoding/json"
	"strings"

	"aranea-agents/pkg/apierror"
)

// ── Meta Team agent identities (design D5, 73-self-iteration-v3) ────────────
//
// The Meta Team is the platform-internal pipeline executing the seven-stage
// self-improvement loop. Observer/Governor/Applier are pure-code stages;
// Analyst/Patcher/Verifier/Critic involve LLM calls with the structured
// output contracts below.

const (
	SIAgentObserver = "si_observer"
	SIAgentAnalyst  = "si_analyst"
	SIAgentPatcher  = "si_patcher"
	SIAgentVerifier = "si_verifier"
	SIAgentCritic   = "si_critic"
	SIAgentGovernor = "si_governor"
	SIAgentApplier  = "si_applier"
)

// ── System prompts (D5 output contracts embedded) ───────────────────────────

// SIAnalystSystemPrompt instructs the Analyst to turn a suggestion + evidence
// snapshot into a Diagnosis (contract: Diagnosis struct).
const SIAnalystSystemPrompt = `你是 Aranea-Agents 平台的自改进诊断分析师（Analyst）。
输入：一条 platform 进化建议（trigger_source、标题、描述）及其证据快照。
任务：定位根因并给出修复策略。只分析，不修改代码。
需要读源码时，先输出一个工具 JSON（每轮一个）：
{"tool":"patcher_fs_read","path":"repo 相对路径"}
{"tool":"patcher_fs_list","path":"相对目录"}
没有只读根时不要编造文件内容，只根据建议与证据判断。

最终必须输出一个 Diagnosis JSON（不要带 tool 字段，不要输出任何额外文字）：
{
  "root_cause": "根因描述（必填，非空）",
  "affected_files": ["repo 相对路径，如 internal/biz/x.go"],
  "impact_scope": "local | module | global",
  "fix_strategy": "修复策略描述（必填，非空）",
  "confidence": 0.0-1.0 的置信度
}
规则：confidence < 0.5 时仍须输出合法 JSON；不确定的 affected_files 留空数组。`

// SIPatcherSystemPrompt instructs the Patcher to produce a unified diff patch
// inside the sandbox worktree (contract: PatcherOutput struct).
const SIPatcherSystemPrompt = `你是 Aranea-Agents 平台的自改进补丁工程师（Patcher）。
输入：Analyst 的 Diagnosis（根因/影响文件/修复策略），以及用户消息中内联的
相关文件当前内容（若有）。需要核对或改更多文件时，先输出一个工具 JSON：
{"tool":"patcher_fs_read","path":"相对路径"}
{"tool":"patcher_fs_list","path":"相对目录"}
{"tool":"patcher_fs_write","path":"相对路径","content":"完整文件内容"}
{"tool":"patcher_git_diff","path":"可选路径"}
写文件后应再调用 patcher_git_diff。官方产物仍是最终 PatcherOutput JSON
（diff 为 unified diff）；工具写入会在返回前还原，pipeline 按 diff 再 apply。

最终必须输出一个 JSON 对象（不要带 tool 字段，不要输出任何额外文字）：
{
  "diff": "标准 unified diff（以 diff --git 开头，必填非空）",
  "files": 变更文件数,
  "additions": 新增行数,
  "deletions": 删除行数,
  "kind": "code | config | prompt | docs | test",
  "summary": "变更摘要"
}
规则：只输出最小修复；禁止修改 CI 工作流、Makefile、go.mod/go.sum、
Ent/wire 生成物与历史迁移文件；diff 中禁止包含任何密钥/令牌/私钥。`

// SIVerifierSystemPrompt instructs the Verifier LLM to summarize gate results
// (the gates themselves are executed by code via RepoSandbox).
const SIVerifierSystemPrompt = `你是 Aranea-Agents 平台的自改进验证员（Verifier）。
输入：沙盒 Gate 执行结果（g1_build / g2_test / g3_lint 的 passed 与 output）。
任务：总结验证结论，指出失败 Gate 的关键输出摘录，供 Patcher 重试时参考。

严格输出一个 JSON 对象（不要输出任何额外文字）：
{
  "gates": [{"gate": "g1_build", "passed": true, "output": "关键输出摘录"}],
  "summary": "总体结论（失败时给出可操作的重试提示）"
}`

// SICriticSystemPrompt instructs the Critic to semantically review the diff
// (contract: CriticReport struct, reused from V2).
const SICriticSystemPrompt = `你是 Aranea-Agents 平台的自改进审查员（Critic）。
输入：Patcher 产出的 unified diff 及相关源码上下文。
任务：语义审查补丁的安全性、正确性风险与架构合规性。

严格输出一个 JSON 对象（不要输出任何额外文字）：
{
  "is_safe": true/false,
  "risk_level": "low | medium | high",
  "concerns": ["逐项列出风险点；is_safe=false 时必填非空"],
  "suggestion": "处置建议"
}
规则：涉及数据损坏、安全漏洞、破坏兼容性的改动必须 is_safe=false。`

// ── Patcher output contract ──────────────────────────────────────────────────

// PatcherOutput is the Patcher Agent structured output (D5).
// Files/Additions/Deletions are declared by the LLM but always normalized to
// the diff-derived truth (LLMs cannot count reliably).
type PatcherOutput struct {
	Diff      string                   `json:"diff"`
	Files     int                      `json:"files"`
	Additions int                      `json:"additions"`
	Deletions int                      `json:"deletions"`
	Kind      SelfImprovementPatchKind `json:"kind"`
	Summary   string                   `json:"summary,omitempty"`
}

// ── Structured-output parsers ────────────────────────────────────────────────

// stripSIJSONFence tolerates a surrounding ```json / ``` code fence.
func stripSIJSONFence(text string) string {
	raw := strings.TrimSpace(text)
	if !strings.HasPrefix(raw, "```") {
		return raw
	}
	lines := strings.Split(raw, "\n")
	if len(lines) < 2 {
		return raw
	}
	lines = lines[1:]
	if n := len(lines); n > 0 && strings.TrimSpace(lines[n-1]) == "```" {
		lines = lines[:n-1]
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// ParseDiagnosisJSON parses the Analyst output into a validated Diagnosis.
func ParseDiagnosisJSON(text string) (*Diagnosis, error) {
	raw := stripSIJSONFence(text)
	if raw == "" {
		return nil, apierror.BadRequest("SELF_IMPROVEMENT", "empty diagnosis output")
	}
	var d Diagnosis
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		return nil, apierror.BadRequest("SELF_IMPROVEMENT", "invalid diagnosis JSON: %s", err)
	}
	if strings.TrimSpace(d.RootCause) == "" {
		return nil, apierror.BadRequest("SELF_IMPROVEMENT", "diagnosis.root_cause is required")
	}
	if strings.TrimSpace(d.FixStrategy) == "" {
		return nil, apierror.BadRequest("SELF_IMPROVEMENT", "diagnosis.fix_strategy is required")
	}
	switch d.ImpactScope {
	case "local", "module", "global":
	default:
		return nil, apierror.BadRequest("SELF_IMPROVEMENT", "diagnosis.impact_scope must be local|module|global, got %q", d.ImpactScope)
	}
	if d.Confidence < 0 || d.Confidence > 1 {
		return nil, apierror.BadRequest("SELF_IMPROVEMENT", "diagnosis.confidence must be in [0,1], got %v", d.Confidence)
	}
	return &d, nil
}

// ParsePatcherOutputJSON parses the Patcher output into a validated
// PatcherOutput. Declared size stats are normalized to ComputeDiffStats(diff).
func ParsePatcherOutputJSON(text string) (*PatcherOutput, error) {
	raw := stripSIJSONFence(text)
	if raw == "" {
		return nil, apierror.BadRequest("SELF_IMPROVEMENT", "empty patcher output")
	}
	var p PatcherOutput
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return nil, apierror.BadRequest("SELF_IMPROVEMENT", "invalid patcher output JSON: %s", err)
	}
	if strings.TrimSpace(p.Diff) == "" {
		return nil, apierror.BadRequest("SELF_IMPROVEMENT", "patcher.diff is required")
	}
	switch p.Kind {
	case PatchKindCode, PatchKindConfig, PatchKindPrompt, PatchKindDocs, PatchKindTest:
	default:
		return nil, apierror.BadRequest("SELF_IMPROVEMENT", "patcher.kind must be code|config|prompt|docs|test, got %q", p.Kind)
	}
	stats := ComputeDiffStats(p.Diff)
	p.Files, p.Additions, p.Deletions = stats.Files, stats.Additions, stats.Deletions
	return &p, nil
}

// ParseCriticReportJSON parses the Critic output into a validated CriticReport.
func ParseCriticReportJSON(text string) (*CriticReport, error) {
	raw := stripSIJSONFence(text)
	if raw == "" {
		return nil, apierror.BadRequest("SELF_IMPROVEMENT", "empty critic output")
	}
	var r CriticReport
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		return nil, apierror.BadRequest("SELF_IMPROVEMENT", "invalid critic report JSON: %s", err)
	}
	switch r.RiskLevel {
	case string(RiskLevelLow), string(RiskLevelMedium), string(RiskLevelHigh):
	default:
		return nil, apierror.BadRequest("SELF_IMPROVEMENT", "critic.risk_level must be low|medium|high, got %q", r.RiskLevel)
	}
	if !r.IsSafe && len(r.Concerns) == 0 {
		return nil, apierror.BadRequest("SELF_IMPROVEMENT", "critic.concerns is required when is_safe=false")
	}
	return &r, nil
}
