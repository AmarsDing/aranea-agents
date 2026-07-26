# 评审报告：递归自我改进（RSI）方案 vs Aranea-Agents 实现差距分析

> 类型：评审报告
> 关联文档：[2026-07-25-research-harness-recursive-self-improvement.md](./2026-07-25-research-harness-recursive-self-improvement.md)（调研报告：Harness 递归自我改进）
> 评审范围：`internal/biz/` 进化相关实现（learning_loop / skill_evolution_loop / skill_evolution_unified / skill_evolution_triggers / evolution）
> 评审日期：2026-07-25

---

## 1. 评审结论（一句话）

**项目的短板不在架构也不在调度，而在「反思者」与「应用闭环」**——调度层（LearningLoopScanner/CuratorWorker）与治理底座（Orchestrator/Gate/回滚/状态机）均已通电，但全链路无 LLM 参与改进内容生成（两条路径都是模板字符串），且 skill 进化在审批后断于「注册新版本」之前。按 P0→P2 路径，可用最小代价从「ACE 半成品」升到「MCE 双层优化」，且全程复用现有统一框架。

## 2. 关键代码事实（评审依据）

| # | 事实 | 证据 | 含义 |
|---|------|------|------|
| F1 | 学习闭环生产路径是 `LearningLoopUsecase.RunLoop`：观测 → 模式聚类 → 建议 | `internal/biz/learning_loop.go:390` | 闭环主干存在 |
| F2 | **后台调度已接线**：`LearningLoopScanner`（60min ticker → `RunLoopAll`）、`CuratorWorker`（2h ticker → 每 skill `RunCuratorFlow`，日上限 20 次）均经 wire 注入 app 启动序列；另有 `EvolutionScanner`/`SkillEvolutionScanner`/`SkillIntelligenceWorker`/`PatternMiningJob` | `internal/cronrunner/jobs/learning_loop.go`、`internal/cronrunner/jobs/skill_curator_worker.go`、`cmd/admin/wire_gen.go:544-578` | 调度层不是缺口（~~初版评审误判，已更正~~） |
| F3 | **反思者缺位且贯穿两条路径**：LearningLoop 建议内容 `proposalContent` 是模板字符串；skill 路径 `generateRuleBasedDraft` 输出固定模板（注释自承 "rule-based (v1)"）；**审批后异步 `GenerateDraftForSuggestion` 仍走同一 rule-based 函数**——全链路无 LLM 参与 | `internal/biz/learning_loop.go:509`、`internal/biz/skill_intelligence.go:557+646` | 「发现」是程序做的，「改进方案」没有反思者（两条路径都是） |
| F4 | 五阶段循环 Solve→Observe→Evolve→Gate→Reload 完整定义，含四维 Gate（功能/安全/性能 20% 退化阈值/风格）、过期机制、状态机 | `internal/biz/skill_evolution_loop.go:185` | 设计完整（参考实现） |
| F5 | `SkillEvolutionLoop.Run` 无生产调用方（仅测试引用）；**但生产上有功能等价的半自动管线** `RunCuratorFlow`：触发检测 → rule-based draft → validating → GateVerifier/rule-based sandbox → ready。**缺 Solve（真实任务执行）与 Reload（注册新版本）** | `internal/biz/skill_intelligence.go:401`；全局搜索无 wire/service/job 调用 `.Run(` | 五阶段中的 Gate 已通电，Solve/Reload 未通电 |
| F6 | Agent 级自动触发器是空占位：`AgentConfigTrigger.Check` 直接 `return nil, nil` | `internal/biz/skill_evolution_triggers.go:278` | DGM-H 路径未启动 |
| F7 | 应用动作 = **整体替换** IDENTITY.md / AGENTS*.md 的 Body（有快照回滚、事务保护） | `internal/biz/evolution.go:204` | ACE 式 context 层改进，但无 delta 更新协议 |
| F8 | 统一治理底座成熟：Orchestrator（触发器注册/冷却期/DB 唯一约束/状态机/过期）+ UnifiedEvolutionSuggestion | `internal/biz/skill_evolution_unified.go:142` | 全项目最值钱的资产 |

## 3. 现状定级（按 RSI 方案框架）

**结论：ACE 级弱自我改进的 ~60%，且是「半自动、人工在环」形态。**

### 3.1 已达成的部分

- ✅ **改进对象定位正确**：改的是 context 层（prompt/persona 文件、skill 版本），与 ACE 路线一致——方案七条路线中风险最低、最适合 chat 产品的切入点
- ✅ **调度层完整**：学习闭环 + Curator 半自动管线均有定时驱动（F2），且 Curator 有日上限熔断
- ✅ **治理底座超前**：冷却期、并发去重（DB UNIQUE）、状态机（AS-FSM-01）、过期机制、快照回滚、事务保护——方案中「受控改进」瓶颈已有工程答案
- ✅ **Gate 多维验证已通电**：`RunCuratorFlow` 在 `uc.gate != nil` 时走 GateVerifier 四维验证（功能/安全/性能/风格），对应方案「可靠评价器是核心瓶颈」的判断
- ✅ **审批后异步链路**：审批 → GenerateDraft + ValidateSuggestion 异步执行（B-14 修复，`context.WithoutCancel` 保证后台存活）

### 3.2 缺失的部分

- ❌ **反思者缺位（核心缺口，贯穿两条路径）**：LearningLoop 建议内容与 skill draft 均为模板字符串，审批后再生成仍是模板（F3）——ACE 的 Reflector（归因分析）与 Curator（LLM 改写）均不存在
- ❌ **Reload 未接线**：skill 审批通过后仅改状态为 ready，无「注册新版本」的生产路径（`SkillReloader` port 无生产实现）；进化闭环在「应用」处断开
- ❌ **Solve 未接线**：无真实任务执行验证（evaluation 数据集已有，未回放进进化 Gate）
- ❌ **整体替换而非 delta 更新**：`files[i].Body = s.Content` 全量覆写（F7），正是方案警告的 **Context Collapse** 风险点（丢失触发条件、失败模式等局部信息）
- ❌ **无规则级归因**：没有 helpful/harmful 计数，无法回答「上次改进有没有用」
- ❌ **观测是聚合指标不是原始 trace**：方案强调「侦探必须现场调查」；当前 `EvolutionObservationReport` 只有成功率/时长/token 聚合值，Curator 无法做因果归因
- ❌ **无递归**：改进机制本身（触发阈值、Gate 标准、模式挖掘参数）全部硬编码——`EvoTriggerScoreThreshold = 60` 这类常量即是「改进者不可被改进」的证据

## 4. 优化后的能力阶梯

| 阶段 | 能力 | 对应方案路线 | 投入性质 |
|------|------|-------------|---------|
| 现状 | 半自动 ACE（定时触发 + 模板建议 + 人工审批 + 整体替换；Gate 已通电，应用处断开） | ACE 60% | — |
| P0 补反思者+闭环 | **LLM Curator**（draft 由 LLM 基于观测/轨迹生成）+ **Reload 接线**（审批通过注册新版本）→ 完整 ACE | ACE 100% | 两个缺口，均复用现有 port |
| P1 补协议 | delta 更新协议 + 规则计数归因 + trace 级观测 + Solve（evaluation 数据集回放进 Gate）→ **可控、可归因、可回滚的 context 进化** | ACE 强化版 | 协议设计 |
| P2 升层 | Curator/Evolver 自身成为进化目标（`EvolutionTargetType: "evolver"`）+ Agent 级自动触发（填充 `AgentConfigTrigger.Check`） | **MCE** | 复用现有统一框架 |
| P3 远期 | 候选 harness 版本库 + 沙箱 A/B + 优化器参数自调 | Meta-Harness / STOP | 需 ADR，当前不建议 |

**P2 是关键洞察**：UnifiedEvolutionSuggestion 框架已是 target-type 可扩展的（skill/agent 只是枚举值），加入 `evolver` 后，MCE 的「双层优化」在架构上零成本落地——这是现有设计被低估的地方。

## 5. 优化方向与程序设计

### P0（补反思者 + 闭环，纯接线）

1. **LLM Curator**：新增 `SkillEvolver` 生产实现（LLM 调用，注入 skill 当前 body + 触发原因 + 失败标签 + 聚合观测），替换 `generateRuleBasedDraft` 的两个调用点（`RunCuratorFlow` step 2、`GenerateDraftForSuggestion`）。LLM 不可用/超时/产出非法时回退 rule-based（best-effort 语义，与项目既有降级风格一致）
2. **Reload 接线**：新增 `SkillReloader` 生产实现（注册 skill 新版本，写 `parent_version_id` + `evolution_reason`），在审批通过且 lifecycle=ready 后触发；沿用 `EvolutionUsecase` 快照回滚模式（先存旧版本快照再切换）

### P1（协议，设计重点）

3. **Delta 更新协议**：prompt 文件改为规则块结构（每规则带固定 ID），Curator 只输出 `{op: add|modify|merge|remove, rule_id, content}` 序列，由程序执行局部更新——直接消除 Context Collapse
4. **计数归因**：规则块带 helpful/harmful 计数，观测落库时按声明的规则 ID 归账；Gate 增加「历史改进有效性」维度
5. **trace 级观测**：Observation 增加原始执行轨迹引用（activities 表已有，关联即可），Curator prompt 注入轨迹片段而非聚合指标
6. **Solve 接线**：evaluation 数据集回放作为 Gate 的功能验证维度（替代 rule-based fallback）

### P2（升层，架构零成本）

6. `EvolutionTargetType` 增加 `"evolver"`，Curator 的 prompt 本身进入进化循环——用同一套 Orchestrator/Gate/审批/回滚治理

### 业务导向判断

项目是 chat 产品而非 checkpoint 恢复系统（AS-EVT-01 已有同类权衡），**人工审批在环是正确终态而非过渡态**。优化目标不是「去掉人」，而是「让人审批时看到可归因、可回滚、有验证证据的高质量建议」。Meta-Harness（代码层自改）与 STOP 在当前业务阶段投入产出比极低，建议仅以 ADR 记录为远期方向。

## 6. 风险与注意事项

| 风险 | 说明 | 缓解 |
|------|------|------|
| LLM 生成内容质量不可控 | Curator LLM 生成 draft 可能引入噪声或语义漂移 | Gate 四维验证兜底 + 人工审批终态不变 + LLM 失败回退 rule-based |
| LLM 调用成本 | CuratorWorker 日上限 20 次 flow，每次新增 1 次 LLM 调用 | 日上限已存在（`CURATOR_DAILY_MAX`），成本有硬顶 |
| Reload 自动注册的风险 | 新版本直接生效可能引入回归 | 沿用快照回滚模式 + 新版本注册后保留旧版本可切换 |
| delta 协议迁移成本 | 存量 prompt 文件非规则块结构 | 首版协议兼容整体替换（无规则 ID 时降级为全量，标记 Warn） |

---

> 下一步：如需实施，建议按 `aranea-coding-guide` 流程先就 P0 出设计方案（`.design.md` 增量），再 TDD 实施。
