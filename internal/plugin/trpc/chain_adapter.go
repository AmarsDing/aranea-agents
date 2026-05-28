package plugintrpc

import (
	"strings"

	"aranea-agents/internal/biz"
)

// builtinCallbackPoints lists callback points implemented per built-in plugin key.
var builtinCallbackPoints = map[string][]string{
	"audit_log":           {"before_agent", "after_agent", "before_model", "after_model", "before_tool", "after_tool", "on_event"},
	"skill_usage_tracker": {"before_tool", "after_tool"},
	"retry_and_reflect":   {"after_agent", "after_tool"},
	"sensitive_data_mask": {"before_model", "after_model"},
	"confirmation_guard":  {"before_tool"},
	"cost_guard":          {"before_model"},
	"model_router":        {"before_model"},
	"permission_guard":    {"before_tool"},
	"output_policy":       {"after_model", "on_event"},
}

// BuiltinCallbackPoints returns declared lifecycle points for a built-in plugin key.
func BuiltinCallbackPoints(pluginKey string) []string {
	key := strings.ToLower(strings.TrimSpace(pluginKey))
	if pts, ok := builtinCallbackPoints[key]; ok {
		out := make([]string, len(pts))
		copy(out, pts)
		return out
	}
	return nil
}

// ValidatePluginCallbackPoints logs when DB callback_points_json diverges from built-in registry.
func ValidatePluginCallbackPoints(p biz.Plugin) {
	declared := BuiltinCallbackPoints(p.Key)
	if len(declared) == 0 {
		return
	}
	if len(p.CallbackPoints) == 0 {
		return
	}
	declSet := make(map[string]struct{}, len(declared))
	for _, pt := range declared {
		declSet[strings.ToLower(strings.TrimSpace(pt))] = struct{}{}
	}
	for _, pt := range p.CallbackPoints {
		norm := strings.ToLower(strings.TrimSpace(pt))
		if _, ok := declSet[norm]; !ok {
			getHookLogger().Warn("plugin: callback_point not implemented by builtin",
				"plugin", p.Key,
				"point", pt,
				"implemented", declared,
			)
		}
	}
}
