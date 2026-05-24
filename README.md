# Aranea-Agents

> 企业级 AI Agent 编排平台 — 基于 Kratos v2 + trpc-agent-go

**Aranea-Agents** 是基于 [trpc-agent-go](./pkg/trpc-agent-go) 的多智能体编排平台。以 Kratos v2 为传输壳层、trpc-agent-go 为运行时内核，提供 Agent / Team / Graph 可视化编排、会话管理、五层记忆、Channel 接入（飞书 / 钉钉等）、Skill / Plugin 治理与多模型接入能力。

- **AI / 贡献者入口**：[AGENTS.md](./AGENTS.md) → [docs/README.md](./docs/README.md)
- **进度与任务板**：[docs/guides/execution-plan.md](./docs/guides/execution-plan.md)
- **模块开发索引**：[docs/需求/README-development.md](./docs/需求/README-development.md)

---

## 技术栈

| 层级 | 选型 |
|------|------|
| 后端 | Go + **Kratos v2**（HTTP / gRPC / WebSocket 传输、Wire DI） |
| Agent 运行时 | **trpc-agent-go**（Runner / Agent / Session / Memory / Tool / Event / Skill / Graph / Team / Planner / Plugin / Artifact / CodeExecutor / Knowledge / Evaluation） |
| 前端 | Vue 3 + Quasar + Pinia + TypeScript |
| 数据库 | SQLite（Ent ORM，单连接池）；向量 / 图可外挂 pgvector |
| 依赖注入 | Wire（编译期），`make wire` 生成；Proto 生成 `make api` |
| 观测 | Prometheus（`/metrics`）+ OTLP Trace/Metrics + FlowLog / Runs 投影 |

---

## 双框架分工

Kratos v2 负责传输层，trpc-agent-go 负责 Agent 编排，二者互不越界。

```
api/**/*.proto          ← 唯一对外契约
        ↓
internal/service        ← 传输桥点：proto ↔ biz 映射 + 框架 Runner 装配
        ↓
internal/biz            ← 领域模型 + Usecase + Repo 接口（禁止 import trpc-agent-go）
        ↓
internal/data           ← Repo 实现（Ent ORM + SQLite）
```

---

## 核心架构

```
┌──────────────────────────────────────────────────────────────┐
│  用户接入层: Web UI / CLI / Channel(飞书等) / A2A / Cron     │
├──────────────────────────────────────────────────────────────┤
│  传输层 (Kratos v2): HTTP :8000 / gRPC :9000 / WS /v1/ws    │
├──────────────────────────────────────────────────────────────┤
│  Service 层: Chat / Agent / Team / Session / Memory / Tool   │
├──────────────────────────────────────────────────────────────┤
│  Biz 层: AgentUsecase / TeamUsecase / MemoryUsecase / ...    │
├──────────────────────────────────────────────────────────────┤
│  Data 层: SQLite (Ent ORM) + PostgreSQL (pgvector，可选)    │
├──────────────────────────────────────────────────────────────┤
│  Agent 运行时 (trpc-agent-go):                                │
│  Runner → Agent/Team/Graph → Memory/Tool/Event/Planner       │
│  → Plugin/Artifact/CodeExecutor/Knowledge/A2A/Callback       │
├──────────────────────────────────────────────────────────────┤
│  模型驱动层: OpenAI/Anthropic/Gemini/Ollama/Hunyuan/Bedrock  │
│  Failover/Hedge 高可用 │ TokenTailor 上下文裁剪               │
└──────────────────────────────────────────────────────────────┘
```

---

## 模块概览

> 详细接入度与任务板见 [execution-plan.md](./docs/guides/execution-plan.md)。

| 等级 | 模块 | 说明 |
|------|------|------|
| **核心可用** | Chat、Agent 全家桶、Provider、Session、Skill、Tools、Cron、Message/WS、Plugin/Callback、Gateway/Runner | 可创建、运行、配置、观测 |
| **可用需闭环** | Team、Graph、MCP、Memory、Channel、Monitor/Token | 主路径可用，生产级治理持续补齐 |
| **有页、Runtime 已通主项** | Knowledge、Artifact、Evaluation、A2A | 管理页与运行时主项已落地 |
| **早期 / 占位** | CLI、TTS、Ecosystem（部分） | 非核心 SLA |

### 五层记忆（L0–L4）

| 层级 | 名称 | 存储 | 功能 |
|------|------|------|------|
| L0 | 感官记忆 | SQLite | 最近对话窗口、上下文压缩快照 |
| L1 | 工作记忆 | SQLite | 当前任务 / 目标追踪 |
| L2 | 情景记忆 | SQLite | 事件片段、重要性评分 |
| L3 | 语义记忆 | pgvector | 向量化知识检索 |
| L4 | 持久记忆 | SQLite | 知识图谱、身份信息 |

---

## 快速开始

### 环境要求

- Go 1.25+
- Node.js 20+
- SQLite 3+
- PostgreSQL 14+（可选，用于向量存储）
- [protoc](https://grpc.io/docs/protoc-installation/)（生成 API 代码时）

### 初始化与构建

```bash
make init      # 安装 protoc 插件、wire、kratos 等工具
make api       # 生成 Proto / OpenAPI / 前端 TS 类型
make wire      # 生成 Wire 依赖注入
make build     # 构建后端
```

### 后端

```bash
# 开发模式 A：免登录（最快）
# Windows PowerShell:
$env:DEPLOY_ENV="dev"
$env:KRATOS_HTTP_AUTH_DISABLED="1"
go run ./cmd/admin -conf ./configs/config.yaml

# Linux / macOS:
# DEPLOY_ENV=dev KRATOS_HTTP_AUTH_DISABLED=1 go run ./cmd/admin -conf ./configs/config.yaml
```

```bash
# 开发模式 B：真实 Cookie 登录（与生产一致）
# $env:DEPLOY_ENV="dev"
# $env:KRATOS_AUTH_SECRET="local-dev-only-change-me-32chars-minimum"
# go run ./cmd/admin -conf ./configs
```

本地账号（模式 A 或 B）：**`dev` / `dev`**（bypass 时自动种子）。

健康检查：

```bash
curl http://localhost:8000/healthz
# 期望 auth_mode: bypass | jwt
```

**WebSocket**（聊天流式、监控）：走 **HTTP 同端口** `ws://<host>:8000/v1/ws`。开发时经 Quasar 代理为 `ws://localhost:9001/v1/ws`。`config.yaml` 中的 `server.ws.addr:8002` 为历史字段，当前实现挂在 Kratos HTTP 上，**不要**单独连 8002。

认证设计详见 [docs/需求/admin-auth.design.md](./docs/需求/admin-auth.design.md)。

### 前端

```bash
cd web
npm install
npm run dev    # http://localhost:9001（勿用 :9000，该端口为 gRPC）
```

页面须使用 **http://localhost:9001**，API / WS 经 Vite 代理到 `:8000`，会话 **HttpOnly Cookie** 才会自动携带。

### Channel 图标（可选）

```bash
go run ./cmd/fetch-channel-icons
```

### Windows 提示

**Ctrl+C 无法退出**（多见于 Windows + Cursor 终端）：再按一次 Ctrl+C 强制退出；或 `netstat -ano | findstr :8000` 查 PID 后 `taskkill /PID <pid> /F`。

---

## 开发与验证

| 命令 | 说明 |
|------|------|
| `make help` | 查看全部 Make 目标 |
| `make test` | 运行 Go 测试 |
| `make lint` | 跨平台 lint + go vet + gofmt |
| `make runtime-boundary` | 检查 Agent 运行时 import 边界 |
| `make ci` | 完整 CI：lint + test + smoke |

**提交 / PR 前**建议全量验证：

```bash
make api && make wire && make build && make test && make lint && make runtime-boundary
cd web && npm run lint && npm test && npm run build
```

编码规范：

| 场景 | 文档 |
|------|------|
| 后端 Go | [docs/guides/AI-DEVELOPMENT-SPECIFICATION.md](./docs/guides/AI-DEVELOPMENT-SPECIFICATION.md) |
| 前端 | [docs/guides/frontend-guide.md](./docs/guides/frontend-guide.md) |
| Kratos 分层 | [docs/guides/kratos-framework-guide.md](./docs/guides/kratos-framework-guide.md) |
| trpc-agent-go | [docs/guides/trpc-agent-go-framework.md](./docs/guides/trpc-agent-go-framework.md) |

---

## 文档导航

| 文档 | 路径 | 说明 |
|------|------|------|
| **文档总入口** | [docs/README.md](./docs/README.md) | AI 工作流、文档索引、分级验证 |
| **系统架构总览** | [docs/需求/0 系统框图.md](./docs/需求/0%20系统框图.md) | 分层架构、数据流、模块依赖 |
| **架构健康度 / 路线图** | [docs/需求/0-system-development.md](./docs/需求/0-system-development.md) | 模块诊断、OpenClaw 对照、开发顺序 |
| **模块解耦架构** | [docs/需求/0-module-decoupling-architecture.md](./docs/需求/0-module-decoupling-architecture.md) | Chat / Channel / Agent 边界 |
| **执行计划** | [docs/guides/execution-plan.md](./docs/guides/execution-plan.md) | 当前迭代、Top 任务、里程碑 |
| **需求与设计** | [docs/需求/](./docs/需求/) | 40+ 模块需求规格与 `*.design.md` |
| **前端 UX** | [docs/frontend/UX.md](./docs/frontend/UX.md) | 日夜双模、玻璃材质、组件规范 |
| **变更记录** | [docs/changelog/](./docs/changelog/) | 按日期归档的实现摘要 |

---

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
│   ├── server/           # 传输层 (HTTP/gRPC/WS)
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
    ├── guides/           # 编码规范与执行计划
    ├── 需求/             # 需求规格 + 设计文档
    ├── changelog/        # 变更记录
    └── frontend/         # 前端设计参考
```

---

## 路线图（摘要）

当前优先级（详见 [execution-plan.md](./docs/guides/execution-plan.md)）：

1. **M55** — Channel ↔ Web 同步、长任务路由、TurnBlock UI
2. **M53** — Team × Graph 编排融合（执行单链收敛）
3. Memory 冲突 / 级联、Graph Webhook / 熔断、Telemetry gRPC 采样
4. Artifact Chat 引用、Evolution 趋势图、Monitor Dashboard 完善

---

## License

See [LICENSE](./LICENSE).
