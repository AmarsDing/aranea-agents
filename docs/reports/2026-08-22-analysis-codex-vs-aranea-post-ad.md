# Codex vs Aranea：A–D 落地后重估

> 日期：2026-08-22  
> 类型：analysis  
> 前序：[2026-08-22-analysis-codex-vs-aranea.md](./2026-08-22-analysis-codex-vs-aranea.md)（实施前评分）  
> Codex 快照：`F:\myproject\openai-codex` @ `4f39251a`

上午报告加权：Codex **8.6** / Aranea **7.3**（差 1.3）。  
A–D 代码落地后重估：Codex **8.6** / Aranea **7.8**（差 **0.8**）。  
E1/E2/E4/E5/E6 后再估：Codex **8.6** / Aranea **8.1**（差 **0.5**）。  
E7/E7b/E8/E9/E10 后再估：Codex **8.6** / Aranea **8.3**（差 **0.3**）。  
E3 + MCP 脏刷新后再估：Codex **8.6** / Aranea **8.4**（差 **0.2**）。  
平台分不变：Codex ~3.5 / Aranea ~8.5（不加权）。

读法：合同层、执行回路、增量 world state、OS 进程围栏、MCP 脏刷新、默认 ACP argv 与 §7 金丝雀已补上。剩下的硬差距在 **完整 OS token/Seatbelt 档**、真实编码桥二进制冒烟 / M3 管理页、MCP templates/subscribe。

---

## 1. 分项重估


| 维度 | Codex | 实施前 | 实施后 | 本轮改了什么 | 仍缺（代码） |
|------|-------|--------|--------|--------------|--------------|
| Harness | 9.0 | 7.0 | 7.8 | D1 ACP `OnPermission` → 确认卡；**E2** 语音；**E9** `DefaultACPLaunch` | 主循环仍是 ChatOrchestrator 分期；真实二进制冒烟 / M3 管理页未做 |
| Context | 8.5 | 7.0 | **8.4** | B1 `AGENTS.md`；B2 Handoff Card；A3 `total_lines`；**E6** `get_context_remaining`；**E7** world-state diff；**E7b** 中途 compact 回注 | 无 Codex 第三条「开新窗不摘要」 |
| Exec | 9.0 | 7.5 | **8.3** | D2 路径沙箱；**E1** 安全 shell 免确认；**E4** `duration_ms`；**E5** 禁 `save_file` 覆盖；**E3** Job Object / bwrap | 无 Seatbelt / 完整 FS+网络策略；`total_lines` 仍主要走截断路径 |
| Discovery | 8.5 | 7.0 | 8.0 | B3 BM25；B4 `tool_hints`；MCP **broker** | 直连 `mcp_tool_set` 并未默认 Deferred（和 Codex defer 不是同一条路） |
| Skill | 8.0 | 7.5 | 7.8 | B5 `$slug`；A4 `body_too_long` | 无仓库 `.agents/skills` FS 扫描 |
| MCP | 8.0 | 8.0 | **8.3** | D3 仅评估；**E8** list/read + 脏标记 mid-turn `tools/list` | Agent-as-server 不实现；客户端无 templates/subscribe |
| Prompt | 9.0 | 6.5 | **8.2** | A1/A2 烤进 instruction；**E5** hook 禁 `save_file` 覆盖 | 无 |
| Memory | 8.5 | 7.0 | 8.0 | C1–C4 summary / citation / sleep-time / L1 board；**E10** §7 金丝雀 | 无文件-only 记忆（有意） |

加权（15/15/15/10/10/10/10/15）：A–D 后 ≈ **7.8**；E1–E6 ≈ **8.1**；E7–E10 ≈ **8.3**；E3+热刷新后再估 ≈ **8.4**。

---

## 2. 深入逻辑：两边实际怎么走

### 2.1 主循环

**Codex**：用户输入、Exec/Patch 审批、压缩、MCP 刷新、挂起都以 Op 回到同一 thread 循环（见深挖报告 §2.3 与 `codex_thread.rs` / TUI `Op::ExecApproval`）。

**Aranea**：`ChatOrchestrator` 分期（规划门 → 澄清 → Runner → 确认门 → `AfterNativeTurn` 压缩）。D1 把外部 ACP permission 接到 `ConfirmActivity`，复用卡片，但编码桥等待在 `agentbridge_approval.go` 的 `OnPermission` select，chat 工具等待在 `tool_confirmation.go`。两条 await，不是一条 Op 队列。

**不要**为此改成 `spawn_agent` 主编排。可做的是审批语义继续复用，而不是再开第四套协议。

### 2.2 脚本 / `shell_exec`（不是全部拒绝）

`InferPermissionMode`（`permission_state.go`）：

- `coding` / `spirit` / `full`（空 profile 也是 coding）→ `needs_approval` → 沙箱 **不** 拦 `shell_exec`
- `read_only` / `research`（无写 allow）→ `read_only` → `workspace_sandbox` **整工具拒绝** `shell_exec`
- 可写会话里 `shell_exec` 默认 `RequiresConfirmation`，走确认门；「始终允许」已有 session grant

因此 `go test` 的路径是：**coding 专项下安全命令直接跑**（E1：`go test` / `go vet` / `git status|diff|log` / `rg` / `ls|dir` / linter 免第一张卡；`rm` / `git push` 等高危仍确认；未知命令保持确认）。不是禁脚本。和 Codex 仍差 on-failure 档与 OS token（E3）。

### 2.3 上下文

**Codex**：Base 稳定 + developer；`agents_md.rs` 链；`world_state` 增量；工具输出带 exit / 墙钟 / 行数；模型可 `get_context_remaining`。

**Aranea 已对齐**：

- `BuildSystemPrompt` 顺序：role → industry → `<working_contract>` → `<permission_state>` → internal_config（`prompt.go`）
- `loadProjectAgentsMD`：与 working_contract 同门；未信任根不读
- `compress.DefaultSystemPrompt` 以 Handoff Card 开头；`session/compressor.go` 级联 MemoryCompact → LLM（硬阈值才上 LLM）

**E6**：coding / spirit / full profile 挂 `get_context_remaining`（读本轮 `ContextBudget` + 256K 窗）。

**E7 / E7b**：动态 cue 仍是 user-role + `ToolName = aranea.dynamic_cue`（不进 DeepSeek system prefix）。同一 invocation 指纹未变则跳过；行集变化发 `<world_state_diff>`。紧急截断丢掉历史/尾 cue 后，把上一份 `<world_state>` 插回最后一条真实 user 消息之前。

**Shell 回执**：`hostexec.mapExecResult` 现有 `exit_code` + 前台 **`duration_ms`（E4）**；长任务仍有 `running_for_ms`。`total_lines` 仍主要在 decorator / size limiter 截断时出现。

### 2.4 沙箱

**Codex**：`windows-sandbox-rs` / Seatbelt / bwrap + `FileSystemSandboxPolicy` + `NetworkSandboxPolicy`。

**Aranea**：`workspace_sandbox.go` priority 8（先于确认门 10）。只读拦写工具；`path`/`file`/`cwd` 做 `containPathUnderRoot`。`command` 正文不检查，所以可写模式下 `rm -rf` 仍靠确认门 + command safety。E3：`hostexec` 默认打开进程围栏（Windows Job Object；Linux 有 `bwrap` 时 `--die-with-parent`）。不是 Codex 那套完整 FS/网络策略。

### 2.5 记忆

读路径已接近 Codex 合同：`<memory_summary>` ≤800 token；L4 召回不再虚增 `use_count`；sleep-time 高信号门 + 隔离 consolidator；无 L1 时注入上一会话 task board。形态仍是 DB 分层（正确），不是 `~/.codex/memories` 文件。

### 2.6 Codex 源码补记（submission_loop）

二次核对 `codex-rs/core` 后，以下几条比上午报告更硬，评分不改，只收紧「还能涨」的优先级：

| 点 | Codex 锚点 | 对 Aranea 的含义 |
|----|------------|------------------|
| 统一入口 | `session/handlers.rs` `submission_loop`；`protocol.rs` `Op`（`TurnInput` / `ExecApproval` / `PatchApproval` / `RequestPermissionsResponse` / `Compact` / `RefreshMcpServers` / `SuspendTurnAndShutdown` / `RecoverTurn`） | 所有突变进同一队列。Aranea 不要抄成 spawn 循环，但编码 turn **没有** `RecoverTurn` / worker 交接 |
| 采样环 | `session/turn.rs` `run_turn`：pre-compact → WorldState → 采样 → 工具 → `needs_follow_up` → 可选 mid-turn compact | 压缩有两种回注：`DoNotInject`（回合前）vs `BeforeLastUserMessage`（回合中）。Aranea `AfterNativeTurn` 只覆盖回合后 |
| HITL 范围 | `request_permissions.rs`：`Turn` / `Session`（**无全局**）；`ApprovedForSession` 按命令 hash / patch 路径缓存；可走 Guardian | D1 `allow_always` 仅本编码任务。内部 `shell_exec` 有 session grant，缺「失败再问」和 execpolicy 补丁落盘 |
| 权限是工具 | `RequestPermissionsHandler`：模型可申请扩权，结果 merge 进 `PermissionProfile` | Aranea 权限是 profile 推断 + 确认门，模型不能在回合内申请「加写某个根」 |
| 回执三件套 | `format_exec_output_for_model`：`Exit code` / `Wall time` / `Total output lines` | A3 有 `total_lines`；shell 仍常缺 exit / duration |
| 上下文增量 | `WorldStateSection::render_diff`：权限 / AGENTS.md / skill catalog 只发相对上一步的 diff | `permission_state` 烤在 instruction 里利于 cache；动态 cue 已做行级 diff（E7） |
| MCP | `refresh_mcp_if_dirty` 可在 step capture 热刷新；`mcp-server` 把 Codex 自己暴露出去 | D3 已否决本期做 Agent-as-server；热刷新仍值得单独评估 |
| 挂起 | `turn_suspension.rs`：flush → 取消 RegularTask **不发终态** → 关 writer；`RecoverTurn` 同 `turn_id` 空输入续采。**有意丢掉**进程内 pending | 澄清挂起存在；编码未完成根 turn 的 durable handoff 仍无 |

`spawn_agent` 在 Codex 是**另开 child thread + 自己的 submission_loop**，父线程只收 mailbox。这仍不是 Aranea 编排该抄的东西。

---

## 3. 下一波提升（只做可吸收的）

| ID | 优先级 | 项 | 状态 |
|----|--------|----|------|
| E1 | P0 | coding `shell_exec` 审批降噪（安全命令免卡；高危/未知仍确认） | ✅ 2026-08-22 |
| E2 | P0 | 语音消费 `coding_task_approval` + spirit store 通知 | ✅ 2026-08-22 |
| E3 | P1 | Windows Job Object；Linux bwrap `--die-with-parent` | ✅ 2026-08-22（非完整 Seatbelt/文件系统策略） |
| E4 | P1 | 前台 shell 回执补 `duration_ms`（`exit_code` 已有） | ✅ 2026-08-22 |
| E5 | P1 | 编码专项 hook：禁止用 `save_file` 整文件覆盖替代 `diff_edit` | ✅ 2026-08-22 |
| E6 | P1 | `get_context_remaining`（coding / spirit / full） | ✅ 2026-08-22 |
| E7 | P2 | 动态 cue 改增量（对标 `WorldStateSection::render_diff`） | ✅ 2026-08-22 |
| E7b | P2 | 超长 turn 中途压缩回注合同（摘要必须落在模型期望的位置） | ✅ 2026-08-22 |
| E8 | P2 | MCP Resource 三件套；评估 mid-turn catalog refresh（不做 Agent-as-server） | ✅ list/read 已有；脏标记热刷新已落地；templates/subscribe 仍不实现 |
| E9 | P2 | 76 adapter + CodeBuddy 冒烟 + M3 | 🟡 默认 argv ✅；fake ACP e2e ✅；真实二进制 / M3 页 📋 |
| E10 | P3 | §7 金丝雀（token、隔日偏好、citation） | ✅ 2026-08-22 |

禁止项不变：spawn_agent 主编排、文件-only 记忆、丢掉知识库、Code Mode 优先、去掉 SSE、全员共用编码 prompt。

---

## 4. 结论

A–D 把「告诉模型你是谁、能干什么、手册在哪、记忆怎么读」补到可用。  
E1/E2/E4/E5/E6 把执行回路的噪音、回执、编辑纪律和上下文自助开窗补上。  
E7–E10 把增量 cue、中途 compact 回注、MCP Resource 边界、默认 ACP argv 和 §7 金丝雀补上。  
E3 补了进程围栏（Job Object / bwrap），MCP 脏标记会在下一轮刷新直连 catalog。  
再追 Codex，优先完整 FS/网络沙箱档与真实编码桥冒烟，不要继续加长 system prompt。
