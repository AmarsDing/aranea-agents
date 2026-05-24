# Kratos 框架层职责速查

> **定位**：本文定义 Kratos v2 各层的**职责边界与约束**，是 AI 编码时判断"代码该放哪层"的框架性参考。
> **与 SPEC 的关系**：[AI-DEVELOPMENT-SPECIFICATION.md](./AI-DEVELOPMENT-SPECIFICATION.md) 是唯一行为准则，本文是其第二章的框架性展开。项目特有约束以 SPEC 为准。
> **不包含**：通用教程、beer-shop 示例代码、中间件实现细节——这些需要时查阅 Kratos 官方文档。

---

## 目录

- [1. 分层架构总览](#1-分层架构总览)
- [2. 各层职责与约束](#2-各层职责与约束)
- [3. 依赖方向铁律](#3-依赖方向铁律)
- [4. API 与 Proto 规范](#4-api-与-proto-规范)
- [5. Wire 依赖注入](#5-wire-依赖注入)
- [6. 中间件体系](#6-中间件体系)
- [7. 错误处理](#7-错误处理)
- [8. 配置管理](#8-配置管理)

---

## 1. 分层架构总览

```
┌──────────────────────────────────────────────┐
│  api/**/*.proto                              │
│  唯一对外契约：RPC + HTTP 注解 + 错误枚举     │
└──────────────┬───────────────────────────────┘
               ↓ 代码生成
┌──────────────▼───────────────────────────────┐
│  Service 层 (Application)                     │
│  实现 proto 接口；DTO ↔ DO 转换；协调 UseCase  │
│  项目特有：Runner 装配入口                      │
└──────────────┬───────────────────────────────┘
               │ 调用
┌──────────────▼───────────────────────────────┐
│  Biz 层 (Domain)                              │
│  领域实体定义；Repo 接口定义（依赖倒置）         │
│  业务逻辑 (UseCase)；业务错误变量               │
└──────────────┬───────────────────────────────┘
               │ 依赖接口
┌──────────────▼───────────────────────────────┐
│  Data 层 (Infrastructure)                     │
│  实现 Repo 接口；数据库访问 (Ent ORM)           │
│  PO ↔ DO 转换；gRPC 客户端（BFF 场景）         │
└──────────────────────────────────────────────┘
```

**跨层只允许向内依赖。违反即停。**

---

## 2. 各层职责与约束

### 2.1 Service 层

| 维度 | 规则 |
|------|------|
| **职责** | 实现 proto 接口；proto ↔ biz 类型映射；协调 biz Usecase；Runner 装配 |
| **结构体** | 嵌入 `v1.UnimplementedXxxServiceServer`；构造函数只接收 `*biz.XxxUsecase` |
| **类型转换** | 独立函数 `toProtoXxx`（biz→proto）、`fromProtoXxx`（proto→biz） |
| **错误映射** | `kerrors.FromError(err)` 或 `kerrors.BadRequest/InternalServer` |
| **禁止** | 写业务逻辑（if/for 判断）；直接 import `internal/data`；绕过 `internal/tools` 拼装底层 tool |

### 2.2 Biz 层

| 维度 | 规则 |
|------|------|
| **职责** | 定义领域模型（纯 Go struct）；定义 Repo 接口（依赖倒置）；Usecase 编排 |
| **模型** | 纯 Go struct，字段用基本类型，不用 proto 类型 |
| **Repo 接口** | 在 biz 中定义，data 层实现；`var _ biz.XxxRepo = (*xxxRepo)(nil)` 编译期检查 |
| **错误** | 统一使用 `kerrors.BadRequest/NotFound/InternalServer`，禁止 `fmt.Errorf` |
| **禁止** | import `api/*/v1`；import `pkg/trpc-agent-go` 任何包；依赖框架运行时 toolset/skill 类型 |

### 2.3 Data 层

| 维度 | 规则 |
|------|------|
| **职责** | 实现 biz 定义的 Repo 接口；封装数据库操作；PO ↔ DO 转换 |
| **数据库访问** | 仅通过 `d.Ent()`（SQLite）/ `d.Postgres()`（pgvector），禁止另开连接 |
| **Ent 转换** | `entXxxToBiz` / `bizXxxToEnt`，放在对应 Repo 文件中 |
| **Data 结构** | 持有 `*ent.Client`、`*sql.DB`（pgvector）；返回清理函数 `func()` |
| **禁止** | 在 `NewData` 外另开 SQLite `sql.Open`；长期维护不进 Ent 的裸 SQL |

### 2.4 Server 层

| 维度 | 规则 |
|------|------|
| **职责** | 创建 HTTP/gRPC/WebSocket 实例；注册 Service；注册中间件 |
| **注册** | `v1.RegisterXxxHTTPServer(srv, svc)` / `v1.RegisterXxxServiceServer(srv, svc)` |
| **中间件** | 统一在 `NewHTTPServer`/`NewGRPCServer` 中注册 |
| **禁止** | 写业务路由或手写 `HandleFunc`；new `runner.Runner` 或 `llmagent.New` |

---

## 3. 依赖方向铁律

```
api/**/*.proto          ← 唯一对外契约
        ↓
internal/service        ← 传输桥点
        ↓
internal/biz            ← 领域核心（禁止 import trpc-agent-go、api/*/v1）
        ↓
internal/data           ← Repo 实现
```

**逐包 import 规则**（详见 [AI-DEVELOPMENT-SPECIFICATION.md §1.3](./AI-DEVELOPMENT-SPECIFICATION.md#13-逐包-import-规则)）：

**Kratos 标准 4 层**：

| 包 | 可 import | 不可 import |
|----|-----------|-------------|
| `server/*` | service, conf, kratos, pkg/auth | trpc-agent-go, runner, llmagent |
| `biz/*` | stdlib, kratos errors, 本仓 biz/data API | trpc-agent-go, api/*/v1 |
| `service/*` | biz, 项目扩展模块, 框架装配 API | 绕过 tools 直连拼装底层 tool |
| `data/*` | biz（实现 Repo 接口）, conf, Ent, pgvector | api/*/v1, trpc-agent-go |

**项目扩展模块**：

| 包 | 可 import | 不可 import |
|----|-----------|-------------|
| `agent/*` | biz, provider, data(如需), session/trpc, trpc-agent-go | — |
| `team/*` | biz, agent, provider, tools, trpc-agent-go | api/*/v1 |
| `channel/*` | biz, channel/port, event | 对方 Service 具体类型, api/*/v1 |
| `graph/adapter` | biz, agent, event | 无关业务 Usecase |
| `provider/*` | biz, trpc-agent-go model 适配 | — |
| `tools/*` | biz, trpc-agent-go tool API | — |

---

## 4. API 与 Proto 规范

### 4.1 Proto 文件规范

| 规则 | 规范 |
|------|------|
| 路径 | `api/kratos/<module>/v1/<module>.proto` |
| package | `<模块>.service.v1` 或 `<模块>.interface.v1` |
| go_package | `api/<路径>;v1` |
| HTTP 注解 | 每个 RPC 配 `google.api.http` |
| 必填标记 | `(google.api.field_behavior) = REQUIRED` |
| 命名 | proto 字段 `snake_case`，Go 生成 `CamelCase` |
| 请求/响应 | `XxxReq` / `XxxReply` |

### 4.2 错误枚举

```protobuf
enum ErrorReason {
    option (errors.default_code) = 500;
    NOT_FOUND = 0 [(errors.code) = 404];
    INVALID_ARG = 1 [(errors.code) = 400];
}
```

### 4.3 代码生成

```bash
make api     # 生成 Go + TypeScript
make config  # 仅改 conf.proto 时
```

**禁止修改工具生成的代码。**

### 4.4 服务分类

| 类型 | 暴露协议 | package 命名 | 说明 |
|------|----------|--------------|------|
| 内部服务 | 仅 gRPC | `xxx.service.v1` | 供其他微服务调用 |
| BFF 接口 | HTTP + gRPC | `xxx.interface.v1` / `xxx.admin.v1` | 面向前端/管理后台 |

---

## 5. Wire 依赖注入

### 5.1 各层 ProviderSet

```go
// server/server.go
var ProviderSet = wire.NewSet(NewHTTPServer, NewGRPCServer)

// biz/biz.go
var ProviderSet = wire.NewSet(NewXxxUseCase)

// data/data.go
var ProviderSet = wire.NewSet(NewData, NewXxxRepo)

// service/service.go
var ProviderSet = wire.NewSet(NewXxxService)
```

### 5.2 Wire 声明

```go
//go:build wireinject

func wireApp(*conf.Server, *conf.Data, log.Logger) (wireOut, func(), error) {
    panic(wire.Build(
        server.ProviderSet, data.ProviderSet,
        biz.ProviderSet, service.ProviderSet, newApp,
    ))
}
```

### 5.3 规则

1. `wire.go` 必须有 `//go:build wireinject` 构建标签
2. 运行 `make wire` 生成 `wire_gen.go`
3. **禁止手动编辑 `wire_gen.go`**
4. 每层新增构造器后，需更新对应层的 `ProviderSet`
5. **`wire.go` 禁止全局 bootstrap**：不得在 provider 内调用 `SetGlobal*` / `SetCredentialKeyResolver` / `mcpobserve.Set*`；副作用放进 `data`/`biz`/`service` 的 `New*` 或 `main` 生命周期钩子（详见 [AI-DEVELOPMENT-SPECIFICATION.md §5.4](./AI-DEVELOPMENT-SPECIFICATION.md#54-依赖注入)）
6. 改 Wire 后本地 **`make wire-clean`**；PR 须通过 CI `wire-clean` job
7. **`make lint` R11** 自动扫描 `cmd/admin/wire.go` 中的 bootstrap 反模式

---

## 6. 中间件体系

### 6.1 推荐注册顺序

```
recovery → tracing → logging → auth
```

请求进入按注册顺序执行，响应返回按倒序执行（FILO）。

### 6.2 选择性中间件（selector）

对特定路由定制中间件（如白名单免认证）：

```go
selector.Server(
    jwt.Server(func(token *jwt2.Token) (interface{}, error) {
        return []byte(ac.ApiKey), nil
    }),
).Match(NewWhiteListMatcher()).Build()
```

**注意**：selector 匹配的是 **operation**（gRPC path，格式 `/包名.服务名/方法名`），不是 HTTP 路由。

### 6.3 规则

- 中间件只在 `internal/server` 注册
- 禁止把 Kratos middleware 逻辑复制进 `pkg/trpc-agent-go`
- Client 和 Server 都注册 tracing 中间件，确保全链路追踪

---

## 7. 错误处理

### 7.1 错误结构

| 字段 | 类型 | 说明 |
|------|------|------|
| `code` | int | HTTP Status Code |
| `reason` | string | 业务错误码，同一服务内唯一 |
| `message` | string | 用户可读信息 |
| `metadata` | map | 附加信息 |

### 7.2 创建错误

```go
kerrors.BadRequest("AGENT", "id is required")
kerrors.NotFound("AGENT", "agent not found")
kerrors.InternalServer("AGENT", err.Error())
kerrors.FromError(err)
```

### 7.3 Biz 层错误变量

```go
var (
    ErrNotFound     = kerrors.NotFound("AGENT", "agent not found")
    ErrInvalidInput = kerrors.BadRequest("AGENT", "invalid input")
)
```

### 7.4 Proto 错误枚举

```go
v1.ErrorXxxFailed("message: %s", detail)
if v1.IsXxxFailed(err) { /* handle */ }
```

---

## 8. 配置管理

### 8.1 配置结构

在 `internal/conf/conf.proto` 中定义，保证类型安全。

### 8.2 配置加载

```go
c := config.New(config.WithSource(file.NewSource(flagconf)))
if err := c.Load(); err != nil { panic(err) }
var bc conf.Bootstrap
if err := c.Scan(&bc); err != nil { panic(err) }
```

### 8.3 优先级

环境变量 > 系统设置 > 配置文件 > 代码默认值

### 8.4 热更新

通过 Kratos config source 支持，不自行实现 watch。
