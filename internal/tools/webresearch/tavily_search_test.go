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

func TestTavilySearch_errorPaths(t *testing.T) {
	tests := []struct {
		name      string
		query     string
		handler   http.HandlerFunc
		wantErr   bool
		errSubstr string
	}{
		{
			name:  "empty query",
			query: "",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			},
			wantErr:   true,
			errSubstr: "query is required",
		},
		{
			name:  "whitespace query",
			query: "   ",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			},
			wantErr:   true,
			errSubstr: "query is required",
		},
		{
			name:  "non-200 status",
			query: "test",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusForbidden)
				w.Write([]byte(`{"error":"forbidden"}`))
			},
			wantErr:   true,
			errSubstr: "tavily status 403",
		},
		{
			name:  "invalid json response",
			query: "test",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`not json at all`))
			},
			wantErr:   true,
			errSubstr: "tavily decode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(tt.handler)
			defer srv.Close()

			cfg := webresearch.Config{
				Provider:        "tavily",
				APIKey:          "test-key",
				MaxResults:      5,
				Timeout:         10 * time.Second,
				TavilySearchURL: srv.URL,
			}
			p, err := webresearch.NewTavilyProvider(cfg, loggateway.NewNoop())
			if err != nil {
				t.Fatal(err)
			}

			_, err = webresearch.ProviderSearch(p, context.Background(), tt.query)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tt.errSubstr)
			}
		})
	}
}

func TestTavilySearch_responseTimeZero(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(time.Millisecond)
		json.NewEncoder(w).Encode(map[string]any{
			"query":         "test",
			"answer":        "answer text",
			"results":       []map[string]any{},
			"response_time": 0,
		})
	}))
	defer srv.Close()

	cfg := webresearch.Config{
		Provider:        "tavily",
		APIKey:          "test-key",
		MaxResults:      5,
		Timeout:         10 * time.Second,
		TavilySearchURL: srv.URL,
	}
	p, err := webresearch.NewTavilyProvider(cfg, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}

	resp, err := webresearch.ProviderSearch(p, context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}
	if resp.ResponseTime == 0 {
		t.Error("response_time should be > 0 when server sleeps")
	}
}
