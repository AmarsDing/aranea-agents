package webresearch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestResearchTool_Call_mockTavily(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"query": "test query",
			"answer": "summary answer",
			"results": []map[string]any{
				{"title": "Example", "url": "https://example.com", "content": "snippet text", "score": 0.9},
			},
			"response_time": 0.12,
		})
	}))
	defer srv.Close()

	cfg := Config{
		Provider:        ProviderTavily,
		APIKey:          "test-key",
		MaxResults:      3,
		FetchTop:        0,
		Timeout:         30 * time.Second,
		TavilySearchURL: srv.URL,
	}
	tool, err := NewTool(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	out, err := tool.Call(context.Background(), []byte(`{"query":"test query"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp, ok := out.(researchOutput)
	if !ok {
		t.Fatalf("type %T", out)
	}
	if len(resp.Sources) != 1 || resp.Sources[0].Title != "Example" {
		t.Fatalf("sources=%+v", resp.Sources)
	}
	if resp.Answer != "summary answer" {
		t.Fatalf("answer=%q", resp.Answer)
	}
}

func TestTruncateUTF8_multibyte(t *testing.T) {
	s := "你好世界"
	got := truncateUTF8(s, 7)
	if got == "" || len(got) > 7+20 {
		t.Fatalf("truncate=%q len=%d", got, len(got))
	}
}
