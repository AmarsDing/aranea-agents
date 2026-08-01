# Self-Iteration V3 — 平台自改进 开发计划

> 需求文档：[73-self-iteration-v3.md](./73-self-iteration-v3.md)
> 设计文档：[73-self-iteration-v3.design.md](./73-self-iteration-v3.design.md)
> **版本**：2026-07-31 | **状态**：✅ Phase 1–5 全部完成（T1.1–T5.4）；遗留：运行时端到端冒烟（灰度开启后造信号验证）

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
| T1.1 | Ent Schema：`self_improvement_runs` + `patch_outcomes`（含 `entsql.Annotation{Table}`、索引）→ `go generate` | Data | ✅ | 生成物提交，`go build ./internal/data/...` 通过 |
| T1.2 | DDL 迁移登记（表创建 SQL，幂等 IF NOT EXISTS；observing 部分索引）入 `ddl_migration_registry.go` | Data | ✅ | 迁移版本号 YYYYMMDD，启动应用成功 |
| T1.3 | 确认/放宽 `unified_evolution_suggestions` 的 target_type/action_type CHECK 约束（若存在）以接纳 platform/patch_code 等新枚举 | Data | ✅ | 插入 platform 行不报错 |
| T1.4 | biz 领域模型：`self_improvement_types.go`（Run/Outcome/枚举/JSON 子结构） | Biz | ✅ | `go build ./internal/biz/...` 通过 |
| T1.5 | 运行状态机 `self_improvement_state_machine.go`（D3 迁移表，复用 GenericStateMachine）+ 全迁移表单测 | Biz | ✅ | `go test ./internal/biz/ -run TestSelfImprovementRunSM -count=1` 绿 |
| T1.6 | biz 端口：RunReader/Writer + PatchOutcomeWriter（窄接口 + Stability 标注） | Biz | ✅ | build 通过 |
| T1.7 | data Repo 实现 + CAS UpdateStatus + entErrToBizErr 翻译 + PG 集成测试 | Data | ✅ | `go test -tags=integration ./internal/data/ -run TestSelfImprovement -count=1` 绿 |
| T1.8 | 信号源窄端口（D 4.3）+ 4 触发器实现 `EvolutionTrigger` + 阈值/去重单测（mock 端口） | Biz | ✅ | `go test ./internal/biz/ -run TestPlatformTrigger -count=1` 绿 |
| T1.9 | 信号源端口的数据层适配（KB 聚类查询、monitor 指标查询、eval 基线查询、测试失败查询） | Data/Biz | ✅ | `TestSignalRepo_*`/`TestTestRunFileReader_*` 7 项集成测试绿（2026-07-30） |
| T1.10 | 编排器注册 4 触发器 + `self_improve_observe` worker（扫描→CheckAndCreate→建 run） | Biz/Cmd | ✅ | `make wire && go build ./cmd/admin` 通过（2026-07-30） |
| T1.11 | 配置块 `self_improvement`（默认 enabled=false）+ 配置解析测试 | Cmd | ✅ | `internal/conf` 测试绿（2026-07-30） |

**阶段验收（2026-07-30 通过）**：`go build ./...`、`go vet ./...`、`araneactl lint`、`fmtcheck` 全绿；`go test -race ./internal/biz/ ./internal/conf/ ./internal/cronrunner/...` 及 data 层 SI 集成测试（真实 PG）全绿。注：验证须在干净 GOCACHE 下进行——默认缓存曾因并行工作流导出数据不一致产生幻影编译错误。剩余手工验收项：造信号 → 库中出现 platform 建议 + run(detected)（待运行时验证）。

### Phase 2 — 沙盒与补丁（P1）

| ID | 任务 | 层 | 状态 | DoD |
|----|------|----|------|-----|
| T2.1 | `biz.RepoSandbox` 端口 + GateKind/GateResult 类型 | Biz | ✅ | build 通过（2026-07-30；类型在 T1.4 已落地为 `SandboxGateKind/SandboxGateResult`，本任务补端口 + 契约测试） |
| T2.2 | `service.RepoSandboxRunner`：worktree 准备/清理 + ApplyDiff + G1-G3 执行（超时/输出截断/进程组杀绝） | Service | ✅ | fixture 仓库集成测试 13 项全绿（2026-07-30） |
| T2.3 | 受影响包推导（diff 文件清单 → go 包/前端范围）+ 单测 | Biz | ✅ | 表驱动测试绿（2026-07-30） |
| T2.4 | 保护文件清单校验器（diff 路径解析 + glob 匹配，D9）+ 单测 | Biz | ✅ | 命中/放行用例全绿（2026-07-30，doublestar/v4 glob） |
| T2.5 | Patcher 工具集（fs_read/fs_write/git_diff，worktree 作用域限制） | Tools | ✅ | 工具单测 11 项全绿（2026-07-30） |
| T2.6 | diff 规模上限（500 行）与敏感信息检测（复用 V2 SEL-08 模式） | Biz | ✅ | 单测绿（2026-07-30，12 类 regex，命中只报类别不泄漏原文） |

**阶段验收（2026-07-30 通过）**：fixture 仓库上对测试补丁跑通 G1-G3（编译破坏补丁 G1 拒、测试失败补丁 G2 拒）；保护清单命中即拒（表驱动 14 用例）；`go build ./...`、`go vet`（改动包）、`araneactl lint`（0 violations）、`fmtcheck`（本任务文件）全绿；`go test -race ./internal/biz/ ./internal/conf/ ./internal/tools/patcherfs/` 及 `go test ./internal/service/`（跳过 2 个 models.dev 网络依赖用例）全绿。验证在独立 GOCACHE 下进行。

**P2 落地偏差（已记录，Phase 3 回收）**：
1. G3 Lint 以 `go vet` 为确定性下限；`golangci-lint run <pkgs>` 包级接线推迟 Phase 3（D4 原文保留）
2. web 侧 gate（pnpm build/test/lint）未进 `RepoSandboxRunner`，Phase 3 随 Meta Team 集成一并接入（`DeriveAffectedScopes` 已输出 web 范围标志）
3. 进程组杀绝沿用项目惯例 `exec.CommandContext`（Windows 无进程组语义；超时即 Kill 主进程，管道断开使子进程退出）

### Phase 3 — Meta Team 与治理（P1）

| ID | 任务 | 层 | 状态 | DoD |
|----|------|----|------|-----|
| T3.1 | Analyst/Patcher/Verifier/Critic 的 Agent 定义与结构化输出契约（D5 表） | Biz | ✅ | prompt + schema 单测绿（2026-07-30，`self_improvement_agents.go`：SIAgent* 标识 + 4 份系统提示词 + ParseDiagnosisJSON/ParsePatcherOutputJSON/ParseCriticReportJSON） |
| T3.2 | Meta Team 图编排（含 verify 失败回 patching 重试回路，attempts 上限 3） | Biz | ✅ | 编排单测（mock LLM）绿（2026-07-30，`self_improvement_pipeline.go`：Diagnose→Patch→Verify 重试回路→Govern，fail-fast 策略门禁不消耗沙盒 Gate） |
| T3.3 | Critic G4 接入（复用 V2 输出契约 is_safe/risk_level/concerns）+ 日配额 10 | Service | ✅ | 配额/降级单测绿（2026-07-30，`service/self_improvement_critic.go` 6 项测试：成功/非法 JSON/LLM 错误/配额耗尽降级/nil caller/diff 截断） |
| T3.4 | RiskClassifier（D6 规则矩阵 R1-R5）纯代码实现 + 全规则表驱动单测 | Biz | ✅ | 单测全绿（2026-07-30，`self_improvement_risk.go`，表驱动 16 用例覆盖 R1-R5 + 通道映射） |
| T3.5 | 治理路由：low→auto、medium→auto+notify、high→审批 activity（复用聊天审批） | Biz/Service | ✅ | 端到端集成测试绿（2026-07-30，`self_improvement_router.go`：auto/notify 日配额 5 超限转 approval；TestSIPipeline_EndToEnd_AutoChannel 信号→诊断→补丁→验证→治理→applying 全链路） |
| T3.6 | Meta Team 过程活动挂载（resolveParentActivityID 规范）+ 用户介入指令（暂停/跳过/回滚） | Biz | ✅ | 事件树断言测试绿（2026-07-30，`self_improvement_activity.go` 确定性 ID 两级树 + `self_improvement_control.go` SIControlPlane pause/skip_retry/rollback；13 项测试含重试 attempt 子树与 fail-fast 无 dangling 断言） |

**阶段验收（2026-07-30 通过）**：端到端「信号→诊断→补丁→验证→治理路由」biz 集成测试可运行（mock LLM/git）：`TestSIPipeline_EndToEnd_AutoChannel` + 路由 7 项 + 事件树/介入 13 项全绿；`go build ./internal/... ./pkg/...`、`go vet ./internal/biz/ ./internal/service/`、`gofmt`（本任务文件）、`go test -race ./internal/biz/`（SI 40 项）、`go test ./internal/service/ -run TestSICritic`（6 项）全绿。验证在独立 GOCACHE 下进行。注：`go build ./...` 与 `araneactl lint` 各有一处**并行会话**（memory canary）未完成的 wire 接线错误（`out.MemoryCanary undefined`）与 main.go 201 行超限，与本阶段改动无关，不越权修改对方集成点文件。

**P3 落地偏差（已记录，Phase 4 回收）**：
1. 审批/通知/活动挂载的 service 层适配器（`SIApprovalSink`→聊天审批、`SINotifier`→管理员通知、`SIActivitySink`→Activity/EventBus）为 wire 级接线，随 Phase 4 Applier/Watchdog 一并接入
2. pause 指令的恢复入口（resume worker 重驱动非终态 run）属 Phase 4
3. rollback 指令当前语义为 pre-apply 中止（→rejected）；applied/observing 的强制回滚属 Phase 4 Applier
4. P2 偏差 1（golangci-lint 包级接线）本阶段未回收，随 Phase 4 Gate 强化一并处理；G3 仍以 `go vet` 为确定性下限
5. P2 偏差 2（web 侧 gate 进 Runner）本阶段未回收；流水线已透传 `DeriveAffectedScopes` 的 go 包清单，web 范围标志的执行接线随 Phase 4 一并处理

### Phase 4 — 应用与学习（P1）

| ID | 任务 | 层 | 状态 | DoD |
|----|------|----|------|-----|
| T4.1 | Applier：热加载通道（config/prompt）+ 代码合并通道（commit 标记 + ff 合并，冲突转人工） | Service | ✅ | fixture 仓库集成测试绿 |
| T4.2 | Watchdog worker：observing 扫描 + 滑窗指标对比 + 自动 revert + 通知 | Cmd/Biz | ✅ | 单测（mock 指标）绿 |
| T4.3 | 手动 approve/reject/close/rollback 入口（P4 暂经管理 API 内部路径，P5 落 Proto） | Biz | ✅ | 单测（-race）绿 |
| T4.4 | Outcome worker：终态归因 verdict + KB 负面样本 + 触发器自适应降频 | Cmd/Biz | ✅ | 单测绿 |
| T4.5 | 观察窗并发上限 3 + 同核心路径串行队列 + Apply 编排 usecase + Router apply driver 挂钩 | Biz | ✅ | 并发单测（-race）绿 |
| T4.6 | W6 全链接线：21 个 wire provider（sandbox/applier/LLM stages/适配器/usecases）+ drive/watchdog/outcome 三 worker 注册 | Cmd | ✅ | `make wire && go build ./...` 绿，worker 冒烟测试绿 |

**阶段验收（2026-07-31 通过）**：W6 全链接线落地——21 个 wire provider（sandbox/applier/3 LLM stages/5 适配器/6 usecases/3 workers，全部 gated on `self_improvement.enabled`）+ drive/watchdog/outcome 三 worker 注册进 `startBackgroundWorkers`；端到端「应用→观察→指标退化→自动回滚→成效记录」链路经 usecase 级测试覆盖。验证：`make wire`、`go build ./...`、`go vet`（改动包）、`go test -race ./internal/biz/ ./internal/conf/ ./internal/cronrunner/...`、`go test ./internal/service/ -run TestSI` 全绿；gofmt 本会话文件干净。修复：watchdog/drive worker 冒烟测试竞态（async safego 扫描 vs 直接读共享计数 → stub 互斥访问器 + 轮询等待模式）。

**P3 偏差回收（W6 完成）**：
1. ✅ 审批/通知/活动适配器已接线（`SIMonitorApprovalSink`/`SIMonitorNotifier`/`SIMonitorActivitySink`，Monitor Events 通道）
2. ✅ pause 恢复入口已落地（drive worker 陈旧恢复：`SIStaleTimeout` 默认 30m，diagnosing/patching/verifying→detected 重驱动）
3. ✅ applied/observing 强制回滚已落地（Watchdog `ScanOnce` + Admin `Rollback`，经 `SIApplier.Rollback`）
4. P2 偏差 1（golangci-lint 包级接线）未回收：G3 仍以 `go vet` 为确定性下限
5. P2 偏差 2（web 侧 gate 进 Runner）未回收：`DeriveAffectedScopes` web 标志已透传，执行接线待后续

**W6 接线备注**：
- `provideSelfImprovementAdminUsecase` 函数已备，但未注册进 wire.Build——T4.3 内部路径暂无消费者，Wire 不允许 unused provider；P5 Proto/控制台落地时注册并暴露 RPC
- LLM stages 复用平台 `DefaultRefineLLM`（`SystemSettingUsecase.GetRefineLLM`），未配置时 stage=nil，pipeline 报「stages not wired」明确错误（不 panic）
- sandbox repoRoot = `sandbox.repo_root` 配置（空 → 进程工作目录 `os.Getwd()`）：未配置时启用自改进的 admin 必须从仓库根启动（灰度特性，默认 disabled）
- T4.3 去重（2026-07-31）：实施期曾并存 `self_improvement_operator.go` 与 `self_improvement_admin.go` 两份同职责用例；保留签名对齐 design §7 的 Admin 版本并删除 Operator 版本（含各自测试重写/清理）

### Phase 5 — 控制台与竞赛材料（P2）

| ID | 任务 | 层 | 状态 | DoD |
|----|------|----|------|-----|
| T5.1 | Proto `self_improvement/v1`（9 个 RPC，§七）+ `make api` | Proto | ✅ | 生成物提交，build 通过 |
| T5.2 | Console usecase（List/Get/Stats/Rules）+ Service 层 9 RPC（admin 鉴权、operator 身份、分页）+ SIRiskRules 可配置化（SystemSetting 持久化 + Pipeline/Router 消费）+ wire 注册 | Biz/Service/Data/Cmd | ✅ | biz/service/data 测试绿，`make wire && go build ./cmd/admin` 绿 |
| T5.3 | 前端控制台 4 组件 + feature store + i18n 双语言包 | Web | ✅ | `pnpm lint && pnpm test && pnpm build` 绿（2026-07-31；组件落 `components/self-improvement/`，RiskRulesDialog 未做——API 就绪 UI 待补，diff 纯文本渲染） |
| T5.4 | 竞赛四件套更新（需求/概要/详细设计/实施进度对齐实现） | Docs | ✅ | 评审维度映射完整（2026-07-31，`competition/15-平台自改进/`：实施进度 v2.0 全量重写 + 概要 v1.1 + 详设 v1.1 + 需求 v1.1 版本对齐，8 项落地偏差全量记录） |

**T5.2 落地备注（2026-07-31）**：
- `provideSelfImprovementAdminUsecase` 已注册进 wire.Build（W6 备注回收），`SelfImprovementService` 经 `NewSelfImprovementService` 注入并注册 HTTP/gRPC（feature disabled 时 nil-guard 不注册）
- 风险规则链：`SIRiskRules`（biz，0/空继承默认）→ `si_risk_rule_repo.go`（Raw SQL 读写 `system_settings`，迁移 20261121 四列）→ `provideSIRiskRules` 启动加载（失败回退 config/代码默认）→ `NewSIRiskClassifierWithRules`（Pipeline Govern 阶段）+ 治理路由日配额（DB > config > 默认）
- 服务端校验：阈值 ≥0、low ≤ medium（均非零时）、glob 非空且过 doublestar.ValidatePattern；GetRiskRules 返回 configured（原始）+ effective（归一化）双视图

**阶段验收（2026-07-31 通过）**：后端 `make api && make wire && make build` 绿；前端 `pnpm lint && pnpm test && pnpm build` 绿（store 单测覆盖）；竞赛四件套已同步实现口径。

## 5. 改动文件清单（P1 范围）

**新增（Phase 1 已落地）**：
- `internal/data/ent/schema/self_improvement_run.go`、`patch_outcome.go`（+ go generate 产物）
- `internal/data/sql/migrations/20261120_self_improvement_observing_index.sql`（原 20261115，与数据迁移 monitor_trace_interrupted_backfill 版本碰撞，2026-07-30 重编号）
- `internal/biz/self_improvement_types.go`、`self_improvement_state_machine.go`、`self_improvement_repo.go`（端口）、`self_improvement_triggers.go`、`self_improvement_observe.go`
- `internal/data/self_improvement_repo.go`、`self_improvement_signals.go`、`self_improvement_testrun_reader.go`
- `internal/cronrunner/jobs/self_improve_observe_worker.go`
- `internal/conf/self_improvement.go`
- 测试：`internal/biz/self_improvement_*_test.go`、`internal/data/self_improvement_*_test.go`、`internal/conf/self_improvement_test.go`、`internal/cronrunner/jobs/self_improve_observe_worker_test.go`

**修改（Phase 1 已落地）**：
- `internal/data/ddl_migration_registry.go`（登记迁移）
- `cmd/admin/wire.go`（+ wire_gen.go regenerate：触发器注册、observe usecase/worker provider）
- `cmd/admin/workers.go`（注册 observe worker，`goAfterReady("self_improve_observe")`）
- `internal/conf/conf.proto` + `conf.pb.go`（self_improvement 配置块）
- `configs/config.yaml`（注释示例配置块，默认 enabled=false）

**新增（Phase 2 已落地）**：
- `internal/biz/self_improvement_patch.go`（diff 解析/受影响包推导/保护清单/规模上限/敏感信息检测）+ `_test.go`（含 T2.1 端口契约测试）
- `internal/service/repo_sandbox_runner.go`（worktree 沙盒 + ApplyDiff + G1-G3）+ `_test.go`（fixture 仓库集成测试）
- `internal/tools/patcherfs/patcherfs.go`（patcher_fs_read/patcher_fs_write/patcher_git_diff）+ `_test.go`

**修改（Phase 2 已落地）**：
- `internal/biz/self_improvement_repo.go`（新增 `RepoSandbox` 端口，T2.1）
- `internal/conf/conf.proto` + `conf.pb.go`（Patch/Sandbox 子块，`make config` 重生成）
- `internal/conf/self_improvement.go` + `_test.go`（SIMaxDiffLines/SIDailyAutoApplyQuota/SIMaxAttempts/SIGateTimeouts/SIWorktreeRoot 访问器）
- `configs/config.yaml`（patch/sandbox 注释示例）
- `.gitignore`（`/.aranea-self-improve/`）
- `go.mod`/`go.sum`（`doublestar/v4` 转直接依赖）

**新增（Phase 3 已落地）**：
- `internal/biz/self_improvement_agents.go`（SIAgent* 标识、4 份系统提示词、ParseDiagnosisJSON/ParsePatcherOutputJSON/ParseCriticReportJSON，T3.1）+ `_test.go`
- `internal/biz/self_improvement_pipeline.go`（Meta Team 编排 + 重试回路 + fail-fast 门禁 + 活动挂载/用户介入集成，T3.2/T3.6）+ `_test.go`
- `internal/service/self_improvement_critic.go`（Critic G4 LLM 接入 + 日配额 10，T3.3）+ `_test.go`
- `internal/biz/self_improvement_risk.go`（RiskClassifier D6 R1-R5 + 通道映射，T3.4）+ `_test.go`
- `internal/biz/self_improvement_router.go`（SIGovernanceRouter + SINotifier/SIApprovalSink 端口 + auto 日配额 5 超限转 approval，T3.5）+ `_test.go`
- `internal/biz/self_improvement_activity.go`（确定性活动 ID + SIActivityRecord + SIActivitySink 端口，T3.6）+ `_test.go`
- `internal/biz/self_improvement_control.go`（SIControlCommand/SIControlPlane/ErrSIRunPaused，T3.6）+ `_test.go`

**新增（Phase 4 已落地）**：
- `internal/service/self_improvement_applier.go`（SIRepoApplier：ApplyHotReload 快照通道 + ApplyCodeMerge 代码 ff 合并 + Rollback revert/快照恢复，T4.1）+ `_test.go`（fixture 仓库集成测试 8 例）
- `internal/biz/self_improvement_apply.go`（SelfImprovementApplyUsecase：kind 路由 + 冲突 escalate 转人工 + 观察窗并发上限 3 + 核心路径串行 PromoteEligible，T4.5）+ `_test.go`（12 例，含 -race 并发测试）
- `internal/biz/self_improvement_watchdog.go`（SelfImprovementWatchdogUsecase：基线采集 + 到期滑窗对比 + 自动 revert + 通知，T4.2）+ `_test.go`
- `internal/biz/self_improvement_admin.go`（SelfImprovementAdminUsecase：手动 approve/reject/close/rollback，签名对齐 design §7——Approve 带 reason、Reject reason 必填，T4.3）+ `_test.go`（10 例，-race 绿）
- `internal/biz/self_improvement_outcome.go`（SelfImprovementOutcomeUsecase：verdict 归因 + KB 负面样本 + 触发器降频，T4.4）+ `_test.go`
- `internal/biz/self_improvement_drive.go`（SelfImprovementDriveUsecase：全链驱动——detected 异步 pipeline / 陈旧中途态 recover / awaiting_governance 路由去重 / applying 重驱动 / applied 晋升，Phase 4 增补）+ `_test.go`
- `internal/service/self_improvement_adapters.go`（W6 端口适配器：SIMonitorNotifier/SIMonitorApprovalSink/SIMonitorActivitySink/SIKBNegativePatternSink/SIOrchestratorFeedbackSink）+ `_test.go`
- `internal/service/self_improvement_meta_team.go`（W6 LLM 阶段：SIAnalystAgent/SIPatcherAgent，复用 DefaultRefineLLM）
- `internal/cronrunner/jobs/self_improve_watchdog_worker.go`（观察窗巡检调度，T4.2）+ `_test.go`
- `internal/cronrunner/jobs/self_improve_outcome_worker.go`（终态归因调度，T4.4）+ `_test.go`
- `internal/cronrunner/jobs/self_improve_drive_worker.go`（全链驱动调度，Phase 4 增补）+ `_test.go`

**修改（Phase 4 已落地）**：
- `internal/biz/self_improvement_repo.go`（新增 `ErrSIMergeConflict` 哨兵 + `ListTerminalPendingOutcome`/`ListRecentOutcomesByTrigger` 端口方法，T4.1/T4.4）
- `internal/biz/self_improvement_state_machine.go`（新增 `RunEventApplyEscalate`：applying→awaiting_governance 冲突转人工，T4.5；`RunEventRecover`：diagnosing/patching/verifying→detected 陈旧恢复，Drive）
- `internal/biz/self_improvement_risk.go`（导出 `SICoreAreas`/`SICoreAreasIntersect` 核心路径区域判定，T4.5）
- `internal/biz/self_improvement_router.go`（新增 `SIApplyDriver` 端口 + auto/notify 迁移后驱动挂钩，T4.5）
- `internal/biz/self_improvement_pipeline_test.go`（siRunStore fake 并发安全 + others 列表/CAS 支持，T4.5；stub 补 ListTerminalPendingOutcome 方法）
- `internal/conf/conf.proto` + `conf.pb.go`（ObserveWindow/watchdog_interval/outcome_interval/drive_interval/stale_timeout + `sandbox.repo_root`，T4.2/T4.4/W6）
- `internal/conf/self_improvement.go` + `_test.go`（SIObserveWindow*/SIWatchdogInterval/SIOutcomeInterval/SIDriveInterval/SIStaleTimeout/SIRepoRoot 访问器）
- `internal/data/self_improvement_repo.go` + `_test.go`（ListTerminalPendingOutcome/ListRecentOutcomesByTrigger PG 集成测试）
- `internal/data/self_improvement_signals.go` + `_test.go`（SIMetricsReader.Snapshot 滑窗指标快照，T4.2）
- `internal/biz/self_improvement_observe_test.go`（stub 补 ListTerminalPendingOutcome 方法）
- `internal/service/repo_sandbox_runner_test.go`（fixture 关闭 autocrlf，Windows 下 patch 内容不被转换）
- `cmd/admin/wire.go` + `wire_gen.go`（W6：21 个 provider 接线 + `make wire` 重生成；`provideSelfImprovementAdminUsecase` 备好未注册，P5 消费）
- `cmd/admin/workers.go`（注册 self_improve_drive/watchdog/outcome 三 worker，`goAfterReady`）
- `configs/config.yaml`（self_improvement 注释示例补齐 observe_window/watchdog_interval/outcome_interval/drive_interval/stale_timeout/sandbox.repo_root）

## 6. 验收标准（总）

1. P1-P4 每阶段 DoD 全绿后才进入下一阶段（小步快跑，R5）
2. 全量验证：`make api && make wire && make build && make test && make lint` + `cd web && pnpm lint && pnpm test && pnpm build`
3. 运行时验证（R3）：启动 admin，手工造 error cluster 信号 → 观察建议/run 流转日志（`logs/aranea-pipeline.log`）与聊天界面活动树
4. 文档同步（DOC-SYNC）：每阶段完成更新本文件状态标记；Proto/Schema 变更同步 design.md

---

*文档版本：2026-07-31 — Phase 1–5（T1.1–T5.4）全部完成并验证；竞赛四件套同步实现口径（8 项落地偏差记录于实施进度文档 v2.0）。遗留：运行时端到端冒烟待灰度开启后执行。*
*历史版本：2026-07-31 — Phase 1–4（T1.1–T4.6）+ W6 全链接线完成并验证，状态标记与文件清单同步；design.md §五/§6 已同步实际接线（24 providers + drive worker + 配置块）。*
*历史版本：2026-07-30 — Phase 1（T1.1–T1.11）、Phase 2（T2.1–T2.6）、Phase 3（T3.1–T3.6）完成并验证。*
