## ADDED Requirements

### Requirement: SinkGroup independent queue model
Each Sink registered with Pipeline SHALL be wrapped in a SinkGroup with an independent goroutine and channel buffer. A slow Sink SHALL NOT block other SinkGroups.

#### Scenario: Slow Sink does not affect other Sinks
- **WHEN** EventBusSink.Write() takes 200ms per call (e.g., EventBus congestion)
- **AND** FileSink is also registered
- **THEN** FileSink SHALL continue writing without delay
- **AND** EventBusSink's channel SHALL drop entries when full (DropNewest policy)

#### Scenario: SinkGroup buffer overflow
- **WHEN** a SinkGroup's channel buffer is full
- **AND** drop_policy is DropNewest
- **THEN** the new entry SHALL be dropped
- **AND** SinkGroup.dropped counter SHALL increment by 1

#### Scenario: SinkGroup with block policy
- **WHEN** a SinkGroup's channel buffer is full
- **AND** drop_policy is DropBlock
- **THEN** Pipeline.Emit SHALL block until the channel has space
- **AND** no entries SHALL be dropped for this SinkGroup

### Requirement: SinkGroup.Emit method signature
SinkGroup.Emit SHALL be a non-blocking method that returns immediately after routing the entry to the channel (or dropping it). The method signature SHALL be `Emit(entry LogEntry) error`.

#### Scenario: Emit returns nil on successful routing
- **WHEN** SinkGroup.Emit(entry) is called
- **AND** the channel has space
- **THEN** the entry SHALL be placed in the channel
- **AND** nil SHALL be returned

#### Scenario: Emit returns error on drop
- **WHEN** SinkGroup.Emit(entry) is called
- **AND** the channel is full with DropNewest policy
- **THEN** the entry SHALL be dropped
- **AND** an error indicating drop SHALL be returned

### Requirement: Per-SinkGroup configurable buffer size
Each SinkGroup SHALL support a configurable channel buffer size. Default values SHALL be: FileSink=8192, StdoutSink=4096, EventBusSink=2048.

#### Scenario: Custom buffer size via config
- **WHEN** LoggingSink config specifies buffer_size=16384 for a file Sink
- **THEN** the SinkGroup's channel SHALL be created with capacity 16384

#### Scenario: Default buffer size when not specified
- **WHEN** LoggingSink config does not specify buffer_size
- **THEN** the SinkGroup SHALL use the default buffer size for that Sink type

### Requirement: SinkGroup panic recovery
Each SinkGroup goroutine SHALL recover from panics in Sink.Write() and increment an error counter, without crashing the goroutine.

#### Scenario: Sink.Write() panics
- **WHEN** a Sink's Write() method panics
- **THEN** the SinkGroup goroutine SHALL recover
- **AND** sink_errors counter SHALL increment by 1
- **AND** the SinkGroup SHALL continue processing subsequent entries

### Requirement: SinkGroup lifecycle management
Pipeline.Close() SHALL wait for all SinkGroup goroutines to drain their channels before returning.

#### Scenario: Graceful shutdown with pending entries
- **WHEN** Pipeline.Close() is called
- **AND** a SinkGroup channel has 100 pending entries
- **THEN** the SinkGroup SHALL write all 100 entries to the Sink
- **AND** Pipeline.Close() SHALL wait for the SinkGroup goroutine to finish

### Requirement: DropPolicy enum
DropPolicy SHALL be a Go enum type (`type DropPolicy int`) with values DropNewest=0 and DropBlock=1, aligned with the Proto DropPolicy enum.

#### Scenario: DropPolicy values match Proto enum
- **WHEN** a LoggingSink config has drop_policy=DROP_POLICY_NEWEST
- **THEN** the SinkGroup SHALL use DropNewest DropPolicy
