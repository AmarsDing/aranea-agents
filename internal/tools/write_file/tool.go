// Package write_file 在工作区内创建或覆盖 UTF-8 文本文件并按需创建父目录。
package write_file

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"aranea-agents/internal/tools/toolapi"
	"aranea-agents/internal/tools/workspace"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

type wfArgs struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// impl 在工作区内创建或覆盖 UTF-8 文件，并自动创建缺失的父目录。
type impl struct{}

func New() toolapi.Tool {
	return &impl{}
}

func (*impl) Meta() toolapi.Meta {
	return toolapi.Meta{
		Name:        "write_file",
		TitleZh:     "写入文件",
		SummaryZh:   "写入或覆盖工作区内的文本文件内容，按需创建多级父目录（危险操作需谨慎授权）。",
		Description: `Create or overwrite a UTF-8 file under the workspace. Parent directories are created as needed.`,
	}
}

func (*impl) SupportsLocalInvoke() bool { return true }

func (*impl) InvokeLocal(ctx context.Context, argsMap map[string]any) (map[string]any, error) {
	_ = ctx
	path := toolapi.ArgString(argsMap, "path")
	var content string
	if v, ok := argsMap["content"]; ok && v != nil {
		content = fmt.Sprint(v)
	}
	abs, rel, err := workspace.ResolvePath(strings.TrimSpace(path))
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		return nil, err
	}
	return map[string]any{"path": rel, "written": len(content)}, nil
}

func (*impl) OpenAIFunction() map[string]any {
	desc := (&impl{}).Meta().Description
	return toolapi.BuildOpenAISpec("write_file", desc,
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":    map[string]any{"type": "string", "description": "Relative path under workspace root"},
				"content": map[string]any{"type": "string", "description": "Full file contents"},
			},
			"required": []string{"path", "content"},
		})
}

func (*impl) ADKTool() (tool.Tool, error) {
	desc := (&impl{}).Meta().Description
	return functiontool.New(functiontool.Config{
		Name:        "write_file",
		Description: desc,
	}, func(_ tool.Context, in wfArgs) (map[string]any, error) {
		return (&impl{}).InvokeLocal(context.Background(),
			map[string]any{"path": in.Path, "content": in.Content})
	})
}
