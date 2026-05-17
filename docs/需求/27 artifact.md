# M12: Artifact 制品 — 详细需求

> 对标 `pkg/trpc-agent-go/artifact` 包，实现制品存储和版本管理。
>
> **2026-05-17 现状对齐**（2026-05-17 二批复核）：
> - ✅ Biz / FS Repo / Service / **HTTP+gRPC 已注册**（`internal/server/http.go`、`grpc.go`）。
> - 🟡 **Runner 侧制品闭环**：Skill `CodeExecutor` 已注入（`trpc_build.go`）；工具产出物与 trpc `artifact.Service` 持久化联动仍弱。
> - ❌ S3 / COS 后端未启用，仅本地 FS；多租户路径隔离待 M2（EP-WS-01）。
>
> 进度以 `guides/execution-plan.md` 附录 A 为准。运维要点见 §6。

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

---

## 6. 运维指南

> 原 `guides/artifact.md` 内容，2026-05-17 合入。

Aranea-Agents 支持通过 Artifact Service 保存和检索 Agent 在会话中产生的二进制制品（文件）。

### 6.1 架构

```
Frontend (base64 HTTP)
    │
    ▼
internal/service/artifact.go   ← Kratos HTTP handler
    │
    ▼
internal/biz/artifact.go       ← ArtifactUsecase + ArtifactRepo interface
    │
    ▼
internal/data/artifactfs/      ← local filesystem implementation
    │  repo.go                 ← versioned storage in {artifact.dir}/<session>/<name>/v<N>.bin
    │  meta.json               ← per-artifact metadata sidecar
    │
    ▼
internal/artifact/trpc/        ← adapter for trpc-agent-go runtime
```

### 6.2 API

| 方法 | 路径 | 说明 |
|------|------|------|
| `POST` | `/v1/artifacts` | 上传（base64 编码） |
| `GET`  | `/v1/artifacts/{id}` | 下载（base64 编码） |
| `GET`  | `/v1/artifacts?session_id=…` | 列出会话的制品元数据 |
| `DELETE` | `/v1/artifacts/{id}` | 删除所有版本 |

#### 上传请求体

```json
{
  "session_id": "sess-abc123",
  "name": "report.pdf",
  "mime_type": "application/pdf",
  "data_base64": "<standard-base64>"
}
```

大小限制：每个制品 **50 MB**。

#### 响应 — ArtifactMeta

```json
{
  "id": "a9f3c1d2e4b5",
  "session_id": "sess-abc123",
  "name": "report.pdf",
  "mime_type": "application/pdf",
  "size": 204800,
  "sha256": "e3b0c44298fc…",
  "storage_kind": "fs",
  "storage_uri": "/data/artifacts/sess-abc123/report.pdf/v1.bin",
  "version": 1,
  "created_at": "2026-05-17T10:30:00Z"
}
```

### 6.3 存储配置

| 配置键 | 默认值 | 说明 |
|--------|--------|------|
| `data.artifact.dir` | `./data/artifacts` | 制品文件根目录 |

每个制品存储为：
```
{artifact.dir}/{session_id}/{name}/v{version}.bin
{artifact.dir}/{session_id}/{name}/meta.json
```

上传相同 `session_id` + `name` 的文件会创建新版本（`v2.bin`、`v3.bin`…）。

### 6.4 指标

| 指标 | 类型 | 标签 | 说明 |
|------|------|------|------|
| `aranea_artifact_upload_bytes_total` | Counter | — | 总上传字节数 |
| `aranea_artifact_download_bytes_total` | Counter | — | 总下载字节数 |
| `aranea_artifact_storage_bytes` | Gauge | — | 磁盘总字节数（近似） |

### 6.5 Agent 集成

Agent 通过 `trpc-agent-go` artifact service 访问制品：

```go
svc := ctx.Value(artifact.ServiceKey{}).(artifact.Service)

version, err := svc.SaveArtifact(ctx, sessionInfo, "output.csv", &artifact.Artifact{
    Data:     csvBytes,
    MimeType: "text/csv",
})

a, err := svc.LoadArtifact(ctx, sessionInfo, "output.csv", nil)
```

### 6.6 已知限制

- 二进制存储仅使用本地文件系统。S3/GCS 后端计划在 S6 实现。
- 制品不在节点间复制。多实例部署需使用共享卷。
- `data_base64` 编码增加约 33% 开销；大文件（> 10 MB）应优先使用分块流式传输（计划中）。
