# Plugin 插件 — 开发计划

> **版本**：2026-05-17 | **状态**：✅ 端到端可用
> **需求**：[22 plugin.md](./22%20plugin.md) · **设计**：[22 plugin.design.md](./22%20plugin.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：—

---

## 1. 模块定位

Plugin 插件系统：支持通过 trpc 协议加载外部插件，扩展 Agent 的工具、Skill 和 Hook 能力。

**代码锚点**：
- `api/kratos/plugin/v1/` — Plugin CRUD RPC
- `internal/service/plugin.go` — PluginService
- `internal/biz/plugin.go` — PluginUsecase
- `internal/data/plugin.go` — PluginRepo
- `internal/plugin/trpc/runtime.go` — plugintrpc.Runtime
- `internal/agent/trpc_build.go` — Plugin 注入

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| Plugin CRUD | ✅ | Create/Update/Delete/Get/List |
| Plugin 运行时 | ✅ | `plugintrpc.Runtime` + trpc 连接 |
| Plugin 注入 | ✅ | `BuildTRPCLLMAgent` 中 `WithPlugins` |
| Plugin 工具发现 | ✅ | 连接 Plugin → 列出工具 |
| Plugin Hook | ✅ | BeforeTurn / AfterTurn Hook |
| 前端管理 | ✅ | Plugin 设置页 |

---

## 3. 差距与优化

1. **P2**：Plugin 无沙箱隔离，恶意插件可能影响主进程安全。
2. **P3**：Plugin 无版本管理，更新后无法回滚。
3. **P3**：Plugin 无市场/分享机制。

---

## 4. 开发阶段

- **Phase 1**：Plugin 沙箱隔离（进程级隔离）
- **Phase 2**：Plugin 版本管理
- **Phase 3**：Plugin 市场

---

## 5. 任务清单

| # | 任务 | 优先级 | EP |
|---|------|--------|-----|
| 1 | Plugin 进程隔离方案设计 | P2 | — |
| 2 | Plugin 版本表 + 回滚 API | P3 | — |
| 3 | Plugin 市场前端页面 | P3 | — |

---

## 6. 验收标准

- [ ] Plugin 运行在独立进程中，崩溃不影响主进程
- [ ] Plugin 可管理多个版本并回滚

---

## 7. 依赖与风险

- 进程隔离增加通信开销
- Plugin 市场需与 Ecosystem 模块联动
