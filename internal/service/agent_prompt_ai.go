package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/provider"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

const promptFileAIEditSystem = `你是 Agent 提示文件编辑助手。根据用户指令修订 Markdown 提示文件。
只输出修订后的完整文件正文，不要解释、不要代码围栏、不要前后缀说明。`

// PromptFileAIEditor revises prompt file bodies via the platform LLM catalog.
type PromptFileAIEditor struct {
	catalog *biz.LlmProviderModelUsecase
	rt      *provider.RoundTrip
	lg      loggateway.Logger
}

func NewPromptFileAIEditor(catalog *biz.LlmProviderModelUsecase, rt *provider.RoundTrip, lg loggateway.Logger) *PromptFileAIEditor {
	return &PromptFileAIEditor{catalog: catalog, rt: rt, lg: lg}
}

func (e *PromptFileAIEditor) Revise(ctx context.Context, providerName, modelName, fileName, currentBody, instruction string) (string, error) {
	if e == nil || e.catalog == nil || e.rt == nil {
		return "", apierror.Internal(apierror.DomainAgentFile, "prompt file ai editor not configured")
	}
	instruction = strings.TrimSpace(instruction)
	if instruction == "" {
		return "", apierror.BadRequest(apierror.DomainAgentFile, "instruction is required")
	}
	m, err := e.resolveModel(ctx, providerName, modelName)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	user := fmt.Sprintf("文件: %s\n\n当前内容:\n%s\n\n编辑指令:\n%s", fileName, currentBody, instruction)
	req := &trpcmodel.Request{
		Messages: []trpcmodel.Message{
			trpcmodel.NewSystemMessage(promptFileAIEditSystem),
			trpcmodel.NewUserMessage(user),
		},
	}
	ch, err := m.GenerateContent(ctx, req)
	if err != nil {
		e.lg.Warn("prompt file ai generate content failed",
			loggateway.StepID("agent_prompt_ai.generate"),
			loggateway.Err(err))
		return "", apierror.Internal(apierror.DomainAgentFile, "prompt file ai edit failed")
	}
	var sb strings.Builder
	for resp := range ch {
		if resp.Error != nil {
			e.lg.Warn("prompt file ai stream error",
				loggateway.StepID("agent_prompt_ai.stream"),
				loggateway.Str("error", resp.Error.Message))
			return "", apierror.Internal(apierror.DomainAgentFile, "prompt file ai edit failed")
		}
		for _, c := range resp.Choices {
			if c.Delta.Content != "" {
				sb.WriteString(c.Delta.Content)
			}
			if c.Message.Content != "" {
				sb.WriteString(c.Message.Content)
			}
		}
	}
	out := strings.TrimSpace(sb.String())
	out = strings.TrimPrefix(out, "```markdown")
	out = strings.TrimPrefix(out, "```md")
	out = strings.TrimPrefix(out, "```")
	out = strings.TrimSuffix(out, "```")
	return strings.TrimSpace(out), nil
}

func (e *PromptFileAIEditor) resolveModel(ctx context.Context, providerName, modelName string) (trpcmodel.Model, error) {
	models, err := e.catalog.List(ctx)
	if err != nil {
		return nil, err
	}
	providerName = strings.TrimSpace(providerName)
	modelName = strings.TrimSpace(modelName)
	var picked biz.ProviderModel
	for _, row := range models {
		if providerName != "" && !strings.EqualFold(row.Provider, providerName) {
			continue
		}
		if modelName != "" && row.Model != modelName && row.ID != modelName {
			continue
		}
		picked = row
		break
	}
	if picked.Provider == "" && len(models) > 0 {
		picked = models[0]
	}
	if picked.Provider == "" {
		return nil, apierror.BadRequest(apierror.DomainAgentFile, "no matching model in catalog")
	}
	return provider.TRPCModelForProviderModel(ctx, e.catalog, e.rt, picked.Provider, picked.Model, e.lg)
}
