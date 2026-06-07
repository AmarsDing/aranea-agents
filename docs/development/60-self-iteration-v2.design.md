# Self-Iteration V2 — 实现设计文档

> 对应需求：[60-self-iteration-v2.md](./60-self-iteration-v2.md)
> 遵循规范：四层架构（Server→Service→Biz→Data）+ Wire DI
> OpenSpec Change：`openspec/changes/self-iteration-v2/design.md`
> **当前进度**：Phase 1–3 ✅ 已落地

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

**选择**：新增 `skill_intelligence_worker` Cron Job，每 10 分钟扫描未分析的 `skill_invocation`

**理由**：
- 当前 `AnalyzeInvocation`/`ScoreSkill`/`GenerateReport` 仅在显式调用时触发
- Skill 调用通过 `recordSkillInvocation` 记录后，无后续分析
- Cron Worker 批量处理，避免每次调用都触发分析（性能考虑）

**Worker 逻辑**：

```
每 10 分钟：
1. 查询最近 10 分钟内未分析的 skill_invocation（WHERE analyzed_at IS NULL）
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
    Job         string            `json:"job"`
    File        string            `json:"file"`
    Line        int               `json:"line"`
    ErrorCode   string            `json:"error_code"`
    Message     string            `json:"message"`
    StackTrace  string            `json:"stack_trace"`
    RelatedCode string            `json:"related_code"`
    Metadata    map[string]string `json:"metadata"`
}
```

### 3.2 FailurePattern（Ent Schema，持久化）

```go
// internal/data/ent/schema/failure_pattern.go
type FailurePattern struct {
    ID           string    // UUID
    Source       string    // "runtime" | "ci" | "mined"
    Type         string    // FailureType
    PatternHash  string    // SHA256(pattern_regex)，用于精确索引
    PatternRegex string    // 正则表达式
    FixAction    string    // JSON(FixAction)
    Confidence   float64   // 0-1，置信度
    SuccessCount int       // 成功修复次数
    FailCount    int       // 修复失败次数
    Version      int       // 版本号，支持回滚
    IsActive     bool      // 是否启用（审计机制）
    CreatedAt    time.Time
    UpdatedAt    time.Time
}
```

**索引**：
- `(source, type)` — 按来源和类型查询
- `(pattern_hash)` — 精确索引，不使用 pattern_regex 做索引
- `(is_active, confidence DESC)` — 活跃规则按置信度排序

### 3.3 ExperienceReport 扩展

在已有 `ExperienceReport` 基础上新增字段：

| 字段 | 类型 | 说明 |
|------|------|------|
| `RootCauseAnalysis` | string | 根因分析结果（JSON） |
| `SuggestedFix` | string | 人类可读的修复建议 |

### 3.4 SkillEvolutionSuggestion（Ent Schema，新增）

```go
// internal/data/ent/schema/skill_evolution_suggestion.go
type SkillEvolutionSuggestion struct {
    ID              string    // UUID
    SkillID         string    // 关联 Skill
    Type            string    // "prompt_optimize" | "tool_adjust" | "config_tune"
    Status          string    // "pending" | "approved" | "rejected" | "expired"
    TriggerReason   string    // 触发原因（JSON）
    SuggestedChange string    // 建议变更内容（SKILL.md 草案）
    SandboxResult   string    // Sandbox 验证结果（JSON）
    ParentVersionID string    // 父版本 Skill ID
    EvolutionReason string    // 进化原因
    LifecycleStatus string    // Skill 生命周期状态
    ExpiresAt       time.Time // 过期时间（创建后 7 天）
    CreatedAt       time.Time
    UpdatedAt       time.Time
}
```

**索引**：`(skill_id, created_at)`、`(status, expires_at)`

---

## 四、接口设计

### 4.1 RootCauseAnalyzer

```go
// internal/biz/monitor/root_cause_analyzer.go
type RootCauseAnalyzer interface {
    Analyze(ctx context.Context, stepID, phase string, err error, metadata map[string]any) (*RootCauseResult, error)
    AnalyzeFromReport(ctx context.Context, report *FailureReport) (*RootCauseResult, error)
}
```

**实现者**：`RootCauseEngine`（已实现，无需修改方法签名）

**消费者**：
- `SkillIntelligenceUsecase`（通过 Wire 注入）✅
- `PredictiveHealUsecase`（通过 Wire 注入）✅

> **实现备注**：Biz 层 `skill_intelligence.go` 另定义了 `AnalyzeInvocationFailure(ctx, inv)` 方法用于解耦 biz→monitor 依赖，与 monitor 包的接口互补。

### 4.2 HealthMetricsProvider

```go
// internal/tools/skillrecommend/health_provider.go
type HealthMetricsProvider interface {
    GetRecentSuccessRate(ctx context.Context, skillID string, days int) (float64, error)
    GetRecentAvgDuration(ctx context.Context, skillID string, days int) (float64, error)
}
```

**实现者**：Biz 层 `SkillHealthAggregator` 适配器 ✅

**消费者**：`DynamicRankFactors`（Tools 层）✅

### 4.3 FailurePatternReader / FailurePatternWriter

```go
// internal/biz/monitor/failure_pattern_repo.go
type FailurePatternReader interface {
    FindByHash(ctx context.Context, patternHash string) (*FailurePattern, error)
    FindActiveByType(ctx context.Context, source, failureType string) ([]*FailurePattern, error)
    ListActive(ctx context.Context, limit, offset int) ([]*FailurePattern, error)
}

type FailurePatternWriter interface {
    Create(ctx context.Context, pattern *FailurePattern) error
    UpdateCounts(ctx context.Context, id string, success bool) error
    Deactivate(ctx context.Context, id string) error
    UpsertFromSync(ctx context.Context, patterns []*FailurePattern) error
}
```

**实现者**：`internal/data/failure_pattern_repo.go` ✅

---

## 五、Cron Job 设计

> 以下 Cron Job 均已实现并注册到 Wire DI。

### 5.1 skill_intelligence_worker ✅

| 项 | 说明 |
|----|------|
| 频率 | 每 15 分钟（实际实现） |
| 路径 | `internal/cronrunner/jobs/skill_intelligence_worker.go` |
| 逻辑 | 查询 `analyzed_at IS NULL` 的 `skill_invocation` → 批量 AnalyzeInvocation → ScoreSkill → GenerateReport（集成 RootCauseAnalyzer）→ 更新 `analyzed_at` |

### 5.2 failure_pattern_sync ✅

| 项 | 说明 |
|----|------|
| 频率 | 每日 |
| 路径 | `internal/cronrunner/jobs/failure_pattern_sync.go` |
| 逻辑 | 从 `RootCauseEngine` 规则 + `.auto-fix/patterns.jsonl` 同步到 `failure_pattern` 表 |

### 5.3 predictive_heal ✅

| 项 | 说明 |
|----|------|
| 频率 | 每 5 分钟 |
| 路径 | `internal/cronrunner/jobs/predictive_heal.go` |
| 逻辑 | 读取系统指标 → 匹配 FailurePattern 前置条件 → 计算预测置信度 → 高置信度（> 0.8）时执行预防行动 |

### 5.4 pattern_mining ✅

| 项 | 说明 |
|----|------|
| 频率 | 每日 |
| 路径 | `internal/cronrunner/jobs/pattern_mining.go` |
| 逻辑 | 读取 patterns.jsonl + HealRecord → 聚类相似失败模式 → 提取共性修复策略 → 写入 failure_pattern 表（source="mined"） |

---

## 六、CI/CD 改造设计

> 以下改造已在 `.github/workflows/auto-fix.yml` 中实现。

### 6.1 auto-fix.yml 改造 ✅

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

> 以下绑定均已实现。

### 7.1 新增绑定 ✅

| 接口 | 实现 | Wire Set |
|------|------|----------|
| `RootCauseAnalyzer` | `RootCauseEngine` | `internal/biz/wire.go` |
| `FailurePatternReader` | `FailurePatternRepo` | `internal/data/wire.go` |
| `FailurePatternWriter` | `FailurePatternRepo` | `internal/data/wire.go` |
| `HealthMetricsProvider` | `SkillHealthAggregatorAdapter` | `internal/biz/wire.go` |

### 7.2 新增 Provider ✅

| Provider | 位置 | 说明 |
|----------|------|------|
| `NewFailurePatternRepo` | `internal/data/failure_pattern_repo.go` | FailurePattern 数据层 |
| `NewSkillHealthAggregatorAdapter` | `internal/biz/` | HealthMetricsProvider 适配器 |
| `NewSkillIntelligenceService` | `internal/service/skill_intelligence.go` | Skill Intelligence API |
| `NewPredictiveHealUsecase` | `internal/biz/monitor/predictive_heal.go` | 预测性自愈 |
| `NewPatternMiningUsecase` | `internal/biz/monitor/pattern_mining.go` | 知识库动态挖掘 |

### 7.3 新增 Cron Job 注册 ✅

| Job | 注册位置 |
|-----|----------|
| `skill_intelligence_worker` | `internal/cronrunner/wire.go` |
| `failure_pattern_sync` | `internal/cronrunner/wire.go` |
| `predictive_heal` | `internal/cronrunner/wire.go` |
| `pattern_mining` | `internal/cronrunner/wire.go` |

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

### 9.1 skill_intelligence.proto ✅

> 实际路径：`api/kratos/skill_intelligence/v1/skill_intelligence.proto`（Kratos 规范路径）

```protobuf
service SkillIntelligenceService {
  rpc ListExperienceReports(ListExperienceReportsRequest) returns (ListExperienceReportsResponse) {
    option (google.api.http) = {get: "/v1/skill-intelligence/experience-reports"};
  }
  rpc GetExperienceReport(GetExperienceReportRequest) returns (ExperienceReportDetail) {
    option (google.api.http) = {get: "/v1/skill-intelligence/experience-reports/{id}"};
  }
  rpc ListEvolutionSuggestions(ListEvolutionSuggestionsRequest) returns (ListEvolutionSuggestionsResponse) {
    option (google.api.http) = {get: "/v1/skill-intelligence/evolution-suggestions"};
  }
  rpc ReviewEvolutionSuggestion(ReviewEvolutionSuggestionRequest) returns (ReviewEvolutionSuggestionResponse) {
    option (google.api.http) = {post: "/v1/skill-intelligence/evolution-suggestions/{id}/review"; body: "*"};
  }
}
```

---

*文档版本：2026-06-06 — Phase 1–3 已落地，标注实现状态。*
