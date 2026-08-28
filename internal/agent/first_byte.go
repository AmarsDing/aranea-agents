package agent

import (
	"context"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/provider"
)

// ResolveFirstByteTimeout returns the per-model-pack first-byte stall
// budget. catalog ConfigJSON first_byte_timeout_sec overrides the 30s
// product default so thinking packs can wait 60–90s without a global raise.
// Missing catalog, empty JSON, or non-positive sec fall back to
// DefaultFirstByteTimeout.
func ResolveFirstByteTimeout(ctx context.Context, catalog biz.TeamModelCatalog, providerName, model string) time.Duration {
	if catalog == nil {
		return DefaultFirstByteTimeout
	}
	p := strings.TrimSpace(providerName)
	m := strings.TrimSpace(model)
	if p == "" || m == "" {
		return DefaultFirstByteTimeout
	}
	row, err := catalog.GetByProviderAndModel(ctx, p, m)
	if err != nil {
		return DefaultFirstByteTimeout
	}
	if d := provider.FirstByteTimeoutFromConfigJSON(row.ConfigJSON); d > 0 {
		return d
	}
	return DefaultFirstByteTimeout
}
