# 会话端到端测试报告（chat-e2e-20260823）

> 执行依据：`00-master-plan.md`（已评审）。环境：Docker `aranea-admin` HTTP 8810 + quasar dev 9301，模型 deepseek-v4-flash。
> 证据目录：`f:\myproject\test\chat-e2e-20260823\`（响应体 / WS 事件流 jsonl / 运行日志 / 截图全部落盘）。
> 结论：**24 项 PASS / 1 项 FAIL（BASIC-05 自动标题）/ 2 项记录问题（并发确认竞态、pin 空 Content-Type 容错）**。

## 一、总览

| 阶段 | 用例 | 结果 | 关键证据 |
|------|------|------|---------|
| P0 环境 | ENV-01/02/03 | PASS | `env02-login.json`；healthz 200 |
| P1 基础链路 | BASIC-01~04 | PASS | `basic04b-ws.jsonl`（19 streaming + turn/task completed） |
| P1 | **BASIC-05 自动标题** | **FAIL** | 根因已定位，见问题清单 BUG-01 |
| P2 任务能力 | TASK-A1/A2/A3 | PASS | `task-a1/a2/a3.json`；JSON 输出可解析 |
| P2 | TASK-B 子代理委派 | PASS（首轮触发竞态，二轮串行审批通过） | `taskb2-run.log`：3 审批 → running → completed |
| P2 | TASK-C Team 编排 | PASS | `taskc-ws.jsonl`：2249 streaming / 55 step / 3 专家接力交付报告 |
| P3 控制面 | CTRL-01/02/03 | PASS | `p3-run.log`：stop→cancelled；enqueue 自动接续 |
| P4 会话管理 | MGMT-01/03/04 | PASS | `p4-run.log`；delete 200 + get 404 |
| P4 | MGMT-02 | PASS（补验后） | pin/unpin 需带 Content-Type，见 BUG-03 |
| P5 UI | UI-01/02 | PASS | `ui-final.png`：发送+流式回复渲染；重登后历史完整恢复 |

## 二、关键链路细节

### BASIC-04 WS 流式（`basic04b-ws.jsonl.summary.json`）
事件序列完整：`task.created → turn.started → step.created/streaming×19/completed → turn.completed → task.completed`，另含 L2 记忆召回 + 知识库 chunk 命中 notice。回复"巴黎三句话"正确。
> 探针首跑 90s 无事件为**测试工具问题**：v2 协议 `kind` 是顶层字段，探针按旧路径解析失败；修探针后通过，非产品缺陷。

### TASK-B（`taskb2-run.log`）
Spirit 收到"分三步、每步一个子代理"任务后发起 3 个 `subagents_spawn` 确认。首轮**并发审批 3 个 confirm 活动**时 1 个 accepted、1 个 accepted:false、1 个报 "activity is not in tool_blocked state (current: completed)"——CAS 竞态（BUG-02）。改串行自动审批后 3 个全部通过，running 约 5 分钟后 completed。

### TASK-C（`taskc-ws.jsonl.summary.json` + `ui02-messages.json`）
完整链路：plan_and_execute 路由失败（名册无「研究/调研」类专家）→ 精灵**自主降级** tool_search → tool_load `subagents_spawn` → 2 专家并行调研 + 1 专家汇总 → 交付完整《HTTP/2 vs HTTP/3 对比报告》（含双对比表，事实准确）。
- 第 1 个 spawn 确认 5 分钟超时未响应，模型识别后重新发起——超时容错行为符合设计。
- `plan_and_execute` 路由失败与 2026-08-23 组织架构重构后名册构成有关，降级路径有效，记为观察项不判 FAIL。

### P3 控制面（`p3-run.log`）
- CTRL-01：`stop` → `{"stopped":true}`；run-status `running→cancelled`；session `interrupted / user_cancelled`。
- CTRL-02：生成中 enqueue → `accepted+queued`，pending 可查；首轮完成后自动执行，最终 completed，messages=5 顺序正确。
- CTRL-03：running 态字段齐全（runId/agentName/eventCount=319）。

### P5 UI（`ui-final.png`）
登录 → 聊天页 → TASK-C 历史渲染（报告/子代理卡片「完成 18:40/18:45」齐全）→ Enter 发送"你好，请用一句话介绍你自己" → 5 秒后回复渲染（"你好！我是你的精灵管家…"）→ 输入框清空、会话列表时间与上下文比例（14%）同步刷新。浏览器会话丢失重登后历史经 REST 完整恢复（覆盖 UI-02）。

## 三、问题清单

| 编号 | 级别 | 问题 | 根因 / 证据 | 建议处置 |
|------|------|------|------------|---------|
| BUG-01 | **P2** | **自动标题不生成**：WS/HTTP 首轮后 session.title 保持默认值 | [title.go](file:///f:/myproject/aranea-agents/internal/biz/session/title.go) 的 `maybeAutoTitleFromUserMessage` 挂在旧 `AppendChatTurn/AppendChatMessage` 路径；原生 WS 聊天走 ActivityProjector 持久化，不经过这两个方法 → 钩子永不触发 | 在 ActivityProjector 落库路径（或 turn.completed 后）补自动标题触发点。**待你决策后修复** |
| BUG-02 | **P2** | **并发确认竞态**：同时 confirm 多个 tool_blocked 活动，部分被 CAS 顶成 cancelled/completed，报 "not in tool_blocked state" | `taskb2-run.log` 第 3-5 行（并发 3 审批只有 1 个真正 accepted）；串行审批无此问题 | confirm 处理按 turn 串行化，或冲突时返回可重试语义。**待你决策后修复** |
| BUG-03 | P3 | pin/unpin 无 Content-Type 时报 `400 CODEC "unregister Content-Type: "`，报错文案误导 | `p4-run.log` 第 7/9 行；带 `Content-Type: application/json` + `{}` 后 pinnedAt 正常设置/清除（补验通过） | 动作型端点容忍空 body，或返回明确提示。低优先 |
| OBS-04 | P3 | WS 实际挂在 HTTP 端口 8810 上，8812 拒绝连接；与方案/PORT-PLAN 记载不一致 | 执行中改连 8810 才握手成功 | 与 8800 vs 8810 权威端口问题一并裁定后同步文档 |
| OBS-05 | P3 | UI 合成 `.click()` 点发送按钮不生效（输入框值未清），Enter 键正常；真实鼠标点击未复现 | `ui-send2.js` vs `ui-send3.js` 对比 | 疑发送按钮需完整 pointer 事件序列；真实用户操作大概率无感，归档观察 |
| OBS-06 | P3 | 方案中账号记录为 dev/dev，实际密码 dev123456 | `body-login.json` | 文档笔误，已在此更正 |

## 四、耗时参考

| 场景 | 耗时 |
|------|------|
| 单轮问答（首轮冷） | 7.3s |
| 多轮上下文第 2/3 轮 | 3.4s / 1.9s |
| 工具调用轮 | 3.5s |
| JSON 结构化输出轮 | 5.5s |
| TASK-B 三子代理（含确认等待） | ~5min |
| TASK-C 三专家团队报告 | 7m34s |
| UI 发送→回复渲染 | ~5s |

## 五、结论

会话模块**能完成指定任务**：基础问答、多轮上下文、工具调用、结构化输出、子代理委派、三专家团队编排、停止/排队/状态控制、会话管理、UI 流式渲染全链路验证通过。唯一功能缺陷是 **BUG-01 自动标题不触发**（根因明确、修复点明确）；**BUG-02 并发确认竞态**影响自动化批量审批场景，真实 UI 串行点击不触发。两项 P2 修复方案均已整理，待你决策后实施。

## 六、P2 修复验证（2026-08-23 22:55–23:00 复测，已闭环）

按 [01-fix-plan-p2-bugs.md](file:///f:/myproject/aranea-agents/docs/testing/chat-e2e-20260823/01-fix-plan-p2-bugs.md) 完成修复 + 集成回归，两项均 PASS。

### BUG-01 自动标题 — PASS
- 修复要点：`AutoTitleFromUserMessage` 导出 + 新增 `task.created` 订阅器（[session_auto_title_subscriber.go](file:///f:/myproject/aranea-agents/internal/service/session_auto_title_subscriber.go)）+ wire 装配启动。
- 单测：4 个 subscriber 测试 + title.go 原有测试，全部 `go test -race` 通过。
- 集成（`verify-basic05.log`）：
  - HTTP 变体：4.4s 后 title => **HTTP协议三大特点简明总结**（changed=True）
  - WS 变体：turn running 期间 title => **TCP拥塞控制算法及其适用场景**（changed_while_turn_running=True）

### BUG-02 并发确认竞态 — PASS
- 修复要点：awaitChans 注册表双 key（sessionID+toolCallID）+ `confirmToolGate` 先投递后落库 + 拒发返 409 + 工具级 channel 寻址。
- **补一个补丁（修复中发现的二级缺陷）**：原 `EmitConfirmRequest` 未把 `args.ToolCallID` 写入 confirm step，导致 3 个并发 confirm 全部落到 session-level channel，第 1 个赢、其他 409。补三处：
  - [activity.go](file:///f:/myproject/aranea-agents/internal/biz/activity.go)：`ActivityConfirmParams` 加 `ToolCallID` 字段
  - [tool_confirmation.go](file:///f:/myproject/aranea-agents/internal/agent/tool_confirmation.go)：调用 `EmitConfirmRequest` 时传 `args.ToolCallID`
  - [projector.go](file:///f:/myproject/aranea-agents/internal/agent/v2/projector.go)：`EmitConfirmRequest` 写入 `step.ToolCallID = params.ToolCallID`
- 单测：9 个新测试（chat_usecase_test / chat_submit_await_reply_test / session_auto_title_subscriber_test），全部 `go test -race` 通过。
- 集成（`verify-taskb-parallel.log`）：
  - **3 个并行 confirm 全部 HTTP 200 accepted=true**（原事故：1 个 200 / 1 个 409 / 1 个 400 desync）
  - 后续 2 个串行 confirm 也都 200；final_status=completed；approvals=5；desync_count=0

### p3 控制面回归 — PASS（无回归）
`p3-run.log`：CTRL-01 stop→cancelled；CTRL-02 enqueue→completed messages=5；CTRL-03 run-status running/cancelled 字段齐全。修复未引入控制面回退。

### 测试过程踩坑（留档）
- 启用 `subagents_spawn` 触发并行 confirm：原事故复现依赖该工具，但 2026-08-23 组织重构后 `enabled=false`，LLM 改走 team orchestration 不再触发 confirm gate → 验证前临时 `UPDATE tools SET enabled=true`，验证后已恢复 `enabled=false`。
- 测试 stub race：`recordingAutoTitler.err` 字段在订阅器 goroutine 读、主测试写的 race，加 `setErr` 锁保护后通过。
- 容器中 pre-existing race：`TestPlanExecutor_FailedStepBlocksDownstream` / `TestPlanExecutor_SequentialDAG` 在 `plan_executor.go` DAG dispatch 有 pre-existing race（与本修复无关，未触动）。
