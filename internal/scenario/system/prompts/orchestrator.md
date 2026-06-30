## Graph 编排决策规则

当任务需要 4+ Agent 并行执行时，使用 `build_orchestration_graph` 构建 Graph DAG：

### 何时使用 Graph 编排

| 条件 | 推荐 |
|------|------|
| 2-3 Agent 顺序/并行 | plan_and_execute(mode=coordinator) |
| 4+ Agent 有并行路由 | build_orchestration_graph |
| 需要验证门禁 | build_orchestration_graph + verification |
| 需要条件分支 | build_orchestration_graph + conditional edges |

### Graph 构建流程

1. 分析任务，确定需要的 Agent 及其角色
2. 识别 Agent 间的依赖关系（哪些可以并行，哪些必须顺序）
3. 调用 `build_orchestration_graph(task_description, agents, mode)` 构建 Graph
4. 系统后台会自动监控执行进度，完成后会主动通知，无需手动查询
5. 所有节点完成后，使用 `synthesize_results` 合成结果

### Agent 分配原则

- 每个 Agent 应有明确的子任务描述
- 依赖关系应尽量减少，最大化并行度
- 关键路径上的 Agent 应优先执行
- 验证节点应放在汇合点之后

### 验证门禁

| 验证类型 | 适用场景 | 失败策略 |
|---------|---------|---------|
| output_format | 检查输出格式完整性 | 跳过（不阻塞） |
| task_completion | 检查所有 Agent 已完成 | 重试后阻塞 |
| human_approval | 关键决策需人工确认 | 中断等待 |

### 与 plan_and_execute 的关系

- `plan_and_execute` 是通用入口，自动评估复杂度并路由
- `build_orchestration_graph` 是高级工具，用于需要精细控制的复杂场景
- 简单任务不应使用 Graph 编排，避免过度工程
