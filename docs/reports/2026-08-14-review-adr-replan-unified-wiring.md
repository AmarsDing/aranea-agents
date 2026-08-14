# ADR-F: 重规划统一接线（P4-G2）——静态声明先验 + 智能轨兜底落地，Reflexion 有界重试

## 状态：已接受（2026-08-14）

## 背景

差距矩阵（plan §三 D4/G2）：重规划「双轨断裂」——

1. **team 域（生产主力）无智能轨**：`BuildTeamGraphRoot` 路径只装配节点级静态 `failure_recovery`（`fallback_agent` / `on_failure=skip`），`RuntimeReplanner` 从未注入（全仓 grep 确认 `OnNodeFailure` 仅 graph run 域 `buildNodeCallbacks` + 测试引用）；
2. **graph run 域的智能轨是观测性空壳**：C-23 策略下 `ReplanRetry` 只 stash `ControlCommand` 后传播原错误（注释明示 "NOT returned as AfterNode result"），`insert_fallback` 发 `InterruptError` 但 `buildInsertFallbackAction` 的 prev/next 节点 ID 从 `failedNode+"_prev"` 命名约定派生——即使消费也接不上真实拓扑；
3. **重试不携带失败上下文**：静态 `RetryPolicy` 是同参重试，模型会在同一个坑上反复失败（Reflexion 缺口）。

理论支持：

- **LLMCompiler**（arXiv:2307.05760）Joiner：每步 finish-or-replan 是编排器的核心决策，replan 必须**真正落地**而非记录意图；
- **Reflexion**（arXiv:2303.11366）：失败原因写成 verbal 反馈注入下次尝试——重试不带上下文的期望收益趋零；
- **Self-Refine 有界性**（arXiv:2303.17651）：自修复环必须有硬预算，防无限循环（承接 P4 实施约束 #1）。

机制确认（代码核实）：

| 机制 | 位置 | 性质 |
|------|------|------|
| AfterNode 合并顺序：**per-node 先 → global 后** | `pkg/trpc-agent-go/graph/executor.go:4013-4016` | 静态轨天然先跑，恢复成功则 `nodeErr==nil`，智能轨自动跳过——双轨仲裁顺序已是框架事实，只需文档化 |
| AfterNode 在 RetryPolicy 耗尽后才跑；返回 `(result, nil)` 恢复节点并继续路由下游 | `executor.go:3351-3411` `finalizeFailedExecution`→`finalizeRecoveredExecution` | 智能轨落地通道 |
| 框架**无任务重入队 API**（AfterNode 只能恢复/传播/替换错误） | `executor.go` retry 路径全文 | G2「retry=重入队」需等效落地 |
| 静态 fallback 已在 callback 内同步执行替补 agent 并恢复 | `internal/graph/trpc/failure_recovery.go:30-59` | 智能重试的同构先例（callback 内同步重执行是已验证模式） |
| `GraphAgent.Run` 把 `RunOptions.RuntimeState` 合并进 initialState；`StateKeyParentAgent` 常驻 | `internal/graph/trpc/builder.go:439-466` | team 域 callbacks 注入通道 |
| `GraphAgent.nodeAgents` 按节点 ID 解析成员 agent（`NewAgentNodeFunc(nodeID)` 可重执行任意 agent 节点） | `builder.go:486-499` + `state_graph.go:2954` | 智能重试的执行载体 |
| team 域 interrupt 消费链完整：tee→`MarkTeamGraphInterrupt`→waiting_human（P3-3 已做 suspend/wake） | `internal/team/runner_graph_event_tee.go:35-45` | `insert_fallback` HITL 降级的现成出口 |
| replanner 计数器必须随执行流结束释放（A5 防泄漏） | `runtime_adapter.go:186-187` 既有 `ReleaseExecution` | team 域需对等收口点 |

## 决策

### D1: 统一仲裁顺序（文档化框架事实，不改代码路径）

节点失败处理链：**静态声明先验 → 智能轨兜底**。

1. 节点级 `failureRecoveryAfterNode`（per-node，先跑）：声明了 `fallback_agent` 的同步换脑恢复；声明 `on_failure=skip` 的标记跳过；恢复成功 → 错误清除，智能轨不触发；
2. 全局 replanner AfterNode（global，后跑）：仅处理静态轨**未声明或恢复失败**的失败；
3. replanner 自身异常/未知严重度/预算耗尽 → 传播原始错误（fail-closed，承接 P4 约束 #2）。

### D2: team 域接线（修复断裂核心）

1. `buildNodeCallbacks` 从 `trpcGraphBuilderFactory` 方法提取为包级构造函数 `NewReplanNodeCallbacks(g, replanner, lg, ids...)`（捕获图实例用于节点类型查询），graph run 域（`buildRuntime`）与 team 域（`BuildTeamGraphRoot`）共用同一构造；
2. team 域传递通道：`GraphAgent` 新增 `SetNodeCallbacks(cb)`——`Run` 时若 `initialState[StateKeyNodeCallbacks]` 为空则注入持有值。graph run 域的 `trpcGraphRuntime.Run` runtimeState 注入保留（显式注入优先于持有值），两域收敛于同一执行语义；
3. **生命周期对等**：team runner 持有 `replanner` 依赖，`runTeamTurn` 收口处 `defer replanner.ReleaseExecution(graphExecID)`（含 HITL 暂停路径——暂停即流结束，resume 重跑时重新注入重新计数，与 graph 域 Resume 语义对齐）。

### D3: 决策落地语义（逐动作）

| replanner 决策 | 现状（C-23） | G2 落地 |
|---|---|---|
| `retry`（transient） | stash ControlCommand，传播错误（空壳） | **Reflexion 智能重试**：callback 内同步重执行节点（`NewAgentNodeFunc(nodeID)`），state 副本 `user_input` 拼入失败反馈；成功即 `(result,nil)` 恢复正常路由；失败传播。仅 agent 节点可重试（function/tool 节点无 prompt 语义，保持 fail-closed）。预算 = `maxReplanAttempts=3`/执行（既有计数器） |
| `insert_fallback`（agent_incapable） | InterruptError（payload 接不上拓扑） | 静态声明场景 D1 已由 per-node 轨先处理；智能轨只对**未声明**场景生效 → InterruptError HITL（graph 域 resume API / team 域 waiting_human 链现成）——不假装能自动选脑，暂停上报 |
| `reroute`（route_blocked） | 传播错误 | 退化为 skip（复用 `skipNodeUpdate`：`SkippedNodesStateKey` 标记 + 下游照常屏障推进），FlowLog 记录「改道=跳过」；不改框架拓扑 |
| `rebuild_subgraph`（subtask_invalid） | 传播错误 | 保持 fail-closed（ControlCommand + 传播错误）——子图重建出本批次范围 |
| unknown / 预算尽 / replanner 异常 | error | 传播原始错误（fail-closed）✓ 不变 |

**推翻 C-23 retry 分支的理由**：C-23 的 fail-closed 建立在「无 Reflexion 注入机制 + 无 team 域视角」上，保守选择导致智能轨空转。G2 的等效落地（callback 内同步重执行）与静态 fallback 同构、有既有先例、有硬预算，且比「同参重入队」更优——反馈注入避免同坑复踩。

### D4: Reflexion 失败反馈格式与隔离

- 反馈文本：`[重试反馈] 上一次尝试失败：{error 摘要}。请分析原因并调整方法后重试。` 拼入重试副本的 `StateKeyUserInput` 前部；
- **checkpoint 隔离**：反馈只写在重执行用的一次性 state 副本上——callback 拿到的 `stateCopy` 是 per-task 隔离副本，但其变更会经 `syncResumeState` 进 checkpoint，故重试输入必须再拷一层，原 state 零污染；
- 反馈键不新增 state key，不污染 schema。

### D5: LLM 分析档默认关闭

关键词规则快路径（`analyzeFailure`）保留为唯一分析器；`unknown` 严重度传播错误。LLM 分析档（unknown → LLM 归因）本批次不实现，对齐 P4 约束 #3「先观察后阻断」。

### D6: FlowLog 观测

- 复用既有 `graph.replan.decided`（决策时刻，replanner 内发射）；
- 新增 `graph.replan.retry`（ok/warn）：智能重试落地——成功 done（含 attempt 计数），失败 warn；登记 `stepTitleRegistry` + `52-flow-logger.design.md` §5.1；
- HITL 升级复用既有 interrupt 链事件，不新增 step。

## 后果

正面：
- team 域（生产主力）获得智能兜底，双轨断裂根治；
- retry 从「同参盲试/意图记录」升级为「携带失败上下文的有界重试」（Reflexion），transient 故障自愈率实质提升；
- 全部落地路径 fail-closed：任何不确定性都传播原始错误或升级 HITL，无静默吞错；
- 零框架改动（不改 executor），落地模式复用静态 fallback 已验证先例。

负面：
- 智能重试在 callback 内同步执行，失败节点墙钟叠加一次 LLM 调用（有界：3 次/执行预算，transient 语义本就需要等待）；
- `insert_fallback` 不做自动选 agent——agent_incapable 且未声明 fallback 时只能 HITL 暂停，自动化上限低于 LLMCompiler Joiner（接受：选错脑比暂停更危险，且需要 capability 匹配基础设施，留待后续批次）；
- `reroute` 退化为 skip 而非真实改道——被绕过节点的下游若强依赖其输出仍会失败（与既有静态 skip 语义一致，不引入新风险）。

## 替代方案

| 方案 | 未选原因 |
|------|---------|
| 框架 executor 增加 requeue API（任务重入队） | 架构级改动触碰 pregel 调度核心，与 RetryPolicy 语义重叠；callback 内同步重执行以 1/10 改动面达到等效效果 |
| `insert_fallback` 自动按 capability 匹配选 agent | 需要 AgentReader + 匹配策略 + 选错回滚机制；选错脑产出「看似有效的错误结果」比 HITL 暂停更糟（DSH fail-loud 原则）；留待 G4 Leader 纠偏落地后由 Leader 指派 |
| `rebuild_subgraph` 真实子图重建 | 框架不支持运行时拓扑改写（StateGraph 编译后冻结）；需要 interrupt→改图→resume 全链，单独立项 |
| 智能轨替代静态轨（统一只用 replanner） | 静态声明是作者先验（零 LLM 成本、确定性）， replanner 是运行时兜底；两者是分层而非竞争关系（D1） |
| 重试反馈写入新 state key 由成员 prompt 消费 | 侵入成员 prompt 构造链（team 编译/MDC 契约），反馈键需进 schema；拼入 user_input 副本零侵入且对所有 agent 节点即时生效 |
