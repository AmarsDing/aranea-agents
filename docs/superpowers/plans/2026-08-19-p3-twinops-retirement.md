# P3 aranea 内置 twinops 工具退役实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 依据总纲 §3.1.2 三阶段迁移路线 P3 阶段，在 aranea 侧彻底移除 17 个内置 twinops 工具（`twin_alarm_query` / `twin_alarm_get` / `twin_alarm_ack` / `twin_line_status` / `twin_line_events` / `twin_device_get` / `twin_device_metrics` / `twin_remediation_status` / `twin_device_search` / `twin_alarm_rule_get` / `twin_collector_status` / `twin_line_probe` / `twin_inspection_query` / `gns3_health_check` / `gns3_exec` / `gns3_fault_inject` / `gns3_fault_clear`），包括：① `builtin_tools_seed.go` 移除 twinops 种子行 + reseed DDL 迁移；② `internal/tools/twinops/` 目录标记废弃；③ `twin_openapi_compat.go` 不再路由 twinops 相关调用。确保 MCP 单通道收敛后 aranea 工具面干净、无残留审计分裂。

**Architecture:** aranea 内置 twinops 工具分三层：① `internal/data/builtin_tools_seed.go` 的 `builtinPlatformToolSeeds` 数组声明工具种子（启动时 INSERT 到 `tools` 表，`ON CONFLICT DO NOTHING` 幂等）；② `internal/tools/twinops/` 实现工具逻辑（`NewToolset` 返回 17 个 `trpctool.Tool`，经 `tool_assembly.go` 注入 Agent 运行时）；③ `internal/service/twin_openapi_compat.go` 提供 OpenAPI 兼容端点（twinmonitor 调用 aranea 的 REST 控制面，与 twinops 工具调用路径无关但需确认无耦合）。退役按「种子移除 → DDL reseed → 代码废弃」顺序执行，确保存量库与新库一致。

**Tech Stack:** Go + aranea-agents（PG `tools` 表 + DDL 迁移注册表 + trpc-agent-go 工具框架）。

**前置依赖：** P2（双跑切换）已完成——12 预设 Agent 的 `tool_whitelist` 全部指向 MCP 工具键，remediate 图节点已改用 MCP 工具，E2E 验证通过（`test/ts10-gns3` 扩展）。**若 P2 未完成，禁止执行本计划**（会导致运维 Agent 无工具可用）。

---

## 全局约定

- **TDD 铁律**：每个 Task 先写失败测试/验证脚本，再补实现。
- **验证命令**（每个 Task 收尾必跑）：`cd f:/myproject/aranea-agents && go build ./cmd/... ./internal/...`（排除存量破损 fixture `test/b1t3-gate`）。
- **DDL 迁移铁律**：新增迁移版本号必须全局唯一且递增（受 `TestMigrationVersionsGloballyUnique` 守卫）；种子函数幂等（`ON CONFLICT DO NOTHING`），重跑安全。
- **commit 风格**：`feat(tools): ...` 或 `refactor(tools): ...`（参照 `git log --oneline` 既有前缀惯例）。

---

## Task 1：T1 `builtin_tools_seed.go` 移除 17 个 twinops 工具种子行

**目标**：从 `builtinPlatformToolSeeds` 数组中删除全部 twinops 工具种子（`twin_*` 13 个 + `gns3_*` 4 个），确保新环境 seed 后 `tools` 表无 twinops 工具。

**Files:**
- Modify: `f:/myproject/aranea-agents/internal/data/builtin_tools_seed.go`

- [ ] **Step 1.1 定位 twinops 种子区块**

```bash
cd f:/myproject/aranea-agents
grep -n "twinops" internal/data/builtin_tools_seed.go
# 预期命中：TwinOps 工具集注释行 + 17 个 {key: "twin_*"/"gns3_*"} 种子行
```

- [ ] **Step 1.2 删除 twinops 种子区块**

在 `builtin_tools_seed.go` 中删除从 `// TwinOps 工具集（方案10 §三 v1.1，17 个）` 注释行开始到 `{key: "gns3_fault_clear", ...}` 行结束的连续区块（含 17 个种子行与注释）。删除后确保 `builtinPlatformToolSeeds` 数组语法完整（前后元素逗号正确）。

> **注意**：保留 `officecli_*` 与 `coding_*` 等其他工具集种子行不变。

- [ ] **Step 1.3 编译验证**

```bash
cd f:/myproject/aranea-agents
go build ./cmd/... ./internal/...
# 预期：0 错误
```

- [ ] **Step 1.4 git commit**

```bash
cd f:/myproject/aranea-agents
git add internal/data/builtin_tools_seed.go
git commit -m "$(cat <<'EOF'
feat(tools): 移除 17 个内置 twinops 工具种子（总纲 P3）

- 删除 twin_* 13 个 + gns3_* 4 个种子行
- 新环境 seed 后 tools 表无 twinops 工具
- 种子函数幂等（ON CONFLICT DO NOTHING），重跑安全

Refs: docs/superpowers/specs/2026-08-19-aranea-twinmonitor-deep-fusion-design.md §3.1.2 P3
EOF
)"
```

---

## Task 2：T2 新增 reseed DDL 迁移（`builtin_platform_tools_twinops_retire`）

**目标**：在 `ddl_migration_registry.go` 新增一个 DDL 迁移，调用 `ddlBuiltinPlatformTools` 函数对存量库执行 reseed，删除 `tools` 表中 twinops 工具行（`category='twinops'` 或 key 前缀匹配）。

**Files:**
- Modify: `f:/myproject/aranea-agents/internal/data/ddl_migration_registry.go`

- [ ] **Step 2.1 确认最新迁移版本号**

```bash
cd f:/myproject/aranea-agents
grep -n "Version.*2026" internal/data/ddl_migration_registry.go | tail -5
# 预期输出：最新版本号为 20261230（agent_runtime_reply_reminder）
```

- [ ] **Step 2.2 新增迁移版本 `20261231`**

在 `ddl_migration_registry.go` 的 `20261230` 迁移之后追加：

```go
	// 20261231 builtin_platform_tools_twinops_retire（总纲 P3 MCP 单通道收敛）：
	// 17 个 twinops 工具种子已从 builtin_tools_seed.go 移除，但存量库 tools 表仍有残留行
	// （20261216 reseed 插入）。本迁移调用 ddlBuiltinPlatformTools 幂等重跑种子函数，
	// 该函数内部按 category='twinops' 或 key 前缀（twin_*/gns3_*）删除残留行。
	// 种子函数幂等（ON CONFLICT DO NOTHING + catalog/registry UPDATE），重跑安全。
	{Version: 20261231, Name: "builtin_platform_tools_twinops_retire", Func: ddlBuiltinPlatformTools},
```

- [ ] **Step 2.3 修改 `ddlBuiltinPlatformTools` 支持 twinops 退役清理**

在 `builtin_tools_seed.go` 中找到 `ddlBuiltinPlatformTools` 函数，在其末尾（全部种子 upsert 完成后）追加 twinops 清理逻辑：

```go
	// 总纲 P3：清理已退役的 twinops 工具行（17 个：twin_* 13 + gns3_* 4）。
	// 仅当种子中已无 twinops 工具时执行（防御性：避免误删未退役版本）。
	twinopsRetired := true
	for _, seed := range builtinPlatformToolSeeds {
		if seed.category == "twinops" {
			twinopsRetired = false
			break
		}
	}
	if twinopsRetired {
		// 按 category 删除（twinops 工具统一 category='twinops'）
		if _, err := db.ExecContext(ctx,
			`DELETE FROM tools WHERE category = 'twinops'`); err != nil {
			return fmt.Errorf("delete retired twinops tools: %w", err)
		}
		// 按 key 前缀兜底（防御 category 不一致的残留）
		if _, err := db.ExecContext(ctx,
			`DELETE FROM tools WHERE key LIKE 'twin\_%' OR key LIKE 'gns3\_%'`); err != nil {
			return fmt.Errorf("delete retired twinops tools by key prefix: %w", err)
		}
	}
```

> **注意**：`db` 变量类型需与 `ddlBuiltinPlatformTools` 函数签名一致（通常是 `*sql.DB` 或 ent 客户端）；`ctx` 为函数上下文参数。若函数签名不同，按实际签名调整。

- [ ] **Step 2.4 编译验证**

```bash
cd f:/myproject/aranea-agents
go build ./cmd/... ./internal/...
# 预期：0 错误
```

- [ ] **Step 2.5 迁移版本唯一性测试**

```bash
cd f:/myproject/aranea-agents
go test ./internal/data/... -run TestMigrationVersionsGloballyUnique -v -count=1
# 预期：PASS（20261231 全局唯一）
```

- [ ] **Step 2.6 git commit**

```bash
cd f:/myproject/aranea-agents
git add internal/data/ddl_migration_registry.go internal/data/builtin_tools_seed.go
git commit -m "$(cat <<'EOF'
feat(tools): 新增 twinops 退役 reseed DDL 迁移（总纲 P3）

- 新增 20261231 builtin_platform_tools_twinops_retire 迁移
- ddlBuiltinPlatformTools 追加 twinops 清理逻辑（category + key 前缀双保险）
- 存量库 tools 表 twinops 残留行经迁移幂等删除

Refs: docs/superpowers/specs/2026-08-19-aranea-twinmonitor-deep-fusion-design.md §3.1.2 P3
EOF
)"
```

---

## Task 3：T3 `internal/tools/twinops/` 目录标记废弃（保留 1 个迭代后删除）

**目标**：在 `internal/tools/twinops/` 目录各文件头部添加 `// Deprecated:` 注释，标记该包已废弃（MCP 单通道收敛后由 twinmonitor MCP-Server 承载），保留 1 个迭代周期供回滚参考，后续迭代再物理删除。

**Files:**
- Modify: `f:/myproject/aranea-agents/internal/tools/twinops/twinops.go`
- Modify: `f:/myproject/aranea-agents/internal/tools/twinops/compensation.go`
- Modify: `f:/myproject/aranea-agents/internal/tools/twinops/twinops_test.go`

- [ ] **Step 3.1 `twinops.go` 头部加废弃注释**

在 `twinops.go` 的 `package twinops` 声明之前（文件顶部）追加：

```go
// Package twinops implements the TwinMonitor x GNS3 custom toolset (方案文档
// competition/10 §三，17 个工具)。业务层实现，不动 vendored trpc 框架。
//
// Deprecated: 总纲 §3.1.2 P3 阶段已将该 17 个工具迁移至 twinmonitor 13-aiops
// MCP-Server（gns3 域 4 个 + network 域 3 个 + ops 域 3 个 + alarm/asset/metric
// 等既有域 7 个）。aranea 作为 MCP Host 经 SSE 通道消费，本包保留 1 个迭代
// 周期供回滚参考，后续迭代物理删除。新代码请使用 MCP 工具键（如 gns3.exec、
// network.line_status、ops.remediation_status）。
```

（保留原有包注释其余内容不变）

- [ ] **Step 3.2 `compensation.go` 头部加废弃注释**

在 `compensation.go` 的 `package twinops` 声明之前追加：

```go
// Deprecated: 见 twinops.go 包级废弃注释。gns3_fault_inject/fault_clear 补偿对
// 已由 twinmonitor MCP-Server 侧 gns3 域工具承载（gns3.fault_inject/fault_clear），
// aranea 经 MCP 通道调用，补偿逻辑由 13 侧 MCP 安全层与 aranea 既有 inverse 框架
// （按 MCP 工具键注册）协同实现。
```

- [ ] **Step 3.3 `twinops_test.go` 头部加废弃注释**

在 `twinops_test.go` 的 `package twinops` 声明之前追加：

```go
// Deprecated: 见 twinops.go 包级废弃注释。本测试文件保留 1 个迭代周期，
// 后续随包物理删除一并移除。MCP 工具的等价测试见 twinmonitor 13-aiops 侧
// mcp_call_test.go（gns3/network/ops 域分支）。
```

- [ ] **Step 3.4 编译验证（含废弃警告）**

```bash
cd f:/myproject/aranea-agents
go build ./cmd/... ./internal/...
# 预期：0 错误；可能产生 deprecated 警告（不影响构建）
go vet ./internal/tools/twinops/...
# 预期：PASS（废弃注释不影响 vet）
```

- [ ] **Step 3.5 git commit**

```bash
cd f:/myproject/aranea-agents
git add internal/tools/twinops/
git commit -m "$(cat <<'EOF'
refactor(tools): internal/tools/twinops/ 标记废弃（总纲 P3）

- 17 个工具已迁移至 twinmonitor MCP-Server（gns3/network/ops 等域）
- 保留 1 个迭代周期供回滚参考，后续迭代物理删除
- 新代码请使用 MCP 工具键（gns3.exec / network.line_status / ops.*）

Refs: docs/superpowers/specs/2026-08-19-aranea-twinmonitor-deep-fusion-design.md §3.1.2 P3
EOF
)"
```

---

## Task 4：T4 确认 `twin_openapi_compat.go` 无 twinops 工具调用耦合

**目标**：确认 `internal/service/twin_openapi_compat.go` 的 OpenAPI 兼容端点（twinmonitor 调 aranea 的 REST 控制面）与 twinops 工具调用路径完全解耦，无残留路由依赖。

**Files:**
- Verify: `f:/myproject/aranea-agents/internal/service/twin_openapi_compat.go`

- [ ] **Step 4.1 检查 twin_openapi_compat.go 是否引用 twinops 工具**

```bash
cd f:/myproject/aranea-agents
grep -n "twinops\|twin_alarm\|gns3_\|twin_line\|twin_device\|twin_remediation\|twin_collector\|twin_inspection" internal/service/twin_openapi_compat.go
# 预期：无命中（该文件只处理 Agent/Graph/Run CRUD 与记忆写入，不涉及工具调用）
```

- [ ] **Step 4.2 若存在耦合则移除**

若 Step 4.1 有命中（如某个端点直接调用 twinops 工具），则删除该耦合代码并确保端点功能不受影响（twinmonitor 侧已改用 MCP 通道）。

- [ ] **Step 4.3 编译验证**

```bash
cd f:/myproject/aranea-agents
go build ./cmd/... ./internal/...
# 预期：0 错误
```

- [ ] **Step 4.4 git commit（若有改动）**

```bash
cd f:/myproject/aranea-agents
git add internal/service/twin_openapi_compat.go
git commit -m "$(cat <<'EOF'
refactor(service): 移除 twin_openapi_compat.go 中 twinops 工具调用耦合（总纲 P3）

- REST 控制面与工具调用路径完全解耦
- twinmonitor 侧已改用 MCP 通道消费工具

Refs: docs/superpowers/specs/2026-08-19-aranea-twinmonitor-deep-fusion-design.md §3.1.2 P3
EOF
)"
```

（若 Step 4.1 无命中，跳过本 commit，在最终报告中注明「无耦合，无需改动」）

---

## Task 5：T5 全量回归测试与 E2E 验证

**目标**：运行全量回归测试（`go test ./...` + `go build`），并执行 `test/ts10-gns3` 扩展 E2E 验证 MCP 通道下 remediate 图闭环正常（取证 → fault_clear → 复核 → 告警恢复）。

**Files:**
- Run: `f:/myproject/aranea-agents/test/ts10-gns3/`（E2E 测试脚本）

- [ ] **Step 5.1 全量单元测试**

```bash
cd f:/myproject/aranea-agents
go test ./internal/... -count=1 -timeout=300s
# 预期：PASS（排除 test/b1t3-gate 存量破损 fixture）
```

- [ ] **Step 5.2 全量构建**

```bash
cd f:/myproject/aranea-agents
go build ./cmd/... ./internal/...
# 预期：0 错误
```

- [ ] **Step 5.3 E2E 验证（MCP 通道 remediate 闭环）**

```bash
cd f:/myproject/aranea-agents
# 前提：P2 已完成，12 预设 Agent tool_whitelist 已切 MCP，remediate 图节点已改用 MCP 工具
# 运行 ts10-gns3 E2E 测试（参照项目记忆「图执行必须走框架 runner」验证流程）
python test/ts10-gns3/run_e2e.py --scenario remediate --channel mcp
# 预期输出：
# [PASS] gns3 取证（gns3.exec 经 MCP 通道）
# [PASS] fault_clear（gns3.fault_clear 经 MCP 通道，HITL interrupt → 13 审批 → Resume）
# [PASS] 复核（gns3.health_check 经 MCP 通道）
# [PASS] 告警 37s 自动 recovered
# [PASS] 全程 <200s / <15 次 LLM 请求（对比旧路径 600s 超时+几十次空转）
# [PASS] 零守卫拦截
```

- [ ] **Step 5.4 验证 tools 表无 twinops 残留**

```bash
psql "$ARANEA_PG_DSN" -c "SELECT COUNT(*) FROM tools WHERE category='twinops' OR key LIKE 'twin\_%' OR key LIKE 'gns3\_%';"
# 预期：0（迁移 20261231 已清理存量库）
```

- [ ] **Step 5.5 git commit（测试报告）**

```bash
cd f:/myproject/aranea-agents
git add test/ts10-gns3/reports/p3-retirement-e2e-$(date +%Y%m%d).md || true
git commit -m "$(cat <<'EOF'
test(tools): P3 twinops 退役全量回归与 E2E 验证报告

- go test ./internal/... PASS
- go build ./cmd/... ./internal/... 0 错误
- MCP 通道 remediate 闭环验证通过（取证→fault_clear→复核→恢复）
- tools 表 twinops 残留行清零

Refs: docs/superpowers/specs/2026-08-19-aranea-twinmonitor-deep-fusion-design.md §3.1.2 P3
EOF
)"
```

---

## 验收清单（Sign-off）

- [ ] T1：`builtin_tools_seed.go` 移除 17 个 twinops 种子行，编译通过。
- [ ] T2：新增 `20261231 builtin_platform_tools_twinops_retire` 迁移，`ddlBuiltinPlatformTools` 追加 twinops 清理逻辑，迁移版本唯一性测试 PASS。
- [ ] T3：`internal/tools/twinops/` 三个文件头部添加 `// Deprecated:` 注释，编译无错误（废弃警告可接受）。
- [ ] T4：`twin_openapi_compat.go` 确认无 twinops 工具调用耦合（或已移除）。
- [ ] T5：全量回归测试 PASS，E2E 验证 MCP 通道 remediate 闭环正常，tools 表无 twinops 残留。
- [ ] 全局：`go build ./cmd/... ./internal/...` 无编译错误。

---

## 发现的总纲与代码不一致之处

1. **`twin_openapi_compat.go` 耦合确认**：总纲 §3.1.2 P3 提到「`twin_openapi_compat.go` 中不再路由 twinops 相关调用」，但该文件实际职责是 twinmonitor → aranea 的 REST 控制面（Agent/Graph/Run CRUD），与 twinops 工具调用路径（aranea 内部 Agent 运行时经 `tool_assembly.go` 挂载）完全分离。计划在 Task 4 中显式验证无耦合，而非假定存在耦合需移除。
2. **`ddlBuiltinPlatformTools` 清理逻辑**：总纲只提到「reseed DDL 迁移」，未明确清理逻辑实现方式。计划按「category + key 前缀双保险」实现（先按 `category='twinops'` 删除，再按 `key LIKE 'twin_%'/'gns3_%'` 兜底），确保存量库残留行清零。
3. **废弃保留周期**：总纲写「保留 1 个迭代后删除」，未明确「1 个迭代」的具体时长。计划按「当前迭代标记废弃，下一迭代物理删除」执行（与项目既有废弃流程一致）。
4. **`test/ts10-gns3` E2E 脚本**：总纲提到「`test/ts10-gns3` 扩展」，但当前代码库中该目录下的 E2E 脚本为 `run_e2e.py`（或其他名称），计划按实际脚本名引用，若不存在则新建 `test/ts10-gns3/run_e2e.py`（参照 `llm_relay.py` 风格）。
