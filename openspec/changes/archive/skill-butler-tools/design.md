# 技能管家工具集 详细设计文档

> 日期：2026-06-02
> 状态：Draft
> 前置文档：`memory-skills-butler/design.md`（技能管家完整设计）、`system-builtin-agents/proposal.md`（管家体系总览）

***

## 一、工具定义

### 1.1 evolve_skill

**功能**：基于失败模式分析优化 Skill body，创建新版本（pending review 状态）。

**学术原则**：P-O1（编排进化原则）——编排策略应基于历史执行数据动态调整。

**输入/输出**：

```go
type EvolveSkillInput struct {
    SkillID         string   `json:"skill_id" jsonschema:"description=待进化的 Skill ID"`
    FailurePatterns []string `json:"failure_patterns" jsonschema:"description=失败模式描述，为空则自动从调用记录分析"`
}

type EvolveSkillOutput struct {
    NewVersion   string `json:"new_version"`
    DiffPreview  string `json:"diff_preview"`
    Status       string `json:"status"`
    Analysis     string `json:"analysis"`
}
```

**核心逻辑**：

```
1. 加载 Skill 当前版本（通过 SkillUsecase.Get 获取 body + tags + description）
2. 收集失败案例：
   → 如果 FailurePatterns 非空，使用用户提供的失败模式
   → 如果 FailurePatterns 为空，从 SkillQueryReader.SearchSkillInvocations 查询 status=failure 的调用记录
   → 提取失败案例的 input_preview + error_message
3. LLM 分析失败原因并生成优化方案：
   → 调用 provider.TRPCModelForProviderModel 获取 LLM 实例
   → 使用失败分析 prompt（见 §3.1）
   → 解析 LLM 返回的 JSON（failure_analysis、optimized_body、changes、confidence）
4. 创建新版本：
   → 调用 SkillUsecase.Create 创建新 Skill（slug 加 "_v2" 后缀，名称加" (优化版)"）
   → 调用 SkillUsecase.ToggleEnabled(id, false) 标记为 pending review
5. 生成 diff preview（当前 body vs 优化 body 的对比摘要）
6. 返回结果
```

### 1.2 optimize_skill

**功能**：分析工具调用权重，生成调整建议。不自动执行，需用户确认。

**学术原则**：P-O4（能力画像原则）+ P-O5（成本感知原则）。

**输入/输出**：

```go
type OptimizeSkillInput struct {
    AgentID string `json:"agent_id" jsonschema:"description=Agent ID，为空则分析全局"`
}

type OptimizeSkillOutput struct {
    Tools         []ToolWeightReport `json:"tools"`
    Suggestions   []ToolSuggestion   `json:"suggestions"`
    Summary       string             `json:"summary"`
}

type ToolWeightReport struct {
    ToolKey        string  `json:"tool_key"`
    CallCount      int     `json:"call_count"`
    SuccessRate    float64 `json:"success_rate"`
    AvgDurationMS  float64 `json:"avg_duration_ms"`
    WeightScore    float64 `json:"weight_score"`
    Recommendation string  `json:"recommendation"`
}

type ToolSuggestion struct {
    ToolKey     string  `json:"tool_key"`
    Action      string  `json:"action"`
    Reason      string  `json:"reason"`
    Confidence  float64 `json:"confidence"`
}
```

**权重计算公式**：

```
WeightScore = normalize(success_rate) * 0.5
            + normalize(call_count) * 0.3
            + normalize(1/duration) * 0.2
```

**Recommendation 判定规则**：

| 条件 | Recommendation |
|------|---------------|
| WeightScore > 0.7 且 success_rate > 0.8 | promote |
| WeightScore < 0.3 或 success_rate < 0.5 | demote |
| 其他 | keep |

**核心逻辑**：

```
1. 从 EvolutionMetricsRepo.GetToolSuccessRate 获取整体成功率
2. 从 ToolInvocationReader（biz.ToolInvocationReader）获取按工具分组的调用明细
3. 聚合计算每个工具的 call_count、success_rate、avg_duration_ms
4. 计算 WeightScore 和 Recommendation
5. LLM 生成优化建议（哪些工具应提升权重、哪些应降级、原因分析）
6. 返回报告 + 建议
```

### 1.3 recommend_skills

**功能**：基于任务描述推荐 Skill 组合。

**学术原则**：P-O4（能力画像原则）——维护每个 Agent 的能力画像，基于历史表现动态更新。

**输入/输出**：

```go
type RecommendSkillsInput struct {
    TaskDescription string `json:"task_description" jsonschema:"description=任务描述"`
    TopK            int    `json:"top_k" jsonschema:"description=返回数量，默认5"`
    AgentID         string `json:"agent_id,omitempty" jsonschema:"description=限定 Agent 范围，为空则全局"`
}

type RecommendSkillsOutput struct {
    Recommendations []SkillRecommendation `json:"recommendations"`
}

type SkillRecommendation struct {
    SkillID    string  `json:"skill_id"`
    Name       string  `json:"name"`
    Slug       string  `json:"slug"`
    Score      float64 `json:"score"`
    Reason     string  `json:"reason"`
}
```

**核心逻辑**：

```
1. 获取候选 Skill 列表（通过 SkillUsecase.List，过滤 enabled=true）
2. 构建 RuntimeCandidate 列表（从 Skill 的 embedding 字段）
3. 调用 SkillUsecase.ScoreByEmbedding(query, candidates) 计算相似度
4. 按 Score 降序排列，取 TopK
5. 为每个推荐生成 Reason（基于 tags 匹配度 + 相似度）
6. 返回推荐列表
```

### 1.4 analyze_skill_usage

**功能**：分析 Skill 调用频率、成功率、趋势，输出健康度报告。

**输入/输出**：

```go
type AnalyzeSkillUsageInput struct {
    SkillID  string `json:"skill_id,omitempty" jsonschema:"description=Skill ID，为空则分析全部"`
    TimeRange string `json:"time_range" jsonschema:"description=时间范围:7d/30d/90d，默认30d"`
}

type AnalyzeSkillUsageOutput struct {
    Skills []SkillHealthReport `json:"skills"`
}

type SkillHealthReport struct {
    SkillID       string  `json:"skill_id"`
    Name          string  `json:"name"`
    InvokeCount7d int     `json:"invoke_count_7d"`
    SuccessRate   float64 `json:"success_rate"`
    AvgDurationMS float64 `json:"avg_duration_ms"`
    Trend         string  `json:"trend"`
    HealthStatus  string  `json:"health_status"`
    Recommendation string `json:"recommendation"`
}
```

**健康度判定规则**（对齐 `memory-skills-butler` design §4.4）：

| 条件 | HealthStatus | Recommendation |
|------|-------------|----------------|
| 调用次数 > 10/周 且 成功率 > 80% | healthy | keep |
| 调用次数 > 5/周 且 成功率 60-80% | warning | evolve |
| 调用次数 < 2/周 或 成功率 < 60% | critical | evolve/retire |
| 30天无调用 | dormant | retire |

**趋势判定**：比较最近 7 天与之前 7 天的调用次数变化率。

| 变化率 | Trend |
|--------|-------|
| > +20% | rising |
| -20% ~ +20% | stable |
| < -20% | declining |
| 调用次数 = 0 | dormant |

**核心逻辑**：

```
1. 从 SkillQueryReader.SearchSkillInvocations 查询调用记录
2. 从 EvolutionMetricsRepo 获取成功率趋势
3. 按 Skill 聚合：调用次数、成功率、平均耗时
4. 计算趋势和健康度
5. 返回报告
```

***

## 二、与 SkillEvolutionUsecase 的集成

### 2.1 当前 EvolutionUsecase 能力

现有 `EvolutionUsecase`（`internal/biz/evolution.go`）提供：

| 方法 | 功能 | 本工具集使用方式 |
|------|------|----------------|
| `GetEvolutionMetrics` | 获取工具成功率、检索质量趋势 | `optimize_skill` 和 `analyze_skill_usage` 复用 |
| `GetEvolutionSuggestions` | 列出进化建议 | `evolve_skill` 可参考已有建议 |
| `ApplySuggestion` | 应用 persona/prompt 类型建议 | `evolve_skill` 创建新版本后可关联建议 |
| `RejectSuggestion` | 拒绝建议 | 不直接使用 |

### 2.2 不新增 SkillEvolutionUsecase

本变更**不新增** `SkillEvolutionUsecase`。工具直接组合现有 Usecase：

```
evolve_skill:
  → SkillUsecase.Get（加载当前 Skill）
  → SkillQueryReader.SearchSkillInvocations（查询失败案例）
  → provider.TRPCModelForProviderModel + LLM（分析+优化）
  → SkillUsecase.Create（创建新版本）
  → SkillUsecase.ToggleEnabled（标记 pending review）

optimize_skill:
  → EvolutionMetricsRepo.GetToolSuccessRate（整体成功率）
  → biz.ToolInvocationReader（调用明细）
  → provider.TRPCModelForProviderModel + LLM（生成建议）

recommend_skills:
  → SkillUsecase.List（获取候选列表）
  → SkillUsecase.ScoreByEmbedding（计算相似度）

analyze_skill_usage:
  → SkillQueryReader.SearchSkillInvocations（调用记录）
  → EvolutionMetricsRepo.GetToolSuccessRate（成功率趋势）
```

### 2.3 未来演进

当 `memory-skills-butler` 变更实施 `ExperienceAnalyticsUsecase` 后，`optimize_skill` 和 `analyze_skill_usage` 可迁移到调用 `ExperienceAnalyticsUsecase.AnalyzeToolWeights()` 和 `AnalyzeSkillHealth()`，替换当前直接查询底层 Repo 的方式。

***

## 三、工具注册方式

### 3.1 包结构

```
internal/tools/skills_butler/
  ├── registry.go              # Deps + RegisterAll + IsSkillsButlerAllowed
  ├── evolve_skill.go          # evolve_skill 工具
  ├── optimize_skill.go        # optimize_skill 工具
  ├── recommend_skills.go      # recommend_skills 工具
  └── analyze_skill_usage.go   # analyze_skill_usage 工具
```

### 3.2 Deps 定义

```go
package skills_butler

type Deps struct {
    SkillUC         SkillUsecasePort
    SkillQuery      SkillQueryPort
    EvolutionRepo   EvolutionMetricsPort
    ToolInvReader   ToolInvocationPort
    ProviderCatalog ProviderCatalogPort
    RoundTrip       RoundTripPort
    ProviderCode    string
    ModelAPIID      string
}
```

**端口接口定义**（在 `registry.go` 中，避免循环导入）：

```go
type SkillUsecasePort interface {
    Get(ctx context.Context, id string) (SkillDetail, error)
    Create(ctx context.Context, in SkillCreateInput) (SkillDetail, error)
    ToggleEnabled(ctx context.Context, id string, enabled bool) (SkillDetail, error)
    List(ctx context.Context, q SkillListInput) (SkillListResult, error)
    ScoreByEmbedding(ctx context.Context, query string, candidates []EmbeddingCandidate) (map[string]float64, error)
}

type SkillQueryPort interface {
    SearchSkillInvocations(ctx context.Context, q InvocationQuery) (InvocationResult, error)
}

type EvolutionMetricsPort interface {
    GetToolSuccessRate(ctx context.Context, agentID string, since time.Time) (float64, []MetricPoint, error)
}

type ToolInvocationPort interface {
    SearchToolInvocations(ctx context.Context, q ToolInvQuery) ([]ToolInvSummary, error)
}

type ProviderCatalogPort interface{}

type RoundTripPort interface{}
```

### 3.3 RegisterAll

```go
func RegisterAll(deps Deps) []trpctool.Tool {
    return []trpctool.Tool{
        newEvolveSkillTool(deps),
        newOptimizeSkillTool(deps),
        newRecommendSkillsTool(deps),
        newAnalyzeSkillUsageTool(deps),
    }
}

func IsSkillsButlerAllowed(agentKey string) bool {
    return agentKey == "__skills__"
}
```

### 3.4 服务层注入

在 `internal/service/cli_admin_tools.go` 中新增：

```go
func (o *ChatOrchestrator) skillsButlerTools(ctx context.Context, ag biz.Agent) []trpctool.Tool {
    if o == nil || !skills_butler.IsSkillsButlerAllowed(strings.TrimSpace(ag.AgentKey)) {
        return nil
    }
    return skills_butler.RegisterAll(skills_butler.Deps{
        SkillUC:         skillsButlerSkillUC{uc: o.td.Catalog.SkillUC},
        SkillQuery:      skillsButlerSkillQuery{repo: o.td.Catalog.SkillQueryReader},
        EvolutionRepo:   skillsButlerEvolutionRepo{repo: o.td.Catalog.EvolutionRepo},
        ToolInvReader:   skillsButlerToolInvReader{reader: o.td.Catalog.ToolInvReader},
        ProviderCatalog: o.td.Catalog.ProviderCatalog,
        RoundTrip:       o.td.RT,
        ProviderCode:    ag.ProviderCode,
        ModelAPIID:      ag.ModelAPIID,
    })
}
```

注入路径对齐现有 `cliAdminTools` 模式：在 `chat_orchestrator_turn.go` 的 `CustomTools` 字段中追加 `o.skillsButlerTools(ctx, ag)`。

### 3.5 工具构建方式

所有工具使用 `trpcfunction.NewFunctionTool[I, O]` 构建，与现有 `spirit_tools.go` 和 `cli_admin/registry.go` 保持一致：

```go
func newEvolveSkillTool(deps Deps) *trpcfunction.FunctionTool[EvolveSkillInput, EvolveSkillOutput] {
    return trpcfunction.NewFunctionTool(
        func(ctx context.Context, input EvolveSkillInput) (EvolveSkillOutput, error) {
            // 实现逻辑
        },
        trpcfunction.WithName("evolve_skill"),
        trpcfunction.WithDescription("基于失败模式分析优化 Skill。分析失败案例，生成优化后的 Skill body，创建新版本（需用户确认后启用）。"),
    )
}
```

***

## 四、LLM 调用设计

### 4.1 evolve_skill 失败分析 Prompt

```
你是一个技能优化专家。请分析以下 Skill 的失败模式，并生成优化方案。

## Skill 信息
- 名称：{skill_name}
- 当前 body：{skill_body}
- 描述：{skill_description}
- 标签：{skill_tags}

## 失败案例
{failure_cases}

## 分析要求
1. 识别失败的根本原因（prompt 不精确？参数缺失？边界条件？）
2. 生成优化后的 body
3. 说明修改了什么以及为什么

## 输出格式
```json
{
  "failure_analysis": "失败原因分析",
  "optimized_body": "优化后的 body",
  "changes": ["修改1", "修改2"],
  "confidence": 0.8
}
```
```

### 4.2 optimize_skill 建议生成 Prompt

```
你是一个工具优化专家。请基于以下工具使用数据，生成工具权重调整建议。

## 工具使用数据
{tool_weight_reports}

## 分析要求
1. 识别哪些工具使用频率高但成功率低（需要优化）
2. 识别哪些工具使用频率低但成功率低（考虑降级）
3. 识别哪些工具使用频率高且成功率高（应提升权重）
4. 给出具体的调整建议和理由

## 输出格式
```json
{
  "suggestions": [
    {"tool_key": "...", "action": "promote/demote/keep", "reason": "...", "confidence": 0.8}
  ],
  "summary": "整体分析摘要"
}
```
```

### 4.3 LLM 调用方式

对齐 `memory-skills-butler` design §10.2 的修正方案：

```go
func callLLM(ctx context.Context, deps Deps, prompt string) (string, error) {
    model, err := provider.TRPCModelForProviderModel(
        ctx, deps.ProviderCatalog, deps.RoundTrip,
        deps.ProviderCode, deps.ModelAPIID,
    )
    if err != nil {
        return "", err
    }
    resp, err := model.Generate(ctx, []*trpcmodel.Content{
        {Role: "user", Parts: []trpcmodel.Part{{Text: prompt}}},
    })
    if err != nil {
        return "", err
    }
    return resp.Text(), nil
}
```

Deps 中 `ProviderCatalog` 和 `RoundTrip` 的实际类型为 `*biz.LlmProviderModelUsecase` 和 `*provider.RoundTrip`，在服务层注入时传入。

***

## 五、数据流

### 5.1 evolve_skill 数据流

```
Agent 调用 evolve_skill(skill_id, failure_patterns)
  → SkillUsecase.Get(skill_id) → 获取当前 Skill body/tags/description
  → SkillQueryReader.SearchSkillInvocations(status=failure) → 获取失败案例
  → callLLM(失败分析 prompt) → 生成优化方案 JSON
  → SkillUsecase.Create(slug+"_v2", optimized_body) → 创建新版本
  → SkillUsecase.ToggleEnabled(new_id, false) → 标记 pending review
  → 返回 EvolveSkillOutput{new_version, diff_preview, status="pending_review"}
```

### 5.2 optimize_skill 数据流

```
Agent 调用 optimize_skill(agent_id)
  → EvolutionMetricsRepo.GetToolSuccessRate(agent_id, since) → 整体成功率
  → ToolInvocationReader.SearchToolInvocations(since=30d) → 调用明细
  → 聚合计算 WeightScore + Recommendation
  → callLLM(建议生成 prompt) → 生成调整建议
  → 返回 OptimizeSkillOutput{tools, suggestions, summary}
```

### 5.3 recommend_skills 数据流

```
Agent 调用 recommend_skills(task_description, top_k)
  → SkillUsecase.List(enabled=true) → 获取候选 Skill 列表
  → 构建 EmbeddingCandidate 列表
  → SkillUsecase.ScoreByEmbedding(task_description, candidates) → 计算相似度
  → 按 Score 降序排列，取 TopK
  → 返回 RecommendSkillsOutput{recommendations}
```

### 5.4 analyze_skill_usage 数据流

```
Agent 调用 analyze_skill_usage(skill_id, time_range)
  → SkillQueryReader.SearchSkillInvocations(since=7d) → 调用记录
  → EvolutionMetricsRepo.GetToolSuccessRate(since) → 成功率趋势
  → 按 Skill 聚合：调用次数、成功率、平均耗时
  → 计算趋势（rising/stable/declining/dormant）
  → 判定健康度（healthy/warning/critical/dormant）
  → 返回 AnalyzeSkillUsageOutput{skills}
```

***

## 六、错误处理

| 错误场景 | 处理方式 |
|----------|----------|
| Skill 不存在 | 返回 BadRequest 错误 |
| 无失败案例可分析 | 返回提示信息，建议先积累使用数据 |
| LLM 调用失败 | 返回 InternalServer 错误，包含原始错误信息 |
| LLM 返回格式错误 | 尝试 JSON 解析，失败则返回原始文本作为 analysis |
| 创建新版本失败 | 返回 InternalServer 错误 |
| 无调用数据 | 返回空报告，提示数据不足 |
| ScoreByEmbedding 失败 | 返回 InternalServer 错误 |

***

## 七、新增文件清单

```
internal/tools/skills_butler/
  ├── registry.go              # Deps + 端口接口 + RegisterAll + IsSkillsButlerAllowed
  ├── evolve_skill.go          # evolve_skill 工具实现
  ├── optimize_skill.go        # optimize_skill 工具实现
  ├── recommend_skills.go      # recommend_skills 工具实现
  └── analyze_skill_usage.go   # analyze_skill_usage 工具实现

internal/service/
  └── cli_admin_tools.go       # 新增 skillsButlerTools() + 适配器类型
  └── chat_orchestrator_turn.go # CustomTools 追加 skillsButlerTools 调用
```
