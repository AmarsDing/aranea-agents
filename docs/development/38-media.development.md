# 38 Media Provider — 开发计划

> **版本**：1.0（2026-08-15）
> **同系列**：需求 → [`38-media.md`](./38-media.md)；设计 → [`38-media.design.md`](./38-media.design.md)
> **定位**：文生图 / 文生视频 / 图生视频的 Provider + 工具 + 落盘装饰器。代码已落地；本文件记录锚点、现状与已知差距。

---

## 1. 模块定位

为 Chat/Team Agent 提供媒体生成工具。运行时从 `media_providers` 按能力选提供方，经 `PersistingProvider` 把远程临时文件落入会话制品。前端在观测节点与工具卡片中预览。

---

## 2. 代码锚点

下列路径均已存在（DOC-SYNC-6）。

### 2.1 Provider

| 路径 | 说明 |
|------|------|
| `internal/provider/media/provider.go` | `MediaProvider` 接口与请求/结果类型 |
| `internal/provider/media/registry.go` | Register / Get / ProviderConfig |
| `internal/provider/media/qwen.go` | 通义万相；`init` 注册 `qwen` |
| `internal/provider/media/comfyui_local.go` | 本地 ComfyUI；`init` 注册 `comfyui_local` |
| `internal/provider/media/persist.go` | `PersistingProvider` + `artifact://` |
| `internal/provider/media/provider_test.go` | 接口 / 注册表 / Name |
| `internal/provider/media/persist_test.go` | 落盘与降级 |

**不存在**：`internal/provider/media/kling.go`（类型名仅出现在注释与 schema）。

### 2.2 工具与装配

| 路径 | 说明 |
|------|------|
| `internal/tools/media/generate_image.go` | `generate_image` |
| `internal/tools/media/generate_video.go` | `generate_video` |
| `internal/tools/media/image_to_video.go` | `image_to_video` |
| `internal/tools/media/progress.go` | 进度接口；`PublishProgress` 为空实现 |
| `internal/tools/media/media_test.go` | 工具执行单测 |
| `internal/agent/tool_assembly_media.go` | `resolveMediaTools` |
| `internal/agent/tool_assembly.go` | 调用 resolve，写入 CustomTools |
| `internal/agent/builder_deps.go` | `MediaProviders` / `ArtifactWriter` |
| `internal/runtime/deps.go` | `TurnReadDeps.MediaProviders` |
| `internal/data/builtin_tools_seed.go` | 三条 media 种子（约 L81–83） |
| `internal/biz/agent_tool_policy.go` | 三工具策略标记 |

### 2.3 目录与持久化

| 路径 | 说明 |
|------|------|
| `internal/biz/media/media.go` | `ProviderReader` 端口 |
| `internal/data/media_provider_repo.go` | Ent 实现 |
| `internal/data/ent/schema/media_provider.go` | Schema |
| `internal/data/sql/migrations/20261008_media_providers.sql` | DDL |
| `internal/data/ddl_migration_registry.go` | 版本 20261014 `media_providers` |
| `internal/data/data.go` | Wire：`NewMediaProviderRepo` |
| `internal/event/flow_log.go` | `media.generate` 标题登记 |

### 2.4 前端

| 路径 | 说明 |
|------|------|
| `web/src/features/chat/mediaTypes.ts` | 类型与工具名常量 |
| `web/src/features/chat/useMediaUrl.ts` | `artifact://` 换签 |
| `web/src/features/chat/__tests__/useMediaUrl.spec.ts` | 换签单测 |
| `web/src/components/chat/observe/NodeMediaPreview.vue` | 节点缩略图 |
| `web/src/components/chat/observe/MediaLightbox.vue` | 灯箱 |
| `web/src/components/chat/tools/MediaToolDetail.vue` | 工具详情 |
| `web/src/stores/chat/activityV2Store.ts` | 从活动流提取媒体 |
| `web/src/components/chat/tools/classifyTool.ts` | 分类 |

无 `api/kratos/media`。无媒体提供方管理页。

---

## 3. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| 接口 + 注册表 | ✅ | `provider.go` / `registry.go` + 单测 |
| Qwen 文生图 | ✅ | `qwen.go` DashScope 异步 |
| ComfyUI 文生图 | ✅ | `comfyui_local.go` prompt/history |
| Qwen/ComfyUI 视频 | 未实现（接口返回错误） | 两文件 `GenerateVideo` / `ImageToVideo` |
| Kling | 无代码 | 无 `kling.go`，未 `Register` |
| 落盘装饰器 | ✅ | `persist.go` + `persist_test.go` |
| 三工具 + 种子 | ✅ | tools/media + builtin_tools_seed |
| 按能力装配 | ✅ | `tool_assembly_media.go` |
| 流程日志 option | 代码有，装配未接 | `WithMediaFlowBus` 未被 `resolveMediaTools` 调用 |
| 进度上报 | 占位 | `PublishProgress` return nil |
| 前端预览 / 换签 | ✅ | observe + useMediaUrl |
| 管理 API / 种子行 | 无 | 空表则工具被跳过 |

---

## 4. 已知差距（非本轮实现）

| ID | 项 | 说明 |
|----|----|------|
| MEDIA-GAP-1 | 视频实现 | Qwen / ComfyUI 的 GenerateVideo、ImageToVideo |
| MEDIA-GAP-2 | Kling | 若产品需要，再补构造器与 Register |
| MEDIA-GAP-3 | FlowBus | `resolveMediaTools` 传入 `WithMediaFlowBus` |
| MEDIA-GAP-4 | 进度 | `progress.go` 接到工具执行上下文 |
| MEDIA-GAP-5 | 提供方 CRUD | 若运维需要 UI/API，另立需求；当前改表 |
| MEDIA-GAP-6 | ComfyUI Size | 工作流宽高写死 1024，未映射请求 Size |

P2-20 只补文档，不改上述代码。

---

## 5. 任务清单（文档轮）

| # | 任务 | 状态 |
|---|------|------|
| 1 | 补 `38-media.md` / `.design.md` / `.development.md` | ✅ 2026-08-15 |
| 2 | 交叉参考 §1.36 与编号表链到三件套 | ✅ |
| 3 | 视频 / Kling / FlowBus / 进度 | 📋 代码差距，非本轮 |

---

## 6. 验收与验证

- 锚点路径均存在。
- `go test ./internal/provider/media/... ./internal/tools/media/...`
- 前端：`useMediaUrl.spec.ts` 覆盖 `artifact://`。

---

## 7. 改动文件清单（P2-20）

| 文件 | 动作 |
|------|------|
| `docs/development/38-media.md` | 新建 |
| `docs/development/38-media.design.md` | 新建 |
| `docs/development/38-media.development.md` | 新建 |
| `docs/development/65-module-cross-reference-full.md` | 更新 Media 卡与编号表 |

---

*文档版本：1.0 — 2026-08-15。*
