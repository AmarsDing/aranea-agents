package data

import (
	"sort"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/modelregistry"
)

func mergeUsageBreakdownByAlias(rows []biz.UsageBreakdownRow) []biz.UsageBreakdownRow {
	if len(rows) == 0 {
		return rows
	}
	type key struct {
		provider string
		model    string
	}
	merged := make(map[key]*biz.UsageBreakdownRow, len(rows))
	for _, row := range rows {
		p, m := modelregistry.UsageDisplayAlias(row.ProviderCode, row.ModelAPIID)
		k := key{provider: p, model: m}
		if ex, ok := merged[k]; ok {
			ex.CallCount += row.CallCount
			ex.InputTokens += row.InputTokens
			ex.OutputTokens += row.OutputTokens
			ex.TotalTokens += row.TotalTokens
			ex.TotalCostMicroUSD += row.TotalCostMicroUSD
			if row.ModelDisplayName != "" {
				ex.ModelDisplayName = row.ModelDisplayName
			}
			continue
		}
		copy := row
		copy.ProviderCode = p
		copy.ModelAPIID = m
		merged[k] = &copy
	}
	out := make([]biz.UsageBreakdownRow, 0, len(merged))
	for _, row := range merged {
		out = append(out, *row)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].TotalCostMicroUSD == out[j].TotalCostMicroUSD {
			return out[i].CallCount > out[j].CallCount
		}
		return out[i].TotalCostMicroUSD > out[j].TotalCostMicroUSD
	})
	return out
}

func aliasUsageEvent(event biz.TokenUsageEvent) biz.TokenUsageEvent {
	provider := strings.TrimSpace(event.CanonicalProviderCode)
	if provider == "" {
		provider, _ = modelregistry.UsageDisplayAlias(event.ProviderCode, event.ModelAPIID)
	}
	event.ProviderCode = provider
	return event
}

func usageProviderWhere(provider string) (string, []any) {
	codes := modelregistry.ProviderUsageQueryCodes(provider)
	if len(codes) == 0 {
		return "provider_code = ?", []any{provider}
	}
	if len(codes) == 1 {
		return "provider_code = ?", []any{codes[0]}
	}
	placeholders := strings.Repeat("?,", len(codes)-1) + "?"
	args := make([]any, len(codes))
	for i, c := range codes {
		args[i] = c
	}
	return "provider_code IN (" + placeholders + ")", args
}
