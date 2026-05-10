// Package read_file 在工作区内按相对路径读取 UTF-8 文本文件；若路径为目录则返回该目录条目列表。
package read_file

import (
	"bytes"
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
	Path      string `json:"path"`
	StartLine int    `json:"start_line,omitempty"`
	EndLine   int    `json:"end_line,omitempty"`
}

const desc = `Read a UTF-8 text file under the workspace root. Argument path is relative to the workspace (e.g. "internal/foo.go"). Optional start_line and end_line (1-based inclusive lines) return only that slice of the file; omit both for full file (still subject to size cap).`

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
	startLine := intFromArg(argsMap, "start_line", 0)
	endLine := intFromArg(argsMap, "end_line", 0)
	content := string(data)
	if startLine > 0 || endLine > 0 {
		if startLine < 0 || endLine < 0 {
			return nil, fmt.Errorf("start_line and end_line must be non-negative")
		}
		if endLine > 0 && startLine > 0 && endLine < startLine {
			return nil, fmt.Errorf("end_line must be >= start_line")
		}
		lines := splitLines(data)
		n := len(lines)
		s := startLine
		if s <= 0 {
			s = 1
		}
		e := endLine
		if e <= 0 || e > n {
			e = n
		}
		if s > n {
			return map[string]any{
				"path": rel, "content": "", "size": 0,
				"start_line": s, "end_line": e, "line_count": n, "empty_range": true,
			}, nil
		}
		if e < s {
			e = s
		}
		slice := lines[s-1 : e]
		content = strings.Join(slice, "\n")
		out := map[string]any{"path": rel, "content": content, "size": len(content), "start_line": s, "end_line": e, "line_count": n}
		return out, nil
	}
	return map[string]any{"path": rel, "content": content, "size": len(data)}, nil
}

func splitLines(data []byte) []string {
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	s := string(data)
	if !strings.Contains(s, "\n") {
		if s == "" {
			return []string{""}
		}
		return []string{s}
	}
	parts := strings.Split(s, "\n")
	// Split removes trailing empty only if ends with newline - normalize so we always have line-per-row for 1-based indexing
	if len(parts) > 0 && parts[len(parts)-1] == "" && strings.HasSuffix(s, "\n") {
		parts = parts[:len(parts)-1]
	}
	return parts
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

// New returns an ADK function tool.
func New() (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "read_file",
		Description: desc,
	}, func(_ tool.Context, in args) (map[string]any, error) {
		m := map[string]any{"path": in.Path}
		if in.StartLine != 0 {
			m["start_line"] = in.StartLine
		}
		if in.EndLine != 0 {
			m["end_line"] = in.EndLine
		}
		return Run(context.Background(), m)
	})
}

// OpenAIFunctionSpec is used by the legacy /chat/completions native tool loop until full Runner migration.
func OpenAIFunctionSpec() map[string]any {
	return specs.OpenAI("read_file", desc, map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{"type": "string", "description": "Relative path under workspace root"},
			"start_line": map[string]any{
				"type":        "number",
				"description": "Optional 1-based start line (inclusive) for a line slice; use with end_line",
			},
			"end_line": map[string]any{
				"type":        "number",
				"description": "Optional 1-based end line (inclusive); omit or 0 means through last line when start_line is set",
			},
		},
		"required": []string{"path"},
	})
}
