package filenorm

import (
	"context"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

type wrapToolSet struct {
	inner trpctool.ToolSet
}

// WrapToolSet normalizes file-tool arguments before delegating to the file ToolSet.
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
		out[i] = wrapCallableTool(t)
	}
	return out
}

func wrapCallableTool(t trpctool.Tool) trpctool.Tool {
	if t == nil {
		return nil
	}
	ct, ok := t.(trpctool.CallableTool)
	if !ok {
		return t
	}
	return &wrapCallable{inner: ct}
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
	return w.inner.Call(ctx, NormalizeFileArgs(name, jsonArgs))
}
