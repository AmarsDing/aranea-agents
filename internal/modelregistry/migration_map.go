package modelregistry

import (
	"sort"
	"strings"
)

const ProviderMigrationVersion = "2026.05"

type ProviderMigrationRule struct {
	Legacy  string
	Catalog string
}

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

func UsageDisplayAlias(provider, model string) (string, string) {
	return MigrateProviderCode(provider), model
}

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
