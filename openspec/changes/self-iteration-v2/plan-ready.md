# 实现计划：self-iteration-v2

## 来源
- 提案：openspec/changes/self-iteration-v2/proposal.md
- 设计：openspec/changes/self-iteration-v2/design.md
- 规格：openspec/changes/self-iteration-v2/specs/
- 任务：openspec/changes/self-iteration-v2/tasks.md

## 实现步骤

### Task 1: RootCauseAnalyzer 接口抽取
- [x] **任务完成**（与 superpowers plan `Task 1`、`tasks.md` 对应条目同步勾选）
- 目标：从 RootCauseEngine 抽取 RootCauseAnalyzer 接口，供 SkillIntelligenceUsecase 和 PredictiveHealUsecase 复用
- 改动文件：
  - `internal/biz/monitor/root_cause_analyzer.go`（新增）
  - `internal/biz/monitor/root_cause_engine.go`（修改：实现接口）
  - `internal/biz/monitor/wire.go`（修改：添加 Wire 绑定）
- 验证方式：`go build ./internal/biz/...` && `go test ./internal/biz/monitor/... -count=1` && `make wire && go build ./cmd/admin`

### Task 2: FailureReport 标准化错误表示
- [x] **任务完成**（与 superpowers plan `Task 2`、`tasks.md` 对应条目同步勾选）
- 目标：定义 FailureReport 结构体和解析器，统一 CI 和运行时的错误描述格式
- 改动文件：
  - `internal/biz/monitor/failure_report.go`（新增：FailureReport 结构体 + FailureType 常量）
  - `internal/biz/monitor/failure_report_parser.go`（新增：ParseCILogs + ParseRuntimeError）
  - `internal/biz/monitor/failure_report_parser_test.go`（新增：测试）
  - `.auto-fix/scripts/parse-logs.py`（新增：CI 侧 Python 脚本）
- 验证方式：`go test ./internal/biz/monitor/... -run TestParse -count=1` 绿色

### Task 3: 统一失败模式知识库
- [x] **任务完成**（与 superpowers plan `Task 3`、`tasks.md` 对应条目同步勾选）
- 目标：新增 failure_pattern 表，统一存储运行时自愈规则和 CI Auto-Fix 模式
- 改动文件：
  - `internal/data/ent/schema/failure_pattern.go`（新增：Ent Schema + 索引）
  - `internal/biz/monitor/failure_pattern_repo.go`（新增：Reader/Writer 接口）
  - `internal/data/failure_pattern.go`（新增：Repo 实现 + Wire 绑定）
  - `internal/cronrunner/jobs/failure_pattern_sync.go`（新增：Cron Job）
- 验证方式：`go generate ./internal/data/ent/...` 无错误 && `go test ./internal/data/... -run TestFailurePattern -count=1` 绿色

### Task 4: Auto-Fix 引擎改造
- [x] **任务完成**（与 superpowers plan `Task 4`、`tasks.md` 对应条目同步勾选）
- 目标：改造 auto-fix.yml，新增结构化日志解析、Critic Agent 步骤、保护文件白名单
- 改动文件：
  - `.github/workflows/auto-fix.yml`（修改：4 处改动）
- 验证方式：YAML 语法正确，actionlint 通过

### Task 5: 集成测试补齐
- [x] **任务完成**（与 superpowers plan `Task 5`、`tasks.md` 对应条目同步勾选）
- 目标：补齐自愈闭环、Skill Intelligence、Chat Turn 核心业务流程的集成测试
- 改动文件：
  - `internal/service/monitor_integration_test.go`（新增）
  - `internal/service/skill_intelligence_integration_test.go`（新增）
  - `internal/service/chat_turn_integration_test.go`（新增）
- 验证方式：`make api && make wire && make build && make test && make lint`

### Task 6: 经验报告诊断
- [x] **任务完成**（与 superpowers plan `Task 6`、`tasks.md` 对应条目同步勾选）
- 目标：扩展 ExperienceReport 字段，集成 RootCauseAnalyzer，新增 Cron Worker 和 API
- 改动文件：
  - `internal/biz/skill_intelligence_types.go`（修改：新增字段）
  - `internal/biz/skill_intelligence.go`（修改：GenerateReport 集成 RootCauseAnalyzer）
  - `internal/data/ent/schema/experience_report.go`（修改：新增字段）
  - `internal/data/skill_intelligence.go`（修改：持久化新字段）
  - `internal/cronrunner/jobs/skill_intelligence_worker.go`（修改：替换占位实现）
  - `api/kratos/skill_intelligence/v1/skill_intelligence.proto`（修改：新增 RPC）
  - `internal/service/skill_intelligence.go`（修改：实现新 RPC）
  - Wire DI 装配
- 验证方式：`make api && make wire && make build` 通过

### Task 7: 推荐排序进化
- [x] **任务完成**（与 superpowers plan `Task 7`、`tasks.md` 对应条目同步勾选）
- 目标：引入 DynamicRankFactors + HealthMetricsProvider 接口桥接
- 改动文件：
  - `internal/tools/skillrecommend/health_provider.go`（新增：接口定义）
  - `internal/tools/skillrecommend/rank.go`（修改：新增 DynamicRankFactors）
  - `internal/tools/skillrecommend/rank_test.go`（修改：测试动态权重）
  - `internal/biz/` 适配器（新增：实现 HealthMetricsProvider）
  - `internal/tools/skillruntime/resolve.go`（修改：调用 DynamicRankFactors）
  - `internal/tools/skillrecommend/rank_feedback.go`（新增：RankFeedback）
- 验证方式：`go test ./internal/tools/skillrecommend/... -count=1` 绿色

### Task 8: Curator Agent 半自动进化
- [x] **任务完成**（与 superpowers plan `Task 8`、`tasks.md` 对应条目同步勾选）
- 目标：实现 Curator Agent 半自动进化流程（触发判定→草案生成→Sandbox 验证→审批）
- 改动文件：
  - `internal/biz/skill_evolution_suggestion_types.go`（新增：领域模型）
  - `internal/data/ent/schema/skill_evolution_suggestion.go`（修改：新增字段）
  - `internal/biz/skill_intelligence_repo.go`（修改：新增端口）
  - `internal/data/skill_evolution_suggestion.go`（新增：Repo 实现）
  - `internal/biz/skill_intelligence.go`（修改：触发条件判定 + CreateSuggestion）
  - `internal/service/skill_curator.go`（新增：Curator Agent 装配）
  - Sandbox Runner 验证
  - 进化建议 API proto + Service
  - Skill 元数据扩展
  - Wire DI 装配
- 验证方式：`make api && make wire && make build` 通过

### Task 9: 前端经验报告与进化审批
- [x] **任务完成**（与 superpowers plan `Task 9`、`tasks.md` 对应条目同步勾选）
- 目标：前端经验报告列表页 + Skill 进化审批 UI
- 改动文件：
  - 前端经验报告列表页组件
  - 前端 Skill 进化审批 UI 组件
- 验证方式：`pnpm lint && pnpm build` 通过

### Task 10: 预测性自愈
- [x] **任务完成**（与 superpowers plan `Task 10`、`tasks.md` 对应条目同步勾选）
- 目标：基于历史模式的预测性自愈，从被动响应进化到主动预防
- 改动文件：
  - `internal/biz/monitor/predictive_heal.go`（新增：PredictiveHealUsecase）
  - `internal/cronrunner/jobs/predictive_heal.go`（新增：Cron Job）
  - `internal/biz/monitor/predictive_heal_test.go`（新增：测试）
  - Wire DI 装配
- 验证方式：`go test ./internal/biz/monitor/... -run TestPredictive -count=1` 绿色

### Task 11: Skill 五阶段进化闭环
- [x] **任务完成**（与 superpowers plan `Task 11`、`tasks.md` 对应条目同步勾选）
- 目标：实现 Solve→Observe→Evolve→Gate→Reload 五阶段流程 + Gate 多维验证 + 过期机制
- 改动文件：
  - `internal/biz/skill_evolution_loop.go`（新增：五阶段流程）
  - `internal/biz/skill_evolution_loop_test.go`（新增：测试）
- 验证方式：`go test ./internal/biz/... -run TestEvolutionLoop -count=1` 绿色

### Task 12: 知识库动态挖掘
- [ ] **任务完成**（与 superpowers plan `Task 12`、`tasks.md` 对应条目同步勾选）
- 目标：从历史修复记录自动提取修复模板，动态更新知识库
- 改动文件：
  - `internal/biz/monitor/pattern_mining.go`（新增：PatternMiningUsecase）
  - `internal/cronrunner/jobs/pattern_mining.go`（新增：Cron Job）
  - `internal/biz/monitor/pattern_mining_test.go`（新增：测试）
  - Wire DI 装配
- 验证方式：`go test ./internal/biz/monitor/... -run TestPatternMining -count=1` 绿色

### Task 13: 全量验证
- [ ] **任务完成**（与 superpowers plan `Task 13`、`tasks.md` 对应条目同步勾选）
- 目标：后端 + 前端全量验证通过
- 改动文件：无新增，仅验证
- 验证方式：`make api && make wire && make build && make test && make lint` && `cd web && pnpm lint && pnpm test && pnpm build`
