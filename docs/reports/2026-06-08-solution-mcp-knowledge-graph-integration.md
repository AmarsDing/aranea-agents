# CodeGraph MCP + Graphify MCP 融入 Aranea-Agents 方案

> 版本：v1.0 | 日期：2026-06-08 | 状态：草案

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
  │   ├── AutoMemoryWorker      → 对话知识（人名/偏好，正则提取）
  │   └── GraphifyImporter      → 代码知识（graph.json → memory_entities/relations）
  │
  └── L0 静态前缀（Phase 4 后）
      ├── SOUL.md / AGENTS.md   → 角色与规则
      └── GRAPH_REPORT.md       → 项目图谱摘要
```

**关键原则**：CodeGraph 和 Graphify 作为 MCP Server 接入，**不修改项目核心代码**，仅通过数据库配置注册。GraphifyImporter 是唯一需要新增的内部代码，用于将 graph.json 数据持久化到 L4。

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
# 安装 CodeGraph
npm install -g @anthropic/codegraph
# 或
npx @anthropic/codegraph init -i    # 在项目根目录初始化索引
```

**MCP 服务器配置**（在管理后台 Agent 设置中添加）：

```json
{
  "name": "codegraph",
  "transport": "stdio",
  "command": "npx",
  "args": ["-y", "@anthropic/codegraph", "serve"],
  "env": {
    "CODEGRAPH_INDEX_PATH": "${workspaceRoot}/.codegraph"
  },
  "timeout_sec": 30
}
```

**Agent 获得的工具**：

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

**Agent 获得的工具**：

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

### 3.2 数据模型映射

#### graph.json → memory_entities

| graph.json 字段 | memory_entities 字段 | 说明 |
|-----------------|---------------------|------|
| `node.id` | `name`（去重后） | 实体唯一标识 |
| `node.label` | `entity_type` | 实体类型（function/class/file/concept/rationale） |
| `node.source_file` | `metadata.source_file` | 来源文件路径 |
| `node.community` | `metadata.module` | Leiden 社区标签 → 模块边界 |
| — | `source` = `"graphify"` | 区分 AutoMemoryWorker 的 `"conversation"` 来源 |
| — | `confidence` = 1.0 | Graphify EXTRACTED 节点默认高置信度 |
| — | `status` = `"active"` | 与现有 L4 实体状态一致 |

#### graph.json → memory_relations

| graph.json 字段 | memory_relations 字段 | 说明 |
|-----------------|---------------------|------|
| `edge.source` | `source_entity_id` | 关系起点 |
| `edge.target` | `target_entity_id` | 关系终点 |
| `edge.relation` | `relation_type` | 关系类型（calls/imports/implements/explains/depends_on） |
| `edge.confidence` | `confidence` | EXTRACTED→1.0, INFERRED→0.5, AMBIGUOUS→0.2 |
| `edge.confidence_score` | `metadata.confidence_score` | 原始浮点分数 |
| — | `source` = `"graphify"` | 区分来源 |

### 3.3 新增代码设计

**唯一新增文件**：`internal/data/knowledge/graphify_importer.go`

```go
package knowledge

import (
    "context"
    "crypto/sha256"
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
    Source           string  `json:"source"`
    Target           string  `json:"target"`
    Relation         string  `json:"relation"`
    Confidence       string  `json:"confidence"`       // EXTRACTED / INFERRED / AMBIGUOUS
    ConfidenceScore  float64 `json:"confidence_score"`
    SourceFile       string  `json:"source_file"`
    Weight           float64 `json:"weight"`
}

// GraphifyGraph 对应 graph.json 的顶层结构。
type GraphifyGraph struct {
    Nodes []GraphifyNode `json:"nodes"`
    Edges []GraphifyEdge `json:"edges"`
}

// GraphifyImporter 将 graphify 的 graph.json 导入 L4 实体/关系表。
type GraphifyImporter struct {
    admin biz.SessionAdminStore
    lg    loggateway.Logger
}

// NewGraphifyImporter 创建导入器。
func NewGraphifyImporter(admin biz.SessionAdminStore, lg loggateway.Logger) *GraphifyImporter {
    return &GraphifyImporter{admin: admin, lg: lg}
}

// Import 全量导入 graph.json 到 L4 表。
func (i *GraphifyImporter) Import(ctx context.Context, graphPath string, scopeType, scopeID string) error

// ImportIncremental 增量导入（对比 fingerprint，仅处理变更）。
func (i *GraphifyImporter) ImportIncremental(ctx context.Context, graphPath string, scopeType, scopeID string) error

// fingerprint 生成节点/边的唯一指纹，用于增量去重。
func fingerprint(parts ...string) string {
    h := sha256.New()
    for _, p := range parts {
        h.Write([]byte(p))
    }
    return fmt.Sprintf("%x", h.Sum(nil))[:16]
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

### 3.4 与现有 L4 管道的关系

```
L4 写入路径（已有）：
  AutoMemoryWorker → Extract() → memory_entities/relations
  数据来源：对话消息（人名/偏好，正则提取）
  source 字段："conversation"

L4 写入路径（新增）：
  GraphifyImporter → 解析 graph.json → memory_entities/relations
  数据来源：代码库静态分析 + 语义提取
  source 字段："graphify"

两条路径互补，不冲突：
  - AutoMemoryWorker 关注"用户说了什么"（对话知识）
  - GraphifyImporter 关注"代码库是什么结构"（代码知识）
  - L4MemoryCue() 已实现按 source 过滤查询，无需修改
```

### 3.5 对 L4MemoryCue() 的影响

**无需修改**。`L4MemoryCue()` 的现有实现（[l4_prompt.go](file:///f:/project/aranea-agents/internal/agent/l4_prompt.go)）已支持：

1. **按关键词过滤实体**：`admin.ListEntityRows(..., query, ...)` — query 参数可匹配 graphify 导入的实体名
2. **置信度过滤**：`confidence < l4CueMinConfidence(0.3)` 被过滤 — graphify 的 AMBIGUOUS 边（0.2）会被过滤，INFERRED（0.5）标记为 tentative，EXTRACTED（1.0）正常注入
3. **邻居关系注入**：`admin.NeighborhoodJSON(...)` — graphify 导入的关系会自动出现在邻居查询结果中
4. **token 预算控制**：`policy.L4PersonaMaxChars` 和 `policy.L4GraphMaxNeighbors` 已有上限控制

**效果**：GraphifyImporter 写入后，L4 注入内容从"几乎为空"变为"包含代码结构图谱"，Agent 在每次对话中自动获得项目结构知识。

### 3.6 触发方式

| 方式 | 实现 | 优先级 | 说明 |
|------|------|--------|------|
| **手动** | 管理后台"重建知识图谱"按钮 → API → `GraphifyImporter.Import()` | P0 必须 | 运维可控 |
| **半自动** | git post-commit hook → `graphify --update` → 触发 `ImportIncremental()` | P1 推荐 | 代码变更自动同步 |
| **定时** | cron job（低频，如每天一次） | P2 可选 | 兜底保障 |

### 3.7 Wire 注入

在 `internal/data/` 的 ProviderSet 中注册：

```go
// internal/data/knowledge/graphify_importer.go 的 Wire 注入
wire.NewSet(NewGraphifyImporter)
```

在 `internal/biz/` 的 Usecase 中按需调用（如 `MemoryAdminUsecase` 新增 `RebuildGraphifyGraph` 方法）。

### 3.8 预估代码量

| 文件 | 行数 | 说明 |
|------|------|------|
| `internal/data/knowledge/graphify_importer.go` | ~400 行 | 导入器核心逻辑 |
| `internal/data/knowledge/graphify_importer_test.go` | ~200 行 | 单元测试 |
| `internal/biz/memory_admin_usecase.go`（修改） | ~30 行 | 新增 RebuildGraphifyGraph 方法 |
| **合计** | **~630 行** | |

---

## 4. 阶段 3：L0 图谱摘要注入

### 4.1 设计

Graphify 的 `GRAPH_REPORT.md` 包含项目结构摘要（模块划分、核心实体、关键设计决策），将其注入 Layer 1 静态前缀，让 Agent 在每次对话开始时就"知道"项目全貌。

### 4.2 注入位置

在三层前缀架构中，`GRAPH_REPORT.md` 归入 **Layer 1（静态前缀）**：

```
Layer 1（静态前缀，缓存命中率最高）：
  - SOUL.md / AGENTS.md 内容          ← 已有
  - Skill 文件内容                     ← 已有
  - GRAPH_REPORT.md 摘要              ← 新增（~2K-5K token）
```

### 4.3 实现

在 [l4_prompt.go](file:///f:/project/aranea-agents/internal/agent/l4_prompt.go) 或新建 `graphify_prompt.go` 中：

```go
// GraphifyReportCue 读取 GRAPH_REPORT.md 并返回摘要文本。
// 文件不存在时返回空字符串，零影响。
func GraphifyReportCue(workspacePath string) string {
    reportPath := filepath.Join(workspacePath, "graphify-out", "GRAPH_REPORT.md")
    data, err := os.ReadFile(reportPath)
    if err != nil {
        return ""
    }
    content := strings.TrimSpace(string(data))
    if content == "" {
        return ""
    }
    // 截断到预算上限（避免超大报告撑爆前缀）
    const maxChars = 8000
    if len(content) > maxChars {
        content = content[:maxChars] + "\n... (truncated)"
    }
    return fmt.Sprintf("\n## Project Knowledge Graph Summary\n%s", content)
}
```

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
Layer 1 缓存失效（文件内容 hash 变化）
    │
    ▼
下次 Agent 请求自动刷新
```

### 4.5 预估代码量

| 文件 | 行数 | 说明 |
|------|------|------|
| `internal/agent/graphify_prompt.go` | ~40 行 | 读取 + 截断 + 格式化 |
| **合计** | **~40 行** | |

---

## 5. 与 Phase 路线图的融合

| Phase | 原有核心任务 | +Graphify/CodeGraph 融合任务 | 新增代码量 |
|-------|------------|---------------------------|-----------|
| **Phase 1** | L0 简化 + Prompt Caching + Embedding 单写 | MCP 配置注册（管理后台操作） | **0 行** |
| **Phase 2** | 三级压缩 + L1 选择性注入 + 结构化 Episode | 无额外任务 | **0 行** |
| **Phase 3** | 单跳巩固 + L1/L3/L4 职责厘清 | + GraphifyImporter（graph.json → L4） | **~630 行** |
| **Phase 4** | 多代理加密 + L2 Episode 分叉 + Embedding 增量重建 | + GRAPH_REPORT.md → Layer 1 注入 | **~40 行** |

**总新增代码量：~670 行**，分布在 Phase 3 和 Phase 4。

---

## 6. 风险与缓解

| 风险 | 影响 | 缓解措施 |
|------|------|---------|
| Graphify 是 Python 进程，MCP 启动失败 | Agent 丢失图谱查询能力 | MCP 重连机制（已有 `WithSessionReconnect`）；Agent 不依赖 MCP 完成核心任务 |
| graph.json 过大导致 L4 表膨胀 | L4MemoryCue 注入 token 超预算 | 按社区分片导入；L4MemoryCue 已有 `maxPaths` 和 `maxChars` 上限控制 |
| Graphify INFERRED 边不准确 | Agent 基于错误关系推理 | confidence < 0.6 标记 tentative；confidence < 0.3 过滤不注入 |
| CodeGraph 与 Graphify 查询结果冲突 | Agent 困惑 | 两者查询维度不同（结构 vs 语义），冲突概率低；如有冲突以 CodeGraph 结构数据为准 |
| Graphify 项目较新（2026.4 发布） | API 不稳定 | MCP 接口解耦；GraphifyImporter 只消费 graph.json 文件格式，不依赖 API |
| Python 运行时依赖 | 部署复杂度增加 | Graphify 仅在 `scan`/`serve` 时需要 Python；Importer 只读 JSON 文件，无需 Python |

---

## 7. 验证计划

### 7.1 阶段 1 验证

```bash
# 1. 安装 CodeGraph 和 Graphify
npm install -g @anthropic/codegraph
uv tool install "graphifyy[mcp]"

# 2. 初始化索引
cd f:/project/aranea-agents
npx @anthropic/codegraph init -i
graphify scan

# 3. 在管理后台为 Agent 注册两个 MCP 服务器

# 4. 创建对话，验证 Agent 能调用 MCP 工具
# 预期：Agent 可使用 codegraph_search、graphify_query 等工具
```

### 7.2 阶段 2 验证

```bash
# 1. 实现 GraphifyImporter

# 2. 运行导入
curl -X POST /api/admin/knowledge/graphify/import \
  -d '{"graph_path": "graphify-out/graph.json", "scope_type": "agent", "scope_id": "<agent-id>"}'

# 3. 验证 L4 表数据
# 预期：memory_entities 中出现 source="graphify" 的实体
# 预期：memory_relations 中出现 source="graphify" 的关系

# 4. 验证 L4MemoryCue 注入
# 预期：Agent 对话中 L4 graph context 包含代码结构实体
```

### 7.3 阶段 3 验证

```bash
# 1. 验证 GRAPH_REPORT.md 存在
ls graphify-out/GRAPH_REPORT.md

# 2. 验证 Layer 1 注入
# 预期：Agent 系统提示词中出现 "Project Knowledge Graph Summary" 段落
```
