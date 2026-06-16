package event

import (
	frameworktracing "trpc.group/trpc-go/trpc-agent-go/event/tracing"
)

// SpanContext holds span tree data.
// Delegates to the framework tracing.SpanContext implementation.
//
// TECH-DEBT(P2-alignment): type alias delegates to framework; evaluate if
// project-specific extensions are needed in future iterations.
type SpanContext = frameworktracing.SpanContext

// NewSpanContext creates a SpanContext with initialized maps.
var NewSpanContext = frameworktracing.NewSpanContext

// UsageContext holds usage metadata.
// Delegates to the framework tracing.UsageContext implementation.
//
// TECH-DEBT(P2-alignment): type alias delegates to framework; evaluate if
// project-specific extensions are needed in future iterations.
type UsageContext = frameworktracing.UsageContext

// NewUsageContext creates a UsageContext with turnStart set to now.
var NewUsageContext = frameworktracing.NewUsageContext
