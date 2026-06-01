package webresearch

import (
	"context"
	"fmt"
)

var ApplyConfigDefaults = applyConfigDefaults

var NewSearchProvider = newSearchProvider

var NewTavilyProvider = newTavilyProvider

var NewSerpAPIProvider = newSerpAPIProvider

var RedactedURL = redactedURL

var TruncateStr = truncate

var FirstNonEmpty = firstNonEmpty

var ConfigInt = configInt

var ConfigBool = configBool

var TruncateUTF8 = truncateUTF8

var BuildHTTPClient = buildHTTPClient

var EnrichHits = enrichHits

var ProviderSearch = func(p any, ctx context.Context, query string) (*SearchResponse, error) {
	type hasSearch interface {
		search(context.Context, string) (*SearchResponse, error)
	}
	s, ok := p.(hasSearch)
	if !ok {
		return nil, fmt.Errorf("not a search provider")
	}
	return s.search(ctx, query)
}
