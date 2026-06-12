package webresearch

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/outboundguard"

	trpchttpfetch "trpc.group/trpc-go/trpc-agent-go/tool/webfetch/httpfetch"
)

// enrichHits fetches page bodies for hits missing content (up to fetchTop URLs).
// Returns per-URL warning messages; a non-nil aggregate error means total fetch failure.
func enrichHits(ctx context.Context, hits []Hit, fetchTop int, cfg Config, lg loggateway.Logger) ([]string, error) {
	if fetchTop <= 0 || len(hits) == 0 {
		return nil, nil
	}
	var urls []string
	for i := range hits {
		if len(urls) >= fetchTop {
			break
		}
		if strings.TrimSpace(hits[i].Content) != "" {
			continue
		}
		u := strings.TrimSpace(hits[i].URL)
		if u == "" {
			continue
		}
		// Validate URL against SSRF — skip internal/private addresses.
		if err := outboundguard.ValidateURL(u); err != nil {
			if lg != nil {
				lg.Warn("skipping non-public URL in search results",
					loggateway.StepID("tool.webresearch.ssrf_skip"),
					loggateway.Str("url", u),
					loggateway.Err(err))
			}
			continue
		}
		urls = append(urls, u)
	}
	if len(urls) == 0 {
		return nil, nil
	}

	fetchTool := trpchttpfetch.NewTool(trpchttpfetch.WithHTTPClient(buildHTTPClient(cfg, lg)))
	ct, ok := fetchTool.(interface {
		Call(context.Context, []byte) (any, error)
	})
	if !ok {
		return nil, fmt.Errorf("web_research: http fetch tool unavailable")
	}
	args, err := json.Marshal(map[string][]string{"urls": urls})
	if err != nil {
		return nil, err
	}
	out, err := ct.Call(ctx, args)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Results []struct {
			RetrievedURL string `json:"retrieved_url"`
			Content      string `json:"content"`
			Error        string `json:"error"`
		} `json:"results"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, err
	}
	byURL := make(map[string]string, len(parsed.Results))
	var warnings []string
	for _, r := range parsed.Results {
		u := strings.TrimSpace(r.RetrievedURL)
		if strings.TrimSpace(r.Error) != "" {
			warnings = append(warnings, fmt.Sprintf("%s: %s", u, strings.TrimSpace(r.Error)))
			continue
		}
		if strings.TrimSpace(r.Content) == "" {
			continue
		}
		byURL[u] = r.Content
	}
	for i := range hits {
		if strings.TrimSpace(hits[i].Content) != "" {
			continue
		}
		if c, ok := byURL[strings.TrimSpace(hits[i].URL)]; ok {
			hits[i].Content = c
		}
	}
	return warnings, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
