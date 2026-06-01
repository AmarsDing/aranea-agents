package modelregistry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

const defaultFetchTimeout = 120 * time.Second

type FetchResult struct {
	Body        []byte
	ETag        string
	NotModified bool
}

var catalogFetchHook func(ctx context.Context, sourceURL, ifNoneMatch string) (FetchResult, error)

func FetchDirectory(ctx context.Context, sourceURL, ifNoneMatch string) (FetchResult, error) {
	if catalogFetchHook != nil {
		return catalogFetchHook(ctx, sourceURL, ifNoneMatch)
	}
	return fetchCatalogWithRetry(ctx, sourceURL, ifNoneMatch)
}

func ParseDirectory(body []byte) (Directory, error) {
	var cat Directory
	if err := json.Unmarshal(body, &cat); err != nil {
		return nil, fmt.Errorf("parse catalog: %w", err)
	}
	if len(cat) == 0 {
		return nil, fmt.Errorf("parse catalog: empty providers")
	}
	return cat, nil
}

func SHA256Hex(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
