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
	Path       string `json:"path"`
	MaxEntries int    `json:"max_entries,omitempty"`
}

const desc = `List files and directories under a path relative to the workspace root. Use "" or "." for the workspace root. Do not call again with the same path in one task once you have a result—use read_file or workspace_search to inspect files or proceed with analysis. Optional max_entries caps the listing (items may be truncated).`

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
	items := workspace.DirEntriesAsItems(entries)
	out := map[string]any{"path": rel, "items": items}
	maxEntries := intFromArg(argsMap, "max_entries", 0)
	if maxEntries > 0 && len(items) > maxEntries {
		out["items"] = items[:maxEntries]
		out["truncated"] = true
	}
	return out, nil
}

func intFromArg(m map[string]any, key string, def int) int {
	if m == nil {
		return def
	}
	v, ok := m[key]
	if !ok || v == nil {
		return def
	}
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case int64:
		return int(t)
	default:
		return def
	}
}

func New() (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "list_files",
		Description: desc,
	}, func(_ tool.Context, in args) (map[string]any, error) {
		m := map[string]any{"path": in.Path}
		if in.MaxEntries != 0 {
			m["max_entries"] = in.MaxEntries
		}
		return Run(context.Background(), m)
	})
}

func OpenAIFunctionSpec() map[string]any {
	return specs.OpenAI("list_files", desc, map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{"type": "string", "description": "Directory relative to workspace; empty means root"},
			"max_entries": map[string]any{
				"type":        "number",
				"description": "Optional max number of directory entries to return; if exceeded, items are truncated and truncated=true is set",
			},
		},
	})
}
