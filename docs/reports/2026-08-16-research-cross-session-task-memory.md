# research: 跨会话长任务记忆（视频观点 × Aranea 现状对照）

> 日期：2026-08-16
> 作者：AI（Trae）
> 版本：v1.0
> 素材：抖音视频《Agent跨会话长任务怎么记？别存对话》（小哲讲agent面试，2026-08-15，https://v.douyin.com/7EsYwWS5Y8Y/）
> 关联文档：`docs/development/70-orchestration-longtask-memory.*` 三件套、`docs/reports/2026-06-17-research-orchestration-longtask-memory-upgrade.md`

---

## 摘要

视频核心观点：**跨会话长任务不要存对话，要存任务的增量快照**——用任务状态机记"做到哪了"，用关键证据链记"为什么"，状态层轻量必读、证据层按需检索。对照排查结论：**Aranea 图执行/团队域已是该观点的完整落地（checkpoint + 显式状态机 + 交付物引用化），chat 域有压缩级联与 L0-L4 分层记忆兜底，但缺少面向模型的结构化"任务状态表"，工具结果证据仍是全量入史、截断式丢弃**。四个可借鉴的增量优化点见 §五。

---

## 一、视频内容纪要

> 来源为视频页官方文案与 AI 章节摘要，未获取逐字稿。

**面试题**："一个任务跨了三次会话，Agent 怎么记住做到哪了？"

**错误答法**：每次把历史对话全带上——上下文窗口扛不住，且噪音淹没信号。

**正确做法**（视频章节脉络）：

| 章节 | 要点 |
|------|------|
| 真实场景 | 跨会话长任务全量回放对话的工程痛点 |
| 正确做法 | 设计状态系统：**存任务增量快照，不存对话** |
| 分层设计 | **任务状态机**（状态层，轻量必读）+ **关键证据链**（证据层，按需检索），存储复杂度从 O(全量对话) 降为 O(状态变更) |
| 面试回答 | 强调两件事：**任务状态表设计**（字段抽象、跨会话可恢复）与**按需检索证据路径**（状态是索引，证据按需加载） |

---

## 二、排查范围与方法

对照视频四个主张，逐一核实 Aranea 代码现状：

1. 存增量快照而非全量对话
2. 任务状态机
3. 关键证据链
4. 状态层/证据层分层 + 按需检索

排查路径：`internal/graph/adapter/`、`internal/agent/`（L0 快照/压缩）、`internal/compress/`、`internal/biz/`（状态机族/交付物契约/session_run）、`internal/tools/deliverable/`、`pkg/trpc-agent-go/internal/flow/llmflow/`、`docs/development/70-*.development.md`（落地状态）。

---

## 三、Aranea 现状对照（已实现部分）

### 3.1 图执行域：教科书级"存状态不存对话" ✅

| 视频主张 | Aranea 实现 | 证据 |
|---------|------------|------|
| 增量快照 | 框架 CheckpointSaver，Postgres `sessionruncheckpoint` 表；**P1-8 强制所有图执行启用 checkpoint**，进程重启可恢复 | `internal/graph/adapter/runtime_adapter.go:783-808` |
| 跨会话恢复不带对话 | Resume 仅传 `CheckpointRef(lineageID)` + `ResumeChannel` 值，空 message 不落库 | `runtime_adapter.go:248-323`（`Resume`） |
| 任务状态机 | GraphExecution 显式状态机：5 状态 7 转换，authoritative 模式（非法转换拒绝） | `internal/biz/graph_execution_state_machine.go`；70 模块 P0-2 ✅ |
| 状态可查可改 | TimeTravel：GetState / History / EditState | `runtime_adapter.go:335-391` |
| 历史装配 | 2026-08-16 根治：执行驱动走框架 `runner.Run`（注入 sessionService），llmflow 从 session 累积工具/LLM 历史，消息逐轮累积（4→6→8→10 实测） | `runtime_adapter.go:140-157` 注释 |

**点评**：2026-08-16 的"模型全盲"根治，本质就是从"裸调 agent.Run（无状态）"修正为"状态外置 + 框架装配历史"，与视频观点方向完全一致且更深（HITL Interrupt/Resume、TimeTravel 是视频未覆盖的能力）。

### 3.2 团队域：状态层/证据层分离已落地 ✅

| 视频主张 | Aranea 实现 | 证据 |
|---------|------------|------|
| 状态层轻量必读 | 交付物信封 v2：summary 三段式（结论+要点+载荷清单）随上下文传递 | 项目记忆「交付物信封 v2 长文交付范式」 |
| 证据层按需检索 | 长文本体存 `structured_json` 的 `{"title","format","content"}` 契约键，下游用 `read_upstream_deliverable(team_id, key)` 工具按 key 取全文（返回 620KB tail 截断保护） | `internal/tools/deliverable/upstream_reader.go:17-43`、`internal/tools/decorator.go:118-122` |
| 契约校验 | 上下游 DeliverableContract（name/type/format/schema），不匹配返回 LLM-actionable 的 `ContractMismatchError` | `internal/biz/deliverable_contract.go:55-87` |
| 状态字段化 | `EnableStateDeliverable` 注入 deliverable StateField，成员经 set/get/ack 工具读写 | `runtime_adapter.go:582-610` |

### 3.3 chat 域：压缩级联 + 分层记忆兜底 ✅（被动式）

- **压缩级联默认开**（2026-08-13 N2 修复）：`ContextCompactionEnabled` / `MemoryCompactEnabled` / `SessionSummaryEnabled` 默认 true——修复前框架摘要消费侧从未生效，历史无上限增长（`__spirit__` 实测平均 prompt 60K tokens）。`internal/biz/agent_defaults.go:127-141`
- **双压缩统一**（2026-07-20）：软触发走 LLM 摘要（`internal/compress/service.go`），硬触发走确定性 emergency truncation；结果写 L0 快照。`internal/agent/context_compression_inject.go`、`internal/agent/l0_snapshot_persist.go:190-199`
- **L0-L4 分层记忆注入**：L1 working / L2 episodic / L3 semantic（召回预算 800 tokens 档）/ L4 knowledge graph（邻居 6、2 跳）。`l0_snapshot_persist.go:539-571`（分段统计可见各层）
- **Durable Resume**：turn 级崩溃恢复（`TryClaimDurableResume` + `DurableResumeSpec`）。`internal/biz/session_run.go:268-277`、`internal/event/durable_resume_context.go`
- **状态机族齐全**：Run / TeamRun v1+v2 / Team / TeamStage / GraphStage / Activity / Task(agentbridge) 均有显式状态机（AS-FSM-01）。
- **70 模块 Phase 0-3 基本完成**（仅记忆增强 E-1/E-2 两项 ⏳，与本主题弱相关）。

---

## 四、差距分析（对照视频分层设计）

### G1：chat 域缺"任务状态表"——进度记忆靠摘要推断，不靠状态字段

视频强调的核心产物是**任务状态表**（任务 ID、阶段、已完成步骤、下一步指针）。Aranea chat 域跨会话恢复注入的是：

```
系统提示 + L1/L2/L3/L4 记忆 + session summary（自由文本）+ 最近窗口历史
```

session summary 是**叙事性自由文本**，模型需从中"推断"进度；没有结构化的 `goal / done[] / next / blockers` 字段。对"三次会话后精确续跑"场景，自由文本摘要的进度保真度低于结构化状态表。

> 图执行域无此问题——图拓扑本身即业务状态机，checkpoint 精确到节点。差距集中在 chat 长任务（Spirit 多轮编排但未走图的场景）。

### G2：工具结果证据"全量入史 + 截断丢弃"，未引用化

团队间交付物已引用化（G3.2），但**单 agent / 图节点内的普通工具结果**仍全量进 session 事件历史；超限后走 `decorator.go` 的 MaxBytes tail 截断——**截断即丢失**，不是"留引用、按需取"。图执行 runner 明确未注入 artifact 服务（`runtime_adapter.go:633-637` 注释："不注入 memory/artifact"）。

长图执行中 session 事件（工具/LLM 历史）随节点数线性膨胀，目前只能靠 context compaction 触阈值后被动压缩。

### G3：压缩是"损失式"，产物无结构

emergency truncation 直接丢消息；LLM summary 是自由文本。两者都不产出结构化任务状态。视频观点的启示：压缩产物应从"摘要"升级为"**任务状态快照 + 摘要**"双段式——状态段管精确进度，摘要段管语义连续。

### G4：框架消息过滤能力未启用

框架提供 `WithMessageTimelineFilterMode` / `WithEventFilterKey`（分支级历史隔离），项目标注"未使用，评估中"（`docs/trpc-agent-go/02-agent.md:293`）。图执行全节点共享同一 session（execID 维度），节点间历史互相可见——对长跑图，节点级历史隔离或"节点完成即摘要入 state"可进一步压缩上下文。

---

## 五、对 Aranea 的启发与优化建议

> 按投入产出排序，均为增量增强，不改框架（遵守 FW-R1）。

### 建议 1（优先）：chat 长任务引入结构化 TaskBoard 状态字段

- **做法**：session 级 `task_board` JSON 字段（goal / done_steps[] / next_step / blockers[] / updated_at），turn 末由轻量 pass 增量更新（增量快照语义）；跨会话装配时作为 system 段注入（轻量必读），与 session summary 并存——**状态表管"做到哪"，摘要管"聊了什么"**。
- **落点**：`agent_runtime_settings` 加开关；装配在 prompt builder 的记忆注入段旁；更新钩子挂 turn 完成路径（参照 auto_memory 挂法，注意三消费方排查教训——先 grep 全部调用方）。
- **验证**：构造跨 3 会话长任务（如多轮排障），对比有/无 TaskBoard 时模型对"下一步"的回答准确率与 prompt token 量。

### 建议 2：工具结果 artifact 化 + 引用句柄

- **做法**：大 payload 工具结果（如 gns3_exec 全量回显）落 artifact/对象存储，session 历史只留 `artifact_ref + 头部摘要`；配 `read_artifact(ref, range)` 工具按需取——把 deliverable 信封 v2 的"引用化"模式从团队间推广到工具结果。
- **前提**：图执行 runner 需注入 artifact service（当前显式不注入），属业务层接线，不动框架。
- **注意**：先评估 decorator 现有 tail 截断的实际命中率，命中率低则收益有限，避免过度工程。

### 建议 3：压缩产物双段化

- **做法**：`internal/compress` 的摘要 prompt 增加结构化段落输出（任务状态 JSON + 叙事摘要），写入 session summary 时拆两字段；装配时状态段全量、摘要段按预算裁剪。
- **收益**：G1 的低成本替代方案——不新增存储，只升级压缩产物结构。

### 建议 4（观察项）：图节点历史隔离评估

- 启用框架 `TimelineFilterMode` / 分支过滤的评估（现标注"评估中"），或节点完成时将结果摘要写 checkpoint state、后续节点以 state 为主历史为辅。
- **建议先观测**：用 L0 快照统计长图执行各节点的 `history` 段 token 占比，超阈值再动手。

### 不建议做的

- **不要**为 chat 域另建一套 checkpoint 体系——图执行的 checkpoint/状态机已验证有效，chat 长任务的正确路径是"复杂任务转图执行"（70 模块 NL2Graph/强制规划已具备），而非在 chat 里重造。
- **不要**把全量对话从 session 表移除——事件全量落库是审计/TimeTravel/证据链的底座，视频反对的是"全量进上下文"，不是"全量持久化"。Aranea 现状（全量持久化 + 裁剪装配）方向正确。

---

## 六、结论

视频观点在 Aranea 中的映射：**图执行/团队域 = 完整实现甚至超标**（checkpoint + 状态机 + TimeTravel + 交付物引用化）；**chat 域 = 方向正确（压缩级联 + 分层记忆）但停留在"摘要推断进度"阶段**。最有价值的借鉴是把"任务状态表"作为一等公民引入 chat 长任务（建议 1/3），把"证据按需检索"从团队交付物推广到工具结果（建议 2）。三者均为业务层增量，符合"基于 trpc 框架增强、不另起炉灶"的既定原则。
