# Agent 提示文件 — 开发计划

> **版本**：2026-05-17 | **状态**：✅ 端到端可用
> **需求**：[6 agent-setting-file.md](./6%20agent-setting-file.md) · **设计**：[6 agent-setting-file.design.md](./6%20agent-setting-file.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：—

---

## 1. 模块定位

Agent 提示文件管理：用户可为 Agent 上传/编辑提示文件（markdown/text），文件内容在 Agent 构建时注入 system prompt。

**代码锚点**：
- `api/kratos/agent/v1/agent.proto` — PromptFile CRUD RPC
- `internal/data/ent/schema/agent_prompt_file.go` — Ent Schema
- `internal/biz/agent_usecase.go` — PromptFile 管理
- `internal/agent/trpc_build.go` — prompt 文件注入

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| PromptFile CRUD | ✅ | Create/Update/Delete/List |
| Agent 构建注入 | ✅ | `BuildTRPCLLMAgent` 读取 prompt 文件 |
| 前端管理 | ✅ | Agent 设置页提示文件区域 |

---

## 3. 差距与优化

1. **P3**：提示文件无版本历史，编辑后无法回滚。
2. **P3**：提示文件无语法高亮编辑器（前端为纯文本 textarea）。

---

## 4. 开发阶段

- **Phase 1**：提示文件版本历史（可选）
- **Phase 2**：Markdown 编辑器集成

---

## 5. 任务清单

| # | 任务 | 优先级 | EP |
|---|------|--------|-----|
| 1 | 提示文件版本表 + 历史查询 API | P3 | — |
| 2 | 前端 CodeMirror/Monaco 编辑器 | P3 | — |

---

## 6. 验收标准

- [ ] 提示文件编辑后可查看历史版本
- [ ] 编辑器支持 Markdown 语法高亮

---

## 7. 依赖与风险

无重大依赖。
