# Spirit Recovery

## Overview
Spirit 编排异常恢复机制，启动时自动恢复 interrupted 编排，Graph Checkpoint 恢复，Team 超时检测。

## Requirements

### REQ-SR-01: 启动时自动恢复
- 扩展现有 RecoverOrphanedRunningSessions
- 恢复 OrchestrationHandle：running → interrupted（有 Checkpoint）/ failed（无 Checkpoint）
- 恢复 Team 状态：running → interrupted
- 恢复 TeamRun 状态：running → failed

### REQ-SR-02: Graph Checkpoint 恢复
- 检测 status=interrupted 的 OrchestrationHandle
- 加载最新 Checkpoint
- 重建 GraphAgent
- ResumeFromLatest(checkpoint)

### REQ-SR-03: Team 超时检测
- 为 pending/running 状态的 Team 增加超时检测
- 超时后自动标记为 failed
- 触发依赖调度（scheduleDependentTeams）

### REQ-SR-04: waiting_deps 超时恢复
- 为 waiting_deps（新状态 pending+depends_on）的 Team 增加超时
- 超时后检查前置依赖是否已完成
- 已完成 → 触发调度
- 未完成 → 标记为 failed

### REQ-SR-05: Phase 1/2 中断恢复
- status=draft 的 TaskPlan → 重新执行 Phase 2/3
- status=draft 的 AllocationPlan → 重新执行 Phase 3
- status=pending 的 OrchestrationHandle → 重新执行

### REQ-SR-06: Team+Session 联合事务
- AssembleTeam 中 Team 创建和 Session 创建在同一 Ent 事务中
- 避免部分创建导致的数据不一致
