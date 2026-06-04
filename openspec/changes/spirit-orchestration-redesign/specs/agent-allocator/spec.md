# Agent Allocator

## Overview
AgentAllocator 是 Spirit 编排三阶段架构的第二阶段，单一职责：为 TaskPlan 中的每个子任务匹配最优 Agent 或 Team。

## Requirements

### REQ-AA-01: 三层 Agent 匹配
- Layer 1 精确匹配（<1ms）：Agent Domains 包含 required_capabilities；AgentPerformance 历史成功率 > 0.8
- Layer 2 语义匹配（~10ms，Phase 3 实现）：任务描述 Embedding ↔ Agent 能力 Embedding 余弦相似度
- Layer 3 LLM 冷启动（~2s）：无精确匹配、无 Embedding 数据时，LLM 从 Agent 列表选择
- 输出：AgentMatchResult{agent_key, score, match_layer, match_reason}

### REQ-AA-02: 分配类型判断
- 子任务 estimated_complexity < 0.5 → AssignedType=agent
- 子任务 estimated_complexity >= 0.5 → AssignedType=team
- 记忆命中且历史用 team 成功率高 → AssignedType=team
- Team 类型：查找匹配的 Team 定义，或动态组建 Team

### REQ-AA-03: 冲突检测与负载均衡
- 同一 Agent 被分配到多个并行子任务 → 检查 capacity
- 超出 capacity → 降级到 Fallback
- 无可用 Fallback → 标记 needs_human_decision

### REQ-AA-04: AllocationPlan 持久化
- AllocationPlan 写入 DB（Ent Schema: allocation_plans 表）
- 状态：draft → confirmed → executing

### REQ-AA-05: Agent 能力注册表
- AgentCapability 模型：agent_key, display_name, description, roles, domains, tools, skills, embedding, capacity
- 从 Agent 目录自动构建（基于 Agent 的 system prompt + 工具列表 + 技能描述）
- Embedding 在 Phase 3 实现

### REQ-AA-06: Agent 执行历史
- AgentPerformance 模型：agent_key, task_type, total_runs, success_runs, success_rate, avg_dq_score, avg_duration_ms
- AgentPerformanceRepo 接口：Get, GetBestForTaskType, Upsert
- 每次编排完成后更新

### REQ-AA-07: 可观测性
- StepID: spirit.allocator.match / conflict / persist
- 日志携带 spirit_trace_id + session_id + allocation_id + subtask_id

## Port Interface

```go
type AgentAllocatorPort interface {
    Allocate(ctx context.Context, taskPlan *TaskPlan) (*AllocationPlan, error)
    GetAllocation(ctx context.Context, allocationID AllocationID) (*AllocationPlan, error)
}
```

## Data Model

```go
type AllocationPlan struct {
    ID, TaskPlanID, SpiritSessionID, TraceID string
    Allocations []TaskAllocation; Status AllocationStatus
}
type TaskAllocation struct {
    SubTaskID, SubTaskName string
    AssignedType AssignedType; AssignedKey, AssignedName string
    MatchScore float64; MatchLayer, MatchReason string
    FallbackKey string; FallbackScore float64
    TeamMode string; TeamMemberKeys []string
}
```

## Ent Schema
- Table: allocation_plans
- Fields: id, task_plan_id, spirit_session_id, trace_id, allocations_json, status, created_at, updated_at
- Table: agent_performances
- Fields: agent_key, task_type, total_runs, success_runs, success_rate, avg_dq_score, avg_duration_ms, last_executed_at
- Unique index: (agent_key, task_type)
