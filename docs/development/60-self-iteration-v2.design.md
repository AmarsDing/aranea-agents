# Self-Iteration V2 — 实现设计文档

> 对应需求：[60-self-iteration-v2.md](./60-self-iteration-v2.md)
> 遵循规范：四层架构（Server→Service→Biz→Data）+ Wire DI
> OpenSpec Change：`openspec/changes/self-iteration-v2/design.md`

---

## 一、架构全景图

### 1.1 三阶段闭环架构

```
┌─────────────────────────────────────────────────────────────────────┐
│                    阶段一：闭环加固                                   │
│                                                                     │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────────────┐  │
│  │ FailureReport│───►│RootCause     │───►│ failure_pattern      │  │
│  │ 标准化错误   │    │Analyzer 接口  │    │ 统一知识库            │  │
│  └──────────────┘    └──────────────┘    └──────────────────────┘  │
│         │                                          ▲               │
│         ▼                                          │               │
│  ┌──────────────┐    ┌──────────────┐    ┌────────┴─────────────┐ │
│  │ Auto-Fix     │───►│ Critic Agent │───►│ failure_pattern_sync │ │
│  │ 引擎改造     │    │ 语义回归检查  │    │ Cron Job             │ │
│  └──────────────┘    └──────────────┘    └──────────────────────┘  │
└─────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────┐
│                    阶段二：Skill Intelligence 落地                    │
│                                                                     │
│  ┌──────────────────────┐    ┌──────────────────────┐              │
│  │ skill_intelligence   │───►│ ExperienceReport     │              │
│  │ _worker Cron Job     │    │ + RootCauseAnalysis  │              │
│  └──────────────────────┘    └──────────────────────┘              │
│         │                              │                            │
│         ▼                              ▼                            │
│  ┌──────────────────────┐    ┌──────────────────────┐              │
│  │ DynamicRankFactors   │    │ Curator Agent        │              │
│  │ + HealthMetrics      │    │ + Sandbox Runner     │              │
│  │   Provider           │    │ + 进化审批 UI         │              │
│  └──────────────────────┘    └──────────────────────┘              │
└─────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────┐
│                    阶段三：自我进化闭环                                │
│                                                                     │
│  ┌──────────────────┐    ┌──────────────────────────────────────┐  │
│  │ PredictiveHeal   │───►│ Skill 五阶段进化闭环                  │  │
│  │ Usecase          │    │ Solve→Observe→Evolve→Gate→Reload     │  │
│  └──────────────────┘    └──────────────────────────────────────┘  │
│         │                              │                            │
│         ▼                              ▼                            │
│  ┌──────────────────┐    ┌──────────────────────────────────────┐  │
│  │ predictive_heal  │    │ PatternMiningWorker                  │  │
│  │ Cron Job         │    │ + pattern_mining Cron Job            │  │
│  └──────────────────┘    └──────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────┘
```

### 1.2 数据流全景

```
CI 失败 ──► FailureReport ──► Auto-Fix ──► Critic Agent ──► PR / 知识库
                                        │
运行时错误 ──► RootCauseAnalyzer ────────┤
                                        ▼
                              failure_pattern 知识库
                                        │
                    ┌───────────────────┼───────────────────┐
                    ▼                   ▼                   ▼
           PredictiveHeal      PatternMining      Skill Intelligence
           (预测性自愈)        (动态挖掘)          (经验报告+进化)
```

### 1.3 现有代码基线

| 能力 | 现有实现 |
|------|----------|
| 运行时自愈 | `SelfHealObserver` + `RootCauseEngine`（12 条内置规则）+ 滑动窗口断路器 |
| CI Auto-Fix | `auto-fix.yml` 完整流水线（日志提取→分类→修复→验证→PR） |
| Skill Intelligence | `SkillIntelligenceUsecase`（AnalyzeInvocation/ScoreSkill/GenerateReport） |
| 知识库（改造前） | 运行时 `RootCauseEngine` 规则与 CI `.auto-fix/patterns.jsonl` 互相独立 |
| 集成测试（改造前） | 仅 3 个文件覆盖 Agent CRUD/Chat API/Channel Turn Preview |

---

## 二、核心设计决策

### D1: RootCauseEngine 接口抽取

**选择**：抽取 `RootCauseAnalyzer` 接口，由 `RootCauseEngine` 实现

**理由**：
- `SkillIntelligenceUsecase` 和 `PredictiveHealUsecase` 都需要根因分析能力
- 当前 `RootCauseEngine` 是具体结构体，无法通过 Wire 注入到其他包
- 接口抽取符合依赖倒置原则，Biz 层同包内接口定义不违反分层规范

**备选方案**：
- A) 直接使用 `RootCauseEngine` 具体类型 → 否决，违反依赖倒置，无法 Wire 注入
- B) 将 `RootCauseEngine` 移到独立包 → 否决，增加包间复杂度，同包接口更简洁

### D2: FailureReport 标准化错误表示

**选择**：定义 `FailureReport` 结构体，统一 CI 和运行时的错误描述格式

**理由**：
- 受 SWE-agent ACI 启发：为 LLM Agent 设计专用交互界面，而非复用人类格式
- 当前 CI 日志是原始文本，LLM 需要自行解析，效率低且不稳定
- 结构化表示让分类路由更精确，减少 LLM 误判

### D3: 统一失败模式知识库

**选择**：新增 `failure_pattern` 表（SQLite/Ent），合并运行时规则与 CI 模式

**理由**：
- 当前两套知识库互相隔离：运行时 12 条硬编码规则 vs CI 4 个手写 Markdown 模板
- 统一存储后可实现跨场景学习
- 为阶段三的动态挖掘提供数据基础

### D4: Critic Agent 语义回归检查

**选择**：在 Auto-Fix 验证通过后，用 LLM 对比 diff 检查语义偏差

**理由**：
- 受 CodeMender 的 LLM 批评工具启发
- 当前验证只有 `make test && make lint`，无法检测语义级回归
- Critic Agent 输出结构化评审结果，辅助人工 Review

**流程**：

```
Auto-Fix 生成 patch → go vet + pnpm build 通过
    │
    ▼
Critic Agent (LLM)
    ├─ 输入：原始代码 + 修复后代码 + FailureReport
    ├─ 输出：CriticResult{is_safe, risk_level, concerns[], suggestion}
    └─ risk_level: low/medium/high
    │
    ▼
low → 直接创建 PR
medium → 创建 PR + 添加 "needs-careful-review" 标签
high → 放弃修复，记录到知识库
```

### D5: DynamicRankFactors 接口桥接

**选择**：在 Tools 层定义 `HealthMetricsProvider` 接口，由 Biz 层实现并注入

**理由**：
- `DynamicRankFactors`（Tools 层）需要读取 Skill 健康指标（Biz 层）
- 直接依赖违反"Tools 不依赖 Biz"的分层原则
- 接口桥接是标准解法：Tools 定义接口，Biz 实现，Wire 注入

### D6: Skill Intelligence Cron Worker

**选择**：新增 `skill_intelligence_worker` Cron Job，每 15 分钟扫描未分析的 `skill_invocation`

**理由**：
- 当前 `AnalyzeInvocation`/`ScoreSkill`/`GenerateReport` 仅在显式调用时触发
- Skill 调用通过 `recordSkillInvocation` 记录后，无后续分析
- Cron Worker 批量处理，避免每次调用都触发分析（性能考虑）

**Worker 逻辑**：

```
每 15 分钟：
1. 查询最近 15 分钟内未分析的 skill_invocation（WHERE analyzed_at IS NULL）
2. 批量 AnalyzeInvocation → 失败标签
3. ScoreSkill → 健康评分
4. GenerateReport → 经验报告持久化（集成 RootCauseAnalyzer）
5. 更新 skill_invocation.analyzed_at
```

### D7: Curator Agent 半自动进化

**选择**：通过 `ChatOrchestrator` 调用自身 Agent 生成 Skill 草案

**理由**：
- 项目本身是 AI Agent 平台，已有完整的 LLM 集成
- 通过 `POST /v1/chat/messages` 调用自身的 Agent，无需外部 API key
- 符合"用自身 AI 能力进化自身"的原则
- 修复 PR 需人工 review，保证安全性

**安全约束**：
- 仅修改 Skill 的 SKILL.md（提示词），不修改代码文件
- 每日 Curator 调用上限 20 次（Token 成本控制）
- 进化建议 7 天未审批自动过期

### D8: 预测性自愈

**选择**：基于历史模式的趋势预测 + 提前干预

**理由**：
- 当前运行时自愈是被动响应（错误发生后才触发）
- 竞品趋势（A-Evolve/Live-SWE-agent）表明预测性运维是下一步
- 基于 `FailurePattern` 知识库的趋势分析，可在错误发生前干预

**安全约束**：
- 仅对置信度 > 0.8 的预测执行预防行动
- 预防行动有冷却期（同类型 30 分钟内不重复执行）
- 所有预防行动记录到 HealRecord，可审计

### D9: 知识库动态挖掘

**选择**：Cron Job 从历史修复记录自动提取修复模板

**理由**：
- 当前 `known-fixes/` 是 4 个手写 Markdown 模板，覆盖面窄
- 受 Live-SWE-agent 启发：Agent 不仅修复 bug，还能改进自身的修复策略
- 动态挖掘让知识库持续进化，减少人工维护

---

## 三、数据模型设计

### 3.1 FailureReport（内存结构体，非持久化）

```go
// internal/biz/monitor/failure_report.go
type FailureType string

const (
    FailureTypeLint      FailureType = "lint_error"
    FailureTypeTest      FailureType = "test_failure"
    FailureTypeBuild     FailureType = "build_failure"
    FailureTypeProtoSync FailureType = "proto_sync"
    FailureTypeRuntime   FailureType = "runtime_error"
)

type FailureReport struct {
    ID          string            `json:"id"`
    Type        FailureType       `json:"type"`
    Source      string            `json:"source"`       // "ci" or "runtime"
    Job         string            `json:"job"`          // CI job name or runtime component
    File        string            `json:"file"`         // source file path
    Line        int               `json:"line"`         // line number (0 if unknown)
    ErrorCode   string            `json:"error_code"`   // machine-readable error code
    Message     string            `json:"message"`      // human-readable error message
    StackTrace  string            `json:"stack_trace"`  // full stack trace (runtime errors)
    RelatedCode string            `json:"related_code"` // surrounding code snippet
    Metadata    map[string]string `json:"metadata"`     // extra key-value pairs
}
```

### 3.2 FailurePattern（Ent Schema，持久化）

**表名**：`failure_pattern`
**Schema 路径**：`internal/data/ent/schema/failure_pattern.go`

```go
type FailurePattern struct {
    ID           string               // UUID, MaxLen(64), Unique, Immutable
    Source       FailurePatternSource // "runtime" | "ci" | "mined", MaxLen(32)
    Type         string               // FailureType, MaxLen(64)
    PatternHash  string               // SHA256(pattern_regex), MaxLen(64)
    PatternRegex string               // 正则表达式, Text
    FixAction    FixAction            // JSON(FixAction), Text
    Confidence   float64              // 0-1, Default(0.5)
    SuccessCount int                  // Default(0)
    FailCount    int                  // Default(0)
    Version      int                  // Default(1)
    IsActive     bool                 // Default(true)
    CreatedAt    time.Time            // Immutable
    UpdatedAt    time.Time            // auto-update via SQL
}
```

**索引**：
- `(source, type)` — 按来源和类型查询
- `(pattern_hash)` — 精确索引，不使用 pattern_regex 做索引
- `(is_active, confidence)` — 活跃规则按置信度排序

### 3.3 ExperienceReport（Ent Schema，持久化）

**表名**：`experience_reports`
**Schema 路径**：`internal/data/ent/schema/experience_report.go`

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | string(256) | UUID, Immutable, Unique |
| `tenant_id` | string(256) | 租户 ID |
| `session_id` | string(256) | 会话 ID |
| `invocation_id` | string(256) | Skill 调用 ID |
| `skill_id` | string(256) | Skill ID |
| `skill_name` | string(256) | Skill 名称（可选） |
| `is_success` | bool | 是否成功 |
| `score` | int | 健康评分 |
| `failure_tags` | JSON([]string) | 失败标签 |
| `flow_summary` | Text | 流程摘要 |
| `optimization_advice` | Text | 优化建议 |
| `selection_snapshot` | JSON(map) | 选择快照 |
| `root_cause_analysis` | Text | 根因分析结果（V2 新增） |
| `suggested_fix` | Text | 人类可读的修复建议（V2 新增） |
| `generated_suggestion_id` | string(256) | 生成的建议 ID |
| `created_at` | string | 创建时间 |

**索引**：
- `(skill_id, created_at)` — `idx_experience_report_skill_time`
- `(invocation_id)` — `idx_experience_report_invocation`

### 3.4 SkillEvolutionSuggestion（Ent Schema，持久化）

**表名**：`skill_evolution_suggestions`（注意：复数）
**Schema 路径**：`internal/data/ent/schema/skill_evolution_suggestion.go`

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | string(256) | UUID, Immutable, Unique |
| `skill_id` | string(256) | 关联 Skill |
| `type` | string(64) | "prompt_optimize" \| "tool_adjust" \| "config_tune" |
| `status` | string(64) | "pending" \| "approved" \| "rejected" \| "expired"，Default("pending") |
| `source_report_ids` | JSON([]string) | 来源经验报告 ID 列表 |
| `trigger_reason` | Text | 触发原因 |
| `draft_skill_body` | Text | 建议变更内容（SKILL.md 草案） |
| `draft_version_id` | string(256) | 草案版本 ID |
| `sandbox_passed` | bool | Sandbox 验证是否通过 |
| `sandbox_result` | JSON(map) | Sandbox 验证结果 |
| `pre_verify_result` | JSON(map) | 预验证结果 |
| `approved_by` | string(256) | 审批人 |
| `rejected_by` | string(256) | 拒绝人 |
| `rejection_reason` | Text | 拒绝原因 |
| `created_at` | string | 创建时间 |
| `resolved_at` | string | 解决时间（可选） |
| `parent_version_id` | string(256) | 父版本 Skill ID（Curator 进化追踪） |
| `evolution_reason` | Text | 进化原因 |
| `lifecycle_status` | string(64) | Skill 生命周期状态，Default("draft") |

> **注**：过期时间不作为持久化字段，由 Biz 层常量 `EvoExpirationDays = 7` 结合 `created_at` 计算得出。

**索引**：
- `(skill_id, status)` — `idx_evo_suggestion_skill_status`
- `(status, created_at)` — `idx_evo_suggestion_status_time`

---

## 四、接口设计

### 4.1 RootCauseAnalyzer

```go
// internal/biz/monitor/root_cause_analyzer.go
type RootCauseAnalyzer interface {
    // Analyze performs root cause analysis for the given step/phase error and
    // returns the best-matching result. Returns nil if no rule matches.
    Analyze(ctx context.Context, stepID, phase string, err error, metadata map[string]any) (*RootCauseResult, error)

    // AnalyzeFromReport performs root cause analysis from a standardized
    // FailureReport. It converts the report into the internal metadata format
    // and delegates to Analyze. Returns nil if no rule matches.
    AnalyzeFromReport(ctx context.Context, report *FailureReport) (*RootCauseResult, error)
}
```

**实现者**：`RootCauseEngine`（已实现，无需修改方法签名）

**消费者**：
- `SkillIntelligenceUsecase`（通过 Wire 注入）
- `PredictiveHealUsecase`（通过 Wire 注入）

> **实现备注**：Biz 层 `skill_intelligence.go` 另定义了 `AnalyzeInvocationFailure(ctx, inv)` 方法用于解耦 biz→monitor 依赖，与 monitor 包的接口互补。

### 4.2 HealthMetricsProvider

```go
// internal/tools/skillrecommend/health_provider.go
type HealthMetricsProvider interface {
    GetRecentSuccessRate(ctx context.Context, skillID string, days int) (float64, error)
    GetRecentAvgDuration(ctx context.Context, skillID string, days int) (float64, error)
}
```

**实现者**：Biz 层 `SkillHealthAggregator` 适配器

**消费者**：`DynamicRankFactors`（Tools 层）

### 4.3 FailurePatternReader / FailurePatternWriter

```go
// internal/biz/monitor/failure_pattern_repo.go

// FailurePatternReader provides read access to the failure pattern knowledge base.
type FailurePatternReader interface {
    // ListBySource returns all patterns matching the given source.
    ListBySource(ctx context.Context, source FailurePatternSource) ([]FailurePattern, error)
    // GetByPatternHash returns the pattern with the given hash, or nil if not found.
    GetByPatternHash(ctx context.Context, hash string) (*FailurePattern, error)
    // ListActive returns all active patterns, ordered by confidence descending.
    ListActive(ctx context.Context) ([]FailurePattern, error)
}

// FailurePatternWriter provides write access to the failure pattern knowledge base.
type FailurePatternWriter interface {
    // Create inserts a new failure pattern.
    Create(ctx context.Context, pattern FailurePattern) error
    // Update updates an existing failure pattern by ID.
    Update(ctx context.Context, pattern FailurePattern) error
    // IncrementSuccess atomically increments the success_count for the pattern with the given ID.
    IncrementSuccess(ctx context.Context, id string) error
    // IncrementFail atomically increments the fail_count for the pattern with the given ID.
    IncrementFail(ctx context.Context, id string) error
    // Deactivate sets is_active = false for the pattern with the given ID.
    Deactivate(ctx context.Context, id string) error
}
```

**实现者**：`internal/data/failure_pattern_repo.go`

---

## 五、Cron Job 设计

> 以下 Cron Job 均已实现并注册到 Wire DI。

### 5.1 skill_intelligence_worker

| 项 | 说明 |
|----|------|
| 频率 | 每 15 分钟（默认，可通过构造参数覆盖） |
| 路径 | `internal/cronrunner/jobs/skill_intelligence_worker.go` |
| 逻辑 | 查询 `analyzed_at IS NULL` 的 `skill_invocation` → 批量 AnalyzeInvocation → ScoreSkill → GenerateReport（集成 RootCauseAnalyzer）→ 更新 `analyzed_at` |

### 5.2 failure_pattern_sync

| 项 | 说明 |
|----|------|
| 频率 | 每日（默认 24h，可通过环境变量 `FAILURE_PATTERN_SYNC_INTERVAL` 覆盖） |
| 路径 | `internal/cronrunner/jobs/failure_pattern_sync.go` |
| 逻辑 | 从 `RootCauseEngine` 规则 + `.auto-fix/patterns.jsonl` 同步到 `failure_pattern` 表 |

### 5.3 predictive_heal

| 项 | 说明 |
|----|------|
| 频率 | 每 5 分钟（默认，可通过环境变量 `PREDICTIVE_HEAL_INTERVAL` 覆盖） |
| 路径 | `internal/cronrunner/jobs/predictive_heal.go` |
| 逻辑 | 读取系统指标 → 匹配 FailurePattern 前置条件 → 计算预测置信度 → 高置信度（> 0.8）时执行预防行动 |

### 5.4 pattern_mining

| 项 | 说明 |
|----|------|
| 频率 | 每日（默认 24h，可通过环境变量 `PATTERN_MINING_INTERVAL` 覆盖） |
| 路径 | `internal/cronrunner/jobs/pattern_mining.go` |
| 逻辑 | 读取 patterns.jsonl + HealRecord → 聚类相似失败模式 → 提取共性修复策略 → 写入 failure_pattern 表（source="mined"） |

---

## 六、CI/CD 改造设计

> 以下改造已在 `.github/workflows/auto-fix.yml` 中实现。

### 6.1 auto-fix.yml 改造

**新增步骤**：

```yaml
# 步骤 1：结构化日志解析（新增）
- name: Parse failure logs
  run: python3 .auto-fix/scripts/parse-logs.py > failure-report.json
  env:
    FAILURE_TYPE: ${{ steps.detect-type.outputs.type }}

# 步骤 2：Agent 诊断（改造，使用结构化输入）
- name: Agent diagnosis
  run: |
    if [ -f failure-report.json ]; then
      # 使用结构化 FailureReport 作为输入
      curl -X POST $ARANEA_API_URL/v1/chat/messages \
        -H "Content-Type: application/json" \
        -d @failure-report.json
    fi

# 步骤 3：Critic Agent 语义检查（新增）
- name: Critic Agent check
  if: env.ENABLE_CRITIC_AGENT != 'false'
  run: |
    curl -X POST $ARANEA_API_URL/v1/chat/messages \
      -H "Content-Type: application/json" \
      -d '{"session_id": "'$ARANEA_CRITIC_SESSION'", "message": "Review this diff..."}'
  env:
    ENABLE_CRITIC_AGENT: ${{ vars.ENABLE_CRITIC_AGENT }}
```

**白名单细化**：

```yaml
# 保护文件检查（改造）
- name: Check protected files
  run: |
    PROTECTED_PATTERNS=(
      ".github/workflows/*"
      "Makefile"
      "go.mod" "go.sum"
      "api/**/*.proto"
    )
    ALLOWLIST_PATTERNS=(
      "internal/biz/monitor/*"
    )
    # 检查 diff 中的文件是否匹配保护模式
    # 白名单中的文件跳过保护检查
```

---

## 七、Wire DI 影响分析

### 7.1 新增绑定

| 接口 | 实现 | Wire Set 位置 |
|------|------|----------|
| `RootCauseAnalyzer` | `RootCauseEngine` | `cmd/admin/wire.go` |
| `FailurePatternReader` | `FailurePatternRepo` | `cmd/admin/wire.go` |
| `FailurePatternWriter` | `FailurePatternRepo` | `cmd/admin/wire.go` |
| `HealthMetricsProvider` | `SkillHealthAggregatorAdapter` | `cmd/admin/wire.go` |

### 7.2 新增 Provider

| Provider | 位置 | 说明 |
|----------|------|------|
| `NewFailurePatternRepo` | `internal/data/failure_pattern_repo.go` | FailurePattern 数据层 |
| `NewSkillHealthAggregatorAdapter` | `internal/biz/` | HealthMetricsProvider 适配器 |
| `NewSkillIntelligenceService` | `internal/service/skill_intelligence.go` | Skill Intelligence API |
| `NewSkillEvolutionSuggestionService` | `internal/service/skill_evolution_suggestion.go` | 进化建议 API |
| `NewSkillCuratorService` | `internal/service/skill_curator.go` | Curator Agent 装配 |
| `NewPredictiveHealUsecase` | `internal/biz/monitor/predictive_heal.go` | 预测性自愈 |
| `NewPatternMiningUsecase` | `internal/biz/monitor/pattern_mining.go` | 知识库动态挖掘 |
| `provideSkillIntelligenceWorker` | `cmd/admin/wire.go` | Cron Job Provider |
| `provideFailurePatternSyncJob` | `cmd/admin/wire.go` | Cron Job Provider |
| `providePredictiveHealJob` | `cmd/admin/wire.go` | Cron Job Provider |
| `providePatternMiningJob` | `cmd/admin/wire.go` | Cron Job Provider |

### 7.3 新增 Cron Job 注册

| Job | 注册位置 |
|-----|----------|
| `skill_intelligence_worker` | `cmd/admin/wire.go` + `cmd/admin/workers.go` |
| `failure_pattern_sync` | `cmd/admin/wire.go` + `cmd/admin/workers.go` |
| `predictive_heal` | `cmd/admin/wire.go` + `cmd/admin/workers.go` |
| `pattern_mining` | `cmd/admin/wire.go` + `cmd/admin/workers.go` |

> Cron Job 通过 `cmd/admin/workers.go` 中的 `goAfterReady` 在 ReadinessGate 通过后启动。

---

## 八、风险与缓解措施

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| Critic Agent 增加 LLM 调用成本 | 中 | 仅在 test/lint 通过后触发；每日上限 10 次 |
| 动态挖掘的规则质量不可控 | 高 | 新规则初始 confidence=0.5；3 次成功验证后提升；每周人工审核 |
| 预测性自愈误触发 | 高 | 置信度阈值 0.8 + 冷却期 30min + 严重度分级 + 仅对高严重度执行 |
| Curator Agent Token 成本失控 | 中 | 每日调用上限 20 次；仅对触发条件的 Skill 执行 |
| RootCauseAnalyzer 接口变更影响现有自愈 | 低 | 接口是 `RootCauseEngine` 方法的子集，现有功能不变 |
| failure_pattern 表数据量增长 | 低 | 定期清理 90 天前的低置信度规则；成功/失败计数聚合 |
| HealthMetricsProvider 接口桥接增加复杂度 | 低 | 标准解法，Biz 层适配器仅包装已有 `SkillHealthAggregator` 方法 |
| 集成测试环境依赖 | 中 | 使用 build tag `integration` 隔离；CI 中可选运行 |

---

## 九、Proto 设计

### 9.1 skill_intelligence.proto

**路径**：`api/kratos/skill_intelligence/v1/skill_intelligence.proto`
**Package**：`kratos.skill_intelligence.v1`

```protobuf
service SkillIntelligenceService {
  rpc ListExperienceReports(ListExperienceReportsRequest) returns (ListExperienceReportsResponse) {
    option (google.api.http) = {get: "/v1/skill-intelligence/experience-reports"};
  }
  rpc GetExperienceReport(GetExperienceReportRequest) returns (GetExperienceReportResponse) {
    option (google.api.http) = {get: "/v1/skill-intelligence/experience-reports/{id}"};
  }
}
```

**关键消息**：
- `ExperienceReport`：包含 `root_cause_analysis`（字段 14）和 `suggested_fix`（字段 15）两个 V2 新增字段
- `ListExperienceReportsResponse`：包含 `failure_tag_counts` 和 `root_cause_reports` 聚合字段

### 9.2 skill_evolution_suggestion.proto

**路径**：`api/kratos/skill_evolution_suggestion/v1/skill_evolution_suggestion.proto`
**Package**：`kratos.skill_evolution_suggestion.v1`

```protobuf
service SkillEvolutionSuggestionService {
  rpc ListSkillEvolutionSuggestions(ListSkillEvolutionSuggestionsRequest) returns (ListSkillEvolutionSuggestionsResponse) {
    option (google.api.http) = { get: "/v1/skill-evolution-suggestions" };
  }
  rpc GetSkillEvolutionSuggestion(GetSkillEvolutionSuggestionRequest) returns (GetSkillEvolutionSuggestionResponse) {
    option (google.api.http) = { get: "/v1/skill-evolution-suggestions/{id}" };
  }
  rpc ApproveSkillEvolutionSuggestion(ApproveSkillEvolutionSuggestionRequest) returns (ApproveSkillEvolutionSuggestionResponse) {
    option (google.api.http) = { post: "/v1/skill-evolution-suggestions/{id}/approve" body: "*" };
  }
  rpc RejectSkillEvolutionSuggestion(RejectSkillEvolutionSuggestionRequest) returns (RejectSkillEvolutionSuggestionResponse) {
    option (google.api.http) = { post: "/v1/skill-evolution-suggestions/{id}/reject" body: "*" };
  }
  rpc TriggerCuratorFlow(TriggerCuratorFlowRequest) returns (TriggerCuratorFlowResponse) {
    option (google.api.http) = { post: "/v1/skill-evolution-suggestions/trigger-curator" body: "*" };
  }
}
```

**关键消息**：
- `SkillEvolutionSuggestionMsg`：包含 `parent_version_id`（字段 17）、`evolution_reason`（字段 18）、`lifecycle_status`（字段 19）三个 Curator Agent 进化追踪字段

---

## 十、前端组件设计

### 10.1 经验报告列表页

**路径**：`web/src/pages/ExperienceReportListPage.vue`

**功能**：
- 调用 `ListExperienceReports` API 展示经验报告列表
- 支持 Skill ID、开始日期、结束日期筛选
- 展示成功/失败、评分、失败标签、流程摘要
- 展示根因分析结果和修复建议（V2 新增）

### 10.2 进化建议列表页

**路径**：`web/src/pages/EvolutionSuggestionListPage.vue`

**功能**：
- 调用 `ListSkillEvolutionSuggestions` API 展示进化建议列表
- 支持 Agent/Skill 维度切换、状态筛选
- 提供"触发 Curator"按钮（调用 `TriggerCuratorFlow` API）
- 提供 Approve/Reject 操作（调用 `ApproveSkillEvolutionSuggestion` / `RejectSkillEvolutionSuggestion` API）
- 展示沙箱验证结果与触发原因

---

*文档版本：2026-06-17 — 按三件套内容边界重组，修正 Proto/接口/字段/注册位置与代码一致。*
