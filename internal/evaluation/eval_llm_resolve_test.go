package evaluation

import (
	"context"
	"os"
	"testing"

	"aranea-agents/internal/biz"
)

type stubEvalLLMReader struct {
	setting biz.EvalLLMSetting
}

func (s stubEvalLLMReader) Get(_ context.Context) (biz.SystemSetting, error) {
	return biz.SystemSetting{EvalLLM: s.setting}, nil
}

func TestResolveJudgeProviderModelFromSettings(t *testing.T) {
	t.Setenv("KRATOS_EVAL_JUDGE_PROVIDER", "")
	t.Setenv("KRATOS_EVAL_JUDGE_MODEL", "")
	t.Setenv("KRATOS_EVAL_SIM_PROVIDER", "")
	t.Setenv("KRATOS_EVAL_SIM_MODEL", "")

	reader := stubEvalLLMReader{setting: biz.EvalLLMSetting{
		JudgeProvider: "openai",
		JudgeModel:    "gpt-4o-mini",
	}}
	prov, mod := resolveJudgeProviderModel(context.Background(), reader)
	if prov != "openai" || mod != "gpt-4o-mini" {
		t.Fatalf("judge from settings: %s/%s", prov, mod)
	}
}

func TestResolveSimProviderModelFallsBackToJudgeSettings(t *testing.T) {
	t.Setenv("KRATOS_EVAL_SIM_PROVIDER", "")
	t.Setenv("KRATOS_EVAL_SIM_MODEL", "")

	reader := stubEvalLLMReader{setting: biz.EvalLLMSetting{
		JudgeProvider: "anthropic",
		JudgeModel:    "claude-3-haiku",
	}}
	prov, mod := resolveSimProviderModel(context.Background(), reader)
	if prov != "anthropic" || mod != "claude-3-haiku" {
		t.Fatalf("sim fallback to judge settings: %s/%s", prov, mod)
	}
}

func TestEnvOverridesSettings(t *testing.T) {
	t.Setenv("KRATOS_EVAL_SIM_PROVIDER", "env-prov")
	t.Setenv("KRATOS_EVAL_SIM_MODEL", "env-mod")

	reader := stubEvalLLMReader{setting: biz.EvalLLMSetting{
		SimProvider: "db-prov",
		SimModel:    "db-mod",
	}}
	prov, mod := resolveSimProviderModel(context.Background(), reader)
	if prov != "env-prov" || mod != "env-mod" {
		t.Fatalf("env should win: %s/%s", prov, mod)
	}
	_ = os.Unsetenv("KRATOS_EVAL_SIM_PROVIDER")
	_ = os.Unsetenv("KRATOS_EVAL_SIM_MODEL")
}
