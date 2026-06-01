package plugintrpc

import (
	"regexp"
	"sort"
	"strings"

	"aranea-agents/pkg/loggateway"
)

// ModelRouterRule is one configurable routing rule (rules[] in model_router config).
type ModelRouterRule struct {
	Model    string   `json:"model"`
	Contains []string `json:"contains"`
	Regex    string   `json:"regex"`
	MinChars int      `json:"min_chars"`
	Priority int      `json:"priority"`

	compiledRegex *regexp.Regexp
}

func (r *ModelRouterRule) compile(lg loggateway.Logger) {
	if pat := strings.TrimSpace(r.Regex); pat != "" {
		if re, err := regexp.Compile(pat); err == nil {
			r.compiledRegex = re
		} else {
			lg.Warn("model_router rule regex compile failed",
				loggateway.StepID("plugin.model_router.compile_fail"),
				loggateway.Str("model", r.Model),
				loggateway.Str("regex", pat),
				loggateway.Err(err),
			)
		}
	}
}

func (r *ModelRouterRule) compiled() *regexp.Regexp {
	return r.compiledRegex
}

func compileModelRouterRules(rules []ModelRouterRule, lg loggateway.Logger) {
	for i := range rules {
		rules[i].compile(lg)
	}
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
	for i := range sorted {
		rule := &sorted[i]
		model := strings.TrimSpace(rule.Model)
		if model == "" {
			continue
		}
		if rule.MinChars > 0 && len(prompt) < rule.MinChars {
			continue
		}
		if re := rule.compiled(); re != nil {
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
