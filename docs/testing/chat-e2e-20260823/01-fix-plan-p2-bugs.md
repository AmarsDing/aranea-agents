# P2 问题修复方案（chat-e2e-20260823 跟进）

> 来源：`result.md` 问题清单 BUG-01 / BUG-02。本文档仅含方案，**经你确认后才动代码**。
> 合规声明：两处改动全部位于 aranea-agents 自有代码（`internal/biz` / `internal/service`），**不触碰 vendored trpc-agent-go 框架**（FW-R1~R3）。

---

## BUG-01 自动标题不生成

### 现象与证据
- BASIC-05：WS/HTTP 首轮对话后轮询 30s，`session.title` 保持默认值（证据：`basic05-session*.json`）。

### 根因（代码链路已核实）
1. 自动标题唯一触发点 `maybeAutoTitleFromUserMessage` 只挂在 [message_usecase.go](file:///f:/myproject/aranea-agents/internal/biz/session/message_usecase.go#L149-L181) 的 `AppendChatTurn` / `AppendChatMessage` 两个方法尾部。
2. 原生 v2 聊天（WS 与 HTTP unary）的消息持久化走 **ActivityProjector → task.created 事件 → event_router 落库**，全程不调用上述两个方法（它们背后的 writer 在 v2 下是 [NoopMessageWriter](file:///f:/myproject/aranea-agents/internal/biz/session/noop_message_writer.go#L17-L24)，仅存留给 team runner / compressor 等旧路径）。
3. 因此钩子对主聊天链路**永不触发**。

### 修复方案（推荐：方案 A）

**方案 A：新增 `task.created` 事件订阅器，在事件层触发自动标题**

- `ActivityProjector.OnTurnStart` 对**根 turn**（`TeamStageID == ""`，即用户真实输入）必发 `task.created`，事件携带 `Task.SessionID + Task.UserMessage`（[projector.go](file:///f:/myproject/aranea-agents/internal/agent/v2/projector.go#L285-L291)、[task.go](file:///f:/myproject/aranea-agents/internal/biz/task.go#L619-L631)）。team member 子 turn 不发此事件 —— 天然保证「只给根会话起名、不给子代理会话起名」。
- 新组件订阅事件总线（`Bus.Subscribe`，voice 已有同款生产用法 [session.go](file:///f:/myproject/aranea-agents/internal/voice/session.go#L844)），滤出 `*biz.TaskCreatedEvent` 且 `UserMessage != ""`，复用现有 `maybeAutoTitleFromUserMessage` 逻辑（snippet 立即改名 + 异步 LLM 生成，15s 超时）。
- 事件触发时机与旧钩子语义一致（收到用户消息即触发，不等回复）。

**改动点清单**

| # | 文件 | 改动 |
|---|------|------|
| 1 | `internal/biz/session/message_usecase.go` | 把 `maybeAutoTitleFromUserMessage` 导出为 `AutoTitleFromUserMessage(ctx, sessionID, content)`（私有方法保留为薄封装，旧两处钩子行为不变） |
| 2 | `internal/service/session_auto_title_subscriber.go`（新增，约 80 行） | 启动时 `Subscribe` 一次，goroutine 循环消费：类型断言 `*biz.TaskCreatedEvent` → 调 `AutoTitleFromUserMessage`；错误仅 Warn 不落失败（标题是锦上添花，绝不能影响主链路）；随 appCtx 退出 |
| 3 | `cmd/admin/wire.go`（+ wire_gen.go 重新生成） | 装配订阅器并在服务启动时拉起 goroutine |
| 4 | 单测 | 订阅器：task.created 触发 / 非 task 事件忽略 / UserMessage 空忽略 / usecase 报错不 panic；usecase 导出方法沿用既有 title 测试模式 |

**边界与兼容**
- **幂等**：[title.go](file:///f:/myproject/aranea-agents/internal/biz/session/title.go#L46-L60) `shouldAutoNameSession` 门控——仅默认标题（空/untitled/新会话等）才动作；用户手动改过名的会话零副作用（一次 GetSessionByID 读）。
- **双触发分析**：team runner 等旧路径走 `AppendChatMessage` 钩子但**不发** task.created（member turn）；根会话走 task.created 但不再走 Append*。两路径互补不重叠；极端重叠时第二次读到非默认标题即跳过，最坏情况是 snippet 同值重写一次，无害。
- **覆盖面**：WS、HTTP unary、UI、渠道入口（飞书等）——只要走 ActivityProjector 全部自动覆盖，无需逐入口改造。
- 性能：每条用户消息 +1 次会话读；仅首条消息触发写 + 异步 LLM。

**为何不选另两个方案**
- 方案 B（在 `OnTurnStart` 内联调用）：侵入 per-turn 热路径，agent/v2 层反向依赖 biz/session 的标题服务，分层变脏；同步 DB 写增加 turn 启动延迟。
- 方案 C（在 event_router 持久化 task 时触发）：同样跨层依赖，且持久化有 deadletter 重放路径，重放会重复触发，需额外去重机制。

**验证方案**
1. `go test ./internal/biz/session/... ./internal/service/...` 新增单测全绿。
2. Docker 重建后重跑 BASIC-05：新建会话发消息 → 轮询 title ≤30s → 先变为 snippet（秒级）→ 后变为 LLM 标题（≤15s）。
3. 回归：TASK-C 类 team 会话，子代理 member 会话标题保持默认不被改；手动改名后再发消息标题不被覆盖。

---

## BUG-02 并行工具确认的并发竞态

### 现象与证据（`taskb2-run.log` 第 3-5 行，三个请求同秒发出）
| 请求 | 结果 | 异常点 |
|------|------|--------|
| confirm s7 | `accepted:true, status:completed` | 正常 |
| confirm s8 | `accepted:false, status:completed` | **DB 已置 completed，但批准 token 未送达门禁** |
| confirm s9 | `400 activity is not in tool_blocked state (current: completed)` | 本请求未写库，step 却已是 completed |

### 根因链（四环，全部代码核实）

1. **await channel 是会话级单槽，并行确认互相覆盖**：[chat_usecase.go](file:///f:/myproject/aranea-agents/internal/biz/chat_usecase.go#L361-L368) `awaitChans[sessionID]` 每个会话仅一格，`RegisterAwaitChannel` 直接**关闭旧 channel 并覆盖**。3 个并行 `subagents_spawn` 确认各自建 channel 注册 → 只有最后一个在册，其余等待方悬空。
2. **投递不携带 ToolCallID，无法寻址到具体确认**：`AwaitReplyMsg{RunID, Reply}`（[chat_orchestrator_turn_dispatch.go](file:///f:/myproject/aranea-agents/internal/service/chat_orchestrator_turn_dispatch.go#L184-L194)）仅按 sessionID 找 channel —— 批准可能送进**另一个工具调用**的 channel。而 ToolCallID 在门禁上下文里现成可用（[chat_orch_await.go](file:///f:/myproject/aranea-agents/internal/service/chat_orch_await.go#L251-L256) 已读它写 awaitMeta），confirm step 也持久化了 `ToolCallID`（[step.go](file:///f:/myproject/aranea-agents/internal/biz/step.go#L24)）。
3. **先落库后投递，失败无回滚**：[chat_confirm.go](file:///f:/myproject/aranea-agents/internal/service/chat_confirm.go#L216-L264) `confirmToolGate` 先 `UpdateStep(completed)` 再 `submitAwaitReply`；投递被拒（channel 满/已被覆盖）时状态已提交 → s8 的 `accepted:false + completed` 脱钩态。
4. s9 的 400：其 channel 已被后来者覆盖、等待方被运行侧推进/终结后 step 被置 completed，本请求读到的即非 blocked 态。

### 修复方案（B + C 组合治本，A 作防御）

**方案 B：await channel 按 tool_call_id 寻址（治本）**
- 注册表 key 由 `sessionID` 改为 `sessionID + "\x00" + toolCallID`；`AwaitReplyMsg` 增加 `ToolCallID` 字段。
- 注册侧（`MakeAwaitReplyFunc`）：toolCtx 中 `ToolConfirmRequestFromContext` 取 ToolCallID 作 key（现成逻辑，见上）。
- 投递侧（`confirmToolGate`）：用 `step.ToolCallID` 精确投递。
- **兼容回退**：`await_user_reply`、clarification 等无 ToolCallID 的老路径仍注册/查找 session 级单槽（它们从不并行，语义不变）；`TrySend` 先查精确 key，未命中回退 session 槽。

**方案 C：投递成功才落库（治脱钩）**
- `confirmToolGate` 调整顺序：**先 `submitAwaitReply`，成功后才 `UpdateStep(completed)`**；投递被拒 → 返回 `409 CONFLICT`（可重试语义），step 保持 `tool_blocked`，前端可提示「另一个确认正在处理，请稍候」。
- 反向窗口分析：token 已送达但 UpdateStep 失败的概率远低于 channel 拒绝，且失败时门禁恢复执行、用户重试 confirm 会因状态已非 blocked 得到明确 400，不产生幽灵 completed。
- `IsExternalCodingConfirm`（ACP bridge）与 playbook confirm 分支**不调整顺序**（它们不经 await channel，无此问题），仅主干工具确认路径改动。

**方案 A：ConfirmActivity per-session 串行锁（~~防御层~~，经综合评估后砍掉）**
- 原设想：`sync.Map[sessionID]*sync.Mutex` 串行化同会话 confirm。
- **砍掉理由**：B 落地后并行 confirm 各自寻址，不再冲突；双点击/重试场景下，C 的顺序保证第二次 `TrySend` 必然失败 → 409 且不落库，状态已正确。锁只改变错误呈现方式（400 vs 409），不提升正确性，还引入锁实例生命周期管理（sync.Map 不清理会泄漏会话级 mutex）。过度设计，不采纳。

**改动点清单（最终）**

| # | 文件 | 改动 |
|---|------|------|
| 1 | `internal/biz/chat_usecase.go` | awaitChans 双 key（session+toolCallID）注册/加载/发送/删除；保留 session 级槽位兼容 |
| 2 | `internal/biz/`（AwaitReplyMsg 定义处） | 增加 `ToolCallID` 字段 |
| 3 | `internal/service/chat_orch_await.go` | `MakeAwaitReplyFunc` 用 ToolCallID 注册精确槽位 |
| 4 | `internal/service/chat_confirm.go` | `confirmToolGate` 主干：先投递（带 ToolCallID）后落库；拒绝 → 409 |
| 5 | 单测 | 双 key 注册/寻址/回退；并行 3 confirm 全 accepted；投递失败状态保持 tool_blocked |

**不改动**：playbook confirm、coding-bridge confirm、clarification、`await_user_reply` 自由文本路径的行为语义。

**验证方案**
1. 新增单测全绿；`go build ./...`。
2. Docker 重建后跑 **TASK-B 并发变体**（p2-taskb2.ps1 改并发审批）：3 个 confirm 同秒发出 → 断言**全部** `accepted:true` 且 3 个子代理全部 queued（复现原场景，验证竞态消除）。
3. 回归：单 confirm 批准/拒绝/5min 超时；clarification 问答；voice 确认路径（voice_confirm_test 既有用例全绿）。

---

## 执行建议

| 项 | 建议 |
|----|------|
| 顺序 | 先 BUG-01（小、独立、无耦合），后 BUG-02 |
| 分支 | 各一个 commit，便于独立回滚 |
| 验证环境 | Docker `dev-up.ps1` 重建后实跑（后端一律 Docker 运行规约） |
| 回归基线 | 重跑 `p1-basic.ps1`（含 BASIC-05 新断言）+ TASK-B 并发变体 + `p3-ctrl.ps1` |

## 决策结论（综合评估，2026-08-23）

| 决策点 | 结论 | 理由 |
|--------|------|------|
| BUG-01 方案选型 | **采用方案 A（task.created 订阅器）** | 事件驱动零侵入热路径；根 turn 天然过滤（team 子 turn 不发 task.created）；WS/HTTP/渠道全入口一处覆盖；`shouldAutoNameSession` 幂等门控 |
| BUG-02 方案组合 | **B + C，砍掉 A** | B 按 tool_call_id 寻址后并行冲突面已消除；C 保证投递失败不落库，双点击/重试得到明确 409；串行锁只改错误呈现不提升正确性，且有锁泄漏负担，属过度设计 |
| 409 前端配套 | **本轮仅后端返回，前端另立小项** | 竞态 409 是低频兜底（真实 UI 串行点击基本不触发），现有 confirm 失败的通用提示已够用；前端「按钮防重复点击 + 409 专用文案」单独排期，不阻塞本次修复 |

**执行顺序**：BUG-01（小、独立）→ BUG-02，各一个 commit；Docker 重建后按上表验证方案回归。
