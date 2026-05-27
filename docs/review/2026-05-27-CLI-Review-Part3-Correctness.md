# CLI Review · Part 3 · 功能正确性 / 业务闭环 / 测试（2026-05-27）

> 审查范围：`internal/pkginstall/**`、`internal/cli/cmd/{pkg,chat}.go`、`internal/cli/repl/**`、
> `internal/cli/client/ws.go`、`internal/tools/cli_admin/**`、`internal/data/seed_system_admin.go`
> 与 `internal/data/data.go` 的 `seedInitialData`、`internal/cli/**/*_test.go`、
> `internal/pkginstall/**/*_test.go`。
>
> 仅关注 **功能正确性 / 业务闭环 / 测试覆盖**，不重复架构红线扫描（已由 Part 1 覆盖）。

---

## A. Package Install 闭环

### A1. Installer 安装顺序（必检 1）
**结论**：✅ 与设计文档一致。`Install` 顺序写死为 MCP → Skill → Org(行业/部门/岗位) → Agent → Team → Graph，其中 Agent/Team 由 step 3 的 orgimport 一次性 Apply 完成，步骤 4/5 只是占位输出。
**证据**：`internal/pkginstall/installer.go:49-118`（顺序与 `totalSteps=6` 常量）；`installer.go:77-101` 把 Agents/Teams 塞进 orgimport Spec，第 4/5 步仅打印 `done (via org import)` 占位。

### A2. 失败处理策略（必检 2）
**结论**：🟠 **continue-on-error**，每个资源失败都被记录到 `result.Steps[]` 和 `result.Errors[]`，但没有抛错中断；甚至 orgimport 内部错误也只追加到 `result.Errors`。CLI 端 (`cmd/pkg.go:111-120`) 只是打印警告然后正常退出，进程退出码 `0`。
**问题**：对调用方（人或 AI Agent）来说，**部分失败 ≠ 安装失败**——无法用 `$?` 判定。
**证据**：`installer.go:127-138, 91-99`；`cmd/pkg.go:106-122`。

### A3. `FetchFromURL`（必检 3）
| 子项 | 结论 | 证据 |
|---|---|---|
| 清理临时目录 | ✅ 返回 `cleanup func()`，CLI 默认 `defer cleanup()`（除非 `--keep-temp`） | `loader.go:39-67`、`cmd/pkg.go:63-67, 146` |
| `--ref` → `git clone -b` | ✅ ref 非空时追加 `--branch` | `loader.go:46-49` |
| Shallow clone | ✅ `--depth 1` 写死 | `loader.go:46` |
| 是否仅支持 git | ⚠️ **仅 git**，无 zip/tar/http(s) 包格式支持；URL 直接交给 `git clone` | `loader.go:55, 62-65` |
| 鉴权 | ⚠️ 无（依赖宿主 `git` PATH 中的 credential helper），私库需用户预先配好 SSH/PAT |

### A4. 路径校验（必检 4）
**结论**：🔴 **无任何 `..` 逃逸防护**。`manifest.spec.skills[].path`、`skills[].subpath`、`graphs[].file` 都直接 `filepath.Join(pkgDir, …)`，恶意 manifest 可读取宿主任意文件后上传给后端 `/v1/skills/import` 或读 `/etc/passwd` 注入 `/v1/graph/import`。
**证据**：
- `installer.go:233` `zipPath = filepath.Join(pkgDir, spec.Path)`
- `installer.go:243` `srcDir = filepath.Join(tmpDir, spec.Subpath)`
- `installer.go:311` `jsonPath := filepath.Join(pkgDir, spec.File)`

### A5. `pkg validate`（必检 5）
**结论**：✅ 仅 `FetchFromURL` → `LoadManifestFromDir` → `ValidateManifest`，不构造 `Installer`，**完全不写后端**。
**注意**：仍会 `git clone` 一次完整代码——不是纯本地校验。
**证据**：`cmd/pkg.go:132-167`。

### A6. **新发现 — `installSkill` 的临时 zip 路径 bug**（高优先级）
**严重度**：🔴
**问题**：
```62:73:internal/pkginstall/installer.go
	} else if spec.URL != "" {
		// Clone the skill from URL to a temp dir, zip the subpath.
		tmpDir, cleanup, err := FetchFromURL(spec.URL, spec.Ref, ins.Quiet)
		if err != nil {
			return StepResult{Resource: resource, Action: "error", Message: err.Error()}
		}
		defer cleanup()
		srcDir := tmpDir
		if spec.Subpath != "" {
			srcDir = filepath.Join(tmpDir, spec.Subpath)
		}
		tmpZip := filepath.Join(os.TempDir(), "skill-*.zip")
		if err := zipDir(srcDir, tmpZip); err != nil {
```
`zipDir`（`ziputil.go:13-22`）检测到 `*` 时**只在内部**改写 `destPath = f.Name()`，**不向调用方回传**新路径。随后 caller 用字面量 `"…/skill-*.zip"` 去 `os.Open(zipPath)`（`installer.go:255`），在 Linux/macOS 上会因路径含字面 `*` 而失败，在 Windows 上文件名含 `*` 直接非法。
**结论**：基于 `url:` 字段的 skill 安装路径**实测必然 fail**——这是个隐藏的功能阻断。
**修复建议**：让 `zipDir` 返回真实 path，或在调用方先 `os.CreateTemp` 拿到名字再传入。

---

## B. AI 工具闭环（关键）

### B1. `cli_admin_pkg_install_from_url`（必检 6）
**结论**：✅ **真实实现**（非 stub）。tool 函数体把 `pkginstall.FetchFromURL` → `LoadManifestFromDir` → `ValidateManifest` → `Installer{}.Install` 全跑了一遍。
**入参 schema**：完整含 `url`(required) / `ref` / `decision` / `dry_run`。
**出参**：返回 `created/updated/skipped/errors[]/steps[]`，AI 可读。
**证据**：`internal/tools/cli_admin/pkg_install_from_url.go:30-96`。

### B2. `cli_admin_skill_install_from_url`（**重要附加发现**）
**结论**：🔴 **STUB**。函数构造了 payload 后直接 `_ = body`，返回 `status:"triggered"` 的假成功消息，从未发起任何 HTTP 调用。
**证据**：
```36:54:internal/tools/cli_admin/skill_install_from_url.go
		payload := map[string]any{ ... }
		body, _ := json.Marshal(payload)
		_ = body

		// For now, return a pending status with instructions.
		// The actual multipart upload is handled by pkginstall.Installer.
		return skillInstallOutput{
			Status:  "triggered",
			Message: fmt.Sprintf("正在从 %s 安装 Skill，使用 cli_admin_skill_import_status 查询进度", input.URL),
		}, nil
```
**修复建议**：要么删除该工具让 AI 改用 `cli_admin_pkg_install_from_url`，要么真去走 `pkginstall.Installer.installSkill`。

### B3. **核心闭环缺口 — `cli_admin.RegisterAll` 从未被调用**
**严重度**：🔴 **致命**
**结论**：`internal/tools/cli_admin/registry.go:71` 的 `RegisterAll(deps)` **在整个仓库中没有任何调用方**（grep `internal/tools/cli_admin` 仅命中文档与 seed 注释，未命中任何 service/agent 装配代码）。
**意味着**：
- DB 里 `tools` 表确实有 `cli_admin_*` 条目（`seed_system_admin.go:46-85` 写入）；
- 但系统管家 Agent 真实运行时拿到这些工具 key 后，**trpc-agent-go 的 toolset 里找不到对应 `function.Tool` 实例**，调用会失败（具体表现取决于框架的未注册工具处理，可能是 `tool not found` 或被 LLM 静默忽略）。
- 因此 **`aranea pkg install` (CLI 路径) 能装东西，但让系统管家 Agent 装东西的链路是断的**。
**证据**：
- `grep cli_admin.RegisterAll` 全仓 0 命中（仅 `docs/需求/25-cli-development-plan-2026-05-27.md:302` 描述了"应当"装配）。
- `internal/biz/agent_effective_tools.go:98-104, 128-129, 183` 把 `group:cli_admin` 解析为带前缀的 key 列表，**只解析 key、不绑实现**。
**修复建议**：在 `internal/service` 的系统管家 Runner 装配处调用 `cli_admin.RegisterAll(cli_admin.Deps{...})` 把返回的 `[]trpctool.Tool` 注入到该 Agent 的 toolset。

### B4. `SeedSystemAdminAgent` 幂等性（必检 7）
**结论**：✅ 幂等。使用 `INSERT … ON CONFLICT(agent_key) DO NOTHING`，重复启动不会重复插入。
**问题**：⚠️ **未绑定 tool 关联表**。代码只写 `agents` 与 `tools` 两张表，**没有写 `agent_tools` 关联**；而是依赖 `config_json:'{"tools_profile":"system_admin"}'`（`seed_system_admin.go:37`）由 effective-tools 计算时把 `group:cli_admin` 展开。这条隐式链路成立，但与 B3 致命缺口叠加后仍然跑不通。
**证据**：`seed_system_admin.go:28-42`；`agent_effective_tools.go:183`。

### B5. REPL → WS → 系统管家 Agent 链路（必检 8）
**结论**：⚠️ **chat 默认不路由到 `__system_admin__`**。
- `cmd/chat.go:13-50` 的 `--agent` flag 默认空；
- `repl.sendMessage` 仅在 `agentKey != ""` 时塞 `payload["agent_key"]`（`repl/repl.go:153-160`），否则交由服务端选默认；
- 设计文档（`25-cli-PRD-2026-05-27.md:143`）把 `/agent` 作为切换斜杠命令，未声明默认路由策略；
- 因此用户必须显式 `aranea chat --agent __system_admin__` 或会话内 `/agent __system_admin__` 才能与系统管家对话。
**额外问题**：`/agent <key>` 只改本地变量，**不开新 session**（设计文档 §4.5 要求"切换 Agent（开新 session）"）。证据：`internal/cli/repl/slash.go:36-43`。

---

## C. REPL 正确性

### C1. `/slash` 命令对照（必检 9）

| `/` 命令 | 设计文档（25-cli-PRD §4.5） | 实际实现 | 备注 |
|---|---|---|---|
| `/help` | ✓ | ✅ `slash.go:28-30` | 别名 `/h /?` |
| `/agent <key>` | ✓ 切换并开新 session | 🟠 只切本地 `r.agentKey`，**不开新 session** | `slash.go:36-43` |
| `/session new\|list\|resume` | ✓ | 🔴 **仅显示当前 ID**，无 new/list/resume | `slash.go:32-34` |
| `/yes` | ✓ 会话内跳过所有确认 | 🔴 **未实现** | grep `parts[0] == "yes"` 无命中 |
| `/quit` `/exit` | ✓ | ✅ `slash.go:25-26`（含 `/q` 别名） | |
| `/tools` | ✓ | 🔴 **未实现** | |
| `/expand` | ✓ 展开上一次工具结果 | 🔴 **未实现** | |
| `/copy` | ✓ 复制上条回复 | 🔴 **未实现** | |
| `/dry-run on\|off` | ✓ | 🟡 仅 toggle，**忽略 on/off 参数** | `slash.go:45-53` |
| `/cancel` | — | ✅ 额外实现 | `slash.go:55-60` |
| `/clear` | — | ✅ 额外实现 | `slash.go:62-64` |

### C2. WS 事件渲染（必检 10）

| Event Type | 实际渲染 | 备注 |
|---|---|---|
| `text_delta` | ✅ 流式输出 | `render.go:48-52` |
| `text_done` | ✅ 收尾换行 | `render.go:53-58` |
| `tool_call`（running/success/error） | ✅ 三态分别 `⚙ / ✓ / ✗` | `render.go:60-96` |
| `tool_result` | ✅ 闭合工具块 | `render.go:98-107` |
| `runner_completion` / `run_status` | 🟡 仅做收尾换行，不展示状态详情 | `render.go:109-114` |
| `error` | ✅ 显示错误消息 | `render.go:115-121` |
| `transfer` | 🟡 仅显示目标 agent，不显示 reason / payload | `render.go:123-134` |
| `pong` / `connected` | ✅ 静默忽略 | `render.go:136-143` |
| `server_shutdown` | ✅ 提示行 | `render.go:139-140` |
| **default** | 🟠 **静默 ignore** | `render.go:145-147`，未知 type 既不打 debug 也不报警，调试期排查困难 |

未涵盖（且未列入设计）：`message_start`、`tool_args_delta`、`memory_update`、`audit_*`、`session_meta_*` 等常见后端事件——若后端有发，前端**完全沉默**。

### C3. 断线重连（必检 11）
**结论**：🔴 **未实现**。`client.WSConn.readPump` 一旦遇到 read error 就关闭 channel 退出（`ws.go:79-100`），REPL 的 `renderLoop` (`repl.go:162-166`) range over 完关闭的 channel 后**静默退出 goroutine**，inputLoop 仍然继续，但 `r.conn.Send` 会基于已关闭的底层 conn 直接报错——用户体验是"发了消息但没回复"。文档也未明确声明放弃重连。

---

## D. 测试覆盖矩阵

### D1. `internal/cli/**` 测试文件清单（必检 12）
| 路径 | 行数级别 | 覆盖范围 |
|---|---|---|
| `internal/cli/execute_test.go` | 短 | `ExitCodeOf` 不同错误码映射（4 个 case） |
| `internal/cli/config/config_test.go` | 中 | Load/Save 配置（默认值、文件写入） |
| `internal/cli/output/output_test.go` | 中 | 5 个 golden 输出对比（agent_ls / error_skill_blocked × json/text/tty） |
| `internal/cli/client/client_test.go` | 中 | HTTP Bearer / Accept / 401 解码 |
| `internal/cli/client/errors_test.go` | 中 | 错误解码、状态码映射 |
| `internal/cli/client/skill_test.go` | 短 | List/Get Skill |
| `internal/cli/client/agent_test.go` | 短 | Agent CRUD |
| `internal/cli/client/tool_test.go` | 短 | Tool 相关接口 |

### D2. 核心包覆盖（必检 13）
| 包 | 测试 | 评 |
|---|---|---|
| `cli/client` | ✅ 4 个 | 主要走 httptest，覆盖契约 |
| `cli/config` | ✅ 1 个 | 基础 OK |
| `cli/clierr` (在 `internal/cli/clierr/`) | 🔴 **无** | `clierr.go` 未单测 |
| `cli/output` | ✅ 1 个（含 golden） | 见 D3 |
| `cli/execute`/`cli/exit` | ✅ 1 个 | 仅 exit 码 |
| **`cli/repl`** | 🔴 **无任何 _test.go** | slash 解析、render switch、输入循环全部裸奔 |
| **`pkginstall`** | 🔴 **无任何 _test.go** | installer/loader/manifest/ziputil 全部未测 |
| **`tools/cli_admin`** | 🔴 **无任何 _test.go** | 6 个工具 0 测试 |
| `orgimport` | 未列入审查范围（但本次 grep 同样 0 单测命中） | — |

### D3. Golden 测试（必检 14）
**结论**：✅ 有，但仅 5 个：`agent_ls_{json,text_pipe,text_tty}.golden` + `error_skill_blocked_{json,text}.golden`。
**问题**：覆盖面极窄；`pkg install` / `chat` / `skill ls` / `tool runs` 等命令的输出格式无契约保护。

### D4. pkginstall 失败分支（必检 15）
**结论**：🔴 **完全无测试**。`Installer.Install` 中所有 error 分支（MCP `doJSON` 失败、skill multipart 失败、graph 文件读取失败、orgimport `Apply` 失败、`FetchFromURL` git 缺失等）都没有任何 `_test.go` 校验，A6 的 zip 字面 `*` bug 没被任何测试捕获正是其副产品。

---

## E. 问题清单（按严重度）

### 🔴 Critical（功能阻断 / 闭环缺口）
1. **`cli_admin.RegisterAll` 从未被装配到 Runner**——AI 系统管家 Agent 全部 `cli_admin_*` 工具**不可调用**。证据：`internal/tools/cli_admin/registry.go:71`，全仓 0 调用方。
   **修复**：在 `internal/service` 装配 `__system_admin__` Agent 时显式调用并把返回的工具注入该 Agent 的 toolset。
2. **`cli_admin_skill_install_from_url` 是 stub**——AI 单独调它装 Skill 永远只回"triggered"假成功。证据：`internal/tools/cli_admin/skill_install_from_url.go:42-48`。
   **修复**：直接复用 `pkginstall.installSkill` 逻辑，或移除该工具改导引到 `pkg_install_from_url`。
3. **`pkginstall.installSkill` URL 路径有 zip 字面 `*` bug**——任何 `skills[].url` 形式的安装实测失败。证据：`installer.go:245` + `ziputil.go:13-22`。
4. **manifest 路径无 `..` 防护**——恶意 package 可读宿主任意文件并经后端导入接口外泄。证据：`installer.go:233, 243, 311`。
5. **`internal/cli/repl/**` 与 `internal/pkginstall/**` 测试覆盖率为 0**——核心闭环代码无自动化回归保护，A6/A2 类问题难被 CI 拦截。

### 🟠 High（行为偏离设计）
6. **失败处理 fail-soft**：partial install 不影响进程退出码，CLI 和 AI 都无法用 `errors[]` 之外的方式判定失败级别。证据：`cmd/pkg.go:106-122`。
7. **`/session` `/yes` `/tools` `/expand` `/copy` 5 个 PRD 明令命令未实现**；`/agent` 不开新 session、`/dry-run` 忽略 `on|off`。证据：`slash.go` 整体。
8. **WS 无断线重连**——README/PRD 未说明，用户体验差。证据：`ws.go:79-100` + `repl.go:162-166`。
9. **`aranea chat` 默认不路由到系统管家**——大多数用户首次 `aranea chat` 不会想到要 `--agent __system_admin__`，闭环可达性差。证据：`cmd/chat.go:13-50`。

### 🟡 Medium
10. **render 对未知 event 静默丢弃**——后端新增事件类型时 CLI 无任何提示，回归难察觉。证据：`render.go:145-147`。
11. **`FetchFromURL` 仅支持 git**——无 zip/tar/http(s) 直链，限制分发渠道（设计未明确收口，可改为 P2）。证据：`loader.go:55`。
12. **`pkg validate` 仍要 git clone**——若仅想本地预检查 `aranea-package.yaml`，没有"指向本地目录"的入口。证据：`cmd/pkg.go:132-167`。

### 🟢 Low
13. **`SeedSystemAdminAgent` 用裸 SQL** 跳过 ent setter——当 `agents` 表结构升级时易漏字段；建议改用 ent 客户端 + `Upsert`。证据：`seed_system_admin.go:28-42`。
14. **`renderLoop` goroutine 没有完成信号**——`r.conn.events` 关闭后 goroutine 静默退出，无日志。证据：`repl.go:162-166`。

---

## F. 评分

- **功能正确性**：**4 / 10**（A6 zip bug、B2 stub、B3 RegisterAll 未装配三处叠加，最关键的 AI 自动化路径跑不通）
- **业务闭环完整性**：**3 / 10**（系统管家 Agent ↔ cli_admin 工具 ↔ pkginstall 三段中，最后一段未连接；REPL 默认入口也不指向系统管家）
- **测试覆盖**：**3 / 10**（client/config/output 有基础 case 与少量 golden；repl、pkginstall、cli_admin 三个最核心包 0 测试，失败分支无回归）
