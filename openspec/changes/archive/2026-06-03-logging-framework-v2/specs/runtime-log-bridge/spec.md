## ADDED Requirements

### Requirement: RuntimeLogAdapter implements trpc-agent-go/log.Logger
A RuntimeLogAdapter SHALL be created in `internal/adapter/runtime_log.go` that implements the `trpc.group/trpc-go/trpc-agent-go/log.Logger` interface, delegating all calls to a `loggateway.Logger` instance. The adapter SHALL be placed in `internal/adapter/` (NOT `pkg/loggateway/`) to avoid pkg/ layer cross-dependency (loggateway SHALL NOT import trpc-agent-go).

#### Scenario: Info-level runtime log forwarded to Pipeline
- **WHEN** RuntimeLogAdapter.Info("agent started") is called
- **THEN** loggateway.Logger.Info("agent started") SHALL be called
- **AND** the entry SHALL enter logpipeline.Pipeline

#### Scenario: Formatted runtime log forwarded to Pipeline
- **WHEN** RuntimeLogAdapter.Infof("model %s invoked", "gpt-4") is called
- **THEN** loggateway.Logger.Info("model gpt-4 invoked") SHALL be called

#### Scenario: Debug-level runtime log respects loggateway level
- **WHEN** RuntimeLogAdapter.Debug("trace detail") is called
- **AND** loggateway level is set to "info"
- **THEN** the entry SHALL NOT be written to any Sink

### Requirement: Fatal level special handling
RuntimeLogAdapter.Fatal() and RuntimeLogAdapter.Fatalf() SHALL synchronously write to os.Stderr and an independent *zap.Logger (not through Pipeline), then call os.Exit(1). The independent zap.Logger is the only allowed "direct write" exception, used solely for Fatal level because Pipeline's async dispatch may not flush before exit.

#### Scenario: Fatal log is written before exit
- **WHEN** RuntimeLogAdapter.Fatal("critical failure") is called
- **THEN** the message SHALL be written to os.Stderr synchronously
- **AND** the message SHALL be written to the independent zap.Logger synchronously
- **AND** os.Exit(1) SHALL be called after both writes complete

#### Scenario: Fatal does not go through Pipeline
- **WHEN** RuntimeLogAdapter.Fatal("critical failure") is called
- **THEN** loggateway.Logger SHALL NOT be called for this entry
- **AND** the entry SHALL NOT enter logpipeline.Pipeline

### Requirement: Replace log.Default at startup
In cmd/admin/main.go, after loggateway.New() succeeds, `trpc.group/trpc-go/trpc-agent-go/log.Default` and `log.ContextDefault` SHALL be replaced with a RuntimeLogAdapter instance wrapping the loggateway.Logger.

#### Scenario: Runtime logs appear in Pipeline output after startup
- **WHEN** the application starts with loggateway configured
- **AND** an Agent run produces runtime logs via log.Infof()
- **THEN** those logs SHALL appear in the Pipeline output (file/stdout/EventBus)

#### Scenario: Runtime logs before loggateway init go to default stdout
- **WHEN** trpc-agent-go code calls log.Info() before loggateway.New() completes
- **THEN** the log SHALL go to the default zap.Sugar stdout output
- **AND** no logs SHALL be lost during the startup window

### Requirement: RuntimeLogAdapter preserves context fields
RuntimeLogAdapter SHALL support With() to preset context fields (e.g., session_id, agent_key) that are automatically attached to all forwarded log entries. With() SHALL return a new RuntimeLogAdapter instance (immutable pattern).

#### Scenario: Preset fields attached to runtime logs
- **WHEN** RuntimeLogAdapter.With(loggateway.SessionID("abc")) is called
- **AND** the returned adapter.Info("step completed") is called
- **THEN** the log entry SHALL include session_id="abc" in its fields

#### Scenario: With returns new instance
- **WHEN** RuntimeLogAdapter.With(fields...) is called
- **THEN** a new RuntimeLogAdapter instance SHALL be returned
- **AND** the original adapter SHALL remain unchanged

### Requirement: No modification to trpc-agent-go source code
The RuntimeLogAdapter SHALL NOT require any changes to the trpc-agent-go source code. It SHALL only use the public `log.Logger` interface and `log.Default`/`log.ContextDefault` variables.

#### Scenario: trpc-agent-go remains unmodified
- **WHEN** RuntimeLogAdapter is integrated
- **THEN** no files under `pkg/trpc-agent-go/` SHALL be modified

### Requirement: Adapter in internal/ layer, not pkg/
RuntimeLogAdapter SHALL be placed in `internal/adapter/` to avoid pkg/ layer cross-dependency. `pkg/loggateway` SHALL NOT import `pkg/trpc-agent-go`.

#### Scenario: loggateway does not depend on trpc-agent-go
- **WHEN** RuntimeLogAdapter is integrated
- **THEN** `pkg/loggateway/` SHALL NOT import any package from `pkg/trpc-agent-go/`
