package evaluation

import (
	"context"
	"fmt"
	"os"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/provider"
	"aranea-agents/pkg/loggateway"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// EvalLLMSettingsReader loads persisted eval LLM defaults (typically biz.SystemSettingRepo).
type EvalLLMSettingsReader interface {
	Get(ctx context.Context) (biz.SystemSetting, error)
}

func loadEvalLLMSetting(ctx context.Context, sys EvalLLMSettingsReader) biz.EvalLLMSetting {
	if sys == nil {
		return biz.EvalLLMSetting{}
	}
	row, err := sys.Get(ctx)
	if err != nil {
		return biz.EvalLLMSetting{}
	}
	return row.EvalLLM
}

// ResolveJudgeProviderModel exposes judge provider/model for usage attribution.
func ResolveJudgeProviderModel(ctx context.Context, sys EvalLLMSettingsReader) (string, string) {
	return resolveJudgeProviderModel(ctx, sys)
}

func resolveJudgeProviderModel(ctx context.Context, sys EvalLLMSettingsReader) (string, string) {
	prov := strings.TrimSpace(os.Getenv("KRATOS_EVAL_JUDGE_PROVIDER"))
	mod := strings.TrimSpace(os.Getenv("KRATOS_EVAL_JUDGE_MODEL"))
	if prov != "" && mod != "" {
		return prov, mod
	}
	st := loadEvalLLMSetting(ctx, sys)
	if st.JudgeConfigured() {
		return st.JudgeProvider, st.JudgeModel
	}
	prov = strings.TrimSpace(os.Getenv("KRATOS_EVAL_SIM_PROVIDER"))
	mod = strings.TrimSpace(os.Getenv("KRATOS_EVAL_SIM_MODEL"))
	if prov != "" && mod != "" {
		return prov, mod
	}
	if st.SimConfigured() {
		return st.SimProvider, st.SimModel
	}
	return "", ""
}

func resolveSimProviderModel(ctx context.Context, sys EvalLLMSettingsReader) (string, string) {
	prov := strings.TrimSpace(os.Getenv("KRATOS_EVAL_SIM_PROVIDER"))
	mod := strings.TrimSpace(os.Getenv("KRATOS_EVAL_SIM_MODEL"))
	if prov != "" && mod != "" {
		return prov, mod
	}
	st := loadEvalLLMSetting(ctx, sys)
	if st.SimConfigured() {
		return st.SimProvider, st.SimModel
	}
	prov = strings.TrimSpace(os.Getenv("KRATOS_EVAL_JUDGE_PROVIDER"))
	mod = strings.TrimSpace(os.Getenv("KRATOS_EVAL_JUDGE_MODEL"))
	if prov != "" && mod != "" {
		return prov, mod
	}
	if st.JudgeConfigured() {
		return st.JudgeProvider, st.JudgeModel
	}
	return "", ""
}

func resolveJudgeModel(ctx context.Context, catalog *biz.LlmProviderModelUsecase, rt *provider.RoundTrip, sys EvalLLMSettingsReader, lg loggateway.Logger) (trpcmodel.Model, error) {
	if rt == nil {
		rt = &provider.RoundTrip{}
	}
	if prov, mod := resolveJudgeProviderModel(ctx, sys); prov != "" && mod != "" {
		return provider.TRPCModelForProviderModel(ctx, catalog, rt, prov, mod, lg)
	}
	models, err := catalog.List(ctx)
	if err != nil {
		return nil, err
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("no models in catalog")
	}
	pm := pickJudgeModel(models)
	return provider.TRPCModelForProviderModel(ctx, catalog, rt, pm.Provider, pm.Model, lg)
}

func pickJudgeModel(models []biz.ProviderModel) biz.ProviderModel {
	if len(models) == 0 {
		return biz.ProviderModel{}
	}
	for _, m := range models {
		name := strings.ToLower(m.Model)
		if strings.Contains(name, "mini") || strings.Contains(name, "flash") || strings.Contains(name, "lite") {
			return m
		}
	}
	return models[0]
}

func resolveSimModel(ctx context.Context, catalog *biz.LlmProviderModelUsecase, rt *provider.RoundTrip, sys EvalLLMSettingsReader, lg loggateway.Logger) (trpcmodel.Model, error) {
	if prov, mod := resolveSimProviderModel(ctx, sys); prov != "" && mod != "" {
		return provider.TRPCModelForProviderModel(ctx, catalog, rt, prov, mod, lg)
	}
	return resolveJudgeModel(ctx, catalog, rt, sys, lg)
}
