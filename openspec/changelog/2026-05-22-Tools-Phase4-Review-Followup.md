# Tools Phase 4 — Review Follow-up (P0–P2)

**日期**：2026-05-22  
**模块**：Tools (23)

## 摘要

Phase 4 代码审查后续：闭环 `mtime_ms` 乐观锁、修正 AppliedEdits 计数与 encoding 缓存、Activity 摘要、别名迁移与文档同步。

## 框架层

| 项 | 说明 |
|----|------|
| `read_file` | 响应新增 `mtime_ms`；description 说明供 `expected_mtime_ms` 使用 |
| `diff_edit` | 已有文件禁止空 `search`；`replace` 体积上限；AppliedEdits 仅计实际变更 edit |
| `patch.ApplyEdits` | 返回 `(content, replacements, appliedEdits, err)` |
| `storeSaveFileView` | 写盘后读回磁盘再 `storeFileViewAfterRead`，保持 encoding |
| `diff_edit` / `patch_file` | description 注明同文件勿并行编辑 |

## 项目桥接

| 项 | 说明 |
|----|------|
| `runtime_alias.go` | `edit_file` → `diff_edit`（原 → `replace_content`） |
| `tool_policy_keys.go` | 同上，allow/deny 策略键与运行时对齐 |
| `activity_meta.go` | `file_name` 摘要；`applied_edits` / `applied_hunks` / `structured_patch` 计数 |
| `builtin_tools_seed.go` | `read_file` / `replace_content` / 新工具 schema 与运行时 `file_name` 对齐 |

## 文档

- [23 tools-fragment-edit.design.md §4](../需求/23%20tools-fragment-edit.design.md) — SessionFileState 改为 `toolcache` + `editcontent.go`
- [23 tools-fragment-edit.md §5](../需求/23%20tools-fragment-edit.md) — 验收勾选
- [23-tools-development.md](../需求/23-tools-development.md) — Phase 4.10 ✅
- [Review](../review/2026-05-22-Tools-Phase4-Fragment-Edit-Review.md) — 架构/影响域审查

## 测试

```bash
cd pkg/trpc-agent-go && go test ./tool/file/ -run "DiffEdit|PatchFile|FileViewCache|ReadFile_ReturnsMtime" -count=1
cd pkg/trpc-agent-go && go test ./tool/file/patch/... -count=1
go test ./internal/agent/... -run Activity -count=1
go test ./internal/tools/testexec/... -count=1
```

## 待办（P3）

- Monitor/Chat 卡片 UI 级 diff 预览（消费 `structured_patch` 全文）
- 大文件行区间 patch（>1MB）
