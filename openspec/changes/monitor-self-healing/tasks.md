## Phase 1: Runtime Auto-Heal Foundation

### Task 1.1: Define AutoHealStrategy interface
- **File**: `pkg/trpc-agent-go/internal/flow/processor/autoheal.go`
- **What**: Define `AutoHealStrategy` interface with `CanHeal`, `Heal`, `MaxAttempts`, `Backoff` methods. Define `AutoHealConfig` struct and `WithAutoHealConfig` RunOption.
- **Spec**: REQ-RH-01, REQ-RH-06
- **Test**: Unit test for interface compliance and config defaults
- [x] `AutoHealStrategy` 接口已定义（CanHeal/Heal/MaxAttempts/Backoff）
- [x] `AutoHealConfig` struct 已定义，`DefaultAutoHealConfig()` 已提供
- [x] `AutoHealResult` struct 和 `ExecuteAutoHeal` 辅助函数已实现（额外）
- [ ] `WithAutoHealConfig` RunOption 未实现
- [ ] 单元测试未编写

### Task 1.2: LLM call auto-heal (retry + fallback)
- **File**: `pkg/trpc-agent-go/internal/flow/llmflow/llmflow.go`
- **What**: Integrate AutoHealStrategy into LLM flow. On transient errors (timeout/429/context-exceeded), execute retry with backoff. On context-exceeded, attempt compress-and-retry. Emit FlowLog with auto_healed metadata.
- **Spec**: REQ-RH-02, REQ-RH-05
- **Test**: Unit test with mock LLM returning transient errors, verify retry behavior and event emission
- [ ] 未实现。llmflow.go 中无任何 AutoHealStrategy/AutoHealConfig/ExecuteAutoHeal 引用，无 auto-heal 集成逻辑

### Task 1.3: MCP connection auto-reconnect
- **File**: `pkg/trpc-agent-go/mcp/mcpbroker/`
- **What**: Add reconnection logic to MCP broker. On connection drop, attempt reconnect up to 3 times with exponential backoff. Emit FlowLog with auto_healed metadata.
- **Spec**: REQ-RH-03, REQ-RH-05
- **Test**: Integration test with mock MCP server that drops connection
- [x] MCP 重连逻辑已实现，但位于 `pkg/trpc-agent-go/tool/mcp/` 而非 `mcp/mcpbroker/`
- [x] `toolset.go` 中 `executeWithSessionReconnect` 方法支持最多 N 次重连（默认3次），使用 singleflight 防并发
- [x] `config.go` 中已有 `SessionReconnectConfig`、`ReconnectObserver`、`WithSessionReconnect` 等配置
- [ ] 未发射带 `auto_healed` metadata 的 FlowLog（使用 `ReconnectObserver` 回调机制替代）
- [ ] 任务指定路径 `mcp/mcpbroker/` 不存在，实际在 `tool/mcp/`
- [ ] 集成测试未编写

### Task 1.4: Tool execution auto-retry
- **File**: `pkg/trpc-agent-go/internal/flow/processor/functioncall.go`
- **What**: Add retry logic for transient tool execution failures. Non-transient errors skip retry. Emit FlowLog with auto_healed metadata.
- **Spec**: REQ-RH-04, REQ-RH-05
- **Test**: Unit test with mock tool returning transient vs non-transient errors
- [ ] AutoHealStrategy 集成未实现。`functioncall.go` 中已有框架原有的 `toolretry` 集成（`executeCallableTool` 使用 `toolretry.Execute`），但未集成 AutoHealStrategy，未发射 auto_healed FlowLog

### Task 1.5: Heal circuit breaker
- **File**: `pkg/trpc-agent-go/internal/flow/processor/autoheal.go`
- **What**: Implement circuit breaker that stops auto-heal after 5 consecutive failures within 10 minutes. Emit `heal_circuit_open` event. Reset after 30 minutes.
- **Spec**: REQ-RH-07
- **Test**: Unit test for circuit open/close behavior
- [x] `HealCircuitBreaker` struct已实现（NewHealCircuitBreaker/RecordSuccess/RecordFailure/IsOpen）
- [ ] 断路器仅检查 `maxConsecutiveFailures`，**无时间窗口限制**（任务要求"10分钟内5次"，实际为"连续5次"）
- [ ] **无 `heal_circuit_open` 事件发射逻辑**
- [ ] 重置时长由调用方参数决定，**无默认30分钟**
- [ ] 单元测试未编写

## Phase 2: Monitor Observer

### Task 2.1: HealRecord Ent Schema
- **File**: `internal/data/ent/schema/heal_record.go`
- **What**: Define Ent schema with fields: id, rule_id, trigger_type, trace_id, session_id, step_id, fix_action_type, confidence, status, runtime_auto_healed, runtime_heal_attempts, reason, created_at. Run `go generate`.
- **Spec**: REQ-SO-05
- **Test**: Verify schema generates correctly
- [x] Schema 定义完整，包含所有任务要求的字段和索引

### Task 2.2: HealRecordRepo
- **File**: `internal/data/heal_record_repo.go`
- **What**: Implement `HealRecordRepo` interface (Insert, List, DeleteOlderThan) using Ent. Add TTL cleanup cron job.
- **Spec**: REQ-SO-05
- **Test**: Unit test with in-memory SQLite
- [ ] 未实现。文件不存在。当前 `SelfHealUsecase` 使用内存 `[]HealRecord` 切片存储，而非 Ent 持久化

### Task 2.3: SelfHealObserver
- **File**: `internal/biz/monitor/self_heal_observer.go`
- **What**: Implement observer that subscribes to FlowLog events, tracks auto-heal success/failure, fires alerts for repeated failures and unhealed errors. Replace current SelfHealUsecase.
- **Spec**: REQ-SO-01 through REQ-SO-04, REQ-SO-08
- **Test**: Unit test with mock EventBus, verify alert firing logic
- [ ] 未实现。文件不存在。当前仍使用 `SelfHealUsecase`

### Task 2.4: Heal Stats + ListHealRecords API
- **File**: `api/kratos/monitor/v1/monitor.proto`, `internal/service/monitor.go`
- **What**: Add `GetHealStats` and `ListHealRecords` RPCs. Implement service layer. Run `make api`.
- **Spec**: REQ-SO-06, REQ-SO-07
- **Test**: Service layer unit test
- [ ] 未实现。Proto 中无 GetHealStats/ListHealRecords RPC，Service 层无相关代码

### Task 2.5: Wire integration
- **File**: `cmd/admin/wire.go`
- **What**: Wire SelfHealObserver, HealRecordRepo, and update MonitorService constructor. Replace SelfHealUsecase with SelfHealObserver.
- **Spec**: All REQ-SO
- **Test**: `make wire && go build ./cmd/admin`
- [ ] 未实现。Wire 中仍使用 `provideSelfHealUsecase`，无 SelfHealObserver/HealRecordRepo 注入

## Phase 3: RootCauseEngine Enhancement

### Task 3.1: AutoHealed + HealAttempts dimensions
- **File**: `internal/biz/monitor/root_cause_engine.go`
- **What**: Add `AutoHealed *bool` and `HealAttempts int` to RootCauseCondition. Update matchRule to check these fields. Add `rc-repeated-auto-heal-failure` built-in rule. Update existing rules with `AutoHealed: boolPtr(false)`.
- **Spec**: REQ-RC-01 through REQ-RC-04
- **Test**: Unit test for new condition matching
- [ ] 未实现。RootCauseCondition struct 无 AutoHealed/HealAttempts 字段，matchRule 不检查这些维度，无 rc-repeated-auto-heal-failure 规则

### Task 3.2: Severity-based cooldown
- **File**: `internal/biz/monitor/self_heal_observer.go`
- **What**: Replace global 5-minute cooldown with severity-based cooldown map. Update checkCooldown to use rule severity.
- **Spec**: REQ-RC-05
- **Test**: Unit test for different cooldown durations
- [ ] 未实现。当前 SelfHealUsecase 使用全局 5 分钟冷却（SelfHealCooldownSec = 300），无基于严重程度的冷却映射

### Task 3.3: AddRules validation
- **File**: `internal/biz/monitor/root_cause_engine.go`
- **What**: AddRules returns error for invalid rules (empty ID/Name, bad regex) instead of silently skipping.
- **Spec**: REQ-RC-06
- **Test**: Unit test for validation errors
- [ ] 未实现。当前 AddRules 对无效规则（重复 ID、regex 编译失败）静默跳过（continue），函数签名不返回 error

## Phase 4: DiagBundle Enhancement

### Task 4.1: Auto-heal metadata in bundle
- **File**: `internal/biz/monitor/diag_bundle.go`
- **What**: Parse auto_healed/heal_strategy/heal_attempts/heal_success from flow entry metadata. Include in bundle output. Add self_heal_summary to manifest.
- **Spec**: REQ-DB-01, REQ-DB-02
- **Test**: Unit test with mock data containing auto_heal metadata
- [ ] 未实现。DiagBundle 中无 auto_healed/heal_strategy/heal_attempts/heal_success/self_heal_summary 相关代码

### Task 4.2: Runtime heal state in RootCauseResult
- **File**: `internal/biz/monitor/root_cause_engine.go`, `internal/biz/monitor/diag_bundle.go`
- **What**: Add RuntimeAutoHealed and RuntimeHealAttempts to RootCauseResult. Populate from trigger metadata.
- **Spec**: REQ-DB-03
- **Test**: Unit test
- [ ] 未实现。RootCauseResult 无 RuntimeAutoHealed/RuntimeHealAttempts 字段

### Task 4.3: read_flow_logs tool enhancement
- **File**: `internal/tools/custom/tool_read_flow_logs.go`
- **What**: Add auto_healed, heal_strategy, heal_attempts, heal_success fields to flowLogEntry output.
- **Spec**: REQ-DB-01
- **Test**: Unit test with mock FlowLogUsecase
- [ ] 未实现。flowLogEntry struct 无 auto_healed/heal_strategy/heal_attempts/heal_success 字段

## Phase 5: Migration + Cleanup

### Task 5.1: Migrate SelfHealUsecase → SelfHealObserver
- **File**: `internal/biz/monitor/self_heal.go`, `internal/service/monitor.go`
- **What**: Update DiagnoseAndHeal API to return observed_healed/observed_failed status. Deprecate SelfHealUsecase (keep for backward compat during transition).
- **Test**: Integration test
- [ ] 未实现。SelfHealUsecase 仍活跃，无 deprecated 标记，DiagnoseAndHeal 未返回 observed_healed/observed_failed 状态

### Task 5.2: Remove DefaultHealActionHandler
- **File**: `internal/biz/monitor/heal_action_handler.go`
- **What**: Remove DefaultHealActionHandler (no longer needed, runtime handles healing). Update Wire providers.
- **Test**: Build verification
- [ ] 未实现。DefaultHealActionHandler 仍存在且完整实现，Wire 中仍通过 NewDefaultHealActionHandler 注入
