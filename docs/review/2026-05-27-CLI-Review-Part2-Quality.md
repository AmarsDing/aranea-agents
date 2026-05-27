# CLI Review · Part 2 · 代码质量 / 错误处理 / 安全（2026-05-27）

> 范围：`internal/cli/cmd/*.go`（team / plugin / mcp / cron / channel / graph / session / pkg / login / config）、
> `internal/cli/client/`、`internal/cli/config/`、`internal/cli/clierr/`、`internal/cli/output/`、`internal/cli/ui/`、
> `internal/pkginstall/`、`cmd/aranea/main.go`。
> 架构红线由另一审查负责，本文件聚焦质量 / 错误 / 安全。

---

## 1. 重复代码评估

7 个 P1 资源 cmd 文件均为"列出 / 详情 / 创建 / 更新 / 删除（+确认）/ 业务动作 / 行映射 helper"模板，
结构几乎相同，只是 proto 类型不同。

| 文件 | 总行数 | CRUD 子命令 | 模板化重复部分（估算） | 重复占比 |
|---|---:|---:|---:|---:|
| `cmd/team.go`    | 227 |  ls/get/create/update/delete + run/runs | 约 110 行 | 48% |
| `cmd/plugin.go`  | 148 |  ls/enable/disable/order-set/config-set | 约 70 行  | 47% |
| `cmd/mcp.go`     | 182 |  ls/get/add/update/delete/test          | 约 120 行 | 66% |
| `cmd/cron.go`    | 210 |  ls/get/add/update/delete/trigger/runs  | 约 130 行 | 62% |
| `cmd/channel.go` | 208 |  ls/get/add/update/delete/test/toggle    | 约 130 行 | 63% |
| `cmd/graph.go`   | 221 |  ls/get/create/update/delete/import/export | 约 110 行 | 50% |
| `cmd/session.go` | 162 |  ls/get/send/messages                   | 约 35 行  | 22% |
| **合计** | **1358** | — | **~705** | **~52%** |

具体重复模式：

1. **"读文件 + protojson.Unmarshal" 模板**重复 14 次（mcp/cron/channel/graph/team 的 add+update）。每次约 8 行：
   ```go
   data, err := readFile(filePath)
   if err != nil { return &cli.CLIError{Code: "FILE_READ_ERROR", Message: err.Error()} }
   uopts := protojson.UnmarshalOptions{DiscardUnknown: true}
   if err := uopts.Unmarshal(data, req); err != nil {
       return &cli.CLIError{Code: "FILE_PARSE_ERROR", Message: fmt.Sprintf("……: %v", err)}
   }
   ```
2. **"删除前确认"模板**重复 7 次（team/plugin disable/mcp/cron/channel/graph delete），每次 8 行。
3. **`xxxToRow` / `xxxsToRows` helpers** 7 套，逻辑完全对称。
4. **flag 注册 + `MarkFlagRequired("file")`** 重复 ~10 次。

**抽象建议**：

- 引入泛型 helper（Go 1.21+ 已可用）：

  ```go
  func loadProtoFromFile[T proto.Message](path string, req T) error
  func confirmAndDo(cc *cli.Context, prompt string, fn func() error) error
  ```

- 把"protojson + CLIError 封装"集中到 `internal/cli/cmd/internal/util.go`，
  预计可削减 **300+ 行**（22%）。如果再用 reflect/泛型把 `xxxToRow` 改为
  "通过 `prioritizedKeys + reflect`"统一渲染，可再削 **150 行**。

- 进一步可考虑 `resourceCmd(name string, ops Ops[T])` 工厂，但收益递减，
  当前阶段先抽 helper 即可。

---

## 2. CLIError 一致性

| 文件 | `CLIError` | `fmt.Errorf` | `errors.New` | 备注 |
|---|---:|---:|---:|---|
| `team.go`         | 5 | 0 | 0 | ✓ 全部 CLIError |
| `mcp.go`          | 5 | 0 | 0 | ✓ |
| `cron.go`         | 5 | 0 | 0 | ✓ |
| `channel.go`      | 5 | 0 | 0 | ✓ |
| `graph.go`        | 7 | 0 | 0 | ✓ |
| `session.go`      | 2 | 0 | 0 | ✓ |
| `plugin.go`       | 1 | 0 | 0 | ✓ |
| `pkg.go`          | 7 | 0 | 0 | ✓ |
| `agent.go` / `tool.go` / `skill.go` | 6 / 2 / 7 | 0 | 0 | ✓ |
| `config_cmd.go`   | 4 | **1** | 0 | `保存配置失败` 用 fmt.Errorf |
| `login.go`        | 1 | **1** | 0 | 同上 |
| `chat.go`         | 1 | **1** | 0 | `未配置服务器地址` |
| `import.go`       | 3 | **3** | 0 | 加载 / 写入 / Apply 失败均 fmt.Errorf |

**问题**：6 处 `fmt.Errorf` 直接返回，**`ExitCodeOf` 对非 `*CLIError` 一律映射为
`ExitNetworkError`（3）**（`clierr/clierr.go:45-46`）—— "保存配置失败"或"加载规格失败"
被报告为"网络错误"，与 PRD §5.2 退出码语义不符。

修复建议：包装为 `&cli.CLIError{Code:"CONFIG_SAVE_ERROR", Cause: err}` 等。

---

## 3. 错误处理

### 3.1 HTTP 重试（`client/retry.go`）

- 仅对 **GET/HEAD/OPTIONS** 重试，POST/PUT/DELETE 不重试 ✓（语义安全）。
- `maxRetries = 3`，退避 200ms / 600ms / 2s。
- 重试条件：`err != nil` 或 `status >= 500`。
- **不区分 429**：当前 `< 500` 直接返回不重试，**429（Too Many Requests）不重试**。
- **不解析 `Retry-After` 头**。
- **不区分 503**：归入 `>= 500` 已重试 ✓。
- ⚠️ 在 `len(delays)=3`、循环 3 次时，最后一次失败后 **不会再等待**（OK），
  但中间错误时若 `err != nil` 也会立即继续 sleep —— 没有 jitter，可能形成"羊群效应"。

### 3.2 HTTP timeout

- `client.NewClient` 使用 `http.DefaultClient`（`retryDoer.inner`），**未设置 `Timeout`**。
  长连接 / 上游卡死会让 CLI 无限等待。
- 全局 `--timeout` flag **不存在**；仅 `aranea pkg install --timeout`、
  `aranea import org --timeout` 各自传给 `pkginstall.Installer` / `orgimport.ApplyOptions` 的
  内部 `http.Client.Timeout`。
- WebSocket：`HandshakeTimeout: 10 * time.Second` ✓。
- **建议**：在 `NewClient` 内构造 `&http.Client{Timeout: 60s}`，并暴露 `--timeout` 全局 flag。

### 3.3 `pkginstall.Installer.Install` 失败处理

- 6 个步骤**串行执行**，**单步失败不会中断后续步骤**（`installer.go:49-119`）。
  - 单个资源失败 → `result.Errors = append(...)`、继续下一个资源 / 下一步。
  - `result` 末尾汇总 created/updated/skipped/errors。
- **没有回滚 ledger**：已创建的 MCP 服务器在 Skills 失败时不会被删除。
- **没有 abort-on-first-error 开关**。
- 边界 bug：`installer.go:245` `filepath.Join(os.TempDir(), "skill-*.zip")` 把字面量 `*`
  作为路径名，多次安装会**复用同一个文件名 + race**；应改为 `os.CreateTemp`。

### 3.4 panic safety

- `cmd/aranea/main.go` **未设置 `defer recover()`**；任意子命令 panic 将导致 stack trace
  直写 stderr。低概率但栈帧里可能包含 token 局部变量（详见 §4 Token 泄漏）。

### 3.5 `clierr.ExitCodeOf` 覆盖度

代码中实际抛出的 `CLIError.Code` vs. `ExitCodeOf` 显式匹配：

| Code（实际使用） | 是否覆盖 | 实际退出码 | 建议 |
|---|---|---|---|
| `USER_CANCELED` | ✓ | `ExitUserCanceled` (4) | OK |
| `SKILL_IMPORT_BLOCKED` | ✓ | `ExitConflictBlocked` (5) | OK |
| `CONFIRMATION_REQUIRED` | ✓ | `ExitConflictBlocked` (5) | OK |
| `UNAUTHENTICATED/UNAUTHORIZED/FORBIDDEN` | ✓ | `ExitAuthError` (6) | OK |
| `INSECURE_CONFIG_PERM` / `CONFIG_INVALID` | ✓ | `ExitUsage` (1) | OK |
| `NETWORK_ERROR` | ✓ | `ExitNetworkError` (3) | OK |
| **`CONFIRM_REQUIRED`**（session.go:78）| ✗ **拼写不一致** | 默认 `ExitNetworkError` (3) | 🔴 应改为 `CONFIRMATION_REQUIRED` |
| `LOGIN_NO_TOKEN`（login.go:32）| ✗ | `ExitNetworkError` (3) | 应映射 `ExitAuthError` (6) |
| `FILE_READ_ERROR` / `FILE_PARSE_ERROR` | ✗ | `ExitNetworkError` (3) | 应映射 `ExitUsage` (1) |
| `MISSING_CONTENT`（session.go:83）| ✗ | `ExitNetworkError` (3) | 应映射 `ExitUsage` (1) |
| `CONFIG_KEY_UNKNOWN` / `CONFIG_VALUE_INVALID` | ✗ | `ExitNetworkError` (3) | 应映射 `ExitUsage` (1) |
| `PKG_FETCH_ERROR` / `PKG_MANIFEST_ERROR` / `PKG_MANIFEST_INVALID` / `PKG_INSTALL_ERROR` | ✗ | `ExitNetworkError` (3) | 至少 INVALID 应是 `ExitUsage` |

**评估**：至少 10 个真实使用的 Code **未被显式映射**，全部落入 `ExitNetworkError` 兜底。
这是非常严重的语义错误：脚本根据退出码做分支判断会把"用户输错 config key"误认作"网络故障"。

---

## 4. 安全

### 4.1 Token 文件权限

- `config/config.go:137` `os.WriteFile(path, data, 0o600)`、`FixPerm(path)` → `os.Chmod(0o600)`。
- `config/paths.go:57` `EnsureSecurePerm`：Windows skip，Unix 校验 `mode <= 0600`。
- Windows ACL 等价处理只在注释中说明（`FixPerm` 注释、`Save` 注释），**未实际**对 Windows
  做 ACL 强化（NTFS 默认 user 私有目录已是 ACL 隔离）。可接受，但应在 `docs/security.md`
  显式说明限制。

### 4.2 Token 泄漏扫描

- `client/http.go:131-144 logRequest`：对 `Authorization` 头做 `MaskToken`，正确。
- `cmd/config_cmd.go:106` 在 `--show-token` 时先 stderr 输出明确警告，再返回明文 ✓。
- `cmd/login.go` 不打印 token；登录成功只打印 user / path ✓。
- `internal/cli/ctx.go:60` Logger 写入 `logOut`，`debug=false` 时为 `io.Discard` ✓。

唯一漏洞 → 见 §4.3 WS URL。

### 4.3 WS URL 带 token 的日志泄漏 🔴

- `client/ws.go:67`：
  ```go
  return nil, fmt.Errorf("ws dial %s: %w", u.String(), err)
  ```
  `u.String()` 包含 `?session_id=...&token=<JWT>`（line 55-59 构造）。
  Dial 失败时整段 URL（含完整 JWT）会出现在错误链中 → cobra 打印到 stderr →
  CI 日志 / `--debug` log / 上游捕获均会泄漏 token。
- ws.go 没有 `Debug` 路径，但错误消息本身就足以泄漏。
- 修复：`u.Redacted()` 已无效（query 不会被 redact），需手动 `q.Del("token")` 后重新 encode 再格式化。

### 4.4 路径穿越（pkginstall manifest） 🟠

`installer.go`：

- L233 `zipPath = filepath.Join(pkgDir, spec.Path)`
- L243 `srcDir = filepath.Join(tmpDir, spec.Subpath)`
- L311 `jsonPath := filepath.Join(pkgDir, spec.File)`

**未校验** `spec.Path` / `spec.Subpath` / `spec.File` 是否含 `..`、是否绝对路径。
攻击者只需在 `aranea-package.yaml` 写：

```yaml
graphs:
  - file: ../../../etc/passwd
```

`os.ReadFile` 即可越界读取 CLI 主机本地文件并将内容 POST 到任意后端
（本场景下后端是用户自己的，但跨用户安装第三方 package 时是真实威胁）。
应在 `ValidateManifest` 加：

```go
if strings.Contains(spec.File, "..") || filepath.IsAbs(spec.File) { return error }
```

### 4.5 git clone 命令注入 🟠

`pkginstall/loader.go:46-65`：

- 使用 `exec.Command("git", args...)`，每个参数独立 argv → **shell 注入不可达** ✓。
- 但 `args = append(args, repoURL, tmpDir)` **未加 `--` 分隔符**。
  git 早期版本对以 `-` 开头的 URL（如 `--upload-pack=…`）会解释为 flag。
  攻击者构造 `aranea pkg install --upload-pack=/tmp/evil` 可能触发 RCE on the user's box。
- 修复：`args = append(args, "--", repoURL, tmpDir)`。
- `ref` 走 `--branch <ref>`，git 内部会校验，相对安全。

### 4.6 MaskToken 边界 🟡

`config/secret.go:9-24`：

- `""` → `""` ✓
- `len < 4` → `"***"` ✓
- `len == 4` → `n = 40/4 = 10 → clamp 4` → `"***" + token[0:]` = **完整泄漏**
- `len == 8` → `n = 5 → clamp 4` → 显示后 4 字符（50% 泄漏）
- `len == 10` → `n = 4` → 40% 泄漏
- 典型 JWT（≥200 字符）`n = 0` → `"***"` ✓

实际 token 都 ≥100 字符，但短 token（如登录失败重定向、PAT 短码）暴露过多。
建议把阈值从 `< 4` 提到 `<= 10`，并 hard cap n ≤ 2。

### 4.7 大文件 OOM 🟡

`installer.go:267, 332 mustReadAll`：把整个 skill zip 读入内存再丢入 multipart buffer。
`config.go SkillConfig.MaxZipMB = 100` 没有在 installer 中校验。
建议改成 `io.Copy(fw, f)`。

---

## 5. 问题清单（按严重度）

### 🔴 Blocker / Critical

1. **WS URL Dial 错误泄漏 token**
   - 证据：`internal/cli/client/ws.go:67` `fmt.Errorf("ws dial %s: %w", u.String(), err)`，其中 `u.String()` 含 `?token=<JWT>`。
   - 修复：构造一份去掉 `token` 的副本用于日志。

2. **`CONFIRM_REQUIRED` 拼写不一致导致退出码错误**
   - 证据：`internal/cli/cmd/session.go:78` 用 `CONFIRM_REQUIRED`，`clierr/clierr.go:64` 只匹配 `CONFIRMATION_REQUIRED` → 实际返回 `ExitNetworkError(3)` 而非 `ExitConflictBlocked(5)`。
   - 修复：统一为 `CONFIRMATION_REQUIRED`。

3. **HTTP 客户端无 Timeout**
   - 证据：`internal/cli/client/http.go:50` `http.DefaultClient` 无超时；CI / 受限网络下会无限挂起。
   - 修复：`&http.Client{Timeout: 60s}`，并暴露 `--timeout` 全局 flag。

### 🟠 High

4. **`ExitCodeOf` 漏覆盖 10+ Code，全部落入 `ExitNetworkError`**
   - 证据：`clierr/clierr.go:60-74` 仅匹配 6 个 Code；实际抛出的 `FILE_READ_ERROR / FILE_PARSE_ERROR / CONFIG_*_INVALID / LOGIN_NO_TOKEN / PKG_*_ERROR / MISSING_CONTENT` 均落入 default。
   - 修复：补充 switch 分支或引入 `CategoryHint` 字段。

5. **路径穿越未校验**
   - 证据：`internal/pkginstall/installer.go:233, 243, 311` 用 `filepath.Join(pkgDir, spec.*)`，`ValidateManifest`（`loader.go:70-81`）只校验 `Version/Metadata.Name`。
   - 修复：拒绝 `..` 和 `filepath.IsAbs`。

6. **git clone 未加 `--` 分隔符**
   - 证据：`internal/pkginstall/loader.go:53` `args = append(args, repoURL, tmpDir)`，repoURL 若以 `-` 开头会被解释为 flag。
   - 修复：`args = append(args, "--", repoURL, tmpDir)`。

7. **`pkginstall.Install` 无 abort-on-error 与回滚**
   - 证据：`installer.go:49-119` 步骤间不中断；MCP 创建成功后 Skill 失败不回滚。
   - 修复：增加 `--strict` flag；记录 ledger 用于 `aranea pkg uninstall <name>`。

8. **6 处 `fmt.Errorf` 绕过 CLIError**
   - 证据：`config_cmd.go:92`、`login.go:41`、`chat.go:32`、`import.go:75,82,135`。
   - 修复：统一包装为 `&cli.CLIError{Code:..., Cause: err}`。

### 🟡 Medium

9. **MaskToken 对短 token 暴露过多**
   - 证据：`config/secret.go:13-22`，4 字符 token 全部明文。
   - 修复：阈值改 `<= 10`，n hard cap 2。

10. **`mustReadAll` 整块加载 skill zip**
    - 证据：`installer.go:267,332` 把多兆字节 zip 读入内存。
    - 修复：`io.Copy(fw, f)`；同时按 `SkillConfig.MaxZipMB` 限制。

11. **`filepath.Join(os.TempDir(), "skill-*.zip")` 字面量 `*`**
    - 证据：`installer.go:245` 把 `*` 视为普通字符 → 多次安装路径冲突 / race。
    - 修复：`os.CreateTemp(os.TempDir(), "skill-*.zip")`。

12. **HTTP 重试不区分 429 / 不读 Retry-After / 无 jitter**
    - 证据：`client/retry.go:30-39`。
    - 修复：429 单独处理，读 `Retry-After`，加 ±20% jitter。

13. **`cmd/aranea/main.go` 无 panic recover**
    - 证据：`cmd/aranea/main.go:35-45`。
    - 修复：在 `main()` 顶层 `defer` 中 recover 并打印精简 stack（去掉局部变量）。

### 🟢 Low

14. **7 个资源 cmd 文件 ~52% 重复**（详见 §1）。
    - 修复：抽 generic helper，预计削 ~400 行。

15. **`graph.go:209` `os.WriteFile(outputPath, b, 0644)`**：导出文件权限 0644，可能包含敏感 graph 配置；至少应 0600 或可配置。

16. **`pluginConfigSetCmd` 强制要求 `--config`**（`plugin.go:125` `MarkFlagRequired`），但 default 已是 `"{}"`；二者冲突，UX 不一致。

### 🔵 Nit

17. `team.go:182` 等多个 helper 分隔线注释含乱码（`???`），疑似编码问题，应统一 UTF-8。

18. `pkg.go:50-122` `pkgInstallCmd` 直接用 `fmt.Fprintf(os.Stdout, ...)` 输出进度，绕过 `cc.Printer` —— JSON 模式下污染输出。

19. `ws.go:97` 满 channel 丢弃事件无日志（"drop if channel is full"），调试时难以发现。

20. `cmd/session.go:154-160` `sessionToRow` 中 `last_msg_at` 旁边的空格对齐不一致（与其他 cmd 风格略偏）。

---

## 6. 评分

| 维度 | 分数 | 备注 |
|---|---:|---|
| 代码质量    | **6.0/10** | 结构清晰、错误类型一致性强，但 7 个资源 cmd 文件约 52% 模板重复未抽取；helper 命名/分隔线含乱码。 |
| 错误处理    | **5.5/10** | retryDoer + CLIError 设计良好，但 ExitCodeOf 漏覆盖 10+ 实际使用的 Code、无全局 HTTP timeout、pkginstall 无 abort/rollback、main 无 panic recover。 |
| 安全        | **5.5/10** | Token 文件权限、Authorization log mask 都到位；但 WS URL 错误泄漏完整 JWT、pkginstall 缺路径穿越校验、git clone 无 `--`、MaskToken 短 token 暴露过多。 |

修复 §5 的 3 个 🔴 + 5 个 🟠 后预期可拉到 8/8/8。
