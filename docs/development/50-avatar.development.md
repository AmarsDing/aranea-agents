# Avatar 头像 — 开发计划

> **版本**：2026-06-17 | **状态**：核心功能已实现，增强功能待开发
> **需求**：[50-avatar.md](./50-avatar.md) · **设计**：[50-avatar.design.md](./50-avatar.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：—

---

## 1. 模块定位

Avatar 头像：管理 Agent/Team 的头像，支持上传、裁剪、存储和展示。

**代码锚点**：
- `api/kratos/avatar/v1/avatar.proto` — Avatar Proto 定义
- `internal/biz/avatar/` — Avatar Usecase + 图片处理（`avatar.go`, `image.go`）
- `internal/biz/avatar_channel_refresh.go` — 渠道图标刷新实现
- `internal/biz/avatar_channel_seed.go` — 内置渠道图标 seed
- `internal/biz/avatar_agent_seed.go` — 内置 Agent 头像 seed
- `internal/biz/avatar.go` — Biz re-export（向后兼容）
- `internal/data/avatar.go` — Avatar Repo 实现
- `internal/data/ent/schema/avatar_asset.go` — Ent Schema
- `internal/service/avatar.go` — Avatar Service
- `cmd/admin/wire_gen.go` — Wire 注入
- `web/src/features/avatar/` — 前端 API + Composables
- `web/src/components/avatar/` — 前端组件
- `web/src/stores/avatar/index.ts` — Pinia Store

---

## 2. 现状评估

### 2.1 各层实现状态

| 层 | 状态 | 文件 |
|----|------|------|
| Proto | ✅ 已有 | `api/kratos/avatar/v1/avatar.proto`（6 RPC） |
| Biz | ✅ 已有 | `internal/biz/avatar/` 子包 + `internal/biz/avatar_*.go` |
| Data | ✅ 已有 | `internal/data/avatar.go`, `internal/data/ent/schema/avatar_asset.go` |
| Service | ✅ 已有 | `internal/service/avatar.go` |
| Wire | ✅ 已有 | `cmd/admin/wire_gen.go` 已注册 |
| Web API | ✅ 已有 | `web/src/features/avatar/api.ts` |
| Web Store | ✅ 已有 | `web/src/stores/avatar/index.ts` |
| Web 组件 | ✅ 已有 | `web/src/components/avatar/AgentAvatarPicker.vue`, `ResolvedAvatarImg.vue`, `AgentAvatarQ.vue` |

### 2.2 功能实现状态

| 功能 | 状态 | 证据 |
|------|------|------|
| Avatar CRUD 全链路 | ✅ | Proto 6 RPC → Service → Usecase → Repo → 前端 API + Store + 组件 |
| 图片自动中心裁剪 + 压缩 | ✅ | 前端 `prepareAvatarUploadFile` + 后端 `ProcessAvatarUpload`（512px 主图 + 128px 缩略图） |
| 缩略图自动生成 | ✅ | 后端 `resizeSquare(128)` + `thumbnail_data` 列 |
| 渠道平台图标刷新 | ✅ | `channelIconRefresher`：Iconify API → SVG → PNG → upsert |
| 内置头像 Seed | ✅ | `EnsureAgentAvatars` + `EnsureChannelPlatformAvatars` |
| 头像选择器 | ✅ | `AgentAvatarPicker`：双 Tab、分组、上传、选择确认 |
| 头像展示 | ✅ | `ResolvedAvatarImg` + `AgentAvatarQ` |
| 交互式裁剪 | ❌ | `vue-advanced-cropper` 未安装，当前为自动中心裁剪 |
| Agent 引用检查 | ❌ | `CheckReferences` / `FindByIcon` 不存在 |
| 内置头像删除保护 | ❌ | 服务端未校验 `is_system` |
| CDN 加速 | ❌ | 图片直接从 DB 读取，无 CDN 层 |

### 2.3 API 端点状态

| 方法 | 路径 | 状态 |
|------|------|------|
| GET | `/v1/avatar-assets` | ✅ |
| GET | `/v1/avatar-assets/:id/file` | ✅ |
| GET | `/v1/avatar-assets/:id/thumbnail` | ✅ |
| POST | `/v1/avatar-assets` | ✅ |
| DELETE | `/v1/avatar-assets/:id` | ✅（部分，无引用检查） |
| POST | `/v1/avatar-assets/channel-platform-icons:refresh` | ✅ |
| GET | `/v1/avatar-assets/:id/references` | ❌ 待实现 |

---

## 3. 差距与优化

1. **P1**：交互式裁剪 — 用户上传后可自选裁剪区域（`vue-advanced-cropper`），而非自动中心裁剪。
2. **P2**：Agent 引用检查 — 删除头像前检查是否有 Agent 引用，有引用时禁止删除。
3. **P3**：内置头像删除保护 — 服务端校验 `is_system=true` 时拒绝删除。
4. **P4**：CDN 加速 — 大图片加载慢，可考虑缓存层或 CDN。

---

## 4. 开发阶段

- **Phase 1**：交互式裁剪（前端 `vue-advanced-cropper` 集成）
- **Phase 2**：Agent 引用检查 + 删除保护（后端 + 前端）
- **Phase 3**：内置头像删除保护（后端）
- **Phase 4**：CDN 加速

---

## 5. 任务清单

| # | 任务 | 优先级 | 状态 | 涉及文件 |
|---|------|--------|------|----------|
| 1 | 安装 `vue-advanced-cropper` | P1 | ❌ | `web/package.json` |
| 2 | 新增 `AvatarCropperStep.vue` 裁剪子组件 | P1 | ❌ | `web/src/components/avatar/AvatarCropperStep.vue` |
| 3 | `AgentAvatarPicker.vue` 集成裁剪流程 | P1 | ❌ | `web/src/components/avatar/AgentAvatarPicker.vue` |
| 4 | Proto 新增 `CheckAvatarReferences` RPC | P2 | ❌ | `api/kratos/avatar/v1/avatar.proto` |
| 5 | Agent Repo 新增 `FindByIcon` 方法 | P2 | ❌ | `internal/data/agent_repo.go` |
| 6 | Usecase 新增 `CheckReferences` + `AgentCatalogRepository` 依赖 | P2 | ❌ | `internal/biz/avatar/avatar.go` |
| 7 | Service 实现 `CheckAvatarReferences` | P2 | ❌ | `internal/service/avatar.go` |
| 8 | Wire 注入更新 | P2 | ❌ | `cmd/admin/wire.go`, `cmd/admin/wire_gen.go` |
| 9 | 前端 API 新增 `checkAvatarReferences` | P2 | ❌ | `web/src/features/avatar/api.ts` |
| 10 | 前端删除确认交互 | P2 | ❌ | `web/src/components/avatar/AgentAvatarPicker.vue` |
| 11 | `DeleteAvatarAsset` 增加 `is_system` 校验 | P3 | ❌ | `internal/biz/avatar/avatar.go` |
| 12 | CDN 加速配置 | P4 | ❌ | — |

> 任务涉及的设计细节详见 [50-avatar.design.md](./50-avatar.design.md) 对应章节：
> - 任务 1-3：[§9.3 交互式裁剪扩展](./50-avatar.design.md#93-交互式裁剪扩展)
> - 任务 4-9：[§2.2 Proto 扩展](./50-avatar.design.md#22-proto-扩展引用检查)、[§3.9 引用检查扩展](./50-avatar.design.md#39-引用检查扩展)、[§4.6 AgentCatalogRepository.FindByIcon 扩展](./50-avatar.design.md#46-agentcatalogrepositoryfindbyicon-扩展)、[§7.2 引用检查扩展](./50-avatar.design.md#72-引用检查扩展)、[§9.4 删除确认增强](./50-avatar.design.md#94-删除确认增强)
> - 任务 11：[§3.8 DeleteAvatarAsset](./50-avatar.design.md#38-deleteavatarasset)

---

## 6. 验收标准

- [x] 头像 CRUD 全链路可用（列表/上传/文件获取/缩略图/删除/渠道图标刷新）
- [x] 服务端自动压缩上传图片至 512px 正方形
- [x] 服务端自动生成 128×128 缩略图存入 `thumbnail_data`
- [ ] 用户可在上传时交互式裁剪 Avatar（`vue-advanced-cropper`）
- [ ] 删除头像前检查 Agent 引用，有引用时禁止删除
- [ ] 前端删除操作有引用提示
- [ ] 内置头像不可被普通用户删除（`is_system=true` 保护）
- [ ] Avatar 通过 CDN 加速加载

---

## 7. 依赖与风险

| 项 | 说明 |
|----|------|
| `vue-advanced-cropper` | 需评估与 Quasar/QDialog 的兼容性 |
| `AgentCatalogRepository` | 引用检查需跨模块依赖 Agent Repo，需注意循环依赖 |
| BLOB 存储 | 大量头像时库体积增长，需定期 VACUUM |
| Wire 注入 | 任务 8 需修改 `cmd/admin/wire.go` 并重新生成 `wire_gen.go`（`make wire`） |

---

*开发计划版本：与 `50-avatar.md` 需求规格、`50-avatar.design.md` 设计文档成对维护。状态标记反映代码真实状态（DOC-SYNC-5）。*
