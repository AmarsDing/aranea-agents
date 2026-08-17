package argnorm

import (
	"context"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// WrapTool normalizes call arguments for a standalone tool that uses
// known alias schemas. Other tools are returned unchanged so Streamable
// capability is not stripped.
func WrapTool(t trpctool.Tool) trpctool.Tool {
	if t == nil {
		return nil
	}
	name := ""
	if decl := t.Declaration(); decl != nil {
		name = decl.Name
	}
	if !needsNorm(name) {
		return t
	}
	ct, ok := t.(trpctool.CallableTool)
	if !ok {
		return t
	}
	w := &wrapCallable{inner: ct}
	if st, ok := t.(trpctool.StreamableTool); ok {
		return &wrapStreamable{wrapCallable: w, stream: st}
	}
	return w
}

type wrapToolSet struct {
	inner trpctool.ToolSet
}

// WrapToolSet normalizes arguments for every CallableTool in a ToolSet
// whose declaration name has known aliases.
func WrapToolSet(ts trpctool.ToolSet) trpctool.ToolSet {
	if ts == nil {
		return nil
	}
	return &wrapToolSet{inner: ts}
}

func (w *wrapToolSet) Name() string {
	if w.inner == nil {
		return ""
	}
	return w.inner.Name()
}

func (w *wrapToolSet) Close() error {
	if w.inner == nil {
		return nil
	}
	return w.inner.Close()
}

func (w *wrapToolSet) Tools(ctx context.Context) []trpctool.Tool {
	if w.inner == nil {
		return nil
	}
	raw := w.inner.Tools(ctx)
	if len(raw) == 0 {
		return raw
	}
	out := make([]trpctool.Tool, len(raw))
	for i, t := range raw {
		out[i] = WrapTool(t)
	}
	return out
}

type wrapCallable struct {
	inner trpctool.CallableTool
}

func (w *wrapCallable) Declaration() *trpctool.Declaration {
	if w == nil || w.inner == nil {
		return nil
	}
	return w.inner.Declaration()
}

func (w *wrapCallable) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	name := ""
	if decl := w.Declaration(); decl != nil {
		name = decl.Name
	}
	return w.inner.Call(ctx, NormalizeArgs(name, jsonArgs))
}

type wrapStreamable struct {
	*wrapCallable
	stream trpctool.StreamableTool
}

func (w *wrapStreamable) StreamableCall(ctx context.Context, jsonArgs []byte) (*trpctool.StreamReader, error) {
	name := ""
	if decl := w.Declaration(); decl != nil {
		name = decl.Name
	}
	return w.stream.StreamableCall(ctx, NormalizeArgs(name, jsonArgs))
}

var (
	_ trpctool.CallableTool   = (*wrapCallable)(nil)
	_ trpctool.StreamableTool = (*wrapStreamable)(nil)
)
