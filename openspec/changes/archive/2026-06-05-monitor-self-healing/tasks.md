## Phase 1: Runtime Auto-Heal Foundation

### Task 1.1: Define AutoHealStrategy interface
- **File**: `pkg/trpc-agent-go/internal/flow/processor/autoheal.go`
- **What**: Define `AutoHealStrategy` interface with `CanHeal`, `Heal`, `MaxAttempts`, `Backoff` methods. Define `AutoHealConfig` struct and `WithAutoHealConfig` RunOption.
- **Spec**: REQ-RH-01, REQ-RH-06
- **Test**: Unit test for interface compliance and config defaults
- [x] `AutoHealStrategy` 接口已定义（CanHeal/Heal/MaxAttempts/Backoff）
- [x] `AutoHealConfig` struct 已定义，`DefaultAutoHealConfig()` 已提供
- [x] `AutoHealResult` struct 和 `ExecuteAutoHeal` 辅助函数已实现（额外）
- [ ] `WithAutoHealConfig` RunOption 未实现（低优先级，运行时可通过配置注入）
- [ ] 单元测试未编写

### Task 1.2: LLM call auto-heal (retry + fallback)
- **File**: `pkg/trpc-agent-go/internal/flow/llmflow/llmflow.go`
- **What**: Integrate AutoHealStrategy into LLM flow. On transient errors (timeout/429/context-exceeded), execute retry with backoff. On context-exceeded, attempt compress-and-retry. Emit FlowLog with auto_healed metadata.
- **Spec**: REQ-RH-02, REQ-RH-05
- **Test**: Unit test with mock LLM returning transient errors, verify retry behavior and event emission
- [ ] 未实现。llmflow.go 中无任何 AutoHealStrategy/AutoHealConfig/ExecuteAutoHeal 引用，无 auto-heal 集成逻辑
- **Note**: 涉及 trpc-agent-go 框架层改动，风险较高，建议在独立迭代中完成

### Task 1.3: MCP connection auto-reconnect
- **File**: `pkg/trpc-agent-go/tool/mcp/` (原指定 `mcp/mcpbroker/` 不存在)
- **What**: Add reconnection logic to MCP broker. On connection drop, attempt reconnect up to 3 times with exponential backoff. Emit FlowLog with auto_healed metadata.
- **Spec**: REQ-RH-03, REQ-RH-05
- **Test**: Integration test with mock MCP server that drops connection
- [x] MCP 重连逻辑已实现，位于 `pkg/trpc-agent-go/tool/mcp/`
- [x] `toolset.go` 中 `executeWithSessionReconnect` 方法支持最多 N 次重连（默认3次），使用 singleflight 防并发
- [x] `config.go` 中已有 `SessionReconnectConfig`、`ReconnectObserver`、`WithSessionReconnect` 等配置
- [ ] 未发射带 `auto_healed` metadata 的 FlowLog（使用 `ReconnectObserver` 回调机制替代）
- [ ] 集成测试未编写

### Task 1.4: Tool execution auto-retry
- **File**: `pkg/trpc-agent-go/internal/flow/processor/functioncall.go`
- **What**: Add retry logic for transient tool execution failures. Non-transient errors skip retry. Emit FlowLog with auto_healed metadata.
- **Spec**: REQ-RH-04, REQ-RH-05
- **Test**: Unit test with mock tool returning transient vs non-transient errors
- [ ] AutoHealStrategy 集成未实现。`functioncall.go` 中已有框架原有的 `toolretry` 集成（`executeCallableTool` 使用 `toolretry.Execute`），但未集成 AutoHealStrategy，未发射 auto_healed FlowLog
- **Note**: 涉及 trpc-agent-go 框架层改动，风险较高，建议在独立迭代中完成

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
- [x] `heal_record_repo.go` 已实现，包含 InsertHealRecord/ListHealRecords/DeleteHealRecordsOlderThan
- [x] 使用 raw SQL 实现（非 Ent 生成代码，但接口完整）
- [ ] TTL cleanup cron job 未实现

### Task 2.3: SelfHealObserver
- **File**: `internal/biz/monitor/self_heal_observer.go`
- **What**: Implement observer that subscribes to FlowLog events, tracks auto-heal success/failure, fires alerts for repeated failures and unhealed errors. Replace current SelfHealUsecase.
- **Spec**: REQ-SO-01 through REQ-SO-04, REQ-SO-08
- **Test**: Unit test with mock EventBus, verify alert firing logic
- [x] `SelfHealObserver` 已实现，包含 ObserveFlowLogEvent/StartEventDrivenObservation/GetHealStats/ListHealRecords
- [x] REQ-SO-01: 处理 flow_phase=error 事件
- [x] REQ-SO-02: auto_healed=true + heal_success=true → observed_healed
- [x] REQ-SO-03: auto_healed=true + heal_success=false → observed_failed, 3+连续失败触发告警
- [x] REQ-SO-04: auto_healed=false → 根因分析 + 告警
- [x] REQ-SO-08: 基于严重程度的冷却期
- [x] `DiagnoseAndObserve` 方法已实现（同步诊断+观测，替代 SelfHealUsecase.DiagnoseAndHeal）
- [ ] 单元测试未编写

### Task 2.4: Heal Stats + ListHealRecords API
- **File**: `api/kratos/monitor/v1/monitor.proto`, `internal/service/monitor.go`
- **What**: Add `GetHealStats` and `ListHealRecords` RPCs. Implement service layer. Run `make api`.
- **Spec**: REQ-SO-06, REQ-SO-07
- **Test**: Service layer unit test
- [x] `GetHealStats` RPC 已实现（MonitorService.GetHealStats → SelfHealObserver.GetHealStats）
- [x] `ListHealRecords` RPC 已实现（MonitorService.ListHealRecords → SelfHealObserver.ListHealRecords）
- [x] Proto 定义和 Service 层代码完整

### Task 2.5: Wire integration
- **File**: `cmd/admin/wire.go`
- **What**: Wire SelfHealObserver, HealRecordRepo, and update MonitorService constructor. Replace SelfHealUsecase with SelfHealObserver.
- **Spec**: All REQ-SO
- **Test**: `make wire && go build ./cmd/admin`
- [x] `provideSelfHealObserver` 已在 wire.go 中注册
- [x] `NewHealRecordRepo` 已在 data provider set 中
- [x] `provideRootCauseEngine` 已在 wire.go 中注册
- [x] MonitorService 构造函数已包含 selfHealObserver 参数
- [x] wire_gen.go 已重新生成，构建通过

## Phase 3: RootCauseEngine Enhancement

### Task 3.1: AutoHealed + HealAttempts dimensions
- **File**: `internal/biz/monitor/root_cause_engine.go`
- **What**: Add `AutoHealed *bool` and `HealAttempts int` to RootCauseCondition. Update matchRule to check these fields. Add `rc-repeated-auto-heal-failure` built-in rule. Update existing rules with `AutoHealed: boolPtr(false)`.
- **Spec**: REQ-RC-01 through REQ-RC-04
- **Test**: Unit test for new condition matching
- [x] `RootCauseCondition` 已包含 `AutoHealed *bool` 和 `HealAttempts int` 字段
- [x] `matchRule` 已检查 AutoHealed 和 HealAttempts 维度
- [x] `rc-repeated-auto-heal-failure` 内置规则已添加（AutoHealed=true, HealAttempts=3）
- [x] `RootCauseResult` 已包含 `RuntimeAutoHealed` 和 `RuntimeHealAttempts` 字段
- [x] `Evaluate` 方法已从 metadata 填充 RuntimeAutoHealed/RuntimeHealAttempts

### Task 3.2: Severity-based cooldown
- **File**: `internal/biz/monitor/self_heal_observer.go`
- **What**: Replace global 5-minute cooldown with severity-based cooldown map. Update checkCooldown to use rule severity.
- **Spec**: REQ-RC-05
- **Test**: Unit test for different cooldown durations
- [x] `SeverityCooldown` 映射已定义（critical=30min, high=10min, medium=5min, low=2min）
- [x] `checkCooldown` 方法已使用 severity 参数查找对应冷却时长
- [x] 默认冷却时长为 medium（5min）

### Task 3.3: AddRules validation
- **File**: `internal/biz/monitor/root_cause_engine.go`
- **What**: AddRules returns error for invalid rules (empty ID/Name, bad regex) instead of silently skipping.
- **Spec**: REQ-RC-06
- **Test**: Unit test for validation errors
- [x] `AddRules` 已返回 error（签名 `func (e *RootCauseEngine) AddRules(rules []RootCauseRule) error`）
- [x] 空 ID 校验：`addRules: rule at index %d has empty ID`
- [x] 空 Name 校验：`addRules: rule %q has empty Name`
- [x] 重复 ID 校验：`addRules: rule %q has duplicate ID`
- [x] 无效 regex 校验：`addRules: rule %q has invalid regex %q: %w`

## Phase 4: DiagBundle Enhancement

### Task 4.1: Auto-heal metadata in bundle
- **File**: `internal/biz/monitor/diag_bundle.go`
- **What**: Parse auto_healed/heal_strategy/heal_attempts/heal_success from flow entry metadata. Include in bundle output. Add self_heal_summary to manifest.
- **Spec**: REQ-DB-01, REQ-DB-02
- **Test**: Unit test with mock data containing auto_heal metadata
- [x] `Generate` 方法已解析 flow entries 中的 auto_healed/heal_success 元数据
- [x] `manifest["self_heal_summary"]` 已包含 auto_heal_count/heal_success_count/heal_fail_count

### Task 4.2: Runtime heal state in RootCauseResult
- **File**: `internal/biz/monitor/root_cause_engine.go`, `internal/biz/monitor/diag_bundle.go`
- **What**: Add RuntimeAutoHealed and RuntimeHealAttempts to RootCauseResult. Populate from trigger metadata.
- **Spec**: REQ-DB-03
- **Test**: Unit test
- [x] `RootCauseResult` 已包含 `RuntimeAutoHealed bool` 和 `RuntimeHealAttempts int` 字段
- [x] `Evaluate` 方法已从 metadata 填充这些字段

### Task 4.3: read_flow_logs tool enhancement
- **File**: `internal/tools/custom/tool_read_flow_logs.go`
- **What**: Add auto_healed, heal_strategy, heal_attempts, heal_success fields to flowLogEntry output.
- **Spec**: REQ-DB-01
- **Test**: Unit test with mock FlowLogUsecase
- [x] `flowLogEntry` struct 已包含 AutoHealed/HealStrategy/HealAttempts/HealSuccess 字段
- [x] 从 PayloadJSON 解析 auto_healed/heal_strategy/heal_attempts/heal_success

## Phase 5: Migration + Cleanup

### Task 5.1: Migrate SelfHealUsecase → SelfHealObserver
- **File**: `internal/biz/monitor/self_heal.go`, `internal/service/monitor.go`
- **What**: Update DiagnoseAndHeal API to return observed_healed/observed_failed status. Deprecate SelfHealUsecase (keep for backward compat during transition).
- **Test**: Integration test
- [x] `SelfHealUsecase` 已标记 Deprecated
- [x] `SelfHealObserver.DiagnoseAndObserve` 已实现（同步诊断+观测）
- [x] `MonitorService.DiagnoseAndHeal` 已优先使用 SelfHealObserver，SelfHealUsecase 作为 fallback
- [x] 返回 observed_healed/observed_failed 状态

### Task 5.2: Remove DefaultHealActionHandler
- **File**: `internal/biz/monitor/heal_action_handler.go`
- **What**: Remove DefaultHealActionHandler (no longer needed, runtime handles healing). Update Wire providers.
- **Test**: Build verification
- [x] `heal_action_handler.go` 已删除
- [x] `provideSelfHealUsecase` 已更新为传入 nil handler
- [x] `NewSelfHealUsecase` 已允许 nil handler（handler 为 nil 时跳过修复，返回 observed_failed）
- [x] Wire 重新生成，构建通过
