# 调研：Leaderboard G 维度（Context Learning, Rules & Workflow Execution）提升方案

> 触发：域 B 复测后用户追问「G 规则学习执行是指哪方面，我们没有吗，如何提升」。
> 口径说明：官方未公开叶子指标定义（站点仅给维度名），本文按维度命名 + 行业同构基准（LongMemEval / CL-Bench / ContinualSkillBench）推断能力面，并以本仓库代码证据盘点现状。

## 1. G 维度测什么

按命名拆解为三个子能力：

| 子能力 | 含义 | 典型考法 |
|--------|------|---------|
| **Context Learning**（上下文学习） | 从当前对话/文档上下文中学习预训练之外的新规则、契约、格式约定，并在后续作答中遵守 | 给定一份本地规程/标签体系/输出契约，任务答案必须满足全部条款（conjunctive pass，漏一条即失败） |
| **Rules**（规则记忆与一致执行） | 用户授与的 standing instruction（格式偏好、禁令、审批约束）跨会话长期一致执行，不被遗忘、不被相似度召回漏掉 | 植入规则 → 长间隔后多轮提问，检查每次回答都符合规则 |
| **Workflow Execution**（工作流记忆与执行） | 学过的多步流程/SOP（步骤、顺序、条件、工具）能正确复现执行，并从执行经验中改进 | 教会一个操作流程 → 之后要求执行同类任务，检查步骤齐全、顺序正确、条件分支对 |

**榜单观察**：G 是全行业最低分维度——MemoraX 29.99、MemOS 9.82、NTES-SMART 27.74（对比 A 维度 89.89/68.95/55.57）。G 是当前的**区分度维度**：大多数系统只做「事实存取」，不做「规则/流程的习得与执行保障」。我们首评 G=29.0（部署异常期），与榜首持平，提升空间真实存在。

## 2. 现状盘点（代码证据）

| 能力 | 现状 | 证据 |
|------|------|------|
| 规则事实写入 | **有** | `instruction/agent_instruction` 类型 → agent 作用域（`immediate_fact_writer.go`，R1 规则） |
| 规则召回注入 | **有，但走相似度门控** | instruction 类事实进 L3 融合召回（打分+budget 门控），**无钉住通道** |
| 偏好钉住注入 | **有（仅 preference）** | [agent_memory_runtime_policy.go:14-15](file:///f:/myproject/aranea-agents/internal/biz/agent_memory_runtime_policy.go#L14-L15)：resident profile card 与 **pinned preference block 不受 budget 门控，100% 注入**——E 维度强的原因；但**该通道未覆盖 instruction 规则** |
| 规则实际执行 | **软约束有效（观测）** | 域 B 评测 ab 系列推理 trace 显式引用并执行「禁止无证据断言」「敏感信息不写入记忆」；49 条回答全量遵循表格化偏好 |
| 规则执行校验 | **无** | 注入后无 output guard/合规校验，纯靠模型自觉 |
| 任务经验库（Agent Case） | **有（M2/M3）** | [agent_case.go:24-37](file:///f:/myproject/aranea-agents/internal/biz/agent_case.go#L24-L37)：goal/approach/outcome/pitfalls/tools_used 结构化落 `memory_agent_cases`，M3 召回注入已接（评测中观测到"历史任务经验"被引用）；**approach 为自由文本，无步骤结构** |
| case→skill 蒸馏（M4） | **未做** | 代码注释标明 M4 为后续消费方，工具经验未聚合为可复用 skill |
| SOP/工作流记忆 | **无** | 无 SOP 实体；图执行（graph runtime）与记忆系统无双向关联（执行过的图不沉淀为可召回流程） |
| G 维评测基准 | **无** | 主计划域 G（Skill/规则）未建；域 B 的 50 题不覆盖 |

## 3. 差距分析（四个断点）

1. **规则召回是概率性的**：instruction 与事实共用相似度门控召回 + budget 截断——问句与规则语义不近时规则缺席，规则执行从根上不稳定。preference 已有 100% 钉住先例，instruction 没有。
2. **执行端零保障**：规则命中后无校验回路。CL-Bench 数据（GPT-5.1 单条要求 76% 通过 vs 整任务 22.55% 通过）证明：软约束下「漏一条必含字段/一个精确标签」是行业通病，conjunctive 计分下致命。
3. **经验无结构、不蒸馏**：case 的 approach 是自由文本，无法做步骤级召回与执行跟踪；tools_used/pitfalls 未按工具聚合，同样的坑反复踩（本次评测 knowledge_search 降级空转即活例——该经验未沉淀为 pitfall 反哺）。
4. **流程记忆缺位**：SOP 无实体、无存储、无执行对齐，Workflow Execution 子能力裸奔。

## 4. 综合提升方案（P0→P4）

### P0 本地 G 维基准先行（评测驱动，无猜兵）
- 建 `docs/testing/agent-eval-20260818/0G-rules-workflow/`：3 类 × 10 题——
  - **G1 规则一致性**：植入 5 条 standing rule（格式/禁令/称谓/审批口径），20 轮后 10 题抽查每次回答全合规；
  - **G2 流程复现**：教会 3 个运维 SOP（含步骤顺序+条件分支），之后要求执行变体任务，按步骤点计分；
  - **G3 上下文契约**：给长规程文档+任务，按 rubric 条款 conjunctive 计分（CL-Bench 缩小版）。
- 判分沿用双轨（自动+人工复核），先出基线再优化。

### P1 规则钉住通道（投入最小、收益最直接）
- 将 `instruction` 类事实纳入 pinned block（与 preference 同通道，100% 注入、不受 recall budget 门控）：改动集中在 runtime policy 的注入装配处。
- 规则数量级小（agent 作用域通常 <50 条），全量注入成本可控；超长时按 priority/confidence 截断。
- 规则冲突与新旧覆盖：复用 bi-temporal 机制，同槽位规则新写即 supersede 旧写（治理侧已有 D 维验证基础）。
- **收益映射**：Rules 子能力从「概率执行」变「确定执行」。

### P2 规则执行校验回路（output guard 轻量版）
- 对**可机检规则**（格式类：必须表格/必含字段/禁用词/精确标签/JSON schema）在提取时打 `verifiable` 标记 + 机器校验器；回答生成后自检，违规则一次修复重生成。
- 对**不可机检规则**走 LLM-judge 抽检（采样率可配），违规事件写入当前 case 的 pitfalls（经验回流 P3）。
- 借鉴 CL-Bench 的 specification-induction：检测规则密集上下文时，prompt 引导「先列合规清单再作答」（纯 prompt 层模式，可先实验）。
- **收益映射**：把「76% 单条通过 → 22% 整任务通过」的漏斗补上，对应 Context Learning 与 Rules 的 conjunctive 计分。

### P3 经验结构化与 case→skill 蒸馏（M4 落地）
- `AgentCase.Approach` 升级结构化：`steps[]{order, action, tool_key, condition, expected_result}`，自由文本作 fallback（兼容存量）。
- 工具经验聚合：按 `tool_key` 聚合 tools_used/outcome/pitfalls → 「工具使用手册」注入工具目录块（如「gns3_exec 前先 health_check」「knowledge_search 降级时停止重试」——直接消化本次评测发现的空转教训）。
- 与循环守卫联动：被拦截的调用模式自动沉淀为 pitfall。
- **收益映射**：Workflow Execution 的「经验驱动改进」+ 直接缓解 token 空转成本。

### P4 SOP 流程记忆（中期）
- 新增 SOP 实体（Ent Schema：`memory_agent_sops`）：从 ≥N 次同目标高质量 case 聚合蒸馏产出，含步骤/条件/预期/关联工具，走版本化治理（与事实同一套 confidence/decay/双时序）。
- 召回：任务匹配时 SOP 整块注入；执行时与 graph runtime 节点对齐（步骤完成度可跟踪、可恢复）。
- 图执行履历反哺：执行单完成后把成功路径蒸馏回 SOP 库（闭环）。
- **收益映射**：Workflow Execution 子能力从 0 到 1。

## 5. 节奏建议

| 阶段 | 内容 | 重评收益 |
|------|------|---------|
| 第一波 | P0（基准）+ P1（钉住） | Rules 确定性执行，G 维立即可测提升 |
| 第二波 | P2（校验回路） | Context Learning conjunctive 通过率 |
| 第三波 | P3（蒸馏）+ P4（SOP） | Workflow Execution 从 0 到 1 |

P1 与 sh-04 提取缺口修复、knowledge_search 降级守卫同属 `amc-2026.08-r2` 重评前置项，建议合并排期。

## 6. 风险与注意

- **token 成本**：钉住规则块 + 校验回路会增加每轮 token；规则块建议 ≤ budget 硬顶（可配），校验回路仅对 verifiable 规则触发。
- **误判风险**：LLM-judge 抽检会有假阳性，违规结论只写 pitfall 不阻断用户（观测先行，严格阻断后置）。
- **框架禁令**：全部改动在 `internal/` 业务层（biz/data/agent 装配），不碰 vendored trpc-agent-go（FW-R1）。
- **G 维度官方口径未公开**：以上子能力拆解为行业同构推断；P0 基准建好后，若官方公布叶子指标应对齐校准。

---

## 7. 深入评审与优化（2026-08-18 复审，实施前定稿）

对第 2/4 节方案逐条做了代码级评审，**修正一处关键前提、裁剪两项范围**：

### 7.1 评审证据与结论

| 方案假设 | 评审发现（证据） | 结论 |
|---------|----------------|------|
| 规则（instruction）无钉住通道，走相似度门控 | 钉住通道**已存在**：FR-M3 `PinnedPreferenceCueWithIDs`（[composite_prompt.go:38](file:///f:/myproject/aranea-agents/internal/agent/composite_prompt.go#L38)），注入点 [memory_inject.go:339](file:///f:/myproject/aranea-agents/internal/agent/memory_inject.go#L339)，kinds=`{"preference","constraint"}`；bi-temporal 过滤（`valid_until=''`）与 user+agent 双作用域已内建于 [ListActivePreferenceFacts](file:///f:/myproject/aranea-agents/internal/data/memory_shim_l3.go#L1340) | P1 不是新建通道，而是**扩展 kinds 覆盖** |
| —（未察觉） | **双分类法并存**：consolidation 路径产出 `preference/constraint`（已钉住，生产库 116+76 行）；immediate self-marking 路径产出 `user_preference/user_identity/agent_instruction`（**未钉住**，[immediate_fact_writer.go:122](file:///f:/myproject/aranea-agents/internal/biz/immediate_fact_writer.go#L122)）；且 **`agent_instruction` 生产库 0 行**——self-marking 规则路径未投产 | 真正的缺口：钉住 kinds 未覆盖 self-marking 分类法 |
| 钉住可能挤出偏好 | 钉住查询 `ORDER BY importance DESC`；immediate 写入 importance=0.8 > consolidation 常见 0.6 → 规则类自然优先占坑 | 单池 cap=10 即可，无需拆分 |
| 评测中规则被执行 = instruction 召回成功 | 实际：规则被提取为 `preference` kind（已钉住）或来自静态 instruction | 评测观测与架构分析自洽 |

### 7.2 优化后的实施范围（本轮）

| 项 | 内容 | 改动面 |
|---|------|--------|
| **P1（优化版）** | 钉住 kinds `{"preference","constraint"}` → `{"preference","constraint","user_preference","agent_instruction"}`；prefix 映射增 `user_preference→PREFERENCE`、`agent_instruction→RULE` | `composite_prompt.go` + 单测 |
| **P2a（并入 P1）** | 钉住块头部强化合规引导（规则每次作答必须遵守、先核对再输出） | 同上（文案）+ 文档同步 memory.md/memory.design.md |
| **P0（聚焦 G1）** | 本地 G1 规则一致性基准：植入 4 类规则（格式/禁令/尾注/流程确认）→ 新会话 10 题探针 → 格式+关键词双轨判分 | `docs/testing/agent-eval-20260818/19-rules-compliance/`（主计划编号 03 已被 knowledge-rag 占用，顺延取 19） |
| ~~P2 阻塞式校验回路~~ | **裁剪**：流式输出下生成后拦截架构侵入大；改为后续异步审计方案另行设计 | — |
| ~~P3-lite case 步骤化~~ | **裁剪**：cron LLM 路径运行时验证成本高，与 P4 SOP 合并下一轮，配合 G2 基准 | — |
| ~~P4 SOP~~ | 维持中期规划不变 | — |

### 7.3 风险复核（优化后）

- token 零增量风险：cap=10×200 runes 不变，新增 kinds 当前生产行数 5+0。
- 纯增量行为：preference/constraint 既有路径不动；无框架改动（FW-R1 ✅）。
- 已知既有现象不扩大：钉住与 fused recall 对同一事实可能重复注入（preference 已如此），本轮不加 dedup（YAGNI）。

---

## 8. 实施与测试结论（2026-08-18 完成）

### 8.1 实施清单

| 项 | 文件 | 内容 |
|---|------|------|
| P1 钉住 kinds 扩展 | [composite_prompt.go](file:///f:/myproject/aranea-agents/internal/agent/composite_prompt.go) | `pinnedPreferenceKinds` 扩展为 `{preference, constraint, user_preference, agent_instruction}`；`agent_instruction` 渲染前缀 `RULE`；钉住块头部增合规引导（每次作答前逐条核对、不得因相关性低忽略） |
| P1 单测 | composite_prompt_test.go | kinds 覆盖、RULE 前缀、bi-temporal/双作用域过滤钉死 |
| **接线根治（评审外发现）** | [chat_orch_agent_build.go](file:///f:/myproject/aranea-agents/internal/service/chat_orch_agent_build.go)、[runner_team_trpc_phases.go](file:///f:/myproject/aranea-agents/internal/team/runner_team_trpc_phases.go)、[a2a_endpoint.go](file:///f:/myproject/aranea-agents/internal/service/a2a_endpoint.go) | 三处手工装配点漏接 `MemoryFactInjectCounter`/`MemoryProfileCardReader`（wire_gen 有、手工点无）→ 钉住块正常注入但 injected_count 永不落库、档案卡不进 prompt。已对齐 |
| P0 G1 基准 | [19-rules-compliance/](file:///f:/myproject/aranea-agents/docs/testing/agent-eval-20260818/19-rules-compliance)（cases.md / sample-rules-compliance.json / run.ps1） | 4 规则植入 → 跨会话 10 探针 → 格式+关键词双轨判分 + G-FACTS 落库抽查 + G-PIN 钉住验证 |
| 文档同步 | memory.md / memory.design.md | 钉住 kinds 扩展与 RULE 前缀口径 |

### 8.2 测试结果（G1 终版 run5，证据 19-rules-compliance/evidence/）

| 指标 | 结果 | 判定口径 | 结论 |
|------|------|---------|------|
| R1 表格格式 | 4/4 = 100% | 语义规则 ≥80% | PASS |
| R2 禁令词 | 10/10 = 100% | 机械规则 100% | PASS |
| R3 固定尾注 | 10/10 = 100% | 机械规则 100% | PASS |
| R4 变更确认流程 | 3/3 = 100% | 语义规则 ≥80% | PASS |
| 探针总判定 | **10/10 PASS，0 FAIL/REVIEW** | — | PASS |
| G-FACTS 规则落库 | 4/4 规则词命中 | — | PASS |
| G-PIN 钉住注入 | **4/4 规则事实 injectedCount=43** | >0 即钉住生效 | PASS |

**钉住生效的数理自证**：run4 结束时 injectedCount=29，run5 恰好 14 个 fresh turn（4 植入 + 10 探针）→ 43 = 29+14，与 once-per-turn 递增设计逐点吻合。运行日志佐证：`memory cue build timing` 报 `injected_facts=15`、`cue_chars=1191`。

### 8.3 测试过程踩坑（测量侧，已修）

1. **判分取错字段**（run2 全灭）：Raw 含思考链，模型 reasoning 中引用禁令词原文造成 R2 假阳性 → 判分只取 `agentMessage.content_markdown`。
2. **G-PIN 取首条命中**（run3 误判）：同规则词命中多条事实（重复提取+显式规则），首条可能未入钉住 top10 → 取命中集 injectedCount 最大值（钉住语义=任一承载事实被注入）。
3. **分页参数名错误**（run4 G-PIN 1/4 误判）：`ListMemoryFacts` 契约是 `limit/offset`，`page_size=200` 静默回退默认 20 条，高 importance 钉住事实被截在首页外 → 改 `limit=200`（DB 直查证实 4 条 0.8 规则事实 injected_count 全为 29）。

### 8.4 结论

P1（规则钉住通道）+ P0（G1 基准）实施完毕并全绿：规则类事实（含 self-marking `agent_instruction` 路径）100% 注入、不受召回 budget 门控，Rules 子能力从「概率执行」转为「确定执行」。G1 基准可重复运行（`run.ps1`，支持 `-Pilot`/`-SkipPlant`），作为后续 P2-P4 与重评前的回归门。

