# Self-Iteration V2 — 任务清单

**Goal:** 激活自愈闭环 + 落地 Skill Intelligence + 建立进化闭环，从"被动修复"进化到"主动进化"。

**Design Doc:** [design.md](./design.md)

**Non-goals:**
- 不改变已有 skill_health/skill_evolution/skills_butler 的业务逻辑
- 不改变 trpc-agent-go 框架层
- 不实现全自动化进化（仅半自动，需人工审批）
- 不做 K8s 部署或 staging 环境
- 不做性能自动调优

---

## 1. Phase 1 — 闭环加固：RootCauseAnalyzer 接口抽取

- [x] 1.1 创建 `internal/biz/monitor/root_cause_analyzer.go`：定义 `RootCauseAnalyzer` 接口（Analyze + AnalyzeFromReport 方法）。DoD: `go build ./internal/biz/...` 通过 <!-- 已实现: RootCauseAnalyzer 接口定义 -->
- [x] 1.2 修改 `internal/biz/monitor/root_cause_engine.go`：让 `RootCauseEngine` 实现 `RootCauseAnalyzer` 接口。DoD: `go build ./internal/biz/...` 通过 <!-- 已实现: Analyze 方法委托 Evaluate -->
- [x] 1.3 Wire 绑定：在 `internal/biz/wire.go` 中添加 `RootCauseAnalyzer` 的 Wire 绑定。DoD: `make wire && go build ./cmd/admin` 通过 <!-- 已实现: wire.Bind + NewRootCauseEngine -->
- [x] 1.4 验证：`go test ./internal/biz/monitor/... -count=1` 绿色 <!-- 已实现: 接口满足测试通过 -->

## 2. Phase 1 — 闭环加固：FailureReport 标准化错误表示

- [x] 2.1 创建 `internal/biz/monitor/failure_report.go`：定义 `FailureReport` 结构体和 `FailureType` 常量。DoD: `go build ./internal/biz/...` 通过 <!-- 已实现: FailureReport + FailureType -->
- [x] 2.2 创建 `internal/biz/monitor/failure_report_parser.go`：实现 `ParseCILogs` 和 `ParseRuntimeError` 函数。DoD: `go build ./internal/biz/...` 通过 <!-- 已实现: 解析器 + AnalyzeFromReport -->
- [x] 2.3 创建 `internal/biz/monitor/failure_report_parser_test.go`：测试 Go 编译错误/测试失败/Lint 错误解析。DoD: `go test ./internal/biz/monitor/... -run TestParse -count=1` 绿色 <!-- 已实现: 13 个测试用例通过 -->
- [x] 2.4 创建 `.auto-fix/scripts/parse-logs.py`：CI 侧 Python 脚本，将原始日志解析为 FailureReport JSON。DoD: `python3 parse-logs.py < testdata/build_failure.txt` 输出有效 JSON <!-- 已实现: Python CLI 脚本 -->

## 3. Phase 1 — 闭环加固：统一失败模式知识库

- [x] 3.1 创建 `internal/data/ent/schema/failure_pattern.go`：Ent Schema + 索引 (source,type) + (pattern_hash) + (is_active,confidence)。DoD: `go generate ./internal/data/ent/...` 无错误 <!-- 已实现: Ent Schema 生成成功 -->
- [x] 3.2 创建 `internal/biz/monitor/failure_pattern_repo.go`：定义 `FailurePatternReader`/`FailurePatternWriter` 接口。DoD: `go build ./internal/biz/...` 通过 <!-- 已实现: Reader/Writer 接口 -->
- [x] 3.3 创建 `internal/data/failure_pattern.go`：实现 Repo 接口 + Wire 绑定。DoD: `go build ./internal/data/...` 通过 <!-- 已实现: raw SQL repo + 7 个测试通过 -->
- [x] 3.4 创建 `internal/cronrunner/jobs/failure_pattern_sync.go`：Cron Job，每日从 RootCauseEngine 规则 + patterns.jsonl 同步到 failure_pattern 表。DoD: `go build ./internal/cronrunner/...` 通过 <!-- 已实现: syncRuntimeRules + syncCIPatterns -->
- [x] 3.5 验证：`go test ./internal/data/... -run TestFailurePattern -count=1` 绿色 <!-- 已实现: 7/7 PASS -->

## 4. Phase 1 — 闭环加固：Auto-Fix 引擎改造

- [x] 4.1 修改 `.github/workflows/auto-fix.yml`：新增 parse-logs.py 步骤，将原始日志解析为 FailureReport JSON，传给后续步骤。DoD: YAML 语法正确 <!-- 已实现: Parse failure logs 步骤 -->
- [x] 4.2 修改 `.github/workflows/auto-fix.yml`：新增 Critic Agent 步骤（调用 ARANEA_API_URL + ARANEA_CRITIC_SESSION），根据 risk_level 决定是否创建 PR。DoD: YAML 语法正确 <!-- 已实现: Critic Agent review 步骤 -->
- [x] 4.3 修改 `.github/workflows/auto-fix.yml`：新增保护文件白名单（允许 internal/biz/monitor/ 目录）。DoD: YAML 语法正确 <!-- 已实现: WHITELIST_DIRS 白名单 -->
- [x] 4.4 修改 `.github/workflows/auto-fix.yml`：新增 ENABLE_CRITIC_AGENT 环境变量支持。DoD: YAML 语法正确 <!-- 已实现: GitHub Variables 支持 -->
- [x] 4.5 验证：`actionlint .github/workflows/auto-fix.yml` 通过（或手动检查语法） <!-- 已实现: js-yaml 验证通过 -->

## 5. Phase 1 — 闭环加固：集成测试补齐

- [x] 5.1 创建 `internal/service/monitor_integration_test.go`：自愈闭环集成测试（注入错误→检测→根因分析→FixAction 生成→验证）。DoD: `go test -tags=integration ./internal/service/... -run TestMonitorIntegration -count=1` 绿色 <!-- 已实现: FailurePattern CRUD + Mining + SelfHeal 联动 -->
- [x] 5.2 创建 `internal/service/skill_intelligence_integration_test.go`：Skill Intelligence 集成测试（Skill 调用失败→AnalyzeInvocation→GenerateReport→持久化→查询）。DoD: `go test -tags=integration ./internal/service/... -run TestSkillIntelligenceIntegration -count=1` 绿色 <!-- 已实现: GenerateReport+RCA + EvolutionTriggers -->
- [x] 5.3 创建 `internal/service/chat_turn_integration_test.go`：Chat Turn 集成测试（创建 Session→发送消息→Agent 响应→Memory 写入）。DoD: `go test -tags=integration ./internal/service/... -run TestChatTurnIntegration -count=1` 绿色 <!-- 已实现: 23 个子测试 -->
- [x] 5.4 Phase 1 全量验证：`make api && make wire && make build && make test && make lint` <!-- 已实现: 通过 -->

## 6. Phase 2 — Skill Intelligence 落地：经验报告诊断

- [x] 6.1 扩展 `internal/biz/skill_intelligence_types.go`：ExperienceReport 新增 RootCauseAnalysis 和 SuggestedFix 字段。DoD: `go build ./internal/biz/...` 通过 <!-- 已实现: 新增 2 个字段 -->
- [x] 6.2 修改 `internal/biz/skill_intelligence.go`：GenerateReport 集成 RootCauseAnalyzer 接口，调用 AnalyzeFromReport。DoD: `go build ./internal/biz/...` 通过 <!-- 已实现: RCA 集成 + adapter -->
- [x] 6.3 创建 `internal/data/ent/schema/experience_report.go`：Ent Schema + 索引 (skill_id, created_at)。DoD: `go generate ./internal/data/ent/...` 无错误 <!-- 已实现: root_cause_analysis + suggested_fix 字段 -->
- [x] 6.4 修改 `internal/data/skill_intelligence.go`：实现新增字段持久化。DoD: `go build ./internal/data/...` 通过 <!-- 已实现: Create/BatchCreate/ListUnanalyzed/MarkAnalyzed -->
- [x] 6.5 创建 `internal/cronrunner/jobs/skill_intelligence_worker.go`：每 10 分钟扫描未分析的 skill_invocation，批量 AnalyzeInvocation/ScoreSkill/GenerateReport。DoD: `go build ./internal/cronrunner/...` 通过 <!-- 已实现: 替换占位实现为真实批处理 -->
- [x] 6.6 定义 `api/skill_intelligence/v1/skill_intelligence.proto`：ListExperienceReports / GetExperienceReport。DoD: `make api` 通过 <!-- 已实现: field 14+15 -->
- [x] 6.7 创建 `internal/service/skill_intelligence.go`：实现 Service 层。DoD: `go build ./internal/service/...` 通过 <!-- 已实现: toProto 映射新字段 -->
- [x] 6.8 Wire DI 装配：新增 SkillIntelligenceService + Cron Job 注册。DoD: `make wire && make build` 通过 <!-- 已实现: RCA adapter + 注入 -->

## 7. Phase 2 — Skill Intelligence 落地：推荐排序进化

- [x] 7.1 创建 `internal/tools/skillrecommend/health_provider.go`：定义 `HealthMetricsProvider` 接口（GetRecentSuccessRate + GetRecentAvgDuration）。DoD: `go build ./internal/tools/...` 通过 <!-- 已实现: 接口定义 -->
- [x] 7.2 修改 `internal/tools/skillrecommend/rank.go`：新增 `DynamicRankFactors` 函数，从 HealthMetricsProvider 读取近期指标动态调整权重。DoD: `go build ./internal/tools/...` 通过 <!-- 已实现: 动态权重调整 -->
- [x] 7.3 创建 `internal/tools/skillrecommend/rank_test.go`：测试动态权重调整（高成功率/低成功率/无数据场景）。DoD: `go test ./internal/tools/skillrecommend/... -count=1` 绿色 <!-- 已实现: 4 个测试通过 -->
- [x] 7.4 创建 Biz 层适配器：在 `internal/biz/` 中实现 `HealthMetricsProvider` 接口（适配 SkillHealthAggregator）。DoD: `go build ./internal/biz/...` 通过 <!-- 已实现: SkillHealthMetricsAdapter -->
- [x] 7.5 修改 `internal/tools/skillruntime/resolve.go`：在 ResolveSkillSlugsDetailed 中调用 DynamicRankFactors。DoD: 排序因子写入 selection_reason <!-- 已实现: 替换 DefaultRankFactors -->
- [x] 7.6 创建 `internal/tools/skillrecommend/rank_feedback.go`：定义 RankFeedback 结构体和记录逻辑。DoD: `go build ./internal/tools/...` 通过 <!-- 已实现: RankFeedback 结构体 -->

## 8. Phase 2 — Skill Intelligence 落地：Curator Agent 半自动进化

- [ ] 8.1 创建 `internal/biz/skill_evolution_suggestion_types.go`：SkillEvolutionSuggestion 领域模型 + 类型/状态枚举。DoD: `go build ./internal/biz/...` 通过
- [ ] 8.2 创建 `internal/data/ent/schema/skill_evolution_suggestion.go`：Ent Schema + 索引。DoD: `go generate ./internal/data/ent/...` 无错误
- [ ] 8.3 扩展 `internal/biz/skill_intelligence_repo.go`：定义 SkillEvolutionSuggestionReader/Writer 端口。DoD: `go build ./internal/biz/...` 通过
- [ ] 8.4 实现 Data 层 Repo：`internal/data/skill_evolution_suggestion.go`。DoD: `go build ./internal/data/...` 通过
- [ ] 8.5 修改 `internal/biz/skill_intelligence.go`：实现触发条件判定（7d 成功率 < 60% 或同一失败标签 >= 5 次）+ CreateSuggestion。DoD: `go build ./internal/biz/...` 通过
- [ ] 8.6 创建 `internal/service/skill_curator.go`：Curator Agent 装配与 invoke（通过 ChatOrchestrator 调用自身 Agent）。DoD: `go build ./internal/service/...` 通过
- [ ] 8.7 实现 Sandbox Runner 验证：使用 codeexecutor.CodeExecutor 隔离执行。DoD: 隔离执行，不影响生产
- [ ] 8.8 定义进化建议 API proto + 实现 Service 层。DoD: `make api && go build ./...` 通过
- [ ] 8.9 Skill 元数据扩展：新增 parent_version_id / evolution_reason / lifecycle_status 字段。DoD: `go generate ./internal/data/ent/...` 无错误
- [ ] 8.10 Wire DI 装配。DoD: `make wire && make build` 通过

## 9. Phase 2 — Skill Intelligence 落地：前端

- [ ] 9.1 前端经验报告列表页：调用 ListExperienceReports API，显示失败标签分布图 + 根因分析卡片。DoD: `pnpm lint && pnpm build` 通过
- [ ] 9.2 前端 Skill 进化审批 UI：显示进化建议列表 + Approve/Reject 操作。DoD: `pnpm lint && pnpm build` 通过

## 10. Phase 3 — 自我进化闭环：预测性自愈

- [ ] 10.1 创建 `internal/biz/monitor/predictive_heal.go`：PredictiveHealUsecase，基于 FailurePattern 知识库的趋势预测。DoD: `go build ./internal/biz/...` 通过
- [ ] 10.2 创建 `internal/cronrunner/jobs/predictive_heal.go`：每 5 分钟扫描系统指标 + 匹配前置条件模式。DoD: `go build ./internal/cronrunner/...` 通过
- [ ] 10.3 创建 `internal/biz/monitor/predictive_heal_test.go`：测试置信度阈值 + 冷却期 + 审计记录。DoD: `go test ./internal/biz/monitor/... -run TestPredictive -count=1` 绿色
- [ ] 10.4 Wire DI 装配。DoD: `make wire && make build` 通过

## 11. Phase 3 — 自我进化闭环：Skill 五阶段进化闭环

- [ ] 11.1 创建 `internal/biz/skill_evolution_loop.go`：实现 Solve→Observe→Evolve→Gate→Reload 五阶段流程。DoD: `go build ./internal/biz/...` 通过
- [ ] 11.2 实现 Gate 多维验证：功能正确性（Sandbox Runner）+ 安全性（敏感信息检测）+ 性能（Token/耗时对比 >20% 拒绝）+ 风格（araneactl lint）。DoD: `go build ./internal/biz/...` 通过
- [ ] 11.3 实现进化建议过期机制：7 天未审批自动标记 expired。DoD: `go build ./internal/biz/...` 通过
- [ ] 11.4 创建 `internal/biz/skill_evolution_loop_test.go`：测试五阶段流程 + Gate 验证 + 过期机制。DoD: `go test ./internal/biz/... -run TestEvolutionLoop -count=1` 绿色

## 12. Phase 3 — 自我进化闭环：知识库动态挖掘

- [ ] 12.1 创建 `internal/biz/monitor/pattern_mining.go`：PatternMiningUsecase，聚类相似失败模式 + 提取共性修复策略。DoD: `go build ./internal/biz/...` 通过
- [ ] 12.2 创建 `internal/cronrunner/jobs/pattern_mining.go`：每日执行挖掘，写入 failure_pattern 表（source="mined"）。DoD: `go build ./internal/cronrunner/...` 通过
- [ ] 12.3 创建 `internal/biz/monitor/pattern_mining_test.go`：测试聚类 + 置信度提升 + 自动禁用。DoD: `go test ./internal/biz/monitor/... -run TestPatternMining -count=1` 绿色
- [ ] 12.4 Wire DI 装配。DoD: `make wire && make build` 通过

## 13. 全量验证

- [ ] 13.1 后端全量验证：`make api && make wire && make build && make test && make lint`
- [ ] 13.2 前端全量验证：`cd web && pnpm lint && pnpm test && pnpm build`
