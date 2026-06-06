# M59: Chat 管家模式 — 实现设计

> 对应需求：[59-chat-spirit-mode.md](./59-chat-spirit-mode.md)
> 遵循：[AI-DEVELOPMENT-SPECIFICATION.md](../guides/AI-DEVELOPMENT-SPECIFICATION.md) · [AGENT_RUNTIME_BOUNDARY.md](../AGENT_RUNTIME_BOUNDARY.md)
> **实现差距与迭代计划**以 [59-chat-spirit-mode-development.md](./59-chat-spirit-mode-development.md) 为准

---

## 一、模块概述

### 1.1 设计定位

以精灵（Spirit）为 Chat 页面唯一对话入口，左侧列表从"Agent/Team 平铺"重构为"精灵 + 任务团队树"：

- **精灵简视图**：用户只与精灵对话，不感知 Agent/Team 细节
- **任务团队视图**：精灵自动组建的团队在左侧动态展示，支持展开成员树
- **执行观测视图**：点击团队/成员进入任务执行面板或只读面板

**前置依赖**：[system-builtin-agents-design](../superpowers/specs/2026-05-31-system-builtin-agents-design.md) 中精灵 Agent 定义、`assemble_team` 工具、Session 树状模型。

### 1.2 分层与依赖

```
api/kratos/session/v1/session.proto   ← Session 扩展字段（parent_session_id 等）
api/kratos/team/v1/team.proto         ← Team 扩展字段（spirit_session_id 等）
        ↓
internal/service/
  chat.go                              ← 识别 __spirit__ → buildSpiritTeam
  spirit_team.go                       ← 精灵团队组装逻辑（assemble_team 回调）
        ↓
internal/biz/
  session/usecase.go                   ← Session 树查询（ListByParentSessionID）
  team_usecase.go                      ← Create 支持 AutoCreated / SpiritSessionID
        ↓
internal/agent/
  trpc_build.go                        ← 精灵 Agent 构建（BuildTRPCLLMAgentCached）
  orchestration.go                     ← 编排管家工具注册
        ↓
internal/team/
  runner.go                            ← Team Run（复用现有）
  status_projector.go                  ← Agent 状态投影（复用现有）
        ↓
web/src/
  features/spirit/                     ← 精灵域（api.ts / types.ts / composable）
  stores/spirit/                       ← useSpiritTeamStore
  components/spirit/                   ← 精灵专用组件
  components/chat/                     ← Chat 面板扩展
```

**红线**：`internal/biz` 不 import `pkg/trpc-agent-go`；精灵构建仅在 `internal/service`；Team 编译仅在 `internal/team`。

### 1.3 影响域

| 包 | 变更类型 | 说明 |
|----|----------|------|
| `internal/biz/session` | 扩展 | Session 树查询、TaskSummary / TeamDisplayName 字段 |
| `internal/biz/team` | 扩展 | AutoCreated / SpiritSessionID 字段 |
| `internal/service` | 新增 | spirit_team.go、精灵对话路由逻辑 |
| `internal/agent` | 扩展 | 精灵 Agent 种子数据、工具注册 |
| `internal/event` | 扩展 | spirit_team_assembled 等新 EnvelopeType |
| `web/src/features/spirit` | 新增 | 类型、API、composable |
| `web/src/stores/spirit` | 新增 | useSpiritTeamStore |
| `web/src/components/spirit` | 新增 | 6 个新组件 |
| `web/src/components/chat` | 修改 | ChatEntitySidebar 重构、ChatMessagePanel 三模式 |
| `api/kratos/session/v1` | 扩展 | Session Proto 字段 |
| `api/kratos/team/v1` | 扩展 | Team Proto 字段 |

**不改动**：`internal/server` 直连 runtime；`internal/data` 除新增字段外无 schema 变更；Team 编译/运行流程不变。

---

## 二、Session 树状模型

### 2.1 数据结构

基于 [system-builtin-agents-design](../superpowers/specs/2026-05-31-system-builtin-agents-design.md) 已定义的 `ParentSessionID` / `RootSessionID` / `AgentDepth`：

```
Spirit Session (root)
  ├── ParentSessionID: null
  ├── RootSessionID: self.ID
  ├── AgentDepth: 0
  └── OwnerType: "agent", AgentID: __spirit__
      │
      ├── Team Session A (child)
      │   ├── ParentSessionID: spirit_session.ID
      │   ├── RootSessionID: spirit_session.ID
      │   ├── AgentDepth: 1
      │   ├── OwnerType: "team", TeamID: team_A.ID
      │   └── MetadataJSON.child_session_ids: [...]
      │
      └── Team Session B (child)
          └── ...同上
```

### 2.2 Session 扩展字段

| 字段 | 类型 | 存储位置 | 说明 |
|------|------|---------|------|
| `TaskSummary` | `string` | `sessions.metadata_json.task_summary` | 精灵生成的任务摘要 |
| `TeamDisplayName` | `string` | `sessions.metadata_json.team_display_name` | 团队显示名称 |
| `ChildSessionIDs` | `[]string` | `sessions.metadata_json.child_session_ids` | 子 Session ID 列表 |

### 2.3 Session 树查询接口

```go
type SessionTreeReader interface {
    ListByParentSessionID(ctx context.Context, parentSessionID string) ([]Session, error)
    GetRootSession(ctx context.Context, sessionID string) (Session, error)
}
```

Wire 绑定：`SessionUsecase` 实现 `SessionTreeReader`，通过 `SessionRepository` 查询。

---

## 三、Team 扩展

### 3.1 新增字段

| 字段 | 类型 | 说明 |
|------|------|------|
| `SpiritSessionID` | `string` | 创建该团队的精灵 Session ID |
| `TaskDescription` | `string` | 任务描述（来自 `assemble_team` 调用） |
| `AutoCreated` | `bool` | 是否由精灵自动创建 |

### 3.2 Team 创建流程（精灵路径）

```
精灵调用 assemble_team 工具
  → spirit_team.go.AssembleTeam()
    ├── 1. 解析任务描述 + 选定 Agent + 编排模式
    ├── 2. teamUC.Create(ctx, Team{
    │         SpiritSessionID: spiritSessionID,
    │         TaskDescription: taskDesc,
    │         AutoCreated: true,
    │         DefinitionJSON: generatedDefJSON,
    │     })
    ├── 3. sessionUC.Create(ctx, Session{
    │         OwnerType: "team",
    │         TeamID: team.ID,
    │         ParentSessionID: spiritSessionID,
    │         RootSessionID: spiritSessionID,
    │         AgentDepth: 1,
    │     })
    ├── 4. 更新 Spirit Session 的 child_session_ids
    └── 5. 发射 spirit_team_assembled Envelope
```

---

## 四、WS 事件协议

### 4.1 新增 EnvelopeType

```go
const (
    EnvelopeTypeSpiritTeamAssembled EnvelopeType = "spirit_team_assembled"
    EnvelopeTypeSpiritTeamCompleted EnvelopeType = "spirit_team_completed"
    EnvelopeTypeSpiritTeamFailed    EnvelopeType = "spirit_team_failed"
)
```

### 4.2 事件载荷

**spirit_team_assembled**：

```go
env.Metadata = map[string]any{
    "team_id":     teamID,
    "team_name":   teamName,
    "session_id":  teamSessionID,
    "members":     memberNames,
    "mode":        mode,
    "task_summary": taskSummary,
}
```

**spirit_team_completed**：

```go
env.Metadata = map[string]any{
    "team_id":        teamID,
    "session_id":     teamSessionID,
    "result_summary": resultSummary,
    "duration_ms":    durationMS,
}
```

**spirit_team_failed**：

```go
env.Metadata = map[string]any{
    "team_id":       teamID,
    "session_id":    teamSessionID,
    "error_message": errMsg,
    "failed_step":   failedStep,
}
```

### 4.3 复用现有事件

| EnvelopeType | 来源 | 精灵模式用途 |
|-------------|------|-------------|
| `team_step_started` | Team Runner | 成员开始执行 → 更新成员状态 |
| `team_step_finished` | Team Runner | 成员执行完成 → 更新成员状态 |
| `member_message_start/delta/done` | Team Runner | 成员消息流 → 任务执行面板 |
| `session.status_changed` | SessionStatusPublisher | 团队 Session 状态 → 团队卡片 Badge |
| `orchestration_agent_status` | StatusProjector | Agent 节点实时状态 → 成员树/时间线 |
| `team_run_started/finished/failed` | Team Runner | 团队 Run 生命周期 → 团队卡片状态 |

---

## 五、Session 与进化体系关联

### 5.1 Session 记录的信息

| 信息类别 | 字段/存储位置 | 进化用途 |
|---------|-------------|---------|
| 任务上下文 | `Session.MetadataJSON` | 编排进化：任务类型→模式推荐 |
| 团队组成 | `Team.DefinitionSnapshotJSON` | 编排进化：成员组合效率分析 |
| 执行轨迹 | `TeamRun` + `TeamRunStep` | 技能进化：工具调用模式检测 |
| 编排活动 | `OrchestrationStep` | 编排进化：DQ Score 计算 |
| 参与者画像 | `SessionParticipant` | Agent 进化：能力画像更新 |
| 工具调用明细 | `ChatMessage.OptionsJSON` | 技能进化：Skill 健康度分析 |
| 记忆提取锚点 | `Memory L1-L4` | 记忆：事实/实体/关系提取 |
| 进化指标 | `AgentRuntimeSettings.evolution_*` | Agent 进化：工具成功率/检索质量 |
| 父子关联 | `ParentSessionID` / `RootSessionID` | 全链路：从 Team 回溯到 Spirit |

### 5.2 进化数据流

```
Session 执行轨迹
    ├──→ 技能进化：工具调用模式 → Skill 提案 → 技能管家分析
    ├──→ Agent 进化：多 Team 表现 → 能力画像 → tool_weight_json 调整
    ├──→ 记忆：L0-L4 全链路提取 → 记忆管家 dream_cycle 输入
    ├──→ 知识图谱：协作关系 + 产出物 → L4 实体-关系 → GraphRAG
    └──→ 编排进化：模式成功率 + DQ Score → 编排策略优化
```

---

## 六、前端架构

### 6.1 新增目录结构

```
web/src/features/spirit/
  types.ts                    ← SpiritTeam / SpiritMember / PanelMode 类型
  api.ts                      ← listSpiritTeams / getSpiritTeamDetail
  useSpiritWorkspace.ts       ← 精灵工作区编排 composable

web/src/stores/spirit/
  index.ts                    ← useSpiritTeamStore

web/src/components/spirit/
  SpiritEntry.vue             ← 精灵入口卡片
  TeamTaskCard.vue            ← 团队任务卡片（含展开/折叠）
  TeamMemberTreeNode.vue      ← 成员树节点
  TaskExecutionPanel.vue      ← 任务执行面板（概览 + 时间线 + 对话流）
  MemberReadOnlyPanel.vue     ← 成员只读面板
  TeamAssemblyCard.vue        ← 精灵对话中的团队组建卡片
  TaskCompletionCard.vue      ← 精灵对话中的任务完成汇报卡片
```

### 6.2 Store 设计

**useSpiritTeamStore**：

```typescript
interface SpiritTeamState {
  teams: SpiritTeam[]
  expandedTeamIds: Set<string>
  activePanelMode: 'spirit' | 'team' | 'member'
  activeTeamId: string | null
  activeMemberId: string | null
  loading: boolean
}

interface SpiritTeam {
  id: string
  teamName: string
  taskSummary: string
  status: SessionStatus
  mode: string
  memberAvatars: string[]
  completedSteps: number
  totalSteps: number
  spiritSessionId: string
  teamSessionId: string
  members: SpiritMember[]
  sharedAgentIds: string[]
}

interface SpiritMember {
  agentId: string
  agentKey: string
  displayName: string
  role: string
  status: 'idle' | 'working' | 'waiting' | 'completed' | 'failed'
  avatarUrl: string
}
```

核心 actions：

- `loadSpiritTeams(spiritSessionId)` — 加载精灵下的团队列表
- `selectTeam(teamId)` — 切换到团队执行面板
- `selectMember(agentId)` — 切换到成员只读面板
- `returnToSpirit()` — 返回精灵对话
- `toggleTeamExpand(teamId)` — 展开/折叠团队成员树
- `archiveTeam(teamId)` — 归档已完成团队

### 6.3 ChatEntitySidebar 重构

现有 `ChatEntitySidebar.vue` 接收 `agents` / `teams` props，按行业/部门分组展示。

重构为精灵模式：

```vue
<template>
  <div class="spirit-sidebar">
    <SpiritEntry
      :active="panelMode === 'spirit'"
      @click="returnToSpirit"
    />
    <ChatSectionHeader title="进行中的团队" :count="activeTeams.length" />
    <TeamTaskCard
      v-for="team in activeTeams"
      :key="team.id"
      :team="team"
      :expanded="expandedTeamIds.has(team.id)"
      :active="activeTeamId === team.id"
      @click="selectTeam(team.id)"
      @toggle-expand="toggleTeamExpand(team.id)"
    />
    <ChatSectionHeader
      v-if="completedTeams.length"
      title="已完成的团队"
      :count="completedTeams.length"
      collapsible
    />
    <!-- 已完成团队折叠区 -->
  </div>
</template>
```

### 6.4 ChatMessagePanel 三模式

```typescript
type PanelMode = 'spirit' | 'team' | 'member'
```

| 模式 | 组件 | 输入框 | WS 连接 |
|------|------|--------|---------|
| `spirit` | 标准 ChatMessagePanel | 显示 | Spirit Session WS |
| `team` | TaskExecutionPanel | 隐藏 | Team Session WS（订阅） |
| `member` | MemberReadOnlyPanel | 隐藏 | 复用 Team Session WS（过滤） |

### 6.5 面包屑导航

```
精灵 > 后端 API 开发团队 > Golang 工程师
```

实现：`useSpiritWorkspace` composable 维护 `breadcrumbItems: ComputedRef<BreadcrumbItem[]>`，由 `TaskExecutionPanel` / `MemberReadOnlyPanel` 顶部渲染。

---

## 七、API 扩展

### 7.1 Session Proto

```protobuf
message Session {
  // 已有字段...
  string parent_session_id = 20;
  string root_session_id = 21;
  int32 agent_depth = 22;
}

message ListChildSessionsRequest {
  string parent_session_id = 1;
}

message ListChildSessionsResponse {
  repeated Session sessions = 1;
}
```

### 7.2 Team Proto

```protobuf
message Team {
  // 已有字段...
  string spirit_session_id = 20;
  string task_description = 21;
  bool auto_created = 22;
}
```

### 7.3 精灵团队查询 API

```protobuf
message ListSpiritTeamsRequest {
  string spirit_session_id = 1;
  repeated string status_filter = 2; // running, completed, failed, waiting_human
}

message SpiritTeamView {
  string team_id = 1;
  string team_name = 2;
  string task_summary = 3;
  string status = 4;
  string mode = 5;
  int32 completed_steps = 6;
  int32 total_steps = 7;
  string team_session_id = 8;
  repeated SpiritMemberView members = 9;
  repeated string shared_agent_ids = 10;
}

message SpiritMemberView {
  string agent_id = 1;
  string agent_key = 2;
  string display_name = 3;
  string role = 4;
  string status = 5;
}
```

---

## 八、测试策略

| 层 | 文件 | 覆盖 |
|----|------|------|
| Biz | `session_tree_test.go` | Session 树查询、深度限制 |
| Biz | `team_spirit_test.go` | AutoCreated Team 创建、SpiritSessionID 关联 |
| Service | `spirit_team_test.go` | AssembleTeam 流程、Envelope 发射 |
| Service | `chat_spirit_test.go` | `__spirit__` 路由、buildSpiritTeam |
| 前端 | `useSpiritTeamStore.spec.ts` | 团队列表加载、面板切换、展开状态 |
| 前端 | `TaskExecutionPanel.spec.ts` | 三区布局、WS 实时更新 |
| 前端 | `MemberReadOnlyPanel.spec.ts` | 只读模式、输入框隐藏 |

E2E：SP-E2E-01（精灵对话 → 组建团队 → 查看执行面板 → 查看成员 → 返回精灵）。

---

## 九、与关联模块

| 模块 | 关系 |
|------|------|
| 1 Chat | 精灵对话面板、团队组建卡片、任务执行面板 |
| 11 Team | 精灵自动创建 Team、TeamRun 状态追踪 |
| 53 Orchestration | Agent 节点状态投影、执行时间线 |
| 10 Session | Session 树状关联 |
| superpowers Builtin Agents | 精灵/编排管家定义、8 个专属工具 |
| superpowers Memory/Butler | Session 数据 → 记忆管家/技能管家分析输入 |
| 1 Chat Execution Trace | ChatExecutionCard 复用 |

---

## 十、P0 实施优化记录

> 2026-06-01：P0 全量实施完成后的代码质量优化记录。

### 10.1 后端优化

| 优化项 | 原问题 | 修复方案 |
|--------|--------|----------|
| `biz.SpiritAgentKey` 常量统一 | `spiritAgentKey` 在 `spirit_team.go` 和 `seed_system_admin.go` 各定义一次 | 抽取到 `internal/biz/agent_types.go`，两处统一引用 `biz.SpiritAgentKey` |
| `CompressorDeps` 聚合接口 | `session.Compressor` 接收 7 个独立接口参数，Wire 绑定困难 | 定义 `CompressorDeps` 聚合接口嵌入 7 个子接口，`NewCompressor` 简化为接收 `deps CompressorDeps` |
| `GetRootSession` 循环保护 | 无限循环风险（数据循环引用时） | 增加最大遍历深度限制 `maxDepth = 10` |
| `truncateTaskDesc` rune 截断 | 按字节截断可能截断中文等多字节字符 | 改用 `utf8.RuneCountInString` + `[]rune` 截断 |
| `seed_system_admin.go` kerrors | 3 处使用 `fmt.Errorf` 违反红线 #10 | 改为 `kerrors.InternalServer("SEED", ...)` |
| Proto 缺失字段 | `micro_compact_enabled`/`memory_compact_enabled`/`tool_result_gate_enabled` 未在 proto 定义 | 添加到 `agent.proto` 字段号 121-123 |
| `plugin.NewUsecase` 缺 lg | Wire 注入缺少 `loggateway.Logger` 参数 | 添加 `lg loggateway.Logger` 参数 |
| `ReadinessProbe` Wire 绑定 | `server.ReadinessProbe` 接口未绑定实现 | 添加 `wire.Bind(new(server.ReadinessProbe), new(*data.Data))` |

### 10.2 前端优化

| 优化项 | 原问题 | 修复方案 |
|--------|--------|----------|
| TaskExecutionPanel XSS | `v-html` 渲染未经 sanitize 的内容，`renderMarkdown` 是空壳 | 接入项目已有的 `renderChatMarkdown`（markdown-it + DOMPurify） |
| `archiveTeam` 错误 API | Store 中 `archiveTeam` 调用 `getSpiritTeamDetail` 而非归档 API | P0 阶段改为本地移除（后端归档 API 在 P1 实现） |
| `api.ts` 内联 import | `mapSpiritMember` 使用内联 `import("./types")` | 改为顶部 `import type { SpiritMember } from "./types"` |
