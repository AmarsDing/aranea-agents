package service

import (
	"context"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/session"
	"aranea-agents/internal/provider"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

const titleGenPrompt = `根据用户的第一条消息，生成一个简短的对话标题（不超过20字）。只返回标题文本，不要解释、不要引号。`

type LLMSessionTitleGenerator struct {
	catalog *biz.LlmProviderModelUsecase
	rt      *provider.RoundTrip
	// usageRef lazily resolves the usage usecase for aux title-generation
	// recording (P1-2). Late binding breaks a wire DI cycle: the title
	// generator sits upstream of UsageUsecase (SessionUsecase → generator).
	usageRef *biz.UsageUsecaseRef
	lg       loggateway.Logger
}

func NewLLMSessionTitleGenerator(catalog *biz.LlmProviderModelUsecase, rt *provider.RoundTrip, usageRef *biz.UsageUsecaseRef, lg loggateway.Logger) *LLMSessionTitleGenerator {
	return &LLMSessionTitleGenerator{catalog: catalog, rt: rt, usageRef: usageRef, lg: lg}
}

func (g *LLMSessionTitleGenerator) Generate(ctx context.Context, req session.TitleGenRequest) (string, error) {
	if g.catalog == nil {
		return "", nil
	}

	m, prov, mod, err := g.resolveModel(ctx)
	if err != nil {
		g.lg.Warn("session title: resolve model failed", loggateway.StepID("session.title_fail"), loggateway.Err(err))
		return "", apierror.Internal(apierror.DomainSession, "session title: resolve model failed")
	}

	// Timeout is owned by the caller (generateTitleAsync wraps with 15s).
	llmReq := &trpcmodel.Request{
		Messages: []trpcmodel.Message{
			trpcmodel.NewSystemMessage(titleGenPrompt),
			trpcmodel.NewUserMessage(req.UserMessage),
		},
	}

	start := time.Now()
	ch, err := m.GenerateContent(ctx, llmReq)
	if err != nil {
		g.lg.Warn("session title: llm call failed", loggateway.StepID("session.title_fail"), loggateway.Err(err))
		return "", apierror.Internal(apierror.DomainSession, "session title: llm call failed")
	}

	var sb strings.Builder
	var usage *trpcmodel.Usage
	for resp := range ch {
		if resp.Error != nil {
			g.lg.Warn("session title: llm error", loggateway.StepID("session.title_fail"), loggateway.Str("error", resp.Error.Message))
			return "", apierror.Internal(apierror.DomainSession, "session title: llm error")
		}
		// 单轮流式：usage 通常在末块给出（累计值），取最后一个非空。
		if resp.Usage != nil {
			usage = resp.Usage
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
	g.recordUsage(ctx, req, prov, mod, usage, time.Since(start))

	title := strings.TrimSpace(sb.String())
	if runes := []rune(title); len(runes) > 50 {
		title = string(runes[:50])
	}
	return title, nil
}

// recordUsage persists the title-generation aux usage (P1-2, 2026-08-19).
// Zero-token responses (provider returned no usage) are skipped.
func (g *LLMSessionTitleGenerator) recordUsage(ctx context.Context, req session.TitleGenRequest, prov, mod string, usage *trpcmodel.Usage, latency time.Duration) {
	if usage == nil {
		return
	}
	u := g.usageRef.Get()
	if u == nil {
		return
	}
	if usage.PromptTokens <= 0 && usage.CompletionTokens <= 0 {
		return
	}
	if err := u.RecordAuxLLMUsage(ctx, biz.AuxLLMUsageInput{
		Kind:          biz.UsageKindAuxTitle,
		SessionID:     req.SessionID,
		AgentID:       req.AgentID,
		Provider:      prov,
		Model:         mod,
		Status:        "success",
		PromptTok:     usage.PromptTokens,
		CompletionTok: usage.CompletionTokens,
		CachedTok:     usage.PromptTokensDetails.CachedTokens,
		UsageSource:   biz.UsageSourceResponse,
		Latency:       latency,
	}); err != nil {
		g.lg.Warn("session title: usage record failed", loggateway.StepID("session.title"), loggateway.SessionID(req.SessionID), loggateway.Err(err))
	}
}

func (g *LLMSessionTitleGenerator) resolveModel(ctx context.Context) (trpcmodel.Model, string, string, error) {
	models, err := g.catalog.List(ctx)
	if err != nil {
		return nil, "", "", err
	}
	if len(models) == 0 {
		return nil, "", "", apierror.NotFound(apierror.DomainSession, "no models in catalog")
	}

	pm, ok := biz.PickTitleModel(models)
	if !ok {
		return nil, "", "", apierror.NotFound(apierror.DomainSession, "no models in catalog")
	}
	m, err := provider.TRPCModelForProviderModel(ctx, g.catalog, g.rt, pm.Provider, pm.Model, g.lg)
	if err != nil {
		return nil, "", "", err
	}
	return m, pm.Provider, pm.Model, nil
}
