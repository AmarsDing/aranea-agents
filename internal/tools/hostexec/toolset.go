package hostexec

import (
	"context"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	trpchostexec "trpc.group/trpc-go/trpc-agent-go/tool/hostexec"

	"aranea-agents/internal/tools/hostexecnorm"
)

type redactingToolSet struct {
	inner trpctool.ToolSet
	env   map[string]string
}

func NewRedactingToolSet(ts trpctool.ToolSet, env map[string]string) trpctool.ToolSet {
	if ts == nil {
		return nil
	}
	return &redactingToolSet{inner: ts, env: env}
}

func (r *redactingToolSet) Name() string {
	if r.inner == nil {
		return ""
	}
	return r.inner.Name()
}

func (r *redactingToolSet) Close() error {
	if r.inner == nil {
		return nil
	}
	return r.inner.Close()
}

func (r *redactingToolSet) Tools(ctx context.Context) []trpctool.Tool {
	if r.inner == nil {
		return nil
	}
	raw := r.inner.Tools(ctx)
	if len(raw) == 0 {
		return raw
	}
	out := make([]trpctool.Tool, len(raw))
	for i, t := range raw {
		out[i] = r.wrapTool(t)
	}
	return out
}

func (r *redactingToolSet) wrapTool(t trpctool.Tool) trpctool.Tool {
	if t == nil {
		return nil
	}
	ct, ok := t.(trpctool.CallableTool)
	if !ok {
		return t
	}
	return &redactingCallable{inner: ct, env: r.env}
}

type redactingCallable struct {
	inner trpctool.CallableTool
	env   map[string]string
}

func (rc *redactingCallable) Declaration() *trpctool.Declaration {
	if rc.inner == nil {
		return nil
	}
	return rc.inner.Declaration()
}

func (rc *redactingCallable) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	if rc.inner == nil {
		return nil, nil
	}
	result, err := rc.inner.Call(ctx, jsonArgs)
	if err != nil {
		return result, err
	}
	return rc.redactResult(result), nil
}

func (rc *redactingCallable) redactResult(result any) any {
	m, ok := result.(map[string]any)
	if !ok {
		return result
	}
	if output, _ := m["output"].(string); output != "" {
		m["output"] = RedactOutput(rc.env, output)
	}
	return m
}

func BuildHostexecToolSet(baseDir string, env map[string]string) (trpctool.ToolSet, error) {
	var opts []trpchostexec.Option
	if baseDir != "" {
		opts = append(opts, trpchostexec.WithBaseDir(baseDir))
	}
	ts, err := trpchostexec.NewToolSet(opts...)
	if err != nil {
		return nil, err
	}
	normalized := hostexecnorm.WrapToolSet(ts)
	if len(env) == 0 {
		return normalized, nil
	}
	return NewRedactingToolSet(normalized, env), nil
}
