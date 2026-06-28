# Chat 对话模块 — 需求规格

> 对话框是用户与 Agent/Team 交互的核心入口，负责 HTTP/WS 发起对话、WebSocket + ActivityEventBus 实时事件、上下文管理、用量记录、停止生成、人工等待回复与待执行队列。
>
> **2026-05-17 现状对齐**：当前代码已移除独立 SSE `/v1/chat/messages/stream` 路由，实时事件以 `/v1/ws` WebSocket + ActivityEvent 为主通道（ADR-02 + ADR-03 完成后，Envelope 通用信封已删除，统一为 ActivityEvent + MonitorEvent 双类型协议）；HTTP `SendChatMessage` 保留为非流式/后台入口，并被 WS 上行、Channel、Cron 等复用。WS 重连改用 `ListActivities` RPC 拉取增量 Activity（替代原 EventBuffer 回放）。
>
> **文档边界**：本文档仅包含用户故事、功能需求清单、验收标准、非功能需求与用户视角的交互规格。代码分层、API 契约、WebSocket 协议、数据模型、实现细节见 [1-chat.design.md](./1-chat.design.md)；开发进度、技术债务、任务清单见 [1-chat.development.md](./1-chat.development.md)。

---

## 一、用户故事

### 1.1 核心对话

- 作为用户，我可以通过对话框向 Agent 或 Team 发送消息，并实时看到回复（流式文本、工具调用、推理过程）。
- 作为用户，我可以在对话过程中随时停止生成。
- 作为用户，我可以在对话进行中连续发送多条后续消息，而不必等待当前回复结束。
- 作为用户，我可以看到当前会话的上下文使用比例，了解剩余可用上下文。
- 作为用户，首次对话后会话标题能自动生成，便于在历史列表中识别。
- 作为用户，当 Agent 需要我确认或回复时，我能看到明确的等待提示并提交回复。

### 1.2 对话阶段连续发送（Follow-up Queue / 待发送队列）

> **产品定位**：对标 Cursor 等 IDE 聊天窗口——Agent 正在生成回复或执行工具时，用户仍可连续按 Enter 发送多条后续消息；消息不会丢失，也不会打断当前 turn，而是进入**对话队列**按策略处理。

- 作为用户，当 Agent 正在思考/流式输出/调用工具时，我可以继续输入并发送多条消息，而不必等待当前回复结束。
- 作为用户，我可以在消息区底部看到**待发送队列**中的消息，并取消或编辑尚未执行的内容。
- 作为用户，当队列已满或运行已结束时，我应收到明确错误提示（`CHAT_QUEUE_FULL` / `CHAT_RUN_ENDED`），而不是静默失败。

### 1.3 人工等待回复（AwaitUserReply）

- 作为用户，当 Agent 调用 `await_user_reply` 工具暂停运行时，我能看到「等待用户回复」横幅。
- 作为用户，我可以在横幅中输入回复并提交，恢复 Agent 运行。
- 作为用户，服务重启后，`awaiting_user` 状态应能恢复，我不应丢失等待中的对话。

### 1.4 执行过程可见性

- 作为用户，发送消息后，我能看到 Agent 依次调用了哪些工具/Skill/MCP，以及当前是否仍在执行。
- 作为用户，每张执行卡片标题显示**能力名称**（如 `read_file`、`skill_run`、`mcp_call`），执行中显示「正在执行」态，完成后显示**耗时**。
- 作为用户，卡片根据**成功 / 失败 / 阻塞（需确认）** 显示不同颜色与图标，失败时我能看到错误摘要。
- 作为用户，卡片**默认折叠**，不占用阅读空间；需要时点击展开查看参数 JSON、结果摘要或 stderr。
- 作为 Team 用户，在 Team 会话中，卡片标明**执行成员 Agent**（author / agent_key）。
- 作为用户，刷新页面或 WS 重连后，仍能从历史 Activity 中恢复已完成的执行卡片。
- 作为管理员，敏感参数（API Key、token）在卡片详情中**脱敏**，不泄露明文。

### 1.5 Activity-First 统一渲染（ADR-02 + ADR-03）

> **产品定位**：后端将运行时事件投影为 Activity 语义单元；前端零推理消费——按 `activity.kind` 直接渲染对应 Block 组件，无需从 Envelope 字段推断渲染类型。

- 作为用户，无论与 Spirit / Team / Agent / 独立 Agent 对话，消息流都通过统一的 `ActivityStream` 渲染，体验一致。
- 作为用户，我能看到 10 种 Activity 类型：用户消息（task）、推理过程（thinking）、工具调用（action）、最终回复（reply）、计划（plan）、待确认（confirm）、系统通知（notice）、子 Session 创建（session）、Team 阶段（team_stage）、Graph 阶段（graph_stage）。
- 作为用户，工具调用卡片按**工具类别**（shell/browser/file_read/file_write/file_search/web_search/mcp/code/todo/other）展示差异化 UI，例如文件读卡片显示行号高亮，shell 卡片分 stdout/stderr 区。
- 作为用户，当 Agent 调用 `subagent_spawn` 或 Team 分发任务时，我能看到「子 Session 创建」阶段块，并可点击进入子 Session 查看其独立 Activity 流。
- 作为 Team 用户，我能看到 Coordinator/Swarm 阶段切换的 `team_stage` 块，了解当前 Team 执行阶段。
- 作为 Graph 用户，我能看到 Graph 节点开始/结束的 `graph_stage` 块，了解 Graph 执行进度。
- 作为用户，WS 断线重连后，前端通过 `ListActivities` RPC 拉取增量 Activity 恢复时间线，顶栏显示「正在同步历史 Activity…」。

### 1.6 Session 父子树导航

> **产品定位**：Spirit 主会话、Team 会话、子 Agent 会话形成父子树，用户可在树形侧栏中导航查看任意层级的 Activity 流。

- 作为用户，我能在 Session 树侧栏看到当前根 Session 下的所有子 Session（递归层级），每个节点显示 Session 类型图标、深度 badge、执行阶段、进度百分比。
- 作为用户，点击 Session 树节点可切换到该 Session 的 Activity 流，子 Session 的 Activity 独立加载（懒加载缓存，切换不丢失已加载内容）。
- 作为用户，Session 树深度受 `subagents_max_generation_depth`（默认 3）和 `max_session_depth`（默认 5）限制，超出时收到明确错误提示而非静默失败。
- 作为用户，我能在 Session 树中识别 Spirit 主会话（`auto_awesome` 图标）、Team 会话（`groups` 图标）、子 Agent 会话（`smart_toy` 图标）、独立会话（`chat` 图标）。

---

## 二、功能需求清单

### 2.1 对话发送

- 支持 HTTP `POST /v1/chat/messages` 非流式对话。
- 支持 WebSocket `/v1/ws` 上行 `user_message` 触发对话。
- 支持 Channel（飞书/Lark webhook 等）和 Cron 入口复用同一对话主链路。
- 支持对话选项：对话模式（default/plan/code）、Provider、Model、附件引用、知识库白名单。

### 2.2 对话停止

- 支持 HTTP `POST /v1/chat/stop` 停止当前会话的生成。
- 支持 WebSocket 上行 `cancel` 停止当前 run。
- 单 Agent 与 Team 路径均支持停止。
- 停止当前 run **不自动清空** Pending FIFO（已排队消息保留，除非用户手动取消）。

### 2.3 待执行队列（Follow-up Queue）

- 运行中再次发送消息自动入队（Steerable 直注或 Pending FIFO 降级）。
- 待发送列表可见，支持取消（`POST /v1/chat/pending/cancel`）和编辑（`POST /v1/chat/pending/update`）。
- 支持 `POST /v1/chat/enqueue` 显式入队。
- 队列容量：每 session 最多 32 条 Pending，超出返回 `CHAT_QUEUE_FULL`。
- 入队成功后通过 WS ActivityEvent（Kind=notice，Domain=system）即时通知前端。
- 待执行 turn 处理 600s 超时，可被 `StopGeneration` 取消。

### 2.4 RunStatus

- `GET /v1/chat/run-status` 返回当前/最近一次 run 状态：`idle | pending | running | awaiting_user | completed | failed | cancelled | sync`。
- 状态变更通过 WS ActivityEvent（Kind=notice/task，Domain=chat）实时推送。
- 切换会话时前端通过 HTTP 校准一次状态。
- 状态通过 `state_json` 持久化，服务重启后可恢复。

### 2.5 AwaitUserReply

- `POST /v1/chat/await-reply` 提交人工回复，恢复 `awaiting_user` 状态的 run。
- 单 Agent 和 Team 路径均通过 `makeAwaitReplyFunc` 注入 AwaitHook。
- 前端在 `awaiting_user` 时展示提交回复横幅。
- 服务重启后 `awaiting_user` 状态可通过 `PendingAwaitUserReplyRoute` 恢复；`resumeInFlight` 防双 turn。

### 2.6 Session 标题自动生成

- 首次对话时触发标题生成。
- 先用截取方式快速设置标题（用户消息前 22 字符，即时反馈）。
- 异步调用 LLM 生成高质量标题（15s 超时，失败静默）。
- LLM 优先选择轻量模型（mini/flash/lite/small）。

### 2.7 对话选项

- `GET /v1/chat/options` 获取对话选项。
- `dialog_mode`：default（标准对话）/ plan（深思考，启用 BuiltinPlanner）/ code（仅代码）。
- `provider`：动态从 LLM Catalog 获取可用 Provider 列表。
- `model`：动态从 LLM Catalog 获取可用 Model 列表。

### 2.8 人工确认工具（ConfirmActivity）

- `POST /v1/chat/activities/{activity_id}/confirm` 提交工具确认（approved=true 恢复执行，approved=false 取消工具）。
- 工具阻塞时 RunStatus 进入 `awaiting_user`（`await_kind=tool_confirm`）。

### 2.9 消息反馈

- `POST /v1/chat/messages/{message_id}/feedback` 提交 👍/👎 反馈（positive/negative）。

### 2.10 后台任务

- `GET /v1/chat/jobs` 列出后台任务（按 session/agent/status 过滤）。
- `POST /v1/chat/jobs/{id}/cancel` 取消后台任务。

### 2.11 Activity-First 统一渲染（ADR-02 + ADR-03）

- 所有 chat + system 业务事件统一通过 `ActivityEvent` 在 `/v1/ws` 传输；monitor 事件（log/flow_log/mcp/alert）通过 `MonitorEvent` 传输。
- `ActivityEvent` 携带 `Domain`（chat | system）：chat 域持久化到 `activities` 表并加入时间线渲染；system 域仅推送 WS 作为通知（toast/notification，不进时间线）。
- 前端 `ActivityStream.vue` 作为统一渲染器，按 `activity.kind`（10 种：task/thinking/action/reply/plan/confirm/notice/session/team_stage/graph_stage）分发到对应 Block 组件。
- 失败统一通过 `task.failed` 表达（ActivityKind 无 error kind），错误摘要写入 `activity.content`，错误码写入 `metadata.error_code`。
- `GET /v1/sessions/{session_id}/activities` 支持按 `since_updated_at` 增量拉取，用于 WS 重连恢复时间线。

### 2.12 Session 父子树

- Session 表支持 `parent_session_id` / `root_session_id` / `agent_depth` / `session_type`（spirit/team/agent/standalone）/ `member_agent_key` / `execution_stage` / `completed_steps` / `total_steps` / `progress_pct` 字段。
- `GET /v1/sessions/{root_session_id}/tree` 返回 Session 父子树（单次查询 + 内存构建，任意深度）。
- 子 Session 创建受 `subagents_max_generation_depth`（默认 3）和 `max_session_depth`（默认 5）深度限制，超出返回明确错误。
- 前端 `SessionTreeSidebar.vue` + `SessionTreeNode.vue`（递归）渲染 Session 树，节点显示类型图标、深度 badge、执行阶段、进度百分比。

### 2.13 工具类别细分

- `Kind=action` 的 Activity 携带 `tool_category` 字段（10 种：shell/browser/file_read/file_write/file_search/web_search/mcp/code/todo/other）。
- 后端 `ToolCategorizer`（精确匹配注册表 + 前缀兜底）在投影时识别工具类别。
- 前端 `ActionBlock.vue` 作为容器，按 `tool_category` 动态渲染差异化子组件（如 `ShellActionBlock` 突出 stdout/stderr 分区，`FileReadActionBlock` 显示行号高亮，`McpActionBlock` 显示 server_key + tool 名）。

### 2.14 Team / Graph 阶段显示

- Team 阶段切换通过 `Kind=team_stage` Activity 表达，前端 `TeamStageBlock.vue` 渲染 Coordinator/Swarm 阶段。
- Graph 节点开始/结束通过 `Kind=graph_stage` Activity 表达，前端 `GraphStageBlock.vue` 渲染 Graph 执行进度。
- 子 Session 创建通过 `Kind=session` Activity（Event=child_created）表达，前端 `SessionStageBlock.vue` 渲染并可点击进入子 Session。

---

## 三、非功能需求

| 项 | 要求 |
|----|------|
| 实时性 | WS 到达后 200ms 内 UI 反映 running；完成态随 ActivityEvent=completed 即时更新 |
| 性能 | 单轮 ≥50 张执行卡片时列表仍流畅（虚拟滚动或增量 DOM） |
| 可访问性 | 卡片 header 为 `button` 或带 `aria-expanded`；状态不仅依赖颜色 |
| 国际化 | 文案走 `vue-i18n`（`chat.activity.*`） |
| 安全 | 密钥字段脱敏；详情默认可复制但带审计提示（P2） |
| 暗黑模式 | 聊天记录正文、代码块、工具结果、时间戳等文本必须保证对比度 |
| 上下文进度颜色阈值 | `<0.6` 绿 / `0.6-0.8` 黄 / `>0.8` 红 |
| WS 连接数 | 每个 session 最多 5 条连接（`maxSessionConns=5`） |
| 持久化策略 | 并行异步：persist fire-and-forget（persistChan 容量 1024）+ publish 同步；重试 5 次指数退避（100/200/400/800/1600ms）；死信环形缓冲 512 条 FIFO + activityID 去重 |
| 重连恢复 | WS 断线重连后通过 `ListActivities?since={updated_at}` 拉取增量 Activity；服务端无状态（无 EventBuffer） |
| Activity 表 | 唯一真相源；`messages`/`event_store`/`event_wal` 表已 DROP |

---

## 四、前端交互需求（用户视角）

### 4.1 左侧 Agent/Team 列表（ChatEntitySidebar）

1. 宽度 120px，高度 100%；Agent 和 Team 分组显示
2. Agent 按分类树（PlatformResourceTreeNode）分组显示
3. Team 按分组显示
4. 默认 Agent 显示在 Agent 组最上方，不可拖拽调序
5. 默认 Team 显示在 Team 组最上方，不可拖拽调序
6. 顶部搜索框：按名称搜索 Agent 和 Team，带输入提示和过滤
7. 条目：左侧显示工作状态指示灯（bolt/task_alt）和名称，右侧设置和删除按钮
8. 设置按钮：弹框显示 Agent/Team 设置界面（ChatSettingsDialog）
9. 删除按钮：弹出确认对话框（ChatDeleteDialog），工作中不可删除，删除需填写名称
10. 选中时背景高亮，右侧显示 Session 历史，中间显示最近 Session 内容
11. 首次进入默认选中默认 Agent
12. 列表右侧中间折叠按钮，带动画

### 4.2 右侧 Session 历史栏（ChatSessionSidebar）

1. 宽度 120px，高度 100%
2. 按时间线分组显示（置顶、今天、昨天、7天内、30天内、更早）
3. 每条：左侧圆环显示上下文额度比，右侧显示 Session 名称和时间
4. 支持置顶、收藏、重命名、历史追踪操作
5. 底部：左侧新建 Session，右侧一键删除历史 Session
6. 列表左侧中间折叠按钮，带动画

### 4.3 中间对话区域（ChatMessagePanel）

1. 顶部：Session 标题 + 上下文使用比例
2. 对话内容区：使用 `q-chat-message` 显示头像、时间、内容
3. 消息气泡区分用户/助手/工具事件/Team 成员
4. 助手消息中的 `reasoning` 内容需展示，展示方式：折叠区域（默认 live tail 最后两行，单击/滚轮/双击展开）
5. 工具事件以结构化卡片展示（默认折叠，展开可见参数与结果）
6. 流式消息显示打字动画
7. 底部输入区域：初始高度 100px，autogrow，最高 400px
8. 输入框底部工具条：
   - 左侧：对话模式 `QSelect`、模型提供商 `QSelect`、上下文使用量 `QCircularProgress`
   - 右侧：文件导入、发送/停止按钮
9. 文件导入时，输入框上方显示文件方框（进度、名称、关闭按钮）
10. 待执行消息列表显示在消息区底部
11. 滚动到底部按钮

### 4.4 输入与发送交互

- `Enter` 发送，`Shift + Enter` 换行
- Session 内可切换模型，后续发送使用当前选择的模型
- 模型回复或工具执行中，发送按钮切换为停止图标，点击可暂停/停止
- 运行中可连续发送（Enter 不阻塞），消息自动入队
- 入队即时反馈：WS ActivityEvent（Kind=notice，Domain=system）触发待发送列表刷新
- 队列满/运行结束：Toast + 错误码（`CHAT_QUEUE_FULL` / `CHAT_RUN_ENDED`）
- `awaiting_user` 时展示提交回复横幅
- WS 重连后顶栏显示「正在同步历史 Activity…」，通过 `ListActivities` RPC 拉取增量

---

## 五、验收标准

### 5.1 核心对话与执行可见性

- [x] 无 `/v1/chat/messages/stream` 当前端点表述
- [x] WS 控制消息在需求/设计文档中完整描述
- [x] Team 停止/待执行与单 Agent 一致
- [x] AwaitUserReply：后端 + Chat 页可提交回复
- [x] `activity.pending_id` 前端可消费（原 `error.pending_id`）
- [x] `session_turns` Agent + Team 均有记录
- [x] Channel/Cron 不绕过 active run 互斥
- [x] Team `member_agent_key` 后端发射 + 前端增量展示
- [x] 工具执行结构化卡片（参数/结果/耗时/`is_long_running`）
- [x] Reasoning 折叠/展示符合产品规格
- [x] RunStatus 与 WS ActivityEvent 一致（切换会话时 HTTP 校准）
- [x] WS 重连后用户可见「同步中」状态
- [x] 单 Agent 对话：调用任意已挂载工具时，Chat 中出现对应卡片；执行中显示「正在执行」，完成后显示耗时与成功/失败态
- [x] Skill：`skill_load` / `skill_run` 显示 Skill 图标与 skill 名称摘要
- [x] MCP：`mcp_call` 显示 server 与 tool 名摘要
- [x] 卡片**默认折叠**；展开后可见参数与结果
- [x] 同一工具调用不产生 duplicate 卡片（activity.id 稳定 upsert）
- [x] WS 断线重连 + `ListActivities` 拉取增量后，卡片状态与线上一致
- [x] 刷新会话历史：已完成轮次的卡片从 `activities` 表只读还原（持久化命中）
- [x] Team 会话：卡片展示成员 Agent 标识（P1）
- [x] 失败工具调用：卡片红色边框 + 错误摘要，助手正文仍可继续输出

### 5.2 Activity-First 架构（ADR-02 + ADR-03）

- [x] Envelope 通用信封已删除，WS 下行为 `activity_event?` + `monitor_event?` 双类型协议
- [x] 10 种 ActivityKind 全覆盖：task/thinking/action/reply/plan/confirm/notice/session/team_stage/graph_stage
- [x] 7 种 ActivityEventType 全覆盖：created/streaming/updated/completed/failed/cancelled/child_created
- [x] `Domain=chat` 持久化到 `activities` 表；`Domain=system` 仅推送 WS 不持久化
- [x] 失败统一通过 `task.failed` 表达（无 `ActivityKindError`）
- [x] `ActivityStream.vue` 作为统一渲染器，按 `activity.kind` 分发到 10 种 Block 组件
- [x] 并行异步持久化：persist fire-and-forget + publish 同步
- [x] 重试预算：5 次指数退避（100/200/400/800/1600ms）+ `select` on done channel（Close 不阻塞）
- [x] 死信环形缓冲：512 条 FIFO + activityID 去重
- [x] `messages`/`event_store`/`event_wal` 表已 DROP，`activities` 表为唯一真相源

### 5.3 Session 父子树

- [x] Session 表含 9 个父子树字段（parent_session_id/root_session_id/agent_depth/session_type/member_agent_key/execution_stage/completed_steps/total_steps/progress_pct）
- [x] `GetSessionTree` RPC 单次查询 + 内存构建树（任意深度）
- [x] 深度受 `subagents_max_generation_depth`（默认 3）+ `max_session_depth`（默认 5）限制
- [x] 前端 `SessionTreeSidebar.vue` + `SessionTreeNode.vue`（递归）渲染，节点显示类型图标/深度 badge/执行阶段/进度
- [x] 子 Session Activity 懒加载缓存（`ensureActivitiesLoaded` 命中跳过 RPC）

### 5.4 工具类别与 Team/Graph 阶段

- [x] 10 种 ToolCategory：shell/browser/file_read/file_write/file_search/web_search/mcp/code/todo/other
- [x] `ToolCategorizer` 精确匹配注册表 + 前缀兜底
- [x] `ActionBlock.vue` 按 `tool_category` 动态渲染 10 种差异化子组件
- [x] `team_stage` Activity 表达 Team 阶段切换，`TeamStageBlock.vue` 渲染
- [x] `graph_stage` Activity 表达 Graph 节点进度，`GraphStageBlock.vue` 渲染
- [x] `session` Activity（Event=child_created）表达子 Session 创建，`SessionStageBlock.vue` 渲染并可点击进入子 Session

> 实现进度、技术债务与优化方向详见 [1-chat.development.md](./1-chat.development.md)。

---

## 子模块：Chat 执行过程卡片

> **模块**：Chat 对话 · 执行可见性
> **上级模块**：[1 chat.md](./1%20chat.md)
> **技术设计**：[1 chat-execution-trace.design.md](./1%20chat-execution-trace.design.md)
> **遵循**：[guides/AI-DEVELOPMENT-SPECIFICATION.md](../guides/AI-DEVELOPMENT-SPECIFICATION.md) · [guides/frontend-guide.md](../guides/frontend-guide.md)

---

## 1. 背景与目标

用户在 Chat 中与 Agent 对话时，需要**实时看到 Agent 正在做什么**（调用工具、加载/运行 Skill、通过 MCP 访问外部能力等），而不是仅看到最终文本回复或零散的 Markdown 提示。

本需求在**聊天消息流**中插入可折叠的**执行过程卡片（Execution Activity Card）**：卡片以图标 + 名称 + 状态为主信息，默认折叠详情，展开后可查看参数与结果。

> **与 Monitor 的边界**：Monitor → Logs / Traces 面向运维排障；Chat 执行卡片面向**终端用户理解 Agent 行为**，二者可共享同一条 `trace_id` / Span，但**不在 Chat 中展示 FlowLog 原始步骤**。

---

## 2. 用户故事

| ID | 角色 | 故事 | 优先级 |
|----|------|------|--------|
| U1 | 对话用户 | 发送消息后，我能看到 Agent 依次调用了哪些工具/Skill/MCP，以及当前是否仍在执行 | P0 |
| U2 | 对话用户 | 每张卡片标题显示**能力名称**（如 `read_file`、`skill_run`、`mcp_call`），执行中显示「正在执行」态，完成后显示**耗时** | P0 |
| U3 | 对话用户 | 卡片根据**成功 / 失败 / 阻塞（需确认）** 显示不同颜色与图标，失败时我能看到错误摘要 | P0 |
| U4 | 对话用户 | 卡片**默认折叠**，不占用阅读空间；需要时点击展开查看参数 JSON、结果摘要或 stderr | P0 |
| U5 | Team 用户 | 在 Team 会话中，卡片标明**执行成员 Agent**（author / agent_key） | P1 |
| U6 | 对话用户 | 刷新页面或 WS 重连后，仍能从历史消息中恢复已完成的执行卡片（与当轮持久化一致） | P1 |
| U7 | 管理员 | 敏感参数（API Key、token）在卡片详情中**脱敏**，不泄露明文 | P0 |

---

## 3. 功能范围

### 3.1 纳入范围（P0）

| 能力类型 | 典型名称 | 说明 |
|----------|----------|------|
| 平台工具 | `read_file`、`save_file`、`exec_command`、`todo_write` 等 | catalog / runtime 工具 |
| Skill | `skill_load`、`skill_run`、`skill_search` | 框架 Skill 工具族 |
| MCP | `mcp_call`、`mcp_list_tools` 及 MCP ToolSet 挂载工具 | 含 server_key / tool 名 |
| 内置能力 | `knowledge_search`、`call_agent`、`await_user_reply` | 随 Agent 策略挂载 |

### 3.2 纳入范围（P1）

| 能力类型 | 说明 |
|----------|------|
| 子 Agent | `transfer_to_agent`、`spawn_subagent` |
| Memory | `load_memory`、`preload_memory` |
| Team 步骤 | 与 `team_step_*` 事件对齐的成员级卡片（可选与成员气泡并列） |

### 3.3 不纳入范围

- Monitor FlowLog / Process Log 的 UI 迁移（仍在 Monitor → Logs）
- Graph 节点执行卡片（归属 Graph 执行页，见 `graphs` 模块）
- 修改 Agent 框架 `pkg/trpc-agent-go` 内部 Tool 语义（Aranea 仅在投影层扩展）

---

## 4. 交互规格

### 4.1 卡片布局（折叠态 — 默认）

```
┌─────────────────────────────────────────────────────────┐
│ [图标]  read_file                    正在执行…  ⏳      │
└─────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────┐
│ [图标]  skill_run · planning-and-task    1.2s  ✓      │
└─────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────┐
│ [图标]  mcp_call · github/list_issues    320ms  ✗      │
└─────────────────────────────────────────────────────────┘
```

| 区域 | 规则 |
|------|------|
| 图标 | 按 `activity_kind` 映射（tool / skill / mcp / …），见设计文档 §6 |
| 主标题 | `display_label`：优先 catalog 中文名，其次 runtime 名 |
| 副标题（可选） | 单行摘要：路径、skill 名、MCP server:tool，**不显示完整 JSON** |
| 右侧状态 | 执行中：`正在执行` + 动画；完成：耗时（≥1s 保留 1 位小数 + `s`，否则 `ms`） |
| 状态标记 | 成功 ✓ / 失败 ✗ / 阻塞 ⚠ / 长任务 ⏱ |

### 4.2 卡片布局（展开态）

点击卡片头部或 chevron 展开：

- **参数**：格式化 JSON（可滚动，最大高度约 280px）
- **结果**：stdout / JSON / 错误信息分区展示
- **元数据**（可选折叠）：`trace_id`、`run_id`、`invocation_id`、`duration_ms`

### 4.3 时间线位置

- 卡片插入在**当轮助手回复流**中，与 `text_delta` 交错，顺序等于 Agent **实际执行顺序**
- 同一 `activity_id` 仅对应**一张卡片**：`running` → `success|failed` 为**原地更新**，不新增重复行
- 不替代最终助手 Markdown 气泡；卡片与文本气泡均为 `role=assistant` 时间线上的独立条目

### 4.4 状态文案

| status | 折叠态标签 | 颜色语义 |
|--------|------------|----------|
| `running` | 正在执行 | warning / accent（UX `--color-warning`） |
| `success` | （仅耗时） | success（`--color-success`） |
| `failed` | 失败 + 耗时 | danger（`--color-danger`） |
| `blocked` | 待确认 | warning |
| `cancelled` | 已取消 | muted |

---

## 5. 非功能需求

| 项 | 要求 |
|----|------|
| 实时性 | WS 到达后 **200ms 内** UI 反映 running；完成态随 `tool_result` 即时更新 |
| 性能 | 单轮 ≥50 张卡片时列表仍流畅（虚拟滚动或增量 DOM，见设计 §8） |
| 可访问性 | 卡片 header 为 `button` 或带 `aria-expanded`；状态不仅依赖颜色 |
| 国际化 | 文案走 `vue-i18n`（`chat.activity.*`） |
| 安全 | 密钥字段脱敏；详情默认可复制但带审计提示（P2） |

---

## 6. 验收标准

> **注**：本子模块的验收标准已被主文档 §5.1 取代（AF 架构后）。下方为历史验收记录。

- [x] 单 Agent 对话：调用任意已挂载工具时，Chat 中出现对应卡片；执行中显示「正在执行」，完成后显示耗时与成功/失败态
- [x] Skill：`skill_load` / `skill_run` 显示 Skill 图标与 skill 名称摘要
- [x] MCP：`mcp_call` 显示 server 与 tool 名摘要
- [x] 卡片**默认折叠**；展开后可见参数与结果
- [x] 同一工具调用不产生 duplicate 卡片（activity.id 稳定 upsert）
- [x] WS 断线重连 + `ListActivities` 拉取增量后，卡片状态与线上一致
- [x] 刷新会话历史：已完成轮次的卡片从 `activities` 表只读还原（持久化命中）
- [x] Team 会话：卡片展示成员 Agent 标识（P1）
- [x] 失败工具调用：卡片红色边框 + 错误摘要，助手正文仍可继续输出

---

## 7. 关联文档

| 文档 | 关系 |
|------|------|
| [1 chat.md](./1%20chat.md) | Chat 主需求、WS ActivityEvent 总览 |
| [1 chat.design.md](./1%20chat.design.md) | `ActivityStream` 投影 + `tool_category` 细分 |
| [52-flow-logger.design.md](./52-flow-logger.design.md) | Span / trace_id 同源；Chat 不展示 flow_log |
| [23 tools.md](./23%20tools.md) | 工具 catalog 与 risk_level |
| [20 skill.md](./20%20skill.md) | Skill 运行时 |
| `aranea-frontend-guide` SKILL §6 | 玻璃卡片视觉 token |

---

## 子模块：Team 团队历史显示需求

> **状态**：2026-06-28 新增 | **修正**：本节修正/替代主文档 §1.5 Activity-First 中关于 Team 显示的部分
> **设计**：详见 [1-chat.design.md §子模块：Team 团队历史显示设计](./1-chat.design.md)
> **开发计划**：详见 [1-chat.development.md §子模块：Team 团队历史显示开发计划](./1-chat.development.md)

### A.1 核心修正点

| 维度 | 原需求 | 修正后 |
|------|--------|--------|
| Team 排序 | 按活跃度排序 | 按 graph 流程图顺序（后端 WS 指令创建顺序） |
| 任务面板 | 团队卡片列表 | 三层结构：plan → graph → team/agent 任务栏 |
| 面板产生 | 每次 team 组建产生新面板 | 任务计划面板固定位置，不重复产生，通过 WS 更新状态 |
| MaxDepth | 硬编码 = 2 | 由 `AgentRuntimeSetting.MaxSessionDepth` 配置 |
| 排序字段 | 全局 Seq 递增 | 用现有 Timestamp 字段，移除全局 Seq |

### A.2 Agent 统一性原则

**所有 agent 本质相同**：
- 精灵（父节点）和子 agent（包裹在 team 外衣下）都是 agent
- 会话输出内容和展现形式相同（thinking + action + reply）
- 后端交互逻辑相同（ActivityProjector 路径）
- 区别仅在于父子关系（`parentActivityId`）和深度（`agent_depth`）

### A.3 用户故事

#### A.3.1 简单对话模式
- 作为用户，当我发送简单问题时，精灵直接 thinking + reply，不组建团队
- 作为用户，我能看到精灵的推理过程（thinking，可折叠）和最终回复（reply）

#### A.3.2 Team 模式（team-card）
- 作为用户，当我发送复杂任务时，精灵评估并拆解任务，显示**任务计划面板**（plan，固定位置，不重复产生）
- 作为用户，我能看到任务计划面板中的任务拆解列表和依赖关系
- 作为用户，当计划变更时，原面板更新新计划（不产生新面板）
- 作为用户，计划面板完成后可折叠为摘要（显示"✅ N 项任务已完成"）
- 作为用户，我能看到 **Graph 流程图**（graph_stage，在 plan 之后、team 之前独立显示）
- 作为用户，我能看到 **Team 任务栏**（team-card），显示团队名称、任务名称、创建时间、成员头像、进度条、状态、耗时
- 作为用户，我能在 team-card 尾部点击对话框补充信息（点击横向展开，显示发送按钮）
- 作为用户，我能点击暂停/恢复按钮控制 team 执行
- 作为用户，我能点击 team-card 展开查看成员列表
- 作为用户，展开后能看到每个成员的 thinking/action/reply 序列
- 作为用户，team 失败时能看到错误信息和重试按钮

#### A.3.3 子 Agent 模式（agent-card）
- 作为用户，当精灵直接调用子 agent（subagent_spawn）时，我能看到 **agent-card**（简化版）
- 作为用户，agent-card 显示 agent 名称、状态、时间，以及暂停/恢复按钮
- 作为用户，我能在 agent-card 尾部点击对话框补充信息（与 team-card 一致交互）
- 作为用户，我能点击 agent-card 展开查看该 agent 的 thinking/action/reply 序列

#### A.3.4 历史恢复
- 作为用户，刷新页面后，能从数据库完全复原历史对话内容
- 作为用户，历史加载时只加载 spirit 根 session 事件，子 session 事件按需懒加载
- 作为用户，点击 team-card/agent-card 展开时，懒加载子 session 事件
- 作为用户，已完成的 team 默认折叠，进行中的 team 默认展开

### A.4 功能需求

#### A.4.1 任务计划面板（PlanBlock）
- **作用**：第一，为 agent 执行提供任务执行指导（在 session 记忆中保持任务方向）；第二，为用户提供进度可观测性
- **作用范围**：每个 turn 独立一个任务计划面板
- **固定语义**：同一 turn 内只产生一个面板，后续 plan 更新事件在原面板更新
- **折叠行为**：支持折叠；进行中默认展开；**初始渲染时**若所有 plan item 已完成则自动折叠为摘要（X/N）；运行中变为全部完成不触发自动折叠（用户意图优先）；可手动切换
- **状态更新**：由执行者发出（team_stage/agent 执行状态变化时更新对应 plan item 状态）
- **计划变更**：直接更新 plan 内容（替换原面板的 items 列表），不引入 diff 标记（不显示"➕新增/⊘已移除/✏️已变更"）。理由：plan 变更通常发生在拆解任务过程中，diff 标记增加 UI 复杂度而无实质价值；用户关心的是当前最新的 plan，而非变更历史

#### A.4.2 Graph 流程图（GraphStageBlock）
- **位置**：在 plan 之后、team 之前独立显示
- **时机**：Spirit 完成 team 分配后创建
- **节点状态**：pending（灰色）/ running（蓝色+pulse）/ completed（绿色✓）/ failed（红色✗）/ interrupted（黄色⏸）。状态值与 Plan item 状态值、team 状态值保持一致，详见 [设计 B.4.4](./1-chat.design.md#b44-graph-流程图graphstageblock)

#### A.4.3 Team 任务栏（TeamCard）
- **布局**：长条卡片，头部2:中部6:尾部2
- **头部**（20%）：上中下三部分（1:1:1）—— 团队名称 / 任务名称 / 创建时间
- **中部**（60%）：上下两部分（1:2）—— 成员头像+名称 / 进度条:状态:耗时（3:1:1）
- **尾部**（20%）：对话框（收缩状态，点击横向展开，显示发送按钮）+ 停止/恢复按钮
- **进度计算**：子任务完成数 X/N（completed/total * 100%）
- **暂停/恢复**：running 显示"⏸ 停止"；interrupted 显示"▶ 恢复"；终态隐藏
- **重试**：failed/interrupted 状态显示"🔄 重试"按钮
- **用户补充信息**：输入框 + 发送按钮，触发 `POST /v1/teams/{id}/inject`
- **展开/折叠**：running 默认展开；终态默认折叠；可手动切换

#### A.4.4 Agent 卡片（AgentCard）
- **布局**：简化版，头部80% + 尾部20%
- **头部**：avatar + agent 名称 + status badge + 创建时间
- **尾部**：暂停/恢复按钮 + 对话框（与 team-card 一致交互）
- **展开后**：thinking（折叠）/ action（折叠）/ reply（展开）
- **无团队信息、无进度条**（单个 agent，直接显示状态）

#### A.4.5 折叠规则
| 节点类型 | 折叠行为 |
|---------|---------|
| thinking | 默认折叠（不区分进行中/完成，减少噪音） |
| action | 默认折叠（不区分进行中/完成，减少噪音） |
| task | 始终展开 |
| reply | 始终展开 |
| team-card | running 默认展开，终态默认折叠，可手动切换 |
| agent-card | 同 team-card |
| plan | 支持折叠：进行中默认展开，初始渲染时若全部完成则自动折叠为摘要 |
| graph_stage | 始终展开 |

> 详见 [设计 B.4.5 折叠规则](./1-chat.design.md#b45-折叠规则统一整理)。统一规则：用户手动展开/折叠后状态由用户掌控，不被状态变化自动覆盖（用户意图优先）。

#### A.4.6 异常处理
- **Team 失败**：手动重试（不自动重试，避免无限循环）；显示错误信息和重试按钮
- **Member 失败**：Team 自治决策（跳过/重新分配/标记 team 失败）；不自动重试 member
- **卡住场景**：先记录不实现（主要卡在工具执行上）；后续迭代设计心跳检测 + 卡住告警

#### A.4.7 历史加载 
- **策略**：只加载 spirit 根 session 事件，子 session 事件按需懒加载
- **流程**：进入 spirit session → ListBySession(spiritSessionID) → 按 parentActivityId 构建 ActivityTree → 渲染
- **懒加载触发**：点击 team-card/agent-card 展开时，检查子 session activity 是否已加载，未加载则调用 ListBySession(teamSessionID/agentSessionID)
- **后端修复**：direct-publish 事件（Team/Graph）必须填 SpiritSessionID（当前未填，需修复）

### A.5 验收标准

- [ ] 简单对话：精灵直接 thinking + reply，无 plan/graph/team
- [ ] Team 模式：plan → graph → team-card 顺序显示
- [ ] 任务计划面板：固定位置，不重复产生，支持折叠
- [ ] team-card 布局：头部2:中部6:尾部2，符合设计
- [ ] team-card 尾部：对话框横向展开 + 发送按钮 + 暂停/恢复按钮
- [ ] team-card 展开：显示成员列表，成员展开显示 thinking/action/reply
- [ ] agent-card：简化版布局，含补充输入框
- [ ] plan 状态更新：由 team_stage 事件驱动
- [ ] 进度计算：X/N 简单实现
- [ ] Team 失败：手动重试
- [ ] 历史加载：只加载 spirit 根 session，子 session 懒加载
- [ ] direct-publish 事件：SpiritSessionID 已填充
- [ ] 排序：用 Timestamp，无全局 Seq
- [ ] MaxDepth：从 AgentRuntimeSetting.MaxSessionDepth 读取
