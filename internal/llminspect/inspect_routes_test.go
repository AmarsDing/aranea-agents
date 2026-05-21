package llminspect

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestInspectOllamaModel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"models": []map[string]any{
				{"name": "llama3:latest", "model": "llama3:latest", "details": map[string]string{"parameter_size": "8B"}},
			},
		})
	}))
	defer srv.Close()

	out, err := inspectOllamaModel(Input{
		ProviderCode: "local",
		ProviderType: "ollama",
		ModelAPIID:   "llama3:latest",
		APIBaseURL:   srv.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.OK || out.Source != "ollama" {
		t.Fatalf("got %+v", out)
	}
}

func TestInspectBedrockModel(t *testing.T) {
	out, err := inspectBedrockModel(Input{
		ProviderCode: "aws",
		ProviderType: "bedrock",
		ModelAPIID:   "anthropic.claude-3-sonnet",
		AWSRegion:    "us-east-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.OK || out.Source != "bedrock" {
		t.Fatalf("got %+v", out)
	}
}

func TestInspectGeminiModel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"models": []map[string]any{
				{
					"name":             "models/gemini-1.5-flash",
					"displayName":      "Gemini 1.5 Flash",
					"inputTokenLimit":  1000000,
					"outputTokenLimit": 8192,
				},
			},
		})
	}))
	defer srv.Close()

	out, err := inspectGeminiModel(Input{
		ProviderCode: "google",
		ProviderType: "gemini",
		ModelAPIID:   "gemini-1.5-flash",
		APIBaseURL:   srv.URL + "/v1beta",
		APIKey:       "test-key",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.OK || out.Source != "gemini" {
		t.Fatalf("got %+v", out)
	}
}
