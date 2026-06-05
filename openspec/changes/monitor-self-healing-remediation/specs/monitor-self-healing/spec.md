## MODIFIED Requirements

### Requirement: Circuit breaker 触发条件
原为"连续 5 次失败触发 circuit open"。现改为"10 分钟内 5 次 auto-heal 事件触发 circuit open"，并通过 cooldown 机制实现 30 分钟自动重置。

#### Scenario: 滑动时间窗口触发
- **WHEN** 10 分钟内同一 stepID 发生 5 次 auto-heal 失败
- **THEN** circuit breaker 触发，调用 `fireCircuitOpenAlert()` 发射告警
- **实现**: `SelfHealObserver.healEvents` map 按 stepID 维护时间戳切片，每次事件追加并裁剪窗口外条目，`countInWindow >= CircuitBreakerThreshold` 时触发

#### Scenario: 自动重置（cooldown 等效实现）
- **WHEN** circuit open 后 30 分钟内无新 auto-heal 失败事件
- **THEN** cooldown 过期，circuit breaker 可再次触发告警
- **实现**: `fireCircuitOpenAlert()` 设置 `cooldowns[ruleID] = now`，`checkCooldown()` 检查 `time.Since(last) > GetSeverityCooldown("critical")`（=30min）。未实现显式 half-open 状态机，cooldown 过期等效于 half-open

#### Scenario: 事件发射
- **WHEN** circuit breaker 触发
- **THEN** 通过 `AlertNotifier.Notify()` 发射告警，持久化 `HealRecord`（trigger_type=circuit_breaker_open），日志 stepID 为 `monitor.heal_circuit_open`
- **实现**: `fireCircuitOpenAlert()` 方法，通知 payload 包含 `circuit_breaker=true`、`window`、`threshold`、`auto_reset_after` 字段

## ADDED Requirements

### Requirement: TTL cleanup cron
系统 SHALL 实现定时任务，每小时清理超过 TTL 的 `heal_records` 记录。

#### Scenario: 定时清理
- **WHEN** cron 触发
- **THEN** 删除 `created_at < now - TTL` 的记录（默认 TTL = 72h）
- **实现**: `AutoHealTTLCleanup` 结构体，调用 `HealRecordRepo.DeleteHealRecordsOlderThan(ctx, cutoff)`
- **配置**: `AUTO_HEAL_TTL_INTERVAL`（默认 1h）、`AUTO_HEAL_TTL_MAX_AGE`（默认 72h）
- **注**: 当前实现删除所有超 TTL 记录，不过滤 status。如需仅清理已 resolved 记录，需添加 status 过滤

### Requirement: SelfHealObserver 观察角色
`SelfHealUsecase` 已 Deprecated，由 `SelfHealObserver` 替代观察角色。

#### Scenario: 观察运行时 auto-heal 成功
- **WHEN** FlowLog 事件 `auto_healed=true` 且 `heal_success=true`
- **THEN** 记录 `HealRecord`（status=observed_healed），清除该 stepID 的滑动窗口计数

#### Scenario: 观察运行时 auto-heal 失败
- **WHEN** FlowLog 事件 `auto_healed=true` 且 `heal_success=false`
- **THEN** 记录 `HealRecord`（status=observed_failed），追加滑动窗口时间戳，超阈值触发 circuit open

#### Scenario: 未 auto-heal 的错误事件
- **WHEN** FlowLog 事件 `auto_healed=false` 且 `flow_phase=error`
- **THEN** 运行 root cause analysis，高置信度（≥0.7）触发告警

#### Scenario: Severity-dependent cooldown
- **WHEN** 告警触发后
- **THEN** 按 severity 设置 cooldown（critical=30min, high=10min, medium=5min, low=2min），cooldown 期间不重复告警
