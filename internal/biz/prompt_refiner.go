package biz

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-kratos/kratos/v2/errors"
)

// Refine error reasons (B-6). Each maps to a distinct front-end UX branch.
const (
	refineReasonUnknownScope = "REFINE_UNKNOWN_SCOPE"
	refineReasonTooLong      = "REFINE_INPUT_TOO_LONG"
	refineReasonNoLLM        = "REFINE_NO_LLM"
	refineReasonLLMFailed    = "REFINE_LLM_FAILED"
)

// PromptRefiner provides AI-assisted refinement for all prompt scopes.
// It is the biz-layer core of PGO-3; the service layer calls it via HTTP.
// PGO-3-BIZ-02.
type PromptRefiner struct {
	agents  AgentRepository
	sys     *SystemSettingUsecase
	catalog *LlmProviderModelUsecase
	llm     LLMCaller
}

// NewPromptRefiner wires the refiner.
func NewPromptRefiner(agents AgentRepository, sys *SystemSettingUsecase, catalog *LlmProviderModelUsecase, llm LLMCaller) *PromptRefiner {
	return &PromptRefiner{agents: agents, sys: sys, catalog: catalog, llm: llm}
}

// RefineRequest is the biz-layer input for a refinement call.
type RefineRequest struct {
	Scope        FieldScope
	ResourceID   string // agent_id / category_id (for model selection & audit)
	FileName     string // only for ScopeAgentFile
	OriginalText string
	UserHint     string // optional free-text instruction from user ("more formal", etc.)
	TargetMode   string // complete / task / minimized (influences length budget)
}

// RefineResult is returned by PromptRefiner.Refine.
type RefineResult struct {
	Refined      string
	Diff         string
	TokensBefore int
	TokensAfter  int
	Provider     string
	Model        string
	ModelSource  ModelSource
}

// ModelSource describes which strategy resolved the LLM.
type ModelSource string

const (
	ModelSourceAgent         ModelSource = "agent_model"
	ModelSourceSystemDefault ModelSource = "system_default"
	ModelSourceCatalogFirst  ModelSource = "catalog_first"
)

// refineCallTimeout caps the LLM call. PGO-3-BIZ-05 (B-7).
const refineCallTimeout = 60 * time.Second

// Refine refines the given text using the FieldGuide for the scope.
func (r *PromptRefiner) Refine(ctx context.Context, req RefineRequest) (*RefineResult, error) {
	guide, ok := GetFieldGuide(req.Scope, req.FileName)
	if !ok {
		return nil, ErrRefineUnknownScope(string(req.Scope), req.FileName)
	}
	if err := validateRefineInput(req, guide); err != nil {
		return nil, err
	}

	provider, model, source, err := r.resolveModel(ctx, req)
	if err != nil {
		return nil, err
	}

	// Spec extraction uses a different prompt shape (markdown → YAML) than field
	// refinement (free text → polished free text). Branch here to keep FieldGuide
	// data structures uniform. PGO-4 B-2.
	var sys, user string
	if req.Scope == ScopeSpecExtract {
		sys = buildSpecExtractSystemPrompt(guide)
		user = strings.TrimSpace(req.OriginalText)
	} else {
		sys = buildRefineSystemPrompt(guide, req.TargetMode)
		user = buildRefineUserPrompt(req.OriginalText, req.UserHint, guide)
	}

	callCtx, cancel := context.WithTimeout(ctx, refineCallTimeout)
	defer cancel()
	refined, totalTok, err := r.llm.Call(callCtx, LLMCallRequest{
		Provider: provider,
		Model:    model,
		System:   sys,
		User:     user,
	})
	if err != nil {
		return nil, errors.InternalServer(refineReasonLLMFailed, "llm call failed").WithCause(err)
	}
	refined = strings.TrimSpace(refined)
	// Spec extraction may wrap YAML in ``` fences; strip them.
	if req.Scope == ScopeSpecExtract {
		refined = stripCodeFence(refined)
	}

	// Enforce hard budget (truncate at line boundary).
	if guide.Budget.Hard > 0 && utf8.RuneCountInString(refined) > guide.Budget.Hard {
		refined = truncateAtLineBoundary(refined, guide.Budget.Hard)
	}

	return &RefineResult{
		Refined:      refined,
		Diff:         unifiedDiffSimple(req.OriginalText, refined),
		TokensBefore: estimateTokenCount(req.OriginalText),
		TokensAfter:  totalTok,
		Provider:     provider,
		Model:        model,
		ModelSource:  source,
	}, nil
}

// resolveModel picks the LLM config using a 3-tier fallback:
//  1. Agent's own provider/model (for agent scopes)
//  2. Platform DefaultRefineLLM setting
//  3. First enabled model from catalog
func (r *PromptRefiner) resolveModel(ctx context.Context, req RefineRequest) (string, string, ModelSource, error) {
	// Tier 1: agent's own model for agent-scoped requests.
	if req.Scope == ScopeAgentDescription || req.Scope == ScopeAgentFile {
		if strings.TrimSpace(req.ResourceID) != "" && r.agents != nil {
			ag, err := r.agents.GetAgentByID(ctx, req.ResourceID)
			if err == nil && strings.TrimSpace(ag.Provider) != "" && strings.TrimSpace(ag.Model) != "" {
				return ag.Provider, ag.Model, ModelSourceAgent, nil
			}
		}
	}

	// Tier 2: platform system setting DefaultRefineLLM.
	if r.sys != nil {
		s, err := r.sys.Get(ctx)
		if err == nil && strings.TrimSpace(s.DefaultRefineLLM.Provider) != "" && strings.TrimSpace(s.DefaultRefineLLM.Model) != "" {
			return s.DefaultRefineLLM.Provider, s.DefaultRefineLLM.Model, ModelSourceSystemDefault, nil
		}
	}

	// Tier 3: first enabled model from catalog.
	if r.catalog != nil {
		models, err := r.catalog.List(ctx)
		if err == nil {
			for _, m := range models {
				if m.Provider != "" && m.Model != "" && m.Enabled {
					return m.Provider, m.Model, ModelSourceCatalogFirst, nil
				}
			}
		}
	}

	return "", "", "", ErrRefineNoLLMAvailable()
}

// ─── Prompt builders ────────────────────────────────────────────────────────

func buildRefineSystemPrompt(guide FieldGuide, mode string) string {
	budget := guide.Budget.Soft
	if budget == 0 {
		budget = 800
	}
	if strings.ToLower(mode) == "task" && budget > 400 {
		budget = 400
	}

	var b strings.Builder
	b.WriteString("你是一名专业的 AI Prompt 工程师，专注于为 Agent 平台撰写高质量 prompt 内容。\n\n")
	b.WriteString(fmt.Sprintf("当前字段：%s\n", guide.TitleZh))
	b.WriteString(fmt.Sprintf("字段用途：%s\n\n", guide.Purpose))

	if len(guide.ShouldWrite) > 0 {
		b.WriteString("该写的内容：\n")
		for _, item := range guide.ShouldWrite {
			b.WriteString("- " + item + "\n")
		}
		b.WriteString("\n")
	}
	if len(guide.ShouldAvoid) > 0 {
		b.WriteString("不该写的内容：\n")
		for _, item := range guide.ShouldAvoid {
			b.WriteString("- " + item + "\n")
		}
		b.WriteString("\n")
	}
	b.WriteString(fmt.Sprintf("字符预算：目标在 %d 字符以内（当前模式：%s）。\n\n", budget, modeLabel(mode)))
	b.WriteString("请直接输出优化后的内容，不要解释你的修改，不要输出 markdown 代码块包装（除非原文就是 markdown）。")
	return b.String()
}

func buildRefineUserPrompt(original, userHint string, guide FieldGuide) string {
	var b strings.Builder
	b.WriteString("请优化以下内容：\n\n")
	b.WriteString("---\n")
	b.WriteString(original)
	b.WriteString("\n---\n")
	if strings.TrimSpace(userHint) != "" {
		b.WriteString("\n用户补充说明：")
		b.WriteString(strings.TrimSpace(userHint))
		b.WriteString("\n")
	}
	if len(guide.Examples) > 0 {
		b.WriteString("\n参考示例（仅供风格参考，不要照搬）：\n")
		for _, ex := range guide.Examples {
			b.WriteString(fmt.Sprintf("（%s 行业）%s\n", ex.Industry, ex.Body))
		}
	}
	return strings.TrimSpace(b.String())
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func modeLabel(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "complete", "":
		return "complete（完整）"
	case "task":
		return "task（任务，需紧凑）"
	case "minimized":
		return "minimized（极简）"
	default:
		return mode
	}
}

// unifiedDiffSimple returns a line-level unified diff for display in the UI.
// For a production-quality diff, consider calling `diff -u` via exec; this
// implementation is intentionally simple to avoid C deps. PGO-3.
func unifiedDiffSimple(original, revised string) string {
	origLines := strings.Split(original, "\n")
	revLines := strings.Split(revised, "\n")
	var b strings.Builder
	b.WriteString("--- original\n+++ revised\n")
	maxLen := len(origLines)
	if len(revLines) > maxLen {
		maxLen = len(revLines)
	}
	for i := 0; i < maxLen; i++ {
		ol := ""
		rl := ""
		if i < len(origLines) {
			ol = origLines[i]
		}
		if i < len(revLines) {
			rl = revLines[i]
		}
		if ol == rl {
			b.WriteString(" " + ol + "\n")
		} else {
			if i < len(origLines) {
				b.WriteString("-" + ol + "\n")
			}
			if i < len(revLines) {
				b.WriteString("+" + rl + "\n")
			}
		}
	}
	return b.String()
}

// estimateTokenCount approximates token count from character count.
// Assumes ~2.5 chars/token for mixed CJK+English content.
func estimateTokenCount(s string) int {
	chars := utf8.RuneCountInString(s)
	return (chars*10 + 24) / 25 // ≈ chars / 2.5
}

func truncateAtLineBoundary(s string, maxChars int) string {
	runes := []rune(s)
	if len(runes) <= maxChars {
		return s
	}
	sub := string(runes[:maxChars])
	if idx := strings.LastIndexByte(sub, '\n'); idx > maxChars/2 {
		return strings.TrimRight(sub[:idx], " \t") + "\n…"
	}
	return strings.TrimRight(sub, " \t") + "…"
}

// validateRefineInput checks input limits against the scope's budget. PGO-3.
// Spec extraction can accept much larger inputs than field refinement.
func validateRefineInput(req RefineRequest, guide FieldGuide) error {
	limit := 5000
	if req.Scope == ScopeSpecExtract {
		limit = 30000
	}
	if guide.Budget.Hard > 0 && guide.Budget.Hard*4 > limit {
		// Allow the input to be up to 4× the output budget — refinement usually
		// shortens text, and spec_extract trades verbose markdown for compact YAML.
		limit = guide.Budget.Hard * 4
	}
	if utf8.RuneCountInString(req.OriginalText) > limit {
		return ErrRefineTooLong(limit)
	}
	return nil
}

// buildSpecExtractSystemPrompt constructs the system prompt for converting
// markdown organisational descriptions into YAML org spec. PGO-4 B-2.
func buildSpecExtractSystemPrompt(_ FieldGuide) string {
	return `你是一名组织架构信息抽取助手。请将用户提供的 markdown 描述（可能是公司介绍、岗位说明、流程文档）抽取为以下结构的 YAML：

version: 1
metadata:
  source_file: ""
  generated_by: "aranea-import"
spec:
  industries:
    - key: <kebab-case>
      name: <中文名>
      description: <行业说明>
      departments:
        - key: <kebab-case>
          name: <中文名>
          description: <部门职责>
          positions:
            - key: <kebab-case>
              name: <中文名>
              description: <岗位职责>
  agents:
    - key: <kebab-case>
      display_name: <Agent 显示名>
      category_position: "<industry_key>/<dept_key>/<pos_key>"
      provider: ""
      model: ""
      system_prompt_mode: "complete"
      agent_description: <Agent 简介>
  teams: []

要求：
1. 只输出有效的 YAML，不要任何解释文字或 markdown 代码块包装；
2. 所有 key 使用 kebab-case，避免空格 / 中文；
3. 缺失字段输出空字符串 ""，不要省略字段；
4. 若 markdown 中无 agent / team 信息，输出空数组 [] 即可。`
}

// stripCodeFence removes optional triple-backtick fencing around YAML output.
func stripCodeFence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	lines := strings.Split(s, "\n")
	if len(lines) >= 2 {
		lines = lines[1:]
	}
	if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "```" {
		lines = lines[:len(lines)-1]
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// ─── Sentinel errors ─────────────────────────────────────────────────────────

// ErrRefineUnknownScope maps to 400 with reason REFINE_UNKNOWN_SCOPE.
func ErrRefineUnknownScope(scope, fileName string) error {
	key := scope
	if fileName != "" {
		key += "/" + fileName
	}
	return errors.BadRequest(refineReasonUnknownScope, "unknown scope: "+key)
}

// ErrRefineNoLLMAvailable maps to 503 with reason REFINE_NO_LLM.
func ErrRefineNoLLMAvailable() error {
	return errors.ServiceUnavailable(refineReasonNoLLM, "no LLM available for refinement; configure DefaultRefineLLM in system settings")
}

// ErrRefineTooLong maps to 400 with reason REFINE_INPUT_TOO_LONG.
func ErrRefineTooLong(limit int) error {
	return errors.BadRequest(refineReasonTooLong, fmt.Sprintf("original_text exceeds %d character limit", limit))
}
