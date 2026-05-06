// Package list_files 列出工作区某相对目录下的文件与子目录条目（含可选大小与时间戳）。
package list_files

import (
	"context"
	"os"
	"strings"

	"aranea-agents/internal/tools/argmap"
	"aranea-agents/internal/tools/specs"
	"aranea-agents/internal/tools/workspace"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

type args struct {
	Path string `json:"path"`
}

const desc = `List files and directories under a path relative to the workspace root. Use "" or "." for root.`

func Run(ctx context.Context, argsMap map[string]any) (map[string]any, error) {
	_ = ctx
	p := argmap.String(argsMap, "path")
	abs, rel, err := workspace.ResolvePath(strings.TrimSpace(p))
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return nil, err
	}
	return map[string]any{"path": rel, "items": workspace.DirEntriesAsItems(entries)}, nil
}

func New() (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "list_files",
		Description: desc,
	}, func(_ tool.Context, in args) (map[string]any, error) {
		return Run(context.Background(), map[string]any{"path": in.Path})
	})
}

func OpenAIFunctionSpec() map[string]any {
	return specs.OpenAI("list_files", desc, map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{"type": "string", "description": "Directory relative to workspace; empty means root"},
		},
	})
}
