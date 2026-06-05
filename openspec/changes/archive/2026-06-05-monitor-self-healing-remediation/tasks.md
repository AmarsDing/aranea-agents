# Monitor-self-healing 审计修复 — 任务清单

**Goal**: 补齐原 monitor-self-healing 变更的 12 项缺口。

**Non-goals:**
- Phase 1.2/1.4 框架层集成（需 trpc-agent-go 支持，独立迭代）

---

## 1. Circuit breaker 修正

- [x] 1.1 修改 `internal/biz/monitor/self_heal_observer.go` 中 circuit breaker 逻辑：从"连续 5 次"改为"10 分钟滑动窗口内 5 次"。DoD: `SelfHealObserver.healEvents` map 按 stepID 维护滑动时间窗口，常量 `CircuitBreakerWindow=10min` / `CircuitBreakerThreshold=5`。**注：实际实现位于 biz 层 `self_heal_observer.go`，非 data 层 `monitor.go`。**
- [x] 1.2 添加 30 分钟自动重置逻辑。DoD: 常量 `CircuitBreakerResetAfter=30min` 已定义，通过 severity-dependent cooldown 机制间接实现——circuit open 后 30 分钟 cooldown 过期即可再次触发告警。**注：未实现显式 half-open 状态机，而是通过 cooldown 机制等效实现。**
- [x] 1.3 添加 `heal_circuit_open` 事件发射。DoD: `fireCircuitOpenAlert()` 方法通过 `AlertNotifier.Notify()` 发射告警，同时持久化 `HealRecord`（trigger_type=circuit_breaker_open），日志 stepID 为 `monitor.heal_circuit_open`。**注：未通过 FlowLog 独立发射事件，而是通过 HealRecord 持久化 + AlertNotifier 通知。**
- [x] 1.4 编写 `internal/biz/monitor/self_heal_observer_test.go` 单元测试（覆盖 circuit breaker 滑动窗口、cooldown、auto_heal 成功/失败、根因分析等场景）。DoD: `go test ./internal/biz/monitor/... -run TestSelfHealObserver -count=1` 绿色

## 2. TTL cleanup cron

- [x] 2.1 创建 `internal/cronrunner/jobs/auto_heal_ttl_cleanup.go`：每小时清理超过 TTL 的 heal_records 记录。DoD: `go build ./internal/cronrunner/...` 通过。**注：实际操作的是 `heal_records` 表（非 `auto_heal_events`），`DeleteHealRecordsOlderThan` 删除 `created_at < cutoff` 的所有记录，未过滤 `status=resolved`。**
- [x] 2.2 编写单元测试。DoD: `go test ./internal/cronrunner/... -run TestAutoHealTTLCleanup -count=1` 绿色

## 3. 补齐单元测试

- [x] 3.1 编写 Phase 1.1 单元测试（`AutoHealConfig` + `ComputeBackoff` + `HealCircuitBreaker`）。DoD: `go test ./internal/flow/processor/... -run "TestDefaultAutoHealConfig|TestComputeBackoff|TestHealCircuitBreaker" -count=1` 绿色。**注：`WithAutoHealConfig` RunOption 尚未实现，测试范围调整为已实现的 AutoHealConfig/ComputeBackoff/HealCircuitBreaker。**
- [x] 3.2 编写 Phase 1.3 集成测试（auto_healed FlowLog 发射）。DoD: 已由 `self_heal_observer_test.go` 中的 `TestSelfHealObserver_AutoHealSuccess` 和 `TestSelfHealObserver_AutoHealFailure_SlidingWindow` 覆盖。**注：MCP 重连使用 ReconnectObserver 回调而非直接发射 FlowLog，测试聚焦于 SelfHealObserver 接收端处理逻辑。**
- [x] 3.3 编写 Phase 2.3 SelfHealObserver 单元测试。DoD: `go test ./internal/biz/monitor/... -run TestSelfHealObserver -count=1` 绿色

## 4. 全量验证

- [x] 4.1 运行 `make build && make test && make lint`。DoD: 全部通过
