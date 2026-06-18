# ADR-01: DB 事务重试与 RunRegistry TOCTOU 修复

## 状态：已接受

## 背景

完整消息链审查报告（`2026-06-18-review-full-message-chain-and-solutions.md`）发现两个可靠性问题：

1. **问题 11（DB 事务超时无重试）**：`ExecInTx` 使用 30s 硬超时作为安全网防止死锁，但超时后直接返回错误，无自动重试。在 No-Timeout 架构下（任务持续运行直到完成或用户取消），DB 瞬态故障（死锁、序列化冲突、连接超时）不应导致长任务失败。

2. **问题 12（RunRegistry TOCTOU 风险）**：`RunRegistry.StoreRunner` 和 `StoreCancelable` 使用 `load` + `store` 两步操作，存在 TOCTOU（Time-of-Check-Time-of-Use）竞态：两个并发 goroutine 可能同时 load 到旧值，然后各自 store，导致后一个 store 覆盖前一个的数据（如丢失 cancel func 或 runner 引用）。

## 决策

### D1: DB 事务重试包装器（T2.1）

新增 `Data.ExecInTxWithRetry(ctx, fn)` 方法，包装 `ExecInTx`：

- **重试策略**：最多 3 次重试（共 4 次尝试），指数退避 1s → 2s → 4s
- **可重试错误**：`apierror.CodeInternal`（未知 DB 错误）、Postgres `deadlock_detected`(40P01)、`serialization_failure`(40001)、`context.DeadlineExceeded`（tx 超时，非 caller 取消）
- **不可重试错误**：`context.Canceled`（caller 主动取消）、`apierror.CodeConflict`/`CodeBadRequest`/`CodeNotFound`（业务错误，重试无意义）
- **caller ctx 取消**：在每次尝试前检查 `ctx.Err()`；在退避等待期间通过 `select { case <-ctx.Done(): }` 立即响应取消，不浪费等待时间

### D2: RunRegistry 原子化更新（T2.2）

在 `activeRunMap` 中新增 `mu sync.Mutex` 和 `updateOrStore` 方法：

- **`updateOrStore(key, update)`**：在 mutex 保护下执行 load-modify-store，确保原子性
- **`StoreRunner`/`StoreCancelable` 改用 `updateOrStore`**：回调函数接收 `(existing, ok)`，合并新旧字段（保留 cancel/runner/runID）
- **plain `load`/`store`/`delete` 保持无锁**：这些单操作本身是并发安全的（sync.Map 保证），只有复合操作需要 mutex
- **`StorePlaceholder` 保持 plain `store`**：仅在 StoreRunner/StoreCancelable 之前种子占位，无需保留字段

## 后果

### 正面影响

- DB 瞬态故障自动恢复，长任务不会因 DB 死锁/超时直接失败
- RunRegistry 并发安全，StoreRunner/StoreCancelable 不再丢失数据
- 错误链完整保留：`apierror.Wrap` 通过 `Cause` 字段 + `Unwrap()` 保持错误链，`errors.As` 可穿透 apierror 找到底层 `*pq.Error`

### 负面影响

- DB 重试增加最坏情况延迟：4 次尝试 + 7s 退避 = 最坏 ~10s（仅对瞬态错误）
- `fn` 回调必须是幂等的：重试可能多次执行 fn，副作用（事件发布、WS 推送）必须延迟到事务提交后
- `activeRunMap.mu` 在高并发 StoreRunner/StoreCancelable 时有轻微锁竞争（实际场景中同一 session 并发 store 极少）

## 替代方案

### A1: 无限重试（与 LLM 重试一致）

LLM 重试使用 `maxRetries = -1`（无限重试），DB 重试考虑过同样方案。

**未选择原因**：DB 瞬态故障通常是死锁或连接问题，无限重试可能加剧死锁（多个事务互相等待）。3 次重试 + 指数退避足以覆盖大多数瞬态故障，同时避免雪崩。

### A2: ManagedMap 通用容器

报告建议用通用 `ManagedMap` 替代裸 `sync.Map`。

**未选择原因**：`activeRunMap` 只有两个方法（StoreRunner/StoreCallable）需要原子化，新增 `updateOrStore` 方法更轻量。通用 `ManagedMap` 会增加抽象层，而当前只有一个使用场景（YAGNI）。

### A3: Cancel 方法也用 updateOrStore

`Cancel` 方法仍有 load-check-delete TOCTOU 模式。

**未选择原因**：T2.2 的范围限定为 StoreRunner/StoreCancelable（报告 Sprint 2 T2.2）。Cancel 的 TOCTOU 是预存问题，且 `runID` CAS 检查（`current.runID == run.runID`）已提供部分保护。完整修复需重新设计 Cancel 语义，留待后续迭代。

## 验证

- `go build ./...` ✅
- `go vet ./internal/data/... ./internal/runtime/...` ✅
- `go test -race ./internal/runtime/...` ✅（含 3 个新并发测试：64 goroutines + 200 iterations）
- `go test -race -run "TestExecInTxWithRetry|TestIsRetryableDBError" ./internal/data/` ✅（8 测试 + 11 子测试，19.6s）
- aranea-review：0 阻断、0 建议（已修复 dead code）、0 提示

## 参考

- 审查报告：`docs/reports/2026-06-18-review-full-message-chain-and-solutions.md` §6.2.2 问题 11/12、§8.2 Sprint 2 T2.1/T2.2
- 代码：`internal/data/tx_retry.go`、`internal/runtime/run_registry.go`
- 测试：`internal/data/tx_retry_test.go`、`internal/runtime/run_registry_test.go`
- 相关规范：CS-B15（重试逻辑上限 3 次 + 指数退避）、红线 #21（共享状态锁保护）、AS-FSM-01（状态机显式化）
