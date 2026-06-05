# M60: Spirit Parallel Orchestrator — 深度业务实现详细设计

> **版本**：2026-06-06
> **定位**：基于需求文档 [60-spirit-parallel-orchestrator.md](./60-spirit-parallel-orchestrator.md) 和现有骨架代码，分析每个验收标准的实现差距，给出深度业务实现的详细设计方案。
> **前置**：P1 + P2 骨架已完成，Wire 注入链已修复，集成问题已修复。
> **实现状态**：✅ P0/P1/P2 差距已全部修复 + 深度架构审查修复已完成（2026-06-06）

---

## 一、差距总览

### 1.1 验收标准差距矩阵

| 验收 ID | 摘要 | 骨架状态 | 差距级别 | 差距描述 |
|---------|------|---------|---------|---------|
| SPO-01 | 同一精灵 Session 支持多团队并行 | ✅ 骨架完成 | **P0** | 团队创建后不自动启动 Runner，处于"悬停"状态 |
| SPO-02 | 并行度可配置，超限拒绝 | ✅ 骨架完成 | P2 | TeamTimeout/AutoArchiveAfter/MaxSessionDepth 未实际使用 |
| SPO-03 | 团队进度实时监控 + 精灵主动通知 | ⚠️ 部分完成 | **P1** | 进度只有 0%/100%，无中间进度；CurrentStep 始终为空 |
| SPO-04 | 取消团队 + 释放配额 | ✅ 骨架完成 | — | 无差距 |
| SPO-05 | Task DAG 依赖调度 | ⚠️ 部分完成 | **P0** | 依赖团队激活后不自动启动 Runner；DependencyScheduler 死代码 |
| SPO-06 | 拓扑路由自动选择编排模式 | ✅ 骨架完成 | P1 | RouteTopology 简化版，computeDepth/computeWidth 未实现 |
| SPO-07 | Synthesis Engine 结果合成 | ⚠️ 部分完成 | **P0** | Summary/KeyFindings 始终为空；部分失败场景未处理 |
| SPO-08 | DQ Score 驱动编排缓存 | ⚠️ 部分完成 | **P1** | 缓存仅内存存储重启丢失；DQ Score 仅时间惩罚 |
| SPO-09 | 编排策略进化闭环 | ⚠️ 部分完成 | **P1** | DQ<0.5 优化建议未生成；进化护栏未接入 |

### 1.2 差距分级定义

| 级别 | 含义 | 影响 |
|------|------|------|
| **P0** | 业务断裂 — 核心流程无法走通 | 用户无法获得预期功能 |
| **P1** | 验收不通过 — 功能存在但不满足验收标准 | 验收失败 |
| **P2** | 功能不完善 — 有定义但未实际使用 | 用户体验不完整 |

---

## 二、P0 问题详细设计

### 2.1 P0-01：团队创建后无自动启动 Runner

**现状**：`AssembleTeam` 只创建 Team + Session 记录，不触发 Runner 执行。团队处于 `"active"` 状态但无实际运行。

**根因**：精灵 Agent 的 `assemble_team` 工具返回 `team_id` 和 `session_id`，但没有任何代码向团队 session 发送初始消息来触发 `RunTurnFromInput`。

**设计方案**：在 `SpiritTeamAssembler.AssembleTeam` 完成后，自动向团队 session 发送初始消息触发 Runner 执行。

#### 2.1.1 新增 `start_team` 精灵工具

```
工具名：start_team
输入：{ team_id: string, initial_message: string }
输出：{ team_id: string, status: string, session_id: string }
职责：向指定团队的 session 发送初始消息，触发 Runner 执行
```

**实现位置**：`internal/tools/spirit_tools.go`

```go
type StartTeamInput struct {
    TeamID         string `json:"team_id" jsonschema:"description=要启动的团队 ID"`
    InitialMessage string `json:"initial_message" jsonschema:"description=发送给团队的初始任务消息"`
}

type StartTeamOutput struct {
    TeamID    string `json:"team_id"`
    Status    string `json:"status"`
    SessionID string `json:"session_id"`
}
```

**端口扩展**：`SpiritTeamControllerPort` 新增方法：

```go
type SpiritTeamControllerPort interface {
    CancelTeam(ctx context.Context, teamID string) error
    CheckTeamProgress(ctx context.Context, spiritSessionID string) ([]biz.TeamProgress, error)
    StartTeam(ctx context.Context, teamID string, initialMessage string) (biz.Session, error)
}
```

#### 2.1.2 `SpiritTeamAssembler.StartTeam` 实现

**实现位置**：`internal/service/spirit_team.go`

```go
func (a *SpiritTeamAssembler) StartTeam(ctx context.Context, teamID string, initialMessage string) (biz.Session, error) {
    team, err := a.spiritUC.GetTeam(ctx, teamID)
    if err != nil {
        return biz.Session{}, err
    }
    if team.Status != "active" && team.Status != "assembled" {
        return biz.Session{}, kerrors.BadRequest("SPIRIT", "team must be active or assembled to start")
    }
    sessions, err := a.sessionUC.ListByTeamID(ctx, teamID)
    if err != nil || len(sessions) == 0 {
        return biz.Session{}, kerrors.NotFound("SPIRIT", "team session not found")
    }
    session := sessions[0]
    // 通过 ChatOrchestrator 发送消息触发 Runner
    // 需要 ChatOrchestrator 暴露一个内部方法
    return session, nil
}
```

**关键问题**：`StartTeam` 需要调用 `ChatOrchestrator.Execute` 来发送消息触发 Runner，但 `SpiritTeamAssembler` 不应直接依赖 `ChatOrchestrator`（循环依赖风险）。

**解决方案**：定义 `TeamStarterPort` 接口在 biz 层，由 `ChatOrchestrator` 实现：

```go
// internal/biz/spirit_team_usecase.go
type TeamStarterPort interface {
    StartTeamTurn(ctx context.Context, sessionID string, content string) error
}
```

`ChatOrchestrator` 实现此接口，内部调用 `executeTeamTurnViaHooks`。

**Wire 绑定**：在 `cmd/admin/wire.go` 中将 `ChatOrchestrator` 作为 `TeamStarterPort` 注入 `SpiritTeamAssembler`。

#### 2.1.3 自动启动策略

**方案 A（推荐）**：`assemble_team` 工具返回后，精灵 Agent 自行决定是否调用 `start_team`。

- 优点：精灵 Agent 有控制权，可以在启动前做额外配置
- 缺点：依赖 LLM 正确调用 `start_team`，可能遗漏

**方案 B**：`assemble_team` 内部自动调用 `StartTeam`。

- 优点：流程简洁，不依赖 LLM 二次调用
- 缺点：无法在启动前做额外配置

**建议**：采用方案 B + 可选 `initial_message` 参数。`assemble_team` 的 `TaskPrompt` 字段自动作为初始消息发送给团队。

```go
// assemble_team 工具内部，AssembleTeam 成功后：
if params.TaskDescription != "" {
    starter.StartTeamTurn(ctx, session.ID, params.TaskDescription)
}
```

#### 2.1.4 依赖团队激活后自动启动

`scheduleDependentTeams` 在激活团队时，同步触发 Runner：

```go
// team_turn_hooks.go scheduleDependentTeams 中，Update 成功后：
if o.teamStarter != nil {
    sessions, _ := o.sessionUC.ListByTeamID(ctx, t.ID)
    if len(sessions) > 0 {
        go safego.Go(func() {
            o.teamStarter.StartTeamTurn(context.WithoutCancel(ctx), sessions[0].ID, t.TaskDescription)
        })
    }
}
```

---

### 2.2 P0-02：SynthesisResult 的 Summary/KeyFindings 始终为空

**现状**：`SpiritSynthesisService.SynthesizeResults` 构建 `TeamSynthesisResult` 时只填了 `TeamID/TeamName/TaskName/Status`，`Summary` 和 `KeyFindings` 始终为空。

**根因**：没有从团队执行结果中提取摘要和关键发现。

**设计方案**：从团队 Session 的最后一条 Assistant 消息中提取 Summary 和 KeyFindings。

#### 2.2.1 新增 `extractTeamOutput` 方法

**实现位置**：`internal/biz/spirit_team_usecase.go`

```go
func (u *SpiritTeamUsecase) ExtractTeamOutput(ctx context.Context, teamID string) (summary string, keyFindings string, err error) {
    sessions, err := u.sessionUC.ListByTeamID(ctx, teamID)
    if err != nil || len(sessions) == 0 {
        return "", "", nil
    }
    // 查询团队 session 的最后一条 assistant 消息
    messages, err := u.sessionUC.ListMessages(ctx, sessions[0].ID, 1)
    if err != nil || len(messages) == 0 {
        return "", "", nil
    }
    // 从最后一条 assistant 消息中提取
    for i := len(messages) - 1; i >= 0; i-- {
        if messages[i].Role == "assistant" {
            content := messages[i].ContentMarkdown
            summary = TruncateRunes(content, 500)
            keyFindings = extractKeyFindings(content)
            return summary, keyFindings, nil
        }
    }
    return "", "", nil
}
```

#### 2.2.2 `extractKeyFindings` 简单提取

```go
func extractKeyFindings(content string) string {
    // 提取 markdown 中的关键行（以 - / * / 1. / > 开头的行）
    var findings []string
    for _, line := range strings.Split(content, "\n") {
        trimmed := strings.TrimSpace(line)
        if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") ||
           strings.HasPrefix(trimmed, "> ") || regexp.MustCompile(`^\d+\.\s`).MatchString(trimmed) {
            if len(findings) < 5 {
                findings = append(findings, trimmed)
            }
        }
    }
    return strings.Join(findings, "\n")
}
```

#### 2.2.3 SynthesisService 集成

```go
// SpiritSynthesisService.SynthesizeResults 中：
for _, t := range teams {
    summary, keyFindings, _ := s.spiritUC.ExtractTeamOutput(ctx, t.ID)
    teamResults = append(teamResults, biz.TeamSynthesisResult{
        TeamID:      t.ID,
        TeamName:    t.DisplayName,
        TaskName:    t.TaskDescription,
        Status:      t.Status,
        Summary:     summary,
        KeyFindings: keyFindings,
    })
}
```

#### 2.2.4 部分失败合成

当前只合成 `completed` 的团队，`failed` 的被排除。需改为包含 `failed` 团队但标注原因：

```go
// 改为同时包含 completed 和 failed
for _, t := range teams {
    if t.Status != "completed" && t.Status != "failed" {
        continue
    }
    summary, keyFindings, _ := s.spiritUC.ExtractTeamOutput(ctx, t.ID)
    if t.Status == "failed" {
        summary = "[执行失败] " + summary
    }
    teamResults = append(teamResults, biz.TeamSynthesisResult{...})
}
```

**SynthesisEngine.inferStrategy** 同步更新：

```go
func (e *SynthesisEngine) inferStrategy(input SynthesisInput) SynthesisStrategy {
    if input.Template != "" {
        return SynthesisStrategyTemplate
    }
    hasFailed := false
    completedCount := 0
    for _, r := range input.TeamResults {
        if r.Status == "completed" {
            completedCount++
        }
        if r.Status == "failed" {
            hasFailed = true
        }
    }
    if hasFailed {
        return SynthesisStrategyHybrid
    }
    if completedCount == len(input.TeamResults) && len(input.TeamResults) <= 3 {
        return SynthesisStrategyTemplate
    }
    return SynthesisStrategyHybrid
}
```

**级联标注**（依赖链中断场景）：当 failed 团队的下游团队被阻塞时，在合成结果中标注：

```go
// 在 SynthesisService 中，检查 failed 团队的下游
for _, t := range teams {
    if t.Status == "failed" && t.DagNodeID != "" {
        for _, other := range teams {
            if containsString(other.DependsOn, t.DagNodeID) && other.Status == "waiting_deps" {
                teamResults = append(teamResults, biz.TeamSynthesisResult{
                    TeamID:   other.ID,
                    TeamName: other.DisplayName,
                    TaskName: other.TaskDescription,
                    Status:   "blocked",
                    Summary:  fmt.Sprintf("被失败团队 %s 阻塞", t.DisplayName),
                })
            }
        }
    }
}
```

---

### 2.3 P0-03：DependencyScheduler 死代码清理

**现状**：`DependencyScheduler`（`spirit_dependency_scheduler.go`）定义了完整的调度逻辑，但没有任何调用者。实际调度由 `team_turn_hooks.go` 中的 `scheduleDependentTeams` 实现。

**设计方案**：删除 `DependencyScheduler`，将其中有价值的逻辑合并到 `TaskDAG` 和 `scheduleDependentTeams` 中。

#### 2.3.1 保留 `TeamAssemblerPort` 接口

`TeamAssemblerPort` 被 `DependencyScheduler` 定义但实际未使用。检查是否有其他引用：

- 如果无引用 → 删除
- 如果有引用 → 移到 `spirit_team_usecase.go`（更符合 biz 层规范）

#### 2.3.2 删除文件

删除 `internal/biz/spirit_dependency_scheduler.go`，确认 `DependencyScheduler` 和 `TeamAssemblerPort` 无其他引用。

---

## 三、P1 问题详细设计

### 3.1 P1-01：团队进度只有 0%/100%，无中间进度

**现状**：`CheckTeamProgress` 只检查 Run 的 status，`success` 则 100%，否则 0%。`CurrentStep` 始终为空。

**设计方案**：基于团队 Session 的消息数量和工具调用进度计算中间进度。

#### 3.1.1 进度计算算法

```
进度 = (已完成步骤数 / 预估总步骤数) * 100

已完成步骤数 = 团队 session 中的 assistant 消息数
预估总步骤数 = max(已完成步骤数 + 1, ParallelConfig.MaxTeamConcurrency * 2)
```

更精确的方案：基于团队 Run 的 TurnInput/TurnResult 统计：

```go
func (u *SpiritTeamUsecase) CheckTeamProgress(ctx context.Context, spiritSessionID string) ([]TeamProgress, error) {
    // ... 查询所有团队 ...
    for i := range teams {
        tp := TeamProgress{...}
        runs, _ := u.teamUC.ListRuns(ctx, teams[i].ID, 10)
        if len(runs) > 0 {
            totalRuns := len(runs)
            completedRuns := 0
            for _, r := range runs {
                if r.Status == "success" {
                    completedRuns++
                }
                tp.DurationMs += int64(r.DurationMS)
            }
            // 进度 = 已完成 Run 数 / 总 Run 数
            if totalRuns > 0 {
                tp.ProgressPct = float64(completedRuns) / float64(totalRuns) * 100
            }
            // 当前步骤
            if teams[i].Status == "active" {
                tp.CurrentStep = fmt.Sprintf("执行中 (已完成 %d/%d 轮)", completedRuns, totalRuns)
            }
        }
        if teams[i].Status == "completed" {
            tp.ProgressPct = 100
            tp.CurrentStep = "已完成"
        }
        if teams[i].Status == "waiting_deps" {
            tp.ProgressPct = 0
            tp.CurrentStep = "等待依赖完成"
        }
        out = append(out, tp)
    }
    return out, nil
}
```

#### 3.1.2 实时进度事件

`spirit_team_progress` 事件当前只在 `scheduleDependentTeams` 和 `CancelTeam` 中发布。需在团队 Turn 完成时也发布：

```go
// team_turn_hooks.go executeTeamTurnViaHooks 中，RunTurnFromInput 完成后：
if sess.ParentSessionID != "" && strings.TrimSpace(sess.TeamID) != "" {
    // 发布进度更新事件
    progress := computeTeamProgress(ctx, o.team.TeamUC, sess.TeamID)
    env := event.NewEnvelope(event.EnvelopeTypeSpiritTeamProgress, "team-turn-hooks", sess.ParentSessionID)
    env.TeamID = strings.TrimSpace(sess.TeamID)
    env.Metadata = map[string]any{
        "team_id":      sess.TeamID,
        "progress_pct": progress.ProgressPct,
        "current_step": progress.CurrentStep,
    }
    o.td.Pipeline.Bus.Publish(ctx, env)
}
```

---

### 3.2 P1-02：团队列表排序未实现

**现状**：前端 `teams` 列表按添加顺序排列，未按状态排序。

**需求**：US-01 要求按状态排序：running → waiting → completed → failed。

**设计方案**：在前端 Store 中添加排序 computed。

```typescript
// stores/spirit/index.ts
const sortedTeams = computed(() => {
  const statusOrder: Record<string, number> = {
    running: 0, assembled: 0, assembling: 0,
    waiting_deps: 1,
    completed: 2,
    failed: 3,
    cancelled: 4,
  };
  return [...teams.value].sort((a, b) => {
    const orderA = statusOrder[a.status] ?? 99;
    const orderB = statusOrder[b.status] ?? 99;
    return orderA - orderB;
  });
});
```

---

### 3.3 P1-03：依赖图文本展示未实现

**现状**：US-04 要求"精灵回复中展示任务依赖图（文本形式）"，但当前 `assemble_team` 工具返回的 `AssembleTeamOutput` 不包含 DAG 文本表示。

**设计方案**：在 `TaskDAG` 上新增 `ToTextDiagram()` 方法，返回文本形式的依赖图。

#### 3.3.1 `TaskDAG.ToTextDiagram()`

```go
func (d *TaskDAG) ToTextDiagram() string {
    if d == nil || len(d.Nodes) == 0 {
        return ""
    }
    var sb strings.Builder
    sb.WriteString("📋 任务依赖图：\n")
    for _, node := range d.OrderedNodes() {
        prefix := "  "
        if len(node.DependsOn) == 0 {
            prefix = "▶ "
        } else {
            prefix = "⏳ "
        }
        sb.WriteString(fmt.Sprintf("%s%s: %s", prefix, node.ID, node.Description))
        if len(node.DependsOn) > 0 {
            sb.WriteString(fmt.Sprintf(" (依赖: %s)", strings.Join(taskNodeIDsToStrings(node.DependsOn), ", ")))
        }
        sb.WriteString("\n")
    }
    return sb.String()
}
```

#### 3.3.2 `assemble_team` 工具返回 DAG 文本

当 `TaskDAGJSON` 非空时，在 `AssembleTeamOutput` 中包含 DAG 文本：

```go
type AssembleTeamOutput struct {
    TeamID         string `json:"team_id"`
    SessionID      string `json:"session_id"`
    TeamName       string `json:"team_name"`
    TopologyReason string `json:"topology_reason,omitempty"`
    DAGDiagram     string `json:"dag_diagram,omitempty"`
}
```

在 `assembleDAGTeams` 返回时附带 DAG 文本。

---

### 3.4 P1-04：OrchestrationCache 持久化

**现状**：`OrchestrationCache` 使用内存 `map[string]*OrchestrationCacheEntry` 存储，服务重启后丢失。

**需求**：设计文档指定存储在 `AgentRuntimeSettings.ExtraJSON` 中 `orchestration_cache` 键。

**设计方案**：在 `OrchestrationCache` 初始化时从 `AgentRuntimeSettings` 加载，每次 `Put` 时持久化。

#### 3.4.1 新增 `OrchestrationCacheRepo` 接口

```go
// internal/biz/spirit_orchestration_cache.go
type OrchestrationCacheRepo interface {
    LoadCacheJSON(ctx context.Context) (string, error)
    SaveCacheJSON(ctx context.Context, jsonStr string) error
}
```

#### 3.4.2 Data 层实现

```go
// internal/data/orchestration_cache_repo.go
type orchestrationCacheRepo struct {
    data *Data
}

func (r *orchestrationCacheRepo) LoadCacheJSON(ctx context.Context) (string, error) {
    // 从 AgentRuntimeSettings.ExtraJSON 中读取 orchestration_cache 键
}

func (r *orchestrationCacheRepo) SaveCacheJSON(ctx context.Context, jsonStr string) error {
    // 写入 AgentRuntimeSettings.ExtraJSON 的 orchestration_cache 键
}
```

#### 3.4.3 OrchestrationCache 集成

```go
func NewOrchestrationCache(repo OrchestrationCacheRepo, lg loggateway.Logger) *OrchestrationCache {
    c := &OrchestrationCache{
        entries: make(map[string]*OrchestrationCacheEntry),
        repo:    repo,
        lg:      lg,
    }
    // 启动时加载
    ctx := context.Background()
    if jsonStr, err := repo.LoadCacheJSON(ctx); err == nil {
        c.LoadFromJSON(jsonStr)
    }
    return c
}

func (c *OrchestrationCache) Put(entry OrchestrationCacheEntry) {
    c.mu.Lock()
    defer c.mu.Unlock()
    entry.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
    c.entries[entry.TaskPattern] = &entry
    // 异步持久化
    go safego.Go(func() {
        if jsonStr, err := c.ToJSON(); err == nil {
            c.repo.SaveCacheJSON(context.Background(), jsonStr)
        }
    })
}
```

---

### 3.5 P1-05：DQ Score 三元分解

**现状**：`ComputeDQScore` 仅基于时间惩罚。

**需求**：设计文档要求 `DQ Score = Validity * 0.4 + Specificity * 0.3 + Correctness * 0.3`。

**设计方案**：

#### 3.5.1 DQ Score 三元分解

```go
type DQScoreBreakdown struct {
    Validity     float64 `json:"validity"`      // 结果有效性（0-1）
    Specificity  float64 `json:"specificity"`    // 结果具体性（0-1）
    Correctness  float64 `json:"correctness"`    // 结果正确性（0-1）
    Overall      float64 `json:"overall"`        // 加权总分
    DurationMs   int64   `json:"duration_ms"`    // 执行时长
}

func ComputeDQScoreV2(teamResult TeamSynthesisResult, durationMs int64, toolInvocations []ToolInvocation) DQScoreBreakdown {
    if teamResult.Status != "completed" {
        return DQScoreBreakdown{Overall: 0.0}
    }

    // Validity: 工具调用成功率
    validity := 1.0
    if len(toolInvocations) > 0 {
        successCount := 0
        for _, inv := range toolInvocations {
            if inv.Status == "success" {
                successCount++
            }
        }
        validity = float64(successCount) / float64(len(toolInvocations))
    }

    // Specificity: 结果长度和结构化程度
    specificity := 0.5
    contentLen := len(teamResult.Summary + teamResult.KeyFindings)
    if contentLen > 100 {
        specificity = 0.7
    }
    if contentLen > 500 {
        specificity = 0.9
    }
    if teamResult.KeyFindings != "" {
        specificity = min(specificity+0.1, 1.0)
    }

    // Correctness: 基于时间效率的代理指标
    correctness := 1.0
    if durationMs > 0 {
        timePenalty := float64(durationMs) / 60000.0
        if timePenalty > 5.0 {
            timePenalty = 5.0
        }
        correctness -= timePenalty * 0.1
    }
    if correctness < 0.1 {
        correctness = 0.1
    }

    overall := validity*0.4 + specificity*0.3 + correctness*0.3
    return DQScoreBreakdown{
        Validity:    validity,
        Specificity: specificity,
        Correctness: correctness,
        Overall:     overall,
        DurationMs:  durationMs,
    }
}
```

#### 3.5.2 数据来源

- **Validity**：从 `ToolInvocation` 表查询团队 session 下的工具调用记录
- **Specificity**：从团队最后一条 assistant 消息的内容长度和结构化程度推断
- **Correctness**：基于执行时长的代理指标（无人工标注时的替代方案）

#### 3.5.3 调用入口

```go
// team_turn_hooks.go recordSpiritTeamCompletion 中：
toolInvs, _ := o.team.TeamUC.ListToolInvocations(ctx, team.ID)
dqBreakdown := biz.ComputeDQScoreV2(biz.TeamSynthesisResult{...}, durationMs, toolInvs)
o.orchCache.RecordCompletion(ctx, taskPattern, topology, dqBreakdown.Overall, 1, durationMs)
```

---

### 3.6 P1-06：编排优化建议生成

**现状**：DQ Score < 0.5 时没有生成优化建议。

**需求**：DQ Score < 0.5 时，进化系统生成编排优化建议。

**设计方案**：在 `recordSpiritTeamCompletion` 中，当 DQ Score < 0.5 时，调用 `EvolutionUsecase` 生成编排优化建议。

#### 3.6.1 新增 `EvolutionSuggestionPort`

```go
// internal/biz/spirit_orchestration_cache.go
type EvolutionSuggestionPort interface {
    CreateOrchestrationSuggestion(ctx context.Context, agentKey string, taskPattern string, currentTopology TopologyType, dqScore float64) error
}
```

#### 3.6.2 EvolutionUsecase 实现

```go
func (u *EvolutionUsecase) CreateOrchestrationSuggestion(ctx context.Context, agentKey string, taskPattern string, currentTopology TopologyType, dqScore float64) error {
    // 生成拓扑优化建议
    suggestedTopology := u.suggestAlternativeTopology(currentTopology, dqScore)
    suggestion := EvolutionSuggestion{
        AgentKey:  agentKey,
        Type:      "orchestration",
        Title:     fmt.Sprintf("编排优化建议: %s → %s (DQ=%.2f)", currentTopology, suggestedTopology, dqScore),
        Content:   fmt.Sprintf("任务模式 '%s' 使用 %s 拓扑的 DQ Score 为 %.2f，建议尝试 %s 拓扑", taskPattern, currentTopology, dqScore, suggestedTopology),
        Status:    "pending",
    }
    return u.suggestionRepo.Create(ctx, suggestion)
}
```

#### 3.6.3 拓扑替代建议

```go
func (u *EvolutionUsecase) suggestAlternativeTopology(current TopologyType, dqScore float64) TopologyType {
    // 简单策略：按拓扑质量排序推荐
    alternatives := []TopologyType{
        TopologyCoordinator,
        TopologyHybrid,
        TopologyParallel,
        TopologySequential,
    }
    for _, alt := range alternatives {
        if alt != current {
            return alt
        }
    }
    return current
}
```

#### 3.6.4 进化护栏接入

```go
// 在 CreateOrchestrationSuggestion 中检查护栏
func (u *EvolutionUsecase) checkGuardrails(agent Agent, dqScore float64) bool {
    if agent.GuardrailMaxChangePerPeriod > 0 {
        // 检查本周期内已生成的建议数量
        recentCount := u.suggestionRepo.CountRecent(ctx, agent.Key, 24*time.Hour)
        maxChanges := int(float64(recentCount) * agent.GuardrailMaxChangePerPeriod)
        if recentCount >= maxChanges && maxChanges > 0 {
            return false // 护栏阻止
        }
    }
    // DQ Score < 0.3 时触发回滚
    if dqScore < 0.3 {
        u.lg.Warn("DQ Score 严重偏低，建议回滚到上一个稳定拓扑",
            loggateway.StepID("spirit.evolution.rollback"),
            loggateway.Float64("dq_score", dqScore),
        )
    }
    return true
}
```

---

### 3.7 P1-07：RouteTopology 算法完善

**现状**：`RouteTopology` 是简化版，`computeDepth` 和 `computeWidth` 未实现。

**设计方案**：实现完整的拓扑路由算法，对齐 AdaptOrch 论文。

```go
func (d *TaskDAG) RouteTopology() TopologyType {
    if len(d.Nodes) == 0 {
        return TopologyCoordinator
    }
    if len(d.Roots) == len(d.Nodes) {
        return TopologyParallel
    }
    // 计算深度和宽度
    depth := d.computeDepth()
    width := d.computeMaxWidth()

    // 依赖链深度 > 3 → coordinator
    if depth > 3 {
        return TopologyCoordinator
    }
    // 有依赖且宽度 > 1 → hybrid
    if width > 1 {
        return TopologyHybrid
    }
    // 有依赖但宽度 = 1 → sequential
    return TopologySequential
}

func (d *TaskDAG) computeDepth() int {
    depthMap := make(map[TaskNodeID]int, len(d.Nodes))
    var calcDepth func(id TaskNodeID) int
    calcDepth = func(id TaskNodeID) int {
        if dep, ok := depthMap[id]; ok {
            return dep
        }
        node := d.Nodes[id]
        if node == nil {
            return 0
        }
        maxDep := 0
        for _, depID := range node.DependsOn {
            if dep := calcDepth(depID); dep > maxDep {
                maxDep = dep
            }
        }
        depthMap[id] = maxDep + 1
        return depthMap[id]
    }
    maxDepth := 0
    for id := range d.Nodes {
        if dep := calcDepth(id); dep > maxDepth {
            maxDepth = dep
        }
    }
    return maxDepth
}

func (d *TaskDAG) computeMaxWidth() int {
    levelMap := make(map[int]int)
    for id := range d.Nodes {
        level := d.computeNodeLevel(id)
        levelMap[level]++
    }
    maxWidth := 0
    for _, w := range levelMap {
        if w > maxWidth {
            maxWidth = w
        }
    }
    return maxWidth
}

func (d *TaskDAG) computeNodeLevel(id TaskNodeID) int {
    node := d.Nodes[id]
    if node == nil || len(node.DependsOn) == 0 {
        return 0
    }
    maxLevel := 0
    for _, depID := range node.DependsOn {
        if level := d.computeNodeLevel(depID); level > maxLevel {
            maxLevel = level
        }
    }
    return maxLevel + 1
}
```

---

## 四、P2 问题详细设计

### 4.1 P2-01：团队超时未实现

**设计方案**：在 `AssembleTeam` 创建团队后，注册超时回调。

```go
func (u *SpiritTeamUsecase) AssembleTeam(ctx context.Context, params SpiritTeamParams) (SpiritTeamResult, error) {
    // ... 创建团队和 session ...

    // 注册超时回调
    cfg := u.resolveParallelConfig(ctx, params.SpiritSessionID)
    if cfg.TeamTimeoutSeconds > 0 {
        time.AfterFunc(cfg.TeamTimeout(), func() {
            safego.Go(func() {
                team, err := u.teamUC.Get(context.Background(), team.ID)
                if err != nil || team.Status == "completed" || team.Status == "failed" {
                    return
                }
                u.teamUC.Update(context.Background(), team.ID, Team{Status: "failed"})
                // 发布超时事件
            })
        })
    }

    return SpiritTeamResult{Team: team, Session: teamSession}, nil
}
```

### 4.2 P2-02：自动归档未实现

**设计方案**：团队完成超过 `AutoArchiveAfter` 后自动归档。

```go
// 在 CheckTeamProgress 或定时任务中检查
func (u *SpiritTeamUsecase) AutoArchiveCompleted(ctx context.Context, spiritSessionID string) {
    cfg := u.resolveParallelConfig(ctx, spiritSessionID)
    teams, _ := u.ListCompletedTeams(ctx, spiritSessionID)
    for _, t := range teams {
        if time.Since(t.UpdatedAt) > cfg.AutoArchiveAfter() {
            u.teamUC.Update(ctx, t.ID, Team{Status: "archived"})
        }
    }
}
```

### 4.3 P2-03：Session 树深度限制未实现

**设计方案**：在 `AssembleTeam` 创建 team session 时检查深度。

```go
func (u *SpiritTeamUsecase) AssembleTeam(ctx context.Context, params SpiritTeamParams) (SpiritTeamResult, error) {
    // ... 创建团队 ...

    cfg := u.resolveParallelConfig(ctx, params.SpiritSessionID)
    // 检查 Session 树深度
    parentSession, err := u.sessionUC.Get(ctx, spiritSessionID)
    if err != nil {
        return SpiritTeamResult{}, err
    }
    if parentSession.AgentDepth >= cfg.MaxSessionDepth {
        return SpiritTeamResult{}, kerrors.BadRequest("SPIRIT",
            fmt.Sprintf("session tree depth (%d) exceeds max (%d)", parentSession.AgentDepth, cfg.MaxSessionDepth))
    }

    teamSession, err := u.sessionUC.Create(ctx, Session{
        AgentDepth: parentSession.AgentDepth + 1,
        // ...
    })
}
```

---

## 五、前端深度实现设计

### 5.1 前端差距总览

| 组件 | 差距 | 优先级 |
|------|------|--------|
| `ParallelTeamOverview.vue` | 团队列表未排序 | P1 |
| `TeamProgressCard.vue` | 进度条只有 0%/100%；无耗时显示 | P1 |
| `SynthesisResultCard.vue` | teamResults 数据源不完整 | P0 |
| Store `handleSpiritEnvelope` | `spirit_team_progress` 缺少 progress_pct 更新 | P1 |
| 依赖图可视化 | 未实现文本形式 DAG 展示 | P1 |
| 并行配额实时更新 | 取消团队后配额未实时更新 | P2 |

### 5.2 Store 增强

#### 5.2.1 进度实时更新

```typescript
case "spirit_team_progress":
    if (teamId) {
        const pct = Number(md.progress_pct ?? 0);
        const step = String(md.current_step ?? "");
        updateTeamStatus(teamId, String(md.status ?? "running"));
        // 更新进度
        const team = teams.value.find(t => t.id === teamId);
        if (team && pct >= 0) {
            team.completedSteps = Math.round(pct * team.totalSteps / 100);
        }
    }
    break;
```

#### 5.2.2 团队列表排序

```typescript
const sortedTeams = computed(() => {
    const statusOrder: Record<string, number> = {
        assembling: 0, assembled: 0, running: 0,
        waiting_deps: 1,
        completed: 2,
        failed: 3,
        cancelled: 4,
    };
    return [...teams.value].sort((a, b) => (statusOrder[a.status] ?? 99) - (statusOrder[b.status] ?? 99));
});
```

### 5.3 TeamProgressCard 增强

#### 5.3.1 耗时显示

```vue
<div v-if="durationText" class="team-progress-card__duration text-caption text-grey-6">
    ⏱ {{ durationText }}
</div>
```

```typescript
const durationText = computed(() => {
    if (!props.team.durationMs) return "";
    const seconds = Math.floor(props.team.durationMs / 1000);
    if (seconds < 60) return `${seconds}s`;
    const minutes = Math.floor(seconds / 60);
    return `${minutes}m ${seconds % 60}s`;
});
```

需要在 `SpiritTeam` 类型中新增 `durationMs` 字段，并在 WS 事件中传递。

### 5.4 DAG 文本展示组件

新增 `DAGDiagramCard.vue`：

```vue
<template>
  <div v-if="diagram" class="dag-diagram-card">
    <div class="dag-diagram-card__title">任务依赖图</div>
    <pre class="dag-diagram-card__content">{{ diagram }}</pre>
  </div>
</template>
```

在 `ParallelTeamOverview.vue` 中，当存在 DAG 团队时显示。

---

## 六、实现优先级与任务拆分

### 6.1 Phase 3 — 深度业务实现

| 排序 | ID | 任务 | 差距级别 | 影响域 |
|------|-----|------|---------|--------|
| 1 | SPO-DP-01 | `TeamStarterPort` 接口 + `ChatOrchestrator` 实现 + Wire 绑定 | P0 | biz + service + wire |
| 2 | SPO-DP-02 | `start_team` 工具 + `assemble_team` 自动启动 | P0 | tools + service |
| 3 | SPO-DP-03 | `scheduleDependentTeams` 激活后自动启动 | P0 | service/team_turn_hooks |
| 4 | SPO-DP-04 | `ExtractTeamOutput` + Summary/KeyFindings 提取 | P0 | biz + service |
| 5 | SPO-DP-05 | 部分失败合成 + 级联标注 | P0 | biz + service |
| 6 | SPO-DP-06 | 删除 `DependencyScheduler` 死代码 | P0 | biz |
| 7 | SPO-DP-07 | 进度中间值计算 + CurrentStep | P1 | biz |
| 8 | SPO-DP-08 | 实时进度事件（Turn 完成时发布） | P1 | service |
| 9 | SPO-DP-09 | 前端团队列表排序 + 进度更新 | P1 | frontend |
| 10 | SPO-DP-10 | `TaskDAG.ToTextDiagram()` + DAG 文本展示 | P1 | biz + tools + frontend |
| 11 | SPO-DP-11 | `OrchestrationCache` 持久化 | P1 | biz + data |
| 12 | SPO-DP-12 | DQ Score 三元分解 | P1 | biz |
| 13 | SPO-DP-13 | 编排优化建议生成 + 进化护栏接入 | P1 | biz |
| 14 | SPO-DP-14 | `RouteTopology` 算法完善 | P1 | biz |
| 15 | SPO-DP-15 | 团队超时实现 | P2 | biz |
| 16 | SPO-DP-16 | 自动归档实现 | P2 | biz |
| 17 | SPO-DP-17 | Session 树深度限制 | P2 | biz |
| 18 | SPO-DP-18 | 前端 TeamProgressCard 耗时显示 | P2 | frontend |

### 6.2 依赖关系

```
SPO-DP-01 → SPO-DP-02 → SPO-DP-03 (TeamStarterPort → start_team → 依赖调度自动启动)
SPO-DP-04 → SPO-DP-05 (ExtractTeamOutput → 部分失败合成)
SPO-DP-07 → SPO-DP-08 (进度计算 → 实时事件)
SPO-DP-12 → SPO-DP-13 (DQ Score V2 → 优化建议)
SPO-DP-11 (缓存持久化独立)
SPO-DP-06 (死代码清理独立)
```

---

## 七、风险与缓解

| 风险 | 缓解 |
|------|------|
| `TeamStarterPort` 循环依赖 | 接口定义在 biz 层，ChatOrchestrator 在 service 层实现，Wire 绑定 |
| `StartTeamTurn` 异步启动失败 | 使用 `safego.Go` + 失败时发布 `spirit_team_failed` 事件 |
| DQ Score 三元分解数据不完整 | Validity 基于工具调用记录（已有），Specificity 基于内容长度（代理），Correctness 基于时间（代理） |
| OrchestrationCache 持久化性能 | 异步写入，不阻塞主流程 |
| 团队超时 `time.AfterFunc` 内存泄漏 | 使用 `sync.Map` 注册/取消超时回调，团队完成时取消 |
| 前端 WS 消息风暴 | 进度事件节流（500ms），按团队分组 |

---

## 八、验证计划

### 8.1 P0 验证

| 验证项 | 方法 |
|--------|------|
| 团队创建后自动启动 | 精灵对话中调用 `assemble_team`，验证团队 Runner 自动执行 |
| 依赖团队激活后自动启动 | DAG 场景中前置团队完成后，验证依赖团队自动开始执行 |
| SynthesisResult 包含 Summary | 调用 `synthesize_results`，验证返回结果包含非空 Summary |
| 部分失败合成 | 模拟部分团队失败，验证合成结果包含失败标注 |

### 8.2 P1 验证

| 验证项 | 方法 |
|--------|------|
| 进度中间值 | 团队执行中调用 `check_team_progress`，验证进度在 0-100% 之间 |
| 团队列表排序 | 前端验证团队按 running → waiting → completed → failed 排序 |
| DAG 文本展示 | 前端验证 DAG 团队展示文本依赖图 |
| 缓存持久化 | 重启服务后验证编排缓存仍存在 |
| DQ Score 三元分解 | 验证 DQ Score 包含 Validity/Specificity/Correctness 分项 |
| 优化建议生成 | DQ Score < 0.5 时验证生成 pending 建议 |

### 8.3 全量验证

```bash
# 后端
make api && make wire && make build && make test && make lint

# 前端
cd web && pnpm lint && pnpm test && pnpm build
```

---

## 九、Phase 4 差距与详细实现 — 智能增强

> 基于 AI Agent 工作模式启发，补充 4 个关键增强的详细实现设计。

### 9.1 Phase 4 差距矩阵

| 验收 ID | 摘要 | 骨架状态 | 差距级别 | 差距描述 |
|---------|------|---------|---------|---------|
| SPO-10 | 任务复杂度智能评估 | ❌ 不存在 | **P0** | 无形式化复杂度评估机制，Spirit 路由决策依赖 LLM 软约束 |
| SPO-11 | Graph DAG 动态编排 | ❌ 不存在 | **P1** | 编排管家只有线性工具调用，无法利用 Graph 并行/条件/检查点能力 |
| SPO-12 | 编排验证门禁节点 | ❌ 不存在 | **P1** | Graph 无自动化验证门禁，质量检查依赖最终合成 |

### 9.2 SPO-10 详细实现：TaskComplexityClassifier

#### 9.2.1 `assess_complexity` 工具实现

**实现位置**：`internal/tools/spirit/assess_complexity.go`

```go
type assessComplexityTool struct {
    rules *ComplexityRuleEngine
}

func (t *assessComplexityTool) Call(ctx context.Context, input AssessComplexityInput) (*AssessComplexityOutput, error) {
    level := t.rules.Assess(input.UserMessage)
    path := levelToPath(level)
    return &AssessComplexityOutput{
        Level:         string(level),
        Reasoning:     t.rules.LastReasoning(),
        SuggestedPath: path,
    }, nil
}

func levelToPath(level ComplexityLevel) string {
    switch level {
    case ComplexitySimple:
        return "direct_answer"
    case ComplexityModerate:
        return "single_butler"
    case ComplexityComplex:
        return "orchestrator"
    default:
        return "single_butler"
    }
}
```

#### 9.2.2 规则引擎实现

**实现位置**：`internal/tools/spirit/complexity_rules.go`

```go
type ComplexityLevel string

const (
    ComplexitySimple   ComplexityLevel = "simple"
    ComplexityModerate ComplexityLevel = "moderate"
    ComplexityComplex  ComplexityLevel = "complex"
)

type ComplexityRuleEngine struct {
    simplePatterns    []string
    complexIndicators []string
    lastReasoning     string
    mu                sync.Mutex
}

func NewComplexityRuleEngine() *ComplexityRuleEngine {
    return &ComplexityRuleEngine{
        simplePatterns: []string{
            "什么是", "解释一下", "帮我看看", "怎么用",
            "是什么意思", "告诉我", "列出", "显示",
            "what is", "explain", "show me", "how to use",
        },
        complexIndicators: []string{
            "分析", "对比", "编写", "设计", "规划", "编排",
            "多个", "跨行业", "团队", "协作", "流程",
            "analyze", "compare", "design", "plan", "orchestrate",
        },
    }
}

func (r *ComplexityRuleEngine) Assess(message string) ComplexityLevel {
    r.mu.Lock()
    defer r.mu.Unlock()

    lower := strings.ToLower(message)

    for _, p := range r.simplePatterns {
        if strings.Contains(lower, strings.ToLower(p)) {
            r.lastReasoning = fmt.Sprintf("匹配简单问答模式: %s", p)
            return ComplexitySimple
        }
    }

    complexHits := 0
    for _, p := range r.complexIndicators {
        if strings.Contains(lower, strings.ToLower(p)) {
            complexHits++
        }
    }
    if complexHits >= 2 {
        r.lastReasoning = fmt.Sprintf("匹配 %d 个复杂任务指标", complexHits)
        return ComplexityComplex
    }
    if complexHits == 1 {
        r.lastReasoning = "匹配 1 个复杂任务指标，但不足以确定，降级为 moderate"
        return ComplexityModerate
    }

    r.lastReasoning = "无法通过规则确定复杂度，使用安全默认值 moderate"
    return ComplexityModerate
}

func (r *ComplexityRuleEngine) LastReasoning() string {
    r.mu.Lock()
    defer r.mu.Unlock()
    return r.lastReasoning
}
```

#### 9.2.3 工具注册

**实现位置**：`internal/tools/spirit_tools.go` 中 `NewSpiritTools` 函数追加：

```go
tools = append(tools,
    NewAssessComplexityTool(NewComplexityRuleEngine()),
)
```

#### 9.2.4 Spirit Prompt 更新

**实现位置**：`internal/scenario/system/prompts/spirit.md`

在现有 Prompt 末尾追加：

```
## 决策规则（强制）
1. 收到用户消息后，先调用 assess_complexity 评估复杂度
2. 根据评估结果路由：
   - simple → 直接回答，不委派
   - moderate → 委派给最相关的单一管家
   - complex → 委派给 __orchestrator__
3. 禁止跳过 assess_complexity 直接委派
4. 禁止对 simple 级别任务委派给管家
```

### 9.3 SPO-11 详细实现：GraphOrchestration

#### 9.3.1 `build_orchestration_graph` 工具实现

**实现位置**：`internal/tools/orchestrator/build_graph.go`

```go
type buildOrchestrationGraphTool struct {
    deps OrchestratorGraphDeps
}

type OrchestratorGraphDeps struct {
    GraphExecutor func() biz.GraphExecutor
    SessionIDFunc func(ctx context.Context) string
}

func (t *buildOrchestrationGraphTool) Call(
    ctx context.Context, input BuildOrchestrationGraphInput,
) (*BuildOrchestrationGraphOutput, error) {
    cfg := t.buildGraphConfig(input)

    graphID := fmt.Sprintf("orchestration_%d", time.Now().UnixMilli())
    sessID := t.deps.SessionIDFunc(ctx)

    executionID, err := t.deps.GraphExecutor().ExecuteGraphBuildConfig(
        ctx, graphID, sessID, cfg, map[string]any{
            "task_description": input.TaskPrompt,
        },
    )
```

#### 9.3.2 Graph 拓扑生成关键逻辑

`buildGraphConfig` 方法根据 `AgentAssignment.DependsOn` 自动生成 DAG：
- 无依赖的 Agent → 从 entry_point 直连（可并行）
- 有依赖的 Agent → 从依赖 Agent 连边
- 所有 Agent 完成后 → JoinEdge 到 merge_results
- merge_results → verify_results（验证门禁节点）

#### 9.3.3 与 `assemble_team` 共存策略

**P0 阶段**：两个工具共存，编排管家 Prompt 中增加决策规则：

```
## 编排方式选择
- 简单任务（2-3 Agent，顺序执行）→ 使用 assemble_team
- 复杂任务（4+ Agent，有并行/条件路由）→ 使用 build_orchestration_graph
```

**P1 阶段**：`assemble_team` 内部重构为调用 `build_orchestration_graph`。

#### 9.3.4 依赖注入路径

`OrchestratorGraphDeps.GraphExecutor` 通过 `ChatOrchestratorDeps.Team.GraphFactory` 获取：

```go
func (o *ChatOrchestrator) orchestratorTools(ctx context.Context, ag biz.Agent) []trpctool.Tool {
    if ag.AgentKey != "__orchestrator__" { return nil }
    return orchestrator.RegisterAll(orchestrator.Deps{
        // ... 现有依赖
        GraphDeps: orchestrator.OrchestratorGraphDeps{
            GraphExecutor: func() biz.GraphExecutor { return o.td.Team.GraphFactory },
        },
    })
}
```

### 9.4 SPO-12 详细实现：VerificationGate

#### 9.4.1 验证节点类型定义

**实现位置**：`internal/tools/orchestrator/verification.go`

```go
type VerificationType string

const (
    VerifyOutputFormat   VerificationType = "output_format"
    VerifyTaskCompletion VerificationType = "task_completion"
    VerifyHumanApproval VerificationType = "human_approval"
)

type VerificationConfig struct {
    Type          VerificationType `json:"type"`
    NodeID        string           `json:"node_id"`
    InjectAfter   string           `json:"inject_after"`    // 在哪个节点后注入
    FailureAction string           `json:"failure_action"`  // skip / retry_then_block / fail_fast
}
```

#### 9.4.2 验证函数实现

**实现位置**：`internal/tools/orchestrator/verify_funcs.go`

```go
func verifyOutputFormat(ctx context.Context, state graph.State) (any, error) {
    results, ok := state["agent_results"]
    if !ok { return nil, fmt.Errorf("no agent results found in state") }

    agentResults, ok := results.(map[string]any)
    if !ok { return nil, fmt.Errorf("agent_results is not a map") }

    var issues []string
    for agentKey, result := range agentResults {
        resultStr, ok := result.(string)
        if !ok || resultStr == "" {
            issues = append(issues, fmt.Sprintf("agent %s returned empty result", agentKey))
        }
    }

    if len(issues) > 0 {
        return map[string]any{"verified": false, "issues": issues}, nil
    }
    return map[string]any{"verified": true}, nil
}

func verifyTaskCompletion(ctx context.Context, state graph.State) (any, error) {
    results, ok := state["agent_results"]
    if !ok { return map[string]any{"verified": false, "reason": "no results"}, nil }

    agentResults, ok := results.(map[string]any)
    if !ok { return map[string]any{"verified": false, "reason": "invalid results"}, nil }

    completedCount := 0
    for _, result := range agentResults {
        if resultStr, ok := result.(string); ok && resultStr != "" {
            completedCount++
        }
    }

    completionRate := float64(completedCount) / float64(len(agentResults))
    if completionRate >= 0.8 {
        return map[string]any{"verified": true, "completion_rate": completionRate}, nil
    }
    return map[string]any{
        "verified":        false,
        "completion_rate": completionRate,
        "reason":          fmt.Sprintf("only %.0f%% agents completed", completionRate*100),
    }, nil
}
```

#### 9.4.3 验证节点注入到 Graph

在 `buildGraphConfig` 中根据 `VerificationConfig` 注入验证节点：

```go
func (t *buildOrchestrationGraphTool) addVerificationNodes(
    cfg *biz.GraphBuildConfig, verifyConfigs []VerificationConfig,
) {
    for _, vc := range verifyConfigs {
        verifyNodeID := vc.NodeID
        if verifyNodeID == "" {
            verifyNodeID = fmt.Sprintf("verify_%s", vc.InjectAfter)
        }

        failureAction := biz.FailureActionSkip
        switch vc.FailureAction {
        case "retry_then_block":
            failureAction = biz.FailureActionRetryThenBlock
        case "fail_fast":
            failureAction = biz.FailureActionFailFast
        }

        interruptAfter := vc.Type == VerifyHumanApproval

        cfg.Nodes = append(cfg.Nodes, biz.NodeDef{
            ID:             verifyNodeID,
            Type:           "function",
            Instruction:    fmt.Sprintf("验证类型: %s", vc.Type),
            FailureAction:  failureAction,
            InterruptAfter: interruptAfter,
        })

        // 修改边：inject_after → verify_node → 原下游
        for i, e := range cfg.Edges {
            if e.From == vc.InjectAfter {
                cfg.Edges[i].From = verifyNodeID
            }
        }
        cfg.Edges = append(cfg.Edges, biz.EdgeDef{
            From: vc.InjectAfter, To: verifyNodeID,
        })

        if interruptAfter {
            cfg.InterruptAfter = append(cfg.InterruptAfter, verifyNodeID)
        }
    }
}
```

### 9.5 GAP-3 详细实现：AdaptiveTeamMode

#### 9.5.1 Spirit Team 构建逻辑

**实现位置**：`internal/service/chat_orchestrator_spirit.go`（新增文件）

```go
type SpiritTeamMode string

const (
    SpiritModeCoordinator SpiritTeamMode = "coordinator"
    SpiritModeSwarm       SpiritTeamMode = "swarm"
    SpiritModeDirect      SpiritTeamMode = "direct"
)

func (o *ChatOrchestrator) selectSpiritMode(
    complexityLevel string,
) SpiritTeamMode {
    switch complexityLevel {
    case "simple":
        return SpiritModeDirect
    case "moderate":
        return SpiritModeDirect
    case "complex":
        return SpiritModeCoordinator
    default:
        return SpiritModeCoordinator
    }
}

func (o *ChatOrchestrator) buildSpiritTeam(
    ctx context.Context, spiritAg biz.Agent, deps chatagent.TRPCBuilderDeps,
    mode SpiritTeamMode,
) (agent.Agent, error) {
    spiritAgent, err := chatagent.BuildTRPCAgentCached(ctx, spiritAg, deps)
    if err != nil { return nil, err }

    if mode == SpiritModeDirect {
        return spiritAgent, nil
    }

    butlers, err := o.loadSystemButlers(ctx, deps)
    if err != nil { return nil, err }

    switch mode {
    case SpiritModeCoordinator:
        return trpcteam.New(spiritAgent, butlers)
    case SpiritModeSwarm:
        return trpcteam.NewSwarm(
            "spirit_swarm", spiritAgent.Info().Name,
            append([]agent.Agent{spiritAgent}, butlers...),
        )
    default:
        return trpcteam.New(spiritAgent, butlers)
    }
}

func (o *ChatOrchestrator) loadSystemButlers(
    ctx context.Context, deps chatagent.TRPCBuilderDeps,
) ([]agent.Agent, error) {
    var butlerKeys = []string{"__orchestrator__", "__system_admin__", "__memory__", "__skills__", "__monitor__"}
    var members []agent.Agent
    for _, key := range butlerKeys {
        b, err := o.td.Catalog.Agents.GetAgentByAgentKey(ctx, key)
        if err != nil { continue }
        memberDeps := deps
        memberDeps.CustomTools = o.systemBuiltinTools(ctx, b)
        member, err := chatagent.BuildTRPCAgentCached(ctx, b, memberDeps)
        if err != nil { continue }
        members = append(members, member)
    }
    return members, nil
}
```

#### 9.5.2 与 `runSingleAgentViaTRPC` 集成

**修改位置**：`internal/service/chat_orchestrator_turn.go`

```go
// 在 __spirit__ 分支中：
if ag.AgentKey == "__spirit__" {
    mode := o.selectSpiritMode(complexityLevel) // 从 assess_complexity 结果获取
    root, err = o.buildSpiritTeam(ctx, ag, deps, mode)
} else {
    root, err = chatagent.BuildTRPCAgentCached(ctx, ag, deps)
}
```

### 9.6 Phase 4 实现优先级

| 排序 | ID | 任务 | 差距级别 | 影响域 |
|------|-----|------|---------|--------|
| 1 | SPO-P4-01 | `ComplexityRuleEngine` + `assess_complexity` 工具 | P0 | tools/spirit |
| 2 | SPO-P4-02 | Spirit Prompt 强制决策规则 | P0 | scenario/prompts |
| 3 | SPO-P4-03 | `chat_orchestrator_spirit.go` + Team 模式选择 | P0 | service |
| 4 | SPO-P4-04 | `runSingleAgentViaTRPC` 集成 Spirit 模式选择 | P0 | service |
| 5 | SPO-P4-05 | `build_orchestration_graph` 工具 | P1 | tools/orchestrator |
| 6 | SPO-P4-06 | `buildGraphConfig` DAG 生成逻辑 | P1 | tools/orchestrator |
| 7 | SPO-P4-07 | 验证节点类型定义 + 验证函数 | P1 | tools/orchestrator |
| 8 | SPO-P4-08 | 验证节点注入到 Graph | P1 | tools/orchestrator |
| 9 | SPO-P4-09 | `OrchestratorGraphDeps` 依赖注入 | P1 | service |
| 10 | SPO-P4-10 | 编排管家 Prompt Graph 编排决策规则 | P1 | scenario/prompts |

### 9.7 依赖关系

```
SPO-P4-01 → SPO-P4-02 (规则引擎 → Prompt 规则)
SPO-P4-01 → SPO-P4-03 (assess_complexity → Team 模式选择)
SPO-P4-03 → SPO-P4-04 (buildSpiritTeam → runSingleAgentViaTRPC 集成)
SPO-P4-05 → SPO-P4-06 (工具定义 → DAG 生成逻辑)
SPO-P4-06 → SPO-P4-07 (DAG 生成 → 验证节点)
SPO-P4-07 → SPO-P4-08 (验证函数 → 注入逻辑)
SPO-P4-05 → SPO-P4-09 (工具 → 依赖注入)
SPO-P4-06 → SPO-P4-10 (DAG 生成 → Prompt 规则)
```

### 9.8 风险与缓解

| 风险 | 缓解 |
|------|------|
| `assess_complexity` 规则引擎覆盖不全 | P0 使用安全默认值 moderate；P1 引入历史数据优化 |
| `build_orchestration_graph` 生成的 DAG 不合理 | P0 保留 assemble_team 作为回退；P1 增加模板缓存 |
| Graph 验证节点增加执行时间 | 验证节点使用 FailureAction=Skip，验证失败不阻塞 |
| Spirit Team 模式选择错误 | P0 默认 Coordinator（最安全）；P1 基于成功率自动优化 |
| `OrchestratorGraphDeps` 循环依赖 | 接口定义在 biz 层，实现注入在 service 层 |

### 9.9 验证计划

#### 9.9.1 P0 验证

| 验证项 | 方法 |
|--------|------|
| assess_complexity 规则引擎 | 单元测试覆盖 simple/moderate/complex 三级 |
| Spirit 强制决策规则 | 精灵对话中验证先调用 assess_complexity 再路由 |
| Team 模式选择 | 验证 simple→Direct, moderate→Direct, complex→Coordinator |

#### 9.9.2 P1 验证

| 验证项 | 方法 |
|--------|------|
| Graph DAG 生成 | 验证并行/串行/混合拓扑正确性 |
| 验证节点注入 | 验证 output_format/task_completion/human_approval 三种类型 |
| 验证函数逻辑 | 验证空结果检测、完成度计算 |
| HITL 中断 | 验证 human_approval 验证节点触发 interrupt |

#### 9.9.3 全量验证

```bash
make api && make wire && make build && make test && make lint
cd web && pnpm lint && pnpm test && pnpm build
```

---

## 十、深度架构审查修复记录

> 2026-06-06：对 Spirit Team 全链路进行深度架构审查，发现并修复 7 个严重问题 + 5 个中等问题 + 3 个轻微问题。

### 10.1 严重问题修复

| ID | 问题 | 修复方案 | 影响文件 |
|----|------|----------|----------|
| S3 | OrchestrationCache.ToJSON() 递归 RLock 导致死锁 | 提取 `listLocked()` 内部方法，`ToJSON()` 和 `List()` 共用 | `spirit_orchestration_cache.go` |
| S4 | 超时回调仅转换状态，不触发依赖调度/事件发布/AllDone 检查 | 新增 `TimeoutHandler` 接口（biz 层），`TeamStarter` 实现，`BeforeStart` 阶段注入 | `spirit_team_usecase.go`, `spirit_team.go`, `app.go` |
| S5 | `interrupted` 状态被 `CheckAllTeamsCompleted` 错误视为终态 | switch 增加 `TeamStatusInterrupted` case；`IsTeamStatusActive` 同步增加 | `spirit_team_usecase.go`, `team_types.go` |
| FS1 | 前后端 SpiritTeamMode 枚举不一致 | 对齐为 `coordinator/sequential/parallel/critic_loop/swarm/adaptive/direct` | `types.ts`, `TeamTaskCard.vue`, `TeamAssemblyCard.vue`, `TeamProgressCard.vue` |
| FS2 | 前后端 SpiritTeamStatus 枚举不一致 | 对齐为 `pending/running/completed/failed/cancelled/interrupted/archived` | `types.ts`, `stores/spirit/index.ts` |
| FS3 | SynthesisResultCard 使用 v-html 渲染未净化内容 | 替换为 `renderChatMarkdown()`（已通过安全审计） | `SynthesisResultCard.vue` |
| FS4 | cancelTeam 成功后从列表移除团队 | 改为 `updateTeamStatus(teamId, 'cancelled')`，与后端行为一致 | `stores/spirit/index.ts` |

### 10.2 中等问题修复

| ID | 问题 | 修复方案 | 影响文件 |
|----|------|----------|----------|
| M11 | HandleTeamTurnResult failed/cancelled 路径不取消超时定时器 | 入口统一调用 `CancelTimeoutTimer` | `spirit_team.go` |
| M13 | BuildGraphConfig 无循环检测和依赖验证 | DFS 三色标记法循环检测 + 悬空依赖跳过 + 环时降级顺序链 | `build_graph.go` |
| M8 | 前端 spirit_team_progress status 来源混用导致状态回退 | 增加状态转换合法性校验，禁止 running→pending 回退 | `stores/spirit/index.ts` |
| M6 | mode 默认值 `??` 不覆盖空字符串 | 改为 `\|\|` 运算符 | `stores/spirit/index.ts` |
| M7 | synthesizedAt 显示原始 ISO 时间戳 | 使用 `toLocaleString()` 格式化 | `SynthesisResultCard.vue` |

### 10.3 轻微问题修复

| ID | 问题 | 修复方案 | 影响文件 |
|----|------|----------|----------|
| L11 | AutoArchiveCompletedTeams 静默忽略 TransitionStatus 错误 | 添加 Warn 日志 | `spirit_team_usecase.go` |
| L17 | checkAllTeamsCompleted 在循环内重复调用 | 移到循环外统一调用一次 | `spirit_team.go` |
| WIRE | provideFailurePatternSyncJob 接口注入 + 测试 stub 缺失 | 改为接收接口类型 + 补全 GetTeamByKey stub | `wire.go`, `*_test.go` |

### 10.4 aranea-review 审查结论

- **1 个阻断项**（R-01: IsTeamStatusActive 与 CheckAllTeamsCompleted 对 interrupted 语义不一致）→ 已修复
- **10 个建议项**（构造函数参数过多、魔法数字、函数超长等）→ 记录备忘，后续迭代处理
- **1 个提示项**（NewOrchestrationCache 允许 repo 为 nil）→ 可接受
- **合规性清单**：依赖方向向内 ✅ | Runner 装配在 Service ✅ | goroutine 走 safego ✅ | 日志用 loggateway ✅ | 业务错误用 kerrors ✅ | 跨模块通过窄接口 ✅
