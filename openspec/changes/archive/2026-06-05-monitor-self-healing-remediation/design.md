## Context

原 monitor-self-healing 变更归档后审计发现 12 项缺口。circuit breaker 实现为"连续 5 次"而非"10 分钟内 5 次"，缺少 30 分钟重置和事件发射。

## Goals / Non-Goals

**Goals:**
- 修正 circuit breaker 为滑动时间窗口（10min 内 5 次）
- 添加 30 分钟重置 + `heal_circuit_open` 事件发射
- 补齐 TTL cleanup cron
- 补齐 4 项单元测试

**Non-Goals:**
- Phase 1.2/1.4 框架层集成（需 trpc-agent-go 支持）

## Decisions

### D1: Circuit breaker 实现

**决策**: 用 `map[string][]time.Time` 按 stepID 维护滑动时间窗口，记录最近 10 分钟的 heal 事件时间戳，超过 5 次触发 circuit open。通过 severity-dependent cooldown 机制实现 30 分钟自动重置（critical severity cooldown = 30min）。

**实际实现**:
- 位置：`internal/biz/monitor/self_heal_observer.go`（非原计划的 `internal/data/monitor.go`）
- 数据结构：`healEvents map[string][]time.Time`（按 stepID 分组的时间戳切片），每次 `ObserveFlowLogEvent` 处理 auto-heal 失败时追加时间戳并裁剪窗口外条目
- 常量：`CircuitBreakerWindow = 10 * time.Minute`，`CircuitBreakerThreshold = 5`，`CircuitBreakerResetAfter = 30 * time.Minute`
- 触发逻辑：`countInWindow >= CircuitBreakerThreshold` 时调用 `fireCircuitOpenAlert()`
- 事件发射：`fireCircuitOpenAlert()` 通过 `AlertNotifier.Notify()` 发射告警通知，同时持久化 `HealRecord`（trigger_type=circuit_breaker_open, status=alert_fired），日志 stepID 为 `monitor.heal_circuit_open`
- 30 分钟重置：未实现显式 half-open 状态机，而是通过 cooldown 机制等效实现——circuit open 后 cooldown 期间不会重复告警，30 分钟后 cooldown 过期可再次触发
- **与原设计差异**：原设计为"环形缓冲区"，实际实现为按 stepID 分组的时间戳切片滑动窗口

### D2: TTL cleanup cron

**决策**: 每小时扫描 `heal_records` 表，删除 `created_at < now - TTL` 的记录。

**实际实现**:
- 位置：`internal/cronrunner/jobs/auto_heal_ttl_cleanup.go`
- 结构体：`AutoHealTTLCleanup`，通过 `NewAutoHealTTLCleanup()` 构造
- 配置：`AUTO_HEAL_TTL_INTERVAL`（默认 1h）、`AUTO_HEAL_TTL_MAX_AGE`（默认 72h）
- 清理逻辑：调用 `HealRecordRepo.DeleteHealRecordsOlderThan(ctx, cutoff)`，删除 `created_at < cutoff` 的所有记录
- **与原设计差异**：原设计要求"仅清理已 resolved 的事件"，实际实现删除所有超过 TTL 的记录（不过滤 status）。原设计引用的表名为 `auto_heal_events`，实际操作的是 `heal_records` 表

### D3: SelfHealObserver 架构

**补充说明**: 代码中 `SelfHealUsecase` 已标记为 Deprecated，由 `SelfHealObserver` 替代观察角色。`SelfHealObserver` 的职责：
- REQ-SO-01: 监听 flow_phase=error 事件
- REQ-SO-02: auto_healed=true + heal_success=true → observed_healed
- REQ-SO-03: auto_healed=true + heal_success=false → observed_failed，滑动窗口计数，超阈值触发 circuit open
- REQ-SO-04: auto_healed=false + phase=error → root cause analysis，高置信度触发告警
- REQ-SO-08: Cooldown 按 severity 分级（critical=30min, high=10min, medium=5min, low=2min）

## Risks / Trade-offs

- **[Risk] 滑动窗口内存占用** → 按 stepID 分组的时间戳切片，每个 stepID 最多保留窗口内条目（最多 5 条即触发 circuit open），内存可控
- **[Risk] TTL cleanup 可能影响审计** → 当前实现删除所有超 TTL 记录（不过滤 status），审计记录可能被清理。如需仅清理已 resolved 记录，需在 `DeleteHealRecordsOlderThan` 中添加 status 过滤条件
- **[Risk] 无显式 half-open 状态** → 30 分钟重置通过 cooldown 间接实现，无独立状态机。当前行为等效于：circuit open 后 30 分钟内不重复告警，30 分钟后可再次触发。如需严格的 closed → open → half-open → closed 状态机，需额外实现
