# Skill 技能 — 开发计划

> **版本**：2026-05-17 | **状态**：✅ 端到端可用
> **需求**：[20 skill.md](./20%20skill.md) · **设计**：[20 skill.design.md](./20%20skill.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：—

---

## 1. 模块定位

Skill 技能系统：管理 Agent 可用的技能包（预定义工具组合 + 提示模板），支持技能的创建、安装、卸载和运行时注入。

**代码锚点**：
- `api/kratos/skill/v1/` — Skill CRUD RPC
- `internal/service/skill.go` — SkillService
- `internal/biz/skill.go` — SkillUsecase
- `internal/data/skill.go` — SkillRepo
- `internal/skill/trpc/` — trpc-agent-go Skill 桥接
- `internal/agent/trpc_build.go` — Skill 注入

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| Skill CRUD | ✅ | Create/Update/Delete/Get/List |
| Skill 安装/卸载 | ✅ | Agent 绑定/解绑 Skill |
| Skill 运行时注入 | ✅ | `BuildTRPCLLMAgent` 中 `WithSkills` |
| Skill DB Repository | ✅ | `trpcskill.Repository` |
| 前端管理 | ✅ | Skill 设置页 |

---

## 3. 差距与优化

1. **P2**：Skill 无版本管理，更新后无法回滚到旧版本。
2. **P3**：Skill 无依赖声明，安装时无法检查前置依赖。
3. **P3**：Skill 无市场/分享机制，用户间无法共享技能。

---

## 4. 开发阶段

- **Phase 1**：Skill 版本管理（version 字段 + 回滚）
- **Phase 2**：Skill 依赖声明与检查
- **Phase 3**：Skill 市场/分享机制

---

## 5. 任务清单

| # | 任务 | 优先级 | EP |
|---|------|--------|-----|
| 1 | Skill 版本表 + 版本回滚 API | P2 | — |
| 2 | Skill 依赖声明 schema + 安装时检查 | P3 | — |
| 3 | Skill 市场前端页面 | P3 | — |

---

## 6. 验收标准

- [ ] Skill 可管理多个版本并回滚
- [ ] 安装 Skill 时自动检查依赖
- [ ] 用户可浏览和安装共享 Skill

---

## 7. 依赖与风险

- Skill 市场需与 Ecosystem 模块联动
- 版本管理需考虑与 Agent 绑定的兼容性
