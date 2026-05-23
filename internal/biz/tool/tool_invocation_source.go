package tool

// ToolInvocationSource labels where a tool_invocations row was recorded.
const (
	ToolInvocationSourceRuntime  = "trpc"       // trpc-agent-go AfterTool hook
	ToolInvocationSourceEventBus = "event_bus"  // EventBus tool_result consumer
)
