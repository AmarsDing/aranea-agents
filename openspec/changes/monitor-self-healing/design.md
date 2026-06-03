## Architecture Overview

### Core Principle: "Fix at the source, observe at the monitor"

修复动作在错误发生的地方执行（trpc-agent-go 运行时），Monitor 层只做观测、统计、告警。

```
┌─────────────────────────────────────────────────┐
│  trpc-agent-go 运行时                             │
│                                                  │
│  LLM 调用失败 → 内置 retry + fallback             │
│  MCP 断连    → 内置 reconnect + health check      │
│  Tool 失败   → 内置 retry + 参数修正              │
│  JSON 畸形   → 内置 jsonrepair（已有）             │
│  上下文超限  → 内置 compress + retry               │
│                                                  │
│  ↓ 自愈结果作为结构化 FlowLog 事件上报              │
│  {auto_healed:true, heal_strategy:"retry",        │
│   heal_attempts:2, heal_success:true}             │
└─────────────────────────────────────────────────┘
         ↓ 事件流
┌─────────────────────────────────────────────────┐
│  Monitor 层（SelfHealObserver）                    │
│                                                  │
│  1. 订阅 FlowLog 事件                             │
│  2. 统计自愈成功率（auto_healed=true vs false）     │
│  3. 识别"反复自愈失败"的模式 → 触发告警             │
│  4. 持久化 HealRecord 到 SQLite                   │
│  5. 生成修复建议给运维（不是自动执行）               │
│  6. 将自愈效果数据反馈给运行时策略优化               │
└─────────────────────────────────────────────────┘
```

## Component Design

### 1. RuntimeAutoHeal（运行时内嵌自愈）

**位置**: `pkg/trpc-agent-go/internal/flow/processor/`

**新增接口**:
```go
// AutoHealStrategy defines how a component auto-heals from errors.
type AutoHealStrategy interface {
    CanHeal(err error) bool
    Heal(ctx context.Context, attempt int) error
    MaxAttempts() int
    Backoff(attempt int) time.Duration
}
```

**集成点**:
- `llmflow.go`: LLM 调用失败时，根据错误类型选择 retry/fallback 策略
- `functioncall.go`: Tool 执行失败时，选择 retry 策略
- `mcpbroker`: MCP 连接断开时，触发 reconnect
- 自愈结果写入 FlowLog 事件（新增 `auto_healed` 元数据字段）

**FlowLog 事件扩展**:
```json
{
  "step_id": "llm-call",
  "flow_phase": "error",
  "auto_healed": true,
  "heal_strategy": "retry_with_backoff",
  "heal_attempts": 2,
  "heal_success": true,
  "heal_backoff_ms": 4000
}
```

### 2. SelfHealObserver（Monitor 层观测器）

**位置**: `internal/biz/monitor/self_heal_observer.go`

**替代当前 SelfHealUsecase**，职责从"执行修复"变为"观测修复效果"：

```go
type SelfHealObserver struct {
    repo      HealRecordRepo     // 持久化
    engine    *RootCauseEngine   // 根因分析
    notifier  AlertNotifier      // 告警
    lg        loggateway.Logger
}

// ObserveFlowLogEvent 处理每条 FlowLog 事件
func (o *SelfHealObserver) ObserveFlowLogEvent(ctx context.Context, event FlowLogEvent) {
    // 1. 如果 auto_healed=true，记录自愈成功
    // 2. 如果 auto_healed=false 且 phase=error，运行根因分析
    // 3. 如果根因匹配且运行时未自愈，触发告警
    // 4. 如果同一规则反复自愈失败（3次+），升级告警
}
```

### 3. HealRecord 持久化

**Ent Schema**: `internal/data/ent/schema/heal_record.go`

| 字段 | 类型 | 说明 |
|------|------|------|
| id | string | UUID |
| rule_id | string | 匹配的根因规则 ID |
| trigger_type | string | manual / auto_error_event / auto_repeated_failure |
| trace_id | string | 关联 trace |
| session_id | string | 关联 session |
| step_id | string | 出错步骤 |
| fix_action_type | string | retry / reconnect / fallback / log_only |
| confidence | float64 | 置信度 |
| status | string | applied / observed_healed / observed_failed / alert_fired |
| runtime_auto_healed | bool | 运行时是否已自愈 |
| runtime_heal_attempts | int | 运行时自愈尝试次数 |
| reason | string | 原因说明 |
| created_at | time.Time | 创建时间 |

### 4. RootCauseEngine 增强

**新增条件维度**:
```go
type RootCauseCondition struct {
    StepID          string
    Phase           string
    ErrorCodes      []string
    Pattern         string
    AutoHealed      *bool  // nil=不关心, true=只匹配已自愈, false=只匹配未自愈
    HealAttempts    int    // 运行时自愈尝试次数阈值
    Prerequisites   []Prerequisite
}
```

**新增规则示例**:
```go
{
    ID: "rc-repeated-auto-heal-failure",
    Condition: RootCauseCondition{
        AutoHealed:   boolPtr(true),
        HealAttempts: 3,  // 运行时已尝试 3 次自愈
    },
    RootCause:  "Runtime auto-heal has failed repeatedly",
    Severity:   "critical",
    FixAction:  FixAction{Type: "log_only"},  // 只告警，不自动修复
}
```

**冷却期分级**:
```go
var severityCooldown = map[string]time.Duration{
    "critical": 30 * time.Minute,
    "high":     10 * time.Minute,
    "medium":   5 * time.Minute,
    "low":      2 * time.Minute,
}
```

### 5. API 变更

**新增 RPC**:
```protobuf
rpc ListHealRecords(ListHealRecordsRequest) returns (ListHealRecordsResponse) {
    option (google.api.http) = {get: "/v1/monitor/heal-records"};
}
```

**DiagnoseAndHeal 调整**:
- 返回值增加 `runtime_auto_healed` / `runtime_heal_attempts` 字段
- 当运行时已自愈时，status 为 `observed_healed` 而非 `applied`

### 6. read_flow_logs 工具增强

flowLogEntry 增加字段：
```go
type flowLogEntry struct {
    // ... 现有字段 ...
    AutoHealed     bool   `json:"auto_healed"`
    HealStrategy   string `json:"heal_strategy,omitempty"`
    HealAttempts   int    `json:"heal_attempts,omitempty"`
    HealSuccess    bool   `json:"heal_success,omitempty"`
}
```

## Migration Path

1. **Phase 1**: 保留当前 SelfHealUsecase，新增 SelfHealObserver 并行运行
2. **Phase 2**: 运行时自愈能力就绪后，SelfHealObserver 接管观测职责
3. **Phase 3**: 移除 SelfHealUsecase 的修复执行逻辑，保留诊断 API

## Non-goals

- 不修改 trpc-agent-go 的核心重试逻辑（已有能力不重复实现）
- 不做前端自愈仪表盘（单独变更）
- 不做跨实例 cooldown 共享（需要 Redis，当前 SQLite 单实例）
