package biz

import (
	"encoding/json"
	"strings"
)

// MergeAgentConfigJSON shallow-merges patch into base (top-level keys from patch win).
// Used on Agent catalog PATCH so partial other_config updates do not wipe unrelated keys.
func MergeAgentConfigJSON(baseJSON, patchJSON string) string {
	merged := MergeToolConfigJSON(baseJSON, patchJSON)
	if len(merged) == 0 {
		return "{}"
	}
	b, err := json.Marshal(merged)
	if err != nil {
		return strings.TrimSpace(patchJSON)
	}
	return string(b)
}
