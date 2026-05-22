package hostexecnorm

import (
	"context"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

type wrapToolSet struct {
	inner trpctool.ToolSet
}

// WrapToolSet normalizes exec_command call arguments before delegating to hostexec.
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
	if ct, ok := t.(trpctool.CallableTool); ok {
		return &wrapCallable{inner: ct}
	}
	return t
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
	return w.inner.Call(ctx, NormalizeExecArgs(jsonArgs))
}
