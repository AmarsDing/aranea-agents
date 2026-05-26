package plugintrpc

import (
	"context"
	"fmt"
	"runtime/debug"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/metrics"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// wrapResilientHooks swallows non-block hook errors so one rule cannot abort a turn.
func wrapResilientHooks(entries []callbacks.Callback) []callbacks.Callback {
	if len(entries) == 0 {
		return entries
	}
	out := make([]callbacks.Callback, 0, len(entries))
	for _, cb := range entries {
		if wrapped := wrapResilient(cb); wrapped != nil {
			out = append(out, wrapped)
		}
	}
	return out
}

func wrapResilient(cb callbacks.Callback) callbacks.Callback {
	switch h := cb.(type) {
	case callbacks.BeforeAgentHook:
		return callbacks.NewBeforeAgentHook(h.Priority(), func(ctx context.Context, args *trpcagent.BeforeAgentArgs) (res *trpcagent.BeforeAgentResult, err error) {
			defer func() { err = recoverHookPanic("before_agent", recover(), err) }()
			res, err = h.HandleBeforeAgent(ctx, args)
			err = resilientHookErr("before_agent", err)
			return
		})
	case callbacks.AfterAgentHook:
		return callbacks.NewAfterAgentHook(h.Priority(), func(ctx context.Context, args *trpcagent.AfterAgentArgs) (res *trpcagent.AfterAgentResult, err error) {
			defer func() { err = recoverHookPanic("after_agent", recover(), err) }()
			res, err = h.HandleAfterAgent(ctx, args)
			err = resilientHookErr("after_agent", err)
			return
		})
	case callbacks.BeforeModelHook:
		return callbacks.NewBeforeModelHook(h.Priority(), func(ctx context.Context, args *trpcmodel.BeforeModelArgs) (res *trpcmodel.BeforeModelResult, err error) {
			defer func() { err = recoverHookPanic("before_model", recover(), err) }()
			res, err = h.HandleBeforeModel(ctx, args)
			err = resilientHookErr("before_model", err)
			return
		})
	case callbacks.AfterModelHook:
		return callbacks.NewAfterModelHook(h.Priority(), func(ctx context.Context, args *trpcmodel.AfterModelArgs) (res *trpcmodel.AfterModelResult, err error) {
			defer func() { err = recoverHookPanic("after_model", recover(), err) }()
			res, err = h.HandleAfterModel(ctx, args)
			err = resilientHookErr("after_model", err)
			return
		})
	case callbacks.BeforeToolHook:
		return callbacks.NewBeforeToolHook(h.Priority(), func(ctx context.Context, args *trpctool.BeforeToolArgs) (res *trpctool.BeforeToolResult, err error) {
			defer func() { err = recoverHookPanic("before_tool", recover(), err) }()
			res, err = h.HandleBeforeTool(ctx, args)
			err = resilientHookErr("before_tool", err)
			return
		})
	case callbacks.AfterToolHook:
		return &resilientAfterToolHook{inner: h}
	}
	return cb
}

type resilientAfterToolHook struct {
	inner callbacks.AfterToolHook
}

func (r *resilientAfterToolHook) Point() callbacks.CallbackPoint { return r.inner.Point() }
func (r *resilientAfterToolHook) Priority() int                  { return r.inner.Priority() }

func (r *resilientAfterToolHook) HandleAfterTool(ctx context.Context, args *trpctool.AfterToolArgs) (res *trpctool.AfterToolResult, err error) {
	defer func() { err = recoverHookPanic("after_tool", recover(), err) }()
	res, err = r.inner.HandleAfterTool(ctx, args)
	err = resilientHookErr("after_tool", err)
	return
}

func resilientHookErr(point string, err error) error {
	if err == nil {
		return nil
	}
	if metrics.IsBlockedErr(err) {
		return err
	}
	hookLogger.Warn("hook: non-block error suppressed", "point", point, "error", err)
	return nil
}

// recoverHookPanic turns a hook panic into a logged warning, preserving the same
// "non-block swallow" semantics as resilientHookErr (TPM-P1-05). If the deferred
// hook body already produced an error, we keep it; otherwise return nil so the
// caller's turn proceeds. The panic stack is captured for postmortem.
func recoverHookPanic(point string, recovered any, prior error) error {
	if recovered == nil {
		return prior
	}
	stack := debug.Stack()
	hookLogger.Error("hook: panic recovered",
		"point", point,
		"panic", fmt.Sprintf("%v", recovered),
		"stack", string(stack))
	if prior != nil {
		return prior
	}
	return nil
}
