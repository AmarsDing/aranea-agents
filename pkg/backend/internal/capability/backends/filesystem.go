package backends

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"arenea/backend/internal/capability/schema"
	"arenea/backend/internal/capability/toolctx"
)

const FileToolMaxReadBytes = 1024 * 1024

const (
	readFileToolDescription = "Read raw bytes of a UTF-8 text file located inside the workspace source tree. " +
		"Argument: path (string) relative to the project root, e.g. \"backend/internal/domain/models.go\". " +
		"Use only when the user explicitly asks to read or inspect a source file. " +
		"DO NOT use to answer questions about teams, members, sessions, agents, providers, models, dialog mode or any in-app metadata - that information is supplied in the Runtime Context block, not on disk."
	listFilesToolDescription = "List files and directories under a workspace path. " +
		"Argument: path (string), empty or \".\" lists the project root. " +
		"Use only for source-tree exploration when the user asked about files or folders. " +
		"DO NOT use to count team members, sessions, agents or any in-app entity; those counts are present in the Runtime Context block."
	writeFileToolDescription = "Create or overwrite a UTF-8 file inside the workspace sandbox. " +
		"Arguments: path (string), content (string). Parent directories are created automatically. " +
		"Use only when the user explicitly asked to save, write or generate a file. " +
		"DO NOT use to take notes about the conversation, persist memory or store agent state."
	editFileToolDescription = "Replace exactly one text occurrence in an existing workspace file. " +
		"Arguments: path (string), old_string (string), new_string (string). Fails if old_string is missing or ambiguous. " +
		"Use only after read_file confirmed the exact target text. " +
		"DO NOT call speculatively - if you don't already know the precise old_string, refuse and ask the user."
)

type ReadFileTool struct{ Base }
type ListFilesTool struct{ Base }
type WriteFileTool struct{ Base }
type EditFileTool struct{ Base }

func NewReadFileTool() *ReadFileTool {
	return &ReadFileTool{Base: Base{Key: "read_file", Label: "读取文件", Desc: readFileToolDescription, ToolCategory: "filesystem", Required: []string{"path"}, InSchema: schema.JSONSchemaOf[schema.FilePathInput](), OutSchema: schema.JSONSchemaOf[schema.ReadFileOutput]()}}
}

func NewListFilesTool() *ListFilesTool {
	return &ListFilesTool{Base: Base{Key: "list_files", Label: "文件列表", Desc: listFilesToolDescription, ToolCategory: "filesystem", InSchema: schema.JSONSchemaOf[schema.FilePathInput](), OutSchema: schema.JSONSchemaOf[schema.FileListOutput]()}}
}

func NewWriteFileTool() *WriteFileTool {
	return &WriteFileTool{Base: Base{Key: "write_file", Label: "写入文件", Desc: writeFileToolDescription, ToolCategory: "filesystem", Required: []string{"path", "content"}, InSchema: schema.JSONSchemaOf[schema.WriteFileInput](), OutSchema: schema.JSONSchemaOf[schema.WriteFileOutput]()}}
}

func NewEditFileTool() *EditFileTool {
	return &EditFileTool{Base: Base{Key: "edit_file", Label: "编辑文件", Desc: editFileToolDescription, ToolCategory: "filesystem", Required: []string{"path", "old_string", "new_string"}, InSchema: schema.JSONSchemaOf[schema.EditFileInput](), OutSchema: schema.JSONSchemaOf[schema.EditFileOutput]()}}
}

func (t *ReadFileTool) Execute(_ *toolctx.ToolContext, params map[string]any) (map[string]any, error) {
	path, rel, err := ResolveWorkspacePath(stringParam(params, "path"))
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return NewListFilesTool().Execute(nil, map[string]any{"path": rel})
	}
	if info.Size() > FileToolMaxReadBytes {
		return nil, fmt.Errorf("file %q is too large (%d bytes, max %d)", rel, info.Size(), FileToolMaxReadBytes)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return map[string]any{"path": rel, "content": string(data), "size": len(data)}, nil
}

func (t *ListFilesTool) Execute(_ *toolctx.ToolContext, params map[string]any) (map[string]any, error) {
	path, rel, err := ResolveWorkspacePath(stringParam(params, "path"))
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		item := map[string]any{"name": entry.Name(), "isDir": entry.IsDir()}
		if info, statErr := entry.Info(); statErr == nil {
			item["size"] = info.Size()
			item["modTime"] = info.ModTime().Format("2006-01-02T15:04:05Z07:00")
		}
		items = append(items, item)
	}
	return map[string]any{"path": rel, "items": items}, nil
}

func (t *WriteFileTool) Execute(_ *toolctx.ToolContext, params map[string]any) (map[string]any, error) {
	path, rel, err := ResolveWorkspacePath(stringParam(params, "path"))
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

func (t *EditFileTool) Execute(_ *toolctx.ToolContext, params map[string]any) (map[string]any, error) {
	oldString := fmt.Sprint(params["old_string"])
	if oldString == "" {
		return nil, fmt.Errorf("old_string is required")
	}
	path, rel, err := ResolveWorkspacePath(stringParam(params, "path"))
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

func ResolveWorkspacePath(rawPath string) (absPath string, relPath string, err error) {
	root, err := WorkspaceRoot()
	if err != nil {
		return "", "", err
	}
	input := strings.TrimSpace(rawPath)
	if input == "" || input == "." {
		input = "."
	}
	input = filepath.FromSlash(input)
	if !filepath.IsAbs(input) && filepath.Base(root) == "aranea" {
		parts := strings.Split(filepath.Clean(input), string(filepath.Separator))
		if len(parts) > 1 && strings.EqualFold(parts[0], "aranea") {
			input = filepath.Join(parts[1:]...)
		}
	}
	candidate := input
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(root, candidate)
	}
	candidate, err = filepath.Abs(filepath.Clean(candidate))
	if err != nil {
		return "", "", err
	}
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return "", "", err
	}
	if rel == "." {
		return candidate, ".", nil
	}
	if strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." || filepath.IsAbs(rel) {
		return "", "", fmt.Errorf("path %q is outside workspace sandbox", rawPath)
	}
	return candidate, filepath.ToSlash(rel), nil
}

func WorkspaceRoot() (string, error) {
	for _, key := range []string{"ARANEA_WORKSPACE_ROOT", "WORKSPACE_ROOT"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return filepath.Abs(filepath.Clean(value))
		}
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	wd, err = filepath.Abs(wd)
	if err != nil {
		return "", err
	}
	for dir := wd; ; dir = filepath.Dir(dir) {
		if hasDir(filepath.Join(dir, "backend")) && hasDir(filepath.Join(dir, "frontend")) {
			return dir, nil
		}
		aranea := filepath.Join(dir, "aranea")
		if hasDir(filepath.Join(aranea, "backend")) && hasDir(filepath.Join(aranea, "frontend")) {
			return aranea, nil
		}
		next := filepath.Dir(dir)
		if next == dir {
			break
		}
	}
	return wd, nil
}

func hasDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
