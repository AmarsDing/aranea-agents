# Avatar 头像模块 — 实现设计文档

> 对应需求：`50 Avatar.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

头像资源库：上传、裁剪、选择、存储。不设独立路由页，仅在 Agent 创建/编辑流程中弹出。图片像素数据直接落在数据库（BLOB），不依赖对象存储或本地文件目录。

### 当前实现状态

| 层 | 状态 | 文件 |
|----|------|------|
| Proto | ✅ 已有 | `api/kratos/avatar/v1/avatar.proto` |
| Biz | ✅ 已有 | `internal/biz/avatar.go` |
| Data | ✅ 已有 | `internal/data/avatar.go`, `internal/data/ent/schema/avatar_asset.go` |
| Service | ✅ 已有 | `internal/service/avatar.go` |
| Wire | ✅ 已有 | 已注册 |
| Web API | ✅ 已有 | `web/src/features/avatar/api.ts` |
| Web Store | ✅ 已有 | `web/src/stores/avatar/index.ts` |
| Web 组件 | ✅ 已有 | `web/src/components/avatar/AgentAvatarPicker.vue`, `ResolvedAvatarImg.vue`, `AgentAvatarQ.vue` |

### 需要增强的功能

| 功能 | 说明 | 优先级 |
|------|------|--------|
| 图片裁剪 | 上传后支持 1:1 裁剪（vue-advanced-cropper） | P1 |
| 缩略图自动生成 | 上传时服务端自动生成 64×64 缩略图 | P2 |
| Agent 引用检查 | 删除前检查是否有 Agent 引用该头像 | P3 |
| 图片压缩 | 服务端上传时压缩至 256px 正方形 | P4 |

---

## 二、Proto 层

### 2.1 现有 Proto（已完成）

```protobuf
// api/kratos/avatar/v1/avatar.proto

service AvatarService {
  rpc ListAvatarAssets(ListAvatarAssetsRequest) returns (ListAvatarAssetsResponse) {
    option (google.api.http) = { get: "/v1/avatar-assets" };
  }
  rpc CreateAvatarAsset(CreateAvatarAssetRequest) returns (AvatarAsset) {
    option (google.api.http) = { post: "/v1/avatar-assets" body: "*" };
  }
  rpc GetAvatarFile(GetAvatarBlobRequest) returns (GetAvatarBlobResponse) {
    option (google.api.http) = { get: "/v1/avatar-assets/{id}/file" };
  }
  rpc GetAvatarThumbnail(GetAvatarBlobRequest) returns (GetAvatarBlobResponse) {
    option (google.api.http) = { get: "/v1/avatar-assets/{id}/thumbnail" };
  }
  rpc DeleteAvatarAsset(DeleteAvatarAssetRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = { delete: "/v1/avatar-assets/{id}" };
  }
}
```

### 2.2 Proto 扩展（引用检查）

```protobuf
message CheckAvatarReferencesRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
}

message CheckAvatarReferencesResponse {
  bool has_references = 1;
  repeated string agent_ids = 2;
  repeated string agent_names = 3;
}

service AvatarService {
  // ... 已有方法 ...

  rpc CheckAvatarReferences(CheckAvatarReferencesRequest) returns (CheckAvatarReferencesResponse) {
    option (google.api.http) = { get: "/v1/avatar-assets/{id}/references" };
  }
}
```

### 2.3 Proto 扩展（渠道平台图标刷新）

```protobuf
message RefreshChannelPlatformIconsRequest {}

message RefreshChannelPlatformIconsResponse {
  int32 updated = 1;
  int32 failed = 2;
}

service AvatarService {
  // ... 已有方法 ...

  rpc RefreshChannelPlatformIcons(RefreshChannelPlatformIconsRequest) returns (RefreshChannelPlatformIconsResponse) {
    option (google.api.http) = { post: "/v1/avatar-assets/channel-platform-icons:refresh" body: "*" };
  }
}
```

**说明**：从 Iconify API（`https://api.iconify.design/simple-icons/{slug}.svg`）下载 SVG 图标，渲染为 PNG 后 upsert 到 `avatar_assets` 表（`category=channel`）。下载/渲染失败时回退到嵌入 PNG 或文字标签渲染。

---

## 三、Biz 层

### 3.1 领域模型（已实现）

```go
type AvatarAsset struct {
    ID            string
    Key           string
    Name          string
    Description   string
    MimeType      string
    WorkspaceID   string
    OwnerUserID   string
    Source        string
    IsSystem      bool
    FileSizeBytes int
    WidthPx       int
    HeightPx      int
    SortOrder     int
    CreatedAt     string
}

type AvatarImage struct {
    ID       string
    MimeType string
    Data     []byte
}
```

### 3.2 Usecase（已实现 + 增强）

```go
type AvatarUsecase struct {
    repo    AvatarRepo
    agents  AgentCatalogRepository
}

func NewAvatarUsecase(repo AvatarRepo, agents AgentCatalogRepository) *AvatarUsecase

func (uc *AvatarUsecase) ListAvatarAssets(ctx context.Context, scope, workspaceID, ownerUserID string) ([]AvatarAsset, error)
func (uc *AvatarUsecase) UploadAvatar(ctx context.Context, data []byte, filename string, workspaceID, ownerUserID string) (AvatarAsset, error)
func (uc *AvatarUsecase) DeleteAvatarAsset(ctx context.Context, id string) error
func (uc *AvatarUsecase) GetAvatarImage(ctx context.Context, id string, thumbnail bool) (AvatarImage, error)

func (uc *AvatarUsecase) CheckReferences(ctx context.Context, id string) (hasRefs bool, agentIDs []string, agentNames []string, err error)
```

### 3.3 UploadAvatar 增强（服务端压缩 + 缩略图）

```go
func (uc *AvatarUsecase) UploadAvatar(ctx context.Context, data []byte, filename string, workspaceID, ownerUserID string) (AvatarAsset, error) {
    if len(data) == 0 {
        return AvatarAsset{}, errors.BadRequest("AVATAR", "avatar file is required")
    }
    const max = 2 * 1024 * 1024
    if len(data) > max {
        return AvatarAsset{}, errors.BadRequest("AVATAR", "avatar file must be <= 2MB")
    }
    mt := http.DetectContentType(data)
    if mt != "image/png" && mt != "image/jpeg" && mt != "image/webp" {
        return AvatarAsset{}, errors.BadRequest("AVATAR", "unsupported avatar type")
    }

    compressed, width, height, err := resizeAvatar(data, mt, 256)
    if err != nil {
        compressed = data
        width, height = 0, 0
    }

    thumbnail, _, _, err := resizeAvatar(data, mt, 64)
    if err != nil {
        thumbnail = compressed
    }

    id := newAvatarID()
    name := strings.TrimSpace(filename)
    if name == "" {
        name = "上传头像"
    }
    asset := AvatarAsset{
        ID:            id,
        Key:           "upload-" + id,
        Name:          name,
        Description:   "用户上传头像",
        MimeType:      mt,
        WorkspaceID:   workspaceID,
        OwnerUserID:   ownerUserID,
        Source:        "upload",
        IsSystem:      false,
        FileSizeBytes: len(compressed),
        WidthPx:       width,
        HeightPx:      height,
        SortOrder:     1000,
    }
    return uc.repo.CreateAvatarAsset(ctx, asset, compressed, thumbnail)
}

func resizeAvatar(data []byte, mimeType string, maxSize int) ([]byte, int, int, error) {
    img, _, err := image.Decode(bytes.NewReader(data))
    if err != nil {
        return nil, 0, 0, err
    }
    bounds := img.Bounds()
    w, h := bounds.Dx(), bounds.Dy()
    if w > maxSize || h > maxSize {
        if w > h {
            h = h * maxSize / w
            w = maxSize
        } else {
            w = w * maxSize / h
            h = maxSize
        }
        img = resize.Resize(uint(w), uint(h), img, resize.Lanczos3)
    }
    var buf bytes.Buffer
    switch mimeType {
    case "image/png":
        png.Encode(&buf, img)
    case "image/jpeg":
        jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85})
    case "image/webp":
        webp.Encode(&buf, img, &webp.Options{Quality: 85})
    default:
        png.Encode(&buf, img)
    }
    return buf.Bytes(), w, h, nil
}
```

### 3.4 DeleteAvatarAsset 增强（引用检查）

```go
func (uc *AvatarUsecase) DeleteAvatarAsset(ctx context.Context, id string) error {
    if strings.TrimSpace(id) == "" {
        return errors.BadRequest("AVATAR", "avatar id is required")
    }
    hasRefs, agentIDs, _, err := uc.CheckReferences(ctx, id)
    if err != nil {
        return err
    }
    if hasRefs {
        return errors.BadRequest("AVATAR", fmt.Sprintf("avatar is in use by agents: %s", strings.Join(agentIDs, ", ")))
    }
    return uc.repo.SoftDeleteAvatarAsset(ctx, id)
}

func (uc *AvatarUsecase) CheckReferences(ctx context.Context, id string) (bool, []string, []string, error) {
    agents, err := uc.agents.FindByIcon(ctx, id)
    if err != nil {
        return false, nil, nil, err
    }
    if len(agents) == 0 {
        return false, nil, nil, nil
    }
    ids := make([]string, len(agents))
    names := make([]string, len(agents))
    for i, a := range agents {
        ids[i] = a.ID
        names[i] = a.DisplayName
    }
    return true, ids, names, nil
}
```

---

## 四、Data 层

### 4.1 Ent Schema（已实现）

文件：`internal/data/ent/schema/avatar_asset.go`

```go
func (AvatarAsset) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").Immutable().Unique().MaxLen(256),
        field.String("asset_key").Unique().MaxLen(512),
        field.String("name").MaxLen(1024),
        field.Bytes("image_data"),
        field.Bytes("thumbnail_data").Optional(),
        field.String("mime_type").Default("image/png"),
        field.String("workspace_id").Default(""),
        field.String("owner_user_id").Default(""),
        field.String("source").Default("system"),
        field.Bool("is_system").Default(false),
        field.Int("file_size_bytes").Default(0),
        field.Int("width_px").Default(0),
        field.Int("height_px").Default(0),
        field.Text("description").Default(""),
        field.String("status").Default("active"),
        field.Bool("enabled").Default(true),
        field.Int("sort_order").Default(0),
        field.Text("config_json").Default(""),
        field.Text("metadata_json").Default(""),
        field.String("created_at").Default(""),
        field.String("updated_at").Default(""),
        field.String("deleted_at").Default(""),
    }
}
```

### 4.2 AvatarRepo（已实现）

```go
type AvatarRepo interface {
    ListAvatarAssets(ctx context.Context, scope, workspaceID, ownerUserID string) ([]AvatarAsset, error)
    GetAvatarImage(ctx context.Context, id string, thumbnail bool) (AvatarImage, error)
    CreateAvatarAsset(ctx context.Context, asset AvatarAsset, imageData, thumbnailData []byte) (AvatarAsset, error)
    SoftDeleteAvatarAsset(ctx context.Context, id string) error
}
```

### 4.3 AgentCatalogRepository 扩展

```go
type AgentCatalogRepository interface {
    // ... 已有方法 ...

    FindByIcon(ctx context.Context, icon string) ([]Agent, error)
}
```

FindByIcon Ent 查询：

```go
func (r *agentRepo) FindByIcon(ctx context.Context, icon string) ([]biz.Agent, error) {
    rows, err := r.data.Ent().Agent.Query().
        Where(
            agent.IconEQ(icon),
            agent.DeletedAtEQ(""),
        ).
        All(ctx)
    if err != nil {
        return nil, err
    }
    out := make([]biz.Agent, 0, len(rows))
    for _, po := range rows {
        out = append(out, entToBizAgent(po))
    }
    return out, nil
}
```

---

## 五、Service 层

### 5.1 已有实现

```go
type AvatarService struct {
    v1.UnimplementedAvatarServiceServer
    uc *biz.AvatarUsecase
}

func (s *AvatarService) ListAvatarAssets(ctx, req) (*ListAvatarAssetsResponse, error)
func (s *AvatarService) CreateAvatarAsset(ctx, req) (*AvatarAsset, error)
func (s *AvatarService) GetAvatarFile(ctx, req) (*GetAvatarBlobResponse, error)
func (s *AvatarService) GetAvatarThumbnail(ctx, req) (*GetAvatarBlobResponse, error)
func (s *AvatarService) DeleteAvatarAsset(ctx, req) (*emptypb.Empty, error)
```

### 5.2 新增引用检查

```go
func (s *AvatarService) CheckAvatarReferences(ctx context.Context, req *v1.CheckAvatarReferencesRequest) (*v1.CheckAvatarReferencesResponse, error) {
    hasRefs, agentIDs, agentNames, err := s.uc.CheckReferences(ctx, req.GetId())
    if err != nil {
        return nil, err
    }
    return &v1.CheckAvatarReferencesResponse{
        HasReferences: hasRefs,
        AgentIds:      agentIDs,
        AgentNames:    agentNames,
    }, nil
}
```

---

## 六、Wire 注入

已有注册，无需修改。增强 `AvatarUsecase` 时需添加 `AgentCatalogRepository` 依赖：

```go
func NewAvatarUsecase(repo AvatarRepo, agents AgentCatalogRepository) *AvatarUsecase {
    return &AvatarUsecase{repo: repo, agents: agents}
}
```

---

## 七、Web 前端设计

### 7.1 文件结构

```
web/src/
├── features/avatar/
│   ├── api.ts                       ← 已有
│   ├── iconModel.ts                 ← 已有
│   ├── useAgentAvatarPreview.ts     ← 已有
│   ├── useAvatarThumbnailSrc.ts     ← 已有
│   └── useAvatarPickerDialog.ts     ← 已有
├── components/avatar/
│   ├── AgentAvatarPicker.vue        ← 已有，增强裁剪
│   ├── AgentAvatarQ.vue             ← 已有
│   └── ResolvedAvatarImg.vue        ← 已有
├── stores/avatar/
│   └── index.ts                     ← 已有
└── components/agents/
    ├── AgentCreateDialog.vue        ← 已有
    └── AgentSettingsHeader.vue      ← 已有
```

### 7.2 AgentAvatarPicker.vue 增强

在现有 Picker 中增加裁剪子步骤：

```
┌─────────────────────────────────────────────────────────┐
│  选择头像                                          [×]   │
├─────────────────────────────────────────────────────────┤
│  [ 内置 ]  [ 我的上传 ]     （QTabs）                    │
├─────────────────────────────────────────────────────────┤
│  ┌────┐ ┌────┐ ┌────┐   网格 QAvatar + 选中描边         │
│  └────┘ └────┘ └────┘                                    │
├─────────────────────────────────────────────────────────┤
│  [ 从本地上传 ]                                           │
├─────────────────────────────────────────────────────────┤
│  裁剪区域（上传后显示）：                                   │
│  ┌──────────────────────────────────────────────────┐   │
│  │  vue-advanced-cropper (1:1 固定比例)              │   │
│  │  ┌────────────────────────────────────────────┐  │   │
│  │  │                                            │  │   │
│  │  │           图片裁剪区域                       │  │   │
│  │  │                                            │  │   │
│  │  └────────────────────────────────────────────┘  │   │
│  └──────────────────────────────────────────────────┘   │
├─────────────────────────────────────────────────────────┤
│  [ 取消 ]  [ 跳过裁剪 ]  [ 裁剪并上传 ]                    │
└─────────────────────────────────────────────────────────┘
```

裁剪流程实现：

```typescript
import { Cropper } from 'vue-advanced-cropper'
import 'vue-advanced-cropper/dist/style.css'

interface CropperState {
  showCropper: boolean
  cropperImage: string | null
  cropperFile: File | null
}

async function onFileChange(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ""
  if (!file) return

  const validationError = validateAvatarFileForUpload(file)
  if (validationError) {
    $q.notify({ type: 'negative', message: validationError })
    return
  }

  cropperImage.value = URL.createObjectURL(file)
  cropperFile.value = file
  showCropper.value = true
}

async function cropAndUpload() {
  if (!cropperRef.value) return
  const { canvas } = cropperRef.value.getResult()
  const blob = await new Promise<Blob>((resolve) => canvas.toBlob(resolve, 'image/png'))
  const croppedFile = new File([blob], cropperFile.value?.name ?? 'avatar.png', { type: 'image/png' })
  await uploadFromFile(croppedFile)
  showCropper.value = false
  URL.revokeObjectURL(cropperImage.value!)
}
```

### 7.3 删除确认增强

删除前检查引用：

```typescript
export async function checkAvatarReferences(id: string): Promise<{ has_references: boolean; agent_ids: string[]; agent_names: string[] }> {
  const svc = createAvatarService()
  const res = await svc.CheckAvatarReferences({ id })
  return {
    has_references: res.hasReferences,
    agent_ids: res.agentIds ?? [],
    agent_names: res.agentNames ?? [],
  }
}
```

删除流程：

```typescript
async function handleDelete(id: string) {
  const refs = await checkAvatarReferences(id)
  if (refs.has_references) {
    $q.notify({
      type: 'warning',
      message: `该头像正被 ${refs.agent_names.length} 个 Agent 使用中，无法删除`,
    })
    return
  }
  await deleteAvatarAsset(id)
  store.invalidateAll()
}
```

### 7.4 ResolvedAvatarImg.vue

已有实现，展示头像图片：

```typescript
interface Props {
  icon: string
  alt?: string
}
```

- 当 `icon` 为 `avatar_assets.id` 格式时，使用 `/v1/avatar-assets/{id}/thumbnail` 获取缩略图
- 当 `icon` 为 URL 格式时，直接使用 URL
- 当 `icon` 为空时，显示默认占位头像

---

## 八、实现计划

| 阶段 | 内容 | 涉及文件 |
|------|------|---------|
| P1 | 前端裁剪组件集成 | `web/src/components/avatar/AgentAvatarPicker.vue`, 新增 `AvatarCropperStep.vue` |
| P2 | 服务端图片压缩 + 缩略图生成 | `internal/biz/avatar.go`（resizeAvatar 函数） |
| P3 | Agent 引用检查 + 删除保护 | `internal/biz/avatar.go`, `internal/data/agent_repo.go`, `api/kratos/avatar/v1/avatar.proto` |
| P4 | 前端删除确认交互 | `web/src/features/avatar/api.ts`, `web/src/components/avatar/AgentAvatarPicker.vue` |

---

## 九、验收标准

1. 上传头像后可裁剪为 1:1 正方形（vue-advanced-cropper）
2. 服务端自动压缩上传图片至 256px 正方形
3. 服务端自动生成 64×64 缩略图存入 `thumbnail_data`
4. 删除头像前检查 Agent 引用，有引用时禁止删除
5. 前端删除操作有引用提示
6. 已有功能（列表/上传/缩略图/文件获取）不受影响
7. 内置头像不可被普通用户删除（`is_system=true` 保护）
