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
	// StepIDChatAgentBuild — the BUILD phase where agent deps are assembled,
	// the agent is constructed (or fetched from cache), and the runner is created.
	// Previously invisible to the user; now surfaced so the frontend can show
	// "正在准备" during the 0-15s build window.
	StepIDChatAgentBuild = "chat.agent.build"

	// StepIDChatLLMInvoke — the 3-15s wait between user message and the
	// LLM's first streamed token. The single biggest contributor to the
	// "黑屏" feel that this feature was created to fix. P0 step.
	StepIDChatLLMInvoke = "chat.llm.invoke"

	// StepIDChatIntentPass — the intent recognition pass that runs before
	// the main LLM call. Typically 0.5-3s. Surfaced so the frontend can
	// show "正在理解意图" during this phase.
	StepIDChatIntentPass = "chat.intent.pass"

	// StepIDChatProactiveRecall — proactive memory recall that runs before
	// BUILD to surface relevant memories based on the conversation context
	// (P3-11). Typically 0.1-2s. Surfaced so the frontend can show
	// "正在检索相关记忆" instead of a black screen between message send
	// and the BUILD phase. Only emits start/done when the recaller is
	// actually wired (no-op early returns stay silent).
	StepIDChatProactiveRecall = "chat.proactive_recall"

	// StepIDChatPrePlanningGate — the hard-gate planning path that runs
	// after Intent Pass when the pre-planning gate decides ForcePlanning.
	// The Service layer directly invokes TaskPlanner.Plan() to create and
	// persist a plan. Surfaced so the frontend can show "正在创建任务规划"
	// during this potentially multi-second step. Only emits start/done
	// when ForcePlanning=true (the common soft-gate path stays silent).
	StepIDChatPrePlanningGate = "chat.pre_planning_gate"
)
