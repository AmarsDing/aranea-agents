## MODIFIED Requirements

### Requirement: Gateway single-write path
Gateway SHALL NOT directly write to zap.Logger. All log entries SHALL be emitted through Pipeline.Emit only. FileSink SHALL internally use zapcore.Core for JSON file writing. Gateway SHALL receive Pipeline at construction time (not via post-construction SetPipeline).

#### Scenario: Gateway.Info writes through Pipeline only
- **WHEN** Gateway.Info("message", fields...) is called
- **THEN** Pipeline.Emit SHALL be called exactly once
- **AND** zap.Logger.Info SHALL NOT be called directly on the Gateway

#### Scenario: Gateway constructed with Pipeline
- **WHEN** loggateway.New(bc.Logging, pipeline) is called
- **THEN** the Gateway SHALL hold a reference to Pipeline from construction
- **AND** SetPipeline SHALL NOT be needed

#### Scenario: FileSink uses zapcore internally
- **WHEN** FileSink.Write(entry) is called
- **THEN** the entry SHALL be serialized using zapcore.JSONEncoder
- **AND** written to lumberjack via zapcore.AddSync

#### Scenario: No duplicate log files
- **WHEN** a log entry is emitted at info level
- **THEN** exactly one JSON line SHALL appear in the output file
- **AND** no duplicate entry SHALL exist in any other log file

### Requirement: Configuration-driven Sink registration
Pipeline Sink registration SHALL be driven by conf.Logging.Sinks configuration. Hard-coded Sink registration in main.go SHALL be replaced by config-based registration. SinkType and DropPolicy SHALL use Proto enum (not free-form strings) for compile-time safety.

#### Scenario: FileSink registered via config
- **WHEN** conf.Logging.Sinks contains {name: "file", type: SINK_TYPE_FILE, buffer_size: 8192}
- **THEN** a FileSink SHALL be created with buffer_size=8192
- **AND** registered with Pipeline as a SinkGroup

#### Scenario: EventBusSink registered via config
- **WHEN** conf.Logging.Sinks contains {name: "eventbus", type: SINK_TYPE_EVENTBUS, buffer_size: 2048, drop_policy: DROP_POLICY_NEWEST}
- **THEN** an EventBusSink SHALL be created with the specified config

#### Scenario: Unknown Sink type
- **WHEN** conf.Logging.Sinks contains an unknown SinkType enum value
- **THEN** Pipeline initialization SHALL log a warning and skip that Sink
- **AND** other Sinks SHALL be registered normally

### Requirement: loggateway.Global() deprecation
loggateway.Global() SHALL be marked as deprecated. New code SHALL use constructor injection. Existing call sites SHALL be migrated incrementally per module. Test code (61 call sites) SHALL NOT be migrated — Global() usage in tests is reasonable.

#### Scenario: New code uses constructor injection
- **WHEN** a new service/biz/data struct is created
- **THEN** it SHALL receive loggateway.Logger via constructor parameter
- **AND** it SHALL NOT call loggateway.Global()

#### Scenario: Existing Global() calls continue to work
- **WHEN** existing code calls loggateway.Global()
- **THEN** it SHALL return the global Gateway instance
- **AND** a deprecation comment SHALL be present on the function

#### Scenario: Test code Global() calls remain
- **WHEN** test code calls loggateway.Global()
- **THEN** it SHALL work as before
- **AND** no migration SHALL be required for test code

## ADDED Requirements

### Requirement: LoggingSink Proto message with enums
A new LoggingSink message SHALL be added to conf.proto with fields: name, type (SinkType enum), buffer_size, drop_policy (DropPolicy enum), and config map. SinkType and DropPolicy SHALL be Proto enums.

#### Scenario: Proto compilation
- **WHEN** make api is run after adding LoggingSink message and enums
- **THEN** the generated Go code SHALL compile without errors
- **AND** existing Logging message fields SHALL remain unchanged

### Requirement: SinkGroup stats aggregation
Pipeline.Stats() SHALL aggregate stats from all SinkGroups, including per-Sink dropped counts and buffer utilization.

#### Scenario: Stats include per-Sink metrics
- **WHEN** Pipeline.Stats() is called
- **THEN** the result SHALL include per-Sink dropped count
- **AND** per-Sink channel length and capacity

### Requirement: Eliminate boundInfraRef() global mutable state
FlowTracker SHALL receive Infra as a constructor parameter. The global `boundInfraRef()` function and `BindInfra()` mutation SHALL be eliminated for FlowTracker usage. FlowTracker.emit() SHALL always use the injected Infra.Publish(), with no fallback to a global reference.

**Known limitation**: Infra.Publish() internally still uses monitorBusRef() → boundInfraRef(). This proposal only eliminates the global reference at the FlowTracker layer. Infra-level cleanup is a future independent change.

#### Scenario: FlowTracker uses injected Infra
- **WHEN** FlowTracker.emit() is called
- **THEN** it SHALL use the Infra instance provided at construction time
- **AND** it SHALL NOT call boundInfraRef()

#### Scenario: FlowTracker created before BindInfra
- **WHEN** a FlowTracker is created with an Infra instance
- **AND** BindInfra() has not yet been called
- **THEN** FlowTracker.emit() SHALL still work correctly via the injected Infra
- **AND** no global state dependency SHALL exist

#### Scenario: boundInfraRef removal
- **WHEN** all FlowTracker/TraceEmitter instances use injected Infra
- **THEN** boundInfraRef() and BindInfra() SHALL be marked deprecated
- **AND** they SHALL be removed in a future cleanup iteration
