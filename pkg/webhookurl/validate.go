// Package webhookurl validates outbound Hook/Gateway webhook targets (SSRF guard).
package webhookurl

import (
	"fmt"
	"net/url"
	"strings"

	"aranea-agents/pkg/outboundguard"
)

// ValidateNotifyURL ensures url is a safe outbound HTTPS/HTTP POST target.
// Also used for Gateway outbound webhooks (gateway_webhooks.url).
func ValidateNotifyURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("invalid webhook url: %w", err)
	}
	scheme := strings.ToLower(strings.TrimSpace(u.Scheme))
	if scheme != "https" && scheme != "http" {
		return fmt.Errorf("webhook url must use http or https")
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("webhook url host is required")
	}
	if err := outboundguard.ValidatePublicHost(host); err != nil {
		return fmt.Errorf("webhook url: %w", err)
	}
	return nil
}
