# Tools Phase 5 — 工作区统一 Code Review

> **评分**：90 / 100 | **风险等级**：P2（可合并使用）  
> **范围**：`workspace_root` 统一、hostexec 绑定、`workdir` 兼容、tool_confirm、`workspace_exec` 装配  
> **依据**：[docs/README.md](../README.md) · [AI-DEVELOPMENT-SPECIFICATION.md](../guides/AI-DEVELOPMENT-SPECIFICATION.md)  
> **审查时间**：2026-05-22

**关联文档**：[23-tools-development.md §Phase 5](../需求/23-tools-development.md#phase-5工具工作区统一p0) · [23 tools.design.md §7.8](../需求/23%20tools.design.md#78-工具工作区统一phase-5) · [changelog](../changelog/2026-05-22-Tools-Phase5-Workspace-Unification.md)

---

## 总评

| 维度 | 评级 | 说明 |
|------|------|------|
| 架构分层 | **优秀** | 解析在 `internal/agent/tool_assembly.go`；装配在 `internal/tools`；框架 hostexec 不改动 |
| 单一职责 | **良好** | `hostexecnorm` 仅参数归一；`applyToolWorkspaceDirs` 仅目录注入 |
| 业务逻辑 | **良好** | 优先级链清晰；无效 shell 目录 warn 后回退；confirm 覆盖 `exec_command` |
| 代码质量 | **良好** | 单测覆盖装配与 confirm；无 import cycle |
| 文档一致性 | **良好** | 设计 §7.8 矩阵、development Phase 5、execution-plan 已同步 |
| 测试 | **中等** | 缺 E2E Chat+shell 联调脚本；Windows 预存 file 测试有路径分隔符失败（与 Phase 5 无关） |

**结论**：Phase 5 **已完成**；日常 shell 与 file 工具共用工作区根，符合 Cursor 式产品预期。

---

## 架构（符合双框架分工）

```
internal/agent/tool_assembly.go
  resolveToolWorkspaceRoot → resolveAgentFilesystemDir
  applyToolWorkspaceDirs   → FilesystemDir / ShellExecDir / ClaudeCodeDir
        ↓
internal/tools/trpc/toolsets.go   ToolsetConfig 桥接
internal/tools/trpc/runtime_config.go  shell_exec base_dir override
        ↓
internal/tools/toolset.go
  hostexec.NewToolSet(WithBaseDir) + hostexecnorm.WrapToolSet
        ↓
pkg/trpc-agent-go/tool/hostexec   exec_command
```

**红线检查**：通过 — `internal/biz` 无 trpc import；`internal/server` 无 Runner 调用。

---

## 做得好的

1. **单次解析、多处注入** — `applyToolWorkspaceDirs` 避免 file/shell 各算各的 CWD。
2. **Override 可覆盖 shell 根** — `runtime_config` 支持 `base_dir` / `working_dir` 等键。
3. **confirm 闭环** — `runtimeConfirmAliases` + `catalogRequiresConfirm` 双向覆盖 `exec_command`。
4. **legacy 参数** — `hostexecnorm.NormalizeExecArgs` 独立包，无 `internal/tools` 循环依赖。
5. **workspace_exec 边界** — registry `Factory` 返回 `nil,nil` + 注释，避免 nil executor 误挂载。

---

## 残余风险（P2 / P3）

| ID | 优先级 | 问题 | 建议 |
|----|--------|------|------|
| WS-P2-01 | P2 | `workdir` 可为 OS 绝对路径（框架 hostexec 行为） | Prompt 已引导相对路径；P3 可加 workspace 子路径校验 |
| WS-P2-02 | P2 | `resolveToolWorkspaceRoot` 仅为 `resolveAgentFilesystemDir` 别名 | 可接受（语义命名）；或合并并保留注释 |
| WS-P2-03 | P2 | 仅启用 shell 未启用 file 时仍 mkdir workspace | 符合预期；文档已说明 |
| WS-P3-01 | P3 | 缺自动化 E2E：`save_file` + `exec_command` 同相对路径 | 手工 Web 联调清单见 development §Phase 5 验收 |

---

## 影响域

| 域 | 影响 |
|----|------|
| 启用 `shell_exec` 的 Agent | 默认 cwd 从进程目录 → workspace_root |
| 已有依赖「进程 CWD」的脚本/Agent | 行为变更；需显式 `workdir` 或调整 workspace |
| file / claude_code | 默认同根；Override 仍可独立目录 |
| catalog / Proto / 前端 | 无破坏性变更 |
| `workspace_exec` | 不再通过 catalog 单独 nil 挂载 |

---

## 测试

```bash
go test ./internal/tools/... ./internal/agent/... -run "hostexec|ShellExec|Confirm" -count=1
go test ./internal/tools/testexec/... -count=1
make runtime-boundary
```

---

## 文档同步清单

- [x] `23-tools-development.md` Phase 5 ✅
- [x] `23 tools.design.md` §7.8 矩阵
- [x] `execution-plan.md` TW-5-xx
- [x] `23-tools-review.md` 综合更新
- [x] 本 review 文档
