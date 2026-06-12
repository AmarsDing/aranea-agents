package skill

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

type LLMSkillGenerator interface {
	Generate(ctx context.Context, prompt string) (string, error)
}

// LLMCallerAdapter adapts biz.LLMCaller to LLMSkillGenerator.
// It uses the platform's DefaultRefineLLM to resolve provider/model credentials.
type LLMCallerAdapter struct {
	caller   biz.LLMCaller
	provider string
	model    string
}

func NewLLMCallerAdapter(caller biz.LLMCaller, provider, model string) *LLMCallerAdapter {
	return &LLMCallerAdapter{caller: caller, provider: provider, model: model}
}

func (a *LLMCallerAdapter) Generate(ctx context.Context, prompt string) (string, error) {
	text, _, err := a.caller.Call(ctx, biz.LLMCallRequest{
		Provider: a.provider,
		Model:    a.model,
		System:   "You are a skill definition generator for an AI agent platform. Generate valid SKILL.md files based on detected behavioral patterns.",
		User:     prompt,
	})
	return text, err
}

type SkillAutoCreator struct {
	generator LLMSkillGenerator
	lg        loggateway.Logger
}

func NewSkillAutoCreator(generator LLMSkillGenerator, lg loggateway.Logger) *SkillAutoCreator {
	return &SkillAutoCreator{generator: generator, lg: lg}
}

func (c *SkillAutoCreator) GenerateSKILLMD(ctx context.Context, patternDesc string, toolHistory []biz.ToolCallRecord) (string, string, error) {
	prompt := buildSkillPrompt(patternDesc, toolHistory)
	genCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	result, err := c.generator.Generate(genCtx, prompt)
	if err != nil {
		if genCtx.Err() == context.DeadlineExceeded {
			return "", "", apierror.Unavailable(apierror.DomainSkill, "skill generation timed out")
		}
		return "", "", apierror.Internal(apierror.DomainSkill, "generate skill").WithCause(err)
	}
	name, content, err := parseSkillOutput(result)
	if err != nil {
		return "", "", err
	}
	if !strings.HasPrefix(strings.TrimSpace(content), "---") {
		return "", "", apierror.BadRequest(apierror.DomainSkill, "generated SKILL.md must start with YAML front matter (---)")
	}
	if name == "" {
		hash := sha256.Sum256([]byte(patternDesc))
		name = fmt.Sprintf("auto_skill_%x", hash[:4])
	}
	return name, content, nil
}

func buildSkillPrompt(patternDesc string, toolHistory []biz.ToolCallRecord) string {
	var sb strings.Builder
	sb.WriteString("You are a skill definition generator for an AI agent platform.\n")
	sb.WriteString("Based on the following detected behavioral pattern and tool call history, generate a SKILL.md file.\n\n")
	sb.WriteString("The SKILL.md must:\n")
	sb.WriteString("1. Start with YAML front matter enclosed in ---\n")
	sb.WriteString("2. Include a 'name' field in the front matter\n")
	sb.WriteString("3. Include a 'description' field in the front matter\n")
	sb.WriteString("4. Include a 'triggers' section listing when this skill should activate\n")
	sb.WriteString("5. Include a 'steps' section with the workflow steps\n\n")
	sb.WriteString("Output format:\n")
	sb.WriteString("NAME: <skill_name>\n")
	sb.WriteString("---\n<YAML front matter>\n---\n<skill body>\n\n")
	sb.WriteString("Detected pattern:\n")
	sb.WriteString(patternDesc)
	sb.WriteString("\n\n")
	if len(toolHistory) > 0 {
		sb.WriteString("Tool call history:\n")
		for _, tc := range toolHistory {
			status := "success"
			if !tc.Success {
				status = "failure"
			}
			sb.WriteString(fmt.Sprintf("- %s(%s) -> %s [%s]\n", tc.ToolName, tc.Arguments, tc.Result, status))
		}
	}
	return sb.String()
}

func parseSkillOutput(output string) (string, string, error) {
	output = strings.TrimSpace(output)
	nameLine := ""
	content := output
	if idx := strings.Index(output, "NAME:"); idx != -1 {
		afterName := output[idx+5:]
		end := strings.Index(afterName, "\n")
		if end == -1 {
			end = len(afterName)
		}
		nameLine = strings.TrimSpace(afterName[:end])
		contentStart := strings.Index(afterName, "---")
		if contentStart != -1 {
			content = strings.TrimSpace(afterName[contentStart:])
		} else {
			content = afterName[end+1:]
		}
	}
	return nameLine, content, nil
}
