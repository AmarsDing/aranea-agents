# Artifact 制品模块 — 实现设计文档

> 对应需求：`27 artifact.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`
> 更新日期：2026-05-21

---

## 一、模块概述

制品存储与版本管理：Agent 运行产出物（文件/图片/代码）的持久化存储。对标 trpc-agent-go `artifact` 包，桥接到项目自身存储层，支持本地文件系统存储，预留 S3/COS 云存储扩展。

---

## 二、架构

```
Frontend (base64 HTTP/gRPC)
    │
    ▼
internal/service/artifact.go        ← Kratos Service：proto ↔ biz 映射 + base64 编解码 + PreviewArtifact + SignDownloadUrl + ServeSignedDownload
    │
    ▼
internal/biz/artifact.go            ← ArtifactUsecase + ArtifactRepo 接口定义
    │
    ▼
internal/data/artifactfs/repo.go    ← FSArtifactRepo：本地文件系统实现
    │
    ▼
internal/artifact/trpc/service.go   ← ServiceAdapter：桥接 biz → trpc artifact.Service

internal/artifact/sign.go           ← HMAC-SHA256 签名/验签（签名下载 URL）

internal/skill/trpc/artifact_executor.go ← artifactSavingExecutor：CodeExecutor 产出物自动保存
```

依赖方向：service → biz ← data，biz 层禁止 import trpc-agent-go。

---

## 三、Proto 层

### 3.1 已实现

文件：`api/kratos/artifact/v1/artifact.proto`

```protobuf
service ArtifactService {
  rpc UploadArtifact(UploadArtifactRequest) returns (ArtifactMeta) {
    option (google.api.http) = { post: "/v1/artifacts" body: "*" };
  }
  rpc GetArtifact(GetArtifactRequest) returns (ArtifactData) {
    option (google.api.http) = { get: "/v1/artifacts/{id}" };
  }
  rpc ListArtifacts(ListArtifactsRequest) returns (ListArtifactsResponse) {
    option (google.api.http) = { get: "/v1/artifacts" };
  }
  rpc DeleteArtifact(DeleteArtifactRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = { delete: "/v1/artifacts/{id}" };
  }
  rpc PreviewArtifact(PreviewArtifactRequest) returns (PreviewArtifactResponse) {
    option (google.api.http) = { get: "/v1/artifacts/{id}/preview" };
  }
  rpc SignDownloadUrl(SignDownloadUrlRequest) returns (SignDownloadUrlResponse) {
    option (google.api.http) = { post: "/v1/artifacts/{id}/sign-download" body: "*" };
  }
}
```

### 3.2 消息定义

| 消息 | 字段 | 说明 |
|------|------|------|
| **ArtifactMeta** | id, session_id, name, mime_type, size, sha256, storage_kind, storage_uri, version, created_at | 制品元数据（无二进制载荷） |
| **ArtifactData** | meta (ArtifactMeta), data_base64 | 元数据 + base64 编码的二进制载荷 |
| **UploadArtifactRequest** | session_id (REQUIRED), name (REQUIRED), mime_type, data_base64 (REQUIRED) | 上传请求 |
| **GetArtifactRequest** | id (REQUIRED), version | 获取请求；version=0 返回最新版本 |
| **ListArtifactsRequest** | session_id, limit, offset | 列表请求 |
| **ListArtifactsResponse** | items (repeated ArtifactMeta), total | 列表响应 |
| **DeleteArtifactRequest** | id (REQUIRED) | 删除请求，删除该 ID 的所有版本 |
| **PreviewArtifactRequest** | id (REQUIRED), version | 预览请求；返回按 MIME 类型分类的预览内容 |
| **PreviewArtifactResponse** | meta (ArtifactMeta), preview_kind, text_content, data_base64 | 预览响应；preview_kind: text / image / pdf / binary |
| **SignDownloadUrlRequest** | id (REQUIRED), version, ttl_seconds | 签名下载请求；ttl_seconds 最大 86400 |
| **SignDownloadUrlResponse** | url, expires_at | 签名下载响应；url 含 HMAC-SHA256 token + expires |

### 3.3 已实现（在线预览 / 签名下载）

PreviewArtifact 和 SignDownloadUrl RPC 已在 §3.1 中列出并实现。签名下载的实际文件流通过独立 HTTP 端点 `/v1/artifacts/download` 提供（非 gRPC），由 `ServeSignedDownload` 方法处理。

---

## 四、Biz 层

### 4.1 领域模型

文件：`internal/biz/artifact.go`

```go
type Artifact struct {
    ID          string
    SessionID   string
    Name        string
    MimeType    string
    Size        int64
    SHA256      string
    StorageKind string
    StorageURI  string
    Version     int
    CreatedAt   string
}
```

### 4.2 Repo 接口

```go
type ArtifactRepo interface {
    Save(ctx context.Context, sessionID, name, mimeType string, data []byte) (Artifact, error)
    Load(ctx context.Context, id string, version int) (Artifact, []byte, error)
    List(ctx context.Context, sessionID string, limit, offset int) ([]Artifact, int, error)
    Delete(ctx context.Context, id string) error
    ListBySessionAndName(ctx context.Context, sessionID, name string) ([]Artifact, error)
}
```

### 4.3 Usecase

```go
type ArtifactUsecase struct { repo ArtifactRepo }

func (uc *ArtifactUsecase) Save(ctx, sessionID, name, mimeType string, data []byte) (Artifact, error)
func (uc *ArtifactUsecase) Load(ctx, id string, version int) (Artifact, []byte, error)
func (uc *ArtifactUsecase) List(ctx, sessionID string, limit, offset int) ([]Artifact, int, error)
func (uc *ArtifactUsecase) Delete(ctx, id string) error
func (uc *ArtifactUsecase) ListVersions(ctx, sessionID, name string) ([]Artifact, error)
```

### 4.4 ID 生成

`NewArtifactID()` 使用 `crypto/rand` 生成 12 字节随机 hex（24 字符）。

---

## 五、Data 层

### 5.1 存储后端

#### 已实现：FSArtifactRepo（本地文件系统）

文件：`internal/data/artifactfs/repo.go`

存储布局：
```
{root}/{session_id}/{artifact_id}-v{version}.bin   ← 二进制数据
{root}/{session_id}/{artifact_id}-v{version}.json  ← 元数据 sidecar
```

配置：

| 配置方式 | 键 | 默认值 |
|----------|-----|--------|
| 环境变量 | `ARTIFACT_STORAGE_DIR` | `data/artifacts` |

关键行为：
- **Save**：互斥锁保护，自动计算版本号递增，SHA-256 校验，JSON sidecar 写入。
- **Load**：version ≤ 0 返回最新版本；按 ID 前缀扫描所有 session 目录查找。
- **List**：去重保留每个 name 的最新版本，按 created_at 降序排列，支持分页。
- **Delete**：按 ID 前缀删除所有版本文件（bin + json）。
- **ListBySessionAndName**：返回指定 session+name 的全部版本，按版本号升序。

#### 待实现：S3 / COS 后端（Phase 4 — **后续支持**）

> 当前生产默认 `FSArtifactRepo`。S3/COS 实现可桥接 `pkg/trpc-agent-go/artifact/s3|cos`，通过配置选择后端；多租户路径隔离依赖 M2（EP-WS-01）。

---

## 六、Service 层

文件：`internal/service/artifact.go`

| 方法 | 说明 |
|------|------|
| `UploadArtifact` | 校验 session_id/name 必填，base64 解码，**10 MB** 上限（超限返回明确错误），MIME 缺省 `application/octet-stream` |
| `GetArtifact` | 按 ID + version 加载，not found 返回 Kratos NotFound 错误 |
| `ListArtifacts` | limit 默认 50，返回去重后的最新版本列表 |
| `DeleteArtifact` | 按 ID 删除所有版本 |
| `PreviewArtifact` | 按 MIME 分类预览：text（≤512KB 截断）/ image（base64）/ pdf（base64）/ binary |
| `SignDownloadUrl` | 生成 HMAC-SHA256 签名 URL，TTL 可配置（默认 15 分钟，最大 24 小时） |
| `ServeSignedDownload` | 独立 HTTP handler，验证签名后流式返回二进制文件，设置 Content-Disposition |

---

## 七、trpc 适配层

文件：`internal/artifact/trpc/service.go`

ServiceAdapter 桥接 `biz.ArtifactUsecase` → `trpcartifact.Service` 接口：

| trpc 方法 | 桥接逻辑 |
|-----------|----------|
| `SaveArtifact` | `uc.Save()` → 返回版本号 |
| `LoadArtifact` | `uc.ListVersions()` 按文件名查找 → `uc.Load()` 按 ID+版本加载 |
| `ListArtifactKeys` | `uc.List()` → 提取 name 列表 |
| `DeleteArtifact` | `uc.ListVersions()` → 逐个 `uc.Delete()` |
| `ListVersions` | `uc.ListVersions()` → 提取版本号列表 |

编译期接口校验：`var _ trpcartifact.Service = (*ServiceAdapter)(nil)`

---

## 八、Wire 注入

### 8.1 已实现

```
data.ProviderSet  → NewArtifactRepo       (→ artifactfs.NewFSArtifactRepo)
biz.ProviderSet   → NewArtifactUsecase
service.ProviderSet → NewArtifactService
```

Wire 生成（`cmd/admin/wire_gen.go`）：
```
artifactRepo := data.NewArtifactRepo()
artifactUsecase := biz.NewArtifactUsecase(artifactRepo)
artifactService := service.NewArtifactService(artifactUsecase)
```

HTTP/gRPC 注册：
- `http.go`：`artifactv1.RegisterArtifactServiceHTTPServer(srv, artifactSvc)`
- `http.go`：`/v1/artifacts/download` 签名下载路由（`ServeSignedDownload`）
- `grpc.go`：`artifactv1.RegisterArtifactServiceServer(srv, artifactSvc)`

### 8.2 已实现（Runner 注入）

```
TRPCRunnerDeps.ArtifactService 字段已添加
NewTRPCRunner 中 trpcrunner.WithArtifactService(deps.ArtifactService) 注入
Wire provideArtifactRuntimeService 提供 trpcartifact.Service 实例
```

---

## 九、Web 前端设计

### 9.1 已实现

| 层 | 文件 | 说明 |
|----|------|------|
| 类型 | `features/artifact/types.ts` | ArtifactMeta / ArtifactData / UploadArtifactInput / ListArtifactsParams / ListArtifactsResult / ArtifactPreview |
| API | `features/artifact/api.ts` | listArtifacts / getArtifact / uploadArtifact / deleteArtifact / previewArtifact / signDownloadUrl / artifactDownloadHref |
| Store | `stores/artifact/index.ts` | useArtifactStore：artifacts / total / loading / loadArtifacts / upload / get / remove |
| 预览组件 | `features/artifact/ArtifactPreview.vue` | 独立预览组件：图片 `<img>` / PDF `<iframe>` / 代码 `<pre>` + 下载按钮 |
| 列表组件 | `features/artifact/ArtifactList.vue` | 制品列表：MIME 图标 + 文件大小 + 版本号 + 预览弹窗 + 签名下载 |
| 管理页面 | `pages/ArtifactsPage.vue` | 完整管理页：列表/上传/预览/签名下载/删除，使用 ArtifactPreview 组件 |
| Chat 面板 | `components/chat/ChatSessionArtifactsPanel.vue` | Chat 会话制品面板，使用 ArtifactList 组件 |
| 服务 | `services/index.ts` | createArtifactService → createArtifactServiceClient |

### 9.2 待实现

无。前端 P1–P3 功能已全部实现。

---

## 十、运维指南

### 10.1 API

| 方法 | 路径 | 说明 |
|------|------|------|
| `POST` | `/v1/artifacts` | 上传（base64 编码） |
| `GET`  | `/v1/artifacts/{id}` | 下载（base64 编码），可选 `?version=N` |
| `GET`  | `/v1/artifacts?session_id=…` | 列出会话的制品元数据 |
| `DELETE` | `/v1/artifacts/{id}` | 删除所有版本 |
| `GET`  | `/v1/artifacts/{id}/preview` | 预览制品（按 MIME 分类返回），可选 `?version=N` |
| `POST` | `/v1/artifacts/{id}/sign-download` | 生成签名下载 URL |
| `GET`  | `/v1/artifacts/download?id=…&token=…&expires=…` | 签名下载文件流（ServeSignedDownload） |

#### 上传请求体

```json
{
  "session_id": "sess-abc123",
  "name": "report.pdf",
  "mime_type": "application/pdf",
  "data_base64": "<standard-base64>"
}
```

大小限制：每个制品 **10 MB**；超过限制返回 `400` 并提示不支持。流式上传为后续支持。

#### 响应 — ArtifactMeta

```json
{
  "id": "a9f3c1d2e4b5",
  "session_id": "sess-abc123",
  "name": "report.pdf",
  "mime_type": "application/pdf",
  "size": 204800,
  "sha256": "e3b0c44298fc…",
  "storage_kind": "local",
  "storage_uri": "/data/artifacts/sess-abc123/a9f3c1d2e4b5-v0.bin",
  "version": 0,
  "created_at": "2026-05-17T10:30:00Z"
}
```

### 10.2 存储配置

| 配置方式 | 键 | 默认值 | 说明 |
|----------|-----|--------|------|
| 环境变量 | `ARTIFACT_STORAGE_DIR` | `./data/artifacts` | 制品文件根目录 |

每个制品存储为：
```
{root}/{session_id}/{artifact_id}-v{version}.bin
{root}/{session_id}/{artifact_id}-v{version}.json
```

上传相同 `session_id` + `name` 的文件会创建新版本（版本号递增）。

### 10.3 指标

| 指标 | 类型 | 标签 | 说明 |
|------|------|------|------|
| `aranea_artifact_upload_bytes_total` | Counter | — | 总上传字节数 |
| `aranea_artifact_download_bytes_total` | Counter | — | 总下载字节数 |
| `aranea_artifact_storage_bytes` | Gauge | — | 磁盘总字节数（近似） |

### 10.4 Agent 集成

Agent 通过 `trpc-agent-go` artifact service 访问制品：

```go
svc := ctx.Value(artifact.ServiceKey{}).(artifact.Service)

version, err := svc.SaveArtifact(ctx, sessionInfo, "output.csv", &artifact.Artifact{
    Data:     csvBytes,
    MimeType: "text/csv",
})

a, err := svc.LoadArtifact(ctx, sessionInfo, "output.csv", nil)
```

### 10.5 已知限制

- 二进制存储仅使用本地文件系统。S3/COS 后端计划后续实现。
- 制品不在节点间复制。多实例部署需使用共享卷。
- `data_base64` 编码增加约 33% 开销；大文件（> 10 MB）应优先使用分块流式传输（计划中）。
