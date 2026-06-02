# Tools Phase 4 — 片段级文件编辑

**日期**：2026-05-22  
**模块**：Tools (23)

## 摘要

在 trpc-agent-go `file` ToolSet 内新增 `diff_edit`、`patch_file`，并引入 invocation 级 SessionFileState（`toolcache.FileView`），实现 Cursor 式片段编辑：多片段原子提交、unified diff 应用、同会话读缓存。

## 框架层（pkg/trpc-agent-go）

| 项 | 说明 |
|----|------|
| `tool/internal/textfile` | encoding / line ending / quote fuzzy；claudecode 复用 |
| `tool/file/patch/` | hunk 类型、ApplyEdits、unified 解析 |
| `diffedit.go` | 多 edit 原子提交；结构化错误（EditNotUnique / EditNotFound） |
| `patchfile.go` | unified diff + 结构化 hunk |
| `editcontent.go` | loadEditSnapshot / commitEditSnapshot + mtime 乐观锁 |
| `internal/toolcache/file_views.go` | SessionFileState；read 后 store、edit 命中缓存 |

## 项目桥接

| 项 | 说明 |
|----|------|
| `builtin_tools_seed.go` | catalog `diff_edit` / `patch_file` |
| `agent_effective_tools.go` | filesystem 组 |
| `activity_meta.go` | 活动流中文标签 |
| `prompt.go` | search → read → diff_edit 工作流 |
| `testexec/config.go` | 在线测试映射 |
| `useAgentToolsCatalog.ts` | Agent 设置默认 key |

## 测试

```bash
cd pkg/trpc-agent-go && go test ./tool/file/ -run "DiffEdit|PatchFile|FileViewCache" -count=1
cd pkg/trpc-agent-go && go test ./tool/file/patch/... -count=1
go test ./internal/tools/testexec/... -count=1
```

## 待办（可选）

- ~~`edit_file` 别名迁移至 `diff_edit`~~ → ✅ [Review Follow-up](./2026-05-22-Tools-Phase4-Review-Followup.md)
- ~~Activity 卡片消费 `structured_patch`~~ → ✅ Summary 计数；UI diff 预览仍 P3
- 大文件行区间 patch（>1MB）

## 文档

- [23-tools-development.md §Phase 4](../需求/23-tools-development.md#phase-4片段级文件编辑p1)
- [23 tools-fragment-edit.md](../需求/23%20tools-fragment-edit.md)
