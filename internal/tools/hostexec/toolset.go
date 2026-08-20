package hostexec

import (
	"context"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	trpchostexec "trpc.group/trpc-go/trpc-agent-go/tool/hostexec"
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
	// Always redact — even on error, the result may contain sensitive values
	// (e.g., partial command output with env vars in stderr).
	return rc.redactResult(result), err
}

// redactStringFields applies RedactOutput to every string field in the map,
// covering output, stderr, error, and any other string values that may leak
// sensitive environment variable values.
func (rc *redactingCallable) redactStringFields(m map[string]any) map[string]any {
	for key, val := range m {
		if s, ok := val.(string); ok && s != "" {
			m[key] = RedactOutput(rc.env, s)
		}
	}
	return m
}

func (rc *redactingCallable) redactResult(result any) any {
	m, ok := result.(map[string]any)
	if !ok {
		return result
	}
	return rc.redactStringFields(m)
}

func BuildHostexecToolSet(baseDir string, env map[string]string) (trpctool.ToolSet, error) {
	var opts []trpchostexec.Option
	if baseDir != "" {
		opts = append(opts, trpchostexec.WithBaseDir(baseDir))
	}
	if len(env) > 0 {
		opts = append(opts, trpchostexec.WithBaseEnv(env))
	}
	ts, err := trpchostexec.NewToolSet(opts...)
	if err != nil {
		return nil, err
	}
	// UTF-8 归一化须在最内层：redacting/sessionEnhance/recorder/日志文件
	// 全部消费包装后的字符串，一次归一化全链路受益。
	ts = WrapUTF8Norm(ts)
	if len(env) > 0 {
		ts = NewRedactingToolSet(ts, env)
	}
	return WrapSessionEnhance(ts, baseDir, nil), nil
}
