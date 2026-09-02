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

// ResolveStallPolicy returns the per-model-pack post-first-byte liveness
// policy: catalog ConfigJSON stall_timeout_sec / stall_max_attempts override
// the provider package defaults (90s / 5 reconnects) so long-thinking packs
// can widen the gap budget and weak-link packs can widen the reconnect cap.
// Missing catalog, empty JSON, or missing fields fall back to the defaults.
func ResolveStallPolicy(ctx context.Context, catalog biz.TeamModelCatalog, providerName, model string) (time.Duration, int) {
	stallTimeout, maxReconnects := provider.DefaultStallTimeout, provider.DefaultStallMaxReconnects
	if catalog == nil {
		return stallTimeout, maxReconnects
	}
	p := strings.TrimSpace(providerName)
	m := strings.TrimSpace(model)
	if p == "" || m == "" {
		return stallTimeout, maxReconnects
	}
	row, err := catalog.GetByProviderAndModel(ctx, p, m)
	if err != nil {
		return stallTimeout, maxReconnects
	}
	if d, n := provider.StallPolicyFromConfigJSON(row.ConfigJSON); d > 0 || n > 0 {
		if d > 0 {
			stallTimeout = d
		}
		if n > 0 {
			maxReconnects = n
		}
	}
	return stallTimeout, maxReconnects
}
