// Package read_file 在工作区内按相对路径读取 UTF-8 文本文件；若路径为目录则返回该目录条目列表。
package read_file

import (
	"context"
	"fmt"
	"os"
	"strings"

	"aranea-agents/internal/tools/argmap"
	"aranea-agents/internal/tools/specs"
	"aranea-agents/internal/tools/workspace"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

const maxBytes = 1024 * 1024

type args struct {
	Path string `json:"path"`
}

const desc = `Read a UTF-8 text file under the workspace root. Argument path is relative to the workspace (e.g. "internal/foo.go").`

// Run executes the read side-effect (also used by legacy OpenAI tool loop).
func Run(ctx context.Context, argsMap map[string]any) (map[string]any, error) {
	_ = ctx
	path := argmap.String(argsMap, "path")
	abs, rel, err := workspace.ResolvePath(strings.TrimSpace(path))
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		entries, err := os.ReadDir(abs)
		if err != nil {
			return nil, err
		}
		return map[string]any{"path": rel, "items": workspace.DirEntriesAsItems(entries)}, nil
	}
	if info.Size() > int64(maxBytes) {
		return nil, fmt.Errorf("file %q is too large (%d bytes, max %d)", rel, info.Size(), maxBytes)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, err
	}
	return map[string]any{"path": rel, "content": string(data), "size": len(data)}, nil
}

// New returns an ADK function tool.
func New() (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "read_file",
		Description: desc,
	}, func(_ tool.Context, in args) (map[string]any, error) {
		return Run(context.Background(), map[string]any{"path": in.Path})
	})
}

// OpenAIFunctionSpec is used by the legacy /chat/completions native tool loop until full Runner migration.
func OpenAIFunctionSpec() map[string]any {
	return specs.OpenAI("read_file", desc, map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{"type": "string", "description": "Relative path under workspace root"},
		},
		"required": []string{"path"},
	})
}
