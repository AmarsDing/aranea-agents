package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Native tool keys wired for OpenAI function calling in native admin chat.
const nativeFSMaxReadBytes = 1024 * 1024

var nativeFilesystemToolOrder = []string{"read_file", "list_files", "write_file", "edit_file"}

func nativeToolDefinitionsForKeys(enabled map[string]bool) []map[string]any {
	var out []map[string]any
	for _, key := range nativeFilesystemToolOrder {
		if !enabled[key] {
			continue
		}
		switch key {
		case "read_file":
			out = append(out, openAITool("read_file",
				"Read a UTF-8 text file under the workspace root. Argument path is relative to the workspace (e.g. \"internal/foo.go\").",
				map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path": map[string]any{"type": "string", "description": "Relative path under workspace root"},
					},
					"required": []string{"path"},
				}))
		case "list_files":
			out = append(out, openAITool("list_files",
				"List files and directories under a path relative to the workspace root. Use \"\" or \".\" for root.",
				map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path": map[string]any{"type": "string", "description": "Directory relative to workspace; empty means root"},
					},
				}))
		case "write_file":
			out = append(out, openAITool("write_file",
				"Create or overwrite a UTF-8 file under the workspace. Parent directories are created as needed.",
				map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path":    map[string]any{"type": "string", "description": "Relative path under workspace root"},
						"content": map[string]any{"type": "string", "description": "Full file contents"},
					},
					"required": []string{"path", "content"},
				}))
		case "edit_file":
			out = append(out, openAITool("edit_file",
				"Replace exactly one occurrence of old_string with new_string in an existing file. Prefer read_file first.",
				map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path":       map[string]any{"type": "string"},
						"old_string": map[string]any{"type": "string"},
						"new_string": map[string]any{"type": "string"},
					},
					"required": []string{"path", "old_string", "new_string"},
				}))
		}
	}
	return out
}

func openAITool(name, description string, parameters map[string]any) map[string]any {
	return map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        name,
			"description": description,
			"parameters":  parameters,
		},
	}
}

func toolArgString(params map[string]any, key string) string {
	if params == nil {
		return ""
	}
	v, ok := params[key]
	if !ok || v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

func executeNativeFilesystemTool(name string, argsJSON string) (map[string]any, error) {
	var params map[string]any
	if strings.TrimSpace(argsJSON) != "" {
		if err := json.Unmarshal([]byte(argsJSON), &params); err != nil {
			return nil, fmt.Errorf("invalid JSON arguments for %s: %w", name, err)
		}
	}
	switch name {
	case "read_file":
		return execReadFile(params)
	case "list_files":
		return execListFiles(params)
	case "write_file":
		return execWriteFile(params)
	case "edit_file":
		return execEditFile(params)
	default:
		return nil, fmt.Errorf("unknown native tool %q", name)
	}
}

func execReadFile(params map[string]any) (map[string]any, error) {
	path, rel, err := ResolveWorkspacePath(toolArgString(params, "path"))
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return execListFiles(map[string]any{"path": rel})
	}
	if info.Size() > int64(nativeFSMaxReadBytes) {
		return nil, fmt.Errorf("file %q is too large (%d bytes, max %d)", rel, info.Size(), nativeFSMaxReadBytes)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return map[string]any{"path": rel, "content": string(data), "size": len(data)}, nil
}

func execListFiles(params map[string]any) (map[string]any, error) {
	p := toolArgString(params, "path")
	path, rel, err := ResolveWorkspacePath(p)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	items := dirEntriesToItems(entries)
	return map[string]any{"path": rel, "items": items}, nil
}

func dirEntriesToItems(entries []os.DirEntry) []map[string]any {
	items := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		item := map[string]any{"name": entry.Name(), "isDir": entry.IsDir()}
		if info, statErr := entry.Info(); statErr == nil {
			item["size"] = info.Size()
			item["modTime"] = info.ModTime().Format("2006-01-02T15:04:05Z07:00")
		}
		items = append(items, item)
	}
	return items
}

func execWriteFile(params map[string]any) (map[string]any, error) {
	path, rel, err := ResolveWorkspacePath(toolArgString(params, "path"))
	if err != nil {
		return nil, err
	}
	content := fmt.Sprint(params["content"])
	if err = os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	if err = os.WriteFile(path, []byte(content), 0o644); err != nil {
		return nil, err
	}
	return map[string]any{"path": rel, "written": len(content)}, nil
}

func execEditFile(params map[string]any) (map[string]any, error) {
	oldString := fmt.Sprint(params["old_string"])
	if oldString == "" {
		return nil, fmt.Errorf("old_string is required")
	}
	path, rel, err := ResolveWorkspacePath(toolArgString(params, "path"))
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	content := string(data)
	count := strings.Count(content, oldString)
	if count == 0 {
		return nil, fmt.Errorf("old_string was not found in %q", rel)
	}
	if count > 1 {
		return nil, fmt.Errorf("old_string matched %d times in %q; provide more context", count, rel)
	}
	next := strings.Replace(content, oldString, fmt.Sprint(params["new_string"]), 1)
	if err = os.WriteFile(path, []byte(next), 0o644); err != nil {
		return nil, err
	}
	return map[string]any{"path": rel, "replacements": 1}, nil
}
