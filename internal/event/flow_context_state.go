package event

import (
	frameworktracing "trpc.group/trpc-go/trpc-agent-go/event/tracing"
)

// FlowContext holds flow step timing data.
// Delegates to the framework tracing.FlowContext implementation.
//
// TECH-DEBT(P2-alignment): type alias delegates to framework; evaluate if
// project-specific extensions are needed in future iterations.
type FlowContext = frameworktracing.FlowContext

// NewFlowContext creates a FlowContext with initialized timers map.
var NewFlowContext = frameworktracing.NewFlowContext
