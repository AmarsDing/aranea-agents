package ingest

import "context"

// RawItem is a normalized ingest hit before ContentItem persistence.
type RawItem struct {
	SourceType string
	SourceURL  string
	Title      string
	Body       string
	Lang       string
	RawPayload string
}

// Connector fetches external candidates (RSS / search / future platforms).
type Connector interface {
	Name() string
	Fetch(ctx context.Context, cursor string) (items []RawItem, nextCursor string, err error)
}
