// Package read_file 在工作区内按相对路径读取 UTF-8 文本文件；若路径为目录则返回该目录条目列表。
package read_file

import (
	"context"
	"fmt"
	"os"
	"strings"

	"aranea-agents/internal/tools/toolapi"
	"aranea-agents/internal/tools/workspace"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

const maxBytes = 1024 * 1024

type args struct {
	Path string `json:"path"`
}

// impl 实现在工作区内按相对路径读取 UTF-8 文件；若为目录则回退到「列出该目录」（与原先行为一致）。
type impl struct{}

// New 返回统一 registry 可用的 Tool。
func New() toolapi.Tool {
	return &impl{}
}

func (*impl) Meta() toolapi.Meta {
	return toolapi.Meta{
		Name:        "read_file",
		TitleZh:     "读取文件",
		SummaryZh:   "读取工作区内指定相对路径的文件内容（UTF-8）；若目标是目录则返回该目录条目列表而非报错。",
		Description: `Read a UTF-8 text file under the workspace root. Argument path is relative to the workspace (e.g. "internal/foo.go").`,
	}
}

func (*impl) SupportsLocalInvoke() bool { return true }

func (*impl) InvokeLocal(ctx context.Context, argsMap map[string]any) (map[string]any, error) {
	_ = ctx
	path := toolapi.ArgString(argsMap, "path")
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

func (*impl) OpenAIFunction() map[string]any {
	return toolapi.BuildOpenAISpec("read_file",
		new(impl).Meta().Description,
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{"type": "string", "description": "Relative path under workspace root"},
			},
			"required": []string{"path"},
		})
}

func (*impl) ADKTool() (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "read_file",
		Description: (&impl{}).Meta().Description,
	}, func(_ tool.Context, in args) (map[string]any, error) {
		return (&impl{}).InvokeLocal(context.Background(), map[string]any{"path": in.Path})
	})
}
