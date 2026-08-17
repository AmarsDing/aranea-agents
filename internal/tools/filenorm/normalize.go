package filenorm

import (
	"encoding/json"
	"strconv"
	"strings"
)

// NormalizeFileArgs maps common LLM aliases onto the file ToolSet schema.
// Models often send path/content instead of file_name/contents, which would
// otherwise make read_file/save_file fail as if the path were missing.
func NormalizeFileArgs(toolName string, jsonArgs []byte) []byte {
	name := strings.TrimSpace(toolName)
	if name == "" || len(jsonArgs) == 0 {
		return jsonArgs
	}
	var m map[string]any
	if err := json.Unmarshal(jsonArgs, &m); err != nil || len(m) == 0 {
		return jsonArgs
	}
	changed := false
	switch name {
	case "read_file", "save_file", "replace_content", "diff_edit", "patch_file":
		if copyStringIfEmpty(m, "file_name", "path", "file", "filename", "filepath", "file_path") {
			changed = true
		}
		if name == "save_file" {
			if copyStringIfEmpty(m, "contents", "content", "body", "text") {
				changed = true
			}
		}
		if name == "read_file" {
			if copyNumberIfEmpty(m, "start_line", "start", "offset", "from_line") {
				changed = true
			}
			if copyNumberIfEmpty(m, "num_lines", "limit", "count", "n", "max_lines") {
				changed = true
			}
		}
		if name == "replace_content" {
			if copyStringIfEmpty(m, "old_string", "old", "from") {
				changed = true
			}
			if copyStringIfEmpty(m, "new_string", "new", "to") {
				changed = true
			}
		}
	case "list_file":
		if copyStringIfEmpty(m, "path", "dir", "directory", "folder", "file_name") {
			changed = true
		}
	case "search_file":
		if copyStringIfEmpty(m, "path", "dir", "directory") {
			changed = true
		}
		if copyStringIfEmpty(m, "pattern", "glob") {
			changed = true
		}
	case "search_content":
		if copyStringIfEmpty(m, "path", "dir", "directory", "folder") {
			changed = true
		}
		if copyStringIfEmpty(m, "file_pattern", "glob", "file_glob", "include") {
			changed = true
		}
		if copyStringIfEmpty(m, "content_pattern", "query", "search", "text", "content", "keyword", "regex", "pattern") {
			changed = true
		}
	default:
		return jsonArgs
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
