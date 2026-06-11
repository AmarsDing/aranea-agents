package service

import (
	"context"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/provider"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

const titleGenPrompt = `根据用户的第一条消息，生成一个简短的对话标题（不超过20字）。只返回标题文本，不要解释、不要引号。`

type LLMSessionTitleGenerator struct {
	catalog *biz.LlmProviderModelUsecase
	rt      *provider.RoundTrip
	lg      loggateway.Logger
}

func NewLLMSessionTitleGenerator(catalog *biz.LlmProviderModelUsecase, rt *provider.RoundTrip, lg loggateway.Logger) *LLMSessionTitleGenerator {
	return &LLMSessionTitleGenerator{catalog: catalog, rt: rt, lg: lg}
}

func (g *LLMSessionTitleGenerator) Generate(ctx context.Context, userMessage string) (string, error) {
	if g.catalog == nil {
		return "", nil
	}

	m, err := g.resolveModel(ctx)
	if err != nil {
		g.lg.Warn("session title: resolve model failed", loggateway.StepID("session.title_fail"), loggateway.Err(err))
		return "", apierror.Internal("SESSION", "session title: resolve model: %v", err)
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
		g.lg.Warn("session title: llm call failed", loggateway.StepID("session.title_fail"), loggateway.Err(err))
		return "", apierror.Internal("SESSION", "session title: llm call: %v", err)
	}

	var sb strings.Builder
	for resp := range ch {
		if resp.Error != nil {
			g.lg.Warn("session title: llm error", loggateway.StepID("session.title_fail"), loggateway.Str("error", resp.Error.Message))
			return "", apierror.Internal("SESSION", "session title: llm error: %s", resp.Error.Message)
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
	if runes := []rune(title); len(runes) > 50 {
		title = string(runes[:50])
	}
	return title, nil
}

func (g *LLMSessionTitleGenerator) resolveModel(ctx context.Context) (trpcmodel.Model, error) {
	models, err := g.catalog.List(ctx)
	if err != nil {
		return nil, err
	}
	if len(models) == 0 {
		return nil, apierror.NotFound("SESSION", "no models in catalog")
	}

	pm := biz.PickTitleModel(models)
	return provider.TRPCModelForProviderModel(ctx, g.catalog, g.rt, pm.Provider, pm.Model, g.lg)
}
