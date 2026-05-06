// Package list_files 列出工作区某相对目录下的文件与子目录条目（含可选大小与时间戳）。
package list_files

import (
	"context"
	"os"
	"strings"

	"aranea-agents/internal/tools/toolapi"
	"aranea-agents/internal/tools/workspace"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

type args struct {
	Path string `json:"path"`
}

// impl 列出工作区某相对目录下的文件与子目录条目（可用于浏览工程结构）。
type impl struct{}

func New() toolapi.Tool {
	return &impl{}
}

func (*impl) Meta() toolapi.Meta {
	return toolapi.Meta{
		Name:        "list_files",
		TitleZh:     "列目录",
		SummaryZh:   "列出工作区内某相对路径下的文件名、是否目录以及可选的大小与修改时间。",
		Description: `List files and directories under a path relative to the workspace root. Use "" or "." for root.`,
	}
}

func (*impl) SupportsLocalInvoke() bool { return true }

func (*impl) InvokeLocal(ctx context.Context, argsMap map[string]any) (map[string]any, error) {
	_ = ctx
	p := toolapi.ArgString(argsMap, "path")
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

func (*impl) OpenAIFunction() map[string]any {
	desc := (&impl{}).Meta().Description
	return toolapi.BuildOpenAISpec("list_files", desc,
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{"type": "string", "description": "Directory relative to workspace; empty means root"},
			},
		})
}

func (*impl) ADKTool() (tool.Tool, error) {
	desc := (&impl{}).Meta().Description
	return functiontool.New(functiontool.Config{
		Name:        "list_files",
		Description: desc,
	}, func(_ tool.Context, in args) (map[string]any, error) {
		return (&impl{}).InvokeLocal(context.Background(), map[string]any{"path": in.Path})
	})
}
