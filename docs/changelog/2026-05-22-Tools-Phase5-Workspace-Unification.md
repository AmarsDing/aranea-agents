# Tools Phase 5 — 工具工作区统一

**日期**：2026-05-22  
**模块**：Tools (23)

## 摘要

统一 file 与 shell 的 `workspace_root`，修复 hostexec 在 Server 进程 CWD 执行、`working_dir`/`workdir` 参数不一致、以及 `exec_command` 未纳入 tool_confirm 的问题。不涉及 Desktop App / Electron（53 文档已删除）。

## 实现

| 项 | 说明 |
|----|------|
| `resolveToolWorkspaceRoot` | `internal/agent/tool_assembly.go` 单次解析，file + shell + claude_code 默认同根 |
| `ShellExecDir` | `AssemblyConfig` / `ToolsetConfig` / `runtime_config` 桥接 |
| hostexec `WithBaseDir` | `internal/tools/toolset.go` 绑定工作区 |
| `hostexecnorm` | `working_dir` → `workdir` 兼容；`WrapToolSet` 无 import cycle |
| tool_confirm | `exec_command` ↔ catalog `shell_exec` 别名 |
| prompt | `RuntimeCapabilityCue` 口径与实现一致 |
| TestTool | `testexec/config.go` 传 shell workspace |
| `workspace_exec` | registry 禁止 nil executor 独立挂载 |

## 测试

- `go test ./internal/tools/... ./internal/agent/...`
- `TestAssemble_hostexecUsesShellExecDir`
- `hostexecnorm/normalize_test.go`
- `tool_confirm_gate_catalog_test.go`

## 文档

- [23-tools-development.md](../需求/23-tools-development.md) Phase 5 ✅
- [23 tools.design.md §7.8](../需求/23%20tools.design.md) 工具×目录矩阵
