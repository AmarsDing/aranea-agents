package modelregistry

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"aranea-agents/pkg/loggateway"
)

const (
	maxFetchAttempts = 4
	fetchRetryBase   = 2 * time.Second
)

func isRetryableFetchErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "fetch catalog: http 429") ||
		strings.Contains(msg, "fetch catalog: http 5") {
		return true
	}
	return strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "temporary failure") ||
		strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "tls handshake timeout") ||
		strings.Contains(msg, "i/o timeout")
}

func fetchCatalogWithRetry(ctx context.Context, sourceURL, ifNoneMatch string, lg loggateway.Logger) (FetchResult, error) {
	return attemptCatalogFetch(ctx, func() (FetchResult, error) {
		return fetchCatalogHTTPOnce(ctx, sourceURL, ifNoneMatch)
	}, lg)
}

func attemptCatalogFetch(ctx context.Context, fetch func() (FetchResult, error), lg loggateway.Logger) (FetchResult, error) {
	var lastErr error
	for attempt := 0; attempt < maxFetchAttempts; attempt++ {
		if attempt > 0 {
			delay := fetchRetryBase * time.Duration(1<<(attempt-1))
			lg.Warn("Model registry fetch retry", loggateway.StepID("model_registry.fetch.retry"), loggateway.Int("attempt", attempt+1), loggateway.Duration(int64(delay/time.Millisecond)), loggateway.Err(lastErr))
			select {
			case <-ctx.Done():
				return FetchResult{}, ctx.Err()
			case <-time.After(delay):
			}
		}
		res, err := fetch()
		if err == nil {
			return res, nil
		}
		lastErr = err
		if !isRetryableFetchErr(err) {
			return FetchResult{}, err
		}
	}
	return FetchResult{}, fmt.Errorf("fetch catalog after %d attempts: %w", maxFetchAttempts, lastErr)
}

func fetchCatalogHTTPOnce(ctx context.Context, sourceURL, ifNoneMatch string) (FetchResult, error) {
	sourceURL = strings.TrimSpace(sourceURL)
	if sourceURL == "" {
		sourceURL = DefaultPolicy().SourceURL
	}
	if err := ValidateDirectorySourceURL(sourceURL); err != nil {
		return FetchResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return FetchResult{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "aranea-agents/model-catalog-sync")
	ifNoneMatch = strings.TrimSpace(ifNoneMatch)
	if ifNoneMatch != "" {
		req.Header.Set("If-None-Match", ifNoneMatch)
	}

	client := &http.Client{Timeout: defaultFetchTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return FetchResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotModified {
		return FetchResult{
			ETag:        strings.TrimSpace(resp.Header.Get("ETag")),
			NotModified: true,
		}, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return FetchResult{}, fmt.Errorf("fetch catalog: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return FetchResult{}, err
	}
	return FetchResult{
		Body: body,
		ETag: strings.TrimSpace(resp.Header.Get("ETag")),
	}, nil
}
