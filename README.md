# 🏗️ Aranea-Agents 项目分析文档

> 基于项目源码的全面架构分析报告

---

## 一、项目概述

**Aranea-Agents** 是一个企业级 **AI Agent 平台**，提供完整的 Agent 生命周期管理、多模态对话、团队协作编排和记忆系统。

### 技术栈概览

| 层面 | 技术 | 版本 |
|------|------|------|
| 后端框架 | Go Kratos | v2.9.2 |
| Agent 运行时 | trpc-agent-go + Google ADK | - |
| 数据库 | SQLite (Ent ORM) + PostgreSQL (pgvector) | - |
| 前端框架 | Vue 3 + Quasar | Vue ^3.5 / Quasar ^2.19 |
| 构建工具 | Vite | ^6.3 |
| 状态管理 | Pinia | ^3.0 |
| 路由 | Vue Router | ^4.5 |

---

## 二、后端架构详解

### 2.1 Kratos 分层架构

项目严格遵循 Kratos DDD 分层模式：

```
┌─────────────────────────────────────────────────────────┐
│                    API Layer                            │
│   api/kratos/*/v1/*.proto                              │
│   (Protobuf 定义 HTTP/gRPC 接口)                        │
├─────────────────────────────────────────────────────────┤
│                    Service Layer                        │
│   internal/service/                                    │
│   (实现 proto 定义的 RPC，DTO↔DO 转换)                 │
├─────────────────────────────────────────────────────────┤
│                    Biz Layer                            │
│   internal/biz/                                        │
│   (业务逻辑核心，Usecase 编排，Repository 接口定义)     │
├─────────────────────────────────────────────────────────┤
│                    Data Layer                           │
│   internal/data/                                       │
│   (Repository 实现，Ent ORM + 原生 SQL)                │
├─────────────────────────────────────────────────────────┤
│                    Server Layer                         │
│   internal/server/                                     │
│   (HTTP/gRPC/SSE Server 创建与服务注册)                 │
└─────────────────────────────────────────────────────────┘
```

### 2.2 核心目录结构

```
aranea-agents/
├── api/kratos/           # Proto API 定义 (17+ 模块)
│   ├── agent/v1/         # Agent CRUD + 运行时设置
│   ├── chat/v1/          # 聊天消息接口
│   ├── session/v1/       # 会话管理
│   ├── team/v1/          # 团队编排
│   ├── memory/v1/        # 五层记忆系统
│   ├── tool/v1/          # 工具管理
│   ├── skill/v1/         # 技能管理
│   ├── plugin/v1/        # 插件管理
│   ├── mcp_server/v1/    # MCP 服务器
│   ├── channel/v1/       # 外部渠道
│   ├── cron/v1/          # 定时任务
│   ├── monitor/v1/       # 监控日志
│   └── ...
├── cmd/admin/            # 应用入口
│   ├── main.go           # 启动文件
│   └── wire.go           # Wire 依赖注入
├── configs/              # 配置文件
│   └── config.yaml       # 服务配置
├── internal/             # 核心业务代码
│   ├── agent/            # Agent 运行时 (ADK + trpc)
│   ├── biz/              # 业务用例
│   ├── data/             # 数据访问
│   ├── server/           # 传输层
│   ├── service/          # 服务实现
│   ├── team/             # 团队运行器
│   ├── tool/             # 工具注册
│   └── ...
├── pkg/trpc-agent-go/    # 腾讯 trpc-agent-go 框架
└── web/                  # Vue 3 前端
```

### 2.3 传输层配置

| 服务 | 端口 | 用途 |
|------|------|------|
| HTTP | `:8000` | REST API |
| gRPC | `:9000` | gRPC API |
| SSE | `:8001` | 实时事件流（Team Run、Monitor） |

### 2.4 数据存储架构

```
┌─────────────────────────────────────────────┐
│           SQLite (Ent ORM)                  │
│   - Agent / Session / Team / Tool / Skill   │
│   - Plugin / Cron / Hook / Admin            │
│   - L0-L2 Memory (感官/工作/情景记忆)       │
├─────────────────────────────────────────────┤
│           PostgreSQL (pgvector)             │
│   - L3 Memory (语义记忆向量化存储)           │
│   - 向量检索 (Embedding 1536维)             │
├─────────────────────────────────────────────┤
│           Redis (可选)                      │
│   - 缓存层                                  │
└─────────────────────────────────────────────┘
```

### 2.5 五层记忆系统

| 层级 | 名称 | 存储 | 功能描述 |
|------|------|------|----------|
| **L0** | 感官记忆 | SQLite | 最近对话窗口、上下文压缩快照 |
| **L1** | 工作记忆 | SQLite | 当前任务/目标追踪 |
| **L2** | 情景记忆 | SQLite | 事件片段、重要性评分 |
| **L3** | 语义记忆 | pgvector | 向量化知识检索 |
| **L4** | 持久记忆 | SQLite | 知识图谱、身份信息 |

---

## 三、trpc-agent-go 框架分析

### 3.1 框架定位

**trpc-agent-go** 是腾讯开源的 Go 语言 AI Agent 框架，本项目通过 `replace` 指令引入本地开发版本：

```go
replace trpc.group/trpc-go/trpc-agent-go => ./pkg/trpc-agent-go
```

### 3.2 核心模块

| 模块 | 职责 | 关键文件 |
|------|------|----------|
| **agent** | Agent 核心抽象 | `agent.go`, `invocation.go`, `callbacks.go` |
| **graph** | 图执行引擎 | `graph.go`, `executor.go`, `state.go`, `checkpoint.go` |
| **model** | LLM 模型抽象 | `model.go`, `registry.go`, `request.go` |
| **session** | 会话管理 | `session.go`, `state.go` |
| **runner** | 运行器 | `runner.go` |
| **team** | 多 Agent 团队 | `team.go`, `swarm.go` |
| **memory** | 记忆系统 | `memory.go`, `tool/` |
| **event** | 事件系统 | `event.go` |

### 3.3 Graph 执行引擎特性

- **Pregel-style BSP**：批量同步并行执行模型
- **状态流转**：`map[string]any` 类型的状态在节点间传递
- **检查点**：支持执行状态保存与恢复
- **时间旅行**：支持执行过程回溯
- **人机中断**：支持执行中断与恢复（Interrupt/Resume）

### 3.4 Team 编排模式

| 模式 | 说明 |
|------|------|
| **Coordinator** | 协调者 Agent 调度成员 Agent 作为工具调用 |
| **Swarm** | 成员 Agent 之间通过 `transfer_to_agent` 自由传递控制权 |

---

## 四、前端架构

### 4.1 项目结构

```
web/src/
├── pages/              # 页面组件 (23+ 页面)
│   ├── ChatPage.vue         # 聊天页面
│   ├── AgentsPage.vue       # Agent 列表
│   ├── SessionsPage.vue     # 会话管理
│   ├── TeamsPage.vue        # 团队编排
│   ├── MemoryCenterPage.vue # 记忆中心
│   ├── SkillsPage.vue       # 技能管理
│   ├── ToolsPage.vue        # 工具管理
│   └── MonitorPage.vue      # 监控页面
├── features/           # 功能模块
│   ├── chat/                # 聊天功能
│   ├── agents/              # Agent 管理
│   ├── session/             # 会话管理
│   ├── memory/              # 记忆系统
│   └── ...
├── components/         # 通用组件
├── stores/             # Pinia 状态管理
├── services/           # API 客户端 (Proto 自动生成)
├── router/             # 路由配置
└── i18n/               # 国际化 (中/英)
```

### 4.2 路由配置

| 路径 | 页面 | 功能 |
|------|------|------|
| `/` | Overview | 概览 |
| `/chat` | ChatPage | 聊天对话 |
| `/agents` | AgentsPage | Agent 列表 |
| `/agents/:id/settings` | AgentSettingsPage | Agent 设置 |
| `/sessions` | SessionsPage | 会话管理 |
| `/memory` | MemoryCenterPage | 记忆中心 |
| `/team` | TeamsPage | 团队编排 |
| `/skills` | SkillsPage | 技能管理 |
| `/tools` | ToolsPage | 工具管理 |
| `/monitor` | MonitorPage | 监控 |

---

## 五、核心业务模块

### 5.1 模块清单

| 模块 | 功能描述 |
|------|----------|
| **Agent** | Agent CRUD、运行时设置、提示词文件管理 |
| **Chat** | 消息发送、对话模式、流式响应 |
| **Session** | 会话创建/查询/归档、时间线展示 |
| **Team** | 团队定义、成员管理、编排执行 |
| **Memory** | L0-L4 五层记忆读写 |
| **Tool** | 工具注册、权限策略、执行记录 |
| **Skill** | 技能导入、验证、运行 |
| **Plugin** | 插件安装、启用/禁用 |
| **MCP Server** | MCP 服务器配置、端点管理 |
| **Channel** | 外部消息渠道（飞书等） |
| **Cron** | 定时任务定义、执行记录 |
| **Hook** | 钩子事件、触发配置 |
| **Monitor** | 日志流、审计追踪 |
| **Usage** | 用量统计、成本核算 |

---

## 六、Agent 运行时架构

### 6.1 双运行时模式

项目支持两种 Agent 运行时：

1. **Google ADK 模式**
   - 使用 `google.golang.org/adk` 包
   - 通过 `NewADKRunner()` 创建
   - 适用于标准 ADK 场景

2. **trpc-agent-go 模式**
   - 使用 `trpc.group/trpc-go/trpc-agent-go`
   - 通过 `NewTRPCRunner()` 创建
   - 支持复杂图执行和团队编排

### 6.2 执行流程

```
用户请求 → ChatService → Agent 构建 → Runner 执行 → Event 流 → 响应
                              ↓
                    BuildLLMAgent(ctx, agent, deps)
                              ↓
                    注入: Provider/Model/Tools/Memory/Artifacts
```

---

## 七、开发与部署

### 7.1 环境要求

- Go 1.25+
- Node.js 20+
- SQLite 3+
- PostgreSQL 14+ (可选，用于向量存储)
- Redis 7+ (可选)

### 7.2 构建命令

```bash
# 初始化工具
make init

# 生成 Proto 代码
make api

# 构建后端
make build

# 启动开发服务器
cd cmd/admin && go run . -conf ../../configs
```

### 7.3 前端开发

```bash
cd web
npm install
npm run serve
```

---

## 八、架构亮点

1. **双 Agent 运行时**：同时支持 Google ADK 和 trpc-agent-go，灵活适配不同场景
2. **五层记忆系统**：完整的记忆层次架构，从短期到长期记忆
3. **Graph 执行引擎**：支持复杂工作流编排、检查点、时间旅行
4. **Team 多 Agent 协作**：Coordinator 和 Swarm 两种编排模式
5. **Proto 全链路**：API 定义到前端客户端自动生成
6. **SSE 实时流**：实时事件推送（团队执行、监控日志）
7. **混合数据库**：SQLite + pgvector + Redis 的组合存储方案

---

## 九、技术文档索引

| 文档 | 路径 |
|------|------|
| 平台架构 | `docs/design/platform-architecture.md` |
| 需求文档 | `docs/需求/` |
| 开发规范 | `docs/guides/` |
| trpc-agent-go 文档 | `pkg/trpc-agent-go/docs/` |

---

*Generated on 2026-05-12*
