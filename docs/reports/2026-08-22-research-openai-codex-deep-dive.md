# OpenAI Codex 开源仓库深挖（业务循环 / Context / 工具 / Skill / MCP / Prompt / 记忆）

> 日期：2026-08-22  
> 类型：research  
> 源码：`F:\myproject\openai-codex`（`git clone --depth 1 https://github.com/openai/codex.git`）  
> 快照：`4f39251a010a8bd7d692d25fb33832ff06f1635a`（2026-08-22 00:42 UTC，`Add unfinished root turn suspension`）  
> 许可：Apache-2.0  
> 对照报告：[2026-08-22-analysis-codex-vs-aranea.md](./2026-08-22-analysis-codex-vs-aranea.md)

---

## 0. 结论先看

Codex CLI 不是「又一个 chat wrapper」。它是一套 **本机编程 Agent Harness**：Rust 核心（`codex-rs/core`）负责 turn / 审批 / sandbox / 工具路由，TUI / `codex exec` / App Server / MCP Server / TS·Python SDK 都是同一业务核的不同入口。

和 Aranea 最该对齐的不是 UI，而是这七个机制：

1. **Submission 循环**：用户输入、审批回传、压缩、挂起、子 Agent 通信都是同一条 `Op` 总线。
2. **分层 Context + World State**：base / developer / `AGENTS.md` / memory_summary / 历史之外，每 step 还有 **world state diff**（权限、环境、skill catalog、工具 namespace）。压缩默认是交接摘要，也有「直接开新窗、不做 LLM 摘要」的 token-budget 路径。
3. **工具暴露分级**：Direct / Deferred / CodeMode / Hidden；MCP 与内置工具可进 `tool_search`，schema 不一次全塞。**没有**一等 `read_file` / `grep`——仓库搜索靠 shell `rg`，TUI `@` 文件搜索不进模型工具面。
4. **Skill 三级披露 + 工具面**：catalog（name+description，有 token 帽）常驻 → `$skill` / `skills.read` 读全文 → references/scripts 按需。另有 `skills/list`。
5. **MCP 当一等配置**：stdio + streamable HTTP、OAuth、per-server allow/deny、per-tool 审批、资源工具、Codex 自身也可当 MCP server。
6. **Prompt 是产品**：人格、preamble、plan、验证时机、最终答案排版都写进 base instructions，而不是散落在 hook 里。
7. **记忆是文件工作区 + 两阶段巩固 + 工具面**：`memory_summary.md` 进 prompt；`MEMORY.md` / rollout / skill 按需；`memories.{list,read,search,add_ad_hoc_note}` 给模型；启动时 Phase1 抽 rollout、Phase2 派 consolidation 子 Agent。旧 SQLite memory 表已 drop（migration 0035）。

下文按源码证据展开。官方站点文档多数只是外链，**真相在仓库**。

---

## 1. 仓库与产品形态

### 1.1 目录

| 路径 | 角色 |
|------|------|
| `codex-rs/` | 主体。`core` 是业务核；`tui` / `exec` / `app-server` / `mcp-server` 是入口 |
| `codex-rs/protocol` | 事件、模型、`BASE_INSTRUCTIONS` |
| `codex-rs/prompts` | compact / review / permissions / goals / realtime 模板 |
| `codex-rs/skills` | Skill 解析、选择、`$skill` mention |
| `codex-rs/memories/{read,write}` | 记忆读注入 / 两阶段写管线 |
| `codex-rs/tools` | ToolSpec、`defer_loading`、`tool_search`、MCP tool 适配 |
| `codex-rs/config` | `config.toml` 分层 + MCP 类型 |
| `codex-rs/ext/{skills,mcp,memories,goal,...}` | 扩展点：廉价 skill 选择器、hosted MCP、记忆 prompt |
| `sdk/{typescript,python}` | 嵌入式 SDK |
| `docs/` | 贡献/安装/sandbox；产品文档指向 developers.openai.com |

语言以 **Rust** 为主（约 95%+），另有 npm 包装器 `codex-cli`、TS/Python SDK、Bazel 构建。

### 1.2 入口面

同一 `codex-core` 被这些面复用：

- 交互 TUI：`codex`
- 无头：`codex exec --json`（JSONL 事件流；Aranea 的 `pkg/trpc-agent-go/agent/codex` 已接这条）
- App Server：JSON-RPC，给 IDE / Desktop
- MCP Server：让别的客户端把 Codex 当工具
- Review / resume / fork / cloud apply

核心 crate 自评（`codex-rs/core/README.md`）：*「This crate implements the business logic for Codex. It is designed to be used by the various Codex UIs written in Rust.」*

### 1.3 安全底座（业务循环的前置条件）

`codex-core` 假定平台沙箱可用：

| 平台 | 机制 |
|------|------|
| macOS | `/usr/bin/sandbox-exec`（Seatbelt）；workspace-write 时 `.git` / `.codex` 只读 |
| Linux | bubblewrap（优先 PATH 上的 `bwrap`，否则捆绑二进制）；WSL2 可用，WSL1 拒绝 |
| Windows | 提升沙箱 + 受限 token；split filesystem policy 不支持则 fail-closed |

审批策略与沙箱策略是 **正交** 的：`never` / `on-request` / `unless-trusted` × `read-only` / `workspace-write` / `danger-full-access`。Prompt 模板在 `codex-rs/prompts/templates/permissions/`。

---

## 2. 业务逻辑：Turn / Thread / 审批 / 子 Agent

### 2.1 主循环是 `Op` 总线，不是「chat 函数」

`codex-rs/core/src/session/handlers.rs` 的 `submission_loop` 从 channel 收 `Submission`，按 `Op` 分发。这是整机状态机：

| Op | 含义 |
|----|------|
| `TurnInput` | 用户一轮输入（含 mode / reply oneshot） |
| `RecoverTurn` | 崩溃/中断后恢复 |
| `SuspendTurnAndShutdown` | 未完成根 turn 挂起后关机（本快照最新合入） |
| `Interrupt` | 打断当前 turn |
| `ExecApproval` / `PatchApproval` | shell / apply_patch 审批回传 |
| `UserInputAnswer` / `RequestPermissionsResponse` / `DynamicToolResponse` | 模型向用户要输入、要权限、动态工具回包 |
| `Compact` | 手动上下文压缩 |
| `RefreshMcpServers` / `ReloadUserConfig` | 热刷新 |
| `InterAgentCommunication` | 父子/兄弟 Agent 消息 |
| `RealtimeConversation*` | 语音实时通道 |
| `ThreadSettings` | 线程级模型/effort 变更 |
| `Shutdown` | 退出循环 |

含义：Codex 把 **HITL、压缩、子 Agent、配置热更新** 都做成同一条控制面，而不是 chat 主路径旁边再挂一堆旁路。

### 2.2 会话对象

- `Session`（`core/src/session/session.rs`）：一条交互线程的运行时，持有 services / telemetry / MCP / extensions。
- `TurnContext`：本轮模型、工具模式、developer instructions、权限 profile、环境快照。
- `Thread` + `rollout`：可 resume / fork；rollout 是记忆 Phase 1 的原料。
- `SessionServices`：MCP handler cache、skill 扩展、tool search cache、analytics。

TUI 另有 `codex resume` / `codex fork`。模型自己也能 `spawn_agent`（见 §4）。

### 2.3 一回合怎么走（逻辑顺序）

```
用户输入
  → 解析 $skill / 结构化 Skill mention
  → 加载 AGENTS.md 链（受信任项目）+ user instructions
  → 注入 memory_summary（若启用且非 sub-agent）
  → 按 Feature / ToolMode 组 ToolRegistry
  → 模型流式输出：preamble →（可选 update_plan）→ function calls
  → PreToolUse hook → 沙箱/审批门 → 执行 → PostToolUse hook
  → 截断工具输出（保留 exit code / 墙钟 / 行数）
  → 并行工具（声明了 supports_parallel 的）
  → 自动 compact 或模型调用 get_context_remaining / new_context_window
  → 最终答案（含可选 memory citation）
```

Guardian reviewer 源会把工具面收成 `exec_command` + `write_stdin` + `view_image`，避免审查员自己改仓库。

### 2.4 子 Agent

`spec_plan.rs` 的 `add_collaboration_tools`：

- **v1**：`spawn_agent` / `send_input` / `wait_agent` / `resume_agent` / `close_agent`
- **v2**（feature）：`spawn_agent` / `send_message` / `followup_task` / `wait_agent` / `interrupt_agent` / `list_agents`，可进独立 namespace「Tools for spawning and managing sub-agents.」

约束在业务层写死：记忆 Phase 2 的 consolidation 子 Agent **关闭 collab**，防止递归委派；spawn 有 depth limit（`exceeds_thread_spawn_depth_limit`）。

这是「模型可调用的轻量子 Agent」，不是 Aranea 的编制表 / 花名册编排。两者不该互相替代。

---

## 3. Context：项目文档、预算、压缩、模型可见窗口

### 3.1 `AGENTS.md` 是运行时契约，不是 README 约定

`codex-rs/core/src/agents_md.rs`：

1. 从 cwd 向上找 `project_root_markers`（默认 `.git`）。
2. 从项目根走到 cwd，每层只取一个候选：`AGENTS.override.md` > `AGENTS.md` > 配置的 fallback 文件名。
3. 拼接进 developer / user instructions；总字节受 `project_doc_max_bytes` 限制，超了截断。
4. **未信任项目直接不读项目文档**。
5. 多环境（本地 + remote exec）共享同一字节预算。

Base instructions（`protocol/src/prompts/base_instructions/default.md`）把规则写成模型必须遵守的 spec：

- 作用域 = 该文件所在目录树
- 更深的 `AGENTS.md` 覆盖更浅的
- 系统/developer/用户指令优先于 `AGENTS.md`
- 根到 cwd 的内容已经在 developer message 里，**不要再读一遍**

这是 Codex 比多数 Agent 平台更「像资深同事」的关键：仓库自己带操作手册，且嵌套覆盖。

### 3.2 每轮进模型的分层

大致栈（从稳到动）：

1. **Base instructions**（人格、工作法、工具纪律、最终答案格式）
2. **Developer instructions**（会话/协作/Guardian/记忆 read-path；`$CODEX_HOME/AGENTS.md` 用户层）
3. **World State diff**（`session/world_state.rs`）：权限、环境、personality、AgentsMd 状态、skill catalog 摘要、deferred 工具 namespace——只发相对上一 step 的增量
4. **Permissions instructions**（当前 sandbox + approval 模板）
5. **Project docs**（仓库 `AGENTS.md` 链，`--- project-doc ---` 拼接）
6. **Memory summary**（`~/.codex/memories/memory_summary.md`，约 2500 token 帽）
7. **历史 + 本轮 user / skill / 图像**（`ContextManager.for_prompt`）
8. **工具 schema**（Direct 全量；Deferred 只露 `tool_search`）

动态 cue 尽量不打进稳定前缀，利于 prompt cache。`capture_step_context` → `build_initial_context_with_world_state` 是每 sampling step 的组装入口。

### 3.3 压缩：交接，不是丢弃

`codex-rs/prompts/templates/compact/prompt.md`：

> You are performing a CONTEXT CHECKPOINT COMPACTION. Create a handoff summary for another LLM that will resume the task.

要求写清：进度与关键决策、约束/偏好、下一步、继续所需的关键数据。  
`summary_prefix.md` 把摘要标成「另一只模型的思考过程摘要」，让后续模型接着干而不是重探。

运行时：

- 手动 `Op::Compact` → `run_compact_task`（`SUMMARIZATION_PROMPT` 或 `config.compact_prompt`）
- `AutoCompactWindow` / `run_inline_auto_compact_task`：token 超 `model_auto_compact_token_limit`
- 远程 compact API：`compact_remote.rs` / `compact_remote_v2.rs`
- **Token-budget 路径**：`compact_token_budget.rs` 可 **直接 `start_new_context_window()`，不做 LLM 摘要**（与 Aranea 硬截断更近）
- 模型工具：`get_context_remaining`、`new_context_window`
- Compact 前后有 hook；mid-turn 可用 `InitialContextInjection::BeforeLastUserMessage` 把 world state 重新垫回去

主路径哲学仍是「花一次小成本换可恢复任务状态」；第三条路径说明 Codex 也承认摘要不是唯一退路。

### 3.4 工具输出也是 Context 的一部分

`format_exec_output_for_model` 固定结构：

```
Exit code: N
Wall time: X.Y seconds
Total output lines: M   # 仅截断时
Output:
<truncated body>
```

截断策略在 `codex-utils-output-truncation`。模型始终知道「还有多少没看见」，而不是静默丢尾。

---

## 4. 工具调研：目录、延迟加载、并行、调研型工具

### 4.1 内置工具清单（`spec_plan.rs` + handlers）

| 工具 | 作用 |
|------|------|
| `exec_command` | 统一 exec（shell / login shell / 多环境 `environment_id`） |
| `write_stdin` | 给仍在跑的进程写 stdin（PTY） |
| `apply_patch` | 结构化补丁；虚拟 CLI `--codex-run-as-apply-patch` |
| `view_image` | 读图，可要 original detail |
| `update_plan` | 步骤计划（pending / in_progress / completed） |
| `list_mcp_resources` / `list_mcp_resource_templates` / `read_mcp_resource` | MCP 资源面 |
| `request_permissions` | 运行时要升权 |
| `request_user_input` | 向用户要结构化输入 |
| `send_user_message_async` | 实验：异步回用户 |
| `get_context_remaining` / `new_context_window` | 上下文预算 |
| `current_time` / `sleep` | 时间与等待 |
| `wait_for_environment` | DeferredExecutor：等远程/环境就绪 |
| `list_available_plugins_to_install` / `request_plugin_install` | 插件市场 |
| `web_search` | Hosted Responses 工具（账号能力门控） |
| `tool_search` | BM25 发现被 `defer_loading` 的工具 |
| `spawn_agent` 及 v2 协作族 | 子 Agent |
| `imagegen` | 图像生成 namespace（feature） |
| `web.run` | 客户端独立 web search（`ext/web-search`，与 hosted `web_search` 互斥路径） |
| `skills.list` / `skills.read` | 扩展：列 catalog / 读 SKILL.md 全文（host skill 也可走 prompt 注入） |
| `memories.list` / `read` / `search` / `add_ad_hoc_note` | 扩展：操作 `~/.codex/memories` |
| `test_sync` | 仅测试 |

另有 **Code Mode**：模型不直接 function-call，而是在进程内执行一段「调工具的程序」。`ToolMode::Direct | CodeMode | CodeModeOnly`。Guardian 与部分 namespace 可强制 Direct-only。

**没有** computer-use / Playwright 内置工具；浏览器只用于 OAuth。代码搜索不是独立 tool，base instructions 要求用 `rg`。`file-search` crate 只服务 TUI `@` mention。

### 4.2 工具调研（Tool Search）——这是和 Aranea 最对齐的一块

`codex-rs/tools/src/tool_search.rs` + `core/src/tools/handlers/tool_search.rs`：

1. 注册时给可延迟工具打 `defer_loading = true`，并从 schema 去掉 `output_schema`（搜索索引只要名字/描述）。
2. 搜索文本 = namespace + description + 各 tool name/description。
3. 运行时用 **BM25**（`SearchEngineBuilder`）检索，默认 limit 常量 `TOOL_SEARCH_DEFAULT_LIMIT`。
4. `ToolSearchHandlerCache` 按 registry 身份缓存，避免每轮重建索引。
5. Direct 与 Deferred **互斥**：开了 search 的工具从 Direct 面拿掉，只出现在 search 结果里。

这不是「再写一个 tool_search 工具」那么简单，而是 **Responses API 级的 deferred catalog**：模型先看到少而稳的核心工具，长尾 MCP/插件按查询加载。

### 4.3 执行面

- **并行**：MCP 可标 `supports_parallel_tool_calls`；core 有 `tools/parallel.rs`。
- **Hook**：PreToolUse 可改参数或否决；PostToolUse 看结果。
- **审批**：exec / patch / network / MCP per-tool。
- **插件**：`dynamic_tools`、extension executors、hosted web search。

### 4.4 「调研」工具的产品分层

Codex 把调研拆成三层，而不是一只万能 search：

| 层 | 机制 | 典型用途 |
|----|------|----------|
| 仓库内 | 模型用 `rg` / `rg --files`（写进 base instructions） | 代码与配置 |
| 工具目录 | `tool_search` BM25 | 发现 MCP/插件能力 |
| 外网 | `web_search`（hosted） | 文档、API、现状 |
| 记忆 | `MEMORY.md` grep + rollout 指针 | 跨会话经验 |

Skill `openai-docs` 还带 scripts 去拉最新模型/手册，属于「调研技能化」。

---

## 5. Skill

### 5.1 发现与范围

`SkillMetadata`（`skills/src/model.rs`）：

- `name` / `description` / `short_description`
- `interface`（展示名、图标、默认 prompt）
- `dependencies.tools`（可声明 MCP：type/value/transport/command/url/oauth port）
- `policy.allow_implicit_invocation`、`products` 门控
- `scope`：User / Repo / System / Admin
- `plugin_id` / `remote_plugin_id`

加载：`SkillRootLoader` 扫有序 roots，带 snapshot cache；产品过滤在 load 时做。

发现路径（`ext/skills/src/host_roots.rs`，后去重）：

| 来源 | 路径 | Scope |
|------|------|-------|
| 项目 config | `<project>/.codex/skills/` 或 `<project>/skills/` | Repo |
| 用户 home | `~/.agents/skills/`、`$CODEX_HOME/skills/`（后者 deprecated） | User |
| 系统嵌入 | `$CODEX_HOME/skills/.system/`（bundled samples） | System |
| Admin | `/etc/codex/skills/` 或 system config layer | Admin |
| 仓库上走 | `<root>..<cwd>/.agents/skills/` | Repo |
| Plugin | `PluginSkillRoot` | Plugin |

Catalog 有 `skills.max_context_tokens`（约 context 的 2%，封顶约 10k）。文案要求：**先完整读 SKILL.md 再行动，不要把 skill 解读委派给 subagent**。

### 5.2 选择：显式 > mention > 隐式

`selection.rs`：

1. 结构化 `UserInput::Skill { name, path }` 先按路径对齐。
2. 文本里扫 `$skill-name`；重名只在无歧义时用 plain name。
3. 禁用 path 不选。
4. `ext/skills/src/dynamic_skill_selector.rs` 另有廉价选择器族（Fielded BM25、LRU、char n-gram、RRF），**shadow 模式每轮可跑、不改模型可见目录**——用来做实验指标，而不是立刻改变行为。

隐式：shell 命令命中 skill 路径时 `detect_implicit_skill_invocation`，telemetry `invoke_type=implicit`。可用 `allow_implicit_invocation=false` 关掉。

### 5.3 三级披露（官方 sample 写进 `skill-creator`）

```
1. name + description     → 选择期常驻（便宜、要能区分）
2. SKILL.md body          → 被选中/mention 后注入
3. references/ / scripts/ → 任务真正需要时再读或执行
```

Frontmatter 只要 `name` + `description`；`metadata.short-description` 可选。解析器会 **修复** 第三方不合法 YAML（例如 `description: Build for AWS: ECS`），避免一堆 skill 静默失败。

### 5.4 与 `AGENTS.md` 的边界

| | `AGENTS.md` | Skill |
|--|-------------|-------|
| 作用域 | 目录树，碰这个树里的文件就必须遵守 | 任务级能力包 |
| 加载 | 启动/换 cwd 自动拼进 developer | 目录常驻，正文按需 |
| 谁写 | 仓库贡献者 | 用户 / 插件 / 记忆巩固产出 |

记忆 Phase 2 还可以把可复用流程写成 `~/.codex/memories/skills/<name>/`，等于 **从 rollout 长出 skill**。

---

## 6. MCP

### 6.1 配置（`config/src/mcp_types.rs`）

传输只有两种（2025-06-18 spec）：

- **stdio**：`command` / `args` / `env` / `env_vars` / `cwd`
- **streamable_http**：`url` / `bearer_token_env_var` / `http_headers` / `env_http_headers` / `http_headers_helper`（本地命令打出动态 header JSON）

stdio 上写 `url` / `oauth` / `http_headers` 会硬拒绝。  
HTTP 上写 `args` / `env` / `cwd` 同样拒绝。

每服务器还有：

| 字段 | 作用 |
|------|------|
| `enabled` / `required` | 关停；`codex exec` 下 required 启动失败即退出 |
| `environment_id` | 本地 vs executor 环境 |
| `supports_parallel_tool_calls` | 该 server 全部工具可并行 |
| `omit_tools_from` | 从 Direct / Deferred / CodeMode 等面隐藏 |
| `enabled_tools` / `disabled_tools` | 白/黑名单 |
| `default_tools_approval_mode` + `tools.{name}` | 默认与 per-tool 审批 |
| `startup_timeout_sec` / `tool_timeout_sec` | 超时 |
| `oauth` / `oauth_resource` / `scopes` / `auth` | OAuth / ChatGPT 登录 |

OAuth 凭据名按 environment 隔离（`executor:{env}:{server}`），避免本地与远程抢同一 key。

### 6.2 运行时

- `build_tool_router` 把 MCP 工具 append 进 registry，再套 exposure 策略。
- 有 server 时自动加 MCP **资源** 三件套（list / templates / read）。
- `Op::RefreshMcpServers` 热刷新。
- `ext/mcp`：Apps feature 贡献 hosted plugin runtime；executor plugin 可按线程贡献 MCP。
- Skill 可声明 MCP 依赖（`mcp_skill_dependencies.rs`）。

### 6.3 Codex 自己当 MCP

`codex-rs/mcp-server` + CLI `codex mcp` / `codex mcp-server`：把 Codex 能力暴露给 Cursor / Claude / 其他 harness。这是双向的——Codex 既消费 MCP，也提供 MCP。

Aranea 目前主要是 **消费方 + 管理面**；把某个 Agent 暴露成 MCP server 仍弱。

---

## 7. Prompt

### 7.1 模板资产

| 位置 | 内容 |
|------|------|
| `protocol/src/prompts/base_instructions/default.md` | 主系统提示（人格、AGENTS spec、preamble、plan、执行、验证、最终格式） |
| `models-manager/prompt.md` | 同上的模型侧副本 + personality placeholder |
| `prompts/templates/compact/` | 压缩交接 |
| `prompts/templates/permissions/` | sandbox × approval 组合说明 |
| `prompts/templates/review/` | review rubric + 成功/中断退出 |
| `prompts/templates/goals/` | 目标预算、续跑、目标变更 |
| `prompts/templates/realtime/` | 语音开始/结束/backend |
| `memories/write/templates/` | Phase1 抽取、Phase2 巩固 |
| `ext/memories/templates/memories/read_path.md` | 读路径：何时查记忆、怎么查、怎么引用 |
| `tui/prompt_for_init_command.md` | `/init` 生成 `AGENTS.md` |

`prompts` crate 只 **导出常量与渲染函数**，不在这里塞业务 if。

### 7.2 Base instructions 里真正值钱的产品决策

摘自 `default.md`（不是空话）：

- **人格**：concise, direct, friendly；默认不超过约 10 行，复杂任务再放松。
- **Preamble**：工具调用前 1–2 句（约 8–12 词），相关动作合并说；琐碎单文件 read 可省略。
- **Plan**：非平凡 / 多阶段 / 用户一次提多件事才用 `update_plan`；一步一短句；恰好一个 `in_progress`；不要把计划全文再复述一遍。
- **执行**：keep going until resolved；根因修复；不顺手修无关测试；不主动 commit/分支；不乱加 copyright；patch 后不要再读文件验货。
- **验证时机**：`never` 审批模式主动测；交互审批模式先问再跑 lint/test；测试类任务例外。
- **野心 vs 精确**：新项目可有创意；旧仓库外科手术。
- **最终答案**：`**Title Case**` 段头、`-` 列表、路径可点击、禁止 nested bullets、禁止 ANSI。
- **工具纪律**：搜索优先 `rg`；编辑只用 `apply_patch`。

这些是 **编码 Agent 的 UX 合同**。Aranea 的 system prompt 今天更偏角色/岗位/记忆自标记，缺这一层「怎么在终端里当同事」。

### 7.3 记忆读路径 Prompt（`read_path.md`）

决策边界写得很硬：

- 自包含小问题（时间、翻译、一行命令）**跳过记忆**
- 提到 MEMORY_SUMMARY 里的仓库/模块、要一致性、任务含糊 → **默认走记忆**
- 快路径：skim summary → grep `MEMORY.md` → 只打开 1–2 个被指针点到的 rollout/skill → 没有命中就停
- 预算：最好 ≤ 4–6 次记忆查找
- 引用：最终回复末尾唯一一块 `<oai-mem-citation>`
- **禁止模型直接改 MEMORY.md**；用户明示更新时只往 `extensions/ad_hoc/notes/` 丢小文件，留给巩固管线

这是「渐进披露 + 防记忆污染」的完整合同。

---

## 8. 记忆

### 8.1 磁盘布局（`~/.codex/memories/`）

| 文件 | 何时进模型 |
|------|------------|
| `memory_summary.md` | **每轮 developer 注入**（首行必须 `v1`；超 token 截断） |
| `MEMORY.md` | 模型 grep 的手册/路由层 |
| `rollout_summaries/*.md` | 被 MEMORY.md 指到才读 |
| `skills/<name>/` | 巩固出来的可复用流程 |
| `raw_memories.md` | Phase2 输入，临时 |
| `phase2_workspace_diff.md` | 给巩固 Agent 看的 git diff |
| `extensions/ad_hoc/notes/` | 用户/模型显式更新的小条 |

目录本身是 git baseline（`codex-git-utils`），Phase2 用工作区脏检测决定要不要跑巩固 Agent。

### 8.2 写管线：启动时两阶段（`memories/README.md`）

触发：根会话启动，且非 ephemeral、记忆 feature 开、**不是 sub-agent**、state DB 可用。后台异步。

**Phase 1 — 按 thread 抽 rollout**

- 从 state DB claim 一批空闲足够久、未在飞、年龄窗口内的 interactive rollout
- 并行（有并发帽）送给模型，结构化输出：`raw_memory` / `rollout_summary` / `rollout_slug`
- 无高信号则空对象（鼓励 no-op）
- 密钥脱敏；失败带 backoff，不热循环

**Phase 2 — 全局巩固**

- 单全局锁
- 按 usage_count + last_usage 选 top-N stage-1
- 同步 `raw_memories.md` + `rollout_summaries/`
- 有 diff 才 spawn consolidation 子 Agent：无审批、无网络、只能写本地、**关 collab**
- 成功后重置 git baseline

高信号定义（模板原文）：稳定用户偏好、能少打断用户的决策触发器、失败盾（症状→原因→修复→验证→停止）、仓库地图、可复现成功路径。明确不要：泛建议、密钥、大段工具输出、一次性闲聊。

### 8.3 读与治理

- `memories/read`：注入、citation 解析、按读到的路径记 usage（MEMORY.md / summary / raw / rollout / skills）
- Citation 反写 `last_usage` / `usage_count`，喂给下一轮 Phase2 排序
- 模型不能自由改手册，避免把幻觉写进长期记忆

### 8.4 和「会话记忆」的关系

Codex 的 thread/rollout 是 **短期任务磁带**；`~/.codex/memories` 是 **跨会话蒸馏**。  
它几乎不做 Aranea 那种 L3 双时态 fact 表 / L4 图谱。它赌的是：**文件 + grep + 渐进披露** 对编码场景足够，且对模型更自然。

---

## 9. 其他值得记下的机制（非本次主轴，但对齐时别丢）

| 机制 | 要点 |
|------|------|
| Hooks | managed hooks；`requirements.toml` 可 `allow_managed_hooks_only` |
| Plugins / Apps | 安装、marketplace、executor plugin 自带 MCP |
| File watcher | 工作区变化 |
| Goal | 长任务目标与预算模板 |
| Guardian | 审查员工具面收窄 |
| Code Mode | 用程序编排工具，减 schema |
| Exec policy | 命令策略（`docs/execpolicy.md`） |
| Thread store | rollout 持久化与迁移 |

---

## 10. 对 Aranea 的直接含义（详见对照报告）

Codex 赢在 **编码 harness 的完成度**：沙箱、补丁、项目文档、延迟工具、记忆文件夹、prompt 合同。  
Aranea 赢在 **公司级平台**：组织/岗位、Team Graph、知识库、MCP 管理台、L0–L4、语音、多租户治理。

不要把 Codex 整仓搬进 Aranea。要搬的是 **可移植的合同**：

1. 项目文档链（`AGENTS.md` 运行时注入）
2. 压缩 = 交接摘要
3. 工具 Direct/Deferred 与协议级 `defer_loading`
4. Skill 三级披露 + `$skill` + 隐式调用观测
5. 记忆：常驻 summary + 按需手册 + sleep-time 巩固 Agent
6. 编码 Agent 的 preamble / plan / 验证时机 prompt

评分、分项差距、分阶段方案见 [2026-08-22-analysis-codex-vs-aranea.md](./2026-08-22-analysis-codex-vs-aranea.md)。
