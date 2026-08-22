# 76 编程 Agent 桥接（Coding Agent Bridge）设计文档

> 设计文档 ｜ 2026-08-12 ｜ 状态：已批准
> 需求见 [76-coding-agent-bridge.md](./76-coding-agent-bridge.md) ｜ 方案调研见 [reports/2026-08-12-plan-spirit-coding-agent-bridge.md](../reports/2026-08-12-plan-spirit-coding-agent-bridge.md)

---

## 1. 架构总览

```
用户语音/聊天 ──► 精灵助手编排（spirit_team，现有入口零改动）
                      │ intent 识别：编程任务
                      ▼
              CodingBridge Tools（internal/tools/codingbridge/，3 个 trpc tool）
                      │
                      ▼
              AgentBridgeService（internal/service/agentbridge.go）
                ├── TaskManager：任务状态机 + 持久化 + 事件聚合限流
                ├── ProcessManager：子进程 spawn/复用/回收/崩溃检测
                └── ApprovalRelay：ACP permission ↔ 确认门事件链路
                      │ stdio NDJSON (JSON-RPC 2.0)
                      ▼
              ACP Client（internal/agentbridge/acp/，协议客户端子集）
                      │
        ┌─────────────┼────────────────┐
        ▼             ▼                ▼
  codebuddy --acp   claude-code-acp   codex-acp      （外部子进程）
```

**关键选型决策**：不扩展现有 `biz/a2a.RemoteAgent`——其为 A2A URL 模型（RemoteURL/AgentCardURL），与本地 stdio 子进程语义不符。新建独立 biz 域 `internal/biz/agentbridge`，避免污染 A2A 联邦域。

## 2. 技术选型：ACP 协议子集

ACP（Agent Client Protocol）= JSON-RPC 2.0 over stdio NDJSON，Client（本系统）spawn Agent 进程并长连接驱动。

| 能力 | ACP 方法/通知 | 本模块用途 |
|------|--------------|-----------|
| 握手 | `initialize` | 协商 protocolVersion=1 + capabilities（声明 fs/terminal 不支持，由 agent 本地执行） |
| 建会话 | `session/new`（cwd=项目路径） | 每个派发任务一个 ACP session |
| 派发 | `session/prompt` | 发送任务描述，阻塞至完成（流式） |
| 进度 | `session/update`（agent→client 通知） | 聚合为进度事件（限流） |
| 审批 | `session/request_permission`（agent→client 请求，需响应） | 中继到确认门 |
| 取消 | `session/cancel`（client→agent 通知） | 任务取消 |
| 认证 | `authenticate` | 一期不实现（依赖 CLI 自身登录态） |

**不实现**（YAGNI）：`session/load`（会话恢复）、`fs/read_text_file`/`terminal/*`（client 能力代理）、MCP-over-ACP、unstable_ 前缀方法。

## 3. 代码分层与包结构

```
internal/agentbridge/acp/        # ACP 协议客户端（无业务依赖，可独立测试）
    ├── client.go                # Client：Initialize/NewSession/Prompt/Cancel
    ├── conn.go                  # NDJSON 帧读写 + JSON-RPC id 路由 + 挂起请求表
    ├── process.go               # 子进程 spawn/stdout pipe/退出监视
    └── types.go                 # 协议类型（SessionUpdate/PermissionRequest 等）

internal/biz/agentbridge/        # 领域层：实体 + Repo 窄接口 + Usecase
    ├── types.go                 # CodingAgent/CodingProject/CodingTask 实体
    ├── repo.go                  # AgentRepo/ProjectRepo/TaskRepo 窄接口（≤5 方法）
    ├── task_state_machine.go    # 显式状态机（AS-FSM-01）
    └── usecase.go               # AgentBridgeUsecase：任务生命周期编排

internal/data/                   # coding_agent_repo.go / coding_project_repo.go / coding_task_repo.go
internal/data/ent/schema/        # coding_agent.go / coding_project.go / coding_task.go

internal/service/agentbridge.go  # AgentBridgeService：对 tools/proto 暴露用例
internal/service/agentbridge_approval.go  # 审批中继（复用确认门事件链路）

internal/tools/codingbridge/     # dispatch/check/cancel 三个工具 + ToolSet 注册

api/kratos/agentbridge/v1/       # 管理 API proto（agent/project CRUD、task 查询/取消、审批回传）

web/src/pages/AgentBridgePage.vue（M3）+ 确认卡片扩展（M2）
```

**依赖方向**：`tools/codingbridge` → `service/agentbridge` → `biz/agentbridge` ← `data`。`acp` 包被 service 依赖，biz 不感知 ACP（端口隔离）。

## 4. 数据模型

### coding_agents（编程工具注册表）

| 字段 | 类型 | 说明 |
|------|------|------|
| id | uuid | 主键 |
| workspace | string | 工作区（默认 default） |
| agent_key | string | 唯一标识：`claude_code` / `codex` / `codebuddy` |
| display_name | string | 显示名 |
| command | string | 启动命令（如 `codebuddy`、`npx`、`claude-code-acp`）。E9：`codebuddy` / `claude_code` / `codex` 可省略，由 `DefaultACPLaunch` 填 argv |
| args_json | json | 启动参数（如 `["--acp"]`、`["-y","@zed-industries/claude-code-acp"]`） |
| env_json | json | 附加环境变量 |
| enabled | bool | 启用开关 |
| last_probe_ok | bool | 最近探测结果 |
| last_probe_error | string | 最近探测错误 |
| created_at / updated_at | time | |

唯一约束：`(workspace, agent_key)`

### coding_projects（项目目录注册表）

| 字段 | 类型 | 说明 |
|------|------|------|
| id | uuid | 主键 |
| workspace | string | 工作区 |
| name | string | 项目名（语音解析键，如 `aranea-agents`） |
| path | string | 本机绝对路径（如 `F:\aranea-agents`） |
| description | string | 描述（辅助 LLM 消歧） |
| created_at / updated_at | time | |

唯一约束：`(workspace, name)`

### coding_tasks（任务表）

| 字段 | 类型 | 说明 |
|------|------|------|
| id | uuid | 任务 ID（播报/查询用短码取前 8 位） |
| workspace | string | 工作区 |
| session_id | string | 发起任务的精灵会话 ID（审批卡片路由目标） |
| agent_id | uuid | 外键 → coding_agents |
| project_id | uuid | 外键 → coding_projects |
| prompt | text | 任务描述 |
| status | string | 状态机枚举（见 §5） |
| acp_session_id | string | ACP 侧 session ID |
| summary | text | 完成摘要（截断 4000 字符） |
| error | text | 失败原因 |
| progress_count | int | 进度事件计数（限流统计） |
| created_at / updated_at / completed_at | time | |

索引：`(session_id, created_at)`、`(status)`

## 5. 任务状态机（AS-FSM-01）

文件：`internal/biz/agentbridge/task_state_machine.go`

```
dispatched ──agent_started──► running ──permission_requested──► awaiting_approval
    │                           │   ▲                              │
    │ start_failed              │   │ permission_resolved          │ timeout(5min)
    ▼                           │   │                              ▼
 failed                         │   │                          cancelled（挂起语义并入取消，
    ▲                           │   │                              恢复=dispatch 新任务）
    │                           │   │
    └── crash/exit ≠ 0 ─────────┘   └──prompt_done──► done
                                    └──cancel──► cancelling──► cancelled
```

| 状态 | 含义 |
|------|------|
| dispatched | 已落库，进程未启动 |
| running | ACP session/prompt 执行中 |
| awaiting_approval | 等待用户审批（计时 5 分钟） |
| cancelling | 取消信号已发，待进程确认 |
| done / failed / cancelled | 终态 |

非法转换返回 `ErrInvalidTransition`；所有转换写流程日志（K5）。

## 6. 核心流程序列图

### 6.1 任务派发 + 审批中继（核心）

```
用户        精灵       Tools        Service       ACP Client     外部Agent    确认门/前端
 │ 语音指令   │           │             │              │              │            │
 │──────────►│ dispatch  │             │              │              │            │
 │           │──────────►│ DispatchTask│              │              │            │
 │           │           │────────────►│ 解析项目/探测 │              │            │
 │           │           │             │ spawn+init   │              │            │
 │           │           │             │─────────────►│──spawn──────►│            │
 │           │           │             │              │──initialize─►│            │
 │           │           │             │              │──session/new►│            │
 │           │           │             │              │──prompt─────►│            │
 │           │           │             │              │◄─session/update────────────│ (限流聚合→进度事件)
 │           │           │             │              │◄─request_permission────────│
 │           │           │             │◄─onPermission│              │            │
 │           │           │             │ 构造确认事件 ──────────────────────────────────►│ 卡片+TTS
 │◄────────────────────────────────────────────────────────────────────────────────│ 播报
 │ 语音"允许" │           │             │              │              │            │
 │────────────────────────────────────────────────────►│ ConfirmBridge│            │
 │           │           │             │              │──permission response───────►│ 继续执行
 │           │           │             │              │◄─prompt result─────────────│
 │◄─TTS 播报 │◄─结果卡片◄│◄─GetTask────│◄─done────────│              │            │
```

### 6.2 项目名消歧

派发工具入参 `project_name` → ProjectRepo 精确匹配 → 失败则前缀/模糊匹配：
- 1 个候选 → 直接使用
- 多个候选 → 工具返回 clarify 结构（复用 StepCreatedEvent 澄清链路），用户选择后重新 dispatch
- 0 个候选 → 返回错误"未找到项目 X，请先注册"，附已注册项目列表

## 7. 接口定义

### 7.1 Biz 端口（`internal/biz/agentbridge/`）

```go
// Stability:evolving
type AgentRepo interface {
    GetByKey(ctx context.Context, workspace, key string) (*CodingAgent, error)
    List(ctx context.Context, workspace string) ([]*CodingAgent, error)
    Upsert(ctx context.Context, agent *CodingAgent) error
    UpdateProbe(ctx context.Context, id string, ok bool, errMsg string) error
}

// Stability:evolving
type ProjectRepo interface {
    GetByName(ctx context.Context, workspace, name string) (*CodingProject, error)
    Match(ctx context.Context, workspace, query string) ([]*CodingProject, error) // 精确+前缀+包含
    List(ctx context.Context, workspace string) ([]*CodingProject, error)
    Upsert(ctx context.Context, p *CodingProject) error
    Delete(ctx context.Context, id string) error
}

// Stability:evolving
type TaskRepo interface {
    Create(ctx context.Context, t *CodingTask) error
    Get(ctx context.Context, id string) (*CodingTask, error)
    UpdateStatus(ctx context.Context, id string, from, to TaskStatus, patch TaskPatch) error // CAS
    ListBySession(ctx context.Context, sessionID string, limit int) ([]*CodingTask, error)
    ListActive(ctx context.Context) ([]*CodingTask, error) // 启动恢复用
}

// ACPSession 是 biz 对协议层的端口（service 实现注入）
// Stability:internal
type ACPSession interface {
    Prompt(ctx context.Context, cwd, prompt string, h EventHandler) (string, error) // 返回结果摘要
    Cancel(ctx context.Context) error
    Close() error
}

type EventHandler interface {
    OnUpdate(update SessionUpdate)        // 进度（限流在 service 层）
    OnPermission(req PermissionRequest) PermissionResponse // 同步等待审批（带超时）
}
```

### 7.2 Service 方法（`internal/service/agentbridge.go`）

```go
DispatchTask(ctx, sessionID, agentKey, projectQuery, prompt) (*CodingTask, []Candidate, error)
GetTask(ctx, id) / ListSessionTasks(ctx, sessionID) / CancelTask(ctx, id)
ConfirmBridgePermission(ctx, taskID string, optionID string) error  // 审批回传
// 管理 API
UpsertAgent / ListAgents / ProbeAgent / UpsertProject / ListProjects / DeleteProject
```

### 7.3 Proto API（`api/kratos/agentbridge/v1/agentbridge.proto`）

| RPC | HTTP | 说明 |
|-----|------|------|
| UpsertAgent / ListAgents / DeleteAgent / ProbeAgent | POST/GET/DELETE `/v1/agentbridge/agents` | 工具注册管理 |
| UpsertProject / ListProjects / DeleteProject | POST/GET/DELETE `/v1/agentbridge/projects` | 项目注册管理 |
| ListTasks / GetTask / CancelTask | GET/POST `/v1/agentbridge/tasks` | 任务查询/取消 |
| ConfirmPermission | POST `/v1/agentbridge/tasks/{id}/confirm` | 审批回传（WS 消息同语义） |

### 7.4 精灵工具（`internal/tools/codingbridge/`）

| 工具名 | 入参 | 返回 |
|--------|------|------|
| `coding_dispatch_task` | agent_key, project_name, task | task_id + 状态 / clarify 候选 / 错误 |
| `coding_check_task` | task_id（缺省=本会话最近一个） | 状态 + 进度摘要 |
| `coding_cancel_task` | task_id（缺省=本会话最近执行中） | 取消结果 |

## 8. 审批中继设计（复用确认门）

**关键复用点**（已核实代码）：

1. **出站**：ACP `session/request_permission` → 构造与 `ToolConfirmationRequest` 同构的 step 事件（agent 层 `tool_confirm_gate.go` 的事件结构）→ `event.Bus` → WS → 前端精灵确认卡片 `HoloConfirmCard.vue`（companion 域，自带倒计时与队列展示）。卡片扩展字段：`source=external_coding`、agent_key、project_name（前端按 source 展示工具名标题）
2. **回传**：复用 `ChatService.ConfirmActivity`（[chat_confirm.go](../../internal/service/chat_confirm.go)）的 HTTP/WS 双通道语义，新增 `ConfirmBridgePermission` 用例：校验任务处于 awaiting_approval → 解析 optionID → 写回 ACP response → 状态机转 running
3. **语音侧**：复用 clarify 链路——审批事件触发 TTS 播报（"Claude Code 想执行 go test，允许吗？"），用户语音作答（允许/拒绝/始终允许）经现有澄清路由解析后调用 `ConfirmBridgePermission`。**E2（2026-08-22）已接**：`coding_task_approval` / `coding_task_cancelled`（`speak:true`）由 `voice.Session.maybeSpeakCodingApproval` 走委派 FIFO 口播；前端 `spirit/handleSystemNotice` 写 `pendingCodingApproval` 并 toast
4. **超时**：awaiting_approval 起 5 分钟计时，超时回传 ACP cancelled + 状态机转 cancelled + 语音告知
5. **选项映射**：ACP permission options（allow_once/allow_always/reject_once 等 kind）→ 卡片按钮；`allow_always` 作用域限定为**本任务内**（内存缓存，不落 tool-grants 库，避免与内部工具授权语义混淆）

## 9. 前端组件设计

| 组件 | 位置 | 说明 |
|------|------|------|
| HoloConfirmCard 扩展 | `web/src/components/companion/HoloConfirmCard.vue` + `useCompanionConfirms.ts` | 识别 `source=external_coding`，标题渲染 `{agent_display_name} · {project_name}`；按钮组不变（自带倒计时/队列语义直接复用） |
| CodingTaskCard | `web/src/components/chat/v2/CodingTaskCard.vue` | 结果卡片：状态图标 + 工具/项目/耗时/摘要 + "在 Trae 中打开"按钮 |
| AgentBridgePage | `web/src/pages/`（M3） | 两个表格：编程工具（启停/探测状态）、项目目录（名称/路径/描述） |
| Trae 拉起 | CodingTaskCard 内 | 调用 `POST /v1/agentbridge/trae/open {project_id}`，后端执行 `trae <path>`（`explorer` 兜底） |

事件注册点：`internal/event/bus_v2.go` 新事件类型 `coding_task_progress` / `coding_task_completed`；`ws_v2_wire_convert.go` 增加转换。

## 10. 错误处理

| 场景 | 检测 | 处理 |
|------|------|------|
| 命令不存在 | 派发前 `exec.LookPath` 探测 | 失败错误 + 语音引导；UpdateProbe(false) |
| initialize 失败/版本不符 | initialize 响应校验 protocolVersion | 任务 failed，错误明确写"协议版本不兼容" |
| 进程崩溃 | stdout EOF / Wait() 返回 | 挂起请求全部报错返回；任务 failed（K2）；进程从池中移除 |
| prompt 超时 | 单任务 30 分钟硬上限（ctx） | Cancel + kill 进程组，任务 failed（timeout） |
| 审批超时 | awaiting_approval 5 分钟计时器 | ACP cancelled 响应 + 任务 cancelled + 语音告知 |
| 服务重启 | 启动时 ListActive | 全部标记 failed（reason=service_restart）；残留子进程按 pid 文件 kill |
| 并发超限 | 同 agent_key 活跃任务 ≥ 2 | M1：直接返回排队提示（不入库）；M3：引入 `queued` 前置态（状态机扩展：queued →dispatched） |

## 11. 日志规范

流程日志 step_id（登记 `internal/event/flow_log.go` stepTitleRegistry + 同步 `52-flow-logger.design.md` §5.1）：

| step_id | 节点 | 级别 |
|---------|------|------|
| `agentbridge.task.dispatch` | K1 派发 | Info |
| `agentbridge.task.done` | K1 完成 | Info |
| `agentbridge.task.failed` | K2 失败 | Error |
| `agentbridge.task.cancelled` | K1 取消 | Info |
| `agentbridge.approval.request` | K1 审批请求 | Info |
| `agentbridge.approval.timeout` | K3 审批超时降级 | Warn |
| `agentbridge.process.spawn` / `agentbridge.process.exit` | K7 进程生命周期 | Info/Error |
| `agentbridge.probe.degraded` | K3 探测失败 | Warn |

进程日志：loggateway 结构化字段（`loggateway.Str("agent_key",...)` / `TaskID` 等）；`session/update` 事件经计数器限流（每任务 5s 窗口聚合 1 条），禁止每事件一条。

## 12. 安全设计

- **目录白名单**：cwd 必须来自 coding_projects 注册表，禁止用户直接传路径；启动前 `filepath.Clean` + 存在性校验
- **进程组隔离**：Windows `CREATE_NEW_PROCESS_GROUP`，取消时 taskkill /T 整组终止
- **审批默认开启**：M2 起所有 permission 请求必须中继；M1 阶段通过 initialize capabilities 声明由 agent 自身策略控制（接受 agent 默认权限模式）
- **摘要脱敏**：结果摘要入库前截断 4000 字符；不入库任何 env_json 中的密钥值（仅管理界面可见，API 返回掩码）
- **审计**：派发/审批决策（允许/拒绝/超时）全部流程日志留痕

## 13. Wire 装配点

- `data.go`：`NewCodingAgentRepo` / `NewCodingProjectRepo` / `NewCodingTaskRepo` 绑定
- `service.go`：`NewAgentBridgeService`（注入 repo + ACP client factory + event bus + FlowLogWriter）
- `tools` registry：`codingbridge.NewToolSet(svc)` 注册
- 启动钩子：`RecoverActiveTasks`（ListActive → 标记 failed + 清理 pid）

---

## 子模块：ACP 默认启动 adapter（E9 / M2-5）

> 日期：2026-08-22。仍是 **同一套 ACP stdio**，不是第二协议。管理页（M3 `AgentBridgePage`）未做。

`internal/biz/agentbridge/launch.go`：

| `agent_key`（及别名） | command | args |
|----------------------|---------|------|
| `codebuddy`（`code_buddy` / `tencent_codebuddy`） | `codebuddy` | `--acp` |
| `claude_code`（`claude` / `claude-code` / `claudecode`） | `claude-code-acp` | （空） |
| `codex`（`openai_codex` / `codex_acp`） | `npx` | `-y` `@zed-industries/codex-acp` |

`UpsertAgent` 在 command 为空且 key 已知时调用 `ApplyDefaultLaunch`；显式 argv 不覆盖。未知 key 仍要求 command。

端到端冒烟：`TestClientEndToEndWithFakeAgent`（`internal/agentbridge/acp/client_test.go`）已覆盖 initialize → session → prompt → done。真实 `codebuddy --acp` 本机二进制冒烟仍是人工项（M1-14）。
