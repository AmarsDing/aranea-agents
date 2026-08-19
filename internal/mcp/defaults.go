package mcp

import "time"

const (
	DefaultProbeTimeoutSec     = 10
	DefaultHealthInterval      = 5 * time.Minute
	DefaultSustainedErrorAfter = 5 * time.Minute
	DefaultSessionReconnectMax = 3
	DefaultOAuth2TimeoutSec    = 15
	DefaultRuntimeTimeoutSec   = 60
	RecentReconnectWindow      = 24 * time.Hour
	// DefaultDiscoveryTimeoutSec caps one tool-discovery handshake (P2) when
	// config_json.timeout_sec is unset; DiscoveryInterval is the health-runner
	// fallback cadence for refreshing tool metadata on connectivity-only
	// servers.
	DefaultDiscoveryTimeoutSec = 15
	DefaultDiscoveryInterval   = 30 * time.Minute
)
