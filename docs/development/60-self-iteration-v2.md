# Self-Iteration V2 — 从被动修复到主动进化的自我迭代闭环

本文档描述 Aranea-Agents **自我迭代 V2** 的功能需求，涵盖标准化错误表示、统一知识库、语义回归检查、预测性自愈、Skill 进化闭环、知识库动态挖掘等核心能力。目标是从"检测→修复"的线性流程进化为"修复→学习→预防"的完整闭环。

> 对应 OpenSpec Change：`openspec/changes/self-iteration-v2/`
> 设计文档：[60-self-iteration-v2.design.md](./60-self-iteration-v2.design.md)
> 开发计划：[60-self-iteration-v2.development.md](./60-self-iteration-v2.development.md)

---

## 0. 背景与动机

### 0.1 能力愿景

自我迭代 V2 旨在将 Aranea-Agents 从"被动修复"的线性流程进化为"修复→学习→预防"的完整闭环。核心能力包括：

| 能力域 | 愿景 |
|--------|------|
| 标准化错误表示 | CI 与运行时共享统一的错误描述格式，提升 LLM 分类路由精确度 |
| 统一知识库 | 运行时自愈规则与 CI 修复模式合并为单一知识库，支持跨场景学习 |
| 语义回归检查 | Auto-Fix 验证通过后，用 LLM 对比 diff 检查语义偏差 |
| 预测性自愈 | 基于历史失败模式和当前系统指标，预测即将发生的错误并提前干预 |
| Skill 进化闭环 | Solve→Observe→Evolve→Gate→Reload 五阶段 Skill 自我进化 |
| 知识库动态挖掘 | 每日从历史修复记录中自动提取修复模板，动态更新知识库 |
| Skill Intelligence | 经验报告诊断、动态推荐排序、Curator Agent 半自动进化 |

### 0.2 竞品趋势

竞品分析（Copilot Autofix / CodeMender / A-Evolve / Live-SWE-agent / EvoSkills）表明，行业趋势正从"被动修复"走向"主动进化"，关键差异化在于：**确定性兜底 + 语义回归检查 + 知识库动态进化 + 协同验证**。

### 0.3 非目标

- 不改变已有 Skill 健康/进化/管家模块的业务逻辑
- 不改变 trpc-agent-go 框架层
- 不实现全自动化进化（仅半自动，需人工审批）
- 不做 K8s 部署配置或 staging 环境
- 不做性能自动调优（仅采集指标，不做自动参数调整）
- 不修改任何 proto 文件的已有定义（仅新增）

---

## 1. FailureReport 标准化错误表示

### 1.1 需求描述

定义统一的错误描述格式，供 CI 和运行时共享。受 SWE-agent ACI 启发：为 LLM Agent 设计专用交互界面，而非复用人类格式。当前 CI 日志是原始文本，LLM 需要自行解析，效率低且不稳定；结构化表示让分类路由更精确，减少 LLM 误判。

### 1.2 验收标准

| ID | 验收标准 |
|----|----------|
| FR-01 | 标准化错误表示包含：ID(UUID)、Type(lint_error/test_failure/build_failure/proto_sync/runtime_error)、Source(ci/runtime)、Job、File、Line、ErrorCode、Message、StackTrace、RelatedCode、Metadata |
| FR-02 | CI 侧可将 Go 编译错误日志解析为标准化错误，type 为 "build_failure"，包含 file/line/error_code/message |
| FR-03 | CI 侧可将 Go 测试失败日志解析为标准化错误，type 为 "test_failure"，包含 file/line/error_code/message/stack_trace |
| FR-04 | CI 侧可将 Lint 错误日志解析为标准化错误，type 为 "lint_error"，包含 file/line/error_code/message |
| FR-05 | 运行时错误信息可转换为标准化错误，source 字段为 "runtime" |
| FR-06 | CI 侧提供脚本可将原始日志解析为标准化错误 JSON |

### 1.3 优先级

P0（阶段一）

---

## 2. 统一失败模式知识库 (failure_pattern_store)

### 2.1 需求描述

新增统一失败模式知识库，统一存储运行时自愈规则和 CI Auto-Fix 模式，合并当前互相隔离的两套知识库。统一存储后可实现跨场景学习：CI 修复模式可被运行时参考，运行时规则可被 CI 使用，并为动态挖掘提供数据基础。

### 2.2 验收标准

| ID | 验收标准 |
|----|----------|
| FPS-01 | 知识库记录包含：ID(UUID)、Source(runtime/ci/mined)、Type、PatternHash(SHA256)、PatternRegex、FixAction(JSON)、Confidence(0-1)、SuccessCount、FailCount、Version、IsActive、CreatedAt、UpdatedAt |
| FPS-02 | 索引 `(source, type)`、`(pattern_hash)`、`(is_active, confidence DESC)` 可正确创建 |
| FPS-03 | 系统可将运行时自愈规则同步到知识库，source 为 "runtime"，confidence 为 0.9 |
| FPS-04 | 系统可将 CI 修复记录聚合后同步到知识库，source 为 "ci" |
| FPS-05 | 通过 `pattern_hash` 字段可精确索引查询，不使用 `pattern_regex` 做索引 |
| FPS-06 | 每条记录维护版本号，支持回滚到历史版本 |
| FPS-07 | 动态挖掘生成的新规则 version 从 1 开始，source 为 "mined"，confidence 为 0.5 |
| FPS-08 | 管理员禁用某条 mined 规则时，将 `is_active` 设为 false，不影响其他版本 |
| FPS-09 | 新 mined 规则经过 3 次成功验证后，confidence 从 0.5 提升到 0.8 |
| FPS-10 | 当 mined 规则的 `fail_count > success_count * 2` 时，自动将 `is_active` 设为 false |

### 2.3 优先级

P0（阶段一）

---

## 3. Critic Agent 语义回归检查

### 3.1 需求描述

在 Auto-Fix 验证通过后，用 LLM 对比 diff 检查语义偏差。受 CodeMender 的 LLM 批评工具启发：用一个 LLM Agent 审查另一个 LLM Agent 的修复。当前验证只有测试和 lint，无法检测语义级回归（如修复了 A 但破坏了 B 的隐含契约）。

### 3.2 验收标准

| ID | 验收标准 |
|----|----------|
| CA-01 | Critic Agent 输出结构化评审结果，包含 `is_safe`(bool)、`risk_level`(low/medium/high)、`concerns`([]string)、`suggestion`(string) |
| CA-02 | 当 `risk_level` 为 "low" 时，直接创建 auto-fix PR |
| CA-03 | 当 `risk_level` 为 "medium" 时，创建 auto-fix PR 并添加 "needs-careful-review" 标签 |
| CA-04 | 当 `risk_level` 为 "high" 时，放弃修复，记录到知识库，不创建 PR |
| CA-05 | Critic Agent 当日调用次数达到 10 次时，跳过 Critic Agent 步骤，直接创建 PR（降级为无语义检查） |
| CA-06 | 支持通过环境变量 `ENABLE_CRITIC_AGENT=false` 跳过 Critic Agent 步骤 |

### 3.3 优先级

P1（阶段一）

---

## 4. 预测性自愈 (predictive_heal)

### 4.1 需求描述

基于历史失败模式和当前系统指标，预测即将发生的错误并提前干预，从被动响应进化到主动预防。基于失败模式知识库的趋势分析，可在错误发生前干预。

### 4.2 验收标准

| ID | 验收标准 |
|----|----------|
| PH-01 | 预测性自愈仅对置信度 > 0.8 的预测执行预防行动 |
| PH-02 | Provider 延迟持续上升且历史有 RateLimit 失败模式时，预测 RateLimit 错误，置信度 > 0.8 时提前切换 Provider |
| PH-03 | Memory 使用率 > 80% 且历史有 OOM 失败模式时，预测 OOM 错误，置信度 > 0.8 时预热 Memory 缓存或限流 |
| PH-04 | 预测置信度 <= 0.8 时，仅记录预测结果，不执行预防行动 |
| PH-05 | 同类型预防行动在 30 分钟内已执行过时，跳过本次预防行动，记录冷却期命中 |
| PH-06 | 所有预防行动记录到 HealRecord，包含 prediction_basis、confidence、action_taken、result 字段 |

### 4.3 优先级

P2（阶段三）

---

## 5. Skill 五阶段进化闭环 (skill_evolution_loop)

### 5.1 需求描述

实现 Solve→Observe→Evolve→Gate→Reload 五阶段 Skill 进化闭环，每个阶段独立可审计。受 A-Evolve 启发，建立完整的 Skill 自我进化机制。

### 5.2 验收标准

| ID | 验收标准 |
|----|----------|
| SEL-01 | Solve 阶段：使用当前 Skill 配置执行目标任务，记录执行结果 |
| SEL-02 | Observe 阶段：采集结构化日志、性能指标、Skill 调用成功率，存入经验报告 |
| SEL-03 | Evolve 阶段：调用 Curator Agent 分析观察数据，生成 Skill 草案（SKILL.md） |
| SEL-04 | Gate 阶段：执行多维验证——功能正确性（Sandbox Runner）+ 安全性（CodeQL）+ 性能（Token/耗时对比）+ 风格（araneactl lint） |
| SEL-05 | Reload 阶段：Gate 验证通过且人工审批完成后，注册新 Skill 版本，标记 parent_version_id 和 evolution_reason |
| SEL-06 | Gate 任一维度失败则拒绝进化 |
| SEL-07 | 性能退化 > 20% 则拒绝进化 |
| SEL-08 | 检查 Skill 草案不含敏感信息（API key/password/token） |
| SEL-09 | 7 天未审批的进化建议自动标记为 expired |

### 5.3 优先级

P2（阶段三）

---

## 6. 知识库动态挖掘 (pattern_mining)

### 6.1 需求描述

每日从历史修复记录中自动提取修复模板，动态更新知识库。受 Live-SWE-agent 启发：Agent 不仅修复 bug，还能改进自身的修复策略。动态挖掘让知识库持续进化，减少人工维护。

### 6.2 验收标准

| ID | 验收标准 |
|----|----------|
| PM-01 | 系统可将相同 error_code + 相似 stack_trace 的修复记录聚类为同一模式 |
| PM-02 | 同一聚类中有 >= 3 次成功修复时，提取共性 diff 模式，生成修复模板，写入知识库（source="mined"） |
| PM-03 | 同一聚类中成功修复 < 3 次时，不生成修复模板，等待更多数据 |
| PM-04 | 新挖掘规则初始 confidence=0.5，is_active=true |
| PM-05 | mined 规则经过 3 次成功验证后，confidence 提升到 0.8 |
| PM-06 | 同一 pattern_hash 的规则被重新挖掘时，创建新版本记录，version 递增 |

### 6.3 优先级

P2（阶段三）

---

## 7. Auto-Fix 引擎改造

### 7.1 需求描述

改造 CI Auto-Fix 引擎，新增结构化错误输入、Critic Agent 语义检查步骤、保护文件白名单细化，激活已有但从未运行的 Auto-Fix 闭环。

### 7.2 验收标准

| ID | 验收标准 |
|----|----------|
| AFE-01 | CI 失败后自动提取失败日志并解析为结构化错误 JSON（而非原始文本），按 type 路由到对应修复策略 |
| AFE-02 | 错误 type 为 "lint_error" 时，使用 araneactl lint --fix + eslint --fix + stylelint --fix 进行规则修复 |
| AFE-03 | 错误 type 为 "test_failure" 或 "build_failure" 时，优先使用自托管 Agent 诊断，未配置时回退到 pattern-fix |
| AFE-04 | Auto-Fix 生成的 patch 通过 go vet + pnpm build 验证后，调用 Critic Agent 评估修复的语义安全性 |
| AFE-05 | auto-fix 修复的文件位于 `internal/biz/monitor/` 目录时，允许修复，不触发保护文件拒绝 |
| AFE-06 | auto-fix 尝试修改 `.github/workflows/`、`Makefile`、`go.mod/sum`、proto 文件时，拒绝修复，记录保护文件命中 |

### 7.3 优先级

P1（阶段一）

---

## 8. Skill Intelligence Phase 2-5 落地

### 8.1 需求描述

完成 Skill Intelligence Phase 2-5 核心功能：经验报告自动分析与展示、动态推荐排序、Curator Agent 半自动进化。

### 8.2 验收标准

| ID | 验收标准 |
|----|----------|
| SI-01 | 系统每 15 分钟扫描未分析的 Skill 调用记录，批量执行分析、评分、生成经验报告 |
| SI-02 | 一条 Skill 调用记录分析完成后，更新其分析时间字段为当前时间 |
| SI-03 | 经验报告生成时调用根因分析，将根因分析结果写入经验报告的根因分析字段 |
| SI-04 | 根因分析返回修复动作时，将修复动作转换为人类可读的修复建议，写入经验报告的修复建议字段 |
| SI-05 | 提供经验报告列表和详情 API，按 Skill 查询经验报告，按 `created_at DESC` 排序 |
| SI-06 | 动态推荐排序从健康指标提供者读取近期指标动态调整排序权重 |
| SI-07 | Skill 7d 成功率 > 80% 时，降低其探索奖励，使其在排序中更稳定 |
| SI-08 | Skill 7d 成功率 < 40% 时，提升其探索奖励或降低历史成功权重 |
| SI-09 | Skill 无近期调用数据时，使用静态默认排序因子 |
| SI-10 | Skill 被选中执行后，记录排序反馈（skill_id, rank_score, actual_success, timestamp） |
| SI-11 | Skill 7d 成功率 < 60% 或同一失败标签出现 >= 5 次时，创建进化建议并触发 Curator Agent |
| SI-12 | Curator Agent 通过 ChatOrchestrator 调用自身 Agent，输入失败模式+历史调用记录+现有 Skill 列表，输出 Skill 草案（SKILL.md） |
| SI-13 | Curator Agent 生成的 Skill 草案在 Sandbox Runner 中隔离执行验证 |
| SI-14 | Curator Agent 每日调用上限 20 次 |
| SI-15 | 进化建议 7 天未审批自动过期 |
| SI-16 | 经验报告列表页提供 KPI 概览（报告总数/成功率/平均评分/失败记录，后端按筛选条件全量聚合透出 success_count/failure_count/avg_score）；失败标签以水平条形图展示且标签中文化（unknown→「未分类」弱化）；含根因分析/建议修复/优化建议的报告支持表格行内展开查看 |

### 8.3 优先级

P1（阶段二）

---

## 9. 根因分析能力复用

### 9.1 需求描述

将根因分析能力抽取为可复用模块，供 Skill Intelligence 和预测性自愈共同使用。当前根因分析能力与具体实现耦合，无法被其他模块复用；抽取为可复用能力符合依赖倒置原则。

### 9.2 验收标准

| ID | 验收标准 |
|----|----------|
| RCA-01 | 根因分析能力提供 `Analyze(ctx, stepID, phase, err, metadata)` 和 `AnalyzeFromReport(ctx, report)` 两种调用方式 |
| RCA-02 | 现有根因分析引擎实现该可复用能力接口 |
| RCA-03 | 依赖注入装配时，将现有引擎作为可复用能力接口的实现注入 |
| RCA-04 | Skill Intelligence 通过注入的可复用能力接口调用 `AnalyzeFromReport`，不直接依赖具体引擎类型 |
| RCA-05 | 预测性自愈通过注入的可复用能力接口调用 `Analyze`，不直接依赖具体引擎类型 |

### 9.3 优先级

P0（阶段一）

---

## 10. 动态推荐排序权重调整

### 10.1 需求描述

引入动态推荐排序，从 Skill 健康聚合器读取近期指标动态调整排序权重，替代静态排序因子。动态排序需通过接口桥接避免分层违规（Tools 层不直接依赖 Biz 层）。

### 10.2 验收标准

| ID | 验收标准 |
|----|----------|
| DRF-01 | 健康指标提供者接口包含 `GetRecentSuccessRate(ctx, skillID, days)` 和 `GetRecentAvgDuration(ctx, skillID, days)` 方法 |
| DRF-02 | Skill 健康聚合器适配为健康指标提供者接口的实现 |
| DRF-03 | 动态推荐排序通过健康指标提供者接口读取指标，不直接依赖 Biz 层 |
| DRF-04 | 依赖注入装配时，将 Biz 层适配器作为健康指标提供者注入到 Tools 层 |

### 10.3 优先级

P1（阶段二）

---

## 11. 三阶段实施路线图

### 阶段一：闭环加固

**目标**：激活已有但断裂的自愈闭环，统一知识库，补齐集成测试

| 需求 | 优先级 |
|------|--------|
| 根因分析能力复用 | P0 |
| FailureReport 标准化错误表示 | P0 |
| 统一失败模式知识库 | P0 |
| Auto-Fix 引擎改造 | P1 |
| 集成测试补齐 | P1 |

**阶段验收**：后端全量验证（api/wire/build/test/lint）全绿

### 阶段二：Skill Intelligence 落地

**目标**：完成 Skill Intelligence Phase 2-5 核心价值功能

| 需求 | 优先级 |
|------|--------|
| 经验报告诊断 | P1 |
| 推荐排序进化 | P1 |
| Curator Agent 半自动进化 | P1 |
| 前端展示 | P1 |

**阶段验收**：后端全量验证通过 + 前端 `pnpm lint && pnpm test && pnpm build`

### 阶段三：自我进化闭环

**目标**：建立"修复→学习→预防"的完整进化闭环

| 需求 | 优先级 |
|------|--------|
| 预测性自愈 | P2 |
| Skill 五阶段进化闭环 | P2 |
| 知识库动态挖掘 | P2 |

**阶段验收**：全量验证通过 + 进化闭环端到端可运行

---

## 12. 非功能需求

- **可回滚性**：每个阶段独立，可单独回滚；所有变更为增量（新增表/接口/Cron Job），不修改已有业务逻辑
- **安全性**：Curator Agent 仅修改 Skill 的 SKILL.md（提示词），不修改代码文件；进化建议需人工审批
- **成本控制**：Critic Agent 每日上限 10 次；Curator Agent 每日上限 20 次
- **可审计性**：所有预防行动记录到 HealRecord；进化建议有完整生命周期状态追踪
- **测试隔离**：集成测试使用 build tag `integration` 隔离

---

*文档版本：2026-06-17 — 按三件套内容边界重组，聚焦用户视角的能力需求与验收标准。*
