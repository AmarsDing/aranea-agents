# Avatar 头像模块 — 实现设计文档

> 对应需求：`50-avatar.md`
> 开发计划：`50-avatar.development.md`

---

## 一、模块概述

头像资源库：上传、裁剪、选择、存储。不设独立路由页，仅在 Agent 创建/编辑流程中弹出。图片像素数据直接落在数据库（BLOB），不依赖对象存储或本地文件目录。

### 当前实现状态

| 层 | 状态 | 文件 |
|----|------|------|
| Proto | ✅ 已有 | `api/kratos/avatar/v1/avatar.proto` |
| Biz | ✅ 已有 | `internal/biz/avatar/` 子包（`avatar.go`, `image.go`, `avatar_channel_refresh.go`） |
| Data | ✅ 已有 | `internal/data/avatar.go`, `internal/data/ent/schema/avatar_asset.go` |
| Service | ✅ 已有 | `internal/service/avatar.go` |
| Wire | ✅ 已有 | 已注册 |
| Web API | ✅ 已有 | `web/src/features/avatar/api.ts` |
| Web Store | ✅ 已有 | `web/src/stores/avatar/index.ts` |
| Web 组件 | ✅ 已有 | `web/src/components/avatar/AgentAvatarPicker.vue`, `ResolvedAvatarImg.vue`, `AgentAvatarQ.vue` |

### 已实现 vs 待实现

| 功能 | 状态 | 说明 |
|------|------|------|
| 头像 CRUD 全链路 | ✅ | Proto → Service → Usecase → Repo → 前端 |
| 自动中心裁剪 + 压缩 | ✅ | 前端 Canvas 居中裁剪 + 后端 `ProcessAvatarUpload`（512px 主图 + 128px 缩略图） |
| 渠道平台图标刷新 | ✅ | 从 Iconify API 下载 SVG → 渲染品牌色 PNG → upsert |
| 头像选择器 | ✅ | `AgentAvatarPicker`（双 Tab、分组、上传、选择确认） |
| 头像展示 | ✅ | `ResolvedAvatarImg`（纯 img）、`AgentAvatarQ`（QAvatar 封装 + 占位图标） |
| 交互式裁剪 | ❌ | `vue-advanced-cropper` 未安装，当前为自动中心裁剪 |
| Agent 引用检查 | ❌ | `CheckReferences` / `FindByIcon` 不存在，删除无引用保护 |
| 内置头像删除保护 | ❌ | 服务端未校验 `is_system` 禁止删除 |

---

## 二、Proto 层

### 2.1 现有 Proto（已实现）

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
  rpc RefreshChannelPlatformIcons(RefreshChannelPlatformIconsRequest) returns (RefreshChannelPlatformIconsResponse) {
    option (google.api.http) = { post: "/v1/avatar-assets/channel-platform-icons:refresh" body: "*" };
  }
}
```

`AvatarAsset` message 包含字段：id, key, name, description, mime_type, workspace_id, owner_user_id, source, is_system, file_size_bytes, width_px, height_px, sort_order, created_at, **category**。

### 2.2 Proto 扩展（引用检查 — 待实现）

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

---

## 三、Biz 层

### 3.1 代码结构

Biz 层头像代码位于 `internal/biz/avatar/` 子包：

| 文件 | 职责 |
|------|------|
| `avatar.go` | 核心类型定义（`Asset`, `Image`, `Repo`, `Usecase`, `ChannelIconRefresher` 接口） |
| `image.go` | 图片处理：`ProcessAvatarUpload`（中心裁剪 + 主图 512px + 缩略图 128px + JPEG 编码） |
| `avatar_channel_refresh.go` | 渠道图标刷新：从 Iconify API 下载 SVG → 渲染品牌色 PNG → upsert |
| `avatar_usecase_test.go` | Usecase 测试 |
| `image_test.go` / `image_more_test.go` | 图片处理测试 |

`internal/biz/avatar.go` 仅做 re-export（类型别名 + `NewAvatarUsecase` 变量导出），保持向后兼容。

### 3.2 领域模型（已实现）

```go
// internal/biz/avatar/avatar.go

type Asset struct {
    ID            string
    Key           string
    Name          string
    Description   string
    MimeType      string
    WorkspaceID   string
    OwnerUserID   string
    Source        string
    Category      string  // agent | channel
    IsSystem      bool
    FileSizeBytes int
    WidthPx       int
    HeightPx      int
    SortOrder     int
    CreatedAt     string
}

type Image struct {
    ID       string
    MimeType string
    Data     []byte
}
```

### 3.3 Repo 接口（已实现）

```go
type Repo interface {
    ListAvatarAssets(ctx context.Context, scope, workspaceID, ownerUserID string) ([]Asset, error)
    GetAvatarAssetByKey(ctx context.Context, assetKey string) (Asset, error)
    GetAvatarImage(ctx context.Context, id string, thumbnail bool) (Image, error)
    CreateAvatarAsset(ctx context.Context, asset Asset, imageData, thumbnailData []byte) (Asset, error)
    UpdateAvatarAssetImages(ctx context.Context, id string, imageData, thumbnailData []byte, mime string, width, height, fileSize int) error
    SoftDeleteAvatarAsset(ctx context.Context, id string) error
}
```

### 3.4 Usecase（已实现）

```go
type Usecase struct {
    repo     Repo
    refresher ChannelIconRefresher
}

func NewUsecase(repo Repo, refresher ChannelIconRefresher) *Usecase

// 已实现方法
func (uc *Usecase) ListAvatarAssets(ctx, scope, workspaceID, ownerUserID) ([]Asset, error)
func (uc *Usecase) GetAvatarImage(ctx, id, thumbnail) (Image, error)
func (uc *Usecase) UploadAvatar(ctx, data, filename, workspaceID, ownerUserID) (Asset, error)
func (uc *Usecase) DeleteAvatarAsset(ctx, id) error
func (uc *Usecase) RefreshChannelPlatformIcons(ctx) (*RefreshChannelPlatformIconsResult, error)
```

**注意**：`Usecase` 依赖 `Repo` + `ChannelIconRefresher`，**不依赖** `AgentCatalogRepository`。引用检查功能需要新增此依赖。

### 3.5 UploadAvatar 实现（已实现）

```go
func (uc *Usecase) UploadAvatar(ctx context.Context, data []byte, filename string, workspaceID, ownerUserID string) (Asset, error) {
    // 1. 校验：非空、≤2MB、MIME 类型检测
    // 2. 调用 ProcessAvatarUpload(data, mime) → 中心裁剪 + 主图 512px + 缩略图 128px
    // 3. 生成 ID + asset，调用 repo.CreateAvatarAsset
}
```

### 3.6 图片处理（已实现）

```go
// internal/biz/avatar/image.go

func ProcessAvatarUpload(data []byte, mime string) (imageData, thumbData []byte, width, height int, err error)
// 流程：解码 → 中心裁剪为正方形 → 主图 resizeSquare(512) → 缩略图 resizeSquare(128) → JPEG 编码

func resizeSquare(src image.Image, maxEdge int) image.Image
// 使用 golang.org/x/image/draw 的 CatmullRom 算法

func encodeJPEG(img image.Image, quality int) ([]byte, error)
```

### 3.7 渠道图标刷新（已实现）

```go
// internal/biz/avatar/avatar_channel_refresh.go

type ChannelIconRefresher interface {
    RefreshChannelPlatformIcons(ctx context.Context, repo Repo) (*RefreshChannelPlatformIconsResult, error)
}

// channelIconRefresher 实现：
// 1. 遍历 ChannelPlatformAvatarSpecs()（20+ 渠道类型 → simple-icons slug 映射）
// 2. 从 Iconify API 下载 SVG
// 3. renderBrandIcon → 带品牌色背景的 PNG
// 4. 失败时回退到嵌入式 PNG 或文字标签渲染
// 5. 调用 ProcessAvatarUpload 处理
// 6. 按 asset_key 查询已有记录，存在则 UpdateAvatarAssetImages，不存在则 CreateAvatarAsset
```

### 3.8 DeleteAvatarAsset（已实现 — 无引用检查）

```go
func (uc *Usecase) DeleteAvatarAsset(ctx context.Context, id string) error {
    if strings.TrimSpace(id) == "" {
        return avatarError("avatar id is required")
    }
    return uc.repo.SoftDeleteAvatarAsset(ctx, id)
}
```

当前删除仅做软删，**不检查 Agent 引用**，也不校验 `is_system` 保护。

### 3.9 待实现：引用检查

```go
// 需要在 Usecase 中新增 AgentCatalogRepository 依赖
type Usecase struct {
    repo     Repo
    refresher ChannelIconRefresher
    agents   AgentCatalogRepository  // 新增
}

func (uc *Usecase) CheckReferences(ctx context.Context, id string) (hasRefs bool, agentIDs []string, agentNames []string, err error) {
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
        field.String("category").Default("agent"),
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

索引：`(is_system, sort_order)` 和 `(workspace_id, owner_user_id)`。

### 4.2 AvatarRepo（已实现）

```go
type avatarRepo struct {
    data *Data
}

// 6 个方法全部实现：
// ListAvatarAssets — 支持 scope 过滤（system/mine/default），含软删除和 enabled 过滤
// GetAvatarAssetByKey — 按 asset_key 查询
// GetAvatarImage — 读取 image_data 或 thumbnail_data
// CreateAvatarAsset — 写入数据库，默认 avatarPersistSize=256
// UpdateAvatarAssetImages — 更新图片数据
// SoftDeleteAvatarAsset — 设置 deleted_at + status=deleted
```

### 4.3 待实现：AgentCatalogRepository.FindByIcon

```go
// 需要在 Agent repo 中新增方法
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
func (s *AvatarService) RefreshChannelPlatformIcons(ctx, req) (*RefreshChannelPlatformIconsResponse, error)
```

### 5.2 待实现：引用检查

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

当前注入（已实现）：

```go
// cmd/admin/wire_gen.go
repo := data.NewAvatarRepo(dataData)
channelIconRefresher := biz.NewChannelIconRefresher()
usecase := avatar.NewUsecase(repo, channelIconRefresher)
avatarService := service.NewAvatarService(usecase)
```

引用检查功能需要修改 `Usecase` 构造，新增 `AgentCatalogRepository` 依赖：

```go
usecase := avatar.NewUsecase(repo, channelIconRefresher, agentRepo)
```

---

## 七、Web 前端设计

### 7.1 文件结构

```
web/src/
├── features/avatar/
│   ├── api.ts                       ← 已有：上传、列表、缩略图、渠道图标刷新
│   ├── iconModel.ts                 ← 已有：icon 类型判断工具函数
│   ├── types.ts                     ← 已有：AvatarAsset 类型定义
│   ├── index.ts                     ← 已有：统一导出
│   ├── useAgentAvatarPreview.ts     ← 已有：icon → QAvatar 的 icon + img src 映射
│   ├── useAvatarThumbnailSrc.ts     ← 已有：icon → 缩略图 data URL 响应式解析
│   ├── useAvatarPickerDialog.ts     ← 已有：Picker 弹层状态管理
│   ├── prepareAvatarUpload.ts       ← 已有：上传前 Canvas 居中裁剪 + 缩放（512px）
│   └── resolveAvatarAssetFetchId.ts ← 已有：agent.icon → catalog asset id 解析
├── components/avatar/
│   ├── AgentAvatarPicker.vue        ← 已有：双 Tab 选择器 + 上传（无交互式裁剪）
│   ├── AgentAvatarQ.vue             ← 已有：QAvatar 封装 + 占位图标
│   └── ResolvedAvatarImg.vue        ← 已有：纯 img 展示
├── stores/avatar/
│   └── index.ts                     ← 已有：useAvatarCatalogStore
└── components/agents/
    ├── AgentCreateDialog.vue        ← 已有
    └── AgentSettingsHeader.vue      ← 已有
```

### 7.2 AgentAvatarPicker.vue（当前实现）

当前 Picker 功能：
- `QDialog` 弹层，含 system/mine 两个 Tab
- 支持 agent/channel 两种 scope
- 文件上传（`<input type="file">`）→ `prepareAvatarUploadFile` 自动中心裁剪 → 直接上传
- **无交互式裁剪器**（`vue-advanced-cropper` 未安装）

### 7.3 待实现：交互式裁剪

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

实现要点：

```typescript
import { Cropper } from 'vue-advanced-cropper'
import 'vue-advanced-cropper/dist/style.css'

// 安装：pnpm add vue-advanced-cropper

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

### 7.4 待实现：删除确认增强

删除前检查引用：

```typescript
// web/src/features/avatar/api.ts 新增
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

### 7.5 ResolvedAvatarImg.vue（已实现）

展示头像图片：

```typescript
interface Props {
  icon: string
  alt?: string
}
```

- 当 `icon` 为 `avatar_assets.id` 格式时，使用 `useAvatarThumbnailSrc` 获取缩略图 data URL
- 当 `icon` 为 http/data/blob URL 格式时，直接使用 URL
- 当 `icon` 为空时，显示默认占位头像

### 7.6 AgentAvatarQ.vue（已实现）

封装 `<q-avatar>` 的展示组件：

- 通过 `useAgentAvatarPreview` 解析 icon → 缩略图 src + Quasar icon name
- 有缩略图时渲染 `<img>`，否则用 Material icon 占位（默认 `smart_toy`）

---

## 八、实现计划

| 阶段 | 内容 | 状态 | 涉及文件 |
|------|------|------|---------|
| P1 | 前端交互式裁剪组件集成 | ❌ 待实现 | `web/src/components/avatar/AgentAvatarPicker.vue`, 新增 `AvatarCropperStep.vue`, 安装 `vue-advanced-cropper` |
| P2 | Agent 引用检查 + 删除保护 | ❌ 待实现 | `api/kratos/avatar/v1/avatar.proto`, `internal/biz/avatar/avatar.go`, `internal/data/agent_repo.go`, `internal/service/avatar.go` |
| P3 | 前端删除确认交互 | ❌ 待实现 | `web/src/features/avatar/api.ts`, `web/src/components/avatar/AgentAvatarPicker.vue` |
| P4 | 内置头像删除保护 | ❌ 待实现 | `internal/biz/avatar/avatar.go`（DeleteAvatarAsset 增加 is_system 校验） |

---

## 九、验收标准

1. ~~服务端自动压缩上传图片~~ ✅ 已实现（512px 主图 + 128px 缩略图，JPEG 编码）
2. ~~服务端自动生成缩略图~~ ✅ 已实现（128×128）
3. ~~渠道平台图标刷新~~ ✅ 已实现
4. 上传头像后可交互式裁剪为 1:1 正方形（`vue-advanced-cropper`）— 待实现
5. 删除头像前检查 Agent 引用，有引用时禁止删除 — 待实现
6. 前端删除操作有引用提示 — 待实现
7. 内置头像不可被普通用户删除（`is_system=true` 保护）— 待实现
8. 已有功能（列表/上传/缩略图/文件获取/渠道图标刷新）不受影响
