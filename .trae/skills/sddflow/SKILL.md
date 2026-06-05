---
name: sddflow
description: "OpenSpec + Superpowers workflow orchestrator. Use /sddflow brainstorming for design exploration, /sddflow spec to generate specs + translate, /sddflow amend to revise requirements before close, /sddflow build to execute, /sddflow close to verify and archive. Bridges requirements specs and engineering execution."
argument-hint: "brainstorming | spec | amend | build | close"
---

# sddflow - 工作流协调器

根据用户调用的子命令和项目当前状态，路由到对应阶段。

## 续接与中断恢复

如果本轮没有显式 `/sddflow ...` 子命令，但上一轮已经进入 sddflow 任一阶段，并且用户是在补充范围、回答确认问题、说"继续"、修正需求、或说明新增/移除边界：

1. 默认继续上一 sddflow 阶段，不把该回复当作普通编码请求
2. 如果上一阶段是 brainstorming、spec 或 amend，只能继续产出/更新 OpenSpec 文档和计划文档，不得修改任何代码或实现文件
3. 如果上一阶段是 build，但用户补充的是需求、验收条件或规格边界变更，切到 `/sddflow amend`，不要直接改代码
4. 只有用户显式调用 `/sddflow build`，或状态检测明确进入 build 阶段后，才允许修改代码或实现文件
5. 中断后恢复时，先重新读取当前阶段文件和 `openspec/changes/` 状态，再继续执行

**路由优先级（高于文件状态推断）：**

1. **续接回复** — 无显式子命令时，保持上一 sddflow 阶段；不因目录中已有 `plan-ready.md` 而自动进入 build
2. **显式子命令** — 用户指定 `/sddflow <phase>` 时，按该阶段执行并检查前置条件
3. **裸 `/sddflow`** — 仅此时执行下方「状态检测」自动路由
4. **build 中需求变更** — 路由到 amend

典型场景：
- brainstorming 阶段询问"是否只覆盖企业端？"后，用户回复"运营端也要做回显"。这仍是设计范围修正，必须继续 brainstorming 文档收敛，不能直接进入代码实现。

## 阶段写入边界

| 阶段 | 允许写入 | 禁止写入 |
|------|----------|----------|
| brainstorming | `openspec/changes/**/proposal.md` | 任何代码或实现文件 |
| spec | `openspec/changes/**`、`plan-ready.md`、`docs/superpowers/plans/*.md` | 任何代码或实现文件 |
| amend | `openspec/changes/**`、`plan-ready.md`、`docs/superpowers/plans/*.md` | 代码、测试、其他实现文件 |
| build | 代码、测试、实现计划 checkbox 状态 | 规格文档（除非另开变更）；勿运行 OpenSpec Apply（与 sddflow build 冲突） |
| close | 归档、验证记录、`close-issues.md` | 代码、测试、其他实现文件 |

如果用户在 brainstorming/spec/amend 阶段提出"就按这个做""范围改成 X""继续"等话术，不代表进入 build；必须先完成该阶段文档产物并提示下一步。

## 子命令

| 命令 | 阶段 | 说明 |
|------|------|------|
| `/sddflow brainstorming` | brainstorming | 深度设计，多轮探索，产出 `proposal.md` |
| `/sddflow spec` | spec | 按 OpenSpec: Proposal + AGENTS.md 补齐规格，再翻译与 writing-plans |
| `/sddflow amend` | amend | build/close 前受控修改需求、规格和计划 |
| `/sddflow build` | build | 调用 Superpowers 执行实现 |
| `/sddflow close` | close | 验证一致性 + 归档 |

## 状态检测

当用户调用 `/sddflow` 不带子命令，或调用某个子命令需要确认前置条件时，执行以下状态检测：

| 检查项 | 怎么查 | 结果 |
|--------|--------|------|
| 有活跃变更？ | `openspec/changes/` 下是否有非 archive 子目录 | 有→继续 |
| 有 plan-ready.md？ | 变更目录下是否有 `plan-ready.md` | 有→看实现状态 |
| 实现已开始？ | `docs/superpowers/plans/` 下是否有计划文件 | 有→看是否完成 |
| 实现已完成？ | 计划文件全部 checkbox 已勾选 | 是→close 阶段 |

判定结果：
- 无活跃变更 → brainstorming 阶段
- 有变更但无 `proposal.md` → brainstorming 阶段
- 有 `proposal.md` 但无 plan-ready.md → spec 阶段（补生成翻译）
- 有 plan-ready.md 但实现未开始 → build 阶段
- 实现进行中 → 继续 build 阶段（断点恢复）
- 实现已完成 → close 阶段

## 路由

根据子命令或状态检测结果，读取对应阶段文件并执行：

1. 如果这是上一 sddflow 阶段的续接回复，先按"续接与中断恢复"保持阶段（**不得**因已有 `plan-ready.md` 覆盖为 build）
2. 如果用户在 build 中明确提出需求变更、补充 spec、修改验收条件或重新生成规格，路由到 amend
3. 如果用户指定了子命令（如 `/sddflow build`），按指定阶段执行，但检查前置条件
4. 如果用户只输入 `/sddflow`（无子命令、非续接），执行状态检测，自动路由到对应阶段
5. 读取当前 sddflow skill 目录下的阶段文件：`<阶段>.md`（与本 `SKILL.md` 同目录；不要依赖 Claude 专属环境变量）
6. 按阶段文件中的流程执行，并遵守阶段写入边界

### 前置条件检查

| 阶段 | 前置条件 | 不满足时提示 |
|------|----------|-------------|
| brainstorming | 无 | — |
| spec | 需要有活跃变更目录或有用户需求 | "请先用 /sddflow brainstorming 描述需求" |
| amend | 需要有活跃变更目录，通常需要 plan-ready.md | "还没有可修订的活跃变更，请先完成 /sddflow spec" |
| build | 需要存在 plan-ready.md | "请先完成 /sddflow spec 生成规格和翻译" |
| close | 需要实现已完成 | "实现尚未完成，请先用 /sddflow build 执行" |
