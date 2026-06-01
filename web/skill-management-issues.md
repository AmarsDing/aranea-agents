# Skill 管理页面 UI 审查问题清单

> 审查日期：2026-05-29
> 审查范围：SkillsPage、SkillRunsPage、AgentSettingsSkillsTab 及其所有子组件、composable、store、API 层

---

## 第一轮问题（已修复 ✅）

| # | 级别 | 问题 | 状态 |
|---|------|------|------|
| 1 | P0 | SkillRunsTable 死导入且路径错误 | ✅ 已修复 |
| 2 | P1 | useSkillsPage 双重 API 请求 | ✅ 已修复 |
| 3 | P1 | useSkillRunsPage 双重 API 请求 | ✅ 已修复 |
| 4 | P1 | usage_count_7d 值为 0 时被错误映射 | ✅ 已修复 |
| 5 | P2 | SkillUploadPlaceholder 关闭后状态未重置 | ✅ 已修复 |
| 6 | P2 | SkillRunsPage 缺少 `to` 结束时间筛选 | ✅ 已修复 |
| 7 | P2 | SkillRunsPage 时间筛选使用纯文本 ISO 输入 | ✅ 已修复 |
| 8 | P2 | SkillRunsPage 筛选栏缺少刷新按钮 | ✅ 已修复 |

---

## 第二轮深入下沉问题

---

## 问题 9：[BUG-数据] SkillEditorDialog 切换文件时未检查未保存更改

- **文件**：`web/src/components/skills/SkillEditorDialog.vue` 第 122-137 行
- **现象**：`selectFile()` 函数直接切换到新文件并加载内容，没有检查当前文件是否有未保存的更改。如果用户编辑了文件 A 但未保存，直接点击文件 B，文件 A 的修改会丢失
- **影响**：用户意外丢失未保存的编辑内容
- **修复**：在 `selectFile()` 中添加未保存更改检查，若有未保存修改则弹出确认对话框

---

## 问题 10：[BUG-数据] useSkillsPage 中 getSkillFilesystemHealth 绕过 Store 直接调 API

- **文件**：`web/src/features/skills/useSkillsPage.ts` 第 4 行、第 38-44 行
- **现象**：`getSkillFilesystemHealth` 直接从 `features/skills/api` 导入并调用，绕过了 Store 层。其他所有 API 调用都经过 `useSkillsStore`
- **影响**：违反项目数据流规范（API → Store → Composable），filesystemHealth 状态不在 Store 中管理，无法被其他组件共享
- **修复**：将 `getSkillFilesystemHealth` 调用迁入 `useSkillsStore`，composable 通过 Store 访问

---

## 问题 11：[BUG-UX] SkillTable 发布按钮仅对 draft 状态显示，但已发布 Skill 无法重新发布

- **文件**：`web/src/components/skills/SkillTable.vue` 第 87 行
- **现象**：`v-if="props.row.status === 'draft'"` 条件使得只有草稿状态的 Skill 显示发布按钮。已归档（archived）的 Skill 无法通过 UI 重新发布，但后端 `PublishSkill` 接口可能支持此操作
- **影响**：归档状态的 Skill 无法通过 UI 重新发布，用户需要先手动改状态
- **修复**：将条件改为 `v-if="props.row.status !== 'published'"`，让草稿和归档状态都显示发布按钮

---

## 问题 12：[BUG-UX] SkillRunsPage 状态筛选缺少 pending 选项

- **文件**：`web/src/features/skills/useSkillRunsPage.ts` 第 19-22 行
- **现象**：`statusOptions` 只包含 `success` 和 `failure`，但后端 `SkillInvocation` 的 `status` 字段类型定义为 `"success" | "failure" | "pending" | string`，proto 中也支持 `pending` 状态
- **影响**：用户无法筛选正在执行中的 Skill 调用记录
- **修复**：在 `statusOptions` 中添加 `{ label: "执行中", value: "pending" }`

---

## 问题 13：[BUG-UX] SkillDeleteDialog 删除后列表翻页逻辑有边界问题

- **文件**：`web/src/features/skills/useSkillsPage.ts` 第 129-146 行
- **现象**：`deleteTargetSkill()` 中先 `await loadRows()`，然后检查 `rows.value.length === 0 && page.value > 1`。但 `loadRows()` 已经根据当前 page 加载了数据，如果当前页最后一条被删除，`rows` 会为空。此时 `page.value -= 1` 后再次 `await loadRows()`，但如果删除的是第 1 页的唯一一条记录，`page.value` 会变为 0，导致无效请求
- **影响**：删除第 1 页唯一记录时，page 变为 0，发送无效 API 请求
- **修复**：在 `page.value -= 1` 后增加 `page.value = Math.max(1, page.value)` 保护

---

## 修复优先级

| 优先级 | 问题编号 | 说明 |
|--------|---------|------|
| P1 | #9 | 数据丢失风险 |
| P1 | #10 | 数据流违规 |
| P2 | #11 | UX 功能缺失 |
| P2 | #12 | UX 筛选不完整 |
| P1 | #13 | 边界条件 bug |
