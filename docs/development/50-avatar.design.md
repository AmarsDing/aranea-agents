# Avatar 头像模块 — 实现设计文档

> 对应需求：`50-avatar.md`
> 开发计划：`50-avatar.development.md`

---

## 一、模块概述

头像资源库：上传、裁剪、选择、存储。不设独立路由页，仅在 Agent 创建/编辑流程中弹出。图片像素数据直接落在数据库（BLOB），不依赖对象存储或本地文件目录。

### 代码锚点

| 层 | 文件 |
|----|------|
| Proto | `api/kratos/avatar/v1/avatar.proto` |
| Biz | `internal/biz/avatar/` 子包（`avatar.go`, `image.go`） |
| Biz（渠道刷新） | `internal/biz/avatar_channel_refresh.go` |
| Biz（种子数据） | `internal/biz/avatar_channel_seed.go`, `internal/biz/avatar_agent_seed.go` |
| Biz（re-export） | `internal/biz/avatar.go` |
| Data | `internal/data/avatar.go`, `internal/data/ent/schema/avatar_asset.go` |
| Service | `internal/service/avatar.go` |
| Wire | `cmd/admin/wire_gen.go` |
| Web API | `web/src/features/avatar/api.ts` |
| Web Store | `web/src/stores/avatar/index.ts` |
| Web 组件 | `web/src/components/avatar/AgentAvatarPicker.vue`, `ResolvedAvatarImg.vue`, `AgentAvatarQ.vue` |

> 各层当前实现状态详见 [50-avatar.development.md §2 现状评估](./50-avatar.development.md#2-现状评估)。

---

## 二、Proto 层

### 2.1 现有 Proto

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

### 2.2 Proto 扩展：引用检查

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

Biz 层头像代码分布在两个位置：

**`internal/biz/avatar/` 子包**：

| 文件 | 职责 |
|------|------|
| `avatar.go` | 核心类型定义（`Asset`, `Image`, `Repo`, `Usecase`, `ChannelIconRefresher` 接口） |
| `image.go` | 图片处理：`ProcessAvatarUpload`（中心裁剪 + 主图 512px + 缩略图 128px + JPEG 编码） |
| `avatar_usecase_test.go` | Usecase 测试 |
| `image_test.go` / `image_more_test.go` | 图片处理测试 |

**`internal/biz/` 父包**（依赖 `avatar` 子包）：

| 文件 | 职责 |
|------|------|
| `avatar.go` | re-export（类型别名 + `NewAvatarUsecase` 变量导出），保持向后兼容 |
| `avatar_channel_refresh.go` | `channelIconRefresher` 实现：从 Iconify API 下载 SVG → 渲染品牌色 PNG → upsert |
| `avatar_channel_seed.go` | `EnsureChannelPlatformAvatars`：内置渠道平台图标 seed（嵌入式 PNG） |
| `avatar_agent_seed.go` | `EnsureAgentAvatars`：内置 Agent 头像 seed |

### 3.2 领域模型

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

### 3.3 Repo 接口

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

### 3.4 Usecase

```go
type Usecase struct {
    repo      Repo
    refresher ChannelIconRefresher
}

func NewUsecase(repo Repo, refresher ChannelIconRefresher) *Usecase

// 方法
func (uc *Usecase) ListAvatarAssets(ctx, scope, workspaceID, ownerUserID) ([]Asset, error)
func (uc *Usecase) GetAvatarImage(ctx, id, thumbnail) (Image, error)
func (uc *Usecase) UploadAvatar(ctx, data, filename, workspaceID, ownerUserID) (Asset, error)
func (uc *Usecase) DeleteAvatarAsset(ctx, id) error
func (uc *Usecase) RefreshChannelPlatformIcons(ctx) (*RefreshChannelPlatformIconsResult, error)
```

**注意**：`Usecase` 依赖 `Repo` + `ChannelIconRefresher`，**不依赖** `AgentCatalogRepository`。引用检查功能需要新增此依赖。

### 3.5 UploadAvatar 实现

```go
func (uc *Usecase) UploadAvatar(ctx context.Context, data []byte, filename string, workspaceID, ownerUserID string) (Asset, error) {
    // 1. 校验：非空、≤2MB、MIME 类型检测（image/png、image/jpeg、image/webp、image/gif）
    // 2. 调用 ProcessAvatarUpload(data, mime) → 中心裁剪 + 主图 512px + 缩略图 128px
    // 3. 生成 ID + asset，调用 repo.CreateAvatarAsset
}
```

### 3.6 图片处理

```go
// internal/biz/avatar/image.go

const (
    AvatarMainMaxPx  = 512
    avatarThumbMaxPx = 128
)

// ProcessAvatarUpload 解码、中心裁剪正方形、输出主图 + 缩略图。
// 返回值：main, thumb, width, height, outMime, err
func ProcessAvatarUpload(data []byte, mime string) (main []byte, thumb []byte, width, height int, outMime string, err error)
// 流程：解码 → 中心裁剪为正方形 → 主图 resizeSquare(512) → 缩略图 resizeSquare(128) → JPEG 编码（主图 quality=92，缩略图 quality=88）
// outMime 固定为 "image/jpeg"

func resizeSquare(src image.Image, maxEdge int) *image.RGBA
// 使用 golang.org/x/image/draw 的 CatmullRom 算法

func encodeJPEG(img image.Image, quality int) ([]byte, error)
```

### 3.7 渠道图标刷新

```go
// internal/biz/avatar_channel_refresh.go

type channelIconRefresher struct{}

func NewChannelIconRefresher() avatar.ChannelIconRefresher

// RefreshChannelPlatformIcons 实现：
// 1. 遍历 simpleIconsSlug（30+ 渠道类型 → simple-icons slug 映射）
// 2. 从 Iconify API (https://api.iconify.design/simple-icons) 下载 SVG
// 3. renderBrandIcon → 带品牌色背景的 PNG（iconCanvasSize=256, iconPadding=48）
// 4. 失败时回退到嵌入式 PNG 或文字标签渲染
// 5. 调用 ProcessAvatarUpload 处理
// 6. 按 asset_key 查询已有记录，存在则 UpdateAvatarAssetImages，不存在则 CreateAvatarAsset
```

### 3.8 DeleteAvatarAsset

```go
func (uc *Usecase) DeleteAvatarAsset(ctx context.Context, id string) error {
    if strings.TrimSpace(id) == "" {
        return apierror.BadRequest("AVATAR", "avatar id is required")
    }
    return uc.repo.SoftDeleteAvatarAsset(ctx, id)
}
```

当前删除仅做软删，**不检查 Agent 引用**，也不校验 `is_system` 保护。

### 3.9 引用检查扩展

引用检查功能需要在 `Usecase` 中新增 `AgentCatalogRepository` 依赖：

```go
type Usecase struct {
    repo      Repo
    refresher ChannelIconRefresher
    agents    AgentCatalogRepository  // 新增
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

## 四、数据模型

### 4.1 Ent Schema

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

索引（`internal/data/ent/schema/avatar_asset.go` `Indexes()`）：

| 索引名 | 字段 |
|--------|------|
| `idx_avatar_assets_system` | `(is_system, sort_order)` |
| `idx_avatar_assets_workspace_owner` | `(workspace_id, owner_user_id)` |

### 4.2 列说明

| 列 | 说明 |
|----|------|
| `image_data` | **主头像二进制**；列表详情展示默认从此读流（或通过缩略图列）。 |
| `thumbnail_data` | 可选；当前实现为 128×128，**网格选图**与 **Agent 列表卡片** 优先用缩略图接口以降低带宽。 |
| `mime_type` | 与 `image_data` 一致，供 HTTP `Content-Type` 使用。 |
| `category` | 资源分类：`agent`（Agent 头像）/ `channel`（渠道平台图标）。 |
| `source` / `is_system` | 预置与上传区分、保护预置不可被普通用户删。 |
| `asset_key` | 唯一业务键，用于 upsert（如渠道图标的 `channel-{slug}` 格式）。 |

### 4.3 引用与删除规则

| 规则 | 说明 |
|------|------|
| **Agent 引用** | `agents.icon` **存 `avatar_assets.id`（TEXT/UUID）**。 |
| **删除上传** | 软删 `avatar_assets` 前检查是否有 `agents.icon` 指向该 id；**有引用则禁止删除**或先让用户改头像（推荐）。 |
| **硬删** | 清理软删记录时一并丢弃 BLOB，缩小库文件（VACUUM 策略由运维定）。 |

### 4.4 BLOB 与库体积

| 项 | 说明 |
|----|------|
| **限额** | 单张原图压至 ≤512px、≤200KB 级再入库，避免单行过大拖慢备份。 |
| **缩略图** | 当前实现为 128×128 JPEG，列表只读小图。 |
| **PostgreSQL** | 对应类型为 `BYTEA`；其余语义不变。 |

### 4.5 AvatarRepo 实现

```go
// internal/data/avatar.go

type avatarRepo struct {
    data *Data
}

func NewAvatarRepo(d *Data) biz.AvatarRepo
```

6 个方法实现说明：

| 方法 | 说明 |
|------|------|
| `ListAvatarAssets` | 支持 scope 过滤（system/mine/default），含软删除和 enabled 过滤，额外过滤 `length(image_data) > 0` |
| `GetAvatarAssetByKey` | 按 asset_key 查询 |
| `GetAvatarImage` | 读取 image_data 或 thumbnail_data（thumbnail 为空时回退到 image_data） |
| `CreateAvatarAsset` | 写入数据库，默认 `avatarPersistSize=256`（仅当 WidthPx/HeightPx 为 0 时填充） |
| `UpdateAvatarAssetImages` | 更新图片数据 |
| `SoftDeleteAvatarAsset` | 设置 `deleted_at` + `status=deleted` |

### 4.6 AgentCatalogRepository.FindByIcon 扩展

引用检查功能需要在 Agent repo 中新增方法：

```go
// internal/data/agent_repo.go

func (r *agentRepo) FindByIcon(ctx context.Context, icon string) ([]biz.Agent, error) {
    rows, err := r.data.RW().Read(ctx).Agent.Query().
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

## 五、存储策略

### 5.1 `agents.icon` 约定

| 策略 | 说明 |
|------|------|
| **推荐** | 存 **`avatar_assets.id`**（与 `2-agents-create.md` 中 `agents.icon` 字段兼容：类型仍为字符串，**语义为资源主键而非 URL**）。 |
| **禁止** | 不再把外链 URL 写入 `icon`（历史数据可一次性迁移：下载转 BLOB 入库或清空让用户重选）。 |
| **展示** | 所有 `QAvatar`/`QImg` 使用 **§六 API 契约** 中的只读接口 URL，不要拼静态 CDN。 |

内置预置数据：迁移脚本向 `avatar_assets` 插入 **`image_data`/`thumbnail_data` BLOB**（`is_system=1`，`owner_user_id` 为空）。

### 5.2 内置头像 Seed

| 文件 | 职责 |
|------|------|
| `internal/biz/avatar_agent_seed.go` | `EnsureAgentAvatars`：内置 Agent 头像 seed（从 `agenticons` 嵌入式资源加载） |
| `internal/biz/avatar_channel_seed.go` | `EnsureChannelPlatformAvatars`：内置渠道平台图标 seed（从 `channelicons` 嵌入式资源加载） |

---

## 六、API 契约

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/v1/avatar-assets` | Query：`scope=system|mine`，`workspace_id`；**仅返回元数据**：`id`、`mime_type`、`width_px` 等；**不返回 base64**，避免列表爆体。 |
| GET | `/v1/avatar-assets/:id/file` | **读主图**：响应体为二进制，`Content-Type` = `mime_type`；供 `<img :src="...">` 或 `QImg`。需鉴权或与会话 Cookie 同源。 |
| GET | `/v1/avatar-assets/:id/thumbnail` | **读缩略图**（若无缩略图可 302 或回退为 `/file`）。 |
| POST | `/v1/avatar-assets` | 上传；校验类型/大小；**服务端自动中心裁剪 + 压缩**；写入 `image_data`（及 `thumbnail_data`）；返回 **`{ id }`**。 |
| DELETE | `/v1/avatar-assets/:id` | 软删；校验权限。 |
| POST | `/v1/avatar-assets/channel-platform-icons:refresh` | 从 Iconify API 重新获取渠道平台图标并更新 DB；返回 `{ updated, failed }`。 |
| GET | `/v1/avatar-assets/:id/references` | 检查头像是否被 Agent 引用；返回 `{ has_references, agent_ids, agent_names }`。 |

**前端 `src` 写法示例**：`const avatarSrc = (id) => \`/api/v1/avatar-assets/${id}/thumbnail\``（或 `/file`）；若走 JWT 无 Cookie，可用短期签名 query：`/file?token=...`。

**权限**：`scope=system` 元数据全员可读；**二进制流**与 `mine` 列表仍按工作区/用户隔离；禁止遍历他人 `id`。

---

## 七、Service 层

### 7.1 已有实现

```go
// internal/service/avatar.go

type AvatarService struct {
    v1.UnimplementedAvatarServiceServer
    uc *biz.AvatarUsecase
}

func NewAvatarService(uc *biz.AvatarUsecase) *AvatarService

func (s *AvatarService) ListAvatarAssets(ctx, req) (*ListAvatarAssetsResponse, error)
func (s *AvatarService) CreateAvatarAsset(ctx, req) (*AvatarAsset, error)
func (s *AvatarService) GetAvatarFile(ctx, req) (*GetAvatarBlobResponse, error)
func (s *AvatarService) GetAvatarThumbnail(ctx, req) (*GetAvatarBlobResponse, error)
func (s *AvatarService) DeleteAvatarAsset(ctx, req) (*emptypb.Empty, error)
func (s *AvatarService) RefreshChannelPlatformIcons(ctx, req) (*RefreshChannelPlatformIconsResponse, error)
```

### 7.2 引用检查扩展

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

## 八、Wire 注入

当前注入（`cmd/admin/wire_gen.go`）：

```go
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

## 九、Web 前端设计

### 9.1 文件结构

```
web/src/
├── features/avatar/
│   ├── api.ts                       ← API 调用：上传、列表、缩略图、渠道图标刷新
│   ├── iconModel.ts                 ← icon 类型判断工具函数
│   ├── types.ts                     ← AvatarAsset 类型定义
│   ├── index.ts                     ← 统一导出
│   ├── useAgentAvatarPreview.ts     ← icon → QAvatar 的 icon + img src 映射
│   ├── useAvatarThumbnailSrc.ts     ← icon → 缩略图 data URL 响应式解析
│   ├── useAvatarPickerDialog.ts     ← Picker 弹层状态管理
│   ├── prepareAvatarUpload.ts       ← 上传前 Canvas 居中裁剪 + 缩放（512px）
│   └── resolveAvatarAssetFetchId.ts ← agent.icon → catalog asset id 解析
├── components/avatar/
│   ├── AgentAvatarPicker.vue        ← 双 Tab 选择器 + 上传（无交互式裁剪）
│   ├── AgentAvatarQ.vue             ← QAvatar 封装 + 占位图标
│   └── ResolvedAvatarImg.vue        ← 纯 img 展示
├── stores/avatar/
│   └── index.ts                     ← useAvatarCatalogStore
└── components/agents/
    ├── AgentCreateDialog.vue
    └── AgentSettingsHeader.vue
```

### 9.2 AgentAvatarPicker.vue

当前 Picker 功能：
- `QDialog` 弹层，含 system/mine 两个 Tab
- 支持 agent/channel 两种 scope（通过 `useAvatarPickerDialog` 的 `scope` 参数）
- 文件上传（`<input type="file">`）→ `prepareAvatarUploadFile` 自动中心裁剪 → 直接上传
- **无交互式裁剪器**（`vue-advanced-cropper` 未安装）

Props：

| Props | 说明 |
|--------|------|
| `modelValue` | 当前选中的 **`avatar_assets.id`**（与写入 `agents.icon` 的值一致）。 |
| `open` | 弹层开关（双向绑定 `update:open`）。 |
| `scope` | `'agent'` \| `'channel'`，默认 `'agent'`。 |

Emits：

| Emits | 说明 |
|--------|------|
| `update:modelValue` | 用户确认选用某资源后的 **头像资源 id**。 |
| `update:open` | 弹层开关变化。 |

### 9.3 交互式裁剪扩展

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
│  │  │           图片裁剪区域                       │  │   │
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

### 9.4 删除确认增强

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

### 9.5 ResolvedAvatarImg.vue

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

### 9.6 AgentAvatarQ.vue

封装 `<q-avatar>` 的展示组件：

- 通过 `useAgentAvatarPreview` 解析 icon → 缩略图 src + Quasar icon name
- 有缩略图时渲染 `<img>`，否则用 Material icon 占位（默认 `smart_toy`）

Props：`icon`、`alt`、`size`（默认 `'56px'`）、`rounded`、`avatarClass`。

---

*设计文档版本：与 `50-avatar.md` 需求规格、`50-avatar.development.md` 开发计划成对维护。*
