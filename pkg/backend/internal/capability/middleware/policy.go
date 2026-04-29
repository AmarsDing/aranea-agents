package middleware

import (
	"encoding/json"
	"strings"
)

// JSONStringList parses a JSON string array from agent settings JSON columns.
func JSONStringList(raw string) []string {
	var items []string
	if strings.TrimSpace(raw) == "" || json.Unmarshal([]byte(raw), &items) != nil {
		return nil
	}
	return items
}

// ToolSet expands group tokens and single tool keys into a lookup set for allow/deny.
func ToolSet(items []string) map[string]bool {
	out := map[string]bool{}
	for _, item := range items {
		item = strings.TrimSpace(item)
		switch item {
		case "group:filesystem":
			for _, key := range []string{"read_file", "write_file", "list_files", "edit_file"} {
				out[key] = true
			}
		case "group:web":
			out["web_search"] = true
			out["web_fetch"] = true
		case "edit":
			out["edit_file"] = true
		case "browser":
			out["web_fetch"] = true
		case "":
		default:
			out[item] = true
		}
	}
	return out
}

// ProfileAllows mirrors service tool profiles for the runtime exposure path so policy
// previews and actual ADK tool surfaces stay aligned.
func ProfileAllows(profile string, name string) bool {
	switch strings.ToLower(strings.TrimSpace(profile)) {
	case "chat_only", "minimal":
		return false
	case "read_only", "safe":
		return name == "datetime" || name == "read_file" || name == "list_files"
	case "coding":
		return name == "datetime" || name == "read_file" || name == "write_file" || name == "list_files" || name == "edit_file" || name == "web_fetch"
	case "research":
		return name == "datetime" || name == "read_file" || name == "list_files" || name == "web_fetch" || name == "web_search"
	case "system_admin", "full":
		return true
	default:
		return false
	}
}
