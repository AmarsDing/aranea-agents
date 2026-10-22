package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"aranea-agents/internal/agent/codeexecutor"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	trpcagentcodeexec "trpc.group/trpc-go/trpc-agent-go/codeexecutor"
)

// Compile-time check: SandboxRunner implements the biz.SandboxRunner port
// consumed by biz.GateVerifier (skill merge / evolution Gate stage).
var _ biz.SandboxRunner = (*SandboxRunner)(nil)

// SandboxRunner validates skill evolution suggestions in an isolated environment.
// It performs rule-based validation by default, and optionally uses CodeExecutor
// for isolated execution when available.
type SandboxRunner struct {
	uc       *biz.SkillIntelligenceUsecase
	factory  *codeexecutor.Factory          // per-surface policy read (may be nil）
	executor trpcagentcodeexec.CodeExecutor // optional, nil = rule-based only
	lg       loggateway.Logger
}

// NewSandboxRunner creates a SandboxRunner with optional CodeExecutor.
// If executor is nil, only rule-based validation is performed.
//
// Executor 经 per-surface 显式链解析（P0-1 doNow ③：skill_gate 面由
// SurfacePolicy 决定 degradede/enforce/refuse；默认 degrade = 旧硬编码
// Resolve(TypeLocal) 同语义，零行为变更）。executor==nil 不再静默放行：
// 校验结果追加结构化 skipped check（见 executorUnavailableCheck）。
func NewSandboxRunner(uc *biz.SkillIntelligenceUsecase, factory *codeexecutor.Factory, lg loggateway.Logger) *SandboxRunner {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	var executor trpcagentcodeexec.CodeExecutor
	if factory != nil {
		executor = factory.ResolveForSurface(context.Background(), codeexecutor.SurfaceSkillGate, "")
	}
	return &SandboxRunner{uc: uc, factory: factory, executor: executor, lg: lg}
}

// executorUnavailableCheck builds the structured skipped check appended when
// the code executor is unavailable (§12.1-③：与既有 check{Name,Passed,Message}
// JSON 对齐，不新增顶层键）：
//   - 默认 degrade（含无 factory）：可见降级跳过（Passed=true，明示 skipped，
//     不再是静默通过）；
//   - 面级 enforce/refuse 或全局 strict（83 FR-3）：fail-closed（Passed=false）
//     ——fail-closed 仅限严格语义（评审 doNow ③/§2.5 #1：严格下宁可门禁变严
//     可见，不可静默跳过执行校验）。
func (s *SandboxRunner) executorUnavailableCheck() check {
	passed := true
	msg := "code execution skipped: executor unavailable (degraded surface — rule checks only)"
	if s.factory != nil {
		switch {
		case s.factory.SurfacePolicyFor(codeexecutor.SurfaceSkillGate) != codeexecutor.PolicyDegrade:
			passed = false
			msg = "code execution required by surface policy, executor unavailable"
		case s.factory.StrictFallback():
			passed = false
			msg = "code execution unavailable under strict fallback policy"
		}
	}
	return check{Name: "code_execution_skipped", Passed: passed, Message: msg}
}

// applyCodeExecution 在 executor 可用时追加执行校验；不可用时追加结构化
// skipped（fail-closed 仅限 strict/面级严格），返回是否仍通过。
// factory==nil 是 legacy 构造面（线上 wire 恒注入 factory；nil 仅存在于测试
// 通路）——保持旧行为零变更：不追加 skipped check。
func (s *SandboxRunner) applyCodeExecution(ctx context.Context, suggestion *biz.SkillEvolutionSuggestion, result *sandboxValidationResult) {
	if !result.Passed {
		return
	}
	if s.executor == nil {
		if s.factory == nil {
			return // legacy path: keep byte-identical behavior
		}
		skipped := s.executorUnavailableCheck()
		result.Checks = append(result.Checks, skipped)
		if !skipped.Passed {
			result.Passed = false
			result.Message = "Code execution validation unavailable"
		}
		return
	}
	codeResult := s.codeExecutionValidation(ctx, suggestion)
	result.Checks = append(result.Checks, codeResult.Checks...)
	if !codeResult.Passed {
		result.Passed = false
		result.Message = "Code execution validation failed"
	}
}

// RunSandbox implements biz.SandboxRunner for the Gate verification stage.
// Unlike ValidateSuggestion, it is a pure check: it validates the given draft
// body (rule-based + optional code execution) without reading or writing any
// suggestion state in the DB.
func (s *SandboxRunner) RunSandbox(ctx context.Context, skillID string, draftBody string) (bool, json.RawMessage, error) {
	suggestion := &biz.SkillEvolutionSuggestion{SkillID: skillID, DraftSkillBody: draftBody}
	result := s.ruleBasedValidation(suggestion)
	s.applyCodeExecution(ctx, suggestion, result)
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return false, nil, apierror.Internal("SANDBOX_RUNNER", "marshal sandbox result: %s", err)
	}
	return result.Passed, resultJSON, nil
}

// ValidateSuggestion runs sandbox validation on an evolution suggestion.
// It performs rule-based validation first, then optionally runs code execution
// validation if a CodeExecutor is available and the draft contains code blocks.
func (s *SandboxRunner) ValidateSuggestion(ctx context.Context, suggestionID string) (bool, json.RawMessage, error) {
	suggestion, err := s.uc.GetEvolutionSuggestion(ctx, suggestionID)
	if err != nil {
		return false, nil, err
	}
	if suggestion == nil {
		return false, nil, apierror.NotFound("SANDBOX_RUNNER", "suggestion not found: %s", suggestionID)
	}

	// Update lifecycle status to validating
	if lcErr := s.uc.UpdateSuggestionLifecycleStatus(ctx, suggestionID, biz.EvoLifecycleValidating); lcErr != nil {
		s.lg.Warn("SandboxRunner: failed to set validating status",
			loggateway.StepID("sandbox.validate"),
			loggateway.Str("suggestion_id", suggestionID),
			loggateway.Err(lcErr))
	}

	result := s.ruleBasedValidation(suggestion)

	// If rule-based checks pass: code execution when available, else a
	// structured skipped check (fail-closed only under strict/posture).
	s.applyCodeExecution(ctx, suggestion, result)

	resultJSON, _ := json.Marshal(result)

	// Update the suggestion with sandbox result
	if err := s.uc.UpdateSuggestionSandboxResult(ctx, suggestionID, result.Passed, resultJSON); err != nil {
		s.lg.Warn("SandboxRunner: failed to update sandbox result",
			loggateway.StepID("sandbox.validate"),
			loggateway.Str("suggestion_id", suggestionID),
			loggateway.Err(err))
	}

	// Update lifecycle status based on validation result
	lifecycleStatus := biz.EvoLifecycleDraft
	if result.Passed {
		lifecycleStatus = biz.EvoLifecycleReady
	}
	if lcErr := s.uc.UpdateSuggestionLifecycleStatus(ctx, suggestionID, lifecycleStatus); lcErr != nil {
		s.lg.Warn("SandboxRunner: failed to update lifecycle status",
			loggateway.StepID("sandbox.validate"),
			loggateway.Str("suggestion_id", suggestionID),
			loggateway.Err(lcErr))
	}

	return result.Passed, resultJSON, nil
}

// codeExecutionValidation attempts to execute code blocks from the draft body
// in an isolated environment using CodeExecutor.
func (s *SandboxRunner) codeExecutionValidation(ctx context.Context, suggestion *biz.SkillEvolutionSuggestion) *sandboxValidationResult {
	result := &sandboxValidationResult{}

	blocks := s.extractCodeBlocks(suggestion.DraftSkillBody)
	if len(blocks) == 0 {
		// No code blocks found — not an error, just nothing to execute.
		result.Passed = true
		result.Message = "No code blocks to execute"
		return result
	}

	var allPassed = true
	for i, block := range blocks {
		input := trpcagentcodeexec.CodeExecutionInput{
			CodeBlocks: []trpcagentcodeexec.CodeBlock{
				{Language: block.Language, Code: block.Code},
			},
		}
		execResult, err := s.executor.ExecuteCode(ctx, input)
		checkName := fmt.Sprintf("code_block_%d_execution", i+1)
		if err != nil {
			result.Checks = append(result.Checks, check{
				Name:    checkName,
				Passed:  false,
				Message: fmt.Sprintf("Execution error: %s", err.Error()),
			})
			allPassed = false
			continue
		}
		// Check for non-zero exit code in output.
		hasError := strings.Contains(execResult.Output, "[exit ") && !strings.Contains(execResult.Output, "[exit 0]")
		result.Checks = append(result.Checks, check{
			Name:    checkName,
			Passed:  !hasError && !strings.Contains(execResult.Output, "[OOM killed]") && !strings.Contains(execResult.Output, "[timeout]"),
			Message: s.truncateOutput(execResult.Output, 200),
		})
		if hasError {
			allPassed = false
		}
	}

	result.Passed = allPassed
	if allPassed {
		result.Message = "Code execution validation passed"
	} else {
		result.Message = "Some code blocks failed execution"
	}
	return result
}

// codeBlock represents an extracted code block from a draft body.
type codeBlock struct {
	Language string
	Code     string
}

// extractCodeBlocks extracts fenced code blocks from markdown text.
func (s *SandboxRunner) extractCodeBlocks(draftBody string) []codeBlock {
	var blocks []codeBlock
	lines := strings.Split(draftBody, "\n")
	var inBlock bool
	var lang string
	var codeLines []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !inBlock && strings.HasPrefix(trimmed, "```") {
			inBlock = true
			lang = strings.TrimPrefix(trimmed, "```")
			lang = strings.TrimSpace(lang)
			codeLines = nil
			continue
		}
		if inBlock && strings.HasPrefix(trimmed, "```") {
			inBlock = false
			if len(codeLines) > 0 {
				blocks = append(blocks, codeBlock{
					Language: lang,
					Code:     strings.Join(codeLines, "\n"),
				})
			}
			continue
		}
		if inBlock {
			codeLines = append(codeLines, line)
		}
	}
	return blocks
}

func (s *SandboxRunner) truncateOutput(output string, maxLen int) string {
	if len(output) <= maxLen {
		return output
	}
	return output[:maxLen] + "..."
}

type sandboxValidationResult struct {
	Passed bool `json:"passed"`
	// Validator names the producer of this payload (F10): the SandboxRunner
	// check set differs from the GateVerifier gate, so consumers can tell what
	// "passed" actually verified.
	Validator string  `json:"validator"`
	Checks    []check `json:"checks"`
	Message   string  `json:"message"`
}

type check struct {
	Name    string `json:"name"`
	Passed  bool   `json:"passed"`
	Message string `json:"message"`
}

func (s *SandboxRunner) ruleBasedValidation(suggestion *biz.SkillEvolutionSuggestion) *sandboxValidationResult {
	result := &sandboxValidationResult{Validator: biz.SandboxValidatorSandboxRunner}

	// Check 1: Draft body is not empty
	c1 := check{Name: "draft_body_not_empty", Passed: suggestion.DraftSkillBody != "", Message: "Draft body must not be empty"}
	result.Checks = append(result.Checks, c1)

	// Check 2: Draft body length is reasonable (< 10000 chars)
	c2 := check{Name: "draft_body_length", Passed: len(suggestion.DraftSkillBody) < 10000, Message: "Draft body length must be under 10000 chars"}
	result.Checks = append(result.Checks, c2)

	// Check 3: Skill ID is valid
	c3 := check{Name: "skill_id_valid", Passed: suggestion.SkillID != "", Message: "Skill ID must be valid"}
	result.Checks = append(result.Checks, c3)

	// Overall pass if all checks pass
	result.Passed = c1.Passed && c2.Passed && c3.Passed
	if result.Passed {
		result.Message = "All validation checks passed"
	} else {
		result.Message = "Some validation checks failed"
	}

	return result
}
