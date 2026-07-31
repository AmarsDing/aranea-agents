# Self-Iteration V3 — 平台自改进（Meta 级自我迭代）设计文档

> 需求文档：[73-self-iteration-v3.md](./73-self-iteration-v3.md)
> 开发计划：[73-self-iteration-v3.development.md](./73-self-iteration-v3.development.md)
> 前置设计：[60-self-iteration-v2.design.md](./60-self-iteration-v2.design.md)

---

## 一、架构全景图

### 1.1 七环闭环架构

```
┌──────────────────────────────────────────────────────────────────────────┐
│ ① Observe 感知（biz，EvolutionTrigger 插件，复用统一编排器）                  │
│   ErrorClusterTrigger / PerfBottleneckTrigger / EvalRegressionTrigger /   │
│   TestFailureTrigger → UnifiedEvolutionSuggestion(target_type=platform)   │
│        ↓                                                                  │
│ ② Diagnose 归因（Meta Team · Analyst Agent）                               │
│   FailureReport + trace/指标快照 + 源码只读工具 → Diagnosis(JSON)           │
│        ↓                                                                  │
│ ③ Patch 修补（Meta Team · Patcher Agent）                                  │
│   git worktree(self-improve/<run_id>) → unified diff；保护文件清单拦截      │
│        ↓                                                                  │
│ ④ Verify 验证（Meta Team · Verifier Agent + RepoSandboxRunner）            │
│   G1 编译 → G2 受影响包单测 → G3 Lint → G4 Critic 语义审查 → G5 Eval 基线   │
│        ↓                                                                  │
│ ⑤ Govern 治理（RiskClassifier，纯代码）                                     │
│   low=自动应用 │ medium=自动应用+通知 │ high=人工审批（聊天 activity/API）   │
│        ↓                                                                  │
│ ⑥ Apply 应用（Applier）                                                    │
│   配置/Prompt=热加载 │ 代码=git 合并+重启生效 → 观察窗 24h → 退化自动 revert │
│        ↓                                                                  │
│ ⑦ Learn 学习（PatchOutcome 归因 + FailurePattern KB 反哺①）                 │
└──────────────────────────────────────────────────────────────────────────┘
```

### 1.2 与 V2 资产复用关系

| V2 资产 | 位置 | V3 复用方式 |
|---------|------|------------|
| FailureReport | `internal/biz/monitor/failure_report.go` | Observe/Diagnose 的结构化错误输入 |
| FailurePattern KB | `internal/data/ent/schema/failure_pattern.go` | 触发信号源 + Learn 负面样本写入（source=self_improvement） |
| RootCauseAnalyzer | `internal/biz/monitor/root_cause_analyzer.go` | Analyst Agent 的归因能力接口 |
| Critic Agent | `.auto-fix/` + V2 CI | G4 语义回归审查（改造为平台内 Agent，复用输出契约） |
| SkillEvolutionOrchestrator | `internal/biz/skill_evolution_unified.go` | 注册 platform 触发器；复用 pending 去重/冷却/状态机 |
| UnifiedEvolutionStateMachine | `internal/biz/evolution_suggestion_state_machine.go` | 建议生命周期（pending→approved→applied→rolled_back） |
| SandboxRunner | `internal/service/sandbox_runner.go` | 扩展为 RepoSandboxRunner（worktree 级 G1-G3） |
| 审批 activity | guardrail approval + chat confirm | 高风险人工审批通道 |

### 1.3 现有代码基线（V3 直接依赖）

| 依赖 | 锚点 | 说明 |
|------|------|------|
| EvolutionTrigger 接口 | [skill_evolution_unified.go:220](../../internal/biz/skill_evolution_unified.go#L220) | 四触发器实现此接口 |
| 编排器注册 | `SkillEvolutionOrchestrator.RegisterTrigger` | 线程安全，启动期注册 |
| 监控指标 | `internal/biz/monitor/`（18-monitor） | PerfBottleneckTrigger 数据源 |
| 评估体系 | `internal/service/evaluation.go`（33-evaluation） | EvalRegressionTrigger/G5 数据源 |
| 后台 worker | `cmd/admin/workers.go` | 感知扫描 worker 注册模式 |
| 事件/活动 | `internal/event/`（34-event-system） | Meta Team 过程可视 |

---

## 二、核心设计决策

### D1：建议载体复用统一进化建议（不建新建议表）

`UnifiedEvolutionSuggestion` 已具备 target/action/trigger/status/priority/draft/metadata 完整字段与去重、冷却、状态机。V3 仅扩展枚举：

- `EvolutionTargetType` 新增 `platform`
- `EvolutionActionType` 新增 `patch_code` / `tune_config` / `patch_prompt`

理由：复用成熟的 pending 去重（`HasPendingForTarget`）、per-action-type 冷却（`EvoTriggerCooldownHours`）、审批状态机，避免另起一套建议生命周期。**闭环执行状态**（diagnosing/patching/verifying/...）不进建议表，由新表 `self_improvement_runs` 承载（见 D3）。

### D2：四类触发器，信号源各司其职

| 触发器 | TargetType/ActionType | 信号源 | 触发条件（默认，可配置） |
|--------|----------------------|--------|--------------------------|
| ErrorClusterTrigger | platform / patch_code | FailurePattern KB + 运行时错误日志 | 同 error_code+相似堆栈 7d ≥5 次 |
| PerfBottleneckTrigger | platform / patch_code 或 tune_config | monitor trace/usage 指标 | 步骤 P95 超基线 2× 或 token 超基线 50% |
| EvalRegressionTrigger | platform / patch_code 或 patch_prompt | 33-evaluation 基线 | 基准分数退化 >10% |
| TestFailureTrigger | platform / patch_code | 测试运行结果（cron 全量测试） | 同一测试连续 2 轮失败 |

所有触发器实现 `EvolutionTrigger` 接口，Check() 产出 `UnifiedEvolutionSuggestion`，证据快照写入 metadata（JSON）。触发器只感知、不行动——行动由 Meta Team 执行。

### D3：闭环执行状态机独立于建议状态机（AS-FSM-01）

建议状态机（复用）管「批不批」；运行状态机（新增 `SelfImprovementRunStateMachine`）管「做到哪一步」：

```
detected → diagnosing → patching → verifying → awaiting_governance
    → applying → applied → observing → closed        （正常路径）
    ↘ verify_failed（重试 ≤3 次回 patching） ↘ rolled_back（终态）
    ↘ rejected / failed（终态）
```

| 状态 | 进入事件 | 说明 |
|------|----------|------|
| detected | create | 建议创建即产生 run（1:1 绑定 suggestion_id） |
| diagnosing | diagnose | Analyst 归因中 |
| patching | patch | Patcher 生成补丁中（verify 失败重试也回此态） |
| verifying | verify | G1-G5 执行中 |
| awaiting_governance | verify_pass | 等待风险分级路由/人工审批 |
| applying | apply | 应用执行中 |
| applied | apply_done | 已应用（代码类待重启） |
| observing | observe | 观察窗计时中 |
| closed | close / auto_close | 观察窗通过，终态 |
| verify_failed | verify_fail（重试耗尽） | 终态 |
| rolled_back | rollback | 终态 |
| rejected | reject | 审批拒绝，终态 |
| failed | error | 异常终止，终态 |

>3 状态实体必须有显式状态机：文件 `internal/biz/self_improvement_state_machine.go`，复用 `shared.GenericStateMachine`。

### D4：git worktree 沙盒（RepoSandboxRunner）

V2 的 SandboxRunner 校验的是 SKILL.md 草案；V3 需要对整个仓库打补丁并执行 build/test/lint。设计 `RepoSandboxRunner`（service 层，实现 biz 端口 `biz.RepoSandbox`）：

```
Apply(workspaceDir, diff) → {ok, err}
RunGate(ctx, workspaceDir, gate, scope) → GateResult{gate, passed, output, duration}
```

- **worktree 隔离**：`git worktree add .aranea-self-improve/<run_id> -b self-improve/<run_id> <base_ref>`；目录纳入 `.gitignore`；run 结束清理（`git worktree remove --force`）
- **G1 编译**：worktree 内 `go build ./...`；前端补丁时 `pnpm build`（web 子目录，pnpm install 复用主仓 node_modules 硬链或独立 install——P2 先独立 install，正确性优先）
- **G2 单测**：由 diff 文件清单推导受影响包（`internal/biz/foo.go` → `./internal/biz/...`），`go test <pkgs> -count=1`；前端 `pnpm test`
- **G3 Lint**：`golangci-lint run <pkgs>`（全量 make lint 太慢，P2 先包级）+ `pnpm lint`
- **超时与资源**：每 Gate 独立超时（G1 5min / G2 10min / G3 5min），子进程组杀绝，输出截断 64KB 入报告
- **生产隔离**：沙盒只读配置副本，测试走独立 PG schema（testhelper 模式）；禁止访问生产外部服务（环境变量白名单注入）

> P2 落地注记（2026-07-30）：G3 以 `go vet <pkgs>` 为确定性下限（golangci-lint 包级接线推迟 Phase 3）；web 侧 gate（pnpm）未进 Runner，`DeriveAffectedScopes` 已输出 web 范围标志，Phase 3 随 Meta Team 集成接入；进程组杀绝沿用项目惯例 `exec.CommandContext`（Windows 无进程组语义）。diff 解析/受影响包推导/保护清单（doublestar glob）/500 行规模上限/SEL-08 敏感信息检测实现于 `internal/biz/self_improvement_patch.go`；Patcher 工具集（patcher_fs_read/patcher_fs_write/patcher_git_diff，worktree 作用域限定）实现于 `internal/tools/patcherfs/`。

### D5：Meta Team 编排映射（竞赛 AgentTeams 基点）

Meta Team 定义为平台内置 SpiritTeam（复用 53-team-graph-orchestration），图编排：

```
Observer(代码节点,非LLM) → Analyst(LLM) → Patcher(LLM) → Verifier(代码节点执行Gate+LLM总结)
    → Critic(LLM) → Governor(代码节点,非LLM) → [人工审批点] → Applier(代码节点)
```

| 成员 | 类型 | 输入 | 输出（结构化 JSON） |
|------|------|------|---------------------|
| Observer | 代码节点 | 4 触发器建议 | DiagnosisTask（建议+证据快照打包） |
| Analyst | LLM Agent | DiagnosisTask + 源码只读工具 | Diagnosis{root_cause,affected_files,impact_scope,fix_strategy,confidence} |
| Patcher | LLM Agent | Diagnosis + worktree 读写工具 | Patch{diff,files,additions,deletions,kind} |
| Verifier | 代码+LLM | Patch + RepoSandboxRunner | VerificationReport{gates:[{gate,passed,output}]} |
| Critic | LLM Agent | diff + 相关源码 | CriticReport{is_safe,risk_level,concerns,suggestion} |
| Governor | 代码节点 | Patch+CriticReport+VerificationReport | GovernanceDecision{risk_level,channel(auto/notify/approval),rule_hits} |
| Applier | 代码节点 | GovernanceDecision+Patch | ApplyResult{commit_sha,reload_mode,rollback_pointer} |

- 上下文传递：全部结构化 JSON 经 TeamStage 事件树挂载（遵循既有 team 活动挂载规范，ParentActivityID 用 resolveParentActivityID）
- 重试回路：Verifier 失败 → 带 Gate 输出回 Patcher（≤3 次，计数入 run.attempts）
- 用户介入：会话指令（暂停/跳过重试/强制回滚）经 chat 命令解析入 run 控制通道

### D6：风险分级规则（RiskClassifier，纯代码）

输入：Patch（文件清单+行数+kind）+ CriticReport。规则表（默认，可配置覆盖）：

| 规则 | 条件 | 等级 |
|------|------|------|
| R1 | kind ∈ {config, prompt, docs, i18n, test} 且单文件且 diff ≤100 行 | low |
| R2 | 单文件业务代码且非核心路径且 diff ≤300 行 | medium |
| R3 | 文件数 >1，或命中核心路径（`internal/service/chat*`、`internal/agent/`、`internal/data/ent/schema/`、proto、DDL 迁移新增），或 diff >300 行 | high |
| R4 | CriticReport.risk_level=high 或 is_safe=false | high（强制升级） |
| R5 | 触发保护文件清单 | reject（不进入分级，直接拒绝） |

通道映射：low→auto、medium→auto+notify、high→approval。每日 auto 配额 5 次，超限一律转 approval（复用 V2 日配额计数模式）。

> P3 落地注记（2026-07-30）：Meta Team 编排实现于 `internal/biz/self_improvement_pipeline.go`（D5：Diagnose→Patch→Verify 重试回路→Govern，fail-fast 策略门禁在 Verify 前不消耗沙盒 Gate）；Agent 标识/提示词/结构化输出解析于 `self_improvement_agents.go`；RiskClassifier（R1-R5）于 `self_improvement_risk.go`；Critic G4 + 日配额 10 于 `internal/service/self_improvement_critic.go`；治理路由 `SIGovernanceRouter`（auto/notify 配额 5 超限转 approval、SINotifier/SIApprovalSink 端口）于 `self_improvement_router.go`；过程活动挂载（确定性 ID 两级树 `si-run:<id>` → 阶段子节点，patching/verifying 按 attempt 分叉）与用户介入控制面（pause→ErrSIRunPaused 非终态驻留、skip_retry、rollback=pre-apply 中止→rejected）于 `self_improvement_activity.go`/`self_improvement_control.go`。审批/通知/活动的 service 层适配器与 pause 恢复入口属 wire 级接线，推迟 Phase 4（见 development.md P3 落地偏差）。

### D7：应用与观察窗（Applier + Watchdog）

- **配置/Prompt 类**：复用 58-prompt-governance 的热加载通道（RuntimeProfile reload），应用即生效，回滚=回退快照
- **代码类**：worktree 内 `git commit`（message 含 `self-improvement: true` + run_id 尾注）→ 合并主分支（fast-forward 优先，冲突则转人工）→ 状态 applied（待重启生效）；**不做运行中热替换**
- **观察窗**：applied → observing，Watchdog（cron，5min）对比应用前后各 1h 滑窗指标：错误率 +50% 或 P95 +30%（可配置）→ 自动 `git revert` + 热加载回退 → rolled_back + 管理员通知
- **手动控制**：管理员可 close（提前确认有效）或 rollback（立即回滚）

### D8：成效学习（Learn）

`patch_outcomes` 记录闭环终态归因：

- verdict 判定：closed→effective；rolled_back→regressed；verify_failed/rejected/failed→neutral（不计入有效样本）
- regressed 补丁的 pattern（error_code + 文件特征哈希）写 FailurePattern KB（source=self_improvement、is_active=true、负面标记 negative=true）
- 触发器自适应：同一 trigger_source 连续 3 次 neutral/regressed → 该触发器冷却期 ×2（内存态 + 配置持久化）

### D9：保护文件清单（与 V2 AFE-06 对齐并扩展）

```
.github/workflows/**  Makefile  go.mod  go.sum
api/kratos/**/*.proto（已有 message 禁止改；允许新增 message/service——diff 级校验，P3 实现）
internal/data/sql/migrations/**（历史文件禁改；允许新增迁移文件）
cmd/admin/wire_gen.go（禁手改；允许 wire regenerate 产物）
internal/data/ent/*.go（Ent 生成物，禁手改；允许 go generate 产物）
```

校验时机：Patcher 产出 diff 后、进入 Verify 前（fail-fast）。实现：解析 unified diff 的 `+++ b/<path>` 清单逐条匹配 glob。

### D10：成本控制

| 项 | 配额 | 说明 |
|----|------|------|
| Critic | 10 次/日 | 复用 V2 配额机制 |
| Patcher | 20 次/日 | 同模式 |
| 自动应用 | 5 次/日 | 超限转人工 |
| 观察窗并发 | ≤3 个 observing | 防多补丁指标互相污染归因 |

---

## 三、数据模型设计

### 3.1 SelfImprovementRun（Ent Schema，新增表 `self_improvement_runs`)

| 字段 | 类型 | 说明 |
|------|------|------|
| id | string (UUID) | 主键，`newAgentCatalogID()` |
| suggestion_id | string，唯一 | 1:1 绑定 UnifiedEvolutionSuggestion |
| status | string(24) | 运行状态机状态（D3） |
| trigger_source | string(32) | error_cluster/perf_bottleneck/eval_regression/test_failure |
| patch_kind | string(16) | code/config/prompt/docs/test，空=未产出 |
| risk_level | string(8) | low/medium/high，空=未分级 |
| base_ref | string(64) | worktree 基准 commit |
| branch | string(128) | self-improve/<run_id> |
| worktree_path | string(256) | 沙盒目录（清理后保留路径仅审计） |
| diff | text | unified diff 全文 |
| diff_stats | JSON | {files,additions,deletions} |
| diagnosis | JSON | Analyst 输出（D5） |
| verification_report | JSON | 五级 Gate 结果数组 |
| critic_report | JSON | is_safe/risk_level/concerns/suggestion |
| governance | JSON | 分级决策+命中规则+通道 |
| attempts | int，默认 0 | 补丁-验证重试计数（上限 3） |
| approved_by | string，可空 | 审批人（高风险） |
| applied_commit | string(64)，可空 | 应用后的合并 commit |
| rollback_pointer | string(64)，可空 | revert 目标/热加载快照 ID |
| observe_until | time，可空 | 观察窗截止 |
| closed_reason | string(64)，可空 | 终态原因 |
| metadata | JSON | 扩展证据（trace IDs、指标快照引用） |
| created_at / updated_at | time | 审计 |

索引：`suggestion_id`（唯一）、`(status)`、`(trigger_source, created_at)`、`(observe_until)`（Watchdog 扫描，部分索引 WHERE status='observing' 由迁移 SQL 建）。

### 3.2 PatchOutcome（Ent Schema，新增表 `patch_outcomes`）

| 字段 | 类型 | 说明 |
|------|------|------|
| id | string (UUID) | 主键 |
| run_id | string，索引 | 关联 run |
| suggestion_id | string | 冗余便于按建议查询 |
| verdict | string(16) | effective/neutral/regressed |
| metrics_before | JSON | 应用前 1h 滑窗 {error_rate,p95_ms,alert_count} |
| metrics_after | JSON | 观察窗内同口径 |
| rollback_reason | string(256)，可空 | 回滚触发原因 |
| pattern_hash | string(32)，可空 | regressed 时写 KB 的模式哈希 |
| created_at | time | |

### 3.3 复用表

- `unified_evolution_suggestions`：target_type=platform 行（枚举扩展，无需 DDL；DDL 迁移仅登记新枚举值的 CHECK 放松——如现有 CHECK 约束不含 platform 需一次迁移放宽，见开发计划 T1.3）
- `failure_patterns`：Learn 写入 source=self_improvement

---

## 四、接口设计（biz 层端口，窄接口 ≤5 方法，Stability 标注）

### 4.1 运行存储

```go
// Stability:evolving
type SelfImprovementRunReader interface {
    GetByID(ctx context.Context, id string) (*SelfImprovementRun, error)
    GetBySuggestionID(ctx context.Context, suggestionID string) (*SelfImprovementRun, error)
    List(ctx context.Context, filter RunFilter) ([]SelfImprovementRun, error) // 状态/风险/触发源/分页
    ListObserving(ctx context.Context, now time.Time) ([]SelfImprovementRun, error) // Watchdog
}

// Stability:evolving
type SelfImprovementRunWriter interface {
    Create(ctx context.Context, run *SelfImprovementRun) error
    UpdateStatus(ctx context.Context, id string, from, to RunStatus, patch RunPatch) error // CAS from 状态
    RecordAttempt(ctx context.Context, id string) error
}

// Stability:evolving
type PatchOutcomeWriter interface {
    Create(ctx context.Context, outcome *PatchOutcome) error
    ListByRun(ctx context.Context, runID string) ([]PatchOutcome, error)
}
```

`UpdateStatus` 采用 CAS（WHERE status=from）保证状态机迁移原子性；冲突返回 CodeConflict。

### 4.2 沙盒端口

```go
// Stability:evolving
type RepoSandbox interface {
    PrepareWorktree(ctx context.Context, runID, baseRef string) (path string, cleanup func(), err error)
    ApplyDiff(ctx context.Context, path, diff string) error
    RunGate(ctx context.Context, path string, gate SandboxGateKind, pkgs []string) (SandboxGateResult, error) // G1/G2/G3
}
```

> 落地注记（2026-07-30，Phase 2）：类型名为 `SandboxGateKind`/`SandboxGateResult`（加前缀避免与 `verification_gate.go` 的 `GateResult` 冲突）。G4（Critic Agent）与 G5（Eval 基线）不经 `RepoSandboxRunner` 执行，由 Meta Team 相应阶段处理。

### 4.3 信号源端口（触发器依赖，面向既有子系统的窄读接口）

```go
// Stability:evolving — FailurePattern KB 聚类读
type ErrorClusterReader interface { ListErrorClusters(ctx, since time.Time, minCount int) ([]ErrorCluster, error) }
// Stability:evolving — monitor 指标读
type PerfMetricsReader interface { GetStepLatencyBaseline(ctx, window string) ([]StepLatencyStat, error); GetTokenUsageAnomaly(ctx, window string) ([]TokenAnomaly, error) }
// Stability:evolving — evaluation 基线读
type EvalBaselineReader interface { GetLatestBaseline(ctx) (*EvalBaseline, error); GetPreviousBaseline(ctx) (*EvalBaseline, error) }
// Stability:evolving — 测试结果读
type TestRunReader interface { ListRecentFailures(ctx, rounds int) ([]TestFailure, error) }
```

### 4.4 治理与应用

```go
// Stability:evolving
type RiskClassifier interface { Classify(p Patch, critic CriticReport) GovernanceDecision } // 纯代码，无 ctx/io
// Stability:evolving
type Applier interface {
    ApplyHotReload(ctx context.Context, run *SelfImprovementRun) (rollbackRef string, err error)
    ApplyCodeMerge(ctx context.Context, run *SelfImprovementRun) (commitSHA string, err error)
    Rollback(ctx context.Context, run *SelfImprovementRun, reason string) error
}
```

---

## 五、Worker / Cron 设计

| Worker | 周期 | 职责 | 模式参照 |
|--------|------|------|----------|
| self_improve_observe | 15min | 对 platform 目标调用编排器 CheckAndCreate；为 pending 建议创建 run(status=detected) 并启动 Meta Team 会话 | V2 skill_intelligence_worker |
| self_improve_watchdog | 5min | 扫描 observing 且 observe_until<=now 的 run：指标对比→close 或自动 rollback | V2 predictive_heal |
| self_improve_outcome | 1h | 终态 run 生成 PatchOutcome + KB 反哺 + 触发器自适应降频 | V2 pattern_mining |

分布式安全：复用现有 worker 单实例运行约定（workers.go 启动模式），Watchdog 的 rollback 以 run 状态 CAS 防重。

---

## 六、Wire DI 影响分析

### 6.1 新增绑定（cmd/admin/wire.go）

- `SelfImprovementRunReader/Writer`、`PatchOutcomeWriter` → data 层实现
- 4 触发器 → 构造后注册进既有 `SkillEvolutionOrchestrator`（ProvideSet 返回注册器 func 或在 ProvideRunners 侧注册，参照现有 Trigger 装配方式）
- `RepoSandbox` → `service.RepoSandboxRunner`
- `RiskClassifier` / `Applier` → biz 实现
- 3 个 worker → `startBackgroundWorkers` 注册

### 6.2 配置新增（config.yaml）

```yaml
self_improvement:
  enabled: false                # 默认关闭，灰度开启
  observe_interval: 15m
  error_cluster: {window_days: 7, min_count: 5}
  perf: {latency_factor: 2.0, token_factor: 1.5}
  eval: {regression_threshold: 0.10}
  patch: {max_diff_lines: 500, daily_auto_apply_quota: 5, max_attempts: 3}
  sandbox: {gate_timeouts: {g1: 5m, g2: 10m, g3: 5m}, worktree_root: ".aranea-self-improve"}
  observe_window: {duration: 24h, error_rate_factor: 1.5, p95_factor: 1.3, max_concurrent_observing: 3}
```

---

## 七、Proto 设计（P5 控制台 API，本阶段仅定义契约）

`api/kratos/self_improvement/v1/self_improvement.proto`：

| RPC | HTTP | 说明 | 鉴权 |
|-----|------|------|------|
| ListRuns | GET /api/v1/self-improvement/runs | 运行列表（状态/风险/触发源筛选+分页） | admin |
| GetRun | GET /api/v1/self-improvement/runs/{id} | 详情（诊断/diff/验证/治理/时间线） | admin |
| ApproveRun | POST /api/v1/self-improvement/runs/{id}:approve | 高风险审批通过（body: reason） | admin |
| RejectRun | POST /api/v1/self-improvement/runs/{id}:reject | 审批拒绝（body: reason 必填） | admin |
| RollbackRun | POST /api/v1/self-improvement/runs/{id}:rollback | 手动回滚 | admin |
| CloseRun | POST /api/v1/self-improvement/runs/{id}:close | 观察窗提前关闭 | admin |
| GetOutcomeStats | GET /api/v1/self-improvement/outcome-stats | 成效统计 | admin |
| UpdateRiskRules | PUT /api/v1/self-improvement/risk-rules | 分级规则配置 | admin |

P1-P4 阶段高风险审批经由既有聊天审批 activity 完成，不落 Proto。

---

## 八、前端组件设计（P5）

| 组件 | 位置 | 说明 |
|------|------|------|
| SelfImprovementPage | `web/src/pages/SelfImprovementPage.vue` | 控制台主页：统计卡片 + runs 表格（筛选/分页） |
| RunDetailDrawer | `web/src/features/self-improvement/RunDetailDrawer.vue` | 详情抽屉：时间线 + diff 渲染（diff2html）+ 五级 Gate 结果 + 审批操作 |
| OutcomeStatsPanel | `web/src/features/self-improvement/OutcomeStatsPanel.vue` | 成效图表（有效率/回滚率/触发源分布，复用 echarts 封装） |
| RiskRulesDialog | `web/src/features/self-improvement/RiskRulesDialog.vue` | 分级规则配置 |

数据流遵循前端铁律：feature store（Pinia）→ services/kratos/self_improvement；文案全部 i18n（skillsPage 模式参照）。

---

## 九、风险与缓解

| 风险 | 影响 | 缓解 |
|------|------|------|
| 补丁引入隐性回归（测试未覆盖） | 生产劣化 | G4 Critic + 观察窗自动回滚 + 高风险人工审批三重兜底 |
| worktree 磁盘膨胀 | 磁盘占满 | run 终态即清理；启动时扫描残留清理；单 run worktree 配额 |
| 多补丁观察窗指标互相污染 | 误回滚 | 观察窗并发 ≤3；同核心路径补丁串行（governance 队列） |
| LLM 生成恶意/敏感内容补丁 | 安全事故 | 保护文件清单 + 敏感信息检测（复用 V2 SEL-08）+ Critic 审查 |
| 沙盒命令逃逸（diff 含脚本） | 沙盒被利用 | Gate 命令白名单（仅 go/pnpm/golangci-lint 固定参数）；环境变量白名单；无网络（除 pnpm install） |
| Ent/Proto 生成物被手改 | 构建破坏 | 保护清单禁手改，仅允许 regenerate 流程（D9） |
| 触发器风暴（错误爆发产生大量建议） | LLM 成本失控 | 编排器 pending 去重 + per-action 冷却 + Patcher 日配额 |

---

## 十、测试策略

| 层 | 测试 | 说明 |
|----|------|------|
| biz 单测 | 状态机全迁移表、RiskClassifier 规则矩阵、触发器阈值/去重、verdict 判定 | 无 IO，表驱动 |
| data 集成 | run/outcome Repo CRUD + CAS 冲突 + 观察窗查询 | testhelper.SetupTestPG，tag=integration |
| service 集成 | RepoSandboxRunner 对 fixture 仓库执行 G1-G3（小 Go module fixture） | tag=integration |
| 端到端（P4） | httptest：信号→run→审批→应用（mock git/LLM） | tag=integration |
| 前端（P5） | store 单测 + 组件渲染 | pnpm test |

---

*文档版本：2026-07-29 — 初始版本（与设计确认稿一致：全量代码可补丁 + 风险分级审批 + 平台内 Meta Team + 代码类合并重启生效/配置类热加载）。*
