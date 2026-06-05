## Context

spirit-orchestration-redesign 变更已归档，但 aranea-review 全栈代码审查发现 3 个阻断级问题和 10 个建议级问题。阻断项涉及错误处理、DAG 编译核心路径、Service 层业务逻辑泄漏，必须修复才能保证生产可用。

**当前状态**：所有阻断级和建议级修复已全部实现（2026-06-05 代码审查确认）。

当前技术约束：
- biz 层不得 import trpc-agent-go，框架交互通过 agent/tools 层桥接
- Service 层只做 proto↔biz 映射 + Runner 编排，不含业务逻辑
- 业务错误必须使用 kerrors，禁止 fmt.Errorf 返回业务错误
- 前端依赖方向：components → features，禁止 features → components 反向依赖

## Goals / Non-Goals

**Goals:**
- 修复 3 个阻断级问题：错误处理、DAG Definition 未替换、Service 层业务逻辑
- 修复 2 个 fmt.Errorf 业务错误（dag_graph_compiler.go、agent_as_tool.go）
- 修复 DEV-11：spirit_trace_id 在 ChatOrchestrator turn 入口生成
- 修复 DEV-04：spirit profile 工具名引用更新
- 修复前端 features→components 反向依赖
- 标记未完成实现为技术债务（DEV-02/06/07）

**Non-Goals:**
- 不实现 DEV-02（Graph Checkpoint 完整恢复），仅标记 debt
- 不实现 DEV-06（Team 超时检测），仅标记 debt
- 不实现 DEV-07（Phase 1/2 中断恢复），仅标记 debt
- 不实现 DEV-03（AgentCapability.Capacity 冲突检测），仅标记 debt
- 不实现 DEV-05（Layer 2 Embedding 匹配），保持 TF-IDF 占位
- 不拆分 TaskOrchestratorPort 接口（6 方法 > 5 建议，但功能内聚，暂不拆分）

## Decisions

### D1: Service 层业务逻辑下沉到 biz.SpiritTeamUsecase

**选择**：将 recordTeamCompletion/scheduleDependentTeams/checkAllTeamsCompleted 三个方法从 Service 层移到 biz.SpiritTeamUsecase

**替代方案**：
- A) 新建 biz.TeamDAGScheduler Usecase → 被否决，scheduleDependentTeams 与 SpiritTeamUsecase 的 AssembleTeam 紧密关联，拆分增加复杂度
- B) 保留在 Service 层但标记 TECH-DEBT → 被否决，这是架构红线（BA4），必须修复

**理由**：DQ Score 计算、拓扑推断、进化建议创建是纯业务规则；DAG 依赖解析和团队激活调度是核心编排逻辑；全部团队完成检查是业务判断。三者均不属于 Service 层的"映射+编排"职责。

### D2: DAG Definition JSON 写入 Team

**选择**：在 orchestrateDAG() 中，assembler.AssembleTeam() 创建 Team 后，将 DAG 编译的 DefinitionJSON 写入 Team 的 DefinitionJSON 字段

**替代方案**：
- A) 修改 assembler 接受外部 DefinitionJSON → 被否决，assembler 有自己的 Definition 构建逻辑，修改接口影响面大
- B) 在 assembler 之前先编译 DAG Definition，跳过 assembler → 被否决，丧失 Team+Session 联合事务创建

**理由**：先走 assembler 创建 Team（保证事务），再覆盖 DefinitionJSON（保证 DAG 结构正确），两步操作影响面最小。

### D3: spirit_trace_id 在 ChatOrchestrator turn 入口生成

**选择**：在 ChatOrchestrator 的 turn 入口处生成 spirit_trace_id 并注入 context

**替代方案**：
- A) 在 plan_and_execute 工具入口生成 → 被否决，simple/moderate 路径不经过 TaskPlanner
- B) 在 Spirit agent 的 system prompt 初始化时生成 → 被否决，trace ID 应与用户消息一一对应

**理由**：ChatOrchestrator turn 入口是所有 Spirit 编排路径的必经之路，在此生成可覆盖所有复杂度级别。

## Risks / Trade-offs

| 风险 | 缓解措施 |
|------|---------|
| [R1] Service→biz 下沉可能破坏现有调用链 | 保持方法签名一致，Service 层改为委托调用 biz 方法 |
| [R2] DAG Definition 覆盖可能导致 Team 运行异常 | 编写集成测试验证编译后 Definition 的正确性 |
| [R3] spirit_trace_id 注入可能影响非 Spirit session | 仅在 Spirit session 的 turn 入口注入，非 Spirit 路径不受影响 |
