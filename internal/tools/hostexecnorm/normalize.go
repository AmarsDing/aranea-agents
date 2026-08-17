package hostexecnorm

import (
	"encoding/json"
	"strconv"
	"strings"
)

// NormalizeExecArgs maps common LLM/catalog aliases onto the hostexec schema
// so exec_command does not fail with "command required" or ignore cwd.
//
//	command     ← cmd, cmd_line, cmdline; argv arrays and optional args/argv
//	workdir     ← working_dir, cwd, dir, directory, working_directory
//	timeout_sec ← timeout, timeout_seconds
func NormalizeExecArgs(jsonArgs []byte) []byte {
	var m map[string]any
	if err := json.Unmarshal(jsonArgs, &m); err != nil || len(m) == 0 {
		return jsonArgs
	}
	changed := false
	if coerceCommand(m) {
		changed = true
	}
	if copyStringIfEmpty(m, "command", "cmd", "cmd_line", "cmdline") {
		changed = true
	}
	if copyStringIfEmpty(m, "workdir", "working_dir", "cwd", "dir", "directory", "working_directory") {
		changed = true
	}
	if copyNumberIfEmpty(m, "timeout_sec", "timeout", "timeout_seconds") {
		changed = true
	}
	if copyNumberIfEmpty(m, "yield_time_ms", "block_until_ms", "yieldMs") {
		changed = true
	}
	if copyStringIfEmpty(m, "notify_pattern", "notify_on_output") {
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

// coerceCommand turns argv-style payloads into the hostexec string schema.
// Models often send `"command": ["ls","-la"]` or `"command":"git","args":["status"]`,
// which json.Unmarshal cannot store in `Command string`.
func coerceCommand(m map[string]any) bool {
	prefix := flattenCommandValue(m["command"])
	fromAlias := false
	if prefix == "" {
		for _, src := range []string{"cmd", "cmd_line", "cmdline"} {
			if s := flattenCommandValue(m[src]); s != "" {
				prefix = s
				fromAlias = true
				delete(m, src)
				break
			}
		}
	}
	extra := flattenCommandValue(m["args"])
	if extra == "" {
		extra = flattenCommandValue(m["argv"])
	}
	if extra != "" {
		delete(m, "args")
		delete(m, "argv")
	}
	joined := prefix
	if extra != "" {
		if joined != "" {
			joined = joined + " " + extra
		} else {
			joined = extra
		}
	}
	if joined == "" {
		return false
	}
	if orig, ok := m["command"].(string); ok && orig == joined && !fromAlias && extra == "" {
		return false
	}
	m["command"] = joined
	return true
}

func flattenCommandValue(v any) string {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case []any:
		parts := make([]string, 0, len(x))
		for _, item := range x {
			s := stringifyCmdPart(item)
			if s == "" {
				continue
			}
			parts = append(parts, quoteCmdPart(s))
		}
		return strings.Join(parts, " ")
	case []string:
		parts := make([]string, 0, len(x))
		for _, item := range x {
			s := strings.TrimSpace(item)
			if s == "" {
				continue
			}
			parts = append(parts, quoteCmdPart(s))
		}
		return strings.Join(parts, " ")
	default:
		return ""
	}
}

func stringifyCmdPart(v any) string {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case float64:
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	case json.Number:
		return x.String()
	case bool:
		return strconv.FormatBool(x)
	default:
		return ""
	}
}

func quoteCmdPart(s string) string {
	if !strings.ContainsAny(s, " \t\"'") {
		return s
	}
	return strconv.Quote(s)
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
