# Research: Chat 精灵模式实施现状评估与可观测性 UX 方案

> **日期**：2026-06-08 | **版本**：v1.2 | **作者**：AI 调研
> **关联**：[59-chat-spirit-mode.md](../development/59-chat-spirit-mode.md) · [59-chat-spirit-mode.design.md](../development/59-chat-spirit-mode.design.md)
>
> **v1.2 修订说明**：基于对 Agent 动作可观测性、任务中断恢复、历史消息持久化的三次深度验证，新增附录 B（可观测性现状矩阵）和附录 C（中断恢复机制评估），修正 S-02/S-04 方案的业务合理性评估。

---

## 摘要

本报告包含三部分：(1) 对 Chat 精灵模式（M59）12 项验收标准的逐项代码核实，发现 4 项标记与实际不符、3 项组件已编写但未集成；(2) 对 Claude Code / OpenAI Codex / Cursor 3 / Trae 四款竞品的聊天界面可观测性设计深度调研，提炼出三层可观测性架构和四种折叠策略；(3) 基于评估与调研，提出 6 项可观测性 UX 方案，遵循"可观测性强但不影响主要内容显示"原则，每项方案均给出实现方式、对主内容的影响量级和优先级建议。

---

## 第一部分：需求实施真实情况评估

### 评估方法

对前端 `web/src/components/spirit/`、`web/src/components/chat/`、`web/src/stores/` 和后端 `internal/biz/`、`internal/service/`、`internal/tools/` 逐项搜索组件、API、Store 状态和测试文件，与需求文档 [59-chat-spirit-mode.md](../development/59-chat-spirit-mode.md) §7 验收标准索引逐条对照。

### 评估结果

| ID | 摘要 | 文档标记 | 实际状态 | 关键差距 |
|----|------|---------|---------|---------|
| SP-01 | 左侧仅显示精灵+团队树 | ✅ | **部分实现** | Agent 列表仍完整显示，精灵并非唯一入口 |
| SP-02 | 精灵区分简单/任务型对话 | ✅ | **后端已实现，前端无独立展示** | 后端 Plan 阶段有复杂度评估，前端无任务分析卡片 |
| SP-03 | 团队卡片展示名称/状态/成员/进度 | ✅ | **已实现，细节偏差** | 头像数量(5 vs 需求4)、排序逻辑不完整 |
| SP-04 | 任务执行面板三区布局 | ✅ | **组件存在但未集成** | ParallelTeamOverview/TeamProgressCard/SynthesisResultCard 已编写但 TaskExecutionPanel 未使用 |
| SP-05 | 成员树形展开+状态 | ❌ | **确认未实现** | 展开功能存在但不包含成员树 |
| SP-06 | 成员只读面板 | ❌占位符 | **确认占位符** | 仅"成员详情面板（P1 实现）"文字 |
| SP-07 | 多任务并行+Agent复用隔离 | ✅ | **后端完整，前端缺共用标识** | 缺"共用 Agent"展示 |
| SP-08 | 团队生命周期 | ⚠️ | **部分实现** | 取消+自动归档已实现，手动归档+重试未实现 |
| SP-09 | 面包屑+返回精灵 | ⚠️ | **与文档一致** | 返回精灵已实现，面包屑未实现 |
| SP-11 | 三阶段编排 | ✅ | **后端完整，前端无UI** | Plan/Allocate/Orchestrate 逻辑完整，但前端无对应可视化 |
| SP-12 | DAG编排图+并行团队概览 | ✅ | **组件孤立，未集成** | DAGDiagramCard 和 ParallelTeamOverview 存在但未被任何面板使用 |
| SP-13 | 综合结果合成 | ✅ | **后端完整，前端集成位置不同** | SynthesisResultCard 在精灵对话面板展示，不在三区布局中 |

### 关键差距详解

#### 差距 1：SP-04 三区布局组件已编写但未集成（最大差距）

需求描述的任务执行面板三区布局：

```
┌─ ParallelTeamOverview ─────────────────────┐
│ 并行团队计数 + DAG 依赖图 + 并行配额进度条  │
├─ TeamProgressCard (×N) ────────────────────┤
│ 团队名称 + 状态 + 进度条 + 成员头像 + 取消  │
├─ SynthesisResultCard ──────────────────────┤
│ 合成策略标签 + 各团队结果摘要               │
└────────────────────────────────────────────┘
```

实际 `TaskExecutionPanel.vue` 使用的是简化版布局：

```
┌─ 顶部概览 ─────────────────────────────────┐
│ 团队名称 + 状态 + 进度 + "返回精灵"按钮     │
├─ 执行时间线 ────────────────────────────────┤
│ ChatExecutionCard 列表                      │
├─ 对话输出 ──────────────────────────────────┤
│ 可折叠的 assistant 消息                     │
└────────────────────────────────────────────┘
```

`ParallelTeamOverview`、`TeamProgressCard`、`SynthesisResultCard` 三个组件均已编写且功能完整，但 `TaskExecutionPanel` 未引用它们。

#### 差距 2：SP-01 精灵并非唯一入口

`ChatEntitySidebar.vue` 同时展示精灵入口 + Agent 列表（含"系统 Agent"和"系统内置"分组）+ 团队列表。需求要求"左侧列表仅显示精灵入口（`__spirit__`），不显示其他 Agent 和 Team"。

#### 差距 3：SP-12 DAG 组件孤立

`DAGDiagramCard.vue` 和 `ParallelTeamOverview.vue` 功能完整（含 DAG 文本图、并行配额进度条、合成结果），但未被 `TaskExecutionPanel` 或 `ChatMessagePanel` 引用，属于"孤立组件"。

#### 差距 4：SP-11 三阶段编排前端无可视化

后端 `plan_and_execute` 工具完整实现了 Plan → Allocate → Orchestrate 三阶段，前端 Store 接收了 `planCreated`、`allocationCreated`、`orchestrationStarted` 等事件，但没有对应的 UI 展示。用户无法看到"精灵正在规划→正在分配→正在编排"的过程。

### 风险项

| # | 风险 | 严重度 | 说明 |
|---|------|--------|------|
| 1 | 后端 Spirit HTTP 路由不存在 | — | 前端通过通用 `/v1/teams` API + `spirit_session_id` 字段过滤获取数据，核心操作通过 Agent Tool 机制完成，不存在独立的 `/v1/spirit` 路由。这不是 bug，而是架构设计：Spirit 交互以 WS 事件 + Tool 调用为主通道，HTTP API 仅做辅助查询 |
| 2 | `spirit_team_usecase_test.go` 被 `//go:build ignore` 禁用 | 高 | 核心 usecase 无活跃测试 |
| 3 | 前端 Spirit 组件零测试覆盖 | 中 | 所有 spirit 组件和 store 没有单元测试 |
| 4 | 孤立组件增加维护负担 | 低 | ParallelTeamOverview 等组件存在但未使用，可能随 API 变更而失效 |

---

## 第二部分：竞品聊天界面可观测性设计调研

### 调研范围

| 产品 | 版本/时期 | 界面形态 | 调研重点 |
|------|----------|---------|---------|
| Claude Code | 2025-2026 | 终端 TUI（自研 Ink 引擎） | 语境加载消息、可折叠工具输出、颜色编码 Diff |
| OpenAI Codex | 2025-2026 | CLI (Rust/Ratatui) + 云端 Web | 可验证证据、三级审批模型、任务卡片 |
| Cursor 3 | 2026 | IDE Agents Window | Agent 卡片状态机、Agent Tabs、Usage Ring |
| Trae | 2025-2026 | IDE 面板 + SOLO 独立端 | 对话流自动折叠、To-Do List、@Agent 提及 |

### 核心发现 1：可观测性三层架构

四款产品不约而同地形成了三层可观测性架构，每层解决不同粒度的信息需求：

```
┌─────────────────────────────────────────────────────┐
│ L1 环境层 (Ambient)                                  │
│ 不阻塞内容的轻量指示——颜色、图标、微动画、状态文案    │
│ 用户无需主动操作即可感知                              │
│                                                      │
│ 代表：Claude Code 语境加载消息                        │
│       Trae 对话流自动折叠                             │
│       Cursor Agent 状态标签                           │
├─────────────────────────────────────────────────────┤
│ L2 结构层 (Structural)                               │
│ 任务分解和进度追踪——卡片、列表、进度条、DAG 图       │
│ 用户需主动查看但不需要离开主界面                      │
│                                                      │
│ 代表：Trae To-Do List                                │
│       Cursor Agent 卡片                               │
│       Codex 任务卡片                                  │
├─────────────────────────────────────────────────────┤
│ L3 证据层 (Evidential)                               │
│ 可验证的执行结果——Diff、日志引用、测试输出            │
│ 用户主动展开查看，通常在任务完成后回溯                │
│                                                      │
│ 代表：Codex 终端日志引用                              │
│       Cursor Diff 预览                                │
│       Claude Code 颜色编码 Diff                       │
└─────────────────────────────────────────────────────┘
```

**设计原则**：L1 始终可见但零干扰，L2 按需可见但不遮挡，L3 主动展开才可见。

### 核心发现 2：四种折叠策略

"折叠"是解决可观测性 vs 可读性矛盾的核心模式。四种折叠策略各有适用场景：

| 策略 | 机制 | 代表产品 | 适用场景 | 对主内容的影响 |
|------|------|---------|---------|--------------|
| **按完成状态折叠** | 已完成步骤自动收起，仅保留摘要行 | Trae | 长对话、多步骤工作流 | 减少视觉噪音 60%+ |
| **按信息密度折叠** | 工具输出默认折叠为"工具名+状态+耗时"，点击展开 | Claude Code | 工具调用密集场景 | 折叠态仅占 1 行 |
| **按空间布局折叠** | 多 Agent 面板可并排/网格/堆叠切换 | Cursor 3 | 多 Agent 并行监控 | 零额外空间，空间复用 |
| **按信任度折叠** | 高自主模式下减少审批步骤和状态展示 | Codex | 用户对 Agent 高度信任时 | 渐进式减少展示 |

### 核心发现 3：竞品逐项对比

#### Claude Code — 终端 TUI

| 设计模式 | 实现方式 | 可观测性层级 | 对主内容影响 |
|---------|---------|------------|------------|
| 语境加载消息 | 思考时显示"Reading files…"而非通用 spinner | L1 | 零额外空间，替换空白等待 |
| 可折叠工具输出 | 工具调用结果默认折叠，只显示工具名+状态摘要 | L2→L3 | 折叠态 1 行，展开态完整 |
| 颜色编码 Diff | 文件编辑渲染为 +/- 行号的颜色编码 diff | L3 | 内联展示，不额外占位 |
| Braille 点阵 Spinner | 自研动画 braille-dot spinner | L1 | 极轻量，1 字符宽 |
| Thinking Block | 扩展思维推理过程以可折叠区块展示 | L2 | 默认折叠，点击展开 |
| 虚拟滚动 | VirtualMessageList 只渲染可见窗口 | 基础设施 | 6 小时长会话内存恒定 |

**最值得借鉴**：语境加载消息——让"等待"本身成为信息传递的载体。

#### OpenAI Codex — CLI + 云端 Web

| 设计模式 | 实现方式 | 可观测性层级 | 对主内容影响 |
|---------|---------|------------|------------|
| 可验证证据 | 完成后引用终端日志和测试输出作为证据 | L3 | 仅在完成后展示 |
| 三级审批模型 | suggest→auto-edit→full-auto | L1→L3 | 信任度越高，展示越少 |
| 任务卡片 | 每个任务独立卡片，含描述、状态、耗时 | L2 | 卡片式，可滚动 |
| Reasoning 展示 | 模型推理过程作为独立消息类型 | L2 | 独立消息，不混入对话 |
| Code/Ask 双模式 | "Code"执行编码，"Ask"仅问答 | L1 | 意图明确，减少无关操作 |

**最值得借鉴**：可验证证据——将可观测性从"过程展示"提升到"结果验证"。

#### Cursor 3 — Agent-First IDE

| 设计模式 | 实现方式 | 可观测性层级 | 对主内容影响 |
|---------|---------|------------|------------|
| Agent 卡片状态机 | Planning→Executing→Reviewing→Done | L1+L2 | 标签式，~20px/条目 |
| Agent Tabs | 多 Agent 可并排/网格/垂直堆叠 | L2 | 空间复用，零额外占用 |
| Usage Ring | 计划用量以环形图展示 | L1 | 固定位置，不随内容滚动 |
| 暂停/重启/Fork | 从卡片直接操作 Agent | L2 | 操作按钮，不占内容区 |
| Diff 预览 | 代码变更以内联 diff 展示 | L3 | 按需展开 |
| 后台 Agent | 最多 8 个并行，每个独立卡片 | L2 | 卡片列表，可折叠 |

**最值得借鉴**：Agent 卡片状态机——将 Agent 生命周期以状态标签形式可视化，是"环境可观测性"的典范。Usage Ring 事件（社区 200+ 点赞要求修复）证明成本可观测是刚需。

#### Trae — IDE 面板 + SOLO 独立端

| 设计模式 | 实现方式 | 可观测性层级 | 对主内容影响 |
|---------|---------|------------|------------|
| 对话流自动折叠 | 已完成步骤自动收起，仅当前活跃步骤展开 | L1 | 减少视觉噪音 60%+ |
| To-Do List 实时拆解 | 任务自动拆解为待办清单，实时更新进度 | L2 | 侧边栏，不占主内容 |
| 终端命令卡片 | Agent 执行终端命令显示为只读卡片 | L2 | 卡片式，可折叠 |
| @Agent 提及 | 类似社交媒体的 @提及方式调用不同 Agent | L1 | 符号式，零额外空间 |
| #Context 上下文标签 | #Web/#Doc/#Workspace 等标签统一上下文 | L1 | 标签式，~30px/标签 |
| Webview 实时预览 | 内置 Webview，代码修改后实时预览 | L3 | 独立面板 |

**最值得借鉴**：对话流自动折叠——最有效的"不干扰主对话"模式，已完成步骤自动收起保持视觉焦点。

### 核心发现 4：学术验证

CHI 2025 论文《Assistance or Disruption?》通过 Codellaborator 设计探针实验证明：**存在感指示器（Presence Indicators）和交互上下文支持能显著减轻主动式 AI Agent 带来的工作流干扰**，同时提升用户对 AI 过程的感知。这从学术角度验证了"环境可观测性"设计的价值——用户需要知道 Agent 在做什么，但不需要被 Agent 的过程信息淹没。

---

## 第三部分：可观测性 UX 方案

### 设计原则

基于评估和调研，提出以下设计原则：

| # | 原则 | 说明 | 来源 |
|---|------|------|------|
| DP-1 | **环境可观测性优先** | 状态信息以"环境感知"方式呈现（颜色、图标、微动画），不占用主内容区空间 | Claude Code 语境加载消息 |
| DP-2 | **渐进式信息披露** | 默认只展示 L1 环境层，用户主动交互才展开 L2/L3 | 四款竞品共同模式 |
| DP-3 | **完成即折叠** | 已完成的步骤/团队/工具调用自动收起，保持视觉焦点在活跃内容 | Trae 对话流自动折叠 |
| DP-4 | **状态即视觉** | 颜色、图标、动画三位一体传达状态，减少文字描述 | 需求文档 §3.2 已有原则 |
| DP-5 | **证据后置** | 过程信息轻量展示，详细证据（Diff、日志）仅在用户主动查看时展开 | Codex 可验证证据 |

### 方案总览

| # | 方案名 | 借鉴来源 | 可观测性层级 | 对主内容影响 | 优先级 |
|---|--------|---------|------------|------------|--------|
| S-01 | 对话流自动折叠 | Trae | L1 | 减少视觉噪音 60%+ | P0 |
| S-02 | 语境加载消息 | Claude Code | L1 | 零额外空间 | P0 |
| S-03 | Agent 状态标签 | Cursor 3 | L1 | 每条目 ~20px | P0 |
| S-04 | 底部状态栏 | VS Code | L1 | 固定 24px | P1 |
| S-05 | 侧边栏状态脉冲 | 原创 | L1 | 零持续影响 | P1 |
| S-06 | 可折叠工具输出 | Claude Code | L2→L3 | 折叠态 1 行 | P0 |

### S-01：对话流自动折叠

**借鉴**：Trae 对话流自动折叠

**问题**：当前精灵对话面板中，工具调用、团队组建卡片、执行步骤全部平铺展示，长对话时用户需要大量滚动才能找到当前活跃内容。

**方案**：

```
折叠前（当前）：
┌─ 🏗️ 组建团队 ────────────────────┐  ← 已完成
│ 任务：开发用户注册 API              │
│ 编排模式：sequential                │
│ 成员：Golang 工程师 → 代码审查员    │
│ 状态：执行中                  ⚡    │
└────────────────────────────────────┘
┌─ 🏗️ 团队已组建 ──────────────────┐  ← 已完成
│ 任务：开发用户注册 API              │
│ 团队：后端 API 开发团队             │
│ 状态：执行中                  ⚡    │
│ [查看团队执行面板 →]                │
└────────────────────────────────────┘
┌─ 🔧 工具调用: plan_and_execute ──┐  ← 已完成
│ 输入：{task: "开发用户注册 API"}    │
│ 输出：{team_id: "team_123", ...}   │
└────────────────────────────────────┘
┌─ ✅ 任务完成 ─────────────────────┐  ← 当前活跃
│ 团队：后端 API 开发团队             │
│ 结果：用户注册 API 开发完成          │
│ 耗时：3 分 20 秒                    │
│ [查看详情 →]                        │
└────────────────────────────────────┘

折叠后（方案）：
┌─ 🏗️ 组建团队 → 已完成 ✓ 3.2s ──┐  ← 1 行摘要
┌─ 🏗️ 团队已组建 → 后端API开发团队 ┐  ← 1 行摘要
┌─ 🔧 plan_and_execute → ✓ 1.5s ──┐  ← 1 行摘要
┌─ ✅ 任务完成 ─────────────────────┐  ← 当前活跃，完整展示
│ 团队：后端 API 开发团队             │
│ 结果：用户注册 API 开发完成          │
│ 耗时：3 分 20 秒                    │
│ [查看详情 →]                        │
└────────────────────────────────────┘
```

**折叠规则**：

| 内容类型 | 折叠条件 | 折叠态展示 | 展开态展示 |
|---------|---------|-----------|-----------|
| 团队组建卡片 | 团队状态 ≠ running | 图标 + "组建团队" + 团队名 + ✓/✗ + 耗时 | 完整卡片 |
| 工具调用卡片 | 工具状态 = completed/failed | 工具名 + ✓/✗ + 耗时 | 完整输入输出 |
| 团队完成卡片 | 始终折叠（非活跃内容） | 图标 + "任务完成" + 团队名 + 耗时 | 完整结果 |
| 精灵直接回复 | 不折叠 | — | 完整展示 |
| 当前活跃步骤 | 不折叠 | — | 完整展示 |

**实现要点**：
- `groupMessagesByTurn` 当前不区分"已完成"和"进行中"的 block（`TurnBlockGroup` 无状态字段），需要扩展：为每个 block 增加 `isCompleted: boolean` 计算属性（判断依据：block 内所有工具调用均 completed/failed，且 assistant 消息已到达）
- 折叠态为单行摘要，点击可展开
- 新消息到达时，前一个活跃步骤自动折叠
- 提供"展开全部"按钮，方便回溯

**对主内容影响**：减少视觉噪音 60%+（假设平均 5 个步骤中 4 个已完成折叠）

---

### S-02：语境加载消息

**借鉴**：Claude Code 语境加载消息

**问题**：当前精灵思考/执行时显示通用 spinner，用户不知道 Agent 在做什么，等待体验差。

**方案**：

```
当前（通用 spinner）：
┌─────────────────────────────┐
│ 🤔 思考中...                 │  ← 无信息量
└─────────────────────────────┘

方案（语境加载消息）：
┌─────────────────────────────┐
│ 🔄 正在处理任务…             │  ← butler.orchestration.started
└─────────────────────────────┘
┌─────────────────────────────┐
│ 🔍 正在分析任务复杂度…       │  ← spirit_plan_created
└─────────────────────────────┘
┌─────────────────────────────┐
│ 👥 正在分配 Agent 角色…      │  ← spirit_allocation_created
└─────────────────────────────┘
┌─────────────────────────────┐
│ 🏗️ 正在编排执行流程…         │  ← spirit_orchestration_started
└─────────────────────────────┘
┌─────────────────────────────┐
│ ⚡ 后端API开发团队 执行中… 40%│  ← spirit_team_progress
└─────────────────────────────┘
┌─────────────────────────────┐
│ ✅ 后端API开发团队 任务完成   │  ← spirit_team_completed
└─────────────────────────────┘
```

**消息映射表**：

| 事件 | 语境加载消息 | 图标 | 备注 |
|------|------------|------|------|
| `butler.orchestration.started` | "正在处理任务…" | 🔄 | 此事件在三阶段流程**开始前**一次性发布，仅携带 `task_prompt` 和 `mode`，无 `phase` 字段 |
| `spirit_plan_created` | "正在分析任务复杂度…" | 🔍 | Phase 1 Plan 完成后发布，携带 `complexity_level`、`strategy`、`subtask_count` |
| `spirit_allocation_created` | "正在分配 Agent 角色…" | 👥 | Phase 2 Allocate 完成后发布，携带 `allocation_count` |
| `spirit_orchestration_started` | "正在编排执行流程…" | 🏗️ | Phase 3 Orchestrate 启动时发布，携带 `strategy`、`team_ids` |
| `spirit_team_assembled` | "团队已组建，{team_name} 开始执行" | ⚡ | 团队组装完成，携带 `team_name`、`mode`、`total_steps` |
| `spirit_team_progress` | "{team_name} 执行中… {progress_pct}%" | ⚡ | 团队级进度更新，携带 `status`、`progress_pct`、`duration_ms`。**注意**：此事件是扁平事件，无 `step_start`/`step_complete` 子类型，不携带具体 Agent 名称 |
| `spirit_team_completed` | "{team_name} 任务完成" | ✅ | 携带 `duration_ms` |
| `spirit_team_failed` | "{team_name} 任务失败" | ✗ | 携带 `error`（可选） |

**v1.0 勘误**：v1.0 中将 `butler.orchestration.started` 误写为按阶段（Plan/Allocate/Orchestrate）发布，实际上该事件仅发布一次且无 `phase` 字段。三阶段的精确追踪应使用 `spirit_plan_created` → `spirit_allocation_created` → `spirit_orchestration_started` 事件链。此外，v1.0 中 `spirit_team_progress` 的 `step_start`/`step_complete` 子类型不存在，该事件是团队级进度更新，不携带 Agent 级步骤信息。

**v1.2 补充**：虽然 `spirit_team_progress` 不携带 Agent 级信息，但 `tool_call` / `tool_result` Envelope 携带 `AgentKey`/`AgentName`/`AgentID` 字段，且 `ActivityKind` 区分 `skill`/`tool`/`mcp`/`subagent`/`memory`/`knowledge` 类型。因此，**Agent 级语境消息在技术上可行**——只需监听 `tool_call`/`tool_result` 事件，按 `AgentName` 过滤，即可展示"Golang 工程师正在读取文件…"等消息。这比扩展 `spirit_team_progress` payload 更轻量，且复用了现有事件流。

**实现要点**：
- 利用现有 WS 事件流，将事件类型映射为语境加载消息
- 消息以流式方式替换（非追加），保持 1 行
- 消息样式：浅色背景 + 左侧彩色竖线（颜色按阶段区分）
- 与 S-01 自动折叠配合：加载消息完成后自动折叠为摘要行

**对主内容影响**：零额外空间，替换原有空白等待区域。

---

### S-03：Agent 状态标签

**借鉴**：Cursor 3 Agent 卡片状态机

**问题**：当前团队成员状态仅在展开后才能看到，用户无法一眼感知团队执行进度。

**方案**：

在团队卡片和任务执行面板中，为每个 Agent 显示状态标签：

```
┌─ 后端 API 开发团队 ────────────────────────┐
│                                             │
│  Golang 工程师  [Executing]  ← 蓝色标签     │
│  代码审查员     [Waiting]    ← 灰色标签     │
│  测试工程师     [Waiting]    ← 灰色标签     │
│                                             │
└─────────────────────────────────────────────┘
```

**状态标签定义**：

后端 `AgentNodeStatus`（`orchestration_status.go`）定义了 17 种细粒度状态，前端需将其聚合为用户友好的展示标签：

| 后端 AgentNodeStatus | 聚合展示标签 | 标签文案 | 颜色 | 图标 | 动画 |
|---------------------|------------|---------|------|------|------|
| idle, queued, scheduled | Queued | "排队中" | 灰色 | ○ | 无 |
| running, thinking, tool_running, transferring, retrying | Active | "执行中" | 蓝色 | ⚡ | 左边框呼吸动画 |
| waiting_input, waiting_review, waiting_assign, blocked | Suspended | "等待中" | 橙色 | ⏸ | 无 |
| success | Done | "已完成" | 绿色 | ✓ | 无 |
| failed, timed_out | Failed | "失败" | 红色 | ✗ | 无 |
| skipped | Skipped | "已跳过" | 灰色 | ⊘ | 无 |
| cancelled | Cancelled | "已取消" | 灰色 | ⊘ | 无 |

**v1.0 勘误**：v1.0 中定义的 5 种状态（idle/working/waiting/completed/failed）与后端实际状态体系不符。后端 `AgentNodeStatus` 有 17 种细粒度状态，需通过 `DisplayStatus` 聚合函数映射为 7 种展示标签。此外，`SpiritMember.status` 类型为 `string`（非枚举），后端发送的典型值为 "idle"、"running"、"error"，与 `AgentNodeStatus` 的 17 种状态是两套体系——前者来自 `DefinitionJSON`，后者来自编排观察台。S-03 方案需明确使用哪套状态源。

**实现要点**：
- **状态源选择**：团队卡片（`TeamTaskCard`）使用 `SpiritMember.status`（简单 3 值：idle/running/error），任务执行面板使用 `AgentNodeStatus`（17 值聚合为 7 种标签）
- 复用 `SessionStatusBadge` 组件的样式体系
- 标签宽度固定（~80px），避免布局抖动
- Active 状态的呼吸动画使用 CSS `@keyframes`，不使用 JS 动画
- 标签在团队卡片折叠态也可见（显示为紧凑模式：头像+状态色点）

**对主内容影响**：每条目增加 ~20px 标签高度。

---

### S-04：底部状态栏

**借鉴**：VS Code 底部状态栏模式

**问题**：用户需要切换到左侧栏才能看到全局并行状态，打断阅读流。

**方案**：

在聊天面板底部固定一行状态栏：

```
┌─ 聊天面板主内容区 ──────────────────────────┐
│                                              │
│  （精灵对话 / 任务执行面板 / 成员只读面板）   │
│                                              │
├──────────────────────────────────────────────┤
│ ⚡ 2 teams running │ 📊 2/3 quota │ 🔵 1.2k tokens │ ✅ Team A done  │
└──────────────────────────────────────────────┘
```

**状态栏字段**：

| 字段 | 内容 | 点击行为 |
|------|------|---------|
| 活跃团队 | "⚡ N teams running" | 切换到团队列表 |
| 并行配额 | "📊 N/M quota" | 展开配额详情 |
| Token 消耗 | "🔵 X.Xk tokens" | 展开消耗明细 |
| 最近事件 | "✅/❌ 最近完成的团队/步骤" | 跳转到对应卡片 |

**实现要点**：
- 使用 Quasar `q-bar` 组件，`position: sticky; bottom: 0`
- 高度固定 24px，不随内容滚动
- 字段按优先级排列，窄屏时自动隐藏低优先级字段
- 颜色编码：running=蓝色、completed=绿色、failed=红色
- 仅在精灵模式激活时显示
- **Token 消耗数据源**：`TeamRunStep` 表有 `token_in`/`token_out` 字段，`spirit_team_completed` 事件可扩展携带 token 统计；当前事件 payload 不含 token 字段，需后端扩展或在 `spirit_team_completed`/`spirit_teams_all_completed` 事件中增加 `total_token_in`/`total_token_out`

**对主内容影响**：固定 24px，不随内容滚动。

---

### S-05：侧边栏状态脉冲

**借鉴**：原创设计（结合 Cursor Usage Ring 的"注意力引导"思路）

**问题**：用户在阅读精灵对话时，左侧团队状态变化无法被感知，需要主动切换查看。

**方案**：

左侧团队卡片在状态变化时短暂脉冲高亮：

```
正常态：
┌─ 后端 API 开发团队 ────────┐
│ running  2/5 步骤           │  ← 标准背景色
└────────────────────────────┘

脉冲态（running → completed 瞬间）：
┌─ 后端 API 开发团队 ────────┐
│ completed 5/5 步骤          │  ← 绿色脉冲高亮，1.5s 后恢复
└────────────────────────────┘
```

**脉冲规则**：

| 状态变化 | 脉冲颜色 | 持续时间 |
|---------|---------|---------|
| → running | 蓝色 | 1.0s |
| → completed | 绿色 | 1.5s |
| → failed | 红色 | 2.0s |
| → interrupted | 橙色 | 1.5s |

**实现要点**：
- 使用 CSS `@keyframes pulse` 动画，不使用 JS
- 脉冲结束后自动移除动画 class
- 多个团队同时变化时，脉冲独立触发
- 脉冲期间卡片左侧边框加粗 2px（颜色同脉冲色）

**对主内容影响**：零持续影响，脉冲仅持续 1-2 秒。

---

### S-06：可折叠工具输出

**借鉴**：Claude Code 可折叠工具输出

**问题**：当前工具调用卡片（`ChatExecutionCard`）展开时占据大量空间，多个工具调用连续展示时严重影响对话可读性。

**v1.0 勘误**：v1.0 声称需要"增加折叠/展开状态"，实际上 `ChatExecutionCard` 已使用 `<q-expansion-item>` 作为根元素，**已内置折叠/展开功能**。当前的问题是：(1) 默认全部展开，没有"完成后自动折叠"逻辑；(2) 折叠态的 header 仍然较宽，没有精简为单行摘要。

**方案**：

```
折叠态（默认）：
┌─ 🔧 plan_and_execute ─── ✓ 1.5s ──┐  ← 1 行
┌─ 🔧 write_file ───────── ✓ 0.3s ──┐  ← 1 行
┌─ 🔧 run_tests ────────── ✓ 2.1s ──┐  ← 1 行

展开态（点击后）：
┌─ 🔧 plan_and_execute ─── ✓ 1.5s ──────────────┐
│ 输入：                                           │
│   task: "开发用户注册 API"                        │
│   complexity: "complex"                          │
│ 输出：                                           │
│   team_id: "team_123"                            │
│   members: ["golang-engineer", "code-reviewer"]  │
│   orchestration: "sequential"                    │
└──────────────────────────────────────────────────┘
```

**折叠规则**：

| 工具状态 | 默认态 | 展示内容 |
|---------|--------|---------|
| running | 展开 | 工具名 + spinner + 实时输出（流式） |
| completed | 折叠 | 工具名 + ✓ + 耗时 |
| failed | 折叠（红色高亮） | 工具名 + ✗ + 错误摘要 |

**实现要点**：
- **不需要新增折叠/展开功能**——`ChatExecutionCard` 已使用 `<q-expansion-item>` 实现折叠/展开
- 需要新增的是**自动折叠逻辑**：监听工具状态变化，completed/failed 时自动折叠（设置 `expanded = false`）
- 需要优化折叠态 header：当前 header 包含标题+摘要+状态图标+时长，信息密度已较高，但可进一步精简为单行
- 以 `tool_call_id` 为 upsert 键（已实现，无需修改）
- running 状态的工具调用始终展开，完成后自动折叠（配合 S-01）
- 提供"展开全部工具调用"按钮

**对主内容影响**：折叠态仅占 1 行（~32px），相比展开态（~200px）节省 80%+ 空间。

---

### 方案优先级与依赖关系

```
S-01 对话流自动折叠 ──────┐
                          ├──→ 核心体验提升（P0）
S-02 语境加载消息 ────────┘

S-06 可折叠工具输出 ──────→ 配合 S-01 实现完整折叠体验（P0）

S-03 Agent 状态标签 ──────→ 团队执行面板增强（P0）

S-04 底部状态栏 ──────────→ 全局状态感知（P1，依赖 S-03）
S-05 侧边栏状态脉冲 ─────→ 注意力引导（P1，独立）
```

**建议实施顺序**：
1. **第一批（P0）**：S-01 + S-02 + S-06 + S-03，解决核心可观测性体验
2. **第二批（P1）**：S-04 + S-05，增强全局感知和注意力引导

### 方案与现有差距的对应关系

| 现有差距 | 解决方案 | 说明 |
|---------|---------|------|
| SP-04 三区布局未集成 | S-01 + S-03 + S-06 | 不强求三区布局，而是通过折叠+标签+可折叠工具输出实现同等可观测性 |
| SP-11 三阶段编排前端无UI | S-02 | 语境加载消息覆盖 Plan→Allocate→Orchestrate 全过程 |
| SP-12 DAG 组件孤立 | S-03 + S-04 | Agent 状态标签提供 L1 环境感知，底部状态栏提供全局并行概览 |
| SP-01 精灵非唯一入口 | 不在本次方案范围 | 需独立决策：是否隐藏 Agent 列表 |

---

## 结论与建议

### 核心结论

1. **需求文档完成度标记需修正**：SP-04、SP-12 标记为 ✅ 但组件未集成，建议降级为 ⚠️
2. **竞品已形成"环境可观测性"共识**：L1 环境层（零干扰）→ L2 结构层（按需查看）→ L3 证据层（主动展开）三层架构是行业最佳实践
3. **"折叠"是核心解法**：Trae 的对话流自动折叠和 Claude Code 的可折叠工具输出是最有效的"可观测性强但不影响主要内容"的模式
4. **现有孤立组件应优先集成**：ParallelTeamOverview、DAGDiagramCard 等组件已编写，应在 S-01/S-03 方案中复用

### 下一步建议

| # | 行动项 | 优先级 | 前置条件 |
|---|--------|--------|---------|
| 1 | 修正需求文档 SP-04、SP-12 状态标记 | 高 | 无 |
| 2 | 启用 `spirit_team_usecase_test.go`（移除 `//go:build ignore`） | 高 | 无 |
| 3 | 实施 S-01 + S-02 + S-06 方案（核心折叠体验） | 高 | 无 |
| 4 | 实施 S-03 方案（Agent 状态标签） | 高 | #3 完成 |
| 5 | 集成 ParallelTeamOverview 到 TaskExecutionPanel | 中 | #4 完成 |
| 6 | 实施 S-04 + S-05 方案（全局感知增强） | 中 | #4 完成 |

---

## 参考资料

- [Claude Code CLI 架构解析](https://blog.csdn.net/EnjoyEDU/article/details/160969489)
- [How Claude Code is built - Gergely Orosz](https://objects.githubusercontent.com/github-production-repository-file-5c1aeb/921879783/22500516)
- [Claude Code TUI 架构深度分析](https://y-agent.github.io/inside-claude-code/08-cli-commands-ui.html)
- [OpenAI Codex CLI 架构分析](https://www.philschmid.de/openai-codex-cli)
- [OpenAI Codex Agent Loop 设计](https://www.zenml.io/llmops-database/building-production-ready-ai-agents-openai-codex-cli-architecture-and-agent-loop-design)
- [OpenAI Codex 官方介绍](https://openai.com/zh-Hans-CN/index/introducing-codex/)
- [Cursor 3 Changelog](https://cursor.com/changelog/3-0)
- [Cursor 3 Agent-First Review](https://www.openaitoolshub.org/en/blog/cursor-3-agent-first-review)
- [Cursor 3 Background Agents Review](https://effloow.com/articles/cursor-3-review-background-agents-2026)
- [Cursor Usage Indicator 问题](https://dredyson.com/why-the-missing-usage-indicator-in-cursors-agents-window-will-change-everything-in-2025/)
- [Trae Agent 2.0 博客](https://www.trae.ai/blog/product_thought_0617)
- [Trae @Agent 设计哲学](https://www.trae.ai/blog/product_thought_0421)
- [Trae Changelog](https://www.trae.ai/changelog)
- [CHI 2025: Assistance or Disruption?](http://arxiv.org/pdf/2502.18658)

---

## 附录 A：v1.0 勘误表

基于对前后端代码的二次深度验证，v1.0 中存在以下事实错误，已在 v1.1 中修正：

| # | 错误描述 | v1.0 原文 | 实际情况 | 影响方案 | 修正措施 |
|---|---------|----------|---------|---------|---------|
| E-01 | `butler.orchestration.started` 被描述为按阶段发布 | "Plan/Allocate/Orchestrate 三阶段分别发布" | 该事件仅在三阶段流程**开始前**发布一次，携带 `task_prompt` 和 `mode`，无 `phase` 字段 | S-02 语境加载消息 | 改用 `spirit_plan_created` → `spirit_allocation_created` → `spirit_orchestration_started` 事件链 |
| E-02 | `spirit_team_progress` 被描述为有 `step_start`/`step_complete` 子类型 | "step_start: {agent_name} 正在… / step_complete: {agent_name} 完成" | 该事件是扁平事件，携带 `team_id`、`status`、`progress_pct`、`duration_ms`，无子类型，不携带 Agent 名称 | S-02 语境加载消息 | 改为团队级进度展示 "{team_name} 执行中… {progress_pct}%" |
| E-03 | Agent 状态标签定义为 5 种 | "idle/working/waiting/completed/failed" | 后端 `AgentNodeStatus` 有 17 种细粒度状态，需聚合为 7 种展示标签；`SpiritMember.status` 是另一套简单体系（idle/running/error） | S-03 Agent 状态标签 | 重新定义 7 种聚合标签，明确两套状态源的使用场景 |
| E-04 | `ChatExecutionCard` 被描述为需要增加折叠/展开功能 | "复用现有 ChatExecutionCard 组件，增加折叠/展开状态" | 该组件已使用 `<q-expansion-item>` 实现折叠/展开，实际需要的是"完成后自动折叠"逻辑 | S-06 可折叠工具输出 | 修正为实现要点：新增自动折叠逻辑，而非新增折叠功能 |
| E-05 | 后端 `/v1/spirit` HTTP 路由被描述为"可能缺失" | "前端 API 调用可能 404" | Spirit 没有独立 HTTP 路由，这是架构设计：前端通过通用 `/v1/teams` API + `spirit_session_id` 过滤获取数据，核心操作通过 Agent Tool 机制完成 | 第一部分风险项 | 降级为"非风险"，修正为架构说明 |
| E-06 | `groupMessagesByTurn` 被描述为可直接用于自动折叠 | "在 groupMessagesByTurn 分组基础上，对已完成分组自动折叠" | `TurnBlockGroup` 无状态字段，不区分"已完成"和"进行中"的 block | S-01 对话流自动折叠 | 需扩展 `TurnBlockGroup` 增加 `isCompleted` 计算属性 |

### 业务合理性补充评估

| 评估项 | 结论 | 说明 |
|--------|------|------|
| S-01 对话流自动折叠 | **合理** | 核心价值明确，但需注意 `TurnBlockGroup` 扩展的复杂度——需在 `groupMessagesByTurn.ts` 中增加状态判断逻辑，可能影响现有消息分组的性能 |
| S-02 语境加载消息 | **合理，范围恢复** | v1.1 评估为"范围缩小"，但 v1.2 验证发现 `tool_call`/`tool_result` Envelope 已携带 `AgentName`/`ActivityKind` 字段，Agent 级语境消息**技术上可行**，无需后端扩展。只需前端监听 `tool_call` 事件并按 `AgentName` 过滤 |
| S-03 Agent 状态标签 | **合理，但需明确状态源** | 两套状态体系（`SpiritMember.status` vs `AgentNodeStatus`）的使用场景需在实现时明确：侧边栏团队卡片用简单状态，任务执行面板用细粒度状态 |
| S-04 底部状态栏 | **合理，Token 字段需后端扩展** | `TeamRunStep` 表有 token 字段，但 WS 事件 payload 不含 token 统计，需后端在 `spirit_team_completed`/`spirit_teams_all_completed` 事件中增加 token 字段 |
| S-05 侧边栏状态脉冲 | **合理** | 纯前端实现，无后端依赖，风险最低 |
| S-06 可折叠工具输出 | **合理，工作量低于预期** | `ChatExecutionCard` 已有折叠/展开功能，仅需增加"完成后自动折叠"逻辑，实现成本远低于 v1.0 估计 |

### 中断恢复对方案的影响

| 场景 | 当前恢复机制 | 对方案的影响 |
|------|------------|------------|
| 用户点击 Stop | WS cancel → CancelTeam API → 团队+子 session 标记 interrupted | S-01 折叠逻辑需处理 interrupted 状态的 block（应视为"已完成"并折叠） |
| 软件崩溃/服务器重启 | SessionStatusGuard 将 running→interrupted；Orchestrator.Recover 从 checkpoint 恢复 | S-02 语境加载消息需在重连后重新加载团队状态；S-04 底部状态栏需显示 interrupted 团队数 |
| WS 断线重连 | 指数退避 + lastEventId 事件回放 + reloadTeams | S-02 语境消息在回放期间应静默（避免重复闪烁）；S-05 脉冲在回放期间应禁用 |
| 用户重新打开历史 session | loadMessages 从数据库加载历史消息 | S-01 折叠逻辑需在加载历史消息时正确判断 block 完成状态；S-06 工具卡片需从 OptionsJSON.tool_event 恢复折叠态 |

---

## 附录 B：Agent 动作可观测性现状矩阵

### B.1 可观测性层级

| 动作类型 | 实时 WS 事件 | 持久化 | UI 展示 | 粒度 |
|---------|------------|--------|---------|------|
| 工具调用 | `tool_call` / `tool_result` | `tool_invocations` 表 + `chat_messages` 表（`act-{tool_call_id}`） | `ChatExecutionCard`（含 DiffViewer） | per-tool-call |
| Skill 调用 | 同上（`activity_kind=skill`） | `skill_invocations` 表 + `tool_invocations` 表 | `ChatExecutionCard`（Skill 图标） | per-invocation |
| 文件读写 | 同上（`activity_kind=tool`） | 同上 | `ChatExecutionCard`（文件图标 + 路径摘要 + DiffViewer） | per-file-op |
| MCP 调用 | 同上（`activity_kind=mcp`） | `tool_invocations` 表 | `ChatExecutionCard` | per-call |
| 子 Agent 调用 | 同上（`activity_kind=subagent`） | `tool_invocations` 表 | `ChatExecutionCard` | per-call |
| 成员消息 | `member_message_start` / `member_delta` / `member_message_done` | `chat_messages` 表（`OptionsJSON.team_member`） | 消息流（色条分栏 + memberLabel） | per-message |
| 团队步骤 | `team_step_started` / `team_step_finished` | `team_run_steps` 表 | Observatory Timeline | per-step |
| 编排状态 | `orchestration_agent_status` | `orchestration_steps` 表（500ms 批量写入） | Observatory Kanban + Graph Canvas | per-node |
| Graph 执行 | `graph_node_start` / `graph_node_end` / `graph_step` / `graph_task_status` | `graph_executions` 表 | Observatory Graph Canvas | per-node |

### B.2 关键发现

1. **工具/Skill/文件操作已完全可观测**：每次工具调用都有完整的 WS 事件 + 数据库持久化 + UI 展示，粒度为 per-tool-call
2. **`EnvelopeToolCall` 信息密度极高**：包含 `AgentKey`/`AgentName`/`ActivityKind`/`DisplayLabel`/`Summary`/`DurationMS`/`Status` 等字段，足以支撑 S-02 Agent 级语境消息
3. **成员消息过滤已实现**：`OptionsJSON.team_member` 字段存在且可用，SP-06 成员只读面板的实现基础已具备
4. **Observatory 页面功能完整但入口隐蔽**：`TeamRunObservatoryPage` 有 Graph Canvas + Kanban + Timeline + Summary + HITL，但用户需从团队详情页跳转，不在精灵对话面板内

### B.3 可观测性缺口

| 缺口 | 说明 | 建议 |
|------|------|------|
| 工具调用原始 `result_json` 不持久化到 Message 表 | 只有摘要，无结构化结果 | 当前设计合理（避免大 JSON 存储），通过 `tool_invocations` 表可查询 |
| TeamRunStep 不含单个工具调用详情 | 只有 `ToolCallCount` | 通过 `tool_invocations` 表按 `session_id` 查询可补充 |
| 无内置按成员过滤消息的 UI | `team_member` 字段存在但无 UI 过滤组件 | SP-06 实现时需新增过滤 UI |
| Skill 调用无独立 WS 事件类型 | 通过 `activity_kind=skill` 区分 | 当前设计合理，无需独立事件 |
| `ToolRegistered`/`ToolUpdated`/`ToolRemoved` 事件未实现 | 需求 2.10 定义但未实现 | P2 优先级 |

---

## 附录 C：中断恢复机制评估

### C.1 恢复机制全景

```
┌─ 服务器启动 ─────────────────────────────────────────────┐
│ SessionStatusGuard.OnStartup()                            │
│  ├─ RecoverOrphanedRunningSessions → running→interrupted  │
│  ├─ recoverOrphanedRunningTeams → running→interrupted     │
│  └─ recoverInterruptedOrchestrations → checkpoint 恢复    │
├─ 服务器关闭 ─────────────────────────────────────────────┤
│ SessionStatusGuard.OnShutdown()                           │
│  └─ running→interrupted (StatusReasonServerShutdown)      │
├─ WS 断线 ───────────────────────────────────────────────┤
│ ws-transport.ts: 指数退避重连 + lastEventId 事件回放      │
│ spirit store: reloadTeams() 重新加载团队状态               │
│ chat stream: loadMessages() 增量同步消息                   │
├─ 用户取消 ──────────────────────────────────────────────┤
│ WS cancel → CancelTeam → 团队+子session→interrupted       │
├─ 团队超时 ──────────────────────────────────────────────┤
│ 内存定时器 → TeamStatusFailed → HandleTeamTimeout         │
│ ⚠️ 定时器不持久化，重启后丢失                              │
└─ 恢复执行 ──────────────────────────────────────────────┘
  ResumeTeamRunExecution API → 需要 graph_execution_id
  Orchestrator.Recover → 需要 checkpoint_id
  ⚠️ Phase 1/2 中断恢复未实现（TODO: DEV-07）
```

### C.2 恢复能力评估

| 场景 | 恢复能力 | 数据丢失风险 | 说明 |
|------|---------|------------|------|
| 用户点击 Stop | **完全可恢复** | 无 | 团队标记 interrupted，Session/Message/TeamRun/TeamRunStep 全部保留 |
| 服务器优雅关闭 | **完全可恢复** | 无 | OnShutdown 将 running→interrupted，所有数据已持久化 |
| 服务器崩溃 | **部分可恢复** | 低 | running→interrupted（启动时恢复）；Orchestrator 可从 checkpoint 恢复；但 Phase 1/2 草稿（TaskPlan/AllocationPlan）不恢复 |
| WS 断线 | **完全可恢复** | 无 | 事件回放 + 增量同步，无数据丢失 |
| 超时定时器丢失 | **低风险** | 无 | 重启后 running team 会被标记 interrupted，不会永远卡住；但不会自动触发超时失败 |
| 用户关闭浏览器 | **完全可恢复** | 无 | 重新打开后从数据库加载历史消息和团队状态 |

### C.3 对方案的关键约束

1. **S-01 对话流自动折叠**：需处理 interrupted 状态的 block——interrupted 应视为"已完成"并折叠，但需显示中断标记（而非 ✓）
2. **S-02 语境加载消息**：WS 重连事件回放期间应静默（`onReplayState` 回调），避免回放历史事件时产生闪烁
3. **S-04 底部状态栏**：需显示 interrupted 团队计数（如"⚡ 1 running │ ⏸ 1 interrupted"），让用户知道有中断的团队可恢复
4. **S-05 侧边栏状态脉冲**：WS 回放期间应禁用脉冲动画，避免历史状态变化触发误脉冲
5. **S-06 可折叠工具输出**：加载历史消息时，需从 `OptionsJSON.tool_event` 恢复工具卡片的折叠态（而非默认全部展开）

### C.4 建议新增：S-07 中断恢复提示

**问题**：当团队因崩溃/超时被标记为 interrupted 时，用户可能不知道可以恢复执行。

**方案**：在任务执行面板中，interrupted 状态的团队显示恢复提示卡片：

```
┌─ ⏸ 团队已中断 ──────────────────────────────┐
│ 后端 API 开发团队 因服务器重启而中断           │
│ 已完成 3/5 步骤，可从断点恢复执行              │
│ [恢复执行]  [取消团队]                         │
└──────────────────────────────────────────────┘
```

**实现要点**：
- 复用 `SessionStatusBadge` 的 interrupted 状态样式
- "恢复执行"按钮调用 `ResumeTeamRunExecution` API（需 `graph_execution_id`）
- 如果团队无 `graph_execution_id`，显示"此团队不支持断点恢复"
- 恢复成功后发布 `spirit_team_progress` 事件（status=running），前端更新状态

**对主内容影响**：仅 interrupted 状态时显示，正常执行时不可见。
