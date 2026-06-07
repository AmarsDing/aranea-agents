# Artifact 制品模块 — 实现设计文档

> 对应需求：`27 artifact.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`
> 更新日期：2026-06-06

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
internal/biz/artifact/              ← ArtifactUsecase + ArtifactReader/ArtifactWriter/ArtifactRepo 接口定义
    │
    ▼
internal/data/artifactfs/repo.go    ← FSArtifactRepo：本地文件系统实现
    │
    ▼
internal/artifact/trpc/service.go   ← ServiceAdapter：桥接 biz → trpc artifact.Service

internal/artifact/sign.go           ← HMAC-SHA256 签名/验签（签名下载 URL）；生产 fail-closed

internal/artifact/storage_factory.go ← 存储后端工厂：local/s3/cos 选择（S3/COS 委托 trpc-agent-go）

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
  rpc DeleteArtifactVersion(DeleteArtifactVersionRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = { delete: "/v1/artifacts/{id}/versions/{version}" };
  }
  rpc ListArtifactVersions(ListArtifactVersionsRequest) returns (ListArtifactVersionsResponse) {
    option (google.api.http) = { get: "/v1/artifacts/{id}/versions" };
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
| **ListArtifactsRequest** | session_id, limit, offset, query, mime_type_prefix | 列表请求；query 按名称模糊匹配，mime_type_prefix 按 MIME 前缀过滤 |
| **ListArtifactsResponse** | items (repeated ArtifactMeta), total | 列表响应 |
| **DeleteArtifactRequest** | id (REQUIRED) | 删除请求，删除该 ID 的所有版本 |
| **DeleteArtifactVersionRequest** | id (REQUIRED), version (REQUIRED) | 按版本删除请求 |
| **ListArtifactVersionsRequest** | id (REQUIRED) | 版本列表请求 |
| **ListArtifactVersionsResponse** | items (repeated ArtifactMeta) | 版本列表响应 |
| **PreviewArtifactRequest** | id (REQUIRED), version | 预览请求；返回按 MIME 类型分类的预览内容 |
| **PreviewArtifactResponse** | meta (ArtifactMeta), preview_kind, text_content, data_base64 | 预览响应；preview_kind: text / image / pdf / binary |
| **SignDownloadUrlRequest** | id (REQUIRED), version, ttl_seconds | 签名下载请求；ttl_seconds 最大 86400 |
| **SignDownloadUrlResponse** | url, expires_at | 签名下载响应；url 含 HMAC-SHA256 token + expires |

### 3.3 已实现（在线预览 / 签名下载）

PreviewArtifact 和 SignDownloadUrl RPC 已在 §3.1 中列出并实现。签名下载的实际文件流通过独立 HTTP 端点 `/v1/artifacts/download` 提供（非 gRPC），由 `ServeSignedDownload` 方法处理。

---

## 四、Biz 层

### 4.1 领域模型

文件：`internal/biz/artifact/`（子包，`internal/biz/artifact.go` 为 re-export 桥接层）

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

接口拆分为读写分离，`ArtifactRepo` 为组合接口：

```go
type Reader interface {
    Load(ctx context.Context, id string, version int) (Artifact, []byte, error)
    LoadMeta(ctx context.Context, id string, version int) (Artifact, error)
    List(ctx context.Context, sessionID string, limit, offset int) ([]Artifact, int, error)
    ListBySessionAndName(ctx context.Context, sessionID, name string) ([]Artifact, error)
}

type Writer interface {
    Save(ctx context.Context, sessionID, name, mimeType string, data []byte) (Artifact, error)
    Delete(ctx context.Context, id string) error
    DeleteVersion(ctx context.Context, id string, version int) error
}

type Repo interface {
    Reader
    Writer
}
```

### 4.3 Usecase

```go
type ArtifactUsecase struct { repo Repo }

func (uc *ArtifactUsecase) Save(ctx, sessionID, name, mimeType string, data []byte) (Artifact, error)
func (uc *ArtifactUsecase) Load(ctx, id string, version int) (Artifact, []byte, error)
func (uc *ArtifactUsecase) LoadMeta(ctx, id string, version int) (Artifact, error)
func (uc *ArtifactUsecase) List(ctx, sessionID string, limit, offset int, query, mimePrefix string) ([]Artifact, int, error)
func (uc *ArtifactUsecase) Delete(ctx, id string) error
func (uc *ArtifactUsecase) DeleteVersion(ctx, id string, version int) error
func (uc *ArtifactUsecase) ListVersions(ctx, sessionID, name string) ([]Artifact, error)
func (uc *ArtifactUsecase) StorageBytes(ctx) (int64, error)
func (uc *ArtifactUsecase) Preview(ctx, id string, version int) (PreviewResult, error)
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
- **LoadMeta**：仅加载元数据（不读取二进制文件）。
- **List**：去重保留每个 name 的最新版本，按 created_at 降序排列，支持分页。
- **Delete**：按 ID 前缀删除所有版本文件（bin + json）。
- **DeleteVersion**：按 ID + 版本号删除指定版本文件。
- **ListBySessionAndName**：返回指定 session+name 的全部版本，按版本号升序。
- **StorageBytes**：统计磁盘总字节数（近似）。

#### 已实现：存储后端工厂

文件：`internal/artifact/storage_factory.go`

`NewArtifactService()` 根据 `StorageConfig.Backend` 选择存储后端：
- `local`：返回 nil（走 FSArtifactRepo）
- `s3` / `cos`：委托 `trpc-agent-go/artifact/s3|cos` 框架包创建实例

S3/COS 的具体实现委托给 trpc-agent-go 框架包，项目自身不维护独立实现目录。

#### 待实现：S3 / COS 生产配置（Phase 4 — **后续支持**）

> 当前生产默认 `FSArtifactRepo`。S3/COS 工厂已存在但生产配置与多租户路径隔离依赖 M2（EP-WS-01）。

---

## 六、Service 层

文件：`internal/service/artifact.go`

| 方法 | 说明 |
|------|------|
| `UploadArtifact` | 校验 session_id/name 必填，base64 解码，**10 MB** 上限（超限返回明确错误），MIME 缺省 `application/octet-stream` |
| `GetArtifact` | 按 ID + version 加载，not found 返回 Kratos NotFound 错误 |
| `ListArtifacts` | limit 默认 50，返回去重后的最新版本列表；支持 `query` 名称模糊匹配 + `mime_type_prefix` MIME 前缀过滤 |
| `DeleteArtifact` | 按 ID 删除所有版本 |
| `DeleteArtifactVersion` | 按 ID + version 删除指定版本 |
| `ListArtifactVersions` | 按 ID 列出全部版本历史 |
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
| 类型 | `features/artifact/types.ts` | ArtifactMeta / ArtifactData / UploadArtifactInput / ListArtifactsParams（含 query/mime_type_prefix） / ListArtifactsResult / ArtifactPreview / SignDownloadUrlResult |
| API | `features/artifact/api.ts` | listArtifacts / getArtifact / uploadArtifact / deleteArtifact / deleteArtifactVersion / previewArtifact / signDownloadUrl / artifactDownloadHref / listArtifactVersions |
| Store | `stores/artifact/index.ts` | useArtifactStore：artifacts / total / loading / loadArtifacts / upload / get / remove / removeVersion / listVersions / signDownload / loadPreview / artifactDownloadHref |
| 预览组件 | `features/artifact/ArtifactPreview.vue` | 独立预览组件：图片 `<img>` / PDF `<iframe>` / 代码 `<pre>` + 下载按钮 |
| 列表组件 | `features/artifact/ArtifactList.vue` | 制品列表：MIME 图标 + 文件大小 + 版本号 + 预览弹窗 + 签名下载 |
| 上传对话框 | `components/artifact/ArtifactsUploadDialog.vue` | 上传对话框组件 |
| 详情对话框 | `components/artifact/ArtifactsDetailDialog.vue` | 详情对话框组件（含版本选择） |
| 管理页面 | `pages/ArtifactsPage.vue` | 完整管理页：列表/上传/预览/签名下载/删除，使用 ArtifactPreview + ArtifactsUploadDialog + ArtifactsDetailDialog |
| Chat 面板 | `components/chat/ChatSessionArtifactsPanel.vue` | Chat 会话制品面板，使用 ArtifactList 组件 |
| 消息附件 | `components/chat/ChatMessageAttachments.vue` | Chat 消息气泡内嵌附件 chip + 预览 Dialog |
| Composable | `features/artifact/useArtifactList.ts` | 列表 composable |
| Composable | `features/artifact/useArtifactsPage.ts` | 页面 composable |
| Composable | `features/artifact/useArtifactPreview.ts` | 预览 composable |
| 工具 | `features/artifact/limits.ts` | 上传限制常量 |
| 工具 | `features/artifact/fileBase64.ts` | Base64 编解码工具 |
| 工具 | `features/artifact/artifactTableUi.ts` | 表格 UI 配置 |
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
| `GET`  | `/v1/artifacts?session_id=…&query=…&mime_type_prefix=…` | 列出会话的制品元数据，支持名称模糊匹配和 MIME 前缀过滤 |
| `DELETE` | `/v1/artifacts/{id}` | 删除所有版本 |
| `DELETE` | `/v1/artifacts/{id}/versions/{version}` | 删除指定版本 |
| `GET`  | `/v1/artifacts/{id}/versions` | 列出制品的全部版本历史 |
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

- S3/COS 存储工厂已实现（委托 trpc-agent-go 框架包），但生产配置与多租户路径隔离待 Phase 4（依赖 M2 EP-WS-01）。
- 制品不在节点间复制。多实例部署需使用共享卷或配置 S3/COS 后端。
- `data_base64` 编码增加约 33% 开销；大文件（> 10 MB）应优先使用分块流式传输（计划中）。
- 签名密钥生产环境 fail-closed（缺少密钥拒绝服务），开发环境回退到不安全密钥。
