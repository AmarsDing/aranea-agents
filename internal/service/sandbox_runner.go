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

// SandboxRunner validates skill evolution suggestions in an isolated environment.
// It performs rule-based validation by default, and optionally uses CodeExecutor
// for isolated execution when available.
type SandboxRunner struct {
	uc       *biz.SkillIntelligenceUsecase
	executor trpcagentcodeexec.CodeExecutor // optional, nil = rule-based only
	lg       loggateway.Logger
}

// NewSandboxRunner creates a SandboxRunner with optional CodeExecutor.
// If executor is nil, only rule-based validation is performed.
func NewSandboxRunner(uc *biz.SkillIntelligenceUsecase, factory *codeexecutor.Factory, lg loggateway.Logger) *SandboxRunner {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	var executor trpcagentcodeexec.CodeExecutor
	if factory != nil {
		executor = factory.Resolve(context.Background(), codeexecutor.TypeLocal, "")
	}
	return &SandboxRunner{uc: uc, executor: executor, lg: lg}
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

	// If rule-based checks pass and CodeExecutor is available, run code validation.
	if result.Passed && s.executor != nil {
		codeResult := s.codeExecutionValidation(ctx, suggestion)
		result.Checks = append(result.Checks, codeResult.Checks...)
		if !codeResult.Passed {
			result.Passed = false
			result.Message = "Code execution validation failed"
		}
	}

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
	Passed  bool    `json:"passed"`
	Checks  []check `json:"checks"`
	Message string  `json:"message"`
}

type check struct {
	Name    string `json:"name"`
	Passed  bool   `json:"passed"`
	Message string `json:"message"`
}

func (s *SandboxRunner) ruleBasedValidation(suggestion *biz.SkillEvolutionSuggestion) *sandboxValidationResult {
	result := &sandboxValidationResult{}

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
