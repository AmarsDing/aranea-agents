package service_test

import (
	"testing"
	"time"

	v1 "aranea-agents/api/kratos/system_setting/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/service"
)

func TestToProtoSystemSettings(t *testing.T) {
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	row := biz.SystemSetting{
		RootDirectory:                     "/data",
		WorkDirectory:                     "/data/work",
		GlobalMonthlyMicroUSD:             5000000,
		A2APublicBaseURL:                  "https://a2a.example.com",
		CredentialEncryptionKeyConfigured: true,
		MCPAllowAdHocHTTP:                 true,
		KnowledgeEmbed: biz.KnowledgeEmbedSetting{
			Provider:  "gemini",
			BaseURL:   "",
			Model:     "gemini-embedding-001",
			Dim:       768,
			HasAPIKey: true,
		},
		EvalLLM: biz.EvalLLMSetting{
			SimProvider:   "openai",
			SimModel:      "gpt-4o",
			JudgeProvider: "openai",
			JudgeModel:    "gpt-4o",
		},
		WebResearch: biz.WebResearchSetting{
			Provider:    "tavily",
			HasAPIKey:   true,
			MaxResults:  5,
			FetchTop:    3,
			SearchDepth: "basic",
			TimeoutSec:  30,
			HTTPProxy:   "",
		},
		UpdateTime: now,
	}

	got := service.ToProtoSystemSettings(row)

	if got.WorkDirectory != row.WorkDirectory {
		t.Errorf("WorkDirectory = %q, want %q", got.WorkDirectory, row.WorkDirectory)
	}
	if got.RootDirectory != row.RootDirectory {
		t.Errorf("RootDirectory = %q, want %q", got.RootDirectory, row.RootDirectory)
	}
	if got.GlobalMonthlyMicroUsd != row.GlobalMonthlyMicroUSD {
		t.Errorf("GlobalMonthlyMicroUsd = %d, want %d", got.GlobalMonthlyMicroUsd, row.GlobalMonthlyMicroUSD)
	}
	if got.A2APublicBaseUrl != row.A2APublicBaseURL {
		t.Errorf("A2APublicBaseUrl = %q, want %q", got.A2APublicBaseUrl, row.A2APublicBaseURL)
	}
	if got.CredentialEncryptionKeyConfigured != row.CredentialEncryptionKeyConfigured {
		t.Errorf("CredentialEncryptionKeyConfigured = %v, want %v", got.CredentialEncryptionKeyConfigured, row.CredentialEncryptionKeyConfigured)
	}
	if got.McpAllowAdhocHttp != row.MCPAllowAdHocHTTP {
		t.Errorf("McpAllowAdhocHttp = %v, want %v", got.McpAllowAdhocHttp, row.MCPAllowAdHocHTTP)
	}
	if got.KnowledgeEmbed == nil {
		t.Fatal("KnowledgeEmbed is nil")
	}
	if got.KnowledgeEmbed.Provider != "gemini" {
		t.Errorf("KnowledgeEmbed.Provider = %q, want %q", got.KnowledgeEmbed.Provider, "gemini")
	}
	if got.EvalLlm == nil {
		t.Fatal("EvalLlm is nil")
	}
	if got.EvalLlm.SimProvider != "openai" {
		t.Errorf("EvalLlm.SimProvider = %q, want %q", got.EvalLlm.SimProvider, "openai")
	}
	if got.WebResearch == nil {
		t.Fatal("WebResearch is nil")
	}
	if got.WebResearch.Provider != "tavily" {
		t.Errorf("WebResearch.Provider = %q, want %q", got.WebResearch.Provider, "tavily")
	}
	if got.UpdateTime == nil {
		t.Fatal("UpdateTime is nil")
	}
}

func TestToProtoWebResearch(t *testing.T) {
	tests := []struct {
		name string
		row  biz.WebResearchSetting
		want *v1.WebResearchSettings
	}{
		{
			name: "configured",
			row: biz.WebResearchSetting{
				Provider:    "tavily",
				HasAPIKey:   true,
				MaxResults:  10,
				FetchTop:    5,
				SearchDepth: "advanced",
				TimeoutSec:  60,
				HTTPProxy:   "http://proxy:8080",
			},
			want: &v1.WebResearchSettings{
				Provider:    "tavily",
				HasApiKey:   true,
				MaxResults:  10,
				FetchTop:    5,
				SearchDepth: "advanced",
				TimeoutSec:  60,
				HttpProxy:   "http://proxy:8080",
				Configured:  true,
			},
		},
		{
			name: "empty",
			row:  biz.WebResearchSetting{},
			want: &v1.WebResearchSettings{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.ToProtoWebResearch(tt.row)
			if got.Provider != tt.want.Provider {
				t.Errorf("Provider = %q, want %q", got.Provider, tt.want.Provider)
			}
			if got.MaxResults != tt.want.MaxResults {
				t.Errorf("MaxResults = %d, want %d", got.MaxResults, tt.want.MaxResults)
			}
			if got.FetchTop != tt.want.FetchTop {
				t.Errorf("FetchTop = %d, want %d", got.FetchTop, tt.want.FetchTop)
			}
			if got.SearchDepth != tt.want.SearchDepth {
				t.Errorf("SearchDepth = %q, want %q", got.SearchDepth, tt.want.SearchDepth)
			}
			if got.TimeoutSec != tt.want.TimeoutSec {
				t.Errorf("TimeoutSec = %d, want %d", got.TimeoutSec, tt.want.TimeoutSec)
			}
			if got.HttpProxy != tt.want.HttpProxy {
				t.Errorf("HttpProxy = %q, want %q", got.HttpProxy, tt.want.HttpProxy)
			}
			if got.HasApiKey != tt.want.HasApiKey {
				t.Errorf("HasApiKey = %v, want %v", got.HasApiKey, tt.want.HasApiKey)
			}
		})
	}
}

func TestToProtoKnowledgeEmbed(t *testing.T) {
	tests := []struct {
		name string
		row  biz.KnowledgeEmbedSetting
	}{
		{
			name: "with_provider_and_key",
			row: biz.KnowledgeEmbedSetting{
				Provider:  "gemini",
				Model:     "gemini-embedding-001",
				Dim:       768,
				HasAPIKey: true,
			},
		},
		{
			name: "huggingface_with_base_url",
			row: biz.KnowledgeEmbedSetting{
				Provider: "huggingface",
				BaseURL:  "http://localhost:8080",
				Model:    "bge-large",
				Dim:      1024,
			},
		},
		{
			name: "empty",
			row:  biz.KnowledgeEmbedSetting{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.ToProtoKnowledgeEmbed(tt.row)
			if got.Provider != tt.row.Provider {
				t.Errorf("Provider = %q, want %q", got.Provider, tt.row.Provider)
			}
			if got.BaseUrl != tt.row.BaseURL {
				t.Errorf("BaseUrl = %q, want %q", got.BaseUrl, tt.row.BaseURL)
			}
			if got.Model != tt.row.Model {
				t.Errorf("Model = %q, want %q", got.Model, tt.row.Model)
			}
			if got.Dim != int32(tt.row.Dim) {
				t.Errorf("Dim = %d, want %d", got.Dim, tt.row.Dim)
			}
			if got.HasApiKey != tt.row.HasAPIKey {
				t.Errorf("HasApiKey = %v, want %v", got.HasApiKey, tt.row.HasAPIKey)
			}
		})
	}
}

func TestToProtoEvalLLM(t *testing.T) {
	tests := []struct {
		name string
		row  biz.EvalLLMSetting
	}{
		{
			name: "both_configured",
			row: biz.EvalLLMSetting{
				SimProvider:   "openai",
				SimModel:      "gpt-4o",
				JudgeProvider: "anthropic",
				JudgeModel:    "claude-3",
			},
		},
		{
			name: "sim_only",
			row: biz.EvalLLMSetting{
				SimProvider: "openai",
				SimModel:    "gpt-4o",
			},
		},
		{
			name: "empty",
			row:  biz.EvalLLMSetting{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.ToProtoEvalLLM(tt.row)
			if got.SimProvider != tt.row.SimProvider {
				t.Errorf("SimProvider = %q, want %q", got.SimProvider, tt.row.SimProvider)
			}
			if got.SimModel != tt.row.SimModel {
				t.Errorf("SimModel = %q, want %q", got.SimModel, tt.row.SimModel)
			}
			if got.JudgeProvider != tt.row.JudgeProvider {
				t.Errorf("JudgeProvider = %q, want %q", got.JudgeProvider, tt.row.JudgeProvider)
			}
			if got.JudgeModel != tt.row.JudgeModel {
				t.Errorf("JudgeModel = %q, want %q", got.JudgeModel, tt.row.JudgeModel)
			}
		})
	}
}

func TestToProtoEvalLLM_Configured(t *testing.T) {
	tests := []struct {
		name           string
		row            biz.EvalLLMSetting
		wantConfigured bool
	}{
		{
			name:           "both_configured",
			row:            biz.EvalLLMSetting{SimProvider: "a", SimModel: "b", JudgeProvider: "c", JudgeModel: "d"},
			wantConfigured: true,
		},
		{
			name:           "sim_only",
			row:            biz.EvalLLMSetting{SimProvider: "a", SimModel: "b"},
			wantConfigured: true,
		},
		{
			name:           "judge_only",
			row:            biz.EvalLLMSetting{JudgeProvider: "c", JudgeModel: "d"},
			wantConfigured: true,
		},
		{
			name:           "none_configured",
			row:            biz.EvalLLMSetting{},
			wantConfigured: false,
		},
		{
			name:           "sim_missing_model",
			row:            biz.EvalLLMSetting{SimProvider: "a"},
			wantConfigured: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.ToProtoEvalLLM(tt.row)
			if got.Configured != tt.wantConfigured {
				t.Errorf("Configured = %v, want %v", got.Configured, tt.wantConfigured)
			}
		})
	}
}

func TestHasWebResearchUpdate(t *testing.T) {
	tests := []struct {
		name string
		req  *v1.UpdateSystemSettingsRequest
		want bool
	}{
		{"nil_request", nil, false},
		{"empty_request", &v1.UpdateSystemSettingsRequest{}, false},
		{"provider_set", &v1.UpdateSystemSettingsRequest{WebResearchProvider: "tavily"}, true},
		{"api_key_set", &v1.UpdateSystemSettingsRequest{WebResearchApiKey: "sk-xxx"}, true},
		{"max_results_set", &v1.UpdateSystemSettingsRequest{WebResearchMaxResults: 10}, true},
		{"fetch_top_set", &v1.UpdateSystemSettingsRequest{WebResearchFetchTop: 5}, true},
		{"search_depth_set", &v1.UpdateSystemSettingsRequest{WebResearchSearchDepth: "advanced"}, true},
		{"timeout_set", &v1.UpdateSystemSettingsRequest{WebResearchTimeoutSec: 60}, true},
		{"http_proxy_set", &v1.UpdateSystemSettingsRequest{WebResearchHttpProxy: "http://proxy"}, true},
		{"api_key_spaces_only", &v1.UpdateSystemSettingsRequest{WebResearchApiKey: "   "}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := service.HasWebResearchUpdate(tt.req); got != tt.want {
				t.Errorf("HasWebResearchUpdate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHasKnowledgeEmbedUpdate(t *testing.T) {
	tests := []struct {
		name string
		req  *v1.UpdateSystemSettingsRequest
		want bool
	}{
		{"nil_request", nil, false},
		{"empty_request", &v1.UpdateSystemSettingsRequest{}, false},
		{"provider_set", &v1.UpdateSystemSettingsRequest{KnowledgeEmbedProvider: "gemini"}, true},
		{"base_url_set", &v1.UpdateSystemSettingsRequest{KnowledgeEmbedBaseUrl: "http://localhost"}, true},
		{"model_set", &v1.UpdateSystemSettingsRequest{KnowledgeEmbedModel: "text-embedding-3"}, true},
		{"dim_set", &v1.UpdateSystemSettingsRequest{KnowledgeEmbedDim: 1536}, true},
		{"api_key_set", &v1.UpdateSystemSettingsRequest{KnowledgeEmbedApiKey: "sk-xxx"}, true},
		{"api_key_spaces_only", &v1.UpdateSystemSettingsRequest{KnowledgeEmbedApiKey: "   "}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := service.HasKnowledgeEmbedUpdate(tt.req); got != tt.want {
				t.Errorf("HasKnowledgeEmbedUpdate() = %v, want %v", got, tt.want)
			}
		})
	}
}
