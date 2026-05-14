# Aranea-Agents

> 企业级 AI Agent 编排平台 — 基于 Kratos v2 + trpc-agent-go

## 项目定位

**Aranea-Agents** 是基于 trpc-agent-go 的多智能体编排平台。以 Kratos v2 为传输壳层、trpc-agent-go 为运行时内核，提供 Agent/Team/Graph 三级编排、五层记忆系统、RAG 知识库、可视化评估平台和多模型接入能力。

## 技术栈

| 层级 | 选型 |
|------|------|
| 后端 | Go + **Kratos v2**（HTTP/gRPC/SSE 传输、Wire DI） |
| Agent 运行时 | **trpc-agent-go**（Runner/Agent/Session/Memory/Tool/Event/Skill/Graph/Team/Planner/Knowledge/CodeExecutor/Evaluation/A2A/Artifact/Callback） |
| 前端 | Vue 3 + Quasar + Pinia + TypeScript |
| 数据库 | SQLite（Ent ORM）+ PostgreSQL（pgvector 向量存储） |
| 依赖注入 | Wire（编译期）；proto 代码生成 `make api` |

## 核心架构

```
┌──────────────────────────────────────────────────────────────┐
│  用户接入层: Web UI / CLI / Channel(飞书等) / A2A / Cron     │
├──────────────────────────────────────────────────────────────┤
│  传输层 (Kratos v2): HTTP :8000 / gRPC :9000 / SSE :8001    │
├──────────────────────────────────────────────────────────────┤
│  Service 层: Chat / Agent / Team / Session / Memory / Tool   │
├──────────────────────────────────────────────────────────────┤
│  Biz 层: AgentUsecase / TeamUsecase / MemoryUsecase / ...    │
├──────────────────────────────────────────────────────────────┤
│  Data 层: SQLite (Ent ORM) + PostgreSQL (pgvector)           │
├──────────────────────────────────────────────────────────────┤
│  Agent 运行时 (trpc-agent-go):                                │
│  Runner → Agent/Team/Graph → Memory/Tool/Event/Planner       │
│  → Plugin/Artifact/CodeExecutor/Knowledge/A2A/Callback       │
├──────────────────────────────────────────────────────────────┤
│  模型驱动层: OpenAI/Anthropic/Gemini/Ollama/Hunyuan/Bedrock  │
│  Failover/Hedge 高可用 │ TokenTailor 上下文裁剪               │
└──────────────────────────────────────────────────────────────┘
```

## 核心模块

| 模块 | 功能 | 状态 |
|------|------|------|
| **Agent** | 单 Agent 构建、运行时设置、提示词管理 | ✅ 已实现 |
| **Team** | 5种编排模式（Coordinator/Swarm/Sequential/Parallel/CriticLoop） | ✅ 已实现 |
| **Graph** | 图工作流引擎、条件路由、HITL、检查点、时间旅行 | ⚠️ 部分实现 |
| **Memory** | L0-L4 五层记忆、自动提取、检索增强 | ⚠️ 部分实现 |
| **Session** | 会话管理、时间轴、摘要压缩 | ⚠️ 部分实现 |
| **Runner** | ManagedRunner/SteerableRunner、AgentFactory | ⚠️ 部分实现 |
| **Tool** | FunctionTool/StreamableTool/MCP/Skill 统一挂载 | ⚠️ 部分实现 |
| **Provider** | 多厂商模型接入、Failover/Hedge、TokenTailor | ✅ 已实现 |
| **Planner** | Builtin/ReAct/A2UI 三种规划模式 | ⚠️ 部分实现 |
| **Knowledge** | RAG 知识库、文档处理、向量化检索 | ❌ 未实现 |
| **CodeExecutor** | Local/E2B/Jupyter/Container 代码执行 | ⚠️ 部分实现 |
| **Evaluation** | LLM-as-Judge、用户模拟、pass@k 指标 | ❌ 未实现 |
| **A2A** | Agent-to-Agent 通信协议 | ❌ 未实现 |
| **Artifact** | 制品存储与版本管理 | ❌ 未实现 |
| **Callback** | 全链路回调钩子 | ❌ 未实现 |
| **Gateway** | 并发控制、运行状态、AwaitUserReply | ⚠️ 部分实现 |
| **Event** | StateDelta/Extensions/FilterKey/Branch/Actions | ⚠️ 部分实现 |
| **Plugin** | 运行时回调扩展机制 | ⚠️ 部分实现 |
| **Skill** | 技能注册、Agent 绑定、运行时挂载 | ✅ 已实现 |
| **MCP** | MCP 服务器管理、工具发现 | ⚠️ 部分实现 |

## 五层记忆系统

| 层级 | 名称 | 存储 | 功能 |
|------|------|------|------|
| L0 | 感官记忆 | SQLite | 最近对话窗口、上下文压缩快照 |
| L1 | 工作记忆 | SQLite | 当前任务/目标追踪 |
| L2 | 情景记忆 | SQLite | 事件片段、重要性评分 |
| L3 | 语义记忆 | pgvector | 向量化知识检索 |
| L4 | 持久记忆 | SQLite | 知识图谱、身份信息 |

## 快速开始

### 环境要求

- Go 1.25+
- Node.js 20+
- SQLite 3+
- PostgreSQL 14+（可选，用于向量存储）

### 后端

```bash
make init      # 初始化工具
make api       # 生成 Proto 代码
make build     # 构建后端
cd cmd/admin && go run . -conf ../../configs  # 启动服务
```

### 前端

```bash
cd web
npm install
npm run serve
```

## 文档导航

| 文档 | 路径 | 说明 |
|------|------|------|
| **AI 编码规范** | [docs/guides/AI-DEVELOPMENT-SPECIFICATION.md](docs/guides/AI-DEVELOPMENT-SPECIFICATION.md) | AI 编码唯一行为准则（十章整合版） |
| **框架工程化解读** | [docs/guides/trpc-agent-go-framework.md](docs/guides/trpc-agent-go-framework.md) | trpc-agent-go 核心接口与项目映射 |
| **功能对齐计划** | [docs/guides/plan.md](docs/guides/plan.md) | 18 模块对齐清单与实施阶段 |
| **系统架构总览** | [docs/需求/0 系统框图.md](docs/需求/0%20系统框图.md) | 系统框图、数据流图、模块依赖矩阵 |
| **需求与设计文档** | [docs/需求/](docs/需求/) | 40+ 模块需求规格与实现设计 |
| **文档入口** | [docs/README.md](docs/README.md) | AI 编码工作流与完整文档索引 |

## 目录结构

```
aranea-agents/
├── api/kratos/           # Proto API 定义 (17+ 模块)
├── cmd/admin/            # 应用入口 + Wire 依赖注入
├── configs/              # 配置文件
├── internal/             # 核心业务代码
│   ├── agent/            # Agent 运行时构建
│   ├── biz/              # 领域模型 + Usecase
│   ├── data/             # 数据访问 (Ent ORM)
│   ├── server/           # 传输层 (HTTP/gRPC/SSE)
│   ├── service/          # Service 实现
│   ├── team/             # Team 编排运行器
│   ├── tools/            # 工具装配 (TurnMount)
│   ├── provider/         # LLM 模型驱动
│   ├── graph/            # Graph 工作流构建
│   ├── memory/           # 记忆服务适配
│   ├── session/          # 会话存储适配
│   ├── skill/            # 技能运行时
│   ├── channel/          # 外部通道集成
│   ├── cronrunner/       # 定时任务调度
│   └── ...
├── pkg/trpc-agent-go/    # trpc-agent-go 框架 (本地 replace)
├── web/                  # Vue 3 + Quasar 前端
└── docs/                 # 项目文档
    ├── guides/           # 编码规范
    ├── 需求/             # 需求规格 + 设计文档
    ├── changelog/        # 变更记录
    └── frontend/         # 前端设计参考
```
