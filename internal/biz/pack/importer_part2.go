package pack

import (
	"encoding/json"
)

// 毫秒→秒的单位换算常量。spec 字段名是 RecoveryTimeoutMs，biz 字段名是
// ResetTimeoutSeconds，避免散落的 magic 1000。
const msPerSecond = 1000

func msToSec(ms int) int { return ms / msPerSecond }

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func sliceToJSONList(items []string) string {
	if len(items) == 0 {
		return "[]"
	}
	data, err := json.Marshal(items)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func buildSkillRuntimeJSON(spec *AgentSkillsSpec) string {
	type runtimePolicy struct {
		AllowedSlugs []string `json:"allowed_slugs"`
		DeniedSlugs  []string `json:"denied_slugs"`
	}
	p := runtimePolicy{
		AllowedSlugs: spec.Allowed,
		DeniedSlugs:  spec.Denied,
	}
	data, err := json.Marshal(p)
	if err != nil {
		return ""
	}
	return string(data)
}
