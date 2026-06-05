---
name: sddflow/build
description: Strict pre-flight file check, then execute with subagent-driven-development + TDD
---

# Build: 执行实现

## 目标

### 0. 依赖检测

执行前检查以下依赖是否可用（**不在 build 阶段生成或重写计划文件**）：

| 依赖 | 检测方式 | 不可用时 |
|------|----------|----------|
| 详细实现计划 | `docs/superpowers/plans/` 下存在含变更名的 `.md` 文件 | **终止 build**，提示先完成 `/sddflow spec` |
| Superpowers subagent-driven-development | skills 目录下是否存在 `subagent-driven-development/SKILL.md` | 降级为按 plan 文件逐步手动执行 |
| Superpowers test-driven-development | skills 目录下是否存在 `test-driven-development/SKILL.md` | 提示安装；仍按 plan 执行，须自述遵守 TDD |
| OpenSpec CLI | `openspec` 命令是否可执行 | 不影响 build；close 归档可改用 `OpenSpec: Archive` 或 `openspec archive` |

如果 Superpowers 子技能缺失，提示用户：
> "Superpowers 未完整安装，build 将使用手动执行模式。安装后体验更佳：请在当前工具中安装 Superpowers 插件"

**禁止**在 build 阶段调用 `writing-plans`；计划必须在 spec 阶段已生成。


严格校验 spec 阶段产出的所有文件完整性，通过后用 `subagent-driven-development` 逐 Task 派发子代理执行，每个子代理强制遵循 `test-driven-development` 铁律。

**build 阶段不生成任何计划文件。** 计划在 `/sddflow spec` 阶段已由 `writing-plans` 完成。

## 中断续接规则

- 被打断后继续回复、说"继续"、补充实现细节 → 保持 build 阶段，从 plan 文件的 checkbox 状态恢复
- 用户明确要求修改需求/规格/验收条件/功能边界 → **立即切到 `/sddflow amend`**，amend 完成后再回到 build

---

## 阶段 1：前置文件完整性校验

<HARD-GATE>
以下必填文件必须存在且通过校验，任何一项不通过都不允许进入执行阶段。
`design.md` 按 OpenSpec 可选规则处理，缺失不阻断 build。
</HARD-GATE>

### 1.1 确定活跃变更

检查 `openspec/changes/` 下非 archive 子目录。多个时列出并让用户选择。

### 1.2 文件完整性检查

对找到的变更目录，逐项检查：

| 检查项 | 路径 | 不通过时 |
|--------|------|----------|
| 提案文件 | `openspec/changes/<变更名>/proposal.md` | 提示先运行 `/sddflow brainstorming` |
| 规格目录 | `openspec/changes/<变更名>/specs/`（非空） | 提示先运行 `/sddflow spec` |
| 任务清单 | `openspec/changes/<变更名>/tasks.md` | 提示先运行 `/sddflow spec` |
| 翻译计划 | `openspec/changes/<变更名>/plan-ready.md` | 提示先运行 `/sddflow spec` |
| 详细实现计划 | `docs/superpowers/plans/` 下有 `<变更名>` 对应文件 | 提示先运行 `/sddflow spec` |
| Checkbox 扩展 | 三份任务文档均含 `- [ ]` / `- [x]`（见 spec「三文档 Checkbox 对齐扩展」） | 提示先运行 `/sddflow spec` 补齐 checkbox |

**可选参考（不阻断）：**

| 检查项 | 路径 | 说明 |
|--------|------|------|
| 技术方案 | `openspec/changes/<变更名>/design.md` | 若存在则作为实现参考输入；不存在且符合 OpenSpec 可选规则则跳过，不得要求 spec 补空文件 |

任一必填项不通过，输出完整缺失列表后终止：

> "build 前置校验未通过，缺少以下文件：
> - [缺失文件列表]
>
> 请先完成 `/sddflow spec` 生成全部必填文件后再执行 build。"

### 1.3 Plan 文件内容校验

找到 plan 文件后，验证其内容质量：

| 校验项 | 规则 |
|--------|------|
| 包含 Task | 至少 1 个 `### Task N:` |
| Trace / Sync | 每个 Task 都有 `> **trace:**` 与 `> **sync:**`（含 tasks.md 与 plan-ready.md 原文行） |
| 无占位符 | 不含 "TODO"、"TBD"、"实现待定" |
| Checkbox 语法 | plan 中每个 Step 与每个 Task 末尾 **Task complete** 均有 `- [ ]` 或 `- [x]` |
| plan-ready / tasks | 对应变更的 plan-ready **任务完成** 行、tasks.md 任务行均为 checkbox 语法 |

任一不通过，提示：

> "plan 文件内容校验失败：[具体原因]。请先运行 `/sddflow spec` 重新生成计划，或用 `/sddflow amend` 修订后重新生成。"

### 1.4 断点恢复判断

plan 文件通过校验后，读取其 checkbox 状态：

| 状态 | 处理 |
|------|------|
| 全部 `[x]` | 实现已完成，提示 `/sddflow close`，不再执行 |
| 部分 `[x]` | 断点恢复 — 从第一个未勾选 Task 继续（进入阶段 2） |
| 全部 `[ ]` | 全新执行（进入阶段 2） |

---

## 阶段 2：执行实现

> **使用 Superpowers `subagent-driven-development` skill**
> **每个子代理强制遵循 `test-driven-development` skill**

### 2.1 启动前准备

1. **完整读取** plan 文件一次，提取所有 Task（含完整文本）
2. 用 TodoWrite 创建任务列表，每个 Task 一条，初始状态 `[ ]`（已完成的标 `[x]`）
3. 记录变更名和相关文件路径，供子代理使用

### 2.2 逐 Task 执行

**连续执行，不在 Task 间停下来询问进度。** 唯一停止原因：BLOCKED 无法解决、真正的歧义阻塞执行、或全部完成。

```
对每个未完成 Task 按以下顺序执行：

┌─ 派发 Implementer 子代理 ──────────────────────────────────────┐
│  • 提供 Task 完整文本（不让子代理自己读 plan 文件）             │
│  • 提供项目上下文（相关文件路径、架构说明、技术栈）             │
│  • 明确要求：                                                   │
│    - 必须遵循 test-driven-development (Red → Verify RED         │
│      → Green → Verify GREEN → Refactor)                        │
│    - 每完成一个 Step 立即勾选 plan 文件中对应 checkbox          │
│    - 不允许先做完所有 Step 再批量勾选                           │
│  • 子代理状态：DONE / DONE_WITH_CONCERNS / NEEDS_CONTEXT        │
│    / BLOCKED                                                   │
└────────────────────────────────────────────────────────────────┘
         ↓ DONE 或 DONE_WITH_CONCERNS
┌─ 派发 Spec Reviewer 子代理 ────────────────────────────────────┐
│  • 确认代码满足 plan-ready.md 中对应需求                        │
│  • ✅ → 进入代码质量审查                                         │
│  • ❌ → Implementer 修复 → 重新 Spec 审查                        │
└────────────────────────────────────────────────────────────────┘
         ↓ ✅
┌─ 派发 Code Quality Reviewer 子代理 ────────────────────────────┐
│  • 审查代码质量（命名、结构、耦合、测试覆盖率）                 │
│  • ✅ → 执行 2.5 三文档 checkbox 同步（plan Step/Task、tasks、plan-ready） │
│  • ❌ → Implementer 修复 → 重新 Quality 审查                    │
└────────────────────────────────────────────────────────────────┘
```

**处理子代理状态：**

| 状态 | 处理方式 |
|------|----------|
| `DONE` | 进入 Spec 审查 |
| `DONE_WITH_CONCERNS` | 读取 concerns；正确性/范围问题先解决再审查；观察性问题记录后继续 |
| `NEEDS_CONTEXT` | 提供缺失上下文，重新派发 |
| `BLOCKED` | 提供更多上下文重试；仍阻塞则升级给用户 |

**绝不：**
- 跳过 Spec Compliance 审查
- 在 Spec 审查通过前进行 Code Quality 审查
- 让两个 Implementer 子代理同时执行（防止冲突）
- 接受"差不多符合"（reviewer 有问题 = 未完成）

### 2.3 TDD 铁律（子代理必须遵守）

每个 Step 内的代码实现按 Red-Green-Refactor 循环执行：

1. **RED** — 写失败测试，运行并确认以预期原因 FAIL
2. **GREEN** — 写最小实现代码，运行确认 PASS
3. **REFACTOR** — 清理代码，保持测试绿色

**铁律：没有先看到测试 FAIL，就没有实现代码。有代码未先写测试？删除，重来。**

### 2.4 实时 checkbox 勾选

<HARD-GATE>
每完成一个 Step，立即更新 plan 文件中对应 `- [ ]` 为 `- [x]`。
禁止等所有 Step 或所有 Task 完成后批量勾选。
</HARD-GATE>

| 时机 | 操作 |
|------|------|
| 每个 Step 完成后 | 立即修改 plan 文件，将该 Step 的 `- [ ]` 改为 `- [x]` |
| 整个 Task 所有 Step `[x]` 后 | 执行三文档同步（见 2.5） |

### 2.5 Task 完成后：同步 plan-ready.md、tasks.md

每个 Task 全部 Step 勾选完毕后，**立即**按 plan 文件中该 Task 的 `> **sync:**` 与 `> **trace:**` 同步以下三处（顺序不限，须在同一轮完成）：

**A. superpowers plan（当前文件）**

1. 该 Task 下所有 Step 已为 `[x]`（应在 2.4 中逐步完成）
2. 将该 Task 末尾 `- [ ] **Task complete**` 改为 `- [x] **Task complete**`

**B. tasks.md**

1. 打开 `openspec/changes/<变更名>/tasks.md`
2. 用 `sync` 中 `tasks.md →` 后的**原文整行**定位对应 `- [ ]` 条目
3. 改为 `- [x]`，末尾追加：`<!-- 已实现: [简短描述] -->`

**C. plan-ready.md**

1. 打开 `openspec/changes/<变更名>/plan-ready.md`
2. 用 `sync` 中 `plan-ready.md →` 后的 `### Task N: ...` 标题定位对应 Task 块
3. 将该 Task 下 `- [ ] **任务完成**` 改为 `- [x] **任务完成**`

**规则：**

- 整个 superpowers plan Task 完成才同步 B、C，不允许部分同步
- `trace` 用于校验：`sync` 行与源文件不一致 → 记录警告并尝试用 `trace` 回退匹配
- 任一文档匹配失败 → 记录警告，不阻塞执行，但阶段 3 一致性检查会暴露遗漏

---

## 阶段 3：完成验证

所有 Task 执行完毕后，运行最终一致性检查：

- [ ] `openspec/changes/<变更名>/tasks.md` 所有条目为 `[x]`
- [ ] `openspec/changes/<变更名>/plan-ready.md` 所有 **任务完成** checkbox 为 `[x]`
- [ ] plan 文件所有 checkbox 为 `[x]`
- [ ] 三文档 Task 数量与 plan 中 `### Task N` 数量一致
- [ ] superpowers plan 每个 Task 的 **Task complete** 均为 `[x]`

**不一致时：**

| 情况 | 处理 |
|------|------|
| tasks.md 有未勾选 | 回到阶段 2 执行遗漏 Task |
| plan-ready.md 有未勾选 | 回到阶段 2 执行 2.5 同步 plan-ready |
| plan 文件有未勾选 | 回到阶段 2 执行遗漏 Step |
| 数量不一致 | 重新执行 2.5 后再比对 |

全部通过后提示：

> "所有实现任务已完成，plan 文件、plan-ready.md 与 tasks.md 已同步。
>
> 接下来可以用 `/sddflow close` 验证规格一致性并归档。"

---

## 关键原则

- **build 阶段不生成任何计划文件** — 计划在 spec 阶段已完成
- **build 阶段不修改规格文档** — 发现需求遗漏或规格错误 → `/sddflow amend`
- **plan-ready.md 是锁定的输入** — 子代理按计划执行，不重新解读需求
- **断点恢复依赖文件系统** — 不依赖 AI 会话记忆，任何时候重启都从 checkbox 状态恢复
- **plan 与 plan-ready.md、tasks.md 三向同步** — 每个 Task 完成后按 `sync` 勾选三处 checkbox
- **`design.md` 遵循 OpenSpec optional 语义** — 缺失不得阻断 build，不得要求 spec 补空 `design.md`；若存在则作为实现参考
