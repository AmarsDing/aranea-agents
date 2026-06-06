# Tools — 片段级文件编辑（diff_edit / patch_file）

> **版本**：1.1 | **状态**：✅ 已实现（2026-05-22）
> **所属模块**：**Tools（23）** — `filesystem` 分类运行时能力增强
> **设计**：[23 tools-fragment-edit.design.md](./23%20tools-fragment-edit.design.md) · **开发计划**：[23-tools-development.md §Phase 4](./23-tools-development.md#phase-4片段级文件编辑p1)
> **父文档**：[23 tools.md](./23%20tools.md) · [23 tools.design.md](./23%20tools.design.md)

---

## 0. 模块归属

| 维度 | 归属 |
|------|------|
| **产品模块** | Tools 工具管理（编号 23） |
| **工具分类** | `filesystem` |
| **框架实现** | `pkg/trpc-agent-go/tool/file`（trpc-agent-go 框架层，真相源） |
| **项目桥接** | `internal/tools`（Registry / Assemble / 别名）、`internal/data`（catalog seed）、`internal/agent`（Prompt 工作流提示） |
| **不涉及** | Proto RPC 变更、新数据库表、前端新页面（仅 catalog 展示同步） |

与 Agent 创建/设置（2/5）、Channel、Memory 等模块无直接耦合；生效路径为 **Agent 工具装配 → 运行时 ToolSet**。

---

## 1. 背景与问题

### 1.1 现状

默认 `file` ToolSet 提供：

| 工具 | 行为 | 问题 |
|------|------|------|
| `save_file` | 整文件覆盖写入 | 大文件 token 高、易截断、误改无关行 |
| `replace_content` | 单次精确 `old_string` → `new_string` | 一处修改一次调用；无会话缓存；整文件读写 |

Agent 系统提示已引导 `search_content → read_file → replace_content/save_file`，但缺少 **Cursor 式片段编辑**：只传变更块、单文件多处修改一次提交、同会话减少重复读盘。

### 1.2 目标

| 目标 | 说明 |
|------|------|
| **降 token** | 模型只输出变更片段，不传整文件 |
| **降往返** | 单文件 N 处修改合并为 1 次工具调用 |
| **提速** | 同会话内 read/edit 命中内存缓存，磁盘 1 读 1 写 |
| **可诊断** | 匹配失败返回行号、上下文 snippet，便于模型 self-heal |

### 1.3 非目标（本期不做）

- 跨文件原子事务（多文件各自独立提交）
- 在线 diff 可视化 UI
- 替代 `claudecode` ToolSet（Bash / NotebookEdit 等仍走 claudecode）
- 二进制文件、`.ipynb` 的片段编辑
- 基于 LSP / AST 的结构化重构

---

## 2. 用户故事

### US-FE-1：Agent 片段编辑大文件

**作为** 使用 coding Agent 的开发者，**我希望** Agent 修改 50KB+ 源文件时只提交变更片段，**以便** 响应更快且不易因 token 截断写坏文件。

**验收**：

- Agent 可通过 `diff_edit` 一次提交同一文件的多个 `search`/`replace` 块
- 不使用 `save_file` 覆盖已有大文件（工具描述与 Prompt 明确约束）
- 单次编辑工具参数体积显著小于整文件（见 §5 量化指标）

### US-FE-2：Agent 应用 unified diff

**作为** Agent，**我希望** 在已生成标准 diff 时直接应用，**以便** 不必把 diff 手工转成 search/replace 块。

**验收**：

- Agent 可通过 `patch_file` 传入 unified diff 或结构化 hunk 列表
- 任一 hunk 与磁盘内容不一致时整次失败且不落盘
- 失败响应包含 hunk 索引与 expected/actual 行摘要

### US-FE-3：同会话连续编辑

**作为** Agent，**我希望** 对刚读过的文件再次编辑时不必重复读盘，**以便** 多轮修改更流畅。

**验收**：

- 同 invocation 内：`read_file` 后 `diff_edit`/`patch_file` 优先使用会话缓存
- 写盘后缓存更新，后续编辑不再额外 `ReadFile`
- 外部进程修改文件（mtime 变化）时拒绝静默覆盖，提示 re-read

### US-FE-4：平台管理员识别新工具

**作为** 平台使用者，**我希望** 在 Tools  catalog 中看到 `diff_edit` / `patch_file` 及风险级别，**以便** 通过 Agent allow/deny 控制暴露范围。

**验收**：

- `builtin_tools_seed` 含两条 catalog 记录，分类 `filesystem`，风险 `medium`
- Effective Tools / Agent 工具矩阵包含新 key
- 活动流展示中文标签（如「片段编辑」「应用补丁」）

---

## 3. 功能规格

### 3.1 新增工具：`diff_edit`

| 项 | 规格 |
|----|------|
| **用途** | 对已有文本文件施加 1～N 处片段替换 |
| **必填参数** | `file_name`；`edits[]`（每项含 `search`、`replace`） |
| **可选参数** | `replace_all`（per-edit）；`expected_mtime_ms`（乐观并发） |
| **默认策略** | 每处 `search` 须唯一匹配，否则报错 |
| **原子性** | 所有 edit 在内存校验通过后一次写盘；任一失败则不写 |
| **新建文件** | 仅当 `search` 为空且目标不存在或为空文件时允许 |
| **默认启用** | 是（与 `read_file` 同属 filesystem profile） |

### 3.2 新增工具：`patch_file`

| 项 | 规格 |
|----|------|
| **用途** | 应用 unified diff 或结构化 hunk 列表 |
| **输入模式** | `patch`（字符串）与 `hunks[]`（结构化）二选一 |
| **必填参数** | `file_name`；`patch` 或 `hunks` |
| **可选参数** | `expected_mtime_ms` |
| **校验** | hunk 的删除行须与当前文件逐行一致（含 context 行） |
| **原子性** | 同 `diff_edit` |
| **默认启用** | 是 |

### 3.3 会话文件缓存（Fast Path）

| 项 | 规格 |
|----|------|
| **范围** | 单次 Agent invocation 内，按绝对路径缓存文本内容与 mtime |
| **写入时机** | `read_file` 成功后；`diff_edit` / `patch_file` / `save_file` 写盘后 |
| **失效** | 磁盘 mtime 与缓存不一致；invocation 结束 |
| **不替代** | Tool 结果缓存（`metadata_json.cache_enabled`，只读幂等工具） |

### 3.4 与现有工具的分工

| 场景 | 推荐工具 |
|------|----------|
| 新建文件 | `save_file` |
| 极小文件全量重写（如 <100 行配置） | `save_file` |
| 修改已有文件（默认） | `diff_edit` |
| 已有 unified diff | `patch_file` |
| 单次简单替换（兼容） | `replace_content`（`edit_file` 别名） |

### 3.5 编辑工作流（Prompt 约束）

推荐顺序：

```
search_content（定位）
  → read_file（大文件用 start_line / num_lines）
  → diff_edit（默认）或 patch_file（已有 diff）
  → 失败则 re-read 并重试
```

禁止：用 `save_file` 修改已有中大型源文件。

---

## 4. 产品决策

| 决策项 | 默认值 |
|--------|--------|
| 风险级别 | `medium`（与 `save_file` / `replace_content` 一致） |
| 二次确认 | 否（除非 Agent Override 或 profile 另行要求） |
| `edit_file` 别名 | Phase 1 仍指向 `replace_content`；Phase 2 可选迁移至 `diff_edit` |
| 并行执行 | 支持（`SupportsConcurrency: true`，与 file ToolSet 一致） |
| 大文件策略 | 先 `read_file` 分段；Phase 2 可选行区间 patch（>1MB） |
| claudecode | 不合并；需要 Bash/Notebook 时继续启用 claudecode ToolSet |

---

## 5. 验收标准

### 5.1 功能

- [x] `diff_edit` 可在单次调用中应用 ≥2 处非连续替换且原子提交
- [x] `patch_file` 可应用标准 unified diff；hunk mismatch 时零副作用
- [x] 同 invocation 内第二次编辑同一文件不触发额外磁盘读（mtime 未变）
- [x] 外部修改文件后编辑被拒绝并提示 re-read（`expected_mtime_ms` / cache mtime）
- [x] `read_file` 响应含 `mtime_ms`，供编辑工具乐观锁
- [x] catalog、Effective Tools、testexec 映射、Activity 中文标签齐全
- [x] `replace_content` / `save_file` 保持可用，无破坏性变更

### 5.2 性能（量化）

| 指标 | 目标 |
|------|------|
| 单文件 3 处修改 tool 调用数 | ≤2（1 read + 1 diff_edit） |
| 50KB 文件编辑 LLM 输出 | ≪ 50KB（片段级，通常 <2KB） |
| 单次工具执行（I/O） | ≤10ms 量级（1 读 + 内存 patch + 1 写，不含 LLM） |

### 5.3 安全

- [x] 路径校验与现有 `file` ToolSet 一致（禁止 `..`、绝对路径）
- [x] 拒绝二进制与 `.ipynb`
- [x] 单次 patch / search / replace 体积上限（见设计文档常量）

---

## 6. 与相关文档边界

| 文档 | 本文关系 |
|------|----------|
| [23 tools.md](./23%20tools.md) | 父需求；catalog 总表由父文档索引，细节在本文 |
| [23 tools-fragment-edit.design.md](./23%20tools-fragment-edit.design.md) | 技术方案、Schema、分层、代码锚点 |
| [23-tools-development.md](./23-tools-development.md) | 任务拆分、Phase、实现差距（**进度真相**） |
| [guides/trpc-agent-go-framework.md](../guides/trpc-agent-go-framework.md) | 框架 Tool / ToolSet 通用约定 |
| [guides/AI-DEVELOPMENT-SPECIFICATION.md](../guides/AI-DEVELOPMENT-SPECIFICATION.md) | 分层红线；实现不得违反 biz 不 import trpc-agent-go |

---

*文档版本：1.0 — 片段级文件编辑产品需求；实现状态以 [23-tools-development.md](./23-tools-development.md) 为准。*
