# Self-Iteration V3 — 平台自改进 开发计划

> 需求文档：[73-self-iteration-v3.md](./73-self-iteration-v3.md)
> 设计文档：[73-self-iteration-v3.design.md](./73-self-iteration-v3.design.md)
> **版本**：2026-07-29 | **状态**：📋 P1 待启动

---

## 1. 模块定位

V3 将平台自身全量代码作为进化对象，由平台内 Meta Team 执行「观察→归因→修补→验证→治理→应用→学习」七环闭环。复用 V2 全部基础设施，新增：2 张表、4 个 platform 触发器、运行状态机、RepoSandboxRunner、RiskClassifier、Applier、3 个 worker、（P5）控制台前端。

## 2. 依赖关系与代码锚点

| 依赖 | 锚点 | 关系 |
|------|------|------|
| 统一进化编排器 | `internal/biz/skill_evolution_unified.go` | 注册 platform 触发器（复用去重/冷却） |
| 统一状态机 | `internal/biz/evolution_suggestion_state_machine.go` | 建议生命周期复用 |
| FailureReport/Pattern KB | `internal/biz/monitor/failure_report*.go`、`internal/data/ent/schema/failure_pattern.go` | 触发信号源 + Learn 反哺 |
| RootCauseAnalyzer | `internal/biz/monitor/root_cause_analyzer.go` | Analyst 归因能力 |
| SandboxRunner | `internal/service/sandbox_runner.go` | 扩展模式参照（不改动） |
| 监控指标 | `internal/biz/monitor/`（18-monitor） | PerfBottleneckTrigger 数据源 |
| 评估体系 | `internal/service/evaluation.go`（33-evaluation） | EvalRegressionTrigger 数据源 |
| 后台 worker | `cmd/admin/workers.go` | 3 个新 worker 注册 |
| Team 编排 | `internal/biz/spirit_team_usecase.go`（53-team-graph-orchestration） | Meta Team 载体 |
| 审批 activity | guardrail approval + chat confirm | 高风险人工通道 |
| 热加载 | 58-prompt-governance reload 通道 | 配置/Prompt 类应用 |

## 3. 现状评估

- V2（60-self-iteration-v2）Phase 1-3 已全部落地 ✅，复用资产就绪
- 统一进化编排器支持 `RegisterTrigger` 动态注册，`UnifiedEvolutionSuggestion` 的 target/action 为字符串枚举，扩展 platform 枚举无需改表结构（需确认 CHECK 约束，见 T1.3）
- 无现存 platform 级自改进代码，全部为增量新建

## 4. 任务清单

### Phase 1 — 感知接入（P0）

**目标**：数据模型 + 4 触发器 + 编排器接入，「信号→建议→run(detected)」链路打通

| ID | 任务 | 层 | 状态 | DoD |
|----|------|----|------|-----|
| T1.1 | Ent Schema：`self_improvement_runs` + `patch_outcomes`（含 `entsql.Annotation{Table}`、索引）→ `go generate` | Data | ⏳ | 生成物提交，`go build ./internal/data/...` 通过 |
| T1.2 | DDL 迁移登记（表创建 SQL，幂等 IF NOT EXISTS；observing 部分索引）入 `ddl_migration_registry.go` | Data | ⏳ | 迁移版本号 YYYYMMDD，启动应用成功 |
| T1.3 | 确认/放宽 `unified_evolution_suggestions` 的 target_type/action_type CHECK 约束（若存在）以接纳 platform/patch_code 等新枚举 | Data | ⏳ | 插入 platform 行不报错 |
| T1.4 | biz 领域模型：`self_improvement_types.go`（Run/Outcome/枚举/JSON 子结构） | Biz | ⏳ | `go build ./internal/biz/...` 通过 |
| T1.5 | 运行状态机 `self_improvement_state_machine.go`（D3 迁移表，复用 GenericStateMachine）+ 全迁移表单测 | Biz | ⏳ | `go test ./internal/biz/ -run TestSelfImprovementRunSM -count=1` 绿 |
| T1.6 | biz 端口：RunReader/Writer + PatchOutcomeWriter（窄接口 + Stability 标注） | Biz | ⏳ | build 通过 |
| T1.7 | data Repo 实现 + CAS UpdateStatus + entErrToBizErr 翻译 + PG 集成测试 | Data | ⏳ | `go test -tags=integration ./internal/data/ -run TestSelfImprovement -count=1` 绿 |
| T1.8 | 信号源窄端口（D 4.3）+ 4 触发器实现 `EvolutionTrigger` + 阈值/去重单测（mock 端口） | Biz | ⏳ | `go test ./internal/biz/ -run TestPlatformTrigger -count=1` 绿 |
| T1.9 | 信号源端口的数据层适配（KB 聚类查询、monitor 指标查询、eval 基线查询、测试失败查询） | Data/Biz | ⏳ | 单测/集成测试绿 |
| T1.10 | 编排器注册 4 触发器 + `self_improve_observe` worker（扫描→CheckAndCreate→建 run） | Biz/Cmd | ⏳ | `make wire && go build ./cmd/admin` 通过 |
| T1.11 | 配置块 `self_improvement`（默认 enabled=false）+ 配置解析测试 | Cmd | ⏳ | build + 测试绿 |

**阶段验收**：`go build ./... && go test ./internal/biz/... ./internal/data/... -count=1` 全绿；手工造信号 → 库中出现 platform 建议 + run(detected)

### Phase 2 — 沙盒与补丁（P1）

| ID | 任务 | 层 | 状态 | DoD |
|----|------|----|------|-----|
| T2.1 | `biz.RepoSandbox` 端口 + GateKind/GateResult 类型 | Biz | ⏳ | build 通过 |
| T2.2 | `service.RepoSandboxRunner`：worktree 准备/清理 + ApplyDiff + G1-G3 执行（超时/输出截断/进程组杀绝） | Service | ⏳ | fixture 仓库集成测试绿 |
| T2.3 | 受影响包推导（diff 文件清单 → go 包/前端范围）+ 单测 | Biz | ⏳ | 表驱动测试绿 |
| T2.4 | 保护文件清单校验器（diff 路径解析 + glob 匹配，D9）+ 单测 | Biz | ⏳ | 命中/放行用例全绿 |
| T2.5 | Patcher 工具集（fs_read/fs_write/git_diff，worktree 作用域限制） | Tools | ⏳ | 工具单测绿 |
| T2.6 | diff 规模上限（500 行）与敏感信息检测（复用 V2 SEL-08 模式） | Biz | ⏳ | 单测绿 |

**阶段验收**：fixture 仓库上对测试补丁跑通 G1-G3；保护清单命中即拒

### Phase 3 — Meta Team 与治理（P1）

| ID | 任务 | 层 | 状态 | DoD |
|----|------|----|------|-----|
| T3.1 | Analyst/Patcher/Verifier/Critic 的 Agent 定义与结构化输出契约（D5 表） | Biz | ⏳ | prompt + schema 单测 |
| T3.2 | Meta Team 图编排（含 verify 失败回 patching 重试回路，attempts 上限 3） | Biz | ⏳ | 编排单测（mock LLM）绿 |
| T3.3 | Critic G4 接入（复用 V2 输出契约 is_safe/risk_level/concerns）+ 日配额 10 | Service | ⏳ | 配额/降级单测绿 |
| T3.4 | RiskClassifier（D6 规则矩阵 R1-R5）纯代码实现 + 全规则表驱动单测 | Biz | ⏳ | 单测全绿 |
| T3.5 | 治理路由：low→auto、medium→auto+notify、high→审批 activity（复用聊天审批） | Biz/Service | ⏳ | 端到端集成测试绿 |
| T3.6 | Meta Team 过程活动挂载（resolveParentActivityID 规范）+ 用户介入指令（暂停/跳过/回滚） | Biz | ⏳ | 事件树断言测试绿 |

**阶段验收**：端到端「信号→诊断→补丁→验证→审批单」httptest 可运行（mock LLM/git）

### Phase 4 — 应用与学习（P1）

| ID | 任务 | 层 | 状态 | DoD |
|----|------|----|------|-----|
| T4.1 | Applier：热加载通道（config/prompt）+ 代码合并通道（commit 标记 + ff 合并，冲突转人工） | Service | ⏳ | fixture 仓库集成测试绿 |
| T4.2 | Watchdog worker：observing 扫描 + 滑窗指标对比 + 自动 revert + 通知 | Cmd/Biz | ⏳ | 单测（mock 指标）绿 |
| T4.3 | 手动 close/rollback 入口（P4 暂经管理 API 内部路径，P5 落 Proto） | Service | ⏳ | 集成测试绿 |
| T4.4 | Outcome worker：终态归因 verdict + KB 负面样本 + 触发器自适应降频 | Cmd/Biz | ⏳ | 单测绿 |
| T4.5 | 观察窗并发上限 3 + 同核心路径串行队列 | Biz | ⏳ | 并发单测绿 |

**阶段验收**：端到端「应用→观察→指标退化→自动回滚→成效记录」可运行

### Phase 5 — 控制台与竞赛材料（P2）

| ID | 任务 | 层 | 状态 | DoD |
|----|------|----|------|-----|
| T5.1 | Proto `self_improvement/v1`（8 个 RPC，§七）+ `make api` | Proto | ⏳ | 生成物提交，build 通过 |
| T5.2 | Service 层 8 RPC（admin 鉴权、entErrToBizErr、分页） | Service | ⏳ | service 测试绿 |
| T5.3 | 前端控制台 4 组件 + feature store + i18n 双语言包 | Web | ⏳ | `pnpm lint && pnpm test && pnpm build` 绿 |
| T5.4 | 竞赛四件套更新（需求/概要/详细设计/实施进度对齐实现） | Docs | ⏳ | 评审维度映射完整 |

**阶段验收**：全量验证（后端 make api/wire/build/test/lint + 前端三件套）+ 竞赛材料同步

## 5. 改动文件清单（P1 范围）

**新增**：
- `internal/data/ent/schema/self_improvement_run.go`、`patch_outcome.go`（+ go generate 产物）
- `internal/data/sql/migrations/YYYYMMDD_self_improvement.sql`
- `internal/biz/self_improvement_types.go`、`self_improvement_state_machine.go`、`self_improvement_repo.go`（端口）、`self_improvement_triggers.go`
- `internal/data/self_improvement_repo.go`
- `internal/biz/*_test.go`、`internal/data/self_improvement_repo_test.go`

**修改**：
- `internal/data/ddl_migration_registry.go`（登记迁移）
- `cmd/admin/wire.go`（+ wire_gen.go regenerate）
- `cmd/admin/workers.go`（注册 observe worker）
- `internal/config/`（self_improvement 配置块）

## 6. 验收标准（总）

1. P1-P4 每阶段 DoD 全绿后才进入下一阶段（小步快跑，R5）
2. 全量验证：`make api && make wire && make build && make test && make lint` + `cd web && pnpm lint && pnpm test && pnpm build`
3. 运行时验证（R3）：启动 admin，手工造 error cluster 信号 → 观察建议/run 流转日志（`logs/aranea-pipeline.log`）与聊天界面活动树
4. 文档同步（DOC-SYNC）：每阶段完成更新本文件状态标记；Proto/Schema 变更同步 design.md

---

*文档版本：2026-07-29 — 初始版本。*
