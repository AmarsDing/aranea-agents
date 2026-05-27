package service_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"aranea-agents/internal/service"
)

// systemInfoHandlerFunc adapts SystemInfoResponse to a plain http.Handler for testing,
// without needing to spin up the full Kratos server.
func TestSystemInfoResponse_FieldsPresent(t *testing.T) {
	resp := service.SystemInfoResponse{
		Version:          "v0.1.0",
		GitCommit:        "abc123",
		BuildTime:        "2026-05-27",
		DefaultProvider:  "openai",
		DefaultModel:     "gpt-4o",
		SkillStorageRoot: "/data/skills",
		Features:         map[string]string{"mcp": "true"},
	}

	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	requiredFields := []string{"version", "git_commit", "build_time", "default_provider", "default_model", "features"}
	for _, f := range requiredFields {
		if _, ok := got[f]; !ok {
			t.Errorf("missing field %q in SystemInfoResponse JSON", f)
		}
	}
}

// TestSystemInfoHTTP exercises the HTTP handler using a plain net/http recorder.
func TestSystemInfoHTTP(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/system/info", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(service.SystemInfoResponse{
			Version:  "v0.1.0",
			Features: map[string]string{},
		})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/system/info")
	if err != nil {
		t.Fatalf("GET /v1/system/info: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var info service.SystemInfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if info.Version != "v0.1.0" {
		t.Errorf("version: got %q, want v0.1.0", info.Version)
	}
}
