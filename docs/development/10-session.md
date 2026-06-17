# Session 会话历史

本文档描述 **会话历史（Session）** 的产品目标、用户故事与验收口径：会话列表与治理、历史追踪时间轴、上下文消耗、Team/Agent 编排索引。

- 技术方案见 [`10-session.design.md`](./10-session.design.md)
- 实现差距与任务以 [`10-session.development.md`](./10-session.development.md) 为准

> **框架对齐**：运行时以 trpc-agent-go `session.Service` 为长期目标；当前业务会话主存储为 SQLite + Ent（`sessions` / `messages`），Runner 状态经 `runner_snapshot_json` 持久化。

---

## 0. 会话历史追踪弹窗（用户交互规格）

### 0.1 用户体验

在 Chat 右侧 Session 列表中，每条会话的更多菜单新增 **历史追踪**。点击后弹出 Quasar `QDialog`，内部使用 `QTimeline` 纵向时间轴展示整个会话链路：

| 区域 | 内容 |
|------|------|
| 顶部 | 会话标题、事件数量、消息/工具/Skill/MCP 统计 |
| 中轴 | `QTimeline` 时间轴，按事件发生时间排序 |
| 左侧 | 对话内容：用户消息、Agent 消息、Team 成员消息，默认折叠预览，点击展开完整 Markdown |
| 右侧 | 外部调用：Tool、Skill、MCP 等，显示调用标签、状态、耗时、输入输出摘要和错误 |

外部调用使用不同标签：

| 类型 | 标签 | 颜色建议 |
|------|------|----------|
| Tool | `Tool` | primary / info |
| Skill | `Skill` | teal |
| MCP | `MCP` | teal |
| Message | `User` / `Agent` / `Team` | grey / primary |

> 后端 API 契约与实施策略详见 [10-session.design.md §2 Proto 层](./10-session.design.md#二proto-层) 与 [10-session.development.md §2 现状评估](./10-session.development.md#2-现状评估2026-06-17)

---

## 1. 核心目标

| 目标 | 说明 |
|------|------|
| 追踪历史会话 | 支持按时间、Agent、Team、状态、模型、上下文消耗比例检索历史 session |
| 查看会话属性 | 展示发生时间、最后活跃时间、消息数、调用次数、Token/费用、context window 消耗比例 |
| 完整追踪链路 | 每轮对话都能看到模型、工具、Skill、MCP 的调用名称、耗时、token、状态、输入输出与错误 |
| 区分 Team / 单 Agent | 单 Agent session 只绑定 `agent_id`；Team session 绑定 `team_id`，并记录参与编排的多个 Agent |
| 支撑 Agent 编排 | Session 是编排运行的主索引，所有 run、step、message、tool call、token usage、trace 都通过 `session_id` 串联 |
| 支持问题复盘 | 能回答「哪个 Agent 在什么时候消耗了多少上下文」「Team 编排卡在哪一步」「失败来自模型、工具还是子 Agent」 |
| 支持上下文治理 | 当 context 消耗过高时触发摘要、归档、裁剪、分支或新 session 建议 |

---

## 2. Session 在编排中的作用

Session 是一次持续交互或任务执行的 **上下文容器**，也是编排层的 **运行边界**。

| 角色 | 作用 |
|------|------|
| 上下文容器 | 保存用户输入、assistant 回复、系统摘要、附件、工具结果、子 Agent 输出 |
| 编排主索引 | Team 编排中的 planner、worker、reviewer、tool step 都挂在同一个 `session_id` 下 |
| 状态机 | 记录 `idle`、`running`、`completed`、`interrupted`、`awaiting_confirmation` 等状态 |
| 指标聚合点 | 聚合上下文窗口消耗、Token、费用、模型调用次数、延迟、错误率 |
| 权限与归属边界 | 区分 workspace、user、agent、team，避免 Team 会话与个人 Agent 会话混用 |
| 可回放单元 | 复盘时按 session 加载 run timeline、消息流、模型调用流水和事件轨迹 |

Team session 与单 Agent session 的主要差异：

| 维度 | 单 Agent Session | Team Session |
|------|------------------|--------------|
| 归属 | `owner_type = 'agent'`，`agent_id` 必填 | `owner_type = 'team'`，`team_id` 必填 |
| 参与者 | 一个主 Agent | 多个 Agent，可有 planner / executor / reviewer 等角色 |
| 消息流 | 用户与单 Agent 对话为主 | 用户、Team coordinator、子 Agent、工具事件混合 |
| 上下文窗口 | 主要看当前模型上下文 | 既看 Team 总上下文，也看每个参与 Agent 的上下文 |
| 编排记录 | 可没有 run/step，普通聊天即可 | 必须记录 run、step、agent handoff、tool call |

> 状态机定义详见 [10-session.design.md §3.5 Session 状态机](./10-session.design.md#35-session-状态机)

---

## 3. 功能需求清单

### 3.1 会话生命周期

| 需求 ID | 需求描述 | 验收标准 |
|---------|----------|----------|
| FR-S-01 | 创建会话（单 Agent / Team） | `owner_type=agent` 时 `agent_id` 必填；`owner_type=team` 时 `team_id` 必填；创建后默认状态 `idle` |
| FR-S-02 | 搜索会话列表 | 支持 owner_type/agent_id/team_id/status/context_status/keyword/user_id/sort_by/sort_order 筛选 + 分页 |
| FR-S-03 | 查看会话详情 | 返回 session 属性、参与者、轮次、消息、runs、trace 等关联数据 |
| FR-S-04 | 重命名 / 部分更新会话 | 支持更新 title/tags_json/visibility/metadata_json/dialog_mode/default_provider/default_model |
| FR-S-05 | 归档会话 | `running`/`awaiting_confirmation` 状态不可归档；归档后 `status=archived` |
| FR-S-06 | 恢复归档会话 | 恢复后 `status=idle`，清空 `archived_at`/`deleted_at` |
| FR-S-07 | 删除会话（软删除） | `running`/`awaiting_confirmation` 状态不可删除；删除后对用户不可见，usage 统计保留 |
| FR-S-08 | 置顶会话 | `pinned_at` 字段 + Pin/Unpin 操作；列表置顶分组展示 |
| FR-S-09 | 导出会话 | 支持 Markdown / JSON 格式导出 |
| FR-S-10 | 会话状态转换 | 支持 idle→running→completed/interrupted/awaiting_confirmation 状态机转换 |

### 3.2 历史追踪与时间轴

| 需求 ID | 需求描述 | 验收标准 |
|---------|----------|----------|
| FR-T-01 | 历史追踪弹窗 | Chat 侧栏会话菜单入口；QTimeline 展示消息+工具+Skill+MCP 混合时间轴 |
| FR-T-02 | Timeline 分页 | 支持 limit/offset/kind_filter/sort_order 参数 |
| FR-T-03 | 消息列表分页 | 支持 limit/offset 分页 + after_revision 增量拉取 |
| FR-T-04 | 消息搜索 | 支持 FTS5 全文检索（无 FTS 表时 LIKE 回退） |
| FR-T-05 | 对话轮次列表 | 按 turn_number 升序分页查询 |

### 3.3 上下文治理

| 需求 ID | 需求描述 | 验收标准 |
|---------|----------|----------|
| FR-C-01 | 上下文消耗追踪 | `context_used_ratio = prompt_tokens / context_window_tokens`；状态阈值 0.6/0.8/0.95 |
| FR-C-02 | 上下文状态分级 | normal(<60%)/warning(60-80%)/critical(80-95%)/exceeded(≥95%) |
| FR-C-03 | 异步摘要压缩 | `context_used_ratio` 超阈值时触发 LLM 生成滚动摘要；防抖可配置 |
| FR-C-04 | 压缩后上下文重置 | 压缩完成后 `context_used_ratio` 重置为压缩后估算值 |
| FR-C-05 | 压缩状态查询 | 支持 `GetCompressStatus` 查询当前压缩状态 |
| FR-C-06 | 手动压缩 | 支持 `CompactSession` RPC 手动触发压缩 |

### 3.4 批量治理

| 需求 ID | 需求描述 | 验收标准 |
|---------|----------|----------|
| FR-B-01 | 行内删除 | 操作列删除按钮；`running` 状态禁用；二次确认 |
| FR-B-02 | 批量选择归档/删除 | 勾选模式；批量归档无二次确认，批量删除需确认 |
| FR-B-03 | 按保留天数归档 | 输入保留天数 → 预览命中数 → 确认执行 |
| FR-B-04 | 按保留天数删除 | 同上 + 强调永久删除 + 可选「包含已归档」 |
| FR-B-05 | 批量预览 | dry-run 返回 matched/skipped_running/sample_ids |

#### 批量治理语义约定（用户视角）

| 概念 | 说明 |
|------|------|
| **保留 N 天** | 以 `last_message_at`（空则 `updated_at`，再空则 `created_at`）为基准，**最近 N 天内**的 session **保留** |
| **清理范围** | **早于** cutoff 的 session 进入批量归档或批量删除 |
| **用户侧「永久删除」** | 从 UI 不可恢复；后端仍为软删除，符合设计原则 |
| **安全排除** | 默认不处理 `running`/`awaiting_confirmation` 状态的 session；已 `deleted` 的跳过 |

> 批量 RPC 契约详见 [10-session.design.md §2.3 批量操作 RPC](./10-session.design.md#23-批量操作-rpc)

### 3.5 编排可观测性

| 需求 ID | 需求描述 | 验收标准 |
|---------|----------|----------|
| FR-O-01 | Run 列表 | 按会话查询 run 列表，展示 phase/状态/时间 |
| FR-O-02 | 参与者列表 | Team session 展示参与 Agent、角色、贡献指标 |
| FR-O-03 | 子会话树 | 支持查询父会话的子会话列表（session tree 模型） |
| FR-O-04 | 活动列表 | 支持按会话/轮次查询活动（Activity-First 架构） |

### 3.6 非功能需求

| 需求 ID | 需求描述 |
|---------|----------|
| NFR-S-01 | 列表查询在 10w 会话规模下 P99 < 500ms（含索引） |
| NFR-S-02 | Timeline 全量无 2000 cap，超大会话（10w+ 事件）需流式或 cursor |
| NFR-S-03 | 软删除优先，不破坏成本统计和审计链路 |
| NFR-S-04 | 框架对齐优先：先查 trpc-agent-go 框架 API 再实现，不在 biz 重写运行时 |
| NFR-S-05 | 分层铁律：`internal/biz` 不得 import `pkg/trpc-agent-go` |

---

## 4. 数据模型与架构设计

> 数据模型总览、数据库表设计、后端模块设计、API 契约、前端组件设计、上下文压缩架构、trpc-agent-go 对齐路径均已迁移至设计文档。

- 数据模型总览与表设计详见 [10-session.design.md §4 数据层](./10-session.design.md#四数据层)
- 后端模块设计与分层详见 [10-session.design.md §1 模块概述](./10-session.design.md#一模块概述) 与 [§3 Biz 层](./10-session.design.md#三biz-层)
- API 契约（Proto/RPC/HTTP 路由）详见 [10-session.design.md §2 Proto 层](./10-session.design.md#二proto-层)
- 前端组件设计与 UX 规范详见 [10-session.design.md §8 Web 前端设计](./10-session.design.md#八web-前端设计)
- 上下文压缩架构详见 [10-session.design.md §6 上下文压缩设计](./10-session.design.md#六上下文压缩设计)
- trpc-agent-go 对齐路径详见 [10-session.design.md §7 trpc-agent-go 对齐路径](./10-session.design.md#七trpc-agent-go-对齐路径)
- 关键设计原则详见 [10-session.design.md §9 关键设计原则](./10-session.design.md#九关键设计原则)

---

## 5. 分阶段落地与能力矩阵

> Phase 划分、任务清单、状态标记、能力矩阵均已迁移至开发计划文档。

- Phase 划分与任务清单详见 [10-session.development.md §4 开发阶段](./10-session.development.md#4-开发阶段) 与 [§5 任务清单](./10-session.development.md#5-任务清单)
- 能力矩阵与实现状态详见 [10-session.development.md §2 现状评估](./10-session.development.md#2-现状评估2026-06-17) 与 [§9 已完成项速查](./10-session.development.md#9-已完成项速查勿重复排期)
