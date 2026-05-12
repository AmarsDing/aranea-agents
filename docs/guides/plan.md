# Aranea Agents — trpc-agent-go 功能对齐与优化清单

> 本文档对比项目现有实现与 `pkg/trpc-agent-go` 框架能力，梳理出需要完善/优化的功能模块。
> 每个模块均给出：现状分析、trpc 框架参照、优化目标、具体步骤、涉及文件、验收标准。
> AI 可按照本清单自主执行优化。

---

## 总览：功能对齐矩阵

| 功能模块 | trpc-agent-go 框架能力 | 项目现有实现 | 对齐状态 | 优先级 |
|---------|----------------------|-------------|---------|--------|
| **M1: Skill 运行时** | `skill.Repository` + `tool/skill/{load,run,list_docs,select_docs}` + 渐进披露 + 工作区执行 | ADK `skilltoolset` + 自定义 `skillruntime` + ZIP 导入 + 文件管理 | ✅ 已对齐 | P0 |
| **M2: Agent 构建** | `llmagent.New(cfg)` + 占位符变量 + IncludeContents + Planner | ADK `BuildLLMAgent` + `BuilderDeps` | ✅ 已对齐 | P0 |
| **M3: Team 编排** | `team.NewCoordinator` / `team.NewSwarm` + AgentTool + TransferTool | 自定义 `team.Runner` + Coordinator/Swarm | ✅ 已对齐 | P1 |
| **M4: Graph 工作流** | `graph.StateGraph` + 节点/边/条件路由 + HITL + 检查点 + 时间旅行 | 无 | ✅ 已对齐 | P1 |
| **M5: Session 管理** | `session.Service` + 多后端(SQLite/Redis/PG/MySQL) + 摘要压缩 | ADK Session + 自定义 `sessionmemory.Store` + SQLite | ✅ 已对齐 | P1 |
| **M6: Memory 记忆** | `memory.Service` + 自动提取(Auto) + 工具驱动(Agentic) + 多后端 | ADK Memory + SQLite session entities + pgvector | ✅ 已对齐 | P2 |
| **M7: Tool 工具体系** | `tool.Tool`/`tool.ToolSet` + FunctionTool + MCP + 流式工具 + 重试 + 过滤 | ADK 工具注册 + 自定义 workspace/shell/web 工具 + MCP 管理 | ✅ 已对齐 | P1 |
| **M8: MCP 集成** | `tool/mcp.ToolSet` + `tool/mcpbroker.Broker` + STDIO/SSE/StreamableHTTP | `biz.MCPServerUsecase` + 数据库管理 MCP 配置 | ⚠️ 部分实现 | P2 |
| **M9: Model 模型层** | `model.Model` 接口 + OpenAI/Gemini/Anthropic/Ollama/Bedrock + Failover/Hedge | `provider.openai` 适配 + `biz.LlmProviderModelUsecase` | ✅ 已对齐 | P2 |
| **M10: Plugin 插件** | `plugin.Plugin` + Runner 级生命周期钩子(BeforeModel/AfterTool/OnEvent) | ADK `DefaultRunnerPlugins` + 自定义 `agent/adk_plugins.go` | ⚠️ 部分实现 | P2 |
| **M11: Planner 规划** | `planner.BuiltinPlanner` / `planner.ReActPlanner` + 思考链 | 无 | ✅ 已对齐 | P2 |
| **M12: Artifact 制品** | `artifact.Service` + S3/COS 后端 + 版本管理 | ADK Artifact + `agent/adk_artifact.go` | ⚠️ 部分实现 | P2 |
| **M13: Knowledge 知识库** | `knowledge.Service` + OCR + Query + RAG | 无 | ❌ 未实现 | P3 |
| **M14: CodeExecutor** | `codeexecutor.CodeExecutor` + Local/E2B + WorkspaceRegistry | `tools/shell_exec` + `tools/workspace/sandbox` | ⚠️ 部分实现 | P2 |
| **M15: A2A 协议** | `a2a.A2AServer` + `a2a.A2AAgent` + AgentCard 自动生成 | 无 | ❌ 未实现 | P3 |
| **M16: Gateway 网关** | `gateway.Gateway` + HTTP webhook + 会话并发控制 + status/cancel | Kratos HTTP/gRPC server + SSE | ⚠️ 部分实现 | P3 |
| **M17: Evaluation 评估** | `evaluation.AgentEvaluator` + 评估集 + 指标 + LLM-as-Judge | 无 | ❌ 未实现 | P3 |
| **M18: Event 事件** | `event.Event` + 流式 + 标签 + 自定义事件 | ADK 事件 + SSE 投影 | ⚠️ 部分实现 | P2 |

---

## M1: Skill 运行时 — 向 trpc 框架对齐 [P0]

### 现状分析

项目当前使用 **ADK（Google Agent Development Kit）** 的 `skilltoolset` 包来挂载 Skill：

```
internal/tools/skillruntime/toolset.go → skilltoolset.New(ctx, skilltoolset.Config{Source: skill.NewFileSystemSource(fs)})
```

核心问题：
1. **工具集不完整**：ADK skilltoolset 只提供 `skill_load`/`list_skills`/`load_skill_resource`，缺少 trpc 框架的 `skill_run`（工作区执行）、`skill_list_docs`、`skill_select_docs`
2. **无渐进披露**：trpc 框架实现了三层信息模型（概览→正文→文档），项目当前直接注入全部内容
3. **无工作区执行**：trpc 的 `skill_run` 在隔离工作区中执行脚本并收集输出文件，项目无此能力
4. **无 SkillLoadMode**：trpc 支持 `turn`/`session` 两种加载模式控制状态生命周期
5. **无 Prompt Cache 优化**：trpc 支持 `WithSkillsLoadedContentInToolResults(true)` 稳定 system prompt
6. **无 SkillFilter**：trpc 支持按请求过滤可见技能
7. **ZIP 导入与 trpc 无关**：项目的 `skill/importer` 是产品层功能，与框架 skill 无冲突

### trpc 框架参照

```
pkg/trpc-agent-go/
├── skill/
│   ├── repository.go          # FSRepository: 扫描 SKILL.md 目录，提供 Summaries/Get/Path
│   ├── context_repository.go  # ContextAwareRepository: 按请求过滤
│   ├── state_keys.go          # 会话状态键定义
│   └── state_order.go         # 加载顺序管理
├── tool/skill/
│   ├── load.go                # skill_load 工具：注入正文+文档到会话状态
│   ├── run.go                 # skill_run 工具：工作区执行脚本+收集输出
│   ├── list_docs.go           # skill_list_docs 工具：列出可用文档
│   ├── select_docs.go         # skill_select_docs 工具：选择文档
│   ├── exec.go                # 执行辅助
│   ├── stager.go              # 技能目录物化到工作区
│   └── schema.go              # JSON Schema 构建
└── agent/llmagent/
    └── option.go              # WithSkills / WithSkillFilter / WithAllowedSkillTools / WithSkillLoadMode 等
```

### 优化目标

将项目 Skill 运行时从 ADK `skilltoolset` 迁移到 trpc `skill.Repository` + `tool/skill/*` 工具集，实现：
1. 渐进披露（概览→正文→文档三层模型）
2. 工作区脚本执行（`skill_run`）
3. 文档选择（`skill_list_docs` / `skill_select_docs`）
4. 会话状态生命周期管理（`SkillLoadMode`）
5. Prompt Cache 优化（`WithSkillsLoadedContentInToolResults`）
6. 请求级技能过滤（`WithSkillFilter`）

### 具体步骤

#### 步骤 1.1：创建 trpc skill.Repository 适配器

**目标**：将项目现有的文件系统 Skill 存储桥接为 trpc `skill.Repository` 接口

**涉及文件**：
- 新建 `internal/skill/trpc/repository.go`
- 修改 `internal/pkg/skillstorage/root.go`（可能需要导出更多方法）

**实现要点**：
```go
// internal/skill/trpc/repository.go
package trpc

import (
    trpcskill "trpc.group/trpc-go/trpc-agent-go/skill"
)

// FSRepositoryAdapter wraps the project skill storage as a trpc skill.Repository.
type FSRepositoryAdapter struct {
    root string
    // delegate to trpc skill.FSRepository
    delegate *trpcskill.FSRepository
}

func NewFSRepositoryAdapter(root string) (*FSRepositoryAdapter, error) {
    repo, err := trpcskill.NewFSRepository(root)
    if err != nil {
        return nil, err
    }
    return &FSRepositoryAdapter{root: root, delegate: repo}, nil
}

func (a *FSRepositoryAdapter) Summaries() []trpcskill.Summary {
    return a.delegate.Summaries()
}

func (a *FSRepositoryAdapter) Get(name string) (*trpcskill.Skill, error) {
    return a.delegate.Get(name)
}

func (a *FSRepositoryAdapter) Path(name string) (string, error) {
    return a.delegate.Path(name)
}
```

**验收标准**：`FSRepositoryAdapter` 实现 `trpcskill.Repository` 接口，能正确扫描项目 Skill 存储目录并返回 Summaries/Get/Path

#### 步骤 1.2：创建 trpc skill 工具集工厂

**目标**：将 trpc `tool/skill` 包的 LoadTool/RunTool/ListDocsTool/SelectDocsTool 集成到项目工具注册体系

**涉及文件**：
- 新建 `internal/skill/trpc/tools.go`
- 修改 `internal/tools/registry/keys.go`（添加新工具键名）
- 修改 `internal/tools/registry/adk_enabled.go`（集成新工具）

**实现要点**：
```go
// internal/skill/trpc/tools.go
package trpc

import (
    trpcskill "trpc.group/trpc-go/trpc-agent-go/skill"
    trpcskilltool "trpc.group/trpc-go/trpc-agent-go/tool/skill"
    "trpc.group/trpc-go/trpc-agent-go/codeexecutor"
)

// SkillToolsetConfig holds configuration for building the trpc skill toolset.
type SkillToolsetConfig struct {
    Repo           trpcskill.Repository
    Executor       codeexecutor.CodeExecutor
    ForceSaveArtifacts bool
}

// BuildSkillTools creates the full set of trpc skill tools.
func BuildSkillTools(cfg SkillToolsetConfig) []tool.Tool {
    var tools []tool.Tool
    tools = append(tools, trpcskilltool.NewLoadTool(cfg.Repo))
    tools = append(tools, trpcskilltool.NewRunTool(cfg.Repo, cfg.Executor))
    tools = append(tools, trpcskilltool.NewListDocsTool(cfg.Repo))
    tools = append(tools, trpcskilltool.NewSelectDocsTool(cfg.Repo))
    return tools
}
```

**验收标准**：`BuildSkillTools` 返回 4 个 trpc skill 工具实例，工具声明正确

#### 步骤 1.3：替换 ADK skilltoolset 为 trpc skill 工具集

**目标**：在 Agent 构建时，用 trpc skill 工具替换 ADK skilltoolset

**涉及文件**：
- 修改 `internal/tools/skillruntime/toolset.go`（替换 `NewSkillToolsetFromFS` 和 `AppendEnabledPublishedSkillToolsets`）
- 修改 `internal/tools/turn_mount.go`（更新 SkillToolset 挂载逻辑）
- 修改 `internal/tools/catalog/assemble.go`（更新工具集组装）
- 修改 `internal/agent/adk_build.go`（更新 BuilderDeps 和 BuildLLMAgent）

**实现要点**：
1. `AppendEnabledPublishedSkillToolsets` 改为调用 `trpc.NewFSRepositoryAdapter` + `trpc.BuildSkillTools`
2. `BuilderDeps` 增加 `SkillRepo trpcskill.Repository` 和 `CodeExecutor codeexecutor.CodeExecutor` 字段
3. `BuildLLMAgent` 中将 trpc skill 工具追加到 `deps.Tools`

**验收标准**：Agent 对话时能调用 `skill_load`/`skill_run`/`skill_list_docs`/`skill_select_docs`，渐进披露生效

#### 步骤 1.4：集成 CodeExecutor 用于 skill_run

**目标**：为 `skill_run` 提供 Local 或 E2B 执行器

**涉及文件**：
- 新建 `internal/skill/trpc/executor.go`
- 修改 `internal/adkdeps/deps.go`（TurnDeps 增加 CodeExecutor）

**实现要点**：
```go
// internal/skill/trpc/executor.go
package trpc

import (
    localexec "trpc.group/trpc-go/trpc-agent-go/codeexecutor/local"
)

// NewLocalExecutor creates a local code executor for skill_run.
func NewLocalExecutor() (codeexecutor.CodeExecutor, error) {
    return localexec.New(), nil
}
```

**验收标准**：`skill_run` 能在本地工作区执行脚本并返回输出

#### 步骤 1.5：实现 SkillFilter 按请求过滤

**目标**：根据 `SkillRuntimePolicy`（allowed_slugs/denied_slugs/allowed_tags）过滤可见技能

**涉及文件**：
- 修改 `internal/skill/trpc/repository.go`（增加 `ContextAwareRepository` 实现）
- 修改 `internal/tools/skillruntime/resolve.go`（复用现有过滤逻辑）

**实现要点**：
实现 `trpcskill.ContextAwareRepository` 接口，在 `SummariesForContext` 中应用 Layer A/B 过滤

**验收标准**：不同 Agent/请求只能看到被策略允许的技能概览

#### 步骤 1.6：清理旧 ADK skilltoolset 代码

**目标**：移除对 `google.golang.org/adk/tool/skilltoolset` 的依赖

**涉及文件**：
- 删除或标记废弃 `internal/tools/skillruntime/toolset.go` 中的 ADK 引用
- 更新 `go.mod`（确认 ADK skilltoolset 无其他引用后可移除）

**验收标准**：`go build ./...` 通过，无 ADK skilltoolset 引用残留

---

## M2: Agent 构建 — 向 trpc 框架对齐 [P0]

### 现状分析

项目使用 ADK `llmagent.New(cfg)` 构建 Agent，但：
1. **无占位符变量**：trpc 支持 `{key}`/`{key?}`/`{user:subkey}` 会话状态注入，项目未使用
2. **无 IncludeContents 控制**：trpc 支持 `IncludeContentsDefault`/`IncludeContentsNone`，项目通过 `L0SnapshotMode` 模拟
3. **无 Planner**：trpc 支持 `BuiltinPlanner`/`ReActPlanner`，项目无规划能力
4. **无 SkillLoadMode**：trpc 支持 `turn`/`session` 模式，项目未实现
5. **无 Prompt Cache 优化**：trpc 支持 `WithSkillsLoadedContentInToolResults`，项目未实现

### 优化目标

1. 迁移到 trpc `llmagent.New(cfg)` 构建
2. 启用占位符变量注入
3. 集成 Planner
4. 启用 SkillLoadMode 和 Prompt Cache 优化

### 具体步骤

#### 步骤 2.1：迁移 BuildLLMAgent 到 trpc llmagent

**目标**：将 `internal/agent/adk_build.go` 的 `BuildLLMAgent` 从 ADK 迁移到 trpc

**涉及文件**：
- 修改 `internal/agent/adk_build.go`
- 修改 `internal/agent/trpc_runtime.go`
- 修改 `internal/agent/adk_runner_runtime.go`

**实现要点**：
1. 导入 `trpcagent "trpc.group/trpc-go/trpc-agent-go/agent/llmagent"`
2. 将 `llmagent.Config` 替换为 trpc 版本
3. 将 `llmagent.New(cfg)` 替换为 trpc 版本
4. 保留 `BuilderDeps` 结构但调整字段类型

**验收标准**：Agent 仍能正常对话，工具挂载正常

#### 步骤 2.2：启用占位符变量

**目标**：在 Agent Instruction 中支持 `{key}` 占位符

**涉及文件**：
- 修改 `internal/agent/prompt.go`
- 修改 `internal/agent/adk_build.go`

**实现要点**：
1. 在 `BuildSystemPrompt` 中使用 `{user:name}`/`{temp:timezone}` 等占位符
2. 通过 Session State 注入运行时值

**验收标准**：Agent Instruction 中的占位符在运行时被正确替换

#### 步骤 2.3：集成 Planner

**目标**：为 Agent 添加规划能力

**涉及文件**：
- 新建 `internal/agent/planner.go`
- 修改 `internal/agent/adk_build.go`

**实现要点**：
```go
import (
    trpcplanner "trpc.group/trpc-go/trpc-agent-go/planner"
    "trpc.group/trpc-go/trpc-agent-go/planner/react"
)

// 在 BuildLLMAgent 中
cfg.Planner = react.NewPlanner(m)
```

**验收标准**：Agent 在复杂任务中先规划再执行

---

## M3: Team 编排 — 向 trpc 框架对齐 [P1]

### 现状分析

项目实现了自定义 `team.Runner`，支持 Coordinator 和 Swarm 两种模式，但：
1. **未使用 trpc team 包**：trpc 提供了 `team.NewCoordinator`/`team.NewSwarm`，项目自己实现
2. **AgentTool 未使用 trpc 版本**：trpc 的 `tool/agent.AgentTool` 支持流式内部转发
3. **TransferTool 未使用 trpc 版本**：trpc 的 `tool/transfer.TransferTool` 更完善

### 优化目标

1. 迁移到 trpc `team.NewCoordinator`/`team.NewSwarm`
2. 使用 trpc `tool/agent.AgentTool` 和 `tool/transfer.TransferTool`
3. 保留产品层的 Team 管理 API

### 具体步骤

#### 步骤 3.1：迁移 Team 构建到 trpc team 包

**涉及文件**：
- 修改 `internal/team/builder.go`
- 修改 `internal/team/definition.go`
- 修改 `internal/team/runner.go`

**实现要点**：
1. `BuildCoordinatorTeam` 使用 `team.NewCoordinator`
2. `BuildSwarmTeam` 使用 `team.NewSwarm`
3. 保留 `team.Runner` 作为产品层封装

**验收标准**：Coordinator 和 Swarm 模式通过 trpc team 包构建并正常运行

#### 步骤 3.2：替换 AgentTool 和 TransferTool

**涉及文件**：
- 修改 `internal/tools/registry/keys.go`
- 修改 `internal/agent/native_tools.go`

**验收标准**：Agent 间 Transfer 和 AgentTool 调用正常

---

## M4: Graph 工作流 — 新增实现 [P1]

### 现状分析

项目无 Graph 工作流能力。trpc 框架提供了完整的 `graph.StateGraph`：
- 节点类型：LLM/Tool/Agent/Function
- 边类型：普通/条件
- 状态管理：Schema + Reducer
- 高级特性：HITL（中断/恢复）、检查点、时间旅行、子图
- 两种执行引擎：BSP（默认）/ DAG

### 优化目标

1. 集成 trpc `graph.StateGraph` 构建器
2. 通过 API 暴露 Graph 工作流定义和执行
3. 实现 HITL 场景

### 具体步骤

#### 步骤 4.1：创建 Graph 构建服务

**涉及文件**：
- 新建 `internal/graph/builder.go`
- 新建 `internal/graph/schema.go`
- 新建 `api/kratos/graph/v1/` proto 定义

**实现要点**：
```go
import (
    trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
    trpcgraphagent "trpc.group/trpc-go/trpc-agent-go/agent/graphagent"
)

func BuildGraphFromDefinition(def GraphDefinition) (*trpcgraph.StateGraph, error) {
    schema := trpcgraph.NewStateSchema()
    g := trpcgraph.NewStateGraph(schema)
    // 添加节点和边...
    return g, nil
}
```

**验收标准**：能通过 API 定义并执行简单 Graph 工作流

#### 步骤 4.2：实现 Graph 执行端点

**涉及文件**：
- 新建 `internal/service/graph.go`
- 新建 `internal/server/register_graph.go`

**验收标准**：通过 gRPC/REST 触发 Graph 工作流执行

#### 步骤 4.3：实现 HITL 中断/恢复

**涉及文件**：
- 修改 `internal/graph/builder.go`
- 新建 `internal/graph/hitl.go`

**验收标准**：Graph 执行到中断点暂停，用户确认后恢复

---

## M5: Session 管理 — 向 trpc 框架对齐 [P1]

### 现状分析

项目使用 ADK Session + 自定义 `sessionmemory.Store`（SQLite），但：
1. **未使用 trpc Session**：trpc 提供了 `session.Service` + 多后端 + 摘要压缩
2. **摘要压缩独立实现**：项目 `internal/compress` 独立实现了摘要，未用 trpc 内置
3. **无 Redis/PG 后端**：trpc 支持 Redis/PostgreSQL/MySQL/ClickHouse

### 优化目标

1. 迁移到 trpc `session.Service`
2. 使用 trpc 内置摘要压缩
3. 支持 Redis 后端用于生产

### 具体步骤

#### 步骤 5.1：创建 trpc SessionService 适配器

**涉及文件**：
- 新建 `internal/session/trpc/service.go`
- 修改 `internal/adkdeps/deps.go`

**实现要点**：
桥接项目现有 SQLite session 存储到 trpc `session.Service` 接口

**验收标准**：trpc Runner 使用适配后的 SessionService 正常运行

#### 步骤 5.2：集成 trpc 摘要压缩

**涉及文件**：
- 修改 `internal/session/trpc/service.go`
- 可能废弃 `internal/compress/`

**验收标准**：长对话自动触发摘要，token 消耗降低

#### 步骤 5.3：支持 Redis Session 后端

**涉及文件**：
- 新建 `internal/session/trpc/redis.go`
- 修改配置文件

**验收标准**：生产环境使用 Redis 存储 Session

---

## M6: Memory 记忆 — 向 trpc 框架对齐 [P2]

### 现状分析

项目使用 ADK Memory + SQLite session entities + pgvector，但：
1. **无自动提取模式**：trpc 支持 `memory.extractor` 从对话中自动提取记忆
2. **无记忆工具**：trpc 提供 `memory_add`/`memory_update`/`memory_search`/`memory_load`/`memory_clear`
3. **无 Mem0 集成**：trpc 支持 `memory/mem0` 后端

### 优化目标

1. 迁移到 trpc `memory.Service`
2. 启用自动提取模式
3. 集成记忆工具

### 具体步骤

#### 步骤 6.1：创建 trpc MemoryService 适配器

**涉及文件**：
- 新建 `internal/memory/trpc/service.go`

**验收标准**：trpc MemoryService 正常工作

#### 步骤 6.2：启用自动提取

**涉及文件**：
- 修改 `internal/memory/trpc/service.go`
- 修改 `internal/agent/adk_build.go`

**验收标准**：对话后自动提取用户信息并存储

#### 步骤 6.3：集成记忆工具

**涉及文件**：
- 修改 `internal/tools/registry/keys.go`
- 修改 `internal/tools/registry/adk_builtin.go`

**验收标准**：Agent 可调用 `memory_search`/`memory_load` 等工具

---

## M7: Tool 工具体系 — 向 trpc 框架对齐 [P1]

### 现状分析

项目实现了自定义工具注册体系，但：
1. **未使用 trpc Tool 接口**：trpc 有 `tool.Tool`/`tool.CallableTool`/`tool.StreamableTool`
2. **无流式工具**：trpc 支持流式工具响应
3. **无工具重试**：trpc 支持单次调用重试
4. **无工具过滤**：trpc 支持 `tool.Filter`
5. **FunctionTool 包装不一致**：项目用 ADK FunctionTool，trpc 有自己的 `tool/function`

### 优化目标

1. 迁移到 trpc `tool.Tool`/`tool.ToolSet` 接口
2. 使用 trpc `tool/function` 包装函数工具
3. 启用工具重试和过滤

### 具体步骤

#### 步骤 7.1：迁移工具注册到 trpc 接口

**涉及文件**：
- 修改 `internal/tools/registry/adk_enabled.go`
- 修改 `internal/tools/registry/adk_builtin.go`
- 修改 `internal/tools/catalog/assemble.go`

**实现要点**：
将 `[]tool.Tool`（ADK）替换为 `[]trpctool.Tool`（trpc），使用 `trpctool.Function` 包装函数工具

**验收标准**：所有内置工具通过 trpc Tool 接口注册

#### 步骤 7.2：迁移 workspace 工具

**涉及文件**：
- 修改 `internal/tools/read_file/tool.go`
- 修改 `internal/tools/list_files/tool.go`
- 修改 `internal/tools/write_file/tool.go`
- 修改 `internal/tools/edit_file/tool.go`
- 修改 `internal/tools/shell_exec/tool.go`
- 修改 `internal/tools/web_search/tool.go`

**实现要点**：
每个工具实现 `trpctool.CallableTool` 接口（`Declaration()` + `Call()`）

**验收标准**：所有 workspace 工具通过 trpc 接口工作

#### 步骤 7.3：启用工具重试

**涉及文件**：
- 修改 `internal/tools/catalog/assemble.go`

**验收标准**：工具调用失败时自动重试

---

## M8: MCP 集成 — 向 trpc 框架对齐 [P2]

### 现状分析

项目通过数据库管理 MCP Server 配置，但运行时 MCP 工具集挂载仍依赖 ADK。trpc 提供了完整的 `tool/mcp.ToolSet`（STDIO/SSE/StreamableHTTP）和 `tool/mcpbroker.Broker`。

### 优化目标

1. 使用 trpc `tool/mcp.ToolSet` 挂载 MCP 工具
2. 使用 trpc `tool/mcpbroker.Broker` 管理 MCP 连接

### 具体步骤

#### 步骤 8.1：创建 MCP ToolSet 工厂

**涉及文件**：
- 新建 `internal/mcp/trpc/toolset.go`

**验收标准**：从数据库配置创建 trpc MCP ToolSet

#### 步骤 8.2：集成 MCP Broker

**涉及文件**：
- 新建 `internal/mcp/trpc/broker.go`

**验收标准**：MCP 连接生命周期正确管理

---

## M9: Model 模型层 — 向 trpc 框架对齐 [P2]

### 现状分析

项目通过 `provider.openai` 适配 OpenAI 兼容 API。trpc 提供了更丰富的模型支持：
- OpenAI/Gemini/Anthropic/Ollama/Bedrock/Hunyuan
- Failover/Hedge 策略
- Token 计数（tiktoken）

### 优化目标

1. 使用 trpc `model/openai` 替换自定义适配器
2. 启用 Failover 多模型降级
3. 启用 Token 计数

### 具体步骤

#### 步骤 9.1：迁移到 trpc model/openai

**涉及文件**：
- 修改 `internal/provider/openai/llm.go`
- 修改 `internal/provider/registry.go`

**验收标准**：模型调用通过 trpc openai 适配器

#### 步骤 9.2：启用 Failover

**涉及文件**：
- 新建 `internal/provider/failover.go`

**验收标准**：主模型不可用时自动切换到备用模型

---

## M10: Plugin 插件 — 向 trpc 框架对齐 [P2]

### 现状分析

项目使用 ADK `DefaultRunnerPlugins`，但 trpc 提供了更完善的 Plugin 体系：
- Runner 级生命周期钩子
- BeforeModel/AfterModel/BeforeTool/AfterTool/OnEvent
- Identity Plugin 示例

### 优化目标

1. 迁移到 trpc `plugin.Plugin` 体系
2. 实现自定义 Plugin（日志、审计、安全）

### 具体步骤

#### 步骤 10.1：迁移到 trpc Plugin

**涉及文件**：
- 修改 `internal/agent/adk_plugins.go`
- 修改 `internal/agent/adk_runner.go`

**验收标准**：Runner 使用 trpc Plugin 体系

#### 步骤 10.2：实现自定义 Plugin

**涉及文件**：
- 新建 `internal/plugin/audit.go`
- 新建 `internal/plugin/security.go`

**验收标准**：审计和安全 Plugin 正常工作

---

## M11: Planner 规划 — 新增实现 [P2]

### 现状分析

项目无规划能力。trpc 提供了：
- `BuiltinPlanner`：适用于支持原生思考的模型
- `ReActPlanner`：适用于不支持原生思考的模型

### 优化目标

1. 集成 trpc Planner
2. 在 Agent 构建时可选启用

### 具体步骤

#### 步骤 11.1：集成 ReActPlanner

**涉及文件**：
- 新建 `internal/agent/planner.go`
- 修改 `internal/agent/adk_build.go`

**验收标准**：Agent 在复杂任务中先规划再执行

---

## M12: Artifact 制品 — 向 trpc 框架对齐 [P2]

### 现状分析

项目使用 ADK Artifact。trpc 提供了 `artifact.Service` + S3/COS 后端 + 版本管理。

### 优化目标

1. 迁移到 trpc `artifact.Service`
2. 支持 S3/COS 后端

### 具体步骤

#### 步骤 12.1：迁移到 trpc ArtifactService

**涉及文件**：
- 新建 `internal/artifact/trpc/service.go`
- 修改 `internal/agent/adk_artifact.go`

**验收标准**：Artifact 存取正常

---

## M13: Knowledge 知识库 — 新增实现 [P3]

### 现状分析

项目无知识库能力。trpc 提供了 `knowledge.Service` + OCR + Query + RAG。

### 优化目标

1. 集成 trpc Knowledge
2. 支持文档加载和检索

### 具体步骤

#### 步骤 13.1：集成 trpc Knowledge

**涉及文件**：
- 新建 `internal/knowledge/trpc/service.go`

**验收标准**：Agent 可检索知识库内容

---

## M14: CodeExecutor — 向 trpc 框架对齐 [P2]

### 现状分析

项目有 `shell_exec` 和 `workspace/sandbox`，但未使用 trpc `codeexecutor`。trpc 提供了 Local/E2B 执行器 + WorkspaceRegistry。

### 优化目标

1. 使用 trpc `codeexecutor` 替换自定义实现
2. 支持 E2B 容器执行

### 具体步骤

#### 步骤 14.1：集成 trpc LocalExecutor

**涉及文件**：
- 新建 `internal/executor/trpc/local.go`
- 修改 Skill 集成代码

**验收标准**：`skill_run` 和 `workspace_exec` 通过 trpc 执行器运行

---

## M15: A2A 协议 — 新增实现 [P3]

### 现状分析

项目无 A2A 能力。trpc 提供了完整的 A2A 解决方案。

### 优化目标

1. 实现 A2A Server 暴露本地 Agent
2. 实现 A2A Agent 调用远程 Agent

### 具体步骤

#### 步骤 15.1：实现 A2A Server

**涉及文件**：
- 新建 `internal/a2a/server.go`
- 新建 `api/kratos/a2a/v1/` proto 定义

**验收标准**：本地 Agent 可通过 A2A 协议被远程调用

#### 步骤 15.2：实现 A2A Agent

**涉及文件**：
- 新建 `internal/a2a/agent.go`

**验收标准**：Agent 可调用远程 A2A 服务

---

## M16: Gateway 网关 — 向 trpc 框架对齐 [P3]

### 现状分析

项目使用 Kratos HTTP/gRPC server + SSE。trpc 提供了 `gateway.Gateway` + 会话并发控制 + status/cancel。

### 优化目标

1. 集成 trpc Gateway 的会话并发控制
2. 实现 status/cancel 端点

### 具体步骤

#### 步骤 16.1：集成会话并发控制

**涉及文件**：
- 修改 `internal/service/chat.go`
- 修改 `internal/server/http.go`

**验收标准**：同一 session 同时只运行一个任务

---

## M17: Evaluation 评估 — 新增实现 [P3]

### 现状分析

项目无评估能力。trpc 提供了 `evaluation.AgentEvaluator` + 评估集 + 指标 + LLM-as-Judge。

### 优化目标

1. 集成 trpc Evaluation
2. 支持自动化评估

### 具体步骤

#### 步骤 17.1：集成 trpc Evaluation

**涉及文件**：
- 新建 `internal/evaluation/service.go`

**验收标准**：可对 Agent 进行自动化评估

---

## M18: Event 事件 — 向 trpc 框架对齐 [P2]

### 现状分析

项目使用 ADK 事件 + SSE 投影。trpc 提供了更完善的事件系统：标签、自定义事件、流式事件。

### 优化目标

1. 迁移到 trpc Event 体系
2. 支持自定义事件和标签

### 具体步骤

#### 步骤 18.1：迁移到 trpc Event

**涉及文件**：
- 修改 `internal/service/adk_turn.go`
- 修改 `internal/team/runner_team_adk.go`
- 修改 `internal/server/sse.go`

**验收标准**：事件流通过 trpc Event 体系正确投影到 SSE

---

## 执行顺序与依赖关系

```
Phase 1 (P0 — 核心对齐，无外部依赖):
  M1.1 → M1.2 → M1.3 → M1.4 → M1.5 → M1.6   (Skill 运行时)
  M2.1 → M2.2 → M2.3                           (Agent 构建)

Phase 2 (P1 — 编排与存储，依赖 Phase 1):
  M7.1 → M7.2 → M7.3                           (Tool 工具体系)
  M3.1 → M3.2                                   (Team 编排)
  M5.1 → M5.2 → M5.3                           (Session 管理)
  M4.1 → M4.2 → M4.3                           (Graph 工作流)

Phase 3 (P2 — 增强能力，依赖 Phase 2):
  M6.1 → M6.2 → M6.3                           (Memory 记忆)
  M8.1 → M8.2                                   (MCP 集成)
  M9.1 → M9.2                                   (Model 模型层)
  M10.1 → M10.2                                 (Plugin 插件)
  M11.1                                         (Planner 规划)
  M12.1                                         (Artifact 制品)
  M14.1                                         (CodeExecutor)
  M18.1                                         (Event 事件)

Phase 4 (P3 — 扩展能力，依赖 Phase 3):
  M13.1                                         (Knowledge 知识库)
  M15.1 → M15.2                                 (A2A 协议)
  M16.1                                         (Gateway 网关)
  M17.1                                         (Evaluation 评估)
```

---

## 通用规则（AI 执行时必须遵守）

1. **每步完成后运行 `go build ./cmd/admin` 和 `go vet ./internal/... ./pkg/...`**，确保编译通过
2. **遵循 `docs/guides/AI-DEVELOPMENT-SPECIFICATION.md`** 中的编码规范
3. **优先编辑现有文件**，避免创建不必要的新文件
4. **错误处理统一使用 `kerrors`**，不使用 `fmt.Errorf`
5. **依赖注入通过 Wire**，不在 `func main` 中手动组装
6. **每步只做一个模块的一个步骤**，不跨模块操作
7. **保留产品层 API 兼容**，trpc 迁移是内部实现替换，不改变外部 API
8. **添加单元测试**，每个新函数/方法至少一个测试用例
9. **使用 `pkg/strutil`** 中的工具函数，不重复实现
10. **提交前确认 `go build` + `go vet` + 测试全部通过**
