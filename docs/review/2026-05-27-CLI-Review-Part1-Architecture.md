# CLI Review · Part 1 · 架构与红线（2026-05-27）

> 范围：`cmd/aranea/`、`internal/cli/`、`internal/pkginstall/`、`internal/tools/cli_admin/`、`internal/orgimport/`、`internal/data/seed_system_admin.go`。
> 必读规范：`.cursor/rules/trpc-agent-framework-first.mdc`、`AGENTS.md` 红线段、`docs/AGENT_RUNTIME_BOUNDARY.md`（仓库内未提供，按 AGENTS.md 红线段执行）。

---

## 1. 红线扫描结果

对 CLI 二进制及其依赖包扫描禁忌 import：

| 禁忌包 | 在 `cmd/aranea/` 命中 | 在 `internal/cli/` 命中 | 在 `internal/pkginstall/` 命中 | 在 `internal/tools/cli_admin/` 命中 | 在 `internal/orgimport/` 命中 |
|---|---|---|---|---|---|
| `aranea-agents/internal/biz` | — | — | — | — | — |
| `aranea-agents/internal/data` | — | — | — | — | — |
| `aranea-agents/internal/agent` | — | — | — | — | — |
| `aranea-agents/internal/server` | — | — | — | — | — |
| `aranea-agents/internal/service` | — | — | — | — | — |
| `aranea-agents/pkg/trpc-agent-go` | — | — | — | — | — |
| `aranea-agents/internal/conf` | — | — | — | — | — |

**结论**：CLI 进程及其闭包内**无任何禁忌包 import**，红线全部遵循。

补充观察：
- `cmd/aranea/main.go:21-26` 显式注释“mirrors `conf.PGOCLIImportEnabled()` without importing `internal/conf`”，将 ENV 解析复刻一份避免把 kratos proto 带进 CLI 体积——主动避坑，符合“轻量 CLI”意图。
- `internal/tools/cli_admin/` 虽 import 了 `trpc.group/trpc-go/trpc-agent-go/tool`（合规的上游 SDK），但**不**走 `aranea-agents/pkg/trpc-agent-go`；且本包不被 CLI 进程依赖，只在 server 端注入 `__system_admin__` agent 时使用。该 import 不进入 CLI 二进制。
- `internal/data/seed_system_admin.go` 仅依赖 `internal/data/ent`，本身不被 `cmd/aranea/` 或 `internal/cli/**` 引用——属服务侧 seed，合规。

---

## 2. cli_admin Bridge 设计

**评估**：✅ 良好。采用 Deps 注入 + Repository 接口模式，严格隔离 `internal/biz`。

证据（`internal/tools/cli_admin/registry.go`）：

```20:43:internal/tools/cli_admin/registry.go
type Deps struct {
	SkillRepo SkillRepository
	AgentRepo AgentRepository
	APIBaseURL string
	APIToken string
}

type SkillRepository interface {
	ListSkills(ctx context.Context, keyword string, limit, offset int32) ([]SkillItem, int32, error)
	GetSkill(ctx context.Context, id string) (*SkillItem, error)
}

type AgentRepository interface {
	ListAgents(ctx context.Context, keyword string, limit, offset int32) ([]AgentItem, int32, error)
	GetAgent(ctx context.Context, id string) (*AgentItem, error)
}
```

- `cli_admin` 不 import `internal/biz`，由 `internal/service` 注入符合接口的实现，避免循环依赖。
- 文档化在 `registry.go` 头注释中（“concrete implementations are injected from internal/service to avoid circular imports”）——契约显式可读。
- `IsCLIAdminAllowed` 在工具层做了 agent_key 白名单校验，权限边界清晰。

唯一可改进点：`Deps` 直接持有 `APIToken string`——明文长期驻留进程内存，不算严重，但若未来支持 token 轮换需要 callback/provider 形式。

---

## 3. WS token 携带方式

**评估**：🟠 高风险——客户端把 token 放在 URL query。

证据（`internal/cli/client/ws.go:46-69`）：

```46:69:internal/cli/client/ws.go
func (w *WSClient) Dial(ctx context.Context, sessionID string) (*WSConn, error) {
	wsBase := strings.Replace(w.Base, "http://", "ws://", 1)
	wsBase = strings.Replace(wsBase, "https://", "wss://", 1)

	u, err := url.Parse(wsBase + "/v1/ws")
	...
	q := u.Query()
	q.Set("session_id", sessionID)
	if w.Token != "" {
		q.Set("token", w.Token)
	}
	u.RawQuery = q.Encode()
	...
	headers := http.Header{}
	conn, _, err := dialer.DialContext(ctx, u.String(), headers)
```

风险分析：
- URL query 会写入：Nginx/反向代理的 access log、CDN 日志、浏览器历史（如果未来 web 客户端复用）、错误信息（看 `client/ws.go:67` 错误里就把整个 `u.String()` 回显了 → **token 直接出现在错误文案中**）。
- gorilla/websocket 完全支持把 token 放在 header（标准做法）。
- 服务端 `pkg/auth/request_token.go` 的解析顺序是 Cookie → Bearer → query，**已经支持** Authorization Bearer，客户端切换是无侵入的。

修复建议：
1. `headers.Set("Authorization", "Bearer "+w.Token)`，从 URL 中移除 `token` query。
2. `fmt.Errorf("ws dial %s: %w", u.String(), err)` 应只回显 `u.Host+u.Path`，避免把 query 落到错误日志。

---

## 4. REPL 事件契约一致性

REPL `render.go` switch 的 type 字符串 vs 服务端实际发出的 type：

| REPL 期望（render.go） | 服务端定义位置 | 一致性 |
|---|---|---|
| `text_delta` | `internal/event/contract/envelope.go:16` | ✅ |
| `text_done` | `internal/event/contract/envelope.go:17` | ✅ |
| `tool_call` | `internal/event/contract/envelope.go:18` | ✅ |
| `tool_result` | `internal/event/contract/envelope.go:19` | ✅ |
| `runner_completion` | `internal/event/contract/envelope.go:22` | ✅ |
| `run_status` | `internal/event/contract/envelope.go:25` | ✅ |
| `transfer` | `internal/event/contract/envelope.go:21` | ✅ |
| `error` | `EnvelopeTypeError`（contract） | ✅ |
| `pong` | `internal/server/ws.go:608` | ✅ |
| `connected` | `internal/server/ws.go:392` | ✅ |
| `server_shutdown` | `internal/server/ws.go:208` | ✅ |

**结论**：所有事件 type 在服务端均有定义来源，**无不匹配项**。

潜在隐患（非不一致，但是契约脆弱）：
- 字面值在 REPL 端硬编码字符串，未引用 `internal/event/contract` 的常量。若未来 contract 改名（如 `runner_completion` → `runner_done`），编译期不会捕获，只在运行时静默丢事件（命中 `default` 分支被忽略）。
- 建议：把这些字面值集中放到 `internal/event/contract` 的导出常量（或一个轻量级 `eventtypes` 包，不引入框架运行时依赖），REPL 引用之。

---

## 5. 循环依赖

**结论**：✅ 无循环依赖。依赖方向单调。

依赖图（CLI 闭包）：

```
cmd/aranea/main.go
  └─ internal/cli/{cmd,client,config,output,repl,ui,clierr}
       └─ internal/cli/cmd/import.go ─┐
                                       ├─→ internal/orgimport
                                       │     └─ stdlib + yaml.v3（无 aranea-agents/internal/*）
                                       │
                                       └─ (无 pkginstall / cli_admin)

internal/tools/cli_admin（服务端注入侧，非 CLI 闭包）
  └─ internal/pkginstall
       └─ internal/orgimport
```

证据：
- `internal/orgimport/{loader,planner,validator,applier,spec}.go` 的 import block 仅含 stdlib + `gopkg.in/yaml.v3`，无任何 `aranea-agents/internal/*`。
- `internal/pkginstall/{installer,manifest}.go` 仅 import `aranea-agents/internal/orgimport`（单向），不反向依赖 cli。
- `internal/cli/cmd/import.go` 只 import `internal/orgimport`，未触及 pkginstall 或 cli_admin——CLI 端只走 HTTP/spec 流，不本地执行包安装；包安装由 server 端 `cli_admin` 工具触发。
- `cli_admin` 不被任何 cli 包引用，且不反向 import cli。

---

## 6. 问题清单（按严重度）

- 🔴 **Blocker**: 无。
- 🟠 **High**:
  1. **WS 客户端在 URL query 携带 token**（`internal/cli/client/ws.go:57`），且错误信息回显完整 URL（`:67`），存在日志/反向代理日志泄漏风险。修复见 §3。
- 🟡 **Medium**:
  2. **REPL 事件 type 字面值未集中常量化**（`internal/cli/repl/render.go:47-147`），契约重构会静默丢事件。建议引入 `internal/event/contract` 的导出常量并被双方引用。
  3. **`pgoImportEnabled()` 复刻 `conf` 逻辑**（`cmd/aranea/main.go:23-26`）——避坑思路对，但若 `conf.PGOCLIImportEnabled()` 行为演化（如读 TOML 配置而非 ENV），CLI 会漂移。建议抽出 `internal/featureflag` 轻量包供双方共享，或在 conf 那一行加 cross-reference 注释。
- 🟢 **Low**:
  4. `cli_admin.Deps` 直接持 `APIToken string` 明文，未来 token 轮换需改为 provider 回调。
  5. `internal/cli/cmd/import.go` 走 HTTP applier，与 `internal/pkginstall` 的 server 端 installer 是两条独立路径，未来 sync 维护成本需注意。

---

## 7. 评分

- 架构合理性：**8.5/10**（Bridge 隔离干净、依赖单调、避免把框架体积带进 CLI）。
- 红线遵循度：**10/10**（七个禁忌包全员零命中）。

**总结**：CLI 的架构与红线遵循度优秀——七个禁忌 import 全部零命中，cli_admin 用 Repository 接口干净隔离 biz，orgimport/pkginstall 单向依赖、无循环。唯一明显问题是 WS 客户端用 URL query 传 token（且把完整 URL 回显进错误信息），属生产环境应在合并前修复的 High 项；服务端早已支持 `Authorization: Bearer`，切换零侵入。
