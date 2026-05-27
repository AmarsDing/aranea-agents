# CLI 模块 Code Review · 总报告（2026-05-27）

> 三分册详报：
> - [Part 1 · 架构与红线](./2026-05-27-CLI-Review-Part1-Architecture.md)
> - [Part 2 · 代码质量 / 错误 / 安全](./2026-05-27-CLI-Review-Part2-Quality.md)
> - [Part 3 · 功能正确性 / 业务闭环 / 测试](./2026-05-27-CLI-Review-Part3-Correctness.md)
>
> 审查范围：`cmd/aranea/`、`internal/cli/`、`internal/pkginstall/`、`internal/tools/cli_admin/`、`internal/orgimport/`、`internal/data/seed_system_admin.go`。

---

## TL;DR

**一句话**：架构与红线遵循近乎完美（10/10），但**业务闭环未连接**——`cli_admin_*` 工具从未注入 Runner、`skill_install_from_url` 是 stub、`installSkill` 的 zip 路径有字面 `*` bug。**CLI 直接命令路径几乎可用，AI 自动化路径目前跑不通**。

### 雷达评分

| 维度 | 分数 | 速评 |
|---|---:|---|
| 架构合理性 | **8.5/10** | Bridge 隔离干净、依赖单调、CLI 体积控制良好 |
| 红线遵循度 | **10/10** | 七个禁忌 import 全部零命中 |
| 代码质量 | **6.0/10** | 7 个资源 cmd 文件 ~52% 模板重复未抽取 |
| 错误处理 | **5.5/10** | `ExitCodeOf` 漏覆盖 10+ Code、无全局 HTTP timeout |
| 安全 | **5.5/10** | WS URL 泄漏 JWT、路径穿越未防护、git clone 无 `--` |
| 功能正确性 | **4.0/10** | zip 字面 `*` bug 阻断 `skills[].url` 安装 |
| **业务闭环** | **3.0/10** | 🔴 **最关键问题**：系统管家 → cli_admin 工具 → pkginstall 链路断裂 |
| 测试覆盖 | **3.0/10** | repl / pkginstall / cli_admin 三个核心包 0 测试 |

**综合**：**5.5 / 10**。修完 Top 5 后可拉到 8/10。

---

## Top 5 必修（按致命度）

### 🔴 1. `cli_admin.RegisterAll` 从未被装配到 Runner —— AI 闭环致命缺口

- **现状**：`internal/tools/cli_admin/registry.go:71` 的 `RegisterAll(deps)` 全仓 0 调用。DB `tools` 表虽有 8 条 `cli_admin_*` 记录、effective-tools 也会把 `group:cli_admin` 展开成 key 列表，但 trpc-agent-go toolset 里**没有对应的 `function.Tool` 实例**，AI 调用结果为 "tool not found"。
- **影响**：用户在 `aranea chat` 里跟系统管家说"装个 https://github.com/foo/bar"，AI 调工具拿不到实现，整条 Cursor 式工具调用闭环不通。
- **修复**：在 `internal/service` 装配 `__system_admin__` Agent 时显式 `cli_admin.RegisterAll(cli_admin.Deps{...})`，把返回的 `[]trpctool.Tool` 注入该 Agent 的 toolset。

### 🔴 2. `cli_admin_skill_install_from_url` 是 stub

- **现状**：`internal/tools/cli_admin/skill_install_from_url.go:42-48` 构造完 payload 后 `_ = body`，直接返回 `status:"triggered"` 假成功。从未发起 HTTP 调用。
- **影响**：即使 #1 修好，AI 单独调它装 Skill 也只会回假成功。
- **修复**：复用 `pkginstall.installSkill` 逻辑；或移除该工具，引导 AI 用 `cli_admin_pkg_install_from_url`（后者**是真实现**）。

### 🔴 3. `pkginstall.installSkill` 的 zip 临时路径有字面 `*` bug

- **现状**：`internal/pkginstall/installer.go:245` 用 `filepath.Join(os.TempDir(), "skill-*.zip")`，把 `*` 当文件名传给 `zipDir`；`ziputil.go:13-22` 的 `*` 解析只在内部生效不回传给 caller；后续 `os.Open(zipPath)` 在 Linux/macOS 因字面 `*` 失败，在 Windows 文件名非法直接报错。
- **影响**：任何 `skills[].url` 形式的安装实测必然失败 —— CLI 直接命令 `aranea pkg install` 和 #1 修好后的 AI 路径**都被堵死**。
- **修复**：让 `zipDir` 返回真实 path，或调用方先 `os.CreateTemp(os.TempDir(), "skill-*.zip")` 拿到名字再传入。

### 🔴 4. WS 客户端 URL 携带 token 且错误回显完整 URL —— JWT 泄漏

- **现状**：`internal/cli/client/ws.go:57` 把 token 放在 `?token=<JWT>`；`:67` `fmt.Errorf("ws dial %s: %w", u.String(), err)` 错误链含完整 URL → cobra 打印到 stderr → CI 日志 / 反向代理 access log 全部泄漏完整 JWT。
- **影响**：生产环境一次 WS 握手失败就泄密。
- **修复**：
  1. 改用 `headers.Set("Authorization", "Bearer "+w.Token)`（服务端已支持，零侵入）；
  2. 错误格式化用 `u.Host+u.Path` 而非 `u.String()`。

### 🔴 5. `pkginstall` manifest 路径无 `..` / 绝对路径防护

- **现状**：`installer.go:233, 243, 311` 的 `spec.Path` / `spec.Subpath` / `spec.File` 全部直接 `filepath.Join(pkgDir, …)`，`ValidateManifest`（`loader.go:70-81`）只校验 `Version/Metadata.Name`。
- **影响**：恶意 `aranea-package.yaml` 写 `graphs[].file: ../../../etc/passwd` 即可读宿主任意文件并经 `/v1/graph/import` 外泄。
- **修复**：`ValidateManifest` 增加 `strings.Contains(p, "..") || filepath.IsAbs(p)` 拒绝。

---

## Top 5 强烈建议（High，本周内修）

| # | 问题 | 证据 | 修复 |
|---|---|---|---|
| 6 | `ExitCodeOf` 漏覆盖 10+ Code（`FILE_READ_ERROR` / `PKG_*` / `CONFIG_*_INVALID` / `LOGIN_NO_TOKEN`），全部落入 `ExitNetworkError(3)`；`CONFIRM_REQUIRED` 拼写与 `CONFIRMATION_REQUIRED` 不一致 | `clierr/clierr.go:60-74`、`session.go:78` | 补 switch 分支 + 统一拼写 |
| 7 | HTTP 客户端无全局 Timeout（用 `http.DefaultClient`），上游卡死会无限挂起 | `client/http.go:50` | `&http.Client{Timeout: 60s}` + 暴露 `--timeout` flag |
| 8 | `git clone` 未加 `--` 分隔符，`-` 开头的恶意 URL 可被解释为 flag（潜在 RCE） | `pkginstall/loader.go:53` | `args = append(args, "--", repoURL, tmpDir)` |
| 9 | `pkginstall.Install` continue-on-error 但不影响退出码，partial 失败被静默 | `installer.go:91-99`、`cmd/pkg.go:106-122` | 增加 `--strict` 选项 + 失败数 > 0 时退码 ≠ 0 |
| 10 | `aranea chat` 默认不路由到 `__system_admin__`，用户必须显式 `--agent __system_admin__` 才能用系统管家 | `cmd/chat.go:13-50` | chat 默认 `--agent __system_admin__`，或 README 顶部高亮 |

---

## Top 5 可选优化（Medium / Low）

1. **REPL `/slash` 命令缺失**：`/session new\|list\|resume` / `/yes` / `/tools` / `/expand` / `/copy` 全部未实现；`/agent` 不开新 session（违 PRD §4.5）；`/dry-run` 忽略 `on\|off` 参数。
2. **7 个资源 cmd 文件 ~52% 模板重复**：抽 generic helper `loadProtoFromFile[T]` / `confirmAndDo`，预计可削 ~400 行。
3. **REPL 事件 type 硬编码字面值**：未引用 `internal/event/contract` 常量，contract 改名会静默落入 `default` 分支被吞。
4. **REPL 对未知 event 静默丢弃**（`render.go:145-147`），调试期排查困难；至少打 debug 日志。
5. **测试覆盖空白**：`internal/cli/repl/`、`internal/pkginstall/`、`internal/tools/cli_admin/` 三个最核心包 **0 测试**，failure 分支无回归保护——Top 1/2/3 三个 Blocker 没被 CI 拦截正是直接后果。

---

## 关键发现总览

### ✅ 做得好
- **红线遵循 10/10**：CLI 进程及其闭包内**无任何禁忌 import**（`internal/biz` / `data` / `agent` / `server` / `service` / `pkg/trpc-agent-go` / `internal/conf` 七项全员零命中）
- **`cli_admin` Bridge 设计干净**：Deps 注入 + Repository 接口模式严格隔离 biz
- **依赖图单调无环**：cmd/aranea → cli → orgimport，cli_admin/pkginstall 单向依赖 orgimport
- **REPL 事件契约一致**：11 个 event type 全部能在 server 端找到定义源
- **`SeedSystemAdminAgent` 幂等**：用 `ON CONFLICT DO NOTHING`
- **`cli_admin_pkg_install_from_url` 是真实现**（区别于 #2 的 stub）

### ⚠️ 存在但风险可控
- Windows 上 token 文件用 0600 仅 Unix 生效（NTFS user 私有目录默认 ACL 隔离，可接受但应文档化）
- `FetchFromURL` 仅支持 git，无 zip/tar 直链（设计内可作为 P2）
- HTTP 重试无 jitter / 不读 `Retry-After`（429 不重试）

### ❌ 必须修复
见上方 Top 5 + Top 5。

---

## 修复优先级路线图

```
本周必修（Blocker）
├── #1  装配 cli_admin.RegisterAll 到 __system_admin__ Runner    ← 解锁 AI 闭环
├── #2  填实 skill_install_from_url 或移除                        ← 移除歧义
├── #3  修 installSkill 的 zip 字面 * bug                         ← 解锁 pkg install
├── #4  WS 改 Bearer + 错误不含 token                             ← 阻断 JWT 泄漏
└── #5  manifest 路径穿越校验                                      ← 阻断本地文件外泄

本周强烈建议（High）
├── #6  ExitCodeOf 补全 + 统一 CONFIRMATION_REQUIRED
├── #7  HTTP 全局 Timeout + --timeout flag
├── #8  git clone 加 --
├── #9  pkginstall 失败 → 进程退码 ≠ 0
└── #10 chat 默认路由 __system_admin__

下个迭代（Medium）
├── REPL /slash 命令补全
├── 资源 cmd 模板抽 helper
├── 事件 type 常量化
└── repl/pkginstall/cli_admin 三个核心包补测试

P2（Low / Nit）
├── FetchFromURL 支持 zip/tar 直链
├── pkg validate --local 模式
├── MaskToken 短 token 阈值
└── 注释乱码统一 UTF-8
```

---

## 附录 A：依赖图

```
cmd/aranea/main.go
  └─ internal/cli/{cmd, client, config, output, repl, ui, clierr}
       └─ internal/cli/cmd/import.go ──→ internal/orgimport
                                              └─ stdlib + yaml.v3 only

服务端注入侧（非 CLI 闭包）
  internal/service/<system_admin_assembler>
    └─ internal/tools/cli_admin       ← 🔴 #1 缺这条边
         └─ internal/pkginstall
              └─ internal/orgimport
```

## 附录 B：测试覆盖矩阵

| 包 | 测试 | 评 |
|---|---|---|
| `internal/cli/client` | ✅ 4 个 | httptest 覆盖契约 |
| `internal/cli/config` | ✅ 1 个 | 基础 OK |
| `internal/cli/output` | ✅ 1 个（5 golden） | 覆盖面窄 |
| `internal/cli/execute` | ✅ 1 个 | 仅退出码 |
| `internal/cli/clierr` | 🔴 无 | — |
| **`internal/cli/repl`** | 🔴 无 | slash / render / 输入循环裸奔 |
| **`internal/pkginstall`** | 🔴 无 | installer / loader / manifest / ziputil 全部未测 |
| **`internal/tools/cli_admin`** | 🔴 无 | 6 个工具 0 测试 |
| `internal/orgimport` | 🔴 无 | （历史问题） |

## 附录 C：与 PRD / Design 偏差表

| PRD/Design 条款 | 实际实现 | 差距 |
|---|---|---|
| PRD §4.5 `/agent <key>` 开新 session | 仅改本地变量 | 未开新 session |
| PRD §4.5 `/session new\|list\|resume` | 仅显示当前 ID | 三个子命令未实现 |
| PRD §4.5 `/yes` / `/tools` / `/expand` / `/copy` | 未实现 | 4 个命令缺失 |
| PRD §5.2 退出码语义 | 10+ Code 落入 ExitNetworkError | `ExitCodeOf` 漏覆盖 |
| Design §6.2 包安装回滚策略 | 无回滚 ledger | continue-on-error 但不退码 |
| Design §13 红线 | 全部遵守 | ✅ |

---

**总结**：CLI 模块的"架构层"是教科书级别的——红线遵循、依赖单调、接口隔离都很到位；但"业务闭环层"还有一段未连接的最后一公里（Top 5 中 #1/#2/#3）。修完这三处加 WS token 泄漏（#4）和路径穿越（#5），整套"给 AI 一个 URL，一句话装齐"的 Cursor 式体验就能真正跑通。
