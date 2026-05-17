# M12: Artifact 制品 — 详细需求

> 对标 `pkg/trpc-agent-go/artifact` 包，实现制品存储和版本管理。
>
> **2026-05-17 现状对齐**：以下"现状分析"已被代码反超。当前实现状态：
> - ✅ `internal/biz/artifact.go` / `internal/data/artifactfs/repo.go` / `internal/service/artifact.go` / `internal/artifact/trpc/service.go` 已具备 Save / Load / List / Delete；REST `api/kratos/artifact/v1/*` 已生成。
> - ❌ **未在 `internal/server/http.go` / `grpc.go` 注册 ArtifactService**，对外不可访问。
> - ❌ **未把 trpc Artifact Service 注入 CodeExecutor / Runner**，工具产生的二进制结果仍丢失。
> - ❌ S3 / COS 后端未启用，仅本地 FS。
>
> 后续以 `guides/execution-plan.md` §3 EP-BIZ-03 为准；运维要点见 `guides/artifact.md`。

---

## 1. 现状分析（已过期，保留参考）

项目无 Artifact 制品管理能力。当前代码执行结果仅作为文本返回，无持久化存储。

---

## 2. trpc 框架参照

```
pkg/trpc-agent-go/artifact/
├── artifact.go    # Artifact 结构：MIME 类型 + 二进制内容
├── service.go     # Service 接口：Save/Load/List/Delete
├── cos/           # 腾讯云 COS 后端
│   ├── client.go
│   ├── option.go
│   └── service.go
├── s3/            # AWS S3 后端
│   ├── options.go
│   └── service.go
└── inmemory/      # 内存后端（测试用）
    └── service.go
```

### Service 接口

```go
type Service interface {
    SaveArtifact(ctx context.Context, sessionInfo SessionInfo, filename string, artifact *Artifact) (int, error)
    LoadArtifact(ctx context.Context, sessionInfo SessionInfo, filename string, version *int) (*Artifact, error)
    ListArtifactKeys(ctx context.Context, sessionInfo SessionInfo) ([]string, error)
    DeleteArtifact(ctx context.Context, sessionInfo SessionInfo, filename string) error
    ListVersions(ctx context.Context, sessionInfo SessionInfo, filename string) ([]int, error)
}
```

### SessionInfo

```go
type SessionInfo struct {
    AppName   string
    UserID    string
    SessionID string
}
```

### Artifact

```go
type Artifact struct {
    MIMEType string
    Data     []byte
}
```

---

## 3. 需求清单

### 3.1 Artifact Service 适配器

**需求**：桥接 trpc `artifact.Service` 到项目存储

**实现要点**：
- 新建 `internal/artifact/trpc/service.go`
- 先实现 InMemory 后端用于测试
- 后续增加 SQLite 本地存储后端
- 最终支持 S3/COS 云存储后端

**验收标准**：Artifact 可保存/加载/列出/删除

### 3.2 集成到 Runner

**需求**：Runner 支持注入 ArtifactService

**实现要点**：
- 在 `TRPCRunnerDeps` 中增加 `ArtifactService trpcartifact.Service`
- 在 `NewTRPCRunner` 中通过 `trpcrunner.WithArtifactService` 注入

**验收标准**：Agent 执行中可通过 Artifact 工具保存/加载制品

### 3.3 版本管理

**需求**：同一文件支持多版本

**实现要点**：
- `SaveArtifact` 返回递增的版本号
- `LoadArtifact` 支持指定版本，nil 返回最新版本
- `ListVersions` 列出所有版本号

**验收标准**：同一文件可保存多个版本，可按版本加载

### 3.4 CodeExecutor 产出物收集

**需求**：代码执行产出物自动保存为 Artifact

**实现要点**：
- `codeexecutor.CodeExecutionResult.OutputFiles` 自动保存
- 通过 `workspaceexec` 工具的 `save_artifact` 功能

**验收标准**：代码执行产生的文件自动保存为 Artifact

### 3.5 API 端点

**需求**：通过 REST API 管理制品

**实现要点**：
- `GET /artifacts?session_id=xxx` — 列出制品
- `GET /artifacts/:filename?version=N` — 加载制品
- `POST /artifacts/:filename` — 保存制品
- `DELETE /artifacts/:filename` — 删除制品

**验收标准**：通过 API 可管理制品的完整生命周期

### 3.6 S3/COS 云存储（超越层）

**需求**：支持云存储后端

**实现要点**：
- 集成 trpc `artifact/s3` 和 `artifact/cos` 包
- 配置文件增加存储后端选择
- 支持按租户配置不同存储后端

**验收标准**：制品可存储到 S3/COS，按租户隔离

---

## 4. 涉及文件

| 文件 | 操作 | 说明 |
|------|------|------|
| `internal/artifact/trpc/service.go` | 新建 | Artifact Service 适配器 |
| `internal/artifact/trpc/sqlite.go` | 新建 | SQLite 存储后端 |
| `internal/agent/trpc_runtime.go` | 修改 | Runner 注入 ArtifactService |
| `internal/service/artifact.go` | 新建 | Artifact 服务层 |
| `internal/server/register_artifact.go` | 新建 | Artifact HTTP 端点 |
| `web/src/features/artifacts/` | 新建 | 前端制品管理 |

---

## 5. 验收标准总览

1. Artifact 可保存/加载/列出/删除
2. 同一文件支持多版本管理
3. 代码执行产出物自动保存为 Artifact
4. 通过 API 可管理制品
5. 支持 S3/COS 云存储后端（超越层）
