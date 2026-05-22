# Tools Phase 4 — 片段编辑 Code Review

> **评分**：88 / 100 | **风险等级**：P2（可合并使用）  
> **范围**：`diff_edit` / `patch_file` / SessionFileState + 项目桥接 + P0–P2 审查后续  
> **依据**：[docs/README.md](../README.md) · [AI-DEVELOPMENT-SPECIFICATION.md](../guides/AI-DEVELOPMENT-SPECIFICATION.md)  
> **审查时间**：2026-05-22

**关联文档**：[需求](../需求/23%20tools-fragment-edit.md) · [设计](../需求/23%20tools-fragment-edit.design.md) · [开发计划 Phase 4](../需求/23-tools-development.md#phase-4片段级文件编辑p1) · [Phase 5 Review](./2026-05-22-Tools-Phase5-Workspace-Unification-Review.md) · [23-tools-review](./23-tools-review.md) · [changelog Phase 4](../changelog/2026-05-22-Tools-Phase4-Fragment-Edit.md) · [Review Follow-up](../changelog/2026-05-22-Tools-Phase4-Review-Followup.md)

---

## 总评

| 维度 | 评级 | 说明 |
|------|------|------|
| 架构分层 | **良好** | 算法在 `pkg/trpc-agent-go`；`internal/biz` 无 trpc import；Service 未写编辑逻辑 |
| 单一职责 | **良好** | `textfile` / `patch` 纯函数；`editcontent` I/O+缓存；工具 façade 薄 |
| 业务逻辑 | **良好** | 原子 apply、mtime 乐观锁、`read_file.mtime_ms` 闭环、AppliedEdits 计数修正 |
| 代码质量 | **良好** | 边界校验、结构化错误、encoding 写回一致 |
| 文档一致性 | **良好** | 需求/设计/development/changelog 已同步；本 review 记录残余 P2/P3 |
| 测试 | **中等** | 核心 happy path + 多 hunk；缺并行竞态、utf16 回归 |

**结论**：Phase 4 可视为 **已完成基线**；本次 review 修复 policy 别名漂移与 Activity `applied_hunks` 摘要。

---

## 架构（符合双框架分工）

```
internal/agent/prompt.go          ← 工作流提示
internal/data/builtin_tools_seed  ← catalog
internal/biz/agent_effective_tools← filesystem 组
internal/tools/runtime_alias.go   ← 运行时工具名
internal/biz/tool_policy_keys.go  ← allow/deny 策略键（须与 runtime 对齐）
internal/agent/activity_meta.go   ← 活动流摘要
        ↓ Assemble(file)
pkg/trpc-agent-go/tool/file/      ← ★ 实现真相源
  diffedit.go / patchfile.go / editcontent.go
  patch/ · internal/textfile/
pkg/trpc-agent-go/internal/toolcache/file_views.go  ← per-invocation 缓存
```

**红线检查**：通过 — 无 Proto 变更、无 Ent 新表、无 `internal/biz` import 框架运行时。

---

## 做得好的

1. **SessionFileState 挂 Invocation** — 与 Agent LRU 缓存兼容，优于 ToolSet 实例级方案。
2. **storeSaveFileView 读回磁盘** — 与 `read_file` encoding 一致，避免 utf16 漂移。
3. **ApplyEdits 三返回值** — `appliedEdits` 不含 no-op（search==replace）。
4. **Prompt 工作流** — `search_content → read_file → diff_edit` 已写入 `prompt.go`。
5. **Feature 开关** — `WithDiffEditEnabled` / `WithPatchFileEnabled` 默认可关。

---

## 本次 Review 修复

| ID | 问题 | 修复 |
|----|------|------|
| FRAG-P1-01 | `tool_policy_keys`: `edit_file`→`replace_content` 与 `runtime_alias`→`diff_edit` 不一致，allow/deny 与运行时解析分裂 | ✅ `tool_policy_keys.go` 改为 `diff_edit` |
| FRAG-P2-01 | Activity `fileEditResultSummary` 未读 `patch_file.applied_hunks` | ✅ 优先 `applied_hunks`，fallback `structured_patch` |

---

## 残余风险（P2 / P3）

| ID | 优先级 | 问题 | 建议 |
|----|--------|------|------|
| FRAG-P2-02 | P2 | 同文件并行 `diff_edit` 无锁，Invocation map 可能 lost update | 已文档化 + Prompt；P3 可考虑 Registry 禁并发 |
| FRAG-P2-03 | P2 | 磁盘删除后仍用 cache 编辑可静默重建文件 | 设计 §4 边界说明；可选：Stat 失败且 cache 命中时要求 re-read |
| FRAG-P2-04 | P2 | `replace_content` 仍 `string(data)` 全字节，非 textfile 解码 | 与 Phase 4 无关；独立小改 |
| FRAG-P2-05 | P2 | Catalog seed `ON CONFLICT DO NOTHING`，已有 DB 行 schema 不自动更新 | 运维 sync 或 migration 脚本 |
| FRAG-P3-01 | P3 | Monitor/Chat UI 级 diff 预览（消费完整 `structured_patch`） | 前端卡片迭代 |
| FRAG-P3-02 | P3 | 大文件行区间 patch（>1MB） | Phase 2 backlog |
| FRAG-P3-03 | P3 | `editFileSnapshot.Raw` 未使用 | 删除或用于二进制二次校验 |

---

## 影响域

| 域 | 影响 |
|----|------|
| 默认 file ToolSet | +2 工具；token / 选择面略增 |
| `edit_file` 别名 | 运行时与策略均 → `diff_edit`；依赖 `replace_content` 语义的 Agent 需显式保留 `replace_content` |
| `read_file` / 写工具 | +cache 写入；兼容 |
| claudecode | 复用 `textfile`+`patch`；子模块独立测试 |
| 无 Proto / DB / 新页 | 低 |

---

## 测试建议

```bash
cd pkg/trpc-agent-go && go test ./tool/file/patch/... ./tool/file/ -run "DiffEdit|PatchFile|FileViewCache|ReadFile_ReturnsMtime" -count=1
go test ./internal/agent/... -run "BuildSummary|FileEdit|ActivityMeta" -count=1
go test ./internal/tools/testexec/... -count=1
```

---

## 文档同步清单

- [x] `23 tools-fragment-edit.md` §5 验收
- [x] `23 tools-fragment-edit.design.md` §4 SessionFileState
- [x] `23-tools-development.md` Phase 4 / 4.10
- [x] `execution-plan.md` TW-4-11 / TW-4-12
- [x] changelog Phase 4 + Review Follow-up
- [x] 本 review 文档
