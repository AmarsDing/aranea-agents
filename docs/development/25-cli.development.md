# 25 Aranea CLI — 开发计划（Dev Plan, 2026-05-27）

> **版本**：3.1（取代 `25-cli-development.md` v2.0）
> **同系列**：需求 → [`25-cli.md`](./25-cli.md)；设计 → [`25-cli.design.md`](./25-cli.design.md)；上层方案 → [`25-cli-implementation.md`](./25-cli-implementation.md)
> **任务 ID 约定**：`CLI-XX` 前缀；每条 ≤1 天（≤8h），含明确 DoD 与可执行验收信号；多步动作拆分到 `CLI-XX.N` 子任务。
> **执行者**：AI agent / 工程师；每个任务都给出"AI 落地提示词"（在 §6 集中），可直接喂给 cursor-agent / claude code。

---

## 0. 文档导读

读取顺序：

1. §1 现状 → 确认起点；
2. §2 阶段总览 → 选择当前要做的 phase；
3. §3 任务表（CLI-01..30）→ 锁定要做的任务；
4. §4 依赖图 → 排顺序；
5. §5 风险登记 → 知道踩什么坑；
6. §6 AI 落地提示词模板 → 让 AI 一次拿到完整上下文；
7. §7 PR 模板 / §8 验收 checklist → 收尾。

不要先看任务表再倒推 PRD / 设计：本计划的每条任务都假设你已经读过 PRD §6 关键流程与设计 §2~§7。

---

## 1. 现状

| 项 | 状态 | 证据 |
|----|------|------|
| `cmd/aranea/` | **已实现** | `cmd/aranea/main.go` 存在，完整 cobra CLI 入口 |
| `internal/cli/` | **已实现** | 7 子包：`client/` `clierr/` `cmd/` `config/` `output/` `repl/` `ui/` |
| `cobra` 主依赖 | **已引入** | `go.mod` 含 `github.com/spf13/cobra v1.10.2` |
| `internal/tools/cli_admin/` | **已实现** | 含 registry + agent_tools + skill_install + pkg_install |
| `__system_admin__` agent 种子 | **已实现** | `internal/data/seed_system_admin.go` 含 SeedSystemAdminAgent 等 8 个 seed 函数 |
| `GET /v1/system/info` | **已实现** | `internal/service/system_info.go` + 手动注册路由；但字段与设计有差距（缺 `system_admin_agent_id/key`、`skill_max_zip_mb`） |
| Skill import multipart 来源字段 | **待确认** | `internal/service/skill_import.go` 待 review |
| `pkg/auth.Auth` | `UserID + Access` 两字段 | `pkg/auth/auth.go:10-13` |
| `/v1/admins/login` 路径 | 已存在并放 noAuthPaths | `api/kratos/admin/v1/admin.proto:50-55`、`pkg/auth/middleware.go:17` |
| WS envelope 类型 | **部分实现** | `internal/event/contract/envelope.go`：`tool_call` / `tool_result` 已定义；`tool.error` 未定义（使用通用 `error` 类型） |
| Skill proto 既有 RPC | `import / import-status / import-apply / refine-conflict-group` 已 proto-first | `api/kratos/skill/v1/skill.proto` |
| Chat `AwaitUserReply` / `EnqueueUserMessage` | 已 proto-first | `api/kratos/chat/v1/chat.proto:223,229` |
| `cmd/araneactl/lint` 规则 | **已实现** | `cmd/araneactl/lint/main.go` 含 R12 黑名单检查（`r12CLINoBackendImport`）；配套 `r12_test.go` + `testdata/r12_violation.go.txt` |
| `docs/guides/cli-quickstart.md` | **已创建** | 文件存在；`docs/README.md` 索引文件不存在（仓库无顶层 docs/README） |
| `monitor` 命令 | **已实现** | `cmd/aranea/main.go` 已注册 `NewMonitorCmd`；`internal/cli/cmd/monitor.go` 提供 `audit-logs`/`events`/`traces` 子命令；`internal/cli/client/monitor.go` 调用 `/v1/monitor/*` RPC（CLI-27 全部 7 类资源已实现） |
| `make cli` target | **已实现** | Makefile 含 `cli` / `cli-all` target |
| REPL | **已实现** | `internal/cli/repl/` 含 repl.go / slash.go / render.go / history.go |
| 额外命令（实施中新增） | **已实现** | `graph` / `pkg` / `import` / `pack` 命令已存在 |
| `--timeout` 全局 flag | **已实现** | `cmd/aranea/main.go:160` 注册为 PersistentFlag，默认 60s |
| 配置错误脱敏 | **已实现** | `internal/cli/config/config.go::sanitizeConfigError`：配置解析错误信息经 `preview.RedactAndTruncate` 脱敏，防止 API key/token 泄漏到错误输出（2026-07-20 Grok Build 借鉴） |

> 结论：P0 核心任务（CLI-01~13）已基本完成（R12 lint 已上线、quickstart 已落地）；P1 部分任务（CLI-20/21/22/23/27）已实现（CLI-27 全部 7 类资源完成）；主要缺口为 `tool.error` envelope、`SystemInfoResponse` 字段补齐、CLI-28/29/30。

### 1.1 后端契约（BE-1~BE-6）实现状态

> 设计文档 §4.2 定义的目标契约；本表跟踪代码实际实现状态（从设计文档迁移，避免设计文档混入进度信息）。

| ID | 接口 | 状态 | 证据 / 差距 |
|----|------|------|------------|
| BE-1 | `GET /v1/system/info` | ⚠️ 部分实现 | `internal/service/system_info.go` + `internal/server/http.go` 手动注册路由；**缺** `system_admin_agent_id` / `system_admin_agent_key` / `skill_max_zip_mb` 字段 |
| BE-2 | `POST /v1/skills/import` 接收 multipart 来源字段 | ❌ 未实现 | `internal/service/skill_import_http.go` 仅解析 `file` 字段；未解析 `source` / `source_url` / `source_ref` / `source_subpath` / `client_validation` |
| BE-3 | `SeedSystemAdminAgent()` 注入 `__system_admin__` | ✅ 已实现 | `internal/data/seed_system_admin.go::SeedSystemAdminAgent`（`readonly=1`, `kind=system`, `tools_profile=system_admin`）；含 SeedSpiritAgent/SeedMemoryAgent/SeedSkillsAgent/SeedBuiltinCLIAdminTools |
| BE-4 | `SeedBuiltinTools()` + `SeedToolGroups()` 注册 `cli_admin_*` | ✅ 已实现 | `internal/data/seed_system_admin.go::SeedBuiltinCLIAdminTools` 注册 `cli_admin_*` 工具记录 |
| BE-5 | `internal/tools/cli_admin/` 工具实现 | ⚠️ 部分实现 | 已实现：`skill_list` / `skill_install_from_url` / `pkg_install_from_url` / `agent_list` / `agent_get`；**待实现**：`skill_get` / `skill_import_status` / `skill_import_apply` / `team_*` / `plugin_*` / `mcp_*` / `cron_*` / `channel_*` / `provider_*` / `session_*` |
| BE-6 | WS 下行 envelope 细化 | ⚠️ 部分实现 | `internal/event/contract/envelope.go`：`tool_call` / `tool_result` 已定义；**`tool.error` 未定义**（使用通用 `EnvelopeTypeError = "error"`）；`message.delta` 实际为 `EnvelopeTypeTextDelta = "text_delta"` |

---

## 2. 阶段总览

| 阶段 | 周数 | 任务区间 | 退出条件 |
|------|------|----------|----------|
| **P0 MVP** | 1 周 | CLI-01..13 | `./bin/aranea` 对真实 admin 后端能正确登录 + agent/skill/tool 全套 CRUD；R12 黑名单 lint 上线；编译/单测/lint 全绿 |
| **P1 对话 + 安装** | 2~3 周 | CLI-20..30 | 系统管家 Agent 上线 + WS 流式 + skill install from URL happy path 跑通 + 剩余 7 类资源全套 + completion 脚本 |
| **P2 体验优化** | 按需 | CLI-40..46 | 由产品决定取舍 |

P0 与 P1 之间存在硬依赖：CLI-20（系统管家 Agent 种子）必须先于 CLI-22/23/24。

---

## 3. 任务表

### 3.1 任务表字段约定

| 字段 | 含义 |
|------|------|
| ID | `CLI-XX[.N]` |
| 标题 | 一句话动名词 |
| 工种 | `CLI`（终端二进制）/ `BE`（后端） |
| 估时 | 人天（半天=0.5；上限 1 天） |
| 依赖 | 必须先完成的任务 ID |
| 修改 / 新增 | 涉及的文件 / 目录 |
| DoD | "Definition of Done" — 验收的可执行信号 |
| 关联 | PRD 用户故事 / 设计章节 / 验收准则 R# |

> 工种 CLI = 不需要碰后端代码；工种 BE = 必须改后端；两栏都列时表示同任务跨端。

---

### 3.2 Phase P0（MVP，1 周）

#### CLI-01 创建 CLI 入口骨架（cobra root + Execute） ✅ 已完成
- **工种**：CLI
- **估时**：0.5
- **依赖**：—
- **新增**：
  - `cmd/aranea/main.go`（≤30 行；声明 `Version` / `Commit`；调 `internal/cli.Execute`）
  - `internal/cli/execute.go` / `root.go` / `ctx.go` / `buildinfo.go` / `exit.go`
  - 主 `go.mod` 新增 `github.com/spf13/cobra`
- **DoD**：
  - `go build ./cmd/aranea/` 通过；
  - `./bin/aranea --help` 显示顶层命令（`version / login / config / system / agent / skill / tool / help / completion`）；
  - `./bin/aranea` 无参数时打印"对话模式（REPL）将在 P1 上线，请使用子命令"并 exit 0；
  - `internal/cli/execute_test.go`：root.RunE 返回特定 error 时 ExitCodeOf 正确。
- **关联**：设计 §3.1 / §3.2 / §14.1；PRD §3.3

#### CLI-02 Config 包（CLIConfig + 跨平台路径 + 权限校验） ✅ 已完成
- **工种**：CLI
- **估时**：1
- **依赖**：CLI-01
- **新增**：
  - `internal/cli/config/{config.go, paths.go, secret.go, config_test.go}`
  - 新增依赖 `github.com/pelletier/go-toml/v2`
- **DoD**：
  - `Load(path)` 在文件不存在时返回默认配置（不报错）；
  - `Save(path)` 写文件后权限 0600（Linux/mac 测试断言；Win 测试断言"不报错 + 打印 warning"）；
  - `Load(path)` 检测到 token 字段所在文件权限 > 0600 时返回 `*CLIError{Code:"INSECURE_CONFIG_PERM"}` 且不暴露 token 值；
  - `aranea config path` / `config get base_url` / `config set base_url <v>` 工作；
  - `config get backend.token` 默认输出 `***<last4>`；`--show-token` 输出全文（stderr 警告）。
- **关联**：设计 §3.3 / §8；PRD §7 / R10 / R41

#### CLI-03 HTTP Client（Bearer / UA / 重试 / --debug） ✅ 已完成
- **工种**：CLI
- **估时**：1
- **依赖**：CLI-01
- **新增**：
  - `internal/cli/client/{http.go, retry.go, client_test.go}`
- **DoD**：
  - `Client.Do(ctx, GET, path, nil, *SomeResp)` 通过 `httptest` 验证 Header 含 `Authorization: Bearer <token>`、`User-Agent: aranea/...`、`Accept: application/json`；
  - 幂等 GET 在 5xx / 网络 error 时指数退避重试 3 次（200ms→2s）；写动作不重试；
  - `--debug` 时 stderr 输出请求 method/path/headers（Authorization mask）+ body 前 1KB；
  - 响应 4xx/5xx 时返回 `*CLIError`，404 → `Code=NOT_FOUND`、401 → `Code=UNAUTHENTICATED`（按 Kratos `reason` 透传）。
- **关联**：设计 §3.4 / §6 / §14.2

#### CLI-04 错误解码与退出码 ✅ 已完成
- **工种**：CLI
- **估时**：0.5
- **依赖**：CLI-03
- **新增**：
  - `internal/cli/client/errors.go`
  - `internal/cli/exit.go`（CLIError 类型 + ExitCodeOf）
  - `internal/cli/client/errors_test.go`（golden）
- **DoD**：
  - 单测覆盖 400 / 401 / 403 / 404 / 409 / 500 / 网络 error 共 7 类；
  - 退出码映射符合 PRD §5.2；
  - `ExitCodeOf(USER_CANCELED) == 4`；`ExitCodeOf(SKILL_IMPORT_BLOCKED) == 5`；`ExitCodeOf(401/403) == 6`。
- **关联**：设计 §6；PRD §5.2

**实施偏差**：错误类型从 `internal/cli/exit.go` 的 `CLIError` 改为独立包 `internal/cli/clierr/` 的 `Error`，避免循环依赖。

#### CLI-05 Output / Printer（text + json + TTY 检测 + golden） ✅ 已完成
- **工种**：CLI
- **估时**：1
- **依赖**：CLI-01
- **新增**：
  - `internal/cli/output/{printer.go, text.go, json.go, kv.go, output_test.go}`
  - `internal/cli/ui/{tty.go, color.go, spinner.go, table.go, prompt.go}`
  - 新增依赖：`github.com/mattn/go-isatty` / `github.com/fatih/color` / `github.com/olekukonko/tablewriter`
- **DoD**：
  - `--output json` 输出可被 `jq` 解析；
  - 非 TTY 自动退化 key=value；
  - `--quiet` 时 list 每行一个 ID；
  - golden 测试：`agent_ls_text_tty.golden` / `agent_ls_text_pipe.golden` / `agent_ls_json.golden` / `error_skill_blocked_text.golden` / `error_skill_blocked_json.golden`；
  - `UPDATE_GOLDEN=1 go test ./internal/cli/output/...` 能刷新 golden。
- **关联**：设计 §3.5 / §10.2；PRD §5

#### CLI-06 `aranea version`（含 reachability 探测） ✅ 已完成
- **工种**：CLI
- **估时**：0.5
- **依赖**：CLI-01..05
- **新增**：
  - `internal/cli/cmd/version.go`
- **DoD**：
  - 无后端时 `aranea version` 仍输出 `aranea vX.Y.Z (commit=...)`，并打印"backend: unreachable (<url>)"，但 exit 0；
  - 有后端时输出 `backend: <url> ✓ <backend_version>`（探测 `GET /` 或对照 `cmd/admin` 既有 health/ready 端点，**不**用 `/v1/system/info` 做匿名探测，避免 401）；
  - 网络明显失败时 exit 3。
- **关联**：PRD US-01 / R1

#### CLI-07 后端新增 `GET /v1/system/info` ✅ 已完成（有差距）
- **工种**：BE
- **估时**：1
- **依赖**：—（可与 CLI-01..05 并行）
- **新增 / 修改**：
  - `internal/service/system_info.go`（手动 HTTP handler，非 proto 生成）+ `internal/server/http.go` 注册路由
- **DoD**：
  - `curl http://127.0.0.1:8080/v1/system/info -H "Authorization: Bearer <token>"` 返回 JSON，含 `version / git_commit / build_time / default_provider / default_model / system_admin_agent_id / system_admin_agent_key / skill_max_zip_mb / skill_storage_root / features`；
  - `system_admin_agent_*` 在 P0 阶段（未 seed）允许返回空串；
  - 单测：未鉴权请求返回 401；
  - **不**改 `noAuthPaths`。
- **关联**：设计 §4.2.1；PRD BE-1

**差距**：缺少 `system_admin_agent_id` / `system_admin_agent_key` / `skill_max_zip_mb` 字段，需后续补齐。

#### CLI-08 `aranea login` 实现 ✅ 已完成
- **工种**：CLI（+少量代码考古 A1）
- **估时**：1
- **依赖**：CLI-02, CLI-03, CLI-04
- **新增**：
  - `internal/cli/cmd/login.go`
  - `internal/cli/client/admin.go`
- **DoD**：
  - 先读 `internal/service/admin.go` + `pkg/auth/cookie.go` 确认 `/v1/admins/login` 返回 token 的实际位置（body 字段 / Set-Cookie / header）；在 PR 描述中显式记录"假设 A1 验证结果"；
  - 登录成功后 token 落 `[backend].token`，文件 0600；
  - 失败：4xx → exit 2 + 错误码；5xx / 网络 → exit 3；
  - stdout 仅打印 `token saved to <path>` + 用户名；不打印 token；
  - `--debug` 模式日志中 token mask 为 `***<last4>`；
  - 后续 `aranea agent ls` 不再 401。
- **关联**：PRD US-01 / R2 / R9；设计 §3.2 / §14.4 / §15 A1

#### CLI-09 `aranea agent` 全套子命令 ✅ 已完成
- **工种**：CLI
- **估时**：1（建议拆 CLI-09.1 ls/get、CLI-09.2 create/update、CLI-09.3 delete/enable/disable/tools/tools-set）
- **依赖**：CLI-03, CLI-05
- **新增**：
  - `internal/cli/cmd/agent.go`
  - `internal/cli/client/agent.go`
  - `internal/cli/cmd/agent_test.go`（每个子命令 1 条 httptest）
- **DoD**：
  - 9 个子命令各跑通一次 manual smoke（对真实 admin 后端）；
  - `--file agent.yaml` 支持 yaml/json 两种格式（yaml 先转 json，再 `protojson.Unmarshal` 到 pb）；
  - `update` 自动生成 FieldMask（按 `--file` 中提供的字段）；
  - `delete` 无 `--yes` 时拒绝 + 提示；
  - `tools-set` 接受 `--allow x,y --deny z`，CLI 内 join 后 PUT；
  - `enable`/`disable` 实施前对照 proto（若无独立 RPC 则调 `UpdateAgent` + FieldMask `enabled`，在 PR 注明）。
- **关联**：PRD US-02..05；设计 §14.3 / §15 A5

#### CLI-10 `aranea skill` 全套子命令（不含 install） ✅ 已完成
- **工种**：CLI
- **估时**：1
- **依赖**：CLI-03, CLI-05
- **新增**：
  - `internal/cli/cmd/skill.go`
  - `internal/cli/client/skill.go`
  - `internal/cli/cmd/skill_test.go`
- **DoD**：
  - 8 个子命令（ls/get/create/update/delete/enable/disable/publish）各跑通；
  - `publish` 高风险二次确认。
- **关联**：PRD US-04

#### CLI-11 `aranea tool` 子命令 ✅ 已完成
- **工种**：CLI
- **估时**：0.5
- **依赖**：CLI-03, CLI-05
- **新增**：
  - `internal/cli/cmd/tool.go`
  - `internal/cli/client/tool.go`
  - 配套 httptest
- **DoD**：4 个子命令均通过；`enable`/`disable` 二次确认。
- **关联**：PRD US-04

#### CLI-12 Makefile target + lint R12 黑名单 ✅ 已完成
- **工种**：CLI
- **估时**：1
- **依赖**：CLI-01..11（建议在所有 CLI 业务文件落地后引入 R12，避免误伤）
- **修改 / 新增**：
  - `Makefile`：新增 `cli` / `cli-all` target；
  - `cmd/araneactl/lint/main.go`：新增 `r12CLINoBackendImport()`（与现有 R1/R2 同模式：AST 扫 import），并在 main 中调度；
  - `cmd/araneactl/lint/r12_test.go`（构造一个"故意违例"的 fixture 文件，断言 lint 失败）。
- **DoD**：
  - `make cli` 产出 `./bin/aranea`；
  - `make cli-all` 产出 6 个二进制（win/mac/linux × amd64/arm64）；
  - `make lint` 通过；
  - 手工构造一个 `internal/cli/cmd/_bad.go` 引入 `internal/biz` → `make lint` 失败；删除后通过。
- **关联**：设计 §7.3；PRD R11 / R40

✅ `make cli` 已完成；✅ R12 lint 规则已实现（`cmd/araneactl/lint/main.go::r12CLINoBackendImport` + `r12_test.go` + `testdata/r12_violation.go.txt`）；⚠️ `cli-all` 仅覆盖 Linux/amd64（Makefile `cli-all` target 仅编译 `GOOS=linux GOARCH=amd64`）

#### CLI-13 文档 `docs/guides/cli-quickstart.md` ✅ 已完成（有残留）
- **工种**：CLI
- **估时**：0.5
- **依赖**：CLI-01..12
- **新增**：
  - `docs/guides/cli-quickstart.md`（10 分钟上手：装 → login → agent ls → skill ls → 配置）
  - 修改 `docs/README.md` 索引；
- **DoD**：用户照文档复制粘贴可端到端跑通 5 个命令；中文 markdown ≤ 200 行。
- **关联**：PRD R12

✅ `docs/guides/cli-quickstart.md` 已创建；⚠️ `docs/README.md` 索引文件不存在（仓库无顶层 docs/README，无法完成"链接进索引"部分）

#### **P0 退出条件（必须全部满足）**

- [x] CLI-01..13 ✅ 已完成（CLI-12 R12 lint 已上线；CLI-13 quickstart 已落地；残留：`cli-all` 仅 Linux/amd64、`docs/README.md` 索引缺失）
- [x] `./bin/aranea agent ls` 对真实 `cmd/admin` 后端可正确分页；
- [x] `make lint` / `make test` / `go build ./cmd/aranea/` 全绿；
- [x] PRD §9.1 的 R0..R12 全绿（R11 R12 lint 已实现；R12 quickstart 已创建）；
- [ ] 在 `docs/changelog/2026-XX-XX-CLI-P0-MVP.md` 写一份变更说明，列入 `make ci` 不收紧（不强制 `make cli`）。

> **差距汇总**：CLI-07 缺少 `system_admin_agent_id` / `system_admin_agent_key` / `skill_max_zip_mb` 字段；`cli-all` 仅覆盖 Linux/amd64；`docs/README.md` 索引文件不存在。

---

### 3.3 Phase P1（对话 + Skill 安装 + 剩余资源，2~3 周）

#### CLI-20 后端 `__system_admin__` 种子 + 工具组种子 ✅ 已完成
- **工种**：BE
- **估时**：1
- **依赖**：CLI-07（依赖 system_info 已上线，便于在 `system_info` 返回 `system_admin_agent_id`）
- **新增 / 修改**：
  - `internal/data/ent/schema/agent.go`：新增 `readonly` (bool) / `kind` (enum) 字段
  - `internal/data/seed_system_admin.go`：upsert agent_key `__system_admin__`
  - `internal/data/seed_builtin_tools.go`：upsert `cli_admin_*` 工具记录（不带实现，占位）
  - `internal/data/seed_tool_groups.go`：建 `group:cli_admin`
  - 在 `cmd/admin` 启动流程调上述三个 seed
  - 迁移：`internal/data/sql/agent_readonly_kind.sql`（或 Atlas / Ent 迁移）
- **DoD**：
  - 重启后端，DB 中存在 `agent_key = "__system_admin__"`，`readonly=1`, `kind="system"`, `tools_profile="system_admin"`；
  - 重复启动 idempotent；
  - 删除该 agent 的 RPC 返回 `READONLY_AGENT` 错误（在 `internal/biz/agent.go::Delete` 处加 guard）；
  - `system_info` 返回的 `system_admin_agent_id / key` 非空。
- **关联**：设计 §4.2.3；PRD BE-3 / R20 / R31

#### CLI-21 后端 `cli_admin_*` 首批工具实现 ✅ 已完成
- **工种**：BE
- **估时**：1
- **依赖**：CLI-20
- **新增**：
  - `internal/tools/cli_admin/registry.go`
  - `internal/tools/cli_admin/skill_list.go` / `skill_get.go` / `skill_install_from_url.go` / `skill_import_status.go` / `skill_import_apply.go` / `agent_list.go` / `agent_get.go`
  - `internal/tools/cli_admin/registry_test.go`：模拟 system_admin Agent 调用工具
  - `internal/service`：在装配 system_admin Agent 时通过 `cli_admin.RegisterAll(deps)` 注入
- **DoD**：
  - 后端单测：构造一次最简 Agent + Runner，给 system_admin Agent 喂消息 `"list skills"`，断言调用了 `cli_admin_skill_list` 且响应非空；
  - 白名单：`IsCLIAdminAllowed(agentKey)` 在非 `__system_admin__` Agent 装配时拒绝加载 `group:cli_admin`。
- **关联**：设计 §4.2.4；PRD BE-5 / R21

#### CLI-22 CLI WS Client + envelope 编解码 + WS envelope 后端细化 ⚠️ 部分完成
- **工种**：BE + CLI
- **估时**：1.5（建议拆为 CLI-22.1 后端细化 0.5d、CLI-22.2 CLI WS 客户端 1d）
- **依赖**：CLI-01..05
- **新增 / 修改**：
  - 【BE】`internal/service/chat_wire.go`：把 ADK Event 投影到 WS envelope 时填充稳定 type（`tool.call / tool.result / tool.error / message.delta`），payload schema 见设计 §4.2.5
  - 【CLI】`internal/cli/client/ws.go`：Dial / Send / readPump / 关闭；事件解码为 `Envelope` 结构体
  - 【CLI】`internal/cli/client/ws_test.go`：起一个 mock WS server 推 5 类事件 + done，断言 CLI 全部正确解码
- **DoD**：
  - mock WS server 推 5 类事件 + done，CLI 解码 100% 正确；
  - 真实后端：手工跑 `/v1/ws`，浏览器 / `websocat` 看到 type 与 CLI 期望一致；
  - **代码考古 A2 完成**（WS token 怎么带），结果回写本任务的 PR。
- **关联**：设计 §4.2.5 / §15 A2 A3；PRD BE-6 / R22

WS 客户端已实现，但 `tool.error` envelope 类型未定义（使用通用 `error` 类型）

#### CLI-23 REPL 主循环 + slash 命令 + render ✅ 已完成
- **工种**：CLI
- **估时**：1.5
- **依赖**：CLI-22
- **新增**：
  - `internal/cli/repl/{repl.go, slash.go, render.go, history.go, repl_test.go}`
  - `internal/cli/cmd/chat.go`（含 `aranea` 无参数路由 + `aranea chat`）
  - 新增依赖 `github.com/peterh/liner` / `github.com/gorilla/websocket`
- **DoD**：
  - `aranea` 启动进入 REPL，横幅含版本 / 后端 / Agent / Session；
  - `/help / /quit / /agent / /session new / /yes / /dry-run on|off` 可工作；
  - 在 mock WS server 上跑一遍 "用户消息 → tool.call → tool.result → done" 序列，渲染断言通过；
  - `Ctrl+C` 在生成中 = 中断（POST `/v1/chat/run-status/cancel` 或对照 proto 真实 cancel API）；在 prompt 阶段 = exit 4；
  - `Ctrl+D` 退出 REPL。
- **关联**：设计 §5.3；PRD US-10 / R23

#### CLI-24 `aranea skill install <url>` 直接命令实现 ⚠️ 待确认
- **工种**：CLI
- **估时**：1
- **依赖**：CLI-10
- **新增**：
  - `internal/cli/cmd/skill_install.go`
  - `internal/cli/skillinstall/{parseurl.go, locate.go, validate.go, pack.go}`
  - `internal/cli/client/multipart.go`（multipart upload 封装）
  - 新增依赖 `github.com/go-git/go-git/v5`
- **DoD**：
  - 对一个 happy-path GitHub 仓库（如 `https://github.com/anthropic/skills/tree/main/figma-code-connect`）执行 install，看到 6 步进度，最终 exit 0 + 新 skill_id；
  - 非交互 + warn + 无 `--decision` → exit 5；
  - `--decision skip / keep / refine` 三种均正确驱动后端 apply；
  - block → exit 5 + 打印 block 原因；
  - `--keep-temp` 保留临时目录；不传时安装成功后清理；
  - `--ref main` / `--subpath xxx` 工作。
- **关联**：设计 §5.2；PRD US-11 / R24

#### CLI-25 后端 Skill import 接收 multipart 来源字段 ⚠️ 待确认
- **工种**：BE
- **估时**：0.5
- **依赖**：—（可与 CLI-21..24 并行）
- **修改**：
  - `internal/service/skill_import.go`（multipart form 解析）
  - 持久化字段写入 `SkillImportJob.metadata_json` + 写 `tool_invocations` / `audit_logs`
  - 单测 + httptest
- **DoD**：
  - curl 用 multipart `-F file=@a.zip -F source=cli_url -F source_url=... -F source_ref=main -F source_subpath=x -F client_validation='{}'` 上传，DB 中可见对应字段；
  - 既有调用方（Web 上传）不受影响（字段全部 optional）。
- **关联**：设计 §4.2.2；PRD BE-2 / R25

#### CLI-26 `aranea skill import / import-status / import-apply` 子命令 ⚠️ 待确认
- **工种**：CLI
- **估时**：0.5
- **依赖**：CLI-10, CLI-25
- **新增**：
  - `internal/cli/cmd/skill_import.go`（不与 install 合并；CLI-24 调的是同一 client 函数）
- **DoD**：每个子命令各一条 httptest；手工对真实后端 import 一个本地 zip → 看到 job_id → status pending → apply → 成功。
- **关联**：PRD US-12 / R26

#### CLI-27 剩余资源命令（team / plugin / mcp / cron / channel / session / monitor） ✅ 已完成
- **工种**：CLI
- **估时**：3（建议拆 CLI-27.1 team、CLI-27.2 plugin、CLI-27.3 mcp、CLI-27.4 cron、CLI-27.5 channel、CLI-27.6 session、CLI-27.7 monitor，每个 0.5d）
- **依赖**：CLI-03, CLI-05
- **新增**：
  - `internal/cli/cmd/{team,plugin,mcp,cron,channel,session,monitor}.go`
  - `internal/cli/client/{team,plugin,mcp,cron,channel,session,monitor}.go`
- **DoD**：每个资源至少 1 条 happy-path smoke + 1 条 httptest；`channel send` 必须 `--yes`，否则即使 TTY 也拒绝（与删除不同：channel send 是外部副作用，无 prompt 路径）。
- **关联**：PRD US-12 / R27

✅ team / plugin / mcp / cron / channel / session / monitor 七类已实现（`cmd/aranea/main.go` 已注册全部命令）；`cron pause`/`cron resume` 额外补充实现（PRD §3.2 要求）；`monitor` 提供 `audit-logs`/`events`/`traces` 三个子命令，支持 `--action`/`--resource`/`--actor`/`--keyword`/`--event-type`/`--agent-id`/`--status`/`--provider`/`--model` 过滤

#### CLI-28 `aranea completion <shell>` ❌ 未完成
- **工种**：CLI
- **估时**：0.5
- **依赖**：CLI-01
- **新增**：
  - `internal/cli/cmd/completion.go`（cobra 自带 `GenBashCompletion` 等，直接转 stdout）
  - `docs/guides/cli-quickstart.md` 追加补全章节
- **DoD**：bash / zsh / fish / powershell 四种各跑一次（写到临时文件、source / `. .`、tab 补全 `aranea ag<TAB>` → `aranea agent`）。
- **关联**：PRD US-13 / R28

#### CLI-29 后端剩余 `cli_admin_*` 工具全量 ❌ 未完成
- **工种**：BE
- **估时**：2（建议按资源拆 0.5d × 4：team/plugin/mcp/cron + channel/provider/session）
- **依赖**：CLI-21
- **新增**：
  - `internal/tools/cli_admin/{team,plugin,mcp,cron,channel,provider,session}_*.go`
  - 每个工具一条单测
- **DoD**：所有 `cli_admin_*` 都在 `RegisterAll` 中注册；模拟 system_admin Agent 调用每个工具均能返回合法结果。
- **关联**：PRD R29

#### CLI-30 P1 文档与变更说明 ❌ 未完成
- **工种**：CLI
- **估时**：0.5
- **依赖**：CLI-20..29
- **新增**：
  - `docs/changelog/2026-XX-XX-CLI-P1.md`
  - 更新 `docs/guides/cli-quickstart.md` 加入对话模式与 skill install 章节
- **DoD**：链接进 `docs/README.md`；变更说明覆盖所有 P1 任务的对外影响。

#### **P1 退出条件**

- [ ] CLI-20..30 全绿（CLI-20/21/23/27 ✅；CLI-22 ⚠️ 部分完成；CLI-24/25/26 ⚠️ 待确认；CLI-28/29/30 ❌ 未完成）；
- [ ] PRD §9.2 R20..R29 + §9.3 R30..R34 全绿；
- [ ] 手工跑 `aranea` 进 REPL → "帮我把 figma-code-connect 装上" → 看到工具调用 + 二次确认 + 成功；
- [ ] Web 控制台 `/tools/runs` 看到 `source=cli` 的记录。

> **差距汇总**：CLI-22 `tool.error` envelope 类型未定义；CLI-24/25/26 待确认实现状态；CLI-28/29/30 未完成。

---

### 3.4 Phase P2（体验优化，按需）

| ID | 任务 | 工种 | 估时 | 备注 |
|----|------|------|------|------|
| CLI-40 | `--output yaml / table` | CLI | 0.5 | 引入 `gopkg.in/yaml.v3` |
| CLI-41 | REPL 上下箭头历史搜索 | CLI | 0.5 | `peterh/liner` 自带 |
| CLI-42 | 错误信息中文翻译表（按 `error.code`） | CLI | 1 | i18n map + fallback 原文 |
| CLI-43 | 跨平台二进制发布（GitHub Release / OSS） | CLI + ops | 1 | 与 admin 同版本号 |
| CLI-44 | 系统 keyring 集成 | CLI | 1 | macOS Keychain / Win Credential / libsecret；fallback 0600 文件 |
| CLI-45 | `aranea init` 引导 | CLI | 0.5 | 首启检测后端 + 写默认 config（不偷偷拉远端） |
| CLI-46 | E2E smoke 纳入 `make ci` | CLI + ops | 1 | `make smoke-cli`：启 admin → login → ls → 关 admin |

### 3.5 Phase P3（API 覆盖补全，2026-07-28）✅ 已完成

> 背景：服务端 proto 增至 37 个服务后，CLI 覆盖度审查发现 orgimport 断路 bug 与大量未覆盖域。本轮全部落地。

#### CLI-50 orgimport applier 断路修复 ✅

| 问题 | 修复 | 代码锚点 |
|------|------|----------|
| `/v1/agent-categories*` 路由不存在 | 类别 upsert 改走 OrganizationService（`POST /v1/organization`、`PATCH /v1/organization/{id}`），字段 `key`→`org_key`，level `industry`→`company` 映射 | `internal/orgimport/applier.go::upsertCategory/orgLevel` |
| `PUT /v1/agents/{id}`、`PUT /v1/teams/{id}` 405 | 改 PATCH（body 即 Agent/Team 消息本身） | `applier.go::upsertAgent/upsertTeam` |
| `?agent_key=`/`?key=` 过滤被静默忽略，items[0] 误配任意记录 | Agent 分页全量 + 客户端精确匹配 `agentKey`；Team/Organization 全量拉取精确匹配 `teamKey`/`orgKey` | `applier.go::lookupAgentByKey/lookupTeamByKey/lookupCategoryByKey` |
| Team payload `key/name/members` 与 proto 不符 | 改 `team_key`/`display_name`/`definition_json`（OrchestrationSpec `{version:2,mode:sequential,members[]}`） | `applier.go::upsertTeam` |
| Team 成员引用本次导入范围外的既有 agent 时 `agent_id` 静默为空 | 兜底 `lookupAgentByKey` 精确查询；解析失败则整队放弃并由 Apply 聚合错误继续下一队 | `applier.go::upsertTeam`（成员 fallback，2026-07-28 评审修复） |

回归测试：`internal/orgimport/applier_test.go`（10 用例，含"列表首项为无关记录时不得误更新"与"成员解析失败不得写团队"回归）。

#### CLI-51 已有域子命令补全 ✅

| 域 | 新增子命令 | 代码锚点 |
|----|-----------|----------|
| session | `archive` `restore` `pin` `unpin` `compact` `export` | `internal/cli/cmd/session.go`、`client/session.go` |
| skill | `files` `file-get` `file-put` `file-delete` `import`（两阶段：multipart 上传 → 轮询 → `--apply`） | `cmd/skill.go`、`client/skill.go` |
| cron | `reset-failures` | `cmd/cron.go`、`client/cron.go` |
| mcp | `validate`（校验未保存配置负载，POST /v1/mcp-servers/validate；校验失败返回 `VALIDATION_FAILED` 非零退出，符合 US-08 CI 契约） | `cmd/mcp.go`、`client/mcp.go` |
| tool | `test` | `cmd/tool.go`、`client/tool.go` |

#### CLI-52 新域命令（7 个）✅

| 命令 | 子命令 | 代码锚点 |
|------|--------|----------|
| `memory` | `facts ls`、`proposals ls/approve/reject`、`search`、`recall-debug` | `cmd/memory.go`、`client/memory.go` |
| `knowledge` | `collections ls/get/create/delete`、`documents ls/get/delete`、`search` | `cmd/knowledge.go`、`client/knowledge.go` |
| `eval` | `datasets ls/get/create`、`runs ls/get/create`、`results` | `cmd/evaluation.go`、`client/evaluation.go` |
| `org` | `ls` `tree` `get` `create` `update` `delete` `reorder` | `cmd/organization.go`、`client/organization.go` |
| `taxonomy` | `ls` `tree` `get` `create` `update` `delete` `reorder` | `cmd/taxonomy.go`、`client/taxonomy.go` |
| `model-catalog` | `ls` `get` `policy` `policy-set` `sync` | `cmd/model_catalog.go`、`client/model_catalog.go` |
| `a2a` | `discover`（POST /v1/a2a/remote-discover）、`remote-agents ls/get/add/delete`、`audit ls`、`config get` | `cmd/a2a.go`、`client/a2a.go` |

偏差说明：`a2a remote-agents get` 由 ls 客户端过滤实现（proto 无单 get 端点）；`a2a config` 只读（proto 无 PUT）；`memory recall-debug` 为 POST。全部已在 `cmd/aranea/main.go` 注册。

#### P3 退出条件

- [x] `go build ./internal/cli/... ./cmd/aranea/...` 通过
- [x] `go test ./internal/cli/... ./internal/orgimport/...` 全绿（client/cmd/orgimport 全部 ok）

---

## 4. 依赖关系图

```
P0
  CLI-01 ──┬── CLI-02 (config)
           ├── CLI-03 (http client) ──┬── CLI-04 (errors)  ──┐
           │                          ├── CLI-08 (login) ────┤
           │                          ├── CLI-09 (agent)     │
           │                          ├── CLI-10 (skill)     │
           │                          └── CLI-11 (tool)      │
           ├── CLI-05 (output) ──── (被 CLI-09/10/11/06 依赖) │
           └── CLI-06 (version) ←────────────────────────────┘
  CLI-07 (BE system info)  独立；CLI-06 在测真后端时联调

  全 P0 完成 → CLI-12 (Makefile + R12 lint)
                  └── CLI-13 (docs)

P1
  CLI-07 ─► CLI-20 (BE seed) ─┬─► CLI-21 (BE cli_admin first batch)
                              └─► CLI-29 (BE cli_admin rest)
  P0 done ─► CLI-22 (WS envelope + WS client) ─► CLI-23 (REPL)
  CLI-10 ─► CLI-24 (skill install)
  CLI-25 (BE multipart) ── 与 CLI-24/26 联调
  CLI-10 + CLI-25 ─► CLI-26 (import/import-status/import-apply)
  CLI-03 + CLI-05 ─► CLI-27.1~27.7 (7 资源)
  CLI-01 ─► CLI-28 (completion)
  全 P1 ─► CLI-30 (docs/changelog)
```

---

## 5. 风险登记

| # | 风险 | 来源 | 影响 | 缓解 / 触发后动作 |
|---|------|------|------|--------------------|
| R-D1 | WS envelope 子类型在 `internal/server/ws.go` 当前**只**有粗粒度 `system` + `payload`，没有 `tool.call/result/error` | 实施 CLI-22 | REPL 无法分类渲染工具步骤 | CLI-22.1 是硬前置；如果 BE 工作量超 0.5d，临时方案：CLI 在 payload 里反向解析 string contains，但要在 PR 注释中 mark 为 TODO |
| R-D2 | `/v1/admins/login` 返回 token 在 cookie 而非 body | 实施 CLI-08 | CLI 取不到 token | CLI-08 第一步是代码考古 A1；如果是 cookie，CLI 改为读 `Set-Cookie` 并提取 JWT；同步更新 PRD §6.1 |
| R-D3 | `cobra` 引入 ~3MB 二进制体积增量 + 多间接依赖 | CLI-01 | 主 module 依赖图变重 | 接受；若不可接受换 `urfave/cli`，但要求 PR 显式列对比 |
| R-D4 | Windows 控制台对 ANSI / spinner 兼容差（cmd.exe vs Windows Terminal） | CLI-05 | 输出乱码 | 启动调 `enable_virtual_terminal_processing`；spinner 用 ASCII；`NO_COLOR` 自动降级；非 TTY 强制关 |
| R-D5 | `go-git` 在私仓 SSH 凭据上行为不一致 | CLI-24 | install ssh URL 失败 | MVP 仅声明支持 https；ssh fallback 到本地 `git` 命令（如有）；私仓推到 P2 |
| R-D6 | CLI 与后端版本不一致时 API 字段缺失 | 所有 | 命令难诊断 | `aranea version` 强制比对 `system_info.version`；不一致 warn；`--output json` 时 warn 在 stderr |
| R-D7 | `internal/biz` 被 CLI 误 import | 实施期 | 二进制爆炸 / 框架污染 | CLI-12 R12 lint + CI 阻断；每个 PR 在 desc 复述红线 |
| R-D8 | `--yes` 在 CI 被滥用 | 任意写命令 | 误删 | 后端 delete 接口要求 `confirm_key`，CLI 只传不豁免；`channel send` 强制 `--yes` 才能跑 |
| R-D9 | Skill 临时目录在 Windows 文件句柄未释放导致清理失败 | CLI-24 | 磁盘占用 | `defer` + 容错；记录到 `~/.cache/aranea/logs/`；提供 `aranea config set skill.keep_temp false` |
| R-D10 | 系统管家 Agent 在 P1 上线前就有 Web 用户尝试调用 | CLI-20..23 之间 | 用户疑惑 | 在 `cmd/admin` 启动日志里输出"系统管家 Agent 已 seed，仅 CLI 可用"；Web 端在 P1 上线后再放展示 |

---

## 6. AI 落地提示词模板

> 给 cursor-agent / claude code 使用。每个任务复制一份，把 `{ID}` / `{TITLE}` / `{依赖任务}` 等占位符替换后喂进去。

```text
你是 Aranea-Agents 项目的实施 AI。本次只完成任务 {ID}: {TITLE}。

【必读】
1) docs/需求/25-cli-PRD-2026-05-27.md  —— 产品需求与验收准则（重点 §6 关键流程 + §9 R# 验收）
2) docs/需求/25-cli-design-2026-05-27.md —— 技术设计（重点 §2 目录结构 + §3 类型契约 + §13 红线）
3) docs/需求/25-cli-development-plan-2026-05-27.md —— 本计划（找 {ID} 的 DoD/依赖/新增文件清单）
4) AGENTS.md + .cursor/rules/trpc-agent-framework-first.mdc + docs/AGENT_RUNTIME_BOUNDARY.md
5) （视任务）相关 proto： api/kratos/<svc>/v1/*.proto；相关 service： internal/service/...

【红线（违反即停）】
- cmd/aranea/** 与 internal/cli/** 严禁 import: internal/biz、internal/data、internal/agent、
  internal/server、internal/service、pkg/trpc-agent-go
- HTTP 路径以 /v1/* 为准（proto google.api.http 注解是唯一真相源），不是 /api/v1/*
- 配置 token 文件权限必须 0600；不在 stdout 打印 token；--debug 中 mask 为 ***<last4>
- 系统管家 Agent 种子与 cli_admin_* 工具集只在后端实现，CLI 不参与

【本任务】
- 依赖：{依赖任务}（已完成）
- 修改 / 新增文件：{文件列表}
- DoD（必须全绿）：
  {DoD 列表}
- 关联：{PRD/设计/R# 引用}

【写作纪律】
- 用 CodeGraph 优先查结构性符号；不重复 grep 已查到的同一信息
- 单元 / 契约 / golden 测试在同 PR 内交付
- 任何与本计划假设不一致的情况，在 PR description 显式记录 "假设 AX 验证结果"
- PR 标题：[CLI-XX] <一句话>；body 模板见开发计划 §7
- 不顺带 refactor 相邻模块

完成后：跑 `go build ./cmd/aranea/` + `go test ./internal/cli/...` + `make lint`；
全绿后整理 PR 描述（§7 模板）。
```

---

## 7. PR 模板

```markdown
## 任务
- CLI-XX: <一句话>
- 依赖：CLI-YY..ZZ（均已合入）

## 主要改动
- [新增] internal/cli/...
- [修改] api/kratos/.../*.proto（若涉及）
- [修改] internal/service/...（若涉及）

## DoD（逐条勾选）
- [ ] R0/R1/...

## 假设验证
- A1: <实际行为> （与设计文档 §15 假设的差异）
- ...

## 红线 self-review
- [ ] CLI 二进制无禁忌 import
- [ ] HTTP 路径 /v1/*
- [ ] token 文件 0600 / 不暴露明文

## 验证命令
- go build ./cmd/aranea/
- go test ./internal/cli/... ./internal/service/...
- make lint
- 手工：./bin/aranea <子命令> --debug ...（贴关键输出截图）

## 相关文档
- PRD: docs/需求/25-cli-PRD-2026-05-27.md (US-XX, R-XX)
- 设计: docs/需求/25-cli-design-2026-05-27.md (§X.Y)
- 计划: docs/需求/25-cli-development-plan-2026-05-27.md (CLI-XX)
```

---

## 8. 验收 checklist（汇总）

### 8.1 P0 出口
- [x] CLI-01..13 ✅ 已完成（CLI-12 R12 lint 已上线；CLI-13 quickstart 已落地；残留：`cli-all` 仅 Linux/amd64、`docs/README.md` 索引缺失）
- [x] PRD §9.1 R0..R12 全绿（R11 R12 lint 已实现；R12 quickstart 已创建）
- [x] `make cli` / `make lint` / `make test` 全绿
- [x] `./bin/aranea agent ls` 对真实 admin 后端正确分页
- [ ] `docs/changelog/2026-XX-XX-CLI-P0-MVP.md` 已合入
- [x] 原 `25-cli-development.md` 顶部已加 superseded 指向本计划

### 8.2 P1 出口
- [ ] CLI-20..30 全绿（CLI-20/21/23/27 ✅；CLI-22 ⚠️；CLI-24/25/26 ⚠️ 待确认；CLI-28/29/30 ❌）
- [ ] PRD §9.2 R20..R29 全绿（R27 monitor 已实现）
- [ ] PRD §9.3 R30..R34 全绿
- [ ] 手工跑通对话模式 + skill install
- [ ] Web 控制台 `/tools/runs` 可见 `source=cli` 的记录
- [ ] `docs/changelog/2026-XX-XX-CLI-P1.md` 已合入

### 8.3 跨平台
- [ ] `make cli-all` 三平台 amd64+arm64 全部编译通过
- [ ] Win / mac / Linux 至少各跑一次 `aranea version` + `aranea agent ls`

---

## 9. 与原 `25-cli.development.md` 的差异

| 项 | 原 v2.0 | 本 v3.0 |
|----|---------|---------|
| 任务编号 | `1.1..1.17 / 2.1..2.18 / 3.1..3.18 / 4.1..4.7`（4 phase × 多个） | `CLI-01..30`（统一前缀 + 阶段集中） |
| Phase 1 包含对话相关 dbAgentLoader / ConfirmPlugin | 是 | **删除**，移入 P1 且改为 WS 客户端模式（不在 CLI 进程构建 Runner） |
| 任务粒度 | 大量 ≥1 天的混合任务（如 "Phase 2.16 BuildSystemAdminAgent" 与 CLI 集成耦合在一起） | 每条 ≤1 天；BE / CLI 工种分开；联调任务显式拆 |
| 红线检查 | 未在任务表内显式拦截 | CLI-12 R12 lint 在 P0 退出条件 |
| 任务依赖图 | 仅 phase 内 | 跨 phase + BE/CLI 明确 |
| DoD | 多为复选框列表 | 每条任务包含可执行验收信号（命令 / 期望输出） |
| AI 落地 | 无提示词模板 | §6 直接给可复制提示词；§7 PR 模板 |
| Skill install 工具实现位置 | `internal/tools/cli_admin/skill_install.go`（后端） + `cmd/aranea/skill/install.go`（前端）混淆 | 后端工具 = CLI-21；CLI 端命令 = CLI-24；两者用 multipart upload 各自实现，不共享代码 |

---

## 10. 时间盘点

| 阶段 | 估时合计（理想） | 缓冲 (×1.5) | 推荐排程 |
|------|------------------|-------------|----------|
| P0 | 8.5 人天 | ~13 人天 | 1 人 2 周；2 人 1 周（按 BE/CLI 拆分 CLI-07 与其余并行） |
| P1 | 12 人天 | ~18 人天 | 1 人 3~4 周；建议 BE / CLI 各 1 人并行 ~2 周 |
| P2 | 5.5 人天 | ~8 人天 | 按需 |

> 估时仅含实现 + 单测 + 文档；不含 review 周期。

---

*文档版本：3.2 — 2026-06-24；与 [`25-cli.md`](./25-cli.md) / [`25-cli.design.md`](./25-cli.design.md) 同步。CLI-27 monitor 命令已实现，cron pause/resume 补充。*
