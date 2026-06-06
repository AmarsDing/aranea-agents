# M59: Chat 管家模式 — 开发计划

> **版本**：2026-06-01 | **状态**：✅ P0 已完成
> **需求**：[59-chat-spirit-mode.md](./59-chat-spirit-mode.md) · **设计**：[59-chat-spirit-mode.design.md](./59-chat-spirit-mode.design.md)

---

## 1. 模块定位

Chat 管家模式：精灵为唯一对话入口，左侧列表重构为精灵 + 任务团队树，中间面板支持精灵对话/任务执行/成员只读三种模式。

**代码锚点**：

| 层级 | 路径 | 阶段 |
|------|------|------|
| Service 精灵路由 | `internal/service/chat.go` | P0 |
| Service 团队组装 | `internal/service/spirit_team.go` | P0 |
| Biz Session 树 | `internal/biz/session/usecase.go` | P0 |
| Biz Team 扩展 | `internal/biz/team_usecase.go` | P0 |
| Event | `internal/event/envelope.go` | P0 |
| 前端 Store | `web/src/stores/spirit/index.ts` | P0 |
| 前端组件 | `web/src/components/spirit/` | P0-P1 |
| Proto | `api/kratos/session/v1/session.proto` | P0 |

---

## 2. 前置依赖

| 依赖 | 状态 | 说明 |
|------|------|------|
| 精灵 Agent 种子数据 | ✅ | `__spirit__` Agent 行 + Ownership=system_builtin |
| `assemble_team` 工具 | ✅ | SpiritTeamAssembler 实现团队组装 |
| Session 树字段 | ✅ | ParentSessionID / RootSessionID / AgentDepth |
| ChatEntitySidebar 重构 | ✅ | 从 Agent/Team 平铺 → 精灵 + 团队树 |
| Team AutoCreated 字段 | ✅ | 区分精灵创建 vs 用户手动创建 |

---

## 3. 开发阶段

### Phase P0 — 核心骨架（约 2 周）

> **目标**：精灵为唯一入口，左侧列表重构，团队列表展示，任务执行面板基础版，Session 树关联。

| ID | 任务 | 影响域 | 验收 |
|----|------|--------|------|
| SP-BE-01 | Session Proto 扩展：parent_session_id / root_session_id / agent_depth | `api/kratos/session/v1` | `make api && make build` 通过 |
| SP-BE-02 | Session Biz：ListByParentSessionID / GetRootSession 查询 | `internal/biz/session` | 单测通过 |
| SP-BE-03 | Session Data：parent_session_id 索引 + 查询实现 | `internal/data` | 单测通过 |
| SP-BE-04 | Team Proto 扩展：spirit_session_id / task_description / auto_created | `api/kratos/team/v1` | `make api && make build` 通过 |
| SP-BE-05 | Team Biz：Create 支持 AutoCreated / SpiritSessionID | `internal/biz/team` | 单测通过 |
| SP-BE-06 | spirit_team.go：AssembleTeam 流程（创建 Team + 创建 Session + 发射 Envelope） | `internal/service` | 集成测试通过 |
| SP-BE-07 | chat.go：识别 `__spirit__` → buildSpiritTeam 路由 | `internal/service` | 精灵对话走 Team 路径 |
| SP-BE-08 | Event：spirit_team_assembled / completed / failed EnvelopeType | `internal/event` | 单测通过 |
| SP-BE-09 | 精灵 Agent 种子数据（`__spirit__` + Ownership=system_builtin） | `internal/data/seed` | 启动后精灵 Agent 可查 |
| SP-FE-01 | `features/spirit/types.ts` + `api.ts` | `web/src/features/spirit` | 类型与 Proto 对齐 |
| SP-FE-02 | `useSpiritTeamStore`：团队列表 + 面板模式 + 展开/折叠 | `web/src/stores/spirit` | Store 单测通过 |
| SP-FE-03 | `SpiritEntry.vue`：精灵入口卡片 | `web/src/components/spirit` | 点击切换精灵对话 |
| SP-FE-04 | `ChatEntitySidebar.vue` 重构：精灵 + 团队树 | `web/src/components/chat` | SP-01 验收 |
| SP-FE-05 | `TeamTaskCard.vue`：团队卡片（名称/状态/成员/进度） | `web/src/components/spirit` | SP-03 验收 |
| SP-FE-06 | `TaskExecutionPanel.vue` 基础版：概览 + 时间线 | `web/src/components/spirit` | SP-04 验收 |
| SP-FE-07 | `ChatMessagePanel.vue` 三模式切换 | `web/src/components/chat` | 精灵/团队/成员面板切换 |
| SP-FE-08 | `TeamAssemblyCard.vue`：精灵对话中的团队组建卡片 | `web/src/components/spirit` | SP-02 验收 |

**不涉及**：成员树形展开、成员只读面板、Agent 复用标识、归档/重试。

---

### Phase P1 — 交互增强（约 2 周）

> **目标**：成员树/只读面板、多任务并行、Agent 复用标识、团队生命周期管理、面包屑导航。

| ID | 任务 | 影响域 | 验收 |
|----|------|--------|------|
| SP-FE-09 | `TeamMemberTreeNode.vue`：成员树节点（名称/角色/状态） | `web/src/components/spirit` | SP-05 验收 |
| SP-FE-10 | `TeamTaskCard.vue` 展开成员树 | `web/src/components/spirit` | 展开/折叠 + 成员状态 |
| SP-FE-11 | `MemberReadOnlyPanel.vue`：只读面板（无输入框） | `web/src/components/spirit` | SP-06 验收 |
| SP-FE-12 | 成员消息过滤：按 `OptionsJSON.team_member` 过滤 | `web/src/stores/spirit` | 成员只看自己的消息 |
| SP-FE-13 | 多任务并行：精灵连续下达多任务，左侧多团队卡片 | `web/src/stores/spirit` | SP-07 验收 |
| SP-FE-14 | Agent 复用标识：团队卡片标注"共用 Agent" | `web/src/components/spirit` | 复用 Agent 可见标识 |
| SP-FE-15 | `TaskCompletionCard.vue`：精灵对话中的任务完成汇报 | `web/src/components/spirit` | 团队完成后精灵通知 |
| SP-FE-16 | 团队归档：手动归档 + 自动归档（7 天） | `web/src/stores/spirit` | SP-08 验收 |
| SP-FE-17 | 失败团队：重试/放弃按钮 | `web/src/components/spirit` | SP-08 验收 |
| SP-FE-18 | 面包屑导航：精灵 > 团队 > 成员 | `web/src/features/spirit` | SP-09 验收 |
| SP-FE-19 | 返回精灵按钮 + WS 连接保持 | `web/src/components/spirit` | 切换不丢 WS |
| SP-BE-10 | ListSpiritTeams RPC：按 spirit_session_id 查团队列表 | `api/kratos/session/v1` | 前端团队列表数据源 |
| SP-BE-11 | ArchiveTeam RPC：归档已完成团队 | `api/kratos/team/v1` | 归档后列表不显示 |

---

### Phase P2 — 进化闭环（约 3 周）

> **目标**：Session 数据 → 技能/记忆/编排分析，Agent 能力画像 → 团队组建优化，知识图谱数据积累。

| ID | 任务 | 影响域 | 验收 |
|----|------|--------|------|
| SP-EVO-01 | Session 执行轨迹 → 技能管家分析输入 | `internal/biz` | 技能管家可消费 Team Session 数据 |
| SP-EVO-02 | Session 执行轨迹 → 记忆管家分析输入 | `internal/biz` | 记忆管家 dream_cycle 可消费 |
| SP-EVO-03 | 编排效率分析：模式成功率 + DQ Score | `internal/biz` | 精灵可参考历史编排效率选择模式 |
| SP-EVO-04 | Agent 能力画像 → 团队组建优化 | `internal/agent` | assemble_team 参考 Agent 历史表现 |
| SP-EVO-05 | 知识图谱数据积累：协作关系 + 产出物 | `internal/memory` | L4 实体-关系图谱丰富 |
| SP-EVO-06 | 编排进化：失败模式分析 → FailurePolicy 调整 | `internal/team` | 基于历史失败调整默认 FailurePolicy |

---

## 4. 任务板（P0 当前冲刺）

| 排序 | ID | 任务 | 状态 |
|------|-----|------|------|
| 1 | SP-BE-01 | Session Proto 扩展 | ✅ |
| 2 | SP-BE-02 | Session Biz 树查询 | ✅ |
| 3 | SP-BE-03 | Session Data 查询实现 | ✅ |
| 4 | SP-BE-04 | Team Proto 扩展 | ✅ |
| 5 | SP-BE-05 | Team Biz AutoCreated | ✅ |
| 6 | SP-BE-06 | spirit_team.go AssembleTeam | ✅ |
| 7 | SP-BE-07 | chat.go 精灵路由 | ✅ |
| 8 | SP-BE-08 | Event 新增 EnvelopeType | ✅ |
| 9 | SP-BE-09 | 精灵 Agent 种子数据 | ✅ |
| 10 | SP-FE-01 | 前端 types + api | ✅ |
| 11 | SP-FE-02 | useSpiritTeamStore | ✅ |
| 12 | SP-FE-03 | SpiritEntry.vue | ✅ |
| 13 | SP-FE-04 | ChatEntitySidebar 重构 | ✅ |
| 14 | SP-FE-05 | TeamTaskCard.vue | ✅ |
| 15 | SP-FE-06 | TaskExecutionPanel.vue | ✅ |
| 16 | SP-FE-07 | ChatMessagePanel 三模式 | ✅ |
| 17 | SP-FE-08 | TeamAssemblyCard.vue | ✅ |

---

## 5. 验收标准

### Phase P0

- [x] `make api && make wire && make build` 通过
- [x] `go test ./internal/biz/session/... ./internal/service/... -count=1` 通过
- [x] 精灵 Agent 种子数据启动后可查
- [x] 左侧列表仅显示精灵 + 团队（SP-01）
- [x] 精灵区分简单/任务型对话（SP-02）
- [x] 团队卡片展示名称/状态/成员/进度（SP-03）
- [x] 任务执行面板三区布局（SP-04）
- [x] `cd web && pnpm lint && pnpm test && pnpm build` 通过

### Phase P1

- [ ] 成员树形展开 + 状态（SP-05）
- [ ] 成员只读面板无输入框（SP-06）
- [ ] 多任务并行 + Agent 复用隔离（SP-07）
- [ ] 团队归档/重试/放弃（SP-08）
- [ ] 面包屑导航 + 返回精灵（SP-09）

### Phase P2

- [ ] Session 数据可被技能管家消费（SP-10）
- [ ] 编排效率分析可输出 DQ Score
- [ ] Agent 能力画像可被 assemble_team 参考

---

## 6. 依赖与风险

| 风险 | 缓解 |
|------|------|
| 精灵 Agent 种子数据与现有 Agent 冲突 | `agent_key=__spirit__` + `Ownership=system_builtin` 唯一约束 |
| ChatEntitySidebar 重构影响现有 Chat 功能 | feature flag 控制精灵/专家模式切换 |
| Session 树查询性能 | `parent_session_id` 加索引；深度限制 2 层 |
| Agent 复用时 L3/L4 共享导致记忆污染 | L3/L4 只读共享，写入仍按 Session 隔离 |
| WS 事件量增大（每个团队独立 WS） | 复用 Team Session WS，前端按 team_id 过滤 |
| 团队自动归档误删 | 归档仅移除列表显示，Session/TeamRun 保留 |

---

## 7. 关联文档更新

| 文档 | 更新内容 | 时机 |
|------|---------|------|
| [1-chat.md](./1-chat.md) | 新增精灵模式章节 | P0 |
| [10-session.md](./10-session.md) | Session 树状模型 | P0 |
| [11-multi-agent.md](./11-multi-agent.md) | 精灵自动创建 Team | P0 |
| [architecture-blueprint.md](../architecture-blueprint.md) | 精灵模块卡片 | P0 |
| [module-cross-reference.md](../module-cross-reference.md) | M59 模块卡片 | P0 |
| [7-agent-evolution.md](./7-agent-evolution.md) | 进化指标数据源扩展 | P2 |
| [memory/L4.md](./memory/L4.md) | 协作关系 → 实体-关系 | P2 |


---

## 子模块：Chat Spirit Mode Review

> **审查日期**：2026-06-01
> **审查范围**：需求文档 · 设计文档 · 架构蓝图 · 模块交叉参考 · 后端/前端实际代码
> **审查目标**：从项目整体架构和设计出发，从业务角度审查业务逻辑是否合理

---

## 一、总体评价

M59 的业务定位清晰——"精灵为唯一入口，用户只需描述需求"——这一设计理念与当前"Agent/Team 平铺"的认知负担问题高度契合。P0 阶段的核心骨架（精灵路由、Team 组装、Session 树、前端三模式面板）已基本落地。但从**业务逻辑完整性**和**跨模块一致性**角度，存在若干需要关注的问题，按严重度分为三级。

---

## 二、🔴 严重问题（业务逻辑缺陷，可能导致功能异常）

### S1. 每次精灵 Turn 都创建新 Team，无复用机制

**位置**：`internal/service/spirit_team.go` `buildSpiritTeam`

**问题**：`executeSpiritTeamTurn` 每次被调用都会通过 `buildSpiritTeam` → `AssembleTeam` 创建一个全新的 Team + Team Session。这意味着：

- 用户在精灵对话中发送**每条消息**都会产生一个独立 Team，而非在同一个 Team 中继续工作
- `TeamKey = "spirit_" + spiritSessionID` 会导致第二次创建时**唯一约束冲突**
- 需求 US-02 明确说"精灵调用 `assemble_team` 工具组建团队"，但当前实现是**无条件组建**，而非由精灵 LLM 自主判断何时需要组建

**业务影响**：这与需求设计的核心逻辑冲突——需求是"简单对话精灵直接回复，复杂任务精灵自动组建团队"，但实现是"每条消息都组建团队"。

**建议**：
1. 精灵 Turn 应先走普通 LLM 对话流程，由精灵 Agent 通过 `assemble_team` 工具调用触发 Team 组装
2. 只有当精灵主动调用工具时才走 `executeSpiritTeamTurn`，而非在路由层无条件拦截所有 `__spirit__` 请求

---

### S2. `panelMode` 为 `undefined` 时 Composer 不渲染

**位置**：`web/src/components/chat/ChatMessagePanel.vue`

**问题**：`panelMode` 是 optional prop，当 `undefined` 时：
- `v-else` 分支（spirit 模式）被选中，渲染消息列表
- 但 `ChatComposer` 内部用 `v-if="panelMode === 'spirit'"` 判断，`undefined !== 'spirit'`，导致**输入框不显示**

**业务影响**：非精灵模式下（未传 `panelMode` prop）用户无法输入消息，功能完全不可用。

**建议**：将 `ChatComposer` 的条件改为 `v-if="!panelMode || panelMode === 'spirit'"`，或给 `panelMode` 设置默认值 `'spirit'`。

---

### S3. `EnvelopeTypeSpiritTeamCompleted/Failed` 已定义未实现

**位置**：`internal/event/contract/envelope.go`

**问题**：两种关键事件类型已在 Proto/常量中定义，但后端没有任何代码发布它们。当前只发布了 `SpiritTeamAssembled`。

**业务影响**：
- 前端无法通过 WS 感知精灵 Team 执行完成/失败
- US-08 团队生命周期管理（`running → completed → failed`）无法闭环
- US-03 团队卡片的状态无法实时更新
- 精灵无法在对话中主动通知用户任务结果（US-08 "精灵可主动汇报"）

**建议**：在 Team Run 完成回调（`team_turn_hooks.go`）中，检测 `AutoCreated=true` 的 Team，发射 `SpiritTeamCompleted/Failed` 事件。

---

## 三、🟡 中等问题（设计缺陷或技术债，影响可维护性/扩展性）

### M1. `appendChildSessionID` 无并发保护

**位置**：`internal/biz/spirit_team_usecase.go` `appendChildSessionID`

**问题**：读-改-写 MetadataJSON 中的 `child_session_ids` 数组，没有乐观锁/CAS 保护。虽然当前同一 Session 同一时刻只有一个活跃 Run，但这是一个脆弱的假设——多任务并行（US-07）时可能触发竞态。

**建议**：使用 `compress_version` 的 CAS 机制（项目已有 `TryIncrementCompressVersion`），或改用 `parent_session_id` 索引反向查询替代冗余数组。

---

### M2. `MetadataJSON.child_session_ids` 与 `ParentSessionID` 冗余

**位置**：`internal/biz/spirit_team_usecase.go`

**问题**：子 Session 已通过 `ParentSessionID` 关联到精灵 Session（且有索引 `idx_sessions_parent`），`MetadataJSON.child_session_ids` 是冗余的反向索引。两套关联机制可能导致不一致——如果 `appendChildSessionID` 失败但 Session 创建成功，就会出现"有子 Session 但父 Session 不知道"的情况。

**建议**：移除 `child_session_ids`，统一使用 `ListByParentSessionID` 查询。如果需要缓存，在前端 Store 层维护。

---

### M3. `spiritAssembler == nil` 时精灵路由静默降级

**位置**：`internal/service/chat_orchestrator_turn.go`

**问题**：路由条件 `ag.AgentKey == biz.SpiritAgentKey && o.spiritAssembler != nil`，当 `spiritAssembler` 未注入时，`__spirit__` Agent 的请求会**静默降级为普通 Agent 对话**，不会报错。精灵功能"消失"而无明显告警。

**建议**：当 `AgentKey == SpiritAgentKey && spiritAssembler == nil` 时，返回 `kerrors.InternalServer` 错误，明确告知配置异常。

---

### M4. `SpiritTeam.status` / `mode` 类型过于宽泛

**位置**：`web/src/features/spirit/types.ts`

**问题**：`status` 和 `mode` 均为裸 `string`，缺乏联合类型约束。导致：
- `TeamTaskCard.vue` 和 `TaskExecutionPanel.vue` 中 `team.status as any` 强制断言
- `TeamAssemblyCard.vue` 中 `status` 定义为联合类型 `"assembling" | "assembled" | "completed" | "failed"`，与 `SpiritTeam.status` 的 `string` 不一致
- 拼写错误无法在编译期捕获

**建议**：定义 `SpiritTeamStatus` 和 `SpiritTeamMode` 联合类型，与后端 Proto 对齐。

---

### M5. 类型导入违反红线 #13

**位置**：`ChatEntitySidebar.vue`、`ChatMessagePanel.vue` 等多处

**问题**：展示组件从 `features/spirit/api.ts` 导入类型，而非 `types.ts`。项目红线 #13 明确要求"展示组件从 `features/<域>/api.ts` 引类型"但同时"共享类型放在 `features/<域>/types.ts`，组件只 import types"。

**建议**：统一从 `types.ts` 导入类型。

---

### M6. API 映射层双键名兼容是技术债

**位置**：`web/src/features/spirit/api.ts`

**问题**：`raw.teamName ?? raw.team_name` 模式大量出现，说明后端 API 契约不统一。`listSpiritTeams` 的响应结构也不确定（`data?.items ?? data?.teams`）。

**建议**：与后端对齐为一种命名风格（Proto JSON 默认 camelCase），移除双份映射。

---

### M7. `ListSpiritTeams` RPC 归属包不一致

**位置**：开发计划 SP-BE-10 指定 `api/kratos/session/v1`，但语义上是按 `spirit_session_id` 查 Team 列表

**问题**：放在 `session/v1` 意味着 SessionService 需要依赖 TeamUsecase，违反了"Session 不应知道 Team"的领域边界。放在 `team/v1` 的 TeamService 下更符合职责划分。

**建议**：将 `ListSpiritTeams` 归属到 `team/v1`，或新增独立的 `spirit/v1` 服务。

---

### M8. Team Ent Schema 缺少 `spirit_session_id` 索引

**位置**：`internal/data/ent/schema/team.go`

**问题**：`spirit_session_id` 是 `ListSpiritTeams` 的核心查询条件，但未创建索引，将导致全表扫描。

**建议**：添加 `index.Fields("spirit_session_id", "deleted_at").StorageKey("idx_teams_spirit_session")`。

---

## 四、🟢 轻微问题（可优化项，不影响核心功能）

| 编号 | 问题 | 位置 | 建议 |
|------|------|------|------|
| L1 | `modeLabel` 映射重复 | `TeamTaskCard.vue` + `TeamAssemblyCard.vue` | 抽取为共享 composable |
| L2 | Store computed 与组件 computed 重复 | `stores/spirit/index.ts` + `ChatEntitySidebar.vue` | 直接使用 Store 的 computed |
| L3 | `memberAvatars` 与 `members[].avatarUrl` 数据冗余 | `features/spirit/types.ts` | 统一为从 `members` 派生 |
| L4 | `GetRootSession` N+1 查询 | `internal/biz/session/usecase.go` | 深层树时考虑递归 CTE |
| L5 | `AutoCreated` Team 无清理机制 | `internal/biz/team_types.go` | P1 归档功能需覆盖 |
| L6 | `ArchiveTeam` 缺少 Proto 定义和 `archived_at` 字段 | `api/kratos/team/v1/team.proto` | 明确归档与删除语义区分 |
| L7 | `CreateSessionRequest` 缺少精灵模式字段 | `api/kratos/session/v1/session.proto` | 暴露 `parent_session_id` 等字段 |
| L8 | 硬编码中文文案未走 i18n | `ChatEntitySidebar.vue` | 统一走 `t()` 函数 |
| L9 | `formatTime` 未指定 locale | `TaskExecutionPanel.vue` | 指定 locale 或使用 dayjs |

---

## 五、架构合规性审查

| 检查项 | 结果 | 说明 |
|--------|------|------|
| `internal/biz` 不 import `pkg/trpc-agent-go` | ✅ 通过 | 精灵逻辑在 `spirit_team_usecase.go` 中纯 biz 实现 |
| `internal/biz` 不 import `api/*/v1` | ✅ 通过 | Proto 映射在 Service 层 |
| Runner 装配只在 `internal/service` | ✅ 通过 | `executeSpiritTeamTurn` 在 Service 层 |
| Service 层不写业务逻辑 | ⚠️ 边界模糊 | `buildSpiritTeam` 中 Mode 硬编码为 `"coordinator"` 属于业务决策，应在 biz 层 |
| 跨模块调用通过窄接口 | ✅ 通过 | `SpiritTeamAssembler` 通过 `SpiritTeamUsecase` 端口交互 |
| 前端展示组件不 import Store | ✅ 通过 | Spirit 组件均为 props/emits |
| 前端展示组件不直接调 API | ✅ 通过 | API 调用在 Store action 中 |
| 消息分组使用堆栈模型 | ⚠️ 需验证 | `TaskExecutionPanel` 中消息过滤逻辑需确认是否遵循 `groupMessagesByTurn` |

---

## 六、业务逻辑完整性矩阵

| 需求用户故事 | 后端实现 | 前端实现 | 闭环状态 |
|-------------|---------|---------|---------|
| US-01 精灵为唯一入口 | ✅ `__spirit__` 路由 | ✅ `SpiritEntry` + 侧边栏重构 | ✅ 闭环 |
| US-02 简单/任务型区分 | ❌ 每次都创建 Team | ✅ `TeamAssemblyCard` | ❌ **核心逻辑缺失** |
| US-03 团队列表展示 | ✅ `ListSpiritTeams`（P1） | ✅ `TeamTaskCard` | ⚠️ 后端 P1 才实现 |
| US-04 任务执行面板 | ✅ Team Run 事件流 | ✅ `TaskExecutionPanel` | ⚠️ Completed/Failed 事件缺失 |
| US-05 成员树形展开 | P1 | P1 | — |
| US-06 成员只读面板 | P1 | P1（空壳占位） | — |
| US-07 多任务并行 | ⚠️ TeamKey 冲突风险 | ✅ Store 支持多 Team | ❌ 并发安全问题 |
| US-08 团队生命周期 | ❌ Completed/Failed 事件未实现 | ⚠️ `archiveTeam` 仅本地 | ❌ **未闭环** |
| US-09 返回精灵对话 | ✅ `returnToSpirit` | ✅ 返回按钮 + 面包屑 | ✅ 闭环 |

---

## 七、优先修复建议

| 优先级 | 问题 | 修复方向 |
|--------|------|---------|
| **P0** | S1: 每次 Turn 都创建 Team | 改为精灵 LLM 自主决策是否调用 `assemble_team`，路由层不拦截 |
| **P0** | S2: Composer 不渲染 | `panelMode` 默认值或条件修正 |
| **P0** | S3: Completed/Failed 事件未实现 | 在 Team Run 回调中发射事件 |
| **P1** | M1: 并发保护 | CAS 或移除冗余数组 |
| **P1** | M4: 类型约束 | 定义联合类型 |
| **P1** | M8: 缺索引 | 添加 `spirit_session_id` 索引 |
| **P2** | 其余中等问题 | 逐步清理技术债 |

---

## 八、核心结论

M59 的架构分层和模块划分是合理的，但**最关键的业务逻辑——精灵何时组建团队——当前实现与需求设计存在根本性偏差**（S1）。需求设计是"精灵 LLM 自主判断后调用工具"，实现是"路由层无条件拦截"。这会导致简单对话也触发 Team 组装，与 US-02 的"简单对话精灵直接回复"直接矛盾。建议优先修正此逻辑，使精灵路由回归到"工具调用触发"而非"AgentKey 匹配触发"。

---

## 九、修复记录（2026-06-01）

> **修复范围**：S1 / S2 / S3 / M1 / M2 / M3 / M4 / M5 / M6 / M8

### S1 ✅ 已修复：移除路由层拦截，实现 assemble_team 工具

**修复方案**：方案 A（框架原生模式）

| 变更 | 文件 | 说明 |
|------|------|------|
| 移除路由拦截 | `chat_orchestrator_turn.go` | 删除 `AgentKey == "__spirit__"` 条件分支，精灵走 `runSingleAgentViaTRPC` |
| 移除 spiritAssembler 字段 | `chat_orchestrator.go` | 从 `ChatOrchestrator` / `ChatOrchestratorDeps` 中移除 |
| 删除旧方法 | `spirit_team.go` | 删除 `executeSpiritTeamTurn` / `buildSpiritTeam` |
| 新增 assemble_team 工具 | `tools/spirit_tools.go` | `NewAssembleTeamTool` + `NewListButlersTool` + `NewQueryButlerStatusTool` |
| 新增 spirit profile | `agent_effective_tools.go` | `toolProfiles["spirit"]` = assemble_team + list_butlers + query_butler_status + memory_search + datetime |
| 新增 GetActiveTeam | `spirit_team_usecase.go` | 查询精灵会话下是否有活跃 Team，有则复用 |
| 新增 ListBySpiritSessionID | `team_usecase.go` + `team_repo.go` | TeamRepository 接口新增方法 |

**核心逻辑变更**：精灵 Agent 现在作为普通 LLMAgent 运行，LLM 自主决定是否调用 `assemble_team` 工具。简单对话直接回复，复杂任务才组建团队。

### S2 ✅ 已修复：panelMode undefined 时 Composer 渲染

**修复**：`ChatMessagePanel.vue` 中 `v-if="panelMode === 'spirit'"` → `v-if="!panelMode || panelMode === 'spirit'"`

### S3 ✅ 已修复：SpiritTeamCompleted/Failed 事件发射

**修复**：`team_turn_hooks.go` 中新增 `publishSpiritTeamLifecycleEvent` 方法，在 Team Run 完成/失败时检测 `AutoCreated=true` 的精灵 Team，发射对应事件。

### M1 ✅ 已修复：appendChildSessionID 并发保护

**修复**：随 M2 一起移除了 `appendChildSessionID` 方法，消除了并发问题。

### M2 ✅ 已修复：child_session_ids 冗余

**修复**：移除 `appendChildSessionID` 方法及其调用。子 Session 通过 `ParentSessionID` 反查即可。

### M3 ✅ 已修复：spiritAssembler 静默降级

**修复**：随 S1 一起移除了 `spiritAssembler` 字段和路由拦截逻辑，问题不再存在。

### M4 ✅ 已修复：SpiritTeam 类型约束

**修复**：`types.ts` 中新增 `SpiritTeamStatus` 和 `SpiritTeamMode` 联合类型，`SpiritTeam.status` 和 `SpiritTeam.mode` 使用联合类型。

### M5 ✅ 已修复：类型导入红线

**修复**：所有展示组件从 `features/spirit/types.ts` 导入类型，移除 `api.ts` 中的 re-export。

### M6 ✅ 已修复：API 双键名兼容

**修复**：`api.ts` 中统一使用 camelCase 单键，移除 `raw.xxx ?? raw.yyy_snake_case` 回退模式和 `data?.items ?? data?.teams` 回退。

### M8 ✅ 已修复：Team Ent Schema 索引

**修复**：`team.go` 新增 `Indexes()` 方法，定义 `(spirit_session_id, deleted_at)` 复合索引。

### 未修复项

| 编号 | 原因 |
|------|------|
| M7 | `ListSpiritTeams` RPC 归属需要 Proto 变更，属于 P1 范畴 |
| L1-L9 | 轻微问题，不影响核心功能，后续迭代清理 |
