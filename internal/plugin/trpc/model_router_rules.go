package plugintrpc

import (
	"regexp"
	"sort"
	"strings"
)

// ModelRouterRule is one configurable routing rule (rules[] in model_router config).
type ModelRouterRule struct {
	Model    string   `json:"model"`
	Contains []string `json:"contains"`
	Regex    string   `json:"regex"`
	MinChars int      `json:"min_chars"`
	Priority int      `json:"priority"`
}

func resolveModelFromRules(prompt string, rules []ModelRouterRule) string {
	if len(rules) == 0 {
		return ""
	}
	promptLower := strings.ToLower(prompt)
	sorted := append([]ModelRouterRule(nil), rules...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Priority == sorted[j].Priority {
			return i < j
		}
		return sorted[i].Priority > sorted[j].Priority
	})
	for _, rule := range sorted {
		model := strings.TrimSpace(rule.Model)
		if model == "" {
			continue
		}
		if rule.MinChars > 0 && len(prompt) < rule.MinChars {
			continue
		}
		if pat := strings.TrimSpace(rule.Regex); pat != "" {
			re, err := regexp.Compile(pat)
			if err != nil {
				continue
			}
			if re.MatchString(prompt) {
				return model
			}
		}
		for _, sub := range rule.Contains {
			sub = strings.TrimSpace(sub)
			if sub == "" {
				continue
			}
			if strings.Contains(promptLower, strings.ToLower(sub)) {
				return model
			}
		}
	}
	return ""
}
