# CodeGraph MCP + Graphify MCP 融入 Aranea-Agents 方案

> 版本：v1.4 | 日期：2026-06-08 | 状态：草案（v1.4 修订：Import() 三步执行+ID 解析、rt.Workspace 语义澄清、批量写入 SQL 语义统一、name_normalized 规则说明）

---

## 1. 定位与分工

### 1.1 两者职责划分

| 维度 | CodeGraph MCP | Graphify MCP |
|------|--------------|-------------|
| **核心能力** | 代码结构查询（AST 符号级） | 语义知识图谱查询（概念级） |
| **回答的问题** | 函数在哪？谁调用了它？影响面多大？ | 模块怎么划分？设计意图是什么？哪些概念关联？ |
| **数据来源** | Tree-sitter AST 实时解析 | 离线预构建 graph.json（AST + LLM 语义提取） |
| **查询粒度** | 函数/类/调用链 | 概念/社区/设计意图/跨模态关联 |
| **类比** | 字典（精确查找） | 百科全书（理解上下文） |
| **来源项目** | [colbymchenry/codegraph](https://github.com/colbymchenry/codegraph) | [safishamsi/graphify](https://github.com/safishamsi/graphify) |

### 1.2 与项目现有系统的关系

```
Aranea Agent 运行时
  │
  ├── MCP ToolSets（外部工具，配置注册）
  │   ├── CodeGraph MCP        → 代码结构查询
  │   └── Graphify MCP          → 语义图谱查询
  │
  ├── L4 知识图谱（内部基础设施，已有代码）
  │   ├── AutoMemoryWorker      → 对话知识（人名/偏好，正则提取，scope_type=agent）
  │   └── GraphifyImporter      → 代码知识（graph.json → memory_entities/relations，scope_type=codebase）
  │
  ├── L4 Prompt 注入（双通道隔离）
  │   ├── L4MemoryCue()         → 对话知识注入（scope_type=agent，已有）
  │   └── L4CodeCue()           → 代码知识注入（scope_type=codebase，新增）
  │
  └── L0 装配层（Phase 4 后）
      ├── system.prompt / system.skill → 角色与规则（已有）
      ├── memory.l4                    → L4CodeCue() 代码图谱段（新增）
      └── memory.l4.report             → GraphifyReportCue() 图谱摘要段（新增）
```

**关键原则**：

1. CodeGraph 和 Graphify 作为 MCP Server 接入，**不修改项目核心代码**，仅通过数据库配置注册
2. GraphifyImporter 导入的代码实体使用 **`scope_type="codebase"`** 隔离，与 AutoMemoryWorker 的 `scope_type="agent"` 对话实体**物理隔离**
3. 代码知识注入走独立的 **`L4CodeCue()`** 函数，与对话知识注入 `L4MemoryCue()` **逻辑隔离**，各有独立的开关和预算控制

---

## 2. 阶段 1：MCP Server 即插即用

### 2.1 集成机制

项目已有完整的 MCP Server 注册链路，**零代码改动**：

```
管理后台配置
    │
    ▼
resolveMCPServers()                    [tool_assembly.go:285]
    │  从 deps.MCPTooling.EffectiveServersForAgent() 获取 Agent 绑定的 MCP 服务器
    │  解析 config_json → mcpconfig.ServerConfig
    ▼
buildMCPToolSet()                      [toolset.go:872]
    │  cfg.ToConnectionConfig() → trpcmcp.ConnectionConfig
    │  trpcmcp.NewMCPToolSet(connCfg, opts...)
    ▼
Agent 运行时
    │  trpcllmagent.WithToolSets(ts.ToolSets)
    ▼
Agent 自动获得 MCP 工具
```

### 2.2 CodeGraph MCP 注册

**前置条件**：

```bash
# 安装 CodeGraph（包名以 colbymchenry/codegraph 官方发布为准）
npm install -g codegraph
# 或
npx codegraph init -i    # 在项目根目录初始化索引
```

> **注意**：npm 包名以 [colbymchenry/codegraph](https://github.com/colbymchenry/codegraph) 官方发布为准，当前写法为示意。

**MCP 服务器配置**（在管理后台 Agent 设置中添加）：

```json
{
  "name": "codegraph",
  "transport": "stdio",
  "command": "npx",
  "args": ["-y", "codegraph", "serve"],
  "env": {
    "CODEGRAPH_INDEX_PATH": "${workspaceRoot}/.codegraph"
  },
  "timeout_sec": 30
}
```

**Agent 获得的工具**（工具名以 CodeGraph MCP 官方文档为准）：

| 工具 | 用途 | 典型查询 |
|------|------|---------|
| `codegraph_search` | 符号搜索 | "找到 AuthService 的定义" |
| `codegraph_callers` | 调用者追踪 | "谁调用了 ValidateToken？" |
| `codegraph_callees` | 被调用追踪 | "LoginHandler 调用了哪些函数？" |
| `codegraph_impact` | 影响面分析 | "修改 SessionStore 会影响哪些模块？" |
| `codegraph_node` | 符号详情 | "Show me the signature of BuildTRPCLLMAgent" |
| `codegraph_context` | 模块上下文 | "memory 模块包含哪些文件和符号？" |
| `codegraph_explore` | 模块探索 | "给我看 internal/biz/ 的结构概览" |

### 2.3 Graphify MCP 注册

**前置条件**：

```bash
# 安装 Graphify（含 MCP 扩展）
uv tool install "graphifyy[mcp]"

# 在项目根目录生成知识图谱
cd /path/to/project
graphify scan
# 输出：graphify-out/graph.json, graphify-out/graph.html, graphify-out/GRAPH_REPORT.md
```

**MCP 服务器配置**（在管理后台 Agent 设置中添加）：

```json
{
  "name": "graphify",
  "transport": "stdio",
  "command": "graphify",
  "args": ["serve"],
  "env": {
    "GRAPHIFY_GRAPH_PATH": "${workspaceRoot}/graphify-out/graph.json"
  },
  "timeout_sec": 30
}
```

**Agent 获得的工具**（工具名以 Graphify MCP 官方文档为准）：

| 工具 | 用途 | 典型查询 |
|------|------|---------|
| `graphify_query` | 语义查询子图 | "认证模块涉及哪些文件和概念？" |
| `graphify_neighbors` | 获取邻居节点 | "AuthService 周围有哪些相关实体？" |
| `graphify_path` | 最短路径 | "从 HTTP 入口到数据库层的调用路径是什么？" |
| `graphify_explain` | 概念解释 | "L4 知识图谱的设计意图是什么？" |

### 2.4 两个 MCP 的协同使用场景

| 场景 | 先用 | 后用 | 原因 |
|------|------|------|------|
| 理解一个函数的实现 | CodeGraph `search` | CodeGraph `node` | 精确定位 |
| 理解一个模块的职责 | Graphify `query` | CodeGraph `context` | 先看语义，再看结构 |
| 评估修改影响面 | CodeGraph `impact` | Graphify `neighbors` | 先看直接调用，再看语义关联 |
| 理解设计决策 | Graphify `explain` | — | 只有 Graphify 有 rationale 节点 |
| 追踪跨模块调用链 | CodeGraph `callers` | Graphify `path` | 先看直接调用，再看跨模块路径 |
| 新人理解项目全貌 | Graphify `query` | CodeGraph `explore` | 先看模块划分，再看各模块结构 |

### 2.5 增量更新

**CodeGraph**：索引存储在 `.codegraph/` 目录，文件变更后自动增量更新（亚秒级）。

**Graphify**：需要手动或通过 git hook 触发更新：

```bash
# 手动增量更新
graphify --update

# 通过 git hook 自动更新（Graphify 内置支持）
graphify hook install    # 安装 post-commit hook
# 每次 commit 后自动执行 graphify --update
```

---

## 3. 阶段 2：GraphifyImporter — graph.json → L4 数据填充

### 3.1 为什么需要 GraphifyImporter

MCP Server 提供的是**实时查询**能力，但有两个局限：

1. **MCP 进程不可用时图谱不可查**：Python 进程崩溃、启动失败、超时等场景
2. **L4 记忆注入无法利用图谱数据**：`L4MemoryCue()` 从 `memory_entities` / `memory_relations` 读取，MCP 查询结果不进入这些表

GraphifyImporter 将 graph.json 的数据**持久化到 L4 表**，使图谱知识进入记忆注入管道。

### 3.2 核心设计决策：双通道隔离

**问题**：L4 知识图谱是为"对话知识"设计的（人名、偏好、用户画像），而 Graphify 导入的是"代码结构知识"（函数、类、文件、概念）。两者语义模型不同，强行共用同一套注入管道会导致数据污染和 prompt 质量下降。

**解决方案**：使用 `scope_type` 做物理隔离 + 独立注入函数做逻辑隔离。

| 维度 | 对话知识（已有） | 代码知识（新增） |
|------|-----------------|-----------------|
| **scope_type** | `"agent"` | `"codebase"` |
| **scope_id** | agentID | workspaceID 或项目路径 hash |
| **entity_type** | person / preference / user_profile | code:function / code:class / code:file / code:concept / code:rationale |
| **source_kind** | `"extracted"` / `"auto_memory"` | `"graphify"` |
| **注入函数** | `L4MemoryCue()` | `L4CodeCue()` |
| **注入段名** | `memory.l4` | `memory.l4.code` |
| **开关** | `l4_enabled` | `l4_code_inject`（新增） |
| **预算控制** | `L4PersonaMaxChars` | `L4CodeMaxChars`（新增） |

**隔离效果**：
- `ListEntityRows` 按 `scope_type` 过滤时，两种实体天然分离
- `L4MemoryCue()` 继续只查 `scope_type="agent"` 的对话实体，不受代码实体影响
- `L4CodeCue()` 只查 `scope_type="codebase"` 的代码实体，有独立的格式化和预算控制
- `NeighborhoodJSON` 的 BFS 遍历**必须**在各自的 scope 内进行（见下方设计约束）

**设计约束（必须修复）**：当前 `NeighborhoodJSON` 的 BFS 实现（`internal/data/memory_shim_l4.go`）在遍历 `memory_relations` 时**不过滤 `scope_type`**，会导致跨 scope 边界遍历（代码实体的邻居查到对话实体）。实施阶段 2 时**必须**在 BFS 的 SQL 查询中增加 `AND scope_type = ?` 条件，确保遍历不越界。

### 3.3 数据模型映射

#### graph.json → memory_entities

| graph.json 字段 | memory_entities 字段 | 说明 |
|-----------------|---------------------|------|
| `node.id` | `name` | 实体唯一标识 |
| `"code:" + node.label` | `entity_type` | 加 `code:` 前缀，如 `code:function`、`code:class`、`code:file`、`code:concept`、`code:rationale` |
| `normalizeName(node.id)` | `name_normalized` | 与 AutoMemoryWorker 的 `normalizeName()` 一致：小写 + 去除特殊字符，用于 UNIQUE 约束匹配 |
| — | `scope_type` = `"codebase"` | 与对话实体物理隔离 |
| — | `scope_id` = workspaceID | 项目级作用域 |
| `node.source_file` | `metadata_json.source_file` | 来源文件路径 |
| `node.community` | `metadata_json.module` | Leiden 社区标签 → 模块边界 |
| — | `source_kind` = `"graphify"` | 区分 AutoMemoryWorker 的 `"extracted"` / `"auto_memory"` 来源 |
| — | `confidence` | EXTRACTED→1.0, INFERRED→0.5, AMBIGUOUS→0.2 |
| — | `status` = `"active"` | 与现有 L4 实体状态一致 |
| — | `importance` | EXTRACTED→0.8, INFERRED→0.5, AMBIGUOUS→0.3 |

> **为什么用 `code:` 前缀而非新表**：`memory_entities` 的 UNIQUE 约束是 `(scope_type, scope_id, entity_type, name_normalized)`，`scope_type="codebase"` 已保证物理隔离。`code:` 前缀提供额外的语义可读性，方便调试和人工查询。

#### graph.json → memory_relations

| graph.json 字段 | memory_relations 字段 | 说明 |
|-----------------|---------------------|------|
| `edge.source` → ID 解析 | `source_id` | graph.json 的节点 ID → 查 memory_entities 获取 UUID（见 §3.4 Import 三步执行） |
| `edge.target` → ID 解析 | `target_id` | 同上 |
| `edge.relation` | `relation_type` | 关系类型（calls/imports/implements/explains/depends_on） |
| — | `scope_type` = `"codebase"` | 与对话关系物理隔离 |
| — | `scope_id` = workspaceID | 项目级作用域 |
| `edge.confidence` | `confidence` | EXTRACTED→1.0, INFERRED→0.5, AMBIGUOUS→0.2 |
| `edge.confidence_score` | `metadata_json.confidence_score` | 原始浮点分数 |
| — | `source_kind` = `"graphify"` | 区分来源 |
| `edge.weight` | `weight` | 边权重 |

### 3.4 新增代码设计

**新增文件**：`internal/data/knowledge/graphify_importer.go`

```go
package knowledge

import (
    "context"
    "encoding/json"
    "fmt"
    "os"

    "aranea-agents/internal/biz"
    "aranea-agents/pkg/loggateway"
)

// GraphifyNode 对应 graph.json 中的 node。
type GraphifyNode struct {
    ID             string  `json:"id"`
    Label          string  `json:"label"`
    FileType       string  `json:"file_type"`
    SourceFile     string  `json:"source_file"`
    SourceLocation string  `json:"source_location"`
    Community      int     `json:"community"`
    CapturedAt     string  `json:"captured_at"`
}

// GraphifyEdge 对应 graph.json 中的 edge。
type GraphifyEdge struct {
    Source          string  `json:"source"`
    Target          string  `json:"target"`
    Relation        string  `json:"relation"`
    Confidence      string  `json:"confidence"`       // EXTRACTED / INFERRED / AMBIGUOUS
    ConfidenceScore float64 `json:"confidence_score"`
    SourceFile      string  `json:"source_file"`
    Weight          float64 `json:"weight"`
}

// GraphifyGraph 对应 graph.json 的顶层结构。
type GraphifyGraph struct {
    Nodes []GraphifyNode `json:"nodes"`
    Edges []GraphifyEdge `json:"edges"`
}

// GraphifyImporter 将 graphify 的 graph.json 导入 L4 实体/关系表。
// 导入的实体使用 scope_type="codebase" 隔离，不与对话实体混淆。
type GraphifyImporter struct {
    l4Repo biz.L4GraphWriter  // 仅依赖 L4 写入接口，不依赖 SessionAdminStore
    lg     loggateway.Logger
}

// NewGraphifyImporter 创建导入器。
func NewGraphifyImporter(l4Repo biz.L4GraphWriter, lg loggateway.Logger) *GraphifyImporter {
    return &GraphifyImporter{l4Repo: l4Repo, lg: lg}
}

// Import 全量导入 graph.json 到 L4 表。三步执行：
// 1. BatchUpsertEntities — 插入所有节点实体，获得 UUID
// 2. 构建 ID 映射表 — graphNodeID → memoryEntityUUID
// 3. BatchUpsertRelations — 用映射后的 UUID 构建关系并插入
// 全量替换：先 DeleteCodebaseScope 清除旧数据，再执行上述三步。
func (i *GraphifyImporter) Import(ctx context.Context, graphPath string, workspaceID string) error

// ImportIncremental 增量导入（对比 graph.json 文件 hash，仅文件变更时才重新导入）。
// 内部调用 Import() 全量替换，依赖 UpsertEntity 的 ON CONFLICT DO UPDATE 幂等性。
func (i *GraphifyImporter) ImportIncremental(ctx context.Context, graphPath string, workspaceID string) error

// resolveEntityIDs 在步骤 1 完成后，查询 memory_entities 构建 graphNodeID → entityUUID 映射。
// 查询条件：scope_type="codebase" AND scope_id=workspaceID AND name IN (graphNodeIDs)
func (i *GraphifyImporter) resolveEntityIDs(ctx context.Context, scopeID string, graphNodeIDs []string) (map[string]string, error)

// codeEntityType 将 graphify 的 label 映射为带 code: 前缀的 entity_type。
func codeEntityType(label string) string {
    return "code:" + label
}

// confidenceFromLabel 将 graphify 置信度标签映射为浮点数。
func confidenceFromLabel(label string) float64 {
    switch label {
    case "EXTRACTED":
        return 1.0
    case "INFERRED":
        return 0.5
    case "AMBIGUOUS":
        return 0.2
    default:
        return 0.3
    }
}
```

**关键设计变更**：

1. **依赖 `biz.L4GraphWriter` 而非 `biz.SessionAdminStore`**：GraphifyImporter 只需写入能力，不需要 SessionAdminStore 的全部接口
2. **移除 `fingerprint()` 函数**：增量导入简化为"对比 graph.json 文件 hash → 文件变更时全量替换"。`UpsertEntity` 的 `INSERT ... ON CONFLICT DO UPDATE` 已保证幂等性，无需额外去重
3. **`codeEntityType()` 函数**：统一添加 `code:` 前缀，确保代码实体类型与对话实体类型不冲突

### 3.5 前置修改：biz 层结构体 + data 层 SQL

**问题**：当前 `biz.L4EntityWrite` 和 `biz.L4RelationWrite` 缺少 `SourceKind` 字段，data 层 `UpsertEntity` / `UpsertRelation` 中 `source_kind` 和 `workspace_id` 被硬编码为空字符串。GraphifyImporter 无法写入 `source_kind="graphify"`。

**修改 1**：`biz.L4EntityWrite` 新增字段

```go
// internal/biz/memory_l4.go — L4EntityWrite 新增字段
type L4EntityWrite struct {
    ID             string
    ScopeType      string
    ScopeID        string
    WorkspaceID    string   // 新增：当前硬编码为 ""，需参数化
    UserID         string
    EntityType     string
    Name           string
    NameNormalized string
    Description    string
    Importance     float64
    Confidence     float64
    SourceKind     string   // 新增：当前硬编码为 ""，需参数化
    MetadataJSON   string
}
```

**修改 2**：`biz.L4RelationWrite` 新增字段

```go
// internal/biz/memory_l4.go — L4RelationWrite 新增字段
type L4RelationWrite struct {
    ScopeType    string
    ScopeID      string
    WorkspaceID  string   // 新增：当前硬编码为 ""，需参数化
    SourceID     string
    TargetID     string
    RelationType string
    Weight       float64
    Confidence   float64
    SourceKind   string   // 新增：当前硬编码为 ""，需参数化
}
```

**修改 3**：`UpsertEntity` SQL 参数化

```sql
-- internal/data/memory_l4.go — UpsertEntity 修改
-- 原：workspace_id = '', source_kind = ''
-- 改：workspace_id = ?, source_kind = ?
INSERT INTO memory_entities (
    id, scope_type, scope_id, workspace_id, user_id,
    entity_type, name, name_normalized, aliases_json, description, attributes_json,
    importance, confidence, use_count, source_kind,
    status, metadata_json, created_at, updated_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(scope_type, scope_id, entity_type, name_normalized) DO UPDATE SET
    name = excluded.name, description = excluded.description,
    importance = excluded.importance, confidence = excluded.confidence,
    source_kind = excluded.source_kind,   -- 新增：冲突时也更新 source_kind
    metadata_json = excluded.metadata_json, updated_at = excluded.updated_at
```

**修改 4**：`UpsertRelation` SQL 参数化 + ON CONFLICT 修复

```sql
-- internal/data/memory_l4.go — UpsertRelation 修改
-- 原 ON CONFLICT(source_id, target_id, relation_type) — 3 列，与表 UNIQUE 约束不匹配
-- 改 ON CONFLICT(scope_type, scope_id, source_id, target_id, relation_type) — 5 列，与表定义一致
INSERT INTO memory_relations (
    id, scope_type, scope_id, workspace_id, user_id,
    source_id, target_id, relation_type, weight, confidence,
    source_kind, status, metadata_json, created_at, updated_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(scope_type, scope_id, source_id, target_id, relation_type) DO UPDATE SET
    weight = excluded.weight, confidence = excluded.confidence,
    source_kind = excluded.source_kind,   -- 新增
    metadata_json = excluded.metadata_json, updated_at = excluded.updated_at
```

> **已有 bug 修复**：原 `UpsertRelation` 的 `ON CONFLICT(source_id, target_id, relation_type)` 只用了 3 列，而 `memory_relations` 表的 UNIQUE 约束是 5 列 `(scope_type, scope_id, source_id, target_id, relation_type)`。当不同 scope 下存在相同 `(source_id, target_id, relation_type)` 时会匹配到错误行。此 bug 在引入 `scope_type="codebase"` 后必然触发，必须一并修复。

### 3.6 批量写入接口

现有 L4 层只有单条 `UpsertEntity` / `UpsertRelation`，graph.json 可能有数百到数千节点。需在 `biz.L4GraphWriter` 接口新增批量方法：

```go
// biz.L4GraphWriter 新增方法
BatchUpsertEntities(ctx context.Context, entities []biz.L4EntityWrite) error
BatchUpsertRelations(ctx context.Context, relations []biz.L4RelationWrite) error
// DeleteCodebaseScope 删除指定 codebase scope 的所有实体和关系（级联删除）。
// 先删 memory_relations（WHERE scope_type=scopeType AND scope_id=scopeID），
// 再删 memory_entities（同条件），避免孤儿关系。
DeleteCodebaseScope(ctx context.Context, scopeType, scopeID string) error
```

**实现**（`internal/data/memory_l4.go`）：使用与单条方法相同的 `INSERT ... ON CONFLICT DO UPDATE` 语义（非 `INSERT OR REPLACE`），保留 `use_count`、`created_at` 等未传入字段。单事务提交。`DeleteCodebaseScope` 在单事务中先删关系再删实体，保证数据一致性。

### 3.7 与现有 L4 管道的关系

```
L4 写入路径（已有）：
  AutoMemoryWorker → WriteFromUserText() → UpsertEntity/UpsertRelation
  数据来源：对话消息（人名/偏好，正则提取）
  scope_type = "agent"
  source_kind = "extracted" / "auto_memory"

L4 写入路径（新增）：
  GraphifyImporter → Import() → BatchUpsertEntities/BatchUpsertRelations
  数据来源：代码库静态分析 + 语义提取
  scope_type = "codebase"
  source_kind = "graphify"

两条路径物理隔离（scope_type 不同），不冲突：
  - AutoMemoryWorker 关注"用户说了什么"（对话知识）
  - GraphifyImporter 关注"代码库是什么结构"（代码知识）
  - L4MemoryCue() 只查 scope_type="agent"，不受代码实体影响
  - L4CodeCue() 只查 scope_type="codebase"，不受对话实体影响
```

### 3.8 L4CodeCue() — 代码知识注入函数（新增）

**新增文件**：`internal/agent/l4_code_prompt.go`

> **注意**：以下为示意伪代码，非编译就绪。实际实现需适配 `ListEntityRows` 返回的 `[][]byte` JSON 结构和 `NeighborhoodJSON` 的返回格式。

```go
// L4CodeCue 从 L4 知识图谱中提取代码结构知识，注入 Agent prompt。
// 与 L4MemoryCue() 隔离：只查 scope_type="codebase" 的实体。
// workspaceID 从 buildRuntimeMemoryCue 的 rt.Workspace 传入，保持与 L2/L3 调用方式一致。
func L4CodeCue(ctx context.Context, admin biz.SessionAdminStore, workspaceID string, policy biz.MemoryRuntimePolicy, query string) string {
    if admin == nil || !policy.L4CodeInject || workspaceID == "" {
        return ""
    }

    l4CodeMinConfidence := 0.3
    l4CodeTentativeThresh := 0.6
    maxPaths := policy.L4CodeMaxPaths

    // 1. 查询代码实体（scope_type="codebase"）
    rows, _, err := admin.ListEntityRows(ctx,
        "codebase", workspaceID,  // scope_type + scope_id 隔离
        "", "", "", "active", query,
        int32(maxPaths+4), 0,
    )
    if err != nil || len(rows) == 0 {
        return ""
    }

    // 2. 置信度过滤 + 格式化
    var sb strings.Builder
    sb.WriteString("## L4 code knowledge graph\n")
    var firstEntityID string
    for i, row := range rows {
        var e struct {
            ID         string  `json:"id"`
            EntityType string  `json:"entity_type"`
            Name       string  `json:"name"`
            Desc       string  `json:"description"`
            Confidence float64 `json:"confidence"`
        }
        if err := json.Unmarshal(row, &e); err != nil {
            continue
        }
        if e.Confidence < l4CodeMinConfidence {
            continue
        }
        if i == 0 || firstEntityID == "" {
            firstEntityID = e.ID
        }
        tentative := ""
        if e.Confidence < l4CodeTentativeThresh {
            tentative = " (tentative)"
        }
        fmt.Fprintf(&sb, "- [%s] %s: %s%s\n", e.EntityType, e.Name, e.Desc, tentative)
    }

    // 3. 邻居子图注入（可选，独立开关）
    if policy.L4CodeInjectNeighbors && firstEntityID != "" {
        hops := int32(policy.L4GraphMaxHops)
        maxN := int32(policy.L4CodeMaxPaths)
        neighbors := admin.NeighborhoodJSON(ctx, firstEntityID, hops, maxN, "")
        if len(neighbors) > 0 {
            neighborStr := string(neighbors)
            maxChars := policy.L4CodeMaxChars
            if len(neighborStr) > maxChars {
                neighborStr = neighborStr[:maxChars] + "\n... (truncated)"
            }
            sb.WriteString("\n### Code neighborhood\n")
            sb.WriteString(neighborStr)
        }
    }

    return sb.String()
}
```

**关键设计说明**：

1. **`workspaceID` 来源**：`L4CodeCue` 不再从 `ag.Settings.Workspace` 自行获取 workspaceID，而是由 `buildRuntimeMemoryCue()` 通过 `rt.Workspace`（`MemoryRuntimeContext.Workspace`）传入。`rt.Workspace` 优先从 session state 获取，回退到 `ag.Settings.Workspace`，与 L2/L3 的调用方式一致
2. **`firstEntityID` 追踪**：从遍历结果中提取第一个通过置信度过滤的实体 ID，用于邻居子图查询
3. **邻居子图截断**：使用 `L4CodeMaxChars` 控制邻居 JSON 的字符数上限

**新增配置项**（`agent_runtime_settings` 表 ALTER TABLE）：

| 列名 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `l4_code_inject` | INTEGER | 0 | 代码知识注入开关 |
| `l4_code_inject_neighbors` | INTEGER | 0 | 代码邻居子图注入开关 |
| `l4_code_max_paths` | INTEGER | 20 | 代码实体最大返回数 |
| `l4_code_max_chars` | INTEGER | 4000 | 代码知识段最大字符数 |

### 3.9 对 L4MemoryCue() 的影响

**无需修改**。`L4MemoryCue()` 的现有实现调用 `ListEntityRows(ctx, "agent", ag.ID, ...)`，硬编码 `scope_type="agent"`，不会查到 `scope_type="codebase"` 的代码实体。

**对 `buildRuntimeMemoryCue()` 的影响**：需在 `internal/agent/memory_inject.go` 的 `buildRuntimeMemoryCue()` 函数中新增 `L4CodeCue()` 调用，将代码知识追加到 `recallParts`：

```go
// internal/agent/memory_inject.go — buildRuntimeMemoryCue() 新增
// 在现有的 L4 注入之后（第 153-157 行之后）
if policy.L4CodeInject {
    if l4code := L4CodeCue(ctx, deps.MemoryAdmin, rt.Workspace, policy, keyword); l4code != "" {
        recallParts = append(recallParts, l4code)
    }
}
```

**对 `newMemoryInjectBeforeHook()` 的影响**：需在 `hasDep` 判断中增加 `L4CodeInject` 条件：

```go
// internal/agent/memory_inject.go — newMemoryInjectBeforeHook() 修改
hasDep = hasDep || (policy.L4CodeInject && deps.MemoryAdmin != nil)
```

### 3.10 触发方式

| 方式 | 实现 | 优先级 | 说明 |
|------|------|--------|------|
| **手动** | 管理后台"重建代码图谱"按钮 → API → `GraphifyImporter.Import()` | P0 必须 | 运维可控 |
| **半自动** | git post-commit hook → `graphify --update` → 触发 `ImportIncremental()` | P1 推荐 | 代码变更自动同步 |
| **定时** | cron job（低频，如每天一次） | P2 可选 | 兜底保障 |

### 3.11 Wire 注入

在 `internal/data/data.go` 的 ProviderSet 中注册：

```go
// internal/data/data.go — ProviderSet 新增
wire.NewSet(knowledge.NewGraphifyImporter)
```

在 `internal/biz/` 的 Usecase 中按需调用（如 `MemoryAdminUsecase` 新增 `RebuildCodeGraph` 方法）。

### 3.12 预估代码量

| 文件 | 行数 | 说明 |
|------|------|------|
| `internal/biz/memory_l4.go`（修改） | ~30 行 | L4EntityWrite/L4RelationWrite 新增 SourceKind/WorkspaceID 字段 + L4GraphWriter 批量方法签名 |
| `internal/data/memory_l4.go`（修改） | ~80 行 | SourceKind/WorkspaceID 参数化 + UpsertRelation ON CONFLICT 修复 + 批量方法实现 |
| `internal/data/knowledge/graphify_importer.go` | ~380 行 | 导入器核心逻辑（含三步执行 + ID 解析 + 批量写入） |
| `internal/data/knowledge/graphify_importer_test.go` | ~200 行 | 单元测试 |
| `internal/agent/l4_code_prompt.go` | ~70 行 | L4CodeCue() 代码知识注入 |
| `internal/data/memory_shim_l4.go`（修改） | ~15 行 | NeighborhoodJSON BFS 增加 scope_type 过滤 |
| `internal/biz/memory_admin_usecase.go`（修改） | ~60 行 | 新增 RebuildCodeGraph 方法 |
| `internal/agent/memory_inject.go`（修改） | ~15 行 | buildRuntimeMemoryCue 新增 L4CodeCue + GraphifyReportCue 调用 + hasDep 条件 |
| `internal/agent/graphify_report_prompt.go`（新增） | ~25 行 | GraphifyReportCue 函数 |
| `internal/data/sql/memory_chain.sql`（修改） | ~10 行 | ALTER TABLE 新增配置列 |
| **合计** | **~885 行** | |

---

## 4. 阶段 3：L0 图谱摘要注入

### 4.1 设计

Graphify 的 `GRAPH_REPORT.md` 包含项目结构摘要（模块划分、核心实体、关键设计决策），将其注入 Agent prompt，让 Agent 在每次对话开始时就"知道"项目全貌。

### 4.2 注入方式：复用 BeforeModel Hook 通道

**不新建独立的注入通道**，而是复用已有的 `newMemoryInjectBeforeHook` → `buildRuntimeMemoryCue` 注入管道。

实际的记忆注入流程（`internal/agent/memory_inject.go`）：

```
newMemoryInjectBeforeHook()
    │  注册 BeforeModel callback（优先级 5，LayerDynamic）
    ▼
buildRuntimeMemoryCue()
    │  L1MemoryCue() → result.L1Cue
    │  L2MemoryCue() / L3MemoryCue() / CompositeMemoryCue() → recallParts
    │  L4MemoryCue() → recallParts
    │  L4CodeCue() → recallParts（阶段 2 新增）
    │  GraphifyReportCue() → recallParts（本阶段新增）
    ▼
result.JoinCues()
    │  L1Cue + "\n\n" + RecallCue
    ▼
trpcmodel.NewSystemMessage(cue) → 前置注入到 messages
```

**注入位置**：`GraphifyReportCue()` 的输出追加到 `recallParts`，作为 `RecallCue` 的一部分注入。

### 4.3 实现

**新增文件**：`internal/agent/graphify_report_prompt.go`

```go
// GraphifyReportCue 读取 GRAPH_REPORT.md 并返回摘要文本，注入 Agent prompt。
// 文件不存在时返回空字符串，零影响。
// workspacePath 为文件系统路径（非 workspace ID），由调用方从 agent 配置或环境获取。
func GraphifyReportCue(workspacePath string) string {
    if workspacePath == "" {
        return ""
    }
    reportPath := filepath.Join(workspacePath, "graphify-out", "GRAPH_REPORT.md")
    data, err := os.ReadFile(reportPath)
    if err != nil {
        return "" // 文件不存在，静默跳过
    }
    content := strings.TrimSpace(string(data))
    if content == "" {
        return ""
    }
    // 截断到预算上限（避免超大报告撑爆 prompt）
    const maxChars = 8000
    if len(content) > maxChars {
        content = content[:maxChars] + "\n... (truncated)"
    }
    return fmt.Sprintf("## Project Knowledge Graph Summary\n%s", content)
}
```

在 `buildRuntimeMemoryCue()` 中调用：

```go
// internal/agent/memory_inject.go — buildRuntimeMemoryCue() 新增
// 在 L4CodeCue 之后追加
// workspacePath 需从 agent 配置或环境变量获取，而非 rt.Workspace（那是 ID 不是路径）
// 降级方案：若无法获取路径，跳过 GRAPH_REPORT.md 注入
if reportCue := GraphifyReportCue(deps.WorkspacePath); reportCue != "" {
    recallParts = append(recallParts, reportCue)
}
```

> **`rt.Workspace` 语义说明**：`MemoryRuntimeContext.Workspace` 的值来源是 `sessionStateString(inv.Session.State, "workspace")`，回退到 `ag.Settings.Workspace`。此值在当前系统中为 workspace 标识符（非文件系统路径），因此：
> - `L4CodeCue` 将其用作 `scope_id`（数据库查询键）— 语义匹配
> - `GraphifyReportCue` 将其用作文件路径前缀 — **语义不匹配**，需额外处理
>
> **修复方案**：`GraphifyReportCue` 不使用 `rt.Workspace`，改为从 `ag.Settings` 的扩展配置中读取 workspace 路径，或使用约定的相对路径（如当前工作目录下的 `graphify-out/`）。具体实现待确认 `ag.Settings` 中是否有 workspace 路径字段，若无则降级为仅检查当前工作目录。

**优势**：
1. **复用已有管道**：与其他 memory cue 使用相同的注入路径，由 `newMemoryInjectBeforeHook` 统一管理
2. **可追踪**：注入内容通过 `memoryInjectCueContent()` 包装后作为 system message 前置，可在 L0 快照中溯源
3. **文件不存在时零影响**：`GraphifyReportCue` 返回空字符串，不改变任何行为
4. **无需额外开关**：GRAPH_REPORT.md 的存在性本身就是开关——有文件就注入，没有就不注入

### 4.4 增量更新

```
git commit
    │
    ▼
post-commit hook: graphify --update
    │
    ▼
graphify-out/GRAPH_REPORT.md 变更
    │
    ▼
下次 Agent 请求时 GraphifyReportCue() 读取新内容
    │
    ▼
buildRuntimeMemoryCue() 自动使用新内容（每次请求实时读文件）
```

### 4.5 预估代码量

| 文件 | 行数 | 说明 |
|------|------|------|
| `internal/agent/graphify_report_prompt.go`（新增） | ~25 行 | GraphifyReportCue 函数（已计入 §3.12） |
| `internal/agent/memory_inject.go`（修改） | ~4 行 | buildRuntimeMemoryCue 新增 GraphifyReportCue 调用（已计入 §3.12） |
| **合计** | **~0 行**（已合并到 Phase 3） | |

---

## 5. 与 Phase 路线图的融合

| Phase | 原有核心任务 | +Graphify/CodeGraph 融合任务 | 新增代码量 |
|-------|------------|---------------------------|-----------|
| **Phase 1** | L0 简化 + Prompt Caching + Embedding 单写 | MCP 配置注册（管理后台操作） | **0 行** |
| **Phase 2** | 三级压缩 + L1 选择性注入 + 结构化 Episode | 无额外任务 | **0 行** |
| **Phase 3** | 单跳巩固 + L1/L3/L4 职责厘清 | + GraphifyImporter（graph.json → L4 codebase scope）+ NeighborhoodJSON scope 修复 + L4EntityWrite/L4RelationWrite SourceKind 参数化 + UpsertRelation ON CONFLICT 修复 + GraphifyReportCue | **~885 行** |
| **Phase 4** | 多代理加密 + L2 Episode 分叉 + Embedding 增量重建 | 无额外任务 | **0 行** |

**总新增代码量：~885 行**，集中在 Phase 3。

---

## 6. 风险与缓解

| 风险 | 影响 | 缓解措施 |
|------|------|---------|
| Graphify 是 Python 进程，MCP 启动失败 | Agent 丢失图谱查询能力 | MCP 重连机制（已有 `WithSessionReconnect`）；Agent 不依赖 MCP 完成核心任务 |
| graph.json 过大导致 L4 表膨胀 | L4CodeCue 注入 token 超预算 | `scope_type="codebase"` 隔离不影响对话知识；L4CodeCue 有独立的 `L4CodeMaxChars` 和 `L4CodeMaxPaths` 上限控制 |
| Graphify INFERRED 边不准确 | Agent 基于错误关系推理 | confidence < 0.6 标记 tentative；confidence < 0.3 过滤不注入 |
| CodeGraph 与 Graphify 查询结果冲突 | Agent 困惑 | 两者查询维度不同（结构 vs 语义），冲突概率低；如有冲突以 CodeGraph 结构数据为准 |
| Graphify 项目较新（2026.4 发布） | API 不稳定 | MCP 接口解耦；GraphifyImporter 只消费 graph.json 文件格式，不依赖 API |
| Python 运行时依赖 | 部署复杂度增加 | Graphify 仅在 `scan`/`serve` 时需要 Python；Importer 只读 JSON 文件，无需 Python |
| 代码实体与对话实体混淆 | prompt 质量下降 | `scope_type="codebase"` 物理隔离 + `L4CodeCue()` 逻辑隔离 + `code:` 前缀语义区分 |
| `rt.Workspace` 为空 | L4CodeCue 无法获取 workspaceID | 降级处理：Workspace 为空时不注入代码知识，仅依赖 MCP 查询 |
| UpsertRelation ON CONFLICT 3 列 vs 表 UNIQUE 5 列 | 跨 scope 写入时关系数据覆盖 | 已在 §3.5 修改 4 中修复：ON CONFLICT 改为 5 列匹配 |
| AutoMemoryWorker 未传 SourceKind | 已有对话实体 source_kind 为空 | 向后兼容：空字符串等价于 `"extracted"`（表默认值）；后续迭代补传 |
| `rt.Workspace` 是 ID 非路径 | GraphifyReportCue 无法定位文件 | `GraphifyReportCue` 改用 `deps.WorkspacePath`（独立字段），降级为检查当前工作目录 |

---

## 7. 验证计划

### 7.1 阶段 1 验证

```bash
# 1. 安装 CodeGraph 和 Graphify（包名以官方发布为准）
npm install -g codegraph
uv tool install "graphifyy[mcp]"

# 2. 初始化索引
cd f:/project/aranea-agents
npx codegraph init -i
graphify scan

# 3. 在管理后台为 Agent 注册两个 MCP 服务器

# 4. 创建对话，验证 Agent 能调用 MCP 工具
# 预期：Agent 可使用 codegraph_search、graphify_query 等工具
```

### 7.2 阶段 2 验证

```bash
# 1. 实现 GraphifyImporter + L4CodeCue + NeighborhoodJSON scope 修复 + SourceKind 参数化 + UpsertRelation ON CONFLICT 修复

# 2. 运行导入
curl -X POST /api/admin/knowledge/graphify/import \
  -d '{"graph_path": "graphify-out/graph.json", "workspace_id": "<workspace-id>"}'

# 3. 验证 L4 表数据隔离
# 预期：memory_entities 中出现 scope_type="codebase", source_kind="graphify" 的实体
# 预期：memory_entities 中 scope_type="agent" 的对话实体不受影响
# 预期：entity_type 带 "code:" 前缀（如 code:function, code:class）
# 预期：source_kind 列值为 "graphify"（非空字符串）

# 4. 验证 L4MemoryCue 不受影响
# 预期：L4MemoryCue() 只返回 scope_type="agent" 的对话实体，不包含代码实体

# 5. 验证 L4CodeCue 注入
# 预期：L4CodeCue() 返回 scope_type="codebase" 的代码实体
# 预期：Agent 对话中 RecallCue 包含 "L4 code knowledge graph" 段

# 6. 验证 NeighborhoodJSON scope 隔离
# 预期：从代码实体出发的 BFS 只返回 scope_type="codebase" 的邻居
# 预期：不会跨域查到 scope_type="agent" 的对话实体

# 7. 验证 UpsertRelation ON CONFLICT 修复
# 预期：同一 (source_id, target_id, relation_type) 在不同 scope 下可共存
# 预期：ON CONFLICT 更新匹配 5 列 (scope_type, scope_id, source_id, target_id, relation_type)

# 8. 验证批量写入性能
# 预期：500 节点 graph.json 导入 < 5 秒

# 9. 验证 rt.Workspace 降级
# 预期：Workspace 为空时 L4CodeCue() 返回空字符串，不报错
```

### 7.3 阶段 3 验证

```bash
# 1. 验证 GRAPH_REPORT.md 存在
ls graphify-out/GRAPH_REPORT.md

# 2. 验证 BeforeModel hook 注入
# 预期：Agent 对话中 RecallCue 包含 "Project Knowledge Graph Summary" 段

# 3. 验证文件不存在时零影响
rm graphify-out/GRAPH_REPORT.md
# 预期：Agent 正常运行，无报错，RecallCue 中无图谱摘要段
```
