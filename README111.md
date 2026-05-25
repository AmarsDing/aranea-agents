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

$env:ARANEA_TEAM_GRAPH_RUNTIME = "1"
# 开发模式 A：免登录（最快）
$env:DEPLOY_ENV="dev"
$env:KRATOS_HTTP_AUTH_DISABLED="1"
go run ./cmd/admin -conf ./configs/config.yaml

# 开发模式 B：真实 Cookie 登录（与生产一致）
# $env:DEPLOY_ENV="dev"
# $env:KRATOS_AUTH_SECRET="local-dev-only-change-me-32chars-minimum"
# go run ./cmd/admin -conf ./configs

# 自检：curl http://localhost:8000/healthz  → auth_mode: bypass | jwt
```

本地账号（模式 A 或 B）：**`dev` / `dev`**（bypass 时自动种子）。

**Ctrl+C 无法退出**（多见于 Windows + Cursor 终端）：再按一次 Ctrl+C 强制退出；或 `netstat -ano | findstr :8000` 查 PID 后 `taskkill /PID <pid> /F`。

**WebSocket**（聊天流式、监控）：走 **HTTP 同端口** `ws://<host>:8000/v1/ws`（开发时经 Quasar 代理为 `ws://localhost:9001/v1/ws`）。`config.yaml` 里的 `server.ws.addr:8002` 为历史字段，当前实现挂在 Kratos HTTP 上，**不要**单独连 8002。

认证设计详见 [docs/需求/admin-auth.design.md](docs/需求/admin-auth.design.md)。

### 前端

```bash
cd web
npm install
npm run dev    # http://localhost:9001（勿用 :9000，该端口为 gRPC）
```
channel 图标获取
go run ./cmd/fetch-channel-icons

页面须使用 **http://localhost:9001**，API/WS 经 Vite 代理到 `:8000`，会话 **HttpOnly Cookie** 才会自动携带。

照文档 @docs/README.md，进行review,评级，注重 代码质量  业务逻辑   架构与设计模式，代码可读性与风格，错误处理与健壮性，影响范围与回归风险

照文档 @docs/README.md，进行优化，优化时考虑 代码质量  业务逻辑   架构与设计模式，代码可读性与风格，错误处理与健壮性，影响范围与回归风险
----------





架构设计审查（AI 辅助，人工决策）
分层架构合理性：是否遵循单一职责、依赖倒置原则
模块边界划分：接口设计是否清晰，是否存在循环依赖
技术选型匹配度：是否使用了合适的技术栈解决当前问题
扩展性与可复用性：是否为未来需求预留了合理的扩展点
系统耦合度：是否存在过度耦合或过度设计
数据流向：数据在系统中的传递是否清晰、安全

2. 代码质量与风格
编码规范：命名规范、缩进、空格、括号风格、文件组织
代码简洁性：是否存在重复代码、冗余逻辑、死代码
可读性：变量 / 函数命名是否表意清晰，代码结构是否直观
复杂度控制：函数 / 类是否过大，圈复杂度是否超标
最佳实践：是否遵循语言和框架的最佳实践
代码异味：是否存在魔法数字、硬编码、过长参数列表等问题
3. 功能正确性验证（AI 辅助，人工确认）
需求匹配度：代码是否准确实现了产品需求
边界条件处理：空值、极值、异常输入是否正确处理
逻辑分支覆盖：所有 if/else、switch 分支是否都正确
并发正确性：多线程 / 协程场景下是否存在竞态条件
数据一致性：数据库操作、缓存更新是否保证数据一致
算法正确性：核心算法是否实现正确，是否有更优解

4. 性能与资源效率（AI 辅助，人工验证）
算法复杂度：时间复杂度和空间复杂度是否合理
数据库性能：是否存在慢查询、N+1 查询、未使用索引
内存使用：是否存在内存泄漏、不必要的大对象创建
网络请求：是否存在重复请求、长连接未释放
资源释放：文件句柄、数据库连接、网络连接是否正确释放
批量处理：是否可以通过批量操作提升性能

7. 可维护性审查（AI 辅助）
代码模块化：是否将复杂逻辑拆分为小的、可复用的函数 / 类
注释质量：是否有必要的注释，注释是否准确、不过时
变更影响：本次修改的影响范围是否可控
向后兼容：是否破坏了现有 API 或功能
技术债务：是否引入了新的技术债务，是否有偿还计划
8. 错误处理与鲁棒性（AI 主力）
异常捕获：是否捕获了正确的异常，是否存在空捕获
错误信息：错误信息是否清晰、有用，便于排查问题
降级策略：关键功能是否有降级和熔断机制
重试机制：重试逻辑是否合理，是否存在无限重试
幂等性：接口和操作是否保证幂等性
9. 兼容性审查（AI 辅助）
版本兼容性：是否兼容旧版本的客户端 / 服务端
平台兼容性：是否在目标平台上都能正常运行
浏览器兼容性：前端代码是否兼容目标浏览器
数据库兼容性：是否兼容目标数据库版本
依赖兼容性：第三方依赖版本是否兼容
10. 合规与规范审查（AI 完全胜任）
许可证合规：第三方依赖的许可证是否符合公司要求
代码所有权：是否存在抄袭或未授权的代码
公司规范：是否遵循公司内部的编码规范和流程
数据合规：是否符合 GDPR、个人信息保护法等法规要求
11. 业务逻辑审查（必须人工，AI 几乎无法胜任）
业务规则准确性：是否正确实现了复杂的业务规则
业务流程合理性：代码是否符合实际业务流程
业务风险：是否存在业务层面的风险和漏洞
领域模型：领域模型是否准确反映了业务概念
产品体验：代码实现是否符合产品设计的用户体验
----------


 架构设计审查（AI 辅助，人工决策）
分层架构合理性：是否遵循单一职责、依赖倒置原则
模块边界划分：接口设计是否清晰，是否存在循环依赖
技术选型匹配度：是否使用了合适的技术栈解决当前问题
扩展性与可复用性：是否为未来需求预留了合理的扩展点
系统耦合度：是否存在过度耦合或过度设计
数据流向：数据在系统中的传递是否清晰、安全
2. 代码质量与风格
编码规范：命名规范、缩进、空格、括号风格、文件组织
代码简洁性：是否存在重复代码、冗余逻辑、死代码
可读性：变量 / 函数命名是否表意清晰，代码结构是否直观
复杂度控制：函数 / 类是否过大，圈复杂度是否超标
最佳实践：是否遵循语言和框架的最佳实践
代码异味：是否存在魔法数字、硬编码、过长参数列表等问题
3. 功能正确性验证（AI 辅助，人工确认）
需求匹配度：代码是否准确实现了产品需求
边界条件处理：空值、极值、异常输入是否正确处理
逻辑分支覆盖：所有 if/else、switch 分支是否都正确
并发正确性：多线程 / 协程场景下是否存在竞态条件
数据一致性：数据库操作、缓存更新是否保证数据一致
算法正确性：核心算法是否实现正确，是否有更优解
4. 性能与资源效率（AI 辅助，人工验证）
算法复杂度：时间复杂度和空间复杂度是否合理
数据库性能：是否存在慢查询、N+1 查询、未使用索引
内存使用：是否存在内存泄漏、不必要的大对象创建
网络请求：是否存在重复请求、长连接未释放
资源释放：文件句柄、数据库连接、网络连接是否正确释放
批量处理：是否可以通过批量操作提升性能
5. 安全性审查（AI 主力，人工复核高危项）
注入攻击：SQL 注入、XSS、CSRF、命令注入
认证与授权：权限控制是否正确，是否存在越权漏洞
敏感数据处理：密码、密钥、个人信息是否加密存储和传输
输入验证：所有外部输入是否都进行了严格验证
依赖安全：第三方依赖是否存在已知漏洞
日志安全：是否泄露敏感信息到日志中
6. 可测试性审查（AI 辅助）
单元测试覆盖：核心逻辑是否有足够的单元测试
测试代码质量：测试用例是否清晰、有效，是否存在重复
依赖注入：是否使用依赖注入便于 mock 测试
可观测性：是否有合适的日志、指标、链路追踪
异常测试：是否测试了异常场景和错误路径
7. 可维护性审查（AI 辅助）
代码模块化：是否将复杂逻辑拆分为小的、可复用的函数 / 类
注释质量：是否有必要的注释，注释是否准确、不过时
变更影响：本次修改的影响范围是否可控
向后兼容：是否破坏了现有 API 或功能
技术债务：是否引入了新的技术债务，是否有偿还计划
8. 错误处理与鲁棒性（AI 主力）
异常捕获：是否捕获了正确的异常，是否存在空捕获
错误信息：错误信息是否清晰、有用，便于排查问题
降级策略：关键功能是否有降级和熔断机制
重试机制：重试逻辑是否合理，是否存在无限重试
幂等性：接口和操作是否保证幂等性
9. 兼容性审查（AI 辅助）
版本兼容性：是否兼容旧版本的客户端 / 服务端
平台兼容性：是否在目标平台上都能正常运行
浏览器兼容性：前端代码是否兼容目标浏览器
数据库兼容性：是否兼容目标数据库版本
依赖兼容性：第三方依赖版本是否兼容
10. 合规与规范审查（AI 完全胜任）
许可证合规：第三方依赖的许可证是否符合公司要求
代码所有权：是否存在抄袭或未授权的代码
公司规范：是否遵循公司内部的编码规范和流程
数据合规：是否符合 GDPR、个人信息保护法等法规要求
11. 业务逻辑审查（必须人工，AI 几乎无法胜任）
业务规则准确性：是否正确实现了复杂的业务规则
业务流程合理性：代码是否符合实际业务流程
业务风险：是否存在业务层面的风险和漏洞
领域模型：领域模型是否准确反映了业务概念
产品体验：代码实现是否符合产品设计的用户体验
12. 文档与注释审查（AI 辅助）
API 文档：接口文档是否完整、准确
架构文档：是否有必要的架构设计文档
代码注释：关键逻辑是否有清晰的注释
README：项目 README 是否完整，包含运行和部署说明



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
TODO:
1 内置agent

2 graph智能体编排

3 channel 需求细化 测试

4 mcp 需求细化 测试 

5 plugin 需求细化 测试

6 Hook 需求细化  测试

7 制品 需求细化 测试

8 评估管理  需求细化  测试

9 A2A 测试

10 tools 功能完善 测试

11 监控面板需求 UI 完善