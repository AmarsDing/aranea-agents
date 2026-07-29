# Harness 借鉴方案：Signals 双通道 + 安全阀（连跳上限）

> 来源：抖音 @云淡风轻《Harness引擎，自研harness编排工具》
> 视频整理：07-Harness引擎视频整理.md
> 日期：2026-07-27

---

## 一、现状分析

### 1.1 Activity 事件模型现状

当前 Activity 模型（`internal/biz/activity.go`）已具备完整的类型系统：

```
Kind:    task / thinking / action / reply / notice / confirm / plan / session / team_stage / graph_stage
Event:   created / streaming / updated / completed / failed / cancelled
Domain:  chat（持久化） / system（WS-only）
Status:  pending / running / tool_running / tool_blocked / completed / failed / ...
```

**关键发现：当前模型没有"机器信号"与"人类叙事"的分离。**

- 子智能体（团队成员）的产出直接作为 `reply` Activity 推送给前端——这是"给人看的"
- 引擎的门控判定（PrePlanningGate 的 `QuickAssess`）是**独立的纯计算**，不从子智能体产出中提取信号
- 团队执行完成后，`checkAllTeamsCompleted` 直接触发 synthesis turn，没有从成员产出中提取结构化判定

### 1.2 Spirit 自主循环现状

当前自动推进链路：

```
用户消息 → IntentPass → PrePlanningGate（复杂度评估）
  ├─ Simple → 直接回答
  └─ Moderate/Complex → 强制 plan_and_execute
      → TaskPlanner.Plan() → 团队组建 → 并行执行
      → checkAllTeamsCompleted → synthesis turn 注入
      → 精灵 LLM 合成结果 → 回复用户
```

**关键发现：没有连跳上限。**

- PrePlanningGate 是唯一的"质量门控"，但它只做一次复杂度评估
- 从"强制规划"到"团队组建"到"并行执行"到"合成"，全自动无人工介入点
- `clarification gate` 和 `confirmation_guard` 只在特定条件下触发（歧义检测、高危工具），不是通用的连跳限制
- `OrchestrationSpec` 有 `LoopMaxIterations`，但那是 critic_loop 的迭代上限，不是 Spirit 编排自动推进的连跳上限

---

## 二、Signals 双通道方案

### 2.1 核心思想（Harness 借鉴）

> 子智能体产出拆成：**结构化信号（给引擎判门控，用户不可见）** + **干净的人类报告（给用户）**。

机器消费的信号与人看的叙事分离——引擎用结构化数据做决策，用户看纯文本报告。

### 2.2 系统层面

| 维度 | 现状 | 目标 |
|------|------|------|
| 子智能体产出 | 单通道：reply Activity 直接推送，既给人看又给系统用 | 双通道：signal（机器）+ reply（人类） |
| 门控判定 | 独立 QuickAssess（纯计算，不依赖子智能体产出） | 从子智能体 signal 中提取判定依据 |
| 团队合成 | checkAllTeamsCompleted 直接触发 synthesis，无质量判断 | 从成员 signal 中提取成功/失败/质量指标，决定是否需要人工审核 |

**具体改动**：

1. **Activity 模型增加 Signal 通道**——在 `biz.Activity` 的 `Meta` 中预留 `signal` 命名空间：
   ```go
   // Meta["signal"] = map[string]any{
   //     "gate_decision": "pass" | "fail" | "needs_review",
   //     "quality_score":  0.85,
   //     "findings_count": 3,
   //     "severity":       "low" | "medium" | "high",
   // }
   ```

2. **团队成员完成时产生双通道产出**：
   - `reply` Activity（Kind=reply）：干净的人类语言报告，推送给前端
   - `signal` 数据（Meta["signal"]）：结构化判定数据，供引擎消费，WS 推送时过滤

3. **引擎门控从 signal 中读取判定**：
   - `checkAllTeamsCompleted` 不再只看"全部完成"，还看"全部通过"
   - 如果某成员的 signal 标记 `gate_decision=fail`，即使全部完成也触发人工审核

### 2.3 架构层面

**改动点 1：Activity 模型扩展**

文件：`internal/biz/activity.go`

在 `Activity` struct 的 `Meta` 中约定 `signal` key，或新增 `Signal` 字段：

```go
// Signal carries machine-consumable structured data from sub-agent results.
// The engine reads Signal for gate decisions; the frontend never renders it.
type ActivitySignal struct {
    GateDecision   string  `json:"gate_decision,omitempty"`   // pass / fail / needs_review
    QualityScore   float64 `json:"quality_score,omitempty"`
    FindingsCount  int     `json:"findings_count,omitempty"`
    Severity       string  `json:"severity,omitempty"`        // low / medium / high / critical
    Confidence     float64 `json:"confidence,omitempty"`
    Metadata       map[string]any `json:"metadata,omitempty"`
}
```

**改动点 2：ActivityEvent WS 过滤**

文件：`internal/biz/activity_event.go` / WS 推送层

- `ActivityEvent` 增加 `ContainsSignal` 标记
- WS 推送时，如果 Activity 包含 signal 数据但 kind 不是 reply/notice，则剥离 signal 字段后推送
- 或者：signal 作为独立的 `ActivityEventType = "signal"`，不推送到前端

**改动点 3：团队成员产出分离**

文件：`internal/agent/v2/projector.go` / `internal/service/spirit_team.go`

- `HandleTeamTurnResult` 在团队完成时，从成员的最终产出中提取 signal
- 提取方式：LLM 后处理（从 reply 文本中解析结构化 JSON 信号）或工具返回（deliverable 工具直接返回结构化数据）
- signal 存入 TeamRun/MemberSession 的 Meta 中，供 checkAllTeamsCompleted 消费

**改动点 4：引擎门控消费 signal**

文件：`internal/service/spirit_team.go` — `checkAllTeamsCompleted`

```go
// 伪代码
func (s *TeamStarter) checkAllTeamsCompleted(ctx context.Context, spiritSessionID string) {
    result := s.team.SpiritUC.CheckAllTeamsCompleted(ctx, spiritSessionID)
    
    // 新增：收集团队成员的 signal
    signals := collectMemberSignals(ctx, spiritSessionID)
    
    // 门控判定：任何成员的 signal 标记 fail 或 needs_review → 人工审核
    if needsHumanReview(signals) {
        // 发布 confirm Activity，暂停流程
        emitHumanReviewRequest(ctx, spiritSessionID, signals)
        return
    }
    
    // 全部通过 → 触发 synthesis
    triggerSynthesis(ctx, spiritSessionID, result)
}
```

### 2.4 用户层面

| 场景 | 用户看到 | 引擎看到 |
|------|---------|---------|
| 团队成员完成 | "市场调研完成，发现 3 个竞品……"（干净文本报告） | `{"gate_decision":"pass","quality_score":0.85,"findings_count":3}` |
| 团队全部完成 | "所有团队已完成，正在合成简报……" | 各成员 signal 汇总 → 判定是否触发 synthesis |
| 质量不达标 | "成本估算存在不确定性，需要您确认后继续" | `{"gate_decision":"needs_review","severity":"medium"}` → 人工审核 |

**核心原则**：用户永远看不到 JSON/结构化数据。用户看到的永远是干净的人类语言。

---

## 三、安全阀（连跳上限）方案

### 3.1 核心思想（Harness 借鉴）

> 连跳上限——防止全自动流程无限空转、永不交还控制权。

全自动流程连续推进 N 步后，必须暂停交还控制权给用户。这是 Spirit 动态编排自主循环的安全底线。

### 3.2 系统层面

| 维度 | 现状 | 目标 |
|------|------|------|
| 自动推进 | 从 PrePlanningGate → 规划 → 组队 → 执行 → 合成，全自动无限制 | 引入 hopCount，连续自动推进 N 步后强制暂停 |
| 控制权交还 | 只有 clarification gate 和 confirmation_guard 在特定条件下触发 | 通用连跳上限：超过 MaxAutoHops 强制插入 confirm |
| 用户感知 | 用户不知道系统自动推进了多少步 | 暂停时告知"已连续自动推进 N 步" |

**具体改动**：

1. **引入 hopCount 状态**：在 Spirit 编排会话中跟踪"连续自动推进步数"
2. **每次自动推进时 hopCount++**：无需人工确认的阶段转换计为一次 hop
3. **超过 MaxAutoHops 时强制暂停**：插入 confirm Activity，等待用户确认
4. **用户确认后 hopCount 重置**：用户可以选择"继续自动"或"逐步确认"

### 3.3 架构层面

**改动点 1：hopCount 状态定义**

文件：`internal/biz/orchestration_state.go`（新增）或 `internal/service/spirit_team.go`

```go
// AutoHopState tracks the count of consecutive automatic hops in a Spirit
// orchestration session. When the count exceeds MaxAutoHops, the engine
// must pause and return control to the user.
type AutoHopState struct {
    SpiritSessionID string    `json:"spirit_session_id"`
    HopCount        int       `json:"hop_count"`
    MaxAutoHops     int       `json:"max_auto_hops"`     // default: 5
    LastHopAt       time.Time `json:"last_hop_at"`
    PausedAtHop     int       `json:"paused_at_hop"`     // hop count when paused (0 = not paused)
}

// Hop 计数规则：
// - PrePlanningGate 判定为 ForcePlanning → hop+1（自动进入规划路径）
// - TaskPlanner.Plan() 完成 → hop+1（自动进入团队组建）
// - 团队组建完成 → hop+1（自动开始执行）
// - checkAllTeamsCompleted 触发 synthesis → hop+1（自动进入合成）
// - 人工确认后 → hopCount 重置为 0
```

**改动点 2：自动推进拦截**

文件：`internal/service/spirit_team.go` — `checkAllTeamsCompleted`

```go
func (s *TeamStarter) checkAllTeamsCompleted(ctx context.Context, spiritSessionID string) {
    result := s.team.SpiritUC.CheckAllTeamsCompleted(ctx, spiritSessionID)
    
    // 新增：连跳上限检查
    hopState := s.getAutoHopState(ctx, spiritSessionID)
    if hopState.HopCount >= hopState.MaxAutoHops {
        // 连跳上限触发：暂停并请求用户确认
        s.emitAutoHopLimitConfirm(ctx, spiritSessionID, hopState)
        return
    }
    
    // 正常推进
    hopState.HopCount++
    s.saveAutoHopState(ctx, hopState)
    
    // 原有逻辑：触发 synthesis
    // ...
}
```

**改动点 3：hopCount 重置**

文件：`internal/service/chat_orchestrator_turn.go`

- 用户回复确认消息后，重置该 spirit session 的 hopCount
- 用户发送新消息（非确认回复），也重置 hopCount（新一轮交互）

**改动点 4：hopCount 持久化**

- hopCount 存入 `spirit_sessions` 表的 Meta 或独立的 `auto_hop_states` 表
- 进程重启后恢复，避免重启绕过连跳上限

### 3.4 用户层面

**场景 1：正常自动推进（hopCount < MaxAutoHops）**

```
用户：帮我调研医疗云市场
系统：【自动】复杂度评估 → 中等任务，走规划路径
系统：【自动】拆解为 5 步 DAG
系统：【自动】组建医疗云研究小组（4 成员）
系统：【自动】并行执行中…
系统：【自动】合成简报
系统：医疗云市场调研简报：……
```

**场景 2：连跳上限触发（hopCount >= MaxAutoHops）**

```
用户：帮我分析 10 个行业的市场趋势
系统：【自动】复杂度评估 → 复杂任务，走规划路径
系统：【自动】拆解为 12 步 DAG
系统：【自动】组建第一批团队（3 个行业组）
系统：【自动】第一批执行完成，合成第一批结果
系统：【自动】组建第二批团队（3 个行业组）
系统：⏸️ 已连续自动推进 5 步。是否继续自动执行剩余 4 个行业的分析？
       [继续自动] [逐步确认] [暂停任务]
```

**场景 3：用户选择"逐步确认"**

```
系统：⏸️ 已切换为逐步确认模式
系统：第二批团队已组建完成（金融/医疗/教育），是否开始执行？
       [开始执行] [查看团队配置] [跳过]
```

---

## 四、与 Harness 的对照映射

| Harness 概念 | Aranea-Agents 对应 | 差距/方案 |
|-------------|-------------------|----------|
| Stage（阶段） | DAG Step / TeamStage | ✅ 已有 |
| StageMachine（阶段状态机） | PlanExecutor / TeamStageStateMachine | ✅ 已有 |
| Gate（门控） | PrePlanningGate / clarification_gate / confirmation_guard | ⚠️ 有门控但不统一，无双通道信号 |
| Runner（执行者） | Team member（self=精灵，delegate=团队成员） | ✅ 已有 |
| Sub-agent（子智能体） | Team member agent | ✅ 已有 |
| **Signals 双通道** | **无** | **❌ 方案：Activity Meta["signal"] + WS 过滤** |
| 工具白名单 | 团队工具授权 + permission_guard | ✅ 已有 |
| confirmed_plan | Graph StateFields 共享黑板 + deliverable | ✅ 已有 |
| **连跳上限** | **无** | **❌ 方案：AutoHopState + 强制暂停** |

---

## 五、实施优先级

| 优先级 | 改动 | 影响面 | 工作量 |
|--------|------|--------|--------|
| **P0** | 连跳上限（AutoHopState） | spirit_team.go + chat_orchestrator_turn.go | 中 |
| **P1** | Signals 双通道（Activity Meta["signal"]） | activity.go + projector.go + spirit_team.go | 中 |
| **P2** | WS 过滤 signal 数据 | WS 推送层 | 小 |
| **P3** | hopCount 持久化 | data 层 + migration | 小 |

---

## 六、对竞赛 PPT 的启示（叙事结构）

Harness 视频的叙事结构值得借鉴：

```
问题（为什么需要） → 核心方案（怎么做） → 概念图鉴（关键设计） → 设计哲学（为什么这样设计） → 价值对比（与竞品差异）
```

当前 PPT 的结构是**按评审维度组织**（D1-D5），这是"答卷式"叙事。可以在保持评审维度的同时，在 §2 方案设计部分借鉴 Harness 的叙事逻辑：

1. **问题先行**：先讲"裸循环的三个缺口"（无质量门控、无流程一致性、无复用性）
2. **方案对照**：我们的三层编排引擎如何解决这三个缺口
3. **设计哲学**：硬驱动（流程归引擎，执行归模型）→ 声明式可插拔 → 正交矩阵 → 零侵入
4. **差异化**：与 Claude Code 等产品的根本差异对比表

这样让评审在"看答案"的同时，也能"理解设计思路"。
