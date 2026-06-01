package webresearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"aranea-agents/pkg/loggateway"
)

const tavilySearchURL = "https://api.tavily.com/search"

type tavilyProvider struct {
	client *http.Client
	apiKey string
	cfg    Config
}

func newTavilyProvider(cfg Config, lg loggateway.Logger) (*tavilyProvider, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, fmt.Errorf("web_research: tavily api_key is required (tool config or TAVILY_API_KEY)")
	}
	return &tavilyProvider{
		client: buildHTTPClient(cfg, lg),
		apiKey: strings.TrimSpace(cfg.APIKey),
		cfg:    cfg,
	}, nil
}

type tavilySearchRequest struct {
	Query             string `json:"query"`
	SearchDepth       string `json:"search_depth,omitempty"`
	MaxResults        int    `json:"max_results,omitempty"`
	IncludeAnswer     bool   `json:"include_answer,omitempty"`
	IncludeRawContent bool   `json:"include_raw_content,omitempty"`
}

type tavilySearchResponse struct {
	Query        string        `json:"query"`
	Answer       string        `json:"answer"`
	Results      []tavilyHit   `json:"results"`
	ResponseTime float64       `json:"response_time"`
}

type tavilyHit struct {
	Title      string  `json:"title"`
	URL        string  `json:"url"`
	Content    string  `json:"content"`
	RawContent string  `json:"raw_content"`
	Score      float64 `json:"score"`
}

func (p *tavilyProvider) search(ctx context.Context, query string) (*SearchResponse, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("web_research: query is required")
	}
	body, err := json.Marshal(tavilySearchRequest{
		Query:             query,
		SearchDepth:       p.cfg.SearchDepth,
		MaxResults:        p.cfg.MaxResults,
		IncludeAnswer:     p.cfg.IncludeAnswer,
		IncludeRawContent: p.cfg.IncludeRawContent,
	})
	if err != nil {
		return nil, err
	}
	searchURL := tavilySearchURL
	if u := strings.TrimSpace(p.cfg.TavilySearchURL); u != "" {
		searchURL = u
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, searchURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	started := time.Now()
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("web_research: tavily request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("web_research: tavily status %d: %s", resp.StatusCode, truncate(string(raw), 512))
	}

	var parsed tavilySearchResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("web_research: tavily decode: %w", err)
	}

	out := &SearchResponse{
		Query:        firstNonEmpty(parsed.Query, query),
		Answer:       strings.TrimSpace(parsed.Answer),
		Provider:     ProviderTavily,
		ResponseTime: parsed.ResponseTime,
	}
	if out.ResponseTime <= 0 {
		out.ResponseTime = time.Since(started).Seconds()
	}
	for _, r := range parsed.Results {
		content := strings.TrimSpace(r.Content)
		if content == "" {
			content = strings.TrimSpace(r.RawContent)
		}
		out.Results = append(out.Results, Hit{
			Title:   strings.TrimSpace(r.Title),
			URL:     strings.TrimSpace(r.URL),
			Snippet: strings.TrimSpace(r.Content),
			Content: content,
			Score:   r.Score,
		})
	}
	return out, nil
}
