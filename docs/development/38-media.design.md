# 38 Media Provider — 设计文档

> **版本**：1.0（2026-08-15）
> **同系列**：需求 → [`38-media.md`](./38-media.md)；开发计划 → [`38-media.development.md`](./38-media.development.md)
> **规范**：独立于 LLM Provider（`internal/provider/` 文本模型）。媒体生成是异步长任务（submit → poll → fetch）。

---

## 0. 架构决策

| # | 决策 | 理由 |
|---|------|------|
| ADR-M-01 | 独立 `MediaProvider` 接口，不复用 LLM Provider | 调用形态不同（异步轮询 vs 同步/流式文本） |
| ADR-M-02 | 无独立 proto；只走 Agent 工具 | 生成发生在 Turn 内，结果进工具输出与制品 |
| ADR-M-03 | 注册表 + `init()` 构造器 | 与额外 LLM provider 的 Register 模式一致 |
| ADR-M-04 | `PersistingProvider` 装饰器落盘 | 远程临时 URL 会过期；失败降级保留原 URL |
| ADR-M-05 | 按能力选「最早创建的 active」提供方 | 目录简单；不做加权路由 |

---

## 1. 总体架构

```
Chat / Team Turn
    │
    ▼
internal/agent/tool_assembly_media.go
    │  ActiveProviderFor(capability)
    ▼
internal/biz/media.ProviderReader  ←  internal/data/media_provider_repo.go
    │  Get(provider_type, cfg)
    ▼
internal/provider/media.Registry
    ├─ qwen          (DashScope 文生图)
    └─ comfyui_local (ComfyUI HTTP 文生图)
         │
         ▼
    PersistingProvider（装饰器）
         │  Save → artifact://<id>
         ▼
    trpc function tool  (generate_image / generate_video / image_to_video)
         │
         ▼
前端：mediaTypes / useMediaUrl / NodeMediaPreview / MediaLightbox / MediaToolDetail
```

**依赖方向**：tools/media → provider/media；agent 装配 → biz/media 端口 ← data。biz 不依赖具体 Qwen/ComfyUI。

---

## 2. 核心类型

### 2.1 `MediaProvider`（`internal/provider/media/provider.go`）

```go
type MediaProvider interface {
    Name() string
    GenerateImage(ctx context.Context, req ImageRequest) (*ImageResult, error)
    GenerateVideo(ctx context.Context, req VideoRequest) (*VideoResult, error)
    ImageToVideo(ctx context.Context, req ImageToVideoRequest) (*VideoResult, error)
}
```

| 请求 | 主要字段 |
|------|----------|
| `ImageRequest` | Prompt, Size（`1024x1024` / `1792x1024` / `1024x1792`）, Style, Count(1–4), Seed |
| `VideoRequest` | Prompt, DurationMs, FPS, Resolution（`720p` / `1080p`）, Seed |
| `ImageToVideoRequest` | ImageArtifactID, Prompt, DurationMs, FPS |

`MediaArtifact`：`artifact_id` / `url` / `mime_type` / 可选宽高、时长、thumbnail。

### 2.2 注册表（`registry.go`）

`ProviderConfig`：Name、ProviderType（`comfyui_local` / `qwen` / `kling`）、BaseURL、APIKey、Extra。

`Register(name, constructor)` 在各实现 `init()` 中调用。`Get` 按 **ProviderType** 取构造器。`kling` 仅出现在类型注释中，**没有** `Register("kling", …)`。

### 2.3 实现对照

| ProviderType | 文件 | GenerateImage | GenerateVideo / ImageToVideo |
|--------------|------|---------------|------------------------------|
| `qwen` | `qwen.go` | DashScope `wanx2.1-t2i-turbo`，`X-DashScope-Async`，3s 轮询、3 分钟超时 | 返回 `not yet implemented` |
| `comfyui_local` | `comfyui_local.go` | `POST {BaseURL}/prompt` + `GET /history/{prompt_id}`；固定 SD1.5 checkpoint 工作流 | 返回 `not yet implemented` |

Qwen 默认尺寸 `1024*1024`（DashScope 用 `*`）；工具层默认 `1024x1024`（`x`）。ComfyUI 工作流宽高写死 1024，忽略请求 Size。

### 2.4 `PersistingProvider`（`persist.go`）

装饰内层 Provider：对每条 `http(s)` URL 下载（30s、体积受制品上限约束）→ `artifact.Saver.Save` → 改写 `ArtifactID` 与 `URL=artifact://<id>`。

- 无 Saver、非 http(s)、无 session ID、下载/落盘失败：保留原 artifact，打 Warn，**不**失败工具。
- Session ID 来自 `trpcagent.InvocationFromContext`。
- 可选 `WithMediaFlowBus` 发流程日志 `media.generate`（start/done/error）。装配路径当前未传入该 option（见开发计划差距）。
- 日志：远程 URL 只记 sha256 前 12 位；prompt extras 截断 50 字。

---

## 3. 工具契约

无独立 REST。工具为 trpc `function.Tool`：

| 工具名 | 文件 | 输入 | 默认 |
|--------|------|------|------|
| `generate_image` | `internal/tools/media/generate_image.go` | prompt（必填）, size, style, count | size=`1024x1024`, count=1（上限 4） |
| `generate_video` | `generate_video.go` | prompt（必填）, duration_ms, fps, resolution | 5000ms / 24 / `720p` |
| `image_to_video` | `image_to_video.go` | image_artifact_id（必填）, prompt, duration_ms, fps | 5000ms / 24 |

`mp == nil` 时返回 `media provider not configured`。装配层在无提供方时**不会**注册该工具，此错误主要出现在单测或未走装配的调用。

进度：`progress.go` 的 `PublishProgress` 目前为空实现（返回 nil）。

---

## 4. 装配

`internal/agent/tool_assembly.go` 在有效工具集上调用 `resolveMediaTools`（`tool_assembly_media.go`）：

1. `deps.MediaProviders == nil` → 整段跳过；
2. 对每个已启用 key，`ActiveProviderFor(capability)`；
3. `mediaprovider.Get(cfg.ProviderType, …)`；
4. `NewPersistingProvider(inner, deps.ArtifactWriter, lg)`（无 FlowBus）；
5. 任一步失败：Warn 并跳过该工具，**不**失败 Agent 构建。

能力映射：`generate_image`→`image`，`generate_video`→`video`，`image_to_video`→`image_to_video`。

注入：`TRPCBuilderDeps.MediaProviders` / `ArtifactWriter`；`runtime.TurnReadDeps.MediaProviders`；Wire：`data.NewMediaProviderRepo`。

种子：`internal/data/builtin_tools_seed.go` 三条，`category=media`，`enabled=false`，`riskLevel=medium`。`agent_tool_policy.go` 将三者标为需策略感知的工具。

---

## 5. 数据模型

表 `media_providers`（Ent `internal/data/ent/schema/media_provider.go`；DDL `internal/data/sql/migrations/20261008_media_providers.sql`；迁移版本 20261014）。

| 列 | 说明 |
|----|------|
| id | 主键 |
| name | 唯一显示名 |
| provider_type | `comfyui_local` / `qwen` /（预留）`kling` |
| base_url | DashScope 或 ComfyUI 根 URL |
| api_key | 敏感字段 |
| config_json | 提供方 Extra，默认 `{}` |
| capabilities | JSON 数组，如 `["image"]` |
| status | 默认 `active` |
| created_at / updated_at | 文本时间戳 |

`ProviderReader.ActiveProviderFor`：`status=active`，按 `created_at` 升序，返回第一条 `Supports(cap)` 的行；否则 `NotFound`。

无种子行：空表则所有媒体工具在装配时被跳过。无 `api/kratos` Media 服务。

Biz 端口：`internal/biz/media/media.go` 的 `ProviderReader`（Stability: evolving）。

---

## 6. 前端

无独立 Media 页面。消费方：

| 路径 | 职责 |
|------|------|
| `web/src/features/chat/mediaTypes.ts` | `MediaArtifact`、`MEDIA_TOOL_NAMES` 单一来源 |
| `web/src/features/chat/useMediaUrl.ts` | `artifact://` → 签名下载 URL（缓存 10min）；http(s) 透传 |
| `web/src/components/chat/observe/NodeMediaPreview.vue` | 节点最多 3 张缩略图 |
| `web/src/components/chat/observe/MediaLightbox.vue` | 放大预览 |
| `web/src/components/chat/tools/MediaToolDetail.vue` | 工具卡片详情 |
| `web/src/stores/chat/activityV2Store.ts` | 按工具名提取媒体产物 |
| `web/src/components/chat/tools/classifyTool.ts` | 媒体工具分类 |

观测画布组件在 `web/src/components/chat/observe/`，归属架构图 39，不是本模块文档范围。

---

## 7. 事件与日志

| 通道 | 内容 |
|------|------|
| 工具结果 | 普通 Chat/Team 工具事件，前端从 tool result 抽 `artifacts` |
| 流程日志 | step id `media.generate`（`internal/event/flow_log.go` 已登记标题「媒体生成」） |
| 进程日志 | 失败 Error；落盘 Info（artifact_id / bytes / url_hash）；跳过 Warn |

---

## 8. 与 LLM Provider(9) / Artifact(27) 边界

- 禁止把媒体模型登记进 LLM catalog，或反过来用 LLM Provider 调 DashScope 文生图。
- 制品存储、签名下载、体积上限归 Artifact(27)；本模块只调用 `Saver` 并改写 URL。

---

*文档版本：1.0 — 2026-08-15；按仓库代码现状编写。*
