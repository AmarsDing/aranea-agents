## Why

Aranea-Agents 已具备运行时自愈（SelfHealObserver + RootCauseEngine 12 条规则）和 CI Auto-Fix（GitHub Actions + 自托管 Agent 诊断）的双轨自愈能力，以及 Skill Intelligence Phase 1 的基础框架（Usecase/Repo/Data/Service），但存在三个核心缺陷：

1. **闭环未激活**：CI Auto-Fix 的 `stats.json` 全为 0，从未实际运行；两套知识库（运行时 RootCauseEngine 规则 vs CI `.auto-fix/patterns.jsonl`）互相隔离
2. **Skill 智能未落地**：Phase 2-5 的 61 步未实现，经验报告诊断、推荐排序优化、半自动进化等核心价值功能缺失
3. **缺少进化闭环**：当前是"检测→修复"的线性流程，缺少"修复→学习→预防"的反馈闭环，无法从每次修复中持续优化

竞品分析（Copilot Autofix/CodeMender/A-Evolve/Live-SWE-agent/EvoSkills）表明，行业趋势正从"被动修复"走向"主动进化"，关键差异化在于：确定性兜底 + 语义回归检查 + 知识库动态进化 + 协同验证。

## What Changes

### 阶段一：闭环加固

- 新增 `FailureReport` 标准化错误表示，改造 CI Auto-Fix 日志解析为结构化输入（受 SWE-agent ACI 启发）
- 新增 Critic Agent 步骤，修复验证通过后用 LLM 对比 diff 检查语义偏差（受 CodeMender 启发）
- 统一运行时自愈与 CI Auto-Fix 的知识库：新增 `failure_pattern` 表，Cron Job 每日同步
- 补齐集成测试：自愈闭环、Skill Intelligence、Chat Turn 核心业务流程

### 阶段二：Skill Intelligence 落地

- 完成 Skill Intelligence Phase 2-5 核心功能：经验报告 Cron Worker、API、前端展示
- `GenerateReport` 集成 `RootCauseEngine` 根因分析（通过抽取的 `RootCauseAnalyzer` 接口）
- 推荐排序引入动态权重调整（`DynamicRankFactors`），通过 `HealthMetricsProvider` 接口桥接 Biz 层（受 A-Evolve Gate 启发）
- 半自动进化 Curator Agent：通过 ChatOrchestrator 调用自身 Agent 生成 Skill 草案，Sandbox Runner 验证

### 阶段三：自我进化闭环

- 预测性自愈：基于历史模式预测即将发生的错误，提前干预
- Skill 五阶段进化闭环：Solve → Observe → Evolve → Gate → Reload（受 A-Evolve 启发）
- Gate 多维验证：功能正确性 + 安全性 + 性能 + 风格
- 知识库动态进化：从历史修复记录自动提取修复模板（受 Live-SWE-agent 启发），版本化 + 回滚 + 审计

## Capabilities

### New Capabilities

- `failure-report`: 标准化 CI/运行时失败表示结构体 + 解析器，统一错误描述格式
- `failure-pattern-store`: 统一失败模式知识库（SQLite），合并运行时规则与 CI 模式，支持动态挖掘和版本化
- `critic-agent`: LLM 语义回归检查 Agent，在 test/lint 通过后对比 diff 检查语义偏差
- `predictive-heal`: 基于历史模式的预测性自愈，从被动响应进化到主动预防
- `skill-evolution-loop`: Skill 五阶段进化闭环（Solve→Observe→Evolve→Gate→Reload），含多维 Gate 验证
- `pattern-mining`: 从历史修复记录自动提取修复模板，动态更新知识库

### Modified Capabilities

- `auto-fix-engine`: 新增 FailureReport 结构化输入 + Critic Agent 语义检查 + 保护文件白名单细化
- `skill-intelligence`: Phase 2-5 落地——经验报告 Cron Worker/API/前端、动态推荐排序、Curator Agent 半自动进化
- `root-cause-engine`: 抽取 `RootCauseAnalyzer` 接口，供 SkillIntelligenceUsecase 和 PredictiveHealUsecase 复用
- `skillrecommend-rank`: 引入 `DynamicRankFactors`，通过 `HealthMetricsProvider` 接口动态调整权重

## Impact

- **Biz 层**：`internal/biz/monitor/` 新增 `failure_report.go`/`failure_pattern_repo.go`/`predictive_heal.go`/`critic_agent.go`；`internal/biz/` 扩展 `skill_intelligence.go` 集成 RootCauseAnalyzer；`internal/biz/monitor/root_cause_engine.go` 抽取接口
- **Data 层**：`internal/data/ent/schema/` 新增 `failure_pattern.go`；`internal/data/` 新增 `failure_pattern.go`
- **Service 层**：`internal/service/` 新增 Skill Intelligence API 实现
- **Tools 层**：`internal/tools/skillrecommend/rank.go` 新增 `DynamicRankFactors` + `HealthMetricsProvider` 接口
- **CronRunner**：`internal/cronrunner/jobs/` 新增 `skill_intelligence_worker.go`/`failure_pattern_sync.go`/`predictive_heal.go`/`pattern_mining.go`
- **CI/CD**：`.github/workflows/auto-fix.yml` 改造日志解析 + 新增 Critic Agent 步骤
- **前端**：新增经验报告列表页、Skill 进化审批 UI
- **Proto**：新增 `skill_intelligence.proto`（ListExperienceReports / GetExperienceReport）
- **Wire DI**：新增 `FailurePatternRepo`/`RootCauseAnalyzer`/`HealthMetricsProvider` 绑定
- **集成测试**：新增 4 个集成测试文件

## Non-goals

- 不改变已有 `skill_health`/`experience_analytics`/`skill_evolution`/`skills_butler` 的业务逻辑
- 不改变 trpc-agent-go 框架层
- 不实现全自动化进化（仅半自动，需人工审批）
- 不做 K8s 部署配置或 staging 环境（defer 到独立变更）
- 不做性能自动调优（仅采集指标，不做自动参数调整）
- 不修改任何 proto 文件的已有定义（仅新增）
