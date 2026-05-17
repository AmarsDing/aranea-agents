package auth

import (
	"net/http"
	"strings"
	"sync"
)

// webhookRegistry holds path prefixes that are allowed to bypass cookie auth.
// A channel handler must call RegisterWebhookPath at init time; any
// /webhooks/* path that does not match a registered prefix is rejected with 401.
// EP-SEC-03: prevents unregistered/unknown webhook paths from silently passing
// the auth middleware without any signed-request check.
var webhookRegistry = &webhookPathRegistry{prefixes: []string{}}

type webhookPathRegistry struct {
	mu       sync.RWMutex
	prefixes []string
}

// RegisterWebhookPath marks a URL path prefix as a known webhook endpoint.
// Call this once per channel adapter during server wiring, e.g.:
//
//	auth.RegisterWebhookPath("/webhooks/")
func RegisterWebhookPath(prefix string) {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return
	}
	webhookRegistry.mu.Lock()
	defer webhookRegistry.mu.Unlock()
	for _, p := range webhookRegistry.prefixes {
		if p == prefix {
			return
		}
	}
	webhookRegistry.prefixes = append(webhookRegistry.prefixes, prefix)
}

// isRegisteredWebhookPath returns true if path matches a registered prefix.
func isRegisteredWebhookPath(path string) bool {
	webhookRegistry.mu.RLock()
	defer webhookRegistry.mu.RUnlock()
	for _, p := range webhookRegistry.prefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

// hasWebhookSigningHeader returns true when the request carries at least one
// recognised webhook signing header.  This is a lightweight early-reject guard;
// the actual cryptographic verification happens inside the channel handler.
func hasWebhookSigningHeader(r *http.Request) bool {
	knownHeaders := []string{
		"X-Lark-Signature",    // Feishu / Lark
		"X-Hub-Signature-256", // GitHub
		"X-Hub-Signature",     // GitHub (legacy SHA-1)
		"X-Signature",         // generic
		"X-Webhook-Signature", // generic
	}
	for _, h := range knownHeaders {
		if strings.TrimSpace(r.Header.Get(h)) != "" {
			return true
		}
	}
	return false
}
