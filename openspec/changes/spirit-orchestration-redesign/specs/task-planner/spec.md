# Task Planner

## Overview
TaskPlanner 是 Spirit 编排三阶段架构的第一阶段，单一职责：评估任务复杂度 + AI 拆分任务 + 输出策略。

## Requirements

### REQ-TP-01: 六维复杂度评估
- 输入：用户消息 + Intent Artifact + 历史记忆
- 评估维度：语义复杂度(0.25)、结构复杂度(0.15)、领域跨度(0.15)、工具需求(0.10)、上下文需求(0.10)、历史难度(0.25)
- 输出：ComplexityLevel(simple/moderate/complex) + ComplexityScore(0.0-1.0) + DimensionScores
- 语义复杂度复用 Intent Pass 的 LLM 调用，不增加额外 API 开销
- 分级阈值：simple < 0.3, moderate 0.3-0.6, complex >= 0.6

### REQ-TP-02: AI 任务拆分（仅 complex 级别）
- 触发条件：ComplexityLevel == complex
- 使用独立 LLM 调用，结构化 prompt 约束输出
- 输出：SubTask[]（含 id/name/description/depends_on/required_capabilities/priority/estimated_complexity）
- required_capabilities 使用预定义标签体系：go-backend, go-kratos, vue3-frontend, quasar-ui, devops, database, architecture, testing, security, research, documentation, api-design
- 不指定具体 Agent Key（Phase 2 职责）
- 生成 TaskDAG（依赖关系 + 拓扑排序）

### REQ-TP-03: 策略输出
- simple → StrategyDirect（直接回答）
- moderate → StrategySingleAgent（Agent-as-Tool）
- complex → 根据 TaskDAG 结构选择：parallel/dag/coordinator
- 记忆命中 DQ>0.7 时复用历史拓扑
- 输出 OrchestrationStrategy + TopologyHint + StrategyReason

### REQ-TP-04: TaskPlan 持久化
- TaskPlan 写入 DB（Ent Schema: task_plans 表）
- 状态：draft → confirmed → executing → completed/failed
- Spirit LLM 可通过 ConfirmPlan 调整计划（合并/拆分/新增/删除子任务）

### REQ-TP-05: 记忆查询
- 查询 OrchestrationCache.SuggestTopology()
- 查询 AgentPerformance.GetBestForTaskType()
- 查询 Embedding 相似案例（Phase 3 实现）
- 命中结果写入 TaskPlan.MemoryHit

### REQ-TP-06: 可观测性
- 每步日志携带 spirit_trace_id + session_id + plan_id
- StepID: spirit.planner.assess / route / memory / decompose / persist / confirm

## Port Interface

```go
type TaskPlannerPort interface {
    Plan(ctx context.Context, input PlanInput) (*TaskPlan, error)
    GetPlan(ctx context.Context, planID PlanID) (*TaskPlan, error)
    ConfirmPlan(ctx context.Context, planID PlanID, adjustments PlanAdjustments) (*TaskPlan, error)
}
```

## Data Model

```go
type TaskPlan struct {
    ID, SpiritSessionID, TraceID string
    UserMessage string; IntentArtifact *intent.Artifact
    ComplexityLevel; ComplexityScore float64; Dimensions DimensionScores
    SubTasks []SubTask; TaskDAG *TaskDAG; DecomposeReason string
    Strategy OrchestrationStrategy; StrategyReason string; TopologyHint TopologyType
    MemoryHit *MemoryHit; Status PlanStatus
}
```

## Ent Schema
- Table: task_plans
- Fields: id, spirit_session_id, trace_id, user_message, intent_artifact_json, complexity_level, complexity_score, dimensions_json, sub_tasks_json, dag_json, decompose_reason, strategy, strategy_reason, topology_hint, memory_hit_json, status, created_at, updated_at
