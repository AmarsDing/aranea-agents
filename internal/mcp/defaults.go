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
)
