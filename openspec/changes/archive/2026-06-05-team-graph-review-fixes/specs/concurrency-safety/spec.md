## ADDED Requirements

### Requirement: evictIfNeeded must hold execMu when reading GraphExecution fields
`evictIfNeeded()` SHALL hold `exec.execMu.RLock()` when reading `exec.Status`, `exec.FinishedAt`, `exec.StartedAt`, or `exec.runtime` fields. The lock order SHALL be: release `uc.mu` before acquiring `execMu`, then re-acquire `uc.mu` if needed.

#### Scenario: evictIfNeeded reads exec.Status under execMu protection
- **WHEN** `evictIfNeeded()` is called and iterates over `uc.executions`
- **THEN** it SHALL acquire `exec.execMu.RLock()` before reading any `exec.*` field
- **AND** it SHALL release `exec.execMu.RUnlock()` after reading

#### Scenario: evictIfNeeded lock order prevents deadlock
- **WHEN** `evictIfNeeded()` needs both `uc.mu` and `execMu`
- **THEN** it SHALL release `uc.mu` before acquiring `execMu`
- **AND** re-acquire `uc.mu` after releasing `execMu` if further map operations are needed

### Requirement: gc must hold execMu when modifying GraphExecution fields
`gc()` SHALL hold `exec.execMu.Lock()` when modifying `exec.Status`, `exec.FinishedAt`, or `exec.runtime` fields.

#### Scenario: gc modifies exec.Status under execMu protection
- **WHEN** `gc()` sets `exec.Status = "failed"` or `exec.FinishedAt = &now`
- **THEN** it SHALL hold `exec.execMu.Lock()` during the modification
- **AND** it SHALL release `exec.execMu.Unlock()` after modification

#### Scenario: gc cancels runtime under execMu protection
- **WHEN** `gc()` calls `exec.runtime.Cancel()`
- **THEN** it SHALL hold `exec.execMu.Lock()` during the call

### Requirement: consumeRuntimeEvents must read evicted under execMu
`consumeRuntimeEvents` SHALL read `exec.evicted` while holding `exec.execMu`.

#### Scenario: evicted field read under lock
- **WHEN** `consumeRuntimeEvents` reads `exec.evicted`
- **THEN** it SHALL be within a section protected by `exec.execMu`

### Requirement: TeamGraphRunCoordinator.finisher must be concurrency-safe
`TeamGraphRunCoordinator.finisher` SHALL be protected by `sync.Once` to ensure safe concurrent access after initialization.

#### Scenario: SetFinisher called exactly once
- **WHEN** `SetFinisher` is called during startup wiring
- **THEN** `sync.Once` SHALL ensure the finisher is set exactly once
- **AND** subsequent calls SHALL be no-ops

#### Scenario: finisher read after SetFinisher
- **WHEN** a safego goroutine reads `c.finisher`
- **THEN** the read SHALL be safe without additional locking (guaranteed by sync.Once initialization before goroutine start)

### Requirement: CompileToCompiledTeam linked graph path must apply adaptive mode
`CompileToCompiledTeam` SHALL apply `applyAdaptiveAgentDestinations` in the linked graph path when the compile mode is "adaptive" or "swarm".

#### Scenario: linked graph with adaptive mode
- **WHEN** `CompileToCompiledTeam` processes a Team Definition with `linked_graph_id` and mode "adaptive"
- **THEN** it SHALL call `applyAdaptiveAgentDestinations(cfg)` before `finalizeRuntimeGraphConfig`
- **AND** node Destinations SHALL be populated with transfer edge targets

#### Scenario: linked graph with non-adaptive mode
- **WHEN** `CompileToCompiledTeam` processes a Team Definition with `linked_graph_id` and mode "sequential"
- **THEN** it SHALL NOT call `applyAdaptiveAgentDestinations`
