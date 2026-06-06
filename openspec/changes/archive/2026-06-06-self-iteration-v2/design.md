## Context

Aranea-Agents 已具备双轨自愈能力和 Skill Intelligence Phase 1 基础框架，但存在三重断裂：

**当前状态**：
- 运行时自愈：`SelfHealObserver` + `RootCauseEngine`（12 条内置规则）+ 滑动窗口断路器，已实现但 `RootCauseEngine` 是具体结构体，无法被其他模块复用
- CI Auto-Fix：`auto-fix.yml` 完整流水线（日志提取→分类→修复→验证→PR），但 `stats.json` 全为 0，从未实际运行；日志解析为原始文本，非结构化
- Skill Intelligence：`SkillIntelligenceUsecase`（AnalyzeInvocation/ScoreSkill/GenerateReport）已实现，但 Cron Worker 未实现，无法自动触发；Phase 2-5 的 61 步未落地
- 知识库隔离：运行时 `RootCauseEngine` 规则与 CI `.auto-fix/patterns.jsonl` 互相独立，无法共享学习成果
- 集成测试：仅 3 个文件覆盖 Agent CRUD/Chat API/Channel Turn Preview，核心业务流程无覆盖

**约束**：
- 四层架构（Server→Service→Biz→Data）+ Wire DI
- Biz 层禁止 import `pkg/trpc-agent-go`
- Tools 层禁止直接依赖 Biz 层（需接口桥接）
- 日志统一使用 `pkg/loggateway.Logger`
- 错误统一使用 `kerrors`

## Goals / Non-Goals

**Goals:**
1. 激活 CI Auto-Fix 闭环，让已有机制真正运转
2. 统一运行时自愈与 CI Auto-Fix 的知识库，实现跨场景学习
3. 完成 Skill Intelligence Phase 2-5 核心价值功能
4. 建立"修复→学习→预防"的完整进化闭环
5. 补齐核心业务流程的集成测试

**Non-Goals:**
- 不改变已有 `skill_health`/`skill_evolution`/`skills_butler` 的业务逻辑
- 不改变 trpc-agent-go 框架层
- 不实现全自动化进化（仅半自动，需人工审批）
- 不做 K8s 部署或 staging 环境
- 不做性能自动调优
- 不修改已有 proto 定义

## Decisions

### D1: RootCauseEngine 接口抽取

**选择**：抽取 `RootCauseAnalyzer` 接口，由 `RootCauseEngine` 实现

**理由**：
- `SkillIntelligenceUsecase` 和 `PredictiveHealUsecase` 都需要根因分析能力
- 当前 `RootCauseEngine` 是具体结构体，无法通过 Wire 注入到其他包
- 接口抽取符合依赖倒置原则，Biz 层同包内接口定义不违反分层规范

**接口设计**：

```go
// internal/biz/monitor/root_cause_analyzer.go
type RootCauseAnalyzer interface {
    Analyze(ctx context.Context, stepID, phase string, err error, metadata map[string]any) (*RootCauseResult, error)
    AnalyzeFromReport(ctx context.Context, report *FailureReport) (*RootCauseResult, error)
}
```

**备选方案**：
- A) 直接使用 `RootCauseEngine` 具体类型 → 否决，违反依赖倒置，无法 Wire 注入
- B) 将 `RootCauseEngine` 移到独立包 → 否决，增加包间复杂度，同包接口更简洁

### D2: FailureReport 标准化错误表示

**选择**：定义 `FailureReport` 结构体，统一 CI 和运行时的错误描述格式

**理由**：
- 受 SWE-agent ACI 启发：为 LLM Agent 设计专用交互界面，而非复用人类格式
- 当前 CI 日志是原始文本，LLM 需要自行解析，效率低且不稳定
- 结构化表示让分类路由更精确，减少 LLM 误判

**结构体设计**：

```go
// internal/biz/monitor/failure_report.go
type FailureType string

const (
    FailureTypeLint        FailureType = "lint_error"
    FailureTypeTest        FailureType = "test_failure"
    FailureTypeBuild       FailureType = "build_failure"
    FailureTypeProtoSync   FailureType = "proto_sync"
    FailureTypeRuntime     FailureType = "runtime_error"
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

**CI 侧解析**：在 `auto-fix.yml` 中新增 Python 脚本步骤，将原始日志解析为 `FailureReport` JSON，传给 Agent/pattern-fix。

### D3: 统一失败模式知识库

**选择**：新增 `failure_pattern` 表（SQLite/Ent），合并运行时规则与 CI 模式

**理由**：
- 当前两套知识库互相隔离：运行时 12 条硬编码规则 vs CI 4 个手写 Markdown 模板
- 统一存储后可实现跨场景学习：CI 修复模式可被运行时参考，运行时规则可被 CI 使用
- 为阶段三的动态挖掘提供数据基础

**Ent Schema**：

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

**索引**：`(source, type)`, `(pattern_hash)`, `(is_active, confidence DESC)`

**同步机制**：Cron Job `failure_pattern_sync` 每日从 `RootCauseEngine` 规则 + `.auto-fix/patterns.jsonl` 同步到统一知识库。

### D4: Critic Agent 语义回归检查

**选择**：在 Auto-Fix 验证通过后，用 LLM 对比 diff 检查语义偏差

**理由**：
- 受 CodeMender 的 LLM 批评工具启发：用一个 LLM Agent 审查另一个 LLM Agent 的修复
- 当前验证只有 `make test && make lint`，无法检测语义级回归（如修复了 A 但破坏了 B 的隐含契约）
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

**实现**：通过 `ChatOrchestrator` 调用自身 Agent，与 auto-fix 的 Agent 诊断共用 Chat API。

### D5: DynamicRankFactors 接口桥接

**选择**：在 Tools 层定义 `HealthMetricsProvider` 接口，由 Biz 层实现并注入

**理由**：
- `DynamicRankFactors`（Tools 层）需要读取 Skill 健康指标（Biz 层）
- 直接依赖违反"Tools 不依赖 Biz"的分层原则
- 接口桥接是标准解法：Tools 定义接口，Biz 实现，Wire 注入

**接口设计**：

```go
// internal/tools/skillrecommend/health_provider.go
type HealthMetricsProvider interface {
    GetRecentSuccessRate(ctx context.Context, skillID string, days int) (float64, error)
    GetRecentAvgDuration(ctx context.Context, skillID string, days int) (float64, error)
}
```

**Biz 层实现**：`SkillHealthAggregator` 已有 `GetHealthMetrics` 方法，新增适配器即可。

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

**Curator Agent 流程**：

```
SkillIntelligenceUsecase 触发条件判定
（7d 成功率 < 60% 或 同一失败标签出现 >= 5 次）
    │
    ▼
CreateSuggestion (SkillEvolutionSuggestion)
    │
    ▼
Curator Agent (ChatOrchestrator)
    ├─ System Prompt: "你是 Skill 优化专家..."
    ├─ 输入：失败模式 + 历史调用记录 + 现有 Skill 列表
    └─ 输出：Skill 草案 (SKILL.md)
    │
    ▼
Sandbox Runner (codeexecutor.CodeExecutor / E2B)
    ├─ 隔离执行，不影响生产
    └─ 验证：功能正确性 + Token 消耗 + 耗时
    │
    ▼
前端审批 UI → Approve/Reject
    │
    ▼
RegisterApproved → Skill 注册
```

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

**预测逻辑**：

```
PredictiveHealUsecase (Cron Job, 每 5 分钟)
    │
    ├─ 读取系统指标（Provider 延迟、Memory 使用率、Session 堆积）
    ├─ 匹配 FailurePattern 中的前置条件模式
    ├─ 计算预测置信度
    │   ├─ Provider 延迟持续上升 + 历史有 RateLimit 模式 → 预测 RateLimit
    │   └─ Memory 使用率 > 80% + 历史有 OOM 模式 → 预测 OOM
    └─ 预防行动（仅高置信度 > 0.8）
        ├─ 提前切换 Provider
        ├─ 预热 Memory 缓存
        └─ 限流降级
```

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

**挖掘逻辑**：

```
PatternMiningWorker (Cron Job, 每日)
    │
    ├─ 读取 patterns.jsonl（CI 修复记录）
    ├─ 读取 HealRecord（运行时修复记录）
    ├─ 聚类相似失败模式
    │   ├─ 相同 error_code + 相似 stack_trace → 同一模式
    │   └─ 成功修复的 diff 模式提取
    ├─ 生成修复模板
    │   ├─ 写入 failure_pattern 表（source="mined"）
    │   └─ 版本号递增
    └─ 审计机制
        ├─ 新挖掘的规则 confidence = 0.5（低于内置规则的 0.9）
        ├─ 经过 3 次成功验证后 confidence 提升到 0.8
        └─ 每周人工审核 mined 规则，可禁用（is_active=false）
```

## Risks / Trade-offs

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| Critic Agent 增加 LLM 调用成本 | 中 | 仅在 test/lint 通过后触发；每日上限 10 次 |
| 动态挖掘的规则质量不可控 | 高 | 新规则初始 confidence=0.5；3 次成功验证后提升；每周人工审核 |
| 预测性自愈误触发 | 高 | 置信度阈值 0.8 + 冷却期 30min + 严重度分级 + 仅对高严重度执行 |
| Curator Agent Token 成本失控 | 中 | 每日调用上限 20 次；仅对触发条件的 Skill 执行 |
| RootCauseAnalyzer 接口变更影响现有自愈 | 低 | 接口是 `RootCauseEngine` 方法的子集，现有功能不变 |
| failure_pattern 表数据量增长 | 低 | 定期清理 90 天前的低置信度规则；成功/失败计数聚合 |

## Migration Plan

### Phase 1: 闭环加固

1. 抽取 `RootCauseAnalyzer` 接口 + Wire 绑定
2. 定义 `FailureReport` 结构体 + CI 日志解析脚本
3. 新增 `failure_pattern` Ent Schema + Data 层实现
4. 改造 `auto-fix.yml`：结构化日志解析 + Critic Agent 步骤
5. 新增 Cron Job: `failure_pattern_sync`
6. 补齐集成测试
7. 验证：`make api && make wire && make build && make test && make lint`

### Phase 2: Skill Intelligence 落地

1. 扩展 `ExperienceReport` 增加 `RootCauseAnalysis`/`SuggestedFix` 字段
2. 新增 `skill_intelligence_worker` Cron Job
3. 新增 `skill_intelligence.proto` + Service 层实现
4. 新增 `DynamicRankFactors` + `HealthMetricsProvider` 接口桥接
5. 实现 Curator Agent + Sandbox Runner 验证
6. 前端经验报告列表页 + 进化审批 UI
7. Wire DI 装配
8. 验证：全量验证通过

### Phase 3: 自我进化闭环

1. 新增 `PredictiveHealUsecase` + Cron Job
2. 实现 Skill 五阶段进化闭环（Solve→Observe→Evolve→Gate→Reload）
3. Gate 多维验证实现
4. 新增 `PatternMiningWorker` + 审计机制
5. 验证：全量验证通过

### 回滚策略

- 每个 Phase 独立，可单独回滚
- `failure_pattern` 表新增不影响现有表
- `RootCauseAnalyzer` 接口是增量变更，不影响现有 `RootCauseEngine` 使用
- Cron Job 可通过删除注册禁用
- Critic Agent 步骤可通过环境变量 `ENABLE_CRITIC_AGENT=false` 跳过

## Open Questions

1. Critic Agent 的 LLM 模型选择：使用项目已配置的 Provider 还是专用轻量模型？需权衡成本和效果
2. 预测性自愈的指标采集：当前系统指标（Provider 延迟、Memory 使用率）是否已有 Prometheus 指标可读？若无，需新增指标采集
3. `failure_pattern` 表的清理策略：90 天保留期是否合理？是否需要按 source 区分保留期？
4. Skill 进化闭环的 Gate 验证用例来源：由 Curator Agent 自动生成测试用例，还是复用已有测试？
