# Artifact 制品模块 — 实现设计文档

> 对应需求：`27 artifact.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

制品存储与版本管理：Agent 运行产出物（文件/图片/代码）的持久化存储。对标 trpc-agent-go `artifact` 包，支持 S3/COS/本地存储。

---

## 二、Proto 层

### 2.1 待新增

```protobuf
service ArtifactService {
  rpc ListArtifacts(ListArtifactsRequest) returns (ListArtifactsResponse) {
    option (google.api.http) = { get: "/v1/artifacts" };
  }
  rpc GetArtifact(GetArtifactRequest) returns (Artifact) {
    option (google.api.http) = { get: "/v1/artifacts/{id}" };
  }
  rpc SaveArtifact(SaveArtifactRequest) returns (Artifact) {
    option (google.api.http) = { post: "/v1/artifacts" body: "*" };
  }
  rpc DeleteArtifact(DeleteArtifactRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = { delete: "/v1/artifacts/{id}" };
  }
  rpc DownloadArtifact(DownloadArtifactRequest) returns (stream ArtifactChunk) {
    option (google.api.http) = { get: "/v1/artifacts/{id}/download" };
  }
}
```

---

## 三、Biz 层

### 3.1 领域模型

```go
type Artifact struct {
    ID        string
    SessionID string
    AgentID   string
    Name      string
    MimeType  string
    Size      int64
    StorageRef string  // 存储路径
    Version   int32
    CreatedAt string
}

type ArtifactService interface {
    Save(ctx, artifact Artifact, data []byte) (Artifact, error)
    Load(ctx, id string) ([]byte, error)
    List(ctx, query) ([]Artifact, error)
    Delete(ctx, id string) error
}
```

---

## 四、Data 层

### 4.1 存储后端

```go
// internal/artifact/local/service.go
type LocalService struct {
    basePath string
}

// internal/artifact/s3/service.go
type S3Service struct {
    client *s3.Client
    bucket string
}

// internal/artifact/cos/service.go
type COSService struct {
    client *cos.Client
    bucket string
}
```

### 4.2 Ent Schema

- `internal/data/ent/schema/artifact.go`

---

## 五、Service 层

```go
func (s *ArtifactService) SaveArtifact(ctx, req) (*Artifact, error)
func (s *ArtifactService) GetArtifact(ctx, req) (*Artifact, error)
func (s *ArtifactService) ListArtifacts(ctx, req) (*ListArtifactsResponse, error)
func (s *ArtifactService) DeleteArtifact(ctx, req) (*emptypb.Empty, error)
```

---

## 六、Wire 注入

待新增：
```
data.ProviderSet → NewArtifactRepo
biz.ProviderSet → NewArtifactUsecase
service.ProviderSet → NewArtifactService
artifact.ProviderSet → NewArtifactStorage
```

---

## 七、Web 前端设计

### 7.1 组件

**ArtifactList.vue**：制品列表（嵌入 Chat 和 Session 页面）

**ArtifactPreview.vue**：制品预览（图片/PDF/代码）

### 7.2 API

```typescript
export async function listArtifacts(query: ArtifactQuery): Promise<ArtifactListResult>
export async function downloadArtifact(id: string): Promise<Blob>
export async function deleteArtifact(id: string): Promise<void>
```
