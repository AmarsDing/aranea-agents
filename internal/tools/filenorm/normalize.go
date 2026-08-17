package filenorm

import (
	"encoding/json"
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
