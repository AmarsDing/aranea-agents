# spirit-orchestration-remediation

Spirit-orchestration-redesign 审计修复。原 2026-06-05-spirit-orchestration-redesign 82.6% 完成，4 项已知缺口：Checkpoint 重建、TF-IDF 占位、AgentPerformance 集成、集成测试。

## 当前状态

基于代码审查，review-fixes 变更中的所有阻断级和建议级修复已全部实现：

### 已修复项
- ✅ DEV-01: DAG 编译后的 Definition JSON 已通过 `UpdateTeamDefinitionJSON` 写入 Team
- ✅ DEV-11: spirit_trace_id 已在 ChatOrchestrator turn 入口生成
- ✅ DEV-04: spirit profile 已包含新工具名（双写过渡期新旧并存）
- ✅ 错误处理：task_orchestrator_impl.go / dag_graph_compiler.go / agent_as_tool.go 均已使用 kerrors
- ✅ Service→biz 下沉：RecordTeamCompletion / ScheduleDependentTeams / CheckAllTeamsCompleted 已移至 biz 层
- ✅ 前端反向依赖修复：buildGraphFromDefinition 已移至 features/teams/graphUtils.ts
- ✅ 技术债务标记：DEV-02/03/05/07 均有 TODO(debt) 标记

### 仍待修复项
- ❌ DEV-02: Graph Checkpoint 恢复不完整（GraphAgent 重建未实现）
- ❌ DEV-03: AgentCapability.Capacity 冲突检测未实现
- ❌ DEV-05: Layer 2 语义匹配仍使用 TF-IDF 占位（待 pgvector 集成）
- ❌ DEV-06: Team 超时检测未实现
- ❌ DEV-07: Phase 1/2 中断恢复未实现
- ❌ T3.3: Agent 能力 Embedding（Layer 2 语义匹配）未实现
- ❌ T3.6: 全量验证未执行
