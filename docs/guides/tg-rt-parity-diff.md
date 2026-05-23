# TG-RT-PARITY — Native vs Graph 差异说明

> **状态**：build/runtime 对齐 ✅ · run 级 team_summary 成员指纹对比 ✅ · 全 LLM E2E ⏳  
> **代码**：`internal/team/parity_test.go` · `parity_runtime_test.go` · `parity_run_test.go`

## 已覆盖（自动化）

| 检查项 | Native | Graph |
|--------|--------|-------|
| 编译 entry/finish | ✅ | ✅ |
| agent 节点数量 | ✅ | ✅ |
| member key 与 graph agent_name 对齐 | ✅ | ✅ |
| `BuildStateGraphWithAgents` + `NewGraphAgent` | N/A | ✅ |

## Step 持久化策略（BL-03 后）

| 路径 | team_run_steps 来源 |
|------|---------------------|
| **Native**（`graphExecID==""`） | 流结束后 bulk `persistStep`（每 member） |
| **Graph 首跑** | `StartGraphStepWatch` 订阅 `member_message_done` / `graph_node_end`；无 step 时 anchor fallback |
| **Graph HITL defer** | 首跑 watch 部分 step + resume `FinalizeGraphTeamRun` |
| **Graph resume** | `graphWatchStepsAndFinalize` + `PersistGraphRunStep` |

## 已知可接受 diff（蓝图 §519）

Graph 路径额外 WS envelope（Native 无）：

- `graph_node_start` / `graph_node_end` / `graph_node_error`
- `graph_execution_done`
- `orchestration_agent_status`（Observatory 投影）

Native 路径额外 envelope：

- 同步 `team_step_started` / `team_step_finished` bulk 序列（Graph 改为 per-node 事件驱动）

## Run 级 parity

| 检查项 | 状态 | 说明 |
|--------|------|------|
| `team_summary` 成员指纹（agent_key / tool_call_count / tokens / status） | ✅ | `TestParityRunSummary_AllModes` |
| Native vs Graph WS 独占 envelope 文档化 | ✅ | `TestParityRunEnvelopeDiff_documented` |
| `team_run.token_in/out` run 级聚合 | ✅ | Native 总量 vs Graph `enrichTeamRunMetricsFromSteps` |
| 真实 LLM stub 六 mode 执行对比 | ✅ | `TestParityRunE2E_stubStreamAllModes` |
| WS 序列 hash 逐条对比（harness） | ✅ | persistStep 双路径 hash 一致 |
| 生产 Graph event-watch WS diff | 📋 | 见 §已知可接受 diff |

Harness：`internal/team/parity_run_test.go`（fixture 级）；全 LLM E2E 待独立 PR。
