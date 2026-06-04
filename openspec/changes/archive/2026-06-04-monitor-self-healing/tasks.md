## Phase 1: Runtime Auto-Heal Foundation

### Task 1.1: Define AutoHealStrategy interface
- **File**: `pkg/trpc-agent-go/internal/flow/processor/autoheal.go`
- **What**: Define `AutoHealStrategy` interface with `CanHeal`, `Heal`, `MaxAttempts`, `Backoff` methods. Define `AutoHealConfig` struct and `WithAutoHealConfig` RunOption.
- **Spec**: REQ-RH-01, REQ-RH-06
- **Test**: Unit test for interface compliance and config defaults

### Task 1.2: LLM call auto-heal (retry + fallback)
- **File**: `pkg/trpc-agent-go/internal/flow/llmflow/llmflow.go`
- **What**: Integrate AutoHealStrategy into LLM flow. On transient errors (timeout/429/context-exceeded), execute retry with backoff. On context-exceeded, attempt compress-and-retry. Emit FlowLog with auto_healed metadata.
- **Spec**: REQ-RH-02, REQ-RH-05
- **Test**: Unit test with mock LLM returning transient errors, verify retry behavior and event emission

### Task 1.3: MCP connection auto-reconnect
- **File**: `pkg/trpc-agent-go/mcp/mcpbroker/`
- **What**: Add reconnection logic to MCP broker. On connection drop, attempt reconnect up to 3 times with exponential backoff. Emit FlowLog with auto_healed metadata.
- **Spec**: REQ-RH-03, REQ-RH-05
- **Test**: Integration test with mock MCP server that drops connection

### Task 1.4: Tool execution auto-retry
- **File**: `pkg/trpc-agent-go/internal/flow/processor/functioncall.go`
- **What**: Add retry logic for transient tool execution failures. Non-transient errors skip retry. Emit FlowLog with auto_healed metadata.
- **Spec**: REQ-RH-04, REQ-RH-05
- **Test**: Unit test with mock tool returning transient vs non-transient errors

### Task 1.5: Heal circuit breaker
- **File**: `pkg/trpc-agent-go/internal/flow/processor/autoheal.go`
- **What**: Implement circuit breaker that stops auto-heal after 5 consecutive failures within 10 minutes. Emit `heal_circuit_open` event. Reset after 30 minutes.
- **Spec**: REQ-RH-07
- **Test**: Unit test for circuit open/close behavior

## Phase 2: Monitor Observer

### Task 2.1: HealRecord Ent Schema
- **File**: `internal/data/ent/schema/heal_record.go`
- **What**: Define Ent schema with fields: id, rule_id, trigger_type, trace_id, session_id, step_id, fix_action_type, confidence, status, runtime_auto_healed, runtime_heal_attempts, reason, created_at. Run `go generate`.
- **Spec**: REQ-SO-05
- **Test**: Verify schema generates correctly

### Task 2.2: HealRecordRepo
- **File**: `internal/data/heal_record_repo.go`
- **What**: Implement `HealRecordRepo` interface (Insert, List, DeleteOlderThan) using Ent. Add TTL cleanup cron job.
- **Spec**: REQ-SO-05
- **Test**: Unit test with in-memory SQLite

### Task 2.3: SelfHealObserver
- **File**: `internal/biz/monitor/self_heal_observer.go`
- **What**: Implement observer that subscribes to FlowLog events, tracks auto-heal success/failure, fires alerts for repeated failures and unhealed errors. Replace current SelfHealUsecase.
- **Spec**: REQ-SO-01 through REQ-SO-04, REQ-SO-08
- **Test**: Unit test with mock EventBus, verify alert firing logic

### Task 2.4: Heal Stats + ListHealRecords API
- **File**: `api/kratos/monitor/v1/monitor.proto`, `internal/service/monitor.go`
- **What**: Add `GetHealStats` and `ListHealRecords` RPCs. Implement service layer. Run `make api`.
- **Spec**: REQ-SO-06, REQ-SO-07
- **Test**: Service layer unit test

### Task 2.5: Wire integration
- **File**: `cmd/admin/wire.go`
- **What**: Wire SelfHealObserver, HealRecordRepo, and update MonitorService constructor. Replace SelfHealUsecase with SelfHealObserver.
- **Spec**: All REQ-SO
- **Test**: `make wire && go build ./cmd/admin`

## Phase 3: RootCauseEngine Enhancement

### Task 3.1: AutoHealed + HealAttempts dimensions
- **File**: `internal/biz/monitor/root_cause_engine.go`
- **What**: Add `AutoHealed *bool` and `HealAttempts int` to RootCauseCondition. Update matchRule to check these fields. Add `rc-repeated-auto-heal-failure` built-in rule. Update existing rules with `AutoHealed: boolPtr(false)`.
- **Spec**: REQ-RC-01 through REQ-RC-04
- **Test**: Unit test for new condition matching

### Task 3.2: Severity-based cooldown
- **File**: `internal/biz/monitor/self_heal_observer.go`
- **What**: Replace global 5-minute cooldown with severity-based cooldown map. Update checkCooldown to use rule severity.
- **Spec**: REQ-RC-05
- **Test**: Unit test for different cooldown durations

### Task 3.3: AddRules validation
- **File**: `internal/biz/monitor/root_cause_engine.go`
- **What**: AddRules returns error for invalid rules (empty ID/Name, bad regex) instead of silently skipping.
- **Spec**: REQ-RC-06
- **Test**: Unit test for validation errors

## Phase 4: DiagBundle Enhancement

### Task 4.1: Auto-heal metadata in bundle
- **File**: `internal/biz/monitor/diag_bundle.go`
- **What**: Parse auto_healed/heal_strategy/heal_attempts/heal_success from flow entry metadata. Include in bundle output. Add self_heal_summary to manifest.
- **Spec**: REQ-DB-01, REQ-DB-02
- **Test**: Unit test with mock data containing auto_heal metadata

### Task 4.2: Runtime heal state in RootCauseResult
- **File**: `internal/biz/monitor/root_cause_engine.go`, `internal/biz/monitor/diag_bundle.go`
- **What**: Add RuntimeAutoHealed and RuntimeHealAttempts to RootCauseResult. Populate from trigger metadata.
- **Spec**: REQ-DB-03
- **Test**: Unit test

### Task 4.3: read_flow_logs tool enhancement
- **File**: `internal/tools/custom/tool_read_flow_logs.go`
- **What**: Add auto_healed, heal_strategy, heal_attempts, heal_success fields to flowLogEntry output.
- **Spec**: REQ-DB-01
- **Test**: Unit test with mock FlowLogUsecase

## Phase 5: Migration + Cleanup

### Task 5.1: Migrate SelfHealUsecase → SelfHealObserver
- **File**: `internal/biz/monitor/self_heal.go`, `internal/service/monitor.go`
- **What**: Update DiagnoseAndHeal API to return observed_healed/observed_failed status. Deprecate SelfHealUsecase (keep for backward compat during transition).
- **Test**: Integration test

### Task 5.2: Remove DefaultHealActionHandler
- **File**: `internal/biz/monitor/heal_action_handler.go`
- **What**: Remove DefaultHealActionHandler (no longer needed, runtime handles healing). Update Wire providers.
- **Test**: Build verification
