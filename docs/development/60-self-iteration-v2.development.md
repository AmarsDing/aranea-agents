# Self-Iteration V2 — 开发计划

> **版本**：2026-06-17 | **状态**：✅ Phase 1–3 已落地
> **需求**：[60-self-iteration-v2.md](./60-self-iteration-v2.md) · **设计**：[60-self-iteration-v2.design.md](./60-self-iteration-v2.design.md)
> **OpenSpec Change**：`openspec/changes/self-iteration-v2/`

---

## 1. 模块定位

自我迭代 V2：从被动修复到主动进化的自我迭代闭环。激活 CI Auto-Fix、统一知识库、落地 Skill Intelligence、建立进化闭环。

**代码锚点**：

| 层 | 路径 |
|----|----------|
| Biz | `internal/biz/monitor/root_cause_analyzer.go`、`internal/biz/monitor/failure_report.go`、`internal/biz/monitor/failure_report_parser.go`、`internal/biz/monitor/failure_pattern_repo.go`、`internal/biz/monitor/predictive_heal.go`、`internal/biz/monitor/pattern_mining.go`、`internal/biz/skill_evolution_loop.go`、`internal/biz/skill_evolution_suggestion_types.go` |
| Biz（扩展） | `internal/biz/skill_intelligence.go`（集成 RootCauseAnalyzer）、`internal/biz/skill_intelligence_repo.go`（新增端口） |
| Data | `internal/data/ent/schema/failure_pattern.go`、`internal/data/ent/schema/skill_evolution_suggestion.go`、`internal/data/ent/schema/experience_report.go`、`internal/data/failure_pattern_repo.go`、`internal/data/skill_evolution_suggestion.go` |
| Service | `internal/service/skill_intelligence.go`、`internal/service/skill_curator.go`、`internal/service/skill_evolution_suggestion.go` |
| Tools | `internal/tools/skillrecommend/health_provider.go`、`internal/tools/skillrecommend/rank.go`（扩展）、`internal/tools/skillrecommend/rank_feedback.go` |
| Cron | `internal/cronrunner/jobs/skill_intelligence_worker.go`、`internal/cronrunner/jobs/failure_pattern_sync.go`、`internal/cronrunner/jobs/predictive_heal.go`、`internal/cronrunner/jobs/pattern_mining.go` |
| Wire DI | `cmd/admin/wire.go`（Provider + 绑定）、`cmd/admin/workers.go`（Cron Job 启动注册） |
| Proto | `api/kratos/skill_intelligence/v1/skill_intelligence.proto`、`api/kratos/skill_evolution_suggestion/v1/skill_evolution_suggestion.proto` |
| CI/CD | `.github/workflows/auto-fix.yml`（改造）、`.auto-fix/scripts/parse-logs.py`（新增） |
| 前端 | `web/src/pages/ExperienceReportListPage.vue`、`web/src/pages/EvolutionSuggestionListPage.vue` |
| 集成测试 | `internal/service/monitor_integration_test.go`、`internal/service/skill_intelligence_integration_test.go`、`internal/service/chat_turn_integration_test.go` |

---

## 2. 依赖关系图

```
Phase 1（闭环加固）
├── 1. RootCauseAnalyzer 接口抽取 ─────────────────────┐
│   1.1 定义接口                                        │
│   1.2 RootCauseEngine 实现接口                        │
│   1.3 Wire 绑定                                       │
│   1.4 验证                                            │
├── 2. FailureReport 标准化错误表示 ────────────────────┤
│   2.1 定义结构体                                      │
│   2.2 解析器                                          │
│   2.3 解析器测试                                      │
│   2.4 CI Python 脚本                                  │
├── 3. 统一失败模式知识库 ─────────────────────────────┤
│   3.1 Ent Schema ← 依赖 2.1(FailureReport)           │
│   3.2 Reader/Writer 接口                              │
│   3.3 Data 层实现                                     │
│   3.4 Cron Job ← 依赖 1.1(RootCauseAnalyzer)         │
│   3.5 验证                                            │
├── 4. Auto-Fix 引擎改造 ← 依赖 2.4 + 3.3             │
│   4.1 结构化日志解析步骤                               │
│   4.2 Critic Agent 步骤                               │
│   4.3 白名单细化                                      │
│   4.4 环境变量支持                                    │
│   4.5 验证                                            │
└── 5. 集成测试补齐 ← 依赖 1~4 全部完成                │
    5.1 自愈闭环集成测试                                 │
    5.2 Skill Intelligence 集成测试                      │
    5.3 Chat Turn 集成测试                               │
    5.4 Phase 1 全量验证

Phase 2（Skill Intelligence 落地）← 依赖 Phase 1 完成
├── 6. 经验报告诊断 ← 依赖 1.1(RootCauseAnalyzer)      │
│   6.1 ExperienceReport 扩展                           │
│   6.2 GenerateReport 集成 RootCauseAnalyzer           │
│   6.3 Ent Schema                                      │
│   6.4 Data 层实现                                     │
│   6.5 skill_intelligence_worker Cron Job              │
│   6.6 Proto 定义                                      │
│   6.7 Service 层                                      │
│   6.8 Wire DI 装配                                    │
├── 7. 推荐排序进化                                     │
│   7.1 HealthMetricsProvider 接口                      │
│   7.2 DynamicRankFactors                              │
│   7.3 测试                                            │
│   7.4 Biz 层适配器                                    │
│   7.5 ResolveSkillSlugsDetailed 集成                  │
│   7.6 RankFeedback                                    │
├── 8. Curator Agent 半自动进化                         │
│   8.1 SkillEvolutionSuggestion 类型                   │
│   8.2 Ent Schema                                      │
│   8.3 Reader/Writer 端口                              │
│   8.4 Data 层实现                                     │
│   8.5 触发条件判定                                     │
│   8.6 Curator Agent 装配                              │
│   8.7 Sandbox Runner 验证                             │
│   8.8 进化建议 API（Proto + Service）                 │
│   8.9 Skill 元数据扩展                                │
│   8.10 Wire DI 装配                                   │
└── 9. 前端 ← 依赖 6.7 + 8.8                           │
    9.1 经验报告列表页                                   │
    9.2 Skill 进化审批 UI

Phase 3（自我进化闭环）← 依赖 Phase 2 完成
├── 10. 预测性自愈 ← 依赖 1.1 + 3.3                    │
│   10.1 PredictiveHealUsecase                          │
│   10.2 Cron Job                                       │
│   10.3 测试                                           │
│   10.4 Wire DI 装配                                   │
├── 11. Skill 五阶段进化闭环 ← 依赖 8 + 10             │
│   11.1 五阶段流程                                     │
│   11.2 Gate 多维验证                                  │
│   11.3 过期机制                                       │
│   11.4 测试                                           │
├── 12. 知识库动态挖掘 ← 依赖 3.3                      │
│   12.1 PatternMiningUsecase                           │
│   12.2 Cron Job                                       │
│   12.3 测试                                           │
│   12.4 Wire DI 装配                                   │
└── 13. 全量验证
    13.1 后端全量验证
    13.2 前端全量验证
```

---

## 3. 任务清单

### Phase 1 — 闭环加固：RootCauseAnalyzer 接口抽取

| ID | 任务 | 层 | 优先级 | 状态 | DoD |
|----|------|-----|--------|------|-----|
| 1.1 | 创建 `internal/biz/monitor/root_cause_analyzer.go`：定义 `RootCauseAnalyzer` 接口 | Biz | P0 | ✅ | `go build ./internal/biz/...` 通过 |
| 1.2 | 修改 `internal/biz/monitor/root_cause_engine.go`：让 `RootCauseEngine` 实现 `RootCauseAnalyzer` 接口 | Biz | P0 | ✅ | `go build ./internal/biz/...` 通过 |
| 1.3 | Wire 绑定：在 `cmd/admin/wire.go` 中添加 `RootCauseAnalyzer` 的 Wire 绑定 | Wire | P0 | ✅ | `make wire && go build ./cmd/admin` 通过 |
| 1.4 | 验证 | — | P0 | ✅ | `go test ./internal/biz/monitor/... -count=1` 绿色 |

### Phase 1 — 闭环加固：FailureReport 标准化错误表示

| ID | 任务 | 层 | 优先级 | 状态 | DoD |
|----|------|-----|--------|------|-----|
| 2.1 | 创建 `internal/biz/monitor/failure_report.go`：定义 `FailureReport` 结构体和 `FailureType` 常量 | Biz | P0 | ✅ | `go build ./internal/biz/...` 通过 |
| 2.2 | 创建 `internal/biz/monitor/failure_report_parser.go`：实现 `ParseCILogs` 和 `ParseRuntimeError` 函数 | Biz | P0 | ✅ | `go build ./internal/biz/...` 通过 |
| 2.3 | 创建 `internal/biz/monitor/failure_report_parser_test.go`：测试 Go 编译错误/测试失败/Lint 错误解析 | Biz | P0 | ✅ | `go test ./internal/biz/monitor/... -run TestParse -count=1` 绿色 |
| 2.4 | 创建 `.auto-fix/scripts/parse-logs.py`：CI 侧 Python 脚本 | CI | P0 | ✅ | `python3 parse-logs.py < testdata/build_failure.txt` 输出有效 JSON |

### Phase 1 — 闭环加固：统一失败模式知识库

| ID | 任务 | 层 | 优先级 | 状态 | DoD |
|----|------|-----|--------|------|-----|
| 3.1 | 创建 `internal/data/ent/schema/failure_pattern.go`：Ent Schema + 索引 | Data | P0 | ✅ | `go generate ./internal/data/ent/...` 无错误 |
| 3.2 | 创建 `internal/biz/monitor/failure_pattern_repo.go`：定义 `FailurePatternReader`/`FailurePatternWriter` 接口 | Biz | P0 | ✅ | `go build ./internal/biz/...` 通过 |
| 3.3 | 创建 `internal/data/failure_pattern_repo.go`：实现 Repo 接口 + Wire 绑定 | Data | P0 | ✅ | `go build ./internal/data/...` 通过 |
| 3.4 | 创建 `internal/cronrunner/jobs/failure_pattern_sync.go`：Cron Job | Cron | P1 | ✅ | `go build ./internal/cronrunner/...` 通过 |
| 3.5 | 验证 | — | P0 | ✅ | `go test ./internal/data/... -run TestFailurePattern -count=1` 绿色 |

### Phase 1 — 闭环加固：Auto-Fix 引擎改造

| ID | 任务 | 层 | 优先级 | 状态 | DoD |
|----|------|-----|--------|------|-----|
| 4.1 | 修改 `.github/workflows/auto-fix.yml`：新增 parse-logs.py 步骤 | CI | P1 | ✅ | YAML 语法正确 |
| 4.2 | 修改 `.github/workflows/auto-fix.yml`：新增 Critic Agent 步骤 | CI | P1 | ✅ | YAML 语法正确 |
| 4.3 | 修改 `.github/workflows/auto-fix.yml`：新增保护文件白名单 | CI | P1 | ✅ | YAML 语法正确 |
| 4.4 | 修改 `.github/workflows/auto-fix.yml`：新增 ENABLE_CRITIC_AGENT 环境变量 | CI | P1 | ✅ | YAML 语法正确 |
| 4.5 | 验证 | — | P1 | ✅ | `actionlint .github/workflows/auto-fix.yml` 通过 |

### Phase 1 — 闭环加固：集成测试补齐

| ID | 任务 | 层 | 优先级 | 状态 | DoD |
|----|------|-----|--------|------|-----|
| 5.1 | 创建 `internal/service/monitor_integration_test.go`：自愈闭环集成测试 | Service | P1 | ✅ | `go test -tags=integration ./internal/service/... -run TestMonitorIntegration -count=1` 绿色 |
| 5.2 | 创建 `internal/service/skill_intelligence_integration_test.go`：Skill Intelligence 集成测试 | Service | P1 | ✅ | `go test -tags=integration ./internal/service/... -run TestSkillIntelligenceIntegration -count=1` 绿色 |
| 5.3 | 创建 `internal/service/chat_turn_integration_test.go`：Chat Turn 集成测试 | Service | P1 | ✅ | `go test -tags=integration ./internal/service/... -run TestChatTurnIntegration -count=1` 绿色 |
| 5.4 | Phase 1 全量验证 | — | P0 | ✅ | `make api && make wire && make build && make test && make lint` |

### Phase 2 — Skill Intelligence 落地：经验报告诊断

| ID | 任务 | 层 | 优先级 | 状态 | DoD |
|----|------|-----|--------|------|-----|
| 6.1 | 扩展 `internal/biz/skill_intelligence_types.go`：ExperienceReport 新增字段 | Biz | P1 | ✅ | `go build ./internal/biz/...` 通过 |
| 6.2 | 修改 `internal/biz/skill_intelligence.go`：GenerateReport 集成 RootCauseAnalyzer | Biz | P1 | ✅ | `go build ./internal/biz/...` 通过 |
| 6.3 | 创建 `internal/data/ent/schema/experience_report.go`：Ent Schema + 索引 | Data | P1 | ✅ | `go generate ./internal/data/ent/...` 无错误 |
| 6.4 | 修改 `internal/data/skill_intelligence.go`：实现新增字段持久化 | Data | P1 | ✅ | `go build ./internal/data/...` 通过 |
| 6.5 | 创建 `internal/cronrunner/jobs/skill_intelligence_worker.go`：Cron Job | Cron | P1 | ✅ | `go build ./internal/cronrunner/...` 通过 |
| 6.6 | 定义 `api/kratos/skill_intelligence/v1/skill_intelligence.proto` | Proto | P1 | ✅ | `make api` 通过 |
| 6.7 | 创建 `internal/service/skill_intelligence.go`：Service 层 | Service | P1 | ✅ | `go build ./internal/service/...` 通过 |
| 6.8 | Wire DI 装配 | Wire | P1 | ✅ | `make wire && make build` 通过 |

### Phase 2 — Skill Intelligence 落地：推荐排序进化

| ID | 任务 | 层 | 优先级 | 状态 | DoD |
|----|------|-----|--------|------|-----|
| 7.1 | 创建 `internal/tools/skillrecommend/health_provider.go`：`HealthMetricsProvider` 接口 | Tools | P1 | ✅ | `go build ./internal/tools/...` 通过 |
| 7.2 | 修改 `internal/tools/skillrecommend/rank.go`：新增 `DynamicRankFactors` | Tools | P1 | ✅ | `go build ./internal/tools/...` 通过 |
| 7.3 | 创建 `internal/tools/skillrecommend/rank_test.go`：测试动态权重调整 | Tools | P1 | ✅ | `go test ./internal/tools/skillrecommend/... -count=1` 绿色 |
| 7.4 | 创建 Biz 层适配器：实现 `HealthMetricsProvider` 接口 | Biz | P1 | ✅ | `go build ./internal/biz/...` 通过 |
| 7.5 | 修改 `internal/tools/skillruntime/resolve.go`：集成 DynamicRankFactors | Tools | P1 | ✅ | 排序因子写入 selection_reason |
| 7.6 | 创建 `internal/tools/skillrecommend/rank_feedback.go`：RankFeedback 结构体 | Tools | P1 | ✅ | `go build ./internal/tools/...` 通过 |

### Phase 2 — Skill Intelligence 落地：Curator Agent 半自动进化

| ID | 任务 | 层 | 优先级 | 状态 | DoD |
|----|------|-----|--------|------|-----|
| 8.1 | 创建 `internal/biz/skill_evolution_suggestion_types.go`：领域模型 | Biz | P1 | ✅ | `go build ./internal/biz/...` 通过 |
| 8.2 | 创建 `internal/data/ent/schema/skill_evolution_suggestion.go`：Ent Schema | Data | P1 | ✅ | `go generate ./internal/data/ent/...` 无错误 |
| 8.3 | 扩展 `internal/biz/skill_intelligence_repo.go`：Reader/Writer 端口 | Biz | P1 | ✅ | `go build ./internal/biz/...` 通过 |
| 8.4 | 实现 Data 层 Repo：`internal/data/skill_evolution_suggestion.go` | Data | P1 | ✅ | `go build ./internal/data/...` 通过 |
| 8.5 | 修改 `internal/biz/skill_intelligence.go`：触发条件判定 + CreateSuggestion | Biz | P1 | ✅ | `go build ./internal/biz/...` 通过 |
| 8.6 | 创建 `internal/service/skill_curator.go`：Curator Agent 装配与 invoke | Service | P1 | ✅ | `go build ./internal/service/...` 通过 |
| 8.7 | 实现 Sandbox Runner 验证 | Service | P1 | ✅ | 隔离执行，不影响生产 |
| 8.8 | 定义进化建议 API：`api/kratos/skill_evolution_suggestion/v1/skill_evolution_suggestion.proto` + `internal/service/skill_evolution_suggestion.go` | Proto+Service | P1 | ✅ | `make api && go build ./...` 通过 |
| 8.9 | Skill 元数据扩展：新增 parent_version_id / evolution_reason / lifecycle_status | Data | P1 | ✅ | `go generate ./internal/data/ent/...` 无错误 |
| 8.10 | Wire DI 装配 | Wire | P1 | ✅ | `make wire && make build` 通过 |

### Phase 2 — Skill Intelligence 落地：前端

| ID | 任务 | 层 | 优先级 | 状态 | DoD |
|----|------|-----|--------|------|-----|
| 9.1 | 前端经验报告列表页 `web/src/pages/ExperienceReportListPage.vue`：调用 ListExperienceReports API | Web | P1 | ✅ | `pnpm lint && pnpm build` 通过 |
| 9.2 | 前端 Skill 进化审批 UI `web/src/pages/EvolutionSuggestionListPage.vue`：进化建议列表 + Approve/Reject + 触发 Curator | Web | P1 | ✅ | `pnpm lint && pnpm build` 通过 |
| 9.3 | 经验报告页优化（2026-08-23）：后端聚合字段 `success_count`/`failure_count`/`avg_score`（单条 GROUP BY 按 is_success 分组、按组计数加权重算总平均）+ KPI 概览条 + 失败标签水平条形图（中文化、unknown 弱化）+ 根因分析空态压缩 + 表格行内展开 RCA/修复建议/优化建议 | Data+Web | P1 | ✅ | `go test ./internal/biz ./internal/data` + `npx eslint` 通过；docker dev-up 后浏览器运行时验证通过 |

### Phase 3 — 自我进化闭环：预测性自愈

| ID | 任务 | 层 | 优先级 | 状态 | DoD |
|----|------|-----|--------|------|-----|
| 10.1 | 创建 `internal/biz/monitor/predictive_heal.go`：PredictiveHealUsecase | Biz | P2 | ✅ | `go build ./internal/biz/...` 通过 |
| 10.2 | 创建 `internal/cronrunner/jobs/predictive_heal.go`：Cron Job | Cron | P2 | ✅ | `go build ./internal/cronrunner/...` 通过 |
| 10.3 | 创建 `internal/biz/monitor/predictive_heal_test.go`：测试 | Biz | P2 | ✅ | `go test ./internal/biz/monitor/... -run TestPredictive -count=1` 绿色 |
| 10.4 | Wire DI 装配 | Wire | P2 | ✅ | `make wire && make build` 通过 |

### Phase 3 — 自我进化闭环：Skill 五阶段进化闭环

| ID | 任务 | 层 | 优先级 | 状态 | DoD |
|----|------|-----|--------|------|-----|
| 11.1 | 创建 `internal/biz/skill_evolution_loop.go`：五阶段流程 | Biz | P2 | ✅ | `go build ./internal/biz/...` 通过 |
| 11.2 | 实现 Gate 多维验证 | Biz | P2 | ✅ | `go build ./internal/biz/...` 通过 |
| 11.3 | 实现进化建议过期机制 | Biz | P2 | ✅ | `go build ./internal/biz/...` 通过 |
| 11.4 | 创建 `internal/biz/skill_evolution_loop_test.go`：测试 | Biz | P2 | ✅ | `go test ./internal/biz/... -run TestEvolutionLoop -count=1` 绿色 |

### Phase 3 — 自我进化闭环：知识库动态挖掘

| ID | 任务 | 层 | 优先级 | 状态 | DoD |
|----|------|-----|--------|------|-----|
| 12.1 | 创建 `internal/biz/monitor/pattern_mining.go`：PatternMiningUsecase | Biz | P2 | ✅ | `go build ./internal/biz/...` 通过 |
| 12.2 | 创建 `internal/cronrunner/jobs/pattern_mining.go`：Cron Job | Cron | P2 | ✅ | `go build ./internal/cronrunner/...` 通过 |
| 12.3 | 创建 `internal/biz/monitor/pattern_mining_test.go`：测试 | Biz | P2 | ✅ | `go test ./internal/biz/monitor/... -run TestPatternMining -count=1` 绿色 |
| 12.4 | Wire DI 装配 | Wire | P2 | ✅ | `make wire && make build` 通过 |

### 全量验证

| ID | 任务 | 优先级 | 状态 | DoD |
|----|------|--------|------|-----|
| 13.1 | 后端全量验证 | P0 | ✅ | `make api && make wire && make build && make test && make lint` |
| 13.2 | 前端全量验证 | P0 | ✅ | `cd web && pnpm lint && pnpm test && pnpm build` |

---

## 4. 每阶段验证标准

### Phase 1 验证标准

| 验证项 | 命令 | 预期 |
|--------|------|------|
| Biz 层编译 | `go build ./internal/biz/...` | 无错误 |
| Data 层编译 | `go build ./internal/data/...` | 无错误 |
| Wire 注入 | `make wire && go build ./cmd/admin` | 无错误 |
| 单元测试 | `go test ./internal/biz/monitor/... -count=1` | 绿色 |
| Data 层测试 | `go test ./internal/data/... -run TestFailurePattern -count=1` | 绿色 |
| 集成测试 | `go test -tags=integration ./internal/service/... -count=1` | 绿色 |
| 全量验证 | `make api && make wire && make build && make test && make lint` | 全绿 |

### Phase 2 验证标准

| 验证项 | 命令 | 预期 |
|--------|------|------|
| Proto 生成 | `make api` | 无错误 |
| Service 层编译 | `go build ./internal/service/...` | 无错误 |
| Tools 层测试 | `go test ./internal/tools/skillrecommend/... -count=1` | 绿色 |
| Wire 注入 | `make wire && make build` | 无错误 |
| 前端构建 | `cd web && pnpm lint && pnpm build` | 无错误 |
| 全量验证 | `make api && make wire && make build && make test && make lint` | 全绿 |

### Phase 3 验证标准

| 验证项 | 命令 | 预期 |
|--------|------|------|
| Biz 层测试 | `go test ./internal/biz/... -run TestPredictive -count=1` | 绿色 |
| Biz 层测试 | `go test ./internal/biz/... -run TestEvolutionLoop -count=1` | 绿色 |
| Biz 层测试 | `go test ./internal/biz/monitor/... -run TestPatternMining -count=1` | 绿色 |
| Wire 注入 | `make wire && make build` | 无错误 |
| 全量验证 | `make api && make wire && make build && make test && make lint` | 全绿 |
| 前端验证 | `cd web && pnpm lint && pnpm test && pnpm build` | 全绿 |

---

## 5. 回滚策略

### 5.1 通用原则

- 每个 Phase 独立，可单独回滚
- 所有变更均为增量（新增表/接口/Cron Job），不修改已有业务逻辑
- Cron Job 可通过删除注册禁用

### 5.2 各组件回滚方式

| 组件 | 回滚方式 | 影响 |
|------|----------|------|
| `RootCauseAnalyzer` 接口 | 删除 Wire 绑定，恢复直接使用 `RootCauseEngine` | 无，接口是增量变更 |
| `FailureReport` 结构体 | 删除相关文件 | 无，纯新增 |
| `failure_pattern` 表 | 删除 Ent Schema + Data 层 + Cron Job | 无，新表不影响现有表 |
| `failure_pattern_sync` Cron Job | 删除 Cron Job 注册 | 无 |
| Auto-Fix 改造 | 恢复 `auto-fix.yml` 到改造前版本 | 无 |
| Critic Agent | 设置 `ENABLE_CRITIC_AGENT=false` | 跳过语义检查 |
| `skill_intelligence_worker` Cron Job | 删除 Cron Job 注册 | 无 |
| `DynamicRankFactors` | 恢复使用静态 `RankFactors` | 无 |
| `HealthMetricsProvider` | 删除 Wire 绑定 | 无 |
| Curator Agent | 删除 Cron Job + Service | 无 |
| `PredictiveHealUsecase` | 删除 Cron Job + Wire 绑定 | 无 |
| Skill 进化闭环 | 删除相关 Biz 层代码 | 无 |
| `PatternMiningWorker` | 删除 Cron Job + Wire 绑定 | 无 |

### 5.3 数据库回滚

- `failure_pattern` 表：`DROP TABLE failure_pattern;`
- `skill_evolution_suggestions` 表：`DROP TABLE skill_evolution_suggestions;`
- `experience_reports` 扩展字段：Ent migration 回滚
- Skill 元数据扩展字段：Ent migration 回滚

---

## 6. 风险与缓解

| 风险 | 缓解措施 |
|------|----------|
| Phase 1 任务间存在依赖，需严格按序执行 | 按 1→2→3→4→5 顺序，每组内部可并行 |
| Ent Schema 变更需要 `go generate` | 每次新增 Schema 后立即执行 `go generate` |
| Wire DI 装配复杂度增加 | 每个 Phase 末尾统一做 Wire 验证 |
| 集成测试需要完整环境 | 使用 build tag `integration` 隔离 |
| 前端依赖后端 API | Phase 2 后端完成后才启动前端开发 |

---

*文档版本：2026-06-17 — Phase 1–3 全部落地，所有任务状态更新为 ✅；修正代码锚点与实际代码一致。*
