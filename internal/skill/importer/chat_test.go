package importer

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

var errAny = errors.New("any-error-sentinel")

func TestChatCompletionsEndpoint(t *testing.T) {
	tests := []struct {
		name string
		base string
		want string
	}{
		{
			name: "deepseek_provider",
			base: "https://api.deepseek.com",
			want: "https://api.deepseek.com/chat/completions",
		},
		{
			name: "deepseek_provider_case_insensitive",
			base: "https://API.DEEPSEEK.COM",
			want: "https://API.DEEPSEEK.COM/chat/completions",
		},
		{
			name: "openai_provider",
			base: "https://api.openai.com",
			want: "https://api.openai.com/v1/chat/completions",
		},
		{
			name: "trailing_slash",
			base: "https://api.openai.com/",
			want: "https://api.openai.com/v1/chat/completions",
		},
		{
			name: "already_has_v1",
			base: "https://api.openai.com/v1",
			want: "https://api.openai.com/v1/chat/completions",
		},
		{
			name: "already_has_v1_trailing_slash",
			base: "https://api.openai.com/v1/",
			want: "https://api.openai.com/v1/chat/completions",
		},
		{
			name: "already_has_chat_completions",
			base: "https://api.openai.com/v1/chat/completions",
			want: "https://api.openai.com/v1/chat/completions",
		},
		{
			name: "deepseek_with_trailing_slash",
			base: "https://api.deepseek.com/",
			want: "https://api.deepseek.com/chat/completions",
		},
		{
			name: "custom_base",
			base: "https://my-llm.example.com",
			want: "https://my-llm.example.com/v1/chat/completions",
		},
		{
			name: "whitespace_padding",
			base: "  https://api.openai.com  ",
			want: "https://api.openai.com/v1/chat/completions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := chatCompletionsEndpoint(tt.base)
			if got != tt.want {
				t.Errorf("chatCompletionsEndpoint(%q) = %q, want %q", tt.base, got, tt.want)
			}
		})
	}
}

func TestAnthropicMessagesEndpoint(t *testing.T) {
	tests := []struct {
		name string
		base string
		want string
	}{
		{
			name: "standard_base",
			base: "https://api.anthropic.com",
			want: "https://api.anthropic.com/v1/messages",
		},
		{
			name: "trailing_slash",
			base: "https://api.anthropic.com/",
			want: "https://api.anthropic.com/v1/messages",
		},
		{
			name: "already_has_v1",
			base: "https://api.anthropic.com/v1",
			want: "https://api.anthropic.com/v1/messages",
		},
		{
			name: "already_has_v1_trailing_slash",
			base: "https://api.anthropic.com/v1/",
			want: "https://api.anthropic.com/v1/messages",
		},
		{
			name: "already_has_messages",
			base: "https://api.anthropic.com/v1/messages",
			want: "https://api.anthropic.com/v1/messages",
		},
		{
			name: "whitespace_padding",
			base: "  https://api.anthropic.com  ",
			want: "https://api.anthropic.com/v1/messages",
		},
		{
			name: "custom_base",
			base: "https://my-proxy.example.com",
			want: "https://my-proxy.example.com/v1/messages",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := anthropicMessagesEndpoint(tt.base)
			if got != tt.want {
				t.Errorf("anthropicMessagesEndpoint(%q) = %q, want %q", tt.base, got, tt.want)
			}
		})
	}
}

func TestBuildSimilarityPrompt(t *testing.T) {
	candidate := candidateState{
		public: biz.SkillImportCandidate{
			Name:        "Daily Report",
			Description: "Generates daily reports",
		},
		body: "This skill generates daily reports from data sources.",
	}
	source := biz.SkillSimilaritySource{
		Name:        "Weekly Summary",
		Description: "Summarizes weekly metrics",
		Body:        "This skill creates weekly summaries.",
	}

	prompt := buildSimilarityPrompt(candidate, source)

	if !strings.Contains(prompt, candidate.public.Name) {
		t.Error("prompt should contain candidate name")
	}
	if !strings.Contains(prompt, candidate.public.Description) {
		t.Error("prompt should contain candidate description")
	}
	if !strings.Contains(prompt, candidate.body) {
		t.Error("prompt should contain candidate body")
	}
	if !strings.Contains(prompt, source.Name) {
		t.Error("prompt should contain existing skill name")
	}
	if !strings.Contains(prompt, source.Description) {
		t.Error("prompt should contain existing skill description")
	}
	if !strings.Contains(prompt, source.Body) {
		t.Error("prompt should contain existing skill body")
	}
	if strings.TrimSpace(prompt) == "" {
		t.Error("prompt should not be empty")
	}
	if !strings.Contains(prompt, "similarity_score") {
		t.Error("prompt should mention similarity_score field")
	}
	if !strings.Contains(prompt, "recommendation") {
		t.Error("prompt should mention recommendation field")
	}
}

func TestBuildRefinePrompt(t *testing.T) {
	group := biz.SkillConflictGroup{
		GroupID: "g1",
		ExistingSkills: []biz.SkillSimilaritySource{
			{Name: "Existing Skill", Description: "An existing skill", Body: "Body of existing skill"},
		},
	}
	candidates := []candidateState{
		{
			public: biz.SkillImportCandidate{
				Name:        "New Skill A",
				Description: "A new candidate skill",
			},
			body: "Body of new skill A",
		},
		{
			public: biz.SkillImportCandidate{
				Name:        "New Skill B",
				Description: "Another candidate skill",
			},
			body: "Body of new skill B",
		},
	}

	prompt := buildRefinePrompt(group, candidates, "Preserve all tool references")

	if !strings.Contains(prompt, "New Skill A") {
		t.Error("prompt should contain first candidate name")
	}
	if !strings.Contains(prompt, "New Skill B") {
		t.Error("prompt should contain second candidate name")
	}
	if !strings.Contains(prompt, "Existing Skill") {
		t.Error("prompt should contain existing skill name")
	}
	if !strings.Contains(prompt, "Preserve all tool references") {
		t.Error("prompt should contain instructions")
	}
	if !strings.Contains(prompt, "merged_name") {
		t.Error("prompt should mention merged_name field")
	}
	if strings.TrimSpace(prompt) == "" {
		t.Error("prompt should not be empty")
	}
}

func TestBuildRefinePrompt_EmptyInstructions(t *testing.T) {
	group := biz.SkillConflictGroup{
		GroupID: "g1",
	}
	candidates := []candidateState{
		{
			public: biz.SkillImportCandidate{Name: "X", Description: "D"},
			body:   "B",
		},
	}

	prompt := buildRefinePrompt(group, candidates, "")

	if strings.Contains(prompt, "额外要求") {
		t.Error("prompt should not contain extra instructions section when instructions is empty")
	}
}

func TestParseRefineResult(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    biz.SkillRefineResult
		wantErr error
	}{
		{
			name: "valid_json",
			raw:  `{"merged_name":"Merged","merged_description":"Desc","merged_body":"Body","merged_tags":[{"name":"t1","source":"user"}]}`,
			want: biz.SkillRefineResult{
				MergedName:        "Merged",
				MergedDescription: "Desc",
				MergedBody:        "Body",
				MergedTags:        []biz.SkillTag{{Name: "t1", Source: "user"}},
			},
			wantErr: nil,
		},
		{
			name:    "missing_merged_name",
			raw:     `{"merged_description":"Desc","merged_body":"Body"}`,
			wantErr: ErrRefineResultInvalid,
		},
		{
			name:    "missing_merged_body",
			raw:     `{"merged_name":"Name","merged_description":"Desc"}`,
			wantErr: ErrRefineResultInvalid,
		},
		{
			name:    "empty_merged_name",
			raw:     `{"merged_name":"  ","merged_body":"Body"}`,
			wantErr: ErrRefineResultInvalid,
		},
		{
			name:    "empty_merged_body",
			raw:     `{"merged_name":"Name","merged_body":"  "}`,
			wantErr: ErrRefineResultInvalid,
		},
		{
			name:    "invalid_json",
			raw:     `not json at all`,
			wantErr: errAny,
		},
		{
			name: "extra_fields_ignored",
			raw:  `{"merged_name":"N","merged_body":"B","extra_field":"ignored"}`,
			want: biz.SkillRefineResult{
				MergedName: "N",
				MergedBody: "B",
			},
			wantErr: nil,
		},
		{
			name: "markdown_wrapped_json",
			raw:  "```json\n{\"merged_name\":\"N\",\"merged_body\":\"B\"}\n```",
			want: biz.SkillRefineResult{
				MergedName: "N",
				MergedBody: "B",
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseRefineResult(tt.raw)
			if tt.wantErr != nil {
				if errors.Is(tt.wantErr, errAny) {
					if err == nil {
						t.Error("expected an error, got nil")
					}
				} else if !errors.Is(err, tt.wantErr) {
					t.Errorf("parseRefineResult() error = %v, wantErr %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.MergedName != tt.want.MergedName {
				t.Errorf("MergedName = %q, want %q", got.MergedName, tt.want.MergedName)
			}
			if got.MergedBody != tt.want.MergedBody {
				t.Errorf("MergedBody = %q, want %q", got.MergedBody, tt.want.MergedBody)
			}
			if tt.want.MergedDescription != "" && got.MergedDescription != tt.want.MergedDescription {
				t.Errorf("MergedDescription = %q, want %q", got.MergedDescription, tt.want.MergedDescription)
			}
			if len(tt.want.MergedTags) > 0 && len(got.MergedTags) != len(tt.want.MergedTags) {
				t.Errorf("MergedTags length = %d, want %d", len(got.MergedTags), len(tt.want.MergedTags))
			}
		})
	}
}

func TestProviderModelHasCredentials(t *testing.T) {
	tests := []struct {
		name    string
		cfgJSON string
		want    bool
	}{
		{
			name:    "both_present",
			cfgJSON: `{"api_base_url":"https://api.openai.com","api_key":"sk-123"}`,
			want:    true,
		},
		{
			name:    "api_key_set_true",
			cfgJSON: `{"api_base_url":"https://api.openai.com","api_key_set":true}`,
			want:    true,
		},
		{
			name:    "api_key_set_false",
			cfgJSON: `{"api_base_url":"https://api.openai.com","api_key_set":false}`,
			want:    false,
		},
		{
			name:    "missing_api_key",
			cfgJSON: `{"api_base_url":"https://api.openai.com"}`,
			want:    false,
		},
		{
			name:    "missing_base_url",
			cfgJSON: `{"api_key":"sk-123"}`,
			want:    false,
		},
		{
			name:    "empty_strings",
			cfgJSON: `{"api_base_url":"","api_key":""}`,
			want:    false,
		},
		{
			name:    "whitespace_only",
			cfgJSON: `{"api_base_url":"   ","api_key":"  "}`,
			want:    false,
		},
		{
			name:    "invalid_json",
			cfgJSON: `not json`,
			want:    false,
		},
		{
			name:    "empty_json",
			cfgJSON: `{}`,
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := providerModelHasCredentials(tt.cfgJSON)
			if got != tt.want {
				t.Errorf("providerModelHasCredentials() = %v, want %v", got, tt.want)
			}
		})
	}
}

type stubLlmLister struct {
	listRows []biz.ProviderModel
	listErr  error
	resolved map[string]biz.ProviderModel
}

func stubKey(provider, model string) string {
	return provider + "/" + model
}

func (s *stubLlmLister) List(_ context.Context) ([]biz.ProviderModel, error) {
	return s.listRows, s.listErr
}

func (s *stubLlmLister) GetByProviderAndModel(_ context.Context, provider, model string) (biz.ProviderModel, error) {
	if m, ok := s.resolved[stubKey(provider, model)]; ok {
		return m, nil
	}
	return biz.ProviderModel{}, nil
}

func TestResolveChatModel(t *testing.T) {
	ctx := context.Background()

	t.Run("nil_llm_returns_error", func(t *testing.T) {
		eng := &Engine{llm: nil}
		_, err := eng.resolveChatModel(ctx, "", "")
		if !errors.Is(err, ErrNoChatModelConfigured) {
			t.Errorf("expected ErrNoChatModelConfigured, got %v", err)
		}
	})

	t.Run("list_error_propagates", func(t *testing.T) {
		listErr := errors.New("db connection failed")
		eng := &Engine{llm: &stubLlmLister{listErr: listErr}}

		_, err := eng.resolveChatModel(ctx, "", "")
		if !errors.Is(err, listErr) {
			t.Errorf("expected list error, got %v", err)
		}
	})

	t.Run("no_models_with_credentials", func(t *testing.T) {
		rows := []biz.ProviderModel{
			{
				Provider:   "openai",
				Model:      "gpt-4",
				Enabled:    true,
				DeletedAt:  "2025-01-01",
				ConfigJSON: `{"api_base_url":"https://api.openai.com","api_key_set":true}`,
			},
			{
				Provider:   "deepseek",
				Model:      "deepseek-chat",
				Enabled:    true,
				ConfigJSON: `{"api_base_url":"","api_key":""}`,
			},
		}
		eng := &Engine{llm: &stubLlmLister{listRows: rows}}

		_, err := eng.resolveChatModel(ctx, "", "")
		if !errors.Is(err, ErrNoChatModelConfigured) {
			t.Errorf("expected ErrNoChatModelConfigured, got %v", err)
		}
	})

	t.Run("skips_disabled_models", func(t *testing.T) {
		rows := []biz.ProviderModel{
			{
				Provider:   "openai",
				Model:      "gpt-4",
				Enabled:    false,
				ConfigJSON: `{"provider_type":"openai","api_base_url":"https://api.openai.com","api_key_set":true}`,
			},
		}
		eng := &Engine{llm: &stubLlmLister{listRows: rows}}

		_, err := eng.resolveChatModel(ctx, "", "")
		if !errors.Is(err, ErrNoChatModelConfigured) {
			t.Errorf("expected ErrNoChatModelConfigured for disabled model, got %v", err)
		}
	})

	t.Run("explicit_provider_model_no_match", func(t *testing.T) {
		rows := []biz.ProviderModel{
			{
				Provider:   "openai",
				Model:      "gpt-4",
				Enabled:    true,
				ConfigJSON: `{"provider_type":"openai","api_base_url":"https://api.openai.com","api_key_set":true}`,
			},
		}
		eng := &Engine{llm: &stubLlmLister{listRows: rows}}

		_, err := eng.resolveChatModel(ctx, "anthropic", "claude-3")
		if !errors.Is(err, ErrNoChatModelConfigured) {
			t.Errorf("expected ErrNoChatModelConfigured for non-matching provider, got %v", err)
		}
	})
}

func TestResolveChatModel_WithCredentials(t *testing.T) {
	ctx := context.Background()

	t.Run("explicit_provider_and_model", func(t *testing.T) {
		rows := []biz.ProviderModel{
			{
				Provider:   "openai",
				Model:      "gpt-4",
				Enabled:    true,
				ConfigJSON: `{"provider_type":"openai","api_base_url":"https://api.openai.com","api_key_set":true}`,
			},
			{
				Provider:   "deepseek",
				Model:      "deepseek-chat",
				Enabled:    true,
				ConfigJSON: `{"provider_type":"deepseek","api_base_url":"https://api.deepseek.com","api_key_set":true}`,
			},
		}
		resolved := map[string]biz.ProviderModel{
			stubKey("deepseek", "deepseek-chat"): {
				Provider:   "deepseek",
				Model:      "deepseek-chat",
				ConfigJSON: `{"provider_type":"deepseek","api_base_url":"https://api.deepseek.com","api_key":"ds-xyz"}`,
			},
		}
		eng := &Engine{llm: &stubLlmLister{listRows: rows, resolved: resolved}}

		cfg, err := eng.resolveChatModel(ctx, "deepseek", "deepseek-chat")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.ProviderType != "deepseek" {
			t.Errorf("ProviderType = %q, want %q", cfg.ProviderType, "deepseek")
		}
		if cfg.ModelAPIID != "deepseek-chat" {
			t.Errorf("ModelAPIID = %q, want %q", cfg.ModelAPIID, "deepseek-chat")
		}
		if cfg.APIBaseURL != "https://api.deepseek.com" {
			t.Errorf("APIBaseURL = %q, want %q", cfg.APIBaseURL, "https://api.deepseek.com")
		}
		if cfg.APIKey != "ds-xyz" {
			t.Errorf("APIKey = %q, want %q", cfg.APIKey, "ds-xyz")
		}
	})

	t.Run("empty_provider_picks_first_enabled_with_credentials", func(t *testing.T) {
		rows := []biz.ProviderModel{
			{
				Provider:   "disabled-provider",
				Model:      "model-a",
				Enabled:    false,
				ConfigJSON: `{"api_base_url":"https://api.example.com","api_key_set":true}`,
			},
			{
				Provider:   "openai",
				Model:      "gpt-4",
				Enabled:    true,
				ConfigJSON: `{"provider_type":"openai","api_base_url":"https://api.openai.com","api_key_set":true}`,
			},
		}
		resolved := map[string]biz.ProviderModel{
			stubKey("openai", "gpt-4"): {
				Provider:   "openai",
				Model:      "gpt-4",
				ConfigJSON: `{"provider_type":"openai","api_base_url":"https://api.openai.com","api_key":"sk-abc"}`,
			},
		}
		eng := &Engine{llm: &stubLlmLister{listRows: rows, resolved: resolved}}

		cfg, err := eng.resolveChatModel(ctx, "", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.ModelAPIID != "gpt-4" {
			t.Errorf("ModelAPIID = %q, want %q", cfg.ModelAPIID, "gpt-4")
		}
		if cfg.APIKey != "sk-abc" {
			t.Errorf("APIKey = %q, want %q", cfg.APIKey, "sk-abc")
		}
	})

	t.Run("skips_deleted_picks_next", func(t *testing.T) {
		rows := []biz.ProviderModel{
			{
				Provider:   "openai",
				Model:      "gpt-4",
				Enabled:    true,
				DeletedAt:  "2025-01-01",
				ConfigJSON: `{"provider_type":"openai","api_base_url":"https://api.openai.com","api_key_set":true}`,
			},
			{
				Provider:   "deepseek",
				Model:      "deepseek-chat",
				Enabled:    true,
				ConfigJSON: `{"provider_type":"deepseek","api_base_url":"https://api.deepseek.com","api_key_set":true}`,
			},
		}
		resolved := map[string]biz.ProviderModel{
			stubKey("deepseek", "deepseek-chat"): {
				Provider:   "deepseek",
				Model:      "deepseek-chat",
				ConfigJSON: `{"provider_type":"deepseek","api_base_url":"https://api.deepseek.com","api_key":"ds-xyz"}`,
			},
		}
		eng := &Engine{llm: &stubLlmLister{listRows: rows, resolved: resolved}}

		cfg, err := eng.resolveChatModel(ctx, "", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.ProviderType != "deepseek" {
			t.Errorf("ProviderType = %q, want %q", cfg.ProviderType, "deepseek")
		}
		if cfg.ModelAPIID != "deepseek-chat" {
			t.Errorf("ModelAPIID = %q, want %q", cfg.ModelAPIID, "deepseek-chat")
		}
	})
}

func TestDecodeModelJSON(t *testing.T) {
	t.Run("valid_json", func(t *testing.T) {
		var out struct {
			Name string `json:"name"`
		}
		err := decodeModelJSON(`{"name":"test"}`, &out)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out.Name != "test" {
			t.Errorf("Name = %q, want %q", out.Name, "test")
		}
	})

	t.Run("invalid_json", func(t *testing.T) {
		var out struct{}
		err := decodeModelJSON(`not json`, &out)
		if err == nil {
			t.Error("expected error for invalid JSON")
		}
	})

	t.Run("empty_string", func(t *testing.T) {
		var out struct{}
		err := decodeModelJSON("", &out)
		if err == nil {
			t.Error("expected error for empty string")
		}
	})

	t.Run("markdown_code_block_json", func(t *testing.T) {
		var out struct {
			Name string `json:"name"`
		}
		raw := "```json\n{\"name\":\"from_markdown\"}\n```"
		err := decodeModelJSON(raw, &out)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out.Name != "from_markdown" {
			t.Errorf("Name = %q, want %q", out.Name, "from_markdown")
		}
	})

	t.Run("markdown_code_block_no_lang", func(t *testing.T) {
		var out struct {
			Name string `json:"name"`
		}
		raw := "```\n{\"name\":\"no_lang\"}\n```"
		err := decodeModelJSON(raw, &out)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out.Name != "no_lang" {
			t.Errorf("Name = %q, want %q", out.Name, "no_lang")
		}
	})

	t.Run("json_embedded_in_text", func(t *testing.T) {
		var out struct {
			Name string `json:"name"`
		}
		raw := "Here is the result: {\"name\":\"embedded\"} and some trailing text"
		err := decodeModelJSON(raw, &out)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out.Name != "embedded" {
			t.Errorf("Name = %q, want %q", out.Name, "embedded")
		}
	})
}

func TestCompleteOpenAICompatible(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("expected Bearer test-key, got %s", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected application/json content-type")
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}

		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": "Hello from model"}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	cfg := chatModelCfg{
		ProviderType: "openai",
		APIBaseURL:   srv.URL,
		APIKey:       "test-key",
		ModelAPIID:   "gpt-4",
	}

	result, err := completeOpenAICompatible(context.Background(), srv.Client(), cfg, "test prompt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Hello from model" {
		t.Errorf("result = %q, want %q", result, "Hello from model")
	}
}

func TestCompleteOpenAICompatible_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte("rate limited"))
	}))
	defer srv.Close()

	cfg := chatModelCfg{
		ProviderType: "openai",
		APIBaseURL:   srv.URL,
		APIKey:       "test-key",
		ModelAPIID:   "gpt-4",
	}

	_, err := completeOpenAICompatible(context.Background(), srv.Client(), cfg, "test")
	if err == nil {
		t.Fatal("expected error for non-2xx status")
	}
	if !errors.Is(err, ErrChatCompletionFailed) {
		t.Errorf("expected ErrChatCompletionFailed, got %v", err)
	}
}

func TestCompleteOpenAICompatible_EmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"choices": []map[string]any{},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	cfg := chatModelCfg{
		ProviderType: "openai",
		APIBaseURL:   srv.URL,
		APIKey:       "test-key",
		ModelAPIID:   "gpt-4",
	}

	_, err := completeOpenAICompatible(context.Background(), srv.Client(), cfg, "test")
	if !errors.Is(err, ErrEmptyChatResponse) {
		t.Errorf("expected ErrEmptyChatResponse, got %v", err)
	}
}

func TestCompleteAnthropic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("x-api-key") != "anthropic-key" {
			t.Errorf("expected x-api-key anthropic-key, got %s", r.Header.Get("x-api-key"))
		}
		if r.Header.Get("anthropic-version") != "2023-06-01" {
			t.Errorf("expected anthropic-version 2023-06-01, got %s", r.Header.Get("anthropic-version"))
		}

		resp := map[string]any{
			"content": []map[string]any{
				{"text": "Anthropic response"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	cfg := chatModelCfg{
		ProviderType: "anthropic",
		APIBaseURL:   srv.URL,
		APIKey:       "anthropic-key",
		ModelAPIID:   "claude-3",
	}

	result, err := completeAnthropic(context.Background(), srv.Client(), cfg, "test prompt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Anthropic response" {
		t.Errorf("result = %q, want %q", result, "Anthropic response")
	}
}

func TestCompleteAnthropic_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("bad request"))
	}))
	defer srv.Close()

	cfg := chatModelCfg{
		ProviderType: "anthropic",
		APIBaseURL:   srv.URL,
		APIKey:       "anthropic-key",
		ModelAPIID:   "claude-3",
	}

	_, err := completeAnthropic(context.Background(), srv.Client(), cfg, "test")
	if !errors.Is(err, ErrAnthropicFailed) {
		t.Errorf("expected ErrAnthropicFailed, got %v", err)
	}
}

func TestCompleteAnthropic_EmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"content": []map[string]any{},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	cfg := chatModelCfg{
		ProviderType: "anthropic",
		APIBaseURL:   srv.URL,
		APIKey:       "anthropic-key",
		ModelAPIID:   "claude-3",
	}

	_, err := completeAnthropic(context.Background(), srv.Client(), cfg, "test")
	if !errors.Is(err, ErrEmptyAnthropicResponse) {
		t.Errorf("expected ErrEmptyAnthropicResponse, got %v", err)
	}
}

func TestNormalizeConflictRisk(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want string
	}{
		{name: "string_low", in: "low", want: "low"},
		{name: "string_medium", in: "medium", want: "medium"},
		{name: "string_high", in: "high", want: "high"},
		{name: "string_case_insensitive", in: "High", want: "high"},
		{name: "string_padded", in: "  medium  ", want: "medium"},
		{name: "string_unknown", in: "severe", want: ""},
		{name: "number_high", in: 0.85, want: "high"},
		{name: "number_boundary_high", in: 0.66, want: "high"},
		{name: "number_medium", in: 0.5, want: "medium"},
		{name: "number_boundary_medium", in: 0.33, want: "medium"},
		{name: "number_low", in: 0.1, want: "low"},
		{name: "number_zero", in: 0.0, want: "low"},
		{name: "nil", in: nil, want: ""},
		{name: "bool_ignored", in: true, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeConflictRisk(tt.in); got != tt.want {
				t.Errorf("normalizeConflictRisk(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestParseSimilarityResult guards the regression where LLMs violate the
// declared response schema (numeric conflict_risk, evidence as a bare string,
// quoted numbers): strict typed unmarshal used to fail the whole pair
// comparison, silently producing no conflict groups.
func TestParseSimilarityResult(t *testing.T) {
	t.Run("well_formed", func(t *testing.T) {
		metrics, reason, evidence, err := parseSimilarityResult(`{"similarity_score":0.9,"name_similarity":0.8,"description_similarity":0.9,"body_similarity":0.95,"trigger_similarity":0.7,"tool_similarity":0.6,"conflict_risk":"high","recommendation":"block_duplicate","confidence":0.9,"reason":"near duplicate","evidence":["same purpose","same steps"]}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if metrics.SimilarityScore != 0.9 || metrics.ConflictRisk != "high" || metrics.Recommendation != "block_duplicate" {
			t.Errorf("metrics = %+v", metrics)
		}
		if reason != "near duplicate" {
			t.Errorf("reason = %q", reason)
		}
		if len(evidence) != 2 {
			t.Errorf("evidence = %v", evidence)
		}
	})

	t.Run("numeric_conflict_risk", func(t *testing.T) {
		metrics, _, _, err := parseSimilarityResult(`{"similarity_score":0.9,"conflict_risk":0.85,"recommendation":"block_duplicate"}`)
		if err != nil {
			t.Fatalf("should tolerate numeric conflict_risk: %v", err)
		}
		if metrics.ConflictRisk != "high" {
			t.Errorf("ConflictRisk = %q, want high", metrics.ConflictRisk)
		}
	})

	t.Run("evidence_as_bare_string", func(t *testing.T) {
		_, _, evidence, err := parseSimilarityResult(`{"similarity_score":0.9,"conflict_risk":"high","evidence":"both review code changes"}`)
		if err != nil {
			t.Fatalf("should tolerate bare-string evidence: %v", err)
		}
		if len(evidence) != 1 || evidence[0] != "both review code changes" {
			t.Errorf("evidence = %v", evidence)
		}
	})

	t.Run("quoted_numeric_scores", func(t *testing.T) {
		metrics, _, _, err := parseSimilarityResult(`{"similarity_score":"0.9","confidence":"0.8","conflict_risk":"low"}`)
		if err != nil {
			t.Fatalf("should tolerate quoted numbers: %v", err)
		}
		if metrics.SimilarityScore != 0.9 || metrics.Confidence != 0.8 {
			t.Errorf("metrics = %+v", metrics)
		}
	})

	t.Run("defaults_applied", func(t *testing.T) {
		metrics, _, _, err := parseSimilarityResult(`{"similarity_score":0.5}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if metrics.Recommendation != "suggest_refine" {
			t.Errorf("Recommendation = %q, want suggest_refine", metrics.Recommendation)
		}
		if metrics.ConflictRisk != "medium" {
			t.Errorf("ConflictRisk = %q, want medium", metrics.ConflictRisk)
		}
	})

	t.Run("invalid_json", func(t *testing.T) {
		if _, _, _, err := parseSimilarityResult(`not json`); err == nil {
			t.Error("expected error for invalid JSON")
		}
	})
}

func TestCompleteChat_RoutesToAnthropic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"content": []map[string]any{
				{"text": "from anthropic route"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	cfg := chatModelCfg{
		ProviderType: "anthropic",
		APIBaseURL:   srv.URL,
		APIKey:       "anthropic-key",
		ModelAPIID:   "claude-3",
	}

	result, err := completeChat(context.Background(), cfg, "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "from anthropic route" {
		t.Errorf("result = %q, want %q", result, "from anthropic route")
	}
}

func TestCompleteChat_RoutesToOpenAI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": "from openai route"}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	cfg := chatModelCfg{
		ProviderType: "openai",
		APIBaseURL:   srv.URL,
		APIKey:       "test-key",
		ModelAPIID:   "gpt-4",
	}

	result, err := completeChat(context.Background(), cfg, "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "from openai route" {
		t.Errorf("result = %q, want %q", result, "from openai route")
	}
}

// TestCompleteOpenAICompatible_ThinkingDisabled guards FN-1: DeepSeek 推理模型
// 默认开启思考段，相似度短任务响应 30s+ 导致导入超时；请求体必须显式关闭。
func TestCompleteOpenAICompatible_ThinkingDisabled(t *testing.T) {
	tests := []struct {
		name         string
		baseURL      func(srvURL string) string
		wantThinking bool
	}{
		{
			name:         "deepseek_injects_thinking_disabled",
			baseURL:      func(string) string { return "https://api.deepseek.com" },
			wantThinking: true,
		},
		{
			name:         "deepseek_case_insensitive",
			baseURL:      func(string) string { return "https://API.DEEPSEEK.COM" },
			wantThinking: true,
		},
		{
			name:         "openai_no_thinking_field",
			baseURL:      func(srvURL string) string { return srvURL },
			wantThinking: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotBody map[string]any
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
					t.Errorf("failed to decode request body: %v", err)
				}
				resp := map[string]any{
					"choices": []map[string]any{
						{"message": map[string]string{"content": "ok"}},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
			}))
			defer srv.Close()

			base := tt.baseURL(srv.URL)
			cfg := chatModelCfg{ProviderType: "openai", APIBaseURL: base, APIKey: "k", ModelAPIID: "m"}

			if tt.wantThinking {
				// DeepSeek 真实域名不可达，仅验证请求体构造：用传输层把
				// api.deepseek.com 重定向到 httptest 服务器。
				client := &http.Client{
					Transport: &rewriteHostTransport{target: srv.URL},
				}
				if _, err := completeOpenAICompatible(context.Background(), client, cfg, "p"); err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			} else {
				if _, err := completeOpenAICompatible(context.Background(), srv.Client(), cfg, "p"); err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}

			thinking, has := gotBody["thinking"]
			if has != tt.wantThinking {
				t.Fatalf("thinking field present = %v, want %v (body=%v)", has, tt.wantThinking, gotBody)
			}
			if tt.wantThinking {
				m, ok := thinking.(map[string]any)
				if !ok || m["type"] != "disabled" {
					t.Errorf("thinking = %v, want {\"type\":\"disabled\"}", thinking)
				}
			}
		})
	}
}

// rewriteHostTransport rewrites any request URL to the test server while
// preserving the original path — lets tests exercise provider-specific request
// construction (e.g. api.deepseek.com) without external network access.
type rewriteHostTransport struct {
	target string
}

func (rt *rewriteHostTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	target := strings.TrimRight(rt.target, "/") + clone.URL.Path
	if clone.URL.RawQuery != "" {
		target += "?" + clone.URL.RawQuery
	}
	u, err := http.NewRequestWithContext(req.Context(), req.Method, target, req.Body)
	if err != nil {
		return nil, err
	}
	u.Header = clone.Header
	return http.DefaultTransport.RoundTrip(u)
}

// stubEngineWithSimilarityServer builds an Engine whose resolved chat model
// points at the given test server.
func stubEngineWithSimilarityServer(srvURL string) *Engine {
	rows := []biz.ProviderModel{{
		Provider:   "test",
		Model:      "m",
		Enabled:    true,
		ConfigJSON: `{"api_base_url":"` + srvURL + `","api_key":"k"}`,
	}}
	resolved := map[string]biz.ProviderModel{
		stubKey("test", "m"): {
			Provider:   "test",
			Model:      "m",
			ConfigJSON: `{"provider_type":"openai","api_base_url":"` + srvURL + `","api_key":"k"}`,
		},
	}
	return &Engine{
		llm: &stubLlmLister{listRows: rows, resolved: resolved},
		lg:  loggateway.NewNoop(),
	}
}

func similarityJobState(candidateID string) *jobState {
	candidate := biz.SkillImportCandidate{
		CandidateID:      candidateID,
		Name:             "cand",
		ValidationStatus: "pass",
	}
	return &jobState{
		public: biz.SkillImportJob{Candidates: []biz.SkillImportCandidate{candidate}},
		candidates: map[string]candidateState{
			candidateID: {public: candidate, body: "candidate body"},
		},
		createdAt: time.Now(),
	}
}

// TestInspectSimilarity_Parallel guards FN-1: 相似度检查必须并发执行（串行
// N×30s 会击穿前端超时），且高分对仍聚合成 conflict group + candidate warn。
func TestInspectSimilarity_Parallel(t *testing.T) {
	var inflight atomic.Int32
	var maxInflight atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cur := inflight.Add(1)
		defer inflight.Add(-1)
		for {
			m := maxInflight.Load()
			if cur <= m || maxInflight.CompareAndSwap(m, cur) {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": `{"similarity_score":0.9,"conflict_risk":"high","recommendation":"suggest_refine","reason":"dup","evidence":["e"]}`}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	eng := stubEngineWithSimilarityServer(srv.URL)
	job := similarityJobState("cand-1")
	existing := make([]biz.SkillSimilaritySource, 6)
	for i := range existing {
		existing[i] = biz.SkillSimilaritySource{ID: "s" + string(rune('a'+i)), Name: "existing"}
	}

	start := time.Now()
	if err := eng.inspectSimilarity(context.Background(), job, existing); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	elapsed := time.Since(start)

	if got := len(job.public.ConflictGroups); got != len(existing) {
		t.Errorf("ConflictGroups = %d, want %d", got, len(existing))
	}
	if job.public.Candidates[0].ValidationStatus != "warn" {
		t.Errorf("candidate ValidationStatus = %q, want warn", job.public.Candidates[0].ValidationStatus)
	}
	if got := maxInflight.Load(); got < 2 {
		t.Errorf("max concurrent LLM calls = %d, want >= 2 (parallelism evidence)", got)
	}
	// 串行下 6×50ms=300ms；并发上限 5 时应显著更低。留足抖动余量。
	if elapsed > 250*time.Millisecond {
		t.Errorf("elapsed = %v, want < 250ms (serial would be ~300ms)", elapsed)
	}
}

// TestInspectSimilarity_CallTimeout guards FN-1: 单次 LLM 调用受独立超时熔断，
// 慢响应不得拖垮整个导入流程（超时的对仅记 warn，不产生 conflict group）。
func TestInspectSimilarity_CallTimeout(t *testing.T) {
	old := similarityCallTimeout
	similarityCallTimeout = 50 * time.Millisecond
	defer func() { similarityCallTimeout = old }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": `{"similarity_score":0.9}`}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	eng := stubEngineWithSimilarityServer(srv.URL)
	job := similarityJobState("cand-1")
	existing := []biz.SkillSimilaritySource{{ID: "s1", Name: "slow"}}

	start := time.Now()
	if err := eng.inspectSimilarity(context.Background(), job, existing); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	elapsed := time.Since(start)

	if got := len(job.public.ConflictGroups); got != 0 {
		t.Errorf("ConflictGroups = %d, want 0 (timed-out pair must be skipped)", got)
	}
	if elapsed > 400*time.Millisecond {
		t.Errorf("elapsed = %v, want < 400ms (server sleeps 500ms; call must be cut at 50ms)", elapsed)
	}
}
