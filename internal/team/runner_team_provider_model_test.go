package team

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

type stubTeamModelCatalog struct {
	models  []biz.ProviderModel
	getErr  error
	listErr error
}

func (s *stubTeamModelCatalog) GetByProviderAndModel(_ context.Context, provider, model string) (biz.ProviderModel, error) {
	if s.getErr != nil {
		return biz.ProviderModel{}, s.getErr
	}
	for _, m := range s.models {
		if m.Provider == provider && m.Model == model {
			return m, nil
		}
	}
	return biz.ProviderModel{}, biz.ErrProviderModelNotFound
}

func (s *stubTeamModelCatalog) List(context.Context) ([]biz.ProviderModel, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.models, nil
}

type stubTeamRefineLLM struct {
	setting biz.RefineLLMSetting
	err     error
}

func (s *stubTeamRefineLLM) GetRefineLLM(context.Context) (biz.RefineLLMSetting, error) {
	return s.setting, s.err
}

func TestResolveTeamProviderModel_AgentModelNotInCatalog_FallsBack(t *testing.T) {
	// 复现线上错位场景：agent 配置的 deepseek/deepseek-chat 不在目录中，
	// 执行链路 build 期回退到系统默认模型，观测链路也必须回退到同一模型。
	catalog := &stubTeamModelCatalog{models: []biz.ProviderModel{
		{Provider: "deepseek", Model: "deepseek-v4-flash", Enabled: true},
	}}
	refine := &stubTeamRefineLLM{setting: biz.RefineLLMSetting{Provider: "deepseek", Model: "deepseek-v4-flash"}}
	sess := biz.Session{}
	ag := biz.Agent{Provider: "deepseek", Model: "deepseek-chat"}

	prov, mod := resolveTeamProviderModel(context.Background(), catalog, refine, loggateway.NewNoop(), "", "", sess, ag)
	if prov != "deepseek" || mod != "deepseek-v4-flash" {
		t.Fatalf("expected fallback to deepseek/deepseek-v4-flash, got %s/%s", prov, mod)
	}
}

func TestResolveTeamProviderModel_AgentModelInCatalog_KeepsOriginal(t *testing.T) {
	catalog := &stubTeamModelCatalog{models: []biz.ProviderModel{
		{Provider: "deepseek", Model: "deepseek-v4-flash", Enabled: true},
	}}
	sess := biz.Session{}
	ag := biz.Agent{Provider: "deepseek", Model: "deepseek-v4-flash"}

	prov, mod := resolveTeamProviderModel(context.Background(), catalog, nil, loggateway.NewNoop(), "", "", sess, ag)
	if prov != "deepseek" || mod != "deepseek-v4-flash" {
		t.Fatalf("expected agent model kept, got %s/%s", prov, mod)
	}
}

func TestResolveTeamProviderModel_ExplicitOption_NotInCatalog_FallsBack(t *testing.T) {
	catalog := &stubTeamModelCatalog{models: []biz.ProviderModel{
		{Provider: "openai", Model: "gpt-4o-mini", Enabled: true},
	}}
	sess := biz.Session{DefaultProvider: "deepseek", DefaultModel: "deepseek-v4-flash"}
	ag := biz.Agent{Provider: "deepseek", Model: "deepseek-v4-flash"}

	// 显式指定的模型不在目录中 → 重置 → 无 refine → 落到目录首个启用模型。
	prov, mod := resolveTeamProviderModel(context.Background(), catalog, nil, loggateway.NewNoop(), "deepseek", "deepseek-chat", sess, ag)
	if prov != "openai" || mod != "gpt-4o-mini" {
		t.Fatalf("expected fallback to first enabled openai/gpt-4o-mini, got %s/%s", prov, mod)
	}
}

func TestResolveTeamProviderModel_PriorityChain(t *testing.T) {
	catalog := &stubTeamModelCatalog{models: []biz.ProviderModel{
		{Provider: "deepseek", Model: "deepseek-v4-flash", Enabled: true},
		{Provider: "openai", Model: "gpt-4o-mini", Enabled: true},
	}}
	sess := biz.Session{DefaultProvider: "openai", DefaultModel: "gpt-4o-mini"}
	ag := biz.Agent{Provider: "deepseek", Model: "deepseek-v4-flash"}

	// provOpt 优先于 session 与 agent 默认。
	prov, mod := resolveTeamProviderModel(context.Background(), catalog, nil, loggateway.NewNoop(), "openai", "gpt-4o-mini", sess, ag)
	if prov != "openai" || mod != "gpt-4o-mini" {
		t.Fatalf("expected explicit option to win, got %s/%s", prov, mod)
	}

	// session 默认优先于 agent 默认。
	prov, mod = resolveTeamProviderModel(context.Background(), catalog, nil, loggateway.NewNoop(), "", "", sess, ag)
	if prov != "openai" || mod != "gpt-4o-mini" {
		t.Fatalf("expected session default to win over agent default, got %s/%s", prov, mod)
	}
}

func TestResolveTeamProviderModel_CatalogDBError_KeepsOriginal(t *testing.T) {
	catalog := &stubTeamModelCatalog{getErr: errors.New("db down")}
	sess := biz.Session{}
	ag := biz.Agent{Provider: "deepseek", Model: "deepseek-chat"}

	// 目录查询故障（非 NotFound）不阻断：保留原值，与单 agent 路径一致。
	prov, mod := resolveTeamProviderModel(context.Background(), catalog, nil, loggateway.NewNoop(), "", "", sess, ag)
	if prov != "deepseek" || mod != "deepseek-chat" {
		t.Fatalf("expected original values on catalog DB error, got %s/%s", prov, mod)
	}
}
