# CLI 命令行 — 开发计划

> **版本**：2026-05-17 | **状态**：❌ 未实现
> **需求**：[25 cli.md](./25%20cli.md) · **设计**：[25 cli.design.md](./25%20cli.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：—

---

## 1. 模块定位

CLI 命令行工具：提供命令行界面管理 Agent/Team/Tool/Skill 等资源，支持脚本化操作和 CI/CD 集成。

**代码锚点**：
- 无对应代码实现

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| CLI 框架 | ❌ | 无 cobra/urfavecli 集成 |
| Agent 管理 | ❌ | 无 CLI 命令 |
| Team 管理 | ❌ | 无 CLI 命令 |
| Tool 管理 | ❌ | 无 CLI 命令 |
| 对话交互 | ❌ | 无 CLI 对话模式 |

---

## 3. 差距与优化

1. **P2**：CLI 工具完全未实现，开发者无法通过命令行管理系统。
2. **P3**：无 REPL 交互模式，无法在终端中与 Agent 对话。

---

## 4. 开发阶段

- **Phase 1**：CLI 框架搭建（cobra）+ 基础 CRUD 命令
- **Phase 2**：CLI 对话交互模式
- **Phase 3**：CLI 插件系统

---

## 5. 任务清单

| # | 任务 | 优先级 | EP |
|---|------|--------|-----|
| 1 | `cmd/aranea/`：cobra 框架 + root 命令 | P2 | — |
| 2 | Agent/Team/Tool CRUD 子命令 | P2 | — |
| 3 | CLI 对话模式（REPL） | P3 | — |
| 4 | CLI 输出格式化（JSON/Table） | P3 | — |

---

## 6. 验收标准

- [ ] `aranea agent list` 可列出所有 Agent
- [ ] `aranea chat <agent_id>` 可与 Agent 对话
- [ ] `aranea --help` 显示完整命令列表

---

## 7. 依赖与风险

- CLI 需与 HTTP API 对齐
- 需考虑认证 token 管理
