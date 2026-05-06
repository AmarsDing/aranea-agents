// Package web_search provides a small web search tool for OpenAI-compatible models (function calling).
// It uses DuckDuckGo's lightweight JSON endpoint (no API key); quality is limited vs hosted search APIs.
package web_search

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"aranea-agents/internal/tools/argmap"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

type searchArgs struct {
	Query string  `json:"query"`
	Limit float64 `json:"limit,omitempty"`
}

const desc = `Search the public web for a short factual answer and related links (DuckDuckGo instant answer + topics). Use for news, documentation, or facts the model cannot know from training. Pass query as the user's search terms.`

// New builds the ADK function tool named web_search (matches platform catalog tool_key).
func New() (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "web_search",
		Description: desc,
	}, func(tc tool.Context, in searchArgs) (map[string]any, error) {
		return Run(tc, map[string]any{"query": in.Query, "limit": in.Limit})
	})
}

// Run performs a lightweight DuckDuckGo lookup.
func Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	q := strings.TrimSpace(argmap.String(args, "query"))
	if q == "" {
		return nil, fmt.Errorf("query is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	u := "https://api.duckduckgo.com/?" + url.Values{
		"q": []string{q},
		"format":         []string{"json"},
		"no_html":        []string{"1"},
		"skip_disambig":  []string{"1"},
	}.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Aranea-Agent/1.0 (web_search)")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("search HTTP %d", resp.StatusCode)
	}

	var payload struct {
		Abstract       string `json:"Abstract"`
		AbstractURL    string `json:"AbstractURL"`
		Heading        string `json:"Heading"`
		Definition     string `json:"Definition"`
		DefinitionURL  string `json:"DefinitionURL"`
		RelatedTopics  []any  `json:"RelatedTopics"`
		Answer         string `json:"Answer"`
		Results        []any  `json:"Results"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode search response: %w", err)
	}

	topics := summarizeRelatedTopics(payload.RelatedTopics, 8)
	out := map[string]any{
		"query":            q,
		"abstract":         strings.TrimSpace(payload.Abstract),
		"abstract_url":     strings.TrimSpace(payload.AbstractURL),
		"heading":          strings.TrimSpace(payload.Heading),
		"definition":       strings.TrimSpace(payload.Definition),
		"definition_url":   strings.TrimSpace(payload.DefinitionURL),
		"instant_answer":   strings.TrimSpace(payload.Answer),
		"related_snippets": topics,
	}
	if payload.Results != nil {
		out["raw_result_count"] = len(payload.Results)
	}
	return out, nil
}

func summarizeRelatedTopics(raw []any, max int) []string {
	var out []string
	var walk func(any)
	walk = func(v any) {
		if len(out) >= max || v == nil {
			return
		}
		switch t := v.(type) {
		case map[string]any:
			if txt, ok := t["Text"].(string); ok && strings.TrimSpace(txt) != "" {
				out = append(out, strings.TrimSpace(txt))
			}
			if topics, ok := t["Topics"].([]any); ok {
				for _, x := range topics {
					walk(x)
					if len(out) >= max {
						return
					}
				}
			}
		case []any:
			for _, x := range t {
				walk(x)
				if len(out) >= max {
					return
				}
			}
		}
	}
	for _, item := range raw {
		walk(item)
		if len(out) >= max {
			break
		}
	}
	return out
}
