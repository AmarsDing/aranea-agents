# Artifact 云存储

## 一、需求文档

### 1.2 背景

trpc-agent-go 框架提供了 Artifact Service 接口和两种云存储后端实现：S3（兼容 AWS S3/MinIO/R2 等）和 COS（腾讯云对象存储）。当前项目 `internal/artifact/` 有签名和 trpc 服务适配器，但底层使用本地 SQLite 存储（通过 `biz.ArtifactUsecase` + `biz.ArtifactRepo`）。生产环境需要云存储后端以支持大规模文件存储和 CDN 分发。

### 1.2 目标

- 集成框架 S3 后端，支持 AWS S3 / MinIO / Cloudflare R2 等 S3 兼容存储
- 集成框架 COS 后端，支持腾讯云对象存储
- 存储后端可配置切换（local / s3 / cos）
- 保持现有 `artifact.Service` 接口不变，仅替换底层实现

### 1.3 功能需求

| # | 功能 | 优先级 | 说明 |
|---|------|--------|------|
| F1 | S3 后端集成 | P0 | 实现 `artifact.Service` 接口，支持 S3 兼容存储 |
| F2 | COS 后端集成 | P0 | 实现 `artifact.Service` 接口，支持腾讯云 COS |
| F3 | 存储后端配置切换 | P0 | 通过环境变量/配置文件切换 local / s3 / cos |
| F4 | S3 连接配置 | P0 | endpoint / region / credentials / path_style 等 |
| F5 | COS 连接配置 | P0 | bucket_url / secret_id / secret_key 等 |
| F6 | 签名 URL 适配 | P1 | 云存储后端使用预签名 URL 替代本地签名 |
| F7 | 数据迁移 | P2 | 从 local 迁移到云存储的工具 |

### 1.4 非功能需求

- 云存储操作不得阻塞主服务（大文件上传走异步）
- 签名 URL 有效期可配置（默认 15 分钟）
- S3/COS 凭证通过环境变量注入，不硬编码
- 本地存储模式继续作为开发环境默认
- 文件版本号递增必须原子性

### 1.5 验收标准

- 配置 S3 后端后，Artifact 正确存储到 S3 兼容存储
- 配置 COS 后端后，Artifact 正确存储到腾讯云 COS
- 未配置云存储时，行为与现有本地存储一致
- SaveArtifact / LoadArtifact / ListArtifactKeys / DeleteArtifact / ListVersions 全部正常
- 签名 URL 可正确访问云存储文件

---

## 二、设计文档

### 2.1 框架参考（trpc-agent-go）

**核心包路径**：

| 子能力 | 包路径 |
|--------|--------|
| Artifact 定义 | `pkg/trpc-agent-go/artifact/artifact.go` |
| Service 接口 | `pkg/trpc-agent-go/artifact/service.go` |
| S3 后端 | `pkg/trpc-agent-go/artifact/s3/service.go` |
| COS 后端 | `pkg/trpc-agent-go/artifact/cos/service.go` |
| InMemory 后端 | `pkg/trpc-agent-go/artifact/inmemory/service.go` |

**核心类型和函数**：

```go
// artifact.Service — 存储服务接口
type Service interface {
    SaveArtifact(ctx context.Context, sessionInfo SessionInfo, filename string, artifact *Artifact) (int, error)
    LoadArtifact(ctx context.Context, sessionInfo SessionInfo, filename string, version *int) (*Artifact, error)
    ListArtifactKeys(ctx context.Context, sessionInfo SessionInfo) ([]string, error)
    DeleteArtifact(ctx context.Context, sessionInfo SessionInfo, filename string) error
    ListVersions(ctx context.Context, sessionInfo SessionInfo, filename string) ([]int, error)
}

// artifact.Artifact — 内容定义
type Artifact struct {
    Data     []byte `json:"data,omitempty"`
    MimeType string `json:"mime_type,omitempty"`
    URL      string `json:"url,omitempty"`
    Name     string `json:"name,omitempty"`
}

// artifact.SessionInfo — 会话信息
type SessionInfo struct {
    AppName   string
    UserID    string
    SessionID string
}

// s3.Service — S3 兼容后端
type Service struct {
    client     s3storage.Client
    ownsClient bool
    logger     log.Logger
}

func NewService(ctx context.Context, bucket string, opts ...Option) (*Service, error)

// s3.Option
func WithEndpoint(endpoint string) Option
func WithRegion(region string) Option
func WithCredentials(accessKeyID, secretAccessKey string) Option
func WithSessionToken(token string) Option
func WithPathStyle(enabled bool) Option
func WithRetries(n int) Option
func WithClient(client s3storage.Client) Option
func WithLogger(logger log.Logger) Option

// cos.Service — 腾讯云 COS 后端
type Service struct {
    cosClient client
}

func NewService(name, bucketURL string, opts ...Option) (*Service, error)

// cos.Option
func WithClient(client *cos.Client) Option
func WithHTTPClient(client *http.Client) Option
func WithTimeout(timeout time.Duration) Option
func WithSecretID(secretID string) Option
func WithSecretKey(secretKey string) Option
```

**S3 对象名格式**：
- Session 作用域：`{app_name}/{user_id}/{session_id}/{filename}/{version}`
- User 作用域：`{app_name}/{user_id}/user/{filename}/{version}`

**COS 对象名格式**：
- Session 作用域：`artifact/{app_name}/{user_id}/{session_id}/{filename}/{version}`
- User 作用域：`artifact/{app_name}/{user_id}/user/{filename}/{version}`

### 2.2 当前项目现状

| 位置 | 现状 |
|------|------|
| `internal/artifact/trpc/service.go` | `ServiceAdapter` 实现 `artifact.Service`，底层使用 `biz.ArtifactUsecase`（本地 SQLite） |
| `internal/artifact/sign.go` | HMAC-SHA256 签名机制，用于本地文件下载 URL |
| `internal/biz/artifact.go` | `ArtifactUsecase` + `ArtifactRepo` 接口，本地存储 |
| `internal/data/` | `ArtifactRepo` Ent 实现，SQLite 存储 |

**当前架构**：
```
artifact.Service 接口
  → ServiceAdapter（trpc/service.go）
    → biz.ArtifactUsecase
      → biz.ArtifactRepo（data 层 Ent 实现）
        → SQLite
```

### 2.3 架构设计

**模块在四层架构中的位置**：

```
internal/artifact        ← 新增云存储后端工厂
        ↓
internal/service         ← Wire 注入时根据配置选择后端
        ↓
internal/biz             ← ArtifactUsecase 保持不变（local 模式）
        ↓
internal/data            ← 本地存储实现保持不变
```

**新增/修改的文件清单**：

| 文件 | 操作 | 说明 |
|------|------|------|
| `internal/artifact/storage_factory.go` | 新增 | 存储后端工厂，根据配置创建 local/s3/cos Service |
| `internal/artifact/s3_adapter.go` | 新增 | S3 后端适配器（包装框架 s3.Service） |
| `internal/artifact/cos_adapter.go` | 新增 | COS 后端适配器（包装框架 cos.Service） |
| `internal/artifact/trpc/service.go` | 修改 | `NewServiceAdapter` 支持传入不同后端 |
| `internal/conf/` | 修改 | 配置文件新增 artifact 存储后端配置 |
| `cmd/admin/wire.go` | 修改 | Wire 注入根据配置选择后端 |

**接口设计**：

```go
// internal/artifact/storage_factory.go

type StorageBackend string

const (
    StorageBackendLocal StorageBackend = "local"
    StorageBackendS3    StorageBackend = "s3"
    StorageBackendCOS   StorageBackend = "cos"
)

type StorageConfig struct {
    Backend         StorageBackend `json:"backend"`
    S3Bucket        string         `json:"s3_bucket,omitempty"`
    S3Endpoint      string         `json:"s3_endpoint,omitempty"`
    S3Region        string         `json:"s3_region,omitempty"`
    S3AccessKey     string         `json:"s3_access_key,omitempty"`
    S3SecretKey     string         `json:"s3_secret_key,omitempty"`
    S3PathStyle     bool           `json:"s3_path_style,omitempty"`
    COSBucketURL    string         `json:"cos_bucket_url,omitempty"`
    COSSecretID     string         `json:"cos_secret_id,omitempty"`
    COSSecretKey    string         `json:"cos_secret_key,omitempty"`
}

func NewArtifactService(ctx context.Context, cfg StorageConfig, localUC *biz.ArtifactUsecase) (trpcartifact.Service, error)

// internal/artifact/s3_adapter.go

type S3ServiceAdapter struct {
    svc *s3.Service
}

func NewS3ServiceAdapter(ctx context.Context, cfg StorageConfig) (*S3ServiceAdapter, error)

// internal/artifact/cos_adapter.go

type COSServiceAdapter struct {
    svc *cos.Service
}

func NewCOSServiceAdapter(cfg StorageConfig) (*COSServiceAdapter, error)
```

**数据流图**：

```
配置文件 artifact.backend = "s3"
  → Wire 注入
    → NewArtifactService(ctx, cfg, localUC)
      ├─ local → ServiceAdapter{localUC}（现有）
      ├─ s3    → S3ServiceAdapter → s3.NewService(ctx, bucket, WithEndpoint, WithCredentials, ...)
      └─ cos   → COSServiceAdapter → cos.NewService(name, bucketURL, WithSecretID, WithSecretKey, ...)
        → artifact.Service 接口
          → Runner 装配时注入
            → Agent 运行时调用 SaveArtifact/LoadArtifact
```

### 2.4 与框架的集成方式

1. **直接使用框架实现**：`s3.NewService()` 和 `cos.NewService()` 直接构建框架的云存储 Service
2. **接口对齐**：框架的 `artifact.Service` 接口与项目 `ServiceAdapter` 实现的接口一致，无需适配
3. **配置注入**：S3/COS 的凭证和连接参数通过 `StorageConfig` 传递，环境变量优先
4. **签名 URL**：S3/COS 后端可使用预签名 URL（框架 `Artifact.URL` 字段），替代本地 HMAC 签名
5. **版本管理**：框架 S3/COS 实现自动管理版本号（通过 ListVersions 计算下一个版本）

### 2.5 错误处理

| 场景 | 处理方式 |
|------|----------|
| S3 连接失败 | `NewService` 返回 `"failed to create storage client"` 错误 |
| S3 上传失败 | `SaveArtifact` 返回 `"failed to upload artifact"` 错误 |
| S3 下载失败 | `LoadArtifact` 返回 `"failed to download artifact"` 错误，NotFound 返回 nil |
| COS 认证失败 | `NewService` 返回错误 |
| COS 上传失败 | `SaveArtifact` 返回 `"failed to upload artifact"` 错误 |
| COS 下载失败 | `LoadArtifact` 返回错误，NotFound 返回 nil |
| SessionInfo 字段为空 | `validateSessionInfo` 返回 `ErrEmptySessionInfo` |
| filename 为空或含非法字符 | `validateFilename` 返回 `ErrEmptyFilename` / `ErrInvalidFilename` |
| artifact 为 nil | 返回 `ErrNilArtifact` |
| 并发写同一文件 | S3/COS 可能版本号冲突，需外部同步或使用唯一文件名 |

---

## 三、开发计划

### 3.1 任务拆解

| 任务ID | 描述 | 依赖 | 预估复杂度 |
|--------|------|------|-----------|
| AS-01 | `internal/artifact/storage_factory.go`：存储后端工厂 | 无 | M |
| AS-02 | `internal/artifact/s3_adapter.go`：S3 后端适配器 | AS-01 | M |
| AS-03 | `internal/artifact/cos_adapter.go`：COS 后端适配器 | AS-01 | M |
| AS-04 | `internal/conf/`：配置文件新增 artifact 存储后端配置 | AS-01 | S |
| AS-05 | `internal/artifact/trpc/service.go`：`NewServiceAdapter` 支持不同后端 | AS-01 | S |
| AS-06 | `cmd/admin/wire.go`：Wire 注入根据配置选择后端 | AS-02, AS-03, AS-05 | M |
| AS-07 | 签名 URL 适配（S3 预签名 / COS 临时 URL） | AS-02, AS-03 | M |
| AS-08 | 单元测试：工厂、适配器 | AS-02, AS-03 | M |
| AS-09 | 集成测试：S3/COS 端到端（使用 MinIO mock） | AS-06 | L |
| AS-10 | `make wire` 更新 Wire 注入 | AS-06 | S |

### 3.2 开发顺序

```
AS-01 → AS-02 ─┐
        AS-03 ─┤→ AS-05 → AS-06 → AS-07 → AS-08 → AS-09 → AS-10
        AS-04 ─┘
```

### 3.3 验证方案

| 验证项 | 方法 |
|--------|------|
| 工厂函数 | `go test ./internal/artifact/... -run TestStorageFactory -count=1` |
| S3 适配器 | `go test ./internal/artifact/... -run TestS3Adapter -count=1` |
| COS 适配器 | `go test ./internal/artifact/... -run TestCOSAdapter -count=1` |
| 本地存储兼容 | `go test ./internal/artifact/... -run TestLocal -count=1` |
| Wire 注入 | `make wire && go build ./cmd/admin` |
| 全量验证 | `make wire && make build && make test` |
