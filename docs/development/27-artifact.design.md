# Artifact 制品模块 — 实现设计文档

> 对应需求：[27-artifact.md](./27-artifact.md)
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`
> 更新日期：2026-06-17

---

## 一、模块概述

制品存储与版本管理：Agent 运行产出物（文件/图片/代码）的持久化存储。对标 trpc-agent-go `artifact` 包，桥接到项目自身存储层，支持本地文件系统存储，预留 S3/COS 云存储扩展。

### 1.1 trpc-agent-go 框架参照

trpc-agent-go `artifact` 包定义了 Agent 运行时制品的核心抽象：

- **Artifact**：MIME 类型 + 二进制数据 + 可选 URL + 可选名称。
- **SessionInfo**：AppName + UserID + SessionID，标识制品归属。
- **Service 接口**：SaveArtifact / LoadArtifact / ListArtifactKeys / DeleteArtifact / ListVersions。

项目通过 `internal/artifact/trpc/service.go` 桥接该接口到自身存储层，使 Agent 运行时可透明使用制品能力。

---

## 二、架构

```
Frontend (base64 HTTP/gRPC)
    │
    ▼
internal/service/artifact.go        ← Kratos Service：proto ↔ biz 映射 + base64 编解码 + PreviewArtifact + SignDownloadUrl + ServeSignedDownload
    │
    ▼
internal/biz/artifact/              ← Usecase + Reader/Writer/Repo 接口 + TurnCollector + 附件解析 + 过滤 + 限制
    │
    ▼
internal/data/artifactfs/repo.go    ← FSArtifactRepo：本地文件系统实现
    │
    ▼
internal/artifact/trpc/service.go   ← ServiceAdapter：桥接 biz → trpc artifact.Service

internal/artifact/sign.go           ← Signer：HMAC-SHA256 签名/验签（生产 fail-closed）

internal/artifact/storage_factory.go ← 存储后端工厂：local/s3/cos 选择（S3/COS 委托 trpc-agent-go）

internal/skill/trpc/artifact_executor.go ← artifactSavingExecutor：CodeExecutor 产出物自动保存

internal/provider/media/persist.go     ← PersistingProvider：媒体生成产物（远程临时 URL）下载落盘 + artifact:// URL 重写

internal/service/system_reveal.go      ← POST /v1/system/reveal：本地打开制品所在文件夹（features.local_reveal_enabled 门控）
```

依赖方向：service → biz ← data，biz 层禁止 import trpc-agent-go。

---

## 三、Proto 层

### 3.1 服务定义

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
| **ListArtifactsRequest** | session_id, limit, offset, query, mime_type_prefix | 列表请求；query 按名称/session/mime 模糊匹配，mime_type_prefix 按 MIME 前缀过滤 |
| **ListArtifactsResponse** | items (repeated ArtifactMeta), total | 列表响应 |
| **DeleteArtifactRequest** | id (REQUIRED) | 删除请求，删除该 ID 的所有版本 |
| **DeleteArtifactVersionRequest** | id (REQUIRED), version (REQUIRED) | 按版本删除请求 |
| **ListArtifactVersionsRequest** | id (REQUIRED) | 版本列表请求 |
| **ListArtifactVersionsResponse** | items (repeated ArtifactMeta) | 版本列表响应 |
| **PreviewArtifactRequest** | id (REQUIRED), version | 预览请求；返回按 MIME 类型分类的预览内容 |
| **PreviewArtifactResponse** | meta (ArtifactMeta), preview_kind, text_content, data_base64 | 预览响应；preview_kind: text / image / pdf / binary |
| **SignDownloadUrlRequest** | id (REQUIRED), version, ttl_seconds | 签名下载请求；ttl_seconds 最大 86400 |
| **SignDownloadUrlResponse** | url, expires_at | 签名下载响应；url 含 HMAC-SHA256 token + expires |

### 3.3 签名下载端点

签名下载的实际文件流通过独立 HTTP 端点 `/v1/artifacts/download` 提供（非 gRPC），由 `ServeSignedDownload` 方法处理。

---

## 四、Biz 层

### 4.1 领域模型

文件：`internal/biz/artifact/artifact.go`（子包，`internal/biz/artifact.go` 为 re-export 桥接层，将 `artifact.Usecase` 别名为 `biz.ArtifactUsecase`）

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

接口拆分为读写分离，`Repo` 为组合接口：

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
    DeleteVersion(ctx context.Context, sessionID, name string, version int) error
}

type Repo interface {
    Reader
    Writer
}
```

### 4.3 Usecase

```go
type Usecase struct {
    repo Repo
    lg   loggateway.Logger
}

func NewUsecase(repo Repo, lg loggateway.Logger) *Usecase

func (uc *Usecase) Save(ctx, sessionID, name, mimeType string, data []byte) (Artifact, error)
func (uc *Usecase) Load(ctx, id string, version int) (Artifact, []byte, error)
func (uc *Usecase) LoadMeta(ctx, id string, version int) (Artifact, error)
func (uc *Usecase) List(ctx, sessionID string, limit, offset int, query, mimePrefix string) ([]Artifact, int, error)
func (uc *Usecase) Delete(ctx, id string) error
func (uc *Usecase) DeleteVersion(ctx, id string, version int) error
func (uc *Usecase) ListVersions(ctx, sessionID, name string) ([]Artifact, error)
func (uc *Usecase) StorageBytes(ctx) (int64, error)
func (uc *Usecase) Preview(ctx, id string, version int) (PreviewResult, error)
```

关键行为：
- **Save**：校验大小上限（`ValidateUploadSize`），保存后若有 TurnCollector 则自动收集引用。
- **Delete**：按 ID 解析 session+name，列出所有版本后逐个删除（逻辑制品删除）。
- **DeleteVersion**：按 ID 解析 session+name，再按版本号删除单版本。
- **List**：当 query/mimePrefix 非空时全量拉取后内存过滤（`FilterArtifacts`），再做分页。
- **Preview**：按 MIME 分类（text/image/pdf/binary），text 截断 512 KB。

### 4.4 子包辅助文件

| 文件 | 职责 |
|------|------|
| `limits.go` | `MaxUploadBytes`（10 MB）常量 + `ValidateUploadSize` + `ErrSizeExceeded` |
| `filter.go` | `FilterArtifacts(items, query, mimePrefix)` 内存过滤（名称/session/mime 模糊匹配 + MIME 前缀） |
| `turn_collector.go` | `TurnCollector` + `WithTurnCollector` / `CollectorFromContext`：单次 turn 产出物引用收集 |
| `attachments_resolve.go` | 附件解析：从制品 ID 列表加载元数据，校验 session 归属 |
| `options_merge.go` | 消息 options_json 合并附件引用 |

### 4.5 ID 生成

`NewArtifactID()` 使用 `crypto/rand` 生成 12 字节随机 hex（24 字符）。

### 4.6 预览分类

```go
type PreviewKind string

const (
    PreviewKindText   PreviewKind = "text"
    PreviewKindImage  PreviewKind = "image"
    PreviewKindPDF    PreviewKind = "pdf"
    PreviewKindBinary PreviewKind = "binary"
)

type PreviewResult struct {
    Meta        Artifact
    Kind        PreviewKind
    TextContent string // Kind == text 时填充
    Data        []byte // Kind == image/pdf 时填充
}
```

文本截断阈值：`maxTextPreviewBytes = 512 << 10`（512 KB）。

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
- **Delete**：按 ID 删除所有版本文件（bin + json）。
- **DeleteVersion**：按 session+name+version 删除指定版本文件。
- **ListBySessionAndName**：返回指定 session+name 的全部版本，按版本号升序。
- **StorageBytes**：统计磁盘总字节数（近似）。

#### 已实现：存储后端工厂

文件：`internal/artifact/storage_factory.go`

`NewArtifactService(ctx, cfg)` 根据 `StorageConfig.Backend` 选择存储后端：
- `local`：返回 nil（走 FSArtifactRepo）
- `s3`：委托 `trpc-agent-go/artifact/s3` 框架包创建实例
- `cos`：委托 `trpc-agent-go/artifact/cos` 框架包创建实例

`StorageConfig` 字段：

| 字段 | 说明 |
|------|------|
| `Backend` | `local` / `s3` / `cos` |
| `S3Bucket` / `S3Endpoint` / `S3Region` | S3 配置 |
| `S3AccessKey` / `S3SecretKey` | S3 凭据（可由环境变量 `ARTIFACT_S3_ACCESS_KEY` / `ARTIFACT_S3_SECRET_KEY` 覆盖） |
| `S3PathStyle` | S3 path-style 访问 |
| `COSBucketURL` / `COSSecretID` / `COSSecretKey` | COS 配置（可由环境变量 `ARTIFACT_COS_SECRET_ID` / `ARTIFACT_COS_SECRET_KEY` 覆盖） |

S3/COS 的具体实现委托给 trpc-agent-go 框架包，项目自身不维护独立实现目录。

#### 待实现：S3 / COS 生产配置（Phase 4 — **后续支持**）

> 当前生产默认 `FSArtifactRepo`。S3/COS 工厂已存在但生产配置与多租户路径隔离依赖 M2（EP-WS-01）。

---

## 六、Service 层

文件：`internal/service/artifact.go`

```go
type ArtifactService struct {
    v1.UnimplementedArtifactServiceServer
    uc     *biz.ArtifactUsecase
    signer *artifact.Signer
}

func NewArtifactService(uc *biz.ArtifactUsecase, signer *artifact.Signer) *ArtifactService
```

| 方法 | 说明 |
|------|------|
| `UploadArtifact` | 校验 session_id/name 必填，base64 解码，**10 MB** 上限（超限返回明确错误），MIME 缺省 `application/octet-stream`；记录 `aranea_artifact_upload_bytes_total` |
| `GetArtifact` | 按 ID + version 加载，not found 返回 Kratos NotFound 错误；记录 `aranea_artifact_download_bytes_total` |
| `ListArtifacts` | limit 默认 50，返回去重后的最新版本列表；支持 `query` 名称模糊匹配 + `mime_type_prefix` MIME 前缀过滤 |
| `DeleteArtifact` | 按 ID 删除所有版本；刷新存储 Gauge |
| `DeleteArtifactVersion` | 按 ID + version 删除指定版本；not found 返回 404 |
| `ListArtifactVersions` | 按 ID 解析 session+name 后列出全部版本历史 |
| `PreviewArtifact` | 按 MIME 分类预览：text（≤512KB 截断）/ image（base64）/ pdf（base64）/ binary |
| `SignDownloadUrl` | 生成 HMAC-SHA256 签名 URL，TTL 可配置（默认 15 分钟，最大 24 小时）；生产缺密钥返回 503 |
| `ServeSignedDownload` | 独立 HTTP handler，验证签名后流式返回二进制文件，设置 Content-Disposition |

---

## 七、签名下载

文件：`internal/artifact/sign.go`

### 7.1 Signer

```go
type Signer struct {
    lg       loggateway.Logger
    once     sync.Once
    cached   []byte
    cacheErr error
}

func NewSigner(lg loggateway.Logger) *Signer
func (s *Signer) SignKey() ([]byte, error)
func (s *Signer) DownloadToken(id string, version int, expires time.Time) (string, error)
func (s *Signer) VerifyDownloadToken(id string, version int, expiresUnix int64, token string) (bool, error)
```

### 7.2 密钥解析顺序

1. `KRATOS_ARTIFACT_SIGN_KEY` 环境变量
2. `KRATOS_AUTH_SECRET` 环境变量
3. 开发环境（`DEPLOY_ENV`/`KRATOS_ENV`/`APP_ENV` 为 dev/development/local/test）回退到不安全密钥 `aranea-artifact-dev-key`
4. 其他环境（含空/staging/prod）返回 `ErrSignKeyMissing`（fail-closed）

### 7.3 辅助函数

- `DefaultDownloadExpiry()`：默认过期时间（now + 15 分钟）
- `ParseExpires(raw)`：解析 expires 查询参数为 unix 时间戳

---

## 八、trpc 适配层

文件：`internal/artifact/trpc/service.go`

ServiceAdapter 桥接 `biz.ArtifactUsecase` → `trpcartifact.Service` 接口：

```go
type ServiceAdapter struct {
    uc *biz.ArtifactUsecase
}

var _ trpcartifact.Service = (*ServiceAdapter)(nil)
```

| trpc 方法 | 桥接逻辑 |
|-----------|----------|
| `SaveArtifact` | `uc.Save()` → 返回版本号 |
| `LoadArtifact` | `uc.ListVersions()` 按文件名查找 → `uc.Load()` 按 ID+版本加载 |
| `ListArtifactKeys` | `uc.List()` → 提取 name 列表 |
| `DeleteArtifact` | `uc.ListVersions()` → 逐个 `uc.Delete()` |
| `ListVersions` | `uc.ListVersions()` → 提取版本号列表 |

---

## 九、CodeExecutor 集成

文件：`internal/skill/trpc/artifact_executor.go`

`artifactSavingExecutor` 包装 `codeexecutor.CodeExecutor`，自动保存产出物：

```go
type artifactSavingExecutor struct {
    inner codeexecutor.CodeExecutor
    lg    loggateway.Logger
}

func WrapWithArtifactSave(inner codeexecutor.CodeExecutor, lg loggateway.Logger) codeexecutor.CodeExecutor
```

- 执行后遍历 `OutputFiles`，跳过空名/空内容/超 10 MB 的文件
- MIME 缺省时按文件扩展名推断（`mime.TypeByExtension`），再缺省 `application/octet-stream`
- 通过 `codeexecutor.SaveArtifactHelper(ctx, name, data, mimeType)` 保存

---

## 十、Wire 注入

### 10.1 已实现

```
data.ProviderSet  → NewArtifactRepo       (→ artifactfs.NewFSArtifactRepo)
biz.ProviderSet   → NewArtifactUsecase
service.ProviderSet → NewArtifactService
```

Wire 生成（`cmd/admin/wire_gen.go`）：
```
artifactRepo := data.NewArtifactRepo()
artifactUsecase := biz.NewArtifactUsecase(artifactRepo, logger)
artifactSigner := artifact.NewSigner(logger)
artifactService := service.NewArtifactService(artifactUsecase, artifactSigner)
```

HTTP/gRPC 注册：
- `http.go`：`artifactv1.RegisterArtifactServiceHTTPServer(srv, artifactSvc)`
- `http.go`：`/v1/artifacts/download` 签名下载路由（`ServeSignedDownload`）
- `grpc.go`：`artifactv1.RegisterArtifactServiceServer(srv, artifactSvc)`

### 10.2 Runner 注入

```
TRPCRunnerDeps.ArtifactService 字段已添加
NewTRPCRunner 中 trpcrunner.WithArtifactService(deps.ArtifactService) 注入
Wire provideArtifactRuntimeService 提供 trpcartifact.Service 实例
Wire provideArtifactSigner 提供 *artifact.Signer 实例
```

---

## 十一、Web 前端设计

### 11.1 已实现

| 层 | 文件 | 说明 |
|----|------|------|
| 类型 | `features/artifact/types.ts` | ArtifactMeta / ArtifactData / UploadArtifactInput / ListArtifactsParams（含 query/mime_type_prefix） / ListArtifactsResult / ArtifactPreview / SignDownloadUrlResult |
| API | `features/artifact/api.ts` | listArtifacts / getArtifact / uploadArtifact / deleteArtifact / deleteArtifactVersion / previewArtifact / signDownloadUrl / artifactDownloadHref / listArtifactVersions / revealArtifact / fetchLocalRevealEnabled |
| Store | `stores/artifact/index.ts` | useArtifactStore：artifacts / total / loading / loadArtifacts / upload / get / remove / removeVersion / listVersions / signDownload / loadPreview / artifactDownloadHref |
| 预览组件 | `features/artifact/ArtifactPreview.vue` | 独立预览组件：图片 `<img>` / PDF `<iframe>` / 代码 `<pre>` / 音频 `<audio controls>` / 视频 `<video controls>`（签名直链 `inline=1`）+ 下载按钮 |
| 上传对话框 | `components/artifact/ArtifactsUploadDialog.vue` | 上传对话框组件 |
| 详情对话框 | `components/artifact/ArtifactsDetailDialog.vue` | 详情对话框组件（含版本选择 + 完整落盘路径展示/复制 + reveal 按钮） |
| 管理页面 | `pages/ArtifactsPage.vue` | 资源管理器：会话产物 / 全部产物（按 session 分组）双 Tab + 路由 `?session=<id>` 过滤 + 上传/预览/签名下载/删除 |
| chat 入口 | `pages/ChatPage.vue` | chat 头部「产物」按钮 → `/artifacts?session=<id>`（原孤儿组件 `ChatSessionArtifactsPanel.vue` 方案已废弃并删除） |
| 消息附件 | `components/chat/ChatMessageAttachments.vue` | Chat 消息气泡内嵌附件 chip + 预览 Dialog |
| 媒体产物 | `components/chat/tools/MediaToolDetail.vue` | 媒体工具产出渲染，`artifact://` 经 `useMediaUrl()` 解析 |
| Composable | `features/chat/useMediaUrl.ts` | `artifact://<id>` → 新鲜签名直链解析（MediaToolDetail / MediaLightbox / NodeMediaPreview 共用） |
| Composable | `features/artifact/useArtifactsPage.ts` | 页面 composable（双 Tab / 分组 / 筛选 / 上传 / 详情） |
| Composable | `features/artifact/useArtifactPreview.ts` | 预览 composable（含 audio/video 分支） |
| 工具 | `features/artifact/limits.ts` | 上传限制常量 |
| 工具 | `features/artifact/fileBase64.ts` | Base64 编解码工具 |
| 工具 | `features/artifact/artifactTableUi.ts` | 表格 UI 配置 |
| 服务 | `services/index.ts` | createArtifactService → createArtifactServiceClient |

### 11.2 待实现

无。前端 P1–P3 功能已全部实现。

---

## 十二、运维指南

### 12.1 API

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
| `GET`  | `/v1/artifacts/download?id=…&token=…&expires=…&version=…` | 签名下载文件流（ServeSignedDownload）；`&inline=1` 时 `Content-Disposition: inline` 支持音视频内联播放 |
| `POST` | `/v1/system/reveal` | 本地打开制品所在文件夹（环境变量 `FEATURES_LOCAL_REVEAL_ENABLED` 默认关闭，未开启时路由不注册返回 404） |

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

### 12.2 存储配置

| 配置方式 | 键 | 默认值 | 说明 |
|----------|-----|--------|------|
| 环境变量 | `ARTIFACT_STORAGE_DIR` | `./data/artifacts` | 制品文件根目录 |
| 环境变量 | `KRATOS_ARTIFACT_SIGN_KEY` | — | 签名密钥（优先） |
| 环境变量 | `KRATOS_AUTH_SECRET` | — | 签名密钥（次选） |
| 环境变量 | `ARTIFACT_S3_ACCESS_KEY` / `ARTIFACT_S3_SECRET_KEY` | — | S3 凭据 |
| 环境变量 | `ARTIFACT_COS_SECRET_ID` / `ARTIFACT_COS_SECRET_KEY` | — | COS 凭据 |

每个制品存储为：
```
{root}/{session_id}/{artifact_id}-v{version}.bin
{root}/{session_id}/{artifact_id}-v{version}.json
```

上传相同 `session_id` + `name` 的文件会创建新版本（版本号递增）。

### 12.3 指标

| 指标 | 类型 | 标签 | 说明 |
|------|------|------|------|
| `aranea_artifact_upload_bytes_total` | Counter | — | 总上传字节数 |
| `aranea_artifact_download_bytes_total` | Counter | — | 总下载字节数 |
| `aranea_artifact_storage_bytes` | Gauge | — | 磁盘总字节数（近似） |

### 12.4 Agent 集成

Agent 通过 `trpc-agent-go` artifact service 访问制品：

```go
svc := ctx.Value(artifact.ServiceKey{}).(artifact.Service)

version, err := svc.SaveArtifact(ctx, sessionInfo, "output.csv", &artifact.Artifact{
    Data:     csvBytes,
    MimeType: "text/csv",
})

a, err := svc.LoadArtifact(ctx, sessionInfo, "output.csv", nil)
```

### 12.5 已知限制

- S3/COS 存储工厂已实现（委托 trpc-agent-go 框架包），但生产配置与多租户路径隔离待 Phase 4（依赖 M2 EP-WS-01）。
- 制品不在节点间复制。多实例部署需使用共享卷或配置 S3/COS 后端。
- `data_base64` 编码增加约 33% 开销；大文件（> 10 MB）应优先使用分块流式传输（计划中）。
- 签名密钥生产环境 fail-closed（缺少密钥返回 503），开发环境回退到不安全密钥。

---

## 十三、Phase 5：媒体产物持久化 + 资源管理器（设计）

> 背景：2026-07-22 多模态现状调研发现两个结构性缺陷——(1) 媒体生成工具产物仅存远程临时 URL（dashscope/ComfyUI），过期即失效且不进制品列表；(2) 音视频制品无内联播放。叠加资源管理诉求（本会话产物直达、跨会话浏览、落盘路径可见、本地打开文件夹），形成 Phase 5。

### 13.1 现状数据流（缺陷标注）

```
用户上传 → POST /v1/artifacts (base64) → FSArtifactRepo.Save
    → data/artifacts/<session>/<id>-v<n>.bin + .json
    → 消息 options_json.attachments[] 引用 → ChatMessageAttachments chip → ArtifactPreview

媒体工具 → MediaProvider.Generate* → MediaArtifact{ArtifactID:"qwen_<task>_0", URL:<远程临时URL>}
    → 仅写入 step ToolResult JSON → MediaToolDetail 缩略图 → MediaLightbox
    ✗ 不落盘、不进 TurnCollector、URL 过期后历史消息图片失效
```

### 13.2 媒体产物持久化（P0）

**方案：MediaProvider 装饰器**（对比：turn pipeline 后处理 tool result —— 需解析每种工具的结果 JSON，耦合脆弱，弃用）。

新增 `internal/provider/media/persist.go`：

```go
// PersistingProvider 包装 MediaProvider，将生成产物落盘到制品存储。
type PersistingProvider struct {
    inner    MediaProvider
    artifacts artifactbiz.Writer   // biz 窄接口，仅 Save
    http     *http.Client          // 抓取远程 URL，带超时
    lg       loggateway.Logger
}
```

- `GenerateImage/GenerateVideo/ImageToVideo` 调用 inner 后，对每个 `MediaArtifact`：
  1. `http.Get(URL)`（30s 超时，大小上限校验复用 `ValidateUploadSize`）
  2. sessionID 从 ctx 解析（trpc artifact session 上下文，与 `SaveArtifactHelper` 同一机制）
  3. `artifacts.Save(ctx, sessionID, name, mime, data)` → 经 `Usecase.Save` 自动进 TurnCollector → 挂到消息附件
  4. 成功：`ArtifactID` 换真实 ID，`URL` 换为 `artifact://<id>` 方案（见 13.3）
  5. 失败：记 Warn，保留原远程 URL（best-effort 降级，不阻断工具结果）
- 文件命名：`<tool>-<UTC时间戳>-<序号>.<ext>`（ext 由 mime 推导），同名冲突由版本机制自然处理。
- Wire：构造 `PersistingProvider` 包装现有 qwen/comfyui provider，注入点不变。

### 13.3 `artifact://` URL 方案（P0）

签名下载 URL 有 TTL（≤24h），不能长期嵌入历史消息。约定：

- 落盘产物的 `MediaArtifact.URL = "artifact://<artifact_id>"`。
- 前端渲染前经 `resolveMediaUrl(url)` 解析：以 `artifact://` 开头 → 调 `signDownloadUrl(id)` 取新鲜签名直链；`http(s)://` 原样渲染（兼容未落盘降级产物与历史数据）。
- 解析点：`MediaToolDetail.vue`、`MediaLightbox.vue`、`NodeMediaPreview.vue` 共用 composable `useMediaUrl()`。

### 13.4 音视频预览（P0）

后端：
- `PreviewKind` 增加 `audio` / `video`（`filter.go` 分类函数按 mime 前缀判定）。
- audio/video 的 PreviewArtifact **不返回 data_base64**（避免大文件 base64），响应仅含 meta + preview_kind；前端改走签名直链。
- 签名下载 handler `ServeSignedDownload` 增加 `inline=1` 参数：`Content-Disposition: inline` 并透传 `Content-Type`，支持 `<audio>/<video>` src 直接播放与 Range 请求（视频拖动）。

前端：
- `useArtifactPreview` + `ArtifactPreview.vue` 增加 audio/video 分支：`<audio controls>` / `<video controls>`，src 为 `artifactDownloadHref(id) + '&inline=1'`。

### 13.5 资源管理器（ArtifactsPage 升级，P1）

页面双视图 Tab：

| Tab | 数据源 | 说明 |
|-----|--------|------|
| 会话产物 | `listArtifacts(session_id)` | 现有表格，增强：详情对话框显示完整落盘路径 + 复制按钮 |
| 全部产物 | `listArtifacts(session_id="")` | 按 session 分组的分区列表（组头 = session ID + 产物数 + 总大小），复用现有跨会话查询能力，无需新 API |

- **路由参数**：`/artifacts?session=<id>` 进入时自动填充 session 筛选并切到「会话产物」Tab。
- **chat 入口**：chat 页头部（SpiritStatusBar 区域）加「产物」按钮 → `router.push({ name: 'artifacts', query: { session: 当前sessionID } })`。
- **完整路径**：service 层 `GetArtifact`/详情响应中 `storage_uri` 补绝对路径（`filepath.Abs(root + storage_uri)`，仅 local 后端）；前端 `ArtifactsDetailDialog` 展示 + 复制按钮。
- **孤儿组件处置**：已采用头部按钮方案；`ChatSessionArtifactsPanel.vue`（连同 `ArtifactList.vue` / `useArtifactList.ts`）已于 2026-07-22 清理删除。

### 13.6 本地打开文件夹（P2）

- 新端点 `POST /v1/system/reveal`，body `{ "artifact_id": "<id>" }`。
- Service：解析绝对路径 → **强制校验位于 artifact root 内**（`filepath.Rel` 防穿越）→ 按 OS 调起：
  - Windows: `explorer /select,<abs>`
  - macOS: `open -R <abs>`
  - Linux: `xdg-open <dir>`
- **开关**：环境变量 `FEATURES_LOCAL_REVEAL_ENABLED`（默认 `false`，`internal/conf/features_artifact.go`）；未开启时路由不注册（404），前端通过 `GET /v1/system/info` 的 `features.local_reveal` 字段探知后隐藏按钮（探知失败按未启用处理）。
- 纯浏览器/远程部署形态下降级为「复制路径」。

### 13.7 安全约束

| 项 | 约束 |
|----|------|
| reveal 路径 | 必须 `filepath.Rel(root, target)` 不以 `..` 开头，否则 400 |
| inline 播放 | 仅 `audio/*` `video/*` `image/*` 允许 inline；其余强制 attachment，防 HTML/JS 注入 |
| 媒体抓取 | 仅 http/https；30s 超时；大小上限复用 `ValidateUploadSize`（10MB）；失败降级不落盘 |
| local_reveal | 默认关闭；生产部署文档明确仅本地单机使用 |

### 13.8 改动文件清单（预估）

后端：
- `internal/provider/media/persist.go`（新增）+ Wire 注入（`cmd/admin` / `internal/runtime/deps.go`）
- `internal/biz/artifact/filter.go`（PreviewKind 分类扩展）
- `internal/service/artifact.go`（ServeSignedDownload inline 参数、storage_uri 绝对路径）
- `internal/service/system_reveal.go`（新增）+ `internal/server/http.go` 条件注册 + 配置项

前端：
- `web/src/features/artifact/ArtifactPreview.vue` + `useArtifactPreview.ts`（audio/video）
- `web/src/features/chat/useMediaUrl.ts`（新增 composable）；`MediaToolDetail.vue` / `MediaLightbox.vue` / `NodeMediaPreview.vue` 接入
- `web/src/pages/ArtifactsPage.vue` + `useArtifactsPage.ts`（Tab、路由 query、详情路径）
- chat 头部按钮（SpiritStatusBar 所在组件）
- `web/src/features/artifact/api.ts`（reveal 调用）
