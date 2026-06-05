---
name: sddflow/close
description: Verify implementation with Superpowers + OpenSpec, archive change, then finish development branch
---

# Close: 验证 + 归档 + 收尾

## 目标

按顺序执行五个阶段，确保实现正确性、规格一致性，完成 OpenSpec 归档，最后处理开发分支。**每个阶段必须通过才能进入下一阶段。**

## 中断续接规则

- 被打断后继续回复 → 保持 close 阶段，从中断的步骤恢复
- close 阶段不允许修改代码；发现问题记录后切到 `/sddflow amend` 或 `/sddflow build` 修复，修复完再回到 close

## 前置条件

- `docs/superpowers/plans/` 下对应 plan 文件的所有 checkbox 已勾选
- `openspec/changes/<变更名>/tasks.md` 所有条目为 `[x]`

有未完成 task 时终止：
> "还有 N 个任务未完成。请先用 `/sddflow build` 完成实现后再归档。"

---

## 阶段 1：实现验证（Superpowers verification-before-completion）

> **使用 Superpowers `verification-before-completion` skill**

**不允许跳过，不允许用"应该通过"代替实际运行。**

### 1.1 测试套件验证

运行项目完整测试命令（以实际技术栈为准）：

```bash
# 根据项目选择对应命令
npm test / pytest / cargo test / go test ./... / ...
```

**必须看到实际输出，确认：**
- 所有测试通过（0 failures）
- 无警告或错误输出

**测试失败时终止：**
> "测试未通过（N failures）。请先用 `/sddflow build` 修复失败测试，再执行 close。"

### 1.2 构建验证（如适用）

```bash
npm run build / cargo build --release / ...
```

确认 exit code 为 0。

### 1.3 需求清单验证

重新读取 `plan-ready.md` 和 `openspec/changes/<变更名>/tasks.md`，逐条对照代码确认：

| 验证项 | 要求 |
|--------|------|
| 每条 task | 对应实现文件存在 |
| 每条 task | 对应测试文件存在且通过 |
| 所有 checkbox | 均为 `[x]` |

**有任何项目无法用实际证据（文件路径、测试输出）确认时，记录到 `close-issues.md` 并终止：**
> "验证失败：以下需求无实现证据 — [列表]。请先修复再执行 close。"

---

## 阶段 2：代码审查（Superpowers requesting-code-review，可选）

> **使用 Superpowers `requesting-code-review` skill**

询问用户是否执行代码审查：

> "是否在归档前执行最终代码审查？（推荐在合并到主分支前执行）"

**用户选择执行时：**

1. 获取 git SHAs：
   ```bash
   BASE_SHA=$(git rev-parse origin/main 2>/dev/null || git merge-base HEAD main)
   HEAD_SHA=$(git rev-parse HEAD)
   ```

2. 派发 code reviewer 子代理，提供：
   - 变更描述（来自 proposal.md 摘要）
   - 需求文档（plan-ready.md 路径）
   - BASE_SHA 和 HEAD_SHA

3. 处理 reviewer 反馈：

| 等级 | 处理方式 |
|------|----------|
| Critical | 终止 close，切到 `/sddflow build` 修复后重新 close |
| Important | 终止 close，切到 `/sddflow build` 修复后重新 close |
| Minor | 记录到 `close-issues.md`，可选修复，不阻塞继续 |

**用户选择跳过时：** 记录"用户跳过代码审查"，继续阶段 3。

---

## 阶段 3：规格一致性验证

> 本项目无 `/opsx:verify` 命令；按下列步骤手工验证，并配合 OpenSpec CLI。

### 3.1 CLI 校验（如可用）

```bash
openspec validate <变更名> --strict
```

校验失败 → 修正规格或实现后重新 close。

### 3.2 三维度对照（记录到 `openspec/changes/<变更名>/close-issues.md`）

| 维度 | 检查内容 |
|------|----------|
| **Completeness** | `tasks.md` checkbox 全部 `[x]`；plan 文件 checkbox 全部 `[x]`；所有规格需求有实现证据 |
| **Correctness** | 实现与 `specs/**/spec.md` 中 Requirement、Scenario 一致 |
| **Coherence** | 若存在 `design.md`：代码遵循其决策；若无 `design.md`：代码与 `proposal.md` / `specs/**` 一致，且符合项目既有模式 |

### 3.3 处理结果

| 等级 | 处理方式 |
|------|----------|
| CRITICAL | 终止 close；写入 `close-issues.md`；提示 `/sddflow amend` + `/sddflow build` 后重新 close |
| WARNING | 展示给用户，询问是否修复；用户确认后可继续归档 |
| SUGGESTION | 记录到 `close-issues.md`，不阻塞归档 |

**存在 CRITICAL 时输出：**
> "规格验证失败，存在 N 个 CRITICAL 问题（见 close-issues.md）。请用 `/sddflow amend` 修订需求或用 `/sddflow build` 补充实现后再执行 close。"

---

## 阶段 4：归档（OpenSpec: Archive + openspec/specs 同步）

> **优先使用 Cursor/Claude 命令 `OpenSpec: Archive`，或 OpenSpec CLI `openspec archive`**

### 4.1 前置确认

阶段 3 通过（无 CRITICAL）后，向用户确认：

> "验证通过。准备归档变更 `<变更名>`。此操作将：
> 1. 将 delta specs 同步合并到 `openspec/specs/`（除非明确为纯工具变更）
> 2. 将 `openspec/changes/<变更名>/` 移动到 `openspec/changes/archive/YYYY-MM-DD-<变更名>/`
>
> 确认归档？"

### 4.2 执行归档

**方式 A — 编辑器命令（推荐）：** 调用 `OpenSpec: Archive`，按提示选择变更 ID。

**方式 B — CLI：**

```bash
openspec archive <变更名> --yes
```

归档流程应完成：

1. 检查所有 artifact 与 `tasks.md` 完成状态
2. 评估 delta specs 与 `openspec/specs/` 的差异
3. 同步 delta specs 到 `openspec/specs/<capability>/spec.md`（推荐；纯工具变更可用 `--skip-specs`）
4. 将变更目录移入 `openspec/changes/archive/`

归档后运行（如 CLI 可用）：

```bash
openspec validate --strict
```

### 4.3 确认归档结果

归档完成后确认：
- `openspec/changes/archive/YYYY-MM-DD-<变更名>/` 目录存在
- `openspec/specs/` 相关能力的规格已更新（如有 delta specs）
- `openspec/changes/<变更名>/` 已不在 active changes 列表

---

## 阶段 5：收尾开发分支（Superpowers finishing-a-development-branch）

> **如果使用了 git worktree 或 feature branch，则执行本阶段**
> **使用 Superpowers `finishing-a-development-branch` skill**

**声明：** 输出：
> "正在使用 finishing-a-development-branch skill 收尾开发分支。"

### 5.1 检测是否需要执行

检查当前工作区：

```bash
GIT_DIR=$(cd "$(git rev-parse --git-dir)" 2>/dev/null && pwd -P)
GIT_COMMON=$(cd "$(git rev-parse --git-common-dir)" 2>/dev/null && pwd -P)
```

- 普通仓库且在主分支上 → 跳过本阶段，直接到完成提示
- 在 feature branch 或 worktree 上 → 执行本阶段

### 5.2 再次验证测试（finishing-a-development-branch 要求）

```bash
# 运行完整测试套件
<项目测试命令>
```

测试失败时 finishing-a-development-branch 会拒绝继续，需先修复。

### 5.3 呈现选项

按 `finishing-a-development-branch` skill 流程，呈现：

```
1. 合并到 <base-branch>（本地 merge）
2. 推送并创建 Pull Request
3. 保留分支现状（稍后处理）
4. 丢弃此分支
```

### 5.4 执行选择并清理 worktree

按用户选择执行，worktree 清理规则：
- 选项 1（merge）或 选项 4（discard）→ 清理 worktree
- 选项 2（PR）或 选项 3（keep）→ 保留 worktree

---

## 完成提示

> "变更 `<变更名>` 已完成全部关闭流程：
>
> ✅ 阶段 1：实现验证通过（测试全绿）
> ✅ 阶段 2：代码审查完成（或已跳过）
> ✅ 阶段 3：OpenSpec 验证通过
> ✅ 阶段 4：已归档至 openspec/changes/archive/YYYY-MM-DD-<变更名>/
> ✅ 阶段 5：开发分支已处理（或不适用）
>
> 可以开始下一个变更了。"

---

## 关键原则

- close 阶段**不修改任何代码或实现文件**，只做验证、记录、归档
- 验证必须有实际命令输出作为证据，不允许用"应该通过"代替运行
- CRITICAL 问题必须修复后才能归档，WARNING 可询问用户决定
- finishing-a-development-branch 放在最后，确保归档完成后再处理分支
- 不一致项优先记录到 `close-issues.md`，不在 close 阶段现场修代码
