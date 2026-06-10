package event

// StepID namespacing for the chat-visible execution_progress envelope stream.
//
// All orchestration steps that surface in the AgentTreeTimeline MUST use a
// step_id from this set (or extend it here first). Using a magic string in
// the call site is forbidden because the frontend merges envelopes by
// step_id to compute per-step status transitions.
//
// Conventions:
//   - dot-separated namespace: "<domain>.<subsystem>.<verb>"
//   - one constant per *public* step (rare user-visible steps)
//   - private internals stay as string literals at the call site
//
// @see docs/reports/2026-06-10-proposal-execution-progress-inline.md
const (
	// StepIDChatLLMInvoke — the 3-15s wait between user message and the
	// LLM's first streamed token. The single biggest contributor to the
	// "黑屏" feel that this feature was created to fix. P0 step.
	StepIDChatLLMInvoke = "chat.llm.invoke"
)
