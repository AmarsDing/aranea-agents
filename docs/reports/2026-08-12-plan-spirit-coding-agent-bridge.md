# 精灵助手控制专业编程 IDE/CLI —— 调研与方案设计

> 日期：2026-08-12 ｜ 类型：调研 + 方案 ｜ 状态：设计已获用户批准，待实施计划

---

## 1. 背景与目标

通过精灵助手（Spirit Agent）以语音/聊天方式控制专业编程工具——**Claude Code、Codex、CodeBuddy（WorkBuddy 企业版）、Trae**——实现：

- **任务派发**：自然语言指令 → 外部编程 agent 在指定项目目录执行任务
- **结果回收**：任务完成后语音播报 + 结果卡片
- **双向中继（核心诉求）**：外部 agent 运行中产生的工具执行确认、任务澄清等消息，由 Aranea 截获并通知用户，用户操作后回传给外部 agent 继续执行

### 已确认的需求边界

| 决策点 | 结论 |
|--------|------|
| 控制形态 | 任务派发 + 结果回收（后台 headless，非 GUI 可视化操控） |
| 审批策略 | 外部工具的中途确认/澄清消息必须中继到 Aranea，用户语音或卡片操作后回传 |
| 触发入口 | 语音 + 聊天都要 |
| Trae 角色 | 仅"拉起/打开项目 + 查看任务完成结果"，不看 diff 和代码细节；不驱动 SOLO |
| 结果呈现 | 只关心任务完成结果，不需要 diff/代码级展示 |

---

## 2. 调研结论：四个目标的可编程控制面

| 目标 | Headless CLI | 标准协议 | 常驻服务 | 官方 SDK |
|------|-------------|---------|---------|---------|
| **Claude Code** | `claude -p` + `--output-format stream-json` + `--allowedTools` + `--resume` | ACP（Zed 社区 adapter：`claude-code-acp`） | — | Agent SDK（TS/Python，子进程+stdio） |
| **Codex** | `codex exec --json`（JSONL 事件流） | ACP（Zed 社区 adapter：`codex-acp`） | `codex remote-control`（daemon+WebSocket）、App Server（双向 JSON-RPC） | Codex SDK（TS/Python） |
| **CodeBuddy** | `codebuddy -p` | **原生 ACP**：`codebuddy --acp`（另支持 streamable-http 传输） | `codebuddy daemon start --port`（可注册系统服务） | 有 |
| **Trae** | ❌ 无官方 headless | ❌ 无 ACP | ❌ | ❌ |

关键判断：

1. **三个 CLI Agent 的控制面高度趋同**：headless 模式 + JSON 事件流 + 会话恢复 + 权限模式 + MCP。
2. **ACP（Agent Client Protocol）正在成为业界标准**——Zed + JetBrains 推动，定位"agent 版 LSP"，JSON-RPC 2.0 over stdio NDJSON。协议原生内置 `session/request_permission`（审批请求）与 `session/update`（流式进度），**正是双向中继诉求的协议级答案**。当前协议版本统一为 v1。
3. **Trae 是异类**：无任何官方编程接口。可行路径仅 Electron CDP（脆弱）、VS Code 扩展桥接（间接）、MCP（方向相反）。一期不做任务派发。
4. **一次性 headless（`claude -p` / `codex exec`）无法中途交互**，不满足审批中继需求，只能走双向协议（ACP / App Server）。
5. **Go 无官方 ACP SDK**（官方仅 TypeScript；第三方有 Java）。协议本身简单，Go 实现客户端子集可控（数百行）。

---

## 3. 方案对比与选型

### 方案 A：ACP 统一桥接（✅ 已选定）

```
精灵助手(语音/聊天) → Aranea 后端编排 → ACP Bridge (Go 实现客户端)
    ├── stdio ──► codebuddy --acp              （原生支持）
    ├── stdio ──► claude-code-acp ──► claude    （Zed 社区 adapter）
    └── stdio ──► codex-acp ──► codex           （Zed 社区 adapter）
```

- 一套协议代码驱动三个工具；审批中继、流式进度、会话恢复均为协议内置
- 代价：Claude/Codex 依赖社区 adapter（Zed 官方维护，生态活跃）；Go 需自实现 ACP 客户端子集

### 方案 B：每工具原生接口直连（未选）

- Claude Code → `--permission-prompt-tool` MCP 审批回调 + stream-json；Codex → App Server JSON-RPC；CodeBuddy → daemon HTTP/ACP
- 无 adapter 依赖、能力最全，但三套协议三倍维护成本，审批模型各异难统一

### 方案 C：纯 headless 一次性派发（排除）

- 权限全靠启动参数预授权，**无法满足中途确认中继的核心诉求**

**Trae（所有方案相同）**：一期仅 `trae <path>` 拉起 + 结果卡片入口；SOLO 驱动属逆向 UI 脆弱路径，二期 PoC 再定。

---

## 4. 总体设计

### 4.1 架构分层

```
用户语音/聊天 ──► 精灵助手编排（spirit_team，复用现有入口）
                      │ intent：编程任务派发
                      ▼
              CodingDispatchTool（agent 层新工具，trpc tool.Tool）
                      │
                      ▼
              AgentBridgeService（service 层新增）
                ├── ACP Client Manager：spawn/复用/回收 CLI 子进程
                ├── 任务状态机（显式状态机文件，遵 AS-FSM-01）
                └── 审批中继器
                      │ stdio NDJSON (JSON-RPC 2.0)
        ┌─────────────┼────────────────┐
        ▼             ▼                ▼
  codebuddy --acp   claude-code-acp   codex-acp
```

### 4.2 核心组件

| 组件 | 位置 | 职责 |
|------|------|------|
| **ACP Go 客户端** | `internal/agentbridge/acp` | initialize / session/new / session/prompt / session/cancel / authenticate；处理 session/update 流与 session/request_permission；子进程 spawn、stdin/stdout pipe、崩溃检测 |
| **AgentBridgeService** | `internal/service/` | DispatchTask / CancelTask / GetTaskStatus；任务状态机（dispatched→running→awaiting_approval→done/failed/cancelled）；session/update 限流聚合为进度事件 |
| **审批中继** | BridgeService 内 | ACP permission → 构造与 ToolConfirmationRequest 同构事件 → event.Bus → WS → ConfirmCard；用户确认走 chat_confirm 同款回传路径 → 写回 ACP stdio；语音侧复用 clarify 播报 + 语音作答路由 |
| **CodingDispatchTool** | `internal/tools/` | 精灵助手三个工具：`dispatch_coding_task` / `check_coding_task` / `cancel_coding_task` |
| **Agent 注册表** | 扩展 RemoteAgent 模型 | protocol 枚举加 `acp_stdio`；字段：command/args/env/adapter 包名；项目目录注册表（"XX 项目" → 磁盘路径映射） |

### 4.3 复用的现有链路

| 现有机制 | 复用方式 |
|---------|---------|
| 工具确认门（`internal/agent/tool_confirm_gate.go` → ConfirmCard → `internal/service/chat_confirm.go` 回传恢复） | 审批中继的事件构造与回传路径同构复用 |
| clarify 语音播报 + 语音作答（StepCreatedEvent → TTS → SubmitClarification） | 审批请求的 TTS 播报与语音作答路由复用 |
| event.Bus → WS → 前端卡片 | 新事件类型注册接入（`internal/event/bus_v2.go`、`ws_v2_wire_convert.go`、前端卡片组件） |
| RemoteAgent 注册模型（`internal/biz/remote_agent*.go`） | 扩展 protocol 枚举与字段，复用存储与管理 |
| 精灵助手工具注册（`internal/tools/tool_register.go`） | 新工具注入精灵助手工具集 |

### 4.4 一次任务的完整数据流

1. 用户语音："让 Claude Code 给 aranea-agents 跑测试并修失败的" → ASR → intent 识别 → `dispatch_coding_task`
2. BridgeService 解析项目路径 → spawn/reuse `claude-code-acp` 进程 → initialize → session/new(cwd) → session/prompt（K1 流程日志）
3. session/update 流 → 限流聚合 → 进度事件（高频路径按红线用计数器/时间窗限流）
4. agent 请求执行 `go test` → session/request_permission → 中继 → ConfirmCard + TTS 播报
5. 用户语音/点击"允许" → 路由回传 → ACP 响应 → agent 继续
6. session/prompt 完成 → 结果摘要 TTS 播报 + 结果卡片（含"在 Trae 中打开"入口）

### 4.5 错误处理

| 场景 | 处理 |
|------|------|
| adapter 未安装 | dispatch 前探测（command -v / npm ls），语音引导安装（K3） |
| adapter 进程崩溃 / exit code 非 0 | 任务标记 failed + 语音告知（K2 流程日志 + Error 进程日志） |
| 审批超时 | 5 分钟无响应 → ACP 回 cancelled，任务挂起可恢复 |
| protocolVersion 协商失败 | initialize 阶段明确报错，不静默降级 |
| 并发上限 | 同 agent 并发会话数上限，超出排队 |

### 4.6 日志规范

- K1（流程入口/出口）、K2（错误路径）、K3（降级）、K7（子进程生命周期）必须覆盖
- 新增 step_id 登记 `internal/event/flow_log.go` stepTitleRegistry 并同步 `docs/development/52-flow-logger.design.md` §5.1
- session/update 高频事件必须限流，禁止每事件一条日志

---

## 5. 分期实施

| 期 | 内容 | 验证 |
|---|------|------|
| **M1** | ACP Go 客户端 + CodeBuddy 原生对接（最适首验）+ 派发/进度/结果回收（先白名单全自动，无审批中继） | fake ACP server 集成测试 + codebuddy 真实任务冒烟 |
| **M2** | 审批中继（确认门复用 + 语音播报作答）+ claude-code-acp / codex-acp adapter 接入 | 三工具真实任务的审批全流程 |
| **M3** | Trae 拉起联动 + 结果卡片入口 + Agent/项目管理界面 | 端到端 UI 验收 |

---

## 6. 风险与缓解

| 风险 | 等级 | 缓解 |
|------|------|------|
| claude-code-acp / codex-acp 为社区 adapter，可能滞后于 CLI 版本 | 中 | M1 先用 CodeBuddy 原生 ACP 验证全链路；adapter 版本探测 + protocolVersion 协商失败明确报错 |
| ACP 协议 v1 后续演进（unstable_ 前缀方法） | 低 | 客户端只实现 stable 子集；方法调用前查 capabilities |
| Go 无官方 ACP SDK | 低 | 协议为 JSON-RPC over NDJSON，自实现子集约数百行；接口隔离在 `internal/agentbridge/acp` 包 |
| 外部 agent 执行高危命令 | 中 | 审批中继默认开启；项目目录注册表白名单；超时会话自动取消 |
| Trae SOLO 无编程接口 | 已知 | 一期不做；二期 PoC 评估扩展桥接可行性 |
