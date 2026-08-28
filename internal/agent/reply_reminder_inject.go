package agent

import (
	"context"

	"aranea-agents/internal/agent/callbacks"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// replyReminderStateKey is the invocation state key set by the AfterTool hook
// to signal that the next BeforeModel call should inject a reply reminder.
const replyReminderStateKey = "aranea.reply_reminder"

// replyReminderCue is the system message injected after each tool call to
// remind the LLM to output a brief reply before calling the next tool.
// This enforces the "中间回复规则" from DECISION.md at the plugin level,
// ensuring the model doesn't go silent for multiple tool calls in a row.
const replyReminderCue = `[系统提醒] 你刚刚执行了一个工具。在调用下一个工具之前，请先输出一段简短回复：
1. 已完成：工具执行了什么、结果如何
2. 下一步：接下来准备做什么
这能提高可观测性，让用户实时了解执行进度。`

// newReplyReminderAfterHook returns an AfterTool hook that sets invocation
// state after each tool call. The companion BeforeModel hook
// (newReplyReminderBeforeHook) reads this state and injects a system message
// reminding the LLM to output a reply.
//
// This implements Problem 6 方案B (plugin injection): even when the system
// prompt rule is not followed by the LLM, the plugin-level reminder provides
// a second layer of enforcement.
func newReplyReminderAfterHook() callbacks.AfterToolHook {
	return callbacks.NewAfterToolHook(0, func(ctx context.Context, args *trpctool.AfterToolArgs) (*trpctool.AfterToolResult, error) {
		if args == nil {
			return &trpctool.AfterToolResult{}, nil
		}
		inv, ok := trpcagent.InvocationFromContext(ctx)
		if !ok || inv == nil {
			return &trpctool.AfterToolResult{}, nil
		}
		inv.SetState(replyReminderStateKey, true)
		return &trpctool.AfterToolResult{}, nil
	})
}

// newReplyReminderBeforeHook returns a BeforeModel hook that reads the
// reply-reminder state and injects a system message if a tool was called
// since the last LLM call. The state is cleared after injection so the
// reminder is only injected once per tool call.
func newReplyReminderBeforeHook() callbacks.Callback {
	return callbacks.NewBeforeModelHook(3, callbacks.LayerDynamic, func(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
		if args == nil || args.Request == nil {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		inv, ok := trpcagent.InvocationFromContext(ctx)
		if !ok || inv == nil {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		v, found := inv.GetState(replyReminderStateKey)
		if !found {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		// Clear state so the reminder is only injected once per tool call.
		inv.SetState(replyReminderStateKey, false)

		need, ok := v.(bool)
		if !ok || !need {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}

		args.Request.Messages = replaceDynamicCue(args.Request.Messages, replyReminderCueMarker, replyReminderCueMarker+replyReminderCue)
		return &trpcmodel.BeforeModelResult{Context: ctx}, nil
	})
}
