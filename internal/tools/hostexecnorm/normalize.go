package hostexecnorm

import (
	"encoding/json"
	"strconv"
	"strings"
)

// NormalizeExecArgs maps common LLM/catalog aliases onto the hostexec schema
// so exec_command does not fail with "command required" or ignore cwd.
//
//	command     ← cmd, cmd_line, cmdline
//	workdir     ← working_dir, cwd, dir, directory, working_directory
//	timeout_sec ← timeout, timeout_seconds
func NormalizeExecArgs(jsonArgs []byte) []byte {
	var m map[string]any
	if err := json.Unmarshal(jsonArgs, &m); err != nil || len(m) == 0 {
		return jsonArgs
	}
	changed := false
	if copyStringIfEmpty(m, "command", "cmd", "cmd_line", "cmdline") {
		changed = true
	}
	if copyStringIfEmpty(m, "workdir", "working_dir", "cwd", "dir", "directory", "working_directory") {
		changed = true
	}
	if copyNumberIfEmpty(m, "timeout_sec", "timeout", "timeout_seconds") {
		changed = true
	}
	if !changed {
		return jsonArgs
	}
	out, err := json.Marshal(m)
	if err != nil {
		return jsonArgs
	}
	return out
}

func copyStringIfEmpty(m map[string]any, dest string, srcs ...string) bool {
	if !missingString(m, dest) {
		return false
	}
	for _, src := range srcs {
		s, ok := m[src].(string)
		if !ok || strings.TrimSpace(s) == "" {
			continue
		}
		m[dest] = s
		if src != dest {
			delete(m, src)
		}
		return true
	}
	return false
}

func copyNumberIfEmpty(m map[string]any, dest string, srcs ...string) bool {
	if !missing(m, dest) {
		return false
	}
	for _, src := range srcs {
		if src == dest {
			continue
		}
		v, ok := m[src]
		if !ok || v == nil {
			continue
		}
		switch n := v.(type) {
		case float64, float32, int, int32, int64, json.Number:
			m[dest] = v
			delete(m, src)
			return true
		case string:
			s := strings.TrimSpace(n)
			if s == "" {
				continue
			}
			parsed, err := strconv.ParseFloat(s, 64)
			if err != nil {
				continue
			}
			m[dest] = parsed
			delete(m, src)
			return true
		}
	}
	return false
}

func missing(m map[string]any, key string) bool {
	v, ok := m[key]
	return !ok || v == nil
}

func missingString(m map[string]any, key string) bool {
	v, ok := m[key]
	if !ok || v == nil {
		return true
	}
	s, isStr := v.(string)
	if !isStr {
		return false
	}
	return strings.TrimSpace(s) == ""
}
