package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/provider"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

const titleGenPrompt = `根据用户的第一条消息，生成一个简短的对话标题（不超过20字）。只返回标题文本，不要解释、不要引号。`

type LLMSessionTitleGenerator struct {
	catalog *biz.LlmProviderModelUsecase
	rt      *provider.RoundTrip
}

func NewLLMSessionTitleGenerator(catalog *biz.LlmProviderModelUsecase, rt *provider.RoundTrip) *LLMSessionTitleGenerator {
	return &LLMSessionTitleGenerator{catalog: catalog, rt: rt}
}

func (g *LLMSessionTitleGenerator) Generate(ctx context.Context, userMessage string) (string, error) {
	if g.catalog == nil {
		return "", nil
	}

	m, err := g.resolveModel(ctx)
	if err != nil {
		return "", fmt.Errorf("session title: resolve model: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	req := &trpcmodel.Request{
		Messages: []trpcmodel.Message{
			trpcmodel.NewSystemMessage(titleGenPrompt),
			trpcmodel.NewUserMessage(userMessage),
		},
	}

	ch, err := m.GenerateContent(ctx, req)
	if err != nil {
		return "", fmt.Errorf("session title: llm call: %w", err)
	}

	var sb strings.Builder
	for resp := range ch {
		if resp.Error != nil {
			return "", fmt.Errorf("session title: llm error: %s", resp.Error.Message)
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

	title := strings.TrimSpace(sb.String())
	if len(title) > 50 {
		title = title[:50]
	}
	return title, nil
}

func (g *LLMSessionTitleGenerator) resolveModel(ctx context.Context) (trpcmodel.Model, error) {
	models, err := g.catalog.List(ctx)
	if err != nil {
		return nil, err
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("no models in catalog")
	}

	pm := pickTitleModel(models)
	return provider.TRPCModelForProviderModel(ctx, g.catalog, g.rt, pm.Provider, pm.Model)
}

func pickTitleModel(models []biz.ProviderModel) biz.ProviderModel {
	for _, m := range models {
		name := strings.ToLower(m.Model)
		if strings.Contains(name, "mini") || strings.Contains(name, "flash") || strings.Contains(name, "lite") || strings.Contains(name, "small") {
			return m
		}
	}
	return models[0]
}
