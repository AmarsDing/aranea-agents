package a2a

import (
	"fmt"
	"net"
	"strings"
)

const (
	PublicBaseSourceEnv     = "env"
	PublicBaseSourceDB      = "db"
	PublicBaseSourceConfig  = "config"
	PublicBaseSourceDerived = "derived"
)

// PublicBaseURLInput resolves the externally advertised A2A endpoint prefix.
type PublicBaseURLInput struct {
	EnvOverride string
	DBURL       string
	ConfigURL   string
	HTTPAddr    string
	PathPrefix  string
}

// PublicBaseURLResult is the effective public base (no trailing slash) and its source.
type PublicBaseURLResult struct {
	URL    string
	Source string
}

// ResolvePublicBaseURL priority: env > system settings (DB) > config > derived from HTTP listen addr.
func ResolvePublicBaseURL(in PublicBaseURLInput) PublicBaseURLResult {
	if u := strings.TrimRight(strings.TrimSpace(in.EnvOverride), "/"); u != "" {
		return PublicBaseURLResult{URL: u, Source: PublicBaseSourceEnv}
	}
	if u := strings.TrimRight(strings.TrimSpace(in.DBURL), "/"); u != "" {
		return PublicBaseURLResult{URL: u, Source: PublicBaseSourceDB}
	}
	if u := strings.TrimRight(strings.TrimSpace(in.ConfigURL), "/"); u != "" {
		return PublicBaseURLResult{URL: u, Source: PublicBaseSourceConfig}
	}
	prefix := strings.TrimRight(strings.TrimSpace(in.PathPrefix), "/")
	if prefix == "" {
		prefix = "/v1/a2a/public"
	}
	host := normalizeListenHost(strings.TrimSpace(in.HTTPAddr))
	if host == "" {
		host = "127.0.0.1:8000"
	}
	return PublicBaseURLResult{
		URL:    fmt.Sprintf("http://%s%s", host, prefix),
		Source: PublicBaseSourceDerived,
	}
}

func normalizeListenHost(addr string) string {
	if addr == "" {
		return ""
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		if strings.HasPrefix(addr, ":") {
			return "127.0.0.1" + addr
		}
		return addr
	}
	switch host {
	case "", "0.0.0.0", "::", "[::]":
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
}
