package service

import (
	"context"
	"net/url"
	"strconv"

	"github.com/go-kratos/kratos/v2/transport"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
)

// pageQueryFromContext reads page/page_size (or camelCase) from the HTTP query.
// ok is false when the request is not HTTP or neither page nor page_size is present,
// so callers can keep the legacy unpaginated List path for pickers/health/resolvers.
func pageQueryFromContext(ctx context.Context) (page, pageSize int32, ok bool) {
	q, hasQuery := httpQuery(ctx)
	if !hasQuery {
		return 0, 0, false
	}
	pageRaw := firstQuery(q, "page")
	sizeRaw := firstQuery(q, "page_size", "pageSize")
	if pageRaw == "" && sizeRaw == "" {
		return 0, 0, false
	}
	page, pageSize = 1, 20
	if pageRaw != "" {
		if n, err := strconv.Atoi(pageRaw); err == nil && n > 0 {
			page = int32(n)
		}
	}
	if sizeRaw != "" {
		if n, err := strconv.Atoi(sizeRaw); err == nil && n > 0 {
			pageSize = int32(n)
		}
	}
	return page, pageSize, true
}

func searchQueryFromContext(ctx context.Context) string {
	q, ok := httpQuery(ctx)
	if !ok {
		return ""
	}
	return firstQuery(q, "search", "q", "keyword")
}

func queryParamFromContext(ctx context.Context, keys ...string) string {
	q, ok := httpQuery(ctx)
	if !ok {
		return ""
	}
	return firstQuery(q, keys...)
}

func httpQuery(ctx context.Context) (url.Values, bool) {
	tr, ok := transport.FromServerContext(ctx)
	if !ok {
		return nil, false
	}
	ht, ok := tr.(*khttp.Transport)
	if !ok || ht.Request() == nil {
		return nil, false
	}
	return ht.Request().URL.Query(), true
}

func firstQuery(q url.Values, keys ...string) string {
	for _, k := range keys {
		if v := q.Get(k); v != "" {
			return v
		}
	}
	return ""
}
