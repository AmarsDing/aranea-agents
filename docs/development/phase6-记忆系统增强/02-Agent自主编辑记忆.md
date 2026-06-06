# 02 — Agent 自主编辑记忆

> **借鉴来源**：MemGPT/Letta（Core Memory Blocks + memory_replace/insert/rethink）
> **优先级**：高
> **影响层**：L1 工作记忆、L3 语义记忆
> **最后更新**：2026-06-06

---

## 一、需求文档

### 1.1 背景

当前 Agent 操作记忆的方式是工具调用式：`memory_add`/`memory_update`/`memory_delete`/`memory_clear`。这些工具存在以下局限：

1. **无法精确编辑**：`memory_update` 只能整体替换一条记忆的内容，无法修改特定部分
2. **无法重写**：Agent 无法对已有记忆进行深度重构（如根据新信息重新组织记忆结构）
3. **无法插入**：Agent 无法在已有记忆的特定位置插入新信息
4. **被动管理**：记忆写入由 Extractor 自动驱动，Agent 缺乏主动管理能力

### 1.2 MemGPT/Letta 的做法

Letta Agent 通过以下工具自主管理 Core Memory Blocks：

| 工具 | 功能 | 示例 |
|------|------|------|
| `memory_replace` | 在记忆块中查找并替换特定内容 | 将"喜欢 Python"替换为"喜欢 TypeScript" |
| `memory_insert` | 在记忆块的特定位置插入新内容 | 在技术栈列表中添加"Rust" |
| `memory_rethink` | 完全重写一个记忆块 | 根据新对话重新组织用户画像 |

**关键洞察**：Agent 自主编辑记忆比自动提取更精准——Agent 知道哪些信息重要、哪些需要更新、哪些应该删除。

### 1.3 目标

1. 新增 `memory_replace` 工具：在已有记忆中查找并替换特定片段
2. 新增 `memory_rethink` 工具：深度重写已有记忆的内容和结构
3. 新增 `memory_insert` 工具：在已有记忆中插入新信息
4. 保留现有 `memory_add`/`memory_update`/`memory_delete` 工具不变
5. 所有编辑操作写入操作日志（provenance）

### 1.4 功能需求

#### P0 — 必须实现

| ID | 需求 | 说明 |
|----|------|------|
| AE-P0-1 | memory_replace 工具 | Agent 可在 L3 Fact 中查找并替换特定文本片段 |
| AE-P0-2 | memory_rethink 工具 | Agent 可深度重写一条 L3 Fact 的内容 |
| AE-P0-3 | 操作日志 | 所有编辑操作记录到 action_log，支持 provenance 追溯 |
| AE-P0-4 | 向量索引同步 | 编辑后自动将 fact 标记为 `index_status=stale` |

#### P1 — 应该实现

| ID | 需求 | 说明 |
|----|------|------|
| AE-P1-1 | memory_insert 工具 | Agent 可在 L3 Fact 特定位置插入新信息 |
| AE-P1-2 | L1 字段级编辑 | Agent 可直接编辑 L1 工作记忆的特定字段 |
| AE-P1-3 | 编辑冲突检测 | 并发编辑时检测冲突并提示 Agent |

#### P2 — 可以实现

| ID | 需求 | 说明 |
|----|------|------|
| AE-P2-1 | 编辑预览 | Agent 可预览编辑结果后再确认 |
| AE-P2-2 | 批量编辑 | Agent 可一次性编辑多条相关记忆 |

### 1.5 验收标准

1. Agent 通过 `memory_replace` 可精确修改 Fact 中的特定片段，不影响其余内容
2. Agent 通过 `memory_rethink` 可完全重写 Fact 内容
3. 编辑操作后向量索引自动标记为 stale，下次索引同步时更新
4. 所有编辑操作有完整的 action_log 记录
5. 现有 `memory_add`/`memory_update`/`memory_delete` 行为不变
6. `make wire && make build && make test` 全部通过

---

## 二、设计文档

### 2.1 工具定义

#### memory_replace

```go
// 在指定记忆中查找并替换文本
type MemoryReplaceInput struct {
    MemoryID   string `json:"memory_id" jsonschema:"description=要编辑的记忆ID"`
    OldText    string `json:"old_text" jsonschema:"description=要查找的文本片段"`
    NewText    string `json:"new_text" jsonschema:"description=替换后的文本"`
}

type MemoryReplaceOutput struct {
    Success    bool   `json:"success"`
    OldContent string `json:"old_content"`
    NewContent string `json:"new_content"`
    Message    string `json:"message"`
}
```

#### memory_rethink

```go
// 深度重写一条记忆
type MemoryRethinkInput struct {
    MemoryID   string `json:"memory_id" jsonschema:"description=要重写的记忆ID"`
    NewContent string `json:"new_content" jsonschema:"description=重写后的完整内容"`
    Reason     string `json:"reason" jsonschema:"description=重写原因（用于 provenance）"`
}

type MemoryRethinkOutput struct {
    Success    bool   `json:"success"`
    OldContent string `json:"old_content"`
    NewContent string `json:"new_content"`
    Message    string `json:"message"`
}
```

#### memory_insert

```go
// 在记忆中插入新信息
type MemoryInsertInput struct {
    MemoryID   string `json:"memory_id" jsonschema:"description=要插入信息的记忆ID"`
    AfterText  string `json:"after_text" jsonschema:"description=在此文本之后插入"`
    InsertText string `json:"insert_text" jsonschema:"description=要插入的文本"`
}

type MemoryInsertOutput struct {
    Success    bool   `json:"success"`
    OldContent string `json:"old_content"`
    NewContent string `json:"new_content"`
    Message    string `json:"message"`
}
```

### 2.2 实现位置

| 组件 | 文件路径 | 说明 |
|------|----------|------|
| 工具注册 | `internal/tools/memory/tools.go` | 新增 3 个工具到注册列表 |
| 工具实现 | `internal/tools/memory/replace.go` | memory_replace 实现 |
| 工具实现 | `internal/tools/memory/rethink.go` | memory_rethink 实现 |
| 工具实现 | `internal/tools/memory/insert.go` | memory_insert 实现 |
| 操作日志 | `internal/data/memory_shim_action_log.go` | 编辑操作写入 action_log |
| 索引同步 | `internal/data/memory_fact_index_sync.go` | 编辑后标记 stale |

### 2.3 与现有工具的关系

| 现有工具 | 新工具 | 区别 |
|---------|--------|------|
| `memory_update` | `memory_replace` | update 整体替换；replace 查找替换特定片段 |
| `memory_update` | `memory_rethink` | update 简单替换；rethink 深度重写+记录原因 |
| `memory_add` | `memory_insert` | add 新增独立记忆；insert 在已有记忆中插入 |

---

## 三、开发计划

| # | 任务 | 涉及文件 | 优先级 |
|---|------|----------|--------|
| T1 | memory_replace 工具实现 | `internal/tools/memory/replace.go` | P0 |
| T2 | memory_rethink 工具实现 | `internal/tools/memory/rethink.go` | P0 |
| T3 | 操作日志集成 | `internal/data/memory_shim_action_log.go` | P0 |
| T4 | 索引同步标记 | `internal/data/memory_fact_index_sync.go` | P0 |
| T5 | 工具注册 | `internal/tools/memory/tools.go` | P0 |
| T6 | memory_insert 工具实现 | `internal/tools/memory/insert.go` | P1 |
| T7 | L1 字段级编辑 | `internal/data/memory_shim_l1.go` | P1 |
| T8 | 集成测试 | `internal/tools/memory/replace_test.go` | P0 |
| T9 | 全量构建验证 | 全局 | P0 |
