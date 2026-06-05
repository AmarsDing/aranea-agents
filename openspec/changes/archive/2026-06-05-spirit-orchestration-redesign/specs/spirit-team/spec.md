# Spirit Team (Modified)

## Overview
SpiritTeamUsecase 的 AssembleTeam 流程重构，Team 状态机归一，moderate 路径实现。

## Requirements

### REQ-ST-01: Team 状态机归一
- 删除 active 状态（语义模糊）
- 新增 pending 状态（创建完成，等待执行）
- running 状态（正在执行）
- 新增 interrupted 状态（异常中断，可恢复）
- 保留 completed/failed/cancelled
- 删除 assembled 状态
- 状态转换：pending→running, running→completed/failed/cancelled/interrupted, interrupted→running

### REQ-ST-02: P0 Bug 修复
- AutoStart=false 的无依赖节点初始状态改为 pending（而非 active）
- AutoStart=true 的无依赖节点初始状态改为 pending，立即启动后转为 running
- 有依赖节点初始状态为 pending + depends_on 字段

### REQ-ST-03: moderate 路径实现
- 使用 Agent-as-Tool（agenttool.NewTool）实现 moderate 路径
- AgentMatcher 找到最优 Agent → 包装为 AgentTool → Spirit 调用
- 删除 list_butlers / query_butler_status 工具

### REQ-ST-04: Team+Session 联合事务
- AssembleTeam 中 Team 创建和 Session 创建在同一 Ent 事务中
- 使用 d.Ent().Tx() 包裹两个创建操作

### REQ-ST-05: checkAllTeamsCompleted 修复
- 将 active 视为"仍在进行中"改为：pending/running 视为"仍在进行中"
- interrupted 状态的 Team 不阻止完成事件发布（需先尝试恢复）

### REQ-ST-06: UpdateTeamDefinitionJSON（review-fixes 新增）
- SpiritTeamUsecase 新增 UpdateTeamDefinitionJSON(ctx, teamID, definitionJSON) 方法
- 用于 TaskOrchestrator 在 DAG 编排后将编译后的 Definition JSON 写入 Team
- 修复 DEV-01：DAG 编译核心路径现在生效
- 写入失败返回 kerrors.InternalServer

### REQ-ST-07: Service→biz 业务逻辑下沉（review-fixes 新增）
- RecordTeamCompletion：DQ Score 计算 + 拓扑推断 + 进化建议创建，从 Service 层移至 biz.SpiritTeamUsecase
- ScheduleDependentTeams：DAG 依赖调度，返回 DependentTeamAction 列表，Service 层负责执行动作
- CheckAllTeamsCompleted：全完成检查，返回 AllTeamsCompletedResult，Service 层负责发布事件
