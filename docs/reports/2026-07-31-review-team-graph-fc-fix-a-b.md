# Team-Graph 编排修复（F-C / Fix A / Fix B）全维度终审报告

> 审查对象：Phase 11.E 三个修复 —— F-C 图节点事件桥接、Fix A sort_order 归一化与执行图归因、Fix B graph_executions 收敛。
> 审查范围：`internal/team/`（definition / graph_attribution / runner_graph_event_tee / runner_team_trpc / team_graph_run_coordinator / runner_mediator / runner_team_turn / runner_helpers）、`internal/biz/`（graph_execution_usecase / graph / orchestration_observatory）及对应测试；tee 部分已随 69c58158e 提交，其余为工作区变更。
> 维度加载：1（架构）、2（质量）、3（正确性）、8（错误处理）、4（DB 性能）、6（可测试性）、11（业务逻辑）、14（状态机/事件）、7（可维护性）、12（文档同步）、15（测试）。

## 概要

| 维度 | 🔴 阻断 | 🟡 建议 | 🟢 提示 |
|------|---------|---------|---------|
| 后端 — 架构合规 | 0 | 0 | 0 |
| 后端 — 分层合规 | 0 | 0 | 0 |
| 后端 — OOP | 0 | 1 | 0 |
| 后端 — Agent 运行时 | 0 | 0 | 1 |
| 后端 — 并发安全 | 0 | 0 | 0 |
| 后端 — 错误处理 | 0 | 0 | 1 |
| 后端 — 业务逻辑正确性 | 0 | 0 | 1 |
| 后端 — 编程规范 | 0 | 2 | 0 |
| 后端 — 测试 | 0 | 0 | 0 |
| 文档同步 | 0 | 0 | 0 |
| 构建与回归 | 0 | 0 | 0 |
| **合计** | **0** | **3** | **3** |

## 阻断项

无。

## 建议项

| ID | 维度 | 文件 | 问题描述 | 修复建议 |
|----|------|------|----------|----------|
| S1 | OOP（BI6） | [team_graph_run_coordinator.go](file:///f:/aranea-agents/internal/team/team_graph_run_coordinator.go#L50-L57) | `TeamGraphExecutionBackend` 接口方法数 6（>5）：Register/MarkInterrupt/RecordNodeEnd/Finalize/Resume/GetExecution | 内部端口且职责内聚（team graph 执行后端），可接受；后续若再增长按读写拆 Reader/Writer |
| S2 | 编程规范（CS-B7） | [graph_execution_usecase.go](file:///f:/aranea-agents/internal/biz/graph_execution_usecase.go#L495) | `RecordTeamGraphNodeEnd` 参数 6 个（ctx+5） | 镜像后端端口既有签名风格（Register 7 参为既有事实）；若后续再加参数改 Option struct |
| S3 | 可维护性 | [definition.go](file:///f:/aranea-agents/internal/team/definition.go#L131) · [orchestration_observatory.go](file:///f:/aranea-agents/internal/biz/orchestration_observatory.go#L213) | sort_order 归一化规则在 team/biz 两处有意镜像（biz 不能反向依赖 team），存在漂移风险 | 注释已互相标注同步义务且两侧各有测试覆盖；若出现第三处使用，下沉共享包 |

## 提示项

| ID | 维度 | 文件 | 描述 |
|----|------|------|------|
| T1 | 错误处理（BE4） | [runner_team_turn.go](file:///f:/aranea-agents/internal/team/runner_team_turn.go#L262) · [runner_helpers.go](file:///f:/aranea-agents/internal/team/runner_helpers.go#L193) | `_ = r.mediator.FinalizeTeamGraphExecution(...)` 为有意 best-effort：失败已在 coordinator 内 Warn 记录（`team.graph.exec_finalize_fail`），非静默吞错，不违反红线 #22 意图 |
| T2 | 业务逻辑 | [event_bridge.go](file:///f:/aranea-agents/internal/graph/trpc/event_bridge.go#L169) | steps_json 的 step_index 取自框架 step_number，sequential 执行可能全为 0；与 standalone 路径（consumeRuntimeEvents 同源）行为一致，`upsertGraphStep` 以 NodeID+StepIndex 为键不受影响 |
| T3 | DB 性能 | [graph_execution_usecase.go](file:///f:/aranea-agents/internal/biz/graph_execution_usecase.go#L495) | RecordTeamGraphNodeEnd 每 node_end 一次 UpdateRun，与 standalone 路径逐节点落库行为一致，量级可接受 |

## 亮点

- **依赖方向合规**：biz 侧新增方法零框架依赖；tee 复用 `graphtrpc.EventBridge.ConvertEvent + ActivityEventToSystemNotice` 同一转换链，未另起第二套事件映射（BR7 精神）
- **状态机合规（FSM）**：`FinalizeTeamGraphExecution` 经 `applyExecTransition` 走显式状态机，前置终态守卫保证幂等（迟到的相反结局 finalize 不得翻转终态，有测试锁定）
- **并发安全**：execMu 锁内改状态、`SnapshotForPersist` 后锁外落库，与既有 `MarkTeamGraphInterrupt` 模式一致；tee goroutine 走 `safego.Go`，`in` 关闭即退出，`out` 缓冲 64 防反压死锁
- **跨 execution 隔离**：`handleGraphWatchNotice` 按 `execution_id != sess.execID` 提前丢弃（L493-496），有专项测试锁定
- **测试质量**：幂等性、跨 exec 隔离、upsert 去重、归因回退（nil ct/无 agent 节点/成员被移除）、0 基/稀疏/重复/显式重排 sort_order 全谱系；`-race` 通过
- **文档同步完整**：design §N + development Phase 11.E + 运行时验证证据（run=b89266b7）均已落档

## 构建与回归证据

| 项 | 结果 |
|----|------|
| `go build ./internal/...` | ✅ |
| `go test ./internal/biz/ -run "TestRecordTeamGraphNodeEnd\|TestFinalizeTeamGraphExecution\|TestBuildOrchestrationRegistryFromDefinition\|TestRegisterTeamGraphExecution"` | ✅ ok 0.538s |
| `go test ./internal/team/`（全量） | ✅ ok 0.498s |
| `go test -race ./internal/team/ -run "TestTeeGraphStageNotices\|TestTeamGraphRunCoordinator\|TestBuildAttribution"` | ✅ ok 1.750s |
| `go vet ./internal/team/ ./internal/biz/` | ✅ 无输出 |
| 运行时验证（V3） | ✅ 见 development 文档 Phase 11.E（steps_json 落库、status 收敛、归属正确） |

## 后端合规性清单

- [x] 依赖方向向内（biz 不 import data/service/trpc-agent-go/proto）
- [x] 跨模块通过窄接口（TeamGraphCoordAccess 5 方法、mediator 解循环依赖）
- [x] 无工具生成代码的手动修改
- [x] goroutine 走 safego，有明确退出路径（红线 #13/#23）
- [x] 业务错误用 apierror（applyExecTransition 非法转换返回 BadRequest）
- [x] 日志用 loggateway.Logger，结构化字段 + Err(err)（红线 #16）
- [x] 共享状态有锁保护，-race 通过（红线 #21）
- [x] 无静默吞错误（`_ =` 处均有内部 Warn 落账）
- [x] 外部输入/返回值 nil 检查（uc/exec/ct/meta 全链路判 nil）
- [x] 状态机：>3 状态实体 GraphExecution 经显式 FSM 转换，终态幂等
- [x] 事件：tee 高频事件过滤（pregel/state/channel/custom），符合限流红线
- [x] 文档同步（DOC-SYNC-1/5/6）：design §N、development Phase 11.E、代码锚点有效

## 结论

**通过，可提交。** 0 阻断项；3 建议项均为可接受的内部端口形态，不阻断合并，后续迭代处理。
