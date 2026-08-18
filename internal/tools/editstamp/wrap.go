package editstamp

import (
	"context"
	"encoding/json"
	"strings"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

var mutating = map[string]bool{
	"save_file":       true,
	"diff_edit":       true,
	"patch_file":      true,
	"replace_content": true,
	"delete_file":     true,
}

type toolSetWrap struct {
	inner   trpctool.ToolSet
	baseDir string
}

// WrapToolSet records successful file mutations into the recent-edit stamp
// so read_lints can lint those paths when the model omits path/paths.
func WrapToolSet(inner trpctool.ToolSet, baseDir string) trpctool.ToolSet {
	if inner == nil {
		return nil
	}
	return &toolSetWrap{inner: inner, baseDir: baseDir}
}

func (s *toolSetWrap) Name() string {
	if s.inner == nil {
		return ""
	}
	return s.inner.Name()
}

func (s *toolSetWrap) Close() error {
	if s.inner == nil {
		return nil
	}
	return s.inner.Close()
}

func (s *toolSetWrap) Tools(ctx context.Context) []trpctool.Tool {
	if s.inner == nil {
		return nil
	}
	raw := s.inner.Tools(ctx)
	out := make([]trpctool.Tool, len(raw))
	for i, t := range raw {
		ct, ok := t.(trpctool.CallableTool)
		if !ok {
			out[i] = t
			continue
		}
		name := ""
		if d := ct.Declaration(); d != nil {
			name = d.Name
		}
		if !mutating[name] {
			out[i] = t
			continue
		}
		out[i] = &stampTool{inner: ct, baseDir: s.baseDir}
	}
	return out
}

type stampTool struct {
	inner   trpctool.CallableTool
	baseDir string
}

func (t *stampTool) Declaration() *trpctool.Declaration {
	if t.inner == nil {
		return nil
	}
	return t.inner.Declaration()
}

func (t *stampTool) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	if t.inner == nil {
		return nil, nil
	}
	out, err := t.inner.Call(ctx, jsonArgs)
	if err == nil {
		if p := fileNameFromArgs(jsonArgs); p != "" {
			Record(t.baseDir, p)
		}
	}
	return out, err
}

func fileNameFromArgs(jsonArgs []byte) string {
	var m map[string]any
	if err := json.Unmarshal(jsonArgs, &m); err != nil {
		return ""
	}
	for _, k := range []string{"file_name", "path", "file", "filename"} {
		s, _ := m[k].(string)
		s = strings.TrimSpace(s)
		if s != "" {
			return s
		}
	}
	return ""
}
