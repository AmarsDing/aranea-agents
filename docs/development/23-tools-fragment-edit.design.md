# Tools — 片段级文件编辑 — 技术设计

> **版本**：1.1 | **状态**：✅ 已实现（2026-05-22）
> **需求**：[23 tools-fragment-edit.md](./23%20tools-fragment-edit.md) · **开发计划**：[23-tools-development.md §Phase 4](./23-tools-development.md#phase-4片段级文件编辑p1)
> **父设计**：[23 tools.design.md §七 运行时层](./23%20tools.design.md#七运行时层)

---

## 1. 设计目标

在 **trpc-agent-go `tool/file` ToolSet** 内新增 `diff_edit`、`patch_file`，并引入 **SessionFileState** 会话缓存，使片段编辑在运行时达到：

- **O(变更大小)** 的模型输出与内存处理
- **1 读 + 1 写** 的磁盘访问（同 invocation 缓存命中时 0 读）
- **原子多 hunk** 提交

---

## 2. 分层与依赖

```
┌─────────────────────────────────────────────────────────┐
│  internal/agent/prompt.go          ← 编辑工作流提示        │
│  internal/data/builtin_tools_seed  ← catalog 种子         │
│  internal/biz/agent_effective_tools← toolGroupsFilesystem │
│  internal/tools/runtime_alias.go   ← 别名（Phase 2）      │
│  internal/tools/testexec/config.go ← 在线测试映射         │
│  internal/agent/activity_meta.go   ← 活动流中文标签        │
└──────────────────────────┬──────────────────────────────┘
                           │ Assemble(file ToolSet)
┌──────────────────────────▼──────────────────────────────┐
│  internal/tools/toolset.go         ← Registry 不变       │
│  internal/tools/trpc/toolsets.go   ← FilesystemDir 注入  │
└──────────────────────────┬──────────────────────────────┘
                           │
┌──────────────────────────▼──────────────────────────────┐
│  pkg/trpc-agent-go/tool/file/      ← ★ 实现真相源        │
│    diffedit.go / patchfile.go                            │
│    editcontent.go        ← load/commit + SessionFileState │
│    patch/                ← parse / apply / validate        │
│  pkg/trpc-agent-go/tool/internal/textfile/  ← 抽取共享   │
│    （encoding / line ending / quote fuzzy）               │
└───────────────────────────────────────────────────────────┘
         ↑ 复用逻辑来源（抽取，不复制业务到 internal/）
         pkg/trpc-agent-go/tool/claudecode/
           file_state.go · common.go (buildStructuredPatch)
```

**红线**：

- 实现放在 `pkg/trpc-agent-go/tool/file`，**禁止**在 `internal/biz` import trpc-agent-go 实现编辑逻辑
- `internal/tools` 仅做装配、别名、catalog，不写 patch 算法

---

## 3. 工具 API

### 3.1 `diff_edit`

**Declaration name**：`diff_edit`

```json
{
  "type": "object",
  "properties": {
    "file_name": {
      "type": "string",
      "description": "Relative file path under base_directory"
    },
    "edits": {
      "type": "array",
      "minItems": 1,
      "maxItems": 20,
      "items": {
        "type": "object",
        "properties": {
          "search": { "type": "string", "description": "Text to find; multi-line allowed" },
          "replace": { "type": "string", "description": "Replacement text; empty string deletes" },
          "replace_all": { "type": "boolean", "default": false }
        },
        "required": ["search", "replace"]
      }
    },
    "expected_mtime_ms": {
      "type": "integer",
      "description": "Optional optimistic lock from prior read_file"
    }
  },
  "required": ["file_name", "edits"]
}
```

**Description 要点**（写入 function.WithDescription）：

- Read file first; use `search_content` to locate symbols when needed
- Only changed fragments — never whole file
- Prefer over `save_file` for modifications
- Use `patch_file` when you already have unified diff

### 3.2 `patch_file`

**Declaration name**：`patch_file`

```json
{
  "type": "object",
  "properties": {
    "file_name": { "type": "string" },
    "patch": {
      "type": "string",
      "description": "Unified diff text; mutually exclusive with hunks"
    },
    "hunks": {
      "type": "array",
      "description": "Structured hunks; mutually exclusive with patch",
      "items": {
        "type": "object",
        "properties": {
          "old_start": { "type": "integer" },
          "old_lines": { "type": "integer" },
          "new_start": { "type": "integer" },
          "new_lines": { "type": "integer" },
          "lines": {
            "type": "array",
            "items": { "type": "string" },
            "description": "Unified diff body lines with ' ', '-', '+' prefixes"
          }
        },
        "required": ["old_start", "old_lines", "new_start", "new_lines", "lines"]
      }
    },
    "expected_mtime_ms": { "type": "integer" }
  },
  "required": ["file_name"]
}
```

约束：`patch` 与 `hunks` 必须且仅能提供一个。

### 3.3 响应（共用字段）

```json
{
  "base_directory": "...",
  "file_name": "...",
  "applied_edits": 2,
  "total_replacements": 3,
  "structured_patch": [ { "old_start": 42, "old_lines": 5, "new_start": 42, "new_lines": 7, "lines": ["-...", "+..."] } ],
  "message": "Applied 2 edits to 'internal/foo.go'"
}
```

`structured_patch` 格式与 claudecode `patchHunk` 对齐，供 Activity / 前端 diff 预览扩展。

---

## 4. SessionFileState

### 4.1 数据结构

实现位于 **`editcontent.go`**（load/commit 编排）与 **`internal/toolcache/file_views.go`**（per-invocation 存取），**不**挂在 `fileToolSet` 实例字段上。

```go
// internal/toolcache/file_views.go

type FileView struct {
    Content    string
    MtimeMs    int64
    Encoding   string
    LineEnding string
    Mode       os.FileMode
}
```

- 挂在 **`agent.Invocation.State`**（`toolcache.StoreFileViewFromContext`），与 `skill_run` 输出缓存同模式
- 与 `BuildTRPCLLMAgentCached`（Agent LRU ~10min）兼容，避免跨 session 泄漏
- **同一文件勿并行** `diff_edit` / `patch_file`；可选 `expected_mtime_ms`（来自 `read_file.mtime_ms`）作乐观锁

### 4.2 读写协议

| 操作 | 行为 |
|------|------|
| `read_file` | 读盘后 `storeFileViewAfterRead`；响应含 **`mtime_ms`** |
| `diff_edit` / `patch_file` | `loadEditSnapshot` → cache 命中且 mtime 一致则跳过 ReadFile → apply → `commitEditSnapshot` → `storeFileView` |
| `save_file` / `replace_content` | 写盘后 `storeSaveFileView`（读回磁盘再解码，保持 encoding 一致） |
| mtime 不匹配 | 返回 `file_modified_externally`，hint re-read |
| 磁盘文件已删、cache 仍命中 | **同 invocation 设计取舍**：仍用 cache 内容编辑并可写回重建；外部删改以 mtime 为准；见 Review FRAG-P2-03 |

### 4.3 与 claudecode fileState 的关系

| 项 | claudecode | file ToolSet SessionFileState |
|----|------------|-------------------------------|
| read-before-write 强制 | 是 | **否**（Prompt 软约束 + mtime 硬约束） |
| partial read 视图 | 是 | Phase 1 全文件；Phase 2 可记录 slice |
| 引号 fuzzy | 是 | **复用**抽取后的 `findActualString` |

抽取目标包：`pkg/trpc-agent-go/tool/internal/textfile`（claudecode 与 file 共同 import）。

---

## 5. 核心算法

### 5.1 diff_edit 流程

```
resolvePath(file_name)
  → loadContent (cache or ReadFile)
  → validate text / not binary / not .ipynb
  → check expected_mtime_ms
  → for each edit in edits:
        resolve search (exact → whitespace → quote fuzzy)
        count matches; enforce replace_all policy
        apply strings.Replace on in-memory content
  → buildStructuredPatch(old, new)
  → WriteFile (preserve mode)
  → storeView
  → return response
```

任一 edit 失败：**不 WriteFile**，返回结构化错误（含 `edit_index`、`match_lines`）。

### 5.2 patch_file 流程

```
parse patch string OR validate hunks[]
  → loadContent
  → for each hunk (ascending old_start):
        verify '-' lines against file lines at old_start
        apply splice
  → WriteFile + storeView
```

Unified diff 解析子集（Phase 1）：

- `@@ -old_start,old_lines +new_start,new_lines @@` hunk header
- 行前缀：` ` context、`-` delete、`+` insert
- 不支持：git binary patch、`diff --git` 多文件（单文件 only）

### 5.3 patch 包

```
pkg/trpc-agent-go/tool/file/patch/
  hunk.go           // patchHunk 类型（自 claudecode 迁入）
  apply.go          // ApplyHunks(content, hunks) (string, error)
  parse_unified.go  // ParseUnifiedDiff(patch) ([]patchHunk, error)
  validate.go       // ValidateHunk(fileLines, hunk) error
```

---

## 6. file ToolSet 注册

在 `file.go` 的 `NewToolSet` 中：

```go
type fileToolSet struct {
    // ...existing fields...
    diffEditEnabled   bool  // default true
    patchFileEnabled  bool  // default true
}
```

工具列表追加（在 `replaceContentEnabled` 之后）：

- `diffEditTool()`
- `patchFileTool()`

Limits（常量）：

| 常量 | 默认值 |
|------|--------|
| `maxEditsPerCall` | 20 |
| `maxPatchBytes` | 256 KiB |
| `maxEditSearchBytes` | 64 KiB per search block |
| `maxEditReplaceBytes` | 256 KiB per replace block |

---

## 7. 项目集成清单

| # | 位置 | 变更 |
|---|------|------|
| 1 | `internal/data/builtin_tools_seed.go` | 新增 `diff_edit`、`patch_file` seed |
| 2 | `internal/biz/agent_effective_tools.go` | `toolGroupsFilesystem` 追加 key |
| 3 | `internal/biz/tool_policy_keys.go` | 若需 policy alias |
| 4 | `internal/tools/runtime_alias.go` | Phase 2：`edit_file` → `diff_edit`（可选） |
| 4b | `internal/biz/tool_policy_keys.go` | **须与 runtime_alias 同步**（allow/deny 策略键） |
| 5 | `internal/tools/testexec/config.go` | `AssemblyForCatalogKey` 映射到 `file` |
| 6 | `internal/agent/prompt.go` | 编辑工作流：`diff_edit` 优先 |
| 7 | `internal/agent/activity_meta.go` | 中文标签 |
| 8 | `web/src/features/agents/useAgentToolsCatalog.ts` | filesystem 组展示（若硬编码列表） |

**Registry**：仍注册 `file` ToolSet 一条，不新增 Registry 名。

---

## 8. 错误响应结构

工具错误通过 `message` + 结构化 JSON 字段返回（function tool result）：

**diff_edit — 歧义匹配**

```json
{
  "error": "edit_not_unique",
  "edit_index": 1,
  "match_count": 3,
  "match_lines": [42, 108, 205],
  "hint": "Add more context to search or set replace_all=true"
}
```

**patch_file — hunk 不一致**

```json
{
  "error": "hunk_mismatch",
  "hunk_index": 0,
  "expected_lines": ["-    return 1"],
  "actual_lines": ["-    return 0"],
  "hint": "Re-read file and regenerate patch"
}
```

**并发冲突**

```json
{
  "error": "file_modified_externally",
  "hint": "Call read_file again before editing"
}
```

---

## 9. 性能设计

| 层级 | 手段 | 预期 |
|------|------|------|
| 模型 | 片段参数 | 生成时间主导项，显著下降 |
| 运行时 | SessionFileState | 同文件二次编辑省 1 次 ReadFile |
| 运行时 | 单次多 edit | N 次 tool call → 1 次 |
| 运行时 | 内存 patch | 毫秒级（Go string splice） |
| Phase 2 | 行区间读 | >1MB 文件仅加载 hunk ±context |

**端到端**：磁盘 I/O 已接近 Cursor 本地写入；Agent 场景仍受 LLM 推理约束，无法达到 IDE 即时感。

---

## 10. 安全

- 复用 `resolvePath`、`maxFileSize`、文本校验（与 `read_file` / `replace_content` 一致）
- `maxPatchBytes` / `maxEditSearchBytes` / `maxEditReplaceBytes` 防 DoS
- **同文件勿并行** `diff_edit` / `patch_file`（Invocation 缓存无锁；Prompt + `expected_mtime_ms` 软/硬约束）
- 写操作保留原 `FileMode`
- 不经过 Tool 结果缓存（非幂等）

---

## 11. 测试策略

| 包 | 覆盖 |
|----|------|
| `tool/file/patch/*_test.go` | unified 解析、hunk apply、偏移累加 |
| `tool/file/diffedit_test.go` | 多 edit 原子性、fuzzy、歧义、mtime_ms |
| `tool/file/patchfile_test.go` | mismatch 回滚、新建文件 |
| `internal/toolcache/file_views.go` + `editcontent.go` | cache hit / mtime 失效 |
| `internal/tools/testexec/config_test.go` | catalog key 映射 |

---

## 12. 迁移阶段（技术侧）

| 阶段 | 内容 |
|------|------|
| **T1** | 抽取 `textfile` + `patch` 包；`patch_file`（hunks 模式） |
| **T2** | unified diff 解析；`diff_edit` |
| **T3** | SessionFileState + read/save/replace 写回缓存 |
| **T4** | catalog / prompt / activity 集成 |
| **T5** | 可选：`edit_file` 别名 → `diff_edit`；`replace_content` deprecated 描述 |

任务编号与验收勾选见 [23-tools-development.md §Phase 4](./23-tools-development.md#phase-4片段级文件编辑p1)。

---

*文档版本：1.0 — 片段级文件编辑技术设计；不含修复记录与 sprint 进度。*
