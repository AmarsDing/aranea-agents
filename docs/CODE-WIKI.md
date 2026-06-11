# Aranea-Agents Code Wiki

> 企业级多智能体编排平台 — 基于 trpc-agent-go 运行时 + Kratos v2 传输壳层

---

## 目录

1. [项目概览](#1-项目概览)
2. [技术栈与核心依赖](#2-技术栈与核心依赖)
3. [整体架构](#3-整体架构)
4. [目录结构](#4-目录结构)
5. [后端架构详解](#5-后端架构详解)
6. [trpc-agent-go 框架详解](#6-trpc-agent-go-框架详解)
7. [前端架构详解](#7-前端架构详解)
8. [核心数据流](#8-核心数据流)
9. [API 层与 Proto 定义](#9-api-层与-proto-定义)
10. [数据库与存储](#10-数据库与存储)
11. [事件系统](#11-事件系统)
12. [依赖注入 (Wire)](#12-依赖注入-wire)
13. [配置体系](#13-配置体系)
14. [项目运行方式](#14-项目运行方式)
15. [关键模块索引](#15-关键模块索引)

---

## 1. 项目概览

**Aranea-Agents** 是基于 trpc-agent-go 的企业级多智能体编排平台。核心主旨：让一个人通过"精灵"（Spirit 动态编排引擎）同时控制 N 家虚拟公司，行业专家 Agent 团队自动协作完成从分析、决策到执行的全流程。

**核心能力**：
- **Agent 全生命周期管理**：创建、配置、运行、监控、进化
- **六模式 Team 编排**：Sequential / Parallel / Coordinator / CriticLoop / Swarm / Spirit
- **Graph 图编排**：DAG 有向无环图任务编排，支持条件边、检查点、中断恢复
- **五层记忆架构**：L0（系统提示）→ L1（会话摘要）→ L2（语义召回）→ L3（工作记忆）→ L4（长期事实）
- **A2A 协议**：Google Agent-to-Agent 标准，联邦发现与跨组织互操作
- **13 种 IM Channel**：飞书/Slack/Discord/微信/钉钉/QQ/Telegram/Line 等
- **MCP 支持**：Model Context Protocol 工具集成
- **技能进化**：自动发现、融合去重、版本管理、人机协同审批
- **可观测性**：全链路追踪、根因分析、自动自愈

---

## 2. 技术栈与核心依赖

### 后端

| 类别 | 技术 | 说明 |
|------|------|------|
| 语言 | Go 1.26+ | 主开发语言 |
| 传输框架 | Kratos v2 | HTTP/gRPC/WebSocket 传输壳层 |
| Agent 运行时 | trpc-agent-go | Agent 编排内核（本地 vendor） |
| ORM | Ent v0.14 | SQLite 数据模型 |
| 向量存储 | pgvector / SQLite | 向量嵌入存储 |
| 依赖注入 | Wire | 编译期 DI |
| 数据库 | SQLite（主存储）+ PostgreSQL（向量） | 读写分离 |
| CLI | Cobra | 命令行工具 |
| 认证 | JWT (golang-jwt/v5) | Token 认证 |
| IM SDK | 飞书/Slack/Discord/钉钉/QQ/Telegram | 多 Channel 接入 |
| 监控 | Prometheus + Langfuse + OpenTelemetry | 可观测性 |
| 容器 | Docker + Testcontainers | 部署与测试 |

### 前端

| 类别 | 技术 | 说明 |
|------|------|------|
| 框架 | Vue 3 + Quasar | SPA 应用 |
| 状态管理 | Pinia | 响应式 Store |
| 语言 | TypeScript | 类型安全 |
| 构建 | Vite | 开发与构建 |
| 测试 | Vitest + Playwright | 单元测试 + E2E |
| 国际化 | vue-i18n | 中英双语 |

---

## 3. 整体架构

```
┌─────────────────────────────────────────────────────────────────┐
│                        客户端层                                  │
│  Web (Vue 3/Quasar)  │  CLI (Cobra)  │  IM Channels (13+)      │
│  WebSocket / HTTP     │  HTTP         │  Webhook / WebSocket     │
└────────────┬──────────┴───────┬───────┴──────────┬──────────────┘
             │                  │                  │
┌────────────▼──────────────────▼──────────────────▼──────────────┐
│                     传输壳层 (Kratos v2)                         │
│  HTTP Server (:8000) │ gRPC Server (:9000) │ WS Server (:8002)  │
│  中间件: Auth / Tracing / Recovery / Validate / CORS            │
└────────────┬──────────────────┬──────────────────┬──────────────┘
             │                  │                  │
┌────────────▼──────────────────▼──────────────────▼──────────────┐
│                      服务层 (Service)                            │
│  ChatService / AgentService / TeamService / GraphService / ...  │
│  30+ 服务，每个服务对应一个 Proto 定义                             │
└────────────┬───────────────────────────────────────────────────┘
             │
┌────────────▼───────────────────────────────────────────────────┐
│                      业务层 (Biz / Usecase)                     │
│  ChatUsecase / AgentUsecase / TeamUsecase / GraphUsecase / ... │
│  领域事件 / 事件总线 / 编排引擎 / 记忆系统 / 评估系统             │
└────────────┬───────────────────────────────────────────────────┘
             │
┌────────────▼───────────────────────────────────────────────────┐
│                   运行时层 (trpc-agent-go)                       │
│  Runner / Agent / Session / Memory / Tool / Skill / Graph /     │
│  Team / Model / Artifact / Plugin / Event / Knowledge           │
└────────────┬───────────────────────────────────────────────────┘
             │
┌────────────▼───────────────────────────────────────────────────┐
│                      数据层 (Data / Repo)                       │
│  SQLite (Ent ORM) │ PostgreSQL (pgvector) │ 文件系统 (Artifact) │
│  读写分离 │ 自动迁移 │ 向量嵌入 │ 懒加载种子                     │
└────────────────────────────────────────────────────────────────┘
```

**双框架分工**：
- **Kratos v2**：传输层（HTTP/gRPC/WebSocket）、配置、鉴权、中间件、Wire DI
- **trpc-agent-go**：Agent 编排（Runner/Agent/Session/Memory/Tool/Event/Skill/Graph/Team）

---

## 4. 目录结构

```
aranea-agents/
├── api/kratos/                  # Proto API 定义（7 个服务）
│   ├── a2a/v1/                  # A2A 协议
│   ├── chat/v1/                 # 聊天服务
│   ├── cron/v1/                 # 定时任务
│   ├── hook/v1/                 # Webhook 钩子
│   ├── pack/v1/                 # 打包导入导出
│   ├── team/v1/                 # 团队管理
│   └── tool/v1/                 # 工具管理
├── cmd/                         # 入口程序
│   ├── admin/                   # 主服务（Wire DI 入口）
│   ├── aranea/                  # CLI 工具
│   ├── araneactl/               # Lint 工具
│   └── dbfix/                   # 数据库修复工具
├── configs/                     # 配置文件
│   └── config.yaml              # 运行时配置
├── internal/                    # 内部包（不可外部导入）
│   ├── a2a/                     # A2A 协议实现
│   ├── agent/                   # Agent 构建/工厂/提示词
│   ├── biz/                     # 业务逻辑层（Usecase）
│   ├── channel/                 # IM Channel 适配器（13+）
│   ├── cli/                     # CLI 客户端与命令
│   ├── compress/                # LLM 响应压缩
│   ├── conf/                    # 配置 Proto 定义
│   ├── cronrunner/              # Cron 任务执行器
│   ├── data/                    # 数据访问层（Repo 实现）
│   ├── event/                   # 事件总线
│   ├── evaluation/              # 评估系统
│   ├── graph/                   # Graph 图编排适配
│   ├── knowledge/               # 知识库入库与检索
│   ├── llmcontext/              # LLM 上下文窗口管理
│   ├── mcp/                     # MCP 默认配置/健康检查
│   ├── metrics/                 # Prometheus 指标
│   ├── modelregistry/           # 模型注册表（models.dev 同步）
│   ├── orgimport/               # 组织架构导入
│   ├── outbound/                # 出站 LLM 路由
│   ├── plugin/                  # 插件系统适配
│   ├── provider/                # LLM Provider 目录
│   ├── runtime/                 # 运行时网关
│   ├── server/                  # HTTP/gRPC/WebSocket 服务器
│   ├── service/                 # 服务层（30+ 服务）
│   ├── session/                 # Session 管理
│   ├── skill/                   # Skill 仓库与导入
│   ├── team/                    # Team 构建与运行
│   ├── telemetry/               # 遥测（Langfuse/OTel）
│   └── tools/                   # 内置工具集
├── pkg/                         # 可复用公共包
│   ├── apierror/                # API 错误封装
│   ├── appctx/                  # 应用上下文
│   ├── auth/                    # 认证中间件
│   ├── ctxuser/                 # 上下文用户提取
│   ├── jsonutil/                # JSON 工具
│   ├── loggateway/              # 日志网关（统一日志接口）
│   ├── logpipeline/             # 日志管道（多 Sink）
│   ├── outboundguard/           # 出站安全守卫
│   ├── outboundwebhook/         # Webhook 签名
│   ├── safego/                  # 安全 Goroutine
│   ├── strutil/                 # 字符串工具
│   ├── trpc-agent-go/           # Agent 运行时框架（vendor）
│   ├── trpcscope/               # trpc 作用域
│   ├── validate/                # 请求验证
│   └── webhookurl/              # Webhook URL 工具
└── web/                         # 前端 Vue 3 应用
    ├── src/
    │   ├── boot/                # 启动插件
    │   ├── config/              # 前端配置
    │   ├── css/                 # 主题样式
    │   ├── domain/              # 领域类型
    │   ├── features/            # 功能 API 层
    │   ├── i18n/                # 国际化
    │   ├── pages/               # 页面组件
    │   ├── realtime/            # 实时通信
    │   ├── router/              # 路由
    │   ├── services/            # Kratos 生成的 HTTP 客户端
    │   ├── shared/              # 共享工具
    │   ├── stores/              # Pinia Store
    │   └── utils/               # 工具函数
    └── e2e/                     # E2E 测试
```

---

## 5. 后端架构详解

### 5.1 分层架构

项目严格遵循 **Kratos DDD 分层**，依赖方向为单向：`Service → Biz → Data`。

```
┌─────────────────────────────────────────┐
│  Service 层 (internal/service/)         │  ← 传输适配，薄层
│  接收 Proto 请求，调用 Biz Usecase       │
├─────────────────────────────────────────┤
│  Biz 层 (internal/biz/)                 │  ← 业务核心，定义接口
│  Usecase 编排业务逻辑，定义 Repo 接口     │
├─────────────────────────────────────────┤
│  Data 层 (internal/data/)               │  ← 数据访问，实现接口
│  实现 Biz 定义的 Repo 接口，操作数据库     │
└─────────────────────────────────────────┘
```

**关键规则**：
- Biz 层定义接口（如 `AgentRepository`），Data 层实现接口
- Service 层不做业务逻辑，只做参数转换和调用 Biz
- 依赖方向：Service → Biz ← Data（Biz 是核心，Data 实现 Biz 的接口）

### 5.2 Service 层核心服务

| 服务 | 文件 | 职责 |
|------|------|------|
| `ChatService` | `service/chat.go` | 聊天消息处理、Turn 执行、流式响应 |
| `AgentService` | `service/agent.go` | Agent CRUD、配置管理 |
| `TeamService` | `service/team.go` | Team 编排、编译、运行 |
| `GraphService` | `service/graph.go` | Graph DAG 编排、执行、追踪 |
| `SkillService` | `service/skill.go` | Skill 仓库、导入、进化 |
| `ToolService` | `service/tool.go` | Tool 注册、审计、预览 |
| `SessionService` | `service/session.go` | Session 管理、批量操作 |
| `MemoryService` | `service/memory.go` | 五层记忆管理、召回 |
| `CronService` | `service/cron.go` | 定时任务 CRUD |
| `HookService` | `service/hook.go` | Webhook 钩子管理 |
| `A2AService` | `service/a2a.go` | A2A 协议发现与调用 |
| `PackService` | `service/pack.go` | 配置打包导入导出 |
| `UsageService` | `service/usage.go` | Token 用量统计与配额 |
| `MonitorService` | `service/monitor.go` | 审计日志、链路追踪 |
| `PluginService` | `service/plugin.go` | 插件管理与审计 |
| `MCPServerService` | `service/mcp_server.go` | MCP 服务器管理 |
| `KnowledgeService` | `service/knowledge.go` | 知识库入库与检索 |
| `EvaluationService` | `service/evaluation.go` | 评估运行与评分 |
| `ArtifactService` | `service/artifact.go` | 产物签名下载 |
| `AvatarService` | `service/avatar.go` | 头像管理 |
| `ChannelService` | `service/channel.go` | IM Channel 管理 |
| `EcosystemService` | `service/ecosystem.go` | 行业生态管理 |
| `OrganizationService` | `service/organization.go` | 组织架构管理 |
| `GatewayService` | `service/gateway.go` | API 网关 |
| `EventService` | `service/event.go` | 事件流 |
| `ModelCatalogService` | `service/model_catalog.go` | 模型目录 |
| `AdminService` | `service/admin.go` | 管理员认证 |
| `SystemSettingService` | `service/system_setting.go` | 系统设置 |

### 5.3 Biz 层核心 Usecase

| Usecase | 文件 | 职责 |
|---------|------|------|
| `ChatUsecase` | `biz/chat_usecase.go` | 聊天核心逻辑，Turn 编排 |
| `AgentUsecase` | `biz/agent_usecase.go` | Agent 生命周期管理 |
| `TeamUsecase` | `biz/team_usecase.go` | Team 编排与运行 |
| `GraphUsecase` | `biz/graph.go` | Graph DAG 编排 |
| `MemoryUsecase` | `biz/memory.go` | 五层记忆管理 |
| `SkillUsecase` | `biz/skill.go` | Skill 进化与去重 |
| `SessionUsecase` | `biz/session/usecase.go` | Session 状态管理 |
| `CronUsecase` | `biz/cron/cron.go` | 定时任务调度 |
| `HookUsecase` | `biz/hook/hook.go` | Webhook 投递 |
| `A2AUsecase` | `biz/a2a/a2a.go` | A2A 联邦发现 |
| `UsageUsecase` | `biz/usage/usage.go` | Token 计费与配额 |
| `MonitorUsecase` | `biz/monitor/monitor.go` | 可观测性与自愈 |
| `EvolutionUsecase` | `biz/evolution.go` | Agent 进化 |
| `KnowledgeUsecase` | `biz/knowledge.go` | 知识库管理 |
| `EvalUsecase` | `biz/evaluation.go` | 评估系统 |
| `ArtifactUsecase` | `biz/artifact/artifact.go` | 产物管理 |
| `PluginUsecase` | `biz/plugin/plugin.go` | 插件管理 |
| `TaskUsecase` | `biz/task.go` | 任务管理 |
| `WebhookUsecase` | `biz/webhook.go` | Webhook 管理 |

### 5.4 关键接口定义（Biz 层）

```go
// Agent 仓库接口
type AgentRepository interface {
    GetAgentByAgentKey(ctx context.Context, key string) (*Agent, error)
    GetAgentByID(ctx context.Context, id string) (*Agent, error)
    SearchAgents(ctx context.Context, q AgentListQuery) (*AgentListResult, error)
    CreateAgent(ctx context.Context, agent *Agent) error
    UpdateAgent(ctx context.Context, agent *Agent) error
    DeleteAgent(ctx context.Context, id string) error
}

// Team 读写接口
type TeamReader interface { ... }
type TeamWriter interface { ... }

// Session 运行接口
type TurnGateway interface { ... }
type TurnExecutor interface { ... }
type TurnControlGateway interface { ... }

// 记忆接口
type MemoryRepo interface { ... }
type EmbeddingService interface { ... }

// 事件总线接口
type Bus interface {
    Publish(ctx context.Context, env Envelope)
    Subscribe(opts SubscribeOptions) (<-chan Envelope, func())
}
```

### 5.5 Agent 构建流程

```
AgentFactory.NewAgent()
  ├── 选择 Provider（OpenAI/Anthropic/Gemini/Ollama/...）
  ├── 构建 trpc-agent-go Agent 实例
  │   ├── 设置 System Prompt（L1~L4 记忆注入）
  │   ├── 组装 Toolset（内置 + MCP + 自定义 + Agent-as-Tool）
  │   ├── 配置 Callback Chain（流式输出、事件发布、用量记录）
  │   └── 设置 Model Selector（模型选择策略）
  └── 返回可运行的 Agent 实例
```

### 5.6 Team 编排模式

| 模式 | 说明 | 实现位置 |
|------|------|----------|
| Sequential | 顺序执行 | `team/runner.go` |
| Parallel | 并行 + Synthesizer | `team/runner.go` |
| Coordinator | 协调者 + Worker | `team/runner.go` |
| CriticLoop | 生成-批评循环 | `team/runner.go` |
| Swarm | 群智协作 | `pkg/trpc-agent-go/team/swarm.go` |
| Spirit | 动态编排（DAG + 自动规划） | `biz/spirit_*.go` |

---

## 6. trpc-agent-go 框架详解

trpc-agent-go 是项目的 Agent 运行时内核，以 vendor 方式嵌入 `pkg/trpc-agent-go/`。

### 6.1 核心概念关系

```
Runner (运行器)
  ├── Agent (智能体)
  │   ├── Tool[] (工具集)
  │   ├── SubAgents[] (子智能体)
  │   └── Callbacks (回调链)
  ├── Session (会话)
  │   ├── State (状态)
  │   ├── Events[] (事件流)
  │   ├── Tracks[] (追踪)
  │   └── Summaries[] (摘要)
  ├── Memory (记忆)
  │   ├── SQLite / Postgres / Redis
  │   └── L0~L4 分层
  ├── Model (模型)
  │   ├── OpenAI / Anthropic / Gemini / Ollama / Bedrock
  │   └── Registry (注册表)
  ├── Graph (图编排)
  │   ├── Node[] / Edge[] / ConditionalEdge[]
  │   ├── Checkpoint (检查点)
  │   └── StateGraph (状态图)
  ├── Team (团队编排)
  │   ├── Coordinator / Swarm
  │   └── Member Agents
  ├── Skill (技能)
  │   └── Repository (仓库)
  ├── Artifact (产物)
  │   └── S3 / COS / Local
  ├── Plugin (插件)
  │   └── Manager (管理器)
  └── Knowledge (知识)
      └── OCR / Query / Embedding
```

### 6.2 核心接口

```go
// Agent 接口 — 智能体的核心抽象
type Agent interface {
    Run(ctx context.Context, invocation *Invocation) (<-chan Event, error)
    Tools() []tool.Tool
    Info() *Info
    SubAgents() []Agent
    FindSubAgent(name string) (Agent, bool)
}

// Tool 接口 — 工具的核心抽象
type Tool interface {
    Declaration() *schema.FunctionDeclaration
    Execute(ctx context.Context, input string) (*Content, error)
}

// Model 接口 — LLM 模型的核心抽象
type Model interface {
    Generate(ctx context.Context, req *Request) (*Response, error)
    StreamGenerate(ctx context.Context, req *Request) (<-chan ResponseChunk, error)
}

// Memory 接口 — 记忆的核心抽象
type Memory interface {
    Get(ctx context.Context, key string) (string, error)
    Set(ctx context.Context, key string, value string) error
    Delete(ctx context.Context, key string) error
    Search(ctx context.Context, query string, opts ...Option) ([]Result, error)
}

// Session — 会话状态容器
type Session struct {
    ID        string
    AppName   string
    UserID    string
    State     map[string]any
    Events    []Event
    Tracks    []Track
    Summaries []Summary
}

// Graph — DAG 图编排
type Graph struct {
    nodes    map[string]NodeFunc
    edges    []Edge
    condEdges []ConditionalEdge
}

// Team — 多 Agent 团队编排
type Team struct {
    members    []Agent
    mode       TeamMode  // coordinator / swarm
    coordinator Agent
}
```

### 6.3 Runner 执行流程

```
Runner.Run(ctx, agent, session, message)
  1. 创建 Invocation（执行上下文）
  2. 加载 Session 状态
  3. 注入 Memory（L0~L4）
  4. 调用 Agent.Run()
     a. 构建 LLM Request（消息 + 工具声明）
     b. 调用 Model.StreamGenerate()
     c. 解析流式响应（文本 / 工具调用）
     d. 如有工具调用 → 执行 Tool → 递归继续
     e. 发布 Event 到事件流
  5. 持久化 Session 状态
  6. 更新 Memory（异步）
  7. 记录 Usage（Token 用量）
```

### 6.4 模型适配器

| 适配器 | 路径 | 说明 |
|--------|------|------|
| OpenAI | `model/openai/` | GPT-4o / GPT-4 / GPT-3.5 |
| Anthropic | `model/anthropic/` | Claude 3.5 / Claude 3 |
| Gemini | `model/gemini/` | Gemini 1.5 Pro/Flash |
| Ollama | `model/ollama/` | 本地模型 |
| Bedrock | `model/bedrock/` | AWS Bedrock |
| Hunyuan | `model/hunyuan/` | 腾讯混元 |
| Hedge | `model/hedge/` | 对冲容错（多模型 fallback） |

### 6.5 工具系统

| 工具类型 | 路径 | 说明 |
|----------|------|------|
| MCP 工具 | `tool/mcp/` | Model Context Protocol |
| 文件工具 | `tool/file/` | 读写/搜索/编辑文件 |
| Agent 工具 | `tool/agent/` | Agent-as-Tool |
| Skill 工具 | `tool/skill/` | 技能执行 |
| 邮件工具 | `tool/email/` | 发送邮件 |
| TODO 工具 | `tool/todo/` | 任务管理 |
| OpenAPI 工具 | `tool/openapi/` | REST API 调用 |
| WebFetch | `tool/webfetch/` | 网页抓取 |
| Wikipedia | `tool/wikipedia/` | 维基百科查询 |
| Claude Code | `tool/claudecode/` | 代码编辑工具集 |
| HostExec | `tool/hostexec/` | 主机命令执行 |

---

## 7. 前端架构详解

### 7.1 分层架构

```
┌─────────────────────────────────────────┐
│  Pages (src/pages/)                     │  ← 页面组件
│  ChatPage / TeamsPage / AgentsPage / ... │
├─────────────────────────────────────────┤
│  Stores (src/stores/)                   │  ← 状态管理 (Pinia)
│  chatStore / teamsStore / agentsStore   │
├─────────────────────────────────────────┤
│  Features (src/features/)               │  ← API 调用层
│  chat/api / teams/api / graph/api       │
├─────────────────────────────────────────┤
│  Services (src/services/)               │  ← Kratos 生成的 HTTP 客户端
│  自动从 Proto 生成                       │
├─────────────────────────────────────────┤
│  Realtime (src/realtime/)               │  ← WebSocket 实时通信
│  Envelope 解析 / 事件分发                │
└─────────────────────────────────────────┘
```

### 7.2 Pinia Store 列表

| Store | 文件 | 职责 |
|-------|------|------|
| `useAuthStore` | `stores/auth.ts` | 用户认证、登录登出、Token 管理 |
| `useAppStore` | `stores/app.ts` | 应用全局状态 |
| `useChatStore` | `stores/chat/` | 聊天消息、会话管理、流式响应 |
| `useTeamsStore` | `stores/teams/` | Team 列表与编排状态 |
| `useGraphStore` | `stores/graph/` | Graph DAG 可视化与执行 |
| `useToolsStore` | `stores/tools/` | Tool 注册与管理 |
| `useMCPStore` | `stores/mcp/` | MCP 服务器连接状态 |
| `useHooksStore` | `stores/hooks/` | Webhook 钩子管理 |
| `useCronStore` | `stores/cron/` | 定时任务管理 |
| `useEventStore` | `stores/event/` | 实时事件流 |
| `useA2AStore` | `stores/a2a/` | A2A 协议管理 |
| `useUsageStore` | `stores/usage/` | Token 用量统计 |

### 7.3 页面组件

| 页面 | 文件 | 功能 |
|------|------|------|
| `ChatPage` | `pages/ChatPage.vue` | 聊天对话界面 |
| `AgentsPage` | `pages/AgentsPage.vue` | Agent 管理界面 |
| `TeamsPage` | `pages/TeamsPage.vue` | Team 编排界面 |
| `GraphsPage` | `pages/GraphsPage.vue` | Graph DAG 编辑器 |
| `ToolsPage` | `pages/ToolsPage.vue` | Tool 管理界面 |
| `SkillsPage` | `pages/SkillsPage.vue` | Skill 仓库界面 |
| `HooksPage` | `pages/HooksPage.vue` | Webhook 管理界面 |
| `A2APage` | `pages/A2APage.vue` | A2A 协议界面 |
| `LoginPage` | `pages/LoginPage.vue` | 登录页面 |

### 7.4 实时通信

前端通过 WebSocket (`/v1/ws`) 接收实时事件：

```
WebSocket 连接
  ├── 认证（JWT Token）
  ├── 三优先级消息队列
  │   ├── High: 工具结果、错误、运行完成
  │   ├── Normal: 文本流、状态变更
  │   └── Low: 日志、调试信息
  ├── 事件回放（断线重连）
  └── 心跳保活
```

### 7.5 主题系统

- 主题定义：`src/css/app-theme.sass`
- 启动配置：`src/boot/theme.ts`
- 支持亮色/暗色模式切换
- Quasar 主题变量覆盖

---

## 8. 核心数据流

### 8.1 聊天消息流

```
用户输入
  │
  ▼
HTTP POST /v1/chat/message  或  WebSocket send
  │
  ▼
ChatService.SendChatMessage()
  │
  ├── 鉴权 (auth.Middleware)
  ├── 参数验证 (validate.Middleware)
  │
  ▼
ChatUsecase.ExecuteTurn()
  │
  ├── 创建/获取 Session
  ├── 构建 Agent (AgentFactory)
  │   ├── 加载 Prompt (L1~L4 记忆注入)
  │   ├── 组装 Tools (内置 + MCP + 自定义)
  │   └── 选择 Model (ModelSelector)
  │
  ▼
Runner.Run(agent, session, message)
  │
  ├── 调用 LLM (Model.StreamGenerate)
  ├── 解析响应 (文本 / ToolCall)
  ├── 执行工具 (Tool.Execute)
  ├── 发布事件 (EventBus.Publish)
  │   ├── → WebSocket 推送到前端
  │   ├── → Monitor 消费（追踪/日志）
  │   ├── → Usage 消费（Token 计费）
  │   ├── → Memory Worker 消费（记忆提取）
  │   └── → Webhook 消费（钩子触发）
  │
  ▼
流式响应返回客户端
```

### 8.2 Team 编排流

```
TeamService.CompileTeam()
  │
  ├── 解析 Team 定义
  ├── 构建 Member Agents
  ├── 选择编排模式
  │
  ▼
TeamRunner.Run()
  │
  ├── Sequential: Agent1 → Agent2 → ... → Synthesizer
  ├── Parallel: Agent1 ‖ Agent2 ‖ ... → Synthesizer
  ├── Coordinator: Coordinator 分派 → Workers 执行 → 汇总
  ├── CriticLoop: Generator → Critic → 迭代优化
  ├── Swarm: 群智协作，动态路由
  └── Spirit: DAG 规划 → 动态分配 → 执行
  │
  ▼
事件流 → 前端实时更新
```

### 8.3 记忆系统流

```
对话消息
  │
  ▼
TurnMemoryWorker (事件消费)
  │
  ├── L0: 系统提示（静态）
  ├── L1: 会话摘要（压缩历史）
  │   └── CompressService.LLM 压缩
  ├── L2: 语义召回（向量搜索）
  │   └── Embedding → pgvector/SQLite 搜索
  ├── L3: 工作记忆（近期上下文）
  │   └── 最近 N 条消息
  └── L4: 长期事实（结构化知识）
      └── LLM 提取 → 事实存储
  │
  ▼
Agent 构建时注入记忆到 Prompt
```

---

## 9. API 层与 Proto 定义

### 9.1 Proto 服务一览

| Proto 服务 | 路径 | 核心 RPC 方法 |
|------------|------|---------------|
| `ChatService` | `chat/v1/` | SendChatMessage, GetChatOptions, StopGeneration, EnqueueUserMessage |
| `AgentService` | `agent/v1/` | ListAgents, CreateAgent, UpdateAgent, DeleteAgent, GetAgent |
| `TeamService` | `team/v1/` | ListTeams, CreateTeam, UpdateTeam, DeleteTeam, CompileTeam, RunTeam |
| `ToolService` | `tool/v1/` | ListTools, CreateTool, UpdateTool, DeleteTool |
| `SkillService` | `skill/v1/` | ListSkills, ImportSkill, EvolveSkill |
| `SessionService` | `session/v1/` | ListSessions, GetSession, DeleteSession |
| `MemoryService` | `memory/v1/` | ListMemories, RecallMemories, DeleteMemory |
| `CronService` | `cron/v1/` | ListCronTasks, CreateCronTask, TriggerCronTask |
| `HookService` | `hook/v1/` | ListHooks, CreateHook, UpdateHook, DeleteHook |
| `A2AService` | `a2a/v1/` | Discover, Invoke, UpdateAgentCard |
| `PackService` | `pack/v1/` | ExportPack, ImportPack, ValidatePack |
| `AdminService` | `admin/v1/` | Login, GetAdminInfo |
| `MonitorService` | `monitor/v1/` | GetFlowLogs, GetTrace, Diagnose |
| `UsageService` | `usage/v1/` | GetUsage, GetQuota |
| `PluginService` | `plugin/v1/` | ListPlugins, RunPlugin |
| `MCPServerService` | `mcp_server/v1/` | ListMCPServers, CreateMCPServer |
| `KnowledgeService` | `knowledge/v1/` | Ingest, Search |
| `EvaluationService` | `evaluation/v1/` | RunEvaluation, GetScores |
| `ArtifactService` | `artifact/v1/` | Upload, Download |
| `EcosystemService` | `ecosystem/v1/` | ListEcosystems |
| `OrganizationService` | `organization/v1/` | ListOrganizations, GetTree |
| `EventService` | `event/v1/` | StreamEvents |
| `GatewayService` | `gateway/v1/` | Proxy |
| `ModelCatalogService` | `model_catalog/v1/` | ListModels, SyncModels |
| `SystemSettingService` | `system_setting/v1/` | Get, Update |

### 9.2 传输协议

| 协议 | 端口 | 用途 |
|------|------|------|
| HTTP | `:8000` | REST API（Proto 生成的 HTTP 路由 + 自定义路由） |
| gRPC | `:9000` | gRPC API（Proto 生成的 gRPC 服务） |
| WebSocket | `:8002` | 实时事件推送（聊天流式响应、监控日志） |

### 9.3 HTTP 中间件链

```
Request
  → CorsDevFilter()          // CORS 开发环境过滤
  → auth.Middleware()         // JWT 认证
  → WorkspaceFilter()         // 工作空间过滤
  → tracing.Server()          // 链路追踪
  → recovery.Recovery()       // Panic 恢复
  → validate.Middleware()     // 参数验证
  → Handler                   // 业务处理
```

### 9.4 自定义路由

| 路由 | 方法 | 说明 |
|------|------|------|
| `/webhooks/{channel_key}` | POST | IM Channel Webhook 回调 |
| `/v1/artifacts/download` | GET | 签名下载（免认证） |
| `/v1/system/info` | GET | CLI 系统信息 |
| `/v1/skill/import/multipart` | POST | Skill 文件上传 |
| `/.well-known/a2a/*` | GET/POST | A2A 公开端点（免认证） |
| `/api/v1/admin/ecosystem/preset/*` | POST/GET | 行业预设管理 |
| `/healthz` | GET | 健康检查 |
| `/metrics` | GET | Prometheus 指标 |

---

## 10. 数据库与存储

### 10.1 存储架构

```
┌─────────────────────────────────────────────┐
│  SQLite (主存储)                              │
│  ├── Ent ORM 自动迁移                        │
│  ├── WAL 模式 + 读写分离                     │
│  ├── 写连接: MaxOpenConns=1                  │
│  └── 读连接: MaxOpenConns=2                  │
├─────────────────────────────────────────────┤
│  PostgreSQL (向量存储，可选)                   │
│  ├── pgvector 扩展                           │
│  ├── Agent 记忆嵌入向量                      │
│  ├── 知识库嵌入向量                          │
│  └── MaxOpenConns=8                          │
├─────────────────────────────────────────────┤
│  文件系统 (Artifact 存储)                     │
│  └── 本地文件系统 (artifactfs)                │
└─────────────────────────────────────────────┘
```

### 10.2 核心数据模型 (Ent Schema)

| Schema | 说明 |
|--------|------|
| `Admin` | 管理员账户 |
| `Agent` | Agent 配置（名称、提示词、模型、工具等） |
| `Team` | Team 定义（成员、编排模式） |
| `Session` | 会话（状态、消息） |
| `SessionRun` | 会话运行记录 |
| `SessionTurn` | 会话回合 |
| `Message` | 聊天消息 |
| `CronTask` / `CronTaskRun` | 定时任务 |
| `GraphTask` / `GraphTaskRun` | Graph 任务 |
| `Hook` / `HookDelivery` | Webhook 钩子 |
| `SkillVersion` | Skill 版本 |
| `PlatformTool` | 平台工具 |
| `PlatformHook` | 平台钩子 |
| `EventStore` | 事件存储 |
| `FlowLogEvent` | 流日志事件 |
| `UsageQuota` | 用量配额 |
| `BudgetAlert` | 预算告警 |
| `Organization` | 组织架构 |
| `CompiledTeam` | 编译后的 Team |
| `HealRecord` | 自愈记录 |
| `TeamRun` / `TeamRunStep` | Team 运行记录 |
| `TaskPlan` | 任务计划 |
| `AvatarAsset` | 头像资源 |

### 10.3 读写分离

Data 层实现了读写分离：

```go
type Data struct {
    entClient  *ent.Client  // 写 Ent 客户端
    readClient *ent.Client  // 读 Ent 客户端
    rawDB      *sql.DB      // 写原始 SQL
    readDB     *sql.DB      // 读原始 SQL
    rw         *ReadWriteClient  // 事务感知的读写客户端
    rwDB       *ReadWriteDB      // 事务感知的读写 DB
}
```

---

## 11. 事件系统

### 11.1 事件总线架构

```
┌──────────────────────────────────────────────┐
│  EventBus (in-process)                       │
│  ├── SessionBus  — 会话事件                   │
│  └── MonitorBus  — 监控事件                   │
├──────────────────────────────────────────────┤
│  发布者                                       │
│  ├── Runner (Agent 运行事件)                  │
│  ├── TeamRunner (Team 运行事件)               │
│  ├── GraphExecutor (Graph 执行事件)           │
│  └── CronRunner (Cron 任务事件)               │
├──────────────────────────────────────────────┤
│  消费者                                       │
│  ├── WebSocket Server → 前端实时推送           │
│  ├── MonitorUsecase → 审计日志/追踪           │
│  ├── UsageUsecase → Token 计费               │
│  ├── TurnMemoryWorker → 记忆提取              │
│  ├── WebhookDispatcher → 钩子投递             │
│  ├── FlowLogUsecase → 流日志                  │
│  └── ToolUsecase → 工具审计                   │
└──────────────────────────────────────────────┘
```

### 11.2 事件类型

| 事件类型 | 优先级 | 说明 |
|----------|--------|------|
| `EnvelopeTypeToolResult` | Critical | 工具执行结果 |
| `EnvelopeTypeError` | Critical | 错误事件 |
| `EnvelopeTypeRunnerCompletion` | Critical | 运行完成 |
| `EnvelopeTypeStateDelta` | Critical | 状态变更 |
| `EnvelopeTypeTokenUsage` | Critical | Token 用量 |
| `EnvelopeTypeRunStatus` | Critical | 运行状态 |
| `EnvelopeTypeCheckpoint` | Critical | 检查点 |
| `EnvelopeTypeSessionStatusChanged` | Critical | 会话状态变更 |
| `EnvelopeTypeTeamRunFinished` | Critical | Team 运行完成 |
| `EnvelopeTypeGraphNodeEnd` | Critical | Graph 节点完成 |
| `EnvelopeTypeTextDelta` | Normal | 文本流增量 |
| `EnvelopeTypeLog` | Low | 日志事件 |

### 11.3 背压与丢弃策略

- **Critical 事件**：永不丢弃，使用 `BlockUpTo` 策略
- **Normal 事件**：队列满时丢弃最旧（`DropOldest`）
- **Low 事件**：队列满时丢弃最新（`DropNewest`）

---

## 12. 依赖注入 (Wire)

### 12.1 Wire ProviderSet 组织

```
cmd/admin/wire.go
  ├── data.ProviderSet      → 130+ Repo 实现
  ├── biz.ProviderSet       → 80+ Usecase 实现
  ├── service.ProviderSet   → 30+ Service 实现
  ├── event.ProviderSet     → 事件总线基础设施
  ├── server.ProviderSet    → HTTP/gRPC/WS 服务器
  └── 其他模块 ProviderSet
```

### 12.2 Wire 绑定示例

```go
// Service 层绑定 Biz 接口
wire.Bind(new(biz.TurnGateway), new(*ChatService))
wire.Bind(new(biz.TurnExecutor), new(*ChatOrchestrator))
wire.Bind(new(biz.GraphExecutor), new(*GraphService))
wire.Bind(new(biz.SessionProjection), new(*SessionProjectionAdapter))
wire.Bind(new(biz.EmbeddingService), new(*MemoryEmbeddingAdapter))
```

### 12.3 Wire 生成

```bash
make wire   # 生成 cmd/admin/wire_gen.go
```

---

## 13. 配置体系

### 13.1 配置结构 (conf.proto)

```
Bootstrap
  ├── Server
  │   ├── HTTP (network, addr, timeout)
  │   ├── GRPC (network, addr, timeout)
  │   ├── WS (enable, network, addr)
  │   ├── Monitor (process_log_enabled)
  │   ├── OpenAI (enable, addr, base_path, model_name)
  │   └── A2APublicBaseURL
  ├── Data
  │   ├── Database (driver, source) — 遗留
  │   ├── Sqlite (enable, source) — 推荐
  │   ├── Postgres (source, vector_dim) — 向量存储
  │   ├── Redis (network, addr, timeout)
  │   └── InitialAdmin (name, email, password, access)
  ├── Logging
  │   ├── level, output_dir, max_size_mb, ...
  │   └── sinks[] (name, type, buffer_size, drop_policy)
  ├── DebugRecorder (enable, dir, mode)
  ├── Langfuse (enable, keys, base_url)
  └── Runtime
      ├── WS (连接/队列/超时参数)
      ├── Hook (投递/重试参数)
      ├── SelfHeal (自愈/熔断参数)
      ├── MemoryQueue (记忆队列参数)
      ├── Webhook (限流参数)
      ├── AutoMemory (自动记忆参数)
      └── ActivityFlusher (活动刷新参数)
```

### 13.2 环境变量覆盖

配置支持环境变量覆盖，格式：`SECTION__KEY`（双下划线分隔层级），如：
- `DATA__POSTGRES__SOURCE` — PostgreSQL 连接串
- `DATA__INITIAL_ADMIN__PASSWORD` — 初始管理员密码

---

## 14. 项目运行方式

### 14.1 环境准备

```bash
# 安装 Go 工具链
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
go install github.com/go-kratos/kratos/cmd/protoc-gen-go-http/v2@latest
go install github.com/google/wire/cmd/wire@latest

# 一键安装
make init
```

### 14.2 构建命令

```bash
# 生成 Proto 代码
make api          # API Proto → Go HTTP/gRPC + TypeScript
make config       # 内部 Proto → Go

# Wire 依赖注入
make wire

# 编译
make build        # 编译所有二进制
make cli          # 编译 CLI 工具

# 测试
make test         # 运行所有测试
make lint         # 运行 golangci-lint

# 完整验证（提交前）
make api && make wire && make build && make test && make lint
```

### 14.3 启动服务

```bash
# 后端主服务
go run ./cmd/admin -conf ./configs

# 或使用编译后的二进制
./bin/admin -conf ./configs
```

默认端口：
- HTTP: `0.0.0.0:8000`
- gRPC: `0.0.0.0:9000`
- WebSocket: `0.0.0.0:8002`

### 14.4 前端开发

```bash
cd web

# 安装依赖
pnpm install

# 开发模式
pnpm dev

# 构建
pnpm build

# 测试
pnpm test        # 单元测试
pnpm test:e2e    # E2E 测试

# Lint
pnpm lint
```

### 14.5 Docker 部署

```bash
# 使用 Docker Compose
docker-compose -f docker-compose.executor.yml up

# PostgreSQL 模式
docker-compose -f docker-compose.postgres.yml up
```

### 14.6 CLI 工具

```bash
# 编译 CLI
make cli

# 使用
./bin/aranea chat          # 交互式聊天
./bin/aranea agent list    # 列出 Agent
./bin/aranea team list     # 列出 Team
./bin/aranea skill list    # 列出 Skill
./bin/aranea login         # 登录
```

---

## 15. 关键模块索引

### 15.1 后端模块速查

| 模块 | 路径 | 核心结构体/接口 | 职责 |
|------|------|-----------------|------|
| Agent 工厂 | `internal/agent/` | `AgentFactory`, `AgentBuilder` | 构建 Agent 实例 |
| Team 构建 | `internal/team/` | `TeamBuilder`, `TeamRunner` | Team 编排与运行 |
| Graph 适配 | `internal/graph/trpc/` | `GraphBuilder`, `GraphValidator` | Graph DAG 编排 |
| Session 管理 | `internal/session/` | `SessionProvider`, `SessionRuntime` | 会话生命周期 |
| 记忆系统 | `internal/biz/memory*.go` | `MemoryUsecase`, `TurnMemoryWorker` | 五层记忆 |
| 知识库 | `internal/knowledge/` | `Ingestor`, `Retriever`, `Chunker` | 知识入库与检索 |
| 出站路由 | `internal/outbound/` | `Router`, `Adapter` | LLM 请求路由 |
| Provider 目录 | `internal/provider/` | `Catalog` | 模型提供商目录 |
| 模型注册表 | `internal/modelregistry/` | `Store`, `Sync` | models.dev 同步 |
| 压缩服务 | `internal/compress/` | `LLMService`, `CompressCache` | LLM 响应压缩 |
| LLM 上下文 | `internal/llmcontext/` | `Window` | 上下文窗口管理 |
| Cron 运行器 | `internal/cronrunner/` | `CronRunner` | 定时任务执行 |
| Channel 适配 | `internal/channel/` | `Surface`, `Ingress` | IM 消息路由 |
| 评估系统 | `internal/evaluation/` | `Runner`, `LLMJudge` | Agent 评估 |
| 组织导入 | `internal/orgimport/` | `Loader`, `Applier` | 组织架构导入 |
| A2A 协议 | `internal/a2a/` | `Invoker`, `RemoteClient` | A2A 联邦调用 |
| 插件系统 | `internal/plugin/trpc/` | `Manager`, `Registry` | 插件管理 |
| MCP 支持 | `internal/mcp/` | `Defaults`, `HealthRunner` | MCP 配置与监控 |
| 遥测 | `internal/telemetry/` | `Sampler`, `LangfuseAdapter` | 可观测性 |
| 工具系统 | `internal/tools/` | `ToolRegistry`, `Toolset` | 工具注册与执行 |
| Skill 仓库 | `internal/skill/` | `Repository`, `Importer` | Skill 存储 |
| 事件总线 | `internal/event/` | `Bus`, `Buffer`, `Envelope` | 事件发布订阅 |
| 日志网关 | `pkg/loggateway/` | `Logger`, `Gateway` | 统一日志接口 |
| 日志管道 | `pkg/logpipeline/` | `Pipeline`, `Sink` | 多 Sink 日志 |
| 认证 | `pkg/auth/` | `Middleware`, `TokenVerifier` | JWT 认证 |
| 运行时网关 | `internal/runtime/` | `Gateway`, `Lane` | 运行时调度 |

### 15.2 trpc-agent-go 核心模块速查

| 模块 | 路径 | 核心接口 | 职责 |
|------|------|----------|------|
| Agent | `pkg/trpc-agent-go/agent/` | `Agent`, `Invocation` | 智能体核心抽象 |
| Runner | `pkg/trpc-agent-go/runner/` | `Runner` | 运行器，编排 Agent 执行 |
| Session | `pkg/trpc-agent-go/session/` | `Session`, `Ingestor` | 会话状态管理 |
| Memory | `pkg/trpc-agent-go/memory/` | `Memory` | 记忆存储（多后端） |
| Model | `pkg/trpc-agent-go/model/` | `Model`, `Registry` | LLM 模型适配 |
| Tool | `pkg/trpc-agent-go/tool/` | `Tool`, `ToolSet` | 工具核心抽象 |
| Graph | `pkg/trpc-agent-go/graph/` | `Graph`, `Executor` | DAG 图编排 |
| Team | `pkg/trpc-agent-go/team/` | `Team`, `Runtime` | 多 Agent 团队 |
| Skill | `pkg/trpc-agent-go/skill/` | `Repository` | 技能仓库 |
| Event | `pkg/trpc-agent-go/event/` | `Event` | 事件定义 |
| Artifact | `pkg/trpc-agent-go/artifact/` | `Artifact` | 产物管理 |
| Plugin | `pkg/trpc-agent-go/plugin/` | `Manager` | 插件管理 |
| Knowledge | `pkg/trpc-agent-go/knowledge/` | `Knowledge` | 知识检索 |
| Evaluation | `pkg/trpc-agent-go/evaluation/` | `Evaluation` | 评估框架 |
| Planner | `pkg/trpc-agent-go/planner/` | `Planner` | 任务规划 |
| Prompt | `pkg/trpc-agent-go/prompt/` | `Template` | 提示词模板 |
| A2A Server | `pkg/trpc-agent-go/server/a2a/` | `Server` | A2A 协议服务端 |
| OpenAI Server | `pkg/trpc-agent-go/server/openai/` | `Server` | OpenAI 兼容服务端 |

### 15.3 前端模块速查

| 模块 | 路径 | 职责 |
|------|------|------|
| 路由 | `src/router/` | 页面路由定义与导航守卫 |
| Store | `src/stores/` | Pinia 状态管理（12 个 Store） |
| API | `src/features/` | 后端 API 调用封装 |
| 服务 | `src/services/` | Kratos Proto 生成的 HTTP 客户端 |
| 实时 | `src/realtime/` | WebSocket 事件解析 |
| 页面 | `src/pages/` | 9 个主要页面组件 |
| 启动 | `src/boot/` | Quasar 启动插件 |
| 配置 | `src/config/` | 前端运行时配置 |
| 主题 | `src/css/` | SASS 主题定义 |
| 国际化 | `src/i18n/` | 中英双语 |
| 领域 | `src/domain/` | 领域类型定义 |
| 共享 | `src/shared/` | 通用工具函数 |

---

## 附录：项目红线速查

| 编号 | 规则 | 说明 |
|------|------|------|
| #1 | 分层依赖方向 | Service → Biz ← Data，禁止反向依赖 |
| #2 | Biz 定义接口 | Biz 层定义 Repo 接口，Data 层实现 |
| #3 | 禁止 log/slog | 统一使用 `pkg/loggateway.Logger` |
| #4 | 禁止 loggateway.Global() | 新代码必须构造注入 Logger |
| #5 | Wire DI | 所有依赖通过 Wire 注入，禁止手动创建 |
| #6 | Proto 优先 | API 变更必须先改 Proto，再生成代码 |
| #7 | 只改相关文件 | 不顺带 refactor 相邻模块 |
| #8 | 最小代码原则 | 不添加未请求的功能/抽象 |
| #9 | sddflow 流程 | 新变更必须走 brainstorming → spec → build → close |
| #10 | TDD 铁律 | 先写失败测试，再写最小实现 |
