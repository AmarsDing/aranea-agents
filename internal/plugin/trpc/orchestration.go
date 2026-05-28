package plugintrpc

// Callback orchestration boundaries:
//
// 1. Runner WithPlugins — DB-backed built-in plugins + framework plugins (identity, guardrail,
//    toolcallid, messagemerger). Order: plugins.sort_order ASC at Runtime.Apply, then framework
//    plugins appended by Manager.RunnerPluginsForAgent.
//
// 2. LLMAgent Callback Chain — product metrics, tool timing/recorder + hook rules.
//    Order: fixed product priorities (timing=5, recorder=50) + hooks at 300+sort_order.
//
// 3. ModelSelector — model_router / cost_guard catalog swaps only (no duplicate BeforeModel routing).
//
// 4. Hook rules — user-defined Chain entries; on_event via productEventPlugin bridge.
//
// confirmation_guard Runner plugin blocks directly via BeforeTool CustomResult.
// permission_guard denies deny_tools via BeforeTool CustomResult.
