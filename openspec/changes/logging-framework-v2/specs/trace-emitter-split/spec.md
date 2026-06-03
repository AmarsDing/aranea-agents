## ADDED Requirements

### Requirement: FlowTracker component
FlowTracker SHALL be a standalone component responsible for flow step lifecycle tracking (LogStart/LogDone/LogSkip/LogWarn/LogError/LogCritical). It SHALL NOT contain span management or usage aggregation logic. It SHALL hold its own FlowContext with an independent mutex.

#### Scenario: LogStart emits flow log entry
- **WHEN** FlowTracker.LogStart("chat.agent.build", "Building agent") is called
- **THEN** a FlowLogEntry with step.phase="start" SHALL be emitted via EventBus
- **AND** the step timer SHALL be recorded in FlowContext

#### Scenario: LogDone calculates duration
- **WHEN** FlowTracker.LogDone("chat.agent.build", "Agent built") is called
- **AND** LogStart was called 150ms earlier for the same stepID
- **THEN** a FlowLogEntry with step.phase="done" and timing.duration_ms=150 SHALL be emitted

#### Scenario: LogError publishes error envelope
- **WHEN** FlowTracker.LogError("chat.llm.invoke", "Model timeout") is called
- **THEN** a FlowLogEntry with step.phase="error" and severity="error" SHALL be emitted
- **AND** if shouldPublishFlowChatError returns true, an EnvelopeTypeError SHALL also be published to EventBus

### Requirement: SpanCollector component
SpanCollector SHALL be a standalone component responsible for span tree management (startSpan/endSpan/FinishRoot). It SHALL hold its own SpanContext with an independent mutex. It SHALL NOT depend on FlowTracker or UsageAggregator.

#### Scenario: Start and end span
- **WHEN** SpanCollector.StartSpan("llm.call", rootID, attrs) is called
- **THEN** a new span entry with status="running" SHALL be created in SpanContext
- **AND** the span ID SHALL be returned

#### Scenario: EndSpan records duration
- **WHEN** SpanCollector.EndSpan(spanID, "ok") is called
- **AND** the span was started 200ms earlier
- **THEN** the span's duration_ms SHALL be set to 200
- **AND** the span's status SHALL be set to "ok"

#### Scenario: FinishRoot closes all pending spans
- **WHEN** FinishRoot("error") is called
- **AND** there are 3 pending spans (1 LLM, 2 tool calls)
- **THEN** all pending spans SHALL be ended with status "error"

### Requirement: UsageAggregator component
UsageAggregator SHALL be a standalone component responsible for usage metadata collection (ObserveFrameworkEvent/mergeLLMSpan/MetadataJSON). It SHALL hold its own UsageContext with an independent mutex. It SHALL NOT depend on FlowTracker or SpanCollector.

#### Scenario: Merge LLM span with token counts
- **WHEN** UsageAggregator.MergeLLMSpan(100, 50) is called
- **THEN** the current open LLM span SHALL be updated with prompt_tokens=100, completion_tokens=50

#### Scenario: MetadataJSON output
- **WHEN** UsageAggregator.MetadataJSON() is called
- **THEN** it SHALL return a JSON string containing trace_id, spans array, and trace_root_ms

#### Scenario: OTel span ID sync
- **WHEN** UsageAggregator.SyncOtelSpanIDs(src) is called
- **THEN** matching spans SHALL have their otel_id field populated from the source

### Requirement: TraceContext split into independent contexts
The current monolithic TraceContext SHALL be split into three independent structs, each with its own mutex:
- FlowContext: flow step timing data
- SpanContext: span tree data
- UsageContext: usage metadata data

#### Scenario: No shared mutex between components
- **WHEN** FlowTracker is writing to FlowContext
- **AND** SpanCollector is writing to SpanContext
- **THEN** neither SHALL block the other (independent mutexes)

### Requirement: FlowTracker composes SpanCollector and UsageAggregator
FlowTracker SHALL hold optional references to SpanCollector and UsageAggregator. When present, FlowTracker.LogStart/LogDone SHALL delegate span tracking to SpanCollector, and ObserveFrameworkEvent SHALL delegate to UsageAggregator.

#### Scenario: FlowTracker with SpanCollector
- **WHEN** FlowTracker.LogStart("chat.llm.invoke", "Calling LLM") is called
- **AND** FlowTracker holds a SpanCollector reference
- **THEN** SpanCollector.StartSpan SHALL be called with the step ID
- **AND** a FlowLogEntry SHALL be emitted

#### Scenario: FlowTracker without SpanCollector
- **WHEN** FlowTracker.LogStart("chat.llm.invoke", "Calling LLM") is called
- **AND** FlowTracker does NOT hold a SpanCollector reference
- **THEN** only a FlowLogEntry SHALL be emitted (no span tracking)

### Requirement: shouldPublishFlowChatError belongs to FlowTracker
The shouldPublishFlowChatError function SHALL remain in FlowTracker, as it is a flow-level decision (based on flow type), not a span-level or usage-level concern.

#### Scenario: FlowTracker decides whether to publish chat error
- **WHEN** FlowTracker.LogError is called
- **THEN** FlowTracker SHALL evaluate shouldPublishFlowChatError using its own FlowContext data
- **AND** if true, publish EnvelopeTypeError via Infra

### Requirement: TraceEmitter backward compatibility via embedding wrapper
TraceEmitter SHALL be an embedding wrapper for FlowTracker (`type TraceEmitter struct { *FlowTracker }`), preserving all existing method signatures. Existing call sites SHALL NOT require changes. Type alias is NOT used because Go type alias requires identical underlying types.

#### Scenario: Existing code using TraceEmitter
- **WHEN** existing code calls TraceEmitter.LogStart(stepID, message)
- **THEN** the call SHALL work identically to FlowTracker.LogStart(stepID, message)

#### Scenario: TraceEmitter is not a type alias
- **WHEN** TraceEmitter is defined
- **THEN** it SHALL be `type TraceEmitter struct { *FlowTracker }` (embedding wrapper)
- **AND** it SHALL NOT be `type TraceEmitter = FlowTracker` (type alias)
