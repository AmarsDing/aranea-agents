# 25 Aranea CLI — 需求文档（PRD, 2026-05-27）

> **版本**：3.1（取代 `25 cli.md` v2.0）
> **同系列**：设计 → [`25-cli.design.md`](./25-cli.design.md)；开发计划 → [`25-cli.development.md`](./25-cli.development.md)；上层方案 → [`25-cli-implementation.md`](./25-cli-implementation.md)
> **范围**：终端可执行二进制 `aranea`；不包括 `cmd/araneactl/`（开发者 lint/fmtcheck 工具链，与本 PRD 共存）。

---

## 0. 文档导读

本 PRD 描述 **Aranea CLI** 的产品需求与验收准则。技术结构、目录划分、关键代码骨架在 **设计文档**；阶段任务表、DoD、依赖图、估时在 **开发计划**。本文档不重复设计文档内容，只在必要处给出引用。

读者顺序：

1. 产品 / 评审：读完本文档；不需读设计与计划。
2. 实施 AI / 工程师：先读本 §1–§3 锁定目标，再跳到设计文档与开发计划。
3. 仅修复或回归：读 §6 验收准则 + 对应开发计划的 CLI-XX 任务。

---

## 1. 产品定位

Aranea CLI 是与后端 `cmd/admin` **完全异构**的终端控制台：

- 目标用户：Aranea 平台的运维 / 开发者 / 高级使用者；在终端 / SSH / CI 环境内完成日常 Agent / Skill / Tool 管理与对话。
- 部署形态：单一可执行二进制 `aranea`（Win / macOS / Linux × amd64 / arm64，`CGO_DISABLED=1`，零运行时依赖）。
- 通信形态：**只**通过后端公开的 HTTP `/v1/*` 与 WebSocket `/v1/ws` 接口；CLI 进程内**不**启动 Agent Runner、**不**读写数据库、**不** import `internal/biz` / `internal/agent` / `pkg/trpc-agent-go`（红线，见设计文档 §3.4）。
- 与 Web 控制台的关系：覆盖 Web 全部"管理类"动作（CRUD + 启停 + 安装 + 监控）；对话模式与 Web Chat 共用同一后端会话表 `sessions/messages`。

### 1.1 与原方案的关键差异（必须对齐）

为避免使用过期细节落地，本 PRD 对原 `25 cli.md`（v2.0）做以下勘误与收窄；详细差异说明见 [`25-cli-implementation.md` §0](./25-cli-implementation.md)。

| # | 项 | 原方案 | 本 PRD |
|---|----|--------|--------|
| D1 | REST 路径前缀 | `/api/v1/*` | **`/v1/*`**（仓库所有 proto `google.api.http` 注解均为 `/v1/*`） |
| D2 | 登录接口 | 占位 | 真实接口 `POST /v1/admins/login`（已落 `pkg/auth/middleware.go::noAuthPaths`） |
| D3 | 多工作区 | `workspace_id` 字段在配置中生效 | 配置字段保留为占位字符串；本期**不实现**多租户切换（`pkg/auth.Auth` 仅 `UserID + Access`） |
| D4 | 对话模式实现 | CLI 进程内构建 `trpcrunner.Runner` + `dbAgentLoader` + `SQLiteSessionService` | **删除**；CLI 仅作为 WS 客户端连 `/v1/ws`，Runner 装配仍只在 `internal/service` |
| D5 | 二次确认机制 | 新增 `ConfirmPlugin` | **删除**；复用 `chat.proto` 既有 `AwaitUserReply` RPC + WS 事件 |
| D6 | Skill 安装扩展 | 改 proto 字段 | **不改 proto 服务**；走 multipart form 字段 `source / source_url / source_ref / source_subpath / client_validation` 写入 `metadata_json` |
| D7 | CLI HTTP 客户端 | 手抄请求/响应 struct | **不抄**；直接 import `api/kratos/*/v1` pb 类型，用 `protojson` 编解码 |
| D8 | 输出格式 | MVP 4 种（text/json/yaml/table） | MVP 收窄为 **text + json** 两种；yaml/table 推到 P1 |
| D9 | `--workspace_id` 全局 flag | 全局 flag 之一 | **移除**（与 D3 一致） |

> 历史方案中 `25 cli.md` / `25 cli.design.md` / `25-cli.development.md` 与本 PRD 不一致处，**以本 PRD 为准**；原三份将在顶部标注 superseded。

---

## 2. 用户故事（User Stories）

每条故事以 `As a <角色>, I want <行为>, so that <价值>` 表述，后接关键验收要点（详细 DoD 在开发计划）。

### 2.1 P0（MVP，必须）

**US-01 安装与首次使用**
> 作为一位运维，我想在我的笔记本上下载单一二进制 `aranea` 并执行 `aranea version`，确认二进制可执行、后端可达，然后用 `aranea login` 登录后端。

要点：

- 二进制单文件可执行，无依赖动态库；
- `aranea version` 在没登录、无网时也能输出本地版本与 `commit`；
- `aranea login --base-url <url> --user <name> --password <pwd>` 成功后写 token 到 `config.toml`（文件权限 0600，Win 跳过 chmod 仅打印提示）。

**US-02 列出与查看资源**
> 作为开发者，我想用 `aranea agent ls`、`aranea skill ls`、`aranea tool ls` 列出后端资源，并用 `... get <id>` 看详情。

要点：

- 支持 `--page` / `--page-size` / `--search`；
- `--output text` 默认（TTY 带色 + 表格，管道 key=value）；
- `--output json` 输出 `protojson` 可被 `jq` 解析；
- 401 / 403 时退出码 6，提示重跑 `aranea login`。

**US-03 资源 CRUD**
> 作为运维，我想用 YAML/JSON 文件创建 / 更新 Agent / Skill，避免在 Web 端手填。

要点：

- `aranea agent create --file agent.yaml`、`agent update <id> --file agent.yaml`；
- `--file -` 支持从 stdin 读取；
- 删除带二次确认（除非 `--yes`），关键写动作（delete / disable）携带 `confirm_key` 给后端二次校验；
- 错误信息 `text` 模式带红色 `code` + 黄色 `hint`；`json` 模式按 §5.3 schema 透传。

**US-04 启停 Agent / Skill / Tool**
> 作为运维，我想快速 enable / disable 某个 Agent 或 Skill 或全局 Tool。

要点：

- 子命令：`agent enable|disable <id>`、`skill enable|disable|publish <id>`、`tool enable|disable <id>`；
- 高风险（如 `tool` 启停）默认要求二次确认；
- 退出码与错误码映射见 §5.2。

**US-05 Agent 工具策略读写**
> 作为开发者，我想用 `aranea agent tools <id>` 看 Agent 当前生效工具集，用 `aranea agent tools-set <id> --allow ... --deny ...` 改策略。

要点：

- 对应后端 `/v1/agents/{id}/tools/effective` 与 `/v1/agents/{agent_id}/tools/policy`；
- `--allow` / `--deny` 接受逗号分隔多值（与 `tools_allow_json` / `tools_deny_json` 一致），CLI 端 join 后 PUT。

**US-06 系统信息**
> 作为运维，我想用 `aranea system info` 一次看到后端版本、commit、build_time、默认 provider / model、系统管家 agent_key。

要点：

- 新增后端 `GET /v1/system/info`（详见设计文档 §6.1）；
- 字段缺失时 CLI 不崩溃，缺啥打"N/A"；
- 后端不可达 → 退出码 3。

**US-07 配置管理**
> 作为开发者，我想用 `aranea config path / get / set` 在不打开编辑器的前提下查看与修改配置。

要点：

- 跨平台路径：`$XDG_CONFIG_HOME/aranea/config.toml`（Linux/mac）；`%APPDATA%\aranea\config.toml`（Win）；
- `set` 写文件保留原注释（依赖 `pelletier/go-toml/v2`）；
- `set backend.token` 写入后文件权限校正为 0600；
- 不暴露 token 明文（`get` 显示为 `***<last4>`，需要 `--show-token` 才显全文，且打印一条 stderr 警告）。

**US-08 CI 友好输出**
> 作为 CI，我想 `aranea skill ls --output json --quiet` 输出能直接管道喂给 `jq`，并且任何错误都映射到稳定的非零退出码。

要点：

- 非 TTY 自动关闭色、spinner，`text` 退化为 `key=value`；
- `--quiet` 在 list 上每行一个 ID，在 get 上仅输出 ID + 状态；
- 退出码语义稳定（见 §5.2），与脚本行为合约。

---

### 2.2 P1（应做，第 2~3 周）

**US-10 自然语言驱动管理**
> 作为开发者，我想 `aranea`（无参数）直接进入对话 REPL，对系统管家说"帮我把 figma-code-connect 装上"。

要点：

- 启动横幅：版本 / 后端地址 / 当前 Agent / Session 状态；
- 流式渲染：模型 delta + 工具调用折叠块 + ✓/⚠/✗ 状态；
- 二次确认走后端 `AwaitUserReply` RPC（不在 CLI 端塞确认状态机）；
- 高风险操作未 `--yes` / `/yes` 时阻塞等待用户回复；
- 内置斜杠命令：`/help /agent /session /yes /quit /tools /expand /copy /dry-run`（详见设计文档 §4.5）。

**US-11 安装远端 Skill**
> 作为开发者，我想 `aranea skill install https://github.com/anthropic/skills/tree/main/figma-code-connect` 一行命令把它装到后端。

要点：

- 支持 URL 形态：`github / gitlab / gitee / codeberg` 仓库根 / 子目录 / blob；`https://.../skill.zip` 直接 zip URL；
- 参数：`--ref <branch/tag/sha>`、`--subpath <dir>`、`--decision skip|keep|refine`、`--keep-temp`、`--enable`、`--publish`；
- 状态机：解析 URL → 解析 ref → fetch → 定位 SKILL.md → 本地预校验 → 打包 zip → POST `/v1/skills/import`（multipart）→ 轮询 `job_id` 1.5s × 80 次（cap 120s）→ apply；
- 冲突组：`pass` 自动 apply；`warn` 询问或按 `--decision`；`block` 立即 `exit 5`；
- 失败回退见 §6.2。

**US-12 剩余资源 CLI**
> 作为运维，我想完整覆盖 `team / plugin / mcp / cron / channel / session / monitor` 七类资源的 CRUD + 动作。

要点：

- 子命令清单见 §3.2；
- 验收以每个资源一条 happy-path smoke 通过为准；
- 危险动作（`channel send` / 删除资源 / 启停高风险 Tool）默认强制二次确认；CI 必须显式 `--yes`。

**US-13 Shell 补全**
> 作为开发者，我想 `aranea completion zsh > _aranea && source _aranea` 后获得子命令、flag、资源 ID 的 tab 补全。

要点：

- bash / zsh / fish / powershell 四种均输出可用脚本（cobra 自带能力）；
- 资源 ID 补全为可选增强（基于 `aranea <res> ls --output json` 缓存），P2 再做。

---

### 2.3 P2（可选，按需）

| ID | 故事 |
|----|------|
| US-20 | `--output yaml / table` 切换 |
| US-21 | REPL 上下箭头历史搜索 |
| US-22 | 错误信息中文翻译表（按 `error.code`） |
| US-23 | 跨平台二进制发布（GitHub Release / OSS） |
| US-24 | 系统 keyring 集成（macOS Keychain / Windows Credential Manager / libsecret） |
| US-25 | `aranea init` 引导（首启检测后端 + 写默认 config） |
| US-26 | E2E smoke 纳入 `make ci`（带 admin 后端启停） |

---

### 2.4 显式不做（OUT-OF-SCOPE）

| # | 项 | 理由 |
|---|----|------|
| N1 | 多工作区 / 多租户切换 | 后端模型暂无对应字段 |
| N2 | 远程 OIDC / SSO | 后端无对应实现；沿用 `/v1/admins/login` |
| N3 | 插件式第三方 CLI 命令 | 安全面过大，禁止 `aranea` 加载外部代码 |
| N4 | 自更新 / auto-upgrade | 仅 `aranea version` 打印升级指引 |
| N5 | TUI 框架（bubbletea / tview） | 增量价值不抵复杂度 |
| N6 | CLI 进程内嵌后端 / Runner | 违反红线（设计 §3.4） |
| N7 | CLI 直连数据库 | 违反红线（设计 §3.4） |
| N8 | 把 `internal/agent` / `internal/biz` 暴露给 CLI 二进制 | 违反红线（设计 §3.4） |

---

## 3. 功能矩阵

### 3.1 入口与全局能力

| 入口 | 类型 | 阶段 | 说明 |
|------|------|------|------|
| `aranea`（无参数） | REPL | P1 | 进入对话模式，连 WS `/v1/ws`，默认 Agent = `__system_admin__` |
| `aranea chat [...]` | REPL | P1 | 显式入口；可指定 `--session` / `--agent` |
| `aranea version` | 信息 | P0 | 含后端 reachability 探测 |
| `aranea login [...]` | 鉴权 | P0 | 写 token 到 config，文件 0600 |
| `aranea config <get/set/path>` | 配置 | P0 | 跨平台路径；`set backend.token` 自动 0600 |
| `aranea completion <shell>` | 补全 | P1 | cobra 自带 |
| `aranea <资源> <动作> [...]` | 直接命令 | P0/P1 | 见 §3.2 |
| `aranea --help` / `<cmd> --help` | 帮助 | P0 | cobra 自带 |

### 3.2 资源 / 动作矩阵（MVP + P1）

| 资源 | 子命令 | 阶段 | 对应后端路径 | 备注 |
|------|--------|------|--------------|------|
| `agent` | `ls` | P0 | `GET /v1/agents` | 支持 `--page --page-size --search` |
| `agent` | `get <id>` | P0 | `GET /v1/agents/{id}` | |
| `agent` | `create --file <f>` | P0 | `POST /v1/agents` | YAML/JSON → pb |
| `agent` | `update <id> --file <f>` | P0 | `PATCH /v1/agents/{id}` | FieldMask 由 CLI 计算 |
| `agent` | `delete <id> [--yes]` | P0 | `DELETE /v1/agents/{id}` | 二次确认 |
| `agent` | `enable <id>` / `disable <id>` | P0 | 待后端补 `PATCH /v1/agents/{id}/enabled` 或复用 `update` 的 `enabled` 字段 | 实施前对照 proto；若 proto 无单独 RPC，CLI 调 update + FieldMask |
| `agent` | `tools <id>` | P0 | `GET /v1/agents/{agent_id}/tools/effective` | |
| `agent` | `tools-set <id> --allow ... --deny ...` | P0 | `PUT /v1/agents/{agent_id}/tools/policy` | |
| `skill` | `ls / get / create / update / delete / enable / disable / publish` | P0 | `/v1/skills*` | 按 proto |
| `skill` | `install <url>` | P1 | clone + multipart 上传 + 轮询 | 状态机见 §6.2 |
| `skill` | `import <zip>` | P1 | `POST /v1/skills/import` | |
| `skill` | `import-status <job_id>` | P1 | `GET /v1/skills/import/{job_id}` | |
| `skill` | `import-apply <job_id>` | P1 | `POST /v1/skills/import/{job_id}/apply` | |
| `tool` | `ls / get / enable / disable` | P0 | `/v1/tools*` | `enable/disable` 高风险二次确认 |
| `system` | `info` | P0 | **新增** `GET /v1/system/info` | 见 §3.4 |
| `graph` | `ls / get / create / update / delete` | P1 | `/v1/graphs*` | Graph 工作流管理 |
| `pkg` | `install <url>` | P1 | 走 skill import + multipart | 包安装快捷方式 |
| `team` | `ls / get / create / update / delete / run / runs / run-events` | P1 | `/v1/teams*` / `/v1/team-runs*` | |
| `plugin` | `ls / get / enable / disable / order-set / config-set` | P1 | `/v1/plugins*` | |
| `mcp` | `ls / get / add / update / delete / test` | P1 | `/v1/mcp-servers*` | |
| `cron` | `ls / get / add / update / delete / pause / resume / trigger` | P1 | `/v1/cron-tasks*` | |
| `channel` | `ls / get / add / update / delete / test / send` | P1 | `/v1/channels*` | `send` 强制 `--yes` |
| `session` | `ls / get / send` | P1 | `/v1/sessions*` + `/v1/chat/messages` | |
| `monitor` | `audit-logs / events / traces` | P1 | `/v1/monitor/*` | |
| `session` | `archive / restore / pin / unpin / compact / export` | P3 | `/v1/sessions/{id}/archive` 等、`/v1/sessions/{id}/export` | 2026-07-28 补全 |
| `skill` | `files / file-get / file-put / file-delete` | P3 | `/v1/skills/{id}/files*`、`/v1/skills/{id}/file` | 2026-07-28 补全 |
| `cron` | `reset-failures` | P3 | `POST /v1/cron-tasks/{id}/reset-failures` | 2026-07-28 补全 |
| `mcp` | `validate` | P3 | `POST /v1/mcp-servers/validate`（校验未保存配置负载） | 2026-07-28 补全 |
| `tool` | `test` | P3 | `POST /v1/tools/{id}/test` | 2026-07-28 补全 |
| `memory` | `facts ls`、`proposals ls/approve/reject`、`search`、`recall-debug` | P3 | `/v1/memory/*` | 2026-07-28 新增域 |
| `knowledge` | `collections ls/get/create/delete`、`documents ls/get/delete`、`search` | P3 | `/v1/knowledge/*` | 2026-07-28 新增域 |
| `eval` | `datasets ls/get/create`、`runs ls/get/create`、`results` | P3 | `/v1/evaluation/*` | 2026-07-28 新增域 |
| `org` | `ls / tree / get / create / update / delete / reorder` | P3 | `/v1/organization*` | 2026-07-28 新增域 |
| `taxonomy` | `ls / tree / get / create / update / delete / reorder` | P3 | `/v1/taxonomy*` | 2026-07-28 新增域 |
| `model-catalog` | `ls / get / policy / policy-set / sync` | P3 | `/v1/model-catalog*` | 2026-07-28 新增域 |
| `a2a` | `discover`、`remote-agents ls/get/add/delete`、`audit ls`、`config get` | P3 | `/v1/a2a/*` | 2026-07-28 新增域 |

> **proto 路径校验**：列表中"对应后端路径"以实施时 `api/kratos/*/v1/*.proto` 的 `google.api.http` 注解为唯一真相源。若某动作 proto 无对应 RPC，**CLI 不私自构造**，开 issue 由后端补。

### 3.3 全局 flag

| Flag | 类型 | 默认 | 说明 |
|------|------|------|------|
| `--base-url` | string | `http://127.0.0.1:8080` | 覆盖 `[backend].base_url` |
| `--token` | string | 空 | 覆盖 `[backend].token` |
| `--output` / `-o` | enum `text / json` | `text` | MVP 只两种；yaml/table P1 |
| `--quiet` / `-q` | bool | false | 仅关键字段 |
| `--yes` / `-y` | bool | false | 跳过二次确认（高风险写） |
| `--debug` | bool | false | HTTP 请求/响应详情到 stderr |
| `--config` | string | 跨平台默认路径 | 覆盖配置文件 |
| `--no-color` | bool | false | 同 `NO_COLOR=1` |
| `--timeout` | int | 60 | HTTP 请求超时秒数 |

### 3.4 后端契约新增（必须由后端先实现）

> 以下接口为 CLI 依赖的后端新增能力。技术契约（字段定义、payload schema、实现方式）见设计文档 §4.2；实现进度见开发计划 §1.1。

| ID | 接口 | 说明 |
|----|------|------|
| BE-1 | `GET /v1/system/info` | 见设计 §4.2.1；返回后端版本 / 系统管家 Agent 信息 / Skill 限制等 |
| BE-2 | `POST /v1/skills/import` 接收 multipart form 字段 `source / source_url / source_ref / source_subpath / client_validation`，写入 `metadata_json` | 见设计 §4.2.2 |
| BE-3 | `SeedSystemAdminAgent()` 注入 `__system_admin__`（`readonly=1`, `kind=system`, `tools_profile=system_admin`） | 见设计 §4.2.3 |
| BE-4 | `SeedBuiltinTools()` + `SeedToolGroups()` 注册 `cli_admin_*` 工具与 `group:cli_admin` 工具组 | 见设计 §4.2.3 |
| BE-5 | `internal/tools/cli_admin/` 工具实现（首批：`skill_list / get / install_from_url / import_status / import_apply / agent_list / get`） | 见设计 §4.2.4 |
| BE-6 | WS 下行细化 envelope：`tool.call / tool.result / tool.error / await.user.reply` 子类型，落在 `internal/service/chat_wire.go`（不新增 server 直接依赖 `pkg/trpc-agent-go`） | 见设计 §4.2.5 |

---

## 4. 角色与权限

当前后端 `pkg/auth.Auth` 模型：`UserID int64 + Access string`，没有 workspace / tenant。CLI 沿用现状。

| 角色 | 能力 |
|------|------|
| 后端 admin（`access=admin`） | CLI 所有命令全部可用 |
| 普通 admin（`access != admin`） | 后端拒绝大部分写动作；CLI 仅显示 403 + 提示联系管理员 |
| 未登录（无 token） | 仅 `version / config / login / completion / help` 可用；其余命令直接拒绝并指引 `aranea login` |

> 多工作区 / 多租户视后端模型成熟度后续单独立项；本期 CLI 不假设其存在。

---

## 5. 终端体验、错误与退出码

### 5.1 终端输出

| 场景 | 行为 |
|------|------|
| TTY + 支持色 | 带色 + 对齐表格；spinner |
| TTY + 不支持色（或 `NO_COLOR=1` / `--no-color`） | 纯文本 + ASCII spinner |
| 非 TTY（管道 / CI） | 关 spinner / 色；`text` 退化 `key=value`；建议 `--output json` |
| 大表格 | 终端宽度不足时优先截断中间列；用户应改 `-o json` 取完整 |
| 长任务 | 阻塞 > 200ms 显示 spinner；> 5s 显示 `(已耗时 X s)` |
| 链接 | 在结果末尾打印 Web 控制台 URL（如 `查看：<base_url>/skills/<id>`） |
| 国际化 | 默认跟随系统中文；`--lang en` 切英文（本期可只翻译关键消息） |

### 5.2 退出码

| 码 | 场景 |
|----|------|
| 0 | 成功 |
| 1 | 参数错误 / cobra 解析失败 |
| 2 | 后端业务错误（4xx，非 401/403），如 `AGENT_NOT_FOUND` |
| 3 | 网络错误 / DNS / TLS / 5xx |
| 4 | 用户取消（二次确认输入 `n` / prompt 阶段 Ctrl+C） |
| 5 | 校验或冲突阻塞（`SKILL_IMPORT_BLOCKED`；warn 且非交互且无 `--decision`） |
| 6 | 401 / 403 鉴权失败（提示重跑 `aranea login`） |
| 130 | 执行中 Ctrl+C |

### 5.3 错误展示

**text 模式**：

```
Error  SKILL_IMPORT_BLOCKED (HTTP 409)
message: 候选 Skill 与已有 Skill 同名（figma-code-connect）。
hint   : 用 `--decision skip` 跳过，或 `--decision keep` 保留两份。
```

**json 模式**（CLI 端固化，golden 测试覆盖）：

```json
{
  "error": {
    "code": "SKILL_IMPORT_BLOCKED",
    "message": "...",
    "http_status": 409,
    "metadata": { "job_id": "job_7f3", "group_id": "group_01" }
  }
}
```

约束：

- text 模式下 `code` 红色加粗，`hint` 黄色；
- json 模式输出 schema 与上例严格一致，缺字段也保留 null，不省略 key；
- 后端原始 `metadata` 透传。

---

## 6. 关键流程详细需求

### 6.1 登录（US-01）

```
aranea login --base-url <url> --user <name> --password <pwd>
```

- 调用后端登录接口（`/v1/admins/login`，已放行 noAuthPaths；请求/响应 schema 见设计 §4.1）；
- 成功：token 写入 `config.toml` 的 `[backend].token`；文件权限校正为 0600；
- 失败：HTTP 4xx → 退出码 2；网络 / 5xx → 退出码 3；
- 安全：CLI 不在 stdout 打印 token；`--debug` 模式下日志中也 mask 为 `***<last4>`。

### 6.2 Skill install from URL（US-11）

支持的 URL 形态：

| URL 形态 | 处理 |
|----------|------|
| `https://github.com/<owner>/<repo>` | clone 默认分支根目录；自动发现 SKILL.md（§6.2.1） |
| `https://github.com/<owner>/<repo>/tree/<ref>/<subpath>` | clone `<ref>`；只打包 `<subpath>` |
| `https://github.com/<owner>/<repo>/blob/<ref>/<subpath>/SKILL.md` | 取该 SKILL.md 所在目录为根 |
| `git@github.com:.../*.git` / `ssh://...` | 走 SSH（P1 标注；MVP-P1 仅声明支持 https，ssh 在 P2） |
| `https://gitlab.com/...` / `gitee.com` / `codeberg.org` | 同 GitHub 规则 |
| `https://example.com/path/skill.zip` | 直接下载 zip，跳过 clone |
| `npm:<pkg>` / `pypi:<pkg>` | 不支持 |

参数：

| 参数 | 默认 | 说明 |
|------|------|------|
| `--ref <ref>` | 默认分支 | branch / tag / sha |
| `--subpath <dir>` | 自动发现 | 强制 SKILL.md 所在目录 |
| `--name <slug>` | 由目录派生 | 覆盖默认 slug |
| `--decision skip\|keep\|refine` | 无（交互询问） | 冲突默认决策（非交互必填） |
| `--enable` | false | 安装后立即启用（需 published） |
| `--publish` | false | 安装后发布 |
| `--keep-temp` | false | 保留 clone / zip 临时文件 |

#### 6.2.1 自动发现 SKILL 根目录

| 规则 | 优先级 |
|------|--------|
| 根目录存在 `SKILL.md` | 直接采用 |
| 仅一个一级子目录有 `SKILL.md` | 采用该子目录 |
| 多个子目录有 `SKILL.md` 且仓库无 `skills.json` / `pyproject.toml` 声明 | 交互模式让用户选；脚本模式 `--subpath` 必填 |
| 未发现 `SKILL.md` | 报 `SKILL_NOT_FOUND` → exit 5 |

#### 6.2.2 本地预校验

| 检查 | 失败行为 |
|------|----------|
| 存在 `SKILL.md` | 终止 |
| frontmatter 含 `name` / `description` | 终止 |
| 包内不含 `.git` / `node_modules` / `venv` / `*.dll` / `*.exe`；不超过单文件 50MB | 终止；`--allow-large` 强制 |
| zip 内总大小 ≤ 100MB（`[skill].max_zip_mb` 可调） | 终止 |

#### 6.2.3 上传与轮询

- 上传：multipart form 含 `file=<zip>` + 来源字段（`source` / `source_url` / `source_ref` / `source_subpath` / `client_validation`）；字段定义与写入位置见设计 §4.2.2；
- 轮询 `GET /v1/skills/import/{job_id}` 1.5s × 80 次（cap 120s）；
- 终态：`pass` 自动 apply；`warn` 询问或按 `--decision`；`block` 立即 `exit 5`；
- 超时回退：提示 `aranea skill import-status <job_id>` 与 `aranea skill import-apply <job_id> --decision ...`。

#### 6.2.4 失败回退

| 失败点 | 行为 |
|--------|------|
| URL 解析失败 | exit 1 + 候选 URL 示例 |
| clone 失败 | 删除临时目录；exit 3；私仓提示走 git credential helper |
| 本地预校验失败 | 不上传；`--keep-temp` 保留临时目录 |
| 上传失败 | 保留 zip 路径；提示 `aranea skill import <path>` 重试 |
| 轮询超时 | 提示手工查；不删 zip |
| apply 部分成功 | 打印 `created_skill_ids` / `skipped_candidate_ids` + 跳过原因 |

### 6.3 对话模式（US-10，P1）

UX 要点：

- 启动横幅一行：`Aranea CLI vX.Y.Z | 后端 <url> ✓ | Agent: 系统管家 | Session: new`；
- 提示符 `aranea> `；执行中切 `aranea⏵ ` + spinner；
- 输入：默认 `Enter` 发送；`Shift+Enter` 或行末 `\` 换行；`Ctrl+L` 清屏；`Ctrl+C` 在生成中=中断；在 prompt 阶段=取消（exit 4）；`Ctrl+D` 退出；
- 流式渲染按 WS 下行事件 type 分类渲染（模型增量追加文本、工具调用折叠块、工具结果 ✓、工具错误 ✗、等待用户回复弹 prompt、本轮结束回到提示符）；
  - 事件 type 定义与 payload schema 见设计 §4.2.5；数据流见设计 §5.3；

斜杠命令：

| 命令 | 作用 |
|------|------|
| `/help` | 命令列表 |
| `/agent <key>` | 切换 Agent（开新 session） |
| `/team <key>` | 切换 Team 编排（开新 session） |
| `/session new \| list \| resume <id>` | 会话管理 |
| `/model <provider>:<model>` | 临时切换模型 |
| `/tools` | 列当前 Agent 可用工具 |
| `/expand` | 展开上一个工具结果完整内容 |
| `/copy` | 复制上一条回复（跨平台剪贴板，失败时打印到 stderr 并提示手动） |
| `/dry-run on\|off` | 工具调用仅打印将要发送的 HTTP 请求，不真正执行 |
| `/yes` | 当前会话内跳过所有二次确认 |
| `/quit` / `/exit` | 退出 |

---

## 7. 配置与本地存储

### 7.1 配置项（用户视角）

CLI 配置文件为 TOML 格式，路径见 §7.3。用户可配置以下分区：

| 分区 | 作用 | 关键字段 |
|------|------|----------|
| `[backend]` | 后端连接 | `base_url`（后端地址）、`token`（JWT，自动 0600）、`workspace_id`（占位，本期无效） |
| `[ui]` | 输出 | `output`（`text` / `json`，P1 增 `yaml` / `table`）、`color`（`auto` / `always` / `never`） |
| `[skill]` | Skill 安装 | `default_decision`（`ask` / `skip` / `keep` / `refine`）、`max_zip_mb`（100）、`keep_temp`（false） |
| `[chat]`（P1） | 对话模式 | `default_agent`（`__system_admin__`）、`auto_resume`（true） |
| `[telemetry]` | 遥测 | `enabled`（false，对齐 24 telemetry） |

> 配置 schema 的 Go 类型定义与 TOML 序列化规则见设计 §3.3 / §8.2。

### 7.2 配置优先级

```
--flag  >  env (ARANEA_*)  >  config.toml  >  内置默认值
```

环境变量映射：`ARANEA_BASE_URL` / `ARANEA_TOKEN` / `ARANEA_OUTPUT` / `ARANEA_CONFIG` / `ARANEA_DEBUG` / `ARANEA_NO_COLOR`。

### 7.3 本地存储布局（用户视角）

| 用途 | 路径 | 说明 |
|------|------|------|
| 配置文件 | `$XDG_CONFIG_HOME/aranea/config.toml`（Linux/mac）；`%APPDATA%\aranea\config.toml`（Win） | 必须；权限 0600 |
| 日志 | `<UserCacheDir>/aranea/logs/cli-YYYY-MM-DD.log` | 按天切割，单文件 ≤ 5MB |
| 临时文件 | `<UserCacheDir>/aranea/tmp/<job_id>/` | Skill 安装临时目录，apply 成功后清理（除非 `keep_temp=true`） |

> **不存在** `~/.aranea/sessions/` 文件；CLI 不在本地落会话历史，对话历史以后端 `sessions/messages` 表为单一真相源。
> 跨平台路径解析的代码实现见设计 §8.1。

---

## 8. 安全、审计与边界

| 项 | 控制 |
|----|------|
| CLI 不允许任意 shell 执行 | 系统管家工具集**不**包含 `shell_exec`；用户需要时走 Web 控制台 + 高风险确认 |
| CLI 不直接读写工作区任意文件 | 只在 `~/.aranea/config.toml`、`<UserCacheDir>/aranea/{logs,tmp}/` 写入；Skill 安装临时目录 apply 后清理 |
| Token 落盘安全 | 文件 0600；CLI 启动时若发现 `>0644` 拒绝读 token 并提示 `chmod 600` |
| 危险动作 | 默认要求确认；`--yes` / `/yes` 仅当前进程会话内有效，重启失效；后端对 delete / channel send 等仍做 `confirm_key` 二次校验 |
| 审计 | 每次 `cli_admin_*` 调用（P1）经后端走标准 `tool_invocations` + `audit_logs`，标记 `source=cli`；CLI 不持有"绕过审计"能力 |
| Dry-run | `/dry-run on` 在对话模式生效（只打印请求不执行）；直接命令的 dry-run 由各 RPC 自带（如 proto 暴露 `dry_run` 字段），CLI 仅传递 |
| 远程风险升级 | 后端可根据工作区策略对单次调用升级风险（例如生产环境 channel send 强制人工） |

---

## 9. 验收准则汇总

> 本节是合规清单（验收标准）。**进度状态（✅/❌/⚠️）统一在开发计划 §8 跟踪**，本节不重复。

### 9.1 P0（必须）

- **R0** `make cli` 产出 `./bin/aranea`；`go build ./cmd/aranea/` 编译通过；
- **R1** `aranea version` 在无后端时也能输出本地版本与 commit；后端不可达 → exit 3 + 友好提示；
- **R2** `aranea login` 成功后 token 写入 `config.toml`，文件权限 0600（Win 跳过 chmod 仅提示）；
- **R3** `aranea agent ls / get / create / update / delete / enable / disable / tools / tools-set` 全部 happy path 通过 + httptest；
- **R4** `aranea skill ls / get / create / update / delete / enable / disable / publish` 同上；
- **R5** `aranea tool ls / get / enable / disable` 同上；
- **R6** `aranea system info` 显示后端版本 / commit / 默认 provider / `__system_admin__` agent_key；
- **R7** `--output json` 输出可被 `jq` 解析；`--quiet` 输出每行一个 ID；
- **R8** 删除 / 启停高风险动作无 `--yes` 时拒绝执行并提示；
- **R9** 401 / 403 → exit 6 + 重跑 login 提示；
- **R10** `aranea config path` 在 Win / mac / Linux 输出正确路径；旧 `config.toml` 缺字段时 CLI 不崩溃（用默认值）；
- **R11** `cmd/araneactl/lint` 新增 R12 黑名单生效（CLI 误 import biz/agent/server/service/trpc-agent-go 时 lint 失败）；
- **R12** `docs/guides/cli-quickstart.md` 落地，链接进 `docs/README.md`；

### 9.2 P1（应做）

- **R20** 后端 `__system_admin__` 种子存在（重启后端不重复创建）；
- **R21** 后端 `cli_admin_*` 首批工具注册成功；系统管家 Agent 单测能调用工具；
- **R22** WS 客户端能正确解码 5 类事件（`message.delta / tool.call / tool.result / tool.error / await.user.reply`）；
- **R23** `aranea` 启动进入 REPL；`/help / /quit` 工作；一次完整 skill install 对话流跑通；
- **R24** `aranea skill install <github-url>` happy path 成功；冲突 warn 时按 `--decision` 工作；
- **R25** 后端 multipart 接收 `source / source_url / source_ref / source_subpath / client_validation` 写入 `metadata_json`；`tool_invocations` / `audit_logs` 可见来源；
- **R26** `aranea skill import / import-status / import-apply` 全部 httptest 通过；
- **R27** `team / plugin / mcp / cron / channel / session / monitor` 每个资源至少 1 条 smoke；
- **R28** `aranea completion bash/zsh/powershell` 三种各跑一次；
- **R29** 后端剩余 `cli_admin_*` 工具（team/plugin/mcp/cron/channel/provider/session）单测覆盖；

### 9.3 安全与审计

- **R30** CLI 二进制不调用 `shell_exec` / `write_file` 等高危工具（在 P1 系统管家 Agent 工具集层面约束）；
- **R31** `__system_admin__` Agent 不可删除、不可改名；尝试删除返回 `READONLY_AGENT`；
- **R32** `--yes` / `/yes` 仅当前进程会话内生效；
- **R33** Skill 安装在 block 时立即 exit 5；非交互终端 + 无 `--decision` 遇 warn 也以 exit 5 退出；
- **R34** CLI 调用在 Web 控制台 `/tools/runs` 看到 `source=cli` 的记录（P1 系统管家工具触发）；

### 9.4 配置与可移植

- **R40** 跨平台编译三平台 amd64+arm64 全部通过（`make cli-all`）；
- **R41** `config.toml` 权限不安全时 CLI 拒绝读 token；
- **R42** 升级新版本旧 `config.toml` 兼容；
- **R43** 临时目录 `<UserCacheDir>/aranea/tmp/<job>/` 在 apply 成功后清理（除非 `keep_temp=true`）；Windows 文件句柄清理失败时 defer + 记录日志，不阻塞退出。

---

## 10. 待拍板问题（带候选与推荐）

| # | 问题 | 候选 | 推荐 |
|---|------|------|------|
| Q1 | CLI 二进制是否随后端同 release 打包？ | (a) 同 release 附三平台二进制；(b) 独立 release tag | **(a)**：版本严格对齐，避免客户端/服务端 API 漂移 |
| Q2 | REPL 行编辑库 | (a) `bufio.Scanner` 自绘；(b) `peterh/liner`；(c) `chzyer/readline` | **(b)**：纯 Go、跨平台、依赖少 |
| Q3 | `__system_admin__` 是否在 Web Agent 列表展示？ | (a) 隐藏；(b) 锁定展示（readonly） | **(b)**：避免 Web 用户疑惑"CLI 调用看不到 agent" |
| Q4 | `cli_admin_*` 工具是否允许其他用户 Agent 启用？ | (a) 后端硬编码白名单仅 `__system_admin__`；(b) 走 `tools_allow_json` 自由组合 | **(a)**：降低误用面，P2 再放开 |
| Q5 | 非交互 + warn 默认行为 | (a) 自动 skip；(b) exit 5 | **(b) exit 5**：CI 友好（明确失败） |
| Q6 | Skill 私仓凭据 | (a) 完全依赖 `git` credential helper；(b) CLI 配置中支持 PAT | **(a)** for MVP；(b) 推到 P2 |
| Q7 | `aranea init` 引导 | (a) 不做；(b) 首启检测后端 + 写默认 config + 可选拉一个 starter skill | **(a) for MVP**；P2 再做（避免首启时偷偷请求） |

---

## 11. 与其他模块的关系

| 模块 | 关系 |
|------|------|
| `1 chat.md` | 对话模式复用 chat 后端 WS / RPC；UI 从 Quasar 切换为终端折叠块 |
| `2-8 agent*` | `aranea agent *` 与 P1 系统管家 Agent 复用 Agent 模型与策略 |
| `11 multi-agent.md` | `aranea team *` 与 `aranea chat --team` 复用 Team 编排 |
| `19 mcp.md` | `aranea mcp *` 直接复用 MCP 表与 `/v1/mcp-servers` API |
| `20 skill.md` | §6.2 严格依赖 skill 导入 / 冲突组 / 炼化设计 |
| `21 cron.md` | `aranea cron *` 复用 `cron_task` |
| `22 plugin.md` | `aranea plugin *` 复用启停 / 排序；CLI / Web 审计一致 |
| `23 tools.md` | P1 新增 `cli_admin_*` 一组 Tool，扩展 `tool_invocations.source = cli` |
| `17 channel.md` | `aranea channel send` 受同样的高风险二次确认约束 |
| `18 monitor.md` | CLI 调用全部进入 audit / events / traces；监控页可按 `source=cli` 过滤 |
| `24 telemetry.md` | CLI 与框架共用 OTel 初始化 |
| `cmd/araneactl/` | 开发者 lint / fmtcheck，共存；新增 R12 黑名单守 CLI 红线 |

---

*文档版本：3.1 — 2026-06-06；与设计 [`25-cli.design.md`](./25-cli.design.md)、计划 [`25-cli.development.md`](./25-cli.development.md) 同步。若实施中发现仓库代码与本 PRD 假设不一致（特别是 WS envelope 子类型、Skill import multipart 字段、`/v1/admins/login` 响应体），以代码为准，并补一份 `docs/changelog/` 变更记录回写本 PRD。*
