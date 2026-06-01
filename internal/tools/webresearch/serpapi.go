package webresearch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const serpAPIBaseURL = "https://serpapi.com/search.json"

type serpAPIProvider struct {
	client *http.Client
	apiKey string
	cfg    Config
}

func newSerpAPIProvider(cfg Config) (*serpAPIProvider, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, fmt.Errorf("web_research: serpapi api_key is required (tool config or SERPAPI_API_KEY)")
	}
	return &serpAPIProvider{
		client: buildHTTPClient(cfg),
		apiKey: strings.TrimSpace(cfg.APIKey),
		cfg:    cfg,
	}, nil
}

func (p *serpAPIProvider) search(ctx context.Context, query string) (*SearchResponse, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("web_research: query is required")
	}
	baseURL := serpAPIBaseURL
	if u := strings.TrimSpace(p.cfg.SerpAPIBaseURL); u != "" {
		baseURL = u
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("engine", "google")
	q.Set("q", query)
	// TPM-P2-08: SerpAPI only supports query-string auth; header auth is not available.
	// The key MUST NOT appear in any log output. Use redactedURL when logging.
	q.Set("api_key", p.apiKey)
	q.Set("num", fmt.Sprintf("%d", p.cfg.MaxResults))
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}

	started := time.Now()
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("web_research: serpapi request failed for %s: %w", redactedURL(u), err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("web_research: serpapi status %d: %s", resp.StatusCode, truncate(string(raw), 512))
	}

	var parsed struct {
		OrganicResults []struct {
			Title   string `json:"title"`
			Link    string `json:"link"`
			Snippet string `json:"snippet"`
		} `json:"organic_results"`
		AnswerBox struct {
			Answer     string `json:"answer"`
			Snippet    string `json:"snippet"`
			Title      string `json:"title"`
			Link       string `json:"link"`
			Result     string `json:"result"`
			Displayed  string `json:"displayed_link"`
		} `json:"answer_box"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("web_research: serpapi decode: %w", err)
	}

	out := &SearchResponse{
		Query:        query,
		Provider:     ProviderSerpAPI,
		ResponseTime: time.Since(started).Seconds(),
	}
	ab := parsed.AnswerBox
	switch {
	case strings.TrimSpace(ab.Answer) != "":
		out.Answer = strings.TrimSpace(ab.Answer)
	case strings.TrimSpace(ab.Result) != "":
		out.Answer = strings.TrimSpace(ab.Result)
	case strings.TrimSpace(ab.Snippet) != "":
		out.Answer = strings.TrimSpace(ab.Snippet)
	}
	for _, r := range parsed.OrganicResults {
		out.Results = append(out.Results, Hit{
			Title:   strings.TrimSpace(r.Title),
			URL:     strings.TrimSpace(r.Link),
			Snippet: strings.TrimSpace(r.Snippet),
		})
		if len(out.Results) >= p.cfg.MaxResults {
			break
		}
	}
	return out, nil
}

func redactedURL(u *url.URL) string {
	c := *u
	q := c.Query()
	if q.Has("api_key") {
		q.Set("api_key", "***")
	}
	c.RawQuery = q.Encode()
	return c.String()
}
