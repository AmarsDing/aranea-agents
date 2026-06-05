# Task Orchestrator

## Overview
TaskOrchestrator 是 Spirit 编排三阶段架构的第三阶段，单一职责：根据 TaskPlan + AllocationPlan 构建执行图并执行。

## Requirements

### REQ-TO-01: 编排策略执行
- StrategyDirect → Spirit 直接回答，无编排
- StrategySingleAgent → Agent-as-Tool（agenttool.NewTool）
- StrategyParallel → ParallelAgent（框架原生）
- StrategyDAG → GraphAgent（DAG 编排，核心路径）
- StrategyCoordinator → Team(ModeCoordinator)

### REQ-TO-02: DAG → Graph 编译
- DAGToGraphCompiler 将 TaskDAG + AllocationPlan 转换为 Team Definition JSON
- 每个 SubTask → Definition Member（agent_key 来自 AllocationPlan）
- 每个 depends_on → Member 间依赖
- 生成的 Definition 走现有 CompileToCompiledTeam 编译
- 不修改现有 graph_compile.go

### REQ-TO-03: Graph Checkpoint
- 使用 SQLite CheckpointSaver（已有实现）
- 每步完成后自动保存 Checkpoint
- OrchestrationHandle 记录最新 checkpoint_id

### REQ-TO-04: 结果合成
- 所有叶子节点完成 → 触发 synthesis 节点
- SynthesisEngine 三策略：template（<3团队）/ prompt（>=3团队）/ hybrid（部分失败）
- 合成结果持久化到 OrchestrationHandle.SynthesisResultJSON

### REQ-TO-05: OrchestrationHandle 持久化
- 写入 DB（Ent Schema: orchestrations 表）
- 状态：pending → running → completed/failed/cancelled/interrupted
- interrupted 状态表示异常中断，可从 Checkpoint 恢复

### REQ-TO-06: 编排进度查询
- CheckProgress 返回 []TaskProgress
- 基于 Graph 事件（graph.node.start/complete）提供细粒度进度
- 替代当前 CheckTeamProgress 的 DB 轮询

### REQ-TO-07: 编排取消
- Cancel 通过 Runner.Cancel() 取消执行
- 更新 OrchestrationHandle.Status=cancelled
- 取消依赖团队（级联取消）

### REQ-TO-08: 可观测性
- StepID: spirit.orchestrator.strategy / graph_build / graph_agent / execute / checkpoint / synthesize / learn / recover
- Graph 节点执行事件自动产生：graph.node.start/complete/error

### REQ-TO-09: DAG Definition 写入 Team（review-fixes 新增）
- orchestrateDAG() 中 assembler.AssembleTeam() 创建 Team 后，通过 SpiritTeamUsecase.UpdateTeamDefinitionJSON 将 DAG 编译的 DefinitionJSON 写入 Team
- 写入失败时非致命降级（仅日志告警，Team 使用 assembler 生成的原始 Definition）
- 修复 DEV-01：DAG 编译核心路径现在生效

### REQ-TO-10: 在线学习闭环（实现补充）
- 编排完成后 Synthesize() 自动调用 learnFromOrchestration()
- 更新 OrchestrationCache：RecordCompletionWithAgents（DQ Score + 拓扑 + Agent 列表）
- 更新 AgentPerformance：每个参与 Agent 的 TotalRuns/SuccessRuns/SuccessRate/AvgDQScore
- DQ Score 计算：全成功 0.8+findings*0.05（上限 1.0），部分成功 0.5，全失败 0.2

## Port Interface

```go
type TaskOrchestratorPort interface {
    Orchestrate(ctx context.Context, taskPlan *TaskPlan, allocPlan *AllocationPlan) (*OrchestrationHandle, error)
    CheckProgress(ctx context.Context, orchestrationID OrchestrationID) ([]TaskProgress, error)
    Cancel(ctx context.Context, orchestrationID OrchestrationID) error
    Synthesize(ctx context.Context, orchestrationID OrchestrationID) (*SynthesisOutput, error)
    Recover(ctx context.Context, orchestrationID OrchestrationID) error
    RecoverAllInterrupted(ctx context.Context) error  // DEV-09: 扩展方法，被 SessionStatusGuard 使用
}
```

## Data Model

```go
type OrchestrationHandle struct {
    ID, TaskPlanID, AllocationID, SpiritSessionID, TraceID string
    Strategy OrchestrationStrategy; GraphExecutionID string; TeamIDs []string
    Status OrchestrationStatus; CheckpointID string
    SynthesisResult *SynthesisOutput; SynthesisResultJSON string
}
```

## Ent Schema
- Table: orchestrations
- Fields: id, task_plan_id, allocation_id, spirit_session_id, trace_id, strategy, graph_execution_id, team_ids_json, status, checkpoint_id, synthesis_result_json, created_at, updated_at
