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
	"aranea-agents/pkg/loggateway"
)

func TestSerpAPISearch(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		maxResults int
		handler    http.HandlerFunc
		wantErr    bool
		errSubstr  string
		wantAnswer string
		wantHits   int
	}{
		{
			name:       "happy path with organic results and answer",
			query:      "golang testing",
			maxResults: 5,
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}
				if r.URL.Query().Get("q") != "golang testing" {
					http.Error(w, "bad query", http.StatusBadRequest)
					return
				}
				json.NewEncoder(w).Encode(map[string]any{
					"organic_results": []map[string]any{
						{"title": "Go Testing", "link": "https://go.dev/testing", "snippet": "Testing in Go"},
						{"title": "Go Docs", "link": "https://go.dev/doc", "snippet": "Go documentation"},
					},
					"answer_box": map[string]any{"answer": "Use testing package"},
				})
			},
			wantAnswer: "Use testing package",
			wantHits:   2,
		},
		{
			name:       "empty query",
			query:      "",
			maxResults: 5,
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			},
			wantErr:   true,
			errSubstr: "query is required",
		},
		{
			name:       "whitespace query",
			query:      "   ",
			maxResults: 5,
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			},
			wantErr:   true,
			errSubstr: "query is required",
		},
		{
			name:       "non-200 status",
			query:      "test",
			maxResults: 5,
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(`{"error":"server error"}`))
			},
			wantErr:   true,
			errSubstr: "serpapi status 500",
		},
		{
			name:       "invalid json response",
			query:      "test",
			maxResults: 5,
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{invalid json}`))
			},
			wantErr:   true,
			errSubstr: "serpapi decode",
		},
		{
			name:       "answer box with result field",
			query:      "2+2",
			maxResults: 5,
			handler: func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode(map[string]any{
					"organic_results": []map[string]any{},
					"answer_box":      map[string]any{"result": "4"},
				})
			},
			wantAnswer: "4",
			wantHits:   0,
		},
		{
			name:       "answer box with snippet field only",
			query:      "test",
			maxResults: 5,
			handler: func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode(map[string]any{
					"organic_results": []map[string]any{},
					"answer_box":      map[string]any{"snippet": "snippet answer"},
				})
			},
			wantAnswer: "snippet answer",
			wantHits:   0,
		},
		{
			name:       "empty answer box",
			query:      "test",
			maxResults: 5,
			handler: func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode(map[string]any{
					"organic_results": []map[string]any{
						{"title": "Result", "link": "https://example.com", "snippet": "snippet"},
					},
					"answer_box": map[string]any{},
				})
			},
			wantAnswer: "",
			wantHits:   1,
		},
		{
			name:       "max results limit",
			query:      "test",
			maxResults: 2,
			handler: func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode(map[string]any{
					"organic_results": []map[string]any{
						{"title": "A", "link": "https://a.com", "snippet": "a"},
						{"title": "B", "link": "https://b.com", "snippet": "b"},
						{"title": "C", "link": "https://c.com", "snippet": "c"},
					},
				})
			},
			wantHits: 2,
		},
		{
			name:       "no organic results",
			query:      "obscure query",
			maxResults: 5,
			handler: func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode(map[string]any{
					"organic_results": []map[string]any{},
				})
			},
			wantHits:   0,
			wantAnswer: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(tt.handler)
			defer srv.Close()

			cfg := webresearch.Config{
				Provider:       "serpapi",
				APIKey:         "test-key",
				MaxResults:     tt.maxResults,
				Timeout:        10 * time.Second,
				SerpAPIBaseURL: srv.URL,
			}
			p, err := webresearch.NewSerpAPIProvider(cfg, loggateway.NewNoop())
			if err != nil {
				t.Fatal(err)
			}

			resp, err := webresearch.ProviderSearch(p, context.Background(), tt.query)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				if tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Fatalf("error = %q, want substring %q", err.Error(), tt.errSubstr)
				}
				return
			}

			if resp.Answer != tt.wantAnswer {
				t.Fatalf("Answer = %q, want %q", resp.Answer, tt.wantAnswer)
			}
			if len(resp.Results) != tt.wantHits {
				t.Fatalf("Results count = %d, want %d", len(resp.Results), tt.wantHits)
			}
			if resp.Provider != "serpapi" {
				t.Fatalf("Provider = %q, want serpapi", resp.Provider)
			}
			if resp.Query != strings.TrimSpace(tt.query) {
				t.Fatalf("Query = %q, want %q", resp.Query, strings.TrimSpace(tt.query))
			}
		})
	}
}

func TestSerpAPISearch_requestFailed(t *testing.T) {
	cfg := webresearch.Config{
		Provider:       "serpapi",
		APIKey:         "test-key",
		MaxResults:     5,
		Timeout:        100 * time.Millisecond,
		SerpAPIBaseURL: "http://127.0.0.1:0",
	}
	p, err := webresearch.NewSerpAPIProvider(cfg, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}

	_, err = webresearch.ProviderSearch(p, context.Background(), "test")
	if err == nil {
		t.Fatal("expected error for unreachable server")
	}
	if !strings.Contains(err.Error(), "serpapi request failed") {
		t.Fatalf("error = %q, want substring 'serpapi request failed'", err.Error())
	}
}

func TestSerpAPISearch_cancelledContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"organic_results": []map[string]any{},
		})
	}))
	defer srv.Close()

	cfg := webresearch.Config{
		Provider:       "serpapi",
		APIKey:         "test-key",
		MaxResults:     5,
		Timeout:        10 * time.Second,
		SerpAPIBaseURL: srv.URL,
	}
	p, err := webresearch.NewSerpAPIProvider(cfg, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = webresearch.ProviderSearch(p, ctx, "test")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}
