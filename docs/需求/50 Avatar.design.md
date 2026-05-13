# Avatar 头像模块 — 实现设计文档

> 对应需求：`50 Avatar.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

头像资源库：上传、裁剪、选择、存储。不设独立路由页，仅在 Agent 创建/编辑流程中弹出。

---

## 二、Proto 层

### 2.1 待新增

```protobuf
rpc ListAvatars(ListAvatarsRequest) returns (ListAvatarsResponse) {
  option (google.api.http) = { get: "/v1/avatars" };
}
rpc UploadAvatar(UploadAvatarRequest) returns (Avatar) {
  option (google.api.http) = { post: "/v1/avatars" body: "*" };
}
rpc DeleteAvatar(DeleteAvatarRequest) returns (google.protobuf.Empty) {
  option (google.api.http) = { delete: "/v1/avatars/{id}" };
}
rpc GetAvatarImage(GetAvatarImageRequest) returns (stream AvatarChunk) {
  option (google.api.http) = { get: "/v1/avatars/{id}/image" };
}
```

---

## 三、Biz 层

### 3.1 领域模型

```go
type AvatarAsset struct {
    ID        string
    Name      string
    MimeType  string  // "image/png"/"image/jpeg"/"image/svg+xml"
    Size      int64
    Width     int32
    Height    int32
    Data      []byte  // BLOB 存储
    CreatedAt string
}
```

### 3.2 Usecase

```go
func (uc *AvatarUsecase) List(ctx) ([]AvatarAsset, error)
func (uc *AvatarUsecase) Upload(ctx, name string, data []byte) (AvatarAsset, error)
func (uc *AvatarUsecase) Delete(ctx, id string) error
func (uc *AvatarUsecase) GetImage(ctx, id string) ([]byte, string, error)
```

### 3.3 引用检查

删除前检查是否有 Agent 引用该头像：

```go
func (uc *AvatarUsecase) Delete(ctx, id string) error {
    refs, _ := uc.agents.FindByIcon(ctx, id)
    if len(refs) > 0 {
        return errors.New("avatar is in use by agents")
    }
    return uc.repo.Delete(ctx, id)
}
```

---

## 四、Data 层

### 4.1 Ent Schema

文件：`internal/data/ent/schema/avatar_asset.go`

```go
func (AvatarAsset) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").DefaultFunc(uuid.NewString),
        field.String("name").NotEmpty(),
        field.String("mime_type").NotEmpty(),
        field.Int64("size").Default(0),
        field.Int("width").Default(0),
        field.Int("height").Default(0),
        field.Bytes("data"),
        field.String("created_at").Default(time.NowString),
    }
}
```

Agent 表 `icon` 字段存储 `avatar_assets.id`。

---

## 五、Service 层

```go
func (s *AvatarService) ListAvatars(ctx, req) (*ListAvatarsResponse, error)
func (s *AvatarService) UploadAvatar(ctx, req) (*Avatar, error)
func (s *AvatarService) DeleteAvatar(ctx, req) (*emptypb.Empty, error)
func (s *AvatarService) GetAvatarImage(ctx, req) (io.Reader, error)
```

---

## 六、Wire 注入

待新增：
```
data.ProviderSet → NewAvatarRepo
biz.ProviderSet → NewAvatarUsecase
service.ProviderSet → NewAvatarService
```

---

## 七、Web 前端设计

### 7.1 文件结构

```
web/src/features/agents/
└── components/
    └── AgentAvatarPicker.vue   ← 头像选择弹层
```

### 7.2 组件设计

**AgentAvatarPicker.vue**：

| 区域 | 组件 | 说明 |
|------|------|------|
| 预设头像 | `QAvatar` 网格 | 内置头像列表 |
| 上传 | `QFile` | 上传新头像 |
| 裁剪 | `cropperjs` | 裁剪上传图片 |
| 预览 | `QAvatar` | 当前选中 |

交互流程：
1. 点击 Agent 表单头像区域 → 弹出 Picker
2. 选择预设或上传 → 裁剪 → 确认
3. `agents.icon` 更新为 `avatar_assets.id`

### 7.3 API

```typescript
export async function listAvatars(): Promise<Avatar[]>
export async function uploadAvatar(file: File): Promise<Avatar>
export async function deleteAvatar(id: string): Promise<void>
export async function getAvatarUrl(id: string): string  // → /v1/avatars/{id}/image
```
