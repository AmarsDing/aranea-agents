package browser

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// SessionToolSet serializes browser tool calls (one Playwright tab) and
// stamps next_tool=browser_snapshot after mutating actions so the model
// refreshes refs before the next click/type.
type SessionToolSet struct {
	inner trpctool.ToolSet
	mu    sync.Mutex
}

var _ trpctool.ToolSet = (*SessionToolSet)(nil)

// WrapSession wraps inner with a process-wide browser session lock.
// Returns nil if inner is nil.
func WrapSession(inner trpctool.ToolSet) *SessionToolSet {
	if inner == nil {
		return nil
	}
	return &SessionToolSet{inner: inner}
}

func (s *SessionToolSet) Name() string {
	if s == nil || s.inner == nil {
		return ""
	}
	return s.inner.Name()
}

func (s *SessionToolSet) Close() error {
	if s == nil || s.inner == nil {
		return nil
	}
	return s.inner.Close()
}

func (s *SessionToolSet) Tools(ctx context.Context) []trpctool.Tool {
	if s == nil || s.inner == nil {
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
		out[i] = &sessionCallable{inner: ct, set: s}
	}
	return out
}

type sessionCallable struct {
	inner trpctool.CallableTool
	set   *SessionToolSet
}

var _ trpctool.CallableTool = (*sessionCallable)(nil)

func (c *sessionCallable) Declaration() *trpctool.Declaration {
	if c == nil || c.inner == nil {
		return nil
	}
	d := c.inner.Declaration()
	if d == nil {
		return nil
	}
	name := baseBrowserToolName(d.Name)
	if name != "browser_snapshot" && name != "browser_navigate" {
		return d
	}
	cp := *d
	if name == "browser_snapshot" && !strings.Contains(cp.Description, "next_tool") {
		cp.Description += " Call this after navigate/click/type so element refs stay valid. Mutating tools stamp next_tool=browser_snapshot."
	}
	if name == "browser_navigate" && !strings.Contains(cp.Description, "browser_snapshot") {
		cp.Description += " After navigation, call browser_snapshot before click/type."
	}
	return &cp
}

func (c *sessionCallable) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	if c == nil || c.inner == nil {
		return nil, nil
	}
	if c.set != nil {
		c.set.mu.Lock()
		defer c.set.mu.Unlock()
	}
	out, err := c.inner.Call(ctx, jsonArgs)
	if err != nil {
		return out, err
	}
	name := ""
	if d := c.inner.Declaration(); d != nil {
		name = d.Name
	}
	if !needsSnapshotHint(name) {
		return out, nil
	}
	return stampNextTool(out, "browser_snapshot"), nil
}

func needsSnapshotHint(name string) bool {
	base := baseBrowserToolName(name)
	switch classifyBrowserTool(name) {
	case SubGroupNavigate, SubGroupInteract:
		return true
	}
	return strings.Contains(base, "mouse") || strings.Contains(base, "drag")
}

func stampNextTool(out any, toolName string) any {
	m := resultMap(out)
	m["next_tool"] = toolName
	m["next_tool_hint"] = "Call browser_snapshot before the next interaction so refs stay valid."
	return m
}

func resultMap(v any) map[string]any {
	if v == nil {
		return map[string]any{}
	}
	if m, ok := v.(map[string]any); ok {
		cp := make(map[string]any, len(m)+2)
		for k, val := range m {
			cp[k] = val
		}
		return cp
	}
	if s, ok := v.(string); ok {
		return map[string]any{"output": s}
	}
	b, err := json.Marshal(v)
	if err != nil {
		return map[string]any{"output": v}
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil || m == nil {
		return map[string]any{"output": v}
	}
	return m
}
