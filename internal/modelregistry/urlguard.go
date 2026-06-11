package modelregistry

import (
	"fmt"
	"net/url"
	"strings"

	"aranea-agents/pkg/outboundguard"
)

var allowedCatalogHosts = map[string]struct{}{
	"models.dev":     {},
	"www.models.dev": {},
}

func ValidateDirectorySourceURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid source_url: %w", err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("source_url must use https")
	}
	host := strings.ToLower(strings.TrimSpace(u.Hostname()))
	if host == "" {
		return fmt.Errorf("source_url host is required")
	}
	path := strings.ToLower(strings.TrimSpace(u.Path))
	if _, ok := allowedCatalogHosts[host]; ok {
		if path == "/api.json" {
			return nil
		}
		return fmt.Errorf("source_url on models.dev must be /api.json")
	}
	if err := outboundguard.ValidatePublicHost(host); err != nil {
		return fmt.Errorf("source_url: %w", err)
	}
	return nil
}

func ValidateLogoSourceURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fmt.Errorf("empty logo url")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid logo url: %w", err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("logo url must use https")
	}
	host := strings.ToLower(strings.TrimSpace(u.Hostname()))
	if host != "models.dev" && host != "www.models.dev" {
		return fmt.Errorf("logo url host not allowed")
	}
	if !strings.HasPrefix(strings.ToLower(u.Path), "/logos/") {
		return fmt.Errorf("logo url path must be under /logos/")
	}
	return nil
}
