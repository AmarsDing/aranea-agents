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
