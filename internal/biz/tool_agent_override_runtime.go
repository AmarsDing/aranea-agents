package biz

import (
	"encoding/json"
	"strings"
)

// ApplyAgentToolOverrides adjusts effective tool rows using per-agent overrides.
// Priority: override (deny / allow / inherit+enabled) wins over profile/catalog policy.
func ApplyAgentToolOverrides(eff *AgentEffectiveTools, catalog []Tool, overrides []ToolAgentOverride) {
	if eff == nil || len(overrides) == 0 {
		return
	}
	catalogByKey := make(map[string]Tool, len(catalog))
	for _, t := range catalog {
		catalogByKey[t.Key] = t
	}
	index := make(map[string]int, len(eff.Items))
	for i, it := range eff.Items {
		index[it.ToolKey] = i
	}
	for _, o := range overrides {
		key := strings.TrimSpace(o.ToolKey)
		if key == "" {
			continue
		}
		if i, ok := index[key]; ok {
			applyOverrideToEffectiveItem(&eff.Items[i], o, eff.ToolsEnabled)
			continue
		}
		tool, ok := catalogByKey[key]
		if !ok {
			continue
		}
		st, rsn, en := computeEffectiveToolState(
			AgentRuntimeSettings{ToolsEnabled: eff.ToolsEnabled, ToolsProfile: eff.Profile},
			tool, eff.Profile, computePolicyAllowedSet(eff.Profile, eff.Allow, catalog), computePolicyDenySet(eff.Deny, catalog),
		)
		item := EffectiveAgentTool{
			ToolKey:        key,
			DisplayName:    tool.DisplayName,
			Category:       tool.Category,
			Source:         tool.Source,
			Enabled:        en,
			EffectiveState: st,
			Reason:         rsn,
		}
		applyOverrideToEffectiveItem(&item, o, eff.ToolsEnabled)
		eff.Items = append(eff.Items, item)
		index[key] = len(eff.Items) - 1
	}
}

func applyOverrideToEffectiveItem(item *EffectiveAgentTool, o ToolAgentOverride, toolsEnabled bool) {
	if item == nil {
		return
	}
	mode := strings.ToLower(strings.TrimSpace(o.Mode))
	switch mode {
	case "deny":
		item.Enabled = false
		item.EffectiveState = "denied"
		item.Reason = "override_deny"
		return
	case "allow", "override":
		if !toolsEnabled {
			item.Enabled = false
			item.EffectiveState = "denied"
			item.Reason = "agent_tools_disabled"
			return
		}
		item.Enabled = true
		item.EffectiveState = "allowed"
		item.Reason = "override_allow"
		return
	default:
		if !toolsEnabled {
			item.Enabled = false
			item.EffectiveState = "denied"
			item.Reason = "agent_tools_disabled"
			return
		}
		if o.Enabled {
			item.Enabled = true
			item.EffectiveState = "allowed"
			item.Reason = "override_enabled"
		} else {
			item.Enabled = false
			item.EffectiveState = "denied"
			item.Reason = "override_disabled"
		}
	}
}

// MergeToolConfigJSON merges catalog/default config with an agent override object (override wins).
func MergeToolConfigJSON(baseJSON, overrideJSON string) map[string]any {
	out := map[string]any{}
	mergeJSONMap(out, baseJSON)
	mergeJSONMap(out, overrideJSON)
	return out
}

func mergeJSONMap(dst map[string]any, raw string) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" {
		return
	}
	var patch map[string]any
	if json.Unmarshal([]byte(raw), &patch) != nil {
		return
	}
	for k, v := range patch {
		dst[k] = v
	}
}
