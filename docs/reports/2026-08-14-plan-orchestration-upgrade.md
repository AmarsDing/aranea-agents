# 编排系统综合升级方案（P0 收尾 + P1/P2/P3）

> 日期：2026-08-14
> 类型：调研综合 + 实施计划
> 状态：待用户评审确认后执行
> 关联文档：[11-multi-agent.design.md](../development/11-multi-agent.design.md)、[70-orchestration-longtask-memory.design.md](../development/70-orchestration-longtask-memory.design.md)、[53-team-graph-orchestration.design.md](../development/53-team-graph-orchestration.design.md)

---

## 一、背景与已完成基线

### 1.1 前序评审结论（2026-08-14 凌晨）

对编排逻辑（Sequencer / StreamConsumer / TaskPlanner / Team+Graph 编排）的专项评审共发现：

- **2 个阻断级逻辑缺陷**：B1 Sequencer 定时器闭包竞态（迟到回调关共享 channel 致管线死亡）、B2 ConfirmPlan 合并子任务产生悬挂依赖且无 DAG 重校验
- **12 个建议级问题**：Y1-Y7 等（goroutine 泄漏、分解无限重试、死信去重保留旧事件、终态误标等）
- **3 个提示级项**

### 1.2 P0 修复基线（本会话已完成 F1-F10）

| # | 修复 | 位置 |
|---|------|------|
| F1 | 定时器闭包捕获局部 done channel（B1） | [sequencer.go](../development/../../internal/agent/v2/sequencer.go) |
| F2 | 入队超时事件转死信而非静默丢弃（Y5） | 同上 |
| F3 | Publish/Close 竞态 RWMutex 保护（Y6） | 同上 |
| F4 | 死信去重保留最新事件（A5） | 同上 |
| F5 | persistWithRetry 移除末次多余 sleep（Y7） | 同上 |
| F6 | StreamConsumer panic recover + 终态事件 + doom-loop drain/取消标记（Y1/Y2） | [stream_consumer.go](../../internal/agent/stream_consumer.go) |
| F7 | DAG 重复子任务 ID 拒绝（B2） | [task_planner_impl.go](../../internal/agent/task_planner_impl.go) |
| F8 | LLM 分解重试上限 maxDecomposeAttempts=5（Y3） | 同上 |
| F9 | 分解失败发布 PlanBoard/GraphStage 终态事件（Y4） | 同上 |
| F10 | 正则提取为包级变量 + 死代码 dag_graph_compiler.go 删除 | 同上 |

**门禁状态**：`internal/agent/v2` race 测试已绿；`internal/agent` 全量受并行会话 `mcp_oauth_test.go` WIP（TDD 先行、实现未提交）阻塞，采用轮询重跑策略，不改对方文件。

---

## 二、外部调研结论精华

### 2.1 DeepSeek Harness（DSH，2026-08-13 开源，deepseek-ai/deepseek-harness，TypeScript）

DeepSeek 首个官方 Agent 运行框架，核心范式是**事件溯源 + 事件驱动 agent loop**（非状态机/图）：

| 机制 | 要点 | 对本项目启示 |
|------|------|-------------|
| **"Model-visible means logged" 铁律** | 凡进入模型请求的内容必须可从 append-only session 日志重建，且有**运行时不变量断言**；fork/resume/replay/遥测全部从同一事件流派生 | 我们有 event.Bus + activities 持久化双轨，但缺"模型可见即可重建"的运行时断言——可作为 Sequencer 的不变量校验引入 |
| **Turn/Step 二级语义** | step = 一次模型请求 + 其触发的工具调用；turn 在"无欠账"（工具结果消化完、无新输入）时关闭；被 pre-step 拒绝的 0-step turn 也落日志 | 校准我们 Turn/Step 生命周期边界；0-step turn 落日志便于审计 |
| **Waterfall 拦截点** | `agent/pre-step`（可改写或拒绝）、`agent/request`、`tools/pre|post-execute` 是必须 `next()` 委托的瀑布链；策略全挂拦截点不改 loop | 我们中间件链可引入"改写 vs 拒绝"显式决策返回类型（reject \| enter(messages)） |
| **Inbox 三级唤醒语义** | `followup`（排队新 turn 并唤醒）/ `steer`（注入下一 step 边界）/ `inject`（静默上下文不唤醒）；`next-turn`/`next-step` 双有序队列，所有变更落持久化事件 | 直接解决 Team 协作中"用户插话 vs 系统注入"的时序难题，可用于主 agent 与 member session 消息路由 |
| **Subagent 能力缝 fail-loud** | `SubagentCapabilities{outputSchema, depthLimit, toolFilter, persona}` 启动前校验，不支持即 `UNSUPPORTED_CAPABILITY` 报错，禁止静默降级；`maxDepth` 绝对委派深度上限；`toolFilter` "可见性即权限"（过滤即从提示词消失且拒绝执行） | 我们 Tool/Channel/子代理注册可引入同类能力声明；Team 派生树需要深度上限治理 |
| **取消原因类型化** | `AgentCancelCause = user \| parent \| hook \| disposed`，父取消级联子 agent，dispose 与取消语义分离 | 我们 Run 取消粒度较粗，可借鉴 |
| **思考强度路由** | V4 模型三档 effort（low/high/max），日常 agent 任务推荐 high | 模型路由层按任务复杂度选档，成本治理手段 |
| **Preset = 插件组合** | Standard/Minimal/Creator 共享同一内核，评测态与生产态一致 | Graph/Team 可以"可组合 profile"提供评测态/生产态一致性 |

### 2.2 PiAgent 消歧（两个真实所指）

**"PiAgent" 不是单一权威系统**，消歧后两个所指均有价值：

**候选 A：Ensemble QSP 的 PI Agent（arXiv:2607.07666，阿斯利康，2026-07）**——PI = Principal Investigator：

| 机制 | 要点 | 对本项目启示 |
|------|------|-------------|
| **有界三层记忆** | 短期滚动窗（专家 ≤20k 字符）+ 中期结构化项目状态 JSON（只注入任务相关切片，实测中位 301 token）+ 长期领域手册；注入量与项目时长解耦 | 我们 Session/Memory 可用结构化 project-state 替代对话历史拼接；与 L0 压缩设计互补 |
| **模型级联实证** | 前沿 PI + 中档 Reviewer + 低成本 Writer：精度不变（11/11 全对）、成本约半（$5.14 vs $10.51）；**但低成本 Writer 输入 token 5.5 倍**（反复自修正） | 为 model router 提供论文级佐证；警示：路由策略按**总成本**而非单价评估 |
| **质量门禁 = checklist 注入** | Review 环节自动注入领域校验清单（非靠模型自觉） | 可在工具/评审节点实现"域校验器 + 自动上下文注入" |
| **角色非重叠 + 配置化 Leader** | 5 个 Worker 职能严格不重叠；新领域 = 新 Leader 配置（prompt+checklist+领域 JSON），orchestrator 零改动 | Team/Graph 定义与领域配置解耦，Leader 行为配置化 |
| **权衡警示** | 多 Agent 协调墙钟 ~4-5 倍（105 vs 18-25 分钟） | 延迟敏感链路需提供单进程串行降级路径 |

**候选 B：pi-agentteam（Pi 生态社区编排）**：

| 机制 | 要点 | 对本项目启示 |
|------|------|-------------|
| **任务看板归 Leader 所有** | 任务事实单一写者；Worker `report_done`/`report_blocked` 显式汇报；Leader 评审后显式启动下游 | 对应 PlanBoard 单一写者原则，避免多写者冲突 |
| **类型化消息** | `assignment`/`question`/`inform` 三型 + 任务 ID 路由 + 解除阻塞时事件通知 | 与 event.Bus/FlowLog 天然契合，可规范 Team 内部消息协议 |
| **最小权限工具护栏** | researcher/planner 只读 → implementer 全工具 | 按 agent 角色下发工具白名单 |

### 2.3 阿里 AgentTeams / HiClaw（2026-03 开源 → 2026-05 商业化）

| 机制 | 要点 | 对本项目启示 |
|------|------|-------------|
| **三层 Leader-Worker** | Manager → TeamLeader（可跳过、可转移）→ Worker；声明式 CRD（Manager/Team/Worker/Human 四种 Kind） | 我们 dept_lead 已有借调概念；分层治理思路一致，缺"可转移 Leader"语义 |
| **通信治理即拓扑** | `peerMentions`/`channelPolicy` 字段控制 Worker 互 @ 与跨 Team 通信；"老板不越级"由房间拓扑约束 | 我们 Team 内部通信可用声明式策略字段约束 |
| **动态激活/深休眠** | Manager 自动停空闲 Worker、派任务时拉起；ACS Sandbox 快照深休眠不计费、秒级恢复；Worker 进程内 Subagent 链串行（零网络跳数） | 长任务场景的成员驻留成本治理方向（P3 候选，架构级） |
| **网关层模型 fallback** | 主模型 4xx/5xx/超时自动切备选（仅降级一次）+ 令牌级健康摘除 | 我们 ModelSelector 目前仅 "default/auto"，无 fallback 级联——P2 直接可补 |
| **进化飞轮** | 执行轨迹/工具调用日志 → 清洗/自动评估/SFT/RLHF → 反哺 Prompt/技能库/组织结构 | 我们已有 self_improvement 管线，可对齐"轨迹→评估→重构"闭环（P3） |
| **凭证零信任** | 网关托管真实凭证，Worker 持 Consumer Token（工牌），per-route 授权 | 安全向参考，本期不展开 |

### 2.4 前沿论文与框架（前序会话已调研，结论并入）

- **MAST（多智能体失败分类法）**：14 种失败模式归类为规范/系统/验证三类——作为 P3 评估 rubric 与 FlowLog 失败标注的词表来源
- **ADAS / AFlow**：自动化 agent 结构与工作流搜索——P3 自进化编排的理论依据，短期不落地
- **LangGraph**：checkpoint/thread 恢复语义、durable execution——P1 可靠性对齐的参照系
- **AutoGen / OpenAI Agents SDK**：消息协议与 handoff 语义——Team 内部消息协议规范化参考

---

## 三、现状差距矩阵（对照调研 → 本项目）

| 维度 | 外部最佳实践 | 本项目现状 | 差距等级 |
|------|-------------|-----------|---------|
| 事件溯源不变量 | DSH "model-visible means logged" + 运行时断言 | event.Bus + activities 双轨持久化，无重建断言 | **中**（P1） |
| Turn/Step 边界 | DSH 二级语义 + 0-step turn 落日志 | Task/Turn/Step 实体齐备（v2 状态机已有），边界语义未显式化 | 低（P1 文档+校验） |
| 插话/注入时序 | DSH followup/steer/inject 三级唤醒 | Team 运行中注入语义粗糙 | **中**（P1/P2） |
| 子代理治理 | DSH maxDepth + toolFilter + fail-loud 能力校验 | 无委派深度上限；工具按角色过滤未见 | **中**（P1） |
| 取消语义 | 类型化 cause + 父级联子 | Run 取消粒度粗 | 低（P2） |
| 记忆分层 | Ensemble QSP 有界三层 + 结构化 project-state | 有 memory L0-L4 + 压缩注入，无"中期 project-state JSON"形态 | 中（P2） |
| 模型级联 | 前沿 Leader + 低成本 Writer 实证有效；网关 fallback | ModelSelector 仅 default/auto，无级联/fallback/成本核算 | **高**（P2，收益最直接） |
| 评测/生产一致性 | DSH preset 共享内核 | 无评测态 profile | 低（P3） |
| 自进化闭环 | AgentTeams 轨迹→评估→重构飞轮；MAST rubric | self_improvement 管线已有雏形，未接编排轨迹 | 中（P3） |
| Sequencer 认知复杂度 | — | struct 26 字段（上限 15），已挂 TECH-DEBT(COG) | **高**（P1 偿还） |
| v1/v2 双轨 | — | task_orchestrator/task_planner 新旧路径并存 | 中（P1 收敛） |

---

## 四、最终方案

> 原则：P1 只做可靠性与债务偿还（不改外部行为契约）；P2 做效率与智能调度（可配置、默认保守）；P3 做自进化（先观测后自动）。每批独立门禁。

### P1 — 可靠性对齐与债务偿还（先行批次）

| # | 任务 | 来源 | 改动要点 | 涉及文件 | 验证 |
|---|------|------|---------|---------|------|
| P1-1 | **Sequencer 拆分**（偿还 TECH-DEBT(COG)） | AS-COG-01 | 26 字段按职责拆为：事件持久化子管理器（persistChan/retry/deadLetter）、流式批合子管理器、定时器子管理器；构造函数注入不变，外部行为零变化 | `internal/agent/v2/sequencer.go`（拆出 `sequencer_persist.go`、`sequencer_stream.go`、`sequencer_timer.go`） | 既有 race/可靠性回归测试全绿 |
| P1-2 | **事件溯源不变量校验** | DSH §2.1 | 在 Sequencer 增加开发态断言：模型可见的 step 输入必须可从活动事件流重建（校验入口放在 debug 构建或 `ARANEA_ORCH_INVARIANT=1`）；不达标即 Error 级进程日志 + FlowLog warn | `internal/agent/v2/sequencer.go`、`internal/agent/v2/invariant_check.go`（新增） | 单测构造不可重建场景触发断言 |
| P1-3 | **拦截点显式决策类型** | DSH §2.1 waterfall | 在 tool 确认门（tool_confirm_gate）与 pre-step 注入链引入 `Decision{Reject \| Rewrite(msg) \| Pass}` 显式返回，替换布尔/隐式语义 | `internal/agent/tool_confirm_gate.go`、`callback_chain.go` | 既有确认门测试适配通过 |
| P1-4 | **子代理委派深度上限 + fail-loud 能力校验** | DSH subagent | Team/Graph 派生链记录 depth，超过 `maxDelegateDepth`（默认 4，可配）拒绝并报 `UNSUPPORTED_CAPABILITY` 类错误；禁止静默截断 | `internal/biz/team_usecase.go`、`internal/agent/agent_factory.go` | 单测：深度 5 派生被拒 |
| P1-5 | **v1/v2 编排路径收敛盘点** | 差距矩阵 | 全局搜索 v1 编排残留调用点，输出收敛清单（哪些调用点仍走旧路径、能否切换），只盘点+切换低风险点，不强行删 | `internal/biz/task_orchestrator.go` 等 | grep 报告 + 切换点测试绿 |

> **P1-5 盘点结论（2026-08-14 执行）**：✅ 事件路径已收敛——Sequencer 仅 v2 一份（`internal/agent/v2/sequencer.go`），v1 双轨仅存于注释；编排策略层分工明确非双轨：`TaskOrchestratorImpl`（agent 层策略：direct/single/team/DAG/NL2Graph）、`ChatOrchestrator`（service 层 chat 入口）、`RealTeamOrchestrator`（service 层 team 执行，stub 为降级）。**无残留切换点，无需代码改动。**
| P1-6 | **取消时非终态 step 清扫**（0-step 审计的本项目形态） | DSH turn 语义 | dagRun 取消后，in-flight running step（dispatch 走 `ctx.Done()` 裸返回不落终态）与 never-dispatched pending step 永久滞留 DB——wg 屏障后统一清扫为 Skipped + PlanStepSkippedEvent + GraphNode→Interrupted | `internal/service/plan_executor.go` | 单测：取消后两 step 均 Skipped 且事件齐全 |

### P2 — 效率与智能调度

| # | 任务 | 来源 | 改动要点 | 涉及文件 | 验证 |
|---|------|------|---------|---------|------|
| P2-1 | **模型级联路由（Leader/Worker 分档）** | Ensemble QSP 实证 + HiClaw fallback | 扩展 ModelSelector：新增 `cascade` 模式——Team Leader/planner 用高档模型、member/executor 用成本档；**按总 token 成本核算**（记录每 run 的分档 token 用量），非仅单价 | `internal/biz/agent_settings.go`、`internal/agent/agent_allocator_impl.go`、`internal/biz/usage` | 单测：级联选择正确分档 + usage 记账字段齐全 |
| P2-2 | **模型 fallback 降级（仅一次）** | HiClaw 网关 | 主模型 4xx/5xx/超时自动切备选模型一次，FlowLog 记录降级事件（K3 节点） | `internal/agent/llm_caller_impl.go` | 单测 mock 失败→切换→不再二次降级 |
| P2-3 | **Inbox 三级注入语义** | DSH followup/steer/inject | Team 运行中消息注入分级：用户插话=steer（下一 step 边界消费）、系统上下文=inject（静默排队）、显式追问=followup（唤醒新 turn） | `internal/biz/team_usecase.go`、`internal/agent/v2/` | 集成测试：三级语义时序正确 |
| P2-4 | **中期 project-state JSON** | Ensemble QSP 有界记忆 | Team/长任务场景引入结构化项目状态（活跃请求/最近变更/里程碑/决策摘要），滚动更新、按切片注入，替代对话历史全量拼接 | `internal/biz/team_run.go`、`internal/agent/memory_inject.go` | 单测：注入切片 ≤ 预算、字段滚动正确 |
| P2-5 | **思考强度路由** | DeepSeek V4 effort | 按任务复杂度选 thinking 档（简单=off/low、日常=high、复杂=max），接入模型调用参数 | `internal/agent/llm_caller_impl.go`、`agent_settings.go` | 单测：分档映射正确 |
| P2-6 | **取消原因类型化 + 父级联子** | DSH AgentCancelCause | Run/TeamRun 取消带 cause（user/parent/system/disposed），父取消级联子 run，终态事件带原因 | `internal/biz/*state_machine*.go`、`internal/agent/v2/` | 单测：级联取消终态正确 |

### P3 — 自进化编排（观测先行）

| # | 任务 | 来源 | 改动要点 | 涉及文件 | 验证 |
|---|------|------|---------|---------|------|
| P3-1 | **编排轨迹采集 → MAST 失败标注** | MAST + AgentTeams 飞轮 | 从 activities/FlowLog 提取 Team/Graph run 轨迹，按 MAST 14 失败模式词表自动标注（规则+LLM 双通道），先入库存观测 | `internal/biz/self_improvement_*`、新增 `internal/biz/orchestration_trace.go` | 单测：标注词表映射正确 |
| P3-2 | **评估 rubric + 反哺闭环** | AgentTeams 双飞轮 | 标注结果接入既有 self_improvement 管线：bad case → prompt/技能重构建议（先人工评审后应用） | `internal/biz/self_improvement_router.go` 等 | 集成测试：bad case 生成建议记录 |
| P3-3 | **成员驻留治理（动态激活）** | HiClaw 深休眠 | Team 长任务空闲 member session 超时挂起（状态落库、唤醒时恢复），降常驻成本——架构级，需 ADR | `internal/biz/team_usecase.go`、`internal/agent/v2/` | ADR + 集成测试 |
| P3-4 | **评测态 profile** | DSH preset | Graph/Team 提供评测 profile（固定 seed/模型/工具集），与生产同内核 | 编排入口层 | 文档 + 冒烟 |

### 架构级改动（须 ADR，随批次提交）

| ADR | 决策 | 批次 |
|-----|------|------|
| ADR-A | Sequencer 拆分为子管理器组合（P1-1） | P1 |
| ADR-B | 事件溯源不变量作为开发态强制断言（P1-2） | P1 |
| ADR-C | 模型级联路由策略与总成本核算口径（P2-1） | P2 |
| ADR-D | 成员 session 挂起/恢复语义（P3-3） | P3 |

---

## 五、执行批次与门禁

| 批次 | 内容 | 门禁 |
|------|------|------|
| Batch-0 | P0 门禁收尾（等并行会话 mcp_oauth 落地后全量 race+test） | `go test ./internal/agent/ ./internal/agent/v2/ -race` 绿 |
| Batch-1 ✅ | P1-1 + P1-5 + P1-6（纯内部重构，零契约变化） | agent+v2 全量 race 绿（agent 29.8s / v2 5.8s）+ service 绿 + `go build ./cmd/... ./internal/... ./api/... ./pkg/...` 绿（2026-08-14 通过） |
| Batch-2 ✅ | P1-2 + P1-3 + P1-4（行为边界显式化） | v2 全量 race 绿 + agent(27.4s)/callbacks/subagent 绿 + build 绿（2026-08-14 通过）；新增 invariant_check.go 15 个单测 + decision_test + delegation_depth_test。偏差记录：P1-2 FlowLog warn 支路暂缓——v2 层无 FlowLogWriter 端口（红线 3），先仅进程日志观察误报率 |
| Batch-3 ✅ | P2-1 + P2-2（模型路由，收益最直接） | agent/event/team 全量绿（agent 27.6s）+ fallback race 绿 + vet 绿（2026-08-14 通过）；新增 model_selector_cascade_test + team_cascade_test + llmcaller_fallback_test（6 用例）。usage 记账回归：分档用量经 FlowLog `team.model_cascade.route` 聚合（ADR-C），未改 usage Schema。ADR 落盘：[ADR-C](./2026-08-14-review-adr-model-cascade.md) |
| Batch-4 | P2-3 + P2-4 + P2-5 + P2-6 | 同上 |
| Batch-5 | P3-1 + P3-2（观测闭环） | 同上 |
| Batch-6 | P3-3 + P3-4（架构级，先 ADR 评审） | ADR 评审 + 全量 |

每批次遵循：TDD（先失败测试）→ 实现 → 门禁 → 文档同步（三件套状态标记）。

---

## 六、风险与注意事项

1. **并行会话干扰**：当前有会话在改 `internal/biz/agent_usecase.go` 拆分与 MCP OAuth TDD。执行时避开 `wire.go`/`wire_gen.go` 及对方在改文件，遇编译断裂先重读确认对方进度。
2. **P2-1 模型级联的 token 放大风险**：Ensemble QSP 实证低成本模型输入 token 可放大 5.5 倍——级联必须配总成本核算与上限熔断，不能只看单价。
3. **P1-2 不变量断言的误报风险**：先以"仅日志、不阻断"模式上线观察，再决定是否升级为阻断。
4. **多 Agent 墙钟开销**：Ensemble QSP 实测 4-5 倍协调开销——P2/P3 所有"智能化"不得引入同步等待链路。
5. **GOCACHE 幻影**：并行工作流下一切"编译通过"结论以干净缓存为准（`$env:GOCACHE` 独立目录复跑）。

---

## 七、调研信息来源

- DeepSeek Harness：[github.com/deepseek-ai/deepseek-harness](https://github.com/deepseek-ai/deepseek-harness)、[架构文档](https://github.com/deepseek-ai/deepseek-harness/blob/master/docs/architecture.md)、[core 子系统](https://github.com/deepseek-ai/deepseek-harness/blob/master/docs/subsystems/core.md)、[subagent 子系统](https://github.com/deepseek-ai/deepseek-harness/blob/master/docs/subsystems/subagent.md)、[DeepSeek API 更新日志](https://api-docs.deepseek.com/updates/)、[Thinking Mode 指南](https://api-docs.deepseek.com/guides/thinking_mode)
- Ensemble QSP：[arXiv:2607.07666](https://arxiv.org/abs/2607.07666)
- pi-agentteam：[npm/jsdelivr](https://www.jsdelivr.com/package/npm/pi-agentteam)、[Pi-Agents-Team](https://github.com/KristjanPikhof/Pi-Agents-Team)
- 阿里：[HiClaw/AgentTeams GitHub](https://github.com/alibaba/hiclaw)、[AgentTeams 产品页](https://www.aliyun.com/product/agentteams)、[阿里技术实践](http://news.qq.com/rain/a/20260611A01KH400)
- 存疑已排除：网传 DeepSeek "MSRC 五阶闭环/Thinker Engine" 为第三方营销文案，官方渠道无此物，不采信。
