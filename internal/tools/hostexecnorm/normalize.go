package hostexecnorm

import "encoding/json"

// NormalizeExecArgs maps legacy catalog fields to hostexec schema (working_dir → workdir).
func NormalizeExecArgs(jsonArgs []byte) []byte {
	var m map[string]any
	if err := json.Unmarshal(jsonArgs, &m); err != nil || len(m) == 0 {
		return jsonArgs
	}
	if _, hasWorkdir := m["workdir"]; hasWorkdir {
		return jsonArgs
	}
	wd, hasLegacy := m["working_dir"]
	if !hasLegacy {
		return jsonArgs
	}
	m["workdir"] = wd
	delete(m, "working_dir")
	out, err := json.Marshal(m)
	if err != nil {
		return jsonArgs
	}
	return out
}
