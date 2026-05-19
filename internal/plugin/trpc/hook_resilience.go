package plugintrpc

import (
	"context"
	"log/slog"

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
		return callbacks.NewBeforeAgentHook(h.Priority(), func(ctx context.Context, args *trpcagent.BeforeAgentArgs) (*trpcagent.BeforeAgentResult, error) {
			res, err := h.HandleBeforeAgent(ctx, args)
			return res, resilientHookErr("before_agent", err)
		})
	case callbacks.AfterAgentHook:
		return callbacks.NewAfterAgentHook(h.Priority(), func(ctx context.Context, args *trpcagent.AfterAgentArgs) (*trpcagent.AfterAgentResult, error) {
			res, err := h.HandleAfterAgent(ctx, args)
			return res, resilientHookErr("after_agent", err)
		})
	case callbacks.BeforeModelHook:
		return callbacks.NewBeforeModelHook(h.Priority(), func(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
			res, err := h.HandleBeforeModel(ctx, args)
			return res, resilientHookErr("before_model", err)
		})
	case callbacks.AfterModelHook:
		return callbacks.NewAfterModelHook(h.Priority(), func(ctx context.Context, args *trpcmodel.AfterModelArgs) (*trpcmodel.AfterModelResult, error) {
			res, err := h.HandleAfterModel(ctx, args)
			return res, resilientHookErr("after_model", err)
		})
	case callbacks.BeforeToolHook:
		return callbacks.NewBeforeToolHook(h.Priority(), func(ctx context.Context, args *trpctool.BeforeToolArgs) (*trpctool.BeforeToolResult, error) {
			res, err := h.HandleBeforeTool(ctx, args)
			return res, resilientHookErr("before_tool", err)
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

func (r *resilientAfterToolHook) HandleAfterTool(ctx context.Context, args *trpctool.AfterToolArgs) (*trpctool.AfterToolResult, error) {
	res, err := r.inner.HandleAfterTool(ctx, args)
	return res, resilientHookErr("after_tool", err)
}

func resilientHookErr(point string, err error) error {
	if err == nil {
		return nil
	}
	if metrics.IsBlockedErr(err) {
		return err
	}
	slog.Warn("hook: non-block error suppressed", "point", point, "error", err)
	return nil
}
