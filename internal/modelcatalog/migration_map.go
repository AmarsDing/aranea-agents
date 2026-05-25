package modelcatalog

import (
	"sort"
	"strings"
)

// ProviderMigrationVersion bumps when embed rules change (checkpoint versioning).
const ProviderMigrationVersion = "2026.05"

// ProviderMigrationRule is one legacy → models.dev id pair (read-only, shipped with releases).
type ProviderMigrationRule struct {
	Legacy  string
	Catalog string
}

// ListProviderMigrationRules returns built-in rename rules for API/UI (deterministic order).
func ListProviderMigrationRules() []ProviderMigrationRule {
	out := make([]ProviderMigrationRule, 0, len(ProviderMigration))
	keys := make([]string, 0, len(ProviderMigration))
	for legacy := range ProviderMigration {
		keys = append(keys, legacy)
	}
	sort.Strings(keys)
	for _, legacy := range keys {
		out = append(out, ProviderMigrationRule{Legacy: legacy, Catalog: ProviderMigration[legacy]})
	}
	return out
}

// MigrateProviderCode maps legacy provider_code to models.dev canonical id.
func MigrateProviderCode(code string) string {
	code = strings.TrimSpace(code)
	if code == "" {
		return code
	}
	if target, ok := ProviderMigration[code]; ok {
		return target
	}
	return code
}

// UsageDisplayAlias returns normalized provider/model for read-only Usage aggregation.
func UsageDisplayAlias(provider, model string) (string, string) {
	return MigrateProviderCode(provider), model
}

// ProviderUsageQueryCodes returns provider_code values to match in usage queries
// (canonical id plus any legacy aliases that map to it).
func ProviderUsageQueryCodes(provider string) []string {
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return nil
	}
	set := map[string]struct{}{provider: {}}
	for legacy, target := range ProviderMigration {
		if target == provider || legacy == provider {
			set[legacy] = struct{}{}
			set[target] = struct{}{}
		}
	}
	out := make([]string, 0, len(set))
	for code := range set {
		out = append(out, code)
	}
	return out
}
