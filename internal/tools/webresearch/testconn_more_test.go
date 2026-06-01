package webresearch_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/tools/webresearch"
)

func TestTestConnection(t *testing.T) {
	tests := []struct {
		name      string
		cfg       webresearch.Config
		handler   http.HandlerFunc
		wantErr   bool
		errSubstr string
		wantOK    bool
	}{
		{
			name: "not ready config",
			cfg: webresearch.Config{
				Provider: "tavily",
			},
			wantErr:   true,
			errSubstr: "api_key is required",
		},
		{
			name: "unsupported provider",
			cfg: webresearch.Config{
				Provider: "bing",
				APIKey:   "key",
			},
			wantErr:   true,
			errSubstr: "unsupported provider",
		},
		{
			name: "tavily success",
			cfg: webresearch.Config{
				Provider:        "tavily",
				APIKey:          "test-key",
				MaxResults:      1,
				Timeout:         10 * time.Second,
				TavilySearchURL: "PLACEHOLDER",
			},
			handler: func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode(map[string]any{
					"query":         "test",
					"results":       []map[string]any{{"title": "R", "url": "https://example.com", "content": "c", "score": 0.5}},
					"response_time": 0.1,
				})
			},
			wantOK: true,
		},
		{
			name: "serpapi success",
			cfg: webresearch.Config{
				Provider:       "serpapi",
				APIKey:         "test-key",
				MaxResults:     1,
				Timeout:        10 * time.Second,
				SerpAPIBaseURL: "PLACEHOLDER",
			},
			handler: func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode(map[string]any{
					"organic_results": []map[string]any{{"title": "R", "link": "https://example.com", "snippet": "s"}},
				})
			},
			wantOK: true,
		},
		{
			name: "search error non-200",
			cfg: webresearch.Config{
				Provider:        "tavily",
				APIKey:          "test-key",
				MaxResults:      1,
				Timeout:         10 * time.Second,
				TavilySearchURL: "PLACEHOLDER",
			},
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(`{"error":"fail"}`))
			},
			wantErr:   true,
			errSubstr: "tavily status 500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var srv *httptest.Server
			if tt.handler != nil {
				srv = httptest.NewServer(tt.handler)
				defer srv.Close()

				if tt.cfg.Provider == "tavily" && tt.cfg.TavilySearchURL == "PLACEHOLDER" {
					tt.cfg.TavilySearchURL = srv.URL
				}
				if tt.cfg.Provider == "serpapi" && tt.cfg.SerpAPIBaseURL == "PLACEHOLDER" {
					tt.cfg.SerpAPIBaseURL = srv.URL
				}
			}

			result, err := webresearch.TestConnection(context.Background(), tt.cfg, nil)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				if tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Fatalf("error = %q, want substring %q", err.Error(), tt.errSubstr)
				}
				if result.OK {
					t.Fatal("result.OK should be false on error")
				}
				return
			}
			if result.OK != tt.wantOK {
				t.Fatalf("OK = %v, want %v", result.OK, tt.wantOK)
			}
			if tt.wantOK {
				if result.ResultCount < 0 {
					t.Fatalf("ResultCount = %d, want >= 0", result.ResultCount)
				}
				if result.LatencyMS < 0 {
					t.Fatalf("LatencyMS = %d, want >= 0", result.LatencyMS)
				}
				if !strings.Contains(result.Message, "search OK") {
					t.Fatalf("Message = %q, want substring 'search OK'", result.Message)
				}
			}
		})
	}
}
