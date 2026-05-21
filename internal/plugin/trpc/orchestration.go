package plugintrpc

// Callback orchestration boundaries (see docs/需求/22 plugin.design.md §11):
//
// 1. Runner WithPlugins — DB-backed built-in plugins (audit_log, guards, …).
//    Order: plugins.sort_order ASC at Runtime.Apply (from enabled plugin list query).
//
// 2. LLMAgent Callback Chain — product metrics, tool confirmation, tool timing/recorder.
//    Order: fixed product priorities (confirm=10, timing=5, recorder=50) + hooks at 300+sort_order.
//
// 3. ModelSelector — model_router / cost_guard catalog swaps only (no duplicate BeforeModel routing).
//
// 4. Hook rules — user-defined Chain entries; on_event via productEventPlugin bridge.
//
// 5. Chain-mirrored plugins (P3) — optional callback_orchestration:"chain" on non-exclusive
//    plugins; excluded from Runner to prevent double trigger. See orchestration_policy.go.
//
// Tool confirmation: unified ConfirmGate in LLMAgent Chain (priority 10).
// Catalog requires_confirmation + confirmation_guard confirm_tools/patterns merge in agent.buildToolConfirmGate.
// When AwaitUserReply hook is present, ConfirmGate blocks mid-turn until user approves.
// confirmation_guard Runner plugin is telemetry-only; permission_guard only denies deny_tools.
