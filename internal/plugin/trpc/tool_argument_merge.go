package plugintrpc

import (
	"encoding/json"
)

// mergeToolArgumentsJSON applies modify_patch to current tool arguments.
//
// Precedence (documented in docs/需求/28 callback.design.md §Hook modify):
//   - "arguments": replaces the entire argument object when marshaling succeeds.
//   - "merge_arguments": deep-merges into the current object; nested maps merge recursively;
//     scalars and arrays from the patch replace the destination value.
func mergeToolArgumentsJSON(current []byte, patch map[string]any) []byte {
	if len(patch) == 0 {
		return nil
	}
	if raw, ok := patch["arguments"]; ok && raw != nil {
		if b, err := json.Marshal(raw); err == nil {
			return b
		}
	}
	mergeMap, _ := patch["merge_arguments"].(map[string]any)
	if len(mergeMap) == 0 {
		return nil
	}
	base := map[string]any{}
	if len(current) > 0 {
		_ = json.Unmarshal(current, &base)
	}
	deepMergeMap(base, mergeMap)
	b, err := json.Marshal(base)
	if err != nil {
		return nil
	}
	return b
}

// deepMergeMap merges src into dst. Maps recurse; other types replace dst[k].
func deepMergeMap(dst, src map[string]any) {
	for k, v := range src {
		if v == nil {
			dst[k] = nil
			continue
		}
		srcMap, srcIsMap := v.(map[string]any)
		if !srcIsMap {
			dst[k] = v
			continue
		}
		if dstVal, ok := dst[k]; ok {
			if dstMap, ok := dstVal.(map[string]any); ok {
				deepMergeMap(dstMap, srcMap)
				continue
			}
		}
		dst[k] = cloneMapAny(srcMap)
	}
}

func cloneMapAny(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		if child, ok := v.(map[string]any); ok {
			out[k] = cloneMapAny(child)
			continue
		}
		out[k] = v
	}
	return out
}
