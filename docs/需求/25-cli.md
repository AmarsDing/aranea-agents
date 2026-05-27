# Aranea CLI（终端控制台 + Agent 对话操作）

> ⚠️ **状态：SUPERSEDED（2026-05-27）**
> 本文档（v2.0）的产品需求已被取代，请改读：
> - 新需求：[`25-cli-PRD-2026-05-27.md`](./25-cli-PRD-2026-05-27.md)（v3.0）
> - 新设计：[`25-cli-design-2026-05-27.md`](./25-cli-design-2026-05-27.md)
> - 新开发计划：[`25-cli-development-plan-2026-05-27.md`](./25-cli-development-plan-2026-05-27.md)
> - 上层方案对账：[`25-cli-implementation-plan-2026-05-27.md`](./25-cli-implementation-plan-2026-05-27.md)
>
> 与新文档的关键差异（路径前缀、对话模式不在 CLI 进程构建 Runner、CLI 直接复用 pb 类型、复用 AwaitUserReply、删除 workspace_id 等）见新 PRD §1.1。
> 本文档保留作为历史，**不要再据此实施**。

---

本文档定义 **Aranea CLI** 的产品需求：命令体系、对话式系统管家、Skill 安装流程、配置 / 会话 / 审计 / 输出的落地方案。

CLI 的目标是把现有 Web 控制台（`/skills`、`/agents`、`/teams`、`/tools`、`/plugins`、`/mcp-servers`、`/cron`、`/channels`、`/monitor` 等）所有可执行能力，**都搬到终端**：既支持 `aranea skill install <url>` 这样的脚本化命令，也支持 `aranea` 进入交互式对话，由内置「系统管家 Agent」用自然语言完成跨模块操作。

典型场景：

> 用户在终端输入：
>
> ```
> $ aranea
> aranea> 帮我把 https://github.com/anthropic/skills/tree/main/figma-code-connect 这个 skill 装上
> ```
>
> 系统管家 Agent 调用 `skill_install_from_url` 工具：拉取仓库 → 定位 `SKILL.md` → 本地校验编写规范 → 打包 zip → 调用 `POST /api/v1/skills/import` → 轮询 `job_id` → 与用户确认冲突组（无冲突直接 apply，有相似冲突询问保留 / 跳过 / 炼化）→ apply 入库 → 反馈安装结果与新 Skill ID。

---

## 0. 需求结论

### 0.1 本期范围

| 模块 | 本期是否做 | 说明 |
|------|------------|------|
| CLI 二进制 `aranea` | 是 | 单一可执行文件，跨平台（Windows / macOS / Linux），由后端 Go 项目同源发布 |
| 直接命令模式 | 是 | `aranea <资源> <动作> [参数]`，覆盖 Skill / Agent / Team / Tool / Plugin / MCP / Cron / Channel / Session / Monitor / System |
| 对话模式（REPL） | 是 | `aranea` 或 `aranea chat` 进入；与内置「系统管家 Agent」对话，自然语言驱动管理工具 |
| 系统管家 Agent | 是 | 内置 Agent（`agent_key = __system_admin__`），固定 Profile、固定 Tool 集、不在普通列表展示 |
| 安装 Skill from URL | 是 | 支持 `github.com / gitlab.com / git+https / ssh / 子目录 / 原始 zip URL`，本地完成 clone + 子目录定位 + 打包 + 上传 + 轮询 + 冲突处理 |
| Agent 对话执行所有管理操作 | 是 | 通过新增 `cli_admin_*` 内置 Tool 集；所有动作走后端 REST API，CLI 不直接写数据库 |
| 配置 / 会话 / 历史本地存储 | 是 | `~/.aranea/`（Windows: `%APPDATA%\aranea\`），含 `config.toml`、`sessions/*.jsonl`、`logs/cli-*.log` |
| 输出格式切换 | 是 | 默认人类可读（带色 + 表格）；`--json` 输出结构化 JSON；`--quiet` 仅返回关键字段 |
| 远程后端 / 本地内嵌后端 | 是 | 默认连接 `http://127.0.0.1:8080`；可 `aranea --base-url https://aranea.example.com` 远程接入 |
| 自动补全 | 是 | 生成 PowerShell / Bash / Zsh / Fish 补全脚本 |
| 升级与版本检测 | 是 | `aranea version` / `aranea upgrade`（仅打印升级指引，不强制自更新） |
| Web 控制台同源登录 | 后续 | 本期默认单用户本地控制台，无需登录；远程后端接入预留 `aranea login` |
| 插件式第三方 CLI 命令 | 后续 | 不允许 CLI 加载任意外部代码 |

### 0.2 默认产品决策

| 决策项 | 默认值 |
|--------|--------|
| 二进制名 | `aranea`；备选短别名 `arn`（仅 PATH 别名，不重复实现） |
| 默认后端地址 | `http://127.0.0.1:8080`，可被环境变量 `ARANEA_BASE_URL` 覆盖，也可被 `--base-url` 覆盖 |
| 配置文件路径 | `$HOME/.aranea/config.toml`（Windows：`%APPDATA%\aranea\config.toml`） |
| 会话历史 | `$HOME/.aranea/sessions/<session_id>.jsonl`，与 Web 控制台 `sessions` 表关联 |
| 默认对话 Agent | `__system_admin__` 系统管家；可通过 `aranea chat --agent <key>` 切换为任意已存在 Agent |
| 默认 Team | 不预设；`aranea chat --team <key>` 可切换 |
| 输出主题 | 自动检测终端是否支持 ANSI 色；不支持时回退纯文本；可被 `NO_COLOR=1` 强制关闭 |
| 输出格式 | `text`（默认）/ `json` / `yaml` / `table`；通过 `--output` 切换 |
| 长任务进度 | WS 事件流或 HTTP 轮询驱动终端进度条；失败可继续追加日志 |
| 危险操作 | 高风险动作（`channel send`、删除资源、装入未签名 Skill 等）默认要求 `--yes` 或对话二次确认 |
| 系统管家 Agent 的工具集 | 见 §6；**不开放**普通业务工具（如 `web_search`、`tts`），只开放 `cli_admin_*` 管理工具 |
| 自动补全脚本 | `aranea completion <shell>` 输出补全脚本到 stdout |
| Telemetry | 默认关闭；可在 `config.toml` 开启匿名错误上报，对齐 `24 telemetry.md` |

### 0.3 角色与权限

当前产品为单用户本地控制台，CLI 默认拥有与 Web 控制台一致的全部能力。安全边界由后端按以下规则保证：

| 风险 | 控制方式 |
|------|----------|
| 删除资源 | CLI 必须显式 `--yes` 或对话内输入资源名；后端二次校验 |
| 启停高风险 Tool / Plugin | CLI 与对话模式都要求二次确认；调用 `PATCH /api/v1/tools/:id/enabled` 时携带 `confirm_key` |
| 写入文件 / 执行命令 | `cli_admin_*` 工具集**不**包含通用 `write_file` / `shell_exec`；如需写入由用户在 Web 端操作 |
| 安装 Skill | 默认进入 §5 的安全检查流程；未通过结构校验或名称重复时阻塞 |
| 远程后端接入 | 通过 `aranea login` 写入 token；token 仅落本地 `config.toml`，权限按目标后端策略 |
| 审计 | CLI 每次调用 admin tool 都通过后端走标准 `tool_invocations` 记录，UI 上可见来源 `source=cli` |

---

## 1. 入口策略

Aranea CLI 采用**双层入口**策略：

| 入口 | 类型 | 作用 |
|------|------|------|
| `aranea` | 默认进入对话模式 | 直接进入 REPL，与系统管家 Agent 对话 |
| `aranea chat [--session <id>] [--agent <key>] [--team <key>]` | 对话模式 | 显式进入对话模式，可指定 Agent / Team / Session |
| `aranea <资源> <动作> [...]` | 直接命令模式 | 脚本化 / 自动化操作，见 §2 |
| `aranea version` | 信息命令 | 打印 CLI 版本、git commit、后端可达性 |
| `aranea config <get/set/edit/path>` | 配置命令 | 管理本地配置 |
| `aranea login [--base-url ...]` | 登录命令 | 远程后端接入（本期占位） |
| `aranea completion <bash/zsh/fish/powershell>` | 补全命令 | 输出补全脚本 |
| `aranea --help` / `aranea <资源> --help` | 帮助 | 显示帮助信息 |

---

## 2. 资源 / 动作命令体系

约定：所有动作动词与后端 REST 一一对应。

| 资源 | 子命令（动作） | 对应后端 |
|------|----------------|----------|
| `agent` | `ls`、`get`、`create`、`update`、`delete`、`enable`、`disable`、`run`、`tools`、`tools-set` | `/api/v1/agents`、`/api/v1/agents/:id/tools/effective`、`/api/v1/agents/:id/tools/policy` |
| `team` | `ls`、`get`、`create`、`update`、`delete`、`run`、`runs`、`run-events` | `/api/v1/teams`、`/api/v1/team-runs`、`/api/v1/team-run-events` |
| `skill` | `ls`、`get`、`create`、`update`、`delete`、`enable`、`disable`、`publish`、`install <url>`、`import <zip-path>`、`import-status <job_id>`、`import-apply <job_id>` | `/api/v1/skills`、`/api/v1/skills/import` |
| `tool` | `ls`、`get`、`enable`、`disable`、`config-set` | `/api/v1/tools` |
| `plugin` | `ls`、`get`、`enable`、`disable`、`order-set`、`config-set` | `/api/v1/plugins` |
| `mcp` | `ls`、`get`、`add`、`update`、`delete`、`test` | `/api/v1/mcp-servers` |
| `cron` | `ls`、`get`、`add`、`update`、`delete`、`pause`、`resume`、`trigger` | `/api/v1/cron-tasks` |
| `channel` | `ls`、`get`、`add`、`update`、`delete`、`test`、`send` | `/api/v1/channels` |
| `session` | `ls`、`get`、`send` | `/api/v1/sessions`、`/api/v1/chat/messages` |
| `monitor` | `audit-logs`、`events`、`traces` | `/api/v1/monitor/audit-logs`、`/api/v1/monitor/events` |
| `system` | `info` | `/api/v1/system/info`（新增） |

### 2.1 通用 flags

| Flag | 全局 / 局部 | 说明 |
|------|-------------|------|
| `--base-url` | 全局 | 覆盖默认后端地址 |
| `--token` | 全局 | 覆盖配置文件中的 token |
| `--output` / `-o` | 全局 | 输出格式：`text`（默认）/ `json` / `yaml` / `table` |
| `--quiet` / `-q` | 全局 | 仅输出关键字段（ID、状态等） |
| `--yes` / `-y` | 局部（写操作） | 跳过二次确认 |
| `--page` / `--page-size` | 局部（列表命令） | 分页参数 |
| `--search` | 局部（列表命令） | 搜索关键词 |

---

## 3. 对话模式（REPL）

### 3.1 交互行为

```text
$ aranea
Aranea CLI v1.0.0 | 后端 http://127.0.0.1:8080 ✓ | Agent: 系统管家 | Session: new

Tip: 直接说人话；输入 /help 看命令；Ctrl+C 退出。

aranea> 帮我把 https://github.com/anthropic/skills 这个仓库里的 figma-code-connect 装上
```

| 区域 | 行为 |
|------|------|
| 启动横幅 | 一行：版本、后端地址、当前 Agent、当前 Session（若复用） |
| 提示符 | `aranea> `；执行中切换为 `aranea⏵ `（spinner 动画） |
| 输入 | 多行：默认 `Enter` 发送；`Shift+Enter` 或 `\` 行末换行；`Esc Esc` 取消正在输入；`Ctrl+L` 清屏 |
| 模型回复 | 流式渲染；引用块、代码块、Markdown 表格在终端中按 ANSI 色渲染 |
| 工具调用 | 每次 tool call 在终端显示折叠块：`▼ skill_install_from_url(url=…)`，工具结果默认折叠摘要，`/expand` 展开 |
| 用户确认 | 高风险动作进入交互气泡：`确认安装 figma-code-connect？(y/N)`，或下钻菜单选 `保留 / 跳过 / 炼化` |
| 结束执行 | 模型回复完后回到提示符；最近一次响应可用 `/copy` 复制到剪贴板 |

### 3.2 内置斜杠命令

| 命令 | 作用 |
|------|------|
| `/help` | 显示常用斜杠命令 |
| `/agent <key>` | 切换对话 Agent；`/agent default` 回到系统管家 |
| `/team <key>` | 切换为 Team 编排模式 |
| `/session new` | 开新会话 |
| `/session list` | 最近 20 个会话 |
| `/session resume <id>` | 切换到指定会话 |
| `/model <provider>:<model>` | 临时切换模型 |
| `/tools` | 列出当前 Agent 可用工具 |
| `/expand` | 展开上一条工具结果完整内容 |
| `/copy` | 复制上一条回复 |
| `/dry-run on/off` | 工具调用是否仅打印将要发送的 HTTP 请求 |
| `/yes` | 临时跳过本会话内的所有确认 |
| `/quit` | 退出 |

### 3.3 系统管家 Agent 的核心约定

| 项 | 值 |
|----|----|
| `agent_key` | `__system_admin__` |
| 显示名 | `系统管家` |
| 类型 | 内置 Agent，由后端 `SeedSystemAdminAgent()` 在启动时确保存在 |
| 是否在 Agent 列表展示 | 列表展示但锁定（`readonly=true`），不可删除、不可改名 |
| 默认模型 | 跟随当前 `default_provider` + `default_model`；可被 `/model` 临时覆盖 |
| `tools_profile` | `system_admin`（专用 profile，仅含 `cli_admin_*` 与必要的 `web_fetch` / `read_file`） |
| `tools_allow` | 仅 `group:cli_admin`、`web_fetch`、`read_file`、`datetime` |
| `tools_deny` | `shell_exec`、`write_file`、`create_image`、`tts` 等高风险 / 与系统管理无关的工具 |
| 是否可被普通 Chat 页选中 | 否；仅 CLI 默认会话 + Web 控制台「系统管理」入口可见 |

### 3.4 安装 Skill GitHub URL 的对话样例

```text
aranea> 帮我把 https://github.com/anthropic/skills/tree/main/figma-code-connect 装一下

▼ skill_install_from_url(url=…/figma-code-connect)
  step 1/6  解析 URL                       ✓ owner=anthropic repo=skills subpath=figma-code-connect ref=main
  step 2/6  下载子目录                     ✓ 18 个文件，263 KB
  step 3/6  本地预校验 SKILL 编写规范      ✓ 包含 SKILL.md、frontmatter 完整
  step 4/6  打包 zip                       ✓ /tmp/aranea/figma-code-connect-2026-04-26.zip
  step 5/6  上传到后端 (/api/v1/skills/import)  job_id=job_7f3
  step 6/6  轮询 job_7f3                   warn ⚠ 1 个相似冲突组

冲突组 group_01：相似度 42% (medium risk)
  候选：figma-code-connect            （本次上传）
  已有：Figma Code Connect v1.0.0     （4 月 25 日由 admin 创建）
  原因：Both describe mapping Figma components to code snippets

请选择处理方式：
  [1] 跳过该候选            （不入库，保留已有）
  [2] 强制保留两份          （warn 升级为接受）
  [3] 让我用 AI 炼化合并    （生成新草稿，需人工确认）

aranea> 3

▼ skill_refine_conflict_group(job_id=job_7f3, group_id=group_01)
  using model: openai/gpt-4.1
  ✓ 生成合并草稿（merged_name="Figma Code Connect", body 1.8KB）

预览（前 10 行）：
  # Figma Code Connect
  Creates and maintains Figma Code Connect template files that map …
  …

是否将合并结果作为新草稿入库？(y/N) y

▼ skill_import_apply(job_id=job_7f3, decisions=[merge_group_with_ai])
  ✓ 入库成功 → skill_id=skill_8a2  status=draft  enabled=false

已完成。提示：
  · 新 Skill 是 draft 状态，运行 `aranea skill publish skill_8a2` 即可发布。
  · 旧 Skill `figma-code-connect` 未被自动归档，可用 `aranea skill disable figma-code-connect` 停用。
```

要求：

1. 工具步骤必须**步进式输出**（不能等结果一次性吐出来），每一步带 ✓ / ⚠ / ✗ 状态。
2. 任何会变更系统状态的步骤（apply、覆盖、删除、停用、启用高风险）都需要二次确认，除非 `--yes` 或会话内 `/yes`。
3. 工具内部失败必须把 HTTP 状态码、`error.code`、`error.message` 完整展示，并给出可执行的修复指引。

---

## 4. Skill 从 URL 安装：核心流程

### 4.1 支持的 URL 形态

| URL 形态 | 处理方式 |
|----------|----------|
| `https://github.com/<owner>/<repo>` | 克隆默认分支根目录；要求根目录有 `SKILL.md`，否则按 §4.3 自动发现 |
| `https://github.com/<owner>/<repo>/tree/<ref>/<subpath>` | 克隆 `<ref>`，仅打包 `<subpath>` 子目录 |
| `https://github.com/<owner>/<repo>/blob/<ref>/<subpath>/SKILL.md` | 取 `SKILL.md` 所在目录为根 |
| `git@github.com:<owner>/<repo>.git` / `ssh://...` | 走 SSH，需要本地有可用 key |
| `https://gitlab.com/...` / `https://gitee.com/...` / `https://codeberg.org/...` | 同 GitHub 解析规则 |
| `https://example.com/path/skill.zip` | 直接下载 zip，跳过 clone |
| `npm:<pkg>` / `pypi:<pkg>` | 后续迭代，本期不支持 |

参数：

| 参数 | 默认 | 说明 |
|------|------|------|
| `--ref <branch/tag/sha>` | 仓库默认分支 | 锁定版本 |
| `--subpath <dir>` | 自动发现 | 强制指定 SKILL.md 所在目录 |
| `--name <slug>` | 由目录名派生 | 覆盖默认 slug |
| `--enable` | false | 安装后立即启用（需 `published`） |
| `--publish` | false | 安装后自动发布 |
| `--decision skip\|keep\|refine` | 进入冲突组时的默认决定 | 与 `--yes` 配合使用 |
| `--keep-temp` | false | 保留 clone / zip 临时文件用于排错 |

### 4.2 流程状态机

```text
parse_url
  └─> resolve_ref       (HEAD 或显式 ref)
        └─> fetch       (git clone --depth 1 / wget zip)
              └─> locate_skill_root   (§4.3)
                    └─> local_validate (§4.4)
                          └─> pack_zip
                                └─> POST /api/v1/skills/import
                                      └─> poll job (1.5s × 80 次)
                                            ├─ pass     → POST apply (import_passed)
                                            ├─ warn     → 询问 / 按 --decision 自动
                                            └─ block    → 报错退出 (exit 5)
```

### 4.3 自动发现 SKILL 根目录

当用户给的是仓库根，需要自动定位 `SKILL.md`：

| 规则 | 优先级 |
|------|--------|
| 根目录存在 `SKILL.md` | 直接采用 |
| 仅一个一级子目录有 `SKILL.md` | 采用该子目录 |
| 多个子目录有 `SKILL.md`，但仓库存在 `skills.json` / `pyproject.toml` 声明 | 用户必须 `--subpath` 选一个 |
| 未发现任何 `SKILL.md` | 报错 `SKILL_NOT_FOUND`，退出 5 |
| 多个 `SKILL.md` 且无声明 | 进入对话流程让 Agent 询问；脚本模式 `--subpath` 必填 |

### 4.4 本地预校验

CLI **不替代**后端校验，但本地先做一次轻量检查可以避免无效上传：

| 检查 | 失败行为 |
|------|----------|
| 存在 `SKILL.md` | 终止，返回路径建议 |
| `SKILL.md` frontmatter 必含 `name` / `description` | 终止 |
| 包内不含 `.git`、`node_modules`、`venv`、`*.dll`、`*.exe`、超过 50MB 单文件 | 拒绝并打印命中规则；可 `--allow-large` 强制 |
| zip 内总大小 ≤ 100MB（默认） | 拒绝；可在 `config.toml` 调高 `skill.max_zip_mb` |

### 4.5 冲突组交互

无冲突：直接 apply；输出新 Skill ID。

warn（相似度 ≥ 0.2）：

```text
冲突组 group_01：相似度 42% (medium risk)
  候选：figma-code-connect (本次上传)
  已有：Figma Code Connect v1.0.0
  评估：name=31%  description=58%  body=47%  trigger=52%  tool=66%  confidence=84%
  原因：两个 Skill 都在描述 Figma 组件与代码模板的映射流程。
  证据：Both mention Code Connect template files
        Both describe mapping Figma components to code snippets

请选择 (1-3，或 q 取消)：
  [1] skip       本次跳过该候选
  [2] keep       保留两份（警告升级为接受）
  [3] refine     调用 AI 炼化合并（默认模型 openai/gpt-4.1）
```

block：直接 exit 5 并附 block 原因表。

### 4.6 失败回退

| 失败点 | 回退策略 |
|--------|----------|
| `git clone` 失败 | 删除临时目录；报告网络 / 凭据错误 |
| 本地预校验失败 | 不上传，临时目录可 `--keep-temp` 保留 |
| 上传失败 | 临时 zip 保留路径，提示 `aranea skill import <path>` 可重试 |
| 轮询超时 | 提示 `aranea skill import-status <job_id>` 手工查询 |
| apply 部分成功 | 输出 `created_skill_ids` 与 `skipped_candidate_ids`，并解释跳过原因 |

---

## 5. 系统管家 Agent 的 Tool 集（CLI Admin Toolkit）

新增一组内置 Tool，统一前缀 `cli_admin_`，分类 `system`，源 `builtin`，对齐 `23 tools.md` 的 Tool 模型与 `tool_invocations` 审计。

### 5.1 通用约定

| 约定 | 说明 |
|------|------|
| 命名 | `cli_admin_<resource>_<action>`，例如 `cli_admin_skill_install_from_url` |
| 入口 | 全部走后端 REST，不在 CLI 端直接读写数据库 |
| 风险 | 默认 `medium`；删除 / 启停高风险工具 / 安装 Skill / 发送 channel 消息提升为 `high` |
| 参数 schema | 每个工具暴露 `parameters_schema`，必含 `idempotency_key`、`dry_run` 字段 |
| 系统注入字段 | `agent_id`、`session_id`、`source=cli` 由后端注入，模型不可控制 |
| 输出 | 成功返回业务对象 + `next_actions[]`（建议下一步命令）；失败返回 `error.code` / `error.message` |
| 长任务 | 创建 job 的 Tool 同时返回 `job_id`，并提供配套 `cli_admin_*_status` 工具供轮询 |

### 5.2 Tool 列表

| Tool Key | 风险 | 后端调用 | 说明 |
|----------|------|----------|------|
| `cli_admin_skill_list` | low | `GET /api/v1/skills` | 搜索 / 分页查询 Skill |
| `cli_admin_skill_install_from_url` | high | clone + `POST /api/v1/skills/import` + 轮询 | §4 主流程 |
| `cli_admin_skill_install_from_path` | high | `POST /api/v1/skills/import`（zip 流） | 本地路径 zip 安装 |
| `cli_admin_skill_import_status` | low | `GET /api/v1/skills/import/:job_id` | 轮询导入状态 |
| `cli_admin_skill_import_apply` | high | `POST /api/v1/skills/import/:job_id/apply` | 应用导入决策 |
| `cli_admin_skill_refine_conflict` | medium | `POST /api/v1/skills/import/:job_id/conflict-groups/:group_id/refine` | AI 炼化冲突组 |
| `cli_admin_skill_enable` / `_disable` | medium | `PATCH /api/v1/skills/:id/enabled` | 启停 |
| `cli_admin_skill_delete` | high | `DELETE /api/v1/skills/:id` | 软删 |
| `cli_admin_agent_list` / `_get` / `_create` / `_update` / `_delete` | low~high | `/api/v1/agents*` | Agent CRUD |
| `cli_admin_agent_tools_get` / `_set` | medium | `/api/v1/agents/:id/tools/effective` & `/policy` | Agent 工具策略 |
| `cli_admin_team_list` / `_create` / `_update` / `_delete` / `_run` | low~high | `/api/v1/teams*` | Team |
| `cli_admin_tool_list` / `_enable` / `_disable` / `_config_set` | medium~high | `/api/v1/tools*` | 全局 Tool |
| `cli_admin_plugin_list` / `_enable` / `_disable` / `_order_set` / `_config_set` | medium | `/api/v1/plugins*` | Plugin |
| `cli_admin_mcp_list` / `_add` / `_update` / `_delete` / `_test` | medium~high | `/api/v1/mcp-servers*` | MCP |
| `cli_admin_cron_list` / `_add` / `_update` / `_delete` / `_pause` / `_resume` / `_trigger` | medium | `/api/v1/cron-tasks*` | Cron |
| `cli_admin_channel_list` / `_add` / `_update` / `_delete` / `_test` / `_send` | high | `/api/v1/channels*` | Channel |
| `cli_admin_provider_list` / `_add` / `_update` / `_delete` / `_inspect` | medium | `/api/v1/llm-provider-models*` | LLM Provider |
| `cli_admin_session_list` / `_get` / `_send` | low~medium | `/api/v1/sessions*`、`/api/v1/chat/messages` | Session |

### 5.3 系统管家 Agent 的 Instruction 约束

```markdown
你是 Aranea 系统管家。你只通过 `cli_admin_*` 工具完成系统管理操作。

行为约束：
1. 永远不要伪造工具结果。如果用户要求的操作没有对应工具，明确说明「当前 CLI 不支持该操作」。
2. 任何写操作（create / update / delete / enable / disable / install / apply / send）都必须：
   - 先用 list / get 类工具确认目标存在；
   - 在执行前用一句话向用户复述将要发生的变更；
   - 涉及删除、覆盖、装入未签名 Skill 时显式询问确认。
3. 安装 Skill：
   - 输入是 git URL / 子目录 URL / zip URL 时，调用 `skill_install_from_url`；
   - 输入是本地路径 zip 时，调用 `skill_install_from_path`；
   - 收到冲突组（warn）时，向用户呈现「跳过 / 保留两份 / AI 炼化」三选一，并解释每个选项的后果；
   - 收到 block 时，**绝不**绕过；告诉用户原因。
4. 配置 Tool / Plugin / MCP / Cron / Channel 时，先用对应 list 工具读出当前配置，再 patch 需要修改的字段，避免覆盖未声明字段。
5. 失败时输出 `error.code`、`error.message`、对应 HTTP 路径与可能的修复命令；不要捏造解决方案。
6. 输出语言跟随用户的语言（默认中文）。
7. 你看不到用户终端，不能假设用户能直接看到很长的工具输出，必要时主动 summarize。
```

---

## 6. 安全、审计与边界

| 项 | 控制 |
|----|------|
| 不允许 CLI 直接执行任意 shell | `cli_admin_*` 工具集**不**包含 `shell_exec`；用户如需运行命令请走 Web 控制台 + 高风险确认 |
| 不允许 CLI 直接读写工作区任意文件 | CLI 只能在 `~/.aranea/` 与临时目录 `~/.aranea/tmp/<job>/` 写入；Skill 安装的 zip 在 apply 后清理 |
| 远程后端凭据 | `token` 落 `config.toml`，文件权限要求 `0600`；CLI 拒绝读取权限超过 0644 的 token 文件并提示 `chmod 600` |
| 危险动作 | 默认弹出确认；`--yes` 与 `/yes` 仅在当前会话内生效，重启后失效 |
| 审计 | 每次 `cli_admin_*` 调用通过后端走标准 `tool_invocations` + `audit_logs`；CLI 不持有「绕过审计」的能力 |
| Dry-run | 所有写操作工具都必须支持 `dry_run=true`，后端在 dry-run 时返回「将要发生的变更摘要」而不真正执行 |
| 风险升级 | 后端仍可对单次调用基于工作区策略升级风险，例如 channel 在生产环境强制人工确认 |

---

## 7. 输出与终端体验

| 场景 | 行为 |
|------|------|
| 无 TTY（管道 / CI） | 默认关闭色与进度条；`text` 输出降级为 `key=value` 单行；建议 CI 用 `--output json` |
| 大表格 | 超过终端宽度时优先截断中间列；`-o yaml` / `-o json` 输出全量 |
| Spinner | 所有阻塞 > 200ms 的操作显示 spinner；超过 5s 显示 `(已耗时 X 秒)` |
| 错误高亮 | `code` 高亮红 + 加粗，`hint` 黄色 |
| 链接 | 输出 Web 控制台对应路径（如 `查看：http://127.0.0.1:8080/skills/skill_8a2`） |
| 国际化 | 默认中文；`LANG=en_US.UTF-8` 或 `--lang en` 切换英文（本期可只做关键消息） |

---

## 8. 配置文件

`~/.aranea/config.toml`：

```toml
[backend]
base_url = "http://127.0.0.1:8080"
token    = ""
workspace_id = ""

[ui]
output  = "text"      # text | json | yaml | table
color   = "auto"      # auto | always | never

[skill]
default_decision = "ask"   # ask | skip | keep | refine
refine_provider  = ""      # 留空跟随后端 default
refine_model     = ""
max_zip_mb       = 100
keep_temp        = false

[chat]
default_agent  = "__system_admin__"
default_team   = ""
auto_resume    = true     # 启动时自动恢复上次会话

[telemetry]
enabled = false
```

---

## 9. 验收标准

### 9.1 直接命令

- `aranea skill ls` 能正确分页显示当前 Skill。
- `aranea skill install <github-url>` 能完成 §4 全流程；无冲突时不需任何交互即入库。
- `aranea agent tools-set <key> --allow ... --deny ...` 修改后 Web 控制台立即可见。
- `aranea --output json` 输出可被 `jq` 解析；`--quiet` 输出每行一个 ID。
- 删除 / 启停高风险动作在没有 `--yes` 时拒绝执行并提示。

### 9.2 对话模式

- `aranea` 直接启动进入对话；横幅展示后端可达性与 Agent 名称。
- 对话中工具调用以折叠块逐步出现，含 ✓ / ⚠ / ✗ 状态。
- 「装 GitHub 上的 figma-code-connect」语句能被识别并正确执行 §4 流程。
- 出现冲突组时能以菜单形式让用户选择 `skip / keep / refine`。
- 每次 `cli_admin_*` 调用都能在 Web 控制台 `/tools/runs` 看到一条 `source=cli` 的记录。

### 9.3 安全与审计

- CLI 不能调用 `shell_exec` / `write_file` 等高危 / 与系统管理无关工具。
- `__system_admin__` Agent 不可删除、不可改名；尝试删除返回 `READONLY_AGENT` 错误。
- `--yes` / `/yes` 仅在当前进程会话内生效。
- 安装 Skill 在 block 时立即 exit 5；未给 `--decision` 且非交互终端时遇到 warn 也以 exit 5 退出。

### 9.4 配置与可移植性

- `aranea config path` 在 Windows / macOS / Linux 输出正确路径。
- `config.toml` 的 `token` 文件权限不安全时 CLI 拒绝读取并打印修复建议。
- 升级到新版本后旧 `config.toml` 仍兼容（缺字段使用默认值）。

---

## 10. 待确认问题

1. CLI 二进制是否随后端打包发布，还是单独 release？
2. 远程后端接入是否本期就引入 token / OIDC 登录？目前后端是否有用户体系？
3. `__system_admin__` Agent 是否在 Web 控制台 Agent 列表展示？还是隐藏在「系统」分类下？
4. `cli_admin_*` 工具是否允许其他自建 Agent 通过修改 `tools_allow_json` 启用？建议默认拒绝，需要后端硬编码白名单。
5. Skill 安装的临时目录 `~/.aranea/tmp/` 在 apply 成功后是否自动清理？默认清理还是保留 7 天？
6. CI 模式（`CI=true` 或非 TTY）下 `aranea skill install <url>` 默认行为：遇到 warn 应该 `skip` 还是 `exit 5`？本文默认 `exit 5`。
7. 对话模式的多 Agent 切换：`/agent <key>` 是开新 session，还是同一 session 内仅改变下一条消息的执行 Agent？建议开新 session 以避免历史污染。
8. Skill 来自 git 私仓时的凭据如何管理？`git` 自身的 credential helper 是否足够？
9. 是否提供一个低门槛的 `aranea init` 引导（首次启动检测后端 / 配置默认 provider / 安装初始 Skill 包）？

---

## 11. 与现有模块的关系

| 模块 | 关系 |
|------|------|
| `1 chat.md` | 对话模式复用 chat 后端；UI 形态从 Quasar `QChatMessage` 切换为终端折叠块 |
| `2-8 agent*` | `aranea agent *` 直接命令与 §5 工具复用 Agent 模型与策略 |
| `11 multi-agent.md` | `aranea team *` 与 `aranea chat --team` 复用 Team 编排 |
| `19 mcp.md` | `aranea mcp *` 直接复用 MCP 表与 `/mcp-servers` API |
| `20 skill.md` | §4 安装链路严格依赖 `20 skill.md` 的导入 / 冲突组 / 炼化设计 |
| `21 cron.md` | `aranea cron *` 复用 `cron_task` |
| `22 plugin.md` | `aranea plugin *` 复用插件启停与排序；CLI / Web 审计一致 |
| `23 tools.md` | §5 新增 `cli_admin_*` 一组 Tool，并扩展 `tool_invocations.source = cli` |
| `17 channel.md` | `aranea channel send` 受同样的高风险二次确认约束 |
| `18 monitor.md` | CLI 调用全部进入 audit / events / traces；监控页可按 `source=cli` 过滤 |
| `24 telemetry.md` | CLI 与框架共用 OTel 初始化 |

---

*文档版本：2.0 — 需求与设计分离；需求文档聚焦产品规格、用户故事、验收标准；技术设计见 `25 cli.design.md`。*
