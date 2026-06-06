# Agent 头像 — 管理组件与存储库设计

本文档定义 **头像资源库**（持久化）与 **头像管理 UI**（组件）。**不设独立「头像管理」路由页**；仅在 **Agent 创建 / 编辑** 流程中，用户点击头像（`QAvatar`）时弹出组件进行选择、上传与裁剪。

### 与 `2 agents-create.md` 的对应关系（双向关联）

| 文档 | 职责 |
|------|------|
| **`2 agents-create.md` §3.8 头像** | 产品触点：头像在表单中的位置（与显示名称同区）、`QAvatar` 尺寸、点击行为摘要、**`agents.icon` 存 `avatar_assets.id`**。 |
| **`50 Avatar.md`（本文）** | 实现规格：`avatar_assets` 表（BLOB）、**`AgentAvatarPicker`** 弹层、裁剪与上传、**只读出图 API**、删除与引用规则。 |

修改头像方案时，请 **同时核对** 上述两处，避免表单字段与存储/API 脱节。

---

## 1. 设计目标

| 项目 | 说明 |
|------|------|
| **入口** | 创建或编辑 Agent 表单内，点击 **头像区域**（约 80–100px `QAvatar`）。 |
| **形态** | `QDialog`（或 `QBottomSheet` 移动端），内嵌选图、上传、裁剪、确认。 |
| **无独立后台页** | 不提供 `/settings/avatars` 类全屏管理；「我的头像」仅在弹层内以网格维护（可选删除自己上传的项）。 |
| **落库** | **图片字节存数据库**（`avatar_assets.image_data` BLOB）；Agent 侧仅存 **`avatar_assets.id`**（写入 `agents.icon` 或独立列 `icon_asset_id`，见 §5）。 |

---

## 2. 前端组件

### 2.1 建议命名与职责

| 名称 | 职责 |
|------|------|
| **`AgentAvatarPicker.vue`**（或 `AgentAvatarManagerDialog.vue`） | 头像弹层：展示内置图、当前用户已上传图、上传入口、裁剪、选中回写。 |
| **父级（创建/编辑表单）** | 维护头像 **资源 id**（及本地预览 `objectURL` 仅用于裁剪流程）；点击 `QAvatar` 时 `pickerOpen = true`；监听 `@confirm` 更新 `v-model`。 |

### 2.2 Props / Emits（约定）

| Props | 说明 |
|--------|------|
| `modelValue` | 当前选中的 **`avatar_assets.id`**（与写入 `agents.icon` 的值一致）。 |
| `workspaceId` | 多租户时传入，用于拉取「我的」资源列表。 |

| Emits | 说明 |
|--------|------|
| `update:modelValue` | 用户确认选用某资源后的 **头像资源 id**。 |
| `cancel` | 关闭且不修改（可选）。 |

### 2.3 弹层内布局（Quasar）

```
┌─────────────────────────────────────────────────────────┐
│  选择头像                                          [×]   │
├─────────────────────────────────────────────────────────┤
│  [ 内置 ]  [ 我的上传 ]     （QTabs）        │
├─────────────────────────────────────────────────────────┤
│  ┌────┐ ┌────┐ ┌────┐   网格 QAvatar / QImg + 选中描边      │
│  └────┘ └────┘ └────┘                                    │
│  …                                                         │
├─────────────────────────────────────────────────────────┤
│  [ 从本地上传 ]  （QUploader 或隐藏 input + QBtn）          │
├─────────────────────────────────────────────────────────┤
│                    [ 取消 ]  [ 确定 ]                      │
└─────────────────────────────────────────────────────────┘
```

| 区域 | 组件建议 |
|------|-----------|
| 容器 | `QDialog` + `QCard`，`max-width` 约 480–560px |
| 分区 | `QTabs`：`内置`（`is_system = 1`）/ `我的上传`（当前用户 + 工作区） |
| 网格 | `div.row` + `QAvatar`/`QImg` `size` 统一，选中项 `selected` class 或 `QBadge` 勾 |
| 上传 | 选文件后进入 **裁剪子步骤**（见 §2.4），确认后再 `POST` 入库 |
| 底部 | `QCardActions`：`取消` 关闭不写回；`确定` 将当前选中项 `emit` 给父组件 |

### 2.4 裁剪流程

| 步骤 | 说明 |
|------|------|
| 1 | 用户选择本地图片 → 在 **`QDialog` 嵌套** 或 **全屏 Step** 中展示裁剪区。 |
| 2 | 推荐库：`vue-advanced-cropper`（固定 1:1）；导出 `blob`/`dataURL`。 |
| 3 | 上传 `multipart/form-data` 至 `POST /avatar-assets`（§4），服务端写入 **BLOB** 后返回 **`id`**（无外链 URL）。 |
| 4 | 自动选中该项，网格缩略图使用 **§4 只读图片 URL**；用户再点「确定」把 `id` 回写给父级。 |

**约束（产品）**：仅允许 **正方形或裁剪为正方形**；最大边长如 512px（服务端可再压图）。

### 2.5 与创建 / 编辑表单的衔接

| 场景 | 行为 |
|------|------|
| **创建** | 默认头像为内置占位或空；点击打开 `AgentAvatarPicker`；确认后 `icon` 参与创建 `POST`。 |
| **编辑** | `QAvatar` 的 `src` 指向 **`GET /avatar-assets/{agents.icon}/file`**（或等价路径）；点击可更换；未点确定关闭则保留原值。 |
| **列表/会话** | `src` 同上；需 **登录态 Cookie** 或 **签名 query**；不打开本组件。 |

---

## 3. 存储库：`avatar_assets` 表

头像 **像素数据直接落在数据库**（`BLOB`），不依赖对象存储或本地文件目录。内置图与用户上传 **同一套表结构**；预置数据由迁移脚本插入二进制。

### 3.1 DDL（SQLite 3 示例）

```sql
CREATE TABLE avatar_assets (
    id                 TEXT PRIMARY KEY,

    -- 主图（必填）：裁剪后的正方形位图
    image_data         BLOB NOT NULL,
    mime_type          TEXT NOT NULL CHECK (mime_type IN (
                            'image/jpeg', 'image/png', 'image/webp'
                        )),

    -- 可选：列表/网格缩略图，减轻列表页拉大图压力；可与 image_data 相同
    thumbnail_data     BLOB,

    workspace_id       TEXT,
    owner_user_id      TEXT,
    source             TEXT NOT NULL CHECK (source IN ('system', 'upload')),
    is_system          INTEGER NOT NULL DEFAULT 0 CHECK (is_system IN (0, 1)),

    file_size_bytes    INTEGER,
    width_px           INTEGER,
    height_px          INTEGER,

    created_at         TEXT NOT NULL DEFAULT (datetime('now')),
    deleted_at         TEXT
);

CREATE INDEX idx_avatar_assets_workspace_owner
    ON avatar_assets (workspace_id, owner_user_id)
    WHERE deleted_at IS NULL;

CREATE INDEX idx_avatar_assets_system
    ON avatar_assets (is_system)
    WHERE deleted_at IS NULL AND is_system = 1;
```

### 3.2 列说明

| 列 | 说明 |
|----|------|
| `image_data` | **主头像二进制**；列表详情展示默认从此读流（或通过缩略图列）。 |
| `thumbnail_data` | 可选；例如 64×64 / 128×128，**网格选图**与 **Agent 列表卡片** 优先用缩略图接口以降低带宽。 |
| `mime_type` | 与 `image_data` 一致，供 HTTP `Content-Type` 使用。 |
| `source` / `is_system` | 同前：预置与上传区分、保护预置不可被普通用户删。 |

### 3.3 引用与删除

| 规则 | 说明 |
|------|------|
| **Agent 引用** | `agents.icon` **存 `avatar_assets.id`（TEXT/UUID）**；可选增加 `FOREIGN KEY (icon) REFERENCES avatar_assets(id)`（SQLite 需 `PRAGMA foreign_keys=ON`）。 |
| **删除上传** | 软删 `avatar_assets` 前检查是否有 `agents.icon` 指向该 id；**有引用则禁止删除**或先让用户改头像（推荐）。 |
| **硬删** | 清理软删记录时一并丢弃 BLOB，缩小库文件（VACUUM 策略由运维定）。 |

### 3.4 BLOB 与库体积

| 项 | 说明 |
|----|------|
| **限额** | 单张原图建议仍压至 ≤512px、≤200KB 级再入库，避免单行过大拖慢备份。 |
| **缩略图** | 强烈建议维护 `thumbnail_data`，列表只读小图。 |
| **PostgreSQL** | 对应类型为 `BYTEA`；其余语义不变。 |

---

## 4. API 建议

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/avatar-assets` | Query：`scope=system|mine`，`workspace_id`；**仅返回元数据**：`id`、`mime_type`、`width_px` 等；**不返回 base64**，避免列表爆体。 |
| GET | `/avatar-assets/:id/file` | **读主图**：响应体为二进制，`Content-Type` = `mime_type`；供 `<img :src="...">` 或 `QImg`。需鉴权或与会话 Cookie 同源。 |
| GET | `/avatar-assets/:id/thumbnail` | **读缩略图**（若无缩略图可 302 或回退为 `/file`）。 |
| POST | `/avatar-assets` | `multipart` 上传；校验类型/大小；**可选服务端再压图**；写入 `image_data`（及 `thumbnail_data`）；返回 **`{ id }`**。 |
| DELETE | `/avatar-assets/:id` | 软删；校验权限与 **Agent 引用**。 |
| POST | `/avatar-assets/channel-platform-icons:refresh` | 从 Iconify API 重新获取渠道平台图标并更新 DB；返回 `{ updated, failed }`。 |

**前端 `src` 写法示例**：`const avatarSrc = (id) => \`/api/avatar-assets/${id}/thumbnail\``（或 `/file`）；若走 JWT 无 Cookie，可用短期签名 query：`/file?token=...`。

**权限**：`scope=system` 元数据全员可读；**二进制流**与 `mine` 列表仍按工作区/用户隔离；禁止遍历他人 `id`。

---

## 5. `agents.icon` 约定

| 策略 | 说明 |
|------|------|
| **推荐** | 存 **`avatar_assets.id`**（与 `2 agents-create.md` 中 `agents.icon` 字段兼容：类型仍为字符串，**语义为资源主键而非 URL**）。 |
| **禁止** | 不再把外链 URL 写入 `icon`（历史数据可一次性迁移：下载转 BLOB 入库或清空让用户重选）。 |
| **展示** | 所有 `QAvatar`/`QImg` 使用 **§4 只读接口 URL**，不要拼静态 CDN。 |

内置预置数据：迁移脚本向 `avatar_assets` 插入 **`image_data`/`thumbnail_data` BLOB**（`is_system=1`，`owner_user_id` 为空）。

---

## 6. 校验与限额（建议）

| 项 | 建议值 |
|----|--------|
| 文件类型 | `image/jpeg`、`image/png`、`image/webp` |
| 大小 | ≤ 2 MB（上传前前端校验 + 服务端再验） |
| 输出边长 | 256–512 px 正方形 |
| 列表分页 | 内置一次加载；上传较多时 `mine` 分页或虚拟滚动 |

---

## 7. 验收要点

- [ ] 创建/编辑页点击头像仅通过弹层完成选图，**无独立管理页**亦可完成闭环。  
- [ ] 内置 / 我的上传 分区清晰；选中态与「确定」回写正确。  
- [ ] 上传 → 裁剪 → **BLOB 入库** → `agents.icon` 写入资源 id → 列表/表单通过 **`/file` 或 `/thumbnail`** 正常显示。  
- [ ] 预置资源不可被普通用户删除；用户资源删除有引用保护或明确策略。  
- [ ] 与 `2 agents-create.md` §3.8 行为一致（图库 + 上传 + 裁剪 + 写入 icon）。  

---

*文档版本：**头像二进制存 `avatar_assets` BLOB**；`agents.icon` 存资源 id；只读接口出图。Quasar + SQLite 示例，PostgreSQL 可用 `BYTEA`。产品表单触点见 **`2 agents-create.md` §3.8**。*

