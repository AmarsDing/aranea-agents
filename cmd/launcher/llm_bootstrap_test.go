package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestMergeAPIKeyIntoConfig(t *testing.T) {
	got, err := mergeAPIKeyIntoConfig(`{"api_base_url":"https://api.deepseek.com"}`, "sk-test")
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(got), &m); err != nil {
		t.Fatalf("merged config not JSON: %v", err)
	}
	if m["api_key"] != "sk-test" || m["api_base_url"] != "https://api.deepseek.com" {
		t.Fatalf("unexpected merged config: %s", got)
	}
	// empty existing config
	got, err = mergeAPIKeyIntoConfig("", "sk-x")
	if err != nil || !strings.Contains(got, "sk-x") {
		t.Fatalf("empty config merge failed: %q %v", got, err)
	}
}

func TestPickProviderModel(t *testing.T) {
	items := []providerModelItem{
		{ID: "pm-1", Provider: "openai", Model: "gpt-4.1-mini"},
		{ID: "pm-2", Provider: "deepseek", Model: "deepseek-v4-flash"},
		{ID: "pm-3", Provider: "deepseek", Model: "deepseek-v4-pro"},
	}
	got := pickProviderModel(items, "deepseek", "deepseek-v4-flash")
	if got == nil || got.ID != "pm-2" {
		t.Fatalf("expected pm-2, got %+v", got)
	}
	if pickProviderModel(items, "deepseek", "nonexistent") != nil {
		t.Fatal("no fallback to arbitrary model")
	}
	if pickProviderModel(items, "absent", "deepseek-v4-flash") != nil {
		t.Fatal("unknown provider must not match")
	}
}

// TestApplyLLMBootstrap exercises the full login → find → PATCH flow against
// a fake backend, and verifies the bootstrap file is consumed on success.
func TestApplyLLMBootstrap(t *testing.T) {
	var patchedBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/admins/login":
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["username"] != "admin" || body["password"] != "changeme" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{"id":1,"name":"admin","token":"tok-123"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/llm-provider-models":
			if r.Header.Get("Authorization") != "Bearer tok-123" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{"items":[{"id":"pm-2","key":"deepseek:deepseek-v4-flash","name":"DeepSeek V4 Flash","provider":"deepseek","model":"deepseek-v4-flash","configJson":"{\"api_base_url\":\"https://api.deepseek.com\"}","enabled":true}],"total":1}`))
		case r.Method == http.MethodPatch && r.URL.Path == "/v1/llm-provider-models/pm-2":
			b := make([]byte, r.ContentLength)
			_, _ = r.Body.Read(b)
			patchedBody = string(b)
			_, _ = w.Write([]byte(`{"id":"pm-2"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	root := t.TempDir()
	if err := os.MkdirAll(root+string(os.PathSeparator)+"configs", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := saveLLMBootstrap(root, llmBootstrap{Provider: "deepseek", Model: "deepseek-v4-flash", APIKey: "sk-test"}); err != nil {
		t.Fatalf("save bootstrap: %v", err)
	}
	if err := applyLLMBootstrap(root, srv.URL, func(string, ...any) {}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !strings.Contains(patchedBody, "sk-test") {
		t.Fatalf("PATCH body missing api key: %s", patchedBody)
	}
	if !strings.Contains(patchedBody, "providerModel") {
		t.Fatalf("PATCH body must wrap providerModel: %s", patchedBody)
	}
	if loadLLMBootstrap(root) != nil {
		t.Fatal("bootstrap file must be removed after successful apply")
	}
}

func TestApplyLLMBootstrapKeepsFileOnFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	root := t.TempDir()
	_ = os.MkdirAll(root+string(os.PathSeparator)+"configs", 0o755)
	if err := saveLLMBootstrap(root, llmBootstrap{Provider: "deepseek", Model: "m", APIKey: "k"}); err != nil {
		t.Fatal(err)
	}
	if err := applyLLMBootstrap(root, srv.URL, func(string, ...any) {}); err == nil {
		t.Fatal("expected error on 500")
	}
	if loadLLMBootstrap(root) == nil {
		t.Fatal("bootstrap file must be kept for retry on failure")
	}
}
