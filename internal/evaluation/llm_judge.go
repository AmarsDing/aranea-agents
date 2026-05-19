package evaluation

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/provider"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

const llmJudgePrompt = `You are an evaluation judge. Score how well ACTUAL matches EXPECTED for the given INPUT.
Reply with ONLY a decimal number between 0 and 1 (e.g. 0.85). No explanation.`

// NewLLMJudge builds an LLM-as-Judge scorer from the provider catalog (EP-RT-08).
// Env overrides: KRATOS_EVAL_JUDGE_PROVIDER, KRATOS_EVAL_JUDGE_MODEL
func NewLLMJudge(catalog *biz.LlmProviderModelUsecase, rt *provider.RoundTrip) LLMJudge {
	if catalog == nil || rt == nil {
		return nil
	}
	return func(ctx context.Context, input, expected, actual string) (float32, error) {
		m, err := resolveJudgeModel(ctx, catalog, rt)
		if err != nil {
			return 0, err
		}
		runCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		userMsg := fmt.Sprintf("INPUT:\n%s\n\nEXPECTED:\n%s\n\nACTUAL:\n%s", input, expected, actual)
		req := &trpcmodel.Request{
			Messages: []trpcmodel.Message{
				trpcmodel.NewSystemMessage(llmJudgePrompt),
				trpcmodel.NewUserMessage(userMsg),
			},
		}
		ch, err := m.GenerateContent(runCtx, req)
		if err != nil {
			return 0, fmt.Errorf("llm judge: %w", err)
		}
		var sb strings.Builder
		for resp := range ch {
			if resp.Error != nil {
				return 0, fmt.Errorf("llm judge: %s", resp.Error.Message)
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
		return parseJudgeScore(sb.String())
	}
}

func resolveJudgeModel(ctx context.Context, catalog *biz.LlmProviderModelUsecase, rt *provider.RoundTrip) (trpcmodel.Model, error) {
	prov := strings.TrimSpace(os.Getenv("KRATOS_EVAL_JUDGE_PROVIDER"))
	mod := strings.TrimSpace(os.Getenv("KRATOS_EVAL_JUDGE_MODEL"))
	if rt == nil {
		rt = &provider.RoundTrip{}
	}
	if prov != "" && mod != "" {
		return provider.TRPCModelForProviderModel(ctx, catalog, rt, prov, mod)
	}
	models, err := catalog.List(ctx)
	if err != nil {
		return nil, err
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("no models in catalog")
	}
	pm := pickJudgeModel(models)
	return provider.TRPCModelForProviderModel(ctx, catalog, rt, pm.Provider, pm.Model)
}

func pickJudgeModel(models []biz.ProviderModel) biz.ProviderModel {
	for _, m := range models {
		name := strings.ToLower(m.Model)
		if strings.Contains(name, "mini") || strings.Contains(name, "flash") || strings.Contains(name, "lite") {
			return m
		}
	}
	return models[0]
}

func parseJudgeScore(raw string) (float32, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)
	if i := strings.IndexAny(raw, "\n\r"); i >= 0 {
		raw = strings.TrimSpace(raw[:i])
	}
	f, err := strconv.ParseFloat(raw, 32)
	if err != nil {
		return 0, fmt.Errorf("llm judge: parse score %q: %w", raw, err)
	}
	if f < 0 {
		f = 0
	}
	if f > 1 {
		f = 1
	}
	return float32(f), nil
}
