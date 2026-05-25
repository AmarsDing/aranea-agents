package modelcatalog

import (
	"fmt"
	"strings"
)

var validSyncPolicies = map[string]struct{}{
	"off":        {},
	"scheduled":  {},
}

var validAutoApplyModes = map[string]struct{}{
	"none":                          {},
	"metadata_and_pricing":          {},
	"full_spec":                     {},
	"full_spec_and_runtime_overlay": {},
}

// NormalizePolicy validates and normalizes catalog sync policy fields.
func NormalizePolicy(p Policy) (Policy, error) {
	if strings.TrimSpace(p.SourceURL) == "" {
		p.SourceURL = DefaultPolicy().SourceURL
	}
	if err := ValidateCatalogSourceURL(p.SourceURL); err != nil {
		return Policy{}, err
	}
	p.SyncPolicy = strings.ToLower(strings.TrimSpace(p.SyncPolicy))
	if p.SyncPolicy == "" {
		p.SyncPolicy = "scheduled"
	}
	if _, ok := validSyncPolicies[p.SyncPolicy]; !ok {
		return Policy{}, fmt.Errorf("invalid sync_policy %q (want off|scheduled)", p.SyncPolicy)
	}
	if p.SyncIntervalHours <= 0 {
		p.SyncIntervalHours = 24
	}
	p.AutoApply = strings.ToLower(strings.TrimSpace(p.AutoApply))
	if p.AutoApply == "" {
		p.AutoApply = "metadata_and_pricing"
	}
	if _, ok := validAutoApplyModes[p.AutoApply]; !ok {
		return Policy{}, fmt.Errorf("invalid auto_apply %q", p.AutoApply)
	}
	return p, nil
}
