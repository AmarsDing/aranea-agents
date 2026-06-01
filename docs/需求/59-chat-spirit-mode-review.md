# M59 Chat 管家模式 — 业务逻辑审查报告

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
