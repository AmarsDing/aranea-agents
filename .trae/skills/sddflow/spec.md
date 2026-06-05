---
name: sddflow/spec
description: Complete OpenSpec change artifacts per AGENTS.md and OpenSpec Proposal, then translate to plan-ready.md and writing-plans
---

# Spec: 生成规格 + 实现计划

## 目标

1. **按 OpenSpec 官方流程**补齐 `specs/`、`tasks.md`；`design.md` **按需**创建（`proposal.md` 通常已由前一阶段提供）
2. 翻译为工程视角的 `plan-ready.md`（sddflow 翻译层，不改变 OpenSpec 制品格式）
3. 调用 Superpowers `writing-plans` skill 生成可执行的详细实现计划（`docs/superpowers/plans/YYYY-MM-DD-<变更名>.md`）

**spec 阶段产出全部文档后，build 阶段直接进入执行，不再生成计划。**

## 中断续接规则

如果用户在本阶段被打断后继续回复、补充范围、要求调整规格、或确认规格摘要，仍然停留在 spec 阶段。只更新 `openspec/changes/**`、`plan-ready.md` 和 plan 文件，不要修改任何代码或实现文件。

## 前置条件

- `openspec/changes/` 下存在活跃变更目录（由 brainstorming 阶段创建）
- 变更目录下至少有 `proposal.md`

---

## 步骤 1：确认活跃变更

检查 `openspec/changes/` 下是否有活跃变更（非 archive 子目录）。

没有时提示：
> "还没有活跃变更。请先用 /sddflow brainstorming 创建需求。"

多个时列出并让用户选择：
> "检测到多个活跃变更：[列表]。要对哪个生成规格？"

---

## 步骤 2：生成 OpenSpec 规格制品（specs/ + tasks.md；design.md 按需）

<HARD-GATE>
本步骤**必须**完全遵循 OpenSpec 官方规范，**禁止**使用 sddflow 自订格式、自订章节结构或替代生成流程。
</HARD-GATE>

### 2.1 必读与唯一规范来源

执行前**完整阅读**并严格遵守（冲突时以 OpenSpec 为准）：

1. `openspec/AGENTS.md` — Stage 1、Creating Change Proposals、Spec File Format、Delta Operations、Troubleshooting
2. 项目内 **`OpenSpec: Proposal`** 命令（如 `.claude/commands/openspec/proposal.md`）中 `<!-- OPENSPEC:START -->` … `<!-- OPENSPEC:END -->` 的 **Guardrails** 与 **Steps**

**声明（必须输出）：**
> "正在按 OpenSpec 官方流程补齐 specs/ 与 tasks.md（design.md 按需；遵循 openspec/AGENTS.md 与 OpenSpec: Proposal）。"

### 2.2 执行方式

在已确认 `<变更名>` 且 `openspec/changes/<变更名>/proposal.md` 已存在的前提下，**逐条执行 OpenSpec: Proposal 的步骤 1–7**（与 AGENTS.md Stage 1 一致）：

1. 调研上下文：`openspec/project.md`、`openspec list`、`openspec list --specs`，必要时 `rg` / `openspec show` / 阅读相关代码
2. 确认 `change-id` 为 `<变更名>`；**不要**用 sddflow 模板替换已有 `proposal.md`，仅在 OpenSpec 规范要求或与用户确认后，将内容对齐为 AGENTS.md 的 `## Why` / `## What Changes` / `## Impact` 结构
3. 将变更映射到 capability，按 OpenSpec: Proposal 步骤 3 拆分多能力 delta
4. 按 AGENTS.md **Creating Change Proposals** 第 5 节判定是否创建 `design.md`；不需要则**不得**添加空 `design.md`
5. 在 `openspec/changes/<变更名>/specs/<capability>/spec.md` 撰写 delta（`## ADDED|MODIFIED|REMOVED|RENAMED Requirements`；每条 requirement 至少一个 `#### Scenario:`；`MODIFIED` 须粘贴 `openspec/specs/` 中完整 requirement 后再改）
6. 按 OpenSpec: Proposal 步骤 6 撰写 `tasks.md`（有序、可勾选、含验证项）；生成后须符合「三文档 Checkbox 对齐扩展」中 **tasks.md** 条款
7. 运行严格校验（见 2.3）

**禁止：**

- 自订 spec 模板、自订 requirement/scenario 写法（一律以 AGENTS.md 为准）
- 凭经验生成 delta 而未对照 `openspec/specs/` 与 AGENTS.md
- 跳过 `openspec validate` 或忽略校验错误

### 2.3 校验

```bash
openspec validate <变更名> --strict
```

校验失败时：使用 `openspec show <变更名> --json --deltas-only` 排查，按 AGENTS.md **Troubleshooting** 修正后重新校验，直至通过。

---

## 步骤 3：与用户确认规格

展示规格摘要，逐项确认：

> "以下是规格摘要：
> - **提案**：[proposal.md 核心内容]
> - **设计**：[若存在 design.md → 核心决策；否则 → 无 design.md（OpenSpec 判定无需技术设计文档）]
> - **规格**：[specs/ 变更列表]
> - **任务**：[tasks.md 任务列表]
>
> 有需要调整的地方吗？"

用户确认后才进入步骤 4。

---

## 三文档 Checkbox 对齐扩展（通用）

以下扩展**不改变**各文档的原有生成逻辑，仅在生成结果上**追加/保留** Markdown checkbox（`- [ ]` / `- [x]`），用于判断「是否已实现」并在 `/sddflow build` 时三向同步。`[ ]` = 未完成，`[x]` = 已完成。

| 文档 | 原有生成逻辑（不得替换） | sddflow 追加：必须有 checkbox |
|------|------------------------|------------------------------|
| **tasks.md** | OpenSpec: Proposal 步骤 6 + AGENTS.md 任务清单结构 | 每条**可交付/可验收**的实现项必须以 `- [ ]` 或 `- [x]` 开头；保留 OpenSpec 章节与编号（如 `## 1. Implementation`、`- [ ] 1.1 ...`），**禁止**改成无 checkbox 的纯列表或段落 |
| **plan-ready.md** | 步骤 4 翻译层（目标 / 改动文件 / 验证方式） | 每个 `### Task N:` 块内**首行**必须为 `- [ ] **任务完成**`（build 完成该 Task 后改为 `[x]`）；不得删除翻译正文，仅追加该行 |
| **superpowers plan** | Superpowers `writing-plans`（Header、Task、Step、TDD 步骤） | 保留 writing-plans 规定的**每个 Step** `- [ ]`；在每个 `### Task N` 块**末尾**（最后一个 Step 之后）追加 `- [ ] **Task complete**`（该 Task 全部 Step 已为 `[x]` 后方可勾选，并与 plan-ready、tasks 同步） |

**对齐规则：**

- `Task N` 序号在三份文档间一一对应（从 1 递增）
- 判断「整个 Task 是否完成」：三份文档中该 Task 的**任务级** checkbox 均为 `[x]`，且 superpowers plan 该 Task 下**所有 Step** checkbox 均为 `[x]`
- 判断「变更是否全部完成」：三份文档中**全部**任务级 checkbox 与 superpowers plan **全部** Step checkbox 均为 `[x]`（供 `/sddflow close` 前置检查）

步骤 2、4、5 完成后须自检上表；缺 checkbox 或 Task 序号不一致 → 修正后再进入下一步。

---

## 步骤 4：生成 plan-ready.md（翻译层）

将 OpenSpec 制品翻译为工程视角的执行格式（**并满足上文「三文档 Checkbox 对齐扩展」中 plan-ready.md 条款**）。

**翻译规则：**
1. 每个 OpenSpec Task 拆成 2-5 个细粒度步骤（对应 2-5 分钟工作量）
2. 每个步骤必须指明改哪个文件
3. 每个步骤必须有验证方式
4. **按执行依赖排序，不是按功能模块排序**
5. 记录来源路径，方便回溯

**输出路径：** `openspec/changes/<变更名>/plan-ready.md`

```markdown
# 实现计划：<变更名>

## 来源
- 提案：openspec/changes/<变更名>/proposal.md
- 设计：<若存在 design.md 则写路径；否则写「无（OpenSpec 判定无需）」>
- 规格：openspec/changes/<变更名>/specs/
- 任务：openspec/changes/<变更名>/tasks.md

## 实现步骤

### Task 1: <任务名>
- [ ] **任务完成**（与 superpowers plan `Task 1`、`tasks.md` 对应条目同步勾选）
- 目标：<做什么>
- 改动文件：<哪些文件>
- 验证方式：<怎么验证>

### Task 2: ...
```

步骤 4 中每个 `### Task N` 的标题与序号须与 `tasks.md`、`writing-plans` 产出中的 Task N **一一对应**。

---

## 步骤 5：生成详细实现计划（writing-plans）

<HARD-GATE>
本步骤**必须**完整遵循 Superpowers **`writing-plans`** skill（读取并执行其全文：Scope Check、File Structure、Bite-Sized Task Granularity、Plan Document Header、Task Structure、No Placeholders、Self-Review）。**禁止**用本节自订格式替代 writing-plans 的 Task/Step 结构或占位符规则。
</HARD-GATE>

### 5.1 调用 writing-plans

1. **先读取** Superpowers `writing-plans` skill（`writing-plans/SKILL.md`）
2. **必须声明**（与 skill 一致，可中英任选其一）：
   > "I'm using the writing-plans skill to create the implementation plan."
   > 或：「正在使用 writing-plans skill 生成详细实现计划。」
3. **输入上下文**（除 writing-plans 要求的 spec 外，sddflow 强制一并阅读）：
   - `openspec/changes/<变更名>/plan-ready.md`
   - `openspec/changes/<变更名>/tasks.md`
   - `openspec/changes/<变更名>/proposal.md`、`design.md`（若存在）、`specs/`
4. **保存路径**（与 writing-plans 默认一致，变更名替换 feature-name）：
   `docs/superpowers/plans/YYYY-MM-DD-<变更名>.md`

按 writing-plans 顺序执行：**Scope Check → File Structure → 分解 Task/Step → 写入计划 → Self-Review**。

### 5.2 sddflow 追溯与 Checkbox 扩展（在 writing-plans 规则之上追加）

在**不修改** writing-plans 规定的 Plan Header 与 Task/Step 主体结构的前提下，追加：

1. **Checkbox** — 须满足「三文档 Checkbox 对齐扩展」中 **superpowers plan** 条款（含每个 Task 末尾的 `- [ ] **Task complete**`）
2. **追溯字段** — 供 `/sddflow build` 定位并同步三份文档：

**Plan Header 在 writing-plans 必填项之后追加：**

```markdown
**Traceability (sddflow):**
- plan-ready: `openspec/changes/<变更名>/plan-ready.md`
- tasks: `openspec/changes/<变更名>/tasks.md`
- plan: `docs/superpowers/plans/YYYY-MM-DD-<变更名>.md`
```

**每个 `### Task N` 正文开头（位于 `**Files:**` 之前）必须包含：**

```markdown
> **trace:** plan-ready.md → `### Task N: <与 plan-ready 完全一致的标题>` | tasks.md → `<tasks.md 中对应条目的原文整行，含 checkbox>`
> **sync:** tasks.md → `<与 trace 中 tasks.md 行相同的原文整行>` | plan-ready.md → `### Task N: <标题>`
```

规则：
- `Task N` 序号在 plan 文件、`plan-ready.md`、`tasks.md` 三者间**一一对应**（N 从 1 递增，不跳号）
- `trace` / `sync` 中的 `tasks.md`、`plan-ready.md` 引用必须是**可精确匹配的原文**（build 靠此行定位勾选位置）
- 每个 OpenSpec `tasks.md` 顶层待实现 checkbox 条目至少对应一个 superpowers plan `Task`；每个 `plan-ready.md` 的 `### Task N` 至少对应一个 superpowers plan `Task`
- writing-plans 的每个 Step 仍使用 `- [ ]` checkbox（build 每完成 Step 立即勾选）

**每个 Task 末尾追加（最后一个 Step 之后）：**

```markdown
- [ ] **Task complete**（本 Task 全部 Step 为 `[x]` 后勾选；与 plan-ready **任务完成**、tasks.md 对应行同步）
```

**build 阶段同步契约（写入计划时须自检）：**

| 完成粒度 | 须勾选位置 |
|----------|------------|
| 每个 Step 完成 | superpowers plan 对应该 Step 的 `- [ ]` → `[x]` |
| 整个 Task 完成 | 该 Task 全部 Step 为 `[x]`；superpowers plan **Task complete** → `[x]`；`tasks.md` 中 `sync` 指向行 → `[x]`；`plan-ready.md` 中 **任务完成** → `[x]` |

### 5.3 禁止占位符与自检

**占位符：** 以 writing-plans skill **No Placeholders** 为准（禁止 TBD/TODO/无代码的「写测试」等）；不得因 sddflow 追溯字段而省略完整代码与命令。

**写完计划后依次执行：**

1. writing-plans **Self-Review**（Spec coverage、Placeholder scan、Type consistency）
2. **三文档 Checkbox 自检**（见「三文档 Checkbox 对齐扩展」）：tasks.md、plan-ready.md、superpowers plan 任务级与 Step 级 checkbox 齐全
3. sddflow 追溯自检（全部为真才进入步骤 6）：
   - [ ] `docs/superpowers/plans/YYYY-MM-DD-<变更名>.md` 已存在
   - [ ] Plan Header 含 writing-plans 必填项 + `**Traceability (sddflow):**`
   - [ ] 至少 1 个 Task，且 Task 数与 `tasks.md`、`plan-ready.md` 的 `### Task` 数量一致
   - [ ] 每个 Task 含 `> **trace:**`、`> **sync:**`，且末尾有 `- [ ] **Task complete**`
   - [ ] 每个 Step 为 writing-plans 粒度，含完整代码块与 Run/Expected（无占位符）

有任一不通过 → 按 writing-plans Self-Review 修正后重做追溯自检，禁止进入步骤 6。

---

## 步骤 6：汇总确认并提示

向用户展示所有产出：

> "spec 阶段完成，以下文件已就绪：
>
> **OpenSpec 规格：**
> - `openspec/changes/<变更名>/proposal.md` ✓
> - `openspec/changes/<变更名>/design.md` ✓（若存在；否则注明「无 design.md，符合 OpenSpec 可选规则」）
> - `openspec/changes/<变更名>/specs/` ✓
> - `openspec/changes/<变更名>/tasks.md` ✓
>
> **执行计划：**
> - `openspec/changes/<变更名>/plan-ready.md` ✓
> - `docs/superpowers/plans/YYYY-MM-DD-<变更名>.md` ✓
>
> 接下来可以用 `/sddflow build` 开始实现。build 会严格校验以上文件完整性后直接执行，不再重新生成计划。"

---

## 关键原则

- **OpenSpec 制品（design.md / specs/ / tasks.md）只按 AGENTS.md + OpenSpec: Proposal 生成**，sddflow 不在此步骤引入自订规则
- **一条代码都不许写** — spec 阶段只产出文档
- 只允许写 `openspec/changes/**`、`plan-ready.md`、`docs/superpowers/plans/*.md`，禁止修改任何代码
- 翻译（plan-ready.md、writing-plans）在用户确认 OpenSpec 规格后进行，不改变 delta spec 格式
- 三份任务文档均须满足「三文档 Checkbox 对齐扩展」，便于对齐是否已实现
- 步骤 5 须通过 writing-plans Self-Review、Checkbox 自检与追溯自检，否则不允许结束 spec 阶段
- plan-ready.md 的 `## 来源` 部分必须写明路径
- 按执行依赖排序是翻译的关键步骤：先依赖后依赖方
