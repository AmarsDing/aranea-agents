package agent

import (
	"context"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/agent/intent"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// intentReorderHookPriority makes the intent reorder run after every other
// message-mutating BeforeModel hook (memory cue 5, knowledge cue 6), so the
// intent context lands as the final message of the request.
const intentReorderHookPriority = 100

// newIntentReorderBeforeHook moves injected intent-context system messages to
// the END of the request message list (P2 TTFT optimization).
//
// The framework content processor appends RunOptions.InjectedContextMessages
// right after the system block, BEFORE session history. Intent JSON changes
// every turn, so that position invalidates the prompt-cache prefix for the
// entire history. Moving it to the tail keeps [system block + history + user]
// a monotonically growing, cacheable prefix; only the trailing dynamic block
// (memory cue / knowledge cue / reply reminder / intent) is reprocessed.
//
// The hook runs on the freshly rebuilt request of every model call (tool-loop
// re-entries included), so the move is idempotent and never accumulates.
func newIntentReorderBeforeHook() callbacks.Callback {
	return callbacks.NewBeforeModelHook(intentReorderHookPriority, callbacks.LayerDynamic, func(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
		if args == nil || args.Request == nil {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		msgs := args.Request.Messages
		// Fast path: no intent context anywhere — no allocation, no copy.
		hasIntent := false
		for i := range msgs {
			if intent.IsIntentContextContent(msgs[i].Content) {
				hasIntent = true
				break
			}
		}
		if !hasIntent {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		// Stable partition: non-intent messages keep their relative order,
		// intent messages (normally exactly one) append at the tail as a
		// user-role dynamic cue so they stay out of DeepSeek's system prefix.
		body := make([]trpcmodel.Message, 0, len(msgs))
		var tail []trpcmodel.Message
		for _, m := range msgs {
			if intent.IsIntentContextContent(m.Content) {
				tail = append(tail, asDynamicCue(m.Content))
				continue
			}
			body = append(body, m)
		}
		args.Request.Messages = append(body, tail...)
		return &trpcmodel.BeforeModelResult{Context: ctx}, nil
	})
}
