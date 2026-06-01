// Package serviceawaitreply provides a service-integrated await_user_reply tool
// that blocks the current agent turn mid-flight until the user sends a reply via
// the AwaitUserReply RPC, or the context is cancelled.
//
// Unlike the framework's built-in await_reply_tool (which only marks routing state
// for the next turn), this tool also:
//   - Writes a channel into the service's awaitChans map (EP-RT-02).
//   - Sets the session run-status to "awaiting_user" while blocked.
//   - Returns the user's reply text to the agent so it can continue.
package serviceawaitreply

import (
	"context"
	"fmt"

	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// ReplyFunc is a blocking callback that must:
//  1. Register the session in the service's awaitChans map.
//  2. Set the run-status to "awaiting_user".
//  3. Block until the user sends a reply via AwaitUserReply RPC or ctx is done.
//  4. Return the reply text (or an error if ctx is done).
type ReplyFunc func(ctx context.Context) (reply string, err error)

type replyFuncKey struct{}

// WithReplyFunc injects fn into ctx so the ServiceTool can retrieve it at call time.
func WithReplyFunc(ctx context.Context, fn ReplyFunc) context.Context {
	return context.WithValue(ctx, replyFuncKey{}, fn)
}

// ReplyFuncFromContext retrieves the ReplyFunc previously injected with WithReplyFunc.
func ReplyFuncFromContext(ctx context.Context) ReplyFunc {
	fn, _ := ctx.Value(replyFuncKey{}).(ReplyFunc)
	return fn
}

// ServiceTool is the await_user_reply tool that integrates with ChatService's
// awaitChans mechanism to block mid-turn waiting for a user reply.
type ServiceTool struct {
	lg loggateway.Logger
}

func New(lg loggateway.Logger) trpctool.CallableTool {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &ServiceTool{lg: lg}
}

// Declaration implements trpctool.Tool.
func (t *ServiceTool) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{
		Name: "await_user_reply",
		Description: "Pause the current agent turn and wait for the user to send a " +
			"reply before continuing. Use right before asking for missing information.",
		InputSchema: &trpctool.Schema{
			Type:       "object",
			Properties: map[string]*trpctool.Schema{},
		},
	}
}

// Call implements trpctool.CallableTool.
// It marks the framework routing state so the next user turn routes here, then
// blocks via the injected ReplyFunc until the user responds or ctx is done.
func (t *ServiceTool) Call(ctx context.Context, _ []byte) (any, error) {
	// Mark framework routing state (so the runner persists await_user_reply route).
	if inv, ok := trpcagent.InvocationFromContext(ctx); ok && inv != nil {
		if err := trpcagent.MarkAwaitingUserReply(inv); err != nil {
			t.lg.Warn("MarkAwaitingUserReply failed",
				loggateway.StepID("tool.await_user_reply.mark_fail"),
				loggateway.Err(err))
		}
	}

	fn := ReplyFuncFromContext(ctx)
	if fn == nil {
		// No hook configured – return gracefully (tool is enabled but service hook
		// is absent, e.g. in tests). Do not block.
		return map[string]any{
			"success": false,
			"message": "await_user_reply: reply hook not configured; tool is a no-op in this context",
		}, nil
	}

	reply, err := fn(ctx)
	if err != nil {
		return nil, fmt.Errorf("await_user_reply: %w", err)
	}
	return map[string]any{
		"success": true,
		"reply":   reply,
	}, nil
}
