package rss

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"aranea-agents/internal/biz/content"
	"aranea-agents/internal/connector/ingest"
)

// Adapter is a real RSS/Atom fetcher (low-risk ingest). Not a platform publisher.
type Adapter struct {
	FeedURL    string
	HTTPClient *http.Client
}

func (a *Adapter) Name() string { return "rss" }

type rssFeed struct {
	Channel struct {
		Items []rssItem `xml:"item"`
	} `xml:"channel"`
	Entries []atomEntry `xml:"entry"`
}

type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	GUID        string `xml:"guid"`
}

type atomEntry struct {
	Title string `xml:"title"`
	Link  struct {
		Href string `xml:"href,attr"`
	} `xml:"link"`
	Summary string `xml:"summary"`
	ID      string `xml:"id"`
}

func (a *Adapter) Fetch(ctx context.Context, cursor string) ([]ingest.RawItem, string, error) {
	if a.FeedURL == "" {
		return nil, "", fmt.Errorf("rss: feed URL required")
	}
	client := a.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.FeedURL, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("rss: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, "", err
	}
	return ParseFeedXML(body)
}

// ParseFeedXML parses RSS or Atom XML into RawItems (testable without network).
func ParseFeedXML(data []byte) ([]ingest.RawItem, string, error) {
	var feed rssFeed
	if err := xml.Unmarshal(data, &feed); err != nil {
		return nil, "", err
	}
	out := make([]ingest.RawItem, 0)
	for _, it := range feed.Channel.Items {
		out = append(out, ingest.RawItem{
			SourceType: content.SourceRSS,
			SourceURL:  strings.TrimSpace(it.Link),
			Title:      strings.TrimSpace(it.Title),
			Body:       strings.TrimSpace(it.Description),
			RawPayload: it.GUID,
		})
	}
	for _, it := range feed.Entries {
		out = append(out, ingest.RawItem{
			SourceType: content.SourceRSS,
			SourceURL:  strings.TrimSpace(it.Link.Href),
			Title:      strings.TrimSpace(it.Title),
			Body:       strings.TrimSpace(it.Summary),
			RawPayload: it.ID,
		})
	}
	return out, "", nil
}
