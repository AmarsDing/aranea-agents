# Self-Iteration V2 — 从被动修复到主动进化的自我迭代闭环

本文档描述 Aranea-Agents **自我迭代 V2** 的功能需求，涵盖标准化错误表示、统一知识库、语义回归检查、预测性自愈、Skill 进化闭环、知识库动态挖掘等核心能力。目标是从"检测→修复"的线性流程进化为"修复→学习→预防"的完整闭环。

> 对应 OpenSpec Change：`openspec/changes/self-iteration-v2/`
> 设计文档：[60-self-iteration-v2.design.md](./60-self-iteration-v2.design.md)
> 开发计划：[60-self-iteration-v2.development.md](./60-self-iteration-v2-development.md)
> **当前进度**：Phase 1–3 ✅ 已落地

---

## 0. 背景与动机

### 0.1 当前状态

| 能力 | 现状 | 问题 |
|------|------|------|
| 运行时自愈 | `SelfHealObserver` + `RootCauseEngine`（12 条内置规则）+ 滑动窗口断路器 | ✅ `RootCauseAnalyzer` 接口已抽取，可被其他模块复用 |
| CI Auto-Fix | `auto-fix.yml` 完整流水线（日志提取→分类→修复→验证→PR） | ✅ 结构化 FailureReport 输入 + Critic Agent + 白名单已实现 |
| Skill Intelligence | `SkillIntelligenceUsecase`（AnalyzeInvocation/ScoreSkill/GenerateReport）已实现 | ✅ Cron Worker 已实现；经验报告 API + Curator Agent + 进化建议已落地 |
| 知识库 | 运行时 `RootCauseEngine` 规则与 CI `.auto-fix/patterns.jsonl` 互相独立 | ✅ `failure_pattern` 表统一存储；Cron Job 同步运行时规则和 CI 模式 |
| 集成测试 | 仅 3 个文件覆盖 Agent CRUD/Chat API/Channel Turn Preview | ✅ 自愈闭环 + Skill Intelligence + Chat Turn 集成测试已补齐 |
| 预测性自愈 | — | ✅ `PredictiveHealUsecase` + Cron Job 已实现 |
| Skill 进化闭环 | — | ✅ 五阶段闭环（Solve→Observe→Evolve→Gate→Reload）已实现 |
| 知识库动态挖掘 | — | ✅ `PatternMiningUsecase` + Cron Job 已实现 |

### 0.2 竞品趋势

竞品分析（Copilot Autofix / CodeMender / A-Evolve / Live-SWE-agent / EvoSkills）表明，行业趋势正从"被动修复"走向"主动进化"，关键差异化在于：**确定性兜底 + 语义回归检查 + 知识库动态进化 + 协同验证**。

### 0.3 非目标

- 不改变已有 `skill_health` / `skill_evolution` / `skills_butler` 的业务逻辑
- 不改变 trpc-agent-go 框架层
- 不实现全自动化进化（仅半自动，需人工审批）
- 不做 K8s 部署配置或 staging 环境
- 不做性能自动调优（仅采集指标，不做自动参数调整）
- 不修改任何 proto 文件的已有定义（仅新增）

---

## 1. FailureReport 标准化错误表示

### 1.1 需求描述

定义 `FailureReport` 结构体，统一 CI 和运行时的错误描述格式。受 SWE-agent ACI 启发：为 LLM Agent 设计专用交互界面，而非复用人类格式。当前 CI 日志是原始文本，LLM 需要自行解析，效率低且不稳定；结构化表示让分类路由更精确，减少 LLM 误判。

### 1.2 验收标准

| ID | 验收标准 |
|----|----------|
| FR-01 | `FailureReport` 结构体包含：ID(UUID)、Type(lint_error/test_failure/build_failure/proto_sync/runtime_error)、Source(ci/runtime)、Job、File、Line、ErrorCode、Message、StackTrace、RelatedCode、Metadata |
| FR-02 | `ParseCILogs` 函数可将 Go 编译错误日志解析为 `FailureReport`，type 为 "build_failure"，包含 file/line/error_code/message |
| FR-03 | `ParseCILogs` 函数可将 Go 测试失败日志解析为 `FailureReport`，type 为 "test_failure"，包含 file/line/error_code/message/stack_trace |
| FR-04 | `ParseCILogs` 函数可将 Lint 错误日志解析为 `FailureReport`，type 为 "lint_error"，包含 file/line/error_code/message |
| FR-05 | `ParseRuntimeError` 函数可将运行时错误信息转换为 `FailureReport`，source 字段为 "runtime" |
| FR-06 | CI 侧 Python 脚本 `parse-logs.py` 可将原始日志解析为 FailureReport JSON |

### 1.3 优先级

P0（阶段一）

---

## 2. 统一失败模式知识库 (failure_pattern_store)

### 2.1 需求描述

新增 `failure_pattern` 表（SQLite/Ent），统一存储运行时自愈规则和 CI Auto-Fix 模式，合并当前互相隔离的两套知识库。统一存储后可实现跨场景学习：CI 修复模式可被运行时参考，运行时规则可被 CI 使用，并为阶段三的动态挖掘提供数据基础。

### 2.2 验收标准

| ID | 验收标准 |
|----|----------|
| FPS-01 | `failure_pattern` 表包含：ID(UUID)、Source(runtime/ci/mined)、Type、PatternHash(SHA256)、PatternRegex、FixAction(JSON)、Confidence(0-1)、SuccessCount、FailCount、Version、IsActive、CreatedAt、UpdatedAt |
| FPS-02 | 索引 `(source, type)`、`(pattern_hash)`、`(is_active, confidence DESC)` 可正确创建 |
| FPS-03 | `failure_pattern_sync` Cron Job 可将 RootCauseEngine 的 12 条内置规则同步到 failure_pattern 表，source 为 "runtime"，confidence 为 0.9 |
| FPS-04 | `failure_pattern_sync` Cron Job 可将 `.auto-fix/patterns.jsonl` 中的修复记录聚合后同步到 failure_pattern 表，source 为 "ci" |
| FPS-05 | 通过 `pattern_hash` 字段可精确索引查询，不使用 `pattern_regex` 做索引 |
| FPS-06 | 每条 failure_pattern 记录维护版本号，支持回滚到历史版本 |
| FPS-07 | 动态挖掘生成的新规则 version 从 1 开始，source 为 "mined"，confidence 为 0.5 |
| FPS-08 | 管理员禁用某条 mined 规则时，将 `is_active` 设为 false，不影响其他版本 |
| FPS-09 | 新 mined 规则经过 3 次成功验证后，confidence 从 0.5 提升到 0.8 |
| FPS-10 | 当 mined 规则的 `fail_count > success_count * 2` 时，自动将 `is_active` 设为 false |

### 2.3 优先级

P0（阶段一）

---

## 3. Critic Agent 语义回归检查

### 3.1 需求描述

在 Auto-Fix 验证通过后，用 LLM 对比 diff 检查语义偏差。受 CodeMender 的 LLM 批评工具启发：用一个 LLM Agent 审查另一个 LLM Agent 的修复。当前验证只有 `make test && make lint`，无法检测语义级回归（如修复了 A 但破坏了 B 的隐含契约）。

### 3.2 验收标准

| ID | 验收标准 |
|----|----------|
| CA-01 | Critic Agent 输出结构化 `CriticResult`，包含 `is_safe`(bool)、`risk_level`(low/medium/high)、`concerns`([]string)、`suggestion`(string) |
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

基于历史失败模式和当前系统指标，预测即将发生的错误并提前干预，从被动响应进化到主动预防。基于 `FailurePattern` 知识库的趋势分析，可在错误发生前干预。

### 4.2 验收标准

| ID | 验收标准 |
|----|----------|
| PH-01 | `PredictiveHealUsecase` 仅对置信度 > 0.8 的预测执行预防行动 |
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
| PM-01 | `PatternMiningWorker` 可将相同 error_code + 相似 stack_trace 的修复记录聚类为同一模式 |
| PM-02 | 同一聚类中有 >= 3 次成功修复时，提取共性 diff 模式，生成修复模板，写入 failure_pattern 表（source="mined"） |
| PM-03 | 同一聚类中成功修复 < 3 次时，不生成修复模板，等待更多数据 |
| PM-04 | 新挖掘规则初始 confidence=0.5，is_active=true |
| PM-05 | mined 规则经过 3 次成功验证后，confidence 提升到 0.8 |
| PM-06 | 同一 pattern_hash 的规则被重新挖掘时，创建新版本记录，version 递增 |

### 6.3 优先级

P2（阶段三）

---

## 7. Auto-Fix 引擎改造

### 7.1 需求描述

改造 CI Auto-Fix 引擎，新增 FailureReport 结构化输入、Critic Agent 语义检查步骤、保护文件白名单细化，激活已有但从未运行的 Auto-Fix 闭环。

### 7.2 验收标准

| ID | 验收标准 |
|----|----------|
| AFE-01 | CI 失败后自动提取失败日志并解析为结构化 FailureReport JSON（而非原始文本），按 type 路由到对应修复策略 |
| AFE-02 | FailureReport.type 为 "lint_error" 时，使用 araneactl lint --fix + eslint --fix + stylelint --fix 进行规则修复 |
| AFE-03 | FailureReport.type 为 "test_failure" 或 "build_failure" 时，优先使用自托管 Agent 诊断，未配置时回退到 pattern-fix.sh |
| AFE-04 | Auto-Fix 生成的 patch 通过 go vet + pnpm build 验证后，调用 Critic Agent 评估修复的语义安全性 |
| AFE-05 | auto-fix 修复的文件位于 `internal/biz/monitor/` 目录时，允许修复，不触发保护文件拒绝 |
| AFE-06 | auto-fix 尝试修改 `.github/workflows/`、`Makefile`、`go.mod/sum`、proto 文件时，拒绝修复，记录保护文件命中 |

### 7.3 优先级

P1（阶段一）

---

## 8. Skill Intelligence Phase 2-5 落地

### 8.1 需求描述

完成 Skill Intelligence Phase 2-5 核心功能：经验报告 Cron Worker/API/前端展示、动态推荐排序、Curator Agent 半自动进化。

### 8.2 验收标准

| ID | 验收标准 |
|----|----------|
| SI-01 | `skill_intelligence_worker` Cron Job 每 10 分钟扫描未分析的 `skill_invocation`（`analyzed_at IS NULL`），批量执行 AnalyzeInvocation/ScoreSkill/GenerateReport |
| SI-02 | 一条 `skill_invocation` 分析完成后，更新其 `analyzed_at` 字段为当前时间 |
| SI-03 | `GenerateReport` 调用 `RootCauseAnalyzer.AnalyzeFromReport`，将根因分析结果写入 `ExperienceReport.RootCauseAnalysis` |
| SI-04 | 根因分析返回 FixAction 时，将 FixAction 转换为人类可读的修复建议，写入 `ExperienceReport.SuggestedFix` |
| SI-05 | 提供 `ListExperienceReports` 和 `GetExperienceReport` API，按 Skill 查询经验报告，按 `created_at DESC` 排序 |
| SI-06 | `DynamicRankFactors` 从 `HealthMetricsProvider` 读取近期指标动态调整排序权重 |
| SI-07 | Skill 7d 成功率 > 80% 时，降低其 ExplorationBonus，使其在排序中更稳定 |
| SI-08 | Skill 7d 成功率 < 40% 时，提升其 ExplorationBonus 或降低 HistoricalSuccess 权重 |
| SI-09 | Skill 无近期调用数据时，使用静态默认 RankFactors |
| SI-10 | Skill 被选中执行后，记录 `RankFeedback{skill_id, rank_score, actual_success, timestamp}` |
| SI-11 | Skill 7d 成功率 < 60% 或同一失败标签出现 >= 5 次时，创建 `SkillEvolutionSuggestion` 并触发 Curator Agent |
| SI-12 | Curator Agent 通过 `ChatOrchestrator` 调用自身 Agent，输入失败模式+历史调用记录+现有 Skill 列表，输出 Skill 草案（SKILL.md） |
| SI-13 | Curator Agent 生成的 Skill 草案在 Sandbox Runner（codeexecutor.CodeExecutor/E2B）中隔离执行验证 |
| SI-14 | Curator Agent 每日调用上限 20 次 |
| SI-15 | 进化建议 7 天未审批自动过期 |

### 8.3 优先级

P1（阶段二）

---

## 9. RootCauseAnalyzer 接口抽取

### 9.1 需求描述

从 `RootCauseEngine` 抽取 `RootCauseAnalyzer` 接口，供 `SkillIntelligenceUsecase` 和 `PredictiveHealUsecase` 复用。当前 `RootCauseEngine` 是具体结构体，无法通过 Wire 注入到其他包；接口抽取符合依赖倒置原则。

### 9.2 验收标准

| ID | 验收标准 |
|----|----------|
| RCA-01 | `RootCauseAnalyzer` 接口包含 `Analyze(ctx, stepID, phase, err, metadata)` 和 `AnalyzeFromReport(ctx, report)` 方法 |
| RCA-02 | `RootCauseEngine` 实现 `RootCauseAnalyzer` 接口 |
| RCA-03 | Wire DI 装配时，将 `RootCauseEngine` 作为 `RootCauseAnalyzer` 接口的实现注入 |
| RCA-04 | `SkillIntelligenceUsecase` 通过注入的 `RootCauseAnalyzer` 接口调用 `AnalyzeFromReport`，不直接依赖 `RootCauseEngine` 具体类型 |
| RCA-05 | `PredictiveHealUsecase` 通过注入的 `RootCauseAnalyzer` 接口调用 `Analyze`，不直接依赖 `RootCauseEngine` 具体类型 |

### 9.3 优先级

P0（阶段一）

---

## 10. DynamicRankFactors 动态权重调整

### 10.1 需求描述

在 Tools 层定义 `HealthMetricsProvider` 接口，由 Biz 层实现并注入，避免 Tools 直接依赖 Biz。引入 `DynamicRankFactors`，从 SkillHealthAggregator 读取近期指标动态调整排序权重，替代静态 RankFactors。

### 10.2 验收标准

| ID | 验收标准 |
|----|----------|
| DRF-01 | `HealthMetricsProvider` 接口包含 `GetRecentSuccessRate(ctx, skillID, days)` 和 `GetRecentAvgDuration(ctx, skillID, days)` 方法 |
| DRF-02 | Biz 层 `SkillHealthAggregator` 适配为 `HealthMetricsProvider` 接口的实现 |
| DRF-03 | `DynamicRankFactors` 通过 `HealthMetricsProvider` 接口读取指标，不直接依赖 Biz 层 |
| DRF-04 | Wire DI 装配时，将 Biz 层适配器作为 `HealthMetricsProvider` 注入到 Tools 层 |

### 10.3 优先级

P1（阶段二）

---

## 11. 三阶段实施路线图

### 阶段一：闭环加固

**目标**：激活已有但断裂的自愈闭环，统一知识库，补齐集成测试

| 需求 | 优先级 | 关键交付 |
|------|--------|----------|
| RootCauseAnalyzer 接口抽取 | P0 | 接口定义 + Wire 绑定 |
| FailureReport 标准化错误表示 | P0 | 结构体 + 解析器 + CI Python 脚本 |
| 统一失败模式知识库 | P0 | failure_pattern 表 + Cron 同步 |
| Auto-Fix 引擎改造 | P1 | 结构化日志解析 + Critic Agent + 白名单 |
| 集成测试补齐 | P1 | 自愈闭环 + Skill Intelligence + Chat Turn |

**阶段验收**：`make api && make wire && make build && make test && make lint` 全绿

### 阶段二：Skill Intelligence 落地

**目标**：完成 Skill Intelligence Phase 2-5 核心价值功能

| 需求 | 优先级 | 关键交付 |
|------|--------|----------|
| 经验报告诊断 | P1 | RootCauseAnalysis + Cron Worker + API + 前端 |
| 推荐排序进化 | P1 | DynamicRankFactors + HealthMetricsProvider |
| Curator Agent 半自动进化 | P1 | 触发判定 + ChatOrchestrator + Sandbox Runner |
| 前端展示 | P1 | 经验报告列表页 + 进化审批 UI |

**阶段验收**：全量验证通过 + 前端 `pnpm lint && pnpm test && pnpm build`

### 阶段三：自我进化闭环

**目标**：建立"修复→学习→预防"的完整进化闭环

| 需求 | 优先级 | 关键交付 |
|------|--------|----------|
| 预测性自愈 | P2 | PredictiveHealUsecase + Cron Job |
| Skill 五阶段进化闭环 | P2 | Solve→Observe→Evolve→Gate→Reload |
| 知识库动态挖掘 | P2 | PatternMiningWorker + 审计机制 |

**阶段验收**：全量验证通过 + 进化闭环端到端可运行

---

## 12. 验收要点

- [x] FailureReport 结构体定义完整，CI/运行时解析器可正确解析各类错误
- [x] failure_pattern 表创建成功，Cron Job 可同步运行时规则和 CI 模式
- [x] Critic Agent 可在 Auto-Fix 验证通过后执行语义回归检查，按 risk_level 分级处理
- [x] Auto-Fix 引擎支持结构化 FailureReport 输入，白名单机制生效
- [x] RootCauseAnalyzer 接口抽取完成，Wire 绑定正确
- [x] skill_intelligence_worker Cron Job 可自动触发分析
- [x] 经验报告 API 可查询，包含根因分析和修复建议
- [x] DynamicRankFactors 可根据健康指标动态调整排序权重
- [x] Curator Agent 可通过 ChatOrchestrator 生成 Skill 草案
- [x] 预测性自愈仅对高置信度预测执行预防行动，冷却期生效
- [x] Skill 五阶段进化闭环可端到端运行，Gate 多维验证生效
- [x] 知识库动态挖掘可从历史修复记录提取修复模板
- [x] 集成测试覆盖自愈闭环、Skill Intelligence、Chat Turn 核心业务流程

---

*文档版本：2026-06-06 — Phase 1–3 已落地，验收要点全部勾选。*
