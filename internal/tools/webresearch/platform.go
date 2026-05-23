package webresearch

import "strings"

// PlatformFields holds platform-level defaults (system settings) for web_research.
// APIKey is only populated when resolving runtime config, not for catalog HasAPIKey checks.
type PlatformFields struct {
	HasAPIKey   bool
	APIKey      string
	Provider    string
	MaxResults  int
	FetchTop    int
	SearchDepth string
	TimeoutSec  int
	HTTPProxy   string
}

// PlatformHasAPIKey reports whether platform settings include a stored or inline key.
func PlatformHasAPIKey(p *PlatformFields) bool {
	if p == nil {
		return false
	}
	return p.HasAPIKey || strings.TrimSpace(p.APIKey) != ""
}
