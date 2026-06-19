---
name: "aranea-coding-guide"
description: "Aranea-Agents 项目统一编码指南。当在本项目编写 Go 后端代码时自动触发，提供架构铁律、分层规范、框架集成、代码探索与验证的完整指导。"
---

# Aranea-Agents 统一编码指南

> **文档地位**：本项目 Go 后端编码的权威规范。`project_rules.md` 为索引 + 全局约束，详细规范只在本 SKILL 中；内容冲突时以 SKILL 为准。
> **通用 Go OOP 规范**：见 `go-oop-guide` SKILL（接口设计、组合嵌入、工厂构造、设计模式等）。
> **前端规范**：见 `aranea-frontend-guide` SKILL，不在本文范围。

---

## 目录

- [第一章：架构总纲](#第一章架构总纲)
- [第二章：27 条红线](#第二章27-条红线含-3-条已降级为编程规范)
- [第三章：代码探索约束](#第三章代码探索约束)
- [第四章：决策树](#第四章决策树)
- [第五章：分层编码规范](#第五章分层编码规范)
- [第六章：Agent 运行时规范](#第六章agent-运行时规范)（含 §6.13 框架反模式清单、§6.14 框架文档导航地图）
- [第七章：项目代码风格](#第七章项目代码风格)
- [第八章：API 与 Proto 规范](#第八章api-与-proto-规范)
- [第九章：模块化设计](#第九章模块化设计)
- [第十章：任务速查卡](#第十章任务速查卡)
- [第十一章：AI 编码自检清单](#第十一章ai-编码自检清单)
- [第十二章：验证命令](#第十二章验证命令)
- [第十三章：模块关联强制检查](#第十三章模块关联强制检查)
- [第十四章：编程规范](#第十四章编程规范)
- [第十五章：业务逻辑正确性约定](#第十五章业务逻辑正确性约定)（状态机/事件可靠性/不变量/边界条件）
- [第十六章：测试约定](#第十六章测试约定)（分层测试/mock 策略/事件流/并发测试）
- [第十七章：架构合理性验证](#第十七章架构合理性验证as-fit-01-落地)（AS-FIT-01 落地，P1 占位）

---

## 第一章：架构总纲

### 1.1 双框架分工

| 框架 | 职责边界 | 禁止 |
|------|----------|------|
| **Kratos v2** | 传输层（HTTP/gRPC/WebSocket）、配置、鉴权、中间件、Wire 依赖注入 | 不承载 Agent 编排、不实现第二套事件循环 |
| **trpc-agent-go** | Agent 编排（Runner/Agent/Session/Memory/Tool/Event/Skill/Graph/Team） | 不直接写业务数据库、不处理 HTTP 路由 |

### 1.2 依赖方向铁律

```
api/**/*.proto          ← 唯一对外契约
        ↓
internal/service        ← 传输桥点：proto ↔ biz 映射 + 框架 Runner 装配
        ↓
internal/biz            ← 领域模型 + Usecase + Repo 接口定义
        ↓
internal/data           ← Repo 实现（Ent ORM + pgvector）
```

**跨层只允许向内依赖。违反即停。**

### 1.3 逐包 import 规则

**Kratos 标准 4 层**：

| 包路径 | ✅ 允许 import | ❌ 禁止 import |
|--------|----------------|----------------|
| `internal/server/*` | `internal/service`、`internal/conf`、kratos、`pkg/auth`、`pkg/validate` | `pkg/trpc-agent-go` / 框架运行时私有 import、`runner.Runner`、`llmagent.New` |
| `internal/biz/*` | stdlib、kratos errors、本仓 biz/data API | `pkg/trpc-agent-go` 任何包、`api/*/v1`、框架运行时 toolset/skill 类型 |
| `internal/service/*` | `internal/biz`、项目扩展模块、框架 Runner/Agent 装配 API | 绕过 `internal/tools` 大量直连拼装底层 `tool` |
| `internal/data/*` | `internal/biz`（实现 Repo 接口）、`internal/conf`、Ent、pgvector | `api/*/v1`、`pkg/trpc-agent-go` |

**项目扩展模块**：

| 包路径 | ✅ 允许 import | ❌ 禁止 import |
|--------|----------------|----------------|
| `internal/agent/*` | `internal/biz`、`internal/provider`、`internal/data/...`（如需）、`internal/session/trpc`、`pkg/trpc-agent-go` | — |
| `internal/team/*` | `internal/biz`、`internal/agent`、`internal/provider`、`internal/tools`、`pkg/trpc-agent-go` | `api/*/v1` |
| `internal/channel/*` | `internal/biz`、`internal/channel/port`、`internal/event` | 对方 Service 具体类型、`api/*/v1` |
| `internal/graph/adapter` | `internal/biz`、`internal/agent`、`internal/event` | 无关业务 Usecase |
| `internal/provider/*` | `internal/biz`、`pkg/trpc-agent-go` / 框架 `model` 适配 | — |
| `internal/tools/*` | `internal/biz`、框架 `tool` API | — |

### 1.4 各包职责映射

| 能力 | 主要包 | 关键函数/类型 |
|------|--------|---------------|
| 会话快照读写 | `internal/session/trpc` | `SQLiteSessionService` |
| Agent 构建 | `internal/agent` | `BuildLLMAgent` |
| LLM 模型驱动 | `internal/provider` | `ModelForProviderModel` |
| 工具注册与装配 | `internal/tools` | `Assemble`、`Registry` |
| 工具适配层 | `internal/tools/trpc` | `BuildToolsets` |
| Runner 内存/插件/用户ID | `internal/agent` | `NewTRPCMemoryService`、`DefaultRunnerPlugins`、`UserIDFromCtx` |
| Team 工作流 | `internal/team` | `BuildWorkflowRoot` + `runner.Run` |
| Agent-as-Tool | `internal/tools` | `AgentToolConfig` → `trpcagenttool.NewTool` |
| MCP ToolSet | `internal/tools` | `MCPServerConfig` → `trpcmcp.NewMCPToolSet` |
| MCP Broker | `internal/tools` | `MCPBrokerConfig` → `trpcmcpbroker.New` |
| 记忆服务 | `internal/memory/trpc` | `NewSQLiteMemoryService` |
| Stream 流式工具 | 框架 `tool.StreamableTool` | 项目层无需额外包 |

---

## 第二章：27 条红线（含 3 条已降级为编程规范）

> 违反即停，不可绕过。其中 #16/#19/#20 已降级为编程规范 CS-B1/CS-B2/CS-B18，保留编号便于追溯。
> 红线分两类：**架构边界**（#1-#9, #12, #15, #17, #18, #27）防止模块耦合越界与框架源码污染；**运行时正确性**（#10, #11, #13, #14, #21-#26）防止生产事故。

| # | 红线 | 正确做法 |
|---|------|----------|
| 1 | `internal/server/*` 不得 new `runner.Runner` 或 `llmagent.New` | Runner 装配只在 `internal/service` |
| 2 | `internal/biz/*` 不得 import `pkg/trpc-agent-go` 任何包 | 框架交互通过 `internal/agent`/`internal/tools` 桥接 |
| 3 | 框架 `plugin` 回调不得直接写数据库 | 经 broker/async 异步写 |
| 4 | 不得绕过 `internal/session/trpc` 把 Ent 行塞进 `session.Event` | 通过 `session/trpc` 适配 |
| 5 | 不得在 transport 层解析工具参数或拼接 prompt | 工具装配在 `internal/tools`，prompt 在 `internal/agent` |
| 6 | 不得为框架运行时另起独立 HTTP 监听 | 框架运行时复用 Kratos HTTP Server |
| 7 | 不得把 Kratos middleware 逻辑复制进 `pkg/trpc-agent-go` | 中间件只在 `internal/server` |
| 8 | `internal/biz` 不得直接依赖框架运行时 toolset/skill 类型 | 通过 `internal/tools` 的 biz 友好接口 |
| 9 | Service 层不得写业务逻辑 | Service 只做 proto↔biz 映射 + Runner 编排 |
| 10 | 不得在 `NewData` 外另开 SQLite `sql.Open` | 仅通过 `d.RW()`/`d.RWDB()`/`d.Postgres()` 访问，禁止已废弃的 `d.Ent()`/`d.RawDB()`/`d.ReadDB()` |
| 11 | 不得修改工具生成的代码（protoc/wire/Ent 等） | 改源头 → 重新生成 → 提交生成物 |
| 12 | 不得在 Server 层写业务路由或手写 `HandleFunc` | 只做 `Register*HTTPServer`/`Register*ServiceServer` |
| 13 | 所有 `go func()` 必须走 `pkg/safego.Go` / `pkg/safego.GoRecover` | 禁止裸 `go func()` 不处理 panic |
| 14 | 不得在 biz 层使用 `fmt.Errorf` 或 `errors.New` 返回业务错误 | 统一使用 `apierror.BadRequest/NotFound/Internal/...`（见 §7.1） |
| 15 | 非 Service 层不得 import `api/*/v1` proto 包 | proto 映射只在 Service 层；biz 定义端口接口 |
| 16 | 禁止使用 `log/slog` 记录日志 → **编程规范 CS-B1** | 统一使用 `pkg/loggateway.Logger`（`lg.Info/Warn/Error` + `loggateway.StepID/Err/Str`）；`event.SysLog*` / `event.SessionSysLog*` 已废弃 |
| 17 | 跨模块调用不得持有对方 Service 具体类型 | 通过 biz 级窄接口（端口）交互，Wire 绑定在 Service 层 |
| 18 | Graph 运行时类型不得泄漏到 biz | biz 暴露 `GraphBuildConfig`/`GraphRuntime`/`GraphExecutor` 端口 |
| 19 | 不得新增已无调用者的 deprecated 方法 → **编程规范 CS-B2** | 死代码即删，不保留 Deprecated 标记 |
| 20 | 禁止过度设计 → **编程规范 CS-B18** | YAGNI：单一实现不预抽接口；未请求的配置项/扩展点不添加；三处复用前不提取公共函数 |
| 21 | 共享 map/slice 并发读写必须同步 | 用 `sync.Map`/`sync.Mutex`/`chan`；并发访问的 struct 字段必须加锁或用 atomic |
| 22 | 禁止 `_ =` 忽略 error 返回值 | 除 `io.Close`/`fmt.Fprintf` 等明确无副作用的调用外，error 必须处理或显式注释忽略原因 |
| 23 | goroutine 必须有退出路径 | 通过 `ctx.Done()`/`select`/明确 done channel 退出；禁止无退出条件的 `for {}` 循环 |
| 24 | 跨表/跨 Repo 写操作必须包事务 | 用 `Data.ExecInTx` 或 biz 层 `XxxTxRepo.ExecInTx`，禁止跨 Repo 多次独立写不包事务 |
| 25 | 禁止日志输出敏感字段明文 | credential/api_key/password/token/secret 等用 `loggateway.Redacted()` 或标记 `.Sensitive()`，禁止 `Str("key", value)` 直出 |
| 26 | 外部输入/接口返回值使用前必须 nil 检查 | proto 请求字段、Repo 返回值、第三方 API 响应使用前判 nil；指针解引用前必须确认非 nil |
| 27 | 不得修改 `pkg/trpc-agent-go` 框架源码 | 框架视为只读依赖；能力扩展通过适配层（`internal/agent`/`internal/tools`/`internal/memory`/`internal/session` 等）实现；如需框架本身改动，提 issue/PR 到上游框架仓库，禁止在本仓内直接改框架源码 |

> **降级说明**：红线 #16（log/slog）→ CS-B1、#19（deprecated 方法）→ CS-B2、#20（YAGNI 过度设计）→ CS-B18 已降级为编程规范（见第十四章），因可通过 linter/静态分析约束或属可维护性问题，不属于运行时正确性违反。红线编号不变，但违反级别从"阻断"降为"建议"。
>
> **运行时红线重点**：#21-#26 是生产事故高发区，违反会直接导致 panic/数据竞争/数据不一致/敏感信息泄漏。审查时这些红线优先级高于架构边界红线。

---

## 第三章：代码探索约束

> 本项目已配置 CodeGraph MCP。**编码前先查结构，禁止盲目 grep 扫库。**

| # | 约束 | 说明 |
|---|------|------|
| C1 | 结构性查询**必须优先 CodeGraph** | 符号定义、调用链、影响面、模块上下文 → `codegraph_*` 工具 |
| C2 | **禁止**按符号名 grep 先于 CodeGraph | `codegraph_search` 一次返回 kind + 位置 + 签名 |
| C3 | **禁止**用 grep 重复验证 CodeGraph 结构结果 | AST 索引为准；浪费 token 且更易漏 |
| C4 | grep / Read **仅用于**非结构场景 | 字符串字面量、注释、日志文案；或已定位文件内的局部阅读 |
| C5 | 需要模块全貌时用 `codegraph_context` 或 `codegraph_explore` | 不要 `codegraph_search` + 多次 Read 拼装 |
| C6 | 索引缺失时先问用户是否 `codegraph init -i` | 未初始化前可退回 grep，但应提示初始化 |

**工具选型速查**：

| 问题 | 工具 |
|------|------|
| "X 定义在哪？" / "查找符号 X" | `codegraph_search` |
| "谁调用了函数 Y？" | `codegraph_callers` |
| "Y 调用了什么？" | `codegraph_callees` |
| "改 Z 会影响什么？" | `codegraph_impact` |
| "看 Y 的签名/源码" | `codegraph_node` |
| "获取任务/领域的聚焦上下文" | `codegraph_context` |
| "调研陌生模块/主题" | `codegraph_explore` |

---

## 第四章：决策树

### 4.1 代码该放哪？

```
你要做什么？
│
├─ 新增 HTTP/gRPC 接口 ──────────→ api/**/*.proto → internal/service → internal/server
│
├─ 新增业务逻辑 ─────────────────→ internal/biz（模型 + Repo 接口 + Usecase）
│
├─ 新增数据库表/查询 ────────────→ internal/data/ent/schema → go generate → internal/data
│
├─ 新增 LLM Agent 能力 ──────────→ internal/agent（BuildLLMAgent 扩展）
│
├─ 新增工具 ─────────────────────→ internal/tools（Registry 注册 + Assemble 装配）
│
├─ 新增 Team 工作流 ─────────────→ internal/team（BuildWorkflowRoot）
│
├─ 新增 LLM 厂商 ────────────────→ internal/provider（实现 model.LLM）
│
├─ 新增记忆能力 ─────────────────→ internal/memory（适配器 → trpcmemory.Service）
│
├─ 新增横切关注点（鉴权/中间件）→ internal/server + pkg/auth
│
└─ 新增前端页面 ─────────────────→ 见 docs/guides/frontend-guide.md
```

### 4.2 OOP 抽象选型

> 通用 Go OOP 规范（接口设计、组合嵌入、设计模式等）见 `go-oop-guide` SKILL。

```
需要多态？         → 接口
需要代码复用？     → 组合（嵌入 struct）
需要解耦模块？     → 端口-适配器（接口在 biz，实现在 data）
需要灵活构造？     → Functional Options
需要横切关注点？   → 中间件/装饰器
只在本包用？       → 不需要接口，直接用 struct
```

---

## 第五章：分层编码规范

### 5.1 Service 层——传输桥点

**职责**：实现 proto 接口，做 proto ↔ biz 类型映射，编排框架 Runner 调用。

**结构体模板**：

```go
type XxxService struct {
    v1.UnimplementedXxxServiceServer
    uc *biz.XxxUsecase
}

func NewXxxService(uc *biz.XxxUsecase) *XxxService {
    return &XxxService{uc: uc}
}
```

**编码规则**：

| 规则 | ✅ 正确 | ❌ 错误 |
|------|---------|---------|
| 嵌入 Unimplemented | `v1.UnimplementedXxxServiceServer` | 不嵌入，手写所有方法 |
| 构造函数 | `NewXxxService(uc *biz.XxxUsecase)` | `NewXxxService(uc, repo, db, runner)` |
| 类型转换命名 | `toProtoXxx`（biz→proto）、`fromProtoXxx`（proto→biz） | 在方法内内联转换逻辑 |
| 错误映射 | biz 错误直接透传（APIToKratos 中间件翻译） | `fmt.Errorf("...")` 返回、`mapXxxError` 二次映射 |
| 业务逻辑 | 调 `uc.XxxMethod()` | 在 Service 中写 if/for 业务判断 |

**Runner 装配规则**：

```go
func (s *ChatService) SendChatMessage(ctx context.Context, req *chatv1.SendChatMessageRequest) (*chatv1.SendChatMessageResponse, error) {
    // 1. proto → biz 参数
    // 2. 调 biz Usecase 获取 Agent/Session
    // 3. 调 internal/agent 构建 Agent
    // 4. 调 internal/agent 构建 Runner
    // 5. runner.Run → 事件流 → 投影为 proto 响应
}
```

**桥接约定**：`internal/service` 内 Kratos service 在方法中构造框架 `Runner`，将 RPC/HTTP 请求译为会话执行入口，将会话事件流投影为 unary 或 WebSocket。**不在 `internal/server` 或 `internal/biz` 中直接使用框架运行时。**

### 5.2 Biz 层——领域核心

**职责**：定义领域模型、Usecase 编排、Repo 接口。

1. **模型定义**：纯 Go struct，字段用基本类型，不用 proto 类型
2. **Repo 接口定义在 biz**，data 层实现
3. **Usecase 结构**：只接收接口或具体依赖，不接收"上帝对象"
4. **错误处理**：统一使用 `apierror`，禁止 `fmt.Errorf` 和 `errors.New` 作为最终返回值（见 §7.1）
5. **分页**：统一使用 `biz.ListOption` + `pagination.go`
6. **禁止 import**：`api/*/v1`、`pkg/trpc-agent-go` 任何包

### 5.3 Data 层——数据访问

**职责**：实现 biz 定义的 Repo 接口，封装数据库操作。

1. **数据库访问**：仅通过 `d.RW()`（Ent）/ `d.RWDB()`（Raw SQL）/ `d.Postgres()`，禁止另开连接（红线 #10、DB-R6）
   - Ent 读：`d.RW().Read(ctx)`；Ent 写：`d.RW().Write(ctx)`
   - Raw SQL 读：`d.RWDB().ReadDB(ctx)`；Raw SQL 写：`d.RWDB().WriteDB(ctx)`
   - **禁止使用已废弃访问器**：`d.Ent()`/`d.RawDB()`/`d.ReadDB()`/`d.ReadEnt()`/`d.ReadClient()` 已废弃
2. **Ent 转换函数**：`entXxxToBiz` / `bizXxxToEnt`，放在对应 Repo 文件中
3. **编译期检查**：`var _ biz.XxxRepo = (*xxxRepo)(nil)`

### 5.4 数据库编码规范（SQLite + Ent ORM）

> 源自 `docs/sqlite问题和解决方案.md` 的核心设计原则，所有 data 层开发必须遵守。

#### 5.4.1 Schema 管理

- **单一 Schema 真相源**：所有表必须进 Ent Schema，`go generate` 是唯一的 Schema 演进方式
- **禁止野生表**：不得在 Ent Schema 之外通过 Raw SQL 创建新表
- **Ent 不支持的特性**（FTS5、pgvector、`BEGIN IMMEDIATE`）：在 Ent Schema 中标注 `Annotations`，用 Raw Query 补充但不另建表
- **新增数据库表/查询**：`internal/data/ent/schema` → `go generate ./internal/data/ent` → `internal/data`

#### 5.4.2 数据访问模式

**Raw SQL → Ent Repo 迁移策略**：

| 场景 | 方案 |
|------|------|
| 简单 CRUD | 直接替换为 Ent API |
| 复杂查询 | 保留 `ent.Client.QueryContext()`，但用 Ent 生成的类型做结果映射 |
| SQLite 特有语法 | 通过 Ent 的 Raw Query + 类型映射保留 |

**Ent 无法覆盖的场景**：

| 场景 | 方案 |
|------|------|
| `ON CONFLICT DO UPDATE WHERE` | Ent 的 `OnConflictColumns` + `UpdateSet` |
| `INSERT OR IGNORE` | Ent 的 `OnConflictColumns` + 不更新 |
| `json_set()`/`json_remove()` | 保留 Raw SQL，但封装为 Repo 方法 |
| FTS5 全文搜索 | 保留 Raw SQL（Ent 不支持 FTS5） |
| pgvector 向量搜索 | 保留 Raw SQL（Ent 不支持向量） |
| `BEGIN IMMEDIATE` | 保留 Raw SQL（Ent 不支持事务隔离级别） |
| 50+ 列大表 | Ent 生成后用 `SetXxx()` 链式调用 |

#### 5.4.3 Repo 接口规范

- **方法数上限**：每个 Repo 接口 ≤ 5 方法（编程规范 CS-B4）
- **拆分维度**：按读写职责拆分（`XxxReader`/`XxxWriter`），或按业务子域拆分（`TeamRunRepo`/`OrchestrationStepRepo`）
- **Wire 绑定**：按需注入窄接口，消费方只看到自己需要的方法
- **接口定义位置**：biz 层定义接口，data 层实现

#### 5.4.4 事务管理

- **统一事务接口**：一套 `TransactionManager` 覆盖 Ent + Raw SQL，通过 context 传播事务对象
- **Raw SQL Repo 从 ctx 获取事务**：优先使用 `TxExecerFromCtx(ctx)` 获取已开启的事务，无事务时回退到 `d.RWDB().ReadDB(ctx)`/`d.RWDB().WriteDB(ctx)`
- **压缩操作**：必须通过 CAS + 事务保证原子性（`TryIncrementCompressVersion` + `CompressSessionInTx`）

#### 5.4.5 读写分离

- **SQLite 双连接**：写连接 `entClient`（`MaxOpenConns=1`），读连接 `readClient`（`MaxOpenConns=2`）
- **Ent Repo**：读用 `d.RW().Read(ctx)`，写用 `d.RW().Write(ctx)`
- **Raw SQL Repo**：读用 `d.RWDB().ReadDB(ctx)`，写用 `d.RWDB().WriteDB(ctx)`
- **事务内访问**：使用 `EntClientFromCtx(ctx)` / `TxExecerFromCtx(ctx)` 从 context 获取事务连接
- **连接收口**：不得在 `NewData` 外另开 SQLite 连接（红线 #10、DB-R1）
- **禁止使用已废弃访问器**：`d.Ent()`/`d.RawDB()`/`d.ReadDB()`/`d.ReadEnt()`/`d.ReadClient()` 已废弃（DB-R6）

#### 5.4.6 Schema 迁移

- **框架化迁移**：所有 Schema 变更（包括 `ALTER TABLE ADD COLUMN`）纳入统一迁移框架
- **迁移要素**：有版本号、有依赖顺序、可回滚
- **禁止散落 patch**：不得新增 `*_patch.go` 模式的迁移，统一走迁移框架

### 5.5 Server 层——传输注册

**职责**：创建 HTTP/gRPC/WebSocket 实例，注册 Service。

```go
v1.RegisterXxxHTTPServer(srv, svc)
v1.RegisterXxxServiceServer(srv, svc)
```

**禁止**写业务路由：`srv.Route("/v1").HandleFunc("/custom", handler)`

**中间件**：统一在 `NewHTTPServer`/`NewGRPCServer` 中注册。推荐注册顺序：`recovery → tracing → logging → auth`

---

## 第六章：Agent 运行时规范

### 6.1 框架真相源

**`pkg/trpc-agent-go` 是 Agent 框架的唯一真相源。**

| 原则 | ✅ 正确 | ❌ 错误 |
|------|---------|---------|
| 先查框架 API | 查 `pkg/trpc-agent-go` 的 Runner/Agent/Tool API 后再实现 | 在 biz 重写运行时逻辑 |
| 不复制框架 | 调用框架 API | 把框架内部实现整块复制到业务目录 |
| 编排语义归框架 | Runner/Agent/Tool/Session/Event 在框架中 | 在业务包中平行维护编排逻辑 |
| 框架源码只读 | 视 `pkg/trpc-agent-go` 为只读依赖，扩展走适配层 | 直接修改 `pkg/trpc-agent-go` 源码（红线 #27） |

### 6.2 运行时装配层次

```
internal/service        ← Runner 装配入口（调 agent/team/tools）
internal/agent          ← Agent 构建（BuildLLMAgent、Memory、Plugins）
internal/team           ← Team 工作流（BuildWorkflowRoot、Runner）
internal/tools          ← 工具注册中心 + Assemble 装配（Registry + AssemblyConfig）
internal/tools/trpc     ← 向后兼容适配层（ToolsetConfig → AssemblyConfig → Assemble）
internal/mcp/config|probe|metadata|health ← MCP 配置解析、探活、元数据、定时健康检查
internal/agent/tool_assembly.go ← Agent 回合 MCP 解析与 OAuth 头注入
internal/tools/toolset.go      ← MCPToolSet / MCPBroker 装配
internal/tools/skillruntime    ← Skill 工具集解析
internal/tools/skillrouter     ← Skill 检测与分类
internal/tools/custom         ← 自定义工具实现
internal/provider       ← LLM 模型驱动（ModelForProviderModel）
internal/runtimedeps    ← 运行时依赖注入（TurnDeps、Runtime 聚合）
internal/compress       ← L0 上下文压缩（长对话摘要）
internal/memory         ← 会话记忆（SQLite 适配器 → trpcmemory.Service）
internal/session        ← 会话存储（TRPC SessionService 适配）
internal/skill          ← 技能系统（导入、执行、Watch 热重载）
internal/graph          ← 图编排（TRPC Graph Builder）
internal/channel        ← 渠道集成（飞书 Webhook 等）
internal/cronrunner     ← 定时任务（Cron 调度与执行）
internal/llminspect     ← LLM 调试检查（模型连通性探测）
```

### 6.3 Agent 构建规范

**BuildLLMAgent 调用链**：

```go
deps := agent.BuilderDeps{
    Catalog:      s.llmCatalog,
    AgentUC:      s.agentsUC,
    Agents:       s.agents,
    ToolsCatalog: s.toolsCatalog,
    RT:           s.runtime,
    Memory:       agent.RunnerMemoryForRuntime(s.runtime),
    Provider:     provider,
    Model:        model,
}
root, err := agent.BuildLLMAgent(ctx, ag, deps)
runner, err := agent.NewTRPCRunnerForRuntime(root, sessSvc, s.runtime)
eventCh, err := runner.Run(ctx, userID, sessionID, userMessage)
```

| 规则 | 说明 |
|------|------|
| BuilderDeps 是 DTO | 不含框架运行时类型，只含 biz 模型 + 可选依赖标记 |
| Memory 由 Wire 注入 | 通过 `runtimedeps.Runtime.SessionMemory`，不在 Service 手动选择 |
| 工具统一挂载 | 通过 `TurnMount.Attach`，不分散在多处 |

### 6.4 工具装配规范

**核心装配入口**：`internal/tools/toolset.go` 的 `Assemble(ctx, cfg)`

**适配层入口**：`internal/tools/trpc/toolsets.go` 的 `BuildToolsets(ctx, cfg)`

**装配顺序**（在 `Assemble` 内部）：
1. Registry 注册工具（按 enabled 列表匹配，调用 Factory/ToolSetFactory）
2. 带配置覆盖的工具（file→WithBaseDir、geminifetch→WithModel 等）
3. OpenAPI Spec ToolSet
4. workspace_exec 扩展工具
5. AgentTool（`AgentToolConfig` → `trpcagenttool.NewTool`）
6. MCP ToolSet（`MCPServerConfig` → `trpcmcp.NewMCPToolSet`）
7. MCP Broker（`MCPBrokerConfig` → `trpcmcpbroker.New` → `broker.Tools()`）
8. CustomTools

**规则**：

| # | 规则 | ✅ 正确 | ❌ 错误 |
|---|------|---------|---------|
| 1 | 新增工具先注册 | `Registry()` 注册 `ToolRegistration` + `builtin_tools_seed.go` 种子 | 直接在 Service 中手写 tool 实例 |
| 2 | 需配置的工具 | `AssemblyConfig` 增加字段 + `Assemble` 增加覆盖逻辑 | 硬编码配置值 |
| 3 | Chat/Team 共用 | 同一 `BuildToolsets` 逻辑 | Chat 和 Team 各写一套装配 |
| 4 | 工具策略 | biz 层解析为 effective tool keys，tools 层只做框架映射 | tools 层解析 allow/deny 策略 |
| 5 | 适配层职责 | `ToolsetConfig` → `AssemblyConfig` → `Assemble` | 适配层直接拼装底层 tool |

### 6.5 Team 编排规范

| 模式 | 实现方式 | 适用场景 |
|------|----------|----------|
| Coordinator | 协调者 Agent 调度成员作为工具 | 需要中央决策 |
| Swarm | 成员间 `transfer_to_agent` 传递控制权 | 自由协作 |

**编码规则**：
1. Team Runner 在 `internal/team`，不溢出到 service 或 biz
2. 成员 Agent 独立构建：每个成员用自己的 Settings、Skill 策略、MCP 服务器列表
3. 事件流通过 `biz.TeamRunEventBroker` 发布 WebSocket

### 6.6 记忆系统规范

| 组件 | 职责 |
|------|------|
| `memory.Service` | 记忆 CRUD 接口（Add/Update/Delete/Clear/Read/Search） |
| `memory/tool.ToolSet` | 6 个记忆工具（add/search/load/update/delete/clear） |
| `memory/extractor` | 自动提取（LLM 从对话中提取 fact/episode） |

**两种记忆模式**：

| 模式 | 行为 | 接入方式 |
|------|------|----------|
| Agentic | Agent 主动调用 `memory_add`/`memory_search` 等工具 | `llmagent.WithMemoryService(service)` |
| Auto | 对话结束后 LLM 自动提取记忆 | `service.EnqueueAutoMemoryJob(ctx, session)` |

**规则**：

| # | 规则 | ✅ 正确 | ❌ 错误 |
|---|------|---------|---------|
| 1 | MemoryService 由 Wire 注入 | 有 Store → `NewSQLiteMemoryService`；无 → in-memory | Service 手动选择后端 |
| 2 | 记忆工具注入 | `service.Tools()` 返回 6 个 `tool.Tool`，追加到 Agent 工具列表 | 手动构造记忆工具实例 |
| 3 | 用户隔离 | `GetAppAndUserFromContext(ctx)` 获取 app+user 维度隔离 | 不做用户隔离 |
| 4 | 记忆写入 | 经 broker/async 异步写 | 在 plugin 回调中直接写库（红线 3） |

### 6.7 Provider 集成约定

| 原则 | ✅ 正确 | ❌ 错误 |
|------|---------|---------|
| 厂商连接收口 | `internal/provider` 承载初始化、解析、调用 | 在 agent/service 中直接写 HTTP 客户端 |
| 契约对齐 | 入参/出参以 `pkg/trpc-agent-go/model` 为准 | 在业务包中平行维护另一套驱动接口 |
| 新增厂商 | 扩展 `Registry` + 子包实现 `model.LLM` | 在 agent 中硬编码厂商 URL |

### 6.8 Stream 流式工具规范

**框架三层 Tool 接口**：`Tool`（Declaration）→ `CallableTool`（+Call）→ `StreamableTool`（+StreamableCall）

**执行流程**：框架自动根据接口类型分派 Call 或 StreamableCall。流式工具必须以 `FinalResultChunk` 或 `FinalResultStateChunk` 结束。

### 6.9 Agent-as-Tool 与 MCP Broker 规范

**Agent-as-Tool**（`trpcagenttool.NewTool`）：子 Agent 作为工具，支持 SkipSummarization、StreamInner、HistoryScope、ResponseMode 等选项。

**MCP Broker**（`trpcmcpbroker.New`）：4 个运行时发现工具（`mcp_list_servers`、`mcp_list_tools` 等）。`AllowAdHocHTTP` 默认 false。

### 6.10 框架核心接口速查

**Agent 接口**：

```go
type Agent interface {
    Run(ctx context.Context, invocation *Invocation) (<-chan *event.Event, error)
    Tools() []tool.Tool
    Info() Info
    SubAgents() []Agent
    FindSubAgent(name string) Agent
}
```

**Runner 接口**：

```go
type Runner interface {
    Run(ctx, userID, sessionID string, message model.Message, runOpts ...RunOption) (<-chan *event.Event, error)
    Close() error
}
type ManagedRunner interface {
    Runner
    Cancel(requestID string) bool
    RunStatus(requestID string) (RunStatus, bool)
}
type SteerableRunner interface {
    ManagedRunner
    EnqueueUserMessage(requestID string, message model.Message) error
}
```

**Model 接口**：

```go
type Model interface {
    GenerateContent(ctx context.Context, request *Request) (<-chan *Response, error)
    Info() Info
}
```

**Tool 接口**：

```go
type Tool interface { Declaration() *Declaration }
type CallableTool interface { Call(ctx, jsonArgs) (any, error); Tool }
type StreamableTool interface { StreamableCall(ctx, jsonArgs) (*StreamReader, error); Tool }
```

**Memory Service 接口**：

```go
type Service interface {
    AddMemory(ctx, userKey, memory, topics, ...AddOption) error
    UpdateMemory(ctx, memoryKey, memory, topics, ...UpdateOption) error
    DeleteMemory(ctx, memoryKey) error
    ClearMemories(ctx, userKey) error
    ReadMemories(ctx, userKey, limit) ([]*Entry, error)
    SearchMemories(ctx, userKey, query, ...SearchOption) ([]*Entry, error)
    Tools() []tool.Tool
    EnqueueAutoMemoryJob(ctx, sess) error
    Close() error
}
```

### 6.11 框架包 → 项目包对照

| 框架包 | 项目包 | 职责 |
|--------|--------|------|
| `agent/llmagent` | `internal/agent` | Agent 构建（BuildTRPCLLMAgent） |
| `runner` | `internal/agent` | Runner 创建（NewTRPCRunner） |
| `model/*` | `internal/provider` | LLM 模型适配（TRPCModelForProviderModel） |
| `session/*` | `internal/session` | 会话存储适配 |
| `memory/*` | `internal/memory` | 记忆服务适配 |
| `tool/*` | `internal/tools/trpc` | 工具集适配 |
| `skill` | `internal/skill/trpc` | 技能仓库适配 |
| `team` | `internal/team` | Team 编排 |
| `graph` | `internal/graph` | 图编排 |
| `plugin` | `internal/agent` | 插件注册（DefaultRunnerPlugins） |
| `event` | `internal/service` | 事件投影为 Envelope → EventBus → `/v1/ws` |

### 6.12 关键桥接函数

| 桥接函数 | 位置 | 作用 |
|----------|------|------|
| `BuildTRPCLLMAgent` | `internal/agent/trpc_build.go` | biz.Agent → trpcagent.Agent |
| `NewTRPCRunner` | `internal/agent/trpc_runtime.go` | 创建 Runner 并注入 Session/Memory |
| `RunTRPCUserTurn` | `internal/agent/trpc_runtime.go` | 执行一轮用户对话 |
| `TRPCModelForProviderModel` | `internal/provider/trpc_llm.go` | biz Provider 配置 → trpcmodel.Model |
| `BuildWorkflowRoot` | `internal/team/` | biz Team → trpcagent.Agent（Team 模式） |

### 6.13 框架反模式清单（AI 高频踩坑）

> 以下反模式在代码审查中反复出现，AI 编码时必须主动避免。

#### 6.13.1 装配层反模式

| # | 反模式 | 后果 | 正确做法 |
|---|--------|------|----------|
| AP1 | 在 `internal/biz` 直接 `runner.New()` / `llmagent.New()` | biz 依赖框架，违反红线 #2 | 通过 `internal/agent` 桥接，biz 只定义端口 |
| AP2 | 在 `internal/service` 手写 tool 实例 | 工具散落，无法统一管理 | `Registry()` 注册 `ToolRegistration` + `builtin_tools_seed.go` 种子 |
| AP3 | Chat 和 Team 各写一套工具装配逻辑 | 重复代码 + 行为不一致 | 共用 `BuildToolsets(ctx, cfg)` |
| AP4 | 在 Service 手动选择 Memory 后端 | 装配逻辑泄漏到 Service | Wire 注入 `runtimedeps.Runtime.SessionMemory` |
| AP5 | 复制框架 `llmagent` 内部逻辑到业务包 | 双份维护，框架升级即坏 | 调用框架 API，通过桥接函数包装 |

#### 6.13.2 运行时反模式

| # | 反模式 | 后果 | 正确做法 |
|---|--------|------|----------|
| AP6 | 在 plugin 回调同步写数据库 | 阻塞 Runner 事件循环，性能崩塌 | 经 broker/async 异步写（红线 #3） |
| AP7 | `StreamableTool` 不发 `FinalResultChunk` | 流不结束，客户端永久等待 | 必须以 `FinalResultChunk` 或 `FinalResultStateChunk` 结束（§6.8） |
| AP8 | Memory 不做用户隔离 | 跨用户数据泄漏 | `GetAppAndUserFromContext(ctx)` 获取维度隔离（§6.6） |
| AP9 | 手动构造记忆工具实例 | 绕过框架注入，行为不一致 | `service.Tools()` 返回 6 个工具追加到 Agent |
| AP10 | 裸 `go func()` 启动 Runner 消费 goroutine | panic 未恢复，goroutine 泄漏 | `pkg/safego.Go` + ctx 退出路径（红线 #13/#23） |

#### 6.13.3 数据流反模式

| # | 反模式 | 后果 | 正确做法 |
|---|--------|------|----------|
| AP11 | 绕过 `internal/session/trpc` 把 Ent 行塞进 `session.Event` | 框架 Session 与 DB 不一致 | 通过 `session/trpc` 适配（红线 #4） |
| AP12 | 在 transport 层解析工具参数或拼接 prompt | 职责错位，无法复用 | 工具装配在 `internal/tools`，prompt 在 `internal/agent`（红线 #5） |
| AP13 | 跨表/跨 Repo 多次独立写不包事务 | 数据不一致 | `Data.ExecInTx` 或 biz `XxxTxRepo.ExecInTx`（红线 #24） |
| AP14 | 事件投影时丢失事件类型分级 | Critical 事件丢失，Important 事件过载 | 按 AS-EVT-01 分级处理（§15.2） |

#### 6.13.4 框架 API 陷阱

| # | 陷阱 | 正确做法 |
|---|------|----------|
| AP15 | `Runner.Run` 返回 `<-chan *event.Event`，消费完不关闭 | goroutine 泄漏 | 消费完显式 `Close()` 或在 ctx 取消时关闭 |
| AP16 | `Model.GenerateContent` 的 `<-chan *Response` 中途断开 | 调用方未处理 EOF/错误 | 消费循环检查 `resp.Err()` 和 channel 关闭 |
| AP17 | `ManagedRunner.Cancel(requestID)` 返回 false | 取消已完成的 Run | 调用前检查 `RunStatus` |
| AP18 | `SteerableRunner.EnqueueUserMessage` 在非 streaming Run 调用 | panic | 仅在 streaming Run 中调用 |

### 6.14 框架文档导航地图

> **遇到框架问题时，先查对应文档，再读源码。** 框架文档是真相源，业务代码是消费者。

#### 6.14.1 问题类型 → 框架文档映射

| 问题类型 | 框架文档路径 | 何时读 |
|---------|-------------|--------|
| Runner 生命周期/取消/状态 | `pkg/trpc-agent-go/docs/zh/runner.md` | 实现 Runner 装配、取消、状态查询 |
| Agent 构建/子 Agent/工具挂载 | `pkg/trpc-agent-go/docs/zh/agent.md` | 实现 `BuildLLMAgent`、Agent-as-Tool |
| Tool 三层接口/流式工具 | `pkg/trpc-agent-go/docs/zh/tool.md` | 新增工具、实现 StreamableTool |
| Memory 两种模式/自动提取 | `pkg/trpc-agent-go/docs/zh/memory.md` | 接入记忆、配置自动提取 |
| Event 类型/事件流/可靠性 | `pkg/trpc-agent-go/docs/zh/event.md` | 事件投影、可靠性分级 |
| Session 存储/摘要/压缩 | `pkg/trpc-agent-go/docs/zh/session.md` | 会话持久化、压缩配置 |
| Team 编排/协调者/Swarm | `pkg/trpc-agent-go/docs/zh/team.md` | 多 Agent 协作 |
| Graph 图编排/节点/边 | `pkg/trpc-agent-go/docs/zh/graph.md` | 图式工作流 |
| Skill 仓库/热重载/路由 | `pkg/trpc-agent-go/docs/zh/skill.md` | 技能系统 |
| Plugin 插件/回调/生命周期 | `pkg/trpc-agent-go/docs/zh/plugin.md` | 插件注册、回调处理 |
| Model LLM 适配/流式响应 | `pkg/trpc-agent-go/docs/zh/model.md` | 新增 LLM 厂商 |
| 错误处理/错误类型 | `pkg/trpc-agent-go/docs/zh/error-handling.md` | 框架错误传播 |
| 可观测性/追踪/指标 | `pkg/trpc-agent-go/docs/zh/observability.md` | 集成 telemetry |
| 知识库/向量存储/嵌入 | `pkg/trpc-agent-go/docs/zh/knowledge/` | RAG、向量检索 |
| A2A 互操作 | `pkg/trpc-agent-go/docs/zh/a2a.md` | Agent 间通信 |

#### 6.14.2 框架源码导航（文档未覆盖时）

| 框架包 | 项目桥接包 | 何时读源码 |
|--------|-----------|-----------|
| `pkg/trpc-agent-go/runner` | `internal/agent/trpc_runtime.go` | Runner 行为不确定时 |
| `pkg/trpc-agent-go/agent/llmagent` | `internal/agent/trpc_build.go` | Agent 构建细节 |
| `pkg/trpc-agent-go/tool` | `internal/tools/trpc/` | Tool 接口实现 |
| `pkg/trpc-agent-go/memory` | `internal/memory/` | Memory Service 实现 |
| `pkg/trpc-agent-go/event` | `internal/service/`（事件投影） | Event 结构/类型 |
| `pkg/trpc-agent-go/session` | `internal/session/` | Session 存储适配 |

#### 6.14.3 决策树：查文档 vs 读源码 vs 问 biz 端口

```
遇到框架相关问题
  ├─ 是"框架提供什么 API"？→ 读框架文档（§6.14.1）
  ├─ 是"框架 API 行为细节"？→ 读框架源码（§6.14.2）
  ├─ 是"项目如何桥接框架"？→ 读项目桥接包（§6.11 对照表）
  ├─ 是"业务逻辑该怎么放"？→ 查 biz 端口接口（§5.2 + §9.3）
  └─ 是"框架是否支持某特性"？→ 先读文档，文档无则读源码确认，禁止假设
```

#### 6.14.4 扩展 vs 包装决策

| 场景 | 决策 | 示例 |
|------|------|------|
| 框架提供接口，项目需实现 | **扩展**（实现框架接口） | 实现 `tool.Tool`/`memory.Service` |
| 框架行为需调整但不改接口 | **包装**（业务层包装） | `internal/agent` 包装 `llmagent` |
| 框架不提供某能力 | **业务层自建**（不碰框架） | `internal/compress` L0 压缩 |
| 框架行为不确定 | **读源码确认**，禁止假设 | Runner 取消语义 |

---

## 第七章：项目代码风格

> 通用 Go 代码风格（命名、函数设计、错误处理、并发等）见 `go-oop-guide` SKILL。本章只列**项目特定约束**。

### 7.1 错误处理（项目约束）

**核心原则**：`apierror.Error` 是项目唯一的内部错误模型，`kerrors` 仅在传输层（middleware）出现。

#### 7.1.1 分层规则

| 层 | 规则 | 禁止 |
|---|------|------|
| **Data** | 所有 repo 方法错误必须经过 `entErrToBizErr(err, domain)` 返回 | 直接 `apierror.*` 构造、`fmt.Errorf` 返回、`errors.New` 返回 |
| **Biz** | 哨兵定义为 `var ErrXxx = apierror.Xxx(domain, msg)`；Usecase 返回哨兵或 `apierror.Wrap` | `errors.New` 哨兵、`fmt.Errorf` 作为最终返回值 |
| **Service** | biz 错误直接透传（APIToKratos 中间件翻译）；框架运行时错误用 `TurnError` 包装 | `errors.Is(err, sql.ErrNoRows)` 判断、`mapXxxError` 二次映射 |
| **Middleware** | `APIToKratos` 统一翻译 apierror → kerrors | — |

#### 7.1.2 各层示例

**Data 层**（唯一出口 `entErrToBizErr`）：

```go
func (r *agentRepo) Get(ctx context.Context, id string) (*biz.Agent, error) {
    entAgent, err := r.data.readClient(ctx).Agent.Get(ctx, id)
    if err != nil {
        return nil, entErrToBizErr(err, apierror.DomainAgent)
    }
    return entAgentToBiz(entAgent), nil
}
```

**Biz 层**（哨兵 + Wrap）：

```go
var (
    ErrNotFound     = apierror.NotFound(apierror.DomainAgent, "agent not found")
    ErrInvalidInput = apierror.BadRequest(apierror.DomainAgent, "invalid input")
    ErrKeyConflict  = apierror.Conflict(apierror.DomainAgent, "agent key already exists")
)

func (uc *AgentUsecase) GetAgent(ctx context.Context, id string) (*Agent, error) {
    agent, err := uc.repo.Get(ctx, id)
    if err != nil {
        return nil, err  // data 层已翻译，直接透传
    }
    return agent, nil
}
```

**Service 层**（透传 + 框架错误包装）：

```go
func (s *ChatService) GetAgent(ctx context.Context, req *v1.GetAgentRequest) (*v1.GetAgentReply, error) {
    agent, err := s.uc.GetAgent(ctx, req.Id)
    if err != nil {
        return nil, err  // APIToKratos 中间件自动翻译
    }
    return toProtoAgentReply(agent), nil
}

// 框架运行时错误 → TurnError 包装
eventCh, err := runner.Run(ctx, userID, sessionID, message)
if err != nil {
    return nil, TurnError(TurnErrAgentBuildFailed, err.Error())
}
```

#### 7.1.3 apierror.Error 设计规则

| 规则 | 说明 |
|------|------|
| `Is()` 比较 Code + Domain | 精确匹配，避免不同子系统的同 Code 错误互相匹配；Code-only 匹配用 `apierror.From + ae.Code` |
| `Wrap()` 不覆盖已有 apierror | 已是 apierror 则透传，不改变 Code/Domain |
| `ToKratos()` reason = `Domain + "_" + Code` | 如 `"AGENT_NOT_FOUND"`，前端可按 reason 精确分支 |
| `CodeInternal` 类型的 Message 不暴露给客户端 | ToKratos 时替换为通用消息，原始信息只写日志 |
| Domain 使用 `apierror.DomainXxx` 常量 | 禁止硬编码字符串 |

#### 7.1.4 两条错误通道

| 通道 | 错误模型 | 前端消费方式 |
|------|---------|------------|
| HTTP CRUD | apierror → APIToKratos → HTTP status + reason + message | axiosHandler.ts 按 status 分流，kratosError.ts 按 reason 分支 |
| WebSocket Chat | TurnError → EnvelopeError → WS push | errorCodeHints.ts 按 error_code 分流 |

#### 7.1.5 错误码速查

| 场景 | apierror Code | HTTP Status |
|------|--------------|-------------|
| 参数校验失败 | `CodeBadRequest` | 400 |
| 未认证 | `CodeUnauthorized` | 401 |
| 无权限 | `CodeForbidden` | 403 |
| 记录不存在 | `CodeNotFound` | 404 |
| 唯一约束冲突 | `CodeConflict` | 409 |
| 限流 | `CodeRateLimit` | 429 |
| 服务不可用 | `CodeUnavailable` | 503 |
| 内部错误 | `CodeInternal` | 500 |

#### 7.1.6 迁移纪律

- 红线 #14 更新为：**不得在 biz 层使用 `fmt.Errorf` 或 `errors.New` 返回业务错误**，统一使用 `apierror.*`
- 已有 `TECH-DEBT(BE1)` 标记的 `errors.New` 哨兵需逐步迁移为 `apierror.*`
- 迁移后同步删除 service 层对应的 `mapXxxError` 函数
- `kerrors` 直接使用仅在 `internal/tools/`、`internal/graph/`、`internal/team/`、`internal/channel/` 迁移完成前允许，新代码禁止

### 7.2 依赖注入（项目约束）

1. **Wire ProviderSet**：每层一个，在 `biz.go`/`data.go`/`service.go`/`server.go` 中定义
2. **构造函数参数**：只接收接口或具体依赖，不接收 `*Data` 之外的"上帝对象"
3. **禁止手动编辑 `wire_gen.go`**：必须通过 `make wire` 生成
4. **`cmd/admin/wire.go` 只做组装**：`wire.Build(...)` + 少量跨层 `provide*`；**禁止**在 provider 内做进程级全局注册
5. **全局/副作用注册归属**：

| 场景 | 正确位置 | 禁止 |
|------|----------|------|
| Repo 就绪后注册 biz 解析器 | `data.NewXxxRepo` / `biz.NewXxxUsecase` 构造函数 | `provideXxxBootstrap` + 占位 `*Bootstrap` 类型 |
| EventBus 等进程单例 | `main.newApp` 的 `kratos.BeforeStart` / `AfterStop` | `cmd/admin/wire.go` 内 `SetGlobal*` |
| 观测/工具包全局钩子 | 对应包的 `New*` 或 `service` 层构造函数 | Wire provider 内 `mcpobserve.Set*` 等 |

6. 改 Wire 后本地 **`make wire-clean`**；PR 须通过 CI `wire-clean` job

**反模式示例（已禁止）**：

```go
// ❌ cmd/admin/wire.go — 占位类型 + 全局副作用
type credentialKeyBootstrap struct{}
func provideCredentialKeyBootstrap(repo biz.SystemSettingRepo) *credentialKeyBootstrap {
    biz.SetCredentialKeyResolver(...)
    return &credentialKeyBootstrap{}
}

// ✅ internal/data/system_setting.go — Repo 构造时注册
func NewSystemSettingRepo(d *Data) biz.SystemSettingRepo {
    repo := &systemSettingRepo{data: d}
    biz.SetCredentialKeyResolver(func(ctx context.Context) ([]byte, error) {
        return biz.ResolveCredentialAESKey(ctx, repo)
    })
    return repo
}
```

### 7.3 并发（项目约束）

| 规则 | ✅ 正确 | ❌ 错误 |
|------|---------|---------|
| context 传递 | 所有跨层调用必须传递 `ctx` | `go func() { doWork() }()` 不传 ctx |
| goroutine | 必须走 `pkg/safego.Go` / `pkg/safego.GoRecover`（红线 13） | 裸 `go func()` 不处理 panic |
| MCP 子进程 | context 取消时清理 | 子进程泄漏 |
| WebSocket 流 | 处理客户端断连 | 不检测客户端断连 |
| 共享状态 | `sync.Mutex`/`sync.RWMutex` | 全局变量 |

### 7.4 日志（项目约束）

**禁止 `log/slog`**，统一使用 `pkg/loggateway.Logger`：

- `x.lg.Info/Warn/Error(msg, loggateway.StepID("..."), loggateway.Err(err))` — 结构化日志
- `x.lg.With(loggateway.SessionID(sid))` — 绑定会话上下文
- `loggateway.Global()` — 独立函数使用全局 Logger
- `event.SysLog*` / `event.SessionSysLog*` — **已废弃**，禁止新增调用

### 7.5 命名（项目补充）

> 通用命名规则见 `go-oop-guide` SKILL §十二。以下为本项目补充。

| 场景 | 规范 | ✅ 示例 |
|------|------|---------|
| 文件名 | 小写+下划线，按职责拆分 | `agent_repo.go`, `trpc_build.go` |
| 类型转换 | 独立函数 | `toProtoXxx`/`fromProtoXxx` |
| 构造函数 | 统一 `NewXxx`，返回指针 | `NewAgentUsecase` |

---

## 第八章：API 与 Proto 规范

### 8.1 Proto 定义规则

| 规则 | ✅ 正确 | ❌ 错误 |
|------|---------|---------|
| 路径 | `api/kratos/<module>/v1/<module>.proto` | 随意放置 |
| HTTP 注解 | 每个 RPC 配 `google.api.http` | 只定义 RPC 不配 HTTP path |
| 必填标记 | `(google.api.field_behavior) = REQUIRED` | 不标记必填 |
| 命名 | proto 字段 `snake_case`，Go 生成 `CamelCase` | proto 字段用 camelCase |
| 契约完整性 | 全部能力在 proto 中定义 | 一半在 proto，一半手写路由 |
| 请求/响应 | `XxxReq` / `XxxReply` | 随意命名 |

### 8.2 代码生成流程

```bash
make init    # 首次安装插件
make api     # 生成 Go + TypeScript
make wire    # 生成 Wire 依赖注入
make config  # 仅改 conf.proto 时
```

**禁止修改工具生成的代码（红线 11）。**

### 8.3 迁移与迭代硬约束

1. 对外能力必须在 Proto 中印全，禁止「一半在 proto，一半用手写 srv.Route / HandleFunc」
2. 修改 `.proto` 必须跑生成，提交全部生成物
3. SQLite 侧以 `*ent.Client` 为主入口，禁止在 `NewData` 里再 `sql.Open` 同一 DSN
4. 表结构进 Ent，禁止长期平行维护「仅存 SQL、不进 Ent」
5. 业务模块 HTTP 只做 `Register<Module>HTTPServer`，gRPC 只做 `Register<Module>ServiceServer`

---

## 第九章：模块化设计

### 9.1 新增功能模块的标准结构

```
api/kratos/<module>/v1/<module>.proto     ← API 契约
internal/biz/<module>.go                  ← 模型 + Repo 接口 + Usecase
internal/biz/<module>_types.go            ← 领域类型定义（如需拆分）
internal/data/<module>_repo.go            ← Repo 实现
internal/data/ent/schema/<module>.go      ← Ent Schema
internal/service/<module>.go              ← Service 实现
internal/server/http.go                   ← 注册 RegisterXxxHTTPServer
cmd/admin/wire.go                         ← Wire 注入
```

### 9.2 模块间通信

| 方式 | ✅ 正确 | ❌ 错误 |
|------|---------|---------|
| 同步调用 | Usecase 之间通过接口调用 | 直接 import 另一模块的 data |
| 异步事件 | 通过 `Broker` 发布/订阅 | 通过全局变量共享状态 |
| 状态共享 | 数据库 | 包级变量 |
| 跨模块调用 | 通过 biz 级窄接口（端口） | 持有对方 Service 完整具体类型 |

### 9.3 模块解耦端口

| 模块 | 端口 | 用途 | 位置 |
|------|------|------|------|
| Channel → Chat | `biz.NativeTurnGateway` | 同步 Turn 执行 + 运行控制 | `internal/biz/turn_input.go` |
| Channel → Graph | `biz.GraphExecutor` | Graph 执行（返回 executionID） | `internal/biz/graph.go` |
| Team → Chat | `biz.TurnInput` | Team turn 输入（proto 映射在 service） | `internal/biz/turn_input.go` |
| Graph → Biz | `biz.GraphBuildConfig` / `biz.GraphRuntime` | Graph 配置与运行时端口 | `internal/biz/` |
| Graph → Resolver | `build_deps.go` 接口 | Agent/Tool/Model resolver 分离注入 | `internal/graph/trpc/build_deps.go` |

**端口设计原则**：

1. **接口定义在 biz 层**：消费方 import biz 接口，不 import 具体实现
2. **Wire 绑定在 service 层**：`wire.Bind(new(biz.XxxPort), new(*XxxService))`
3. **返回值用 biz 类型**：端口方法返回 `string`/`biz.Xxx`，不返回 proto 类型
4. **构造函数收窄**：只接收需要的端口，不接收"上帝对象"

### 9.4 配置管理

1. **配置来源优先级**：环境变量 > 系统设置 > 配置文件 > 代码默认值
2. **配置结构**：在 `internal/conf/conf.proto` 中定义
3. **热更新**：通过 Kratos config source 支持，不自行实现 watch

---

## 第十章：任务速查卡

### 新增 API

```
1. api/kratos/<module>/v1/<module>.proto  ← 定义 RPC + HTTP 注解
2. make api                                ← 生成 Go + TS
3. internal/biz/<module>.go                ← 模型 + Repo 接口 + Usecase
4. internal/data/ent/schema/<module>.go    ← Ent Schema
5. go generate ./internal/data/ent         ← 生成 Ent 代码
6. internal/data/<module>_repo.go          ← Repo 实现
7. internal/service/<module>.go            ← Service（嵌入 Unimplemented*）
8. internal/server/http.go                 ← Register*HTTPServer
9. cmd/admin/wire.go                       ← Wire 注入
10. go build ./cmd/admin                   ← 验证编译
11. web/src/services/index.ts              ← 导出 createXxxService
```

### 新增工具

```
1. internal/tools/registry.go              ← Registry() 中注册 ToolRegistration
2. internal/tools/builtin_tools_seed.go    ← 添加种子数据
3. 需要配置 → AssemblyConfig 增加字段 + Assemble 中增加覆盖逻辑
4. internal/tools/custom/                  ← 自定义工具实现（如需）
5. Chat 和 Team 共用 BuildToolsets，验证两处生效
```

### 新增数据实体

```
1. internal/data/ent/schema/xxx.go         ← Fields/Index/Edge
2. go generate ./internal/data/ent         ← 生成 Ent 代码
3. internal/data/xxx.go                    ← Repo 实现（entXxxToBiz / bizXxxToEnt）
4. internal/biz/xxx.go                     ← 模型 + Repo 接口 + Usecase
5. internal/biz/biz.go                     ← ProviderSet 添加 NewXxxUsecase
6. internal/data/data.go                   ← ProviderSet 添加 NewXxxRepo
```

### 新增 LLM Provider

```
1. internal/provider/trpc_llm.go           ← MapProviderType 中添加映射
2. 如需自定义选项 → buildProviderOptions 中添加分支
3. 确保框架 model/<provider>/ 包已实现 model.Model 接口
```

### 新增 Session/Memory 后端

```
1. internal/session/ 或 internal/memory/   ← 创建适配器
2. 通过 Wire 注入到 Runner 的 SessionService/MemoryService
3. 框架侧实现 session.Service / memory.Service 接口
```

---

## 第十一章：AI 编码自检清单

### 改动前（代码探索）

- [ ] **结构性问题已用 CodeGraph**：符号 / 调用链 / 影响面 / 模块上下文，未 grep 先于 `codegraph_*`
- [ ] **探索结果可信**：未对 CodeGraph 返回的结构信息做重复 grep 验证
- [ ] **已读模块交叉参考手册**：在 `docs/development/65-module-cross-reference-full.md` 中找到目标模块卡片，确认上游依赖、下游影响、共享契约、事件、数据库、前端对应

### 改动中（行为自检 — Karpathy 原则）

- [ ] **假设显式化**：实现前已声明关键假设；存在多种理解时已呈现而非静默选择；困惑时已停下来提问
- [ ] **最小实现**：未添加未请求的功能/抽象/灵活性/可配置性；未处理不可能场景的错误；200 行能 50 行完成则已重写；单一实现未预抽接口（CS-B18）
- [ ] **外科手术式修改**：未"顺手改善"相邻代码/注释/格式；匹配现有风格；发现无关死代码只提不删；每行改动可追溯到用户请求

### 改动中（逐层检查）

- [ ] **Service 层**：只做映射和编排，无业务逻辑
- [ ] **Biz 层**：无 `pkg/trpc-agent-go` import，无 proto import
- [ ] **Data 层**：仅 `d.RW()`/`d.RWDB()`/`d.Postgres()` 访问，无并联 SQLite 连接，无已废弃的 `d.Ent()`/`d.RawDB()`/`d.ReadDB()`
- [ ] **Agent/Tools/Team 层**：框架 API 调用合规，不复制框架内部逻辑
- [ ] **框架源码只读**：未修改 `pkg/trpc-agent-go` 任何文件，能力扩展走适配层（红线 #27）
- [ ] **模块解耦**：跨模块调用走 biz 级窄接口，不持有对方 Service 具体类型
- [ ] **Channel**：不 import `graphv1` 等 proto 包，不持有 `*ChatService`
- [ ] **Team**：不 import chat proto，输入用 `biz.TurnInput`
- [ ] **Graph**：biz 层不见 trpc graph 类型，resolver 通过接口注入
- [ ] **新增工具**：先在 `Registry()` 注册 `ToolRegistration`，再在 `builtin_tools_seed.go` 添加种子
- [ ] **流式工具**：实现 `StreamableTool` 接口，必须发送 `FinalResultChunk`
- [ ] **记忆工具**：通过 `memory.Service.Tools()` 注入，不手动构造
- [ ] **MCP Broker**：`AllowAdHocHTTP` 默认 false，安全边界明确
- [ ] **错误处理**：使用 `apierror`，不用 `fmt.Errorf`/`errors.New`（见 §7.1）
- [ ] **日志**：使用 `loggateway.Logger`，不用 `log/slog`，不用 `event.SysLog*`
- [ ] **goroutine**：走 `pkg/safego`，无裸 `go func()`，有明确退出路径（红线 #13/#23）
- [ ] **并发安全**：共享 map/slice 并发读写已加锁或用 sync.Map/chan（红线 #21）
- [ ] **错误不吞**：无 `_ =` 忽略 error 返回值（红线 #22）
- [ ] **事务边界**：跨表/跨 Repo 写操作已包 `ExecInTx`（红线 #24）
- [ ] **敏感信息**：日志无 credential/api_key/password/token 明文（红线 #25）
- [ ] **nil 检查**：外部输入/接口返回值/指针解引用前已判 nil（红线 #26）
- [ ] **OOP 合规**：见 `go-oop-guide` SKILL（接口方法 ≤ 5、接口定义在使用方、返回具体类型参数接收接口、无上帝对象注入）
- [ ] **编程规范合规**：CS-B5 函数 ≤ 80 行、CS-B6 圈复杂度 ≤ 15、CS-B7 参数 ≤ 5 个、CS-B8 无魔法数字、CS-B9 DB 查询带超时、CS-B10 循环内无逐条 DB、CS-B12 敏感字段不日志、CS-B17 技术债务已标记、CS-B18 无过度设计
- [ ] **框架反模式已避免**：未在 biz 直接 new Runner、未在 plugin 回调同步写库、StreamableTool 发了 FinalResultChunk、Memory 做了用户隔离（§6.13 AP1-AP18）
- [ ] **框架文档已查**：遇到框架问题先查 `pkg/trpc-agent-go/docs/zh/` 对应文档，未假设框架行为（§6.14）
- [ ] **状态机已定义**：实体 >3 状态时定义了 `*_state_machine.go`，状态变更经 `Transition`，终态无转换（§15.1）
- [ ] **事件可靠性分级正确**：Critical 事件先写后发、Important 阻塞发送异步持久化、Informational 尽力而为（§15.2）
- [ ] **不变量已识别保护**：唯一性/引用完整性/业务规则/状态一致性不变量有 DB 约束或代码守卫（§15.3）
- [ ] **边界条件已测试**：空输入/超长/并发/非法转换/外部依赖失败/资源耗尽/取消中断（§15.4）
- [ ] **测试分层正确**：Service 集成测试、Biz 单元测试、Data 仓储测试、Agent 桥接测试（§16.1）
- [ ] **Mock 策略正确**：项目 Repo 手写 Fake、框架接口可 mockgen、Mock 实现完整接口、测试用 Noop Logger（§16.2）
- [ ] **并发测试通过**：`go test -race` 通过，并发场景不变量保持（§16.5）

### 改动后（构建与验证）

- [ ] `make api` 已执行（如改了 proto）
- [ ] `make wire` 已执行（如改了 Wire 声明）
- [ ] `make wire-clean` 通过（`wire_gen.go` 与 `wire.go` 同步）
- [ ] `make lint` 通过（含 R11：wire.go 无全局 bootstrap）
- [ ] `go build ./cmd/admin` 通过
- [ ] 无红线违反

### 全链路合并检查

- [ ] `api/**/*.proto` 覆盖本迭代全部 `/v1` 能力，`make api`，Go + TS 已提交
- [ ] `internal/biz` / data / service / server 合规；Wire + `go build ./cmd/admin`
- [ ] `web/src/services/index.ts` `createXXXService`
- [ ] 前端合规：见 `docs/guides/frontend-guide.md` 自检清单

---

## 第十二章：验证命令

| 改动类型 | 最小验证 |
|----------|----------|
| 仅 Service + 单测 | `go test ./internal/service/... -run TestXxx -count=1` |
| 仅 Biz / Data | `go test ./internal/biz/... ./internal/data/... -count=1` |
| Proto 变更 | `make api && go build ./...` |
| Wire 注入 | `make wire && go build ./cmd/admin` |
| 前端 | `cd web && pnpm lint && pnpm test && pnpm build` |
| **提交前（全量）** | 后端：`make api && make wire && make build && make test && make lint`；前端：`cd web && pnpm lint && pnpm test && pnpm build` |

---

## 第十四章：编程规范

> 编程规范是编码质量的硬约束，可通过 linter/静态分析/编码模式自动或半自动执行。违反不等于架构破坏，但影响代码质量和可维护性。完整维度检查清单见 `docs/review-dimension-checklists.md`。

| 编号 | 规范 | 约束方式 | 来源 |
|------|------|----------|------|
| CS-B1 | 禁止使用 `log/slog`，统一 `pkg/loggateway.Logger` | linter 禁止 import | 原红线 #16 |
| CS-B2 | 不得新增已无调用者的 deprecated 方法，死代码即删 | 静态分析 | 原红线 #19 |
| CS-B3 | 压缩操作必须通过 CAS + 事务保证原子性 | 代码审查 | 新增 |
| CS-B4 | Repository 接口方法 ≤ 5，超过按职责域拆分子接口 | linter/审查 | 新增 |
| CS-B5 | 函数体不超过 80 行，超过必须拆分 | linter/审查 | 新增 |
| CS-B6 | 圈复杂度不超过 15，超过必须简化分支 | linter | 新增 |
| CS-B7 | 参数列表不超过 5 个，超过用 Option struct | 审查 | 新增 |
| CS-B8 | 禁止魔法数字，必须定义命名常量 | linter | 新增 |
| CS-B9 | 数据库查询必须带 `context` 超时，禁止裸 `d.Ent().Query()` | 审查 | 新增 |
| CS-B10 | 循环内禁止逐条 DB 操作，必须批量 | 审查 | 新增 |
| CS-B11 | 外部输入必须校验后才进入 biz 层 | 审查 | 新增 |
| CS-B12 | 敏感字段（key/secret/token）禁止日志输出 | linter/审查 | 新增 |
| CS-B13 | 核心业务 Usecase 必须有单元测试，覆盖率 ≥ 70% | CI 门槛 | 新增 |
| CS-B14 | 错误路径必须有测试用例覆盖 | 审查 | 新增 |
| CS-B15 | 重试次数上限 3 次，必须指数退避 | 审查 | 新增 |
| CS-B16 | 写操作必须保证幂等（相同请求不产生副作用） | 审查 | 新增 |
| CS-B17 | 技术债务用 `// TODO(debt):` 标记，含 issue 编号和预期偿还时间 | linter/审查 | 新增 |
| CS-B18 | 禁止过度设计：单一实现不预抽接口；未请求的配置项/扩展点不添加；三处复用前不提取公共函数 | 审查 | 原红线 #20 |

### 14.1 编程规范与红线的关系

| 维度 | 架构边界红线（#1-#9, #12, #15, #17, #18） | 运行时正确性红线（#10, #11, #13, #14, #21-#26） | 编程规范（CS-B1~B18） |
|------|-------------------------------------------|------------------------------------------------|----------------------|
| 违反后果 | 模块耦合/架构腐化 | panic/数据竞争/数据不一致/敏感信息泄漏 | 代码质量下降/可维护性降低 |
| 检测方式 | 代码审查（人工） | 代码审查 + race detector + linter | linter/静态分析（自动）+ 审查 |
| 修复优先级 | 🔴 阻断（必须修复） | � 阻断（必须修复，生产事故优先） | �🟡 建议（推荐修复） |
| 示例 | biz import trpc-agent-go | 共享 map 无锁并发读写 | 函数超过 80 行 |

### 14.2 维度检查清单引用

编码时按维度 A 面预防，详见 `docs/review-dimension-checklists.md`：
- 所有编码：维度 1（架构）、2（质量）、3（正确性）、8（错误处理）
- 涉及 DB：+ 维度 4（性能）
- 涉及外部输入/API：+ 维度 5（安全）
- 涉及 Usecase：+ 维度 6（可测试性）、11（业务逻辑）
- 涉及跨模块：+ 维度 7（可维护性）、12（文档同步）

---

## 附录：关键文件索引

| 文件 | 用途 |
|------|------|
| `internal/tools/toolset.go` | 工具注册中心（Registry + AssemblyConfig + Assemble） |
| `internal/tools/tool.go` | 项目级工具类型别名 |
| `internal/tools/trpc/toolsets.go` | 向后兼容适配层 |
| `internal/memory/trpc/sqlite_adapter.go` | Memory Service SQLite 适配器 |
| `internal/agent/trpc_build.go` | Agent 构建 + 工具集装配入口 |
| `internal/agent/trpc_runtime.go` | Runner 创建 + 用户 Turn 执行 |
| `internal/provider/trpc_llm.go` | LLM 模型驱动 |
| `internal/biz/turn_input.go` | Turn 输入端口定义 |
| `internal/biz/graph.go` | Graph 端口定义 |
| `pkg/trpc-agent-go/docs/mkdocs/zh/` | 框架官方文档（深度用法时查阅） |

---

## 第十三章：模块关联强制检查

> **模块不是孤岛。改任何模块前，必须先读关联文档。** 违反即停。

### 13.1 关联文档索引

| 文档 | 路径 | 定位 |
|------|------|------|
| **模块交叉参考** | `docs/development/65-module-cross-reference-full.md` | "改模块 X 时必须注意谁"（动态关联、影响面） |
| **系统架构总览** | `docs/development/0-system-diagram.md` | "每个模块是什么"（静态结构、全貌） |
| **数据库架构** | `docs/development/66-database-architecture.md` | 数据库设计与访问模式 |

### 13.2 开发前强制步骤

```
步骤 1：定位目标模块 → 读系统架构总览对应章节（了解静态结构）
步骤 2：读交叉参考手册 → 找到目标模块卡片（8 维度关联）
步骤 3：查变更影响表 → 确定需要同步修改的文件清单
步骤 4：按依赖方向逐层修改 → 验证时覆盖所有影响面
```

### 13.3 场景→章节速查

| 你要做什么 | 必须读的交叉参考章节 |
|-----------|---------------------|
| 新增/修改 biz 端口接口 | §四·端口接口变更影响表 + 目标模块卡片「下游影响」 |
| 新增/修改共享类型（DTO/struct） | §四·共享类型变更影响表 + 目标模块卡片「共享类型」 |
| 新增/修改事件类型（Envelope/EventBus） | §四·事件类型变更影响表 + 目标模块卡片「事件生产/消费」 |
| 新增/修改数据库 Schema | §四·数据库 Schema 变更影响表 + 目标模块卡片「数据库」 |
| 新增 LLM Provider | §五·场景演练 #1（7 个模块链路） |
| 新增工具 | §五·场景演练 #2（6 个模块链路） |
| 新增渠道平台 | §五·场景演练 #3（5 个模块链路） |
| 新增 Envelope 类型 | §五·场景演练 #4（前后端 4 层链路） |
| 修改 TurnInput/TurnResult | §五·场景演练 #5（最大影响面 8 模块） |

### 13.4 典型遗漏案例

| 遗漏 | 后果 |
|------|------|
| 改了 biz 接口签名但忘了 data 层实现 | 编译不通过 |
| 改了 AssemblyConfig 但忘了 service/chat 的 Wire 构造函数 | 运行时 nil pointer |
| 新增 Envelope 类型但没加前端 dispatcher 处理 | WS 消息丢失 |
| 加了 Ent 字段但没跑 go generate | ORM 不认识新列 |
| 改了 TRPCBuilderDeps 但没同步 team/trpc_build.go | Team 功能崩溃 |
| 新增工具但只在 Chat 验证没在 Team 验证 | Team 场景缺工具 |

---

## 第十五章：业务逻辑正确性约定

> 本章指导如何保证业务逻辑的准确性：状态机、事件可靠性、不变量、边界条件。
> 架构原则见 `project_rules.md` AS-FSM-01/AS-EVT-01，本章是**编码落地指导**。

### 15.1 状态机编码规范（AS-FSM-01 落地）

#### 15.1.1 何时必须定义状态机

任何实体拥有 **>3 种状态**时，必须定义显式状态机。当前需补全的实体：

| 实体 | 状态数 | 文件位置 |
|------|--------|---------|
| Run | 5（Pending/Running/Succeeded/Failed/Cancelled） | `internal/biz/run_state_machine.go` |
| Session | 已有，需统一接口 | `internal/biz/session_state_machine.go` |
| TeamRun | 6 | `internal/biz/team_run_state_machine.go` |
| GraphExecution | 5 | `internal/biz/graph_execution_state_machine.go` |

#### 15.1.2 状态机文件模板

文件名：`*_state_machine.go`，与实体同包。

```go
package biz

import "fmt"

// RunState 表示 Run 的状态枚举。
type RunState string

const (
    RunStatePending   RunState = "pending"
    RunStateRunning   RunState = "running"
    RunStateSucceeded RunState = "succeeded"
    RunStateFailed    RunState = "failed"
    RunStateCancelled RunState = "cancelled"
)

// runTransition 定义合法的状态转换：from → (event → to)。
var runTransitions = map[RunState]map[string]RunState{
    RunStatePending: {
        "start":    RunStateRunning,
        "cancel":   RunStateCancelled,
        "fail":     RunStateFailed,
    },
    RunStateRunning: {
        "succeed":  RunStateSucceeded,
        "fail":     RunStateFailed,
        "cancel":   RunStateCancelled,
    },
    // 终态（Succeeded/Failed/Cancelled）无转换
}

// Transition 校验状态转换是否合法。
// 返回目标状态；非法转换返回 error。
func (s RunState) Transition(event string) (RunState, error) {
    events, ok := runTransitions[s]
    if !ok {
        return "", fmt.Errorf("state %q is terminal, no transitions", s)
    }
    next, ok := events[event]
    if !ok {
        return "", fmt.Errorf("invalid transition: state=%q event=%q", s, event)
    }
    return next, nil
}

// Guard 是可选守卫条件，返回 false 则拒绝转换。
type RunGuard func(ctx context.Context, from RunState, event string) bool
```

#### 15.1.3 状态机使用规则

| # | 规则 | 说明 |
|---|------|------|
| 1 | 状态变更必须经 `Transition` | 禁止直接赋值 `run.State = "running"` |
| 2 | 守卫条件用 `Guard` | 如"Running→Succeeded 需检查所有 ToolResult 已落库" |
| 3 | 终态不可逆 | Succeeded/Failed/Cancelled 是终态，无转换 |
| 4 | 并发安全 | 状态机本身无状态，转换表是只读 map，可并发读 |
| 5 | 持久化前校验 | Repo.Update 前调用 `Transition` 校验，非法转换返回 `apierror.BadRequest` |

#### 15.1.4 状态机测试要求

```go
// 必须覆盖：所有合法转换 + 所有非法转换被拒绝 + 终态无转换
func TestRunStateMachine(t *testing.T) {
    // 合法转换
    tests := []struct{ from, event, want RunState }{
        {RunStatePending, "start", RunStateRunning},
        {RunStateRunning, "succeed", RunStateSucceeded},
        // ... 所有合法转换
    }
    for _, tt := range tests {
        got, err := tt.from.Transition(tt.event)
        require.NoError(t, err)
        require.Equal(t, tt.want, got)
    }

    // 非法转换被拒绝
    _, err := RunStateSucceeded.Transition("start")
    require.Error(t, err)

    // 终态无转换
    for _, terminal := range []RunState{RunStateSucceeded, RunStateFailed, RunStateCancelled} {
        _, err := terminal.Transition("any")
        require.Error(t, err)
    }
}
```

### 15.2 事件可靠性分级编码（AS-EVT-01 落地）

#### 15.2.1 事件级别选择决策树

```
新增事件类型
  ├─ 丢失会导致数据损坏或用户可见错误？→ Critical
  │   例：ToolResult（工具结果丢失=Agent 决策错误）、Error（错误丢失=静默故障）
  ├─ 丢失会导致状态不一致但可恢复？→ Important
  │   例：StateDelta（状态增量丢失=前端状态漂移）、RunStatus（状态丢失=前端不更新）
  └─ 丢失仅影响调试/日志？→ Informational
      例：TextDelta（文本增量丢失=流式体验降级）、FlowLog（日志丢失=可观测性下降）
```

#### 15.2.2 各级别编码要求

| 级别 | 持久化 | 发送方式 | 重试 | 编码要求 |
|------|--------|---------|------|---------|
| Critical | SQLite WAL（先写） | WBPF（Write-Before-Publish-Forward） | 是 | 必须先写 EventStore 再发 EventBus |
| Important | SQLite EventStore（异步） | BlockUpTo（阻塞发送至超时） | 否 | 允许异步持久化，但发送时阻塞 |
| Informational | 不持久化 | 尽力而为 | 否 | 直接发 EventBus，丢弃不报错 |

#### 15.2.3 事件投影编码规范

```go
// Service 层事件投影示例
func (s *ChatService) projectEvent(ctx context.Context, e *event.Event) {
    switch e.Type {
    case event.TypeToolResult:
        // Critical：先写后发
        if err := s.eventStore.Save(ctx, e); err != nil {
            s.lg.Error("save critical event failed", loggateway.Err(err))
            return // 不发送，避免丢失
        }
        s.eventBus.Publish(ctx, e)

    case event.TypeStateDelta:
        // Important：阻塞发送，异步持久化
        s.eventBus.PublishBlockUpTo(ctx, e, 100*time.Millisecond)
        s.safego.Go(func() { s.eventStore.SaveAsync(ctx, e) })

    case event.TypeTextDelta:
        // Informational：尽力而为
        s.eventBus.Publish(ctx, e) // 丢弃不报错
    }
}
```

### 15.3 不变量识别与保护

#### 15.3.1 必须识别的不变量类型

| 类型 | 示例 | 保护方式 |
|------|------|---------|
| 唯一性不变量 | 一个 Session 同一时刻只能有一个活跃 Run | DB 唯一约束 + 状态机守卫 |
| 引用完整性不变量 | Message 必须属于有效 Session | 外键约束 + Repo 校验 |
| 业务规则不变量 | Agent 的 tool_keys 必须是已注册工具 | Service 层校验 + 禁止绕过 |
| 状态一致性不变量 | Run 终态后不能再产生事件 | 状态机 + 事件过滤 |

#### 15.3.2 不变量编码规范

```go
// 1. DB 层：唯一约束（Schema 定义）
field.String("session_id").Optional(),
// + DDL 迁移：CREATE UNIQUE INDEX idx_runs_session_active ON runs(session_id) WHERE state = 'running'

// 2. Repo 层：引用完整性校验
func (r *runRepo) Create(ctx context.Context, run *biz.Run) error {
    // 校验 Session 存在
    if _, err := r.sessionRepo.Get(ctx, run.SessionID); err != nil {
        return fmt.Errorf("validate session: %w", err)
    }
    // ... 创建
}

// 3. Service 层：业务规则校验
func (s *ChatService) StartRun(ctx context.Context, req *chatv1.StartRunRequest) error {
    // 校验 tool_keys 都是已注册工具
    for _, key := range req.ToolKeys {
        if !s.toolsCatalog.Exists(key) {
            return apierror.BadRequest("unknown tool: %s", key)
        }
    }
    // ... 启动 Run
}

// 4. 状态机守卫：状态一致性
var runGuards = map[RunState]map[string]RunGuard{
    RunStateRunning: {
        "succeed": func(ctx context.Context, from RunState, event string) bool {
            // 守卫：所有 ToolResult 必须已落库
            return allToolResultsPersisted(ctx)
        },
    },
}
```

### 15.4 边界条件测试要求

每个业务功能必须测试以下边界条件：

| 边界条件 | 测试要求 |
|---------|---------|
| 空输入 | 空列表、空字符串、nil 指针 |
| 超长输入 | 超过 DB 字段长度、超过 LLM token 限制 |
| 并发场景 | 同一 Session 并发 StartRun（应拒绝） |
| 状态转换非法 | 终态后再转换（应返回 error） |
| 外部依赖失败 | LLM 超时、DB 锁冲突、MCP 服务不可达 |
| 资源耗尽 | 连接池满、channel 满、磁盘满 |
| 取消中断 | ctx 取消后操作是否正确回滚 |

---

## 第十六章：测试约定

> 本章指导各层测试粒度、框架 mock 策略、事件流/状态机/并发测试。
> 测试循环流程见 `aranea-test-loop` SKILL，本章是**测试设计指导**。

### 16.1 分层测试粒度

| 层级 | 测试类型 | Mock 范围 | 验证重点 |
|------|---------|----------|---------|
| Service | 集成测试 | mock Runner/Agent/Repo | proto↔biz 映射、事件投影、Wire 装配 |
| Biz | 单元测试 | mock Repo（接口） | 业务逻辑、状态转换、不变量 |
| Data | 仓储测试 | 真实 SQLite（内存模式） | Ent 查询、错误翻译、事务 |
| Agent | 桥接测试 | mock 框架 Agent/Runner | biz→trpc 转换、BuilderDeps 装配 |
| Tools | 工具测试 | mock 框架 Tool 接口 | 工具注册、AssemblyConfig 覆盖 |

### 16.2 框架 Mock 策略

#### 16.2.1 何时用 Fake vs Mock

| 场景 | 选择 | 示例 |
|------|------|------|
| 需要复杂状态/行为 | **Fake**（内存实现） | `FakeRepo` 实现 Repo 接口，内存 map 存储 |
| 只需验证调用 | **Mock**（stub 调用） | `MockRunner` stub `Run` 返回预设事件流 |
| 框架接口难以 Fake | **Mock** | `Runner.Run` 返回 channel，用 mock 构造事件 |

#### 16.2.2 框架核心接口 Mock 模板

```go
// MockRunner 实现 runner.Runner
type MockRunner struct {
    events []*event.Event
    err    error
}

func (m *MockRunner) Run(ctx context.Context, userID, sessionID string, msg model.Message, opts ...runner.RunOption) (<-chan *event.Event, error) {
    if m.err != nil {
        return nil, m.err
    }
    ch := make(chan *event.Event, len(m.events))
    for _, e := range m.events {
        ch <- e
    }
    close(ch)
    return ch, nil
}
func (m *MockRunner) Close() error { return nil }

// MockModel 实现 model.Model
type MockModel struct {
    responses []*model.Response
}
func (m *MockModel) GenerateContent(ctx context.Context, req *model.Request) (<-chan *model.Response, error) {
    ch := make(chan *model.Response, len(m.responses))
    for _, r := range m.responses {
        ch <- r
    }
    close(ch)
    return ch, nil
}

// FakeMemoryService 实现 memory.Service（内存版）
type FakeMemoryService struct {
    mu      sync.Mutex
    entries map[string][]*memory.Entry
}
// ... 实现所有接口方法
```

#### 16.2.3 Mock 使用规则

| # | 规则 | 说明 |
|---|------|------|
| 1 | Mock 放 `*_test.go` 同包 | 不导出，仅测试用 |
| 2 | 禁止 mock 项目内部 Repo 用 mockgen | 项目 Repo 是接口，手写 Fake 更清晰 |
| 3 | 框架接口可用 mockgen | 框架接口稳定，mockgen 省力 |
| 4 | Mock 必须实现完整接口 | 编译期检查：`var _ runner.Runner = (*MockRunner)(nil)` |
| 5 | 测试用 `loggateway.NewNoop()` | 禁止 `loggateway.Global()`（deprecated） |

### 16.3 事件流测试

#### 16.3.1 事件序列验证

```go
func TestChatRun_EventSequence(t *testing.T) {
    runner := &MockRunner{
        events: []*event.Event{
            event.NewRunStatusEvent(event.RunStatusRunning),
            event.NewTextDeltaEvent("Hello"),
            event.NewTextDeltaEvent(" world"),
            event.NewToolResultEvent(toolResult),
            event.NewRunStatusEvent(event.RunStatusSucceeded),
        },
    }
    svc := NewChatService(runner, /* ... */)

    var collected []*event.Envelope
    svc.eventBus.Subscribe(func(ctx context.Context, e *event.Envelope) {
        collected = append(collected, e)
    })

    err := svc.Run(context.Background(), &chatv1.RunRequest{ /* ... */ })
    require.NoError(t, err)

    // 验证事件序列
    require.Len(t, collected, 5)
    assert.Equal(t, "run_status", collected[0].Type)
    assert.Equal(t, "text_delta", collected[1].Type)
    assert.Equal(t, "text_delta", collected[2].Type)
    assert.Equal(t, "tool_result", collected[3].Type)
    assert.Equal(t, "run_status", collected[4].Type)
}
```

#### 16.3.2 事件可靠性测试

```go
// Critical 事件：先写后发
func TestCriticalEvent_WriteBeforePublish(t *testing.T) {
    var publishCalled bool
    store := &FailingEventStore{failOnSave: true}
    bus := &RecordingEventBus{}
    svc := NewChatService(store, bus, /* ... */)

    svc.projectEvent(ctx, event.NewToolResultEvent(toolResult))

    // Store 失败时，不应发布
    assert.False(t, publishCalled)
    assert.Len(t, bus.events, 0)
}
```

### 16.4 状态机测试

见 §15.1.4，必须覆盖：
- 所有合法转换（正向测试）
- 所有非法转换被拒绝（反向测试）
- 终态无转换（边界测试）
- 守卫条件返回 false 时拒绝转换

### 16.5 并发测试

#### 16.5.1 Race Detector

```bash
# 所有测试必须通过 race detector
go test -race ./internal/biz/... -count=1
go test -race ./internal/service/... -count=1
```

#### 16.5.2 并发场景测试

```go
// 同一 Session 并发 StartRun 应拒绝
func TestStartRun_ConcurrentRejection(t *testing.T) {
    svc := NewChatService(/* ... */)
    ctx := context.Background()

    var wg sync.WaitGroup
    var successCount int32
    for i := 0; i < 10; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            err := svc.StartRun(ctx, &chatv1.StartRunRequest{SessionId: "same-session"})
            if err == nil {
                atomic.AddInt32(&successCount, 1)
            }
        }()
    }
    wg.Wait()

    // 不变量：同一 Session 只能有一个活跃 Run
    assert.Equal(t, int32(1), successCount)
}
```

### 16.6 测试覆盖率要求

| 层级 | 覆盖率目标 | 强制 |
|------|-----------|------|
| Biz 层业务逻辑 | ≥ 80% | 是 |
| Service 层事件投影 | ≥ 70% | 是 |
| Data 层 Repo | ≥ 60% | 建议 |
| 状态机 | 100%（所有转换） | 是 |
| 不变量 | 100%（所有不变量） | 是 |

---

## 第十七章：架构合理性验证（AS-FIT-01 落地）

> 本章节为 P1 优先级，暂占位。详细内容见 `project_rules.md` AS-FIT-01。
> 核心要求：依赖方向、分层隔离、接口窄化、状态机覆盖、认知复杂度必须通过自动化验证。
> 实现路径：`make archlint` → CI 集成 → golangci-lint 自定义规则。
